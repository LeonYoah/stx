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
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {CompactTimeFilter} from '../CompactTimeFilter';

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh'}),
}));

vi.mock('@/components/arc/lib/use-today', () => ({
  useToday: () => new Date(2026, 10, 14),
}));

vi.mock('@/components/arc/date-range-picker/date-range-picker', () => ({
  DateRangePicker: ({
    label,
    onChange,
  }: {
    label: string;
    onChange?: (range: {start: Date; end: Date}) => void;
  }) => (
    <button
      type='button'
      aria-label={label}
      onClick={() => onChange?.({start: new Date(2026, 10, 1), end: new Date(2026, 10, 3)})}
    >
      mock-range
    </button>
  ),
  defaultDateRangePresets: [],
}));

function renderFilter(overrides: Partial<Parameters<typeof CompactTimeFilter>[0]> = {}) {
  const props = {
    startTime: '',
    endTime: '',
    onStartTimeChange: vi.fn(),
    onEndTimeChange: vi.fn(),
    onClear: vi.fn(),
    onApplyPreset: vi.fn(),
    ...overrides,
  };
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <CompactTimeFilter {...props} />
    </NextIntlClientProvider>,
  );
  return props;
}

describe('CompactTimeFilter', () => {
  it('applies a calendar range as start-of-day and end-of-day wall times', () => {
    const props = renderFilter();
    fireEvent.click(screen.getByRole('button', {name: zh.common.dateRange.label}));
    expect(props.onStartTimeChange).toHaveBeenCalledWith('2026-11-01T00:00');
    expect(props.onEndTimeChange).toHaveBeenCalledWith('2026-11-03T23:59:59.999');
  });

  it('keeps relative hour presets on the segmented control', () => {
    const props = renderFilter();
    fireEvent.click(screen.getByRole('button', {name: zh.monitoringCenter.alerts.timePresets.last1h}));
    expect(props.onApplyPreset).toHaveBeenCalledWith(60);
  });
});
