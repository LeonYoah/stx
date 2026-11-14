/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/date-range-picker.json). Upstream license:
 * https://uiarc.dev/license
 *
 * Local STX adaptations, if any, remain under the same MIT terms and do not
 * relicense the upstream Arc source as Apache-2.0.
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

import { useMemo, useSyncExternalStore } from "react";

/** Shared by calendar and date-range-picker, which both re-export it, so one copy keeps their "today" in step. */
const dateKey = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
const fromKey = (key: string) => {
  const [year = 1970, month = 1, day = 1] = key.split("-").map(Number);
  return new Date(year, month - 1, day);
};

/** Today turns over at local midnight; returning to the tab, focusing the window or waking the device reads it again. */
const subscribeToday = (notify: () => void) => {
  let timer = 0;
  const schedule = () => {
    const now = new Date();
    const nextMidnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
    timer = window.setTimeout(() => { notify(); schedule(); }, nextMidnight.getTime() - now.getTime() + 1000);
  };
  const onVisible = () => { if (document.visibilityState === "visible") notify(); };
  schedule();
  document.addEventListener("visibilitychange", onVisible);
  window.addEventListener("focus", notify);
  return () => { window.clearTimeout(timer); document.removeEventListener("visibilitychange", onVisible); window.removeEventListener("focus", notify); };
};
const readToday = () => dateKey(new Date());
const serverToday = () => "";

/** The viewer's local date. Undefined on the server and during hydration, so markup never depends on the server clock or time zone; afterwards it follows the real date across midnight. */
export function useToday() {
  const key = useSyncExternalStore(subscribeToday, readToday, serverToday);
  return useMemo(() => (key ? fromKey(key) : undefined), [key]);
}
