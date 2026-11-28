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

import {describe, expect, it} from 'vitest';
import {
  buildMetricFlowBar,
  buildMetricFlowSankey,
  buildPairedMetricRows,
  buildPerTableMetricRows,
  extractJobMetricSummary,
  summarizeMetricFlow,
} from '../sync-studio-utils';
import type {SyncJobInstance} from '@/lib/services/sync';
import {buildMetricTableAlignment} from '../metric-table-alignment';

const flowMetrics = (over: Record<string, unknown> = {}) => ({
  SourceReceivedCount: 1000,
  SinkWriteCount: 1000,
  SinkCommittedCount: 1000,
  SourceReceivedBytesPerSeconds: 1200,
  SinkWriteBytesPerSeconds: 1200,
  ...over,
});

describe('summarizeMetricFlow', () => {
  it('reports a neutral balanced summary when counts and commits match', () => {
    const flow = summarizeMetricFlow(flowMetrics());
    expect(flow.countState).toBe('balanced');
    expect(flow.tone).toBe('neutral');
    expect(flow.pendingCommitCount).toBe(0);
    expect(flow.backlogCount).toBe(0);
    expect(flow.segments.map((item) => item.key)).toEqual([
      'metricFlowBalanced',
      'metricFlowAllCommitted',
    ]);
  });

  it('flags written-behind-read as warning with the completion percent', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({
        SourceReceivedCount: 5000,
        SinkWriteCount: 3100,
        SinkCommittedCount: 2800,
      }),
    );
    expect(flow.countState).toBe('behind');
    expect(flow.tone).toBe('warning');
    expect(flow.backlogCount).toBe(1900);
    expect(flow.pendingCommitCount).toBe(300);
    expect(flow.segments[0]).toEqual({
      key: 'metricFlowBehind',
      values: {percent: 62},
    });
    expect(
      flow.segments.map((item) => item.key),
    ).toContain('metricFlowPendingCommitCount');
  });

  it('flags pending commit alone as warning', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({SinkCommittedCount: 900}),
    );
    expect(flow.countState).toBe('balanced');
    expect(flow.pendingCommitCount).toBe(100);
    expect(flow.tone).toBe('warning');
    expect(flow.segments.map((item) => item.key)).toEqual([
      'metricFlowBalanced',
      'metricFlowPendingCommitCount',
    ]);
  });

  it('treats missing commit metrics as fully pending commit', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({SinkCommittedCount: undefined}),
    );
    expect(flow.hasCommitMetric).toBe(false);
    expect(flow.committedCount).toBe(0);
    expect(flow.pendingCommitCount).toBe(1000);
    expect(flow.tone).toBe('warning');
  });

  it('flags read-but-nothing-written as writeZero warning', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({SinkWriteCount: 0, SinkCommittedCount: 0}),
    );
    expect(flow.countState).toBe('writeZero');
    expect(flow.tone).toBe('warning');
    expect(flow.segments.map((item) => item.key)).toEqual([
      'metricFlowWriteZero',
    ]);
  });

  it('keeps write-ahead neutral and reports the extra rows', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({SinkWriteCount: 1400, SinkCommittedCount: 1400}),
    );
    expect(flow.countState).toBe('ahead');
    expect(flow.tone).toBe('neutral');
    expect(flow.overWriteCount).toBe(400);
    expect(flow.segments[0].key).toBe('pipelineExtraWrite');
  });

  it('flags a lagging write rate as warning even when counts balanced', () => {
    const flow = summarizeMetricFlow(
      flowMetrics({SinkWriteBytesPerSeconds: 600}),
    );
    expect(flow.speedState).toBe('lag');
    expect(flow.tone).toBe('warning');
    expect(flow.segments.map((item) => item.key)).toContain(
      'pipelineSpeedLag',
    );
  });

  it('marks failed jobs as danger regardless of counts', () => {
    const flow = summarizeMetricFlow(flowMetrics(), {failed: true});
    expect(flow.tone).toBe('danger');
  });

  it('returns the empty state for zeroed metrics', () => {
    const flow = summarizeMetricFlow({
      SourceReceivedCount: 0,
      SinkWriteCount: 0,
    });
    expect(flow.countState).toBe('empty');
    expect(flow.tone).toBe('neutral');
    expect(flow.segments.map((item) => item.key)).toEqual(['metricFlowEmpty']);
  });
});

describe('buildMetricFlowBar', () => {
  it('splits the bar into committed / pending / backlog over the read scale', () => {
    const bar = buildMetricFlowBar(1000, 700, 600);
    expect(bar.total).toBe(1000);
    const [committed, pending, backlog] = bar.segments;
    expect(committed).toEqual({key: 'committed', value: 600, share: 0.6});
    expect(pending).toEqual({key: 'pendingCommit', value: 100, share: 0.1});
    expect(backlog).toEqual({key: 'backlog', value: 300, share: 0.3});
    expect(bar.overWriteCount).toBe(0);
  });

  it('extends the scale to write when write overshoots read', () => {
    const bar = buildMetricFlowBar(500, 1000, 800);
    expect(bar.total).toBe(1000);
    expect(bar.overWriteCount).toBe(500);
    const sum = bar.segments.reduce((acc, item) => acc + item.share, 0);
    expect(sum).toBeCloseTo(1);
    expect(bar.segments[0].value).toBe(800);
    expect(bar.segments[1].value).toBe(200);
    expect(bar.segments[2].value).toBe(0);
  });

  it('handles zero reads without division errors', () => {
    const bar = buildMetricFlowBar(0, 0, 0);
    expect(bar.total).toBe(0);
    expect(bar.segments.every((item) => item.share === 0)).toBe(true);
    const over = buildMetricFlowBar(0, 500, 100);
    expect(over.total).toBe(500);
    expect(over.overWriteCount).toBe(500);
    expect(over.segments[2].value).toBe(0);
  });

  it('never lets pending commit exceed the written rows', () => {
    const bar = buildMetricFlowBar(100, 100, 250);
    expect(bar.segments[0].value).toBe(100);
    expect(bar.segments[1].value).toBe(0);
  });
});

const sankeyOpts = {
  labels: {
    committed: '已提交',
    pendingCommit: '待提交',
    backlog: '待写',
    otherSources: (count: number) => `其它 ${count} 张表`,
    otherSink: '其它',
  },
  colors: {table: 'table-color', outcome: 'outcome-color', warning: 'warn-color'},
};

const tableMetrics = (
  entries: Array<{read: number; write: number; committed?: number}>,
) => {
  const source: Record<string, number> = {};
  const sink: Record<string, number> = {};
  const committed: Record<string, number> = {};
  entries.forEach((entry, index) => {
    const table = `db.t${index}`;
    source[`Source[${index}].${table}`] = entry.read;
    sink[`Sink[${index}].${table}`] = entry.write;
    if (entry.committed !== undefined) {
      committed[`Sink[${index}].${table}`] = entry.committed;
    }
  });
  const metrics: Record<string, unknown> = {
    TableSourceReceivedCount: source,
    TableSinkWriteCount: sink,
  };
  if (Object.keys(committed).length > 0) {
    metrics.TableSinkCommittedCount = committed;
  }
  return metrics;
};


const aliasedMetrics = {
  TableSourceReceivedCount: {
    'Source[0].wcan-common-tbox1-r1p24': '0',
    'Optional[wcan-common-tbox1-r1p24]': '233132',
  },
  TableSinkWriteCount: {
    'Sink[0].ah1110.ods_wcan_common_tbox1_json': '0',
    'default.default.default': '233132',
  },
  TableSinkCommittedCount: {
    'Sink[0].ah1110.ods_wcan_common_tbox1_json': '0',
    'default.default.default': '232832',
  },
};

const singleLinkVertices = [
  {type: 'source', metricPrefix: 'Source[0]', tablePaths: ['wcan-common-tbox1-r1p24']},
  {type: 'transform', metricPrefix: 'Transform[0]', tablePaths: []},
  {type: 'sink', metricPrefix: 'Sink[0]', tablePaths: ['ah1110.ods_wcan_common_tbox1_json']},
];

describe('metric table alignment', () => {
  it('merges alias and fallback keys into the only source and sink tables while preserving raw keys', () => {
    const alignment = buildMetricTableAlignment(singleLinkVertices);
    const rows = buildPerTableMetricRows(aliasedMetrics, alignment);
    expect(rows).toHaveLength(2);
    const source = rows.find((row) => row.rowTone === 'source');
    const sink = rows.find((row) => row.rowTone === 'sink');
    expect(source).toMatchObject({
      nodeLabel: 'Source #1',
      tablePath: 'wcan-common-tbox1-r1p24',
      sourceCount: '233,132',
      sinkCount: '-',
    });
    expect(sink).toMatchObject({
      nodeLabel: 'Sink #1',
      tablePath: 'ah1110.ods_wcan_common_tbox1_json',
      sourceCount: '-',
      sinkCount: '233,132',
      committedCount: '232,832',
    });
    expect(source?.rawTables).toEqual([
      'Optional[wcan-common-tbox1-r1p24]',
      'Source[0].wcan-common-tbox1-r1p24',
    ]);
    expect(sink?.rawTables).toEqual([
      'Sink[0].ah1110.ods_wcan_common_tbox1_json',
      'default.default.default',
    ]);
  });

  it('pairs different source and sink table names when the single link is unambiguous', () => {
    const alignment = buildMetricTableAlignment(singleLinkVertices);
    const paired = buildPairedMetricRows(aliasedMetrics, alignment);
    expect(paired).toHaveLength(1);
    expect(paired[0]).toMatchObject({
      sourceNode: 'Source #1',
      sourceTable: 'wcan-common-tbox1-r1p24',
      sinkNode: 'Sink #1',
      sinkTable: 'ah1110.ods_wcan_common_tbox1_json',
      sourceCount: '233,132',
      sinkCount: '233,132',
      committedCount: '232,832',
    });
  });

  it('falls back to raw keys when the topology does not identify a single link', () => {
    const rows = buildPerTableMetricRows(aliasedMetrics);
    expect(rows).toHaveLength(4);
    expect(rows.find((row) => row.rawTable === 'Optional[wcan-common-tbox1-r1p24]')?.rowTone).toBe('source');
    expect(rows.find((row) => row.rawTable === 'default.default.default')?.rowTone).toBe('sink');
    const paired = buildPairedMetricRows(aliasedMetrics);
    expect(paired).toHaveLength(0);
  });

  it('does not align when either endpoint has several logical tables', () => {
    const alignment = buildMetricTableAlignment([
      {type: 'source', metricPrefix: 'Source[0]', tablePaths: ['topic-a', 'topic-b']},
      {type: 'sink', metricPrefix: 'Sink[0]', tablePaths: ['db.t']},
    ]);
    expect(alignment.source).toBeUndefined();
    expect(alignment.sink).toBeUndefined();
    const rows = buildPerTableMetricRows(
      {TableSourceReceivedCount: {'Optional[topic-a]': '100'}},
      alignment,
    );
    expect(rows[0].rawTable).toBe('Optional[topic-a]');
  });

  it('disables single-link alignment when metrics report dynamic multi-table keys or template DAG paths', () => {
    const dynamicMetrics = {
      TableSourceReceivedCount: {
        'Source[0].db.orders_1': '100',
        'Source[0].db.orders_2': '200',
      },
      TableSinkWriteCount: {
        'Sink[0].ods.orders_1': '100',
        'Sink[0].ods.orders_2': '200',
      },
    };
    const alignment = buildMetricTableAlignment(
      [
        {type: 'source', metricPrefix: 'Source[0]', tablePaths: ['db.orders_1']},
        {type: 'sink', metricPrefix: 'Sink[0]', tablePaths: ['ods.orders_1']},
      ],
      dynamicMetrics,
    );
    expect(alignment.source).toBeUndefined();
    expect(alignment.sink).toBeUndefined();
    const paired = buildPairedMetricRows(dynamicMetrics, alignment);
    expect(paired).toHaveLength(2);

    const templateAlignment = buildMetricTableAlignment([
      {type: 'source', metricPrefix: 'Source[0]', tablePaths: ['orders_.*']},
      {type: 'sink', metricPrefix: 'Sink[0]', tablePaths: ['ods.${table_name}']},
    ]);
    expect(templateAlignment.source).toBeUndefined();
    expect(templateAlignment.sink).toBeUndefined();
  });

  it('preserves prefixed multi-table pairing when a stray unprefixed key shares the same leaf name', () => {
    const paired = buildPairedMetricRows({
      TableSourceReceivedCount: {
        'Source[0].db.orders': '100',
        'Source[0].db.users': '200',
      },
      TableSinkWriteCount: {
        'Sink[0].ods.orders': '100',
        'Sink[0].ods.users': '200',
        'ods.orders': '0',
      },
    });
    expect(paired).toHaveLength(2);
    expect(paired.map((row) => row.key)).toEqual(['orders', 'users']);
  });

  it('raises zero per-table counters to job-level totals on an unambiguous single-link job', () => {
    const metrics = {
      SourceReceivedCount: '300',
      SinkWriteCount: '300',
      SinkCommittedCount: '300',
      TableSourceReceivedCount: {'Source[0].wcan-common-tbox1-r1p24': '0'},
      TableSinkWriteCount: {'Sink[0].ah1110.ods_wcan_common_tbox1_json': '0'},
    };
    const alignment = buildMetricTableAlignment(singleLinkVertices, metrics);
    const paired = buildPairedMetricRows(metrics, alignment);
    expect(paired).toHaveLength(1);
    expect(paired[0]).toMatchObject({
      sourceCount: '300',
      sinkCount: '300',
      committedCount: '300',
    });
  });

  it('counts an aligned single-link aliased job as 1 logical table in extractJobMetricSummary', () => {
    const summary = extractJobMetricSummary({
      result_preview: {
        job_dag: {
          vertexInfoMap: [
            {vertexId: 1, type: 'source', vertexName: 'pipeline-1 [Source[0]-Kafka]', tablePaths: ['wcan-common-tbox1-r1p24']},
            {vertexId: 2, type: 'sink', vertexName: 'pipeline-1 [Sink[0]-ClickHouse]', tablePaths: ['ah1110.ods_wcan_common_tbox1_json']},
          ],
        },
        metrics: aliasedMetrics,
      },
    } as unknown as SyncJobInstance);
    expect(summary.tableCount).toBe(1);
    expect(summary.isMultiTable).toBe(false);
  });
});

describe('buildMetricFlowSankey', () => {
  it('builds source → sink → commit-state links plus a backlog branch', () => {
    const sankey = buildMetricFlowSankey(
      tableMetrics([
        {read: 500, write: 400, committed: 300},
        {read: 300, write: 300, committed: 300},
      ]),
      sankeyOpts,
    );
    expect(sankey.totalLinks).toBe(2);
    expect(sankey.mergedLinks).toBe(0);
    const linkOf = (source: string, target: string) =>
      sankey.links.find((link) => link.source === source && link.target === target);
    expect(linkOf('s:t0', 't:t0')?.value).toBe(400);
    expect(linkOf('s:t0', 'o:backlog')?.value).toBe(100);
    expect(linkOf('t:t0', 'o:committed')?.value).toBe(300);
    expect(linkOf('t:t0', 'o:pending')?.value).toBe(100);
    expect(linkOf('s:t1', 't:t1')?.value).toBe(300);
    expect(linkOf('t:t1', 'o:committed')?.value).toBe(300);
    expect(linkOf('t:t1', 'o:pending')).toBeUndefined();
    expect(sankey.nodes.map((node) => node.id)).toEqual([
      's:t0',
      't:t0',
      's:t1',
      't:t1',
      'o:committed',
      'o:pending',
      'o:backlog',
    ]);
  });

  it('omits outcome nodes whose total is zero', () => {
    const sankey = buildMetricFlowSankey(
      tableMetrics([
        {read: 100, write: 100, committed: 100},
        {read: 50, write: 50, committed: 50},
      ]),
      sankeyOpts,
    );
    expect(sankey.nodes.some((node) => node.id === 'o:pending')).toBe(false);
    expect(sankey.nodes.some((node) => node.id === 'o:backlog')).toBe(false);
    expect(sankey.nodes.some((node) => node.id === 'o:committed')).toBe(true);
  });

  it('treats a missing per-table commit metric as fully pending', () => {
    const sankey = buildMetricFlowSankey(
      tableMetrics([
        {read: 100, write: 80},
        {read: 60, write: 60},
      ]),
      sankeyOpts,
    );
    expect(sankey.nodes.some((node) => node.id === 'o:committed')).toBe(false);
    const pending = sankey.links
      .filter((link) => link.target === 'o:pending')
      .reduce((sum, link) => sum + link.value, 0);
    expect(pending).toBe(140);
  });

  it('merges links beyond the top N into a single other pair', () => {
    const entries = Array.from({length: 12}, (_, index) => ({
      read: 100 - index,
      write: 50,
      committed: 50,
    }));
    const sankey = buildMetricFlowSankey(tableMetrics(entries), {
      ...sankeyOpts,
      maxLinks: 8,
    });
    expect(sankey.totalLinks).toBe(12);
    expect(sankey.mergedLinks).toBe(4);
    const otherSource = sankey.nodes.find((node) => node.id === 's:__other__');
    const otherSink = sankey.nodes.find((node) => node.id === 't:__other__');
    expect(otherSource?.label).toBe('其它 4 张表');
    expect(otherSink?.label).toBe('其它');
    const merged = sankey.links.find(
      (link) => link.source === 's:__other__' && link.target === 't:__other__',
    );
    expect(merged?.value).toBe(200);
    const mergedBacklog = sankey.links.find(
      (link) => link.source === 's:__other__' && link.target === 'o:backlog',
    );
    // 后 4 条 read = 92/91/90/89，与 write=50 的差值合计 162。 / Backlog sum of the last 4 merged links.
    expect(mergedBacklog?.value).toBe(162);
  });

  it('skips zero-flow links so single-link jobs stay below the render gate', () => {
    const sankey = buildMetricFlowSankey(
      tableMetrics([{read: 0, write: 0}, {read: 10, write: 10, committed: 10}]),
      sankeyOpts,
    );
    expect(sankey.totalLinks).toBe(1);
  });

  it('marks only anomaly branches with the warning color', () => {
    const sankey = buildMetricFlowSankey(
      tableMetrics([
        {read: 500, write: 400, committed: 300},
        {read: 300, write: 300, committed: 300},
      ]),
      sankeyOpts,
    );
    const colors = Object.fromEntries(
      sankey.nodes.map((node) => [node.id, node.color]),
    );
    expect(colors['s:t0']).toBe('table-color');
    expect(colors['t:t0']).toBe('table-color');
    expect(colors['o:committed']).toBe('outcome-color');
    expect(colors['o:pending']).toBe('warn-color');
    expect(colors['o:backlog']).toBe('warn-color');

    const withPendingColor = buildMetricFlowSankey(
      tableMetrics([
        {read: 500, write: 400, committed: 300},
        {read: 300, write: 300, committed: 300},
      ]),
      {
        ...sankeyOpts,
        colors: {...sankeyOpts.colors, pending: 'info-color'},
      },
    );
    const pendingColors = Object.fromEntries(
      withPendingColor.nodes.map((node) => [node.id, node.color]),
    );
    expect(pendingColors['o:pending']).toBe('info-color');
    expect(pendingColors['o:backlog']).toBe('warn-color');
  });

  it('switches Sankey flow weights and fallback outcome node in bytes mode', () => {
    const metrics = {
      TableSourceReceivedCount: {'Source[0].db.small': 500, 'Source[1].db.wide': 20},
      TableSinkWriteCount: {'Sink[0].db.small': 500, 'Sink[1].db.wide': 20},
      TableSourceReceivedBytes: {'Source[0].db.small': 4096, 'Source[1].db.wide': 65536},
      TableSinkWriteBytes: {'Sink[0].db.small': 4096, 'Sink[1].db.wide': 49152},
    };
    const sankey = buildMetricFlowSankey(metrics, {
      ...sankeyOpts,
      labels: {...sankeyOpts.labels, written: '已写入'},
      unit: 'bytes',
    });
    // 体积口径按字节降序排序：db.wide（64 KB）排在 db.small（4 KB）之前；缺少表级提交字节时落到 o:written。
    // Byte mode sorts by byte volume descending (db.wide before db.small) and routes writes to o:written when TableSinkCommittedBytes is absent.
    expect(sankey.nodes[0].id).toBe('s:wide');
    expect(sankey.nodes.some((node) => node.id === 'o:written')).toBe(true);
    expect(sankey.nodes.some((node) => node.id === 'o:pending')).toBe(false);
    const backlog = sankey.links.find((link) => link.source === 's:wide' && link.target === 'o:backlog');
    expect(backlog?.value).toBe(16384);
  });
});

describe('metric byte mode helpers', () => {
  it('detects byte availability from global or per-table byte maps', () => {
    expect(hasMetricByteData({SourceReceivedCount: 100, SinkWriteCount: 100})).toBe(false);
    expect(hasMetricByteData({SourceReceivedBytes: 2048, SinkWriteBytes: 2048})).toBe(true);
    expect(
      hasMetricByteData({
        TableSourceReceivedBytes: {'Source[0].db.a': 1024, 'Source[1].db.b': 2048},
      }),
    ).toBe(true);
    expect(
      resolveMetricByteTotals({
        TableSourceReceivedBytes: {'Source[0].db.a': 1024, 'Source[1].db.b': 2048},
      }).readBytes,
    ).toBe(3072);
  });

  it('degrades cleanly without false pending warning when SinkCommittedBytes is missing', () => {
    const flow = summarizeMetricFlow(
      {SourceReceivedBytes: 10240, SinkWriteBytes: 10240},
      {unit: 'bytes'},
    );
    expect(flow.hasCommitMetric).toBe(false);
    expect(flow.pendingCommitCount).toBe(0);
    expect(flow.tone).toBe('neutral');
    expect(flow.segments.map((item) => item.key)).toEqual(['metricFlowVolumeBalanced']);

    const bar = buildMetricFlowBar(flow.readCount, flow.writeCount, flow.committedCount, {
      hasCommitMetric: flow.hasCommitMetric,
    });
    expect(bar.segments).toEqual([
      {key: 'written', value: 10240, share: 1},
      {key: 'backlog', value: 0, share: 0},
    ]);
  });

  it('reports pending commit bytes when SinkCommittedBytes is present', () => {
    const flow = summarizeMetricFlow(
      {SourceReceivedBytes: 10240, SinkWriteBytes: 10240, SinkCommittedBytes: 8192},
      {unit: 'bytes'},
    );
    expect(flow.hasCommitMetric).toBe(true);
    expect(flow.pendingCommitCount).toBe(2048);
    expect(flow.tone).toBe('warning');
    expect(flow.segments.map((item) => item.key)).toEqual([
      'metricFlowVolumeBalanced',
      'metricFlowPendingCommitBytes',
    ]);
  });
});
