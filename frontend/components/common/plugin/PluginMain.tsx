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
 * Plugin Marketplace Main Component
 * 插件市场主组件
 */

'use client';

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Card,
  CardContent,
} from '@/components/ui/card';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import {toast} from 'sonner';
import {
  RefreshCw,
  Search,
  Puzzle,
  Package,
  HardDrive,
  Trash2,
  CheckSquare,
  Server,
  Upload,
  DownloadCloud,
} from 'lucide-react';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
import {
  WorkspaceHeader,
  StatPillsBar,
  type StatPillItem,
} from '@/components/common/layout';
import {PluginService} from '@/lib/services/plugin';
import {usePackages} from '@/hooks/use-installer';
import {resolveSeatunnelVersion} from '@/lib/seatunnel-version';
import {ClusterService} from '@/lib/services/cluster';
import type {
  Plugin,
  MirrorSource,
  AvailablePluginsResponse,
  LocalPlugin,
  PluginDownloadProgress,
  InstalledPlugin,
  PluginDependency,
  OfficialDependenciesResponse,
} from '@/lib/services/plugin';
import type {ClusterInfo} from '@/lib/services/cluster';
import {Progress} from '@/components/ui/progress';
import {matchesConnectorProcessingMode} from './connector-quick-helper';
import {PluginGrid} from './PluginGrid';
import {PluginDetailDialog} from './PluginDetailDialog';
import {InstallPluginDialog} from './InstallPluginDialog';
import {BatchInstallDialog} from './BatchInstallDialog';
import {Pagination} from '@/components/ui/pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';


interface InstallDialogContext {
  plugin: Plugin;
  version: string;
  selectedProfileKeys: string[];
  attachedConnectors: string[];
  dependencies: PluginDependency[];
}

function normalizeProfileKeys(keys?: string[]): string[] {
  return Array.from(new Set((keys || []).map((item) => item.trim()).filter(Boolean))).sort();
}

function splitAutomaticAttachments(dependencies?: PluginDependency[]) {
  const items = dependencies || [];
  return {
    attachedConnectors: Array.from(
      new Set(
        items
          .filter((item) => item.target_dir === 'connectors')
          .map((item) => item.artifact_id),
      ),
    ),
    dependencies: items,
  };
}

function pluginDownloadKey(name: string, version: string): string {
  return `${name}:${version}`;
}

function pluginClusterInstallKey(name: string, version: string): string {
  return `${name}:${version}`;
}

/**
 * Plugin Marketplace Main Component
 * 插件市场主组件
 */
export function PluginMain() {
  const t = useTranslations();

  // Data state / 数据状态
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshingConnectors, setRefreshingConnectors] = useState(false);
  const [catalogRefreshedAt, setCatalogRefreshedAt] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  // Filter state / 过滤状态
  const [searchKeyword, setSearchKeyword] = useState('');
  const [filterCategory, setFilterCategory] = useState<string>('all');
  const [selectedMirror, setSelectedMirror] = useState<MirrorSource>('aliyun');
  const [selectedVersion, setSelectedVersion] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'available' | 'local' | 'custom'>(
    'available',
  );
  const {packages} = usePackages();
  const availableVersions = packages?.versions || [];
  const recommendedVersion = resolveSeatunnelVersion(packages);

  // Local plugins state / 本地插件状态
  const [localPlugins, setLocalPlugins] = useState<LocalPlugin[]>([]);
  const [localPluginsLoading, setLocalPluginsLoading] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [pluginToDelete, setPluginToDelete] = useState<LocalPlugin | null>(
    null,
  );
  const [deleteUninstallClusterIds, setDeleteUninstallClusterIds] = useState<
    Set<number>
  >(new Set());
  const [isDeletingLocalPlugin, setIsDeletingLocalPlugin] = useState(false);

  // Local plugins pagination state / 本地插件分页状态
  const [localPluginsPage, setLocalPluginsPage] = useState(1);
  const [localPluginsPageSize, setLocalPluginsPageSize] = useState(10);

  // Dialog state / 对话框状态
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [isInstallOpen, setIsInstallOpen] = useState(false);
  const [installContext, setInstallContext] = useState<InstallDialogContext | null>(null);
  const [selectedProfileKeysByPlugin, setSelectedProfileKeysByPlugin] = useState<Record<string, string[]>>({});
  const [isBatchProfileDialogOpen, setIsBatchProfileDialogOpen] = useState(false);
  const [batchProfilePlugins, setBatchProfilePlugins] = useState<Plugin[]>([]);
  const [batchProfileOfficialDeps, setBatchProfileOfficialDeps] = useState<
    Record<string, OfficialDependenciesResponse | null>
  >({});
  const [batchProfileLoading, setBatchProfileLoading] = useState(false);
  const [activeBatchProfileTab, setActiveBatchProfileTab] = useState('');

  // Download state / 下载状态
  const [downloadingPlugins, setDownloadingPlugins] = useState<Set<string>>(
    new Set(),
  );
  const [downloadedPlugins, setDownloadedPlugins] = useState<Set<string>>(
    new Set(),
  );
  const [isDownloadingAll, setIsDownloadingAll] = useState(false);

  // Active downloads state / 活动下载状态
  const [activeDownloads, setActiveDownloads] = useState<
    PluginDownloadProgress[]
  >([]);

  // Batch selection state / 批量选择状态
  const [selectedLocalPlugins, setSelectedLocalPlugins] = useState<Set<string>>(
    new Set(),
  );
  const [isBatchInstallOpen, setIsBatchInstallOpen] = useState(false);

  const lastLoadedAvailableQueryRef = useRef<string | null>(null);

  // Plugin installation status per cluster / 每个集群的插件安装状态
  // Map: pluginName:version -> { clusterId -> InstalledPlugin }
  const [pluginClusterStatus, setPluginClusterStatus] = useState<
    Map<string, Map<number, InstalledPlugin>>
  >(new Map());
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);

  /**
   * Load available plugins
   * 加载可用插件列表
   */
  const buildAvailableQueryKey = useCallback(
    (version: string) => `${version || 'default'}`,
    [],
  );

  const filteredAvailablePlugins = useMemo(() => {
    let filtered = plugins;

    if (filterCategory !== 'all') {
      filtered = filtered.filter(
        (plugin) =>
          matchesConnectorProcessingMode(
            plugin.name,
            filterCategory,
            plugin.category,
          ),
      );
    }

    if (searchKeyword) {
      const keyword = searchKeyword.toLowerCase();
      filtered = filtered.filter(
        (plugin) =>
          plugin.name.toLowerCase().includes(keyword) ||
          plugin.display_name.toLowerCase().includes(keyword) ||
          plugin.description.toLowerCase().includes(keyword),
      );
    }

    return filtered;
  }, [filterCategory, plugins, searchKeyword]);

  const loadPlugins = useCallback(
    async (options?: {force?: boolean}) => {
      const requestVersion = selectedVersion || recommendedVersion || '';
      const queryKey = buildAvailableQueryKey(requestVersion);

      if (!options?.force && lastLoadedAvailableQueryRef.current === queryKey) {
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const result: AvailablePluginsResponse =
          await PluginService.listAvailablePlugins(
            requestVersion || undefined,
            selectedMirror,
          );

        setPlugins(result.plugins || []);
        setTotal(result.total || (result.plugins || []).length);
        setCatalogRefreshedAt(result.catalog_refreshed_at || '');
        lastLoadedAvailableQueryRef.current = buildAvailableQueryKey(
          result.version || requestVersion,
        );

        if (result.version && result.version !== selectedVersion) {
          setSelectedVersion(result.version);
        }

        if (
          result.source === 'remote' &&
          !result.cache_hit &&
          (result.total || 0) > 0
        ) {
          toast.info(t('plugin.loadedFromMaven'), {
            description: t('plugin.loadedFromMavenDesc'),
          });
        }
      } catch (err) {
        const errorMsg =
          err instanceof Error ? err.message : t('plugin.loadError');
        setError(errorMsg);
        toast.error(errorMsg);
        setPlugins([]);
        setTotal(0);
        lastLoadedAvailableQueryRef.current = null;
      } finally {
        setLoading(false);
      }
    },
    [
      buildAvailableQueryKey,
      recommendedVersion,
      selectedMirror,
      selectedVersion,
      t,
    ],
  );

  /**
   * Load local downloaded plugins and their cluster installation status
   * 加载本地已下载插件列表及其集群安装状态
   */
  const loadLocalPlugins = useCallback(async () => {
    setLocalPluginsLoading(true);
    try {
      // Load local plugins, active downloads, and clusters in parallel / 并行加载本地插件、活动下载和集群
      const [localResult, downloadsResult, clustersResult] = await Promise.all([
        PluginService.listLocalPlugins(),
        PluginService.listActiveDownloads(),
        ClusterService.getClusters({current: 1, size: 100}),
      ]);

      setLocalPlugins(localResult || []);
      setDownloadedPlugins(
        new Set(
          (localResult || []).map((item) => pluginDownloadKey(item.name, item.version)),
        ),
      );
      setActiveDownloads(downloadsResult || []);

      // Filter available clusters / 过滤可用集群
      const availableClusters = (clustersResult?.clusters || []).filter(
        (c: ClusterInfo) => c.status === 'running' || c.status === 'stopped',
      );
      setClusters(availableClusters);

      // Update downloadingPlugins set / 更新下载中集合
      const downloading = new Set(
        downloadsResult
          ?.filter((d) => d.status === 'downloading')
          .map((d) => pluginDownloadKey(d.plugin_name, d.version)) || [],
      );
      setDownloadingPlugins(downloading);

      // Load installed plugins for each cluster / 加载每个集群的已安装插件
      const statusMap = new Map<string, Map<number, InstalledPlugin>>();
      await Promise.all(
        availableClusters.map(async (cluster: ClusterInfo) => {
          try {
            const installedPlugins = await PluginService.listInstalledPlugins(
              cluster.id,
            );
            for (const plugin of installedPlugins) {
              const statusKey = pluginClusterInstallKey(
                plugin.plugin_name,
                plugin.version,
              );
              if (!statusMap.has(statusKey)) {
                statusMap.set(statusKey, new Map());
              }
              statusMap.get(statusKey)!.set(cluster.id, plugin);
            }
          } catch {
            // Ignore errors / 忽略错误
          }
        }),
      );
      setPluginClusterStatus(statusMap);
    } catch (err) {
      console.error('Failed to load local plugins:', err);
      setLocalPlugins([]);
    } finally {
      setLocalPluginsLoading(false);
    }
  }, []);

  const getSelectedProfileKeysForPlugin = useCallback(
    (pluginName: string) => selectedProfileKeysByPlugin[pluginName] || [],
    [selectedProfileKeysByPlugin],
  );

  const setSelectedProfileKeysForPlugin = useCallback(
    (pluginName: string, keys: string[]) => {
      setSelectedProfileKeysByPlugin((prev) => ({
        ...prev,
        [pluginName]: normalizeProfileKeys(keys),
      }));
    },
    [],
  );

  const requiresProfileSelection = useCallback((plugin: Plugin) => {
    return plugin.artifact_id === 'connector-jdbc' || plugin.name === 'jdbc';
  }, []);

  const applyDefaultBatchProfiles = useCallback(
    (requiredPlugins: Plugin[], officialDepsMap: Record<string, OfficialDependenciesResponse | null>) => {
      setSelectedProfileKeysByPlugin((prev) => {
        const next = {...prev};
        requiredPlugins.forEach((plugin) => {
          if ((next[plugin.name] || []).length > 0) {
            return;
          }
          const profiles = officialDepsMap[plugin.name]?.profiles || [];
          next[plugin.name] = normalizeProfileKeys(
            profiles.map((profile) => profile.profile_key),
          );
        });
        return next;
      });
    },
    [],
  );

  const openBatchProfileDialog = useCallback(async () => {
    const requiredPlugins = plugins.filter(requiresProfileSelection);
    if (requiredPlugins.length === 0) {
      return false;
    }

    setBatchProfilePlugins(requiredPlugins);
    setActiveBatchProfileTab(requiredPlugins[0]?.name || '');
    setBatchProfileOfficialDeps({});
    setBatchProfileLoading(true);
    setIsBatchProfileDialogOpen(true);

    try {
      const version = selectedVersion || recommendedVersion || '';
      const entries = await Promise.all(
        requiredPlugins.map(async (plugin) => {
          const data = await PluginService.getOfficialDependencies(
            plugin.name,
            version || plugin.version,
          );
          return [plugin.name, data] as const;
        }),
      );
      const nextMap = Object.fromEntries(entries);
      setBatchProfileOfficialDeps(nextMap);
      applyDefaultBatchProfiles(requiredPlugins, nextMap);
      return true;
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : t('plugin.loadError');
      toast.error(errorMsg);
      setIsBatchProfileDialogOpen(false);
      return false;
    } finally {
      setBatchProfileLoading(false);
    }
  }, [
    applyDefaultBatchProfiles,
    plugins,
    recommendedVersion,
    requiresProfileSelection,
    selectedVersion,
    t,
  ]);

  const executeDownloadAllPlugins = useCallback(
    async (selectedProfiles?: Record<string, string[]>) => {
      try {
        setIsDownloadingAll(true);
        toast.info(t('plugin.downloadAllStarted', {count: total}));

        const result = await PluginService.downloadAllPlugins(
          selectedVersion,
          selectedMirror,
          selectedProfiles,
        );

        toast.success(
          t('plugin.downloadAllSuccess', {
            total: result.total,
            downloaded: result.downloaded,
            skipped: result.skipped,
          }),
        );
        void loadLocalPlugins();
      } catch (err) {
        const errorMsg =
          err instanceof Error ? err.message : t('plugin.downloadAllFailed');
        toast.error(errorMsg);
      } finally {
        setIsDownloadingAll(false);
      }
    },
    [loadLocalPlugins, selectedMirror, selectedVersion, t, total],
  );

  const handleConfirmBatchProfileDownload = useCallback(async () => {
    const selectedProfiles = batchProfilePlugins.reduce<Record<string, string[]>>(
      (acc, plugin) => {
        const keys = normalizeProfileKeys(selectedProfileKeysByPlugin[plugin.name]);
        if (keys.length > 0) {
          acc[plugin.name] = keys;
        }
        return acc;
      },
      {},
    );
    setIsBatchProfileDialogOpen(false);
    await executeDownloadAllPlugins(selectedProfiles);
  }, [batchProfilePlugins, executeDownloadAllPlugins, selectedProfileKeysByPlugin]);

  const batchProfileDialogIncomplete = useMemo(
    () =>
      batchProfilePlugins.some(
        (plugin) => normalizeProfileKeys(selectedProfileKeysByPlugin[plugin.name]).length === 0,
      ),
    [batchProfilePlugins, selectedProfileKeysByPlugin],
  );

  const openInstallDialog = useCallback(
    (
      plugin: Plugin,
      version: string,
      selectedProfileKeys: string[],
      dependencies: PluginDependency[],
      attachedConnectors: string[],
    ) => {
      setInstallContext({
        plugin,
        version,
        selectedProfileKeys: normalizeProfileKeys(selectedProfileKeys),
        dependencies,
        attachedConnectors,
      });
      setIsInstallOpen(true);
    },
    [],
  );

  useEffect(() => {
    if (activeTab !== 'available') {
      return;
    }
    void loadPlugins();
  }, [activeTab, loadPlugins]);

  useEffect(() => {
    void PluginService.listLocalPlugins()
      .then((items) => {
        setDownloadedPlugins(
          new Set(
            (items || []).map((item) => pluginDownloadKey(item.name, item.version)),
          ),
        );
      })
      .catch(() => {
        // ignore preload errors
      });
  }, [selectedVersion]);

  useEffect(() => {
    if (!selectedVersion && recommendedVersion) {
      setSelectedVersion(recommendedVersion);
    }
  }, [selectedVersion, recommendedVersion]);

  // Load local plugins when switching to local tab / 切换到本地插件标签时加载
  useEffect(() => {
    if (activeTab === 'local') {
      loadLocalPlugins();
    }
  }, [activeTab, loadLocalPlugins]);

  // Poll for active downloads when there are downloading plugins / 有下载中的插件时轮询
  useEffect(() => {
    if (
      activeTab !== 'local' ||
      activeDownloads.filter((d) => d.status === 'downloading').length === 0
    ) {
      return;
    }

    const interval = setInterval(() => {
      loadLocalPlugins();
    }, 2000); // Poll every 2 seconds / 每2秒轮询一次

    return () => clearInterval(interval);
  }, [activeTab, activeDownloads, loadLocalPlugins]);

  /**
   * Handle search
   * 处理搜索
   */
  const handleSearch = () => {
    if (activeTab === 'local') {
      setLocalPluginsPage(1);
    }
  };

  /**
   * Handle refresh
   * 处理刷新
   */
  const handleRefresh = () => {
    if (activeTab === 'local') {
      loadLocalPlugins();
    } else {
      const requestVersion = selectedVersion || recommendedVersion || '';
      setRefreshingConnectors(true);
      setError(null);
      void PluginService.refreshAvailablePlugins(
        requestVersion || undefined,
        selectedMirror,
      )
        .then((result) => {
          setPlugins(result.plugins || []);
          setTotal(result.total || (result.plugins || []).length);
          setCatalogRefreshedAt(result.catalog_refreshed_at || '');
          lastLoadedAvailableQueryRef.current = buildAvailableQueryKey(
            result.version || requestVersion,
          );
          toast.success(t('plugin.refreshConnectorsSuccess'));
        })
        .catch((err) => {
          const errorMsg =
            err instanceof Error ? err.message : t('plugin.refreshConnectorsFailed');
          setError(errorMsg);
          toast.error(errorMsg);
        })
        .finally(() => {
          setRefreshingConnectors(false);
        });
    }
  };

  /**
   * 获取本地插件已安装到的集群列表
   * Get clusters where a local plugin is installed
   */
  const getInstalledClustersForLocalPlugin = useCallback(
    (plugin: LocalPlugin) => {
      const clusterStatusMap = pluginClusterStatus.get(
        pluginClusterInstallKey(plugin.name, plugin.version),
      );

      if (!clusterStatusMap) {
        return [];
      }

      return clusters.filter((cluster) => clusterStatusMap.has(cluster.id));
    },
    [clusters, pluginClusterStatus],
  );

  const deleteDialogInstalledClusters = useMemo(
    () =>
      pluginToDelete ? getInstalledClustersForLocalPlugin(pluginToDelete) : [],
    [getInstalledClustersForLocalPlugin, pluginToDelete],
  );

  /**
   * 打开本地插件删除确认对话框
   * Open local plugin deletion dialog
   */
  const openDeleteLocalPluginDialog = useCallback((plugin: LocalPlugin) => {
    setPluginToDelete(plugin);
    // 删除本地文件默认不卸载集群插件，避免误删运行环境资产。
    // Local deletion does not uninstall cluster plugins by default to avoid accidental runtime asset removal.
    setDeleteUninstallClusterIds(new Set());
    setDeleteDialogOpen(true);
  }, []);

  /**
   * 处理删除对话框开关状态变化
   * Handle delete dialog open state changes
   */
  const handleDeleteDialogOpenChange = useCallback(
    (open: boolean) => {
      if (isDeletingLocalPlugin && !open) {
        return;
      }
      setDeleteDialogOpen(open);
      if (!open) {
        setPluginToDelete(null);
        setDeleteUninstallClusterIds(new Set());
      }
    },
    [isDeletingLocalPlugin],
  );

  /**
   * 切换删除前是否从某个集群卸载插件
   * Toggle whether to uninstall the plugin from one cluster before deletion
   */
  const handleToggleDeleteUninstallCluster = useCallback(
    (clusterId: number, checked: boolean) => {
      setDeleteUninstallClusterIds((prev) => {
        const next = new Set(prev);
        if (checked) {
          next.add(clusterId);
        } else {
          next.delete(clusterId);
        }
        return next;
      });
    },
    [],
  );

  /**
   * 切换删除对话框中的全部已安装集群
   * Toggle all installed clusters in the deletion dialog
   */
  const handleToggleAllDeleteUninstallClusters = useCallback(
    (checked: boolean) => {
      if (!checked) {
        setDeleteUninstallClusterIds(new Set());
        return;
      }

      setDeleteUninstallClusterIds(
        new Set(deleteDialogInstalledClusters.map((cluster) => cluster.id)),
      );
    },
    [deleteDialogInstalledClusters],
  );

  /**
   * 处理删除本地插件以及可选的集群卸载
   * Handle delete local plugin and optional cluster uninstalls
   */
  const handleDeleteLocalPlugin = async () => {
    if (!pluginToDelete) {
      return;
    }

    setIsDeletingLocalPlugin(true);
    try {
      const selectedClusterIds = Array.from(deleteUninstallClusterIds);
      if (selectedClusterIds.length > 0) {
        // 先卸载集群插件，成功后再删除本地文件，避免本地文件已删但集群卸载失败。
        // Uninstall from clusters first, then delete local files to avoid losing local files when cluster uninstall fails.
        await Promise.all(
          selectedClusterIds.map((clusterId) =>
            PluginService.uninstallPlugin(clusterId, pluginToDelete.name),
          ),
        );
      }

      await PluginService.deleteLocalPlugin(
        pluginToDelete.name,
        pluginToDelete.version,
      );
      toast.success(
        selectedClusterIds.length > 0
          ? t('plugin.deleteWithUninstallSuccess', {
              count: selectedClusterIds.length,
            })
          : t('plugin.deleteSuccess'),
      );
      await loadLocalPlugins();
      // Also remove from downloadedPlugins set / 同时从已下载集合中移除
      setDownloadedPlugins((prev) => {
        const next = new Set(prev);
        next.delete(pluginDownloadKey(pluginToDelete.name, pluginToDelete.version));
        return next;
      });
      setDeleteDialogOpen(false);
      setPluginToDelete(null);
      setDeleteUninstallClusterIds(new Set());
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : t('plugin.deleteFailed');
      toast.error(errorMsg);
    } finally {
      setIsDeletingLocalPlugin(false);
    }
  };

  /**
   * Format file size
   * 格式化文件大小
   */
  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) {
      return '0 B';
    }
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  /**
   * Get filtered local plugins
   * 获取过滤后的本地插件
   */
  const getFilteredLocalPlugins = useCallback(() => {
    return localPlugins.filter((plugin) => {
      if (searchKeyword) {
        const keyword = searchKeyword.toLowerCase();
        if (!plugin.name.toLowerCase().includes(keyword)) {
          return false;
        }
      }
      if (
        filterCategory !== 'all' &&
        !matchesConnectorProcessingMode(
          plugin.name,
          filterCategory,
          plugin.category,
        )
      ) {
        return false;
      }
      if (selectedVersion && plugin.version !== selectedVersion) {
        return false;
      }
      return true;
    });
  }, [localPlugins, searchKeyword, filterCategory, selectedVersion]);

  /**
   * Get paginated local plugins
   * 获取分页后的本地插件
   */
  const getPaginatedLocalPlugins = useCallback(() => {
    const filtered = getFilteredLocalPlugins();
    const startIndex = (localPluginsPage - 1) * localPluginsPageSize;
    const endIndex = startIndex + localPluginsPageSize;
    return filtered.slice(startIndex, endIndex);
  }, [getFilteredLocalPlugins, localPluginsPage, localPluginsPageSize]);

  /**
   * Get total pages for local plugins
   * 获取本地插件总页数
   */
  const getLocalPluginsTotalPages = useCallback(() => {
    const filtered = getFilteredLocalPlugins();
    return Math.ceil(filtered.length / localPluginsPageSize);
  }, [getFilteredLocalPlugins, localPluginsPageSize]);

  // Reset page when filters change / 过滤条件变化时重置页码
  useEffect(() => {
    setLocalPluginsPage(1);
  }, [searchKeyword, filterCategory, selectedVersion]);

  /**
   * Handle select all local plugins
   * 处理全选本地插件
   */
  const handleSelectAllLocalPlugins = (checked: boolean) => {
    if (checked) {
      const filteredPlugins = getFilteredLocalPlugins();
      const allKeys = new Set(
        filteredPlugins.map((p) => `${p.name}:${p.version}`),
      );
      setSelectedLocalPlugins(allKeys);
    } else {
      setSelectedLocalPlugins(new Set());
    }
  };

  /**
   * Handle select single local plugin
   * 处理选择单个本地插件
   */
  const handleSelectLocalPlugin = (plugin: LocalPlugin, checked: boolean) => {
    const key = `${plugin.name}:${plugin.version}`;
    setSelectedLocalPlugins((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(key);
      } else {
        next.delete(key);
      }
      return next;
    });
  };

  /**
   * Get selected plugins for batch install
   * 获取批量安装的选中插件
   */
  const getSelectedPluginsForBatchInstall = (): Plugin[] => {
    return localPlugins
      .filter((p) => selectedLocalPlugins.has(`${p.name}:${p.version}`))
      .map((p) => ({
        name: p.name,
        display_name: p.name,
        category: p.category,
        version: p.version,
        description: '',
        group_id: 'org.apache.seatunnel',
        artifact_id: p.artifact_id || `connector-${p.name}`,
        dependencies: p.dependencies || [],
      }));
  };

  /**
   * Handle view plugin detail
   * 处理查看插件详情
   */
  const handleViewDetail = (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    setIsDetailOpen(true);
  };

  /**
   * Clear all filters
   * 清除所有过滤条件
   */
  const handleClearFilters = () => {
    setSearchKeyword('');
    setFilterCategory('all');
  };

  /**
   * Handle install plugin
   * 处理安装插件
   */
  const handleInstallPlugin = (plugin: Plugin) => {
    const selectedProfileKeys = getSelectedProfileKeysForPlugin(plugin.name);
    if (requiresProfileSelection(plugin) && selectedProfileKeys.length === 0) {
      setSelectedPlugin(plugin);
      setIsDetailOpen(true);
      toast.info(t('plugin.selectScenarioFirst'));
      return;
    }

    const {attachedConnectors, dependencies} = splitAutomaticAttachments(
      plugin.dependencies || [],
    );
    openInstallDialog(
      plugin,
      selectedVersion || plugin.version,
      selectedProfileKeys,
      dependencies,
      attachedConnectors,
    );
  };

  /**
   * Handle download plugin
   * 处理下载插件到 Control Plane
   */
  const handleDownloadPlugin = async (plugin: Plugin, profileKeys?: string[]) => {
    const pluginVersion = selectedVersion || plugin.version;
    const downloadKey = pluginDownloadKey(plugin.name, pluginVersion);
    const selectedProfileKeys = normalizeProfileKeys(
      profileKeys ?? getSelectedProfileKeysForPlugin(plugin.name),
    );

    if (requiresProfileSelection(plugin) && selectedProfileKeys.length === 0) {
      setSelectedPlugin(plugin);
      setIsDetailOpen(true);
      toast.info(t('plugin.selectScenarioFirst'));
      return;
    }

    setSelectedProfileKeysForPlugin(plugin.name, selectedProfileKeys);

    try {
      setDownloadingPlugins((prev) => new Set(prev).add(downloadKey));

      await PluginService.downloadPlugin(
        plugin.name,
        pluginVersion,
        selectedMirror,
        selectedProfileKeys,
      );

      toast.success(t('plugin.downloadStarted'));

      setDownloadingPlugins((prev) => {
        const next = new Set(prev);
        next.delete(downloadKey);
        return next;
      });
      void loadLocalPlugins();
    } catch (err) {
      setDownloadingPlugins((prev) => {
        const next = new Set(prev);
        next.delete(downloadKey);
        return next;
      });

      const errorMsg =
        err instanceof Error ? err.message : t('plugin.downloadFailed');
      toast.error(errorMsg);
    }
  };

  /**
   * Handle download all plugins
   * 处理一键下载所有插件
   */
  const handleDownloadAllPlugins = async () => {
    const requiredPlugins = plugins.filter(requiresProfileSelection);
    if (requiredPlugins.length > 0) {
      await openBatchProfileDialog();
      return;
    }
    await executeDownloadAllPlugins();
  };

  // Count plugins by category is no longer needed since all are connectors
  // 不再需要按分类统计，因为所有插件都是连接器

  const containerVariants = {
    hidden: {opacity: 0},
    visible: {
      opacity: 1,
      transition: {
        duration: 0.5,
        staggerChildren: 0.1,
        ease: easeOut,
      },
    },
  };

  const itemVariants = {
    hidden: {opacity: 0, y: 20},
    visible: {
      opacity: 1,
      y: 0,
      transition: {duration: 0.6, ease: easeOut},
    },
  };

  const formatCatalogRefreshedAt = useCallback((value: string) => {
    if (!value) {
      return '-';
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return value;
    }
    return date.toLocaleString();
  }, []);

  // 胶囊状态栏数据项 / Stat pills items data model
  const statItems: StatPillItem[] = useMemo(() => {
    const downloadingCount = activeDownloads.filter(
      (d) => d.status === 'downloading',
    ).length;
    const localCount = getFilteredLocalPlugins().length;
    const localCountDisplay =
      downloadingCount > 0 ? `${localCount}+${downloadingCount}` : localCount;

    return [
      {
        key: 'available',
        label: t('plugin.available'),
        count: total,
        icon: <Package className='h-3.5 w-3.5' />,
        variant: 'default',
        dataTestId: 'plugin-tab-available',
      },
      {
        key: 'local',
        label: t('plugin.localPlugins'),
        count: localCountDisplay,
        icon: <HardDrive className='h-3.5 w-3.5' />,
        variant: downloadingCount > 0 ? 'info' : 'default',
        pulse: downloadingCount > 0,
        dataTestId: 'plugin-tab-local',
      },
      {
        key: 'custom',
        label: t('plugin.custom'),
        icon: <Upload className='h-3.5 w-3.5' />,
        variant: 'default',
        dataTestId: 'plugin-tab-custom',
      },
    ];
  }, [activeDownloads, getFilteredLocalPlugins, t, total]);

  return (
    <motion.div
      data-testid='plugin-marketplace-root'
      className='space-y-4'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      {/* Header / 页面头部 */}
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          icon={<Puzzle className='h-5 w-5 text-primary' />}
          title={t('plugin.marketplace')}
          subtitle={t('plugin.marketplaceDesc')}
          actions={
            <div className='flex items-center gap-2'>
              <Button
                variant='default'
                size='sm'
                onClick={handleDownloadAllPlugins}
                disabled={loading || refreshingConnectors || isDownloadingAll || total === 0}
              >
                <DownloadCloud
                  className={`h-4 w-4 mr-1.5 ${isDownloadingAll ? 'animate-pulse' : ''}`}
                />
                {isDownloadingAll
                  ? t('plugin.downloadingAll')
                  : t('plugin.downloadAll')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                onClick={handleRefresh}
                disabled={loading || refreshingConnectors || localPluginsLoading}
              >
                <RefreshCw
                  className={`h-4 w-4 mr-1.5 ${(activeTab === 'available' ? refreshingConnectors : localPluginsLoading) ? 'animate-spin' : ''}`}
                />
                {activeTab === 'available'
                  ? t('plugin.refreshConnectors')
                  : t('common.refresh')}
              </Button>
            </div>
          }
        />
      </motion.div>

      {/* Error display / 错误显示 */}
      {error && (
        <Card className='border-destructive bg-destructive/5'>
          <CardContent className='p-3 text-sm text-destructive'>
            {error}
          </CardContent>
        </Card>
      )}

      {/* 一体化紧凑卡片容器 / Unified Compact Table & Grid Container */}
      <Card className='border-border/70 shadow-xs overflow-hidden'>
        {/* 工具栏第一行：胶囊导航与快捷操作 / Toolbar Row 1: Stat pills bar & Quick actions */}
        <div className='p-3 sm:p-3.5 border-b bg-muted/20 flex flex-wrap items-center justify-between gap-2.5'>
          <StatPillsBar
            items={statItems}
            activeKey={activeTab}
            onChange={(key) =>
              setActiveTab(key as 'available' | 'local' | 'custom')
            }
          />

          <div className='flex items-center gap-2 flex-wrap'>
            {activeTab === 'available' && catalogRefreshedAt && (
              <span className='text-xs text-muted-foreground hidden sm:inline-block'>
                {t('plugin.catalogRefreshedAt')}: {formatCatalogRefreshedAt(catalogRefreshedAt)}
              </span>
            )}
            {activeTab === 'local' && selectedLocalPlugins.size > 0 && (
              <div className='flex items-center gap-2'>
                <Badge variant='secondary' className='text-xs'>
                  {t('plugin.selectedCount', {
                    count: selectedLocalPlugins.size,
                  })}
                </Badge>
                <Button size='sm' className='h-8 text-xs' onClick={() => setIsBatchInstallOpen(true)}>
                  <CheckSquare className='h-3.5 w-3.5 mr-1.5' />
                  {t('plugin.batchInstall')}
                </Button>
              </div>
            )}
          </div>
        </div>

        {/* 工具栏第二行：高密度过滤栏 / Toolbar Row 2: Compact Filter Bar */}
        <div className='p-3 sm:p-3.5 border-b bg-muted/5 flex flex-wrap items-center gap-2.5'>
          <div className='flex-1 min-w-[180px] max-w-sm'>
            <Input
              data-testid='plugin-search-input'
              placeholder={t('plugin.searchPlaceholder')}
              value={searchKeyword}
              onChange={(e) => setSearchKeyword(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              className='h-8 text-xs'
            />
          </div>

          {/* Version selector / 版本选择器 */}
          <Select value={selectedVersion} onValueChange={setSelectedVersion}>
            <SelectTrigger className='w-[130px] h-8 text-xs' data-testid='plugin-version-select'>
              <SelectValue placeholder={t('plugin.version')} />
            </SelectTrigger>
            <SelectContent>
              {(availableVersions.length > 0
                ? availableVersions
                : recommendedVersion
                  ? [recommendedVersion]
                  : []
              ).map((version) => (
                <SelectItem key={version} value={version} className='text-xs'>
                  v{version}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Category selector / 分类选择器 */}
          <Select value={filterCategory} onValueChange={setFilterCategory}>
            <SelectTrigger className='w-[140px] h-8 text-xs' data-testid='plugin-category-select'>
              <SelectValue placeholder={t('plugin.category.all')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all' className='text-xs'>
                {t('plugin.category.all')}
              </SelectItem>
              <SelectItem value='realtime' className='text-xs'>
                {t('plugin.category.realtime')}
              </SelectItem>
              <SelectItem value='offline' className='text-xs'>
                {t('plugin.category.offline')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* Mirror selector (only in available tab) / 镜像源选择器（仅在线可用标签页展示） */}
          {activeTab === 'available' && (
            <Select
              value={selectedMirror}
              onValueChange={(v) => setSelectedMirror(v as MirrorSource)}
            >
              <SelectTrigger className='w-[140px] h-8 text-xs' data-testid='plugin-mirror-select'>
                <SelectValue placeholder={t('plugin.mirror')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='aliyun' className='text-xs'>
                  {t('installer.mirrors.aliyun')}
                </SelectItem>
                <SelectItem value='huaweicloud' className='text-xs'>
                  {t('installer.mirrors.huaweicloud')}
                </SelectItem>
                <SelectItem value='apache' className='text-xs'>
                  {t('installer.mirrors.apache')}
                </SelectItem>
              </SelectContent>
            </Select>
          )}

          <Button variant='outline' size='sm' className='h-8 text-xs' onClick={handleSearch}>
            <Search className='h-3.5 w-3.5 mr-1.5' />
            {t('common.search')}
          </Button>

          {(searchKeyword || filterCategory !== 'all') && (
            <Button variant='ghost' size='sm' className='h-8 text-xs' onClick={handleClearFilters}>
              {t('common.clearFilters')}
            </Button>
          )}
        </div>

        {/* 内容展示区 / Content display area */}
        <CardContent className='p-4'>
          {activeTab === 'available' && (
            <PluginGrid
              plugins={filteredAvailablePlugins}
              loading={loading}
              onViewDetail={handleViewDetail}
              showInstallButton={true}
              onInstall={handleInstallPlugin}
              onDownload={handleDownloadPlugin}
              downloadingPlugins={downloadingPlugins}
              downloadedPlugins={downloadedPlugins}
            />
          )}

          {activeTab === 'local' && (
            <>
              {localPluginsLoading &&
              localPlugins.length === 0 &&
              activeDownloads.length === 0 ? (
                <div className='text-center py-8 text-muted-foreground'>
                  <RefreshCw className='h-8 w-8 mx-auto animate-spin mb-4' />
                  <p>{t('common.loading')}</p>
                </div>
              ) : localPlugins.length === 0 && activeDownloads.length === 0 ? (
                <div className='text-center py-8 text-muted-foreground'>
                  <HardDrive className='h-12 w-12 mx-auto mb-4 opacity-50' />
                  <p>{t('plugin.noDownloadedPlugins')}</p>
                  <p className='text-sm mt-2'>
                    {t('plugin.downloadFromAvailable')}
                  </p>
                </div>
              ) : (
                <>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className='w-[50px]'>
                          <Checkbox
                            checked={
                              getFilteredLocalPlugins().length > 0 &&
                              getFilteredLocalPlugins().every((p) =>
                                selectedLocalPlugins.has(
                                  `${p.name}:${p.version}`,
                                ),
                              )
                            }
                            onCheckedChange={handleSelectAllLocalPlugins}
                          />
                        </TableHead>
                        <TableHead>{t('plugin.name')}</TableHead>
                        <TableHead>{t('plugin.category.label')}</TableHead>
                        <TableHead>{t('plugin.version')}</TableHead>
                        <TableHead>{t('plugin.installedClusters')}</TableHead>
                        <TableHead>{t('plugin.fileSize')}</TableHead>
                        <TableHead className='text-right'>
                          {t('common.actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {/* Active downloads / 活动下载 */}
                      {activeDownloads
                        .filter((d) => d.status === 'downloading')
                        .map((download) => (
                          <TableRow
                            key={`downloading-${download.plugin_name}-${download.version}`}
                            className='bg-blue-50 dark:bg-blue-950'
                          >
                            <TableCell>
                              <Checkbox disabled />
                            </TableCell>
                            <TableCell className='font-medium'>
                              <div className='flex items-center gap-2'>
                                <RefreshCw className='h-4 w-4 animate-spin text-blue-500' />
                                {download.plugin_name}
                              </div>
                            </TableCell>
                            <TableCell>
                              <Badge variant='outline'>
                                {t('plugin.downloading')}
                              </Badge>
                            </TableCell>
                            <TableCell>v{download.version}</TableCell>
                            <TableCell>
                              <span className='text-muted-foreground text-sm'>
                                -
                              </span>
                            </TableCell>
                            <TableCell>
                              <div className='space-y-1'>
                                <div className='flex items-center justify-between text-sm'>
                                  <span>
                                    {download.current_artifact_kind ===
                                    'dependency'
                                      ? t('plugin.downloadingDependencies')
                                      : t('plugin.downloadingConnector')}
                                  </span>
                                  <span>{download.progress}%</span>
                                </div>
                                {download.current_artifact && (
                                  <div className='text-xs text-muted-foreground'>
                                    {download.current_artifact_kind === 'dependency'
                                      ? t('plugin.currentDownloadingDependency', {
                                          artifact: download.current_artifact,
                                        })
                                      : t('plugin.currentDownloadingConnector', {
                                          artifact: download.current_artifact,
                                        })}
                                  </div>
                                )}
                                <Progress
                                  value={download.progress}
                                  className='h-2'
                                />
                                <div className='flex flex-wrap gap-3 text-xs text-muted-foreground'>
                                  <span>
                                    {t('plugin.connectorProgress', {
                                      completed: download.connector_completed || 0,
                                      total: download.connector_count || 0,
                                    })}
                                  </span>
                                  <span>
                                    {t('plugin.dependencyProgress', {
                                      completed: download.dependency_completed || 0,
                                      total: download.dependency_count || 0,
                                    })}
                                  </span>
                                </div>
                                {download.selected_profile_keys &&
                                  download.selected_profile_keys.length > 0 && (
                                    <div className='flex flex-wrap gap-2 text-xs text-muted-foreground'>
                                      <span>{t('plugin.selectedProfiles')}</span>
                                      {download.selected_profile_keys.map((profileKey) => (
                                        <Badge
                                          key={profileKey}
                                          variant='secondary'
                                          className='text-xs'
                                        >
                                          {profileKey}
                                        </Badge>
                                      ))}
                                    </div>
                                  )}
                                {download.total_bytes &&
                                  download.total_bytes > 0 && (
                                    <div className='text-xs text-muted-foreground'>
                                      {formatFileSize(
                                        download.downloaded_bytes || 0,
                                      )}{' '}
                                      / {formatFileSize(download.total_bytes)}
                                      {download.speed && download.speed > 0 && (
                                        <span className='ml-2'>
                                          ({formatFileSize(download.speed)}/s)
                                        </span>
                                      )}
                                    </div>
                                  )}
                              </div>
                            </TableCell>
                            <TableCell className='text-right'>
                              <Badge variant='secondary'>
                                {t('plugin.downloading')}
                              </Badge>
                            </TableCell>
                          </TableRow>
                        ))}
                      {/* Filtered local plugins / 过滤后的本地插件 */}
                      {getPaginatedLocalPlugins().map((plugin) => {
                        // Get cluster installation status for this plugin / 获取此插件的集群安装状态
                        const installedClusters =
                          getInstalledClustersForLocalPlugin(plugin);

                        return (
                          <TableRow
                            key={`${plugin.name}-${plugin.version}`}
                            data-testid={`plugin-local-row-${plugin.name}-${plugin.version}`}
                          >
                            <TableCell>
                              <Checkbox
                                checked={selectedLocalPlugins.has(
                                  `${plugin.name}:${plugin.version}`,
                                )}
                                onCheckedChange={(checked) =>
                                  handleSelectLocalPlugin(
                                    plugin,
                                    checked as boolean,
                                  )
                                }
                              />
                            </TableCell>
                            <TableCell className='font-medium'>
                              {plugin.name}
                            </TableCell>
                            <TableCell>
                              <Badge variant='outline'>
                                {plugin.category === 'connector'
                                  ? t('plugin.category.connector')
                                  : plugin.category}
                              </Badge>
                            </TableCell>
                            <TableCell>v{plugin.version}</TableCell>
                            <TableCell>
                              {installedClusters.length === 0 ? (
                                <span className='text-muted-foreground text-sm'>
                                  {t('plugin.notInstalled')}
                                </span>
                              ) : (
                                <div className='flex flex-wrap gap-1'>
                                  {installedClusters.map((cluster) => (
                                    <Badge
                                      key={cluster.id}
                                      variant='outline'
                                      className='text-xs'
                                    >
                                      {cluster.name}
                                    </Badge>
                                  ))}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>{formatFileSize(plugin.size)}</TableCell>
                            <TableCell className='text-right space-x-1'>
                              <Button
                                variant='outline'
                                size='sm'
                                data-testid={`plugin-local-manage-${plugin.name}-${plugin.version}`}
                                onClick={() => {
                                  const pluginForInstall: Plugin = {
                                    name: plugin.name,
                                    display_name: plugin.name,
                                    category: plugin.category,
                                    version: plugin.version,
                                    description: '',
                                    group_id: 'org.apache.seatunnel',
                                    artifact_id:
                                      plugin.artifact_id || `connector-${plugin.name}`,
                                    dependencies: plugin.dependencies || [],
                                  };
                                  openInstallDialog(
                                    pluginForInstall,
                                    plugin.version,
                                    plugin.selected_profile_keys || [],
                                    plugin.dependencies || [],
                                    plugin.attached_connectors || [],
                                  );
                                }}
                              >
                                <Server className='h-4 w-4 mr-1' />
                                {t('plugin.managePlugin')}
                              </Button>
                              <Button
                                variant='ghost'
                                size='sm'
                                data-testid={`plugin-local-delete-${plugin.name}-${plugin.version}`}
                                className='text-destructive hover:text-destructive'
                                onClick={() => openDeleteLocalPluginDialog(plugin)}
                              >
                                <Trash2 className='h-4 w-4' />
                              </Button>
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                  {/* Local plugins pagination / 本地插件分页 */}
                  {getFilteredLocalPlugins().length > 0 && (
                    <div className='mt-4'>
                      <Pagination
                        currentPage={localPluginsPage}
                        totalPages={getLocalPluginsTotalPages()}
                        pageSize={localPluginsPageSize}
                        totalItems={getFilteredLocalPlugins().length}
                        onPageChange={setLocalPluginsPage}
                        onPageSizeChange={(size) => {
                          setLocalPluginsPageSize(size);
                          setLocalPluginsPage(1);
                        }}
                        pageSizeOptions={[10, 20, 50, 100]}
                      />
                    </div>
                  )}
                </>
              )}
            </>
          )}

          {activeTab === 'custom' && (
            <div className='text-center py-12'>
              <Upload className='h-12 w-12 mx-auto text-muted-foreground mb-4 opacity-60' />
              <p className='text-muted-foreground text-sm'>
                {t('plugin.customComingSoon')}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Plugin Detail Dialog / 插件详情对话框 */}
      {selectedPlugin && (
        <PluginDetailDialog
          open={isDetailOpen}
          onOpenChange={setIsDetailOpen}
          plugin={selectedPlugin}
          seatunnelVersion={selectedVersion}
          selectedProfileKeys={getSelectedProfileKeysForPlugin(selectedPlugin.name)}
          onSelectedProfileKeysChange={(keys) =>
            setSelectedProfileKeysForPlugin(selectedPlugin.name, keys)
          }
          onDownload={handleDownloadPlugin}
        />
      )}

      {/* Install Plugin Dialog / 安装插件对话框 */}
      {installContext && (
        <InstallPluginDialog
          open={isInstallOpen}
          onOpenChange={(next) => {
            setIsInstallOpen(next);
            if (!next) {
              setInstallContext(null);
            }
          }}
          plugin={installContext.plugin}
          version={installContext.version}
          selectedProfileKeys={installContext.selectedProfileKeys}
          attachedConnectors={installContext.attachedConnectors}
          dependencies={installContext.dependencies}
        />
      )}

      <Dialog
        open={isBatchProfileDialogOpen}
        onOpenChange={(next) => {
          if (batchProfileLoading || isDownloadingAll) {
            return;
          }
          setIsBatchProfileDialogOpen(next);
        }}
      >
        <DialogContent className='sm:max-w-3xl max-h-[85vh] overflow-hidden flex flex-col'>
          <DialogHeader>
            <DialogTitle>{t('plugin.downloadAllProfileDialogTitle')}</DialogTitle>
            <DialogDescription>
              {t('plugin.downloadAllProfileDialogDescription')}
            </DialogDescription>
          </DialogHeader>

          <div className='flex-1 overflow-y-auto pr-1'>
            {batchProfileLoading ? (
              <div className='py-8 text-sm text-muted-foreground'>
                {t('plugin.downloadAllProfileLoading')}
              </div>
            ) : batchProfilePlugins.length === 0 ? (
              <div className='py-8 text-sm text-muted-foreground'>
                {t('plugin.downloadAllProfileEmpty')}
              </div>
            ) : (
              <Tabs
                value={activeBatchProfileTab}
                onValueChange={setActiveBatchProfileTab}
                className='w-full'
              >
                <TabsList className='w-full flex flex-wrap h-auto justify-start'>
                  {batchProfilePlugins.map((plugin) => (
                    <TabsTrigger key={plugin.name} value={plugin.name}>
                      {plugin.display_name || plugin.name}
                    </TabsTrigger>
                  ))}
                </TabsList>

                {batchProfilePlugins.map((plugin) => {
                  const officialDeps = batchProfileOfficialDeps[plugin.name];
                  const profiles = officialDeps?.profiles || [];
                  const selectedKeys = normalizeProfileKeys(
                    selectedProfileKeysByPlugin[plugin.name],
                  );

                  return (
                    <TabsContent
                      key={plugin.name}
                      value={plugin.name}
                      className='mt-4 space-y-4'
                    >
                      <div className='flex items-center justify-between gap-3'>
                        <div>
                          <h3 className='font-medium'>
                            {plugin.display_name || plugin.name}
                          </h3>
                          <p className='text-sm text-muted-foreground'>
                            {t('plugin.downloadAllProfilePluginHint')}
                          </p>
                        </div>
                        <div className='flex items-center gap-2'>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              setSelectedProfileKeysForPlugin(
                                plugin.name,
                                profiles.map((profile) => profile.profile_key),
                              )
                            }
                          >
                            {t('plugin.selectAllProfiles')}
                          </Button>
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            onClick={() => setSelectedProfileKeysForPlugin(plugin.name, [])}
                          >
                            {t('plugin.clearAllProfiles')}
                          </Button>
                        </div>
                      </div>

                      {profiles.length === 0 ? (
                        <div className='rounded-md border border-dashed p-4 text-sm text-muted-foreground'>
                          {t('plugin.downloadAllProfileEmpty')}
                        </div>
                      ) : (
                        <div className='space-y-3'>
                          {profiles.map((profile) => {
                            const checked = selectedKeys.includes(profile.profile_key);
                            return (
                              <label
                                key={profile.profile_key}
                                className='flex items-start gap-3 rounded-md border p-3 cursor-pointer'
                              >
                                <Checkbox
                                  checked={checked}
                                  onCheckedChange={(nextChecked) => {
                                    const next = new Set(selectedKeys);
                                    if (nextChecked) {
                                      next.add(profile.profile_key);
                                    } else {
                                      next.delete(profile.profile_key);
                                    }
                                    setSelectedProfileKeysForPlugin(
                                      plugin.name,
                                      Array.from(next),
                                    );
                                  }}
                                />
                                <div className='space-y-1'>
                                  <div className='font-medium'>
                                    {profile.profile_name || profile.profile_key}
                                  </div>
                                  <div className='text-xs text-muted-foreground'>
                                    {profile.profile_key}
                                  </div>
                                </div>
                              </label>
                            );
                          })}
                        </div>
                      )}
                    </TabsContent>
                  );
                })}
              </Tabs>
            )}
          </div>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => setIsBatchProfileDialogOpen(false)}
              disabled={batchProfileLoading || isDownloadingAll}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleConfirmBatchProfileDownload()}
              disabled={batchProfileLoading || batchProfileDialogIncomplete || isDownloadingAll}
            >
              {t('plugin.downloadAllProfileConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog / 删除确认对话框 */}
      <AlertDialog
        open={deleteDialogOpen}
        onOpenChange={handleDeleteDialogOpenChange}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('plugin.deleteConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('plugin.deleteConfirmDesc', {
                name: pluginToDelete?.name || '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>

          {deleteDialogInstalledClusters.length > 0 && (
            <div className='space-y-3 rounded-md border p-3'>
              <div className='space-y-1'>
                <div className='text-sm font-medium'>
                  {t('plugin.deleteInstalledClustersTitle')}
                </div>
                <p className='text-xs text-muted-foreground'>
                  {t('plugin.deleteInstalledClustersDesc')}
                </p>
              </div>
              <label className='flex items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm'>
                <Checkbox
                  checked={
                    deleteUninstallClusterIds.size ===
                    deleteDialogInstalledClusters.length
                  }
                  disabled={isDeletingLocalPlugin}
                  onCheckedChange={(checked) =>
                    handleToggleAllDeleteUninstallClusters(checked === true)
                  }
                />
                <span>{t('plugin.deleteSelectAllClusters')}</span>
              </label>
              <div className='max-h-48 space-y-2 overflow-y-auto pr-1'>
                {deleteDialogInstalledClusters.map((cluster) => (
                  <label
                    key={cluster.id}
                    className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'
                    data-testid={`plugin-delete-uninstall-cluster-${cluster.id}`}
                  >
                    <span className='min-w-0'>
                      <span className='block truncate font-medium'>
                        {cluster.name}
                      </span>
                      <span className='block text-xs text-muted-foreground'>
                        v{cluster.version}
                      </span>
                    </span>
                    <Checkbox
                      checked={deleteUninstallClusterIds.has(cluster.id)}
                      disabled={isDeletingLocalPlugin}
                      onCheckedChange={(checked) =>
                        handleToggleDeleteUninstallCluster(
                          cluster.id,
                          checked === true,
                        )
                      }
                    />
                  </label>
                ))}
              </div>
            </div>
          )}

          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeletingLocalPlugin}>
              {t('common.cancel')}
            </AlertDialogCancel>
            <Button
              type='button'
              onClick={handleDeleteLocalPlugin}
              disabled={isDeletingLocalPlugin}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {isDeletingLocalPlugin
                ? t('common.processing')
                : deleteUninstallClusterIds.size > 0
                  ? t('plugin.deleteAndUninstallSelected', {
                      count: deleteUninstallClusterIds.size,
                    })
                  : t('plugin.deleteLocalOnly')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Batch Install Dialog / 批量安装对话框 */}
      <BatchInstallDialog
        open={isBatchInstallOpen}
        onOpenChange={(open: boolean) => {
          setIsBatchInstallOpen(open);
          if (!open) {
            setSelectedLocalPlugins(new Set());
          }
        }}
        plugins={getSelectedPluginsForBatchInstall()}
        version={selectedVersion}
      />

    </motion.div>
  );
}
