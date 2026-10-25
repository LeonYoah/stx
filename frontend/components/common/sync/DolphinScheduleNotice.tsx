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

import {useTranslations} from 'next-intl';
import {AlertTriangle, CalendarClock, Info} from 'lucide-react';

import type {
  DolphinBuiltinSchedule,
  DolphinWorkflowDetail,
} from '@/lib/services/sync/types';

/**
 * 推送到已有工作流前的定时说明：
 * - 工作流在线时推送需先下线、写入后再上线（期间定时不触发，暂停的定时保持暂停）；下线的工作流保持下线；
 * - 定时属于整个工作流，STX 内置 cron 不会沿用，推送后关闭，改由工作流定时驱动；工作流没有定时则不会自动运行。
 * Schedule notes before pushing into an existing workflow:
 * - an online workflow is offlined, written and re-onlined (its schedules do not fire meanwhile and paused ones stay
 *   paused); an offline workflow stays offline;
 * - schedules belong to the whole workflow, so the STX cron is not carried over and is turned off; the workflow's own
 *   schedule drives the task, and a workflow without a schedule never runs automatically.
 */
export function DolphinScheduleNotice({
  workflow,
  builtinSchedule,
}: {
  workflow: DolphinWorkflowDetail;
  builtinSchedule?: DolphinBuiltinSchedule | null;
}) {
  const t = useTranslations('workbenchStudio');
  const online = (workflow.release_state ?? '').toUpperCase() === 'ONLINE';
  const schedules = workflow.schedules ?? [];
  const noSchedule = !workflow.schedule_error && schedules.length === 0;

  return (
    <div
      className='space-y-2.5 rounded-lg border border-border/50 bg-muted/10 p-3 text-xs'
      data-testid='dolphin-schedule-notice'
    >
      <div className='flex items-center gap-1.5 font-medium'>
        <CalendarClock className='size-3.5 shrink-0' />
        {t('dolphinScheduleNoticeTitle')}
      </div>

      <div className='flex items-start gap-2 rounded-md border border-sky-500/30 bg-sky-500/10 px-2.5 py-2 text-sky-800 dark:text-sky-300'>
        <Info className='mt-0.5 size-3.5 shrink-0' />
        <span>
          {online
            ? t('dolphinWorkflowOnlineCycle')
            : t('dolphinWorkflowOfflineKept')}
        </span>
      </div>

      <div className='space-y-1'>
        <div className='text-[11px] text-muted-foreground'>
          {t('dolphinWorkflowSchedules')}
        </div>
        {workflow.schedule_error ? (
          <p className='break-words text-amber-700 dark:text-amber-400'>
            {t('dolphinScheduleReadFailed', {error: workflow.schedule_error})}
          </p>
        ) : noSchedule ? (
          <p className='text-muted-foreground'>
            {t('dolphinNoWorkflowSchedule')}
          </p>
        ) : (
          <ul className='flex flex-wrap gap-1.5'>
            {schedules.map((item) => {
              const scheduleOnline =
                item.release_state.toUpperCase() === 'ONLINE';
              return (
                <li
                  key={item.id}
                  className='flex items-center gap-1.5 rounded border border-border/60 px-1.5 py-0.5'
                >
                  <code className='whitespace-nowrap font-mono text-[11px]'>
                    {item.crontab}
                  </code>
                  {item.timezone ? (
                    <span className='whitespace-nowrap text-[11px] text-muted-foreground'>
                      {item.timezone}
                    </span>
                  ) : null}
                  <span
                    className={
                      scheduleOnline
                        ? 'whitespace-nowrap text-[11px] text-green-700 dark:text-green-400'
                        : 'whitespace-nowrap text-[11px] text-muted-foreground'
                    }
                  >
                    {scheduleOnline
                      ? t('dolphinScheduleOnline')
                      : t('dolphinSchedulePaused')}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div
        className={
          noSchedule
            ? 'flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-2.5 py-2 text-amber-700 dark:text-amber-400'
            : 'flex items-start gap-2 text-muted-foreground'
        }
      >
        {noSchedule ? (
          <AlertTriangle className='mt-0.5 size-3.5 shrink-0' />
        ) : (
          <Info className='mt-0.5 size-3.5 shrink-0' />
        )}
        <span>
          {builtinSchedule?.cron_expr
            ? t('dolphinBuiltinCronNotCarried', {
                cron: builtinSchedule.cron_expr,
              })
            : t('dolphinWorkflowScheduleDrives')}
          {noSchedule ? ` ${t('dolphinNoScheduleNeverRuns')}` : ''}
        </span>
      </div>
    </div>
  );
}
