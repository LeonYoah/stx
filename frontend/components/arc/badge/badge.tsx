/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/badge.json). Upstream license:
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

import { isValidElement, useEffect, useLayoutEffect, useRef } from "react";
import type { HTMLAttributes, ReactNode } from "react";
import { AnimatePresence, animate, motion, useIsPresent, useMotionValue, useReducedMotion, type AnimationPlaybackControls, type HTMLMotionProps, type TargetAndTransition, type Transition } from "motion/react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./badge.module.css";

export type BadgeTone = "neutral" | "success" | "info" | "warning" | "danger";
export type BadgeSize = "sm" | "md" | "lg";
export type BadgeColor = "teal" | "sky" | "orchid" | "violet" | "rose" | "amber";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
  size?: BadgeSize;
  icon?: ReactNode;
  /** A small status dot in the tone colour. Ignored when an icon is set. */
  dot?: boolean;
  /** Categorical colour from the shared tone family, for labels such as method tags. Overrides tone. */
  color?: BadgeColor;
  /** Draws a hairline border in the tone colour. */
  outline?: boolean;
  /** Fully rounded ends instead of the default soft corners. */
  pill?: boolean;
}

const exitFast: Transition = { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] };
const textIn: TargetAndTransition = { opacity: 0, y: "0.3em", filter: `blur(${motionTokens.blur.soft}px)` };
const textOut: TargetAndTransition = { opacity: 0, y: "-0.3em", filter: `blur(${motionTokens.blur.subtle}px)`, transition: exitFast };
const iconIn: TargetAndTransition = { opacity: 0, scale: .6, filter: `blur(${motionTokens.blur.subtle}px)` };
const shown: TargetAndTransition = { opacity: 1, y: "0em", scale: 1, filter: "blur(0px)" };
const fadeOnly: TargetAndTransition = { opacity: 0, transition: { duration: motionTokens.duration.instant } };

/** Outgoing copies are hidden from assistive tech while they fade, so only the current text is read. */
function Swap(props: HTMLMotionProps<"span">) {
  const present = useIsPresent();
  return <motion.span {...props} aria-hidden={present ? props["aria-hidden"] : true} />;
}

/** A new icon component crossfades in; re-rendering the same icon stays still. */
function iconKey(icon: ReactNode) {
  if (!isValidElement(icon)) {return "icon";}
  const type = icon.type as string | { displayName?: string; name?: string };
  return typeof type === "string" ? type : type.displayName ?? type.name ?? "icon";
}

export function Badge({ tone = "neutral", size = "md", icon, dot = false, color, outline = false, pill = false, className, children, ...props }: BadgeProps) {
  const reduce = useReducedMotion();
  const text = typeof children === "string" || typeof children === "number" ? String(children) : null;
  const glyphKey = icon ? iconKey(icon) : "";
  const body = useRef<HTMLSpanElement>(null);
  const content = useRef<HTMLSpanElement>(null);
  // Width stays auto at rest. Only a new label or icon springs it from the old size to the new one; passive reflows (a font swap, a hidden parent) follow instantly.
  const width = useMotionValue<number | "auto">("auto");
  const changedAt = useRef(0);
  useLayoutEffect(() => { changedAt.current = performance.now(); }, [text, glyphKey]);
  useEffect(() => {
    const node = content.current;
    if (!node || typeof ResizeObserver === "undefined") {return;}
    let last: number | undefined;
    let controls: AnimationPlaybackControls | undefined;
    const settle = () => { width.jump("auto"); if (body.current) {body.current.style.width = "auto";} };
    const observer = new ResizeObserver(([entry]) => {
      if (!entry) {return;}
      const next = entry.borderBoxSize?.[0]?.inlineSize ?? node.offsetWidth;
      const current = width.get();
      const from = typeof current === "number" ? current : last;
      last = next;
      controls?.stop();
      if (reduce || from === undefined || from === next || performance.now() - changedAt.current > 120) {return settle();}
      // Pin the old width before this frame paints, then spring to the new one.
      if (body.current) {body.current.style.width = `${from}px`;}
      controls = animate(width, [from, next], { ...motionTokens.spring.morph, onComplete: settle });
    });
    observer.observe(node);
    return () => { observer.disconnect(); controls?.stop(); };
  }, [width, reduce]);
  const enter: Transition = reduce ? { duration: motionTokens.duration.instant } : { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] };
  const mark = icon ? styles.withIcon : dot ? styles.withDot : undefined;
  const classes = [styles.badge, styles[tone], color ? styles[color] : undefined, styles[size], mark, outline ? styles.outline : undefined, pill ? styles.pill : undefined, className].filter(Boolean).join(" ");
  // The pill follows its content: text and icon swap in place while the width springs to the new size.
  return <span {...props} className={classes}>
    <motion.span ref={body} className={styles.body} style={{ width }}>
      <span ref={content} className={styles.content}>
        {!icon && dot ? <span className={styles.icon} aria-hidden="true"><span className={styles.dot} /></span> : null}
        {icon ? <span className={styles.icon} aria-hidden="true"><AnimatePresence mode="popLayout" initial={false}><Swap key={glyphKey} className={styles.glyph} initial={reduce ? { opacity: 0 } : iconIn} animate={shown} exit={reduce ? fadeOnly : { ...iconIn, transition: exitFast }} transition={reduce ? enter : motionTokens.spring.snappy}>{icon}</Swap></AnimatePresence></span> : null}
        {text === null ? children : <span className={styles.label}><AnimatePresence mode="popLayout" initial={false}><Swap key={text} className={styles.text} initial={reduce ? { opacity: 0 } : textIn} animate={shown} exit={reduce ? fadeOnly : textOut} transition={enter}>{text}</Swap></AnimatePresence></span>}
      </span>
    </motion.span>
  </span>;
}

export default Badge;
