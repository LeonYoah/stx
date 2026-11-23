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
import {
  Edit3,
  Lightbulb,
  PlusCircle,
  ShieldCheck,
  Tag,
  User,
} from 'lucide-react';
import {cn} from '@/lib/utils';
import {useLocale} from '@/lib/i18n';
import {
  getLocalizedMemory,
  type TroubleshootingMemoryEntry,
} from '@/lib/services/troubleshooting';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/arc/button/button';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {formatDateTz} from '@/lib/timezone';

export interface TroubleshootingMemoryCardProps {
  /**
   * 命中的排障经验条目（若无则展示引导卡片）
   * Matched troubleshooting memory entry (shows onboarding callout if absent)
   */
  matchedMemory?: TroubleshootingMemoryEntry | null;

  /**
   * 针对当前指纹的全部命中数量（可选）
   * Total count of matched memories for current fingerprint (optional)
   */
  totalMatches?: number;

  /**
   * 触发记录或编辑经验回调
   * Callback to open save/edit memory dialog
   */
  onAddOrEdit?: () => void;

  /**
   * 自定义类名
   * Custom CSS class name
   */
  className?: string;
}

// 格式化日期时间
// Format ISO date string into readable local date time
function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatDateTz(parsed);
}

/**
 * 经验库方案置顶回显卡片
 * Sticky playbook recall card for matched fingerprints
 */
export function TroubleshootingMemoryCard({
  matchedMemory,
  totalMatches = 0,
  onAddOrEdit,
  className,
}: TroubleshootingMemoryCardProps) {
  const t = useTranslations('troubleshooting');
  const tCommon = useTranslations('common');
  const {locale} = useLocale();
  // 官方经典方案自动做中英文两版自适应，自定义方案保持原样
  // Adapt official preset memories bilingually; keep custom entries as-is
  const memory = getLocalizedMemory(matchedMemory, locale);

  // 未命中历史经验时的引导提示态
  // Guidance prompt state when no historical solution is matched
  if (!memory) {
    return (
      <div
        className={cn(
          'rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 text-xs',
          className,
        )}
      >
        <div className='flex items-start gap-2.5'>
          <div className='p-1.5 rounded-lg bg-muted text-muted-foreground mt-0.5 shrink-0'>
            <Lightbulb className='h-4 w-4' />
          </div>
          <div className='space-y-0.5'>
            <div className='font-medium text-foreground flex items-center gap-1.5'>
              <span>{t('title')}</span>
              <span className='text-[11px] font-normal text-muted-foreground'>
                ({t('notRecordedForFingerprint')})
              </span>
            </div>
            <p className='text-muted-foreground text-[11px] leading-relaxed text-pretty'>
              {t('noMemoryFound')}
            </p>
          </div>
        </div>
        {onAddOrEdit && (
          <Button
            variant='secondary'
            size='sm'
            onClick={onAddOrEdit}
            className='shrink-0'
          >
            <PlusCircle className='size-3.5' />
            {t('recordSolution')}
          </Button>
        )}
      </div>
    );
  }

  // 命中历史排障经验的置顶回显态
  // Prominent recall state when matching historical solution is found
  return (
    <div
      className={cn(
        'rounded-xl border border-border/60 bg-muted/20 p-3.5 space-y-2.5 text-xs',
        className,
      )}
    >
      {/* 头部：标题与元数据 / Card Header: Title & Metadata */}
      <div className='flex flex-wrap items-center justify-between gap-2 border-b border-border/60 pb-2'>
        <div className='flex items-center gap-2 flex-wrap min-w-0'>
          <div className='p-1 rounded-md bg-muted text-muted-foreground shrink-0'>
            <Lightbulb className='h-4 w-4' />
          </div>
          <span className='font-medium text-foreground text-sm'>
            {memory.title}
          </span>
          {memory.is_preset ? (
            <Badge variant='outline' className='text-[10px] py-0 px-1.5'>
              {t('presetSolution')}
            </Badge>
          ) : (
            <Badge variant='secondary' className='text-[10px] py-0 px-1.5'>
              {t('teamSolution')}
            </Badge>
          )}
          {totalMatches > 1 && (
            <Badge
              variant='secondary'
              className='text-[10px] py-0 px-1.5 tabular-nums'
            >
              {t('matchedCount', {count: totalMatches})}
            </Badge>
          )}
        </div>

        {/* 作者与编辑操作 / Author and Edit Actions */}
        <div className='flex items-center gap-2'>
          <span className='text-[11px] text-muted-foreground flex items-center gap-1 font-mono tabular-nums'>
            <User className='h-3 w-3' />
            {memory.author} · {formatDateTime(memory.updated_at)}
          </span>
          {onAddOrEdit && (
            <Button
              variant='ghost'
              size='sm'
              onClick={onAddOrEdit}
              className='-mr-1'
            >
              <Edit3 className='size-3' />
              {memory.is_preset ? t('addNote') : t('edit')}
            </Button>
          )}
        </div>
      </div>

      {/* 根本原因剖析（若有） / Root Cause (if available) */}
      {memory.root_cause && (
        <div className='space-y-1 rounded-lg border border-border/50 bg-background/60 p-2.5'>
          <div className='text-[11px] font-medium text-muted-foreground'>
            {t('rootCause')}
          </div>
          <p className='text-muted-foreground leading-relaxed text-pretty'>
            {memory.root_cause}
          </p>
        </div>
      )}

      {/* 核心必填：已验证的解决方案与执行步骤 / Core Verified Solution & Remediation Steps */}
      <div className='space-y-1.5 rounded-lg border border-border/60 bg-background/80 p-3'>
        <div className='flex items-center justify-between gap-2'>
          <span className='font-medium text-foreground flex items-center gap-1.5 text-xs'>
            <ShieldCheck className='h-3.5 w-3.5 text-primary' />
            {t('solutionActionsHint')}
          </span>
          <CopyButton
            value={memory.solution}
            variant='plain'
            label={t('copySolution')}
            copiedLabel={tCommon('copied')}
            failedLabel={tCommon('copyFailed')}
          />
        </div>
        <pre className='whitespace-pre-wrap font-sans text-xs leading-relaxed text-foreground select-text text-pretty'>
          {memory.solution}
        </pre>
      </div>

      {/* 防范建议与标签 / Preventive Tips & Tags */}
      <div className='flex flex-wrap items-center justify-between gap-2 pt-0.5 text-[11px] text-muted-foreground'>
        {memory.preventive_tips ? (
          <div className='flex items-start gap-1.5 leading-snug'>
            <Lightbulb className='mt-0.5 h-3 w-3 shrink-0' />
            <span className='shrink-0 whitespace-nowrap font-medium text-foreground'>
              {t('preventiveTips')}:
            </span>
            <span className='min-w-0 text-pretty'>{memory.preventive_tips}</span>
          </div>
        ) : (
          <div />
        )}

        {memory.tags && memory.tags.length > 0 && (
          <div className='flex items-center gap-1 flex-wrap'>
            <Tag className='h-3 w-3 opacity-60' />
            {memory.tags.map((tag) => (
              <span
                key={tag}
                className='px-1.5 py-0.2 rounded bg-background/80 border border-border/50 text-[10px] font-mono'
              >
                #{tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
