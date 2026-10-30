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

// 英文事件总数使用 ICU 复数，中文不区分单复数 / English event totals use ICU plural; Chinese has no plural form
import {createTranslator} from 'next-intl';
import {describe, expect, it} from 'vitest';
import en from '@/lib/i18n/locales/en.json';
import zh from '@/lib/i18n/locales/zh.json';

describe('workbenchStudio.jobEvents.totalBadge', () => {
  it('pluralizes the English event count', () => {
    const t = createTranslator({locale: 'en', messages: en, namespace: 'workbenchStudio.jobEvents'});
    expect(t('totalBadge', {total: 1})).toBe('1 event');
    expect(t('totalBadge', {total: 0})).toBe('0 events');
    expect(t('totalBadge', {total: 12})).toBe('12 events');
  });

  it('keeps the Chinese count format', () => {
    const t = createTranslator({locale: 'zh', messages: zh, namespace: 'workbenchStudio.jobEvents'});
    expect(t('totalBadge', {total: 3})).toBe('共 3 条');
  });
});
