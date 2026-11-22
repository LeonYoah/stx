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
import {TooltipProvider} from '@/components/ui/tooltip';
import {MonitorConfigPanel} from '../MonitorConfigPanel';

const getMonitorConfig = vi.hoisted(() => vi.fn());
const updateMonitorConfig = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services/monitor', () => ({
  getMonitorConfig,
  updateMonitorConfig,
}));

const baseConfig = {
  id: 1,
  cluster_id: 11,
  auto_monitor: true,
  auto_restart: false,
  monitor_interval: 5,
  restart_delay: 10,
  max_restarts: 3,
  time_window: 300,
  cooldown_period: 1800,
  config_version: 2,
  last_sync_at: '2026-11-22T01:00:00Z',
  created_at: '2026-11-01T08:00:00Z',
  updated_at: '2026-11-22T01:00:00Z',
};

function renderPanel(onDirtyChange?: (dirty: boolean) => void) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <TooltipProvider>
        <MonitorConfigPanel
          clusterId={11}
          clusterName='demo'
          onDirtyChange={onDirtyChange}
        />
      </TooltipProvider>
    </NextIntlClientProvider>,
  );
}

describe('MonitorConfigPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getMonitorConfig.mockResolvedValue({...baseConfig});
  });

  it('persists auto_restart immediately via partial PUT without overwriting unsaved numeric inputs', async () => {
    const onDirtyChange = vi.fn();
    updateMonitorConfig.mockResolvedValue({
      ...baseConfig,
      auto_restart: true,
      config_version: 3,
    });

    renderPanel(onDirtyChange);
    await waitFor(() => {
      expect(screen.getByTestId('monitor-config-panel')).toBeInTheDocument();
    });

    // 先修改数值参数，确认上报脏态 / Edit numeric field first and verify dirty callback
    const intervalInput = screen.getByLabelText(
      `${zh.monitor.monitorInterval} (${zh.monitor.seconds})`,
    );
    fireEvent.change(intervalInput, {target: {value: '15'}});
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);

    // 拨动 auto_restart 开关：仅提交 { auto_restart: true }，不发送未保存的 15
    // Toggling auto_restart sends only { auto_restart: true } and preserves unsaved 15
    const sw = screen.getByTestId('monitor-auto-restart-switch');
    fireEvent.click(sw);

    await waitFor(() => {
      expect(updateMonitorConfig).toHaveBeenCalledWith(11, {
        auto_restart: true,
      });
    });
    expect(
      screen.getByTestId('monitor-auto-restart-status'),
    ).toHaveTextContent(zh.monitor.autoRestartSaved);
    expect((intervalInput as HTMLInputElement).value).toBe('15');
  });

  it('rolls back auto_restart switch and shows inline retry on failure', async () => {
    updateMonitorConfig.mockRejectedValueOnce(new Error('save rejected'));
    renderPanel();

    await waitFor(() => {
      expect(
        screen.getByTestId('monitor-auto-restart-switch'),
      ).toBeInTheDocument();
    });

    const sw = screen.getByTestId('monitor-auto-restart-switch');
    expect(sw).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(sw);

    await waitFor(() => {
      expect(
        screen.getByTestId('monitor-auto-restart-status'),
      ).toHaveTextContent('save rejected');
    });
    expect(sw).toHaveAttribute('aria-checked', 'false');

    // 点击重试成功 / Clicking retry succeeds
    updateMonitorConfig.mockResolvedValueOnce({
      ...baseConfig,
      auto_restart: true,
      config_version: 3,
    });
    fireEvent.click(screen.getByRole('button', {name: zh.monitoring.retry}));
    await waitFor(() => {
      expect(sw).toHaveAttribute('aria-checked', 'true');
    });
  });
});
