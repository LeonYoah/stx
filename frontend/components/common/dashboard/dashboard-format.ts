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

/**
 * 紧凑数字：12,345 → 12.3k，1,234,567 → 1.2M；用于表格与指标说明。
 * Compact numbers: 12,345 → 12.3k, 1,234,567 → 1.2M; for tables and KPI captions.
 */
export function formatCompactNumber(
  value: number | null | undefined,
  locale?: string,
): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '-';
  }
  return new Intl.NumberFormat(locale || 'en', {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(value);
}

const SCALED_UNITS = ['', 'K', 'M', 'B', 'T'] as const;

/**
 * 大数字缩放：把任意大小的数压成 ≤999.9 的值 + 单位后缀（K/M/B/T），供宽度受限的
 * 指标卡 / 标题数字复用，避免长数字溢出相邻卡片。小于 1000 的原样返回。
 * Scale large numbers into a value ≤999.9 plus a unit suffix (K/M/B/T) so width-limited
 * KPI cards and headline numbers never overflow into neighboring cards. Values below
 * 1000 pass through unchanged.
 */
export function scaledCompact(value: number | null | undefined): {
  value: number;
  decimals: number;
  unit: string;
} {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return {value: 0, decimals: 0, unit: ''};
  }
  const sign = Math.sign(value) || 1;
  let scaled = Math.abs(value);
  let tier = 0;
  // 先按 1000 分档；取整后若顶到 1000 再进一档（999.96 → 1.0M 而不是 1000.0K）。
  // Tier by 1000 first; if the rounded value reaches 1000, step up once more
  // (999.96 → 1.0M rather than 1000.0K).
  while (scaled >= 1000 && tier < SCALED_UNITS.length - 1) {
    scaled /= 1000;
    tier += 1;
  }
  if (tier === 0) {
    return {value, decimals: 0, unit: ''};
  }
  let rounded = Math.round(scaled * 10) / 10;
  if (rounded >= 1000 && tier < SCALED_UNITS.length - 1) {
    scaled /= 1000;
    tier += 1;
    rounded = Math.round(scaled * 10) / 10;
  }
  return {value: sign * rounded, decimals: 1, unit: SCALED_UNITS[tier]};
}

/**
 * 时长（秒）→ 「2 天 4 小时」/「3 小时 12 分」/「5 分」，只取最大的两级。
 * Duration (seconds) → "2d 4h" / "3h 12m" / "5m", keeping only the two largest units.
 */
export function formatDurationSeconds(
  seconds: number,
  units: {d: string; h: string; m: string},
): string {
  const total = Math.max(0, Math.floor(seconds));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days > 0) {
    return `${days}${units.d} ${hours}${units.h}`;
  }
  if (hours > 0) {
    return `${hours}${units.h} ${minutes}${units.m}`;
  }
  // 不足 1 分钟显示「<1 分」，避免出现「耗时 0 分」。/ Under a minute reads "<1m" instead of "0m".
  return minutes > 0 ? `${minutes}${units.m}` : `<1${units.m}`;
}

/** 趋势至少覆盖这么久才算「较 1 小时前」。/ The trend must span this long to count as "vs. 1 hour ago". */
const MIN_TREND_SPAN_MS = 50 * 60 * 1000;

/**
 * 写入速率相对变化：最近点对比约 1 小时前的第一个点。趋势不足约 1 小时（如作业刚启动）、样本不足或基数为 0 时返回 null，
 * 避免把「刚启动时的低值」当成 1 小时前。
 * Relative write-rate change: the latest point vs. the first point about an hour ago. Returns null when the trend spans
 * less than about an hour (e.g. a job just started), samples are too few or the base is zero, so a just-started low value
 * is never passed off as "an hour ago".
 */
export function trendChangePercent(points: [number, number][]): number | null {
  if (points.length < 2) {
    return null;
  }
  const [firstTs, first] = points[0];
  const [lastTs, last] = points[points.length - 1];
  if (lastTs - firstTs < MIN_TREND_SPAN_MS) {
    return null;
  }
  if (!Number.isFinite(first) || !Number.isFinite(last) || first <= 0) {
    return null;
  }
  return Math.round(((last - first) / first) * 100);
}

/**
 * 共享分类色板：四条 Arc --series-* + 混合色，随主题/明暗自适应。
 * 语义色查不到的维度按名字排序轮流取这里面的色，同名恒同色。
 * Shared categorical palette: the four Arc --series-* tokens plus mixes, theme/dark
 * adaptive. Dimensions without a semantic color cycle through this palette by sorted
 * name so a name always keeps one color.
 */
export const SERIES_PALETTE = [
  'var(--series-2)',
  'var(--series-1)',
  'var(--series-3)',
  'var(--series-4)',
  'color-mix(in oklab, var(--series-2) 55%, var(--series-4))',
  'color-mix(in oklab, var(--series-1) 55%, var(--series-3))',
  'color-mix(in oklab, var(--series-4) 55%, var(--series-2))',
  'color-mix(in oklab, var(--series-3) 55%, var(--series-1))',
] as const;

/** 中性线色（参考线 / 阈值线 / 中性维度）。/ Neutral line color (references, thresholds, neutral dims). */
export const NEUTRAL_LINE =
  'color-mix(in oklab, var(--foreground) 46%, var(--surface))';

/**
 * 维度语义色表：读/rx 与写/tx 沿用作业吞吐的 --series-1/--series-2 约定；
 * 风险型维度（iowait、steal、BLOCKED、queued）用语义色；其余点名维度固定到分类色。
 * 键一律小写匹配。/ Semantic colors per dimension: read/rx and write/tx reuse the
 * job-throughput --series-1/--series-2 convention; risky dimensions (iowait, steal,
 * BLOCKED, queued) take status colors; other named dims pin to categorical colors.
 * Keys match case-insensitively.
 */
const DIMENSION_COLORS: Record<string, string> = {
  rx: 'var(--series-1)',
  read: 'var(--series-1)',
  tx: 'var(--series-2)',
  write: 'var(--series-2)',
  user: 'var(--series-1)',
  system: 'var(--series-2)',
  nice: 'var(--series-3)',
  irq: 'var(--series-4)',
  softirq: 'color-mix(in oklab, var(--series-4) 55%, var(--series-2))',
  iowait: 'var(--warning)',
  steal: 'var(--danger)',
  used: 'var(--series-2)',
  cached: 'var(--series-3)',
  available: 'var(--series-4)',
  active: 'var(--series-1)',
  size: 'var(--series-4)',
  queued: 'var(--warning)',
  owned: 'var(--series-1)',
  backup: 'var(--series-4)',
  rss: 'var(--series-2)',
  offheap: 'var(--series-4)',
  total: 'var(--series-1)',
  loaded: 'var(--series-1)',
  unloaded: 'var(--warning)',
  current: 'var(--series-1)',
  daemon: 'var(--series-3)',
  peak: NEUTRAL_LINE,
  committed: NEUTRAL_LINE,
  blocked: 'var(--danger)',
  waiting: 'var(--warning)',
  timed_waiting: 'var(--series-3)',
  request_slot: 'var(--series-1)',
  report_metrics: 'var(--series-3)',
  established: 'var(--series-1)',
  allocated: 'var(--series-1)',
  open: 'var(--series-1)',
  '1m': 'var(--series-1)',
  '5m': 'var(--series-3)',
  '15m': 'var(--series-4)',
};

/**
 * 维度名子串规则：JVM 代际 / 池名这类维度名不固定（不同 GC 名字不同），
 * 按子串归一到语义色。/ Substring rules for JVM generations and pools whose exact
 * names vary by GC — normalized to semantic colors by substring.
 */
const DIMENSION_RULES: [RegExp, string][] = [
  [/eden|young|minor/i, 'var(--series-1)'],
  [/survivor/i, 'var(--series-3)'],
  [/old|tenured|major/i, 'var(--series-2)'],
  [/metaspace/i, 'var(--series-1)'],
  [/code|codeheap/i, 'var(--series-3)'],
  [/compressed|class space/i, 'var(--series-4)'],
  [/direct/i, 'var(--series-1)'],
  [/mapped/i, 'var(--series-4)'],
];

/**
 * 维度名 → 语义色：先精确匹配（取首词处理 "rx eth0"），再子串规则；查不到返回
 * undefined，调用方走色板回退。/ Dimension name → semantic color: exact match on the
 * first word (handles "rx eth0"), then substring rules; undefined means the caller
 * should fall back to the palette.
 */
export function dimensionColor(label: string): string | undefined {
  const key = label.trim().toLowerCase();
  if (DIMENSION_COLORS[key]) {
    return DIMENSION_COLORS[key];
  }
  const head = key.split(/\s+/)[0];
  if (DIMENSION_COLORS[head]) {
    return DIMENSION_COLORS[head];
  }
  for (const [re, color] of DIMENSION_RULES) {
    if (re.test(key)) {
      return color;
    }
  }
  return undefined;
}

/**
 * 给一组维度名配色：语义色命中的取语义色（同名多设备时后续实例逐级调淡保持方向一致），
 * 其余按名字序轮流取 SERIES_PALETTE —— 同色语义不同设备仍可分辨，未知名稳定同色。
 * Colors a label set: semantic hits take the semantic color (extra devices of the same
 * direction fade progressively so eth0/eth1 stay separable); unknowns cycle
 * SERIES_PALETTE in sorted order — stable per name.
 */
export function colorForLabels(labels: string[]): Map<string, string> {
  const colors = new Map<string, string>();
  const sorted = Array.from(new Set(labels)).sort();
  const semanticGroups = new Map<string, string[]>();
  let palette = 0;
  for (const name of sorted) {
    const semantic = dimensionColor(name);
    if (semantic) {
      const group = semanticGroups.get(semantic) ?? [];
      group.push(name);
      semanticGroups.set(semantic, group);
    } else {
      colors.set(name, SERIES_PALETTE[palette % SERIES_PALETTE.length]);
      palette += 1;
    }
  }
  // 同语义多实例：第一个满色，其余 65% / 45% / 30% 调淡（保持方向色不变）。
  // Same semantic across devices: the first keeps the full color, the rest fade to
  // 65% / 45% / 30% while staying on the same hue.
  const FADES = [100, 65, 45, 30];
  for (const [color, names] of semanticGroups) {
    names.forEach((name, index) => {
      const fade = FADES[Math.min(index, FADES.length - 1)];
      colors.set(
        name,
        fade === 100
          ? color
          : `color-mix(in oklab, ${color} ${fade}%, var(--surface))`,
      );
    });
  }
  return colors;
}

/**
 * 字节量纲：IEC 1024 进制，≥100 取整、其余一位小数。
 * Byte quantities in IEC units (1024-based): whole numbers at ≥100, one decimal below.
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return '—';
  }
  if (bytes < 1024) {
    return `${Math.round(bytes)} B`;
  }
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let value = bytes;
  let index = -1;
  do {
    value /= 1024;
    index++;
  } while (value >= 1024 && index < units.length - 1);
  return `${value >= 100 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

/** 速率序列的窗口统计。/ Windowed stats for a rate series. */
export interface RateSummary {
  avg: number;
  peak: number;
}

/**
 * 窗口内速率点的均值与峰值；忽略 NaN / Infinity / null，没有有效点时返回 null。
 * 均值跟随头部时间窗，与「窗口内平均」口径一致；不用最后一个点是因为它和
 * 指标栏的 10 秒对账实时值口径不同，两处数字对不上会让人困惑。
 * Average and peak of the rate points inside the window; NaN / Infinity / null are
 * ignored and an all-invalid series returns null. The average follows the header's
 * time window, matching the "window average" wording; the last point is not used
 * because it is a different sampling from the KPI strip's 10-second reconcile and
 * the two numbers would disagree.
 */
export function summarizeRate(points: [number, number][]): RateSummary | null {
  let sum = 0;
  let count = 0;
  let peak = Number.NEGATIVE_INFINITY;
  for (const point of points) {
    const value = point?.[1];
    if (!Number.isFinite(value)) {
      continue;
    }
    sum += value;
    count += 1;
    if (value > peak) {
      peak = value;
    }
  }
  if (count === 0) {
    return null;
  }
  return {avg: sum / count, peak};
}
