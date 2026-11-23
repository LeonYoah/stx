/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

'use client';

// 软色徽章基座 + 中性标签：跨模块通用（告警 / 诊断列表行），从
// monitoring/status-chips.tsx 提升；内置零静止开销溢出提示，替代原生 title。
// Generic soft-tint chip base + neutral label shared across modules
// (alerts / diagnostics list rows), promoted from monitoring/status-chips.tsx;
// embeds zero-rest-overhead overflow tooltip instead of slow native title.
import {useRef, type ReactNode} from 'react';
import {Badge} from '@/components/ui/badge';
import {cn} from '@/lib/utils';
import {
  useOverflowTooltip,
  type OverflowTooltipSide,
} from './OverflowTooltip';

const tones = {
  rose: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20',
  amber: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20',
  blue: 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20',
  emerald: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20',
  muted: 'bg-muted text-muted-foreground border-border',
} as const;

export type ChipTone = keyof typeof tones;

export interface SoftChipProps {
  tone: ChipTone;
  className?: string;
  children: ReactNode;
  /** 允许在窄列内收缩省略并在溢出时弹出完整原文提示 / Shrink & ellipsize in narrow columns with overflow tooltip */
  truncate?: boolean;
  /** 自定义完整提示内容（兼容历史 title 传参，不再落到原生 HTML title）/ Full tooltip text (accepts legacy title prop without native HTML title) */
  title?: string;
  tooltip?: ReactNode;
  alwaysTooltip?: boolean;
  side?: OverflowTooltipSide;
}

function resolveChipTooltip(
  tooltip: ReactNode | undefined,
  title: string | undefined,
  children: ReactNode,
): ReactNode {
  if (tooltip !== undefined) {
    return tooltip;
  }
  if (title !== undefined) {
    return title;
  }
  if (typeof children === 'string' || typeof children === 'number') {
    return String(children);
  }
  return null;
}

/** 软色徽章基座：圆角统一收敛为 rounded-md，与 ui/badge 和 8px 控件同族。
    Soft-tint chip base: radius converged to rounded-md, same family as ui/badge and the 8px controls. */
export function SoftChip({
  tone,
  className,
  children,
  truncate = false,
  title,
  tooltip,
  alwaysTooltip = false,
  side = 'top',
}: SoftChipProps) {
  const chipRef = useRef<HTMLSpanElement>(null);
  const textRef = useRef<HTMLSpanElement>(null);
  const enableTooltip =
    truncate || title !== undefined || tooltip !== undefined || alwaysTooltip;
  const resolvedTooltip = enableTooltip
    ? resolveChipTooltip(tooltip, title, children)
    : null;

  const {triggerProps, portal} = useOverflowTooltip({
    targetRef: truncate ? textRef : chipRef,
    anchorRef: chipRef,
    content: resolvedTooltip,
    alwaysShow: alwaysTooltip,
    side,
    disabled: !enableTooltip,
  });

  return (
    <>
      <span
        ref={chipRef}
        {...triggerProps}
        className={cn(
          'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-semibold',
          truncate ? 'shrink min-w-0 max-w-full' : 'shrink-0',
          tones[tone],
          className,
        )}
      >
        {truncate ? (
          <span ref={textRef} className='min-w-0 truncate'>
            {children}
          </span>
        ) : (
          children
        )}
      </span>
      {portal}
    </>
  );
}

export interface NeutralChipProps {
  children: ReactNode;
  className?: string;
  /** 兼容历史 title 传参：转为按需主题化溢出提示，不再使用慢速原生 title / Legacy title prop routed to lazy themed overflow tooltip */
  title?: string;
  tooltip?: ReactNode;
  alwaysTooltip?: boolean;
  side?: OverflowTooltipSide;
}

/** 中性小标签（范围、渠道类型、字段名、异常类等）：窄列自动省略并在溢出时悬停展示完整原文。
    Neutral small label (scope, channel type, exception class, etc.): ellipsizes in narrow tracks with lazy overflow tooltip. */
export function NeutralChip({
  children,
  className,
  title,
  tooltip,
  alwaysTooltip = false,
  side = 'top',
}: NeutralChipProps) {
  const badgeRef = useRef<HTMLSpanElement>(null);
  const textRef = useRef<HTMLSpanElement>(null);
  const resolvedTooltip = resolveChipTooltip(tooltip, title, children);

  const {triggerProps, portal} = useOverflowTooltip({
    targetRef: textRef,
    anchorRef: badgeRef,
    content: resolvedTooltip,
    alwaysShow: alwaysTooltip,
    side,
  });

  return (
    <>
      {/* Badge 默认 w-fit + nowrap：栅格列比文本窄时必须允许收缩省略，
          否则整个 pill 溢出盖住下一列。
          Badge defaults to w-fit + nowrap: inside a grid track narrower than
          the text it must shrink and ellipsize, or the pill overflows the next column. */}
      <Badge
        ref={badgeRef}
        variant='outline'
        {...triggerProps}
        className={cn(
          'min-w-0 max-w-full shrink-0 px-1.5 py-0 text-[11px] font-normal',
          className,
        )}
      >
        <span ref={textRef} className='min-w-0 truncate'>
          {children}
        </span>
      </Badge>
      {portal}
    </>
  );
}
