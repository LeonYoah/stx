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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {BellRing, ShieldAlert} from 'lucide-react';
import gsap from 'gsap';
import {useGSAP} from '@gsap/react';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {WorkspaceHeader} from '@/components/common/layout';
import {MonitoringAlertsCenter} from './MonitoringAlertsCenter';
import {MonitoringPolicyCenter} from './MonitoringPolicyCenter';

// 注册 GSAP 核心与 React 扩展插件
// Register GSAP core and React extension plugin
if (typeof window !== 'undefined') {
  gsap.registerPlugin(useGSAP);
}

// 告警工作台支持的标签页类型
// Supported tab keys for the monitoring & alerting center workspace
type MonitoringTab = 'alerts' | 'policies';

function resolveTab(tab: string | null): MonitoringTab {
  if (tab === 'alerts') {
    return 'alerts';
  }
  if (
    tab === 'policies' ||
    tab === 'rules' ||
    tab === 'integrations' ||
    tab === 'notifications' ||
    tab === 'history'
  ) {
    return 'policies';
  }
  // 默认聚焦告警中心 / Default to alerts center
  return 'alerts';
}

export function MonitoringCenterWorkspace() {
  const t = useTranslations('monitoringCenter');
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const workspaceRef = useRef<HTMLDivElement>(null);

  const initialTab = useMemo(
    () => resolveTab(searchParams.get('tab')),
    [searchParams],
  );
  const [activeTab, setActiveTab] = useState<MonitoringTab>(initialTab);

  // GSAP 辅助动效：工作区标题与选项卡平滑淡入
  // GSAP auxiliary animation: workspace header and tabs subtle entrance
  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(
        {
          reduceMotion: '(prefers-reduced-motion: reduce)',
        },
        (context) => {
          const {reduceMotion} = context.conditions as {reduceMotion: boolean};
          if (reduceMotion) {
            return;
          }

          gsap.from('.workspace-header-animate', {
            opacity: 0,
            y: -6,
            duration: 0.3,
            ease: 'power2.out',
            clearProps: 'opacity,transform',
          });
        },
      );
    },
    {scope: workspaceRef},
  );

  useEffect(() => {
    setActiveTab(resolveTab(searchParams.get('tab')));
  }, [searchParams]);

  // 切换标签页并同步更新 URL 参数
  // Switch tab and sync with URL query parameters
  const handleTabChange = useCallback(
    (value: string) => {
      const nextTab = value as MonitoringTab;
      setActiveTab(nextTab);
      const params = new URLSearchParams(searchParams.toString());
      if (nextTab === 'alerts') {
        params.delete('tab');
      } else {
        params.set('tab', nextTab);
      }
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname);
    },
    [pathname, router, searchParams],
  );

  return (
    <div ref={workspaceRef} className='space-y-3.5 flex-1 flex flex-col'>
      {/* 页面标题栏（全局统一紧凑型设计） / Unified Compact Workspace Header */}
      <WorkspaceHeader
        title={t('title')}
        subtitle={t('subtitle')}
        icon={<BellRing />}
        badge={
          <span className='inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground'>
            <ShieldAlert className='h-3 w-3' />
            Alerts & Policies
          </span>
        }
        actions={
          <Tabs
            value={activeTab}
            onValueChange={handleTabChange}
            className='w-full sm:w-auto'
          >
            <TabsList className='grid w-full grid-cols-2 sm:w-[280px] bg-muted/60 p-1 h-8.5'>
              <TabsTrigger
                value='alerts'
                className='data-[state=active]:bg-background data-[state=active]:shadow-xs text-xs font-medium py-1'
              >
                {t('tabs.alerts')}
              </TabsTrigger>
              <TabsTrigger
                value='policies'
                className='data-[state=active]:bg-background data-[state=active]:shadow-xs text-xs font-medium py-1'
              >
                {t('tabs.policies')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        }
      />

      {/* 标签页主体内容 / Tab Contents */}
      <div className='flex-1 flex flex-col'>
        {activeTab === 'alerts' ? (
          <MonitoringAlertsCenter />
        ) : (
          <MonitoringPolicyCenter />
        )}
      </div>
    </div>
  );
}
