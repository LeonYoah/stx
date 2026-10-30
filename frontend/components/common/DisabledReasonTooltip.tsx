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

import type {ReactElement} from 'react';

import {HELP_HINT_DELAY_MS} from '@/components/common/HelpHint';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';

interface DisabledReasonTooltipProps {
  /** 不可用原因；为空时原样渲染子元素 / Why the control is unavailable; renders children as-is when empty */
  reason?: string | null;
  /**
   * 子元素需自行设置 aria-disabled 并拦截点击，保持可聚焦以便键盘打开 Tooltip。
   * The child must set aria-disabled and swallow clicks itself, staying focusable for keyboard users.
   */
  children: ReactElement;
}

/**
 * 给「不可用」按钮挂上原因 Tooltip / Attach a reason tooltip to an unavailable control.
 */
export function DisabledReasonTooltip({
  reason,
  children,
}: DisabledReasonTooltipProps) {
  if (!reason) {
    return children;
  }
  return (
    <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent className='max-w-xs text-xs leading-relaxed'>
        {reason}
      </TooltipContent>
    </Tooltip>
  );
}

/** aria-disabled 按钮的通用样式 / Shared look for aria-disabled buttons */
export const ARIA_DISABLED_CLASS = 'cursor-not-allowed opacity-50';
