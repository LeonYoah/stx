/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Heat Calendar (https://beui.dev/charts/heat-calendar).
 * Registry: https://beui.dev/r/heat-calendar.json
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

import {type ReactNode, useMemo, useState} from 'react';
import {NumberTicker} from '@/components/motion/number-ticker';
import {Tooltip} from '@/components/motion/tooltip';
import {cn} from '@/lib/utils';
import {useHeatCalendar} from './context';

export function HeatCalendarTooltip({
  children,
  className,
}: {
  children?:
    | ReactNode
    | ((
        data: NonNullable<ReturnType<typeof useHeatCalendar>['tooltip']>,
      ) => ReactNode);
  className?: string;
}) {
  const {gridRef, tooltipId, tip, tooltip, unit, t, formats} =
    useHeatCalendar();
  const [dismissed, setDismissed] = useState<typeof tip>(null);
  const anchorRef = useMemo(
    () => ({
      get current() {
        return tip
          ? (gridRef.current?.querySelector<HTMLElement>(
              `[data-heat-cell="${tip.w}-${tip.d}"]`,
            ) ?? null)
          : null;
      },
    }),
    [gridRef, tip],
  );
  return (
    <Tooltip
      key={tip ? `${tip.w}-${tip.d}` : 'closed'}
      open={tooltip !== null && dismissed !== tip}
      onOpenChange={(open) => {
        if (!open) {setDismissed(tip);}
      }}
      id={tooltipId}
      anchorRef={anchorRef}
      className={cn('flex flex-wrap items-center gap-1.5', className)}
      content={
        tooltip &&
        (typeof children === 'function'
          ? children(tooltip)
          : (children ?? (
              <>
                <span className='inline-flex items-center gap-1 font-mono tabular-nums'>
                  <NumberTicker
                    value={tooltip.days > 1 ? tooltip.total : tooltip.count}
                    duration={0.35}
                    startOnView={false}
                  />{' '}
                  {unit}
                </span>
                <span className='text-muted-foreground'>
                  {tooltip.days > 1 && tooltip.startDate && tooltip.endDate
                    ? t('range', {
                        start: formats.range.format(tooltip.startDate),
                        end: formats.range.format(tooltip.endDate),
                      })
                    : formats.day.format(tooltip.date)}
                </span>
                {tooltip.days > 1 ? (
                  <span className='text-muted-foreground'>
                    {t('days', {days: tooltip.days})}
                  </span>
                ) : null}
              </>
            )))
      }
    />
  );
}
