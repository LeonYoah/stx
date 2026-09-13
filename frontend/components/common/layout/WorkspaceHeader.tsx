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
import gsap from 'gsap';
import {useGSAP} from '@gsap/react';
import {cn} from '@/lib/utils';
import {shouldReduceMotion} from '@/lib/animations/gsap-motion';

export interface WorkspaceHeaderProps {
  // 左侧图标组件
  // Leading icon component
  icon: React.ReactNode;
  // 主标题文本或节点
  // Main title text or node
  title: React.ReactNode;
  // 副标题或描述信息
  // Subtitle or description text
  subtitle?: React.ReactNode;
  // 标题旁的徽标标签（可选）
  // Optional badge or tag next to title
  badge?: React.ReactNode;
  // 右侧操作区按钮（可选）
  // Optional action buttons on the right
  actions?: React.ReactNode;
  // 自定义容器类名
  // Custom container class name
  className?: string;
}

// 统一的工作空间紧凑型头部组件（释放垂直可用空间，内置微动效）
// Unified compact workspace header component (conserves vertical space, includes subtle entrance motion)
export function WorkspaceHeader({
  icon,
  title,
  subtitle,
  badge,
  actions,
  className,
}: WorkspaceHeaderProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // GSAP 头部轻量级淡入动效
  // GSAP subtle entrance fade-in animation for header
  useGSAP(
    () => {
      if (shouldReduceMotion()) {
        return;
      }
      gsap.from('.workspace-header-content', {
        opacity: 0,
        y: -6,
        duration: 0.28,
        ease: 'power2.out',
        clearProps: 'opacity,transform',
      });
    },
    {scope: containerRef},
  );

  return (
    <div
      ref={containerRef}
      className={cn(
        'workspace-header-content flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between border-b pb-3',
        className,
      )}
    >
      <div className='flex items-center gap-3'>
        <div className='flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/20 shadow-2xs'>
          {icon}
        </div>
        <div>
          <div className='flex items-center gap-2 flex-wrap'>
            <h1 className='text-xl font-bold tracking-tight text-foreground'>
              {title}
            </h1>
            {badge}
          </div>
          {subtitle && (
            <p className='text-xs text-muted-foreground mt-0.5'>{subtitle}</p>
          )}
        </div>
      </div>

      {actions && (
        <div className='flex flex-wrap items-center gap-2'>{actions}</div>
      )}
    </div>
  );
}
