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
  brushMinSpan,
  downsamplePoints,
  latestValue,
  pickAxisLabel,
  scalePoints,
  toBarChartData,
  toBrushAnnotations,
  toLineChartData,
  toPipelineShare,
} from '../metrics-trend-data';

const label = (ts: number) => `t-${ts}`;

describe('metrics-trend-data', () => {
  it('downsamples while keeping the first and last points', () => {
    const points = Array.from({length: 10}, (_, i) => [i * 1000, i] as [number, number]);
    const sampled = downsamplePoints(points, 4);
    expect(sampled[0]).toEqual([0, 0]);
    expect(sampled[sampled.length - 1]).toEqual([9000, 9]);
    expect(sampled.length).toBeLessThanOrEqual(4);
  });

  it('aligns several series onto one line-chart axis', () => {
    const {data, series} = toLineChartData(
      [
        {key: 'read', name: 'Read', points: [[1, 2], [3, 4]]},
        {key: 'write', name: 'Write', points: [[1, 1], [2, 8]]},
      ],
      label,
      label,
    );
    expect(series.map((item) => item.key)).toEqual(['read', 'write']);
    expect(data).toHaveLength(3);
    expect(data[0].values).toEqual({read: 2, write: 1});
    expect(data[1].values).toEqual({read: 0, write: 8});
    expect(data[2].values).toEqual({read: 4, write: 0});
  });

  it('splits pipeline counts into exclusive donut slices', () => {
    expect(toPipelineShare(100, 80, 50, {
      committed: 'Committed',
      written: 'Written',
      backlog: 'Backlog',
    })).toEqual([
      {key: 'committed', label: 'Committed', value: 50},
      {key: 'written', label: 'Written', value: 30},
      {key: 'backlog', label: 'Backlog', value: 20},
    ]);
    expect(toPipelineShare(0, 0, 0, {
      committed: 'Committed',
      written: 'Written',
      backlog: 'Backlog',
    })).toEqual([]);
  });

  it('maps checkpoint points onto bars and annotations onto brush labels', () => {
    const bars = toBarChartData([[10, 1.5], [20, 2]], label, label);
    expect(bars).toEqual([
      {key: '10', label: 't-10', axisLabel: 't-10', value: 1.5},
      {key: '20', label: 't-20', axisLabel: 't-20', value: 2},
    ]);
    expect(toBrushAnnotations(
      [{ts_ms: 10, kind: 'restart', summary: 'engine restart'}],
      (kind) => kind === 'restart' ? 'Restart' : kind,
    )).toEqual([{date: 10, label: 'Restart', description: 'engine restart'}]);
  });

  it('keeps brush min span inside the series window', () => {
    const points: [number, number][] = [[0, 1], [60_000, 2]];
    expect(brushMinSpan(points, 15)).toBeLessThanOrEqual(60_000);
    expect(brushMinSpan(points, 15)).toBeGreaterThanOrEqual(1000);
    expect(latestValue([[1, 1], [2, Number.NaN], [3, 9]])).toBe(9);
    expect(scalePoints([[1, 0.25]], 100)).toEqual([[1, 25]]);
    expect(pickAxisLabel(0, 20, 'a')).toBe('a');
    expect(pickAxisLabel(1, 20, 'a')).toBeUndefined();
  });
});
