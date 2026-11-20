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

import {useCallback, useMemo, useState} from 'react';
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

  const initialFromUrl = useMemo(() => {
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
    // 仅挂载时读 URL；后续由 commit 写回。Only read URL on mount; later writes go through commit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const [page, setPageState] = useState(initialFromUrl.page);
  const [pageSize, setPageSizeState] = useState(initialFromUrl.pageSize);
  const [filters, setFiltersState] = useState<F>(initialFromUrl.filters);

  const commit = useCallback(
    (nextPage: number, nextPageSize: number, nextFilters: F) => {
      setPageState(nextPage);
      setPageSizeState(nextPageSize);
      setFiltersState(nextFilters);
      if (!urlSync || !filterKeys) {
        return;
      }
      const next = writeSearchParams(searchParams, {
        page: nextPage,
        pageSize: nextPageSize,
        defaultPageSize,
        filters: nextFilters,
        filterKeys,
        pageKey,
        pageSizeKey,
        initialFilters,
      });
      const qs = next.toString();
      const href = qs ? `${pathname}?${qs}` : pathname;
      const current = searchParams.toString();
      if (qs !== current) {
        router.replace(href, {scroll: false});
      }
    },
    [
      urlSync,
      filterKeys,
      searchParams,
      defaultPageSize,
      pageKey,
      pageSizeKey,
      initialFilters,
      pathname,
      router,
    ],
  );

  const setPage = useCallback(
    (next: number) => {
      commit(Math.max(1, next), pageSize, filters);
    },
    [commit, pageSize, filters],
  );

  const setPageSize = useCallback(
    (next: number) => {
      commit(1, Math.max(1, next), filters);
    },
    [commit, filters],
  );

  const patchFilters = useCallback(
    (patch: Partial<F>) => {
      commit(1, pageSize, {...filters, ...patch});
    },
    [commit, pageSize, filters],
  );

  const replaceFilters = useCallback(
    (next: F) => {
      commit(1, pageSize, next);
    },
    [commit, pageSize],
  );

  const resetFilters = useCallback(() => {
    commit(1, pageSize, initialFilters);
  }, [commit, pageSize, initialFilters]);

  return {
    page,
    pageSize,
    filters,
    setPage,
    setPageSize,
    patchFilters,
    replaceFilters,
    resetFilters,
  };
}
