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
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/rest"
)

// Transport 按访问模式为集群内 HTTP 端点构造 BaseURL 与认证 RoundTripper，
// 供 EngineClient / ConfigToolClient / embedts 抓取器 / WebUI 代理统一复用。
// Transport builds base URLs and authenticated RoundTrippers for in-cluster HTTP endpoints
// per access mode, shared by EngineClient / ConfigToolClient / embedts scrapers / WebUI proxy.
type Transport struct {
	manager         *ClientManager
	hostID          uint
	namespace       string
	accessMode      AccessMode
	externalBaseURL string
}

// NewTransport 创建集群级传输器。
// NewTransport creates a cluster-scoped transport.
func NewTransport(manager *ClientManager, hostID uint, namespace string, accessMode AccessMode, externalBaseURL string) *Transport {
	if accessMode == "" {
		accessMode = AccessModeAPIServerProxy
	}
	if namespace == "" {
		namespace = "default"
	}
	return &Transport{
		manager:         manager,
		hostID:          hostID,
		namespace:       namespace,
		accessMode:      accessMode,
		externalBaseURL: strings.TrimRight(strings.TrimSpace(externalBaseURL), "/"),
	}
}

// AccessMode 返回当前传输模式。
// AccessMode returns the current access mode.
func (t *Transport) AccessMode() AccessMode {
	return t.accessMode
}

// ServiceBaseURL 返回访问指定 Service 端口的 BaseURL（不含尾部斜杠）。
// ServiceBaseURL returns the base URL for reaching a service port (no trailing slash).
func (t *Transport) ServiceBaseURL(ctx context.Context, serviceName string, port int) (string, error) {
	switch t.accessMode {
	case AccessModeAPIServerProxy:
		cfg, err := t.manager.RESTConfig(ctx, t.hostID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/api/v1/namespaces/%s/services/http:%s:%d/proxy",
			strings.TrimRight(cfg.Host, "/"), url.PathEscape(t.namespace), url.PathEscape(serviceName), port), nil
	case AccessModeInCluster:
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, t.namespace, port), nil
	case AccessModeExternal:
		if t.externalBaseURL == "" {
			return "", fmt.Errorf("k8s: external_base_url is required for external access mode")
		}
		return t.externalBaseURL, nil
	default:
		return "", fmt.Errorf("k8s: unsupported access mode %q", t.accessMode)
	}
}

// PodBaseURL 返回访问指定 Pod 端口的 BaseURL（不含尾部斜杠）。
// PodBaseURL returns the base URL for reaching a pod port (no trailing slash).
func (t *Transport) PodBaseURL(ctx context.Context, podName, podIP string, port int) (string, error) {
	switch t.accessMode {
	case AccessModeAPIServerProxy:
		cfg, err := t.manager.RESTConfig(ctx, t.hostID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/api/v1/namespaces/%s/pods/http:%s:%d/proxy",
			strings.TrimRight(cfg.Host, "/"), url.PathEscape(t.namespace), url.PathEscape(podName), port), nil
	case AccessModeInCluster:
		if podIP == "" {
			return "", fmt.Errorf("k8s: pod ip is required for in_cluster pod access")
		}
		return fmt.Sprintf("http://%s:%d", podIP, port), nil
	case AccessModeExternal:
		if t.externalBaseURL == "" {
			return "", fmt.Errorf("k8s: external_base_url is required for external access mode")
		}
		return t.externalBaseURL, nil
	default:
		return "", fmt.Errorf("k8s: unsupported access mode %q", t.accessMode)
	}
}

// RoundTripper 返回带认证与 TLS 的 RoundTripper：apiserver_proxy 经 rest.TransportFor
// 自动附带 kubeconfig/token 认证；in_cluster 与 external 走默认直连传输。
// RoundTripper returns an authenticated RoundTripper: apiserver_proxy uses rest.TransportFor
// to attach kubeconfig/token auth and TLS; in_cluster and external use a plain direct transport.
func (t *Transport) RoundTripper(ctx context.Context) (http.RoundTripper, error) {
	if t.accessMode == AccessModeAPIServerProxy {
		cfg, err := t.manager.RESTConfig(ctx, t.hostID)
		if err != nil {
			return nil, err
		}
		rt, err := rest.TransportFor(cfg)
		if err != nil {
			return nil, fmt.Errorf("k8s: build transport failed: %w", err)
		}
		return rt, nil
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	return base, nil
}

// HTTPClient 构造带正确传输的 http.Client，timeout<=0 时用 15s 默认。
// HTTPClient builds an http.Client with the proper transport; a non-positive timeout falls
// back to 15s.
func (t *Transport) HTTPClient(ctx context.Context, timeout time.Duration) (*http.Client, error) {
	rt, err := t.RoundTripper(ctx)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{Transport: rt, Timeout: timeout}, nil
}

// HostTransportRegistry 按 URL host 注册定制 RoundTripper（apiserver_proxy 主机维度），
// 供只拿到 endpoint 字符串的调用方（如 config tool client）透明继承 TLS/认证。
// HostTransportRegistry registers custom RoundTrippers by URL host (apiserver_proxy hosts) so
// callers that only receive an endpoint string (e.g. the config tool client) transparently
// inherit TLS/auth material.
type HostTransportRegistry struct {
	mu sync.RWMutex
	byHost map[string]http.RoundTripper
}

// NewHostTransportRegistry 创建空注册表。
// NewHostTransportRegistry creates an empty registry.
func NewHostTransportRegistry() *HostTransportRegistry {
	return &HostTransportRegistry{byHost: map[string]http.RoundTripper{}}
}

// Register 登记 host（含端口）到 RoundTripper 的映射。
// Register records the host(including port) → RoundTripper mapping.
func (r *HostTransportRegistry) Register(rawURL string, rt http.RoundTripper) {
	if r == nil || rt == nil {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byHost[u.Host] = rt
}

// Lookup 返回 host 对应 RoundTripper；未注册返回 nil。
// Lookup returns the RoundTripper for a host; nil when unregistered.
func (r *HostTransportRegistry) Lookup(host string) http.RoundTripper {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byHost[host]
}

// DefaultHostTransportRegistry 是进程级共享注册表，由 router 装配期写入。
// DefaultHostTransportRegistry is the process-wide shared registry populated during router wiring.
var DefaultHostTransportRegistry = NewHostTransportRegistry()

// DelegatingRoundTripper 按请求 host 委托已注册 Transport，未命中回退 base。
// DelegatingRoundTripper delegates to registered transports by request host, falling back to base.
type DelegatingRoundTripper struct {
	Base     http.RoundTripper
	Registry *HostTransportRegistry
}

// RoundTrip implements http.RoundTripper.
func (d *DelegatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	registry := d.Registry
	if registry == nil {
		registry = DefaultHostTransportRegistry
	}
	if rt := registry.Lookup(req.URL.Host); rt != nil {
		return rt.RoundTrip(req)
	}
	base := d.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
