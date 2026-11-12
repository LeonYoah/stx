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
  AlertTriangle,
} from 'lucide-react';
import {motion, useReducedMotion} from 'motion/react';
import {
  WorkspaceHeader,
  StatPillsBar,
  ModuleNavTabs,
  type StatPillItem,
} from '@/components/common/layout';
import {PluginService} from '@/lib/services/plugin';
import {usePackages} from '@/hooks/use-installer';
import {resolveSeatunnelVersion} from '@/lib/seatunnel-version';
import {resolvePreferredClusterId} from '@/lib/cluster-preference';
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
  PluginListSource,
} from '@/lib/services/plugin';
import type {ClusterInfo} from '@/lib/services/cluster';
import {
  getConnectorCapability,
  matchesConnectorProcessingMode,
  getConnectorSemanticIcon,
} from './connector-quick-helper';
import {PluginGrid} from './PluginGrid';
import {PluginDetailDialog} from './PluginDetailDialog';
import {InstallPluginDialog} from './InstallPluginDialog';
import {BatchInstallDialog} from './BatchInstallDialog';
import {InstalledClusterStack} from './InstalledClusterStack';
import {resolvePluginDownloadFailureMessage} from './plugin-download-status';
import {
  buildPluginPageVariants,
  getClusterStackInitials,
} from './plugin-motion';
import {Pagination} from '@/components/ui/pagination';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {DropdownMenuItem} from '@/components/ui/dropdown-menu';
import {formatDateTimeTz} from '@/lib/timezone';
import {
  MarketplaceListToolbar,
  MarketplaceMorphCatalog,
  MarketplaceOverviewBento,
  useMarketplaceViewMode,
  type MarketplaceCatalogItem,
  type MarketplaceOverviewModel,
} from '@/components/common/marketplace';


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
  const reduceMotion = useReducedMotion() ?? false;
  const {container: containerVariants, item: itemVariants} = useMemo(
    () => buildPluginPageVariants(reduceMotion),
    [reduceMotion],
  );
  const [availableViewMode, setAvailableViewMode] = useMarketplaceViewMode(
    'stx.marketplace.plugins.available.view',
    'card',
  );
  const [localViewMode, setLocalViewMode] = useMarketplaceViewMode(
    'stx.marketplace.plugins.local.view',
    'table',
  );

  // Data state / 数据状态
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshingConnectors, setRefreshingConnectors] = useState(false);
  const [catalogRefreshedAt, setCatalogRefreshedAt] = useState<string>('');
  // 目录来源：seed 表示官方 Maven 不可用、回退内置清单，下载可能失败。
  // Catalog source: seed means Maven unavailable / bundled fallback; downloads may fail.
  const [catalogSource, setCatalogSource] = useState<PluginListSource | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);

  // Filter state / 过滤状态
  const [searchKeyword, setSearchKeyword] = useState('');
  const [filterCategory, setFilterCategory] = useState<string>('all');
  const [selectedMirror, setSelectedMirror] = useState<MirrorSource>('aliyun');
  const [selectedVersion, setSelectedVersion] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'available' | 'local' | 'custom'>(
    'available',
  );
  // 用户手动改过版本后，不再被默认集群 / packages 推荐覆盖。
  // After a manual version pick, do not overwrite with cluster / packages defaults.
  const userOverrideVersionRef = useRef(false);
  const {packages, loading: packagesLoading} = usePackages();
  const availableVersions = packages?.versions || [];
  const recommendedVersion = resolveSeatunnelVersion(packages);
  // 用于解析默认版本的集群列表（含偏好 / 唯一默认集群）。
  // Cluster list used to resolve default version (preferred / sole default).
  const [preferenceClusters, setPreferenceClusters] = useState<ClusterInfo[]>(
    [],
  );
  const [preferenceClustersLoaded, setPreferenceClustersLoaded] =
    useState(false);

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
  // 已提示过的失败下载，避免轮询重复 toast。
  // Already-notified failed downloads to avoid toast spam while polling.
  const notifiedFailedDownloadsRef = useRef<Set<string>>(new Set());

  // Plugin installation status per cluster / 每个集群的插件安装状态
  // Map: pluginName:version -> { clusterId -> InstalledPlugin }
  const [pluginClusterStatus, setPluginClusterStatus] = useState<
    Map<string, Map<number, InstalledPlugin>>
  >(new Map());
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);

  // 默认集群版本：唯一集群或本地偏好；无明确默认则不用集群版本。
  // Preferred cluster version: sole cluster or stored preference; skip when ambiguous.
  const preferredClusterVersion = useMemo(() => {
    const preferredId = resolvePreferredClusterId(preferenceClusters);
    if (preferredId == null) {
      return '';
    }
    const cluster = preferenceClusters.find((item) => item.id === preferredId);
    return String(cluster?.version || '').trim();
  }, [preferenceClusters]);

  // 版本优先级：默认集群版本 > packages 最新推荐。
  // Version priority: preferred cluster version > packages recommended.
  const resolvedDefaultVersion =
    preferredClusterVersion || recommendedVersion || '';

  // 下拉选项需包含当前选中版本（集群版本可能不在 packages 列表中）。
  // Version options must include the current selection (cluster version may be absent from packages).
  const versionOptions = useMemo(() => {
    const base =
      availableVersions.length > 0
        ? availableVersions
        : recommendedVersion
          ? [recommendedVersion]
          : [];
    if (selectedVersion && !base.includes(selectedVersion)) {
      return [selectedVersion, ...base];
    }
    return base;
  }, [availableVersions, recommendedVersion, selectedVersion]);

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

  // 下载进度映射：key -> progress 百分比数值 (0-100)
  // Download progress mapping: key -> progress percentage (0-100)
  const downloadProgressMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const d of activeDownloads) {
      if (d.progress !== undefined && d.progress !== null) {
        map.set(pluginDownloadKey(d.plugin_name, d.version), d.progress);
      }
    }
    return map;
  }, [activeDownloads]);

  const loadPlugins = useCallback(
    async (options?: {force?: boolean}) => {
      // 必须带明确版本请求，禁止空 version 触发后端 DefaultVersion(2.3.12) 锁死 UI。
      // Always request with an explicit version; never let empty version lock UI to backend DefaultVersion.
      const requestVersion =
        selectedVersion || resolvedDefaultVersion || '';
      if (!requestVersion) {
        setLoading(false);
        return;
      }
      const queryKey = buildAvailableQueryKey(requestVersion);

      if (!options?.force && lastLoadedAvailableQueryRef.current === queryKey) {
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const result: AvailablePluginsResponse =
          await PluginService.listAvailablePlugins(
            requestVersion,
            selectedMirror,
          );

        setPlugins(result.plugins || []);
        setTotal(result.total || (result.plugins || []).length);
        setCatalogRefreshedAt(result.catalog_refreshed_at || '');
        setCatalogSource(result.source || null);
        lastLoadedAvailableQueryRef.current = buildAvailableQueryKey(
          result.version || requestVersion,
        );
        // 不再用响应 version 覆盖选中项，避免后端兜底抢跑覆盖推荐 / 集群版本。
        // Do not overwrite selection from response.version (avoids backend fallback racing defaults).

        if (result.source === 'seed' && (result.total || 0) > 0) {
          toast.warning(t('plugin.catalogFromSeedTitle'), {
            description: t('plugin.catalogFromSeedDesc'),
            duration: 8000,
          });
        } else if (
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
        setCatalogSource(null);
        lastLoadedAvailableQueryRef.current = null;
      } finally {
        setLoading(false);
      }
    },
    [
      buildAvailableQueryKey,
      resolvedDefaultVersion,
      selectedMirror,
      selectedVersion,
      t,
    ],
  );

  /**
   * 合并下载中集合：内容不变时复用旧 Set，避免无意义重渲染。
   * Merge downloading set; reuse previous Set when membership is unchanged.
   */
  const mergeDownloadingPlugins = useCallback(
    (
      prev: Set<string>,
      downloadsResult: PluginDownloadProgress[] | null | undefined,
      downloadedKeys: Set<string>,
    ) => {
      const downloading = new Set(
        downloadsResult
          ?.filter((d) => d.status === 'downloading')
          .map((d) => pluginDownloadKey(d.plugin_name, d.version)) || [],
      );
      const next = new Set<string>();
      for (const key of downloading) {
        if (!downloadedKeys.has(key)) {
          next.add(key);
        }
      }
      for (const key of prev) {
        if (downloadedKeys.has(key)) {
          continue;
        }
        const listed = (downloadsResult || []).find(
          (d) => pluginDownloadKey(d.plugin_name, d.version) === key,
        );
        if (listed) {
          if (listed.status === 'downloading') {
            next.add(key);
          }
          continue;
        }
        if (!notifiedFailedDownloadsRef.current.has(key)) {
          next.add(key);
        }
      }
      if (next.size === prev.size && [...next].every((key) => prev.has(key))) {
        return prev;
      }
      return next;
    },
    [],
  );

  /**
   * 轻量轮询：只拉活动下载与本地落盘，不打集群安装态、不闪 loading。
   * Lightweight poll: active downloads + local disk only; skip cluster status and loading flash.
   */
  const pollActiveDownloads = useCallback(async () => {
    try {
      const [localResult, downloadsResult] = await Promise.all([
        PluginService.listLocalPlugins(),
        PluginService.listActiveDownloads(),
      ]);

      const downloadedKeys = new Set(
        (localResult || []).map((item) =>
          pluginDownloadKey(item.name, item.version),
        ),
      );
      setDownloadedPlugins((prev) => {
        if (
          prev.size === downloadedKeys.size &&
          [...downloadedKeys].every((key) => prev.has(key))
        ) {
          return prev;
        }
        return downloadedKeys;
      });
      setLocalPlugins((prev) => {
        const next = localResult || [];
        if (
          prev.length === next.length &&
          prev.every(
            (item, index) =>
              item.name === next[index]?.name &&
              item.version === next[index]?.version &&
              item.size === next[index]?.size,
          )
        ) {
          return prev;
        }
        return next;
      });
      setActiveDownloads((prev) => {
        const next = downloadsResult || [];
        if (
          prev.length === next.length &&
          prev.every(
            (item, index) =>
              item.plugin_name === next[index]?.plugin_name &&
              item.version === next[index]?.version &&
              item.status === next[index]?.status &&
              item.progress === next[index]?.progress &&
              item.error === next[index]?.error &&
              item.message === next[index]?.message,
          )
        ) {
          return prev;
        }
        return next;
      });

      for (const item of downloadsResult || []) {
        if (item.status !== 'failed') {
          continue;
        }
        const failKey = pluginDownloadKey(item.plugin_name, item.version);
        if (notifiedFailedDownloadsRef.current.has(failKey)) {
          continue;
        }
        notifiedFailedDownloadsRef.current.add(failKey);
        toast.error(
          t('plugin.downloadFailedNamed', {
            name: item.plugin_name,
            version: item.version,
          }),
          {
            description:
              item.error || item.message || t('plugin.downloadFailedHint'),
            duration: 10000,
          },
        );
      }

      setDownloadingPlugins((prev) =>
        mergeDownloadingPlugins(prev, downloadsResult, downloadedKeys),
      );
    } catch (err) {
      console.error('Failed to poll active downloads:', err);
    }
  }, [mergeDownloadingPlugins, t]);

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
      const downloadedKeys = new Set(
        (localResult || []).map((item) => pluginDownloadKey(item.name, item.version)),
      );
      setDownloadedPlugins(downloadedKeys);
      setActiveDownloads(downloadsResult || []);
      // 同步偏好集群列表，保持默认版本解析与本地 Tab 数据一致。
      // Keep preference clusters in sync with local-tab fetch for default version resolution.
      setPreferenceClusters(clustersResult?.clusters ?? []);
      setPreferenceClustersLoaded(true);

      // Filter available clusters / 过滤可用集群
      const availableClusters = (clustersResult?.clusters || []).filter(
        (c: ClusterInfo) => c.status === 'running' || c.status === 'stopped',
      );
      setClusters(availableClusters);

      // 下载失败时主动 toast（请求本身只表示入队成功，真正失败在异步任务里）。
      // Surface async download failures; the download API only means the job was queued.
      for (const item of downloadsResult || []) {
        if (item.status !== 'failed') {
          continue;
        }
        const failKey = pluginDownloadKey(item.plugin_name, item.version);
        if (notifiedFailedDownloadsRef.current.has(failKey)) {
          continue;
        }
        notifiedFailedDownloadsRef.current.add(failKey);
        toast.error(
          t('plugin.downloadFailedNamed', {
            name: item.plugin_name,
            version: item.version,
          }),
          {
            description:
              item.error || item.message || t('plugin.downloadFailedHint'),
            duration: 10000,
          },
        );
      }

      setDownloadingPlugins((prev) =>
        mergeDownloadingPlugins(prev, downloadsResult, downloadedKeys),
      );

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
  }, [mergeDownloadingPlugins, t]);

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
      const version = selectedVersion || resolvedDefaultVersion || '';
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
    resolvedDefaultVersion,
    requiresProfileSelection,
    selectedVersion,
    t,
  ]);

  const executeDownloadAllPlugins = useCallback(
    async (selectedProfiles?: Record<string, string[]>) => {
      try {
        setIsDownloadingAll(true);
        if (catalogSource === 'seed') {
          toast.warning(t('plugin.downloadOnSeedWarning'), {
            description: t('plugin.downloadOnSeedWarningDesc'),
            duration: 6000,
          });
        }
        toast.info(t('plugin.downloadAllStarted', {count: total}));

        const result = await PluginService.downloadAllPlugins(
          selectedVersion,
          selectedMirror,
          selectedProfiles,
        );

        if ((result.failed || 0) > 0) {
          toast.error(
            t('plugin.downloadAllPartialFailed', {
              failed: result.failed,
              downloaded: result.downloaded,
              total: result.total,
            }),
            {duration: 10000},
          );
        } else {
          toast.success(
            t('plugin.downloadAllSuccess', {
              total: result.total,
              downloaded: result.downloaded,
              skipped: result.skipped,
            }),
          );
        }
        void loadLocalPlugins();
      } catch (err) {
        const errorMsg =
          err instanceof Error ? err.message : t('plugin.downloadAllFailed');
        toast.error(errorMsg);
      } finally {
        setIsDownloadingAll(false);
      }
    },
    [
      catalogSource,
      loadLocalPlugins,
      selectedMirror,
      selectedVersion,
      t,
      total,
    ],
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

  // 预加载集群列表，用于解析默认版本（不依赖「本地插件」Tab）。
  // Prefetch clusters to resolve default version (independent of local-plugins tab).
  useEffect(() => {
    void ClusterService.getClusters({current: 1, size: 100})
      .then((result) => {
        setPreferenceClusters(result?.clusters ?? []);
      })
      .catch(() => {
        setPreferenceClusters([]);
      })
      .finally(() => {
        setPreferenceClustersLoaded(true);
      });
  }, []);

  // 自动填充默认版本：优先默认集群，否则 packages 推荐；用户手选后不再覆盖。
  // Auto-fill default version: preferred cluster first, else packages recommended; stop after user pick.
  useEffect(() => {
    if (userOverrideVersionRef.current) {
      return;
    }
    if (!preferenceClustersLoaded) {
      return;
    }
    if (!preferredClusterVersion && packagesLoading) {
      return;
    }
    if (!resolvedDefaultVersion) {
      return;
    }
    if (selectedVersion !== resolvedDefaultVersion) {
      setSelectedVersion(resolvedDefaultVersion);
    }
  }, [
    packagesLoading,
    preferenceClustersLoaded,
    preferredClusterVersion,
    resolvedDefaultVersion,
    selectedVersion,
  ]);

  useEffect(() => {
    if (activeTab !== 'available') {
      return;
    }
    // 等默认版本解析完成后再拉列表，避免空 version 抢跑。
    // Wait until default version is resolved before loading available plugins.
    if (!preferenceClustersLoaded) {
      return;
    }
    if (!preferredClusterVersion && packagesLoading) {
      return;
    }
    if (!selectedVersion && !resolvedDefaultVersion) {
      setLoading(false);
      return;
    }
    void loadPlugins();
  }, [
    activeTab,
    loadPlugins,
    packagesLoading,
    preferenceClustersLoaded,
    preferredClusterVersion,
    resolvedDefaultVersion,
    selectedVersion,
  ]);

  // 预加载本地下载、失败进度与集群安装量，供「在线可用」卡片与「本地插件」表共用。
  // Preload local downloads, failures, and cluster install counts for both available cards and the local table.
  useEffect(() => {
    if (!selectedVersion) {
      return;
    }
    void loadLocalPlugins();
  }, [selectedVersion, loadLocalPlugins]);

  // Load local plugins when switching to local tab / 切换到本地插件标签时加载
  useEffect(() => {
    if (activeTab === 'local') {
      loadLocalPlugins();
    }
  }, [activeTab, loadLocalPlugins]);

  // 有下载中任务时轻量轮询进度（不限本地 Tab）；避免全量 loadLocalPlugins 拖动详情/列表闪烁。
  // Lightweight progress poll while downloads run; avoid full loadLocalPlugins flashing detail/list UI.
  useEffect(() => {
    const hasDownloading =
      activeDownloads.some((d) => d.status === 'downloading') ||
      downloadingPlugins.size > 0;
    if (!hasDownloading) {
      return;
    }

    const interval = setInterval(() => {
      void pollActiveDownloads();
    }, 2000);

    return () => clearInterval(interval);
  }, [activeDownloads, downloadingPlugins, pollActiveDownloads]);

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
      const requestVersion = selectedVersion || resolvedDefaultVersion || '';
      if (!requestVersion) {
        return;
      }
      setRefreshingConnectors(true);
      setError(null);
      void PluginService.refreshAvailablePlugins(
        requestVersion,
        selectedMirror,
      )
        .then((result) => {
          setPlugins(result.plugins || []);
          setTotal(result.total || (result.plugins || []).length);
          setCatalogRefreshedAt(result.catalog_refreshed_at || '');
          setCatalogSource(result.source || null);
          lastLoadedAvailableQueryRef.current = buildAvailableQueryKey(
            result.version || requestVersion,
          );
          if (result.source === 'seed') {
            toast.warning(t('plugin.catalogFromSeedTitle'), {
              description: t('plugin.catalogFromSeedDesc'),
              duration: 8000,
            });
          } else {
            toast.success(t('plugin.refreshConnectorsSuccess'));
          }
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

    // seed 目录只是内置清单，Maven 可能尚无构件；先警告再继续入队。
    // Seed catalog is a bundled list only — Maven may lack artifacts; warn then queue.
    if (catalogSource === 'seed') {
      toast.warning(t('plugin.downloadOnSeedWarning'), {
        description: t('plugin.downloadOnSeedWarningDesc'),
        duration: 6000,
      });
    }

    try {
      setDownloadingPlugins((prev) => new Set(prev).add(downloadKey));
      // 允许同一插件再次失败时重新提示。
      // Allow re-notifying if the same plugin fails again.
      notifiedFailedDownloadsRef.current.delete(downloadKey);

      await PluginService.downloadPlugin(
        plugin.name,
        pluginVersion,
        selectedMirror,
        selectedProfileKeys,
      );

      toast.success(t('plugin.downloadStarted'));
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

  // 失败下载映射：在线卡片与本地表共享同一套失败语义；无 error/message 时写入兜底文案，避免 Boolean('') 误判为未失败。
  // Failed-download map shared by cards and the local table; store a fallback when error/message are empty so Boolean('') cannot hide failure.
  const failedDownloadMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const item of activeDownloads) {
      if (item.status !== 'failed') {
        continue;
      }
      map.set(
        pluginDownloadKey(item.plugin_name, item.version),
        resolvePluginDownloadFailureMessage(
          item.error,
          item.message,
          t('plugin.downloadFailedHint'),
        ),
      );
    }
    return map;
  }, [activeDownloads, t]);

  // 已安装集群数量：供在线卡片展示安装量，语义与本地表一致。
  // Installed cluster counts for available cards; same meaning as the local table.
  const installedClusterCountMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const [key, clusterMap] of pluginClusterStatus.entries()) {
      map.set(key, clusterMap.size);
    }
    return map;
  }, [pluginClusterStatus]);

  const formatCatalogRefreshedAt = useCallback((value: string) => {
    if (!value) {
      return '-';
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return value;
    }
    return formatDateTimeTz(date);
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

  /**
   * Overview bento from real loaded data only — hide zeros / unready slices.
   * 总览 bento 只用已加载真实数据；0 / 未就绪项直接隐藏。
   */
  const overviewModel: MarketplaceOverviewModel = useMemo(() => {
    const roleCounts = {source: 0, sink: 0, transform: 0, both: 0};
    for (const plugin of plugins) {
      const cap = getConnectorCapability(plugin.name, plugin.category);
      if (cap.isTransform) {
        roleCounts.transform += 1;
      } else if (cap.supportsSource && cap.supportsSink) {
        roleCounts.both += 1;
      } else if (cap.supportsSource) {
        roleCounts.source += 1;
      } else if (cap.supportsSink) {
        roleCounts.sink += 1;
      }
    }

    const installedClusterIds = new Set<number>();
    for (const clusterMap of pluginClusterStatus.values()) {
      for (const id of clusterMap.keys()) {
        installedClusterIds.add(id);
      }
    }
    const bubbleClusters = clusters
      .filter((c) => installedClusterIds.has(c.id))
      .slice(0, 8)
      .map((c) => ({
        id: String(c.id),
        label: c.name,
        mark: getClusterStackInitials(c.name),
        toneClass: 'bg-primary/15 text-primary',
      }));

    const downloadingCount = activeDownloads.filter(
      (d) => d.status === 'downloading',
    ).length;
    const localCount = localPlugins.length;
    const slices = [
      {
        id: 'source',
        label: t('marketplaceUi.distSource'),
        count: roleCounts.source,
        toneClass: 'bg-sky-500/15',
      },
      {
        id: 'sink',
        label: t('marketplaceUi.distSink'),
        count: roleCounts.sink,
        toneClass: 'bg-amber-500/15',
      },
      {
        id: 'transform',
        label: t('marketplaceUi.distTransform'),
        count: roleCounts.transform,
        toneClass: 'bg-violet-500/15',
      },
      {
        id: 'both',
        label: t('marketplaceUi.distBoth'),
        count: roleCounts.both,
        toneClass: 'bg-emerald-500/15',
      },
    ].filter((s) => s.count > 0);

    const stats = [
      ...(localCount > 0
        ? [
            {
              id: 'local',
              label: t('marketplaceUi.overviewLocal'),
              value: localCount,
            },
          ]
        : []),
      ...(downloadingCount > 0
        ? [
            {
              id: 'downloading',
              label: t('marketplaceUi.overviewDownloading'),
              value: downloadingCount,
            },
          ]
        : []),
      ...(installedClusterIds.size > 0
        ? [
            {
              id: 'clusters',
              label: t('marketplaceUi.overviewInstalledClusters'),
              value: installedClusterIds.size,
            },
          ]
        : []),
    ];

    return {
      hero: {
        label: t('marketplaceUi.overviewAvailable'),
        value: total > 0 ? total : '',
        caption:
          total > 0 ? t('marketplaceUi.overviewAvailableCaption') : undefined,
        bubbles: bubbleClusters.length > 0 ? bubbleClusters : undefined,
      },
      distribution:
        slices.length > 0
          ? {
              title: t('marketplaceUi.distributionTitle'),
              slices,
              footer: t('marketplaceUi.distributionFooter'),
            }
          : undefined,
      stats,
    };
  }, [
    activeDownloads,
    clusters,
    localPlugins.length,
    pluginClusterStatus,
    plugins,
    t,
    total,
  ]);

  /** Local + active-download catalog items for MorphCatalog. / 本地与活动下载的 MorphCatalog 条目。 */
  const localCatalogItems: MarketplaceCatalogItem[] = useMemo(() => {
    const downloadRows: MarketplaceCatalogItem[] = activeDownloads
      .filter((d) => d.status === 'downloading' || d.status === 'failed')
      .map((download) => {
        const id = `download-${download.plugin_name}-${download.version}-${download.status}`;
        const failed = download.status === 'failed';
        return {
          id,
          title: download.plugin_name,
          subtitle: `v${download.version}`,
          icon: (
            <span
              className={`inline-flex size-9 items-center justify-center rounded-full ${
                failed
                  ? 'bg-destructive/10 text-destructive'
                  : 'bg-sky-500/15 text-sky-700 dark:text-sky-300'
              }`}
            >
              {failed ? (
                <AlertTriangle className='size-4' />
              ) : (
                <RefreshCw className='size-4 animate-spin motion-reduce:animate-none' />
              )}
            </span>
          ),
          badges: [
            {
              label: failed
                ? t('marketplaceUi.statusFailed')
                : t('marketplaceUi.statusDownloading'),
              highlight: failed,
            },
          ],
          status: {
            label: failed
              ? t('marketplaceUi.statusFailed')
              : t('marketplaceUi.statusDownloading'),
            tone: failed ? 'error' : 'info',
          },
          fields: [
            {
              label: t('marketplaceUi.fieldVersion'),
              value: `v${download.version}`,
            },
            ...(failed
              ? [
                  {
                    label: t('marketplaceUi.statusFailed'),
                    value:
                      download.error ||
                      download.message ||
                      t('plugin.downloadFailedHint'),
                  },
                ]
              : []),
          ],
          progress: failed
            ? null
            : {
                label: t('marketplaceUi.progressDownload'),
                value: download.progress || 0,
                display: `${download.progress || 0}%`,
              },
        };
      });

    const pluginRows: MarketplaceCatalogItem[] = getPaginatedLocalPlugins().map(
      (plugin) => {
        const key = `${plugin.name}:${plugin.version}`;
        const installedClusters = getInstalledClustersForLocalPlugin(plugin);
        const semanticIcon = getConnectorSemanticIcon(
          plugin.name,
          plugin.category,
        );
        const IconComponent = semanticIcon.icon;
        return {
          id: key,
          title: plugin.name,
          subtitle: `v${plugin.version}`,
          leading: (
            <Checkbox
              checked={selectedLocalPlugins.has(key)}
              onCheckedChange={(checked) =>
                handleSelectLocalPlugin(plugin, checked === true)
              }
              onClick={(e) => e.stopPropagation()}
              aria-label={plugin.name}
            />
          ),
          icon: (
            <span
              className={`inline-flex size-9 items-center justify-center rounded-full ${semanticIcon.bgColor} ${semanticIcon.textColor}`}
            >
              <IconComponent className='size-4' />
            </span>
          ),
          badges: [
            {
              label:
                plugin.category === 'connector'
                  ? t('plugin.category.connector')
                  : plugin.category,
            },
            ...(installedClusters.length > 0
              ? [
                  {
                    label: `${t('plugin.installed')} · ${installedClusters.length}`,
                    highlight: true,
                  },
                ]
              : []),
          ],
          status: {
            label:
              installedClusters.length > 0
                ? t('plugin.installed')
                : t('plugin.notInstalled'),
            tone: installedClusters.length > 0 ? 'ok' : 'muted',
          },
          aside: (
            <InstalledClusterStack
              clusters={installedClusters.map((cluster) => ({
                id: cluster.id,
                name: cluster.name,
              }))}
            />
          ),
          fields: [
            {
              label: t('marketplaceUi.fieldVersion'),
              value: `v${plugin.version}`,
            },
            {
              label: t('marketplaceUi.fieldSize'),
              value: formatFileSize(plugin.size),
            },
            {
              label: t('marketplaceUi.fieldClusters'),
              value:
                installedClusters.length > 0
                  ? String(installedClusters.length)
                  : t('plugin.notInstalled'),
            },
          ],
          meta: formatFileSize(plugin.size),
          menuActions: (
            <>
              <DropdownMenuItem
                onClick={(e) => {
                  e.stopPropagation();
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
                <Server className='mr-2 size-4' />
                {t('plugin.managePlugin')}
              </DropdownMenuItem>
              <DropdownMenuItem
                className='text-destructive'
                onClick={(e) => {
                  e.stopPropagation();
                  openDeleteLocalPluginDialog(plugin);
                }}
              >
                <Trash2 className='mr-2 size-4' />
                {t('common.delete')}
              </DropdownMenuItem>
            </>
          ),
        };
      },
    );

    return [...downloadRows, ...pluginRows];
  }, [
    activeDownloads,
    getInstalledClustersForLocalPlugin,
    getPaginatedLocalPlugins,
    openDeleteLocalPluginDialog,
    openInstallDialog,
    selectedLocalPlugins,
    t,
  ]);

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
          tabs={
            <ModuleNavTabs
              reorderGroupId='packages-plugins'
              items={[
                {
                  key: 'packages',
                  label: t('installer.packageManagement'),
                  href: '/packages',
                  icon: <Package className='size-3.5' />,
                },
                {
                  key: 'plugins',
                  label: t('dock.pluginMarketplace'),
                  href: '/plugins',
                  icon: <Puzzle className='size-3.5' />,
                },
              ]}
              activeKey='plugins'
            />
          }
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

      {/* seed 目录：官方 Maven 尚无该版本构件时的回退清单提示。
          Seed catalog banner: bundled fallback when official Maven lacks this version. */}
      {catalogSource === 'seed' && activeTab === 'available' && !error && (
        <Card className='border-amber-500/40 bg-amber-500/5'>
          <CardContent className='flex gap-2.5 p-3 text-sm text-amber-900 dark:text-amber-200'>
            <AlertTriangle className='mt-0.5 h-4 w-4 shrink-0' />
            <div className='space-y-0.5'>
              <div className='font-medium'>{t('plugin.catalogFromSeedTitle')}</div>
              <p className='text-xs leading-relaxed text-amber-800/90 dark:text-amber-200/80'>
                {t('plugin.catalogFromSeedDesc')}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      <MarketplaceOverviewBento model={overviewModel} />

      {/* 一体化紧凑卡片容器 / Unified Compact Table & Grid Container */}
      <Card className='overflow-hidden rounded-xl border-border/60 shadow-xs'>
        {/* 工具栏第一行：胶囊导航与快捷操作 / Toolbar Row 1: Stat pills bar & Quick actions */}
        <div className='flex flex-wrap items-center justify-between gap-2.5 border-b bg-muted/20 p-3 sm:p-3.5'>
          <StatPillsBar
            items={statItems}
            activeKey={activeTab}
            onChange={(key) =>
              setActiveTab(key as 'available' | 'local' | 'custom')
            }
          />

          <div className='flex flex-wrap items-center gap-2'>
            {activeTab === 'available' && catalogRefreshedAt && (
              <span className='hidden text-xs text-muted-foreground sm:inline-block'>
                {t('plugin.catalogRefreshedAt')}:{' '}
                {formatCatalogRefreshedAt(catalogRefreshedAt)}
              </span>
            )}
            {activeTab === 'local' && selectedLocalPlugins.size > 0 && (
              <div className='flex items-center gap-2'>
                <Badge variant='secondary' className='rounded-full text-xs'>
                  {t('plugin.selectedCount', {
                    count: selectedLocalPlugins.size,
                  })}
                </Badge>
                <Button
                  size='sm'
                  className='h-8 rounded-md text-xs'
                  onClick={() => setIsBatchInstallOpen(true)}
                >
                  <CheckSquare className='mr-1.5 h-3.5 w-3.5' />
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
          <Select
            value={selectedVersion}
            onValueChange={(value) => {
              userOverrideVersionRef.current = true;
              setSelectedVersion(value);
            }}
          >
            <SelectTrigger className='w-[130px] h-8 text-xs' data-testid='plugin-version-select'>
              <SelectValue placeholder={t('plugin.version')} />
            </SelectTrigger>
            <SelectContent>
              {versionOptions.map((version) => (
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
        <CardContent className='space-y-4 p-4'>
          {activeTab === 'available' && (
            <>
              <MarketplaceListToolbar
                title={t('marketplaceUi.listAvailable')}
                count={filteredAvailablePlugins.length}
                viewMode={availableViewMode}
                onViewModeChange={setAvailableViewMode}
              />
              <PluginGrid
                plugins={filteredAvailablePlugins}
                loading={loading}
                error={error}
                viewMode={availableViewMode}
                onViewDetail={handleViewDetail}
                showInstallButton={true}
                onInstall={handleInstallPlugin}
                onDownload={handleDownloadPlugin}
                downloadingPlugins={downloadingPlugins}
                downloadedPlugins={downloadedPlugins}
                downloadProgressMap={downloadProgressMap}
                failedDownloadMap={failedDownloadMap}
                installedClusterCountMap={installedClusterCountMap}
              />
            </>
          )}

          {activeTab === 'local' && (
            <>
              <MarketplaceListToolbar
                title={t('marketplaceUi.listLocal')}
                count={getFilteredLocalPlugins().length}
                viewMode={localViewMode}
                onViewModeChange={setLocalViewMode}
                trailing={
                  getFilteredLocalPlugins().length > 0 ? (
                    <Checkbox
                      checked={
                        getFilteredLocalPlugins().length > 0 &&
                        getFilteredLocalPlugins().every((p) =>
                          selectedLocalPlugins.has(`${p.name}:${p.version}`),
                        )
                      }
                      onCheckedChange={handleSelectAllLocalPlugins}
                      aria-label={t('plugin.batchInstall')}
                    />
                  ) : null
                }
              />
              <MarketplaceMorphCatalog
                items={localCatalogItems}
                viewMode={localViewMode}
                loading={
                  localPluginsLoading &&
                  localPlugins.length === 0 &&
                  activeDownloads.length === 0
                }
                empty={
                  <div className='space-y-1 text-center'>
                    <HardDrive className='mx-auto mb-2 size-10 opacity-50' />
                    <p>{t('plugin.noDownloadedPlugins')}</p>
                    <p className='text-xs'>{t('plugin.downloadFromAvailable')}</p>
                  </div>
                }
                data-testid='plugin-local-catalog'
              />
              {getFilteredLocalPlugins().length > 0 && (
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
              )}
            </>
          )}

          {activeTab === 'custom' && (
            <div className='space-y-4'>
              <MarketplaceListToolbar
                title={t('marketplaceUi.listCustom')}
                count={0}
                viewMode={availableViewMode}
                onViewModeChange={setAvailableViewMode}
              />
              <div className='rounded-xl border border-dashed border-border/60 py-12 text-center'>
                <Upload className='mx-auto mb-4 size-12 text-muted-foreground opacity-60' />
                <p className='text-sm text-muted-foreground'>
                  {t('marketplaceUi.customComingSoon')}
                </p>
              </div>
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
              // 弹窗关闭时静默同步本地已下载插件与安装状态。
              // Silently refresh local plugins and cluster install status on dialog close.
              void loadLocalPlugins();
            }
          }}
          onDownloadComplete={() => {
            // 下载完成时触发主界面数据刷新，使卡片与列表即时呈现就绪状态。
            // Trigger main view refresh upon download completion to reflect ready state immediately.
            void loadLocalPlugins();
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
