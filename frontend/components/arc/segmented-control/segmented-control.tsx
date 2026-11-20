/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/segmented-control.json). Upstream license:
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

import { useEffect, useId, useLayoutEffect, useRef, type KeyboardEvent, type ReactNode } from "react";
import { LayoutGroup, motion, useReducedMotion } from "motion/react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./segmented-control.module.css";

// 本地修改：label 放宽为 ReactNode 以支持纯图标段；纯图标段必须传 ariaLabel。
// Local modification: label accepts ReactNode for icon-only segments, which
// must then provide ariaLabel for an accessible name.
export interface Segment { value: string; label: ReactNode; /** Optional content after the label, such as a badge. */ accessory?: ReactNode; /** Accessible name for icon-only segments. / 纯图标段的无障碍名。 */ ariaLabel?: string; /** Stable hook for tests. / 测试锚点。 */ testId?: string; }
export interface SegmentedControlProps {
  options: Segment[];
  value: string;
  onValueChange: (value: string) => void;
  label?: string;
  /** Called when the pointer or focus reaches an option, before it is chosen. Use it to start loading what that option shows. */
  onOptionIntent?: (value: string) => void;
  className?: string;
  /**
   * 跨实例共享的 LayoutGroup 键：同一逻辑切换被渲染成两个不同时挂载的实例
   * （如二级视图的两侧各挂一份）时，传同一个键可让选中态胶囊继承上一实例
   * 的位置做形变，而不是瞬移。默认按实例隔离（useId），互不串扰。
   * Shared LayoutGroup key across remounted instances: when one logical switch
   * is rendered as two never-coexisting instances, passing the same key lets
   * the selection pill morph from the previous instance's position instead of
   * teleporting. Defaults to a per-instance useId so unrelated controls never
   * interfere.
   */
  layoutGroup?: string;
}

export default function SegmentedControl({ options, value, onValueChange, label, onOptionIntent, className, layoutGroup }: SegmentedControlProps) {
  const id = useId();
  const reduced = useReducedMotion();
  const track = useRef<HTMLDivElement>(null);

  // When the options are wider than the container, the track scrolls inside itself. Edges fade only on the side with more to see.
  useLayoutEffect(() => {
    const node = track.current;
    if (!node) {return;}
    const edges = () => {
      const rest = node.scrollWidth - node.clientWidth - node.scrollLeft;
      node.toggleAttribute("data-fade-start", node.scrollLeft > 1);
      node.toggleAttribute("data-fade-end", rest > 1);
    };
    edges();
    node.addEventListener("scroll", edges, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(edges);
    observer?.observe(node);
    return () => { node.removeEventListener("scroll", edges); observer?.disconnect(); };
  }, [options.length]);

  // The selected option is always scrolled fully into view, with a little room so it clears the fade.
  const first = useRef(true);
  useEffect(() => {
    const node = track.current;
    const button = node?.querySelector<HTMLElement>('[aria-pressed="true"]');
    if (!node || !button || node.scrollWidth <= node.clientWidth) { first.current = false; return; }
    const room = 20; const start = button.offsetLeft - room; const end = button.offsetLeft + button.offsetWidth + room - node.clientWidth;
    const left = node.scrollLeft > start ? start : node.scrollLeft < end ? end : node.scrollLeft;
    if (left !== node.scrollLeft) {node.scrollTo({ left: Math.max(0, left), behavior: first.current || reduced ? "auto" : "smooth" });}
    first.current = false;
  }, [value, reduced]);

  // Arrow keys, Home and End move the selection like a tab list; only the selected option is a tab stop.
  const selectedIndex = Math.max(0, options.findIndex(option => option.value === value));
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    const last = options.length - 1;
    const target = event.key === "ArrowRight" || event.key === "ArrowDown" ? (selectedIndex === last ? 0 : selectedIndex + 1)
      : event.key === "ArrowLeft" || event.key === "ArrowUp" ? (selectedIndex === 0 ? last : selectedIndex - 1)
        : event.key === "Home" ? 0 : event.key === "End" ? last : -1;
    if (target < 0 || !options[target]) {return;}
    event.preventDefault();
    onValueChange(options[target].value);
    track.current?.querySelector<HTMLElement>(`[data-value="${CSS.escape(options[target].value)}"]`)?.focus({ preventScroll: true });
  };

  return <div className={`${styles.root} ${className ?? ""}`} role="group" aria-label={label}>
    <LayoutGroup id={layoutGroup ?? id}><motion.div ref={track} layoutScroll className={styles.track}>
      {options.map((option, index) => <button key={option.value} id={`${id}-${option.value}`} className={styles.button} type="button" data-value={option.value} data-testid={option.testId} aria-pressed={value === option.value} aria-label={option.ariaLabel} tabIndex={index === selectedIndex ? 0 : -1} onClick={() => onValueChange(option.value)} onKeyDown={onKeyDown} onPointerEnter={onOptionIntent ? () => onOptionIntent(option.value) : undefined} onFocus={onOptionIntent ? () => onOptionIntent(option.value) : undefined}>
        {value === option.value && <motion.span className={styles.selection} layoutId="selection" layoutDependency={value} transition={reduced ? { duration: 0 } : motionTokens.spring.morph} aria-hidden="true" />}
        <span className={styles.label}>{option.label}{option.accessory}</span>
      </button>)}
    </motion.div></LayoutGroup>
  </div>;
}
