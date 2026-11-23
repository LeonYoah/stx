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

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {
  ClusterStatus,
  DeploymentMode,
  NodeRole,
  NodeStatus,
  type ClusterInfo,
} from '@/lib/services/cluster/types';
import {ClusterMain} from '../ClusterMain';

const mockPush = vi.hoisted(() => vi.fn());
let currentSearch = '';
const mockReplace = vi.hoisted(() =>
  vi.fn((url: string) => {
    const idx = url.indexOf('?');
    currentSearch = idx >= 0 ? url.slice(idx + 1) : '';
  }),
);

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mockPush, replace: mockReplace}),
  usePathname: () => '/clusters',
  useSearchParams: () => new URLSearchParams(currentSearch),
}));

const getClustersSafe = vi.hoisted(() => vi.fn());
const getClusterSafe = vi.hoisted(() => vi.fn());
const getClusterStatusSafe = vi.hoisted(() => vi.fn());
const getNodesSafe = vi.hoisted(() => vi.fn());
const getClusterOverviewSafe = vi.hoisted(() => vi.fn());
const startClusterSafe = vi.hoisted(() => vi.fn());
const stopClusterSafe = vi.hoisted(() => vi.fn());
const restartClusterSafe = vi.hoisted(() => vi.fn());
const deleteClusterSafe = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    cluster: {
      getClustersSafe,
      getClusterSafe,
      getClusterStatusSafe,
      getNodesSafe,
      startClusterSafe,
      stopClusterSafe,
      restartClusterSafe,
      deleteClusterSafe,
    },
    monitoring: {
      getClusterOverviewSafe,
    },
  },
}));

// 子弹窗与向导不参与列表交互单测，避免拉入无关服务依赖
// Stub dialogs/wizards in list tests to keep focus on ClusterMain interactions
vi.mock('../CreateClusterDialog', () => ({
  CreateClusterDialog: () => null,
}));
vi.mock('../ClusterDeployWizard', () => ({
  ClusterDeployWizard: () => null,
}));
vi.mock('../EditClusterDialog', () => ({
  EditClusterDialog: () => null,
}));

function makeCluster(overrides: Partial<ClusterInfo> = {}): ClusterInfo {
  return {
    id: 1,
    name: 'prod-seatunnel',
    cluster_code: 'st-prod-01',
    description: 'Production cluster',
    deployment_mode: DeploymentMode.HYBRID,
    version: '2.3.12',
    status: ClusterStatus.RUNNING,
    install_dir: '/opt/seatunnel',
    config: {},
    node_count: 3,
    online_nodes: 3,
    health_status: 'healthy',
    created_at: '2026-11-01T08:00:00Z',
    updated_at: '2026-11-22T01:00:00Z',
    ...overrides,
  };
}

function renderMain() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <ClusterMain />
    </NextIntlClientProvider>,
  );
}

describe('ClusterMain', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    currentSearch = '';
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });

    getClustersSafe.mockResolvedValue({
      success: true,
      data: {
        total: 2,
        clusters: [
          makeCluster({id: 1, name: 'prod-seatunnel'}),
          makeCluster({
            id: 2,
            name: 'staging-seatunnel',
            cluster_code: 'st-stage-02',
            deployment_mode: DeploymentMode.SEPARATED,
            status: ClusterStatus.RUNNING,
            node_count: 2,
            online_nodes: 0,
            health_status: 'unhealthy',
          }),
        ],
      },
    });
    getClusterSafe.mockImplementation(async (id: number) => ({
      success: true,
      data: makeCluster({id, name: id === 1 ? 'prod-seatunnel' : 'staging-seatunnel'}),
    }));
    getClusterStatusSafe.mockResolvedValue({
      success: true,
      data: {
        cluster_id: 1,
        cluster_name: 'prod-seatunnel',
        status: ClusterStatus.RUNNING,
        health_status: 'healthy',
        total_nodes: 3,
        online_nodes: 3,
        offline_nodes: 0,
        nodes: [],
      },
    });
    getNodesSafe.mockResolvedValue({
      success: true,
      data: [
        {
          id: 11,
          cluster_id: 1,
          host_id: 101,
          host_name: 'node-01',
          host_ip: '10.0.0.11',
          role: NodeRole.MASTER_WORKER,
          install_dir: '/opt/seatunnel',
          hazelcast_port: 5801,
          api_port: 8080,
          worker_port: 5802,
          status: NodeStatus.RUNNING,
          process_pid: 1234,
          process_status: 'running',
          created_at: '2026-11-01T08:00:00Z',
          updated_at: '2026-11-22T01:00:00Z',
        },
      ],
    });
    getClusterOverviewSafe.mockResolvedValue({
      success: true,
      data: {stats: {active_alerts_1h: 0}},
    });
  });

  it('defaults to table view and highlights running cluster with 0 online nodes as unhealthy', async () => {
    renderMain();

    await waitFor(() => {
      expect(screen.getByTestId('cluster-list-view')).toBeInTheDocument();
    });

    expect(screen.getByTestId('cluster-row-1')).toBeInTheDocument();
    expect(screen.getByTestId('cluster-row-2')).toBeInTheDocument();
    expect(
      screen.getByText(zh.cluster.healthStatuses.unhealthy),
    ).toBeInTheDocument();
  });

  it('switches to card view via ViewToggle and syncs view=grid to URL', async () => {
    const {rerender} = renderMain();
    await waitFor(() => {
      expect(screen.getByTestId('cluster-list-view')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText(zh.cluster.list.viewGrid));
    expect(mockReplace).toHaveBeenCalledWith('/clusters?view=grid', {
      scroll: false,
    });

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ClusterMain />
      </NextIntlClientProvider>,
    );
    expect(screen.getByTestId('cluster-grid-view')).toBeInTheDocument();
    expect(screen.getByTestId('cluster-card-1')).toBeInTheDocument();
  });

  it('opens DetailHost quick detail rail on row click and navigates to console', async () => {
    const {rerender} = renderMain();
    await waitFor(() => {
      expect(screen.getByTestId('cluster-row-1')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId('cluster-row-1'));
    expect(mockReplace).toHaveBeenCalledWith('/clusters?cluster=1', {
      scroll: false,
    });

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ClusterMain />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('cluster-quick-detail')).toBeInTheDocument();
    });
    expect(screen.getByText('node-01')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('cluster-quick-open-console'));
    expect(mockPush).toHaveBeenCalledWith('/clusters/1');
  });

  it('renders first-run empty state vs filtered empty state', async () => {
    getClustersSafe.mockResolvedValueOnce({
      success: true,
      data: {total: 0, clusters: []},
    });
    renderMain();

    await waitFor(() => {
      expect(
        screen.getByText(zh.cluster.list.emptyFirstTitle),
      ).toBeInTheDocument();
    });

    // 切换到带筛选条件时显示筛选空态 / Filtered empty state when filters are active
    currentSearch = 'status=running';
    getClustersSafe.mockResolvedValueOnce({
      success: true,
      data: {total: 0, clusters: []},
    });
    renderMain();

    await waitFor(() => {
      expect(
        screen.getByText(zh.cluster.list.emptyFilteredTitle),
      ).toBeInTheDocument();
    });
  });

  it('keeps stale rows on refresh error and exposes inline retry', async () => {
    renderMain();
    await waitFor(() => {
      expect(screen.getByTestId('cluster-row-1')).toBeInTheDocument();
    });

    getClustersSafe.mockResolvedValueOnce({
      success: false,
      error: 'network timeout',
    });
    fireEvent.click(screen.getByRole('button', {name: zh.common.refresh}));

    await waitFor(() => {
      expect(screen.getByText('network timeout')).toBeInTheDocument();
    });
    // 旧数据仍然可见 / Stale rows stay visible
    expect(screen.getByTestId('cluster-row-1')).toBeInTheDocument();
  });

  it('shows error state instead of empty state when list load fails, including after retry', async () => {
    getClustersSafe.mockResolvedValue({
      success: false,
      error: 'sql: Scan error on column index 8',
    });
    renderMain();

    await waitFor(() => {
      expect(
        screen.getByText('sql: Scan error on column index 8'),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText(zh.cluster.list.emptyFirstTitle),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: zh.common.retry}));

    await waitFor(() => {
      expect(
        screen.getByText('sql: Scan error on column index 8'),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText(zh.cluster.list.emptyFirstTitle),
    ).not.toBeInTheDocument();
    expect(getClustersSafe.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it('surfaces config soft-load warnings from the list response', async () => {
    getClustersSafe.mockResolvedValueOnce({
      success: true,
      data: {
        total: 1,
        clusters: [
          makeCluster({
            id: 7,
            name: 'degraded',
            config_error: 'invalid config JSON',
          }),
        ],
        warnings: ['cluster "degraded" (id=7): invalid config JSON'],
      },
    });
    renderMain();

    await waitFor(() => {
      expect(screen.getByTestId('cluster-list-warnings')).toBeInTheDocument();
    });
    expect(screen.getByTestId('cluster-config-error-7')).toBeInTheDocument();
    expect(
      screen.getByText(zh.cluster.list.configReadFailed),
    ).toBeInTheDocument();
  });
});
