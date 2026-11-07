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
  REDUCED_MOTION_MAX_HOLD_MS,
  SPLASH_FONT_TIMEOUT_MS,
  canReducedMotionBeginExit,
} from '../splash-brand-policy';
import {REDUCED_MOTION_TIMELINE} from '../splash-timeline';

// 减弱动态品牌闸门：慢字体/失败时仍须先有 fallback 再退场，不能空屏 dispose。
// Reduced-motion brand gate: slow/failed fonts still need a fallback before exit.
describe('canReducedMotionBeginExit', () => {
  const exitAt = REDUCED_MOTION_TIMELINE.exit;

  it('does not exit before the scheduled exit time', () => {
    expect(
      canReducedMotionBeginExit({
        t: exitAt - 1,
        exitAt,
        brandReady: true,
        assetsSettled: true,
      }),
    ).toBe(false);
  });

  it('exits on schedule when a fallback brand is already ready', () => {
    expect(
      canReducedMotionBeginExit({
        t: exitAt,
        exitAt,
        brandReady: true,
        assetsSettled: false,
      }),
    ).toBe(true);
  });

  it('holds exit while fonts are still pending and brand is missing', () => {
    // 经典失败窗口：1700–2120ms 之间字体仍在 2200ms 预热，且从未 layoutBrand。
    // Classic failure window: between 1700–2120ms fonts are still warming and brand was never laid out.
    const midExit = exitAt + REDUCED_MOTION_TIMELINE.exitDuration / 2;
    expect(midExit).toBeLessThan(SPLASH_FONT_TIMEOUT_MS);
    expect(
      canReducedMotionBeginExit({
        t: midExit,
        exitAt,
        brandReady: false,
        assetsSettled: false,
      }),
    ).toBe(false);
  });

  it('allows exit after assets settle even if brand build failed', () => {
    expect(
      canReducedMotionBeginExit({
        t: exitAt + 50,
        exitAt,
        brandReady: false,
        assetsSettled: true,
      }),
    ).toBe(true);
  });

  it('allows exit at the hard hold ceiling so splash cannot hang forever', () => {
    expect(
      canReducedMotionBeginExit({
        t: REDUCED_MOTION_MAX_HOLD_MS,
        exitAt,
        brandReady: false,
        assetsSettled: false,
      }),
    ).toBe(true);
    expect(
      canReducedMotionBeginExit({
        t: REDUCED_MOTION_MAX_HOLD_MS - 1,
        exitAt,
        brandReady: false,
        assetsSettled: false,
      }),
    ).toBe(false);
  });

  it('keeps max hold at or beyond the font timeout budget', () => {
    expect(REDUCED_MOTION_MAX_HOLD_MS).toBeGreaterThanOrEqual(
      SPLASH_FONT_TIMEOUT_MS,
    );
    expect(REDUCED_MOTION_MAX_HOLD_MS).toBeGreaterThan(
      exitAt + REDUCED_MOTION_TIMELINE.exitDuration,
    );
  });
});
