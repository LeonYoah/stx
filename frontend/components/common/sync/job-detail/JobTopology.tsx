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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Activity, GripHorizontal} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Skeleton} from '@/components/ui/skeleton';
import {TableLoadingBar} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import type {SyncRealtimeHint} from '@/lib/services/sync';
import {
  buildMetricIndex,
  buildRealtimeOverlay,
  formatPercent,
  mergeRuntimeWithPlan,
  normalizeJobDag,
  type DagVertex,
} from './dag-model';
import JobDagCanvas, {type DagMode} from './JobDagCanvas';
import {DagNodeDrawer} from './DagNodeDrawer';
import {OperatorList} from './OperatorList';
import {useJobPlanDag, useJobRealtime, type JobRealtimeState} from './use-job-realtime';

// 图区高度占比与上下限，参考 Flink overview 的 max(280, min(高度*0.4, 500))，这里图更重要所以占比更大。
// Graph area ratio and bounds, after Flink overview's max(280, min(height*0.4, 500)); the graph matters more here.
const GRAPH_RATIO = 0.62;
const GRAPH_MIN = 200;
const LIST_MIN = 120;

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
 * 拓扑上方的一行瓶颈提示：状态、更新时间与瓶颈提示；数据不可用时只显示原因（节点上的数据显示 N/A）。
 * One-line bottleneck strip above the topology: status, update time and hints; when data is unavailable only the
 * reason is shown (nodes show N/A).
 */
function RealtimeHintsBar({state, vertices}: {state: JobRealtimeState; vertices: DagVertex[]}) {
  const tRealtime = useTranslations('workbenchStudio.realtimeInsight');
  const {data, loading, refreshing, error} = state;
  const names = useMemo(() => new Map(vertices.map((vertex) => [vertex.vertexId, vertex.displayName])), [vertices]);
  const nameOf = (vertexId?: number) => (typeof vertexId === 'number' ? names.get(vertexId) || `#${vertexId}` : '-');
  const fetchedAt = data?.fetched_at_ms ? new Date(data.fetched_at_ms).toLocaleTimeString() : '';
  // 原因码与提示码来自后端，未知值显示原码，避免缺键报错。
  // Reason and hint codes come from the backend; unknown values show the raw code instead of a missing-key error.
  const translate = (key: string, fallback: string, values?: Record<string, string>) =>
    tRealtime.has(key) ? tRealtime(key, values) : fallback;
  const reason = data?.reason || 'collecting';

  return (
    <div className='relative flex flex-wrap items-center gap-x-2 gap-y-1 overflow-hidden rounded-md border border-border/60 bg-muted/10 px-3 py-1.5 text-xs' data-testid='sync-realtime-insight'>
      <TableLoadingBar loading={refreshing} />
      <Activity className='size-3.5 text-primary' />
      <span className='font-medium'>{tRealtime('title')}</span>
      {data ? (
        <Badge variant='outline' className='whitespace-nowrap text-[10px]'>
          {data.available ? tRealtime('live') : translate(`reason.${reason}`, reason)}
        </Badge>
      ) : null}
      {fetchedAt ? <span className='whitespace-nowrap font-mono text-[11px] text-muted-foreground'>{tRealtime('updatedAt', {time: fetchedAt})}</span> : null}
      <span className='text-muted-foreground/40'>|</span>
      {loading ? (
        <Skeleton className='h-3.5 w-48' />
      ) : error ? (
        <span className='text-destructive'>{error}</span>
      ) : !data?.available ? (
        <span className='text-muted-foreground'>{translate(`reasonDetail.${reason}`, reason)}</span>
      ) : data.hints.length === 0 ? (
        <span className='text-muted-foreground'>{tRealtime('noBottleneck')}</span>
      ) : (
        data.hints.map((hint, index) => (
          <span key={`${hint.code}-${hint.vertex_id ?? ''}-${hint.queue_id ?? ''}-${index}`} className='inline-flex items-center gap-1'>
            <Badge variant='outline' className={cn('whitespace-nowrap px-1 py-0 text-[10px]', severityClass(hint.severity))}>
              {tRealtime(`severity.${hint.severity}`)}
            </Badge>
            <span>{translate(`hint.${hint.code}`, hint.code, {target: nameOf(hint.vertex_id), value: formatPercent(hint.value)})}</span>
          </span>
        ))
      )}
    </div>
  );
}

export interface JobTopologyProps {
  mode: DagMode;
  /** 运行模式为引擎 job_dag，plan 模式为 config tool 的 webui_job.jobDag。/ Engine job_dag in runtime mode, config tool jobDag in plan mode. */
  jobDag: unknown;
  metrics?: unknown;
  /** 运行模式下用于实时指标与计划 DAG。/ Used for realtime metrics and the plan DAG in runtime mode. */
  jobId?: number;
  /** 实时可观测（3.x 集群模式）且拓扑可见时才轮询。/ Poll only when realtime is supported (3.x cluster mode) and visible. */
  realtimeEnabled?: boolean;
  running?: boolean;
  warnings?: string[];
  className?: string;
}

/**
 * 作业拓扑（对照 Flink job overview）：上方提示条，中间图与算子列表上下可拖动分栏，右侧节点抽屉。
 * Job topology (modeled on Flink's job overview): hint strip on top, a draggable split between graph and operator list,
 * and a node drawer on the right.
 */
export default function JobTopology({mode, jobDag, metrics, jobId, realtimeEnabled = false, running = false, warnings, className}: JobTopologyProps) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const runtime = mode === 'runtime';
  const [selectedVertexId, setSelectedVertexId] = useState<number | null>(null);
  const [drawerFull, setDrawerFull] = useState(false);

  const plan = useJobPlanDag(jobId ?? null, runtime && Boolean(jobId));
  const realtime = useJobRealtime(jobId ?? null, runtime && realtimeEnabled, running);

  const baseGraph = useMemo(() => normalizeJobDag(jobDag), [jobDag]);
  const planGraph = useMemo(
    () => (plan.data?.available && plan.data.webui_job?.jobDag ? normalizeJobDag(plan.data.webui_job.jobDag) : null),
    [plan.data],
  );
  const graph = useMemo(() => (runtime ? mergeRuntimeWithPlan(baseGraph, planGraph) : baseGraph), [runtime, baseGraph, planGraph]);
  const metricIndex = useMemo(() => (runtime ? buildMetricIndex(metrics, graph.vertices) : undefined), [runtime, metrics, graph.vertices]);
  const overlay = useMemo(
    () => (runtime && realtimeEnabled ? buildRealtimeOverlay(realtime.data, graph.edges) : undefined),
    [runtime, realtimeEnabled, realtime.data, graph.edges],
  );
  const selectedVertex = selectedVertexId === null ? null : graph.vertices.find((vertex) => vertex.vertexId === selectedVertexId) ?? null;

  useEffect(() => {
    if (selectedVertexId !== null && !graph.vertices.some((vertex) => vertex.vertexId === selectedVertexId)) {
      setSelectedVertexId(null);
    }
  }, [graph.vertices, selectedVertexId]);

  // 上下分栏：按容器高度给初值，拖动后保留用户的高度（同 Flink flink-resize）。
  // Vertical split: initial height from the container, then the user's dragged height is kept (like Flink's flink-resize).
  const splitRef = useRef<HTMLDivElement | null>(null);
  const [graphHeight, setGraphHeight] = useState<number | null>(null);
  const draggedRef = useRef(false);
  useEffect(() => {
    const element = splitRef.current;
    if (!element) {
      return;
    }
    const observer = new ResizeObserver(([entry]) => {
      const total = entry.contentRect.height;
      setGraphHeight((prev) => {
        const wanted = draggedRef.current && prev !== null ? prev : Math.round(total * GRAPH_RATIO);
        return Math.max(GRAPH_MIN, Math.min(wanted, total - LIST_MIN));
      });
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const onResizeStart = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    const element = splitRef.current;
    if (!element) {
      return;
    }
    event.preventDefault();
    const top = element.getBoundingClientRect().top;
    const total = element.getBoundingClientRect().height;
    const onMove = (moveEvent: PointerEvent) => {
      draggedRef.current = true;
      setGraphHeight(Math.max(GRAPH_MIN, Math.min(moveEvent.clientY - top, total - LIST_MIN)));
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  }, []);

  const selectVertex = useCallback((vertexId: number | null) => {
    setSelectedVertexId(vertexId);
    if (vertexId === null) {
      setDrawerFull(false);
    }
  }, []);

  if (graph.vertices.length === 0) {
    return <div className={cn('flex h-full items-center justify-center text-sm text-muted-foreground', className)}>{t('noTopology')}</div>;
  }

  return (
    <div className={cn('flex h-full min-h-0 flex-col gap-2', className)}>
      {runtime && realtimeEnabled ? <RealtimeHintsBar state={realtime} vertices={graph.vertices} /> : null}
      {warnings && warnings.length > 0 ? (
        <div className='flex flex-wrap gap-1.5'>
          {warnings.map((warning, index) => (
            <Badge key={`${warning}-${index}`} variant='outline' className='text-[11px] font-normal text-amber-700 dark:text-amber-300'>
              {warning}
            </Badge>
          ))}
        </div>
      ) : null}
      <div className='flex min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
        <div ref={splitRef} className={cn('flex min-w-0 flex-1 flex-col', drawerFull && selectedVertex && 'hidden')}>
          <div className='relative min-h-0 shrink-0' style={{height: graphHeight ?? '62%'}}>
            <JobDagCanvas
              graph={graph}
              mode={mode}
              metricIndex={metricIndex}
              overlay={overlay}
              selectedVertexId={selectedVertexId}
              onSelectVertex={selectVertex}
            />
          </div>
          <div
            role='separator'
            aria-orientation='horizontal'
            aria-label={t('resizeSplit')}
            className='flex h-2 shrink-0 cursor-row-resize items-center justify-center border-y border-border/60 bg-muted/30 hover:bg-muted'
            onPointerDown={onResizeStart}
          >
            <GripHorizontal className='size-3 text-muted-foreground' />
          </div>
          <div className='min-h-0 flex-1 overflow-auto'>
            <OperatorList
              vertices={graph.vertices}
              mode={mode}
              metricIndex={metricIndex}
              overlay={overlay}
              selectedVertexId={selectedVertexId}
              onSelectVertex={selectVertex}
            />
          </div>
        </div>
        {selectedVertex ? (
          <div className={cn('min-h-0 shrink-0', drawerFull ? 'flex-1' : 'w-[42%] min-w-[340px] max-w-[560px]')}>
            <DagNodeDrawer
              vertex={selectedVertex}
              mode={mode}
              metricIndex={metricIndex}
              overlay={overlay}
              plan={
                runtime
                  ? {
                      loading: plan.loading,
                      available: Boolean(plan.data?.available),
                      reason: plan.data?.reason,
                      message: plan.data?.message,
                      error: plan.error,
                      onRefresh: plan.refresh,
                    }
                  : undefined
              }
              full={drawerFull}
              onToggleFull={() => setDrawerFull((value) => !value)}
              onClose={() => selectVertex(null)}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
}
