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

import {cn} from '@/lib/utils';
import type {XYRow} from './model';

export interface XYLegendProps {
  rows: readonly XYRow[];
  hidden: readonly string[];
  onToggle: (id: string) => void;
  onHover: (id: string | null) => void;
  canHover: boolean;
  label: string;
  className?: string;
}

/** Each entry is a toggle: pressed means the series is drawn. */
export function XYLegend({
  rows,
  hidden,
  onToggle,
  onHover,
  canHover,
  label,
  className,
}: XYLegendProps) {
  return (
    <ul
      aria-label={label}
      className={cn(
        'flex flex-wrap items-center gap-x-1 gap-y-1 text-xs',
        className,
      )}
    >
      {rows.map((row) => {
        const shown = !hidden.includes(row.id);
        return (
          <li key={row.id}>
            <button
              type='button'
              aria-pressed={shown}
              onClick={() => onToggle(row.id)}
              onPointerEnter={() => {
                if (canHover && shown) {onHover(row.id);}
              }}
              onPointerLeave={() => onHover(null)}
              onFocus={() => {
                if (shown) {onHover(row.id);}
              }}
              onBlur={() => onHover(null)}
              className={cn(
                'flex min-h-9 items-center gap-2 rounded-md px-2 text-muted-foreground transition-[color,opacity] duration-150 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                !shown && 'opacity-50',
              )}
            >
              <span
                aria-hidden='true'
                className={cn(
                  'size-2.5 shrink-0 rounded-full border-2 transition-colors duration-150',
                )}
                style={{
                  borderColor: row.color,
                  backgroundColor: shown ? row.color : 'transparent',
                }}
              />
              <span
                className={cn(
                  !shown && 'line-through decoration-muted-foreground/60',
                )}
              >
                {row.name}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}
