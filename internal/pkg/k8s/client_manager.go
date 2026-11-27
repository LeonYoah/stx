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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
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
)

// Credentials 是构建 K8s 客户端所需的明文凭据（已解密）。
// Credentials holds decrypted plaintext credential material for building K8s clients.
type Credentials struct {
	AuthMode        AuthMode
	APIURL          string
	Namespace       string
	Kubeconfig      string
	Token           string
	CACert          string
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

// ParseKubeconfigServerURL 解析 kubeconfig 内容并提取内嵌的 API Server 地址。
// ParseKubeconfigServerURL validates kubeconfig YAML and extracts its embedded API server URL.
func ParseKubeconfigServerURL(kubeconfig string) (string, error) {
	if strings.TrimSpace(kubeconfig) == "" {
		return "", fmt.Errorf("k8s: kubeconfig content is required")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return "", fmt.Errorf("k8s: invalid kubeconfig: %w", err)
	}
	return strings.TrimRight(strings.TrimSpace(cfg.Host), "/"), nil
}

// BuildRESTConfig 从凭据构建 rest.Config；kubeconfig 模式直接解析内嵌连接与 TLS 配置，
// token 模式使用显式 API URL + Bearer Token（可选 CA 或跳过 TLS 校验）。
// BuildRESTConfig builds a rest.Config from credentials; kubeconfig mode parses embedded
// server and TLS config directly, while token mode uses explicit API URL + bearer token.
func BuildRESTConfig(cred *Credentials) (*rest.Config, error) {
	if cred == nil {
		return nil, fmt.Errorf("k8s: credentials are required")
	}
	mode := cred.AuthMode
	if mode == "" {
		mode = AuthModeKubeconfig
	}
	switch mode {
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
		if !cred.InsecureSkipTLS && strings.TrimSpace(cred.CACert) != "" {
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
	// 客户端单请求 15s 超时 + 5s TCP 建连超时 + 放宽 QPS/Burst，避免 54 条 SSAR 预检被 client-go 默认 5 QPS 限速拖到 9 秒。
	// Use a 15s request timeout, 5s TCP dial timeout, and higher QPS/Burst so 54 SSAR checks don't stall on client-go's default 5 QPS throttle.
	restConfig.Timeout = 15 * time.Second
	if restConfig.QPS <= 0 || restConfig.QPS < 50 {
		restConfig.QPS = 50
	}
	if restConfig.Burst <= 0 || restConfig.Burst < 100 {
		restConfig.Burst = 100
	}
	if restConfig.Dial == nil {
		restConfig.Dial = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
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

const (
	// RBACScopeRequired 单命名空间最小权限运行必需的核心权限。
	// RBACScopeRequired marks core permissions strictly required for single-namespace runtime operations.
	RBACScopeRequired = "required"
	// RBACScopeExtended 跨命名空间发现或自动创建每集群专属 RBAC 所需的扩展权限。
	// RBACScopeExtended marks extended permissions needed for multi-namespace listing or auto per-cluster RBAC.
	RBACScopeExtended = "extended"
)

// PermissionMode 根据 RBAC 预检结果推断出的 K8s 主机授权模式。
// PermissionMode represents the authorization tier inferred from RBAC precheck results.
type PermissionMode string

const (
	// PermissionModeFull 全面授权：核心权限齐备，且可列举命名空间与自动创建专属 RBAC。
	// PermissionModeFull grants all core rules plus cluster-wide namespace listing and per-cluster RBAC provisioning.
	PermissionModeFull PermissionMode = "full"
	// PermissionModeMinimal 最小授权：核心单命名空间运行时权限齐备，但受限于单命名空间或禁止自动建 RBAC。
	// PermissionModeMinimal grants all core single-namespace rules while extended cluster/RBAC permissions are absent.
	PermissionModeMinimal PermissionMode = "minimal"
	// PermissionModeInsufficient 权限不足：存在核心运行时权限缺失。
	// PermissionModeInsufficient indicates one or more core runtime rules are denied.
	PermissionModeInsufficient PermissionMode = "insufficient"
)

// RBACCheckItem 单项 RBAC 预检结果。
// RBACCheckItem is one RBAC precheck outcome.
type RBACCheckItem struct {
	Resource string `json:"resource"`
	Verb     string `json:"verb"`
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// RBACRequiredRules 测试连接必须覆盖的资源与动作；与伴生 Proxy、常驻集群运维、日志读取的
// 按真实 K8s RBAC 模型拆填 Group/Resource/Subresource/Verb；services 不含 patch
// （Session 模式最小权限，Master HTTP 自发布归属尚为 Open Discussion）；services/proxy 与
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
	{"apps", "replicasets", "", "list"},
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
	{"", "secrets", "", "update"},
	{"", "secrets", "", "delete"},
	{"", "serviceaccounts", "", "get"},
	{"", "serviceaccounts", "", "list"},
	{"", "persistentvolumeclaims", "", "get"},
	{"", "persistentvolumeclaims", "", "list"},
	{"", "events", "", "list"},
	{"batch", "jobs", "", "get"},
	{"batch", "jobs", "", "list"},
	{"batch", "jobs", "", "create"},
	{"batch", "jobs", "", "delete"},
}

// RBACExtendedRules 用于识别全面授权能力（跨命名空间枚举与自动建 SA/Role/RoleBinding）。
// 缺少这些权限不阻塞单命名空间最小权限部署，系统会自动回退到单命名空间预建身份模式。
// RBACExtendedRules probes elevated capabilities (cluster-scoped namespace listing and
// automatic per-cluster RBAC creation). Missing these does not block single-namespace minimal mode.
var RBACExtendedRules = []struct {
	Group         string
	Resource      string
	Subresource   string
	Verb          string
	ClusterScoped bool
}{
	{"", "namespaces", "", "list", true},
	{"", "namespaces", "", "create", true},
	{"", "serviceaccounts", "", "create", false},
	{"rbac.authorization.k8s.io", "roles", "", "create", false},
	{"rbac.authorization.k8s.io", "rolebindings", "", "create", false},
	{"", "endpoints", "", "list", false},
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

	type rbacRuleSpec struct {
		ns          string
		group       string
		resource    string
		subresource string
		verb        string
		scope       string
	}
	specs := make([]rbacRuleSpec, 0, len(RBACRequiredRules)+len(RBACExtendedRules))
	for _, rule := range RBACRequiredRules {
		specs = append(specs, rbacRuleSpec{
			ns:          namespace,
			group:       rule.Group,
			resource:    rule.Resource,
			subresource: rule.Subresource,
			verb:        rule.Verb,
			scope:       RBACScopeRequired,
		})
	}
	for _, rule := range RBACExtendedRules {
		targetNS := namespace
		if rule.ClusterScoped {
			targetNS = ""
		}
		specs = append(specs, rbacRuleSpec{
			ns:          targetNS,
			group:       rule.Group,
			resource:    rule.Resource,
			subresource: rule.Subresource,
			verb:        rule.Verb,
			scope:       RBACScopeExtended,
		})
	}

	// 并发执行 SSAR 预检并保持确定性顺序；遇非 403 网络/超时错误立即取消并返回错误，避免误判为「权限不足」。
	// Run SSAR checks concurrently with deterministic ordering; abort on transport/timeout errors so they never masquerade as insufficient RBAC.
	items := make([]RBACCheckItem, len(specs))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	const maxWorkers = 12
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for i, spec := range specs {
		wg.Add(1)
		go func(idx int, s rbacRuleSpec) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				return
			}
			review := &authv1.SelfSubjectAccessReview{
				Spec: authv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &authv1.ResourceAttributes{
						Namespace:   s.ns,
						Group:       s.group,
						Resource:    s.resource,
						Subresource: s.subresource,
						Verb:        s.verb,
					},
				},
			}
			resp, reqErr := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(runCtx, review, metav1.CreateOptions{})
			item := RBACCheckItem{Resource: s.resource, Verb: s.verb, Scope: s.scope}
			if s.subresource != "" {
				item.Resource = s.resource + "/" + s.subresource
			}
			if reqErr != nil {
				if apierrors.IsForbidden(reqErr) {
					item.Allowed = false
					item.Reason = reqErr.Error()
					items[idx] = item
					return
				}
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("k8s: rbac precheck failed: %w", reqErr)
					cancel()
				}
				errMu.Unlock()
				return
			}
			item.Allowed = resp.Status.Allowed
			item.Reason = resp.Status.Reason
			items[idx] = item
		}(i, spec)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("k8s: rbac precheck cancelled: %w", ctx.Err())
	}
	return items, nil
}

// AllRequiredRBACAllowed 判断所有核心（required）RBAC 检查是否全部放行。
// AllRequiredRBACAllowed reports whether every core (required) RBAC check is allowed.
func AllRequiredRBACAllowed(items []RBACCheckItem) bool {
	if len(items) == 0 {
		return false
	}
	hasRequired := false
	for _, item := range items {
		if item.Scope == RBACScopeExtended {
			continue
		}
		hasRequired = true
		if !item.Allowed {
			return false
		}
	}
	return hasRequired
}

func rbacRuleAllowed(items []RBACCheckItem, resource, verb string) bool {
	for _, item := range items {
		if item.Resource == resource && item.Verb == verb {
			return item.Allowed
		}
	}
	return false
}

// CanManageRBAC 判断当前凭据是否具备按集群自动创建隔离 ServiceAccount/Role/RoleBinding 的权限。
// CanManageRBAC reports whether the credential can provision per-cluster SA/Role/RoleBinding
// (including endpoints read access required by K8s privilege-escalation rules).
func CanManageRBAC(items []RBACCheckItem) bool {
	return rbacRuleAllowed(items, "serviceaccounts", "create") &&
		rbacRuleAllowed(items, "roles", "create") &&
		rbacRuleAllowed(items, "rolebindings", "create") &&
		rbacRuleAllowed(items, "endpoints", "list")
}

// CanListNamespaces 判断当前凭据是否具备集群级列举 Namespace 的权限。
// CanListNamespaces reports whether the credential has cluster-scoped namespaces list permission.
func CanListNamespaces(items []RBACCheckItem) bool {
	return rbacRuleAllowed(items, "namespaces", "list")
}

// EvaluatePermissionMode 根据预检结果自动归纳授权模式（full / minimal / insufficient）。
// EvaluatePermissionMode infers the effective permission tier (full / minimal / insufficient).
func EvaluatePermissionMode(items []RBACCheckItem) PermissionMode {
	if !AllRequiredRBACAllowed(items) {
		return PermissionModeInsufficient
	}
	if CanManageRBAC(items) && CanListNamespaces(items) {
		return PermissionModeFull
	}
	return PermissionModeMinimal
}

// ServerVersion 读取 API Server 版本，用于连通性测试；优先走支持 ctx 取消/超时的 /version 请求。
// ServerVersion reads the API server version for connectivity tests, preferring a context-aware /version call.
func (m *ClientManager) ServerVersion(ctx context.Context, hostID uint) (string, error) {
	clientset, err := m.Clientset(ctx, hostID)
	if err != nil {
		return "", err
	}
	if rc := clientset.Discovery().RESTClient(); rc != nil {
		body, err := rc.Get().AbsPath("/version").DoRaw(ctx)
		if err != nil {
			return "", fmt.Errorf("k8s: read server version failed: %w", err)
		}
		var info version.Info
		if err := json.Unmarshal(body, &info); err != nil {
			return "", fmt.Errorf("k8s: parse server version failed: %w", err)
		}
		return info.GitVersion, nil
	}
	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("k8s: read server version failed: %w", err)
	}
	return info.GitVersion, nil
}

func parseServiceAccountUsername(username string) (namespace, name string) {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return "", ""
	}
	rest := strings.TrimPrefix(username, prefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// ParseBearerTokenServiceAccount 从 K8s ServiceAccount JWT payload 的 sub 字段提取命名空间与 SA 名称（仅做只读身份识别，不替代 API Server 验签）。
// ParseBearerTokenServiceAccount extracts namespace and SA name from a K8s ServiceAccount JWT sub claim (read-only hint; API server still verifies signatures).
func ParseBearerTokenServiceAccount(token string) (namespace, name string) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[1] == "" {
		return "", ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return "", ""
	}
	return parseServiceAccountUsername(claims.Sub)
}

// WhoAmIServiceAccount 尽最大努力识别当前控制面凭据对应的 ServiceAccount 名称，供前端动态排除自身高权限身份。
// WhoAmIServiceAccount best-effort identifies the control-plane credential's ServiceAccount name so UI excludes it from runner SA picks.
func (m *ClientManager) WhoAmIServiceAccount(ctx context.Context, hostID uint) string {
	entry, err := m.entry(ctx, hostID)
	if err != nil || entry == nil {
		return ""
	}
	if entry.restConfig != nil && entry.restConfig.BearerToken != "" {
		if _, name := ParseBearerTokenServiceAccount(entry.restConfig.BearerToken); name != "" {
			return name
		}
	}
	if entry.clientset == nil {
		return ""
	}
	subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	resp, err := entry.clientset.AuthenticationV1().SelfSubjectReviews().Create(subCtx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil || resp == nil {
		return ""
	}
	_, name := parseServiceAccountUsername(resp.Status.UserInfo.Username)
	return name
}

// ClassifyK8sError 将底层 client-go / 网络错误归纳为前端可映射的稳定错误码。
// ClassifyK8sError maps client-go and transport errors to stable error codes for UI guidance.
func ClassifyK8sError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if apierrors.IsUnauthorized(err) {
		return "unauthorized"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "client.timeout"):
		return "timeout"
	case strings.Contains(msg, "x509:"),
		strings.Contains(msg, "tls:"),
		strings.Contains(msg, "certificate"):
		return "tls_error"
	case strings.Contains(msg, "provide credentials"),
		strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "401"):
		return "unauthorized"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "dial tcp"),
		strings.Contains(msg, "network is unreachable"):
		return "unreachable"
	case strings.Contains(msg, "invalid kubeconfig"),
		strings.Contains(msg, "credentials are required"),
		strings.Contains(msg, "api url is required"),
		strings.Contains(msg, "token is required"):
		return "invalid_config"
	default:
		return "unknown"
	}
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

// ListNamespaceObjects 按 kind 列出 namespace 内已有对象名（secrets/serviceaccounts/pvcs），
// 供创建向导高级选项下拉选择；未知 kind 返回错误。
// ListNamespaceObjects lists existing object names in a namespace by kind
// (secrets/serviceaccounts/pvcs) for the wizard advanced dropdowns; unknown kinds error.
func (m *ClientManager) ListNamespaceObjects(ctx context.Context, hostID uint, namespace, kind string) ([]string, error) {
	clientset, err := m.Clientset(ctx, hostID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	switch kind {
	case "secrets":
		list, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
	case "serviceaccounts":
		list, err := clientset.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
	case "pvcs":
		list, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
	default:
		return nil, fmt.Errorf("k8s: unsupported object kind %q", kind)
	}
	return names, nil
}
