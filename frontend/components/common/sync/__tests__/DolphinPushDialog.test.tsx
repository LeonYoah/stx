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

// 推送弹窗必须把 Dolphin Open API Token 做成可见必填，而不是藏在 placeholder 里。
// The push dialog must show a labeled required Dolphin Open API token, not a hidden placeholder.

import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {DolphinPushDialog} from '../DolphinPushDialog';

const listSchedulerConnections = vi.fn();
const createSchedulerConnection = vi.fn();
const listDolphinProjects = vi.fn();
const pushDolphin = vi.fn();

// Radix Popper 依赖 ResizeObserver，jsdom 未提供 / Radix Popper needs ResizeObserver, which jsdom lacks
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

vi.mock('@/lib/services/sync/sync.service', () => ({
  SyncService: {
    listSchedulerConnections: (...args: unknown[]) =>
      listSchedulerConnections(...args),
    createSchedulerConnection: (...args: unknown[]) =>
      createSchedulerConnection(...args),
    listDolphinProjects: (...args: unknown[]) => listDolphinProjects(...args),
    listDolphinWorkflows: vi.fn().mockResolvedValue([]),
    getDolphinWorkflow: vi.fn().mockResolvedValue(null),
    pushDolphin: (...args: unknown[]) => pushDolphin(...args),
  },
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

function renderDialog(status?: {
  published_version?: number;
  pushed_version?: number;
  binding?: {id: number} | null;
}) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <DolphinPushDialog
        open
        taskId={2}
        onOpenChange={vi.fn()}
        status={status as never}
      />
    </NextIntlClientProvider>,
  );
}

describe('DolphinPushDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listDolphinProjects.mockResolvedValue([]);
    createSchedulerConnection.mockResolvedValue({
      id: 9,
      name: 'ds-live',
      token_set: true,
    });
  });

  it('shows a labeled required Open API token when no connection exists', async () => {
    listSchedulerConnections.mockResolvedValue({items: []});
    renderDialog();

    expect(
      await screen.findByLabelText(/Dolphin Open API Token/),
    ).toBeTruthy();
    expect(screen.getByTestId('dolphin-open-api-token')).toHaveAttribute(
      'type',
      'password',
    );
    expect(
      screen.getByText(/调用 Dolphin Open API 必填/),
    ).toBeTruthy();
    expect(screen.queryByText('选择连接')).toBeNull();
  });

  it('refuses to save a connection without the Open API token', async () => {
    const {toast} = await import('sonner');
    listSchedulerConnections.mockResolvedValue({items: []});
    renderDialog();

    fireEvent.change(await screen.findByLabelText('连接名称'), {
      target: {value: 'ds-live'},
    });
    fireEvent.change(screen.getByLabelText(/Open API 地址/), {
      target: {value: 'http://127.0.0.1:12345/dolphinscheduler'},
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', {name: '保存连接'}));
    });

    expect(createSchedulerConnection).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalledWith('请填写 Dolphin Open API Token');
  });

  it('submits the Open API token when creating a connection', async () => {
    listSchedulerConnections.mockResolvedValue({items: []});
    renderDialog();

    fireEvent.change(await screen.findByLabelText('连接名称'), {
      target: {value: 'ds-live'},
    });
    fireEvent.change(screen.getByLabelText(/Open API 地址/), {
      target: {value: 'http://127.0.0.1:12345/dolphinscheduler'},
    });
    fireEvent.change(screen.getByTestId('dolphin-open-api-token'), {
      target: {value: 'ds-open-api-token'},
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', {name: '保存连接'}));
    });

    await waitFor(() => {
      expect(createSchedulerConnection).toHaveBeenCalledWith({
        name: 'ds-live',
        base_url: 'http://127.0.0.1:12345/dolphinscheduler',
        token: 'ds-open-api-token',
        dolphin_version: undefined,
      });
    });
  });

  it('blocks push with aria-disabled and a reason tooltip until a new published version exists', async () => {
    listSchedulerConnections.mockResolvedValue({items: []});
    renderDialog({
      published_version: 1,
      pushed_version: 1,
      binding: {id: 4},
    });

    expect(
      await screen.findByText(/已推送 v1，请先发布新版本再更新节点/),
    ).toBeTruthy();
    const dryRun = screen.getByRole('button', {name: '预演'});
    const confirm = screen.getByRole('button', {name: '确认推送'});
    // aria-disabled 而不是原生 disabled，按钮仍可聚焦以显示原因 / aria-disabled keeps the buttons focusable for the reason tooltip
    expect(dryRun).toHaveAttribute('aria-disabled', 'true');
    expect(confirm).toHaveAttribute('aria-disabled', 'true');
    expect(confirm).not.toBeDisabled();
    fireEvent.click(confirm);
    fireEvent.click(dryRun);
    expect(pushDolphin).not.toHaveBeenCalled();
    await act(async () => {
      confirm.focus();
      fireEvent.focus(confirm);
    });
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      /已推送 v1，请先发布新版本再更新节点/,
    );
  });
});
