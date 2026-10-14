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
  resolveDisplayTimezone,
  setUserTimezonePreference,
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
