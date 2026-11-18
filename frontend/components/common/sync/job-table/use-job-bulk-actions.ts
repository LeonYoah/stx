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

import {useCallback, useState} from 'react';
import services from '@/lib/services';
import type {SyncJobInstance, SyncJobStatus} from '@/lib/services/sync';

export type JobBulkAction = 'stop' | 'recover';

const STOPPABLE: SyncJobStatus[] = ['pending', 'running', 'doing_savepoint'];
const RECOVERABLE: SyncJobStatus[] = ['failed', 'canceled'];

/** 判断作业能否执行批量动作；不能的会被跳过。/ Whether a job is eligible for a bulk action; ineligible jobs are skipped. */
export function canApplyJobBulkAction(
  action: JobBulkAction,
  job: SyncJobInstance,
): boolean {
  return (action === 'stop' ? STOPPABLE : RECOVERABLE).includes(job.status);
}

export interface JobBulkResult {
  action: JobBulkAction;
  ok: number;
  skipped: number;
  failures: {id: number; message: string}[];
}

/**
 * 批量停止 / 恢复：逐个调用现有接口（与工作台同一条路径），串行执行避免同时压向集群；结果逐条汇总，供页面内呈现。
 * Bulk stop / recover: calls the existing endpoints one by one (the same path as the workbench), serially so the
 * cluster is not hit all at once; results are collected per job for in-page display.
 */
export function useJobBulkActions(onDone?: () => void) {
  const [running, setRunning] = useState<JobBulkAction | null>(null);
  const [result, setResult] = useState<JobBulkResult | null>(null);

  const run = useCallback(
    async (
      action: JobBulkAction,
      jobs: SyncJobInstance[],
    ): Promise<JobBulkResult> => {
      setRunning(action);
      const outcome: JobBulkResult = {action, ok: 0, skipped: 0, failures: []};
      try {
        for (const job of jobs) {
          if (!canApplyJobBulkAction(action, job)) {
            outcome.skipped += 1;
            continue;
          }
          try {
            if (action === 'stop') {
              await services.sync.cancelJob(job.id, {
                stop_with_savepoint: false,
              });
            } else {
              await services.sync.recoverJob(job.id);
            }
            outcome.ok += 1;
          } catch (err) {
            outcome.failures.push({
              id: job.id,
              message: err instanceof Error ? err.message : String(err),
            });
          }
        }
      } finally {
        setRunning(null);
        setResult(outcome);
        onDone?.();
      }
      return outcome;
    },
    [onDone],
  );

  return {running, result, run, clearResult: () => setResult(null)};
}
