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

import {describe, expect, it} from 'vitest';
import {
  SPLASH_TIMELINE,
  buildDivePath,
  createRandom,
  diveParam,
  progress,
  quadBezier,
} from '../splash-timeline';

// 开屏时间轴与路径数学单测
// Unit tests for the splash timeline and path math
describe('splash-timeline', () => {
  it('keeps the storyboard in order', () => {
    const T = SPLASH_TIMELINE;
    expect(T.starIn[1]).toBeLessThanOrEqual(T.glide[0]);
    expect(T.glide[1]).toBeLessThanOrEqual(T.charge[0]);
    expect(T.charge[1]).toBe(T.dive[0]);
    expect(T.impact).toBeGreaterThan(T.dive[0]);
    expect(T.impact).toBeLessThan(T.dive[1]);
    expect(T.gather[0]).toBeGreaterThan(T.impact);
    expect(T.brand[0]).toBeGreaterThanOrEqual(T.gather[0]);
    expect(T.welcome).toBeGreaterThan(T.brand[0]);
    expect(T.exit).toBeGreaterThan(T.welcome);
  });

  it('hits the path midpoint exactly at impact', () => {
    expect(diveParam(SPLASH_TIMELINE.impact)).toBeCloseTo(0.5, 2);
    expect(diveParam(SPLASH_TIMELINE.dive[0])).toBe(0);
    expect(diveParam(SPLASH_TIMELINE.dive[1])).toBe(1);
  });

  it('routes the dive through the singularity center', () => {
    const start = {x: 180, y: 140};
    const center = {x: 720, y: 450};
    const {a, c, b} = buildDivePath(start, center, 50);
    const mid = quadBezier(a, c, b, 0.5);

    expect(mid.x).toBeCloseTo(center.x, 6);
    expect(mid.y).toBeCloseTo(center.y, 6);
    expect(quadBezier(a, c, b, 0)).toEqual(start);
    // 终点越过中心冲向右下 / Exit continues past the center toward bottom-right.
    expect(b.x).toBeGreaterThan(center.x);
    expect(b.y).toBeGreaterThan(center.y);
  });

  it('clamps progress and handles empty ranges', () => {
    expect(progress(-5, 0, 10)).toBe(0);
    expect(progress(5, 0, 10)).toBe(0.5);
    expect(progress(50, 0, 10)).toBe(1);
    expect(progress(10, 10, 10)).toBe(1);
  });

  it('produces a deterministic random sequence', () => {
    const a = createRandom(42);
    const b = createRandom(42);
    const seq = Array.from({length: 5}, () => a());
    expect(Array.from({length: 5}, () => b())).toEqual(seq);
    seq.forEach((v) => {
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    });
  });
});
