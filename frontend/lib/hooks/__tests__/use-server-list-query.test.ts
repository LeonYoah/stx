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

import {act, renderHook} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {useServerListQuery} from '../use-server-list-query';

const replace = vi.fn();
let search = '';

vi.mock('next/navigation', () => ({
  useRouter: () => ({replace}),
  usePathname: () => '/hosts',
  useSearchParams: () => new URLSearchParams(search),
}));

describe('useServerListQuery', () => {
  beforeEach(() => {
    replace.mockReset();
    search = '';
  });

  it('patchFilters 回第 1 页', () => {
    const {result} = renderHook(() =>
      useServerListQuery({
        initialFilters: {status: 'all', q: ''},
        defaultPageSize: 10,
      }),
    );

    act(() => result.current.setPage(3));
    expect(result.current.page).toBe(3);

    act(() => result.current.patchFilters({status: 'connected'}));
    expect(result.current.page).toBe(1);
    expect(result.current.filters.status).toBe('connected');
  });

  it('setPageSize 回第 1 页', () => {
    const {result} = renderHook(() =>
      useServerListQuery({
        initialFilters: {status: 'all'},
        defaultPageSize: 10,
      }),
    );

    act(() => result.current.setPage(2));
    act(() => result.current.setPageSize(50));
    expect(result.current.page).toBe(1);
    expect(result.current.pageSize).toBe(50);
  });

  it('resetFilters 恢复初始筛选并回第 1 页', () => {
    const {result} = renderHook(() =>
      useServerListQuery({
        initialFilters: {status: 'all', q: ''},
        defaultPageSize: 10,
      }),
    );

    act(() => result.current.patchFilters({status: 'offline', q: 'node'}));
    act(() => result.current.setPage(2));
    act(() => result.current.resetFilters());
    expect(result.current.page).toBe(1);
    expect(result.current.filters).toEqual({status: 'all', q: ''});
  });

  it('urlSync 写入非默认 page / filter，省略 all 哨兵', () => {
    const {result} = renderHook(() =>
      useServerListQuery({
        initialFilters: {status: 'all', q: ''},
        defaultPageSize: 10,
        urlSync: {filterKeys: ['status', 'q']},
      }),
    );

    act(() => result.current.patchFilters({status: 'connected', q: 'web'}));
    expect(replace).toHaveBeenCalled();
    const href = String(replace.mock.calls.at(-1)?.[0]);
    expect(href).toContain('status=connected');
    expect(href).toContain('q=web');
    expect(href).not.toContain('page=');

    act(() => result.current.setPage(2));
    const href2 = String(replace.mock.calls.at(-1)?.[0]);
    expect(href2).toContain('page=2');
  });

  it('urlSync 从 URL 读取初始 page 与 filters', () => {
    search = 'page=3&status=pending&q=edge';
    const {result} = renderHook(() =>
      useServerListQuery({
        initialFilters: {status: 'all', q: ''},
        defaultPageSize: 10,
        urlSync: {filterKeys: ['status', 'q']},
      }),
    );
    expect(result.current.page).toBe(3);
    expect(result.current.filters.status).toBe('pending');
    expect(result.current.filters.q).toBe('edge');
  });
});
