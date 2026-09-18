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

import React, {useRef} from 'react';
import {useGSAP} from '@gsap/react';
import {animateHairlineProgress} from '@/lib/animations/gsap-motion';
import {cn} from '@/lib/utils';

export interface TableLoadingBarProps {
  // 是否处于加载或刷新状态
  // Whether loading or background refreshing is active
  loading: boolean;
  // 自定义类名
  // Custom class name
  className?: string;
}

// 表格顶部极细微动效加载条（用于非阻塞后台刷新，避免高度塌陷和白屏闪烁）
// Hairline progress bar for non-blocking table reloads to eliminate height collapse and flicker
export function TableLoadingBar({loading, className}: TableLoadingBarProps) {
  const barRef = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      if (loading) {
        animateHairlineProgress('.table-hairline-progress');
      }
    },
    {scope: barRef, dependencies: [loading]},
  );

  if (!loading) {
    return null;
  }

  return (
    <div
      ref={barRef}
      className={cn(
        'relative h-[2px] w-full overflow-hidden bg-primary/10 select-none',
        className,
      )}
    >
      <div className='table-hairline-progress absolute inset-y-0 w-2/5 rounded-full bg-linear-to-r from-transparent via-primary to-transparent opacity-90' />
    </div>
  );
}
