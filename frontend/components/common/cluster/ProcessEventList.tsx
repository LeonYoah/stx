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
 * 进程事件列表组件：接入 TableCard 零塌陷加载 + Arc Select + SoftChip + 0.5px 细线表格。
 * Process lifecycle event list using TableCard zero-collapse loading, Arc Select, SoftChip, and 0.5px table dividers.
 */
import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  Activity,
  AlertTriangle,
  Play,
  RefreshCw,
  Square,
  XCircle,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Select} from '@/components/arc/select/select';
import {
  NeutralChip,
  SoftChip,
  TableCard,
  type ChipTone,
} from '@/components/common/layout';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  listProcessEvents,
  type ProcessEvent,
  type ProcessEventFilter,
  type ProcessEventType,
} from '@/lib/services/monitor';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {
  LIFECYCLE_POLL_INTERVAL_MS,
  LIFECYCLE_POLL_TIMEOUT_MS,
} from './lifecycle-status-poll';

interface ProcessEventListProps {
  clusterId: number;
  nodeId?: number;
  /** 事件列表成功刷新后联动主状态（节点/头部），避免「事件已重启、主状态仍停」。 / After events reload, refresh main status so events and header/nodes stay aligned. */
  onMainStatusRefresh?: () => void;
  /**
   * 启停/重启后由详情页递增；>0 时有界静默轮询事件列表（卸载即停）。
   * Bumped by detail after lifecycle ops; when >0, bounded silent event poll (stops on unmount).
   */
  lifecyclePollEpoch?: number;
}

const EVENT_TYPE_META: Record<
  ProcessEventType,
  {tone: ChipTone; icon: React.ReactNode; labelKey: string}
> = {
  started: {
    tone: 'emerald',
    icon: <Play className='size-3' aria-hidden />,
    labelKey: 'monitor.events.started',
  },
  stopped: {
    tone: 'muted',
    icon: <Square className='size-3' aria-hidden />,
    labelKey: 'monitor.events.stopped',
  },
  crashed: {
    tone: 'rose',
    icon: <XCircle className='size-3' aria-hidden />,
    labelKey: 'monitor.events.crashed',
  },
  restarted: {
    tone: 'emerald',
    icon: <RefreshCw className='size-3' aria-hidden />,
    labelKey: 'monitor.events.restarted',
  },
  restart_failed: {
    tone: 'rose',
    icon: <AlertTriangle className='size-3' aria-hidden />,
    labelKey: 'monitor.events.restartFailed',
  },
  restart_limit_reached: {
    tone: 'rose',
    icon: <AlertTriangle className='size-3' aria-hidden />,
    labelKey: 'monitor.events.restartLimitReached',
  },
  cluster_restart_requested: {
    tone: 'blue',
    icon: <RefreshCw className='size-3' aria-hidden />,
    labelKey: 'monitor.events.clusterRestartRequested',
  },
  node_restart_requested: {
    tone: 'blue',
    icon: <RefreshCw className='size-3' aria-hidden />,
    labelKey: 'monitor.events.nodeRestartRequested',
  },
  node_stop_requested: {
    tone: 'amber',
    icon: <Square className='size-3' aria-hidden />,
    labelKey: 'monitor.events.nodeStopRequested',
  },
  node_offline: {
    tone: 'rose',
    icon: <AlertTriangle className='size-3' aria-hidden />,
    labelKey: 'monitor.events.nodeOffline',
  },
  node_recovered: {
    tone: 'emerald',
    icon: <Activity className='size-3' aria-hidden />,
    labelKey: 'monitor.events.nodeRecovered',
  },
};

const PAGE_SIZE = 20;

export function ProcessEventList({
  clusterId,
  nodeId,
  onMainStatusRefresh,
  lifecyclePollEpoch = 0,
}: ProcessEventListProps) {
  const t = useTranslations();
  const [events, setEvents] = useState<ProcessEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [eventTypeFilter, setEventTypeFilter] = useState<
    ProcessEventType | 'all'
  >('all');
  const reqSeqRef = useRef(0);
  /** 当前持有可见 loading 的请求序号；静默轮询不得写入。 / Seq that owns visible loading; silent polls must not claim it. */
  const loadingOwnerSeqRef = useRef<number | null>(null);
  const mountedRef = useRef(true);
  const pollSessionRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onMainStatusRefreshRef = useRef(onMainStatusRefresh);
  onMainStatusRefreshRef.current = onMainStatusRefresh;
  const pageRef = useRef(page);
  pageRef.current = page;
  const eventTypeFilterRef = useRef(eventTypeFilter);
  eventTypeFilterRef.current = eventTypeFilter;
  const nodeIdRef = useRef(nodeId);
  nodeIdRef.current = nodeId;
  const clusterIdRef = useRef(clusterId);
  clusterIdRef.current = clusterId;

  const clearPollTimer = useCallback(() => {
    if (pollTimerRef.current !== null) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  const stopLifecycleEventPoll = useCallback(() => {
    pollSessionRef.current += 1;
    clearPollTimer();
  }, [clearPollTimer]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopLifecycleEventPoll();
    };
  }, [stopLifecycleEventPoll]);

  type LoadEventsOptions = {
    /** 静默刷新：不亮 loading，避免事件区闪烁与滚回顶部。 / Silent: no loading bar, no flicker or scroll jump. */
    silent?: boolean;
    /** 生命周期轮询中跳过主状态联动（详情页已有状态轮询）。 / Skip main-status linkage during lifecycle poll (detail already polls status). */
    skipMainStatusRefresh?: boolean;
  };

  const loadEvents = useCallback(
    async (options?: LoadEventsOptions) => {
      const silent = options?.silent === true;
      const skipMainStatusRefresh = options?.skipMainStatusRefresh === true;
      const seq = ++reqSeqRef.current;
      if (!silent) {
        setLoading(true);
        loadingOwnerSeqRef.current = seq;
        setError(null);
      } else if (
        // 静默抢占序号后立刻丢掉过期可见 loading，避免被挂起的非静默请求拖住转圈。
        // Silent seq bump drops stale visible loading so a hung non-silent cannot keep spinning.
        loadingOwnerSeqRef.current !== null &&
        loadingOwnerSeqRef.current < seq
      ) {
        loadingOwnerSeqRef.current = null;
        setLoading(false);
      }
      // 成功/失败/被序号丢弃/卸载时：若本请求持有可见 loading，必须释放，避免刷新图标一直转。
      // On success/fail/stale-discard/unmount: release visible loading if this request owns it.
      const releaseLoadingIfOwned = () => {
        if (loadingOwnerSeqRef.current !== seq) {
          return;
        }
        loadingOwnerSeqRef.current = null;
        if (mountedRef.current) {
          setLoading(false);
        }
      };
      try {
        const filter: ProcessEventFilter = {
          page: pageRef.current,
          page_size: PAGE_SIZE,
        };
        const currentNodeId = nodeIdRef.current;
        if (currentNodeId) {
          filter.node_id = currentNodeId;
        }
        if (eventTypeFilterRef.current !== 'all') {
          filter.event_type = eventTypeFilterRef.current;
        }

        const response = await listProcessEvents(
          clusterIdRef.current,
          filter,
        );
        if (!mountedRef.current || seq !== reqSeqRef.current) {
          releaseLoadingIfOwned();
          return;
        }
        setEvents(response.events ?? []);
        setTotal(response.total ?? 0);
        // 静默成功也要清掉错误卡，否则旧 error 会一直挡着新数据。
        // Silent success must clear a prior error card so it does not cover fresh rows.
        setError(null);
        if (!skipMainStatusRefresh) {
          onMainStatusRefreshRef.current?.();
        }
      } catch (err) {
        if (!mountedRef.current || seq !== reqSeqRef.current) {
          releaseLoadingIfOwned();
          return;
        }
        if (!silent) {
          setError(
            err instanceof Error ? err.message : t('monitor.loadEventsError'),
          );
        }
      } finally {
        releaseLoadingIfOwned();
      }
    },
    [t],
  );

  useEffect(() => {
    void loadEvents();
  }, [loadEvents, clusterId, eventTypeFilter, nodeId, page]);

  // 仅在已挂载期间 epoch 递增时开轮询；带着旧 epoch 切回节点页只走首屏 loadEvents，避免空转 90s。
  // Start poll only when epoch bumps while mounted; remount with a stale epoch uses the initial load only.
  const prevLifecyclePollEpochRef = useRef(lifecyclePollEpoch);

  // 启停/重启后有界静默轮询事件：立即拉一拍，再按与主状态相同的间隔/超时续拉；离开节点页卸载即停。
  // Bounded silent event poll after lifecycle: one immediate fetch, then same interval/timeout as status; stop on unmount.
  useEffect(() => {
    const prevEpoch = prevLifecyclePollEpochRef.current;
    prevLifecyclePollEpochRef.current = lifecyclePollEpoch;
    if (
      !lifecyclePollEpoch ||
      lifecyclePollEpoch < 1 ||
      lifecyclePollEpoch === prevEpoch
    ) {
      return;
    }
    if (!mountedRef.current) {
      return;
    }
    stopLifecycleEventPoll();
    if (!mountedRef.current) {
      return;
    }
    const session = pollSessionRef.current;
    const startedAt = Date.now();

    const schedule = (delayMs: number) => {
      if (!mountedRef.current || session !== pollSessionRef.current) {
        return;
      }
      clearPollTimer();
      pollTimerRef.current = setTimeout(() => {
        void tick();
      }, delayMs);
    };

    const tick = async () => {
      if (!mountedRef.current || session !== pollSessionRef.current) {
        return;
      }
      await loadEvents({silent: true, skipMainStatusRefresh: true});
      if (!mountedRef.current || session !== pollSessionRef.current) {
        return;
      }
      if (Date.now() - startedAt >= LIFECYCLE_POLL_TIMEOUT_MS) {
        clearPollTimer();
        return;
      }
      schedule(LIFECYCLE_POLL_INTERVAL_MS);
    };

    void tick();
    return () => {
      if (session === pollSessionRef.current) {
        stopLifecycleEventPoll();
      }
    };
  }, [
    lifecyclePollEpoch,
    loadEvents,
    stopLifecycleEventPoll,
    clearPollTimer,
  ]);

  const filterOptions = [
    {value: 'all', label: t('monitor.allEvents')},
    ...(Object.keys(EVENT_TYPE_META) as ProcessEventType[]).map((key) => ({
      value: key,
      label: t(EVENT_TYPE_META[key].labelKey),
    })),
  ];

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <section className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex items-baseline gap-2'>
          <h3 className='text-sm font-semibold text-foreground'>
            {t('monitor.eventsTitle')}
          </h3>
          <span className='text-xs text-muted-foreground tabular-nums'>
            {t('monitor.eventsDescription', {total})}
          </span>
        </div>
        <div className='flex items-center gap-2'>
          <div className='w-44'>
            <Select
              aria-label={t('monitor.filterByType')}
              value={eventTypeFilter}
              onValueChange={(value) => {
                setEventTypeFilter(value as ProcessEventType | 'all');
                setPage(1);
              }}
              options={filterOptions}
            />
          </div>
          <Button
            type='button'
            variant='secondary'
            size='sm'
            iconOnly
            aria-label={t('common.refresh')}
            title={t('common.refresh')}
            onClick={() => void loadEvents()}
          >
            <RefreshCw
              className={`size-3.5 ${loading ? 'animate-spin' : ''}`}
              aria-hidden
            />
          </Button>
        </div>
      </div>

      <TableCard
        loading={loading}
        hasData={events.length > 0}
        skeletonColumns={7}
        skeletonRows={5}
        error={error}
        onRetry={() => void loadEvents()}
        pagination={
          total > 0
            ? {
                currentPage: page,
                totalPages,
                pageSize: PAGE_SIZE,
                totalItems: total,
                onPageChange: setPage,
              }
            : undefined
        }
        empty={
          <EmptyState
            icon={<Activity className='size-6' aria-hidden />}
            title={t('monitor.noEvents')}
            description={t('monitor.eventsDescription', {total: 0})}
          />
        }
      >
        <div className='overflow-x-auto'>
          <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
            <TableHeader>
              <TableRow>
                <TableHead>{t('monitor.eventType')}</TableHead>
                <TableHead>{t('monitor.hostname')}</TableHead>
                <TableHead>{t('monitor.ip')}</TableHead>
                <TableHead>{t('monitor.processName')}</TableHead>
                <TableHead>{t('monitor.pid')}</TableHead>
                <TableHead>{t('monitor.role')}</TableHead>
                <TableHead>{t('monitor.eventTime')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((event) => {
                const meta = EVENT_TYPE_META[event.event_type];
                return (
                  <TableRow key={event.id}>
                    <TableCell>
                      {meta ? (
                        <SoftChip tone={meta.tone} className='font-medium'>
                          {meta.icon}
                          <span className='whitespace-nowrap'>
                            {t(meta.labelKey)}
                          </span>
                        </SoftChip>
                      ) : (
                        <NeutralChip>{event.event_type}</NeutralChip>
                      )}
                    </TableCell>
                    <TableCell className='text-sm'>
                      {event.hostname || '-'}
                    </TableCell>
                    <TableCell className='font-mono text-xs tabular-nums'>
                      {event.ip || '-'}
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {event.process_name || '-'}
                    </TableCell>
                    <TableCell className='font-mono text-xs tabular-nums'>
                      {event.pid || '-'}
                    </TableCell>
                    <TableCell>
                      <NeutralChip>{event.role || '-'}</NeutralChip>
                    </TableCell>
                    <TableCell className='text-xs text-muted-foreground tabular-nums'>
                      {formatDateTimeMinuteTz(event.created_at)}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      </TableCard>
    </section>
  );
}
