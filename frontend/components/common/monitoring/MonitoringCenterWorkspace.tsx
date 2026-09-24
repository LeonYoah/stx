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
import {BellRing, ScanSearch} from 'lucide-react';
import gsap from 'gsap';
import {useGSAP} from '@gsap/react';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {WorkspaceHeader, ModuleNavTabs} from '@/components/common/layout';
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
  const tDock = useTranslations('dock');
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

  // 切换标签页并同步更新 URL 参数；离开「规则与通知」时清理三级 section，避免脏 query
  // Switch tab and sync URL; clear tertiary section when leaving Rules & Notifications
  const handleTabChange = useCallback(
    (value: string) => {
      const nextTab = value as MonitoringTab;
      setActiveTab(nextTab);
      const params = new URLSearchParams(searchParams.toString());
      if (nextTab === 'alerts') {
        params.delete('tab');
        params.delete('section');
      } else {
        params.set('tab', nextTab);
        // 进入 policies 时保留已有 section；缺省由 PolicyCenter 处理为 rules
        // Keep existing section when entering policies; PolicyCenter defaults to rules
      }
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname);
    },
    [pathname, router, searchParams],
  );

  // 兼容旧深链 tab=history → policies + section=history；非 policies 时清理 section
  // Legacy deep-link tab=history → policies + section=history; clear section off policies
  useEffect(() => {
    const rawTab = searchParams.get('tab');
    const resolved = resolveTab(rawTab);
    const section = searchParams.get('section');
    const params = new URLSearchParams(searchParams.toString());
    let dirty = false;

    if (resolved !== 'policies' && section) {
      params.delete('section');
      dirty = true;
    }

    // 历史 alias：tab=history 映射到 policies，并默认打开投递记录分段
    // History alias: map tab=history to policies and open delivery section
    if (rawTab === 'history') {
      params.set('tab', 'policies');
      if (!section || section === 'rules') {
        params.set('section', 'history');
      }
      dirty = true;
    }

    if (dirty) {
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname);
    }
  }, [pathname, router, searchParams]);

  return (
    <div ref={workspaceRef} className='space-y-3.5 flex-1 flex flex-col'>
      {/* 页面标题栏（全局统一紧凑型设计） / Unified Compact Workspace Header */}
      <WorkspaceHeader
        title={t('title')}
        subtitle={t('subtitle')}
        icon={<BellRing />}
        tabs={
          <ModuleNavTabs
            reorderGroupId='monitoring-diagnostics'
            items={[
              {
                key: 'monitoring',
                label: t('title'),
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
          <Tabs
            value={activeTab}
            onValueChange={handleTabChange}
            className='w-full sm:w-auto'
          >
            <TabsList className='grid w-full grid-cols-2 sm:w-[320px] bg-muted/60 p-1 h-8.5'>
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
