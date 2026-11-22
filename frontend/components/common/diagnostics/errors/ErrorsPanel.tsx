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
 * 错误组面板：频次分段（服务端 min/max_occurrences）+ 关键字搜索 + TableCard 列表
 * + 常驻详情栏 / 抽屉，与巡检策略面板、告警中心同款布局；group_id 随 URL 深链。
 * Error group panel: frequency segments (server-side min/max_occurrences) +
 * keyword search + TableCard list + persistent rail / drawer, same layout as the
 * policies panel and the alert center; group_id stays deep-linked via the URL.
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Lightbulb, RefreshCw} from 'lucide-react';
import {toast} from '@/lib/toast';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  DiagnosticsClusterOption,
  DiagnosticsErrorGroup,
  DiagnosticsErrorEvent,
} from '@/lib/services/diagnostics';
import {cn} from '@/lib/utils';
import {DiagnosticsClusterFilter} from '../DiagnosticsClusterFilter';
import {ErrorGroupDetail} from './ErrorGroupDetail';
import {
  formatErrorDateTime,
  formatNodeOrigin,
  heatFilterToOccurrenceRange,
  occurrenceHeatTone,
  type ErrorHeatFilter,
} from './error-utils';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {SoftChip, NeutralChip, TruncatedText} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';

const PAGE_SIZE = 20;
const EVENT_LIMIT = 20;

export interface ErrorsPanelProps {
  /** 工作台所选集群：列表与事件按事件维度预过滤。Selected workspace cluster; list and events are pre-filtered on the event scope. */
  clusterId?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** URL 深链的目标错误组。Deep-linked error group id from the URL. */
  groupId?: number;
  onSelectGroup?: (groupId: number | null) => void;
  /** 工具栏集群筛选变化时回填 cluster_id。Writes cluster_id back through the toolbar filter. */
  onClusterChange?: (clusterId: number | undefined) => void;
}

export function ErrorsPanel({
  clusterId,
  clusterOptions,
  groupId,
  onSelectGroup,
  onClusterChange,
}: ErrorsPanelProps) {
  const t = useTranslations('diagnosticsCenter.errors');
  const tsT = useTranslations('troubleshooting');
  const commonT = useTranslations('common');
  const narrow = useDetailNarrow();

  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState('');
  const [heatFilter, setHeatFilter] = useState<ErrorHeatFilter>('all');
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [groups, setGroups] = useState<DiagnosticsErrorGroup[]>([]);
  const [groupTotal, setGroupTotal] = useState(0);

  const [selectedGroupId, setSelectedGroupId] = useState<number | null>(
    groupId ?? null,
  );
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedGroup, setSelectedGroup] =
    useState<DiagnosticsErrorGroup | null>(null);
  const [groupEvents, setGroupEvents] = useState<DiagnosticsErrorEvent[]>([]);
  const groupsRequestIdRef = useRef(0);
  const detailRequestIdRef = useRef(0);
  const searchRef = useRef<HTMLInputElement>(null);
  // 名称飞入来源行：仅点击/回车打开时写入，J/K、头部 ↑↓ 与深链不写。
  // Fly-in origin row: written only on click/Enter open — never by J/K,
  // header arrows or deep links.
  const flyOriginRef = useRef<HTMLElement | null>(null);
  const [memoriesVersion, setMemoriesVersion] = useState(0);

  const drawerOpen = selectedGroupId !== null;
  const drawerGroup = useDrawerItem(selectedGroup);

  const clearSelectedGroupDetail = useCallback(() => {
    detailRequestIdRef.current += 1;
    setLoadingDetail(false);
    setSelectedGroup(null);
    setGroupEvents([]);
  }, []);

  const loadGroups = useCallback(async () => {
    const requestId = groupsRequestIdRef.current + 1;
    groupsRequestIdRef.current = requestId;
    setLoading(true);
    try {
      const result = await services.diagnostics.getErrorGroupsSafe({
        cluster_id: clusterId,
        keyword: keyword || undefined,
        page,
        page_size: PAGE_SIZE,
        ...heatFilterToOccurrenceRange(heatFilter),
      });
      if (groupsRequestIdRef.current !== requestId) {
        return;
      }
      if (!result.success || !result.data) {
        setError(result.error || t('loadGroupsError'));
        setGroups([]);
        setGroupTotal(0);
        return;
      }
      setError(null);
      setGroups(result.data.items || []);
      setGroupTotal(result.data.total || 0);
    } finally {
      if (groupsRequestIdRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [clusterId, keyword, heatFilter, page, t]);

  const loadGroupDetail = useCallback(
    async (targetGroupId: number) => {
      const requestId = detailRequestIdRef.current + 1;
      detailRequestIdRef.current = requestId;
      setLoadingDetail(true);
      try {
        const result = await services.diagnostics.getErrorGroupDetailSafe(
          targetGroupId,
          EVENT_LIMIT,
          {cluster_id: clusterId},
        );
        if (detailRequestIdRef.current !== requestId) {
          return;
        }
        if (!result.success || !result.data) {
          toast.error(result.error || t('loadDetailError'));
          return;
        }
        setSelectedGroup(result.data.group);
        setGroupEvents(result.data.events || []);
      } finally {
        if (detailRequestIdRef.current === requestId) {
          setLoadingDetail(false);
        }
      }
    },
    [clusterId, t],
  );

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  // URL 深链：外部传入 group_id 时打开对应详情。
  // Deep link: an external group_id opens that group's detail.
  useEffect(() => {
    if (groupId) {
      setSelectedGroupId(groupId);
      void loadGroupDetail(groupId);
    }
  }, [groupId, loadGroupDetail]);

  const openGroupDetail = useCallback(
    (targetGroup: DiagnosticsErrorGroup) => {
      setSelectedGroupId(targetGroup.id);
      setSelectedGroup(targetGroup);
      onSelectGroup?.(targetGroup.id);
      void loadGroupDetail(targetGroup.id);
    },
    [loadGroupDetail, onSelectGroup],
  );

  const handleCloseDetail = useCallback(() => {
    setSelectedGroupId(null);
    clearSelectedGroupDetail();
    onSelectGroup?.(null);
  }, [clearSelectedGroupDetail, onSelectGroup]);

  // 「/」聚焦搜索。「/」 focuses the search field.
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

  const commitKeyword = useCallback(() => {
    setKeyword(keywordInput.trim());
    setPage(1);
  }, [keywordInput]);

  // 当前页行命中方案映射（本地记忆存储，同步计算）。
  // Per-page matched-solution map (local memory store, computed synchronously).
  const groupMemoryMap = useMemo(() => {
    const map = new Map<number, boolean>();
    groups.forEach((group) => {
      const matches = services.troubleshooting.findMatchingMemories({
        fingerprint: group.fingerprint,
        exception_class: group.exception_class,
        title: group.title,
        target_type: 'error',
      });
      if (matches.length > 0) {
        map.set(group.id, true);
      }
    });
    return map;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groups, memoriesVersion]);

  const totalPages = Math.max(1, Math.ceil(groupTotal / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);

  const activeIndex = selectedGroup
    ? groups.findIndex((g) => g.id === selectedGroup.id)
    : -1;
  const detailNav =
    selectedGroup && activeIndex >= 0
      ? {
          index: activeIndex,
          total: groups.length,
          onPrev: () =>
            activeIndex > 0 && openGroupDetail(groups[activeIndex - 1]),
          onNext: () =>
            activeIndex < groups.length - 1 &&
            openGroupDetail(groups[activeIndex + 1]),
        }
      : undefined;

  const onRowKeyDown = (e: React.KeyboardEvent<HTMLElement>, id: number) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const target = groups.find((g) => g.id === id);
      if (target) {
        if (selectedGroupId !== target.id) {
          flyOriginRef.current = e.currentTarget;
        }
        openGroupDetail(target);
      }
    }
  };

  const filtersActive = keyword !== '' || heatFilter !== 'all';
  const seg = (key: ErrorHeatFilter, label: string) => ({value: key, label});

  return (
    <div className={layout.page} style={{overflowY: 'auto'}}>
      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('freqLabel')}
          value={heatFilter}
          onValueChange={(v) => {
            setHeatFilter(v as ErrorHeatFilter);
            setPage(1);
          }}
          options={[
            seg('all', t('freqAll')),
            seg('high', t('freqHigh')),
            seg('mid', t('freqMid')),
            seg('low', t('freqLow')),
          ]}
        />
        <div className={layout.toolbarSearch}>
          <SearchField
            ref={searchRef}
            label={t('searchLabel')}
            value={keywordInput}
            onValueChange={(v) => {
              setKeywordInput(v);
              // 清空输入即移除已提交关键字；新关键字仍按 Enter 提交。
              // Clearing the input also drops the committed keyword; new
              // keywords still submit on Enter.
              if (v === '' && keyword !== '') {
                setKeyword('');
                setPage(1);
              }
            }}
            placeholder={t('keywordPlaceholder')}
            clearLabel={t('searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitKeyword();
              } else if (e.key === 'Escape' && keywordInput) {
                setKeywordInput('');
                if (keyword !== '') {
                  setKeyword('');
                  setPage(1);
                }
              }
            }}
          />
        </div>
        <DiagnosticsClusterFilter
          value={clusterId}
          clusterOptions={clusterOptions}
          onChange={onClusterChange}
        />
        <div className={layout.toolbarRight}>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => void loadGroups()}
            disabled={loading}
            data-testid='errors-refresh'
          >
            <RefreshCw
              className={cn('size-3.5', loading && 'animate-spin')}
              aria-hidden
            />
            {commonT('refresh')}
          </Button>
        </div>
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='errors'>
          {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
              Single main card; skeleton/error/empty states are owned by TableCard. */}
          <TableCard
            loading={loading}
            hasData={groups.length > 0}
            skeletonColumns={5}
            skeletonRows={6}
            error={error}
            onRetry={() => void loadGroups()}
            pagination={{
              currentPage: safePage,
              totalPages,
              pageSize: PAGE_SIZE,
              totalItems: groupTotal,
              onPageChange: setPage,
            }}
            empty={
              <EmptyState
                title={filtersActive ? t('empty') : t('emptyAll')}
                description={filtersActive ? t('emptyDesc') : t('emptyAllDesc')}
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setKeywordInput('');
                        setKeyword('');
                        setHeatFilter('all');
                        setPage(1);
                      }}
                    >
                      {commonT('clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={layout.rowHead} aria-hidden>
              <span>{t('columns.group')}</span>
              <span>{t('columns.exception')}</span>
              <span>{t('columns.node')}</span>
              <span>{t('columns.occurrences')}</span>
              <span>{t('columns.lastSeen')}</span>
            </div>
            <div className={layout.tableScroll}>
              {groups.map((group) => (
                <div
                  key={group.id}
                  data-mrow={group.id}
                  data-testid={`error-row-${group.id}`}
                  role='button'
                  tabIndex={0}
                  onClick={(e) => {
                    if (selectedGroupId !== group.id) {
                      flyOriginRef.current = e.currentTarget;
                    }
                    openGroupDetail(group);
                  }}
                  onKeyDown={(e) => onRowKeyDown(e, group.id)}
                  className={cn(
                    layout.row,
                    selectedGroupId === group.id && layout.rowActive,
                  )}
                >
                  <div className={layout.rowName}>
                    <div className={layout.rowNameInline}>
                      <TruncatedText
                        text={group.title}
                        className={layout.name}
                        detailName
                      />
                      {groupMemoryMap.get(group.id) ? (
                        <SoftChip tone='emerald'>
                          <Lightbulb className='size-3' aria-hidden />
                          {tsT('hasSolution')}
                        </SoftChip>
                      ) : null}
                    </div>
                    <TruncatedText
                      text={group.sample_message || group.fingerprint}
                      className={layout.sub}
                    />
                  </div>
                  <NeutralChip>{group.exception_class || '-'}</NeutralChip>
                  <TruncatedText
                    text={formatNodeOrigin({
                      nodeId: group.last_node_id,
                      hostId: group.last_host_id,
                      hostName: group.last_host_name,
                      hostIp: group.last_host_ip,
                    })}
                    className={layout.sub}
                  />
                  <SoftChip tone={occurrenceHeatTone(group.occurrence_count)}>
                    {t('occurrencesShort', {count: group.occurrence_count})}
                  </SoftChip>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {formatErrorDateTime(group.last_seen_at)}
                  </span>
                </div>
              ))}
            </div>
          </TableCard>
        </div>

        <DetailHost
          open={drawerOpen}
          hasContent={drawerGroup != null || loadingDetail}
          onOpenChange={(o) => !o && handleCloseDetail()}
          narrow={narrow}
          title={t('detailTitle')}
          meta={drawerGroup?.title}
          nav={detailNav}
          wide
          drawerWidth='w-[min(92vw,560px)]'
          flyOrigin={flyOriginRef}
          nameSlot='meta'
        >
          <ErrorGroupDetail
            key={drawerGroup?.id ?? 'pending'}
            group={drawerGroup ?? null}
            events={groupEvents}
            loading={loadingDetail}
            clusterOptions={clusterOptions}
            memoriesVersion={memoriesVersion}
            onMemoriesChanged={() => setMemoriesVersion((v) => v + 1)}
            onClose={handleCloseDetail}
          />
        </DetailHost>
      </div>
    </div>
  );
}
