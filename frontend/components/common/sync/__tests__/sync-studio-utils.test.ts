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
  filterTree,
  extractJobMetricSummary,
  isCursorInsideValueRegion,
  isNodeMatchingScope,
  resolveEnumSuggestionItems,
  resolveEnumValueBounds,
  resolveOptionAssignmentContext,
  resolveVariableCompletionContext,
  resolveVariableSuggestions,
} from '../sync-studio-utils';

describe('sync-studio-utils Monaco completion & assignment tests', () => {
  it('correctly resolves unquoted boolean assignment context regardless of cursor position', () => {
    const line = '  auto_commit = true';
    // equalsIndex is 14 ('=' is at index 14, col 15). Value 'true' occupies cols 17..21
    // Before 'true' (col 17)
    const atStart = resolveOptionAssignmentContext(line, 17);
    expect(atStart.inValueRegion).toBe(true);
    expect(atStart.optionKey).toBe('auto_commit');
    expect(atStart.bounds?.quoted).toBe(false);
    expect(atStart.bounds?.value).toBe('true');
    expect(atStart.bounds?.startColumn).toBe(17);
    expect(atStart.bounds?.endColumn).toBe(21);

    // Inside 'tr|ue' (col 19)
    const inMiddle = resolveOptionAssignmentContext(line, 19);
    expect(inMiddle.inValueRegion).toBe(true);
    expect(inMiddle.optionKey).toBe('auto_commit');
    expect(inMiddle.bounds?.value).toBe('true');

    // After 'true|' (col 21)
    const atEnd = resolveOptionAssignmentContext(line, 21);
    expect(atEnd.inValueRegion).toBe(true);
    expect(atEnd.optionKey).toBe('auto_commit');
    expect(atEnd.bounds?.value).toBe('true');
  });

  it('correctly resolves unquoted enum values like CLUSTER', () => {
    const line = 'job_schedule_strategy = CLUSTER';
    // 'job_schedule_strategy' is 21 chars. '=' is at index 22 (col 23). 'CLUSTER' is cols 25..32
    const ctx = resolveOptionAssignmentContext(line, 28);
    expect(ctx.inValueRegion).toBe(true);
    expect(ctx.optionKey).toBe('job_schedule_strategy');
    expect(ctx.bounds?.quoted).toBe(false);
    expect(ctx.bounds?.value).toBe('CLUSTER');
    expect(ctx.bounds?.startColumn).toBe(25);
    expect(ctx.bounds?.endColumn).toBe(32);
  });

  it('correctly resolves quoted enum values like "INITIAL"', () => {
    const line = 'startup.mode = "INITIAL"';
    const ctx = resolveOptionAssignmentContext(line, 18);
    expect(ctx.inValueRegion).toBe(true);
    expect(ctx.optionKey).toBe('startup.mode');
    expect(ctx.bounds?.quoted).toBe(true);
    expect(ctx.bounds?.value).toBe('INITIAL');
  });

  it('returns inValueRegion=false if cursor is in trailing comment', () => {
    const line = 'auto_commit = true # this is a comment';
    const commentStartCol = line.indexOf('#') + 1; // 1-based
    const insideComment = resolveOptionAssignmentContext(line, commentStartCol + 3);
    expect(insideComment.inValueRegion).toBe(false);
  });

  it('provides default boolean true/false enum items when schema is boolean', () => {
    const items = resolveEnumSuggestionItems({
      type: 'boolean',
      enum_values: [],
    });
    expect(items).toEqual([
      {label: 'true', value: 'true'},
      {label: 'false', value: 'false'},
    ]);
  });

  describe('resolveVariableCompletionContext', () => {
    it('detects when cursor is right after {{', () => {
      const line = '    password = {{';
      // 1-based column 18 is right after '{{'
      const ctx = resolveVariableCompletionContext(line, 18);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('');
      expect(ctx.startColumn).toBe(18);
      expect(ctx.endColumn).toBe(18);
      expect(ctx.hasClosingBraces).toBe(false);
    });

    it('detects when cursor is inside {{}} auto-closed by editor', () => {
      const line = '    password = {{}}';
      // 1-based column 18 is between '{{' and '}}'
      const ctx = resolveVariableCompletionContext(line, 18);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('');
      expect(ctx.startColumn).toBe(18);
      expect(ctx.endColumn).toBe(20); // covers '}}'
      expect(ctx.hasClosingBraces).toBe(true);
    });

    it('detects when user types m after {{ without closing braces', () => {
      const line = '    password = {{m';
      // 1-based column 19 is right after 'm'
      const ctx = resolveVariableCompletionContext(line, 19);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('m');
      expect(ctx.startColumn).toBe(18);
      expect(ctx.endColumn).toBe(19);
      expect(ctx.hasClosingBraces).toBe(false);
    });

    it('detects when user types m inside existing {{}}', () => {
      const line = '    password = {{m}}';
      // 1-based column 19 is right after 'm', before '}}'
      const ctx = resolveVariableCompletionContext(line, 19);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('m');
      expect(ctx.startColumn).toBe(18);
      expect(ctx.endColumn).toBe(21); // covers 'm}}'
      expect(ctx.hasClosingBraces).toBe(true);
    });

    it('handles spaced {{ mysql }} style', () => {
      const line = '    password = {{ mysql }}';
      // column 24 is 1-based index right after 'mysql'
      const ctx = resolveVariableCompletionContext(line, 24);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('mysql');
      expect(ctx.startColumn).toBe(19); // after space
      expect(ctx.hasLeadingSpace).toBe(true);
      expect(ctx.hasClosingBraces).toBe(true);
    });

    it('returns inVariable=false when outside {{}}', () => {
      const line = '    password = "root"';
      const ctx = resolveVariableCompletionContext(line, 17);
      expect(ctx.inVariable).toBe(false);
    });

    it('handles second {{ when multiple placeholders exist on line', () => {
      const line = 'url = "jdbc:mysql://{{host}}:{{port}}"';
      // Cursor is at 36 (after 'port')
      const ctx = resolveVariableCompletionContext(line, 36);
      expect(ctx.inVariable).toBe(true);
      expect(ctx.query).toBe('port');
      expect(ctx.startColumn).toBe(32);
      expect(ctx.hasClosingBraces).toBe(true);
    });
  });

  describe('resolveVariableSuggestions', () => {
    const mockMonaco = {
      languages: {
        CompletionItemKind: {
          Variable: 4,
          Constant: 14,
          Keyword: 17,
        },
      },
    };

    it('generates suggestions sorted with custom first, global second, system third', () => {
      const ctx = resolveVariableCompletionContext('password = {{m', 15);
      const customVars = [
        {id: '1', key: 'mysqlpas', value: 'secret123', type: 'secret'},
        {id: '2', key: 'max_connections', value: '10', type: 'string'},
      ];
      const globalVars = [
        {
          id: 1,
          key: 'mysql_host',
          value: '10.0.0.1',
          value_type: 'string',
          created_at: '',
          updated_at: '',
        },
      ];

      const items = resolveVariableSuggestions(
        mockMonaco,
        ctx,
        customVars as any,
        globalVars as any,
        {lineNumber: 5, column: 15},
      );

      expect(items.length).toBeGreaterThanOrEqual(3);

      // Custom variable 'mysqlpas' is first priority
      const mysqlpas = items.find((it) => it.label === 'mysqlpas');
      expect(mysqlpas).toBeDefined();
      expect(mysqlpas.sortText).toBe('0_mysqlpas');
      expect(mysqlpas.detail).toContain('[自定义变量]');
      expect(mysqlpas.detail).toContain('******'); // Secret masked
      expect(mysqlpas.insertText).toBe('mysqlpas}}');

      // Global variable 'mysql_host' is second priority
      const mysqlHost = items.find((it) => it.label === 'mysql_host');
      expect(mysqlHost).toBeDefined();
      expect(mysqlHost.sortText).toBe('1_mysql_host');
      expect(mysqlHost.detail).toContain('[全局变量]');
      expect(mysqlHost.insertText).toBe('mysql_host}}');

      // System variable 'system.biz.date' is third priority
      const bizDate = items.find((it) => it.label === 'system.biz.date');
      expect(bizDate).toBeDefined();
      expect(bizDate.sortText).toBe('2_system.biz.date');
      expect(bizDate.detail).toContain('[系统内置]');
      expect(bizDate.insertText).toBe('system.biz.date}}');
    });

    it('inserts space before closing braces if leading space is present', () => {
      const ctx = resolveVariableCompletionContext('password = {{ m', 16);
      const customVars = [
        {id: '1', key: 'mysqlpas', value: 'secret123', type: 'secret'},
      ];
      const items = resolveVariableSuggestions(
        mockMonaco,
        ctx,
        customVars as any,
        [],
        {lineNumber: 5, column: 16},
      );
      const mysqlpas = items.find((it) => it.label === 'mysqlpas');
      expect(mysqlpas.insertText).toBe('mysqlpas }}');
    });
  });

  describe('filterTree with filterScope tests', () => {
    const sampleTree = [
      {
        id: 100,
        node_type: 'folder' as const,
        name: 'Project Alpha',
        description: '',
        cluster_id: 1,
        engine_version: '2.3.13',
        mode: 'streaming' as const,
        status: 'draft' as const,
        content_format: 'hocon' as const,
        content: '',
        job_name: '',
        definition: {},
        sort_order: 1,
        current_version: 1,
        children: [
          {
            id: 1,
            node_type: 'file' as const,
            name: 'my_private_task.conf',
            created_by: 4,
            is_owner: true,
            is_public: false,
            description: '',
            cluster_id: 1,
            engine_version: '2.3.13',
            mode: 'streaming' as const,
            status: 'draft' as const,
            content_format: 'hocon' as const,
            content: 'env {}',
            job_name: 't1',
            definition: {},
            sort_order: 1,
            current_version: 1,
          },
          {
            id: 2,
            node_type: 'file' as const,
            name: 'public_shared_task.conf',
            created_by: 1, // Created by admin
            is_owner: false,
            is_public: true, // Public task
            description: '',
            cluster_id: 1,
            engine_version: '2.3.13',
            mode: 'streaming' as const,
            status: 'draft' as const,
            content_format: 'hocon' as const,
            content: 'env {}',
            job_name: 't2',
            definition: {},
            sort_order: 2,
            current_version: 1,
          },
          {
            id: 3,
            node_type: 'file' as const,
            name: 'other_user_private_task.conf',
            created_by: 2, // Created by user 2
            is_owner: false,
            is_public: false, // Private task of user 2
            description: '',
            cluster_id: 1,
            engine_version: '2.3.13',
            mode: 'streaming' as const,
            status: 'draft' as const,
            content_format: 'hocon' as const,
            content: 'env {}',
            job_name: 't3',
            definition: {},
            sort_order: 3,
            current_version: 1,
          },
        ],
      },
    ];

    it('returns all tasks when scope is all', () => {
      const result = filterTree(sampleTree, '', 'all', 4);
      expect(result).toHaveLength(1);
      expect(result[0].children).toHaveLength(3);
    });

    it('keeps own private tasks and public tasks, but filters out non-own private tasks when scope is mine_and_public', () => {
      const result = filterTree(sampleTree, '', 'mine_and_public', 4);
      expect(result).toHaveLength(1);
      const childNames = result[0].children?.map((c) => c.name);
      // Own task kept
      expect(childNames).toContain('my_private_task.conf');
      // Public task kept ("公开任务也属于自己任务")
      expect(childNames).toContain('public_shared_task.conf');
      // Other user's private task filtered out!
      expect(childNames).not.toContain('other_user_private_task.conf');
    });

    it('keeps only tasks created by user when scope is only_mine', () => {
      const result = filterTree(sampleTree, '', 'only_mine', 4);
      expect(result).toHaveLength(1);
      const childNames = result[0].children?.map((c) => c.name);
      expect(childNames).toEqual(['my_private_task.conf']);
    });
  });

  // 测试作业指标摘要提取与 SeaTunnel 3.0 多表聚合逻辑
  // Test job metric summary extraction and SeaTunnel 3.0 multi-table aggregation logic
  describe('extractJobMetricSummary tests', () => {
    it('correctly extracts single-table standard metrics', () => {
      const job: any = {
        id: 101,
        result_preview: {
          metrics: {
            SourceReceivedCount: 1250,
            SinkWriteCount: 1248,
            SourceReceivedQPS: 125,
            SinkWriteQPS: 124.8,
          },
        },
      };

      const summary = extractJobMetricSummary(job);
      expect(summary.readCount).toBe(1250);
      expect(summary.writeCount).toBe(1248);
      expect(summary.averageSpeed).toBeCloseTo(124.9);
      expect(summary.tableCount).toBe(0);
      expect(summary.isMultiTable).toBe(false);
    });

    it('correctly aggregates SeaTunnel 3.0 per-table metrics and identifies multi-table jobs', () => {
      const job: any = {
        id: 102,
        result_preview: {
          metrics: {
            // 全局读写未直接提供数字，而是细化到具体表
            // Global counts not provided directly, fine-grained to specific tables
            TableSourceReceivedCount: {
              'db.users': 1500,
              'db.orders': 3500,
            },
            TableSinkWriteCount: {
              'sink_db.users': 1500,
              'sink_db.orders': 3498,
            },
            TableSourceReceivedQPS: {
              'db.users': 150,
              'db.orders': 350,
            },
            TableSinkWriteQPS: {
              'sink_db.users': 150,
              'sink_db.orders': 348,
            },
          },
        },
      };

      const summary = extractJobMetricSummary(job);
      expect(summary.readCount).toBe(5000);
      expect(summary.writeCount).toBe(4998);
      expect(summary.averageSpeed).toBe(499); // (500 + 498) / 2
      expect(summary.tableCount).toBe(4);
      expect(summary.isMultiTable).toBe(true);
    });
  });
});

