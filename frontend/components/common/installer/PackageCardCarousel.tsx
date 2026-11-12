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

import type {ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {MoreHorizontal} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Skeleton} from '@/components/ui/skeleton';
import {
  AppleCardCarousel,
  AppleCarouselItem,
} from '@/components/inspira/ui/apple-card-carousel';
import {MarketplaceStatusDot} from '@/components/common/marketplace';
import {hasMarketplaceMenuActions} from '@/components/common/marketplace/MorphCatalog';
import type {MarketplaceCatalogItem} from '@/components/common/marketplace';
import {cn} from '@/lib/utils';

/**
 * Apple-style package carousel for card view (Inspira layout, STX package data).
 * 安装包卡片视图的 Apple 风格轮播（Inspira 布局 + STX 安装包数据）。
 *
 * Blank card surface does not open detail — only explicit actions.
 * 卡片空白不打开详情，只保留显式操作。
 */
export function PackageCardCarousel({
  items,
  loading = false,
  error = null,
  empty,
  'data-testid': dataTestId = 'package-card-carousel',
}: {
  items: MarketplaceCatalogItem[];
  loading?: boolean;
  error?: ReactNode | null;
  empty?: ReactNode;
  'data-testid'?: string;
}) {
  const t = useTranslations('marketplaceUi');

  if (error) {
    return (
      <div
        data-testid={`${dataTestId}-error`}
        role='alert'
        className='rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-8 text-center text-sm text-destructive'
      >
        {error}
      </div>
    );
  }

  if (loading && items.length === 0) {
    return (
      <div
        data-testid={`${dataTestId}-loading`}
        className='flex gap-3 overflow-hidden py-4'
      >
        {Array.from({length: 3}).map((_, i) => (
          <Skeleton
            key={i}
            className='h-72 w-56 shrink-0 rounded-3xl md:h-80 md:w-72'
          />
        ))}
      </div>
    );
  }

  if (!loading && items.length === 0) {
    return (
      <div
        data-testid={`${dataTestId}-empty`}
        className='flex min-h-[180px] items-center justify-center rounded-xl border border-dashed border-border/60 px-4 py-10 text-sm text-muted-foreground'
      >
        {empty ?? t('emptyList')}
      </div>
    );
  }

  return (
    <AppleCardCarousel
      aria-label={t('packageCarouselAria')}
      data-testid={dataTestId}
    >
      {items.map((item, index) => {
        const showMenu = hasMarketplaceMenuActions(item.menuActions);
        return (
          <AppleCarouselItem key={item.id} index={index}>
            <article
              data-testid={`package-carousel-card-${item.id}`}
              // Inspira AppleCard: rounded-3xl fixed tile; STX fills with package meta + actions.
              // Inspira 为圆角大卡；STX 填入安装包元信息与显式操作。
              className={cn(
                'relative flex h-72 w-56 flex-col overflow-hidden rounded-3xl border border-border/60 bg-card shadow-xs md:h-80 md:w-72',
              )}
            >
              <div className='pointer-events-none absolute inset-x-0 top-0 z-10 h-24 bg-gradient-to-b from-primary/25 via-transparent to-transparent' />
              <div className='relative z-20 flex flex-1 flex-col p-5'>
                <div className='flex items-start gap-3'>
                  {item.icon ? (
                    <div className='flex size-10 shrink-0 items-center justify-center rounded-2xl bg-background/80 ring-1 ring-border/50'>
                      {item.icon}
                    </div>
                  ) : null}
                  <div className='min-w-0'>
                    <h3 className='truncate text-lg font-semibold tracking-tight text-foreground'>
                      {item.title}
                    </h3>
                    {item.subtitle ? (
                      <p className='mt-0.5 truncate text-xs text-muted-foreground'>
                        {item.subtitle}
                      </p>
                    ) : null}
                  </div>
                </div>

                <div className='mt-4 flex flex-wrap gap-1.5'>
                  {(item.badges || []).map((badge) => (
                    <span
                      key={`${item.id}-${badge.label}`}
                      className={cn(
                        'rounded-full border border-border/60 px-2 py-0.5 text-[11px]',
                        badge.highlight &&
                          'border-primary/30 bg-primary/15 text-primary',
                      )}
                    >
                      {badge.label}
                    </span>
                  ))}
                </div>

                {item.status ? (
                  <div className='mt-3'>
                    <MarketplaceStatusDot
                      label={item.status.label}
                      tone={item.status.tone}
                    />
                  </div>
                ) : null}

                {item.progress ? (
                  <div className='mt-3 text-[11px] text-muted-foreground'>
                    {item.progress.label}: {item.progress.display}
                  </div>
                ) : null}

                {item.fields && item.fields.length > 0 ? (
                  <div className='mt-3 space-y-1.5 text-xs'>
                    {item.fields.slice(0, 3).map((field) => (
                      <div
                        key={`${item.id}-${field.label}`}
                        className='flex min-w-0 justify-between gap-2'
                      >
                        <span className='shrink-0 text-muted-foreground'>
                          {field.label}
                        </span>
                        <span className='truncate font-medium text-foreground'>
                          {field.value}
                        </span>
                      </div>
                    ))}
                  </div>
                ) : null}

                <div className='mt-auto flex items-center justify-end gap-1 border-t border-border/40 pt-3'>
                  {item.primaryActions}
                  {showMenu ? (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          className='size-8 rounded-md'
                          aria-label={t('moreActions')}
                          data-testid={`package-carousel-more-${item.id}`}
                        >
                          <MoreHorizontal className='size-4' />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='end' className='min-w-[10rem]'>
                        {item.menuActions}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  ) : null}
                </div>
              </div>
            </article>
          </AppleCarouselItem>
        );
      })}
    </AppleCardCarousel>
  );
}
