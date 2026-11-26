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

package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/seatunnel"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// executeK8sSTXJavaProxyCommand 是六个标准 Proxy 接口的 K8s 适配层：
// 把 K8s 伴生 Proxy 的 Deployment 语义映射成与物理机一致的 *STXJavaProxyStatus，
// 前端无需感知基础设施差异。
// executeK8sSTXJavaProxyCommand adapts the six standard proxy APIs for K8s: the companion
// proxy Deployment semantics are mapped onto the same *STXJavaProxyStatus contract used by
// bare-metal clusters so the frontend never needs to care about infrastructure differences.
func (s *Service) executeK8sSTXJavaProxyCommand(ctx context.Context, clusterInfo *Cluster, commandType string) (*STXJavaProxyStatus, error) {
	cfg := clusterInfo.K8sSessionConfig()
	if cfg == nil {
		return nil, fmt.Errorf("cluster: k8s session config missing")
	}
	mode := cfg.JavaProxy.Mode
	if mode == "" {
		mode = k8s.JavaProxyModeManaged
	}
	code := clusterInfo.EffectiveClusterCode()
	proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)

	switch commandType {
	case "status":
		return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), nil
	case "start":
		if mode == k8s.JavaProxyModeDisabled {
			return nil, fmt.Errorf("cluster: java proxy is disabled for this cluster")
		}
		ensure := s.EnsureK8sJavaProxy(ctx, clusterInfo)
		status := s.k8sSTXJavaProxyStatus(ctx, clusterInfo)
		if !ensure.Healthy && mode == k8s.JavaProxyModeManaged {
			status.Message = strings.TrimSpace(ensure.Message + " " + strings.Join(ensure.PodEvents, " · "))
			return status, fmt.Errorf("%s", firstNonEmpty(ensure.Message, "k8s java proxy failed to become healthy"))
		}
		return status, nil
	case "stop":
		if mode != k8s.JavaProxyModeManaged {
			return nil, fmt.Errorf("cluster: stop is only supported for managed java proxy mode")
		}
		if err := proxyMgr.StopProxy(ctx, cfg, code); err != nil {
			return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), err
		}
		return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), nil
	case "restart":
		if mode == k8s.JavaProxyModeManaged {
			epoch := seatunnel.ProxyEpochForVersion(clusterInfo.Version)
			if err := proxyMgr.RestartProxy(ctx, cfg, code, clusterInfo.ID, epoch); err != nil {
				return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), err
			}
			return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), nil
		}
		if mode == k8s.JavaProxyModeDisabled {
			return nil, fmt.Errorf("cluster: java proxy is disabled for this cluster")
		}
		// reuse/external 的 Proxy 生命周期不归本集群管，重探活并返回最新状态。
		// reuse/external proxy lifecycles are not owned by this cluster; re-probe and
		// return the latest status.
		return s.k8sSTXJavaProxyStatus(ctx, clusterInfo), nil
	}
	return nil, fmt.Errorf("cluster: unsupported stx java proxy command %q", commandType)
}

// k8sSTXJavaProxyStatus 把 K8s 伴生 Proxy 的实时状态映射为标准契约。
// 注意 Role 必须留空：前端会把非空 role 拼进 cluster.roles.<role> 翻译键，填值会渲染出"(未定义)"。
// k8sSTXJavaProxyStatus maps the live state of the companion proxy onto the standard
// contract. Role must stay empty: the frontend interpolates a non-empty role into the
// cluster.roles.<role> translation key, which would render as "(undefined)".
func (s *Service) k8sSTXJavaProxyStatus(ctx context.Context, clusterInfo *Cluster) *STXJavaProxyStatus {
	cfg := clusterInfo.K8sSessionConfig()
	status := &STXJavaProxyStatus{
		ClusterID:   clusterInfo.ID,
		ClusterName: clusterInfo.Name,
		Version:     clusterInfo.Version,
		Service:     "stx_java_proxy",
		Role:        "",
	}
	if cfg == nil {
		status.Message = "k8s session config missing"
		return status
	}
	mode := cfg.JavaProxy.Mode
	if mode == "" {
		mode = k8s.JavaProxyModeManaged
	}
	port := cfg.JavaProxy.Port
	if port <= 0 {
		port = 18080
	}
	status.Port = port
	status.JvmOpts = cfg.JavaProxy.JVMOpts
	status.Managed = mode == k8s.JavaProxyModeManaged

	code := clusterInfo.EffectiveClusterCode()
	deploymentName, serviceName, _ := k8s.ProxyResourceNames(code)
	if cfg.JavaProxy.DeploymentName != "" {
		deploymentName = cfg.JavaProxy.DeploymentName
	}
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}

	switch mode {
	case k8s.JavaProxyModeDisabled:
		status.Message = "java proxy disabled"
		return status
	case k8s.JavaProxyModeExternal:
		status.HostName = "external"
		status.Endpoint = strings.TrimRight(cfg.JavaProxy.ExternalURL, "/")
		if status.Endpoint != "" && probeK8sProxyHealthzDirect(status.Endpoint) {
			status.Running = true
			status.Healthy = true
		} else {
			status.Message = "external java proxy health probe failed"
		}
		return status
	}

	targetNs := cfg.Namespace
	targetService := serviceName
	if mode == k8s.JavaProxyModeReuseCluster && cfg.JavaProxy.ReuseClusterID > 0 {
		if reuse, err := s.repo.GetByID(ctx, cfg.JavaProxy.ReuseClusterID, false); err == nil && reuse != nil {
			if reuseCfg := reuse.K8sSessionConfig(); reuseCfg != nil {
				targetNs = reuseCfg.Namespace
				_, reuseSvc, _ := k8s.ProxyResourceNames(reuse.EffectiveClusterCode())
				if reuseCfg.JavaProxy.ServiceName != "" {
					reuseSvc = reuseCfg.JavaProxy.ServiceName
				}
				targetService = reuseSvc
			}
		}
		status.HostName = fmt.Sprintf("%s/%s", targetNs, targetService)
	} else {
		status.HostName = fmt.Sprintf("%s/%s", cfg.Namespace, deploymentName)
	}
	status.Endpoint = fmt.Sprintf("http://%s.%s.svc:%d", targetService, targetNs, port)

	if mode == k8s.JavaProxyModeManaged {
		proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)
		if ps, err := proxyMgr.GetProxyStatus(ctx, cfg, code); err == nil && ps != nil {
			// 读取 Deployment 判定期望副本：spec.replicas == 0 即为 stopped。
			// Read the deployment for desired replicas: spec.replicas == 0 means stopped.
			desired := int32(1)
			if clientset, cerr := s.k8sManager.Clientset(ctx, cfg.HostID); cerr == nil {
				if dep, derr := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, deploymentName, metav1.GetOptions{}); derr == nil && dep.Spec.Replicas != nil {
					desired = *dep.Spec.Replicas
				}
			}
			status.Running = ps.Exists && desired > 0
			if ps.Exists {
				status.LogPath = firstActiveProxyPodName(ctx, s, cfg, code)
			}
			if !status.Running {
				if len(ps.PodEvents) > 0 {
					status.Message = strings.Join(ps.PodEvents, " · ")
				}
				return status
			}
		}
	}

	// 探活决定 Healthy：managed/reuse 走 K8sTransport 经 API 代理访问 Service。
	// Health probing decides Healthy: managed/reuse go through K8sTransport via the
	// apiserver proxy to reach the service.
	if status.Running || mode == k8s.JavaProxyModeReuseCluster {
		transport := k8s.NewTransport(s.k8sManager, cfg.HostID, targetNs, cfg.AccessMode, cfg.ExternalBaseURL)
		if base, err := transport.ServiceBaseURL(ctx, targetService, port); err == nil {
			if client, err := transport.HTTPClient(ctx, 6*time.Second); err == nil {
				if resp, err := client.Get(strings.TrimRight(base, "/") + "/healthz"); err == nil {
					resp.Body.Close()
					status.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 300
					status.Running = true
				}
			}
		}
		if !status.Healthy && status.Message == "" {
			status.Message = "java proxy health probe failed"
		}
	}
	return status
}

// probeK8sProxyHealthzDirect 直连探活外部 Proxy。
// probeK8sProxyHealthzDirect probes an external proxy endpoint directly.
func probeK8sProxyHealthzDirect(baseURL string) bool {
	probe := &K8sProxyStatus{PodEvents: []string{}}
	probeExternalProxy(probe, baseURL)
	return probe.Healthy
}

// firstActiveProxyPodName 返回第一个活跃 Proxy Pod 名（用于日志卡片副标题），失败返回空串。
// firstActiveProxyPodName returns the first active proxy pod name (for the log card
// subtitle), or empty on failure.
func firstActiveProxyPodName(ctx context.Context, s *Service, cfg *k8s.SessionConfig, code string) string {
	proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)
	podName, _, err := proxyMgr.GetProxyLogs(ctx, cfg, code, 1)
	if err != nil {
		return ""
	}
	return podName
}
