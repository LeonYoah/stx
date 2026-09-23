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
 * 全局集群偏好：唯一集群视为默认；多集群记住上次选择，供各处下拉自动带出。
 * Global cluster preference: sole cluster is default; remember last pick for auto-select.
 */

const PREFERRED_CLUSTER_STORAGE_KEY = 'stx:preferred-cluster-id';

/** 带数字 id 的集群项 / Cluster-like item with numeric id */
export type ClusterIdLike = {
  id: number;
};

export type ResolvePreferredClusterOptions = {
  /** 无偏好且多集群时是否回退到列表第一项 / Fall back to first item when no preference */
  fallbackToFirst?: boolean;
};

/**
 * 读取本地记住的偏好集群 ID。
 * Read remembered preferred cluster id from localStorage.
 */
export function getStoredPreferredClusterId(): number | null {
  if (typeof window === 'undefined') {
    return null;
  }
  try {
    const raw = window.localStorage.getItem(PREFERRED_CLUSTER_STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const parsed = Number.parseInt(raw, 10);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
  } catch {
    return null;
  }
}

/**
 * 记住用户选择的集群，供其他页面下拉默认使用。
 * Remember the cluster the user picked for other selectors.
 */
export function rememberPreferredClusterId(
  clusterId: number | string | null | undefined,
): void {
  if (typeof window === 'undefined') {
    return;
  }
  const parsed =
    typeof clusterId === 'number'
      ? clusterId
      : Number.parseInt(String(clusterId ?? ''), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return;
  }
  try {
    window.localStorage.setItem(PREFERRED_CLUSTER_STORAGE_KEY, String(parsed));
  } catch {
    // ignore quota / private mode
  }
}

/**
 * 清除记住的偏好集群。
 * Clear remembered preferred cluster id.
 */
export function clearPreferredClusterId(): void {
  if (typeof window === 'undefined') {
    return;
  }
  try {
    window.localStorage.removeItem(PREFERRED_CLUSTER_STORAGE_KEY);
  } catch {
    // ignore
  }
}

/**
 * 解析应自动选中的集群：唯一集群优先；否则用本地偏好；可选回退第一项。
 * Resolve auto-selected cluster: sole cluster first, then stored preference, optional first item.
 */
export function resolvePreferredClusterId(
  clusters: ClusterIdLike[] | null | undefined,
  options?: ResolvePreferredClusterOptions,
): number | null {
  const list = Array.isArray(clusters)
    ? clusters.filter((item) => item?.id > 0)
    : [];
  if (list.length === 0) {
    return null;
  }

  // 全局只有一个集群时，直接视为默认集群。
  // When only one cluster exists, treat it as the default.
  if (list.length === 1) {
    const soleId = list[0].id;
    rememberPreferredClusterId(soleId);
    return soleId;
  }

  const stored = getStoredPreferredClusterId();
  if (stored != null && list.some((item) => item.id === stored)) {
    return stored;
  }

  if (options?.fallbackToFirst) {
    return list[0].id;
  }
  return null;
}

/**
 * 当前集群是否为「唯一默认」展示标记。
 * Whether this cluster should show the sole-default badge.
 */
export function isSoleDefaultCluster(
  clusters: ClusterIdLike[] | null | undefined,
  clusterId: number,
  totalOverride?: number,
): boolean {
  const list = Array.isArray(clusters) ? clusters : [];
  const total =
    typeof totalOverride === 'number' && totalOverride >= 0
      ? totalOverride
      : list.length;
  return total === 1 && list.length === 1 && list[0]?.id === clusterId;
}

/**
 * 空选时填入偏好集群 ID 字符串（用于编辑器 / Select value）。
 * Fill empty selection with preferred cluster id string for editors / Selects.
 */
export function fillPreferredClusterId(
  currentId: string | number | null | undefined,
  clusters: ClusterIdLike[] | null | undefined,
  options?: ResolvePreferredClusterOptions,
): string {
  const current = String(currentId ?? '').trim();
  if (
    current &&
    current !== '0' &&
    current !== '__empty__' &&
    current !== 'all'
  ) {
    return current;
  }
  const preferred = resolvePreferredClusterId(clusters, options);
  return preferred != null ? String(preferred) : '';
}
