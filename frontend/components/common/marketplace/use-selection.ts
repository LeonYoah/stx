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

import {useCallback, useMemo, useState} from 'react';
import type {MarketplaceCatalogItem} from './types';
import type {MarketplaceSelectionState} from './MorphCatalog';

/**
 * Table multi-select state with indeterminate-friendly helpers.
 * 表格多选状态，配合全选 indeterminate。
 */
export function useMarketplaceSelection(itemIds: string[]) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());

  const onToggle = useCallback((id: string, selected: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (selected) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  }, []);

  const onToggleAll = useCallback(
    (selected: boolean) => {
      setSelectedIds((prev) => {
        if (!selected) {
          const next = new Set(prev);
          for (const id of itemIds) {
            next.delete(id);
          }
          return next;
        }
        const next = new Set(prev);
        for (const id of itemIds) {
          next.add(id);
        }
        return next;
      });
    },
    [itemIds],
  );

  const onClear = useCallback(() => {
    setSelectedIds(new Set());
  }, []);

  const removeIds = useCallback((ids: string[]) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      for (const id of ids) {
        next.delete(id);
      }
      return next;
    });
  }, []);

  const keepOnly = useCallback((ids: string[]) => {
    setSelectedIds(new Set(ids));
  }, []);

  const selection: MarketplaceSelectionState = useMemo(
    () => ({
      selectedIds,
      onToggle,
      onToggleAll,
      onClear,
    }),
    [onClear, onToggle, onToggleAll, selectedIds],
  );

  const selectedList = useMemo(
    () => itemIds.filter((id) => selectedIds.has(id)),
    [itemIds, selectedIds],
  );

  return {
    selectedIds,
    selectedList,
    selection,
    onClear,
    removeIds,
    keepOnly,
    setSelectedIds,
  };
}

/** Filter catalog items by current selection set. / 按当前选中集合过滤目录项。 */
export function filterSelectedItems(
  items: MarketplaceCatalogItem[],
  selectedIds: ReadonlySet<string>,
): MarketplaceCatalogItem[] {
  return items.filter((item) => selectedIds.has(item.id));
}
