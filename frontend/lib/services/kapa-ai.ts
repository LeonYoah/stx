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
 * Bird mark used by our custom themed launcher button.
 * 自定义主题悬浮按钮使用的青鸾图形标。
 */
export const KAPA_PROJECT_MARK_PATH = '/brand/stx-mark.png';
/**
 * Padded square mark for Kapa modal header (safe margin so crest is not clipped).
 * 带安全边距的方形青鸾标，专供 Kapa 弹窗标题（避免顶部被裁切）。
 */
export const KAPA_MODAL_MARK_PATH = '/brand/stx-mark-kapa.png';
/**
 * Sync widget theme with next-themes (`class="dark"` on <html>).
 * 与 next-themes 同步（<html> 上的 class="dark"）。
 */
export const KAPA_COLOR_SCHEME_SELECTOR = '.dark';
/** Modal header bird-mark display size (asset already has safe padding). / 弹窗标题青鸾标显示尺寸（资源自带安全边距） */
export const KAPA_MODAL_LOGO_SIZE_PX = '28';

/**
 * Origins / host patterns that may show Ask AI (console + widget).
 * 允许展示 Ask AI 的域名规则（控制台与 Widget）。
 */
export const KAPA_ALLOWED_ORIGIN_EXACT = ['https://leonyoah.github.io'] as const;
export const KAPA_ALLOWED_ORIGIN_PATTERNS = [
  /^https:\/\/\w+\.stx\.com$/,
  /^https:\/\/\w+\.120501\.xyz$/,
] as const;

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
 * Whether Ask AI is allowed on the given URL (or current location).
 * 当前（或指定）地址是否允许展示 / 调用 Ask AI。
 *
 * Allowed: leonyoah.github.io, *.stx.com, *.120501.xyz, localhost, 127.x.x.x
 * 允许：leonyoah.github.io、*.stx.com、*.120501.xyz、localhost、127.x.x.x
 */
export function isKapaAskAiAllowed(href?: string): boolean {
  if (typeof window === 'undefined' && !href) {
    return false;
  }

  let url: URL;
  try {
    url = new URL(href || window.location.href);
  } catch {
    return false;
  }

  const host = url.hostname.toLowerCase();
  if (host === 'localhost' || host === '127.0.0.1' || /^127\.\d+\.\d+\.\d+$/.test(host)) {
    return true;
  }

  if ((KAPA_ALLOWED_ORIGIN_EXACT as readonly string[]).includes(url.origin)) {
    return true;
  }

  return KAPA_ALLOWED_ORIGIN_PATTERNS.some((pattern) => pattern.test(url.origin));
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
 * Resolve modal / project logo URL.
 * Use the padded square mark so Kapa's header never clips the bird crest.
 * 弹窗 logo 使用带安全边距的方形青鸾标，避免标题栏裁切鸟冠。
 */
export function getKapaProjectLogo(_variant?: 'light' | 'dark'): string {
  return resolveBrandAssetUrl(KAPA_MODAL_MARK_PATH);
}

/**
 * Launcher FAB bird-mark (unpadded).
 * 悬浮按钮用的青鸾图形标（无额外边距）。
 */
export function getKapaLauncherImage(): string {
  return resolveBrandAssetUrl(KAPA_PROJECT_MARK_PATH);
}

/**
 * Inject CSS so Kapa modal logo stays fully visible inside the header slot.
 * 注入样式，确保 Kapa 弹窗 logo 在标题槽内完整可见。
 */
function ensureKapaLogoStyles(): void {
  const id = 'st-kapa-logo-fix';
  if (document.getElementById(id)) {
    return;
  }
  const style = document.createElement('style');
  style.id = id;
  style.textContent = `
    img[src*="stx-mark-kapa"],
    img[src*="stx-mark.png"] {
      object-fit: contain !important;
      object-position: center !important;
      max-height: 28px !important;
      max-width: 28px !important;
      height: 28px !important;
      width: 28px !important;
      padding: 0 !important;
      box-sizing: content-box !important;
    }
    /* Avoid parent overflow clipping the crest / 避免父级 overflow 裁切鸟冠 */
    img[src*="stx-mark-kapa"] {
      overflow: visible !important;
    }
  `;
  document.head.appendChild(style);
}

/**
 * Apply modal logo URL + compact sizing so the bird crest is not clipped.
 * 写入弹窗 logo，并控制尺寸，避免青鸾顶部被标题栏裁切。
 */
export function applyKapaThemeLogos(): void {
  if (typeof document === 'undefined') {
    return;
  }

  ensureKapaLogoStyles();

  const modalLogo = getKapaProjectLogo();
  const script = document.getElementById(KAPA_SCRIPT_ID);
  if (script) {
    script.setAttribute('data-project-logo', modalLogo);
    script.setAttribute('data-modal-logo-src', modalLogo);
  }

  const sizePx = `${KAPA_MODAL_LOGO_SIZE_PX}px`;
  document.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || '';
    const inFab = Boolean(img.closest('[data-testid="stx-ask-ai-fab"]'));
    if (inFab) {
      return;
    }

    // Force padded Kapa mark into any STX brand img inside the widget
    // 将 Widget 内 STX 品牌图统一为带边距的 Kapa 图形标
    if (
      src.includes('stx-logo') ||
      (src.includes('stx-mark') && !src.includes('stx-mark-kapa'))
    ) {
      if (img.src !== modalLogo) {
        img.src = modalLogo;
      }
    }

    if (src.includes('stx-mark') || src.includes('stx-logo')) {
      img.style.height = sizePx;
      img.style.width = sizePx;
      img.style.maxHeight = sizePx;
      img.style.maxWidth = sizePx;
      img.style.objectFit = 'contain';
      img.style.objectPosition = 'center';
      img.style.display = 'block';
    }
  });
}

/**
 * Watch theme + DOM mutations so logos stay correct when the modal mounts.
 * 监听主题与 DOM 变化，确保弹窗挂载后 logo 尺寸与资源仍正确。
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
  kapaThemeObserver.observe(document.body, {
    childList: true,
    subtree: true,
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

  // Hard gate: do not load Ask AI outside allowlisted domains
  // 硬门禁：非白名单域名不加载 Ask AI
  if (!isKapaAskAiAllowed()) {
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

  // Upgrade path: remount when old script still shows default launcher, wide lockup, or oversized logo
  // 升级路径：旧脚本仍展示默认悬浮球、宽锁章或未缩小 logo 时重建
  const existingScript = document.getElementById(KAPA_SCRIPT_ID);
  if (
    existingScript &&
    (existingScript.getAttribute('data-launcher-button-hidden') !== 'true' ||
      !(existingScript.getAttribute('data-project-logo') || '').includes(
        'stx-mark-kapa',
      ) ||
      existingScript.getAttribute('data-modal-image-height') !==
        KAPA_MODAL_LOGO_SIZE_PX)
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
    // Hide Kapa default launcher; STX renders a theme-aware custom button
    // 隐藏 Kapa 默认悬浮球；由 STX 渲染跟随主题的自定义按钮
    // Square mark only — wide lockups crop badly in the modal header slot
    // 仅用方形图形标——宽锁章会在弹窗标题槽里被裁坏
    const modalLogo = getKapaProjectLogo();
    script.setAttribute('data-project-logo', modalLogo);
    script.setAttribute('data-modal-logo-src', modalLogo);
    script.setAttribute('data-modal-title', 'Ask AI');
    // Shrink logo + give header a bit more vertical room so the crest is not clipped
    // 缩小 logo，并略增标题区高度，避免青鸾顶部被裁切
    script.setAttribute('data-modal-image-height', KAPA_MODAL_LOGO_SIZE_PX);
    script.setAttribute('data-modal-image-width', KAPA_MODAL_LOGO_SIZE_PX);
    script.setAttribute('data-modal-logo-height', `${KAPA_MODAL_LOGO_SIZE_PX}px`);
    script.setAttribute('data-modal-logo-width', `${KAPA_MODAL_LOGO_SIZE_PX}px`);
    script.setAttribute('data-modal-logo-max-height', `${KAPA_MODAL_LOGO_SIZE_PX}px`);
    script.setAttribute('data-modal-header-min-height', '56px');
    script.setAttribute('data-modal-header-padding-y', '14px');
    script.setAttribute('data-launcher-button-hidden', 'true');
    script.setAttribute('data-color-scheme-selector', KAPA_COLOR_SCHEME_SELECTOR);
    script.setAttribute('data-modal-override-open-id', KAPA_TRIGGER_ID);
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
  if (!isKapaAskAiAllowed()) {
    return false;
  }

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
