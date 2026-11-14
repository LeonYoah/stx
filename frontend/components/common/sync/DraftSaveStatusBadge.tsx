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

import {AlertCircle, Check, Loader2} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {HELP_HINT_DELAY_MS} from '@/components/common/HelpHint';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import type {DraftSaveStatus} from './sync-studio-utils';

/**
 * 顶栏草稿保存状态指示（手动保存与离开前落盘共用）。
 * Top-bar draft save status (shared by manual save and leave flush).
 */
export function DraftSaveStatusBadge({
  status,
  onRetry,
  className,
}: {
  status: DraftSaveStatus;
  onRetry?: () => void;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio');

  if (status === 'idle') {
    return null;
  }

  if (status === 'saving') {
    return (
      <span
        data-testid='studio-draft-save-status'
        data-status='saving'
        aria-live='polite'
        className={cn(
          'inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-border/60 bg-muted/30 px-2 text-[11px] text-muted-foreground',
          className,
        )}
      >
        <Loader2 className='size-3 animate-spin' aria-hidden />
        <span className='whitespace-nowrap'>{t('draftSaving')}</span>
      </span>
    );
  }

  if (status === 'saved') {
    return (
      <span
        data-testid='studio-draft-save-status'
        data-status='saved'
        aria-live='polite'
        className={cn(
          'inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-2 text-[11px] text-emerald-700 dark:text-emerald-300',
          className,
        )}
      >
        <Check className='size-3' aria-hidden />
        <span className='whitespace-nowrap'>{t('draftSaved')}</span>
      </span>
    );
  }

  return (
    <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
      <TooltipTrigger asChild>
        <button
          type='button'
          data-testid='studio-draft-save-status'
          data-status='error'
          aria-live='polite'
          aria-label={t('draftSaveFailedRetry')}
          className={cn(
            'inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-2 text-[11px] text-destructive transition-colors hover:bg-destructive/15',
            className,
          )}
          onClick={onRetry}
        >
          <AlertCircle className='size-3' aria-hidden />
          <span className='whitespace-nowrap'>{t('draftSaveFailed')}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent>{t('draftSaveFailedRetry')}</TooltipContent>
    </Tooltip>
  );
}
