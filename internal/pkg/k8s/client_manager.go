/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package k8s

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// AuthMode K8s 主机认证模式。
// AuthMode selects how STX authenticates to a Kubernetes API server.
type AuthMode string

const (
	// AuthModeKubeconfig 使用完整 kubeconfig 内容认证（默认）。
	// AuthModeKubeconfig authenticates with full kubeconfig content (default).
	AuthModeKubeconfig AuthMode = "kubeconfig"
	// AuthModeToken 使用 API URL + Bearer Token（可选 CA）认证。
	// AuthModeToken authenticates with API URL + bearer token (optional CA).
	AuthModeToken AuthMode = "token"
	// AuthModeInCluster 控制面自身运行在集群内，使用 Pod ServiceAccount。
	// AuthModeInCluster uses the pod ServiceAccount when the control plane runs inside the cluster.
	AuthModeInCluster AuthMode = "in_cluster"
)

// Credentials 是构建 K8s 客户端所需的明文凭据（已解密）。
// Credentials holds decrypted plaintext credential material for building K8s clients.
type Credentials struct {
	AuthMode       AuthMode
	APIURL         string
	Namespace      string
	Kubeconfig     string
	Token          string
	CACert         string
	InsecureSkipTLS bool
}

// fingerprint 计算凭据指纹用于缓存失效；只哈希材料本身，不输出到日志。
// fingerprint hashes credential material for cache invalidation; it is never logged.
func (c *Credentials) fingerprint() string {
	h := sha256.New()
	h.Write([]byte(c.AuthMode))
	h.Write([]byte{0})
	h.Write([]byte(c.APIURL))
	h.Write([]byte{0})
	h.Write([]byte(c.Kubeconfig))
	h.Write([]byte{0})
	h.Write([]byte(c.Token))
	h.Write([]byte{0})
	h.Write([]byte(c.CACert))
	if c.InsecureSkipTLS {
		h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CredentialsLoader 按主机 ID 加载明文凭据；由 host 域注入实现，避免 pkg/k8s 反向依赖业务域。
// CredentialsLoader loads plaintext credentials by host ID; the host domain injects the
// implementation so pkg/k8s does not depend back on business domains.
type CredentialsLoader func(ctx context.Context, hostID uint) (*Credentials, error)

type clientEntry struct {
	fingerprint string
	restConfig  *rest.Config
	clientset   kubernetes.Interface
}

// ClientsetFactory 从 rest.Config 构建 Clientset；默认为 kubernetes.NewForConfig，
// 测试可注入 fake clientset 工厂。
// ClientsetFactory builds a clientset from a rest.Config; it defaults to
// kubernetes.NewForConfig and tests may inject a fake clientset factory.
type ClientsetFactory func(cfg *rest.Config) (kubernetes.Interface, error)

// ClientManager 按主机缓存 Clientset 与 RESTConfig；凭据指纹变化自动失效重建。
// ClientManager caches clientsets and REST configs per host; entries rebuild automatically
// when the credential fingerprint changes.
type ClientManager struct {
	loader  CredentialsLoader
	factory ClientsetFactory
	mu      sync.RWMutex
	cache   map[uint]*clientEntry
}

// NewClientManager 创建客户端管理器。
// NewClientManager creates a client manager.
func NewClientManager(loader CredentialsLoader) *ClientManager {
	return NewClientManagerWithFactory(loader, nil)
}

// NewClientManagerWithFactory 创建带自定义 Clientset 工厂的管理器（测试注入 fake clientset）。
// NewClientManagerWithFactory creates a manager with a custom clientset factory
// (tests inject fake clientsets).
func NewClientManagerWithFactory(loader CredentialsLoader, factory ClientsetFactory) *ClientManager {
	if factory == nil {
		factory = func(cfg *rest.Config) (kubernetes.Interface, error) {
			return kubernetes.NewForConfig(cfg)
		}
	}
	return &ClientManager{loader: loader, factory: factory, cache: map[uint]*clientEntry{}}
}

// Invalidate 主动失效指定主机的缓存（凭据更新、连接失败自愈时调用）。
// Invalidate drops the cached entry for a host (call after credential updates or on
// connection-failure healing).
func (m *ClientManager) Invalidate(hostID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, hostID)
}

// BuildRESTConfig 从凭据构建 rest.Config；kubeconfig 优先解析内嵌 API URL，token 模式要求显式 API URL。
// BuildRESTConfig builds a rest.Config from credentials; kubeconfig mode uses the embedded
// API URL, token mode requires an explicit API URL.
func BuildRESTConfig(cred *Credentials) (*rest.Config, error) {
	if cred == nil {
		return nil, fmt.Errorf("k8s: credentials are required")
	}
	mode := cred.AuthMode
	if mode == "" {
		mode = AuthModeKubeconfig
	}
	switch mode {
	case AuthModeInCluster:
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("k8s: in-cluster config unavailable: %w", err)
		}
		return cfg, nil
	case AuthModeKubeconfig:
		if strings.TrimSpace(cred.Kubeconfig) == "" {
			return nil, fmt.Errorf("k8s: kubeconfig content is required for kubeconfig auth mode")
		}
		cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(cred.Kubeconfig))
		if err != nil {
			return nil, fmt.Errorf("k8s: invalid kubeconfig: %w", err)
		}
		return cfg, nil
	case AuthModeToken:
		if strings.TrimSpace(cred.APIURL) == "" {
			return nil, fmt.Errorf("k8s: api url is required for token auth mode")
		}
		if strings.TrimSpace(cred.Token) == "" {
			return nil, fmt.Errorf("k8s: token is required for token auth mode")
		}
		cfg := &rest.Config{
			Host:        strings.TrimRight(strings.TrimSpace(cred.APIURL), "/"),
			BearerToken: strings.TrimSpace(cred.Token),
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: cred.InsecureSkipTLS,
			},
		}
		if strings.TrimSpace(cred.CACert) != "" {
			cfg.TLSClientConfig.CAData = []byte(cred.CACert)
		}
		return cfg, nil
	default:
		return nil, fmt.Errorf("k8s: unsupported auth mode %q", mode)
	}
}

func (m *ClientManager) entry(ctx context.Context, hostID uint) (*clientEntry, error) {
	cred, err := m.loader(ctx, hostID)
	if err != nil {
		return nil, err
	}
	fp := cred.fingerprint()

	m.mu.RLock()
	cached, ok := m.cache[hostID]
	m.mu.RUnlock()
	if ok && cached.fingerprint == fp {
		return cached, nil
	}

	restConfig, err := BuildRESTConfig(cred)
	if err != nil {
		return nil, err
	}
	// 客户端默认 10s 超时防止 API Server 假死拖死业务协程；长请求（log follow）自行控制。
	// A default 10s client timeout keeps a hung API server from pinning business goroutines;
	// long-running requests (log follow) manage their own deadlines.
	restConfig.Timeout = 30 * time.Second
	clientset, err := m.factory(restConfig)
	if err != nil {
		return nil, fmt.Errorf("k8s: build clientset failed: %w", err)
	}
	entry := &clientEntry{fingerprint: fp, restConfig: restConfig, clientset: clientset}
	m.mu.Lock()
	m.cache[hostID] = entry
	m.mu.Unlock()
	return entry, nil
}

// RESTConfig 返回指定主机的 rest.Config。
// RESTConfig returns the rest.Config for a host.
func (m *ClientManager) RESTConfig(ctx context.Context, hostID uint) (*rest.Config, error) {
	entry, err := m.entry(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return entry.restConfig, nil
}

// Clientset 返回指定主机的 Clientset。
// Clientset returns the clientset for a host.
func (m *ClientManager) Clientset(ctx context.Context, hostID uint) (kubernetes.Interface, error) {
	entry, err := m.entry(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return entry.clientset, nil
}

// RBACCheckItem 单项 RBAC 预检结果。
// RBACCheckItem is one RBAC precheck outcome.
type RBACCheckItem struct {
	Resource string `json:"resource"`
	Verb     string `json:"verb"`
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason,omitempty"`
}

// RBACRequiredRules 测试连接必须覆盖的资源与动作；与伴生 Proxy、常驻集群运维、日志读取的
// 按真实 K8s RBAC 模型拆填 Group/Resource/Subresource/Verb；services 不含 patch
//（Session 模式最小权限，Master HTTP 自发布归属尚为 Open Discussion）；services/proxy 与
// pods/proxy 需要 create，因为 REST 代理提交作业与伴生 Proxy 调用都是 POST 语义。
// RBACRequiredRules follows the real K8s RBAC model with Group/Resource/Subresource/Verb split;
// services drops patch (session-mode least privilege, master HTTP self-publish ownership still an
// open discussion); services/proxy and pods/proxy need create because proxied job submission and
// companion proxy calls are POST semantics.
var RBACRequiredRules = []struct {
	Group       string
	Resource    string
	Subresource string
	Verb        string
}{
	{"", "pods", "", "get"},
	{"", "pods", "", "list"},
	{"", "pods", "", "watch"},
	{"", "pods", "", "create"},
	{"", "pods", "", "delete"},
	{"", "pods", "proxy", "get"},
	{"", "pods", "proxy", "create"},
	{"", "pods", "log", "get"},
	{"apps", "statefulsets", "", "get"},
	{"apps", "statefulsets", "", "list"},
	{"apps", "statefulsets", "", "create"},
	{"apps", "statefulsets", "", "update"},
	{"apps", "statefulsets", "", "patch"},
	{"apps", "statefulsets", "", "delete"},
	{"apps", "deployments", "", "get"},
	{"apps", "deployments", "", "list"},
	{"apps", "deployments", "", "create"},
	{"apps", "deployments", "", "update"},
	{"apps", "deployments", "", "patch"},
	{"apps", "deployments", "", "delete"},
	{"", "services", "", "get"},
	{"", "services", "", "list"},
	{"", "services", "", "create"},
	{"", "services", "", "update"},
	{"", "services", "", "delete"},
	{"", "services", "proxy", "get"},
	{"", "services", "proxy", "create"},
	{"", "configmaps", "", "get"},
	{"", "configmaps", "", "list"},
	{"", "configmaps", "", "create"},
	{"", "configmaps", "", "update"},
	{"", "configmaps", "", "patch"},
	{"", "configmaps", "", "delete"},
	{"", "secrets", "", "get"},
	{"", "secrets", "", "list"},
	{"", "secrets", "", "create"},
	{"batch", "jobs", "", "get"},
	{"batch", "jobs", "", "list"},
	{"batch", "jobs", "", "create"},
	{"batch", "jobs", "", "delete"},
}

// PrecheckRBAC 对目标 Namespace 执行只读 SelfSubjectAccessReview 预检，不写入任何 K8s 对象。
// PrecheckRBAC runs read-only SelfSubjectAccessReview checks against the target namespace
// without writing any K8s object.
func (m *ClientManager) PrecheckRBAC(ctx context.Context, hostID uint, namespace string) ([]RBACCheckItem, error) {
	clientset, err := m.Clientset(ctx, hostID)
	if err != nil {
		return nil, err
	}
	if namespace == "" {
		namespace = "default"
	}
	items := make([]RBACCheckItem, 0, len(RBACRequiredRules))
	for _, rule := range RBACRequiredRules {
		review := &authv1.SelfSubjectAccessReview{
			Spec: authv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authv1.ResourceAttributes{
					Namespace:   namespace,
					Group:       rule.Group,
					Resource:    rule.Resource,
					Subresource: rule.Subresource,
					Verb:        rule.Verb,
				},
			},
		}
		resp, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		item := RBACCheckItem{Resource: rule.Resource, Verb: rule.Verb}
		if rule.Subresource != "" {
			item.Resource = rule.Resource + "/" + rule.Subresource
		}
		if err != nil {
			item.Allowed = false
			item.Reason = fmt.Sprintf("review failed: %v", err)
		} else {
			item.Allowed = resp.Status.Allowed
			item.Reason = resp.Status.Reason
		}
		items = append(items, item)
	}
	return items, nil
}

// ServerVersion 读取 API Server 版本，用于连通性测试。
// ServerVersion reads the API server version for connectivity tests.
func (m *ClientManager) ServerVersion(ctx context.Context, hostID uint) (string, error) {
	clientset, err := m.Clientset(ctx, hostID)
	if err != nil {
		return "", err
	}
	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("k8s: read server version failed: %w", err)
	}
	return info.GitVersion, nil
}

// ListNamespaces 列举集群 Namespace 名称，用于主机发现向导。
// ListNamespaces lists namespace names for the host discovery wizard.
func (m *ClientManager) ListNamespaces(ctx context.Context, hostID uint) ([]string, error) {
	clientset, err := m.Clientset(ctx, hostID)
	if err != nil {
		return nil, err
	}
	list, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: list namespaces failed: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}
