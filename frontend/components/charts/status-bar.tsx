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

// beui.dev/charts/status-bar

import {cn} from '@/lib/utils';
import {
  StatusBarContext,
  type StatusBarProps,
  useStatusBarModel,
} from './status-bar/context';
import {StatusBarLegend} from './status-bar/legend';
import {StatusBarPlot} from './status-bar/plot';
import {StatusBarTooltip} from './status-bar/tooltip';

/** Compose Plot, Tooltip, and Legend, or omit children for the complete chart. */
export function StatusBar({
  data,
  statuses,
  label,
  activeId,
  defaultActiveId,
  onActiveChange,
  labels,
  children,
  className,
  ...props
}: StatusBarProps) {
  const model = useStatusBarModel({
    data,
    statuses,
    label,
    activeId,
    defaultActiveId,
    onActiveChange,
    labels,
  });
  const {t} = model;
  return (
    <StatusBarContext.Provider value={model}>
      <section
        aria-label={model.label}
        {...props}
        data-slot='status-bar'
        className={cn('min-w-0 w-full space-y-3', className)}
      >
        {children === undefined ? (
          <>
            <StatusBarPlot />
            <StatusBarTooltip />
            <StatusBarLegend />
          </>
        ) : (
          children
        )}
        {/* Color is never the only way to read the history. The complete data
            stays available to screen readers without dozens of tab stops. */}
        <div className='sr-only'>
          <table>
            <caption>{t('tableCaption', {label: model.label})}</caption>
            <thead>
              <tr>
                <th scope='col'>{t('period')}</th>
                <th scope='col'>{t('status')}</th>
                <th scope='col'>{t('details')}</th>
              </tr>
            </thead>
            <tbody>
              {model.rows.map(({datum, status}) => (
                <tr key={datum.id}>
                  <th scope='row'>{datum.label}</th>
                  <td>{status.label}</td>
                  <td>{datum.description ?? ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </StatusBarContext.Provider>
  );
}

export type {
  StatusBarDatum,
  StatusBarProps,
  StatusBarStatus,
} from './status-bar/context';
export {STATUS_BAR_STATUSES, useStatusBar} from './status-bar/context';
export type {StatusBarLegendProps} from './status-bar/legend';
export {StatusBarLegend} from './status-bar/legend';
export {type StatusBarMessages, statusBarMessages} from './status-bar/messages';
export type {StatusBarPlotProps} from './status-bar/plot';
export {StatusBarPlot} from './status-bar/plot';
export type {
  StatusBarTooltipData,
  StatusBarTooltipProps,
} from './status-bar/tooltip';
export {StatusBarTooltip} from './status-bar/tooltip';
