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
 * Audit Service
 * 审计服务
 *
 * Handles audit log and command log operations including listing and retrieval.
 * 处理审计日志和命令日志操作，包括列表查询和详情获取。
 */

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import {
  CommandLogInfo,
  CommandLogListData,
  AuditLogInfo,
  AuditLogListData,
  ListCommandLogsRequest,
  ListAuditLogsRequest,
  ListCommandLogsResponse,
  GetCommandLogResponse,
  ListAuditLogsResponse,
  GetAuditLogResponse,
} from './types';

/**
 * Audit Service class
 * 审计服务类
 */
export class AuditService extends BaseService {
  /**
   * Command logs API base path
   * 命令日志 API 基础路径
   */
  protected static readonly commandsPath = '/commands';

  /**
   * Audit logs API base path
   * 审计日志 API 基础路径
   */
  protected static readonly auditLogsPath = '/audit-logs';

  // ==================== Command Log Methods 命令日志方法 ====================

  /**
   * Get command log list with filtering and pagination
   * 获取命令日志列表（支持过滤和分页）
   *
   * @param params - Query parameters / 查询参数
   * @returns Command log list data / 命令日志列表数据
   */
  static async getCommandLogs(
    params: ListCommandLogsRequest,
  ): Promise<CommandLogListData> {
    const response = await apiClient.get<ListCommandLogsResponse>(
      this.commandsPath,
      {
        params: {
          current: params.current,
          size: params.size,
          command_id: params.command_id,
          agent_id: params.agent_id,
          host_id: params.host_id,
          command_type: params.command_type,
          request_id: params.request_id,
          status: params.status,
          start_time: params.start_time,
          end_time: params.end_time,
        },
      },
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }


  /**
   * Get command log by ID
   * 根据 ID 获取命令日志详情
   *
   * @param logId - Command log ID / 命令日志 ID
   * @returns Command log information / 命令日志信息
   */
  static async getCommandLog(logId: number): Promise<CommandLogInfo> {
    const response = await apiClient.get<GetCommandLogResponse>(
      `${this.commandsPath}/${logId}`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  // ==================== Audit Log Methods 审计日志方法 ====================

  /**
   * Get audit log list with filtering and pagination
   * 获取审计日志列表（支持过滤和分页）
   *
   * @param params - Query parameters / 查询参数
   * @returns Audit log list data / 审计日志列表数据
   */
  static async getAuditLogs(
    params: ListAuditLogsRequest,
  ): Promise<AuditLogListData> {
    const response = await apiClient.get<ListAuditLogsResponse>(
      this.auditLogsPath,
      {
        params: {
          current: params.current,
          size: params.size,
          user_id: params.user_id,
          username: params.username,
          client_type: params.client_type,
          action_group: params.action_group,
          action: params.action,
          resource_type: params.resource_type,
          resource_id: params.resource_id,
          trigger: params.trigger,
          start_time: params.start_time,
          end_time: params.end_time,
        },
      },
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Get audit log by ID
   * 根据 ID 获取审计日志详情
   *
   * @param logId - Audit log ID / 审计日志 ID
   * @returns Audit log information / 审计日志信息
   */
  static async getAuditLog(logId: number): Promise<AuditLogInfo> {
    const response = await apiClient.get<GetAuditLogResponse>(
      `${this.auditLogsPath}/${logId}`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  // ==================== Safe Methods (with error handling) 安全方法（带错误处理） ====================

  /**
   * Get command log list (with error handling)
   * 获取命令日志列表（带错误处理）
   *
   * @param params - Query parameters / 查询参数
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getCommandLogsSafe(params: ListCommandLogsRequest): Promise<{
    success: boolean;
    data?: CommandLogListData;
    error?: string;
  }> {
    try {
      const data = await this.getCommandLogs(params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取命令日志列表失败';
      return {
        success: false,
        data: {total: 0, commands: []},
        error: errorMessage,
      };
    }
  }

  /**
   * Get command log by ID (with error handling)
   * 根据 ID 获取命令日志详情（带错误处理）
   *
   * @param logId - Command log ID / 命令日志 ID
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getCommandLogSafe(logId: number): Promise<{
    success: boolean;
    data?: CommandLogInfo;
    error?: string;
  }> {
    try {
      const data = await this.getCommandLog(logId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取命令日志详情失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Get audit log list (with error handling)
   * 获取审计日志列表（带错误处理）
   *
   * @param params - Query parameters / 查询参数
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getAuditLogsSafe(params: ListAuditLogsRequest): Promise<{
    success: boolean;
    data?: AuditLogListData;
    error?: string;
  }> {
    try {
      const data = await this.getAuditLogs(params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取审计日志列表失败';
      return {
        success: false,
        data: {total: 0, logs: []},
        error: errorMessage,
      };
    }
  }

  /**
   * Get audit log by ID (with error handling)
   * 根据 ID 获取审计日志详情（带错误处理）
   *
   * @param logId - Audit log ID / 审计日志 ID
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getAuditLogSafe(logId: number): Promise<{
    success: boolean;
    data?: AuditLogInfo;
    error?: string;
  }> {
    try {
      const data = await this.getAuditLog(logId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取审计日志详情失败';
      return {success: false, error: errorMessage};
    }
  }
}
