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
import {usePathname} from 'next/navigation';
import {memo, Suspense} from 'react';

const MemoizedManagementBar = memo(ManagementBar);

export default function ProjectLayout({children}: {children: React.ReactNode}) {
  const pathname = usePathname();

  return (
    <div className='min-h-screen flex flex-col'>
      {/* 顶部轻量路由加载进度条 / Top lightweight route loading progress bar */}
      <Suspense fallback={null}>
        <RouteProgressBar />
      </Suspense>

      {/* Ask AI 右下角常驻悬浮按钮 / Persistent bottom-right Ask AI floating widget */}
      <KapaWidgetMount />

      {/* 底部常驻 Dock 栏：保持挂载，页面切换时零卸载与零重绘 */}
      {/* Bottom persistent Dock bar: remains mounted across page switches without remounting */}
      <MemoizedManagementBar />

      <div className='flex flex-1 flex-col'>
        <div className='@container/main flex flex-1 flex-col gap-2'>
          {/* 统一页面容器与平滑进场淡入动画 */}
          {/* Unified page container with smooth entry fade-in animation */}
          <div
            key={pathname}
            className='flex flex-col gap-4 mb-8 w-full max-w-none px-3 sm:px-4 lg:px-6 xl:px-8 2xl:px-10 py-6 md:gap-6 animate-in fade-in-50 duration-200'
          >
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
