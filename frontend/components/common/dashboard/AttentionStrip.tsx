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

import Link from 'next/link';
import {useFormatter, useTranslations} from 'next-intl';
import {AlertTriangle, CircleAlert} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import type {HomeAttentionItem} from '@/lib/services/dashboard';
import {cn} from '@/lib/utils';

const MAX_ITEMS = 10;

/**
 * 「需要处理」横条：合并作业失败、CDC 延迟、高背压与告警，严重在前；单行横向滚动，不撑高首屏。
 * 无权查看的作业只展示，不提供跳转。
 * The "needs attention" strip: job failures, CDC delay, high backpressure and alerts, critical first; one horizontally
 * scrolling row that never grows the first viewport. Jobs the user cannot view are display-only.
 */
export function AttentionStrip({
  items,
  jobHref,
}: {
  items: HomeAttentionItem[];
  jobHref: (item: HomeAttentionItem) => string;
}) {
  const t = useTranslations('dashboard.home.attention');
  const format = useFormatter();
  const visible = items.slice(0, MAX_ITEMS);

  // 无事项时退成无框单行文字：空横条留着边框会像一块没收起来的区域。
  // With nothing pending the strip falls back to a borderless one-liner — a
  // framed empty bar reads like a section that failed to collapse.
  if (visible.length === 0) {
    return (
      <p
        data-testid='home-attention'
        className='px-1 text-xs text-muted-foreground'
      >
        {t('title')} · {t('empty')}
      </p>
    );
  }

  return (
    <div
      className='flex min-h-11 items-center gap-2 overflow-x-auto rounded-xl border border-border/70 bg-card/40 px-3 py-1.5'
      data-testid='home-attention'
    >
      <span className='shrink-0 pr-1 text-xs text-muted-foreground'>
        {t('title')}
      </span>
      {visible.map((item, index) => {
        const Icon = item.kind === 'job_failed' ? CircleAlert : AlertTriangle;
        const detail =
          item.kind === 'cdc_delay' && item.value !== undefined
            ? `${format.number(item.value, {maximumFractionDigits: 1})}s`
            : item.kind === 'backpressure' && item.value !== undefined
              ? format.number(item.value, {
                  style: 'percent',
                  maximumFractionDigits: 0,
                })
              : item.detail;
        const body = (
          <>
            <Icon
              className={cn(
                'size-3.5 shrink-0',
                item.severity === 'critical'
                  ? 'text-destructive'
                  : 'text-amber-500',
              )}
              aria-hidden='true'
            />
            <span className='text-muted-foreground'>
              {t(`kind.${item.kind}`)}
            </span>
            <span className='max-w-[16rem] truncate font-medium text-foreground'>
              {item.title}
            </span>
            {detail ? (
              <span className='max-w-[14rem] truncate text-muted-foreground'>
                {detail}
              </span>
            ) : null}
          </>
        );
        const chipClass =
          'inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md border border-border/60 bg-background/60 px-2.5 text-xs';
        const key = `${item.kind}-${item.job_id ?? item.alert_id ?? index}`;
        if (item.kind !== 'alert' && !item.viewable) {
          return (
            <Tooltip key={key}>
              <TooltipTrigger asChild>
                <span className={cn(chipClass, 'cursor-default opacity-80')}>
                  {body}
                </span>
              </TooltipTrigger>
              <TooltipContent>{t('noAccess')}</TooltipContent>
            </Tooltip>
          );
        }
        return (
          <Link
            key={key}
            href={item.kind === 'alert' ? '/monitoring' : jobHref(item)}
            className={cn(
              chipClass,
              'transition-colors hover:border-border hover:bg-muted/40',
            )}
          >
            {body}
          </Link>
        );
      })}
    </div>
  );
}
