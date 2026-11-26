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
import {toast} from '@/lib/toast';
import {HelpCircle, Radio, RefreshCw} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {Button} from '@/components/arc/button/button';
import {Skeleton} from '@/components/ui/skeleton';
import {
  ConfirmDialog,
  SoftChip,
  TableLoadingBar,
  type ChipTone,
} from '@/components/common/layout';
import {formatDateTimeTz} from '@/lib/timezone';
import services from '@/lib/services';
import type {SyncEventChannelState, SyncEventChannelStatus} from '@/lib/services/sync';

type PendingAction = 'enable' | 'disable' | 'rotate' | null;

/**
 * 事件通道状态配色（兼容旧导出，实际渲染走 SoftChip 语义 tone）。
 * Event channel state tone class (kept for existing callers; component uses SoftChip tone).
 */
export function eventChannelStateClass(state: SyncEventChannelState): string {
  switch (state) {
    case 'healthy':
      return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300';
    case 'degraded':
      return 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300';
    case 'pending_restart':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300';
    case 'waiting_events':
      return 'border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300';
    default:
      return 'border-border/60 bg-muted/40 text-muted-foreground';
  }
}

function eventChannelTone(state: SyncEventChannelState): ChipTone {
  switch (state) {
    case 'healthy':
      return 'emerald';
    case 'degraded':
      return 'rose';
    case 'pending_restart':
      return 'amber';
    case 'waiting_events':
      return 'blue';
    default:
      return 'muted';
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
  const [callbackInput, setCallbackInput] = useState('');
  const [callbackSaving, setCallbackSaving] = useState(false);
  const callbackDirtyRef = useRef(false);
  const loadedRef = useRef(false);

  const load = useCallback(async () => {
    if (loadedRef.current) {
      setRefreshing(true);
    }
    try {
      const next = await services.sync.getEventChannel(clusterId);
      setStatus(next);
      if (!callbackDirtyRef.current) {
        setCallbackInput(next.stx_callback_url || '');
      }
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

  const saveCallbackOverride = async () => {
    setCallbackSaving(true);
    try {
      const next = await services.sync.updateEventChannel(clusterId, {
        stx_callback_url: callbackInput.trim(),
      });
      setStatus(next);
      callbackDirtyRef.current = false;
      setCallbackInput(next.stx_callback_url || '');
      toast.success(t('callbackSaved'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setCallbackSaving(false);
    }
  };

  const formatTime = (value?: string) => (value ? formatDateTimeTz(value) : t('never'));

  return (
    <div
      className='relative overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'
      data-testid='cluster-event-channel-card'
    >
      <TableLoadingBar loading={refreshing} />
      <div className='flex flex-wrap items-center gap-2 border-b-[0.5px] border-border/80 px-5 py-3.5'>
        <Radio className='size-4 text-primary' aria-hidden />
        <h3 className='text-sm font-semibold text-foreground'>{t('title')}</h3>
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
          <SoftChip tone={eventChannelTone(status.state)}>
            {t(`state.${status.state}`)}
          </SoftChip>
        ) : null}
        <div className='ml-auto flex flex-wrap items-center gap-1.5'>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            iconOnly
            aria-label={t('refresh')}
            onClick={() => void load()}
          >
            <RefreshCw className='size-3.5' aria-hidden />
          </Button>
          {status?.enabled ? (
            <>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                disabled={submitting}
                onClick={() => setPendingAction('rotate')}
              >
                {t('rotate')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                disabled={submitting}
                onClick={() => setPendingAction('disable')}
              >
                {t('disable')}
              </Button>
            </>
          ) : (
            <Button
              type='button'
              size='sm'
              disabled={submitting || !status?.supported}
              onClick={() => setPendingAction('enable')}
              data-testid='cluster-event-channel-enable'
            >
              {t('enable')}
            </Button>
          )}
        </div>
      </div>
      <div className='space-y-2 px-5 py-4 text-xs'>
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
            {status.supported && !status.enabled ? (
              <p className='text-muted-foreground' data-testid='cluster-event-channel-purpose'>
                {t('purposeLine')}
              </p>
            ) : null}
            {status.enabled ? (
              <div className='flex flex-wrap gap-x-4 gap-y-1 font-mono text-[11px] tabular-nums text-muted-foreground'>
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
            {status.is_kubernetes ? (
              <div className='space-y-1.5 rounded-(--radius-panel) border border-border/60 bg-muted/20 px-3 py-2.5' data-testid='k8s-callback-override'>
                <div className='flex flex-wrap items-center gap-2'>
                  <input
                    type='text'
                    value={callbackInput}
                    onChange={(e) => {
                      callbackDirtyRef.current = true;
                      setCallbackInput(e.target.value);
                    }}
                    placeholder='http://stx.stx-system.svc.cluster.local:17800'
                    aria-label={t('callbackOverrideLabel')}
                    className='h-7 min-w-0 flex-1 rounded-md border border-border bg-transparent px-2 font-mono text-[11px] outline-none focus:border-primary'
                  />
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    disabled={callbackSaving || callbackInput.trim() === (status.stx_callback_url || '')}
                    onClick={() => void saveCallbackOverride()}
                    data-testid='k8s-callback-save'
                  >
                    {t('callbackSave')}
                  </Button>
                </div>
                <p className='text-[11px] leading-relaxed text-muted-foreground'>
                  {callbackInput.trim() === ''
                    ? t('callbackInheritHint', {url: status.default_url || '-'})
                    : t('callbackOverrideHint')}
                </p>
                {/^(localhost|127\.|\[?::1\]?|0\.0\.0\.0)/i.test(
                  (() => {
                    try {
                      return new URL(callbackInput.trim() || status.url || '').hostname;
                    } catch {
                      return '';
                    }
                  })(),
                ) ? (
                  <p className='text-[11px] text-amber-700 dark:text-amber-300' data-testid='k8s-callback-loopback-warning'>
                    {t('callbackLoopbackHint')}
                  </p>
                ) : null}
              </div>
            ) : null}
            {status.state === 'pending_restart' ? (
              <p className='text-amber-700 dark:text-amber-300'>{t('restartHint')}</p>
            ) : null}
            {status.state === 'waiting_events' ? (
              <p className='text-blue-700 dark:text-blue-300'>{t('waitingHint')}</p>
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
      </div>

      <ConfirmDialog
        open={pendingAction !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingAction(null);
          }
        }}
        title={pendingAction ? t(`confirm.${pendingAction}.title`) : ''}
        description={pendingAction ? t(`confirm.${pendingAction}.body`) : ''}
        cancelLabel={t('cancel')}
        confirmLabel={t('confirmAction')}
        confirmVariant={pendingAction === 'disable' ? 'danger' : 'primary'}
        loading={submitting}
        onConfirm={confirmAction}
      />
    </div>
  );
}
