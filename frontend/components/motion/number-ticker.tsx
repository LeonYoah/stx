/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Sparkline dependency (https://beui.dev/charts/sparkline).
 * Registry: https://beui.dev/r/sparkline.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * Local STX adaptations remain under the same MIT terms and are not
 * relicensed as Apache-2.0.
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

import {animate, motion, useInView, useReducedMotion} from 'motion/react';
import {useEffect, useMemo, useRef, useState} from 'react';
import {EASE_OUT} from '@/lib/ease';
import {useLocale} from '@/components/charts/beui-i18n';
import {cn} from '@/lib/utils';

export interface NumberTickerProps {
  value: number;
  /** Digits to pad to (left). */
  pad?: number;
  /**
   * Fraction digits to keep. The value is rounded to this many places before
   * `format` runs (default 0, whole numbers).
   */
  decimals?: number;
  /** Per-digit roll duration in seconds. */
  duration?: number;
  /** Stagger between digits. */
  stagger?: number;
  /** Render only after the element enters the viewport. */
  startOnView?: boolean;
  prefix?: string;
  suffix?: string;
  /** Add a small blur during digit rolls. */
  blur?: boolean;
  className?: string;
  digitClassName?: string;
  /**
   * Insert locale group separators. `true` uses the `LocaleProvider` locale
   * (or the runtime default without one); a BCP 47 tag such as `"de-DE"` pins it.
   */
  locale?: boolean | string;
  /** Custom formatter. Client-only — server components must use `locale` instead. */
  format?: (value: number) => string;
}

const DIGIT_HEIGHT_EM = 1.1;
const DIGITS = Array.from({length: 10}, (_, n) => n);

export function NumberTicker({
  value,
  pad,
  decimals = 0,
  duration = 0.9,
  stagger = 0.04,
  startOnView = true,
  prefix,
  suffix,
  blur = false,
  className,
  digitClassName,
  locale,
  format,
}: NumberTickerProps) {
  const containerRef = useRef<HTMLSpanElement>(null);
  const inView = useInView(containerRef, {once: true, amount: 0.6});
  const [armed, setArmed] = useState(!startOnView);
  const {locale: contextLocale} = useLocale();

  useEffect(() => {
    if (startOnView && inView) {
      setArmed(true);
    }
  }, [startOnView, inView]);

  const text = useMemo(() => {
    const places = Math.max(0, Math.floor(decimals));
    const factor = 10 ** places;
    const rounded = Math.round(value * factor) / factor;
    const formatted = format
      ? format(rounded)
      : locale
        ? rounded.toLocaleString(
            typeof locale === 'string' ? locale : contextLocale,
            {
              minimumFractionDigits: places,
              maximumFractionDigits: places,
            },
          )
        : rounded.toFixed(places);
    return pad ? formatted.padStart(pad, '0') : formatted;
  }, [value, pad, decimals, format, locale, contextLocale]);
  const glyphs = useMemo(() => {
    const chars = text.split('');
    // Key by place value (position from the right): a changing digit keeps its
    // identity and rolls to the new value instead of remounting and replaying
    // from 0. Growing numbers add glyphs on the left without re-keying the
    // ones, tens, hundreds already on screen.
    return chars.map((char, i) => ({char, id: `g-${chars.length - 1 - i}`}));
  }, [text]);
  const readableText = `${prefix ?? ''}${text}${suffix ?? ''}`;

  // Stagger is an entrance flourish. Once the reveal has played, value
  // changes roll every digit immediately — a per-digit delay on live updates
  // reads as lag.
  const [entered, setEntered] = useState(false);
  useEffect(() => {
    if (!armed || entered) {
      return;
    }
    const total = (duration + glyphs.length * stagger) * 1000;
    const t = window.setTimeout(() => setEntered(true), total);
    return () => window.clearTimeout(t);
  }, [armed, entered, duration, stagger, glyphs.length]);

  return (
    <span
      ref={containerRef}
      className={cn('inline-flex items-baseline tabular-nums', className)}
    >
      <span className='sr-only'>{readableText}</span>
      <span aria-hidden='true' className='inline-flex items-baseline'>
        {prefix ? <span>{prefix}</span> : null}
        {glyphs.map(({char, id}, i) => {
          const isDigit = /\d/.test(char);
          if (!isDigit) {
            return (
              <span key={id} className='inline-block'>
                {char}
              </span>
            );
          }
          const digit = Number(char);
          return (
            <Digit
              key={id}
              digit={armed ? digit : 0}
              delay={entered ? 0 : i * stagger}
              duration={duration}
              blur={blur}
              className={digitClassName}
            />
          );
        })}
        {suffix ? <span>{suffix}</span> : null}
      </span>
    </span>
  );
}

function Digit({
  digit,
  delay,
  duration,
  blur,
  className,
}: {
  digit: number;
  delay: number;
  duration: number;
  blur: boolean;
  className?: string;
}) {
  const reduce = useReducedMotion();
  const columnRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (reduce || !blur || !columnRef.current || !Number.isFinite(digit)) {
      return;
    }

    const node = columnRef.current;
    const controls = animate(
      node,
      {filter: ['blur(10px)', 'blur(0px)']},
      {
        duration: Math.min(duration * 0.75, 0.32),
        delay,
        ease: EASE_OUT,
      },
    );

    return () => {
      controls.stop();
      node.style.filter = 'blur(0px)';
    };
  }, [blur, delay, digit, duration, reduce]);

  return (
    // The invisible in-flow digit gives the slot a real text baseline so it
    // lines up with surrounding text; clip-path clips the rolling column
    // without moving that baseline the way overflow: hidden would.
    <span
      className={cn(
        'relative inline-block text-center [clip-path:inset(0)]',
        className,
      )}
      style={{
        height: `${DIGIT_HEIGHT_EM}em`,
        lineHeight: `${DIGIT_HEIGHT_EM}em`,
        width: '1ch',
      }}
    >
      <span className='invisible'>{digit}</span>
      <motion.span
        ref={columnRef}
        initial={{y: 0}}
        animate={{y: `-${digit * DIGIT_HEIGHT_EM}em`}}
        transition={reduce ? {duration: 0} : {duration, delay, ease: EASE_OUT}}
        className='absolute inset-x-0 top-0 flex flex-col items-center will-change-[transform,filter]'
      >
        {DIGITS.map((n) => (
          <span
            key={n}
            className='flex h-[1.1em] items-center justify-center leading-none'
          >
            {n}
          </span>
        ))}
      </motion.span>
    </span>
  );
}
