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
 * 诊断工作台：错误 / 巡检 / 经验库 三个同级 Arc 标签，外壳与告警中心对齐
 * （.arc-compact + Arc Tabs + 模块导航）。巡检标签内部用「巡检记录 | 自动巡检」
 * 分段承载原巡检策略页；旧 ?tab=policies 链接自动改写到 tab=inspections&view=auto。
 * 集群筛选下沉到各面板工具栏的共享 Combobox，cluster_id 是唯一的 URL 事实源；
 * 自动策略表单脏时顶层切换走未保存确认。
 * Diagnostics workspace: errors / inspections / memories as three peer Arc tabs,
 * sharing the monitoring shell (.arc-compact + Arc Tabs + module nav). The
 * inspections tab hosts the old policies page behind a "records | auto" segment;
 * legacy ?tab=policies links rewrite to tab=inspections&view=auto. Cluster
 * filtering lives in each panel toolbar via a shared Combobox with cluster_id as
 * the single URL source of truth; a dirty policy form suspends top-level tab
 * switches behind the unsaved-changes confirmation.
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {ScanSearch, BellRing} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {rememberPreferredClusterId} from '@/lib/cluster-preference';
import type {
  DiagnosticsTabKey,
  DiagnosticsWorkspaceBootstrapData,
} from '@/lib/services/diagnostics';
import {Card, CardContent} from '@/components/ui/card';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/arc/tabs/tabs';
import {WorkspaceHeader, ModuleNavTabs} from '@/components/common/layout';
import {TroubleshootingMemoryCenter} from '@/components/common/troubleshooting';
import {DiagnosticsClusterFilter} from './DiagnosticsClusterFilter';
import {ErrorsPanel} from './errors/ErrorsPanel';
import {InspectionsPanel} from './inspections/InspectionsPanel';
import type {InspectionsView} from './inspections/InspectionsPanel';
import {DirtyCloseDialog} from '@/components/common/layout';

// 解析当前激活的诊断标签页（错误 / 巡检 / 经验库）；旧 policies 链接并入巡检。
// Resolve the active diagnostics tab (errors / inspections / memories); legacy
// "policies" links fold into the inspections tab.
function resolveTab(
  tab: string | null,
  fallback: DiagnosticsTabKey = 'errors',
): DiagnosticsTabKey {
  if (tab === 'policies') {
    return 'inspections';
  }
  if (tab === 'errors' || tab === 'inspections' || tab === 'memories') {
    return tab;
  }
  return fallback;
}

// 工作台二级标签由前端固定，bootstrap.default_tab 只决定默认落点。
// Secondary tabs are fixed in the frontend; bootstrap.default_tab only picks the landing tab.
const DIAGNOSTICS_TABS: DiagnosticsTabKey[] = [
  'errors',
  'inspections',
  'memories',
];

// 巡检标签内部分段：records = 巡检记录（默认，URL 省略 view），auto = 自动巡检策略。
// Segments inside the inspections tab: records = inspection reports (default,
// view omitted from the URL), auto = auto-inspection policies.
function resolveInspectionsView(view: string | null): InspectionsView {
  return view === 'auto' ? 'auto' : 'records';
}

export function DiagnosticsWorkspace() {
  const t = useTranslations('diagnosticsCenter');
  const tDock = useTranslations('dock');
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  const [loading, setLoading] = useState(true);
  const [bootstrap, setBootstrap] =
    useState<DiagnosticsWorkspaceBootstrapData | null>(null);
  // 策略表单脏态由巡检面板上报；顶层切标签时挂起目标标签走未保存确认。
  // The policies view reports form dirty state through the inspections panel; a
  // dirty form suspends top-level tab switches behind the confirmation dialog.
  const [policiesDirty, setPoliciesDirty] = useState(false);
  const [pendingTab, setPendingTab] = useState<DiagnosticsTabKey | null>(null);

  const rawTab = searchParams.get('tab');
  const activeTab = useMemo(
    () =>
      resolveTab(
        rawTab,
        bootstrap?.default_tab ? resolveTab(bootstrap.default_tab) : 'errors',
      ),
    [bootstrap?.default_tab, rawTab],
  );

  const selectedClusterId = searchParams.get('cluster_id') || 'all';
  const source = searchParams.get('source') || '';
  const alertId = searchParams.get('alert_id') || '';
  const groupId = searchParams.get('group_id') || '';
  const reportId = searchParams.get('report_id') || '';
  // finding_id / task_id 仅作为换集群时清理的 URL 键保留，不再单独消费。
  // finding_id / task_id survive only as URL keys cleared on cluster change;
  // nothing consumes them directly anymore.
  const policyId = searchParams.get('policy_id') || '';
  const autoPolicyId = searchParams.get('auto_policy_id') || '';
  const inspectionsView = resolveInspectionsView(searchParams.get('view'));

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

  // 旧 ?tab=policies 链接改写到巡检标签的自动巡检视图，其余深链参数原样保留。
  // Legacy ?tab=policies links rewrite to the auto view inside inspections;
  // every other deep-link param carries over untouched.
  useEffect(() => {
    if (rawTab === 'policies') {
      updateQuery({tab: 'inspections', view: 'auto'});
    }
  }, [rawTab, updateQuery]);

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

  // 切换二级 Tab 并同步 URL；巡检标签内策略表单脏时先确认再跳。
  // Switch the secondary tab and sync the URL; a dirty policy form inside the
  // inspections tab confirms first.
  const handleTabChange = useCallback(
    (value: string) => {
      const next = resolveTab(value);
      if (next === activeTab) {
        return;
      }
      if (activeTab === 'inspections' && policiesDirty) {
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

  // 集群筛选统一走 cluster_id；换集群时清掉指向旧集群的深链与过滤参数。
  // Cluster filtering flows through cluster_id only; switching clusters drops
  // deep links and filters that point at the previous cluster.
  const handleClusterChange = useCallback(
    (clusterId: number | undefined) => {
      if (clusterId) {
        rememberPreferredClusterId(String(clusterId));
      }
      updateQuery({
        cluster_id: clusterId ? String(clusterId) : null,
        source: null,
        alert_id: null,
        group_id: null,
        report_id: null,
        finding_id: null,
        task_id: null,
        policy_id: null,
        auto_policy_id: null,
      });
    },
    [updateQuery],
  );

  const handleViewChange = useCallback(
    (view: InspectionsView) => {
      updateQuery({view: view === 'auto' ? 'auto' : null});
    },
    [updateQuery],
  );

  // 「自动 · 规则名」徽章：跳到自动巡检视图并打开对应策略详情。
  // "Auto · <policy>" chip: jump to the auto view and open that policy detail.
  const handleOpenPolicy = useCallback(
    (id: number) => {
      updateQuery({view: 'auto', policy_id: String(id)});
    },
    [updateQuery],
  );

  // 「查看触发记录」：回巡检记录视图并按该策略过滤。
  // "View trigger records": back to the records view filtered by that policy.
  const handleViewPolicyRecords = useCallback(
    (id: number) => {
      updateQuery({
        view: null,
        policy_id: null,
        report_id: null,
        auto_policy_id: String(id),
      });
    },
    [updateQuery],
  );

  const clusterIdNumber =
    selectedClusterId !== 'all'
      ? Number.parseInt(selectedClusterId, 10)
      : undefined;
  const clusterOptions = bootstrap?.cluster_options || [];

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
              clusterOptions={clusterOptions}
              groupId={groupId ? Number.parseInt(groupId, 10) : undefined}
              onSelectGroup={(value) =>
                updateQuery({group_id: value ? String(value) : null})
              }
              onClusterChange={handleClusterChange}
            />
          ) : null}
        </TabsContent>
        <TabsContent value='inspections' className='min-h-0 flex-1'>
          {activeTab === 'inspections' ? (
            <InspectionsPanel
              clusterId={clusterIdNumber}
              clusterOptions={clusterOptions}
              reportId={reportId ? Number.parseInt(reportId, 10) : undefined}
              onSelectReport={(value) =>
                updateQuery({report_id: value ? String(value) : null})
              }
              onClusterChange={handleClusterChange}
              view={inspectionsView}
              onViewChange={handleViewChange}
              policyId={
                policyId ? Number.parseInt(policyId, 10) : undefined
              }
              onSelectPolicy={(value) =>
                updateQuery({policy_id: value ? String(value) : null})
              }
              autoPolicyId={
                autoPolicyId ? Number.parseInt(autoPolicyId, 10) : undefined
              }
              onAutoPolicyFilter={(value) =>
                updateQuery({auto_policy_id: value ? String(value) : null})
              }
              onOpenPolicy={handleOpenPolicy}
              onViewPolicyRecords={handleViewPolicyRecords}
              onDirtyChange={setPoliciesDirty}
            />
          ) : null}
        </TabsContent>
        <TabsContent value='memories' className='min-h-0 flex-1'>
          {activeTab === 'memories' ? (
            <TroubleshootingMemoryCenter
              clusterId={clusterIdNumber}
              clusterFilter={
                <DiagnosticsClusterFilter
                  value={clusterIdNumber}
                  clusterOptions={clusterOptions}
                  onChange={handleClusterChange}
                />
              }
            />
          ) : null}
        </TabsContent>
      </Tabs>

      <DirtyCloseDialog
        open={pendingTab !== null}
        onDiscard={commitPendingTab}
        onKeepEditing={() => setPendingTab(null)}
      />
    </div>
  );
}
