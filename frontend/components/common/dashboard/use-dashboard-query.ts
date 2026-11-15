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

export type HomeTab = 'cluster' | 'jobs' | 'list';

const HOME_TABS: HomeTab[] = ['cluster', 'jobs', 'list'];

/**
 * P1 默认停在作业列表：集群 / 作业大盘在 P2 / P3 交付后再改回「集群」。
 * P1 defaults to the job list; switch back to "cluster" once the P2 / P3 dashboards ship.
 */
export const DEFAULT_HOME_TAB: HomeTab = 'list';

export interface DashboardQuery {
  tab: HomeTab;
  clusterId: number | undefined;
  status: string;
  mode: string;
  q: string;
  sort: string;
  order: 'asc' | 'desc' | '';
  page: number;
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
