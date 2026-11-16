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

import {cn} from '@/lib/utils';
import {useHeatCalendar} from './context';
import {STEPS} from './utils';

export function HeatCalendarLegend({className}: {className?: string}) {
  const {start, end, step, setStep, fill, canHover, reduce, t, formats} =
    useHeatCalendar();
  return (
    <div
      className={cn(
        'mt-3 flex flex-wrap items-center justify-between gap-3',
        className,
      )}
    >
      <span className='text-xs text-muted-foreground'>
        {start && end
          ? t('range', {
              start: formats.range.format(start),
              end: formats.range.format(end),
            })
          : '\u00a0'}
      </span>
      {/* hovering a step keeps only cells of that level lit, so the legend doubles as a filter */}
      <span
        className='flex items-center gap-1'
        onPointerLeave={() => setStep(null)}
      >
        <span className='mr-0.5 text-xs text-muted-foreground'>
          {t('less')}
        </span>
        {STEPS.map((s, i) => (
          <button
            type='button'
            aria-label={t('level', {level: i})}
            aria-pressed={step === i}
            key={s}
            onPointerEnter={() => {
              if (canHover) {setStep(i);}
            }}
            onFocus={() => setStep(i)}
            onBlur={() => setStep(null)}
            onClick={() => setStep(step === i ? null : i)}
            className='size-3 rounded-[3px] transition-transform duration-150'
            style={{
              background: fill(i),
              transform: !reduce && step === i ? 'scale(1.25)' : undefined,
            }}
          />
        ))}
        <span className='ml-0.5 text-xs text-muted-foreground'>
          {t('more')}
        </span>
      </span>
    </div>
  );
}
