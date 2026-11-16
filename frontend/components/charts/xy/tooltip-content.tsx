/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Bar Chart (https://beui.dev/charts/bar-chart).
 * Registry: https://beui.dev/r/bar-chart.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * 本地改动：beUI 的 lib/i18n 指向 components/charts/beui-i18n.tsx，不占用 STX next-intl 的 frontend/lib/i18n/。
 * Local change: beUI's lib/i18n points at components/charts/beui-i18n.tsx so it does not shadow STX's next-intl
 * frontend/lib/i18n/. Local STX adaptations remain under the same MIT terms and are not relicensed as Apache-2.0.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

'use client';

import {MotionConfig} from 'motion/react';
import {NumberTicker} from '@/components/motion/number-ticker';
import {cn} from '@/lib/utils';
import type {XYChartT} from './messages';

export interface XYTooltipRow {
  id: string;
  name: string;
  color: string;
  value: number | null;
}

function Value({
  value,
  format,
}: {
  value: number;
  format: (value: number) => string;
}) {
  return (
    // Keep the short digit tween when composed inside spring-based Tabs.
    <MotionConfig transition={{type: 'tween'}}>
      <NumberTicker
        value={value}
        // Keep the consumer's exact formatting, including fractions and units.
        format={() => format(value)}
        startOnView={false}
        duration={0.2}
        stagger={0}
        className='whitespace-pre'
      />
    </MotionConfig>
  );
}

export function XYTooltipContent({
  title,
  rows,
  formatValue,
  total,
  compact = false,
  t,
  className,
}: {
  title: string;
  rows: readonly XYTooltipRow[];
  formatValue: (value: number) => string;
  total?: number | null;
  /** Size to the content instead of a fixed panel width, for pointer-following readouts. */
  compact?: boolean;
  t: XYChartT;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'block max-w-full min-w-0',
        compact ? 'min-w-36' : 'w-56',
        className,
      )}
    >
      <span
        className={cn('block text-xs font-medium', compact ? 'mb-1.5' : 'mb-2')}
      >
        {title}
      </span>
      <span className={cn('grid', compact ? 'gap-1' : 'gap-1.5')}>
        {rows.map((row) => (
          <span key={row.id} className='flex items-center gap-2 text-[11px]'>
            <span
              aria-hidden='true'
              className='h-2.5 w-1 shrink-0 rounded-full'
              style={{backgroundColor: row.color}}
            />
            <span className='min-w-0 flex-1 truncate pr-3 text-muted-foreground'>
              {row.name}
            </span>
            <span className='shrink-0 font-mono tabular-nums'>
              {row.value === null ? (
                t('noData')
              ) : (
                <Value value={row.value} format={formatValue} />
              )}
            </span>
          </span>
        ))}
        {total !== undefined && total !== null ? (
          <span className='mt-0.5 flex items-center gap-2 border-t border-border pt-1.5 text-[11px]'>
            <span className='flex-1 text-muted-foreground'>{t('total')}</span>
            <span className='font-mono font-medium tabular-nums'>
              <Value value={total} format={formatValue} />
            </span>
          </span>
        ) : null}
      </span>
    </span>
  );
}
