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

import {Loader2} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {Button} from '@/components/ui/button';

/**
 * 工作台未落盘草稿离开确认：留下 / 保存并离开 / 丢弃并离开。
 * Workbench leave confirm when drafts are unsaved: Stay / Save&Leave / Discard&Leave.
 * 默认焦点落在「留下」，避免误点丢弃。
 * Default focus stays on Stay so Discard is not the accidental default.
 */
export function WorkbenchLeaveDialog({
  open,
  busy,
  onOpenChange,
  onStay,
  onSaveAndLeave,
  onDiscardAndLeave,
}: {
  open: boolean;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onStay: () => void;
  onSaveAndLeave: () => void;
  onDiscardAndLeave: () => void;
}) {
  const t = useTranslations('workbenchStudio');

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (busy) {
          return;
        }
        onOpenChange(next);
        if (!next) {
          onStay();
        }
      }}
    >
      <AlertDialogContent
        className='border border-border/80 shadow-2xl dark:border-border/60 sm:max-w-md'
        data-testid='studio-leave-unsaved-dialog'
        onOpenAutoFocus={(event) => {
          // 默认焦点给「留下」，不要落在「丢弃」。
          // Prefer Stay as the initial focus; never land on Discard.
          event.preventDefault();
          const stay = document.querySelector<HTMLElement>(
            '[data-testid="studio-leave-stay"]',
          );
          stay?.focus();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t('leaveUnsavedTitle')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('leaveUnsavedDescription')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter className='flex-col gap-2 sm:flex-col sm:space-x-0'>
          <AlertDialogCancel
            data-testid='studio-leave-stay'
            disabled={busy}
            onClick={onStay}
          >
            {t('leaveStay')}
          </AlertDialogCancel>
          <Button
            type='button'
            data-testid='studio-leave-save'
            className='gap-1.5'
            disabled={busy}
            onClick={onSaveAndLeave}
          >
            {busy ? (
              <>
                <Loader2 className='size-3.5 animate-spin' aria-hidden />
                <span>{t('leaveSavingAndLeaving')}</span>
              </>
            ) : (
              t('leaveSaveAndLeave')
            )}
          </Button>
          <Button
            type='button'
            variant='outline'
            data-testid='studio-leave-discard'
            className='border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive'
            disabled={busy}
            onClick={onDiscardAndLeave}
          >
            {t('leaveDiscardAndLeave')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
