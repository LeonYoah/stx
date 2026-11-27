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

// ContainerRegistryPanel 单元测试：覆盖管理员权限门控、新建 Dialog 与探测失败阻断、
// 编辑 DetailHost + docker.io 锁定 + 脏关闭确认、BatchBar + AlertDialog 批量删除。
// Unit tests for ContainerRegistryPanel: admin gating, centered create Dialog & failing
// probe semantics, DetailHost edit rail + locked docker.io host + dirty close guard,
// and BatchBar + AlertDialog bulk deletion.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import layout from '@/components/common/layout/list-detail.module.css';
import {ContainerRegistryPanel} from '../ContainerRegistryPanel';

const authState = vi.hoisted(() => ({
  user: {id: 1, username: 'admin', is_admin: true} as {id: number; username: string; is_admin: boolean} | null,
}));

const serviceMocks = vi.hoisted(() => ({
  listContainerRegistries: vi.fn(),
  createContainerRegistry: vi.fn(),
  updateContainerRegistry: vi.fn(),
  deleteContainerRegistry: vi.fn(),
  testContainerRegistry: vi.fn(),
  probeRegistry: vi.fn(),
}));

const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}));

vi.mock('@/hooks/use-auth', () => ({
  useAuth: () => ({user: authState.user}),
}));

vi.mock('@/lib/toast', () => ({
  toast: toastMocks,
}));

vi.mock('@/lib/services/installer/registry.service', () => ({
  listContainerRegistries: serviceMocks.listContainerRegistries,
  createContainerRegistry: serviceMocks.createContainerRegistry,
  updateContainerRegistry: serviceMocks.updateContainerRegistry,
  deleteContainerRegistry: serviceMocks.deleteContainerRegistry,
  testContainerRegistry: serviceMocks.testContainerRegistry,
  probeRegistry: serviceMocks.probeRegistry,
}));

const SAMPLE_ROWS = [
  {
    id: 1,
    host: 'docker.io',
    default_repo: 'apache/seatunnel',
    is_system: true,
    has_auth: false,
    probe_status: 'public',
  },
  {
    id: 2,
    host: 'harbor.corp.local',
    default_repo: 'data/seatunnel',
    username: 'ci',
    is_system: false,
    has_auth: true,
    probe_status: 'ok',
  },
  {
    id: 3,
    host: 'broken.corp.local',
    is_system: false,
    has_auth: false,
    probe_status: 'error',
    probe_error: 'dial tcp timeout',
  },
];

describe('ContainerRegistryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
    authState.user = {id: 1, username: 'admin', is_admin: true};
    serviceMocks.listContainerRegistries.mockResolvedValue(SAMPLE_ROWS);
  });

  it('hides admin mutation actions for non-admin users', async () => {
    authState.user = {id: 2, username: 'viewer', is_admin: false};
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    expect(await screen.findByTestId('registry-readonly-hint')).toBeInTheDocument();
    expect(screen.queryByTestId('registry-add')).not.toBeInTheDocument();
    expect(screen.queryByTestId('registry-open-2')).not.toBeInTheDocument();
    expect(screen.queryByTestId('registry-test-2')).not.toBeInTheDocument();
  });

  it('creates via centered Dialog and reports failed transient probe instead of green success', async () => {
    serviceMocks.probeRegistry.mockResolvedValue({status: 'error', error: '401 Unauthorized'});
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    fireEvent.click(await screen.findByTestId('registry-add'));
    expect(await screen.findByTestId('registry-create-dialog')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('registry-host-input'), {target: {value: 'reg.internal:5000'}});
    fireEvent.click(screen.getByTestId('registry-probe'));

    await waitFor(() => {
      expect(screen.getByTestId('registry-probe-result')).toHaveTextContent('401 Unauthorized');
    });
  });

  it('opens DetailHost for edit, locks docker.io host, passes id on probe, and guards dirty close', async () => {
    serviceMocks.probeRegistry.mockResolvedValue({status: 'ok'});
    serviceMocks.updateContainerRegistry.mockResolvedValue({...SAMPLE_ROWS[0], has_auth: true, username: 'hubuser'});

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    fireEvent.click(await screen.findByTestId('registry-open-1'));
    expect(screen.getByTestId('registry-edit-host-input')).toBeDisabled();
    expect(screen.getByTestId('registry-host-immutable')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('registry-edit-username-input'), {target: {value: 'hubuser'}});
    fireEvent.click(screen.getByTestId('registry-edit-probe'));

    await waitFor(() => {
      expect(serviceMocks.probeRegistry).toHaveBeenCalledWith({
        id: 1,
        host: 'docker.io',
        default_repo: 'apache/seatunnel',
        username: 'hubuser',
        password: '',
      });
    });

    // 关闭未保存抽屉应触发 DirtyCloseDialog / Closing dirty drawer prompts DirtyCloseDialog
    fireEvent.click(screen.getByRole('button', {name: zh.packages.registry.cancel}));
    expect(await screen.findByText(zh.common.dirtyClose.title)).toBeInTheDocument();
  });

  it('skips docker.io and reports partial failure via toast on bulk delete', async () => {
    serviceMocks.deleteContainerRegistry
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('boom'));

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    await screen.findByTestId('registry-open-1');
    const checkboxes = screen.getAllByRole('checkbox');
    // 全选表头复选框 / Click header select-all checkbox
    fireEvent.click(checkboxes[0]);

    const batchBar = await screen.findByTestId('registry-batch-bar');
    expect(batchBar).toHaveTextContent(zh.packages.registry.systemSkippedHint);
    fireEvent.click(screen.getByTestId('registry-batch-delete'));
    fireEvent.click(await screen.findByTestId('registry-batch-confirm'));

    await waitFor(() => {
      expect(serviceMocks.deleteContainerRegistry).toHaveBeenCalledTimes(2);
    });
    expect(serviceMocks.deleteContainerRegistry).not.toHaveBeenCalledWith(1);
    expect(toastMocks.warning).toHaveBeenCalledTimes(1);
  });

  it('activates #178 linked list-detail row state, fly-in markers, and prev/next navigation with dirty guard', async () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    const row1 = await screen.findByTestId('registry-row-1');
    const row2 = screen.getByTestId('registry-row-2');
    // 列表行应携带 data-detail-name 以支撑 DetailHost 名称飞入。
    // Rows must carry data-detail-name so DetailHost can animate name fly-in.
    expect(row1.querySelector('[data-detail-name]')?.textContent).toBe('docker.io');

    // 点击整行打开常驻右侧栏，并挂载 .rowActive 触发零间距联动样式。
    // Clicking the row opens the persistent rail and applies .rowActive for zero-gap linkage.
    fireEvent.click(row1);
    expect(row1).toHaveClass(layout.rowActive);
    expect(row2).not.toHaveClass(layout.rowActive);
    expect(screen.getByTestId('detail-rail')).toBeInTheDocument();
    expect(
      screen.getByTestId('detail-rail').querySelector("[data-detail-name-target='title']")?.textContent,
    ).toBe('docker.io');

    // 通过头部导航切换到下一条，选中行随之切换，并回显该仓库的默认镜像名（不再扫全仓目录）。
    // Header prev/next nav advances the open row, moves .rowActive, and hydrates default_repo without catalog sweeping.
    const nextBtn = screen.getByRole('button', {name: zh.common.detailNav.next});
    fireEvent.click(nextBtn);
    expect(row2).toHaveClass(layout.rowActive);
    expect(row1).not.toHaveClass(layout.rowActive);
    expect(row2).toHaveTextContent('data/seatunnel');
    expect(screen.getByTestId('registry-edit-host-input')).toHaveValue('harbor.corp.local');
    expect(screen.getByTestId('registry-edit-host-input')).not.toBeDisabled();
    expect(screen.getByTestId('registry-edit-default-repo-input')).toHaveValue('data/seatunnel');
    expect(screen.queryByTestId('registry-catalog-section')).not.toBeInTheDocument();

    // 修改后点击上一条应弹出脏态确认弹窗，放弃修改后切回第一条。
    // Modifying a field and switching via nav triggers DirtyCloseDialog; discarding completes the switch.
    fireEvent.change(screen.getByTestId('registry-edit-username-input'), {target: {value: 'ops'}});
    fireEvent.click(screen.getByRole('button', {name: zh.common.detailNav.prev}));
    expect(await screen.findByText(zh.common.dirtyClose.title)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: zh.common.dirtyClose.discard}));
    expect(row1).toHaveClass(layout.rowActive);
    expect(screen.getByTestId('registry-edit-host-input')).toHaveValue('docker.io');
  });

  it('renders TableCard empty and error states with retry', async () => {
    serviceMocks.listContainerRegistries.mockRejectedValueOnce(new Error('network down'));
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    expect(await screen.findByText('network down')).toBeInTheDocument();
    serviceMocks.listContainerRegistries.mockResolvedValueOnce([]);
    fireEvent.click(screen.getByRole('button', {name: zh.monitoring.retry}));
    expect(await screen.findByText(zh.packages.registry.empty)).toBeInTheDocument();
  });

  it('triggers single-row test via icon button and deletes via more-menu AlertDialog', async () => {
    serviceMocks.testContainerRegistry.mockResolvedValueOnce({
      ...SAMPLE_ROWS[1],
      probe_status: 'ok',
    });
    serviceMocks.deleteContainerRegistry.mockResolvedValueOnce(undefined);

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ContainerRegistryPanel />
      </NextIntlClientProvider>,
    );

    const testBtn = await screen.findByTestId('registry-test-2');
    fireEvent.click(testBtn);
    await waitFor(() => {
      expect(serviceMocks.testContainerRegistry).toHaveBeenCalledWith(2);
      expect(toastMocks.success).toHaveBeenCalledWith(zh.packages.registry.probeOk);
    });

    // docker.io 与私有行均应提供对齐的「更多」三点按钮 / Both docker.io and private rows expose aligned more triggers
    expect(screen.getByTestId('registry-more-1')).toBeInTheDocument();
    const moreBtn = screen.getByTestId('registry-more-2');
    fireEvent.pointerDown(moreBtn, {button: 0, ctrlKey: false});
    fireEvent.click(moreBtn);
    fireEvent.click(await screen.findByRole('menuitem', {name: zh.packages.registry.delete}));
    fireEvent.click(await screen.findByTestId('registry-single-delete-confirm'));

    await waitFor(() => {
      expect(serviceMocks.deleteContainerRegistry).toHaveBeenCalledWith(2);
    });
  });
});
