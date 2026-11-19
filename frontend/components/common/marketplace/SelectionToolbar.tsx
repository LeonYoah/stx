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
import {X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {cn} from '@/lib/utils';

/**
 * 底部居中浮动批量操作条：避让底部 Dock，不顶高上方表格造成布局跳动；仅表格有选中时出现。
 * Centered floating bulk action bar above the bottom Dock: avoids pushing the table
 * down when items are selected; rendered in table view when selectedCount > 0.
 */
export function MarketplaceSelectionToolbar({
  selectedCount,
  onClear,
  children,
  hint,
  className,
  'data-testid': dataTestId = 'marketplace-selection-toolbar',
}: {
  selectedCount: number;
  onClear?: () => void;
  children?: ReactNode;
  /** e.g.「将处理 N 项，跳过 M 项」. / 例如跳过说明。 */
  hint?: ReactNode;
  className?: string;
  'data-testid'?: string;
}) {
  const t = useTranslations('marketplaceUi');

  if (selectedCount <= 0) {
    return null;
  }

  return (
    <div className='pointer-events-none fixed inset-x-0 bottom-24 z-30 flex justify-center px-4'>
      <div
        data-testid={dataTestId}
        role='region'
        aria-label={t('selectionToolbarAria')}
        className={cn(
          'pointer-events-auto flex flex-wrap items-center gap-3 rounded-lg border border-border bg-card/95 px-3.5 py-2 shadow-xl backdrop-blur-sm',
          className,
        )}
      >
        <span
          data-testid={`${dataTestId}-count`}
          className='text-sm font-medium tabular-nums text-foreground'
        >
          {t('selectedCount', {count: selectedCount})}
        </span>
        {hint ? (
          <span className='text-xs text-muted-foreground'>{hint}</span>
        ) : null}
        {children ? (
          <div className='flex flex-wrap items-center gap-1.5'>{children}</div>
        ) : null}
        {onClear ? (
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8 rounded-md'
            data-testid={`${dataTestId}-clear`}
            onClick={onClear}
            aria-label={t('clearSelection')}
            title={t('clearSelection')}
          >
            <X className='size-4' aria-hidden />
          </Button>
        ) : null}
      </div>
    </div>
  );
}
