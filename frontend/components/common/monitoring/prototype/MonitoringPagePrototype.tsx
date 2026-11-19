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
 * Monitoring page prototype: alerts / rules / notify as three peer panels.
 * 监控页原型：告警 / 规则 / 通知 三个同级面板（design.md §9.1 信息架构）。
 */
import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/arc/badge/badge';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/arc/tabs/tabs';
import {AlertsPanel} from './AlertsPanel';
import {NotifyPanel} from './NotifyPanel';
import {MonitoringRulesPrototype} from './MonitoringRulesPrototype';
import {PROTO_ALERTS} from './alerts-mock';
import {PROTO_CLUSTER_RULES} from './rules-mock';

export function MonitoringPagePrototype() {
  const t = useTranslations('monitoringProto');
  const [tab, setTab] = useState('alerts');
  const [ruleFocus, setRuleFocus] = useState<string | null>(null);

  const firingCount = PROTO_ALERTS.filter((a) => a.status === 'firing').length;
  const enabledCount = PROTO_CLUSTER_RULES.filter((r) => r.enabled).length;
  const channelCount = PROTO_CHANNELS_COUNT;

  return (
    <div className='flex min-h-screen flex-col bg-background p-4 text-foreground'>
      <Tabs value={tab} onValueChange={setTab} className='flex min-h-0 flex-1 flex-col'>
        <div className='mb-3 flex items-center gap-3'>
          <TabsList>
            <TabsTrigger value='alerts'>
              {t('tabAlerts')}
              <Badge size='sm' tone={firingCount > 0 ? 'danger' : 'neutral'} className='ml-1.5'>
                {firingCount}
              </Badge>
            </TabsTrigger>
            <TabsTrigger value='rules'>
              {t('tabRules')}
              <Badge size='sm' className='ml-1.5'>
                {enabledCount}
              </Badge>
            </TabsTrigger>
            <TabsTrigger value='notify'>
              {t('tabNotify')}
              <Badge size='sm' className='ml-1.5'>
                {channelCount}
              </Badge>
            </TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value='alerts' className='min-h-0 flex-1'>
          <AlertsPanel
            onOpenRule={(ruleKey) => {
              setRuleFocus(ruleKey);
              setTab('rules');
            }}
          />
        </TabsContent>
        <TabsContent value='rules' className='min-h-0 flex-1'>
          <MonitoringRulesPrototype
            key={ruleFocus ?? 'default'}
            rules={PROTO_CLUSTER_RULES}
            initialActiveKey={ruleFocus ?? undefined}
          />
        </TabsContent>
        <TabsContent value='notify' className='min-h-0 flex-1'>
          <NotifyPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

// Channel count kept simple for the tab badge. / 通知面板徽标用的渠道数。
const PROTO_CHANNELS_COUNT = 4;
