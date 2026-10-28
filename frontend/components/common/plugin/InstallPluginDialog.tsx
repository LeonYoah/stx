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

'use client';

import {useState, useEffect, useCallback, useMemo} from 'react';
import {useTranslations} from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent} from '@/components/ui/card';
import {Progress} from '@/components/ui/progress';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Loader2,
  Download,
  Server,
  AlertCircle,
  Info,
  CloudDownload,
  Trash2,
  Layers3,
} from 'lucide-react';
import {toast} from 'sonner';
import type {
  InstalledPlugin,
  Plugin,
  PluginDependency,
  PluginDownloadProgress,
} from '@/lib/services/plugin';
import {PluginService, formatPluginNodeStatuses} from '@/lib/services/plugin';
import {ClusterService} from '@/lib/services/cluster';
import {ClusterStatus} from '@/lib/services/cluster';
import type {ClusterInfo} from '@/lib/services/cluster';

interface InstallPluginDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plugin: Plugin;
  version: string;
  selectedProfileKeys?: string[];
  attachedConnectors?: string[];
  dependencies?: PluginDependency[];
  /** 下载完成回调 / Callback invoked when plugin download is completed */
  onDownloadComplete?: () => void;
}

interface ClusterPluginStatus {
  cluster: ClusterInfo;
  installedPlugin: InstalledPlugin | null;
  versionMismatchPlugin: InstalledPlugin | null;
  loading: boolean;
}

interface PendingRestartCluster {
  id: number;
  name: string;
}

export function isClusterRuntimeAvailable(cluster: ClusterInfo): boolean {
  if (cluster.status !== ClusterStatus.RUNNING) {
    return cluster.status === ClusterStatus.STOPPED;
  }
  return !(cluster.online_nodes === 0 || cluster.health_status === 'unhealthy');
}

export function getClusterDisplayState(cluster: ClusterInfo): {
  variant: 'default' | 'secondary' | 'destructive';
  labelKey: string;
} {
  if (
    cluster.status === ClusterStatus.RUNNING &&
    !isClusterRuntimeAvailable(cluster)
  ) {
    return {
      variant: 'destructive',
      labelKey: 'cluster.healthStatuses.unhealthy',
    };
  }
  return {
    variant: cluster.status === ClusterStatus.RUNNING ? 'default' : 'secondary',
    labelKey: `cluster.statuses.${cluster.status}`,
  };
}

function dedupeStrings(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean)));
}

function getDependencyFileName(dep: PluginDependency): string {
  if (dep.original_file_name?.trim()) {
    return dep.original_file_name.trim();
  }
  const artifact = dep.artifact_id?.trim() || 'dependency';
  const version = dep.version?.trim();
  return version ? `${artifact}-${version}.jar` : `${artifact}.jar`;
}

export function InstallPluginDialog({
  open,
  onOpenChange,
  plugin,
  version,
  selectedProfileKeys = [],
  attachedConnectors = [],
  dependencies = [],
  onDownloadComplete,
}: InstallPluginDialogProps) {
  const t = useTranslations();
  const [clusterStatuses, setClusterStatuses] = useState<ClusterPluginStatus[]>(
    [],
  );
  const [loadingClusters, setLoadingClusters] = useState(true);
  const [downloading, setDownloading] = useState(false);
  const [downloadProgress, setDownloadProgress] =
    useState<PluginDownloadProgress | null>(null);
  const [isDownloaded, setIsDownloaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [operatingClusterId, setOperatingClusterId] = useState<number | null>(
    null,
  );
  const [pendingRestartCluster, setPendingRestartCluster] =
    useState<PendingRestartCluster | null>(null);
  const [restartingClusterId, setRestartingClusterId] = useState<number | null>(
    null,
  );

  const combinedAttachedConnectors = useMemo(
    () =>
      dedupeStrings([
        ...attachedConnectors,
        ...dependencies
          .filter((item) => item.target_dir === 'connectors')
          .map((item) => item.artifact_id),
      ]),
    [attachedConnectors, dependencies],
  );
  const extraDependencies = useMemo(
    () => dependencies.filter((item) => item.target_dir !== 'connectors'),
    [dependencies],
  );
  const dependencyTargetDirs = useMemo(
    () => dedupeStrings(extraDependencies.map((item) => item.target_dir)),
    [extraDependencies],
  );
  const dependencyNote = useMemo(() => {
    if (dependencyTargetDirs.length === 0) {
      return t('plugin.installNoteNoDependencies');
    }
    return t('plugin.installNoteDependencies', {
      dirs: dependencyTargetDirs.join(' / '),
    });
  }, [dependencyTargetDirs, t]);
  const requiresClusterRestart = useMemo(
    () => extraDependencies.some((item) => item.target_dir === 'lib'),
    [extraDependencies],
  );
  const downloadProgressTitle = useMemo(() => {
    if (downloadProgress?.current_artifact_kind === 'dependency') {
      return t('plugin.downloadingDependency');
    }
    if (downloadProgress?.current_artifact_kind === 'connector') {
      return t('plugin.downloadingConnector');
    }
    return t('plugin.downloading');
  }, [downloadProgress?.current_artifact_kind, t]);

  const checkDownloadStatus = useCallback(async () => {
    try {
      const status = await PluginService.getDownloadStatus(
        plugin.name,
        version,
        selectedProfileKeys,
      );
      setDownloadProgress(status);
      if (status.status === 'completed') {
        setIsDownloaded(true);
        setDownloading(false);
        onDownloadComplete?.();
      } else if (status.status === 'downloading') {
        setDownloading(true);
      } else if (status.status === 'failed') {
        setDownloading(false);
        setError(status.error || t('plugin.downloadFailed'));
      } else {
        // 兜底校验本地插件：若下载完成并落盘，后端可能已清理活跃下载并返回 not_started。
        // Fallback check local plugins: when download completes, active task is removed and status may show not_started.
        const localPlugins = await PluginService.listLocalPlugins().catch(() => []);
        const isLocal = (localPlugins || []).some(
          (p) => p.name === plugin.name && p.version === version,
        );
        if (isLocal) {
          setIsDownloaded(true);
          setDownloading(false);
          setDownloadProgress({
            ...status,
            status: 'completed',
            progress: 100,
          });
          onDownloadComplete?.();
        } else if (downloading) {
          // 若既非 downloading 也非 completed 且本地尚未存在，退出下载中标记避免停留在 0%。
          // If neither downloading nor completed and not local, clear downloading flag to avoid sticking at 0%.
          setDownloading(false);
        }
      }
    } catch {
      setIsDownloaded(false);
    }
  }, [downloading, onDownloadComplete, plugin.name, selectedProfileKeys, t, version]);

  const loadClusters = useCallback(async () => {
    setLoadingClusters(true);
    setError(null);
    try {
      const result = await ClusterService.getClusters({current: 1, size: 100});
      const availableClusters = result.clusters.filter(
        (c: ClusterInfo) =>
          c.status === ClusterStatus.RUNNING ||
          c.status === ClusterStatus.STOPPED,
      );

      const statuses: ClusterPluginStatus[] = availableClusters.map(
        (cluster: ClusterInfo) => ({
          cluster,
          installedPlugin: null,
          versionMismatchPlugin: null,
          loading: true,
        }),
      );
      setClusterStatuses(statuses);

      const updatedStatuses = await Promise.all(
        availableClusters.map(async (cluster: ClusterInfo) => {
          try {
            const installedPlugins = await PluginService.listInstalledPlugins(
              cluster.id,
            );
            const installedPlugin = installedPlugins.find(
              (item: InstalledPlugin) =>
                item.plugin_name === plugin.name && item.version === version,
            );
            const versionMismatchPlugin = installedPlugin
              ? null
              : installedPlugins.find(
                  (item: InstalledPlugin) => item.plugin_name === plugin.name,
                ) || null;
            return {
              cluster,
              installedPlugin: installedPlugin || null,
              versionMismatchPlugin,
              loading: false,
            };
          } catch {
            return {
              cluster,
              installedPlugin: null,
              versionMismatchPlugin: null,
              loading: false,
            };
          }
        }),
      );
      setClusterStatuses(updatedStatuses);
    } catch (loadError) {
      const errorMsg =
        loadError instanceof Error ? loadError.message : t('cluster.loadError');
      setError(errorMsg);
      setClusterStatuses([]);
    } finally {
      setLoadingClusters(false);
    }
  }, [plugin.name, t, version]);

  useEffect(() => {
    if (open) {
      void loadClusters();
      void checkDownloadStatus();
    }
  }, [checkDownloadStatus, loadClusters, open]);

  useEffect(() => {
    if (!downloading) {
      return undefined;
    }
    const interval = setInterval(() => {
      void checkDownloadStatus();
    }, 1000);
    return () => clearInterval(interval);
  }, [checkDownloadStatus, downloading]);

  const handleDownload = async () => {
    setDownloading(true);
    setError(null);
    try {
      await PluginService.downloadPlugin(
        plugin.name,
        version,
        undefined,
        selectedProfileKeys,
      );
      toast.info(
        t('plugin.downloadStarted', {name: plugin.display_name || plugin.name}),
      );
    } catch (downloadError) {
      const errorMsg =
        downloadError instanceof Error
          ? downloadError.message
          : t('plugin.downloadFailed');
      setError(errorMsg);
      toast.error(errorMsg);
      setDownloading(false);
    }
  };

  const handleInstall = async (clusterId: number) => {
    if (!isDownloaded) {
      toast.error(t('plugin.downloadFirst'));
      return;
    }

    setOperatingClusterId(clusterId);
    try {
      await PluginService.installPluginAndWait(
        clusterId,
        plugin.name,
        version,
        undefined,
        selectedProfileKeys,
      );
      toast.success(
        t('plugin.installSuccess', {name: plugin.display_name || plugin.name}),
      );
      await refreshClusterStatus(clusterId);

      const targetCluster = clusterStatuses.find(
        (item) => item.cluster.id === clusterId,
      )?.cluster;
      if (requiresClusterRestart && targetCluster) {
        if (targetCluster.status === ClusterStatus.RUNNING) {
          setPendingRestartCluster({
            id: targetCluster.id,
            name: targetCluster.name,
          });
        } else {
          toast.info(
            t('plugin.restartClusterOnNextStartHint', {
              cluster: targetCluster.name,
            }),
          );
        }
      }
    } catch (installError) {
      const errorMsg =
        installError instanceof Error
          ? installError.message
          : t('plugin.installFailed');
      toast.error(errorMsg);
    } finally {
      setOperatingClusterId(null);
    }
  };

  const handleConfirmClusterRestart = async () => {
    if (!pendingRestartCluster) {
      return;
    }
    setRestartingClusterId(pendingRestartCluster.id);
    try {
      const result = await ClusterService.restartClusterSafe(
        pendingRestartCluster.id,
      );
      if (result.success) {
        toast.success(
          t('plugin.restartClusterAfterInstallSuccess', {
            cluster: pendingRestartCluster.name,
          }),
        );
        setPendingRestartCluster(null);
      } else {
        toast.error(
          result.error ||
            t('plugin.restartClusterAfterInstallFailed', {
              cluster: pendingRestartCluster.name,
            }),
        );
      }
    } catch (restartError) {
      const errorMsg =
        restartError instanceof Error
          ? restartError.message
          : t('plugin.restartClusterAfterInstallFailed', {
              cluster: pendingRestartCluster.name,
            });
      toast.error(errorMsg);
    } finally {
      setRestartingClusterId(null);
    }
  };

  const handleUninstall = async (clusterId: number) => {
    setOperatingClusterId(clusterId);
    try {
      await PluginService.uninstallPlugin(clusterId, plugin.name);
      toast.success(t('plugin.uninstallSuccess'));
      await refreshClusterStatus(clusterId);
    } catch (uninstallError) {
      const errorMsg =
        uninstallError instanceof Error
          ? uninstallError.message
          : t('plugin.uninstallFailed');
      toast.error(errorMsg);
    } finally {
      setOperatingClusterId(null);
    }
  };

  const refreshClusterStatus = async (clusterId: number) => {
    try {
      const installedPlugins =
        await PluginService.listInstalledPlugins(clusterId);
      const installedPlugin = installedPlugins.find(
        (item: InstalledPlugin) =>
          item.plugin_name === plugin.name && item.version === version,
      );
      const versionMismatchPlugin = installedPlugin
        ? null
        : installedPlugins.find(
            (item: InstalledPlugin) => item.plugin_name === plugin.name,
          ) || null;
      setClusterStatuses((prev) =>
        prev.map((status) =>
          status.cluster.id === clusterId
            ? {
                ...status,
                installedPlugin: installedPlugin || null,
                versionMismatchPlugin,
              }
            : status,
        ),
      );
    } catch {
      // ignore
    }
  };

  const handleClose = () => {
    if (!downloading && !operatingClusterId) {
      setError(null);
      setDownloadProgress(null);
      setPendingRestartCluster(null);
      onOpenChange(false);
    }
  };

  const getStatusBadge = (
    installedPlugin: InstalledPlugin | null,
    hasVersionMismatch: boolean,
  ) => {
    if (!installedPlugin) {
      if (hasVersionMismatch) {
        return <Badge variant='secondary'>{t('plugin.versionMismatch')}</Badge>;
      }
      return (
        <Badge variant='outline' className='text-muted-foreground'>
          {t('plugin.notInstalled')}
        </Badge>
      );
    }
    if (installedPlugin.status === 'partial') {
      return (
        <Badge
          variant='destructive'
          title={formatPluginNodeStatuses(installedPlugin, t)}
        >
          {t('plugin.partialInstalled')}
        </Badge>
      );
    }
    return <Badge variant='default'>{t('plugin.installedLabel')}</Badge>;
  };

  return (
    <>
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className='w-[96vw] max-w-[96vw] sm:max-w-[920px] max-h-[88vh] overflow-hidden p-0 gap-0'>
          <DialogHeader className='px-6 pt-6 pb-4 shrink-0'>
            <DialogTitle className='flex items-center gap-2'>
              <Download className='h-5 w-5' />
              {t('plugin.managePlugin')}
            </DialogTitle>
            <DialogDescription>
              {t('plugin.managePluginDesc')}
            </DialogDescription>
          </DialogHeader>

          <ScrollArea className='max-h-[calc(88vh-92px)]'>
            <div className='space-y-4 px-6 pb-6'>
              <Card className='bg-muted/50'>
                <CardContent className='pt-4 pb-4 space-y-3'>
                  <div className='flex items-center justify-between gap-4'>
                    <div>
                      <div className='font-medium'>
                        {plugin.display_name || plugin.name}
                      </div>
                      <p className='text-sm text-muted-foreground'>
                        {plugin.name}
                      </p>
                    </div>
                    <Badge variant='secondary'>v{version}</Badge>
                  </div>
                  {selectedProfileKeys.length > 0 && (
                    <div className='flex flex-wrap gap-2'>
                      <span className='text-xs text-muted-foreground'>
                        {t('plugin.selectedProfiles')}
                      </span>
                      {selectedProfileKeys.map((key) => (
                        <Badge key={key} variant='outline'>
                          {key}
                        </Badge>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>

              {!isDownloaded && (
                <Card className='border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950'>
                  <CardContent className='pt-4 pb-4 space-y-3'>
                    <div className='flex items-center justify-between gap-4'>
                      <div className='flex items-center gap-2 text-amber-800 dark:text-amber-200'>
                        <AlertCircle className='h-4 w-4' />
                        <span className='text-sm'>
                          {t('plugin.downloadFirst')}
                        </span>
                      </div>
                      <Button
                        size='sm'
                        onClick={handleDownload}
                        disabled={downloading}
                      >
                        {downloading ? (
                          <Loader2 className='h-4 w-4 mr-2 animate-spin' />
                        ) : (
                          <CloudDownload className='h-4 w-4 mr-2' />
                        )}
                        {t('plugin.download')}
                      </Button>
                    </div>
                    {downloading && downloadProgress && (
                      <div className='space-y-2'>
                        <div className='flex items-center justify-between text-sm'>
                          <span>{downloadProgressTitle}</span>
                          <span>{downloadProgress.progress}%</span>
                        </div>
                        {downloadProgress.current_artifact && (
                          <div className='text-xs text-muted-foreground'>
                            {downloadProgress.current_artifact_kind ===
                            'dependency'
                              ? t('plugin.currentDownloadingDependency', {
                                  artifact: downloadProgress.current_artifact,
                                })
                              : t('plugin.currentDownloadingConnector', {
                                  artifact: downloadProgress.current_artifact,
                                })}
                          </div>
                        )}
                        <Progress
                          value={downloadProgress.progress}
                          className='h-2'
                        />
                        <div className='flex flex-wrap gap-3 text-xs text-muted-foreground'>
                          <span>
                            {t('plugin.connectorProgress', {
                              completed:
                                downloadProgress.connector_completed || 0,
                              total: downloadProgress.connector_count || 0,
                            })}
                          </span>
                          <span>
                            {t('plugin.dependencyProgress', {
                              completed:
                                downloadProgress.dependency_completed || 0,
                              total: downloadProgress.dependency_count || 0,
                            })}
                          </span>
                        </div>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}

              {error && (
                <Card className='border-destructive bg-destructive/10'>
                  <CardContent className='pt-4 pb-4'>
                    <div className='flex items-center gap-2 text-destructive'>
                      <AlertCircle className='h-4 w-4' />
                      <span className='text-sm'>{error}</span>
                    </div>
                  </CardContent>
                </Card>
              )}

              <Card>
                <CardContent className='pt-4 pb-4 space-y-4'>
                  <div>
                    <div className='flex items-center gap-2 text-sm font-medium'>
                      <Layers3 className='h-4 w-4' />
                      {t('plugin.automaticInstallContent')}
                    </div>
                    <p className='mt-1 text-xs text-muted-foreground'>
                      {t('plugin.automaticInstallContentDesc')}
                    </p>
                  </div>

                  <div className='space-y-2'>
                    <div className='text-sm font-medium'>
                      {t('plugin.attachedConnectors')}
                    </div>
                    {combinedAttachedConnectors.length === 0 ? (
                      <p className='text-sm text-muted-foreground'>
                        {t('plugin.noAttachedConnectors')}
                      </p>
                    ) : (
                      <div className='flex flex-wrap gap-2'>
                        {combinedAttachedConnectors.map((artifact) => (
                          <Badge key={artifact} variant='secondary'>
                            {artifact}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className='space-y-2'>
                    <div className='flex items-center justify-between gap-3'>
                      <div className='text-sm font-medium'>
                        {t('plugin.extraDependencies')}
                      </div>
                      {dependencyTargetDirs.length > 0 && (
                        <div className='flex flex-wrap gap-2'>
                          {dependencyTargetDirs.map((targetDir) => (
                            <Badge
                              key={targetDir}
                              variant='outline'
                              className='max-w-[240px] whitespace-normal break-all'
                            >
                              {targetDir}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>
                    {extraDependencies.length === 0 ? (
                      <p className='text-sm text-muted-foreground'>
                        {t('plugin.noExtraDependencies')}
                      </p>
                    ) : (
                      <div className='overflow-x-auto rounded-lg border'>
                        <Table className='min-w-[760px]'>
                          <TableHeader>
                            <TableRow>
                              <TableHead>{t('plugin.source')}</TableHead>
                              <TableHead>{t('plugin.groupId')}</TableHead>
                              <TableHead>{t('plugin.artifactId')}</TableHead>
                              <TableHead>{t('plugin.version')}</TableHead>
                              <TableHead>{t('plugin.targetDir')}</TableHead>
                              <TableHead>{t('plugin.fileName')}</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {extraDependencies.map((dep, index) => (
                              <TableRow
                                key={`${dep.group_id}:${dep.artifact_id}:${index}`}
                              >
                                <TableCell>
                                  <Badge
                                    variant={
                                      dep.source_type === 'upload'
                                        ? 'default'
                                        : 'secondary'
                                    }
                                  >
                                    {dep.source_type === 'upload'
                                      ? t('plugin.sourceUpload')
                                      : t('plugin.sourceMaven')}
                                  </Badge>
                                </TableCell>
                                <TableCell className='font-mono text-xs whitespace-nowrap'>
                                  {dep.group_id}
                                </TableCell>
                                <TableCell className='font-mono text-xs whitespace-nowrap'>
                                  {dep.artifact_id}
                                </TableCell>
                                <TableCell className='font-mono text-xs whitespace-nowrap'>
                                  {dep.version}
                                </TableCell>
                                <TableCell className='min-w-[220px] align-top'>
                                  <Badge
                                    variant='outline'
                                    className='text-xs whitespace-normal break-all text-left'
                                  >
                                    {dep.target_dir}
                                  </Badge>
                                </TableCell>
                                <TableCell className='text-xs break-all'>
                                  {getDependencyFileName(dep)}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>

              <div className='space-y-2'>
                <div className='text-sm font-medium'>
                  {t('plugin.clusterList')}
                </div>
                {loadingClusters ? (
                  <div className='flex items-center justify-center py-8'>
                    <Loader2 className='h-6 w-6 animate-spin text-muted-foreground' />
                  </div>
                ) : clusterStatuses.length === 0 ? (
                  <Card className='border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950'>
                    <CardContent className='pt-4 pb-4'>
                      <div className='flex items-center gap-2 text-amber-800 dark:text-amber-200'>
                        <AlertCircle className='h-4 w-4' />
                        <span className='text-sm'>
                          {t('plugin.noClustersAvailable')}
                        </span>
                      </div>
                    </CardContent>
                  </Card>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('cluster.name')}</TableHead>
                        <TableHead>{t('cluster.status')}</TableHead>
                        <TableHead>{t('plugin.installStatus')}</TableHead>
                        <TableHead className='text-right'>
                          {t('common.actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {clusterStatuses.map(
                        ({
                          cluster,
                          installedPlugin,
                          versionMismatchPlugin,
                          loading,
                        }) => {
                          const isOperating = operatingClusterId === cluster.id;
                          const partialInstall =
                            installedPlugin?.status === 'partial';
                          const installed =
                            Boolean(installedPlugin) && !partialInstall;
                          const clusterRuntimeAvailable =
                            isClusterRuntimeAvailable(cluster);
                          const clusterDisplayState =
                            getClusterDisplayState(cluster);
                          const clusterVersionMismatch =
                            Boolean(cluster.version) &&
                            cluster.version !== version;
                          const hasVersionMismatch =
                            clusterVersionMismatch ||
                            Boolean(versionMismatchPlugin);
                          return (
                            <TableRow key={cluster.id}>
                              <TableCell>
                                <div className='flex items-center gap-2'>
                                  <Server className='h-4 w-4 text-muted-foreground' />
                                  <span className='font-medium'>
                                    {cluster.name}
                                  </span>
                                </div>
                              </TableCell>
                              <TableCell>
                                <Badge variant={clusterDisplayState.variant}>
                                  {t(clusterDisplayState.labelKey)}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                {loading ? (
                                  <Loader2 className='h-4 w-4 animate-spin' />
                                ) : (
                                  <div className='space-y-1'>
                                    {getStatusBadge(
                                      installedPlugin,
                                      hasVersionMismatch,
                                    )}
                                    {hasVersionMismatch && (
                                      <div className='text-xs text-muted-foreground'>
                                        {versionMismatchPlugin
                                          ? t(
                                              'plugin.versionMismatchInstalledHint',
                                              {
                                                clusterVersion:
                                                  cluster.version || '-',
                                                installedVersion:
                                                  versionMismatchPlugin.version,
                                              },
                                            )
                                          : t(
                                              'plugin.versionMismatchClusterHint',
                                              {
                                                clusterVersion:
                                                  cluster.version || '-',
                                                pluginVersion: version,
                                              },
                                            )}
                                      </div>
                                    )}
                                  </div>
                                )}
                              </TableCell>
                              <TableCell className='text-right'>
                                {loading ? (
                                  <Loader2 className='h-4 w-4 animate-spin' />
                                ) : hasVersionMismatch ? (
                                  <Button size='sm' variant='outline' disabled>
                                    {t('plugin.install')}
                                  </Button>
                                ) : !installed ? (
                                  <div className='flex items-center justify-end gap-2'>
                                    <Button
                                      size='sm'
                                      variant='outline'
                                      onClick={() => handleInstall(cluster.id)}
                                      disabled={
                                        !isDownloaded ||
                                        isOperating ||
                                        !clusterRuntimeAvailable
                                      }
                                    >
                                      {isOperating ? (
                                        <Loader2 className='h-4 w-4 mr-1 animate-spin' />
                                      ) : (
                                        <Download className='h-4 w-4 mr-1' />
                                      )}
                                      {partialInstall
                                        ? t('plugin.retryInstall')
                                        : t('plugin.install')}
                                    </Button>
                                    {partialInstall && (
                                      <Button
                                        size='sm'
                                        variant='outline'
                                        onClick={() => handleUninstall(cluster.id)}
                                        disabled={
                                          isOperating || !clusterRuntimeAvailable
                                        }
                                        className='text-destructive hover:text-destructive'
                                      >
                                        <Trash2 className='h-4 w-4 mr-1' />
                                        {t('plugin.uninstall')}
                                      </Button>
                                    )}
                                  </div>
                                ) : (
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    onClick={() => handleUninstall(cluster.id)}
                                    disabled={
                                      isOperating || !clusterRuntimeAvailable
                                    }
                                    className='text-destructive hover:text-destructive'
                                  >
                                    {isOperating ? (
                                      <Loader2 className='h-4 w-4 mr-1 animate-spin' />
                                    ) : (
                                      <Trash2 className='h-4 w-4 mr-1' />
                                    )}
                                    {t('plugin.uninstall')}
                                  </Button>
                                )}
                              </TableCell>
                            </TableRow>
                          );
                        },
                      )}
                    </TableBody>
                  </Table>
                )}
              </div>

              <Card className='bg-blue-50 dark:bg-blue-950 border-blue-200 dark:border-blue-800'>
                <CardContent className='pt-4 pb-4'>
                  <div className='flex items-start gap-2'>
                    <Info className='h-4 w-4 text-blue-600 mt-0.5' />
                    <p className='text-xs text-blue-800 dark:text-blue-200'>
                      {t('plugin.installNoteDynamic', {
                        connectorDir: 'connectors/',
                        dependencyMessage: dependencyNote,
                      })}
                    </p>
                  </div>
                </CardContent>
              </Card>
            </div>
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={Boolean(pendingRestartCluster)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen && restartingClusterId === null) {
            setPendingRestartCluster(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('plugin.restartClusterAfterInstallTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {pendingRestartCluster
                ? t('plugin.restartClusterAfterInstallPrompt', {
                    cluster: pendingRestartCluster.name,
                  })
                : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={restartingClusterId !== null}>
              {t('common.cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault();
                void handleConfirmClusterRestart();
              }}
              disabled={restartingClusterId !== null}
            >
              {restartingClusterId !== null ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : null}
              {t('plugin.restartClusterAfterInstallAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
