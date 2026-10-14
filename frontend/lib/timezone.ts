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
 * 全局时区感知的时间格式化工具。
 * Global timezone-aware time formatting helpers.
 *
 * 后端统一返回 RFC3339（UTC，带 Z）时间；展示时按用户偏好时区转换：
 * 用户时区为空表示“跟随浏览器”。
 * The backend returns RFC3339 (UTC, with Z); display converts to the user's preferred
 * timezone, where an empty preference means "follow the browser".
 */

export const TIMEZONE_STORAGE_KEY = 'stx.user.timezone';

/** 空串表示跟随浏览器 / Empty string means follow the browser. */
export const FOLLOW_BROWSER_TIMEZONE = '';

let userTimezone: string | null = null;

const FALLBACK_TIMEZONES = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Hong_Kong',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Asia/Kolkata',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Paris',
  'America/New_York',
  'America/Chicago',
  'America/Los_Angeles',
  'Australia/Sydney',
];

/** 校验 IANA 时区名 / Validate an IANA timezone name. */
export function isValidTimezone(value: string | null | undefined): value is string {
  if (!value) {
    return false;
  }
  try {
    new Intl.DateTimeFormat('en-US', {timeZone: value});
    return true;
  } catch {
    return false;
  }
}

/** 浏览器时区 / Browser timezone. */
export function getBrowserTimezone(): string {
  try {
    return new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

function readStoredTimezone(): string {
  if (typeof window === 'undefined') {
    return FOLLOW_BROWSER_TIMEZONE;
  }
  try {
    const stored = window.localStorage.getItem(TIMEZONE_STORAGE_KEY) || '';
    return isValidTimezone(stored) ? stored : FOLLOW_BROWSER_TIMEZONE;
  } catch {
    return FOLLOW_BROWSER_TIMEZONE;
  }
}

/** 当前用户时区偏好（空=跟随浏览器）/ Current user preference ('' = follow browser). */
export function getUserTimezonePreference(): string {
  if (userTimezone === null) {
    userTimezone = readStoredTimezone();
  }
  return userTimezone;
}

/** 设置用户时区偏好并持久化到本地 / Set the user preference and persist locally. */
export function setUserTimezonePreference(value: string | null | undefined): void {
  const next = isValidTimezone(value) ? value : FOLLOW_BROWSER_TIMEZONE;
  userTimezone = next;
  if (typeof window !== 'undefined') {
    try {
      if (next) {
        window.localStorage.setItem(TIMEZONE_STORAGE_KEY, next);
      } else {
        window.localStorage.removeItem(TIMEZONE_STORAGE_KEY);
      }
    } catch {
      // ignore storage errors
    }
  }
}

/** 实际用于展示的时区 / Effective display timezone. */
export function resolveDisplayTimezone(): string {
  return getUserTimezonePreference() || getBrowserTimezone();
}

/** 可选 IANA 时区列表 / Selectable IANA timezones. */
export function listTimezones(): string[] {
  const intl = Intl as unknown as {supportedValuesOf?: (key: string) => string[]};
  let values: string[] = [];
  try {
    values = intl.supportedValuesOf ? intl.supportedValuesOf('timeZone') : [];
  } catch {
    values = [];
  }
  const merged = new Set<string>(['UTC', ...(values.length ? values : FALLBACK_TIMEZONES)]);
  return Array.from(merged);
}

type TimeInput = string | number | Date | null | undefined;

const NAIVE_DATETIME = /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d+)?)?$/;

/**
 * 解析时间输入；无偏移的旧格式字符串按 UTC 解释（后端存储为 UTC）。
 * Parse a time input; legacy naive strings are interpreted as UTC (backend storage is UTC).
 */
export function parseTime(value: TimeInput): Date | null {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value;
  }
  if (typeof value === 'number') {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date;
  }
  const trimmed = value.trim();
  const normalized = NAIVE_DATETIME.test(trimmed) ? `${trimmed.replace(' ', 'T')}Z` : trimmed;
  const date = new Date(normalized);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) {
    return null;
  }
  return date;
}

export interface FormatTimeOptions {
  timeZone?: string;
  locale?: string;
  fallback?: string;
}

function format(value: TimeInput, parts: Intl.DateTimeFormatOptions, options: FormatTimeOptions = {}): string {
  const date = parseTime(value);
  if (!date) {
    return options.fallback ?? '-';
  }
  const timeZone = isValidTimezone(options.timeZone) ? options.timeZone : resolveDisplayTimezone();
  try {
    return new Intl.DateTimeFormat(options.locale ?? 'zh-CN', {hour12: false, ...parts, timeZone}).format(date);
  } catch {
    return date.toISOString();
  }
}

/** yyyy/MM/dd HH:mm:ss */
export function formatDateTimeTz(value: TimeInput, options?: FormatTimeOptions): string {
  return format(
    value,
    {year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'},
    options,
  );
}

/** yyyy/MM/dd HH:mm */
export function formatDateTimeMinuteTz(value: TimeInput, options?: FormatTimeOptions): string {
  return format(value, {year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}, options);
}

/** yyyy/MM/dd */
export function formatDateTz(value: TimeInput, options?: FormatTimeOptions): string {
  return format(value, {year: 'numeric', month: '2-digit', day: '2-digit'}, options);
}

/** HH:mm:ss */
export function formatTimeTz(value: TimeInput, options?: FormatTimeOptions): string {
  return format(value, {hour: '2-digit', minute: '2-digit', second: '2-digit'}, options);
}

let systemTimezone = '';

/** 记录系统默认展示时区（来自用户信息 system_timezone）/ Record the system display timezone from user info. */
export function setSystemTimezone(value: string | null | undefined): void {
  systemTimezone = isValidTimezone(value) ? value : '';
}

/**
 * 系统默认展示时区（app.display_timezone）；未知时返回空串。与后端一致，用于未设置时区的 cron。
 * System display timezone (app.display_timezone), '' when unknown. Mirrors the backend fallback for cron without a timezone.
 */
export function getSystemTimezone(): string {
  return systemTimezone;
}

/** 解析 cron 任务实际使用的时区：任务时区 > 系统时区 > 浏览器 / Effective cron timezone: task > system > browser. */
export function resolveCronTimezone(taskTimezone?: string | null): string {
  if (isValidTimezone(taskTimezone)) {
    return taskTimezone;
  }
  return getSystemTimezone() || getBrowserTimezone();
}

const pad2 = (value: number) => String(value).padStart(2, '0');

function wallFieldsInZone(date: Date, timeZone: string) {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(date);
  const get = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? 0);
  return {
    year: get('year'),
    month: get('month'),
    day: get('day'),
    hour: get('hour') % 24,
    minute: get('minute'),
    second: get('second'),
  };
}

/** 某时刻在时区下的 UTC 偏移（毫秒）/ UTC offset (ms) of the zone at the given instant. */
export function timezoneOffsetMs(date: Date, timeZone: string): number {
  const f = wallFieldsInZone(date, timeZone);
  const asUTC = Date.UTC(f.year, f.month - 1, f.day, f.hour, f.minute, f.second);
  return asUTC - Math.floor(date.getTime() / 1000) * 1000;
}

const WALL_TIME = /^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?)?$/;

/**
 * 把无偏移的墙上时间（datetime-local / date 输入值）按时区解释为绝对时刻；默认用户有效时区。
 * Interpret a naive wall time (datetime-local / date input value) in a timezone; defaults to the effective user zone.
 */
export function zonedWallTimeToDate(value: string, timeZone?: string): Date | null {
  const match = WALL_TIME.exec((value || '').trim());
  if (!match) {
    return null;
  }
  const zone = isValidTimezone(timeZone) ? timeZone : resolveDisplayTimezone();
  const [, y, mo, d, h = '0', mi = '0', s = '0', ms = '0'] = match;
  const wallAsUTC = Date.UTC(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s), Number(ms.padEnd(3, '0')));
  // 两次修正以处理夏令时边界 / two passes to settle DST boundaries
  let instant = wallAsUTC - timezoneOffsetMs(new Date(wallAsUTC), zone);
  instant = wallAsUTC - timezoneOffsetMs(new Date(instant), zone);
  return new Date(instant);
}

/** 墙上时间转 RFC3339（UTC，Z）；无效返回 undefined / Wall time to RFC3339 UTC, undefined when invalid. */
export function wallTimeToRFC3339(value: string, timeZone?: string): string | undefined {
  const date = zonedWallTimeToDate(value, timeZone);
  return date ? date.toISOString() : undefined;
}

/** 绝对时刻转 datetime-local 输入值 YYYY-MM-DDTHH:mm（默认用户有效时区）/ Instant to datetime-local input value. */
export function formatDateTimeLocalInput(date: Date, timeZone?: string): string {
  const zone = isValidTimezone(timeZone) ? timeZone : resolveDisplayTimezone();
  const f = wallFieldsInZone(date, zone);
  return `${f.year}-${pad2(f.month)}-${pad2(f.day)}T${pad2(f.hour)}:${pad2(f.minute)}`;
}
