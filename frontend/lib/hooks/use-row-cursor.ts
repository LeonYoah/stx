/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Sankey Chart (https://beui.dev/charts/sankey-chart).
 * Registry: https://beui.dev/r/sankey-chart.json
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

import {useCallback, useLayoutEffect, useRef, useState} from 'react';

/**
 * Where the keyboard or the pointer last moved to: the row's id, stamped with
 * the query it was placed under.
 */
type RowCursor = {id: string; query: string};

/** The cursor's row, or -1 once the query has moved on or the row has left. */
function indexOfCursor(
  rows: readonly {id: string}[],
  query: string,
  cursor: RowCursor | null,
) {
  if (cursor === null || cursor.query !== query) {return -1;}
  return rows.findIndex((row) => row.id === cursor.id);
}

/**
 * The highlighted row of a list whose rows can change under it.
 *
 * Which row is highlighted is resolved during render, never in a passive
 * effect: a passive effect runs after the commit, so a list that had just
 * changed would carry an `aria-activedescendant` naming a row that has left it,
 * and a key pressed in that window would commit the wrong row or nothing at
 * all.
 *
 * The cursor holds the row's id, not its position. A position alone cannot tell
 * a list that shrank from one that swapped its rows for a different set of the
 * same length, and the second case is the one that silently hands Enter to a
 * row the user never chose. A cursor whose row has left the list returns the
 * highlight to the first row rather than to the nearest surviving one: the row
 * the user aimed at is gone, and the first row is where a new query already
 * puts the highlight.
 *
 * Pass the query the list is filtered by. The cursor is stamped with it and
 * dropped when it changes, so a caller cannot forget to clear it — including a
 * caller whose query arrives as a prop and so never runs its own handler. Rows
 * that come back under a query that has moved on cannot revive it either.
 * `moveTo(null)` is for deliberate resets, such as reopening the list.
 *
 * Both callbacks keep one identity for the life of the component, and read the
 * rows and the query through a ref to do it. A caller will put them in an
 * effect's dependencies — the exhaustive-deps rule makes it — and a `moveTo`
 * rebuilt on every keystroke would re-run that effect on every keystroke.
 */
export function useRowCursor(rows: readonly {id: string}[], query: string) {
  const [cursor, setCursor] = useState<RowCursor | null>(null);
  // Written after commit, not during render: a render React discards or has not
  // finished still runs the component body, and an event handler that read this
  // in that window would stamp the cursor with a query the committed tree does
  // not have.
  const latest = useRef({rows, query});
  useLayoutEffect(() => {
    latest.current = {rows, query};
  });

  const cursorRow = indexOfCursor(rows, query, cursor);
  // Cleared rather than ignored: React re-runs this render with the cursor
  // already gone, so rows that come back cannot revive a highlight the user has
  // stopped aiming at.
  if (cursor !== null && cursorRow < 0) {setCursor(null);}

  const moveTo = useCallback(
    (id: string | null) =>
      setCursor(id === null ? null : {id, query: latest.current.query}),
    [],
  );

  const moveActive = useCallback((direction: 1 | -1) => {
    const {rows: live, query: liveQuery} = latest.current;
    const last = live.length - 1;
    if (last < 0) {return;}
    // Steps from the row the cursor is really on, inside the update, so that
    // two keys landing in one batch move two rows rather than one.
    setCursor((current) => {
      const at = Math.max(indexOfCursor(live, liveQuery, current), 0);
      const next = Math.min(Math.max(at + direction, 0), last);
      return {id: live[next].id, query: liveQuery};
    });
  }, []);

  return {activeIndex: cursorRow < 0 ? 0 : cursorRow, moveTo, moveActive};
}
