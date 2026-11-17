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

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Search} from 'lucide-react';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  StatPillsBar,
  type StatPillItem,
} from '@/components/common/layout';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {
  SyncJobStatusGroup,
} from '@/lib/services/sync';
import {JOB_STATUS_GROUPS, type JobListQuery, type JobTablePreset} from './types';
import type {JobBulkAction, JobBulkResult} from './use-job-bulk-actions';

const PILL_VARIANT: Partial<
  Record<SyncJobStatusGroup, StatPillItem['variant']>
> = {
  failed: 'danger',
  running: 'info',
  pending: 'warning',
  finished: 'success',
};

export interface JobTableToolbarProps {
  preset: JobTablePreset;
  query: JobListQuery;
  /** patch 语义；筛选变化时由本组件负责把 page 重置为 1 / Patch semantics; this component resets page on filter changes. */
  onQueryChange: (patch: Partial<JobListQuery>) => void;
  statusCounts: Partial<Record<SyncJobStatusGroup, number>>;
  /** 不传 = 没有批量条 / Omit for no bulk bar. */
  bulk?: {
    selectedCount: number;
    running: JobBulkAction | null;
    result: JobBulkResult | null;
    onStop: () => void;
    onRecover: () => void;
    onClear: () => void;
  };
  testIdPrefix: string;
}

/**
 * 作业表工具栏：状态胶囊 + 关键字 + 模式（global）/ 运行类型（task）筛选 + 批量操作条。
 * 所有筛选变化都附带 page: 1（不依赖宿主）。task 预设的关键字默认收进图标按钮，点击展开。
 * Job table toolbar: status pills, keyword, mode (global) / run type (task) filters and the bulk action bar.
 * Every filter change carries page: 1 (not left to the host). The task preset collapses keyword search behind
 * an icon button that expands on click.
 */
export function JobTableToolbar({
  preset,
  query,
  onQueryChange,
  statusCounts,
  bulk,
  testIdPrefix,
}: JobTableToolbarProps) {
  const t = useTranslations('jobTable');
  const [keyword, setKeyword] = useState(query.keyword);
  // task 预设的搜索框展开状态；外部关键字（如深链）存在时保持展开。
  // Expand state of the task preset's collapsed search; stays open while a keyword exists.
  const [searchOpen, setSearchOpen] = useState(Boolean(query.keyword));
  useEffect(() => setKeyword(query.keyword), [query.keyword]);
  useEffect(() => {
    if (query.keyword) {
      setSearchOpen(true);
    }
  }, [query.keyword]);

  const counts = statusCounts;
  const totalAll = JOB_STATUS_GROUPS.reduce(
    (sum, group) => sum + (counts[group] ?? 0),
    0,
  );
  const pills: StatPillItem[] = [
    {
      key: 'all',
      label: t('groups.all'),
      count: totalAll,
      dataTestId: `${testIdPrefix}-pill-all`,
    },
    ...JOB_STATUS_GROUPS.map((group) => ({
      key: group,
      label: t(`groups.${group}`),
      count: counts[group] ?? 0,
      variant: PILL_VARIANT[group],
      pulse: group === 'failed' && (counts.failed ?? 0) > 0,
      dataTestId: `${testIdPrefix}-pill-${group}`,
    })),
  ];

  const searchForm = (
    <form
      className='relative min-w-[200px] max-w-sm flex-1'
      onSubmit={(event) => {
        event.preventDefault();
        onQueryChange({keyword: keyword.trim(), page: 1});
      }}
    >
      <Search className='pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground' />
      <Input
        value={keyword}
        onChange={(event) => setKeyword(event.target.value)}
        placeholder={t('searchPlaceholder')}
        aria-label={t('searchPlaceholder')}
        className='h-8 pl-8 text-xs'
        data-testid={`${testIdPrefix}-search`}
        autoFocus={preset === 'task' && searchOpen}
      />
    </form>
  );

  return (
    <>
      <StatPillsBar
        items={pills}
        activeKey={query.statusGroup || 'all'}
        onChange={(key) =>
          onQueryChange({
            statusGroup:
              key === 'all' ? '' : (key as SyncJobStatusGroup),
            page: 1,
          })
        }
      />

      <div className='flex flex-wrap items-center gap-2'>
        {preset === 'task' && !searchOpen ? (
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 w-8 px-0'
            aria-label={t('searchPlaceholder')}
            data-testid={`${testIdPrefix}-search-toggle`}
            onClick={() => setSearchOpen(true)}
          >
            <Search className='size-3.5' />
          </Button>
        ) : (
          searchForm
        )}
        {preset === 'global' ? (
          <SegmentedControl
            label={t('columns.mode')}
            value={query.mode || 'all'}
            onValueChange={(value) =>
              onQueryChange({
                mode:
                  value === 'streaming' || value === 'batch' ? value : '',
                page: 1,
              })
            }
            options={[
              {value: 'all', label: t('mode.all')},
              {value: 'streaming', label: t('mode.streaming')},
              {value: 'batch', label: t('mode.batch')},
            ]}
          />
        ) : (
          <SegmentedControl
            label={t('runTypeFilter.label')}
            value={query.runType}
            onValueChange={(value) =>
              onQueryChange({
                runType: value as JobListQuery['runType'],
                page: 1,
              })
            }
            options={[
              {value: 'all', label: t('runTypeFilter.all')},
              {value: 'formal', label: t('runTypeFilter.formal')},
              {value: 'preview', label: t('runTypeFilter.preview')},
            ]}
          />
        )}
      </div>

      {bulk && (bulk.selectedCount > 0 || bulk.result) ? (
        <div
          className='flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-1.5 text-xs'
          data-testid={`${testIdPrefix}-bulk`}
          role='status'
        >
          {bulk.selectedCount > 0 ? (
            <>
              <span className='font-medium'>
                {t('selected', {count: bulk.selectedCount})}
              </span>
              <Button
                size='sm'
                variant='outline'
                className='h-7'
                disabled={Boolean(bulk.running)}
                onClick={bulk.onStop}
              >
                {t('stop')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                className='h-7'
                disabled={Boolean(bulk.running)}
                onClick={bulk.onRecover}
              >
                {t('recover')}
              </Button>
              <Button
                size='sm'
                variant='ghost'
                className='h-7'
                onClick={bulk.onClear}
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
    </>
  );
}
