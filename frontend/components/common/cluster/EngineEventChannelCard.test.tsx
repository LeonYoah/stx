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

import {render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TooltipProvider} from '@/components/ui/tooltip';
import type {SyncEventChannelStatus} from '@/lib/services/sync';
import {EngineEventChannelCard, eventChannelStateClass} from './EngineEventChannelCard';

const getEventChannel = vi.hoisted(() => vi.fn());
const updateEventChannel = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    sync: {getEventChannel, updateEventChannel},
  },
}));

function channelStatus(state: SyncEventChannelStatus['state']): SyncEventChannelStatus {
  return {
    cluster_id: 7,
    enabled: state !== 'disabled',
    state,
    token_set: state !== 'disabled',
    url: 'http://stx/api/v1/sync/engine-events/7',
    version: '3.0.0',
    supported: true,
    report_non_terminal: true,
    restart_required: state === 'pending_restart',
    received_count: 0,
    duplicate_count: 0,
    dropped_count: 0,
    parse_error_count: 0,
    auth_fail_count: 0,
  };
}

function renderCard(state: SyncEventChannelStatus['state']) {
  getEventChannel.mockResolvedValue(channelStatus(state));
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <TooltipProvider>
        <EngineEventChannelCard clusterId={7} />
      </TooltipProvider>
    </NextIntlClientProvider>,
  );
}

describe('EngineEventChannelCard', () => {
  it('pending_restart 显示待重启提示', async () => {
    renderCard('pending_restart');
    await waitFor(() => {
      expect(screen.getByText('待重启')).toBeInTheDocument();
      expect(screen.getByText(/滚动重启/)).toBeInTheDocument();
    });
  });

  it('waiting_events 显示等待事件提示，不再提示重启', async () => {
    renderCard('waiting_events');
    await waitFor(() => {
      expect(screen.getByText('等待事件')).toBeInTheDocument();
      expect(screen.getByText(/等待首个作业事件上报/)).toBeInTheDocument();
    });
    expect(screen.queryByText(/滚动重启/)).not.toBeInTheDocument();
  });

  it('waiting_events 使用 blue 色调类', () => {
    expect(eventChannelStateClass('waiting_events')).toContain('blue');
    expect(eventChannelStateClass('pending_restart')).toContain('amber');
  });
});
