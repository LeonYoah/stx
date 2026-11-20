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
  colorForLabels,
  dimensionColor,
  formatCompactNumber,
  formatDurationSeconds,
  scaledCompact,
  summarizeRate,
  trendChangePercent,
} from '../dashboard-format';

describe('dashboard-format', () => {
  it('formats compact numbers and placeholders', () => {
    expect(formatCompactNumber(12345)).toBe('12.3K');
    expect(formatCompactNumber(undefined)).toBe('-');
    expect(formatCompactNumber(Number.NaN)).toBe('-');
  });

  it('scales large numbers into ≤999.9 plus a unit, keeping KPI cards narrow', () => {
    // 小于 1000 原样通过；向上按 K/M/B/T 分档，取整顶到 1000 时再进一档。
    // Below 1000 passes through; tiers go K/M/B/T, and a rounded 1000 steps up once more.
    expect(scaledCompact(999)).toEqual({value: 999, decimals: 0, unit: ''});
    expect(scaledCompact(0)).toEqual({value: 0, decimals: 0, unit: ''});
    expect(scaledCompact(27200)).toEqual({value: 27.2, decimals: 1, unit: 'K'});
    expect(scaledCompact(1_523_000)).toEqual({value: 1.5, decimals: 1, unit: 'M'});
    expect(scaledCompact(103_800_000_000)).toEqual({value: 103.8, decimals: 1, unit: 'B'});
    expect(scaledCompact(999_960_000)).toEqual({value: 1, decimals: 1, unit: 'B'});
    expect(scaledCompact(-2_500_000)).toEqual({value: -2.5, decimals: 1, unit: 'M'});
    expect(scaledCompact(Number.NaN)).toEqual({value: 0, decimals: 0, unit: ''});
    expect(scaledCompact(undefined)).toEqual({value: 0, decimals: 0, unit: ''});
  });

  it('keeps the two largest duration units', () => {
    const units = {d: 'd', h: 'h', m: 'm'};
    expect(formatDurationSeconds(2 * 86400 + 4 * 3600 + 59, units)).toBe(
      '2d 4h',
    );
    expect(formatDurationSeconds(3 * 3600 + 12 * 60, units)).toBe('3h 12m');
    expect(formatDurationSeconds(30, units)).toBe('<1m');
  });

  it('summarizes a rate series into window average and peak', () => {
    expect(summarizeRate([])).toBeNull();
    expect(
      summarizeRate([
        [0, Number.NaN],
        [1, Number.POSITIVE_INFINITY],
      ]),
    ).toBeNull();
    // NaN / Infinity 的点被忽略，不参与均值。/ NaN and Infinity points are skipped.
    expect(
      summarizeRate([
        [0, 10],
        [1, Number.NaN],
        [2, 30],
        [3, 20],
      ]),
    ).toEqual({avg: 20, peak: 30});
    expect(summarizeRate([[0, 0]])).toEqual({avg: 0, peak: 0});
  });

  it('computes the write-rate change against the first sample', () => {
    const hour = 60 * 60 * 1000;
    expect(
      trendChangePercent([
        [0, 100],
        [hour, 106],
      ]),
    ).toBe(6);
    expect(
      trendChangePercent([
        [0, 100],
        [hour, 50],
      ]),
    ).toBe(-50);
    expect(
      trendChangePercent([
        [0, 0],
        [hour, 50],
      ]),
    ).toBeNull();
    expect(trendChangePercent([[0, 10]])).toBeNull();
    // 作业刚启动 1 分钟：不能说成「较 1 小时前」。/ A job one minute old must not read as "vs. 1 hour ago".
    expect(
      trendChangePercent([
        [0, 75],
        [60_000, 204],
      ]),
    ).toBeNull();
  });
});

describe('dimension colors', () => {
  it('maps semantic dimensions to their token colors', () => {
    // 读写 / 收发与 CPU 模式是大盘全局语义色。/ Read-write / rx-tx and CPU modes are
    // dashboard-wide semantics.
    expect(dimensionColor('rx eth0')).toBe('var(--series-1)');
    expect(dimensionColor('write')).toBe('var(--series-2)');
    expect(dimensionColor('iowait')).toBe('var(--warning)');
    expect(dimensionColor('steal')).toBe('var(--danger)');
    expect(dimensionColor('BLOCKED')).toBe('var(--danger)');
    // JVM 池名走规则匹配。/ JVM pool names resolve through rules.
    expect(dimensionColor('G1 Eden Space')).toBe(dimensionColor('PS Eden Space'));
    // 未知维度返回 undefined，交给色板兜底。/ Unknown dims defer to the palette.
    expect(dimensionColor('hz:executor:whatever')).toBeUndefined();
  });

  it('keeps one color per dimension and fades same-semantic siblings', () => {
    const colors = colorForLabels([
      'rx eth0',
      'rx eth1',
      'tx eth0',
      'weird-device',
    ]);
    // 同语义多实例：首个满色，后续调淡但同一方向。/ Same-semantic siblings: first full
    // color, rest faded on the same hue.
    expect(colors.get('rx eth0')).toBe('var(--series-1)');
    expect(colors.get('rx eth1')).toContain('var(--series-1)');
    expect(colors.get('rx eth1')).not.toBe('var(--series-1)');
    expect(colors.get('tx eth0')).toBe('var(--series-2)');
    // 未知名走稳定色板：同名同色、与语义色不同。/ Unknowns take the stable palette:
    // same name same color, distinct from semantic hues.
    expect(colors.get('weird-device')).toBeDefined();
    expect(colors.get('weird-device')).toBe(
      colorForLabels(['weird-device']).get('weird-device'),
    );
  });
});
