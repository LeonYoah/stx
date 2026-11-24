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

import {useEffect, useMemo, useState} from 'react';
import services from '@/lib/services';
import type {SyncJobSparklines, SyncJobSummary} from '@/lib/services/sync';
import {isRunningStatus} from './job-table-format';

/**
 * 本页运行中作业的速率迷你曲线：每次列表刷新后用一次批量请求取回（Prometheus）。
 * 数据源不可用时返回 null，调用方据此隐藏曲线列。
 * Rate sparklines for the running jobs on this page: one batched request after each list refresh (Prometheus).
 * Returns null when the data source is unavailable so callers hide the sparkline.
 */
export function useJobSparklines(
  items: SyncJobSummary[],
  updatedAt: number | null,
): SyncJobSparklines | null {
  const [sparklines, setSparklines] = useState<SyncJobSparklines | null>(null);
  const runningIds = useMemo(
    () =>
      items.filter((job) => isRunningStatus(job.status)).map((job) => job.id),
    [items],
  );
  const sparkKey = runningIds.join(',');
  useEffect(() => {
    if (!sparkKey) {
      setSparklines(null);
      return;
    }
    let disposed = false;
    services.sync
      .getJobSparklines({ids: sparkKey.split(',').map(Number), range: '1h'})
      .then((data) => {
        if (!disposed) {
          setSparklines(data);
        }
      })
      .catch(() => {
        if (!disposed) {
          setSparklines(null);
        }
      });
    return () => {
      disposed = true;
    };
  }, [sparkKey, updatedAt]);
  return sparklines;
}
