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
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {NextIntlClientProvider} from 'next-intl';
import zh from '@/lib/i18n/locales/zh.json';
import {ProcessEventList} from '../ProcessEventList';

const listProcessEvents = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services/monitor', () => ({
  listProcessEvents: (...args: unknown[]) => listProcessEvents(...args),
}));

vi.mock('@/lib/timezone', () => ({
  formatDateTimeMinuteTz: () => '2026/11/22 23:51',
}));

describe('ProcessEventList main-status linkage', () => {
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
    listProcessEvents.mockReset();
    listProcessEvents.mockResolvedValue({
      events: [
        {
          id: 1,
          event_type: 'restarted',
          hostname: 'stx181-host-1',
          ip: '127.0.0.1',
          process_name: 'stx181-install-1',
          role: 'master/worker',
          pid: 3143723,
          created_at: '2026-11-22T15:51:58Z',
        },
      ],
      total: 1,
    });
  });

  it('calls onMainStatusRefresh after a successful events reload', async () => {
    const onMainStatusRefresh = vi.fn();

    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <ProcessEventList
          clusterId={2}
          onMainStatusRefresh={onMainStatusRefresh}
        />
      </NextIntlClientProvider>,
    );

    await waitFor(() => {
      expect(listProcessEvents).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(onMainStatusRefresh).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole('button', {name: zh.common.refresh}));

    await waitFor(() => {
      expect(onMainStatusRefresh).toHaveBeenCalledTimes(2);
    });
  });
});
