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
 * Host Service
 * 主机服务
 *
 * Handles host management operations including CRUD and install command retrieval.
 * 处理主机管理操作，包括 CRUD 和获取安装命令。
 */

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import {createWebExecutionHeaders} from '../core/execution-headers';
import {
  HostInfo,
  HostListData,
  CreateHostRequest,
  UpdateHostRequest,
  ListHostsRequest,
  ListHostsResponse,
  CreateHostResponse,
  GetHostResponse,
  UpdateHostResponse,
  GetInstallCommandResponse,
  InstallCommandData,
} from './types';

/**
 * Host Service class
 * 主机服务类
 */
export class HostService extends BaseService {
  /**
   * API base path
   * API 基础路径
   */
  protected static readonly basePath = '/hosts';

  /**
   * Get host list with filtering and pagination
   * 获取主机列表（支持过滤和分页）
   *
   * @param params - Query parameters / 查询参数
   * @returns Host list data / 主机列表数据
   */
  static async getHosts(params: ListHostsRequest): Promise<HostListData> {
    const response = await apiClient.get<ListHostsResponse>(this.basePath, {
      params: {
        current: params.current,
        size: params.size,
        name: params.name,
        host_type: params.host_type,
        ip_address: params.ip_address,
        status: params.status,
        agent_status: params.agent_status,
        is_online: params.is_online,
      },
    });

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }


  /**
   * Get host by ID
   * 根据 ID 获取主机详情
   *
   * @param hostId - Host ID / 主机 ID
   * @returns Host information / 主机信息
   */
  static async getHost(hostId: number): Promise<HostInfo> {
    const response = await apiClient.get<GetHostResponse>(
      `${this.basePath}/${hostId}`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Create a new host
   * 创建新主机
   *
   * @param data - Host creation data / 主机创建数据
   * @returns Created host information / 创建的主机信息
   */
  static async createHost(data: CreateHostRequest): Promise<HostInfo> {
    const response = await apiClient.post<CreateHostResponse>(
      this.basePath,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Update an existing host
   * 更新现有主机
   *
   * @param hostId - Host ID / 主机 ID
   * @param data - Host update data / 主机更新数据
   * @returns Updated host information / 更新后的主机信息
   */
  static async updateHost(
    hostId: number,
    data: UpdateHostRequest,
  ): Promise<HostInfo> {
    const response = await apiClient.put<UpdateHostResponse>(
      `${this.basePath}/${hostId}`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Delete a host
   * 删除主机
   *
   * @param hostId - Host ID / 主机 ID
   */
  static async deleteHost(
    hostId: number,
    idempotencyKey?: string,
    confirmationId?: string,
  ): Promise<void> {
    const executionHeaders = createWebExecutionHeaders('host-delete', {
      idempotencyKey,
      confirmationId,
    });
    try {
      await this.deleteWithConfig<null>(`/${hostId}`, {
        headers: executionHeaders.headers,
      });
    } catch (error) {
      const data = (
        error as Error & {
          data?: {confirmation_required?: boolean; confirmation_id?: string};
        }
      ).data;
      if (
        !confirmationId &&
        data?.confirmation_required &&
        data.confirmation_id
      ) {
        await this.deleteHost(
          hostId,
          executionHeaders.idempotencyKey,
          data.confirmation_id,
        );
        return;
      }
      throw error;
    }
  }

  /**
   * Get Agent install command for a host
   * 获取主机的 Agent 安装命令
   *
   * @param hostId - Host ID / 主机 ID
   * @returns Install command data / 安装命令数据
   */
  static async getInstallCommand(hostId: number): Promise<InstallCommandData> {
    const response = await apiClient.get<GetInstallCommandResponse>(
      `${this.basePath}/${hostId}/install-command`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  // ==================== Safe Methods (with error handling) 安全方法（带错误处理） ====================

  /**
   * Get host list (with error handling)
   * 获取主机列表（带错误处理）
   *
   * @param params - Query parameters / 查询参数
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getHostsSafe(params: ListHostsRequest): Promise<{
    success: boolean;
    data?: HostListData;
    error?: string;
  }> {
    try {
      const data = await this.getHosts(params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取主机列表失败';
      return {
        success: false,
        data: {total: 0, hosts: []},
        error: errorMessage,
      };
    }
  }

  /**
   * Get host by ID (with error handling)
   * 根据 ID 获取主机详情（带错误处理）
   *
   * @param hostId - Host ID / 主机 ID
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getHostSafe(hostId: number): Promise<{
    success: boolean;
    data?: HostInfo;
    error?: string;
  }> {
    try {
      const data = await this.getHost(hostId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取主机详情失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Create a new host (with error handling)
   * 创建新主机（带错误处理）
   *
   * @param data - Host creation data / 主机创建数据
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async createHostSafe(data: CreateHostRequest): Promise<{
    success: boolean;
    data?: HostInfo;
    error?: string;
  }> {
    try {
      const result = await this.createHost(data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '创建主机失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Update an existing host (with error handling)
   * 更新现有主机（带错误处理）
   *
   * @param hostId - Host ID / 主机 ID
   * @param data - Host update data / 主机更新数据
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async updateHostSafe(
    hostId: number,
    data: UpdateHostRequest,
  ): Promise<{
    success: boolean;
    data?: HostInfo;
    error?: string;
  }> {
    try {
      const result = await this.updateHost(hostId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '更新主机失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Delete a host (with error handling)
   * 删除主机（带错误处理）
   *
   * @param hostId - Host ID / 主机 ID
   * @returns Result with success status and error message / 包含成功状态和错误信息的结果
   */
  static async deleteHostSafe(hostId: number): Promise<{
    success: boolean;
    error?: string;
  }> {
    try {
      await this.deleteHost(hostId);
      return {success: true};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '删除主机失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Get Agent install command (with error handling)
   * 获取 Agent 安装命令（带错误处理）
   *
   * @param hostId - Host ID / 主机 ID
   * @returns Result with success status, data, and error message / 包含成功状态、数据和错误信息的结果
   */
  static async getInstallCommandSafe(hostId: number): Promise<{
    success: boolean;
    data?: InstallCommandData;
    error?: string;
  }> {
    try {
      const data = await this.getInstallCommand(hostId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取安装命令失败';
      return {success: false, error: errorMessage};
    }
  }
}
