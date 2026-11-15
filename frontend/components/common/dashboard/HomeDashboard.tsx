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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {RefreshCw} from 'lucide-react';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/arc/tabs/tabs';
import {STXMark} from '@/components/icons/logo';
import {WorkspaceHeader} from '@/components/common/layout';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import services from '@/lib/services';
import {HomeService, type HomeAttentionItem} from '@/lib/services/dashboard';
import {formatTimeTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {AttentionStrip} from './AttentionStrip';
import {ClusterTab} from './ClusterTab';
import {DashboardKpiRow} from './DashboardKpiRow';
import {JobListTab} from './JobListTab';
import {jobWorkbenchHref} from './job-list-utils';
import {
  HOME_RANGES,
  type HomeTab,
  useDashboardQuery,
} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';

const SUMMARY_POLL_MS = 30_000;

/**
 * 控制台首页：范围选择 + 指标卡 + 「需要处理」条（三个页签共用），页签为集群 / 作业 / 作业列表。
 * P1 中集群页签暂放 Grafana 折叠面板，作业页签为占位，P2 / P3 替换。
 * The console home: scope picker, KPI cards and the "needs attention" strip shared by all tabs; tabs are clusters /
 * jobs / job list. In P1 the cluster tab still hosts the collapsed Grafana panel and the jobs tab is a placeholder;
 * P2 / P3 replace them.
 */
export function HomeDashboard() {
  const t = useTranslations('dashboard.home');
  const {query, update} = useDashboardQuery();
  const [clusters, setClusters] = useState<{id: number; name: string}[]>([]);

  useEffect(() => {
    let disposed = false;
    services.cluster.getClustersSafe({current: 1, size: 100}).then((result) => {
      if (!disposed && result.success && result.data) {
        setClusters(
          result.data.clusters.map((item) => ({id: item.id, name: item.name})),
        );
      }
    });
    return () => {
      disposed = true;
    };
  }, []);
  const clusterNames = useMemo(
    () => new Map(clusters.map((item) => [item.id, item.name])),
    [clusters],
  );

  const summary = usePolledResource(
    String(query.clusterId ?? 'all'),
    () => HomeService.getSummary({cluster_id: query.clusterId}),
    SUMMARY_POLL_MS,
  );

  /** 指标卡跳转只换页签与筛选，保留当前范围。/ KPI links switch tab and filters while keeping the scope. */
  const buildHref = useCallback(
    (patch: Record<string, string>) => {
      const next = new URLSearchParams();
      if (query.clusterId) {
        next.set('cluster', String(query.clusterId));
      }
      for (const [key, value] of Object.entries(patch)) {
        next.set(key, value);
      }
      return `/dashboard?${next.toString()}`;
    },
    [query.clusterId],
  );

  const partial = summary.data?.partial_errors ?? [];
  const scopeValue = query.clusterId ? String(query.clusterId) : 'all';

  return (
    <div className='flex flex-col gap-4 pb-28' data-testid='home-dashboard'>
      <WorkspaceHeader
        icon={<STXMark className='size-8 object-contain' />}
        iconTint={false}
        title={t('title')}
        actions={
          <div className='flex flex-wrap items-center gap-2'>
            {summary.updatedAt ? (
              <span className='hidden whitespace-nowrap text-xs text-muted-foreground sm:inline'>
                {t('updatedAt', {time: formatTimeTz(summary.updatedAt)})}
              </span>
            ) : null}
            {/* 时间窗只影响趋势，作业列表页签不显示。/ The range only drives trends, so the job list hides it. */}
            {query.tab !== 'list' ? (
              <SegmentedControl
                label={t('clusterTab.range.label')}
                value={query.range}
                onValueChange={(value) =>
                  update({range: value === '1h' ? undefined : value})
                }
                options={HOME_RANGES.map((value) => ({
                  value,
                  label: t(`clusterTab.range.${value}`),
                }))}
              />
            ) : null}
            <Select
              value={scopeValue}
              onValueChange={(value) =>
                update({
                  clusterId: value === 'all' ? undefined : value,
                  page: 1,
                })
              }
            >
              <SelectTrigger
                size='sm'
                className='h-8.5 min-w-40 text-xs'
                aria-label={t('scope.label')}
                data-testid='home-scope'
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('scope.all')}</SelectItem>
                {clusters.map((item) => (
                  <SelectItem key={item.id} value={String(item.id)}>
                    {item.name}
                  </SelectItem>
                ))}
                {query.clusterId && !clusterNames.has(query.clusterId) ? (
                  <SelectItem value={String(query.clusterId)}>
                    {t('scope.clusterFallback', {id: query.clusterId})}
                  </SelectItem>
                ) : null}
              </SelectContent>
            </Select>
            <Button
              variant='outline'
              size='sm'
              onClick={summary.reload}
              disabled={summary.refreshing}
              className='h-8.5 text-xs active:scale-[0.98]'
            >
              <RefreshCw
                className={cn(
                  'mr-1.5 size-3.5',
                  summary.refreshing && 'animate-spin',
                )}
              />
              {t('refresh')}
            </Button>
          </div>
        }
      />

      {summary.error && !summary.data ? (
        <p
          className='rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive'
          role='alert'
        >
          {t('loadError')}: {summary.error}
        </p>
      ) : null}
      {partial.length > 0 ? (
        <p
          className='rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-1.5 text-xs text-amber-600 dark:text-amber-400'
          role='status'
        >
          {t('partial', {
            blocks: partial.map((block) => t(`blocks.${block}`)).join(' / '),
          })}
        </p>
      ) : null}

      <DashboardKpiRow
        summary={summary.data}
        loading={summary.loading}
        buildHref={buildHref}
      />
      <AttentionStrip
        items={summary.data?.attention ?? []}
        jobHref={(item: HomeAttentionItem) =>
          jobWorkbenchHref({task_id: item.task_id, job_id: item.job_id})
        }
      />

      <Tabs
        value={query.tab}
        onValueChange={(value) => update({tab: value as HomeTab})}
        className='flex flex-col gap-3'
      >
        <TabsList aria-label={t('title')}>
          <TabsTrigger value='cluster'>{t('tabs.cluster')}</TabsTrigger>
          <TabsTrigger value='jobs'>{t('tabs.jobs')}</TabsTrigger>
          <TabsTrigger value='list'>{t('tabs.list')}</TabsTrigger>
        </TabsList>
        <TabsContent value='cluster'>
          <ClusterTab query={query} update={update} />
        </TabsContent>
        <TabsContent value='jobs'>
          <EmptyState
            title={t('jobsTab.comingTitle')}
            description={t('jobsTab.comingDesc')}
            action={
              <Button
                size='sm'
                variant='outline'
                onClick={() => update({tab: 'list'})}
              >
                {t('jobsTab.openList')}
              </Button>
            }
          />
        </TabsContent>
        <TabsContent value='list'>
          <JobListTab
            query={query}
            update={update}
            clusterNames={clusterNames}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
