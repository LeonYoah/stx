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
 * Cluster Management Hook
 * 集群管理 Hook
 *
 * Provides state management and data fetching for cluster management.
 * 提供集群管理的状态管理和数据获取功能。
 */

import {useState, useEffect, useCallback, useMemo} from 'react';
import services from '@/lib/services';
import {
  ClusterInfo,
  ClusterStatus,
  DeploymentMode,
  ClusterStatusInfo,
  NodeInfo,
  ListClustersRequest,
  ClusterListData,
} from '@/lib/services/cluster/types';

/**
 * Cluster filter state
 * 集群过滤状态
 */
export interface ClusterFilter {
  name?: string;
  status?: ClusterStatus;
  deploymentMode?: DeploymentMode;
}

/**
 * Cluster hook return type
 * 集群 Hook 返回类型
 */
interface UseClusterReturn {
  /** Cluster list / 集群列表 */
  clusters: ClusterInfo[];
  /** Total count / 总数量 */
  total: number;
  /** Loading state / 加载状态 */
  isLoading: boolean;
  /** Error state / 错误状态 */
  error: string | null;
  /** Current page / 当前页码 */
  currentPage: number;
  /** Page size / 每页数量 */
  pageSize: number;
  /** Total pages / 总页数 */
  totalPages: number;
  /** Current filter / 当前过滤条件 */
  filter: ClusterFilter;
  /** Set current page / 设置当前页码 */
  setCurrentPage: (page: number) => void;
  /** Set filter / 设置过滤条件 */
  setFilter: (filter: ClusterFilter) => void;
  /** Refresh data / 刷新数据 */
  refresh: () => Promise<void>;
  /** Clear filter / 清除过滤条件 */
  clearFilter: () => void;
}

/**
 * Cache structure
 * 缓存结构
 */
interface CachedData {
  data: ClusterListData;
  timestamp: number;
}

// Global cache - shared across all component instances
// 全局缓存 - 所有组件实例共享
const clusterCache = new Map<string, CachedData>();
const CACHE_DURATION = 30 * 1000; // 30 seconds cache duration / 30秒缓存过期时间

/**
 * Generate cache key from request params
 * 从请求参数生成缓存键
 */
function getCacheKey(params: ListClustersRequest): string {
  return JSON.stringify(params);
}

/**
 * Check if cache is valid
 * 检查缓存是否有效
 */
function isCacheValid(cachedData: CachedData): boolean {
  return Date.now() - cachedData.timestamp < CACHE_DURATION;
}

/**
 * Default page size
 * 默认每页数量
 */
const DEFAULT_PAGE_SIZE = 12;

/**
 * Cluster management hook
 * 集群管理 Hook
 *
 * @param initialPageSize - Initial page size / 初始每页数量
 * @returns Cluster management state and methods / 集群管理状态和方法
 */
export function useCluster(initialPageSize: number = DEFAULT_PAGE_SIZE): UseClusterReturn {
  // State / 状态
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(initialPageSize);
  const [filter, setFilter] = useState<ClusterFilter>({});

  /**
   * Calculate total pages
   * 计算总页数
   */
  const totalPages = useMemo(() => Math.ceil(total / pageSize), [total, pageSize]);

  /**
   * Build request params from current state
   * 从当前状态构建请求参数
   */
  const buildRequestParams = useCallback((): ListClustersRequest => {
    return {
      current: currentPage,
      size: pageSize,
      name: filter.name,
      status: filter.status,
      deployment_mode: filter.deploymentMode,
    };
  }, [currentPage, pageSize, filter]);

  /**
   * Fetch clusters data
   * 获取集群数据
   */
  const fetchClusters = useCallback(
    async (forceRefresh = false) => {
      const params = buildRequestParams();
      const cacheKey = getCacheKey(params);

      // Check cache first (unless force refresh)
      // 首先检查缓存（除非强制刷新）
      if (!forceRefresh) {
        const cached = clusterCache.get(cacheKey);
        if (cached && isCacheValid(cached)) {
          setClusters(cached.data.clusters);
          setTotal(cached.data.total);
          setIsLoading(false);
          setError(null);
          return;
        }
      }

      setIsLoading(true);

      try {
        const result = await services.cluster.getClustersSafe(params);

        if (result.success && result.data) {
          setClusters(result.data.clusters);
          setTotal(result.data.total);
          setError(null);

          // Update cache / 更新缓存
          clusterCache.set(cacheKey, {
            data: result.data,
            timestamp: Date.now(),
          });
        } else {
          // 失败时保留旧列表，避免重试落到空态。Keep prior rows so retries do not become an empty state.
          setError(result.error || '获取集群列表失败');
        }
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : '获取集群列表失败';
        setError(errorMessage);
      } finally {
        setIsLoading(false);
      }
    },
    [buildRequestParams],
  );

  /**
   * Refresh data (force refresh, ignore cache)
   * 刷新数据（强制刷新，忽略缓存）
   */
  const refresh = useCallback(async () => {
    // Clear all cache when refreshing
    // 刷新时清除所有缓存
    clusterCache.clear();
    await fetchClusters(true);
  }, [fetchClusters]);

  /**
   * Clear filter
   * 清除过滤条件
   */
  const clearFilter = useCallback(() => {
    setFilter({});
    setCurrentPage(1);
  }, []);

  /**
   * Handle filter change - reset to page 1
   * 处理过滤条件变化 - 重置到第1页
   */
  const handleSetFilter = useCallback((newFilter: ClusterFilter) => {
    setFilter(newFilter);
    setCurrentPage(1);
  }, []);

  /**
   * Fetch data when dependencies change
   * 当依赖变化时获取数据
   */
  useEffect(() => {
    fetchClusters();
  }, [fetchClusters]);

  return {
    clusters,
    total,
    isLoading,
    error,
    currentPage,
    pageSize,
    totalPages,
    filter,
    setCurrentPage,
    setFilter: handleSetFilter,
    refresh,
    clearFilter,
  };
}

/**
 * Single cluster hook return type
 * 单个集群 Hook 返回类型
 */
interface UseClusterDetailReturn {
  /** Cluster data / 集群数据 */
  cluster: ClusterInfo | null;
  /** Cluster status info / 集群状态信息 */
  statusInfo: ClusterStatusInfo | null;
  /** Cluster nodes / 集群节点 */
  nodes: NodeInfo[];
  /** Loading state / 加载状态 */
  isLoading: boolean;
  /** Error state / 错误状态 */
  error: string | null;
  /** Refresh data / 刷新数据 */
  refresh: () => Promise<void>;
  /** Refresh nodes / 刷新节点 */
  refreshNodes: () => Promise<void>;
  /** Refresh status / 刷新状态 */
  refreshStatus: () => Promise<void>;
}

/**
 * Single cluster detail hook
 * 单个集群详情 Hook
 *
 * @param clusterId - Cluster ID / 集群 ID
 * @returns Cluster detail state and methods / 集群详情状态和方法
 */
export function useClusterDetail(clusterId: number | null): UseClusterDetailReturn {
  const [cluster, setCluster] = useState<ClusterInfo | null>(null);
  const [statusInfo, setStatusInfo] = useState<ClusterStatusInfo | null>(null);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  /**
   * Fetch cluster detail
   * 获取集群详情
   */
  const fetchCluster = useCallback(async () => {
    if (!clusterId) {
      setCluster(null);
      return;
    }

    try {
      const result = await services.cluster.getClusterSafe(clusterId);

      if (result.success && result.data) {
        setCluster(result.data);
      } else {
        setError(result.error || '获取集群详情失败');
        setCluster(null);
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : '获取集群详情失败';
      setError(errorMessage);
      setCluster(null);
    }
  }, [clusterId]);

  /**
   * Fetch cluster nodes
   * 获取集群节点
   */
  const fetchNodes = useCallback(async () => {
    if (!clusterId) {
      setNodes([]);
      return;
    }

    try {
      const result = await services.cluster.getNodesSafe(clusterId);

      if (result.success && result.data) {
        setNodes(result.data);
      } else {
        setNodes([]);
      }
    } catch {
      setNodes([]);
    }
  }, [clusterId]);

  /**
   * Fetch cluster status
   * 获取集群状态
   */
  const fetchStatus = useCallback(async () => {
    if (!clusterId) {
      setStatusInfo(null);
      return;
    }

    try {
      const result = await services.cluster.getClusterStatusSafe(clusterId);

      if (result.success && result.data) {
        setStatusInfo(result.data);
      } else {
        setStatusInfo(null);
      }
    } catch {
      setStatusInfo(null);
    }
  }, [clusterId]);

  /**
   * Fetch all data
   * 获取所有数据
   */
  const fetchAll = useCallback(async () => {
    if (!clusterId) {
      setCluster(null);
      setNodes([]);
      setStatusInfo(null);
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      await Promise.all([fetchCluster(), fetchNodes(), fetchStatus()]);
    } finally {
      setIsLoading(false);
    }
  }, [clusterId, fetchCluster, fetchNodes, fetchStatus]);

  /**
   * Refresh all data
   * 刷新所有数据
   */
  const refresh = useCallback(async () => {
    await fetchAll();
  }, [fetchAll]);

  /**
   * Refresh nodes only
   * 仅刷新节点
   */
  const refreshNodes = useCallback(async () => {
    await fetchNodes();
  }, [fetchNodes]);

  /**
   * Refresh status only
   * 仅刷新状态
   */
  const refreshStatus = useCallback(async () => {
    await fetchStatus();
  }, [fetchStatus]);

  /**
   * Fetch data when clusterId changes
   * 当 clusterId 变化时获取数据
   */
  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  return {
    cluster,
    statusInfo,
    nodes,
    isLoading,
    error,
    refresh,
    refreshNodes,
    refreshStatus,
  };
}

/**
 * Clear cluster cache
 * 清除集群缓存
 */
export function clearClusterCache(): void {
  clusterCache.clear();
}
