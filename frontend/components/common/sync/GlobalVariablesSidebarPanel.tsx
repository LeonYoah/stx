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

// 全局变量工作台侧边栏面板组件（在全局变量类型 Tab 下集成全部、文本、时间、保密四类）
// Global variables sidebar panel component for workbench studio (integrates All, Text, Time, and Secret under sub-tabs)

'use client';

import React, {useState, useMemo, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Badge} from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {SyncGlobalVariable} from '@/lib/services/sync/types';
import {
  BUILTIN_TIME_VARIABLE_ITEMS,
  resolveBuiltinPreviewExpression,
  BuiltinTimeVariableItem,
} from './builtin-time-variables';
import {
  Globe2,
  Plus,
  Search,
  Lock,
  FileText,
  Copy,
  Pencil,
  Trash2,
  MoreHorizontal,
  Code2,
  ShieldCheck,
  X,
  KeyRound,
  Clock3,
  CalendarDays,
} from 'lucide-react';
import {cn} from '@/lib/utils';

export type GlobalVariableTabType = 'all' | 'string' | 'time' | 'secret';

export interface GlobalVariablesSidebarPanelProps {
  variables: SyncGlobalVariable[];
  total: number;
  page: number;
  pageSize: number;
  isAdmin?: boolean;
  currentUserId?: number;
  defaultTab?: GlobalVariableTabType | 'vars';
  onPageChange: (page: number) => void;
  onOpenCreate: () => void;
  onOpenEdit: (item: SyncGlobalVariable) => void;
  onDelete: (id: number) => void;
  onCopyValue: (value: string) => void;
  onCopyReference: (key: string) => void;
}

/**
 * 简单分页组件
 * Simple pagination component
 */
function SimplePagination({
  total,
  page,
  pageSize,
  onPageChange,
}: {
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  if (totalPages <= 1) {
    return null;
  }

  return (
    <div className='flex items-center justify-between pt-2 border-t border-border/40 text-[11px] text-muted-foreground'>
      <span>
        {page} / {totalPages}
      </span>
      <div className='flex items-center gap-1'>
        <Button
          size='sm'
          variant='ghost'
          className='h-6 px-2 text-[11px]'
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          {t('prevPage')}
        </Button>
        <Button
          size='sm'
          variant='ghost'
          className='h-6 px-2 text-[11px]'
          disabled={page >= totalPages}
          onClick={() => onPageChange(Math.min(totalPages, page + 1))}
        >
          {t('nextPage')}
        </Button>
      </div>
    </div>
  );
}

/**
 * 全局变量侧边栏面板组件
 * Global variables sidebar panel
 */
export function GlobalVariablesSidebarPanel({
  variables,
  total,
  page,
  pageSize,
  isAdmin = true,
  currentUserId,
  defaultTab = 'all',
  onPageChange,
  onOpenCreate,
  onOpenEdit,
  onDelete,
  onCopyValue,
  onCopyReference,
}: GlobalVariablesSidebarPanelProps) {
  const t = useTranslations('workbenchStudio');

  // 将外部传入的默认 Tab 标准化为内部类型
  // Normalize incoming defaultTab prop to internal type
  const resolveInitialTab = (
    tab?: GlobalVariableTabType | 'vars',
  ): GlobalVariableTabType => {
    if (tab === 'vars' || !tab) return 'all';
    return tab;
  };

  // 类型过滤状态：全部 | 文本 | 时间 | 保密
  // Type filter state: all | string | time | secret
  const [typeFilter, setTypeFilter] = useState<GlobalVariableTabType>(() =>
    resolveInitialTab(defaultTab),
  );

  useEffect(() => {
    if (defaultTab) {
      setTypeFilter(resolveInitialTab(defaultTab));
    }
  }, [defaultTab]);

  // 统一搜索关键字（对全局变量和内置时间变量同时生效）
  // Unified search query (applies to both global and builtin time variables)
  const [searchQuery, setSearchQuery] = useState('');

  // 内置时间变量类别过滤
  // Builtin time variables category filter
  const [timeCategoryFilter, setTimeCategoryFilter] = useState<
    'all' | BuiltinTimeVariableItem['category']
  >('all');

  // 固定的实时求值基准时间
  // Fixed evaluation baseline timestamp
  const now = useMemo(() => new Date(), []);

  // 本地过滤后的全局变量列表
  // Filtered global variables
  const filteredVariables = useMemo(() => {
    return variables.filter((item) => {
      const isSecret = item.value_type === 'secret' || item.value === '******';
      if (typeFilter === 'string' && isSecret) {
        return false;
      }
      if (typeFilter === 'secret' && !isSecret) {
        return false;
      }
      if (!searchQuery.trim()) {
        return true;
      }
      const q = searchQuery.toLowerCase().trim();
      return (
        item.key.toLowerCase().includes(q) ||
        (item.description && item.description.toLowerCase().includes(q))
      );
    });
  }, [variables, typeFilter, searchQuery]);

  // 本地过滤后的内置时间变量列表
  // Filtered built-in time variables
  const filteredTimeVariables = useMemo(() => {
    return BUILTIN_TIME_VARIABLE_ITEMS.filter((item) => {
      if (timeCategoryFilter !== 'all' && item.category !== timeCategoryFilter) {
        return false;
      }
      if (!searchQuery.trim()) {
        return true;
      }
      const q = searchQuery.toLowerCase().trim();
      const desc = t(item.descKey).toLowerCase();
      return item.expr.toLowerCase().includes(q) || desc.includes(q);
    });
  }, [timeCategoryFilter, searchQuery, t]);

  const isTimeMode = typeFilter === 'time';

  return (
    <div className='min-w-0 w-full space-y-3'>
      {/* 顶部标题栏与操作 */}
      {/* Header bar and action */}
      <div className='space-y-2.5 border-b border-border/40 pb-3'>
        <div className='flex items-center justify-between gap-1'>
          <div className='flex items-center gap-1.5 min-w-0'>
            <Globe2 className='size-4 text-primary shrink-0' />
            <span className='font-semibold text-xs tracking-tight text-foreground truncate'>
              {t('globalVariables')}
            </span>
            <Badge
              variant='secondary'
              className='h-4 px-1.5 text-[10px] font-mono text-muted-foreground'
            >
              {isTimeMode ? BUILTIN_TIME_VARIABLE_ITEMS.length : total}
            </Badge>
          </div>

          {!isTimeMode && (
            <Button
              size='sm'
              className='h-7 px-2.5 text-xs gap-1 shadow-xs'
              onClick={onOpenCreate}
            >
              <Plus className='size-3.5' />
              <span>{t('newCreate')}</span>
            </Button>
          )}
        </div>

        <p className='text-[11px] leading-relaxed text-muted-foreground'>
          {isTimeMode ? t('timeVariablesTabDesc') : t('globalVariablesDesc')}
        </p>

        {/* 快捷搜索栏 */}
        {/* Search input */}
        <div className='relative'>
          <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground' />
          <Input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={
              isTimeMode ? t('searchTimeVariables') : t('searchVariables')
            }
            className='h-7 pl-7 pr-7 text-xs bg-muted/20 border-border/50'
          />
          {searchQuery && (
            <button
              type='button'
              onClick={() => setSearchQuery('')}
              className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
            >
              <X className='size-3' />
            </button>
          )}
        </div>

        {/* 类型切换标签：全部 / 文本 / 时间 / 保密 */}
        {/* Type filter tabs: All / Text / Time / Secret */}
        <div className='grid grid-cols-4 gap-1 p-0.5 rounded-md bg-muted/30 border border-border/40 text-[11px]'>
          <button
            type='button'
            onClick={() => setTypeFilter('all')}
            className={cn(
              'py-1 rounded transition-colors text-center font-medium',
              typeFilter === 'all'
                ? 'bg-background text-foreground shadow-2xs font-semibold'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t('filterAll')}
          </button>
          <button
            type='button'
            onClick={() => setTypeFilter('string')}
            className={cn(
              'py-1 rounded transition-colors text-center font-medium flex items-center justify-center gap-1',
              typeFilter === 'string'
                ? 'bg-background text-foreground shadow-2xs font-semibold'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <FileText className='size-2.5' />
            <span>{t('filterText')}</span>
          </button>
          <button
            type='button'
            onClick={() => setTypeFilter('time')}
            className={cn(
              'py-1 rounded transition-colors text-center font-medium flex items-center justify-center gap-1',
              typeFilter === 'time'
                ? 'bg-background text-sky-600 dark:text-sky-400 shadow-2xs font-semibold'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Clock3 className='size-2.5' />
            <span>{t('filterTime')}</span>
          </button>
          <button
            type='button'
            onClick={() => setTypeFilter('secret')}
            className={cn(
              'py-1 rounded transition-colors text-center font-medium flex items-center justify-center gap-1',
              typeFilter === 'secret'
                ? 'bg-background text-amber-500 shadow-2xs font-semibold'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Lock className='size-2.5' />
            <span>{t('filterSecret')}</span>
          </button>
        </div>

        {/* 当切换至“时间”类型时，展示类别筛选细分胶囊 */}
        {/* Category filter pills shown only under Time tab */}
        {isTimeMode && (
          <div className='flex flex-wrap gap-1 pt-1 border-t border-border/30'>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('all')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'all'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('filterAll')}
            </button>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('business')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'business'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('categoryBusiness')}
            </button>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('datetime')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'datetime'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('categoryDatetime')}
            </button>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('format')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'format'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('categoryFormat')}
            </button>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('offset')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'offset'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('categoryOffset')}
            </button>
            <button
              type='button'
              onClick={() => setTimeCategoryFilter('calendar')}
              className={cn(
                'rounded px-2 py-0.5 text-[10px] font-medium transition-colors',
                timeCategoryFilter === 'calendar'
                  ? 'bg-primary text-primary-foreground shadow-2xs'
                  : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t('categoryCalendar')}
            </button>
          </div>
        )}
      </div>

      {/* 主体列表渲染区域 */}
      {/* Main list rendering area */}
      {isTimeMode ? (
        /* 内置时间变量卡片列表 */
        /* Builtin time variables list */
        <div className='space-y-2 pb-1'>
          {filteredTimeVariables.length === 0 ? (
            <div className='flex flex-col items-center justify-center py-8 px-2 text-center rounded-lg border border-dashed border-border/60 bg-muted/10'>
              <CalendarDays className='size-8 text-muted-foreground/40 mb-2' />
              <p className='text-xs font-medium text-muted-foreground'>
                {t('noMatchingTimeVariables')}
              </p>
              {(searchQuery || timeCategoryFilter !== 'all') && (
                <Button
                  variant='outline'
                  size='sm'
                  className='h-7 text-xs mt-3'
                  onClick={() => {
                    setSearchQuery('');
                    setTimeCategoryFilter('all');
                  }}
                >
                  {t('reset')}
                </Button>
              )}
            </div>
          ) : (
            filteredTimeVariables.map((item) => {
              const previewValue = resolveBuiltinPreviewExpression(
                item.expr,
                now,
              );

              return (
                <div
                  key={item.expr}
                  className='group relative rounded-lg border border-border/60 bg-card/70 hover:bg-card/95 hover:border-sky-500/40 hover:shadow-xs transition-all duration-200 p-2.5 space-y-2'
                >
                  {/* 顶栏：变量表达式 + 类别标签 + 复制快捷键 */}
                  {/* Header: Expression badge + category pill + quick copy */}
                  <div className='flex items-center justify-between gap-1.5'>
                    <div className='flex items-center gap-1.5 min-w-0 flex-wrap'>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type='button'
                            onClick={() => onCopyReference(item.expr)}
                            className='inline-flex items-center gap-1 font-mono font-medium text-xs text-foreground bg-muted/60 hover:bg-sky-500/10 hover:text-sky-600 dark:hover:text-sky-400 px-1.5 py-0.5 rounded border border-border/50 transition-colors cursor-pointer text-left'
                          >
                            <Code2 className='size-3 shrink-0 text-sky-500' />
                            <span className='truncate max-w-[150px]'>
                              {`{{${item.expr}}}`}
                            </span>
                          </button>
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs'>
                          {t('copyReference')}: {`{{${item.expr}}}`}
                        </TooltipContent>
                      </Tooltip>

                      <Badge
                        variant='secondary'
                        className='text-[10px] px-1.5 py-0 h-4 font-normal text-muted-foreground shrink-0'
                      >
                        {item.category === 'business'
                          ? t('categoryBusiness')
                          : item.category === 'datetime'
                            ? t('categoryDatetime')
                            : item.category === 'format'
                              ? t('categoryFormat')
                              : item.category === 'offset'
                                ? t('categoryOffset')
                                : t('categoryCalendar')}
                      </Badge>
                    </div>

                    {/* 复制按键组 */}
                    {/* Copy actions */}
                    <div className='flex items-center gap-0.5 shrink-0 opacity-80 group-hover:opacity-100 transition-opacity'>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            size='icon'
                            variant='ghost'
                            className='size-6 text-muted-foreground hover:text-sky-500'
                            onClick={() => onCopyReference(item.expr)}
                          >
                            <Code2 className='size-3' />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs'>
                          {t('copySyntax')}
                        </TooltipContent>
                      </Tooltip>

                      {previewValue ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              size='icon'
                              variant='ghost'
                              className='size-6 text-muted-foreground hover:text-foreground'
                              onClick={() => onCopyValue(previewValue)}
                            >
                              <Copy className='size-3' />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent side='top' className='text-xs'>
                            {t('copyValue')}: {previewValue}
                          </TooltipContent>
                        </Tooltip>
                      ) : null}
                    </div>
                  </div>

                  {/* 实时动态计算预览行 */}
                  {/* Real-time dynamic calculation preview row */}
                  <div className='flex items-center justify-between gap-2 bg-muted/20 rounded p-1.5 border border-border/30 text-xs'>
                    <div className='flex items-center gap-1.5 text-muted-foreground'>
                      <Clock3 className='size-3 text-sky-500 shrink-0' />
                      <span className='text-[11px]'>{t('currentCalculatedValue')}:</span>
                    </div>
                    <span className='font-mono font-medium text-xs text-sky-600 dark:text-sky-400 select-all'>
                      {previewValue || '-'}
                    </span>
                  </div>

                  {/* 表达式描述说明 */}
                  {/* Expression description */}
                  <p className='text-[11px] text-muted-foreground leading-normal'>
                    {t(item.descKey)}
                  </p>
                </div>
              );
            })
          )}
        </div>
      ) : (
        /* 全局变量卡片列表 */
        /* Global variables list */
        <>
          <div className='space-y-2 pb-1'>
            {filteredVariables.length === 0 ? (
              <div className='flex flex-col items-center justify-center py-8 px-2 text-center rounded-lg border border-dashed border-border/60 bg-muted/10'>
                <KeyRound className='size-8 text-muted-foreground/40 mb-2' />
                <p className='text-xs font-medium text-muted-foreground'>
                  {searchQuery || typeFilter !== 'all'
                    ? t('noDetectedVariables')
                    : t('noGlobalVariables')}
                </p>
                {!searchQuery && typeFilter === 'all' && (
                  <Button
                    variant='outline'
                    size='sm'
                    className='h-7 text-xs mt-3 gap-1'
                    onClick={onOpenCreate}
                  >
                    <Plus className='size-3' />
                    {t('newGlobalVariable')}
                  </Button>
                )}
              </div>
            ) : (
              filteredVariables.map((item) => {
                const isSecret =
                  item.value_type === 'secret' || item.value === '******';
                const canEdit =
                  isAdmin || (Boolean(currentUserId) && item.created_by === currentUserId);

                return (
                  <div
                    key={item.id}
                    className='group relative rounded-lg border border-border/60 bg-card/70 hover:bg-card/95 hover:border-primary/40 hover:shadow-xs transition-all duration-200 p-2.5 space-y-2'
                  >
                    {/* 顶栏：变量键名 + 类型标签 + 快捷操作 */}
                    {/* Header: Variable key + type badge + quick actions */}
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
                              <span className='truncate max-w-[130px]'>
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

                      {/* 操作按钮区 */}
                      {/* Actions */}
                      <div className='flex items-center gap-0.5 shrink-0 opacity-80 group-hover:opacity-100 transition-opacity'>
                        {canEdit ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
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
                        ) : (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className='inline-flex'>
                                <Button
                                  size='icon'
                                  variant='ghost'
                                  disabled
                                  className='size-6 text-muted-foreground/40 cursor-not-allowed'
                                >
                                  <Pencil className='size-3' />
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent side='top' className='text-xs'>
                              {t('onlyCreatorOrAdminCanEdit')}
                            </TooltipContent>
                          </Tooltip>
                        )}

                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              size='icon'
                              variant='ghost'
                              className='size-6 text-muted-foreground hover:text-foreground'
                            >
                              <MoreHorizontal className='size-3' />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align='end' className='text-xs'>
                            <DropdownMenuItem
                              onClick={() => onCopyReference(item.key)}
                            >
                              <Code2 className='mr-2 size-3.5' />
                              {t('copyReference')}
                            </DropdownMenuItem>
                            {!isSecret && (
                              <DropdownMenuItem
                                onClick={() => onCopyValue(item.value)}
                              >
                                <Copy className='mr-2 size-3.5' />
                                {t('copyValue')}
                              </DropdownMenuItem>
                            )}
                            {canEdit ? (
                              <>
                                <DropdownMenuItem
                                  onClick={() => onOpenEdit(item)}
                                >
                                  <Pencil className='mr-2 size-3.5' />
                                  {t('edit')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  className='text-destructive focus:text-destructive'
                                  onClick={() => onDelete(item.id)}
                                >
                                  <Trash2 className='mr-2 size-3.5' />
                                  {t('delete')}
                                </DropdownMenuItem>
                              </>
                            ) : null}
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>

                    {/* 变量值展示区域 */}
                    {/* Value row: Masked for secret, plain text for string */}
                    <div className='flex items-center justify-between gap-2 bg-muted/20 rounded p-1.5 border border-border/30'>
                      {isSecret ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <div className='flex items-center gap-1.5 text-xs text-amber-600/90 dark:text-amber-400/90 font-mono select-none cursor-default truncate'>
                              <ShieldCheck className='size-3.5 shrink-0 text-amber-500' />
                              <span className='tracking-widest'>••••••••</span>
                              <span className='text-[10px] text-muted-foreground font-sans'>
                                ({t('secretMaskedBadge')})
                              </span>
                            </div>
                          </TooltipTrigger>
                          <TooltipContent side='bottom' className='text-xs max-w-[240px]'>
                            {t('secretMaskedLabel')}
                          </TooltipContent>
                        </Tooltip>
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className='font-mono text-xs text-foreground/80 truncate max-w-[170px] text-left'>
                              {item.value || '-'}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent
                            side='bottom'
                            className='max-w-[280px] break-all text-xs font-mono'
                          >
                            {item.value || '-'}
                          </TooltipContent>
                        </Tooltip>
                      )}

                      {!isSecret ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              size='icon'
                              variant='ghost'
                              className='size-5 text-muted-foreground hover:text-foreground shrink-0'
                              onClick={() => onCopyValue(item.value)}
                            >
                              <Copy className='size-2.5' />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent side='left' className='text-xs'>
                            {t('copyValue')}
                          </TooltipContent>
                        </Tooltip>
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              size='icon'
                              variant='ghost'
                              className='size-5 text-muted-foreground/60 hover:text-primary shrink-0'
                              onClick={() => onCopyReference(item.key)}
                            >
                              <Code2 className='size-2.5' />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent side='left' className='text-xs'>
                            {t('copyReference')}
                          </TooltipContent>
                        </Tooltip>
                      )}
                    </div>

                    {/* 描述说明 */}
                    {/* Description */}
                    {item.description ? (
                      <p className='text-[11px] text-muted-foreground leading-tight truncate'>
                        {item.description}
                      </p>
                    ) : null}
                  </div>
                );
              })
            )}
          </div>

          {/* 分页 */}
          {/* Pagination */}
          <SimplePagination
            total={total}
            page={page}
            pageSize={pageSize}
            onPageChange={onPageChange}
          />
        </>
      )}
    </div>
  );
}
