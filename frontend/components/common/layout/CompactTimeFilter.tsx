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
import {X} from 'lucide-react';
import type {DateRange} from '@/components/arc/date-range-picker/date-range-picker';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {cn} from '@/lib/utils';
import {formatDateTimeLocalInput} from '@/lib/timezone';
import {dateRangeToWallTimes, wallTimesToDateRange} from '@/lib/date-range';
import {ArcDateRangePicker} from './ArcDateRangePicker';

export interface TimePresetOption {
  label: string;
  minutes: number;
}

export interface CompactTimeFilterProps {
  // 开始时间过滤字符串（YYYY-MM-DDTHH:mm）
  // Start time filter value in YYYY-MM-DDTHH:mm format
  startTime: string;
  // 结束时间过滤字符串（YYYY-MM-DDTHH:mm）
  // End time filter value in YYYY-MM-DDTHH:mm format
  endTime: string;
  // 开始时间变更回调
  // Callback when start time is updated
  onStartTimeChange: (val: string) => void;
  // 结束时间变更回调
  // Callback when end time is updated
  onEndTimeChange: (val: string) => void;
  // 清除起止时间回调
  // Callback when clearing time filters
  onClear: () => void;
  // 当前激活的快捷预设分钟数（为 null 表示自定义或未选）
  // Currently active quick preset in minutes (null indicates custom or unset)
  activePreset?: number | null;
  // 选择时间快捷预设回调
  // Callback when a quick time preset is clicked
  onApplyPreset?: (minutes: number) => void;
  // 自定义预设列表（可选，默认包含 1h、6h、24h、7d）
  // Custom preset options (optional, defaults to 1h, 6h, 24h, 7d)
  presets?: TimePresetOption[];
  // 自定义容器样式
  // Custom container class name
  className?: string;
}

// 格式化 Date 为 datetime-local 输入值，按用户有效时区（用户时区，否则浏览器）。
// Format a Date as a datetime-local value in the effective user timezone (user tz, else browser).
export function formatToDateTimeLocal(date: Date): string {
  return formatDateTimeLocalInput(date);
}

/**
 * 紧凑时间范围筛选：相对时段用分段控件，自定义日历用 Arc DateRangePicker。
 * Compact time filter: relative windows on a segmented control, custom days on Arc DateRangePicker.
 */
export function CompactTimeFilter({
  startTime,
  endTime,
  onStartTimeChange,
  onEndTimeChange,
  onClear,
  activePreset,
  onApplyPreset,
  presets,
  className,
}: CompactTimeFilterProps) {
  const t = useTranslations('monitoringCenter');
  const tRange = useTranslations('common.dateRange');

  const defaultPresets: TimePresetOption[] = presets || [
    {label: t('alerts.timePresets.last1h'), minutes: 60},
    {label: t('alerts.timePresets.last6h'), minutes: 360},
    {label: t('alerts.timePresets.last24h'), minutes: 1440},
    {label: t('alerts.timePresets.last7d'), minutes: 10080},
  ];

  const hasActiveTime = Boolean(startTime || endTime);
  const rangeValue = useMemo(
    () => wallTimesToDateRange(startTime, endTime),
    [startTime, endTime],
  );

  const handleRangeChange = (range: DateRange) => {
    const next = dateRangeToWallTimes(range);
    onStartTimeChange(next.startTime);
    onEndTimeChange(next.endTime);
  };

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-2 pt-1 border-t border-dashed border-border/50',
        className,
      )}
    >
      <SegmentedControl
        label={t('alerts.timeRange')}
        value={activePreset != null ? String(activePreset) : ''}
        onValueChange={(value) => onApplyPreset?.(Number(value))}
        options={defaultPresets.map((preset) => ({
          value: String(preset.minutes),
          label: preset.label,
        }))}
      />
      <ArcDateRangePicker
        value={rangeValue}
        onChange={handleRangeChange}
        label={tRange('label')}
        placeholder={tRange('placeholder')}
      />
      {hasActiveTime ? (
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={onClear}
          aria-label={t('alerts.clearTimeFilter')}
        >
          <X size={14} strokeWidth={1.75} aria-hidden='true' />
          {t('alerts.clearTimeFilter')}
        </Button>
      ) : null}
    </div>
  );
}
