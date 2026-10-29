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

// 实时诊断入口与配色工具的单元测试
// Unit tests for realtime insight entry and tone helpers

import {describe, expect, it} from 'vitest';
import {backpressureTone, latestVertexBusy} from '../job-detail/dag-model';
import {getObservabilityMode, getRealtimeEntry} from '../sync-studio-utils';
import {
  buildTimelineRows,
  engineEventFamily,
  schemaChangeSummary,
} from '../JobEventTimelineDialog';
import {buildObserveURL} from '@/components/common/cluster/SeaTunnelObserveMenu';
import type {SyncJobInstance} from '@/lib/services/sync';

function job(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    id: 1,
    task_id: 1,
    task_version: 1,
    run_type: 'run',
    platform_job_id: 'p1',
    engine_job_id: '1001',
    status: 'running',
    submit_spec: {},
    result_preview: {},
    error_message: '',
    created_by: 1,
    created_at: '',
    ...partial,
  } as SyncJobInstance;
}

describe('getObservabilityMode', () => {
  it('defaults to auto for missing or invalid values', () => {
    expect(getObservabilityMode(undefined)).toBe('auto');
    expect(getObservabilityMode({observability: 'weird'})).toBe('auto');
    expect(getObservabilityMode({observability: 'OFF'})).toBe('off');
    expect(getObservabilityMode({observability: 'on'})).toBe('on');
  });
});

describe('getRealtimeEntry', () => {
  it('builds the Web UI deep link from cluster and engine job id', () => {
    const entry = getRealtimeEntry(
      job({submit_spec: {cluster_id: 3, observability: {injected: true, supported: true}}}),
    );
    expect(entry).toEqual({
      supported: true,
      injected: true,
      running: true,
      webuiPath: '/api/v1/clusters/3/webui/#/jobs/1001',
    });
  });

  it('treats missing observability records as unsupported and finished jobs as not running', () => {
    const entry = getRealtimeEntry(job({status: 'success', submit_spec: {cluster_id: 3}}));
    expect(entry.supported).toBe(false);
    expect(entry.injected).toBe(false);
    expect(entry.running).toBe(false);
  });

  it('has no deep link without a cluster id', () => {
    expect(getRealtimeEntry(job({submit_spec: {}})).webuiPath).toBe('');
    expect(getRealtimeEntry(null).webuiPath).toBe('');
  });
});

describe('realtime tones', () => {
  it('follows the upstream backpressure tiers', () => {
    expect(backpressureTone(0)).toContain('muted');
    expect(backpressureTone(0.01)).toContain('emerald');
    expect(backpressureTone(0.1)).toContain('yellow');
    expect(backpressureTone(0.3)).toContain('orange');
    expect(backpressureTone(0.5)).toContain('red');
  });

  it('reads the busy field matching the vertex type', () => {
    const points = [{ts: 1, sourceReadRatio: 0.2, sinkBusyRatio: 0.9, transformBusyRatio: 0.4}];
    expect(latestVertexBusy({vertex_id: 1, type: 'source', name: '', points})).toBe(0.2);
    expect(latestVertexBusy({vertex_id: 1, type: 'sink', name: '', points})).toBe(0.9);
    expect(latestVertexBusy({vertex_id: 1, type: 'transform', name: '', points})).toBe(0.4);
    expect(latestVertexBusy({vertex_id: 1, type: 'sink', name: '', points: []})).toBeNull();
  });
});

describe('job event timeline helpers', () => {
  const event = (id: number, eventType: string, extra: Record<string, unknown> = {}) => ({
    id,
    cluster_id: 1,
    engine_job_id: '1001',
    event_type: eventType,
    event_time_ms: id * 1000,
    payload: {},
    received_at: '',
    ...extra,
  });

  it('classifies event types', () => {
    expect(engineEventFamily('JOB_STATUS')).toBe('status');
    expect(engineEventFamily('SCHEMA_CHANGE_ADD_COLUMN')).toBe('schema');
    expect(engineEventFamily('READER_MESSAGE_DELAYED')).toBe('delay');
    expect(engineEventFamily('LIFECYCLE_READER_OPEN')).toBe('lifecycle');
    expect(engineEventFamily('SOMETHING_NEW')).toBe('other');
  });

  it('collapses consecutive lifecycle events only', () => {
    const rows = buildTimelineRows([
      event(5, 'JOB_STATUS'),
      event(4, 'LIFECYCLE_READER_OPEN'),
      event(3, 'LIFECYCLE_READER_OPEN'),
      event(2, 'SCHEMA_CHANGE_ADD_COLUMN'),
      event(1, 'LIFECYCLE_ENUMERATOR_OPEN'),
    ]);
    expect(rows.map((row) => row.kind)).toEqual(['event', 'lifecycle', 'event', 'lifecycle']);
    expect(rows[1].kind === 'lifecycle' && rows[1].events.length).toBe(2);
  });

  it('summarizes schema changes by table and column', () => {
    expect(
      schemaChangeSummary({
        tableIdentifier: {databaseName: 'shop', tableName: 'orders'},
        column: {name: 'note'},
      }),
    ).toBe('shop.orders · note');
    expect(schemaChangeSummary({})).toBe('');
    expect(
      schemaChangeSummary({
        tableIdentifier: {databaseName: 'seatunnel_test', schemaName: 'public', tableName: 'cdc_orders'},
        events: [
          {eventType: 'SCHEMA_CHANGE_ADD_COLUMN', column: {name: 'remark'}},
          {eventType: 'SCHEMA_CHANGE_DROP_COLUMN', columnName: 'old'},
        ],
      }),
    ).toBe('seatunnel_test.public.cdc_orders · +remark, -old');
    // MySQL CDC 实测形状：删列的 column 是字符串，增列 / 改列带 sourceType。
    // Real MySQL CDC shapes: DROP carries the column as a string, ADD / MODIFY include sourceType.
    expect(
      schemaChangeSummary({
        tableIdentifier: {databaseName: 'stx_demo', tableName: 'customers'},
        events: [{eventType: 'SCHEMA_CHANGE_DROP_COLUMN', column: 'city'}],
      }),
    ).toBe('stx_demo.customers · -city');
    expect(
      schemaChangeSummary({
        tableIdentifier: {databaseName: 'stx_demo', tableName: 'orders'},
        events: [
          {eventType: 'SCHEMA_CHANGE_ADD_COLUMN', column: {name: 'coupon_code', sourceType: 'VARCHAR(32)'}},
          {eventType: 'SCHEMA_CHANGE_MODIFY_COLUMN', column: {name: 'status', sourceType: 'VARCHAR(32)'}},
        ],
      }),
    ).toBe('stx_demo.orders · +coupon_code VARCHAR(32), ~status VARCHAR(32)');
  });
});

describe('buildObserveURL', () => {
  it('opens the job detail hash route when a job id is given', () => {
    expect(buildObserveURL('/api/v1/clusters/3/webui/', '#/jobs/{job_id}', '1001')).toBe(
      '/api/v1/clusters/3/webui/#/jobs/1001',
    );
    expect(buildObserveURL('http://10.0.0.1:8080', '#/jobs/{job_id}', '1001')).toBe(
      'http://10.0.0.1:8080/#/jobs/1001',
    );
  });

  it('opens the cluster home without a job id and returns empty without a base', () => {
    expect(buildObserveURL('http://10.0.0.1:8080/', '#/jobs/{job_id}')).toBe('http://10.0.0.1:8080/');
    expect(buildObserveURL('', '#/jobs/{job_id}', '1001')).toBe('');
  });
});
