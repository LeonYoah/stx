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

import {useEffect, useMemo, useState} from 'react';

import {cn} from '@/lib/utils';
import {useTranslations} from 'next-intl';

import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  CalendarClock,
  Clock3,
  Copy,
  ExternalLink,
  History,
  Maximize2,
  Unplug,
  Upload,
} from 'lucide-react';
import {toast} from 'sonner';

import {DolphinPushDialog} from '@/components/common/sync/DolphinPushDialog';
import {SyncService} from '@/lib/services/sync/sync.service';
import type {DolphinTaskStatus} from '@/lib/services/sync/types';

import {
  CronExpressionEditor,
  type CronPresetOption,
} from '@/components/common/schedule/CronExpressionEditor';
import {
  dolphinPushBlockedReason,
  getRunModeLabel,
} from '@/components/common/sync/sync-studio-utils';

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
  taskId,
  canEdit = true,
  hasUnsavedChanges = false,
}: {
  value: TaskScheduleValue;
  lastTriggeredAt?: string;
  nextTriggeredAt?: string;
  onChange: (value: TaskScheduleValue) => void;
  onOpenAdvanced?: () => void;
  className?: string;
  taskId?: number;
  canEdit?: boolean;
  /** 定时配置相对已保存版本有改动时为 true / True when the schedule differs from the saved version */
  hasUnsavedChanges?: boolean;
}) {
  const t = useTranslations('workbenchStudio');
  const [dolphin, setDolphin] = useState<DolphinTaskStatus | null>(null);
  const [pushOpen, setPushOpen] = useState(false);

  const reloadDolphin = async () => {
    if (!taskId) {
      setDolphin(null);
      return;
    }
    try {
      setDolphin(await SyncService.getDolphinTaskStatus(taskId));
    } catch {
      setDolphin(null);
    }
  };

  useEffect(() => {
    void reloadDolphin();
  }, [taskId]);
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
  const pushBlocked = dolphin == null ? null : dolphinPushBlockedReason(dolphin);

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
                {/* 生产调度建议收进「轻量」Badge 的 Tooltip；Badge 包成 button 以便键盘聚焦打开，可访问名称沿用可见文字，长建议由 Tooltip 作为描述 */}
                {/* Production advice lives in the "Lightweight" badge Tooltip; wrapped in a button for keyboard focus, accessible name stays the visible text and the Tooltip supplies the description */}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      className='inline-flex cursor-help rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50'
                    >
                      <Badge
                        variant='outline'
                        className='h-4.5 border-amber-500/30 px-1.5 text-[10px] text-amber-600 dark:text-amber-400'
                      >
                        {t('scheduleLightweightBadge')}
                      </Badge>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className='max-w-[320px] text-xs leading-5'>
                    {t('scheduleProductionNoticeDesc')}
                  </TooltipContent>
                </Tooltip>
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
            {dolphin?.builtin_locked ? (
              // 已绑定 Dolphin：开关禁用，原因进 Tooltip；禁用控件外包 span 才能触发
              // Bound to Dolphin: switch disabled with the reason in a Tooltip; wrapped in a span so it fires
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className='inline-flex' tabIndex={0}>
                    <Switch
                      checked={value.enabled}
                      disabled
                      className='pointer-events-none'
                    />
                  </span>
                </TooltipTrigger>
                <TooltipContent side='left' className='max-w-[260px] text-xs'>
                  {t('dolphinBuiltinLocked')}
                </TooltipContent>
              </Tooltip>
            ) : (
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
            )}
          </div>
        </div>

        {hasUnsavedChanges ? (
          <p className='text-[11px] leading-4 text-amber-600 dark:text-amber-400 break-words'>
            {t('scheduleUnsavedHint')}
          </p>
        ) : null}

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

      {taskId ? (
        <div className='rounded-lg border border-border/50 bg-muted/10 p-3.5 space-y-3'>
          <div className='flex items-center justify-between gap-2'>
            <div className='text-xs font-semibold'>{t('dolphinExternalSchedule')}</div>
            {dolphin?.binding ? (
              <Badge variant='outline' className='h-4 px-1.5 text-[10px]'>
                {t('dolphinBound')}
              </Badge>
            ) : null}
          </div>
          {dolphin?.binding ? (
            <div className='space-y-2 text-[11px] text-muted-foreground'>
              <div className='overflow-x-auto whitespace-nowrap font-mono'>
                {dolphin.binding.workflow_name || dolphin.binding.workflow_code} / {dolphin.binding.task_code}
              </div>
              <div className='overflow-x-auto whitespace-nowrap'>
                {t('dolphinPublishedPushed', {
                  published: dolphin.published_version || 0,
                  pushed: dolphin.pushed_version || 0,
                })}
              </div>
              {(dolphin.recent_jobs ?? []).slice(0, 3).map((job) => (
                <div
                  key={job.id}
                  className='flex items-center justify-between gap-2 overflow-x-auto whitespace-nowrap'
                  title={job.external_ref || undefined}
                >
                  <span className='font-mono'>
                    #{job.id}
                    {job.task_version > 0 ? ` v${job.task_version}` : ''}
                    {job.external_ref ? ` ${job.external_ref}` : ''}
                  </span>
                  <span>
                    {getRunModeLabel(job, t)} · {job.status}
                  </span>
                </div>
              ))}
            </div>
          ) : null}
          {pushBlocked === 'unpublished' ? (
            <p className='text-[11px] text-amber-600 dark:text-amber-400'>
              {t('dolphinNeedPublish')}
            </p>
          ) : null}
          {pushBlocked === 'no_new_version' ? (
            <p className='text-[11px] text-amber-600 dark:text-amber-400'>
              {t('dolphinNeedNewVersion', {
                pushed: dolphin?.pushed_version || 0,
              })}
            </p>
          ) : null}
          {/* 推送只写 SHELL 节点；Worker stx_ 登录令牌由用户自行绑定，侧栏不开放令牌管理。 */}
          {/* Push only writes the SHELL node; the worker stx_ login token is bound by the user, not from this sidebar. */}
          <div className='flex flex-wrap gap-1.5'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              className='h-7 px-2 text-[11px]'
              onClick={async () => {
                try {
                  const script = await SyncService.getDolphinScript(taskId);
                  await navigator.clipboard.writeText(
                    `${script.script}\n${script.env_notes}`,
                  );
                  toast.success(t('dolphinScriptCopied'));
                } catch (error) {
                  toast.error(
                    error instanceof Error ? error.message : t('dolphinPushFailed'),
                  );
                }
              }}
            >
              <Copy className='size-3' />
              {t('dolphinCopyScript')}
            </Button>
            {canEdit ? (
              <Button
                type='button'
                size='sm'
                className='h-7 px-2 text-[11px]'
                onClick={() => setPushOpen(true)}
              >
                <Upload className='size-3' />
                {dolphin?.binding ? t('dolphinUpdateNode') : t('dolphinPushTo')}
              </Button>
            ) : null}
            {dolphin?.binding?.console_url ? (
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-7 px-2 text-[11px]'
                onClick={() => window.open(dolphin.binding?.console_url, '_blank')}
              >
                <ExternalLink className='size-3' />
                {t('dolphinOpen')}
              </Button>
            ) : null}
            {canEdit && dolphin?.binding ? (
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-7 px-2 text-[11px] text-destructive'
                onClick={async () => {
                  try {
                    await SyncService.unbindDolphin(taskId);
                    toast.success(t('dolphinUnbound'));
                    await reloadDolphin();
                  } catch (error) {
                    toast.error(
                      error instanceof Error ? error.message : t('dolphinPushFailed'),
                    );
                  }
                }}
              >
                <Unplug className='size-3' />
                {t('dolphinUnbind')}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}

      {taskId ? (
        <DolphinPushDialog
          open={pushOpen}
          onOpenChange={setPushOpen}
          taskId={taskId}
          status={dolphin}
          onPushed={() => void reloadDolphin()}
        />
      ) : null}
    </div>
  );
}
