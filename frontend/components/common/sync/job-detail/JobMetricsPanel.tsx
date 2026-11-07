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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {MetricsDialogContent} from '../StudioDialogs';
import type {SyncJobInstance} from '@/lib/services/sync';
import {MetricsTrendView} from './MetricsTrendView';

type MetricsView = 'snapshot' | 'trend';

/**
 * 指标页签：顶部「快照 | 趋势」分段；快照复用现有指标 UI，趋势走 Prometheus 时序。
 * Metrics tab: top "Snapshot | Trend" segment; snapshot reuses the existing metrics UI, trend uses Prometheus timeseries.
 */
export function JobMetricsPanel({job, active}: {job: SyncJobInstance | null; active: boolean}) {
  const t = useTranslations('workbenchStudio.jobDetail.metricsView');
  const [view, setView] = useState<MetricsView>('snapshot');

  return (
    <div className='flex h-full min-h-0 flex-col gap-2'>
      <div className='flex shrink-0 items-center gap-3'>
        <SegmentedControl
          label={`${t('snapshot')} / ${t('trend')}`}
          value={view}
          onValueChange={(value) => setView(value as MetricsView)}
          options={[
            {value: 'snapshot', label: t('snapshot')},
            {value: 'trend', label: t('trend')},
          ]}
        />
        <span className='text-xs text-muted-foreground'>
          {view === 'snapshot' ? t('snapshotHint') : t('trendHint')}
        </span>
      </div>
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
