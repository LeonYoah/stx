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

type ErrorGroup = {
  id: number;
  fingerprint: string;
  fingerprint_version: string;
  title: string;
  exception_class: string;
  sample_message: string;
  occurrence_count: number;
  first_seen_at: string;
  last_seen_at: string;
  last_cluster_id: number;
  last_node_id: number;
  last_host_id: number;
  last_host_name?: string;
  last_host_ip?: string;
};

const now = '2026-11-20T10:00:00Z';
const prodClusterId = 1;

function buildGroup(overrides: Partial<ErrorGroup>): ErrorGroup {
  return {
    id: 1,
    fingerprint: 'fp-1',
    fingerprint_version: 'v1',
    title: 'group',
    exception_class: 'org.apache.seatunnel.TaskExecuteException',
    sample_message: 'job failed on worker',
    occurrence_count: 1,
    first_seen_at: now,
    last_seen_at: now,
    last_cluster_id: prodClusterId,
    last_node_id: 11,
    last_host_id: 21,
    last_host_name: 'worker-a',
    last_host_ip: '10.0.0.21',
    ...overrides,
  };
}

function buildEvent(groupId: number) {
  return {
    id: 100 + groupId,
    error_group_id: groupId,
    fingerprint: `fp-${groupId}`,
    cluster_id: prodClusterId,
    node_id: 11,
    host_id: 21,
    host_name: 'worker-a',
    host_ip: '10.0.0.21',
    agent_id: 'agent-a',
    role: 'worker',
    install_dir: '/opt/seatunnel',
    source_file: '/opt/seatunnel/logs/engine.log',
    source_kind: 'engine',
    job_id: '100',
    occurred_at: now,
    message: 'boom',
    exception_class: 'org.apache.seatunnel.TaskExecuteException',
    evidence:
      'java.lang.RuntimeException: boom\n\tat org.apache.seatunnel.Engine.run(Engine.java:1)',
    cursor_start: 0,
    cursor_end: 10,
  };
}

function createInitialState() {
  return {
    groups: [
      buildGroup({
        id: 1,
        fingerprint: 'fp-1',
        title: 'hot-oom-failure',
        occurrence_count: 15,
      }),
      buildGroup({
        id: 2,
        fingerprint: 'fp-2',
        title: 'mid-timeout-failure',
        occurrence_count: 5,
      }),
      buildGroup({
        id: 3,
        fingerprint: 'fp-3',
        title: 'cold-single-failure',
        occurrence_count: 1,
        last_cluster_id: 2,
      }),
    ] as ErrorGroup[],
  };
}

/**
 * 诊断工作台 / 错误组接口的模板路由桩：bootstrap、错误组列表（识别频次与关键字
 * 参数）、错误组详情走夹具状态，其余 /api/v1 请求放行给真实后端（登录态等）。
 * Route stubs for the diagnostics workspace / error group endpoints backed by a
 * stateful fixture — the list honours occurrence and keyword params; every other
 * /api/v1 request passes through to the real backend (auth, layout, etc.).
 */
export async function installDiagnosticsErrorsTemplateRoutes(
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

    if (method === 'GET' && pathname === '/api/v1/diagnostics/errors/groups') {
      const minOcc = Number(url.searchParams.get('min_occurrences') ?? '');
      const maxOcc = Number(url.searchParams.get('max_occurrences') ?? '');
      const keyword = (url.searchParams.get('keyword') ?? '').toLowerCase();
      const pageNo = Number(url.searchParams.get('page') ?? '1') || 1;
      const pageSize =
        Number(url.searchParams.get('page_size') ?? '20') || 20;
      let items = state.groups;
      if (!Number.isNaN(minOcc) && url.searchParams.has('min_occurrences')) {
        items = items.filter((g) => g.occurrence_count >= minOcc);
      }
      if (!Number.isNaN(maxOcc) && url.searchParams.has('max_occurrences')) {
        items = items.filter((g) => g.occurrence_count <= maxOcc);
      }
      if (keyword) {
        items = items.filter(
          (g) =>
            g.title.toLowerCase().includes(keyword) ||
            g.exception_class.toLowerCase().includes(keyword) ||
            g.sample_message.toLowerCase().includes(keyword),
        );
      }
      const total = items.length;
      items = items.slice((pageNo - 1) * pageSize, pageNo * pageSize);
      await route.fulfill({
        status: 200,
        json: ok({items, total, page: pageNo, page_size: pageSize}),
      });
      return;
    }

    const groupMatch = pathname.match(
      /^\/api\/v1\/diagnostics\/errors\/groups\/(\d+)$/,
    );
    if (groupMatch && method === 'GET') {
      const id = Number(groupMatch[1]);
      const group = state.groups.find((g) => g.id === id);
      if (!group) {
        await route.fulfill({
          status: 404,
          json: {error_msg: 'error group not found', data: null},
        });
        return;
      }
      await route.fulfill({
        status: 200,
        json: ok({group, events: [buildEvent(id)]}),
      });
      return;
    }

    await route.continue();
  });

  return state;
}

export const diagnosticsErrorsTemplate = {
  listErrorMessage: 'group list exploded',
};
