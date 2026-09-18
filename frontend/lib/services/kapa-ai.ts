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
 * Light lockup for light surfaces; dark lockup (white wordmark) for dark surfaces.
 * 浅色锁章用于亮色表面；深色锁章（白字）用于暗色表面。
 */
export const KAPA_PROJECT_LOGO_PATH = '/brand/stx-logo.png';
export const KAPA_PROJECT_LOGO_DARK_PATH = '/brand/stx-logo-dark.png';
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

/**
 * Get effective Kapa Website ID, allowing environment variable override
 * 获取当前生效的 Kapa Website ID，支持环境变量覆盖
 */
export function getKapaWebsiteId(): string {
  return process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID || KAPA_DEFAULT_WEBSITE_ID;
}

/**
 * Resolve project logo to an absolute URL for the Kapa widget
 * 将项目 logo 解析为 Kapa 小部件所需的绝对 URL（随部署域名变化）
 *
 * @param variant - light | dark lockup / 浅色或深色锁章
 */
export function getKapaProjectLogo(variant: 'light' | 'dark' = 'light'): string {
  const path =
    variant === 'dark' ? KAPA_PROJECT_LOGO_DARK_PATH : KAPA_PROJECT_LOGO_PATH;
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}${path}`;
  }
  return path;
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

  // If already loaded and functional (not an unconsumed queue placeholder), return immediately
  // 若已完全就绪（且非未消费的队列占位符），直接返回成功
  if (isKapaReady(window.Kapa)) {
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
    // Light/dark logos + sync with console theme via .dark on <html>
    // 浅/深色 logo，并通过 <html class="dark"> 与控制台主题同步
    const lightLogo = getKapaProjectLogo('light');
    const darkLogo = getKapaProjectLogo('dark');
    script.setAttribute('data-project-logo', lightLogo);
    script.setAttribute('data-project-logo-dark', darkLogo);
    script.setAttribute('data-modal-logo-src', lightLogo);
    script.setAttribute('data-modal-logo-src-dark', darkLogo);
    script.setAttribute('data-launcher-button-image', lightLogo);
    script.setAttribute('data-launcher-button-image-dark', darkLogo);
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
      // Wait for Kapa global object initialization
      // 等待 Kapa 全局对象初始化就绪
      setTimeout(() => resolve(true), 250);
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
