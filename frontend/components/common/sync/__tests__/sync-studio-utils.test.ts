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
import type {
  SyncJobInstance,
  SyncJobListParams,
} from '@/lib/services/sync';
import {
  buildInsertedCuratedContent,
  buildTaskPayloadFromEditor,
  buildStudioGridTemplateColumns,
  buildStudioGridTemplateRows,
  clampStudioConsoleHeight,
  clampStudioLeftWidth,
  DEFAULT_STUDIO_LAYOUT_PREFS,
  formatRateDisplay,
  isZeroMetricValue,
  detectVariables,
  dolphinPushBlockedReason,
  EMPTY_EDITOR,
  filterTree,
  extractJobMetricSummary,
  countLogicalTables,
  formatMetricDisplayValue,
  getRunModeLabel,
  isCursorInsideValueRegion,
  isNodeMatchingScope,
  patchTreeNode,
  formatValueConstraintHints,
  isCursorAtFinishedEnumValue,
  readStudioLayoutPrefs,
  resolveEffectiveStudioLayout,
  resolveEnumSuggestionItems,
  resolveEnumValueBounds,
  resolveOptionAssignmentContext,
  resolveStudioTopbarTier,
  resolveStudioViewportMode,
  resolveTaskAccessLevel,
  resolveVariableCompletionContext,
  resolveVariableSuggestions,
  studioSaveSegment,
  formatSyncUserFacingError,
  findSyncJobsByIdInput,
  STUDIO_CENTER_MIN_WIDTH,
  STUDIO_CONSOLE_HEIGHT_MAX,
  STUDIO_EDITOR_MIN_HEIGHT,
  STUDIO_LAYOUT_STORAGE_KEY,
  STUDIO_LEFT_WIDTH_MAX,
  STUDIO_RIGHT_WIDTH_MAX,
  writeStudioLayoutPrefs,
  type StudioEffectiveLayoutInput,
} from '../sync-studio-utils';

describe('studio save segment', () => {
  it('shows saved only while an existing file is clean', () => {
    expect(
      studioSaveSegment({pending: false, hasFile: true, dirty: false}),
    ).toBe('saved');
  });

  it('returns to save when the draft changes, including a clean file that becomes dirty', () => {
    expect(
      studioSaveSegment({pending: false, hasFile: true, dirty: true}),
    ).toBe('save');
    expect(
      studioSaveSegment({pending: false, hasFile: false, dirty: false}),
    ).toBe('save');
  });

  it('keeps pending ahead of the draft, and a failed save that stays dirty is save', () => {
    expect(
      studioSaveSegment({pending: true, hasFile: true, dirty: false}),
    ).toBe('pending');
    expect(
      studioSaveSegment({pending: false, hasFile: true, dirty: true}),
    ).toBe('save');
  });
});

describe('studio layout prefs (#110)', () => {
  it('defaults to left expanded and right collapsed when no record', () => {
    expect(DEFAULT_STUDIO_LAYOUT_PREFS).toEqual({
      leftCollapsed: false,
      rightCollapsed: true,
      leftWidth: 240,
      rightWidth: 360,
      consoleHeight: 260,
    });
    expect(readStudioLayoutPrefs({getItem: () => null})).toEqual(
      DEFAULT_STUDIO_LAYOUT_PREFS,
    );
  });

  it('reads and writes independent left/right collapse flags', () => {
    const memory = new Map<string, string>();
    const storage = {
      getItem: (key: string) => memory.get(key) ?? null,
      setItem: (key: string, value: string) => {
        memory.set(key, value);
      },
    };

    writeStudioLayoutPrefs(
      {
        leftCollapsed: true,
        rightCollapsed: false,
        leftWidth: 280,
        rightWidth: 400,
        consoleHeight: 320,
      },
      storage,
    );
    expect(memory.get(STUDIO_LAYOUT_STORAGE_KEY)).toContain('"leftCollapsed":true');
    expect(readStudioLayoutPrefs(storage)).toEqual({
      leftCollapsed: true,
      rightCollapsed: false,
      leftWidth: 280,
      rightWidth: 400,
      consoleHeight: 320,
    });
  });

  it('keeps default widths when an older collapse-only record is stored', () => {
    expect(
      readStudioLayoutPrefs({
        getItem: () => '{"leftCollapsed":true,"rightCollapsed":false}',
      }),
    ).toEqual({
      leftCollapsed: true,
      rightCollapsed: false,
      leftWidth: 240,
      rightWidth: 360,
      consoleHeight: 260,
    });
  });

  it('builds grid columns that free collapsed tracks with 0px (#110 AC11 topbar toggles)', () => {
    expect(buildStudioGridTemplateColumns(false, true)).toBe(
      '240px minmax(0,1fr) 0px',
    );
    expect(buildStudioGridTemplateColumns(true, true)).toBe(
      '0px minmax(0,1fr) 0px',
    );
    expect(buildStudioGridTemplateColumns(false, false, 300, 400)).toBe(
      '300px minmax(0,1fr) 400px',
    );
  });

  it('builds grid rows without a reserved handle track', () => {
    expect(buildStudioGridTemplateRows(false, 280)).toBe('minmax(0,1fr) 280px');
    expect(buildStudioGridTemplateRows(true, 280)).toBe(
      '0px minmax(0,1fr)',
    );
  });

  it('clamps explorer and console sizes to the allowed range', () => {
    expect(clampStudioLeftWidth(80)).toBe(180);
    expect(clampStudioLeftWidth(900)).toBe(420);
    expect(clampStudioConsoleHeight(40)).toBe(140);
    expect(clampStudioConsoleHeight(900)).toBe(560);
  });
});

describe('formatValueConstraintHints', () => {
  it('summarizes matching value constraints for hover', () => {
    const hints = formatValueConstraintHints('port', [
      {
        optionKey: 'port',
        operator: 'GREATER_OR_EQUAL',
        expectValue: 1,
        and: true,
        next: {
          optionKey: 'port',
          operator: 'LESS_OR_EQUAL',
          expectValue: 65535,
        },
      },
      {
        optionKey: 'timeout',
        operator: 'GREATER_THAN',
        expectValue: 0,
      },
    ]);
    expect(hints).toEqual(['≥ 1', '≤ 65535']);
  });

  it('returns empty when option has no constraints', () => {
    expect(formatValueConstraintHints('port', [])).toEqual([]);
    expect(formatValueConstraintHints('port', undefined as any)).toEqual([]);
  });
});

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

  it('treats cursor at end of finished quoted enum as complete (Enter should newline)', () => {
    const line = '  job.mode = "BATCH"';
    const ctx = resolveOptionAssignmentContext(line, line.length + 1);
    expect(ctx.bounds?.value).toBe('BATCH');
    expect(
      isCursorAtFinishedEnumValue(ctx.bounds, line.length + 1, [
        'BATCH',
        'STREAMING',
      ]),
    ).toBe(true);
    // 光标仍在值中间时可切换枚举 / still switchable when cursor is inside the value
    const inside = resolveOptionAssignmentContext(line, ctx.bounds!.startColumn + 1);
    expect(
      isCursorAtFinishedEnumValue(inside.bounds, ctx.bounds!.startColumn + 1, [
        'BATCH',
        'STREAMING',
      ]),
    ).toBe(false);
  });

  it('treats cursor at end of finished unquoted enum as complete', () => {
    const line = 'job_schedule_strategy = CLUSTER';
    const ctx = resolveOptionAssignmentContext(line, line.length + 1);
    expect(ctx.bounds?.value).toBe('CLUSTER');
    expect(
      isCursorAtFinishedEnumValue(ctx.bounds, line.length + 1, [
        'CLUSTER',
        'ENGINE',
      ]),
    ).toBe(true);
  });

  it('still offers enum completion for partial values at end of line', () => {
    const line = '  job.mode = "BAT"';
    const ctx = resolveOptionAssignmentContext(line, line.length + 1);
    expect(ctx.bounds?.value).toBe('BAT');
    expect(
      isCursorAtFinishedEnumValue(ctx.bounds, line.length + 1, [
        'BATCH',
        'STREAMING',
      ]),
    ).toBe(false);
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

    it('keeps only own tasks when scope is mine_and_public; others public/private stay under all', () => {
      const result = filterTree(sampleTree, '', 'mine_and_public', 4);
      expect(result).toHaveLength(1);
      const childNames = result[0].children?.map((c) => c.name);
      expect(childNames).toContain('my_private_task.conf');
      // Other user's public task is not "mine" — visible only under "all"
      expect(childNames).not.toContain('public_shared_task.conf');
      expect(childNames).not.toContain('other_user_private_task.conf');
    });

    it('keeps tasks collaborated on by current user in mine_and_public but not only_mine', () => {
      const tree = [
        {
          ...sampleTree[0],
          children: [
            {
              ...sampleTree[0].children![0],
              id: 9,
              name: 'collab_shared_task.conf',
              created_by: 2,
              is_owner: false,
              is_public: false,
              is_collaborator: true,
            },
          ],
        },
      ];
      expect(
        filterTree(tree, '', 'mine_and_public', 4)[0].children?.map(
          (c) => c.name,
        ),
      ).toEqual(['collab_shared_task.conf']);
      expect(filterTree(tree, '', 'only_mine', 4)).toHaveLength(0);
    });

    it('admin is_owner does not leak others tasks into mine scopes', () => {
      // 管理员视角下后端对全部节点回 is_owner=true（可管理语义），过滤必须看 created_by。
      // Backend returns is_owner=true on every node for admins (manage semantics);
      // filtering must trust created_by instead.
      const tree = [
        {
          ...sampleTree[0],
          children: [
            {
              ...sampleTree[0].children![1],
              id: 8,
              name: 'lyb_admin_visible.conf',
              created_by: 1,
              is_owner: true,
              is_public: false,
              is_collaborator: false,
            },
            {
              ...sampleTree[0].children![0],
              id: 7,
              name: 'mine_admin_task.conf',
              created_by: 4,
              is_owner: true,
              is_public: false,
              is_collaborator: false,
            },
          ],
        },
      ];
      for (const scope of ['mine_and_public', 'only_mine'] as const) {
        const names = filterTree(tree, '', scope, 4)[0].children?.map(
          (c) => c.name,
        );
        expect(names).toEqual(['mine_admin_task.conf']);
      }
    });

    it('keeps only tasks created by user when scope is only_mine', () => {
      const result = filterTree(sampleTree, '', 'only_mine', 4);
      expect(result).toHaveLength(1);
      const childNames = result[0].children?.map((c) => c.name);
      expect(childNames).toEqual(['my_private_task.conf']);
    });

    // 回归 #108：空目录在非 all 作用域必须走 is_owner / 作用域契约，不能一律隐藏。
    // Regression #108: empty folders under non-all scopes must honor ownership/scope, not always hide.
    describe('empty folder visibility by ownership and scope (#108)', () => {
      const baseFolder = {
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
      };

      const ownedEmptyFolder = {
        ...baseFolder,
        id: 1,
        node_type: 'folder' as const,
        name: 'test108-owned-empty',
        created_by: 4,
        is_owner: true,
        is_public: false,
        is_collaborator: false,
        children: [],
      };

      const otherPrivateEmptyFolder = {
        ...baseFolder,
        id: 2,
        node_type: 'folder' as const,
        name: 'other-private-empty',
        created_by: 2,
        is_owner: false,
        is_public: false,
        is_collaborator: false,
        children: [],
      };

      const otherPublicEmptyFolder = {
        ...baseFolder,
        id: 3,
        node_type: 'folder' as const,
        name: 'other-public-empty',
        created_by: 2,
        is_owner: false,
        is_public: true,
        is_collaborator: false,
        children: [],
      };

      const nestedOwnedEmptyUnderForeignShell = [
        {
          ...baseFolder,
          id: 10,
          node_type: 'folder' as const,
          name: 'foreign-parent',
          created_by: 2,
          is_owner: false,
          is_public: false,
          is_collaborator: false,
          children: [
            {
              ...baseFolder,
              id: 11,
              parent_id: 10,
              node_type: 'folder' as const,
              name: 'my-nested-empty',
              created_by: 4,
              is_owner: true,
              is_public: false,
              is_collaborator: false,
              children: [],
            },
            {
              ...baseFolder,
              id: 12,
              parent_id: 10,
              node_type: 'folder' as const,
              name: 'other-nested-empty',
              created_by: 2,
              is_owner: false,
              is_public: false,
              is_collaborator: false,
              children: [],
            },
          ],
        },
      ];

      it('shows owned empty folder in all / mine_and_public / only_mine', () => {
        const tree = [ownedEmptyFolder];
        for (const scope of ['all', 'mine_and_public', 'only_mine'] as const) {
          const result = filterTree(tree, '', scope, 4);
          expect(result.map((n) => n.name)).toEqual(['test108-owned-empty']);
        }
      });

      it('hides other users private empty folder outside all scope', () => {
        const tree = [otherPrivateEmptyFolder];
        expect(filterTree(tree, '', 'all', 4).map((n) => n.name)).toEqual([
          'other-private-empty',
        ]);
        expect(filterTree(tree, '', 'mine_and_public', 4)).toHaveLength(0);
        expect(filterTree(tree, '', 'only_mine', 4)).toHaveLength(0);
      });

      it('hides other users public empty folder outside all scope', () => {
        const tree = [otherPublicEmptyFolder];
        expect(filterTree(tree, '', 'mine_and_public', 4)).toHaveLength(0);
        expect(filterTree(tree, '', 'only_mine', 4)).toHaveLength(0);
        expect(filterTree(tree, '', 'all', 4).map((n) => n.name)).toEqual([
          'other-public-empty',
        ]);
      });

      it('preserves nested owned empty folder path while hiding unauthorized siblings', () => {
        const mine = filterTree(nestedOwnedEmptyUnderForeignShell, '', 'only_mine', 4);
        expect(mine).toHaveLength(1);
        expect(mine[0].name).toBe('foreign-parent');
        expect(mine[0].children?.map((c) => c.name)).toEqual(['my-nested-empty']);

        const mineAndPublic = filterTree(
          nestedOwnedEmptyUnderForeignShell,
          '',
          'mine_and_public',
          4,
        );
        expect(mineAndPublic).toHaveLength(1);
        expect(mineAndPublic[0].children?.map((c) => c.name)).toEqual([
          'my-nested-empty',
        ]);

        const all = filterTree(nestedOwnedEmptyUnderForeignShell, '', 'all', 4);
        expect(all[0].children?.map((c) => c.name)).toEqual([
          'my-nested-empty',
          'other-nested-empty',
        ]);
      });

      it('keeps owned empty folder when keyword matches folder name under only_mine', () => {
        const result = filterTree([ownedEmptyFolder], 'test108', 'only_mine', 4);
        expect(result.map((n) => n.name)).toEqual(['test108-owned-empty']);
        expect(filterTree([ownedEmptyFolder], 'nomatch', 'only_mine', 4)).toHaveLength(
          0,
        );
      });
    });
  });

  // 测试作业指标摘要提取与 SeaTunnel 3.0 多表聚合逻辑
  // Test job metric summary extraction and SeaTunnel 3.0 table-level aggregation logic
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

    it('treats Source/Sink endpoints of one table as single-table', () => {
      const job: any = {
        id: 1015,
        result_preview: {
          metrics: {
            TableSourceReceivedCount: {'Source[0].fake': 10},
            TableSinkWriteCount: {'Sink[0].fake': 10},
            TableSinkCommittedCount: {'Sink[0].fake': 10},
          },
        },
      };

      const summary = extractJobMetricSummary(job);
      expect(summary.readCount).toBe(10);
      expect(summary.writeCount).toBe(10);
      expect(summary.tableCount).toBe(1);
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
              'Source[0].db.users': 1500,
              'Source[0].db.orders': 3500,
            },
            TableSinkWriteCount: {
              'Sink[0].sink_db.users': 1500,
              'Sink[0].sink_db.orders': 3498,
            },
            TableSourceReceivedQPS: {
              'Source[0].db.users': 150,
              'Source[0].db.orders': 350,
            },
            TableSinkWriteQPS: {
              'Sink[0].sink_db.users': 150,
              'Sink[0].sink_db.orders': 348,
            },
          },
        },
      };

      const summary = extractJobMetricSummary(job);
      expect(summary.readCount).toBe(5000);
      expect(summary.writeCount).toBe(4998);
      expect(summary.averageSpeed).toBe(499); // (500 + 498) / 2
      // users / orders 两张逻辑表，Source+Sink 端点不再计成 4
      // Two logical tables (users/orders); Source+Sink endpoints no longer count as 4
      expect(summary.tableCount).toBe(2);
      expect(summary.isMultiTable).toBe(true);
    });

    it('countLogicalTables ignores Source/Sink prefixes', () => {
      expect(
        countLogicalTables(['Source[0].fake', 'Sink[0].fake']),
      ).toBe(1);
      expect(
        countLogicalTables([
          'Source[0].db.users',
          'Source[0].db.orders',
          'Sink[0].archive_users',
          'Sink[0].orders',
        ]),
      ).toBe(2);
    });
  });
});

describe('curated template helpers', () => {
  it('detects credential {{vars}} from curated seed style content', () => {
    const content = `
source {
  Jdbc { password = "{{jdbc_password}}" }
  MySQL-CDC { password = "{{cdc_password}}" }
  FtpFile { password = "{{ftp_password}}" }
  S3File {
    access_key = "{{s3_access_key}}"
    secret_key = "{{s3_secret_key}}"
  }
}
`;
    expect(detectVariables(content)).toEqual([
      'cdc_password',
      'ftp_password',
      'jdbc_password',
      's3_access_key',
      's3_secret_key',
    ]);
  });

  it('replaces env block and keeps other sections when inserting curated env', () => {
    const existing = `env {\n  job.mode = "STREAMING"\n}\n\nsource {\n  FakeSource {}\n}\n`;
    const inserted = buildInsertedCuratedContent(
      existing,
      'env',
      'job.mode = "BATCH"',
    );
    expect(inserted.nextContent).toContain('job.mode = "BATCH"');
    expect(inserted.nextContent).toContain('FakeSource');
    expect(inserted.nextContent).not.toContain('STREAMING');
  });

  it('does not nest env when curated seed already contains env block with comments', () => {
    const seed = `# env · BATCH
# curated-section: env | batch

env {
  parallelism = 1
  job.mode = "BATCH"
}`;
    const inserted = buildInsertedCuratedContent('', 'env', seed);
    expect(inserted.nextContent.match(/\benv\s*\{/g)?.length).toBe(1);
    expect(inserted.nextContent).toContain('parallelism = 1');
    expect(inserted.nextContent).not.toMatch(/env\s*\{\s*\n\s*# env/);

    const withExisting = buildInsertedCuratedContent(
      'env {\n  job.mode = "STREAMING"\n}\n\nsource {\n  FakeSource {}\n}\n',
      'env',
      seed,
    );
    expect(withExisting.nextContent.match(/\benv\s*\{/g)?.length).toBe(1);
    expect(withExisting.nextContent).toContain('FakeSource');
    expect(withExisting.nextContent).not.toContain('STREAMING');
  });

  it('combo insert replaces entire editor content', () => {
    const inserted = buildInsertedCuratedContent(
      'old content',
      'combo',
      'env { job.mode = "BATCH" }',
    );
    expect(inserted.replacesAll).toBe(true);
    expect(inserted.nextContent).toBe('env { job.mode = "BATCH" }');
  });
});

describe('formatMetricDisplayValue', () => {
  it('keeps scalars readable', () => {
    expect(formatMetricDisplayValue(null)).toBe('-');
    expect(formatMetricDisplayValue(12345)).toBe((12345).toLocaleString());
    expect(formatMetricDisplayValue('ok')).toBe('ok');
  });

  it('formats *PerVertex style maps instead of [object Object]', () => {
    expect(
      formatMetricDisplayValue({
        '0': 1.5,
        '1': 0,
      }),
    ).toBe(`0: ${(1.5).toLocaleString()}, 1: 0`);
    expect(formatMetricDisplayValue({})).toBe('{}');
  });

  it('formats nested objects via JSON', () => {
    expect(formatMetricDisplayValue({a: {b: 1}})).toBe('{"a":{"b":1}}');
  });
});

describe('getRunModeLabel', () => {
  const t = ((key: string) => key) as Parameters<typeof getRunModeLabel>[1];

  it('labels Dolphin-triggered schedule jobs separately from built-in cron', () => {
    expect(
      getRunModeLabel(
        {
          id: 1,
          task_id: 2,
          task_version: 1,
          run_type: 'schedule',
          platform_job_id: 'p',
          engine_job_id: '',
          status: 'failed',
          submit_spec: {trigger_source: 'dolphin'},
          result_preview: {},
          error_message: '',
          created_by: 1,
          created_at: '',
          updated_at: '',
          external_ref: 'ds:1001:88',
        },
        t,
      ),
    ).toBe('runModeDolphin');
    expect(
      getRunModeLabel(
        {
          id: 2,
          task_id: 2,
          task_version: 1,
          run_type: 'schedule',
          platform_job_id: 'p',
          engine_job_id: '',
          status: 'success',
          submit_spec: {trigger_source: 'schedule'},
          result_preview: {},
          error_message: '',
          created_by: 0,
          created_at: '',
          updated_at: '',
        },
        t,
      ),
    ).toBe('runModeSchedule');
  });
});

describe('dolphinPushBlockedReason', () => {
  it('requires a published version and a newer version before update', () => {
    expect(dolphinPushBlockedReason({published_version: 0})).toBe('unpublished');
    expect(
      dolphinPushBlockedReason({
        published_version: 1,
        pushed_version: 1,
        binding: {id: 1},
      }),
    ).toBe('no_new_version');
    expect(
      dolphinPushBlockedReason({
        published_version: 2,
        pushed_version: 1,
        binding: {id: 1},
      }),
    ).toBeNull();
  });
});


describe('isZeroMetricValue', () => {
  // 全 0 的扩展指标折叠显示；按算子 / 按表的 map 要每一项都是 0。
  // All-zero extended metrics are collapsed; per-vertex / per-table maps must be zero in every entry.
  it('detects zero numbers, numeric strings and all-zero maps', () => {
    expect(isZeroMetricValue(0)).toBe(true);
    expect(isZeroMetricValue('0.0')).toBe(true);
    expect(isZeroMetricValue({'Source[0]': '0'})).toBe(true);
    expect(isZeroMetricValue({'Source[0]': '0', 'Sink[0]': '3'})).toBe(false);
    expect(isZeroMetricValue('12')).toBe(false);
    expect(isZeroMetricValue('')).toBe(false);
    expect(isZeroMetricValue({})).toBe(false);
    expect(isZeroMetricValue(null)).toBe(false);
  });
});

describe('formatRateDisplay', () => {
  it('rounds raw engine floats to two decimals and keeps non-numeric values', () => {
    expect(formatRateDisplay('1.9704569255360194')).toBe('1.97');
    expect(formatRateDisplay('3983.0990029341087')).toBe('3983.1');
    expect(formatRateDisplay('0')).toBe('0');
    expect(formatRateDisplay('-')).toBe('-');
    expect(formatRateDisplay(undefined)).toBe('-');
  });
});

describe('formatSyncUserFacingError', () => {
  const t = ((key: string) => key) as any;

  it('formats undefined template variable errors nicely', () => {
    const raw = 'sync: undefined template variable: {{asdad}} (未在任务自定义变量或全局变量中配置)';
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.title).toBe('undefinedVariableTitle');
    expect(formatted.description).toContain('{{asdad}}');
    expect(formatted.suggestion).toBeDefined();
  });

  it('formats HOCON brace syntax errors with clear suggestion', () => {
    const raw = "String: 46: expecting a close parentheses ')' here, not: '{'";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.title).toBe('configParseFailedTitle');
    expect(formatted.suggestion).toBeDefined();
  });

  // 普通 HOCON 括号错误经后端 humanize 后仍会提到「{{变量}}」，不能误报为未定义变量。
  // Ordinary HOCON brace errors whose humanized text still mentions "{{变量}}" must stay syntax errors.
  it('keeps ambiguous HOCON humanized brace errors as syntax errors, not undefined variables', () => {
    const raw =
      "sync: 配置解析失败，检测到未加双引号的 {{变量}} 或未闭合括号（若引用了变量请确认变量已定义，或使用双引号包裹 \"{{变量}}\"）：String: 46: expecting a close parentheses ')' here, not: '{'";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.title).toBe('configParseFailedTitle');
    expect(formatted.title).not.toBe('undefinedVariableTitle');
    expect(formatted.suggestion).toContain('语法结构');
  });

  it('classifies raw engine connector-missing errors with an install action', () => {
    const raw =
      "submit job failed: status=500 body=... Unable to create a sink for identifier 'Jdbc' ...";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.title).toBe('connectorMissingTitle');
    expect(formatted.category).toBe('connector');
    expect(formatted.description).toBe('connectorMissingDescription');
    expect(formatted.action?.href).toBe('/plugins?q=jdbc');
    expect(formatted.raw).toBe(raw);
  });

  it('classifies backend connector_missing error_code without engine wording', () => {
    const error = Object.assign(new Error('submit failed'), {
      errorCode: 'connector_missing',
    });
    const formatted = formatSyncUserFacingError(error, 'Default Title', t);
    expect(formatted.category).toBe('connector');
    expect(formatted.action?.href).toBe('/plugins?q=http');
  });

  it('uses the preview-specific description for the injected Http sink', () => {
    const raw = "Unable to create a sink for identifier 'Http'";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t, {
      preview: true,
    });
    expect(formatted.description).toBe('connectorMissingPreviewDescription');
    expect(formatted.action?.href).toBe('/plugins?q=http');
  });

  it('keeps the generic description when a non-Http connector is missing in preview', () => {
    const raw = "Unable to create a source for identifier 'Mysql-CDC'";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t, {
      preview: true,
    });
    expect(formatted.description).toBe('connectorMissingDescription');
    expect(formatted.action?.href).toBe('/plugins?q=mysql-cdc');
  });

  it('does not classify ordinary submit failures as connector-missing', () => {
    const raw = 'submit job failed: status=500 body=connection refused';
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.category).not.toBe('connector');
  });

  it('does not classify API-06 wording with a deeper caused-by as connector-missing', () => {
    const raw =
      "Source[0]-MySQL-CDC official tablePath resolution failed: FactoryException: ErrorCode:[API-06], ErrorDescription:[Factory initialize failed] - Unable to create a source for identifier 'MySQL-CDC'. | caused by: CatalogException: ErrorCode:[API-03], ErrorDescription:[Catalog initialize failed] - Failed connecting to the configured JDBC URL via JDBC.";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.category).not.toBe('connector');
    expect(formatted.description).toContain('JDBC URL');
  });

  // 引擎堆栈按最深 caused-by 分类：API-09 中间层的 "Failed connecting" 不能盖过 Access denied
  // Engine stacktraces classify by deepest caused-by: the intermediate API-09 "Failed connecting"
  // must not mask the real "Access denied" auth failure.
  it('classifies engine save-mode failure by deepest caused-by and summarizes the stack', () => {
    const body = {
      status: 'fail',
      message:
        "CompletionException: org.apache.seatunnel.engine.common.exception.JobException: org.apache.seatunnel.common.exception.SeaTunnelRuntimeException: ErrorCode:[API-09], ErrorDescription:[Handle save mode failed]\n" +
        '\tat org.apache.seatunnel.engine.server.master.JobMaster.handleSaveMode(JobMaster.java:792)\n' +
        'Caused by: org.apache.seatunnel.api.table.catalog.exception.CatalogException: ErrorCode:[API-03], ErrorDescription:[Catalog initialize failed] - Failed connecting to the configured JDBC URL via JDBC.\n' +
        '\tat org.apache.seatunnel.connectors.seatunnel.jdbc.catalog.AbstractJdbcCatalog.getConnection(AbstractJdbcCatalog.java:155)\n' +
        "Caused by: java.sql.SQLException: Access denied for user 'root'@'172.17.0.1' (using password: YES)",
    };
    const raw = `submit job failed: status=500 body=${JSON.stringify(body)}`;
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.category).toBe('auth');
    expect(formatted.description).toContain('Handle save mode failed');
    expect(formatted.description).toContain('Access denied');
    expect(formatted.description).not.toContain('at org.apache');
    // 堆栈区展示解码后的引擎堆栈：保留状态行 + 真实换行的完整链
    // Stacktrace view shows the decoded engine stack: status line + real newlines
    expect(formatted.raw).toContain('status=500');
    expect(formatted.raw).toContain('Caused by: java.sql.SQLException');
    expect(formatted.raw).not.toContain('\\n\\tat');
  });

  it('falls back to whole-message probing when the deepest cause has no signal', () => {
    const body = {
      status: 'fail',
      message:
        'JobException: something failed\n' +
        '\tat com.example.Foo.run(Foo.java:1)\n' +
        'Caused by: java.lang.RuntimeException: connection timed out after 30s',
    };
    const raw = `submit job failed: status=500 body=${JSON.stringify(body)}`;
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.category).toBe('network');
    expect(formatted.description).toContain('timed out');
  });

  it('still classifies when a caused-by layer reports plugin absent', () => {
    const raw =
      "Unable to create a sink for identifier 'Http'. | caused by: RuntimeException: Plugin PluginIdentifier{engineType='seatunnel', pluginType='sink', pluginName='Http'} not found.";
    const formatted = formatSyncUserFacingError(raw, 'Default Title', t);
    expect(formatted.category).toBe('connector');
    expect(formatted.raw).toBe(raw);
  });
});


describe('findSyncJobsByIdInput', () => {
  const job = (id: number, platform?: string, engine?: string) =>
    ({id, platform_job_id: platform, engine_job_id: engine}) as SyncJobInstance;

  const listJobsOf = (
    hits: Record<string, SyncJobInstance[]>,
    calls: string[] = [],
  ) =>
    (async (params: SyncJobListParams) => {
      const key = params.job_id
        ? `id:${params.job_id}`
        : params.platform_job_id
          ? `platform:${params.platform_job_id}`
          : `engine:${params.engine_job_id}`;
      calls.push(key);
      return {items: hits[key] ?? []};
    }) as (params: SyncJobListParams) => Promise<{items?: SyncJobInstance[]}>;

  it('strips # prefixes and matches the internal id first', async () => {
    const calls: string[] = [];
    const found = await findSyncJobsByIdInput(
      listJobsOf({'id:100': [job(100, 'p100')]}, calls),
      '#100',
    );
    expect(found.map((item) => item.id)).toEqual([100]);
    expect(calls).toContain('id:100');
  });

  it('collects platform and engine hits deduped by job id', async () => {
    const target = job(98, '179519364461900000');
    const found = await findSyncJobsByIdInput(
      listJobsOf({
        'platform:179519364461900000': [target],
        'engine:179519364461900000': [target],
      }),
      '179519364461900000',
    );
    expect(found).toHaveLength(1);
    expect(found[0].id).toBe(98);
  });

  it('firstOnly stops after the first hit', async () => {
    const calls: string[] = [];
    await findSyncJobsByIdInput(
      listJobsOf({'id:14': [job(14)]}, calls),
      '14',
      {firstOnly: true},
    );
    expect(calls).toEqual(['id:14']);
  });

  it('returns [] for non-numeric input without hitting the backend', async () => {
    const calls: string[] = [];
    const found = await findSyncJobsByIdInput(listJobsOf({}, calls), 'abc');
    expect(found).toEqual([]);
    expect(calls).toEqual([]);
  });
});

describe('resolveTaskAccessLevel', () => {
  it('resolves the three states and keeps backend-compatible fallbacks', () => {
    expect(resolveTaskAccessLevel(undefined)).toBe('public_readonly');
    expect(resolveTaskAccessLevel({})).toBe('public_readonly');
    expect(
      resolveTaskAccessLevel({access_level: 'public_editable'}),
    ).toBe('public_editable');
    expect(resolveTaskAccessLevel({access_level: 'private'})).toBe('private');
    // 顶层字段优先于 definition；脏值回退 is_public 推导。
    // Top-level wins over definition; dirty values fall back to the is_public derivation.
    expect(
      resolveTaskAccessLevel({
        access_level: 'private',
        definition: {access_level: 'public_editable'},
      }),
    ).toBe('private');
    expect(
      resolveTaskAccessLevel({access_level: 'garbage', is_public: false}),
    ).toBe('private');
    expect(
      resolveTaskAccessLevel({definition: {access_level: 'public_editable'}}),
    ).toBe('public_editable');
    expect(resolveTaskAccessLevel({is_public: false})).toBe('private');
    expect(
      resolveTaskAccessLevel({definition: {is_public: 'false'}}),
    ).toBe('private');
    expect(resolveTaskAccessLevel({is_public: true})).toBe('public_readonly');
  });
});

describe('patchTreeNode permission fields', () => {
  const baseNode = {
    id: 7,
    parent_id: 1,
    node_type: 'file' as const,
    name: 'task.conf',
    description: '',
    cluster_id: 1,
    engine_version: '2.3.8',
    mode: 'batch' as const,
    status: 'draft' as const,
    content_format: 'hocon' as const,
    content: '',
    job_name: 'task',
    definition: {},
    sort_order: 0,
    current_version: 1,
    can_edit: false,
    can_run: false,
    is_owner: false,
    is_collaborator: false,
    is_public: true,
    access_level: 'public_readonly' as const,
  };

  it('syncs permission decoration when the task payload provides it', () => {
    const task = {
      ...baseNode,
      can_edit: true,
      can_run: true,
      is_owner: false,
      is_collaborator: true,
      access_level: 'public_editable' as const,
      created_by: 1,
      created_at: '',
      updated_at: '',
    };
    const [patched] = patchTreeNode([baseNode], task);
    expect(patched.can_edit).toBe(true);
    expect(patched.can_run).toBe(true);
    expect(patched.is_collaborator).toBe(true);
    expect(patched.access_level).toBe('public_editable');
  });

  it('keeps existing permission fields when the payload omits them', () => {
    const task = {
      id: 7,
      name: 'task.conf',
    } as never;
    const [patched] = patchTreeNode([baseNode], task);
    expect(patched.can_edit).toBe(false);
    expect(patched.access_level).toBe('public_readonly');
  });
});

describe('buildTaskPayloadFromEditor access_level', () => {
  it('dual-writes access_level and a derived is_public', () => {
    const payload = buildTaskPayloadFromEditor(
      {
        ...EMPTY_EDITOR,
        name: 'task.conf',
        accessLevel: 'public_editable',
      },
      [],
    );
    expect(payload.definition?.access_level).toBe('public_editable');
    expect(payload.definition?.is_public).toBe(true);
  });

  it('maps private to is_public false', () => {
    const payload = buildTaskPayloadFromEditor(
      {...EMPTY_EDITOR, name: 'task.conf', accessLevel: 'private'},
      [],
    );
    expect(payload.definition?.access_level).toBe('private');
    expect(payload.definition?.is_public).toBe(false);
  });

  it('preserves private legacy editor state when accessLevel is omitted', () => {
    const payload = buildTaskPayloadFromEditor(
      {
        ...EMPTY_EDITOR,
        name: 'legacy-private.conf',
        accessLevel: undefined,
        isPublic: false,
      },
      [],
    );
    expect(payload.definition?.access_level).toBe('private');
    expect(payload.definition?.is_public).toBe(false);
  });
});

describe('studio responsive layout engine', () => {
  const baseInput: StudioEffectiveLayoutInput = {
    containerWidth: 1440,
    containerHeight: 800,
    leftCollapsed: false,
    rightCollapsed: false,
    leftWidth: 240,
    rightWidth: 360,
    consoleHeight: 260,
    consoleMaximized: false,
  };

  it('falls back to wide on SSR / unmeasured first frame (width <= 0)', () => {
    expect(resolveStudioViewportMode(0)).toBe('wide');
    expect(resolveStudioViewportMode(-10)).toBe('wide');
    expect(resolveStudioTopbarTier(0)).toBe('wide');
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 0,
      containerHeight: 0,
    });
    expect(layout.viewportMode).toBe('wide');
    expect(layout.topbarTier).toBe('wide');
    expect(layout.overlaySidebars).toBe(false);
    expect(layout.effectiveLeftWidth).toBe(240);
    expect(layout.effectiveRightWidth).toBe(360);
    expect(layout.maxLeftWidth).toBe(STUDIO_LEFT_WIDTH_MAX);
    expect(layout.maxRightWidth).toBe(STUDIO_RIGHT_WIDTH_MAX);
    expect(layout.gridTemplateColumns).toBe('240px minmax(0,1fr) 360px');
    expect(layout.gridTemplateRows).toBe('minmax(0,1fr) 260px');
  });

  it('resolves viewport mode and topbar tier at every breakpoint', () => {
    const cases: Array<{
      width: number;
      mode: 'wide' | 'medium' | 'compact';
      tier: 'wide' | 'medium' | 'compact';
    }> = [
      {width: 1440, mode: 'wide', tier: 'wide'},
      {width: 1280, mode: 'wide', tier: 'wide'},
      {width: 1279, mode: 'medium', tier: 'wide'},
      {width: 1100, mode: 'medium', tier: 'medium'},
      {width: 960, mode: 'medium', tier: 'medium'},
      {width: 959, mode: 'compact', tier: 'medium'},
      {width: 899, mode: 'compact', tier: 'compact'},
      {width: 800, mode: 'compact', tier: 'compact'},
      {width: 480, mode: 'compact', tier: 'compact'},
    ];
    for (const {width, mode, tier} of cases) {
      expect(resolveStudioViewportMode(width), `mode @${width}`).toBe(mode);
      expect(resolveStudioTopbarTier(width), `tier @${width}`).toBe(tier);
    }
    expect(resolveStudioTopbarTier(1199)).toBe('medium');
    expect(resolveStudioTopbarTier(1200)).toBe('wide');
  });

  it('keeps both sidebars docked in wide and leaves the center above 420px', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 1440,
    });
    expect(layout.viewportMode).toBe('wide');
    expect(layout.dockedLeftCollapsed).toBe(false);
    expect(layout.dockedRightCollapsed).toBe(false);
    expect(layout.gridTemplateColumns).toBe('240px minmax(0,1fr) 360px');
    expect(layout.maxLeftWidth).toBe(STUDIO_LEFT_WIDTH_MAX);
    expect(layout.maxRightWidth).toBe(STUDIO_RIGHT_WIDTH_MAX);
  });

  it('compresses dual sidebars proportionally when they exceed the center budget', () => {
    // 预算 1280-420=860 < 420+560：两侧按下限之上的超出占比分摊 400px 余量。
    // Budget 1280-420=860 < 420+560: the 400px slack over the floors is split by
    // overshoot share.
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 1280,
      leftWidth: 420,
      rightWidth: 560,
    });
    expect(layout.effectiveLeftWidth).toBe(365);
    expect(layout.effectiveRightWidth).toBe(495);
    expect(layout.effectiveLeftWidth + layout.effectiveRightWidth).toBe(860);
    expect(
      1280 - layout.effectiveLeftWidth - layout.effectiveRightWidth,
    ).toBeGreaterThanOrEqual(STUDIO_CENTER_MIN_WIDTH);
    // 拖拽上限联动：左栏最多再吃到 budget-右栏宽。 / Drag ceilings interlock.
    expect(layout.maxLeftWidth).toBe(365);
    expect(layout.maxRightWidth).toBe(495);
    expect(layout.gridTemplateColumns).toBe('365px minmax(0,1fr) 495px');
  });

  it('clamps a single docked sidebar to the budget before touching the center', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 960,
      rightCollapsed: true,
      leftWidth: 420,
    });
    expect(layout.viewportMode).toBe('medium');
    expect(layout.maxLeftWidth).toBe(420);
    expect(layout.effectiveLeftWidth).toBe(420);
    // 中栏仍保 420px / Center still keeps 420px
    expect(960 - layout.effectiveLeftWidth).toBeGreaterThanOrEqual(
      STUDIO_CENTER_MIN_WIDTH,
    );
  });

  it('docks only the left sidebar when both are open in medium', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 1100,
    });
    expect(layout.viewportMode).toBe('medium');
    expect(layout.dockedLeftCollapsed).toBe(false);
    expect(layout.dockedRightCollapsed).toBe(true);
    expect(layout.gridTemplateColumns).toBe('240px minmax(0,1fr) 0px');
  });

  it('docks only the right sidebar when the left is collapsed in medium', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerWidth: 1100,
      leftCollapsed: true,
    });
    expect(layout.dockedLeftCollapsed).toBe(true);
    expect(layout.dockedRightCollapsed).toBe(false);
    // 预算 1100-420=680 > 右栏上限 560，保留用户偏好宽。
    // Budget 1100-420=680 > right max 560, so the preferred width stands.
    expect(layout.effectiveRightWidth).toBe(360);
    expect(layout.maxRightWidth).toBe(560);
  });

  it('switches to overlay drawers and a full-width center in compact', () => {
    for (const width of [800, 480]) {
      const layout = resolveEffectiveStudioLayout({
        ...baseInput,
        containerWidth: width,
      });
      expect(layout.viewportMode).toBe('compact');
      expect(layout.overlaySidebars).toBe(true);
      expect(layout.dockedLeftCollapsed).toBe(true);
      expect(layout.dockedRightCollapsed).toBe(true);
      expect(layout.gridTemplateColumns).toBe('0px minmax(0,1fr) 0px');
    }
  });

  it('clamps the console to leave at least 180px for the editor on short containers', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 600,
      consoleHeight: 560,
    });
    expect(layout.maxConsoleHeight).toBe(420);
    expect(layout.effectiveConsoleHeight).toBe(420);
    expect(layout.gridTemplateRows).toBe('minmax(0,1fr) 420px');
    // 偏好在限内时不被改写。 / A preference inside the limit is kept verbatim.
    const kept = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 600,
      consoleHeight: 260,
    });
    expect(kept.effectiveConsoleHeight).toBe(260);
    expect(kept.maxConsoleHeight).toBe(420);
  });

  it('still leaves the editor 180px when the console just fits', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 500,
      consoleHeight: 390,
    });
    expect(layout.maxConsoleHeight).toBe(320);
    expect(layout.effectiveConsoleHeight).toBe(320);
    expect(500 - layout.effectiveConsoleHeight).toBeGreaterThanOrEqual(
      STUDIO_EDITOR_MIN_HEIGHT,
    );
  });

  it('shares an extremely short container proportionally instead of starving the console', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 300,
      consoleHeight: 260,
    });
    expect(layout.maxConsoleHeight).toBe(135);
    expect(layout.effectiveConsoleHeight).toBe(135);
    expect(layout.gridTemplateRows).toBe('minmax(0,1fr) 135px');
    const floor = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 200,
      consoleHeight: 260,
    });
    expect(floor.maxConsoleHeight).toBe(110);
    expect(floor.effectiveConsoleHeight).toBe(110);
  });

  it('keeps the console default max and full clamp when height is unmeasured', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 0,
      consoleHeight: 900,
    });
    expect(layout.maxConsoleHeight).toBe(STUDIO_CONSOLE_HEIGHT_MAX);
    expect(layout.effectiveConsoleHeight).toBe(560);
  });

  it('lets the console take the grid when maximized regardless of height', () => {
    const layout = resolveEffectiveStudioLayout({
      ...baseInput,
      containerHeight: 300,
      consoleMaximized: true,
    });
    expect(layout.gridTemplateRows).toBe('0px minmax(0,1fr)');
  });
});
