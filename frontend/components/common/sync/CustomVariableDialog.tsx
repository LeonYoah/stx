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

/**
 * 校验变量键名是否为平台保留字
 * Check if variable key is reserved builtin keyword
 */
export function isReservedBuiltinVariableKey(key: string): boolean {
  const trimmed = key.trim();
  if (!trimmed) {
    return false;
  }
  const fixed = new Set([
    'system.biz.date',
    'system.biz.curdate',
    'system.datetime',
    'system.task.execute.path',
    'system.task.instance.id',
    'system.task.definition.name',
    'system.task.definition.code',
    'system.workflow.instance.id',
    'system.workflow.definition.name',
    'system.workflow.definition.code',
    'system.project.name',
    'system.project.code',
  ]);
  if (fixed.has(trimmed) || trimmed.startsWith('system.')) {
    return true;
  }

  // 内置时间函数调用校验，如 add_months(yyyyMMdd, -1)
  if (
    /^(add_months|this_day|last_day|year_week|month_first_day|month_last_day|week_first_day|week_last_day)\s*\(.*\)$/i.test(
      trimmed,
    )
  ) {
    return true;
  }

  // 校验是否为时间格式占位模式（如 yyyyMMdd、yyyy-MM-dd、yyyyMMdd-1 等）
  // 必须包含标准日期占位符（yyyy、MM、dd、HH、mm、ss），且剔除日期占位符和分隔符/偏移量后不得含有普通英文标识符
  const offsetMatch = trimmed.match(/^(.+?)([+-])([0-9*/. ]+)$/);
  const formatExpr = offsetMatch ? offsetMatch[1].trim() : trimmed;
  const dateTokens = ['yyyy', 'MM', 'dd', 'HH', 'mm', 'ss'];
  const hasDateToken = dateTokens.some((token) => formatExpr.includes(token));
  if (!hasDateToken) {
    return false;
  }

  const stripped = formatExpr.replace(/yyyy|MM|dd|HH|mm|ss/g, '');
  if (/[a-zA-Z]/.test(stripped)) {
    // 包含其他英文字符（如 mysqlpass234 中的 pass、password、address 等），为普通变量名
    return false;
  }

  return true;
}

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
