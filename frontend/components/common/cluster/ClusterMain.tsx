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
 * 集群管理列表主组件：对齐告警中心、诊断中心与主机管理的新一代控制台范式。
 * .arc-compact + 单行工具栏（状态/模式 SegmentedControl、SearchField、ViewToggle）+
 * TableCard（默认表格、卡片双视图与 useViewMorph 共享形变）+ DetailHost 常驻右侧速览栏
 * （#178 联动压暗、凹圆角、名称飞入、J/K 导航）+ URL 同步与请求序号竞态保护。
 * Cluster management main component aligned with monitoring/diagnostics/hosts:
 * .arc-compact + single-row toolbar + TableCard (list default / grid with useViewMorph) +
 * DetailHost persistent quick-detail rail (#178 linked motion) + URL sync & race guard.
 */
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {LayoutGroup, motion} from 'motion/react';
import {
  Activity,
  ArrowUpRight,
  Layers,
  LayoutGrid,
  List,
  MoreHorizontal,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Server,
  Square,
  Trash2,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {
  DropdownMenu,
  type DropdownItem,
} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  DetailHost,
  ModuleNavTabs,
  NeutralChip,
  SoftChip,
  TableCard,
  WorkspaceHeader,
  useDetailNarrow,
  type DetailNav,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {ViewToggle} from '@/components/common/ViewToggle';
import {useViewMorph} from '@/lib/animations/view-morph';
import services from '@/lib/services';
import {
  ClusterStatus,
  DeploymentMode,
  OperationType,
  type ClusterInfo,
  type ListClustersRequest,
} from '@/lib/services/cluster/types';
import {toast} from '@/lib/toast';
import {cn} from '@/lib/utils';
import {ClusterCard} from './ClusterCard';
import {ClusterDeployWizard} from './ClusterDeployWizard';
import {ClusterQuickDetail} from './ClusterQuickDetail';
import {ClusterStatusChip} from './ClusterStatusChip';
import {CreateClusterDialog} from './CreateClusterDialog';
import {K8sClusterCreateDialog} from './K8sClusterCreateDialog';
import {EditClusterDialog} from './EditClusterDialog';
import {
  getClusterActionFlags,
  useClusterLifecycle,
} from './useClusterLifecycle';

const PAGE_SIZE = 12;

type StatusFilter = 'all' | ClusterStatus;
type ModeFilter = 'all' | DeploymentMode;
type ViewMode = 'list' | 'grid';

function parseStatusFilter(raw: string | null): StatusFilter {
  if (
    raw === ClusterStatus.RUNNING ||
    raw === ClusterStatus.DEPLOYING ||
    raw === ClusterStatus.STOPPED ||
    raw === ClusterStatus.ERROR
  ) {
    return raw;
  }
  return 'all';
}

function parseModeFilter(raw: string | null): ModeFilter {
  if (raw === DeploymentMode.HYBRID || raw === DeploymentMode.SEPARATED) {
    return raw;
  }
  return 'all';
}

function parseViewMode(raw: string | null): ViewMode {
  return raw === 'grid' ? 'grid' : 'list';
}

/**
 * 首次空态微缩集群拓扑示意 SVG（遵循 Empty States Note：随主题变量自适应，无外部资源）。
 * Miniature cluster topology SVG for first-run empty state, theme-adaptive and self-contained.
 */
function ClusterEmptyIllustration() {
  return (
    <svg
      width='40'
      height='32'
      viewBox='0 0 40 32'
      fill='none'
      aria-hidden='true'
    >
      <rect
        x='3'
        y='4'
        width='15'
        height='10'
        rx='3'
        stroke='currentColor'
        strokeWidth='1.4'
      />
      <rect
        x='22'
        y='4'
        width='15'
        height='10'
        rx='3'
        stroke='currentColor'
        strokeWidth='1.4'
        strokeOpacity='0.55'
      />
      <rect
        x='12'
        y='19'
        width='16'
        height='9'
        rx='3'
        stroke='currentColor'
        strokeWidth='1.4'
        strokeDasharray='2.5 2'
      />
      <path
        d='M10.5 14V16.5H29.5V14M20 16.5V19'
        stroke='currentColor'
        strokeWidth='1.3'
        strokeLinecap='round'
        strokeOpacity='0.6'
      />
    </svg>
  );
}

export function ClusterMain() {
  const t = useTranslations();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const narrow = useDetailNarrow();

  // URL 驱动的筛选、视图与选中集群 ID / URL-driven filters, view mode, and selected cluster ID
  const filterStatus = parseStatusFilter(searchParams.get('status'));
  const filterMode = parseModeFilter(searchParams.get('mode'));
  const urlQuery = searchParams.get('q') ?? '';
  const currentPage = Math.max(1, Number(searchParams.get('page')) || 1);
  const viewMode = parseViewMode(searchParams.get('view'));
  const activeClusterIdParam = Number(searchParams.get('cluster')) || null;

  const [searchInput, setSearchInput] = useState(urlQuery);
  useEffect(() => {
    setSearchInput(urlQuery);
  }, [urlQuery]);

  // 列表数据与请求状态（刷新失败保留旧数据供卡片内重试）/ List data & status (retain stale rows on error)
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [refreshTick, setRefreshTick] = useState(0);
  const reqSeqRef = useRef(0);
  const searchRef = useRef<HTMLInputElement>(null);
  // 名称飞入来源行：仅点击/回车打开时写入，J/K、头部 ↑↓ 与深链不写。
  // Fly-in origin row: set only on click/Enter open — never on J/K, arrows, or deep links.
  const flyOriginRef = useRef<HTMLElement | null>(null);

  // 弹窗状态 / Dialog states
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [isDeployWizardOpen, setIsDeployWizardOpen] = useState(false);
  const [isK8sDialogOpen, setIsK8sDialogOpen] = useState(false);
  const [editingCluster, setEditingCluster] = useState<ClusterInfo | null>(
    null,
  );
  // 关闭详情栏时保留最后一份集群对象，直到退场动画播完卸载 / Keep last cluster for exit animation
  const [lastDrawerCluster, setLastDrawerCluster] =
    useState<ClusterInfo | null>(null);

  const updateQuery = useCallback(
    (updates: Record<string, string | null>, resetPage = false) => {
      const next = new URLSearchParams(searchParams.toString());
      Object.entries(updates).forEach(([key, value]) => {
        if (!value || value === 'all') {
          next.delete(key);
        } else {
          next.set(key, value);
        }
      });
      if (resetPage) {
        next.delete('page');
      }
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  // 「/」快捷键聚焦搜索框 / '/' shortcut focuses search field
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        e.key === '/' &&
        !el?.closest('input, textarea, select, [contenteditable="true"]')
      ) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  // 搜索防抖同步到 URL（300ms），避免每击键发一次请求 / Debounce search input to URL
  useEffect(() => {
    if (searchInput === urlQuery) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      updateQuery({q: searchInput.trim() || null}, true);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput, updateQuery, urlQuery]);

  const loadClusters = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    // 成功前保留旧 error，避免无数据重试窗口落到空态「还没有纳管集群」。
    // Keep prior error until success so a no-data retry cannot fall through to the empty state.
    try {
      const params: ListClustersRequest = {
        current: currentPage,
        size: PAGE_SIZE,
        name: urlQuery || undefined,
        status: filterStatus !== 'all' ? filterStatus : undefined,
        deployment_mode: filterMode !== 'all' ? filterMode : undefined,
      };
      const result = await services.cluster.getClustersSafe(params);
      if (seq !== reqSeqRef.current) {
        return;
      }
      if (result.success && result.data) {
        setClusters(result.data.clusters ?? []);
        setTotal(result.data.total ?? 0);
        setWarnings(result.data.warnings ?? []);
        setError(null);
      } else {
        setError(result.error || t('cluster.loadError'));
      }
    } catch (err) {
      if (seq !== reqSeqRef.current) {
        return;
      }
      setError(err instanceof Error ? err.message : t('cluster.loadError'));
    } finally {
      if (seq === reqSeqRef.current) {
        setLoading(false);
      }
    }
  }, [currentPage, filterMode, filterStatus, t, urlQuery]);

  useEffect(() => {
    void loadClusters();
  }, [loadClusters]);

  const activeCluster = useMemo(
    () =>
      activeClusterIdParam
        ? (clusters.find((c) => c.id === activeClusterIdParam) ?? null)
        : null,
    [activeClusterIdParam, clusters],
  );

  useEffect(() => {
    if (activeCluster) {
      setLastDrawerCluster(activeCluster);
    }
  }, [activeCluster]);

  const drawerCluster = activeCluster ?? lastDrawerCluster;

  const {
    operatingClusterId,
    operatingAction,
    requestLifecycle,
    requestDelete,
    dialogs: lifecycleDialogs,
  } = useClusterLifecycle({
    onComplete: (cluster, action) => {
      if (action === 'delete' && activeClusterIdParam === cluster.id) {
        updateQuery({cluster: null});
      }
      void loadClusters();
    },
  });

  const handleOpenCluster = useCallback(
    (cluster: ClusterInfo | null) => {
      updateQuery({cluster: cluster ? String(cluster.id) : null});
    },
    [updateQuery],
  );

  const activeIndex = activeCluster
    ? clusters.findIndex((c) => c.id === activeCluster.id)
    : -1;

  const detailNav: DetailNav | undefined =
    activeIndex >= 0
      ? {
          index: activeIndex,
          total: clusters.length,
          onPrev: () => {
            if (activeIndex > 0) {
              // J/K 与头部 ↑↓ 切换不触发名称飞入 / Never fly on J/K or header arrow navigation
              flyOriginRef.current = null;
              handleOpenCluster(clusters[activeIndex - 1]);
            }
          },
          onNext: () => {
            if (activeIndex < clusters.length - 1) {
              flyOriginRef.current = null;
              handleOpenCluster(clusters[activeIndex + 1]);
            }
          },
        }
      : undefined;

  const hasActiveFilters =
    Boolean(urlQuery) || filterStatus !== 'all' || filterMode !== 'all';

  const handleClearFilters = useCallback(() => {
    setSearchInput('');
    updateQuery({q: null, status: null, mode: null}, true);
  }, [updateQuery]);

  const handleRefresh = () => {
    setRefreshTick((n) => n + 1);
    void loadClusters();
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const isSoleDefaultCluster = total === 1 && !hasActiveFilters;

  return (
    <div
      className={cn('arc-compact', layout.page, layout.pageViewport)}
      data-testid='cluster-main-page'
    >
      <WorkspaceHeader
        icon={<Layers />}
        title={t('cluster.title')}
        subtitle={t('cluster.description')}
        tabs={
          <ModuleNavTabs
            reorderGroupId='clusters-hosts'
            items={[
              {
                key: 'clusters',
                label: t('cluster.title'),
                href: '/clusters',
                icon: <Layers className='size-3.5' />,
              },
              {
                key: 'hosts',
                label: t('host.title'),
                href: '/hosts',
                icon: <Server className='size-3.5' />,
              },
            ]}
            activeKey='clusters'
          />
        }
        actions={
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => setIsCreateDialogOpen(true)}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('cluster.registerCluster')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={() => setIsDeployWizardOpen(true)}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('cluster.createCluster')}
            </Button>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => setIsK8sDialogOpen(true)}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('cluster.k8s.createK8s')}
            </Button>
          </div>
        }
      />

      {/* 单行工具栏：状态分段 + 搜索框 + 部署模式分段 + 视图切换与刷新 */}
      {/* Single-row toolbar: status segments + search + mode segments + view toggle & refresh */}
      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('cluster.list.statusFilterLabel')}
          value={filterStatus}
          onValueChange={(v) => updateQuery({status: v}, true)}
          options={[
            {value: 'all', label: t('cluster.allStatuses')},
            {value: ClusterStatus.RUNNING, label: t('cluster.statuses.running')},
            {
              value: ClusterStatus.DEPLOYING,
              label: t('cluster.statuses.deploying'),
            },
            {value: ClusterStatus.STOPPED, label: t('cluster.statuses.stopped')},
            {value: ClusterStatus.ERROR, label: t('cluster.statuses.error')},
          ]}
        />

        <div className={layout.toolbarSearch}>
          <SearchField
            ref={searchRef}
            label={t('cluster.list.searchLabel')}
            value={searchInput}
            onValueChange={setSearchInput}
            placeholder={t('cluster.searchPlaceholder')}
            clearLabel={t('cluster.list.searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                updateQuery({q: searchInput.trim() || null}, true);
              } else if (e.key === 'Escape' && searchInput) {
                setSearchInput('');
                updateQuery({q: null}, true);
              }
            }}
          />
        </div>

        <SegmentedControl
          label={t('cluster.list.modeFilterLabel')}
          value={filterMode}
          onValueChange={(v) => updateQuery({mode: v}, true)}
          options={[
            {value: 'all', label: t('cluster.allModes')},
            {value: DeploymentMode.HYBRID, label: t('cluster.modes.hybrid')},
            {
              value: DeploymentMode.SEPARATED,
              label: t('cluster.modes.separated'),
            },
          ]}
        />

        <div className={layout.toolbarRight}>
          {hasActiveFilters ? (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={handleClearFilters}
            >
              {t('common.clearFilters')}
            </Button>
          ) : null}
          <ViewToggle
            label={t('cluster.list.viewModeLabel')}
            value={viewMode}
            onValueChange={(v) =>
              updateQuery({view: v === 'list' ? null : v})
            }
            options={[
              {
                value: 'list',
                icon: List,
                ariaLabel: t('cluster.list.viewList'),
              },
              {
                value: 'grid',
                icon: LayoutGrid,
                ariaLabel: t('cluster.list.viewGrid'),
              },
            ]}
          />
          <Button
            type='button'
            variant='secondary'
            size='sm'
            iconOnly
            aria-label={t('common.refresh')}
            title={t('common.refresh')}
            onClick={handleRefresh}
          >
            <motion.span
              key={refreshTick}
              initial={refreshTick > 0 ? {rotate: 0} : false}
              animate={{rotate: refreshTick > 0 ? 360 : 0}}
              transition={{duration: 0.6, ease: [0.22, 1, 0.36, 1]}}
              className='inline-flex'
            >
              <RefreshCw
                className={cn('size-3.5', loading && 'animate-spin')}
                aria-hidden
              />
            </motion.span>
          </Button>
        </div>
      </div>

      {/* 主内容分栏：唯一主卡片 TableCard + DetailHost 右侧常驻速览栏 */}
      {/* Main split: single TableCard + persistent DetailHost rail */}
      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='clusters'>
          <TableCard
            loading={loading}
            hasData={clusters.length > 0}
            skeletonColumns={6}
            skeletonRows={6}
            error={error}
            onRetry={() => void loadClusters()}
            pagination={
              total > 0
                ? {
                    currentPage,
                    totalPages,
                    pageSize: PAGE_SIZE,
                    totalItems: total,
                    onPageChange: (page) =>
                      updateQuery({page: page > 1 ? String(page) : null}),
                  }
                : undefined
            }
            empty={
              hasActiveFilters ? (
                <EmptyState
                  title={t('cluster.list.emptyFilteredTitle')}
                  description={t('cluster.list.emptyFilteredDesc')}
                  action={
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={handleClearFilters}
                    >
                      {t('common.clearFilters')}
                    </Button>
                  }
                />
              ) : (
                <EmptyState
                  icon={<ClusterEmptyIllustration />}
                  title={t('cluster.list.emptyFirstTitle')}
                  description={t('cluster.list.emptyFirstDesc')}
                  action={
                    <div className='flex flex-wrap items-center justify-center gap-2'>
                      <Button
                        type='button'
                        size='sm'
                        onClick={() => setIsDeployWizardOpen(true)}
                      >
                        <Plus className='size-3.5' aria-hidden />
                        {t('cluster.createCluster')}
                      </Button>
                      <Button
                        type='button'
                        size='sm'
                        variant='secondary'
                        onClick={() => setIsCreateDialogOpen(true)}
                      >
                        <Plus className='size-3.5' aria-hidden />
                        {t('cluster.registerCluster')}
                      </Button>
                    </div>
                  }
                />
              )
            }
          >
            {!error && warnings.length > 0 ? (
              <div
                className='border-b border-(--border) bg-amber-500/5 px-4 py-2 text-xs text-amber-800 dark:text-amber-200'
                role='status'
                data-testid='cluster-list-warnings'
              >
                {t('cluster.list.configWarnings', {count: warnings.length})}
              </div>
            ) : null}
            <LayoutGroup>
              {viewMode === 'grid' ? (
                <div
                  className='grid min-h-0 flex-1 content-start grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-4 overflow-y-auto p-4'
                  data-testid='cluster-grid-view'
                >
                  {clusters.map((cluster) => (
                    <ClusterCard
                      key={cluster.id}
                      cluster={cluster}
                      active={activeCluster?.id === cluster.id}
                      isDefault={isSoleDefaultCluster}
                      operating={operatingClusterId === cluster.id}
                      flyOrigin={flyOriginRef}
                      onSelect={handleOpenCluster}
                      onEdit={setEditingCluster}
                      onLifecycle={requestLifecycle}
                      onDelete={requestDelete}
                    />
                  ))}
                </div>
              ) : (
                <ClusterListTable
                  clusters={clusters}
                  activeClusterId={activeCluster?.id ?? null}
                  isSoleDefaultCluster={isSoleDefaultCluster}
                  operatingClusterId={operatingClusterId}
                  flyOrigin={flyOriginRef}
                  onOpen={handleOpenCluster}
                  onEdit={setEditingCluster}
                  onLifecycle={requestLifecycle}
                  onDelete={requestDelete}
                />
              )}
            </LayoutGroup>
          </TableCard>
        </div>

        <DetailHost
          open={activeCluster != null}
          hasContent={drawerCluster != null}
          onOpenChange={(open) => {
            if (!open) {
              flyOriginRef.current = null;
              handleOpenCluster(null);
            }
          }}
          narrow={narrow}
          title={drawerCluster?.name ?? ''}
          meta={
            drawerCluster
              ? drawerCluster.cluster_code || `#${drawerCluster.id}`
              : undefined
          }
          nav={detailNav}
          drawerWidth='w-[min(92vw,480px)]'
          flyOrigin={flyOriginRef}
          nameSlot='title'
        >
          {drawerCluster ? (
            <ClusterQuickDetail
              key={drawerCluster.id}
              cluster={drawerCluster}
              operatingAction={
                operatingClusterId === drawerCluster.id ? operatingAction : null
              }
              onEdit={setEditingCluster}
              onLifecycle={requestLifecycle}
              onDelete={requestDelete}
            />
          ) : null}
        </DetailHost>
      </div>

      {/* 生命周期与删除二次确认弹窗 / Shared lifecycle & delete confirmation dialogs */}
      {lifecycleDialogs}

      {/* 注册集群对话框 / Register Cluster Dialog */}
      <CreateClusterDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        onSuccess={() => {
          setIsCreateDialogOpen(false);
          void loadClusters();
        }}
      />

      {/* 部署集群向导 / Deploy Cluster Wizard */}
      <ClusterDeployWizard
        open={isDeployWizardOpen}
        onOpenChange={setIsDeployWizardOpen}
        onComplete={() => {
          void loadClusters();
        }}
      />

      {/* K8s 集群创建对话框 / K8s cluster create dialog */}
      <K8sClusterCreateDialog
        open={isK8sDialogOpen}
        onOpenChange={setIsK8sDialogOpen}
        onSuccess={() => {
          setIsK8sDialogOpen(false);
          void loadClusters();
        }}
      />

      {/* 编辑集群对话框 / Edit Cluster Dialog */}
      {editingCluster ? (
        <EditClusterDialog
          open={editingCluster !== null}
          onOpenChange={(open) => {
            if (!open) {
              setEditingCluster(null);
            }
          }}
          cluster={editingCluster}
          onSuccess={() => {
            setEditingCluster(null);
            void loadClusters();
            toast.success(t('cluster.updateSuccess'));
          }}
        />
      ) : null}
    </div>
  );
}

interface ClusterListTableProps {
  clusters: ClusterInfo[];
  activeClusterId: number | null;
  isSoleDefaultCluster: boolean;
  operatingClusterId: number | null;
  flyOrigin: RefObject<HTMLElement | null>;
  onOpen: (cluster: ClusterInfo) => void;
  onEdit: (cluster: ClusterInfo) => void;
  onLifecycle: (cluster: ClusterInfo, operation: OperationType) => void;
  onDelete: (cluster: ClusterInfo) => void;
}

function ClusterListTable({
  clusters,
  activeClusterId,
  isSoleDefaultCluster,
  operatingClusterId,
  flyOrigin,
  onOpen,
  onEdit,
  onLifecycle,
  onDelete,
}: ClusterListTableProps) {
  const t = useTranslations();
  const router = useRouter();
  const morph = useViewMorph('clusters');

  return (
    <>
      <div className={layout.rowHead} role='row'>
        <span>{t('cluster.name')}</span>
        <span>{t('cluster.status')}</span>
        <span>{t('cluster.deploymentMode')}</span>
        <span>{t('cluster.version')}</span>
        <span>{t('cluster.list.onlineNodes')}</span>
        <span className='text-right'>{t('cluster.actions')}</span>
      </div>
      {/* 行必须是 .tableScroll 的直接子元素，以满足 #178 联动 CSS :has() 选择器约束。
          Rows must be direct children of .tableScroll for #178 linked-motion :has() rules. */}
      <div className={layout.tableScroll} data-testid='cluster-list-view'>
        {clusters.map((cluster) => {
          const isActive = activeClusterId === cluster.id;
          const operating = operatingClusterId === cluster.id;
          const {canStart, canStop, canRestart, canDelete} =
            getClusterActionFlags(cluster);
          const modeKey = `cluster.modes.${cluster.deployment_mode}`;
          const modeLabel = t.has(modeKey)
            ? t(modeKey)
            : cluster.deployment_mode;
          const totalNodes = cluster.node_count ?? 0;
          const onlineNodes = cluster.online_nodes ?? 0;
          const hasOffline = totalNodes > 0 && onlineNodes < totalNodes;

          const menuItems: DropdownItem[] = [
            {
              label: t('cluster.list.openConsole'),
              icon: <ArrowUpRight className='size-3.5' aria-hidden />,
              onSelect: () => router.push(`/clusters/${cluster.id}`),
            },
            {
              label: t('common.edit'),
              icon: <Pencil className='size-3.5' aria-hidden />,
              onSelect: () => onEdit(cluster),
            },
            {
              label: t('stUpgrade.entry'),
              icon: <Activity className='size-3.5' aria-hidden />,
              onSelect: () =>
                router.push(`/clusters/${cluster.id}/upgrade/prepare`),
            },
            {
              label: t('cluster.start'),
              icon: <Play className='size-3.5' aria-hidden />,
              disabled: !canStart || operating,
              separatorBefore: true,
              onSelect: () => onLifecycle(cluster, OperationType.START),
            },
            {
              label: t('cluster.stop'),
              icon: <Square className='size-3.5' aria-hidden />,
              disabled: !canStop || operating,
              onSelect: () => onLifecycle(cluster, OperationType.STOP),
            },
            {
              label: t('cluster.restart'),
              icon: <RotateCcw className='size-3.5' aria-hidden />,
              disabled: !canRestart || operating,
              onSelect: () => onLifecycle(cluster, OperationType.RESTART),
            },
            {
              label: t('common.delete'),
              icon: <Trash2 className='size-3.5' aria-hidden />,
              disabled: !canDelete || operating,
              destructive: true,
              separatorBefore: true,
              onSelect: () => onDelete(cluster),
            },
          ];

          const triggerOpen = (el: HTMLElement | null) => {
            flyOrigin.current = el;
            onOpen(cluster);
          };

          return (
            <div
              key={cluster.id}
              role='button'
              tabIndex={0}
              aria-current={isActive ? 'true' : undefined}
              data-testid={`cluster-row-${cluster.id}`}
              className={cn(layout.row, isActive && layout.rowActive)}
              onClick={(e) => triggerOpen(e.currentTarget)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  triggerOpen(e.currentTarget);
                }
              }}
            >
              <div className={layout.rowName}>
                <div className={layout.rowNameInline}>
                  <motion.span
                    layoutId={morph.id(String(cluster.id), 'title')}
                    transition={morph.transition}
                    data-detail-name
                    className={layout.name}
                    title={cluster.name}
                  >
                    {cluster.name}
                  </motion.span>
                  {isSoleDefaultCluster ? (
                    <NeutralChip>{t('cluster.defaultBadge')}</NeutralChip>
                  ) : null}
                  {cluster.config_error ? (
                    <span data-testid={`cluster-config-error-${cluster.id}`}>
                      <SoftChip tone='amber'>
                        {t('cluster.list.configReadFailed')}
                      </SoftChip>
                    </span>
                  ) : null}
                </div>
                <span
                  className={layout.sub}
                  title={cluster.description || cluster.cluster_code || ''}
                >
                  {cluster.description || cluster.cluster_code || `#${cluster.id}`}
                </span>
              </div>

              <div className='flex min-w-0 items-center'>
                <motion.span
                  layoutId={morph.id(String(cluster.id), 'status')}
                  transition={morph.transition}
                  className='inline-flex min-w-0'
                >
                  <ClusterStatusChip cluster={cluster} />
                </motion.span>
              </div>

              <div className='flex min-w-0 items-center'>
                <motion.span
                  layoutId={morph.id(String(cluster.id), 'mode')}
                  transition={morph.transition}
                  className='inline-flex min-w-0 max-w-full'
                >
                  <NeutralChip title={modeLabel}>{modeLabel}</NeutralChip>
                </motion.span>
              </div>

              <span className='truncate font-mono text-xs text-muted-foreground tabular-nums'>
                {cluster.version ? `v${cluster.version}` : '-'}
              </span>

              <span className='truncate text-xs tabular-nums'>
                <span
                  className={cn(
                    'font-medium',
                    hasOffline
                      ? 'text-rose-600 dark:text-rose-400'
                      : 'text-foreground',
                  )}
                >
                  {onlineNodes}
                </span>
                <span className='text-muted-foreground'> / {totalNodes}</span>
              </span>

              <div
                className={layout.rowActions}
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => e.stopPropagation()}
              >
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  iconOnly
                  aria-label={t('cluster.list.openConsole')}
                  title={t('cluster.list.openConsole')}
                  onClick={() => router.push(`/clusters/${cluster.id}`)}
                >
                  <ArrowUpRight className='size-3.5' aria-hidden />
                </Button>
                <DropdownMenu
                  items={menuItems}
                  trigger={
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('common.moreActions')}
                    >
                      <MoreHorizontal className='size-4' aria-hidden />
                    </Button>
                  }
                />
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}
