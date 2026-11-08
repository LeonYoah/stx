/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * This file is from Arc UI (https://uiarc.dev), installed via the public
 * @uiarc shadcn registry (https://uiarc.dev/r/switch.json). Upstream license:
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

import { forwardRef, useEffect, useRef, useState } from "react";
import type { ComponentPropsWithoutRef, ElementRef } from "react";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { animate, motion, useMotionValue, useReducedMotion } from "motion/react";
import type { Transition } from "motion/react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./switch.module.css";

export interface SwitchProps extends ComponentPropsWithoutRef<typeof SwitchPrimitive.Root> {
  label?: string;
}

/** Track inner width (42 - 6 padding) minus the 18px thumb; keep in sync with switch.module.css. */
const size = 18;
const travel = 18;
/** How far the thumb widens toward the other side while pressed. */
const stretch = 5;
/** Critically damped: the thumb lands on its end without overshooting the state it reports. */
const glide: Transition = { type: "spring", visualDuration: 0.3, bounce: 0 };

export const Switch = forwardRef<ElementRef<typeof SwitchPrimitive.Root>, SwitchProps>(function Switch(
  { label, className, checked, defaultChecked, onCheckedChange, onPointerDown, onPointerUp, onPointerLeave, onPointerCancel, onKeyDown, onKeyUp, onBlur, ...props },
  ref,
) {
  const reduceMotion = useReducedMotion();
  const [internal, setInternal] = useState(defaultChecked ?? false);
  const [pressed, setPressed] = useState(false);
  const on = checked ?? internal;
  // A brief stretch along the travel, so the thumb reads as moving mass rather than a sliding dot.
  const scaleX = useMotionValue(1);
  const shown = useRef(on);
  // A pointer or Space press already stretched the thumb, so its release should not add a second stretch on top.
  const releasedAt = useRef(-Infinity);
  useEffect(() => {
    if (shown.current === on) {return;}
    shown.current = on;
    const fromPress = performance.now() - releasedAt.current < 250;
    if (reduceMotion || fromPress) {return;}
    const controls = animate(scaleX, [1, 1.16, 1], { duration: 0.34, times: [0, 0.4, 1], ease: ["easeOut", "easeInOut"] });
    return () => controls.stop();
  }, [on, reduceMotion, scaleX]);
  const extra = pressed && !reduceMotion && !props.disabled ? stretch : 0;
  const classes = [styles.switch, className].filter(Boolean).join(" ");

  return (
    <SwitchPrimitive.Root
      {...props}
      ref={ref}
      checked={on}
      onCheckedChange={next => { if (checked === undefined) {setInternal(next);} onCheckedChange?.(next); }}
      onPointerDown={event => { onPointerDown?.(event); if (event.button === 0) {setPressed(true);} }}
      onPointerUp={event => { onPointerUp?.(event); if (pressed && !props.disabled) {releasedAt.current = performance.now();} setPressed(false); }}
      onPointerLeave={event => { onPointerLeave?.(event); setPressed(false); }}
      onPointerCancel={event => { onPointerCancel?.(event); setPressed(false); }}
      onKeyDown={event => { onKeyDown?.(event); if (event.key === " ") {setPressed(true);} }}
      onKeyUp={event => { onKeyUp?.(event); if (pressed && !props.disabled) {releasedAt.current = performance.now();} setPressed(false); }}
      onBlur={event => { onBlur?.(event); setPressed(false); }}
      className={classes}
      aria-label={props["aria-label"] ?? label}
    >
      <span className={styles.track}>
        {/* The thumb stretches like a held finger and keeps its far edge anchored, then travels on a spring. */}
        <motion.span className={styles.thumb} style={{ scaleX }} initial={false} animate={{ x: on ? travel - extra : 0, width: size + extra }} transition={reduceMotion ? { duration: 0 } : { x: glide, width: motionTokens.spring.snappy }} />
      </span>
      {label ? <span className={styles.label}>{label}</span> : null}
    </SwitchPrimitive.Root>
  );
});

Switch.displayName = "Switch";

export default Switch;
