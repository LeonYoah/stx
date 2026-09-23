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
  KAPA_PROJECT_MARK_PATH,
  KAPA_PROJECT_NAME,
} from '../kapa-ai';

describe('kapa-ai service integration', () => {
  const originalEnvWebsiteId = process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;

  beforeEach(() => {
    document
      .querySelectorAll('#st-kapa-ai-widget-script, #st-kapa-ask-ai-trigger, #st-kapa-logo-fix')
      .forEach((el) => el.remove());
    delete (window as {Kapa?: unknown}).Kapa;
    process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID = originalEnvWebsiteId;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('allows Ask AI on allowlisted hosts only', () => {
    expect(isKapaAskAiAllowed('http://localhost:3000/plugins')).toBe(true);
    expect(isKapaAskAiAllowed('http://127.0.0.1:8080/')).toBe(true);
    expect(isKapaAskAiAllowed('https://leonyoah.github.io/stx-website/')).toBe(true);
    expect(isKapaAskAiAllowed('https://ops.stx.com/')).toBe(true);
    expect(isKapaAskAiAllowed('https://demo.120501.xyz/')).toBe(true);
    expect(isKapaAskAiAllowed('https://example.com/')).toBe(false);
    expect(isKapaAskAiAllowed('https://stx.com/')).toBe(false);
  });

  it('creates trigger element in DOM when ensuring widget', async () => {
    (window as unknown as {Kapa: {open: () => void}}).Kapa = {open: vi.fn()};
    expect(await ensureKapaWidget()).toBe(true);
    const trigger = document.getElementById('st-kapa-ask-ai-trigger');
    expect(trigger).not.toBeNull();
    expect(trigger?.style.display).toBe('none');
  });

  it('calls window.Kapa.open with query if available', async () => {
    const openMock = vi.fn();
    (window as unknown as {Kapa: {open: typeof openMock}}).Kapa = {open: openMock};
    expect(await openSeaTunnelAskAi('How to configure Clickhouse?')).toBe(true);
    expect(openMock).toHaveBeenCalledWith({query: 'How to configure Clickhouse?'});
  });

  it('calls window.Kapa as callable command queue if open is not a direct property', async () => {
    const kapaFnMock = vi.fn();
    (window as unknown as {Kapa: typeof kapaFnMock}).Kapa = kapaFnMock;
    expect(await openSeaTunnelAskAi('How to configure Kafka connector?')).toBe(true);
    expect(kapaFnMock).toHaveBeenCalledWith('open', {
      query: 'How to configure Kafka connector?',
    });
  });

  it('queues command before script finishes loading via preinitialization queue', async () => {
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => node.onload?.(new Event('load')), 10);
      }
      return result;
    });

    initKapaPreinitialization();
    expect(await openSeaTunnelAskAi('如何在 Apache SeaTunnel 中配置 MySQL?')).toBe(true);
    const kapaQueue = window.Kapa as {q?: unknown[][]};
    expect(kapaQueue.q?.[0]).toEqual([
      'open',
      {query: '如何在 Apache SeaTunnel 中配置 MySQL?'},
    ]);
  });

  it('falls back to trigger click if window.Kapa is not initialized', async () => {
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => node.onerror?.(new Event('error')), 10);
      }
      return result;
    });

    const triggerBtn = document.createElement('button');
    triggerBtn.id = 'st-kapa-ask-ai-trigger';
    const clickSpy = vi.fn();
    triggerBtn.addEventListener('click', clickSpy);
    origAppendChild(triggerBtn);

    expect(await openSeaTunnelAskAi()).toBe(true);
    expect(clickSpy).toHaveBeenCalled();
  });

  it('resolves Kapa website ID with environment variable override', () => {
    delete process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;
    expect(getKapaWebsiteId()).toBe(KAPA_DEFAULT_WEBSITE_ID);
    process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID = 'custom-user-kapa-id-123';
    expect(getKapaWebsiteId()).toBe('custom-user-kapa-id-123');
  });

  it('injects local mark and hides default launcher', async () => {
    delete process.env.NEXT_PUBLIC_KAPA_WEBSITE_ID;
    const origAppendChild = document.body.appendChild.bind(document.body);
    vi.spyOn(document.body, 'appendChild').mockImplementation((node) => {
      const result = origAppendChild(node);
      if (node instanceof HTMLScriptElement && node.id === 'st-kapa-ai-widget-script') {
        setTimeout(() => node.onload?.(new Event('load')), 10);
      }
      return result;
    });

    await ensureKapaWidget();
    const script = document.getElementById(
      'st-kapa-ai-widget-script',
    ) as HTMLScriptElement | null;
    expect(script?.getAttribute('data-website-id')).toBe(KAPA_DEFAULT_WEBSITE_ID);
    expect(script?.getAttribute('data-project-name')).toBe(KAPA_PROJECT_NAME);
    expect(script?.getAttribute('data-launcher-button-hidden')).toBe('true');
    expect(script?.getAttribute('data-color-scheme-selector')).toBe(
      KAPA_COLOR_SCHEME_SELECTOR,
    );
    expect(script?.getAttribute('data-project-logo')).toContain(KAPA_PROJECT_MARK_PATH);
    expect(script?.getAttribute('data-modal-title')).toBe('Ask AI');
    expect(script?.getAttribute('data-modal-image-width')).toBeNull();
  });
});
