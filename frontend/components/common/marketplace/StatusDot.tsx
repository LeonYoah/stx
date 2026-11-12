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

import {cn} from '@/lib/utils';
import type {MarketplaceStatusTone} from './types';

const TONE_DOT: Record<MarketplaceStatusTone, string> = {
  ok: 'bg-emerald-500',
  warn: 'bg-amber-500',
  error: 'bg-destructive',
  info: 'bg-sky-500',
  muted: 'bg-muted-foreground/50',
};

/**
 * Compact status indicator: colored dot + label.
 * 紧凑状态指示：色点 + 文案。
 */
export function MarketplaceStatusDot({
  label,
  tone,
  className,
}: {
  label: string;
  tone: MarketplaceStatusTone;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap',
        className,
      )}
    >
      <span
        aria-hidden='true'
        className={cn('size-1.5 shrink-0 rounded-full', TONE_DOT[tone])}
      />
      <span className='text-foreground/90'>{label}</span>
    </span>
  );
}
