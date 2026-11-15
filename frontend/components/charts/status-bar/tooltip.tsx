/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Status Bar (https://beui.dev/charts/status-bar).
 * Registry: https://beui.dev/r/status-bar.json
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
import {
  type StatusBarDatum,
  type StatusBarStatus,
  useStatusBar,
} from './context';

export interface StatusBarTooltipData {
  datum: StatusBarDatum;
  status: StatusBarStatus;
  index: number;
}

export interface StatusBarTooltipProps {
  children?: ReactNode | ((data: StatusBarTooltipData) => ReactNode);
  className?: string;
  side?: 'top' | 'right' | 'bottom' | 'left';
}

export function StatusBarTooltip({
  children,
  className,
  side = 'top',
}: StatusBarTooltipProps) {
  const {
    active,
    activeIndex,
    rows,
    plotRef,
    tooltipId,
    tooltipOpen,
    setTooltipOpen,
  } = useStatusBar();
  return (
    <Tooltip
      id={tooltipId}
      anchorRef={plotRef}
      anchorPoint={{
        x: (Math.max(activeIndex, 0) + 0.5) / Math.max(rows.length, 1),
        y: 0.1,
      }}
      open={tooltipOpen && active !== null}
      onOpenChange={setTooltipOpen}
      side={side}
      className={cn('max-w-64', className)}
      content={
        active
          ? typeof children === 'function'
            ? children({...active, index: activeIndex})
            : (children ?? (
                <span className='flex flex-col gap-1'>
                  <span className='text-muted-foreground'>
                    {active.datum.label}
                  </span>
                  <span className='flex items-center gap-2 font-medium'>
                    <span
                      aria-hidden='true'
                      className='size-1.5 shrink-0 rounded-full'
                      style={{backgroundColor: active.status.color}}
                    />
                    {active.status.label}
                  </span>
                  {active.datum.description ? (
                    <span className='text-muted-foreground'>
                      {active.datum.description}
                    </span>
                  ) : null}
                </span>
              ))
          : null
      }
    />
  );
}
