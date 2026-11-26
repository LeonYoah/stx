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

// Package k8s 提供 STX 接入 Kubernetes 的统一适配层：客户端管理、传输代理、
// 集群发现、常驻集群部署/运维与伴生 stx-java-proxy 托管。
// Package k8s provides STX's unified Kubernetes integration layer: client management,
// transport proxying, cluster discovery, session cluster deploy/ops and companion
// stx-java-proxy hosting.
package k8s

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ManagementSource 常驻集群的来源：纳管已有集群或 STX 一键部署。
// ManagementSource distinguishes adopted clusters from STX-deployed ones.
type ManagementSource string

const (
	// ManagementSourceAdopted 纳管用户已有集群；删除 STX 记录时绝不删除用户资源。
	// ManagementSourceAdopted adopts an existing user cluster; deleting the STX record never
	// deletes user resources.
	ManagementSourceAdopted ManagementSource = "adopted"
	// ManagementSourceSTXDeployed STX 一键部署的集群；卸载时可显式选择清理 STX 创建的 K8s 资源。
	// ManagementSourceSTXDeployed is deployed by STX; teardown may explicitly clean STX-created resources.
	ManagementSourceSTXDeployed ManagementSource = "stx_deployed"
)

// AccessMode K8sTransport 的访问模式。
// AccessMode selects how K8sTransport reaches in-cluster HTTP endpoints.
type AccessMode string

const (
	// AccessModeAPIServerProxy 经 K8s API Server 的 pods/services proxy 访问（默认，已实测）。
	// AccessModeAPIServerProxy reaches pods/services through the API server proxy (default, verified).
	AccessModeAPIServerProxy AccessMode = "apiserver_proxy"
	// AccessModeInCluster 控制面自身在集群内，直连 Pod IP / Service DNS。
	// AccessModeInCluster connects to pod IPs / service DNS directly when the control plane
	// itself runs inside the cluster.
	AccessModeInCluster AccessMode = "in_cluster"
	// AccessModeExternal 直连用户配置的外部入口（Ingress/NodePort/port-forward 等）。
	// AccessModeExternal uses the user-provided external base URL (Ingress/NodePort/port-forward).
	AccessModeExternal AccessMode = "external"
)

// JavaProxyMode 伴生 stx-java-proxy 托管模式。
// JavaProxyMode selects how the companion stx-java-proxy is provided.
type JavaProxyMode string

const (
	// JavaProxyModeManaged STX 自动托管伴生 Deployment（默认）。
	// JavaProxyModeManaged lets STX host the companion Deployment automatically (default).
	JavaProxyModeManaged JavaProxyMode = "managed"
	// JavaProxyModeReuseCluster 复用同 Host/Namespace 下另一集群的伴生 Proxy。
	// JavaProxyModeReuseCluster reuses the companion proxy of another cluster in the same
	// host/namespace to save memory.
	JavaProxyModeReuseCluster JavaProxyMode = "reuse_cluster"
	// JavaProxyModeExternal 用户自备外部 Proxy，经 external_url 直连。
	// JavaProxyModeExternal uses a user-provided external proxy reachable at external_url.
	JavaProxyModeExternal JavaProxyMode = "external"
	// JavaProxyModeDisabled 显式关闭伴生 Proxy，21 项智能辅助不可用。
	// JavaProxyModeDisabled disables the companion proxy; the smart-assist APIs stay unavailable.
	JavaProxyModeDisabled JavaProxyMode = "disabled"
)

// JavaProxyConfig 伴生 stx-java-proxy 配置，与 Cluster.Config["kubernetes"]["java_proxy"] 对应。
// JavaProxyConfig mirrors Cluster.Config["kubernetes"]["java_proxy"].
type JavaProxyConfig struct {
	Mode           JavaProxyMode `json:"mode"`
	ReuseClusterID uint          `json:"reuse_cluster_id"`
	DeploymentName string        `json:"deployment_name"`
	ServiceName    string        `json:"service_name"`
	Port           int           `json:"port"`
	JVMOpts        string        `json:"jvm_opts"`
	ExternalURL    string        `json:"external_url"`
}

// ResourceRequirement master/worker 的资源声明（requests=limits，K8s 集群 immutable QoS 简单化）。
// ResourceRequirement declares cpu/memory for master/worker pods.
type ResourceRequirement struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// SessionResources 常驻集群的 master/worker 资源声明。
// SessionResources groups master/worker resource requirements.
type SessionResources struct {
	Master ResourceRequirement `json:"master"`
	Worker ResourceRequirement `json:"worker"`
	// Hybrid 混合形态资源配比（可空，hybrid 模式使用）。
	// Hybrid is the hybrid-mode resource sizing (nullable, hybrid mode only).
	Hybrid *ResourceRequirement `json:"hybrid,omitempty"`
}

// SessionStorage 常驻集群存储配置。
// SessionStorage holds session-cluster storage options.
type SessionStorage struct {
	CheckpointPVC    string `json:"checkpoint_pvc"`
	DataStorageClass string `json:"data_storage_class"`
	DataSize         string `json:"data_size"`
}

// SessionConfig 常驻 K8s 集群配置，与 Cluster.Config["kubernetes"] 一一对应。
// SessionConfig is the session-cluster configuration mirroring Cluster.Config["kubernetes"].
type SessionConfig struct {
	HostID           uint             `json:"host_id"`
	Namespace        string           `json:"namespace"`
	ManagementSource ManagementSource `json:"management_source"`
	AccessMode       AccessMode       `json:"access_mode"`
	ExternalBaseURL  string           `json:"external_base_url"`
	STXCallbackURL   string           `json:"stx_callback_url"`

	Image            string   `json:"image"`
	ImagePullPolicy  string   `json:"image_pull_policy"`
	ImagePullSecrets []string `json:"image_pull_secrets"`
	ServiceAccount   string   `json:"service_account"`

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

	Resources SessionResources `json:"resources"`
	Storage   SessionStorage   `json:"storage"`
	JavaProxy JavaProxyConfig  `json:"java_proxy"`

	// DeploymentMode 部署形态：separated（master+worker 分离）或 hybrid（单进程混合）。
	// 空值等价 separated，保证既有集群零迁移。
	// DeploymentMode selects separated (master+worker) or hybrid (single-process) topology;
	// empty means separated so existing clusters need no migration.
	DeploymentMode string `json:"deployment_mode,omitempty"`
	// HybridReplicas hybrid 形态下的副本数。
	// HybridReplicas is the replica count in hybrid mode.
	HybridReplicas int `json:"hybrid_replicas,omitempty"`
	// HybridWorkloadName hybrid 形态的 StatefulSet 名称。
	// HybridWorkloadName is the StatefulSet name in hybrid mode.
	HybridWorkloadName string `json:"hybrid_workload_name,omitempty"`

	// JVM 引擎 JVM 参数与容器内存联动（可空走默认渲染）。
	// JVM carries engine JVM options linked with container memory (nullable, default rendering).
	JVM *SessionJVMConfig `json:"jvm,omitempty"`
	// Runtime 高级运行参数（inspect 等开关）。
	// Runtime carries advanced runtime toggles.
	Runtime *SessionRuntimeConfig `json:"runtime,omitempty"`
	// Checkpoint 运行时 Checkpoint 存储草稿（部署时渲染进 seatunnel.yaml）。
	// Checkpoint is the checkpoint storage draft rendered into seatunnel.yaml at deploy time.
	Checkpoint *SessionStorageSpec `json:"checkpoint,omitempty"`
	// IMAP 运行时 IMAP 存储草稿（渲染进 hazelcast yaml）。
	// IMAP is the IMAP storage draft rendered into the hazelcast yaml.
	IMAP *SessionStorageSpec `json:"imap,omitempty"`

	// ExtraMounts 预留多条 ConfigMap/Secret 挂载与伴随环境变量，供后续 Hadoop/Kerberos
	// 挂载包平滑衔接；P1 仅落库不渲染。
	// ExtraMounts reserves additional ConfigMap/Secret mounts with companion env vars for
	// future Hadoop/Kerberos mount packs; P1 persists them without rendering.
	ExtraMounts []ExtraMount `json:"extra_mounts,omitempty"`
}

// SessionJVMConfig 引擎 JVM 配置：堆内存与附加参数。
// SessionJVMConfig carries engine JVM heap and extra options.
type SessionJVMConfig struct {
	Xms string `json:"xms,omitempty"`
	Xmx string `json:"xmx,omitempty"`
	// ExtraOptions 追加 JVM 参数行。
	// ExtraOptions appends raw JVM option lines.
	ExtraOptions []string `json:"extra_options,omitempty"`
}

// SessionRuntimeConfig 高级运行参数。
// SessionRuntimeConfig carries advanced runtime toggles.
type SessionRuntimeConfig struct {
	// EnableHTTPServer 是否启用引擎 HTTP 服务（2.3.9+ 默认开）。
	// EnableHTTPServer toggles the engine HTTP service (default on for 2.3.9+).
	EnableHTTPServer *bool `json:"enable_http_server,omitempty"`
}

// SessionStorageSpec Checkpoint/IMAP 存储草稿：类型 + 连接参数。
// SessionStorageSpec is the checkpoint/IMAP storage draft: type plus connection params.
type SessionStorageSpec struct {
	Enabled     bool              `json:"enabled"`
	StorageType string            `json:"storage_type"` // LOCAL_FILE / S3 / OSS / HDFS / DISABLED
	Namespace   string            `json:"namespace,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Bucket      string            `json:"bucket,omitempty"`
	AccessKey   string            `json:"access_key,omitempty"`
	SecretKey   string            `json:"secret_key,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// IsHybrid 判定 hybrid 部署形态。
// IsHybrid reports hybrid topology.
func (c *SessionConfig) IsHybrid() bool {
	return c.DeploymentMode == "hybrid"
}

// ExtraMount 描述一条附加挂载：来源（ConfigMap/Secret）+ 挂载路径 + 可选环境变量。
// ExtraMount describes one additional mount: source (ConfigMap/Secret) + mount path + optional env vars.
type ExtraMount struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"` // configmap / secret
	Source    string            `json:"source"`
	MountPath string            `json:"mount_path"`
	ReadOnly  bool              `json:"read_only"`
	Env       map[string]string `json:"env,omitempty"`
}

// ApplyDefaults 填充默认值；只覆盖零值字段，不覆盖用户显式配置。
// ApplyDefaults fills default values; only zero-value fields are touched.
func (c *SessionConfig) ApplyDefaults() {
	if c.Namespace == "" {
		c.Namespace = "default"
	}
	if c.ManagementSource == "" {
		c.ManagementSource = ManagementSourceAdopted
	}
	if c.AccessMode == "" {
		c.AccessMode = AccessModeAPIServerProxy
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = "IfNotPresent"
	}
	if c.MasterWorkloadKind == "" {
		c.MasterWorkloadKind = "StatefulSet"
	}
	if c.WorkerWorkloadKind == "" {
		c.WorkerWorkloadKind = "StatefulSet"
	}
	if c.MasterReplicas <= 0 {
		c.MasterReplicas = 1
	}
	if c.WorkerReplicas <= 0 {
		c.WorkerReplicas = 2
	}
	if c.MasterHTTPPort <= 0 {
		c.MasterHTTPPort = 8080
	}
	if c.HazelcastPort <= 0 {
		c.HazelcastPort = 5801
	}
	if c.MetricsPort <= 0 {
		c.MetricsPort = 8080
	}
	if c.MetricsPath == "" {
		c.MetricsPath = "/metrics"
	}
	if c.JavaProxy.Mode == "" {
		c.JavaProxy.Mode = JavaProxyModeManaged
	}
	if c.JavaProxy.Port <= 0 {
		c.JavaProxy.Port = 18080
	}
	if c.JavaProxy.JVMOpts == "" {
		c.JavaProxy.JVMOpts = "-Xms64m -Xmx512m"
	}
}

// Validate 校验端口契约：hazelcast_port 与 master_http_port 均须在 1-65535 范围内且互不相同。
// Validate enforces the port contract: hazelcast_port and master_http_port must both be within
// 1-65535 and differ from each other.
func (c *SessionConfig) Validate() error {
	for _, port := range []struct {
		name  string
		value int
	}{
		{"hazelcast_port", c.HazelcastPort},
		{"master_http_port", c.MasterHTTPPort},
	} {
		if port.value < 1 || port.value > 65535 {
			return fmt.Errorf("k8s: %s must be in range 1-65535, got %d", port.name, port.value)
		}
	}
	if c.HazelcastPort == c.MasterHTTPPort {
		return fmt.Errorf("k8s: hazelcast_port and master_http_port must differ, both are %d", c.HazelcastPort)
	}
	if c.HostID == 0 {
		return fmt.Errorf("k8s: host_id is required")
	}
	switch c.ManagementSource {
	case ManagementSourceAdopted, ManagementSourceSTXDeployed:
	default:
		return fmt.Errorf("k8s: unsupported management_source %q", c.ManagementSource)
	}
	// 仅一键部署强制要求镜像：纳管模式下伴生 Proxy 从目标 Workload Pod 模板提取镜像。
	// Only stx_deployed requires an explicit image: in adopted mode the companion
	// proxy clones the image from the target workload pod template.
	if c.ManagementSource == ManagementSourceSTXDeployed && strings.TrimSpace(c.Image) == "" {
		return fmt.Errorf("k8s: image is required")
	}
	switch c.AccessMode {
	case AccessModeAPIServerProxy, AccessModeInCluster, AccessModeExternal:
	default:
		return fmt.Errorf("k8s: unsupported access_mode %q", c.AccessMode)
	}
	if c.AccessMode == AccessModeExternal && strings.TrimSpace(c.ExternalBaseURL) == "" {
		return fmt.Errorf("k8s: external_base_url is required when access_mode is external")
	}
	switch c.JavaProxy.Mode {
	case JavaProxyModeManaged, JavaProxyModeReuseCluster, JavaProxyModeExternal, JavaProxyModeDisabled:
	default:
		return fmt.Errorf("k8s: unsupported java_proxy.mode %q", c.JavaProxy.Mode)
	}
	if c.JavaProxy.Port < 1 || c.JavaProxy.Port > 65535 {
		return fmt.Errorf("k8s: java_proxy.port must be in range 1-65535, got %d", c.JavaProxy.Port)
	}
	return nil
}

// ParseSessionConfig 从 Cluster.Config["kubernetes"] 的 map 结构解析为强类型配置；
// 用 JSON 往返转换，避免逐字段手写断言遗漏。
// ParseSessionConfig parses Cluster.Config["kubernetes"] into the typed config via a JSON
// round trip, avoiding hand-written per-field assertions.
func ParseSessionConfig(raw interface{}) (*SessionConfig, error) {
	cfg := &SessionConfig{}
	if raw == nil {
		cfg.ApplyDefaults()
		return cfg, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("k8s: invalid kubernetes config: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("k8s: invalid kubernetes config: %w", err)
	}
	cfg.ApplyDefaults()
	return cfg, nil
}
