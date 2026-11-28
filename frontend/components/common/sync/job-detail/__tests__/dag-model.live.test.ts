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

import {readFileSync} from 'node:fs';
import {describe, expect, it} from 'vitest';
import {buildMetricIndex, normalizeJobDag} from '../dag-model';

const fixturePath = process.env.STX_SEATUNNEL_METRICS_FIXTURE;

describe.skipIf(!fixturePath)('buildMetricIndex with a live SeaTunnel response', () => {
  it('keeps nonzero single-vertex source and sink values from real engine metrics', () => {
    const payload = JSON.parse(readFileSync(fixturePath as string, 'utf8')) as {
      jobDag?: unknown;
      metrics?: Record<string, unknown>;
    };
    const graph = normalizeJobDag(payload.jobDag);
    const index = buildMetricIndex(payload.metrics, graph.vertices);
    const source = graph.vertices.find((vertex) => vertex.type === 'source');
    const sink = graph.vertices.find((vertex) => vertex.type === 'sink');

    expect(source?.metricPrefix).toBeTruthy();
    expect(sink?.metricPrefix).toBeTruthy();
    expect(Number(payload.metrics?.SourceReceivedCount)).toBeGreaterThanOrEqual(300);
    expect(Number(payload.metrics?.SinkWriteCount)).toBeGreaterThanOrEqual(300);
    expect(index.vertices.get(source?.metricPrefix as string)?.readCount).toBeGreaterThanOrEqual(300);
    expect(index.vertices.get(sink?.metricPrefix as string)?.writeCount).toBeGreaterThanOrEqual(300);
    if (source?.tablePaths.length === 1) {
      expect(index.tables.get(source.metricPrefix as string)?.get(source.tablePaths[0])?.readCount).toBeGreaterThanOrEqual(300);
    }
    if (sink?.tablePaths.length === 1) {
      expect(index.tables.get(sink.metricPrefix as string)?.get(sink.tablePaths[0])?.writeCount).toBeGreaterThanOrEqual(300);
    }
  });
});
