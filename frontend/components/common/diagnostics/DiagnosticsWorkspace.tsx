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
 * 诊断工作台：错误 / 巡检 / 巡检策略 / 经验库 四个同级 Arc 标签，外壳与告警中心对齐
 * （.arc-compact + Arc Tabs + 模块导航）。巡检策略表单脏时顶层切换走未保存确认。
 * Diagnostics workspace: errors / inspections / policies / memories as four peer
 * Arc tabs, sharing the monitoring shell (.arc-compact + Arc Tabs + module nav).
 * Top-level tab switches are intercepted by the unsaved-changes dialog while a
 * policy form is dirty.
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import Link from 'next/link';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {ArrowUpRight, Server, ScanSearch, BellRing, X} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  rememberPreferredClusterId,
  isSoleDefaultCluster,
} from '@/lib/cluster-preference';
import type {
  DiagnosticsTabKey,
  DiagnosticsWorkspaceBootstrapData,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent} from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/arc/tabs/tabs';
import {WorkspaceHeader, ModuleNavTabs} from '@/components/common/layout';
import {TroubleshootingMemoryCenter} from '@/components/common/troubleshooting';
import {ErrorsPanel} from './errors/ErrorsPanel';
import {InspectionsPanel} from './inspections/InspectionsPanel';
import {PoliciesPanel} from './policies/PoliciesPanel';
// shortcut: 暂直接复用告警中心的未保存确认弹窗；告警中心组件的并行改动落地后，统一提升到
// common/layout。
// Temporary direct reuse of the monitoring unsaved-changes dialog; promote to common/layout
// once the in-flight monitoring component changes land.
import {UnsavedChangesDialog} from '../monitoring/dirty-close';

// 解析当前激活的诊断标签页（错误 / 巡检 / 巡检策略 / 经验库）
// Resolve currently active diagnostics tab (errors / inspections / policies / memories)
function resolveTab(
  tab: string | null,
  fallback: DiagnosticsTabKey = 'errors',
): DiagnosticsTabKey {
  if (
    tab === 'errors' ||
    tab === 'inspections' ||
    tab === 'policies' ||
    tab === 'memories'
  ) {
    return tab;
  }
  return fallback;
}

// 工作台二级标签由前端固定，bootstrap.default_tab 只决定默认落点。
// Secondary tabs are fixed in the frontend; bootstrap.default_tab only picks the landing tab.
const DIAGNOSTICS_TABS: DiagnosticsTabKey[] = [
  'errors',
  'inspections',
  'policies',
  'memories',
];

export function DiagnosticsWorkspace() {
  const t = useTranslations('diagnosticsCenter');
  const tCluster = useTranslations('cluster');
  const tDock = useTranslations('dock');
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  const [loading, setLoading] = useState(true);
  const [bootstrap, setBootstrap] =
    useState<DiagnosticsWorkspaceBootstrapData | null>(null);
  // 策略表单脏态由面板上报；顶层切标签时挂起目标标签走未保存确认。
  // The policies panel reports form dirty state; a dirty form suspends top-level
  // tab switches behind the unsaved-changes confirmation.
  const [policiesDirty, setPoliciesDirty] = useState(false);
  const [pendingTab, setPendingTab] = useState<DiagnosticsTabKey | null>(null);

  const activeTab = useMemo(
    () =>
      resolveTab(
        searchParams.get('tab'),
        bootstrap?.default_tab ? resolveTab(bootstrap.default_tab) : 'errors',
      ),
    [bootstrap?.default_tab, searchParams],
  );

  const selectedClusterId = searchParams.get('cluster_id') || 'all';
  const source = searchParams.get('source') || '';
  const alertId = searchParams.get('alert_id') || '';
  const groupId = searchParams.get('group_id') || '';
  const reportId = searchParams.get('report_id') || '';
  const findingId = searchParams.get('finding_id') || '';
  const taskId = searchParams.get('task_id') || '';
  const hasWorkspaceContext = Boolean(
    selectedClusterId !== 'all' ||
    source ||
    alertId ||
    groupId ||
    reportId ||
    findingId ||
    taskId,
  );

  const updateQuery = useCallback(
    (updates: Record<string, string | null>) => {
      const next = new URLSearchParams(searchParams.toString());
      Object.entries(updates).forEach(([key, value]) => {
        if (!value || value === 'all') {
          next.delete(key);
          return;
        }
        next.set(key, value);
      });

      const queryString = next.toString();
      router.replace(queryString ? `${pathname}?${queryString}` : pathname);
    },
    [pathname, router, searchParams],
  );

  const loadBootstrap = useCallback(async () => {
    setLoading(true);
    try {
      const clusterIDValue =
        selectedClusterId !== 'all'
          ? Number.parseInt(selectedClusterId, 10)
          : 0;
      const result = await services.diagnostics.getWorkspaceBootstrapSafe({
        cluster_id: clusterIDValue > 0 ? clusterIDValue : undefined,
        source: source || undefined,
        alert_id: alertId || undefined,
      });

      if (!result.success || !result.data) {
        toast.error(result.error || t('loadError'));
        setBootstrap(null);
        return;
      }
      setBootstrap(result.data);
    } finally {
      setLoading(false);
    }
  }, [alertId, selectedClusterId, source, t]);

  useEffect(() => {
    void loadBootstrap();
  }, [loadBootstrap]);

  // 仅有一个集群且 URL 未指定时，自动切到该默认集群。
  // When only one cluster exists and URL has no cluster_id, auto-scope to it.
  useEffect(() => {
    if (!bootstrap) {
      return;
    }
    if (searchParams.get('cluster_id')) {
      return;
    }
    const options = bootstrap.cluster_options || [];
    if (options.length !== 1) {
      return;
    }
    const soleId = String(options[0].cluster_id);
    rememberPreferredClusterId(soleId);
    updateQuery({cluster_id: soleId});
  }, [bootstrap, searchParams, updateQuery]);

  const selectedClusterName = useMemo(() => {
    if (!bootstrap || selectedClusterId === 'all') {
      return '';
    }
    const cluster = (bootstrap.cluster_options || []).find(
      (item) => String(item.cluster_id) === selectedClusterId,
    );
    return cluster?.cluster_name || '';
  }, [bootstrap, selectedClusterId]);
  const entrySourceLabel = useMemo(() => {
    switch (source) {
      case 'alerts':
        return t('tasks.entrySource.alerts');
      case 'inspection-finding':
        return t('tasks.entrySource.inspectionFinding');
      case 'cluster-detail':
        return t('tasks.entrySource.clusterDetail');
      case 'cluster-detail-summary':
        return t('tasks.entrySource.clusterDetailSummary');
      default:
        return source;
    }
  }, [source, t]);

  // 切换二级 Tab 并同步 URL；策略表单脏时先确认再跳。
  // Switch the secondary tab and sync the URL; a dirty policy form confirms first.
  const handleTabChange = useCallback(
    (value: string) => {
      const next = resolveTab(value);
      if (next === activeTab) {
        return;
      }
      if (activeTab === 'policies' && policiesDirty) {
        setPendingTab(next);
        return;
      }
      updateQuery({tab: next});
    },
    [activeTab, policiesDirty, updateQuery],
  );

  const commitPendingTab = useCallback(() => {
    const target = pendingTab;
    setPendingTab(null);
    setPoliciesDirty(false);
    if (target) {
      updateQuery({tab: target});
    }
  }, [pendingTab, updateQuery]);

  const clusterIdNumber =
    selectedClusterId !== 'all'
      ? Number.parseInt(selectedClusterId, 10)
      : undefined;

  return (
    <div className='arc-compact flex min-h-0 flex-1 flex-col gap-3.5'>
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        className='flex min-h-0 flex-1 flex-col'
      >
        {/* 头部：二级 Arc 标签放入 actions，与告警中心 chrome 对齐。
            Header: secondary Arc tabs in actions to match the monitoring chrome. */}
        <WorkspaceHeader
          icon={<ScanSearch />}
          title={t('title')}
          subtitle={t('subtitle')}
          tabs={
            <ModuleNavTabs
              reorderGroupId='monitoring-diagnostics'
              items={[
                {
                  key: 'monitoring',
                  label: tDock('monitoringCenter'),
                  href: '/monitoring',
                  icon: <BellRing className='size-3.5' />,
                },
                {
                  key: 'diagnostics',
                  label: t('title'),
                  href: '/diagnostics',
                  icon: <ScanSearch className='size-3.5' />,
                },
              ]}
              activeKey='diagnostics'
            />
          }
          actions={
            <TabsList>
              {DIAGNOSTICS_TABS.map((key) => (
                <TabsTrigger
                  key={key}
                  value={key}
                  data-testid={`diagnostics-tab-${key}`}
                >
                  {t(`tabs.${key}`)}
                </TabsTrigger>
              ))}
            </TabsList>
          }
        />

        {/* 集群上下文与过滤工具条（三个面板共用的集群筛选 + 来源/告警上下文）。
            Cluster context & filter bar (cluster picker plus source/alert context
            shared by the panels). */}
        <div className='flex flex-col gap-2.5 rounded-lg border bg-card/60 p-2.5 shadow-2xs sm:flex-row sm:items-center sm:justify-between'>
          <div className='flex flex-wrap items-center gap-2.5'>
            <div className='flex items-center gap-1.5'>
              <Server className='h-3.5 w-3.5 text-muted-foreground' />
              <Select
                value={selectedClusterId}
                onValueChange={(value) => {
                  if (value !== 'all') {
                    rememberPreferredClusterId(value);
                  }
                  updateQuery({
                    cluster_id: value === 'all' ? null : value,
                    source: null,
                    alert_id: null,
                    group_id: null,
                    report_id: null,
                    finding_id: null,
                    task_id: null,
                  });
                }}
              >
                <SelectTrigger className='h-8 text-xs w-[190px] bg-background'>
                  <SelectValue placeholder={t('filters.cluster')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>
                    {t('filters.allClusters')}
                  </SelectItem>
                  {(bootstrap?.cluster_options || []).map((cluster) => (
                    <SelectItem
                      key={cluster.cluster_id}
                      value={String(cluster.cluster_id)}
                    >
                      {cluster.cluster_name}
                      {isSoleDefaultCluster(
                        (bootstrap?.cluster_options || []).map((item) => ({
                          id: item.cluster_id,
                        })),
                        cluster.cluster_id,
                      )
                        ? ` · ${tCluster('defaultBadge')}`
                        : ''}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* 上下文标签组（仅来源 / 告警；作用范围已由集群下拉表达）。
                Context badges (source / alert only; the cluster picker already
                conveys the scope). */}
            <div className='flex flex-wrap items-center gap-1.5'>
              {source ? (
                <Badge
                  variant='secondary'
                  className='text-xs flex items-center gap-1'
                >
                  {t('context.source', {source: entrySourceLabel})}
                  <X
                    className='h-3 w-3 cursor-pointer opacity-70 hover:opacity-100'
                    onClick={() => updateQuery({source: null})}
                  />
                </Badge>
              ) : null}
              {alertId ? (
                <Badge
                  variant='secondary'
                  className='text-xs flex items-center gap-1'
                >
                  {t('context.alert', {id: alertId})}
                  <X
                    className='h-3 w-3 cursor-pointer opacity-70 hover:opacity-100'
                    onClick={() => updateQuery({alert_id: null})}
                  />
                </Badge>
              ) : null}
              {selectedClusterId !== 'all' ? (
                <Button
                  asChild
                  variant='ghost'
                  size='sm'
                  className='h-7 text-xs px-2 gap-1 text-primary'
                >
                  <Link href={`/clusters/${selectedClusterId}`}>
                    {t('context.goToCluster')}
                    <ArrowUpRight className='h-3 w-3' />
                  </Link>
                </Button>
              ) : null}
            </div>
          </div>

          {hasWorkspaceContext ? (
            <Button
              variant='ghost'
              size='sm'
              className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground'
              onClick={() =>
                updateQuery({
                  cluster_id: null,
                  source: null,
                  alert_id: null,
                  group_id: null,
                  report_id: null,
                  finding_id: null,
                  task_id: null,
                })
              }
            >
              <X className='mr-1 h-3 w-3' />
              {t('clearContext')}
            </Button>
          ) : null}
        </div>

        {loading && !bootstrap ? (
          <Card>
            <CardContent className='py-8 text-sm text-muted-foreground'>
              {t('loading')}
            </CardContent>
          </Card>
        ) : null}

        <TabsContent value='errors' className='min-h-0 flex-1 flex flex-col'>
          {activeTab === 'errors' ? (
            <ErrorsPanel
              clusterId={clusterIdNumber}
              clusterOptions={bootstrap?.cluster_options || []}
              groupId={groupId ? Number.parseInt(groupId, 10) : undefined}
              onSelectGroup={(value) =>
                updateQuery({group_id: value ? String(value) : null})
              }
            />
          ) : null}
        </TabsContent>
        <TabsContent value='inspections' className='min-h-0 flex-1'>
          {activeTab === 'inspections' ? (
            <InspectionsPanel
              clusterId={clusterIdNumber}
              clusterOptions={bootstrap?.cluster_options || []}
              reportId={reportId ? Number.parseInt(reportId, 10) : undefined}
              onSelectReport={(value) =>
                updateQuery({report_id: value ? String(value) : null})
              }
            />
          ) : null}
        </TabsContent>
        <TabsContent value='policies' className='min-h-0 flex-1 flex flex-col'>
          {activeTab === 'policies' ? (
            <PoliciesPanel
              clusterId={clusterIdNumber}
              clusterOptions={bootstrap?.cluster_options || []}
              onDirtyChange={setPoliciesDirty}
            />
          ) : null}
        </TabsContent>
        <TabsContent value='memories' className='min-h-0 flex-1'>
          {activeTab === 'memories' ? (
            <TroubleshootingMemoryCenter
              clusterId={clusterIdNumber}
              clusterName={selectedClusterName || undefined}
            />
          ) : null}
        </TabsContent>
      </Tabs>

      <UnsavedChangesDialog
        open={pendingTab !== null}
        onDiscard={commitPendingTab}
        onKeepEditing={() => setPendingTab(null)}
      />
    </div>
  );
}
