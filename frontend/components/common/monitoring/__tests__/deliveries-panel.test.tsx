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

// 投递记录二级视图：全局投递日志的服务端拉取、渠道|投递记录切换回调、
// 翻页与空态。
// Deliveries subview: server-side fetch of the global delivery log, the
// channels | deliveries switch callback, pagination, and the empty state.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {NotificationChannel} from '@/lib/services/monitoring';
import {DeliveriesPanel} from '../notify/DeliveriesPanel';

const listNotificationDeliveries = vi.fn();

vi.mock('@/lib/services', () => ({
  default: {
    monitoring: {
      listNotificationDeliveries: (...a: unknown[]) =>
        listNotificationDeliveries(...a),
    },
  },
}));

// jsdom 无 matchMedia，TableLoadingBar 的 GSAP 进度条会走到它。
// jsdom lacks matchMedia, which the TableLoadingBar GSAP progress bar reaches.
if (typeof window.matchMedia === 'undefined') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}

const channels: NotificationChannel[] = [
  {
    id: 3,
    name: 'ops-webhook',
    type: 'webhook',
    enabled: true,
    endpoint: 'https://x',
    secret: '',
    description: '',
    created_at: '2026-11-19T00:00:00Z',
    updated_at: '2026-11-19T00:00:00Z',
  },
];

function delivery() {
  return {
    id: 1,
    alert_id: 'remote:fp:1',
    source_type: 'metric',
    source_key: 'k',
    policy_id: 9,
    cluster_id: '6',
    cluster_name: 'prod-st-1',
    alert_name: '节点离线',
    channel_id: 3,
    channel_name: 'ops-webhook',
    event_type: 'firing',
    status: 'sent',
    attempt_count: 2,
    response_status_code: 200,
    sent_at: '2026-11-19T00:00:00Z',
    created_at: '2026-11-19T00:00:00Z',
    updated_at: '2026-11-19T00:00:00Z',
  };
}

function renderPanel(total = 1) {
  listNotificationDeliveries.mockResolvedValue({
    generated_at: '2026-11-19T00:00:00Z',
    page: 1,
    page_size: 10,
    total,
    deliveries: total > 0 ? [delivery()] : [],
  });
  const onViewChange = vi.fn();
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <DeliveriesPanel
        view='deliveries'
        onViewChange={onViewChange}
        channels={channels}
      />
    </NextIntlClientProvider>,
  );
  return onViewChange;
}

describe('DeliveriesPanel', () => {
  beforeEach(() => {
    listNotificationDeliveries.mockReset();
  });

  it('fetches the first page of global deliveries on mount', async () => {
    renderPanel();
    await waitFor(() =>
      expect(listNotificationDeliveries).toHaveBeenCalledWith({
        page: 1,
        page_size: 10,
      }),
    );
    expect(await screen.findByText('节点离线')).toBeInTheDocument();
    expect(screen.getByText('prod-st-1')).toBeInTheDocument();
    expect(screen.getByText('ops-webhook')).toBeInTheDocument();
    expect(screen.getByText('已送达')).toBeInTheDocument();
    expect(screen.getByText('触发')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
  });

  it('asks the parent to switch back to channels via the segmented control', async () => {
    const onViewChange = renderPanel();
    await screen.findByText('节点离线');
    fireEvent.click(screen.getByRole('button', {name: '通知渠道'}));
    expect(onViewChange).toHaveBeenCalledWith('channels');
  });

  it('requests the next page from the pagination footer', async () => {
    renderPanel(25);
    await screen.findByText('节点离线');
    fireEvent.click(screen.getByRole('button', {name: '下一页'}));
    await waitFor(() =>
      expect(listNotificationDeliveries).toHaveBeenCalledWith({
        page: 2,
        page_size: 10,
      }),
    );
  });

  it('shows the empty state when there are no deliveries', async () => {
    renderPanel(0);
    expect(await screen.findByText('暂无投递记录')).toBeInTheDocument();
  });
});
