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

import {afterEach, describe, expect, it} from 'vitest';
import {
  clearPreferredClusterId,
  fillPreferredClusterId,
  isSoleDefaultCluster,
  rememberPreferredClusterId,
  resolvePreferredClusterId,
} from '@/lib/cluster-preference';

describe('cluster-preference', () => {
  afterEach(() => {
    clearPreferredClusterId();
  });

  it('treats the sole cluster as default', () => {
    expect(resolvePreferredClusterId([{id: 7}])).toBe(7);
    expect(isSoleDefaultCluster([{id: 7}], 7)).toBe(true);
    expect(isSoleDefaultCluster([{id: 7}], 7, 1)).toBe(true);
  });

  it('uses remembered preference when multiple clusters exist', () => {
    rememberPreferredClusterId(3);
    expect(
      resolvePreferredClusterId([{id: 1}, {id: 3}, {id: 5}]),
    ).toBe(3);
  });

  it('ignores stale preference and optionally falls back to first', () => {
    rememberPreferredClusterId(99);
    expect(resolvePreferredClusterId([{id: 1}, {id: 2}])).toBeNull();
    expect(
      resolvePreferredClusterId([{id: 1}, {id: 2}], {fallbackToFirst: true}),
    ).toBe(1);
  });

  it('fills empty selection with preferred id', () => {
    expect(fillPreferredClusterId('', [{id: 9}])).toBe('9');
    expect(fillPreferredClusterId('4', [{id: 9}])).toBe('4');
  });
});
