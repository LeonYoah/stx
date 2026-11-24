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
import {ArrowDownWideNarrow, Check, Search} from 'lucide-react';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {StatPillsBar, type StatPillItem} from '@/components/common/layout';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {
  SyncJobStatusGroup,
} from '@/lib/services/sync';
import {TIME_SORT_KEYS} from './job-table-columns';
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
      className={
        preset === 'task'
          ? 'relative w-36 shrink-0 sm:w-44'
          : 'relative min-w-[160px] max-w-sm flex-1'
      }
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
      <>
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
        <SegmentedControl
          label={t('source.label')}
          value={query.source || 'all'}
          onValueChange={(value) =>
            onQueryChange({
              source: value === 'stx' || value === 'external' ? value : '',
              page: 1,
            })
          }
          options={[
            {value: 'all', label: t('source.all')},
            {value: 'stx', label: t('source.stx')},
            {value: 'external', label: t('source.external')},
          ]}
        />
      </>
    ) : // task 预设不再按运行类型筛分：预览与正式运行同列展示，工具栏只留状态 pills 和搜索。
    // The task preset no longer filters by run type — preview and formal runs share
    // one list, and the toolbar keeps only status pills and search.
    null;

  // 排序菜单只收时间维度：字段×方向的 11 项铺满菜单过于重复，方向留给时间列头
  // 点按切换（列头指示在任一维度命中时点亮）。其余列仍走各自列头排序。
  // The sort menu holds only the time dimensions — 11 field×direction rows repeated the
  // same suffixes; direction stays on the time header click (its indicator lights on any
  // of them). Other columns keep their own header sorting.
  const sortKeyLabels: Record<(typeof TIME_SORT_KEYS)[number], string> = {
    started_at: t('startedAt'),
    finished_at: t('finishedAt'),
    duration: t('duration'),
  };
  const sortMenu = (
    <DropdownMenu
      label={t('sortMenu.label')}
      icon={<ArrowDownWideNarrow className='size-4' aria-hidden />}
      // 与监控规则面板同款：当前项 ✓，其余项用等宽占位对齐。
      // Same pattern as the monitoring rules panel: a check on the active option
      // and equal-width spacers keep labels aligned.
      items={TIME_SORT_KEYS.map((key) => ({
        label: sortKeyLabels[key],
        icon:
          query.sort === key ? (
            <Check className='size-4' aria-hidden />
          ) : (
            <span className='size-4' />
          ),
        onSelect: () => onQueryChange({sort: key, order: query.order, page: 1}),
      }))}
    />
  );

  // task 预设：单行不换行——左侧状态胶囊横向滚动，右侧搜索与排序按钮固定单行不裁切。
  // Task preset: single non-wrapping row — status pills scroll horizontally on the left,
  // while search and sort controls stay on one line on the right without vertical clipping.
  if (preset === 'task') {
    return (
      <StatPillsBar
        items={pills}
        activeKey={query.statusGroup || 'all'}
        onChange={handleStatusGroup}
        label={t('columns.status')}
        singleRow
        className='min-h-10 shrink-0 gap-2 border-b border-border/60 px-2 py-1'
        actions={
          <>
            {typeControl}
            {searchControl}
            {sortMenu}
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
        {sortMenu}
      </div>
    </>
  );
}
