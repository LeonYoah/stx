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

import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {HelpCircle, Radio, RefreshCw} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Skeleton} from '@/components/ui/skeleton';
import {TableLoadingBar} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import {formatDateTimeTz} from '@/lib/timezone';
import services from '@/lib/services';
import type {SyncEventChannelState, SyncEventChannelStatus} from '@/lib/services/sync';

type PendingAction = 'enable' | 'disable' | 'rotate' | null;

/**
 * 事件通道状态配色。
 * Event channel state tone.
 */
export function eventChannelStateClass(state: SyncEventChannelState): string {
  switch (state) {
    case 'healthy':
      return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300';
    case 'degraded':
      return 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300';
    case 'pending_restart':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300';
    default:
      return 'border-border/60 bg-muted/40 text-muted-foreground';
  }
}

/**
 * 集群的 SeaTunnel 事件上报通道卡片：查看状态，开启、关闭、轮换 token（会改写 seatunnel.yaml，需滚动重启生效）。
 * SeaTunnel event report channel card for a cluster: view status and enable, disable or rotate the token (rewrites
 * seatunnel.yaml and needs a rolling restart to take effect).
 */
export function EngineEventChannelCard({
  clusterId,
  onConfigChanged,
}: {
  clusterId: number;
  onConfigChanged?: () => void;
}) {
  const t = useTranslations('cluster.eventChannel');
  const [status, setStatus] = useState<SyncEventChannelStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pendingAction, setPendingAction] = useState<PendingAction>(null);
  const [submitting, setSubmitting] = useState(false);
  const loadedRef = useRef(false);

  const load = useCallback(async () => {
    if (loadedRef.current) {
      setRefreshing(true);
    }
    try {
      setStatus(await services.sync.getEventChannel(clusterId));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      loadedRef.current = true;
      setLoading(false);
      setRefreshing(false);
    }
  }, [clusterId]);

  useEffect(() => {
    loadedRef.current = false;
    setLoading(true);
    void load();
  }, [load]);

  const confirmAction = async () => {
    if (!pendingAction) {
      return;
    }
    setSubmitting(true);
    try {
      const request =
        pendingAction === 'rotate'
          ? {rotate_token: true}
          : {enabled: pendingAction === 'enable'};
      const next = await services.sync.updateEventChannel(clusterId, request);
      setStatus(next);
      if (next.config_apply_error) {
        toast.warning(t('appliedWithErrors', {errors: next.config_apply_error}));
      } else {
        toast.success(t('applied'));
      }
      onConfigChanged?.();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSubmitting(false);
      setPendingAction(null);
    }
  };

  const formatTime = (value?: string) => (value ? formatDateTimeTz(value) : t('never'));

  return (
    <Card className='relative overflow-hidden' data-testid='cluster-event-channel-card'>
      <TableLoadingBar loading={refreshing} />
      <CardHeader className='flex flex-row flex-wrap items-center gap-2 space-y-0 pb-3'>
        <Radio className='size-4 text-primary' />
        <CardTitle className='text-sm'>{t('title')}</CardTitle>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type='button'
              className='text-muted-foreground/70 hover:text-foreground'
              aria-label={t('description')}
              data-testid='cluster-event-channel-help'
            >
              <HelpCircle className='size-3.5' />
            </button>
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-xs text-xs leading-relaxed'>
            {t('description')}
          </TooltipContent>
        </Tooltip>
        {status ? (
          <Badge variant='outline' className={cn('whitespace-nowrap text-[11px]', eventChannelStateClass(status.state))}>
            {t(`state.${status.state}`)}
          </Badge>
        ) : null}
        <div className='ml-auto flex flex-wrap items-center gap-1.5'>
          <Button
            size='icon'
            variant='ghost'
            className='size-7'
            aria-label={t('refresh')}
            onClick={() => void load()}
          >
            <RefreshCw className='size-3.5' />
          </Button>
          {status?.enabled ? (
            <>
              <Button size='sm' variant='outline' className='h-7 text-xs' disabled={submitting} onClick={() => setPendingAction('rotate')}>
                {t('rotate')}
              </Button>
              <Button size='sm' variant='outline' className='h-7 text-xs' disabled={submitting} onClick={() => setPendingAction('disable')}>
                {t('disable')}
              </Button>
            </>
          ) : (
            <Button
              size='sm'
              className='h-7 text-xs'
              disabled={submitting || !status?.supported}
              onClick={() => setPendingAction('enable')}
              data-testid='cluster-event-channel-enable'
            >
              {t('enable')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className='space-y-2 text-xs'>
        {loading ? (
          <div className='space-y-2'>
            <Skeleton className='h-4 w-2/3' />
            <Skeleton className='h-4 w-1/2' />
          </div>
        ) : !status ? null : (
          <>
            {!status.supported ? (
              <p className='text-amber-700 dark:text-amber-300'>
                {t('unsupported', {version: status.version || '-'})}
              </p>
            ) : null}
            {status.enabled ? (
              <div className='flex flex-wrap gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground'>
                <span className='whitespace-nowrap'>{t('lastEvent', {time: formatTime(status.last_event_at)})}</span>
                <span className='whitespace-nowrap'>
                  {t('counters', {
                    received: status.received_count,
                    duplicates: status.duplicate_count,
                    dropped: status.dropped_count,
                  })}
                </span>
                {status.auth_fail_count > 0 ? (
                  <span className='whitespace-nowrap text-red-600 dark:text-red-400'>
                    {t('authFails', {count: status.auth_fail_count})}
                  </span>
                ) : null}
                <span className='whitespace-nowrap'>
                  {status.report_non_terminal ? t('modeFull') : t('modeTerminal')}
                </span>
              </div>
            ) : null}
            {status.url ? (
              <div className='overflow-x-auto whitespace-nowrap font-mono text-[11px] text-muted-foreground'>
                {t('url', {url: status.url})}
              </div>
            ) : null}
            {status.state === 'pending_restart' ? (
              <p className='text-amber-700 dark:text-amber-300'>{t('restartHint')}</p>
            ) : null}
            {status.state === 'degraded' ? (
              <p className='text-red-700 dark:text-red-300'>{t('degradedHint')}</p>
            ) : null}
            {status.config_apply_error ? (
              <p className='break-words text-red-700 dark:text-red-300'>
                {t('appliedWithErrors', {errors: status.config_apply_error})}
              </p>
            ) : null}
          </>
        )}
      </CardContent>

      <AlertDialog open={pendingAction !== null} onOpenChange={(open) => !open && setPendingAction(null)}>
        <AlertDialogContent className='border border-border/80 shadow-2xl dark:border-border/60'>
          <AlertDialogHeader>
            <AlertDialogTitle>{pendingAction ? t(`confirm.${pendingAction}.title`) : ''}</AlertDialogTitle>
            <AlertDialogDescription>{pendingAction ? t(`confirm.${pendingAction}.body`) : ''}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={submitting}>{t('cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={submitting}
              onClick={(event) => {
                event.preventDefault();
                void confirmAction();
              }}
            >
              {t('confirmAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
