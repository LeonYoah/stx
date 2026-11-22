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

/**
 * Cluster Plugins Component
 * 集群插件组件
 *
 * Displays installed plugins for a cluster. SeaTunnel plugins are not dynamically
 * loaded, so enable/disable state is not used.
 * 显示集群的已安装插件。SeaTunnel 插件非动态加载，无启用/禁用状态。
 */

import {useState, useEffect, useCallback} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
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
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {toast} from '@/lib/toast';
import {Puzzle, RefreshCw, Trash2, Loader2, Plus, Eye, Layers3, Package} from 'lucide-react';
import {useRouter} from 'next/navigation';
import services from '@/lib/services';
import {InstalledPlugin, PluginDependency, formatPluginNodeStatuses} from '@/lib/services/plugin/types';
import {formatDateTimeTz} from '@/lib/timezone';

interface ClusterPluginsProps {
  clusterId: number;
}

/**
 * Cluster Plugins Component
 * 集群插件组件
 */
export function ClusterPlugins({clusterId}: ClusterPluginsProps) {
  const t = useTranslations();
  const router = useRouter();

  // Data state / 数据状态
  const [plugins, setPlugins] = useState<InstalledPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [operating, setOperating] = useState<string | null>(null);

  // Dialog state / 对话框状态
  const [pluginToUninstall, setPluginToUninstall] = useState<InstalledPlugin | null>(null);
  const [detailPlugin, setDetailPlugin] = useState<InstalledPlugin | null>(null);

  /**
   * Load installed plugins
   * 加载已安装插件
   */
  const loadPlugins = useCallback(async () => {
    setLoading(true);
    try {
      const data = await services.plugin.listInstalledPlugins(clusterId);
      setPlugins(data || []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('plugin.loadError'));
    } finally {
      setLoading(false);
    }
  }, [clusterId, t]);

  useEffect(() => {
    loadPlugins();
  }, [loadPlugins]);

  /**
   * Handle uninstall plugin
   * 处理卸载插件
   */
  const handleUninstall = async () => {
    if (!pluginToUninstall) {return;}

    setOperating(pluginToUninstall.plugin_name);
    try {
      await services.plugin.uninstallPlugin(clusterId, pluginToUninstall.plugin_name);
      toast.success(t('plugin.uninstallSuccess'));
      loadPlugins();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('plugin.uninstallError'));
    } finally {
      setOperating(null);
      setPluginToUninstall(null);
    }
  };

  /**
   * Navigate to plugins page
   * 导航到插件页面
   */
  const goToPluginsPage = () => {
    router.push('/plugins');
  };

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Puzzle className="h-5 w-5" />
            {t('plugin.installedPlugins')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="flex items-center gap-2">
            <Puzzle className="h-5 w-5" />
            {t('plugin.installedPlugins')}
            {plugins.length > 0 && (
              <Badge variant="secondary" className="ml-2">
                {plugins.length}
              </Badge>
            )}
          </CardTitle>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={loadPlugins}>
              <RefreshCw className="h-4 w-4 mr-2" />
              {t('common.refresh')}
            </Button>
            <Button size="sm" onClick={goToPluginsPage}>
              <Plus className="h-4 w-4 mr-2" />
              {t('plugin.addPlugin')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {plugins.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <Puzzle className="h-12 w-12 mx-auto mb-4 opacity-50" />
              <p>{t('plugin.noInstalledPlugins')}</p>
              <Button variant="outline" className="mt-4" onClick={goToPluginsPage}>
                <Plus className="h-4 w-4 mr-2" />
                {t('plugin.browsePlugins')}
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('plugin.name')}</TableHead>
                  <TableHead>{t('plugin.category.label')}</TableHead>
                  <TableHead>{t('plugin.version')}</TableHead>
                  <TableHead>{t('plugin.installedAt')}</TableHead>
                  <TableHead className="w-[260px] text-right">{t('plugin.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {plugins.map((plugin) => {
                  const isOperating = operating === plugin.plugin_name;
                  return (
                    <TableRow
                      key={plugin.id}
                      data-testid={`cluster-plugin-row-${plugin.plugin_name}-${plugin.version}`}
                    >
                      <TableCell className="font-medium">
                        <div className="flex items-center gap-2">
                          <span>{plugin.plugin_name}</span>
                          {plugin.bundled && (
                            <Badge
                              variant="outline"
                              data-testid={`cluster-plugin-bundled-${plugin.plugin_name}`}
                            >
                              {t('plugin.bundled')}
                            </Badge>
                          )}
                          {plugin.status === 'partial' && (
                            <Badge
                              variant="destructive"
                              title={`${t('plugin.partialInstallTip')} ${formatPluginNodeStatuses(plugin, t)}`}
                              data-testid={`cluster-plugin-partial-${plugin.plugin_name}`}
                            >
                              {t('plugin.partialInstalled')}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        {plugin.category ? (
                          <Badge variant="outline">
                            {t(`plugin.category.${plugin.category}`)}
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell>{plugin.version}</TableCell>
                      <TableCell>
                        {formatDateTimeTz(plugin.installed_at)}
                      </TableCell>
                      <TableCell className="w-[260px]">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            data-testid={`cluster-plugin-view-${plugin.plugin_name}-${plugin.version}`}
                            onClick={() => setDetailPlugin(plugin)}
                          >
                            <Eye className="h-4 w-4 mr-1" />
                            {t('plugin.viewDetails')}
                          </Button>
                          {!plugin.bundled && (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setPluginToUninstall(plugin)}
                              disabled={isOperating}
                              title={t('plugin.uninstall')}
                            >
                              <Trash2 className="h-4 w-4 text-destructive mr-1" />
                              <span className="text-destructive">{t('plugin.uninstall')}</span>
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog / 卸载确认对话框 */}
      <AlertDialog open={!!pluginToUninstall} onOpenChange={() => setPluginToUninstall(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('plugin.uninstallPlugin')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('plugin.uninstallConfirm', {
                name: pluginToUninstall?.plugin_name ?? '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleUninstall}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <InstalledPluginDetailDialog
        open={!!detailPlugin}
        onOpenChange={(open) => {
          if (!open) {
            setDetailPlugin(null);
          }
        }}
        plugin={detailPlugin}
      />
    </>
  );
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

function splitDependencies(items: PluginDependency[]) {
  const attachedConnectors = dedupeStrings([
    ...items.filter((item) => item.target_dir === 'connectors').map((item) => item.artifact_id),
  ]);
  const extraDependencies = items.filter((item) => item.target_dir !== 'connectors');
  return {attachedConnectors, extraDependencies};
}

interface InstalledPluginDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plugin: InstalledPlugin | null;
}

function InstalledPluginDetailDialog({
  open,
  onOpenChange,
  plugin,
}: InstalledPluginDetailDialogProps) {
  const t = useTranslations();

  const selectedProfiles = plugin?.selected_profile_keys || [];
  const rawDependencies = plugin?.dependencies || [];
  const metadataAttachedConnectors = plugin?.attached_connectors || [];
  const {attachedConnectors: attachedConnectorsFromDeps, extraDependencies} =
    splitDependencies(rawDependencies);
  const attachedConnectors = dedupeStrings([
    ...metadataAttachedConnectors,
    ...attachedConnectorsFromDeps,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid={`cluster-plugin-detail-dialog-${plugin?.plugin_name || 'empty'}`}
        className="flex max-h-[88vh] w-[96vw] max-w-[96vw] flex-col gap-0 overflow-hidden p-0 sm:max-w-[920px] lg:max-w-[1000px]"
      >
        <DialogHeader className="shrink-0 border-b-[0.5px] border-border/80 bg-muted/10 px-6 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2">
            <Puzzle className="h-5 w-5" />
            {plugin?.plugin_name || t('plugin.viewDetails')}
          </DialogTitle>
          <DialogDescription>
            {plugin
              ? t('plugin.clusterPluginDetailDesc', {
                  version: plugin.version,
                  installedAt: formatDateTimeTz(plugin.installed_at),
                })
              : ''}
          </DialogDescription>
        </DialogHeader>

        {plugin && (
          /* 单层扁平内容画布：去掉弹窗内 6 张嵌套 Card，用留白和 0.5px 细线分隔各组。
             Single-layer flat surface without nested Cards; groups are separated by whitespace and 0.5px hairlines. */
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="px-6 py-5">
              {/* 基础元信息 / Basic metadata */}
              <div className="grid gap-4 pb-5 sm:grid-cols-3">
                <div>
                  <div className="text-xs text-muted-foreground">{t('plugin.version')}</div>
                  <div className="mt-1.5 font-mono text-sm font-medium">{plugin.version}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">{t('plugin.category.label')}</div>
                  <div className="mt-1.5">
                    {plugin.category ? (
                      <Badge variant="outline">{t(`plugin.category.${plugin.category}`)}</Badge>
                    ) : (
                      '—'
                    )}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">{t('plugin.installPath')}</div>
                  <div className="mt-1.5 break-all font-mono text-xs text-foreground">
                    {plugin.install_path || '—'}
                  </div>
                </div>
              </div>

              {/* 已选场景画像 / Selected profiles */}
              <div className="space-y-2.5 border-t-[0.5px] border-border/80 py-5">
                <h4 className="flex items-center gap-2 text-sm font-semibold">
                  <Layers3 className="h-4 w-4 text-primary" />
                  {t('plugin.selectedProfiles')}
                </h4>
                {selectedProfiles.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('plugin.noSelectedProfiles')}
                  </p>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {selectedProfiles.map((profileKey) => (
                      <Badge key={profileKey} variant="secondary">
                        {profileKey}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>

              {/* 附属连接器 / Attached connectors */}
              <div className="space-y-2.5 border-t-[0.5px] border-border/80 py-5">
                <h4 className="flex items-center gap-2 text-sm font-semibold">
                  <Package className="h-4 w-4 text-primary" />
                  {t('plugin.attachedConnectors')}
                </h4>
                {attachedConnectors.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('plugin.noAttachedConnectors')}
                  </p>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {attachedConnectors.map((artifactId) => (
                      <Badge key={artifactId} variant="outline">
                        {artifactId}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>

              {/* 额外依赖 / Extra dependencies */}
              <div className="space-y-3 border-t-[0.5px] border-border/80 pt-5">
                <h4 className="text-sm font-semibold">{t('plugin.extraDependencies')}</h4>
                {extraDependencies.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('plugin.noExtraDependencies')}
                  </p>
                ) : (
                  <div className="overflow-x-auto">
                    <Table className="min-w-[760px] [&_tr]:border-b-[0.5px] [&_tr]:border-border/70">
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('plugin.groupId')}</TableHead>
                          <TableHead>{t('plugin.artifactId')}</TableHead>
                          <TableHead>{t('plugin.version')}</TableHead>
                          <TableHead>{t('plugin.fileName')}</TableHead>
                          <TableHead>{t('plugin.targetDir')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {extraDependencies.map((dep) => (
                          <TableRow
                            key={`${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}`}
                          >
                            <TableCell className="font-mono text-xs">{dep.group_id}</TableCell>
                            <TableCell className="font-medium">{dep.artifact_id}</TableCell>
                            <TableCell>{dep.version}</TableCell>
                            <TableCell className="break-all text-xs">
                              {getDependencyFileName(dep)}
                            </TableCell>
                            <TableCell className="break-all text-xs">{dep.target_dir}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
