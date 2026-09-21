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

// 内置时间系统变量定义与动态预览求值工具
// Built-in time system variable definitions and dynamic preview evaluation utilities

export interface BuiltinTimeVariableItem {
  expr: string;
  descKey: string;
  category: 'business' | 'datetime' | 'format' | 'offset' | 'calendar';
}

/**
 * 平台内置时间变量配置清单
 * Built-in time variable definitions list
 */
export const BUILTIN_TIME_VARIABLE_ITEMS: readonly BuiltinTimeVariableItem[] = [
  {
    expr: 'system.biz.curdate',
    descKey: 'builtinSystemBizCurdateDesc',
    category: 'business',
  },
  {
    expr: 'system.biz.date',
    descKey: 'builtinSystemBizDateDesc',
    category: 'business',
  },
  {
    expr: 'system.datetime',
    descKey: 'builtinSystemDateTimeDesc',
    category: 'datetime',
  },
  {
    expr: 'yyyyMMdd',
    descKey: 'builtinFormatDesc',
    category: 'format',
  },
  {
    expr: 'yyyy-MM-dd',
    descKey: 'builtinFormatDesc',
    category: 'format',
  },
  {
    expr: 'yyyyMMdd+1',
    descKey: 'builtinOffsetDesc',
    category: 'offset',
  },
  {
    expr: 'add_months(yyyyMMdd,-1)',
    descKey: 'builtinAddMonthsDesc',
    category: 'offset',
  },
  {
    expr: 'this_day(yyyy-MM-dd)',
    descKey: 'builtinThisDayDesc',
    category: 'calendar',
  },
  {
    expr: 'last_day(yyyy-MM-dd)',
    descKey: 'builtinLastDayDesc',
    category: 'calendar',
  },
  {
    expr: 'year_week(yyyy-MM-dd)',
    descKey: 'builtinYearWeekDesc',
    category: 'calendar',
  },
  {
    expr: 'month_first_day(yyyy-MM-dd,0)',
    descKey: 'builtinMonthFirstDayDesc',
    category: 'calendar',
  },
  {
    expr: 'month_last_day(yyyy-MM-dd,0)',
    descKey: 'builtinMonthLastDayDesc',
    category: 'calendar',
  },
  {
    expr: 'week_first_day(yyyy-MM-dd,0)',
    descKey: 'builtinWeekFirstDayDesc',
    category: 'calendar',
  },
  {
    expr: 'week_last_day(yyyy-MM-dd,0)',
    descKey: 'builtinWeekLastDayDesc',
    category: 'calendar',
  },
] as const;

/**
 * 补齐两位时间数字
 * Pad time unit to 2 digits
 */
export function padTimeUnit(value: number): string {
  return String(value).padStart(2, '0');
}

/**
 * 格式化日期对象为指定模板字符串
 * Format Date object into template string
 */
export function formatBuiltinPreviewDate(date: Date, pattern: string): string {
  return pattern
    .replaceAll('yyyy', String(date.getFullYear()))
    .replaceAll('MM', padTimeUnit(date.getMonth() + 1))
    .replaceAll('dd', padTimeUnit(date.getDate()))
    .replaceAll('HH', padTimeUnit(date.getHours()))
    .replaceAll('mm', padTimeUnit(date.getMinutes()))
    .replaceAll('ss', padTimeUnit(date.getSeconds()));
}

/**
 * 增减指定月数
 * Add or subtract months
 */
export function addMonths(date: Date, months: number): Date {
  const next = new Date(date.getTime());
  next.setMonth(next.getMonth() + months);
  return next;
}

/**
 * 获取指定日期所在周的周一
 * Get Monday of the week for given date
 */
export function startOfWeek(date: Date): Date {
  const next = new Date(date.getTime());
  const day = next.getDay() === 0 ? 7 : next.getDay();
  next.setDate(next.getDate() - day + 1);
  return next;
}

/**
 * 计算年份与周序号
 * Calculate year and week number
 */
export function yearWeek(
  date: Date,
  weekStart = 1,
): {year: number; week: number} {
  const next = new Date(date.getTime());
  const jsWeekStart = weekStart === 7 ? 0 : weekStart;
  const day = next.getDay();
  const diff = (7 + day - jsWeekStart) % 7;
  next.setDate(next.getDate() - diff);
  const first = new Date(
    next.getFullYear(),
    0,
    1,
    next.getHours(),
    next.getMinutes(),
    next.getSeconds(),
    next.getMilliseconds(),
  );
  const firstDay = first.getDay();
  const firstDiff = (7 + firstDay - jsWeekStart) % 7;
  first.setDate(first.getDate() - firstDiff);
  const week =
    Math.floor(
      (next.getTime() - first.getTime()) / (7 * 24 * 60 * 60 * 1000),
    ) + 1;
  return {year: next.getFullYear(), week};
}

/**
 * 解析内置时间表达式动态预览值
 * Resolve preview value for built-in time expression
 */
export function resolveBuiltinPreviewExpression(
  expr: string,
  now = new Date(),
): string | null {
  const trimmed = expr.trim();
  if (!trimmed) {
    return null;
  }
  if (trimmed === 'system.biz.date') {
    const prev = new Date(now.getTime());
    prev.setDate(prev.getDate() - 1);
    return formatBuiltinPreviewDate(prev, 'yyyyMMdd');
  }
  if (trimmed === 'system.biz.curdate') {
    return formatBuiltinPreviewDate(now, 'yyyyMMdd');
  }
  if (trimmed === 'system.datetime') {
    return formatBuiltinPreviewDate(now, 'yyyyMMddHHmmss');
  }
  if (trimmed === 'system.project.name') {
    return 'STX';
  }
  if (trimmed === 'system.project.code') {
    return 'stx';
  }
  if (/^add_months\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^add_months\((.+),(.+)\)$/);
    if (!match) return null;
    const format = match[1].trim();
    const offset = Number(match[2].trim());
    if (!Number.isFinite(offset)) return null;
    return formatBuiltinPreviewDate(addMonths(now, offset), format);
  }
  if (/^this_day\((.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^this_day\((.+)\)$/);
    return match ? formatBuiltinPreviewDate(now, match[1].trim()) : null;
  }
  if (/^last_day\((.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^last_day\((.+)\)$/);
    if (!match) return null;
    const prev = new Date(now.getTime());
    prev.setDate(prev.getDate() - 1);
    return formatBuiltinPreviewDate(prev, match[1].trim());
  }
  if (/^month_first_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^month_first_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = addMonths(now, Number(match[2].trim()));
    const first = new Date(
      target.getFullYear(),
      target.getMonth(),
      1,
      target.getHours(),
      target.getMinutes(),
      target.getSeconds(),
    );
    return formatBuiltinPreviewDate(first, match[1].trim());
  }
  if (/^month_last_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^month_last_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = addMonths(now, Number(match[2].trim()) + 1);
    const last = new Date(
      target.getFullYear(),
      target.getMonth(),
      0,
      target.getHours(),
      target.getMinutes(),
      target.getSeconds(),
    );
    return formatBuiltinPreviewDate(last, match[1].trim());
  }
  if (/^week_first_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^week_first_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = new Date(now.getTime());
    target.setDate(target.getDate() + Number(match[2].trim()) * 7);
    return formatBuiltinPreviewDate(startOfWeek(target), match[1].trim());
  }
  if (/^week_last_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^week_last_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = new Date(now.getTime());
    target.setDate(target.getDate() + Number(match[2].trim()) * 7);
    const end = startOfWeek(target);
    end.setDate(end.getDate() + 6);
    return formatBuiltinPreviewDate(end, match[1].trim());
  }
  if (
    /^year_week\((.+)\)$/.test(trimmed) ||
    /^year_week\((.+),(.+)\)$/.test(trimmed)
  ) {
    const match = trimmed.match(/^year_week\((.+?)(?:,(.+))?\)$/);
    if (!match) return null;
    const format = match[1].trim();
    const weekStart = match[2] ? Number(match[2].trim()) : 1;
    const result = yearWeek(now, Number.isFinite(weekStart) ? weekStart : 1);
    return format
      .replaceAll('yyyy', String(result.year))
      .replaceAll('MM', padTimeUnit(result.week));
  }
  const offsetMatch = trimmed.match(/^(.+?)([+-])(\d+(?:\/\d+)*)$/);
  if (offsetMatch) {
    const [, format, sign, rawOffset] = offsetMatch;
    const [first, ...rest] = rawOffset.split('/');
    const offset = rest.reduce(
      (acc, value) => acc / Number(value),
      Number(first),
    );
    const hours = (sign === '-' ? -1 : 1) * offset * 24;
    const target = new Date(now.getTime() + hours * 60 * 60 * 1000);
    return formatBuiltinPreviewDate(target, format.trim());
  }
  if (/(yyyy|MM|dd|HH|mm|ss)/.test(trimmed)) {
    return formatBuiltinPreviewDate(now, trimmed);
  }
  return null;
}
