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

import dynamic from 'next/dynamic';
import {useCallback, useEffect, useState} from 'react';
import {consumePostLoginSplashPending} from '@/lib/splash/post-login-splash-session';

/**
 * 动态加载开屏本体，避免日常导航打进主包。
 * Dynamically load the splash body so everyday navigation stays out of the main bundle.
 */
const PostLoginSplash = dynamic(
  () =>
    import('./PostLoginSplash').then((mod) => mod.PostLoginSplash),
  {ssr: false, loading: () => null},
);

/**
 * 主壳门控：仅在会话 pending 时挂载开屏；失败/跳过/结束不阻塞工作台。
 * Main-shell gate: mount splash only when session pending; never block the workbench on failure/skip/end.
 *
 * W2 说明 / W2 note:
 * StrictMode 会二次跑 effect，但会保留 state：第一次 consume 后 setActive(true)，
 * 第二次 consume 为 false 时不会写回 false，因此不会「一定丢播」。
 * 已用 StrictMode 单测验证。刻意不使用模块级 latch，以免布局软 remount / 切页误重播。
 * StrictMode re-runs effects but preserves state: the first consume sets active true;
 * the second consume returning false does not clear it — verified by StrictMode tests.
 * We intentionally avoid a module-level latch that could replay across soft remounts / nav.
 */
export function PostLoginSplashGate() {
  const [active, setActive] = useState(false);

  useEffect(() => {
    // 客户端一次性消费 pending；SSR/首屏 hydration 不渲染动画层
    // Consume pending once on the client; skip splash during SSR / initial hydration
    try {
      if (consumePostLoginSplashPending()) {
        setActive(true);
      }
    } catch (error) {
      console.error('Post-login splash gate failed:', error);
      setActive(false);
    }
  }, []);

  const handleDismiss = useCallback(() => {
    setActive(false);
  }, []);

  if (!active) {
    return null;
  }

  return <PostLoginSplash onDismiss={handleDismiss} />;
}
