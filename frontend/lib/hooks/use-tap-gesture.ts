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

import {useMemo, useRef} from 'react';

/** What a pointerdown recorded, read back by the click that ends its gesture. */
export interface TapRecord<S> {
  /** Which input started the gesture. */
  pointerType: string;
  /** What the surface was showing when it started. */
  state: S;
}

export interface TapGesture<S> {
  /** Record the gesture a pointerdown starts, with the state it starts in. */
  start: (event: {pointerType: string}, state: S) => void;
  /** Read the record and clear it. `null` when no pointer is behind this click. */
  take: () => TapRecord<S> | null;
  /** Drop the record: this gesture will never spend it on a click. */
  drop: () => void;
}

/**
 * The pointer gesture behind a click, recorded where the click cannot report
 * it. A `click` carries no `pointerType` in the engines that matter, so the
 * `pointerdown` before it is the only thing that says which input activated
 * the control — and whether one did at all, since keyboard activation
 * synthesizes a click with no pointer behind it.
 *
 * State goes in with the record because a click reports that no better: a
 * browser that focuses a control on contact can open the very panel the tap
 * was meant to open, and reading "is it open" at click time then undoes it.
 * What the gesture started against is what it acts on.
 *
 * The record is spent by one click and dropped by everything else, because a
 * record that outlives its gesture is worse than none:
 *
 * - A scroll or an OS gesture takes the touch away — `pointercancel`, no click
 *   ever — and the finger would sit in the record until some later click.
 * - That later click is often `Enter` on a keyboard, which arrives with no
 *   pointerdown of its own and would inherit the abandoned finger. A keydown
 *   is the start of a keyboard activation and never part of a tap, so it drops
 *   the record too.
 *
 * Both ends have to be wired by the surface: `drop` on `onPointerCancel` and
 * on `onKeyDown`.
 */
export function useTapGesture<S>(): TapGesture<S> {
  const record = useRef<TapRecord<S> | null>(null);

  return useMemo(
    () => ({
      start: (event, state) => {
        record.current = {pointerType: event.pointerType, state};
      },
      take: () => {
        const spent = record.current;
        record.current = null;
        return spent;
      },
      drop: () => {
        record.current = null;
      },
    }),
    [],
  );
}
