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
  TREND_COLORS,
  colorByName,
  jobLifetimeWindow,
  MAX_TREND_WINDOW_MS,
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
      {key: 'committed', label: 'Committed', value: 50, color: TREND_COLORS.committed},
      {key: 'written', label: 'Written', value: 30, color: TREND_COLORS.write},
      {key: 'backlog', label: 'Backlog', value: 20, color: TREND_COLORS.read},
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

  it('drops an even-step tick that would crowd the forced last tick', () => {
    // 101 个点、step=20：80 号离末尾 20，保留；末尾前一档若只差 1 个点则让位。
    // 101 points, step 20: index 80 sits 20 from the end and stays; a tick one point from the end yields.
    expect(pickAxisLabel(80, 101, 'a')).toBe('a');
    expect(pickAxisLabel(100, 101, 'a')).toBe('a');
    const labeled = Array.from({length: 102}, (_, i) => pickAxisLabel(i, 102, String(i))).filter(Boolean);
    expect(labeled).toContain('101');
    expect(labeled).not.toContain('100');
  });

  it('pins stage colors on lines and keeps vertex colors stable by name', () => {
    const {series} = toLineChartData(
      [
        {key: 'read', name: 'Read', points: [[1, 1]], color: TREND_COLORS.read},
        {key: 'write', name: 'Write', points: [[1, 1]]},
      ],
      label,
      label,
    );
    expect(series[0].color).toBe(TREND_COLORS.read);
    expect(series[1]).not.toHaveProperty('color');
    const source = 'pipeline-1 [Source[0]-FakeSource]';
    const sink = 'pipeline-1 [Sink[0]-Console-MultiTableSink]';
    const colors = colorByName([sink, source, 'pipeline-1 [Transform[0]-Sql]', 'queue--5→2']);
    // Source / Sink 跟随读 / 写色，字母序在前的 Sink 也不会抢走读色。
    // Source / Sink follow read / write even though "Sink" sorts first.
    expect(colors.get(source)).toBe(TREND_COLORS.read);
    expect(colors.get(sink)).toBe(TREND_COLORS.write);
    expect(colors.get('pipeline-1 [Transform[0]-Sql]')).not.toBe(colors.get('queue--5→2'));
    // 同一批名字不论顺序都得到同样的颜色。 / The same set of names yields the same colors regardless of order.
    const reordered = colorByName(['queue--5→2', 'pipeline-1 [Transform[0]-Sql]', source, sink]);
    expect(reordered.get('queue--5→2')).toBe(colors.get('queue--5→2'));
  });

  it('derives the job lifetime window with fallbacks and the 31-day cap', () => {
    const now = Date.parse('2026-11-14T13:00:00Z');
    const start = Date.parse('2026-11-14T12:13:03Z');
    expect(jobLifetimeWindow({started_at: '2026-11-14T12:13:03Z', finished_at: '2026-11-14T12:38:28Z'}, false, now))
      .toEqual({start, end: Date.parse('2026-11-14T12:38:28Z')});
    // 结束早于开始（引擎时区错位）：退到 updated_at。 / Finish before start (engine tz skew): fall back to updated_at.
    expect(jobLifetimeWindow({
      started_at: '2026-11-14T12:13:03Z',
      finished_at: '2026-11-14T04:38:28Z',
      updated_at: '2026-11-14T12:39:44Z',
    }, false, now)).toEqual({start, end: Date.parse('2026-11-14T12:39:44Z')});
    expect(jobLifetimeWindow({started_at: '2026-11-14T12:13:03Z'}, true, now)).toEqual({start, end: now});
    const long = jobLifetimeWindow({started_at: '2026-08-01T00:00:00Z'}, true, now)!;
    expect(long.end - long.start).toBe(MAX_TREND_WINDOW_MS);
    expect(jobLifetimeWindow({}, false, now)).toBeNull();
    expect(jobLifetimeWindow(null, false, now)).toBeNull();
  });
});
