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

// Kubernetes 主机创建弹窗与详情原地编辑测试：
// 验证主机类型置顶分段切换、行内校验、kubeconfig/token 双模式获取命令与自定义控制面 SA、
// Step 2 并发体检/动态排除 control_plane_sa/分类故障提示，以及 HostDetail 原地编辑。
// Tests for Kubernetes CreateHostDialog and in-place HostDetail editing:
// top-level host-type SegmentedControl, inline validation, dual-mode kubeconfig/token guides,
// custom manager SA, Step 2 dynamic control_plane_sa filtering, classified errors, and in-place editing.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {
  AgentStatus,
  HostStatus,
  HostType,
  type HostInfo,
} from '@/lib/services/host/types';
import {CreateHostDialog} from '../CreateHostDialog';
import {HostDetail} from '../HostDetail';

const createHostSafe = vi.hoisted(() => vi.fn());
const updateHostSafe = vi.hoisted(() => vi.fn());
const testK8sConnection = vi.hoisted(() => vi.fn());
const toastSuccess = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    host: {
      createHostSafe,
      updateHostSafe,
      testK8sConnection,
    },
  },
}));

vi.mock('@/lib/toast', () => ({
  toast: {
    success: toastSuccess,
    warning: vi.fn(),
    error: toastError,
    message: vi.fn(),
  },
}));

const SAMPLE_KUBECONFIG = `apiVersion: v1
kind: Config
clusters:
- name: dev
  cluster:
    server: https://10.0.0.10:6443
contexts:
- name: dev
  context:
    cluster: dev
    user: admin
current-context: dev
users:
- name: admin
  user:
    token: kube-token
`;

const SAMPLE_VALID_JWT = `${'a'.repeat(40)}.${'b'.repeat(80)}.${'c'.repeat(60)}`;

function makeK8sHost(overrides: Partial<HostInfo> = {}): HostInfo {
  return {
    id: 42,
    name: 'k8s-prod',
    host_type: HostType.KUBERNETES,
    description: 'Production cluster',
    status: HostStatus.CONNECTED,
    agent_status: AgentStatus.NOT_INSTALLED,
    k8s_api_url: 'https://10.0.0.10:6443',
    k8s_namespace: 'seatunnel',
    k8s_auth_mode: 'kubeconfig',
    has_k8s_kubeconfig: true,
    has_k8s_token: false,
    k8s_insecure_skip_tls: false,
    cpu_usage: 0,
    memory_usage: 0,
    disk_usage: 0,
    last_check: null,
    is_online: true,
    created_at: '2026-11-20T08:00:00Z',
    updated_at: '2026-11-20T08:00:00Z',
    ...overrides,
  };
}

describe('Kubernetes Host Onboarding & In-Place Edit', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    testK8sConnection.mockResolvedValue({
      connected: true,
      server_version: 'v1.33.9',
      permission_mode: 'full',
      can_manage_rbac: true,
      can_list_namespaces: true,
      rbac: [],
      elapsed_ms: 12,
    });
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });
    if (!HTMLElement.prototype.hasPointerCapture) {
      HTMLElement.prototype.hasPointerCapture = () => false;
    }
    if (!HTMLElement.prototype.scrollIntoView) {
      HTMLElement.prototype.scrollIntoView = () => {};
    }
  });

  it('CreateHostDialog puts host type first via SegmentedControl, shows inline errors, and keeps kubeconfig vs token exclusive', async () => {
    const onSuccess = vi.fn();
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({id: 101, name: 'k8s-dev'}),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog
          open
          onOpenChange={vi.fn()}
          onSuccess={onSuccess}
        />
      </NextIntlClientProvider>,
    );

    // 顶部直接用分段控件切换到 Kubernetes / Switch host type to Kubernetes via top SegmentedControl
    fireEvent.click(screen.getByTestId('host-type-kubernetes'));

    // 默认 kubeconfig 模式：展示 kubeconfig 导出命令，不展示 token/API URL/CA/SkipTLS
    // Default kubeconfig mode: shows export command, hides token/API URL/CA/SkipTLS
    expect(screen.getByRole('button', {name: 'Kubeconfig'})).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByText('kubectl config view --raw --flatten --minify')).toBeInTheDocument();
    expect(document.getElementById('k8s_kubeconfig')).toBeInTheDocument();
    expect(document.getElementById('k8s_api_url')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_token')).not.toBeInTheDocument();

    // 空提交走字段行内报错，不再仅靠消失式 toast / Empty submit shows inline field errors instead of toast
    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.nextStepTest}));
    expect(document.getElementById('name-error')).toHaveTextContent(
      zh.host.errors.nameRequired,
    );
    expect(screen.getByText(zh.host.errors.k8sKubeconfigRequired)).toBeInTheDocument();
    expect(toastError).not.toHaveBeenCalled();

    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-dev'},
    });
    fireEvent.change(document.getElementById('k8s_kubeconfig')!, {
      target: {value: SAMPLE_KUBECONFIG},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.nextStepTest}));

    await waitFor(() => {
      expect(createHostSafe).toHaveBeenCalledTimes(1);
    });
    expect(createHostSafe).toHaveBeenCalledWith({
      name: 'k8s-dev',
      host_type: HostType.KUBERNETES,
      description: '',
      k8s_auth_mode: 'kubeconfig',
      k8s_namespace: 'default',
      k8s_kubeconfig: SAMPLE_KUBECONFIG.trim(),
    });
    await waitFor(() => {
      expect(testK8sConnection).toHaveBeenCalledWith(101, expect.any(Object));
    });
  });

  it('CreateHostDialog token mode warns on truncated token, strips line wraps, hides CA when skip-TLS is checked', async () => {
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({id: 102, name: 'k8s-token', k8s_auth_mode: 'token'}),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('host-type-kubernetes'));
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    expect(document.getElementById('k8s_kubeconfig')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_api_url')).toBeInTheDocument();
    expect(document.getElementById('k8s_token')).toBeInTheDocument();
    expect(document.getElementById('k8s_ca_cert')).toBeInTheDocument();

    // 勾选跳过 TLS 后自动收起 CA 证书输入框 / Checking skip-TLS hides CA textarea
    fireEvent.click(document.getElementById('k8s_insecure_skip_tls')!);
    expect(document.getElementById('k8s_ca_cert')).not.toBeInTheDocument();

    // 粘贴截断单段令牌时显示预警 / Truncated token triggers non-blocking amber warning
    fireEvent.change(document.getElementById('k8s_token')!, {
      target: {value: 'only-signature-part'},
    });
    expect(screen.getByTestId('k8s-token-truncated-warning')).toBeInTheDocument();

    // 替换为带换行的完整 JWT：预警消失，提交时自动剥离换行符 / Wrapped full JWT clears warning and strips newlines
    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-token'},
    });
    fireEvent.change(document.getElementById('k8s_api_url')!, {
      target: {value: 'https://10.0.0.20:6443'},
    });
    fireEvent.change(document.getElementById('k8s_token')!, {
      target: {value: `${SAMPLE_VALID_JWT.slice(0, 60)}\n${SAMPLE_VALID_JWT.slice(60)}`},
    });
    expect(screen.queryByTestId('k8s-token-truncated-warning')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.nextStepTest}));

    await waitFor(() => {
      expect(createHostSafe).toHaveBeenCalledWith({
        name: 'k8s-token',
        host_type: HostType.KUBERNETES,
        description: '',
        k8s_auth_mode: 'token',
        k8s_api_url: 'https://10.0.0.20:6443',
        k8s_namespace: 'default',
        k8s_token: SAMPLE_VALID_JWT,
        k8s_insecure_skip_tls: true,
        k8s_ca_cert: '',
      });
    });
    await waitFor(() => {
      expect(testK8sConnection).toHaveBeenCalledWith(102, expect.any(Object));
    });
  });

  it('HostDetail in-place edit enforces mode-exclusive fields and inline required token error when switching from kubeconfig', async () => {
    const updatedHost = makeK8sHost({k8s_auth_mode: 'token', has_k8s_token: true});
    updateHostSafe.mockResolvedValue({success: true, data: updatedHost});
    const onUpdated = vi.fn();

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <HostDetail
          host={makeK8sHost()}
          editing
          onStartEdit={vi.fn()}
          onCancelEdit={vi.fn()}
          onUpdated={onUpdated}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.getByTestId('host-detail-edit-form')).toBeInTheDocument();
    expect(document.getElementById('edit-k8s_kubeconfig')).toBeInTheDocument();
    expect(document.getElementById('edit-k8s_token')).not.toBeInTheDocument();
    expect(screen.getByText('https://10.0.0.10:6443')).toBeInTheDocument();

    // 切到 token 模式时，因尚未存过 token，直接保存应行内提示必填
    // Switching to token when no token is stored must show inline validation error
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    expect(document.getElementById('edit-k8s_kubeconfig')).not.toBeInTheDocument();
    expect(document.getElementById('edit-k8s_api_url')).toBeInTheDocument();
    expect(document.getElementById('edit-k8s_token')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: zh.common.save}));
    expect(screen.getByText(zh.host.errors.k8sTokenRequired)).toBeInTheDocument();
    expect(updateHostSafe).not.toHaveBeenCalled();

    fireEvent.change(document.getElementById('edit-k8s_token')!, {
      target: {value: 'new-sa-token'},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.save}));

    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(42, {
        name: 'k8s-prod',
        description: 'Production cluster',
        k8s_auth_mode: 'token',
        k8s_api_url: 'https://10.0.0.10:6443',
        k8s_namespace: 'seatunnel',
        k8s_insecure_skip_tls: false,
        k8s_token: 'new-sa-token',
      });
    });
    expect(onUpdated).toHaveBeenCalledWith(updatedHost);
  });

  it('CreateHostDialog RBAC templates adapt to Kubeconfig/Token modes and custom control-plane SA', async () => {
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({
        id: 103,
        name: 'k8s-min',
        k8s_namespace: 'team-ns',
        k8s_default_service_account: 'seatunnel-runner',
      }),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('host-type-kubernetes'));
    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-min'},
    });
    fireEvent.change(document.getElementById('k8s_namespace')!, {
      target: {value: 'team-ns'},
    });

    // 展开更多可选设置填写默认 SA / Expand optional settings for default SA
    fireEvent.click(screen.getByTestId('create-host-optional-toggle'));
    fireEvent.change(document.getElementById('k8s_default_service_account')!, {
      target: {value: 'seatunnel-runner'},
    });

    // Kubeconfig 模式下展开 RBAC 模板，验证支持自定义控制面 SA 且输出 Kubeconfig 生成与导出命令
    // Under Kubeconfig mode, verify custom control-plane SA updates YAML & kubeconfig commands
    fireEvent.click(screen.getByTestId('k8s-rbac-template-toggle'));
    fireEvent.change(screen.getByTestId('k8s-rbac-template-manager-sa'), {
      target: {value: 'ops-stx'},
    });
    const yamlPre = screen.getByTestId('k8s-rbac-template-yaml');
    expect(yamlPre.textContent).toContain('namespace: team-ns');
    expect(yamlPre.textContent).toContain('name: ops-stx');
    expect(yamlPre.textContent).toContain('name: ops-stx-role');
    expect(yamlPre.textContent).toContain('name: seatunnel-runner');
    expect(screen.getByTestId('k8s-rbac-template-kubeconfig-cmd').textContent).toContain(
      'create token ops-stx --duration=8760h',
    );
    expect(screen.getByTestId('k8s-rbac-template-export-cmd').textContent).toContain(
      'kubectl config view --raw --flatten --minify',
    );

    // 切到全面权限确认 ClusterRole 与 roleRef 联动 / Full tier keeps ClusterRole & roleRef synced
    fireEvent.click(
      screen.getByRole('button', {name: zh.host.k8sRbacTemplateFull}),
    );
    expect(screen.getByTestId('k8s-rbac-template-yaml').textContent).toContain(
      'name: ops-stx-cluster-role',
    );
    expect(screen.getByTestId('k8s-rbac-template-yaml').textContent).not.toContain(
      'name: stx-manager-cluster-role',
    );

    // 切到 Token 认证模式时自动切换为 Token 签发命令 / Token auth mode switches to create token command
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    expect(screen.getByTestId('k8s-rbac-template-token').textContent).toContain(
      'kubectl -n team-ns create token ops-stx --duration=8760h',
    );

    fireEvent.click(screen.getByRole('button', {name: 'Kubeconfig'}));
    fireEvent.change(document.getElementById('k8s_kubeconfig')!, {
      target: {value: SAMPLE_KUBECONFIG},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.nextStepTest}));

    await waitFor(() => {
      expect(createHostSafe).toHaveBeenCalledWith({
        name: 'k8s-min',
        host_type: HostType.KUBERNETES,
        description: '',
        k8s_auth_mode: 'kubeconfig',
        k8s_namespace: 'team-ns',
        k8s_kubeconfig: SAMPLE_KUBECONFIG.trim(),
        k8s_default_service_account: 'seatunnel-runner',
      });
    });
  });

  it('CreateHostDialog Step 2 excludes dynamic control_plane_sa, saves default SA, shows classified failure hint, and edits without duplicate create', async () => {
    const created = makeK8sHost({
      id: 104,
      name: 'k8s-restricted',
      status: HostStatus.PENDING,
      is_online: false,
      k8s_namespace: 'seatunnel-min',
      k8s_default_service_account: '',
    });
    createHostSafe.mockResolvedValue({success: true, data: created});
    updateHostSafe
      .mockResolvedValueOnce({
        success: true,
        data: {...created, k8s_default_service_account: 'seatunnel-runner'},
      })
      .mockResolvedValueOnce({
        success: true,
        data: {
          ...created,
          k8s_namespace: 'seatunnel-min-2',
          k8s_default_service_account: 'seatunnel-runner',
        },
      });
    testK8sConnection
      .mockResolvedValueOnce({
        connected: true,
        server_version: 'v1.33.9',
        permission_mode: 'minimal',
        can_manage_rbac: false,
        can_list_namespaces: false,
        control_plane_sa: 'ops-stx-ctrl',
        service_accounts: ['default', 'ops-stx-ctrl', 'stx-manager', 'seatunnel-runner'],
        rbac: [],
        elapsed_ms: 15,
      })
      .mockResolvedValueOnce({
        connected: false,
        error_code: 'unauthorized',
        message: 'the server has asked for the client to provide credentials',
        rbac: [],
        elapsed_ms: 20,
      });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('host-type-kubernetes'));
    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-restricted'},
    });
    fireEvent.change(document.getElementById('k8s_namespace')!, {
      target: {value: 'seatunnel-min'},
    });
    fireEvent.change(document.getElementById('k8s_kubeconfig')!, {
      target: {value: SAMPLE_KUBECONFIG},
    });

    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.nextStepTest}));

    await waitFor(() => {
      expect(testK8sConnection).toHaveBeenCalledTimes(1);
    });
    expect(await screen.findByTestId('k8s-guide-permission-mode')).toHaveTextContent(
      zh.host.k8sPermissionModeMinimal,
    );

    // 验证同时排除动态识别出的 ops-stx-ctrl 与默认 stx-manager / Exclude both detected control_plane_sa and stx-manager
    const saTrigger = screen.getByTestId('k8s-guide-sa-select');
    expect(saTrigger).toHaveTextContent('seatunnel-runner');
    fireEvent.keyDown(saTrigger, {key: 'ArrowDown'});
    expect(screen.queryByRole('option', {name: 'ops-stx-ctrl'})).not.toBeInTheDocument();
    expect(screen.queryByRole('option', {name: 'stx-manager'})).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole('option', {name: 'seatunnel-runner'}));

    fireEvent.click(screen.getByTestId('k8s-guide-save-sa'));
    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(104, {
        k8s_default_service_account: 'seatunnel-runner',
      });
    });
    expect(
      await screen.findByText(
        zh.host.k8sGuide.minimalSaReady.replace('{sa}', 'seatunnel-runner'),
      ),
    ).toBeInTheDocument();
    expect(testK8sConnection).toHaveBeenCalledTimes(1);

    // 返回修改凭据后保存并重测，验证更新同一主机且失败时展示分类排障文案
    // Back to edit updates the same host without duplicate create, and 401 failure shows classified hint
    fireEvent.click(screen.getByTestId('k8s-guide-back-edit'));
    fireEvent.change(document.getElementById('k8s_namespace')!, {
      target: {value: 'seatunnel-min-2'},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.host.k8sGuide.saveAndRetest}));

    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(104, {
        name: 'k8s-restricted',
        description: '',
        k8s_auth_mode: 'kubeconfig',
        k8s_namespace: 'seatunnel-min-2',
        k8s_kubeconfig: SAMPLE_KUBECONFIG.trim(),
        k8s_default_service_account: 'seatunnel-runner',
      });
    });
    expect(createHostSafe).toHaveBeenCalledTimes(1);
    expect(
      await screen.findByText(zh.host.k8sGuide.errors.unauthorized),
    ).toBeInTheDocument();
  });
});
