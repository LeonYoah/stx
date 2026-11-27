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
  if (tablePath.toLowerCase() === 'default.default.default') {
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
): MetricTableAlignment {
  const source = endpointAlignment(vertices, 'source');
  const sink = endpointAlignment(vertices, 'sink');
  if (!source || !sink) {
    return {};
  }
  return {source, sink};
}
