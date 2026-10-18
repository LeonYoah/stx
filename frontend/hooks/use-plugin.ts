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
 * Plugin Marketplace Hooks
 * 插件市场 Hooks
 */

'use client';

import {useState, useEffect, useCallback} from 'react';
import {PluginService} from '@/lib/services/plugin';
import type {
  Plugin,
  InstalledPlugin,
  PluginCategory,
  MirrorSource,
  PluginInstallStatus,
} from '@/lib/services/plugin/types';

// ==================== useAvailablePlugins Hook ====================

interface UseAvailablePluginsReturn {
  plugins: Plugin[];
  total: number;
  loading: boolean;
  error: string | null;
  version: string;
  mirror: MirrorSource;
  setVersion: (version: string) => void;
  setMirror: (mirror: MirrorSource) => void;
  refresh: () => Promise<void>;
  filterByCategory: (category: PluginCategory | null) => Plugin[];
  searchPlugins: (keyword: string) => Plugin[];
}

/**
 * Hook for fetching available plugins from Maven repository
 * 从 Maven 仓库获取可用插件的 Hook
 * @param initialVersion - Initial SeaTunnel version / 初始 SeaTunnel 版本
 * @param initialMirror - Initial mirror source / 初始镜像源
 */
export function useAvailablePlugins(
  initialVersion: string = '',
  initialMirror: MirrorSource = 'aliyun',
): UseAvailablePluginsReturn {
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(initialVersion);
  const [mirror, setMirror] = useState<MirrorSource>(initialMirror);

  const fetchPlugins = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await PluginService.listAvailablePlugins(
        version,
        mirror,
      );
      setPlugins(response.plugins || []);
      setTotal(response.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch plugins');
      setPlugins([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [version, mirror]);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  // Filter plugins by category / 按分类过滤插件
  const filterByCategory = useCallback(
    (category: PluginCategory | null): Plugin[] => {
      if (!category) {
        return plugins;
      }
      return plugins.filter((p) => p.category === category);
    },
    [plugins],
  );

  // Search plugins by keyword / 按关键词搜索插件
  const searchPlugins = useCallback(
    (keyword: string): Plugin[] => {
      if (!keyword.trim()) {
        return plugins;
      }
      const lowerKeyword = keyword.toLowerCase();
      return plugins.filter(
        (p) =>
          p.name.toLowerCase().includes(lowerKeyword) ||
          p.display_name.toLowerCase().includes(lowerKeyword) ||
          p.description?.toLowerCase().includes(lowerKeyword),
      );
    },
    [plugins],
  );

  return {
    plugins,
    total,
    loading,
    error,
    version,
    mirror,
    setVersion,
    setMirror,
    refresh: fetchPlugins,
    filterByCategory,
    searchPlugins,
  };
}

// ==================== useInstalledPlugins Hook ====================

interface UseInstalledPluginsReturn {
  plugins: InstalledPlugin[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  getPluginByName: (name: string) => InstalledPlugin | undefined;
  filterByStatus: (
    status: InstalledPlugin['status'] | null,
  ) => InstalledPlugin[];
}

/**
 * Hook for fetching installed plugins on a cluster
 * 获取集群上已安装插件的 Hook
 * @param clusterId - Cluster ID / 集群 ID
 */
export function useInstalledPlugins(
  clusterId: number | null,
): UseInstalledPluginsReturn {
  const [plugins, setPlugins] = useState<InstalledPlugin[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchPlugins = useCallback(async () => {
    if (!clusterId) {
      setPlugins([]);
      return;
    }

    try {
      setLoading(true);
      setError(null);
      const data = await PluginService.listInstalledPlugins(clusterId);
      setPlugins(data || []);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to fetch installed plugins',
      );
      setPlugins([]);
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  // Get plugin by name / 根据名称获取插件
  const getPluginByName = useCallback(
    (name: string): InstalledPlugin | undefined => {
      return plugins.find((p) => p.plugin_name === name);
    },
    [plugins],
  );

  // Filter plugins by status / 按状态过滤插件
  const filterByStatus = useCallback(
    (status: InstalledPlugin['status'] | null): InstalledPlugin[] => {
      if (!status) {
        return plugins;
      }
      return plugins.filter((p) => p.status === status);
    },
    [plugins],
  );

  return {
    plugins,
    loading,
    error,
    refresh: fetchPlugins,
    getPluginByName,
    filterByStatus,
  };
}

// ==================== usePluginInstall Hook ====================

interface UsePluginInstallReturn {
  installing: boolean;
  installStatus: PluginInstallStatus | null;
  error: string | null;
  install: (
    pluginName: string,
    version: string,
    mirror?: MirrorSource,
  ) => Promise<InstalledPlugin>;
  uninstall: (pluginName: string) => Promise<void>;
  enable: (pluginName: string) => Promise<InstalledPlugin>;
  disable: (pluginName: string) => Promise<InstalledPlugin>;
  reset: () => void;
}

/**
 * Hook for managing plugin installation operations
 * 管理插件安装操作的 Hook
 * @param clusterId - Cluster ID / 集群 ID
 * @param onSuccess - Callback on successful operation / 操作成功回调
 */
export function usePluginInstall(
  clusterId: number | null,
  onSuccess?: () => void,
): UsePluginInstallReturn {
  const [installing, setInstalling] = useState(false);
  const [installStatus, setInstallStatus] =
    useState<PluginInstallStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Install plugin / 安装插件
  const install = useCallback(
    async (
      pluginName: string,
      version: string,
      mirror?: MirrorSource,
    ): Promise<InstalledPlugin> => {
      if (!clusterId) {
        throw new Error('Cluster ID is required');
      }

      try {
        setInstalling(true);
        setError(null);
        setInstallStatus({
          plugin_name: pluginName,
          status: 'installing',
          progress: 0,
          message: 'Starting installation...',
        });

        const status = await PluginService.installPluginAndWait(
          clusterId,
          pluginName,
          version,
          mirror,
        );

        setInstallStatus({
          plugin_name: pluginName,
          status: status.status || 'completed',
          progress: status.progress ?? 100,
          message: status.message || 'Installation completed',
          error: status.error,
          task_id: status.task_id,
          version: status.version,
        });

        onSuccess?.();
        // Async API returns progress; callers that need InstalledPlugin should refresh list.
        // 异步 API 返回进度；需要 InstalledPlugin 的调用方应刷新列表。
        return {
          id: 0,
          cluster_id: clusterId,
          plugin_name: pluginName,
          artifact_id: '',
          category: 'connector',
          version,
          status: 'installed',
          install_path: '',
          installed_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as InstalledPlugin;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Installation failed';
        setError(message);
        setInstallStatus({
          plugin_name: pluginName,
          status: 'failed',
          progress: 0,
          error: message,
        });
        throw err;
      } finally {
        setInstalling(false);
      }
    },
    [clusterId, onSuccess],
  );

  // Uninstall plugin / 卸载插件
  const uninstall = useCallback(
    async (pluginName: string): Promise<void> => {
      if (!clusterId) {
        throw new Error('Cluster ID is required');
      }

      try {
        setInstalling(true);
        setError(null);
        await PluginService.uninstallPlugin(clusterId, pluginName);
        onSuccess?.();
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Uninstall failed';
        setError(message);
        throw err;
      } finally {
        setInstalling(false);
      }
    },
    [clusterId, onSuccess],
  );

  // Enable plugin / 启用插件
  const enable = useCallback(
    async (pluginName: string): Promise<InstalledPlugin> => {
      if (!clusterId) {
        throw new Error('Cluster ID is required');
      }

      try {
        setInstalling(true);
        setError(null);
        const result = await PluginService.enablePlugin(clusterId, pluginName);
        onSuccess?.();
        return result;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Enable failed';
        setError(message);
        throw err;
      } finally {
        setInstalling(false);
      }
    },
    [clusterId, onSuccess],
  );

  // Disable plugin / 禁用插件
  const disable = useCallback(
    async (pluginName: string): Promise<InstalledPlugin> => {
      if (!clusterId) {
        throw new Error('Cluster ID is required');
      }

      try {
        setInstalling(true);
        setError(null);
        const result = await PluginService.disablePlugin(clusterId, pluginName);
        onSuccess?.();
        return result;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Disable failed';
        setError(message);
        throw err;
      } finally {
        setInstalling(false);
      }
    },
    [clusterId, onSuccess],
  );

  // Reset state / 重置状态
  const reset = useCallback(() => {
    setInstalling(false);
    setInstallStatus(null);
    setError(null);
  }, []);

  return {
    installing,
    installStatus,
    error,
    install,
    uninstall,
    enable,
    disable,
    reset,
  };
}

// ==================== usePluginDetail Hook ====================

interface UsePluginDetailReturn {
  plugin: Plugin | null;
  loading: boolean;
  error: string | null;
  fetch: (name: string, version?: string) => Promise<Plugin>;
  reset: () => void;
}

/**
 * Hook for fetching plugin details
 * 获取插件详情的 Hook
 */
export function usePluginDetail(): UsePluginDetailReturn {
  const [plugin, setPlugin] = useState<Plugin | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetch = useCallback(
    async (name: string, version?: string): Promise<Plugin> => {
      try {
        setLoading(true);
        setError(null);
        const data = await PluginService.getPluginInfo(name, version);
        setPlugin(data);
        return data;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Failed to fetch plugin details';
        setError(message);
        throw err;
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  const reset = useCallback(() => {
    setPlugin(null);
    setError(null);
  }, []);

  return {
    plugin,
    loading,
    error,
    fetch,
    reset,
  };
}

// ==================== Combined usePluginMarketplace Hook ====================

interface UsePluginMarketplaceReturn {
  // Available plugins / 可用插件
  availablePlugins: Plugin[];
  availableTotal: number;
  availableLoading: boolean;
  availableError: string | null;
  version: string;
  mirror: MirrorSource;
  setVersion: (version: string) => void;
  setMirror: (mirror: MirrorSource) => void;
  refreshAvailable: () => Promise<void>;

  // Installed plugins / 已安装插件
  installedPlugins: InstalledPlugin[];
  installedLoading: boolean;
  installedError: string | null;
  refreshInstalled: () => Promise<void>;

  // Operations / 操作
  installing: boolean;
  installError: string | null;
  installPlugin: (pluginName: string) => Promise<InstalledPlugin>;
  uninstallPlugin: (pluginName: string) => Promise<void>;
  enablePlugin: (pluginName: string) => Promise<InstalledPlugin>;
  disablePlugin: (pluginName: string) => Promise<InstalledPlugin>;

  // Helpers / 辅助方法
  isInstalled: (pluginName: string) => boolean;
  getInstalledPlugin: (pluginName: string) => InstalledPlugin | undefined;
  filterByCategory: (category: PluginCategory | null) => Plugin[];
  searchPlugins: (keyword: string) => Plugin[];
}

/**
 * Combined hook for plugin marketplace functionality
 * 插件市场功能的组合 Hook
 * @param clusterId - Cluster ID for installed plugins / 已安装插件的集群 ID
 * @param initialVersion - Initial SeaTunnel version / 初始 SeaTunnel 版本
 * @param initialMirror - Initial mirror source / 初始镜像源
 */
export function usePluginMarketplace(
  clusterId: number | null,
  initialVersion: string = '',
  initialMirror: MirrorSource = 'aliyun',
): UsePluginMarketplaceReturn {
  // Available plugins state / 可用插件状态
  const {
    plugins: availablePlugins,
    total: availableTotal,
    loading: availableLoading,
    error: availableError,
    version,
    mirror,
    setVersion,
    setMirror,
    refresh: refreshAvailable,
    filterByCategory,
    searchPlugins,
  } = useAvailablePlugins(initialVersion, initialMirror);

  // Installed plugins state / 已安装插件状态
  const {
    plugins: installedPlugins,
    loading: installedLoading,
    error: installedError,
    refresh: refreshInstalled,
    getPluginByName,
  } = useInstalledPlugins(clusterId);

  // Install operations / 安装操作
  const {
    installing,
    error: installError,
    install,
    uninstall,
    enable,
    disable,
  } = usePluginInstall(clusterId, () => {
    // Refresh installed plugins after operation / 操作后刷新已安装插件
    refreshInstalled();
  });

  // Install plugin with current version and mirror / 使用当前版本和镜像安装插件
  const installPlugin = useCallback(
    async (pluginName: string): Promise<InstalledPlugin> => {
      return install(pluginName, version, mirror);
    },
    [install, version, mirror],
  );

  // Check if plugin is installed / 检查插件是否已安装
  const isInstalled = useCallback(
    (pluginName: string): boolean => {
      return installedPlugins.some((p) => p.plugin_name === pluginName);
    },
    [installedPlugins],
  );

  return {
    // Available plugins / 可用插件
    availablePlugins,
    availableTotal,
    availableLoading,
    availableError,
    version,
    mirror,
    setVersion,
    setMirror,
    refreshAvailable,

    // Installed plugins / 已安装插件
    installedPlugins,
    installedLoading,
    installedError,
    refreshInstalled,

    // Operations / 操作
    installing,
    installError,
    installPlugin,
    uninstallPlugin: uninstall,
    enablePlugin: enable,
    disablePlugin: disable,

    // Helpers / 辅助方法
    isInstalled,
    getInstalledPlugin: getPluginByName,
    filterByCategory,
    searchPlugins,
  };
}
