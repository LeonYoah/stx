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
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {NeutralChip, SoftChip} from '../SoftChip';
import {resetOverflowTooltipWarmth} from '../OverflowTooltip';

function mockElementOverflow(
  el: HTMLElement,
  {scrollWidth, clientWidth}: {scrollWidth: number; clientWidth: number},
) {
  Object.defineProperty(el, 'scrollWidth', {
    configurable: true,
    value: scrollWidth,
  });
  Object.defineProperty(el, 'clientWidth', {
    configurable: true,
    value: clientWidth,
  });
}

describe('SoftChip & NeutralChip overflow tooltips', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetOverflowTooltipWarmth();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('NeutralChip omits native title and lazily shows full exception class on overflow', () => {
    const exceptionClass =
      'org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException';
    render(<NeutralChip title={exceptionClass}>{exceptionClass}</NeutralChip>);

    const inner = screen.getByText(exceptionClass);
    const badge = inner.closest('[data-slot="badge"]') as HTMLElement;
    expect(badge.getAttribute('title')).toBeNull();
    expect(screen.queryByRole('tooltip')).toBeNull();

    mockElementOverflow(inner, {scrollWidth: 420, clientWidth: 110});
    fireEvent.pointerEnter(badge, {pointerType: 'mouse'});
    act(() => {
      vi.advanceTimersByTime(180);
    });

    expect(screen.getByRole('tooltip')).toHaveTextContent(exceptionClass);

    fireEvent.pointerDown(badge);
    expect(screen.queryByRole('tooltip')).toBeNull();
  });

  it('SoftChip with truncate shows full policy label on overflow and stays silent when it fits', () => {
    const label = '自动 · e2e-告警触发巡检';
    render(
      <SoftChip tone='blue' truncate>
        {label}
      </SoftChip>,
    );

    const inner = screen.getByText(label);
    const chip = inner.parentElement as HTMLElement;
    expect(chip.getAttribute('title')).toBeNull();

    // 未溢出时不弹出 / No tooltip when content fits
    mockElementOverflow(inner, {scrollWidth: 90, clientWidth: 120});
    fireEvent.pointerEnter(chip, {pointerType: 'mouse'});
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.queryByRole('tooltip')).toBeNull();

    // 溢出后弹出完整原文 / Shows full text when overflowing
    mockElementOverflow(inner, {scrollWidth: 180, clientWidth: 80});
    fireEvent.pointerEnter(chip, {pointerType: 'mouse'});
    act(() => {
      vi.advanceTimersByTime(180);
    });
    expect(screen.getByRole('tooltip')).toHaveTextContent(label);
  });
});
