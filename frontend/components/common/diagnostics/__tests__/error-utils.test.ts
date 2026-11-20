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
  formatNodeOrigin,
  heatFilterToOccurrenceRange,
  occurrenceHeatTone,
  resolveGroupClusterLabel,
} from '../errors/error-utils';

describe('error-utils', () => {
  it('maps heat filters onto the server-side closed occurrence range', () => {
    expect(heatFilterToOccurrenceRange('all')).toEqual({});
    expect(heatFilterToOccurrenceRange('high')).toEqual({min_occurrences: 10});
    expect(heatFilterToOccurrenceRange('mid')).toEqual({
      min_occurrences: 3,
      max_occurrences: 9,
    });
    expect(heatFilterToOccurrenceRange('low')).toEqual({max_occurrences: 2});
  });

  it('picks heat tones by the same thresholds as the old panel', () => {
    expect(occurrenceHeatTone(10)).toBe('rose');
    expect(occurrenceHeatTone(30)).toBe('rose');
    expect(occurrenceHeatTone(3)).toBe('amber');
    expect(occurrenceHeatTone(9)).toBe('amber');
    expect(occurrenceHeatTone(2)).toBe('muted');
    expect(occurrenceHeatTone(0)).toBe('muted');
  });

  it('joins node origin parts with the host name preferred', () => {
    expect(
      formatNodeOrigin({
        hostName: 'worker-a',
        hostIp: '10.0.0.1',
        role: 'worker',
        nodeId: 11,
      }),
    ).toBe('worker-a · worker · node #11');
    expect(formatNodeOrigin({hostIp: '10.0.0.2'})).toBe('10.0.0.2');
    expect(formatNodeOrigin({hostId: 7})).toBe('#7');
    expect(formatNodeOrigin({})).toBe('-');
  });

  it('resolves the owning cluster name by last_cluster_id', () => {
    const options = [
      {cluster_id: 1, cluster_name: 'stx-prod'},
      {cluster_id: 2, cluster_name: 'stx-dev'},
    ];
    expect(resolveGroupClusterLabel(1, options)).toBe('stx-prod');
    expect(resolveGroupClusterLabel(9, options)).toBe('#9');
    expect(resolveGroupClusterLabel(0, options)).toBe('-');
  });
});
