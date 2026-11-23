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

import {render, screen, fireEvent} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zhMessages from '@/lib/i18n/locales/zh.json';
import enMessages from '@/lib/i18n/locales/en.json';
import {
  ClusterStatus,
  DeploymentMode,
  HealthStatus,
  NodeRole,
  NodeStatus,
  type ClusterInfo,
  type NodeInfo,
} from '@/lib/services/cluster/types';
import {ClusterOverviewTab} from '../ClusterOverviewTab';

const baseCluster: ClusterInfo = {
  id: 12,
  name: 'stx22',
  cluster_code: 'stx22',
  description: '',
  deployment_mode: DeploymentMode.HYBRID,
  version: '3.1.0',
  status: ClusterStatus.RUNNING,
  install_dir: '/tmp/seatunnel-3.1.0',
  config: {},
  node_count: 1,
  online_nodes: 1,
  health_status: HealthStatus.UNHEALTHY,
  created_at: '2026-11-22T08:24:00Z',
  updated_at: '2026-11-22T09:03:00Z',
};

const baseNode: NodeInfo = {
  id: 13,
  cluster_id: 12,
  host_id: 10,
  host_name: 'ada',
  host_ip: '192.168.0.105',
  role: NodeRole.MASTER_WORKER,
  install_dir: '/tmp/seatunnel-3.1.0',
  hazelcast_port: 35801,
  api_port: 38080,
  worker_port: 0,
  status: NodeStatus.RUNNING,
  is_online: true,
  process_pid: 1733,
  process_status: 'running',
  created_at: '2026-11-22T08:24:00Z',
  updated_at: '2026-11-22T09:03:00Z',
};

const baseTabProps = {
  nodes: [baseNode],
  webUINode: baseNode,
  logModeSwitching: false,
  runtimeStorage: null,
  stxJavaProxy: null,
  onSwitchJobLogMode: vi.fn(),
  onGoNodes: vi.fn(),
  onGoRuntime: vi.fn(),
};

const baseStatus = {
  cluster_id: 12,
  cluster_name: 'stx22',
  status: ClusterStatus.RUNNING,
  health_status: HealthStatus.UNHEALTHY,
  total_nodes: 1,
  online_nodes: 1,
  offline_nodes: 0,
  running_nodes: 1,
  nodes: [],
};

// CJK 检测：断言英文环境下不出现中文字符（无中英混排）。
// CJK detector: asserts the English locale shows no Chinese characters (no mixed-language output).
const CJK_RE = /[\u3400-\u9fff\uf900-\ufaff]/;

describe('ClusterOverviewTab', () => {
  it('展示健康异常原因、节点端口与 JDK、事件通道状态及运维态势', () => {
    const onGoConfigs = vi.fn();
    const onGoOps = vi.fn();
    const onOpenMonitoring = vi.fn();
    const onOpenDiagnostics = vi.fn();
    const onStartInspection = vi.fn();

    render(
      <NextIntlClientProvider locale='zh' messages={zhMessages}>
        <ClusterOverviewTab
          cluster={baseCluster}
          clusterStatus={{
            cluster_id: 12,
            cluster_name: 'stx22',
            status: ClusterStatus.RUNNING,
            health_status: HealthStatus.UNHEALTHY,
            total_nodes: 1,
            online_nodes: 1,
            offline_nodes: 0,
            running_nodes: 1,
            health_reasons: ['节点 ada 的 SeaTunnel 引擎未就绪：coordinatorService is null'],
            nodes: [],
          }}
          nodes={[baseNode]}
          nodeJavaRuntimes={{
            13: {
              status: 'ok',
              version: 21,
              java_home: '/Library/Java/JavaVirtualMachines/jdk-21.jdk/Contents/Home',
            },
          }}
          webUINode={baseNode}
          enableHTTP={true}
          jobLogMode='per_job'
          logModeSwitching={false}
          runtimeStorage={null}
          stxJavaProxy={null}
          eventChannel={{
            cluster_id: 12,
            enabled: true,
            state: 'waiting_events',
            supported: true,
            url: 'http://127.0.0.1:17800/api/v1/sync/engine-events/12',
            version: '3.1.0',
            token_set: true,
            report_non_terminal: true,
            restart_required: false,
            received_count: 0,
            duplicate_count: 0,
            dropped_count: 0,
            parse_error_count: 0,
            auth_fail_count: 0,
          }}
          monitoringOverview={{
            generated_at: '2026-11-22T09:03:00Z',
            cluster: {
              cluster_id: 12,
              cluster_name: 'stx22',
              status: 'running',
              health_status: 'unhealthy',
            },
            stats: {
              total_nodes: 1,
              online_nodes: 1,
              offline_nodes: 0,
              crashed_events_24h: 0,
              restart_failed_events_24h: 0,
              active_alerts_1h: 2,
            },
            event_stats_24h: {} as never,
            event_stats_1h: {} as never,
            monitor_config: {
              id: 12,
              cluster_id: 12,
              auto_monitor: true,
              auto_restart: true,
              monitor_interval: 5,
              restart_delay: 10,
              max_restarts: 3,
              time_window: 300,
              cooldown_period: 1800,
              config_version: 1,
              last_sync_at: null,
              created_at: '',
              updated_at: '',
            },
            nodes: [],
            recent_events: [],
          }}
          diagnosticsGroupTotal={3}
          onSwitchJobLogMode={vi.fn()}
          onGoNodes={vi.fn()}
          onGoConfigs={onGoConfigs}
          onGoRuntime={vi.fn()}
          onGoOps={onGoOps}
          onOpenMonitoring={onOpenMonitoring}
          onOpenDiagnostics={onOpenDiagnostics}
          onStartInspection={onStartInspection}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.getByTestId('cluster-overview-health-alert')).toHaveTextContent(
      'coordinatorService is null',
    );
    expect(screen.getByText('HZ:35801 · HTTP:38080')).toBeInTheDocument();
    expect(screen.getByText('JDK 21')).toBeInTheDocument();
    expect(
      screen.getByTestId('cluster-overview-event-channel-summary'),
    ).toHaveTextContent('等待事件');

    fireEvent.click(screen.getByTestId('cluster-overview-event-channel-summary'));
    expect(onGoConfigs).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('进入运维中心'));
    expect(onGoOps).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('查看告警'));
    expect(onOpenMonitoring).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('查看错误'));
    expect(onOpenDiagnostics).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('发起巡检'));
    expect(onStartInspection).toHaveBeenCalledTimes(1);
  });

  it('健康集群不渲染健康异常提示条', () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zhMessages}>
        <ClusterOverviewTab
          cluster={{...baseCluster, health_status: HealthStatus.HEALTHY}}
          clusterStatus={{
            cluster_id: 12,
            cluster_name: 'stx22',
            status: ClusterStatus.RUNNING,
            health_status: HealthStatus.HEALTHY,
            total_nodes: 1,
            online_nodes: 1,
            offline_nodes: 0,
            running_nodes: 1,
            health_reasons: [],
            nodes: [],
          }}
          nodes={[baseNode]}
          webUINode={baseNode}
          logModeSwitching={false}
          runtimeStorage={null}
          stxJavaProxy={null}
          onSwitchJobLogMode={vi.fn()}
          onGoNodes={vi.fn()}
          onGoRuntime={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    expect(
      screen.queryByTestId('cluster-overview-health-alert'),
    ).not.toBeInTheDocument();
  });

  it('结构化健康原因在中文环境渲染纯中文', () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zhMessages}>
        <ClusterOverviewTab
          cluster={baseCluster}
          clusterStatus={{
            ...baseStatus,
            health_reasons: [
              'SeaTunnel engine on node ada is not ready: coordinatorService is null / 节点 ada 的 SeaTunnel 引擎未就绪：coordinatorService is null',
              'node worker-b is offline / 节点 worker-b 离线',
            ],
            health_reason_details: [
              {
                code: 'engine_not_ready',
                node: 'ada',
                detail: 'coordinatorService is null',
              },
              {code: 'node_offline', node: 'worker-b'},
            ],
          }}
          {...baseTabProps}
        />
      </NextIntlClientProvider>,
    );

    const alert = screen.getByTestId('cluster-overview-health-alert');
    expect(alert).toHaveTextContent(
      '节点 ada 的 SeaTunnel 引擎未就绪：coordinatorService is null',
    );
    expect(alert).toHaveTextContent('节点 worker-b 离线');
    // 中文环境不得出现英文半句（无中英混排）。
    // The zh locale must not surface the English half (no mixed-language output).
    expect(alert.textContent).not.toContain('engine on node');
    expect(alert.textContent).not.toContain('is offline');
  });

  it('结构化健康原因在英文环境渲染纯英文', () => {
    render(
      <NextIntlClientProvider locale='en' messages={enMessages}>
        <ClusterOverviewTab
          cluster={baseCluster}
          clusterStatus={{
            ...baseStatus,
            health_reasons: [
              'node worker-b is offline / 节点 worker-b 离线',
              'node ada sees only 1/2 cluster members (possible split brain) / 节点 ada 只看到 1/2 个集群成员（可能脑裂）',
            ],
            health_reason_details: [
              {code: 'node_offline', node: 'worker-b'},
              {
                code: 'member_split_brain',
                node: 'ada',
                observed_members: 1,
                expected_members: 2,
              },
            ],
          }}
          {...baseTabProps}
        />
      </NextIntlClientProvider>,
    );

    const alert = screen.getByTestId('cluster-overview-health-alert');
    expect(alert).toHaveTextContent('Node worker-b is offline');
    expect(alert).toHaveTextContent(
      'Node ada sees only 1/2 cluster members (possible split brain)',
    );
    expect(alert.textContent ?? '').not.toMatch(CJK_RE);
  });

  it('无结构化明细时历史双语原因按当前语言裁剪', () => {
    const bilingual =
      'SeaTunnel process on node ada is not running (status: stopped) / 节点 ada 的 SeaTunnel 进程未运行（状态：stopped）';

    const {unmount} = render(
      <NextIntlClientProvider locale='zh' messages={zhMessages}>
        <ClusterOverviewTab
          cluster={baseCluster}
          clusterStatus={{...baseStatus, health_reasons: [bilingual]}}
          {...baseTabProps}
        />
      </NextIntlClientProvider>,
    );
    let alert = screen.getByTestId('cluster-overview-health-alert');
    expect(alert).toHaveTextContent(
      '节点 ada 的 SeaTunnel 进程未运行（状态：stopped）',
    );
    expect(alert.textContent).not.toContain('SeaTunnel process on node');
    unmount();

    render(
      <NextIntlClientProvider locale='en' messages={enMessages}>
        <ClusterOverviewTab
          cluster={baseCluster}
          clusterStatus={{...baseStatus, health_reasons: [bilingual]}}
          {...baseTabProps}
        />
      </NextIntlClientProvider>,
    );
    alert = screen.getByTestId('cluster-overview-health-alert');
    expect(alert).toHaveTextContent(
      'SeaTunnel process on node ada is not running (status: stopped)',
    );
    expect(alert.textContent ?? '').not.toMatch(CJK_RE);
  });
});
