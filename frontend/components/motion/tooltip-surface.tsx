/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Status Bar (https://beui.dev/charts/status-bar).
 * Registry: https://beui.dev/r/status-bar.json
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

import {motion, useReducedMotion} from 'motion/react';
import type {ComponentProps, ReactNode, Ref} from 'react';
import {EASE_OUT} from '@/lib/ease';
import {cn} from '@/lib/utils';

/** Presentation only: the unanimated parent owns measurement and positioning. */
export function TooltipSurface({
  children,
  className,
  ref,
  ready = true,
  style,
  ...props
}: Omit<ComponentProps<typeof motion.span>, 'children'> & {
  children?: ReactNode;
  /** Start the entrance only after the positioning layer has been measured. */
  ready?: boolean;
  ref?: Ref<HTMLSpanElement>;
}) {
  const reduce = useReducedMotion();
  const closed = {opacity: 0, scale: reduce ? 1 : 0.94};
  return (
    <motion.span
      ref={ref}
      role='tooltip'
      initial={closed}
      animate={{
        ...(ready ? {opacity: 1, scale: 1} : closed),
        transition: {duration: 0.18, ease: EASE_OUT},
      }}
      exit={{...closed, transition: {duration: 0.12, ease: EASE_OUT}}}
      style={{transformOrigin: 'var(--tooltip-origin, center)', ...style}}
      className={cn(
        'block whitespace-nowrap rounded-lg border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground shadow-lg',
        className,
      )}
      {...props}
    >
      {children}
    </motion.span>
  );
}
