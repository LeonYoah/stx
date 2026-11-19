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

import type {Preview} from '@storybook/nextjs-vite';
import {useEffect, type ReactNode} from 'react';
import {NextIntlClientProvider} from 'next-intl';
import '@fontsource-variable/inter/index.css';
import '@fontsource-variable/noto-sans-sc/index.css';
import '@fontsource-variable/jetbrains-mono/index.css';
import '../app/globals.css';
import '../components/arc/foundation.css';
import zhMessages from '../lib/i18n/locales/zh.json';
import enMessages from '../lib/i18n/locales/en.json';

type Messages = typeof zhMessages;
const messages: Record<'zh' | 'en', Messages> = {
  zh: zhMessages,
  en: enMessages as Messages,
};

function StoryShell({
  locale,
  theme,
  children,
}: {
  locale: 'zh' | 'en';
  theme: 'light' | 'dark';
  children: ReactNode;
}) {
  useEffect(() => {
    // Tailwind tokens read .dark; Arc foundation.css reads html[data-theme].
    // 双写与应用内 ThemeProvider 行为一致。
    document.documentElement.classList.toggle('dark', theme === 'dark');
    document.documentElement.dataset.theme = theme;
    document.documentElement.lang = locale === 'en' ? 'en' : 'zh-CN';
  }, [theme, locale]);
  return (
    <NextIntlClientProvider
      locale={locale}
      messages={messages[locale]}
      timeZone='Asia/Shanghai'
    >
      <div className='min-h-screen bg-background font-sans'>{children}</div>
    </NextIntlClientProvider>
  );
}

const preview: Preview = {
  globalTypes: {
    locale: {
      name: 'Locale',
      description: '界面语言',
      defaultValue: 'zh',
      toolbar: {
        icon: 'globe',
        items: [
          {value: 'zh', title: '中文'},
          {value: 'en', title: 'English'},
        ],
        showName: true,
      },
    },
    theme: {
      name: 'Theme',
      description: '主题',
      defaultValue: 'light',
      toolbar: {
        icon: 'mirror',
        items: [
          {value: 'light', title: 'Light', icon: 'sun'},
          {value: 'dark', title: 'Dark', icon: 'moon'},
        ],
      },
    },
  },
  decorators: [
    (Story, context) => (
      <StoryShell
        locale={context.globals.locale === 'en' ? 'en' : 'zh'}
        theme={context.globals.theme === 'dark' ? 'dark' : 'light'}
      >
        <Story />
      </StoryShell>
    ),
  ],
  parameters: {
    layout: 'fullscreen',
    backgrounds: {disable: true},
  },
};

export default preview;
