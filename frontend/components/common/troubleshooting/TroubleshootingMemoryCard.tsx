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

import {useState, useCallback} from 'react';
import {
  Check,
  Copy,
  Edit3,
  Lightbulb,
  PlusCircle,
  ShieldCheck,
  Tag,
  User,
} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import {useLocale} from '@/lib/i18n';
import {
  getLocalizedMemory,
  type TroubleshootingMemoryEntry,
} from '@/lib/services/troubleshooting';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';

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
  return parsed.toLocaleDateString();
}

/**
 * 历史排障经验置顶回显卡片组件
 * Troubleshooting memory sticky recall card component
 */
export function TroubleshootingMemoryCard({
  matchedMemory,
  totalMatches = 0,
  onAddOrEdit,
  className,
}: TroubleshootingMemoryCardProps) {
  const {locale} = useLocale();
  const isEn = String(locale).toLowerCase().startsWith('en');
  // 官方经典方案自动做中英文两版自适应，自定义方案保持原样
  // Adapt official preset memories bilingually; keep custom entries as-is
  const memory = getLocalizedMemory(matchedMemory, locale);
  const [copiedSolution, setCopiedSolution] = useState(false);

  // 复制解决方案内容到剪贴板
  // Copy verified solution text to clipboard
  const handleCopySolution = useCallback(
    (text: string) => {
      if (!text) {
        return;
      }
      navigator.clipboard.writeText(text);
      setCopiedSolution(true);
      toast.success(
        isEn ? 'Solution copied to clipboard' : '排障解决方案已复制到剪切板',
      );
      setTimeout(() => setCopiedSolution(false), 2000);
    },
    [isEn],
  );

  // 未命中历史经验时的引导提示态
  // Guidance prompt state when no historical solution is matched
  if (!memory) {
    return (
      <div
        className={cn(
          'rounded-lg border border-dashed border-emerald-500/30 bg-emerald-500/5 dark:bg-emerald-950/15 p-3.5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 text-xs',
          className,
        )}
      >
        <div className='flex items-start gap-2.5'>
          <div className='p-1.5 rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 mt-0.5 shrink-0'>
            <Lightbulb className='h-4 w-4' />
          </div>
          <div className='space-y-0.5'>
            <div className='font-semibold text-foreground flex items-center gap-1.5'>
              <span>{isEn ? 'Troubleshooting Memory' : '排障经验记忆库'}</span>
              <span className='text-[11px] font-normal text-muted-foreground'>
                {isEn
                  ? '(No record for this fingerprint)'
                  : '（尚未记录此指纹经验）'}
              </span>
            </div>
            <p className='text-muted-foreground text-[11px] leading-relaxed'>
              {isEn
                ? 'After resolving this issue, record verified solutions and configuration changes here to auto-recall when similar issues occur.'
                : '排查处理此问题后，可沉淀真实解决方案与配置调整措施，下次发生同类故障时将在此自动置顶回显。'}
            </p>
          </div>
        </div>
        {onAddOrEdit && (
          <Button
            variant='outline'
            size='sm'
            onClick={onAddOrEdit}
            className='h-7 px-2.5 text-xs shrink-0 border-emerald-500/30 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/10 gap-1'
          >
            <PlusCircle className='h-3.5 w-3.5' />
            {isEn ? 'Record Solution' : '记排障方案'}
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
        'rounded-lg border border-emerald-500/40 bg-emerald-500/5 dark:bg-emerald-950/20 p-3.5 space-y-2.5 text-xs shadow-xs',
        className,
      )}
    >
      {/* 头部：标题与元数据 / Card Header: Title & Metadata */}
      <div className='flex flex-wrap items-center justify-between gap-2 border-b border-emerald-500/20 pb-2'>
        <div className='flex items-center gap-2 flex-wrap'>
          <div className='p-1 rounded-md bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 shrink-0'>
            <Lightbulb className='h-4 w-4' />
          </div>
          <span className='font-semibold text-foreground text-sm tracking-tight'>
            {memory.title}
          </span>
          {memory.is_preset ? (
            <Badge
              variant='outline'
              className='text-[10px] py-0 px-1.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10'
            >
              {isEn ? 'Official Preset' : '官方经典方案'}
            </Badge>
          ) : (
            <Badge
              variant='outline'
              className='text-[10px] py-0 px-1.5 border-primary/30 text-primary bg-primary/10'
            >
              {isEn ? 'Team Knowledge' : '团队沉淀'}
            </Badge>
          )}
          {totalMatches > 1 && (
            <Badge variant='secondary' className='text-[10px] py-0 px-1.5'>
              {isEn
                ? `${totalMatches} solutions found`
                : `共 ${totalMatches} 条相关方案`}
            </Badge>
          )}
        </div>

        {/* 作者与编辑操作 / Author and Edit Actions */}
        <div className='flex items-center gap-2'>
          <span className='text-[11px] text-muted-foreground flex items-center gap-1 font-mono'>
            <User className='h-3 w-3' />
            {memory.author} · {formatDateTime(memory.updated_at)}
          </span>
          {onAddOrEdit && (
            <Button
              variant='ghost'
              size='sm'
              onClick={onAddOrEdit}
              className='h-6 px-1.5 text-xs text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/15'
            >
              <Edit3 className='mr-1 h-3 w-3' />
              {memory.is_preset
                ? isEn
                  ? 'Add Note'
                  : '补充心得'
                : isEn
                  ? 'Edit'
                  : '编辑经验'}
            </Button>
          )}
        </div>
      </div>

      {/* 根本原因剖析（若有） / Root Cause (if available) */}
      {memory.root_cause && (
        <div className='space-y-1 bg-background/60 p-2.5 rounded-md border border-emerald-500/15'>
          <div className='font-medium text-foreground text-[11px] flex items-center gap-1 text-muted-foreground'>
            <span>{isEn ? 'Root Cause Analysis:' : '根因分析：'}</span>
          </div>
          <p className='text-muted-foreground leading-relaxed text-xs'>
            {memory.root_cause}
          </p>
        </div>
      )}

      {/* 核心必填：已验证的解决方案与执行步骤 / Core Verified Solution & Remediation Steps */}
      <div className='space-y-1.5 bg-emerald-500/10 dark:bg-emerald-900/25 p-3 rounded-md border border-emerald-500/25'>
        <div className='flex items-center justify-between'>
          <span className='font-semibold text-emerald-950 dark:text-emerald-100 flex items-center gap-1 text-xs'>
            <ShieldCheck className='h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400' />
            {isEn
              ? 'Verified Solution & Remediation:'
              : '解决方案与排障动作（建议采纳）:'}
          </span>
          <Button
            variant='ghost'
            size='sm'
            onClick={() => handleCopySolution(memory.solution)}
            className='h-6 px-1.5 text-xs text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20'
          >
            {copiedSolution ? (
              <>
                <Check className='mr-1 h-3 w-3 text-emerald-600 dark:text-emerald-400' />
                {isEn ? 'Copied' : '已复制'}
              </>
            ) : (
              <>
                <Copy className='mr-1 h-3 w-3' />
                {isEn ? 'Copy Solution' : '复制方案'}
              </>
            )}
          </Button>
        </div>
        <pre className='whitespace-pre-wrap font-sans text-xs leading-relaxed text-foreground select-text'>
          {memory.solution}
        </pre>
      </div>

      {/* 防范建议与标签 / Preventive Tips & Tags */}
      <div className='flex flex-wrap items-center justify-between gap-2 pt-0.5 text-[11px] text-muted-foreground'>
        {memory.preventive_tips ? (
          <div className='flex items-center gap-1 leading-snug'>
            <span className='font-medium text-foreground'>
              {isEn ? '💡 Prevention:' : '💡 防范建议:'}
            </span>
            <span>{memory.preventive_tips}</span>
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
                className='px-1.5 py-0.2 rounded bg-background/80 border text-[10px] font-mono'
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
