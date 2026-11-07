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

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {
  WELCOME_SPLASH_ATTR,
  WELCOME_SPLASH_COVER_TIMEOUT_MS,
  WELCOME_SPLASH_CURTAIN_MS,
  WELCOME_SPLASH_FORCE_KEY,
  WELCOME_SPLASH_MAX_AGE_MS,
  WELCOME_SPLASH_STORAGE_KEY,
  armWelcomeSplash,
  beginWelcomeSplashTransition,
  isWelcomeSplashPending,
  releaseWelcomeSplashCover,
  welcomeSplashBootScript,
} from '@/lib/welcome-splash';

/** 执行首帧内联脚本 / Run the pre-paint boot script. */
function runBootScript() {
  new Function(welcomeSplashBootScript)();
}

function setWebdriver(value: boolean) {
  Object.defineProperty(window.navigator, 'webdriver', {
    configurable: true,
    get: () => value,
  });
}

// 登录开屏跨页契约单测
// Unit tests for the post-login splash cross-page contract
describe('welcome-splash', () => {
  beforeEach(() => {
    sessionStorage.clear();
    localStorage.clear();
    releaseWelcomeSplashCover();
    setWebdriver(false);
    window.history.replaceState({}, '', '/dashboard');
  });

  afterEach(() => {
    vi.useRealTimers();
    document.body.innerHTML = '';
  });

  it('marks <html> and consumes a fresh flag', () => {
    armWelcomeSplash();
    runBootScript();

    expect(isWelcomeSplashPending()).toBe(true);
    expect(document.documentElement.getAttribute(WELCOME_SPLASH_ATTR)).toBe(
      'pending',
    );
    // 消费后刷新页面不会重播 / Consumed, so a reload does not replay.
    expect(sessionStorage.getItem(WELCOME_SPLASH_STORAGE_KEY)).toBeNull();
  });

  it('ignores a stale flag', () => {
    armWelcomeSplash(Date.now() - WELCOME_SPLASH_MAX_AGE_MS - 1);
    runBootScript();

    expect(isWelcomeSplashPending()).toBe(false);
    expect(sessionStorage.getItem(WELCOME_SPLASH_STORAGE_KEY)).toBeNull();
  });

  it('never covers the login page', () => {
    window.history.replaceState({}, '', '/login');
    armWelcomeSplash();
    runBootScript();

    expect(isWelcomeSplashPending()).toBe(false);
  });

  it('skips automated browsers unless forced', () => {
    setWebdriver(true);
    armWelcomeSplash();
    runBootScript();
    expect(isWelcomeSplashPending()).toBe(false);

    localStorage.setItem(WELCOME_SPLASH_FORCE_KEY, '1');
    armWelcomeSplash();
    runBootScript();
    expect(isWelcomeSplashPending()).toBe(true);
  });

  it('removes the cover by itself after the safety timeout', () => {
    vi.useFakeTimers();
    armWelcomeSplash();
    runBootScript();
    expect(isWelcomeSplashPending()).toBe(true);

    vi.advanceTimersByTime(WELCOME_SPLASH_COVER_TIMEOUT_MS);
    expect(isWelcomeSplashPending()).toBe(false);
  });

  it('arms the flag and fades a black curtain before navigation', async () => {
    vi.useFakeTimers();
    const done = vi.fn();
    void beginWelcomeSplashTransition().then(done);

    expect(sessionStorage.getItem(WELCOME_SPLASH_STORAGE_KEY)).not.toBeNull();
    const curtain = document.body.lastElementChild as HTMLElement;
    expect(curtain.style.background).toContain('rgb(0, 0, 0)');
    expect(curtain.style.position).toBe('fixed');
    expect(curtain.style.opacity).toBe('1');

    await vi.advanceTimersByTimeAsync(WELCOME_SPLASH_CURTAIN_MS + 20);
    expect(done).toHaveBeenCalledTimes(1);
  });
});
