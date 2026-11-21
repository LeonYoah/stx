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
 * 主机管理：TableCard + DetailHost（#178 联动/飞入）+ 跨页多选 BatchBar；
 * /hosts/[id] 深链打开常驻详情；状态分段无分桶计数。
 * Host management: TableCard + DetailHost (#178 linked motion/fly-in) +
 * cross-page BatchBar; /hosts/[id] deep-links into the persistent detail;
 * status segments have no bucket counts.
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Layers, Plus, RefreshCw, Server, Trash2} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
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
  BatchBar,
  DetailHost,
  ModuleNavTabs,
  TableCard,
  WorkspaceHeader,
  useDetailNarrow,
  usePagedSelection,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {useServerListQuery} from '@/lib/hooks/use-server-list-query';
import services from '@/lib/services';
import {
  HostInfo,
  HostStatus,
  HostType,
  ListHostsRequest,
} from '@/lib/services/host/types';
import {cn} from '@/lib/utils';
import {CreateHostDialog} from './CreateHostDialog';
import {EditHostDialog} from './EditHostDialog';
import {HostDetail} from './HostDetail';
import {HostInstallGuideDialog} from './HostInstallGuideDialog';
import {HostTable} from './HostTable';

const DEFAULT_PAGE_SIZE = 10;
const ALL_VALUE = 'all';

export interface HostMainProps {
  /** `/hosts/[id]` 深链目标。Deep-link target from `/hosts/[id]`. */
  initialHostId?: number | null;
}

export function HostMain({initialHostId = null}: HostMainProps) {
  const t = useTranslations();
  const router = useRouter();
  const narrow = useDetailNarrow();

  const {page, pageSize, filters, setPage, setPageSize, patchFilters, resetFilters} =
    useServerListQuery({
      initialFilters: {
        name: '',
        hostType: ALL_VALUE,
        status: ALL_VALUE,
      },
      defaultPageSize: DEFAULT_PAGE_SIZE,
    });

  const [nameDraft, setNameDraft] = useState(filters.name);
  const [hosts, setHosts] = useState<HostInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [selectedHostId, setSelectedHostId] = useState<number | null>(
    initialHostId,
  );
  const [selectedHost, setSelectedHost] = useState<HostInfo | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [editHost, setEditHost] = useState<HostInfo | null>(null);
  const [guideHost, setGuideHost] = useState<HostInfo | null>(null);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const [batchBusy, setBatchBusy] = useState(false);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchResult, setBatchResult] = useState<{
    ok: number;
    skipped: number;
    failed: number;
  } | null>(null);
  const flyOriginRef = useRef<HTMLElement | null>(null);
  const listRequestIdRef = useRef(0);
  const detailRequestIdRef = useRef(0);
  const lastSyncedDeepLinkRef = useRef<number | null>(null);

  const pageIds = useMemo(() => hosts.map((h) => h.id), [hosts]);
  const {
    ids: selectedIds,
    setIds: setSelectedIds,
    toggle: toggleSelect,
    togglePage: toggleSelectPage,
    clear: clearSelection,
    pageAllSelected,
    pageSomeSelected,
  } = usePagedSelection(pageIds);

  const drawerOpen = selectedHostId !== null;
  const drawerHost = useDrawerItem(selectedHost);

  const loadHosts = useCallback(async () => {
    const requestId = listRequestIdRef.current + 1;
    listRequestIdRef.current = requestId;
    setLoading(true);
    try {
      const params: ListHostsRequest = {
        current: page,
        size: pageSize,
        name: filters.name || undefined,
        host_type:
          filters.hostType !== ALL_VALUE
            ? (filters.hostType as HostType)
            : undefined,
        status:
          filters.status !== ALL_VALUE
            ? (filters.status as HostStatus)
            : undefined,
      };
      const result = await services.host.getHostsSafe(params);
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      if (result.success && result.data) {
        setHosts(result.data.hosts || []);
        setTotal(result.data.total || 0);
        setError(null);
      } else {
        setError(result.error || t('host.loadError'));
      }
    } catch (err) {
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      setError(err instanceof Error ? err.message : t('host.loadError'));
    } finally {
      if (listRequestIdRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [page, pageSize, filters, t]);

  useEffect(() => {
    void loadHosts();
  }, [loadHosts]);

  const loadHostDetail = useCallback(
    async (hostId: number, seed?: HostInfo) => {
      const requestId = detailRequestIdRef.current + 1;
      detailRequestIdRef.current = requestId;
      if (seed && seed.id === hostId) {
        setSelectedHost(seed);
      }
      setLoadingDetail(true);
      try {
        const result = await services.host.getHostSafe(hostId);
        if (detailRequestIdRef.current !== requestId) {
          return;
        }
        if (result.success && result.data) {
          setSelectedHost(result.data);
        } else if (!seed) {
          toast.error(result.error || t('host.loadError'));
          setSelectedHostId(null);
          setSelectedHost(null);
          lastSyncedDeepLinkRef.current = null;
          router.replace('/hosts');
        }
      } finally {
        if (detailRequestIdRef.current === requestId) {
          setLoadingDetail(false);
        }
      }
    },
    [router, t],
  );

  // 深链：外部 /hosts/[id]（含浏览器前进/后退）打开对应常驻详情，跳过行内已触发的同 id。
  // Deep link: external /hosts/[id] (including Back/Forward) opens the persistent detail; skip ids already opened in-page.
  useEffect(() => {
    if (initialHostId && initialHostId !== lastSyncedDeepLinkRef.current) {
      lastSyncedDeepLinkRef.current = initialHostId;
      setSelectedHostId(initialHostId);
      void loadHostDetail(initialHostId);
    }
  }, [initialHostId, loadHostDetail]);

  const openHostDetail = useCallback(
    (host: HostInfo, origin?: HTMLElement | null) => {
      // 仅点击/回车写入 flyOrigin；J/K 与深链不写（#178）。
      // Only click/Enter write flyOrigin; J/K and deep links do not (#178).
      if (origin) {
        flyOriginRef.current = origin;
      } else {
        flyOriginRef.current = null;
      }
      setSelectedHostId(host.id);
      setSelectedHost(host);
      void loadHostDetail(host.id, host);
      // 仅在深链路由上同步 path；layout.tsx 对 `/hosts/*` 共用同一 key，不触发整页重挂。
      // Sync path only on the deep-link route; layout.tsx shares one key for `/hosts/*` so switching rows will not remount.
      if (initialHostId != null) {
        lastSyncedDeepLinkRef.current = host.id;
        router.replace(`/hosts/${host.id}`, {scroll: false});
      }
    },
    [initialHostId, loadHostDetail, router],
  );

  const handleCloseDetail = useCallback(() => {
    detailRequestIdRef.current += 1;
    lastSyncedDeepLinkRef.current = null;
    setLoadingDetail(false);
    setSelectedHostId(null);
    setSelectedHost(null);
    if (initialHostId != null) {
      router.replace('/hosts', {scroll: false});
    }
  }, [initialHostId, router]);

  const commitName = useCallback(() => {
    const next = nameDraft.trim();
    if (next !== filters.name) {
      patchFilters({name: next});
    }
  }, [nameDraft, filters.name, patchFilters]);

  const handleEdit = (host: HostInfo) => {
    setEditHost(host);
    setIsEditDialogOpen(true);
  };

  const handleDelete = async (host: HostInfo) => {
    const result = await services.host.deleteHostSafe(host.id);
    if (result.success) {
      toast.success(t('host.deleteSuccess'));
      if (selectedHostId === host.id) {
        handleCloseDetail();
      }
      {
        const next = new Set(selectedIds);
        next.delete(host.id);
        setSelectedIds(next);
      }
      void loadHosts();
    } else {
      toast.error(result.error || t('host.deleteError'));
    }
  };

  const runBatchDelete = useCallback(async () => {
    const ids = Array.from(selectedIds);
    if (ids.length === 0 || batchBusy) {
      return;
    }
    setBatchBusy(true);
    setBatchResult(null);
    const outcome = {ok: 0, skipped: 0, failed: 0};
    const failedIds: number[] = [];
    for (const id of ids) {
      const result = await services.host.deleteHostSafe(id);
      if (result.success) {
        outcome.ok += 1;
        if (selectedHostId === id) {
          handleCloseDetail();
        }
      } else {
        outcome.failed += 1;
        failedIds.push(id);
      }
    }
    setBatchResult(outcome);
    setBatchBusy(false);
    setBatchDeleteOpen(false);
    // 保留失败项勾选以便直接重试，清除已成功项。Keep failed items selected for direct retry; clear succeeded ones.
    setSelectedIds(new Set(failedIds));
    void loadHosts();
    const summary = t('host.batchResult', {
      ok: outcome.ok,
      skipped: outcome.skipped,
      failed: outcome.failed,
    });
    if (outcome.failed > 0 && outcome.ok === 0) {
      toast.error(summary);
    } else if (outcome.failed > 0 || outcome.skipped > 0) {
      toast.warning(summary);
    } else {
      toast.success(summary);
    }
  }, [
    selectedIds,
    batchBusy,
    selectedHostId,
    handleCloseDetail,
    setSelectedIds,
    loadHosts,
    t,
  ]);

  const handleHostCreated = (newHost?: HostInfo) => {
    void loadHosts();
    if (newHost && newHost.host_type !== HostType.BARE_METAL) {
      setIsCreateDialogOpen(false);
    }
  };

  const handleOpenInstallGuide = (host: HostInfo) => {
    setGuideHost(host);
    setIsGuideOpen(true);
  };

  const handleHostUpdated = () => {
    setIsEditDialogOpen(false);
    void loadHosts();
    // 仅刷新当前打开的同一台主机详情，不覆写关闭态或其他主机。Only refresh when the edited host is currently open.
    if (editHost && selectedHostId === editHost.id) {
      void loadHostDetail(editHost.id);
    }
    toast.success(t('host.updateSuccess'));
  };

  const handleClearFilters = () => {
    setNameDraft('');
    resetFilters();
  };

  const filtersActive =
    Boolean(filters.name) ||
    filters.hostType !== ALL_VALUE ||
    filters.status !== ALL_VALUE;

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const activeIndex = selectedHost
    ? hosts.findIndex((h) => h.id === selectedHost.id)
    : -1;
  const detailNav =
    selectedHost && activeIndex >= 0
      ? {
          index: activeIndex,
          total: hosts.length,
          // J/K / 头部导航不写 flyOrigin。J/K and header nav never write flyOrigin.
          onPrev: () =>
            activeIndex > 0 && openHostDetail(hosts[activeIndex - 1]),
          onNext: () =>
            activeIndex < hosts.length - 1 &&
            openHostDetail(hosts[activeIndex + 1]),
        }
      : undefined;

  return (
    <div className={cn(layout.page, layout.pageViewport)}>
      <WorkspaceHeader
        icon={<Server />}
        title={t('host.title')}
        subtitle={t('host.description')}
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
            activeKey='hosts'
          />
        }
        actions={
          <>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => void loadHosts()}
              disabled={loading}
            >
              <RefreshCw
                className={cn('size-3.5', loading && 'animate-spin')}
                aria-hidden
              />
              {t('common.refresh')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={() => setIsCreateDialogOpen(true)}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('host.createHost')}
            </Button>
          </>
        }
      />

      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('host.status')}
          value={filters.status}
          onValueChange={(v) => patchFilters({status: v})}
          options={[
            {value: ALL_VALUE, label: t('host.allStatuses')},
            {
              value: HostStatus.CONNECTED,
              label: t('host.statuses.connected'),
            },
            {value: HostStatus.PENDING, label: t('host.statuses.pending')},
            {value: HostStatus.OFFLINE, label: t('host.statuses.offline')},
            {value: HostStatus.ERROR, label: t('host.statuses.error')},
          ]}
        />
        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('host.searchPlaceholder')}
            value={nameDraft}
            onValueChange={(v) => {
              setNameDraft(v);
              if (v === '' && filters.name !== '') {
                patchFilters({name: ''});
              }
            }}
            placeholder={t('host.searchPlaceholder')}
            clearLabel={t('common.searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitName();
              }
            }}
          />
        </div>
        <div className={cn(layout.filterCtrl, layout.filterWmd)}>
          <Select
            label={t('host.hostType')}
            value={filters.hostType}
            onValueChange={(v) => patchFilters({hostType: v})}
            displayValue={
              filters.hostType === ALL_VALUE ? t('host.hostType') : undefined
            }
            options={[
              {value: ALL_VALUE, label: t('host.allTypes')},
              {value: HostType.BARE_METAL, label: t('host.types.bareMetal')},
              {value: HostType.DOCKER, label: t('host.types.docker')},
              {value: HostType.KUBERNETES, label: t('host.types.kubernetes')},
            ]}
          />
        </div>
        {filtersActive ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={handleClearFilters}
          >
            {t('common.clearFilters')}
          </Button>
        ) : null}
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='hosts'>
          <TableCard
            loading={loading}
            hasData={hosts.length > 0}
            skeletonColumns={8}
            skeletonRows={8}
            error={error}
            onRetry={() => void loadHosts()}
            pagination={{
              currentPage: page,
              totalPages,
              pageSize,
              totalItems: total,
              onPageChange: setPage,
              onPageSizeChange: setPageSize,
              pageSizeOptions: [10, 20, 50, 100],
            }}
            empty={
              <EmptyState
                title={t('host.noHosts')}
                description={
                  filtersActive
                    ? t('common.noMatchingData')
                    : t('host.description')
                }
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={handleClearFilters}
                    >
                      {t('common.clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <HostTable
              hosts={hosts}
              selectedHostId={selectedHostId}
              selectedIds={selectedIds}
              pageAllSelected={pageAllSelected}
              pageSomeSelected={pageSomeSelected}
              onToggleSelect={toggleSelect}
              onToggleSelectPage={toggleSelectPage}
              onViewDetail={openHostDetail}
              onEdit={handleEdit}
              onDelete={(host) => void handleDelete(host)}
              onInstallAgent={handleOpenInstallGuide}
            />
          </TableCard>

          <BatchBar
            selectedCount={selectedIds.size}
            busy={batchBusy}
            result={
              batchResult
                ? t('host.batchResult', {
                    ok: batchResult.ok,
                    skipped: batchResult.skipped,
                    failed: batchResult.failed,
                  })
                : null
            }
            onClear={() => {
              clearSelection();
              setBatchResult(null);
            }}
          >
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={batchBusy || selectedIds.size === 0}
              onClick={() => setBatchDeleteOpen(true)}
            >
              <Trash2 className='size-3.5' aria-hidden />
              {t('host.batchDelete')}
            </Button>
          </BatchBar>
        </div>

        <DetailHost
          open={drawerOpen}
          hasContent={drawerHost != null || loadingDetail}
          onOpenChange={(o) => {
            if (!o) {
              handleCloseDetail();
            }
          }}
          narrow={narrow}
          title={drawerHost?.name || t('host.title')}
          meta={drawerHost?.ip_address || drawerHost?.description}
          nav={detailNav}
          flyOrigin={flyOriginRef}
          nameSlot='title'
          wide
          drawerWidth='w-[min(92vw,640px)]'
        >
          <HostDetail
            key={drawerHost?.id ?? 'pending'}
            host={drawerHost ?? null}
            loading={loadingDetail}
            onEdit={() => {
              if (drawerHost) {
                handleEdit(drawerHost);
              }
            }}
          />
        </DetailHost>
      </div>

      <CreateHostDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        onSuccess={handleHostCreated}
        onViewDetail={openHostDetail}
      />

      <HostInstallGuideDialog
        open={isGuideOpen}
        onOpenChange={setIsGuideOpen}
        host={guideHost}
        onConnected={() => {
          void loadHosts();
        }}
        onViewDetail={openHostDetail}
      />

      {editHost ? (
        <EditHostDialog
          open={isEditDialogOpen}
          onOpenChange={setIsEditDialogOpen}
          host={editHost}
          onSuccess={handleHostUpdated}
        />
      ) : null}

      <AlertDialog open={batchDeleteOpen} onOpenChange={setBatchDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('host.batchDeleteConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('host.batchDeleteConfirmDesc', {count: selectedIds.size})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={batchBusy}
              onClick={() => void runBatchDelete()}
            >
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
