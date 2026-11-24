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

'use client';

import {KapaWidgetMount} from '@/components/common/layout/KapaWidgetMount';
import {ManagementBar} from '@/components/common/layout/ManagementBar';
import {RouteProgressBar} from '@/components/common/layout/RouteProgressBar';
import {OnboardingProvider} from '@/components/common/onboarding/OnboardingContext';
import {OnboardingMount} from '@/components/common/onboarding/OnboardingMount';
import {usePathname} from 'next/navigation';
import {memo, Suspense} from 'react';

const MemoizedManagementBar = memo(ManagementBar);

export default function ProjectLayout({children}: {children: React.ReactNode}) {
  const pathname = usePathname();
  // `/hosts/[id]` 复用 `/hosts` 的 HostMain + DetailHost，保持同一 key 避免切行同步地址栏时整页重挂。
  // `/hosts/[id]` reuses `/hosts`'s HostMain + DetailHost; keep one key so switching rows does not remount the page.
  const pageKey = pathname.startsWith('/hosts/') ? '/hosts' : pathname;
  // 工作台需要整屏贴合视口（内部自带三栏与控制台滚动），不能走滚动页的 pb-28 留白，
  // 否则双层高度叠加必出整页滚动条并把控制台顶出视口；其他控制台页面外壳保持原样。
  // The workbench must hug the viewport (it owns its own three-pane and console scroll);
  // the scroll-page pb-28 padding would stack heights past 100dvh, forcing a page
  // scrollbar and pushing the console off-screen. Other console pages keep their shell.
  const isWorkbench =
    pathname === '/workbench' || pathname.startsWith('/workbench/');

  return (
    <OnboardingProvider>
      <div
        className={
          isWorkbench
            ? 'h-dvh min-h-0 flex flex-col overflow-hidden'
            : 'min-h-screen flex flex-col'
        }
      >
        {/* 顶部轻量路由加载进度条 / Top lightweight route loading progress bar */}
        <Suspense fallback={null}>
          <RouteProgressBar />
        </Suspense>

        {/* Ask AI 右下角常驻悬浮按钮 / Persistent bottom-right Ask AI floating widget */}
        <KapaWidgetMount />

        {/* Arc 上手清单：避让 Ask AI，不改其工作台隐藏逻辑 / Onboarding float; avoid Ask AI; keep workbench hide */}
        <OnboardingMount />

        {/* 底部常驻 Dock 栏：保持挂载，页面切换时零卸载与零重绘 */}
        {/* Bottom persistent Dock bar: remains mounted across page switches without remounting */}
        <MemoizedManagementBar />

        <div
          className={
            isWorkbench
              ? 'flex min-h-0 flex-1 flex-col'
              : 'flex flex-1 flex-col'
          }
        >
          <div
            className={
              isWorkbench
                ? '@container/main flex min-h-0 flex-1 flex-col'
                : '@container/main flex flex-1 flex-col gap-2'
            }
          >
            {/* 统一页面容器与平滑进场淡入动画 */}
            {/* Unified page container with smooth entry fade-in animation */}
            <div
              key={pageKey}
              className={
                isWorkbench
                  ? 'flex min-h-0 flex-1 flex-col w-full max-w-none px-2.5 sm:px-3 pt-2.5 pb-[84px] animate-in fade-in-50 duration-200'
                  : 'flex flex-col gap-4 mb-8 pb-28 w-full max-w-none px-3 sm:px-4 lg:px-6 xl:px-8 2xl:px-10 py-6 md:gap-6 animate-in fade-in-50 duration-200'
              }
            >
              {children}
            </div>
          </div>
        </div>
      </div>
    </OnboardingProvider>
  );
}
