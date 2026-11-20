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

import {type KeyboardEvent, useCallback, useRef} from 'react';

/**
 * roving tabindex 键盘导航的目标下标：←/↑ 前移、→/↓ 后移（循环），
 * Home/End 跳首尾；非导航键返回 null。
 * Roving-tabindex navigation target: ←/↑ step back, →/↓ step forward (wrapping),
 * Home/End jump to the edges; non-navigation keys return null.
 */
export function rovingTarget(
  key: string,
  index: number,
  count: number,
): number | null {
  switch (key) {
    case 'ArrowRight':
    case 'ArrowDown':
      return (index + 1) % count;
    case 'ArrowLeft':
    case 'ArrowUp':
      return (index - 1 + count) % count;
    case 'Home':
      return 0;
    case 'End':
      return count - 1;
    default:
      return null;
  }
}

/**
 * roving tabindex 组（tablist / radiogroup）的键盘接线：命中导航键时阻止默认
 * 行为、回调目标下标并把焦点移到那一格（选择跟随焦点由调用方在回调里决定，
 * 本组组件都是移动即选中）。
 * Keyboard wiring for a roving-tabindex group (tablist / radiogroup): on a
 * navigation key it prevents default, reports the target index and moves focus
 * to that cell (whether selection follows focus is the caller's choice — the
 * callers here all select on move).
 */
export function useRovingKeys<T extends HTMLElement>(
  count: number,
  onMove: (index: number) => void,
) {
  const refs = useRef<(T | null)[]>([]);
  /** 绑到第 index 个格子的 ref 回调。/ Ref callback bound to cell `index`. */
  const refAt = useCallback(
    (index: number) => (node: T | null) => {
      refs.current[index] = node;
    },
    [],
  );
  const onKeyDown = (event: KeyboardEvent, index: number) => {
    const target = rovingTarget(event.key, index, count);
    if (target === null) {
      return;
    }
    event.preventDefault();
    onMove(target);
    refs.current[target]?.focus();
  };
  return {refAt, onKeyDown};
}
