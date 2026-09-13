// Copyright (c) 2026 SeaTunnelX
// 全局变量工作台侧边栏面板组件
// Global variables sidebar panel component for workbench studio

'use client';

import React, {useState, useMemo} from 'react';
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
} from 'lucide-react';
import {cn} from '@/lib/utils';

export interface GlobalVariablesSidebarPanelProps {
  variables: SyncGlobalVariable[];
  total: number;
  page: number;
  pageSize: number;
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
  onPageChange,
  onOpenCreate,
  onOpenEdit,
  onDelete,
  onCopyValue,
  onCopyReference,
}: GlobalVariablesSidebarPanelProps) {
  const t = useTranslations('workbenchStudio');

  // 搜索关键字与类型过滤
  // Search query and type filter
  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState<'all' | 'string' | 'secret'>(
    'all',
  );

  // 本地过滤列表
  // Filtered variables
  const filteredVariables = useMemo(() => {
    return variables.filter((item) => {
      const isSecret =
        item.value_type === 'secret' || item.value === '******';
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

  return (
    <div className='min-w-0 w-full space-y-3'>
      {/* 顶部标题与操作 */}
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
              {total}
            </Badge>
          </div>
          <Button
            size='sm'
            className='h-7 px-2.5 text-xs gap-1 shadow-xs'
            onClick={onOpenCreate}
          >
            <Plus className='size-3.5' />
            <span>{t('newCreate')}</span>
          </Button>
        </div>

        <p className='text-[11px] leading-relaxed text-muted-foreground'>
          {t('globalVariablesDesc')}
        </p>

        {/* 快捷搜索栏 */}
        {/* Search input */}
        <div className='relative'>
          <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground' />
          <Input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('searchVariables')}
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

        {/* 类型切换标签 */}
        {/* Type filter tabs */}
        <div className='flex items-center gap-1 p-0.5 rounded-md bg-muted/30 border border-border/40 text-[11px]'>
          <button
            type='button'
            onClick={() => setTypeFilter('all')}
            className={cn(
              'flex-1 py-1 rounded transition-colors text-center font-medium',
              typeFilter === 'all'
                ? 'bg-background text-foreground shadow-2xs'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t('filterAll')}
          </button>
          <button
            type='button'
            onClick={() => setTypeFilter('string')}
            className={cn(
              'flex-1 py-1 rounded transition-colors text-center font-medium flex items-center justify-center gap-1',
              typeFilter === 'string'
                ? 'bg-background text-foreground shadow-2xs'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <FileText className='size-2.5' />
            {t('filterText')}
          </button>
          <button
            type='button'
            onClick={() => setTypeFilter('secret')}
            className={cn(
              'flex-1 py-1 rounded transition-colors text-center font-medium flex items-center justify-center gap-1',
              typeFilter === 'secret'
                ? 'bg-background text-amber-500 shadow-2xs'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Lock className='size-2.5' />
            {t('filterSecret')}
          </button>
        </div>
      </div>

      {/* 变量列表 */}
      {/* Variable cards list */}
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
    </div>
  );
}
