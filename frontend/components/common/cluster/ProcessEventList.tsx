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

interface ProcessEventListProps {
  clusterId: number;
  nodeId?: number;
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

export function ProcessEventList({clusterId, nodeId}: ProcessEventListProps) {
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

  const loadEvents = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    setError(null);
    try {
      const filter: ProcessEventFilter = {
        page,
        page_size: PAGE_SIZE,
      };
      if (nodeId) {
        filter.node_id = nodeId;
      }
      if (eventTypeFilter !== 'all') {
        filter.event_type = eventTypeFilter;
      }

      const response = await listProcessEvents(clusterId, filter);
      if (seq !== reqSeqRef.current) {
        return;
      }
      setEvents(response.events ?? []);
      setTotal(response.total ?? 0);
    } catch (err) {
      if (seq !== reqSeqRef.current) {
        return;
      }
      setError(
        err instanceof Error ? err.message : t('monitor.loadEventsError'),
      );
    } finally {
      if (seq === reqSeqRef.current) {
        setLoading(false);
      }
    }
  }, [clusterId, eventTypeFilter, nodeId, page, t]);

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

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
