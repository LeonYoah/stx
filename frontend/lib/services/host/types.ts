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
 * Host Service Types
 * 主机服务类型定义
 *
 * This file defines all types related to host management.
 * 本文件定义所有与主机管理相关的类型。
 */

 

/**
 * Kubernetes host authentication mode
 * Kubernetes 主机认证方式
 */
export type K8sAuthMode = 'kubeconfig' | 'token';

/**
 * Kubernetes host access mode
 * Kubernetes 主机连接方式
 */
export type K8sAccessMode = 'in_cluster' | 'external';

export interface K8sKubeconfigContextSummary {
  name: string;
  cluster: string;
  user: string;
  namespace?: string;
  server: string;
  has_external_files: boolean;
  has_exec_plugin: boolean;
  has_loopback_server: boolean;
  credential_expires_at?: string | null;
  credential_kind: string;
}

export interface K8sKubeconfigInspectResult {
  valid: boolean;
  error?: string;
  current_context?: string;
  selected_context?: string;
  contexts: K8sKubeconfigContextSummary[];
  warnings?: string[];
}

/**
 * Host type enumeration
 * 主机类型枚举
 */
export enum HostType {
  /** Physical machine or VM managed by Agent / 由 Agent 管理的物理机或虚拟机 */
  BARE_METAL = 'bare_metal',
  /** Docker host managed via Docker API / 通过 Docker API 管理的 Docker 主机 */
  DOCKER = 'docker',
  /** Kubernetes cluster managed via K8s API / 通过 K8s API 管理的 Kubernetes 集群 */
  KUBERNETES = 'kubernetes',
}

/**
 * Host connection status
 * 主机连接状态
 */
export enum HostStatus {
  /** Waiting for connection / 等待连接 */
  PENDING = 'pending',
  /** Connected and operational / 已连接并可操作 */
  CONNECTED = 'connected',
  /** Offline / 离线 */
  OFFLINE = 'offline',
  /** Error state / 错误状态 */
  ERROR = 'error',
}

/**
 * Agent installation status (for bare_metal hosts)
 * Agent 安装状态（用于物理机/VM）
 */
export enum AgentStatus {
  /** Agent not installed / Agent 未安装 */
  NOT_INSTALLED = 'not_installed',
  /** Agent installed and connected / Agent 已安装并已连接 */
  INSTALLED = 'installed',
  /** Agent installed but offline / Agent 已安装但离线 */
  OFFLINE = 'offline',
}

/**
 * Host information returned from API
 * API 返回的主机信息
 */
export interface HostInfo {
  /** Host ID / 主机 ID */
  id: number;
  /** Host name / 主机名称 */
  name: string;
  /** Host type / 主机类型 */
  host_type: HostType;
  /** Description / 描述 */
  description: string;
  /** Connection status / 连接状态 */
  status: HostStatus;

  /** CPU usage percentage (0-100) / CPU 使用率百分比 */
  cpu_usage: number;
  /** Memory usage percentage (0-100) / 内存使用率百分比 */
  memory_usage: number;
  /** Disk usage percentage (0-100) / 磁盘使用率百分比 */
  disk_usage: number;
  /** Whether the host is online / 主机是否在线 */
  is_online: boolean;
  /** Last check time / 最后检查时间 */
  last_check: string | null;

  // bare_metal specific fields / 物理机专用字段
  /** IP address / IP 地址 */
  ip_address?: string;
  /** SSH port / SSH 端口 */
  ssh_port?: number;
  /** Agent ID / Agent ID */
  agent_id?: string;
  /** Agent status / Agent 状态 */
  agent_status?: AgentStatus;
  /** Agent version / Agent 版本 */
  agent_version?: string;
  /** Operating system type / 操作系统类型 */
  os_type?: string;
  /** CPU architecture / CPU 架构 */
  arch?: string;
  /** Number of CPU cores / CPU 核心数 */
  cpu_cores?: number;
  /** Total memory in bytes / 总内存（字节） */
  total_memory?: number;
  /** Total disk space in bytes / 总磁盘空间（字节） */
  total_disk?: number;
  /** Last heartbeat time / 最后心跳时间 */
  last_heartbeat?: string | null;

  // SeaTunnel installation fields / SeaTunnel 安装字段
  /** Whether SeaTunnel is installed / SeaTunnel 是否已安装 */
  seatunnel_installed?: boolean;
  /** SeaTunnel version / SeaTunnel 版本 */
  seatunnel_version?: string;
  /** SeaTunnel installation path / SeaTunnel 安装路径 */
  seatunnel_path?: string;

  // docker specific fields / Docker 专用字段
  /** Docker API URL / Docker API 地址 */
  docker_api_url?: string;
  /** Whether TLS is enabled for Docker / Docker 是否启用 TLS */
  docker_tls_enabled?: boolean;
  /** Docker version / Docker 版本 */
  docker_version?: string;

  // kubernetes specific fields / Kubernetes 专用字段
  /** Kubernetes API URL / Kubernetes API 地址 */
  k8s_api_url?: string;
  /** Kubernetes namespace / Kubernetes 命名空间 */
  k8s_namespace?: string;
  /** Kubernetes version / Kubernetes 版本 */
  k8s_version?: string;
  /** Kubernetes auth mode / K8s 认证方式 */
  k8s_auth_mode?: K8sAuthMode;
  /** Kubernetes connection mode / K8s 连接方式 */
  k8s_access_mode?: K8sAccessMode;
  /** Whether kubeconfig is stored (never returns plaintext) / 是否已存 kubeconfig（永不回明文） */
  has_k8s_kubeconfig?: boolean;
  /** Whether token is stored / 是否已存 token */
  has_k8s_token?: boolean;
  /** Skip TLS verification / 跳过 TLS 校验 */
  k8s_insecure_skip_tls?: boolean;
  /** Optional default ServiceAccount for engine pods in minimal RBAC mode / 最小权限模式下引擎 Pod 默认使用的 ServiceAccount（可选） */
  k8s_default_service_account?: string;
  /** Latest probed RBAC permission tier / 最近一次连通体检识别出的授权模式 */
  k8s_permission_mode?: K8sPermissionMode;
  /** Parsed credential expiration timestamp / 凭据到期时间 */
  k8s_credential_expires_at?: string | null;

  /** Creation time / 创建时间 */
  created_at: string;
  /** Update time / 更新时间 */
  updated_at: string;
}


/**
 * Request to create a new host
 * 创建新主机的请求
 */
export interface CreateHostRequest {
  /** Host name (required) / 主机名称（必填） */
  name: string;
  /** Host type / 主机类型 */
  host_type?: HostType;
  /** Description / 描述 */
  description?: string;

  // bare_metal fields / 物理机字段
  /** IP address (required for bare_metal) / IP 地址（物理机必填） */
  ip_address?: string;
  /** SSH port / SSH 端口 */
  ssh_port?: number;

  // docker fields / Docker 字段
  /** Docker API URL (required for docker) / Docker API 地址（Docker 必填） */
  docker_api_url?: string;
  /** Whether TLS is enabled / 是否启用 TLS */
  docker_tls_enabled?: boolean;
  /** Docker certificate path / Docker 证书路径 */
  docker_cert_path?: string;

  // kubernetes fields / Kubernetes 字段
  /** Kubernetes API URL (required for token mode; auto-extracted for kubeconfig) / K8s API 地址（token 模式必填，kubeconfig 自动解析） */
  k8s_api_url?: string;
  /** Kubernetes namespace / K8s 命名空间 */
  k8s_namespace?: string;
  /** Kubeconfig content / Kubeconfig 内容 */
  k8s_kubeconfig?: string;
  /** Selected kubeconfig context / 指定 Kubeconfig 上下文 */
  k8s_context?: string;
  /** Service account token / 服务账户令牌 */
  k8s_token?: string;
  /** Auth mode / 认证方式 */
  k8s_auth_mode?: K8sAuthMode;
  /** Connection mode / 连接方式 */
  k8s_access_mode?: K8sAccessMode;
  /** CA certificate PEM / CA 证书 PEM */
  k8s_ca_cert?: string;
  /** Skip TLS verification / 跳过 TLS 校验 */
  k8s_insecure_skip_tls?: boolean;
  /** Optional default ServiceAccount for engine pods in minimal RBAC mode / 最小权限模式下引擎 Pod 默认 ServiceAccount（可选） */
  k8s_default_service_account?: string;
}

export type K8sPermissionMode = 'full' | 'minimal' | 'insufficient';

export type K8sConnectErrorCode =
  | 'timeout'
  | 'tls_error'
  | 'unauthorized'
  | 'unreachable'
  | 'invalid_config'
  | 'unknown';

/**
 * RBAC precheck item returned by test-connection / test-connection 返回的 RBAC 预检项
 */
export interface K8sRBACCheckItem {
  resource: string;
  verb: string;
  allowed: boolean;
  reason?: string;
  scope?: 'required' | 'optional' | 'extended';
}

/**
 * K8s connectivity probe result / K8s 连通性体检结果
 */
export interface K8sTestConnectionResult {
  connected: boolean;
  server_version?: string;
  message?: string;
  error_code?: K8sConnectErrorCode;
  permission_mode?: K8sPermissionMode;
  can_manage_rbac?: boolean;
  can_list_namespaces?: boolean;
  control_plane_sa?: string;
  service_accounts?: string[];
  credential_expires_at?: string | null;
  rbac: K8sRBACCheckItem[];
  elapsed_ms: number;
}

export interface K8sDraftTestRequest {
  host_id?: number;
  k8s_auth_mode: K8sAuthMode;
  k8s_namespace?: string;
  k8s_kubeconfig?: string;
  k8s_context?: string;
  k8s_api_url?: string;
  k8s_token?: string;
  k8s_ca_cert?: string;
  k8s_insecure_skip_tls?: boolean;
}

/**
 * Request to update an existing host
 * 更新现有主机的请求
 */
export interface UpdateHostRequest {
  /** Host name / 主机名称 */
  name?: string;
  /** Description / 描述 */
  description?: string;

  // bare_metal fields / 物理机字段
  /** IP address / IP 地址 */
  ip_address?: string;
  /** SSH port / SSH 端口 */
  ssh_port?: number;

  // docker fields / Docker 字段
  /** Docker API URL / Docker API 地址 */
  docker_api_url?: string;
  /** Whether TLS is enabled / 是否启用 TLS */
  docker_tls_enabled?: boolean;
  /** Docker certificate path / Docker 证书路径 */
  docker_cert_path?: string;

  // kubernetes fields / Kubernetes 字段
  /** Kubernetes API URL / K8s API 地址 */
  k8s_api_url?: string;
  /** Kubernetes namespace / K8s 命名空间 */
  k8s_namespace?: string;
  /** Kubeconfig content / Kubeconfig 内容 */
  k8s_kubeconfig?: string;
  /** Selected kubeconfig context / 指定 Kubeconfig 上下文 */
  k8s_context?: string;
  /** Service account token / 服务账户令牌 */
  k8s_token?: string;
  /** Auth mode / 认证方式 */
  k8s_auth_mode?: K8sAuthMode;
  /** Connection mode / 连接方式 */
  k8s_access_mode?: K8sAccessMode;
  /** CA certificate PEM / CA 证书 PEM */
  k8s_ca_cert?: string;
  /** Skip TLS verification / 跳过 TLS 校验 */
  k8s_insecure_skip_tls?: boolean;
  /** Optional default ServiceAccount for engine pods in minimal RBAC mode / 最小权限模式下引擎 Pod 默认 ServiceAccount（可选） */
  k8s_default_service_account?: string;
}

/**
 * Request parameters for listing hosts
 * 获取主机列表的请求参数
 */
export interface ListHostsRequest {
  /** Current page number (1-based) / 当前页码（从 1 开始） */
  current: number;
  /** Page size / 每页数量 */
  size: number;
  /** Filter by name / 按名称过滤 */
  name?: string;
  /** Filter by host type / 按主机类型过滤 */
  host_type?: HostType;
  /** Filter by IP address / 按 IP 地址过滤 */
  ip_address?: string;
  /** Filter by status / 按状态过滤 */
  status?: HostStatus;
  /** Filter by agent status / 按 Agent 状态过滤 */
  agent_status?: AgentStatus;
  /** Filter by online status / 按在线状态过滤 */
  is_online?: boolean;
}

/**
 * Host list data
 * 主机列表数据
 */
export interface HostListData {
  /** Total count / 总数量 */
  total: number;
  /** Host list / 主机列表 */
  hosts: HostInfo[];
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

/**
 * List hosts response type
 * 获取主机列表响应类型
 */
export type ListHostsResponse = BackendResponse<HostListData>;

/**
 * Create host response type
 * 创建主机响应类型
 */
export type CreateHostResponse = BackendResponse<HostInfo>;

/**
 * Get host response type
 * 获取主机详情响应类型
 */
export type GetHostResponse = BackendResponse<HostInfo>;

/**
 * Update host response type
 * 更新主机响应类型
 */
export type UpdateHostResponse = BackendResponse<HostInfo>;

/**
 * Delete host response type
 * 删除主机响应类型
 */
export type DeleteHostResponse = BackendResponse<null>;

/**
 * Install command data
 * 安装命令数据
 */
export interface InstallCommandData {
  /** Install command / 安装命令 */
  command: string;
}

/**
 * Get install command response type
 * 获取安装命令响应类型
 */
export type GetInstallCommandResponse = BackendResponse<InstallCommandData>;

/**
 * Associated cluster info (returned when deletion fails due to cluster association)
 * 关联的集群信息（删除失败时返回）
 */
export interface AssociatedCluster {
  /** Cluster ID / 集群 ID */
  id: number;
  /** Cluster name / 集群名称 */
  name: string;
}
