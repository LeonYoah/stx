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
  Eye,
  Shield,
  UserCheck,
  X,
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
import { useRovingKeys } from '@/components/common/dashboard/use-roving-keys';
import { cn } from '@/lib/utils';
import type { SyncTaskAccessLevel } from '@/lib/services/sync';

// ============================================================================
// 类型定义 / Type Definitions
// ============================================================================

export interface StudioSharePopoverProps {
  isOwner?: boolean;
  isCollaborator?: boolean;
  canEdit?: boolean;
  isPublic?: boolean;
  accessLevel?: SyncTaskAccessLevel;
  createdBy?: number;
  collaboratorIds?: number[];
  workspaceUsers?: Array<{ id: number; username: string; nickname?: string }>;
  isAdmin?: boolean;
  onUpdateSharing?: (accessLevel: SyncTaskAccessLevel, collaboratorIds: number[]) => void | Promise<void>;
  onSavePermissions?: () => void | Promise<void>;
}

// 三态共享级别选项的静态配置：图标与着色约定来自任务设计稿的消歧矩阵。
// Static config of the three sharing levels; icon and color contract follows the task's
// visual disambiguation matrix.
const ACCESS_LEVEL_OPTIONS: Array<{
  value: SyncTaskAccessLevel;
  icon: typeof Lock;
  iconClass: string;
  labelKey: 'accessLevelPrivate' | 'accessLevelPublicReadOnly' | 'accessLevelPublicEditable';
  descKey: 'accessLevelPrivateDesc' | 'accessLevelPublicReadOnlyDesc' | 'accessLevelPublicEditableDesc';
}> = [
  {
    value: 'private',
    icon: Lock,
    iconClass: 'text-amber-600 dark:text-amber-400',
    labelKey: 'accessLevelPrivate',
    descKey: 'accessLevelPrivateDesc',
  },
  {
    value: 'public_readonly',
    icon: Eye,
    iconClass: 'text-sky-600 dark:text-sky-400',
    labelKey: 'accessLevelPublicReadOnly',
    descKey: 'accessLevelPublicReadOnlyDesc',
  },
  {
    value: 'public_editable',
    icon: Globe,
    iconClass: 'text-emerald-600 dark:text-emerald-400',
    labelKey: 'accessLevelPublicEditable',
    descKey: 'accessLevelPublicEditableDesc',
  },
];

// ============================================================================
// 组件实现：顶层任务共享与权限协作浮层
// Component Implementation: Top-Level Task Sharing & Collaborator Popover
// ============================================================================

export function StudioSharePopover({
  isOwner,
  isCollaborator,
  canEdit,
  isPublic = true,
  accessLevel: accessLevelProp,
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

  // access_level 缺失的旧调用方按 is_public 推导，与后端安全默认一致。
  // Legacy callers without access_level fall back to the is_public derivation, matching the
  // backend safe default.
  const accessLevel: SyncTaskAccessLevel =
    accessLevelProp ?? (isPublic === false ? 'private' : 'public_readonly');
  const activeOption =
    ACCESS_LEVEL_OPTIONS.find((option) => option.value === accessLevel) ??
    ACCESS_LEVEL_OPTIONS[1];

  const ownerUser = createdBy
    ? workspaceUsers.find((u) => u.id === createdBy)
    : null;
  const ownerName = ownerUser
    ? ownerUser.username || ownerUser.nickname || `User #${createdBy}`
    : createdBy
      ? `User #${createdBy}`
      : '-';

  const canManage = isOwner || isAdmin;

  const { refAt, onKeyDown } = useRovingKeys<HTMLButtonElement>(
    ACCESS_LEVEL_OPTIONS.length,
    (index) => {
      if (!canManage) {
        return;
      }
      const option = ACCESS_LEVEL_OPTIONS[index];
      if (option) {
        onUpdateSharing?.(option.value, collaboratorIds);
      }
    },
  );

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

  const TriggerIcon = activeOption.icon;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant='ghost'
          size='sm'
          aria-label={t('shareTask')}
          title={t('shareTask')}
          className={cn(
            'group h-9 gap-1.5 rounded-[var(--radius-control)] border border-border/60 bg-muted/20 px-2 text-xs font-normal transition-all hover:bg-muted/50 hover:text-foreground',
            open && 'bg-accent text-accent-foreground ring-1 ring-primary/30',
          )}
        >
          <Users className='size-3.5 text-primary shrink-0 transition-transform group-hover:scale-110' />
          {/* 共享级别收成着色小标：琥珀 Lock=私有、天蓝 Eye=公开只读、翠绿 Globe=公开可协作。
              Sharing level collapses to a colored glyph: amber Lock=private, sky Eye=public read-only,
              emerald Globe=public editable. */}
          <span
            title={t(activeOption.labelKey)}
            className={cn(
              'inline-flex size-3.5 shrink-0 items-center justify-center rounded-full',
              accessLevel === 'private' &&
                'bg-amber-500/15 text-amber-600 dark:text-amber-400',
              accessLevel === 'public_readonly' &&
                'bg-sky-500/15 text-sky-600 dark:text-sky-400',
              accessLevel === 'public_editable' &&
                'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
            )}
          >
            <TriggerIcon className='size-2 shrink-0' />
          </span>

          {/* 协作者人数计数 / Collaborator count */}
          {collaboratorIds.length > 0 ? (
            <span className='rounded-full bg-primary/15 px-1 py-0.2 font-mono text-[9px] font-medium text-primary leading-none'>
              +{collaboratorIds.length}
            </span>
          ) : null}
        </Button>
      </PopoverTrigger>

      <PopoverContent
        align='start'
        sideOffset={6}
        className='arc-compact w-84 p-0 shadow-xl border-border/80 bg-background/95 backdrop-blur-md rounded-lg overflow-hidden'
      >
        {/* 顶部标题栏 / Top Header */}
        <div className='flex items-center justify-between border-b-[0.5px] border-border/80 bg-muted/25 px-3.5 py-2.5'>
          <div className='flex items-center gap-1.5'>
            <Users className='size-3.5 text-primary' />
            <span className='text-xs font-semibold text-foreground'>
              {t('taskSharingAndPerms')}
            </span>
          </div>

          {/* 角色标识徽章：所有者 > 共建者 > 团队可协作 > 只读，四态互斥。 */}
          {/* Role badge, four mutually exclusive states: owner > collaborator > team editable > read-only. */}
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
          ) : canEdit ? (
            <Badge
              variant='outline'
              className='h-5 gap-1 border-emerald-500/30 bg-emerald-500/10 px-1.5 text-[10px] font-normal text-emerald-600 dark:text-emerald-400'
            >
              <Globe className='size-2.5' />
              {t('teamCollaborator')}
            </Badge>
          ) : (
            <Badge
              variant='outline'
              className='h-5 gap-1 border-border/60 bg-muted/40 px-1.5 text-[10px] font-normal text-muted-foreground'
            >
              <Eye className='size-2.5' />
              {t('readOnly')}
            </Badge>
          )}
        </div>

        {/* 所有者信息行 / Owner Information Row */}
        <div className='border-b-[0.5px] border-border/80 px-3.5 py-2.5'>
          <div className='flex items-center justify-between rounded-md border border-border/40 bg-muted/20 px-2.5 py-1.5 text-xs'>
            <span className='text-muted-foreground'>{t('owner')}</span>
            <span className='font-medium text-foreground flex items-center gap-1'>
              <Shield className='size-3 text-primary/70' />
              {ownerName}
            </span>
          </div>
        </div>

        {/* 共享级别竖排单选 / Vertical sharing-level radio rows */}
        <div className='border-b-[0.5px] border-border/80 px-3.5 py-3'>
          <Label className='mb-1.5 block text-xs font-medium text-foreground'>
            {t('taskVisibility')}
          </Label>
          <div
            role='radiogroup'
            aria-label={t('taskVisibility')}
            className='flex flex-col overflow-hidden rounded-md border border-border/60'
          >
            {ACCESS_LEVEL_OPTIONS.map((option, index) => {
              const selected = accessLevel === option.value;
              const OptionIcon = option.icon;
              return (
                <button
                  key={option.value}
                  ref={refAt(index)}
                  type='button'
                  role='radio'
                  aria-checked={selected}
                  tabIndex={selected ? 0 : -1}
                  disabled={!canManage}
                  onClick={() => onUpdateSharing?.(option.value, collaboratorIds)}
                  onKeyDown={(event) => onKeyDown(event, index)}
                  className={cn(
                    'flex items-start gap-2 px-2.5 py-2 text-left transition-colors',
                    index > 0 && 'border-t-[0.5px] border-border/80',
                    selected ? 'bg-primary/5' : 'hover:bg-muted/40',
                    !canManage && 'opacity-60',
                  )}
                >
                  <OptionIcon
                    className={cn('mt-0.5 size-3.5 shrink-0', option.iconClass)}
                  />
                  <span className='min-w-0 flex-1'>
                    <span className='flex items-center justify-between gap-2 text-xs font-medium text-foreground'>
                      {t(option.labelKey)}
                      {selected ? (
                        <Check className='size-3 shrink-0 text-primary' />
                      ) : null}
                    </span>
                    <span className='mt-0.5 block text-[10px] leading-tight text-muted-foreground'>
                      {t(option.descKey)}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        {/* 共建开发者管理 / Collaborators Management */}
        <div className='border-b-[0.5px] border-border/80 px-3.5 py-3'>
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
                  onUpdateSharing?.(accessLevel, [...collaboratorIds, uid]);
                }
              }}
            >
              <SelectTrigger className='mt-1.5 h-7.5 w-full text-xs'>
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
          <div className='mt-2 flex max-h-28 flex-wrap gap-1 overflow-y-auto pr-1'>
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
                    <UserCheck className='size-3 shrink-0 text-sky-500' />
                    <span className='max-w-[120px] truncate'>{name}</span>
                    {canManage ? (
                      <button
                        type='button'
                        className='ml-0.5 rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-muted-foreground/20 hover:text-foreground'
                        onClick={() => {
                          const next = collaboratorIds.filter((id) => id !== uid);
                          onUpdateSharing?.(accessLevel, next);
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
              <p className='py-1 text-[11px] text-muted-foreground/80'>
                {t('noCollaborators')}
              </p>
            )}
          </div>

          {/* 公开可协作时名单语义切换为保留提示，不清空已有共建者。 */}
          {/* Under public editable the list hint changes to a keep-notice; existing collaborators are not cleared. */}
          <p className='mt-2 text-[10px] leading-4 text-muted-foreground/80'>
            {accessLevel === 'public_editable'
              ? t('publicEditableCollaboratorHint')
              : canManage
                ? t('collaboratorHint')
                : t('nonOwnerPermHint')}
          </p>
        </div>

        {/* 底部保存动作栏（仅所有者/管理员展示） */}
        {/* Footer save action bar (shown only for owner/admin) */}
        {canManage && onSavePermissions ? (
          <div className='flex items-center justify-end gap-2 bg-muted/20 px-3.5 py-2'>
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
