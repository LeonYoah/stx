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
import {PanelRightClose} from 'lucide-react';
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
            'flex h-9 w-9 items-center justify-center rounded-md border transition-colors',
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
 * contentMode=scroll：整页滚动（设置/版本等）；fill：子面板自管高度（模板管理列表吃满剩余空间）。
 * 可选 onCollapse：在 icon rail 顶部放简单收起图标（#110 右侧高级配置）。
 * Studio sidebar shell. scroll = page scroll; fill = child owns height (manage list fills leftover).
 * Optional onCollapse: simple collapse icon at the top of the icon rail (#110 right advanced sidebar).
 */
type StudioSidebarShellProps = {
  children: ReactNode;
  rail: ReactNode;
  className?: string;
  contentMode?: 'scroll' | 'fill';
  ariaHidden?: boolean;
} & (
  | {
      /** 提供收起回调时必须同时给 i18n 文案，禁止英文硬编码兜底 / collapseLabel is required with onCollapse */
      onCollapse: () => void;
      collapseLabel: string;
    }
  | {
      onCollapse?: undefined;
      collapseLabel?: undefined;
    }
);

export function StudioSidebarShell({
  children,
  rail,
  className,
  contentMode = 'scroll',
  onCollapse,
  collapseLabel,
  ariaHidden,
}: StudioSidebarShellProps) {
  return (
    <Card
      className={cn(
        'row-start-1 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm',
        className,
      )}
      aria-hidden={ariaHidden || undefined}
    >
      <CardContent className='grid h-full min-h-0 min-w-0 grid-cols-[minmax(0,1fr)_40px] p-0'>
        <div className='min-h-0 min-w-0 overflow-hidden'>
          {contentMode === 'fill' ? (
            <div className='flex h-full min-h-0 min-w-0 flex-col overflow-hidden p-3'>
              {children}
            </div>
          ) : (
            <ScrollArea className='h-full'>
              <div className='min-w-0 p-3'>{children}</div>
            </ScrollArea>
          )}
        </div>
        <div className='flex min-h-0 w-10 shrink-0 flex-col items-center gap-2 border-l border-border/60 bg-muted/10 py-3'>
          {onCollapse ? (
            <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
              <TooltipTrigger asChild>
                <button
                  type='button'
                  aria-label={collapseLabel}
                  data-testid='studio-sidebar-shell-collapse'
                  className='flex h-9 w-9 items-center justify-center rounded-md border border-transparent text-muted-foreground transition-colors hover:bg-muted hover:text-foreground'
                  onClick={onCollapse}
                >
                  <PanelRightClose className='size-4' />
                </button>
              </TooltipTrigger>
              <TooltipContent side='left'>{collapseLabel}</TooltipContent>
            </Tooltip>
          ) : null}
          {rail}
        </div>
      </CardContent>
    </Card>
  );
}
