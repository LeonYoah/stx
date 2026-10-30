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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {ChevronDown, Telescope} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {ClusterWebUIEndpoints} from '@/lib/services/cluster';

// 同一集群的地址缓存 60 秒，避免作业列表里每行都请求一次。
// Cache endpoints per cluster for 60s so a job list does not request them once per row.
const ENDPOINTS_TTL_MS = 60_000;
const endpointsCache = new Map<number, {at: number; data: ClusterWebUIEndpoints}>();

/**
 * 拼出观测地址：传入引擎作业 ID 时打开作业详情（hash 路由），否则打开集群首页。
 * Build an observation URL: with an engine job ID it opens the job detail (hash route), otherwise the cluster home.
 */
export function buildObserveURL(
  baseURL: string,
  jobPathTemplate: string,
  engineJobId?: string,
): string {
  if (!baseURL) {
    return '';
  }
  const base = baseURL.endsWith('/') ? baseURL : `${baseURL}/`;
  const jobId = String(engineJobId ?? '').trim();
  if (!jobId) {
    return base;
  }
  return base + (jobPathTemplate || '#/jobs/{job_id}').replace('{job_id}', encodeURIComponent(jobId));
}

async function loadEndpoints(clusterId: number): Promise<ClusterWebUIEndpoints> {
  const cached = endpointsCache.get(clusterId);
  if (cached && Date.now() - cached.at < ENDPOINTS_TTL_MS) {
    return cached.data;
  }
  const data = await services.cluster.getWebUIEndpoints(clusterId);
  endpointsCache.set(clusterId, {at: Date.now(), data});
  return data;
}

/**
 * 通用入口"跳转到 SeaTunnel 集群观测"：可选经 STX 代理或直连原生地址，实时、离线作业和集群页都可使用。
 * 背压 DAG、指标曲线等由引擎自带的 Web UI 提供，STX 只负责跳转。
 * Shared "Open SeaTunnel cluster observation" entry: choose the STX proxy or the native address; usable for
 * streaming and batch jobs and on the cluster page. The engine's own Web UI provides the backpressure DAG and
 * metric charts; STX only links to it.
 */
export function SeaTunnelObserveMenu({
  clusterId,
  engineJobId,
  variant = 'icon',
  align = 'end',
  className,
  triggerHint,
}: {
  clusterId: number;
  engineJobId?: string;
  variant?: 'icon' | 'button';
  align?: 'start' | 'end';
  className?: string;
  /** 按钮形态下触发器的 Tooltip 说明 / Tooltip text for the trigger in button variant */
  triggerHint?: string;
}) {
  const t = useTranslations('cluster.observe');
  const [endpoints, setEndpoints] = useState<ClusterWebUIEndpoints | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleOpenChange = (open: boolean) => {
    if (!open || loading || !clusterId) {
      return;
    }
    setLoading(true);
    loadEndpoints(clusterId)
      .then((data) => {
        setEndpoints(data);
        setError(null);
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  };

  const open = (url: string) => {
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer');
    }
  };

  const proxyURL = endpoints
    ? buildObserveURL(endpoints.proxy_base_url, endpoints.job_path_template, engineJobId)
    : '';
  const nativeURL = endpoints
    ? buildObserveURL(endpoints.native_base_url, endpoints.job_path_template, engineJobId)
    : '';

  const trigger =
    variant === 'icon' ? (
      <Button
        size='icon'
        variant='ghost'
        className={cn('size-7 text-muted-foreground hover:text-foreground', className)}
        aria-label={t('open')}
        data-testid='seatunnel-observe-trigger'
        onClick={(event) => event.stopPropagation()}
      >
        <Telescope className='size-3.5' />
      </Button>
    ) : (
      <Button
        size='sm'
        variant='outline'
        className={cn('h-7 gap-1.5 px-2 text-xs', className)}
        data-testid='seatunnel-observe-trigger'
        onClick={(event) => event.stopPropagation()}
      >
        <Telescope className='size-3.5' />
        {t('open')}
        <ChevronDown className='size-3.5 opacity-70' />
      </Button>
    );

  return (
    <DropdownMenu onOpenChange={handleOpenChange}>
      {variant === 'icon' ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-xs space-y-1 text-xs leading-relaxed'>
            <p>{t('open')}</p>
            <p className='opacity-80'>{t('webUiFeaturesHint')}</p>
          </TooltipContent>
        </Tooltip>
      ) : (
        // 按钮形态：调用方的说明与「背压/实时指标」说明合并进同一个 Tooltip
        // Button variant: the caller's hint and the backpressure/live-metrics note share one tooltip
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-xs space-y-1 text-xs leading-relaxed'>
            {triggerHint ? <p>{triggerHint}</p> : null}
            <p>{t('webUiFeaturesHint')}</p>
          </TooltipContent>
        </Tooltip>
      )}
      <DropdownMenuContent
        align={align}
        className='min-w-[260px] max-w-[360px]'
        onClick={(event) => event.stopPropagation()}
      >
        <DropdownMenuLabel className='truncate text-xs'>
          {engineJobId ? t('jobTitle', {id: engineJobId}) : t('clusterTitle')}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {loading && !endpoints ? (
          <DropdownMenuItem disabled className='text-xs'>
            {t('loading')}
          </DropdownMenuItem>
        ) : error ? (
          <DropdownMenuItem disabled className='whitespace-normal text-xs text-destructive'>
            {error}
          </DropdownMenuItem>
        ) : endpoints && !endpoints.available ? (
          <DropdownMenuItem disabled className='whitespace-normal text-xs'>
            {t(`reason.${endpoints.reason || 'no_webui_node'}`)}
          </DropdownMenuItem>
        ) : endpoints ? (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuItem onClick={() => open(proxyURL)} data-testid='seatunnel-observe-proxy'>
                  <div className='flex min-w-0 flex-col gap-0.5'>
                    <span>{t('viaProxy')}</span>
                    <span className='truncate font-mono text-[11px] text-muted-foreground'>{proxyURL}</span>
                  </div>
                </DropdownMenuItem>
              </TooltipTrigger>
              <TooltipContent side='left' className='max-w-[240px] text-xs'>
                {t('viaProxyHint')}
              </TooltipContent>
            </Tooltip>
            {/* 拿不到原生地址时用 aria-disabled 而不是 disabled：方向键仍能聚焦到这一项，Tooltip 说明原因 */}
            {/* Without a native address use aria-disabled instead of disabled so arrow keys still reach the item and the tooltip explains why */}
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuItem
                  aria-disabled={!nativeURL || undefined}
                  className={cn(!nativeURL && 'cursor-not-allowed opacity-50')}
                  onSelect={(event) => {
                    if (!nativeURL) {
                      event.preventDefault();
                      return;
                    }
                    open(nativeURL);
                  }}
                  data-testid='seatunnel-observe-native'
                >
                  <div className='flex min-w-0 flex-col gap-0.5'>
                    <span>{t('native')}</span>
                    <span className='truncate font-mono text-[11px] text-muted-foreground'>{nativeURL || '-'}</span>
                  </div>
                </DropdownMenuItem>
              </TooltipTrigger>
              <TooltipContent side='left' className='max-w-[240px] text-xs'>
                {nativeURL ? t('nativeHint') : t('nativeUnavailableHint')}
              </TooltipContent>
            </Tooltip>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
