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

import Link from 'next/link';
import type {CSSProperties} from 'react';
import {ArrowDownRight, ArrowUpRight} from 'lucide-react';
import {AnimatedCounter} from '@/components/arc/animated-counter/animated-counter';
import {Skeleton} from '@/components/ui/skeleton';
import {cn} from '@/lib/utils';

export type KpiTone = 'danger' | 'warning';

export interface KpiStripItem {
  /** 同时用于 data-testid：home-kpi-${key}。/ Doubles as the data-testid suffix. */
  key: string;
  /** 整格跳转目标。/ Whole-cell link target. */
  href: string;
  label: string;
  /** 已按量级缩放后的数值（scaledCompact().value）。/ Value already scaled by scaledCompact(). */
  value: number;
  decimals?: number;
  /** 量级：K / M / B…，和数字同字号。/ Magnitude suffix (K / M / B…), same size as the digits. */
  scale?: string;
  /** 计量单位：行/秒、秒、行、/ 3，小号次要色。/ Measure unit (rows/s, s, rows, / 3), small muted text. */
  unit?: string;
  /** 说明，只显示一行。/ One-line caption. */
  context: string;
  /** 只有写入速率用：涨跌百分比与方向。/ Write rate only: signed change and direction. */
  change?: {percent: number; good: boolean};
  /** 数值染色。/ Value tint. */
  tone?: KpiTone;
}

/**
 * 单元格分隔线：每格画右 + 下两条 1px inset 线，容器外扩对应负边距，
 * 外圈线被底板 overflow-hidden 裁掉。换行处的分隔始终连续，末行空位露底板色。
 * （不用「左 + 上」线：末行短缺处上一格会缺底线，4 列 / 2 列换行时分隔断开。）
 * Cell divider: each cell draws a 1px inset line on its right and bottom edges,
 * the grid compensates with matching negative margins and the board's
 * overflow-hidden clips the outer ring. Dividers stay continuous across wraps
 * and trailing empty slots show the board color. (Left+top lines leave the cell
 * above a short last row without a bottom edge, breaking the grid at 4/2 columns.)
 * KpiStrip 与 ThroughputTabs 共用。/ Shared by KpiStrip and ThroughputTabs.
 */
export const DASHBOARD_CELL_DIVIDER =
  'shadow-[inset_-1px_-1px_0_var(--border)]';

const KPI_NUMBER_STYLE = {'--text-3xl': '1.5rem'} as CSSProperties;
const TONE_FOREGROUND: Record<KpiTone, CSSProperties> = {
  danger: {'--foreground': 'var(--danger)'} as CSSProperties,
  warning: {'--foreground': 'var(--warning)'} as CSSProperties,
};

/**
 * 首页顶部指标栏：一个底板 7 个单元格，格间只有 1px 分隔线；每格整格可点击。
 * 数字经 AnimatedCounter 滚动刷新；染色只加在数字包裹层（局部 --foreground），
 * 标签与说明保持次要色。链接可读名称就是格子文字，不另写 aria-label。
 * The home KPI strip: one board with seven cells separated only by 1px lines,
 * each cell a full-area link. Values roll through AnimatedCounter; the tone
 * override only wraps the number (--foreground scoped), so the label and
 * caption stay muted. The link's accessible name is the cell text itself.
 */
export function KpiStrip({
  items,
  label,
}: {
  items: KpiStripItem[];
  /** nav 的 aria-label。/ aria-label for the nav landmark. */
  label: string;
}) {
  return (
    <nav aria-label={label} data-testid='home-kpi-row'>
      <div className='overflow-hidden rounded-xl border border-border/70 bg-card'>
        <ul className='-mr-px -mb-px grid grid-cols-1 min-[480px]:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'>
          {items.map((item) => (
            <li
              key={item.key}
              className={DASHBOARD_CELL_DIVIDER}
              data-tone={item.tone}
            >
              <Link
                href={item.href}
                data-testid={`home-kpi-${item.key}`}
                className='block h-full px-4 py-3 transition-colors hover:bg-muted/40'
                // 焦点框向内收 2px，避免被底板 overflow-hidden 裁掉。
                // Pull the keyboard focus outline 2px inward so the board's
                // overflow-hidden does not clip it.
                style={
                  {'--focus-outline-offset': '-2px'} as CSSProperties
                }
              >
                <span className='block truncate text-xs text-muted-foreground'>
                  {item.label}
                </span>
                <span
                  className='mt-1.5 flex items-baseline gap-0.5'
                  style={
                    item.tone
                      ? {...KPI_NUMBER_STYLE, ...TONE_FOREGROUND[item.tone]}
                      : KPI_NUMBER_STYLE
                  }
                >
                  <AnimatedCounter
                    value={item.value}
                    decimals={item.decimals ?? 0}
                    suffix={item.scale ?? ''}
                    animateOnView
                  />
                  {item.unit ? (
                    <span className='text-xs text-muted-foreground'>
                      {item.unit}
                    </span>
                  ) : null}
                </span>
                <span className='mt-1 flex items-center gap-1 text-xs text-muted-foreground'>
                  {item.change ? (
                    <span
                      className={cn(
                        'inline-flex shrink-0 items-center gap-0.5 tabular-nums',
                        item.change.good
                          ? 'text-[var(--success)]'
                          : 'text-destructive',
                      )}
                    >
                      {item.change.good ? (
                        <ArrowUpRight className='size-3' aria-hidden='true' />
                      ) : (
                        <ArrowDownRight
                          className='size-3'
                          aria-hidden='true'
                        />
                      )}
                      {`${item.change.percent >= 0 ? '+' : '−'}${Math.abs(item.change.percent)}%`}
                    </span>
                  ) : null}
                  <span className='truncate' title={item.context}>
                    {item.context}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </nav>
  );
}

/**
 * 指标栏骨架：同一底板与网格，每格三条骨架，高度与真实指标栏一致，不塌陷。
 * KPI strip skeleton: same board and grid with three bars per cell, matching
 * the real strip's height so nothing collapses.
 */
export function KpiStripSkeleton({count}: {count: number}) {
  return (
    <div
      className='overflow-hidden rounded-xl border border-border/70 bg-card'
      data-testid='home-kpi-loading'
    >
      <ul className='-mr-px -mb-px grid grid-cols-1 min-[480px]:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'>
        {Array.from({length: count}).map((_, index) => (
          <li
            key={index}
            className={cn(DASHBOARD_CELL_DIVIDER, 'px-4 py-3')}
          >
            <Skeleton className='h-3 w-16' />
            <Skeleton className='mt-1.5 h-6 w-20' />
            <Skeleton className='mt-1 h-3 w-24' />
          </li>
        ))}
      </ul>
    </div>
  );
}
