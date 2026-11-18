/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Table (https://beui.dev/components/motion/table).
 * Registry: https://beui.dev/r/table.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * 本地改动：beUI 的 lib/i18n 指向 components/charts/beui-i18n.tsx，不占用 STX next-intl 的 frontend/lib/i18n/；
 * lib/utils、lib/ease、lib/touch 用仓库已有版本。
 * Local change: beUI's lib/i18n points at components/charts/beui-i18n.tsx so it does not shadow STX's next-intl
 * frontend/lib/i18n/; lib/utils, lib/ease and lib/touch are the copies already in the repo.
 * Local STX adaptations remain under the same MIT terms and are not relicensed as Apache-2.0.
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

import type { useMessages } from "@/components/charts/beui-i18n";

/** English defaults. Override per instance with `labels`, or app-wide with `LocaleProvider`. */
export const tableMessages = {
  empty: "No data",
  selectAll: "Select all rows",
  selectRow: "Select row {row}",
  cellLabel: "{column} for row {row}",
  cellPlaceholder: "Empty",
  columnOptions: "{column} column options",
  reorderColumn: "Reorder {column} column",
  renameColumn: "Rename {column} column",
  resizeColumn: "Resize {column} column",
  rowOptions: "Row {row} options",
  insertBefore: "Insert before",
  insertAfter: "Insert after",
  deleteColumn: "Delete column",
  deleteRow: "Delete row",
};

export type TableMessages = typeof tableMessages;

/** The resolved translator the root hands to its parts. */
export type TableTranslate = ReturnType<typeof useMessages<TableMessages>>;
