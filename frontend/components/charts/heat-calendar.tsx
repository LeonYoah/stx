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

// beui.dev/charts/heat-calendar

import {cn} from '@/lib/utils';
import {
  HeatCalendarContext,
  useHeatCalendarModel,
} from './heat-calendar/context';
import {HeatCalendarGrid} from './heat-calendar/grid';
import {HeatCalendarLegend} from './heat-calendar/legend';
import {HeatCalendarTooltip} from './heat-calendar/tooltip';
import type {HeatCalendarProps} from './heat-calendar/types';

/** Compose Grid, Tooltip and Legend, or omit children for the complete chart. */
export function HeatCalendar({
  children,
  className,
  ...props
}: HeatCalendarProps) {
  const model = useHeatCalendarModel(props);
  return (
    <HeatCalendarContext.Provider value={model}>
      <div className={cn('w-fit max-w-full', className)}>
        {children === undefined ? (
          <>
            <HeatCalendarGrid>
              <HeatCalendarTooltip />
            </HeatCalendarGrid>
            <HeatCalendarLegend />
          </>
        ) : (
          children
        )}
      </div>
    </HeatCalendarContext.Provider>
  );
}

export {useHeatCalendar} from './heat-calendar/context';
export {HeatCalendarGrid} from './heat-calendar/grid';
export {HeatCalendarLegend} from './heat-calendar/legend';
export {
  type HeatCalendarMessages,
  heatCalendarMessages,
} from './heat-calendar/messages';
export {HeatCalendarTooltip} from './heat-calendar/tooltip';
export type {
  HeatCalendarCell,
  HeatCalendarProps,
  HeatCalendarSelection,
} from './heat-calendar/types';
