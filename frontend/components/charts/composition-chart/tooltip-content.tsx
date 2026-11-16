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

import {NumberTicker} from '@/components/motion/number-ticker';
import {cn} from '@/lib/utils';
import {useCompositionChart} from './context';

export function CompositionTooltipContent({className}: {className?: string}) {
  const {column, formatValue, t} = useCompositionChart();
  if (!column) {return null;}
  return (
    <span className={cn('block w-72 max-w-full min-w-0', className)}>
      <span className='mb-2 block text-xs font-medium'>{column.id}</span>
      {column.valid ? (
        <span className='grid gap-2'>
          {column.segments.map((row) => (
            <span key={row.id} className='flex items-center gap-2 text-[11px]'>
              <span
                aria-hidden='true'
                className='size-1.5 shrink-0 rounded-full'
                style={{backgroundColor: row.color}}
              />
              <span className='min-w-0 flex-1 truncate'>{row.name}</span>
              <span
                className='max-w-28 shrink-0 truncate text-muted-foreground tabular-nums'
                title={formatValue(row.value ?? 0)}
              >
                <NumberTicker
                  value={row.value ?? 0}
                  // Keep the consumer's exact formatting, including fractional values and units.
                  format={() => formatValue(row.value ?? 0)}
                  startOnView={false}
                  duration={0.2}
                  stagger={0}
                  className='whitespace-pre'
                />
              </span>
              <span className='w-12 shrink-0 text-right font-mono tabular-nums'>
                <NumberTicker
                  value={row.share}
                  format={() => row.share.toFixed(1)}
                  suffix='%'
                  startOnView={false}
                  duration={0.2}
                  stagger={0}
                />
              </span>
            </span>
          ))}
        </span>
      ) : (
        <span className='block text-xs text-muted-foreground'>
          {t('incomplete')}
        </span>
      )}
    </span>
  );
}
