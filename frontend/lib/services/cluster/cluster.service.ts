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
 * Cluster Service
 * 集群服务
 *
 * Handles cluster management operations including CRUD, node management, and cluster operations.
 * 处理集群管理操作，包括 CRUD、节点管理和集群操作。
 */

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import {localizeBackendText} from '@/lib/i18n/localize-text';
import {
  ClusterInfo,
  ClusterListData,
  ClusterStatusInfo,
  NodeInfo,
  OperationResult,
  CreateClusterRequest,
  UpdateClusterRequest,
  AddNodeRequest,
  AddNodesRequest,
  UpdateNodeRequest,
  PrecheckRequest,
  PrecheckResult,
  ListClustersRequest,
  ListClustersResponse,
  CreateClusterResponse,
  GetClusterResponse,
  UpdateClusterResponse,
  DeleteClusterResponse,
  GetNodesResponse,
  AddNodeResponse,
  AddNodesResponse,
  RemoveNodeResponse,
  ClusterOperationResponse,
  GetClusterStatusResponse,
  GetRuntimeStorageResponse,
  CleanupRuntimeStorageResponse,
  ValidateRuntimeStorageResponse,
  ListRuntimeStorageResponse,
  PreviewRuntimeStorageResponse,
  InspectCheckpointRuntimeStorageResponse,
  InspectIMAPRuntimeStorageResponse,
  PrecheckNodeResponse,
  RuntimeStorageDetails,
  RuntimeStorageCleanupResult,
  RuntimeStorageValidationResult,
  ApplyRuntimeStorageResult,
  RuntimeStorageListResult,
  RuntimeStoragePreviewResult,
  RuntimeStorageCheckpointInspectResult,
  RuntimeStorageCheckpointInspectJobConfig,
  RuntimeStorageIMAPInspectResult,
  StxJavaProxyLogPreviewResponse,
  StxJavaProxyLogPreviewResult,
  StxJavaProxyResponse,
  StxJavaProxyStatus,
  UpdateStxJavaProxyConfigRequest,
} from './types';

/**
 * Cluster Service class
 * 集群服务类
 */
export class ClusterService extends BaseService {
  /**
   * API base path
   * API 基础路径
   */
  protected static readonly basePath = '/clusters';

  // ==================== Cluster CRUD Methods 集群 CRUD 方法 ====================

  /**
   * Get cluster list with filtering and pagination
   * 获取集群列表（支持过滤和分页）
   *
   * @param params - Query parameters / 查询参数
   * @returns Cluster list data / 集群列表数据
   */
  static async getClusters(
    params: ListClustersRequest,
  ): Promise<ClusterListData> {
    const response = await apiClient.get<ListClustersResponse>(this.basePath, {
      params: {
        current: params.current,
        size: params.size,
        name: params.name,
        status: params.status,
        deployment_mode: params.deployment_mode,
      },
    });

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Get cluster by ID
   * 根据 ID 获取集群详情
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Cluster information / 集群信息
   */
  static async getCluster(clusterId: number): Promise<ClusterInfo> {
    const response = await apiClient.get<GetClusterResponse>(
      `${this.basePath}/${clusterId}`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Create a new cluster
   * 创建新集群
   *
   * @param data - Cluster creation data / 集群创建数据
   * @returns Created cluster information / 创建的集群信息
   */
  static async createCluster(data: CreateClusterRequest): Promise<ClusterInfo> {
    const response = await apiClient.post<CreateClusterResponse>(
      this.basePath,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Update an existing cluster
   * 更新现有集群
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param data - Cluster update data / 集群更新数据
   * @returns Updated cluster information / 更新后的集群信息
   */
  static async updateCluster(
    clusterId: number,
    data: UpdateClusterRequest,
  ): Promise<ClusterInfo> {
    const response = await apiClient.put<UpdateClusterResponse>(
      `${this.basePath}/${clusterId}`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Delete a cluster
   * 删除集群
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param options - forceDelete: if true, notify agents to remove install dir on hosts
   */
  static async deleteCluster(
    clusterId: number,
    options?: {forceDelete?: boolean},
  ): Promise<void> {
    const params =
      options?.forceDelete === true ? {force_delete: '1'} : undefined;
    const response = await apiClient.delete<DeleteClusterResponse>(
      `${this.basePath}/${clusterId}`,
      {params},
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
  }

  // ==================== Node Management Methods 节点管理方法 ====================

  /**
   * Get all nodes for a cluster
   * 获取集群的所有节点
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Node list / 节点列表
   */
  static async getNodes(clusterId: number): Promise<NodeInfo[]> {
    const response = await apiClient.get<GetNodesResponse>(
      `${this.basePath}/${clusterId}/nodes`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Add a node to a cluster
   * 向集群添加节点
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param data - Add node request data / 添加节点请求数据
   * @returns Added node information / 添加的节点信息
   */
  static async addNode(
    clusterId: number,
    data: AddNodeRequest,
  ): Promise<NodeInfo> {
    const response = await apiClient.post<AddNodeResponse>(
      `${this.basePath}/${clusterId}/nodes`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Add multiple logical nodes for the same host atomically
   * 为同一主机原子添加多个逻辑节点
   */
  static async addNodes(
    clusterId: number,
    data: AddNodesRequest,
  ): Promise<NodeInfo[]> {
    const response = await apiClient.post<AddNodesResponse>(
      `${this.basePath}/${clusterId}/nodes/batch`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Update a node in a cluster
   * 更新集群中的节点
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param nodeId - Node ID / 节点 ID
   * @param data - Update node request data / 更新节点请求数据
   * @returns Updated node information / 更新后的节点信息
   */
  static async updateNode(
    clusterId: number,
    nodeId: number,
    data: UpdateNodeRequest,
  ): Promise<NodeInfo> {
    const response = await apiClient.put<AddNodeResponse>(
      `${this.basePath}/${clusterId}/nodes/${nodeId}`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Remove a node from a cluster
   * 从集群移除节点
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param nodeId - Node ID / 节点 ID
   */
  static async removeNode(clusterId: number, nodeId: number): Promise<void> {
    const response = await apiClient.delete<RemoveNodeResponse>(
      `${this.basePath}/${clusterId}/nodes/${nodeId}`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
  }

  /**
   * Precheck a node before adding to cluster
   * 添加节点前的预检查
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param data - Precheck request data / 预检查请求数据
   * @returns Precheck result / 预检查结果
   */
  static async precheckNode(
    clusterId: number,
    data: PrecheckRequest,
  ): Promise<PrecheckResult> {
    const response = await apiClient.post<PrecheckNodeResponse>(
      `${this.basePath}/${clusterId}/nodes/precheck`,
      data,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  // ==================== Cluster Operation Methods 集群操作方法 ====================

  /**
   * Start a cluster
   * 启动集群
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Operation result / 操作结果
   */
  static async startCluster(clusterId: number): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/start`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Stop a cluster
   * 停止集群
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Operation result / 操作结果
   */
  static async stopCluster(clusterId: number): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/stop`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Restart a cluster
   * 重启集群
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Operation result / 操作结果
   */
  static async restartCluster(clusterId: number): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/restart`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Get cluster status
   * 获取集群状态
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @returns Cluster status information / 集群状态信息
   */
  static async getClusterStatus(clusterId: number): Promise<ClusterStatusInfo> {
    const response = await apiClient.get<GetClusterStatusResponse>(
      `${this.basePath}/${clusterId}/status`,
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    return response.data.data;
  }

  /**
   * Get runtime storage details
   * 获取运行时存储详情
   */
  static async getRuntimeStorage(
    clusterId: number,
  ): Promise<RuntimeStorageDetails> {
    const response = await apiClient.get<GetRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage`,
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    const data = response.data.data;
    return {
      ...data,
      checkpoint: data.checkpoint
        ? {
            ...data.checkpoint,
            warning: localizeBackendText(data.checkpoint.warning),
            nodes: Array.isArray(data.checkpoint.nodes)
              ? data.checkpoint.nodes.map((node) => ({
                  ...node,
                  message: localizeBackendText(node.message),
                }))
              : [],
          }
        : data.checkpoint,
      imap: data.imap
        ? {
            ...data.imap,
            warning: localizeBackendText(data.imap.warning),
            nodes: Array.isArray(data.imap.nodes)
              ? data.imap.nodes.map((node) => ({
                  ...node,
                  message: localizeBackendText(node.message),
                }))
              : [],
          }
        : data.imap,
    };
  }

  /**
   * Cleanup local IMAP storage
   * 清理本地 IMAP 存储
   */
  static async cleanupIMAPStorage(
    clusterId: number,
  ): Promise<RuntimeStorageCleanupResult> {
    const response = await apiClient.post<CleanupRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage/imap/cleanup`,
      {},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    const data = response.data.data;
    return {
      ...data,
      message: localizeBackendText(data.message),
      nodes: Array.isArray(data.nodes)
        ? data.nodes.map((node) => ({
            ...node,
            message: localizeBackendText(node.message),
          }))
        : [],
    };
  }

  static async listRuntimeStorage(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
    params?: {path?: string; recursive?: boolean; limit?: number},
  ): Promise<RuntimeStorageListResult> {
    const response = await apiClient.post<ListRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage/${kind}/list`,
      {
        path: params?.path,
        recursive: params?.recursive ?? false,
        limit: params?.limit ?? 200,
      },
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    const data = response.data.data;
    return {
      ...data,
      items: Array.isArray(data.items)
        ? data.items.map((item) => ({
            ...item,
            path: localizeBackendText(item.path),
            name: localizeBackendText(item.name),
          }))
        : [],
    };
  }

  static async previewRuntimeStorage(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
    params: {path: string; max_bytes?: number},
  ): Promise<RuntimeStoragePreviewResult> {
    const response = await apiClient.post<PreviewRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage/${kind}/preview`,
      params,
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  static async inspectCheckpointRuntimeStorage(
    clusterId: number,
    params: {
      path: string;
      job_config?: RuntimeStorageCheckpointInspectJobConfig;
    },
  ): Promise<RuntimeStorageCheckpointInspectResult> {
    const response =
      await apiClient.post<InspectCheckpointRuntimeStorageResponse>(
        `${this.basePath}/${clusterId}/runtime-storage/checkpoint/inspect`,
        params,
      );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  static async inspectIMAPRuntimeStorage(
    clusterId: number,
    path: string,
  ): Promise<RuntimeStorageIMAPInspectResult> {
    const response = await apiClient.post<InspectIMAPRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage/imap/inspect`,
      {path},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  static async validateRuntimeStorage(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
  ): Promise<RuntimeStorageValidationResult> {
    const response = await apiClient.post<ValidateRuntimeStorageResponse>(
      `${this.basePath}/${clusterId}/runtime-storage/${kind}/validate`,
      {},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    const data = response.data.data;
    return {
      ...data,
      warning: localizeBackendText(data.warning),
      hosts: Array.isArray(data.hosts)
        ? data.hosts.map((host) => ({
            ...host,
            message: localizeBackendText(host.message),
          }))
        : [],
    };
  }

  static async getStxJavaProxyStatus(
    clusterId: number,
  ): Promise<StxJavaProxyStatus> {
    const response = await apiClient.get<StxJavaProxyResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/status`,
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return {
      ...response.data.data,
      message: localizeBackendText(response.data.data.message),
    };
  }

  static async startStxJavaProxy(
    clusterId: number,
  ): Promise<StxJavaProxyStatus> {
    const response = await apiClient.post<StxJavaProxyResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/start`,
      {},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return {
      ...response.data.data,
      message: localizeBackendText(response.data.data.message),
    };
  }

  static async stopStxJavaProxy(
    clusterId: number,
  ): Promise<StxJavaProxyStatus> {
    const response = await apiClient.post<StxJavaProxyResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/stop`,
      {},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return {
      ...response.data.data,
      message: localizeBackendText(response.data.data.message),
    };
  }

  static async restartStxJavaProxy(
    clusterId: number,
  ): Promise<StxJavaProxyStatus> {
    const response = await apiClient.post<StxJavaProxyResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/restart`,
      {},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return {
      ...response.data.data,
      message: localizeBackendText(response.data.data.message),
    };
  }

  static async updateStxJavaProxyConfig(
    clusterId: number,
    data: UpdateStxJavaProxyConfigRequest,
  ): Promise<StxJavaProxyStatus> {
    const response = await apiClient.put<StxJavaProxyResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/config`,
      data,
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return {
      ...response.data.data,
      message: localizeBackendText(response.data.data.message),
    };
  }

  static async previewStxJavaProxyServiceLog(
    clusterId: number,
    params?: {lines?: number},
  ): Promise<StxJavaProxyLogPreviewResult> {
    const response = await apiClient.get<StxJavaProxyLogPreviewResponse>(
      `${this.basePath}/${clusterId}/stx-java-proxy/logs`,
      {params},
    );
    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }
    return response.data.data;
  }

  // ==================== Safe Methods (with error handling) 安全方法（带错误处理） ====================

  /**
   * Get cluster list (with error handling)
   * 获取集群列表（带错误处理）
   */
  static async getClustersSafe(params: ListClustersRequest): Promise<{
    success: boolean;
    data?: ClusterListData;
    error?: string;
  }> {
    try {
      const data = await this.getClusters(params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取集群列表失败';
      return {
        success: false,
        data: {total: 0, clusters: []},
        error: errorMessage,
      };
    }
  }

  /**
   * Get cluster by ID (with error handling)
   * 根据 ID 获取集群详情（带错误处理）
   */
  static async getClusterSafe(clusterId: number): Promise<{
    success: boolean;
    data?: ClusterInfo;
    error?: string;
  }> {
    try {
      const data = await this.getCluster(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取集群详情失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Create a new cluster (with error handling)
   * 创建新集群（带错误处理）
   */
  static async createClusterSafe(data: CreateClusterRequest): Promise<{
    success: boolean;
    data?: ClusterInfo;
    error?: string;
  }> {
    try {
      const result = await this.createCluster(data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '创建集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Update an existing cluster (with error handling)
   * 更新现有集群（带错误处理）
   */
  static async updateClusterSafe(
    clusterId: number,
    data: UpdateClusterRequest,
  ): Promise<{
    success: boolean;
    data?: ClusterInfo;
    error?: string;
  }> {
    try {
      const result = await this.updateCluster(clusterId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '更新集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Delete a cluster (with error handling)
   * 删除集群（带错误处理）
   */
  static async deleteClusterSafe(
    clusterId: number,
    options?: {forceDelete?: boolean},
  ): Promise<{
    success: boolean;
    error?: string;
  }> {
    try {
      await this.deleteCluster(clusterId, options);
      return {success: true};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '删除集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Get nodes (with error handling)
   * 获取节点列表（带错误处理）
   */
  static async getNodesSafe(clusterId: number): Promise<{
    success: boolean;
    data?: NodeInfo[];
    error?: string;
  }> {
    try {
      const data = await this.getNodes(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取节点列表失败';
      return {success: false, data: [], error: errorMessage};
    }
  }

  /**
   * Add node (with error handling)
   * 添加节点（带错误处理）
   */
  static async addNodeSafe(
    clusterId: number,
    data: AddNodeRequest,
  ): Promise<{
    success: boolean;
    data?: NodeInfo;
    error?: string;
  }> {
    try {
      const result = await this.addNode(clusterId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '添加节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Add multiple logical nodes (with error handling)
   * 批量添加逻辑节点（带错误处理）
   */
  static async addNodesSafe(
    clusterId: number,
    data: AddNodesRequest,
  ): Promise<{
    success: boolean;
    data?: NodeInfo[];
    error?: string;
  }> {
    try {
      const result = await this.addNodes(clusterId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '批量添加节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Update node (with error handling)
   * 更新节点（带错误处理）
   */
  static async updateNodeSafe(
    clusterId: number,
    nodeId: number,
    data: UpdateNodeRequest,
  ): Promise<{
    success: boolean;
    data?: NodeInfo;
    error?: string;
  }> {
    try {
      const result = await this.updateNode(clusterId, nodeId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '更新节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Remove node (with error handling)
   * 移除节点（带错误处理）
   */
  static async removeNodeSafe(
    clusterId: number,
    nodeId: number,
  ): Promise<{
    success: boolean;
    error?: string;
  }> {
    try {
      await this.removeNode(clusterId, nodeId);
      return {success: true};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '移除节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Precheck node (with error handling)
   * 节点预检查（带错误处理）
   */
  static async precheckNodeSafe(
    clusterId: number,
    data: PrecheckRequest,
  ): Promise<{
    success: boolean;
    data?: PrecheckResult;
    error?: string;
  }> {
    try {
      const result = await this.precheckNode(clusterId, data);
      return {success: true, data: result};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '节点预检查失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Start cluster (with error handling)
   * 启动集群（带错误处理）
   */
  static async startClusterSafe(clusterId: number): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.startCluster(clusterId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '启动集群失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '启动集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Stop cluster (with error handling)
   * 停止集群（带错误处理）
   */
  static async stopClusterSafe(clusterId: number): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.stopCluster(clusterId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '停止集群失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '停止集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Restart cluster (with error handling)
   * 重启集群（带错误处理）
   */
  static async restartClusterSafe(clusterId: number): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.restartCluster(clusterId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '重启集群失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '重启集群失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Get cluster status (with error handling)
   * 获取集群状态（带错误处理）
   */
  static async getClusterStatusSafe(clusterId: number): Promise<{
    success: boolean;
    data?: ClusterStatusInfo;
    error?: string;
  }> {
    try {
      const data = await this.getClusterStatus(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取集群状态失败';
      return {success: false, error: errorMessage};
    }
  }

  static async getRuntimeStorageSafe(clusterId: number): Promise<{
    success: boolean;
    data?: RuntimeStorageDetails;
    error?: string;
  }> {
    try {
      const data = await this.getRuntimeStorage(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取运行时存储详情失败';
      return {success: false, error: errorMessage};
    }
  }

  static async cleanupIMAPStorageSafe(clusterId: number): Promise<{
    success: boolean;
    data?: RuntimeStorageCleanupResult;
    error?: string;
  }> {
    try {
      const data = await this.cleanupIMAPStorage(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '清理 IMAP 失败';
      return {success: false, error: errorMessage};
    }
  }

  static async listRuntimeStorageSafe(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
    params?: {path?: string; recursive?: boolean; limit?: number},
  ): Promise<{
    success: boolean;
    data?: RuntimeStorageListResult;
    error?: string;
  }> {
    try {
      const data = await this.listRuntimeStorage(clusterId, kind, params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取运行时存储文件列表失败';
      return {success: false, error: errorMessage};
    }
  }

  static async previewRuntimeStorageSafe(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
    params: {path: string; max_bytes?: number},
  ): Promise<{
    success: boolean;
    data?: RuntimeStoragePreviewResult;
    error?: string;
  }> {
    try {
      const data = await this.previewRuntimeStorage(clusterId, kind, params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '运行时存储预览失败';
      return {success: false, error: errorMessage};
    }
  }

  static async inspectCheckpointRuntimeStorageSafe(
    clusterId: number,
    params: {
      path: string;
      job_config?: RuntimeStorageCheckpointInspectJobConfig;
    },
  ): Promise<{
    success: boolean;
    data?: RuntimeStorageCheckpointInspectResult;
    error?: string;
  }> {
    try {
      const data = await this.inspectCheckpointRuntimeStorage(
        clusterId,
        params,
      );
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : 'Checkpoint 反序列化失败';
      return {success: false, error: errorMessage};
    }
  }

  static async inspectIMAPRuntimeStorageSafe(
    clusterId: number,
    path: string,
  ): Promise<{
    success: boolean;
    data?: RuntimeStorageIMAPInspectResult;
    error?: string;
  }> {
    try {
      const data = await this.inspectIMAPRuntimeStorage(clusterId, path);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : 'IMAP WAL 解析失败';
      return {success: false, error: errorMessage};
    }
  }

  static async validateRuntimeStorageSafe(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
  ): Promise<{
    success: boolean;
    data?: RuntimeStorageValidationResult;
    error?: string;
  }> {
    try {
      const data = await this.validateRuntimeStorage(clusterId, kind);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '运行时存储连通性校验失败';
      return {success: false, error: errorMessage};
    }
  }

  // 保存可视化存储：后端先做真实读写测试，通过后生成配置版本
  // Save visual storage: backend probes read/write first, then creates a config version
  static async applyRuntimeStorageSafe(
    clusterId: number,
    kind: 'checkpoint' | 'imap',
    request: {
      enabled: boolean;
      storage_type: string;
      namespace?: string;
      endpoint?: string;
      bucket?: string;
      access_key?: string;
      secret_key?: string;
      hdfs_namenode_host?: string;
      hdfs_namenode_port?: number;
      hdfs_ha_enabled?: boolean;
      hdfs_name_services?: string;
      hdfs_ha_namenodes?: string;
      hdfs_namenode_rpc_address_1?: string;
      hdfs_namenode_rpc_address_2?: string;
      kerberos_principal?: string;
      kerberos_keytab_file_path?: string;
      hdfs_site_path?: string;
      s3_credentials_provider?: string;
    },
  ): Promise<{success: boolean; data?: ApplyRuntimeStorageResult; error?: string}> {
    try {
      const response = await apiClient.post<{
        error_msg: string;
        data: ApplyRuntimeStorageResult;
      }>(`${this.basePath}/${clusterId}/runtime-storage/${kind}/apply`, request);
      if (response.data.error_msg) {
        throw new Error(localizeBackendText(response.data.error_msg));
      }
      return {success: true, data: response.data.data};
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '保存运行时存储失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * 一键切换作业日志输出模式 (单 Job 独立日志模式 vs 共享混合日志模式)
   * Switch cluster job log output mode (per_job vs mixed)
   */
  static async switchJobLogMode(
    clusterId: number,
    mode: 'per_job' | 'mixed'
  ): Promise<{
    saved: boolean;
    restart_required: boolean;
    mode: 'per_job' | 'mixed';
    message: string;
    config_version?: number;
  }> {
    const response = await apiClient.post<{
      error_msg?: string;
      data: {
        saved: boolean;
        restart_required: boolean;
        mode: 'per_job' | 'mixed';
        message: string;
        config_version?: number;
      };
    }>(`${this.basePath}/${clusterId}/log-mode`, { mode });

    if (response.data.error_msg) {
      throw new Error(localizeBackendText(response.data.error_msg));
    }

    return response.data.data;
  }

  static async switchJobLogModeSafe(
    clusterId: number,
    mode: 'per_job' | 'mixed'
  ): Promise<{
    success: boolean;
    data?: {
      saved: boolean;
      restart_required: boolean;
      mode: 'per_job' | 'mixed';
      message: string;
      config_version?: number;
    };
    error?: string;
  }> {
    try {
      const data = await this.switchJobLogMode(clusterId, mode);
      return { success: true, data };
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '切换日志模式失败',
      };
    }
  }

  static async getStxJavaProxyStatusSafe(clusterId: number): Promise<{
    success: boolean;
    data?: StxJavaProxyStatus;
    error?: string;
  }> {
    try {
      const data = await this.getStxJavaProxyStatus(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取 stx-java-proxy 状态失败';
      return {success: false, error: errorMessage};
    }
  }

  static async startStxJavaProxySafe(clusterId: number): Promise<{
    success: boolean;
    data?: StxJavaProxyStatus;
    error?: string;
  }> {
    try {
      const data = await this.startStxJavaProxy(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '启动 stx-java-proxy 失败';
      return {success: false, error: errorMessage};
    }
  }

  static async stopStxJavaProxySafe(clusterId: number): Promise<{
    success: boolean;
    data?: StxJavaProxyStatus;
    error?: string;
  }> {
    try {
      const data = await this.stopStxJavaProxy(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '停止 stx-java-proxy 失败';
      return {success: false, error: errorMessage};
    }
  }

  static async restartStxJavaProxySafe(clusterId: number): Promise<{
    success: boolean;
    data?: StxJavaProxyStatus;
    error?: string;
  }> {
    try {
      const data = await this.restartStxJavaProxy(clusterId);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '重启 stx-java-proxy 失败';
      return {success: false, error: errorMessage};
    }
  }

  static async updateStxJavaProxyConfigSafe(
    clusterId: number,
    request: UpdateStxJavaProxyConfigRequest,
  ): Promise<{
    success: boolean;
    data?: StxJavaProxyStatus;
    error?: string;
  }> {
    try {
      const data = await this.updateStxJavaProxyConfig(clusterId, request);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '更新 stx-java-proxy 配置失败';
      return {success: false, error: errorMessage};
    }
  }

  static async previewStxJavaProxyServiceLogSafe(
    clusterId: number,
    params?: {lines?: number},
  ): Promise<{
    success: boolean;
    data?: StxJavaProxyLogPreviewResult;
    error?: string;
  }> {
    try {
      const data = await this.previewStxJavaProxyServiceLog(clusterId, params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '查看 stx-java-proxy 日志失败';
      return {success: false, error: errorMessage};
    }
  }

  // ==================== Node Operation Methods 节点操作方法 ====================

  /**
   * Start a node
   * 启动节点
   */
  static async startNode(
    clusterId: number,
    nodeId: number,
  ): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/nodes/${nodeId}/start`,
    );
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    return response.data.data;
  }

  /**
   * Stop a node
   * 停止节点
   */
  static async stopNode(
    clusterId: number,
    nodeId: number,
  ): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/nodes/${nodeId}/stop`,
    );
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    return response.data.data;
  }

  /**
   * Restart a node
   * 重启节点
   */
  static async restartNode(
    clusterId: number,
    nodeId: number,
  ): Promise<OperationResult> {
    const response = await apiClient.post<ClusterOperationResponse>(
      `${this.basePath}/${clusterId}/nodes/${nodeId}/restart`,
    );
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    return response.data.data;
  }

  /**
   * Get node logs
   * 获取节点日志
   *
   * @param clusterId - Cluster ID / 集群 ID
   * @param nodeId - Node ID / 节点 ID
   * @param params - Log query parameters / 日志查询参数
   * @param params.lines - Number of lines (default: 100) / 行数
   * @param params.mode - "tail" (default), "head", "all" / 模式
   * @param params.filter - Filter pattern / 过滤模式
   * @param params.date - Date for rolling logs / 滚动日志日期
   */
  static async getNodeLogs(
    clusterId: number,
    nodeId: number,
    params?: {lines?: number; mode?: string; filter?: string; date?: string},
  ): Promise<{logs: string}> {
    const response = await apiClient.get<{
      error_msg: string;
      data: {logs: string};
    }>(`${this.basePath}/${clusterId}/nodes/${nodeId}/logs`, {params});
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    return response.data.data;
  }

  /**
   * Start node (with error handling)
   * 启动节点（带错误处理）
   */
  static async startNodeSafe(
    clusterId: number,
    nodeId: number,
  ): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.startNode(clusterId, nodeId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '启动节点失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '启动节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Stop node (with error handling)
   * 停止节点（带错误处理）
   */
  static async stopNodeSafe(
    clusterId: number,
    nodeId: number,
  ): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.stopNode(clusterId, nodeId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '停止节点失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '停止节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Restart node (with error handling)
   * 重启节点（带错误处理）
   */
  static async restartNodeSafe(
    clusterId: number,
    nodeId: number,
  ): Promise<{
    success: boolean;
    data?: OperationResult;
    error?: string;
  }> {
    try {
      const data = await this.restartNode(clusterId, nodeId);
      // Check operation result success / 检查操作结果是否成功
      if (!data.success) {
        return {success: false, data, error: data.message || '重启节点失败'};
      }
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '重启节点失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * Get node logs (with error handling)
   * 获取节点日志（带错误处理）
   */
  static async getNodeLogsSafe(
    clusterId: number,
    nodeId: number,
    params?: {lines?: number; mode?: string; filter?: string; date?: string},
  ): Promise<{
    success: boolean;
    data?: {logs: string};
    error?: string;
  }> {
    try {
      const data = await this.getNodeLogs(clusterId, nodeId, params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取节点日志失败';
      return {success: false, error: errorMessage};
    }
  }
}
