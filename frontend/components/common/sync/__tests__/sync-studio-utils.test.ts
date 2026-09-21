import {describe, expect, it} from 'vitest';
import {
  isCursorInsideValueRegion,
  resolveEnumSuggestionItems,
  resolveEnumValueBounds,
  resolveOptionAssignmentContext,
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
});
