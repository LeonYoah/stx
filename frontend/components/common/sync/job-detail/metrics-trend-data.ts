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

import type {BarChartDatum} from '@/components/arc/bar-chart/bar-chart';
import type {BrushChartAnnotation, BrushChartDatum} from '@/components/arc/brush-chart/brush-chart';
import type {DonutChartDatum} from '@/components/arc/donut-chart/donut-chart';
import type {LineChartDatum, LineChartSeries} from '@/components/arc/line-chart/line-chart';
import type {SyncJobTimeseriesAnnotation} from '@/lib/services/sync';

export const MAX_LINE_POINTS = 240;
export const MAX_BAR_POINTS = 36;

export type TrendPoint = [number, number];

export type NamedSeries = {
  key: string;
  name: string;
  points: TrendPoint[];
};

/**
 * 等间隔抽稀，始终保留首尾点，避免 LineChart / BarChart 被过密点拖慢。
 * Evenly downsample while keeping the first and last points so LineChart / BarChart stay responsive.
 */
export function downsamplePoints(points: TrendPoint[], max: number): TrendPoint[] {
  if (max < 2 || points.length <= max) {
    return points;
  }
  const step = (points.length - 1) / (max - 1);
  const out: TrendPoint[] = [];
  let last = -1;
  for (let i = 0; i < max; i += 1) {
    const index = Math.round(i * step);
    if (index === last) {
      continue;
    }
    last = index;
    out.push(points[index]);
  }
  return out;
}

/**
 * 取序列最后一个有限值；空序列为 0。
 * Latest finite value in a series; 0 when empty.
 */
export function latestValue(points: TrendPoint[]): number {
  for (let i = points.length - 1; i >= 0; i -= 1) {
    const value = points[i][1];
    if (Number.isFinite(value)) {
      return value;
    }
  }
  return 0;
}

/**
 * 只在首尾和均匀间隔点上留 axisLabel，让 Arc 轴标签保持稀疏。
 * Keep axisLabel on the ends and even steps so Arc axis ticks stay sparse.
 */
export function pickAxisLabel(index: number, total: number, label: string): string | undefined {
  if (total <= 6) {
    return label;
  }
  const step = Math.max(1, Math.round((total - 1) / 5));
  if (index === 0 || index === total - 1 || index % step === 0) {
    return label;
  }
  return undefined;
}

/**
 * 多序列对齐到同一时间轴，缺测点按 0（Arc LineChart 契约）。
 * Align several series onto one time axis; missing samples become 0 (Arc LineChart contract).
 */
export function toLineChartData(
  series: NamedSeries[],
  formatLabel: (ts: number) => string,
  formatAxis: (ts: number) => string,
  maxPoints = MAX_LINE_POINTS,
): {data: LineChartDatum[]; series: LineChartSeries[]} {
  const lineSeries: LineChartSeries[] = series.map((item, index) => ({
    key: item.key,
    label: item.name,
    area: index === 0,
  }));
  const tsSet = new Set<number>();
  const lookup = series.map((item) => {
    const map = new Map<number, number>();
    downsamplePoints(item.points, maxPoints).forEach(([ts, value]) => {
      tsSet.add(ts);
      map.set(ts, value);
    });
    return map;
  });
  const timestamps = Array.from(tsSet).sort((a, b) => a - b);
  const data: LineChartDatum[] = timestamps.map((ts, index) => {
    const values: Record<string, number> = {};
    series.forEach((item, seriesIndex) => {
      values[item.key] = lookup[seriesIndex].get(ts) ?? 0;
    });
    const label = formatLabel(ts);
    return {
      key: String(ts),
      label,
      axisLabel: pickAxisLabel(index, timestamps.length, formatAxis(ts)),
      values,
    };
  });
  return {data, series: lineSeries};
}

/**
 * 单序列转 BrushChart 点。
 * Convert one series into BrushChart points.
 */
export function toBrushChartData(points: TrendPoint[]): BrushChartDatum[] {
  return points.map(([date, value]) => ({date, value}));
}

/**
 * Brush 窗口下限：约 4 个 step，且不超过整段跨度。
 * Brush min span: about four steps, never longer than the full series.
 */
export function brushMinSpan(points: TrendPoint[], stepSeconds: number): number {
  if (points.length < 2) {
    return Math.max(1_000, stepSeconds * 1000);
  }
  const span = Math.max(1, points[points.length - 1][0] - points[0][0]);
  const stepMs = stepSeconds > 0
    ? stepSeconds * 1000
    : Math.max(1_000, Math.round(span / Math.max(1, points.length - 1)));
  return Math.min(span, Math.max(stepMs * 4, 1_000));
}

/**
 * 作业事件转 Brush 标注。
 * Map job events onto BrushChart annotations.
 */
export function toBrushAnnotations(
  annotations: SyncJobTimeseriesAnnotation[] | undefined,
  labelOf: (kind: string) => string,
): BrushChartAnnotation[] {
  return (annotations ?? []).map((item) => ({
    date: item.ts_ms,
    label: labelOf(item.kind),
    description: item.summary || item.kind,
  }));
}

/**
 * 单序列转 BarChart；过密时抽稀。
 * Convert one series into BarChart rows, downsampling when dense.
 */
export function toBarChartData(
  points: TrendPoint[],
  formatLabel: (ts: number) => string,
  formatAxis: (ts: number) => string,
  maxPoints = MAX_BAR_POINTS,
): BarChartDatum[] {
  const sampled = downsamplePoints(points, maxPoints);
  return sampled.map(([ts, value], index) => {
    const label = formatLabel(ts);
    return {
      key: String(ts),
      label,
      axisLabel: pickAxisLabel(index, sampled.length, formatAxis(ts)),
      value,
    };
  });
}

export type PipelineShareLabels = {
  committed: string;
  written: string;
  backlog: string;
};

/**
 * 用累计读/写/提交把管道拆成互斥三段：已提交、已写未提交、已读未写。
 * Split cumulative read/write/commit into exclusive slices: committed, written, unread backlog.
 */
export function toPipelineShare(
  readCount: number,
  writeCount: number,
  committedCount: number,
  labels: PipelineShareLabels,
): DonutChartDatum[] {
  const read = Math.max(0, readCount);
  const write = Math.max(0, writeCount);
  const committed = Math.max(0, Math.min(committedCount, write, read || committedCount));
  const written = Math.max(0, Math.min(write, read || write) - committed);
  const backlog = Math.max(0, read - write);
  return [
    {key: 'committed', label: labels.committed, value: committed},
    {key: 'written', label: labels.written, value: written},
    {key: 'backlog', label: labels.backlog, value: backlog},
  ].filter((item) => item.value > 0);
}

/**
 * 把 0–1 的忙碌度/背压换成百分数，避免 Arc 轴从 0 起把小数压扁。
 * Scale 0–1 busy/backpressure to percent so Arc's zero-based axis does not flatten the line.
 */
export function scalePoints(points: TrendPoint[], factor: number): TrendPoint[] {
  return points.map(([ts, value]) => [ts, value * factor]);
}
