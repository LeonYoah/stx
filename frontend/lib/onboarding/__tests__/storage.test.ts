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
import {onboardingStorageKey, parseOnboardingState} from '../storage';

describe('onboarding storage migration', () => {
  it('scopes keys by user id', () => {
    expect(onboardingStorageKey(7)).toBe('stx:onboarding:v1:7');
  });

  it('drops legacy plan fields so they cannot lock or false-complete', () => {
    const migrated = parseOnboardingState(
      JSON.stringify({
        version: 1,
        path: 'new',
        displayMode: 'collapsed',
        targetClusterId: 3,
        plannedVersion: '2.3.12',
        plannedPlugins: [{name: 'jdbc', profileKeys: ['mysql']}],
        plannedHostIds: [1, 2],
        skipPluginPreinstall: true,
        resourcesPlanFingerprint: '{"version":"2.3.12"}',
        proxyAcknowledged: true,
        finished: false,
        completedStepIds: ['resources', 'proxy'],
      }),
    );
    expect(migrated.version).toBe(2);
    expect(migrated.path).toBe('new');
    expect(migrated.displayMode).toBe('collapsed');
    expect(migrated.targetClusterId).toBe(3);
    expect(migrated.completedStepIds).toEqual(['resources']);
    expect(migrated).not.toHaveProperty('plannedVersion');
    expect(migrated).not.toHaveProperty('plannedPlugins');
    expect(migrated).not.toHaveProperty('proxyAcknowledged');
  });

  it('preserves three-state displayMode across parse', () => {
    for (const mode of ['expanded', 'collapsed', 'hidden'] as const) {
      expect(
        parseOnboardingState(
          JSON.stringify({version: 2, path: 'existing', displayMode: mode}),
        ).displayMode,
      ).toBe(mode);
    }
  });
});
