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
 * DateRange 转 datetime-local 墙上时间：开始日 00:00，结束日 23:59。
 * Convert a DateRange to datetime-local wall times: start day 00:00, end day 23:59.
 */
export function dateRangeToWallTimes(range: DateRange): {startTime: string; endTime: string} {
  return {
    startTime: `${localDateToYmd(range.start)}T00:00`,
    endTime: `${localDateToYmd(range.end)}T23:59`,
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
