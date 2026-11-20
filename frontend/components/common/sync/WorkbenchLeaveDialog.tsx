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
import {UnsavedChangesDialog} from '@/components/common/layout';

/**
 * 工作台未落盘草稿离开确认：留下 / 保存并离开 / 丢弃并离开。
 * 视觉与交互统一复用规则页共用的 UnsavedChangesDialog。
 * Workbench leave confirm when drafts are unsaved: Stay / Save&Leave / Discard&Leave,
 * reusing the shared rule-page UnsavedChangesDialog shell and focus behavior.
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
    <UnsavedChangesDialog
      open={open}
      busy={busy}
      title={t('leaveUnsavedTitle')}
      description={t('leaveUnsavedDescription')}
      keepLabel={t('leaveStay')}
      discardLabel={t('leaveDiscardAndLeave')}
      saveLabel={t('leaveSaveAndLeave')}
      savingLabel={t('leaveSavingAndLeaving')}
      onOpenChange={onOpenChange}
      onKeepEditing={onStay}
      onDiscard={onDiscardAndLeave}
      onSave={onSaveAndLeave}
      dialogTestId='studio-leave-unsaved-dialog'
      keepTestId='studio-leave-stay'
      saveTestId='studio-leave-save'
      discardTestId='studio-leave-discard'
    />
  );
}
