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
 * Config Service
 * 配置管理服务
 *
 * Provides methods for managing cluster configuration files.
 * 提供集群配置文件管理方法。
 */

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import {localizeBackendText} from '@/lib/i18n/localize-text';
import type {
  ConfigInfo,
  ConfigVersionInfo,
  UpdateConfigRequest,
  RollbackConfigRequest,
  PromoteConfigRequest,
  SyncConfigRequest,
  NormalizeConfigRequest,
  GetClusterConfigsResponse,
  GetConfigResponse,
  UpdateConfigResponse,
  GetConfigVersionsResponse,
  RollbackConfigResponse,
  PromoteConfigResponse,
  SyncConfigResponse,
  NormalizeConfigResponse,
  SyncAllResult,
} from './types';

/**
 * Config Service class
 * 配置服务类
 */
export class ConfigService extends BaseService {
  /**
   * API base path for configs
   * 配置 API 基础路径
   */
  protected static readonly basePath = '';

  /**
   * Get all configs for a cluster
   * 获取集群所有配置
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Config list / 配置列表
   */
  static async getClusterConfigs(clusterId: number): Promise<ConfigInfo[]> {
    const response = await apiClient.get<GetClusterConfigsResponse>(
      `${this.basePath}/clusters/${clusterId}/configs`
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Get config details
   * 获取配置详情
   *
   * @param configId - Config ID / 配置 ID
   * @returns Config info / 配置信息
   */
  static async getConfig(configId: number): Promise<ConfigInfo> {
    const response = await apiClient.get<GetConfigResponse>(
      `${this.basePath}/configs/${configId}`
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Normalize YAML config content
   * 规范化 YAML 配置内容
   *
   * @param request - Normalize request / 规范化请求
   * @returns Normalized content / 规范化后的内容
   */
  static async normalizeConfig(request: NormalizeConfigRequest): Promise<string> {
    const response = await apiClient.post<NormalizeConfigResponse>(
      `${this.basePath}/configs/normalize`,
      request
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data.content;
  }

  /**
   * Update config content
   * 更新配置内容
   *
   * @param configId - Config ID / 配置 ID
   * @param request - Update request / 更新请求
   * @returns Updated config info / 更新后的配置信息
   */
  static async updateConfig(
    configId: number,
    request: UpdateConfigRequest
  ): Promise<ConfigInfo> {
    const response = await apiClient.put<UpdateConfigResponse>(
      `${this.basePath}/configs/${configId}`,
      request
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Get config version history
   * 获取配置版本历史
   *
   * @param configId - Config ID / 配置 ID
   * @returns Version list / 版本列表
   */
  static async getConfigVersions(configId: number): Promise<ConfigVersionInfo[]> {
    const response = await apiClient.get<GetConfigVersionsResponse>(
      `${this.basePath}/configs/${configId}/versions`
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Rollback config to a specific version
   * 回滚配置到指定版本
   *
   * @param configId - Config ID / 配置 ID
   * @param request - Rollback request / 回滚请求
   * @returns Rolled back config info / 回滚后的配置信息
   */
  static async rollbackConfig(
    configId: number,
    request: RollbackConfigRequest
  ): Promise<ConfigInfo> {
    const response = await apiClient.post<RollbackConfigResponse>(
      `${this.basePath}/configs/${configId}/rollback`,
      request
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Promote config to cluster (node config -> template -> all nodes)
   * 推广配置到集群（节点配置 -> 模板 -> 所有节点）
   *
   * @param configId - Config ID / 配置 ID
   * @param request - Promote request / 推广请求
   * @returns Operation result / 操作结果
   */
  static async promoteConfig(
    configId: number,
    request?: PromoteConfigRequest
  ): Promise<SyncAllResult> {
    const response = await apiClient.post<PromoteConfigResponse>(
      `${this.basePath}/configs/${configId}/promote`,
      request || {}
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data as unknown as SyncAllResult;
  }

  /**
   * Sync config from cluster template
   * 从集群模板同步配置
   *
   * @param configId - Config ID / 配置 ID
   * @param request - Sync request / 同步请求
   * @returns Synced config info / 同步后的配置信息
   */
  static async syncFromTemplate(
    configId: number,
    request?: SyncConfigRequest
  ): Promise<ConfigInfo> {
    const response = await apiClient.post<SyncConfigResponse>(
      `${this.basePath}/configs/${configId}/sync`,
      request || {}
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Initialize cluster configs from a host
   * 从主机初始化集群配置
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param hostId - Host ID to pull configs from / 拉取配置的主机 ID
   * @param installDir - SeaTunnel installation directory / SeaTunnel 安装目录
   * @returns Operation result / 操作结果
   */
  static async initClusterConfigs(
    clusterId: number,
    hostId: number,
    installDir: string
  ): Promise<{ message: string }> {
    const response = await apiClient.post<{ error_msg: string; data: { message: string } }>(
      `${this.basePath}/clusters/${clusterId}/configs/init`,
      { host_id: hostId, install_dir: installDir }
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  /**
   * Push config to node
   * 推送配置到节点
   *
   * @param configId - Config ID / 配置 ID
   * @param installDir - SeaTunnel installation directory / SeaTunnel 安装目录
   * @returns Operation result / 操作结果
   */
  static async pushConfigToNode(
    configId: number,
    installDir: string
  ): Promise<{ message: string }> {
    const response = await apiClient.post<{ error_msg: string; data: { message: string } }>(
      `${this.basePath}/configs/${configId}/push`,
      { install_dir: installDir }
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  // ==================== Safe Methods (with error handling) 安全方法（带错误处理） ====================

  /**
   * Get cluster configs (with error handling)
   * 获取集群配置（带错误处理）
   */
  static async getClusterConfigsSafe(clusterId: number): Promise<{
    success: boolean;
    data?: ConfigInfo[];
    error?: string;
  }> {
    try {
      const data = await this.getClusterConfigs(clusterId);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '获取配置列表失败';
      return { success: false, data: [], error: errorMessage };
    }
  }

  /**
   * Get config (with error handling)
   * 获取配置详情（带错误处理）
   */
  static async getConfigSafe(configId: number): Promise<{
    success: boolean;
    data?: ConfigInfo;
    error?: string;
  }> {
    try {
      const data = await this.getConfig(configId);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '获取配置详情失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Update config (with error handling)
   * 更新配置（带错误处理）
   */
  static async updateConfigSafe(
    configId: number,
    request: UpdateConfigRequest
  ): Promise<{
    success: boolean;
    data?: ConfigInfo;
    error?: string;
  }> {
    try {
      const data = await this.updateConfig(configId, request);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '更新配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Get config versions (with error handling)
   * 获取配置版本历史（带错误处理）
   */
  static async getConfigVersionsSafe(configId: number): Promise<{
    success: boolean;
    data?: ConfigVersionInfo[];
    error?: string;
  }> {
    try {
      const data = await this.getConfigVersions(configId);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '获取版本历史失败';
      return { success: false, data: [], error: errorMessage };
    }
  }

  /**
   * Rollback config (with error handling)
   * 回滚配置（带错误处理）
   */
  static async rollbackConfigSafe(
    configId: number,
    request: RollbackConfigRequest
  ): Promise<{
    success: boolean;
    data?: ConfigInfo;
    error?: string;
  }> {
    try {
      const data = await this.rollbackConfig(configId, request);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '回滚配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Promote config (with error handling)
   * 推广配置（带错误处理）
   */
  static async promoteConfigSafe(
    configId: number,
    request?: PromoteConfigRequest
  ): Promise<{
    success: boolean;
    data?: SyncAllResult;
    error?: string;
  }> {
    try {
      const data = await this.promoteConfig(configId, request);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '推广配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Sync from template (with error handling)
   * 从模板同步（带错误处理）
   */
  static async syncFromTemplateSafe(
    configId: number,
    request?: SyncConfigRequest
  ): Promise<{
    success: boolean;
    data?: ConfigInfo;
    error?: string;
  }> {
    try {
      const data = await this.syncFromTemplate(configId, request);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '同步配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Initialize cluster configs (with error handling)
   * 初始化集群配置（带错误处理）
   */
  static async initClusterConfigsSafe(
    clusterId: number,
    hostId: number,
    installDir: string
  ): Promise<{
    success: boolean;
    data?: { message: string };
    error?: string;
  }> {
    try {
      const data = await this.initClusterConfigs(clusterId, hostId, installDir);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '初始化配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Push config to node (with error handling)
   * 推送配置到节点（带错误处理）
   */
  static async pushConfigToNodeSafe(
    configId: number,
    installDir: string
  ): Promise<{
    success: boolean;
    data?: { message: string };
    error?: string;
  }> {
    try {
      const data = await this.pushConfigToNode(configId, installDir);
      return { success: true, data };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '推送配置失败';
      return { success: false, error: errorMessage };
    }
  }

  /**
   * Sync template to all nodes
   * 同步模板到所有节点
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param configType - Config type / 配置类型
   * @returns Operation result / 操作结果
   */
  static async syncTemplateToAllNodes(
    clusterId: number,
    configType: string
  ): Promise<SyncAllResult> {
    const response = await apiClient.post<{ error_msg: string; data: SyncAllResult }>(
      `${this.basePath}/clusters/${clusterId}/configs/sync-all`,
      { config_type: configType }
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }
}
