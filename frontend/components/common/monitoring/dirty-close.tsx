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
 * 弹窗 UI 复用 common/layout/UnsavedChangesDialog，这里只绑定监控文案与 beforeunload。
 * Shared dirty-close guard for monitoring detail editors: delegates dialog UI
 * to common/layout/UnsavedChangesDialog and wires monitoring copy + beforeunload.
 */
import {useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {UnsavedChangesDialog as SharedUnsavedChangesDialog} from '@/components/common/layout';

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

  return (
    <SharedUnsavedChangesDialog
      open={open}
      title={t('unsavedChangesTitle')}
      description={t('unsavedChangesDescription')}
      keepLabel={t('keepEditing')}
      discardLabel={t('discardChanges')}
      onKeepEditing={onKeepEditing}
      onDiscard={onDiscard}
    />
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
