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
 * Config Service Types
 * 配置服务类型定义
 *
 * This file defines all types related to cluster configuration management.
 * 本文件定义所有与集群配置管理相关的类型。
 */

/* eslint-disable no-unused-vars */
/**
 * Config type enumeration
 * 配置类型枚举
 */
export enum ConfigType {
  // 通用配置（Hybrid 模式）
  SEATUNNEL = 'seatunnel.yaml',
  HAZELCAST = 'hazelcast.yaml',
  HAZELCAST_CLIENT = 'hazelcast-client.yaml',
  JVM_OPTIONS = 'jvm_options',
  LOG4J2 = 'log4j2.properties',
  // 环境变量配置 / Environment variables configuration
  SEATUNNEL_ENV = 'seatunnel-env.sh',
  // 分离模式配置（Separated 模式）
  HAZELCAST_MASTER = 'hazelcast-master.yaml',
  HAZELCAST_WORKER = 'hazelcast-worker.yaml',
  JVM_MASTER_OPTIONS = 'jvm_master_options',
  JVM_WORKER_OPTIONS = 'jvm_worker_options',
}
/* eslint-enable no-unused-vars */

/**
 * Config type display names
 * 配置类型显示名称
 */
export const ConfigTypeNames: Record<ConfigType, string> = {
  [ConfigType.SEATUNNEL]: 'SeaTunnel 配置',
  [ConfigType.HAZELCAST]: 'Hazelcast 配置',
  [ConfigType.HAZELCAST_CLIENT]: 'Hazelcast Client 配置',
  [ConfigType.JVM_OPTIONS]: 'JVM 参数',
  [ConfigType.LOG4J2]: 'Log4j2 日志配置',
  [ConfigType.SEATUNNEL_ENV]: 'SeaTunnel 环境变量 (seatunnel-env.sh)',
  [ConfigType.HAZELCAST_MASTER]: 'Hazelcast Master 配置',
  [ConfigType.HAZELCAST_WORKER]: 'Hazelcast Worker 配置',
  [ConfigType.JVM_MASTER_OPTIONS]: 'JVM Master 参数',
  [ConfigType.JVM_WORKER_OPTIONS]: 'JVM Worker 参数',
};

/**
 * Hybrid mode config types
 * Hybrid 模式支持的配置类型
 */
export const HybridConfigTypes: ConfigType[] = [
  ConfigType.SEATUNNEL,
  ConfigType.HAZELCAST,
  ConfigType.HAZELCAST_CLIENT,
  ConfigType.JVM_OPTIONS,
  ConfigType.LOG4J2,
  ConfigType.SEATUNNEL_ENV,
];

/**
 * Separated mode config types
 * Separated 模式支持的配置类型
 */
export const SeparatedConfigTypes: ConfigType[] = [
  ConfigType.SEATUNNEL,
  ConfigType.HAZELCAST_MASTER,
  ConfigType.HAZELCAST_WORKER,
  ConfigType.HAZELCAST_CLIENT,
  ConfigType.JVM_MASTER_OPTIONS,
  ConfigType.JVM_WORKER_OPTIONS,
  ConfigType.LOG4J2,
  ConfigType.SEATUNNEL_ENV,
];

/**
 * Get config types for deployment mode
 * 根据部署模式获取支持的配置类型
 */
export function getConfigTypesForMode(deploymentMode: string): ConfigType[] {
  return deploymentMode === 'separated' ? SeparatedConfigTypes : HybridConfigTypes;
}

/**
 * Config information returned from API
 * API 返回的配置信息
 */
export interface ConfigInfo {
  /** Config ID / 配置 ID */
  id: number;
  /** Cluster ID / 集群 ID */
  cluster_id: number;
  /** Host ID (null for cluster template) / 主机 ID（集群模板为 null） */
  host_id: number | null;
  /** Host name / 主机名称 */
  host_name?: string;
  /** Host IP address / 主机 IP 地址 */
  host_ip?: string;
  /** Config type / 配置类型 */
  config_type: ConfigType;
  /** File path on node / 节点上的文件路径 */
  file_path: string;
  /** Config content / 配置内容 */
  content: string;
  /** Current version number / 当前版本号 */
  version: number;
  /** Whether this is a cluster template / 是否为集群模板 */
  is_template: boolean;
  /** Whether content matches template / 内容是否与模板一致 */
  match_template: boolean;
  /** Update time / 更新时间 */
  updated_at: string;
  /** Updated by user ID / 更新者用户 ID */
  updated_by: number;
  /** Push error message / 推送到节点的错误信息 */
  push_error?: string;
}

/**
 * Config version information
 * 配置版本信息
 */
export interface ConfigVersionInfo {
  /** Version ID / 版本 ID */
  id: number;
  /** Config ID / 配置 ID */
  config_id: number;
  /** Version number / 版本号 */
  version: number;
  /** Config content / 配置内容 */
  content: string;
  /** Comment / 修改说明 */
  comment: string;
  /** Created by user ID / 创建者用户 ID */
  created_by: number;
  /** Creation time / 创建时间 */
  created_at: string;
}

/**
 * Request to update config
 * 更新配置请求
 */
export interface UpdateConfigRequest {
  /** Config content (required) / 配置内容（必填） */
  content: string;
  /** Comment / 修改说明 */
  comment?: string;
}

/**
 * Request to rollback config
 * 回滚配置请求
 */
export interface RollbackConfigRequest {
  /** Target version number (required) / 目标版本号（必填） */
  version: number;
  /** Comment / 修改说明 */
  comment?: string;
}

/**
 * Request to promote config to cluster
 * 推广配置到集群请求
 */
export interface PromoteConfigRequest {
  /** Comment / 修改说明 */
  comment?: string;
}

/**
 * Request to sync config from template
 * 从模板同步配置请求
 */
export interface SyncConfigRequest {
  /** Comment / 修改说明 */
  comment?: string;
}

/**
 * Request to normalize config content
 * 配置内容规范化请求
 */
export interface NormalizeConfigRequest {
  /** Config type / 配置类型 */
  config_type: ConfigType;
  /** Config content / 配置内容 */
  content: string;
}

/**
 * Backend response structure
 * 后端响应结构
 */
export interface BackendResponse<T = unknown> {
  /** Error message, empty string means no error / 错误信息，空字符串表示无错误 */
  error?: string;
  /** Response data / 响应数据 */
  data?: T;
  /** Message / 消息 */
  message?: string;
}

// ==================== Response Types 响应类型 ====================

/** API response wrapper / API 响应包装 */
export interface ApiResponse<T> {
  error_msg: string;
  data: T;
}

/** Get cluster configs response type / 获取集群配置列表响应类型 */
export type GetClusterConfigsResponse = ApiResponse<ConfigInfo[]>;

/** Get config response type / 获取配置详情响应类型 */
export type GetConfigResponse = ApiResponse<ConfigInfo>;

/** Update config response type / 更新配置响应类型 */
export type UpdateConfigResponse = ApiResponse<ConfigInfo>;

/** Get config versions response type / 获取配置版本历史响应类型 */
export type GetConfigVersionsResponse = ApiResponse<ConfigVersionInfo[]>;

/** Rollback config response type / 回滚配置响应类型 */
export type RollbackConfigResponse = ApiResponse<ConfigInfo>;

/** Promote config response type / 推广配置响应类型 */
export type PromoteConfigResponse = ApiResponse<{ message: string }>;

/** Sync config response type / 同步配置响应类型 */
export type SyncConfigResponse = ApiResponse<ConfigInfo>;

/** Normalize config response type / 规范化配置响应类型 */
export type NormalizeConfigResponse = ApiResponse<{
  content: string;
}>;


/**
 * Push error information
 * 推送错误信息
 */
export interface PushError {
  /** Host ID / 主机 ID */
  host_id: number;
  /** Host IP address / 主机 IP 地址 */
  host_ip?: string;
  /** Error message / 错误信息 */
  message: string;
}

/**
 * Sync all result
 * 批量同步结果
 */
export interface SyncAllResult {
  /** Number of configs synced / 同步成功的数量 */
  synced_count: number;
  /** Push errors / 推送失败的节点列表 */
  push_errors: PushError[];
}

/** Sync all response type / 批量同步响应类型 */
export type SyncAllResponse = ApiResponse<{
  message: string;
  synced_count: number;
  push_errors: PushError[];
}>;
