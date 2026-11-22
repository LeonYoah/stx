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

/**
 * 服务端分页列表查询状态：page / pageSize / filters；
 * patchFilters 与 setPageSize 一律回第 1 页（ui-conventions §3）。
 * 可选把 page + 指定 filter 键同步到 URL search（主机深链仍用 path `/hosts/[id]`）。
 * Server-paged list query state: page / pageSize / filters;
 * patchFilters and setPageSize always reset to page 1 (ui-conventions §3).
 * Optional sync of page + selected filter keys to the URL search
 * (host deep-links still use the path `/hosts/[id]`).
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';

export type ServerListFilters = Record<string, string | number | boolean | undefined | null>;

export interface UseServerListQueryOptions<F extends ServerListFilters> {
  initialFilters: F;
  defaultPageSize?: number;
  /** 从 URL 读初始值，并在变更时写回。Read initial values from the URL and write changes back. */
  urlSync?: {
    /** 要同步的 filter 键（值为空串 / undefined 时从 URL 删除）。Filter keys to sync (empty/undefined removes the param). */
    filterKeys: (keyof F & string)[];
    pageKey?: string;
    pageSizeKey?: string;
  };
}

export interface ServerListQuery<F extends ServerListFilters> {
  page: number;
  pageSize: number;
  filters: F;
  setPage: (page: number) => void;
  setPageSize: (pageSize: number) => void;
  /** 合并筛选；始终重置 page=1。Merge filters; always resets page to 1. */
  patchFilters: (patch: Partial<F>) => void;
  /** 用完整筛选对象替换；始终重置 page=1。Replace all filters; always resets page to 1. */
  replaceFilters: (next: F) => void;
  /** 恢复 initialFilters 并回第 1 页。Restore initialFilters and page 1. */
  resetFilters: () => void;
}

function readPositiveInt(raw: string | null, fallback: number): number {
  const n = Number(raw);
  return Number.isFinite(n) && n >= 1 ? Math.floor(n) : fallback;
}

function filtersFromSearchParams<F extends ServerListFilters>(
  sp: URLSearchParams,
  initial: F,
  keys: (keyof F & string)[],
): F {
  const next = {...initial};
  for (const key of keys) {
    const raw = sp.get(key);
    if (raw == null || raw === '') {
      continue;
    }
    const seed = initial[key];
    if (typeof seed === 'number') {
      const n = Number(raw);
      if (Number.isFinite(n)) {
        (next as ServerListFilters)[key] = n;
      }
      continue;
    }
    if (typeof seed === 'boolean') {
      (next as ServerListFilters)[key] = raw === '1' || raw === 'true';
      continue;
    }
    (next as ServerListFilters)[key] = raw;
  }
  return next;
}

function writeSearchParams<F extends ServerListFilters>(
  current: URLSearchParams,
  opts: {
    page: number;
    pageSize: number;
    defaultPageSize: number;
    filters: F;
    filterKeys: (keyof F & string)[];
    pageKey: string;
    pageSizeKey?: string;
    initialFilters: F;
  },
): URLSearchParams {
  const next = new URLSearchParams(current.toString());
  if (opts.page <= 1) {
    next.delete(opts.pageKey);
  } else {
    next.set(opts.pageKey, String(opts.page));
  }
  if (opts.pageSizeKey) {
    if (opts.pageSize === opts.defaultPageSize) {
      next.delete(opts.pageSizeKey);
    } else {
      next.set(opts.pageSizeKey, String(opts.pageSize));
    }
  }
  for (const key of opts.filterKeys) {
    const value = opts.filters[key];
    const initial = opts.initialFilters[key];
    const empty =
      value == null ||
      value === '' ||
      (typeof initial === 'string' && value === initial && initial === '');
    // 「全部」哨兵（如 all）与初始值相同时不写 URL，避免噪音参数。
    // Skip "all"/initial sentinel values so the URL stays clean.
    if (
      empty ||
      value === initial ||
      (typeof value === 'string' && value === 'all' && (initial === 'all' || initial === ''))
    ) {
      next.delete(key);
      continue;
    }
    next.set(key, String(value));
  }
  return next;
}

interface QueryState<F extends ServerListFilters> {
  page: number;
  pageSize: number;
  filters: F;
}

export function useServerListQuery<F extends ServerListFilters>(
  options: UseServerListQueryOptions<F>,
): ServerListQuery<F> {
  const {initialFilters, defaultPageSize = 20, urlSync} = options;
  const pageKey = urlSync?.pageKey ?? 'page';
  const pageSizeKey = urlSync?.pageSizeKey;
  const filterKeys = urlSync?.filterKeys;

  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const currentSearch = searchParams.toString();

  const initialFromUrl = useMemo<QueryState<F>>(() => {
    if (!urlSync || !filterKeys) {
      return {
        page: 1,
        pageSize: defaultPageSize,
        filters: initialFilters,
      };
    }
    return {
      page: readPositiveInt(searchParams.get(pageKey), 1),
      pageSize: pageSizeKey
        ? readPositiveInt(searchParams.get(pageSizeKey), defaultPageSize)
        : defaultPageSize,
      filters: filtersFromSearchParams(searchParams, initialFilters, filterKeys),
    };
    // 仅挂载时读 URL；外部路由变更由下方 effect 同步。Only read URL on mount; external route changes sync in the effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const [state, setState] = useState<QueryState<F>>(initialFromUrl);
  // 同步持有最新状态与配置引用，保证同帧连续调用不丢更新、回调引用稳定。
  // Keep latest state and options in refs so back-to-back calls in one tick never drop updates and callbacks stay stable.
  const stateRef = useRef<QueryState<F>>(state);
  stateRef.current = state;
  const configRef = useRef({
    initialFilters,
    defaultPageSize,
    urlSync,
    filterKeys,
    pageKey,
    pageSizeKey,
    router,
    pathname,
    searchParams,
  });
  configRef.current = {
    initialFilters,
    defaultPageSize,
    urlSync,
    filterKeys,
    pageKey,
    pageSizeKey,
    router,
    pathname,
    searchParams,
  };

  const prevSearchRef = useRef(currentSearch);
  const lastWrittenSearchRef = useRef<string | null>(null);

  // 启用 urlSync 时，响应浏览器前进/后退等外部 URL 变化。
  // When urlSync is enabled, resync state on external URL changes such as browser Back/Forward.
  useEffect(() => {
    if (currentSearch === prevSearchRef.current) {
      return;
    }
    prevSearchRef.current = currentSearch;
    if (lastWrittenSearchRef.current === currentSearch) {
      lastWrittenSearchRef.current = null;
      return;
    }
    const cfg = configRef.current;
    if (!cfg.urlSync || !cfg.filterKeys) {
      return;
    }
    const next: QueryState<F> = {
      page: readPositiveInt(cfg.searchParams.get(cfg.pageKey), 1),
      pageSize: cfg.pageSizeKey
        ? readPositiveInt(
            cfg.searchParams.get(cfg.pageSizeKey),
            cfg.defaultPageSize,
          )
        : cfg.defaultPageSize,
      filters: filtersFromSearchParams(
        cfg.searchParams,
        cfg.initialFilters,
        cfg.filterKeys,
      ),
    };
    stateRef.current = next;
    setState(next);
  }, [currentSearch]);

  const applyTransition = useCallback(
    (updater: (prev: QueryState<F>, cfg: typeof configRef.current) => QueryState<F>) => {
      const cfg = configRef.current;
      const next = updater(stateRef.current, cfg);
      stateRef.current = next;
      setState(next);
      if (!cfg.urlSync || !cfg.filterKeys) {
        return;
      }
      const nextParams = writeSearchParams(cfg.searchParams, {
        page: next.page,
        pageSize: next.pageSize,
        defaultPageSize: cfg.defaultPageSize,
        filters: next.filters,
        filterKeys: cfg.filterKeys,
        pageKey: cfg.pageKey,
        pageSizeKey: cfg.pageSizeKey,
        initialFilters: cfg.initialFilters,
      });
      const qs = nextParams.toString();
      const href = qs ? `${cfg.pathname}?${qs}` : cfg.pathname;
      const current = cfg.searchParams.toString();
      if (qs !== current) {
        lastWrittenSearchRef.current = qs;
        cfg.router.replace(href, {scroll: false});
      }
    },
    [],
  );

  const setPage = useCallback(
    (next: number) => {
      applyTransition((prev) => ({
        ...prev,
        page: Math.max(1, next),
      }));
    },
    [applyTransition],
  );

  const setPageSize = useCallback(
    (next: number) => {
      applyTransition((prev) => ({
        ...prev,
        page: 1,
        pageSize: Math.max(1, next),
      }));
    },
    [applyTransition],
  );

  const patchFilters = useCallback(
    (patch: Partial<F>) => {
      applyTransition((prev) => ({
        ...prev,
        page: 1,
        filters: {...prev.filters, ...patch},
      }));
    },
    [applyTransition],
  );

  const replaceFilters = useCallback(
    (next: F) => {
      applyTransition((prev) => ({
        ...prev,
        page: 1,
        filters: next,
      }));
    },
    [applyTransition],
  );

  const resetFilters = useCallback(() => {
    applyTransition((prev, cfg) => ({
      ...prev,
      page: 1,
      filters: cfg.initialFilters,
    }));
  }, [applyTransition]);

  return {
    page: state.page,
    pageSize: state.pageSize,
    filters: state.filters,
    setPage,
    setPageSize,
    patchFilters,
    replaceFilters,
    resetFilters,
  };
}
