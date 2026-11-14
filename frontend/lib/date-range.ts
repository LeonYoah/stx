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

import type {DateRange} from '@/components/arc/date-range-picker/date-range-picker';

const YMD = /^(\d{4})-(\d{2})-(\d{2})/;

/**
 * 从墙上时间字符串取出日历日 YYYY-MM-DD。
 * Take the calendar day YYYY-MM-DD from a wall-time string.
 */
export function wallTimeToYmd(value: string): string | null {
  const match = YMD.exec((value || '').trim());
  return match ? `${match[1]}-${match[2]}-${match[3]}` : null;
}

/**
 * 把 YYYY-MM-DD 转成本地日历 Date（0 点），供 Arc DateRangePicker 使用。
 * Convert YYYY-MM-DD into a local calendar Date at midnight for Arc DateRangePicker.
 */
export function ymdToLocalDate(ymd: string): Date | null {
  const match = YMD.exec((ymd || '').trim());
  if (!match) {
    return null;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!year || month < 1 || month > 12 || day < 1 || day > 31) {
    return null;
  }
  return new Date(year, month - 1, day);
}

/**
 * 本地日历 Date 格式化为 YYYY-MM-DD。
 * Format a local calendar Date as YYYY-MM-DD.
 */
export function localDateToYmd(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * 把起止墙上时间收成 DateRangePicker 的闭区间日期。缺一端时用另一端补齐。
 * Fold start/end wall times into an inclusive DateRangePicker range. A missing end uses the other side.
 */
export function wallTimesToDateRange(startTime: string, endTime: string): DateRange | null {
  const startYmd = wallTimeToYmd(startTime);
  const endYmd = wallTimeToYmd(endTime);
  if (!startYmd && !endYmd) {
    return null;
  }
  const start = ymdToLocalDate(startYmd ?? endYmd ?? '');
  const end = ymdToLocalDate(endYmd ?? startYmd ?? '');
  if (!start || !end) {
    return null;
  }
  if (start.getTime() <= end.getTime()) {
    return {start, end};
  }
  return {start: end, end: start};
}

/**
 * DateRange 转墙上时间：开始日 00:00，结束日 23:59:59.999（含当天最后一毫秒）。
 * Convert a DateRange to wall times: start day 00:00, end day 23:59:59.999 (last millisecond of that day).
 *
 * 只用 `T23:59` 会变成 23:59:00.000，`created_at <= end_time` 会丢掉 23:59:01–23:59:59。
 * `T23:59` becomes 23:59:00.000, so `created_at <= end_time` drops 23:59:01–23:59:59.
 */
export function dateRangeToWallTimes(range: DateRange): {startTime: string; endTime: string} {
  return {
    startTime: `${localDateToYmd(range.start)}T00:00`,
    endTime: `${localDateToYmd(range.end)}T23:59:59.999`,
  };
}

/**
 * DateRange 转仅日期的 YYYY-MM-DD 闭区间。
 * Convert a DateRange to inclusive YYYY-MM-DD calendar days.
 */
export function dateRangeToYmdPair(range: DateRange): {startDate: string; endDate: string} {
  return {
    startDate: localDateToYmd(range.start),
    endDate: localDateToYmd(range.end),
  };
}

/**
 * 两个 YYYY-MM-DD 收成 DateRange；缺一端时用另一端。
 * Fold two YYYY-MM-DD values into a DateRange; a missing side copies the other.
 */
export function ymdPairToDateRange(startDate: string, endDate: string): DateRange | null {
  return wallTimesToDateRange(startDate, endDate);
}

const WALL_CLOCK = /^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?)?/;
const pad2 = (value: number) => String(value).padStart(2, '0');

/**
 * 解析墙上时间为本地 Date；只有日期时，开始取 00:00、结束取 23:59:59.999。
 * Parse a wall time into a local Date; a bare date maps to 00:00 for the start and 23:59:59.999 for the end.
 */
function wallTimeToLocalDateTime(value: string, side: 'start' | 'end'): Date | null {
  const match = WALL_CLOCK.exec((value || '').trim());
  if (!match) {
    return null;
  }
  const [, y, mo, d, h, mi, sec, ms] = match;
  if (h === undefined) {
    return side === 'start'
      ? new Date(Number(y), Number(mo) - 1, Number(d))
      : new Date(Number(y), Number(mo) - 1, Number(d), 23, 59, 59, 999);
  }
  return new Date(
    Number(y),
    Number(mo) - 1,
    Number(d),
    Number(h),
    Number(mi),
    Number(sec ?? 0),
    Number((ms ?? '0').padEnd(3, '0')),
  );
}

/**
 * 起止墙上时间收成带时刻的 DateRange（给开了 withTime 的选择器回显）。缺一端时用另一端补齐。
 * Fold start/end wall times into a DateRange that keeps the clock (for pickers with withTime). A missing side copies
 * the other.
 */
export function wallTimesToDateTimeRange(startTime: string, endTime: string): DateRange | null {
  const start = wallTimeToLocalDateTime(startTime || endTime, 'start');
  const end = wallTimeToLocalDateTime(endTime || startTime, 'end');
  if (!start || !end) {
    return null;
  }
  return start.getTime() <= end.getTime() ? {start, end} : {start: end, end: start};
}

/**
 * 带时刻的 DateRange 转墙上时间：开始 `YYYY-MM-DDTHH:mm`，结束带秒和毫秒（选择器给的是该分钟的 :59.999），
 * 这样 `created_at <= end_time` 不会丢掉最后一分钟。
 * Convert a timed DateRange to wall times: start is `YYYY-MM-DDTHH:mm`, end keeps seconds and milliseconds (the picker
 * hands over :59.999 of that minute) so `created_at <= end_time` keeps the last minute.
 */
export function dateTimeRangeToWallTimes(range: DateRange): {startTime: string; endTime: string} {
  const {start, end} = range;
  return {
    startTime: `${localDateToYmd(start)}T${pad2(start.getHours())}:${pad2(start.getMinutes())}`,
    endTime: `${localDateToYmd(end)}T${pad2(end.getHours())}:${pad2(end.getMinutes())}:${pad2(end.getSeconds())}.${String(end.getMilliseconds()).padStart(3, '0')}`,
  };
}
