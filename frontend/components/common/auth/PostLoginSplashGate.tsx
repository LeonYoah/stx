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
