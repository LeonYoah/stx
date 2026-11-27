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

// Kubernetes 主机接入、凭据解析、双身份拆分、轮换与删除清理测试：
// 覆盖 3 步宽抽屉（Step 1 连接方式与路径 → Step 2 Plan A 控制面 YAML + kubeconfig/token 解析 →
// Step 3 4 阶段无状态预检 + 保存后引导）、详情页双 SA 独立区块、轮换先测后换与安全清理清单。
// Tests for redesigned Kubernetes host onboarding, dual-SA isolation, rotation & cleanup:
// 3-step Drawer (Step 1 access/prep → Step 2 Plan A control-plane YAML + kubeconfig/token →
// Step 3 4-stage probe + post-save handoff), HostDetail dual-SA blocks, rotation, and safe delete cleanup.

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
import {HostTable} from '../HostTable';

const createHostSafe = vi.hoisted(() => vi.fn());
const updateHostSafe = vi.hoisted(() => vi.fn());
const getHostSafe = vi.hoisted(() => vi.fn());
const testK8sConnection = vi.hoisted(() => vi.fn());
const testK8sDraftConnection = vi.hoisted(() => vi.fn());
const inspectK8sKubeconfig = vi.hoisted(() => vi.fn());
const listK8sNamespaceObjects = vi.hoisted(() => vi.fn());
const toastSuccess = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    host: {
      createHostSafe,
      updateHostSafe,
      getHostSafe,
      testK8sConnection,
      testK8sDraftConnection,
      inspectK8sKubeconfig,
      listK8sNamespaceObjects,
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
    server: https://127.0.0.1:6443
- name: prod
  cluster:
    server: https://10.0.0.10:6443
contexts:
- name: dev
  context:
    cluster: dev
    user: admin
    namespace: seatunnel-dev
- name: prod
  context:
    cluster: prod
    user: admin
    namespace: seatunnel-prod
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
    k8s_access_mode: 'in_cluster',
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

describe('Kubernetes Host Onboarding, Dual-SA Split, Rotation & Cleanup', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    inspectK8sKubeconfig.mockImplementation(async (_raw: string, preferredCtx?: string) => ({
      valid: true,
      current_context: 'dev',
      selected_context: preferredCtx || 'dev',
      contexts: [
        {
          name: 'dev',
          cluster: 'dev',
          user: 'admin',
          namespace: 'seatunnel-dev',
          server: 'https://127.0.0.1:6443',
          has_external_files: false,
          has_exec_plugin: false,
          has_loopback_server: true,
          credential_kind: 'token',
        },
        {
          name: 'prod',
          cluster: 'prod',
          user: 'admin',
          namespace: 'seatunnel-prod',
          server: 'https://10.0.0.10:6443',
          has_external_files: false,
          has_exec_plugin: false,
          has_loopback_server: false,
          credential_expires_at: '2030-01-01T00:00:00Z',
          credential_kind: 'service_account_jwt',
        },
      ],
      warnings: preferredCtx === 'prod' ? [] : ['loopback_server'],
    }));
    testK8sDraftConnection.mockResolvedValue({
      connected: true,
      server_version: 'v1.33.9',
      permission_mode: 'minimal',
      control_plane_sa: 'stx-manager',
      can_manage_rbac: false,
      can_list_namespaces: false,
      credential_expires_at: '2030-01-01T00:00:00Z',
      rbac: [
        {group: '', resource: 'pods', verb: 'get', scope: 'required', allowed: true},
        {group: '', resource: 'secrets', verb: 'create', scope: 'optional', allowed: false},
        {group: '', resource: 'namespaces', verb: 'list', scope: 'extended', allowed: false},
      ],
      elapsed_ms: 14,
    });
    testK8sConnection.mockResolvedValue({
      connected: true,
      server_version: 'v1.33.9',
      permission_mode: 'minimal',
      control_plane_sa: 'stx-manager',
      can_manage_rbac: false,
      can_list_namespaces: false,
      rbac: [],
      elapsed_ms: 12,
    });
    getHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({
        id: 101,
        name: 'k8s-dev',
        k8s_namespace: 'seatunnel-dev',
        k8s_access_mode: 'external',
      }),
    });
    listK8sNamespaceObjects.mockResolvedValue(['default']);
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });
  });

  it('CreateHostDialog walks 3-step Drawer (Step 1 access/prep → Step 2 kubeconfig inspect → Step 3 4-stage probe & handoff)', async () => {
    const onSuccess = vi.fn();
    const onDeployCluster = vi.fn();
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({
        id: 101,
        name: 'k8s-dev',
        k8s_access_mode: 'external',
      }),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog
          open
          onOpenChange={vi.fn()}
          onSuccess={onSuccess}
          onDeployCluster={onDeployCluster}
        />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('host-type-kubernetes'));
    expect(screen.getByTestId('k8s-step-1')).toBeInTheDocument();
    // Step 1 首屏直接可见可编辑命名空间且无 seatunnel-runner 干扰 / Step 1 exposes namespace cleanly with zero engine SA clutter
    expect(document.getElementById('k8s_namespace')).toHaveValue('seatunnel');
    expect(document.getElementById('k8s_default_service_account')).not.toBeInTheDocument();

    // 选择集群外入口并验证流向说明切换 / Pick external endpoint via RadioCards
    fireEvent.click(screen.getByTestId('k8s-access-mode-external'));
    expect(screen.getByText(zh.host.k8sAccessFlowExternal)).toBeInTheDocument();

    // Step 1 空名称走行内校验 / Empty host name validates inline
    fireEvent.click(screen.getByTestId('k8s-step1-next'));
    expect(document.getElementById('name-error')).toHaveTextContent(zh.host.errors.nameRequired);
    expect(toastError).not.toHaveBeenCalled();

    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-dev'},
    });
    fireEvent.click(screen.getByTestId('k8s-step1-next'));

    // Step 2：Plan A 纯控制面模板默认展开（因为默认选了先生成模板）且无引擎 SA / Step 2 shows Plan A control-plane YAML only
    expect(screen.getByTestId('k8s-step-2')).toBeInTheDocument();
    const yamlPre = screen.getByTestId('k8s-rbac-template-yaml');
    expect(yamlPre.textContent).toContain('name: stx-manager');
    expect(yamlPre.textContent).not.toContain('ClusterRole');
    expect(yamlPre.textContent).not.toContain('seatunnel-runner');
    expect(yamlPre.textContent).not.toContain('services/proxy');
    expect(yamlPre.textContent).not.toContain('pods/proxy');

    fireEvent.change(document.getElementById('k8s_kubeconfig')!, {
      target: {value: SAMPLE_KUBECONFIG},
    });
    expect(await screen.findByTestId('create-kubeconfig-warnings')).toBeInTheDocument();
    fireEvent.change(screen.getByTestId('create-kubeconfig-server-override'), {
      target: {value: 'https://192.168.10.25:6443'},
    });
    expect(screen.queryByTestId('create-kubeconfig-warnings')).not.toBeInTheDocument();

    // Step 3：无状态 4 阶段预检，通过前不落库 / Step 3 runs stateless 4-stage probe before persisting
    fireEvent.click(screen.getByTestId('k8s-step2-next'));
    await waitFor(() => {
      expect(testK8sDraftConnection).toHaveBeenCalledTimes(1);
    });
    expect(createHostSafe).not.toHaveBeenCalled();
    expect(screen.getByTestId('k8s-guide-draft-notice')).toBeInTheDocument();
    expect(screen.getByTestId('k8s-guide-stage-list')).toHaveTextContent(zh.host.k8sGuide.stageRbac);
    expect(await screen.findByText('stx-manager')).toBeInTheDocument();
    // 校验 optional 权限不误计入核心必选（应显示 核心 1/1 · 扩展 0/2） / Optional rules must not count as required
    expect(screen.getByTestId('k8s-connect-guide')).toHaveTextContent('核心 1/1 · 扩展 0/2');
    expect(screen.getByTestId('k8s-guide-rbac-details-toggle')).toBeInTheDocument();

    const saveBtn = await screen.findByTestId('k8s-guide-save-host');
    await waitFor(() => expect(saveBtn).not.toBeDisabled());
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(createHostSafe).toHaveBeenCalledWith({
        name: 'k8s-dev',
        host_type: HostType.KUBERNETES,
        description: '',
        k8s_auth_mode: 'kubeconfig',
        k8s_access_mode: 'external',
        k8s_namespace: 'seatunnel-dev',
        k8s_kubeconfig: SAMPLE_KUBECONFIG.trim(),
        k8s_context: 'dev',
        k8s_api_url: 'https://192.168.10.25:6443',
      });
    });

    // 保存后出现可选预备引擎 SA 卡片：未建时拦截绑定，建好后一键检测并设为本主机默认
    // Post-save optional engine SA card: blocks binding when missing, binds default once detected
    expect(await screen.findByTestId('k8s-guide-engine-sa-card')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('k8s-engine-sa-bind-btn'));
    expect(await screen.findByTestId('k8s-engine-sa-bind-feedback')).toHaveTextContent(
      'seatunnel-runner',
    );
    expect(updateHostSafe).not.toHaveBeenCalled();

    listK8sNamespaceObjects.mockResolvedValueOnce(['default', 'seatunnel-runner']);
    updateHostSafe.mockResolvedValueOnce({
      success: true,
      data: makeK8sHost({
        id: 101,
        name: 'k8s-dev',
        k8s_namespace: 'seatunnel-dev',
        k8s_access_mode: 'external',
        k8s_default_service_account: 'seatunnel-runner',
      }),
    });
    fireEvent.click(screen.getByTestId('k8s-engine-sa-bind-btn'));
    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(101, {
        k8s_default_service_account: 'seatunnel-runner',
      });
    });
    expect(screen.getByTestId('k8s-guide-saved-engine-sa-chip')).toHaveTextContent(
      'seatunnel-runner',
    );

    // 保存后出现前往部署集群出口 / Post-save banner exposes Deploy Cluster CTA
    const deployBtn = await screen.findByTestId('k8s-guide-deploy-cluster');
    fireEvent.click(deployBtn);
    expect(onDeployCluster).toHaveBeenCalledTimes(1);
    expect(onSuccess).toHaveBeenCalledTimes(2);
  });

  it('CreateHostDialog Step 2 prep=existing collapses RBAC template, strips wrapped JWT, and warns on skip TLS', async () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('host-type-kubernetes'));
    fireEvent.change(document.getElementById('name')!, {target: {value: 'k8s-token'}});
    fireEvent.click(screen.getByTestId('k8s-prep-mode-existing'));
    fireEvent.click(screen.getByTestId('k8s-step1-next'));

    expect(screen.queryByTestId('k8s-rbac-template-panel')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    fireEvent.click(document.getElementById('k8s_insecure_skip_tls')!);
    expect(screen.getByTestId('k8s-skip-tls-warning')).toBeInTheDocument();

    fireEvent.change(document.getElementById('k8s_token')!, {
      target: {value: 'only-signature-part'},
    });
    expect(screen.getByTestId('k8s-token-truncated-warning')).toBeInTheDocument();

    fireEvent.change(document.getElementById('k8s_api_url')!, {
      target: {value: 'https://10.0.0.20:6443'},
    });
    fireEvent.change(document.getElementById('k8s_token')!, {
      target: {value: `${SAMPLE_VALID_JWT.slice(0, 60)}\n${SAMPLE_VALID_JWT.slice(60)}`},
    });
    expect(screen.queryByTestId('k8s-token-truncated-warning')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('k8s-step2-next'));
    await waitFor(() => {
      expect(testK8sDraftConnection).toHaveBeenCalledWith(
        expect.objectContaining({
          k8s_auth_mode: 'token',
          k8s_api_url: 'https://10.0.0.20:6443',
          k8s_token: SAMPLE_VALID_JWT,
          k8s_insecure_skip_tls: true,
        }),
        expect.any(Object),
      );
    });
    expect(createHostSafe).not.toHaveBeenCalled();
  });

  it('HostDetail separates control-plane & engine SA sections, verifies before rotating credential, and keeps edit mode-exclusive', async () => {
    const updatedHost = makeK8sHost({
      k8s_auth_mode: 'token',
      has_k8s_token: true,
      k8s_access_mode: 'in_cluster',
      k8s_credential_expires_at: '2030-01-01T00:00:00Z',
    });
    updateHostSafe.mockResolvedValue({success: true, data: updatedHost});
    getHostSafe.mockResolvedValue({success: true, data: updatedHost});
    const onUpdated = vi.fn();

    const {rerender} = render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <HostDetail
          host={makeK8sHost({k8s_credential_expires_at: '2020-01-01T00:00:00Z'})}
          editing={false}
          onStartEdit={vi.fn()}
          onCancelEdit={vi.fn()}
          onUpdated={onUpdated}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.getByTestId('host-detail-k8s-expiry')).toHaveTextContent(
      zh.host.k8sCredentialExpired,
    );
    // 详情页独立渲染引擎默认运行身份区块及独立模板 / Separate engine SA block with standalone YAML template
    const engineSection = screen.getByTestId('host-detail-engine-sa-section');
    expect(engineSection).toHaveTextContent(zh.host.k8sEngineSaAutoLabel);
    fireEvent.click(screen.getByTestId('k8s-engine-sa-template-toggle'));
    const engineYaml = screen.getByTestId('k8s-engine-sa-template-yaml');
    expect(engineYaml.textContent).toContain('name: seatunnel-runner');
    expect(engineYaml.textContent).not.toContain('stx-manager');

    // 轮换凭据弹窗：未通过探测前不能提交 / Rotation dialog disables Apply until probed
    fireEvent.click(screen.getByTestId('k8s-rotate-credential-btn'));
    expect(await screen.findByTestId('k8s-rotate-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('k8s-rotate-apply-btn')).toBeDisabled();

    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    fireEvent.change(document.getElementById('rotate_k8s_token')!, {
      target: {value: 'rotated-token'},
    });
    fireEvent.click(screen.getByTestId('k8s-rotate-test-btn'));
    await waitFor(() => {
      expect(screen.getByTestId('k8s-rotate-apply-btn')).not.toBeDisabled();
    });

    fireEvent.click(screen.getByTestId('k8s-rotate-apply-btn'));
    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(42, {
        k8s_auth_mode: 'token',
        k8s_api_url: 'https://10.0.0.10:6443',
        k8s_token: 'rotated-token',
        k8s_insecure_skip_tls: false,
      });
    });

    vi.clearAllMocks();
    updateHostSafe.mockResolvedValue({success: true, data: updatedHost});

    rerender(
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
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    fireEvent.change(document.getElementById('edit-k8s_token')!, {
      target: {value: 'new-sa-token'},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.save}));
    await waitFor(() => {
      expect(updateHostSafe).toHaveBeenCalledWith(42, {
        name: 'k8s-prod',
        description: 'Production cluster',
        k8s_auth_mode: 'token',
        k8s_access_mode: 'in_cluster',
        k8s_api_url: 'https://10.0.0.10:6443',
        k8s_namespace: 'seatunnel',
        k8s_insecure_skip_tls: false,
        k8s_token: 'new-sa-token',
      });
    });
  });

  it('HostTable delete cleanup omits runner SA by default and includes it only when opted in', async () => {
    const k8sHost = makeK8sHost({
      k8s_namespace: 'seatunnel-full',
      k8s_permission_mode: 'full',
      k8s_default_service_account: 'seatunnel-runner',
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <HostTable
          hosts={[k8sHost]}
          selectedIds={new Set()}
          pageAllSelected={false}
          pageSomeSelected={false}
          onToggleSelect={vi.fn()}
          onToggleSelectPage={vi.fn()}
          onViewDetail={vi.fn()}
          onEdit={vi.fn()}
          onDelete={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    const row = screen.getByTestId('host-row-42');
    const menuTrigger = row.querySelector('button[aria-haspopup="menu"]');
    expect(menuTrigger).toBeTruthy();
    fireEvent.keyDown(menuTrigger!, {key: 'ArrowDown'});
    fireEvent.click(await screen.findByText(zh.common.delete));

    const cleanupPanel = await screen.findByTestId('k8s-delete-cleanup-panel');
    expect(cleanupPanel.textContent).toContain('kubectl -n seatunnel-full delete rolebinding stx-manager-binding');
    expect(cleanupPanel.textContent).toContain('kubectl delete clusterrolebinding stx-manager-cluster-binding');
    expect(cleanupPanel.textContent).not.toContain('seatunnel-runner');

    fireEvent.click(screen.getByTestId('k8s-delete-include-runner'));
    expect(cleanupPanel.textContent).toContain('kubectl -n seatunnel-full delete serviceaccount seatunnel-runner');
  });
});
