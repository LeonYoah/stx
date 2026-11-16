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

export interface XYDomain {
  min: number;
  max: number;
  ticks: number[];
}

const tidy = (value: number) => Number(value.toPrecision(12));

function niceStep(raw: number) {
  const power = 10 ** Math.floor(Math.log10(raw));
  const fraction = raw / power;
  return (
    (fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10) * power
  );
}

/** Round a value range out to human tick steps (1, 2, 5 × 10ⁿ). */
export function niceDomain(
  values: readonly number[],
  {
    includeZero = false,
    count = 4,
    fixed,
  }: {
    includeZero?: boolean;
    count?: number;
    fixed?: readonly [number, number];
  } = {},
): XYDomain {
  let min = fixed?.[0] ?? (values.length ? Math.min(...values) : 0);
  let max = fixed?.[1] ?? (values.length ? Math.max(...values) : 1);
  if (includeZero && !fixed) {
    min = Math.min(0, min);
    max = Math.max(0, max);
  }
  if (min === max) {
    if (min === 0) {max = 1;}
    else if (includeZero && min > 0) {min = 0;}
    else {
      const pad = Math.abs(min) * 0.1;
      min -= pad;
      max += pad;
    }
  }
  const step = niceStep((max - min) / count);
  const lo = fixed ? min : tidy(Math.floor(min / step) * step);
  const hi = fixed ? max : tidy(Math.ceil(max / step) * step);
  const ticks: number[] = [];
  for (
    let tick = Math.ceil(lo / step) * step;
    tick <= hi + step / 1e6;
    tick += step
  )
    {ticks.push(tidy(tick));}
  return {min: lo, max: hi, ticks};
}

/** Show every nth label so neighbours never overlap. */
export function labelStride(count: number, length: number, minGap = 64) {
  if (count <= 1 || length <= 0) {return 1;}
  return Math.max(1, Math.ceil((count * minGap) / length));
}
