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
 * Audit Service Types
 * 审计服务类型定义
 *
 * This file defines all types related to audit and command log management.
 * 本文件定义所有与审计和命令日志管理相关的类型。
 */

/* eslint-disable no-unused-vars */

/**
 * Command execution status enumeration
 * 命令执行状态枚举
 */
export enum CommandStatus {
  /** Waiting to be executed / 等待执行 */
  PENDING = 'pending',
  /** Currently executing / 正在执行 */
  RUNNING = 'running',
  /** Completed successfully / 执行成功 */
  SUCCESS = 'success',
  /** Execution failed / 执行失败 */
  FAILED = 'failed',
  /** Cancelled / 已取消 */
  CANCELLED = 'cancelled',
}

/**
 * Command parameters type
 * 命令参数类型
 */
export type CommandParameters = Record<string, unknown>;

/**
 * Audit details type
 * 审计详情类型
 */
export type AuditDetails = Record<string, unknown>;

/**
 * Command log information returned from API
 * API 返回的命令日志信息
 */
export interface CommandLogInfo {
  /** Command log ID / 命令日志 ID */
  id: number;
  /** Unique command ID / 唯一命令 ID */
  command_id: string;
  /** Control-plane request ID shared with the audit row / 与审计行共用的控制层请求编号 */
  request_id?: string;
  /** Shared execution ID when present / 关联的公共执行编号 */
  execution_id?: string;
  /** Agent ID that executed the command / 执行命令的 Agent ID */
  agent_id: string;
  /** Host ID / 主机 ID */
  host_id: number | null;
  /** Command type / 命令类型 */
  command_type: string;
  /** Redacted command actually executed by the Agent / Agent 实际执行且已脱敏的命令 */
  display_command?: string;
  /** Command parameters / 命令参数 */
  parameters: CommandParameters | null;
  /** Execution status / 执行状态 */
  status: CommandStatus;
  /** Execution progress (0-100) / 执行进度 */
  progress: number;
  /** Command output / 命令输出 */
  output: string;
  /** Error message / 错误信息 */
  error: string;
  /** Execution start time / 执行开始时间 */
  started_at: string | null;
  /** Execution finish time / 执行结束时间 */
  finished_at: string | null;
  /** Creation time / 创建时间 */
  created_at: string;
  /** Creator user ID / 创建者用户 ID */
  created_by: number | null;
}

/**
 * Audit log information returned from API
 * API 返回的审计日志信息
 */
export interface AuditLogInfo {
  /** Audit log ID / 审计日志 ID */
  id: number;
  /** User ID / 用户 ID */
  user_id: number | null;
  /** Username / 用户名 */
  username: string;
  /** Action performed / 执行的操作 */
  action: string;
  /** Resource type / 资源类型 */
  resource_type: string;
  /** Resource ID / 资源 ID */
  resource_id: string;
  /** Resource name / 资源名称 */
  resource_name: string;
  /** Trigger: "auto" (Agent) or "manual" (user), empty for legacy / 触发方式 */
  trigger?: string;
  /** Client that issued the action: cli, web, api, or system / 发起端 */
  client_type?: string;
  /** Additional details / 附加详情 */
  details: AuditDetails | null;
  /** Client IP address / 客户端 IP 地址 */
  ip_address: string;
  /** User agent string / 用户代理字符串 */
  user_agent: string;
  /** Creation time / 创建时间 */
  created_at: string;
  /** Agent commands issued by the same request / 同一次请求下发的 Agent 命令数 */
  command_count?: number;
  /** Request ID shared with command logs / 与命令记录共用的请求编号 */
  request_id?: string;
}


/**
 * Request parameters for listing command logs
 * 获取命令日志列表的请求参数
 */
export interface ListCommandLogsRequest {
  /** Current page number (1-based) / 当前页码（从 1 开始） */
  current: number;
  /** Page size / 每页数量 */
  size: number;
  /** Filter by command ID / 按命令 ID 过滤 */
  command_id?: string;
  /** Filter by agent ID / 按 Agent ID 过滤 */
  agent_id?: string;
  /** Filter by host ID / 按主机 ID 过滤 */
  host_id?: number;
  /** Filter by command type / 按命令类型过滤 */
  command_type?: string;
  /** Filter by the control-plane request ID / 按控制层请求编号过滤 */
  request_id?: string;
  /** Filter by status / 按状态过滤 */
  status?: CommandStatus;
  /** Filter by start time (RFC3339 format) / 按开始时间过滤（RFC3339 格式） */
  start_time?: string;
  /** Filter by end time (RFC3339 format) / 按结束时间过滤（RFC3339 格式） */
  end_time?: string;
}

/**
 * Request parameters for listing audit logs
 * 获取审计日志列表的请求参数
 */
export interface ListAuditLogsRequest {
  /** Current page number (1-based) / 当前页码（从 1 开始） */
  current: number;
  /** Page size / 每页数量 */
  size: number;
  /** Filter by user ID / 按用户 ID 过滤 */
  user_id?: number;
  /** Filter by username / 按用户名过滤 */
  username?: string;
  /** Filter by client: web, cli, or system / 按来源过滤 */
  client_type?: string;
  /** Filter by action category / 按操作类别过滤 */
  action_group?: string;
  /** Filter by action / 按操作过滤 */
  action?: string;
  /** Filter by resource type / 按资源类型过滤 */
  resource_type?: string;
  /** Filter by resource ID / 按资源 ID 过滤 */
  resource_id?: string;
  /** Filter by trigger: auto | manual / 按触发方式过滤 */
  trigger?: string;
  /** Filter by start time (RFC3339 format) / 按开始时间过滤（RFC3339 格式） */
  start_time?: string;
  /** Filter by end time (RFC3339 format) / 按结束时间过滤（RFC3339 格式） */
  end_time?: string;
}

/**
 * Command log list data
 * 命令日志列表数据
 */
export interface CommandLogListData {
  /** Total count / 总数量 */
  total: number;
  /** Command log list / 命令日志列表 */
  commands: CommandLogInfo[];
}

/**
 * Audit log list data
 * 审计日志列表数据
 */
export interface AuditLogListData {
  /** Total count / 总数量 */
  total: number;
  /** Audit log list / 审计日志列表 */
  logs: AuditLogInfo[];
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
 * List command logs response type
 * 获取命令日志列表响应类型
 */
export type ListCommandLogsResponse = BackendResponse<CommandLogListData>;

/**
 * Get command log response type
 * 获取命令日志详情响应类型
 */
export type GetCommandLogResponse = BackendResponse<CommandLogInfo>;

/**
 * List audit logs response type
 * 获取审计日志列表响应类型
 */
export type ListAuditLogsResponse = BackendResponse<AuditLogListData>;

/**
 * Get audit log response type
 * 获取审计日志详情响应类型
 */
export type GetAuditLogResponse = BackendResponse<AuditLogInfo>;
