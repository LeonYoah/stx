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
  HostDetail: ({
    host,
    editing,
    onStartEdit,
    onCancelEdit,
    onDirtyChange,
  }: {
    host: HostInfo | null;
    editing: boolean;
    onStartEdit: () => void;
    onCancelEdit: () => void;
    onDirtyChange?: (dirty: boolean) => void;
  }) => (
    <div data-testid='host-detail-body'>
      <span>{host ? `${host.name}:${host.description || ''}` : 'empty'}</span>
      <span data-testid='host-detail-mode'>{editing ? 'editing' : 'view'}</span>
      {editing ? (
        <>
          <button
            type='button'
            data-testid='mock-mark-dirty'
            onClick={() => onDirtyChange?.(true)}
          >
            mark-dirty
          </button>
          <button type='button' onClick={onCancelEdit}>
            取消编辑
          </button>
        </>
      ) : (
        <button type='button' onClick={onStartEdit}>
          进入编辑
        </button>
      )}
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

  it('行菜单编辑直接在右侧 DetailHost 原地进入编辑态，未保存切行弹出 DirtyCloseDialog 拦截', async () => {
    renderHostMain();

    fireEvent.click(await screen.findByTestId('host-row-1'));
    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-alpha:alpha',
      ),
    );
    expect(screen.getByTestId('host-detail-mode')).toHaveTextContent('view');

    // 从行菜单点击另一台主机的编辑，右侧栏直接打开该主机编辑态，不再叠弹窗
    // Row menu Edit switches DetailHost directly into edit mode for that host (no stacked modal)
    const rowB = screen.getByTestId('host-row-2');
    const moreBtn = within(rowB).getByRole('button', {name: '更多操作'});
    fireEvent.pointerDown(moreBtn, {button: 0, ctrlKey: false});
    fireEvent.click(moreBtn);
    const editItem = await screen.findByRole('menuitem', {name: '编辑'});
    fireEvent.click(editItem);

    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-beta:beta',
      ),
    );
    expect(screen.getByTestId('host-detail-mode')).toHaveTextContent('editing');

    // 标脏后切回行 1，应弹出未保存确认弹窗；放弃修改后才切换
    // Switching rows while dirty opens DirtyCloseDialog; discarding completes the switch
    fireEvent.click(screen.getByTestId('mock-mark-dirty'));
    fireEvent.click(screen.getByTestId('host-row-1'));
    const dirtyDialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dirtyDialog).getByRole('button', {name: zh.common.dirtyClose.discard}));

    await waitFor(() =>
      expect(screen.getByTestId('host-detail-body')).toHaveTextContent(
        'node-alpha:alpha',
      ),
    );
    expect(screen.getByTestId('host-detail-mode')).toHaveTextContent('view');
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

  it('待连接的 Kubernetes 主机展示待连接与版本/命名空间，不显示未安装与 CPU 0.0%', async () => {
    const k8sPending = makeHost({
      id: 13,
      name: 'orbstack-k8s',
      host_type: HostType.KUBERNETES,
      status: HostStatus.PENDING,
      agent_status: AgentStatus.NOT_INSTALLED,
      k8s_api_url: 'https://127.0.0.1:26443',
      k8s_namespace: 'seatunnel-min',
      k8s_version: '',
      cpu_usage: 0,
      memory_usage: 0,
      disk_usage: 0,
      is_online: false,
    });
    getHostsSafe.mockResolvedValueOnce({
      success: true,
      data: {hosts: [k8sPending], total: 1},
    });

    renderHostMain();
    const row = await screen.findByTestId('host-row-13');
    expect(within(row).getByText(zh.host.statuses.pendingConnection)).toBeInTheDocument();
    expect(within(row).queryByText(zh.host.statuses.pending)).not.toBeInTheDocument();
    expect(within(row).getByText('ns: seatunnel-min')).toBeInTheDocument();
    expect(within(row).queryByText(/CPU/)).not.toBeInTheDocument();
  });
});
