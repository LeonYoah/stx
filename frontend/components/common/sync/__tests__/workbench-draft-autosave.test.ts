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
  buildTaskPayloadFromEditor,
  EMPTY_EDITOR,
  hasWorkbenchUnsavedRisk,
  isLeavingWorkbenchPath,
  type EditorDraftState,
  type EditorState,
} from '../sync-studio-utils';

function makeEditor(overrides: Partial<EditorState> = {}): EditorState {
  return {
    ...EMPTY_EDITOR,
    id: 1,
    name: 'demo',
    content: 'env {}',
    canEdit: true,
    ...overrides,
  };
}

function makeDraft(
  overrides: Partial<EditorDraftState> & {editor?: EditorState} = {},
): EditorDraftState {
  const editor = overrides.editor || makeEditor();
  return {
    editor,
    customVariableRows: [],
    dirty: true,
    baselineEditor: makeEditor({content: ''}),
    baselineCustomVariableRows: [],
    ...overrides,
  };
}

describe('workbench draft save helpers', () => {
  it('buildTaskPayloadFromEditor maps editor fields without publishing flags', () => {
    const payload = buildTaskPayloadFromEditor(
      makeEditor({
        parentId: 9,
        name: ' orders ',
        description: 'd',
        clusterId: '3',
        content: 'source {}',
        definition: {execution_mode: 'local'},
      }),
      [{id: '1', key: 'k', value: 'v', type: 'string'}],
    );
    expect(payload.parent_id).toBe(9);
    expect(payload.name).toBe('orders');
    expect(payload.cluster_id).toBe(3);
    expect(payload.content).toBe('source {}');
    expect(payload.definition?.custom_variables).toEqual({k: 'v'});
    expect(payload.definition?.execution_mode).toBe('local');
    expect(payload).not.toHaveProperty('publish');
  });

  it('hasWorkbenchUnsavedRisk ignores read-only dirty drafts', () => {
    expect(
      hasWorkbenchUnsavedRisk({
        drafts: {
          1: makeDraft({
            editor: makeEditor({canEdit: false}),
            dirty: true,
          }),
        },
        saveStatus: 'idle',
      }),
    ).toBe(false);
  });

  it('hasWorkbenchUnsavedRisk is true for editable dirty or save error/saving', () => {
    expect(
      hasWorkbenchUnsavedRisk({
        drafts: {1: makeDraft()},
        saveStatus: 'idle',
      }),
    ).toBe(true);
    expect(
      hasWorkbenchUnsavedRisk({
        drafts: {},
        saveStatus: 'saving',
      }),
    ).toBe(true);
    expect(
      hasWorkbenchUnsavedRisk({
        drafts: {},
        saveStatus: 'error',
      }),
    ).toBe(true);
    expect(
      hasWorkbenchUnsavedRisk({
        drafts: {
          1: makeDraft({dirty: false}),
        },
        saveStatus: 'saved',
      }),
    ).toBe(false);
  });

  it('isLeavingWorkbenchPath detects dock navigations away from /workbench', () => {
    expect(isLeavingWorkbenchPath('/dashboard', '/workbench')).toBe(true);
    expect(isLeavingWorkbenchPath('/clusters', '/workbench')).toBe(true);
    expect(isLeavingWorkbenchPath('/workbench', '/workbench')).toBe(false);
    expect(isLeavingWorkbenchPath('/workbench?task=1', '/workbench')).toBe(
      false,
    );
    expect(isLeavingWorkbenchPath('#section', '/workbench')).toBe(false);
  });
});
