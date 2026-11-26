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

// Kubernetes 主机创建与编辑弹窗测试：验证 Kubernetes 类型可选、in_cluster 已下线，
// 且 kubeconfig 与 token 认证字段严格互斥并按模式提交干净载荷。
// Tests for Kubernetes host create/edit dialogs: unlocked K8s type, no in_cluster option,
// strict mutual exclusivity between kubeconfig and token fields, and clean per-mode payloads.

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
import {EditHostDialog} from '../EditHostDialog';

const createHostSafe = vi.hoisted(() => vi.fn());
const updateHostSafe = vi.hoisted(() => vi.fn());
const toastSuccess = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    host: {
      createHostSafe,
      updateHostSafe,
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

describe('Kubernetes Host Dialogs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  it('CreateHostDialog unlocks Kubernetes, hides in_cluster, and keeps kubeconfig vs token mutually exclusive', async () => {
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({id: 101, name: 'k8s-dev'}),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog
          open
          onOpenChange={onOpenChange}
          onSuccess={onSuccess}
        />
      </NextIntlClientProvider>,
    );

    // 切换到 Kubernetes 主机类型 / Switch host type to Kubernetes
    const hostTypeTrigger = document.getElementById('host_type');
    expect(hostTypeTrigger).toBeTruthy();
    fireEvent.keyDown(hostTypeTrigger!, {key: 'ArrowDown'});
    const k8sOption = await screen.findByRole('option', {
      name: zh.host.types.kubernetes,
    });
    expect(k8sOption).not.toHaveAttribute('data-disabled');
    fireEvent.click(k8sOption);

    // 默认 kubeconfig 模式：仅显示 kubeconfig，不显示 in_cluster 或 token/API URL/CA/SkipTLS
    // Default kubeconfig mode: only kubeconfig is rendered, never in_cluster or token/API URL/CA/SkipTLS
    expect(screen.getByRole('button', {name: 'Kubeconfig'})).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByRole('button', {name: 'Token'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /in_cluster/i})).not.toBeInTheDocument();
    expect(document.getElementById('k8s_kubeconfig')).toBeInTheDocument();
    expect(document.getElementById('k8s_api_url')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_token')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_ca_cert')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_insecure_skip_tls')).not.toBeInTheDocument();

    // 填写 kubeconfig 并提交，验证仅提交 kubeconfig 载荷
    // Fill kubeconfig and submit, verifying payload contains only kubeconfig fields
    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-dev'},
    });
    fireEvent.change(document.getElementById('k8s_kubeconfig')!, {
      target: {value: SAMPLE_KUBECONFIG},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.create}));

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
  });

  it('CreateHostDialog token mode hides kubeconfig, hides CA when skip-TLS is checked, and submits clean token payload', async () => {
    createHostSafe.mockResolvedValue({
      success: true,
      data: makeK8sHost({id: 102, name: 'k8s-token', k8s_auth_mode: 'token'}),
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateHostDialog
          open
          onOpenChange={vi.fn()}
          onSuccess={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    const hostTypeTrigger = document.getElementById('host_type')!;
    fireEvent.keyDown(hostTypeTrigger, {key: 'ArrowDown'});
    fireEvent.click(
      await screen.findByRole('option', {name: zh.host.types.kubernetes}),
    );

    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    expect(document.getElementById('k8s_kubeconfig')).not.toBeInTheDocument();
    expect(document.getElementById('k8s_api_url')).toBeInTheDocument();
    expect(document.getElementById('k8s_token')).toBeInTheDocument();
    expect(document.getElementById('k8s_ca_cert')).toBeInTheDocument();

    // 勾选跳过 TLS 后自动收起 CA 证书输入框
    // Checking skip-TLS hides the CA certificate textarea
    fireEvent.click(document.getElementById('k8s_insecure_skip_tls')!);
    expect(document.getElementById('k8s_ca_cert')).not.toBeInTheDocument();

    fireEvent.change(document.getElementById('name')!, {
      target: {value: 'k8s-token'},
    });
    fireEvent.change(document.getElementById('k8s_api_url')!, {
      target: {value: 'https://10.0.0.20:6443'},
    });
    fireEvent.change(document.getElementById('k8s_token')!, {
      target: {value: 'eyJhbGciOiJSUzI1NiJ9.token'},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.create}));

    await waitFor(() => {
      expect(createHostSafe).toHaveBeenCalledTimes(1);
    });
    expect(createHostSafe).toHaveBeenCalledWith({
      name: 'k8s-token',
      host_type: HostType.KUBERNETES,
      description: '',
      k8s_auth_mode: 'token',
      k8s_api_url: 'https://10.0.0.20:6443',
      k8s_namespace: 'default',
      k8s_token: 'eyJhbGciOiJSUzI1NiJ9.token',
      k8s_insecure_skip_tls: true,
      k8s_ca_cert: '',
    });
  });

  it('EditHostDialog renders strictly mode-exclusive fields and enforces required token when switching from kubeconfig', async () => {
    updateHostSafe.mockResolvedValue({success: true});
    const onSuccess = vi.fn();

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <EditHostDialog
          open
          onOpenChange={vi.fn()}
          host={makeK8sHost()}
          onSuccess={onSuccess}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.queryByRole('button', {name: /in_cluster/i})).not.toBeInTheDocument();
    expect(document.getElementById('edit-k8s_kubeconfig')).toBeInTheDocument();
    expect(document.getElementById('edit-k8s_token')).not.toBeInTheDocument();
    expect(screen.getByText('https://10.0.0.10:6443')).toBeInTheDocument();

    // 切到 token 模式时，因尚未存过 token，直接保存应被校验拦截
    // Switching to token when no token is stored must reject empty token submission
    fireEvent.click(screen.getByRole('button', {name: 'Token'}));
    expect(document.getElementById('edit-k8s_kubeconfig')).not.toBeInTheDocument();
    expect(document.getElementById('edit-k8s_api_url')).toBeInTheDocument();
    expect(document.getElementById('edit-k8s_token')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: zh.common.save}));
    expect(toastError).toHaveBeenCalledWith(zh.host.errors.k8sTokenRequired);
    expect(updateHostSafe).not.toHaveBeenCalled();

    // 填入新 token 后提交，仅携带 token 模式字段
    // Submitting after providing token sends only token-mode fields
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
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });
});
