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
import {
  dateRangeToWallTimes,
  dateRangeToYmdPair,
  localDateToYmd,
  wallTimeToYmd,
  wallTimesToDateRange,
  ymdPairToDateRange,
  ymdToLocalDate,
} from '../date-range';

describe('date-range helpers', () => {
  it('parses a wall-time string down to the calendar day', () => {
    expect(wallTimeToYmd('2026-11-01T08:15')).toBe('2026-11-01');
    expect(wallTimeToYmd('2026-11-01')).toBe('2026-11-01');
    expect(wallTimeToYmd('')).toBeNull();
  });

  it('round-trips YYYY-MM-DD through a local Date', () => {
    const date = ymdToLocalDate('2026-11-14');
    expect(date).not.toBeNull();
    expect(localDateToYmd(date!)).toBe('2026-11-14');
  });

  it('builds a range from wall times and expands it to start-of-day / end-of-day', () => {
    const range = wallTimesToDateRange('2026-11-01T08:00', '2026-11-03T19:30');
    expect(range).not.toBeNull();
    expect(dateRangeToWallTimes(range!)).toEqual({
      startTime: '2026-11-01T00:00',
      endTime: '2026-11-03T23:59',
    });
  });

  it('swaps inverted ranges and fills a missing side', () => {
    const inverted = wallTimesToDateRange('2026-11-10T00:00', '2026-11-01T00:00');
    expect(dateRangeToYmdPair(inverted!)).toEqual({
      startDate: '2026-11-01',
      endDate: '2026-11-10',
    });
    const oneSided = wallTimesToDateRange('2026-11-05T12:00', '');
    expect(dateRangeToYmdPair(oneSided!)).toEqual({
      startDate: '2026-11-05',
      endDate: '2026-11-05',
    });
  });

  it('treats YYYY-MM-DD pairs the same as wall times', () => {
    const range = ymdPairToDateRange('2026-03-01', '2026-03-08');
    expect(dateRangeToYmdPair(range!)).toEqual({
      startDate: '2026-03-01',
      endDate: '2026-03-08',
    });
    expect(ymdPairToDateRange('', '')).toBeNull();
  });
});
