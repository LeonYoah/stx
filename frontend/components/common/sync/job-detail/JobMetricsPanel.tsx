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

import {MetricsDialogContent} from '../StudioDialogs';
import type {SyncJobInstance} from '@/lib/services/sync';
import {MetricsTrendView} from './MetricsTrendView';

export type MetricsView = 'snapshot' | 'trend';

/**
 * 指标页签内容。快照 / 趋势切换放在详情页签同一行右侧，避免再叠一条分段条。
 * Metrics body. Snapshot / trend sits on the detail tab row, so it does not stack as a second strip.
 */
export function JobMetricsPanel({
  job,
  active,
  view,
}: {
  job: SyncJobInstance | null;
  active: boolean;
  view: MetricsView;
}) {
  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='min-h-0 flex-1 overflow-auto'>
        {view === 'snapshot' ? (
          <MetricsDialogContent job={job} />
        ) : (
          <MetricsTrendView job={job} active={active && view === 'trend'} />
        )}
      </div>
    </div>
  );
}
