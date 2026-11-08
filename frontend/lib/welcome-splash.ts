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
 * 登录后开屏动画的跨页契约。
 * Cross-page contract for the post-login welcome splash.
 *
 * 两种登录方式都会整页跳转（window.location），React 状态无法跨页保留，
 * 所以登录页只在 sessionStorage 里留一个带时间戳的标记；目标页 <head> 中的
 * 内联脚本在首帧绘制前读取标记，给 <html> 打上属性，globals.css 据此先铺一层
 * 纯黑遮罩，保证用户绝不会先看到首页再看到动画。
 * Both login flows do a full-page navigation, so React state cannot survive.
 * The login page leaves a timestamped flag in sessionStorage; an inline script
 * in the target page's <head> reads it before first paint and marks <html>, and
 * globals.css paints a solid black cover so the home page never flashes first.
 */

/** sessionStorage 标记键 / sessionStorage flag key. */
export const WELCOME_SPLASH_STORAGE_KEY = 'stx:welcome-splash';

/**
 * 强制播放开关（localStorage）：自动化浏览器默认跳过动画，E2E 需要验证动画时置为 "1"。
 * Force switch (localStorage): automated browsers skip the splash by default;
 * set to "1" when an E2E test needs to exercise it.
 */
export const WELCOME_SPLASH_FORCE_KEY = 'stx:welcome-splash-force';

/** <html> 上的状态属性 / State attribute on <html>. */
export const WELCOME_SPLASH_ATTR = 'data-stx-splash';

/**
 * 标记有效期：登录后超过该时长才落地的页面不再播放（例如网络卡住、用户离开）。
 * Flag lifetime: pages landing later than this no longer play the splash.
 */
export const WELCOME_SPLASH_MAX_AGE_MS = 60_000;

/**
 * 黑色遮罩兜底超时：即便脚本异常，遮罩也会自行撤掉，页面不会永久黑屏。
 * Cover safety timeout: the black cover removes itself even if the splash
 * script fails, so the page can never stay black forever.
 */
export const WELCOME_SPLASH_COVER_TIMEOUT_MS = 15_000;

/** 登录页淡入黑幕时长 / Login page fade-to-black duration. */
export const WELCOME_SPLASH_CURTAIN_MS = 380;

/**
 * 首帧前执行的内联脚本：校验标记 → 消费标记 → 给 <html> 打属性 → 设置兜底超时。
 * 自动化浏览器（navigator.webdriver）默认不播放，避免遮挡既有 E2E 的后续点击。
 * Inline pre-paint script: validate flag → consume it → mark <html> → arm the
 * safety timeout. Automated browsers skip by default so existing E2E flows are
 * not blocked by the overlay.
 */
export const welcomeSplashBootScript = `(function(){try{
var k=${JSON.stringify(WELCOME_SPLASH_STORAGE_KEY)};
var raw=sessionStorage.getItem(k);
if(!raw)return;
sessionStorage.removeItem(k);
var age=Date.now()-Number(raw);
var forced=false;
try{forced=localStorage.getItem(${JSON.stringify(WELCOME_SPLASH_FORCE_KEY)})==='1';}catch(e){}
if(!(age>=0&&age<${WELCOME_SPLASH_MAX_AGE_MS}))return;
if(location.pathname.indexOf('/login')===0)return;
if(navigator.webdriver&&!forced)return;
var h=document.documentElement;
h.setAttribute(${JSON.stringify(WELCOME_SPLASH_ATTR)},'pending');
setTimeout(function(){h.removeAttribute(${JSON.stringify(WELCOME_SPLASH_ATTR)});},${WELCOME_SPLASH_COVER_TIMEOUT_MS});
}catch(e){}})();`;

/**
 * 写入开屏标记（登录成功后、跳转前调用）。
 * Arm the splash flag (call after login succeeds, before navigating).
 */
export function armWelcomeSplash(now: number = Date.now()): void {
  try {
    sessionStorage.setItem(WELCOME_SPLASH_STORAGE_KEY, String(now));
  } catch {
    // 隐私模式等禁用存储时静默跳过动画 / Storage disabled: silently skip.
  }
}

/**
 * 当前页面是否应播放开屏（由内联脚本决定）。
 * Whether the current page should play the splash (decided by the boot script).
 */
export function isWelcomeSplashPending(): boolean {
  if (typeof document === 'undefined') {
    return false;
  }
  return document.documentElement.hasAttribute(WELCOME_SPLASH_ATTR);
}

/**
 * 撤掉首帧黑色遮罩（动画层自身不透明期间或结束时调用）。
 * Remove the pre-paint black cover (once the opaque overlay owns the screen).
 */
export function releaseWelcomeSplashCover(): void {
  if (typeof document === 'undefined') {
    return;
  }
  document.documentElement.removeAttribute(WELCOME_SPLASH_ATTR);
}

/**
 * 登录成功后的过场：写标记，并把登录页平滑淡入纯黑，再交给调用方整页跳转。
 * 黑幕与目标页首帧遮罩同色，跨页切换没有闪白、没有首页露出。
 * Post-login hand-off: arm the flag and fade the login page to solid black
 * before the caller navigates. The curtain matches the target page's pre-paint
 * cover, so the page swap shows neither a white flash nor the home page.
 */
export function beginWelcomeSplashTransition(): Promise<void> {
  armWelcomeSplash();
  if (typeof document === 'undefined') {
    return Promise.resolve();
  }

  const curtain = document.createElement('div');
  curtain.setAttribute('aria-hidden', 'true');
  Object.assign(curtain.style, {
    position: 'fixed',
    top: '0',
    right: '0',
    bottom: '0',
    left: '0',
    zIndex: '2147483647',
    background: '#000',
    opacity: '0',
    pointerEvents: 'auto',
    transition: `opacity ${WELCOME_SPLASH_CURTAIN_MS}ms cubic-bezier(.4,0,.2,1)`,
  });
  document.body.appendChild(curtain);

  return new Promise((resolve) => {
    // 强制回流后再改透明度，确保过渡生效 / Force reflow so the transition runs.
    void curtain.offsetWidth;
    curtain.style.opacity = '1';
    window.setTimeout(resolve, WELCOME_SPLASH_CURTAIN_MS + 20);
  });
}
