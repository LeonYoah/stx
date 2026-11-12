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
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import {LayoutGroup, motion, useReducedMotion} from 'motion/react';
import {Eye, MoreHorizontal, RotateCcw} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
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
import {
  CATALOG_TABLE_COLUMNS,
  CATALOG_TABLE_COLUMNS_SELECTABLE,
  CATALOG_TABLE_GRID,
  CATALOG_TABLE_MIN_WIDTH,
  CATALOG_TABLE_MIN_WIDTH_SELECTABLE,
} from './catalog-table-grid';
import {MarketplaceSelectionToolbar} from './SelectionToolbar';
import {MarketplaceStatusDot} from './StatusDot';
import type {MarketplaceCatalogItem, MarketplaceViewMode} from './types';

/** Table ↔ card morph duration stays in the 150–250ms band. / 表卡形变时长落在 150～250ms。 */
const MORPH_TRANSITION = {
  type: 'tween' as const,
  duration: 0.2,
  ease: [0.22, 1, 0.36, 1] as [number, number, number, number],
};

/** Card 3D flip duration (ignored under reduced motion). / 卡片 3D 翻转时长（减少动效时忽略）。 */
const FLIP_TRANSITION = {
  type: 'tween' as const,
  duration: 0.35,
  ease: [0.22, 1, 0.36, 1] as [number, number, number, number],
};

/** Interactive selectors that must not trigger shell onOpen. / 不应触发外壳 onOpen 的交互选择器。 */
export const MARKETPLACE_INTERACTIVE_SELECTOR =
  'button,a,input,select,textarea,label,[role=menuitem],[role=checkbox],[role=menu]';

export type MarketplaceSelectionState = {
  selectedIds: ReadonlySet<string>;
  onToggle: (id: string, selected: boolean) => void;
  onToggleAll: (selected: boolean) => void;
  onClear?: () => void;
  /** When false, row checkbox is disabled. / 返回 false 时该行不可勾选。 */
  isItemSelectable?: (item: MarketplaceCatalogItem) => boolean;
};

/**
 * True when menuActions has at least one renderable child (empty fragments → false).
 * 有可渲染菜单项时为 true；空 Fragment 视为无菜单。
 */
export function hasMarketplaceMenuActions(node: ReactNode): boolean {
  if (node == null || typeof node === 'boolean') {
    return false;
  }
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
 * Table header/rows share CATALOG_TABLE_GRID; card flip is button-only with a11y faces.
 * 表/卡共用目录：表头与行共用 CATALOG_TABLE_GRID；卡片翻面仅按钮触发并做好正背面 a11y。
 */
export function MarketplaceMorphCatalog({
  items,
  viewMode,
  loading = false,
  error = null,
  empty,
  className,
  selection,
  bulkActions,
  bulkHint,
  'data-testid': dataTestId = 'marketplace-morph-catalog',
}: {
  items: MarketplaceCatalogItem[];
  viewMode: MarketplaceViewMode;
  loading?: boolean;
  error?: ReactNode | null;
  empty?: ReactNode;
  className?: string;
  selection?: MarketplaceSelectionState;
  /** Bulk action buttons (table view + selection only). / 批量操作按钮（仅表格+有选中时）。 */
  bulkActions?: ReactNode;
  bulkHint?: ReactNode;
  'data-testid'?: string;
}) {
  const t = useTranslations('marketplaceUi');
  const reduceMotion = useReducedMotion() ?? false;
  const {container, item: itemVariants} = buildMarketplaceGridVariants(reduceMotion);
  const cardHover = buildMarketplaceCardHover(reduceMotion);
  const selectable = Boolean(selection);
  const tableColumnsClass = selectable
    ? CATALOG_TABLE_COLUMNS_SELECTABLE
    : CATALOG_TABLE_COLUMNS;
  const tableGridClass = CATALOG_TABLE_GRID;
  const tableMinWidth = selectable
    ? CATALOG_TABLE_MIN_WIDTH_SELECTABLE
    : CATALOG_TABLE_MIN_WIDTH;

  const selectableItems = useMemo(() => {
    if (!selection) {
      return items;
    }
    return items.filter((item) =>
      selection.isItemSelectable ? selection.isItemSelectable(item) : true,
    );
  }, [items, selection]);

  const allSelected =
    selectableItems.length > 0 &&
    selectableItems.every((item) => selection?.selectedIds.has(item.id));
  const someSelected =
    selectableItems.some((item) => selection?.selectedIds.has(item.id)) &&
    !allSelected;
  const selectedCount = selection?.selectedIds.size ?? 0;
  const showBulkBar = viewMode === 'table' && selectedCount > 0;

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
    <div className={cn('space-y-2', className)}>
      {showBulkBar && selection ? (
        <MarketplaceSelectionToolbar
          selectedCount={selectedCount}
          onClear={selection.onClear}
          hint={bulkHint}
        >
          {bulkActions}
        </MarketplaceSelectionToolbar>
      ) : null}

      <LayoutGroup id={dataTestId}>
        {/* key=viewMode：表↔卡切换时整树重挂，避免卡片停在 variants hidden（opacity:0）。
            Remount on viewMode so cards cannot stick in the hidden entrance state. */}
        <motion.div
          key={`${dataTestId}-${viewMode}`}
          data-testid={dataTestId}
          data-view-mode={viewMode}
          data-reduce-motion={reduceMotion ? 'true' : 'false'}
          data-selectable={selectable ? 'true' : 'false'}
          className={cn(
            'transition-opacity duration-200',
            loading && 'pointer-events-none opacity-60',
            viewMode === 'card'
              ? 'grid gap-3 sm:grid-cols-2 xl:grid-cols-3'
              : 'overflow-x-auto rounded-xl border border-border/60 bg-card/40 shadow-xs',
          )}
          variants={container}
          initial='hidden'
          animate='visible'
        >
          {viewMode === 'table' ? (
            <div
              className={cn(
                'grid gap-x-3 px-3.5',
                tableColumnsClass,
                tableMinWidth,
              )}
              data-testid={`${dataTestId}-table`}
            >
              <div
                data-testid={`${dataTestId}-header`}
                className={cn(
                  tableGridClass,
                  'border-b border-border/60 bg-muted/30 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground',
                )}
              >
                {selectable ? (
                  <div
                    data-col='select'
                    className='flex w-full min-w-0 items-center justify-center'
                  >
                    <Checkbox
                      data-testid={`${dataTestId}-select-all`}
                      checked={
                        allSelected
                          ? true
                          : someSelected
                            ? 'indeterminate'
                            : false
                      }
                      onCheckedChange={(checked) => {
                        selection?.onToggleAll(checked === true);
                      }}
                      aria-label={t('selectAll')}
                    />
                  </div>
                ) : null}
                {/* 每列外层 w-full，保证与行格子左缘对齐（勿测内容盒）。 / Full-width cell shells so left edges match rows. */}
                <span data-col='name' className='block w-full min-w-0'>
                  {t('colName')}
                </span>
                <span data-col='badges' className='block w-full min-w-0'>
                  {t('colBadges')}
                </span>
                <span data-col='status' className='block w-full min-w-0'>
                  {t('colStatus')}
                </span>
                <span
                  data-col='meta'
                  className='block w-full min-w-0 text-right'
                >
                  {t('colMeta')}
                </span>
                <span
                  data-col='actions'
                  className='block w-full min-w-0 text-right'
                >
                  {t('colActions')}
                </span>
              </div>
              {items.map((entry) => (
                <MorphItem
                  key={`${entry.id}-table`}
                  item={entry}
                  mode={viewMode}
                  reduceMotion={reduceMotion}
                  itemVariants={itemVariants}
                  cardHover={cardHover}
                  tableGridClass={tableGridClass}
                  selection={selection}
                />
              ))}
            </div>
          ) : (
            items.map((entry) => (
              <MorphItem
                key={`${entry.id}-card`}
                item={entry}
                mode={viewMode}
                reduceMotion={reduceMotion}
                itemVariants={itemVariants}
                cardHover={cardHover}
                tableGridClass={tableGridClass}
                selection={selection}
              />
            ))
          )}
        </motion.div>
      </LayoutGroup>
    </div>
  );
}

function MorphItem({
  item,
  mode,
  reduceMotion,
  itemVariants,
  cardHover,
  tableGridClass,
  selection,
}: {
  item: MarketplaceCatalogItem;
  mode: MarketplaceViewMode;
  reduceMotion: boolean;
  itemVariants: import('motion/react').Variants;
  cardHover: ReturnType<typeof buildMarketplaceCardHover>;
  tableGridClass: string;
  selection?: MarketplaceSelectionState;
}) {
  const t = useTranslations('marketplaceUi');
  const [flipped, setFlipped] = useState(false);

  useEffect(() => {
    setFlipped(false);
  }, [mode, item.id]);

  // layoutId only on the outer morph shell — never on the flip rotateY layer.
  // layoutId 只挂在表卡形变外壳，绝不与翻面 rotateY 内层共用。
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

  const titleNode = item.onOpen ? (
    <Button
      type='button'
      variant='link'
      data-testid={`marketplace-open-${item.id}`}
      className='h-auto max-w-full min-w-0 justify-start p-0 text-sm font-medium text-foreground'
      onClick={() => item.onOpen?.()}
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
      {/* legacy leading (e.g. old checkbox) ignored when catalog owns selection column */}
      {!selection ? item.leading : null}
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

  const fields = item.fields || [];
  const progress = item.progress;
  const showMenu = hasMarketplaceMenuActions(item.menuActions);
  const hasBackContent = Boolean(
    fields.length > 0 || item.aside || item.description,
  );
  // 减少动态：不翻面、不显示翻转按钮，背面信息改走详情。
  // Reduced motion: no flip UI; secondary info lives in the detail dialog.
  const flipEnabled = hasBackContent && !reduceMotion;
  const itemSelectable = selection
    ? selection.isItemSelectable
      ? selection.isItemSelectable(item)
      : true
    : false;
  const isSelected = selection?.selectedIds.has(item.id) ?? false;

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
        >
          <MoreHorizontal className='size-4' />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-[10rem]'>
        {item.menuActions}
      </DropdownMenuContent>
    </DropdownMenu>
  ) : null;

  const viewDetailButton = item.onOpen ? (
    <Button
      type='button'
      variant='outline'
      size='sm'
      className='h-8 rounded-md text-xs'
      data-testid={`marketplace-detail-${item.id}`}
      onClick={() => item.onOpen?.()}
    >
      <Eye className='mr-1.5 size-3.5' />
      {t('viewDetail')}
    </Button>
  ) : null;

  const renderFlipButton = (face: 'front' | 'back') =>
    flipEnabled ? (
      <Button
        type='button'
        variant='ghost'
        size='sm'
        className='h-8 rounded-md text-xs'
        data-testid={`marketplace-flip-${item.id}-${face}`}
        aria-pressed={flipped}
        aria-label={flipped ? t('flipCardFront') : t('flipCard')}
        onClick={() => setFlipped((prev) => !prev)}
      >
        <RotateCcw className='mr-1.5 size-3.5' />
        {flipped ? t('flipCardFront') : t('flipCard')}
      </Button>
    ) : null;

  const selectCell =
    selection && mode === 'table' ? (
      <div
        data-col='select'
        className='flex w-full min-w-0 items-center justify-center'
      >
        <Checkbox
          data-testid={`marketplace-select-${item.id}`}
          checked={isSelected}
          disabled={!itemSelectable}
          onCheckedChange={(checked) => {
            selection.onToggle(item.id, checked === true);
          }}
          aria-label={t('selectItem', {name: item.title})}
        />
      </div>
    ) : null;

  const tableActions = (
    <div
      data-col='actions'
      data-testid={`marketplace-actions-${item.id}`}
      className='flex w-full min-w-0 items-center justify-end gap-1'
    >
      {item.primaryActions}
      {moreMenu}
    </div>
  );

  // 信息列：meta（如「已下载」）+ 进度；绝不把「已下载」塞进操作列。
  // Meta column holds status text like「已下载」; never park it in the actions column.
  const metaCell = (
    <div
      data-col='meta'
      data-testid={`marketplace-meta-${item.id}`}
      className='min-w-0 w-full space-y-1 text-right text-xs text-muted-foreground'
    >
      {item.meta ? <div className='truncate'>{item.meta}</div> : null}
      {progressBlock}
    </div>
  );

  if (mode === 'table') {
    // 表格行：不用 layoutId / scale 变体，避免 transform 把 subgrid 列缘视觉错位。
    // Table rows: no layoutId / scale variants — transforms would skew subgrid edges.
    return (
      <div
        data-testid={`marketplace-row-${item.id}`}
        data-selected={isSelected ? 'true' : 'false'}
        className={cn(
          tableGridClass,
          'group border-b border-border/60 py-3 last:border-b-0 hover:bg-muted/40',
          isSelected && 'bg-primary/5',
        )}
      >
        {selectCell}
        <div data-col='name' className='min-w-0 w-full'>
          {identity}
        </div>
        <div
          data-col='badges'
          className='flex min-w-0 w-full flex-wrap items-center gap-1.5'
        >
          {badges}
        </div>
        <div data-col='status' className='min-w-0 w-full space-y-1.5'>
          {status}
          {item.aside}
        </div>
        {metaCell}
        {tableActions}
      </div>
    );
  }

  const frontHidden = flipped && flipEnabled;
  const backHidden = !flipped;

  const frontFace = (
    <div
      data-testid={`marketplace-card-front-${item.id}`}
      className='flex h-full flex-col'
      aria-hidden={frontHidden || undefined}
      {...(frontHidden ? ({inert: true} as Record<string, unknown>) : {})}
    >
      <div className='flex items-start justify-between gap-2'>
        {identity}
        {moreMenu && !item.primaryActions && !viewDetailButton ? moreMenu : null}
      </div>

      <div className='mt-3 flex flex-wrap items-center gap-1.5'>
        {badges}
        {status}
      </div>

      {progressBlock ? <div className='mt-3'>{progressBlock}</div> : null}

      {item.meta && !hasBackContent ? (
        <div className='mt-auto pt-3 text-xs text-muted-foreground'>{item.meta}</div>
      ) : (
        <div className='mt-auto' />
      )}

      <div className='mt-3 flex flex-wrap items-center justify-end gap-1 border-t border-border/50 pt-3'>
        {item.primaryActions}
        {viewDetailButton}
        {item.primaryActions || viewDetailButton ? moreMenu : null}
        {renderFlipButton('front')}
      </div>
    </div>
  );

  const backFace = (
    <div
      data-testid={`marketplace-card-back-${item.id}`}
      className='flex h-full flex-col'
      aria-hidden={backHidden || undefined}
      {...(backHidden ? ({inert: true} as Record<string, unknown>) : {})}
    >
      <div className='flex items-start justify-between gap-2'>
        <div className='min-w-0'>
          <div className='truncate text-sm font-medium text-foreground'>
            {item.title}
          </div>
          <div className='text-[11px] text-muted-foreground'>{t('cardBackHint')}</div>
        </div>
        {renderFlipButton('back')}
      </div>

      {item.description ? (
        <p
          data-testid={`marketplace-description-${item.id}`}
          className='mt-3 max-h-24 overflow-y-auto text-xs leading-relaxed text-muted-foreground'
        >
          {item.description}
        </p>
      ) : null}

      {item.aside ? <div className='mt-3'>{item.aside}</div> : null}

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

      {item.meta ? (
        <div className='mt-auto pt-3 text-xs text-muted-foreground'>{item.meta}</div>
      ) : (
        <div className='mt-auto' />
      )}
    </div>
  );

  return (
    <motion.div
      layout={!reduceMotion}
      layoutId={shellLayoutId}
      transition={layoutTransition}
      variants={itemVariants}
      // 显式 initial/animate，避免父级已 visible 时子项卡在 hidden。
      // Explicit initial/animate so children cannot stick hidden when the parent is already visible.
      initial='hidden'
      animate='visible'
      whileHover={cardHover}
      data-testid={`marketplace-card-${item.id}`}
      data-flipped={flipped && flipEnabled ? 'true' : 'false'}
      className='group flex flex-col rounded-xl border border-border/60 bg-card/70 p-4 shadow-xs hover:border-border'
    >
      {/* Inner flip layer: rotateY only — no layoutId here. / 内层只做 rotateY，不挂 layoutId。 */}
      {flipEnabled ? (
        <div className='relative min-h-[11rem] flex-1 [perspective:1200px]'>
          <motion.div
            className='relative h-full w-full'
            style={{transformStyle: 'preserve-3d'}}
            animate={{rotateY: flipped ? 180 : 0}}
            transition={FLIP_TRANSITION}
            data-testid={`marketplace-flip-layer-${item.id}`}
          >
            <div
              className='h-full'
              style={{
                transform: 'rotateY(0deg)',
                backfaceVisibility: 'hidden',
                WebkitBackfaceVisibility: 'hidden',
              }}
            >
              {frontFace}
            </div>
            <div
              className='absolute inset-0 h-full'
              style={{
                transform: 'rotateY(180deg)',
                backfaceVisibility: 'hidden',
                WebkitBackfaceVisibility: 'hidden',
              }}
            >
              {backFace}
            </div>
          </motion.div>
        </div>
      ) : (
        <div className='flex min-h-[11rem] flex-1 flex-col'>{frontFace}</div>
      )}
    </motion.div>
  );
}
