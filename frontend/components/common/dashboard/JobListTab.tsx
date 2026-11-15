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

import {useCallback, useEffect, useMemo, useState} from 'react';
import Link from 'next/link';
import {useFormatter, useTranslations} from 'next-intl';
import {Search} from 'lucide-react';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Sparkline} from '@/components/charts/sparkline';
import {
  StudioSortableColumnHeader,
  StudioTable,
  StudioTableBody,
  StudioTableCell,
  StudioTableColumn,
  StudioTableContent,
  StudioTableEmptyRow,
  StudioTableFooter,
  StudioTableHeader,
  StudioTableRow,
  StudioTableScroll,
} from '@/components/common/sync/StudioDataTable';
import {
  getDisplayJobLifecycleStatus,
  getJobStatusBadgeClass,
  getJobStatusLabel,
} from '@/components/common/sync/sync-studio-utils';
import {
  StatPillsBar,
  TableLoadingBar,
  type StatPillItem,
} from '@/components/common/layout';
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
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {Input} from '@/components/ui/input';
import {Pagination} from '@/components/ui/pagination';
import services from '@/lib/services';
import type {
  SyncJobInstance,
  SyncJobListData,
  SyncJobListSort,
  SyncJobSparklines,
  SyncJobStatusGroup,
} from '@/lib/services/sync';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {formatCompactNumber, formatDurationSeconds} from './dashboard-format';
import {JobPreviewDrawer} from './JobPreviewDrawer';
import {
  isRunningStatus,
  jobDisplayName,
  jobWorkbenchHref,
} from './job-list-utils';
import {usePolledResource} from './use-polled-resource';
import {type JobBulkAction, useJobBulkActions} from './use-job-bulk-actions';
import type {DashboardQuery} from './use-dashboard-query';

const PAGE_SIZE = 50;
const LIST_POLL_MS = 30_000;
const STATUS_GROUPS: SyncJobStatusGroup[] = [
  'failed',
  'running',
  'pending',
  'stopping',
  'canceled',
  'finished',
];
const PILL_VARIANT: Partial<
  Record<SyncJobStatusGroup, StatPillItem['variant']>
> = {
  failed: 'danger',
  running: 'info',
  pending: 'warning',
  finished: 'success',
};

/**
 * 全局作业列表页签：状态胶囊、模式与关键字筛选、服务端排序分页、批量迷你曲线、多选停止 / 恢复与行预览。
 * 筛选条件都在 URL 上；预览运行不计入。
 * The global job list tab: status pills, mode and keyword filters, server-side sort and paging, batched sparklines,
 * multi-select stop / recover and a row preview. Filters live in the URL; preview runs are excluded.
 */
export function JobListTab({
  query,
  update,
  clusterNames,
}: {
  query: DashboardQuery;
  update: (
    patch: Partial<Record<keyof DashboardQuery, string | number | undefined>>,
  ) => void;
  clusterNames: Map<number, string>;
}) {
  const t = useTranslations('dashboard.home.list');
  const format = useFormatter();
  const [keyword, setKeyword] = useState(query.q);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [preview, setPreview] = useState<SyncJobInstance | null>(null);
  const [confirm, setConfirm] = useState<JobBulkAction | null>(null);
  const [sparklines, setSparklines] = useState<SyncJobSparklines | null>(null);

  useEffect(() => setKeyword(query.q), [query.q]);

  const statusGroup = (STATUS_GROUPS as string[]).includes(query.status)
    ? (query.status as SyncJobStatusGroup)
    : '';
  const runningOnly = statusGroup === 'running';
  // 速率 / 延迟排序要求只看运行中；其余情况退回「需要关注优先」。
  // Rate / delay sorts require the running group; otherwise fall back to attention-first.
  const sort: SyncJobListSort =
    (query.sort === 'rate' || query.sort === 'cdc_delay') && !runningOnly
      ? 'attention'
      : (query.sort as SyncJobListSort) || 'attention';
  const order = query.order || 'desc';

  const listKey = JSON.stringify([
    query.clusterId,
    statusGroup,
    query.mode,
    query.q,
    sort,
    order,
    query.page,
  ]);
  const list = usePolledResource<SyncJobListData>(
    listKey,
    () =>
      services.sync.listJobs({
        current: query.page,
        size: PAGE_SIZE,
        status_group: statusGroup || undefined,
        mode:
          query.mode === 'streaming' || query.mode === 'batch'
            ? query.mode
            : undefined,
        cluster_id: query.clusterId,
        keyword: query.q || undefined,
        exclude_run_type: 'preview',
        sort,
        order,
        with_counts: 1,
        with_live: 1,
      }),
    LIST_POLL_MS,
  );
  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  // 选择只保留当前页仍存在的行。/ Keep only selections that still exist on this page.
  useEffect(() => {
    setSelected((prev) => {
      const ids = new Set(items.map((job) => job.id));
      const next = new Set([...prev].filter((id) => ids.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [items]);

  // 迷你曲线：每次列表刷新后，用一次请求取本页运行中作业的写入速率。
  // Sparklines: after each list refresh, one request fetches write rates for this page's running jobs.
  const runningIds = useMemo(
    () =>
      items.filter((job) => isRunningStatus(job.status)).map((job) => job.id),
    [items],
  );
  const sparkKey = runningIds.join(',');
  useEffect(() => {
    if (!sparkKey) {
      setSparklines(null);
      return;
    }
    let disposed = false;
    services.sync
      .getJobSparklines({ids: sparkKey.split(',').map(Number), range: '1h'})
      .then((data) => !disposed && setSparklines(data))
      .catch(() => !disposed && setSparklines(null));
    return () => {
      disposed = true;
    };
  }, [sparkKey, list.updatedAt]);
  const showSparklines = sparklines?.source === 'prometheus';

  const bulk = useJobBulkActions(list.reload);
  const selectedJobs = items.filter((job) => selected.has(job.id));

  const counts = list.data?.status_counts ?? {};
  const totalAll = STATUS_GROUPS.reduce(
    (sum, group) => sum + (counts[group] ?? 0),
    0,
  );
  const pills: StatPillItem[] = [
    {
      key: 'all',
      label: t('groups.all'),
      count: totalAll,
      dataTestId: 'home-job-pill-all',
    },
    ...STATUS_GROUPS.map((group) => ({
      key: group,
      label: t(`groups.${group}`),
      count: counts[group] ?? 0,
      variant: PILL_VARIANT[group],
      pulse: group === 'failed' && (counts.failed ?? 0) > 0,
      dataTestId: `home-job-pill-${group}`,
    })),
  ];

  const toggleSort = useCallback(
    (key: SyncJobListSort) => {
      const nextOrder = sort === key && order === 'desc' ? 'asc' : 'desc';
      update({sort: key, order: nextOrder, page: 1});
    },
    [order, sort, update],
  );
  const sortHeader = (key: SyncJobListSort, label: string) => (
    <StudioSortableColumnHeader
      label={label}
      sortDirection={sort === key ? order : undefined}
      onSort={() => toggleSort(key)}
    />
  );

  const durationUnits = {d: t('units.d'), h: t('units.h'), m: t('units.m')};
  const totalPages = Math.max(
    1,
    Math.ceil((list.data?.total ?? 0) / PAGE_SIZE),
  );
  const allOnPageSelected =
    items.length > 0 && items.every((job) => selected.has(job.id));
  const COLS = 10;

  return (
    <div className='flex flex-col gap-3' data-testid='home-job-list'>
      <StatPillsBar
        items={pills}
        activeKey={statusGroup || 'all'}
        onChange={(key) =>
          update({status: key === 'all' ? undefined : key, page: 1})
        }
      />

      <div className='flex flex-wrap items-center gap-2'>
        <form
          className='relative min-w-[200px] max-w-sm flex-1'
          onSubmit={(event) => {
            event.preventDefault();
            update({q: keyword.trim() || undefined, page: 1});
          }}
        >
          <Search className='pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground' />
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('searchPlaceholder')}
            aria-label={t('searchPlaceholder')}
            className='h-8 pl-8 text-xs'
            data-testid='home-job-search'
          />
        </form>
        <SegmentedControl
          label={t('columns.mode')}
          value={
            query.mode === 'streaming' || query.mode === 'batch'
              ? query.mode
              : 'all'
          }
          onValueChange={(value) =>
            update({mode: value === 'all' ? undefined : value, page: 1})
          }
          options={[
            {value: 'all', label: t('mode.all')},
            {value: 'streaming', label: t('mode.streaming')},
            {value: 'batch', label: t('mode.batch')},
          ]}
        />
      </div>

      {selected.size > 0 || bulk.result ? (
        <div
          className='flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-1.5 text-xs'
          data-testid='home-job-bulk'
          role='status'
        >
          {selected.size > 0 ? (
            <>
              <span className='font-medium'>
                {t('selected', {count: selected.size})}
              </span>
              <Button
                size='sm'
                variant='outline'
                className='h-7'
                disabled={!!bulk.running}
                onClick={() => setConfirm('stop')}
              >
                {t('stop')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                className='h-7'
                disabled={!!bulk.running}
                onClick={() => setConfirm('recover')}
              >
                {t('recover')}
              </Button>
              <Button
                size='sm'
                variant='ghost'
                className='h-7'
                onClick={() => setSelected(new Set())}
              >
                {t('clearSelection')}
              </Button>
            </>
          ) : null}
          {bulk.result ? (
            <span className='text-muted-foreground'>
              {t('bulk.result', {
                ok: bulk.result.ok,
                failed: bulk.result.failures.length,
                skipped: bulk.result.skipped,
              })}
              {bulk.result.failures.slice(0, 3).map((item) => (
                <span key={item.id} className='ml-2 text-destructive'>
                  {t('bulk.itemFailed', {id: item.id, message: item.message})}
                </span>
              ))}
            </span>
          ) : null}
        </div>
      ) : null}

      <div className='relative flex min-h-[480px] flex-col overflow-hidden rounded-lg border border-border/70 bg-card/40 sm:min-h-[calc(100vh-420px)]'>
        <TableLoadingBar loading={list.refreshing} />
        <StudioTable
          label={t('columns.job')}
          busy={list.refreshing}
          className='flex-1'
        >
          <StudioTableScroll>
            <StudioTableContent minWidth={1080}>
              <StudioTableHeader>
                <StudioTableColumn style={{minWidth: 0, width: 36}}>
                  <Checkbox
                    checked={allOnPageSelected}
                    aria-label={t('selected', {count: items.length})}
                    onCheckedChange={(checked) =>
                      setSelected(
                        checked
                          ? new Set(items.map((job) => job.id))
                          : new Set(),
                      )
                    }
                  />
                </StudioTableColumn>
                <StudioTableColumn style={{minWidth: 220}}>
                  {t('columns.job')}
                </StudioTableColumn>
                <StudioTableColumn>{t('columns.cluster')}</StudioTableColumn>
                <StudioTableColumn style={{minWidth: 0}}>
                  {t('columns.mode')}
                </StudioTableColumn>
                <StudioTableColumn
                  allowsSorting
                  sortDirection={sort === 'attention' ? 'desc' : undefined}
                >
                  {sortHeader('attention', t('columns.status'))}
                </StudioTableColumn>
                <StudioTableColumn
                  allowsSorting={runningOnly}
                  sortDirection={sort === 'rate' ? order : undefined}
                  title={runningOnly ? undefined : t('sortNeedsRunning')}
                  style={{minWidth: 180}}
                >
                  {runningOnly
                    ? sortHeader('rate', t('columns.writeRate'))
                    : t('columns.writeRate')}
                </StudioTableColumn>
                <StudioTableColumn align='end'>
                  {t('columns.total')}
                </StudioTableColumn>
                <StudioTableColumn
                  align='end'
                  allowsSorting={runningOnly}
                  sortDirection={sort === 'cdc_delay' ? order : undefined}
                  title={runningOnly ? undefined : t('sortNeedsRunning')}
                >
                  {runningOnly
                    ? sortHeader('cdc_delay', t('columns.cdc'))
                    : t('columns.cdc')}
                </StudioTableColumn>
                <StudioTableColumn
                  allowsSorting
                  sortDirection={sort === 'started_at' ? order : undefined}
                >
                  {sortHeader('started_at', t('columns.started'))}
                </StudioTableColumn>
                <StudioTableColumn align='end' style={{minWidth: 0}}>
                  {t('columns.actions')}
                </StudioTableColumn>
              </StudioTableHeader>
              <StudioTableBody dimmed={list.refreshing}>
                {list.loading && !list.data ? (
                  Array.from({length: 8}).map((_, index) => (
                    <StudioTableRow key={index}>
                      <StudioTableCell colSpan={COLS}>
                        <div className='h-8 animate-pulse rounded bg-muted/40' />
                      </StudioTableCell>
                    </StudioTableRow>
                  ))
                ) : list.error && !list.data ? (
                  <StudioTableEmptyRow colSpan={COLS}>
                    <span className='text-destructive'>{t('loadError')}</span>
                  </StudioTableEmptyRow>
                ) : items.length === 0 ? (
                  <StudioTableEmptyRow colSpan={COLS}>
                    <EmptyState
                      title={t('empty')}
                      description={t('emptyHint')}
                    />
                  </StudioTableEmptyRow>
                ) : (
                  items.map((job) => {
                    const displayStatus = getDisplayJobLifecycleStatus(job);
                    const name = jobDisplayName(job);
                    const live = job.live;
                    const points = sparklines?.items[String(job.id)];
                    const startedMs = Date.parse(
                      job.started_at || job.created_at,
                    );
                    const endMs = job.finished_at
                      ? Date.parse(job.finished_at)
                      : (list.updatedAt ?? startedMs);
                    const duration =
                      Number.isFinite(startedMs) && endMs > startedMs
                        ? (endMs - startedMs) / 1000
                        : 0;
                    return (
                      <StudioTableRow
                        key={job.id}
                        selected={selected.has(job.id)}
                        className='cursor-pointer'
                        onClick={() => setPreview(job)}
                        data-testid={`home-job-row-${job.id}`}
                      >
                        <StudioTableCell
                          onClick={(event) => event.stopPropagation()}
                        >
                          <Checkbox
                            checked={selected.has(job.id)}
                            aria-label={name}
                            onCheckedChange={(checked) =>
                              setSelected((prev) => {
                                const next = new Set(prev);
                                if (checked) {
                                  next.add(job.id);
                                } else {
                                  next.delete(job.id);
                                }
                                return next;
                              })
                            }
                          />
                        </StudioTableCell>
                        <StudioTableCell>
                          <div className='flex min-w-0 max-w-[280px] flex-col'>
                            <span
                              className='truncate font-medium text-foreground'
                              title={name}
                            >
                              {name}
                            </span>
                            <span className='truncate font-mono text-[11px] text-muted-foreground'>
                              {t('taskRef', {id: job.task_id})}
                              {job.engine_job_id
                                ? ` · ${job.engine_job_id}`
                                : ''}
                            </span>
                          </div>
                        </StudioTableCell>
                        <StudioTableCell className='text-muted-foreground'>
                          {job.cluster_id
                            ? (clusterNames.get(job.cluster_id) ??
                              `#${job.cluster_id}`)
                            : '-'}
                        </StudioTableCell>
                        <StudioTableCell>
                          {job.mode ? (
                            <Badge
                              variant='outline'
                              className='rounded-sm px-1.5 py-0 text-[11px]'
                            >
                              {t(`mode.${job.mode}`)}
                            </Badge>
                          ) : (
                            '-'
                          )}
                        </StudioTableCell>
                        <StudioTableCell>
                          <Badge
                            variant='outline'
                            className={cn(
                              'rounded-sm px-2 py-0 text-[11px] leading-tight',
                              getJobStatusBadgeClass(displayStatus),
                            )}
                          >
                            {getJobStatusLabel(displayStatus)}
                          </Badge>
                        </StudioTableCell>
                        <StudioTableCell>
                          <div className='flex items-center gap-2'>
                            {showSparklines ? (
                              <div className='w-24 shrink-0'>
                                {points && points.length > 1 ? (
                                  <Sparkline
                                    data={points.map((point) => point[1])}
                                    label={t('sparklineLabel', {name})}
                                    height={22}
                                    variant='area'
                                    interactive={false}
                                    color='var(--series-2)'
                                  />
                                ) : null}
                              </div>
                            ) : null}
                            <span className='tabular-nums'>
                              {live?.write_qps === undefined
                                ? '-'
                                : `${formatCompactNumber(live.write_qps)}/s`}
                            </span>
                          </div>
                        </StudioTableCell>
                        <StudioTableCell align='end' className='tabular-nums'>
                          {formatCompactNumber(live?.write_total)}
                        </StudioTableCell>
                        <StudioTableCell align='end' className='tabular-nums'>
                          {live?.cdc_delay_seconds === undefined
                            ? '-'
                            : `${format.number(live.cdc_delay_seconds, {maximumFractionDigits: 1})}s`}
                        </StudioTableCell>
                        <StudioTableCell>
                          <div className='flex flex-col whitespace-nowrap'>
                            <span className='tabular-nums text-foreground'>
                              {Number.isFinite(startedMs)
                                ? formatDateTimeMinuteTz(startedMs)
                                : '-'}
                            </span>
                            {duration > 0 ? (
                              <span className='text-[11px] text-muted-foreground'>
                                {isRunningStatus(job.status)
                                  ? t('ranFor', {
                                      duration: formatDurationSeconds(
                                        duration,
                                        durationUnits,
                                      ),
                                    })
                                  : t('tookFor', {
                                      duration: formatDurationSeconds(
                                        duration,
                                        durationUnits,
                                      ),
                                    })}
                              </span>
                            ) : null}
                          </div>
                        </StudioTableCell>
                        <StudioTableCell
                          align='end'
                          onClick={(event) => event.stopPropagation()}
                        >
                          <Button
                            asChild
                            variant='ghost'
                            size='sm'
                            className='h-7 px-2 text-xs'
                          >
                            <Link href={jobWorkbenchHref(job)}>
                              {t('detail')}
                            </Link>
                          </Button>
                        </StudioTableCell>
                      </StudioTableRow>
                    );
                  })
                )}
              </StudioTableBody>
            </StudioTableContent>
          </StudioTableScroll>
          <StudioTableFooter className='mt-auto'>
            <span>{t('total', {count: list.data?.total ?? 0})}</span>
            {totalPages > 1 ? (
              <Pagination
                currentPage={query.page}
                totalPages={totalPages}
                pageSize={PAGE_SIZE}
                totalItems={list.data?.total ?? 0}
                onPageChange={(page) => update({page})}
                showPageSizeSelector={false}
                showTotalItems={false}
              />
            ) : null}
          </StudioTableFooter>
        </StudioTable>
      </div>

      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => !open && setConfirm(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirm === 'recover'
                ? t('bulk.recoverTitle', {count: selected.size})
                : t('bulk.stopTitle', {count: selected.size})}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirm === 'recover'
                ? t('bulk.recoverDesc')
                : t('bulk.stopDesc')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('bulk.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                const action = confirm;
                setConfirm(null);
                if (action) {
                  void bulk
                    .run(action, selectedJobs)
                    .then(() => setSelected(new Set()));
                }
              }}
            >
              {t('bulk.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <JobPreviewDrawer
        job={preview}
        clusterName={
          preview?.cluster_id ? clusterNames.get(preview.cluster_id) : undefined
        }
        onOpenChange={(open) => !open && setPreview(null)}
      />
    </div>
  );
}
