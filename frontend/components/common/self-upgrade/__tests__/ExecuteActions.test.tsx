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

import React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react';
import {ExecuteActions} from '../ExecuteActions';
import {buildExecutePayload} from '@/lib/services/self-upgrade';
import type {
  AgentVersionInfo,
  RestartPlanInfo,
} from '@/lib/services/self-upgrade';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) =>
    values ? `${key}:${JSON.stringify(values)}` : key,
}));

const agents: AgentVersionInfo[] = [
  {
    host_id: 1,
    host_name: 'w1',
    version: '1.0.0',
    online: true,
    up_to_date: false,
  },
  {
    host_id: 2,
    host_name: 'w2',
    version: '1.0.0',
    online: true,
    up_to_date: false,
  },
  {
    host_id: 3,
    host_name: 'w3',
    version: '1.0.0',
    online: false,
    up_to_date: false,
  },
  {
    host_id: 4,
    host_name: 'w4',
    version: 'v1.0.1',
    online: true,
    up_to_date: true,
  },
];

const plan: RestartPlanInfo = {
  control_plane_strategy: 'systemd',
  frontend_detected: true,
  frontend_strategy: 'systemd',
  restarts_agents: true,
  restarts_proxies: true,
  seatunnel_affected: false,
};

afterEach(cleanup);

function renderActions(
  overrides: Partial<React.ComponentProps<typeof ExecuteActions>> = {},
) {
  const onExecute = vi.fn();
  render(
    <ExecuteActions
      targetVersion='v1.0.1'
      canUpgrade
      busy={false}
      restartPlan={plan}
      agents={agents}
      onExecute={onExecute}
      {...overrides}
    />,
  );
  return onExecute;
}

describe('ExecuteActions', () => {
  it('dry-run button sends dry_run immediately without a confirm dialog', () => {
    const onExecute = renderActions();
    fireEvent.click(screen.getByTestId('self-upgrade-dry-run'));
    expect(onExecute).toHaveBeenCalledTimes(1);
    expect(onExecute).toHaveBeenCalledWith('dry_run');
    expect(screen.queryByTestId('self-upgrade-confirm')).toBeNull();
    expect(screen.getByText('dryRunButton')).toBeTruthy();
  });

  it('start upgrade opens a confirm dialog first and only applies after confirming', () => {
    const onExecute = renderActions();
    fireEvent.click(screen.getByTestId('self-upgrade-start'));
    expect(onExecute).not.toHaveBeenCalled();

    const dialog = screen.getByTestId('self-upgrade-confirm');
    expect(
      within(dialog).getByText('confirmTitle:{"version":"1.0.1"}'),
    ).toBeTruthy();
    expect(within(dialog).getByTestId('confirm-target').textContent).toBe(
      'v1.0.1',
    );
    // 组件：控制面、Agent、Proxy，检测到随包前端时包含前端 / components incl. the bundled frontend when detected
    expect(within(dialog).getByTestId('confirm-components').textContent).toBe(
      'componentControlPlane · componentAgent · componentProxy · componentFrontend',
    );
    // 节点数包含离线节点，并提示离线节点重连后升级 / node count includes offline nodes, which upgrade on reconnect
    const nodes = within(dialog).getByTestId('confirm-nodes').textContent ?? '';
    expect(nodes).toContain(
      'confirmNodesValue:{"total":4,"toUpgrade":3,"online":3,"offline":1}',
    );
    expect(nodes).toContain('confirmOfflineNote:{"count":1}');
    // 将重启的进程与重启警告 / processes that restart and the restart warning
    const restarts =
      within(dialog).getByTestId('confirm-restarts').textContent ?? '';
    expect(restarts).toContain('restartControlPlane:{"strategy":"systemd"}');
    expect(restarts).toContain('restartFrontend');
    expect(restarts).toContain('restartAgents');
    expect(restarts).toContain('restartProxies');
    expect(within(dialog).getByText('confirmRestartWarning')).toBeTruthy();
    expect(
      within(dialog).getByTestId('confirm-seatunnel-safe').textContent,
    ).toContain('confirmSeaTunnelSafe');

    fireEvent.click(within(dialog).getByTestId('self-upgrade-confirm-apply'));
    expect(onExecute).toHaveBeenCalledTimes(1);
    expect(onExecute).toHaveBeenCalledWith('apply');
  });

  it('cancelling the confirm dialog sends nothing', () => {
    const onExecute = renderActions();
    fireEvent.click(screen.getByTestId('self-upgrade-start'));
    fireEvent.click(screen.getByText('confirmCancel'));
    expect(onExecute).not.toHaveBeenCalled();
    expect(screen.queryByTestId('self-upgrade-confirm')).toBeNull();
  });

  it('omits the frontend when none is detected and disables both buttons when blocked', () => {
    renderActions({restartPlan: {...plan, frontend_detected: false}});
    fireEvent.click(screen.getByTestId('self-upgrade-start'));
    const dialog = screen.getByTestId('self-upgrade-confirm');
    expect(
      within(dialog).getByTestId('confirm-components').textContent,
    ).not.toContain('componentFrontend');
    expect(
      within(dialog).getByTestId('confirm-restarts').textContent,
    ).not.toContain('restartFrontend');
    cleanup();
    renderActions({canUpgrade: false});
    expect(
      (screen.getByTestId('self-upgrade-start') as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByTestId('self-upgrade-dry-run') as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});

describe('buildExecutePayload', () => {
  it('maps each action to an explicit mode', () => {
    expect(buildExecutePayload('apply')).toEqual({mode: 'apply'});
    expect(buildExecutePayload('dry_run')).toEqual({mode: 'dry_run'});
    expect(buildExecutePayload('apply', 'verify_fail')).toEqual({
      mode: 'apply',
      simulate_failure: 'verify_fail',
    });
  });
});
