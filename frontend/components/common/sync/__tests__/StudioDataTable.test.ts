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
  applyStudioSort,
  compareStudioValues,
  nextStudioSort,
} from '../StudioDataTable';

describe('StudioDataTable sort helpers', () => {
  it('cycles a column through the initial direction then the opposite', () => {
    const first = nextStudioSort(null, 'started', 'desc');
    expect(first).toEqual({key: 'started', direction: 'desc'});
    expect(nextStudioSort(first, 'started', 'desc')).toEqual({
      key: 'started',
      direction: 'asc',
    });
    expect(nextStudioSort(first, 'id')).toEqual({key: 'id', direction: 'asc'});
  });

  it('compares numbers, dates and empty values', () => {
    expect(compareStudioValues(2, 10)).toBeLessThan(0);
    expect(compareStudioValues('2026-01-02T00:00:00Z', '2026-01-01T00:00:00Z')).toBeGreaterThan(
      0,
    );
    expect(compareStudioValues('', 1)).toBeGreaterThan(0);
  });

  it('keeps original order until a descriptor is set', () => {
    const rows = [
      {id: 2, name: 'b'},
      {id: 1, name: 'a'},
    ];
    expect(applyStudioSort(rows, null, (row, key) => row[key as 'id'])).toEqual(rows);
    expect(
      applyStudioSort(rows, {key: 'id', direction: 'asc'}, (row, key) => row[key as 'id']).map(
        (row) => row.id,
      ),
    ).toEqual([1, 2]);
  });
});
