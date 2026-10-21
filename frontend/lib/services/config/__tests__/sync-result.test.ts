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
import {summarizeSyncResult} from '../types';

describe('summarizeSyncResult', () => {
  it('reports partial sync with pending nodes even when nothing was committed', () => {
    const summary = summarizeSyncResult({
      synced_count: 0,
      pending_count: 1,
      node_results: [
        {config_id: 1, host_id: 5, host_ip: '10.0.0.5', status: 'pending_push', version: 1},
        {config_id: 2, host_id: 6, host_ip: '10.0.0.6', status: 'unchanged', version: 2},
      ],
      push_errors: [{host_id: 5, host_ip: '10.0.0.5', message: 'agent not connected'}],
    });
    expect(summary).toEqual({kind: 'partial', count: 0, nodes: '10.0.0.5'});
  });

  it('falls back to push_errors for older responses', () => {
    const summary = summarizeSyncResult({synced_count: 2, push_errors: [{host_id: 9, message: 'x'}]});
    expect(summary.kind).toBe('partial');
    expect(summary.nodes).toBe('Host 9');
  });

  it('distinguishes success from no changes', () => {
    expect(summarizeSyncResult({synced_count: 3, pending_count: 0, push_errors: []}).kind).toBe('success');
    expect(summarizeSyncResult({synced_count: 0, pending_count: 0, push_errors: []}).kind).toBe('noChanges');
    expect(summarizeSyncResult(undefined).kind).toBe('noChanges');
  });
});
