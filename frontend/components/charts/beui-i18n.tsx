/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Sparkline dependency (https://beui.dev/charts/sparkline).
 * Registry: https://beui.dev/r/sparkline.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 * 本地改动：beUI 的 lib/i18n.tsx 改放到 components/charts/beui-i18n.tsx，
 * 不占用 STX next-intl 的 frontend/lib/i18n/；lib/utils、lib/ease 用仓库已有版本。
 * Local change: beUI's lib/i18n.tsx lives at components/charts/beui-i18n.tsx so it
 * does not shadow STX's next-intl frontend/lib/i18n/; lib/utils and lib/ease are
 * the copies already in the repo.
 *
 * Local STX adaptations remain under the same MIT terms and are not
 * relicensed as Apache-2.0.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

'use client';

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
} from 'react';

/**
 * A component's messages. Plain strings with `{name}` placeholders, so a
 * dictionary stays JSON-serializable and can be passed from a server layout.
 * There are no plural forms: write messages that read correctly for any count.
 */
export type Messages = Record<string, string>;

/** A language file: component namespace → any subset of that component's messages. */
export type LocaleMessages = Record<string, Partial<Messages>>;

type LocaleContextValue = {
  locale?: string;
  messages: LocaleMessages;
};

const NO_MESSAGES: LocaleMessages = {};

const LocaleContext = createContext<LocaleContextValue>({
  messages: NO_MESSAGES,
});

/** Without a locale, digits stay plain so server and client render the same text. */
export function formatNumber(value: number, locale?: string) {
  return locale ? new Intl.NumberFormat(locale).format(value) : String(value);
}

export interface LocaleProviderProps {
  /** BCP 47 tag, e.g. `de`, `pt-BR`. Drives `Intl` number formatting. */
  locale: string;
  /**
   * Component namespace → translated strings. Missing keys fall back to English.
   * Pass a stable object (a module constant, imported JSON or a memoized value):
   * a new object each render gives every translated component a new `t`.
   */
  messages?: LocaleMessages;
  children: ReactNode;
}

export function LocaleProvider({
  locale,
  messages = NO_MESSAGES,
  children,
}: LocaleProviderProps) {
  const value = useMemo(() => ({locale, messages}), [locale, messages]);
  return (
    <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
  );
}

export function useLocale(): {locale?: string} {
  return {locale: useContext(LocaleContext).locale};
}

/**
 * Resolves a component's strings: `labels` prop, then the provider's
 * messages for `namespace`, then the component's English `defaults`.
 * Numbers passed as values are formatted for the provider's locale.
 */
export function useMessages<T extends Messages>(
  namespace: string,
  defaults: T,
  labels?: Partial<T>,
) {
  const {locale, messages} = useContext(LocaleContext);
  const provided = messages[namespace];
  // Inline `labels={{ … }}` is a new object every render. Keying it by content
  // keeps `t` one identity until a label changes, so effects and memos that
  // depend on `t` don't re-run per render, and render-time reads never lag.
  const labelsKey = labels ? JSON.stringify(labels) : '';
  const stableLabels = useMemo(
    () => (labelsKey ? (JSON.parse(labelsKey) as Partial<T>) : undefined),
    [labelsKey],
  );

  return useCallback(
    (key: keyof T & string, values?: Record<string, string | number>) => {
      const template = stableLabels?.[key] ?? provided?.[key] ?? defaults[key];
      if (!values) {
        return template;
      }
      return template.replace(/\{(\w+)\}/g, (match, name: string) => {
        const value = values[name];
        if (value === undefined) {
          return match;
        }
        return typeof value === 'number' ? formatNumber(value, locale) : value;
      });
    },
    [defaults, stableLabels, locale, provided],
  );
}
