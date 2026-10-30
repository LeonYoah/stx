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
import {useTranslations} from 'next-intl';

import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';

/** 悬停延迟，避免 delayDuration=0 时鼠标划过就弹出 / Hover delay for hint tooltips */
export const HELP_HINT_DELAY_MS = 300;

interface HelpHintProps {
  /** 提示正文，经 Tooltip 的 aria-describedby 朗读 / Hint body, announced via the tooltip's aria-describedby */
  content: string;
  /**
   * 所说明对象的短名（通常是字段或标题文字），按钮名称为「{label} 说明」；缺省为「说明」。
   * Short name of the described field or heading; the button is named "{label} help", or "Help" when omitted.
   */
  label?: string;
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
  label,
  className,
  contentClassName,
  side,
  'data-testid': testId,
}: HelpHintProps) {
  const t = useTranslations('common');
  // 名称只放短名，正文交给 Tooltip 的 describedby，避免读屏重复朗读或污染标题的可访问名称
  // Keep the accessible name short; the body is exposed through the tooltip's describedby.
  const accessibleName = label ? t('helpLabelFor', {label}) : t('helpLabel');

  return (
    <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
      <TooltipTrigger asChild>
        <button
          type='button'
          aria-label={accessibleName}
          data-testid={testId}
          className={cn(
            '-m-1 inline-flex shrink-0 items-center rounded-sm p-1 text-muted-foreground/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background',
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
