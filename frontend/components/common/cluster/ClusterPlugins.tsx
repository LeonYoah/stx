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
 * 集群已安装插件列表组件：遵循「列表 + DetailHost 右侧常驻详情栏（#178 联动压缩 + 名称飞入 + J/K 切换）+
 * 多选浮动 BatchBar 批量卸载」设计范式；行尾仅保留轻操作，详情压平为单层 0.5px 细线画布。
 * Cluster installed plugins: list + persistent DetailHost rail (#178 linked motion, name fly-in, J/K nav) +
 * multi-select floating BatchBar for batch uninstall; flat single-layer 0.5px hairline detail pane.
 */

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {useRouter} from 'next/navigation';
import {isK8sInfra} from '@/lib/infra';
import {
  Eye,
  Layers3,
  MoreHorizontal,
  Package,
  Plus,
  Puzzle,
  RefreshCw,
  Trash2,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {
  DropdownMenu,
  type DropdownItem,
} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  BatchBar,
  ConfirmDialog,
  DetailHost,
  NeutralChip,
  SoftChip,
  TableCard,
  TruncatedText,
  useDetailNarrow,
  type DetailNav,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {toast} from '@/lib/toast';
import {cn} from '@/lib/utils';
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

function getPluginKey(plugin: InstalledPlugin): string {
  return `${plugin.id}:${plugin.plugin_name}:${plugin.version}`;
}

export function ClusterPlugins({clusterId}: ClusterPluginsProps) {
  const t = useTranslations();
  const router = useRouter();
  const narrow = useDetailNarrow();

  const [plugins, setPlugins] = useState<InstalledPlugin[]>([]);
  // K8s 集群插件遵循不可变镜像原则：只读展示，不提供安装/卸载写操作入口。
  // K8s cluster plugins follow the immutable-image principle: read-only view, no install/uninstall entry points.
  const [isK8sCluster, setIsK8sCluster] = useState(false);
  const [loading, setLoading] = useState(true);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [operating, setOperating] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'partial'>('all');

  // 右侧常驻详情栏状态与名称飞入起点 / Persistent detail rail state & fly-in origin
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [drawerPlugin, setDrawerPlugin] = useState<InstalledPlugin | null>(
    null,
  );
  const flyOriginRef = useRef<HTMLElement | null>(null);

  // 多选与批量卸载确认 / Multi-select & uninstall confirmation
  const [selectedNames, setSelectedNames] = useState<Set<string>>(new Set());
  const [pendingUninstallNames, setPendingUninstallNames] = useState<string[]>(
    [],
  );
  const reqSeqRef = useRef(0);

  const loadPlugins = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    try {
      const data = await services.plugin.listInstalledPlugins(clusterId);
      try {
        const info = await services.cluster.getCluster(clusterId);
        // 统一走 @/lib/infra 判定 K8s 集群形态（兼容历史字面值 kubernetes 与新契约值）。
        // Use shared @/lib/infra predicate for K8s detection (covers legacy kubernetes & contractual shapes).
        setIsK8sCluster(isK8sInfra(info.infra_type));
      } catch {
        setIsK8sCluster(false);
      }
      if (seq !== reqSeqRef.current) {
        return;
      }
      const list = data || [];
      setPlugins(list);
      // 移除已不存在的勾选项 / Prune stale selections
      const existingNames = new Set(
        list.filter((p) => !p.bundled).map((p) => p.plugin_name),
      );
      setSelectedNames((prev) => {
        const next = new Set<string>();
        prev.forEach((name) => {
          if (existingNames.has(name)) {
            next.add(name);
          }
        });
        return next;
      });
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

  const activeIndex = useMemo(
    () =>
      activeKey
        ? filteredPlugins.findIndex((p) => getPluginKey(p) === activeKey)
        : -1,
    [activeKey, filteredPlugins],
  );
  const activePlugin = activeIndex >= 0 ? filteredPlugins[activeIndex] : null;

  useEffect(() => {
    if (activePlugin) {
      setDrawerPlugin(activePlugin);
    }
  }, [activePlugin]);

  const openPluginDetail = useCallback(
    (plugin: InstalledPlugin | null, origin?: HTMLElement | null) => {
      flyOriginRef.current = origin ?? null;
      if (!plugin) {
        setActiveKey(null);
        return;
      }
      setDrawerPlugin(plugin);
      setActiveKey(getPluginKey(plugin));
    },
    [],
  );

  // J / K 键盘切换上一条/下一条插件详情 / J/K navigation for DetailHost
  const detailNav: DetailNav | undefined = useMemo(() => {
    if (activeIndex < 0 || filteredPlugins.length <= 1) {
      return undefined;
    }
    return {
      index: activeIndex,
      total: filteredPlugins.length,
      onPrev: () => {
        if (activeIndex > 0) {
          openPluginDetail(filteredPlugins[activeIndex - 1]);
        }
      },
      onNext: () => {
        if (activeIndex < filteredPlugins.length - 1) {
          openPluginDetail(filteredPlugins[activeIndex + 1]);
        }
      },
    };
  }, [activeIndex, filteredPlugins, openPluginDetail]);

  const selectablePlugins = useMemo(
    () => filteredPlugins.filter((p) => !p.bundled),
    [filteredPlugins],
  );
  const selectedInViewCount = useMemo(
    () =>
      selectablePlugins.filter((p) => selectedNames.has(p.plugin_name)).length,
    [selectablePlugins, selectedNames],
  );
  const headerCheckState: boolean | 'indeterminate' =
    selectablePlugins.length > 0 &&
    selectedInViewCount === selectablePlugins.length
      ? true
      : selectedInViewCount > 0
        ? 'indeterminate'
        : false;

  const toggleSelectAll = () => {
    setSelectedNames((prev) => {
      const next = new Set(prev);
      if (selectedInViewCount === selectablePlugins.length) {
        selectablePlugins.forEach((p) => next.delete(p.plugin_name));
      } else {
        selectablePlugins.forEach((p) => next.add(p.plugin_name));
      }
      return next;
    });
  };

  const toggleSelectOne = (pluginName: string) => {
    setSelectedNames((prev) => {
      const next = new Set(prev);
      if (next.has(pluginName)) {
        next.delete(pluginName);
      } else {
        next.add(pluginName);
      }
      return next;
    });
  };

  const hasActiveFilter = Boolean(searchKeyword.trim()) || statusFilter !== 'all';

  const handleConfirmUninstall = async () => {
    if (pendingUninstallNames.length === 0) {
      return;
    }
    const targets = [...pendingUninstallNames];
    setOperating(true);
    let succeeded = 0;
    let lastError = '';
    try {
      for (const name of targets) {
        try {
          await services.plugin.uninstallPlugin(clusterId, name);
          succeeded += 1;
        } catch (error) {
          lastError =
            error instanceof Error ? error.message : t('plugin.uninstallError');
        }
      }
      if (succeeded === targets.length) {
        toast.success(t('plugin.uninstallSuccess'));
      } else if (succeeded > 0) {
        toast.warning(`${succeeded}/${targets.length}: ${lastError}`);
      } else {
        toast.error(lastError || t('plugin.uninstallError'));
      }
      setSelectedNames((prev) => {
        const next = new Set(prev);
        targets.forEach((name) => next.delete(name));
        return next;
      });
      if (activePlugin && targets.includes(activePlugin.plugin_name)) {
        setActiveKey(null);
      }
      await loadPlugins();
    } finally {
      setOperating(false);
      setPendingUninstallNames([]);
    }
  };

  const goToPluginsPage = () => {
    router.push('/plugins');
  };

  const railOpen = activePlugin !== null;

  return (
    <div className='space-y-4'>
      {/* 顶部单行工具条 / Single-row toolbar */}
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
          {!isK8sCluster ? (
            <Button type='button' size='sm' onClick={goToPluginsPage}>
              <Plus className='size-3.5' aria-hidden />
              {t('plugin.addPlugin')}
            </Button>
          ) : null}
        </div>
      </div>

      {/* 列表 + DetailHost 右侧常驻详情栏（#178 联动压缩 + 名称飞入 + J/K 切换） */}
      {/* List + persistent DetailHost rail (#178 linked motion + name fly-in + J/K nav) */}
      <div className={layout.split}>
        <div
            className={layout.listCol}
            data-mcols={isK8sCluster ? 'cluster-plugins-k8s' : 'cluster-plugins'}
          >
          {isK8sCluster ? (
            <p className='mb-2 rounded-md border border-border/60 bg-muted/40 px-3 py-2 text-xs text-muted-foreground'>
              {t('cluster.k8s.immutablePluginHint')}
            </p>
          ) : null}
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
            <div className={layout.rowHead}>
              {!isK8sCluster ? (
                <div className='flex items-center'>
                  <Checkbox
                    checked={headerCheckState}
                    disabled={selectablePlugins.length === 0 || operating}
                    onCheckedChange={toggleSelectAll}
                    aria-label={t('config.filterAll')}
                  />
                </div>
              ) : null}
              <span>{t('plugin.name')}</span>
              <span>{t('plugin.category.label')}</span>
              <span>{t('plugin.version')}</span>
              <span>{t('plugin.installedAt')}</span>
              <span className='sr-only'>{t('plugin.actions')}</span>
            </div>

            <div className={cn(railOpen && layout.listDimmed)}>
              {filteredPlugins.map((plugin) => {
                const rowKey = getPluginKey(plugin);
                const isActive = activeKey === rowKey;
                const isSelected = selectedNames.has(plugin.plugin_name);
                const menuItems: DropdownItem[] = [
                  {
                    label: t('plugin.viewDetails'),
                    icon: <Eye className='size-3.5' aria-hidden />,
                    onSelect: () => openPluginDetail(plugin),
                  },
                  ...(!plugin.bundled && !isK8sCluster
                    ? [
                        {
                          label: t('plugin.uninstall'),
                          icon: <Trash2 className='size-3.5' aria-hidden />,
                          destructive: true,
                          disabled: operating,
                          onSelect: () =>
                            setPendingUninstallNames([plugin.plugin_name]),
                        },
                      ]
                    : []),
                ];

                return (
                  <div
                    key={rowKey}
                    role='row'
                    tabIndex={0}
                    data-testid={`cluster-plugin-row-${plugin.plugin_name}-${plugin.version}`}
                    className={cn(
                      layout.row,
                      'cursor-pointer',
                      isActive && layout.rowActive,
                    )}
                    onClick={(event) => {
                      const nameEl = event.currentTarget.querySelector<HTMLElement>(
                        '[data-detail-name]',
                      );
                      openPluginDetail(plugin, nameEl);
                    }}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        openPluginDetail(plugin);
                      }
                    }}
                  >
                    {/* 多选框 / Multi-select checkbox */}
                    {!isK8sCluster ? (
                      <div
                        className='flex items-center'
                        onClick={(event) => event.stopPropagation()}
                      >
                        <Checkbox
                          checked={isSelected}
                          disabled={Boolean(plugin.bundled) || operating}
                          onCheckedChange={() =>
                            toggleSelectOne(plugin.plugin_name)
                          }
                          aria-label={plugin.plugin_name}
                          title={
                            plugin.bundled ? t('plugin.bundled') : undefined
                          }
                        />
                      </div>
                    ) : null}

                    {/* 插件名与状态标签（同时保留 E2E view 锚点）/ Name & chips (preserves E2E view anchor) */}
                    <div className='flex min-w-0 items-center gap-1.5'>
                      <button
                        type='button'
                        data-detail-name
                        data-testid={`cluster-plugin-view-${plugin.plugin_name}-${plugin.version}`}
                        className='truncate text-left font-mono text-sm font-medium text-foreground hover:underline'
                        onClick={(event) => {
                          event.stopPropagation();
                          openPluginDetail(plugin, event.currentTarget);
                        }}
                      >
                        {plugin.plugin_name}
                      </button>
                      {plugin.bundled ? (
                        <NeutralChip
                          className='shrink-0'
                          data-testid={`cluster-plugin-bundled-${plugin.plugin_name}`}
                        >
                          {t('plugin.bundled')}
                        </NeutralChip>
                      ) : null}
                      {plugin.status === 'partial' ? (
                        <span
                          className='shrink-0'
                          title={`${t('plugin.partialInstallTip')} ${formatPluginNodeStatuses(plugin, t)}`}
                          data-testid={`cluster-plugin-partial-${plugin.plugin_name}`}
                        >
                          <SoftChip tone='rose'>
                            {t('plugin.partialInstalled')}
                          </SoftChip>
                        </span>
                      ) : null}
                    </div>

                    {/* 分类 / Category */}
                    <div className='min-w-0'>
                      {plugin.category ? (
                        <NeutralChip>
                          {t(`plugin.category.${plugin.category}`)}
                        </NeutralChip>
                      ) : (
                        <span className='text-xs text-muted-foreground'>—</span>
                      )}
                    </div>

                    {/* 版本 / Version */}
                    <span className='truncate font-mono text-xs tabular-nums text-foreground'>
                      {plugin.version || '—'}
                    </span>

                    {/* 安装时间 / Installed at */}
                    <span className='truncate font-mono text-xs tabular-nums text-muted-foreground'>
                      {formatDateTimeMinuteTz(plugin.installed_at)}
                    </span>

                    {/* 行尾轻操作 / Trailing ellipsis menu */}
                    <div
                      className='flex justify-end'
                      onClick={(event) => event.stopPropagation()}
                    >
                      <DropdownMenu
                        trigger={
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            iconOnly
                            aria-label={t('plugin.actions')}
                          >
                            <MoreHorizontal
                              className='size-4'
                              aria-hidden
                            />
                          </Button>
                        }
                        items={menuItems}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          </TableCard>

          {/* 悬浮多选批量条：位于列表卡内部底端，详情栏打开时也居中不遮挡 */}
          {/* Floating multi-select batch bar anchored inside listCol */}
          {selectedNames.size > 0 && !isK8sCluster ? (
            <div
              className={layout.batchBarWrap}
              data-testid='cluster-plugins-batch-bar'
            >
              <BatchBar
                selectedCount={selectedNames.size}
                busy={operating}
                onClear={() => setSelectedNames(new Set())}
              >
                <Button
                  type='button'
                  size='sm'
                  variant='danger'
                  disabled={operating}
                  onClick={() =>
                    setPendingUninstallNames(Array.from(selectedNames))
                  }
                >
                  <Trash2 className='size-3.5' aria-hidden />
                  {t('plugin.uninstall')}
                </Button>
              </BatchBar>
            </div>
          ) : null}
        </div>

        {/* 右侧常驻详情栏：单层扁平画布 + 0.5px 细线 / Persistent right detail rail */}
        <DetailHost
          open={railOpen}
          hasContent={drawerPlugin !== null}
          onOpenChange={(open) => {
            if (!open) {
              flyOriginRef.current = null;
              setActiveKey(null);
            }
          }}
          narrow={narrow}
          title={drawerPlugin?.plugin_name ?? ''}
          nav={detailNav}
          flyOrigin={flyOriginRef}
        >
          {drawerPlugin ? (
            <InstalledPluginRailDetail
              plugin={drawerPlugin}
              operating={operating}
              onUninstall={(name) => setPendingUninstallNames([name])}
            />
          ) : null}
        </DetailHost>
      </div>

      {/* 单项 / 批量卸载二次确认（具名动词）/ Single or batch uninstall confirmation */}
      <ConfirmDialog
        open={pendingUninstallNames.length > 0}
        onOpenChange={(open) => {
          if (!open && !operating) {
            setPendingUninstallNames([]);
          }
        }}
        title={t('plugin.uninstallPlugin')}
        description={t('plugin.uninstallConfirm', {
          name: pendingUninstallNames.join(', '),
        })}
        cancelLabel={t('common.cancel')}
        confirmLabel={t('plugin.uninstall')}
        confirmVariant='danger'
        loading={operating}
        onConfirm={handleConfirmUninstall}
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

/**
 * 右侧常驻插件详情内容：单层扁平画布 + 0.5px 细线分组，零嵌套卡片，兼容既有 E2E testid。
 * Persistent plugin rail detail: flat single-layer surface with 0.5px hairlines and preserved E2E testid.
 */
function InstalledPluginRailDetail({
  plugin,
  operating,
  onUninstall,
}: {
  plugin: InstalledPlugin;
  operating: boolean;
  onUninstall: (pluginName: string) => void;
}) {
  const t = useTranslations();

  const selectedProfiles = plugin.selected_profile_keys || [];
  const rawDependencies = plugin.dependencies || [];
  const metadataAttachedConnectors = plugin.attached_connectors || [];
  const {attachedConnectors: attachedConnectorsFromDeps, extraDependencies} =
    splitDependencies(rawDependencies);
  const attachedConnectors = dedupeStrings([
    ...metadataAttachedConnectors,
    ...attachedConnectorsFromDeps,
  ]);

  return (
    <div
      data-testid={`cluster-plugin-detail-dialog-${plugin.plugin_name}`}
      className='space-y-0 px-5 pb-6'
    >
      {/* 顶部状态标签与操作 / Top status chips & uninstall action */}
      <div className='flex flex-wrap items-center justify-between gap-2 pb-4'>
        <div className='flex flex-wrap items-center gap-1.5'>
          <NeutralChip className='font-mono tabular-nums'>
            v{plugin.version}
          </NeutralChip>
          {plugin.category ? (
            <NeutralChip>{t(`plugin.category.${plugin.category}`)}</NeutralChip>
          ) : null}
          {plugin.bundled ? (
            <NeutralChip>{t('plugin.bundled')}</NeutralChip>
          ) : null}
          {plugin.status === 'partial' ? (
            <SoftChip tone='rose'>{t('plugin.partialInstalled')}</SoftChip>
          ) : null}
        </div>
        {!plugin.bundled ? (
          <Button
            type='button'
            variant='secondary'
            size='sm'
            disabled={operating}
            onClick={() => onUninstall(plugin.plugin_name)}
          >
            <Trash2 className='size-3.5 text-destructive' aria-hidden />
            {t('plugin.uninstall')}
          </Button>
        ) : null}
      </div>

      {/* 基础元信息 / Metadata */}
      <dl className='grid grid-cols-1 gap-y-3 border-t-[0.5px] border-border/80 py-4 text-xs'>
        <div className='min-w-0 space-y-0.5'>
          <dt className='text-muted-foreground'>{t('plugin.installedAt')}</dt>
          <dd className='font-mono text-foreground tabular-nums'>
            {formatDateTimeTz(plugin.installed_at)}
          </dd>
        </div>
        <div className='min-w-0 space-y-0.5'>
          <dt className='text-muted-foreground'>{t('plugin.installPath')}</dt>
          <dd className='text-foreground'>
            <TruncatedText
              text={plugin.install_path}
              fallback='—'
              mode='path'
              mono
              copyable
            />
          </dd>
        </div>
      </dl>

      {/* 已选场景 / Selected profiles */}
      <section className='space-y-2 border-t-[0.5px] border-border/80 py-4'>
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

      {/* 自动附带连接器 / Attached connectors */}
      <section className='space-y-2 border-t-[0.5px] border-border/80 py-4'>
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

      {/* 额外依赖清单（0.5px 细线行，适配侧栏宽度无横向挤压）/ Extra dependencies */}
      <section className='space-y-2 border-t-[0.5px] border-border/80 pt-4'>
        <h4 className='text-xs font-semibold text-foreground'>
          {t('plugin.extraDependencies')}
        </h4>
        {extraDependencies.length === 0 ? (
          <p className='text-xs text-muted-foreground'>
            {t('plugin.noExtraDependencies')}
          </p>
        ) : (
          <ul className='divide-y-[0.5px] divide-border/70 text-xs'>
            {extraDependencies.map((dep, idx) => {
              const fileName = getDependencyFileName(dep);
              return (
                <li
                  key={`${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}:${idx}`}
                  className='space-y-1 py-2.5 first:pt-0 last:pb-0'
                >
                  <div className='flex items-baseline justify-between gap-2'>
                    <span className='truncate font-mono font-medium text-foreground'>
                      {dep.artifact_id}
                    </span>
                    <span className='shrink-0 font-mono text-[11px] text-muted-foreground tabular-nums'>
                      {dep.version}
                    </span>
                  </div>
                  <TruncatedText
                    text={`${dep.group_id} · ${fileName}`}
                    mono
                    textClassName='text-[11px] text-muted-foreground'
                  />
                  <TruncatedText
                    text={dep.target_dir}
                    mode='path'
                    mono
                    textClassName='text-[11px] text-muted-foreground'
                  />
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
