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

import React, { useState } from 'react';
import { useTranslations } from 'next-intl';
import {
  Users,
  Globe,
  Lock,
  Shield,
  UserCheck,
  X,
  ChevronDown,
  Check,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';

// ============================================================================
// 类型定义 / Type Definitions
// ============================================================================

export interface StudioSharePopoverProps {
  isOwner?: boolean;
  isCollaborator?: boolean;
  canEdit?: boolean;
  isPublic?: boolean;
  createdBy?: number;
  collaboratorIds?: number[];
  workspaceUsers?: Array<{ id: number; username: string; nickname?: string }>;
  isAdmin?: boolean;
  onUpdateSharing?: (isPublic: boolean, collaboratorIds: number[]) => void | Promise<void>;
  onSavePermissions?: () => void | Promise<void>;
}

// ============================================================================
// 组件实现：顶层任务共享与权限协作浮层
// Component Implementation: Top-Level Task Sharing & Collaborator Popover
// ============================================================================

export function StudioSharePopover({
  isOwner,
  isCollaborator,
  isPublic = true,
  createdBy,
  collaboratorIds = [],
  workspaceUsers = [],
  isAdmin = false,
  onUpdateSharing,
  onSavePermissions,
}: StudioSharePopoverProps) {
  const t = useTranslations('workbenchStudio');
  const [open, setOpen] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  const ownerUser = createdBy
    ? workspaceUsers.find((u) => u.id === createdBy)
    : null;
  const ownerName = ownerUser
    ? ownerUser.username || ownerUser.nickname || `User #${createdBy}`
    : createdBy
      ? `User #${createdBy}`
      : '-';

  const canManage = isOwner || isAdmin;

  // 触发保存权限
  // Trigger save permissions
  const handleSave = async () => {
    if (!onSavePermissions) {
      return;
    }
    try {
      setIsSaving(true);
      await onSavePermissions();
      setOpen(false);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant='ghost'
          size='sm'
          className={cn(
            'group h-6.5 gap-1.5 rounded-full border border-border/60 bg-muted/20 px-2 text-xs font-normal transition-all hover:bg-muted/50 hover:text-foreground active:scale-95',
            open && 'bg-accent text-accent-foreground ring-1 ring-primary/30',
          )}
        >
          <Users className='size-3.5 text-primary shrink-0 transition-transform group-hover:scale-110' />
          <span className='text-[11px] font-medium'>{t('shareTask')}</span>

          {/* 可见性徽章 / Visibility badge */}
          <span
            className={cn(
              'inline-flex items-center gap-1 rounded-full px-1.5 py-0.2 text-[10px] font-normal leading-none',
              isPublic
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                : 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
            )}
          >
            {isPublic ? (
              <Globe className='size-2.5 shrink-0' />
            ) : (
              <Lock className='size-2.5 shrink-0' />
            )}
            <span>{isPublic ? t('public') : t('private')}</span>
          </span>

          {/* 协作者人数计数 / Collaborator count badge */}
          {collaboratorIds.length > 0 ? (
            <span className='rounded-full bg-primary/15 px-1 py-0.2 font-mono text-[9px] font-medium text-primary leading-none'>
              +{collaboratorIds.length}
            </span>
          ) : null}

          <ChevronDown className='size-3 text-muted-foreground/60 transition-transform group-hover:text-foreground' />
        </Button>
      </PopoverTrigger>

      <PopoverContent
        align='start'
        sideOffset={6}
        className='w-84 p-0 shadow-xl border-border/80 bg-background/95 backdrop-blur-md rounded-lg overflow-hidden'
      >
        {/* 顶部标题栏 / Top Header */}
        <div className='flex items-center justify-between border-b border-border/60 bg-muted/25 px-3.5 py-2.5'>
          <div className='flex items-center gap-1.5'>
            <Users className='size-3.5 text-primary' />
            <span className='text-xs font-semibold text-foreground'>
              {t('taskSharingAndPerms')}
            </span>
          </div>

          {/* 角色标识徽章 / Role Badge */}
          {isOwner ? (
            <Badge
              variant='outline'
              className='h-5 gap-1 border-primary/30 bg-primary/10 px-1.5 text-[10px] font-normal text-primary'
            >
              <Shield className='size-2.5' />
              {t('owner')}
            </Badge>
          ) : isCollaborator ? (
            <Badge
              variant='outline'
              className='h-5 gap-1 border-sky-500/30 bg-sky-500/10 px-1.5 text-[10px] font-normal text-sky-600 dark:text-sky-400'
            >
              <UserCheck className='size-2.5' />
              {t('collaborator')}
            </Badge>
          ) : (
            <Badge
              variant='outline'
              className='h-5 gap-1 border-amber-500/30 bg-amber-500/10 px-1.5 text-[10px] font-normal text-amber-600 dark:text-amber-400'
            >
              <Lock className='size-2.5' />
              {t('readOnlyLocked')}
            </Badge>
          )}
        </div>

        <div className='space-y-3.5 p-3.5'>
          {/* 所有者信息行 / Owner Information Row */}
          <div className='flex items-center justify-between rounded-md border border-border/40 bg-muted/20 px-2.5 py-1.5 text-xs'>
            <span className='text-muted-foreground'>{t('owner')}</span>
            <span className='font-medium text-foreground flex items-center gap-1'>
              <Shield className='size-3 text-primary/70' />
              {ownerName}
            </span>
          </div>

          {/* 可见性卡片选择 / Visibility Card Selector */}
          <div className='space-y-1.5'>
            <Label className='text-xs font-medium text-foreground'>
              {t('taskVisibility')}
            </Label>
            <div className='grid grid-cols-2 gap-2'>
              <button
                type='button'
                disabled={!canManage}
                onClick={() => onUpdateSharing?.(true, collaboratorIds)}
                className={cn(
                  'flex flex-col items-start gap-1 rounded-md border p-2 text-left transition-all',
                  isPublic
                    ? 'border-primary/50 bg-primary/5 shadow-2xs'
                    : 'border-border/60 hover:bg-muted/40 opacity-70',
                  !canManage && 'cursor-not-allowed opacity-60',
                )}
              >
                <div className='flex w-full items-center justify-between'>
                  <span className='flex items-center gap-1.5 text-xs font-medium text-foreground'>
                    <Globe className='size-3 text-emerald-500' />
                    {t('public')}
                  </span>
                  {isPublic ? (
                    <Check className='size-3 text-primary shrink-0' />
                  ) : null}
                </div>
                <span className='text-[10px] leading-tight text-muted-foreground line-clamp-2'>
                  {t('publicVisibilityDesc')}
                </span>
              </button>

              <button
                type='button'
                disabled={!canManage}
                onClick={() => onUpdateSharing?.(false, collaboratorIds)}
                className={cn(
                  'flex flex-col items-start gap-1 rounded-md border p-2 text-left transition-all',
                  !isPublic
                    ? 'border-primary/50 bg-primary/5 shadow-2xs'
                    : 'border-border/60 hover:bg-muted/40 opacity-70',
                  !canManage && 'cursor-not-allowed opacity-60',
                )}
              >
                <div className='flex w-full items-center justify-between'>
                  <span className='flex items-center gap-1.5 text-xs font-medium text-foreground'>
                    <Lock className='size-3 text-amber-500' />
                    {t('private')}
                  </span>
                  {!isPublic ? (
                    <Check className='size-3 text-primary shrink-0' />
                  ) : null}
                </div>
                <span className='text-[10px] leading-tight text-muted-foreground line-clamp-2'>
                  {t('privateVisibilityDesc')}
                </span>
              </button>
            </div>
          </div>

          {/* 共建开发者管理 / Collaborators Management */}
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <Label className='text-xs font-medium text-foreground'>
                {t('manageCollaborators')}
              </Label>
              <span className='font-mono text-[10px] text-muted-foreground'>
                ({collaboratorIds.length})
              </span>
            </div>

            {/* 添加共建者下拉框 / Add Collaborator Select */}
            {canManage ? (
              <Select
                value='__none__'
                onValueChange={(val) => {
                  if (val === '__none__') {
                    return;
                  }
                  const uid = Number(val);
                  if (!uid || isNaN(uid)) {
                    return;
                  }
                  if (!collaboratorIds.includes(uid)) {
                    onUpdateSharing?.(isPublic, [...collaboratorIds, uid]);
                  }
                }}
              >
                <SelectTrigger className='h-7.5 w-full text-xs'>
                  <SelectValue placeholder={t('selectCollaborator')} />
                </SelectTrigger>
                <SelectContent className='max-h-48 w-[var(--radix-select-trigger-width)] min-w-0 text-xs'>
                  <SelectItem
                    value='__none__'
                    disabled
                    className='text-xs text-muted-foreground'
                  >
                    {t('selectCollaborator')}
                  </SelectItem>
                  {workspaceUsers
                    .filter(
                      (u) =>
                        u.id !== createdBy && !collaboratorIds.includes(u.id),
                    )
                    .map((u) => (
                      <SelectItem
                        key={u.id}
                        value={String(u.id)}
                        className='text-xs'
                      >
                        {u.username} {u.nickname ? `(${u.nickname})` : ''}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            ) : null}

            {/* 已有共建开发者徽章列表 / Collaborator Badges List */}
            <div className='flex flex-wrap gap-1 max-h-28 overflow-y-auto pr-1'>
              {collaboratorIds.length > 0 ? (
                collaboratorIds.map((uid) => {
                  const u = workspaceUsers.find((user) => user.id === uid);
                  const name = u ? u.username || u.nickname : `User #${uid}`;
                  return (
                    <Badge
                      key={uid}
                      variant='secondary'
                      className='h-6 gap-1 px-2 text-[11px] font-normal'
                    >
                      <UserCheck className='size-3 text-sky-500 shrink-0' />
                      <span className='max-w-[120px] truncate'>{name}</span>
                      {canManage ? (
                        <button
                          type='button'
                          className='ml-0.5 rounded-full p-0.5 hover:bg-muted-foreground/20 text-muted-foreground hover:text-foreground transition-colors'
                          onClick={() => {
                            const next = collaboratorIds.filter((id) => id !== uid);
                            onUpdateSharing?.(isPublic, next);
                          }}
                          title={t('removeCollaborator')}
                        >
                          <X className='size-2.5' />
                        </button>
                      ) : null}
                    </Badge>
                  );
                })
              ) : (
                <p className='text-[11px] text-muted-foreground/80 py-1'>
                  {t('noCollaborators')}
                </p>
              )}
            </div>
          </div>

          {/* 权限说明提示文案 / Permission Hint */}
          <p className='text-[10px] leading-4 text-muted-foreground/80'>
            {canManage ? t('collaboratorHint') : t('nonOwnerPermHint')}
          </p>
        </div>

        {/* 底部保存动作栏（仅所有者/管理员展示） */}
        {/* Footer save action bar (shown only for owner/admin) */}
        {canManage && onSavePermissions ? (
          <div className='flex items-center justify-end gap-2 border-t border-border/60 bg-muted/20 px-3.5 py-2'>
            <Button
              size='sm'
              className='h-7 px-3 text-xs'
              onClick={handleSave}
              disabled={isSaving}
            >
              {isSaving ? t('saving') : t('saveSharingSettings')}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
