/*
 * MIT License
 *
 * Copyright (c) 2024-2026 rahulv.dev
 *
 * Adapted from Inspira UI apple-card-carousel:
 * https://github.com/unovue/inspira-ui/blob/main/app/components/inspira/ui/apple-card-carousel/
 * Docs: https://inspira-ui.com/docs/cn/components/cards/apple-card-carousel
 * Upstream license: https://github.com/unovue/inspira-ui/blob/main/LICENSE
 *
 * Local STX adaptations (React + Motion, marketplace package data, no blank-click
 * fullscreen detail, reduced-motion) remain under the same MIT terms and are not
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

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import {motion, useReducedMotion} from 'motion/react';
import {ChevronLeft, ChevronRight} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {cn} from '@/lib/utils';

export type AppleCardCarouselProps = {
  children: ReactNode;
  className?: string;
  /** Accessible label for the carousel region. / 轮播区域无障碍名称。 */
  'aria-label'?: string;
  'data-testid'?: string;
};

/**
 * Horizontal Apple-style card carousel (Inspira port): scroll + arrows + keyboard.
 * Inspira Apple 卡片轮播的 React 移植：横向滚动、箭头与键盘。
 *
 * Blank card clicks do not open a fullscreen layer here — STX keeps actions explicit.
 * 空白点击不打开全屏层；STX 侧操作保持显式按钮。
 */
export function AppleCardCarousel({
  children,
  className,
  'aria-label': ariaLabel = 'Card carousel',
  'data-testid': dataTestId = 'apple-card-carousel',
}: AppleCardCarouselProps) {
  const reduceMotion = useReducedMotion() ?? false;
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const checkScrollability = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) {
      return;
    }
    const {scrollLeft, scrollWidth, clientWidth} = el;
    setCanScrollLeft(scrollLeft > 2);
    setCanScrollRight(scrollLeft < scrollWidth - clientWidth - 2);
  }, []);

  useEffect(() => {
    checkScrollability();
    const el = scrollerRef.current;
    if (!el) {
      return;
    }
    const onResize = () => checkScrollability();
    window.addEventListener('resize', onResize);
    el.addEventListener('scroll', checkScrollability, {passive: true});
    return () => {
      window.removeEventListener('resize', onResize);
      el.removeEventListener('scroll', checkScrollability);
    };
  }, [checkScrollability, children]);

  const scrollByDir = (dir: -1 | 1) => {
    const el = scrollerRef.current;
    if (!el) {
      return;
    }
    const delta = Math.min(360, Math.max(220, el.clientWidth * 0.7)) * dir;
    el.scrollBy({
      left: delta,
      behavior: reduceMotion ? 'auto' : 'smooth',
    });
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      scrollByDir(-1);
    } else if (event.key === 'ArrowRight') {
      event.preventDefault();
      scrollByDir(1);
    }
  };

  return (
    <div
      className={cn('relative w-full', className)}
      data-testid={dataTestId}
      data-reduce-motion={reduceMotion ? 'true' : 'false'}
    >
      <div
        ref={scrollerRef}
        role='region'
        aria-label={ariaLabel}
        tabIndex={0}
        onKeyDown={onKeyDown}
        // Inspira: flex overflow-x-scroll overscroll-x-auto scroll-smooth …
        className={cn(
          'flex w-full overflow-x-auto overscroll-x-contain py-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
          !reduceMotion && 'scroll-smooth',
        )}
        data-testid={`${dataTestId}-scroller`}
      >
        <div className='mx-auto flex max-w-7xl flex-row justify-start gap-3 pl-1 pr-8 md:gap-4 md:pr-[20%]'>
          {children}
        </div>
      </div>

      <div className='mt-1 flex justify-end gap-2 pr-1'>
        <Button
          type='button'
          variant='secondary'
          size='icon'
          className='size-9 rounded-full'
          disabled={!canScrollLeft}
          aria-label='Previous'
          data-testid={`${dataTestId}-prev`}
          onClick={() => scrollByDir(-1)}
        >
          <ChevronLeft className='size-5' />
        </Button>
        <Button
          type='button'
          variant='secondary'
          size='icon'
          className='size-9 rounded-full'
          disabled={!canScrollRight}
          aria-label='Next'
          data-testid={`${dataTestId}-next`}
          onClick={() => scrollByDir(1)}
        >
          <ChevronRight className='size-5' />
        </Button>
      </div>
    </div>
  );
}

export type AppleCarouselItemProps = {
  index: number;
  children: ReactNode;
  className?: string;
};

/**
 * Staggered entrance item wrapper (Inspira AppleCarouselItem).
 * 入场交错的单项包装（Inspira AppleCarouselItem）。
 */
export function AppleCarouselItem({
  index,
  children,
  className,
}: AppleCarouselItemProps) {
  const reduceMotion = useReducedMotion() ?? false;
  return (
    <motion.div
      initial={reduceMotion ? false : {opacity: 0, y: 16}}
      animate={{opacity: 1, y: 0}}
      transition={
        reduceMotion
          ? {duration: 0}
          : {duration: 0.45, delay: Math.min(index, 8) * 0.08, ease: 'easeOut'}
      }
      className={cn('shrink-0', className)}
      data-testid={`apple-carousel-item-${index}`}
    >
      {children}
    </motion.div>
  );
}
