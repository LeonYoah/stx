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
 * 登录成功后开屏动画的会话级门控（sessionStorage）。
 * Session-scoped gate for the post-login splash animation (sessionStorage).
 *
 * 设计要点 / Design notes:
 * - 仅在「本次登录成功即将跳转」时标记 pending；不改写用户原 redirect 目标。
 *   Arm pending only right before a successful-login redirect; never rewrite the redirect target.
 * - 进入工作台布局时一次性消费 pending；刷新 / 客户端切页不会重播。
 *   Consume pending once when entering the main shell; refresh / client nav will not replay.
 * - 登出时清理，保证「退出再登录」可再次播放。
 *   Clear on logout so a later login can show the splash again.
 */

/** sessionStorage 键：登录成功后待播放的开屏标记 / Pending splash flag after successful login */
export const POST_LOGIN_SPLASH_PENDING_KEY = 'stx_post_login_splash_pending';

function canUseSessionStorage(): boolean {
  return typeof window !== 'undefined' && typeof window.sessionStorage !== 'undefined';
}

/**
 * 标记「下一次进入主壳时播放开屏」。
 * Mark that the next entry into the main shell should play the splash.
 */
export function markPostLoginSplashPending(): void {
  if (!canUseSessionStorage()) {
    return;
  }
  try {
    window.sessionStorage.setItem(POST_LOGIN_SPLASH_PENDING_KEY, '1');
  } catch {
    // 隐私模式等写入失败时静默跳过，不影响登录跳转。
    // Ignore write failures (e.g. privacy mode); login redirect must still proceed.
  }
}

/**
 * 读取并清除 pending；返回是否应展示开屏。
 * Read and clear the pending flag; returns whether the splash should show.
 */
export function consumePostLoginSplashPending(): boolean {
  if (!canUseSessionStorage()) {
    return false;
  }
  try {
    const pending = window.sessionStorage.getItem(POST_LOGIN_SPLASH_PENDING_KEY) === '1';
    if (pending) {
      window.sessionStorage.removeItem(POST_LOGIN_SPLASH_PENDING_KEY);
    }
    return pending;
  } catch {
    return false;
  }
}

/**
 * 清除开屏会话标记（登出或异常路径）。
 * Clear splash session flags (logout or exceptional paths).
 */
export function clearPostLoginSplashSession(): void {
  if (!canUseSessionStorage()) {
    return;
  }
  try {
    window.sessionStorage.removeItem(POST_LOGIN_SPLASH_PENDING_KEY);
  } catch {
    // ignore
  }
}

/**
 * 仅探测是否仍有 pending（不消费）。测试与调试用。
 * Peek whether pending is set without consuming. For tests / diagnostics.
 */
export function peekPostLoginSplashPending(): boolean {
  if (!canUseSessionStorage()) {
    return false;
  }
  try {
    return window.sessionStorage.getItem(POST_LOGIN_SPLASH_PENDING_KEY) === '1';
  } catch {
    return false;
  }
}
