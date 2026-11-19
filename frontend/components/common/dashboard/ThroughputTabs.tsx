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

import {type CSSProperties, type KeyboardEvent, useRef} from 'react';
import {cn} from '@/lib/utils';
import type {RateSummary} from './dashboard-format';
import {DASHBOARD_CELL_DIVIDER} from './KpiStrip';

export const THROUGHPUT_KEYS = [
  'write',
  'read',
  'write_bytes',
  'read_bytes',
] as const;
export type ThroughputKey = (typeof THROUGHPUT_KEYS)[number];

/** 读用 --series-1、写用 --series-2，与作业详情趋势图同色。/ Read --series-1, write --series-2, matching job trends. */
export const THROUGHPUT_COLOR: Record<ThroughputKey, string> = {
  write: 'var(--series-2)',
  read: 'var(--series-1)',
  write_bytes: 'var(--series-2)',
  read_bytes: 'var(--series-1)',
};

export interface ThroughputTabItem {
  key: ThroughputKey;
  label: string;
  /** 窗口均值 / 峰值；序列为空时为 null，格子里显示「—」。/ Window avg/peak; null when the series is empty, the cell shows "—". */
  summary: RateSummary | null;
  color: string;
}

/**
 * 吞吐指标页签行：4 个指标格替代表单式分段控件，主数值是窗口均值（与
 * 「Top 8 · 窗口内平均」同口径，不和指标栏的实时合计打架），说明行给峰值。
 * roving tabindex + ←/→/Home/End（选择跟随焦点）。
 * Throughput metric tabs: four metric cells replace the segmented control; the
 * headline number is the window average (same basis as "Top 8 · window avg",
 * avoiding a mismatch with the KPI strip's realtime total) with the peak as
 * caption. Roving tabindex + arrow/Home/End keys (selection follows focus).
 */
export function ThroughputTabs({
  items,
  value,
  onValueChange,
  label,
  panelId,
  formatValue,
  peakLabel,
  valueHint,
}: {
  items: ThroughputTabItem[];
  value: ThroughputKey;
  onValueChange: (key: ThroughputKey) => void;
  /** tablist 的 aria-label。/ aria-label of the tablist. */
  label: string;
  /** 受控 tabpanel 的 id。/ id of the controlled tabpanel. */
  panelId: string;
  formatValue: (
    key: ThroughputKey,
    value: number,
  ) => {text: string; unit: string};
  /** 「峰值 {value}」。/ "Peak {value}". */
  peakLabel: (value: string) => string;
  /** 主数值口径提示，挂在格子 title 上。/ Headline-number basis hint, set as the cell's title. */
  valueHint: string;
}) {
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const activate = (index: number) => {
    const item = items[index];
    if (!item) {
      return;
    }
    onValueChange(item.key);
    tabRefs.current[index]?.focus();
  };

  const onKeyDown = (event: KeyboardEvent, index: number) => {
    let target: number | null = null;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        target = (index + 1) % items.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        target = (index - 1 + items.length) % items.length;
        break;
      case 'Home':
        target = 0;
        break;
      case 'End':
        target = items.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    activate(target);
  };

  return (
    // 外包圆角裁剪层：选中格的色条 / 背景会溢出面板左上、右上圆角，在这层裁掉；
    // tablist 的 -mr-px 让最右列右线同样被裁。不给整个吞吐面板加 overflow-hidden，
    // 否则会裁掉 BrushChart 的提示层。
    // An outer rounded clipper: the selected cell's color bar / background would
    // overflow the panel's top corners; the tablist's -mr-px lets it crop the
    // last column's right line too. The whole throughput panel must NOT get
    // overflow-hidden — it would clip BrushChart tooltips.
    <div className='overflow-hidden rounded-t-xl'>
      <div
        role='tablist'
        aria-label={label}
        className='-mr-px grid grid-cols-2 lg:grid-cols-4'
      >
        {items.map((item, index) => {
          const selected = item.key === value;
          const avg = item.summary
            ? formatValue(item.key, item.summary.avg)
            : null;
          const peak = item.summary
            ? formatValue(item.key, item.summary.peak)
            : null;
          return (
            <button
              key={item.key}
              ref={(node) => {
                tabRefs.current[index] = node;
              }}
              type='button'
              role='tab'
              aria-selected={selected}
              aria-controls={panelId}
              tabIndex={selected ? 0 : -1}
              title={valueHint}
              data-testid={`home-throughput-tab-${item.key}`}
              onClick={() => onValueChange(item.key)}
              onKeyDown={(event) => onKeyDown(event, index)}
              className={cn(
                DASHBOARD_CELL_DIVIDER,
                'relative px-4 py-3 text-left transition-colors',
                'focus-visible:bg-muted/30',
                'aria-selected:bg-muted/50',
              )}
              // 焦点框向内收 2px，避免被圆角裁剪层切掉；foundation 的 :focus-visible
              // outline 带 !important，不要再用 outline-none / ring 叠加第二个框。
              // Pull the keyboard focus outline 2px inward so the rounded clipper
              // does not clip it; foundation's :focus-visible outline is
              // !important, so do not stack a second frame via ring/outline-none.
              style={
                {'--focus-outline-offset': '-2px'} as CSSProperties
              }
            >
              {selected ? (
                <span
                  aria-hidden
                  className='absolute inset-x-0 top-0 h-0.5'
                  style={{background: item.color}}
                />
              ) : null}
              <span className='block text-xs text-muted-foreground'>
                {item.label}
              </span>
              <span className='mt-1 flex items-baseline gap-0.5 text-xl font-medium tabular-nums'>
                {avg ? (
                  <>
                    {avg.text}
                    {avg.unit ? (
                      <small className='text-xs text-muted-foreground'>
                        {avg.unit}
                      </small>
                    ) : null}
                  </>
                ) : (
                  '—'
                )}
              </span>
              <span className='mt-0.5 block text-xs text-muted-foreground'>
                {peak ? peakLabel(`${peak.text} ${peak.unit}`.trim()) : ''}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
