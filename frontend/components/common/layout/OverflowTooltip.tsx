/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

'use client';

/**
 * 零静止开销的按需溢出提示原语：
 * - 未悬停/未聚焦时零 Portal、零 Radix Provider/Popper 实例、零 ResizeObserver、零全局事件监听；
 * - 悬停未截断元素时仅做一次 scrollWidth/scrollHeight 判断，不触发任何 React 重渲染；
 * - 仅在真正溢出（或 alwaysShow）时于延迟后通过 Portal + @floating-ui/dom 挂载单个浮层，
 *   不包裹也不重挂触发元素，避免丢焦点或打断点击；共享 300ms 温热窗口让相邻单元格连续浏览零延迟。
 * Zero-rest-overhead lazy overflow tooltip primitive:
 * - Zero portals, Radix Provider/Popper instances, ResizeObservers, or global listeners at rest;
 * - Hovering a non-overflowing element performs a single scrollWidth/scrollHeight check with zero re-renders;
 * - Mounts a single @floating-ui/dom portal only when actually overflowing (or alwaysShow) without wrapping
 *   or remounting the trigger; shares a 300ms warm skip-window for smooth adjacent cell scanning.
 */
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type PointerEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import {createPortal} from 'react-dom';
import {computePosition, flip, offset, shift} from '@floating-ui/dom';
import {cn} from '@/lib/utils';

export type OverflowTooltipSide = 'top' | 'bottom' | 'left' | 'right';

const DEFAULT_DELAY_MS = 180;
const WARM_SKIP_WINDOW_MS = 300;

// 模块级温热窗口：不订阅 React 状态，仅在悬停进入时读取一次，避免打开一个浮层触发全表重渲染。
// Module-level warm window: read lazily on pointer enter with zero React subscriptions so opening one tooltip never re-renders sibling cells.
let warmUntil = 0;

/** 仅供单测重置模块级温热窗口 / Test-only helper to reset the shared warm skip-window. */
export function resetOverflowTooltipWarmth(): void {
  warmUntil = 0;
}

function isElementOverflowing(el: HTMLElement): boolean {
  return (
    el.scrollWidth > el.clientWidth + 1 ||
    el.scrollHeight > el.clientHeight + 2
  );
}

interface OverflowTooltipPortalProps {
  id: string;
  anchorEl: HTMLElement;
  side: OverflowTooltipSide;
  content: ReactNode;
  contentClassName?: string;
  onClose: () => void;
}

function OverflowTooltipPortal({
  id,
  anchorEl,
  side,
  content,
  contentClassName,
  onClose,
}: OverflowTooltipPortalProps) {
  const floatingRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const floatingEl = floatingRef.current;
    if (!floatingEl) {
      return;
    }

    let cancelled = false;
    void computePosition(anchorEl, floatingEl, {
      placement: side,
      strategy: 'fixed',
      middleware: [offset(6), flip({padding: 8}), shift({padding: 8})],
    }).then(({x, y, placement}) => {
      if (cancelled || !floatingRef.current) {
        return;
      }
      floatingRef.current.style.left = `${Math.round(x)}px`;
      floatingRef.current.style.top = `${Math.round(y)}px`;
      floatingRef.current.dataset.side = placement.split('-')[0];
    });

    return () => {
      cancelled = true;
    };
  }, [anchorEl, side, content]);

  useEffect(() => {
    // 仅在浮层实际打开期间挂载滚动与 Esc 关闭监听，关闭即移除。
    // Attach scroll/Escape dismissal listeners only while this single portal is mounted.
    const handleScroll = () => onClose();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('scroll', handleScroll, {
      capture: true,
      passive: true,
    });
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('scroll', handleScroll, {capture: true});
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  if (typeof document === 'undefined') {
    return null;
  }

  return createPortal(
    <div
      ref={floatingRef}
      id={id}
      role='tooltip'
      data-slot='overflow-tooltip'
      data-side={side}
      className={cn(
        // z-[90] 对齐 Arc 浮层层级（dialog 51、dropdown 60、tooltip 90）。
        // z-[90] matches the Arc overlay scale (dialog 51, dropdown 60, tooltip 90).
        'pointer-events-none fixed left-0 top-0 z-[90] max-w-[min(90vw,480px)] rounded-[var(--radius-popover,8px)] border border-background/15 bg-foreground px-2.5 py-1.5 text-left text-xs leading-relaxed text-background shadow-md whitespace-pre-wrap [overflow-wrap:anywhere] animate-in fade-in-0 zoom-in-95 duration-100',
        contentClassName,
      )}
    >
      {content}
    </div>,
    document.body,
  );
}

export interface UseOverflowTooltipOptions {
  /** 用于判定是否溢出的目标元素 / Target element measured for overflow */
  targetRef: RefObject<HTMLElement | null>;
  /** 浮层定位锚点，默认与 targetRef 相同 / Positioning anchor element; defaults to targetRef */
  anchorRef?: RefObject<HTMLElement | null>;
  /** 提示内容；为空时禁用 / Tooltip content; disabled when nullish or empty */
  content: ReactNode;
  /** 即使未截断也展示提示 / Show tooltip even when content does not overflow */
  alwaysShow?: boolean;
  /** 首选弹出方向，默认 top / Preferred placement, defaults to top */
  side?: OverflowTooltipSide;
  /** 首次悬停延迟（毫秒），默认 180 / Initial hover delay in ms, defaults to 180 */
  delayMs?: number;
  /** 显式禁用提示 / Explicitly disable tooltip handling */
  disabled?: boolean;
  contentClassName?: string;
}

export interface OverflowTooltipTriggerProps {
  onPointerEnter?: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave?: (event: PointerEvent<HTMLElement>) => void;
  onPointerDown?: (event: PointerEvent<HTMLElement>) => void;
  onFocus?: (event: FocusEvent<HTMLElement>) => void;
  onBlur?: (event: FocusEvent<HTMLElement>) => void;
  'aria-describedby'?: string;
}

export interface UseOverflowTooltipResult {
  open: boolean;
  triggerProps: OverflowTooltipTriggerProps;
  portal: ReactNode;
}

export function useOverflowTooltip({
  targetRef,
  anchorRef,
  content,
  alwaysShow = false,
  side = 'top',
  delayMs = DEFAULT_DELAY_MS,
  disabled = false,
  contentClassName,
}: UseOverflowTooltipOptions): UseOverflowTooltipResult {
  const tooltipId = useId();
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const timerRef = useRef<number | null>(null);

  const hasContent =
    !disabled &&
    content !== null &&
    content !== undefined &&
    content !== false &&
    content !== '';

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const close = useCallback(() => {
    clearTimer();
    setAnchorEl((prev) => {
      if (prev !== null) {
        warmUntil = Date.now() + WARM_SKIP_WINDOW_MS;
      }
      return null;
    });
  }, [clearTimer]);

  const scheduleOpen = useCallback(
    (immediate: boolean) => {
      if (!hasContent) {
        return;
      }
      const target = targetRef.current;
      if (!target) {
        return;
      }
      if (!alwaysShow && !isElementOverflowing(target)) {
        return;
      }
      const anchor = anchorRef?.current ?? target;
      clearTimer();

      const now = Date.now();
      if (immediate || delayMs <= 0 || now < warmUntil) {
        warmUntil = now + WARM_SKIP_WINDOW_MS;
        setAnchorEl(anchor);
        return;
      }

      timerRef.current = window.setTimeout(() => {
        timerRef.current = null;
        // 延迟结束时再次确认元素仍在文档中且依然溢出，防止快速切页残留浮层。
        // Re-check connection and overflow after the delay to prevent stale tooltips on fast navigation.
        const currentTarget = targetRef.current;
        const currentAnchor = anchorRef?.current ?? currentTarget;
        if (
          !currentTarget ||
          !currentAnchor ||
          !currentAnchor.isConnected ||
          (!alwaysShow && !isElementOverflowing(currentTarget))
        ) {
          return;
        }
        warmUntil = Date.now() + WARM_SKIP_WINDOW_MS;
        setAnchorEl(currentAnchor);
      }, delayMs);
    },
    [alwaysShow, anchorRef, clearTimer, delayMs, hasContent, targetRef],
  );

  useEffect(() => {
    return () => {
      clearTimer();
    };
  }, [clearTimer]);

  const handlePointerEnter = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (event.pointerType === 'touch') {
        return;
      }
      scheduleOpen(false);
    },
    [scheduleOpen],
  );

  const handleFocus = useCallback(() => {
    scheduleOpen(true);
  }, [scheduleOpen]);

  const open = anchorEl !== null && hasContent;

  const triggerProps: OverflowTooltipTriggerProps = hasContent
    ? {
        onPointerEnter: handlePointerEnter,
        onPointerLeave: close,
        onPointerDown: close,
        onFocus: handleFocus,
        onBlur: close,
        'aria-describedby': open ? tooltipId : undefined,
      }
    : {};

  const portal = open ? (
    <OverflowTooltipPortal
      id={tooltipId}
      anchorEl={anchorEl}
      side={side}
      content={content}
      contentClassName={contentClassName}
      onClose={close}
    />
  ) : null;

  return {open, triggerProps, portal};
}
