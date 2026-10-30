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

import {HELP_HINT_DELAY_MS} from '../HelpHint';

// HelpHint 本体与测试和 #89 保持一致；延迟约束单独放在这里，避免两条分支改同一文件
// Kept separate from HelpHint.test.tsx so that file stays identical to the P2 branch.
describe('HelpHint delay', () => {
  it('does not use a zero hover delay', () => {
    expect(HELP_HINT_DELAY_MS).toBeGreaterThan(0);
  });
});
