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
 * 开屏动画时间轴与纯数学工具（无 DOM 依赖，便于单测）。
 * Splash timeline and pure math helpers (DOM-free, unit-testable).
 *
 * 分镜 / Storyboard:
 *   starIn  四芒星在屏幕正中（奇点中心）亮起 / 4-point star ignites at center
 *   glide   星光沿弧线平滑滑向左上角 / star glides along an arc to top-left
 *   charge  蓄力闪烁，漩涡加速 / charge flare, vortex spins up
 *   dive    流星加速下坠，从左上划向右下并穿过漩涡中心 / meteor dives TL→BR through the vortex
 *   impact  穿心瞬间漩涡碎裂、粒子炸开 / vortex shatters, particles burst
 *   gather  粒子从左到右聚合成 LOGO + STX / particles converge into LOGO + STX
 *   brand   清晰品牌层淡入 / crisp brand layer fades in
 *   welcome “欢迎回来”逐字渐入 / "Welcome back" fades in letter by letter
 *   exit    整层淡出露出首页 / overlay fades out, revealing the home page
 */

/** 流星下坠缓动指数：>1 表示越接近奇点越快 / Dive ease-in exponent (accelerates toward the singularity). */
export const DIVE_EASE_POWER = 1.7;

const DIVE_START = 2200;
const DIVE_DURATION = 1100;

/**
 * 流星到达路径中点（奇点中心）的时刻：路径参数 s = u^p = 0.5。
 * Moment the meteor reaches the path midpoint (the singularity): s = u^p = 0.5.
 */
const IMPACT_AT = Math.round(
  DIVE_START + DIVE_DURATION * Math.pow(0.5, 1 / DIVE_EASE_POWER),
);

/** 时间轴（毫秒）/ Timeline in milliseconds. */
export const SPLASH_TIMELINE = {
  starIn: [0, 900],
  glide: [900, 1850],
  charge: [1850, DIVE_START],
  dive: [DIVE_START, DIVE_START + DIVE_DURATION],
  impact: IMPACT_AT,
  shatterDuration: 1150,
  gather: [IMPACT_AT + 820, IMPACT_AT + 2050],
  /** 粒子按目标横坐标从左到右错峰启动的最大延迟 / Max left-to-right stagger. */
  gatherSweep: 380,
  brand: [IMPACT_AT + 2050, IMPACT_AT + 2750],
  welcome: IMPACT_AT + 2400,
  exit: IMPACT_AT + 4050,
  exitDuration: 780,
  skipExitDuration: 460,
} as const;

/**
 * 减弱动态效果模式下的简化时间轴：只做品牌与欢迎语渐显。
 * Reduced-motion timeline: brand and greeting fade only.
 */
export const REDUCED_MOTION_TIMELINE = {
  brand: [0, 500],
  welcome: 250,
  exit: 1700,
  exitDuration: 420,
} as const;

/** 将 t 截断到 [0, 1] / Clamp t to [0, 1]. */
export function clamp01(t: number): number {
  return t < 0 ? 0 : t > 1 ? 1 : t;
}

/** 区间进度 / Progress of t within [start, end]. */
export function progress(t: number, start: number, end: number): number {
  if (end <= start) {
    return t >= end ? 1 : 0;
  }
  return clamp01((t - start) / (end - start));
}

/** 线性插值 / Linear interpolation. */
export function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

export function easeInOutCubic(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

export function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3);
}

export function easeInOutQuart(t: number): number {
  return t < 0.5 ? 8 * t * t * t * t : 1 - Math.pow(-2 * t + 2, 4) / 2;
}

export function easeOutBack(t: number): number {
  const c1 = 1.70158;
  const c3 = c1 + 1;
  return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
}

/** 二维点 / 2D point. */
export interface Point {
  x: number;
  y: number;
}

/** 二次贝塞尔 / Quadratic Bezier. */
export function quadBezier(a: Point, c: Point, b: Point, s: number): Point {
  const u = 1 - s;
  return {
    x: u * u * a.x + 2 * u * s * c.x + s * s * b.x,
    y: u * u * a.y + 2 * u * s * c.y + s * s * b.y,
  };
}

/**
 * 构造流星下坠路径：起点 A（左上）、经过奇点中心 C、冲出屏幕到右下 B。
 * 控制点取 2C - (A + B) / 2，保证 s = 0.5 时恰好穿过 C。
 * Build the dive path: start A (top-left), through singularity C, off-screen
 * at bottom-right B. Control point 2C - (A + B) / 2 makes s = 0.5 hit C exactly.
 */
export function buildDivePath(
  start: Point,
  center: Point,
  bend: number,
): {a: Point; c: Point; b: Point} {
  const dx = center.x - start.x;
  const dy = center.y - start.y;
  const len = Math.hypot(dx, dy) || 1;
  // 法向偏移让轨迹略带弧度，更像被引力弯折 / Normal offset bends the exit like gravity would.
  const nx = -dy / len;
  const ny = dx / len;
  const b = {
    x: center.x + dx * 1.3 + nx * bend,
    y: center.y + dy * 1.3 + ny * bend,
  };
  const c = {
    x: 2 * center.x - (start.x + b.x) / 2,
    y: 2 * center.y - (start.y + b.y) / 2,
  };
  return {a: start, c, b};
}

/** 下坠阶段时间 → 路径参数（重力式加速）/ Dive time → path parameter (gravity ease-in). */
export function diveParam(t: number): number {
  const [start, end] = SPLASH_TIMELINE.dive;
  return Math.pow(progress(t, start, end), DIVE_EASE_POWER);
}

/**
 * 确定性伪随机（mulberry32），保证每次播放构图一致、可测。
 * Deterministic PRNG (mulberry32) so every run has the same composition.
 */
export function createRandom(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
