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
import {Activity} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {SeaTunnelObserveMenu} from '@/components/common/cluster/SeaTunnelObserveMenu';
import {Skeleton} from '@/components/ui/skeleton';
import {TableLoadingBar} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  SyncJobRealtime,
  SyncRealtimeEdge,
  SyncRealtimeHint,
  SyncRealtimeVertex,
} from '@/lib/services/sync';

// 轮询间隔与引擎采集桶（5 秒）对齐。
// Poll interval aligned with the engine collection bucket (5s).
const REALTIME_POLL_MS = 5000;

/**
 * 取节点最新一个点的忙碌度：source 用读取占比，transform/sink 用忙碌占比。
 * Latest busyness of a vertex: read ratio for sources, busy ratio for transforms and sinks.
 */
export function latestVertexBusy(vertex: SyncRealtimeVertex): number | null {
  const point = vertex.points[vertex.points.length - 1];
  if (!point) {
    return null;
  }
  switch (vertex.type) {
    case 'source':
      return point.sourceReadRatio ?? null;
    case 'transform':
      return point.transformBusyRatio ?? null;
    case 'sink':
      return point.sinkBusyRatio ?? null;
    default:
      return null;
  }
}

/**
 * 背压分档，与上游 Web UI 一致：0 灰、<5% 绿、<20% 黄、<50% 橙、≥50% 红。
 * Backpressure tiers matching the upstream Web UI: 0 gray, <5% green, <20% yellow, <50% orange, ≥50% red.
 */
export function backpressureTone(ratio: number): string {
  if (ratio <= 0) {
    return 'text-muted-foreground';
  }
  if (ratio < 0.05) {
    return 'text-emerald-600 dark:text-emerald-400';
  }
  if (ratio < 0.2) {
    return 'text-yellow-600 dark:text-yellow-400';
  }
  if (ratio < 0.5) {
    return 'text-orange-600 dark:text-orange-400';
  }
  return 'text-red-600 dark:text-red-400';
}

function formatPercent(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '-';
  }
  return `${Math.round(value * 100)}%`;
}

function severityClass(severity: SyncRealtimeHint['severity']): string {
  switch (severity) {
    case 'critical':
      return 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300';
    case 'warning':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300';
    default:
      return 'border-border/60 bg-muted/40 text-muted-foreground';
  }
}

/**
 * 运行中作业的实时诊断速览：瓶颈提示、各节点忙碌度、边背压，以及跳转 SeaTunnel Web UI 查看完整背压 DAG。
 * 不自建背压 DAG 与曲线——3.x 引擎自带的 Web UI 已实现，这里只做提示与深链。
 * Realtime insight for a running job: bottleneck hints, vertex busyness, edge backpressure, and a deep link to the
 * SeaTunnel Web UI for the full backpressure DAG. It deliberately does not rebuild the DAG or charts that the 3.x
 * engine Web UI already provides.
 */
export function RealtimeInsightPanel({
  jobId,
  running,
  className,
}: {
  jobId: number;
  running: boolean;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio.realtimeInsight');
  const [data, setData] = useState<SyncJobRealtime | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const hasLoadedRef = useRef(false);

  useEffect(() => {
    hasLoadedRef.current = false;
    setData(null);
    setLoading(true);
    setError(null);
  }, [jobId]);

  useEffect(() => {
    let controller: AbortController | null = null;
    let timer: number | null = null;
    let disposed = false;

    const load = async () => {
      if (disposed || document.visibilityState !== 'visible') {
        return;
      }
      controller?.abort();
      controller = new AbortController();
      if (hasLoadedRef.current) {
        setRefreshing(true);
      }
      try {
        const result = await services.sync.getJobRealtime(jobId, {signal: controller.signal});
        if (!disposed) {
          setData(result);
          setError(null);
        }
      } catch (err) {
        if (!disposed && !(err instanceof DOMException && err.name === 'AbortError')) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (!disposed) {
          hasLoadedRef.current = true;
          setLoading(false);
          setRefreshing(false);
        }
      }
    };

    void load();
    if (running) {
      timer = window.setInterval(() => void load(), REALTIME_POLL_MS);
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        void load();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      disposed = true;
      controller?.abort();
      if (timer !== null) {
        window.clearInterval(timer);
      }
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [jobId, running]);

  const vertexNames = useMemo(() => {
    const names = new Map<number, string>();
    for (const vertex of data?.vertices ?? []) {
      names.set(vertex.vertex_id, vertex.name || `#${vertex.vertex_id}`);
    }
    return names;
  }, [data]);

  const nameOf = (vertexId?: number) =>
    typeof vertexId === 'number' ? vertexNames.get(vertexId) || `#${vertexId}` : '-';

  const latestEdge = (edge: SyncRealtimeEdge) => edge.points[edge.points.length - 1];

  const fetchedAt = data?.fetched_at_ms
    ? new Date(data.fetched_at_ms).toLocaleTimeString()
    : '';

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-lg border border-border/60 bg-muted/10 px-4 py-3',
        className,
      )}
      data-testid='sync-realtime-insight'
    >
      <TableLoadingBar loading={refreshing} />
      <div className='flex flex-wrap items-center gap-2'>
        <Activity className='size-4 text-primary' />
        <span className='text-sm font-semibold'>{t('title')}</span>
        {data ? (
          <Badge variant='outline' className='whitespace-nowrap text-[11px]'>
            {data.available ? t('live') : t(`reason.${data.reason || 'collecting'}`)}
          </Badge>
        ) : null}
        {fetchedAt ? (
          <span className='whitespace-nowrap font-mono text-[11px] text-muted-foreground'>
            {t('updatedAt', {time: fetchedAt})}
          </span>
        ) : null}
        <div className='ml-auto'>
          {data?.cluster_id && data.engine_job_id ? (
            <SeaTunnelObserveMenu
              clusterId={data.cluster_id}
              engineJobId={data.engine_job_id}
              variant='button'
            />
          ) : null}
        </div>
      </div>

      <div className='mt-2.5 space-y-2.5'>
        {loading ? (
          <div className='space-y-2'>
            <Skeleton className='h-4 w-2/3' />
            <Skeleton className='h-4 w-1/2' />
          </div>
        ) : error ? (
          <p className='text-xs text-destructive'>{error}</p>
        ) : !data?.available ? (
          <p className='text-xs text-muted-foreground'>
            {t(`reasonDetail.${data?.reason || 'collecting'}`)}
          </p>
        ) : (
          <>
            {data.hints.length === 0 ? (
              <p className='text-xs text-muted-foreground'>{t('noBottleneck')}</p>
            ) : (
              <ul className='space-y-1.5'>
                {data.hints.map((hint, index) => (
                  <li
                    key={`${hint.code}-${hint.vertex_id ?? ''}-${hint.queue_id ?? ''}-${index}`}
                    className='flex flex-wrap items-center gap-2 text-xs'
                  >
                    <Badge
                      variant='outline'
                      className={cn('whitespace-nowrap text-[10px]', severityClass(hint.severity))}
                    >
                      {t(`severity.${hint.severity}`)}
                    </Badge>
                    <span>
                      {t(`hint.${hint.code}`, {
                        target: nameOf(hint.vertex_id),
                        value: formatPercent(hint.value),
                      })}
                    </span>
                  </li>
                ))}
              </ul>
            )}

            {/* 速览条：各节点最新忙碌度与各边最新背压。 / Snapshot bar: latest vertex busyness and edge backpressure. */}
            <div className='flex flex-wrap gap-1.5'>
              {data.vertices.map((vertex) => (
                <span
                  key={`v-${vertex.vertex_id}`}
                  className='inline-flex items-center gap-1 whitespace-nowrap rounded border border-border/50 bg-background px-1.5 py-0.5 font-mono text-[11px]'
                >
                  <span className='max-w-[220px] truncate' title={vertex.name}>
                    {vertex.name || `#${vertex.vertex_id}`}
                  </span>
                  <span className='text-muted-foreground'>
                    {t('busy', {value: formatPercent(latestVertexBusy(vertex))})}
                  </span>
                </span>
              ))}
              {data.edges.map((edge) => {
                const point = latestEdge(edge);
                const bp = point?.bpRatio ?? 0;
                return (
                  <span
                    key={`e-${edge.queue_id}`}
                    className='inline-flex items-center gap-1 whitespace-nowrap rounded border border-border/50 bg-background px-1.5 py-0.5 font-mono text-[11px]'
                  >
                    <span className='text-muted-foreground'>→</span>
                    <span className='max-w-[180px] truncate' title={nameOf(edge.target_vertex_id)}>
                      {nameOf(edge.target_vertex_id)}
                    </span>
                    <span className={backpressureTone(bp)}>
                      {t('backpressure', {value: formatPercent(bp)})}
                    </span>
                    <span className='text-muted-foreground'>
                      {t('queueFill', {value: formatPercent(point?.queueFillRatio)})}
                    </span>
                  </span>
                );
              })}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
