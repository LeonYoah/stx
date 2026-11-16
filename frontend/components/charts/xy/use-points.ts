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

import {animate, type MotionValue, motionValue} from 'motion/react';
import {useLayoutEffect, useRef, useState} from 'react';
import {SPRING_LAYOUT} from '@/lib/ease';

export type XYAnimatedPoint = {x: MotionValue<number>; y: MotionValue<number>};
export type XYPointTarget = {key: string; x: number; y: number};

/**
 * One spring per coordinate, keyed by identity. Paths, dots and fills read the
 * same values, so a data change morphs every mark together. A resize jumps:
 * the data didn't change, so nothing should appear to move.
 */
export function useAnimatedPoints(
  targets: XYPointTarget[],
  reduce: boolean,
  width: number,
) {
  const shape = targets.map((target) => target.key).join('\n');
  const create = (previous?: Map<string, XYAnimatedPoint>) =>
    new Map(
      targets.map((target) => [
        target.key,
        previous?.get(target.key) ?? {
          x: motionValue(target.x),
          y: motionValue(target.y),
        },
      ]),
    );
  const [stored, setStored] = useState(() => ({shape, points: create()}));
  let points = stored.points;
  if (stored.shape !== shape) {
    points = create(stored.points);
    setStored({shape, points});
  }
  const lastWidth = useRef(width);
  // Hover and focus renders produce the same snapshot, so they never restart a spring.
  const snapshot = JSON.stringify(targets);
  useLayoutEffect(() => {
    const resized = lastWidth.current !== width;
    lastWidth.current = width;
    const next: XYPointTarget[] = JSON.parse(snapshot);
    const controls: ReturnType<typeof animate>[] = [];
    for (const target of next) {
      const point = points.get(target.key);
      if (!point) {
        continue;
      }
      for (const axis of ['x', 'y'] as const) {
        if (reduce || resized) {
          point[axis].jump(target[axis]);
        } else {
          controls.push(animate(point[axis], target[axis], SPRING_LAYOUT));
        }
      }
    }
    return () => {
      for (const control of controls) {
        control.stop();
      }
    };
  }, [snapshot, reduce, points, width]);
  return points;
}
