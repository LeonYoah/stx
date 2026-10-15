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

import {afterEach, describe, expect, it} from 'vitest';
import {
  formatDateTimeTz,
  formatDateTz,
  formatTimeTz,
  isValidTimezone,
  listTimezones,
  parseTime,
  formatDateTimeLocalInput,
  resolveCronTimezone,
  resolveDisplayTimezone,
  setSystemTimezone,
  setUserTimezonePreference,
  wallTimeToRFC3339,
  zonedWallTimeToDate,
} from '../timezone';

describe('timezone formatter', () => {
  afterEach(() => setUserTimezonePreference(''));

  it('formats UTC instants in the requested timezone', () => {
    expect(formatDateTimeTz('2026-10-14T05:31:03Z', {timeZone: 'Asia/Shanghai'})).toBe('2026/10/14 13:31:03');
    expect(formatDateTimeTz('2026-10-14T05:31:03Z', {timeZone: 'UTC'})).toBe('2026/10/14 05:31:03');
    expect(formatDateTz('2026-10-14T20:00:00Z', {timeZone: 'Asia/Shanghai'})).toBe('2026/10/15');
    expect(formatTimeTz('2026-10-14T05:31:03+08:00', {timeZone: 'UTC'})).toBe('21:31:03');
  });

  it('uses the user preference and falls back to the browser', () => {
    setUserTimezonePreference('America/New_York');
    expect(resolveDisplayTimezone()).toBe('America/New_York');
    expect(formatDateTimeTz('2026-01-01T12:00:00Z')).toBe('2026/01/01 07:00:00');
    setUserTimezonePreference('Invalid/Zone');
    expect(resolveDisplayTimezone()).toBe(new Intl.DateTimeFormat().resolvedOptions().timeZone);
  });

  it('treats legacy naive strings as UTC and handles empty values', () => {
    expect(parseTime('2026-10-14 05:31:03')?.toISOString()).toBe('2026-10-14T05:31:03.000Z');
    expect(formatDateTimeTz('', {fallback: '—'})).toBe('—');
    expect(formatDateTimeTz('0001-01-01T00:00:00Z')).toBe('-');
    expect(formatDateTimeTz('garbage')).toBe('-');
  });

  it('validates and lists timezones', () => {
    expect(isValidTimezone('Asia/Shanghai')).toBe(true);
    expect(isValidTimezone('Mars/Base')).toBe(false);
    expect(listTimezones()).toContain('UTC');
  });
});

describe('wall-time inputs', () => {
  afterEach(() => {
    setUserTimezonePreference('');
    setSystemTimezone('');
  });

  it('interprets datetime-local values in the explicit or user timezone, not the browser', () => {
    expect(wallTimeToRFC3339('2026-10-14T15:30', 'Asia/Shanghai')).toBe('2026-10-14T07:30:00.000Z');
    setUserTimezonePreference('America/New_York');
    expect(wallTimeToRFC3339('2026-10-14T03:30')).toBe('2026-10-14T07:30:00.000Z');
    expect(wallTimeToRFC3339('2026-10-14T23:59:59.999')).toBe('2026-10-15T03:59:59.999Z');
    expect(wallTimeToRFC3339('2026-10-14T00:00:00')).toBe('2026-10-14T04:00:00.000Z');
    expect(wallTimeToRFC3339('garbage')).toBeUndefined();
  });

  it('handles DST transitions', () => {
    // 2026-03-08 New York 跳过 02:00-03:00 / spring forward
    expect(zonedWallTimeToDate('2026-03-08T01:30', 'America/New_York')?.toISOString()).toBe('2026-03-08T06:30:00.000Z');
    expect(zonedWallTimeToDate('2026-03-08T03:30', 'America/New_York')?.toISOString()).toBe('2026-03-08T07:30:00.000Z');
    expect(zonedWallTimeToDate('2026-11-01T12:00', 'America/New_York')?.toISOString()).toBe('2026-11-01T17:00:00.000Z');
  });

  it('formats instants as datetime-local values in the user timezone and round-trips', () => {
    const instant = new Date('2026-10-14T07:30:00Z');
    expect(formatDateTimeLocalInput(instant, 'Asia/Shanghai')).toBe('2026-10-14T15:30');
    setUserTimezonePreference('Europe/London');
    const local = formatDateTimeLocalInput(instant);
    expect(local).toBe('2026-10-14T08:30');
    expect(wallTimeToRFC3339(local)).toBe('2026-10-14T07:30:00.000Z');
  });

  it('cron timezone falls back to the system display timezone before the browser', () => {
    expect(resolveCronTimezone('America/New_York')).toBe('America/New_York');
    expect(resolveCronTimezone('')).toBe(new Intl.DateTimeFormat().resolvedOptions().timeZone);
    setSystemTimezone('Asia/Tokyo');
    expect(resolveCronTimezone('')).toBe('Asia/Tokyo');
    expect(resolveCronTimezone('Bad/Zone')).toBe('Asia/Tokyo');
  });
});
