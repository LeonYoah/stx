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
	"strings"
	"time"
)

// Transport 按访问模式（集群内直连 / 外部入口）为集群内 HTTP 端点构造 BaseURL 与直连 RoundTripper，
// 供 EngineClient / ConfigToolClient / embedts 抓取器 / WebUI 代理统一复用。
// Transport builds base URLs and direct RoundTrippers for Kubernetes HTTP endpoints
// (in_cluster or external), shared by EngineClient / ConfigToolClient / embedts / WebUI proxy.
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
		accessMode = AccessModeInCluster
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
func (t *Transport) ServiceBaseURL(_ context.Context, serviceName string, port int) (string, error) {
	switch t.accessMode {
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
func (t *Transport) PodBaseURL(_ context.Context, _ string, podIP string, port int) (string, error) {
	switch t.accessMode {
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

// RoundTripper 返回直连 HTTP Transport（带 5s TCP 建连超时与 keep-alive）。
// RoundTripper returns a direct HTTP transport configured with a 5s dial timeout and keep-alive.
func (t *Transport) RoundTripper(_ context.Context) (http.RoundTripper, error) {
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
