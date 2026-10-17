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

export const MONTH_NAMES = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC'];
export const DOW_NAMES = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT'];

export type CronTranslator = (key: string, values?: Record<string, string | number>) => string;

function pad(value: number) {
  return String(value).padStart(2, '0');
}

export function splitCron(expr: string) {
  return expr.trim().split(' ').filter(Boolean);
}

function normalizeToken(
  token: string,
  namesMap: Record<string, number>,
  isDayOfWeek = false,
) {
  const upper = String(token).toUpperCase();
  if (Object.prototype.hasOwnProperty.call(namesMap, upper)) return namesMap[upper];
  const num = Number(token);
  if (isDayOfWeek && num === 7) return 0;
  return num;
}

function buildNamesMap(items: Array<{name: string; value: number}> = []) {
  const acc: Record<string, number> = {};
  for (const item of items) {
    acc[item.name] = item.value;
  }
  return acc;
}

function addRange(set: Set<number>, start: number, end: number, step = 1) {
  for (let i = start; i <= end; i += step) {
    set.add(i);
  }
}

export function parseField(
  field: string,
  min: number,
  max: number,
  names: Array<{name: string; value: number}> = [],
  isDayOfWeek = false,
) {
  const namesMap = buildNamesMap(names);
  const raw = field.trim();
  if (!raw) throw new Error('Field is empty');

  const values = new Set<number>();
  const parts = raw.split(',').map((item) => item.trim()).filter(Boolean);
  if (parts.length === 0) throw new Error('Invalid field');

  for (const part of parts) {
    if (part === '*') {
      addRange(values, min, max);
      continue;
    }

    if (part.includes('/')) {
      const pieces = part.split('/');
      if (pieces.length !== 2) throw new Error(`Invalid step: ${part}`);

      const base = pieces[0];
      const step = Number(pieces[1]);
      if (!Number.isInteger(step) || step <= 0) throw new Error(`Invalid step: ${part}`);

      let rangeStart = min;
      let rangeEnd = max;

      if (base !== '*') {
        if (base.includes('-')) {
          const [startToken, endToken] = base.split('-');
          rangeStart = normalizeToken(startToken, namesMap, isDayOfWeek);
          rangeEnd = normalizeToken(endToken, namesMap, isDayOfWeek);
        } else {
          rangeStart = normalizeToken(base, namesMap, isDayOfWeek);
          rangeEnd = max;
        }
      }

      if (!Number.isInteger(rangeStart) || !Number.isInteger(rangeEnd)) throw new Error(`Invalid range: ${part}`);
      if (rangeStart < min || rangeEnd > max || rangeStart > rangeEnd) throw new Error(`Out of bounds: ${part}`);

      addRange(values, rangeStart, rangeEnd, step);
      continue;
    }

    if (part.includes('-')) {
      const pieces = part.split('-');
      if (pieces.length !== 2) throw new Error(`Invalid range: ${part}`);
      const start = normalizeToken(pieces[0], namesMap, isDayOfWeek);
      const end = normalizeToken(pieces[1], namesMap, isDayOfWeek);
      if (!Number.isInteger(start) || !Number.isInteger(end)) throw new Error(`Invalid range: ${part}`);
      if (start < min || end > max || start > end) throw new Error(`Out of bounds: ${part}`);
      addRange(values, start, end);
      continue;
    }

    const single = normalizeToken(part, namesMap, isDayOfWeek);
    if (!Number.isInteger(single)) throw new Error(`Invalid token: ${part}`);
    if (single < min || single > max) throw new Error(`Out of bounds: ${part}`);
    values.add(single);
  }

  return [...values].sort((a, b) => a - b);
}

export function parseCron(expr: string) {
  const fields = splitCron(expr);
  if (fields.length !== 5) {
    throw new Error('Only standard 5-field cron is supported');
  }

  const [minute, hour, dayOfMonth, month, dayOfWeek] = fields;

  return {
    raw: expr,
    fields: {minute, hour, dayOfMonth, month, dayOfWeek},
    expanded: {
      minute: parseField(minute, 0, 59),
      hour: parseField(hour, 0, 23),
      dayOfMonth: parseField(dayOfMonth, 1, 31),
      month: parseField(month, 1, 12, MONTH_NAMES.map((name, index) => ({name, value: index + 1}))),
      dayOfWeek: parseField(dayOfWeek, 0, 6, DOW_NAMES.map((name, index) => ({name, value: index})), true),
    },
  };
}

function isWildcardField(value: string) {
  return value.trim() === '*';
}

const WEEKDAY_INDEX: Record<string, number> = {Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6};

/**
 * 取指定时区下的墙上时间字段；未指定时区时使用浏览器本地时区。
 * Wall-clock fields in the given IANA timezone (browser-local when omitted).
 */
export function wallClock(date: Date, timeZone?: string) {
  if (!timeZone) {
    return {
      minute: date.getMinutes(),
      hour: date.getHours(),
      month: date.getMonth() + 1,
      day: date.getDate(),
      weekday: date.getDay(),
    };
  }
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    minute: 'numeric',
    hour: 'numeric',
    month: 'numeric',
    day: 'numeric',
    weekday: 'short',
  }).formatToParts(date);
  const get = (type: string) => parts.find((part) => part.type === type)?.value ?? '';
  return {
    minute: Number(get('minute')),
    hour: Number(get('hour')) % 24,
    month: Number(get('month')),
    day: Number(get('day')),
    weekday: WEEKDAY_INDEX[get('weekday')] ?? 0,
  };
}

export function matchDate(parsed: ReturnType<typeof parseCron>, date: Date, timeZone?: string) {
  const clock = wallClock(date, timeZone);
  const minuteMatched = parsed.expanded.minute.includes(clock.minute);
  const hourMatched = parsed.expanded.hour.includes(clock.hour);
  const monthMatched = parsed.expanded.month.includes(clock.month);
  const domMatched = parsed.expanded.dayOfMonth.includes(clock.day);
  const dowMatched = parsed.expanded.dayOfWeek.includes(clock.weekday);

  const domWildcard = isWildcardField(parsed.fields.dayOfMonth);
  const dowWildcard = isWildcardField(parsed.fields.dayOfWeek);

  let dayMatched = false;
  if (domWildcard && dowWildcard) {
    dayMatched = true;
  } else if (domWildcard) {
    dayMatched = dowMatched;
  } else if (dowWildcard) {
    dayMatched = domMatched;
  } else {
    dayMatched = domMatched || dowMatched;
  }

  return minuteMatched && hourMatched && monthMatched && dayMatched;
}

/**
 * 计算下几次触发时间；timeZone 为任务时区（与后端调度一致），省略时使用浏览器时区。
 * Next fire times; timeZone is the task timezone (matching the backend scheduler), browser-local when omitted.
 */
export function getNextRuns(expr: string, count = 5, from = new Date(), timeZone?: string) {
  const parsed = parseCron(expr);
  const results: Date[] = [];
  // 按绝对分钟步进（所有现行时区偏移都是整分钟）/ step absolute minutes (all real offsets are whole minutes)
  const cursor = new Date(Math.floor(from.getTime() / 60000) * 60000 + 60000);

  let attempts = 0;
  const maxAttempts = 60 * 24 * 366;

  while (results.length < count && attempts < maxAttempts) {
    if (matchDate(parsed, cursor, timeZone)) {
      results.push(new Date(cursor));
    }
    cursor.setTime(cursor.getTime() + 60000);
    attempts += 1;
  }

  return results;
}

export function expressionFromParts(parts: Record<string, string>) {
  return [parts.minute, parts.hour, parts.dayOfMonth, parts.month, parts.dayOfWeek].join(' ');
}

export function parseExpressionToParts(expr: string) {
  const fields = splitCron(expr);
  if (fields.length !== 5) throw new Error('Expected 5 fields');
  const [minute, hour, dayOfMonth, month, dayOfWeek] = fields;
  return {minute, hour, dayOfMonth, month, dayOfWeek};
}

export function cronToText(expr: string, t: CronTranslator) {
  try {
    const fields = splitCron(expr);
    const minute = fields[0];
    const hour = fields[1];
    const dom = fields[2];
    const month = fields[3];
    const dow = fields[4];

    if (expr === '* * * * *') return t('cronEveryMinute');
    if (minute.startsWith('*/') && hour === '*' && dom === '*' && month === '*' && dow === '*') {
      return t('cronEveryNMinutes', {count: minute.slice(2)});
    }
    if (minute.startsWith('*/') && /^\d+$/.test(hour) && dom === '*' && month === '*' && dow === '*') {
      return t('cronEveryNMinutesDuringHour', {count: minute.slice(2), hour: pad(Number(hour))});
    }
    if (expr === '0 * * * *') return t('cronEveryHour');
    if (expr === '0 0 * * *') return t('cronEveryDayAtMidnight');
    if (expr === '0 9 * * MON-FRI') return t('cronWeekdaysAtNine');
    if (expr === '30 8 * * MON-FRI') return t('cronWeekdaysAtEightThirty');
    if (expr === '0 0 * * SUN') return t('cronWeeklySunday');
    if (expr === '0 9 1 * *') return t('cronMonthlyFirstDayNine');
    if (expr === '0 0 1 */3 *') return t('cronQuarterlyFirstDay');
    if (/^\d+$/.test(hour) && /^\d+$/.test(minute) && dom === '*' && month === '*' && dow === '*') {
      return t('cronAtTime', {hour: pad(Number(hour)), minute: pad(Number(minute))});
    }

    return t('cronRunsOnExpr', {expr});
  } catch {
    return t('invalidCronExpression');
  }
}
