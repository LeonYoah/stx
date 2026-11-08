/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/search-field.json). Upstream license:
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
import { forwardRef, useId, useRef } from "react";
import type { InputHTMLAttributes } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Search, X as Xmark } from "lucide-react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./search-field.module.css";
export interface SearchFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> { label: string; value: string; onValueChange: (value: string) => void; /** 清除按钮的无障碍名称，默认英文。Accessible name of the clear button. Defaults to English. */ clearLabel?: string }
export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(function SearchField({ label, value, onValueChange, id, className, clearLabel = "Clear search", ...props }, ref) {
  const generated = useId(); const controlId = id ?? generated; const reduced = useReducedMotion(); const inputRef = useRef<HTMLInputElement | null>(null);
  const setRefs = (node: HTMLInputElement | null) => { inputRef.current = node; if (typeof ref === "function") {ref(node);} else if (ref) {ref.current = node;} };
  // Clearing returns focus to the field, since the clear button unmounts under the pointer.
  function clear() { onValueChange(""); inputRef.current?.focus(); }
  return <div className={styles.field}><label htmlFor={controlId}>{label}</label><div className={styles.shell} data-filled={value ? "true" : undefined}>
    <Search width={18} height={18} aria-hidden="true"/><input {...props} ref={setRefs} id={controlId} type="search" value={value} onChange={event => onValueChange(event.target.value)} className={[styles.input, className].filter(Boolean).join(" ")} />
    {/* The clear button has a reserved slot, so the field never changes width when it appears or leaves. */}
    <span className={styles.clearSlot}><AnimatePresence initial={false}>{value ? <motion.button key="clear" type="button" tabIndex={0} onClick={clear} aria-label={clearLabel} initial={reduced ? { opacity: 0 } : { opacity: 0, scale: .8, filter: `blur(${motionTokens.blur.subtle}px)` }} animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }} exit={reduced ? { opacity: 0, transition: { duration: 0 } } : { opacity: 0, scale: .8, filter: `blur(${motionTokens.blur.subtle}px)`, transition: { duration: motionTokens.duration.instant, ease: [...motionTokens.ease.standard] } }} whileTap={reduced ? undefined : { scale: .96, transition: { duration: motionTokens.duration.instant, ease: [...motionTokens.ease.standard] } }} transition={reduced ? { duration: motionTokens.duration.instant } : { ...motionTokens.spring.snappy, opacity: { duration: motionTokens.duration.fast }, filter: { duration: motionTokens.duration.fast } }}><Xmark width={16} height={16} aria-hidden="true" /></motion.button> : null}</AnimatePresence></span>
  </div></div>;
});
