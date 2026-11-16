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

import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Button} from '@/components/ui/button';

/**
 * 未接入 Prometheus 时趋势区的统一空态（Arc EmptyState），附监控配置入口；说明文案按页签传入。
 * The shared trend empty state without Prometheus (Arc EmptyState), with a link to monitoring settings; each tab
 * passes its own description.
 */
export function TrendEmptyState({description}: {description?: string}) {
  const t = useTranslations('dashboard.home.clusterTab.noProm');
  return (
    <div
      className='rounded-xl border border-dashed border-border/70'
      data-testid='home-trend-empty'
    >
      <EmptyState
        title={t('title')}
        description={description ?? t('desc')}
        action={
          <Button asChild size='sm' variant='outline'>
            <Link href='/monitoring'>{t('action')}</Link>
          </Button>
        }
      />
    </div>
  );
}
