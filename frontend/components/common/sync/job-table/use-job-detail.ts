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
  SyncJobSummary,
} from '@/lib/services/sync';
import {
  getDisplayJobLifecycleStatus,
  isFullJobRecord,
  isJobLifecycleActive,
  mergeJobSummaryWithFull,
} from '../sync-studio-utils';

export interface UseJobDetailOptions {
  /**
   * 宿主持有的作业行：通常是 /sync/jobs 摘要行，也可能是 getJob 全量（原样采纳，不重复请求）。
   * The row held by the host — usually a /sync/jobs summary; a full record is adopted as-is without refetching.
   */
  job: SyncJobSummary | null;
  /** 详情打开开关：false 时不发 getJob，返回的仍是摘要合并视图。/ Open gate: no getJob while false; the merged summary view is still returned. */
  enabled: boolean;
  /**
   * 活动作业的 getJob 轮询间隔（毫秒）；0 / 缺省 = 不轮询，交给宿主已有的刷新通道
   * （工作台的 jobs 3s getJob 轮询会把全量记录喂进来，这里若再轮询就是重复请求）。
   * getJob poll interval for active jobs (ms); 0 / omitted = no polling — the host already refreshes
   * (the workbench's 3s jobs poll feeds full records in, so polling here would duplicate requests).
   */
  activePollMs?: number;
}

/**
 * 作业详情自持完整记录：按 jobId getJob 一次，活动作业可按需轮询；宿主传入的摘要行只做字段级合并，
 * 绝不整行替换已取到的全量（最新 ≠ 最完整）；摘要 updated_at 新于全量时重取，保证终态切换后拓扑 /
 * 指标不落回旧值。全量未到前先返回摘要视图渲染（零塌陷）。
 * Owns the full record for the job detail: one getJob per jobId, optional polling while active; incoming
 * summary rows merge field-wise and never wholesale-replace the fetched record (freshest ≠ most
 * complete); a summary whose updated_at is newer than the held record triggers a refetch so topology /
 * metrics do not fall back to stale values after a terminal switch. Until the full record lands, the
 * merged summary view is returned so the dialog renders without collapsing.
 */
export function useJobDetail({
  job,
  enabled,
  activePollMs = 0,
}: UseJobDetailOptions): SyncJobInstance | null {
  const jobId = job?.id ?? null;
  const [fullJob, setFullJob] = useState<SyncJobInstance | null>(null);
  // 已按此 updated_at 发过补充拉取，避免响应未落地时按每帧摘要重复请求。
  // A refetch was already issued for this updated_at — avoids one request per summary tick while in flight.
  const refetchedAtRef = useRef('');
  // 两条 getJob 通道（打开即取 / 摘要变新重取）可能乱序返回：只采纳不比已持有记录旧的响应。
  // The two getJob paths (fetch-on-open / refetch-on-newer-summary) can resolve out of order: only adopt a
  // response that is not older than the held record.
  const adoptFull = (next: SyncJobInstance) =>
    setFullJob((prev) => {
      if (prev && prev.id === next.id) {
        const prevTs = Date.parse(prev.updated_at || '');
        const nextTs = Date.parse(next.updated_at || '');
        if (Number.isFinite(prevTs) && Number.isFinite(nextTs) && nextTs < prevTs) {
          return prev;
        }
      }
      return next;
    });

  // 换作业时丢掉上一份全量：详情绝不把别的作业的字段带过来。
  // Drop the previous full record on job switch: the detail must never carry another job's fields.
  useEffect(() => {
    refetchedAtRef.current = '';
    setFullJob((prev) => (prev && prev.id === jobId ? prev : null));
  }, [jobId]);

  const detailJob = useMemo<SyncJobInstance | null>(
    () =>
      job
        ? mergeJobSummaryWithFull(
            job,
            fullJob && fullJob.id === job.id ? fullJob : null,
          )
        : null,
    [job, fullJob],
  );

  // 完整记录直接采纳；摘要行若 updated_at 新于已持有的全量，说明后端记录已变（如终态切换），重取详情。
  // A full record is adopted directly; a summary row newer than the held record (e.g. a terminal switch)
  // means the backend record changed, so the detail is refetched.
  useEffect(() => {
    if (!job) {
      return;
    }
    if (isFullJobRecord(job)) {
      setFullJob((prev) => {
        if (prev && prev.id === job.id) {
          const prevTs = Date.parse(prev.updated_at || '');
          const nextTs = Date.parse(job.updated_at || '');
          if (
            Number.isFinite(prevTs) &&
            Number.isFinite(nextTs) &&
            prevTs >= nextTs
          ) {
            return prev;
          }
        }
        return job as SyncJobInstance;
      });
      return;
    }
    if (!enabled || !fullJob || fullJob.id !== job.id) {
      return;
    }
    const rowTs = Date.parse(job.updated_at || '');
    const fullTs = Date.parse(fullJob.updated_at || '');
    if (
      Number.isFinite(rowTs) &&
      Number.isFinite(fullTs) &&
      rowTs > fullTs &&
      job.updated_at !== refetchedAtRef.current
    ) {
      refetchedAtRef.current = job.updated_at || '';
      void services.sync
        .getJob(job.id)
        .then(adoptFull)
        .catch(() => {
          // 瞬时失败保留上一份；下一帧更新的摘要会再次触发。/ Keep the previous copy; the next fresher summary re-triggers.
        });
    }
  }, [job, enabled, fullJob]);

  const active = detailJob
    ? isJobLifecycleActive(getDisplayJobLifecycleStatus(detailJob))
    : false;

  // 打开即取一次全量：摘要行的 result_preview 只留展示键，拓扑 / 指标 / 脚本页签需要完整字段；
  // 终态作业不再轮询，活动作业按 activePollMs 刷新（宿主无自有轮询时才开）。
  // Fetch once on open: a summary row's result_preview keeps display keys only while the topology /
  // metrics / script tabs need the full record. Terminal jobs stop here; active jobs keep polling at
  // activePollMs (only when the host has no polling of its own).
  useEffect(() => {
    if (!enabled || !jobId) {
      return;
    }
    let disposed = false;
    const load = async () => {
      try {
        const next = await services.sync.getJob(jobId);
        if (!disposed) {
          adoptFull(next);
        }
      } catch {
        // 瞬时轮询失败保留上一份数据。/ Transient poll failures keep the previous copy.
      }
    };
    void load();
    if (!active || activePollMs <= 0) {
      return () => {
        disposed = true;
      };
    }
    const timer = window.setInterval(() => {
      void load();
    }, activePollMs);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [enabled, jobId, active, activePollMs]);

  return detailJob;
}
