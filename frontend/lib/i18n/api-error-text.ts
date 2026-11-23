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

import {createTranslator} from 'next-intl';
import {getCurrentLocale, type Locale} from './config';
import zhMessages from './locales/zh.json';
import enMessages from './locales/en.json';

type Messages = typeof zhMessages;

const messagesByLocale: Record<Locale, Messages> = {
  zh: zhMessages,
  en: enMessages as Messages,
};

type ApiErrorTranslator = ReturnType<
  typeof createTranslator<Messages, 'apiErrors'>
>;

const translatorCache = new Map<Locale, ApiErrorTranslator>();

function apiErrorTranslator(locale: Locale) {
  const cached = translatorCache.get(locale);
  if (cached) {
    return cached;
  }
  const translator = createTranslator({
    locale,
    messages: messagesByLocale[locale],
    namespace: 'apiErrors',
    onError: () => undefined,
  });
  translatorCache.set(locale, translator);
  return translator;
}

function lookupLeaf(messages: unknown, path: string): string | undefined {
  let node: unknown = messages;
  for (const part of path.split('.')) {
    if (node === null || typeof node !== 'object') {
      return undefined;
    }
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === 'string' ? node : undefined;
}

export function translateApiError(
  key: string | undefined,
  fallback: string,
  values?: Record<string, string | number>,
): string {
  const trimmed = (key ?? '').trim();
  if (!trimmed) {
    return fallback;
  }
  const locale = getCurrentLocale();
  if (lookupLeaf(messagesByLocale[locale].apiErrors, trimmed) === undefined) {
    return fallback;
  }
  try {
    const translated = apiErrorTranslator(locale)(
      trimmed as never,
      values as never,
    ) as unknown;
    if (typeof translated === 'string' && translated.trim() !== '') {
      return translated;
    }
  } catch {
    return fallback;
  }
  return fallback;
}
