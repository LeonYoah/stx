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

import {fireEvent, render, screen, within} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {DateRangePicker} from '../date-range-picker';

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

if (typeof window.matchMedia !== 'function') {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener() {},
      removeListener() {},
      addEventListener() {},
      removeEventListener() {},
      dispatchEvent() {
        return false;
      },
    }) as MediaQueryList;
}

vi.mock('../../lib/use-today', () => ({
  useToday: () => new Date(2026, 10, 14),
}));

/** 当前（未在退场动画中的）滑动容器；退场元素没有 data-current。 / The live slide container; exiting ones lose data-current. */
const current = () => {
  const node = screen.getByRole('dialog').querySelector('[data-current]');
  if (!node) throw new Error('no current slide');
  return within(node as HTMLElement);
};

describe('DateRangePicker focus and year grid (STX)', () => {
  it('stays open when an in-panel pointer press pulls focus outside, but still closes on keyboard blur', () => {
    render(<DateRangePicker value={{start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)}} onChange={() => {}} />);
    const trigger = screen.getByRole('button', {name: /Date range/});
    fireEvent.click(trigger);
    const panel = screen.getByRole('dialog');
    const inside = panel.querySelector('[data-date]') as HTMLElement;
    // 面板内按下指针后 blur 到外部容器（Radix Dialog 的 FocusScope 场景），面板不该关。
    // A blur toward an outside element during an in-panel pointer press (Radix FocusScope) must not close.
    fireEvent.pointerDown(inside);
    fireEvent.blur(inside, {relatedTarget: document.body});
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    // 手势结束后的普通 blur（键盘移出）仍然关闭。 / A plain blur after the gesture (keyboard leaving) still closes.
    fireEvent.pointerUp(document.body);
    fireEvent.blur(inside, {relatedTarget: document.body});
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
  });

  it('opens a year grid from the month caption and jumps to the picked month', () => {
    render(<DateRangePicker value={{start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)}} onChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    fireEvent.click(current().getByRole('button', {name: 'November 2026'}));
    // 12 个月 + 返回日历的年份标题。 / Twelve months plus the year caption back to the calendar.
    expect(current().getByRole('button', {name: '2026'})).toBeInTheDocument();
    expect(current().getAllByRole('button').filter(b => /^\w+ 2026$/.test(b.getAttribute('aria-label') ?? '')).length).toBeGreaterThanOrEqual(12);
    // 按年翻页（箭头在滑块外，查整个面板）。 / Step by year (the arrows live outside the slide; query the panel).
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', {name: 'Previous year'}));
    fireEvent.click(current().getByRole('button', {name: 'March 2025'}));
    // 回到日历并定位到 2025-03。 / Back to the calendar, landed on March 2025.
    expect(current().getByRole('button', {name: 'March 2025'})).toBeInTheDocument();
    expect(current().getByRole('grid')).toHaveAccessibleName('March 2025');
  });

  it('disables out-of-range months and year arrows at the bounds', () => {
    render(
      <DateRangePicker
        value={{start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)}}
        onChange={() => {}}
        minDate={new Date(2026, 5, 10)}
        maxDate={new Date(2026, 10, 14)}
      />,
    );
    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    fireEvent.click(current().getByRole('button', {name: 'November 2026'}));
    // 超出 maxDate 的 12 月与「下一年」禁用；早于 minDate 的 5 月禁用。
    // December past maxDate and next year are disabled; May before minDate is disabled.
    expect(current().getByRole('button', {name: 'December 2026'})).toBeDisabled();
    expect(within(screen.getByRole('dialog')).getByRole('button', {name: 'Next year'})).toBeDisabled();
    expect(current().getByRole('button', {name: 'May 2026'})).toBeDisabled();
    expect(current().getByRole('button', {name: 'June 2026'})).toBeEnabled();
  });
});

describe('DateRangePicker copy props', () => {
  it('renders localized Apply and Cancel labels when the panel is open', () => {
    render(
      <DateRangePicker
        value={{start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)}}
        onChange={() => {}}
        applyLabel='应用'
        cancelLabel='取消'
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    expect(screen.getByRole('button', {name: '应用'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: '取消'})).toBeInTheDocument();
  });
});

describe('DateRangePicker withTime (STX extension)', () => {
  it('opens with the committed clock and applies a minute-precise range', () => {
    const onChange = vi.fn();
    render(
      <DateRangePicker
        value={{start: new Date(2026, 10, 14, 12, 13), end: new Date(2026, 10, 14, 12, 38, 59, 999)}}
        onChange={onChange}
        withTime
        startTimeLabel='开始时间'
        endTimeLabel='结束时间'
        applyLabel='应用'
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    const start = screen.getByLabelText('开始时间') as HTMLInputElement;
    const end = screen.getByLabelText('结束时间') as HTMLInputElement;
    expect(start.value).toBe('12:13');
    expect(end.value).toBe('12:38');

    fireEvent.change(end, {target: {value: '1245'}});
    fireEvent.blur(end);
    expect(end.value).toBe('12:45');
    fireEvent.click(screen.getByRole('button', {name: '应用'}));
    const range = onChange.mock.calls[0][0] as {start: Date; end: Date};
    expect([range.start.getHours(), range.start.getMinutes(), range.start.getSeconds()]).toEqual([12, 13, 0]);
    // 结束落在所选分钟的最后一毫秒，闭区间不丢最后一分钟。 / End lands on the chosen minute's last ms.
    expect([range.end.getHours(), range.end.getMinutes(), range.end.getSeconds(), range.end.getMilliseconds()])
      .toEqual([12, 45, 59, 999]);
  });

  it('blocks Apply when a same-day end is before the start', () => {
    render(
      <DateRangePicker
        value={{start: new Date(2026, 10, 14, 12, 13), end: new Date(2026, 10, 14, 12, 38, 59, 999)}}
        onChange={() => {}}
        withTime
        startTimeLabel='开始时间'
        endTimeLabel='结束时间'
        applyLabel='应用'
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    const end = screen.getByLabelText('结束时间') as HTMLInputElement;
    fireEvent.change(end, {target: {value: '9'}});
    fireEvent.keyDown(end, {key: 'Enter'});
    expect(end.value).toBe('09:00');
    expect(screen.getByRole('button', {name: '应用'})).toBeDisabled();
    expect(screen.getByLabelText('结束时间')).toHaveAttribute('aria-invalid', 'true');
  });

  it('keeps upstream whole-day behavior without withTime', () => {
    render(
      <DateRangePicker value={{start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)}} onChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    expect(screen.queryByLabelText('Start time')).not.toBeInTheDocument();
  });

  it('reverts invalid clock text and nudges with the arrow keys', () => {
    render(
      <DateRangePicker
        value={{start: new Date(2026, 10, 14, 23, 59), end: new Date(2026, 10, 15, 0, 0, 59, 999)}}
        onChange={() => {}}
        withTime
        startTimeLabel='开始时间'
        endTimeLabel='结束时间'
      />,
    );
    fireEvent.click(screen.getByRole('button', {name: /Date range/}));
    const start = screen.getByLabelText('开始时间') as HTMLInputElement;
    fireEvent.change(start, {target: {value: '25:61'}});
    fireEvent.blur(start);
    expect(start.value).toBe('23:59');
    // 跨过午夜回到 00:00；Shift 一次一小时。 / Wraps past midnight; Shift moves an hour.
    fireEvent.keyDown(start, {key: 'ArrowUp'});
    expect(start.value).toBe('00:00');
    fireEvent.keyDown(start, {key: 'ArrowDown', shiftKey: true});
    expect(start.value).toBe('23:00');
  });
});

