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

/**
 * Cluster Service Types
 * 集群服务类型定义
 *
 * This file defines all types related to cluster management.
 * 本文件定义所有与集群管理相关的类型。
 */

/* eslint-disable no-unused-vars */

/**
 * Deployment mode enumeration
 * 部署模式枚举
 */
export enum DeploymentMode {
  /** Hybrid mode - master and worker on same nodes / 混合模式 - master 和 worker 在同一节点 */
  HYBRID = 'hybrid',
  /** Separated mode - master and worker on different nodes / 分离模式 - master 和 worker 在不同节点 */
  SEPARATED = 'separated',
}

/**
 * Cluster status enumeration
 * 集群状态枚举
 */
export enum ClusterStatus {
  /** Cluster created but not deployed / 集群已创建但未部署 */
  CREATED = 'created',
  /** Cluster is being deployed / 集群正在部署中 */
  DEPLOYING = 'deploying',
  /** Cluster is running normally / 集群正常运行中 */
  RUNNING = 'running',
  /** Cluster has been stopped / 集群已停止 */
  STOPPED = 'stopped',
  /** Cluster is in error state / 集群处于错误状态 */
  ERROR = 'error',
}

/**
 * Node role enumeration
 * 节点角色枚举
 */
export enum NodeRole {
  /** Master node / 主节点 */
  MASTER = 'master',
  /** Worker node / 工作节点 */
  WORKER = 'worker',
  /** Master and Worker node (hybrid mode) / 主节点和工作节点（混合模式） */
  MASTER_WORKER = 'master/worker',
}

/**
 * Node status enumeration
 * 节点状态枚举
 */
export enum NodeStatus {
  /** Node is pending deployment / 节点待部署 */
  PENDING = 'pending',
  /** Node is being installed / 节点正在安装 */
  INSTALLING = 'installing',
  /** Node is running normally / 节点正常运行 */
  RUNNING = 'running',
  /** Node has been stopped / 节点已停止 */
  STOPPED = 'stopped',
  /** Node is in error state / 节点处于错误状态 */
  ERROR = 'error',
  /** Host/agent is offline; display-only / 主机或 Agent 离线，仅展示用 */
  OFFLINE = 'offline',
}

/**
 * Health status enumeration
 * 健康状态枚举
 */
export enum HealthStatus {
  /** All nodes are online and running / 所有节点在线且运行正常 */
  HEALTHY = 'healthy',
  /** One or more nodes are offline or in error state / 一个或多个节点离线或错误 */
  UNHEALTHY = 'unhealthy',
  /** Health status cannot be determined / 无法确定健康状态 */
  UNKNOWN = 'unknown',
}

/**
 * Operation type enumeration
 * 操作类型枚举
 */
export enum OperationType {
  /** Start cluster / 启动集群 */
  START = 'start',
  /** Stop cluster / 停止集群 */
  STOP = 'stop',
  /** Restart cluster / 重启集群 */
  RESTART = 'restart',
}

/**
 * Cluster configuration type
 * 集群配置类型
 */
export type ClusterConfig = Record<string, unknown>;

/**
 * Cluster JVM defaults
 * 集群 JVM 默认值
 */
export interface ClusterJVMConfig {
  hybrid_heap_size?: number;
  master_heap_size?: number;
  worker_heap_size?: number;
}

/**
 * Cluster port defaults
 * 集群端口默认值
 */
export interface ClusterPortConfig {
  master_hazelcast_port?: number;
  master_api_port?: number;
  worker_port?: number;
  java_proxy_port?: number;
}

/**
 * Node-level JVM overrides
 * 节点级 JVM overrides
 */
export interface NodeJVMOverrides {
  hybrid_heap_size?: number;
  master_heap_size?: number;
  worker_heap_size?: number;
}

/**
 * Node-level JSON overrides
 * 节点级 JSON overrides
 */
export interface NodeOverrides {
  jvm?: NodeJVMOverrides;
}

/**
 * Cluster information returned from API
 * API 返回的集群信息
 */
export interface ClusterInfo {
  /** Cluster ID / 集群 ID */
  id: number;
  /** Cluster name / 集群名称 */
  name: string;
  /** Description / 描述 */
  description: string;
  /** Deployment mode / 部署模式 */
  deployment_mode: DeploymentMode;
  /** SeaTunnel version / SeaTunnel 版本 */
  version: string;
  /** Cluster status / 集群状态 */
  status: ClusterStatus;
  /** Installation directory / 安装目录 */
  install_dir: string;
  /** Cluster configuration / 集群配置 */
  config: ClusterConfig;
  /** Number of nodes / 节点数量 */
  node_count: number;
  /** Number of nodes whose host is online / 主机在线的节点数 */
  online_nodes?: number;
  /** Health status: healthy, unhealthy, unknown / 健康状态 */
  health_status?: string;
  /** Creation time / 创建时间 */
  created_at: string;
  /** Update time / 更新时间 */
  updated_at: string;
}

/**
 * Node information returned from API
 * API 返回的节点信息
 */
export interface NodeInfo {
  /** Node ID / 节点 ID */
  id: number;
  /** Cluster ID / 集群 ID */
  cluster_id: number;
  /** Host ID / 主机 ID */
  host_id: number;
  /** Host name / 主机名称 */
  host_name: string;
  /** Host IP address / 主机 IP 地址 */
  host_ip: string;
  /** Node role / 节点角色 */
  role: NodeRole;
  /** SeaTunnel installation directory / SeaTunnel 安装目录 */
  install_dir: string;
  /** Hazelcast cluster port / Hazelcast 集群端口 */
  hazelcast_port: number;
  /** REST API port (Master only) / REST API 端口（仅 Master） */
  api_port: number;
  /** Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式） */
  worker_port: number;
  /** Node-level JSON overrides / 节点级 JSON 覆盖配置 */
  overrides?: NodeOverrides;
  /** Node status / 节点状态 (may be 'offline' when host is offline) */
  status: NodeStatus;
  /** Whether host is online / 主机是否在线 */
  is_online?: boolean;
  /** Process PID / 进程 PID */
  process_pid: number;
  /** Process status / 进程状态 */
  process_status: string;
  /** Creation time / 创建时间 */
  created_at: string;
  /** Update time / 更新时间 */
  updated_at: string;
}

/**
 * Node status information (detailed)
 * 节点状态信息（详细）
 */
export interface NodeStatusInfo {
  /** Node ID / 节点 ID */
  node_id: number;
  /** Host ID / 主机 ID */
  host_id: number;
  /** Host name / 主机名称 */
  host_name: string;
  /** Host IP address / 主机 IP 地址 */
  host_ip: string;
  /** Node role / 节点角色 */
  role: NodeRole;
  /** Node status / 节点状态 */
  status: NodeStatus;
  /** Whether the node is online / 节点是否在线 */
  is_online: boolean;
  /** Process PID / 进程 PID */
  process_pid: number;
  /** Process status / 进程状态 */
  process_status: string;
}

/**
 * Cluster status information (detailed)
 * 集群状态信息（详细）
 */
export interface ClusterStatusInfo {
  /** Cluster ID / 集群 ID */
  cluster_id: number;
  /** Cluster name / 集群名称 */
  cluster_name: string;
  /** Cluster status / 集群状态 */
  status: ClusterStatus;
  /** Health status / 健康状态 */
  health_status: HealthStatus;
  /** Total number of nodes / 节点总数 */
  total_nodes: number;
  /** Number of online nodes / 在线节点数 */
  online_nodes: number;
  /** Number of offline nodes / 离线节点数 */
  offline_nodes: number;
  /** Node status list / 节点状态列表 */
  nodes: NodeStatusInfo[];
}

export interface RuntimeStorageNodeStat {
  node_id: number;
  host_id: number;
  host_name: string;
  path?: string;
  exists: boolean;
  size_bytes: number;
  message?: string;
}

export interface RuntimeStorageSpec {
  kind: string;
  enabled: boolean;
  storage_type: string;
  namespace?: string;
  endpoint?: string;
  bucket?: string;
  external: boolean;
  hdfs_ha_enabled?: boolean;
  hdfs_name_services?: string;
  hdfs_ha_namenodes?: string;
  hdfs_namenode_rpc_address_1?: string;
  hdfs_namenode_rpc_address_2?: string;
  hdfs_failover_proxy_provider?: string;
  hdfs_namenode_host?: string;
  hdfs_namenode_port?: number;
  kerberos_principal?: string;
  kerberos_keytab_file_path?: string;
  hdfs_site_path?: string;
  disable_cache?: boolean;
  s3_credentials_provider?: string;
  size_available: boolean;
  total_size_bytes: number;
  cleanup_supported: boolean;
  warning?: string;
  nodes?: RuntimeStorageNodeStat[];
}

export interface RuntimeStorageDetails {
  cluster_id: number;
  config_source?: string;
  checkpoint?: RuntimeStorageSpec;
  imap?: RuntimeStorageSpec;
  configured_checkpoint?: RuntimeStorageSpec;
  configured_imap?: RuntimeStorageSpec;
}

export interface ApplyRuntimeStorageVersion {
  config_type: string;
  config_id: number;
  version: number;
}

export interface ApplyRuntimeStorageResult {
  saved: boolean;
  restart_required: boolean;
  message: string;
  versions?: ApplyRuntimeStorageVersion[];
  validation?: RuntimeStorageValidationResult;
}

export interface RuntimeStorageCleanupNodeResult {
  host_id: number;
  host_name: string;
  success: boolean;
  message: string;
}

export interface RuntimeStorageCleanupResult {
  cluster_id: number;
  success: boolean;
  message: string;
  nodes?: RuntimeStorageCleanupNodeResult[];
}

export interface RuntimeStorageValidationHostResult {
  host_id: number;
  host_name?: string;
  success: boolean;
  message: string;
  details?: Record<string, string>;
}

export interface RuntimeStorageValidationResult {
  success: boolean;
  kind: 'checkpoint' | 'imap';
  warning?: string;
  hosts?: RuntimeStorageValidationHostResult[];
}

export interface RuntimeStorageListItem {
  path?: string;
  name?: string;
  directory?: boolean;
  size_bytes?: number;
  modified_at?: string;
}

export interface RuntimeStorageListResult {
  cluster_id: number;
  kind: 'checkpoint' | 'imap' | string;
  path?: string;
  items?: RuntimeStorageListItem[];
}

export interface RuntimeStoragePreviewResult {
  cluster_id: number;
  kind: 'checkpoint' | 'imap' | string;
  storage_type?: string;
  path?: string;
  file_name?: string;
  size_bytes?: number;
  truncated?: boolean;
  binary?: boolean;
  encoding?: string;
  text_preview?: string;
  hex_preview?: string;
}

export interface RuntimeStorageCheckpointInspectResult {
  cluster_id: number;
  path?: string;
  file_name?: string;
  storage_type?: string;
  size_bytes?: number;
  truncated?: boolean;
  binary?: boolean;
  encoding?: string;
  text_preview?: string;
  hex_preview?: string;
  pipeline_state?: Record<string, unknown>;
  completed_checkpoint?: Record<string, unknown>;
  action_states?: Record<string, unknown>[];
  task_statistics?: Record<string, unknown>[];
  source_state_inspect?: RuntimeStorageCheckpointSourceStateInspectResult;
}

export interface RuntimeStorageCheckpointInspectJobConfig {
  content: string;
  content_format?: string;
  variables?: Record<string, unknown>;
}

export interface RuntimeStorageCheckpointSourceStateInspectResult {
  pipeline_state?: Record<string, unknown>;
  completed_checkpoint?: Record<string, unknown>;
  sources?: Record<string, unknown>[];
  unsupported_sources?: Record<string, unknown>[];
  warnings?: string[];
  error_message?: string;
}

export interface RuntimeStorageIMAPInspectResult {
  cluster_id: number;
  path?: string;
  file_name?: string;
  storage_type?: string;
  size_bytes?: number;
  truncated?: boolean;
  binary?: boolean;
  encoding?: string;
  text_preview?: string;
  hex_preview?: string;
  entry_count?: number;
  entries?: Record<string, unknown>[];
}

export interface StxJavaProxyStatus {
  cluster_id: number;
  cluster_name?: string;
  node_id?: number;
  host_id?: number;
  host_name?: string;
  host_ip?: string;
  role?: string;
  install_dir?: string;
  version?: string;
  service?: string;
  managed: boolean;
  running: boolean;
  healthy: boolean;
  endpoint?: string;
  port?: number;
  pid?: number;
  log_path?: string;
  message?: string;
}

export interface StxJavaProxyLogPreviewResult {
  cluster_id: number;
  log_path?: string;
  lines?: number;
  logs?: string;
}

/**
 * Node operation result
 * 节点操作结果
 */
export interface NodeOperationResult {
  /** Node ID / 节点 ID */
  node_id: number;
  /** Host ID / 主机 ID */
  host_id: number;
  /** Host name / 主机名称 */
  host_name: string;
  /** Whether the operation succeeded / 操作是否成功 */
  success: boolean;
  /** Result message / 结果消息 */
  message: string;
}

/**
 * Cluster operation result
 * 集群操作结果
 */
export interface OperationResult {
  /** Cluster ID / 集群 ID */
  cluster_id: number;
  /** Operation type / 操作类型 */
  operation: OperationType;
  /** Whether the operation succeeded / 操作是否成功 */
  success: boolean;
  /** Result message / 结果消息 */
  message: string;
  /** Node operation results / 节点操作结果 */
  node_results: NodeOperationResult[];
}

/**
 * Request to create a new cluster
 * 创建新集群的请求
 */
export interface CreateClusterRequest {
  /** Cluster name (required) / 集群名称（必填） */
  name: string;
  /** Description / 描述 */
  description?: string;
  /** Deployment mode (required) / 部署模式（必填） */
  deployment_mode: DeploymentMode;
  /** SeaTunnel version / SeaTunnel 版本 */
  version?: string;
  /** Installation directory / 安装目录 */
  install_dir?: string;
  /** Cluster configuration / 集群配置 */
  config?: ClusterConfig;
  /** Initial nodes to add (from discovery) / 初始节点（来自发现） */
  nodes?: Array<{
    host_id: number;
    install_dir: string;
    role: string;
    hazelcast_port?: number;
    api_port?: number;
  }>;
}

/**
 * Request to update an existing cluster
 * 更新现有集群的请求
 */
export interface UpdateClusterRequest {
  /** Cluster name / 集群名称 */
  name?: string;
  /** Description / 描述 */
  description?: string;
  /** SeaTunnel version / SeaTunnel 版本 */
  version?: string;
  /** Installation directory / 安装目录 */
  install_dir?: string;
  /** Cluster configuration / 集群配置 */
  config?: ClusterConfig;
}

/**
 * Request to add a node to a cluster
 * 向集群添加节点的请求
 */
export interface AddNodeRequest {
  /** Host ID (required) / 主机 ID（必填） */
  host_id: number;
  /** Node role (required) / 节点角色（必填） */
  role: NodeRole;
  /** SeaTunnel installation directory / SeaTunnel 安装目录 */
  install_dir?: string;
  /** Hazelcast cluster port / Hazelcast 集群端口 */
  hazelcast_port?: number;
  /** REST API port (Master only, optional) / REST API 端口（仅 Master，可选） */
  api_port?: number;
  /** Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式） */
  worker_port?: number;
  /** Node-level JSON overrides / 节点级 JSON 覆盖配置 */
  overrides?: NodeOverrides;
  /** Whether to skip precheck / 是否跳过预检查 */
  skip_precheck?: boolean;
}

/**
 * One logical node entry under the same host
 * 同一主机下的一个逻辑节点条目
 */
export interface AddNodeEntryRequest {
  /** Node role / 节点角色 */
  role: NodeRole;
  /** Installation directory override / 安装目录覆盖 */
  install_dir?: string;
  /** Hazelcast port / Hazelcast 端口 */
  hazelcast_port?: number;
  /** API port / API 端口 */
  api_port?: number;
  /** Worker port / Worker 端口 */
  worker_port?: number;
  /** Node-level JSON overrides / 节点级 JSON 覆盖配置 */
  overrides?: NodeOverrides;
}

/**
 * Batch add-node request for the same host
 * 同一主机批量添加节点请求
 */
export interface AddNodesRequest {
  /** Host ID / 主机 ID */
  host_id: number;
  /** Shared installation directory / 共享安装目录 */
  install_dir?: string;
  /** Logical node entries / 逻辑节点条目 */
  entries: AddNodeEntryRequest[];
  /** Whether to skip precheck / 是否跳过预检查 */
  skip_precheck?: boolean;
}

/**
 * Request to update a node in a cluster
 * 更新集群中节点的请求
 */
export interface UpdateNodeRequest {
  /** SeaTunnel installation directory / SeaTunnel 安装目录 */
  install_dir?: string;
  /** Hazelcast cluster port / Hazelcast 集群端口 */
  hazelcast_port?: number;
  /** REST API port (Master only, optional) / REST API 端口（仅 Master，可选） */
  api_port?: number;
  /** Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式） */
  worker_port?: number;
  /** Node-level JSON overrides / 节点级 JSON 覆盖配置 */
  overrides?: NodeOverrides;
}

/**
 * Default port values for different node roles
 * 不同节点角色的默认端口值
 */
export const DefaultPorts = {
  /** Master hazelcast port / Master Hazelcast 端口 */
  MASTER_HAZELCAST: 5801,
  /** Master REST API port / Master REST API 端口 */
  MASTER_API: 8080,
  /** Worker hazelcast port / Worker Hazelcast 端口 */
  WORKER_HAZELCAST: 5802,
};

const JVM_OVERRIDE_KEYS: Record<NodeRole, keyof NodeJVMOverrides> = {
  [NodeRole.MASTER]: 'master_heap_size',
  [NodeRole.WORKER]: 'worker_heap_size',
  [NodeRole.MASTER_WORKER]: 'hybrid_heap_size',
};

function toNumberOrUndefined(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function toPositivePortOrUndefined(value: unknown): number | undefined {
  const parsed = toNumberOrUndefined(value);
  if (parsed === undefined) {
    return undefined;
  }
  return parsed > 0 ? parsed : undefined;
}

/**
 * Get cluster-level JVM defaults from generic cluster config
 * 从通用 cluster config 中提取集群 JVM 默认值
 */
export function getClusterJVMConfig(
  config?: ClusterConfig | null,
): ClusterJVMConfig | undefined {
  const raw = config?.jvm;
  if (!raw || typeof raw !== 'object') {
    return undefined;
  }
  const jvm = raw as Record<string, unknown>;
  const result: ClusterJVMConfig = {
    hybrid_heap_size: toNumberOrUndefined(jvm.hybrid_heap_size),
    master_heap_size: toNumberOrUndefined(jvm.master_heap_size),
    worker_heap_size: toNumberOrUndefined(jvm.worker_heap_size),
  };
  if (
    result.hybrid_heap_size === undefined &&
    result.master_heap_size === undefined &&
    result.worker_heap_size === undefined
  ) {
    return undefined;
  }
  return result;
}

/**
 * Get cluster-level port defaults from generic cluster config
 * 从通用 cluster config 中提取集群端口默认值
 */
export function getClusterPortConfig(
  config?: ClusterConfig | null,
): ClusterPortConfig | undefined {
  const nestedPorts =
    config?.ports && typeof config.ports === 'object'
      ? (config.ports as Record<string, unknown>)
      : undefined;
  const fallbackRoot =
    config && typeof config === 'object'
      ? (config as Record<string, unknown>)
      : undefined;
  const ports = nestedPorts ?? fallbackRoot;

  if (!ports) {
    return undefined;
  }

  const result: ClusterPortConfig = {
    master_hazelcast_port: toPositivePortOrUndefined(
      ports.master_hazelcast_port,
    ),
    master_api_port: toPositivePortOrUndefined(ports.master_api_port),
    worker_port: toPositivePortOrUndefined(ports.worker_port),
    java_proxy_port: toPositivePortOrUndefined(ports.java_proxy_port),
  };

  if (
    result.master_hazelcast_port === undefined &&
    result.master_api_port === undefined &&
    result.worker_port === undefined &&
    result.java_proxy_port === undefined
  ) {
    return undefined;
  }

  return result;
}

/**
 * Read cluster default ports for a role
 * 读取某角色的集群默认端口值
 */
export function getClusterPortDefaultsForRole(
  role: NodeRole,
  config?: ClusterConfig | null,
): {
  hazelcastPort?: number;
  apiPort?: number;
  workerPort?: number;
} {
  const ports = getClusterPortConfig(config);

  if (role === NodeRole.WORKER) {
    return {
      hazelcastPort: ports?.worker_port,
    };
  }

  if (role === NodeRole.MASTER_WORKER) {
    return {
      hazelcastPort: ports?.master_hazelcast_port,
      apiPort: ports?.master_api_port,
      workerPort: ports?.worker_port,
    };
  }

  return {
    hazelcastPort: ports?.master_hazelcast_port,
    apiPort: ports?.master_api_port,
  };
}

/**
 * Get the JVM override key for a role
 * 获取角色对应的 JVM override key
 */
export function getNodeJVMOverrideKey(role: NodeRole): keyof NodeJVMOverrides {
  return JVM_OVERRIDE_KEYS[role];
}

/**
 * Read the current node-level JVM override value for a role
 * 读取某角色当前节点级 JVM override 值
 */
export function getNodeJVMOverrideValue(
  role: NodeRole,
  overrides?: NodeOverrides | null,
): number | undefined {
  const key = getNodeJVMOverrideKey(role);
  return overrides?.jvm?.[key];
}

/**
 * Read the cluster default JVM value for a role
 * 读取某角色的集群默认 JVM 值
 */
export function getClusterJVMValueForRole(
  role: NodeRole,
  config?: ClusterConfig | null,
): number | undefined {
  const key = getNodeJVMOverrideKey(role);
  return getClusterJVMConfig(config)?.[key];
}

/**
 * Build a role-specific node JVM override payload
 * 构建角色对应的节点 JVM override 载荷
 */
export function buildNodeJVMOverride(
  role: NodeRole,
  heapSize?: number,
): NodeOverrides | undefined {
  if (!heapSize || heapSize <= 0) {
    return undefined;
  }
  const key = getNodeJVMOverrideKey(role);
  const jvm: NodeJVMOverrides = {};
  jvm[key] = heapSize;
  return {
    jvm,
  };
}

/**
 * Request parameters for listing clusters
 * 获取集群列表的请求参数
 */
export interface ListClustersRequest {
  /** Current page number (1-based) / 当前页码（从 1 开始） */
  current: number;
  /** Page size / 每页数量 */
  size: number;
  /** Filter by name / 按名称过滤 */
  name?: string;
  /** Filter by status / 按状态过滤 */
  status?: ClusterStatus;
  /** Filter by deployment mode / 按部署模式过滤 */
  deployment_mode?: DeploymentMode;
}

/**
 * Cluster list data
 * 集群列表数据
 */
export interface ClusterListData {
  /** Total count / 总数量 */
  total: number;
  /** Cluster list / 集群列表 */
  clusters: ClusterInfo[];
}

/**
 * Backend response structure
 * 后端响应结构
 */
export interface BackendResponse<T = unknown> {
  /** Error message, empty string means no error / 错误信息，空字符串表示无错误 */
  error_msg: string;
  /** Response data / 响应数据 */
  data: T;
}

// ==================== Response Types 响应类型 ====================

/** List clusters response type / 获取集群列表响应类型 */
export type ListClustersResponse = BackendResponse<ClusterListData>;

/** Create cluster response type / 创建集群响应类型 */
export type CreateClusterResponse = BackendResponse<ClusterInfo>;

/** Get cluster response type / 获取集群详情响应类型 */
export type GetClusterResponse = BackendResponse<ClusterInfo>;

/** Update cluster response type / 更新集群响应类型 */
export type UpdateClusterResponse = BackendResponse<ClusterInfo>;

/** Delete cluster response type / 删除集群响应类型 */
export type DeleteClusterResponse = BackendResponse<null>;

/** Get nodes response type / 获取节点列表响应类型 */
export type GetNodesResponse = BackendResponse<NodeInfo[]>;

/** Add node response type / 添加节点响应类型 */
export type AddNodeResponse = BackendResponse<NodeInfo>;

/** Add nodes response type / 批量添加节点响应类型 */
export type AddNodesResponse = BackendResponse<NodeInfo[]>;

/** Remove node response type / 移除节点响应类型 */
export type RemoveNodeResponse = BackendResponse<null>;

/** Cluster operation response type / 集群操作响应类型 */
export type ClusterOperationResponse = BackendResponse<OperationResult>;

/** Get cluster status response type / 获取集群状态响应类型 */
export type GetClusterStatusResponse = BackendResponse<ClusterStatusInfo>;

/** Runtime storage details response type / 运行时存储详情响应类型 */
export type GetRuntimeStorageResponse = BackendResponse<RuntimeStorageDetails>;

/** Runtime storage cleanup response type / 运行时存储清理响应类型 */
export type CleanupRuntimeStorageResponse =
  BackendResponse<RuntimeStorageCleanupResult>;

/** Runtime storage validate response type / 运行时存储连通性校验响应类型 */
export type ValidateRuntimeStorageResponse =
  BackendResponse<RuntimeStorageValidationResult>;
export type ListRuntimeStorageResponse =
  BackendResponse<RuntimeStorageListResult>;
export type PreviewRuntimeStorageResponse =
  BackendResponse<RuntimeStoragePreviewResult>;
export type InspectCheckpointRuntimeStorageResponse =
  BackendResponse<RuntimeStorageCheckpointInspectResult>;
export type InspectIMAPRuntimeStorageResponse =
  BackendResponse<RuntimeStorageIMAPInspectResult>;

/** STX Java Proxy 响应类型 / STX Java Proxy response type. */
export type StxJavaProxyResponse = BackendResponse<StxJavaProxyStatus>;
export type StxJavaProxyLogPreviewResponse =
  BackendResponse<StxJavaProxyLogPreviewResult>;

/**
 * Request to precheck a node before adding
 * 添加节点前的预检查请求
 */
export interface PrecheckRequest {
  /** Host ID (required) / 主机 ID（必填） */
  host_id: number;
  /** Node role (required) / 节点角色（必填） */
  role: NodeRole;
  /** SeaTunnel installation directory / SeaTunnel 安装目录 */
  install_dir?: string;
  /** Hazelcast cluster port (required) / Hazelcast 集群端口（必填） */
  hazelcast_port: number;
  /** REST API port (Master only, optional) / REST API 端口（仅 Master，可选） */
  api_port?: number;
  /** Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式） */
  worker_port?: number;
}

/**
 * Precheck check item
 * 预检查检查项
 */
export interface PrecheckCheckItem {
  /** Check name / 检查名称 */
  name: string;
  /** Check status: passed, failed, skipped / 检查状态：通过、失败、跳过 */
  status: 'passed' | 'failed' | 'skipped';
  /** Detail message / 详细信息 */
  message: string;
}

/**
 * Precheck result
 * 预检查结果
 */
export interface PrecheckResult {
  /** Whether all checks passed / 是否所有检查都通过 */
  success: boolean;
  /** Overall message / 总体消息 */
  message: string;
  /** Individual check items / 各个检查项 */
  checks: PrecheckCheckItem[];
}

/** Precheck node response type / 节点预检查响应类型 */
export type PrecheckNodeResponse = BackendResponse<PrecheckResult>;

/** Update node response type / 更新节点响应类型 */
export type UpdateNodeResponse = BackendResponse<NodeInfo>;
