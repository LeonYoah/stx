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

import {afterEach, describe, expect, it} from 'vitest';
import {applyRequestLocale} from '../api-client';
import {getCurrentLocale} from '@/lib/i18n/config';
import {translateApiError} from '@/lib/i18n/api-error-text';

function setLocale(locale: 'zh' | 'en') {
  window.localStorage.setItem('locale', locale);
}

afterEach(() => {
  window.localStorage.removeItem('locale');
});

describe('applyRequestLocale', () => {
  it('缺少 Accept-Language 时按当前语言注入 / injects Accept-Language from current locale', () => {
    setLocale('en');
    const config: {headers?: Record<string, unknown>} = {};
    applyRequestLocale(config);
    expect(config.headers?.['Accept-Language']).toBe('en');
  });

  it('未保存语言时跟随当前语言 / follows getCurrentLocale when nothing saved', () => {
    const config: {headers?: Record<string, unknown>} = {headers: {}};
    applyRequestLocale(config);
    expect(config.headers?.['Accept-Language']).toBe(getCurrentLocale());
  });

  it('尊重调用方显式覆写 / respects caller override', () => {
    setLocale('en');
    const config = {headers: {'Accept-Language': 'zh'}};
    applyRequestLocale(config);
    expect(config.headers['Accept-Language']).toBe('zh');
  });
});

describe('translateApiError', () => {
  it('按当前语言返回单语文案 / returns single-language text per locale', () => {
    setLocale('zh');
    expect(translateApiError('network.timeout', 'x')).toBe(
      '请求超时，请检查网络连接',
    );
    setLocale('en');
    expect(translateApiError('network.timeout', 'x')).toBe(
      'Request timed out. Check your network connection.',
    );
  });

  it('未知键回退原始文案 / unknown key falls back', () => {
    setLocale('en');
    expect(translateApiError('connector_missing', 'backend message')).toBe(
      'backend message',
    );
    expect(translateApiError(undefined, 'backend message')).toBe(
      'backend message',
    );
    expect(translateApiError('', 'backend message')).toBe('backend message');
  });
});
