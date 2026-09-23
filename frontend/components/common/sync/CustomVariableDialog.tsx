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
import {toast} from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {
  Lock,
  FileText,
  Eye,
  EyeOff,
  ShieldCheck,
  Code2,
  Sparkles,
} from 'lucide-react';
import {cn} from '@/lib/utils';

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
 * 自定义变量新建/编辑弹窗组件
 * Custom variable creation and editing dialog
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

  // 表单状态
  // Form states
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [valueType, setValueType] = useState<'string' | 'secret'>('string');
  const [description, setDescription] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // 初始化或重置表单
  // Initialize or reset form draft
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

  // 处理保存提交
  // Handle form submission
  const handleSubmit = (event?: React.FormEvent) => {
    if (event) {
      event.preventDefault();
    }
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

    // 新增保密类型时必须输入敏感值
    // Creating secret variable requires non-empty value
    if (!isEditing && valueType === 'secret' && !value) {
      toast.error(t('secretValueCreatePlaceholder'));
      return;
    }

    // 编辑保密类型时，若留空则保留原值
    // When editing secret variable, blank value preserves existing value
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
      <DialogContent className='sm:max-w-[500px] border-border/80 bg-background/95 backdrop-blur shadow-2xl'>
        <form onSubmit={handleSubmit}>
          <DialogHeader className='space-y-1.5 pb-2'>
            <div className='flex items-center gap-2 text-primary font-semibold'>
              {valueType === 'secret' ? (
                <Lock className='size-5 text-amber-500' />
              ) : (
                <FileText className='size-5 text-primary' />
              )}
              <DialogTitle className='text-base'>
                {isEditing ? t('editCustomVariable') : t('newCustomVariable')}
              </DialogTitle>
            </div>
            <DialogDescription className='text-xs text-muted-foreground'>
              {t('customVariableDialogDesc')}
            </DialogDescription>
          </DialogHeader>

          <div className='space-y-4 py-2'>
            {/* 变量类型选择卡片 */}
            {/* Variable type selector cards */}
            <div className='space-y-1.5'>
              <Label className='text-xs font-medium text-foreground/90'>
                {t('variableType')}
              </Label>
              <div className='grid grid-cols-2 gap-2'>
                <button
                  type='button'
                  onClick={() => setValueType('string')}
                  className={cn(
                    'relative flex flex-col items-start gap-1 p-3 rounded-lg border text-left transition-all duration-200',
                    valueType === 'string'
                      ? 'border-primary bg-primary/5 text-foreground shadow-xs ring-1 ring-primary/30'
                      : 'border-border/60 bg-muted/20 text-muted-foreground hover:bg-muted/40 hover:border-border',
                  )}
                >
                  <div className='flex items-center gap-1.5 font-medium text-xs text-foreground'>
                    <FileText className='size-3.5 text-primary' />
                    {t('typeString')}
                  </div>
                  <p className='text-[11px] leading-normal text-muted-foreground'>
                    {t('typeStringDesc')}
                  </p>
                </button>

                <button
                  type='button'
                  onClick={() => setValueType('secret')}
                  className={cn(
                    'relative flex flex-col items-start gap-1 p-3 rounded-lg border text-left transition-all duration-200',
                    valueType === 'secret'
                      ? 'border-amber-500/70 bg-amber-500/5 text-foreground shadow-xs ring-1 ring-amber-500/30'
                      : 'border-border/60 bg-muted/20 text-muted-foreground hover:bg-muted/40 hover:border-border',
                  )}
                >
                  <div className='flex items-center gap-1.5 font-medium text-xs text-amber-500'>
                    <Lock className='size-3.5' />
                    {t('typeSecret')}
                  </div>
                  <p className='text-[11px] leading-normal text-muted-foreground'>
                    {t('typeSecretDesc')}
                  </p>
                </button>
              </div>
            </div>

            {/* 变量键名输入 */}
            {/* Variable Key Input */}
            <div className='space-y-1.5'>
              <div className='flex items-center justify-between'>
                <Label
                  htmlFor='custom-variable-key'
                  className='text-xs font-medium text-foreground/90'
                >
                  {t('variableKey')} <span className='text-destructive'>*</span>
                </Label>
                {key.trim() && (
                  <span className='inline-flex items-center gap-1 font-mono text-[11px] text-primary/90 bg-primary/10 px-1.5 py-0.5 rounded'>
                    <Code2 className='size-3' />
                    {`{{${key.trim()}}}`}
                  </span>
                )}
              </div>
              <Input
                id='custom-variable-key'
                value={key}
                onChange={(event) => setKey(event.target.value)}
                placeholder={t('variableKeyPlaceholder')}
                className='h-8 font-mono text-xs'
                autoComplete='off'
                autoFocus={!isEditing}
              />
              <p className='text-[11px] text-muted-foreground'>
                {t('variableKeyHint')}
              </p>
            </div>

            {/* 变量值输入及安全提示 */}
            {/* Variable Value Input & Security Callout */}
            <div className='space-y-1.5'>
              <Label
                htmlFor='custom-variable-value'
                className='text-xs font-medium text-foreground/90'
              >
                {t('variableValue')}
                {valueType === 'secret' && !isEditing && (
                  <span className='text-destructive'> *</span>
                )}
              </Label>

              {valueType === 'secret' ? (
                <div className='space-y-2'>
                  {isEditing ? (
                    // 编辑敏感凭据提示
                    // Secret editing notice banner
                    <div className='flex items-start gap-2 rounded-lg border border-amber-500/20 bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300/90 leading-relaxed'>
                      <ShieldCheck className='size-4 shrink-0 mt-0.5 text-amber-500' />
                      <div className='text-[11px]'>
                        <span className='font-medium block mb-0.5'>
                          {t('secretMaskedLabel')}
                        </span>
                        {t('secretEditNotice')}
                      </div>
                    </div>
                  ) : (
                    // 新建敏感凭据提示
                    // Secret creation security notice
                    <div className='flex items-start gap-2 rounded-lg border border-primary/20 bg-primary/5 p-2.5 text-xs text-muted-foreground leading-relaxed'>
                      <Lock className='size-4 shrink-0 mt-0.5 text-primary' />
                      <div className='text-[11px]'>
                        <span className='font-medium text-foreground block mb-0.5'>
                          {t('typeSecret')}
                        </span>
                        {t('secretProtectionNotice')}
                      </div>
                    </div>
                  )}

                  <div className='relative'>
                    <Input
                      id='custom-variable-value'
                      type={showPassword ? 'text' : 'password'}
                      value={value}
                      onChange={(event) => setValue(event.target.value)}
                      placeholder={
                        isEditing
                          ? t('secretValueEditPlaceholder')
                          : t('secretValueCreatePlaceholder')
                      }
                      className='h-8 pr-8 font-mono text-xs'
                      autoComplete='new-password'
                    />
                    <button
                      type='button'
                      onClick={() => setShowPassword(!showPassword)}
                      className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground p-0.5'
                      tabIndex={-1}
                    >
                      {showPassword ? (
                        <EyeOff className='size-3.5' />
                      ) : (
                        <Eye className='size-3.5' />
                      )}
                    </button>
                  </div>
                </div>
              ) : (
                <Textarea
                  id='custom-variable-value'
                  value={value}
                  onChange={(event) => setValue(event.target.value)}
                  placeholder={t('variableValuePlaceholder')}
                  rows={2}
                  className='font-mono text-xs resize-none'
                />
              )}
            </div>

            {/* 描述说明 */}
            {/* Description */}
            <div className='space-y-1.5'>
              <Label
                htmlFor='custom-variable-description'
                className='text-xs font-medium text-foreground/90'
              >
                {t('optionalDescription')}
              </Label>
              <Input
                id='custom-variable-description'
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={t('optionalDescription')}
                className='h-8 text-xs'
              />
            </div>

            {/* 语法引用预览底栏 */}
            {/* Syntax Reference Preview Footer */}
            {key.trim() && (
              <div className='flex items-center justify-between p-2 rounded-lg bg-muted/40 border border-border/40 text-xs'>
                <span className='text-[11px] text-muted-foreground flex items-center gap-1'>
                  <Sparkles className='size-3 text-amber-500' />
                  {t('syntaxPreview')}:
                </span>
                <code className='font-mono font-semibold text-xs text-primary bg-background px-1.5 py-0.5 rounded border border-border/50'>
                  {`{{${key.trim()}}}`}
                </code>
              </div>
            )}
          </div>

          <DialogFooter className='pt-2 gap-2 sm:gap-0'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='h-8 text-xs'
              onClick={() => onOpenChange(false)}
            >
              {t('cancel')}
            </Button>
            <Button
              type='submit'
              size='sm'
              className='h-8 text-xs'
            >
              {t('save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
