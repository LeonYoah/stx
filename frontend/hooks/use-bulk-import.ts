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

import {useState, useCallback} from 'react';
import {toast} from '@/lib/toast';

export function useBulkImport(initialItems: string[] = []) {
  const [items, setItems] = useState<string[]>(initialItems);
  const [bulkContent, setBulkContent] = useState('');
  const [allowDuplicates, setAllowDuplicates] = useState(false);

  const handleBulkImport = useCallback(() => {
    const lines = bulkContent
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line.length > 0);

    if (lines.length === 0) {
      toast.error('没有可导入的内容');
      return;
    }

    let importedCount = 0;
    let skippedDuplicates = 0;
    const newItems = [...items];

    for (const line of lines) {
      if (!allowDuplicates && newItems.includes(line)) {
        skippedDuplicates++;
        continue;
      }
      newItems.push(line);
      importedCount++;
    }

    if (importedCount === 0) {
      toast.error('没有新内容被导入');
      return;
    }

    setItems(newItems);
    setBulkContent('');

    const skippedInfo =
      skippedDuplicates > 0 ? `（跳过重复 ${skippedDuplicates} 条）` : '';
    const message = `成功导入 ${importedCount} 个内容${skippedInfo}`;
    toast.success(message);
  }, [bulkContent, items, allowDuplicates]);

  const removeItem = useCallback((index: number) => {
    setItems((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const clearItems = useCallback(() => {
    setItems([]);
  }, []);

  const clearBulkContent = useCallback(() => {
    setBulkContent('');
  }, []);

  const resetBulkImport = useCallback((newItems: string[] = []) => {
    setItems(newItems);
    setBulkContent('');
    setAllowDuplicates(false);
  }, []);

  return {
    items,
    setItems,
    bulkContent,
    setBulkContent,
    allowDuplicates,
    setAllowDuplicates,
    handleBulkImport,
    removeItem,
    clearItems,
    clearBulkContent,
    resetBulkImport,
  };
}
