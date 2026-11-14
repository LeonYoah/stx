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

import {fireEvent, render, screen} from '@testing-library/react';
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

