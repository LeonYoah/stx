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

import {useTranslations} from 'next-intl';
import {EmptyState} from '@/components/arc/empty-state/empty-state';

/** 可观测性被配置关闭时接口返回的 reason。/ The reason returned when observability is switched off in config. */
const REASON_DISABLED = 'observability_disabled';

/**
 * 趋势区的统一空态（Arc EmptyState）。按接口 reason 区分两种情况：可观测性未开启（说明由服务端
 * 配置开启，前端没有开关页可引导），或监控数据暂不可用（存储未就绪 / 查询失败，稍后自动重试）。
 * 两种状态都不放跳监控页的 CTA，避免把用户带到并不存在的“监控配置”入口。说明文案可按页签传入。
 * The shared trend empty state (Arc EmptyState). It tells apart two cases by the API reason:
 * observability is off (explain it is a server-side config switch; there is no settings page to
 * link to) or monitoring data is temporarily unavailable (storage not ready / query failed,
 * retried automatically). Neither case renders a monitoring-page CTA, so users are not led to a
 * nonexistent "monitoring settings" entry. Each tab may pass its own descriptions.
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
      />
    </div>
  );
}
