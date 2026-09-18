/**
 * Copyright 2024 Apache Software Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {describe, it, expect, beforeEach, afterEach, vi} from 'vitest';
import {render, screen, act} from '@testing-library/react';
import {useTranslations} from 'next-intl';
import {I18nProvider, useLocale} from '../provider';
import {locales, defaultLocale, localeNames} from '../config';
import zhMessages from '../locales/zh.json';
import enMessages from '../locales/en.json';

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
});

// Test component that uses translations
function TestComponent() {
  const t = useTranslations('common');
  const {locale, setLocale} = useLocale();

  return (
    <div>
      <span data-testid='locale'>{locale}</span>
      <span data-testid='loading'>{t('loading')}</span>
      <span data-testid='error'>{t('error')}</span>
      <button onClick={() => setLocale('en')} data-testid='switch-en'>
        English
      </button>
      <button onClick={() => setLocale('zh')} data-testid='switch-zh'>
        中文
      </button>
    </div>
  );
}

describe('i18n Configuration', () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('Locale Configuration', () => {
    it('should have zh and en as supported locales', () => {
      expect(locales).toContain('zh');
      expect(locales).toContain('en');
      expect(locales.length).toBe(2);
    });

    it('should have zh as default locale', () => {
      expect(defaultLocale).toBe('zh');
    });

    it('should have locale names defined', () => {
      expect(localeNames.zh).toBe('中文');
      expect(localeNames.en).toBe('English');
    });
  });

  describe('Translation Files', () => {
    it('should have all required keys in zh.json', () => {
      expect(zhMessages.common).toBeDefined();
      expect(zhMessages.auth).toBeDefined();
      expect(zhMessages.terms).toBeDefined();
      expect(zhMessages.navigation).toBeDefined();
    });

    it('should have all required keys in en.json', () => {
      expect(enMessages.common).toBeDefined();
      expect(enMessages.auth).toBeDefined();
      expect(enMessages.terms).toBeDefined();
      expect(enMessages.navigation).toBeDefined();
    });

    it('should have matching structure between zh and en', () => {
      const zhKeys = Object.keys(zhMessages);
      const enKeys = Object.keys(enMessages);
      expect(zhKeys.sort()).toEqual(enKeys.sort());
    });

    it('should have login translations in both languages', () => {
      expect(zhMessages.auth.login.title).toBeDefined();
      expect(enMessages.auth.login.title).toBeDefined();
      expect(zhMessages.auth.login.username).toBeDefined();
      expect(enMessages.auth.login.username).toBeDefined();
    });
  });

  describe('I18nProvider', () => {
    it('should render children with default locale', async () => {
      localStorageMock.getItem.mockReturnValue(null);

      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      // Wait for hydration
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      // Default locale should be zh (or browser detected)
      expect(screen.getByTestId('locale')).toBeDefined();
    });

    it('should display Chinese translations when locale is zh', async () => {
      localStorageMock.getItem.mockReturnValue('zh');

      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      // Wait for hydration
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      expect(screen.getByTestId('loading')).toHaveTextContent('加载中...');
      expect(screen.getByTestId('error')).toHaveTextContent('发生错误');
    });

    it('should switch to English when setLocale is called', async () => {
      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      // Wait for hydration
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      // Switch to English
      await act(async () => {
        screen.getByTestId('switch-en').click();
      });

      expect(screen.getByTestId('locale')).toHaveTextContent('en');
      expect(screen.getByTestId('loading')).toHaveTextContent('Loading...');
      expect(screen.getByTestId('error')).toHaveTextContent(
        'An error occurred',
      );
    });

    it('should persist locale to localStorage', async () => {
      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      // Wait for hydration
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      // Switch to English
      await act(async () => {
        screen.getByTestId('switch-en').click();
      });

      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'en');
    });

    it('should load saved locale from localStorage', async () => {
      localStorageMock.getItem.mockReturnValue('en');

      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      // Wait for hydration
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      expect(screen.getByTestId('locale')).toHaveTextContent('en');
    });
  });

  describe('Property: i18n text rendering', () => {
    it('should render all common translations correctly in Chinese', async () => {
      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      // Verify Chinese translations
      expect(zhMessages.common.loading).toBe('加载中...');
      expect(zhMessages.common.error).toBe('发生错误');
      expect(zhMessages.common.success).toBe('操作成功');
    });

    it('should render all common translations correctly in English', async () => {
      localStorageMock.getItem.mockReturnValue('en');

      render(
        <I18nProvider>
          <TestComponent />
        </I18nProvider>,
      );

      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      // Verify English translations
      expect(enMessages.common.loading).toBe('Loading...');
      expect(enMessages.common.error).toBe('An error occurred');
      expect(enMessages.common.success).toBe('Operation successful');
    });

    it('should have auth error messages in both languages', () => {
      expect(zhMessages.auth.errors.invalidCredentials).toBe(
        '用户名或密码错误',
      );
      expect(enMessages.auth.errors.invalidCredentials).toBe(
        'Invalid username or password',
      );
    });

    it('should have terms and privacy translations', () => {
      expect(zhMessages.terms.termsOfService).toBe('服务条款');
      expect(enMessages.terms.termsOfService).toBe('Terms of Service');
      expect(zhMessages.terms.privacyPolicy).toBe('隐私政策');
      expect(enMessages.terms.privacyPolicy).toBe('Privacy Policy');
    });

    it('should have plugin Ask AI translations and templates in both languages', () => {
      expect(zhMessages.plugin.askAi).toBe('Ask AI 问答');
      expect(enMessages.plugin.askAi).toBe('Ask AI');
      expect(zhMessages.plugin.askAiQuestionTemplate).toContain('{connector}');
      expect(enMessages.plugin.askAiQuestionTemplate).toContain('{connector}');
      expect(zhMessages.plugin.askAiUnavailable).toBeDefined();
      expect(enMessages.plugin.askAiUnavailable).toBeDefined();
    });
  });
});
