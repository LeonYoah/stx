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

import {readFileSync} from 'node:fs';
import path from 'node:path';
import {describe, expect, it} from 'vitest';

describe('onboarding-checklist hide layout CSS contract', () => {
  const css = readFileSync(
    path.join(__dirname, '../onboarding-checklist.module.css'),
    'utf8',
  );

  it('keeps header sticky and uses headerActions without .hide width clamp on the cluster', () => {
    expect(css.includes('position: sticky')).toBe(true);
    expect(css.includes('.headerActions')).toBe(true);
    expect(css.includes('min-width: var(--control-height-sm)')).toBe(true);
    // Narrow @container may clamp each .hide control; headerActions must exist separately.
    // 窄 @container 可钳单个 .hide；headerActions 必须独立存在。
    expect(css.includes('@container (max-width: 380px)')).toBe(true);
  });
});
