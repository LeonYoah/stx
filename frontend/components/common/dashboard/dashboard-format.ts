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
