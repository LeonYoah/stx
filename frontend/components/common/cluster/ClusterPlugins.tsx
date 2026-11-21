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
 * 集群已安装插件列表组件：采用 TableCard 零塌陷骨架加载 + SearchField 搜索 + 单层扁平详情弹窗。
 * Installed cluster plugins panel: TableCard zero-collapse loading, SearchField filter, and flat detail dialog.
 */

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {useRouter} from 'next/navigation';
import {Eye, Layers3, Package, Plus, Puzzle, RefreshCw, Trash2} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  NeutralChip,
  SoftChip,
  TableCard,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
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
import services from '@/lib/services';
import {
  InstalledPlugin,
  PluginDependency,
  formatPluginNodeStatuses,
} from '@/lib/services/plugin/types';
import {formatDateTimeMinuteTz, formatDateTimeTz} from '@/lib/timezone';

interface ClusterPluginsProps {
  clusterId: number;
}

export function ClusterPlugins({clusterId}: ClusterPluginsProps) {
  const t = useTranslations();
  const router = useRouter();

  const [plugins, setPlugins] = useState<InstalledPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [operating, setOperating] = useState<string | null>(null);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'partial'>('all');
  const [pluginToUninstall, setPluginToUninstall] =
    useState<InstalledPlugin | null>(null);
  const [detailPlugin, setDetailPlugin] = useState<InstalledPlugin | null>(null);
  const reqSeqRef = useRef(0);

  const loadPlugins = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    try {
      const data = await services.plugin.listInstalledPlugins(clusterId);
      if (seq !== reqSeqRef.current) {
        return;
      }
      setPlugins(data || []);
    } catch (error) {
      if (seq === reqSeqRef.current) {
        toast.error(
          error instanceof Error ? error.message : t('plugin.loadError'),
        );
      }
    } finally {
      if (seq === reqSeqRef.current) {
        setHasLoaded(true);
        setLoading(false);
      }
    }
  }, [clusterId, t]);

  useEffect(() => {
    void loadPlugins();
  }, [loadPlugins]);

  const partialCount = useMemo(
    () => plugins.filter((item) => item.status === 'partial').length,
    [plugins],
  );

  const filteredPlugins = useMemo(() => {
    const kw = searchKeyword.trim().toLowerCase();
    return plugins.filter((item) => {
      if (statusFilter === 'partial' && item.status !== 'partial') {
        return false;
      }
      if (!kw) {
        return true;
      }
      return (
        item.plugin_name.toLowerCase().includes(kw) ||
        (item.category || '').toLowerCase().includes(kw) ||
        (item.version || '').toLowerCase().includes(kw)
      );
    });
  }, [plugins, searchKeyword, statusFilter]);

  const hasActiveFilter = Boolean(searchKeyword.trim()) || statusFilter !== 'all';

  const handleUninstall = async () => {
    if (!pluginToUninstall) {
      return;
    }
    setOperating(pluginToUninstall.plugin_name);
    try {
      await services.plugin.uninstallPlugin(
        clusterId,
        pluginToUninstall.plugin_name,
      );
      toast.success(t('plugin.uninstallSuccess'));
      void loadPlugins();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('plugin.uninstallError'),
      );
    } finally {
      setOperating(null);
      setPluginToUninstall(null);
    }
  };

  const goToPluginsPage = () => {
    router.push('/plugins');
  };

  return (
    <div className='space-y-4'>
      {/* 顶部单行工具条：搜索、局部异常筛选（条件出现）、刷新与安装入口 */}
      {/* Single-row toolbar: search, conditional partial-install filter, refresh, and add CTA */}
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <div className={layout.toolbarSearch}>
            <SearchField
              label={t('plugin.searchPlaceholder')}
              value={searchKeyword}
              onValueChange={setSearchKeyword}
              placeholder={t('plugin.searchPlaceholder')}
              clearLabel={t('common.clearFilters')}
            />
          </div>
          {partialCount > 0 ? (
            <SegmentedControl
              label={t('plugin.installedPlugins')}
              value={statusFilter}
              onValueChange={(v) => setStatusFilter(v as 'all' | 'partial')}
              options={[
                {value: 'all', label: t('config.filterAll')},
                {value: 'partial', label: t('plugin.partialInstalled')},
              ]}
            />
          ) : null}
          {hasLoaded ? (
            <span className='text-xs text-muted-foreground tabular-nums'>
              {filteredPlugins.length} / {plugins.length}
            </span>
          ) : null}
        </div>

        <div className='flex items-center gap-2'>
          <Button
            type='button'
            variant='secondary'
            size='sm'
            iconOnly
            onClick={() => void loadPlugins()}
            disabled={loading}
            aria-label={t('common.refresh')}
            title={t('common.refresh')}
          >
            <RefreshCw
              className={`size-3.5 ${loading ? 'animate-spin' : ''}`}
              aria-hidden
            />
          </Button>
          <Button type='button' size='sm' onClick={goToPluginsPage}>
            <Plus className='size-3.5' aria-hidden />
            {t('plugin.addPlugin')}
          </Button>
        </div>
      </div>

      {/* 统一 TableCard：零塌陷骨架、顶部细进度条、0.5px 行细线与双情景空态 */}
      {/* Unified TableCard with zero-collapse skeleton, hairline rows, and dual empty states */}
      <TableCard
        loading={loading}
        hasData={filteredPlugins.length > 0}
        skeletonColumns={5}
        skeletonRows={5}
        empty={
          hasActiveFilter ? (
            <EmptyState
              icon={<Puzzle />}
              title={t('plugin.noMatchingPlugins')}
              description={t('plugin.noMatchingPluginsDesc')}
              action={
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={() => {
                    setSearchKeyword('');
                    setStatusFilter('all');
                  }}
                >
                  {t('common.clearFilters')}
                </Button>
              }
            />
          ) : (
            <EmptyState
              icon={<Puzzle />}
              title={t('plugin.noInstalledPlugins')}
              description={t('plugin.browsePlugins')}
              action={
                <Button type='button' size='sm' onClick={goToPluginsPage}>
                  <Plus className='size-3.5' aria-hidden />
                  {t('plugin.browsePlugins')}
                </Button>
              }
            />
          )
        }
      >
        <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
          <TableHeader>
            <TableRow className='hover:bg-transparent'>
              <TableHead>{t('plugin.name')}</TableHead>
              <TableHead>{t('plugin.category.label')}</TableHead>
              <TableHead>{t('plugin.version')}</TableHead>
              <TableHead>{t('plugin.installedAt')}</TableHead>
              <TableHead className='w-[180px] text-right'>
                {t('plugin.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredPlugins.map((plugin) => {
              const isOperating = operating === plugin.plugin_name;
              return (
                <TableRow
                  key={`${plugin.id}-${plugin.plugin_name}-${plugin.version}`}
                  data-testid={`cluster-plugin-row-${plugin.plugin_name}-${plugin.version}`}
                >
                  <TableCell className='font-medium'>
                    <div className='flex flex-wrap items-center gap-1.5'>
                      <span className='font-mono text-sm'>
                        {plugin.plugin_name}
                      </span>
                      {plugin.bundled ? (
                        <NeutralChip
                          data-testid={`cluster-plugin-bundled-${plugin.plugin_name}`}
                        >
                          {t('plugin.bundled')}
                        </NeutralChip>
                      ) : null}
                      {plugin.status === 'partial' ? (
                        <span
                          title={`${t('plugin.partialInstallTip')} ${formatPluginNodeStatuses(plugin, t)}`}
                          data-testid={`cluster-plugin-partial-${plugin.plugin_name}`}
                        >
                          <SoftChip tone='rose'>
                            {t('plugin.partialInstalled')}
                          </SoftChip>
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    {plugin.category ? (
                      <NeutralChip>
                        {t(`plugin.category.${plugin.category}`)}
                      </NeutralChip>
                    ) : (
                      <span className='text-muted-foreground'>—</span>
                    )}
                  </TableCell>
                  <TableCell className='font-mono text-xs tabular-nums whitespace-nowrap'>
                    {plugin.version}
                  </TableCell>
                  <TableCell className='font-mono text-xs text-muted-foreground tabular-nums whitespace-nowrap'>
                    {formatDateTimeMinuteTz(plugin.installed_at)}
                  </TableCell>
                  <TableCell className='text-right'>
                    <div className='flex items-center justify-end gap-1.5'>
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        data-testid={`cluster-plugin-view-${plugin.plugin_name}-${plugin.version}`}
                        onClick={() => setDetailPlugin(plugin)}
                      >
                        <Eye className='size-3.5' aria-hidden />
                        {t('plugin.viewDetails')}
                      </Button>
                      {!plugin.bundled ? (
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          onClick={() => setPluginToUninstall(plugin)}
                          disabled={isOperating}
                          aria-label={t('plugin.uninstall')}
                          title={t('plugin.uninstall')}
                        >
                          <Trash2
                            className='size-3.5 text-destructive'
                            aria-hidden
                          />
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableCard>

      {/* 卸载二次确认：具名动词按钮 / Uninstall confirmation with named action verb */}
      <AlertDialog
        open={Boolean(pluginToUninstall)}
        onOpenChange={(open) => {
          if (!open) {
            setPluginToUninstall(null);
          }
        }}
      >
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
            <AlertDialogAction onClick={() => void handleUninstall()}>
              {t('plugin.uninstall')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <InstalledPluginDetailDialog
        open={Boolean(detailPlugin)}
        onOpenChange={(open) => {
          if (!open) {
            setDetailPlugin(null);
          }
        }}
        plugin={detailPlugin}
      />
    </div>
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
  const attachedConnectors = dedupeStrings(
    items
      .filter((item) => item.target_dir === 'connectors')
      .map((item) => item.artifact_id),
  );
  const extraDependencies = items.filter(
    (item) => item.target_dir !== 'connectors',
  );
  return {attachedConnectors, extraDependencies};
}

interface InstalledPluginDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plugin: InstalledPlugin | null;
}

/**
 * 已安装插件详情弹窗：压平为单层画布，以留白与 0.5px 细线分隔各组信息，去掉嵌套卡片与 ScrollArea。
 * Installed plugin detail dialog flattened into a single-layer surface separated by 0.5px hairlines.
 */
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
        className='arc-compact w-[min(94vw,960px)]! sm:max-w-none! max-h-[86vh]'
      >
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2 font-mono'>
            <Puzzle className='size-4 text-primary' aria-hidden />
            {plugin?.plugin_name || t('plugin.viewDetails')}
          </DialogTitle>
          <DialogDescription className='tabular-nums'>
            {plugin
              ? t('plugin.clusterPluginDetailDesc', {
                  version: plugin.version,
                  installedAt: formatDateTimeTz(plugin.installed_at),
                })
              : ''}
          </DialogDescription>
        </DialogHeader>

        {plugin ? (
          <div className='max-h-[68vh] overflow-y-auto pr-1'>
            {/* 基础元信息 / Metadata */}
            <dl className='grid grid-cols-1 gap-x-6 gap-y-3 border-t-[0.5px] border-border/80 py-4 text-xs sm:grid-cols-3'>
              <div className='min-w-0 space-y-1'>
                <dt className='text-muted-foreground'>{t('plugin.version')}</dt>
                <dd className='font-mono text-sm font-medium tabular-nums'>
                  {plugin.version}
                </dd>
              </div>
              <div className='min-w-0 space-y-1'>
                <dt className='text-muted-foreground'>
                  {t('plugin.category.label')}
                </dt>
                <dd>
                  {plugin.category ? (
                    <NeutralChip>
                      {t(`plugin.category.${plugin.category}`)}
                    </NeutralChip>
                  ) : (
                    '—'
                  )}
                </dd>
              </div>
              <div className='min-w-0 space-y-1'>
                <dt className='text-muted-foreground'>
                  {t('plugin.installPath')}
                </dt>
                <dd
                  className='truncate font-mono text-xs text-foreground'
                  title={plugin.install_path || '—'}
                >
                  {plugin.install_path || '—'}
                </dd>
              </div>
            </dl>

            {/* 已选 Profile / Selected profiles */}
            <section className='space-y-2.5 border-t-[0.5px] border-border/80 py-4'>
              <h4 className='flex items-center gap-1.5 text-xs font-semibold text-foreground'>
                <Layers3 className='size-3.5 text-muted-foreground' aria-hidden />
                {t('plugin.selectedProfiles')}
              </h4>
              {selectedProfiles.length === 0 ? (
                <p className='text-xs text-muted-foreground'>
                  {t('plugin.noSelectedProfiles')}
                </p>
              ) : (
                <div className='flex flex-wrap gap-1.5'>
                  {selectedProfiles.map((profileKey) => (
                    <NeutralChip key={profileKey} className='font-mono'>
                      {profileKey}
                    </NeutralChip>
                  ))}
                </div>
              )}
            </section>

            {/* 附带连接器 / Attached connectors */}
            <section className='space-y-2.5 border-t-[0.5px] border-border/80 py-4'>
              <h4 className='flex items-center gap-1.5 text-xs font-semibold text-foreground'>
                <Package className='size-3.5 text-muted-foreground' aria-hidden />
                {t('plugin.attachedConnectors')}
              </h4>
              {attachedConnectors.length === 0 ? (
                <p className='text-xs text-muted-foreground'>
                  {t('plugin.noAttachedConnectors')}
                </p>
              ) : (
                <div className='flex flex-wrap gap-1.5'>
                  {attachedConnectors.map((artifactId) => (
                    <NeutralChip key={artifactId} className='font-mono'>
                      {artifactId}
                    </NeutralChip>
                  ))}
                </div>
              )}
            </section>

            {/* 额外依赖表 / Extra dependencies */}
            <section className='space-y-2.5 border-t-[0.5px] border-border/80 pt-4'>
              <h4 className='text-xs font-semibold text-foreground'>
                {t('plugin.extraDependencies')}
              </h4>
              {extraDependencies.length === 0 ? (
                <p className='text-xs text-muted-foreground'>
                  {t('plugin.noExtraDependencies')}
                </p>
              ) : (
                <div className='overflow-x-auto'>
                  <Table className='min-w-[700px] [&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                    <TableHeader>
                      <TableRow className='hover:bg-transparent'>
                        <TableHead>{t('plugin.groupId')}</TableHead>
                        <TableHead>{t('plugin.artifactId')}</TableHead>
                        <TableHead>{t('plugin.version')}</TableHead>
                        <TableHead>{t('plugin.fileName')}</TableHead>
                        <TableHead>{t('plugin.targetDir')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {extraDependencies.map((dep, idx) => {
                        const fileName = getDependencyFileName(dep);
                        return (
                          <TableRow
                            key={`${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}:${idx}`}
                          >
                            <TableCell className='font-mono text-xs text-muted-foreground'>
                              {dep.group_id}
                            </TableCell>
                            <TableCell className='font-mono text-xs font-medium'>
                              {dep.artifact_id}
                            </TableCell>
                            <TableCell className='font-mono text-xs tabular-nums whitespace-nowrap'>
                              {dep.version}
                            </TableCell>
                            <TableCell
                              className='max-w-[220px] truncate font-mono text-xs'
                              title={fileName}
                            >
                              {fileName}
                            </TableCell>
                            <TableCell
                              className='max-w-[160px] truncate font-mono text-xs text-muted-foreground'
                              title={dep.target_dir}
                            >
                              {dep.target_dir}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </section>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
