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

// 全局变量新建与编辑弹窗组件
// Global variable create and edit dialog component

'use client';

import React, {useState, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {FileText, Lock} from 'lucide-react';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {
  SyncGlobalVariable,
  CreateSyncGlobalVariableRequest,
} from '@/lib/services/sync/types';
import {VariableDialogForm} from './VariableDialogForm';

export interface GlobalVariableDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variable: SyncGlobalVariable | null;
  onSave: (
    item: SyncGlobalVariable | null,
    payload: CreateSyncGlobalVariableRequest,
  ) => Promise<boolean | void>;
}

/**
 * 全局变量新建/编辑弹窗组件（使用 Arc Dialog + VariableDialogForm）。
 * Global variable creation and editing dialog (Arc Dialog + VariableDialogForm).
 */
export function GlobalVariableDialog({
  open,
  onOpenChange,
  variable,
  onSave,
}: GlobalVariableDialogProps) {
  const t = useTranslations('workbenchStudio');
  const isEditing = variable !== null;

  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [valueType, setValueType] = useState<'string' | 'secret'>('string');
  const [description, setDescription] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (variable) {
      setKey(variable.key);
      const isSecret =
        variable.value_type === 'secret' || variable.value === '******';
      setValueType(isSecret ? 'secret' : 'string');
      // 编辑保密类型时，输入框保持留空以支持“留空保持不变”
      // When editing secret type, keep input empty to support "leave blank to keep unchanged"
      setValue(isSecret ? '' : variable.value || '');
      setDescription(variable.description || '');
      setShowPassword(false);
    } else {
      setKey('');
      setValue('');
      setValueType('string');
      setDescription('');
      setShowPassword(false);
    }
  }, [open, variable]);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const trimmedKey = key.trim();
    if (!trimmedKey) {
      toast.error(t('variableKeyRequired'));
      return;
    }
    if (!isEditing && valueType === 'secret' && !value) {
      toast.error(t('secretValueCreatePlaceholder'));
      return;
    }

    setSubmitting(true);
    try {
      await onSave(variable, {
        key: trimmedKey,
        value,
        value_type: valueType,
        description: description.trim(),
      });
      onOpenChange(false);
    } catch {
      // 外部 onSave 已处理 toast 提示
      // Outer onSave already caught and toasted error
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='arc-compact w-[min(calc(100vw-2rem),500px)]!'
        aria-describedby={undefined}
        title={
          <span className='flex items-center gap-2'>
            {valueType === 'secret' ? (
              <Lock className='size-4 text-amber-500' />
            ) : (
              <FileText className='size-4 text-primary' />
            )}
            <span>
              {isEditing ? t('editGlobalVariable') : t('newGlobalVariable')}
            </span>
          </span>
        }
      >
        <VariableDialogForm
          idPrefix='variable'
          secretHintTestId='variable-value-secret-hint'
          isEditing={isEditing}
          submitting={submitting}
          variableKey={key}
          value={value}
          valueType={valueType}
          description={description}
          showPassword={showPassword}
          onKeyChange={setKey}
          onValueChange={setValue}
          onValueTypeChange={setValueType}
          onDescriptionChange={setDescription}
          onTogglePassword={() => setShowPassword((prev) => !prev)}
          onCancel={() => onOpenChange(false)}
          onSubmit={(e) => void handleSubmit(e)}
        />
      </DialogContent>
    </Dialog>
  );
}
