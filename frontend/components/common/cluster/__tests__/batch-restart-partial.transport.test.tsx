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

/**
 * 批量重启部分失败须用 batchRestartPartial（含 success/failed 计数），禁止单节点文案。
 * Batch restart partial failure must use batchRestartPartial (success/failed counts), not single-node copy.
 */
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {NextIntlClientProvider} from 'next-intl';
import zh from '@/lib/i18n/locales/zh.json';
import {
  ClusterStatus,
  DeploymentMode,
  NodeRole,
  NodeStatus,
} from '@/lib/services/cluster/types';
import {ClusterDetail} from '../ClusterDetail';
import {toast} from '@/lib/toast';

const mockPush = vi.hoisted(() => vi.fn());
const mockReplace = vi.hoisted(() => vi.fn());
const apiGet = vi.hoisted(() => vi.fn());
const apiPost = vi.hoisted(() => vi.fn());

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mockPush, replace: mockReplace}),
  usePathname: () => '/clusters/2',
  useSearchParams: () => new URLSearchParams('tab=nodes'),
}));

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}));

vi.mock('@/components/common/onboarding/OnboardingContext', () => ({
  useOnboardingOptional: () => null,
}));

vi.mock('@/lib/services/core/api-client', () => ({
  default: {
    get: (...args: unknown[]) => apiGet(...args),
    post: (...args: unknown[]) => apiPost(...args),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: {
      request: {use: vi.fn()},
      response: {use: vi.fn()},
    },
  },
}));

vi.mock('@/lib/timezone', () => ({
  formatDateTimeMinuteTz: (value: string) => value,
}));

vi.mock('../ClusterPlugins', () => ({ClusterPlugins: () => null}));
vi.mock('../ClusterConfigs', () => ({ClusterConfigs: () => null}));
vi.mock('../EngineEventChannelCard', () => ({
  EngineEventChannelCard: () => null,
}));
vi.mock('../detail/ClusterOverviewTab', () => ({
  ClusterOverviewTab: () => null,
}));
vi.mock('../detail/ClusterOpsTab', () => ({ClusterOpsTab: () => null}));
vi.mock('../AddNodeDialog', () => ({AddNodeDialog: () => null}));
vi.mock('../EditNodeDialog', () => ({EditNodeDialog: () => null}));
vi.mock('../RemoveNodeDialog', () => ({RemoveNodeDialog: () => null}));
vi.mock('../EditClusterDialog', () => ({EditClusterDialog: () => null}));
vi.mock('../ClusterNodeLogDialog', () => ({
  ClusterNodeLogDialog: () => null,
}));
vi.mock('../scale/ScaleOperationProgress', () => ({
  ScaleOperationProgress: () => null,
}));
vi.mock('../SeaTunnelObserveMenu', () => ({
  SeaTunnelObserveMenu: () => null,
}));
vi.mock('../RuntimeStorageEditor', () => ({
  RuntimeStorageEditor: () => null,
}));
vi.mock('../RuntimeStorageFileExplorer', () => ({
  RuntimeStorageFileExplorer: () => null,
}));
vi.mock('@/components/common/sync/CheckpointPanels', () => ({
  CheckpointInspectDialog: () => null,
}));

function axiosOk<T>(data: T) {
  return Promise.resolve({data, status: 200, statusText: 'OK', headers: {}});
}

function nodesPayload() {
  return {
    error_msg: '',
    data: [
      {
        id: 4,
        cluster_id: 2,
        host_id: 1,
        host_name: 'n-a',
        host_ip: '10.0.0.4',
        role: NodeRole.MASTER_WORKER,
        status: NodeStatus.RUNNING,
        install_dir: '/opt/seatunnel',
        process_pid: 1001,
        api_port: 5801,
        is_online: true,
        created_at: '2026-11-23T04:00:00Z',
        updated_at: '2026-11-23T04:00:00Z',
      },
      {
        id: 5,
        cluster_id: 2,
        host_id: 2,
        host_name: 'n-b',
        host_ip: '10.0.0.5',
        role: NodeRole.MASTER_WORKER,
        status: NodeStatus.RUNNING,
        install_dir: '/opt/seatunnel',
        process_pid: 1002,
        api_port: 5801,
        is_online: true,
        created_at: '2026-11-23T04:00:00Z',
        updated_at: '2026-11-23T04:00:00Z',
      },
    ],
  };
}

describe('batch restart partial toast', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        dispatchEvent: () => false,
      }),
    });

    apiGet.mockImplementation((url: string) => {
      const path = String(url);
      if (path.includes('/clusters/2/nodes') && path.includes('java-runtime')) {
        return axiosOk({
          error_msg: '',
          data: {
            java_home: '/opt/java/openjdk',
            java_version: '11.0.33',
            source: 'process',
          },
        });
      }
      if (path.match(/\/clusters\/2\/nodes$/)) {
        return axiosOk(nodesPayload());
      }
      if (path.match(/\/clusters\/2$/)) {
        return axiosOk({
          error_msg: '',
          data: {
            id: 2,
            name: 'batch-partial',
            cluster_code: 'bp',
            description: '',
            deployment_mode: DeploymentMode.HYBRID,
            version: '2.3.12',
            status: ClusterStatus.RUNNING,
            install_dir: '/opt/seatunnel',
            config: {},
            node_count: 2,
            online_nodes: 2,
            health_status: 'healthy',
            created_at: '2026-11-23T04:00:00Z',
            updated_at: '2026-11-23T04:00:00Z',
          },
        });
      }
      if (path.includes('/clusters/2/status')) {
        return axiosOk({
          error_msg: '',
          data: {
            cluster_id: 2,
            status: ClusterStatus.RUNNING,
            online_nodes: 2,
            total_nodes: 2,
            health_status: 'healthy',
          },
        });
      }
      if (path.includes('/clusters/2/events')) {
        return axiosOk({events: [], total: 0, page: 1, page_size: 20});
      }
      if (path.includes('error-groups') || path.includes('diagnostics')) {
        return axiosOk({error_msg: '', data: {items: [], total: 0}});
      }
      if (path.includes('overview') || path.includes('monitoring')) {
        return axiosOk({
          error_msg: '',
          data: {stats: {active_alerts_1h: 0, restart_failed_events_24h: 0}},
        });
      }
      if (path.includes('scale') || path.includes('operations')) {
        return axiosOk([]);
      }
      return axiosOk({error_msg: '', data: null});
    });

    apiPost.mockImplementation((url: string) => {
      const path = String(url);
      if (path.includes('/nodes/4/restart')) {
        return axiosOk({
          error_msg: '',
          data: {
            cluster_id: 2,
            operation: 'restart',
            success: true,
            message: 'restart accepted',
          },
        });
      }
      if (path.includes('/nodes/5/restart')) {
        const err = Object.assign(new Error('节点重启失败'), {
          name: 'ApiError',
          status: 500,
        });
        return Promise.reject(err);
      }
      return axiosOk({error_msg: '', data: {}});
    });
  });

  it('toasts batchRestartPartial with success/failed counts on mixed outcomes', async () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ClusterDetail clusterId={2} />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('n-a')).toBeTruthy();
      expect(screen.getByText('n-b')).toBeTruthy();
    });

    fireEvent.click(screen.getByLabelText(zh.cluster.nodeList));

    await waitFor(() => {
      expect(screen.getByTestId('cluster-nodes-batch-bar')).toBeTruthy();
    });

    fireEvent.click(
      within(screen.getByTestId('cluster-nodes-batch-bar')).getByRole(
        'button',
        {name: zh.cluster.restart},
      ),
    );
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(
      within(dialog).getByRole('button', {name: zh.cluster.restart}),
    );

    await waitFor(() => {
      expect(toast.warning).toHaveBeenCalledWith(
        zh.cluster.batchRestartPartial
          .replace('{success}', '1')
          .replace('{failed}', '1'),
      );
    });
    expect(toast.error).not.toHaveBeenCalledWith(zh.cluster.nodeRestartError);
  });
});
