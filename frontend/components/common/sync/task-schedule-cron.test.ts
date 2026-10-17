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

import {cronToText, getNextRuns, parseCron} from './task-schedule-cron';

const t = (key: string, values?: Record<string, string | number>) => {
  switch (key) {
    case 'cronEveryMinute': return 'Every minute';
    case 'cronEveryNMinutes': return `Every ${values?.count} minutes`;
    case 'cronEveryNMinutesDuringHour': return `Every ${values?.count} minutes during hour ${values?.hour}:00`;
    case 'cronEveryHour': return 'Every hour';
    case 'cronEveryDayAtMidnight': return 'Every day at 00:00';
    case 'cronWeekdaysAtNine': return 'Weekdays at 09:00';
    case 'cronWeekdaysAtEightThirty': return 'Weekdays at 08:30';
    case 'cronWeeklySunday': return 'Every Sunday at 00:00';
    case 'cronMonthlyFirstDayNine': return 'On the 1st day of every month at 09:00';
    case 'cronQuarterlyFirstDay': return 'Quarterly on day 1 at 00:00';
    case 'cronAtTime': return `At ${values?.hour}:${values?.minute}`;
    case 'cronRunsOnExpr': return `Runs on ${values?.expr}`;
    case 'invalidCronExpression': return 'Invalid cron expression';
    default: return key;
  }
};

describe('task schedule cron', () => {
  it('describes */10 1 * * * without NaN', () => {
    expect(cronToText('*/10 1 * * *', t)).toBe('Every 10 minutes during hour 01:00');
  });

  it('describes fixed daily time', () => {
    expect(cronToText('30 8 * * *', t)).toBe('At 08:30');
  });

  it('supports named weekdays', () => {
    const parsed = parseCron('0 9 * * MON-FRI');
    expect(parsed.expanded.dayOfWeek).toEqual([1, 2, 3, 4, 5]);
  });

  it('returns next runs for complex expression', () => {
    const base = new Date('2026-03-29T00:55:00+08:00');
    const runs = getNextRuns('*/10 1 * * *', 3, base);
    expect(runs).toHaveLength(3);
    expect(runs[0].getHours()).toBe(1);
    expect(runs[0].getMinutes()).toBe(0);
    expect(runs[1].getMinutes()).toBe(10);
    expect(runs[2].getMinutes()).toBe(20);
  });


  it('describes quarterly expression with preset text', () => {
    expect(cronToText('0 0 1 */3 *', t)).toBe('Quarterly on day 1 at 00:00');
  });

  it('supports named months', () => {
    const parsed = parseCron('0 9 1 JAN,MAR,DEC *');
    expect(parsed.expanded.month).toEqual([1, 3, 12]);
  });

  it('treats 7 as sunday in day of week', () => {
    const parsed = parseCron('0 0 * * 7');
    expect(parsed.expanded.dayOfWeek).toEqual([0]);
  });

  it('handles dom and dow together with OR semantics', () => {
    const runs = getNextRuns('0 9 1 * MON', 4, new Date('2026-03-30T08:00:00+08:00'));
    expect(runs).toHaveLength(4);
    expect(runs.every((run) => run.getHours() === 9 && run.getMinutes() === 0)).toBe(true);
    expect(runs.map((run) => ({month: run.getMonth() + 1, date: run.getDate(), day: run.getDay()}))).toEqual([
      {month: 3, date: 30, day: 1},
      {month: 4, date: 1, day: 3},
      {month: 4, date: 6, day: 1},
      {month: 4, date: 13, day: 1},
    ]);
  });

  it('supports list range and step combinations', () => {
    const parsed = parseCron('5,10-12,20-30/5 8 * * *');
    expect(parsed.expanded.minute).toEqual([5, 10, 11, 12, 20, 25, 30]);
  });

  it('supports named month ranges and weekday ranges', () => {
    const parsed = parseCron('0 18 * JAN-MAR MON-WED');
    expect(parsed.expanded.month).toEqual([1, 2, 3]);
    expect(parsed.expanded.dayOfWeek).toEqual([1, 2, 3]);
  });

  it('rejects out-of-bounds values', () => {
    expect(() => parseCron('61 0 * * *')).toThrow(/Out of bounds/);
  });


  it('rejects invalid 6-field cron', () => {
    expect(() => parseCron('0 0 1 * * *')).toThrow(/5-field/);
  });
});

describe('getNextRuns timezone', () => {
  it('evaluates cron in the task timezone, independent of the browser zone', () => {
    const from = new Date('2026-10-14T07:00:00Z');
    const shanghai = getNextRuns('30 15 * * *', 2, from, 'Asia/Shanghai');
    expect(shanghai.map((d) => d.toISOString())).toEqual(['2026-10-14T07:30:00.000Z', '2026-10-15T07:30:00.000Z']);
    const newYork = getNextRuns('30 3 * * *', 1, from, 'America/New_York');
    expect(newYork[0].toISOString()).toBe('2026-10-14T07:30:00.000Z');
    const utc = getNextRuns('0 0 * * MON', 1, from, 'UTC');
    expect(utc[0].toISOString()).toBe('2026-10-19T00:00:00.000Z');
  });
});
