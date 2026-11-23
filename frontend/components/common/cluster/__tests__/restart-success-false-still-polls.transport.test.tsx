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
 * 传输层回归：仅 mock apiClient，走真实 ClusterService / monitor.listProcessEvents /
 * ClusterDetail / useClusterLifecycle。复现 t4 AC2：POST restart 返回 data.success=false
 *（Operation completed with errors）时，旧代码不启动节点/事件轮询，停留页 PID/事件不更新。
 * Transport regression: mock only apiClient; real services + ClusterDetail. Reproduces t4 AC2
 * where restart POST data.success=false skipped lifecycle poll so stay-on-page UI stayed stale.
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

// 无关重型子树 / Unrelated heavy trees
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

const OLD_PID = 3196;
const NEW_PID = 4488;
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

function eventPayload(kind: 'old' | 'requested' | 'done', scope: 'node' | 'cluster') {
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
  const requestedType =
    scope === 'node' ? 'node_restart_requested' : 'cluster_restart_requested';
  if (kind === 'requested') {
    return {
      events: [
        {
          ...base,
          id: 3,
          event_type: requestedType,
          pid: 0,
          created_at: '2026-11-23T05:31:33Z',
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
        created_at: '2026-11-23T05:34:14Z',
      },
      {
        ...base,
        id: 3,
        event_type: requestedType,
        pid: 0,
        created_at: '2026-11-23T05:31:33Z',
      },
      base,
    ],
    total: 3,
    page: 1,
    page_size: 20,
  };
}

describe('transport: restart data.success=false still polls (AC2 class)', () => {
  let restartPosted: boolean;
  let restartScope: 'node' | 'cluster';
  let loadsAfterRestart: number;
  let eventPhase: 'old' | 'requested' | 'done';

  beforeEach(() => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    restartPosted = false;
    restartScope = 'cluster';
    loadsAfterRestart = 0;
    eventPhase = 'old';
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
          return axiosOk(eventPayload('old', restartScope));
        }
        if (loadsAfterRestart < 2) {
          eventPhase = 'requested';
          return axiosOk(eventPayload('requested', restartScope));
        }
        eventPhase = 'done';
        return axiosOk(eventPayload('done', restartScope));
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
      if (path.includes('/clusters/2/restart')) {
        restartPosted = true;
        restartScope = 'cluster';
        loadsAfterRestart = 0;
        // t4 页头实机：HTTP 200 但 data.success=false
        // t4 header restart: HTTP 200 with data.success=false
        return axiosOk({
          error_msg: '',
          data: {
            cluster_id: 2,
            operation: 'restart',
            success: false,
            message: 'Operation completed with errors',
            node_results: [
              {
                node_id: 4,
                host_id: 1,
                host_name: HOST_NAME,
                success: false,
                message: 'partial failure',
              },
            ],
          },
        });
      }
      if (path.includes('/nodes/4/restart')) {
        restartPosted = true;
        restartScope = 'node';
        loadsAfterRestart = 0;
        return axiosOk({
          error_msg: '',
          data: {
            cluster_id: 2,
            operation: 'restart',
            success: false,
            message: 'Operation completed with errors',
            node_results: [
              {
                node_id: 4,
                host_id: 1,
                host_name: HOST_NAME,
                success: false,
                message: 'partial failure',
              },
            ],
          },
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
    return apiGet.mock.calls.filter((c) => String(c[0]).includes(substr)).length;
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

  it('header restart with data.success=false still polls nodes+events to new PID', async () => {
    await renderDetail();
    const nodesBefore = countGets('/clusters/2/nodes');
    const eventsBefore = countGets('/clusters/2/events');

    const headerRestart = screen
      .getAllByRole('button', {name: zh.cluster.restart})
      .find((btn) => !btn.closest('tr'));
    expect(headerRestart).toBeTruthy();
    fireEvent.click(headerRestart!);

    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(
      within(dialog).getByRole('button', {
        name: zh.cluster.list.confirmRestartAction,
      }),
    );

    await waitFor(() => {
      expect(
        apiPost.mock.calls.some((c) =>
          String(c[0]).includes('/clusters/2/restart'),
        ),
      ).toBe(true);
    });

    // 旧闸门：success=false 不 onComplete → 不会出现后续轮询；本断言应在修复后成立。
    // Old gate: success=false skips onComplete → no follow-up poll; this must hold after the fix.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 4);
    });

    await waitFor(() => {
      expect(countGets('/clusters/2/nodes')).toBeGreaterThan(nodesBefore);
      expect(countGets('/clusters/2/events')).toBeGreaterThan(eventsBefore);
      expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
      expect(
        screen.getByText(zh.monitor.events.clusterRestartRequested),
      ).toBeTruthy();
    });
    expect(eventPhase).toBe('done');
  });

  it('node-row restart with data.success=false still polls to new PID+events', async () => {
    await renderDetail();
    const nodesBefore = countGets('/clusters/2/nodes');

    fireEvent.click(
      within(hostRow()).getByRole('button', {name: zh.cluster.restart}),
    );
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(
      within(dialog).getByRole('button', {name: zh.cluster.restart}),
    );

    await waitFor(() => {
      expect(
        apiPost.mock.calls.some((c) =>
          String(c[0]).includes('/nodes/4/restart'),
        ),
      ).toBe(true);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 4);
    });

    await waitFor(() => {
      expect(countGets('/clusters/2/nodes')).toBeGreaterThan(nodesBefore);
      expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
    });
  });
});
