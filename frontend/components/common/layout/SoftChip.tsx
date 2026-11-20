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
// monitoring/status-chips.tsx 提升；告警专属徽章仍留在原文件。
// Generic soft-tint chip base + neutral label shared across modules
// (alerts / diagnostics list rows), promoted from monitoring/status-chips.tsx;
// alert-specific chips stay in that file.
import type {ReactNode} from 'react';
import {Badge} from '@/components/ui/badge';
import {cn} from '@/lib/utils';

const tones = {
  rose: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20',
  amber: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20',
  blue: 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20',
  emerald: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20',
  muted: 'bg-muted text-muted-foreground border-border',
} as const;

export type ChipTone = keyof typeof tones;

/** 软色徽章基座：圆角统一收敛为 rounded-md，与 ui/badge 和 8px 控件同族。
    Soft-tint chip base: radius converged to rounded-md, same family as ui/badge and the 8px controls. */
export function SoftChip({tone, className, children}: {tone: ChipTone; className?: string; children: ReactNode}) {
  return (
    <span className={cn('inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-semibold', tones[tone], className)}>
      {children}
    </span>
  );
}

/** 中性小标签（范围、渠道类型、字段名等）：沿用旧版 ui/badge outline。 / Neutral small label — legacy ui/badge outline. */
export function NeutralChip({children, className, title}: {children: ReactNode; className?: string; title?: string}) {
  return (
    <Badge variant='outline' title={title} className={cn('shrink-0 px-1.5 py-0 text-[11px] font-normal', className)}>
      {children}
    </Badge>
  );
}
