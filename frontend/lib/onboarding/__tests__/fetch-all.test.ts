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

import {describe, expect, it, vi} from 'vitest';
import {
  classifyOnboardingFetchError,
  fetchAllPages,
  LIST_PAGE_SIZE_MAX,
} from '../fetch-all';

describe('LIST_PAGE_SIZE_MAX contract', () => {
  it('matches backend ListHostsRequest.Size max=100', () => {
    expect(LIST_PAGE_SIZE_MAX).toBe(100);
  });
});

describe('fetchAllPages', () => {
  it('never requests size above LIST_PAGE_SIZE_MAX even if asked', async () => {
    const seen: number[] = [];
    await fetchAllPages(
      async ({current, size}) => {
        seen.push(size);
        expect(size).toBeLessThanOrEqual(LIST_PAGE_SIZE_MAX);
        if (current === 1) {
          return {total: 1, items: [{id: 1}]};
        }
        return {total: 1, items: []};
      },
      {pageSize: 200},
    );
    expect(seen[0]).toBe(LIST_PAGE_SIZE_MAX);
  });

  it('pages until full coverage without silent truncate', async () => {
    const fetchPage = vi.fn(async ({current, size}: {current: number; size: number}) => {
      expect(size).toBe(LIST_PAGE_SIZE_MAX);
      if (current === 1) {
        return {
          total: 150,
          items: Array.from({length: 100}, (_, i) => ({id: i + 1})),
        };
      }
      if (current === 2) {
        return {
          total: 150,
          items: Array.from({length: 50}, (_, i) => ({id: 101 + i})),
        };
      }
      return {total: 150, items: []};
    });
    const all = await fetchAllPages(fetchPage);
    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(all).toHaveLength(150);
    expect(all[0]).toEqual({id: 1});
    expect(all[149]).toEqual({id: 150});
  });

  it('stops when a page returns empty even if total claims more', async () => {
    const all = await fetchAllPages(async ({current}) => {
      if (current === 1) {
        return {total: 500, items: [{id: 1}]};
      }
      return {total: 500, items: []};
    });
    expect(all).toHaveLength(1);
  });
});

describe('classifyOnboardingFetchError', () => {
  it('maps ListHostsRequest.Size validation to stable code', () => {
    expect(
      classifyOnboardingFetchError(
        new Error(
          "Key: 'ListHostsRequest.Size' Error:Field validation for 'Size' failed on the 'max' tag",
        ),
        'hosts_fetch_failed',
      ),
    ).toBe('hosts_page_size_invalid');
  });

  it('never returns raw validator prose', () => {
    const raw =
      "Key: 'ListHostsRequest.Size' Error:Field validation for 'Size' failed on the 'max' tag";
    const code = classifyOnboardingFetchError(raw, 'hosts_fetch_failed');
    expect(code).not.toContain('ListHostsRequest');
    expect(code).not.toContain('max');
  });

  it('falls back to provided code for unknown errors', () => {
    expect(
      classifyOnboardingFetchError(new Error('connection refused'), 'hosts_fetch_failed'),
    ).toBe('hosts_fetch_failed');
  });
});
