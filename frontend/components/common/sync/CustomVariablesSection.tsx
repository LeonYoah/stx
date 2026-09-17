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

// 自定义变量配置面板展示组件
// Custom variables configuration section component for workbench studio

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Label} from '@/components/ui/label';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {CustomVariableItem} from './CustomVariableDialog';
import {
  Plus,
  Lock,
  Code2,
  Pencil,
  Trash2,
  Copy,
  KeyRound,
  ShieldCheck,
  HelpCircle,
} from 'lucide-react';

export interface CustomVariablesSectionProps {
  variables: CustomVariableItem[];
  onOpenCreate: () => void;
  onOpenEdit: (item: CustomVariableItem) => void;
  onDelete: (id: string) => void;
  onCopyReference: (key: string) => void;
  onCopyValue: (value: string) => void;
}

/**
 * 任务级自定义变量展示面板
 * Task-level custom variables presentation section
 */
export function CustomVariablesSection({
  variables,
  onOpenCreate,
  onOpenEdit,
  onDelete,
  onCopyReference,
  onCopyValue,
}: CustomVariablesSectionProps) {
  const t = useTranslations('workbenchStudio');

  return (
    <div className='rounded-lg border border-border/50 bg-muted/10 p-3 space-y-2.5'>
      {/* 顶栏：标题 + 数量 + 说明提示 + 新增按钮 */}
      {/* Header: Title + count badge + tooltip + create button */}
      <div className='flex items-center justify-between gap-2'>
        <div className='flex items-center gap-1.5'>
          <Label className='text-xs font-semibold text-foreground'>
            {t('customVariables')}
          </Label>
          <Badge
            variant='secondary'
            className='h-4 px-1.5 text-[10px] font-normal text-muted-foreground'
          >
            {variables.length}
          </Badge>
          <Tooltip>
            <TooltipTrigger asChild>
              <HelpCircle className='size-3 text-muted-foreground/60 hover:text-foreground cursor-help' />
            </TooltipTrigger>
            <TooltipContent side='top' className='text-xs max-w-[260px]'>
              {t('customVariablesDesc')}
            </TooltipContent>
          </Tooltip>
        </div>

        <Button
          type='button'
          size='sm'
          variant='outline'
          className='h-7 px-2 text-xs gap-1'
          onClick={onOpenCreate}
        >
          <Plus className='size-3.5' />
          {t('add')}
        </Button>
      </div>

      {/* 变量列表 */}
      {/* Variables list */}
      <div className='space-y-2'>
        {variables.length === 0 ? (
          // 空状态
          // Empty state
          <div className='flex flex-col items-center justify-center py-6 px-2 text-center rounded-lg border border-dashed border-border/60 bg-background/50'>
            <KeyRound className='size-7 text-muted-foreground/40 mb-1.5' />
            <p className='text-xs font-medium text-muted-foreground'>
              {t('noCustomVariables')}
            </p>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='h-7 text-xs mt-2.5 gap-1'
              onClick={onOpenCreate}
            >
              <Plus className='size-3' />
              {t('addCustomVariable')}
            </Button>
          </div>
        ) : (
          variables.map((item) => {
            const isSecret = item.type === 'secret';

            return (
              <div
                key={item.id}
                className='group relative rounded-lg border border-border/60 bg-background/80 hover:bg-background hover:border-primary/40 hover:shadow-xs transition-all duration-200 p-2.5 space-y-2'
              >
                {/* 顶行：变量引用语法 + 类型标识 + 编辑/删除操作 */}
                {/* Header row: variable reference syntax + type badge + actions */}
                <div className='flex items-center justify-between gap-1.5'>
                  <div className='flex items-center gap-1.5 min-w-0 flex-wrap'>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type='button'
                          onClick={() => onCopyReference(item.key)}
                          className='inline-flex items-center gap-1 font-mono font-medium text-xs text-foreground bg-muted/60 hover:bg-primary/10 hover:text-primary px-1.5 py-0.5 rounded border border-border/50 transition-colors cursor-pointer text-left'
                        >
                          <Code2 className='size-3 shrink-0 text-primary' />
                          <span className='truncate max-w-[140px]'>
                            {`{{${item.key}}}`}
                          </span>
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('copyReference')}: {`{{${item.key}}}`}
                      </TooltipContent>
                    </Tooltip>

                    {isSecret ? (
                      <Badge
                        variant='outline'
                        className='text-[10px] px-1.5 py-0 h-4 border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10 gap-0.5 font-normal shrink-0'
                      >
                        <Lock className='size-2.5' />
                        {t('secretMaskedBadge')}
                      </Badge>
                    ) : (
                      <Badge
                        variant='secondary'
                        className='text-[10px] px-1.5 py-0 h-4 font-normal text-muted-foreground shrink-0'
                      >
                        {t('stringTypeBadge')}
                      </Badge>
                    )}
                  </div>

                  {/* 右侧快捷操作按钮 */}
                  {/* Right quick actions */}
                  <div className='flex items-center gap-0.5 shrink-0 opacity-80 group-hover:opacity-100 transition-opacity'>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          type='button'
                          size='icon'
                          variant='ghost'
                          className='size-6 text-muted-foreground hover:text-primary'
                          onClick={() => onOpenEdit(item)}
                        >
                          <Pencil className='size-3' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('edit')}
                      </TooltipContent>
                    </Tooltip>

                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          type='button'
                          size='icon'
                          variant='ghost'
                          className='size-6 text-muted-foreground hover:text-destructive'
                          onClick={() => onDelete(item.id)}
                        >
                          <Trash2 className='size-3' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('delete')}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                </div>

                {/* 变量值展示区域：保密脱敏 vs 普通文本 */}
                {/* Value display area: Secret masked vs plain text */}
                {isSecret ? (
                  <div className='flex items-center gap-1.5 text-xs text-muted-foreground font-mono bg-amber-500/5 px-2 py-1 rounded border border-amber-500/20 select-none'>
                    <ShieldCheck className='size-3 text-amber-500 shrink-0' />
                    <span className='tracking-widest text-foreground/80 font-bold text-[11px]'>
                      ••••••••
                    </span>
                    <span className='text-[10px] text-muted-foreground/80 ml-auto'>
                      {t('secretMaskedLabel')}
                    </span>
                  </div>
                ) : (
                  <div className='flex items-center justify-between gap-1 text-xs font-mono bg-muted/30 px-2 py-1 rounded border border-border/40'>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className='truncate text-foreground/90 max-w-[200px]'>
                          {item.value || '-'}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent className='max-w-[320px] break-all font-mono text-xs'>
                        {item.value || '-'}
                      </TooltipContent>
                    </Tooltip>
                    {item.value ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            type='button'
                            size='icon'
                            variant='ghost'
                            className='size-5 text-muted-foreground hover:text-foreground shrink-0'
                            onClick={() => onCopyValue(item.value)}
                          >
                            <Copy className='size-3' />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs'>
                          {t('copyValue')}
                        </TooltipContent>
                      </Tooltip>
                    ) : null}
                  </div>
                )}

                {/* 描述信息（如果有） */}
                {/* Optional description */}
                {item.description ? (
                  <p className='text-[11px] text-muted-foreground/80 truncate px-0.5'>
                    {item.description}
                  </p>
                ) : null}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
