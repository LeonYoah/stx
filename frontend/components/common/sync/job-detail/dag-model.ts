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

/**
 * 作业拓扑的纯数据模型：归一化引擎 / config tool 的 WebUI jobDag、合并运行时与计划 DAG、索引按表指标、
 * 叠加实时忙碌度与背压、计算 Flink 风格节点配色。这里不依赖 React，便于单测；
 * dagre 布局在 dag-layout.ts，只随拓扑 chunk 加载。
 * Pure data model for the job topology: normalize the WebUI jobDag from the engine / config tool, merge runtime and
 * plan DAGs, index per-table metrics, overlay realtime busyness and backpressure, and compute Flink-style node colors.
 * Nothing here depends on React so it stays unit-testable; the dagre layout lives in dag-layout.ts and only loads with
 * the topology chunk.
 */

import type {
  SyncJSON,
  SyncJobRealtime,
  SyncRealtimeVertex,
  SyncSinkSaveModePreviewTable,
} from '@/lib/services/sync';

export type DagVertexType = 'source' | 'transform' | 'sink' | string;

/** 画布、算子列表与抽屉共用的节点模型。/ Vertex model shared by the canvas, operator list and drawer. */
export interface DagVertex {
  vertexId: number;
  type: DagVertexType;
  /** 去掉 "pipeline-1 [...]" 外壳后的算子名。/ Operator name without the "pipeline-1 [...]" wrapper. */
  displayName: string;
  /** 去掉 Source[0]- 角色前缀后的连接器名，卡片主标题用。/ Connector name without the Source[0]- role prefix, used as the card title. */
  connector: string;
  /** 引擎或 config tool 给出的原始名称，悬停可见。/ Raw name from the engine or config tool, shown on hover. */
  rawName: string;
  /** 运行时 pipeline 序号，多 pipeline 作业用来区分。/ Runtime pipeline number to tell pipelines apart. */
  pipeline?: number;
  /** 指标键前缀，如 Source[0]。/ Metric key prefix such as Source[0]. */
  metricPrefix?: string;
  tablePaths: string[];
  tableColumns?: Record<string, string[]>;
  tableSchemas?: Record<string, SyncJSON>;
  saveModePreviews?: Record<string, SyncSinkSaveModePreviewTable>;
  saveModeWarnings?: string[];
  /** 运行时节点是否对应上了计划 DAG。/ Whether a runtime vertex matched a plan vertex. */
  planMatched?: boolean;
}

export interface DagEdge {
  id: string;
  source: number;
  target: number;
}

export interface DagGraph {
  vertices: DagVertex[];
  edges: DagEdge[];
  envOptions: Record<string, string>;
}

export interface TableStats {
  readCount?: number;
  readBytes?: number;
  readQps?: number;
  writeCount?: number;
  writeBytes?: number;
  writeQps?: number;
  committedCount?: number;
}

export interface MetricIndex {
  /** prefix → 表 → 指标。/ prefix → table → stats. */
  tables: Map<string, Map<string, TableStats>>;
  /** prefix → 汇总指标。/ prefix → aggregated stats. */
  vertices: Map<string, TableStats>;
}

export interface RealtimeOverlay {
  /** 节点最新忙碌度（0–1）。/ Latest vertex busyness (0–1). */
  busy: Map<number, number>;
  /** 节点被反压程度 = 出边背压最大值（0–1）。/ How backpressured a vertex is = max bp of its outgoing edges (0–1). */
  backpressured: Map<number, number>;
  /** 边 id → 最新背压（0–1）。/ Edge id → latest backpressure (0–1). */
  edgeBackpressure: Map<string, number>;
}

const PIPELINE_WRAPPER = /^pipeline-(\d+)\s*\[(.*)\]\s*$/i;
const METRIC_PREFIX = /^((?:Source|Sink|Transform)\[\d+\])/i;
const TABLE_METRIC_KEY = /^((?:Source|Sink|Transform)\[\d+\])\.(.+)$/i;

function toObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function toNumber(value: unknown): number | undefined {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

/**
 * 解析 "pipeline-1 [Source[0]-Postgres-CDC]" 这类运行时名称。
 * Parse runtime names such as "pipeline-1 [Source[0]-Postgres-CDC]".
 */
export function parseVertexName(raw: string): {
  displayName: string;
  connector: string;
  pipeline?: number;
  metricPrefix?: string;
} {
  const trimmed = raw.trim();
  const wrapped = PIPELINE_WRAPPER.exec(trimmed);
  const displayName = (wrapped ? wrapped[2].trim() : trimmed) || trimmed;
  const pipeline = wrapped ? Number(wrapped[1]) : undefined;
  const prefix = METRIC_PREFIX.exec(displayName);
  const connector = prefix ? displayName.slice(prefix[1].length).replace(/^[-\s]+/, '') : displayName;
  return {
    displayName,
    connector: connector || displayName,
    pipeline: Number.isFinite(pipeline) ? pipeline : undefined,
    metricPrefix: prefix ? prefix[1] : undefined,
  };
}

/**
 * 把引擎或 config tool 的 WebUI jobDag 归一成节点与边；兼容 vertexInfoMap 为对象或数组、ID 为字符串或数字，
 * 多个 pipeline 的边合并后按起止去重。
 * Normalize a WebUI jobDag from the engine or config tool into vertices and edges; accepts vertexInfoMap as an object
 * or array and IDs as strings or numbers, merging edges across pipelines and de-duplicating them by endpoints.
 */
export function normalizeJobDag(jobDag: unknown): DagGraph {
  const root = toObject(jobDag);
  const rawMap = root.vertexInfoMap;
  const rawList = Array.isArray(rawMap) ? rawMap : Object.values(toObject(rawMap));
  const vertices: DagVertex[] = [];
  const seen = new Set<number>();
  for (const item of rawList) {
    const entry = toObject(item);
    const vertexId = Number(entry.vertexId);
    if (!Number.isFinite(vertexId) || seen.has(vertexId)) {
      continue;
    }
    seen.add(vertexId);
    const rawName = String(entry.connectorType || entry.vertexName || entry.name || `#${vertexId}`);
    const parsed = parseVertexName(rawName);
    const tablePaths = Array.isArray(entry.tablePaths)
      ? Array.from(new Set(entry.tablePaths.map((path) => String(path)).filter(Boolean)))
      : [];
    vertices.push({
      vertexId,
      type: String(entry.type || 'transform').toLowerCase(),
      displayName: parsed.displayName,
      connector: parsed.connector,
      rawName,
      pipeline: parsed.pipeline,
      metricPrefix: parsed.metricPrefix,
      tablePaths,
      tableColumns: entry.tableColumns ? (toObject(entry.tableColumns) as Record<string, string[]>) : undefined,
      tableSchemas: entry.tableSchemas ? (toObject(entry.tableSchemas) as Record<string, SyncJSON>) : undefined,
      saveModePreviews: entry.saveModePreviews
        ? (toObject(entry.saveModePreviews) as Record<string, SyncSinkSaveModePreviewTable>)
        : undefined,
      saveModeWarnings: Array.isArray(entry.saveModeWarnings)
        ? entry.saveModeWarnings.map((warning) => String(warning))
        : undefined,
    });
  }
  vertices.sort((left, right) => left.vertexId - right.vertexId);

  const edges: DagEdge[] = [];
  const edgeIds = new Set<string>();
  const pipelines = Object.entries(toObject(root.pipelineEdges)).sort(
    ([left], [right]) => Number(left) - Number(right),
  );
  for (const [, list] of pipelines) {
    if (!Array.isArray(list)) {
      continue;
    }
    for (const rawEdge of list) {
      const edge = toObject(rawEdge);
      const source = Number(edge.inputVertexId);
      const target = Number(edge.targetVertexId);
      if (!seen.has(source) || !seen.has(target)) {
        continue;
      }
      const id = `${source}->${target}`;
      if (edgeIds.has(id)) {
        continue;
      }
      edgeIds.add(id);
      edges.push({id, source, target});
    }
  }

  const envOptions: Record<string, string> = {};
  for (const [key, value] of Object.entries(toObject(root.envOptions))) {
    envOptions[key] = typeof value === 'string' ? value : JSON.stringify(value);
  }
  return {vertices, edges, envOptions};
}

function sameTables(left: string[], right: string[]): boolean {
  if (left.length !== right.length || left.length === 0) {
    return false;
  }
  const set = new Set(left);
  return right.every((path) => set.has(path));
}

/**
 * 把计划 DAG 的表结构与建表计划合并进运行时 DAG。拓扑以运行时为准；先按 ID + 类型匹配，再按类型 + 表集合，
 * 最后按类型 + 指标前缀兜底（多表 Sink 在运行时会改名为 *-MultiTableSink，ID 通常仍一致）。
 * Merge schemas and save-mode plans from the plan DAG into the runtime DAG. The runtime topology wins; vertices match
 * by id + type first, then type + table set, then type + metric prefix (multi-table sinks are renamed to
 * *-MultiTableSink at runtime but usually keep their id).
 */
export function mergeRuntimeWithPlan(runtime: DagGraph, plan: DagGraph | null | undefined): DagGraph {
  if (!plan || plan.vertices.length === 0) {
    return runtime;
  }
  const used = new Set<number>();
  const pick = (predicate: (candidate: DagVertex) => boolean) =>
    plan.vertices.find((candidate) => !used.has(candidate.vertexId) && predicate(candidate));

  const vertices = runtime.vertices.map((vertex) => {
    const match =
      pick((candidate) => candidate.vertexId === vertex.vertexId && candidate.type === vertex.type) ??
      pick((candidate) => candidate.type === vertex.type && sameTables(candidate.tablePaths, vertex.tablePaths)) ??
      pick(
        (candidate) =>
          candidate.type === vertex.type &&
          Boolean(candidate.metricPrefix) &&
          candidate.metricPrefix === vertex.metricPrefix,
      );
    if (!match) {
      return {...vertex, planMatched: false};
    }
    used.add(match.vertexId);
    return {
      ...vertex,
      metricPrefix: vertex.metricPrefix ?? match.metricPrefix,
      tablePaths: vertex.tablePaths.length > 0 ? vertex.tablePaths : match.tablePaths,
      tableColumns: match.tableColumns,
      tableSchemas: match.tableSchemas,
      saveModePreviews: match.saveModePreviews,
      saveModeWarnings: match.saveModeWarnings,
      planMatched: true,
    };
  });
  return {...runtime, vertices};
}

const TABLE_METRIC_FIELDS: Array<[string, keyof TableStats]> = [
  ['TableSourceReceivedCount', 'readCount'],
  ['TableSourceReceivedBytes', 'readBytes'],
  ['TableSourceReceivedQPS', 'readQps'],
  ['TableSinkWriteCount', 'writeCount'],
  ['TableSinkWriteBytes', 'writeBytes'],
  ['TableSinkWriteQPS', 'writeQps'],
  ['TableSinkCommittedCount', 'committedCount'],
];

const JOB_METRIC_FIELDS: Array<[string, keyof TableStats]> = [
  ['SourceReceivedCount', 'readCount'],
  ['SourceReceivedBytes', 'readBytes'],
  ['SourceReceivedQPS', 'readQps'],
  ['SinkWriteCount', 'writeCount'],
  ['SinkWriteBytes', 'writeBytes'],
  ['SinkWriteQPS', 'writeQps'],
  ['SinkCommittedCount', 'committedCount'],
];

function addStat(target: TableStats, field: keyof TableStats, value: number) {
  target[field] = (target[field] ?? 0) + value;
}

/**
 * 一次扫描把 Table* 指标按 "Source[0].库.表" 索引成 prefix → 表 → 指标，并汇总到节点；多表作业不必每个节点重复扫描。
 * 没有按表指标时，若该类型只有一个节点，用作业级指标兜底。
 * Index Table* metrics keyed "Source[0].db.table" into prefix → table → stats in one pass and aggregate per vertex, so
 * multi-table jobs never rescan per vertex. Without per-table metrics, job-level totals fall back to the only vertex of
 * that type.
 */
export function buildMetricIndex(metrics: unknown, vertices: DagVertex[] = []): MetricIndex {
  const root = toObject(metrics);
  const tables = new Map<string, Map<string, TableStats>>();
  const totals = new Map<string, TableStats>();
  for (const [metricKey, field] of TABLE_METRIC_FIELDS) {
    for (const [rawKey, rawValue] of Object.entries(toObject(root[metricKey]))) {
      const match = TABLE_METRIC_KEY.exec(rawKey);
      const value = toNumber(rawValue);
      if (!match || value === undefined) {
        continue;
      }
      const prefix = match[1];
      const table = match[2];
      let byTable = tables.get(prefix);
      if (!byTable) {
        byTable = new Map();
        tables.set(prefix, byTable);
      }
      const stats = byTable.get(table) ?? {};
      stats[field] = value;
      byTable.set(table, stats);
      const total = totals.get(prefix) ?? {};
      addStat(total, field, value);
      totals.set(prefix, total);
    }
  }

  for (const type of ['source', 'sink'] as const) {
    const ofType = vertices.filter((vertex) => vertex.type === type && vertex.metricPrefix);
    if (ofType.length !== 1 || totals.has(ofType[0].metricPrefix as string)) {
      continue;
    }
    const fallback: TableStats = {};
    for (const [metricKey, field] of JOB_METRIC_FIELDS) {
      const isSourceField = metricKey.startsWith('Source');
      if (isSourceField !== (type === 'source')) {
        continue;
      }
      const value = toNumber(root[metricKey]);
      if (value !== undefined) {
        fallback[field] = value;
      }
    }
    if (Object.keys(fallback).length > 0) {
      totals.set(ofType[0].metricPrefix as string, fallback);
    }
  }
  return {tables, vertices: totals};
}

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
 * 把实时接口数据映射到拓扑：节点忙碌度、边背压，以及节点被反压程度（= 出边背压最大值，同 Flink 的 Backpressured）。
 * 引擎的队列只标了目标节点，同一目标有多条入边时无法区分，按该目标的最大值算。
 * Map realtime data onto the topology: vertex busyness, edge backpressure, and how backpressured a vertex is (= max
 * bp of its outgoing edges, like Flink's Backpressured). Engine queues only name the target vertex, so several inputs
 * into one target share that target's maximum.
 */
export function buildRealtimeOverlay(realtime: SyncJobRealtime | null | undefined, edges: DagEdge[]): RealtimeOverlay {
  const busy = new Map<number, number>();
  const backpressured = new Map<number, number>();
  const edgeBackpressure = new Map<string, number>();
  if (!realtime?.available) {
    return {busy, backpressured, edgeBackpressure};
  }
  for (const vertex of realtime.vertices) {
    const value = latestVertexBusy(vertex);
    if (typeof value === 'number' && Number.isFinite(value)) {
      busy.set(vertex.vertex_id, value);
    }
  }
  const byTarget = new Map<number, number>();
  for (const edge of realtime.edges) {
    const point = edge.points[edge.points.length - 1];
    const value = point?.bpRatio;
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      continue;
    }
    byTarget.set(edge.target_vertex_id, Math.max(byTarget.get(edge.target_vertex_id) ?? 0, value));
  }
  for (const edge of edges) {
    const value = byTarget.get(edge.target);
    if (value === undefined) {
      continue;
    }
    edgeBackpressure.set(edge.id, value);
    backpressured.set(edge.source, Math.max(backpressured.get(edge.source) ?? 0, value));
  }
  return {busy, backpressured, edgeBackpressure};
}

type RGB = [number, number, number];

function hexToRgb(hex: string): RGB {
  const value = parseInt(hex.slice(1), 16);
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

function blendRgb(from: RGB, to: RGB, ratio: number): RGB {
  const p = Math.min(1, Math.max(0, ratio));
  return [0, 1, 2].map((index) => Math.round(from[index] + (to[index] - from[index]) * p)) as RGB;
}

// 颜色取自 Flink node.component.ts：默认蓝，忙向红混合，被反压向灰混合。
// Colors from Flink's node.component.ts: default blue, blended toward red when busy and toward gray when backpressured.
const FLINK_DEFAULT = hexToRgb('#1890ff');
const FLINK_BUSY = hexToRgb('#ee2222');
const FLINK_BACKPRESSURED = hexToRgb('#888888');

/**
 * Flink 风格节点配色：先按忙碌度混红，再按被反压程度混灰。背景用随强度变化的透明度，亮暗主题都能看清文字。
 * Flink-style node colors: blend toward red by busyness, then toward gray by backpressure. The background uses an
 * intensity-based alpha so text stays readable in both light and dark themes.
 */
export function nodeColors(
  busy?: number | null,
  backpressured?: number | null,
): {border: string; background: string; intensity: number} | null {
  const hue = blendNodeHue(busy, backpressured);
  if (!hue) {
    return null;
  }
  const [r, g, b] = hue.rgb;
  const alpha = 0.03 + 0.25 * hue.intensity;
  return {border: `rgb(${r}, ${g}, ${b})`, background: `rgba(${r}, ${g}, ${b}, ${alpha.toFixed(3)})`, intensity: hue.intensity};
}

/**
 * 按忙碌度混红、按被反压程度混灰得到节点色相；两者都没有数据时返回 null。
 * Blend the node hue toward red by busyness and toward gray by backpressure; null when neither has data.
 */
function blendNodeHue(busy?: number | null, backpressured?: number | null): {rgb: RGB; intensity: number} | null {
  const hasBusy = typeof busy === 'number' && Number.isFinite(busy);
  const hasBp = typeof backpressured === 'number' && Number.isFinite(backpressured);
  if (!hasBusy && !hasBp) {
    return null;
  }
  let color = FLINK_DEFAULT;
  if (hasBusy) {
    color = blendRgb(color, FLINK_BUSY, busy as number);
  }
  if (hasBp) {
    color = blendRgb(color, FLINK_BACKPRESSURED, backpressured as number);
  }
  const intensity = Math.min(1, Math.max(0, hasBusy ? (busy as number) : 0, hasBp ? (backpressured as number) : 0));
  return {rgb: color, intensity};
}

export type DagTheme = 'light' | 'dark';

const WHITE: RGB = [255, 255, 255];
// 暗色主题 --card（zinc-900 #18181b）。/ The dark theme's --card color (zinc-900 #18181b).
const DARK_CARD: RGB = [24, 24, 27];

/**
 * 卡片底色（参照 Flink：每个节点都是同一色相的色块，状态只让这块颜色连续变化，所以颜色不会“突然出现”）。
 * 色相 = nodeColors 的混合结果（无数据时为 Flink 默认蓝）；底色始终是浅色调，100% 忙碌也只是浅玫红底配红边，文字可读。
 * 两种主题都从卡片底色向色相混合成实色：浅色从白色混，避免白卡片融进白画布；暗色从深色卡片色混，画布点阵不会透出来。
 * Card surface (after Flink: every node is a block of the same hue and state only shifts that color continuously, so
 * color never "suddenly appears"). The hue is nodeColors' blend (Flink default blue without data); the fill stays a
 * light tint, so even 100% busy is a pale rose fill with a red border and readable text. Both themes mix from the card
 * color toward the hue into a solid color: light mode from white so white cards no longer melt into the white canvas,
 * dark mode from the dark card color so the canvas dot grid does not show through.
 */
export function nodeSurface(
  busy: number | null | undefined,
  backpressured: number | null | undefined,
  theme: DagTheme,
): {border: string; background: string} {
  const hue = blendNodeHue(busy, backpressured);
  const intensity = hue?.intensity ?? 0;
  const color = hue?.rgb ?? FLINK_DEFAULT;
  const dark = theme === 'dark';
  const base = dark ? DARK_CARD : WHITE;
  // 暗色底需要更高混合比才看得出色相。/ A dark base needs a higher ratio for the hue to show.
  const fill = blendRgb(base, color, dark ? 0.12 + 0.16 * intensity : 0.08 + 0.12 * intensity);
  const border = blendRgb(base, color, 0.45 + 0.4 * intensity);
  return {background: `rgb(${fill.join(', ')})`, border: `rgb(${border.join(', ')})`};
}

/**
 * 背压分档，与 SeaTunnel Web UI 一致：0 灰、<5% 绿、<20% 黄、<50% 橙、≥50% 红。
 * Backpressure tiers matching the SeaTunnel Web UI: 0 gray, <5% green, <20% yellow, <50% orange, ≥50% red.
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

/** 与 backpressureTone 同档的连线颜色。/ Edge stroke color using the same tiers as backpressureTone. */
export function backpressureStroke(ratio: number | undefined): string | undefined {
  if (ratio === undefined || ratio <= 0) {
    return undefined;
  }
  if (ratio < 0.05) {
    return '#10b981';
  }
  if (ratio < 0.2) {
    return '#eab308';
  }
  if (ratio < 0.5) {
    return '#f97316';
  }
  return '#ef4444';
}

/**
 * 中间省略：长表名保留开头的库名与结尾的表名，比尾部截断更能辨认。
 * Middle ellipsis: keep the database prefix and the table-name suffix, which identifies tables better than tail cuts.
 */
export function middleEllipsis(text: string, maxChars: number): string {
  if (maxChars < 5 || text.length <= maxChars) {
    return text;
  }
  const head = Math.ceil((maxChars - 1) * 0.4);
  const tail = maxChars - 1 - head;
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

/**
 * 卡片上的表名：同一节点的表通常同库，去掉 "库." 前缀，把宽度留给 schema 和表名，再做中间省略。
 * Table label on a card: tables of one vertex usually share the database, so drop the "db." prefix to leave room for
 * the schema and table name, then middle-ellipsize.
 */
export function shortTablePath(path: string, maxChars: number): string {
  const parts = path.split('.');
  const short = parts.length >= 3 ? parts.slice(1).join('.') : path;
  return middleEllipsis(short, maxChars);
}

export function formatPercent(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return 'N/A';
  }
  return `${Math.round(value * 100)}%`;
}

export function formatCount(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '-';
  }
  const abs = Math.abs(value);
  if (abs >= 1e9) {
    return `${(value / 1e9).toFixed(1)}B`;
  }
  if (abs >= 1e6) {
    return `${(value / 1e6).toFixed(1)}M`;
  }
  if (abs >= 1e4) {
    return `${(value / 1e3).toFixed(1)}k`;
  }
  return abs >= 100 || Number.isInteger(value) ? String(Math.round(value)) : value.toFixed(1);
}

export function formatBytes(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '-';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = 0;
  while (Math.abs(size) >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? Math.round(size) : size.toFixed(1)} ${units[unit]}`;
}

/** 拓扑结构签名：只有它变化才重新布局。/ Topology signature; only a change here triggers a relayout. */
export function structureKey(graph: DagGraph): string {
  return `${graph.vertices.map((vertex) => `${vertex.vertexId}:${vertex.type}`).join(',')}|${graph.edges
    .map((edge) => edge.id)
    .join(',')}`;
}

/** 表列表搜索（不区分大小写）。/ Case-insensitive table list search. */
export function filterTables(paths: string[], query: string): string[] {
  const keyword = query.trim().toLowerCase();
  if (!keyword) {
    return paths;
  }
  return paths.filter((path) => path.toLowerCase().includes(keyword));
}
