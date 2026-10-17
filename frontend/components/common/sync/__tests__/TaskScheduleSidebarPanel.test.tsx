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

// 推送侧栏只负责 SHELL 节点，不得带令牌管理入口。
// The push sidebar only owns the SHELL node and must not open token management.

import {render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TaskScheduleSidebarPanel} from '../TaskScheduleSidebarPanel';

const getDolphinTaskStatus = vi.fn();

vi.mock('@/lib/services/sync/sync.service', () => ({
  SyncService: {
    getDolphinTaskStatus: (...args: unknown[]) => getDolphinTaskStatus(...args),
    getDolphinScript: vi.fn(),
    unbindDolphin: vi.fn(),
  },
}));

vi.mock('@/components/common/schedule/CronExpressionEditor', () => ({
  CronExpressionEditor: () => <div data-testid='cron-editor' />,
}));

vi.mock('@/components/common/sync/DolphinPushDialog', () => ({
  DolphinPushDialog: () => null,
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('TaskScheduleSidebarPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getDolphinTaskStatus.mockResolvedValue({
      binding: {
        id: 1,
        task_id: 42,
        kind: 'dolphin',
        connection_id: 3,
        project_code: 11,
        workflow_code: 22,
        task_code: 33,
        workflow_name: 'orders',
        console_url: 'http://127.0.0.1:12345/dolphinscheduler/ui/projects/11/workflow-definition/22',
      },
      published_version: 1,
      pushed_version: 1,
      recent_jobs: [
        {
          id: 7,
          task_id: 42,
          task_version: 1,
          run_type: 'schedule',
          platform_job_id: 'p7',
          engine_job_id: '',
          status: 'failed',
          submit_spec: {trigger_source: 'dolphin'},
          result_preview: {},
          error_message: 'cluster missing',
          created_by: 1,
          created_at: '',
          updated_at: '',
          external_ref: 'ds:1001:88',
        },
      ],
      builtin_locked: true,
    });
  });

  it('keeps push actions and does not open token management', async () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <TaskScheduleSidebarPanel
          value={{enabled: false, cron_expr: '0 0 * * *', timezone: 'Asia/Shanghai'}}
          onChange={vi.fn()}
          taskId={42}
          canEdit
        />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByRole('button', {name: /更新节点/})).toBeTruthy();
    });
    expect(screen.getByRole('button', {name: /复制节点脚本/})).toBeTruthy();
    expect(screen.getByText(/已发布 v1 · 已推送 v1/)).toBeTruthy();
    expect(screen.getByText(/ds:1001:88/)).toBeTruthy();
    expect(screen.getByText(/Dolphin 调度/)).toBeTruthy();
    expect(screen.getByText(/请先发布新版本再更新节点/)).toBeTruthy();
    expect(screen.queryByRole('button', {name: /打开令牌管理/})).toBeNull();
    expect(screen.queryByText('打开令牌管理')).toBeNull();
    expect(
      document.querySelector('a[href="/admin/users?tab=tokens"]'),
    ).toBeNull();
  });
});
