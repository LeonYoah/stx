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

import {HelpCircle} from 'lucide-react';

import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';

/** 悬停延迟，避免 delayDuration=0 时鼠标划过就弹出 / Hover delay for hint tooltips */
export const HELP_HINT_DELAY_MS = 300;

interface HelpHintProps {
  /** 提示内容，同时作为按钮的无障碍名称 / Hint text, also used as the accessible name */
  content: string;
  className?: string;
  contentClassName?: string;
  side?: 'top' | 'right' | 'bottom' | 'left';
  'data-testid'?: string;
}

/**
 * 字段 / 标题旁的问号提示：图标包在 button 里，保证键盘可聚焦并打开 Tooltip。
 * Help icon wrapped in a real button so the tooltip can be opened from the keyboard.
 */
export function HelpHint({
  content,
  className,
  contentClassName,
  side,
  'data-testid': testId,
}: HelpHintProps) {
  return (
    <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
      <TooltipTrigger asChild>
        <button
          type='button'
          aria-label={content}
          data-testid={testId}
          className={cn(
            'inline-flex shrink-0 items-center rounded-sm text-muted-foreground/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            className,
          )}
        >
          <HelpCircle className='size-3.5' />
        </button>
      </TooltipTrigger>
      <TooltipContent
        side={side}
        className={cn('max-w-xs text-xs leading-relaxed', contentClassName)}
      >
        {content}
      </TooltipContent>
    </Tooltip>
  );
}
