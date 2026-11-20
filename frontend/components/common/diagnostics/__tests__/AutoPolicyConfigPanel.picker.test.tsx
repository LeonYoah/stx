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

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  InspectionAutoPolicy,
  InspectionConditionTemplate,
} from '@/lib/services/diagnostics';
import type {AlertRuleCatalogItem} from '@/lib/services/monitoring/types';
import {AutoPolicyConfigPanel} from '../AutoPolicyConfigPanel';

const listAutoPoliciesSafe = vi.hoisted(() => vi.fn());
const listBuiltinConditionTemplatesSafe = vi.hoisted(() => vi.fn());
const getAlertRuleCatalogSafe = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {
    diagnostics: {listAutoPoliciesSafe, listBuiltinConditionTemplatesSafe},
    monitoring: {getAlertRuleCatalogSafe},
  },
}));
vi.mock('@/lib/i18n', () => ({useLocale: () => ({locale: 'zh-CN'})}));

const catalog: AlertRuleCatalogItem[] = [
  {
    rule_key: 'node_offline',
    name: '节点离线',
    category: 'platform_health',
    scope: 'cluster',
  },
  {
    rule_key: 'gc_time_high',
    name: 'GC 耗时过高',
    category: 'metric',
    scope: 'cluster',
  },
  {
    rule_key: 'sync_job_failed',
    name: '作业运行失败',
    category: 'job',
    scope: 'job',
  },
];

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

const policyWithDeadRuleKey: InspectionAutoPolicy = {
  id: 9,
  cluster_id: 0,
  name: 'stale-policy',
  enabled: true,
  conditions: [
    {
      template_code: 'ALERT_FIRING',
      enabled: true,
      rule_keys: ['node_offline', 'stx_server_memory_high'],
    },
  ],
  cooldown_minutes: 30,
  auto_create_task: false,
  auto_start_task: false,
  task_options: {
    include_thread_dump: false,
    include_jvm_dump: false,
    jvm_dump_min_free_mb: 2048,
    selected_resources: [],
  },
  invalid_reason: 'unknown_alert_rule_key',
  invalid_detail: 'stx_server_memory_high',
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
};

function renderPanel() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <AutoPolicyConfigPanel
        open={true}
        onOpenChange={() => {}}
        clusterOptions={[{cluster_id: 1, cluster_name: 'stx-prod'}]}
      />
    </NextIntlClientProvider>,
  );
}

describe('auto policy rule picker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 测试环境没有 matchMedia，模拟普通动画偏好。
    // JSDOM lacks matchMedia; stub the standard preference.
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
      data: {
        items: [policyWithDeadRuleKey],
        total: 1,
        page: 1,
        page_size: 100,
      },
    });
    listBuiltinConditionTemplatesSafe.mockResolvedValue({
      success: true,
      data: [alertFiringTemplate],
    });
    getAlertRuleCatalogSafe.mockResolvedValue({success: true, data: catalog});
  });

  it('groups the catalog by scope into exactly two groups', async () => {
    renderPanel();
    fireEvent.click(await screen.findByRole('button', {name: '编辑'}));

    expect(await screen.findByText('集群级')).toBeVisible();
    expect(screen.getByText('作业级')).toBeVisible();
    // 旧三类分组（platform_health / metric / job 类别文案）不再出现。
    // The legacy three-category group labels no longer render.
    expect(screen.queryByText('平台健康')).not.toBeInTheDocument();
    expect(screen.queryByText('指标告警')).not.toBeInTheDocument();
    expect(screen.queryByText('作业告警')).not.toBeInTheDocument();
    // 目录项出现在选择器里。
    // Catalog entries show up in the picker.
    expect(screen.getByText('GC 耗时过高')).toBeVisible();
    expect(screen.getByText('作业运行失败')).toBeVisible();
  });

  it('shows catalog names for known keys and a removable warning badge for unknown keys', async () => {
    renderPanel();
    fireEvent.click(await screen.findByRole('button', {name: '编辑'}));

    // 目录内键显示目录名，title 保留原始规则键。
    // In-catalog keys render the catalog name with the raw rule key on title.
    const knownBadge = await screen.findByTitle('node_offline');
    expect(knownBadge).toHaveTextContent('节点离线');

    // 目录外键以警示徽标显示原始键，点击移除按钮取消勾选。
    // Out-of-catalog keys show the raw key in a warning badge; its remove button deselects it.
    expect(screen.getByText('stx_server_memory_high')).toBeVisible();
    fireEvent.click(
      screen.getByRole('button', {
        name: '移除告警规则 "stx_server_memory_high"',
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByText('stx_server_memory_high'),
      ).not.toBeInTheDocument(),
    );
  });

  it('renders the translated unknown_alert_rule_key reason in the list', async () => {
    renderPanel();
    expect(
      await screen.findByText(
        /告警规则 "stx_server_memory_high" 已不在规则目录/,
      ),
    ).toBeVisible();
  });
});
