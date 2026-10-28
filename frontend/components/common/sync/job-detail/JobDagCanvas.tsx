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

import '@xyflow/react/dist/style.css';

import {memo, useEffect, useMemo, useRef} from 'react';
import {
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  getBezierPath,
  useReactFlow,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
} from '@xyflow/react';
import {Copy, Table2} from 'lucide-react';
import {useTheme} from 'next-themes';
import {useTranslations} from 'next-intl';
import {cn} from '@/lib/utils';
import {
  backpressureStroke,
  backpressureTone,
  formatCount,
  formatPercent,
  nodeColors,
  shortTablePath,
  structureKey,
  type DagGraph,
  type DagVertex,
  type MetricIndex,
  type RealtimeOverlay,
  type TableStats,
} from './dag-model';
import {copyText} from './copy-text';
import {layoutDag} from './dag-layout';
import {OperatorIcon, operatorAccent} from './operator-style';

export type DagMode = 'plan' | 'runtime';

// 节点固定尺寸（同 Flink 固定宽），布局只取决于结构；长表名靠中间省略 + 悬停 + 复制 + 抽屉完整展示。
// Fixed node size (Flink also uses a fixed width) so layout depends only on structure; long table names rely on middle
// ellipsis, hover, copy and the drawer for the full value.
export const NODE_WIDTH = 260;
const NODE_HEIGHT: Record<DagMode, number> = {plan: 86, runtime: 138};
const TABLE_LABEL_CHARS = 30;

interface OperatorNodeData extends Record<string, unknown> {
  vertex: DagVertex;
  mode: DagMode;
  stats?: TableStats;
  busy?: number;
  backpressured?: number;
  selected: boolean;
  dimmed: boolean;
}

interface OperatorEdgeData extends Record<string, unknown> {
  mode: DagMode;
  backpressure?: number;
  focused: boolean;
  dimmed: boolean;
}

type OperatorNodeType = Node<OperatorNodeData, 'operator'>;
type OperatorEdgeType = Edge<OperatorEdgeData, 'operator'>;

// 被反压程度达到这个值才给卡片染色，低负载时保持中性，颜色只用来提示问题。
// A card is tinted only once busyness / backpressure reaches this level; at low load it stays neutral, so color only
// signals problems.
const HEALTH_TINT_THRESHOLD = 0.2;

/**
 * 卡片里的迷你进度条：忙碌度用 Flink 的蓝→红混色，被反压用与连线相同的分档色。
 * Mini bar inside the card: busyness uses Flink's blue→red blend, backpressure the same tiers as the edges.
 */
function HealthBar({label, value, color, hint}: {label: string; value?: number; color?: string; hint: string}) {
  const ratio = typeof value === 'number' && Number.isFinite(value) ? Math.min(1, Math.max(0, value)) : null;
  return (
    <div className='flex min-w-0 flex-1 items-center gap-1.5' title={hint}>
      <span className='shrink-0 text-[10px] uppercase tracking-wide text-muted-foreground'>{label}</span>
      <div className='h-1 min-w-0 flex-1 overflow-hidden rounded-full bg-muted'>
        {ratio !== null ? (
          <div className='h-full rounded-full transition-[width] duration-500' style={{width: `${Math.max(ratio * 100, 2)}%`, backgroundColor: color ?? 'var(--muted-foreground)'}} />
        ) : null}
      </div>
      <span className='w-8 shrink-0 text-right font-mono text-[10px] text-foreground/80'>{formatPercent(value)}</span>
    </div>
  );
}

/**
 * 算子卡片：类型只靠小图标和一行灰色元信息表达；运行中按 Flink 规则用边框和底色表达健康度，且只在负载明显时出现。
 * Operator card: the type is conveyed only by a small icon and a muted meta line; in runtime mode health follows Flink's
 * rules through border and background color, shown only when the load is noticeable.
 */
const OperatorNode = memo(function OperatorNode({data}: NodeProps<OperatorNodeType>) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const {vertex, mode, stats, busy, backpressured, selected, dimmed} = data;
  const runtime = mode === 'runtime';
  const colors = runtime ? nodeColors(busy, backpressured) : null;
  const tinted = colors !== null && colors.intensity >= HEALTH_TINT_THRESHOLD;
  const firstTable = vertex.tablePaths[0];
  const moreTables = vertex.tablePaths.length - 1;
  const isSink = vertex.type === 'sink';
  const qps = isSink ? stats?.writeQps : stats?.readQps;

  return (
    <div
      className={cn(
        'group relative flex h-full flex-col overflow-hidden rounded-lg border border-border bg-card text-card-foreground shadow-xs transition-[opacity,box-shadow,border-color] duration-200',
        selected ? 'ring-2 ring-primary/50 shadow-md' : 'hover:border-foreground/25 hover:shadow-sm',
        dimmed && 'opacity-35',
      )}
      style={tinted ? {borderColor: colors.border, backgroundColor: colors.background} : undefined}
      data-testid={`dag-node-${vertex.vertexId}`}
    >
      <Handle type='target' position={Position.Left} isConnectable={false} className='!size-1.5 !border-0 !bg-muted-foreground/40' />
      <div className='flex flex-col gap-1 px-3 pt-2.5'>
        <div className='flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground'>
          <OperatorIcon type={vertex.type} className='size-3' />
          <span className='truncate uppercase tracking-wide'>{vertex.metricPrefix ?? vertex.type}</span>
          <span className='text-muted-foreground/50'>·</span>
          <span>#{vertex.vertexId}</span>
          {vertex.pipeline ? <span className='ml-auto'>P{vertex.pipeline}</span> : null}
        </div>
        <div className='truncate text-[13px] font-semibold leading-5' title={vertex.rawName}>
          {vertex.connector}
        </div>
        <div className='flex h-4 min-w-0 items-center gap-1 text-[11px] text-muted-foreground'>
          <Table2 className='size-3 shrink-0' />
          {firstTable ? (
            <>
              <span className='min-w-0 truncate font-mono' title={firstTable}>
                {shortTablePath(firstTable, moreTables > 0 ? TABLE_LABEL_CHARS - 6 : TABLE_LABEL_CHARS)}
              </span>
              {moreTables > 0 ? <span className='shrink-0 rounded bg-muted px-1 font-mono text-[10px]'>+{moreTables}</span> : null}
              <button
                type='button'
                className='nodrag ml-auto shrink-0 rounded p-0.5 opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100'
                aria-label={t('copyTableName')}
                title={t('copyTableName')}
                onClick={(event) => {
                  event.stopPropagation();
                  copyText(firstTable, t('copied'));
                }}
              >
                <Copy className='size-3' />
              </button>
            </>
          ) : (
            <span>{t('noTables')}</span>
          )}
        </div>
      </div>
      {runtime ? (
        <div className='mt-auto space-y-1.5 border-t border-border/60 px-3 pb-2.5 pt-2'>
          <div className='flex items-center gap-2 font-mono text-[11px] text-muted-foreground'>
            <span>{isSink ? t('writeShort', {value: formatCount(stats?.writeCount)}) : t('readShort', {value: formatCount(stats?.readCount)})}</span>
            <span className='ml-auto'>{typeof qps === 'number' ? `${formatCount(qps)}/s` : ''}</span>
          </div>
          <div className='flex items-center gap-3'>
            <HealthBar label='Busy' value={busy} color={nodeColors(busy, undefined)?.border} hint={t('busyMaxHint')} />
            {/* Sink 没有下游，不存在被反压，只显示忙碌度。/ Sinks have no downstream, so they cannot be backpressured. */}
            {!isSink ? (
              <HealthBar label='BP' value={backpressured} color={backpressureStroke(backpressured)} hint={t('backpressuredMaxHint')} />
            ) : null}
          </div>
        </div>
      ) : null}
      <Handle type='source' position={Position.Right} isConnectable={false} className='!size-1.5 !border-0 !bg-muted-foreground/40' />
    </div>
  );
});

function OperatorEdge({id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, markerEnd, data}: EdgeProps<OperatorEdgeType>) {
  const [path, labelX, labelY] = getBezierPath({sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition});
  const bp = data?.backpressure;
  const stroke = data?.mode === 'runtime' ? backpressureStroke(bp) : undefined;
  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        markerEnd={markerEnd}
        style={{
          stroke: stroke ?? 'var(--muted-foreground)',
          strokeOpacity: data?.dimmed ? 0.2 : data?.focused ? 1 : 0.6,
          strokeWidth: data?.focused ? 2.25 : 1.5,
        }}
      />
      {data?.mode === 'runtime' && typeof bp === 'number' ? (
        <EdgeLabelRenderer>
          <div
            className={cn(
              'pointer-events-none absolute rounded border bg-background px-1 py-px font-mono text-[10px] shadow-sm',
              backpressureTone(bp),
              data?.dimmed && 'opacity-30',
            )}
            style={{transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`}}
          >
            BP {formatPercent(bp)}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}

const nodeTypes = {operator: OperatorNode};
const edgeTypes = {operator: OperatorEdge};

export interface JobDagCanvasProps {
  graph: DagGraph;
  mode: DagMode;
  metricIndex?: MetricIndex;
  overlay?: RealtimeOverlay;
  selectedVertexId: number | null;
  onSelectVertex: (vertexId: number | null) => void;
}

function JobDagCanvasInner({graph, mode, metricIndex, overlay, selectedVertexId, onSelectVertex}: JobDagCanvasProps) {
  const {resolvedTheme} = useTheme();
  const reactFlow = useReactFlow<OperatorNodeType, OperatorEdgeType>();
  const nodeHeight = NODE_HEIGHT[mode];
  const layoutKey = `${structureKey(graph)}|${mode}`;

  // 布局只依赖结构签名：指标与实时数据刷新不会触发 dagre 重排（同 Flink updateNode 只改数据）。
  // Layout depends only on the structure signature: metric and realtime refreshes never rerun dagre (like Flink's
  // updateNode, which only changes data).
  const positions = useMemo(
    () => layoutDag(graph, {width: NODE_WIDTH, height: nodeHeight}),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [layoutKey],
  );

  const neighbors = useMemo(() => {
    const ids = new Set<number>();
    if (selectedVertexId === null) {
      return ids;
    }
    ids.add(selectedVertexId);
    for (const edge of graph.edges) {
      if (edge.source === selectedVertexId) {
        ids.add(edge.target);
      }
      if (edge.target === selectedVertexId) {
        ids.add(edge.source);
      }
    }
    return ids;
  }, [graph.edges, selectedVertexId]);

  const nodes = useMemo<OperatorNodeType[]>(
    () =>
      graph.vertices.map((vertex) => ({
        id: String(vertex.vertexId),
        type: 'operator',
        position: positions.get(vertex.vertexId) ?? {x: 0, y: 0},
        width: NODE_WIDTH,
        height: nodeHeight,
        draggable: false,
        connectable: false,
        data: {
          vertex,
          mode,
          stats: vertex.metricPrefix ? metricIndex?.vertices.get(vertex.metricPrefix) : undefined,
          busy: overlay?.busy.get(vertex.vertexId),
          backpressured: overlay?.backpressured.get(vertex.vertexId),
          selected: vertex.vertexId === selectedVertexId,
          dimmed: selectedVertexId !== null && !neighbors.has(vertex.vertexId),
        },
      })),
    [graph.vertices, positions, nodeHeight, mode, metricIndex, overlay, selectedVertexId, neighbors],
  );

  const edges = useMemo<OperatorEdgeType[]>(
    () =>
      graph.edges.map((edge) => {
        const focused = selectedVertexId !== null && (edge.source === selectedVertexId || edge.target === selectedVertexId);
        return {
          id: edge.id,
          source: String(edge.source),
          target: String(edge.target),
          type: 'operator',
          markerEnd: {type: MarkerType.ArrowClosed, width: 14, height: 14},
          data: {
            mode,
            backpressure: overlay?.edgeBackpressure.get(edge.id),
            focused,
            dimmed: selectedVertexId !== null && !focused,
          },
        };
      }),
    [graph.edges, mode, overlay, selectedVertexId],
  );

  // 结构变化后重新适应视图；选中节点时把它和上下游一起适配进视口（同 Flink focusNode 高亮相邻节点），取消选中时恢复全图。
  // 抽屉开合会改变画布宽度，React Flow 通过 ResizeObserver 异步拿到新尺寸，所以稍等再适配。
  // Refit after structural changes; on selection fit the node together with its upstream and downstream (like Flink's
  // focusNode highlighting neighbors), and restore the full view on deselection. Opening or closing the drawer changes
  // the canvas width, which React Flow learns asynchronously through a ResizeObserver, so the fit waits briefly.
  const FIT_DELAY_MS = 80;
  const firstFit = useRef(true);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      void reactFlow.fitView({padding: 0.2, duration: firstFit.current ? 0 : 250, maxZoom: 1.1});
      firstFit.current = false;
    }, firstFit.current ? 0 : FIT_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [layoutKey, reactFlow]);

  useEffect(() => {
    const focus = selectedVertexId === null ? undefined : [...neighbors].map((id) => ({id: String(id)}));
    const timer = window.setTimeout(() => {
      void reactFlow.fitView({nodes: focus, padding: focus ? 0.25 : 0.2, duration: 300, maxZoom: 1.1});
    }, FIT_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [selectedVertexId, neighbors, reactFlow]);

  return (
    <ReactFlow<OperatorNodeType, OperatorEdgeType>
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      colorMode={resolvedTheme === 'dark' ? 'dark' : 'light'}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      onlyRenderVisibleElements
      minZoom={0.2}
      maxZoom={2}
      onNodeClick={(_, node) => onSelectVertex(Number(node.id) === selectedVertexId ? null : Number(node.id))}
      onPaneClick={() => onSelectVertex(null)}
      proOptions={{hideAttribution: false}}
      attributionPosition='bottom-left'
    >
      <Background gap={20} size={1} />
      <Controls position='top-left' showInteractive={false} />
      {graph.vertices.length > 6 ? (
        <MiniMap
          pannable
          zoomable
          position='bottom-right'
          nodeColor={(node) => operatorAccent((node.data as OperatorNodeData).vertex.type)}
        />
      ) : null}
    </ReactFlow>
  );
}

/**
 * Flink 风格作业拓扑画布：dagre 左→右布局、只读节点、运行模式按忙碌度 / 被反压程度混色并在连线上标背压。
 * Flink-style job topology canvas: dagre left-to-right layout, read-only nodes, and in runtime mode node colors blended
 * by busyness / backpressure plus backpressure labels on edges.
 */
export default function JobDagCanvas(props: JobDagCanvasProps) {
  return (
    <ReactFlowProvider>
      <JobDagCanvasInner {...props} />
    </ReactFlowProvider>
  );
}
