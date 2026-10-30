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

import {useState, type ReactElement} from 'react';

import {HELP_HINT_DELAY_MS} from '@/components/common/HelpHint';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';

interface DisabledReasonTooltipProps {
  /** 不可用原因；为空时不弹 Tooltip / Why the control is unavailable; no tooltip when empty */
  reason?: string | null;
  /**
   * 子元素需自行设置 aria-disabled 并拦截点击，保持可聚焦以便键盘打开 Tooltip。
   * The child must set aria-disabled and swallow clicks itself, staying focusable for keyboard users.
   */
  children: ReactElement;
}

/**
 * 给「不可用」按钮挂上原因 Tooltip / Attach a reason tooltip to an unavailable control.
 *
 * 无论有没有原因都渲染同一棵 Tooltip 树，原因出现或消失时子按钮不会重新挂载，键盘焦点不丢。
 * The Tooltip tree is rendered whether or not there is a reason, so the child is never
 * remounted when the reason appears or clears and keyboard focus is kept.
 */
export function DisabledReasonTooltip({
  reason,
  children,
}: DisabledReasonTooltipProps) {
  // 始终受控，没有原因时保持关闭 / Always controlled; stays closed without a reason
  const [open, setOpen] = useState(false);
  return (
    <Tooltip
      delayDuration={HELP_HINT_DELAY_MS}
      open={Boolean(reason) && open}
      onOpenChange={setOpen}
    >
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      {reason ? (
        <TooltipContent className='max-w-xs text-xs leading-relaxed'>
          {reason}
        </TooltipContent>
      ) : null}
    </Tooltip>
  );
}
