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

import {useEffect, useState, useRef} from 'react';
import {usePathname, useSearchParams} from 'next/navigation';

/**
 * 顶部轻量路由切换进度条：提供即时、丝滑的路由跳转视觉反馈
 * Top lightweight route transition progress bar: provides instant and smooth navigation visual feedback
 */
export function RouteProgressBar() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [isNavigating, setIsNavigating] = useState(false);
  const [progress, setProgress] = useState(0);
  const isNavigatingRef = useRef(false);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  // 路由变化时，推进到 100% 并平滑淡出
  // When route changes, advance to 100% and fade out smoothly
  useEffect(() => {
    if (isNavigatingRef.current) {
      isNavigatingRef.current = false;
      setProgress(100);
      const timer = setTimeout(() => {
        setIsNavigating(false);
        setProgress(0);
      }, 220);
      return () => clearTimeout(timer);
    }
  }, [pathname, searchParams]);

  // 全局监听内部导航链接点击，零延迟启动过渡进度条
  // Listen globally for internal link clicks to start transition progress bar with zero latency
  useEffect(() => {
    const handleClick = (event: MouseEvent) => {
      const target = (event.target as HTMLElement).closest('a');
      if (!target || !target.href) {
        return;
      }

      // 忽略新标签页打开、下载或修饰键点击
      // Ignore open in new tab, download, or modifier key clicks
      if (
        target.target === '_blank' ||
        target.hasAttribute('download') ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }

      try {
        const url = new URL(target.href, window.location.href);
        // 目标与当前同源且路径或查询参数发生变化
        // Target is same origin and path or query has changed
        if (
          url.origin === window.location.origin &&
          (url.pathname !== window.location.pathname ||
            url.search !== window.location.search)
        ) {
          isNavigatingRef.current = true;
          setIsNavigating(true);
          setProgress(25);

          if (timerRef.current) {
            clearInterval(timerRef.current);
          }

          // 渐进增加进度条，营造加载动势
          // Increment progress bar to simulate loading momentum
          timerRef.current = setInterval(() => {
            setProgress((prev) => {
              if (prev >= 85) {
                if (timerRef.current) {
                  clearInterval(timerRef.current);
                }
                return 85;
              }
              return prev + 15;
            });
          }, 100);
        }
      } catch {
        // 忽略无效 URL 异常
        // Ignore invalid URL exceptions
      }
    };

    document.addEventListener('click', handleClick, {capture: true});
    return () => {
      document.removeEventListener('click', handleClick, {capture: true});
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
    };
  }, []);

  if (!isNavigating && progress === 0) {
    return null;
  }

  return (
    <div
      aria-hidden='true'
      className='fixed top-0 left-0 right-0 h-[2px] z-[9999] pointer-events-none transition-opacity duration-200'
      style={{
        opacity: progress === 100 ? 0 : 1,
      }}
    >
      <div
        className='h-full bg-gradient-to-r from-primary/50 via-primary to-primary shadow-[0_0_8px_rgba(59,130,246,0.6)] transition-all ease-out'
        style={{
          width: `${progress}%`,
          transitionDuration: progress === 100 ? '160ms' : '240ms',
        }}
      />
    </div>
  );
}
