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

export interface MetricTableAlignmentVertex {
  type?: string;
  metricPrefix?: string;
  tablePaths?: unknown;
}

export interface MetricTableEndpointAlignment {
  type: 'source' | 'sink';
  metricPrefix?: string;
  nodeLabel: string;
  tablePath: string;
  canonicalKey: string;
}

export interface MetricTableAlignment {
  source?: MetricTableEndpointAlignment;
  sink?: MetricTableEndpointAlignment;
}

const PREFIXED_METRIC_KEY = /^(Source|Sink)\[\d+\]\.(.+)$/i;
const OPTIONAL_WRAPPER = /^Optional\[(.+)\]$/i;
const PIPELINE_WRAPPER = /^pipeline-\d+\s*\[(.+)\]$/i;
const METRIC_PREFIX = /^((?:Source|Sink|Transform)\[\d+\])/i;

const SOURCE_TABLE_METRIC_KEYS = [
  'TableSourceReceivedCount',
  'TableSourceReceivedBytes',
  'TableSourceReceivedQPS',
] as const;

const SINK_TABLE_METRIC_KEYS = [
  'TableSinkWriteCount',
  'TableSinkWriteBytes',
  'TableSinkWriteQPS',
  'TableSinkCommittedCount',
  'TableSinkCommittedBytes',
] as const;

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

// 拒绝 default 占位符与动态模板/正则通配表路径，避免把动态多表作业误判为单表。
// Reject default placeholders and template/wildcard paths so dynamic multi-table jobs are never treated as single-table.
function isConcreteSingleTablePath(tablePath: string): boolean {
  const trimmed = tablePath.trim();
  if (!trimmed || trimmed.toLowerCase() === 'default.default.default') {
    return false;
  }
  if (trimmed.includes('${') || /[*?[\]|]/.test(trimmed)) {
    return false;
  }
  return true;
}

function normalizeUnwrappedLeaf(rawKey: string): string {
  const trimmed = rawKey.trim();
  if (!trimmed) {
    return '';
  }
  const prefixed = PREFIXED_METRIC_KEY.exec(trimmed);
  let path = (prefixed?.[2] ?? trimmed).trim();
  const optional = OPTIONAL_WRAPPER.exec(path);
  if (optional?.[1]) {
    path = optional[1].trim();
  }
  if (!path || path.toLowerCase() === 'default.default.default') {
    return '';
  }
  const leaf = path.split('.').pop()?.trim().toLowerCase() || path.toLowerCase();
  return leaf.replace(/^archive_/, '');
}

function sideHasMultipleTables(
  metricsRoot: Record<string, unknown>,
  metricKeys: readonly string[],
): boolean {
  const prefixedPaths = new Set<string>();
  const leafNames = new Set<string>();
  for (const metricKey of metricKeys) {
    for (const rawKey of Object.keys(asRecord(metricsRoot[metricKey]))) {
      const trimmed = rawKey.trim();
      const prefixed = PREFIXED_METRIC_KEY.exec(trimmed);
      if (prefixed?.[2]) {
        const path = prefixed[2].trim().toLowerCase();
        if (path && path !== 'default.default.default') {
          prefixedPaths.add(path);
          if (prefixedPaths.size > 1) {
            return true;
          }
        }
      }
      const leaf = normalizeUnwrappedLeaf(trimmed);
      if (leaf) {
        leafNames.add(leaf);
        if (leafNames.size > 1) {
          return true;
        }
      }
    }
  }
  return false;
}

// 运行时多表熔断：即使静态 job_dag 只列了 1 个表路径，只要指标任一侧已上报多张不同真实表，就必须放弃单链路对齐。
// Runtime multi-table fuse: even if static job_dag lists 1 tablePath, abort single-link alignment when metrics already report multiple real tables on either side.
export function metricsHasMultipleTables(metrics: unknown): boolean {
  const root = asRecord(metrics);
  return (
    sideHasMultipleTables(root, SOURCE_TABLE_METRIC_KEYS) ||
    sideHasMultipleTables(root, SINK_TABLE_METRIC_KEYS)
  );
}

// 轻量读取 job_dag.vertexInfoMap 供非拓扑模块复用，避免与 dag-model.ts 循环依赖。
// Lightweight vertex extractor from job_dag.vertexInfoMap for non-topology callers without importing dag-model.ts.
export function extractAlignmentVerticesFromJobDag(
  jobDag: unknown,
): MetricTableAlignmentVertex[] {
  const root = asRecord(jobDag);
  const rawMap = root.vertexInfoMap;
  const rawList = Array.isArray(rawMap) ? rawMap : Object.values(asRecord(rawMap));
  const vertices: MetricTableAlignmentVertex[] = [];
  for (const item of rawList) {
    const entry = asRecord(item);
    const rawName = String(entry.connectorType || entry.vertexName || entry.name || '').trim();
    const wrapped = PIPELINE_WRAPPER.exec(rawName);
    const displayName = (wrapped ? wrapped[1].trim() : rawName) || rawName;
    const prefixMatch = METRIC_PREFIX.exec(displayName);
    vertices.push({
      type: String(entry.type || 'transform').toLowerCase(),
      metricPrefix: prefixMatch ? prefixMatch[1] : undefined,
      tablePaths: entry.tablePaths,
    });
  }
  return vertices;
}

function endpointNodeLabel(
  type: 'source' | 'sink',
  metricPrefix?: string,
): string {
  const name = type === 'source' ? 'Source' : 'Sink';
  const index = metricPrefix?.match(/\[(\d+)\]/)?.[1];
  return index === undefined ? name : `${name} #${Number(index) + 1}`;
}

function endpointAlignment(
  vertices: MetricTableAlignmentVertex[],
  type: 'source' | 'sink',
): MetricTableEndpointAlignment | undefined {
  const matches = vertices.filter(
    (vertex) => String(vertex.type || '').toLowerCase() === type,
  );
  if (matches.length !== 1) {
    return undefined;
  }
  const vertex = matches[0];
  const tablePaths = Array.isArray(vertex.tablePaths)
    ? Array.from(
        new Set(
          vertex.tablePaths.map((path) => String(path).trim()).filter(Boolean),
        ),
      )
    : [];
  if (tablePaths.length !== 1) {
    return undefined;
  }
  const metricPrefix = vertex.metricPrefix?.trim() || undefined;
  const tablePath = tablePaths[0];
  if (!isConcreteSingleTablePath(tablePath)) {
    return undefined;
  }
  return {
    type,
    metricPrefix,
    nodeLabel: endpointNodeLabel(type, metricPrefix),
    tablePath,
    canonicalKey: metricPrefix ? `${metricPrefix}.${tablePath}` : tablePath,
  };
}

export function buildMetricTableAlignment(
  vertices: MetricTableAlignmentVertex[] = [],
  metrics?: unknown,
): MetricTableAlignment {
  if (metrics !== undefined && metricsHasMultipleTables(metrics)) {
    return {};
  }
  const source = endpointAlignment(vertices, 'source');
  const sink = endpointAlignment(vertices, 'sink');
  if (!source || !sink) {
    return {};
  }
  return {source, sink};
}
