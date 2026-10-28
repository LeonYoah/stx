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

// 日志面板：没拿到第一份结果前显示骨架，不能先给出“暂无日志 / 切换日志模式”；拿到空结果才显示空状态。
// Logs panel: show a skeleton before the first result instead of "no logs / switch log mode"; the empty state appears
// only for an actual empty result.

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeAll, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {SyncJobInstance, SyncJobLogsResult} from '@/lib/services/sync';
import {ConsolePanel} from '../../ConsolePanel';

const job = {
  id: 32,
  task_id: 16,
  task_version: 1,
  run_type: 'run',
  platform_job_id: '1',
  engine_job_id: '1',
  status: 'running',
  submit_spec: {cluster_id: 1},
  result_preview: {},
  error_message: '',
  created_by: 1,
  created_at: '',
} as unknown as SyncJobInstance;

function renderLogs(logsResult: SyncJobLogsResult | null, loading: boolean) {
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <ConsolePanel job={job} logsResult={logsResult} loading={loading} filterMode='all' onFilterChange={vi.fn()} onExpand={vi.fn()} />
    </NextIntlClientProvider>,
  );
}

// jsdom 没有 matchMedia；加载条的动画会查询“减少动画”，这里直接返回 true 跳过动画。
// jsdom lacks matchMedia; the loading bar animation checks reduced motion, so answer true to skip animating.
beforeAll(() => {
  window.matchMedia = vi.fn().mockReturnValue({matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn()}) as never;
});

describe('ConsolePanel logs', () => {
  it('shows a skeleton, not the empty-state guide, before the first result', () => {
    renderLogs(null, true);
    expect(screen.getByTestId('sync-job-logs-skeleton')).toBeTruthy();
    expect(screen.queryByText(zh.workbenchStudio.noLogsTitle)).toBeNull();
  });

  it('shows the empty-state guide only for an actual empty result', () => {
    renderLogs({mode: 'cluster', source: 'agent-file', logs: '', updated_at: '2026-10-28T04:00:00Z'}, false);
    expect(screen.getByText(zh.workbenchStudio.noLogsTitle)).toBeTruthy();
  });

  it('keeps rendered lines while a refresh is in flight', () => {
    renderLogs({mode: 'cluster', source: 'agent-file', logs: '2026-10-28 INFO started', updated_at: '2026-10-28T04:00:00Z'}, true);
    expect(screen.getByText('2026-10-28 INFO started')).toBeTruthy();
    expect(screen.queryByTestId('sync-job-logs-skeleton')).toBeNull();
  });
});
