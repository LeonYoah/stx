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

// 主机列表与常驻详情核心交互验证：详情竞态保护、编辑不覆写其他详情、刷新失败保旧行、批量失败保留勾选。
// Core HostMain interaction tests: detail race guard, editing another host does not clobber detail,
// failed reloads keep existing rows, and partial/failed batch delete retains failed selections.

import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {
  AgentStatus,
  HostStatus,
  HostType,
  type HostInfo,
} from '@/lib/services/host/types';
import {HostMain} from '../HostMain';

const replace = vi.hoisted(() => vi.fn());
const getHostsSafe = vi.hoisted(() => vi.fn());
const getHostSafe = vi.hoisted(() => vi.fn());
const updateHostSafe = vi.hoisted(() => vi.fn());
const deleteHostSafe = vi.hoisted(() => vi.fn());
const toastSuccess = vi.hoisted(() => vi.fn());
const toastWarning = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn(), replace}),
  usePathname: () => '/hosts',
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock('@/lib/services', () => ({
  default: {
    host: {
      getHostsSafe,
      getHostSafe,
      updateHostSafe,
      deleteHostSafe,
    },
  },
}));

vi.mock('@/lib/toast', () => ({
  toast: {
    success: toastSuccess,
    warning: toastWarning,
    error: toastError,
    message: vi.fn(),
  },
}));

vi.mock('../HostDetail', () => ({
  HostDetail: ({host}: {host: HostInfo | null}) => (
    <div data-testid='host-detail-body'>
      {host ? `${host.name}:${host.description || ''}` : 'empty'}
    </div>
  ),
}));

function makeHost(overrides: Partial<HostInfo> & {id: number; name: string}): HostInfo {
  return {
    host_type: HostType.BARE_METAL,
    description: '',
    ip_address: `10.0.0.${overrides.id}`,
    status: HostStatus.CONNECTED,
    agent_status: AgentStatus.INSTALLED,
    cpu_usage: 12,
    memory_usage: 34,
    disk_usage: 56,
    last_check: null,
    is_online: true,
    created_at: '2026-11-20T08:00:00Z',
    updated_at: '2026-11-20T08:00:00Z',
    ...overrides,
  };
}

function renderHostMain(initialHostId?: number | null) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <HostMain initialHostId={initialHostId} />
    </NextIntlClientProvider>,
  );
}

describe('HostMain', () => {
  const hostA = makeHost({id: 1, name: 'node-alpha', description: 'alpha'});
  const hostB = makeHost({id: 2, name: 'node-beta', description: 'beta'});

  beforeEach(() => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
    replace.mockReset();
    getHostsSafe.mockReset();
    getHostSafe.mockReset();
    updateHostSafe.mockReset();
    deleteHostSafe.mockReset();
    toastSuccess.mockReset();
    toastWarning.mockReset();
    toastError.mockReset();

    getHostsSafe.mockResolvedValue({
      success: true,
      data: {hosts: [hostA, hostB], total: 2},
    });
    getHostSafe.mockImplementation(async (id: number) => ({
      success: true,
      data: id === 1 ? hostA : hostB,
    }));
  });

  it('快速切换主机时，慢返回的旧详情不会覆盖新选中主机', async () => {
    let resolveFirst!: (value: {success: true; data: HostInfo}) => void;
    getHostSafe.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    getHostSafe.mockResolvedValueOnce({
      success: true,
      data: {...hostB, description: 'beta-fresh'},
    });

    renderHostMain();
    fireEvent.click(await screen.findByTestId('host-row-1'));
    fireEvent.click(screen.getByTestId('host-row-2'));

    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-beta:beta-fresh',
      ),
    );

    resolveFirst({
      success: true,
      data: {...hostA, description: 'alpha-stale'},
    });

    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-beta:beta-fresh',
      ),
    );
  });

  it('编辑另一台主机后不会覆写当前打开的详情栏', async () => {
    updateHostSafe.mockResolvedValue({success: true});
    renderHostMain();

    fireEvent.click(await screen.findByTestId('host-row-1'));
    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-alpha:alpha',
      ),
    );

    const rowB = screen.getByTestId('host-row-2');
    const moreBtn = within(rowB).getByRole('button', {name: '更多操作'});
    fireEvent.pointerDown(moreBtn, {button: 0, ctrlKey: false});
    fireEvent.click(moreBtn);
    const editItem = await screen.findByRole('menuitem', {name: '编辑'});
    fireEvent.click(editItem);
    const saveBtn = await screen.findByRole('button', {name: '保存'});
    fireEvent.click(saveBtn);

    await waitFor(() => expect(updateHostSafe).toHaveBeenCalledWith(2, expect.any(Object)));
    expect(getHostSafe).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
      'node-alpha:alpha',
    );
  });

  it('刷新失败时保留已有列表行，只显示错误提示', async () => {
    renderHostMain();
    expect(await screen.findByTestId('host-row-1')).toBeInTheDocument();

    getHostsSafe.mockResolvedValueOnce({
      success: false,
      error: 'network timeout',
    });
    fireEvent.click(screen.getByRole('button', {name: '刷新'}));

    await waitFor(() =>
      expect(screen.getByText('network timeout')).toBeInTheDocument(),
    );
    expect(screen.getByTestId('host-row-1')).toBeInTheDocument();
  });

  it('批量删除部分失败时保留失败项勾选并弹出 warning', async () => {
    deleteHostSafe
      .mockResolvedValueOnce({success: true})
      .mockResolvedValueOnce({success: false, error: 'in use'});

    renderHostMain();
    const rowA = await screen.findByTestId('host-row-1');
    const rowB = screen.getByTestId('host-row-2');
    const checkA = within(rowA).getByRole('checkbox');
    const checkB = within(rowB).getByRole('checkbox');

    fireEvent.click(checkA);
    fireEvent.click(checkB);

    fireEvent.click(screen.getByRole('button', {name: '批量删除'}));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', {name: '删除'}));

    await waitFor(() => expect(toastWarning).toHaveBeenCalled());
    expect(toastSuccess).not.toHaveBeenCalled();
    expect(checkA).not.toBeChecked();
    expect(checkB).toBeChecked();
  });
});
