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

import type {CSSProperties, ReactNode} from 'react';
import {LayoutGrid, List} from 'lucide-react';
import {useTranslations} from 'next-intl';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {cn} from '@/lib/utils';
import type {MarketplaceViewMode} from './types';

// 局部收敛 Arc 分段控件圆角与高度，使其在非监控页也与控制台保持一致的紧凑尺度。
// Local token scope so Arc SegmentedControl keeps compact 8px/28px console sizing here too.
const VIEW_TOGGLE_SCOPE: CSSProperties = {
  ['--radius-control' as string]: '0.5rem',
  ['--control-height-sm' as string]: '1.75rem',
};

/**
 * List header: title + muted count + icon-only Arc SegmentedControl (table / card).
 * 列表标题行：标题 + 灰色计数 + 纯图标 Arc 分段视图切换（列表 / 卡片）。
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

      <div
        className='flex items-center gap-2'
        style={VIEW_TOGGLE_SCOPE}
        data-testid='marketplace-view-toggle'
      >
        {trailing}
        <SegmentedControl
          label={t('viewModeLabel')}
          value={viewMode}
          onValueChange={(value) => {
            if (value === 'table' || value === 'card') {
              onViewModeChange(value);
            }
          }}
          options={[
            {
              value: 'table',
              label: <List className='size-4' aria-hidden />,
              ariaLabel: t('viewTable'),
              testId: 'marketplace-view-table',
            },
            {
              value: 'card',
              label: <LayoutGrid className='size-4' aria-hidden />,
              ariaLabel: t('viewCard'),
              testId: 'marketplace-view-card',
            },
          ]}
        />
      </div>
    </div>
  );
}
