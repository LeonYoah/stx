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
import type {NodeJavaRuntimeView} from '@/components/common/java/NodeJavaHomeCell';
import {
  mergeJavaRuntimeViews,
  normalizeLoadClusterDataOptions,
  selectNodesNeedingJavaRuntime,
} from '../node-java-runtime-refresh';

describe('normalizeLoadClusterDataOptions', () => {
  it('keeps notifyError true by default even when silent', () => {
    expect(normalizeLoadClusterDataOptions({silent: true})).toEqual({
      silent: true,
      notifyError: true,
      javaRuntimeMode: 'full',
    });
  });

  it('allows poll/event linkage to disable notifyError explicitly', () => {
    expect(
      normalizeLoadClusterDataOptions({
        silent: true,
        notifyError: false,
        javaRuntimeMode: 'skip',
      }),
    ).toEqual({
      silent: true,
      notifyError: false,
      javaRuntimeMode: 'skip',
    });
  });
});

describe('selectNodesNeedingJavaRuntime / mergeJavaRuntimeViews', () => {
  const ok: NodeJavaRuntimeView = {
    status: 'ok',
    version: 11,
    version_string: '11.0.11',
    java_home: '/jdk',
  };

  it('full mode marks every node for refetch', () => {
    expect(
      selectNodesNeedingJavaRuntime(
        [{id: 1, process_pid: 10}],
        [{id: 1, process_pid: 10}],
        {1: ok},
        'full',
      ),
    ).toEqual([1]);
  });

  it('incremental keeps ok rows when pid is unchanged', () => {
    const merged = mergeJavaRuntimeViews(
      [
        {id: 1, process_pid: 10},
        {id: 2, process_pid: 20},
      ],
      [{id: 1, process_pid: 10}],
      {1: ok},
      'incremental',
    );
    expect(merged[1]).toEqual(ok);
    expect(merged[2]).toEqual({status: 'loading'});
    expect(
      selectNodesNeedingJavaRuntime(
        [
          {id: 1, process_pid: 10},
          {id: 2, process_pid: 20},
        ],
        [{id: 1, process_pid: 10}],
        {1: ok},
        'incremental',
      ),
    ).toEqual([2]);
  });

  it('incremental refetches when pid changes', () => {
    expect(
      selectNodesNeedingJavaRuntime(
        [{id: 1, process_pid: 99}],
        [{id: 1, process_pid: 10}],
        {1: ok},
        'incremental',
      ),
    ).toEqual([1]);
  });
});
