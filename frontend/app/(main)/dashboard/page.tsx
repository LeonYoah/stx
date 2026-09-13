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

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {motion} from 'motion/react';
import {Card, CardHeader, CardTitle} from '@/components/ui/card';
import {Server, Database, Activity, Layers, RefreshCw} from 'lucide-react';
import {OverviewService, OverviewData} from '@/lib/services/dashboard';
import {MonitoringOverview} from '@/components/common/monitoring';
import {Button} from '@/components/ui/button';
import Link from 'next/link';
import {STXMark} from '@/components/icons/logo';

export default function DashboardPage() {
  const t = useTranslations('dashboard');
  const [data, setData] = useState<OverviewData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = async () => {
    setLoading(true);
    const result = await OverviewService.getOverviewDataSafe();
    if (result.success && result.data) {
      setData(result.data);
      setError(null);
    } else {
      setError(result.error || 'Failed to fetch data');
    }
    setLoading(false);
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

  const stats = data?.stats;

  const statCards = [
    {
      title: t('totalHosts'),
      value: stats?.total_hosts ?? 0,
      subValue: `${stats?.online_hosts ?? 0} ${t('online')}`,
      icon: Server,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
      href: '/hosts',
    },
    {
      title: t('totalClusters'),
      value: stats?.total_clusters ?? 0,
      subValue: `${stats?.running_clusters ?? 0} ${t('running')}`,
      icon: Database,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
      href: '/clusters',
    },
    {
      title: t('totalNodes'),
      value: stats?.total_nodes ?? 0,
      subValue: `${stats?.running_nodes ?? 0} ${t('running')}`,
      icon: Layers,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
      href: '/clusters',
    },
    {
      title: t('onlineAgents'),
      value: `${stats?.online_agents ?? 0}/${stats?.total_agents ?? 0}`,
      subValue: stats?.total_agents
        ? t('onlineRate', {
            rate: Math.round((stats.online_agents / stats.total_agents) * 100),
          })
        : t('noAgent'),
      icon: Activity,
      color: 'text-primary',
      bgColor: 'bg-primary/10',
      href: '/hosts',
    },
  ];

  return (
    <div className='space-y-3'>
      <motion.div
        initial={{opacity: 0, y: -20}}
        animate={{opacity: 1, y: 0}}
        transition={{duration: 0.5}}
        className='flex items-center justify-between'
      >
        <div className='flex items-center gap-2.5'>
          <STXMark className='size-8 rounded-lg ring-1 ring-black/5 dark:ring-white/10' />
          <div>
            <h1 className='text-lg font-bold leading-tight'>{t('title')}</h1>
            <p className='text-xs text-muted-foreground'>{t('subtitle')}</p>
          </div>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={fetchData}
          disabled={loading}
        >
          <RefreshCw
            className={`w-4 h-4 mr-2 ${loading ? 'animate-spin' : ''}`}
          />
          {t('refresh')}
        </Button>
      </motion.div>

      {error && (
        <div className='rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-500'>
          {error}
        </div>
      )}

      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-4'>
        {statCards.map((stat, index) => (
          <motion.div
            key={stat.title}
            initial={{opacity: 0, y: 20}}
            animate={{opacity: 1, y: 0}}
            transition={{delay: index * 0.1, duration: 0.5}}
          >
            <Link href={stat.href}>
              <Card className='cursor-pointer border-border/70 hover:shadow-sm transition-shadow'>
                <CardHeader className='flex min-h-[72px] flex-row items-center justify-between space-y-0 px-4 py-3'>
                  <div className='min-w-0 space-y-1'>
                    <CardTitle className='truncate text-[11px] font-medium uppercase tracking-wide text-muted-foreground/90'>
                      {stat.title}
                    </CardTitle>
                    <div className='text-lg font-semibold leading-none'>
                      {stat.value}
                    </div>
                    <p className='truncate text-[11px] text-muted-foreground'>
                      {stat.subValue}
                    </p>
                  </div>
                  <div className={`rounded-md p-1.5 ${stat.bgColor}`}>
                    <stat.icon className={`h-3.5 w-3.5 ${stat.color}`} />
                  </div>
                </CardHeader>
              </Card>
            </Link>
          </motion.div>
        ))}
      </div>

      <motion.div
        initial={{opacity: 0, y: 20}}
        animate={{opacity: 1, y: 0}}
        transition={{delay: 0.35, duration: 0.5}}
      >
        <MonitoringOverview compact />
      </motion.div>
    </div>
  );
}
