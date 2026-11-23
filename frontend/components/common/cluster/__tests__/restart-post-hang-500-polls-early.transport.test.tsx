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
 * t4 实机边界：POST /nodes/:id/restart 挂起 ~30s 后 HTTP 500，期间零 GET。
 * 仅 mock apiClient；确认后轮询必须在 POST settle **之前**开始，并能刷到新 PID/事件。
 * t4 boundary: POST /nodes/:id/restart hangs ~30s then HTTP 500 with zero GETs.
 * Mock apiClient only; poll must start before POST settles and converge to new PID/events.
 */
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {NextIntlClientProvider} from 'next-intl';
import zh from '@/lib/i18n/locales/zh.json';
import {
  ClusterStatus,
  DeploymentMode,
  NodeRole,
  NodeStatus,
} from '@/lib/services/cluster/types';
import {ClusterDetail} from '../ClusterDetail';
import {LIFECYCLE_POLL_INTERVAL_MS} from '../lifecycle-status-poll';

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

const OLD_PID = 4488;
const NEW_PID = 4986;
const HOST_NAME = 't4-dnode-1';

function axiosOk<T>(data: T) {
  return Promise.resolve({data, status: 200, statusText: 'OK', headers: {}});
}

function clusterPayload() {
  return {
    error_msg: '',
    data: {
      id: 2,
      name: 't4-186-events',
      cluster_code: 't4-186',
      description: '',
      deployment_mode: DeploymentMode.HYBRID,
      version: '2.3.12',
      status: ClusterStatus.RUNNING,
      install_dir: '/opt/seatunnel',
      config: {},
      node_count: 1,
      online_nodes: 1,
      health_status: 'healthy',
      created_at: '2026-11-23T04:00:00Z',
      updated_at: '2026-11-23T04:00:00Z',
    },
  };
}

function nodePayload(pid: number) {
  return {
    error_msg: '',
    data: [
      {
        id: 4,
        cluster_id: 2,
        host_id: 1,
        host_name: HOST_NAME,
        host_ip: '10.0.0.4',
        role: NodeRole.MASTER_WORKER,
        status: NodeStatus.RUNNING,
        install_dir: '/opt/seatunnel',
        process_pid: pid,
        api_port: 5801,
        is_online: true,
        created_at: '2026-11-23T04:00:00Z',
        updated_at: '2026-11-23T04:00:00Z',
      },
    ],
  };
}

function statusPayload() {
  return {
    error_msg: '',
    data: {
      cluster_id: 2,
      status: ClusterStatus.RUNNING,
      online_nodes: 1,
      total_nodes: 1,
      health_status: 'healthy',
    },
  };
}

function eventPayload(kind: 'old' | 'requested' | 'done') {
  const base = {
    id: 1,
    event_type: 'started',
    hostname: HOST_NAME,
    ip: '10.0.0.4',
    process_name: 'seatunnel',
    role: 'master/worker',
    pid: OLD_PID,
    created_at: '2026-11-23T04:40:00Z',
  };
  if (kind === 'old') {
    return {events: [base], total: 1, page: 1, page_size: 20};
  }
  if (kind === 'requested') {
    return {
      events: [
        {
          ...base,
          id: 3,
          event_type: 'node_restart_requested',
          pid: 0,
          created_at: '2026-11-23T05:43:17Z',
        },
        base,
      ],
      total: 2,
      page: 1,
      page_size: 20,
    };
  }
  return {
    events: [
      {
        ...base,
        id: 4,
        event_type: 'restarted',
        pid: NEW_PID,
        created_at: '2026-11-23T05:44:04Z',
      },
      {
        ...base,
        id: 3,
        event_type: 'node_restart_requested',
        pid: 0,
        created_at: '2026-11-23T05:43:17Z',
      },
      base,
    ],
    total: 3,
    page: 1,
    page_size: 20,
  };
}

describe('transport: POST hang→500 still polls early (t4 AC1)', () => {
  let restartPosted: boolean;
  let postSettled: boolean;
  let loadsAfterRestart: number;
  let rejectPost: ((reason?: unknown) => void) | null;

  beforeEach(() => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    restartPosted = false;
    postSettled = false;
    loadsAfterRestart = 0;
    rejectPost = null;
    apiGet.mockReset();
    apiPost.mockReset();

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
        if (!restartPosted) {
          return axiosOk(nodePayload(OLD_PID));
        }
        loadsAfterRestart += 1;
        const pid = loadsAfterRestart <= 2 ? OLD_PID : NEW_PID;
        return axiosOk(nodePayload(pid));
      }
      if (path.match(/\/clusters\/2$/)) {
        return axiosOk(clusterPayload());
      }
      if (path.includes('/clusters/2/status')) {
        return axiosOk(statusPayload());
      }
      if (path.includes('/clusters/2/events')) {
        if (!restartPosted) {
          return axiosOk(eventPayload('old'));
        }
        if (loadsAfterRestart < 2) {
          return axiosOk(eventPayload('requested'));
        }
        return axiosOk(eventPayload('done'));
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
      if (path.includes('event-channel') || path.includes('sync')) {
        return axiosOk({error_msg: '', data: null});
      }
      if (path.includes('runtime-storage') || path.includes('stx-java-proxy')) {
        return axiosOk({error_msg: '', data: null});
      }
      if (path.includes('scale') || path.includes('operations')) {
        return axiosOk([]);
      }
      if (path.includes('upgrade') || path.includes('st-upgrade')) {
        return axiosOk({error_msg: '', data: {items: [], total: 0}});
      }
      return axiosOk({error_msg: '', data: null});
    });

    apiPost.mockImplementation((url: string) => {
      const path = String(url);
      if (
        path.includes('/clusters/2/restart') ||
        path.includes('/nodes/4/restart')
      ) {
        restartPosted = true;
        loadsAfterRestart = 0;
        return new Promise((_resolve, reject) => {
          rejectPost = reject;
        });
      }
      return axiosOk({error_msg: '', data: {}});
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  function hostRow(): HTMLElement {
    const matches = screen.getAllByText(HOST_NAME);
    for (const el of matches) {
      const row = el.closest('tr');
      if (row && within(row as HTMLElement).queryByText('#4')) {
        return row as HTMLElement;
      }
    }
    throw new Error('nodes-table host row missing');
  }

  function countGets(substr: string): number {
    return apiGet.mock.calls.filter((c) => String(c[0]).includes(substr))
      .length;
  }

  async function renderDetail() {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ClusterDetail clusterId={2} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(within(hostRow()).getByText(String(OLD_PID))).toBeTruthy();
    });
  }

  function settlePostWith500() {
    const err = Object.assign(new Error('服务器内部错误，请稍后重试'), {
      name: 'ApiError',
      status: 500,
    });
    postSettled = true;
    rejectPost?.(err);
  }

  it('node-row: polls nodes+events before hung POST returns 500, then shows new PID', async () => {
    await renderDetail();
    const nodesBefore = countGets('/clusters/2/nodes');
    const eventsBefore = countGets('/clusters/2/events');

    fireEvent.click(
      within(hostRow()).getByRole('button', {name: zh.cluster.restart}),
    );
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(
      within(dialog).getByRole('button', {name: zh.cluster.restart}),
    );

    await waitFor(() => {
      expect(restartPosted).toBe(true);
      expect(postSettled).toBe(false);
    });

    // POST 仍挂起时就应出现后续 GET（旧实现会等到 500 后才可能 refresh）。
    // Follow-up GETs must appear while POST is still pending (old code waited for 500).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });

    await waitFor(() => {
      expect(postSettled).toBe(false);
      expect(countGets('/clusters/2/nodes')).toBeGreaterThan(nodesBefore);
      expect(countGets('/clusters/2/events')).toBeGreaterThan(eventsBefore);
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });
    await waitFor(() => {
      expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
    });

    settlePostWith500();
    await act(async () => {
      await Promise.resolve();
    });
    // 500 不得取消已在跑的轮询；新 PID 仍在。
    // 500 must not cancel the in-flight poll; new PID stays.
    expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
  });

  it('header: polls before hung cluster restart POST returns 500', async () => {
    await renderDetail();
    const nodesBefore = countGets('/clusters/2/nodes');

    const headerRestart = screen
      .getAllByRole('button', {name: zh.cluster.restart})
      .find((btn) => !btn.closest('tr'));
    fireEvent.click(headerRestart!);
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(
      within(dialog).getByRole('button', {
        name: zh.cluster.list.confirmRestartAction,
      }),
    );

    await waitFor(() => {
      expect(restartPosted).toBe(true);
      expect(postSettled).toBe(false);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });

    await waitFor(() => {
      expect(postSettled).toBe(false);
      expect(countGets('/clusters/2/nodes')).toBeGreaterThan(nodesBefore);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });
    await waitFor(() => {
      expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
    });

    settlePostWith500();
    await act(async () => {
      await Promise.resolve();
    });
    expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
  });
});
