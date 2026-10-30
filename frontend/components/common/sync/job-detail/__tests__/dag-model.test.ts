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

// 作业拓扑数据模型的单元测试；夹具取自线上作业 #26 的真实结构，并补多算子 / 多表场景。
// Unit tests for the job topology model; fixtures mirror production job #26 plus multi-operator / multi-table cases.

import {describe, expect, it} from 'vitest';
import type {SyncJobRealtime} from '@/lib/services/sync';
import {
  buildMetricIndex,
  buildRealtimeOverlay,
  filterTables,
  formatBytes,
  formatCount,
  mergeRuntimeWithPlan,
  middleEllipsis,
  nodeColors,
  nodeSurface,
  normalizeJobDag,
  parseVertexName,
  shortTablePath,
  structureKey,
} from '../dag-model';
import {layoutDag} from '../dag-layout';

// 线上作业 #26 的运行时 job_dag：vertexInfoMap 是数组，边的 ID 是字符串。
// Runtime job_dag of production job #26: vertexInfoMap is an array and edge ids are strings.
const runtimeJob26 = {
  envOptions: {'job.mode': 'STREAMING', parallelism: '1', 'checkpoint.interval': '5000'},
  jobId: '179312051430100000',
  pipelineEdges: {'1': [{inputVertexId: '1', targetVertexId: '2'}]},
  vertexInfoMap: [
    {tablePaths: ['seatunnel_test.public.cdc_orders'], type: 'source', vertexId: 1, vertexName: 'pipeline-1 [Source[0]-Postgres-CDC]'},
    {tablePaths: ['seatunnel_test.public.cdc_orders'], type: 'sink', vertexId: 2, vertexName: 'pipeline-1 [Sink[0]-Console-MultiTableSink]'},
  ],
};

// 同一作业由 config tool 推导的计划 DAG：vertexInfoMap 是对象，pipeline 键不同（"0"）。
// The plan DAG of the same job from the config tool: vertexInfoMap is an object and the pipeline key differs ("0").
const planJob26 = {
  pipelineEdges: {'0': [{inputVertexId: 1, targetVertexId: 2}]},
  vertexInfoMap: {
    '1': {
      vertexId: 1,
      type: 'source',
      connectorType: 'Source[0]-Postgres-CDC',
      tablePaths: ['seatunnel_test.public.cdc_orders'],
      tableColumns: {'seatunnel_test.public.cdc_orders': ['id', 'customer', 'amount', 'updated_at']},
    },
    '2': {
      vertexId: 2,
      type: 'sink',
      connectorType: 'Sink[0]-Console',
      tablePaths: ['seatunnel_test.public.cdc_orders'],
      saveModePreviews: {'seatunnel_test.public.cdc_orders': {supported: false}},
    },
  },
};

const metricsJob26 = {
  SourceReceivedCount: '300011',
  SinkWriteCount: '300011',
  TableSourceReceivedCount: {'Source[0].seatunnel_test.public.cdc_orders': '300011'},
  TableSourceReceivedBytes: {'Source[0].seatunnel_test.public.cdc_orders': '16392561'},
  TableSourceReceivedQPS: {'Source[0].seatunnel_test.public.cdc_orders': '3983.0990029341087'},
  TableSinkWriteCount: {'Sink[0].seatunnel_test.public.cdc_orders': '300011'},
  TableSinkWriteQPS: {'Sink[0].seatunnel_test.public.cdc_orders': '3980.879211284052'},
};

describe('parseVertexName', () => {
  it('strips the pipeline wrapper and finds the metric prefix', () => {
    expect(parseVertexName('pipeline-1 [Source[0]-Postgres-CDC]')).toEqual({
      displayName: 'Source[0]-Postgres-CDC',
      connector: 'Postgres-CDC',
      pipeline: 1,
      metricPrefix: 'Source[0]',
    });
    expect(parseVertexName('Transform[2]-Sql')).toMatchObject({connector: 'Sql', metricPrefix: 'Transform[2]'});
    expect(parseVertexName('Sink[0]-Console-MultiTableSink')).toMatchObject({connector: 'Console-MultiTableSink'});
    expect(parseVertexName('Custom')).toMatchObject({displayName: 'Custom', connector: 'Custom', metricPrefix: undefined});
  });
});

describe('normalizeJobDag', () => {
  it('accepts array vertex maps and string edge ids', () => {
    const graph = normalizeJobDag(runtimeJob26);
    expect(graph.vertices.map((vertex) => [vertex.vertexId, vertex.type, vertex.metricPrefix])).toEqual([
      [1, 'source', 'Source[0]'],
      [2, 'sink', 'Sink[0]'],
    ]);
    expect(graph.edges).toEqual([{id: '1->2', source: 1, target: 2}]);
    expect(graph.envOptions.parallelism).toBe('1');
  });

  it('merges edges across pipelines and drops duplicates and dangling edges', () => {
    const graph = normalizeJobDag({
      pipelineEdges: {
        '2': [{inputVertexId: 3, targetVertexId: 4}],
        '1': [
          {inputVertexId: 1, targetVertexId: 2},
          {inputVertexId: 1, targetVertexId: 2},
          {inputVertexId: 1, targetVertexId: 99},
        ],
      },
      vertexInfoMap: {
        '1': {vertexId: 1, type: 'source', vertexName: 'pipeline-1 [Source[0]-MySQL-CDC]'},
        '2': {vertexId: 2, type: 'sink', vertexName: 'pipeline-1 [Sink[0]-Doris]'},
        '3': {vertexId: 3, type: 'source', vertexName: 'pipeline-2 [Source[1]-Kafka]'},
        '4': {vertexId: 4, type: 'sink', vertexName: 'pipeline-2 [Sink[1]-Hive]'},
      },
    });
    expect(graph.edges.map((edge) => edge.id)).toEqual(['1->2', '3->4']);
    expect(graph.vertices.map((vertex) => vertex.pipeline)).toEqual([1, 1, 2, 2]);
  });

  it('never throws on empty or malformed input', () => {
    expect(normalizeJobDag(undefined)).toEqual({vertices: [], edges: [], envOptions: {}});
    expect(normalizeJobDag({vertexInfoMap: null, pipelineEdges: {'1': null}}).vertices).toEqual([]);
  });
});

describe('mergeRuntimeWithPlan', () => {
  it('enriches runtime vertices with plan schemas by id even when names differ', () => {
    const merged = mergeRuntimeWithPlan(normalizeJobDag(runtimeJob26), normalizeJobDag(planJob26));
    const [source, sink] = merged.vertices;
    expect(source.planMatched).toBe(true);
    expect(source.tableColumns?.['seatunnel_test.public.cdc_orders']).toHaveLength(4);
    expect(sink.displayName).toBe('Sink[0]-Console-MultiTableSink');
    expect(sink.saveModePreviews).toBeDefined();
  });

  it('falls back to type + table set when ids do not line up', () => {
    const runtime = normalizeJobDag({
      vertexInfoMap: [{vertexId: 7, type: 'sink', vertexName: 'pipeline-1 [Sink[0]-Jdbc]', tablePaths: ['a.t1', 'a.t2']}],
    });
    const plan = normalizeJobDag({
      vertexInfoMap: {'2': {vertexId: 2, type: 'sink', connectorType: 'Sink[0]-Jdbc', tablePaths: ['a.t2', 'a.t1'], tableColumns: {'a.t1': ['id']}}},
    });
    expect(mergeRuntimeWithPlan(runtime, plan).vertices[0]).toMatchObject({planMatched: true, tableColumns: {'a.t1': ['id']}});
  });

  it('marks unmatched vertices and keeps the runtime topology', () => {
    const runtime = normalizeJobDag(runtimeJob26);
    const merged = mergeRuntimeWithPlan(runtime, normalizeJobDag({vertexInfoMap: {'9': {vertexId: 9, type: 'transform'}}}));
    expect(merged.vertices.every((vertex) => vertex.planMatched === false)).toBe(true);
    expect(merged.edges).toBe(runtime.edges);
    expect(mergeRuntimeWithPlan(runtime, null)).toBe(runtime);
  });
});

describe('buildMetricIndex', () => {
  it('indexes per-table metrics by vertex prefix and aggregates per vertex', () => {
    const graph = normalizeJobDag(runtimeJob26);
    const index = buildMetricIndex(metricsJob26, graph.vertices);
    expect(index.tables.get('Source[0]')?.get('seatunnel_test.public.cdc_orders')).toMatchObject({
      readCount: 300011,
      readBytes: 16392561,
    });
    expect(index.vertices.get('Sink[0]')?.writeCount).toBe(300011);
  });

  it('aggregates many tables of one multi-table sink', () => {
    const tables = Array.from({length: 500}, (_, i) => `db.public.t${i}`);
    const counts = Object.fromEntries(tables.map((table) => [`Sink[0].${table}`, '2']));
    const index = buildMetricIndex({TableSinkWriteCount: counts});
    expect(index.tables.get('Sink[0]')?.size).toBe(500);
    expect(index.vertices.get('Sink[0]')?.writeCount).toBe(1000);
  });

  it('falls back to job-level totals for the only vertex of a type', () => {
    const graph = normalizeJobDag(runtimeJob26);
    const index = buildMetricIndex({SourceReceivedCount: '12', SinkWriteCount: '10'}, graph.vertices);
    expect(index.vertices.get('Source[0]')?.readCount).toBe(12);
    expect(index.vertices.get('Sink[0]')?.writeCount).toBe(10);
  });
});

describe('buildRealtimeOverlay', () => {
  const realtime = {
    available: true,
    vertices: [
      {vertex_id: 1, type: 'source', name: 'src', points: [{ts: 1, sourceReadRatio: 0.3}]},
      {vertex_id: 2, type: 'sink', name: 'sink', points: [{ts: 1, sinkBusyRatio: 0.95}]},
    ],
    edges: [
      {queue_id: -5, target_vertex_id: 2, points: [{ts: 1, bpRatio: 0.2}, {ts: 2, bpRatio: 0.7}]},
    ],
  } as unknown as SyncJobRealtime;

  it('maps latest busyness to vertices and backpressure to edges and their sources', () => {
    const graph = normalizeJobDag(runtimeJob26);
    const overlay = buildRealtimeOverlay(realtime, graph.edges);
    expect(overlay.busy.get(2)).toBe(0.95);
    expect(overlay.edgeBackpressure.get('1->2')).toBe(0.7);
    expect(overlay.backpressured.get(1)).toBe(0.7);
    expect(overlay.backpressured.has(2)).toBe(false);
  });

  it('is empty when realtime is unavailable', () => {
    const overlay = buildRealtimeOverlay({...realtime, available: false}, []);
    expect(overlay.busy.size + overlay.edgeBackpressure.size + overlay.backpressured.size).toBe(0);
  });
});

describe('nodeColors', () => {
  it('follows Flink: neutral without data, redder when busy, grayer when backpressured', () => {
    expect(nodeColors(undefined, undefined)).toBeNull();
    expect(nodeColors(0, 0)?.border).toBe('rgb(24, 144, 255)');
    expect(nodeColors(1, undefined)?.border).toBe('rgb(238, 34, 34)');
    expect(nodeColors(0.05, 0.3)?.intensity).toBe(0.3);
    expect(nodeColors(0, 1)?.border).toBe('rgb(136, 136, 136)');
  });
});

describe('nodeSurface', () => {
  const channels = (color: string) => (color.match(/[\d.]+/g) ?? []).map(Number);

  // 同 Flink：没有数据的节点也有统一底色，浅色模式是比白色深的实色，卡片不会融进白画布。
  // Like Flink: nodes without data still share one fill; in light mode it is a solid color darker than white so cards
  // do not melt into the white canvas.
  it('gives every node the same tinted base, solid in light mode', () => {
    const idle = nodeSurface(undefined, undefined, 'light');
    expect(idle).toEqual(nodeSurface(null, null, 'light'));
    const [r, , b] = channels(idle.background);
    expect(r).toBeLessThan(255);
    expect(b).toBeGreaterThan(r);
    expect(idle.background.startsWith('rgb(')).toBe(true);
  });

  it('shifts continuously toward red when busy, without a threshold', () => {
    const calm = channels(nodeSurface(0.1, 0, 'light').border);
    const busy = channels(nodeSurface(1, 0, 'light').border);
    expect(busy[0]).toBeGreaterThan(calm[0]);
    expect(busy[2]).toBeLessThan(calm[2]);
  });

  // 暗色模式也是实色（画布点阵不透出），底色比深色卡片亮、边框比底色更亮。
  // Dark mode is solid too (the canvas dot grid does not show through): the fill is lighter than the dark card and the
  // border lighter than the fill.
  it('uses solid tints over the dark card color in dark mode', () => {
    const dark = nodeSurface(0.5, 0.2, 'dark');
    expect(dark.background.startsWith('rgb(')).toBe(true);
    const fill = channels(dark.background);
    expect(fill).toHaveLength(3);
    expect(Math.max(...fill)).toBeGreaterThan(27);
    expect(Math.max(...channels(dark.border))).toBeGreaterThan(Math.max(...fill));
  });
});

describe('layoutDag', () => {
  it('lays a fan-out / fan-in graph left to right without overlapping nodes', () => {
    const graph = normalizeJobDag({
      pipelineEdges: {
        '1': [
          {inputVertexId: 1, targetVertexId: 3},
          {inputVertexId: 2, targetVertexId: 3},
          {inputVertexId: 3, targetVertexId: 4},
          {inputVertexId: 3, targetVertexId: 5},
        ],
      },
      vertexInfoMap: [1, 2, 3, 4, 5].map((id) => ({
        vertexId: id,
        type: id <= 2 ? 'source' : id === 3 ? 'transform' : 'sink',
        vertexName: `pipeline-1 [${id <= 2 ? 'Source' : id === 3 ? 'Transform' : 'Sink'}[${id}]-X]`,
      })),
    });
    const size = {width: 260, height: 120};
    const positions = layoutDag(graph, size);
    expect(positions.size).toBe(5);
    expect(positions.get(1)!.x).toBeLessThan(positions.get(3)!.x);
    expect(positions.get(3)!.x).toBeLessThan(positions.get(4)!.x);
    const boxes = [...positions.values()];
    for (let i = 0; i < boxes.length; i += 1) {
      for (let j = i + 1; j < boxes.length; j += 1) {
        const overlapX = Math.abs(boxes[i].x - boxes[j].x) < size.width;
        const overlapY = Math.abs(boxes[i].y - boxes[j].y) < size.height;
        expect(overlapX && overlapY).toBe(false);
      }
    }
  });

  it('keeps the structure key stable across metric-only changes', () => {
    const graph = normalizeJobDag(runtimeJob26);
    const enriched = mergeRuntimeWithPlan(graph, normalizeJobDag(planJob26));
    expect(structureKey(enriched)).toBe(structureKey(graph));
  });
});

describe('formatting helpers', () => {
  it('middle-ellipsizes long table names keeping both ends', () => {
    const name = 'warehouse_ods.public.customer_order_line_items_history_2026';
    const short = middleEllipsis(name, 30);
    expect(short).toHaveLength(30);
    expect(short.startsWith('warehouse')).toBe(true);
    expect(short.endsWith('history_2026')).toBe(true);
    expect(middleEllipsis('db.t', 30)).toBe('db.t');
    // 卡片去掉库名，长表名仍保留结尾。/ Cards drop the database and keep the end of long table names.
    expect(shortTablePath('seatunnel_test.public.mt_customers', 24)).toBe('public.mt_customers');
    expect(shortTablePath('seatunnel_test.public.warehouse_order_line_items_history_2026', 24).endsWith('history_2026')).toBe(true);
    expect(shortTablePath('fake', 24)).toBe('fake');
  });

  it('formats counts and bytes compactly', () => {
    expect(formatCount(300011)).toBe('300.0k');
    expect(formatCount(42)).toBe('42');
    expect(formatCount(undefined)).toBe('-');
    expect(formatBytes(16392561)).toBe('15.6 MB');
  });

  it('filters tables case-insensitively', () => {
    expect(filterTables(['DB.Orders', 'db.users'], 'order')).toEqual(['DB.Orders']);
    expect(filterTables(['a'], '  ')).toEqual(['a']);
  });
});
