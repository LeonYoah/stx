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

import type {ReactElement} from 'react';
import {act, fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeAll, describe, expect, it} from 'vitest';

import zhMessages from '@/lib/i18n/locales/zh.json';

import {HelpHint} from '../HelpHint';

function renderWithIntl(ui: ReactElement) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zhMessages}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe('HelpHint', () => {
  beforeAll(() => {
    // Radix Popper 依赖 ResizeObserver，jsdom 未提供 / jsdom lacks ResizeObserver
    globalThis.ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  });

  it('renders a focusable button with a short accessible name', () => {
    renderWithIntl(
      <HelpHint
        label='自定义驱动'
        content='不会覆盖系统标准模板'
        data-testid='hint'
      />,
    );
    const trigger = screen.getByTestId('hint');
    expect(trigger).toHaveAttribute('type', 'button');
    expect(trigger).toHaveAttribute('aria-label', '自定义驱动 说明');
    expect(trigger.getAttribute('aria-label')).not.toContain(
      '不会覆盖系统标准模板',
    );
  });

  it('falls back to the generic help label without leaking the body', () => {
    renderWithIntl(<HelpHint content='收件人由对端系统决定' />);
    const trigger = screen.getByRole('button', {name: '说明'});
    expect(trigger.getAttribute('aria-label')).not.toContain('收件人');
  });

  it('opens the tooltip on keyboard focus and exposes the body via describedby', async () => {
    renderWithIntl(
      <HelpHint label='Endpoint' content='收件人由对端系统决定' />,
    );
    const trigger = screen.getByRole('button', {name: 'Endpoint 说明'});
    await act(async () => {
      trigger.focus();
      fireEvent.focus(trigger);
    });
    expect(trigger).toHaveFocus();
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      '收件人由对端系统决定',
    );
    expect(trigger).toHaveAccessibleDescription('收件人由对端系统决定');
  });
});
