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
 * 主机管理：TableCard + DetailHost；/hosts/[id] 深链打开常驻详情；
 * 状态分段无分桶计数；创建/编辑/安装对话框保持原样。
 * Host management: TableCard + DetailHost; /hosts/[id] deep-links into the
 * persistent detail; status segments have no bucket counts; create/edit/install
 * dialogs stay as they are.
 */

import {useCallback, useEffect, useState} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Layers, Plus, RefreshCw, Server} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  DetailHost,
  ModuleNavTabs,
  TableCard,
  WorkspaceHeader,
  useDetailNarrow,
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

  const drawerOpen = selectedHostId !== null;
  const drawerHost = useDrawerItem(selectedHost);

  const loadHosts = useCallback(async () => {
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
      if (result.success && result.data) {
        setHosts(result.data.hosts || []);
        setTotal(result.data.total || 0);
        setError(null);
      } else {
        setError(result.error || t('host.loadError'));
        setHosts([]);
        setTotal(0);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('host.loadError'));
      setHosts([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, filters, t]);

  useEffect(() => {
    void loadHosts();
  }, [loadHosts]);

  const loadHostDetail = useCallback(
    async (hostId: number, seed?: HostInfo) => {
      if (seed && seed.id === hostId) {
        setSelectedHost(seed);
      }
      setLoadingDetail(true);
      try {
        const result = await services.host.getHostSafe(hostId);
        if (result.success && result.data) {
          setSelectedHost(result.data);
        } else if (!seed) {
          toast.error(result.error || t('host.loadError'));
          setSelectedHostId(null);
          setSelectedHost(null);
          router.replace('/hosts');
        }
      } finally {
        setLoadingDetail(false);
      }
    },
    [router, t],
  );

  // 深链：外部 /hosts/[id] 打开对应常驻详情。
  // Deep link: /hosts/[id] opens the matching persistent detail.
  useEffect(() => {
    if (initialHostId) {
      setSelectedHostId(initialHostId);
      void loadHostDetail(initialHostId);
    }
  }, [initialHostId, loadHostDetail]);

  const openHostDetail = useCallback(
    (host: HostInfo) => {
      setSelectedHostId(host.id);
      setSelectedHost(host);
      void loadHostDetail(host.id, host);
      // 仅在深链路由上同步 path，避免 /hosts 列表选中时整页重挂丢失筛选。
      // Sync the path only on the deep-link route so selecting from /hosts
      // does not remount and wipe filters.
      if (initialHostId != null) {
        router.replace(`/hosts/${host.id}`);
      }
    },
    [initialHostId, loadHostDetail, router],
  );

  const handleCloseDetail = useCallback(() => {
    setSelectedHostId(null);
    setSelectedHost(null);
    if (initialHostId != null) {
      router.replace('/hosts');
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
      void loadHosts();
    } else {
      toast.error(result.error || t('host.deleteError'));
    }
  };

  const handleHostCreated = (newHost?: HostInfo) => {
    void loadHosts();
    if (newHost && newHost.host_type !== HostType.BARE_METAL) {
      setIsCreateDialogOpen(false);
      toast.success(t('host.createSuccess'));
    }
  };

  const handleOpenInstallGuide = (host: HostInfo) => {
    setGuideHost(host);
    setIsGuideOpen(true);
  };

  const handleHostUpdated = () => {
    setIsEditDialogOpen(false);
    void loadHosts();
    if (editHost) {
      void loadHostDetail(editHost.id, editHost);
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
            skeletonColumns={7}
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
              onViewDetail={openHostDetail}
              onEdit={handleEdit}
              onDelete={(host) => void handleDelete(host)}
              onInstallAgent={handleOpenInstallGuide}
            />
          </TableCard>
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
    </div>
  );
}
