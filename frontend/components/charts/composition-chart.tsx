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

// beui.dev/charts/composition-chart

import {cn} from '@/lib/utils';
import {
  type CompositionChartProps,
  CompositionContext,
  useCompositionModel,
} from './composition-chart/context';
import {CompositionChartLegend} from './composition-chart/legend';
import {CompositionChartPlot} from './composition-chart/plot';

/** Normalized stacked shares. Zero-total or incomplete periods are shown as gaps. */
export function CompositionChart({
  className,
  children,
  ...props
}: CompositionChartProps) {
  const model = useCompositionModel(props);
  return (
    <CompositionContext.Provider value={model}>
      <section
        aria-label={model.label}
        className={cn('@container w-full space-y-4', className)}
      >
        {children === undefined ? (
          <div className='grid gap-4'>
            <CompositionChartPlot />
            <CompositionChartLegend />
          </div>
        ) : (
          children
        )}
      </section>
    </CompositionContext.Provider>
  );
}

export type {CompositionChartProps} from './composition-chart/context';
export {useCompositionChart} from './composition-chart/context';
export {CompositionChartLegend} from './composition-chart/legend';
export {
  type CompositionChartMessages,
  compositionChartMessages,
} from './composition-chart/messages';
export type {CompositionSeries as CompositionChartSeries} from './composition-chart/model';
export {CompositionChartPlot} from './composition-chart/plot';
