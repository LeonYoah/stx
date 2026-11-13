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
import {
  PACKAGE_APPLE_CARD_HEIGHT_CLASS,
  PACKAGE_APPLE_CARD_WIDTH_CLASS,
} from '@/components/common/marketplace/marketplace-card-size';
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
            className={cn(
              'shrink-0 rounded-3xl',
              PACKAGE_APPLE_CARD_WIDTH_CLASS,
              PACKAGE_APPLE_CARD_HEIGHT_CLASS,
            )}
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
      prevAriaLabel={t('packageCarouselPrev')}
      nextAriaLabel={t('packageCarouselNext')}
      data-testid={dataTestId}
    >
      {items.map((item, index) => {
        const showMenu = hasMarketplaceMenuActions(item.menuActions);
        return (
          <AppleCarouselItem key={item.id} index={index}>
            <article
              data-testid={`package-carousel-card-${item.id}`}
              // 统一高度；overflow-visible 避免底栏按钮边框与 focus 环被裁。
              // Unified height; overflow-visible so footer button borders/focus rings stay intact.
              className={cn(
                'relative flex flex-col overflow-visible rounded-3xl border border-border/60 bg-card shadow-xs',
                PACKAGE_APPLE_CARD_WIDTH_CLASS,
                PACKAGE_APPLE_CARD_HEIGHT_CLASS,
              )}
            >
              {/* 渐变单独裁切圆角，不牵连底栏交互层。
                  Gradient clips to radius alone; does not clip the footer actions. */}
              <div
                aria-hidden='true'
                className='pointer-events-none absolute inset-0 z-10 overflow-hidden rounded-[inherit]'
              >
                <div className='absolute inset-x-0 top-0 h-32 bg-gradient-to-b from-primary/30 via-primary/5 to-transparent' />
              </div>
              <div className='relative z-20 flex min-h-0 flex-1 flex-col p-6 pb-8 md:p-7 md:pb-9'>
                <div className='flex items-start gap-3.5'>
                  {item.icon ? (
                    <div className='flex size-12 shrink-0 items-center justify-center rounded-2xl bg-background/80 ring-1 ring-border/50 md:size-14'>
                      {item.icon}
                    </div>
                  ) : null}
                  <div className='min-w-0 pt-0.5'>
                    <h3 className='truncate text-xl font-semibold tracking-tight text-foreground md:text-2xl'>
                      {item.title}
                    </h3>
                    {item.subtitle ? (
                      <p className='mt-1 truncate text-sm text-muted-foreground'>
                        {item.subtitle}
                      </p>
                    ) : null}
                  </div>
                </div>

                <div className='mt-5 flex flex-wrap gap-2'>
                  {(item.badges || []).map((badge) => (
                    <span
                      key={`${item.id}-${badge.label}`}
                      className={cn(
                        'rounded-full border border-border/60 px-2.5 py-1 text-xs',
                        badge.highlight &&
                          'border-primary/30 bg-primary/15 text-primary',
                      )}
                    >
                      {badge.label}
                    </span>
                  ))}
                </div>

                {item.status ? (
                  <div className='mt-4'>
                    <MarketplaceStatusDot
                      label={item.status.label}
                      tone={item.status.tone}
                    />
                  </div>
                ) : null}

                {item.progress ? (
                  <div className='mt-3 text-sm text-muted-foreground'>
                    {t('progressAria', {
                      label: item.progress.label,
                      display: item.progress.display,
                    })}
                  </div>
                ) : null}

                <div className='mt-4 flex min-h-0 flex-1 flex-col rounded-2xl bg-muted/40 p-3.5 md:p-4'>
                  {item.description ? (
                    <p className='mb-2.5 line-clamp-2 text-sm leading-relaxed text-muted-foreground'>
                      {item.description}
                    </p>
                  ) : null}
                  {item.fields && item.fields.length > 0 ? (
                    <div className='space-y-2 text-sm'>
                      {item.fields.slice(0, 6).map((field) => (
                        <div
                          key={`${item.id}-${field.label}`}
                          className='flex min-w-0 justify-between gap-3'
                        >
                          <span className='shrink-0 text-muted-foreground'>
                            {field.label}
                          </span>
                          <span
                            className='truncate font-medium text-foreground'
                            title={field.value}
                          >
                            {field.value}
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className='text-sm text-muted-foreground'>
                      {item.meta || item.subtitle || item.title}
                    </p>
                  )}
                </div>

                <div className='mt-auto flex min-h-9 shrink-0 items-center justify-end gap-1.5 border-t border-border/40 pt-4'>
                  {/* py-0.5：下载按钮边框/焦点环不被卡片底部裁切。
                      py-0.5 keeps download borders/focus rings clear of the card bottom. */}
                  <div className='flex min-h-9 items-center justify-end gap-1.5 py-0.5'>
                    {item.primaryActions}
                    {showMenu ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            className='size-9 rounded-md'
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
              </div>
            </article>
          </AppleCarouselItem>
        );
      })}
    </AppleCardCarousel>
  );
}
