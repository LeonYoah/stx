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
 * 入口集成：挂载真实 ClusterDetail，点击节点行/页头重启并走 ConfirmDialog，
 * 断言停留页面即可从旧 PID/旧事件轮询到新 PID/新事件；旧 PID 不得被当成达标。
 * Entry integration: mount real ClusterDetail, click row/header restart through ConfirmDialog,
 * assert stay-on-page poll moves old PID/events → new; stale PID must not count as success.
 *
 * 模拟边界：只 mock 网络服务、路由、toast 与无关重型子树；不 mock ClusterDetail /
 * ClusterActions / ClusterNodesTab / ProcessEventList / useLifecycleStatusPoll /
 * useClusterLifecycle，也不手工注入 baselinePids。
 * Mock boundary: network/router/toast/unrelated heavy children only — never the entry
 * components/hooks above, and never hand-injected baselinePids.
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
  type ClusterInfo,
  type NodeInfo,
} from '@/lib/services/cluster/types';
import {ClusterDetail} from '../ClusterDetail';
import {LIFECYCLE_POLL_INTERVAL_MS} from '../lifecycle-status-poll';

const mockPush = vi.hoisted(() => vi.fn());
const mockReplace = vi.hoisted(() => vi.fn());

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

// 无关重型子树：不参与重启入口与事件区断言 / Unrelated heavy trees outside restart+events assertions
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

const getClusterSafe = vi.hoisted(() => vi.fn());
const getNodesSafe = vi.hoisted(() => vi.fn());
const getClusterStatusSafe = vi.hoisted(() => vi.fn());
const restartNodeSafe = vi.hoisted(() => vi.fn());
const restartClusterSafe = vi.hoisted(() => vi.fn());
const getNodeJavaRuntime = vi.hoisted(() => vi.fn());
const listOperations = vi.hoisted(() => vi.fn());
const getErrorGroupsSafe = vi.hoisted(() => vi.fn());
const getClusterOverviewSafe = vi.hoisted(() => vi.fn());
const getEventChannel = vi.hoisted(() => vi.fn());
const getRuntimeStorageSafe = vi.hoisted(() => vi.fn());
const getStxJavaProxyStatusSafe = vi.hoisted(() => vi.fn());
const listProcessEvents = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    cluster: {
      getClusterSafe,
      getNodesSafe,
      getClusterStatusSafe,
      restartNodeSafe,
      restartClusterSafe,
      getNodeJavaRuntime,
      getRuntimeStorageSafe,
      getStxJavaProxyStatusSafe,
      startClusterSafe: vi.fn(),
      stopClusterSafe: vi.fn(),
      startNodeSafe: vi.fn(),
      stopNodeSafe: vi.fn(),
      deleteClusterSafe: vi.fn(),
    },
    clusterScale: {
      listOperations,
    },
    diagnostics: {
      getErrorGroupsSafe,
    },
    monitoring: {
      getClusterOverviewSafe,
    },
    sync: {
      getEventChannel,
    },
    stUpgrade: {
      listTasksSafe: vi.fn().mockResolvedValue({success: true, data: {items: [], total: 0}}),
    },
  },
}));

vi.mock('@/lib/services/monitor', () => ({
  listProcessEvents: (...args: unknown[]) => listProcessEvents(...args),
}));

vi.mock('@/lib/timezone', () => ({
  formatDateTimeMinuteTz: (value: string) => value,
}));

const OLD_PID = 1876;
const NEW_PID = 2810;
const HOST_NAME = 't4-dnode-1';

function makeCluster(): ClusterInfo {
  return {
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
  };
}

function makeNode(pid: number): NodeInfo {
  return {
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
  } as NodeInfo;
}

function eventRow(partial: {
  id: number;
  event_type: string;
  pid?: number;
}) {
  return {
    id: partial.id,
    event_type: partial.event_type,
    hostname: HOST_NAME,
    ip: '10.0.0.4',
    process_name: 'seatunnel',
    role: 'master/worker',
    pid: partial.pid ?? 0,
    created_at: '2026-11-23T04:40:00Z',
  };
}

type RestartKind = 'none' | 'node' | 'cluster';

describe('ClusterDetail restart entry integration (node + header)', () => {
  let restartKind: RestartKind;
  /** 重启后 getNodesSafe 调用次数；前两次仍返回旧 PID。 / getNodesSafe calls after restart; first two stay on old PID. */
  let loadsAfterRestart: number;

  beforeEach(() => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    restartKind = 'none';
    loadsAfterRestart = 0;
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

    getClusterSafe.mockResolvedValue({success: true, data: makeCluster()});
    getClusterStatusSafe.mockResolvedValue({
      success: true,
      data: {
        cluster_id: 2,
        status: ClusterStatus.RUNNING,
        online_nodes: 1,
        total_nodes: 1,
        health_status: 'healthy',
      },
    });
    getNodesSafe.mockImplementation(async () => {
      if (restartKind === 'none') {
        return {success: true, data: [makeNode(OLD_PID)]};
      }
      loadsAfterRestart += 1;
      // 立即刷新 + 至少一拍轮询仍见旧 PID，才能证明 baseline 接线生效。
      // Immediate refresh + ≥1 poll tick still see old PID → proves baseline wiring.
      const pid = loadsAfterRestart <= 2 ? OLD_PID : NEW_PID;
      return {success: true, data: [makeNode(pid)]};
    });
    restartNodeSafe.mockImplementation(async () => {
      restartKind = 'node';
      loadsAfterRestart = 0;
      return {
        success: true,
        data: {
          success: true,
          message: 'Process stopped, auto-restart will start it',
          node_results: [],
        },
      };
    });
    restartClusterSafe.mockImplementation(async () => {
      restartKind = 'cluster';
      loadsAfterRestart = 0;
      return {
        success: true,
        data: {
          success: true,
          message: 'Operation completed successfully',
          node_results: [],
        },
      };
    });
    getNodeJavaRuntime.mockResolvedValue({
      success: true,
      data: {
        java_home: '/opt/java/openjdk',
        java_version: '11.0.33',
        source: 'process',
      },
    });
    listOperations.mockResolvedValue([]);
    getErrorGroupsSafe.mockResolvedValue({
      success: true,
      data: {items: [], total: 0},
    });
    getClusterOverviewSafe.mockResolvedValue({
      success: true,
      data: {stats: {active_alerts_1h: 0, restart_failed_events_24h: 0}},
    });
    getEventChannel.mockResolvedValue(null);
    getRuntimeStorageSafe.mockResolvedValue({success: true, data: null});
    getStxJavaProxyStatusSafe.mockResolvedValue({success: true, data: null});

    listProcessEvents.mockImplementation(async () => {
      if (restartKind === 'none') {
        return {
          events: [eventRow({id: 1, event_type: 'started', pid: OLD_PID})],
          total: 1,
        };
      }
      const stillStale = loadsAfterRestart < 2;
      if (stillStale) {
        if (restartKind === 'node') {
          return {
            events: [
              eventRow({id: 3, event_type: 'node_restart_requested'}),
              eventRow({id: 1, event_type: 'started', pid: OLD_PID}),
            ],
            total: 2,
          };
        }
        return {
          events: [
            eventRow({id: 3, event_type: 'cluster_restart_requested'}),
            eventRow({id: 1, event_type: 'started', pid: OLD_PID}),
          ],
          total: 2,
        };
      }
      if (restartKind === 'node') {
        return {
          events: [
            eventRow({id: 4, event_type: 'restarted', pid: NEW_PID}),
            eventRow({id: 3, event_type: 'node_restart_requested'}),
            eventRow({id: 1, event_type: 'started', pid: OLD_PID}),
          ],
          total: 3,
        };
      }
      return {
        events: [
          eventRow({id: 4, event_type: 'restarted', pid: NEW_PID}),
          eventRow({id: 3, event_type: 'cluster_restart_requested'}),
          eventRow({id: 1, event_type: 'started', pid: OLD_PID}),
        ],
        total: 3,
      };
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  function hostRow(): HTMLElement {
    // 主机名同时出现在节点表与事件表；用节点 ID 单元格定位节点行（busy 时重启按钮可能不可见）。
    // Hostname appears in nodes + events tables; locate the nodes row via its #id cell (restart btn may be busy).
    const matches = screen.getAllByText(HOST_NAME);
    for (const el of matches) {
      const row = el.closest('tr');
      if (!row) {
        continue;
      }
      if (within(row as HTMLElement).queryByText('#4')) {
        return row as HTMLElement;
      }
    }
    throw new Error('nodes-table host row missing');
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

  async function advanceUntilNewPid() {
    // 节点行 PID 仍为旧值；再跑若干轮询直到换新（且证明中途未停）。
    // Node-row PID still stale; keep polling until it changes (proves we did not stop early).
    expect(within(hostRow()).getByText(String(OLD_PID))).toBeTruthy();
    const callsWhileStale = getNodesSafe.mock.calls.length;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS);
    });
    expect(within(hostRow()).queryByText(String(NEW_PID))).toBeNull();
    expect(within(hostRow()).getByText(String(OLD_PID))).toBeTruthy();
    expect(getNodesSafe.mock.calls.length).toBeGreaterThan(callsWhileStale);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });
    await waitFor(() => {
      expect(within(hostRow()).getByText(String(NEW_PID))).toBeTruthy();
    });
    expect(within(hostRow()).queryByText(String(OLD_PID))).toBeNull();
  }

  it('node-row restart confirm: stay on page, old PID is not success, then PID+events update', async () => {
    await renderDetail();

    fireEvent.click(
      within(hostRow()).getByRole('button', {name: zh.cluster.restart}),
    );

    const dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByRole('heading', {
        name: zh.cluster.nodeRestartConfirmTitle,
      }),
    ).toBeTruthy();
    fireEvent.click(
      within(dialog).getByRole('button', {name: zh.cluster.restart}),
    );

    await waitFor(() => {
      expect(restartNodeSafe).toHaveBeenCalledWith(2, 4);
    });

    await waitFor(() => {
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
    });

    await advanceUntilNewPid();

    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
    });
    expect(restartClusterSafe).not.toHaveBeenCalled();
  });

  it('header cluster restart confirm: stay on page, wiring passes baseline without hand injection', async () => {
    await renderDetail();

    const restartButtons = screen.getAllByRole('button', {
      name: zh.cluster.restart,
    });
    const headerRestart = restartButtons.find((btn) => !btn.closest('tr'));
    expect(headerRestart).toBeTruthy();
    fireEvent.click(headerRestart!);

    const dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByRole('heading', {
        name: zh.cluster.restartConfirmTitle,
      }),
    ).toBeTruthy();
    fireEvent.click(
      within(dialog).getByRole('button', {
        name: zh.cluster.list.confirmRestartAction,
      }),
    );

    await waitFor(() => {
      expect(restartClusterSafe).toHaveBeenCalledWith(2);
    });

    await waitFor(() => {
      expect(
        screen.getByText(zh.monitor.events.clusterRestartRequested),
      ).toBeTruthy();
    });

    await advanceUntilNewPid();

    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.restarted)).toBeTruthy();
      expect(
        screen.getByText(zh.monitor.events.clusterRestartRequested),
      ).toBeTruthy();
    });
  });
});
