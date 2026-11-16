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

import {useIsPresent} from 'motion/react';
import {
  type CSSProperties,
  type ReactNode,
  type Ref,
  useCallback,
  useState,
} from 'react';
import {type TooltipSide, useTooltipPosition} from './use-position';

type PositionProps = Parameters<typeof useTooltipPosition>[0];

/** Readiness lives with the mounted overlay, not a trigger ref's attach/detach cycle. */
export function TooltipPositioner({
  children,
  ...position
}: Omit<PositionProps, 'open' | 'onPosition'> & {
  children: (ready: boolean, present: boolean, side: TooltipSide) => ReactNode;
}) {
  const present = useIsPresent();
  const [ready, setReady] = useState(false);
  const [side, setSide] = useState<TooltipSide>(position.side);
  const onPosition = useCallback((placed: TooltipSide) => {
    setReady(true);
    setSide(placed);
  }, []);
  useTooltipPosition({...position, open: present, onPosition});
  return (
    <span
      ref={position.floatingRef}
      inert={!present}
      aria-hidden={!present || undefined}
      className='pointer-events-none fixed left-0 top-0 z-[9999] w-max'
      style={{visibility: 'hidden', maxWidth: 'calc(100vw - 16px)'}}
    >
      {children(ready, present, side)}
    </span>
  );
}

// Each arrow is a 12px box whose base sits on the surface's inner border edge,
// so its fill covers the border beneath it and only the two outer edges stroke.
const ARROW: Record<TooltipSide, {path: string; style: CSSProperties}> = {
  top: {path: 'M0 6L6 12L12 6', style: {top: 'calc(100% - 6px)'}},
  bottom: {path: 'M0 6L6 0L12 6', style: {bottom: 'calc(100% - 6px)'}},
  left: {path: 'M6 0L12 6L6 12', style: {left: 'calc(100% - 6px)'}},
  right: {path: 'M6 0L0 6L6 12', style: {right: 'calc(100% - 6px)'}},
};

export function TooltipArrow({
  side,
  ref,
}: {
  side: TooltipSide;
  ref: Ref<SVGSVGElement>;
}) {
  const {path, style} = ARROW[side];
  return (
    <svg
      ref={ref}
      aria-hidden='true'
      width='12'
      height='12'
      viewBox='0 0 12 12'
      className='pointer-events-none absolute overflow-visible'
      style={style}
    >
      <path
        d={`${path}Z`}
        style={{fill: 'var(--tooltip-arrow-fill, transparent)'}}
      />
      <path
        d={path}
        fill='none'
        strokeWidth='1'
        style={{stroke: 'var(--tooltip-arrow-stroke, transparent)'}}
      />
    </svg>
  );
}
