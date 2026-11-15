/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Status Bar (https://beui.dev/charts/status-bar).
 * Registry: https://beui.dev/r/status-bar.json
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
  type ComponentProps,
  createContext,
  useCallback,
  useContext,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {useHoverCapable} from '@/lib/hooks/use-hover-capable';
import {useMessages} from '@/components/charts/beui-i18n';
import {type StatusBarMessages, statusBarMessages} from './messages';

export interface StatusBarDatum {
  /** Stable, unique identity. Keep it when updating a period's status. */
  id: string;
  /** Consumer-formatted period, date, or event label. */
  label: string;
  /** Must match an ID in statuses. */
  status: string;
  description?: string;
}

export interface StatusBarStatus {
  id: string;
  label: string;
  /** Any CSS color, including theme variables. */
  color: string;
}

export const STATUS_BAR_STATUSES: readonly StatusBarStatus[] = [
  {id: 'operational', label: 'Operational', color: '#059669'},
  {id: 'degraded', label: 'Degraded', color: '#d97706'},
  {id: 'outage', label: 'Outage', color: '#e11d48'},
  {id: 'maintenance', label: 'Maintenance', color: '#2563eb'},
  {id: 'unknown', label: 'No data', color: 'var(--muted-foreground)'},
];

export interface StatusBarProps extends ComponentProps<'section'> {
  /** Observations in display order. The chart never generates or aggregates data. */
  data: readonly StatusBarDatum[];
  /** Override the default status labels and colors, or define your own states. */
  statuses?: readonly StatusBarStatus[];
  label?: string;
  activeId?: string | null;
  defaultActiveId?: string | null;
  onActiveChange?: (id: string | null) => void;
  /** Override any built-in string, including the default status labels. */
  labels?: Partial<StatusBarMessages>;
}

export function useStatusBarModel({
  data,
  statuses: customStatuses,
  label: customLabel,
  activeId,
  defaultActiveId = null,
  onActiveChange,
  labels,
}: StatusBarProps) {
  const t = useMessages('statusBar', statusBarMessages, labels);
  const label = customLabel ?? t('label');
  // Only the built-in states translate; consumer-defined statuses keep their own labels.
  const statuses = useMemo(
    () =>
      customStatuses ??
      STATUS_BAR_STATUSES.map((status) => ({
        ...status,
        // Built-in status ids double as message keys.
        label: t(status.id as keyof StatusBarMessages),
      })),
    [customStatuses, t],
  );
  const rows = useMemo(() => {
    const states = new Map<string, StatusBarStatus>();
    for (const status of statuses) {
      if (states.has(status.id))
        {throw new Error(`StatusBar: duplicate status ID "${status.id}".`);}
      states.set(status.id, status);
    }
    const ids = new Set<string>();
    return data.map((datum) => {
      if (ids.has(datum.id))
        {throw new Error(`StatusBar: duplicate period ID "${datum.id}".`);}
      ids.add(datum.id);
      const status = states.get(datum.status);
      if (!status)
        {throw new Error(
          `StatusBar: define the status "${datum.status}" in statuses.`,
        );}
      return {datum, status};
    });
  }, [data, statuses]);
  const [internalId, setInternalId] = useState(defaultActiveId);
  // Resolve stale identities during render so returning periods never revive a cursor.
  if (internalId !== null && !rows.some(({datum}) => datum.id === internalId))
    {setInternalId(null);}
  const requestedId = activeId === undefined ? internalId : activeId;
  const activeIndex = rows.findIndex(({datum}) => datum.id === requestedId);
  const active = rows[activeIndex] ?? null;
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const plotRef = useRef<HTMLDivElement>(null);
  const tooltipId = useId();
  const latest = useRef({activeId, requestedId, onActiveChange});
  useLayoutEffect(() => {
    latest.current = {activeId, requestedId, onActiveChange};
  });
  const setActive = useCallback((id: string | null) => {
    const current = latest.current;
    if (current.activeId === undefined) {setInternalId(id);}
    if (current.requestedId !== id) {current.onActiveChange?.(id);}
  }, []);
  return {
    rows,
    statuses,
    label,
    t,
    active,
    activeIndex,
    setActive,
    plotRef,
    tooltipId,
    tooltipOpen,
    setTooltipOpen,
    reduce: useReducedMotion(),
    canHover: useHoverCapable(),
  };
}

export const StatusBarContext = createContext<ReturnType<
  typeof useStatusBarModel
> | null>(null);

export function useStatusBar() {
  const context = useContext(StatusBarContext);
  if (!context)
    {throw new Error('Status bar parts must be rendered inside StatusBar.');}
  return context;
}
