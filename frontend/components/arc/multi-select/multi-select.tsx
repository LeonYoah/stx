/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/multi-select.json). Upstream license:
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
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
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

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import type { Variants } from "motion/react";
import { ChevronDown, X } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./multi-select.module.css";

export interface MultiSelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface MultiSelectProps {
  label: string;
  options: MultiSelectOption[];
  value?: string[];
  defaultValue?: string[];
  onValueChange?: (value: string[]) => void;
  placeholder?: string;
  // 本地修改：options 为空时菜单里展示一条说明，否则展开是一根空白条。
  // Local modification: message row inside the menu when options is empty —
  // without it the open menu renders as a bare empty strip.
  emptyMessage?: string;
  description?: string;
  maxVisible?: number;
  disabled?: boolean;
  className?: string;
  /**
   * 本地修改：菜单内搜索。缺省在 options > 7 时自动开启；传 true/false 强制开关。
   * Local modification: in-menu search. Auto-enabled past 7 options; pass a boolean to force.
   */
  searchable?: boolean;
  /** 本地修改：搜索框占位文案。Local: search input placeholder. */
  searchPlaceholder?: string;
  /** 本地修改：搜索无匹配的说明文案。Local: message when the query matches nothing. */
  noMatchMessage?: string;
}

const enter = motionTokens.ease.enter;
const standard = motionTokens.ease.standard;
/** Each chip sits in a slot whose width opens and collapses on a spring, so neighbours travel with it and nothing overlaps. */
const slot: Variants = {
  hidden: { width: 0, opacity: 0 },
  shown: { width: "auto", opacity: 1, transition: { width: motionTokens.spring.smooth, opacity: { duration: motionTokens.duration.fast, ease: enter } } },
  gone: { width: 0, opacity: 0, transition: { width: motionTokens.spring.smooth, opacity: { duration: motionTokens.duration.instant, ease: standard } } },
};
/** The chip itself grows in from .9 with a soft blur and shrinks back as it leaves. */
const chip: Variants = {
  hidden: { scale: 0.9, filter: `blur(${motionTokens.blur.soft}px)` },
  shown: { scale: 1, filter: "blur(0px)", transition: { ...motionTokens.spring.snappy, filter: { duration: motionTokens.duration.standard, ease: enter } } },
  gone: { scale: 0.9, filter: `blur(${motionTokens.blur.subtle}px)`, transition: { duration: motionTokens.duration.instant, ease: standard } },
};
/** Reduced motion keeps short crossfades; resting states match the moving variants so server and client markup agree. */
const fade: Variants = { hidden: { opacity: 0 }, shown: { opacity: 1, y: 0, filter: "blur(0px)", transition: { duration: motionTokens.duration.instant } }, gone: { opacity: 0, transition: { duration: motionTokens.duration.instant } } };
const slotFade: Variants = { hidden: { opacity: 0 }, shown: { width: "auto", opacity: 1, transition: { duration: motionTokens.duration.instant } }, gone: { opacity: 0, transition: { duration: motionTokens.duration.instant } } };
const chipStill: Variants = { hidden: { scale: 1, filter: "blur(0px)" }, shown: { scale: 1, filter: "blur(0px)" }, gone: { scale: 1, filter: "blur(0px)" } };
/** The overflow count rolls: a larger number rises from below, a smaller one drops from above. */
const roll: Variants = {
  hidden: (direction: number) => ({ opacity: 0, y: `${direction * 0.5}em`, filter: `blur(${motionTokens.blur.subtle}px)` }),
  shown: { opacity: 1, y: 0, filter: "blur(0px)", transition: { duration: motionTokens.duration.standard, ease: enter } },
  gone: (direction: number) => ({ opacity: 0, y: `${direction * -0.5}em`, filter: `blur(${motionTokens.blur.subtle}px)`, transition: { duration: motionTokens.duration.instant, ease: standard } }),
};

/** A check that draws itself when an option is picked and retracts when it is removed. */
function CheckMark({ reduce }: { reduce: boolean | null }) {
  return <svg className={styles.check} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <motion.path d="M4 12.5 9 17.5 20 6.5" initial={reduce ? { opacity: 0 } : { pathLength: 0, opacity: 0 }} animate={{ pathLength: 1, opacity: 1 }} exit={reduce ? { opacity: 0 } : { pathLength: 0, opacity: 0 }} transition={reduce ? { duration: motionTokens.duration.instant } : { pathLength: { duration: motionTokens.duration.standard, ease: enter }, opacity: { duration: motionTokens.duration.instant } }} />
  </svg>;
}

export function MultiSelect({ label, options, value, defaultValue = [], onValueChange, placeholder = "Select options", emptyMessage, description, maxVisible = 2, disabled = false, className, searchable, searchPlaceholder = "Search…", noMatchMessage }: MultiSelectProps) {
  const id = useId();
  const labelId = `${id}-label`;
  const valueId = `${id}-value`;
  const rootRef = useRef<HTMLDivElement>(null);
  const [internal, setInternal] = useState(defaultValue);
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const selected = value ?? internal;
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const labelFor = (item: string) => options.find((option) => option.value === item)?.label ?? item;
  const visible = selected.slice(0, maxVisible).map((item) => ({ value: item, label: labelFor(item) }));
  const remaining = Math.max(0, selected.length - visible.length);
  // 本地修改：搜索状态与过滤后的可视列表；关闭时随高亮一起清空。
  // Local: search query + filtered list; cleared together with the highlight on close.
  const searchOn = searchable ?? options.length > 7;
  const [query, setQuery] = useState("");
  const queryNorm = query.trim().toLowerCase();
  const visibleOptions = useMemo(
    () => queryNorm ? options.filter((option) => option.label.toLowerCase().includes(queryNorm) || option.value.toLowerCase().includes(queryNorm)) : options,
    [options, queryNorm],
  );
  // A closed menu forgets its highlight, so the next open starts clean instead of on a stale hovered row.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) { setWasOpen(open); if (!open) {setActiveIndex(-1); setQuery("");} }
  const [previousRemaining, setPreviousRemaining] = useState(remaining);
  const [countDirection, setCountDirection] = useState(1);
  if (previousRemaining !== remaining) { setPreviousRemaining(remaining); setCountDirection(remaining > previousRemaining ? 1 : -1); }

  useEffect(() => {
    const close = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) {setOpen(false);} };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);

  const update = (next: string[]) => { if (value === undefined) {setInternal(next);} onValueChange?.(next); };
  const toggle = (option: MultiSelectOption) => {
    if (disabled || option.disabled) {return;}
    update(selectedSet.has(option.value) ? selected.filter((item) => item !== option.value) : [...selected, option.value]);
  };
  const clear = () => { update([]); setOpen(false); };
  const enabled = visibleOptions.map((option, index) => option.disabled ? -1 : index).filter((index) => index >= 0);
  const step = (direction: 1 | -1) => {
    if (!enabled.length) {return;}
    setOpen(true);
    const current = enabled.indexOf(activeIndex);
    const next = current < 0 ? (direction === 1 ? 0 : enabled.length - 1) : (current + direction + enabled.length) % enabled.length;
    setActiveIndex(enabled[next]!);
  };
  const onKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (disabled) {return;}
    if (event.key === "Enter" && open && activeIndex >= 0) { event.preventDefault(); toggle(visibleOptions[activeIndex]!); return; }
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); setOpen((current) => !current); return; }
    if (event.key === "Escape") { setOpen(false); return; }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); step(event.key === "ArrowDown" ? 1 : -1); }
  };
  // 本地修改：搜索框内沿用同一套方向键/回车导航，焦点不用离开输入框。
  // Local: the search input reuses the same arrow/enter navigation so focus never leaves it.
  const onSearchKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter" && activeIndex >= 0) { event.preventDefault(); toggle(visibleOptions[activeIndex]!); return; }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); step(event.key === "ArrowDown" ? 1 : -1); return; }
    if (event.key === "Escape") { setOpen(false); }
  };
  const reduce = useReducedMotion();

  return <div ref={rootRef} className={[styles.field, className].filter(Boolean).join(" ")}>
    <span id={labelId} className={styles.label}>{label}</span>
    <div className={styles.control}><button type="button" className={`${styles.trigger} ${selected.length ? styles.hasClear : ""}`} disabled={disabled} aria-labelledby={`${labelId} ${valueId}`} aria-haspopup="listbox" aria-expanded={open} aria-controls={`${id}-listbox`} onClick={() => setOpen((current) => !current)} onKeyDown={onKeyDown}>
      <span id={valueId} className={styles.srOnly}>{selected.length ? selected.map(labelFor).join(", ") : placeholder}</span>
      <span className={styles.value} aria-hidden="true">
        <AnimatePresence initial={false}>
          {visible.map((item) => <motion.span key={`chip-${item.value}`} className={styles.slot} variants={reduce ? slotFade : slot} initial="hidden" animate="shown" exit="gone"><motion.span className={styles.chip} variants={reduce ? chipStill : chip}>{item.label}</motion.span></motion.span>)}
          {remaining > 0 && <motion.span key="more" className={styles.slot} variants={reduce ? slotFade : slot} initial="hidden" animate="shown" exit="gone"><motion.span className={styles.more} variants={reduce ? chipStill : chip}>+<span className={styles.count}><AnimatePresence initial={false} custom={countDirection}><motion.span key={remaining} custom={countDirection} variants={reduce ? fade : roll} initial="hidden" animate="shown" exit="gone">{remaining}</motion.span></AnimatePresence></span></motion.span></motion.span>}
          {!selected.length && <motion.span key="placeholder" className={styles.placeholder} variants={fade} initial="hidden" animate="shown" exit="gone">{placeholder}</motion.span>}
        </AnimatePresence>
      </span>
      <ChevronDown className={styles.chevron} size={16} aria-hidden="true" />
    </button>
    <AnimatePresence initial={false}>{selected.length > 0 && !disabled && <motion.button type="button" aria-label="Clear selections" className={styles.clear} onClick={clear}
      initial={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.7, filter: `blur(${motionTokens.blur.subtle}px)` }}
      animate={{ opacity: 1, scale: 1, filter: "blur(0px)", transition: reduce ? { duration: motionTokens.duration.instant } : { ...motionTokens.spring.snappy, opacity: { duration: motionTokens.duration.fast } } }}
      exit={{ opacity: 0, ...(reduce ? {} : { scale: 0.7, filter: `blur(${motionTokens.blur.subtle}px)` }), transition: { duration: motionTokens.duration.instant, ease: standard } }}
      whileTap={{ scale: reduce ? 1 : 0.96, transition: { duration: motionTokens.duration.instant, ease: standard } }}><X size={14} aria-hidden="true" /></motion.button>}</AnimatePresence>
    <AnimatePresence initial={false}>
      {open && <motion.div id={`${id}-listbox`} className={styles.menu} role="listbox" aria-label={label} aria-multiselectable="true"
        initial={reduce ? { opacity: 0 } : { opacity: 0, y: -6, scale: .97 }}
        animate={{ opacity: 1, y: 0, scale: 1, transition: reduce ? { duration: motionTokens.duration.instant } : { ...motionTokens.spring.snappy, opacity: { duration: motionTokens.duration.fast, ease: enter } } }}
        exit={{ opacity: 0, ...(reduce ? {} : { y: -4, scale: .98 }), transition: { duration: motionTokens.duration.instant, ease: standard } }}>
        {searchOn ? <input type="text" role="searchbox" aria-label={searchPlaceholder} className={styles.search} placeholder={searchPlaceholder} value={query} autoFocus onChange={(event) => { setQuery(event.target.value); setActiveIndex(-1); }} onKeyDown={onSearchKeyDown} /> : null}
        {/* 本地修改：列表区限高滚动，长选项不再撑破弹窗/视口。
            Local: capped scrollable list region so long menus can't overflow dialogs/viewport. */}
        <div className={styles.list}>
        {options.length === 0 ? <div className={styles.empty}>{emptyMessage ?? "No options"}</div> : null}
        {options.length > 0 && visibleOptions.length === 0 ? <div className={styles.empty}>{noMatchMessage ?? "No matches"}</div> : null}
        {visibleOptions.map((option, index) => <button type="button" role="option" aria-selected={selectedSet.has(option.value)} aria-disabled={option.disabled || undefined} key={option.value} className={styles.option} data-active={activeIndex === index} disabled={option.disabled} onPointerMove={() => { if (activeIndex !== index) {setActiveIndex(index);} }} onClick={() => toggle(option)}>
          <span>{option.label}</span><AnimatePresence initial={false}>{selectedSet.has(option.value) && <CheckMark key="check" reduce={reduce} />}</AnimatePresence>
        </button>)}
        </div>
      </motion.div>}
    </AnimatePresence>
    </div>
    {description && <span className={styles.description}>{description}</span>}
  </div>;
}
