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

// 工作台 task 预设（自 JobRunsPanel.test 迁移）：点行选中并打开详情；行操作详情 / 恢复 / Savepoint 停止 /
// 停止全部走宿主 onAction；工具栏单行（状态胶囊 + 运行类型 + 可展开搜索）；排序 / 翻页 / 筛选走
// onQueryChange patch；操作可用性与详情共用 getJobActionState。
// Workbench task preset (migrated from JobRunsPanel.test): clicking a row selects the job and opens its
// detail; row actions — details / recover / savepoint stop / stop — all go through the host onAction; the
// toolbar stays a single row (status pills + run type + expandable search); sort / paging / filters patch
// the query via onQueryChange; action availability shares getJobActionState with the detail.

import {fireEvent, render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {SyncJobInstance} from '@/lib/services/sync';
import type {JobDetailTab} from '../../job-detail/types';
import {getJobActionState} from '../../sync-studio-utils';
import {JobTable} from '../JobTable';
import {JobTableToolbar} from '../JobTableToolbar';
import {
  DEFAULT_JOB_LIST_QUERY,
  type JobActorContext,
  type JobListQuery,
  type JobRowAction,
} from '../types';

// jsdom 缺少 matchMedia（Arc SegmentedControl / StudioTable 会用到）。
// jsdom lacks matchMedia (needed by the Arc SegmentedControl / StudioTable).
if (typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

vi.mock('next/link', () => ({
  default: ({href, children}: {href: string; children: React.ReactNode}) => (
    <a href={href}>{children}</a>
  ),
}));
vi.mock('@/components/charts/sparkline', () => ({
  Sparkline: () => <span data-testid='sparkline' />,
}));

const tJobTable = zh.jobTable;

function job(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    id: 26,
    task_id: 11,
    task_version: 3,
    run_type: 'run',
    platform_job_id: '179312051430100000',
    engine_job_id: '179312051430100000',
    status: 'running',
    submit_spec: {cluster_id: 1},
    result_preview: {},
    error_message: '',
    created_by: 1,
    created_at: '2026-11-14T10:00:00Z',
    updated_at: '2026-11-14T10:00:00Z',
    started_at: '2026-11-14T10:00:00Z',
    ...partial,
  } as SyncJobInstance;
}

const DEFAULT_ACTOR: JobActorContext = {
  isAdmin: false,
  isOwner: true,
  canRun: true,
  currentUserId: 1,
  disableRecover: false,
};

function renderTaskTable(
  items: SyncJobInstance[],
  overrides: {
    query?: Partial<JobListQuery>;
    actor?: JobActorContext;
    activeJobId?: number | null;
    pageSize?: number;
    total?: number;
    onQueryChange?: (patch: Partial<JobListQuery>) => void;
    onRowActivate?: (job: SyncJobInstance) => void;
    onOpenDetail?: (job: SyncJobInstance, tab?: JobDetailTab) => void;
    onAction?: (action: JobRowAction, job: SyncJobInstance) => void;
  } = {},
) {
  const handlers = {
    onQueryChange:
      overrides.onQueryChange ??
      vi.fn<(patch: Partial<JobListQuery>) => void>(),
    onRowActivate:
      overrides.onRowActivate ?? vi.fn<(job: SyncJobInstance) => void>(),
    onOpenDetail:
      overrides.onOpenDetail ??
      vi.fn<(job: SyncJobInstance, tab?: JobDetailTab) => void>(),
    onAction:
      overrides.onAction ??
      vi.fn<(action: JobRowAction, job: SyncJobInstance) => void>(),
  };
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <JobTable
        preset='task'
        data={{
          items,
          total: overrides.total ?? items.length,
          loading: false,
          refreshing: false,
          error: null,
          updatedAt: null,
        }}
        query={{...DEFAULT_JOB_LIST_QUERY, ...overrides.query}}
        pageSize={overrides.pageSize ?? 20}
        actor={overrides.actor ?? DEFAULT_ACTOR}
        activeJobId={overrides.activeJobId}
        {...handlers}
        testIdPrefix='studio-job'
      />
    </NextIntlClientProvider>,
  );
  return handlers;
}

function renderTaskToolbar(
  overrides: {
    query?: Partial<JobListQuery>;
    statusCounts?: Parameters<typeof JobTableToolbar>[0]['statusCounts'];
    onQueryChange?: (patch: Partial<JobListQuery>) => void;
  } = {},
) {
  const onQueryChange =
    overrides.onQueryChange ?? vi.fn<(patch: Partial<JobListQuery>) => void>();
  const utils = render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <JobTableToolbar
        preset='task'
        query={{...DEFAULT_JOB_LIST_QUERY, ...overrides.query}}
        onQueryChange={onQueryChange}
        statusCounts={overrides.statusCounts ?? {failed: 2, running: 1}}
        testIdPrefix='studio-job'
      />
    </NextIntlClientProvider>,
  );
  return {onQueryChange, ...utils};
}

describe('JobTable task preset', () => {
  it('renders the job column as #id plus the engine job id, without the cluster column', () => {
    renderTaskTable([job({})]);
    const row = screen.getByTestId('studio-job-row-26');
    expect(within(row).getByText('#26')).toBeTruthy();
    expect(within(row).getByText('179312051430100000')).toBeTruthy();
    expect(screen.queryByText(tJobTable.columns.cluster)).toBeNull();
    // task 预设不渲染复选框列（批量不开）。/ No checkbox column on the task preset (bulk stays off).
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
  });

  it('activates the row on click so the host can select the job and open its detail', () => {
    const handlers = renderTaskTable([job({})]);
    fireEvent.click(screen.getByText('#26'));
    expect(handlers.onRowActivate).toHaveBeenCalledWith(
      expect.objectContaining({id: 26}),
    );
  });

  it('marks only the activeJobId row as selected', () => {
    const older = job({id: 9, started_at: '2026-01-01T00:00:00Z'});
    const newer = job({id: 26});
    renderTaskTable([newer, older], {activeJobId: 9});
    expect(screen.getByTestId('studio-job-row-9').dataset.selected).toBe(
      'true',
    );
    expect(
      screen.getByTestId('studio-job-row-26').dataset.selected,
    ).toBeUndefined();
  });

  it('keeps only details and quick actions inline for a running job; stop does not bubble', () => {
    const handlers = renderTaskTable([job({})]);
    expect(screen.getByRole('button', {name: tJobTable.detail})).toBeTruthy();
    expect(
      screen.getByRole('button', {name: tJobTable.moreActions}),
    ).toBeTruthy();
    // 运行中的作业：行内只有详情 + 停止 + 更多菜单（Savepoint 停止收进菜单）。
    // A running job keeps Details + Stop + the more menu inline (savepoint stop lives in the menu).
    expect(
      screen.queryByRole('button', {name: tJobTable.savepointStop}),
    ).toBeNull();
    expect(screen.queryByRole('button', {name: tJobTable.recover})).toBeNull();

    fireEvent.click(screen.getByRole('button', {name: tJobTable.stop}));
    expect(handlers.onAction).toHaveBeenCalledWith(
      'stop',
      expect.objectContaining({id: 26}),
    );
    // 行内按钮不能冒泡成行激活 / 详情。/ Inline buttons must not bubble into row activation or the detail.
    expect(handlers.onRowActivate).not.toHaveBeenCalled();
    expect(handlers.onOpenDetail).not.toHaveBeenCalled();
  });

  it('opens the detail from the inline detail button with the default tab', () => {
    const handlers = renderTaskTable([job({})]);
    fireEvent.click(screen.getByRole('button', {name: tJobTable.detail}));
    expect(handlers.onOpenDetail).toHaveBeenCalledWith(
      expect.objectContaining({id: 26}),
    );
    expect(handlers.onRowActivate).not.toHaveBeenCalled();
  });

  it('offers savepoint stop and copy job id from the more menu', async () => {
    const handlers = renderTaskTable([job({})]);
    const trigger = screen.getByRole('button', {name: tJobTable.moreActions});
    fireEvent.keyDown(trigger, {key: 'ArrowDown'});
    const savepoint = await screen.findByRole('menuitem', {
      name: tJobTable.savepointStop,
    });
    expect(
      screen.getByRole('menuitem', {name: tJobTable.copyJobId}),
    ).toBeTruthy();
    // 「在工作台打开」只在 global 预设出现。/ "Open in workbench" belongs to the global preset only.
    expect(
      screen.queryByRole('menuitem', {name: tJobTable.openInWorkbench}),
    ).toBeNull();

    fireEvent.click(savepoint);
    expect(handlers.onAction).toHaveBeenCalledWith(
      'savepointStop',
      expect.objectContaining({id: 26}),
    );
    expect(handlers.onRowActivate).not.toHaveBeenCalled();
  });

  it('shows a recover action on terminal jobs and wires it to the host', () => {
    const handlers = renderTaskTable([job({status: 'failed'})]);
    expect(screen.queryByRole('button', {name: tJobTable.stop})).toBeNull();
    fireEvent.click(screen.getByRole('button', {name: tJobTable.recover}));
    expect(handlers.onAction).toHaveBeenCalledWith(
      'recover',
      expect.objectContaining({id: 26}),
    );
  });

  it('disables recover when the task already has an active run', () => {
    renderTaskTable([job({status: 'failed'})], {
      actor: {...DEFAULT_ACTOR, disableRecover: true},
    });
    const button = screen.getByRole('button', {
      name: tJobTable.recover,
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it('keeps the table chrome when there are no job runs', () => {
    renderTaskTable([]);
    expect(screen.getByText(tJobTable.empty)).toBeTruthy();
    expect(screen.getByText(tJobTable.emptyHint)).toBeTruthy();
    // 表头与合计仍在，不塌陷。/ Header and total stay rendered — no collapse.
    expect(screen.getByText(tJobTable.columns.job)).toBeTruthy();
    expect(
      screen.getByText(tJobTable.total.replace('{count}', '0')),
    ).toBeTruthy();
  });

  it('patches the query for sorting instead of opening the detail', () => {
    // 默认已按 started_at desc 排序：换一个排序键再点时间列，预期落回 desc。
    // The default is already started_at desc: start from another sort key so the timeline click lands on desc.
    const handlers = renderTaskTable([job({})], {
      query: {sort: 'attention', order: 'desc', page: 3},
    });
    fireEvent.click(
      screen.getByRole('button', {
        name: tJobTable.sortColumn.replace(
          '{column}',
          tJobTable.columns.timeline,
        ),
      }),
    );
    expect(handlers.onQueryChange).toHaveBeenCalledWith({
      sort: 'started_at',
      order: 'desc',
      page: 1,
    });
    expect(handlers.onOpenDetail).not.toHaveBeenCalled();
    expect(handlers.onRowActivate).not.toHaveBeenCalled();
  });

  it('patches the page when paging to the next page', () => {
    const handlers = renderTaskTable(
      Array.from({length: 20}, (_, index) =>
        job({id: index + 1, engine_job_id: `e${index + 1}`}),
      ),
      {total: 45},
    );
    fireEvent.click(screen.getByRole('button', {name: '2'}));
    expect(handlers.onQueryChange).toHaveBeenCalledWith({page: 2});
  });

  it('opens the metrics tab from the multi-table badge without activating the row', () => {
    const handlers = renderTaskTable([
      job({live: {table_count: 2, read_total: 10, write_total: 8}}),
    ]);
    fireEvent.click(screen.getByTestId('studio-job-multi-table-26'));
    expect(handlers.onOpenDetail).toHaveBeenCalledWith(
      expect.objectContaining({id: 26}),
      'metrics',
    );
    expect(handlers.onRowActivate).not.toHaveBeenCalled();
  });
});

describe('JobTableToolbar task preset', () => {
  it('renders status pills with counts and a collapsed search on one row', () => {
    const {container} = renderTaskToolbar();
    expect(screen.getByTestId('studio-job-pill-all').textContent).toContain(
      '3',
    );
    expect(screen.getByTestId('studio-job-pill-failed').textContent).toContain(
      '2',
    );
    // 运行类型筛选已移除：预览与正式运行同列展示，工具栏没有第二个分段控件。
    // The run-type filter is gone — preview and formal runs share one list, so the
    // toolbar shows no second segmented control.
    expect(screen.queryAllByRole('group')).toHaveLength(0);
    // 关键字默认收成图标按钮；整条工具栏单行不换行（溢出横向滚动）。
    // The keyword input collapses to an icon button by default; the toolbar is one non-wrapping row that
    // scrolls horizontally on overflow.
    expect(screen.getByTestId('studio-job-search-toggle')).toBeTruthy();
    expect(screen.queryByTestId('studio-job-search')).toBeNull();
    const bar = container.firstElementChild as HTMLElement;
    expect(bar.className).toContain('flex-nowrap');
    expect(bar.className).toContain('overflow-x-auto');
  });

  it('patches statusGroup and resets the page when a pill is clicked', () => {
    const {onQueryChange} = renderTaskToolbar({query: {page: 4}});
    fireEvent.click(screen.getByTestId('studio-job-pill-failed'));
    expect(onQueryChange).toHaveBeenCalledWith({
      statusGroup: 'failed',
      page: 1,
    });
    fireEvent.click(screen.getByTestId('studio-job-pill-all'));
    expect(onQueryChange).toHaveBeenCalledWith({statusGroup: '', page: 1});
  });

  it('expands the search icon into an input and submits the keyword', () => {
    const {onQueryChange} = renderTaskToolbar();
    fireEvent.click(screen.getByTestId('studio-job-search-toggle'));
    const input = screen.getByTestId('studio-job-search');
    fireEvent.change(input, {target: {value: 'engine-9'}});
    fireEvent.submit(input.closest('form')!);
    expect(onQueryChange).toHaveBeenCalledWith({
      keyword: 'engine-9',
      page: 1,
    });
  });

  it('patches sort and order from the sort menu and resets the page', async () => {
    const {onQueryChange} = renderTaskToolbar({query: {page: 3}});
    // Radix 菜单在 jsdom 用 pointerDown 打开（keyDown 会踩 roving-focus 空指针）。
    // Radix menus open via pointerDown in jsdom (keyDown hits a roving-focus null ref).
    fireEvent.pointerDown(screen.getByTestId('studio-job-sort'));
    fireEvent.click(
      await screen.findByRole('menuitemradio', {
        name: zh.jobTable.sortMenu.duration.desc,
      }),
    );
    expect(onQueryChange).toHaveBeenCalledWith({
      sort: 'duration',
      order: 'desc',
      page: 1,
    });
  });

  it('keeps rate sorting listed but disabled outside the running group', async () => {
    renderTaskToolbar({query: {statusGroup: 'finished'}});
    fireEvent.pointerDown(screen.getByTestId('studio-job-sort'));
    const rate = await screen.findByRole('menuitemradio', {
      name: zh.jobTable.sortMenu.rate.desc,
    });
    expect(rate.getAttribute('aria-disabled')).toBe('true');
    const duration = screen.getByRole('menuitemradio', {
      name: zh.jobTable.sortMenu.duration.asc,
    });
    expect(duration.getAttribute('aria-disabled')).toBeNull();
  });
});

describe('getJobActionState', () => {
  const ctx = {
    isAdmin: false,
    isOwner: false,
    canRun: true,
    currentUserId: 1,
    disableRecover: false,
  };

  it('lets the initiator stop an active job but not recover it', () => {
    expect(getJobActionState(job({}), ctx)).toEqual({
      showRecover: true,
      canRecover: false,
      showStop: true,
      canStop: true,
    });
  });

  it('blocks stopping for other users and recovering previews', () => {
    expect(getJobActionState(job({created_by: 2}), ctx).canStop).toBe(false);
    expect(
      getJobActionState(job({run_type: 'preview', status: 'failed'}), ctx)
        .showRecover,
    ).toBe(false);
  });

  it('allows recovering a terminal cluster job unless runs are disabled', () => {
    const failed = job({status: 'failed'});
    expect(getJobActionState(failed, ctx)).toMatchObject({
      showStop: false,
      canRecover: true,
    });
    expect(
      getJobActionState(failed, {...ctx, disableRecover: true}).canRecover,
    ).toBe(false);
    expect(getJobActionState(failed, {...ctx, canRun: false}).canRecover).toBe(
      false,
    );
  });
});
