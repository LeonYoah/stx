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
import {
  TruncatedText,
  splitPathForMiddleEllipsis,
} from '../TruncatedText';
import {resetOverflowTooltipWarmth} from '../OverflowTooltip';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
  },
}));

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

describe('splitPathForMiddleEllipsis', () => {
  it('splits long unix paths while preserving distinguishing tail segments', () => {
    expect(
      splitPathForMiddleEllipsis(
        '/Library/Java/JavaVirtualMachines/jdk-26.jdk/Contents/Home',
      ),
    ).toEqual([
      '/Library/Java/JavaVirtualMachines',
      '/jdk-26.jdk/Contents/Home',
    ]);
  });

  it('keeps short paths as a single node without splitting', () => {
    expect(splitPathForMiddleEllipsis('/usr/lib/jvm/java-8')).toBeNull();
    expect(splitPathForMiddleEllipsis('/opt/jdk11')).toBeNull();
    expect(splitPathForMiddleEllipsis('plain-text-value')).toBeNull();
  });
});

describe('TruncatedText', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetOverflowTooltipWarmth();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('mounts zero tooltip portals at rest and when hovering non-overflowing text', () => {
    render(<TruncatedText text='Short job name' data-testid='cell' />);

    const cell = screen.getByTestId('cell');
    expect(screen.queryByRole('tooltip')).toBeNull();
    expect(cell.getAttribute('title')).toBeNull();

    mockElementOverflow(cell, {scrollWidth: 100, clientWidth: 120});
    fireEvent.pointerEnter(cell, {pointerType: 'mouse'});
    act(() => {
      vi.advanceTimersByTime(300);
    });

    expect(screen.queryByRole('tooltip')).toBeNull();
  });

  it('lazily mounts a single tooltip portal on overflow after hover delay and unmounts on leave or Escape', () => {
    render(
      <TruncatedText
        text='very-long-seatunnel-sync-job-name-that-overflows-narrow-column'
        data-testid='cell'
      />,
    );

    const cell = screen.getByTestId('cell');
    mockElementOverflow(cell, {scrollWidth: 320, clientWidth: 120});

    fireEvent.pointerEnter(cell, {pointerType: 'mouse'});
    expect(screen.queryByRole('tooltip')).toBeNull();

    act(() => {
      vi.advanceTimersByTime(180);
    });
    const tooltip = screen.getByRole('tooltip');
    expect(tooltip).toHaveTextContent(
      'very-long-seatunnel-sync-job-name-that-overflows-narrow-column',
    );

    fireEvent.keyDown(window, {key: 'Escape'});
    expect(screen.queryByRole('tooltip')).toBeNull();

    // 300ms 温热窗口内再次移入相邻单元格应立即打开，无额外延迟。
    // Within the 300ms warm window, hovering another overflowing cell opens immediately.
    fireEvent.pointerEnter(cell, {pointerType: 'mouse'});
    expect(screen.getByRole('tooltip')).toBeInTheDocument();

    fireEvent.pointerLeave(cell);
    expect(screen.queryByRole('tooltip')).toBeNull();
  });

  it('forwards data-detail-name for DetailHost fly-in and renders fallback when empty', () => {
    const {rerender} = render(
      <TruncatedText text='OOM Error Group' detailName data-testid='title' />,
    );
    expect(screen.getByTestId('title')).toHaveAttribute('data-detail-name');

    rerender(<TruncatedText text='' fallback='—' data-testid='title' />);
    expect(screen.getByTestId('title')).toHaveTextContent('—');
  });

  it('only renders copy button when copyable is enabled', () => {
    const {rerender} = render(<TruncatedText text='/opt/seatunnel/bin' />);
    expect(screen.queryByRole('button')).toBeNull();

    rerender(<TruncatedText text='/opt/seatunnel/bin' copyable />);
    expect(screen.getByRole('button', {name: 'copy'})).toBeInTheDocument();
  });
});
