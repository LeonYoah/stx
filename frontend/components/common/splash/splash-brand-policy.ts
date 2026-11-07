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

/**
 * 减弱动态下品牌就绪与退场闸门（纯逻辑，便于非视觉回归）。
 * Reduced-motion brand readiness and exit gating (pure logic for non-visual tests).
 *
 * 背景：字体预热最长约 2200ms，而减弱动态时间轴约 1700+420ms；若等字体再 layoutBrand，
 * 导演可能已 dispose，品牌永不出现。策略是先 fallback 品牌，再升级字体/徽标。
 * Fonts may take ~2200ms while reduced-motion exits around 1700+420ms; waiting on
 * fonts before layoutBrand can dispose the director with no brand. Prefer an
 * immediate fallback brand, then upgrade once assets settle.
 */

import {REDUCED_MOTION_TIMELINE} from './splash-timeline';

/** 字体预热上限（与 splash-assets.loadFont 调用一致）/ Font warm-up budget. */
export const SPLASH_FONT_TIMEOUT_MS = 2200;

/** 徽标加载上限，避免 decode 挂起拖死品牌 / Mark decode budget so a hang cannot block brand. */
export const SPLASH_MARK_TIMEOUT_MS = 1800;

/**
 * 减弱动态最长等待：覆盖字体超时后再留短缓冲，仍不跳过开屏。
 * Max reduced-motion hold: font timeout plus a short buffer; splash is not skipped.
 */
export const REDUCED_MOTION_MAX_HOLD_MS =
  SPLASH_FONT_TIMEOUT_MS + REDUCED_MOTION_TIMELINE.exitDuration;

export interface ReducedMotionExitGateInput {
  /** 导演已流逝时间（ms）/ Director elapsed time in ms. */
  t: number;
  /** 计划退场时刻 / Scheduled exit time. */
  exitAt: number;
  /** 是否已有可绘制品牌层 / Whether a drawable brand layer exists. */
  brandReady: boolean;
  /** 资源准备（成功或失败/超时）已结束 / Asset prep finished (ok, fail, or timed out). */
  assetsSettled: boolean;
  /** 最长等待上限 / Hard hold ceiling. */
  maxHoldMs?: number;
}

/**
 * 减弱动态是否允许开始退场：到期后须已有品牌，或资源已结案，或触及最长等待。
 * Whether reduced-motion may begin exit: after exitAt, require brand, settled assets, or max hold.
 */
export function canReducedMotionBeginExit(
  input: ReducedMotionExitGateInput,
): boolean {
  if (input.t < input.exitAt) {
    return false;
  }
  if (input.brandReady || input.assetsSettled) {
    return true;
  }
  const maxHold = input.maxHoldMs ?? REDUCED_MOTION_MAX_HOLD_MS;
  return input.t >= maxHold;
}
