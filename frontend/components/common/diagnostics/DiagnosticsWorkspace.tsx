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
import Link from 'next/link';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  ArrowUpRight,
  ClipboardCheck,
  RefreshCw,
  Server,
  Settings,
  ScanSearch,
  BellRing,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
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
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {cn} from '@/lib/utils';
import {WorkspaceHeader, ModuleNavTabs} from '@/components/common/layout';
import {DiagnosticsErrorCenter} from './DiagnosticsErrorCenter';
import {DiagnosticsInspectionCenter} from './DiagnosticsInspectionCenter';
import {AutoPolicyConfigPanel} from './AutoPolicyConfigPanel';

function resolveTab(
  tab: string | null,
  fallback: DiagnosticsTabKey = 'errors',
): DiagnosticsTabKey {
  if (tab === 'errors' || tab === 'inspections') {
    return tab;
  }
  return fallback;
}

export function DiagnosticsWorkspace() {
  const t = useTranslations('diagnosticsCenter');
  const tDock = useTranslations('dock');
  const commonT = useTranslations('common');
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  const [loading, setLoading] = useState(true);
  const [bootstrap, setBootstrap] =
    useState<DiagnosticsWorkspaceBootstrapData | null>(null);
  const [autoPolicyOpen, setAutoPolicyOpen] = useState(false);

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

  const tabs = bootstrap?.tabs || [];

  return (
    <div className='space-y-4'>
      {/* 头部标题区域（复用全局 WorkspaceHeader） / Workspace Header */}
      <WorkspaceHeader
        icon={<ScanSearch />}
        title={t('title')}
        tabs={
          <ModuleNavTabs
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
        badge={
          <span className='inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground'>
            Diagnostics Center
          </span>
        }
        subtitle={t('subtitle')}
        actions={
          <>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setAutoPolicyOpen(true)}
              className='h-8 text-xs'
            >
              <Settings className='mr-1.5 h-3.5 w-3.5' />
              {t('autoPolicies.buttonLabel')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void loadBootstrap()}
              className='h-8 text-xs'
            >
              <RefreshCw
                className={cn('mr-1.5 h-3.5 w-3.5', loading && 'animate-spin')}
              />
              {commonT('refresh')}
            </Button>
          </>
        }
      />

      {/* 集群上下文与过滤工具条（紧凑型设计，提升可用空间） / Cluster Context & Filter Bar */}
      <div className='flex flex-col gap-2.5 rounded-lg border bg-card/60 p-2.5 shadow-2xs sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-wrap items-center gap-2.5'>
          <div className='flex items-center gap-1.5'>
            <Server className='h-3.5 w-3.5 text-muted-foreground' />
            <Select
              value={selectedClusterId}
              onValueChange={(value) =>
                updateQuery({
                  cluster_id: value === 'all' ? null : value,
                  source: null,
                  alert_id: null,
                  group_id: null,
                  report_id: null,
                  finding_id: null,
                  task_id: null,
                })
              }
            >
              <SelectTrigger className='h-8 text-xs w-[190px] bg-background'>
                <SelectValue placeholder={t('filters.cluster')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('filters.allClusters')}</SelectItem>
                {(bootstrap?.cluster_options || []).map((cluster) => (
                  <SelectItem
                    key={cluster.cluster_id}
                    value={String(cluster.cluster_id)}
                  >
                    {cluster.cluster_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* 上下文标签组 / Context Badges */}
          <div className='flex flex-wrap items-center gap-1.5'>
            <Badge
              variant={selectedClusterName ? 'secondary' : 'outline'}
              className='text-xs'
            >
              {selectedClusterName
                ? t('context.clusterScoped', {name: selectedClusterName})
                : t('context.global')}
            </Badge>
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

      <Tabs
        value={activeTab}
        onValueChange={(value) =>
          updateQuery({tab: resolveTab(value) as string})
        }
      >
        <TabsList className='grid w-full grid-cols-2 gap-1 p-0.5 h-8 md:w-[320px]'>
          <TabsTrigger value='errors' className='flex items-center gap-1.5 text-xs h-7'>
            <AlertTriangle className='h-3.5 w-3.5 text-amber-500' />
            <span>{t('tabs.errors')}</span>
          </TabsTrigger>
          <TabsTrigger value='inspections' className='flex items-center gap-1.5 text-xs h-7'>
            <ClipboardCheck className='h-3.5 w-3.5 text-primary' />
            <span>{t('tabs.inspections')}</span>
          </TabsTrigger>
        </TabsList>

        {loading && !bootstrap ? (
          <Card className='mt-4'>
            <CardContent className='py-8 text-sm text-muted-foreground'>
              {t('loading')}
            </CardContent>
          </Card>
        ) : null}

        {tabs.map((tab) => (
          <TabsContent key={tab.key} value={tab.key} className='mt-4'>
            {tab.key === 'errors' ? (
              <DiagnosticsErrorCenter
                clusterId={
                  selectedClusterId !== 'all'
                    ? Number.parseInt(selectedClusterId, 10)
                    : undefined
                }
                clusterName={selectedClusterName || undefined}
                groupId={groupId ? Number.parseInt(groupId, 10) : undefined}
                onSelectGroup={(value) =>
                  updateQuery({group_id: value ? String(value) : null})
                }
              />
            ) : tab.key === 'inspections' ? (
              <DiagnosticsInspectionCenter
                clusterId={
                  selectedClusterId !== 'all'
                    ? Number.parseInt(selectedClusterId, 10)
                    : undefined
                }
                clusterName={selectedClusterName || undefined}
                reportId={reportId ? Number.parseInt(reportId, 10) : undefined}
                onSelectReport={(value) =>
                  updateQuery({report_id: value ? String(value) : null})
                }
              />
            ) : null}
          </TabsContent>
        ))}
      </Tabs>

      <AutoPolicyConfigPanel
        open={autoPolicyOpen}
        onOpenChange={setAutoPolicyOpen}
        clusterOptions={bootstrap?.cluster_options || []}
      />
    </div>
  );
}
