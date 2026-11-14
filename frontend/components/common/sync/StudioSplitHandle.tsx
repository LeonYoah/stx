/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

'use client';

import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
} from 'react';
import {cn} from '@/lib/utils';
import styles from './studio-split-handle.module.css';

const KEY_STEP = 16;
const KEY_BIG_STEP = 64;

export interface StudioSplitHandleProps {
  /** 分隔条自身方向：vertical 改左右宽，horizontal 改上下高。 / Separator axis: vertical resizes width, horizontal resizes height. */
  orientation: 'vertical' | 'horizontal';
  label: string;
  value: number;
  min: number;
  max: number;
  /** 拖向正方向时减小 value（右侧栏、控制台顶边）。 / Positive pointer delta shrinks value (right sidebar, console top). */
  invert?: boolean;
  onChange: (next: number) => void;
  onDragStart?: () => void;
  onDragEnd?: () => void;
  className?: string;
  style?: CSSProperties;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

const RESIZING_ATTR = 'data-studio-resizing';

/** 把拖动轴写到 html，用 CSS 锁住整页光标。 / Stamp the drag axis on html so CSS can lock the page cursor. */
function lockResizeCursor(axis: 'x' | 'y') {
  document.documentElement.setAttribute(RESIZING_ATTR, axis);
}

/** 拖动结束或卸载时清掉光标锁。 / Drop the cursor lock when the drag ends or the handle unmounts. */
function unlockResizeCursor() {
  document.documentElement.removeAttribute(RESIZING_ATTR);
}

/**
 * 贴在卡片接缝上的 Arc 风格分隔条：平时是一条线，悬停才出握把。
 * Arc-style divider on the card seam: a hairline at rest, grip only on hover.
 */
export function StudioSplitHandle({
  orientation,
  label,
  value,
  min,
  max,
  invert = false,
  onChange,
  onDragStart,
  onDragEnd,
  className,
  style,
}: StudioSplitHandleProps) {
  const dragRef = useRef<{
    pointerId: number;
    start: number;
    origin: number;
  } | null>(null);
  const [active, setActive] = useState(false);
  const isVertical = orientation === 'vertical';

  useEffect(() => {
    return () => {
      if (dragRef.current) {
        unlockResizeCursor();
      }
    };
  }, []);

  const applyDelta = (delta: number) => {
    const signed = invert ? -delta : delta;
    onChange(clamp(Math.round(value + signed), min, max));
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType === 'mouse' && event.button !== 0) {
      return;
    }
    // jsdom 没有 Pointer Capture；浏览器里用来拖过编辑器时仍收到事件。
    // jsdom has no Pointer Capture; in the browser this keeps events while crossing the editor.
    event.currentTarget.setPointerCapture?.(event.pointerId);
    dragRef.current = {
      pointerId: event.pointerId,
      start: isVertical ? event.clientX : event.clientY,
      origin: value,
    };
    setActive(true);
    lockResizeCursor(isVertical ? 'x' : 'y');
    onDragStart?.();
  };

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    const current = isVertical ? event.clientX : event.clientY;
    const signed = invert
      ? drag.origin - (current - drag.start)
      : drag.origin + (current - drag.start);
    onChange(clamp(Math.round(signed), min, max));
  };

  const finish = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    dragRef.current = null;
    setActive(false);
    unlockResizeCursor();
    onDragEnd?.();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const backward = isVertical ? 'ArrowLeft' : 'ArrowUp';
    const forward = isVertical ? 'ArrowRight' : 'ArrowDown';
    if (![backward, forward, 'Home', 'End'].includes(event.key)) {
      return;
    }
    event.preventDefault();
    const step = event.shiftKey ? KEY_BIG_STEP : KEY_STEP;
    if (event.key === 'Home') {
      onChange(min);
      return;
    }
    if (event.key === 'End') {
      onChange(max);
      return;
    }
    const delta = event.key === forward ? step : -step;
    applyDelta(delta);
  };

  return (
    <div
      role='separator'
      tabIndex={0}
      aria-label={label}
      aria-orientation={orientation}
      aria-valuenow={value}
      aria-valuemin={min}
      aria-valuemax={max}
      data-orientation={orientation}
      data-active={active || undefined}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={finish}
      onPointerCancel={finish}
      onKeyDown={onKeyDown}
      className={cn(styles.handle, className)}
      style={style}
    >
      <span className={styles.grip} aria-hidden='true' />
    </div>
  );
}
