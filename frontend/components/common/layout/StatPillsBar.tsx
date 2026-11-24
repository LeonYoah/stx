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
import {cn} from '@/lib/utils';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';

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
  // 分组无障碍名（透传给底层 SegmentedControl 的 aria-label）
  // Accessible group label forwarded to the underlying SegmentedControl
  label?: string;
  // 单行模式：左侧胶囊区横向滚动，右侧操作区不换行不收缩（避免窄屏把操作按钮折成两行并裁切）。
  // Single-row mode: pills scroll horizontally on the left while the actions slot stays
  // non-wrapping and non-shrinking, preventing narrow widths from stacking and clipping controls.
  singleRow?: boolean;
}

// 计数微标的语义配色：仅在数值非 0 时上色变体，0 保持灰色降噪。
// Count chip tinting: variant color only applies to non-zero counts; zeros stay muted.
const COUNT_TONE: Record<StatPillVariant, string> = {
  danger: 'text-destructive',
  warning: 'text-amber-600 dark:text-amber-400',
  success: 'text-emerald-600 dark:text-emerald-400',
  info: 'text-blue-600 dark:text-blue-400',
  default: 'text-foreground/80',
};

function CountAccessory({
  item,
}: {
  item: StatPillItem;
}) {
  const count = item.count ?? 0;
  const nonzero = typeof count === 'number' ? count > 0 : Boolean(count);
  return (
    <span className='ml-1 inline-flex items-center gap-1 align-middle'>
      {item.pulse ? (
        <span className='relative flex h-1.5 w-1.5'>
          <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-rose-400 opacity-75' />
          <span className='relative inline-flex h-1.5 w-1.5 rounded-full bg-rose-500' />
        </span>
      ) : null}
      {item.count !== undefined ? (
        <span
          className={cn(
            'rounded-full bg-muted/70 px-1 font-mono text-[10px] leading-4',
            nonzero
              ? COUNT_TONE[item.variant ?? 'default']
              : 'text-muted-foreground',
          )}
        >
          {item.count}
        </span>
      ) : null}
    </span>
  );
}

// 统一的紧凑状态胶囊栏：内部复用 Arc SegmentedControl，与顶栏「我的/全部」等
// 段控件共用轨道与滑动选中胶囊动效，API 与原 StatPillItem 保持不变。
// Unified compact status pills bar: internally backed by the Arc
// SegmentedControl, sharing the track and morphing selection pill with the
// top-bar segmented controls; the StatPillItem API is unchanged.
export function StatPillsBar({
  items,
  activeKey,
  onChange,
  actions,
  className,
  label,
  singleRow = false,
}: StatPillsBarProps) {
  return (
    <div
      className={cn(
        'flex min-w-0 items-center justify-between gap-2.5',
        singleRow ? 'flex-nowrap overflow-hidden' : 'flex-wrap',
        className,
      )}
    >
      {/* 胶囊轨道外包横向滚动容器：窄屏平滑横滚而不裁切右侧计数徽标。 */}
      {/* Scroll wrapper around the segmented track: narrow widths scroll cleanly instead of clipping pills. */}
      <div
        className={cn(
          'min-w-0 overflow-x-auto scrollbar-none',
          singleRow ? 'flex-1' : 'max-w-full',
        )}
      >
        <SegmentedControl
          label={label}
          value={activeKey}
          onValueChange={onChange}
          className='w-max min-w-0'
          options={items.map((item) => ({
            value: item.key,
            label: item.icon ? (
              <span className='inline-flex items-center gap-1'>
                {item.icon}
                {item.label}
              </span>
            ) : (
              item.label
            ),
            accessory:
              item.count !== undefined || item.pulse ? (
                <CountAccessory item={item} />
              ) : undefined,
            testId: item.dataTestId,
          }))}
        />
      </div>

      {/* 右侧扩展操作区：单行模式禁止换行与收缩，防止高度超出行高被裁切。 */}
      {/* Right optional action buttons slot: single-row mode stays nowrap + shrink-0 so controls never stack and clip. */}
      {actions && (
        <div
          className={cn(
            'flex shrink-0 items-center gap-1.5',
            singleRow ? 'flex-nowrap' : 'flex-wrap',
          )}
        >
          {actions}
        </div>
      )}
    </div>
  );
}
