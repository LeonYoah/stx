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

// 自定义变量新建与编辑弹窗组件
// Custom variable create and edit dialog component

'use client';

import React, {useState, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {FileText, Lock} from 'lucide-react';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {VariableDialogForm} from './VariableDialogForm';
import {isReservedBuiltinVariableKey} from './builtin-time-variables';

export interface CustomVariableItem {
  id: string;
  key: string;
  value: string;
  type?: 'string' | 'secret';
  description?: string;
}

export interface CustomVariableDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variable: CustomVariableItem | null;
  existingKeys?: string[];
  onSave: (payload: {
    key: string;
    value: string;
    type: 'string' | 'secret';
    description?: string;
  }) => void;
}

// 单一实现在 sync-studio-utils，这里只做 re-export 保持既有引用不变
// Single implementation lives in sync-studio-utils; re-exported here to keep existing imports working
export {isReservedBuiltinVariableKey};

/**
 * 自定义变量新建/编辑弹窗组件（使用 Arc Dialog + VariableDialogForm）。
 * Custom variable creation and editing dialog (Arc Dialog + VariableDialogForm).
 */
export function CustomVariableDialog({
  open,
  onOpenChange,
  variable,
  existingKeys = [],
  onSave,
}: CustomVariableDialogProps) {
  const t = useTranslations('workbenchStudio');
  const isEditing = variable !== null;

  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [valueType, setValueType] = useState<'string' | 'secret'>('string');
  const [description, setDescription] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (variable) {
      setKey(variable.key);
      const isSecret = variable.type === 'secret';
      setValueType(isSecret ? 'secret' : 'string');
      // 编辑保密类型时输入框留空，提示用户留空保持原值
      // Keep input empty when editing secret, advising user that blank keeps original
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

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const trimmedKey = key.trim();
    if (!trimmedKey) {
      toast.error(t('variableKeyRequired'));
      return;
    }

    // 格式校验：仅允许字母、数字、下划线、中划线、点
    // Key format validation: only letters, numbers, _, -, .
    if (!/^[a-zA-Z0-9_.-]+$/.test(trimmedKey)) {
      toast.error(t('variableKeyInvalid'));
      return;
    }

    // 保留字冲突校验
    // Check against reserved keywords
    if (isReservedBuiltinVariableKey(trimmedKey)) {
      toast.error(
        t('reservedBuiltinVariableKey', {key: `{{${trimmedKey}}}`}),
      );
      return;
    }

    // 唯一性校验（排除自身）
    // Duplicate check excluding current variable
    const isDuplicate = existingKeys.some((existingKey) => {
      if (
        isEditing &&
        variable &&
        variable.key.trim().toLowerCase() === existingKey.trim().toLowerCase()
      ) {
        return false;
      }
      return existingKey.trim().toLowerCase() === trimmedKey.toLowerCase();
    });
    if (isDuplicate) {
      toast.error(t('duplicateCustomVariableKey'));
      return;
    }

    if (!isEditing && valueType === 'secret' && !value) {
      toast.error(t('secretValueCreatePlaceholder'));
      return;
    }

    const finalValue =
      isEditing && valueType === 'secret' && !value
        ? variable?.value || ''
        : value;

    onSave({
      key: trimmedKey,
      value: finalValue,
      type: valueType,
      description: description.trim(),
    });
    onOpenChange(false);
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
              {isEditing ? t('editCustomVariable') : t('newCustomVariable')}
            </span>
          </span>
        }
      >
        <VariableDialogForm
          idPrefix='custom-variable'
          secretHintTestId='custom-variable-value-secret-hint'
          isEditing={isEditing}
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
          onSubmit={handleSubmit}
        />
      </DialogContent>
    </Dialog>
  );
}
