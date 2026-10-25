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
 * SeaTunnel Plugin Marketplace Types
 * SeaTunnel 插件市场类型定义
 */

// ==================== Enums 枚举 ====================

/**
 * Plugin category
 * 插件分类
 */
export type PluginCategory = 'source' | 'sink' | 'connector' | 'transform';

/**
 * Plugin status
 * 插件状态
 */
export type PluginStatus = 'available' | 'installed' | 'enabled' | 'disabled' | 'partial';

/**
 * Per-node plugin install result
 * 单节点插件安装结果
 */
export interface PluginNodeInstallResult {
  node_id: number;
  host_id: number;
  status: 'installed' | 'failed' | 'skipped' | 'rolled_back' | 'rollback_failed' | string;
  message?: string;
}

/**
 * Format per-node statuses into a short human readable line.
 * 将逐节点状态格式化为简短说明。
 */
export function formatPluginNodeStatuses(
  plugin: {node_statuses?: PluginNodeInstallResult[]} | null | undefined,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  return (plugin?.node_statuses ?? [])
    .map((item) => `${t('plugin.nodeLabel', {id: item.node_id})}: ${t(`plugin.nodeStatus.${item.status}`)}`)
    .join('; ');
}

/**
 * Mirror source for downloading plugins
 * 下载插件的镜像源
 */
export type MirrorSource = 'apache' | 'aliyun' | 'huaweicloud';

/**
 * Available plugin list source
 * 可用插件列表来源
 */
export type PluginListSource = 'database' | 'remote' | 'seed';

/**
 * Official dependency status
 * 官方依赖状态
 */
export type PluginDependencyStatus =
  | 'ready_exact'
  | 'ready_fallback'
  | 'runtime_analyzed'
  | 'not_required'
  | 'unknown';

/**
 * Dependency resolution mode
 * 依赖解析模式
 */
export type DependencyResolutionMode =
  | 'exact'
  | 'fallback'
  | 'runtime'
  | 'none';

/**
 * Dependency source type
 * 依赖来源类型
 */
export type PluginDependencySource = 'maven' | 'upload' | 'official';

// ==================== Plugin Types 插件类型 ====================

/**
 * Plugin dependency
 * 插件依赖项
 */
export interface PluginDependency {
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string; // connectors/ or lib/ or plugins/<mapping>
  source_type?: PluginDependencySource;
  original_file_name?: string;
}

/**
 * Plugin information
 * 插件信息
 */
export interface Plugin {
  name: string;
  display_name: string;
  category: PluginCategory;
  version: string;
  description: string;
  group_id: string;
  artifact_id: string;
  dependencies?: PluginDependency[];
  icon?: string;
  doc_url?: string;
  dependency_status?: PluginDependencyStatus;
  dependency_count?: number;
  dependency_baseline_version?: string;
  dependency_resolution_mode?: DependencyResolutionMode;
}

/**
 * Installed plugin on a cluster
 * 集群上已安装的插件
 * Note: Plugins are managed at cluster level, not host level.
 * 注意：插件在集群级别管理，而非主机级别。
 */
export interface InstalledPlugin {
  id: number;
  cluster_id: number;
  plugin_name: string;
  artifact_id?: string; // Maven artifact ID (e.g., connector-cdc-mysql)
  category: PluginCategory;
  version: string;
  status: PluginStatus;
  install_path: string;
  installed_at: string;
  updated_at: string;
  installed_by?: number;
  selected_profile_keys?: string[];
  attached_connectors?: string[];
  dependencies?: PluginDependency[];
  /** Per-node states for partial installs / 部分安装时的逐节点状态 */
  node_statuses?: PluginNodeInstallResult[];
}

// ==================== Request Types 请求类型 ====================

/**
 * Install plugin request
 * 安装插件请求
 */
export interface InstallPluginRequest {
  plugin_name: string;
  version: string;
  mirror?: MirrorSource;
  profile_keys?: string[];
}

/**
 * Plugin filter options
 * 插件过滤选项
 */
export interface PluginFilter {
  cluster_id?: number;
  category?: PluginCategory;
  status?: PluginStatus;
  keyword?: string;
  page?: number;
  page_size?: number;
}

// ==================== Response Types 响应类型 ====================

/**
 * Available plugins response
 * 可用插件响应
 */
export interface AvailablePluginsResponse {
  plugins: Plugin[];
  total: number;
  version: string;
  mirror: string;
  source: PluginListSource;
  cache_hit: boolean;
  catalog_source_mirror?: string;
  catalog_refreshed_at?: string;
}

/**
 * List available plugins API response
 * 获取可用插件列表 API 响应
 */
export interface ListPluginsResponse {
  error_msg: string;
  data: AvailablePluginsResponse | null;
}

/**
 * Get plugin info API response
 * 获取插件详情 API 响应
 */
export interface GetPluginInfoResponse {
  error_msg: string;
  data: Plugin | null;
}

/**
 * List installed plugins API response
 * 获取已安装插件列表 API 响应
 */
export interface ListInstalledPluginsResponse {
  error_msg: string;
  data: InstalledPlugin[] | null;
}

/**
 * Install plugin API response
 * 安装插件 API 响应
 */
export interface InstallPluginResponse {
  error_msg: string;
  data: PluginInstallStatus | null;
}

/**
 * Uninstall plugin API response
 * 卸载插件 API 响应
 */
export interface UninstallPluginResponse {
  error_msg: string;
  data: unknown;
}

/**
 * Enable/Disable plugin API response
 * 启用/禁用插件 API 响应
 */
export interface EnableDisablePluginResponse {
  error_msg: string;
  data: InstalledPlugin | null;
}

/**
 * Plugin installation status
 * 插件安装状态
 */
export interface PluginInstallStatus {
  task_id?: string;
  cluster_id?: number;
  plugin_name: string;
  version?: string;
  status: string; // pending|downloading|installing|completed|failed
  progress: number;
  message?: string;
  error?: string;
  started_at?: string;
  /** Some nodes still have the plugin / 部分节点仍保留插件 */
  partial?: boolean;
  /** Per-node results / 逐节点结果 */
  node_results?: PluginNodeInstallResult[];
}

// ==================== Plugin Download Types 插件下载类型 ====================

/**
 * Plugin download progress
 * 插件下载进度
 */
export interface PluginDownloadProgress {
  plugin_name: string;
  version: string;
  status: 'not_started' | 'downloading' | 'completed' | 'failed';
  progress: number;
  current_step?: string;
  current_artifact?: string;
  current_artifact_kind?: 'connector' | 'dependency' | string;
  downloaded_bytes?: number;
  total_bytes?: number;
  speed?: number;
  message?: string;
  error?: string;
  start_time?: string;
  end_time?: string;
  connector_count?: number;
  connector_completed?: number;
  dependency_count?: number;
  dependency_completed?: number;
  selected_profile_keys?: string[];
}

/**
 * Download all plugins progress
 * 下载所有插件的进度
 */
export interface DownloadAllPluginsProgress {
  total: number; // 总插件数 / Total plugins
  downloaded: number; // 已下载数 / Downloaded count
  failed: number; // 失败数 / Failed count
  skipped: number; // 跳过数（已存在）/ Skipped count
  status: string; // 状态 / Status
  message: string; // 消息 / Message
}

/**
 * Local plugin (downloaded to Control Plane)
 * 本地插件（已下载到 Control Plane）
 */
export interface LocalPlugin {
  name: string;
  artifact_id?: string;
  version: string;
  category: PluginCategory;
  connector_path: string;
  size: number;
  downloaded_at: string;
  selected_profile_keys?: string[];
  attached_connectors?: string[];
  dependencies?: PluginDependency[];
}

/**
 * Download plugin request
 * 下载插件请求
 */
export interface DownloadPluginRequest {
  version: string;
  mirror?: MirrorSource;
  profile_keys?: string[];
}

export interface DownloadAllPluginsRequest {
  version: string;
  mirror?: MirrorSource;
  selected_plugin_profiles?: Record<string, string[]>;
}

/**
 * Download plugin response
 * 下载插件响应
 */
export interface DownloadPluginResponse {
  error_msg: string;
  data: PluginDownloadProgress | null;
}

/**
 * List local plugins response
 * 获取本地插件列表响应
 */
export interface ListLocalPluginsResponse {
  error_msg: string;
  data: LocalPlugin[] | null;
}

/**
 * Get install progress response
 * 获取安装进度响应
 */
export interface GetInstallProgressResponse {
  error_msg: string;
  data: PluginInstallStatus | null;
}

// ==================== Plugin Dependency Config Types 插件依赖配置类型 ====================

/**
 * Plugin dependency configuration (user-configured)
 * 插件依赖配置（用户配置）
 */
export interface PluginDependencyConfig {
  id: number;
  plugin_name: string;
  seatunnel_version: string;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
  source_type: PluginDependencySource;
  original_file_name?: string;
  file_size?: number;
  checksum?: string;
  created_at: string;
  updated_at: string;
}

export interface PluginDependencyDisable {
  id: number;
  plugin_name: string;
  seatunnel_version: string;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
  created_at: string;
  updated_at: string;
}

/**
 * Official dependency profile item
 * 官方依赖画像条目
 */
export interface PluginDependencyProfileItem {
  id: number;
  profile_id: number;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
  required: boolean;
  source_url: string;
  note: string;
  disabled?: boolean;
  disable_id?: number;
  created_at: string;
  updated_at: string;
}

/**
 * Official dependency profile
 * 官方依赖画像
 */
export interface PluginDependencyProfile {
  id: number;
  seatunnel_version: string;
  plugin_name: string;
  artifact_id: string;
  profile_key: string;
  profile_name: string;
  engine_scope: string;
  target_dir: string;
  applies_to: string;
  include_versions?: string;
  excluded_versions?: string;
  source_kind: string;
  baseline_version_used?: string;
  doc_slug: string;
  doc_source_url: string;
  confidence: string;
  no_additional_dependencies: boolean;
  is_default: boolean;
  note: string;
  content_hash: string;
  created_at: string;
  updated_at: string;
  items?: PluginDependencyProfileItem[];
}

/**
 * Official dependency resolution response
 * 官方依赖解析响应
 */
export interface OfficialDependenciesResponse {
  plugin_name: string;
  seatunnel_version: string;
  dependency_status: PluginDependencyStatus;
  dependency_count: number;
  baseline_version_used?: string;
  dependency_resolution_mode: DependencyResolutionMode;
  profiles: PluginDependencyProfile[];
  effective_dependencies: PluginDependency[];
  disabled_dependencies?: PluginDependencyDisable[];
}

/**
 * Analyze official dependencies request
 * 在线分析官方依赖请求
 */
export interface AnalyzeOfficialDependenciesRequest {
  version: string;
  profile_key?: string;
  force_refresh?: boolean;
}

/**
 * Add dependency request
 * 添加依赖请求
 */
export interface AddDependencyRequest {
  group_id: string;
  artifact_id: string;
  version: string; // 必填 / Required
  seatunnel_version?: string;
  target_dir?: string;
}

export interface UploadDependencyRequest {
  file: File;
  seatunnel_version?: string;
  group_id?: string;
  artifact_id?: string;
  version?: string;
  target_dir?: string;
}

export interface DisableDependencyRequest {
  seatunnel_version?: string;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
}

/**
 * List dependencies response
 * 获取依赖列表响应
 */
export interface ListDependenciesResponse {
  error_msg: string;
  data: PluginDependencyConfig[] | null;
}

/**
 * Add dependency response
 * 添加依赖响应
 */
export interface AddDependencyResponse {
  error_msg: string;
  data: PluginDependencyConfig | null;
}

export interface DisableDependencyResponse {
  error_msg: string;
  data: PluginDependencyDisable | null;
}
