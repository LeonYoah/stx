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

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {SyncJobInstance, SyncJobListData} from '@/lib/services/sync';
import {JobListTab} from '../JobListTab';
import type {DashboardQuery} from '../use-dashboard-query';

// jsdom 缺少 matchMedia / ResizeObserver（Arc 分段控件与动效会用到）。
// jsdom lacks matchMedia / ResizeObserver (used by the Arc segmented control and motion).
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

const sync = vi.hoisted(() => ({
  listJobs: vi.fn(),
  getJobSparklines: vi.fn(),
  cancelJob: vi.fn(),
  recoverJob: vi.fn(),
  getJobTimeseries: vi.fn(),
}));
const auth = vi.hoisted(() => ({getUserInfo: vi.fn()}));
const monitoring = vi.hoisted(() => ({listNotifiableUsers: vi.fn()}));

vi.mock('@/lib/services', () => ({default: {sync, auth, monitoring}}));
// App Router 上下文在 jsdom 里不存在；「完整详情」的 router.push 用桩代替。
// No App Router context under jsdom; stub the push used by the "full details" entry.
vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn(), replace: vi.fn(), prefetch: vi.fn()}),
}));
vi.mock('@/components/charts/sparkline', () => ({
  Sparkline: ({label}: {label: string}) => (
    <span data-testid='sparkline'>{label}</span>
  ),
}));
// 完整详情弹窗由 JobDetailHost 承载；测试只关心宿主把它打开，内部页签逻辑有独立测试。
// The full detail dialog is hosted by JobDetailHost; this suite only asserts the host opens it —
// the dialog's own tabs are covered elsewhere.
vi.mock('@/components/common/sync/job-table/JobDetailHost', () => ({
  JobDetailHost: ({job, open}: {job: SyncJobInstance | null; open: boolean}) =>
    open && job ? <div data-testid='detail-host'>{job.id}</div> : null,
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

const listData: SyncJobListData = {
  total: 2,
  items: [
    job(1, 'running', {live: {write_qps: 1234, write_total: 9000}}),
    job(2, 'failed'),
  ],
  status_counts: {failed: 1, running: 1},
};

const baseQuery: DashboardQuery = {
  tab: 'list',
  clusterId: undefined,
  status: '',
  mode: '',
  source: '',
  q: '',
  sort: '',
  order: '',
  page: 1,
  range: '1h',
  listRange: 'all',
  start: undefined,
  end: undefined,
};

const renderTab = (query: Partial<DashboardQuery> = {}, update = vi.fn()) => {
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <JobListTab
        query={{...baseQuery, ...query}}
        update={update}
        clusterNames={new Map([[1, 'prod']])}
      />
    </NextIntlClientProvider>,
  );
  return update;
};

describe('JobListTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 未登录态：行内操作退化为禁用态，不影响列表行为。/ Signed-out state: row actions degrade to disabled.
    auth.getUserInfo.mockRejectedValue(new Error('unauthorized'));
    monitoring.listNotifiableUsers.mockResolvedValue({users: []});
    sync.listJobs.mockResolvedValue(listData);
    sync.getJobSparklines.mockResolvedValue({
      source: 'none',
      range: '1h',
      metric: 'write',
      step_seconds: 15,
      items: {},
    });
    sync.cancelJob.mockResolvedValue(job(1, 'cancelling'));
  });

  it('requests the global list with server-side filters, excluding previews', async () => {
    renderTab({
      status: 'failed',
      mode: 'batch',
      q: 'orders',
      clusterId: 3,
      page: 2,
    });
    await screen.findByTestId('home-job-row-1');
    expect(sync.listJobs).toHaveBeenCalledWith({
      current: 2,
      size: 50,
      status_group: 'failed',
      mode: 'batch',
      cluster_id: 3,
      keyword: 'orders',
      sort: 'started_at',
      order: 'desc',
      with_counts: 1,
      with_live: 1,
    });
  });

  // 顶栏时间窗 → 活跃窗口参数：预设传 range，自定义传 start/end，'all' 不带窗口参数。
  // The header range maps to active-window params: presets send range, custom sends start/end, 'all' sends none.
  it('maps the header range onto the active-window list params', async () => {
    renderTab({listRange: '6h'});
    await screen.findByTestId('home-job-row-1');
    expect(sync.listJobs.mock.calls[0][0]).toMatchObject({range: '6h'});

    vi.clearAllMocks();
    sync.listJobs.mockResolvedValue(listData);
    renderTab({start: 1700000000000, end: 1700003600000});
    await screen.findByTestId('home-job-row-1');
    expect(sync.listJobs.mock.calls[0][0]).toMatchObject({
      start: 1700000000000,
      end: 1700003600000,
    });
    expect(sync.listJobs.mock.calls[0][0].range).toBeUndefined();
  });

  it('falls back to the start-time order when a rate sort is asked for outside the running group', async () => {
    renderTab({sort: 'rate'});
    await screen.findByTestId('home-job-row-1');
    expect(sync.listJobs.mock.calls[0][0].sort).toBe('started_at');
  });

  it('switches status through the pills and resets the page', async () => {
    const update = renderTab();
    await screen.findByTestId('home-job-row-1');
    fireEvent.click(screen.getByTestId('home-job-pill-failed'));
    expect(update).toHaveBeenCalledWith({status: 'failed', page: 1});
  });

  it('shows sparklines only when Prometheus answers, from one batched request', async () => {
    sync.getJobSparklines.mockResolvedValue({
      source: 'prometheus',
      range: '1h',
      metric: 'write',
      step_seconds: 15,
      items: {
        '1': [
          [1, 10],
          [2, 12],
        ],
      },
    });
    renderTab();
    expect(await screen.findByTestId('sparkline')).toBeTruthy();
    expect(sync.getJobSparklines).toHaveBeenCalledWith({ids: [1], range: '1h'});
    expect(
      within(screen.getByTestId('home-job-row-1')).getByText('- / 1.2K/s'),
    ).toBeTruthy();
  });

  it('hides sparklines without Prometheus', async () => {
    renderTab();
    await screen.findByTestId('home-job-row-1');
    await waitFor(() => expect(sync.getJobSparklines).toHaveBeenCalled());
    expect(screen.queryByTestId('sparkline')).toBeNull();
  });

  it('bulk-stops only eligible jobs and reports the outcome in the page', async () => {
    renderTab();
    await screen.findByTestId('home-job-row-1');
    for (const id of [1, 2]) {
      fireEvent.click(
        within(screen.getByTestId(`home-job-row-${id}`)).getByRole('checkbox'),
      );
    }
    // 行内也有「停止」（未登录禁用态），批量按钮限定在批量条内。
    // Rows also render a (disabled) Stop button; scope the bulk one inside the bulk bar.
    fireEvent.click(
      within(screen.getByTestId('home-job-bulk')).getByRole('button', {
        name: '停止',
      }),
    );
    fireEvent.click(await screen.findByRole('button', {name: '确认'}));
    await waitFor(() => expect(sync.cancelJob).toHaveBeenCalledTimes(1));
    expect(sync.cancelJob).toHaveBeenCalledWith(1, {
      stop_with_savepoint: false,
    });
    expect(
      await screen.findByText('完成 1 个，失败 0 个，跳过 1 个'),
    ).toBeTruthy();
  });

  it('hides the pager on a single page and pages server-side beyond 50 jobs', async () => {
    renderTab();
    await screen.findByTestId('home-job-row-1');
    expect(screen.getByText('共 2 条')).toBeTruthy();
    expect(screen.queryByTitle('下一页')).toBeNull();

    vi.clearAllMocks();
    sync.listJobs.mockResolvedValue({...listData, total: 120});
    sync.getJobSparklines.mockResolvedValue({
      source: 'none',
      range: '1h',
      metric: 'write',
      step_seconds: 15,
      items: {},
    });
    document.body.innerHTML = '';
    const update = renderTab({page: 2});
    await screen.findByTestId('home-job-row-1');
    expect(sync.listJobs.mock.calls[0][0]).toMatchObject({
      current: 2,
      size: 50,
    });
    expect(screen.getByText('共 120 条')).toBeTruthy();
    fireEvent.click(screen.getByTitle('下一页'));
    expect(update).toHaveBeenCalledWith({page: 3});
  });

  it('shows the average rate for finished jobs and the end time column', async () => {
    sync.listJobs.mockResolvedValue({
      total: 1,
      items: [
        job(3, 'success', {
          started_at: '2026-11-14T10:00:00Z',
          finished_at: '2026-11-14T10:10:00Z',
          updated_at: '2026-11-14T10:10:05Z',
          live: {read_total: 6000, write_total: 3000},
        }),
      ],
      status_counts: {finished: 1},
    });
    renderTab();
    const row = await screen.findByTestId('home-job-row-3');
    const rate = within(row).getByText('均 10 / 5/s');
    expect(rate.getAttribute('title')).toContain('平均速率');
    expect(within(row).getByText('6K / 3K')).toBeTruthy();
    expect(within(row).getByText('耗时 10 分')).toBeTruthy();
  });

  it('opens the full detail dialog on row click', async () => {
    renderTab();
    fireEvent.click(await screen.findByTestId('home-job-row-2'));
    expect(screen.getByTestId('detail-host').textContent).toBe('2');
  });
});
