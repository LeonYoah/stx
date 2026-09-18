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
 * Official Apache SeaTunnel Kapa.ai Ask AI Integration
 * 官方 Apache SeaTunnel Kapa.ai 智能问答服务集成
 *
 * Connects directly to the production kapa.ai widget configured on seatunnel.apache.org.
 * 直接连接 seatunnel.apache.org 官网配置的生产级 kapa.ai 智能问答组件。
 */

// Kapa configuration constants matching apache/seatunnel-website
// 与 apache/seatunnel-website 官网生产保持一致的配置常量
export const KAPA_SCRIPT_ID = 'st-kapa-ai-widget-script';
export const KAPA_TRIGGER_ID = 'st-kapa-ask-ai-trigger';
export const KAPA_DEFAULT_WEBSITE_ID = '3a335e8d-d400-4c7d-baad-d820ee0600a7';
export const KAPA_PROJECT_NAME = 'Apache SeaTunnel';
export const KAPA_PROJECT_COLOR = '#0284c7';
export const KAPA_PROJECT_LOGO = 'https://seatunnel.apache.org/image/logo.png';

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
 * Ensure Kapa widget script and hidden trigger element are mounted
 * 确保 Kapa widget 脚本与隐藏触发节点已挂载到 DOM
 *
 * @returns Promise<boolean> - Whether Kapa widget is loaded / 是否成功加载 Kapa 小部件
 */
export function ensureKapaWidget(): Promise<boolean> {
  if (typeof window === 'undefined') {
    return Promise.resolve(false);
  }

  initKapaPreinitialization();

  // Ensure trigger element exists in DOM
  // 确保触发器元素存在于 DOM 中
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
    script.setAttribute('data-project-logo', KAPA_PROJECT_LOGO);
    script.setAttribute('data-modal-override-open-id', KAPA_TRIGGER_ID);
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
 * Trigger opening the SeaTunnel Ask AI modal dialog with optional query
 * 触发唤起 SeaTunnel 官网 Ask AI 对话框，可携带预填提问内容
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
