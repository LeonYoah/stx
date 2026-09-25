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

import type {Metadata} from 'next';
import {Toaster} from '@/components/ui/sonner';
import {ThemeProvider} from '@/components/common/layout/ThemeProvider';
import {I18nProvider} from '@/lib/i18n';
/**
 * 本地自托管字体（OFL），构建与运行均不依赖 Google 外部网络。
 * Self-hosted OFL fonts; no Google external network at build or runtime.
 */
import '@fontsource-variable/inter/index.css';
import '@fontsource-variable/noto-sans-sc/index.css';
import '@fontsource-variable/jetbrains-mono/index.css';
import './globals.css';

export const metadata: Metadata = {
  title: {
    template: '%s - STX',
    default: 'STX',
  },
  description: 'STX，Apache SeaTunnel 一站式运维管理平台',
  manifest: '/favicon/site.webmanifest',
  icons: {
    icon: [
      {
        url: '/favicon/favicon-16x16.png?v=qingluan-2',
        sizes: '16x16',
        type: 'image/png',
      },
      {
        url: '/favicon/favicon-32x32.png?v=qingluan-2',
        sizes: '32x32',
        type: 'image/png',
      },
      {
        url: '/favicon.ico?v=qingluan-2',
        sizes: 'any',
      },
    ],
    shortcut: '/favicon.ico?v=qingluan-2',
    apple: [
      {
        url: '/favicon/apple-touch-icon.png?v=qingluan-2',
        sizes: '180x180',
        type: 'image/png',
      },
    ],
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang='zh-CN' className='hide-scrollbar font-sans' suppressHydrationWarning>
      <body className='hide-scrollbar font-sans antialiased'>
        <I18nProvider>
          <ThemeProvider
            attribute='class'
            defaultTheme='dark'
            enableSystem={false}
            disableTransitionOnChange
          >
            {children}
            <Toaster />
          </ThemeProvider>
        </I18nProvider>
      </body>
    </html>
  );
}
