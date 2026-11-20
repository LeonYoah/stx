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

import {LayoutGrid, List} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {ViewToggle} from '@/components/common/ViewToggle';
import type {MarketplaceViewMode} from './types';

/**
 * Marketplace table/card icon toggle: shared localized labels and stable
 * test ids for installer and plugin catalog pages.
 * 市场类目录页共用的表格/卡片图标切换：统一文案与测试锚点。
 */
export function MarketplaceViewToggle({
  viewMode,
  onViewModeChange,
  className,
}: {
  viewMode: MarketplaceViewMode;
  onViewModeChange: (mode: MarketplaceViewMode) => void;
  className?: string;
}) {
  const t = useTranslations('marketplaceUi');

  return (
    <ViewToggle
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
          icon: List,
          ariaLabel: t('viewTable'),
          testId: 'marketplace-view-table',
        },
        {
          value: 'card',
          icon: LayoutGrid,
          ariaLabel: t('viewCard'),
          testId: 'marketplace-view-card',
        },
      ]}
      testId='marketplace-view-toggle'
      className={className}
    />
  );
}
