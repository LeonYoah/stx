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
