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
import {cn} from '@/lib/utils';
import {MarketplaceBubbleStack} from './BubbleStack';
import {MarketplaceDistributionBubbles} from './DistributionBubbles';
import type {MarketplaceOverviewModel} from './types';

/**
 * Overview bento: hero metric + optional distribution + small stats.
 * Only renders slices/stats that carry real data from the caller.
 * 概览 bento：主指标 + 可选分布 + 小统计；仅渲染调用方提供的真实数据项。
 */
export function MarketplaceOverviewBento({
  model,
  className,
}: {
  model: MarketplaceOverviewModel;
  className?: string;
}) {
  const reduceMotion = useReducedMotion() ?? false;
  // Hide empty / zero / unready stats — never invent placeholder zeros.
  // 隐藏空值、0、未就绪统计，绝不造占位假数。
  const stats = model.stats.filter((item) => {
    if (item.value === '' || item.value === null || item.value === undefined) {
      return false;
    }
    if (typeof item.value === 'number' && item.value <= 0) {
      return false;
    }
    if (typeof item.value === 'string' && item.value.trim() === '0') {
      return false;
    }
    return true;
  });
  const heroReady =
    model.hero.value !== '' &&
    model.hero.value !== null &&
    model.hero.value !== undefined &&
    !(typeof model.hero.value === 'number' && model.hero.value <= 0) &&
    !(typeof model.hero.value === 'string' && model.hero.value.trim() === '0');
  const distributionSlices = (model.distribution?.slices || []).filter(
    (s) => s.count > 0,
  );
  const hasDistribution = distributionSlices.length > 0;

  if (!heroReady && !hasDistribution && stats.length === 0) {
    return null;
  }

  return (
    <motion.div
      data-testid='marketplace-overview-bento'
      className={cn('grid gap-3 lg:grid-cols-12', className)}
      initial={reduceMotion ? false : {opacity: 0, y: 8}}
      animate={{opacity: 1, y: 0}}
      transition={reduceMotion ? {duration: 0.12} : {duration: 0.2}}
    >
      {heroReady ? (
        <div className='rounded-xl border border-border/60 bg-card/60 p-5 shadow-xs lg:col-span-5'>
          <div className='flex items-start justify-between gap-3'>
            <div className='min-w-0'>
              <div className='text-sm text-muted-foreground'>{model.hero.label}</div>
              <div className='mt-2 text-4xl font-semibold tracking-tight tabular-nums'>
                {model.hero.value}
              </div>
              {model.hero.caption ? (
                <p className='mt-2 text-xs text-muted-foreground'>
                  {model.hero.caption}
                </p>
              ) : null}
            </div>
            {model.hero.bubbles && model.hero.bubbles.length > 0 ? (
              <MarketplaceBubbleStack members={model.hero.bubbles} />
            ) : null}
          </div>
        </div>
      ) : null}

      {hasDistribution && model.distribution ? (
        <div className={cn(heroReady ? 'lg:col-span-4' : 'lg:col-span-5')}>
          <MarketplaceDistributionBubbles
            title={model.distribution.title}
            slices={distributionSlices}
            footer={model.distribution.footer}
            className='min-h-[140px]'
          />
        </div>
      ) : null}

      {stats.length > 0 ? (
        <div
          className={cn(
            'grid gap-3',
            hasDistribution && heroReady
              ? 'lg:col-span-3'
              : heroReady || hasDistribution
                ? 'lg:col-span-7'
                : 'lg:col-span-12',
            stats.length > 1 ? 'sm:grid-cols-2 lg:grid-cols-1' : '',
          )}
        >
          {stats.slice(0, 2).map((stat) => (
            <div
              key={stat.id}
              data-testid={`marketplace-stat-${stat.id}`}
              className='rounded-xl border border-border/60 bg-card/60 p-4 shadow-xs'
            >
              <div className='text-xs text-muted-foreground'>{stat.label}</div>
              <div className='mt-1.5 text-2xl font-semibold tabular-nums'>
                {stat.value}
              </div>
              {stat.hint ? (
                <p className='mt-1 text-[11px] text-muted-foreground'>{stat.hint}</p>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}
    </motion.div>
  );
}
