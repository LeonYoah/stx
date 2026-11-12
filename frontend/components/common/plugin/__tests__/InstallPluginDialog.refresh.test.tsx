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

import {act, render, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {Plugin} from '@/lib/services/plugin';
import {InstallPluginDialog} from '../InstallPluginDialog';

const getDownloadStatus = vi.fn();
const listLocalPlugins = vi.fn();
const getClusters = vi.fn();
const listInstalledPlugins = vi.fn();

globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

vi.mock('@/lib/services/plugin', async () => {
  const actual = await vi.importActual<typeof import('@/lib/services/plugin')>(
    '@/lib/services/plugin',
  );
  return {
    ...actual,
    PluginService: {
      ...actual.PluginService,
      getDownloadStatus: (...args: unknown[]) => getDownloadStatus(...args),
      listLocalPlugins: (...args: unknown[]) => listLocalPlugins(...args),
      listInstalledPlugins: (...args: unknown[]) =>
        listInstalledPlugins(...args),
      downloadPlugin: vi.fn(),
      installPluginAndWait: vi.fn(),
      uninstallPlugin: vi.fn(),
    },
  };
});

vi.mock('@/lib/services/cluster', async () => {
  const actual = await vi.importActual<typeof import('@/lib/services/cluster')>(
    '@/lib/services/cluster',
  );
  return {
    ...actual,
    ClusterService: {
      ...actual.ClusterService,
      getClusters: (...args: unknown[]) => getClusters(...args),
      restartClusterSafe: vi.fn(),
    },
  };
});

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

const samplePlugin: Plugin = {
  name: 'jdbc',
  display_name: 'JDBC',
  category: 'source',
  version: '2.3.12',
  description: '',
  group_id: 'org.apache.seatunnel',
  artifact_id: 'connector-jdbc',
  dependencies: [],
};

function renderManageDialog(
  onDownloadComplete: () => void,
  open = true,
) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <InstallPluginDialog
        open={open}
        onOpenChange={vi.fn()}
        plugin={samplePlugin}
        version='2.3.12'
        selectedProfileKeys={[]}
        onDownloadComplete={onDownloadComplete}
      />
    </NextIntlClientProvider>,
  );
}

describe('InstallPluginDialog ready-on-open stability', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getClusters.mockResolvedValue({clusters: []});
    listInstalledPlugins.mockResolvedValue([]);
    getDownloadStatus.mockResolvedValue({
      plugin_name: 'jdbc',
      version: '2.3.12',
      status: 'not_started',
      progress: 0,
    });
    listLocalPlugins.mockResolvedValue([
      {name: 'jdbc', version: '2.3.12', size: 1, category: 'source'},
    ]);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not notify parent or keep requesting when plugin is already local', async () => {
    const onDownloadComplete = vi.fn();
    renderManageDialog(onDownloadComplete);

    await waitFor(() => {
      expect(getDownloadStatus).toHaveBeenCalled();
      expect(listLocalPlugins).toHaveBeenCalled();
      expect(getClusters).toHaveBeenCalledTimes(1);
    });

    const statusCallsAfterSettle = getDownloadStatus.mock.calls.length;

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });

    expect(onDownloadComplete).not.toHaveBeenCalled();
    expect(getDownloadStatus.mock.calls.length).toBe(statusCallsAfterSettle);
    expect(getDownloadStatus.mock.calls.length).toBeLessThanOrEqual(2);
    expect(getClusters).toHaveBeenCalledTimes(1);
  });

  it('idle 60s without download: clusters list stays at open-once', async () => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    const onDownloadComplete = vi.fn();
    renderManageDialog(onDownloadComplete);

    await waitFor(() => {
      expect(getClusters).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(getClusters).toHaveBeenCalledTimes(1);
    expect(listInstalledPlugins.mock.calls.length).toBeLessThanOrEqual(2);
    expect(onDownloadComplete).not.toHaveBeenCalled();
  });

  it('stops all dialog-owned requests within 15s after close', async () => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    const onDownloadComplete = vi.fn();
    const view = renderManageDialog(onDownloadComplete);

    await waitFor(() => {
      expect(getClusters).toHaveBeenCalledTimes(1);
    });

    const clustersAtOpen = getClusters.mock.calls.length;
    const installedAtOpen = listInstalledPlugins.mock.calls.length;
    const statusAtOpen = getDownloadStatus.mock.calls.length;

    view.rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <InstallPluginDialog
          open={false}
          onOpenChange={vi.fn()}
          plugin={samplePlugin}
          version='2.3.12'
          selectedProfileKeys={[]}
          onDownloadComplete={onDownloadComplete}
        />
      </NextIntlClientProvider>,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });

    expect(getClusters.mock.calls.length).toBe(clustersAtOpen);
    expect(listInstalledPlugins.mock.calls.length).toBe(installedAtOpen);
    expect(getDownloadStatus.mock.calls.length).toBe(statusAtOpen);
  });

  it('notifies parent once when in-flight download reaches completed', async () => {
    getDownloadStatus
      .mockResolvedValueOnce({
        plugin_name: 'jdbc',
        version: '2.3.12',
        status: 'downloading',
        progress: 10,
      })
      .mockResolvedValue({
        plugin_name: 'jdbc',
        version: '2.3.12',
        status: 'completed',
        progress: 100,
      });
    listLocalPlugins.mockResolvedValue([]);

    const onDownloadComplete = vi.fn();
    renderManageDialog(onDownloadComplete);

    await waitFor(() => {
      expect(getDownloadStatus.mock.calls.length).toBeGreaterThanOrEqual(1);
    });

    await waitFor(
      () => {
        expect(onDownloadComplete).toHaveBeenCalledTimes(1);
      },
      {timeout: 2500},
    );

    const callsAfterComplete = getDownloadStatus.mock.calls.length;

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 1200));
    });

    expect(getDownloadStatus.mock.calls.length).toBe(callsAfterComplete);
    expect(onDownloadComplete).toHaveBeenCalledTimes(1);
  });
});
