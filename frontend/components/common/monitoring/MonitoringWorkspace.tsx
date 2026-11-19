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
 * Monitoring workspace: alerts / rules / notify as three peer Arc tabs.
 * 监控工作台：告警 / 规则 / 通知 三个同级 Arc 面板（design.md §9.1 信息架构）。
 * URL 契约：tab=alerts|rules|notify；旧 tab=policies/history 与 section=* 映射到新面板；
 * cluster_id 作为告警筛选初值，rule 深链打开规则详情。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {BellRing, ScanSearch} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/arc/tabs/tabs';
import {ModuleNavTabs, WorkspaceHeader} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import {AlertsPanel} from './alerts/AlertsPanel';
import {RulesPanel} from './rules/RulesPanel';
import {NotifyPanel} from './notify/NotifyPanel';
import {useMonitoringFiringCount} from './use-firing-count';
import styles from './monitoring.module.css';

type MonitoringTab = 'alerts' | 'rules' | 'notify';

/**
 * 深链映射：新 tab 值直用；旧 policies/rules/integrations/notifications → rules，
 * 旧 history（或 section=history/deliveries/channels）→ notify。
 * Deep-link mapping: new tab values pass through; legacy policies/rules/integrations/
 * notifications → rules; legacy history or delivery/channel sections → notify.
 */
export function resolveMonitoringTab(
  tab: string | null,
  section: string | null,
): MonitoringTab {
  switch (tab) {
    case 'alerts':
      return 'alerts';
    case 'notify':
    case 'notifications':
    case 'integrations':
    case 'history':
      return 'notify';
    case 'rules':
    case 'policies':
      return section === 'history' ||
        section === 'deliveries' ||
        section === 'channels'
        ? 'notify'
        : 'rules';
    default:
      break;
  }
  if (
    section === 'history' ||
    section === 'deliveries' ||
    section === 'channels'
  ) {
    return 'notify';
  }
  return 'alerts';
}

export function MonitoringWorkspace() {
  const t = useTranslations('monitoring');
  const tMc = useTranslations('monitoringCenter');
  const tDock = useTranslations('dock');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const tabParam = searchParams.get('tab');
  const sectionParam = searchParams.get('section');
  const ruleParam = searchParams.get('rule');
  const clusterParam = searchParams.get('cluster_id');

  const tab = useMemo(
    () => resolveMonitoringTab(tabParam, sectionParam),
    [tabParam, sectionParam],
  );
  const [ruleFocus, setRuleFocus] = useState<string | null>(ruleParam);
  useEffect(() => {
    setRuleFocus(ruleParam);
  }, [ruleParam]);

  const firingCount = useMonitoringFiringCount();

  const setTab = useCallback(
    (next: string) => {
      const params = new URLSearchParams(searchParams.toString());
      params.set('tab', next);
      params.delete('section');
      if (next !== 'rules') {
        params.delete('rule');
      }
      router.replace(`${pathname}?${params.toString()}`, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  const openRule = useCallback(
    (ruleKey: string) => {
      setRuleFocus(ruleKey);
      const params = new URLSearchParams(searchParams.toString());
      params.set('tab', 'rules');
      params.set('rule', ruleKey);
      params.delete('section');
      router.replace(`${pathname}?${params.toString()}`, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  const openNotify = useCallback(() => setTab('notify'), [setTab]);

  return (
    // 容器级 token 收紧挂根节点；portal 表面（抽屉）另挂 styles.scope。
    // Container-level token tightening on the root; portal surfaces get styles.scope too.
    <div className={cn(styles.scope, 'flex min-h-0 flex-1 flex-col gap-3.5')}>
      <Tabs
        value={tab}
        onValueChange={setTab}
        className='flex min-h-0 flex-1 flex-col'
      >
        {/* 与诊断中心共用模块导航；二级面板收进 actions，沿用旧告警中心的壳。
            Shared module nav with diagnostics; secondary panels live in actions like the old workspace. */}
        <WorkspaceHeader
          icon={<BellRing />}
          title={tMc('title')}
          subtitle={tMc('subtitle')}
          tabs={
            <ModuleNavTabs
              reorderGroupId='monitoring-diagnostics'
              items={[
                {
                  key: 'monitoring',
                  label: tMc('title'),
                  href: '/monitoring',
                  icon: <BellRing className='size-3.5' />,
                },
                {
                  key: 'diagnostics',
                  label: tDock('diagnosticsCenter'),
                  href: '/diagnostics',
                  icon: <ScanSearch className='size-3.5' />,
                },
              ]}
              activeKey='monitoring'
            />
          }
          actions={
            <TabsList>
              <TabsTrigger value='alerts' data-testid='monitoring-tab-alerts'>
                {t('tabAlerts')}
                {firingCount > 0 ? (
                  <Badge size='sm' tone='danger' className='ml-1.5'>
                    {firingCount}
                  </Badge>
                ) : null}
              </TabsTrigger>
              <TabsTrigger value='rules' data-testid='monitoring-tab-rules'>
                {t('tabRules')}
              </TabsTrigger>
              <TabsTrigger value='notify' data-testid='monitoring-tab-notify'>
                {t('tabNotify')}
              </TabsTrigger>
            </TabsList>
          }
        />
        <TabsContent value='alerts' className='min-h-0 flex-1'>
          {tab === 'alerts' ? (
            <AlertsPanel
              initialClusterId={clusterParam}
              onOpenRule={openRule}
              onGoRules={() => setTab('rules')}
            />
          ) : null}
        </TabsContent>
        <TabsContent value='rules' className='min-h-0 flex-1'>
          {tab === 'rules' ? (
            <RulesPanel
              key={ruleFocus ?? 'default'}
              initialActiveKey={ruleFocus ?? undefined}
              onConfigureNotify={openNotify}
            />
          ) : null}
        </TabsContent>
        <TabsContent value='notify' className='min-h-0 flex-1'>
          {tab === 'notify' ? <NotifyPanel /> : null}
        </TabsContent>
      </Tabs>
    </div>
  );
}
