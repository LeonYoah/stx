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

import {useEffect, useMemo, useRef, useState} from 'react';
import services from '@/lib/services';
import type {
  SyncJobInstance,
  SyncJobListData,
  SyncJobListParams,
  SyncJobListSort,
  SyncJobStatusGroup,
} from '@/lib/services/sync';
import {usePolledResource} from '../../dashboard/use-polled-resource';
import {
  getDisplayJobLifecycleStatus,
  isJobLifecycleActive,
} from '../sync-studio-utils';
import type {JobListQuery, JobTableScope} from './types';

/**
 * 速率 / CDC 排序的后端约束：只在「运行中」组里做内存排序，其余组回退到开始时间。
 * 放在 hook 里（D8），两个宿主不用各写一遍。
 * Backend constraint for rate / CDC sorting: in-memory sort exists only inside the running group; every other
 * group falls back to started_at. Lives in the hook (D8) so neither host reimplements it.
 */
export function resolveJobListSort(
  query: Pick<JobListQuery, 'sort' | 'statusGroup'>,
): SyncJobListSort {
  if (
    (query.sort === 'rate' || query.sort === 'cdc_delay') &&
    query.statusGroup !== 'running'
  ) {
    return 'started_at';
  }
  return query.sort;
}

/**
 * 按范围与查询条件组装请求参数：固定携带 with_live / with_counts；global 排除预览运行、
 * task 的 runType 映射到 run_type / exclude_run_type。
 * Build request params from scope and query: always with_live / with_counts; the global scope excludes preview
 * runs while the task scope maps runType onto run_type / exclude_run_type.
 */
export function buildJobListParams(
  scope: JobTableScope,
  query: JobListQuery,
  pageSize: number,
): SyncJobListParams {
  const params: SyncJobListParams = {
    current: query.page,
    size: pageSize,
    status_group: query.statusGroup || undefined,
    keyword: query.keyword || undefined,
    sort: resolveJobListSort(query),
    order: query.order,
    with_counts: 1,
    with_live: 1,
  };
  // 活跃窗口：自定义起止毫秒优先，其次滚动预设；两参数与大盘其他端点同名（range/start/end）。
  // Active window: explicit start/end ms wins, then a rolling preset; the params share the names the other
  // dashboard endpoints use (range/start/end).
  if (query.activeFrom && query.activeTo) {
    params.start = query.activeFrom;
    params.end = query.activeTo;
  } else if (query.activeRange) {
    params.range = query.activeRange;
  }
  // 预览与正式运行不再按运行类型筛分，任何范围都不发 run_type / exclude_run_type。
  // Preview and formal runs are no longer filtered by run type — neither run_type nor
  // exclude_run_type is sent for any scope.
  if (scope.kind === 'all') {
    params.mode = query.mode || undefined;
    params.cluster_id = query.clusterId;
  } else {
    params.task_id = scope.taskId;
  }
  return params;
}

export interface UseJobListOptions {
  scope: JobTableScope;
  query: JobListQuery;
  pageSize: number;
  /** 无活动作业时的轮询间隔 / Poll interval while no active job is on the page. */
  pollMs: number;
  /** 本页有活动作业时的间隔（可选，缺省沿用 pollMs）/ Faster interval while the page holds active jobs. */
  activePollMs?: number;
  /** 宿主外部变化（运行 / 停止 / 恢复后）递增，变化时立即重拉 / External bump that triggers an immediate reload. */
  refreshKey?: number | string;
  /** 深链作业：不在本页时单独 getJob 置顶插入（仅 page=1 且无筛选）/ Deep-linked job pinned on top. */
  pinnedJobId?: number;
  /** 宿主手里更新的作业副本，按 id 覆盖，updated_at 新者胜出 / Fresher copies from the host, merged by id. */
  overlay?: SyncJobInstance[];
  /**
   * 范围无效时置 false（如工作台未打开任务）：不发请求、items 为空；缺省 true。
   * Set false while the scope is invalid (e.g. no task open in the workbench): no request fires and items
   * stays empty. Defaults to true.
   */
  enabled?: boolean;
}

export interface UseJobListResult {
  items: SyncJobInstance[];
  total: number;
  statusCounts: Partial<Record<SyncJobStatusGroup, number>>;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  updatedAt: number | null;
  reload: () => void;
}

/**
 * 作业列表取数：服务端分页 / 筛选 / 排序 + 轮询；合并 pinned 深链作业与宿主 overlay 副本。
 * Job list fetching: server-side paging / filtering / sorting with polling; merges a pinned deep-linked job and
 * the host's overlay copies into the returned rows.
 */
export function useJobList({
  scope,
  query,
  pageSize,
  pollMs,
  activePollMs,
  refreshKey,
  pinnedJobId,
  overlay,
  enabled = true,
}: UseJobListOptions): UseJobListResult {
  const params = buildJobListParams(scope, query, pageSize);
  // listKey 变化时 usePolledResource 清空旧数据重新加载（与原实现一致）。
  // A new listKey clears the previous page and reloads (same as the old implementation).
  const listKey = `${scope.kind}:${scope.kind === 'task' ? scope.taskId : 0}:${JSON.stringify(params)}`;
  const paramsRef = useRef(params);
  paramsRef.current = params;

  // 轮询间隔按「本页是否有活动作业」切换：用上一帧的 items 计算，避免自引用。
  // The interval depends on whether the current page holds active jobs; computed from the previous frame's items
  // to avoid self-reference.
  const itemsRef = useRef<SyncJobInstance[]>([]);
  const hasActive = itemsRef.current.some((job) =>
    isJobLifecycleActive(getDisplayJobLifecycleStatus(job)),
  );
  const intervalMs =
    hasActive && activePollMs !== undefined ? activePollMs : pollMs;

  const list = usePolledResource<SyncJobListData>(
    listKey,
    () => services.sync.listJobs(paramsRef.current),
    intervalMs,
    enabled,
  );
  const items = useMemo(() => list.data?.items ?? [], [list.data]);
  itemsRef.current = items;

  // refreshKey 变化时立即重拉（保留旧数据，不走骨架）。/ A refreshKey bump reloads in place (keeps old rows).
  const {reload: reloadList} = list;
  const refreshKeyRef = useRef(refreshKey);
  useEffect(() => {
    if (refreshKeyRef.current === refreshKey) {
      return;
    }
    refreshKeyRef.current = refreshKey;
    reloadList();
  }, [refreshKey, reloadList]);

  // pinned 深链作业：仅在第一页且无任何筛选时，若本页没有该作业则单独取回并置顶。
  // Pinned deep-linked job: only on page 1 with no filters; fetched separately when the page does not contain it.
  const [pinned, setPinned] = useState<SyncJobInstance | null>(null);
  const wantPinned = Boolean(
    pinnedJobId &&
      query.page === 1 &&
      query.statusGroup === '' &&
      query.keyword === '' &&
      list.data !== null &&
      !items.some((job) => job.id === pinnedJobId),
  );
  useEffect(() => {
    if (!wantPinned || pinnedJobId === undefined) {
      setPinned(null);
      return;
    }
    let disposed = false;
    services.sync
      .getJob(pinnedJobId)
      .then((job) => {
        if (!disposed) {
          setPinned(job);
        }
      })
      // 取不到时静默忽略（宿主自己会提示）。/ Silently ignored; the host surfaces its own error.
      .catch(() => {
        if (!disposed) {
          setPinned(null);
        }
      });
    return () => {
      disposed = true;
    };
  }, [wantPinned, pinnedJobId]);

  // overlay 合并：宿主手里更新的副本（如工作台 getJob 轮询）按 id 覆盖，updated_at 新者胜出；
  // overlay 记录不带 live / task_name（没有 with_live），覆盖时保留原行的这两个字段。
  // Overlay merge: fresher copies owned by the host (e.g. workbench getJob polling) win by updated_at; overlay
  // rows carry no live / task_name (fetched without with_live), so the original row keeps both fields.
  const merged = useMemo(() => {
    const base = pinned
      ? [pinned, ...items.filter((job) => job.id !== pinned.id)]
      : items;
    if (!overlay || overlay.length === 0) {
      return base;
    }
    const overlayById = new Map(overlay.map((job) => [job.id, job]));
    return base.map((row) => {
      const fresh = overlayById.get(row.id);
      if (!fresh) {
        return row;
      }
      const rowMs = Date.parse(row.updated_at ?? '');
      const freshMs = Date.parse(fresh.updated_at ?? '');
      if (
        Number.isFinite(freshMs) &&
        (!Number.isFinite(rowMs) || freshMs > rowMs)
      ) {
        return {...fresh, live: row.live, task_name: row.task_name};
      }
      return row;
    });
  }, [items, pinned, overlay]);

  // enabled=false 时对外呈现为空表：不发请求，也不让「加载中」骨架卡住（key 复位会把 loading 留在 true）。
  // While disabled the hook presents an empty, non-loading table: nothing is fetched and the key-reset
  // `loading: true` must not trap the table in skeleton rows.
  return {
    items: enabled ? merged : [],
    total: enabled ? (list.data?.total ?? 0) : 0,
    statusCounts: enabled ? (list.data?.status_counts ?? {}) : {},
    loading: enabled ? list.loading : false,
    refreshing: enabled ? list.refreshing : false,
    error: enabled ? list.error : null,
    updatedAt: enabled ? list.updatedAt : null,
    reload: reloadList,
  };
}
