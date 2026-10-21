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
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import type {StatusResponse} from '@/lib/services/self-upgrade';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) =>
    values ? `${key}:${JSON.stringify(values)}` : key,
}));

// 页头动画依赖 matchMedia，与本测试无关 / the header animation needs matchMedia and is unrelated to this test
vi.mock('@/components/common/layout/WorkspaceHeader', () => ({
  WorkspaceHeader: ({
    title,
    actions,
  }: {
    title: React.ReactNode;
    actions?: React.ReactNode;
  }) => (
    <div>
      {title}
      {actions}
    </div>
  ),
}));

vi.mock('sonner', () => ({
  toast: {message: vi.fn(), success: vi.fn(), error: vi.fn()},
}));

const {selfUpgrade} = vi.hoisted(() => ({
  selfUpgrade: {
    getStatus: vi.fn(),
    getHistory: vi.fn(),
    execute: vi.fn(),
    check: vi.fn(),
    startDownload: vi.fn(),
    importOffline: vi.fn(),
    cancelStage: vi.fn(),
  },
}));
vi.mock('@/lib/services', () => ({default: {selfUpgrade}}));

import {SystemUpgradeMain} from '../SystemUpgradeMain';

function stagedStatus(extra: Partial<StatusResponse> = {}): StatusResponse {
  return {
    current_version: '1.0.0',
    latest_version: '1.0.1',
    update_available: true,
    checked: true,
    overall_status: 'staged',
    stage: {
      status: 'staged',
      progress: 100,
      target_version: '1.0.1',
      assets: [],
      cancellable: false,
    },
    mock_mode: false,
    deployment_type: 'host',
    agents: [
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
        online: false,
        up_to_date: false,
      },
    ],
    agents_outdated: 2,
    restart_plan: {
      control_plane_strategy: 'standalone',
      frontend_detected: false,
      restarts_agents: true,
      restarts_proxies: true,
      seatunnel_affected: false,
    },
    ...extra,
  };
}

beforeEach(() => {
  // jsdom 没有 matchMedia；页面动画（gsap）会读取它 / jsdom lacks matchMedia, which the page animations (gsap) read
  if (!window.matchMedia) {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia;
  }
  Object.values(selfUpgrade).forEach((fn) => fn.mockReset());
  selfUpgrade.getStatus.mockResolvedValue(stagedStatus());
  selfUpgrade.getHistory.mockResolvedValue([]);
  selfUpgrade.execute.mockResolvedValue({
    id: 't1',
    kind: 'execute',
    status: 'upgrading',
    cancellable: false,
  });
});
afterEach(cleanup);

describe('SystemUpgradeMain execute buttons', () => {
  it('dry-run button posts mode=dry_run', async () => {
    render(<SystemUpgradeMain />);
    const btn = await screen.findByTestId('self-upgrade-dry-run');
    await waitFor(() =>
      expect((btn as HTMLButtonElement).disabled).toBe(false),
    );
    fireEvent.click(btn);
    await waitFor(() => expect(selfUpgrade.execute).toHaveBeenCalledTimes(1));
    expect(selfUpgrade.execute).toHaveBeenCalledWith({mode: 'dry_run'});
  });

  it('start upgrade posts mode=apply only after the confirm dialog', async () => {
    render(<SystemUpgradeMain />);
    const btn = await screen.findByTestId('self-upgrade-start');
    await waitFor(() =>
      expect((btn as HTMLButtonElement).disabled).toBe(false),
    );
    fireEvent.click(btn);
    expect(selfUpgrade.execute).not.toHaveBeenCalled();
    const dialog = screen.getByTestId('self-upgrade-confirm');
    expect(within(dialog).getByTestId('confirm-nodes').textContent).toContain(
      'confirmNodesValue:{"total":2,"toUpgrade":2,"online":1,"offline":1}',
    );
    fireEvent.click(within(dialog).getByTestId('self-upgrade-confirm-apply'));
    await waitFor(() => expect(selfUpgrade.execute).toHaveBeenCalledTimes(1));
    expect(selfUpgrade.execute).toHaveBeenCalledWith({mode: 'apply'});
  });

  it('shows the dry-run result clearly as not applied and keeps the upgrade available', async () => {
    selfUpgrade.getStatus.mockResolvedValue(
      stagedStatus({
        active_task: {
          id: 't1',
          kind: 'execute',
          status: 'succeeded',
          execution_mode: 'dry_run',
          dry_run: true,
          cancellable: false,
          restarts: [
            {
              component: 'control_plane',
              strategy: 'standalone',
              status: 'planned',
            },
          ],
        },
      }),
    );
    render(<SystemUpgradeMain />);
    expect(await screen.findByTestId('dry-run-result')).toBeTruthy();
    expect(screen.getByText('dryRunDoneTitle')).toBeTruthy();
    expect(screen.getAllByTestId('execution-mode-badge')[0].textContent).toBe(
      'modeDryRun',
    );
    expect(screen.getByText('restartPlanned')).toBeTruthy();
    await waitFor(() =>
      expect(
        (screen.getByTestId('self-upgrade-start') as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
  });

  it('labels history records as apply or dry run', async () => {
    selfUpgrade.getHistory.mockResolvedValue([
      {
        id: 'a',
        started_at: '',
        completed_at: '',
        duration_ms: 1000,
        from_version: '1.0.0',
        target_version: '1.0.1',
        mode: 'offline',
        deployment_type: 'host',
        status: 'succeeded',
        execution_mode: 'apply',
      },
      {
        id: 'b',
        started_at: '',
        completed_at: '',
        duration_ms: 1000,
        from_version: '1.0.0',
        target_version: '1.0.1',
        mode: 'offline',
        deployment_type: 'host',
        status: 'succeeded',
        execution_mode: 'dry_run',
        dry_run: true,
      },
    ]);
    render(<SystemUpgradeMain />);
    await waitFor(() =>
      expect(screen.getAllByTestId('execution-mode-badge').length).toBe(2),
    );
    const badges = screen
      .getAllByTestId('execution-mode-badge')
      .map((b) => b.textContent);
    expect(badges).toEqual(['modeApply', 'modeDryRun']);
    expect(screen.getByText('historyDryRunDone')).toBeTruthy();
  });
});
