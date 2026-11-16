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

/** 可观测性被配置关闭时接口返回的 reason。/ The reason returned when observability is switched off in config. */
const REASON_DISABLED = 'observability_disabled';

/**
 * 趋势区的统一空态（Arc EmptyState），附监控配置入口。按接口 reason 区分两种情况：
 * 可观测性未开启（提示开启），或监控数据暂不可用（存储未就绪 / 查询失败，稍后自动重试）。说明文案可按页签传入。
 * The shared trend empty state (Arc EmptyState) with a link to monitoring settings. It tells apart two cases by the API
 * reason: observability is off (prompt to enable it) or monitoring data is temporarily unavailable (storage not ready /
 * query failed, retried automatically). Each tab may pass its own descriptions.
 */
export function TrendEmptyState({
  reason,
  description,
  unavailableDescription,
}: {
  reason?: string;
  description?: string;
  unavailableDescription?: string;
}) {
  const t = useTranslations('dashboard.home.clusterTab.noProm');
  const disabled = reason === REASON_DISABLED;
  return (
    <div
      className='rounded-xl border border-dashed border-border/70'
      data-testid='home-trend-empty'
      data-reason={disabled ? 'disabled' : 'unavailable'}
    >
      <EmptyState
        title={disabled ? t('title') : t('unavailableTitle')}
        description={
          disabled
            ? (description ?? t('desc'))
            : (unavailableDescription ?? t('unavailableDesc'))
        }
        action={
          <Button asChild size='sm' variant='outline'>
            <Link href='/monitoring'>{t('action')}</Link>
          </Button>
        }
      />
    </div>
  );
}
