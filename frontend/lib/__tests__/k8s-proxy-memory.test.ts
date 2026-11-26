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
import {deriveProxyContainerMemory} from '../k8s-proxy-memory';

describe('deriveProxyContainerMemory 与后端规格示例一致', () => {
  const cases: Array<[string, number, number]> = [
    ['-Xmx512m', 1024, 256],
    ['-Xmx1024m', 1536, 256],
    ['-Xmx1024m -Xms128m', 1536, 320],
    ['-Xmx2048m -Xms256m', 2560, 448],
    ['-Xmx4096m', 5120, 256],
    ['-Xmx6g', 7680, 256],
    ['', 1024, 256],
    ['-Xmx1G', 1536, 256],
    ['-Xmx512m -Xmx1024m', 1536, 256],
    ['-Xmxabc', 1024, 256],
  ];
  it.each(cases)('jvmOpts %s → limit %i Mi / req %i Mi', (opts, wantLimit, wantReq) => {
    const res = deriveProxyContainerMemory(opts);
    expect(res.limitMi).toBe(wantLimit);
    expect(res.requestMi).toBe(wantReq);
  });

  it('整 Gi 显示为 GiB，其余 MiB', () => {
    expect(deriveProxyContainerMemory('-Xmx512m').limitDisplay).toBe('1 GiB');
    expect(deriveProxyContainerMemory('-Xmx1024m').limitDisplay).toBe('1536 MiB');
  });
});
