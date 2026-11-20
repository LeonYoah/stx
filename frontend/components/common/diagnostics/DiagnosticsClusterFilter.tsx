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

/**
 * 诊断工作台共用的集群筛选：可搜索 Combobox（集群可超 10 项，§18 选型），
 * 首项固定「全部集群」。只暴露 value/options/onChange，由宿主面板接入各自
 * 的 URL 过滤逻辑。
 * Shared diagnostics workspace cluster filter: searchable Combobox (cluster
 * lists can exceed 10, §18 selection rules) with "all clusters" pinned first.
 * Controlled via value/options/onChange so each host panel wires its own URL
 * filtering.
 */
import {useMemo} from 'react';
import {useTranslations} from 'next-intl';
import {Combobox} from '@/components/arc/combobox/combobox';
import {isSoleDefaultCluster} from '@/lib/cluster-preference';
import type {DiagnosticsClusterOption} from '@/lib/services/diagnostics';
import {cn} from '@/lib/utils';
import layout from '@/components/common/layout/list-detail.module.css';

const ALL_VALUE = '__all__';

export interface DiagnosticsClusterFilterProps {
  /** 当前集群 ID；undefined 表示全部集群。Selected cluster id; undefined means all clusters. */
  value?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** 选中集群时回调其 ID，切到「全部集群」回调 undefined。Called with the cluster id, or undefined for "all clusters". */
  onChange?: (clusterId: number | undefined) => void;
}

export function DiagnosticsClusterFilter({
  value,
  clusterOptions,
  onChange,
}: DiagnosticsClusterFilterProps) {
  const t = useTranslations('diagnosticsCenter.filters');
  const tCluster = useTranslations('cluster');

  const options = useMemo(
    () => [
      {value: ALL_VALUE, label: t('allClusters')},
      ...clusterOptions.map((cluster) => ({
        value: String(cluster.cluster_id),
        label: `${cluster.cluster_name}${
          isSoleDefaultCluster(
            clusterOptions.map((item) => ({id: item.cluster_id})),
            cluster.cluster_id,
          )
            ? ` · ${tCluster('defaultBadge')}`
            : ''
        }`,
      })),
    ],
    [clusterOptions, t, tCluster],
  );

  return (
    <div className={cn(layout.filterCtrl, layout.filterWsm)}>
      <Combobox
        label={t('cluster')}
        value={value ? String(value) : ALL_VALUE}
        onValueChange={(next) => {
          const parsed = Number.parseInt(next, 10);
          onChange?.(
            next === ALL_VALUE || !Number.isFinite(parsed) || parsed <= 0
              ? undefined
              : parsed,
          );
        }}
        displayValue={value ? undefined : t('cluster')}
        placeholder={t('cluster')}
        emptyMessage={t('noClusterMatch')}
        options={options}
        data-testid='diagnostics-cluster-filter'
      />
    </div>
  );
}
