/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Sparkline dependency (https://beui.dev/charts/sparkline).
 * Registry: https://beui.dev/r/sparkline.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * Local STX adaptations remain under the same MIT terms and are not
 * relicensed as Apache-2.0.
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

import type {useMessages} from '@/components/charts/beui-i18n';

/**
 * Strings shared by LineChart, BarChart and Sparkline. English defaults;
 * override per instance through each chart's `labels`, or app-wide with
 * `LocaleProvider` under the `xyChart` namespace.
 */
export const xyChartMessages = {
  noData: 'No data',
  /** Inline value for a missing sample in spoken text. */
  missing: 'no data',
  dataCaption: '{label} data',
  category: 'Category',
  total: 'Total',
  series: 'Series',
  seriesValue: '{name} {value}',
  totalValue: 'Total {value}',
};

export type XYChartMessages = typeof xyChartMessages;

export type XYChartT = ReturnType<typeof useMessages<XYChartMessages>>;
