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
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

// ErrK8sHostInvalid 表示创建 K8s 集群时引用的主机不存在或类型不符。
// ErrK8sHostInvalid means the referenced host is missing or not a kubernetes host.
var ErrK8sHostInvalid = errors.New("cluster: k8s host invalid or not kubernetes type")

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
	ManagementSource string `json:"management_source" binding:"required"`
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
	JavaProxyMode         string `json:"java_proxy_mode"`
	JavaProxyJVMOpts      string `json:"java_proxy_jvm_opts"`
	JavaProxyExternalURL  string `json:"java_proxy_external_url"`
	MasterCPU             string `json:"master_cpu"`
	MasterMemory          string `json:"master_memory"`
	WorkerCPU             string `json:"worker_cpu"`
	WorkerMemory          string `json:"worker_memory"`
	CheckpointPVC         string `json:"checkpoint_pvc"`
	DataStorageClass      string `json:"data_storage_class"`
	DataSize              string `json:"data_size"`
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
	cfg.Resources.Master.CPU = req.MasterCPU
	cfg.Resources.Master.Memory = req.MasterMemory
	cfg.Resources.Worker.CPU = req.WorkerCPU
	cfg.Resources.Worker.Memory = req.WorkerMemory
	cfg.Storage.CheckpointPVC = req.CheckpointPVC
	cfg.Storage.DataStorageClass = req.DataStorageClass
	cfg.Storage.DataSize = req.DataSize
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
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
	Healthy     bool     `json:"healthy"`
	Message     string   `json:"message,omitempty"`
	PodPhase    string   `json:"pod_phase,omitempty"`
	PodEvents   []string `json:"pod_events,omitempty"`
	ServiceURL  string   `json:"service_url,omitempty"`
}

// CreateKubernetesResult 创建结果：集群实体 + 伴生 Proxy 状态（失败不阻断保存）。
// CreateKubernetesResult pairs the cluster entity with companion proxy status
// (proxy failure does not block saving).
type CreateKubernetesResult struct {
	Cluster     *Cluster        `json:"cluster"`
	ProxyStatus *K8sProxyStatus `json:"proxy_status,omitempty"`
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
		DeploymentMode: DeploymentModeSeparated,
		Status:         ClusterStatusCreated,
		InfraType:      "kubernetes",
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
	if cfg.ManagementSource == k8s.ManagementSourceSTXDeployed {
		deployer := k8s.NewSessionManager(s.k8sManager)
		if _, err := deployer.Deploy(ctx, &k8s.DeployOptions{Config: cfg, ClusterCode: cluster.EffectiveClusterCode(), ClusterID: cluster.ID, CreateRBAC: true}); err != nil {
			_ = s.repo.Delete(ctx, cluster.ID)
			return nil, fmt.Errorf("deploy kubernetes resources: %w", err)
		}
	}

	k8sJSON, err := k8sConfigJSON(cfg)
	if err != nil {
		return nil, err
	}
	cluster.Config["kubernetes"] = k8sJSON
	if err := s.repo.Update(ctx, cluster); err != nil {
		return nil, err
	}

	proxyStatus := s.EnsureK8sJavaProxy(ctx, cluster)
	return &CreateKubernetesResult{Cluster: cluster, ProxyStatus: proxyStatus}, nil
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
	if cfg.JavaProxy.Mode == k8s.JavaProxyModeDisabled {
		status.Message = "java proxy disabled"
		return status
	}

	proxyMgr := k8s.NewJavaProxyManager(s.k8sManager, s.k8sJarLoader)
	code := cluster.EffectiveClusterCode()
	epoch := seatunnel.ProxyEpochForVersion(cluster.Version)
	if _, err := proxyMgr.EnsureProxy(ctx, cfg, code, cluster.ID, epoch); err != nil {
		status.Message = err.Error()
		return status
	}

	transport := k8s.NewTransport(s.k8sManager, cfg.HostID, cfg.Namespace, cfg.AccessMode, cfg.ExternalBaseURL)
	_, serviceName, _ := k8s.ProxyResourceNames(code)
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	port := cfg.JavaProxy.Port
	if port <= 0 {
		port = 18080
	}
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

	// 伴生 Proxy 启动需加载引擎 jar，首轮探活给足宽限再判定。
	// The companion proxy loads engine jars on boot, so the first health probe gets a generous
	// grace window before being judged failed.
	deadline := time.Now().Add(90 * time.Second)
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
	return ops.Scale(ctx, cfg, component, replicas)
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
func (s *Service) UpgradeK8sImage(ctx context.Context, cluster *Cluster, image string) error {
	cfg, err := s.requireK8sSession(cluster)
	if err != nil {
		return err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	if err := ops.UpgradeImage(ctx, cfg, image); err != nil {
		return err
	}
	// 伴生 Proxy 与集群同镜像，升级需同步滚动否则插件清单版本漂移。
	// The companion proxy shares the cluster image, so it must roll in sync or the plugin
	// inventory version would drift.
	proxyDeployment, _, _ := k8s.ProxyResourceNames(cluster.EffectiveClusterCode())
	if cfg.JavaProxy.DeploymentName != "" {
		proxyDeployment = cfg.JavaProxy.DeploymentName
	}
	if err := ops.PatchProxyImage(ctx, cfg, proxyDeployment, image); err != nil {
		return err
	}
	cfg.Image = image
	k8sJSON, err := k8sConfigJSON(cfg)
	if err != nil {
		return err
	}
	cluster.Config["kubernetes"] = k8sJSON
	return s.repo.Update(ctx, cluster)
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
	return ops.PodLogs(ctx, cfg, podName, int64(tailLines), nil)
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
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	if err := h.service.UpgradeK8sImage(c.Request.Context(), cluster, req.Image); err != nil {
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

// RetryK8sJavaProxy handles POST /api/v1/clusters/:id/k8s/java-proxy/retry — 一键重试拉起伴生 Proxy。
// RetryK8sJavaProxy re-triggers companion proxy provisioning after a failed auto-start.
func (h *Handler) RetryK8sJavaProxy(c *gin.Context) {
	cluster, ok := h.loadK8sCluster(c)
	if !ok {
		return
	}
	status := h.service.EnsureK8sJavaProxy(c.Request.Context(), cluster)
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
	transport := k8s.NewTransport(s.k8sManager, cfg.HostID, cfg.Namespace, cfg.AccessMode, cfg.ExternalBaseURL)
	_, serviceName, _ := k8s.ProxyResourceNames(cluster.EffectiveClusterCode())
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	port := cfg.JavaProxy.Port
	if port <= 0 {
		port = 18080
	}
	base, err := transport.ServiceBaseURL(ctx, serviceName, port)
	if err != nil {
		return nil, err
	}
	client, err := transport.HTTPClient(ctx, 15*time.Second)
	if err != nil {
		return nil, err
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
