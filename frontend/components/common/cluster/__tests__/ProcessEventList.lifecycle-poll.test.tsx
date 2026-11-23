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

import type {ComponentProps} from 'react';
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
import {ProcessEventList} from '../ProcessEventList';
import {
  LIFECYCLE_POLL_INTERVAL_MS,
  LIFECYCLE_POLL_TIMEOUT_MS,
} from '../lifecycle-status-poll';

function refreshIconSpinning(): boolean {
  const btn = screen.getByRole('button', {name: zh.common.refresh});
  const icon = btn.querySelector('svg');
  return Boolean(icon?.classList.contains('animate-spin'));
}

const listProcessEvents = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services/monitor', () => ({
  listProcessEvents: (...args: unknown[]) => listProcessEvents(...args),
}));

vi.mock('@/lib/timezone', () => ({
  formatDateTimeMinuteTz: (value: string) => value,
}));

function eventRow(partial: {
  id: number;
  event_type: string;
  created_at?: string;
  pid?: number;
}) {
  return {
    id: partial.id,
    event_type: partial.event_type,
    hostname: 'host-1',
    ip: '10.0.0.1',
    process_name: 'seatunnel',
    role: 'master/worker',
    pid: partial.pid ?? 1000 + partial.id,
    created_at: partial.created_at ?? '2026-11-23T03:00:00Z',
  };
}

function renderList(
  props: Partial<ComponentProps<typeof ProcessEventList>> = {},
) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <ProcessEventList clusterId={2} {...props} />
    </NextIntlClientProvider>,
  );
}

describe('ProcessEventList lifecycle event poll', () => {
  beforeEach(() => {
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
    vi.useFakeTimers({shouldAdvanceTime: true});
    listProcessEvents.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('AC1: after lifecyclePollEpoch bump, silently shows node_restart_requested then restarted without duplicates', async () => {
    listProcessEvents
      .mockResolvedValueOnce({
        events: [eventRow({id: 1, event_type: 'started'})],
        total: 1,
      })
      .mockResolvedValueOnce({
        events: [
          eventRow({id: 3, event_type: 'node_restart_requested'}),
          eventRow({id: 1, event_type: 'started'}),
        ],
        total: 2,
      })
      .mockResolvedValueOnce({
        events: [
          eventRow({id: 4, event_type: 'restarted'}),
          eventRow({id: 3, event_type: 'node_restart_requested'}),
          eventRow({id: 1, event_type: 'started'}),
        ],
        total: 3,
      });

    const onMainStatusRefresh = vi.fn();
    const {rerender} = renderList({
      lifecyclePollEpoch: 0,
      onMainStatusRefresh,
    });

    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(1);
    });
    expect(onMainStatusRefresh).toHaveBeenCalledTimes(1);

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList
          clusterId={2}
          lifecyclePollEpoch={1}
          onMainStatusRefresh={onMainStatusRefresh}
        />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(2);
    });
    expect(
      screen.getByText(zh.monitor.events.nodeRestartRequested),
    ).toBeTruthy();
    // 静默轮询不联动主状态，避免与详情页状态轮询叠打 / Silent poll skips main-status linkage.
    expect(onMainStatusRefresh).toHaveBeenCalledTimes(1);
    // 有数据时静默刷新不亮 TableLoadingBar（aria-busy）/ No loading bar flicker while data exists.
    const card = document.querySelector('[data-slot="table-card"]');
    expect(card?.getAttribute('aria-busy')).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS);
    });
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(3);
    });

    expect(
      screen.getAllByText(zh.monitor.events.nodeRestartRequested),
    ).toHaveLength(1);
    expect(screen.getAllByText(zh.monitor.events.restarted)).toHaveLength(1);
  });

  it('AC3: unmount stops further event polling', async () => {
    listProcessEvents.mockResolvedValue({
      events: [eventRow({id: 1, event_type: 'started'})],
      total: 1,
    });

    const {rerender, unmount} = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(1);
    });

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
    const callsBeforeUnmount = listProcessEvents.mock.calls.length;
    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 3);
    });
    expect(listProcessEvents.mock.calls.length).toBe(callsBeforeUnmount);
  });

  it('AC3/stale: ignores outdated responses so older payloads cannot overwrite newer ones', async () => {
    let resolveStale!: (value: unknown) => void;
    const stale = new Promise((resolve) => {
      resolveStale = resolve;
    });

    listProcessEvents
      .mockResolvedValueOnce({
        events: [eventRow({id: 1, event_type: 'started'})],
        total: 1,
      })
      .mockReturnValueOnce(stale)
      .mockResolvedValueOnce({
        events: [
          eventRow({id: 3, event_type: 'node_restart_requested'}),
        ],
        total: 1,
      });

    const {rerender} = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(1);
    });

    // 第一次 epoch 递增后请求挂起；再次递增开启新会话并完成较新响应。
    // First epoch bump leaves a request hanging; a second bump opens a new session that resolves newer.
    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(2);
    });

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={2} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(3);
    });
    expect(
      screen.getByText(zh.monitor.events.nodeRestartRequested),
    ).toBeTruthy();

    await act(async () => {
      resolveStale({
        events: [eventRow({id: 2, event_type: 'started'})],
        total: 1,
      });
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(
      screen.getByText(zh.monitor.events.nodeRestartRequested),
    ).toBeTruthy();
    expect(screen.queryByText(zh.monitor.events.started)).toBeNull();
  });

  it('stops polling after the shared lifecycle timeout window', async () => {
    listProcessEvents.mockResolvedValue({
      events: [eventRow({id: 1, event_type: 'started'})],
      total: 1,
    });

    const {rerender} = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(1);
    });
    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThanOrEqual(2);
    });

    const ticks =
      Math.floor(LIFECYCLE_POLL_TIMEOUT_MS / LIFECYCLE_POLL_INTERVAL_MS) + 2;
    for (let i = 0; i < ticks; i += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS);
      });
    }

    const callsAtTimeout = listProcessEvents.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS * 4);
    });
    expect(listProcessEvents.mock.calls.length).toBe(callsAtTimeout);
  });

  it('Strict-style remount with the same epoch still keeps silent polling alive', async () => {
    listProcessEvents.mockResolvedValue({
      events: [eventRow({id: 1, event_type: 'started'})],
      total: 1,
    });

    // 模拟 React Strict remount：卸载再挂载且 epoch 仍为 1，不得永久停轮询。
    // Simulate React Strict remount: unmount/remount with epoch still 1 must not permanently stop polling.
    const first = renderList({lifecyclePollEpoch: 1});
    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThanOrEqual(1);
    });
    first.unmount();

    renderList({lifecyclePollEpoch: 1});
    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
    const callsAfterRemount = listProcessEvents.mock.calls.length;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIFECYCLE_POLL_INTERVAL_MS);
    });
    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThan(
        callsAfterRemount,
      );
    });
  });

  it('keeps current page during silent lifecycle poll (no jump to page 1)', async () => {
    listProcessEvents.mockImplementation(
      (_clusterId: number, filter?: {page?: number}) => {
        const page = filter?.page ?? 1;
        return Promise.resolve({
          events: [
            eventRow({
              id: page * 100,
              event_type: page === 1 ? 'started' : 'stopped',
            }),
          ],
          total: 40,
        });
      },
    );

    const view = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.started)).toBeTruthy();
    });

    const pagination = await waitFor(() => {
      const el = document.querySelector('[data-slot="table-card-footer"]');
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    const page2 = within(pagination).getByRole('button', {name: '2'});
    await act(async () => {
      page2.click();
    });
    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.stopped)).toBeTruthy();
      const lastCall =
        listProcessEvents.mock.calls[listProcessEvents.mock.calls.length - 1];
      expect(lastCall?.[1]).toMatchObject({page: 2});
    });
    const callsAfterPageChange = listProcessEvents.mock.calls.length;

    view.rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(listProcessEvents.mock.calls.length).toBeGreaterThan(
        callsAfterPageChange,
      );
      const silentCall =
        listProcessEvents.mock.calls[listProcessEvents.mock.calls.length - 1];
      expect(silentCall?.[1]).toMatchObject({page: 2});
    });
    expect(screen.getByText(zh.monitor.events.stopped)).toBeTruthy();
  });

  it('WARN: silent poll must not leave refresh-icon loading after superseding a visible refresh', async () => {
    let resolveVisible!: (value: unknown) => void;
    const visible = new Promise((resolve) => {
      resolveVisible = resolve;
    });

    listProcessEvents
      .mockResolvedValueOnce({
        events: [eventRow({id: 1, event_type: 'started'})],
        total: 1,
      })
      .mockReturnValueOnce(visible)
      .mockResolvedValueOnce({
        events: [
          eventRow({id: 2, event_type: 'node_restart_requested'}),
        ],
        total: 1,
      });

    const {rerender} = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.started)).toBeTruthy();
    });
    expect(refreshIconSpinning()).toBe(false);

    fireEvent.click(screen.getByRole('button', {name: zh.common.refresh}));
    await waitFor(() => {
      expect(refreshIconSpinning()).toBe(true);
      expect(listProcessEvents).toHaveBeenCalledTimes(2);
    });

    // 静默轮询抢占序号时立即丢掉可见 loading；即使可见请求仍挂起也不能继续转。
    // Silent seq bump must drop visible loading immediately, even while the visible request hangs.
    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );
    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalledTimes(3);
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
      expect(refreshIconSpinning()).toBe(false);
    });
    const card = document.querySelector('[data-slot="table-card"]');
    expect(card?.getAttribute('aria-busy')).toBeNull();

    await act(async () => {
      resolveVisible({
        events: [eventRow({id: 1, event_type: 'started'})],
        total: 1,
      });
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(refreshIconSpinning()).toBe(false);
    expect(
      screen.getByText(zh.monitor.events.nodeRestartRequested),
    ).toBeTruthy();
  });

  it('WARN: silent poll success clears a prior error card', async () => {
    listProcessEvents
      .mockResolvedValueOnce({
        events: [eventRow({id: 1, event_type: 'started'})],
        total: 1,
      })
      .mockRejectedValueOnce(new Error('boom-load-failed'))
      .mockResolvedValueOnce({
        events: [
          eventRow({id: 3, event_type: 'node_restart_requested'}),
        ],
        total: 1,
      });

    const {rerender} = renderList({lifecyclePollEpoch: 0});
    await waitFor(() => {
      expect(screen.getByText(zh.monitor.events.started)).toBeTruthy();
    });

    fireEvent.click(screen.getByRole('button', {name: zh.common.refresh}));
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeTruthy();
      expect(screen.getByText('boom-load-failed')).toBeTruthy();
    });

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList clusterId={2} lifecyclePollEpoch={1} />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(
        screen.getByText(zh.monitor.events.nodeRestartRequested),
      ).toBeTruthy();
    });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByText('boom-load-failed')).toBeNull();
    expect(refreshIconSpinning()).toBe(false);
  });
});
