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

import {useReducedMotion} from 'motion/react';
import {
  createContext,
  type ReactNode,
  useContext,
  useMemo,
  useState,
} from 'react';
import {useHoverCapable} from '@/lib/hooks/use-hover-capable';
import {useMessages} from '@/components/charts/beui-i18n';
import {
  type CompositionChartMessages,
  compositionChartMessages,
} from './messages';
import {buildComposition, type CompositionSeries} from './model';

export interface CompositionChartProps {
  series: readonly CompositionSeries[];
  /** Unique labels in chronological order. */
  periods: readonly string[];
  view?: 'bar' | 'area';
  period?: string;
  defaultPeriod?: string;
  onPeriodChange?: (period: string) => void;
  formatValue?: (value: number) => string;
  label?: string;
  /** Override any built-in string. */
  labels?: Partial<CompositionChartMessages>;
  className?: string;
  children?: ReactNode;
}

const number = new Intl.NumberFormat('en', {maximumFractionDigits: 2});
export function useCompositionModel({
  series,
  periods,
  view = 'bar',
  period,
  defaultPeriod,
  onPeriodChange,
  formatValue = (value) => number.format(value),
  label: customLabel,
  labels,
}: CompositionChartProps) {
  const t = useMessages('compositionChart', compositionChartMessages, labels);
  const label = customLabel ?? t('label');
  const model = useMemo(
    () => buildComposition(series, periods),
    [series, periods],
  );
  const [internal, setInternal] = useState(defaultPeriod);
  const [pinned, setPinned] = useState<string | null>(null);
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const validSeries = (id: string | null) =>
    model.rows.some((row) => row.id === id);
  if (
    internal !== undefined &&
    !model.columns.some((column) => column.id === internal)
  )
    {setInternal(undefined);}
  if (pinned !== null && !validSeries(pinned)) {setPinned(null);}
  if (hovered !== null && !validSeries(hovered)) {setHovered(null);}
  if (focused !== null && !validSeries(focused)) {setFocused(null);}
  const selected = period === undefined ? internal : period;
  const found = model.columns.findIndex((column) => column.id === selected);
  const index = found >= 0 ? found : model.columns.length - 1;
  const select = (next: string) => {
    if (period === undefined) {setInternal(next);}
    if (next !== model.columns[index]?.id) {onPeriodChange?.(next);}
  };
  const highlight =
    [hovered, focused, pinned].find((id) => id !== null && validSeries(id)) ??
    null;
  return {
    ...model,
    index,
    column: model.columns[index],
    select,
    view,
    label,
    t,
    formatValue,
    pinned,
    setPinned,
    highlight,
    setHovered,
    setFocused,
    reduce: useReducedMotion(),
    canHover: useHoverCapable(),
  };
}

export const CompositionContext = createContext<ReturnType<
  typeof useCompositionModel
> | null>(null);
export function useCompositionChart() {
  const context = useContext(CompositionContext);
  if (!context)
    {throw new Error(
      'Composition chart parts must be rendered inside CompositionChart.',
    );}
  return context;
}
