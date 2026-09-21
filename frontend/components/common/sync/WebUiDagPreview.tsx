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

import {useMemo, useState} from 'react';
import {
  GitBranch,
  Database,
  Cpu,
  ArrowDownToLine,
  ZoomIn,
  ZoomOut,
  Sparkles,
} from 'lucide-react';
import type {
  SyncSinkSaveModePreviewTable,
  SyncWebUIDagEdge,
  SyncJSON,
  SyncWebUIDagPreviewJob,
  SyncWebUIDagVertexInfo,
} from '@/lib/services/sync';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';

interface PositionedNode extends SyncWebUIDagVertexInfo {
  level: number;
  row: number;
  x: number;
  y: number;
  height: number;
}

const NODE_WIDTH = 220;
const NODE_MIN_HEIGHT = 120;
const COLUMN_GAP = 92;
const ROW_GAP = 44;
const PADDING = 28;
const CANVAS_RIGHT_PADDING = 96;

interface SelectedTableDetailState {
  nodeLabel: string;
  tablePath: string;
  columns: string[];
  schema?: SyncJSON;
}

interface SelectedSaveModeDetailState {
  nodeLabel: string;
  tablePath: string;
  preview: SyncSinkSaveModePreviewTable;
}

interface SchemaColumnDetail {
  name: string;
  dataType: string;
  nullable: boolean | null;
  defaultValue: string;
  comment: string;
  primaryKey: boolean;
  uniqueKey: boolean;
}

interface TablePathPreviewItemProps {
  nodeLabel: string;
  path: string;
  preview?: SyncSinkSaveModePreviewTable;
  onSelectDetail: () => void;
  onPreviewDetail: () => void;
}

function toObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => String(item));
}

function extractSchemaColumnDetails(schema?: SyncJSON): SchemaColumnDetail[] {
  const root = toObject(schema);
  const schemaObject = toObject(root.schema);
  const primaryKeyColumns = new Set(
    toStringArray(toObject(schemaObject.primaryKey).columnNames),
  );
  const uniqueColumns = new Set<string>();
  const constraints = schemaObject.constraintKeys;
  if (Array.isArray(constraints)) {
    constraints.forEach((constraint) => {
      const constraintObject = toObject(constraint);
      if (String(constraintObject.constraintType || '') !== 'UNIQUE_KEY') {
        return;
      }
      const columns = constraintObject.columns;
      if (!Array.isArray(columns)) {
        return;
      }
      columns.forEach((column) => {
        const columnObject = toObject(column);
        const columnName = String(columnObject.columnName || '').trim();
        if (columnName) {
          uniqueColumns.add(columnName);
        }
      });
    });
  }

  const rawColumns = Array.isArray(schemaObject.columns) ? schemaObject.columns : [];
  return rawColumns.map((column) => {
    const columnObject = toObject(column);
    const name = String(columnObject.name || '-');
    return {
      name,
      dataType: String(columnObject.dataType || '-'),
      nullable:
        typeof columnObject.nullable === 'boolean'
          ? columnObject.nullable
          : null,
      defaultValue:
        columnObject.defaultValue === null ||
        typeof columnObject.defaultValue === 'undefined'
          ? '-'
          : String(columnObject.defaultValue),
      comment: String(columnObject.comment || '-'),
      primaryKey: primaryKeyColumns.has(name),
      uniqueKey: uniqueColumns.has(name),
    };
  });
}

function normalizeVertices(
  job: SyncWebUIDagPreviewJob,
): SyncWebUIDagVertexInfo[] {
  return Object.values(job.jobDag?.vertexInfoMap || {}).sort(
    (left, right) => left.vertexId - right.vertexId,
  );
}

function normalizeEdges(job: SyncWebUIDagPreviewJob): SyncWebUIDagEdge[] {
  return Object.entries(job.jobDag?.pipelineEdges || {})
    .sort(([left], [right]) => Number(left) - Number(right))
    .flatMap(([, edges]) => edges || []);
}

function computeNodeLevels(
  vertices: SyncWebUIDagVertexInfo[],
  edges: SyncWebUIDagEdge[],
): PositionedNode[] {
  const incoming = new Map<number, number>();
  const outgoing = new Map<number, number[]>();
  const vertexByID = new Map<number, SyncWebUIDagVertexInfo>();

  for (const vertex of vertices) {
    incoming.set(vertex.vertexId, 0);
    outgoing.set(vertex.vertexId, []);
    vertexByID.set(vertex.vertexId, vertex);
  }

  for (const edge of edges) {
    outgoing.set(edge.inputVertexId, [
      ...(outgoing.get(edge.inputVertexId) || []),
      edge.targetVertexId,
    ]);
    incoming.set(
      edge.targetVertexId,
      (incoming.get(edge.targetVertexId) || 0) + 1,
    );
  }

  const queue = vertices
    .filter((vertex) => (incoming.get(vertex.vertexId) || 0) === 0)
    .map((vertex) => vertex.vertexId);

  const levels = new Map<number, number>();
  for (const vertex of vertices) {
    levels.set(vertex.vertexId, 0);
  }

  while (queue.length > 0) {
    const current = queue.shift()!;
    const currentLevel = levels.get(current) || 0;
    for (const next of outgoing.get(current) || []) {
      levels.set(next, Math.max(levels.get(next) || 0, currentLevel + 1));
      incoming.set(next, Math.max(0, (incoming.get(next) || 1) - 1));
      if ((incoming.get(next) || 0) === 0) {
        queue.push(next);
      }
    }
  }

  const rowsByLevel = new Map<number, number>();
  const yOffsetByLevel = new Map<number, number>();
  return vertices.map((vertex) => {
    const level = levels.get(vertex.vertexId) || 0;
    const row = rowsByLevel.get(level) || 0;
    const height = estimateNodeHeight(vertex);
    const y = yOffsetByLevel.get(level) || PADDING;
    rowsByLevel.set(level, row + 1);
    yOffsetByLevel.set(level, y + height + ROW_GAP);
    return {
      ...vertex,
      level,
      row,
      x: PADDING + level * (NODE_WIDTH + COLUMN_GAP),
      y,
      height,
    };
  });
}

function nodeTone(type: string, isSelected: boolean = false): string {
  if (isSelected) {
    return 'border-primary ring-2 ring-primary/40 shadow-lg shadow-primary/10 bg-background/95 dark:bg-card/95';
  }
  switch (type.toLowerCase()) {
    case 'source':
      return 'border-emerald-500/50 hover:border-emerald-500 shadow-sm shadow-emerald-500/5 bg-gradient-to-b from-emerald-500/[0.08] to-background/90 dark:to-card/90';
    case 'sink':
      return 'border-indigo-500/50 hover:border-indigo-500 shadow-sm shadow-indigo-500/5 bg-gradient-to-b from-indigo-500/[0.08] to-background/90 dark:to-card/90';
    default:
      return 'border-amber-500/50 hover:border-amber-500 shadow-sm shadow-amber-500/5 bg-gradient-to-b from-amber-500/[0.08] to-background/90 dark:to-card/90';
  }
}

function nodeAccentBar(type: string): string {
  switch (type.toLowerCase()) {
    case 'source':
      return 'bg-gradient-to-r from-emerald-500 to-teal-400';
    case 'sink':
      return 'bg-gradient-to-r from-indigo-500 to-purple-500';
    default:
      return 'bg-gradient-to-r from-amber-500 to-orange-400';
  }
}

function nodeBadgeTone(type: string): string {
  switch (type.toLowerCase()) {
    case 'source':
      return 'border-emerald-500/30 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300';
    case 'sink':
      return 'border-indigo-500/30 bg-indigo-500/15 text-indigo-700 dark:text-indigo-300';
    default:
      return 'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300';
  }
}

function renderNodeIcon(type: string) {
  switch (type.toLowerCase()) {
    case 'source':
      return <Database className='size-3.5 text-emerald-600 dark:text-emerald-400' />;
    case 'sink':
      return <ArrowDownToLine className='size-3.5 text-indigo-600 dark:text-indigo-400' />;
    default:
      return <Cpu className='size-3.5 text-amber-600 dark:text-amber-400' />;
  }
}

function normalizeTablePaths(paths?: string[]): string[] {
  return (paths || []).filter(Boolean);
}

function resolveSaveModePreview(
  node: SyncWebUIDagVertexInfo,
  path: string,
): SyncSinkSaveModePreviewTable | undefined {
  const previews = node.saveModePreviews || {};
  return previews[path] || previews.__default__;
}

function SaveModePreviewDetail({
  preview,
}: {
  preview: SyncSinkSaveModePreviewTable;
}) {
  const actions = preview.actions || [];
  const warnings = preview.warnings || [];

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap gap-2'>
        {preview.schemaSaveMode ? (
          <Badge variant='outline' className='text-xs'>
            Schema: {preview.schemaSaveMode}
          </Badge>
        ) : null}
        {preview.dataSaveMode ? (
          <Badge variant='outline' className='text-xs'>
            Data: {preview.dataSaveMode}
          </Badge>
        ) : null}
        {preview.completeness ? (
          <Badge variant='secondary' className='text-xs'>
            {preview.completeness}
          </Badge>
        ) : null}
      </div>
      {warnings.length > 0 ? (
        <div className='space-y-1 text-sm text-amber-700 dark:text-amber-300'>
          {warnings.map((warning, index) => (
            <div key={`${warning}-${index}`}>{warning}</div>
          ))}
        </div>
      ) : null}
      {actions.length > 0 ? (
        <div className='space-y-1'>
          {actions.map((action, index) => (
            <div
              key={`${action.actionType || 'action'}-${index}`}
              className='rounded border border-border/50 bg-background/80 p-2'
            >
              <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
                {action.phase ? <span>{action.phase}</span> : null}
                {action.actionType ? <span>{action.actionType}</span> : null}
                {action.resultType ? <span>{action.resultType}</span> : null}
              </div>
              {action.content ? (
                <pre className='mt-1 whitespace-pre-wrap break-all font-mono text-xs text-foreground'>
                  {action.content}
                </pre>
              ) : null}
            </div>
          ))}
        </div>
      ) : warnings.length === 0 ? (
        <div className='text-sm text-muted-foreground'>
          暂无可预览的 SQL / 动作
        </div>
      ) : null}
    </div>
  );
}

function TablePathPreviewItem({
  nodeLabel,
  path,
  preview,
  onSelectDetail,
  onPreviewDetail,
}: TablePathPreviewItemProps) {
  return (
    <div className='rounded-md border border-border/50 bg-background/80 px-2 py-1.5'>
      <div className='flex items-center gap-2'>
        <button
          type='button'
          title={path}
          className='min-w-0 flex-1 text-left text-[11px] hover:text-foreground'
          onClick={onSelectDetail}
        >
          <span className='block truncate'>{path}</span>
        </button>
        {preview ? (
          <button
            type='button'
            className='inline-flex items-center rounded border border-border/60 px-1.5 py-0.5 text-[10px] text-muted-foreground hover:bg-muted'
            title={`${nodeLabel} / ${path}`}
            onClick={onPreviewDetail}
          >
            预览 DDL
          </button>
        ) : null}
      </div>
    </div>
  );
}

function estimateNodeHeight(node: SyncWebUIDagVertexInfo): number {
  const connectorLines = Math.max(
    1,
    Math.ceil((node.connectorType?.length || 0) / 22),
  );
  const tableCount = Math.max(1, normalizeTablePaths(node.tablePaths).length);
  const headerHeight = 72;
  const connectorHeight = connectorLines * 18;
  const tableSectionLabelHeight = 20;
  const tableButtonHeight = tableCount * 30;
  const tableButtonGap = Math.max(0, tableCount - 1) * 6;
  const bottomPadding = 22;
  return Math.max(
    NODE_MIN_HEIGHT,
    headerHeight +
      connectorHeight +
      tableSectionLabelHeight +
      tableButtonHeight +
      tableButtonGap +
      bottomPadding,
  );
}

export function WebUiDagPreview({job}: {job: SyncWebUIDagPreviewJob}) {
  const [selectedTableDetail, setSelectedTableDetail] =
    useState<SelectedTableDetailState | null>(null);
  const [selectedSaveModeDetail, setSelectedSaveModeDetail] =
    useState<SelectedSaveModeDetailState | null>(null);
  const [selectedVertexId, setSelectedVertexId] = useState<number | null>(null);
  const [zoom, setZoom] = useState<number>(1.0);
  const [isFlowActive, setIsFlowActive] = useState<boolean>(true);

  const selectedSchemaColumns = useMemo(
    () => extractSchemaColumnDetails(selectedTableDetail?.schema),
    [selectedTableDetail],
  );
  const vertices = useMemo(() => normalizeVertices(job), [job]);
  const edges = useMemo(() => normalizeEdges(job), [job]);
  const positionedNodes = useMemo(
    () => computeNodeLevels(vertices, edges),
    [vertices, edges],
  );
  const nodeByID = useMemo(
    () =>
      new Map(positionedNodes.map((node) => [node.vertexId, node] as const)),
    [positionedNodes],
  );

  // 拓扑统计
  // Topology statistics
  const stats = useMemo(() => {
    let sources = 0;
    let sinks = 0;
    let transforms = 0;
    let totalTables = 0;
    for (const v of vertices) {
      const type = (v.type || '').toLowerCase();
      if (type === 'source') {
        sources += 1;
      } else if (type === 'sink') {
        sinks += 1;
      } else {
        transforms += 1;
      }
      totalTables += (v.tablePaths || []).length;
    }
    return {sources, sinks, transforms, totalTables};
  }, [vertices]);

  const selectedNode = useMemo(
    () => (selectedVertexId !== null ? nodeByID.get(selectedVertexId) : null),
    [selectedVertexId, nodeByID],
  );

  const width =
    positionedNodes.length === 0
      ? 0
      : Math.max(...positionedNodes.map((node) => node.x)) +
        NODE_WIDTH +
        PADDING +
        CANVAS_RIGHT_PADDING;
  const height =
    positionedNodes.length === 0
      ? 0
      : Math.max(...positionedNodes.map((node) => node.y + node.height)) +
        PADDING;

  return (
    <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]'>
      <style>{`
        @keyframes stx-dag-flow {
          from {
            stroke-dashoffset: 48;
          }
          to {
            stroke-dashoffset: 0;
          }
        }
        .stx-dag-flowing-path {
          stroke-dasharray: 6, 6;
          animation: stx-dag-flow 1.8s linear infinite;
        }
      `}</style>

      {/* 左侧拓扑画布 */}
      <Card className='relative flex flex-col overflow-hidden border-border/60 bg-background/60 shadow-xs'>
        <CardHeader className='border-b border-border/50 px-4 py-2.5 bg-muted/15 flex flex-row items-center justify-between'>
          <CardTitle className='flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-foreground/80'>
            <GitBranch className='size-3.5 text-primary/80' />
            <span>执行拓扑图 (Execution DAG)</span>
          </CardTitle>
          <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
            <Badge variant='outline' className='h-5 rounded px-1.5 text-[10px] font-mono'>
              {vertices.length} 算子
            </Badge>
            <span className='text-muted-foreground/50'>·</span>
            <Badge variant='outline' className='h-5 rounded px-1.5 text-[10px] font-mono'>
              {edges.length} 数据流
            </Badge>
          </div>
        </CardHeader>

        <CardContent className='relative p-0 overflow-hidden flex-1 min-h-[580px]'>
          <div className='h-[580px] w-full overflow-x-auto overflow-y-auto'>
            <div
              className='relative min-h-[580px] min-w-max transition-transform origin-top-left duration-150'
              style={{
                width: Math.max(width, 860),
                height: Math.max(height, 580),
                transform: `scale(${zoom})`,
                backgroundImage:
                  'radial-gradient(circle at 1px 1px, currentColor 1px, transparent 0)',
                backgroundSize: '24px 24px',
              }}
            >
              {/* 点阵微弱色彩 */}
              <div className='absolute inset-0 bg-muted/15 opacity-60 pointer-events-none' />

              <svg
                className='absolute inset-0 pointer-events-none'
                width={Math.max(width, 860)}
                height={Math.max(height, 580)}
              >
                <defs>
                  {/* 标准箭头 */}
                  <marker
                    id='sync-dag-arrow'
                    markerWidth='8'
                    markerHeight='8'
                    refX='7'
                    refY='4'
                    orient='auto'
                  >
                    <path
                      d='M0,1 L7,4 L0,7 z'
                      className='fill-muted-foreground/70'
                    />
                  </marker>
                  {/* 高亮激活箭头 */}
                  <marker
                    id='sync-dag-arrow-active'
                    markerWidth='9'
                    markerHeight='9'
                    refX='8'
                    refY='4'
                    orient='auto'
                  >
                    <path
                      d='M0,1 L8,4 L0,7 z'
                      className='fill-primary'
                    />
                  </marker>
                </defs>

                {edges.map((edge, index) => {
                  const source = nodeByID.get(edge.inputVertexId);
                  const target = nodeByID.get(edge.targetVertexId);
                  if (!source || !target) {
                    return null;
                  }
                  const isEdgeConnectedToSelected =
                    selectedVertexId !== null &&
                    (edge.inputVertexId === selectedVertexId ||
                      edge.targetVertexId === selectedVertexId);

                  const x1 = source.x + NODE_WIDTH;
                  const y1 = source.y + source.height / 2;
                  const x2 = target.x;
                  const y2 = target.y + target.height / 2;
                  const midX = x1 + (x2 - x1) / 2;
                  const path = `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`;

                  return (
                    <g key={`${edge.inputVertexId}-${edge.targetVertexId}-${index}`}>
                      {/* 底层光晕微光线 */}
                      <path
                        d={path}
                        fill='none'
                        stroke={isEdgeConnectedToSelected ? 'var(--primary)' : 'currentColor'}
                        strokeWidth={isEdgeConnectedToSelected ? '5' : '3'}
                        className={cn(
                          'transition-all duration-200',
                          isEdgeConnectedToSelected
                            ? 'opacity-30'
                            : 'opacity-10 text-foreground',
                        )}
                      />
                      {/* 顶层主干或流光虚线 */}
                      <path
                        d={path}
                        fill='none'
                        stroke={isEdgeConnectedToSelected ? 'var(--primary)' : 'currentColor'}
                        strokeWidth={isEdgeConnectedToSelected ? '2' : '1.75'}
                        className={cn(
                          'transition-all duration-200',
                          isEdgeConnectedToSelected
                            ? 'text-primary opacity-90'
                            : 'text-muted-foreground/60',
                          isFlowActive && 'stx-dag-flowing-path',
                        )}
                        markerEnd={
                          isEdgeConnectedToSelected
                            ? 'url(#sync-dag-arrow-active)'
                            : 'url(#sync-dag-arrow)'
                        }
                      />
                    </g>
                  );
                })}
              </svg>

              {positionedNodes.map((node) => {
                const isSelected = selectedVertexId === node.vertexId;
                return (
                  <div
                    key={node.vertexId}
                    className={cn(
                      'absolute rounded-xl border backdrop-blur-md cursor-pointer transition-all duration-200 select-none overflow-hidden group hover:scale-[1.01]',
                      nodeTone(node.type, isSelected),
                    )}
                    style={{
                      left: node.x,
                      top: node.y,
                      width: NODE_WIDTH,
                      minHeight: node.height,
                    }}
                    onClick={() =>
                      setSelectedVertexId((prev) =>
                        prev === node.vertexId ? null : node.vertexId,
                      )
                    }
                  >
                    {/* 顶部彩色发光装饰条 */}
                    <div className={cn('h-1 w-full', nodeAccentBar(node.type))} />

                    <div className='flex h-full flex-col gap-2.5 p-3.5'>
                      {/* 头部：类型徽标与 ID */}
                      <div className='flex items-center justify-between gap-2'>
                        <div className='flex items-center gap-1.5'>
                          {renderNodeIcon(node.type)}
                          <Badge
                            variant='outline'
                            className={cn(
                              'rounded-sm text-[10px] font-semibold uppercase tracking-wider px-1.5 py-0',
                              nodeBadgeTone(node.type),
                            )}
                          >
                            {node.type}
                          </Badge>
                        </div>
                        <span className='font-mono text-[10px] text-muted-foreground font-medium'>
                          #{node.vertexId}
                        </span>
                      </div>

                      {/* 算子连接器名称 */}
                      <div className='space-y-1.5'>
                        <div
                          className='line-clamp-2 text-xs font-semibold text-foreground group-hover:text-primary transition-colors'
                          title={node.connectorType}
                        >
                          {node.connectorType}
                        </div>

                        {/* 表路径容器 */}
                        <div className='space-y-1'>
                          <div className='flex items-center justify-between text-[10px] font-medium uppercase tracking-wider text-muted-foreground'>
                            <span>TablePaths</span>
                            <span className='font-mono text-[9px] opacity-70'>
                              {normalizeTablePaths(node.tablePaths).length} 表
                            </span>
                          </div>
                          <div className='space-y-1.5'>
                            {normalizeTablePaths(node.tablePaths).length > 0 ? (
                              normalizeTablePaths(node.tablePaths).map((path) => (
                                <TablePathPreviewItem
                                  key={`${node.vertexId}-${path}`}
                                  nodeLabel={`#${node.vertexId} ${node.connectorType}`}
                                  path={path}
                                  preview={resolveSaveModePreview(node, path)}
                                  onSelectDetail={() =>
                                    setSelectedTableDetail({
                                      nodeLabel: `#${node.vertexId} ${node.connectorType}`,
                                      tablePath: path,
                                      columns: node.tableColumns?.[path] || [],
                                      schema: node.tableSchemas?.[path],
                                    })
                                  }
                                  onPreviewDetail={() => {
                                    const preview = resolveSaveModePreview(node, path);
                                    if (!preview) {
                                      return;
                                    }
                                    setSelectedSaveModeDetail({
                                      nodeLabel: `#${node.vertexId} ${node.connectorType}`,
                                      tablePath: path,
                                      preview,
                                    });
                                  }}
                                />
                              ))
                            ) : (
                              <Badge
                                variant='secondary'
                                className='rounded border border-border/40 bg-background/60 text-[10px] text-muted-foreground'
                              >
                                default
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* 悬浮视口操控工具栏 (Floating Toolbar) */}
          <div className='absolute bottom-3 left-3 flex items-center gap-1 rounded-lg border border-border/60 bg-background/90 p-1 shadow-md backdrop-blur-md'>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size='icon'
                  variant='ghost'
                  className='size-7 text-muted-foreground hover:text-foreground'
                  onClick={() => setZoom((prev) => Math.min(1.4, Number((prev + 0.15).toFixed(2))))}
                >
                  <ZoomIn className='size-3.5' />
                </Button>
              </TooltipTrigger>
              <TooltipContent side='top'>放大画布 (+15%)</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size='icon'
                  variant='ghost'
                  className='size-7 text-muted-foreground hover:text-foreground'
                  onClick={() => setZoom((prev) => Math.max(0.6, Number((prev - 0.15).toFixed(2))))}
                >
                  <ZoomOut className='size-3.5' />
                </Button>
              </TooltipTrigger>
              <TooltipContent side='top'>缩小画布 (-15%)</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size='icon'
                  variant='ghost'
                  className='size-7 text-muted-foreground hover:text-foreground font-mono text-[10px]'
                  onClick={() => setZoom(1.0)}
                >
                  {Math.round(zoom * 100)}%
                </Button>
              </TooltipTrigger>
              <TooltipContent side='top'>重置为 100%</TooltipContent>
            </Tooltip>

            <div className='h-3.5 w-px bg-border/60 mx-0.5' />

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size='icon'
                  variant='ghost'
                  className={cn(
                    'size-7 text-muted-foreground hover:text-foreground',
                    isFlowActive && 'text-primary bg-primary/10',
                  )}
                  onClick={() => setIsFlowActive((prev) => !prev)}
                >
                  <Sparkles className='size-3.5' />
                </Button>
              </TooltipTrigger>
              <TooltipContent side='top'>
                {isFlowActive ? '关闭流光动态' : '开启流光动态'}
              </TooltipContent>
            </Tooltip>
          </div>
        </CardContent>
      </Card>

      {/* 右侧拓扑属性与联动面板 */}
      <div className='space-y-3.5'>
        {/* 拓扑全貌极简指标卡 */}
        <div className='grid grid-cols-3 gap-2'>
          <div className='rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-2 text-center'>
            <div className='text-[10px] text-emerald-600 dark:text-emerald-400 font-medium uppercase'>输入源</div>
            <div className='text-base font-bold text-foreground font-mono mt-0.5'>{stats.sources}</div>
          </div>
          <div className='rounded-lg border border-amber-500/30 bg-amber-500/5 p-2 text-center'>
            <div className='text-[10px] text-amber-600 dark:text-amber-400 font-medium uppercase'>算子</div>
            <div className='text-base font-bold text-foreground font-mono mt-0.5'>{stats.transforms}</div>
          </div>
          <div className='rounded-lg border border-indigo-500/30 bg-indigo-500/5 p-2 text-center'>
            <div className='text-[10px] text-indigo-600 dark:text-indigo-400 font-medium uppercase'>目标源</div>
            <div className='text-base font-bold text-foreground font-mono mt-0.5'>{stats.sinks}</div>
          </div>
        </div>

        {/* 选中的节点属性 或 作业全景摘要 */}
        {selectedNode ? (
          <Card className='border-primary/40 shadow-xs'>
            <CardHeader className='pb-2.5 pt-3.5 px-4 flex flex-row items-center justify-between border-b border-border/40'>
              <div className='flex items-center gap-1.5'>
                {renderNodeIcon(selectedNode.type)}
                <CardTitle className='text-xs font-semibold'>
                  节点 #{selectedNode.vertexId}
                </CardTitle>
              </div>
              <Button
                size='sm'
                variant='ghost'
                className='h-6 px-1.5 text-[11px] text-muted-foreground hover:text-foreground'
                onClick={() => setSelectedVertexId(null)}
              >
                取消聚焦
              </Button>
            </CardHeader>
            <CardContent className='p-3.5 space-y-3 text-xs'>
              <div className='space-y-1.5'>
                <div className='text-[10px] uppercase font-semibold text-muted-foreground'>算子名称</div>
                <div className='rounded border border-border/50 bg-muted/20 px-2.5 py-1.5 font-mono text-foreground break-all'>
                  {selectedNode.connectorType}
                </div>
              </div>
              <div className='flex items-center justify-between text-xs'>
                <span className='text-muted-foreground'>节点类别:</span>
                <Badge variant='outline' className={nodeBadgeTone(selectedNode.type)}>
                  {selectedNode.type.toUpperCase()}
                </Badge>
              </div>
              <div className='flex items-center justify-between text-xs'>
                <span className='text-muted-foreground'>层级深度 (Level):</span>
                <span className='font-mono font-medium'>{selectedNode.level}</span>
              </div>
              <div className='flex items-center justify-between text-xs'>
                <span className='text-muted-foreground'>涉及表数量:</span>
                <span className='font-mono font-medium'>{normalizeTablePaths(selectedNode.tablePaths).length}</span>
              </div>
            </CardContent>
          </Card>
        ) : (
          <Card className='border-border/60 shadow-xs'>
            <CardHeader className='pb-2.5 pt-3.5 px-4 border-b border-border/40'>
              <CardTitle className='text-xs font-semibold text-foreground/80 uppercase tracking-wider flex items-center justify-between'>
                <span>预览摘要</span>
                <Badge variant='outline' className='text-[10px] font-normal'>
                  {job.jobStatus}
                </Badge>
              </CardTitle>
            </CardHeader>
            <CardContent className='p-3.5 space-y-2 text-xs'>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground'>作业名称</span>
                <span className='font-mono truncate max-w-[170px]' title={job.jobName}>{job.jobName}</span>
              </div>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground'>覆盖表总数</span>
                <span className='font-mono font-semibold'>{stats.totalTables} 张</span>
              </div>
              <div className='mt-2 rounded-md bg-muted/30 p-2.5 text-[11px] leading-relaxed text-muted-foreground'>
                提示：点击画布中的任意算子节点，可高亮上下游连线并聚焦查看算子详情。
              </div>
            </CardContent>
          </Card>
        )}

        {/* 算子节点快速导航列表 */}
        <Card className='border-border/60 shadow-xs'>
          <CardHeader className='pb-2.5 pt-3.5 px-4 border-b border-border/40'>
            <CardTitle className='text-xs font-semibold text-foreground/80 uppercase tracking-wider flex items-center justify-between'>
              <span>算子列表</span>
              <span className='font-mono text-[10px] text-muted-foreground'>{vertices.length}</span>
            </CardTitle>
          </CardHeader>
          <CardContent className='p-2 space-y-1.5 max-h-[300px] overflow-auto'>
            {vertices.map((vertex) => {
              const isSelected = selectedVertexId === vertex.vertexId;
              return (
                <div
                  key={vertex.vertexId}
                  className={cn(
                    'flex items-center justify-between rounded-md border p-2 text-xs transition-colors cursor-pointer',
                    isSelected
                      ? 'border-primary/50 bg-primary/5 font-medium'
                      : 'border-border/40 hover:bg-muted/30',
                  )}
                  onClick={() => setSelectedVertexId(vertex.vertexId)}
                >
                  <div className='flex items-center gap-1.5 truncate pr-2'>
                    {renderNodeIcon(vertex.type)}
                    <span className='font-mono text-[11px] text-muted-foreground'>#{vertex.vertexId}</span>
                    <span className='truncate text-[11px]' title={vertex.connectorType}>{vertex.connectorType}</span>
                  </div>
                  <Badge
                    variant='outline'
                    className={cn('text-[9px] uppercase px-1 py-0', nodeBadgeTone(vertex.type))}
                  >
                    {vertex.type}
                  </Badge>
                </div>
              );
            })}
          </CardContent>
        </Card>
      </div>
      <Dialog
        open={Boolean(selectedTableDetail)}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedTableDetail(null);
          }
        }}
      >
        <DialogContent className='sm:max-w-[720px]'>
          <DialogHeader>
            <DialogTitle>
              {selectedTableDetail?.tablePath || 'Table Detail'}
            </DialogTitle>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='text-sm text-muted-foreground'>
              {selectedTableDetail?.nodeLabel}
            </div>
            {selectedTableDetail?.schema ? (
              <>
                <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
                  <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                    <div className='text-xs text-muted-foreground'>Comment</div>
                    <div className='mt-1 text-sm'>
                      {String(
                        toObject(selectedTableDetail.schema).comment || '-',
                      )}
                    </div>
                  </div>
                  <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                    <div className='text-xs text-muted-foreground'>
                      Partition Keys
                    </div>
                    <div className='mt-2 flex flex-wrap gap-2'>
                      {toStringArray(
                        toObject(selectedTableDetail.schema).partitionKeys,
                      ).length > 0 ? (
                        toStringArray(
                          toObject(selectedTableDetail.schema).partitionKeys,
                        ).map((item) => (
                          <Badge key={item} variant='secondary'>
                            {item}
                          </Badge>
                        ))
                      ) : (
                        <span className='text-sm text-muted-foreground'>-</span>
                      )}
                    </div>
                  </div>
                  <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                    <div className='text-xs text-muted-foreground'>
                      Primary Key
                    </div>
                    <div className='mt-2 flex flex-wrap gap-2'>
                      {toStringArray(
                        toObject(
                          toObject(
                            toObject(selectedTableDetail.schema).schema,
                          ).primaryKey,
                        ).columnNames,
                      ).length > 0 ? (
                        toStringArray(
                          toObject(
                            toObject(
                              toObject(selectedTableDetail.schema).schema,
                            ).primaryKey,
                          ).columnNames,
                        ).map((item) => (
                          <Badge key={item} variant='default'>
                            {item}
                          </Badge>
                        ))
                      ) : (
                        <span className='text-sm text-muted-foreground'>-</span>
                      )}
                    </div>
                  </div>
                </div>

                <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                  <div className='mb-3 text-sm font-medium'>Columns</div>
                  <ScrollArea className='max-h-[360px]'>
                    <table className='w-full text-sm'>
                      <thead className='sticky top-0 bg-background/95'>
                        <tr className='border-b'>
                          <th className='px-2 py-2 text-left'>Name</th>
                          <th className='px-2 py-2 text-left'>Type</th>
                          <th className='px-2 py-2 text-left'>Key</th>
                          <th className='px-2 py-2 text-left'>Nullable</th>
                          <th className='px-2 py-2 text-left'>Default</th>
                          <th className='px-2 py-2 text-left'>Comment</th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedSchemaColumns.map((column) => (
                          <tr
                            key={column.name}
                            className='border-b last:border-0'
                          >
                            <td className='px-2 py-2 font-medium'>
                              {column.name}
                            </td>
                            <td className='px-2 py-2'>
                              {column.dataType}
                            </td>
                            <td className='px-2 py-2'>
                              <div className='flex flex-wrap gap-1'>
                                {column.primaryKey ? (
                                  <Badge variant='default'>PK</Badge>
                                ) : null}
                                {column.uniqueKey ? (
                                  <Badge variant='outline'>UK</Badge>
                                ) : null}
                                {!column.primaryKey && !column.uniqueKey ? (
                                  <span className='text-muted-foreground'>-</span>
                                ) : null}
                              </div>
                            </td>
                            <td className='px-2 py-2'>
                              <Badge
                                variant={
                                  column.nullable === false
                                    ? 'destructive'
                                    : 'secondary'
                                }
                              >
                                {column.nullable === false
                                  ? 'No'
                                  : column.nullable === true
                                    ? 'Yes'
                                    : '-'}
                              </Badge>
                            </td>
                            <td className='px-2 py-2'>
                              {column.defaultValue}
                            </td>
                            <td className='px-2 py-2 text-muted-foreground'>
                              {column.comment}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </ScrollArea>
                </div>

                <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                  <div className='mb-2 text-sm font-medium'>Constraint Keys</div>
                  <div className='flex flex-wrap gap-2'>
                    {(
                      toObject(
                        toObject(selectedTableDetail.schema).schema,
                      ).constraintKeys as Array<Record<string, unknown>>
                    )?.length ? (
                      (
                        toObject(
                          toObject(selectedTableDetail.schema).schema,
                        ).constraintKeys as Array<Record<string, unknown>>
                      ).map((constraint, index) => (
                        <Badge
                          key={`${constraint.constraintName || index}`}
                          variant='outline'
                        >
                          {String(constraint.constraintType || 'CONSTRAINT')}
                        </Badge>
                      ))
                    ) : (
                      <span className='text-sm text-muted-foreground'>-</span>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className='rounded-lg border border-border/60 bg-background/80 p-3'>
                <div className='mb-2 text-sm font-medium'>Columns</div>
                {selectedTableDetail?.columns?.length ? (
                  <div className='flex flex-wrap gap-2'>
                    {selectedTableDetail.columns.map((column) => (
                      <Badge
                        key={column}
                        variant='secondary'
                        className='rounded-md'
                      >
                        {column}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <div className='text-sm text-muted-foreground'>
                    No schema columns available
                  </div>
                )}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={Boolean(selectedSaveModeDetail)}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedSaveModeDetail(null);
          }
        }}
      >
        <DialogContent className='sm:max-w-[760px]'>
          <DialogHeader>
            <DialogTitle>
              {selectedSaveModeDetail?.tablePath || 'DDL 预览'}
            </DialogTitle>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='text-sm text-muted-foreground'>
              {selectedSaveModeDetail?.nodeLabel}
            </div>
            {selectedSaveModeDetail?.preview ? (
              <SaveModePreviewDetail preview={selectedSaveModeDetail.preview} />
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
