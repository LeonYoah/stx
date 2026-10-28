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
import {Badge} from '@/components/ui/badge';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import {formatDateTimeTz} from '@/lib/timezone';
import type {SyncJobInstance} from '@/lib/services/sync';
import {
  getCancelTimeoutMinutes,
  getSubmitOutcomeDeadline,
  getSubmitTimeoutMinutes,
  isCancelOverdue,
  isSubmitOutcomeExpired,
  isSubmitOutcomeUnknown,
} from './sync-studio-utils';

/**
 * 作业状态旁的结果提示：提交确认中、提交超时、停止超时。没有需要提示的情况时不渲染。
 * Outcome hints next to a job status: confirming submit, submit timed out, stop overdue. Renders nothing otherwise.
 */
export function JobOutcomeHints({
  job,
  className,
}: {
  job: SyncJobInstance | null;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio');
  if (!job) {
    return null;
  }
  const hints: Array<{key: string; label: string; tooltip: string; tone: 'warn' | 'error'}> = [];
  if (isSubmitOutcomeUnknown(job)) {
    const deadline = getSubmitOutcomeDeadline(job);
    hints.push({
      key: 'submit-confirming',
      label: t('submitConfirmingBadge'),
      tooltip: t('submitConfirmingTooltip', {deadline: deadline ? formatDateTimeTz(deadline) : '-'}),
      tone: 'warn',
    });
  } else if (isSubmitOutcomeExpired(job)) {
    hints.push({
      key: 'submit-expired',
      label: t('submitExpiredBadge'),
      tooltip: t('submitExpiredTooltip', {minutes: getSubmitTimeoutMinutes(job)}),
      tone: 'error',
    });
  }
  if (isCancelOverdue(job)) {
    hints.push({
      key: 'stop-overdue',
      label: t('stopOverdueBadge'),
      tooltip: t('stopOverdueTooltip', {minutes: getCancelTimeoutMinutes(job)}),
      tone: 'warn',
    });
  }
  if (hints.length === 0) {
    return null;
  }
  return (
    <span className={cn('inline-flex flex-wrap items-center gap-1', className)}>
      {hints.map((hint) => (
        <Tooltip key={hint.key}>
          <TooltipTrigger asChild>
            <Badge
              variant='outline'
              data-testid={`job-outcome-${hint.key}`}
              className={cn(
                'cursor-help whitespace-nowrap rounded-sm px-1.5 py-0 text-[10px] leading-tight',
                hint.tone === 'error'
                  ? 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400'
                  : 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400',
              )}
            >
              {hint.label}
            </Badge>
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-[280px] text-xs'>
            {hint.tooltip}
          </TooltipContent>
        </Tooltip>
      ))}
    </span>
  );
}
