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

import type {ReactNode} from 'react';
import {Card, CardContent} from '@/components/ui/card';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {HELP_HINT_DELAY_MS} from '@/components/common/HelpHint';
import {cn} from '@/lib/utils';

/**
 * 侧边栏图标选项卡按钮
 * Sidebar icon tab button
 */
export function SidebarIconTab({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
      <TooltipTrigger asChild>
        <button
          type='button'
          aria-label={label}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border transition-colors',
            active
              ? 'border-primary/40 bg-primary/10 text-primary'
              : 'border-transparent text-muted-foreground hover:bg-muted hover:text-foreground',
          )}
          onClick={onClick}
        >
          {icon}
        </button>
      </TooltipTrigger>
      <TooltipContent side='left'>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * 工作台侧边栏外壳容器。
 * contentMode=scroll：整页滚动（版本等）；fill：子面板自管高度（模板管理、设置页上下两段）。
 * flush：fill 时不再加外壳内边距，由子面板自己分段留白。
 * 收起入口在顶栏两端（#110 AC11），本组件不再放收起按钮。
 * Studio sidebar shell. scroll = page scroll; fill = child owns height (templates, split settings).
 * flush = fill without shell padding so the child can pad each pane.
 * Collapse lives on the topbar ends (#110 AC11); this shell has no collapse control.
 */
export function StudioSidebarShell({
  children,
  rail,
  className,
  contentMode = 'scroll',
  flush = false,
  ariaHidden,
  inert: inertProp,
  'data-studio-right-sidebar': dataStudioRightSidebar,
}: {
  children: ReactNode;
  rail: ReactNode;
  className?: string;
  contentMode?: 'scroll' | 'fill';
  /** fill 时去掉外壳 padding，让设置页上下两段自己滚动 / Drop shell padding in fill mode so the settings panes scroll themselves */
  flush?: boolean;
  ariaHidden?: boolean;
  /** 收起时设 inert，整棵侧栏移出键盘焦点序 / When collapsed, inert removes the sidebar from keyboard focus */
  inert?: boolean;
  'data-studio-right-sidebar'?: string;
}) {
  return (
    <Card
      data-studio-right-sidebar={dataStudioRightSidebar}
      className={cn(
        'studio-sidebar-chrome row-start-1 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm',
        className,
      )}
      aria-hidden={ariaHidden || undefined}
      inert={inertProp ? true : undefined}
    >
      <CardContent className='grid h-full min-h-0 min-w-0 grid-cols-[minmax(0,1fr)_40px] p-0'>
        <div className='min-h-0 min-w-0 overflow-hidden'>
          {contentMode === 'fill' ? (
            <div
              className={cn(
                'flex h-full min-h-0 min-w-0 flex-col overflow-hidden',
                flush ? '' : 'p-3',
              )}
            >
              {children}
            </div>
          ) : (
            <ScrollArea className='h-full'>
              <div className='min-w-0 p-3'>{children}</div>
            </ScrollArea>
          )}
        </div>
        <div className='flex min-h-0 w-10 shrink-0 flex-col items-center gap-2 border-l border-border/60 bg-muted/10 py-3'>
          {rail}
        </div>
      </CardContent>
    </Card>
  );
}
