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

import {fireEvent, render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {SyncJobInstance} from '@/lib/services/sync';
import {JobTable} from '../JobTable';
import {DEFAULT_JOB_LIST_QUERY, type JobListQuery} from '../types';

// jsdom 缺少 matchMedia / ResizeObserver（StudioTable 与 Arc 组件会用到）。
// jsdom lacks matchMedia / ResizeObserver (StudioTable and Arc components need them).
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
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

vi.mock('next/link', () => ({
  default: ({href, children}: {href: string; children: React.ReactNode}) => (
    <a href={href}>{children}</a>
  ),
}));
vi.mock('@/components/charts/sparkline', () => ({
  Sparkline: () => <span data-testid='sparkline' />,
}));

const job = (
  id: number,
  status: SyncJobInstance['status'],
  extra: Partial<SyncJobInstance> = {},
): SyncJobInstance => ({
  id,
  task_id: 10 + id,
  task_version: 1,
  run_type: 'run',
  platform_job_id: String(id),
  engine_job_id: `e${id}`,
  status,
  submit_spec: {job_name: `job-${id}`},
  result_preview: {},
  error_message: '',
  created_by: 1,
  created_at: '2026-11-14T10:00:00Z',
  updated_at: '2026-11-14T10:00:00Z',
  started_at: '2026-11-14T10:00:00Z',
  cluster_id: 1,
  mode: 'streaming',
  ...extra,
});

const renderTable = (
  items: SyncJobInstance[],
  props: {
    preset?: 'global' | 'task';
    query?: Partial<JobListQuery>;
    updatedAt?: number | null;
    onQueryChange?: (patch: Partial<JobListQuery>) => void;
    selection?: {ids: Set<number>; onChange: (ids: Set<number>) => void};
  } = {},
) => {
  const onQueryChange = props.onQueryChange ?? vi.fn();
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <JobTable
        preset={props.preset ?? 'global'}
        data={{
          items,
          total: items.length,
          loading: false,
          refreshing: false,
          error: null,
          updatedAt: props.updatedAt ?? null,
        }}
        query={{...DEFAULT_JOB_LIST_QUERY, ...props.query}}
        onQueryChange={onQueryChange}
        pageSize={50}
        clusterNames={new Map([[1, 'prod']])}
        actor={{
          isAdmin: true,
          isOwner: false,
          canRun: true,
          disableRecover: false,
        }}
        selection={props.selection}
        onRowActivate={vi.fn()}
        onOpenDetail={vi.fn()}
        onAction={vi.fn()}
        testIdPrefix='job'
      />
    </NextIntlClientProvider>,
  );
  return onQueryChange;
};

describe('JobTable column registry', () => {
  it('global preset shows the cluster column; the task preset hides it', () => {
    const items = [job(1, 'running', {live: {write_qps: 5}})];
    const {unmount} = render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <JobTable
          preset='global'
          data={{
            items,
            total: 1,
            loading: false,
            refreshing: false,
            error: null,
            updatedAt: null,
          }}
          query={DEFAULT_JOB_LIST_QUERY}
          onQueryChange={vi.fn()}
          pageSize={50}
          clusterNames={new Map([[1, 'prod']])}
          actor={{isAdmin: true, isOwner: false, canRun: true, disableRecover: false}}
          onRowActivate={vi.fn()}
          onOpenDetail={vi.fn()}
          onAction={vi.fn()}
          testIdPrefix='job'
        />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText('集群')).toBeTruthy();
    expect(within(screen.getByTestId('job-row-1')).getByText('prod')).toBeTruthy();
    unmount();

    renderTable(items, {preset: 'task'});
    expect(screen.queryByText('集群')).toBeNull();
    // task 预设仍有公共列。/ Shared columns remain on the task preset.
    expect(screen.getByText('读取 / 写入条数')).toBeTruthy();
  });

  it('task preset shows the CDC column only when some row reports a delay', () => {
    renderTable([job(1, 'running', {live: {write_qps: 5}})], {preset: 'task'});
    expect(screen.queryByText('CDC 延迟')).toBeNull();
    renderTable([job(1, 'running', {live: {cdc_delay_seconds: 2}})], {
      preset: 'task',
    });
    expect(screen.getByText('CDC 延迟')).toBeTruthy();
  });

  it('shows the checkbox column only when the host passes a selection', () => {
    renderTable([job(1, 'running')]);
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
    renderTable([job(1, 'running')], {
      selection: {ids: new Set(), onChange: vi.fn()},
    });
    expect(screen.getAllByRole('checkbox').length).toBeGreaterThan(0);
  });

  it('renders live QPS for running jobs and the muted average for finished ones', () => {
    const items = [
      job(1, 'running', {live: {read_qps: 8, write_qps: 4}}),
      job(2, 'success', {
        started_at: '2026-11-14T10:00:00Z',
        finished_at: '2026-11-14T10:10:00Z',
        updated_at: '2026-11-14T10:10:05Z',
        live: {read_total: 6000, write_total: 3000},
      }),
    ];
    renderTable(items);
    expect(
      within(screen.getByTestId('job-row-1')).getByText('8 / 4/s'),
    ).toBeTruthy();
    const avg = within(screen.getByTestId('job-row-2')).getByText('均 10 / 5/s');
    expect(avg.getAttribute('title')).toContain('平均速率');
    expect(avg.className).toContain('text-muted-foreground');
  });

  it('shows the multi-table badge from live.table_count and opens the metrics tab on click', () => {
    const onOpenDetail = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <JobTable
          preset='global'
          data={{
            items: [
              job(1, 'running', {
                live: {write_qps: 5, table_count: 3, read_total: 10},
              }),
              job(2, 'running', {live: {write_qps: 5, table_count: 1}}),
            ],
            total: 2,
            loading: false,
            refreshing: false,
            error: null,
            updatedAt: null,
          }}
          query={DEFAULT_JOB_LIST_QUERY}
          onQueryChange={vi.fn()}
          pageSize={50}
          actor={{isAdmin: true, isOwner: false, canRun: true, disableRecover: false}}
          onRowActivate={vi.fn()}
          onOpenDetail={onOpenDetail}
          onAction={vi.fn()}
          testIdPrefix='job'
        />
      </NextIntlClientProvider>,
    );
    const badge = screen.getByTestId('job-multi-table-1');
    expect(badge.textContent).toContain('3');
    // 单表作业不出现徽章。/ Single-table jobs get no badge.
    expect(screen.queryByTestId('job-multi-table-2')).toBeNull();
    fireEvent.click(badge);
    expect(onOpenDetail).toHaveBeenCalledWith(
      expect.objectContaining({id: 1}),
      'metrics',
    );
  });

  it('colors CDC delay red at >=60s, amber above 0 and plain at 0', () => {
    renderTable([
      job(1, 'running', {live: {cdc_delay_seconds: 65}}),
      job(2, 'running', {live: {cdc_delay_seconds: 12.5}}),
      job(3, 'running', {live: {cdc_delay_seconds: 0}}),
    ]);
    expect(screen.getByTestId('job-cdc-1').className).toContain('text-red-600');
    expect(screen.getByTestId('job-cdc-2').className).toContain(
      'text-amber-600',
    );
    const zero = screen.getByTestId('job-cdc-3');
    expect(zero.className).not.toContain('text-red-600');
    expect(zero.className).not.toContain('text-amber-600');
  });

  it('toggles the running rate sort desc→asc and resets the page; other columns sort desc', () => {
    const onQueryChange = renderTable([job(1, 'running')], {
      query: {statusGroup: 'running', sort: 'rate', order: 'desc', page: 3},
    });
    fireEvent.click(screen.getByText('读取 / 写入速率'));
    expect(onQueryChange).toHaveBeenCalledWith({
      sort: 'rate',
      order: 'asc',
      page: 1,
    });
  });

  it('falls back to descending when sorting a different column', () => {
    const onQueryChange = renderTable([job(1, 'running')], {
      query: {statusGroup: 'running', sort: 'rate', order: 'desc', page: 3},
    });
    fireEvent.click(screen.getByText('时间'));
    expect(onQueryChange).toHaveBeenCalledWith({
      sort: 'started_at',
      order: 'desc',
      page: 1,
    });
  });

  it('switches the time dimension from the header caret menu, keeping the order', async () => {
    const onQueryChange = renderTable([job(1, 'running')], {
      query: {sort: 'started_at', order: 'asc', page: 3},
    });
    // Radix 菜单在 jsdom 用 pointerDown 打开（keyDown 会踩 roving-focus 空指针）。
    // Radix menus open via pointerDown in jsdom (keyDown hits a roving-focus null ref).
    fireEvent.pointerDown(
      screen.getByRole('button', {name: zh.jobTable.sortDimension}),
    );
    const started = await screen.findByRole('menuitem', {
      name: zh.jobTable.startedAt,
    });
    // 当前维度带 ✓ 图标，其余项是等宽占位 span。/ The active dimension carries a
    // check icon; the rest render an equal-width spacer.
    expect(started.querySelector('svg')).toBeTruthy();
    const finished = screen.getByRole('menuitem', {
      name: zh.jobTable.finishedAt,
    });
    expect(finished.querySelector('svg')).toBeNull();
    fireEvent.click(finished);
    expect(onQueryChange).toHaveBeenCalledWith({
      sort: 'finished_at',
      order: 'asc',
      page: 1,
    });
  });

  it('marks the active dimension and toggles its direction on header click', () => {
    const onQueryChange = renderTable([job(1, 'running')], {
      query: {sort: 'duration', order: 'asc', page: 2},
    });
    // 方向指示落在时间列（duration 命中 sortKeys）。/ Direction lands on the time column
    // (duration matches sortKeys).
    const th = screen.getByText('时间').closest('th')!;
    expect(th.getAttribute('aria-sort')).toBe('ascending');
    fireEvent.click(screen.getByText('时间'));
    expect(onQueryChange).toHaveBeenCalledWith({
      sort: 'duration',
      order: 'desc',
      page: 1,
    });
  });

  it('external jobs get the source badge, a mode placeholder, no task ref and no workbench link', async () => {
    renderTable([
      job(7, 'success', {
        source: 'external',
        run_type: 'external',
        task_id: 0,
        task_version: 0,
        mode: undefined,
      }),
      job(8, 'running'),
    ]);
    const ext = screen.getByTestId('job-row-7');
    expect(within(ext).getByText('外部')).toBeTruthy();
    // 「外部提交」与来源徽标同义，不重复渲染。/ "外部提交" duplicates the source badge and is not rendered twice.
    expect(within(ext).queryByText('外部提交')).toBeNull();
    // 缺 jobDag 的外部作业 mode 为空时显示占位符。/ External jobs without jobDag show the mode placeholder.
    expect(
      within(ext).getByText('外部').parentElement!.textContent,
    ).toContain('-');
    // 悬浮提示不再出现「任务 #0」。/ The hover title no longer says "Task #0".
    expect(within(ext).getByText('#7 · e7').title).toBe('#7 · e7');

    const stx = screen.getByTestId('job-row-8');
    expect(within(stx).getByText('STX')).toBeTruthy();
    expect(within(stx).getByText('手动')).toBeTruthy();
    expect(within(stx).getByText('#8 · e8').title).toBe('任务 #18 · e8');

    // 「在工作台打开」对外部作业隐藏。/ "Open in workbench" is hidden for external jobs.
    const extTrigger = within(ext).getByRole('button', {name: '更多操作'});
    fireEvent.pointerDown(extTrigger);
    expect(
      await screen.findByRole('menuitem', {name: '复制作业 ID'}),
    ).toBeTruthy();
    expect(
      screen.queryByRole('menuitem', {name: '在工作台打开'}),
    ).toBeNull();
  });

  it('keeps the workbench link for STX jobs in the global preset', async () => {
    renderTable([job(8, 'running')]);
    const row = screen.getByTestId('job-row-8');
    const trigger = within(row).getByRole('button', {name: '更多操作'});
    fireEvent.pointerDown(trigger);
    await screen.findByRole('menu');
    expect(screen.getByText('在工作台打开')).toBeTruthy();
  });
});
