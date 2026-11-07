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

import {Badge} from '@/components/arc/badge/badge';
import {Button} from '@/components/arc/button/button';
import {ConfirmMorph} from '@/components/arc/confirm-morph/confirm-morph';
import {Switch} from '@/components/arc/switch/switch';
import {HelpHint} from '@/components/common/HelpHint';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  CalendarClock,
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
  const [copyingScript, setCopyingScript] = useState(false);

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

  const scheduleSwitch = dolphin?.builtin_locked ? (
    // 已绑定 Dolphin：开关禁用，原因进 Tooltip；禁用控件外包 span 才能触发。
    // Bound to Dolphin: switch disabled with the reason in a Tooltip; wrap so it still fires.
    <Tooltip>
      <TooltipTrigger asChild>
        <span className='inline-flex' tabIndex={0}>
          <Switch
            checked={value.enabled}
            disabled
            aria-label={t('scheduleEnable')}
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
      aria-label={t('scheduleEnable')}
      onCheckedChange={(checked) =>
        onChange({
          ...value,
          enabled: checked,
          cron_expr: value.cron_expr,
          timezone: timezoneValue,
        })
      }
    />
  );

  return (
    <div className={cn('studio-sidebar-chrome min-w-0 space-y-4', className)}>
      {/* 标题、状态、开关一行排开，不再套卡片，长说明进 HelpHint。 */}
      {/* Title, status, and switch on one row. No nested card; long copy lives in HelpHint. */}
      <div className='space-y-2'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex min-w-0 items-center gap-1.5'>
            <h2 className='truncate text-xs font-medium text-foreground'>
              {t('taskSchedule')}
            </h2>
            <HelpHint
              side='bottom'
              label={t('taskSchedule')}
              content={t('scheduleLightweightBadgeHint')}
              data-testid='schedule-production-notice'
            />
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type='button'
                  className='inline-flex cursor-help rounded-[var(--radius-control)]'
                >
                  <Badge tone='warning' size='sm' outline>
                    {t('scheduleLightweightBadge')}
                  </Badge>
                </button>
              </TooltipTrigger>
              <TooltipContent className='max-w-[320px] text-xs leading-5'>
                {t('scheduleLightweightBadgeHint')}
              </TooltipContent>
            </Tooltip>
            <Badge
              tone={value.enabled ? 'success' : 'neutral'}
              size='sm'
              dot
              pill
            >
              {value.enabled ? t('activeSchedule') : t('suspendedSchedule')}
            </Badge>
          </div>
          <div className='flex shrink-0 items-center gap-1.5'>
            {onOpenAdvanced ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    aria-label={t('openAdvancedSchedule')}
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
            {scheduleSwitch}
          </div>
        </div>
        {hasUnsavedChanges ? (
          <p className='break-words text-[11px] leading-4 text-amber-600 dark:text-amber-400'>
            {t('scheduleUnsavedHint')}
          </p>
        ) : null}
        {/* 上次/下次触发：两列纯文本，不再套一层格子卡片。 */}
        {/* Last/next trigger as two plain columns, not a nested metric card. */}
        <div className='grid grid-cols-2 gap-3'>
          <div className='min-w-0 space-y-1'>
            <div className='flex items-center gap-1 text-[11px] font-medium text-muted-foreground'>
              <History className='size-3 shrink-0' />
              <span className='truncate'>{t('scheduleLastTriggeredAt')}</span>
            </div>
            <div className='truncate font-mono text-xs tabular-nums text-foreground'>
              {formatScheduleTime(lastTriggeredAt)}
            </div>
          </div>
          <div className='min-w-0 space-y-1'>
            <div className='flex items-center gap-1 text-[11px] font-medium text-muted-foreground'>
              <CalendarClock className='size-3 shrink-0' />
              <span className='truncate'>{t('scheduleNextTriggeredAt')}</span>
            </div>
            <div className='truncate font-mono text-xs tabular-nums text-foreground'>
              {value.enabled
                ? formatScheduleTime(nextTriggeredAt)
                : t('scheduleDisabled')}
            </div>
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
            cronPresets: t('cronPresets'),
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
                <button type='button' className='inline-flex cursor-help'>
                  <Badge tone='neutral' size='sm' outline>
                    {t('savedVersionOnly')}
                  </Badge>
                </button>
              </TooltipTrigger>
              <TooltipContent className='max-w-[320px] text-xs leading-5'>
                {t('scheduleSavedVersionHint')}
              </TooltipContent>
            </Tooltip>
          }
        />

      {taskId ? (
        <section className='space-y-2.5 border-t border-border/60 pt-3'>
          {/* 外部调度是同一页的下一段，不再套第二张卡。 */}
          {/* External schedule is the next section of this page, not a second card. */}
          <div className='flex items-center justify-between gap-2'>
            <h3 className='text-xs font-medium text-foreground'>
              {t('dolphinExternalSchedule')}
            </h3>
            {dolphin?.binding ? (
              <Badge tone='info' size='sm' outline>
                {t('dolphinBound')}
              </Badge>
            ) : null}
          </div>
          {dolphin?.binding ? (
            <div className='space-y-1.5 text-[11px] text-muted-foreground'>
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
          <div className='flex flex-wrap items-center gap-1.5'>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              loading={copyingScript}
              onClick={async () => {
                setCopyingScript(true);
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
                } finally {
                  setCopyingScript(false);
                }
              }}
            >
              <Copy className='size-3.5' />
              {t('dolphinCopyScript')}
            </Button>
            {canEdit ? (
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={() => setPushOpen(true)}
              >
                <Upload className='size-3.5' />
                {dolphin?.binding ? t('dolphinUpdateNode') : t('dolphinPushTo')}
              </Button>
            ) : null}
            {dolphin?.binding?.console_url ? (
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() =>
                  window.open(dolphin.binding?.console_url, '_blank')
                }
              >
                <ExternalLink className='size-3.5' />
                {t('dolphinOpen')}
              </Button>
            ) : null}
          </div>
          {canEdit && dolphin?.binding ? (
            // ConfirmMorph 展开后比侧栏按钮宽，单独占一行以免挤开复制/更新。
            // ConfirmMorph grows wider than a sidebar button; keep it on its own row.
            <ConfirmMorph
              tone='danger'
              confirmMode='press'
              icon={<Unplug className='size-3.5' />}
              label={t('dolphinUnbind')}
              prompt={t('dolphinUnbindPrompt')}
              confirmLabel={t('dolphinUnbind')}
              cancelLabel={t('cancel')}
              pendingLabel={t('dolphinUnbindPending')}
              doneLabel={t('dolphinUnbound')}
              errorLabel={t('dolphinPushFailed')}
              onConfirm={async () => {
                await SyncService.unbindDolphin(taskId);
                await reloadDolphin();
              }}
            />
          ) : null}
        </section>
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
