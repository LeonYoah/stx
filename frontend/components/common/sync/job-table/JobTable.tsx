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

import {useMemo} from 'react';
import {useFormatter, useTranslations} from 'next-intl';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {TableLoadingBar} from '@/components/common/layout';
import {Pagination} from '@/components/ui/pagination';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {
  SyncJobInstance,
  SyncJobListSort,
  SyncJobSparklines,
} from '@/lib/services/sync';
import {cn} from '@/lib/utils';
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
} from '../StudioDataTable';
import type {JobDetailTab} from '../job-detail/types';
import {JOB_COLUMNS} from './job-table-columns';
import type {
  JobActorContext,
  JobListQuery,
  JobRowAction,
  JobTableContext,
  JobTablePreset,
} from './types';
import type {UseJobListResult} from './use-job-list';

/** 首屏骨架行数 / Skeleton row count on the first load. */
const SKELETON_ROWS = 8;

export interface JobTableProps {
  preset: JobTablePreset;
  data: Pick<
    UseJobListResult,
    'items' | 'total' | 'loading' | 'refreshing' | 'error' | 'updatedAt'
  >;
  query: JobListQuery;
  /** 排序、翻页都走这里（patch 语义）/ Sorting and paging both go through this patch callback. */
  onQueryChange: (patch: Partial<JobListQuery>) => void;
  pageSize: number;
  sparklines?: SyncJobSparklines | null;
  clusterNames?: Map<number, string>;
  /** 大盘逐行构造，工作台固定一份 / Per-row factory on the dashboard, a fixed value in the workbench. */
  actor: JobActorContext | ((job: SyncJobInstance) => JobActorContext);
  initiatorUsers?: NotificationRecipientUser[];
  currentUserId?: number;
  currentUsername?: string;
  /** 不传 = 没有多选列 / Omit for no checkbox column. */
  selection?: {ids: Set<number>; onChange: (ids: Set<number>) => void};
  /** 批量动作条（通常 BatchBar）：绝对定位挂在表格外层 relative 容器内，
      垂直位置由 BatchBar 的 placement 决定。
      / Bulk action bar (usually BatchBar): absolutely positioned inside the
      outer relative container; vertical placement comes from BatchBar's prop. */
  bulkBar?: React.ReactNode;
  /** 工作台当前选中作业（行高亮）/ Currently selected workbench job (row highlight). */
  activeJobId?: number | null;
  onRowActivate: (job: SyncJobInstance) => void;
  onOpenDetail: (job: SyncJobInstance, tab?: JobDetailTab) => void;
  onAction: (action: JobRowAction, job: SyncJobInstance) => void;
  /** 'home-job' | 'studio-job' / testid prefix per host. */
  testIdPrefix: string;
  className?: string;
}

/**
 * 统一作业表（大盘 global / 工作台 task）：纯受控呈现层 —— 不发请求、不读 URL、不持有业务状态；
 * 列全部来自 job-table-columns 注册表，本组件不按列 id 写特殊分支（复选框列除外）。
 * The unified job table (dashboard global / workbench task presets): a purely controlled presentation layer —
 * no requests, no URL access, no business state. All columns come from the job-table-columns registry; this
 * component carries no column-id-specific branches (except the checkbox column).
 */
export function JobTable({
  preset,
  data,
  query,
  onQueryChange,
  pageSize,
  sparklines = null,
  clusterNames,
  actor,
  initiatorUsers = [],
  currentUserId,
  currentUsername,
  selection,
  bulkBar,
  activeJobId,
  onRowActivate,
  onOpenDetail,
  onAction,
  testIdPrefix,
  className,
}: JobTableProps) {
  const t = useTranslations('jobTable');
  const format = useFormatter();
  const items = data.items;

  // 排序切换：点击当前降序的列变升序，其余一律降序；同时页码回到 1。
  // Sort toggle: clicking the column currently sorted descending flips to ascending; everything else sorts
  // descending. The page always resets to 1.
  const toggleSort = (key: SyncJobListSort) => {
    const nextOrder =
      query.sort === key && query.order === 'desc' ? 'asc' : 'desc';
    onQueryChange({sort: key, order: nextOrder, page: 1});
  };

  const ctx = useMemo<JobTableContext>(
    () => ({
      preset,
      query,
      items,
      updatedAt: data.updatedAt,
      sparklines,
      clusterNames,
      actorOf: typeof actor === 'function' ? actor : () => actor,
      initiatorUsers,
      currentUserId,
      currentUsername,
      selection,
      t,
      format,
      onRowActivate,
      onOpenDetail,
      onAction,
      testIdPrefix,
    }),
    [
      preset,
      query,
      items,
      data.updatedAt,
      sparklines,
      clusterNames,
      actor,
      initiatorUsers,
      currentUserId,
      currentUsername,
      selection,
      t,
      format,
      onRowActivate,
      onOpenDetail,
      onAction,
      testIdPrefix,
    ],
  );

  const columns = useMemo(
    () =>
      JOB_COLUMNS.filter(
        (column) =>
          column.presets.includes(preset) &&
          (column.visibleWhen?.(items, ctx) ?? true),
      ),
    [preset, items, ctx],
  );
  const colSpan = Math.max(1, columns.length);
  const totalPages = Math.max(1, Math.ceil(data.total / pageSize));

  return (
    <div
      className={cn(
        'relative flex flex-col overflow-hidden',
        preset === 'task'
          ? 'min-h-0 flex-1'
          : 'min-h-[480px] rounded-lg border border-border/70 bg-card/40 sm:min-h-[calc(100vh-420px)]',
        className,
      )}
    >
      <TableLoadingBar loading={data.refreshing} />
      <StudioTable
        label={t('columns.job')}
        busy={data.refreshing}
        className='flex-1'
      >
        <StudioTableScroll>
          <StudioTableContent minWidth={preset === 'task' ? 1000 : 1180}>
            <StudioTableHeader>
              {columns.map((column) => {
                const headerNode = column.header(ctx);
                const sortable = Boolean(
                  column.sortKey &&
                    (column.sortableWhen?.(query) ?? true),
                );
                return (
                  <StudioTableColumn
                    key={column.id}
                    align={column.align}
                    allowsSorting={sortable}
                    sortDirection={
                      sortable && query.sort === column.sortKey
                        ? query.order
                        : undefined
                    }
                    title={
                      !sortable && column.sortKey
                        ? t('sortNeedsRunning')
                        : undefined
                    }
                    style={{
                      minWidth: column.minWidth ?? 0,
                      width: column.width,
                    }}
                  >
                    {sortable &&
                    typeof headerNode === 'string' &&
                    column.sortKey ? (
                      <StudioSortableColumnHeader
                        label={headerNode}
                        sortDirection={
                          query.sort === column.sortKey
                            ? query.order
                            : undefined
                        }
                        aria-label={t('sortColumn', {column: headerNode})}
                        onSort={() => toggleSort(column.sortKey!)}
                      />
                    ) : (
                      headerNode
                    )}
                  </StudioTableColumn>
                );
              })}
            </StudioTableHeader>
            <StudioTableBody dimmed={data.refreshing}>
              {data.loading ? (
                Array.from({length: SKELETON_ROWS}).map((_, index) => (
                  <StudioTableRow key={index}>
                    <StudioTableCell colSpan={colSpan}>
                      <div className='h-8 animate-pulse rounded bg-muted/40' />
                    </StudioTableCell>
                  </StudioTableRow>
                ))
              ) : data.error && items.length === 0 ? (
                <StudioTableEmptyRow colSpan={colSpan}>
                  <span className='text-destructive'>{t('loadError')}</span>
                </StudioTableEmptyRow>
              ) : items.length === 0 ? (
                <StudioTableEmptyRow colSpan={colSpan}>
                  <EmptyState title={t('empty')} description={t('emptyHint')} />
                </StudioTableEmptyRow>
              ) : (
                items.map((job) => (
                  <StudioTableRow
                    key={job.id}
                    selected={
                      selection
                        ? selection.ids.has(job.id)
                        : job.id === activeJobId
                    }
                    className='cursor-pointer'
                    onClick={() => onRowActivate(job)}
                    data-testid={`${testIdPrefix}-row-${job.id}`}
                  >
                    {columns.map((column) => (
                      <StudioTableCell key={column.id} align={column.align}>
                        {column.cell(job, ctx)}
                      </StudioTableCell>
                    ))}
                  </StudioTableRow>
                ))
              )}
            </StudioTableBody>
          </StudioTableContent>
        </StudioTableScroll>
        <StudioTableFooter className='mt-auto'>
          <span>{t('total', {count: data.total})}</span>
          {totalPages > 1 ? (
            <Pagination
              currentPage={query.page}
              totalPages={totalPages}
              pageSize={pageSize}
              totalItems={data.total}
              onPageChange={(page) => onQueryChange({page})}
              showPageSizeSelector={false}
              showTotalItems={false}
            />
          ) : null}
        </StudioTableFooter>
      </StudioTable>
      {bulkBar}
    </div>
  );
}
