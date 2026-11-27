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
  Copy,
  Eye,
  HardDrive,
  MoreHorizontal,
} from 'lucide-react';
import {toast} from '@/lib/toast';

import {Button} from '@/components/arc/button/button';
import {DropdownMenu, type DropdownItem} from '@/components/arc/dropdown-menu/dropdown-menu';
import {SearchField} from '@/components/arc/search-field/search-field';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  SoftChip,
  TableLoadingBar,
  TableSkeletonRows,
  TruncatedText,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {Pagination} from '@/components/ui/pagination';
import {cn} from '@/lib/utils';
import type {
  RuntimeStorageListItem,
  RuntimeStorageListResult,
  RuntimeStorageSpec,
} from '@/lib/services/cluster/types';
import {writeClipboard} from '@/components/arc/lib/use-copy-feedback';
import {CopyButton} from '@/components/arc/copy-button/copy-button';

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
  /** 嵌入单层扁平画布时去掉外框 / Remove outer card border when embedded in a flat single-layer surface */
  flat?: boolean;
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
  flat = false,
}: RuntimeStorageFileExplorerProps) {
  const t = useTranslations('cluster.runtimeStorage');
  const tCommon = useTranslations('common');

  const [searchKeyword, setSearchKeyword] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 20;

  const effectiveCurrentPath = currentPath || spec.namespace || '';

  // 构造面包屑链 / Build breadcrumbs chain
  const breadcrumbs = useMemo(() => {
    return buildStorageBreadcrumbs(effectiveCurrentPath, spec.namespace);
  }, [effectiveCurrentPath, spec.namespace]);

  const isAtRoot = breadcrumbs.length <= 1;

  // 上一级路径 / Parent path
  const parentPath = useMemo(() => {
    if (isAtRoot) {return spec.namespace || '';}
    return breadcrumbs[breadcrumbs.length - 2]?.path || spec.namespace || '';
  }, [breadcrumbs, isAtRoot, spec.namespace]);

  // 复制指定项路径 / Copy item path
  const handleCopyItemPath = useCallback(
    (path: string, e?: React.MouseEvent) => {
      e?.stopPropagation();
      void writeClipboard(path);
      toast.success(t('copyPathSuccess'));
    },
    [t],
  );

  // 复制指定项文件名 / Copy item name
  const handleCopyItemName = useCallback(
    (name: string, e?: React.MouseEvent) => {
      e?.stopPropagation();
      void writeClipboard(name);
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
      if (!trimmed) {return true;}
      const haystack = `${item.name || ''} ${item.path || ''}`.toLowerCase();
      return haystack.includes(trimmed);
    });

    return [...filtered].sort((a, b) => {
      // 1. 文件夹置顶 / Directories first
      if (a.directory && !b.directory) {return -1;}
      if (!a.directory && b.directory) {return 1;}

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
    <div
      className={cn(
        'relative overflow-hidden',
        flat
          ? 'bg-transparent'
          : 'rounded-(--radius-panel) border border-(--border) bg-card',
      )}
    >
      <TableLoadingBar loading={loading && Boolean(listing)} />

      {/* 顶部现代化资源管理器地址栏与工具栏 */}
      {/* Modern OS-style address bar & action toolbar */}
      <div
        className={cn(
          'flex flex-col gap-2.5 border-b-[0.5px] border-border/80 sm:flex-row sm:items-center sm:justify-between',
          flat ? 'pb-3' : 'bg-muted/20 p-3',
        )}
      >
        {/* 地址栏与快捷层级跳转 */}
        {/* Address bar with clickable breadcrumbs & level navigation */}
        <div className='flex min-w-0 flex-1 items-center gap-1.5'>
          {/* 返回上一级 / Go up */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                iconOnly
                disabled={loading || isAtRoot}
                onClick={() => onNavigate(parentPath)}
                aria-label={t('upLevel')}
              >
                <ChevronUp className='size-4' aria-hidden />
              </Button>
            </TooltipTrigger>
            <TooltipContent side='bottom'>{t('upLevel')}</TooltipContent>
          </Tooltip>

          {/* 回到根目录 / Go root */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                iconOnly
                disabled={loading || isAtRoot}
                onClick={() => onNavigate(spec.namespace || '')}
                aria-label={t('rootDir')}
              >
                <Home className='size-3.5' aria-hidden />
              </Button>
            </TooltipTrigger>
            <TooltipContent side='bottom'>{t('rootDir')}</TooltipContent>
          </Tooltip>

          {/* 可点击面包屑路径条 / Interactive breadcrumb bar */}
          <div className='flex min-w-0 flex-1 items-center gap-1 overflow-x-auto rounded-lg border border-border/60 bg-background/80 px-2.5 py-1 text-xs'>
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
                <span className='ml-auto shrink-0 inline-flex'>
                  <CopyButton
                    value={effectiveCurrentPath}
                    label={t('copyPath')}
                    variant='plain'
                    iconOnly
                    disabled={!effectiveCurrentPath}
                  />
                </span>
              </TooltipTrigger>
              <TooltipContent side='top'>{t('copyPath')}</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* 右侧搜索与刷新 / Search & Refresh controls */}
        <div className='flex items-center gap-2 shrink-0'>
          <div className={cn(layout.toolbarSearch, 'w-full sm:w-56')}>
            <SearchField
              label={t('searchEntries')}
              value={searchKeyword}
              onValueChange={(next) => {
                setSearchKeyword(next);
                setCurrentPage(1);
              }}
              placeholder={t('searchEntries')}
              clearLabel={tCommon('clearFilters')}
            />
          </div>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                iconOnly
                disabled={loading}
                data-testid={`runtime-storage-explorer-${kind}-refresh`}
                onClick={onRefresh}
                aria-label={tCommon('refresh')}
              >
                <RefreshCw
                  className={cn('size-3.5', loading && 'animate-spin')}
                  aria-hidden
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

        <TableBody className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
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
                  data-testid={`runtime-storage-explorer-${kind}-row`}
                  data-item-name={itemName}
                  data-item-path={itemPath}
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

                      <TruncatedText
                        text={itemName}
                        tooltip={
                          <span className='font-mono text-xs'>{itemPath}</span>
                        }
                        alwaysTooltip={itemPath !== itemName}
                        mono={!item.directory}
                        className='min-w-0 flex-1'
                        textClassName={cn(
                          'text-xs',
                          item.directory
                            ? 'font-medium text-foreground group-hover:text-primary transition-colors'
                            : 'text-foreground/90',
                        )}
                      />

                      {/* 专属类型小胶囊 / Exclusive type badge */}
                      {isSerCheckpoint ? (
                        <SoftChip tone='blue' className='font-mono'>
                          CKPT
                        </SoftChip>
                      ) : isImapWal ? (
                        <SoftChip tone='emerald' className='font-mono'>
                          WAL
                        </SoftChip>
                      ) : null}
                    </div>
                  </TableCell>

                  {/* 大小列 / Size */}
                  <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono tabular-nums'>
                    {item.directory
                      ? item.size_bytes && item.size_bytes > 0
                        ? `${t('directory')} · ${formatBytes(item.size_bytes)}`
                        : t('directory')
                      : formatBytes(item.size_bytes || 0)}
                  </TableCell>

                  {/* 修改时间列 / Modified At */}
                  <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono tabular-nums'>
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
                          type='button'
                          variant='ghost'
                          size='sm'
                          data-testid='runtime-storage-explorer-open-dir'
                          onClick={() => onNavigate(itemPath)}
                        >
                          {t('open')}
                          <ChevronRight className='size-3.5' aria-hidden />
                        </Button>
                      ) : isSerCheckpoint ? (
                        <>
                          <Button
                            type='button'
                            variant='secondary'
                            size='sm'
                            data-testid='runtime-storage-explorer-inspect-checkpoint'
                            loading={isInspecting}
                            onClick={() => onInspectCheckpoint?.(itemPath)}
                          >
                            <Database className='size-3.5' aria-hidden />
                            {t('deserializeCheckpoint')}
                          </Button>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                iconOnly
                                data-testid='runtime-storage-explorer-preview'
                                loading={isPreviewing}
                                onClick={() => onPreviewFile(itemPath)}
                                aria-label={t('preview')}
                              >
                                <Eye className='size-3.5' aria-hidden />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent side='top'>{t('preview')}</TooltipContent>
                          </Tooltip>
                        </>
                      ) : isImapWal ? (
                        <>
                          <Button
                            type='button'
                            variant='secondary'
                            size='sm'
                            data-testid='runtime-storage-explorer-inspect-imap'
                            loading={isInspectingWal}
                            onClick={() => onInspectIMAPWAL?.(itemPath)}
                          >
                            <Database className='size-3.5' aria-hidden />
                            {t('inspectWal')}
                          </Button>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                iconOnly
                                data-testid='runtime-storage-explorer-preview'
                                loading={isPreviewing}
                                onClick={() => onPreviewFile(itemPath)}
                                aria-label={t('preview')}
                              >
                                <Eye className='size-3.5' aria-hidden />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent side='top'>{t('preview')}</TooltipContent>
                          </Tooltip>
                        </>
                      ) : (
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          data-testid='runtime-storage-explorer-preview'
                          loading={isPreviewing}
                          onClick={() => onPreviewFile(itemPath)}
                        >
                          <Eye className='size-3.5' aria-hidden />
                          {t('preview')}
                        </Button>
                      )}

                      {/* 更多功能下拉菜单 / More options dropdown */}
                      <DropdownMenu
                        trigger={
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            iconOnly
                            aria-label={tCommon('actions')}
                          >
                            <MoreHorizontal className='size-3.5' aria-hidden />
                          </Button>
                        }
                        items={
                          [
                            {
                              label: t('copyPath'),
                              icon: <Copy className='size-3.5' />,
                              onSelect: () => handleCopyItemPath(itemPath),
                            },
                            {
                              label: t('copyName'),
                              icon: <Copy className='size-3.5' />,
                              onSelect: () => handleCopyItemName(itemName),
                            },
                            ...(!item.directory
                              ? [
                                  {
                                    label: t('rawHexPreview'),
                                    icon: <Eye className='size-3.5' />,
                                    disabled: isPreviewing,
                                    separatorBefore: true,
                                    onSelect: () => onPreviewFile(itemPath),
                                  },
                                ]
                              : []),
                          ] satisfies DropdownItem[]
                        }
                      />
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
