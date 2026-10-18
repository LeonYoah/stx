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
 * SeaTunnel Plugin Marketplace Service
 * SeaTunnel 插件市场服务
 */

import { BaseService } from '../core/base.service';
import apiClient from '../core/api-client';
import type {
  Plugin,
  InstalledPlugin,
  AvailablePluginsResponse,
  MirrorSource,
  InstallPluginRequest,
  PluginDownloadProgress,
  LocalPlugin,
  PluginInstallStatus,
  PluginDependencyConfig,
  AddDependencyRequest,
  UploadDependencyRequest,
  DisableDependencyRequest,
  PluginDependencyDisable,
  OfficialDependenciesResponse,
  AnalyzeOfficialDependenciesRequest,
  DownloadAllPluginsRequest,
} from './types';

// Import DownloadAllPluginsProgress type / 导入下载所有插件进度类型
type DownloadAllPluginsProgress = {
  total: number;
  downloaded: number;
  failed: number;
  skipped: number;
  status: string;
  message: string;
};

/**
 * Plugin service for managing SeaTunnel plugins
 * 插件服务，用于管理 SeaTunnel 插件
 */
export class PluginService extends BaseService {
  protected static basePath = '';

  // ==================== Available Plugins 可用插件 ====================

  /**
   * List available plugins from Maven repository
   * 从 Maven 仓库获取可用插件列表
   * @param version - SeaTunnel version / SeaTunnel 版本
   * @param mirror - Mirror source / 镜像源
   * @returns Available plugins response / 可用插件响应
   */
  static async listAvailablePlugins(
    version?: string,
    mirror?: MirrorSource
  ): Promise<AvailablePluginsResponse> {
    const params: Record<string, string> = {};
    if (version) {params.version = version;}
    if (mirror) {params.mirror = mirror;}
    return this.get<AvailablePluginsResponse>('/plugins', params);
  }

  static async refreshAvailablePlugins(
    version?: string,
    mirror?: MirrorSource,
  ): Promise<AvailablePluginsResponse> {
    return this.post<AvailablePluginsResponse>(
      '/plugins/refresh',
      {
        version,
        mirror,
      },
    );
  }

  /**
   * Get plugin information by name
   * 根据名称获取插件详情
   * @param name - Plugin name / 插件名称
   * @param version - SeaTunnel version / SeaTunnel 版本
   * @returns Plugin information / 插件信息
   */
  static async getPluginInfo(name: string, version?: string): Promise<Plugin> {
    const params: Record<string, string> = {};
    if (version) {params.version = version;}
    return this.get<Plugin>(`/plugins/${encodeURIComponent(name)}`, params);
  }

  // ==================== Installed Plugins 已安装插件 ====================

  /**
   * List installed plugins on a cluster
   * 获取集群上已安装的插件列表
   * @param clusterId - Cluster ID / 集群 ID
   * @returns List of installed plugins / 已安装插件列表
   */
  static async listInstalledPlugins(clusterId: number): Promise<InstalledPlugin[]> {
    return this.get<InstalledPlugin[]>(`/clusters/${clusterId}/plugins`);
  }

  // ==================== Plugin Installation 插件安装 ====================

  /**
   * Install a plugin on a cluster
   * 在集群上安装插件
   * @param clusterId - Cluster ID / 集群 ID
   * @param pluginName - Plugin name / 插件名称
   * @param version - Plugin version / 插件版本
   * @param mirror - Mirror source / 镜像源
   * @returns Installed plugin information / 已安装插件信息
   */
  static async installPlugin(
    clusterId: number,
    pluginName: string,
    version: string,
    mirror?: MirrorSource,
    profileKeys?: string[],
  ): Promise<PluginInstallStatus> {
    const request: InstallPluginRequest = {
      plugin_name: pluginName,
      version,
      mirror,
      profile_keys: profileKeys,
    };
    // POST creates an async install task and returns immediately.
    // POST 创建异步安装任务并立即返回。
    return this.post<PluginInstallStatus>(`/clusters/${clusterId}/plugins`, request);
  }

  /**
   * Poll install progress until completed or failed.
   * 轮询安装进度直到成功或失败。
   */
  static async waitForInstall(
    clusterId: number,
    pluginName: string,
    options?: {intervalMs?: number; timeoutMs?: number},
  ): Promise<PluginInstallStatus> {
    const intervalMs = options?.intervalMs ?? 1000;
    const timeoutMs = options?.timeoutMs ?? 30 * 60 * 1000;
    const started = Date.now();
    let last: PluginInstallStatus | null = null;

    while (Date.now() - started < timeoutMs) {
      last = await this.getInstallProgress(clusterId, pluginName);
      if (last?.status === 'completed') {
        return last;
      }
      if (last?.status === 'failed') {
        throw new Error(last.error || last.message || 'Plugin installation failed');
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs));
    }

    throw new Error(
      last?.message
        ? `Plugin installation timed out: ${last.message}`
        : 'Plugin installation timed out',
    );
  }

  /**
   * Start async install and wait for terminal status (for UI flows).
   * 启动异步安装并等待终态（供 UI 流程使用）。
   */
  static async installPluginAndWait(
    clusterId: number,
    pluginName: string,
    version: string,
    mirror?: MirrorSource,
    profileKeys?: string[],
    options?: {intervalMs?: number; timeoutMs?: number},
  ): Promise<PluginInstallStatus> {
    await this.installPlugin(clusterId, pluginName, version, mirror, profileKeys);
    return this.waitForInstall(clusterId, pluginName, options);
  }

  /**
   * Uninstall a plugin from a cluster
   * 从集群卸载插件
   * @param clusterId - Cluster ID / 集群 ID
   * @param pluginName - Plugin name / 插件名称
   */
  static async uninstallPlugin(clusterId: number, pluginName: string): Promise<void> {
    await this.delete<unknown>(`/clusters/${clusterId}/plugins/${encodeURIComponent(pluginName)}`);
  }

  // ==================== Plugin Enable/Disable 插件启用/禁用 ====================

  /**
   * Enable a plugin on a cluster
   * 在集群上启用插件
   * @param clusterId - Cluster ID / 集群 ID
   * @param pluginName - Plugin name / 插件名称
   * @returns Updated plugin information / 更新后的插件信息
   */
  static async enablePlugin(clusterId: number, pluginName: string): Promise<InstalledPlugin> {
    return this.put<InstalledPlugin>(
      `/clusters/${clusterId}/plugins/${encodeURIComponent(pluginName)}/enable`
    );
  }

  /**
   * Disable a plugin on a cluster
   * 在集群上禁用插件
   * @param clusterId - Cluster ID / 集群 ID
   * @param pluginName - Plugin name / 插件名称
   * @returns Updated plugin information / 更新后的插件信息
   */
  static async disablePlugin(clusterId: number, pluginName: string): Promise<InstalledPlugin> {
    return this.put<InstalledPlugin>(
      `/clusters/${clusterId}/plugins/${encodeURIComponent(pluginName)}/disable`
    );
  }

  // ==================== Plugin Download 插件下载 ====================

  /**
   * Download a plugin to Control Plane local storage
   * 下载插件到 Control Plane 本地存储
   * @param pluginName - Plugin name / 插件名称
   * @param version - Plugin version / 插件版本
   * @param mirror - Mirror source / 镜像源
   * @returns Download progress / 下载进度
   */
  static async downloadPlugin(
    pluginName: string,
    version: string,
    mirror?: MirrorSource,
    profileKeys?: string[],
  ): Promise<PluginDownloadProgress> {
    return this.post<PluginDownloadProgress>(
      `/plugins/${encodeURIComponent(pluginName)}/download`,
      { version, mirror, profile_keys: profileKeys }
    );
  }

  /**
   * Get download status for a plugin
   * 获取插件的下载状态
   * @param pluginName - Plugin name / 插件名称
   * @param version - Plugin version / 插件版本
   * @returns Download progress / 下载进度
   */
  static async getDownloadStatus(
    pluginName: string,
    version: string,
    profileKeys?: string[],
  ): Promise<PluginDownloadProgress> {
    const params: Record<string, string | string[]> = {version};
    if (profileKeys && profileKeys.length > 0) {
      params.profile_keys = profileKeys;
    }
    return this.get<PluginDownloadProgress>(
      `/plugins/${encodeURIComponent(pluginName)}/download/status`,
      params,
    );
  }

  /**
   * List locally downloaded plugins
   * 获取已下载的本地插件列表
   * @returns List of local plugins / 本地插件列表
   */
  static async listLocalPlugins(): Promise<LocalPlugin[]> {
    return this.get<LocalPlugin[]>('/plugins/local');
  }

  /**
   * List active download tasks
   * 获取活动下载任务列表
   * @returns List of active downloads / 活动下载列表
   */
  static async listActiveDownloads(): Promise<PluginDownloadProgress[]> {
    return this.get<PluginDownloadProgress[]>('/plugins/downloads');
  }

  /**
   * Delete a locally downloaded plugin
   * 删除本地已下载的插件
   * @param pluginName - Plugin name / 插件名称
   * @param version - Plugin version / 插件版本
   */
  static async deleteLocalPlugin(pluginName: string, version: string): Promise<void> {
    await this.delete<unknown>(
      `/plugins/${encodeURIComponent(pluginName)}/local`,
      { version }
    );
  }

  /**
   * Download all plugins for a version
   * 一键下载指定版本的所有插件
   * @param version - Plugin version / 插件版本
   * @param mirror - Mirror source / 镜像源
   * @returns Download progress / 下载进度
   */
  static async downloadAllPlugins(
    version: string,
    mirror?: MirrorSource,
    selectedPluginProfiles?: Record<string, string[]>,
  ): Promise<DownloadAllPluginsProgress> {
    const request: DownloadAllPluginsRequest = {version, mirror};
    if (selectedPluginProfiles && Object.keys(selectedPluginProfiles).length > 0) {
      request.selected_plugin_profiles = selectedPluginProfiles;
    }
    return this.post<DownloadAllPluginsProgress>(
      '/plugins/download-all',
      request,
    );
  }

  // ==================== Plugin Installation Progress 插件安装进度 ====================

  /**
   * Get plugin installation progress on a cluster
   * 获取集群上插件的安装进度
   * @param clusterId - Cluster ID / 集群 ID
   * @param pluginName - Plugin name / 插件名称
   * @returns Installation progress / 安装进度
   */
  static async getInstallProgress(
    clusterId: number,
    pluginName: string
  ): Promise<PluginInstallStatus | null> {
    return this.get<PluginInstallStatus | null>(
      `/clusters/${clusterId}/plugins/${encodeURIComponent(pluginName)}/progress`
    );
  }

  // ==================== Plugin Dependency Config 插件依赖配置 ====================

  /**
   * List dependencies for a plugin
   * 获取插件的依赖列表
   * @param pluginName - Plugin name / 插件名称
   * @returns List of dependencies / 依赖列表
   */
  static async listDependencies(
    pluginName: string,
    version?: string,
  ): Promise<PluginDependencyConfig[]> {
    const params: Record<string, string> = {};
    if (version) {params.version = version;}
    return this.get<PluginDependencyConfig[]>(
      `/plugins/${encodeURIComponent(pluginName)}/dependencies`,
      params,
    );
  }

  /**
   * Add a dependency to a plugin
   * 为插件添加依赖
   * @param pluginName - Plugin name / 插件名称
   * @param dependency - Dependency info / 依赖信息
   * @returns Created dependency / 创建的依赖
   */
  static async addDependency(
    pluginName: string,
    dependency: AddDependencyRequest
  ): Promise<PluginDependencyConfig> {
    return this.post<PluginDependencyConfig>(
      `/plugins/${encodeURIComponent(pluginName)}/dependencies`,
      dependency
    );
  }

  static async uploadDependency(
    pluginName: string,
    request: UploadDependencyRequest,
  ): Promise<PluginDependencyConfig> {
    const formData = new FormData();
    formData.append('file', request.file);
    if (request.seatunnel_version) {formData.append('seatunnel_version', request.seatunnel_version);}
    if (request.group_id) {formData.append('group_id', request.group_id);}
    if (request.artifact_id) {formData.append('artifact_id', request.artifact_id);}
    if (request.version) {formData.append('version', request.version);}
    if (request.target_dir) {formData.append('target_dir', request.target_dir);}

    const response = await apiClient.post<{error_msg: string; data: PluginDependencyConfig | null}>(
      `${this.basePath}/plugins/${encodeURIComponent(pluginName)}/dependencies/upload`,
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      },
    );
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    if (!response.data.data) {
      throw new Error('上传依赖返回为空 / Empty upload dependency response');
    }
    return response.data.data;
  }

  /**
   * Delete a dependency from a plugin
   * 删除插件的依赖
   * @param pluginName - Plugin name / 插件名称
   * @param depId - Dependency ID / 依赖 ID
   */
  static async deleteDependency(pluginName: string, depId: number): Promise<void> {
    await this.delete<unknown>(
      `/plugins/${encodeURIComponent(pluginName)}/dependencies/${depId}`
    );
  }

  static async disableOfficialDependency(
    pluginName: string,
    request: DisableDependencyRequest,
  ): Promise<PluginDependencyDisable> {
    return this.post<PluginDependencyDisable>(
      `/plugins/${encodeURIComponent(pluginName)}/dependencies/disables`,
      request,
    );
  }

  static async enableOfficialDependency(
    pluginName: string,
    disableId: number,
  ): Promise<void> {
    await this.delete<unknown>(
      `/plugins/${encodeURIComponent(pluginName)}/dependencies/disables/${disableId}`,
    );
  }

  // ==================== Official Dependency Baseline 官方依赖基线 ====================

  /**
   * Get official dependencies for a plugin/version
   * 获取插件在指定版本下的官方依赖
   */
  static async getOfficialDependencies(
    pluginName: string,
    version?: string,
    profileKey?: string
  ): Promise<OfficialDependenciesResponse> {
    const params: Record<string, string> = {};
    if (version) {params.version = version;}
    if (profileKey) {params.profile_key = profileKey;}
    return this.get<OfficialDependenciesResponse>(
      `/plugins/${encodeURIComponent(pluginName)}/official-dependencies`,
      params,
    );
  }

  /**
   * Analyze official dependencies from upstream docs
   * 在线分析上游官方文档中的依赖
   */
  static async analyzeOfficialDependencies(
    pluginName: string,
    request: AnalyzeOfficialDependenciesRequest
  ): Promise<OfficialDependenciesResponse> {
    return this.post<OfficialDependenciesResponse>(
      `/plugins/${encodeURIComponent(pluginName)}/official-dependencies/analyze`,
      request,
    );
  }
}
