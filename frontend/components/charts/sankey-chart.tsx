/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Sankey Chart (https://beui.dev/charts/sankey-chart).
 * Registry: https://beui.dev/r/sankey-chart.json
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

// beui.dev/charts/sankey-chart

import {cn} from '@/lib/utils';
import {
  type SankeyChartProps,
  SankeyContext,
  useSankeyModel,
} from './sankey-chart/context';
import {SankeyChartPlot} from './sankey-chart/plot';
import {SankeyChartTooltip} from './sankey-chart/tooltip';

/**
 * Weighted flows between nodes, laid out in columns from sources to outcomes.
 * Compose Plot and Tooltip, or omit children for the complete chart.
 *
 * ```tsx
 * <SankeyChart nodes={nodes} links={links} formatValue={usd}>
 *   <SankeyChartPlot />
 *   <SankeyChartTooltip />
 * </SankeyChart>
 * ```
 */
export function SankeyChart({
  nodes,
  links,
  label,
  formatValue,
  colors,
  nodeWidth,
  nodePadding,
  activeId,
  defaultActiveId,
  onActiveChange,
  labels,
  className,
  children,
  ...props
}: SankeyChartProps) {
  const model = useSankeyModel({
    nodes,
    links,
    label,
    formatValue,
    colors,
    nodeWidth,
    nodePadding,
    activeId,
    defaultActiveId,
    onActiveChange,
    labels,
  });
  const {t} = model;
  return (
    <SankeyContext.Provider value={model}>
      <section
        aria-label={model.label}
        {...props}
        data-slot='sankey-chart'
        className={cn('w-full min-w-0', className)}
      >
        {children === undefined ? (
          <>
            <SankeyChartPlot />
            <SankeyChartTooltip />
          </>
        ) : (
          children
        )}
        <div className='sr-only'>
          <table>
            <caption>{t('tableCaption', {label: model.label})}</caption>
            <thead>
              <tr>
                <th scope='col'>{t('flow')}</th>
                <th scope='col'>{t('value')}</th>
                <th scope='col'>{t('shareOfSource')}</th>
              </tr>
            </thead>
            <tbody>
              {model.flows.map((flow) => (
                <tr key={flow.id}>
                  <th scope='row'>
                    {t('flowRow', {source: flow.source, target: flow.target})}
                  </th>
                  <td>{model.formatValue(flow.value)}</td>
                  <td>
                    {flow.share ? `${(flow.share * 100).toFixed(1)}%` : ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </SankeyContext.Provider>
  );
}

export {
  type SankeyActive,
  type SankeyChartProps,
  useSankeyChart,
} from './sankey-chart/context';
export {
  type SankeyChartMessages,
  sankeyChartMessages,
} from './sankey-chart/messages';
export type {
  PlacedLink as SankeyPlacedLink,
  PlacedNode as SankeyPlacedNode,
  SankeyLink,
  SankeyNode,
} from './sankey-chart/model';
export {SankeyChartPlot, type SankeyChartPlotProps} from './sankey-chart/plot';
export {
  SankeyChartTooltip,
  type SankeyChartTooltipProps,
} from './sankey-chart/tooltip';
