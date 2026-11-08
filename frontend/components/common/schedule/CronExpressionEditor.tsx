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

import ChipGroup from '@/components/arc/chip-group/chip-group';
import {Input} from '@/components/arc/input/input';
import {Input as CompactInput} from '@/components/ui/input';

import {
  cronToText,
  expressionFromParts,
  getNextRuns,
  parseExpressionToParts,
  type CronTranslator,
} from '@/components/common/sync/task-schedule-cron';
import {formatDateTimeTz, resolveCronTimezone} from '@/lib/timezone';

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
  cronPresets?: string;
}

function formatDate(date: Date, timeZone?: string) {
  return formatDateTimeTz(date, {timeZone: resolveCronTimezone(timeZone)});
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
            <span
              title={fullLabel}
              className='block truncate text-[10px] font-medium text-muted-foreground whitespace-nowrap'
            >
              {fullLabel}
            </span>
            <CompactInput
              aria-label={fullLabel}
              value={parts[key]}
              onChange={(e) => setParts((prev) => ({...prev, [key]: e.target.value}))}
              className='h-9 rounded-[var(--radius-control)] px-1 text-center font-mono text-xs'
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
        nextRuns: getNextRuns(
          expression || derivedExpression,
          5,
          new Date(),
          resolveCronTimezone(timezone),
        ),
      } as const;
    } catch (error) {
      return {
        ok: false,
        error: error instanceof Error ? error.message : labels.invalidCronExpression,
      } as const;
    }
  }, [derivedExpression, expression, labels.invalidCronExpression, timezone, translator]);

  const selectedPreset = presets.find(
    (preset) => (expression || derivedExpression) === preset.expr,
  );

  return (
    <div className='space-y-3'>
      {onTimezoneChange ? (
        <Input
          label={labels.timezone}
          value={timezone || ''}
          onChange={(e) => onTimezoneChange(e.target.value)}
          placeholder={labels.cronTimezonePlaceholder}
        />
      ) : null}

      <Input
        label={labels.cronExpression}
        description={labels.cronFiveField}
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
        className='font-mono'
        placeholder={labels.cronExpressionPlaceholder}
      />
      {helper ? (
        <div className='text-xs text-muted-foreground'>{helper}</div>
      ) : null}

      <BuilderFields
        parts={parts}
        setParts={(next) => {
          const resolved = typeof next === 'function' ? next(parts) : next;
          setParts(resolved);
          onExpressionChange(expressionFromParts(resolved));
        }}
        labels={labels}
      />

      {/* 预设用 Chip 单选，侧栏里全部摊开，避免溢出芯片写成英文 +N more。 */}
      {/* Presets are a single ChipGroup. Show every chip so the overflow control never says "+N more". */}
      {presets.length > 0 ? (
        <ChipGroup
          multiple={false}
          label={labels.cronPresets ?? labels.cronExpression}
          value={selectedPreset ? [selectedPreset.key] : []}
          onValueChange={(next) => {
            const preset = presets.find((item) => item.key === next[0]);
            if (!preset) {
              return;
            }
            setParts(parseExpressionToParts(preset.expr));
            onExpressionChange(preset.expr);
          }}
          options={presets.map((preset) => ({
            value: preset.key,
            label: renderPresetLabel(preset.key),
          }))}
        />
      ) : null}

      {/* 预览只留表达式、说明和下次运行，不再套三层边框。 */}
      {/* Preview keeps the expression, the prose, and next runs. No nested cards. */}
      <div className='space-y-2'>
        <div className='flex items-center justify-between gap-2'>
          <span className='text-xs font-medium text-foreground'>
            {labels.schedulePreview}
          </span>
          {footer}
        </div>
        {previewState.ok ? (
          <div className='space-y-2 text-xs'>
            <div>
              <div className='font-mono text-xs text-foreground'>
                {expression || derivedExpression}
              </div>
              <div className='mt-1 leading-relaxed text-muted-foreground'>
                {previewState.text}
              </div>
            </div>
            <div className='min-w-0 space-y-1'>
              <div className='text-[11px] font-medium text-muted-foreground'>
                {labels.nextRuns}
              </div>
              {previewState.nextRuns.map((run, index) => (
                <div
                  key={`${run.toISOString()}-${index}`}
                  className='overflow-x-auto whitespace-nowrap font-mono text-[11px] tabular-nums text-foreground/90'
                >
                  {formatDate(run, timezone)}
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className='text-xs text-destructive'>{previewState.error}</div>
        )}
      </div>
    </div>
  );
}
