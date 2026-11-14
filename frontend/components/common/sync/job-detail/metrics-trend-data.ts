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
  /** 线条颜色（任意 CSS 颜色）；缺省时走 Arc 默认的 accent + 灰阶。 / Line color (any CSS color); falls back to Arc's accent + neutral steps. */
  color?: string;
};

/**
 * 趋势图的阶段配色：同一阶段在折线、环形图里始终同色，取 Arc 的 --series-* 分类色（随主题与明暗自适应）。
 * Stage colors for the trend charts: one stage keeps one color across line and donut charts, using Arc's
 * --series-* categorical tokens (they adapt to theme and dark mode).
 */
export const TREND_COLORS = {
  read: 'var(--series-1)',
  write: 'var(--series-2)',
  checkpoint: 'var(--series-3)',
  committed: 'var(--series-4)',
} as const;

/** 非 Source / Sink 的序列（Transform、背压队列）轮流用剩下两种分类色。 / Non source/sink series (transforms, queues) alternate the two remaining colors. */
const OTHER_CYCLE = [TREND_COLORS.checkpoint, TREND_COLORS.committed] as const;

/**
 * 按算子角色给多条同类序列（各算子忙碌度、各队列背压）配色：Source 用读色、Sink 用写色，与吞吐图一致；
 * 其余按名字排序轮流取色，同名在不同图里拿到同一颜色。
 * Color same-kind series (per-vertex busy, per-queue backpressure) by role: sources take the read color and sinks the
 * write color, matching the throughput chart; the rest alternate by sorted name so a name keeps its color across charts.
 */
export function colorByName(names: string[]): Map<string, string> {
  const colors = new Map<string, string>();
  let other = 0;
  Array.from(new Set(names)).sort().forEach((name) => {
    if (/\bSource\[/i.test(name)) {
      colors.set(name, TREND_COLORS.read);
    } else if (/\bSink\[/i.test(name)) {
      colors.set(name, TREND_COLORS.write);
    } else {
      colors.set(name, OTHER_CYCLE[other % OTHER_CYCLE.length]);
      other += 1;
    }
  });
  return colors;
}

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
  const last = total - 1;
  const step = Math.max(1, Math.round(last / 5));
  if (index === 0 || index === last) {
    return label;
  }
  // 末尾强制留标签，紧挨着它的等距刻度要让位，否则两个时间叠在一起（如 12:38 / 12:39）。
  // The last tick is always kept, so an even-step tick too close to it yields instead of overlapping.
  if (index % step === 0 && last - index >= step / 2) {
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
    ...(item.color ? {color: item.color} : {}),
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
  // 显式给每段阶段色：Arc 默认按出现顺序取色，某段为 0 被过滤后其余段会整体换色。
  // Pin stage colors: Arc colors by order of appearance, so filtering a zero segment would shift the rest.
  return [
    {key: 'committed', label: labels.committed, value: committed, color: TREND_COLORS.committed},
    {key: 'written', label: labels.written, value: written, color: TREND_COLORS.write},
    {key: 'backlog', label: labels.backlog, value: backlog, color: TREND_COLORS.read},
  ].filter((item) => item.value > 0);
}

/**
 * 把 0–1 的忙碌度/背压换成百分数，避免 Arc 轴从 0 起把小数压扁。
 * Scale 0–1 busy/backpressure to percent so Arc's zero-based axis does not flatten the line.
 */
export function scalePoints(points: TrendPoint[], factor: number): TrendPoint[] {
  return points.map(([ts, value]) => [ts, value * factor]);
}
