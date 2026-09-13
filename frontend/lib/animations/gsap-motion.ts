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

import gsap from 'gsap';

// 检查用户是否开启了系统级减弱动效偏好
// Check whether user has enabled prefers-reduced-motion in system settings
export function shouldReduceMotion(): boolean {
  if (typeof window === 'undefined') {
    return false;
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// 状态胶囊横向交错淡入动效
// Stagger entrance animation for status pill items
export function animatePillsEntrance(selector = '.stat-pill-item'): gsap.core.Tween | null {
  if (shouldReduceMotion()) {
    return null;
  }
  return gsap.from(selector, {
    opacity: 0,
    y: -4,
    scale: 0.96,
    duration: 0.28,
    stagger: 0.04,
    ease: 'power2.out',
    clearProps: 'opacity,transform',
  });
}

// 持续脉冲呼吸雷达光环动效（常用于触发中告警或致命错误）
// Continuous pulsing radar glow animation (used for active firing alerts or critical errors)
export function animatePulsingRadar(selector = '.radar-pulse-ring'): gsap.core.Tween | null {
  if (shouldReduceMotion()) {
    return null;
  }
  return gsap.to(selector, {
    scale: 1.45,
    opacity: 0,
    duration: 1.6,
    repeat: -1,
    ease: 'power1.out',
  });
}

// 表格行轻量级交错进入动效
// Subtle staggered entrance animation for high-density table rows
export function animateTableRows(selector = '.data-row-animate'): gsap.core.Tween | null {
  if (shouldReduceMotion()) {
    return null;
  }
  return gsap.from(selector, {
    opacity: 0,
    y: 3,
    duration: 0.24,
    stagger: 0.025,
    ease: 'power2.out',
    clearProps: 'opacity,transform',
  });
}

// 侧边抽屉内各区块平滑滑动入场
// Slide-in entrance animation for sections within the slide-over detail sheet
export function animateSheetSections(selector = '.sheet-section-animate'): gsap.core.Tween | null {
  if (shouldReduceMotion()) {
    return null;
  }
  return gsap.from(selector, {
    opacity: 0,
    x: 12,
    duration: 0.32,
    stagger: 0.06,
    ease: 'power2.out',
    clearProps: 'opacity,transform',
  });
}
