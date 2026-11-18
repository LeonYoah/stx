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

import {buildWorkbenchDeepLinkQuery} from '@/components/common/sync/task-alert-utils';
import {formatCompactNumber} from '@/components/common/dashboard/dashboard-format';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {SyncJobInstance} from '@/lib/services/sync';

/**
 * 作业显示名：提交时的作业名优先，其次任务名，最后用实例 ID（与后端 jobDisplayName 一致）。
 * Job display name: the submitted job name first, then the task name, then the instance ID (matches the backend's
 * jobDisplayName).
 */
export function jobDisplayName(
  job: Pick<SyncJobInstance, 'id' | 'submit_spec' | 'task_name'>,
): string {
  const submitted = job.submit_spec?.job_name;
  if (typeof submitted === 'string' && submitted.trim()) {
    return submitted.trim();
  }
  return job.task_name?.trim() || `#${job.id}`;
}

/**
 * 工作台作业深链（与告警通知同一格式）：/workbench?task=<id>&job=<id>。
 * Workbench job deep link (same format as alert notifications): /workbench?task=<id>&job=<id>.
 */
export function jobWorkbenchHref(job: {
  task_id?: number;
  id?: number;
  job_id?: number;
}): string {
  const query = buildWorkbenchDeepLinkQuery({
    task: job.task_id ? String(job.task_id) : undefined,
    job: String(job.job_id ?? job.id ?? ''),
  });
  return query ? `/workbench?${query}` : '/workbench';
}

/** 运行中组（速率 / 延迟排序只在这里有意义）/ The running group (rate / delay sorts only make sense here). */
export function isRunningStatus(status: string): boolean {
  return status === 'running' || status === 'doing_savepoint';
}

/**
 * 列表展示用的开始 / 结束时刻（毫秒）。结束时间早于开始时间（引擎时区配置错位，如节点跑 UTC 而 STX 按显示时区解析）时
 * 退回 updated_at，与作业详情「作业全程」窗口一致；运行中没有结束时间。
 * Start / end instants (ms) for the list. When the finish time precedes the start (an engine timezone mismatch, e.g.
 * the node runs UTC while STX parses with the display zone) fall back to updated_at, matching the job detail lifetime
 * window; running jobs have no end.
 */
export function jobTimeBounds(
  job: Pick<
    SyncJobInstance,
    'started_at' | 'created_at' | 'finished_at' | 'updated_at' | 'status'
  >,
): {startMs: number | null; endMs: number | null} {
  const parse = (value?: string) => {
    const ms = value ? Date.parse(value) : Number.NaN;
    return Number.isFinite(ms) ? ms : null;
  };
  const startMs = parse(job.started_at) ?? parse(job.created_at);
  if (isRunningStatus(job.status) || !job.finished_at) {
    return {startMs, endMs: null};
  }
  const finished = parse(job.finished_at);
  const updated = parse(job.updated_at);
  const after = (ms: number | null) =>
    ms !== null && (startMs === null || ms >= startMs) ? ms : null;
  return {startMs, endMs: after(finished) ?? after(updated)};
}

/**
 * 作业时长（秒）：结束时间已知时用开始→结束；运行中用开始→nowMs；耗时未知（无开始 / 无基准时刻）返回 0。
 * Job duration in seconds: start→end when an end exists, start→nowMs while running; 0 when the duration is unknown
 * (no start or no base instant).
 */
export function jobDurationSeconds(
  job: Pick<
    SyncJobInstance,
    'started_at' | 'created_at' | 'finished_at' | 'updated_at' | 'status'
  >,
  nowMs: number | null,
): number {
  const {startMs, endMs} = jobTimeBounds(job);
  const untilMs = endMs ?? (isRunningStatus(job.status) ? nowMs : null);
  if (startMs === null || untilMs === null || untilMs <= startMs) {
    return 0;
  }
  return (untilMs - startMs) / 1000;
}

/**
 * 列表的读 / 写速率：运行中用对账快照里的当前速率；已结束用运行期间的平均速率（累计条数 ÷ 耗时秒数），
 * average=true 时界面加「均」并用浅色，避免和实时速率混淆。耗时未知时不估算。
 * Read / write rates for the list: running jobs use the current rate from the reconcile snapshot; finished jobs use the
 * average over their run (rows ÷ duration seconds), flagged average=true so the UI marks it and mutes it. No estimate
 * when the duration is unknown.
 */
export function jobListRates(
  running: boolean,
  live: SyncJobInstance['live'],
  durationSeconds: number,
): {read?: number; write?: number; average: boolean} {
  if (running) {
    return {read: live?.read_qps, write: live?.write_qps, average: false};
  }
  if (!live || durationSeconds <= 0) {
    return {average: true};
  }
  const avg = (total?: number) =>
    total === undefined ? undefined : total / durationSeconds;
  return {
    read: avg(live.read_total),
    write: avg(live.write_total),
    average: true,
  };
}

/**
 * 发起人显示名：本人返回当前用户名，其次查用户列表的 username / nickname，再退到 #uid。
 * Initiator display name: the current user's name for own jobs, then the user list's username / nickname,
 * finally "#uid".
 */
export function resolveJobInitiatorName(
  job: Pick<SyncJobInstance, 'created_by'>,
  currentUserId?: number,
  currentUsername?: string,
  workspaceUsers: NotificationRecipientUser[] = [],
): string {
  if (!job.created_by) {
    return '-';
  }
  if (currentUserId && job.created_by === currentUserId) {
    return currentUsername || 'admin';
  }
  const matched = workspaceUsers.find((user) => user.id === job.created_by);
  if (matched?.username) {
    return matched.username;
  }
  if (matched?.nickname) {
    return matched.nickname;
  }
  if (job.created_by === 1) {
    return 'admin';
  }
  return `User #${job.created_by}`;
}

/**
 * 速率对「读 / 写」格式：compact 数字带 /s 单位；两侧都未知时返回 '-'。
 * Read / write rate pair in compact notation with a /s suffix; '-' when both sides are unknown.
 */
export function formatRatePair(
  read: number | undefined,
  write: number | undefined,
): string {
  if (read === undefined && write === undefined) {
    return '-';
  }
  return `${formatCompactNumber(read)} / ${formatCompactNumber(write)}/s`;
}

/**
 * 累计条数对「读 / 写」格式：compact 数字；两侧都未知时返回 '-'。
 * Read / write totals pair in compact notation; '-' when both sides are unknown.
 */
export function formatTotalPair(
  read: number | undefined,
  write: number | undefined,
): string {
  if (read === undefined && write === undefined) {
    return '-';
  }
  return `${formatCompactNumber(read)} / ${formatCompactNumber(write)}`;
}
