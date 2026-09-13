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

import React from 'react';
import {useTranslations} from 'next-intl';
import {Calendar, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {cn} from '@/lib/utils';

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

// 格式化 Date 对象为 datetime-local 所需的标准 YYYY-MM-DDTHH:mm 本地时间字符串
// Format Date object into standard YYYY-MM-DDTHH:mm string required by HTML5 datetime-local input
export function formatToDateTimeLocal(date: Date): string {
  const pad = (num: number) => String(num).padStart(2, '0');
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

// 统一的紧凑时间范围筛选组件（带快捷预设、明确起止胶囊、防数字截断、暗色模式适配）
// Unified compact time range filter component with quick presets, clear labels, no truncation, and dark mode support
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

  const defaultPresets: TimePresetOption[] = presets || [
    {label: t('alerts.timePresets.last1h'), minutes: 60},
    {label: t('alerts.timePresets.last6h'), minutes: 360},
    {label: t('alerts.timePresets.last24h'), minutes: 1440},
    {label: t('alerts.timePresets.last7d'), minutes: 10080},
  ];

  const hasActiveTime = Boolean(startTime || endTime);

  return (
    <div
      className={cn(
        'flex flex-wrap items-center justify-between gap-2 pt-1 border-t border-dashed border-border/50 text-xs',
        className,
      )}
    >
      {/* 快捷时间预设胶囊组 */}
      {/* Quick time preset buttons group */}
      <div className='flex items-center gap-1.5 flex-wrap'>
        <span className='inline-flex items-center gap-1 text-muted-foreground font-medium text-[11px] mr-0.5 select-none'>
          <Calendar className='h-3.5 w-3.5 text-primary/80' />
          {t('alerts.timeRange')}:
        </span>
        {defaultPresets.map((preset) => (
          <button
            key={preset.minutes}
            type='button'
            onClick={() => onApplyPreset?.(preset.minutes)}
            className={cn(
              'px-2 py-0.5 rounded text-[11px] transition-colors border select-none cursor-pointer',
              activePreset === preset.minutes
                ? 'bg-primary/10 text-primary border-primary/30 font-medium'
                : 'bg-background text-muted-foreground border-border/70 hover:bg-muted/60 hover:text-foreground',
            )}
          >
            {preset.label}
          </button>
        ))}
      </div>

      {/* 精确起止时间输入胶囊 */}
      {/* Precise Datetime Range Input Capsules */}
      <div className='flex items-center gap-1.5 flex-wrap'>
        <div className='inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md border border-input bg-background shadow-2xs'>
          <span className='text-[11px] text-muted-foreground font-medium select-none'>
            {t('alerts.startTime')}:
          </span>
          <input
            type='datetime-local'
            value={startTime}
            onChange={(e) => onStartTimeChange(e.target.value)}
            className='h-6 w-[170px] bg-transparent text-xs font-mono text-foreground focus:outline-hidden dark:[color-scheme:dark]'
          />
        </div>

        <span className='text-muted-foreground text-xs select-none'>~</span>

        <div className='inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md border border-input bg-background shadow-2xs'>
          <span className='text-[11px] text-muted-foreground font-medium select-none'>
            {t('alerts.endTime')}:
          </span>
          <input
            type='datetime-local'
            value={endTime}
            onChange={(e) => onEndTimeChange(e.target.value)}
            className='h-6 w-[170px] bg-transparent text-xs font-mono text-foreground focus:outline-hidden dark:[color-scheme:dark]'
          />
        </div>

        {hasActiveTime && (
          <Button
            variant='ghost'
            size='sm'
            onClick={onClear}
            className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground cursor-pointer'
            title={t('alerts.clearTimeFilter')}
          >
            <X className='mr-1 h-3 w-3' />
            {t('alerts.clearTimeFilter')}
          </Button>
        )}
      </div>
    </div>
  );
}
