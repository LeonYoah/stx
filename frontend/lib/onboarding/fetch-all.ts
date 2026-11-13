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
 * Backend ListHostsRequest / ListClustersRequest Size binding: min=1,max=100.
 * Must never request above this — oversized pages fail validation (t3 defect).
 * 后端 Size 校验上限为 100；禁止超限单页，否则校验失败（t3 缺陷）。
 */
export const LIST_PAGE_SIZE_MAX = 100;

/** Safety cap on page loops / 翻页循环安全上限 */
const MAX_PAGES = 500;

export type PagedListResult<T> = {
  total: number;
  items: T[];
};

/**
 * Walk pages at a legal size until all items are collected (no silent truncate).
 * 以合规 page size 翻页拉全量，禁止静默截断。
 */
export async function fetchAllPages<T>(
  fetchPage: (args: {
    current: number;
    size: number;
  }) => Promise<PagedListResult<T>>,
  options?: {pageSize?: number},
): Promise<T[]> {
  const size = Math.min(
    Math.max(options?.pageSize ?? LIST_PAGE_SIZE_MAX, 1),
    LIST_PAGE_SIZE_MAX,
  );
  const first = await fetchPage({current: 1, size});
  const all = [...(first.items || [])];
  const total = Number.isFinite(first.total) ? first.total : all.length;
  let current = 2;
  while (all.length < total && current <= MAX_PAGES) {
    const page = await fetchPage({current, size});
    const batch = page.items || [];
    if (batch.length === 0) {
      break;
    }
    all.push(...batch);
    current += 1;
  }
  return all;
}

/**
 * Map thrown API/validation errors to stable onboarding error codes (never raw).
 * 将 API/校验异常映射为稳定错误码，禁止回传机器原文。
 */
export function classifyOnboardingFetchError(
  error: unknown,
  fallback: string,
): string {
  const raw =
    error instanceof Error
      ? error.message
      : typeof error === 'string'
        ? error
        : '';
  if (/ListHostsRequest\.Size|hosts.*Size.*max/i.test(raw)) {
    return 'hosts_page_size_invalid';
  }
  if (/ListClustersRequest\.Size|clusters.*Size.*max/i.test(raw)) {
    return 'clusters_page_size_invalid';
  }
  if (/Field validation for 'Size' failed on the 'max' tag/i.test(raw)) {
    return 'list_page_size_invalid';
  }
  // Already a stable snake_case code / 已是稳定错误码
  if (/^[a-z][a-z0-9_]*$/.test(raw)) {
    return raw;
  }
  // Never return raw validator / stack prose / 禁止回传校验器原文
  return fallback;
}
