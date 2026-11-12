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

import {motion, useReducedMotion} from 'motion/react';
import {useTranslations} from 'next-intl';
import {cn} from '@/lib/utils';
import type {MarketplaceDistributionSlice} from './types';

/**
 * Size-coded distribution bubbles (count drives diameter).
 * 按数量映射直径的分布气泡。
 */
export function MarketplaceDistributionBubbles({
  title,
  slices,
  footer,
  className,
}: {
  title: string;
  slices: MarketplaceDistributionSlice[];
  footer?: string;
  className?: string;
}) {
  const t = useTranslations('marketplaceUi');
  const reduceMotion = useReducedMotion() ?? false;
  // Drop zero / empty slices so overview never shows fake zeros.
  // 丢掉 0/空切片，总览不展示假零。
  const liveSlices = slices.filter((slice) => slice.count > 0);
  const total = liveSlices.reduce((sum, slice) => sum + slice.count, 0);
  if (total <= 0 || liveSlices.length === 0) {
    return null;
  }

  const max = Math.max(...liveSlices.map((s) => s.count), 1);

  return (
    <div
      className={cn(
        'flex h-full flex-col rounded-xl border border-border/60 bg-card/60 p-4 shadow-xs',
        className,
      )}
      data-testid='marketplace-distribution-card'
    >
      <div className='text-sm font-medium text-muted-foreground'>{title}</div>
      <div className='mt-3 flex flex-1 flex-wrap items-end gap-3'>
        {liveSlices.map((slice, index) => {
          const ratio = slice.count / max;
          const size = 36 + Math.round(ratio * 44);
          const pct = Math.round((slice.count / total) * 100);
          return (
            <motion.button
              key={slice.id}
              type='button'
              className='inline-flex flex-col items-center gap-1 rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
              initial={reduceMotion ? false : {opacity: 0, scale: 0.9}}
              animate={{opacity: 1, scale: 1}}
              transition={
                reduceMotion
                  ? {duration: 0.01}
                  : {delay: index * 0.04, duration: 0.2}
              }
              whileHover={
                reduceMotion
                  ? undefined
                  : {scale: 1.06, transition: {duration: 0.16}}
              }
              aria-label={t('distributionSliceAria', {
                label: slice.label,
                count: slice.count,
                percent: pct,
              })}
            >
              <span
                className={cn(
                  'inline-flex items-center justify-center rounded-full text-xs font-semibold text-foreground shadow-sm ring-1 ring-border/60',
                  slice.toneClass,
                )}
                style={{width: size, height: size}}
              >
                {slice.count}
              </span>
              <span className='max-w-[4.5rem] truncate text-[11px] text-muted-foreground'>
                {slice.label}
              </span>
            </motion.button>
          );
        })}
      </div>
      {footer ? (
        <p className='mt-3 text-xs text-muted-foreground'>{footer}</p>
      ) : null}
    </div>
  );
}
