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

import type {Page} from '@playwright/test';

function ok<T>(data: T) {
  return {
    error_msg: '',
    data,
  };
}

type PolicyItem = {
  id: number;
  cluster_id: number;
  name: string;
  enabled: boolean;
  conditions: Array<{
    template_code: string;
    enabled: boolean;
    rule_keys?: string[];
  }>;
  cooldown_minutes: number;
  auto_create_task: boolean;
  auto_start_task: boolean;
  task_options: {
    include_thread_dump: boolean;
    include_jvm_dump: boolean;
    jvm_dump_min_free_mb: number;
    selected_resources: string[];
  };
  invalid_reason: string;
  invalid_detail?: string;
  created_at: string;
  updated_at: string;
};

const now = '2026-11-20T10:00:00Z';
const prodClusterId = 1;

const baseTaskOptions = {
  include_thread_dump: false,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
  selected_resources: [] as string[],
};

function buildPolicy(overrides: Partial<PolicyItem>): PolicyItem {
  return {
    id: 1,
    cluster_id: 0,
    name: 'policy',
    enabled: true,
    conditions: [],
    cooldown_minutes: 30,
    auto_create_task: false,
    auto_start_task: false,
    task_options: {...baseTaskOptions},
    invalid_reason: '',
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

function createInitialState() {
  return {
    policies: [
      buildPolicy({id: 1, name: 'global-oom-watch', cluster_id: 0}),
      buildPolicy({
        id: 2,
        name: 'prod-gc-alert',
        cluster_id: prodClusterId,
        conditions: [
          {
            template_code: 'ALERT_FIRING',
            enabled: true,
            rule_keys: ['gc_time_high'],
          },
        ],
      }),
      buildPolicy({
        id: 3,
        name: 'stale-rule-policy',
        cluster_id: prodClusterId,
        conditions: [
          {
            template_code: 'ALERT_FIRING',
            enabled: true,
            rule_keys: ['stx_server_memory_high'],
          },
        ],
        invalid_reason: 'unknown_alert_rule_key',
        invalid_detail: 'stx_server_memory_high',
      }),
    ] as PolicyItem[],
    nextPolicyId: 100,
    createShouldFail: false,
  };
}

const conditionTemplates = [
  {
    code: 'ALERT_FIRING',
    category: 'alert_firing',
    name: '告警触发',
    description: '在告警触发时执行巡检',
    default_threshold: 1,
    default_window_minutes: 0,
    default_cron_expr: '',
    immediate_on_match: false,
  },
];

const ruleCatalog = [
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

/**
 * 诊断工作台 / 巡检策略接口的模板路由桩：bootstrap、策略 CRUD、条件模板、规则目录
 * 全部走夹具状态，其余 /api/v1 请求放行给真实后端（登录态等）。
 * Route stubs for the diagnostics workspace / policies endpoints backed by a
 * stateful fixture; every other /api/v1 request passes through to the real
 * backend (auth, layout, etc.).
 */
export async function installDiagnosticsPoliciesTemplateRoutes(
  page: Page,
): Promise<ReturnType<typeof createInitialState>> {
  const state = createInitialState();

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const {pathname} = url;
    const method = request.method();

    if (method === 'GET' && pathname === '/api/v1/diagnostics/bootstrap') {
      await route.fulfill({
        status: 200,
        json: ok({
          generated_at: now,
          default_tab: 'errors',
          tabs: [],
          cluster_options: [
            {cluster_id: prodClusterId, cluster_name: 'stx-prod'},
            {cluster_id: 2, cluster_name: 'stx-dev'},
          ],
          entry_context: null,
          boundaries: [],
        }),
      });
      return;
    }

    if (
      method === 'GET' &&
      pathname === '/api/v1/diagnostics/auto-policies/templates'
    ) {
      await route.fulfill({status: 200, json: ok(conditionTemplates)});
      return;
    }

    if (method === 'GET' && pathname === '/api/v1/diagnostics/auto-policies') {
      await route.fulfill({
        status: 200,
        json: ok({
          items: state.policies,
          total: state.policies.length,
          page: 1,
          page_size: 100,
        }),
      });
      return;
    }

    if (method === 'POST' && pathname === '/api/v1/diagnostics/auto-policies') {
      if (state.createShouldFail) {
        await route.fulfill({
          status: 500,
          json: {error_msg: 'policy create exploded', data: null},
        });
        return;
      }
      const payload = request.postDataJSON() as Partial<PolicyItem>;
      const created = buildPolicy({
        ...payload,
        id: state.nextPolicyId,
        task_options: {...baseTaskOptions, ...(payload.task_options ?? {})},
      });
      state.nextPolicyId += 1;
      state.policies = [...state.policies, created];
      await route.fulfill({status: 200, json: ok(created)});
      return;
    }

    const policyMatch = pathname.match(
      /^\/api\/v1\/diagnostics\/auto-policies\/(\d+)$/,
    );
    if (policyMatch && method === 'PUT') {
      const id = Number(policyMatch[1]);
      const payload = request.postDataJSON() as Partial<PolicyItem>;
      state.policies = state.policies.map((item) =>
        item.id === id
          ? {...item, ...payload, updated_at: '2026-11-20T11:00:00Z'}
          : item,
      );
      const updated = state.policies.find((item) => item.id === id);
      await route.fulfill({status: 200, json: ok(updated ?? null)});
      return;
    }
    if (policyMatch && method === 'DELETE') {
      const id = Number(policyMatch[1]);
      state.policies = state.policies.filter((item) => item.id !== id);
      await route.fulfill({status: 200, json: ok(null)});
      return;
    }

    if (
      method === 'GET' &&
      pathname === '/api/v1/monitoring/alert-rules/catalog'
    ) {
      await route.fulfill({status: 200, json: ok(ruleCatalog)});
      return;
    }

    await route.continue();
  });

  return state;
}

export const diagnosticsPoliciesTemplate = {
  prodClusterId,
  prodClusterName: 'stx-prod',
  globalPolicyName: 'global-oom-watch',
  prodPolicyName: 'prod-gc-alert',
  stalePolicyName: 'stale-rule-policy',
  createErrorMessage: 'policy create exploded',
};
