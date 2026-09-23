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

import React, { useMemo, useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';
import {
  FileCode2,
  Database,
  Braces,
  FileText,
  File,
  Code2,
  SquareTerminal,
  Folder,
  PlayCircle,
  FilePlus2,
  FolderPlus,
  ArrowRight,
  Activity,
} from 'lucide-react';
import {
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
} from '@/components/ui/command';
import { Badge } from '@/components/ui/badge';
import {
  SyncTaskTreeNode,
  flattenTree,
  getNodeBreadcrumbSegments,
} from './sync-studio-utils';
import { cn } from '@/lib/utils';

// ============================================================================
// 文件图标映射 / File Icon Mapping
// ============================================================================

function getFileIcon(fileName: string) {
  const ext = fileName.includes('.')
    ? `.${fileName.split('.').pop()?.toLowerCase()}`
    : '';

  switch (ext) {
    case '.conf':
    case '.hocon':
      return { icon: FileCode2, color: 'text-sky-500 dark:text-sky-400' };
    case '.json':
      return { icon: Braces, color: 'text-amber-500 dark:text-amber-400' };
    case '.sql':
      return { icon: Database, color: 'text-violet-500 dark:text-violet-400' };
    case '.yaml':
    case '.yml':
      return { icon: FileText, color: 'text-emerald-500 dark:text-emerald-400' };
    case '.xml':
      return { icon: Code2, color: 'text-orange-500 dark:text-orange-400' };
    case '.sh':
      return { icon: SquareTerminal, color: 'text-green-500 dark:text-green-400' };
    default:
      return { icon: File, color: 'text-muted-foreground' };
  }
}

// ============================================================================
// 类型定义 / Type Definitions
// ============================================================================

export interface StudioQuickOpenJobItem {
  id: number;
  task_id: number;
  platform_job_id?: string;
  status?: string;
  task_name?: string;
}

export interface StudioQuickOpenDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tree: SyncTaskTreeNode[];
  recentJobs?: StudioQuickOpenJobItem[];
  onSelectFile: (node: SyncTaskTreeNode) => void | Promise<void>;
  onSelectJob: (jobId: string) => void | Promise<boolean | void>;
  onNewFile?: () => void;
  onNewFolder?: () => void;
}

// ============================================================================
// 组件实现：VSCode 风格 Quick Open 多结果浮层
// Component Implementation: VSCode-style Quick Open Multi-Result Palette
// ============================================================================

export function StudioQuickOpenDialog({
  open,
  onOpenChange,
  tree,
  recentJobs = [],
  onSelectFile,
  onSelectJob,
  onNewFile,
  onNewFolder,
}: StudioQuickOpenDialogProps) {
  const t = useTranslations('workbenchStudio');
  const [query, setQuery] = useState('');

  // 扁平化提取树中所有文件节点并生成路径面包屑
  // Flatten and extract all file nodes from tree with path breadcrumbs
  const allFiles = useMemo(() => {
    const nodes = flattenTree(tree);
    return nodes
      .filter((node) => node.node_type === 'file')
      .map((node) => {
        const breadcrumbs = getNodeBreadcrumbSegments(tree, node.id);
        const folderPath = breadcrumbs.slice(0, -1).join(' / ');
        return {
          node,
          folderPath,
          iconMeta: getFileIcon(node.name),
        };
      });
  }, [tree]);

  // 全局快捷键 Cmd+P / Ctrl+P 侦听
  // Global shortcut Cmd+P / Ctrl+P listener
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'p') {
        event.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onOpenChange]);

  // 重置输入词
  // Reset query on close
  useEffect(() => {
    if (!open) {
      setQuery('');
    }
  }, [open]);

  // 判断是否为纯数字查询（优先定位 Job ID）
  // Check if query is purely numeric (prioritize locating Job ID)
  const isNumericQuery = /^\d+$/.test(query.trim());

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('quickOpen')}
      description={t('quickOpenHint')}
      className='max-w-2xl border-border/80 shadow-2xl backdrop-blur-md'
    >
      <CommandInput
        placeholder={t('quickOpenPlaceholder')}
        value={query}
        onValueChange={setQuery}
        className='h-11 text-xs sm:text-sm'
      />
      <CommandList className='max-h-[380px] p-1.5'>
        <CommandEmpty className='py-8 text-center text-xs text-muted-foreground'>
          {t('noMatchingResults')}
        </CommandEmpty>

        {/* 快捷定位输入数字 Job ID */}
        {/* Direct numerical Job ID jump item */}
        {isNumericQuery ? (
          <CommandGroup heading={t('matchingJobs')}>
            <CommandItem
              value={`job-direct-${query.trim()}`}
              onSelect={() => {
                onOpenChange(false);
                void onSelectJob(query.trim());
              }}
              className='flex items-center justify-between py-2'
            >
              <div className='flex items-center gap-2'>
                <Activity className='size-3.5 text-primary' />
                <span className='font-mono text-xs font-medium text-foreground'>
                  #{query.trim()}
                </span>
                <span className='text-xs text-muted-foreground'>
                  ({t('locate')})
                </span>
              </div>
              <ArrowRight className='size-3 text-muted-foreground' />
            </CommandItem>
          </CommandGroup>
        ) : null}

        {/* 匹配的文件列表 */}
        {/* Matching task files */}
        <CommandGroup heading={`${t('matchingFiles')} (${allFiles.length})`}>
          {allFiles.map(({ node, folderPath, iconMeta }) => {
            const IconComponent = iconMeta.icon;
            return (
              <CommandItem
                key={`file-${node.id}`}
                value={`${node.name} ${folderPath} ${node.id}`}
                onSelect={() => {
                  onOpenChange(false);
                  void onSelectFile(node);
                }}
                className='group flex items-center justify-between gap-3 py-2 px-2.5 rounded-md cursor-pointer transition-colors'
              >
                <div className='flex items-center gap-2.5 min-w-0'>
                  <IconComponent
                    className={cn('size-4 shrink-0', iconMeta.color)}
                  />
                  <div className='flex flex-col min-w-0'>
                    <span className='truncate text-xs font-medium text-foreground group-hover:text-primary transition-colors'>
                      {node.name}
                    </span>
                    {folderPath ? (
                      <span className='flex items-center gap-1 text-[10px] text-muted-foreground/80 truncate'>
                        <Folder className='size-2.5 shrink-0 opacity-60' />
                        <span className='truncate'>{folderPath}</span>
                      </span>
                    ) : null}
                  </div>
                </div>

                {/* 状态徽标与快捷键提示 */}
                {/* Status pill & version */}
                <div className='flex items-center gap-1.5 shrink-0'>
                  {node.current_version ? (
                    <span className='font-mono text-[10px] text-muted-foreground/60'>
                      v{node.current_version}
                    </span>
                  ) : null}
                  <Badge
                    variant='outline'
                    className={cn(
                      'h-4.5 rounded px-1.5 text-[9px] font-normal',
                      node.status === 'published'
                        ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                        : 'border-muted-foreground/30 bg-muted/40 text-muted-foreground',
                    )}
                  >
                    {node.status === 'published' ? t('published') : t('draft')}
                  </Badge>
                </div>
              </CommandItem>
            );
          })}
        </CommandGroup>

        {/* 最近作业实例（若有） */}
        {/* Recent job executions (if available) */}
        {recentJobs.length > 0 ? (
          <>
            <CommandSeparator />
            <CommandGroup heading={t('matchingJobs')}>
              {recentJobs.map((job) => (
                <CommandItem
                  key={`job-${job.id}`}
                  value={`job-${job.id} ${job.platform_job_id || ''} ${job.task_name || ''}`}
                  onSelect={() => {
                    onOpenChange(false);
                    void onSelectJob(String(job.id));
                  }}
                  className='flex items-center justify-between py-2 px-2.5'
                >
                  <div className='flex items-center gap-2'>
                    <PlayCircle className='size-3.5 text-sky-500 shrink-0' />
                    <span className='font-mono text-xs font-semibold text-foreground'>
                      #{job.id}
                    </span>
                    {job.task_name ? (
                      <span className='text-xs text-muted-foreground truncate max-w-[200px]'>
                        {job.task_name}
                      </span>
                    ) : null}
                  </div>

                  {job.status ? (
                    <Badge
                      variant='outline'
                      className='h-4.5 text-[9px] font-normal uppercase'
                    >
                      {job.status}
                    </Badge>
                  ) : null}
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        ) : null}

        {/* 快捷操作组（当输入为空或匹配快捷动作时） */}
        {/* Quick Actions Group */}
        <CommandSeparator />
        <CommandGroup heading={t('quickActions')}>
          {onNewFile ? (
            <CommandItem
              value={`quick-action-new-file ${t('newFile')}`}
              onSelect={() => {
                onOpenChange(false);
                onNewFile();
              }}
              className='flex items-center justify-between py-1.5'
            >
              <div className='flex items-center gap-2 text-xs'>
                <FilePlus2 className='size-3.5 text-muted-foreground' />
                <span>{t('newFile')}</span>
              </div>
            </CommandItem>
          ) : null}
          {onNewFolder ? (
            <CommandItem
              value={`quick-action-new-folder ${t('newFolder')}`}
              onSelect={() => {
                onOpenChange(false);
                onNewFolder();
              }}
              className='flex items-center justify-between py-1.5'
            >
              <div className='flex items-center gap-2 text-xs'>
                <FolderPlus className='size-3.5 text-muted-foreground' />
                <span>{t('newFolder')}</span>
              </div>
            </CommandItem>
          ) : null}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
