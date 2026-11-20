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
import Script from 'next/script';
import {Toaster} from '@/components/ui/sonner';
import {ThemeProvider} from '@/components/common/layout/ThemeProvider';
import {I18nProvider} from '@/lib/i18n';
import {WelcomeSplash} from '@/components/common/splash/WelcomeSplash';
import {welcomeSplashBootScript} from '@/lib/welcome-splash';
/**
 * 本地自托管字体（OFL），构建与运行均不依赖 Google 外部网络。
 * Self-hosted OFL fonts; no Google external network at build or runtime.
 */
import '@fontsource-variable/inter/index.css';
import '@fontsource-variable/noto-sans-sc/index.css';
import '@fontsource-variable/jetbrains-mono/index.css';
import './globals.css';
import '@/components/arc/foundation.css';

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
    <html
      lang='zh-CN'
      data-theme='dark'
      className='hide-scrollbar font-sans'
      suppressHydrationWarning
    >
      <head>
        {/*
         * 首帧前判定是否播放登录开屏，命中则先铺黑色遮罩，杜绝首页先露出。
         * Decide before first paint whether to play the login splash; if so,
         * a black cover goes up first so the home page never flashes.
         */}
        <script dangerouslySetInnerHTML={{__html: welcomeSplashBootScript}} />
        {/*
         * 输入模态追踪：pointerdown 记 pointer、keydown 记 keyboard。
         * Radix 类控件关闭时脚本 refocus 触发器，Chromium 会把这次焦点判成
         * 键盘模态并保留 :focus-visible 描边；foundation.css 按此属性在指针
         * 模态下隐藏描边（产品约定：指针焦点不画环）。
         * Track input modality: pointerdown marks "pointer", keydown marks
         * "keyboard". Radix-style controls refocus their trigger on close and
         * Chromium counts that as keyboard modality, leaving a :focus-visible
         * ring after every mouse selection; foundation.css hides the ring
         * under pointer modality (product rule: pointer focus never rings).
         */}
        <Script
          id='input-modality-tracker'
          strategy='beforeInteractive'
          dangerouslySetInnerHTML={{
            __html: `(function(){var d=document.documentElement;d.dataset.inputModality="pointer";addEventListener("pointerdown",function(){d.dataset.inputModality="pointer"},true);addEventListener("keydown",function(){d.dataset.inputModality="keyboard"},true)})()`,
          }}
        />
      </head>
      {/* 浏览器扩展会向 body 注入属性（如 mpa-*），与 SSR 输出不符属预期噪音。
          Browser extensions inject attributes onto body (e.g. mpa-*); diverging from SSR output is expected noise. */}
      <body
        className='hide-scrollbar font-sans antialiased'
        suppressHydrationWarning
      >
        <I18nProvider>
          <ThemeProvider
            attribute='class'
            defaultTheme='dark'
            enableSystem={false}
            disableTransitionOnChange
          >
            {children}
            <Toaster />
            <WelcomeSplash />
          </ThemeProvider>
        </I18nProvider>
      </body>
    </html>
  );
}
