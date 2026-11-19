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

import type {CSSProperties, ReactNode} from 'react';
import {cn} from '@/lib/utils';

/**
 * 首页大盘图表面板：主 / 次两档外壳统一视觉层级，`empty` 时收成一行矮条
 * （标题 + 空态文案），不再让无数据面板顶着一个大空框。
 * Home dashboard chart panel: primary / secondary shells keep one visual
 * hierarchy; `empty` collapses it into a one-line strip (title + empty text)
 * instead of a tall empty frame.
 */
export function DashboardPanel({
  title,
  hint,
  actions,
  variant = 'secondary',
  flush = false,
  empty = false,
  emptyLabel,
  testId,
  className,
  style,
  children,
}: {
  title?: string;
  hint?: string;
  actions?: ReactNode;
  /** 主面板是全实底卡，次要面板淡底淡边。/ Primary is a solid card; secondary is lighter. */
  variant?: 'primary' | 'secondary';
  /** 去掉内边距（指标页签行、表格面板自带边距）。/ Drop the padding — tab rows and tables pad themselves. */
  flush?: boolean;
  /** 数据已加载且为空时渲染矮条，不渲染 actions / children。/ Loaded-but-empty renders a strip without actions or children. */
  empty?: boolean;
  emptyLabel?: string;
  testId?: string;
  className?: string;
  /** 面板级 CSS 变量覆盖（如图表主题色）。/ Panel-level CSS variable overrides (e.g. chart accent). */
  style?: CSSProperties;
  children?: ReactNode;
}) {
  if (empty) {
    return (
      <section
        className={cn(
          'flex h-11 items-center gap-3 rounded-xl border border-dashed border-border/60 px-4 text-xs',
          className,
        )}
        style={style}
        data-testid={testId}
      >
        {title ? <h3 className='text-foreground/80'>{title}</h3> : null}
        {emptyLabel ? (
          <span className='truncate text-muted-foreground'>{emptyLabel}</span>
        ) : null}
      </section>
    );
  }

  return (
    <section
      className={cn(
        'rounded-xl',
        variant === 'primary'
          ? 'border border-border/70 bg-card'
          : 'border border-border/50 bg-card/30',
        !flush && 'p-4',
        className,
      )}
      style={style}
      data-testid={testId}
    >
      {title || hint || actions ? (
        <div
          className={cn(
            'flex flex-wrap items-center gap-2',
            flush ? 'px-4 pt-3 pb-2' : 'mb-2',
          )}
        >
          {title ? <h3 className='text-sm font-medium'>{title}</h3> : null}
          {hint ? (
            <span className='text-xs text-muted-foreground'>{hint}</span>
          ) : null}
          {/* 窄屏时控件换到下一行，并在容器内横向滚动，不撑破页面。
              On narrow screens controls wrap and scroll inside. */}
          {actions ? (
            <div className='w-full max-w-full overflow-x-auto sm:ml-auto sm:w-auto'>
              {actions}
            </div>
          ) : null}
        </div>
      ) : null}
      {children}
    </section>
  );
}
