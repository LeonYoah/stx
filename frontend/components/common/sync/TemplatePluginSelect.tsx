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

import {useTranslations} from 'next-intl';
import {Combobox} from '@/components/arc/combobox/combobox';
import type {TemplatePluginItem} from './sync-studio-utils';

/**
 * 插件默认参数插入：长列表可搜索，选中即插入，不保留选中值。
 * Plugin default insert: searchable long list; choosing a row inserts it and does not keep a selection.
 */
export function TemplatePluginSelect({
  label,
  placeholder,
  items,
  disabled,
  loading,
  loadingText,
  onSelect,
}: {
  label: string;
  placeholder: string;
  items: TemplatePluginItem[];
  disabled?: boolean;
  loading?: boolean;
  loadingText?: string | null;
  onSelect: (value: string) => void;
}) {
  const t = useTranslations('workbenchStudio');

  return (
    <Combobox
      label={label}
      placeholder={placeholder}
      emptyMessage={t('noMatchingPlugins')}
      description={loading ? loadingText || t('loadingPluginTemplates') : undefined}
      options={items.map((item) => ({
        value: item.value,
        label: item.label,
        keywords: [item.value],
      }))}
      value=''
      disabled={disabled || loading}
      onValueChange={(value) => {
        if (value) {
          onSelect(value);
        }
      }}
    />
  );
}
