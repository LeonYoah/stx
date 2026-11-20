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
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReport,
} from '@/lib/services/diagnostics';
import {InspectionsPanel} from '../inspections/InspectionsPanel';

const getInspectionReportsSafe = vi.hoisted(() => vi.fn());
const getInspectionReportDetailSafe = vi.hoisted(() => vi.fn());
const getTaskSafe = vi.hoisted(() => vi.fn());
const listResourcesSafe = vi.hoisted(() => vi.fn());
const startInspectionSafe = vi.hoisted(() => vi.fn());
const createTaskSafe = vi.hoisted(() => vi.fn());
const listAutoPoliciesSafe = vi.hoisted(() => vi.fn());
const listBuiltinConditionTemplatesSafe = vi.hoisted(() => vi.fn());
const getAlertRuleCatalogSafe = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {
    diagnostics: {
      getInspectionReportsSafe,
      getInspectionReportDetailSafe,
      getTaskSafe,
      listResourcesSafe,
      startInspectionSafe,
      createTaskSafe,
      listAutoPoliciesSafe,
      listBuiltinConditionTemplatesSafe,
      getTaskHTMLUrl: (id: number) => `/tasks/${id}/html`,
      getTaskBundleUrl: (id: number) => `/tasks/${id}/bundle`,
    },
    monitoring: {
      getAlertRuleCatalogSafe,
    },
  },
}));
vi.mock('@/lib/i18n', () => ({useLocale: () => ({locale: 'zh-CN'})}));

function report(
  overrides: Partial<DiagnosticsInspectionReport>,
): DiagnosticsInspectionReport {
  return {
    id: 1,
    cluster_id: 1,
    status: 'completed',
    trigger_source: 'diagnostics_workspace',
    lookback_minutes: 30,
    error_threshold: 1,
    requested_by_user_id: 1,
    requested_by: 'admin',
    summary: 'all good',
    error_message: '',
    finding_total: 0,
    critical_count: 0,
    warning_count: 0,
    info_count: 0,
    started_at: '2026-11-20T09:00:00Z',
    finished_at: '2026-11-20T09:05:00Z',
    created_at: '2026-11-20T09:00:00Z',
    updated_at: '2026-11-20T09:05:00Z',
    ...overrides,
  };
}

function finding(
  overrides: Partial<DiagnosticsInspectionFinding>,
): DiagnosticsInspectionFinding {
  return {
    id: 10,
    report_id: 1,
    cluster_id: 1,
    severity: 'warning',
    category: 'errors',
    check_code: 'ERR_BURST',
    check_name: '错误突增检查',
    summary: 'worker-a 上错误激增',
    recommendation: '查看日志',
    evidence_summary: 'engine.log 中 15 次 TaskExecuteException',
    related_node_id: 11,
    related_host_id: 21,
    related_host_name: 'worker-a',
    related_host_ip: '10.0.0.21',
    related_error_group_id: 0,
    related_alert_id: '',
    created_at: '2026-11-20T09:05:00Z',
    updated_at: '2026-11-20T09:05:00Z',
    ...overrides,
  };
}

const reports: DiagnosticsInspectionReport[] = [
  report({
    id: 1,
    status: 'completed',
    finding_total: 2,
    critical_count: 1,
    warning_count: 1,
    summary: 'hot findings',
  }),
  report({id: 2, status: 'failed', summary: 'boom'}),
  report({id: 3, status: 'running', cluster_id: 2, summary: 'in flight'}),
];

const clusterOptions = [
  {cluster_id: 1, cluster_name: 'stx-prod'},
  {cluster_id: 2, cluster_name: 'stx-dev'},
];

function renderPanel(props?: {
  clusterId?: number;
  reportId?: number;
  onSelectReport?: (id: number | null) => void;
  view?: 'records' | 'auto';
  onViewChange?: (view: 'records' | 'auto') => void;
  policyId?: number;
  onSelectPolicy?: (id: number | null) => void;
  autoPolicyId?: number;
  onAutoPolicyFilter?: (id: number | null) => void;
  onOpenPolicy?: (id: number) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <InspectionsPanel
        clusterId={props?.clusterId}
        clusterOptions={clusterOptions}
        reportId={props?.reportId}
        onSelectReport={props?.onSelectReport}
        view={props?.view}
        onViewChange={props?.onViewChange}
        policyId={props?.policyId}
        onSelectPolicy={props?.onSelectPolicy}
        autoPolicyId={props?.autoPolicyId}
        onAutoPolicyFilter={props?.onAutoPolicyFilter}
        onOpenPolicy={props?.onOpenPolicy}
        onDirtyChange={props?.onDirtyChange}
      />
    </NextIntlClientProvider>,
  );
}

function mockDetail(id: number, data?: object) {
  getInspectionReportDetailSafe.mockImplementation((targetId: number) =>
    Promise.resolve({
      success: true,
      data: {
        report: reports.find((r) => r.id === targetId),
        findings: [finding({report_id: targetId})],
        related_diagnostic_task: null,
        ...data,
      },
    }),
  );
}

describe('InspectionsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // JSDOM 没有 matchMedia；narrow=false 让详情走常驻栏而不是抽屉。
    // JSDOM lacks matchMedia; narrow=false keeps the detail in the persistent rail.
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });
    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {items: reports, total: reports.length, page: 1, page_size: 10},
    });
    mockDetail(1);
    listResourcesSafe.mockResolvedValue({success: true, data: []});
    listAutoPoliciesSafe.mockResolvedValue({
      success: true,
      data: {
        items: [{id: 7, name: '夜间巡检', cluster_id: 1, enabled: true}],
        total: 1,
      },
    });
    listBuiltinConditionTemplatesSafe.mockResolvedValue({
      success: true,
      data: [],
    });
    getAlertRuleCatalogSafe.mockResolvedValue({success: true, data: []});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders report rows with status chips and finding badges', async () => {
    renderPanel();
    expect(await screen.findByTestId('inspection-row-1')).toBeVisible();
    expect(screen.getByTestId('inspection-row-2')).toBeVisible();
    expect(screen.getByTestId('inspection-row-3')).toBeVisible();
    expect(
      within(screen.getByTestId('inspection-row-1')).getByText('已完成'),
    ).toBeVisible();
    expect(
      within(screen.getByTestId('inspection-row-1')).getByText('1 严重'),
    ).toBeVisible();
    expect(
      within(screen.getByTestId('inspection-row-2')).getByText('失败'),
    ).toBeVisible();
    expect(
      within(screen.getByTestId('inspection-row-3')).getByText('执行中'),
    ).toBeVisible();
  });

  it('sends the status filter server-side and resets to page 1', async () => {
    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {items: reports, total: 45, page: 1, page_size: 10},
    });
    renderPanel();
    await screen.findByTestId('inspection-row-1');

    // 翻到第 2 页后切「失败」：请求带 status=failed 且回第 1 页。
    // Paginate, then pick "失败": the request carries status=failed at page 1.
    fireEvent.click(screen.getByRole('button', {name: '2'}));
    await waitFor(() =>
      expect(getInspectionReportsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({page: 2}),
      ),
    );
    fireEvent.click(screen.getByRole('button', {name: '失败'}));
    await waitFor(() =>
      expect(getInspectionReportsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({page: 1, status: 'failed'}),
      ),
    );
  });

  it('silently refreshes while a page holds a running report', async () => {
    // 假定时器必须先于挂载启用，否则已排上的真实 interval 不会走虚拟时钟。
    // Fake timers must be armed before mount, or the already-scheduled real
    // interval keeps ticking on the real clock.
    vi.useFakeTimers();
    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {
        items: [report({id: 9, status: 'running'})],
        total: 1,
        page: 1,
        page_size: 10,
      },
    });
    renderPanel();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId('inspection-row-9')).toBeVisible();
    const callsAfterLoad = getInspectionReportsSafe.mock.calls.length;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(
      getInspectionReportsSafe.mock.calls.length,
    ).toBeGreaterThan(callsAfterLoad);
    // 静默刷新不应触发骨架 / loading 态闪烁。
    // A silent refresh must not flash the skeleton / loading state.
    expect(screen.getByTestId('inspection-row-9')).toBeVisible();
    vi.useRealTimers();
  });

  it('stops auto-refresh when no live reports remain', async () => {
    vi.useFakeTimers();
    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {
        items: [report({id: 9, status: 'completed'})],
        total: 1,
        page: 1,
        page_size: 10,
      },
    });
    renderPanel();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId('inspection-row-9')).toBeVisible();
    const callsAfterLoad = getInspectionReportsSafe.mock.calls.length;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });
    expect(getInspectionReportsSafe.mock.calls.length).toBe(callsAfterLoad);
    vi.useRealTimers();
  });

  it('discards a stale list response when filters change mid-flight', async () => {
    let resolveFirst: ((value: object) => void) | undefined;
    getInspectionReportsSafe
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValue({
        success: true,
        data: {
          items: [report({id: 2, status: 'failed'})],
          total: 1,
          page: 1,
          page_size: 10,
        },
      });
    renderPanel();

    fireEvent.click(screen.getByRole('button', {name: '失败'}));
    expect(await screen.findByTestId('inspection-row-2')).toBeVisible();

    // 首个请求晚到：旧响应不得覆盖已生效的筛选结果。
    // The first request lands late: the stale payload must not overwrite the
    // filtered result.
    resolveFirst?.({
      success: true,
      data: {items: reports, total: 3, page: 1, page_size: 10},
    });
    await waitFor(() =>
      expect(screen.queryByTestId('inspection-row-1')).toBeNull(),
    );
  });

  it('disables the start action without a selected cluster', async () => {
    renderPanel();
    await screen.findByTestId('inspection-row-1');
    expect(screen.getByTestId('inspection-start')).toBeDisabled();
  });

  it('opens the detail, resolves the owning cluster, and syncs report_id', async () => {
    const onSelectReport = vi.fn();
    renderPanel({onSelectReport});
    fireEvent.click(await screen.findByTestId('inspection-row-3'));

    await waitFor(() =>
      expect(getInspectionReportDetailSafe).toHaveBeenCalledWith(3),
    );
    expect(onSelectReport).toHaveBeenCalledWith(3);
    expect(await screen.findByTestId('detail-rail')).toBeVisible();
    // 归属集群按报告自身 cluster_id（2）反查为 stx-dev，与筛选无关。
    // The owning cluster resolves to stx-dev via the report's own cluster_id,
    // independent of the current filter.
    expect(
      within(
        await screen.findByTestId('inspection-report-detail'),
      ).getByText('stx-dev'),
    ).toBeVisible();
  });

  it('navigates between reports with J/K while the detail is open', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTestId('inspection-row-1'));
    await screen.findByTestId('detail-rail');
    await waitFor(() =>
      expect(getInspectionReportDetailSafe).toHaveBeenCalledWith(1),
    );

    // J/K 监听挂在 window 上；在 body 上派发可冒泡且 target 是元素。
    // J/K listeners live on window; dispatching on body bubbles up and keeps a
    // real element as the target.
    fireEvent.keyDown(document.body, {key: 'j'});
    await waitFor(() =>
      expect(getInspectionReportDetailSafe).toHaveBeenCalledWith(2),
    );
    fireEvent.keyDown(document.body, {key: 'k'});
    await waitFor(() =>
      expect(getInspectionReportDetailSafe).toHaveBeenLastCalledWith(1),
    );
  });

  it('deep-links into a report detail via the reportId prop', async () => {
    const onSelectReport = vi.fn();
    renderPanel({reportId: 2, onSelectReport});
    await waitFor(() =>
      expect(getInspectionReportDetailSafe).toHaveBeenCalledWith(2),
    );
    expect(await screen.findByTestId('detail-rail')).toBeVisible();
  });

  it('shows the error state and retries the list load', async () => {
    getInspectionReportsSafe.mockResolvedValue({
      success: false,
      error: 'boom',
    });
    renderPanel();
    expect(await screen.findByText('boom')).toBeVisible();

    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {items: reports, total: reports.length, page: 1, page_size: 10},
    });
    fireEvent.click(screen.getByRole('button', {name: '重试'}));
    expect(await screen.findByTestId('inspection-row-1')).toBeVisible();
  });

  it('renders the auto-trigger chip with the policy name and opens the policy', async () => {
    getInspectionReportsSafe.mockResolvedValue({
      success: true,
      data: {
        items: [
          report({id: 5, trigger_source: 'auto', auto_policy_id: 7}),
          report({id: 6, trigger_source: 'auto'}),
        ],
        total: 2,
        page: 1,
        page_size: 10,
      },
    });
    const onOpenPolicy = vi.fn();
    renderPanel({onOpenPolicy});
    const chip = await screen.findByTestId('inspection-auto-chip-5');
    await waitFor(() => expect(chip).toHaveTextContent('自动 · 夜间巡检'));
    // 无归属策略 id 的自动报告退回普通触发徽章。
    // Auto reports without a policy id keep the plain trigger chip.
    expect(screen.queryByTestId('inspection-auto-chip-6')).toBeNull();
    fireEvent.click(chip);
    expect(onOpenPolicy).toHaveBeenCalledWith(7);
    // 点徽章不触发行选中。Chip clicks must not select the row.
    expect(getInspectionReportDetailSafe).not.toHaveBeenCalled();
  });

  it('sends auto_policy_id server-side and shows a removable filter badge', async () => {
    const onAutoPolicyFilter = vi.fn();
    renderPanel({autoPolicyId: 7, onAutoPolicyFilter});
    await waitFor(() =>
      expect(getInspectionReportsSafe).toHaveBeenCalledWith(
        expect.objectContaining({auto_policy_id: 7}),
      ),
    );
    await screen.findByTestId('inspection-row-1');
    expect(await screen.findByText('触发规则：夜间巡检')).toBeVisible();
    fireEvent.click(screen.getByTestId('inspection-policy-filter-clear'));
    expect(onAutoPolicyFilter).toHaveBeenCalledWith(null);
  });

  it('switches to the auto view through the segment callback', async () => {
    const onViewChange = vi.fn();
    renderPanel({onViewChange});
    await screen.findByTestId('inspection-row-1');
    fireEvent.click(screen.getByRole('button', {name: '自动巡检'}));
    expect(onViewChange).toHaveBeenCalledWith('auto');
  });

  it('renders the policies view when view=auto', async () => {
    renderPanel({view: 'auto'});
    // 自动视图渲染策略列表（来自 PoliciesPanel），分段仍留在工具栏。
    // The auto view renders the policy list (PoliciesPanel) while the view
    // segment stays in the toolbar.
    expect(await screen.findByTestId('policy-row-7')).toBeVisible();
    expect(screen.getByRole('button', {name: '巡检记录'})).toBeVisible();
    expect(screen.getByTestId('policy-create')).toBeVisible();
  });

  it('intercepts a view switch while the policy form is dirty', async () => {
    const onViewChange = vi.fn();
    const onDirtyChange = vi.fn();
    renderPanel({view: 'auto', onViewChange, onDirtyChange});
    fireEvent.click(await screen.findByTestId('policy-row-7'));
    await screen.findByTestId('detail-rail');
    // 改名称产生脏态，再点「巡检记录」→ 未保存确认拦截。
    // Edit the name to go dirty, then pick "巡检记录" → the unsaved dialog
    // intercepts the switch.
    fireEvent.change(screen.getByLabelText('自动巡检名称'), {
      target: {value: 'renamed'},
    });
    await waitFor(() => expect(onDirtyChange).toHaveBeenCalledWith(true));
    fireEvent.click(screen.getByRole('button', {name: '巡检记录'}));
    expect(onViewChange).not.toHaveBeenCalled();
    expect(await screen.findByTestId('discard-confirm-dialog')).toBeVisible();
    fireEvent.click(screen.getByTestId('discard-confirm-action'));
    expect(onViewChange).toHaveBeenCalledWith('records');
  });
});
