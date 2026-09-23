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

import {useEffect, useMemo, useState, type Dispatch, type ReactNode, type SetStateAction} from 'react';

import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';

import {
  cronToText,
  expressionFromParts,
  getNextRuns,
  parseExpressionToParts,
  type CronTranslator,
} from '@/components/common/sync/task-schedule-cron';

export interface CronPresetOption {
  key: string;
  expr: string;
}

export interface CronExpressionEditorLabels {
  cronExpression: string;
  timezone: string;
  cronFiveField: string;
  cronTimezonePlaceholder: string;
  cronExpressionPlaceholder: string;
  cronMinute: string;
  cronHour: string;
  cronDayOfMonth: string;
  cronMonth: string;
  cronDayOfWeek: string;
  schedulePreview: string;
  nextRuns: string;
  invalidCronExpression: string;
}

function formatDate(date: Date) {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function BuilderFields({
  parts,
  setParts,
  labels,
}: {
  parts: Record<string, string>;
  setParts: Dispatch<SetStateAction<Record<string, string>>>;
  labels: CronExpressionEditorLabels;
}) {
  return (
    // 5 段 Cron 字段栅格布局（防止窄栏下标签被迫换行）
    // 5-field Cron grid layout (prevent labels from forced wrapping in narrow columns)
    <div className='grid grid-cols-5 gap-1.5'>
      {(['minute', 'hour', 'dayOfMonth', 'month', 'dayOfWeek'] as const).map((key) => {
        const fullLabel = labels[`cron${key.charAt(0).toUpperCase()}${key.slice(1)}` as keyof CronExpressionEditorLabels] as string;
        return (
          <div key={key} className='min-w-0 space-y-1 text-center'>
            <Label
              title={fullLabel}
              className='block truncate text-[10px] font-medium text-muted-foreground whitespace-nowrap'
            >
              {fullLabel}
            </Label>
            <Input
              value={parts[key]}
              onChange={(e) => setParts((prev) => ({...prev, [key]: e.target.value}))}
              className='h-8 px-1 text-center text-xs font-mono bg-background/70'
            />
          </div>
        );
      })}
    </div>
  );
}

export function CronExpressionEditor({
  expression,
  onExpressionChange,
  timezone,
  onTimezoneChange,
  translator,
  labels,
  presets,
  renderPresetLabel,
  footer,
  helper,
}: {
  expression: string;
  onExpressionChange: (value: string) => void;
  timezone?: string;
  onTimezoneChange?: (value: string) => void;
  translator: CronTranslator;
  labels: CronExpressionEditorLabels;
  presets: CronPresetOption[];
  renderPresetLabel: (key: string) => string;
  footer?: ReactNode;
  helper?: ReactNode;
}) {
  const [parts, setParts] = useState<Record<string, string>>(() => {
    try {
      return parseExpressionToParts(expression || '0 0 * * *');
    } catch {
      return {minute: '0', hour: '0', dayOfMonth: '*', month: '*', dayOfWeek: '*'};
    }
  });

  useEffect(() => {
    try {
      setParts(parseExpressionToParts(expression || '0 0 * * *'));
    } catch {
      // ignore invalid external expression and keep local builder state
    }
  }, [expression]);

  const derivedExpression = useMemo(() => expressionFromParts(parts), [parts]);
  const previewState = useMemo(() => {
    try {
      return {
        ok: true,
        text: cronToText(expression || derivedExpression, translator),
        nextRuns: getNextRuns(expression || derivedExpression, 5),
      } as const;
    } catch (error) {
      return {
        ok: false,
        error: error instanceof Error ? error.message : labels.invalidCronExpression,
      } as const;
    }
  }, [derivedExpression, expression, labels.invalidCronExpression, translator]);

  return (
    <div className='space-y-3'>
      {onTimezoneChange ? (
        <div className='space-y-1.5'>
          <Label className='text-xs'>{labels.timezone}</Label>
          <Input
            value={timezone || ''}
            onChange={(e) => onTimezoneChange(e.target.value)}
            className='h-9 text-xs'
            placeholder={labels.cronTimezonePlaceholder}
          />
        </div>
      ) : null}

      <div className='space-y-1.5'>
        <div className='flex items-center justify-between gap-2'>
          <Label className='text-xs'>{labels.cronExpression}</Label>
          <Badge variant='outline' className='rounded-sm text-[10px]'>
            {labels.cronFiveField}
          </Badge>
        </div>
        <Input
          value={expression}
          onChange={(e) => {
            const nextExpr = e.target.value;
            try {
              setParts(parseExpressionToParts(nextExpr));
            } catch {
              onExpressionChange(nextExpr);
              return;
            }
            onExpressionChange(nextExpr);
          }}
          className='h-9 text-xs font-mono'
          placeholder={labels.cronExpressionPlaceholder}
        />
        {helper ? <div className='text-xs text-muted-foreground'>{helper}</div> : null}
      </div>

      <BuilderFields
        parts={parts}
        setParts={(next) => {
          const resolved = typeof next === 'function' ? next(parts) : next;
          setParts(resolved);
          onExpressionChange(expressionFromParts(resolved));
        }}
        labels={labels}
      />

      {/* 快捷预设按钮组 */}
      {/* Quick preset buttons */}
      <div className='flex flex-wrap gap-1.5'>
        {presets.map((preset) => {
          const isSelected = (expression || derivedExpression) === preset.expr;
          return (
            <Button
              key={preset.key}
              type='button'
              variant={isSelected ? 'default' : 'secondary'}
              className={cn(
                'h-7 rounded-md px-2.5 text-[11px] transition-all',
                isSelected && 'shadow-xs font-medium',
              )}
              onClick={() => {
                setParts(parseExpressionToParts(preset.expr));
                onExpressionChange(preset.expr);
              }}
            >
              {renderPresetLabel(preset.key)}
            </Button>
          );
        })}
      </div>

      {/* 执行预览与即将触发时间 */}
      {/* Schedule preview and upcoming triggers */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='mb-2 flex items-center justify-between gap-2'>
          <Label className='text-xs font-medium'>{labels.schedulePreview}</Label>
          {footer}
        </div>
        {previewState.ok ? (
          <div className='flex flex-col gap-2.5 text-xs'>
            {/* 上半部分：Cron 表达式与可读描述 */}
            {/* Top section: Cron expression and human readable description */}
            <div className='rounded-md border border-border/50 bg-background/70 p-2.5'>
              <div className='font-mono text-xs font-semibold text-primary'>
                {expression || derivedExpression}
              </div>
              <div className='mt-1 text-muted-foreground leading-relaxed text-xs'>
                {previewState.text}
              </div>
            </div>

            {/* 下半部分：未来运行时间（单列平铺，避免窄栏挤压导致时间戳换行） */}
            {/* Bottom section: Next runs (single column to avoid squishing timestamps) */}
            <div className='min-w-0 space-y-1.5'>
              <div className='text-[11px] text-muted-foreground font-medium'>
                {labels.nextRuns}
              </div>
              <div className='space-y-1'>
                {previewState.nextRuns.map((run, index) => (
                  <div
                    key={`${run.toISOString()}-${index}`}
                    className='flex items-center justify-between rounded-md border border-border/50 bg-background/60 px-2.5 py-1 font-mono text-[11px] text-foreground/90'
                  >
                    <span>{formatDate(run)}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        ) : (
          <div className='text-xs text-destructive'>{previewState.error}</div>
        )}
      </div>
    </div>
  );
}
