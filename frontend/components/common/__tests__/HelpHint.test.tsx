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

import {act, fireEvent, render, screen} from '@testing-library/react';
import {beforeAll, describe, expect, it, vi} from 'vitest';

import {HELP_HINT_DELAY_MS, HelpHint} from '../HelpHint';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) =>
    values ? `${key}:${JSON.stringify(values)}` : key,
}));

describe('HelpHint', () => {
  beforeAll(() => {
    // Radix Popper 依赖 ResizeObserver，jsdom 未提供 / jsdom lacks ResizeObserver
    globalThis.ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  });

  it('uses a short accessible name built from the label, not the hint body', () => {
    render(
      <HelpHint
        label='安装路径'
        content='不会覆盖系统标准模板'
        data-testid='hint'
      />,
    );
    const trigger = screen.getByRole('button', {
      name: 'helpLabelFor:{"label":"安装路径"}',
    });
    expect(trigger).toHaveAttribute('type', 'button');
    expect(trigger).toHaveAttribute('data-testid', 'hint');
    expect(trigger).not.toHaveAttribute('aria-label', '不会覆盖系统标准模板');
  });

  it('falls back to the generic help label', () => {
    render(<HelpHint content='正文' />);
    expect(screen.getByRole('button', {name: 'helpLabel'})).toBeTruthy();
  });

  it('opens on keyboard focus and describes the trigger with the hint body', async () => {
    render(<HelpHint label='Endpoint' content='收件人由对端系统决定' />);
    const trigger = screen.getByRole('button', {
      name: 'helpLabelFor:{"label":"Endpoint"}',
    });
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

  it('does not use a zero hover delay', () => {
    expect(HELP_HINT_DELAY_MS).toBeGreaterThan(0);
  });
});
