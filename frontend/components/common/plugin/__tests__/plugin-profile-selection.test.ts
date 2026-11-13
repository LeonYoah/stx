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
  mergeArtifactProfilesIntoMemory,
  normalizeProfileKeys,
  pluginRequiresProfileSelection,
  resolveInstallProfileKeys,
} from '../plugin-profile-selection';

describe('plugin-profile-selection', () => {
  it('normalizes, dedupes, and sorts profile keys', () => {
    expect(normalizeProfileKeys([' mysql ', 'postgresql', 'mysql', ''])).toEqual(
      ['mysql', 'postgresql'],
    );
  });

  it('detects JDBC plugins that require profile selection', () => {
    expect(
      pluginRequiresProfileSelection({
        name: 'jdbc',
        artifact_id: 'connector-jdbc',
      }),
    ).toBe(true);
    expect(
      pluginRequiresProfileSelection({
        name: 'kafka',
        artifact_id: 'connector-kafka',
      }),
    ).toBe(false);
  });

  it('prefers artifact profiles over in-memory picks for install prefill', () => {
    expect(
      resolveInstallProfileKeys({
        artifactKeys: ['mysql'],
        memoryKeys: ['postgresql', 'oracle'],
      }),
    ).toEqual(['mysql']);
    expect(
      resolveInstallProfileKeys({
        artifactKeys: [],
        memoryKeys: ['postgresql'],
      }),
    ).toEqual(['postgresql']);
    expect(
      resolveInstallProfileKeys({
        artifactKeys: null,
        memoryKeys: undefined,
      }),
    ).toEqual([]);
  });

  it('merges downloaded artifact profiles into memory map', () => {
    const merged = mergeArtifactProfilesIntoMemory(
      {kafka: ['n/a'], jdbc: ['old']},
      [
        {name: 'jdbc', selected_profile_keys: ['mysql', 'postgresql']},
        {name: 'kafka', selected_profile_keys: []},
      ],
    );
    expect(merged.jdbc).toEqual(['mysql', 'postgresql']);
    expect(merged.kafka).toEqual(['n/a']);
  });
});
