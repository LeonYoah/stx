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

import {render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import en from '@/lib/i18n/locales/en.json';
import zh from '@/lib/i18n/locales/zh.json';
import type {ProcessEvent, ProcessEventType} from '@/lib/services/monitor';
import {ProcessEventList} from '../ProcessEventList';

const monitor = vi.hoisted(() => ({
  listProcessEvents: vi.fn(),
}));
vi.mock('@/lib/services/monitor', async () => {
  const actual = await vi.importActual<typeof import('@/lib/services/monitor')>(
    '@/lib/services/monitor',
  );
  return {...actual, listProcessEvents: monitor.listProcessEvents};
});

// 后端目前会下发的全部 17 种进程事件类型。
// All 17 process event types the backend currently emits.
const ALL_TYPES: ProcessEventType[] = [
  'started',
  'stopped',
  'crashed',
  'restarted',
  'restart_failed',
  'restart_limit_reached',
  'cluster_restart_requested',
  'node_restart_requested',
  'node_stop_requested',
  'node_offline',
  'node_recovered',
  'agent_offline',
  'agent_recovered',
  'master_unavailable',
  'master_recovered',
  'worker_insufficient',
  'worker_recovered',
];

const events: ProcessEvent[] = ALL_TYPES.map((type, index) => ({
  id: index + 1,
  cluster_id: 12,
  node_id: index + 1,
  host_id: index + 1,
  event_type: type,
  pid: 4000 + index,
  process_name: 'seatunnel',
  install_dir: '/opt/stx',
  role: 'worker',
  details: '',
  created_at: '2026-01-01T00:00:00Z',
  hostname: `node-${index + 1}`,
  ip: `10.0.0.${index + 1}`,
}));

const renderList = (locale: 'zh' | 'en') =>
  render(
    <NextIntlClientProvider
      locale={locale}
      messages={locale === 'zh' ? zh : en}
      timeZone='UTC'
    >
      <ProcessEventList clusterId={12} />
    </NextIntlClientProvider>,
  );

describe('ProcessEventList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    monitor.listProcessEvents.mockResolvedValue({
      events,
      total: events.length,
      page: 1,
      page_size: 20,
    });
  });

  it('localizes all 17 event types and never leaks raw snake_case (zh)', async () => {
    renderList('zh');
    const marker = await screen.findByText('Agent 离线');
    const table = marker.closest('table');
    expect(table).not.toBeNull();
    const rows = within(table as HTMLElement);
    for (const text of [
      'Agent 已恢复',
      'Master 不可用',
      'Master 已恢复',
      '健康 Worker 不足',
      'Worker 已恢复',
    ]) {
      expect(rows.getByText(text)).toBeTruthy();
    }
    // 17 条数据行 + 1 条表头行。/ 17 data rows + 1 header row.
    expect(rows.getAllByRole('row')).toHaveLength(ALL_TYPES.length + 1);
    for (const type of ALL_TYPES) {
      expect(rows.queryByText(type)).toBeNull();
    }
  });

  it('localizes all 17 event types and never leaks raw snake_case (en)', async () => {
    renderList('en');
    const marker = await screen.findByText('Agent Offline');
    const table = marker.closest('table');
    expect(table).not.toBeNull();
    const rows = within(table as HTMLElement);
    for (const text of [
      'Agent Recovered',
      'Master Unavailable',
      'Master Recovered',
      'Workers Insufficient',
      'Worker Recovered',
    ]) {
      expect(rows.getByText(text)).toBeTruthy();
    }
    for (const type of ALL_TYPES) {
      expect(rows.queryByText(type)).toBeNull();
    }
  });
});
