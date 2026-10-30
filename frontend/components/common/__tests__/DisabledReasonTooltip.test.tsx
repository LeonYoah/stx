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
import {beforeAll, describe, expect, it} from 'vitest';

import {DisabledReasonTooltip} from '../DisabledReasonTooltip';

describe('DisabledReasonTooltip', () => {
  beforeAll(() => {
    // Radix Popper 依赖 ResizeObserver，jsdom 未提供 / jsdom lacks ResizeObserver
    globalThis.ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  });

  it('shows no tooltip and no description when there is no reason', async () => {
    render(
      <DisabledReasonTooltip reason={null}>
        <button type='button'>go</button>
      </DisabledReasonTooltip>,
    );
    const button = screen.getByRole('button', {name: 'go'});
    await act(async () => {
      button.focus();
      fireEvent.focus(button);
    });
    expect(screen.queryByRole('tooltip')).toBeNull();
    expect(button).not.toHaveAttribute('aria-describedby');
  });

  it('keeps the same focused button when the reason appears or clears', async () => {
    const {rerender} = render(
      <DisabledReasonTooltip reason={null}>
        <button type='button'>go</button>
      </DisabledReasonTooltip>,
    );
    const button = screen.getByRole('button', {name: 'go'});
    await act(async () => {
      button.focus();
    });
    rerender(
      <DisabledReasonTooltip reason='先完成预检'>
        <button type='button' aria-disabled='true'>
          go
        </button>
      </DisabledReasonTooltip>,
    );
    expect(screen.getByRole('button', {name: 'go'})).toBe(button);
    expect(button).toHaveFocus();
    rerender(
      <DisabledReasonTooltip reason={null}>
        <button type='button'>go</button>
      </DisabledReasonTooltip>,
    );
    expect(screen.getByRole('button', {name: 'go'})).toBe(button);
    expect(button).toHaveFocus();
  });

  it('describes the focused trigger with the disabled reason', async () => {
    render(
      <DisabledReasonTooltip reason='先完成预检'>
        <button type='button' aria-disabled='true'>
          go
        </button>
      </DisabledReasonTooltip>,
    );
    const button = screen.getByRole('button', {name: 'go'});
    await act(async () => {
      button.focus();
      fireEvent.focus(button);
    });
    expect(await screen.findByRole('tooltip')).toHaveTextContent('先完成预检');
    expect(button).toHaveAccessibleDescription('先完成预检');
  });
});
