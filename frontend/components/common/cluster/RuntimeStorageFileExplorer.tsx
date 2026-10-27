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

import React, {useCallback, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  Folder,
  File,
  FileText,
  FileCode,
  Database,
  ChevronRight,
  ChevronUp,
  Home,
  RefreshCw,
  Search,
  X,
  Copy,
  Check,
  Eye,
  Loader2,
  HardDrive,
  MoreHorizontal,
} from 'lucide-react';
import {toast} from 'sonner';

import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Badge} from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {TableLoadingBar} from '@/components/common/layout/TableLoadingBar';
import {TableSkeletonRows} from '@/components/common/layout/TableSkeletonRows';
import {Pagination} from '@/components/ui/pagination';
import {cn} from '@/lib/utils';
import type {
  RuntimeStorageListItem,
  RuntimeStorageListResult,
  RuntimeStorageSpec,
} from '@/lib/services/cluster/types';

/**
 * 格式化字节大小
 * Format byte count into human-readable string
 */
function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) {
    return '-';
  }
  if (bytes === 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const index = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export interface BreadcrumbSegment {
  name: string;
  path: string;
  isRoot: boolean;
  isCurrent: boolean;
}

/**
 * 构造类似操作系统资源管理器的层级可点击面包屑
 * Construct hierarchical clickable breadcrumbs mimicking desktop OS file explorers
 */
export function buildStorageBreadcrumbs(
  currentPath?: string,
  rootPath?: string,
): BreadcrumbSegment[] {
  const normCurrent = (currentPath || '').trim();
  const normRoot = (rootPath || '').trim();

  if (!normCurrent && !normRoot) {
    return [{name: 'Root', path: '', isRoot: true, isCurrent: true}];
  }

  const effectiveRoot = normRoot || normCurrent;
  const schemeMatch = effectiveRoot.match(/^([a-zA-Z0-9_+-]+:(?:\/\/)?)?(.*)$/);
  const scheme = schemeMatch?.[1] || '';
  const rootBody = (schemeMatch?.[2] || effectiveRoot).replace(/\/+$/, '');
  const rootBasename = rootBody.split('/').filter(Boolean).pop() || rootBody || 'Root';

  const currentSchemeMatch = normCurrent.match(/^([a-zA-Z0-9_+-]+:(?:\/\/)?)?(.*)$/);
  const currentBody = (currentSchemeMatch?.[2] || normCurrent).replace(/\/+$/, '');

  // 若处于根目录 / If at root level
  if (!normCurrent || currentBody === rootBody) {
    return [{name: rootBasename, path: effectiveRoot, isRoot: true, isCurrent: true}];
  }

  const segments: BreadcrumbSegment[] = [
    {name: rootBasename, path: effectiveRoot, isRoot: true, isCurrent: false},
  ];

  if (currentBody.startsWith(rootBody)) {
    const subPath = currentBody.slice(rootBody.length).replace(/^\/+/, '');
    const parts = subPath.split('/').filter(Boolean);
    let accum = effectiveRoot.replace(/\/+$/, '');
    parts.forEach((part, index) => {
      accum = `${accum}/${part}`;
      segments.push({
        name: part,
        path: accum,
        isRoot: false,
        isCurrent: index === parts.length - 1,
      });
    });
  } else {
    const parts = currentBody.split('/').filter(Boolean);
    let accum = scheme
      ? scheme + (currentBody.startsWith('/') ? '' : '/')
      : currentBody.startsWith('/')
        ? '/'
        : '';
    parts.forEach((part, index) => {
      accum = accum === '/' || accum.endsWith('/') ? `${accum}${part}` : `${accum}/${part}`;
      segments.push({
        name: part,
        path: accum,
        isRoot: index === 0,
        isCurrent: index === parts.length - 1,
      });
    });
  }

  return segments;
}

export interface RuntimeStorageFileExplorerProps {
  clusterId: number;
  kind: 'checkpoint' | 'imap';
  spec: RuntimeStorageSpec;
  listing?: RuntimeStorageListResult;
  loading?: boolean;
  currentPath?: string;
  onNavigate: (path: string) => void;
  onRefresh: () => void;
  onPreviewFile: (path: string) => void;
  previewLoadingKey?: string | null;
  onInspectCheckpoint?: (path: string) => void;
  inspectCheckpointLoadingPath?: string | null;
  onInspectIMAPWAL?: (path: string) => void;
  inspectIMAPWALLoadingPath?: string | null;
}

/**
 * 运行时存储文件与目录资源管理器组件
 * Desktop-like file explorer for runtime storage inspection & navigation
 */
export function RuntimeStorageFileExplorer({
  kind,
  spec,
  listing,
  loading = false,
  currentPath,
  onNavigate,
  onRefresh,
  onPreviewFile,
  previewLoadingKey,
  onInspectCheckpoint,
  inspectCheckpointLoadingPath,
  onInspectIMAPWAL,
  inspectIMAPWALLoadingPath,
}: RuntimeStorageFileExplorerProps) {
  const t = useTranslations('cluster.runtimeStorage');
  const tCommon = useTranslations('common');

  const [searchKeyword, setSearchKeyword] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [copied, setCopied] = useState(false);
  const pageSize = 20;

  const effectiveCurrentPath = currentPath || spec.namespace || '';

  // 构造面包屑链 / Build breadcrumbs chain
  const breadcrumbs = useMemo(() => {
    return buildStorageBreadcrumbs(effectiveCurrentPath, spec.namespace);
  }, [effectiveCurrentPath, spec.namespace]);

  const isAtRoot = breadcrumbs.length <= 1;

  // 上一级路径 / Parent path
  const parentPath = useMemo(() => {
    if (isAtRoot) return spec.namespace || '';
    return breadcrumbs[breadcrumbs.length - 2]?.path || spec.namespace || '';
  }, [breadcrumbs, isAtRoot, spec.namespace]);

  // 复制当前完整路径 / Copy current full path
  const handleCopyCurrentPath = useCallback(() => {
    if (!effectiveCurrentPath) return;
    void navigator.clipboard.writeText(effectiveCurrentPath);
    setCopied(true);
    toast.success(t('copyPathSuccess'));
    setTimeout(() => setCopied(false), 2000);
  }, [effectiveCurrentPath, t]);

  // 复制指定项路径 / Copy item path
  const handleCopyItemPath = useCallback(
    (path: string, e?: React.MouseEvent) => {
      e?.stopPropagation();
      void navigator.clipboard.writeText(path);
      toast.success(t('copyPathSuccess'));
    },
    [t],
  );

  // 复制指定项文件名 / Copy item name
  const handleCopyItemName = useCallback(
    (name: string, e?: React.MouseEvent) => {
      e?.stopPropagation();
      void navigator.clipboard.writeText(name);
      toast.success(t('copyContentSuccess'));
    },
    [t],
  );

  // 过滤项并进行“文件夹置顶 + 自然排序”
  // Filter items with "directories first + natural alphanumeric sorting"
  const rawItems = listing?.items || [];
  const filteredAndSortedItems = useMemo(() => {
    const trimmed = searchKeyword.trim().toLowerCase();
    const filtered = rawItems.filter((item) => {
      if (!trimmed) return true;
      const haystack = `${item.name || ''} ${item.path || ''}`.toLowerCase();
      return haystack.includes(trimmed);
    });

    return [...filtered].sort((a, b) => {
      // 1. 文件夹置顶 / Directories first
      if (a.directory && !b.directory) return -1;
      if (!a.directory && b.directory) return 1;

      // 2. 按名称自然数排序 / Natural alphanumeric order by name
      const nameA = a.name || a.path || '';
      const nameB = b.name || b.path || '';
      return nameA.localeCompare(nameB, undefined, {
        numeric: true,
        sensitivity: 'base',
      });
    });
  }, [rawItems, searchKeyword]);

  // 统计信息 / Metrics summary
  const dirCount = useMemo(() => {
    return rawItems.filter((item) => item.directory).length;
  }, [rawItems]);
  const fileCount = useMemo(() => {
    return rawItems.filter((item) => !item.directory).length;
  }, [rawItems]);

  // 分页计算 / Pagination calculations
  const totalItems = filteredAndSortedItems.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const normalizedPage = Math.min(Math.max(1, currentPage), totalPages);
  const pagedItems = useMemo(() => {
    const start = (normalizedPage - 1) * pageSize;
    return filteredAndSortedItems.slice(start, start + pageSize);
  }, [filteredAndSortedItems, normalizedPage, pageSize]);

  // 处理整行或直接点击：根据文件类型直达操作
  // Row click dispatcher: navigates directory or triggers primary inspect/preview
  const handleItemClick = useCallback(
    (item: RuntimeStorageListItem) => {
      const itemPath = item.path || item.name || '';
      if (item.directory) {
        onNavigate(itemPath);
        return;
      }
      if (kind === 'checkpoint' && itemPath.endsWith('.ser') && onInspectCheckpoint) {
        onInspectCheckpoint(itemPath);
        return;
      }
      if (kind === 'imap' && itemPath.endsWith('_wal.txt') && onInspectIMAPWAL) {
        onInspectIMAPWAL(itemPath);
        return;
      }
      onPreviewFile(itemPath);
    },
    [kind, onNavigate, onInspectCheckpoint, onInspectIMAPWAL, onPreviewFile],
  );

  return (
    <div className='relative overflow-hidden rounded-xl border border-border/60 bg-card/40 shadow-xs'>
      <TableLoadingBar loading={loading && Boolean(listing)} />

      {/* 顶部现代化资源管理器地址栏与工具栏 */}
      {/* Modern OS-style address bar & action toolbar */}
      <div className='flex flex-col gap-2.5 border-b border-border/50 bg-muted/20 p-3 sm:flex-row sm:items-center sm:justify-between'>
        {/* 地址栏与快捷层级跳转 */}
        {/* Address bar with clickable breadcrumbs & level navigation */}
        <div className='flex min-w-0 flex-1 items-center gap-1.5'>
          {/* 返回上一级 / Go up */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant='outline'
                size='icon'
                className='size-8 shrink-0 active:scale-95'
                disabled={loading || isAtRoot}
                onClick={() => onNavigate(parentPath)}
                aria-label={t('upLevel')}
              >
                <ChevronUp className='size-4' />
              </Button>
            </TooltipTrigger>
            <TooltipContent side='bottom'>{t('upLevel')}</TooltipContent>
          </Tooltip>

          {/* 回到根目录 / Go root */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant='outline'
                size='icon'
                className='size-8 shrink-0 active:scale-95'
                disabled={loading || isAtRoot}
                onClick={() => onNavigate(spec.namespace || '')}
                aria-label={t('rootDir')}
              >
                <Home className='size-3.5' />
              </Button>
            </TooltipTrigger>
            <TooltipContent side='bottom'>{t('rootDir')}</TooltipContent>
          </Tooltip>

          {/* 可点击面包屑路径条 / Interactive breadcrumb bar */}
          <div className='flex min-w-0 flex-1 items-center gap-1 overflow-x-auto rounded-lg border border-border/60 bg-background/80 px-2.5 py-1 text-xs shadow-xs'>
            <HardDrive className='size-3.5 shrink-0 text-muted-foreground mr-0.5' />
            {breadcrumbs.map((seg, idx) => (
              <React.Fragment key={`crumb-${seg.path || idx}`}>
                {idx > 0 && (
                  <ChevronRight className='size-3 shrink-0 text-muted-foreground/40' />
                )}
                {seg.isCurrent ? (
                  <span
                    className='shrink-0 font-mono font-semibold text-foreground truncate max-w-[220px]'
                    title={seg.path}
                  >
                    {seg.name}
                  </span>
                ) : (
                  <button
                    type='button'
                    className='shrink-0 font-mono text-muted-foreground hover:text-primary hover:underline transition-colors max-w-[180px] truncate'
                    title={seg.path}
                    onClick={() => onNavigate(seg.path)}
                  >
                    {seg.name}
                  </button>
                )}
              </React.Fragment>
            ))}

            {/* 复制当前路径按钮 / Copy current path */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant='ghost'
                  size='icon'
                  className='ml-auto size-6 shrink-0 text-muted-foreground hover:text-foreground'
                  onClick={handleCopyCurrentPath}
                  aria-label={t('copyPath')}
                >
                  {copied ? (
                    <Check className='size-3.5 text-emerald-500' />
                  ) : (
                    <Copy className='size-3.5' />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side='top'>{t('copyPath')}</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* 右侧搜索与刷新 / Search & Refresh controls */}
        <div className='flex items-center gap-2 shrink-0'>
          <div className='relative w-full sm:w-56'>
            <Search className='pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground' />
            <Input
              value={searchKeyword}
              onChange={(e) => {
                setSearchKeyword(e.target.value);
                setCurrentPage(1);
              }}
              className='h-8 pl-8 pr-7 text-xs bg-background/80'
              placeholder={t('searchEntries')}
            />
            {searchKeyword && (
              <button
                type='button'
                className='absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                onClick={() => {
                  setSearchKeyword('');
                  setCurrentPage(1);
                }}
              >
                <X className='size-3' />
              </button>
            )}
          </div>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant='outline'
                size='icon'
                className='size-8 shrink-0 active:scale-95'
                disabled={loading}
                onClick={onRefresh}
                aria-label={tCommon('refresh')}
              >
                <RefreshCw
                  className={cn('size-3.5', loading && 'animate-spin')}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent side='bottom'>{tCommon('refresh')}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {/* 数据表格区 / Table display */}
      <Table>
        <TableHeader>
          <TableRow className='bg-muted/10 hover:bg-muted/10'>
            <TableHead className='py-2 px-3 text-xs font-semibold'>
              {t('fileName')}
            </TableHead>
            <TableHead className='py-2 px-3 text-xs font-semibold w-[120px]'>
              {t('size')}
            </TableHead>
            <TableHead className='py-2 px-3 text-xs font-semibold w-[200px]'>
              {t('modifiedAt')}
            </TableHead>
            <TableHead className='py-2 px-3 text-xs font-semibold text-right w-[160px]'>
              {tCommon('actions')}
            </TableHead>
          </TableRow>
        </TableHeader>

        <TableBody
          className={
            loading && listing
              ? 'pointer-events-none opacity-60 transition-opacity duration-200'
              : undefined
          }
        >
          {loading && !listing ? (
            <TableSkeletonRows columns={4} rows={5} />
          ) : filteredAndSortedItems.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={4}
                className='py-12 text-center text-xs text-muted-foreground'
              >
                {searchKeyword ? t('noFilteredEntries') : t('noEntries')}
              </TableCell>
            </TableRow>
          ) : (
            pagedItems.map((item) => {
              const itemPath = item.path || item.name || '';
              const itemName = item.name || itemPath.split('/').pop() || itemPath;
              const previewKey = `${kind}:${itemPath}`;
              const isPreviewing = previewLoadingKey === previewKey;
              const isInspecting = inspectCheckpointLoadingPath === itemPath;
              const isInspectingWal = inspectIMAPWALLoadingPath === itemPath;

              const isSerCheckpoint = kind === 'checkpoint' && itemPath.endsWith('.ser');
              const isImapWal = kind === 'imap' && itemPath.endsWith('_wal.txt');
              const isCodeFile =
                itemPath.endsWith('.conf') ||
                itemPath.endsWith('.json') ||
                itemPath.endsWith('.properties') ||
                itemPath.endsWith('.xml') ||
                itemPath.endsWith('.yaml') ||
                itemPath.endsWith('.yml');

              return (
                <TableRow
                  key={`${kind}-item-${itemPath}`}
                  className='group cursor-pointer border-border/40 hover:bg-muted/50 transition-colors'
                  onClick={() => handleItemClick(item)}
                >
                  {/* 名称与图标列 / Name and icon */}
                  <TableCell className='py-2 px-3'>
                    <div className='flex items-center gap-2.5 min-w-0'>
                      {/* 图标渲染 / Icon rendering */}
                      {item.directory ? (
                        <Folder className='size-4 text-amber-500 fill-amber-500/20 shrink-0' />
                      ) : isSerCheckpoint ? (
                        <Database className='size-4 text-purple-400 shrink-0' />
                      ) : isImapWal ? (
                        <FileText className='size-4 text-emerald-400 shrink-0' />
                      ) : isCodeFile ? (
                        <FileCode className='size-4 text-sky-400 shrink-0' />
                      ) : (
                        <File className='size-4 text-muted-foreground shrink-0' />
                      )}

                      <span
                        className={cn(
                          'truncate text-xs',
                          item.directory
                            ? 'font-medium text-foreground group-hover:text-primary transition-colors'
                            : 'font-mono text-foreground/90',
                        )}
                        title={itemPath}
                      >
                        {itemName}
                      </span>

                      {/* 专属类型小胶囊 / Exclusive type badge */}
                      {isSerCheckpoint ? (
                        <Badge
                          variant='outline'
                          className='h-4 px-1 text-[9px] font-mono border-purple-500/30 text-purple-400 bg-purple-500/10 shrink-0'
                        >
                          CKPT
                        </Badge>
                      ) : isImapWal ? (
                        <Badge
                          variant='outline'
                          className='h-4 px-1 text-[9px] font-mono border-emerald-500/30 text-emerald-400 bg-emerald-500/10 shrink-0'
                        >
                          WAL
                        </Badge>
                      ) : null}
                    </div>
                  </TableCell>

                  {/* 大小列 / Size */}
                  <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono'>
                    {item.directory
                      ? item.size_bytes && item.size_bytes > 0
                        ? `${t('directory')} · ${formatBytes(item.size_bytes)}`
                        : t('directory')
                      : formatBytes(item.size_bytes || 0)}
                  </TableCell>

                  {/* 修改时间列 / Modified At */}
                  <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono'>
                    {item.modified_at || '-'}
                  </TableCell>

                  {/* 操作列：直出快捷按钮 + 更多菜单 */}
                  {/* Actions column: direct shortcut buttons + dropdown menu */}
                  <TableCell className='py-2 px-3 text-right whitespace-nowrap'>
                    <div
                      className='flex items-center justify-end gap-1'
                      onClick={(e) => e.stopPropagation()}
                    >
                      {item.directory ? (
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground active:scale-95'
                          onClick={() => onNavigate(itemPath)}
                        >
                          {t('open')}
                          <ChevronRight className='size-3.5 ml-0.5' />
                        </Button>
                      ) : isSerCheckpoint ? (
                        <>
                          <Button
                            variant='outline'
                            size='sm'
                            className='h-7 px-2 text-xs border-purple-500/30 text-purple-400 hover:text-purple-300 hover:bg-purple-500/10 active:scale-95'
                            disabled={isInspecting}
                            onClick={() => onInspectCheckpoint?.(itemPath)}
                          >
                            {isInspecting ? (
                              <Loader2 className='mr-1 size-3 animate-spin' />
                            ) : (
                              <Database className='mr-1 size-3' />
                            )}
                            {t('deserializeCheckpoint')}
                          </Button>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='size-7 text-muted-foreground hover:text-foreground active:scale-95'
                                disabled={isPreviewing}
                                onClick={() => onPreviewFile(itemPath)}
                                aria-label={t('preview')}
                              >
                                {isPreviewing ? (
                                  <Loader2 className='size-3.5 animate-spin' />
                                ) : (
                                  <Eye className='size-3.5' />
                                )}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent side='top'>{t('preview')}</TooltipContent>
                          </Tooltip>
                        </>
                      ) : isImapWal ? (
                        <>
                          <Button
                            variant='outline'
                            size='sm'
                            className='h-7 px-2 text-xs border-emerald-500/30 text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10 active:scale-95'
                            disabled={isInspectingWal}
                            onClick={() => onInspectIMAPWAL?.(itemPath)}
                          >
                            {isInspectingWal ? (
                              <Loader2 className='mr-1 size-3 animate-spin' />
                            ) : (
                              <Database className='mr-1 size-3' />
                            )}
                            {t('inspectWal')}
                          </Button>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='size-7 text-muted-foreground hover:text-foreground active:scale-95'
                                disabled={isPreviewing}
                                onClick={() => onPreviewFile(itemPath)}
                                aria-label={t('preview')}
                              >
                                {isPreviewing ? (
                                  <Loader2 className='size-3.5 animate-spin' />
                                ) : (
                                  <Eye className='size-3.5' />
                                )}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent side='top'>{t('preview')}</TooltipContent>
                          </Tooltip>
                        </>
                      ) : (
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground active:scale-95'
                          disabled={isPreviewing}
                          onClick={() => onPreviewFile(itemPath)}
                        >
                          {isPreviewing ? (
                            <Loader2 className='mr-1 size-3.5 animate-spin' />
                          ) : (
                            <Eye className='mr-1 size-3.5' />
                          )}
                          {t('preview')}
                        </Button>
                      )}

                      {/* 更多功能下拉菜单 / More options dropdown */}
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant='ghost'
                            size='icon'
                            className='size-7 text-muted-foreground hover:text-foreground'
                            aria-label={tCommon('actions')}
                          >
                            <MoreHorizontal className='size-3.5' />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align='end' className='w-44'>
                          <DropdownMenuItem
                            onClick={(e) => handleCopyItemPath(itemPath, e)}
                          >
                            <Copy className='mr-2 size-3.5' />
                            {t('copyPath')}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={(e) => handleCopyItemName(itemName, e)}
                          >
                            <Copy className='mr-2 size-3.5' />
                            {t('copyName')}
                          </DropdownMenuItem>
                          {!item.directory && (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                disabled={isPreviewing}
                                onClick={() => onPreviewFile(itemPath)}
                              >
                                <Eye className='mr-2 size-3.5' />
                                {t('rawHexPreview')}
                              </DropdownMenuItem>
                            </>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>

      {/* 底部状态信息与分页栏 / Footer status summary & pagination */}
      <div className='flex flex-wrap items-center justify-between gap-3 border-t border-border/50 bg-muted/10 px-3 py-2'>
        <div className='text-xs text-muted-foreground'>
          {searchKeyword
            ? t('filteredCount', {
                filtered: filteredAndSortedItems.length,
                total: rawItems.length,
              })
            : t('summaryCount', {
                total: rawItems.length,
                dirs: dirCount,
                files: fileCount,
              })}
        </div>

        {totalItems > 0 && totalPages > 1 && (
          <div className='min-w-0'>
            <Pagination
              currentPage={normalizedPage}
              totalPages={totalPages}
              pageSize={pageSize}
              totalItems={totalItems}
              showPageSizeSelector={false}
              onPageChange={setCurrentPage}
            />
          </div>
        )}
      </div>
    </div>
  );
}
