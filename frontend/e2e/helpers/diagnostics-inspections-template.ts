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

type InspectionReport = {
  id: number;
  cluster_id: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  trigger_source: string;
  lookback_minutes: number;
  error_threshold: number;
  requested_by_user_id: number;
  requested_by: string;
  summary: string;
  error_message: string;
  finding_total: number;
  critical_count: number;
  warning_count: number;
  info_count: number;
  started_at: string;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
  cluster_name?: string;
};

const now = '2026-11-20T10:00:00Z';
const prodClusterId = 1;

function buildReport(overrides: Partial<InspectionReport>): InspectionReport {
  return {
    id: 1,
    cluster_id: prodClusterId,
    status: 'completed',
    trigger_source: 'diagnostics_workspace',
    lookback_minutes: 30,
    error_threshold: 1,
    requested_by_user_id: 1,
    requested_by: 'admin',
    summary: 'inspection done',
    error_message: '',
    finding_total: 0,
    critical_count: 0,
    warning_count: 0,
    info_count: 0,
    started_at: now,
    finished_at: now,
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

function buildFinding(reportId: number, overrides?: object) {
  return {
    id: 100 + reportId,
    report_id: reportId,
    cluster_id: prodClusterId,
    severity: 'warning',
    category: 'errors',
    check_code: 'ERR_BURST',
    check_name: '错误突增检查',
    summary: 'worker-a 上错误激增',
    recommendation: '查看 engine 日志',
    evidence_summary: 'engine.log 中出现 15 次异常',
    related_node_id: 11,
    related_host_id: 21,
    related_host_name: 'worker-a',
    related_host_ip: '10.0.0.21',
    related_error_group_id: 0,
    related_alert_id: '',
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

function buildTask(reportId: number) {
  return {
    id: 900 + reportId,
    cluster_id: prodClusterId,
    trigger_source: 'inspection_finding',
    source_ref: {inspection_report_id: reportId},
    options: {include_thread_dump: false, include_jvm_dump: false},
    status: 'running',
    current_step: 'COLLECT_LOG_SAMPLE',
    failure_step: 'COLLECT_LOG_SAMPLE',
    failure_reason: '',
    selected_nodes: [],
    summary: '',
    bundle_dir: '',
    manifest_path: '',
    index_path: '',
    created_by: 1,
    created_by_name: 'admin',
    created_at: now,
    updated_at: now,
    steps: [
      {
        id: 1,
        task_id: 900 + reportId,
        code: 'COLLECT_LOG_SAMPLE',
        sequence: 1,
        title: '采集日志样本',
        description: 'collect log sample',
        status: 'running',
        message: '',
        error: '',
        created_at: now,
        updated_at: now,
      },
    ],
    node_executions: [],
  };
}

function buildResources() {
  return [
    {
      code: 'error_context',
      title: '错误上下文',
      description: 'collect error context',
      step_code: 'COLLECT_ERROR_CONTEXT',
      risk: 'R0',
      impact: '低',
      timeout_seconds: 30,
      execution_location: 'agent',
      result_type: 'json',
      bundle_allowed: true,
      admin_only: false,
      default_selected: true,
    },
    {
      code: 'log_sample',
      title: '日志样本',
      description: 'collect log sample',
      step_code: 'COLLECT_LOG_SAMPLE',
      risk: 'R1',
      impact: '低',
      timeout_seconds: 60,
      execution_location: 'agent',
      result_type: 'text',
      bundle_allowed: true,
      admin_only: false,
      default_selected: true,
    },
  ];
}

function createInitialState() {
  return {
    nextReportId: 10,
    reports: [
      buildReport({
        id: 1,
        status: 'completed',
        finding_total: 2,
        critical_count: 1,
        warning_count: 1,
        summary: 'hot report with findings',
      }),
      buildReport({
        id: 2,
        status: 'failed',
        error_message: 'engine unreachable',
        summary: 'inspection crashed',
      }),
      buildReport({
        id: 3,
        status: 'completed',
        cluster_id: 2,
        cluster_name: 'stx-dev',
        summary: 'dev cluster report',
      }),
    ] as InspectionReport[],
  };
}

/**
 * 诊断工作台 / 巡检报告接口的模板路由桩：bootstrap、报告列表（识别状态、集群与
 * 分页参数）、报告详情、发起巡检、诊断任务与资源目录走夹具状态，其余 /api/v1
 * 请求放行给真实后端（登录态等）。
 * Route stubs for the diagnostics workspace / inspection report endpoints
 * backed by a stateful fixture — the list honours status, cluster and
 * pagination params; every other /api/v1 request passes through to the real
 * backend (auth, layout, etc.).
 */
export async function installDiagnosticsInspectionsTemplateRoutes(
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
          default_tab: 'inspections',
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

    if (method === 'GET' && pathname === '/api/v1/diagnostics/inspections') {
      const status = url.searchParams.get('status') ?? '';
      const clusterId = Number(url.searchParams.get('cluster_id') ?? '');
      const pageNo = Number(url.searchParams.get('page') ?? '1') || 1;
      const pageSize =
        Number(url.searchParams.get('page_size') ?? '20') || 20;
      let items = state.reports;
      if (status) {
        items = items.filter((r) => r.status === status);
      }
      if (!Number.isNaN(clusterId) && url.searchParams.has('cluster_id')) {
        items = items.filter((r) => r.cluster_id === clusterId);
      }
      const total = items.length;
      items = items.slice((pageNo - 1) * pageSize, pageNo * pageSize);
      await route.fulfill({
        status: 200,
        json: ok({items, total, page: pageNo, page_size: pageSize}),
      });
      return;
    }

    if (method === 'POST' && pathname === '/api/v1/diagnostics/inspections') {
      const body = request.postDataJSON() as {cluster_id?: number};
      const newReport = buildReport({
        id: state.nextReportId,
        status: 'completed',
        cluster_id: body.cluster_id ?? prodClusterId,
        finding_total: 1,
        info_count: 1,
        summary: 'fresh inspection report',
      });
      state.reports.unshift(newReport);
      state.nextReportId += 1;
      await route.fulfill({
        status: 200,
        json: ok({
          report: newReport,
          findings: [buildFinding(newReport.id)],
          related_diagnostic_task: null,
        }),
      });
      return;
    }

    const inspectionMatch = pathname.match(
      /^\/api\/v1\/diagnostics\/inspections\/(\d+)$/,
    );
    if (inspectionMatch && method === 'GET') {
      const id = Number(inspectionMatch[1]);
      const report = state.reports.find((r) => r.id === id);
      if (!report) {
        await route.fulfill({
          status: 404,
          json: {error_msg: 'inspection report not found', data: null},
        });
        return;
      }
      await route.fulfill({
        status: 200,
        json: ok({
          report,
          findings: report.finding_total
            ? [
                buildFinding(report.id, {severity: 'critical'}),
                buildFinding(report.id + 1, {severity: 'warning'}),
              ]
            : [],
          related_diagnostic_task: null,
        }),
      });
      return;
    }

    if (method === 'POST' && pathname === '/api/v1/diagnostics/tasks') {
      await route.fulfill({status: 200, json: ok(buildTask(1))});
      return;
    }

    const taskMatch = pathname.match(/^\/api\/v1\/diagnostics\/tasks\/(\d+)$/);
    if (taskMatch && method === 'GET') {
      await route.fulfill({
        status: 200,
        json: ok(buildTask(Number(taskMatch[1]))),
      });
      return;
    }

    if (method === 'GET' && pathname === '/api/v1/diagnostics/resources') {
      await route.fulfill({status: 200, json: ok(buildResources())});
      return;
    }

    await route.continue();
  });

  return state;
}

export const diagnosticsInspectionsTemplate = {
  listErrorMessage: 'inspection list exploded',
};
