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

import {useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Skeleton} from '@/components/ui/skeleton';
import {TableLoadingBar} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import {formatDateTimeTz} from '@/lib/timezone';
import services from '@/lib/services';
import type {SyncEngineEvent, SyncJobInstance} from '@/lib/services/sync';
import {getRealtimeEntry} from './sync-studio-utils';

// 运行中作业的时间线刷新间隔（引擎每 10 秒批量上报一次）。
// Timeline refresh interval for running jobs (the engine reports in 10-second batches).
const TIMELINE_POLL_MS = 10000;

export type EngineEventFamily = 'status' | 'schema' | 'delay' | 'lifecycle' | 'other';

/** 按事件类型归类 / Classify an event type into a display family. */
export function engineEventFamily(eventType: string): EngineEventFamily {
  if (eventType === 'JOB_STATUS') {
    return 'status';
  }
  if (eventType.startsWith('SCHEMA_CHANGE_')) {
    return 'schema';
  }
  if (eventType === 'READER_MESSAGE_DELAYED') {
    return 'delay';
  }
  if (eventType.startsWith('LIFECYCLE_')) {
    return 'lifecycle';
  }
  return 'other';
}

/** 时间线行：普通事件，或折叠的一组连续生命周期事件 / Timeline row: one event or a collapsed run of lifecycle events. */
export type TimelineRow =
  | {kind: 'event'; event: SyncEngineEvent}
  | {kind: 'lifecycle'; events: SyncEngineEvent[]};

/**
 * 把连续的生命周期事件折叠成一行，其余事件原样保留。
 * Collapse consecutive lifecycle events into one row and keep other events as they are.
 */
export function buildTimelineRows(events: SyncEngineEvent[]): TimelineRow[] {
  const rows: TimelineRow[] = [];
  for (const event of events) {
    if (engineEventFamily(event.event_type) === 'lifecycle') {
      const last = rows[rows.length - 1];
      if (last && last.kind === 'lifecycle') {
        last.events.push(event);
      } else {
        rows.push({kind: 'lifecycle', events: [event]});
      }
      continue;
    }
    rows.push({kind: 'event', event});
  }
  return rows;
}

/**
 * 从 schema change 事件里取表名与列名做摘要。
 * Summarize a schema change event by table and column names.
 */
export function schemaChangeSummary(payload: Record<string, unknown>): string {
  const table = (payload.tableIdentifier ?? payload.tablePath) as Record<string, unknown> | undefined;
  const tableName = table
    ? [table.databaseName, table.schemaName, table.tableName].filter(Boolean).join('.')
    : '';
  const columnOf = (item: Record<string, unknown>) => {
    const column = item.column as Record<string, unknown> | undefined;
    return String(column?.name ?? item.columnName ?? item.oldColumn ?? '').trim();
  };
  // UPDATE_COLUMNS 把多个子变更放在 events 里（如 PG 的 ADD COLUMN），逐个取列名并标注操作。
  // UPDATE_COLUMNS carries several sub-changes in `events` (e.g. PG ADD COLUMN); list each column with its operation.
  const subEvents = Array.isArray(payload.events) ? (payload.events as Record<string, unknown>[]) : [];
  const columns = subEvents.length
    ? subEvents
        .map((item) => {
          const name = columnOf(item);
          const type = String(item.eventType ?? '');
          if (!name) {
            return '';
          }
          if (type.endsWith('ADD_COLUMN')) {
            return `+${name}`;
          }
          if (type.endsWith('DROP_COLUMN')) {
            return `-${name}`;
          }
          return `~${name}`;
        })
        .filter(Boolean)
        .join(', ')
    : columnOf(payload);
  return [tableName, columns].filter(Boolean).join(' · ');
}

function familyClass(family: EngineEventFamily): string {
  switch (family) {
    case 'status':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300';
    case 'schema':
      return 'border-violet-500/40 bg-violet-500/10 text-violet-700 dark:text-violet-300';
    case 'delay':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300';
    default:
      return 'border-border/60 bg-muted/40 text-muted-foreground';
  }
}

/**
 * 作业事件时间线：引擎上报的状态变化、DDL（schema change）、CDC 延迟与生命周期事件。
 * Job event timeline: engine-reported status changes, DDL (schema change), CDC delay and lifecycle events.
 */
export function JobEventTimelineDialog({
  job,
  open,
  onOpenChange,
}: {
  job: SyncJobInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations('workbenchStudio.jobEvents');
  const [events, setEvents] = useState<SyncEngineEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const loadedRef = useRef(false);
  const jobId = job?.id ?? 0;
  const running = getRealtimeEntry(job).running;

  useEffect(() => {
    if (!open || !jobId) {
      return;
    }
    let disposed = false;
    loadedRef.current = false;
    setLoading(true);
    setExpanded({});
    const load = async () => {
      if (loadedRef.current) {
        setRefreshing(true);
      }
      try {
        const result = await services.sync.listJobEvents(jobId, {page_size: 200});
        if (!disposed) {
          setEvents(result.events);
          setTotal(result.total);
          setError(null);
        }
      } catch (err) {
        if (!disposed) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (!disposed) {
          loadedRef.current = true;
          setLoading(false);
          setRefreshing(false);
        }
      }
    };
    void load();
    const timer = running ? window.setInterval(() => void load(), TIMELINE_POLL_MS) : null;
    return () => {
      disposed = true;
      if (timer !== null) {
        window.clearInterval(timer);
      }
    };
  }, [open, jobId, running]);

  const rows = useMemo(() => buildTimelineRows(events), [events]);

  const eventDetail = (event: SyncEngineEvent): string => {
    const payload = (event.payload ?? {}) as Record<string, unknown>;
    switch (engineEventFamily(event.event_type)) {
      case 'status':
        return event.job_status || '-';
      case 'delay':
        return t('delayValue', {value: `${(Number(payload.delay_ms ?? 0) / 1000).toFixed(1)}s`});
      case 'schema':
        return schemaChangeSummary(payload) || event.event_type;
      default:
        return event.event_type;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='flex max-h-[85vh] w-[94vw] max-w-3xl flex-col overflow-hidden border border-border/80 shadow-2xl dark:border-border/60'>
        <DialogHeader>
          <DialogTitle>{t('title', {id: jobId})}</DialogTitle>
          <DialogDescription>{t('description', {total})}</DialogDescription>
        </DialogHeader>
        <div className='relative min-h-0 flex-1 overflow-auto'>
          <TableLoadingBar loading={refreshing} />
          {loading ? (
            <div className='space-y-2 p-1'>
              {Array.from({length: 4}).map((_, index) => (
                <Skeleton key={index} className='h-6 w-full' />
              ))}
            </div>
          ) : error ? (
            <p className='p-2 text-xs text-destructive'>{error}</p>
          ) : rows.length === 0 ? (
            <p className='p-6 text-center text-xs text-muted-foreground'>{t('empty')}</p>
          ) : (
            <ul className='divide-y divide-border/50' data-testid='sync-job-event-timeline'>
              {rows.map((row, index) => {
                if (row.kind === 'lifecycle') {
                  const first = row.events[row.events.length - 1];
                  const isOpen = Boolean(expanded[index]);
                  return (
                    <li key={`lifecycle-${first.id}`} className='py-1.5'>
                      <button
                        type='button'
                        className='flex w-full flex-wrap items-center gap-2 text-left text-xs'
                        onClick={() => setExpanded((current) => ({...current, [index]: !current[index]}))}
                      >
                        <span className='whitespace-nowrap font-mono text-[11px] text-muted-foreground'>
                          {formatDateTimeTz(first.event_time_ms)}
                        </span>
                        <Badge variant='outline' className={cn('whitespace-nowrap text-[10px]', familyClass('lifecycle'))}>
                          {t('family.lifecycle')}
                        </Badge>
                        <span className='text-muted-foreground'>
                          {t('lifecycleGroup', {count: row.events.length})}
                        </span>
                      </button>
                      {isOpen ? (
                        <ul className='mt-1 space-y-0.5 pl-6'>
                          {row.events.map((event) => (
                            <li key={event.id} className='flex flex-wrap gap-2 font-mono text-[11px] text-muted-foreground'>
                              <span className='whitespace-nowrap'>{formatDateTimeTz(event.event_time_ms)}</span>
                              <span>{event.event_type}</span>
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </li>
                  );
                }
                const family = engineEventFamily(row.event.event_type);
                return (
                  <li key={row.event.id} className='flex flex-wrap items-center gap-2 py-1.5 text-xs'>
                    <span className='whitespace-nowrap font-mono text-[11px] text-muted-foreground'>
                      {formatDateTimeTz(row.event.event_time_ms)}
                    </span>
                    <Badge variant='outline' className={cn('whitespace-nowrap text-[10px]', familyClass(family))}>
                      {t(`family.${family}`)}
                    </Badge>
                    <span className='min-w-0 break-words font-mono text-[11px]' title={row.event.event_type}>
                      {eventDetail(row.event)}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
