'use client';

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

import {useRef, type KeyboardEvent, type PointerEvent} from 'react';
import {cn} from '@/lib/utils';

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
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

/**
 * 工作台网格拖动手柄。Arc ResizablePanels 只有横向，且开合是内部状态，
 * 不能驱动顶栏开关，所以外壳用这根 6px 分隔条改像素尺寸。
 * Workbench grid drag handle. Arc ResizablePanels is horizontal-only and owns
 * collapse internally, so the shell uses this 6px separator for pixel sizes.
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
}: StudioSplitHandleProps) {
  const dragRef = useRef<{
    pointerId: number;
    start: number;
    origin: number;
  } | null>(null);
  const isVertical = orientation === 'vertical';

  const applyDelta = (delta: number) => {
    const signed = invert ? -delta : delta;
    onChange(clamp(Math.round(value + signed), min, max));
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType === 'mouse' && event.button !== 0) {
      return;
    }
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = {
      pointerId: event.pointerId,
      start: isVertical ? event.clientX : event.clientY,
      origin: value,
    };
    onDragStart?.();
  };

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    const current = isVertical ? event.clientX : event.clientY;
    const signed = invert ? drag.origin - (current - drag.start) : drag.origin + (current - drag.start);
    onChange(clamp(Math.round(signed), min, max));
  };

  const finish = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    dragRef.current = null;
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
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={finish}
      onPointerCancel={finish}
      onKeyDown={onKeyDown}
      className={cn(
        'group relative z-20 shrink-0 touch-none outline-none',
        isVertical ? 'h-full w-1.5 cursor-col-resize' : 'h-1.5 w-full cursor-row-resize',
        className,
      )}
    >
      <span
        aria-hidden='true'
        className={cn(
          'pointer-events-none absolute rounded-full bg-border transition-colors group-hover:bg-foreground/70 group-focus-visible:bg-foreground',
          isVertical
            ? 'top-1/2 left-1/2 h-8 w-0.5 -translate-x-1/2 -translate-y-1/2'
            : 'top-1/2 left-1/2 h-0.5 w-8 -translate-x-1/2 -translate-y-1/2',
        )}
      />
    </div>
  );
}
