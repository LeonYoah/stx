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

import type {CSSProperties} from 'react';
import type {LucideIcon} from 'lucide-react';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {cn} from '@/lib/utils';

// 图标视图切换在监控 .scope 之外也要保持 8px/28px 紧凑尺度，token 收敛进组件，
// 调用方不必各自重复局部覆盖。
// Compact 8px/28px control tokens live inside the component so icon view toggles
// stay consistent outside monitoring's .scope without per-caller overrides.
const COMPACT_SCOPE: CSSProperties = {
  ['--radius-control' as string]: '0.5rem',
  ['--control-height-sm' as string]: '1.75rem',
};

export interface ViewToggleOption {
  value: string;
  /** 段内图标，统一按 16px 渲染并 aria-hidden；无障碍名由 ariaLabel 提供。 / Segment icon, rendered at 16px and hidden from AT; the accessible name comes from ariaLabel. */
  icon: LucideIcon;
  ariaLabel: string;
  testId?: string;
}

/**
 * Icon-only segmented view switcher (e.g. table / card, list / grid) on top of
 * Arc SegmentedControl, with compact console tokens applied.
 * 纯图标分段视图切换（如 表格/卡片、列表/网格），基于 Arc SegmentedControl，
 * 内置控制台紧凑尺度。
 */
export function ViewToggle({
  label,
  value,
  onValueChange,
  options,
  className,
  testId,
}: {
  /** 整组的无障碍名 / Accessible name for the whole group */
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  options: ViewToggleOption[];
  className?: string;
  testId?: string;
}) {
  return (
    <span
      className={cn('inline-flex', className)}
      style={COMPACT_SCOPE}
      data-testid={testId}
    >
      <SegmentedControl
        label={label}
        value={value}
        onValueChange={onValueChange}
        options={options.map((option) => {
          const Icon = option.icon;
          return {
            value: option.value,
            label: <Icon className='size-4' aria-hidden />,
            ariaLabel: option.ariaLabel,
            testId: option.testId,
          };
        })}
      />
    </span>
  );
}
