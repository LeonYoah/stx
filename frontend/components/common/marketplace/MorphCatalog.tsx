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
import {LayoutGroup, motion, useReducedMotion} from 'motion/react';
import {MoreHorizontal} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Progress} from '@/components/ui/progress';
import {Skeleton} from '@/components/ui/skeleton';
import {
  buildMarketplaceCardHover,
  buildMarketplaceGridVariants,
  MARKETPLACE_MOTION_INSTANT,
} from '@/lib/animations/marketplace-motion';
import {cn} from '@/lib/utils';
import {MarketplaceStatusDot} from './StatusDot';
import type {MarketplaceCatalogItem, MarketplaceViewMode} from './types';

/** Table ↔ card morph duration stays in the 150–250ms band. / 表卡形变时长落在 150～250ms。 */
const MORPH_TRANSITION = {
  type: 'tween' as const,
  duration: 0.2,
  ease: [0.22, 1, 0.36, 1] as [number, number, number, number],
};

/**
 * Shared table ↔ card catalog with layoutId morph per item id.
 * Loading dim lives on the list container (not rows/cards) so entrance opacity cannot override it.
 * Reduced motion: instant switch, no morph.
 * 表/卡共用目录：按条目 id 做 layoutId 形变；加载变淡挂在列表容器；减少动效时直接切换。
 */
export function MarketplaceMorphCatalog({
  items,
  viewMode,
  loading = false,
  error = null,
  empty,
  className,
  'data-testid': dataTestId = 'marketplace-morph-catalog',
}: {
  items: MarketplaceCatalogItem[];
  viewMode: MarketplaceViewMode;
  loading?: boolean;
  /** Error banner for both table and card views. / 表/卡共用的错误态。 */
  error?: ReactNode | null;
  empty?: ReactNode;
  className?: string;
  'data-testid'?: string;
}) {
  const t = useTranslations('marketplaceUi');
  const reduceMotion = useReducedMotion() ?? false;
  const {container, item: itemVariants} = buildMarketplaceGridVariants(reduceMotion);
  const cardHover = buildMarketplaceCardHover(reduceMotion);

  if (error) {
    return (
      <div
        data-testid={`${dataTestId}-error`}
        role='alert'
        className={cn(
          'rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-8 text-center text-sm text-destructive',
          className,
        )}
      >
        {error}
      </div>
    );
  }

  if (loading && items.length === 0) {
    return (
      <div
        data-testid={`${dataTestId}-loading`}
        className={cn(
          viewMode === 'card'
            ? 'grid gap-3 sm:grid-cols-2 xl:grid-cols-3'
            : 'space-y-2 rounded-xl border border-border/60 bg-card/40 p-2',
          className,
        )}
      >
        {Array.from({length: viewMode === 'card' ? 6 : 5}).map((_, i) => (
          <Skeleton
            key={i}
            className={
              viewMode === 'card' ? 'h-44 rounded-xl' : 'h-14 rounded-md'
            }
          />
        ))}
      </div>
    );
  }

  if (!loading && items.length === 0) {
    return (
      <div
        data-testid={`${dataTestId}-empty`}
        className={cn(
          'flex min-h-[180px] items-center justify-center rounded-xl border border-dashed border-border/60 px-4 py-10 text-sm text-muted-foreground',
          className,
        )}
      >
        {empty ?? t('emptyList')}
      </div>
    );
  }

  return (
    <LayoutGroup id={dataTestId}>
      {/* Dim the list container on refresh so finished row/card entrance opacity:1 cannot override it.
          刷新变淡挂在列表容器：行/卡入场结束后的 inline opacity:1 不会盖掉这里的 class。 */}
      <motion.div
        data-testid={dataTestId}
        data-view-mode={viewMode}
        data-reduce-motion={reduceMotion ? 'true' : 'false'}
        className={cn(
          'transition-opacity duration-200',
          loading && 'pointer-events-none opacity-60',
          viewMode === 'card'
            ? 'grid gap-3 sm:grid-cols-2 xl:grid-cols-3'
            : 'overflow-hidden rounded-xl border border-border/60 bg-card/40 shadow-xs',
          className,
        )}
        variants={container}
        initial='hidden'
        animate='visible'
      >
        {items.map((entry) => (
          <MorphItem
            key={entry.id}
            item={entry}
            mode={viewMode}
            reduceMotion={reduceMotion}
            itemVariants={itemVariants}
            cardHover={cardHover}
          />
        ))}
      </motion.div>
    </LayoutGroup>
  );
}

function MorphItem({
  item,
  mode,
  reduceMotion,
  itemVariants,
  cardHover,
}: {
  item: MarketplaceCatalogItem;
  mode: MarketplaceViewMode;
  reduceMotion: boolean;
  itemVariants: import('motion/react').Variants;
  cardHover: ReturnType<typeof buildMarketplaceCardHover>;
}) {
  const t = useTranslations('marketplaceUi');
  const shellLayoutId = reduceMotion
    ? undefined
    : `marketplace-item-${item.id}`;
  const titleLayoutId = reduceMotion
    ? undefined
    : `marketplace-item-${item.id}-title`;
  const iconLayoutId = reduceMotion
    ? undefined
    : `marketplace-item-${item.id}-icon`;
  const layoutTransition = reduceMotion
    ? MARKETPLACE_MOTION_INSTANT
    : MORPH_TRANSITION;

  const identity = (
    <div className='flex min-w-0 items-center gap-3'>
      {item.leading}
      {item.icon ? (
        <motion.div
          layoutId={iconLayoutId}
          transition={layoutTransition}
          className='flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground ring-1 ring-border/60'
        >
          {item.icon}
        </motion.div>
      ) : null}
      <div className='min-w-0'>
        <motion.div
          layoutId={titleLayoutId}
          transition={layoutTransition}
          className='truncate text-sm font-medium text-foreground'
        >
          {item.title}
        </motion.div>
        {item.subtitle ? (
          <div className='truncate text-xs text-muted-foreground'>
            {item.subtitle}
          </div>
        ) : null}
      </div>
    </div>
  );

  const badges = (item.badges || []).map((badge) => (
    <Badge
      key={`${item.id}-${badge.label}`}
      variant={badge.highlight ? 'default' : 'outline'}
      className={cn(
        'max-w-[10rem] truncate rounded-full text-[11px]',
        badge.highlight &&
          'border-primary/25 bg-primary/15 text-primary hover:bg-primary/20',
      )}
    >
      {badge.label}
    </Badge>
  ));

  const status = item.status ? (
    <MarketplaceStatusDot label={item.status.label} tone={item.status.tone} />
  ) : null;

  const fields = (item.fields || []).slice(0, 4);
  const progress = item.progress;

  const moreMenu =
    item.menuActions != null ? (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8 shrink-0 rounded-md'
            aria-label={t('moreActions')}
            onClick={(e) => e.stopPropagation()}
          >
            <MoreHorizontal className='size-4' />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='min-w-[10rem]'>
          {item.menuActions}
        </DropdownMenuContent>
      </DropdownMenu>
    ) : null;

  if (mode === 'table') {
    return (
      <motion.div
        layout={!reduceMotion}
        layoutId={shellLayoutId}
        transition={layoutTransition}
        variants={itemVariants}
        role={item.onOpen ? 'button' : undefined}
        tabIndex={item.onOpen ? 0 : undefined}
        onClick={item.onOpen}
        onKeyDown={(e) => {
          if (!item.onOpen) {
            return;
          }
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            item.onOpen();
          }
        }}
        data-testid={`marketplace-row-${item.id}`}
        className={cn(
          'group grid grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/60 px-3.5 py-3 last:border-b-0 sm:grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_auto]',
          'hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          item.onOpen && 'cursor-pointer',
        )}
      >
        {identity}
        <div className='hidden min-w-0 flex-wrap items-center gap-1.5 sm:flex'>
          {badges}
        </div>
        <div className='min-w-0 space-y-1.5'>
          {status}
          {item.aside}
          {progress ? (
            <div className='space-y-1'>
              <div className='flex items-center justify-between gap-2 text-[11px] text-muted-foreground'>
                <span className='truncate'>{progress.label}</span>
                <span className='tabular-nums'>{progress.display}</span>
              </div>
              <Progress value={progress.value} className='h-1.5' />
            </div>
          ) : null}
        </div>
        <div className='hidden text-right text-xs text-muted-foreground sm:block'>
          {item.meta || ''}
        </div>
        <div
          className='flex items-center justify-end gap-1'
          onClick={(e) => e.stopPropagation()}
        >
          {moreMenu}
        </div>
      </motion.div>
    );
  }

  return (
    <motion.div
      layout={!reduceMotion}
      layoutId={shellLayoutId}
      transition={layoutTransition}
      variants={itemVariants}
      whileHover={cardHover}
      role={item.onOpen ? 'button' : undefined}
      tabIndex={item.onOpen ? 0 : undefined}
      onClick={item.onOpen}
      onKeyDown={(e) => {
        if (!item.onOpen) {
          return;
        }
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          item.onOpen();
        }
      }}
      data-testid={`marketplace-card-${item.id}`}
      className={cn(
        'group flex flex-col rounded-xl border border-border/60 bg-card/70 p-4 shadow-xs',
        'hover:border-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        item.onOpen && 'cursor-pointer',
      )}
    >
      <div className='flex items-start justify-between gap-2'>
        {identity}
        <div onClick={(e) => e.stopPropagation()}>{moreMenu}</div>
      </div>

      <div className='mt-3 flex flex-wrap items-center gap-1.5'>
        {badges}
        {status}
      </div>
      {item.aside ? <div className='mt-2'>{item.aside}</div> : null}

      {fields.length > 0 ? (
        <div className='mt-3 grid grid-cols-2 gap-x-3 gap-y-2'>
          {fields.map((field) => (
            <div key={`${item.id}-${field.label}`} className='min-w-0'>
              <div className='text-[11px] text-muted-foreground'>{field.label}</div>
              <div className='truncate text-xs font-medium'>{field.value}</div>
            </div>
          ))}
        </div>
      ) : null}

      {progress ? (
        <div className='mt-auto space-y-1.5 pt-3'>
          <div className='flex items-center justify-between gap-2 text-[11px] text-muted-foreground'>
            <span className='truncate'>{progress.label}</span>
            <span className='tabular-nums'>{progress.display}</span>
          </div>
          <Progress value={progress.value} className='h-1.5' />
        </div>
      ) : item.meta ? (
        <div className='mt-auto pt-3 text-xs text-muted-foreground'>{item.meta}</div>
      ) : null}
    </motion.div>
  );
}
