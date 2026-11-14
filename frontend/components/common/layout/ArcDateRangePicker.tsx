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

'use client';

import {useMemo} from 'react';
import {useTranslations} from 'next-intl';
import {
  DateRangePicker,
  defaultDateRangePresets,
  type DateRange,
  type DateRangePreset,
} from '@/components/arc/date-range-picker/date-range-picker';
import {useToday} from '@/components/arc/lib/use-today';
import {useLocale} from '@/lib/i18n';

/** Arc 默认预设的顺序，标签走 next-intl。 / Order matches Arc defaults; labels come from next-intl. */
const PRESET_KEYS = [
  'today',
  'yesterday',
  'last7d',
  'last30d',
  'thisMonth',
  'lastMonth',
  'thisQuarter',
  'yearToDate',
] as const;

export interface ArcDateRangePickerProps {
  value: DateRange | null;
  onChange: (range: DateRange) => void;
  label?: string;
  placeholder?: string;
  className?: string;
  /** 为 false 时不限制最晚可选日。 / When false, the latest selectable day is not clamped. */
  clampToToday?: boolean;
}

/**
 * 全局日期范围选择器：接 Arc DateRangePicker，按当前语言切换 locale / 周起始日 / 预设文案。
 * App-wide date range picker: Arc DateRangePicker with locale, week start, and preset copy for the active language.
 */
export function ArcDateRangePicker({
  value,
  onChange,
  label,
  placeholder,
  className,
  clampToToday = true,
}: ArcDateRangePickerProps) {
  const {locale} = useLocale();
  const t = useTranslations('common.dateRange');
  const today = useToday();
  const intlLocale = locale === 'zh' ? 'zh-CN' : 'en-US';

  const presets = useMemo<DateRangePreset[]>(() => {
    return defaultDateRangePresets.map((preset, index) => {
      const key = PRESET_KEYS[index];
      return {
        ...preset,
        label: key ? t(`presets.${key}`) : preset.label,
      };
    });
  }, [t]);

  return (
    <DateRangePicker
      value={value}
      onChange={onChange}
      label={label ?? t('label')}
      placeholder={placeholder ?? t('placeholder')}
      locale={intlLocale}
      weekStartsOn={locale === 'zh' ? 1 : 0}
      presets={presets}
      maxDate={clampToToday ? today : undefined}
      className={className}
      cancelLabel={t('cancel')}
      applyLabel={t('apply')}
      dayLabel={t('day')}
      daysLabel={t('days')}
      noDatesLabel={t('noDates')}
      pickEndDateLabel={t('pickEndDate')}
      presetsLabel={t('presetsAria')}
      previousMonthLabel={t('previousMonth')}
      nextMonthLabel={t('nextMonth')}
      // `{date}` 留给选择器填入格式化开始日。 / `{date}` is filled in by the picker with the formatted start day.
      startChooseEndLabel={t('startChooseEnd', {date: '{date}'})}
    />
  );
}
