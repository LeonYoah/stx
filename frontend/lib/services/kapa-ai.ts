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
 */

export const KAPA_SCRIPT_ID = 'st-kapa-ai-widget-script';
export const KAPA_TRIGGER_ID = 'st-kapa-ask-ai-trigger';
export const KAPA_DEFAULT_WEBSITE_ID = 'd9390efd-fdc5-4449-8aa1-bb2fd5fe13f3';
export const KAPA_PROJECT_NAME = 'STX';
export const KAPA_PROJECT_COLOR = '#2563eb';
/** Local bird mark for modal + FAB / 本地青鸾图形标（弹窗与悬浮按钮） */
export const KAPA_PROJECT_MARK_PATH = '/brand/stx-mark.png';
/** Sync widget chrome with next-themes / 与 next-themes 同步 */
export const KAPA_COLOR_SCHEME_SELECTOR = '.dark';

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
let kapaLogoObserver: MutationObserver | null = null;

export function getKapaWebsiteId(): string {
  return process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID || KAPA_DEFAULT_WEBSITE_ID;
}

/**
 * Whether Ask AI is allowed on the given URL (or current location).
 * 当前（或指定）地址是否允许展示 / 调用 Ask AI。
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

/** Same-origin mark URL for the Kapa script tag / 同域青鸾标 URL，供 Kapa 脚本使用 */
export function getKapaProjectLogo(): string {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}${KAPA_PROJECT_MARK_PATH}`;
  }
  return KAPA_PROJECT_MARK_PATH;
}

/**
 * Strip Mantine's fixed inline width on modal logo (crops the bird crest).
 * 去掉 Mantine 弹窗 logo 的固定 width（会裁切青鸾顶部）。
 */
function fixKapaModalLogoWidth(): void {
  if (typeof document === 'undefined') {
    return;
  }
  const id = 'st-kapa-logo-fix';
  if (!document.getElementById(id)) {
    const style = document.createElement('style');
    style.id = id;
    style.textContent = `
      img[src*="stx-mark"] {
        width: auto !important;
        max-width: none !important;
        height: 1.375rem !important;
        object-fit: contain !important;
      }
    `;
    document.head.appendChild(style);
  }

  document.querySelectorAll('img[src*="stx-mark"]').forEach((img) => {
    if (img.closest('[data-testid="stx-ask-ai-fab"]')) {
      return;
    }
    const el = img as HTMLImageElement;
    el.style.removeProperty('width');
    el.style.setProperty('width', 'auto', 'important');
  });
}

/** Watch modal mount and keep logo width unset / 监听弹窗挂载并维持 logo 无固定 width */
export function startKapaThemeSync(): void {
  if (typeof window === 'undefined' || kapaLogoObserver) {
    return;
  }
  fixKapaModalLogoWidth();
  kapaLogoObserver = new MutationObserver(() => {
    fixKapaModalLogoWidth();
  });
  kapaLogoObserver.observe(document.body, {childList: true, subtree: true});
}

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

function dispatchKapaOpen(query?: string): boolean {
  if (typeof window === 'undefined') {
    return false;
  }

  const kapa = window.Kapa as any;
  if (!kapa) {
    return false;
  }

  const options: KapaOpenOptions | undefined = query ? {query} : undefined;

  if (typeof kapa.open === 'function') {
    kapa.open(options);
    return true;
  }
  if (typeof kapa === 'function') {
    kapa('open', options);
    return true;
  }
  if (typeof kapa.openModal === 'function') {
    kapa.openModal(options);
    return true;
  }
  return false;
}

/**
 * Mount Kapa script (default launcher hidden; STX FAB opens the modal).
 * 挂载 Kapa 脚本（隐藏默认悬浮球；由 STX FAB 打开弹窗）。
 */
export function ensureKapaWidget(): Promise<boolean> {
  if (typeof window === 'undefined') {
    return Promise.resolve(false);
  }
  if (!isKapaAskAiAllowed()) {
    return Promise.resolve(false);
  }

  initKapaPreinitialization();

  let triggerBtn = document.getElementById(KAPA_TRIGGER_ID);
  if (!triggerBtn) {
    triggerBtn = document.createElement('button');
    triggerBtn.id = KAPA_TRIGGER_ID;
    triggerBtn.style.display = 'none';
    triggerBtn.setAttribute('aria-hidden', 'true');
    document.body.appendChild(triggerBtn);
  }

  if (isKapaReady(window.Kapa)) {
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
    script.async = true;
    script.setAttribute('data-website-id', getKapaWebsiteId());
    script.setAttribute('data-project-name', KAPA_PROJECT_NAME);
    script.setAttribute('data-project-color', KAPA_PROJECT_COLOR);
    script.setAttribute('data-project-logo', getKapaProjectLogo());
    script.setAttribute('data-modal-title', 'Ask AI');
    script.setAttribute('data-launcher-button-hidden', 'true');
    script.setAttribute('data-color-scheme-selector', KAPA_COLOR_SCHEME_SELECTOR);
    script.setAttribute('data-modal-override-open-id', KAPA_TRIGGER_ID);

    const timeoutId = setTimeout(() => resolve(false), 6000);
    script.onload = () => {
      clearTimeout(timeoutId);
      setTimeout(() => {
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

export async function openSeaTunnelAskAi(initialQuery?: string): Promise<boolean> {
  if (!isKapaAskAiAllowed()) {
    return false;
  }

  const loaded = await ensureKapaWidget();
  const kapa = window.Kapa as any;

  if (!loaded) {
    if (isKapaReady(kapa) && dispatchKapaOpen(initialQuery)) {
      return true;
    }
    const trigger = document.getElementById(KAPA_TRIGGER_ID);
    if (trigger) {
      trigger.click();
      return true;
    }
    return false;
  }

  if (isKapaReady(kapa) && dispatchKapaOpen(initialQuery)) {
    return true;
  }

  const startTime = Date.now();
  while (Date.now() - startTime < 1500) {
    if (isKapaReady(window.Kapa)) {
      return dispatchKapaOpen(initialQuery);
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  if (dispatchKapaOpen(initialQuery)) {
    return true;
  }

  const trigger = document.getElementById(KAPA_TRIGGER_ID);
  if (trigger) {
    trigger.click();
    return true;
  }
  return false;
}
