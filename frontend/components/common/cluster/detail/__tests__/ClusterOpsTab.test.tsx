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

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zhMessages from '@/lib/i18n/locales/zh.json';
import {ClusterOpsTab} from '../ClusterOpsTab';

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn(), replace: vi.fn()}),
  usePathname: () => '/clusters/1',
  useSearchParams: () => new URLSearchParams(),
}));

// MonitorConfigPanel 挂载即会请求 /monitor-config；用探针验证 K8s 下绝不挂载。
// MonitorConfigPanel fetches /monitor-config on mount; the probe asserts it never
// mounts for Kubernetes clusters.
const monitorPanelProbe = vi.hoisted(() => vi.fn());
vi.mock('../../MonitorConfigPanel', () => ({
  MonitorConfigPanel: (props: unknown) => {
    monitorPanelProbe(props);
    return <div data-testid='monitor-config-panel' />;
  },
}));

const baseProps = {
  clusterId: 1,
  clusterName: 'demo',
  view: 'diagnostics' as const,
  onViewChange: vi.fn(),
  onGuardDirtyChange: vi.fn(),
  diagnosticsGroups: [],
  diagnosticsGroupTotal: 0,
  diagnosticsLoading: false,
  inspectionStarting: false,
  onStartInspection: vi.fn(),
  upgradeTasks: [],
  upgradeTasksLoading: false,
  upgradeTasksPage: 1,
  upgradeTasksPageSize: 10,
  upgradeTasksTotal: 0,
  onUpgradeTasksPageChange: vi.fn(),
  onOpenUpgradeTask: vi.fn(),
};

function renderTab(props: Partial<Parameters<typeof ClusterOpsTab>[0]> = {}) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zhMessages}>
      <ClusterOpsTab {...baseProps} {...props} />
    </NextIntlClientProvider>,
  );
}

describe('ClusterOpsTab K8s guard boundary', () => {
  it('K8s 集群不渲染「守护与自愈」选项', () => {
    renderTab({isKubernetes: true});
    expect(screen.queryByText(zhMessages.cluster.list.opsViews.guard)).toBeNull();
    expect(screen.getByText(zhMessages.cluster.list.opsViews.diagnostics)).toBeTruthy();
    expect(screen.getByText(zhMessages.cluster.list.opsViews.upgrades)).toBeTruthy();
  });

  it('裸金属集群保留「守护与自愈」选项', () => {
    renderTab({isKubernetes: false});
    expect(screen.getByText(zhMessages.cluster.list.opsViews.guard)).toBeTruthy();
  });

  it('K8s 集群即使 view=guard 也不挂载 MonitorConfigPanel（不发 /monitor-config 请求）', () => {
    monitorPanelProbe.mockClear();
    renderTab({isKubernetes: true, view: 'guard'});
    expect(monitorPanelProbe).not.toHaveBeenCalled();
    expect(screen.queryByTestId('monitor-config-panel')).toBeNull();
  });

  it('裸金属集群 view=guard 正常挂载 MonitorConfigPanel', () => {
    monitorPanelProbe.mockClear();
    renderTab({isKubernetes: false, view: 'guard'});
    expect(monitorPanelProbe).toHaveBeenCalled();
  });
});
