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

import type {ReactNode} from 'react';
import {Tooltip} from '@/components/motion/tooltip';
import {cn} from '@/lib/utils';
import {XYTooltipContent, type XYTooltipRow} from '../xy/tooltip-content';
import {useBarChart} from './context';

export interface BarChartTooltipData {
  category: string;
  index: number;
  rows: XYTooltipRow[];
  total: number;
}

export interface BarChartTooltipProps {
  children?: ReactNode | ((data: BarChartTooltipData) => ReactNode);
  className?: string;
  side?: 'top' | 'right' | 'bottom' | 'left';
}

/**
 * Follows the pointer like the other beUI charts' readouts. Keyboard
 * inspection, which has no pointer, anchors at the end of the category's
 * longest bar instead.
 */
export function BarChartTooltip({
  children,
  className,
  side = 'top',
}: BarChartTooltipProps) {
  const {
    categories,
    visibleRows,
    order,
    totals,
    domain,
    layout,
    orientation,
    activeIndex,
    open,
    setOpen,
    plotRef,
    tooltipId,
    formatValue,
    xy,
  } = useBarChart();
  const category = categories[activeIndex];
  const rows: XYTooltipRow[] = visibleRows.map((row) => ({
    id: row.id,
    name: row.name,
    color: row.color,
    value: row.values[activeIndex] ?? null,
  }));
  const total = totals[activeIndex] ?? 0;
  const reach =
    layout === 'stacked'
      ? total
      : Math.max(0, ...rows.map((row) => row.value ?? 0));
  const along = Math.min(1, reach / (domain.max || 1));
  const across =
    (Math.max(order.indexOf(activeIndex), 0) + 0.5) /
    Math.max(categories.length, 1);
  return (
    <Tooltip
      id={tooltipId}
      anchorRef={plotRef}
      followCursor
      anchorPoint={
        orientation === 'vertical'
          ? {x: across, y: 1 - along}
          : {x: along, y: across}
      }
      open={open && category !== undefined}
      onOpenChange={setOpen}
      side={side}
      className={cn('max-w-64', className)}
      content={
        category === undefined
          ? null
          : typeof children === 'function'
            ? children({category, index: activeIndex, rows, total})
            : (children ?? (
                <XYTooltipContent
                  title={category}
                  rows={rows}
                  formatValue={formatValue}
                  total={
                    layout === 'stacked' && rows.length > 1 ? total : undefined
                  }
                  compact
                  t={xy}
                />
              ))
      }
    />
  );
}
