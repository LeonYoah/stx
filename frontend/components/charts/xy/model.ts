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

export interface XYSeries {
  /** Stable, unique identity. Keep it when updating values so marks morph in place. */
  id: string;
  name: string;
  /** Any CSS color, including theme variables. */
  color: string;
  /** Values aligned with categories. Missing or non-finite values leave a gap. */
  values: readonly (number | null)[];
}

export type XYRow = Omit<XYSeries, 'values'> & {values: (number | null)[]};

/** First of repeated series and category ids wins. Invalid values become gaps, never zeros. */
export function normalizeXY(
  series: readonly XYSeries[],
  categories: readonly string[],
  {nonnegative = false}: {nonnegative?: boolean} = {},
) {
  const columns: {id: string; source: number}[] = [];
  const seenColumns = new Set<string>();
  categories.forEach((id, source) => {
    if (seenColumns.has(id)) {return;}
    seenColumns.add(id);
    columns.push({id, source});
  });
  const seenRows = new Set<string>();
  const rows: XYRow[] = [];
  for (const {values, ...row} of series) {
    if (seenRows.has(row.id)) {continue;}
    seenRows.add(row.id);
    rows.push({
      ...row,
      values: columns.map(({source}) => {
        const value = values[source];
        return typeof value === 'number' &&
          Number.isFinite(value) &&
          (!nonnegative || value >= 0)
          ? value
          : null;
      }),
    });
  }
  return {rows, categories: columns.map((column) => column.id)};
}
