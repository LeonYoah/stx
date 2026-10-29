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

// 表结构解析与建表计划查找；夹具取自线上作业 #26 的 config tool 返回。
// Schema parsing and save-mode plan lookup; fixtures mirror the config tool output for production job #26.

import {describe, expect, it} from 'vitest';
import {normalizeJobDag} from '../dag-model';
import {resolveSaveModePlan, summarizeTableSchema} from '../table-schema';

const cdcOrdersSchema = {
  catalogName: 'Postgres',
  comment: '',
  partitionKeys: [],
  schema: {
    columns: [
      {name: 'id', dataType: 'BIGINT', nullable: false, defaultValue: null, comment: null},
      {name: 'note', dataType: 'STRING', nullable: true, defaultValue: null, comment: null},
      {name: 'amount', dataType: 'Decimal(10, 2)', nullable: true, defaultValue: 0, comment: '金额'},
    ],
    constraintKeys: [{columns: [{columnName: 'id', sortType: 'ASC'}], constraintName: 'cdc_orders_pkey', constraintType: 'UNIQUE_KEY'}],
    primaryKey: {columnNames: ['id'], name: 'cdc_orders_pkey'},
  },
};

describe('summarizeTableSchema', () => {
  it('parses columns, keys and nullable flags', () => {
    const summary = summarizeTableSchema(cdcOrdersSchema);
    expect(summary?.primaryKey).toEqual(['id']);
    expect(summary?.constraintTypes).toEqual(['UNIQUE_KEY']);
    expect(summary?.columns.map((column) => [column.name, column.primaryKey, column.uniqueKey, column.nullable])).toEqual([
      ['id', true, true, false],
      ['note', false, false, true],
      ['amount', false, false, true],
    ]);
    expect(summary?.columns[0].defaultValue).toBe('-');
    expect(summary?.columns[2]).toMatchObject({defaultValue: '0', comment: '金额'});
  });

  it('returns null without a usable schema so callers fall back to column names', () => {
    expect(summarizeTableSchema(undefined)).toBeNull();
    expect(summarizeTableSchema({schema: {columns: []}})).toBeNull();
    expect(summarizeTableSchema({schema: 'broken'})).toBeNull();
  });
});

describe('resolveSaveModePlan', () => {
  it('prefers the per-table plan and falls back to __default__', () => {
    const [vertex] = normalizeJobDag({
      vertexInfoMap: {
        '2': {
          vertexId: 2,
          type: 'sink',
          tablePaths: ['db.t1', 'db.t2'],
          saveModePreviews: {'db.t1': {supported: true, schemaSaveMode: 'CREATE'}, __default__: {supported: false}},
        },
      },
    }).vertices;
    expect(resolveSaveModePlan(vertex, 'db.t1')?.schemaSaveMode).toBe('CREATE');
    expect(resolveSaveModePlan(vertex, 'db.t2')?.supported).toBe(false);
  });
});
