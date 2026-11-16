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

'use client';

import {useCallback, useId, useLayoutEffect, useRef, useState} from 'react';

/** Controlled/uncontrolled value with a stable setter. */
export function useControllable<T>(
  value: T | undefined,
  defaultValue: T,
  onChange?: (next: T) => void,
) {
  const [internal, setInternal] = useState(defaultValue);
  const latest = useRef({value, internal, onChange});
  useLayoutEffect(() => {
    latest.current = {value, internal, onChange};
  });
  const set = useCallback((next: T) => {
    const current = latest.current;
    if (current.value === undefined) {setInternal(next);}
    if (
      (current.value === undefined ? current.internal : current.value) !== next
    )
      {current.onChange?.(next);}
  }, []);
  return [value === undefined ? internal : value, set, setInternal] as const;
}

/**
 * The inspected category is held by id and resolved during render, so a
 * category that leaves the data clears the cursor instead of landing on a
 * neighbour, and one that comes back cannot revive it.
 */
export function useInspection({
  ids,
  activeId,
  defaultActiveId = null,
  onActiveChange,
}: {
  ids: readonly string[];
  activeId?: string | null;
  defaultActiveId?: string | null;
  onActiveChange?: (id: string | null) => void;
}) {
  const [requested, setActive, setInternal] = useControllable(
    activeId,
    defaultActiveId,
    onActiveChange,
  );
  if (activeId === undefined && requested !== null && !ids.includes(requested))
    {setInternal(null);}
  const activeIndex = requested === null ? -1 : ids.indexOf(requested);
  const [open, setOpen] = useState(false);
  // A finger on the plot: inspection marks follow it directly instead of gliding.
  const [touching, setTouching] = useState(false);
  const plotRef = useRef<HTMLDivElement>(null);
  const tooltipId = useId();
  return {
    activeId: activeIndex >= 0 ? requested : null,
    activeIndex,
    setActive,
    open,
    setOpen,
    touching,
    setTouching,
    plotRef,
    tooltipId,
  };
}
