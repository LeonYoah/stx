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
 * 监控详情共用的未保存修改确认弹窗与浏览器离开拦截：
 * 在规则、作业规则或通知渠道编辑存在未保存修改时，拦截关闭或切行并弹出居中的
 * AlertDialog，默认焦点落在「继续编辑」，避免误丢修改。
 * Shared dirty-close guard for monitoring detail editors: intercepts close or
 * row switching with a centered AlertDialog whose initial focus stays on
 * "Keep editing" so edits are not discarded accidentally.
 */
import {useEffect, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {buttonVariants} from '@/components/ui/button';
import {cn} from '@/lib/utils';
import styles from './monitoring.module.css';

export function UnsavedChangesDialog({
  open,
  onDiscard,
  onKeepEditing,
}: {
  open: boolean;
  onDiscard: () => void;
  onKeepEditing: () => void;
}) {
  const t = useTranslations('monitoring');
  const discardingRef = useRef(false);

  useEffect(() => {
    if (open) {
      discardingRef.current = false;
    }
  }, [open]);

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !discardingRef.current) {
          onKeepEditing();
        }
      }}
    >
      <AlertDialogContent
        className={cn(
          'border border-border/80 shadow-2xl dark:border-border/60 sm:max-w-md',
          styles.scope,
        )}
        data-testid='discard-confirm-dialog'
        onOpenAutoFocus={(event) => {
          // 默认焦点留在「继续编辑」，避免回车误触发「放弃修改」。
          // Keep initial focus on "Keep editing" so Enter never discards by accident.
          event.preventDefault();
          const keep = document.querySelector<HTMLElement>(
            '[data-testid="discard-keep-editing"]',
          );
          keep?.focus();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t('unsavedChangesTitle')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('unsavedChangesDescription')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel data-testid='discard-keep-editing'>
            {t('keepEditing')}
          </AlertDialogCancel>
          <AlertDialogAction
            data-testid='discard-confirm-action'
            className={cn(buttonVariants({variant: 'destructive'}))}
            onClick={() => {
              discardingRef.current = true;
              onDiscard();
            }}
          >
            {t('discardChanges')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/** 草稿未保存时拦截浏览器刷新 / 离开。Blocks browser reload/leave while dirty. */
export function useBeforeUnload(dirty: boolean) {
  useEffect(() => {
    if (!dirty) {
      return;
    }
    const handler = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);
}
