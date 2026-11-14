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

"use client";

import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, CSSProperties, FocusEvent as ReactFocusEvent, ReactNode } from "react";
import { AnimatePresence, animate, motion, useIsPresent, useMotionValue, useReducedMotion, useTransform } from "motion/react";
import type { Transition, Variants } from "motion/react";
import { CalendarDays, ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { motionTokens } from "../lib/motion-tokens";
import { useToday } from "../lib/use-today";
import styles from "./date-range-picker.module.css";

/** An inclusive range of whole days. */
export interface DateRange {
  start: Date;
  end: Date;
}

/** A shortcut in the preset rail. `range` receives the viewer's local today. */
export interface DateRangePreset {
  label: string;
  range: (today: Date) => DateRange;
}

/**
 * A range picker whose trigger grows into the panel it opens. The panel shows two months beside a preset rail, or one
 * month with a scrolling preset row when space is tight. The range highlight stretches across each week as you hover
 * or move with the keyboard, the ends glide between days, months slide in the direction you travel, and on Apply the
 * formatted label flies back into the trigger as the surface shrinks around it.
 * Use it for reports, filters, and bookings where a start and end date are chosen together.
 */
export interface DateRangePickerProps {
  /** Controlled value. Pass `null` for no selection. */
  value?: DateRange | null;
  /** Uncontrolled starting value. */
  defaultValue?: DateRange | null;
  /** Called with the applied range. */
  onChange?: (range: DateRange) => void;
  /** Accessible name of the trigger and the dialog. Defaults to "Date range". */
  label?: string;
  placeholder?: string;
  presets?: DateRangePreset[];
  minDate?: Date;
  maxDate?: Date;
  /** 0 is Sunday, 1 is Monday. Defaults to 0. */
  weekStartsOn?: 0 | 1;
  locale?: string;
  /** Force one or two months. `auto` picks two when the boundary is wide enough. */
  months?: "auto" | 1 | 2;
  /** The element the panel should stay inside. Defaults to the viewport. */
  boundary?: () => HTMLElement | null;
  className?: string;
  /** 底部「取消」，默认英文。Cancel button label. Defaults to English. */
  cancelLabel?: string;
  /** 底部「应用」，默认英文。Apply button label. Defaults to English. */
  applyLabel?: string;
  /** 已选 1 天时的单位。Singular day-count unit. */
  dayLabel?: string;
  /** 已选多天时的单位。Plural day-count unit. */
  daysLabel?: string;
  /** 未选日期时的摘要。Summary when no dates are selected. */
  noDatesLabel?: string;
  /** 已选开始日、等待结束日。Hint after picking a start date. */
  pickEndDateLabel?: string;
  /** 预设分组无障碍名称。Accessible name of the preset rail. */
  presetsLabel?: string;
  /** 上一月无障碍名称。Accessible name of previous month. */
  previousMonthLabel?: string;
  /** 下一月无障碍名称。Accessible name of next month. */
  nextMonthLabel?: string;
  /** 仅选开始日的 live region，`{date}` 替换为格式化日期。Live region after picking start; `{date}` is replaced. */
  startChooseEndLabel?: string;
  /**
   * STX 扩展：在日历下方加「开始时间 / 结束时间」（HH:mm）。开启后 `start` 精确到分钟的 :00.000，`end` 为该分钟的 :59.999，
   * 标签带时间；关闭时行为与上游一致（整天闭区间）。
   * STX extension: adds start / end time (HH:mm) under the calendar. When on, `start` lands on the minute (:00.000) and
   * `end` on that minute's last millisecond (:59.999), and labels show the time; off keeps upstream whole-day behavior.
   */
  withTime?: boolean;
  /** 开始时间输入框的标签。Label of the start time input. */
  startTimeLabel?: string;
  /** 结束时间输入框的标签。Label of the end time input. */
  endTimeLabel?: string;
}

type Size = { w: number; h: number };
type Bezier = [number, number, number, number];

const DAY = 864e5;
const startOfDay = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate());
const monthStart = (date: Date) => new Date(date.getFullYear(), date.getMonth(), 1);
const monthEnd = (date: Date) => new Date(date.getFullYear(), date.getMonth() + 1, 0);
const addDays = (date: Date, amount: number) => new Date(date.getFullYear(), date.getMonth(), date.getDate() + amount);
const addMonths = (date: Date, amount: number) => new Date(date.getFullYear(), date.getMonth() + amount, 1);
const shiftMonths = (date: Date, amount: number) => new Date(date.getFullYear(), date.getMonth() + amount, Math.min(date.getDate(), new Date(date.getFullYear(), date.getMonth() + amount + 1, 0).getDate()));
const dayDiff = (a: Date, b: Date) => Math.round((startOfDay(a).getTime() - startOfDay(b).getTime()) / DAY);
const monthDiff = (a: Date, b: Date) => (a.getFullYear() - b.getFullYear()) * 12 + a.getMonth() - b.getMonth();
const sameDay = (a?: Date | null, b?: Date | null) => Boolean(a && b && dayDiff(a, b) === 0);
const keyOf = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
const fromKey = (key: string) => { const [year, month, day] = key.split("-").map(Number); return new Date(year!, month! - 1, day!); };
const ordered = (a: Date, b: Date): DateRange => dayDiff(a, b) <= 0 ? { start: a, end: b } : { start: b, end: a };
const sameRange = (a?: DateRange | null, b?: DateRange | null) => Boolean(a && b && sameDay(a.start, b.start) && sameDay(a.end, b.end));
/** STX：Date 的本地 HH:mm。 / STX: local HH:mm of a Date. */
const clockOf = (date: Date) => `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
/** STX：把 HH:mm 套到某天；`inclusive` 时落在该分钟最后一毫秒。 / STX: put HH:mm on a day; `inclusive` lands on that minute's last ms. */
const atClock = (day: Date, clock: string, inclusive: boolean) => {
  const [h = 0, m = 0] = clock.split(":").map(Number);
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), Number.isFinite(h) ? h : 0, Number.isFinite(m) ? m : 0, inclusive ? 59 : 0, inclusive ? 999 : 0);
};
const DAY_START = "00:00", DAY_END = "23:59";
/** STX：把手输的时刻规范成 HH:mm（接受 1213、12:13、9），不合法返回 null。 / STX: normalize typed clock text to HH:mm (accepts 1213, 12:13, 9); null when invalid. */
const normalizeClock = (raw: string): string | null => {
  const match = /^(\d{1,2})(?::?(\d{2}))?$/.exec(raw.trim());
  if (!match) return null;
  const h = Number(match[1]), m = Number(match[2] ?? 0);
  return h < 24 && m < 60 ? `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}` : null;
};

/**
 * STX：24 小时制 HH:mm 输入。原生 time 输入按浏览器语言显示（英文浏览器会变成 12:13 PM），与界面语言不一致；
 * 失焦或回车时规范化，非法输入回到原值，↑/↓ 调分钟、Shift+↑/↓ 调小时。
 * STX: 24-hour HH:mm field. A native time input follows the browser locale (an English browser shows 12:13 PM), which
 * can disagree with the UI language; text is normalized on blur or Enter, invalid input reverts, ↑/↓ nudge minutes and
 * Shift+↑/↓ hours.
 */
function ClockField({ label, value, invalid, onCommit }: { label: string; value: string; invalid?: boolean; onCommit: (next: string) => void }) {
  const [text, setText] = useState(value);
  const [previous, setPrevious] = useState(value);
  if (previous !== value) { setPrevious(value); setText(value); }
  const commit = () => {
    const next = normalizeClock(text);
    setText(next ?? value);
    if (next && next !== value) onCommit(next);
  };
  const nudge = (delta: number) => {
    const [h = 0, m = 0] = (normalizeClock(text) ?? value).split(":").map(Number);
    const total = (((h * 60 + m + delta) % 1440) + 1440) % 1440;
    const next = `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
    setText(next);
    onCommit(next);
  };
  return <label className={styles.timeField}>
    <span className={styles.timeLabel}>{label}</span>
    <input type="text" inputMode="numeric" autoComplete="off" spellCheck={false} maxLength={5} placeholder="HH:mm" className={styles.timeInput}
      value={text} aria-invalid={invalid || undefined} onChange={event => setText(event.target.value)} onBlur={commit}
      onKeyDown={event => {
        if (event.key === "Enter") { event.preventDefault(); commit(); }
        else if (event.key === "ArrowUp" || event.key === "ArrowDown") { event.preventDefault(); nudge((event.key === "ArrowUp" ? 1 : -1) * (event.shiftKey ? 60 : 1)); }
      }} />
  </label>;
}
const clampDate = (date: Date, min?: Date, max?: Date) => min && dayDiff(date, min) < 0 ? startOfDay(min) : max && dayDiff(date, max) > 0 ? startOfDay(max) : date;

export const defaultDateRangePresets: DateRangePreset[] = [
  { label: "Today", range: today => ({ start: today, end: today }) },
  { label: "Yesterday", range: today => ({ start: addDays(today, -1), end: addDays(today, -1) }) },
  { label: "Last 7 days", range: today => ({ start: addDays(today, -6), end: today }) },
  { label: "Last 30 days", range: today => ({ start: addDays(today, -29), end: today }) },
  { label: "This month", range: today => ({ start: monthStart(today), end: today }) },
  { label: "Last month", range: today => ({ start: addMonths(today, -1), end: monthEnd(addMonths(today, -1)) }) },
  { label: "This quarter", range: today => ({ start: new Date(today.getFullYear(), Math.floor(today.getMonth() / 3) * 3, 1), end: today }) },
  { label: "Year to date", range: today => ({ start: new Date(today.getFullYear(), 0, 1), end: today }) },
];

/** The viewer's local date (lib/use-today.ts, shared with calendar). Re-exported so existing imports from date-range-picker keep working. */
export { useToday } from "../lib/use-today";

const noop = () => () => {};
function useReducedFlag() {
  const hydrated = useSyncExternalStore(noop, () => true, () => false);
  return !!useReducedMotion() && hydrated;
}

const { blur } = motionTokens;
const enterEase = [...motionTokens.ease.enter] as Bezier;
const standardEase = [...motionTokens.ease.standard] as Bezier;
/** Duration springs restated as stiffness and damping, so every retarget keeps the velocity already in flight. */
const physical = (visualDuration: number, bounce: number): Transition => {
  const root = 2 * Math.PI / (visualDuration * 1.2);
  return { type: "spring", stiffness: root * root, damping: 2 * (1 - bounce) * root, mass: 1 };
};
const { smooth, morph, snappy } = motionTokens.spring;
const GROW = physical(smooth.visualDuration, 0), SHRINK = physical(snappy.visualDuration, 0), STRETCH = physical(morph.visualDuration, 0), GLIDE = physical(morph.visualDuration, 0), SLIDE = physical(smooth.visualDuration, 0);
const TRIGGER_RADIUS = 18, PANEL_RADIUS = 26, WIDE_CELL = 36, WIDE_MIN = 712, EDGE = 8;

const roll: Variants = {
  enter: (direction: number) => ({ opacity: 0, y: `${direction * .45}em`, filter: `blur(${blur.subtle}px)` }),
  center: { opacity: 1, y: "0em", filter: "blur(0px)", transition: { duration: motionTokens.duration.standard, ease: enterEase } },
  exit: (direction: number) => ({ opacity: 0, y: `${direction * -.45}em`, filter: `blur(${blur.subtle}px)`, transition: { duration: motionTokens.duration.fast, ease: standardEase } }),
};
const fade: Variants = { enter: { opacity: 0 }, center: { opacity: 1, y: "0em", filter: "blur(0px)", transition: { duration: motionTokens.duration.instant } }, exit: { opacity: 0, transition: { duration: .08 } } };
/** Panel content arrives just behind the growing surface and leaves ahead of it. */
const faceIn: Variants = {
  hidden: { opacity: 0, y: -4, filter: `blur(${blur.soft}px)` },
  shown: { opacity: 1, y: 0, filter: "blur(0px)", transition: { duration: motionTokens.duration.standard, ease: enterEase, delay: motionTokens.stagger.line } },
  gone: { opacity: 0, y: -2, filter: `blur(${blur.soft}px)`, transition: { duration: motionTokens.duration.instant, ease: standardEase } },
};
const faceFade: Variants = { hidden: { opacity: 0 }, shown: { opacity: 1, transition: { duration: motionTokens.duration.instant } }, gone: { opacity: 0, transition: { duration: .08 } } };
const slide: Variants = {
  enter: (direction: number) => ({ opacity: 0, x: direction * 48 }),
  center: { opacity: 1, x: 0, transition: { x: SLIDE, opacity: { duration: motionTokens.duration.standard, ease: enterEase } } },
  exit: (direction: number) => ({ opacity: 0, x: direction * -36, transition: { x: SLIDE, opacity: { duration: motionTokens.duration.fast, ease: standardEase } } }),
};

/** Words roll one by one: a later date rises from below, an earlier one drops from above; words that stay the same hold still. */
function Rolling({ text, direction, reduced }: { text: string; direction: number; reduced: boolean }) {
  const words = text.split(/\s+/).filter(Boolean);
  return <span className={styles.rolling} aria-hidden="true">
    {words.map((word, index) => <span key={index} className={styles.word}>
      <AnimatePresence initial={false} mode="popLayout" custom={direction}>
        <motion.span key={word} className={styles.wordInner} custom={direction} variants={reduced ? fade : roll} initial="enter" animate="center" exit="exit">{word}</motion.span>
      </AnimatePresence>
    </span>)}
  </span>;
}

/** Remembers which way a range moved, so labels roll in time's direction. */
function useDirection(range: DateRange | null) {
  const time = range ? range.start.getTime() * 2 + range.end.getTime() : null;
  const [previous, setPrevious] = useState(time);
  const [direction, setDirection] = useState(1);
  if (time !== previous) { setPrevious(time); setDirection(time === null || previous === null || time >= previous ? 1 : -1); }
  return direction;
}

type Segment = { left: number; width: number } | null;
const cellIn = (date: Date, first: Date) => { const index = dayDiff(date, first); return { row: Math.floor(index / 7), col: index % 7 }; };

/** One week's slice of the highlight. It grows out of the edge the range arrives from, so hovering reads as stretching. */
function Bar({ row, segment, reduced }: { row: number; segment: Segment; reduced: boolean }) {
  const left = useMotionValue(segment?.left ?? 0), width = useMotionValue(segment?.width ?? 0), opacity = useMotionValue(segment ? 1 : 0);
  const leftPct = useTransform(left, value => `${value}%`), widthPct = useTransform(width, value => `${value}%`);
  const wasEmpty = useRef(!segment);
  useLayoutEffect(() => {
    if (!segment) { wasEmpty.current = true; animate(opacity, 0, { duration: reduced ? 0 : motionTokens.duration.exit }); return; }
    if (wasEmpty.current || reduced) { left.jump(segment.left); width.jump(segment.width); }
    else { animate(left, segment.left, STRETCH); animate(width, segment.width, STRETCH); }
    wasEmpty.current = false;
    animate(opacity, 1, { duration: reduced ? 0 : motionTokens.duration.fast });
  }, [left, opacity, reduced, segment, width]);
  return <motion.span className={styles.bar} style={{ left: leftPct, width: widthPct, opacity, top: `calc(${row} * var(--cell) + 3px)` }} />;
}

/** A solid end of the range. It glides from day to day and only fades when the end leaves this month. */
function Thumb({ cell, reduced }: { cell: { row: number; col: number } | null; reduced: boolean }) {
  const x = useMotionValue(cell?.col ?? 0), y = useMotionValue(cell?.row ?? 0), opacity = useMotionValue(cell ? 1 : 0), scale = useMotionValue(cell ? 1 : .8);
  const transform = useTransform(() => `translate(${x.get() * 100}%, ${y.get() * 100}%) scale(${scale.get()})`);
  const hidden = useRef(!cell);
  useLayoutEffect(() => {
    if (!cell) { hidden.current = true; animate(opacity, 0, { duration: reduced ? 0 : motionTokens.duration.instant }); animate(scale, .8, { duration: reduced ? 0 : motionTokens.duration.instant }); return; }
    if (hidden.current || reduced) { x.jump(cell.col); y.jump(cell.row); }
    else { animate(x, cell.col, GLIDE); animate(y, cell.row, GLIDE); }
    hidden.current = false;
    animate(opacity, 1, { duration: reduced ? 0 : motionTokens.duration.fast });
    animate(scale, 1, reduced ? { duration: 0 } : GLIDE);
  }, [cell, opacity, reduced, scale, x, y]);
  return <motion.span className={styles.thumb} style={{ transform, opacity }} />;
}

interface MonthProps {
  month: Date;
  range: DateRange | null;
  tabbable: string;
  today?: Date;
  minDate?: Date;
  maxDate?: Date;
  weekStartsOn: 0 | 1;
  reduced: boolean;
  /** The last move came from the keyboard, so range highlights land instantly instead of gliding. */
  instant?: boolean;
  formatters: { title: Intl.DateTimeFormat; day: Intl.DateTimeFormat; weekday: Intl.DateTimeFormat; weekdayLong: Intl.DateTimeFormat };
  onPick: (date: Date) => void;
  onHover: (date: Date) => void;
  onKey: (event: ReactKeyboardEvent<HTMLButtonElement>, date: Date) => void;
  onFocusDay: (date: Date) => void;
  idBase: string;
}

function Month({ month, range, tabbable, today, minDate, maxDate, weekStartsOn, reduced, instant = false, formatters, onPick, onHover, onKey, onFocusDay, idBase }: MonthProps) {
  const first = useMemo(() => addDays(month, -((month.getDay() - weekStartsOn + 7) % 7)), [month, weekStartsOn]);
  const rows = useMemo(() => Array.from({ length: 6 }, (_, row) => Array.from({ length: 7 }, (_, col) => addDays(first, row * 7 + col))), [first]);
  const titleId = `${idBase}-${keyOf(month)}`;
  const inMonth = (date: Date) => monthDiff(date, month) === 0;

  const lo = range?.start, hi = range?.end;
  const loKey = lo ? keyOf(lo) : "", hiKey = hi ? keyOf(hi) : "";
  /** Each week's slice of the range, in percent of the row. Keyed by the range's days so bars only move when it does. */
  const segments = useMemo<Segment[]>(() => {
    if (!loKey || !hiKey) return rows.map(() => null);
    const start = fromKey(loKey), end = fromKey(hiKey);
    const unit = 100 / 7;
    const colOf = (date: Date) => dayDiff(date, first) % 7;
    return rows.map(row => {
      const days = row.filter(date => monthDiff(date, month) === 0);
      if (!days.length) return null;
      const rowFirst = days[0]!, rowLast = days[days.length - 1]!;
      const from = dayDiff(start, rowFirst) > 0 ? start : rowFirst, to = dayDiff(end, rowLast) < 0 ? end : rowLast;
      if (dayDiff(from, to) <= 0) { const a = colOf(from), b = colOf(to); return { left: a * unit, width: (b - a + 1) * unit }; }
      // The week sits wholly before or after the range: collapse at the edge the range would grow in from.
      if (dayDiff(rowLast, start) < 0) return { left: (colOf(rowLast) + 1) * unit, width: 0 };
      return { left: colOf(rowFirst) * unit, width: 0 };
    });
  }, [first, hiKey, loKey, month, rows]);
  const startCell = useMemo(() => loKey && monthDiff(fromKey(loKey), month) === 0 ? cellIn(fromKey(loKey), first) : null, [loKey, month, first]);
  const endCell = useMemo(() => hiKey && hiKey !== loKey && monthDiff(fromKey(hiKey), month) === 0 ? cellIn(fromKey(hiKey), first) : null, [hiKey, loKey, month, first]);

  return <div className={styles.month}>
    <p id={titleId} className={styles.monthTitle}>{formatters.title.format(month)}</p>
    <div role="grid" aria-labelledby={titleId} className={styles.grid}>
      <div role="row" className={styles.weekdays}>
        {rows[0]!.map(date => <span key={date.getDay()} role="columnheader" aria-label={formatters.weekdayLong.format(date)}>{formatters.weekday.format(date).slice(0, 2)}</span>)}
      </div>
      <div className={styles.weeks}>
        <span className={styles.layer} aria-hidden="true">
          {segments.map((segment, row) => <Bar key={row} row={row} segment={segment} reduced={reduced || instant} />)}
          <Thumb cell={startCell} reduced={reduced || instant} />
          <Thumb cell={endCell} reduced={reduced || instant} />
        </span>
        {rows.map((row, index) => <div key={index} role="row" className={styles.week}>
          {row.map(date => {
            if (!inMonth(date)) return <span key={keyOf(date)} role="gridcell" className={styles.blank} />;
            const key = keyOf(date);
            const disabled = Boolean((minDate && dayDiff(date, minDate) < 0) || (maxDate && dayDiff(date, maxDate) > 0));
            const inRange = Boolean(lo && hi && dayDiff(date, lo) >= 0 && dayDiff(date, hi) <= 0);
            const edge = sameDay(date, lo) || sameDay(date, hi);
            const isToday = sameDay(date, today);
            return <span key={key} role="gridcell" aria-selected={inRange} className={styles.cell}>
              <button type="button" className={styles.day} data-date={key} data-edge={edge || undefined} data-range={inRange || undefined} data-today={isToday || undefined}
                tabIndex={key === tabbable ? 0 : -1} disabled={disabled} aria-current={isToday ? "date" : undefined}
                aria-label={formatters.day.format(date)}
                onClick={() => onPick(date)} onPointerEnter={() => { if (!disabled) onHover(date); }}
                onFocus={() => onFocusDay(date)} onKeyDown={event => onKey(event, date)}>
                <span>{date.getDate()}</span>
              </button>
            </span>;
          })}
        </div>)}
      </div>
    </div>
  </div>;
}

/** The visible months travel together; a leaving set turns inert so focus and queries only find the current one. */
function Months({ children, direction, reduced }: { children: ReactNode; direction: number; reduced: boolean }) {
  const present = useIsPresent();
  return <motion.div className={styles.months} data-current={present || undefined} inert={!present || undefined} custom={direction}
    variants={reduced ? fade : slide} initial="enter" animate="center" exit="exit">{children}</motion.div>;
}

export function DateRangePicker({
  value,
  defaultValue = null,
  onChange,
  label = "Date range",
  placeholder = "Select dates",
  presets = defaultDateRangePresets,
  minDate,
  maxDate,
  weekStartsOn = 0,
  locale = "en-US",
  months = "auto",
  boundary,
  className,
  cancelLabel = "Cancel",
  applyLabel = "Apply",
  dayLabel = "day",
  daysLabel = "days",
  noDatesLabel = "No dates",
  pickEndDateLabel = "Pick an end date",
  presetsLabel = "Presets",
  previousMonthLabel = "Previous month",
  nextMonthLabel = "Next month",
  startChooseEndLabel = "Start {date}. Choose an end date.",
  withTime = false,
  startTimeLabel = "Start time",
  endTimeLabel = "End time",
}: DateRangePickerProps) {
  const reduced = useReducedFlag();
  const today = useToday();
  const uid = useId();
  const [inner, setInner] = useState<DateRange | null>(defaultValue);
  const committed = value !== undefined ? value : inner;

  const [open, setOpen] = useState(false);
  const [compact, setCompact] = useState(false);
  const [panelWidth, setPanelWidth] = useState(0);
  const [view, setView] = useState<Date>(() => monthStart(committed?.end ?? new Date(2000, 0, 1)));
  const [direction, setDirection] = useState(1);
  const [draft, setDraft] = useState<DateRange | null>(committed);
  const [anchor, setAnchor] = useState<Date | null>(null);
  const [hover, setHover] = useState<Date | null>(null);
  const [keyed, setKeyed] = useState(false);
  const [focusKey, setFocusKey] = useState("");
  // STX：草稿里的起止时刻（HH:mm），只在 withTime 时使用。 / STX: draft start/end clock (HH:mm), used only with withTime.
  const [clock, setClock] = useState({ start: DAY_START, end: DAY_END });

  const formatters = useMemo(() => ({
    label: new Intl.DateTimeFormat(locale, { month: "short", day: "numeric", year: "numeric" }),
    // STX：带时间的标签省掉年份，避免触发器过长。 / STX: timed labels drop the year so the trigger stays short.
    timed: new Intl.DateTimeFormat(locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }),
    title: new Intl.DateTimeFormat(locale, { month: "long", year: "numeric" }),
    day: new Intl.DateTimeFormat(locale, { weekday: "long", month: "long", day: "numeric", year: "numeric" }),
    weekday: new Intl.DateTimeFormat(locale, { weekday: "short" }),
    weekdayLong: new Intl.DateTimeFormat(locale, { weekday: "long" }),
  }), [locale]);
  const format = useCallback((range: DateRange | null) => {
    if (!range) return placeholder;
    // STX：带时间时同一天也要显示起止时刻，formatRange 会把相同的日期部分合并。
    // STX: with time, a same-day range still shows both clocks; formatRange merges the shared date part.
    if (withTime) return range.start.getTime() === range.end.getTime() ? formatters.timed.format(range.start) : formatters.timed.formatRange(range.start, range.end);
    return sameDay(range.start, range.end) ? formatters.label.format(range.start) : formatters.label.formatRange(range.start, range.end);
  }, [formatters, placeholder, withTime]);

  const count = compact ? 1 : 2;
  const visible = useMemo(() => Array.from({ length: count }, (_, index) => addMonths(view, index)), [count, view]);
  const shown = anchor ? ordered(anchor, hover ?? anchor) : draft;
  const activePreset = today && !anchor ? presets.findIndex(preset => sameRange(preset.range(today), draft)) : -1;
  const days = shown ? dayDiff(shown.end, shown.start) + 1 : 0;
  const committedDirection = useDirection(committed);
  const shownDirection = useDirection(shown);

  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const railRef = useRef<HTMLDivElement>(null);
  const pendingFocus = useRef<"trigger" | "day" | null>(null);

  /* ---------- One surface: the trigger's box springs to the panel's box and back ---------- */
  // Before the first measurement the surface simply fills the root and the root wraps the trigger, so server markup already looks right.
  const width = useMotionValue<number | string>("100%"), height = useMotionValue<number | string>("100%"), radius = useMotionValue(TRIGGER_RADIUS), x = useMotionValue(0), rootWidth = useMotionValue<number | string>("auto");
  const sizes = useRef<{ trigger: Size; panel: Size | null }>({ trigger: { w: 0, h: 0 }, panel: null });
  const live = useRef({ open: false, reduced: false, ready: false });
  const queued = useRef(false);
  const boundaryRef = useRef(boundary);
  useLayoutEffect(() => { boundaryRef.current = boundary; }, [boundary]);

  const bounds = useCallback(() => {
    const element = boundaryRef.current?.();
    const viewport = { left: 0, right: document.documentElement.clientWidth };
    if (!element) return viewport;
    const rect = element.getBoundingClientRect();
    return { left: Math.max(viewport.left, rect.left), right: Math.min(viewport.right, rect.right) };
  }, []);

  const update = useCallback(() => {
    queued.current = false;
    const { trigger, panel } = sizes.current, state = live.current;
    if (!trigger.w) return;
    const openPanel = state.open && panel ? panel : null;
    const target = openPanel ?? trigger;
    let offset = 0;
    if (openPanel && rootRef.current) {
      const rect = rootRef.current.getBoundingClientRect(), box = bounds();
      offset = Math.min(0, box.right - EDGE - (rect.left + openPanel.w));
      offset = Math.max(offset, box.left + EDGE - rect.left);
    }
    const r = openPanel ? PANEL_RADIUS : TRIGGER_RADIUS;
    if (!state.ready || state.reduced) {
      width.jump(target.w); height.jump(target.h); radius.jump(r); x.jump(offset); rootWidth.jump(trigger.w);
      state.ready = true;
      return;
    }
    const spring = openPanel ? GROW : state.open ? GROW : SHRINK;
    animate(width, target.w, spring); animate(height, target.h, spring); animate(radius, r, spring); animate(x, offset, spring);
    animate(rootWidth, trigger.w, motionTokens.spring.morph);
  }, [bounds, height, radius, rootWidth, width, x]);

  const schedule = useCallback(() => {
    if (queued.current) return;
    queued.current = true;
    queueMicrotask(update);
  }, [update]);

  useLayoutEffect(() => { live.current.open = open; live.current.reduced = reduced; if (!open) sizes.current.panel = null; schedule(); }, [open, reduced, schedule]);

  useLayoutEffect(() => {
    const node = triggerRef.current;
    if (!node) return;
    const read = () => { sizes.current.trigger = { w: node.offsetWidth, h: node.offsetHeight }; schedule(); };
    read();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(read);
    observer.observe(node);
    return () => observer.disconnect();
  }, [schedule]);

  useLayoutEffect(() => {
    const node = panelRef.current;
    if (!open || !node) return;
    const read = () => { sizes.current.panel = { w: node.offsetWidth, h: node.offsetHeight }; schedule(); };
    read();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(read);
    observer.observe(node);
    return () => observer.disconnect();
  }, [open, schedule]);

  // On a short screen (a phone, a trigger low on the page) the panel can open below the fold, with Apply out of reach.
  // Once it has its size, scroll just enough to bring its bottom into view, never past its top. After closing, the
  // trigger comes back into view if that scroll left it off screen.
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => {
      const node = panelRef.current;
      if (!node) return;
      const rect = node.getBoundingClientRect();
      const overflow = rect.bottom - (window.innerHeight - EDGE);
      if (overflow <= 0) return;
      window.scrollBy({ top: Math.min(overflow, Math.max(0, rect.top - EDGE)), behavior: reduced ? "instant" : "smooth" });
    });
    return () => cancelAnimationFrame(frame);
  }, [open, reduced]);
  useEffect(() => {
    if (open) return;
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    if (rect.bottom < 0 || rect.top > window.innerHeight) trigger.scrollIntoView({ block: "center", behavior: reduced ? "instant" : "smooth" });
  }, [open, reduced]);

  /* ---------- Layout: two months when there is room, one compact month otherwise ---------- */
  const measureLayout = useCallback(() => {
    const box = bounds();
    const available = Math.max(0, box.right - box.left - EDGE * 2);
    const single = months === 1 || (months === "auto" && available < WIDE_MIN);
    return { single, width: Math.min(352, available) };
  }, [bounds, months]);

  useEffect(() => {
    if (!open) return;
    const onResize = () => { const next = measureLayout(); setCompact(next.single); setPanelWidth(next.width); schedule(); };
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [measureLayout, open, schedule]);

  const viewFor = (range: DateRange | null, single: boolean, fallback: Date) => {
    const end = monthStart(range?.end ?? fallback);
    return single ? end : addMonths(end, -1);
  };

  const openPanel = () => {
    if (!today) return;
    const layout = measureLayout();
    setCompact(layout.single);
    setPanelWidth(layout.width);
    setDraft(committed);
    setClock(withTime && committed ? { start: clockOf(committed.start), end: clockOf(committed.end) } : { start: DAY_START, end: DAY_END });
    setAnchor(null);
    setHover(null);
    setView(viewFor(committed, layout.single, today));
    setDirection(0);
    setFocusKey(keyOf(committed?.start ?? today));
    pendingFocus.current = "day";
    setOpen(true);
  };

  const close = useCallback((focus: boolean) => {
    if (focus) pendingFocus.current = "trigger";
    setDirection(0);
    setOpen(false);
    setAnchor(null);
    setHover(null);
  }, []);

  // STX：把草稿日期与时刻合成最终区间；同一天结束早于开始时视为无效。
  // STX: combine draft days with the clock; a same-day end before the start is invalid.
  const timed = (range: DateRange | null): DateRange | null => !range || !withTime ? range : { start: atClock(range.start, clock.start, false), end: atClock(range.end, clock.end, true) };
  const apply = () => {
    const next = timed(anchor ? { start: anchor, end: anchor } : draft);
    if (!next || next.end.getTime() < next.start.getTime()) return;
    if (value === undefined) setInner(next);
    onChange?.(next);
    close(true);
  };

  /* ---------- Focus management ---------- */
  useLayoutEffect(() => {
    const target = pendingFocus.current;
    if (!target) return;
    if (target === "trigger" && !open) { pendingFocus.current = null; triggerRef.current?.focus({ preventScroll: true }); return; }
    if (target === "day" && open) {
      const node = panelRef.current?.querySelector<HTMLButtonElement>(`[data-current] [data-date="${focusKey}"]`);
      if (node) { pendingFocus.current = null; node.focus({ preventScroll: true }); }
    }
  }, [focusKey, open, view, compact]);

  useEffect(() => {
    if (!open) return;
    const down = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) close(false); };
    document.addEventListener("pointerdown", down);
    return () => document.removeEventListener("pointerdown", down);
  }, [close, open]);

  const onRootKey = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape" && open) { event.preventDefault(); event.stopPropagation(); close(true); }
  };
  const onRootBlur = (event: ReactFocusEvent<HTMLDivElement>) => {
    const next = event.relatedTarget as Node | null;
    if (open && next && !event.currentTarget.contains(next)) close(false);
  };

  /* ---------- Month navigation and picking ---------- */
  const goTo = (next: Date) => {
    const delta = monthDiff(next, view);
    if (!delta) return;
    setDirection(Math.sign(delta));
    setView(next);
  };

  /** Keeps a range on screen: nothing moves when it already fits, otherwise its last month lands on the right. */
  const reveal = (range: DateRange) => {
    const first = view, last = addMonths(view, count - 1);
    if (monthDiff(range.start, first) >= 0 && monthDiff(range.end, last) <= 0) return;
    goTo(viewFor(range, compact, range.end));
  };

  const pick = (date: Date) => {
    setFocusKey(keyOf(date));
    if (!anchor) { setAnchor(date); setHover(date); return; }
    setDraft(ordered(anchor, date));
    setAnchor(null);
    setHover(null);
  };

  const choosePreset = (preset: DateRangePreset) => {
    if (!today) return;
    const range = preset.range(today);
    setDraft(range);
    // STX：预设表示整天，时刻回到 00:00–23:59。 / STX: presets mean whole days, so the clock resets to 00:00–23:59.
    if (withTime) setClock({ start: DAY_START, end: DAY_END });
    setAnchor(null);
    setHover(null);
    setFocusKey(keyOf(range.start));
    reveal(range);
  };

  const onDayKey = (event: ReactKeyboardEvent<HTMLButtonElement>, date: Date) => {
    const weekday = (date.getDay() - weekStartsOn + 7) % 7;
    const moves: Record<string, () => Date> = {
      ArrowLeft: () => addDays(date, -1), ArrowRight: () => addDays(date, 1), ArrowUp: () => addDays(date, -7), ArrowDown: () => addDays(date, 7),
      Home: () => addDays(date, -weekday), End: () => addDays(date, 6 - weekday),
      PageUp: () => shiftMonths(date, event.shiftKey ? -12 : -1), PageDown: () => shiftMonths(date, event.shiftKey ? 12 : 1),
    };
    const move = moves[event.key];
    if (!move) return;
    event.preventDefault();
    const next = clampDate(move(), minDate, maxDate);
    setFocusKey(keyOf(next));
    setKeyed(true);
    if (anchor) setHover(next);
    pendingFocus.current = "day";
    const first = view, last = addMonths(view, count - 1);
    if (monthDiff(next, first) < 0) goTo(monthStart(next));
    else if (monthDiff(next, last) > 0) goTo(addMonths(monthStart(next), -(count - 1)));
  };

  const onRailKey = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const keys = compact ? ["ArrowLeft", "ArrowRight"] : ["ArrowUp", "ArrowDown"];
    if (!keys.includes(event.key)) return;
    const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("[data-preset]"));
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (at < 0) return;
    event.preventDefault();
    buttons[(at + (event.key === keys[1] ? 1 : -1) + buttons.length) % buttons.length]?.focus();
  };

  /** The day that owns the tab stop: the focused day when it is on screen, else the first visible day of the range, else the 1st. */
  const tabbable = useMemo(() => {
    const onScreen = (date: Date) => visible.some(month => monthDiff(date, month) === 0);
    if (focusKey && onScreen(fromKey(focusKey))) return focusKey;
    if (shown && onScreen(shown.start)) return keyOf(shown.start);
    return keyOf(visible[0]!);
  }, [focusKey, shown, visible]);

  /* ---------- Preset highlight glides between presets ---------- */
  const gx = useMotionValue(0), gy = useMotionValue(0), gw = useMotionValue(0), gh = useMotionValue(0), go = useMotionValue(0);
  useLayoutEffect(() => {
    const rail = railRef.current;
    const node = activePreset < 0 ? null : rail?.querySelector<HTMLElement>(`[data-preset="${activePreset}"]`);
    if (!rail || !node) { animate(go, 0, { duration: reduced ? 0 : motionTokens.duration.exit }); return; }
    const place = [node.offsetLeft, node.offsetTop, node.offsetWidth, node.offsetHeight] as const;
    if (go.get() < .05 || reduced) { gx.jump(place[0]); gy.jump(place[1]); gw.jump(place[2]); gh.jump(place[3]); }
    else { animate(gx, place[0], GLIDE); animate(gy, place[1], GLIDE); animate(gw, place[2], GLIDE); animate(gh, place[3], GLIDE); }
    animate(go, 1, { duration: reduced ? 0 : motionTokens.duration.fast });
    if (compact && (node.offsetLeft < rail.scrollLeft || node.offsetLeft + node.offsetWidth > rail.scrollLeft + rail.clientWidth)) {
      rail.scrollTo({ left: node.offsetLeft - 12, behavior: reduced ? "auto" : "smooth" });
    }
  }, [activePreset, compact, go, gh, gw, gx, gy, open, reduced]);

  /* ---------- Copy ---------- */
  const committedText = format(committed);
  const shownTimed = timed(shown);
  const shownText = shownTimed ? format(shownTimed) : noDatesLabel;
  const clockInvalid = Boolean(shownTimed && shownTimed.end.getTime() < shownTimed.start.getTime());
  const countText = shown ? `${days} ${days === 1 ? dayLabel : daysLabel}` : "";
  const status = !open
    ? ""
    : anchor
      ? startChooseEndLabel.replace("{date}", formatters.label.format(anchor))
      : shown
        ? `${shownText}, ${countText}`
        : "";
  const layoutId = reduced ? undefined : `${uid}-value`;

  const cellSize = compact && panelWidth ? Math.max(32, Math.min(42, Math.floor((panelWidth - 24) / 7))) : WIDE_CELL;

  const quiet = open ? (reduced ? { opacity: 0 } : { opacity: 0, filter: `blur(${blur.subtle}px)` }) : { opacity: 1, filter: "blur(0px)" };
  const quietTransition = open ? { duration: motionTokens.duration.instant, ease: standardEase } : { duration: motionTokens.duration.standard, ease: enterEase, delay: reduced ? 0 : .1 };

  return <motion.div ref={rootRef} className={[styles.root, className].filter(Boolean).join(" ")} style={{ width: rootWidth }} data-open={open || undefined} onKeyDown={onRootKey} onBlur={onRootBlur}>
    {/* The trigger stays in place under the panel. Its icons fade, while the value itself flies to the panel footer and back. */}
    <button ref={triggerRef} type="button" className={styles.trigger} aria-haspopup="dialog" aria-expanded={open} aria-controls={open ? `${uid}-panel` : undefined}
      aria-label={`${label}: ${committedText}`} disabled={!today} inert={open || undefined} onClick={openPanel}>
      <motion.span className={styles.triggerIcon} initial={false} animate={quiet} transition={quietTransition}><CalendarDays size={16} strokeWidth={1.75} aria-hidden="true" /></motion.span>
      {open || !layoutId
        ? <motion.span key="resting" className={styles.value} data-empty={!committed || undefined} initial={false} animate={open ? { opacity: 0 } : { opacity: 1 }} transition={quietTransition}>
          <Rolling text={committedText} direction={committedDirection} reduced={reduced} />
        </motion.span>
        : <motion.span key="shared" layoutId={layoutId} layout="position" className={styles.value} data-empty={!committed || undefined} transition={SHRINK}>
          <Rolling text={committedText} direction={committedDirection} reduced={reduced} />
        </motion.span>}
      <motion.span className={styles.chevron} initial={false} animate={quiet} transition={quietTransition}><ChevronDown size={16} strokeWidth={1.75} aria-hidden="true" /></motion.span>
    </button>

    <motion.div className={styles.surface} style={{ width, height, borderRadius: radius, x }}>
      <AnimatePresence>
        {open && today && <motion.div key="panel" ref={panelRef} id={`${uid}-panel`} className={styles.panel} role="dialog" aria-label={label} data-compact={compact || undefined}
          style={{ "--cell": `${cellSize}px`, width: compact && panelWidth ? panelWidth : undefined } as CSSProperties} exit={{ opacity: 1, transition: { duration: motionTokens.duration.fast } }}>
          <motion.div className={styles.body} variants={reduced ? faceFade : faceIn} initial="hidden" animate="shown" exit="gone">
            <div ref={railRef} className={styles.rail} role="group" aria-label={presetsLabel} onKeyDown={onRailKey}>
              <motion.span className={styles.railHighlight} style={{ x: gx, y: gy, width: gw, height: gh, opacity: go }} aria-hidden="true" />
              {presets.map((preset, index) => <button key={preset.label} type="button" className={styles.preset} data-preset={index} aria-pressed={index === activePreset}
                onClick={() => choosePreset(preset)}>{preset.label}</button>)}
            </div>

            <div className={styles.calendars} onPointerLeave={() => { if (anchor) setHover(null); }}>
              <button type="button" className={styles.navButton} data-side="prev" aria-label={previousMonthLabel} onClick={() => goTo(addMonths(view, -1))}
                disabled={Boolean(minDate && monthDiff(view, minDate) <= 0)}><ChevronLeft size={16} strokeWidth={1.75} aria-hidden="true" /></button>
              <button type="button" className={styles.navButton} data-side="next" aria-label={nextMonthLabel} onClick={() => goTo(addMonths(view, 1))}
                disabled={Boolean(maxDate && monthDiff(addMonths(view, count - 1), maxDate) >= 0)}><ChevronRight size={16} strokeWidth={1.75} aria-hidden="true" /></button>
              {/* The viewport keeps a fixed six-week height and stacks the leaving and arriving months in one cell, so the panel never resizes mid-slide. */}
              <div className={styles.viewport}>
                <AnimatePresence initial={false} custom={direction}>
                  <Months key={`${keyOf(view)}-${count}`} direction={direction} reduced={reduced}>
                    {visible.map(month => <Month key={keyOf(month)} month={month} range={shown} tabbable={tabbable} today={today} minDate={minDate} maxDate={maxDate}
                      weekStartsOn={weekStartsOn} reduced={reduced} formatters={formatters} idBase={uid}
                      onPick={pick} instant={keyed} onHover={date => { setKeyed(false); if (anchor) setHover(date); }} onKey={onDayKey} onFocusDay={date => setFocusKey(keyOf(date))} />)}
                  </Months>
                </AnimatePresence>
              </div>
            </div>
          </motion.div>

          {withTime && <motion.div className={styles.times} variants={reduced ? faceFade : faceIn} initial="hidden" animate="shown" exit="gone">
            <ClockField label={startTimeLabel} value={clock.start} onCommit={next => setClock(current => ({ ...current, start: next }))} />
            <span className={styles.timeDash} aria-hidden="true">–</span>
            <ClockField label={endTimeLabel} value={clock.end} invalid={clockInvalid} onCommit={next => setClock(current => ({ ...current, end: next }))} />
          </motion.div>}
          <div className={styles.footer} data-timed={withTime || undefined}>
            <div className={styles.summary}>
              <motion.span layoutId={layoutId} layout={layoutId ? "position" : undefined} className={styles.summaryValue} transition={GROW}
                variants={layoutId ? undefined : faceFade} initial={layoutId ? undefined : "hidden"} animate={layoutId ? undefined : "shown"} exit={layoutId ? undefined : "gone"}>
                <Rolling text={shownText} direction={shownDirection} reduced={reduced} />
              </motion.span>
              <motion.span className={styles.summaryCount} variants={reduced ? faceFade : faceIn} initial="hidden" animate="shown" exit="gone">
                {countText && <Rolling text={anchor && sameDay(anchor, hover) ? pickEndDateLabel : countText} direction={shownDirection} reduced={reduced} />}
              </motion.span>
            </div>
            <motion.div className={styles.actions} variants={reduced ? faceFade : faceIn} initial="hidden" animate="shown" exit="gone">
              <button type="button" className={styles.ghost} onClick={() => close(true)}>{cancelLabel}</button>
              <button type="button" className={styles.primary} onClick={apply} disabled={!shown || clockInvalid}>{applyLabel}</button>
            </motion.div>
          </div>
          <p className={styles.srOnly} aria-live="polite">{status}</p>
        </motion.div>}
      </AnimatePresence>
    </motion.div>
  </motion.div>;
}

export default DateRangePicker;
