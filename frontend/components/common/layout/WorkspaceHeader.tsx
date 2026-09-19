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
  /** 左侧图标组件 / Leading icon component */
  icon: React.ReactNode;
  /** 主标题 / Main title */
  title: React.ReactNode;
  /** 副标题 / Subtitle */
  subtitle?: React.ReactNode;
  /** 标题旁徽标 / Optional badge next to title */
  badge?: React.ReactNode;
  /** 右侧操作区 / Optional action buttons on the right */
  actions?: React.ReactNode;
  /**
   * 是否用主题色图标容器包装；品牌图（如 STXMark）应设为 false。
   * Wrap Lucide icons in theme primary container; set false for brand marks like STXMark.
   */
  iconTint?: boolean;
  /** 自定义容器类名 / Custom container class name */
  className?: string;
}

/**
 * 统一的工作空间紧凑型头部。
 * Unified compact workspace header.
 */
export function WorkspaceHeader({
  icon,
  title,
  subtitle,
  badge,
  actions,
  iconTint = true,
  className,
}: WorkspaceHeaderProps) {
  const containerRef = useRef<HTMLDivElement>(null);

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
        'workspace-header-content flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between',
        className,
      )}
    >
      <div className='flex items-center gap-2.5'>
        {iconTint ? (
          <div className='flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/20 shadow-2xs [&_svg]:size-[1.125rem]'>
            {icon}
          </div>
        ) : (
          <div className='size-8 shrink-0 [&_img]:size-full [&_img]:object-contain'>
            {icon}
          </div>
        )}
        <div>
          <div className='flex flex-wrap items-center gap-2'>
            <h1 className='text-lg font-bold leading-tight tracking-tight text-foreground'>
              {title}
            </h1>
            {badge}
          </div>
          {subtitle && (
            // 副标题可为含 div 的 ReactNode，用 div 避免 <p> 嵌套块级元素导致 hydration 报错
            // Subtitle may be a ReactNode with divs; use div to avoid invalid <p> nesting / hydration errors
            <div className='mt-0.5 text-xs text-muted-foreground'>{subtitle}</div>
          )}
        </div>
      </div>

      {actions && (
        <div className='flex flex-wrap items-center gap-2'>{actions}</div>
      )}
    </div>
  );
}
