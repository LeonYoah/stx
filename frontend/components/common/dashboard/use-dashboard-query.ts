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

import {useCallback, useMemo} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';

export type HomeTab = 'cluster' | 'jobs' | 'hosts' | 'list';

/** 趋势时间窗 / Trend time range. */
export const HOME_RANGES = ['15m', '1h', '6h', '24h', '7d', '30d', '90d', '1y'] as const;
export type HomeRange = (typeof HOME_RANGES)[number];

const HOME_TABS: HomeTab[] = ['cluster', 'jobs', 'hosts', 'list'];

/**
 * 默认停在集群大盘（P2 起）。
 * Defaults to the cluster dashboard (from P2 on).
 */
export const DEFAULT_HOME_TAB: HomeTab = 'cluster';

export interface DashboardQuery {
  tab: HomeTab;
  clusterId: number | undefined;
  status: string;
  mode: string;
  q: string;
  sort: string;
  order: 'asc' | 'desc' | '';
  page: number;
  range: HomeRange;
  /** 自定义时间窗（毫秒）；有值时优先于 range。/ Custom window (ms); wins over range when set. */
  start: number | undefined;
  end: number | undefined;
}

/**
 * 首页的页签、范围与列表筛选都放在 URL 上，刷新和分享不丢。
 * The home tab, scope and list filters live in the URL so a refresh or shared link keeps them.
 */
export function useDashboardQuery() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const query = useMemo<DashboardQuery>(() => {
    const tab = searchParams.get('tab') as HomeTab | null;
    const cluster = Number(searchParams.get('cluster'));
    const page = Number(searchParams.get('page'));
    const order = searchParams.get('order');
    return {
      tab: tab && HOME_TABS.includes(tab) ? tab : DEFAULT_HOME_TAB,
      clusterId: Number.isInteger(cluster) && cluster > 0 ? cluster : undefined,
      status: searchParams.get('status') ?? '',
      mode: searchParams.get('mode') ?? '',
      q: searchParams.get('q') ?? '',
      sort: searchParams.get('sort') ?? '',
      order: order === 'asc' || order === 'desc' ? order : '',
      page: Number.isInteger(page) && page > 1 ? page : 1,
      range: (HOME_RANGES as readonly string[]).includes(
        searchParams.get('range') ?? '',
      )
        ? (searchParams.get('range') as HomeRange)
        : '1h',
      ...customWindow(searchParams.get('start'), searchParams.get('end')),
    };
  }, [searchParams]);

  /** 合并更新查询参数；空值删除该参数。/ Merge query updates; empty values drop the key. */
  const update = useCallback(
    (
      patch: Partial<Record<keyof DashboardQuery, string | number | undefined>>,
    ) => {
      const next = new URLSearchParams(searchParams.toString());
      const keyMap: Record<keyof DashboardQuery, string> = {
        tab: 'tab',
        clusterId: 'cluster',
        status: 'status',
        mode: 'mode',
        q: 'q',
        sort: 'sort',
        order: 'order',
        page: 'page',
        range: 'range',
        start: 'start',
        end: 'end',
      };
      for (const [field, value] of Object.entries(patch)) {
        const name = keyMap[field as keyof DashboardQuery];
        if (
          value === undefined ||
          value === '' ||
          (field === 'page' && Number(value) <= 1)
        ) {
          next.delete(name);
        } else {
          next.set(name, String(value));
        }
      }
      const search = next.toString();
      router.replace(search ? `${pathname}?${search}` : pathname, {
        scroll: false,
      });
    },
    [pathname, router, searchParams],
  );

  return {query, update};
}

/** 自定义时间窗上限；统一定义在 lib/time-window。/ Custom window cap; defined once in lib/time-window. */
export {MAX_CUSTOM_WINDOW_MS} from '@/lib/time-window';

/**
 * 解析 URL 里的自定义时间窗：两端都是正整数且结束晚于开始才算有效，否则视为未设置。
 * Parse the custom window in the URL: valid only when both ends are positive integers and end is after start.
 */
function customWindow(
  startRaw: string | null,
  endRaw: string | null,
): {start: number | undefined; end: number | undefined} {
  const start = Number(startRaw);
  const end = Number(endRaw);
  if (
    Number.isInteger(start) &&
    Number.isInteger(end) &&
    start > 0 &&
    end > start
  ) {
    return {start, end};
  }
  return {start: undefined, end: undefined};
}
