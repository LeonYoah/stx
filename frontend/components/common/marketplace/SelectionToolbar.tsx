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
 * Prestige-style bulk action strip:「已选 N 项」+ actions; table view only.
 * Prestige 风格批量操作条：已选数量 + 操作；仅表格视图展示。
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
    <div
      data-testid={dataTestId}
      role='region'
      aria-label={t('selectionToolbarAria')}
      className={cn(
        'flex flex-wrap items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2',
        className,
      )}
    >
      <span
        data-testid={`${dataTestId}-count`}
        className='text-sm font-medium text-foreground'
      >
        {t('selectedCount', {count: selectedCount})}
      </span>
      {hint ? (
        <span className='text-xs text-muted-foreground'>{hint}</span>
      ) : null}
      <div className='ml-auto flex flex-wrap items-center gap-1.5'>{children}</div>
      {onClear ? (
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-8 rounded-md text-xs'
          data-testid={`${dataTestId}-clear`}
          onClick={onClear}
          aria-label={t('clearSelection')}
        >
          <X className='mr-1 size-3.5' />
          {t('clearSelection')}
        </Button>
      ) : null}
    </div>
  );
}
