/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/copy-button.json). Upstream license:
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

import { useLayoutEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { CircleAlert, Copy } from "lucide-react";
import { motionTokens } from "../lib/motion-tokens";
import { useCopyFeedback } from "../lib/use-copy-feedback";
import styles from "./copy-button.module.css";

export interface CopyButtonProps {
  value: string;
  label?: string;
  className?: string;
  iconOnly?: boolean;
  variant?: "outline" | "plain";
  disabled?: boolean;
  onCopied?: () => void;
  /** 复制成功后的文案，默认英文，不传则保持原组件。Shown after a successful copy. Defaults to English. */
  copiedLabel?: string;
  /** 复制失败后的文案，默认英文。Shown when the clipboard write fails. Defaults to English. */
  failedLabel?: string;
}

/** Copy feedback is quick and quiet: the old icon and word fade out fast while the new ones fade in, with no blur and no
 *  per-letter motion. The button eases to the width of the current word on a spring with no overshoot, so a long label
 *  like "Copy model id" never leaves empty space next to "Copied", and nothing jumps. */
const ease = [...motionTokens.ease.standard] as [number, number, number, number];
const show = { duration: .16, ease } as const;
const hide = { duration: .1, ease } as const;
const instant = { duration: 0 } as const;
const resize = { type: "spring", visualDuration: .22, bounce: 0 } as const;

/** The success tick draws itself in one short stroke. */
function DrawnCheck({ reduced }: { reduced: boolean }) {
  return <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
    <motion.path d="M4 12l5 5L20 6" initial={reduced ? false : { pathLength: 0 }} animate={{ pathLength: 1 }} transition={{ duration: .22, ease, delay: .04 }} />
  </svg>;
}

export function CopyButton({ value, label = "Copy", className, iconOnly = false, variant = "outline", disabled, onCopied, copiedLabel = "Copied", failedLabel = "Failed" }: CopyButtonProps) {
  const { state, copy } = useCopyFeedback();
  const reduced = useReducedMotion() ?? false;
  const text = state === "copied" ? copiedLabel : state === "error" ? failedLabel : label;
  // Each word's natural width, measured off screen (and again when fonts load or the label changes).
  const measures = useRef<Record<string, HTMLSpanElement | null>>({});
  const [widths, setWidths] = useState<Record<string, number>>({});
  useLayoutEffect(() => {
    if (iconOnly) {return;}
    const read = () => setWidths(Object.fromEntries(Object.entries(measures.current).map(([key, node]) => [key, node?.offsetWidth ?? 0])));
    read();
    if (typeof ResizeObserver === "undefined") {return;}
    const observer = new ResizeObserver(read);
    Object.values(measures.current).forEach(node => { if (node) {observer.observe(node);} });
    return () => observer.disconnect();
  }, [label, copiedLabel, failedLabel, iconOnly]);
  const width = widths[state === "copied" ? "copied" : state === "error" ? "error" : "idle"];
  // The first measured width applies at once; only later changes ease, so the very first copy animates too.
  const [shown, setShown] = useState<number | undefined>(undefined);
  const first = shown === undefined;
  if (width !== undefined && width !== shown) {setShown(width);}

  async function handleCopy() {
    if (await copy(value)) {onCopied?.();}
  }

  return <><button
    type="button"
    className={[styles.button, iconOnly && styles.iconOnly, variant === "plain" && styles.plain, className].filter(Boolean).join(" ")}
    onClick={() => void handleCopy()}
    aria-label={label}
    data-copy-state={state}
    disabled={disabled}
  >
    <span className={styles.icon} aria-hidden="true">
      <AnimatePresence initial={false}>
        <motion.span key={state} className={styles.iconInner} data-state={state}
          initial={reduced ? { opacity: 0 } : { opacity: 0, scale: .85 }} animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, transition: reduced ? instant : hide }} transition={reduced ? instant : show}>
          {state === "copied" ? <DrawnCheck reduced={reduced} /> : state === "error" ? <CircleAlert size={16} strokeWidth={1.75} /> : <Copy size={16} strokeWidth={1.75} />}
        </motion.span>
      </AnimatePresence>
    </span>
    {!iconOnly && <motion.span className={styles.label} aria-hidden="true" initial={false} animate={width ? { width } : undefined} transition={reduced || first ? instant : resize}>
      <span ref={node => { measures.current.idle = node; }} className={styles.measure}>{label}</span>
      <span ref={node => { measures.current.copied = node; }} className={styles.measure}>{copiedLabel}</span>
      <span ref={node => { measures.current.error = node; }} className={styles.measure}>{failedLabel}</span>
      <AnimatePresence initial={false}>
        <motion.span key={text} className={styles.text} initial={reduced ? { opacity: 0 } : { opacity: 0, y: 2 }} animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, transition: reduced ? instant : hide }} transition={reduced ? instant : show}>{text}</motion.span>
      </AnimatePresence>
    </motion.span>}
  </button><span className={styles.srOnly} role="status" aria-live="polite">{state === "idle" ? "" : state === "error" ? `${label}: Could not copy` : `${label}: Copied`}</span></>;
}
