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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/pkg/infra"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

// ErrK8sHostInvalid 表示创建 K8s 集群时引用的主机不存在或类型不符。
// ErrK8sHostInvalid means the referenced host is missing or not a kubernetes host.
var ErrK8sHostInvalid = errors.New("cluster: k8s host invalid or not kubernetes type")

// ErrEventCallbackLoopback 事件通道回调地址为回环/未指定地址，引擎 Pod 内无法到达控制面。
// ErrEventCallbackLoopback rejects loopback/unspecified event callback addresses that
// engine pods can never reach the control plane through.
var ErrEventCallbackLoopback = errors.New("cluster: event channel callback URL host is loopback; inside engine pods it resolves to the container itself, not the STX control plane")

// SetK8sDependencies 注入 K8s 依赖；jarLoader 读取伴生 Proxy jar 字节，生产实现从 lib/ 加载。
// SetK8sDependencies injects K8s dependencies; jarLoader loads companion proxy jar bytes
// (production implementation reads from lib/).
func (s *Service) SetK8sDependencies(mgr *k8s.ClientManager, jarLoader k8s.ProxyJarLoader, hostTypeChecker K8sHostTypeChecker) {
	s.k8sManager = mgr
	s.k8sJarLoader = jarLoader
	s.k8sHostTypeChecker = hostTypeChecker
}

// K8sHostTypeChecker 解耦 host 域：只回答「该主机是否为 kubernetes 类型」。
// K8sHostTypeChecker decouples the host domain: it only answers whether a host is kubernetes-typed.
type K8sHostTypeChecker func(ctx context.Context, hostID uint) (isK8s bool, err error)

// K8sClusterCreateRequest 是 K8s 集群创建请求（纳管或一键部署共用）。
// K8sClusterCreateRequest is the K8s cluster creation request shared by adopt and deploy flows.
type K8sClusterCreateRequest struct {
	Name        string `json:"name" binding:"required"`
	ClusterCode string `json:"cluster_code"`
	Description string `json:"description"`
	Version     string `json:"version"`
	HostID      uint   `json:"host_id" binding:"required"`
	// management_source: adopted / stx_deployed
	ManagementSource   string `json:"management_source" binding:"required"`
	Namespace          string `json:"namespace" binding:"required"`
	AccessMode         string `json:"access_mode"`
	ExternalBaseURL    string `json:"external_base_url"`
	Image              string `json:"image"`
	ImagePullPolicy    string `json:"image_pull_policy"`
	ServiceAccount     string `json:"service_account"`
	MasterWorkloadKind string `json:"master_workload_kind"`
	MasterWorkloadName string `json:"master_workload_name"`
	MasterReplicas     int    `json:"master_replicas"`
	WorkerWorkloadKind string `json:"worker_workload_kind"`
	WorkerWorkloadName string `json:"worker_workload_name"`
	WorkerReplicas     int    `json:"worker_replicas"`
	MasterServiceName  string `json:"master_service_name"`
	ClusterServiceName string `json:"cluster_service_name"`
	MasterHTTPPort     int    `json:"master_http_port"`
	HazelcastPort      int    `json:"hazelcast_port"`
	MetricsPort        int    `json:"metrics_port"`
	MetricsPath        string `json:"metrics_path"`
	ConfigMapName      string `json:"configmap_name"`
	LabelSelector      string `json:"label_selector"`
	// JavaProxy 伴生代理配置；缺省独立部署模式。
	// JavaProxy holds companion proxy settings; standalone deployment is the default.
	JavaProxyMode        string `json:"java_proxy_mode"`
	JavaProxyJVMOpts     string `json:"java_proxy_jvm_opts"`
	JavaProxyExternalURL string `json:"java_proxy_external_url"`
	JavaProxyPort        int    `json:"java_proxy_port"`
	MasterCPU            string `json:"master_cpu"`
	MasterMemory         string `json:"master_memory"`
	WorkerCPU            string `json:"worker_cpu"`
	WorkerMemory         string `json:"worker_memory"`
	CheckpointPVC        string `json:"checkpoint_pvc"`
	DataStorageClass     string `json:"data_storage_class"`
	DataSize             string `json:"data_size"`
	// 统一创建向导扩展字段：均为可选，空值保持既有渲染行为。
	// Unified wizard extension fields: all optional, empty keeps existing rendering.
	ImagePullSecrets string `json:"image_pull_secrets"`
	DeploymentMode   string `json:"deployment_mode"`
	HybridReplicas   int    `json:"hybrid_replicas"`
	HybridCPU        string `json:"hybrid_cpu"`
	HybridMemory     string `json:"hybrid_memory"`
	// JVM/Runtime/Checkpoint/IMAP 草稿直接复用 k8s 包类型，前后端同构。
	// JVM/Runtime/Checkpoint/IMAP drafts reuse the k8s package types so frontend and
	// backend share one shape.
	EnableEventChannel bool                      `json:"enable_event_channel"`
	STXCallbackURL     string                    `json:"stx_callback_url"`
	JVM                *k8s.SessionJVMConfig     `json:"jvm,omitempty"`
	Runtime            *k8s.SessionRuntimeConfig `json:"runtime,omitempty"`
	Checkpoint         *k8s.SessionStorageSpec   `json:"checkpoint,omitempty"`
	IMAP               *k8s.SessionStorageSpec   `json:"imap,omitempty"`
}

// sessionConfigFromRequest 将请求映射为 SessionConfig 并应用默认值与端口契约校验。
// sessionConfigFromRequest maps the request into a SessionConfig, applying defaults and
// port-contract validation.
func sessionConfigFromRequest(req *K8sClusterCreateRequest) (*k8s.SessionConfig, error) {
	cfg := &k8s.SessionConfig{
		HostID:             req.HostID,
		Namespace:          req.Namespace,
		ManagementSource:   k8s.ManagementSource(req.ManagementSource),
		AccessMode:         k8s.AccessMode(req.AccessMode),
		ExternalBaseURL:    req.ExternalBaseURL,
		Image:              req.Image,
		ImagePullPolicy:    req.ImagePullPolicy,
		ServiceAccount:     req.ServiceAccount,
		MasterWorkloadKind: req.MasterWorkloadKind,
		MasterWorkloadName: req.MasterWorkloadName,
		MasterReplicas:     req.MasterReplicas,
		WorkerWorkloadKind: req.WorkerWorkloadKind,
		WorkerWorkloadName: req.WorkerWorkloadName,
		WorkerReplicas:     req.WorkerReplicas,
		MasterServiceName:  req.MasterServiceName,
		ClusterServiceName: req.ClusterServiceName,
		MasterHTTPPort:     req.MasterHTTPPort,
		HazelcastPort:      req.HazelcastPort,
		MetricsPort:        req.MetricsPort,
		MetricsPath:        req.MetricsPath,
		ConfigMapName:      req.ConfigMapName,
		LabelSelector:      req.LabelSelector,
	}
	cfg.JavaProxy.Mode = k8s.JavaProxyMode(req.JavaProxyMode)
	cfg.JavaProxy.JVMOpts = req.JavaProxyJVMOpts
	cfg.JavaProxy.ExternalURL = req.JavaProxyExternalURL
	if req.JavaProxyPort > 0 {
		cfg.JavaProxy.Port = req.JavaProxyPort
	}
	cfg.Resources.Master.CPU = req.MasterCPU
	cfg.Resources.Master.Memory = req.MasterMemory
	cfg.Resources.Worker.CPU = req.WorkerCPU
	cfg.Resources.Worker.Memory = req.WorkerMemory
	cfg.Storage.CheckpointPVC = req.CheckpointPVC
	cfg.Storage.DataStorageClass = req.DataStorageClass
	cfg.Storage.DataSize = req.DataSize
	cfg.DeploymentMode = strings.ToLower(strings.TrimSpace(req.DeploymentMode))
	cfg.HybridReplicas = req.HybridReplicas
	if req.HybridCPU != "" || req.HybridMemory != "" {
		cfg.Resources.Hybrid = &k8s.ResourceRequirement{CPU: req.HybridCPU, Memory: req.HybridMemory}
	}
	for _, secret := range strings.Split(req.ImagePullSecrets, ",") {
		if trimmed := strings.TrimSpace(secret); trimmed != "" {
			cfg.ImagePullSecrets = append(cfg.ImagePullSecrets, trimmed)
		}
	}
	cfg.JVM = req.JVM
	cfg.Runtime = req.Runtime
	cfg.Checkpoint = req.Checkpoint
	cfg.IMAP = req.IMAP
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// deploymentModeFromRequest 按请求形态写集群实体，hybrid 误判会导致健康检查与生命周期
// 全部按分离双 workload 统计而永远红。
// deploymentModeFromRequest persists the requested topology onto the cluster entity;
// misclassifying hybrid makes health checks and lifecycle expect two separated workloads
// and stay red forever.
func deploymentModeFromRequest(req *K8sClusterCreateRequest) DeploymentMode {
	if strings.EqualFold(strings.TrimSpace(req.DeploymentMode), "hybrid") {
		return DeploymentModeHybrid
	}
	return DeploymentModeSeparated
}

// k8sConfigJSON 将 SessionConfig 序列化为 Config["kubernetes"] 的 JSON 对象。
// k8sConfigJSON serializes the SessionConfig into the Config["kubernetes"] JSON object.
func k8sConfigJSON(cfg *k8s.SessionConfig) (map[string]interface{}, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// K8sProxyStatus 伴生 Proxy 健康视图，保存接口一并返回给前端展示。
// K8sProxyStatus is the companion proxy health view returned alongside save responses.
type K8sProxyStatus struct {
	Healthy    bool     `json:"healthy"`
	Message    string   `json:"message,omitempty"`
	PodPhase   string   `json:"pod_phase,omitempty"`
	PodEvents  []string `json:"pod_events,omitempty"`
	ServiceURL string   `json:"service_url,omitempty"`
}

// CreateKubernetesResult 创建结果：集群实体 + 伴生 Proxy 状态（失败不阻断保存）。
// CreateKubernetesResult pairs the cluster entity with companion proxy status
// (proxy failure does not block saving).
type CreateKubernetesResult struct {
	Cluster             *Cluster        `json:"cluster"`
	ProxyStatus         *K8sProxyStatus `json:"proxy_status,omitempty"`
	EventChannelWarning string          `json:"event_channel_warning,omitempty"`
}

// CreateKubernetes 创建 K8s 集群：stx_deployed 先落 K8s 资源，adopted 直接引用既有 Workload；
// 两者保存后都自动拉起伴生 Proxy 并经 K8sTransport 探活 /healthz。
// CreateKubernetes creates a K8s cluster: stx_deployed provisions K8s resources first while
// adopted references existing workloads; both auto-start the companion proxy after saving and
// probe /healthz through K8sTransport.
func (s *Service) CreateKubernetes(ctx context.Context, req *K8sClusterCreateRequest) (*CreateKubernetesResult, error) {
	if s.k8sManager == nil {
		return nil, errors.New("cluster: k8s dependencies not configured")
	}
	if s.k8sHostTypeChecker != nil {
		isK8s, err := s.k8sHostTypeChecker(ctx, req.HostID)
		if err != nil {
			return nil, err
		}
		if !isK8s {
			return nil, ErrK8sHostInvalid
		}
	}

	exists, err := s.repo.GetByName(ctx, req.Name)
	if err != nil && !errors.Is(err, ErrClusterNotFound) {
		return nil, err
	}
	if exists != nil {
		return nil, ErrClusterNameDuplicate
	}

	code := req.ClusterCode
	if code == "" {
		code = SanitizeClusterCode(req.Name, 0)
	}

	cluster := &Cluster{
		Name:           req.Name,
		ClusterCode:    code,
		Description:    req.Description,
		Version:        req.Version,
		DeploymentMode: deploymentModeFromRequest(req),
		Status:         ClusterStatusCreated,
		InfraType:      string(infra.TypeK8sSession),
		K8sHostID:      &req.HostID,
		Config:         ClusterConfig{},
	}
	if err := s.repo.Create(ctx, cluster); err != nil {
		return nil, err
	}

	cfg, err := sessionConfigFromRequest(req)
	if err != nil {
		// 配置非法时回滚刚建的记录，避免留下无 kubernetes 配置的死集群。
		// Roll back the freshly created record on invalid config to avoid dead rows without kubernetes config.
		_ = s.repo.Delete(ctx, cluster.ID)
		return nil, err
	}
	// stx_deployed 采用确定性命名回填工作负载/服务/ConfigMap，保证伴生 Proxy 克隆与
	// 后续运维（scale/rollout/logs）不依赖用户显式输入；adopted 必须保留用户输入的既有命名。
	// stx_deployed backfills workload/service/ConfigMap names with deterministic naming so
	// proxy cloning and later operations (scale/rollout/logs) don't depend on explicit input;
	// adopted clusters must keep the user-supplied existing names.
	if cfg.ManagementSource == k8s.ManagementSourceSTXDeployed {
		names := k8s.ResourceNamesFor(cluster.EffectiveClusterCode())
		if cfg.IsHybrid() {
			// hybrid 形态只有混合 StatefulSet：回填 HybridWorkloadName 且保持分离命名为空，
			// 避免运维操作误查不存在的 master/worker StatefulSet。
			// Hybrid topology has only the mixed StatefulSet: backfill HybridWorkloadName and
			// keep the separated names empty so operations never probe nonexistent
			// master/worker StatefulSets.
			if cfg.HybridWorkloadName == "" {
				cfg.HybridWorkloadName = names.MasterSTS + "-hybrid"
			}
		} else {
			if cfg.MasterWorkloadName == "" {
				cfg.MasterWorkloadName = names.MasterSTS
			}
			if cfg.WorkerWorkloadName == "" {
				cfg.WorkerWorkloadName = names.WorkerSTS
			}
		}
		if cfg.MasterServiceName == "" {
			cfg.MasterServiceName = names.MasterSvc
		}
		if cfg.ClusterServiceName == "" {
			cfg.ClusterServiceName = names.ClusterSvc
		}
		if cfg.ConfigMapName == "" {
			cfg.ConfigMapName = names.ConfigMap
		}
		if cfg.LabelSelector == "" {
			cfg.LabelSelector = k8s.LabelClusterCodeKey + "=" + cluster.EffectiveClusterCode()
		}
	}
	if req.EnableEventChannel && strings.TrimSpace(req.STXCallbackURL) != "" {
		// 回环预检：用户显式覆写回环地址必然后报失败，在创建任何资源前 400。
		// Loopback pre-check: an explicit loopback override guarantees report failure,
		// so reject with 400 before creating any resource.
		override := strings.TrimSpace(req.STXCallbackURL)
		parsed, parseErr := urlpkg.Parse(override)
		if parseErr != nil || parsed == nil || IsLoopbackCallbackHost(parsed.Hostname()) {
			_ = s.repo.Delete(ctx, cluster.ID)
			return nil, ErrEventCallbackLoopback
		}
	}
	if cfg.ManagementSource == k8s.ManagementSourceSTXDeployed {
		// 注册镜像携带私有仓库凭据时，先在目标 namespace 代建 dockerconfigjson Secret
		// 并并入 imagePullSecrets：用户零 kubectl 操作即可拉取私有镜像。
		// When the registered image carries private-registry credentials, materialize the
		// dockerconfigjson secret in the target namespace and merge it into imagePullSecrets
		// so private pulls need zero manual kubectl work.
		if err := s.ensureK8sRegistryCredential(ctx, cfg, cluster); err != nil {
			_ = s.repo.Delete(ctx, cluster.ID)
			return nil, err
		}
		deployer := k8s.NewSessionManager(s.k8sManager)
		if _, err := deployer.Deploy(ctx, &k8s.DeployOptions{Config: cfg, ClusterCode: cluster.EffectiveClusterCode(), ClusterID: cluster.ID, CreateRBAC: cfg.ServiceAccount == ""}); err != nil {
			_ = s.repo.Delete(ctx, cluster.ID)
			return nil, fmt.Errorf("deploy kubernetes resources: %w", err)
		}
	}

	k8sJSON, err := k8sConfigJSON(cfg)
	if err != nil {
		return nil, err
	}
	cluster.Config["kubernetes"] = k8sJSON
	// K8s 常驻集群在纳管或部署完成后即进入运行态（健康检查随后按真实 Pod 与 /overview 判定），
	// 不与伴生 Proxy 是否启用或首轮探活结果耦合，避免 disabled 模式或慢启动永久卡在 created。
	// K8s session clusters become running once adopted or deployed (health checks then evaluate
	// real pods and /overview), decoupled from companion proxy mode or first-probe timing so
	// disabled mode or slow boots never stall in created forever.
	cluster.Status = ClusterStatusRunning
	if err := s.repo.Update(ctx, cluster); err != nil {
		return nil, err
	}

	var eventChannelErr error
	if req.EnableEventChannel && s.k8sEventChannelEnabler != nil {
		// 此时 Pod 仍在拉镜像/启动，ConfigMap 写入几乎总是先于引擎进程读取；
		// 失败不阻断集群创建，返回 warning 由前端提示稍后手工开启。
		// Pods are still pulling images/starting at this point, so the ConfigMap write
		// almost always lands before the engine process reads it; a failure does not
		// block cluster creation and is surfaced as a warning so the user can enable
		// the channel manually later.
		eventChannelErr = s.k8sEventChannelEnabler.EnableEventChannelForK8sDeploy(ctx, cluster.ID, req.Version, req.STXCallbackURL)
	}
	proxyStatus := s.EnsureK8sJavaProxy(ctx, cluster)
	result := &CreateKubernetesResult{Cluster: cluster, ProxyStatus: proxyStatus}
	if eventChannelErr != nil {
		result.EventChannelWarning = eventChannelErr.Error()
	}
	return result, nil
}

// EnsureK8sJavaProxy 拉起伴生 Proxy 并探活 /healthz；失败返回状态不报错（不阻断保存）。
// EnsureK8sJavaProxy starts the companion proxy and probes /healthz; failures are reported
// in the returned status without erroring out (never blocking the save).
func (s *Service) EnsureK8sJavaProxy(ctx context.Context, cluster *Cluster) *K8sProxyStatus {
	status := &K8sProxyStatus{PodEvents: []string{}}
	cfg := cluster.K8sSessionConfig()
	if cfg == nil || s.k8sManager == nil || s.k8sJarLoader == nil {
		status.Message = "k8s session config or dependencies missing"
		return status
	}
	mode := cfg.JavaProxy.Mode
	if mode == "" {
		mode = k8s.JavaProxyModeManaged
	}
	if mode == k8s.JavaProxyModeDisabled {
		status.Message = "java proxy disabled"
		return status
	}

	code := cluster.EffectiveClusterCode()
	proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)
	if mode == k8s.JavaProxyModeManaged {
		epoch := seatunnel.ProxyEpochForVersion(cluster.Version)
		if _, err := proxyMgr.EnsureProxy(ctx, cfg, code, cluster.ID, epoch); err != nil {
			status.Message = err.Error()
			return status
		}
	}

	// external 模式直连用户自备 Proxy；reuse/managed 都走 Namespace 内 Service 寻址。
	// External mode probes the user-provided proxy directly; reuse/managed resolve the
	// in-namespace service.
	if mode == k8s.JavaProxyModeExternal {
		if cfg.JavaProxy.ExternalURL == "" {
			status.Message = "java proxy external_url is empty"
			return status
		}
		status.ServiceURL = strings.TrimRight(cfg.JavaProxy.ExternalURL, "/")
		probeExternalProxy(status, status.ServiceURL)
		return status
	}
	targetCfg := cfg
	_, serviceName, _ := k8s.ProxyResourceNames(code)
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	port := cfg.JavaProxy.Port
	if mode == k8s.JavaProxyModeReuseCluster && cfg.JavaProxy.ReuseClusterID > 0 {
		if reuse, err := s.repo.GetByID(ctx, cfg.JavaProxy.ReuseClusterID, false); err == nil && reuse != nil {
			_, reuseSvc, _ := k8s.ProxyResourceNames(reuse.EffectiveClusterCode())
			serviceName = reuseSvc
			if reuseCfg := reuse.K8sSessionConfig(); reuseCfg != nil {
				targetCfg = reuseCfg
				if reuseCfg.JavaProxy.ServiceName != "" {
					serviceName = reuseCfg.JavaProxy.ServiceName
				}
				if reuseCfg.JavaProxy.Port > 0 {
					port = reuseCfg.JavaProxy.Port
				}
			}
		}
	}
	if port <= 0 {
		port = 18080
	}
	transport := k8s.NewTransport(s.k8sManager, targetCfg.HostID, targetCfg.Namespace, targetCfg.AccessMode, targetCfg.ExternalBaseURL)
	baseURL, err := transport.ServiceBaseURL(ctx, serviceName, port)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	status.ServiceURL = baseURL

	client, err := transport.HTTPClient(ctx, 5*time.Second)
	if err != nil {
		status.Message = err.Error()
		return status
	}

	// 探活采用短等待（就绪即返回），超时不阻断保存：前端展示 proxy_status 与 PodEvents 并可一键重试。
	// The probe uses a short wait (returning as soon as ready); timeout never blocks saving —
	// the UI shows proxy_status with pod events and offers one-click retry.
	deadline := time.Now().Add(18 * time.Second)
	for {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			status.Healthy = true
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			status.Message = "proxy healthz probe timeout"
			if ps, psErr := proxyMgr.GetProxyStatus(ctx, cfg, code); psErr == nil && ps != nil {
				status.PodEvents = ps.PodEvents
			}
			break
		}
		select {
		case <-ctx.Done():
			status.Message = ctx.Err().Error()
			return status
		case <-time.After(3 * time.Second):
		}
	}
	return status
}

// probeExternalProxy 对外部 Proxy 做一次短探活；失败仅落状态不报错。
// probeExternalProxy runs one short health probe against an external proxy; failures only
// land in the status without erroring.
func probeExternalProxy(status *K8sProxyStatus, baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		status.Message = err.Error()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		status.Healthy = true
		return
	}
	status.Message = fmt.Sprintf("external proxy healthz status %d", resp.StatusCode)
}

// DeleteK8sJavaProxy 仅清理伴生 Proxy（adopted 删除边界内唯一允许触碰的 K8s 资源）。
// DeleteK8sJavaProxy removes only the companion proxy — the single K8s resource the adopted
// deletion boundary is allowed to touch.
func (s *Service) DeleteK8sJavaProxy(ctx context.Context, cluster *Cluster) error {
	cfg := cluster.K8sSessionConfig()
	if cfg == nil || s.k8sManager == nil {
		return nil
	}
	proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)
	return proxyMgr.DeleteProxy(ctx, cfg, cluster.EffectiveClusterCode())
}

// UninstallK8sSession 卸载 stx_deployed 集群的 K8s 资源（显式勾选才调用；永不删外部 PVC）。
// UninstallK8sSession tears down stx_deployed K8s resources (only when explicitly confirmed;
// external PVCs are never deleted).
func (s *Service) UninstallK8sSession(ctx context.Context, cluster *Cluster) error {
	cfg := cluster.K8sSessionConfig()
	if cfg == nil || s.k8sManager == nil {
		return nil
	}
	if cfg.ManagementSource != k8s.ManagementSourceSTXDeployed {
		return errors.New("cluster: uninstall only applies to stx_deployed clusters")
	}
	deployer := k8s.NewSessionManager(s.k8sManager)
	return deployer.Uninstall(ctx, cfg, cluster.EffectiveClusterCode())
}

// ScaleK8sComponent 扩缩容 master/worker StatefulSet。
// ScaleK8sComponent scales the master/worker StatefulSet.
func (s *Service) ScaleK8sComponent(ctx context.Context, cluster *Cluster, component string, replicas int) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	if replicas < 0 || replicas > 50 {
		return errors.New("cluster: replicas out of range")
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	if err := ops.Scale(ctx, cfg, component, replicas); err != nil {
		return err
	}
	// 扩缩容成功后同步持久化期望副本数：健康检查与停止/启动恢复都读这份配置，
	// 不落库会在缩容后按旧副本数误报不健康。
	// Persist the desired replicas after scaling: health checks and stop/start restore
	// both read this config, and skipping the write would false-positive unhealthy
	// after a downscale.
	if component == "hybrid" {
		cfg.HybridReplicas = replicas
	} else if component == "master" {
		cfg.MasterReplicas = replicas
	} else if component == "worker" {
		cfg.WorkerReplicas = replicas
	} else {
		return nil
	}
	k8sJSON, err := k8sConfigJSON(cfg)
	if err != nil {
		return err
	}
	cluster.Config["kubernetes"] = k8sJSON
	return s.repo.Update(ctx, cluster)
}

// RolloutRestartK8s 滚动重启 master/worker。
// RolloutRestartK8s performs a rolling restart of master/worker.
func (s *Service) RolloutRestartK8s(ctx context.Context, cluster *Cluster, component string) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	return ops.RolloutRestart(ctx, cfg, component)
}

// UpgradeK8sImage 不可变镜像升级：滚动更新 master/worker 镜像 Tag，并同步滚动伴生 Proxy。
// UpgradeK8sImage performs immutable-image upgrade: rolls master/worker to the new tag and
// rolls the companion proxy in sync.
func (s *Service) UpgradeK8sImage(ctx context.Context, cluster *Cluster, image string, targetVersion ...string) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	// 升级目标为注册私仓镜像时同步代建凭据 Secret 并并入 imagePullSecrets，
	// 否则滚动后的新 Pod 必卡 ImagePullBackOff。
	// When the upgrade target is a registered private-registry image, materialize the
	// credential secret and merge imagePullSecrets first, otherwise rolled pods stall
	// in ImagePullBackOff.
	originalImage := cfg.Image
	cfg.Image = image
	if err := s.ensureK8sRegistryCredential(ctx, cfg, cluster); err != nil {
		cfg.Image = originalImage
		return err
	}
	cfg.Image = originalImage
	ops := k8s.NewSessionManager(s.k8sManager)
	if err := ops.UpgradeImage(ctx, cfg, image); err != nil {
		return err
	}
	// 前端识别出的目标版本随升级同步写回，保证集群元数据与运行镜像一致。
	// The version recognized by the frontend is written back alongside the upgrade so
	// cluster metadata stays consistent with the running image.
	if len(targetVersion) > 0 {
		if version := strings.TrimSpace(targetVersion[0]); version != "" && version != cluster.Version {
			cluster.Version = version
			if err := s.repo.Update(ctx, cluster); err != nil {
				return err
			}
		}
	}
	// 伴生 Proxy 与集群同镜像，升级需同步滚动否则插件清单版本漂移。
	// The companion proxy shares the cluster image, so it must roll in sync or the plugin
	// inventory version would drift.
	// 仅 managed 模式存在 STX 托管的伴生 Deployment 需要同步滚动；reuse/external/disabled
	// 的 Proxy 生命周期不归本集群管。
	// Only managed mode owns an STX-hosted companion deployment that must roll in sync;
	// reuse/external/disabled proxies have lifecycles outside this cluster.
	if mode := cfg.JavaProxy.Mode; mode == "" || mode == k8s.JavaProxyModeManaged {
		proxyDeployment, _, _ := k8s.ProxyResourceNames(cluster.EffectiveClusterCode())
		if cfg.JavaProxy.DeploymentName != "" {
			proxyDeployment = cfg.JavaProxy.DeploymentName
		}
		if err := ops.PatchProxyImage(ctx, cfg, proxyDeployment, image); err != nil {
			return err
		}
	}
	cfg.Image = image
	// 版本优先级：显式 targetVersion > 语义化镜像 tag。非语义化 tag（如 prod-202604）
	// 绝不回写版本，否则集群元数据被无意义字符串污染。
	// Version priority: explicit targetVersion wins over a semantic image tag. Non-semver
	// tags (e.g. prod-202604) never overwrite the version, or cluster metadata gets
	// polluted with meaningless strings.
	if len(targetVersion) == 0 || strings.TrimSpace(targetVersion[0]) == "" {
		if tag := imageTagVersion(image); tag != "" && semverLikeTag(tag) {
			cluster.Version = tag
		}
	}
	k8sJSON, err := k8sConfigJSON(cfg)
	if err != nil {
		return err
	}
	cluster.Config["kubernetes"] = k8sJSON
	return s.repo.Update(ctx, cluster)
}

// semverLikeTag 判定 tag 是否为语义化版本号（v?X.Y.Z）。
// semverLikeTag reports whether a tag is a semantic version (v?X.Y.Z).
func semverLikeTag(tag string) bool {
	trimmed := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// imageTagVersion 提取镜像 Tag 作为版本号；digest 引用与 latest 不视为具体版本。
// imageTagVersion extracts the image tag as a version; digest references and latest do
// not count as concrete versions.
func imageTagVersion(image string) string {
	if strings.Contains(image, "@sha256:") {
		return ""
	}
	idx := strings.LastIndex(image, ":")
	if idx < 0 || strings.Contains(image[idx+1:], "/") {
		return ""
	}
	tag := strings.TrimSpace(image[idx+1:])
	if tag == "" || tag == "latest" {
		return ""
	}
	return tag
}

// GetK8sPods 返回集群 Pod 投影（节点表数据源）。
// GetK8sPods returns the pod projection backing the node table.
func (s *Service) GetK8sPods(ctx context.Context, cluster *Cluster) ([]*k8s.PodInfo, error) {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return nil, err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	return ops.SessionPods(ctx, cfg, cluster.EffectiveClusterCode())
}

// GetK8sPodLogs 拉取 Pod 日志（替代 Agent 文件日志）。
// GetK8sPodLogs fetches pod logs (replacing Agent file logs).
func (s *Service) GetK8sPodLogs(ctx context.Context, cluster *Cluster, podName string, tailLines int) (string, error) {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return "", err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	// 越权防护：只允许读取本集群引擎 Pod 或本集群伴生 Proxy Pod 的日志，
	// 拒绝读取同 Namespace 其他 Pod。
	// Authorization guard: only engine pods of this cluster or its companion proxy pod
	// may be read; other pods in the same namespace are rejected.
	allowed := false
	if pods, err := ops.SessionPods(ctx, cfg, cluster.EffectiveClusterCode()); err == nil {
		for _, pod := range pods {
			if pod.Name == podName {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		proxyDeployment, _, _ := k8s.ProxyResourceNames(cluster.EffectiveClusterCode())
		if strings.HasPrefix(podName, proxyDeployment+"-") {
			allowed = true
		}
	}
	if !allowed {
		return "", fmt.Errorf("cluster: pod %q does not belong to this cluster", podName)
	}
	return ops.PodLogs(ctx, cfg, podName, int64(tailLines), nil)
}

// DeleteK8sPod 删除集群内单个引擎 Pod，由工作负载控制器自动重建。
// 只允许删除本集群引擎 Pod（SessionPods 投影已排除 Proxy 等基础设施 Pod），
// 拒绝删除同命名空间内不属于本集群的 Pod。
// DeleteK8sPod deletes a single engine pod so its workload controller recreates it.
// Only engine pods of this cluster may be deleted (the SessionPods projection already
// excludes infrastructure pods such as the proxy); foreign pods in the namespace are rejected.
func (s *Service) DeleteK8sPod(ctx context.Context, cluster *Cluster, podName string) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	allowed := false
	pods, err := ops.SessionPods(ctx, cfg, cluster.EffectiveClusterCode())
	if err != nil {
		return err
	}
	for _, pod := range pods {
		if pod.Name == podName {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("cluster: pod %q does not belong to this cluster", podName)
	}
	return ops.DeletePod(ctx, cfg, podName)
}

// ReadK8sConfigMap 读取集群 ConfigMap 全部文件键值。
// ReadK8sConfigMap reads all file entries of the cluster ConfigMap.
func (s *Service) ReadK8sConfigMap(ctx context.Context, cluster *Cluster) (map[string]string, error) {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return nil, err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	return ops.ReadConfigMap(ctx, cfg)
}

// WriteK8sConfigMapFile 更新 ConfigMap 单文件，可选滚动生效。
// WriteK8sConfigMapFile updates a single ConfigMap file with optional rolling restart.
func (s *Service) WriteK8sConfigMapFile(ctx context.Context, cluster *Cluster, fileName, content string, restart bool) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	return ops.WriteConfigMapFile(ctx, cfg, fileName, content, restart)
}

// IsLoopbackCallbackHost 判定回调主机是否为回环/未指定地址：Pod 内的 127.0.0.1/localhost
// 指向容器自身而非 STX 控制面，写进引擎配置必然上报失败，必须拦截。
// 该判定是控制面唯一权威实现，sync 包事件通道校验复用此函数。
// IsLoopbackCallbackHost reports whether the callback host is loopback/unspecified:
// inside a pod 127.0.0.1/localhost resolves to the container itself rather than the STX
// control plane, so writing it into engine config guarantees report failure.
// This is the single authoritative control-plane implementation, reused by the sync
// package's event channel validation.
func IsLoopbackCallbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || host == "::1" || host == "[::1]" || host == "0.0.0.0" || host == "::" {
		return true
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	return false
}

func (s *Service) requireK8sSession(cluster *Cluster) (*k8s.SessionConfig, error) {
	if s.k8sManager == nil {
		return nil, errors.New("cluster: k8s dependencies not configured")
	}
	cfg := cluster.K8sSessionConfig()
	if cfg == nil {
		return nil, errors.New("cluster: not a kubernetes cluster or session config invalid")
	}
	return cfg, nil
}

// ==================== HTTP Handlers HTTP 处理器 ====================

// CreateKubernetesCluster handles POST /api/v1/clusters/kubernetes.
func (h *Handler) CreateKubernetesCluster(c *gin.Context) {
	var req K8sClusterCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	result, err := h.service.CreateKubernetes(c.Request.Context(), &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
}

// loadK8sCluster 读取并校验 K8s 集群实体。
// loadK8sCluster loads and validates the K8s cluster entity.
func (h *Handler) loadK8sCluster(c *gin.Context) (*Cluster, bool) {
	var idReq struct {
		ID uint `uri:"id" binding:"required"`
	}
	if err := c.ShouldBindUri(&idReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid cluster id"})
		return nil, false
	}
	cluster, err := h.service.Get(c.Request.Context(), idReq.ID)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return nil, false
	}
	if !cluster.IsKubernetes() {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "cluster is not kubernetes infra"})
		return nil, false
	}
	return cluster, true
}

// ScaleK8s handles POST /api/v1/clusters/:id/k8s/scale.
func (h *Handler) ScaleK8s(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	var req struct {
		Component string `json:"component" binding:"required"`
		Replicas  int    `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	if err := h.service.ScaleK8sComponent(c.Request.Context(), cluster, req.Component, req.Replicas); err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// RolloutRestartK8s handles POST /api/v1/clusters/:id/k8s/rollout-restart.
func (h *Handler) RolloutRestartK8s(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	var req struct {
		Component string `json:"component" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	if err := h.service.RolloutRestartK8s(c.Request.Context(), cluster, req.Component); err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// UpgradeK8sImage handles POST /api/v1/clusters/:id/k8s/upgrade-image.
func (h *Handler) UpgradeK8sImage(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	var req struct {
		Image         string `json:"image" binding:"required"`
		TargetVersion string `json:"target_version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	if err := h.service.UpgradeK8sImage(c.Request.Context(), cluster, req.Image, req.TargetVersion); err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// GetK8sPods handles GET /api/v1/clusters/:id/k8s/pods.
func (h *Handler) GetK8sPodsHandler(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	pods, err := h.service.GetK8sPods(c.Request.Context(), cluster)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	if pods == nil {
		pods = []*k8s.PodInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": pods})
}

// GetK8sPodLogsHandler handles GET /api/v1/clusters/:id/k8s/pods/:pod/logs.
func (h *Handler) GetK8sPodLogsHandler(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	podName := c.Param("pod")
	tail := 500
	if v := c.Query("tail_lines"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			tail = n
		}
	}
	logs, err := h.service.GetK8sPodLogs(c.Request.Context(), cluster, podName, tail)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"logs": logs}})
}

// DeleteK8sPodHandler handles DELETE /api/v1/clusters/:id/k8s/pods/:pod.
func (h *Handler) DeleteK8sPodHandler(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	if err := h.service.DeleteK8sPod(c.Request.Context(), cluster, c.Param("pod")); err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// RetryK8sJavaProxy handles POST /api/v1/clusters/:id/k8s/java-proxy/retry — 一键重试拉起伴生 Proxy。
// RetryK8sJavaProxy re-triggers companion proxy provisioning after a failed auto-start.
func (h *Handler) RetryK8sJavaProxy(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	status := h.service.EnsureK8sJavaProxy(c.Request.Context(), cluster)
	if status != nil && status.Healthy && cluster.Status == ClusterStatusCreated {
		cluster.Status = ClusterStatusRunning
		_ = h.service.repo.Update(c.Request.Context(), cluster)
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": status})
}

// GetK8sConfigMap handles GET /api/v1/clusters/:id/k8s/configmap.
func (h *Handler) GetK8sConfigMap(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	data, err := h.service.ReadK8sConfigMap(c.Request.Context(), cluster)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	if data == nil {
		data = map[string]string{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": data})
}

// UpdateK8sConfigMapFile handles PUT /api/v1/clusters/:id/k8s/configmap/:file.
func (h *Handler) UpdateK8sConfigMapFile(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	fileName := c.Param("file")
	var req struct {
		Content string `json:"content"`
		Restart bool   `json:"restart"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	if err := h.service.WriteK8sConfigMapFile(c.Request.Context(), cluster, fileName, req.Content, req.Restart); err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// ListK8sImageConnectors 经伴生 Proxy 读取镜像内置连接器清单（source/sink/transform 三类合并去重）。
// 插件页只读边界的数据源；Proxy 不可达时返回错误由调用方降级为空。
// ListK8sImageConnectors reads image-bundled connectors through the companion proxy (merging
// source/sink/transform with dedupe). It backs the read-only plugin page; proxy-unreachable
// errors are returned so callers can degrade to empty.
func (s *Service) ListK8sImageConnectors(ctx context.Context, clusterID uint) ([]*pluginapp.ImageConnector, error) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	if cluster == nil || !cluster.IsKubernetes() {
		return nil, errors.New("cluster: not a kubernetes cluster")
	}
	cfg := cluster.K8sSessionConfig()
	if cfg == nil || s.k8sManager == nil {
		return nil, errors.New("cluster: k8s session config or dependencies missing")
	}
	mode := cfg.JavaProxy.Mode
	if mode == "" {
		mode = k8s.JavaProxyModeManaged
	}
	if mode == k8s.JavaProxyModeDisabled {
		return nil, errors.New("cluster: java proxy disabled, image connectors unavailable")
	}
	var base string
	var client *http.Client
	if mode == k8s.JavaProxyModeExternal {
		if cfg.JavaProxy.ExternalURL == "" {
			return nil, errors.New("cluster: java proxy external_url is empty")
		}
		base = strings.TrimRight(cfg.JavaProxy.ExternalURL, "/")
		client = &http.Client{Timeout: 15 * time.Second}
	} else {
		targetCfg := cfg
		_, serviceName, _ := k8s.ProxyResourceNames(cluster.EffectiveClusterCode())
		if cfg.JavaProxy.ServiceName != "" {
			serviceName = cfg.JavaProxy.ServiceName
		}
		port := cfg.JavaProxy.Port
		if mode == k8s.JavaProxyModeReuseCluster && cfg.JavaProxy.ReuseClusterID > 0 {
			if reuse, err := s.repo.GetByID(ctx, cfg.JavaProxy.ReuseClusterID, false); err == nil && reuse != nil {
				_, reuseSvc, _ := k8s.ProxyResourceNames(reuse.EffectiveClusterCode())
				serviceName = reuseSvc
				if reuseCfg := reuse.K8sSessionConfig(); reuseCfg != nil {
					targetCfg = reuseCfg
					if reuseCfg.JavaProxy.ServiceName != "" {
						serviceName = reuseCfg.JavaProxy.ServiceName
					}
					if reuseCfg.JavaProxy.Port > 0 {
						port = reuseCfg.JavaProxy.Port
					}
				}
			}
		}
		if port <= 0 {
			port = 18080
		}
		transport := k8s.NewTransport(s.k8sManager, targetCfg.HostID, targetCfg.Namespace, targetCfg.AccessMode, targetCfg.ExternalBaseURL)
		baseURL, err := transport.ServiceBaseURL(ctx, serviceName, port)
		if err != nil {
			return nil, err
		}
		base = baseURL
		httpClient, err := transport.HTTPClient(ctx, 15*time.Second)
		if err != nil {
			return nil, err
		}
		client = httpClient
	}

	type pluginListResponse struct {
		OK      bool `json:"ok"`
		Plugins []struct {
			FactoryIdentifier string `json:"factoryIdentifier"`
		} `json:"plugins"`
	}
	seen := map[string]struct{}{}
	out := []*pluginapp.ImageConnector{}
	for _, pluginType := range []string{"source", "sink", "transform"} {
		body := []byte(fmt.Sprintf(`{"pluginType":%q}`, pluginType))
		resp, err := client.Post(base+"/api/v1/plugin/list", "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("cluster: proxy plugin list returned status %d", resp.StatusCode)
		}
		var parsed pluginListResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		for _, item := range parsed.Plugins {
			name := strings.TrimSpace(item.FactoryIdentifier)
			if name == "" {
				continue
			}
			if _, dup := seen[strings.ToLower(name)]; dup {
				continue
			}
			seen[strings.ToLower(name)] = struct{}{}
			out = append(out, &pluginapp.ImageConnector{Name: name, Category: pluginType})
		}
	}
	return out, nil
}

// executeK8sLifecycleOperation 实现 K8s 集群统一的 Start/Stop/Restart：
// Stop 先快照实时副本数再缩 0，Start 恢复快照副本（至少 1），Restart 全量滚动重启。
// executeK8sLifecycleOperation implements the unified K8s Start/Stop/Restart lifecycle:
// Stop snapshots live replicas before scaling to zero, Start restores the snapshot (at
// least one), Restart rolls every configured workload.
func (s *Service) executeK8sLifecycleOperation(ctx context.Context, cluster *Cluster, operation OperationType) (*OperationResult, error) {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return nil, err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	result := &OperationResult{ClusterID: cluster.ID, Operation: operation, Success: true, NodeResults: []*NodeOperationResult{}}

	persist := func() error {
		k8sJSON, err := k8sConfigJSON(cfg)
		if err != nil {
			return err
		}
		cluster.Config["kubernetes"] = k8sJSON
		return s.repo.Update(ctx, cluster)
	}

	switch operation {
	case OperationRestart:
		if err := ops.RolloutRestart(ctx, cfg, ""); err != nil {
			return nil, err
		}
		cluster.Status = ClusterStatusRunning
		if err := persist(); err != nil {
			return nil, err
		}
		s.beginMemberGrace(cluster.ID)
		result.Message = "kubernetes workloads rolled"
		return result, nil

	case OperationStop:
		// 快照实时副本：外部 kubectl scale 修改过的也要以真实值为准，否则 Start 恢复错误副本数。
		// Snapshot live replicas: even externally kubectl-scaled values must win, or Start
		// would restore the wrong replica count.
		if cfg.IsHybrid() {
			if cfg.HybridWorkloadName != "" {
				if n, err := ops.GetReplicas(ctx, cfg, "hybrid"); err == nil && n > 0 {
					cfg.HybridReplicas = n
				}
				if err := ops.Scale(ctx, cfg, "hybrid", 0); err != nil {
					return nil, err
				}
			}
		} else {
			if cfg.MasterWorkloadName != "" {
				if n, err := ops.GetReplicas(ctx, cfg, "master"); err == nil && n > 0 {
					cfg.MasterReplicas = n
				}
			}
			if cfg.WorkerWorkloadName != "" {
				if n, err := ops.GetReplicas(ctx, cfg, "worker"); err == nil && n > 0 {
					cfg.WorkerReplicas = n
				}
			}
			if cfg.WorkerWorkloadName != "" {
				if err := ops.Scale(ctx, cfg, "worker", 0); err != nil {
					return nil, err
				}
			}
			if cfg.MasterWorkloadName != "" {
				if err := ops.Scale(ctx, cfg, "master", 0); err != nil {
					return nil, err
				}
			}
		}
		cluster.Status = ClusterStatusStopped
		if err := persist(); err != nil {
			return nil, err
		}
		result.Message = "kubernetes workloads scaled to zero"
		return result, nil

	case OperationStart:
		if cfg.IsHybrid() {
			hybridTarget := cfg.HybridReplicas
			if hybridTarget < 1 {
				hybridTarget = 1
			}
			if cfg.HybridWorkloadName != "" {
				if err := ops.Scale(ctx, cfg, "hybrid", hybridTarget); err != nil {
					return nil, err
				}
			}
		} else {
			masterTarget := cfg.MasterReplicas
			if masterTarget < 1 {
				masterTarget = 1
			}
			workerTarget := cfg.WorkerReplicas
			if workerTarget < 1 {
				workerTarget = 1
			}
			if cfg.MasterWorkloadName != "" {
				if err := ops.Scale(ctx, cfg, "master", masterTarget); err != nil {
					return nil, err
				}
			}
			if cfg.WorkerWorkloadName != "" {
				if err := ops.Scale(ctx, cfg, "worker", workerTarget); err != nil {
					return nil, err
				}
			}
		}
		cluster.Status = ClusterStatusRunning
		if err := persist(); err != nil {
			return nil, err
		}
		s.beginMemberGrace(cluster.ID)
		result.Message = "kubernetes workloads scaled back"
		return result, nil
	}
	return nil, fmt.Errorf("cluster: unsupported operation %q for kubernetes cluster", operation)
}

// preferCredentialedImageRow 在同镜像串的注册行中优先选出带凭据的行。
// preferCredentialedImageRow picks the row carrying credentials among rows
// registered for the same image string, falling back to the first row.
func preferCredentialedImageRow(rows []installerapp.ContainerImage) installerapp.ContainerImage {
	for i := range rows {
		if rows[i].HasCredentials() {
			return rows[i]
		}
	}
	return rows[0]
}

// ensureK8sRegistryCredential 解析注册镜像的私有仓库凭据并代建拉取 Secret。
// 镜像未注册或无凭据时静默跳过；已存在同名 Secret 且归属本集群时原地更新。
// ensureK8sRegistryCredential resolves the registered image's private-registry
// credentials and materializes the pull secret. Unregistered images or missing
// credentials are a no-op; an existing cluster-owned secret is updated in place.
func (s *Service) ensureK8sRegistryCredential(ctx context.Context, cfg *k8s.SessionConfig, cluster *Cluster) error {
	if s.k8sManager == nil {
		return nil
	}
	imageRef := strings.TrimSpace(cfg.Image)
	if imageRef == "" {
		return nil
	}
	if s.repo == nil || s.repo.db == nil {
		// 无数据库句柄时无法查注册表，凭据缺失按未注册处理，不阻断部署。
		// Without a database handle the registry lookup is impossible; treat credentials
		// as absent rather than blocking the deploy.
		return nil
	}
	// 同一镜像串可注册多行（同名不同凭据配置），优先命中带凭据的行，
	// 否则无凭据的默认行会抢先导致私有仓库凭据永远不落 Secret。
	// Multiple rows can register the same image string; prefer the row carrying
	// credentials, otherwise a credential-less default row wins the lookup and the
	// private-registry secret never materializes.
	var rows []installerapp.ContainerImage
	if err := s.repo.db.WithContext(ctx).Where("image = ?", imageRef).Find(&rows).Error; err != nil || len(rows) == 0 {
		// 未注册的自定义镜像：凭据依赖用户预建 secret，不阻断。
		// Unregistered custom images rely on user-provisioned secrets; do not block.
		return nil
	}
	row := preferCredentialedImageRow(rows)
	if !row.HasCredentials() {
		return nil
	}
	password, err := row.DecryptRegistryPassword()
	if err != nil {
		return fmt.Errorf("cluster: decrypt registry credential: %w", err)
	}
	clientset, err := s.k8sManager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	// 凭据 Secret 可能先于 Deploy 的 ensureNamespace 执行（全新 namespace 场景），
	// 必须自建命名空间否则 Secret 创建必炸 namespaces "<ns>" not found。
	// The credential secret can run before Deploy's ensureNamespace (brand-new namespace
	// scenario), so the namespace must be ensured here or secret creation fails hard with
	// namespaces "<ns>" not found.
	if err := k8s.EnsureNamespace(ctx, clientset, cfg.Namespace); err != nil {
		return err
	}
	secretName, err := k8s.EnsureRegistryCredentialSecret(ctx, clientset, cfg.Namespace, cluster.EffectiveClusterCode(), row.RegistryHost, row.RegistryUsername, password)
	if err != nil {
		return err
	}
	for _, existing := range cfg.ImagePullSecrets {
		if existing == secretName {
			return nil
		}
	}
	cfg.ImagePullSecrets = append(cfg.ImagePullSecrets, secretName)
	return nil
}
