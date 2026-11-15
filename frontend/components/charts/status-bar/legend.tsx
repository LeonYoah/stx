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

import type {ComponentProps} from 'react';
import {cn} from '@/lib/utils';
import {useStatusBar} from './context';

export interface StatusBarLegendProps extends ComponentProps<'ul'> {
  showCounts?: boolean;
}

export function StatusBarLegend({
  className,
  showCounts = false,
  children,
  ...props
}: StatusBarLegendProps) {
  const {statuses, rows, t} = useStatusBar();
  return (
    <ul
      aria-label={t('legend')}
      {...props}
      data-slot='status-bar-legend'
      className={cn(
        'flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-muted-foreground',
        className,
      )}
    >
      {children ??
        statuses.map((status) => (
          <li key={status.id} className='flex items-center gap-1.5'>
            <span
              aria-hidden='true'
              className='h-3 w-1.5 rounded-full'
              style={{backgroundColor: status.color}}
            />
            <span>{status.label}</span>
            {showCounts ? (
              <span className='tabular-nums'>
                ({rows.filter((row) => row.status.id === status.id).length})
              </span>
            ) : null}
          </li>
        ))}
    </ul>
  );
}
