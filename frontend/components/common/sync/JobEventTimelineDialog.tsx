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
import {useLocale, useTranslations} from 'next-intl';
import {
  Activity,
  Clock3,
  FileCode2,
  RefreshCw,
  Waypoints,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Skeleton} from '@/components/ui/skeleton';
import {Timeline, type TimelineEvent} from '@/components/arc/timeline/timeline';
import {ExpandableTextPanel, TableLoadingBar} from '@/components/common/layout';
import {formatDateTimeTz, resolveDisplayTimezone} from '@/lib/timezone';
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
  // 增列 / 改列的 column 是对象（带 sourceType），删列的 column 是列名字符串（如 MySQL DROP COLUMN）。
  // ADD / MODIFY carry a column object (with sourceType); DROP carries the column name as a string (e.g. MySQL DROP COLUMN).
  const columnOf = (item: Record<string, unknown>) => {
    if (typeof item.column === 'string') {
      return item.column.trim();
    }
    const column = item.column as Record<string, unknown> | undefined;
    const name = String(column?.name ?? item.columnName ?? item.oldColumn ?? '').trim();
    const sourceType = String(column?.sourceType ?? '').trim();
    return name && sourceType ? `${name} ${sourceType}` : name;
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

/**
 * 事件原始数据：完整字段格式化成 JSON（服务端已脱敏），可复制、可放大。
 * Raw event data: every field as formatted JSON (already redacted server-side), copyable and expandable.
 */
function EventRawData({event}: {event: SyncEngineEvent}) {
  const content = JSON.stringify(
    {
      id: event.id,
      event_type: event.event_type,
      job_status: event.job_status,
      engine_job_id: event.engine_job_id,
      event_time_ms: event.event_time_ms,
      received_at: event.received_at,
      payload: event.payload,
    },
    null,
    2,
  );
  return <ExpandableTextPanel content={content} height={240} className='mt-1.5' />;
}

/** 生命周期折叠组内每条子事件：行内展开自己的原始数据。/ One lifecycle sub-event; expands its raw data in place. */
function LifecycleSubRow({event}: {event: SyncEngineEvent}) {
  const [open, setOpen] = useState(false);
  return (
    <li>
      <button
        type='button'
        className='flex w-full flex-wrap items-center gap-2 py-0.5 text-left font-mono text-[11px] text-muted-foreground hover:text-foreground'
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <span className='whitespace-nowrap'>
          {formatDateTimeTz(event.event_time_ms)}
        </span>
        <span>{event.event_type}</span>
      </button>
      {open ? <EventRawData event={event} /> : null}
    </li>
  );
}

/** 折叠组详情：组内生命周期事件逐条列出。/ Group detail: the folded lifecycle events one per row. */
function LifecycleGroupDetail({events}: {events: SyncEngineEvent[]}) {
  return (
    <ul className='space-y-0.5'>
      {events.map((event) => (
        <LifecycleSubRow key={event.id} event={event} />
      ))}
    </ul>
  );
}

/** 状态事件按结果着色，其余保持中性；标题本身已写明结果，tone 只做点缀。/ Status events get an outcome tone; others stay neutral since the title already states it. */
function eventTone(event: SyncEngineEvent): TimelineEvent['tone'] {
  if (engineEventFamily(event.event_type) !== 'status') {
    return undefined;
  }
  const status = (event.job_status ?? '').toUpperCase();
  if (status === 'FAILED' || status === 'CANCELED' || status === 'ERROR') {
    return 'danger';
  }
  if (status === 'FINISHED') {
    return 'success';
  }
  return undefined;
}

const FAMILY_ICON: Record<EngineEventFamily, React.ReactNode> = {
  status: <Activity className='size-3.5' aria-hidden />,
  schema: <FileCode2 className='size-3.5' aria-hidden />,
  delay: <Clock3 className='size-3.5' aria-hidden />,
  lifecycle: <RefreshCw className='size-3.5' aria-hidden />,
  other: <Waypoints className='size-3.5' aria-hidden />,
};

/**
 * 作业事件时间线：引擎上报的状态变化、DDL（schema change）、CDC 延迟与生命周期事件。作为作业详情的页签内嵌，
 * active 为假（页签不可见）时不请求也不轮询。
 * Job event timeline: engine-reported status changes, DDL (schema change), CDC delay and lifecycle events. Embedded
 * as a job detail tab; nothing is requested or polled while active is false (tab hidden).
 */
export function JobEventTimelineContent({
  job,
  active,
}: {
  job: SyncJobInstance | null;
  active: boolean;
}) {
  const t = useTranslations('workbenchStudio.jobEvents');
  const locale = useLocale();
  const [events, setEvents] = useState<SyncEngineEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const loadedRef = useRef(false);
  const jobId = job?.id ?? 0;
  const running = getRealtimeEntry(job).running;

  useEffect(() => {
    if (!active || !jobId) {
      return;
    }
    let disposed = false;
    loadedRef.current = false;
    setLoading(true);
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
  }, [active, jobId, running]);

  const rows = useMemo(() => buildTimelineRows(events), [events]);

  const timelineEvents = useMemo<TimelineEvent[]>(() => {
    const eventTitle = (event: SyncEngineEvent): string => {
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
    return rows.map((row): TimelineEvent => {
      if (row.kind === 'lifecycle') {
        const last = row.events[row.events.length - 1];
        return {
          id: `lifecycle-${last.id}`,
          at: last.event_time_ms,
          title: t('lifecycleGroup', {count: row.events.length}),
          meta: t('family.lifecycle'),
          detail: <LifecycleGroupDetail events={row.events} />,
          icon: FAMILY_ICON.lifecycle,
        };
      }
      const family = engineEventFamily(row.event.event_type);
      return {
        id: String(row.event.id),
        at: row.event.event_time_ms,
        title: eventTitle(row.event),
        meta: `${t(`family.${family}`)} · ${row.event.event_type}`,
        detail: <EventRawData event={row.event} />,
        tone: eventTone(row.event),
        icon: FAMILY_ICON[family],
      };
    });
  }, [rows, t]);

  return (
    <div className='flex h-full min-h-0 flex-col'>
        <div className='flex items-center gap-2 pb-2'>
          <Badge variant='outline' className='whitespace-nowrap text-[10px] font-normal' data-testid='sync-job-event-total'>
            {t('totalBadge', {total})}
          </Badge>
          <span
            className='text-[10px] text-muted-foreground'
            data-testid='sync-job-event-retention-hint'
          >
            {t('retentionHint')}
          </span>
        </div>
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
          ) : timelineEvents.length === 0 ? (
            <p className='p-6 text-center text-xs text-muted-foreground'>{t('empty')}</p>
          ) : (
            <div data-testid='sync-job-event-timeline'>
              <Timeline
                events={timelineEvents}
                now={Date.now()}
                label={t('feedLabel')}
                timeZone={resolveDisplayTimezone()}
                locale={locale}
                headingLevel={4}
              />
            </div>
          )}
        </div>
    </div>
  );
}
