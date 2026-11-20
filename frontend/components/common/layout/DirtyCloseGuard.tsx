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
 * 未保存修改的关闭拦截：弹窗 UI 复用 UnsavedChangesDialog，这里绑定公共
 * dirtyClose 文案并附加 beforeunload 拦截；dirty 判定由调用方负责。
 * Dirty-close guard: dialog UI delegates to UnsavedChangesDialog; this wrapper
 * binds the shared dirtyClose copy and wires beforeunload. Dirty detection
 * stays with the caller.
 */
import {useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {UnsavedChangesDialog as SharedUnsavedChangesDialog} from './UnsavedChangesDialog';

export function DirtyCloseDialog({
  open,
  onDiscard,
  onKeepEditing,
}: {
  open: boolean;
  onDiscard: () => void;
  onKeepEditing: () => void;
}) {
  const t = useTranslations('common');

  return (
    <SharedUnsavedChangesDialog
      open={open}
      title={t('dirtyClose.title')}
      description={t('dirtyClose.description')}
      keepLabel={t('dirtyClose.keepEditing')}
      discardLabel={t('dirtyClose.discard')}
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
