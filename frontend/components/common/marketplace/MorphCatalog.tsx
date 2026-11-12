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

import {
  Children,
  Fragment,
  isValidElement,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from 'react';
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

/** Interactive selectors that must not trigger shell onOpen. / 不应触发外壳 onOpen 的交互选择器。 */
export const MARKETPLACE_INTERACTIVE_SELECTOR =
  'button,a,input,select,textarea,label,[role=menuitem],[role=checkbox],[role=menu]';

/**
 * True when menuActions has at least one renderable child (empty fragments → false).
 * 有可渲染菜单项时为 true；空 Fragment 视为无菜单。
 */
export function hasMarketplaceMenuActions(node: ReactNode): boolean {
  if (node == null || typeof node === 'boolean') {
    return false;
  }
  // Children.toArray already drops booleans / null; recurse into Fragments.
  // Children.toArray 已去掉 boolean/null；遇到 Fragment 再递归。
  return Children.toArray(node).some((child) => {
    if (isValidElement(child)) {
      if (child.type === Fragment) {
        return hasMarketplaceMenuActions(
          (child.props as {children?: ReactNode}).children,
        );
      }
      return true;
    }
    if (typeof child === 'string') {
      return child.trim().length > 0;
    }
    return child != null;
  });
}

/**
 * Skip shell open when the event lands on / bubbles from nested interactive controls,
 * or when the target is outside the shell (e.g. Radix portal menu).
 * 点在嵌套交互控件上，或目标不在外壳 DOM 内（如 Portal 菜单）时跳过打开。
 */
export function shouldSkipMarketplaceShellOpen(
  target: EventTarget | null,
  currentTarget: EventTarget | null,
): boolean {
  if (!(target instanceof Element) || !(currentTarget instanceof Element)) {
    return true;
  }
  // Portal / detached: menu content is not under the row/card DOM.
  // Portal / 已卸下：菜单内容不在行/卡 DOM 内。
  if (!currentTarget.contains(target)) {
    return true;
  }
  const interactive = target.closest(MARKETPLACE_INTERACTIVE_SELECTOR);
  if (
    interactive &&
    interactive !== currentTarget &&
    currentTarget.contains(interactive)
  ) {
    return true;
  }
  return false;
}

/**
 * Shared table ↔ card catalog with layoutId morph per item id.
 * Row/card shells are NOT role=button; open is a title control. Nested clicks/keys ignored.
 * 表/卡共用目录：整行不做 role=button；打开走标题控件；内部交互不冒泡触发 onOpen。
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
        {viewMode === 'table' ? (
          <div
            data-testid={`${dataTestId}-header`}
            className='hidden grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_auto] items-center gap-3 border-b border-border/60 bg-muted/30 px-3.5 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground sm:grid'
          >
            <span>{t('colName')}</span>
            <span>{t('colBadges')}</span>
            <span>{t('colStatus')}</span>
            <span className='text-right'>{t('colMeta')}</span>
            <span className='text-right'>{t('colActions')}</span>
          </div>
        ) : null}
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

  /**
   * Mouse: open unless the click is on a nested interactive control / portal menu.
   * Keyboard open stays on the title button; shell keydown still ignores nested controls.
   * 鼠标：非嵌套交互 / 非 Portal 时打开。键盘打开仍走标题按钮；外壳 keydown 仍忽略内部控件。
   */
  const handleShellClick = (e: MouseEvent<HTMLElement>) => {
    if (!item.onOpen) {
      return;
    }
    if (shouldSkipMarketplaceShellOpen(e.target, e.currentTarget)) {
      return;
    }
    item.onOpen();
  };

  const handleShellKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    // Keyboard activation is owned by the title button; ignore nested Enter/Space.
    // 键盘打开由标题按钮负责；忽略内部控件冒泡的 Enter/Space。
    if (shouldSkipMarketplaceShellOpen(e.target, e.currentTarget)) {
      return;
    }
    if (!item.onOpen) {
      return;
    }
    if (e.key !== 'Enter' && e.key !== ' ') {
      return;
    }
    e.preventDefault();
    item.onOpen();
  };

  const titleNode = item.onOpen ? (
    <Button
      type='button'
      variant='link'
      data-testid={`marketplace-open-${item.id}`}
      className='h-auto max-w-full min-w-0 justify-start p-0 text-sm font-medium text-foreground'
      onClick={(e) => {
        e.stopPropagation();
        item.onOpen?.();
      }}
      onKeyDown={(e) => e.stopPropagation()}
      aria-label={t('openItem', {name: item.title})}
    >
      <span className='truncate'>{item.title}</span>
    </Button>
  ) : (
    <span className='truncate text-sm font-medium text-foreground'>
      {item.title}
    </span>
  );

  const identity = (
    <div className='flex min-w-0 items-center gap-3'>
      {item.leading}
      {item.icon ? (
        <motion.div
          layoutId={iconLayoutId}
          transition={layoutTransition}
          data-testid={`marketplace-icon-${item.id}`}
          className='flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground ring-1 ring-border/60'
        >
          {item.icon}
        </motion.div>
      ) : null}
      <div className='min-w-0'>
        <motion.div
          layoutId={titleLayoutId}
          transition={layoutTransition}
          className='min-w-0 max-w-full'
        >
          {titleNode}
        </motion.div>
        {item.subtitle ? (
          <div
            data-testid={`marketplace-subtitle-${item.id}`}
            className='truncate text-xs text-muted-foreground'
          >
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
      data-testid={`marketplace-badge-${item.id}`}
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
  const showMenu = hasMarketplaceMenuActions(item.menuActions);

  const progressBlock = progress ? (
    <div className='space-y-1'>
      <div className='flex items-center justify-between gap-2 text-[11px] text-muted-foreground'>
        <span className='truncate'>{progress.label}</span>
        <span className='tabular-nums'>{progress.display}</span>
      </div>
      <Progress
        value={progress.value}
        className='h-1.5'
        aria-label={t('progressAria', {
          label: progress.label,
          display: progress.display,
        })}
      />
    </div>
  ) : null;

  const moreMenu = showMenu ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          className='size-8 shrink-0 rounded-md'
          aria-label={t('moreActions')}
          data-testid={`marketplace-more-${item.id}`}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => e.stopPropagation()}
        >
          <MoreHorizontal className='size-4' />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-[10rem]'>
        {item.menuActions}
      </DropdownMenuContent>
    </DropdownMenu>
  ) : null;

  const actions = (
    <div
      className='flex items-center justify-end gap-1'
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
    >
      {item.primaryActions}
      {moreMenu}
    </div>
  );

  if (mode === 'table') {
    return (
      <motion.div
        layout={!reduceMotion}
        layoutId={shellLayoutId}
        transition={layoutTransition}
        variants={itemVariants}
        onClick={handleShellClick}
        onKeyDown={handleShellKeyDown}
        data-testid={`marketplace-row-${item.id}`}
        className={cn(
          'group grid grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/60 px-3.5 py-3 last:border-b-0 sm:grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_auto]',
          'hover:bg-muted/40',
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
          {progressBlock}
        </div>
        <div className='hidden text-right text-xs text-muted-foreground sm:block'>
          {item.meta || ''}
        </div>
        {actions}
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
      onClick={handleShellClick}
      onKeyDown={handleShellKeyDown}
      data-testid={`marketplace-card-${item.id}`}
      className={cn(
        'group flex flex-col rounded-xl border border-border/60 bg-card/70 p-4 shadow-xs hover:border-border',
        item.onOpen && 'cursor-pointer',
      )}
    >
      <div className='flex items-start justify-between gap-2'>
        {identity}
        {actions}
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

      {progressBlock ? (
        <div className='mt-auto pt-3'>{progressBlock}</div>
      ) : item.meta ? (
        <div className='mt-auto pt-3 text-xs text-muted-foreground'>{item.meta}</div>
      ) : null}
    </motion.div>
  );
}
