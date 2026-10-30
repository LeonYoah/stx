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

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TaskScheduleSidebarPanel} from '../TaskScheduleSidebarPanel';

// Radix Tooltip 打开时测量尺寸，jsdom 没有 ResizeObserver，这里给个空实现
// Radix Tooltip measures size when open; jsdom lacks ResizeObserver, so stub it
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

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

  it('moves schedule copy into the badge tooltip and shows the unsaved hint only when dirty', async () => {
    const renderPanel = (hasUnsavedChanges: boolean) => (
      <NextIntlClientProvider locale='zh' messages={zh}>
        <TaskScheduleSidebarPanel
          value={{enabled: false, cron_expr: '0 0 * * *', timezone: 'Asia/Shanghai'}}
          onChange={vi.fn()}
          taskId={42}
          canEdit
          hasUnsavedChanges={hasUnsavedChanges}
        />
      </NextIntlClientProvider>
    );
    const {rerender} = render(renderPanel(false));

    await waitFor(() => {
      expect(screen.getByRole('button', {name: /更新节点/})).toBeTruthy();
    });
    const ws = zh.workbenchStudio as unknown as Record<string, string>;
    // 常驻长段建议与保存规则不再出现 / Long advice and save rule are no longer persistent
    expect(screen.queryByText(ws.scheduleProductionNoticeDesc)).toBeNull();
    expect(screen.queryByText(ws.scheduleUnsavedHint)).toBeNull();
    expect(screen.queryByText(ws.dolphinBuiltinLocked)).toBeNull();
    expect(screen.getByText(ws.scheduleLightweightBadge)).toBeTruthy();
    // 绑定 Dolphin 时开关禁用 / Switch is disabled while bound to Dolphin
    expect(screen.getByRole('switch').hasAttribute('disabled')).toBe(true);

    rerender(renderPanel(true));
    const unsavedHint = screen.getByText(ws.scheduleUnsavedHint);
    // 未保存提示允许换行，不截断 / The unsaved hint wraps instead of truncating
    expect(unsavedHint.className).not.toContain('truncate');
  });

  it('opens the lightweight badge and locked switch tooltips via keyboard focus', async () => {
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
    const ws = zh.workbenchStudio as unknown as Record<string, string>;

    // 「轻量」Badge 包在 button 里，可访问名称就是可见文字；聚焦后弹出生产调度建议
    // The "Lightweight" badge sits in a button named by its visible text; focus opens the production advice
    const badgeButton = screen.getByRole('button', {name: ws.scheduleLightweightBadge});
    expect(badgeButton.getAttribute('type')).toBe('button');
    fireEvent.focus(badgeButton);
    await waitFor(() => {
      expect(screen.getAllByText(ws.scheduleProductionNoticeDesc).length).toBeGreaterThan(0);
    });

    // 已绑定 Dolphin 时禁用开关外层 span 可聚焦，聚焦后弹出锁定原因
    // While bound to Dolphin the disabled switch's wrapper is focusable and opens the lock reason
    const switchWrapper = screen.getByRole('switch').parentElement as HTMLElement;
    expect(switchWrapper.getAttribute('tabindex')).toBe('0');
    fireEvent.focus(switchWrapper);
    await waitFor(() => {
      expect(screen.getAllByText(ws.dolphinBuiltinLocked).length).toBeGreaterThan(0);
    });
  });
});
