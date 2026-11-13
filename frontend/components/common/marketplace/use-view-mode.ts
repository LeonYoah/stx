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

import {useCallback, useEffect, useState} from 'react';
import type {MarketplaceViewMode} from './types';

/**
 * Persist table/card view mode in localStorage (hydrate after mount to avoid mismatch).
 * 将表/卡视图模式写入 localStorage（挂载后再回填，避免水合不一致）。
 */
export function useMarketplaceViewMode(
  storageKey: string,
  defaultMode: MarketplaceViewMode = 'table',
): [MarketplaceViewMode, (mode: MarketplaceViewMode) => void] {
  const [mode, setModeState] = useState<MarketplaceViewMode>(defaultMode);

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(storageKey);
      if (raw === 'table' || raw === 'card') {
        setModeState(raw);
      }
    } catch {
      // ignore storage failures / 忽略存储失败
    }
  }, [storageKey]);

  const setMode = useCallback(
    (next: MarketplaceViewMode) => {
      setModeState(next);
      try {
        window.localStorage.setItem(storageKey, next);
      } catch {
        // ignore storage failures / 忽略存储失败
      }
    },
    [storageKey],
  );

  return [mode, setMode];
}
