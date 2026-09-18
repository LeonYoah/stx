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
 * STX Kapa.ai Ask AI Integration
 * STX 自有 Kapa.ai 智能问答服务集成
 *
 * Uses the STX project widget (knowledge covers STX + SeaTunnel source/docs).
 * 使用 STX 项目自有 Widget（知识库覆盖 STX 与 SeaTunnel 源码/文档）。
 */

// STX Kapa widget configuration / STX Kapa 小部件配置常量
export const KAPA_SCRIPT_ID = 'st-kapa-ai-widget-script';
export const KAPA_TRIGGER_ID = 'st-kapa-ask-ai-trigger';
export const KAPA_DEFAULT_WEBSITE_ID = 'd9390efd-fdc5-4449-8aa1-bb2fd5fe13f3';
export const KAPA_PROJECT_NAME = 'STX';
export const KAPA_PROJECT_COLOR = '#2563eb';
/**
 * Light lockup (dark wordmark) for light modal surfaces.
 * 浅色锁章（深色字标），用于亮色弹窗表面。
 */
export const KAPA_PROJECT_LOGO_PATH = '/brand/stx-logo.png';
/**
 * Dark lockup (white wordmark) for dark modal surfaces.
 * 深色锁章（白字标），用于暗色弹窗表面。
 */
export const KAPA_PROJECT_LOGO_DARK_PATH = '/brand/stx-logo-dark.png';
/**
 * Bird mark for the floating launcher (brand-blue button; no wordmark contrast issue).
 * 悬浮按钮使用青鸾图形标（按钮底色为品牌蓝，避免字标对比度问题）。
 */
export const KAPA_PROJECT_MARK_PATH = '/brand/stx-mark.png';
/**
 * Sync widget theme with next-themes (`class="dark"` on <html>).
 * 与 next-themes 同步（<html> 上的 class="dark"）。
 */
export const KAPA_COLOR_SCHEME_SELECTOR = '.dark';
/**
 * Lift the floating button above the bottom Dock (esp. mobile right-aligned dock).
 * 上移悬浮按钮，避免与底部 Dock（尤其移动端右下角）重叠。
 */
export const KAPA_BUTTON_POSITION_BOTTOM = '5.5rem';
export const KAPA_BUTTON_POSITION_RIGHT = '1.25rem';

export interface KapaOpenOptions {
  query?: string;
  mode?: string;
  [key: string]: unknown;
}

export type KapaInstance = {
  open?: (options?: KapaOpenOptions) => void;
  close?: () => void;
  openModal?: (options?: KapaOpenOptions) => void;
  closeModal?: () => void;
  q?: unknown[][];
  [key: string]: unknown;
};

export type KapaCallable = {
  (action: string, options?: KapaOpenOptions): void;
  open?: (options?: KapaOpenOptions) => void;
  close?: () => void;
  openModal?: (options?: KapaOpenOptions) => void;
  closeModal?: () => void;
  q?: unknown[][];
  [key: string]: unknown;
};

declare global {
  interface Window {
    Kapa?: KapaCallable | KapaInstance;
  }
}

let kapaLoadingPromise: Promise<boolean> | null = null;
let kapaThemeObserver: MutationObserver | null = null;

/**
 * Get effective Kapa Website ID, allowing environment variable override
 * 获取当前生效的 Kapa Website ID，支持环境变量覆盖
 */
export function getKapaWebsiteId(): string {
  return process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID || KAPA_DEFAULT_WEBSITE_ID;
}

/**
 * Whether the console is currently in dark mode (next-themes class strategy).
 * 控制台当前是否为深色模式（next-themes class 策略）
 */
export function isDocumentDarkMode(): boolean {
  if (typeof document === 'undefined') {
    return false;
  }
  return document.documentElement.classList.contains('dark');
}

/**
 * Resolve a public brand asset to an absolute URL
 * 将 public 品牌资源解析为绝对 URL
 */
function resolveBrandAssetUrl(path: string): string {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}${path}`;
  }
  return path;
}

/**
 * Resolve modal lockup logo for the current (or explicit) color scheme
 * 按当前（或显式指定）主题解析弹窗锁章
 *
 * @param variant - light | dark lockup; omit to follow document theme / 省略则跟随文档主题
 */
export function getKapaProjectLogo(variant?: 'light' | 'dark'): string {
  const resolved =
    variant ?? (isDocumentDarkMode() ? 'dark' : 'light');
  const path =
    resolved === 'dark' ? KAPA_PROJECT_LOGO_DARK_PATH : KAPA_PROJECT_LOGO_PATH;
  return resolveBrandAssetUrl(path);
}

/**
 * Resolve launcher bird-mark URL (theme-agnostic on brand-blue button)
 * 解析悬浮按钮青鸾图形标 URL（品牌蓝底上不受主题影响）
 */
export function getKapaLauncherImage(): string {
  return resolveBrandAssetUrl(KAPA_PROJECT_MARK_PATH);
}

/**
 * Apply theme-matched logos to the Kapa script tag and any already-rendered <img>s.
 * Kapa does not reliably honor *-logo-*-dark attrs for images, so we swap at runtime.
 * 将匹配主题的 logo 写回脚本属性，并替换已渲染的 <img>。
 * Kapa 对图片类 *-dark 属性支持不可靠，因此在运行时主动切换。
 */
export function applyKapaThemeLogos(): void {
  if (typeof document === 'undefined') {
    return;
  }

  const modalLogo = getKapaProjectLogo();
  const launcherImage = getKapaLauncherImage();
  const script = document.getElementById(KAPA_SCRIPT_ID);
  if (script) {
    script.setAttribute('data-project-logo', modalLogo);
    script.setAttribute('data-modal-logo-src', modalLogo);
    script.setAttribute('data-launcher-button-image', launcherImage);
  }

  const brandHint = '/brand/stx-';
  document.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || '';
    if (!src.includes(brandHint) && !src.includes('/brand/stx')) {
      return;
    }
    // Launcher uses mark; modal header uses light/dark lockup
    // 悬浮按钮用图形标；弹窗标题用浅/深色锁章
    if (src.includes('stx-mark')) {
      if (img.src !== launcherImage) {
        img.src = launcherImage;
      }
      return;
    }
    if (
      src.includes('stx-logo') &&
      img.src !== modalLogo
    ) {
      img.src = modalLogo;
    }
  });
}

/**
 * Watch <html class> changes and keep Kapa logos / scheme in sync with the console theme.
 * 监听 <html class> 变化，使 Kapa logo 与配色跟随控制台主题。
 */
export function startKapaThemeSync(): void {
  if (typeof window === 'undefined' || kapaThemeObserver) {
    return;
  }

  applyKapaThemeLogos();
  kapaThemeObserver = new MutationObserver(() => {
    applyKapaThemeLogos();
  });
  kapaThemeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  });
}

/**
 * Check if a functional Kapa instance (not unconsumed internal queue) is ready
 * 检查当前是否存在已就绪的 Kapa 实例（排查未消费的内部排队占位符）
 */
export function isKapaReady(kapa: unknown): boolean {
  if (!kapa) {
    return false;
  }
  const k = kapa as Record<string, unknown> | ((...args: unknown[]) => unknown);
  if ((k as Record<string, unknown>).__stx_queue) {
    return false;
  }
  return (
    typeof (k as Record<string, unknown>).open === 'function' ||
    typeof k === 'function' ||
    typeof (k as Record<string, unknown>).openModal === 'function'
  );
}

/**
 * Pre-initialize window.Kapa queue so calls before script loads are preserved
 * 预初始化 window.Kapa 指令队列，确保脚本加载完成前调用的指令不丢失
 */
export function initKapaPreinitialization(): void {
  if (typeof window === 'undefined') {
    return;
  }

  if (!window.Kapa) {
    const queue: unknown[][] = [];
    const kapaQueue: any = function (...args: unknown[]) {
      queue.push(args);
    };
    kapaQueue.q = queue;
    kapaQueue.__stx_queue = true;
    kapaQueue.open = function (options?: KapaOpenOptions) {
      kapaQueue('open', options);
    };
    kapaQueue.close = function () {
      kapaQueue('close');
    };
    window.Kapa = kapaQueue;
  }
}

/**
 * Dispatch open command to Kapa instance or queued callable
 * 向 Kapa 实例或排队队列分发打开指令
 *
 * @param query - Optional initial search or question / 可选的预填提问文本
 * @returns boolean - Whether invocation was dispatched / 是否成功分发调用
 */
function dispatchKapaOpen(query?: string): boolean {
  if (typeof window === 'undefined') {
    return false;
  }

  const kapa = window.Kapa as any;
  if (!kapa) {
    return false;
  }

  const options: KapaOpenOptions | undefined = query ? {query} : undefined;

  // 1. Standard official method: window.Kapa.open({ query })
  // 1. 官方标准方法：window.Kapa.open({ query })
  if (typeof kapa.open === 'function') {
    kapa.open(options);
    return true;
  }

  // 2. Command queue format: window.Kapa('open', { query })
  // 2. 命令队列形式：window.Kapa('open', { query })
  if (typeof kapa === 'function') {
    kapa('open', options);
    return true;
  }

  // 3. Fallback compatibility for older builds
  // 3. 兼容历史或部分定制版本的 openModal 方法
  if (typeof kapa.openModal === 'function') {
    kapa.openModal(options);
    return true;
  }

  return false;
}

/**
 * Ensure Kapa widget script is mounted (floating button stays visible).
 * 确保 Kapa widget 脚本已挂载；默认展示右下角悬浮按钮（不设置 data-button-hide）。
 *
 * Note: Kapa enables http://localhost by default for local testing even when other domains are restricted.
 * 说明：即使在控制台限制了域名，Kapa 默认仍放行 http://localhost 供本地调试。
 *
 * @returns Promise<boolean> - Whether Kapa widget is loaded / 是否成功加载 Kapa 小部件
 */
export function ensureKapaWidget(): Promise<boolean> {
  if (typeof window === 'undefined') {
    return Promise.resolve(false);
  }

  initKapaPreinitialization();

  // Hidden trigger kept as a programmatic fallback for plugin "Ask AI" entry
  // 保留隐藏触发节点，供插件详情页「Ask AI」入口在 API 不可用时降级点击
  let triggerBtn = document.getElementById(KAPA_TRIGGER_ID);
  if (!triggerBtn) {
    triggerBtn = document.createElement('button');
    triggerBtn.id = KAPA_TRIGGER_ID;
    triggerBtn.style.display = 'none';
    triggerBtn.setAttribute('aria-hidden', 'true');
    document.body.appendChild(triggerBtn);
  }

  // Upgrade path: old script without theme selector / mark launcher → remount
  // 升级路径：旧脚本缺少主题选择器或图形标启动器时重建（须在 isKapaReady 短路之前）
  const existingScript = document.getElementById(KAPA_SCRIPT_ID);
  if (
    existingScript &&
    (existingScript.getAttribute('data-color-scheme-selector') !==
      KAPA_COLOR_SCHEME_SELECTOR ||
      !existingScript.getAttribute('data-launcher-button-image')?.includes(
        'stx-mark',
      ))
  ) {
    existingScript.remove();
    kapaLoadingPromise = null;
    delete (window as {Kapa?: unknown}).Kapa;
    initKapaPreinitialization();
  }

  // If already loaded and functional (not an unconsumed queue placeholder), return immediately
  // 若已完全就绪（且非未消费的队列占位符），直接返回成功
  if (isKapaReady(window.Kapa)) {
    applyKapaThemeLogos();
    startKapaThemeSync();
    return Promise.resolve(true);
  }

  if (!document.getElementById(KAPA_SCRIPT_ID)) {
    kapaLoadingPromise = null;
  } else if (kapaLoadingPromise) {
    return kapaLoadingPromise;
  }

  kapaLoadingPromise = new Promise<boolean>((resolve) => {
    const script = document.createElement('script');
    script.id = KAPA_SCRIPT_ID;
    script.src = 'https://widget.kapa.ai/kapa-widget.bundle.js';
    script.setAttribute('data-website-id', getKapaWebsiteId());
    script.setAttribute('data-project-name', KAPA_PROJECT_NAME);
    script.setAttribute('data-project-color', KAPA_PROJECT_COLOR);
    // Modal lockup follows theme; launcher uses bird mark on brand-blue button
    // 弹窗锁章跟随主题；悬浮按钮在品牌蓝底上使用青鸾图形标
    const modalLogo = getKapaProjectLogo();
    const launcherImage = getKapaLauncherImage();
    script.setAttribute('data-project-logo', modalLogo);
    script.setAttribute('data-modal-logo-src', modalLogo);
    script.setAttribute('data-launcher-button-image', launcherImage);
    script.setAttribute('data-color-scheme-selector', KAPA_COLOR_SCHEME_SELECTOR);
    // Keep default floating button visible; override-open-id only adds extra open targets
    // 保持默认悬浮按钮可见；override-open-id 仅额外绑定打开目标，不会隐藏按钮
    script.setAttribute('data-modal-override-open-id', KAPA_TRIGGER_ID);
    script.setAttribute('data-button-position-bottom', KAPA_BUTTON_POSITION_BOTTOM);
    script.setAttribute('data-button-position-right', KAPA_BUTTON_POSITION_RIGHT);
    script.async = true;

    const timeoutId = setTimeout(() => {
      // Resolve false if timed out (e.g., offline or firewall restriction)
      // 若超时（如隔离网络或防火墙限制），返回 false 优雅降级
      resolve(false);
    }, 6000);

    script.onload = () => {
      clearTimeout(timeoutId);
      // Wait for Kapa global object initialization, then sync logos
      // 等待 Kapa 全局对象初始化后同步 logo
      setTimeout(() => {
        applyKapaThemeLogos();
        startKapaThemeSync();
        resolve(true);
      }, 250);
    };

    script.onerror = () => {
      clearTimeout(timeoutId);
      resolve(false);
    };

    document.body.appendChild(script);
  });

  return kapaLoadingPromise;
}

/**
 * Trigger opening the STX Ask AI modal with optional pre-filled query
 * 触发唤起 STX Ask AI 对话框，可携带预填提问内容
 *
 * @param initialQuery - Optional pre-filled question / 可选的预填提问文本
 * @returns Promise<boolean> - True if successfully opened / 是否成功唤起对话框
 */
export async function openSeaTunnelAskAi(initialQuery?: string): Promise<boolean> {
  const loaded = await ensureKapaWidget();

  const kapa = window.Kapa as any;

  // If script failed to load or is offline
  // 若脚本加载失败或处于离线/被拦截状态
  if (!loaded) {
    // If a mock or standalone instance was present (not our unexecuted queue), allow it
    // 若存在真实的全局实例（非待消费的排队占位符），仍尝试派发
    if (isKapaReady(kapa) && dispatchKapaOpen(initialQuery)) {
      return true;
    }
    // Fallback: try clicking hidden trigger button in case DOM listeners are registered
    // 降级兜底：尝试点击隐藏触发节点（防止某些特殊环境下仅绑定了 DOM 点击事件）
    const trigger = document.getElementById(KAPA_TRIGGER_ID);
    if (trigger) {
      trigger.click();
      return true;
    }
    return false;
  }

  // If Kapa instance is already fully mounted and ready
  // 若 Kapa 官方实例已经挂载就绪，直接调用
  if (isKapaReady(kapa) && dispatchKapaOpen(initialQuery)) {
    return true;
  }

  // If script loaded successfully, wait briefly for Kapa instance to finish initialization
  // 若脚本加载成功，短轮询等待 Kapa 实例完成对象挂载（最多等待 1.5 秒）
  const startTime = Date.now();
  while (Date.now() - startTime < 1500) {
    const currentKapa = window.Kapa as any;
    if (isKapaReady(currentKapa)) {
      return dispatchKapaOpen(initialQuery);
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  // If queue exists, dispatch into queue so it pops up as soon as bundle runs
  // 若当前存在指令队列，推入队列以便 bundle 执行时自动弹出并消费参数
  if (dispatchKapaOpen(initialQuery)) {
    return true;
  }

  // Fallback: try clicking hidden trigger button in case DOM listeners are registered
  // 降级兜底：尝试点击隐藏触发节点（防止某些特殊环境下仅绑定了 DOM 点击事件）
  const trigger = document.getElementById(KAPA_TRIGGER_ID);
  if (trigger) {
    trigger.click();
    return true;
  }

  return false;
}
