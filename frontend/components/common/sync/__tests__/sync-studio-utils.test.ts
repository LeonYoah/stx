import {describe, expect, it} from 'vitest';
import {
  isCursorInsideValueRegion,
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
});
