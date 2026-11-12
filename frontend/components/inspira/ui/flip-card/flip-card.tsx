/*
 * MIT License
 *
 * Copyright (c) 2024-2026 rahulv.dev
 *
 * Adapted from Inspira UI FlipCard.vue:
 * https://github.com/unovue/inspira-ui/blob/main/app/components/inspira/ui/flip-card/FlipCard.vue
 * Docs: https://inspira-ui.com/docs/cn/components/cards/flip-card
 * Upstream license: https://github.com/unovue/inspira-ui/blob/main/LICENSE
 *
 * Local STX adaptations (React + Motion, click-controlled flip instead of
 * group-hover, reduced-motion crossfade, a11y faces) remain under the same MIT
 * terms and are not relicensed as Apache-2.0.
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

import {
  useEffect,
  useRef,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from 'react';
import {motion} from 'motion/react';
import {cn} from '@/lib/utils';

/** Inspira-aligned 3D flip duration (~500ms). / 对齐 Inspira 的约 500ms 翻转时长。 */
const FLIP_TRANSITION = {
  type: 'tween' as const,
  duration: 0.5,
  ease: [0.22, 1, 0.36, 1] as [number, number, number, number],
};

const REDUCE_FADE = {
  type: 'tween' as const,
  duration: 0.18,
  ease: 'easeOut' as const,
};

export type FlipCardRotate = 'x' | 'y';

export type FlipCardProps = {
  flipped: boolean;
  onFlippedChange: (next: boolean) => void;
  front: ReactNode;
  back: ReactNode;
  /** Prefer y to match Inspira default. / 默认 y 轴，与 Inspira 一致。 */
  rotate?: FlipCardRotate;
  reduceMotion?: boolean;
  className?: string;
  /**
   * Return true to ignore activate (nested button/link/menu).
   * 返回 true 时忽略激活（嵌套按钮/链接/菜单）。
   */
  shouldIgnoreActivate?: (
    target: EventTarget | null,
    currentTarget: EventTarget | null,
  ) => boolean;
  /** Accessible name for the flip shell. / 翻转外壳的无障碍名称。 */
  flipLabel: string;
  'data-testid'?: string;
};

/**
 * Click-to-flip card shell ported from Inspira FlipCard (hover → controlled click).
 * 自 Inspira FlipCard 移植的点击翻转外壳（hover 改为受控点击）。
 *
 * layoutId morph must stay on an ancestor — never on this rotating layer.
 * layoutId 形变必须挂在祖先节点，绝不可与本层 rotate 叠加。
 */
export function FlipCard({
  flipped,
  onFlippedChange,
  front,
  back,
  rotate = 'y',
  reduceMotion = false,
  className,
  shouldIgnoreActivate,
  flipLabel,
  'data-testid': dataTestId = 'inspira-flip-card',
}: FlipCardProps) {
  const frontRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLDivElement>(null);
  const prevFlipped = useRef(flipped);

  // 翻面后把焦点移到可见面，避免落在 inert 控件上。
  // After flip, move focus onto the visible face so it never sits on inert controls.
  useEffect(() => {
    if (prevFlipped.current === flipped) {
      return;
    }
    prevFlipped.current = flipped;
    const face = flipped ? backRef.current : frontRef.current;
    face?.focus({preventScroll: true});
  }, [flipped]);

  const activate = (
    event: MouseEvent<HTMLElement> | KeyboardEvent<HTMLElement>,
  ) => {
    if (
      shouldIgnoreActivate?.(event.target, event.currentTarget) ||
      false
    ) {
      return;
    }
    onFlippedChange(!flipped);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') {
      return;
    }
    if (shouldIgnoreActivate?.(event.target, event.currentTarget)) {
      return;
    }
    event.preventDefault();
    onFlippedChange(!flipped);
  };

  if (reduceMotion) {
    return (
      <div
        data-testid={dataTestId}
        data-flipped={flipped ? 'true' : 'false'}
        data-reduce-motion='true'
        role='button'
        tabIndex={0}
        aria-pressed={flipped}
        aria-label={flipLabel}
        className={cn(
          // 默认高度；市场侧通过 className 覆写到更大响应式带（tailwind-merge）。
          // Default height; marketplace overrides via className into a taller responsive band.
          'relative h-[15.5rem] w-full cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
          className,
        )}
        onClick={activate}
        onKeyDown={onKeyDown}
      >
        <div
          ref={frontRef}
          tabIndex={-1}
          data-testid={`${dataTestId}-front-shell`}
          aria-hidden={flipped || undefined}
          {...(flipped ? ({inert: true} as Record<string, unknown>) : {})}
          className={cn(
            'absolute inset-0 overflow-hidden rounded-2xl border border-border/70 bg-card outline-none transition-opacity',
            flipped ? 'pointer-events-none opacity-0' : 'opacity-100',
          )}
        >
          {front}
        </div>
        <div
          ref={backRef}
          tabIndex={-1}
          data-testid={`${dataTestId}-back-shell`}
          aria-hidden={!flipped || undefined}
          {...(!flipped ? ({inert: true} as Record<string, unknown>) : {})}
          className={cn(
            'absolute inset-0 overflow-hidden rounded-2xl border border-border/70 bg-card outline-none transition-opacity',
            flipped ? 'opacity-100' : 'pointer-events-none opacity-0',
          )}
          style={{transitionDuration: `${REDUCE_FADE.duration * 1000}ms`}}
        >
          {back}
        </div>
      </div>
    );
  }

  const axis = rotate === 'x' ? 'rotateX' : 'rotateY';

  return (
    <div
      data-testid={dataTestId}
      data-flipped={flipped ? 'true' : 'false'}
      data-reduce-motion='false'
      role='button'
      tabIndex={0}
      aria-pressed={flipped}
      aria-label={flipLabel}
      // Inspira demo 固定 h-72 w-56；市场卡满宽，高度由 className 响应式放大。
      // Inspira demo pins h-72 w-56; marketplace is full-width with responsive height via className.
      className={cn(
        'relative h-[15.5rem] w-full cursor-pointer perspective-[1000px] outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
        className,
      )}
      onClick={activate}
      onKeyDown={onKeyDown}
    >
      <motion.div
        className='relative h-full w-full rounded-2xl [transform-style:preserve-3d]'
        animate={{[axis]: flipped ? 180 : 0}}
        transition={FLIP_TRANSITION}
        data-testid={`${dataTestId}-layer`}
      >
        {/* Front — Inspira: absolute size-full … backface-hidden */}
        <div
          ref={frontRef}
          tabIndex={-1}
          data-testid={`${dataTestId}-front-shell`}
          aria-hidden={flipped || undefined}
          {...(flipped ? ({inert: true} as Record<string, unknown>) : {})}
          className='absolute inset-0 overflow-hidden rounded-2xl border border-border/70 bg-card outline-none [backface-visibility:hidden]'
          style={{
            backfaceVisibility: 'hidden',
            WebkitBackfaceVisibility: 'hidden',
            transform: `${axis}(0deg)`,
          }}
        >
          {front}
        </div>

        {/* Back — Inspira: absolute … backface-hidden + pre-rotated 180° */}
        <div
          ref={backRef}
          tabIndex={-1}
          data-testid={`${dataTestId}-back-shell`}
          aria-hidden={!flipped || undefined}
          {...(!flipped ? ({inert: true} as Record<string, unknown>) : {})}
          className='absolute inset-0 overflow-hidden rounded-2xl border border-border/70 bg-muted/40 outline-none [backface-visibility:hidden]'
          style={{
            backfaceVisibility: 'hidden',
            WebkitBackfaceVisibility: 'hidden',
            transform: `${axis}(180deg)`,
          }}
        >
          {back}
        </div>
      </motion.div>
    </div>
  );
}
