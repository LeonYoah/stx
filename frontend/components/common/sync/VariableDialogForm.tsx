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
 * Shared Arc form body for GlobalVariableDialog and CustomVariableDialog:
 * RadioCards for type selection, Arc Input for key/secret/description, and
 * Arc Button footer.
 * 全局变量与自定义变量弹窗共用的 Arc 表单体：RadioCards 类型选择、Arc Input 输入
 * 控件与 Arc Button 底栏。
 */
import React from 'react';
import {useTranslations} from 'next-intl';
import {
  Code2,
  Eye,
  EyeOff,
  FileText,
  Lock,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Input} from '@/components/arc/input/input';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import {HelpHint} from '@/components/common/HelpHint';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';

export interface VariableDialogFormProps {
  idPrefix: string;
  secretHintTestId: string;
  isEditing: boolean;
  submitting?: boolean;
  variableKey: string;
  value: string;
  valueType: 'string' | 'secret';
  description: string;
  showPassword: boolean;
  onKeyChange: (next: string) => void;
  onValueChange: (next: string) => void;
  onValueTypeChange: (next: 'string' | 'secret') => void;
  onDescriptionChange: (next: string) => void;
  onTogglePassword: () => void;
  onCancel: () => void;
  onSubmit: (event: React.FormEvent) => void;
}

export function VariableDialogForm({
  idPrefix,
  secretHintTestId,
  isEditing,
  submitting = false,
  variableKey,
  value,
  valueType,
  description,
  showPassword,
  onKeyChange,
  onValueChange,
  onValueTypeChange,
  onDescriptionChange,
  onTogglePassword,
  onCancel,
  onSubmit,
}: VariableDialogFormProps) {
  const t = useTranslations('workbenchStudio');
  const keyId = `${idPrefix}-key`;
  const valueId = `${idPrefix}-value`;
  const descId = `${idPrefix}-description`;

  return (
    <form onSubmit={onSubmit} className='space-y-4'>
      {/* 变量类型选择卡片（Arc RadioCards） / Variable type selector (Arc RadioCards) */}
      <div className='space-y-2'>
        <Label className='text-xs font-medium text-foreground/90'>
          {t('variableType')}
        </Label>
        <RadioCards
          layout='grid'
          minColumnWidth={180}
          value={valueType}
          onValueChange={(next) =>
            onValueTypeChange(next === 'secret' ? 'secret' : 'string')
          }
          options={[
            {
              value: 'string',
              icon: <FileText className='size-4 text-primary' />,
              label: t('typeString'),
              description: t('typeStringDesc'),
            },
            {
              value: 'secret',
              icon: <Lock className='size-4 text-amber-500' />,
              label: t('typeSecret'),
              description: t('typeSecretDesc'),
            },
          ]}
        />
      </div>

      {/* 变量键名输入 / Variable key input */}
      <div className='space-y-1.5'>
        <div className='flex items-center justify-between'>
          <Label
            htmlFor={keyId}
            className='text-xs font-medium text-foreground/90'
          >
            {t('variableKey')} <span className='text-destructive'>*</span>
          </Label>
          {variableKey.trim() ? (
            <span className='inline-flex items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[11px] text-primary/90'>
              <Code2 className='size-3' />
              {`{{${variableKey.trim()}}}`}
            </span>
          ) : null}
        </div>
        <div className='[&_label]:sr-only'>
          <Input
            id={keyId}
            label={t('variableKey')}
            aria-label={t('variableKey')}
            value={variableKey}
            onChange={(event) => onKeyChange(event.target.value)}
            placeholder={t('variableKeyPlaceholder')}
            className='font-mono text-xs'
            autoComplete='off'
            autoFocus={!isEditing}
          />
        </div>
        <p className='text-[11px] text-muted-foreground'>
          {t('variableKeyHint')}
        </p>
      </div>

      {/* 变量值输入及安全提示 / Variable value input & security notice */}
      <div className='space-y-1.5'>
        <div className='flex items-center gap-2'>
          <Label
            htmlFor={valueId}
            className='text-xs font-medium text-foreground/90'
          >
            {t('variableValue')}
            {valueType === 'secret' && !isEditing ? (
              <span className='text-destructive'> *</span>
            ) : null}
          </Label>
          {valueType === 'secret' && !isEditing ? (
            <HelpHint
              label={t('variableValue')}
              content={t('secretProtectionNotice')}
              data-testid={secretHintTestId}
            />
          ) : null}
        </div>

        {valueType === 'secret' ? (
          <div className='space-y-2'>
            {isEditing ? (
              // 编辑敏感凭据提示（留空保持原值，需常显）
              // Secret editing notice banner (blank keeps the current value; stays visible)
              <div className='flex items-start gap-2 rounded-[var(--radius-control)] border border-amber-500/20 bg-amber-500/5 p-2.5 text-xs leading-relaxed text-amber-700 dark:text-amber-300/90'>
                <ShieldCheck className='mt-0.5 size-4 shrink-0 text-amber-500' />
                <div className='text-[11px]'>
                  <span className='mb-0.5 block font-medium'>
                    {t('secretMaskedLabel')}
                  </span>
                  {t('secretEditNotice')}
                </div>
              </div>
            ) : null}

            <div className='relative [&_label]:sr-only'>
              <Input
                id={valueId}
                label={t('variableValue')}
                aria-label={t('variableValue')}
                type={showPassword ? 'text' : 'password'}
                value={value}
                onChange={(event) => onValueChange(event.target.value)}
                placeholder={
                  isEditing
                    ? t('secretValueEditPlaceholder')
                    : t('secretValueCreatePlaceholder')
                }
                className='pr-9 font-mono text-xs'
                autoComplete='new-password'
              />
              <button
                type='button'
                onClick={onTogglePassword}
                aria-label={showPassword ? 'Hide value' : 'Show value'}
                className='absolute right-2.5 top-1/2 -translate-y-1/2 p-0.5 text-muted-foreground hover:text-foreground'
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
            id={valueId}
            value={value}
            onChange={(event) => onValueChange(event.target.value)}
            placeholder={t('variableValuePlaceholder')}
            rows={2}
            className='resize-none rounded-[var(--radius-control)] font-mono text-xs'
          />
        )}
      </div>

      {/* 描述说明 / Description */}
      <Input
        id={descId}
        label={t('optionalDescription')}
        value={description}
        onChange={(event) => onDescriptionChange(event.target.value)}
        placeholder={t('optionalDescription')}
        className='text-xs'
      />

      {/* 语法引用预览底栏 / Syntax reference preview */}
      {variableKey.trim() ? (
        <div className='flex items-center justify-between rounded-[var(--radius-control)] border border-border/50 bg-muted/40 px-3 py-2 text-xs'>
          <span className='flex items-center gap-1 text-[11px] text-muted-foreground'>
            <Sparkles className='size-3 text-amber-500' />
            {t('syntaxPreview')}:
          </span>
          <code className='rounded border border-border/50 bg-background px-1.5 py-0.5 font-mono text-xs font-semibold text-primary'>
            {`{{${variableKey.trim()}}}`}
          </code>
        </div>
      ) : null}

      <div className='flex items-center justify-end gap-2 border-t border-border/50 pt-4'>
        <Button
          type='button'
          variant='secondary'
          size='sm'
          onClick={onCancel}
          disabled={submitting}
        >
          {t('cancel')}
        </Button>
        <Button
          type='submit'
          variant='primary'
          size='sm'
          loading={submitting}
          disabled={submitting}
        >
          {submitting ? t('saving') : t('save')}
        </Button>
      </div>
    </form>
  );
}
