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
import zhMessages from '@/lib/i18n/locales/zh.json';
import {EngineEventChannelCard} from '../EngineEventChannelCard';

const getEventChannel = vi.hoisted(() => vi.fn());
const updateEventChannel = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {
    sync: {getEventChannel, updateEventChannel},
  },
}));

const baseStatus = {
  cluster_id: 5,
  enabled: false,
  state: 'disabled' as const,
  token_set: false,
  url: 'http://127.0.0.1:17800/api/v1/sync/engine-events/5',
  default_url: 'http://127.0.0.1:17800/api/v1/sync/engine-events/5',
  version: '3.1.0',
  supported: true,
  report_non_terminal: true,
  restart_required: false,
  received_count: 0,
  duplicate_count: 0,
  dropped_count: 0,
  parse_error_count: 0,
  auth_fail_count: 0,
};

function renderCard() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zhMessages}>
      <EngineEventChannelCard clusterId={5} />
    </NextIntlClientProvider>,
  );
}

describe('EngineEventChannelCard K8s 回调覆写', () => {
  beforeEach(() => {
    getEventChannel.mockReset();
    updateEventChannel.mockReset();
  });

  it('K8s 集群展示覆写输入框且回环地址给出警告', async () => {
    getEventChannel.mockResolvedValue({...baseStatus, is_kubernetes: true, stx_callback_url: ''});
    renderCard();
    await screen.findByTestId('k8s-callback-override');
    // 继承的是 127.0.0.1 默认地址 → 必须显示回环警告 / inherited loopback default must warn
    await screen.findByTestId('k8s-callback-loopback-warning');
  });

  it('非 K8s 集群不展示覆写区块', async () => {
    getEventChannel.mockResolvedValue({...baseStatus, is_kubernetes: false});
    renderCard();
    await waitFor(() => expect(getEventChannel).toHaveBeenCalled());
    expect(screen.queryByTestId('k8s-callback-override')).toBeNull();
  });

  it('保存覆写提交 stx_callback_url 字段', async () => {
    getEventChannel.mockResolvedValue({...baseStatus, is_kubernetes: true, stx_callback_url: ''});
    updateEventChannel.mockResolvedValue({
      ...baseStatus,
      is_kubernetes: true,
      stx_callback_url: 'http://10.0.2.2:17800',
    });
    renderCard();
    const input = (await screen.findByTestId('k8s-callback-override')).querySelector('input')!;
    fireEvent.change(input, {target: {value: 'http://10.0.2.2:17800'}});
    fireEvent.click(await screen.findByTestId('k8s-callback-save'));
    await waitFor(() =>
      expect(updateEventChannel).toHaveBeenCalledWith(5, {stx_callback_url: 'http://10.0.2.2:17800'}),
    );
  });

  it('填写非回环地址后警告消失', async () => {
    getEventChannel.mockResolvedValue({...baseStatus, is_kubernetes: true, stx_callback_url: ''});
    renderCard();
    const input = (await screen.findByTestId('k8s-callback-override')).querySelector('input')!;
    fireEvent.change(input, {target: {value: 'http://stx.stx-system.svc.cluster.local:17800'}});
    await waitFor(() =>
      expect(screen.queryByTestId('k8s-callback-loopback-warning')).toBeNull(),
    );
  });
});
