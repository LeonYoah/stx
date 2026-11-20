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

// 状态 / 级别徽章：沿用旧告警中心（shadcn 版本）的软色徽章设计，用户明确拍板保留。
// Severity / status chips: keep the original shadcn-era soft-tint badge design
// from the legacy alerts center — explicitly requested to stay unchanged.
import {AlertCircle, AlertTriangle, Archive, BadgeCheck, CheckCircle2, Info, VolumeX} from 'lucide-react';
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

/** 级别徽章：critical 玫瑰 / warning 琥珀 / 其他蓝色，图标 3.5px（旧版 renderSeverityBadge）。 / Severity chip — legacy renderSeverityBadge look. */
export function SeverityChip({severity, label, className}: {severity: string; label: ReactNode; className?: string}) {
  const normalized = severity.trim().toLowerCase();
  const icon = 'h-3.5 w-3.5';
  if (normalized === 'critical') {
    return <SoftChip tone='rose' className={className}><AlertCircle className={icon} aria-hidden />{label}</SoftChip>;
  }
  if (normalized === 'warning') {
    return <SoftChip tone='amber' className={className}><AlertTriangle className={icon} aria-hidden />{label}</SoftChip>;
  }
  return <SoftChip tone='blue' className={className}><Info className={icon} aria-hidden />{label}</SoftChip>;
}

export type AlertStatusChipStatus = 'firing' | 'acknowledged' | 'silenced' | 'resolved' | 'closed' | string;

/** 告警状态徽章：rounded-full 软色，firing 带呼吸点（旧版 renderStatusBadge）。 / Alert status chip — legacy renderStatusBadge look, pulsing dot for firing. */
export function AlertStatusChip({status, label, className}: {status: AlertStatusChipStatus; label: ReactNode; className?: string}) {
  const icon = 'h-3.5 w-3.5';
  switch (status) {
    case 'firing':
      return (
        <SoftChip tone='rose' className={className}>
          <span className='relative flex h-2 w-2'>
            <span className='motion-safe:animate-ping absolute inline-flex h-full w-full rounded-full bg-rose-400 opacity-75' />
            <span className='relative inline-flex h-2 w-2 rounded-full bg-rose-500' />
          </span>
          {label}
        </SoftChip>
      );
    case 'acknowledged':
      return <SoftChip tone='blue' className={className}><BadgeCheck className={icon} aria-hidden />{label}</SoftChip>;
    case 'silenced':
      return <SoftChip tone='amber' className={className}><VolumeX className={icon} aria-hidden />{label}</SoftChip>;
    case 'resolved':
      return <SoftChip tone='emerald' className={className}><CheckCircle2 className={icon} aria-hidden />{label}</SoftChip>;
    default:
      return <SoftChip tone='muted' className={className}><Archive className={icon} aria-hidden />{label}</SoftChip>;
  }
}

/** 投递状态徽章：sent 翠绿 / failed 玫瑰 / 其他灰（同一软色家族）。 / Delivery status chip in the same soft-tint family. */
export function DeliveryChip({status, label, className}: {status: string; label: ReactNode; className?: string}) {
  const icon = 'h-3.5 w-3.5';
  if (status === 'sent') {
    return <SoftChip tone='emerald' className={className}><CheckCircle2 className={icon} aria-hidden />{label}</SoftChip>;
  }
  if (status === 'failed') {
    return <SoftChip tone='rose' className={className}><AlertCircle className={icon} aria-hidden />{label}</SoftChip>;
  }
  return <SoftChip tone='muted' className={className}>{label}</SoftChip>;
}

/** 中性小标签（范围、渠道类型、字段名等）：沿用旧版 ui/badge outline。 / Neutral small label — legacy ui/badge outline. */
export function NeutralChip({children, className, title}: {children: ReactNode; className?: string; title?: string}) {
  return (
    <Badge variant='outline' title={title} className={cn('shrink-0 px-1.5 py-0 text-[11px] font-normal', className)}>
      {children}
    </Badge>
  );
}
