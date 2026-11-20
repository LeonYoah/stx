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
import {describe, expect, it} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {HomeAttentionItem} from '@/lib/services/dashboard';
import {AttentionStrip} from '../AttentionStrip';

const renderStrip = (items: HomeAttentionItem[]) =>
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <AttentionStrip items={items} jobHref={() => '/jobs/1'} />
    </NextIntlClientProvider>,
  );

describe('AttentionStrip', () => {
  it('renders a borderless one-liner when nothing needs attention', () => {
    renderStrip([]);
    const strip = screen.getByTestId('home-attention');
    expect(strip.tagName).toBe('P');
    expect(strip.className).not.toContain('border');
    expect(strip.textContent).toBe('需要处理 · 暂无需要处理的事项');
  });

  it('keeps the framed strip with item chips when items exist', () => {
    renderStrip([
      {
        kind: 'job_failed',
        severity: 'critical',
        title: 'orders-sync',
        job_id: 1,
        task_id: 2,
        at: '2026-11-14T12:00:00Z',
        viewable: true,
      },
    ]);
    const strip = screen.getByTestId('home-attention');
    expect(strip.tagName).toBe('DIV');
    expect(strip.className).toContain('border');
    expect(strip.textContent).toContain('作业失败');
    expect(strip.textContent).toContain('orders-sync');
  });
});
