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
import {cn} from '@/lib/utils';
import {animatePillsEntrance, animatePulsingRadar} from '@/lib/animations/gsap-motion';

export type StatPillVariant = 'default' | 'danger' | 'warning' | 'success' | 'info';

export interface StatPillItem {
  // 胶囊唯一标识键
  // Unique key for the pill item
  key: string;
  // 胶囊展示标签
  // Display label text
  label: string;
  // 统计数值或标记
  // Count number or tag
  count?: number | string;
  // 图标组件（可选）
  // Optional icon component
  icon?: React.ReactNode;
  // 语义高亮变体（如 danger 呈现红色警示）
  // Semantic highlight variant (e.g. danger renders red warning style)
  variant?: StatPillVariant;
  // 是否启用呼吸雷达环微动效（如告警触发中或存在严重错误时）
  // Whether to enable pulsing radar glow motion (e.g. firing alerts or critical errors)
  pulse?: boolean;
  // 测试标识
  // Test identifier
  dataTestId?: string;
}

export interface StatPillsBarProps {
  // 胶囊列表项
  // List of pill items to render
  items: StatPillItem[];
  // 当前选中的胶囊键名
  // Currently active pill key
  activeKey: string;
  // 切换胶囊时的回调函数
  // Callback when a pill is selected
  onChange: (key: string) => void;
  // 右侧辅助操作区（如重置、刷新按钮等）
  // Optional right action buttons (e.g. reset, refresh)
  actions?: React.ReactNode;
  // 自定义容器样式
  // Custom container class name
  className?: string;
}

// 统一的紧凑状态胶囊栏组件（取代臃肿的大 KPI 卡片，释放首屏空间）
// Unified compact status pills bar component (replaces bulky KPI cards, maximizes viewport space)
export function StatPillsBar({
  items,
  activeKey,
  onChange,
  actions,
  className,
}: StatPillsBarProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // GSAP 辅助动效：状态胶囊交错淡入，活跃警报光环脉冲
  // GSAP auxiliary motion: staggered pill entrance and active radar pulse ring
  useGSAP(
    () => {
      animatePillsEntrance('.stat-pill-item');
      animatePulsingRadar('.radar-pulse-ring');
    },
    {scope: containerRef, dependencies: [items.length]},
  );

  // 解析胶囊的样式配色
  // Resolve color and border styles based on semantic variant and active state
  const getPillStyles = (item: StatPillItem) => {
    const isActive = activeKey === item.key;

    if (isActive) {
      switch (item.variant) {
        case 'danger':
          return 'bg-destructive/15 text-destructive border-destructive/30 shadow-2xs font-semibold';
        case 'warning':
          return 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30 shadow-2xs font-semibold';
        case 'success':
          return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30 shadow-2xs font-semibold';
        case 'info':
          return 'bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/30 shadow-2xs font-semibold';
        default:
          return 'bg-primary/10 text-primary border-primary/30 shadow-2xs font-semibold';
      }
    }

    return 'text-muted-foreground border-transparent hover:text-foreground hover:bg-muted/60';
  };

  return (
    <div
      ref={containerRef}
      className={cn(
        'flex flex-wrap items-center justify-between gap-2.5',
        className,
      )}
    >
      {/* 左侧状态胶囊选择组 */}
      {/* Left status pill selector group */}
      <div className='flex items-center gap-1.5 flex-wrap'>
        {items.map((item) => {
          const isActive = activeKey === item.key;
          return (
            <button
              key={item.key}
              type='button'
              data-testid={item.dataTestId}
              onClick={() => onChange(item.key)}
              className={cn(
                'stat-pill-item relative inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border transition-all duration-150 active:scale-[0.97] cursor-pointer select-none',
                getPillStyles(item),
              )}
            >
              {/* 持续呼吸脉冲光环（存在严重警告或触发状态时呈现） */}
              {/* Continuous pulsing radar ring indicator for critical/active states */}
              {item.pulse && (
                <span className='relative flex h-2 w-2'>
                  <span className='radar-pulse-ring absolute inline-flex h-full w-full rounded-full bg-rose-400 opacity-75' />
                  <span className='relative inline-flex rounded-full h-2 w-2 bg-rose-500' />
                </span>
              )}

              {item.icon}
              <span>{item.label}</span>

              {item.count !== undefined && (
                <span
                  className={cn(
                    'font-mono font-semibold px-1.5 py-0.5 rounded-full text-[11px] leading-none',
                    isActive
                      ? 'bg-background/80 shadow-2xs'
                      : 'bg-muted text-muted-foreground',
                  )}
                >
                  {item.count}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* 右侧扩展操作区 */}
      {/* Right optional action buttons slot */}
      {actions && (
        <div className='flex items-center gap-1.5 flex-wrap'>{actions}</div>
      )}
    </div>
  );
}
