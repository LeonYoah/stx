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
