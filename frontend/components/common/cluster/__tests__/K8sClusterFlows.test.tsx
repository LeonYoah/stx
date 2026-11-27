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

// 集群注册（纳管）与部署向导中的 Kubernetes 融合流程测试：
// 1. CreateClusterDialog 切到 Kubernetes 后自动发现并保留 Deployment workload_kind 与 image；
// 2. ClusterDeployWizard 切到 Kubernetes 后收起裸机步骤条并提交 stx_deployed 请求。
// Unified Kubernetes cluster registration (adopt) & deployment wizard tests:
// 1. CreateClusterDialog switches to Kubernetes, discovers a Deployment cluster, and preserves workload_kind + image;
// 2. ClusterDeployWizard switches to Kubernetes, hides the bare-metal stepper, and submits stx_deployed payload.

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
import {CreateClusterDialog} from '../CreateClusterDialog';
import {ClusterDeployWizard} from '../ClusterDeployWizard';
import {K8sResourcesStep} from '../wizard/K8sResourcesStep';
import {INITIAL_K8S_WIZARD_FORM, type K8sWizardFormState} from '../wizard/types';

const mockPush = vi.hoisted(() => vi.fn());
const getHostsSafe = vi.hoisted(() => vi.fn());
const getAllHostsByTypeSafe = vi.hoisted(() => vi.fn());
const listK8sNamespaces = vi.hoisted(() => vi.fn());
const listK8sNamespaceObjects = vi.hoisted(() => vi.fn().mockResolvedValue([]));
const discoverK8sClusters = vi.hoisted(() => vi.fn());
const createKubernetesCluster = vi.hoisted(() => vi.fn());
const toastSuccess = vi.hoisted(() => vi.fn());
const toastWarning = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mockPush, replace: vi.fn()}),
  usePathname: () => '/clusters',
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock('@/hooks/use-installer', () => ({
  usePackages: () => ({packages: null, loading: false}),
}));

vi.mock('@/lib/services', () => ({
  default: {
    host: {
      getHostsSafe,
      getAllHostsByTypeSafe,
      listK8sNamespaces,
      listK8sNamespaceObjects,
      discoverK8sClusters,
    },
    cluster: {
      createKubernetesCluster,
      createClusterSafe: vi.fn(),
      addNodeSafe: vi.fn(),
      startStxJavaProxySafe: vi.fn(),
    },
    discovery: {
      discoverProcessesSafe: vi.fn(),
    },
    config: {
      initClusterConfigsSafe: vi.fn(),
    },
    installer: {
      runPrecheck: vi.fn(),
      startInstallation: vi.fn(),
      getInstallationStatus: vi.fn(),
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

const listContainerRegistries = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services/installer/registry.service', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/services/installer/registry.service')>();
  return {...actual, listContainerRegistries};
});

const apiClientPost = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services/core/api-client', () => ({
  default: {post: apiClientPost, get: vi.fn(), put: vi.fn(), delete: vi.fn()},
}));

const K8S_HOST: HostInfo = {
  id: 7,
  name: 'k8s-staging',
  host_type: HostType.KUBERNETES,
  description: '',
  status: HostStatus.CONNECTED,
  agent_status: AgentStatus.NOT_INSTALLED,
  k8s_api_url: 'https://10.10.0.1:6443',
  k8s_namespace: 'seatunnel',
  k8s_auth_mode: 'kubeconfig',
  has_k8s_kubeconfig: true,
  has_k8s_token: false,
  cpu_usage: 0,
  memory_usage: 0,
  disk_usage: 0,
  last_check: null,
  is_online: true,
  created_at: '2026-11-20T08:00:00Z',
  updated_at: '2026-11-20T08:00:00Z',
};

describe('Unified Kubernetes Cluster Flows', () => {
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

    getAllHostsByTypeSafe.mockImplementation(async (hostType?: HostType) => ({
      success: true,
      hosts: hostType === HostType.KUBERNETES ? [K8S_HOST] : [],
    }));
    getHostsSafe.mockImplementation(
      async (params?: {host_type?: HostType}) => {
        if (params?.host_type === HostType.KUBERNETES) {
          return {success: true, data: {hosts: [K8S_HOST], total: 1}};
        }
        return {success: true, data: {hosts: [], total: 0}};
      },
    );
    listK8sNamespaces.mockResolvedValue(['seatunnel', 'default']);
    discoverK8sClusters.mockResolvedValue([
      {
        base_name: 'st-deploy',
        namespace: 'seatunnel',
        master: {
          kind: 'Deployment',
          name: 'st-deploy-master',
          replicas: 2,
          ready_replicas: 2,
          image: 'apache/seatunnel:3.1.0-custom',
        },
        worker: {
          kind: 'Deployment',
          name: 'st-deploy-worker',
          replicas: 3,
          ready_replicas: 3,
          image: 'apache/seatunnel:3.1.0-custom',
        },
        services: ['st-deploy-master', 'st-deploy'],
        configmap_name: 'st-deploy-config',
        label_selector: 'app=st-deploy',
        master_http_port: 8080,
        hazelcast_port: 5801,
      },
    ]);
  });

  it('CreateClusterDialog adopts discovered K8s Deployment cluster preserving workload_kind and image', async () => {
    const onSuccess = vi.fn();
    createKubernetesCluster.mockResolvedValue({
      cluster: {id: 55},
      proxy_status: {
        healthy: false,
        message: 'pod pending',
        pod_events: ['Pulling image'],
      },
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <CreateClusterDialog
          open
          onOpenChange={vi.fn()}
          onSuccess={onSuccess}
        />
      </NextIntlClientProvider>,
    );

    fireEvent.click(
      screen.getByRole('button', {name: zh.host.types.kubernetes}),
    );

    const candidate = await screen.findByTestId('k8s-discovered-card-0');
    fireEvent.click(candidate);

    expect(document.getElementById('k8s-adopt-name')).toHaveValue('st-deploy');
    expect(document.getElementById('k8s-adopt-master-svc')).toHaveValue(
      'st-deploy-master',
    );
    expect(document.getElementById('k8s-adopt-cluster-svc')).toHaveValue(
      'st-deploy',
    );

    fireEvent.click(
      screen.getByRole('button', {name: zh.cluster.registerCluster}),
    );

    await waitFor(() => {
      expect(createKubernetesCluster).toHaveBeenCalledTimes(1);
    });
    expect(createKubernetesCluster).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'st-deploy',
        host_id: 7,
        namespace: 'seatunnel',
        management_source: 'adopted',
        master_service_name: 'st-deploy-master',
        cluster_service_name: 'st-deploy',
        master_workload_name: 'st-deploy-master',
        worker_workload_name: 'st-deploy-worker',
        master_workload_kind: 'Deployment',
        worker_workload_kind: 'Deployment',
        configmap_name: 'st-deploy-config',
        label_selector: 'app=st-deploy',
        image: 'apache/seatunnel:3.1.0-custom',
      }),
    );
    expect(toastWarning).toHaveBeenCalled();
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it('ClusterDeployWizard walks the unified 6-step Kubernetes wizard and deploys stx_deployed cluster', async () => {
    const onComplete = vi.fn();
    const onOpenChange = vi.fn();
    createKubernetesCluster.mockResolvedValue({
      cluster: {id: 88},
      proxy_status: {healthy: true},
    });
    listContainerRegistries.mockResolvedValue([
      {id: 1, host: 'docker.io', is_system: true, probe_status: 'public'},
    ]);
    getAllHostsByTypeSafe.mockResolvedValue({success: true, hosts: [K8S_HOST]});
    apiClientPost.mockImplementation((url: string) => {
      if (url === '/clusters/k8s/precheck') {
        return Promise.resolve({
          data: {
            passed: true,
            items: [{name: 'api_connectivity', passed: true, level: 'info', message: 'ok'}],
          },
        });
      }
      return Promise.resolve({data: {}});
    });

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ClusterDeployWizard
          open
          onOpenChange={onOpenChange}
          onComplete={onComplete}
        />
      </NextIntlClientProvider>,
    );

    // 裸机 7 步；切到 Kubernetes 后变 6 步（跳过 plugins）。
    // Bare-metal has 7 steps; switching to Kubernetes yields 6 (plugins skipped).
    expect(screen.getByText('1 / 7')).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole('button', {name: zh.host.types.kubernetes}),
    );
    expect(await screen.findByText('1 / 6')).toBeInTheDocument();

    // Step1 basic：名称 + 三层镜像选择（docker.io 预填仓库与镜像名，输入 tag 提取版本）。
    // Step 1 basic: name plus the three-tier picker (docker.io prefills registry
    // and repository; typing the tag extracts the version).
    fireEvent.change(await screen.findByTestId('k8s-name-input'), {
      target: {value: 'k8s-prod'},
    });
    fireEvent.change(await screen.findByTestId('k8s-image-picker-tag-input'), {
      target: {value: '3.2.0'},
    });
    await waitFor(() => {
      expect(screen.getByTestId('k8s-version-input')).toHaveValue('3.2.0');
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.next}));

    // Step2 hosts：唯一 K8s 主机自动默认选中，只需填 namespace。
    // Step 2 hosts: the single Kubernetes host auto-selects; only the namespace
    // needs filling.
    expect(await screen.findByTestId('k8s-host-select')).toBeInTheDocument();
    fireEvent.change(screen.getByTestId('k8s-namespace-input'), {
      target: {value: 'seatunnel'},
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.next}));

    // Step3 config：默认直达。
    // Step 3 config: defaults pass straight through.
    expect(await screen.findByTestId('k8s-config-step')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: zh.common.next}));

    // Step4 precheck：真实端点结果渲染后进入部署步。
    // Step 4 precheck: the real endpoint result renders before entering deploy.
    expect(await screen.findByTestId('k8s-precheck-item-api_connectivity')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: zh.common.next}));

    // Step5 deploy：自动启动并提交扩展负载。
    // Step 5 deploy: auto-starts and submits the extended payload.
    await waitFor(() => {
      expect(createKubernetesCluster).toHaveBeenCalledTimes(1);
    });
    expect(createKubernetesCluster).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'k8s-prod',
        version: '3.2.0',
        host_id: 7,
        namespace: 'seatunnel',
        management_source: 'stx_deployed',
        image: 'apache/seatunnel:3.2.0',
        deployment_mode: 'separated',
        master_replicas: 1,
        worker_replicas: 2,
        master_http_port: 8080,
        hazelcast_port: 5801,
        java_proxy_mode: 'managed',
        enable_event_channel: false,
      }),
    );
  });

  it('K8sResourcesStep excludes control-plane stx-manager and renders standalone engine SA template with refresh', async () => {
    listK8sNamespaceObjects.mockImplementation(
      async (_hostId: number, _ns: string, kind: string) => {
        if (kind === 'serviceaccounts') {
          return ['stx-manager', 'default', 'seatunnel-runner'];
        }
        return [];
      },
    );

    const onChange = vi.fn();
    const form: K8sWizardFormState = {
      ...INITIAL_K8S_WIZARD_FORM,
      name: 'st-k8s',
      cluster_code: 'stk8s',
      image: 'apache/seatunnel:3.2.0',
      version: '3.2.0',
      host_id: 7,
      namespace: 'seatunnel',
    };

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sResourcesStep form={form} onChange={onChange} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByTestId('k8s-advanced-toggle'));
    await waitFor(() => {
      expect(screen.getByTestId('k8s-service-account')).toHaveAttribute('role', 'combobox');
    });
    const saSelect = screen.getByTestId('k8s-service-account');
    fireEvent.keyDown(saSelect, {key: 'ArrowDown'});
    expect(await screen.findByRole('option', {name: 'seatunnel-runner'})).toBeInTheDocument();
    expect(screen.queryByRole('option', {name: 'stx-manager'})).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('k8s-engine-sa-template-toggle'));
    const yaml = screen.getByTestId('k8s-engine-sa-template-yaml');
    expect(yaml.textContent).toContain('name: seatunnel-runner');
    expect(yaml.textContent).not.toContain('stx-manager');

    fireEvent.click(screen.getByTestId('k8s-engine-sa-refresh-btn'));
    await waitFor(() => {
      expect(listK8sNamespaceObjects).toHaveBeenCalledWith(7, 'seatunnel', 'serviceaccounts');
    });
  });
});
