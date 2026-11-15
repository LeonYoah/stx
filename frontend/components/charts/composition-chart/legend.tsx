/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Composition Chart (https://beui.dev/charts/composition-chart).
 * Registry: https://beui.dev/r/composition-chart.json
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

import {cn} from '@/lib/utils';
import {useCompositionChart} from './context';

export function CompositionChartLegend({className}: {className?: string}) {
  const {
    column,
    highlight,
    pinned,
    setPinned,
    setHovered,
    setFocused,
    canHover,
    formatValue,
    t,
  } = useCompositionChart();
  if (!column) {return null;}
  return (
    <div
      className={cn(
        'grid min-w-0 grid-cols-2 gap-x-4 gap-y-1 @min-[480px]:grid-cols-3 @min-[900px]:grid-cols-6',
        className,
      )}
    >
      {column.segments.map((row) => (
        <button
          key={row.id}
          type='button'
          aria-label={t('highlight', {name: row.name})}
          aria-pressed={pinned === row.id}
          onClick={() => setPinned(pinned === row.id ? null : row.id)}
          onPointerEnter={() => {
            if (canHover) {setHovered(row.id);}
          }}
          onPointerLeave={() => setHovered(null)}
          onFocus={() => setFocused(row.id)}
          onBlur={() => setFocused(null)}
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              setPinned(null);
              setHovered(null);
              setFocused(null);
            }
          }}
          className={cn(
            'flex min-h-12 min-w-0 items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-xs transition-opacity duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
            pinned === row.id && 'bg-muted/60',
            highlight && highlight !== row.id && 'opacity-40',
          )}
        >
          <span
            aria-hidden='true'
            className='h-6 w-1 shrink-0 rounded-full'
            style={{backgroundColor: row.color}}
          />
          <span className='grid min-w-0 gap-0.5'>
            <span className='truncate text-muted-foreground'>{row.name}</span>
            <span
              title={
                row.value === null ? t('missingValue') : formatValue(row.value)
              }
              className='shrink-0 font-mono tabular-nums'
            >
              {column.valid ? `${row.share.toFixed(1)}%` : '—'}
            </span>
          </span>
        </button>
      ))}
    </div>
  );
}
