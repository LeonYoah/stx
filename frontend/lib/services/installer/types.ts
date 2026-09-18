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
 * SeaTunnel Installer Types
 * SeaTunnel 安装管理类型定义
 */

// ==================== Enums 枚举 ====================

/**
 * Mirror source for downloading SeaTunnel packages
 * 下载 SeaTunnel 安装包的镜像源
 */
export type MirrorSource = 'aliyun' | 'apache' | 'huaweicloud';

/**
 * Installation mode
 * 安装模式
 */
export type InstallMode = 'online' | 'offline';

/**
 * Deployment mode for SeaTunnel cluster
 * SeaTunnel 集群部署模式
 */
export type DeploymentMode = 'hybrid' | 'separated';

/**
 * Node role in deployment
 * 部署中的节点角色
 */
export type NodeRole = 'master' | 'worker' | 'master/worker';

/**
 * Installation step status
 * 安装步骤状态
 */
export type StepStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'skipped';

/**
 * Installation step names
 * 安装步骤名称
 */
export type InstallStep =
  | 'download'
  | 'verify'
  | 'extract'
  | 'configure_cluster'
  | 'configure_checkpoint'
  | 'configure_imap'
  | 'configure_jvm'
  | 'install_plugins'
  | 'register_cluster'
  | 'complete';

/**
 * Precheck item status
 * 预检查项状态
 */
export type CheckStatus = 'passed' | 'failed' | 'warning';

/**
 * Checkpoint storage type
 * 检查点存储类型
 */
export type CheckpointStorageType = 'LOCAL_FILE' | 'HDFS' | 'OSS' | 'S3';

/**
 * IMAP persistence storage type
 * IMAP 持久化存储类型
 */
export type IMAPStorageType = 'DISABLED' | 'LOCAL_FILE' | 'HDFS' | 'OSS' | 'S3';

/**
 * Job schedule strategy for static slot mode
 * 静态 slot 模式的作业调度策略
 */
export type JobScheduleStrategy = 'WAIT' | 'REJECT';
export type SlotAllocationStrategy = 'RANDOM' | 'SYSTEM_LOAD' | 'SLOT_RATIO';
export type JobLogMode = 'mixed' | 'per_job';

// ==================== Package Types 安装包类型 ====================

/**
 * Package information
 * 安装包信息
 */
export interface PackageInfo {
  version: string;
  file_name: string;
  file_size: number;
  checksum?: string;
  download_urls: Record<MirrorSource, string>;
  is_local: boolean;
  local_path?: string;
  uploaded_at?: string;
}

/**
 * Available versions response
 * 可用版本响应
 */
export interface AvailableVersions {
  versions: string[];
  recommended_version: string;
  local_packages: PackageInfo[];
  version_capabilities: Record<string, SeaTunnelVersionCapabilities>;
}

/**
 * Version-aware runtime capability metadata
 * 版本感知运行时能力元数据
 */
export interface SeaTunnelVersionCapabilities {
  supports_dynamic_slot: boolean;
  supports_slot_num: boolean;
  supports_history_job_expire_minutes: boolean;
  supports_scheduled_deletion_enable: boolean;
  supports_job_schedule_strategy: boolean;
  supports_slot_allocation_strategy: boolean;
  supports_http_service: boolean;
  supports_job_log_mode: boolean;
  default_dynamic_slot: boolean;
  default_static_slot_num: number;
  default_history_job_expire_minutes: number;
  default_scheduled_deletion_enable: boolean;
  default_job_schedule_strategy: JobScheduleStrategy;
  default_slot_allocation_strategy: SlotAllocationStrategy;
  default_http_enabled: boolean;
  default_job_log_mode: JobLogMode;
}

/**
 * Advanced runtime configuration for SeaTunnel engine
 * SeaTunnel 引擎高级运行时配置
 */
export interface RuntimeEngineConfig {
  dynamic_slot: boolean;
  slot_num: number;
  slot_allocation_strategy: SlotAllocationStrategy;
  job_schedule_strategy: JobScheduleStrategy;
  history_job_expire_minutes: number;
  scheduled_deletion_enable: boolean;
  enable_http: boolean;
  job_log_mode: JobLogMode;
}

// ==================== Configuration Types 配置类型 ====================

/**
 * JVM memory configuration (all sizes in GB)
 * JVM 内存配置（所有大小单位为 GB）
 */
export interface JVMConfig {
  /** Heap size for hybrid mode in GB / 混合模式堆大小（GB） */
  hybrid_heap_size: number;
  /** Heap size for master nodes in GB / Master 节点堆大小（GB） */
  master_heap_size: number;
  /** Heap size for worker nodes in GB / Worker 节点堆大小（GB） */
  worker_heap_size: number;
}

/**
 * Checkpoint storage configuration
 * 检查点存储配置
 */
export interface CheckpointConfig {
  storage_type: CheckpointStorageType;
  namespace: string;
  // HDFS configuration / HDFS 配置
  hdfs_namenode_host?: string;
  hdfs_namenode_port?: number;
  // HDFS Kerberos authentication / HDFS Kerberos 认证
  kerberos_principal?: string;
  kerberos_keytab_file_path?: string;
  // HDFS HA mode configuration / HDFS HA 模式配置
  hdfs_ha_enabled?: boolean;
  hdfs_name_services?: string; // e.g., "usdp-bing"
  hdfs_ha_namenodes?: string; // e.g., "nn1,nn2"
  hdfs_namenode_rpc_address_1?: string; // e.g., "usdp-bing-nn1:8020"
  hdfs_namenode_rpc_address_2?: string; // e.g., "usdp-bing-nn2:8020"
  hdfs_failover_proxy_provider?: string; // default: org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider
  // OSS/S3 configuration / OSS/S3 配置
  storage_endpoint?: string;
  storage_access_key?: string;
  storage_secret_key?: string;
  storage_bucket?: string;
}

/**
 * IMAP persistence configuration
 * IMAP 持久化配置
 */
export interface IMAPConfig {
  storage_type: IMAPStorageType;
  namespace: string;
  hdfs_namenode_host?: string;
  hdfs_namenode_port?: number;
  kerberos_principal?: string;
  kerberos_keytab_file_path?: string;
  hdfs_ha_enabled?: boolean;
  hdfs_name_services?: string;
  hdfs_ha_namenodes?: string;
  hdfs_namenode_rpc_address_1?: string;
  hdfs_namenode_rpc_address_2?: string;
  hdfs_failover_proxy_provider?: string;
  storage_endpoint?: string;
  storage_access_key?: string;
  storage_secret_key?: string;
  storage_bucket?: string;
}

/**
 * Plugin installation info for tracking plugin installation during SeaTunnel setup
 * 插件安装信息，用于跟踪 SeaTunnel 安装过程中的插件安装
 */
export interface PluginInstallInfo {
  name: string; // 插件名称 / Plugin name
  category: string; // 插件分类 (source/sink/transform) / Plugin category
  version: string; // 插件版本 / Plugin version
  status: 'pending' | 'downloading' | 'installing' | 'completed' | 'failed'; // 安装状态 / Install status
  progress: number; // 安装进度 (0-100) / Install progress
  message?: string; // 状态消息 / Status message
  error?: string; // 错误信息 / Error message
}

/**
 * Connector installation configuration
 * 连接器安装配置
 */
export interface ConnectorConfig {
  install_connectors: boolean;
  connectors?: string[];
  plugin_repo?: MirrorSource;
  /** Selected plugins for installation / 选择安装的插件列表 */
  selected_plugins?: string[];
  /** Selected profile keys by plugin / 按插件记录选择的画像 */
  selected_plugin_profiles?: Record<string, string[]>;
}

// ==================== Installation Types 安装类型 ====================

/**
 * Installation request
 * 安装请求
 */
export interface InstallationRequest {
  host_id?: string;
  cluster_id?: string;
  version: string;
  install_dir?: string; // Installation directory / 安装目录
  install_mode: InstallMode;
  mirror?: MirrorSource;
  package_path?: string;
  deployment_mode: DeploymentMode;
  node_role: NodeRole;
  master_addresses?: string[]; // Master node addresses for cluster configuration / 集群配置的 master 节点地址
  worker_addresses?: string[]; // Worker node addresses for separated mode / 分离模式的 worker 节点地址
  cluster_port?: number; // Cluster communication port / 集群通信端口
  worker_port?: number; // Worker hazelcast port / Worker Hazelcast 端口
  http_port?: number; // HTTP API port / HTTP API 端口
  java_proxy_port?: number; // Managed stx-java-proxy listen port / 托管 stx-java-proxy 监听端口
  enable_http?: boolean; // Enable SeaTunnel HTTP/Web UI / 是否开启 SeaTunnel HTTP/Web UI
  dynamic_slot?: boolean;
  slot_num?: number;
  slot_allocation_strategy?: SlotAllocationStrategy;
  job_schedule_strategy?: JobScheduleStrategy;
  history_job_expire_minutes?: number;
  scheduled_deletion_enable?: boolean;
  job_log_mode?: JobLogMode;
  jvm?: JVMConfig;
  checkpoint?: CheckpointConfig;
  imap?: IMAPConfig;
  connector?: ConnectorConfig;
}

/**
 * Step information
 * 步骤信息
 */
export interface StepInfo {
  step: InstallStep;
  name: string;
  description: string;
  status: StepStatus;
  progress: number;
  message?: string;
  error?: string;
  start_time?: string;
  end_time?: string;
  retryable: boolean;
}

/**
 * Installation status
 * 安装状态
 */
export interface InstallationStatus {
  id: string;
  host_id: string;
  cluster_id?: string;
  status: StepStatus;
  current_step: InstallStep;
  steps: StepInfo[];
  progress: number;
  message?: string;
  error?: string;
  warnings?: string[];
  start_time: string;
  end_time?: string;
}

// ==================== Precheck Types 预检查类型 ====================

/**
 * Precheck request options
 * 预检查请求选项
 */
export interface PrecheckRequest {
  min_memory_mb?: number;
  min_cpu_cores?: number;
  min_disk_space_mb?: number;
  install_dir?: string;
  ports?: number[];
}

/**
 * Precheck item result
 * 预检查项结果
 */
export interface PrecheckItem {
  name: string;
  status: CheckStatus;
  message: string;
  details?: Record<string, unknown>;
}

/**
 * Precheck result
 * 预检查结果
 */
export interface PrecheckResult {
  items: PrecheckItem[];
  overall_status: CheckStatus;
  summary: string;
}

// ==================== API Response Types API 响应类型 ====================

/**
 * List packages response
 * 获取安装包列表响应
 */
export interface ListPackagesResponse {
  error_msg: string;
  data: AvailableVersions | null;
}

/**
 * Get package info response
 * 获取安装包信息响应
 */
export interface GetPackageInfoResponse {
  error_msg: string;
  data: PackageInfo | null;
}

/**
 * Upload package response
 * 上传安装包响应
 */
export interface UploadPackageResponse {
  error_msg: string;
  data: PackageInfo | null;
}

/**
 * Upload package chunk result
 * 上传安装包分片结果
 */
export interface UploadChunkResult {
  upload_id: string;
  completed: boolean;
  received_chunks: number;
  total_chunks: number;
  package?: PackageInfo;
}

/**
 * Upload package chunk response
 * 上传安装包分片响应
 */
export interface UploadChunkResponse {
  error_msg: string;
  data: UploadChunkResult | null;
}

/**
 * Delete package response
 * 删除安装包响应
 */
export interface DeletePackageResponse {
  error_msg: string;
  data: unknown;
}

/**
 * Precheck response
 * 预检查响应
 */
export interface PrecheckResponse {
  error_msg: string;
  data: PrecheckResult | null;
}

/**
 * Installation response
 * 安装响应
 */
export interface InstallResponse {
  error_msg: string;
  data: InstallationStatus | null;
}

// ==================== Download Types 下载类型 ====================

/**
 * Download status
 * 下载状态
 */
export type DownloadStatus =
  | 'pending'
  | 'downloading'
  | 'completed'
  | 'failed'
  | 'cancelled';

/**
 * Download task
 * 下载任务
 */
export interface DownloadTask {
  id: string;
  version: string;
  mirror: MirrorSource;
  download_url: string;
  status: DownloadStatus;
  progress: number;
  downloaded_bytes: number;
  total_bytes: number;
  speed: number;
  message?: string;
  error?: string;
  start_time: string;
  end_time?: string;
}

/**
 * Download request
 * 下载请求
 */
export interface DownloadRequest {
  version: string;
  mirror?: MirrorSource;
}

/**
 * Runtime storage validation kind
 * 运行时存储校验类型
 */
export type RuntimeStorageValidationKind = 'checkpoint' | 'imap';

/**
 * Runtime storage validation request
 * 运行时存储校验请求
 */
export interface RuntimeStorageValidationRequest {
  host_ids: number[];
  kind: RuntimeStorageValidationKind;
  checkpoint?: CheckpointConfig;
  imap?: IMAPConfig;
}

/**
 * Runtime storage validation result for one host
 * 单主机运行时存储校验结果
 */
export interface RuntimeStorageValidationHostResult {
  host_id: number;
  host_name?: string;
  success: boolean;
  message: string;
  details?: Record<string, string>;
}

/**
 * Runtime storage validation response data
 * 运行时存储校验响应数据
 */
export interface RuntimeStorageValidationResult {
  success: boolean;
  kind: RuntimeStorageValidationKind;
  warning?: string;
  hosts: RuntimeStorageValidationHostResult[];
}

export interface RuntimeStorageValidationResponse {
  error_msg: string;
  data: RuntimeStorageValidationResult | null;
}

/**
 * Download response
 * 下载响应
 */
export interface DownloadResponse {
  error_msg: string;
  data: DownloadTask | null;
}

/**
 * Download list response
 * 下载列表响应
 */
export interface DownloadListResponse {
  error_msg: string;
  data: DownloadTask[] | null;
}
