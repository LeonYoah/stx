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
 * 通用 Arc 二次确认弹窗：基于 Arc Dialog（role='alertdialog' + .arc-compact）与 Arc Button，
 * 禁止遮罩误点关闭；高危操作（confirmVariant='danger'）默认将焦点落在安全取消按钮上。
 * Shared Arc confirmation dialog built on Arc Dialog (role='alertdialog' + .arc-compact) and Arc Button.
 * Blocks backdrop-click dismissal and places initial focus on Cancel for destructive confirmations.
 */
import {useRef, type ReactNode} from 'react';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Button} from '@/components/arc/button/button';
import {cn} from '@/lib/utils';

export interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  /** 附加内容区（例如强制删除勾选框、影响项清单）。Optional extra body content (e.g. force-delete checkbox or impact list). */
  children?: ReactNode;
  cancelLabel: string;
  confirmLabel: string;
  confirmVariant?: 'primary' | 'danger';
  loading?: boolean;
  disabled?: boolean;
  onConfirm: () => void | Promise<void>;
  dialogTestId?: string;
  cancelTestId?: string;
  confirmTestId?: string;
  className?: string;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  cancelLabel,
  confirmLabel,
  confirmVariant = 'primary',
  loading = false,
  disabled = false,
  onConfirm,
  dialogTestId,
  cancelTestId,
  confirmTestId,
  className,
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (loading) {
          return;
        }
        onOpenChange(next);
      }}
    >
      <DialogContent
        role='alertdialog'
        title={title}
        description={description}
        showCloseButton={false}
        className={cn(
          'arc-compact w-[min(calc(100vw-2rem),460px)]!',
          className,
        )}
        data-testid={dialogTestId}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (loading) {
            event.preventDefault();
          }
        }}
        onOpenAutoFocus={(event) => {
          if (confirmVariant === 'danger') {
            // 高危动作默认聚焦取消按钮，防止误触回车直接执行破坏性操作。
            // Focus Cancel first for destructive actions so Enter never fires them accidentally.
            event.preventDefault();
            cancelRef.current?.focus();
          }
        }}
      >
        <div className='space-y-4'>
          {children ? <div className='space-y-3'>{children}</div> : null}
          <div className='flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-end'>
            <Button
              ref={cancelRef}
              type='button'
              variant='secondary'
              size='sm'
              disabled={loading}
              data-testid={cancelTestId}
              onClick={() => onOpenChange(false)}
            >
              {cancelLabel}
            </Button>
            <Button
              type='button'
              variant={confirmVariant}
              size='sm'
              loading={loading}
              disabled={loading || disabled}
              data-testid={confirmTestId}
              onClick={() => void onConfirm()}
            >
              {confirmLabel}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
