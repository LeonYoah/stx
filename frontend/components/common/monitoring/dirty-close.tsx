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
 * Dirty-close guard shared by the monitoring drawers: when a close attempt is
 * intercepted, the drawer's own footer swaps to this inline confirm row
 * (有未保存的修改 / 放弃修改 / 继续编辑) instead of opening another modal.
 * 监控抽屉共用的未保存关闭守卫：拦截关闭后在抽屉 footer 原位切换成内联确认行，
 * 不再弹新组件。
 */
import {useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';

export function DiscardConfirmStrip({
  onDiscard,
  onKeepEditing,
}: {
  onDiscard: () => void;
  onKeepEditing: () => void;
}) {
  const t = useTranslations('monitoring');
  return (
    <div className='flex w-full items-center gap-2' data-testid='discard-confirm'>
      <span className='min-w-0 flex-1 truncate text-xs text-destructive'>
        {t('unsavedChanges')}
      </span>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        className='h-7 rounded-md text-xs text-destructive hover:text-destructive'
        onClick={onDiscard}
      >
        {t('discardChanges')}
      </Button>
      <Button type='button' size='sm' className='h-7 rounded-md text-xs' onClick={onKeepEditing}>
        {t('keepEditing')}
      </Button>
    </div>
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
