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

import {describe, expect, it, vi, beforeEach, afterEach} from 'vitest';
import {
  ensureKapaWidget,
  getKapaWebsiteId,
  initKapaPreinitialization,
  isKapaAskAiAllowed,
  openSeaTunnelAskAi,
  KAPA_COLOR_SCHEME_SELECTOR,
  KAPA_DEFAULT_WEBSITE_ID,
  KAPA_PROJECT_LOGO_PATH,
  KAPA_PROJECT_NAME,
} from '../kapa-ai';

describe('kapa-ai service integration', () => {
  const originalEnvWebsiteId = process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;

  beforeEach(() => {
    // Clean up DOM and global mock before each test
    // 每个测试前重置 DOM 和全局对象模拟
    document
      .querySelectorAll('#st-kapa-ai-widget-script, #st-kapa-ask-ai-trigger')
      .forEach((el) => {
        el.remove();
      });
    delete (window as {Kapa?: unknown}).Kapa;
    process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID = originalEnvWebsiteId;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('allows Ask AI on localhost, 127.x, github pages and stx/120501 subdomains', () => {
    expect(isKapaAskAiAllowed('http://localhost:3000/plugins')).toBe(true);
    expect(isKapaAskAiAllowed('http://127.0.0.1:8080/')).toBe(true);
    expect(isKapaAskAiAllowed('http://127.1.2.3/')).toBe(true);
    expect(isKapaAskAiAllowed('https://leonyoah.github.io/stx-website/')).toBe(
      true,
    );
    expect(isKapaAskAiAllowed('https://ops.stx.com/')).toBe(true);
    expect(isKapaAskAiAllowed('https://demo.120501.xyz/')).toBe(true);
  });

  it('denies Ask AI on unrelated domains', () => {
    expect(isKapaAskAiAllowed('https://example.com/')).toBe(false);
    expect(isKapaAskAiAllowed('https://stx.com/')).toBe(false);
    expect(isKapaAskAiAllowed('http://192.168.1.1/')).toBe(false);
    expect(isKapaAskAiAllowed('https://evil.github.io/')).toBe(false);
  });

  it('creates trigger element in DOM when ensuring widget', async () => {
    // Inject mock Kapa object so ensure returns immediately
    // 注入模拟 Kapa 对象，使 ensureWidget 立即返回
    (window as unknown as {Kapa: {open: () => void}}).Kapa = {
      open: vi.fn(),
    };

    const ready = await ensureKapaWidget();
    expect(ready).toBe(true);

    const trigger = document.getElementById('st-kapa-ask-ai-trigger');
    expect(trigger).not.toBeNull();
    expect(trigger?.style.display).toBe('none');
  });

  it('calls window.Kapa.open with query if available', async () => {
    // Verify standard Kapa.open({ query }) API
    // 验证官方标准 Kapa.open({ query }) 调用
    const openMock = vi.fn();
    (window as unknown as {Kapa: {open: typeof openMock}}).Kapa = {
      open: openMock,
    };

    const success = await openSeaTunnelAskAi('How to configure Clickhouse?');
    expect(success).toBe(true);
    expect(openMock).toHaveBeenCalledWith({query: 'How to configure Clickhouse?'});
  });

  it('calls window.Kapa as callable command queue if open is not a direct property', async () => {
    // Verify callable command queue: window.Kapa('open', { query })
    // 验证可调用指令队列形式：window.Kapa('open', { query })
    const kapaFnMock = vi.fn();
    (window as unknown as {Kapa: typeof kapaFnMock}).Kapa = kapaFnMock;

    const success = await openSeaTunnelAskAi('How to configure Kafka connector?');
    expect(success).toBe(true);
    expect(kapaFnMock).toHaveBeenCalledWith('open', {
      query: 'How to configure Kafka connector?',
    });
  });

  it('queues command before script finishes loading via preinitialization queue', async () => {
    // Simulate script loading in background
    // 模拟脚本后台加载过程
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => {
          node.onload?.(new Event('load'));
        }, 10);
      }
      return result;
    });

    // Verify preinitialization queue mechanism preserves query
    // 验证预初始化队列机制能够在脚本运行前保留提问参数
    initKapaPreinitialization();
    expect(window.Kapa).toBeDefined();

    const success = await openSeaTunnelAskAi('如何在 Apache SeaTunnel 中配置 MySQL?');
    expect(success).toBe(true);

    const kapaQueue = window.Kapa as {q?: unknown[][]};
    expect(kapaQueue.q).toBeDefined();
    expect(kapaQueue.q?.length).toBeGreaterThan(0);
    expect(kapaQueue.q?.[0]).toEqual([
      'open',
      {query: '如何在 Apache SeaTunnel 中配置 MySQL?'},
    ]);
  });

  it('falls back to trigger click if window.Kapa is not initialized', async () => {
    // Mock script append failure
    // 模拟脚本加载失败场景
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => {
          node.onerror?.(new Event('error'));
        }, 10);
      }
      return result;
    });

    // Create trigger button with a click spy
    // 创建带有点击监听的触发器按钮
    const triggerBtn = document.createElement('button');
    triggerBtn.id = 'st-kapa-ask-ai-trigger';
    const clickSpy = vi.fn();
    triggerBtn.addEventListener('click', clickSpy);
    origAppendChild(triggerBtn);

    const success = await openSeaTunnelAskAi();
    expect(success).toBe(true);
    expect(clickSpy).toHaveBeenCalled();
  });

  it('resolves Kapa website ID with environment variable override', () => {
    // Verify fallback to default STX website ID
    // 验证默认使用 STX 自有 Website ID
    delete process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;
    expect(getKapaWebsiteId()).toBe(KAPA_DEFAULT_WEBSITE_ID);

    // Verify override when custom ID is provided
    // 验证提供自定义 ID 时的环境变量覆盖
    process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID = 'custom-user-kapa-id-123';
    expect(getKapaWebsiteId()).toBe('custom-user-kapa-id-123');
  });

  it('injects STX widget script with default launcher hidden for custom FAB', async () => {
    // Ensure script hides Kapa default button so STX themed FAB can own the UI
    // 确保脚本隐藏 Kapa 默认悬浮球，改由 STX 主题 FAB 接管
    delete process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => {
          node.onload?.(new Event('load'));
        }, 10);
      }
      return result;
    });

    await ensureKapaWidget();

    const script = document.getElementById(
      'st-kapa-ai-widget-script',
    ) as HTMLScriptElement | null;
    expect(script).not.toBeNull();
    expect(script?.getAttribute('data-website-id')).toBe(KAPA_DEFAULT_WEBSITE_ID);
    expect(script?.getAttribute('data-project-name')).toBe(KAPA_PROJECT_NAME);
    expect(script?.getAttribute('data-launcher-button-hidden')).toBe('true');
    expect(script?.getAttribute('data-color-scheme-selector')).toBe(
      KAPA_COLOR_SCHEME_SELECTOR,
    );
    expect(script?.getAttribute('data-project-logo')).toContain(KAPA_PROJECT_LOGO_PATH);
  });
});
