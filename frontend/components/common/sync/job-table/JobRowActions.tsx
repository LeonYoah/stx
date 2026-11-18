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

import Link from 'next/link';
import {MoreHorizontal, PanelRightOpen} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import type {SyncJobInstance} from '@/lib/services/sync';
import {copyText} from '../job-detail/copy-text';
import {getJobActionState} from '../sync-studio-utils';
import {jobWorkbenchHref} from './job-table-format';
import type {
  JobActorContext,
  JobRowAction,
  JobTableTranslator,
} from './types';

const ACTION_BUTTON_CLASS =
  'h-7 gap-1 px-2 text-xs text-muted-foreground hover:bg-muted/70 hover:text-foreground';

/**
 * 行内操作（两预设一致）：详情 + 最多一个主操作（活动作业「停止」、可恢复「恢复」）+ 更多菜单
 * （Savepoint 停止 / 在工作台打开 / 复制作业 ID）。可见性与禁用全部由 getJobActionState 判定。
 * Row actions shared by both presets: Details, at most one primary action (Stop on active jobs, Recover on
 * recoverable ones) and a more menu (Savepoint stop / Open in workbench / Copy job ID). Visibility and the
 * disabled state all come from getJobActionState.
 */
export function JobRowActions({
  job,
  actor,
  showWorkbenchLink,
  t,
  onOpenDetail,
  onAction,
}: {
  job: SyncJobInstance;
  actor: JobActorContext;
  /** 「在工作台打开」菜单项仅 global 预设展示 / "Open in workbench" shows on the global preset only. */
  showWorkbenchLink: boolean;
  t: JobTableTranslator;
  onOpenDetail: (job: SyncJobInstance) => void;
  onAction: (action: JobRowAction, job: SyncJobInstance) => void;
}) {
  const actions = getJobActionState(job, actor);
  return (
    // 操作区点击不冒泡成行激活。/ Clicks in the action cluster must not bubble up to row activation.
    <div
      className='flex items-center justify-end gap-0.5 whitespace-nowrap'
      onClick={(event) => event.stopPropagation()}
    >
      <Button
        size='sm'
        variant='ghost'
        className={ACTION_BUTTON_CLASS}
        aria-label={t('detail')}
        onClick={() => onOpenDetail(job)}
      >
        <PanelRightOpen className='size-3.5' />
        {t('detail')}
      </Button>
      {actions.showStop ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button
                size='sm'
                variant='ghost'
                className={ACTION_BUTTON_CLASS}
                disabled={!actions.canStop}
                onClick={() => onAction('stop', job)}
              >
                {t('stop')}
              </Button>
            </span>
          </TooltipTrigger>
          {!actions.canStop ? (
            <TooltipContent>{t('readOnlyCancelTooltip')}</TooltipContent>
          ) : null}
        </Tooltip>
      ) : actions.showRecover ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button
                size='sm'
                variant='ghost'
                className={ACTION_BUTTON_CLASS}
                disabled={!actions.canRecover}
                onClick={() => onAction('recover', job)}
              >
                {t('recover')}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {!actor.canRun
              ? t('readOnlyRecoverTooltip')
              : t('historyRecoverTooltip')}
          </TooltipContent>
        </Tooltip>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size='sm'
            variant='ghost'
            className='h-7 w-7 px-0 text-muted-foreground hover:bg-muted/70 hover:text-foreground'
            aria-label={t('moreActions')}
          >
            <MoreHorizontal className='size-3.5' />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          {actions.showStop ? (
            <DropdownMenuItem
              disabled={!actions.canStop}
              onSelect={() => onAction('savepointStop', job)}
            >
              {t('savepointStop')}
            </DropdownMenuItem>
          ) : null}
          {showWorkbenchLink ? (
            <DropdownMenuItem asChild>
              <Link href={jobWorkbenchHref(job)}>{t('openInWorkbench')}</Link>
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuItem
            onSelect={() => copyText(String(job.id), t('copiedJobId'))}
          >
            {t('copyJobId')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
