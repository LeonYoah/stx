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

import {render} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {ArcDateRangePicker} from '../ArcDateRangePicker';

type PickerCopy = {
  applyLabel?: string;
  cancelLabel?: string;
  dayLabel?: string;
  daysLabel?: string;
  noDatesLabel?: string;
  pickEndDateLabel?: string;
  presetsLabel?: string;
  previousMonthLabel?: string;
  nextMonthLabel?: string;
  startChooseEndLabel?: string;
};

const pickerProps: PickerCopy = {};

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh'}),
}));

vi.mock('@/components/arc/lib/use-today', () => ({
  useToday: () => new Date(2026, 10, 14),
}));

vi.mock('@/components/arc/date-range-picker/date-range-picker', () => ({
  DateRangePicker: (props: PickerCopy) => {
    Object.assign(pickerProps, props);
    return <div data-testid='date-range-picker' />;
  },
  defaultDateRangePresets: [],
}));

describe('ArcDateRangePicker', () => {
  it('passes localized Apply/Cancel and helper copy into DateRangePicker', () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
        <ArcDateRangePicker value={null} onChange={() => {}} />
      </NextIntlClientProvider>,
    );

    expect(pickerProps.applyLabel).toBe(zh.common.dateRange.apply);
    expect(pickerProps.cancelLabel).toBe(zh.common.dateRange.cancel);
    expect(pickerProps.dayLabel).toBe(zh.common.dateRange.day);
    expect(pickerProps.daysLabel).toBe(zh.common.dateRange.days);
    expect(pickerProps.noDatesLabel).toBe(zh.common.dateRange.noDates);
    expect(pickerProps.pickEndDateLabel).toBe(zh.common.dateRange.pickEndDate);
    expect(pickerProps.presetsLabel).toBe(zh.common.dateRange.presetsAria);
    expect(pickerProps.previousMonthLabel).toBe(zh.common.dateRange.previousMonth);
    expect(pickerProps.nextMonthLabel).toBe(zh.common.dateRange.nextMonth);
    expect(pickerProps.startChooseEndLabel).toBe(zh.common.dateRange.startChooseEnd);
  });
});
