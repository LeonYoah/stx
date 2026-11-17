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

import type {SyncJobInstance} from '@/lib/services/sync';
import {
  getDisplayJobLifecycleStatus,
  isJobLifecycleActive,
} from '../sync-studio-utils';

/**
 * 合并工作台「已知作业」三个来源，优先级：jobs（业务判定数组，最新）→ 表格当前页 items → retained 快照。
 * 翻页或筛选把选中作业移出当前页后，selectedJob / detailJob 仍能从 retained 快照解析（父 design D5 附带风险）。
 * Merge the workbench's known-job sources by priority: `jobs` (the run-state array, freshest) → current table
 * page → retained snapshots. Selected / detail jobs keep resolving from the retained snapshot after paging or
 * filtering moves them off the page (the attached risk of D5 in the parent design).
 */
export function mergeKnownJobs(
  jobs: SyncJobInstance[],
  pageItems: SyncJobInstance[],
  retainedJobs: ReadonlyMap<number, SyncJobInstance>,
): Map<number, SyncJobInstance> {
  const merged = new Map<number, SyncJobInstance>();
  retainedJobs.forEach((job, id) => merged.set(id, job));
  for (const job of pageItems) {
    merged.set(job.id, job);
  }
  for (const job of jobs) {
    merged.set(job.id, job);
  }
  return merged;
}

/**
 * 需要补进 3s 轮询的 retained 作业 ID：给定 ID（如 selectedJobId / detailJobId）不在 jobs 里且对应作业
 * 仍活动时返回。刷新结果只回写 retained 快照，不进 jobs，避免污染运行状态判定。
 * IDs that must join the 3s poll: the given ids (e.g. selectedJobId / detailJobId) are absent from `jobs` yet
 * still active. Refresh results only write back the retained snapshot — never `jobs` — so run-state checks
 * stay unaffected.
 */
export function retainedActiveJobIds(
  knownJobs: ReadonlyMap<number, SyncJobInstance>,
  jobs: SyncJobInstance[],
  ids: ReadonlyArray<number | null>,
): number[] {
  const inJobs = new Set(jobs.map((job) => job.id));
  const result: number[] = [];
  for (const id of ids) {
    if (id === null || inJobs.has(id)) {
      continue;
    }
    const snapshot = knownJobs.get(id);
    if (
      snapshot !== undefined &&
      isJobLifecycleActive(getDisplayJobLifecycleStatus(snapshot))
    ) {
      result.push(id);
    }
  }
  return result;
}
