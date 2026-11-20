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
  testIdPrefix: string;
}

/**
 * 作业表工具栏：状态胶囊 + 关键字 + 模式（global）/ 运行类型（task）筛选。
 * 所有筛选变化都附带 page: 1（不依赖宿主）。
 * global 预设是两行布局（胶囊一行、筛选一行）；task 预设压缩为单行 ≤36px——
 * 运行类型与可展开搜索收进胶囊栏右侧的 actions 槽，溢出横向滚动而不是换行。
 * Job table toolbar: status pills, keyword, mode (global) / run type (task) filters.
 * Every filter change carries page: 1 (not left to the host).
 * The global preset uses two rows (pills, then filters); the task preset compresses everything into a single
 * ≤36px row — run type and the expandable search sit in the pills bar's actions slot, overflowing into
 * horizontal scroll instead of wrapping.
 */
export function JobTableToolbar({
  preset,
  query,
  onQueryChange,
  statusCounts,
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

  const handleStatusGroup = (key: string) =>
    onQueryChange({
      statusGroup: key === 'all' ? '' : (key as SyncJobStatusGroup),
      page: 1,
    });

  const searchForm = (
    <form
      className='relative min-w-[160px] max-w-sm flex-1'
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

  // task 预设默认只放一个搜索图标，点击展开为输入框；global 预设常驻输入框。
  // The task preset shows only a search icon until clicked; the global preset keeps the input always visible.
  const searchControl =
    preset === 'task' && !searchOpen ? (
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
    );

  const typeControl =
    preset === 'global' ? (
      <SegmentedControl
        label={t('columns.mode')}
        value={query.mode || 'all'}
        onValueChange={(value) =>
          onQueryChange({
            mode: value === 'streaming' || value === 'batch' ? value : '',
            page: 1,
          })
        }
        options={[
          {value: 'all', label: t('mode.all')},
          {value: 'streaming', label: t('mode.streaming')},
          {value: 'batch', label: t('mode.batch')},
        ]}
      />
    ) : // task 预设不再按运行类型筛分：预览与正式运行同列展示，工具栏只留状态 pills 和搜索。
        // The task preset no longer filters by run type — preview and formal runs share
        // one list, and the toolbar keeps only status pills and search.
        null;

  // task 预设：单行 ≤36px——状态胶囊（带计数）+ 运行类型 + 可展开搜索；溢出横向滚动，不换行。
  // Task preset: a single ≤36px row — status pills (with counts) + run type + expandable search; overflow
  // scrolls horizontally instead of wrapping.
  if (preset === 'task') {
    return (
      <StatPillsBar
        items={pills}
        activeKey={query.statusGroup || 'all'}
        onChange={handleStatusGroup}
        className='h-9 flex-nowrap gap-2 overflow-x-auto overflow-y-hidden border-b border-border/60 px-2 [&>div]:shrink-0 [&>div]:flex-nowrap'
        actions={
          <>
            {typeControl}
            {searchControl}
          </>
        }
      />
    );
  }

  return (
    <>
      <StatPillsBar
        items={pills}
        activeKey={query.statusGroup || 'all'}
        onChange={handleStatusGroup}
      />

      <div className='flex flex-wrap items-center gap-2'>
        {searchControl}
        {typeControl}
      </div>
    </>
  );
}
