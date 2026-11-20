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
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  InspectionAutoPolicy,
  InspectionConditionTemplate,
} from '@/lib/services/diagnostics';
import {PoliciesPanel} from '../policies/PoliciesPanel';

const listAutoPoliciesSafe = vi.hoisted(() => vi.fn());
const listBuiltinConditionTemplatesSafe = vi.hoisted(() => vi.fn());
const getAlertRuleCatalogSafe = vi.hoisted(() => vi.fn());
const updateAutoPolicySafe = vi.hoisted(() => vi.fn());
const deleteAutoPolicySafe = vi.hoisted(() => vi.fn());
const createAutoPolicySafe = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {
    diagnostics: {
      listAutoPoliciesSafe,
      listBuiltinConditionTemplatesSafe,
      updateAutoPolicySafe,
      deleteAutoPolicySafe,
      createAutoPolicySafe,
    },
    monitoring: {getAlertRuleCatalogSafe},
  },
}));
vi.mock('@/lib/i18n', () => ({useLocale: () => ({locale: 'zh-CN'})}));

const alertFiringTemplate: InspectionConditionTemplate = {
  code: 'ALERT_FIRING',
  category: 'alert_firing',
  name: '告警触发',
  description: '在告警触发时执行巡检',
  default_threshold: 1,
  default_window_minutes: 0,
  default_cron_expr: '',
  immediate_on_match: false,
};

function policy(overrides: Partial<InspectionAutoPolicy>): InspectionAutoPolicy {
  return {
    id: 1,
    cluster_id: 0,
    name: 'policy',
    enabled: true,
    conditions: [],
    cooldown_minutes: 30,
    auto_create_task: false,
    auto_start_task: false,
    task_options: {
      include_thread_dump: false,
      include_jvm_dump: false,
      jvm_dump_min_free_mb: 2048,
      selected_resources: [],
    },
    invalid_reason: '',
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    ...overrides,
  };
}

const policies: InspectionAutoPolicy[] = [
  policy({id: 1, name: 'global-oom', cluster_id: 0}),
  policy({id: 2, name: 'prod-gc', cluster_id: 1, enabled: false}),
  policy({
    id: 3,
    name: 'stale-rule',
    cluster_id: 1,
    invalid_reason: 'unknown_alert_rule_key',
    invalid_detail: 'stx_server_memory_high',
  }),
  policy({id: 4, name: 'other-cluster', cluster_id: 2}),
];

function renderPanel(
  clusterId?: number,
  props?: {
    focusPolicyId?: number;
    onSelectPolicy?: (id: number | null) => void;
    onViewRecords?: (id: number) => void;
  },
) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <PoliciesPanel
        clusterId={clusterId}
        clusterOptions={[
          {cluster_id: 1, cluster_name: 'stx-prod'},
          {cluster_id: 2, cluster_name: 'stx-dev'},
        ]}
        focusPolicyId={props?.focusPolicyId}
        onSelectPolicy={props?.onSelectPolicy}
        onViewRecords={props?.onViewRecords}
      />
    </NextIntlClientProvider>,
  );
}

describe('PoliciesPanel', () => {
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
    listAutoPoliciesSafe.mockResolvedValue({
      success: true,
      data: {items: policies, total: policies.length, page: 1, page_size: 100},
    });
    listBuiltinConditionTemplatesSafe.mockResolvedValue({
      success: true,
      data: [alertFiringTemplate],
    });
    getAlertRuleCatalogSafe.mockResolvedValue({success: true, data: []});
    updateAutoPolicySafe.mockResolvedValue({success: true, data: {}});
    deleteAutoPolicySafe.mockResolvedValue({success: true});
    createAutoPolicySafe.mockResolvedValue({success: true, data: {}});
  });

  it('renders policy rows with scope names and the invalid reason line', async () => {
    renderPanel();
    expect(await screen.findByTestId('policy-row-1')).toBeVisible();
    // 作用范围：全局显示「全局」，集群命中名称，未知集群回落 #id。
    // Scope: global shows "全局", a known cluster shows its name, unknown falls back to #id.
    expect(screen.getAllByText('全局').length).toBeGreaterThan(0);
    expect(screen.getAllByText('stx-prod').length).toBeGreaterThan(0);
    // 失效策略显示徽标与翻译后的原因行。
    // The invalid policy shows the badge and its translated reason line.
    const staleRow = screen.getByTestId('policy-row-3');
    expect(
      within(staleRow).getByText('已失效'),
    ).toBeVisible();
    expect(
      screen.getByText(/告警规则 "stx_server_memory_high" 已不在规则目录/),
    ).toBeVisible();
  });

  it('scopes the list and status counts to the selected cluster', async () => {
    renderPanel(1);
    await screen.findByTestId('policy-row-1');
    // 集群 1 视图：全局 + 集群 1，隐藏集群 2。
    // Cluster 1 view: global + cluster 1 rows; cluster 2 hidden.
    expect(screen.getByTestId('policy-row-1')).toBeVisible();
    expect(screen.getByTestId('policy-row-2')).toBeVisible();
    expect(screen.getByTestId('policy-row-3')).toBeVisible();
    expect(screen.queryByTestId('policy-row-4')).not.toBeInTheDocument();
  });

  it('filters by name search', async () => {
    renderPanel();
    await screen.findByTestId('policy-row-1');
    fireEvent.change(screen.getByPlaceholderText('按名称搜索'), {
      target: {value: 'stale'},
    });
    expect(screen.queryByTestId('policy-row-1')).not.toBeInTheDocument();
    expect(screen.getByTestId('policy-row-3')).toBeVisible();
  });

  it('toggles a policy optimistically and calls the update API', async () => {
    renderPanel();
    const row = await screen.findByTestId('policy-row-1');
    fireEvent.click(row.querySelector('[role="switch"]')!);
    await waitFor(() =>
      expect(updateAutoPolicySafe).toHaveBeenCalledWith(1, {enabled: false}),
    );
  });

  it('intercepts a dirty close with the unsaved-changes dialog', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTestId('policy-row-1'));
    expect(await screen.findByTestId('detail-rail')).toBeVisible();

    // 改名称产生脏态，再点关闭 → 未保存确认。
    // Edit the name to go dirty, then close → unsaved confirmation.
    fireEvent.change(screen.getByLabelText('自动巡检名称'), {
      target: {value: 'renamed'},
    });
    fireEvent.click(screen.getByRole('button', {name: 'Close'}));
    await screen.findByTestId('discard-confirm-dialog');

    // 「继续编辑」保留表单与已输入内容。
    // "Keep editing" keeps the form and the typed value.
    fireEvent.click(screen.getByTestId('discard-keep-editing'));
    await waitFor(() =>
      expect(
        screen.queryByTestId('discard-confirm-dialog'),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByLabelText('自动巡检名称')).toHaveValue('renamed');

    // 再点关闭并「放弃修改」→ 详情关闭。
    // Close again and "discard" → the detail closes.
    fireEvent.click(screen.getByRole('button', {name: 'Close'}));
    fireEvent.click(await screen.findByTestId('discard-confirm-action'));
    await waitFor(() =>
      expect(screen.queryByTestId('detail-rail')).not.toBeInTheDocument(),
    );
  });

  it('deletes a policy through the two-step ConfirmMorph', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTestId('policy-row-2'));
    await screen.findByTestId('detail-rail');

    fireEvent.click(screen.getByRole('button', {name: '删除自动巡检'}));
    expect(deleteAutoPolicySafe).not.toHaveBeenCalled();
    // ConfirmMorph 有 280ms 防误触窗口（ARM_MS），等确认按钮激活再点。
    // ConfirmMorph ignores answers within the 280ms arm window; wait it out.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 320));
    });
    // 二次确认后才真正删除。
    // The API fires only after the second-step confirm.
    fireEvent.click(screen.getByRole('button', {name: '删除'}));
    await waitFor(() => expect(deleteAutoPolicySafe).toHaveBeenCalledWith(2));
    await waitFor(() =>
      expect(screen.queryByTestId('detail-rail')).not.toBeInTheDocument(),
    );
  });

  it('renders the behavior sentence and trigger stats columns', async () => {
    getAlertRuleCatalogSafe.mockResolvedValue({
      success: true,
      data: [{rule_key: 'jvm_heap', name: 'JVM 堆内存过高'}],
    });
    listAutoPoliciesSafe.mockResolvedValue({
      success: true,
      data: {
        items: [
          policy({
            id: 9,
            name: 'alert-watch',
            conditions: [
              {
                template_code: 'ALERT_FIRING',
                enabled: true,
                rule_keys: ['jvm_heap'],
              },
            ],
            last_triggered_at: new Date(
              Date.now() - 5 * 60 * 1000,
            ).toISOString(),
            trigger_count_7d: 3,
          }),
          policy({id: 10, name: 'never-ran', trigger_count_7d: 0}),
        ],
        total: 2,
        page: 1,
        page_size: 100,
      },
    });
    renderPanel();
    const row = await screen.findByTestId('policy-row-9');
    // 行为句把启用条件翻译成「当 … 时，自动巡检 …」。
    // The behavior sentence renders enabled conditions as "when …, inspect …".
    expect(
      within(row).getByText(/当 告警 JVM 堆内存过高 触发 时，自动巡检 全局/),
    ).toBeVisible();
    // 触发统计：分钟前的最近触发 + 7 天计数；未触发策略显示 —。
    // Trigger stats: minutes-ago last trigger + 7d count; untriggered shows —.
    expect(within(row).getByText('3 次')).toBeVisible();
    const neverRow = screen.getByTestId('policy-row-10');
    expect(within(neverRow).getAllByText('—').length).toBe(2);
  });

  it('opens the deep-linked policy detail via focusPolicyId', async () => {
    const onSelectPolicy = vi.fn();
    renderPanel(undefined, {focusPolicyId: 2, onSelectPolicy});
    expect(await screen.findByTestId('detail-rail')).toBeVisible();
    await waitFor(() => expect(onSelectPolicy).toHaveBeenCalledWith(2));
  });

  it('navigates to trigger records from a row action', async () => {
    const onViewRecords = vi.fn();
    renderPanel(undefined, {onViewRecords});
    fireEvent.click(await screen.findByTestId('policy-view-records-1'));
    expect(onViewRecords).toHaveBeenCalledWith(1);
  });
});
