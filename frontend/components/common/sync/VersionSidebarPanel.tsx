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

import {useTranslations} from 'next-intl';
import {GitCommit, MoreHorizontal, Trash2} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import type {SyncTaskVersion} from '@/lib/services/sync';

export function VersionSidebarPanel({
  taskId,
  canEdit = true,
  currentVersion,
  versions,
  total,
  page,
  pageSize,
  onPageChange,
  onPreview,
  onCompare,
  onRollback,
  onDelete,
  onPublish,
}: {
  taskId?: number;
  canEdit?: boolean;
  currentVersion: number;
  versions: SyncTaskVersion[];
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPreview: (version: SyncTaskVersion) => void;
  onCompare: (version: SyncTaskVersion) => void;
  onRollback: (versionId: number) => void;
  onDelete: (versionId: number) => void;
  onPublish?: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  if (!taskId) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('selectFileToViewVersions')}
      </div>
    );
  }
  return (
    <div className='space-y-3 min-w-0 w-full'>
      {/* 版本总览卡片 */}
      {/* Version summary card */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='flex items-center justify-between gap-2'>
          <div>
            <div className='text-[11px] uppercase tracking-wide text-muted-foreground'>
              {t('versionManagement')}
            </div>
            <div className='mt-0.5 text-base font-semibold text-foreground'>
              v{currentVersion}
            </div>
          </div>
          <div className='flex items-center gap-1.5'>
            {onPublish && canEdit ? (
              <Button
                size='sm'
                variant='outline'
                className='h-6 gap-1 px-2 text-[11px]'
                onClick={onPublish}
              >
                <GitCommit className='size-3 text-primary' />
                <span>{t('publishNewVersion')}</span>
              </Button>
            ) : null}
            <Badge variant='outline' className='text-xs'>
              {t('totalItems', {count: versions.length})}
            </Badge>
          </div>
        </div>
        <p className='mt-1 text-[11px] leading-relaxed text-muted-foreground'>
          {t('versionManagementDesc')}
        </p>
      </div>

      {/* 版本列表卡片 */}
      {/* Version items list */}
      <div className='space-y-2'>
        {versions.length > 0 ? (
          versions.map((version) => {
            const isActive = version.version === currentVersion;
            return (
              <div
                key={version.id}
                className={cn(
                  'rounded-lg border p-2.5 transition-all space-y-2',
                  isActive
                    ? 'border-primary/40 bg-background shadow-xs'
                    : 'border-border/50 bg-background/60 hover:bg-background/90',
                )}
              >
                <div className='flex items-center justify-between gap-2'>
                  <div className='flex items-center gap-2 min-w-0'>
                    <span className='font-semibold text-xs text-foreground'>
                      v{version.version}
                    </span>
                    {isActive ? (
                      <Badge
                        variant='outline'
                        className='h-4 border-primary/40 bg-primary/10 px-1 text-[10px] text-primary font-normal'
                      >
                        {t('currentActiveVersion')}
                      </Badge>
                    ) : null}
                  </div>
                  <span className='font-mono text-[10px] text-muted-foreground shrink-0'>
                    #{version.id}
                  </span>
                </div>

                <div className='text-[10px] text-muted-foreground'>
                  {new Date(version.created_at).toLocaleString()}
                </div>

                {/* 操作按键组 */}
                {/* Actions button group */}
                <div className='flex items-center justify-between gap-1 pt-2 border-t border-border/40'>
                  <div className='flex items-center gap-1'>
                    <Button
                      size='sm'
                      variant='ghost'
                      className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                      onClick={() => onPreview(version)}
                    >
                      {t('preview')}
                    </Button>
                    <Button
                      size='sm'
                      variant='ghost'
                      className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                      onClick={() => onCompare(version)}
                    >
                      {t('compare')}
                    </Button>
                  </div>

                  <div className='flex items-center gap-0.5'>
                    {!isActive ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span>
                            <Button
                              size='sm'
                              variant='outline'
                              className='h-6 px-2 text-xs'
                              disabled={!canEdit}
                              onClick={() => onRollback(version.id)}
                            >
                              {t('rollback')}
                            </Button>
                          </span>
                        </TooltipTrigger>
                        {!canEdit ? (
                          <TooltipContent side='left'>
                            {t('readOnlyVersionTooltip')}
                          </TooltipContent>
                        ) : null}
                      </Tooltip>
                    ) : null}

                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          size='icon'
                          variant='ghost'
                          className='size-6 text-muted-foreground hover:text-foreground'
                        >
                          <MoreHorizontal className='size-3.5' />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='end'>
                        <DropdownMenuItem
                          className='text-destructive focus:text-destructive'
                          disabled={!canEdit}
                          onClick={() => onDelete(version.id)}
                        >
                          <Trash2 className='mr-2 size-3.5' />
                          {t('delete')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              </div>
            );
          })
        ) : (
          <div className='text-xs text-muted-foreground py-4 text-center'>
            {t('noVersionHistory')}
          </div>
        )}
      </div>
      <SimplePagination
        total={total}
        page={page}
        pageSize={pageSize}
        onPageChange={onPageChange}
      />
    </div>
  );
}

export function SimplePagination({
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
  const totalPages = Math.max(1, Math.ceil(total / Math.max(pageSize, 1)));
  if (total <= pageSize) {
    return null;
  }
  return (
    <div className='flex items-center justify-between gap-2 rounded-lg border border-border/50 bg-muted/10 px-3 py-2 text-xs text-muted-foreground'>
      <span>{t('paginationSummary', {page, totalPages, total})}</span>
      <div className='flex items-center gap-2'>
        <Button
          size='sm'
          variant='outline'
          className='h-7 px-2 text-xs'
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t('prevPage')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          className='h-7 px-2 text-xs'
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t('nextPage')}
        </Button>
      </div>
    </div>
  );
}

