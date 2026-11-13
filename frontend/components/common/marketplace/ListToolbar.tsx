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
import {LayoutGrid, Rows3} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {ToggleGroup, ToggleGroupItem} from '@/components/ui/toggle-group';
import {cn} from '@/lib/utils';
import type {MarketplaceViewMode} from './types';

/**
 * List header: title + muted count + table/card ToggleGroup.
 * Keyboard-reachable, aria-pressed via Radix Toggle, i18n labels; parent persists mode.
 * 列表标题行：标题 + 灰色计数 + 表格/卡片切换。
 * 键盘可达，Radix Toggle 提供 aria-pressed，文案走 i18n；视图记忆由调用方持久化。
 */
export function MarketplaceListToolbar({
  title,
  count,
  countLabel,
  viewMode,
  onViewModeChange,
  className,
  trailing,
}: {
  title: string;
  count: number;
  /** Preformatted count caption; falls back to ICU via marketplaceUi.itemCount. */
  countLabel?: string;
  viewMode: MarketplaceViewMode;
  onViewModeChange: (mode: MarketplaceViewMode) => void;
  className?: string;
  trailing?: ReactNode;
}) {
  const t = useTranslations('marketplaceUi');

  return (
    <div
      data-testid='marketplace-list-toolbar'
      className={cn(
        'flex flex-wrap items-center justify-between gap-3',
        className,
      )}
    >
      <div className='min-w-0'>
        <h2 className='text-base font-semibold tracking-tight'>{title}</h2>
        <p className='text-xs text-muted-foreground'>
          {countLabel ?? t('itemCount', {count})}
        </p>
      </div>

      <div className='flex items-center gap-2'>
        {trailing}
        <ToggleGroup
          type='single'
          value={viewMode}
          onValueChange={(value) => {
            // Keep current selection when the user re-clicks the active item.
            // 再次点击当前项时保持选择，避免清空。
            if (value === 'table' || value === 'card') {
              onViewModeChange(value);
            }
          }}
          variant='outline'
          size='sm'
          className='rounded-md border border-border/60'
          aria-label={t('viewModeLabel')}
          data-testid='marketplace-view-toggle'
        >
          <ToggleGroupItem
            value='table'
            aria-label={t('viewTable')}
            aria-pressed={viewMode === 'table'}
            data-testid='marketplace-view-table'
            className='gap-1.5 rounded-md px-2.5'
          >
            <Rows3 className='size-3.5' />
            <span className='hidden sm:inline'>{t('viewTable')}</span>
          </ToggleGroupItem>
          <ToggleGroupItem
            value='card'
            aria-label={t('viewCard')}
            aria-pressed={viewMode === 'card'}
            data-testid='marketplace-view-card'
            className='gap-1.5 rounded-md px-2.5'
          >
            <LayoutGrid className='size-3.5' />
            <span className='hidden sm:inline'>{t('viewCard')}</span>
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
    </div>
  );
}
