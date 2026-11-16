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

export interface CompositionSeries {
  id: string;
  name: string;
  color: string;
  /** Nonnegative values aligned with periods. Missing/invalid values leave a gap. */
  values: readonly (number | null)[];
}

export function buildComposition(
  series: readonly CompositionSeries[],
  periods: readonly string[],
) {
  const seen = new Set<string>();
  const rows = series.filter((row) => {
    if (seen.has(row.id)) {return false;}
    seen.add(row.id);
    return true;
  });
  const periodIds = new Set<string>();
  const columns = periods.flatMap((id, index) => {
    if (periodIds.has(id)) {return [];}
    periodIds.add(id);
    const values = rows.map((row) => row.values[index]);
    const valid =
      values.length > 0 &&
      values.every(
        (v) => typeof v === 'number' && Number.isFinite(v) && v >= 0,
      );
    // Scale before summing so large finite input values cannot overflow shares.
    const max = valid ? Math.max(0, ...values.map((v) => v ?? 0)) : 0;
    const scaledTotal =
      max > 0 ? values.reduce<number>((sum, v) => sum + (v ?? 0) / max, 0) : 0;
    let offset = 0;
    const segments = rows.map((row, i) => {
      const value = values[i] ?? null;
      const share =
        scaledTotal > 0 ? ((value ?? 0) / max / scaledTotal) * 100 : 0;
      const segment = {...row, value, share, offset};
      offset += share;
      return segment;
    });
    return [{id, valid: valid && scaledTotal > 0, segments}];
  });
  return {rows, columns};
}

/** Separate polygons for contiguous runs; unknown periods never become interpolated data. */
export function compositionArea(
  columns: ReturnType<typeof buildComposition>['columns'],
  row: number,
) {
  const paths: string[] = [];
  let run: number[] = [];
  const flush = () => {
    if (!run.length) {return;}
    const top = run.map((index) => {
      const segment = columns[index].segments[row];
      return `${((index + 0.5) / columns.length) * 100},${100 - segment.offset - segment.share}`;
    });
    const bottom = [...run]
      .reverse()
      .map(
        (index) =>
          `${((index + 0.5) / columns.length) * 100},${100 - columns[index].segments[row].offset}`,
      );
    // A single sample gets a column-width footprint instead of an invisible polygon.
    if (run.length === 1) {
      const index = run[0];
      const segment = columns[index].segments[row];
      const left = (index / columns.length) * 100;
      const right = ((index + 1) / columns.length) * 100;
      paths.push(
        `M${left},${100 - segment.offset} L${left},${100 - segment.offset - segment.share} L${right},${100 - segment.offset - segment.share} L${right},${100 - segment.offset} Z`,
      );
    } else {paths.push(`M${top.join(' L')} L${bottom.join(' L')} Z`);}
    run = [];
  };
  columns.forEach((column, index) => {
    if (column.valid) {run.push(index);}
    else {flush();}
  });
  flush();
  return paths.join(' ');
}
