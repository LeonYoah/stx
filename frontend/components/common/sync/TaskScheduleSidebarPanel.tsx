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

import {cn} from '@/lib/utils';
import {useTranslations} from 'next-intl';

import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {CalendarClock, Clock3, History, Info, Maximize2} from 'lucide-react';

import {
  CronExpressionEditor,
  type CronPresetOption,
} from '@/components/common/schedule/CronExpressionEditor';

export interface TaskScheduleValue {
  enabled: boolean;
  cron_expr: string;
  timezone: string;
}

export function TaskScheduleSidebarPanel({
  value,
  lastTriggeredAt,
  nextTriggeredAt,
  onChange,
  onOpenAdvanced,
  className,
}: {
  value: TaskScheduleValue;
  lastTriggeredAt?: string;
  nextTriggeredAt?: string;
  onChange: (value: TaskScheduleValue) => void;
  onOpenAdvanced?: () => void;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio');
  const presets = useMemo<CronPresetOption[]>(
    () => [
      {key: 'cronPresetEveryDayMidnight', expr: '0 0 * * *'},
      {key: 'cronPresetEveryHour', expr: '0 * * * *'},
      {key: 'cronPresetWeekdaysNine', expr: '0 9 * * MON-FRI'},
      {key: 'cronPresetEveryFifteenMinutes', expr: '*/15 * * * *'},
      {key: 'cronPresetEveryThirtyMinutes', expr: '*/30 * * * *'},
      {key: 'cronPresetWeekdaysEightThirty', expr: '30 8 * * MON-FRI'},
      {key: 'cronPresetWeeklySundayMidnight', expr: '0 0 * * SUN'},
      {key: 'cronPresetMonthlyFirstNine', expr: '0 9 1 * *'},
      {key: 'cronPresetQuarterlyFirst', expr: '0 0 1 */3 *'},
    ],
    [],
  );

  const timezoneValue = value.timezone || 'Asia/Shanghai';

  // 格式化时间展示
  // Format schedule datetime display
  const formatScheduleTime = (timeStr?: string) => {
    if (!timeStr) {
      return t('scheduleNoTriggerYet');
    }
    const parsed = new Date(timeStr);
    if (Number.isNaN(parsed.getTime())) {
      return timeStr;
    }
    return new Intl.DateTimeFormat(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(parsed);
  };

  return (
    <div className={cn('min-w-0 space-y-3.5', className)}>
      {/* 生产调度工具建议提示卡片 */}
      {/* Production scheduling recommendation callout card */}
      <div className='rounded-lg border border-amber-500/25 bg-amber-500/5 p-3 text-xs'>
        <div className='flex items-center gap-1.5 font-medium text-amber-600 dark:text-amber-400'>
          <Info className='size-3.5 shrink-0 text-amber-500' />
          <span>{t('scheduleProductionNoticeTitle')}</span>
          <Badge
            variant='outline'
            className='ml-auto h-4 border-amber-500/30 px-1 text-[10px] text-amber-600 dark:text-amber-400'
          >
            DolphinScheduler / Airflow
          </Badge>
        </div>
        <p className='mt-1 text-[11px] leading-relaxed text-muted-foreground'>
          {t('scheduleProductionNoticeDesc')}
        </p>
      </div>

      {/* 调度开关与状态主卡片 */}
      {/* Scheduling toggle and status main card */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3.5 space-y-3'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-2'>
            <div className='flex size-6 shrink-0 items-center justify-center rounded-md border border-border/50 bg-background/80 text-primary'>
              <Clock3 className='size-3.5' />
            </div>
            <div>
              <div className='flex items-center gap-1.5'>
                <Label className='text-xs font-semibold text-foreground'>
                  {t('taskSchedule')}
                </Label>
                {value.enabled ? (
                  <Badge
                    variant='outline'
                    className='h-4.5 gap-1 border-emerald-500/40 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-600 dark:text-emerald-400'
                  >
                    <span className='size-1.5 rounded-full bg-emerald-500 animate-pulse' />
                    {t('activeSchedule')}
                  </Badge>
                ) : (
                  <Badge
                    variant='outline'
                    className='h-4.5 border-border/50 px-1.5 text-[10px] text-muted-foreground'
                  >
                    {t('suspendedSchedule')}
                  </Badge>
                )}
              </div>
            </div>
          </div>

          <div className='flex items-center gap-1.5'>
            {onOpenAdvanced ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type='button'
                    size='icon'
                    variant='ghost'
                    className='size-7 text-muted-foreground hover:text-foreground'
                    onClick={onOpenAdvanced}
                  >
                    <Maximize2 className='size-3.5' />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side='left'>
                  {t('openAdvancedSchedule')}
                </TooltipContent>
              </Tooltip>
            ) : null}
            <Switch
              checked={value.enabled}
              onCheckedChange={(checked) =>
                onChange({
                  ...value,
                  enabled: checked,
                  cron_expr: value.cron_expr,
                  timezone: timezoneValue,
                })
              }
            />
          </div>
        </div>

        <p className='text-[11px] leading-relaxed text-muted-foreground'>
          {t('taskScheduleHint')}
        </p>

        {/* 最近触发与下一次触发时间统计指标 */}
        {/* Last triggered and next trigger metrics */}
        <div className='grid gap-2.5 rounded-md border border-border/50 bg-background/70 p-2.5 sm:grid-cols-2'>
          <div className='space-y-1 min-w-0'>
            <div className='flex items-center gap-1 text-[10px] uppercase tracking-wider text-muted-foreground font-medium'>
              <History className='size-3 shrink-0' />
              <span>{t('scheduleLastTriggeredAt')}</span>
            </div>
            <div className='font-mono text-xs font-medium text-foreground truncate'>
              {formatScheduleTime(lastTriggeredAt)}
            </div>
          </div>

          <div className='space-y-1 min-w-0'>
            <div className='flex items-center gap-1 text-[10px] uppercase tracking-wider text-muted-foreground font-medium'>
              <CalendarClock className='size-3 shrink-0' />
              <span>{t('scheduleNextTriggeredAt')}</span>
            </div>
            <div className='font-mono text-xs font-medium text-foreground truncate'>
              {value.enabled
                ? formatScheduleTime(nextTriggeredAt)
                : t('scheduleDisabled')}
            </div>
          </div>
        </div>

        {/* Cron 表达式编辑器 */}
        {/* Cron expression editor */}
        <CronExpressionEditor
          expression={value.cron_expr}
          onExpressionChange={(cron_expr) =>
            onChange({...value, cron_expr, timezone: timezoneValue})
          }
          timezone={timezoneValue}
          onTimezoneChange={(timezone) =>
            onChange({...value, timezone, cron_expr: value.cron_expr})
          }
          translator={(key, values) => t(key, values as never)}
          labels={{
            cronExpression: t('cronExpression'),
            timezone: t('timezone'),
            cronFiveField: t('cronFiveField'),
            cronTimezonePlaceholder: t('cronTimezonePlaceholder'),
            cronExpressionPlaceholder: t('cronExpressionPlaceholder'),
            cronMinute: t('cronMinute'),
            cronHour: t('cronHour'),
            cronDayOfMonth: t('cronDayOfMonth'),
            cronMonth: t('cronMonth'),
            cronDayOfWeek: t('cronDayOfWeek'),
            schedulePreview: t('schedulePreview'),
            nextRuns: t('nextRuns'),
            invalidCronExpression: t('invalidCronExpression'),
          }}
          presets={presets}
          renderPresetLabel={(key) => t(key)}
          footer={
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge
                  variant='outline'
                  className='cursor-help rounded-sm text-[10px]'
                >
                  {t('savedVersionOnly')}
                </Badge>
              </TooltipTrigger>
              <TooltipContent className='max-w-[320px] text-xs leading-5'>
                {t('scheduleSavedVersionHint')}
              </TooltipContent>
            </Tooltip>
          }
        />
      </div>
    </div>
  );
}
