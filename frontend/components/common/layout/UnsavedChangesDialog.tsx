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

/**
 * Shared unsaved-changes confirmation dialog built on Arc Dialog + Arc Button:
 * supports two actions (keep editing / discard) or three actions (discard /
 * stay / save & continue), with backdrop-click guard and initial focus on the
 * safe keep button.
 * 跨模块共用的未保存修改确认弹窗（基于 Arc Dialog + Arc Button）：
 * 支持两按钮（继续编辑 / 放弃修改）与三按钮（丢弃 / 留下 / 保存并离开），
 * 禁用遮罩误点关闭，默认焦点始终落在安全留守按钮上。
 */
import {useEffect, useRef} from 'react';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Button} from '@/components/arc/button/button';
import {cn} from '@/lib/utils';

export interface UnsavedChangesDialogProps {
  open: boolean;
  title: string;
  description: string;
  keepLabel: string;
  discardLabel: string;
  onKeepEditing: () => void;
  onDiscard: () => void;
  /** 可选第三动作（如工作台「保存并离开」）。Optional third action (e.g. Workbench "Save and leave"). */
  saveLabel?: string;
  savingLabel?: string;
  busy?: boolean;
  onSave?: () => void;
  onOpenChange?: (open: boolean) => void;
  dialogTestId?: string;
  keepTestId?: string;
  discardTestId?: string;
  saveTestId?: string;
}

export function UnsavedChangesDialog({
  open,
  title,
  description,
  keepLabel,
  discardLabel,
  onKeepEditing,
  onDiscard,
  saveLabel,
  savingLabel,
  busy = false,
  onSave,
  onOpenChange,
  dialogTestId = 'discard-confirm-dialog',
  keepTestId = 'discard-keep-editing',
  discardTestId = 'discard-confirm-action',
  saveTestId = 'discard-save-action',
}: UnsavedChangesDialogProps) {
  const keepRef = useRef<HTMLButtonElement>(null);
  const actionTakenRef = useRef(false);
  const hasSaveAction = Boolean(saveLabel && onSave);

  useEffect(() => {
    if (open) {
      actionTakenRef.current = false;
    }
  }, [open]);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (busy) {
          return;
        }
        onOpenChange?.(next);
        if (!next && !actionTakenRef.current) {
          onKeepEditing();
        }
      }}
    >
      <DialogContent
        role='alertdialog'
        title={title}
        description={description}
        showCloseButton={false}
        className={cn(
          'arc-compact',
          hasSaveAction
            ? 'w-[min(calc(100vw-2rem),480px)]!'
            : 'w-[min(calc(100vw-2rem),420px)]!',
        )}
        data-testid={dialogTestId}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (busy) {
            event.preventDefault();
          }
        }}
        onOpenAutoFocus={(event) => {
          // 默认焦点留在安全留守按钮，避免回车误触发「放弃修改」。
          // Keep initial focus on the safe stay button so Enter never discards accidentally.
          event.preventDefault();
          keepRef.current?.focus();
        }}
      >
        <div className='flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-end'>
          {hasSaveAction ? (
            <>
              <Button
                type='button'
                variant='danger'
                size='sm'
                data-testid={discardTestId}
                className='sm:mr-auto'
                disabled={busy}
                onClick={() => {
                  actionTakenRef.current = true;
                  onDiscard();
                }}
              >
                {discardLabel}
              </Button>
              <Button
                ref={keepRef}
                type='button'
                variant='secondary'
                size='sm'
                data-testid={keepTestId}
                disabled={busy}
                onClick={() => {
                  actionTakenRef.current = true;
                  onKeepEditing();
                }}
              >
                {keepLabel}
              </Button>
              <Button
                type='button'
                variant='primary'
                size='sm'
                data-testid={saveTestId}
                loading={busy}
                disabled={busy}
                onClick={() => {
                  actionTakenRef.current = true;
                  onSave?.();
                }}
              >
                {busy ? savingLabel || saveLabel : saveLabel}
              </Button>
            </>
          ) : (
            <>
              <Button
                ref={keepRef}
                type='button'
                variant='secondary'
                size='sm'
                data-testid={keepTestId}
                onClick={() => {
                  actionTakenRef.current = true;
                  onKeepEditing();
                }}
              >
                {keepLabel}
              </Button>
              <Button
                type='button'
                variant='danger'
                size='sm'
                data-testid={discardTestId}
                onClick={() => {
                  actionTakenRef.current = true;
                  onDiscard();
                }}
              >
                {discardLabel}
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
