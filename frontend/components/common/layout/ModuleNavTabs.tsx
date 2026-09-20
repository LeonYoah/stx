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

import React from 'react';
import Link from 'next/link';
import {cn} from '@/lib/utils';

/**
 * 模块顶层导航 Tab 项定义
 * Navigation tab item definition for top-level twin modules
 */
export interface ModuleNavTabItem {
  /** 唯一标识 / Unique key */
  key: string;
  /** 显示标题 / Display label */
  label: React.ReactNode;
  /** 目标路由链接 / Target route link */
  href: string;
  /** 前置图标 / Leading icon */
  icon?: React.ReactNode;
  /** 可选的徽标数字或文字 / Optional badge count or text */
  badge?: React.ReactNode;
}

export interface ModuleNavTabsProps {
  /** 选项列表 / Tab items list */
  items: ModuleNavTabItem[];
  /** 当前激活的项 key / Currently active item key */
  activeKey: string;
  /** 自定义外层容器类名 / Custom container class name */
  className?: string;
}

/**
 * 模块顶层 Tab 切换栏（用于主机与集群、安装包与插件、错误与告警等孪生模块的无缝切换）
 * Top-level module tab bar for seamless switching between twin domains like Hosts & Clusters
 */
export function ModuleNavTabs({
  items,
  activeKey,
  className,
}: ModuleNavTabsProps) {
  return (
    <nav
      aria-label='Module navigation'
      className={cn(
        'inline-flex items-center gap-1 p-0.5 rounded-lg bg-muted/60 dark:bg-muted/40 border border-border/40 backdrop-blur-xs text-xs',
        className,
      )}
    >
      {items.map((item) => {
        const isActive = item.key === activeKey;

        return (
          <Link
            key={item.key}
            href={item.href}
            prefetch
            className={cn(
              'group relative inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md transition-all duration-150 select-none whitespace-nowrap',
              isActive
                ? 'bg-background text-foreground font-semibold shadow-2xs border border-border/50'
                : 'text-muted-foreground hover:text-foreground hover:bg-background/40 font-medium',
            )}
          >
            {item.icon && (
              <span
                className={cn(
                  'shrink-0 transition-colors [&_svg]:size-3.5',
                  isActive
                    ? 'text-primary'
                    : 'text-muted-foreground group-hover:text-foreground',
                )}
              >
                {item.icon}
              </span>
            )}
            <span>{item.label}</span>
            {item.badge && (
              <span className='ml-0.5 shrink-0'>{item.badge}</span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
