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
 * Discovery Service - 集群发现服务
 * Provides API calls for cluster discovery functionality.
 * 提供集群发现功能的 API 调用。
 * Requirements: 1.2, 1.9, 9.3, 9.4
 */

import apiClient from '../core/api-client';

// DiscoveredProcess represents a discovered SeaTunnel process (enhanced).
// DiscoveredProcess 表示发现的 SeaTunnel 进程（增强版）。
export interface DiscoveredProcess {
  /** Process JDK when resolved from /proc/<pid>/exe / 从进程可执行文件解析到的 JDK */
  java_home?: string;
  pid: number;
  role: string; // master, worker, or hybrid
  install_dir: string;
  version: string; // SeaTunnel version (e.g., 2.3.12)
  hazelcast_port: number; // Hazelcast cluster port
  api_port: number; // REST API port
  cluster_name?: string; // Hazelcast cluster-name / Hazelcast 集群标识
}

// ProcessDiscoveryResult represents the result of process discovery.
// ProcessDiscoveryResult 表示进程发现的结果。
export interface ProcessDiscoveryResult {
  success: boolean;
  message: string;
  processes: DiscoveredProcess[];
}

// DiscoveredNode represents a discovered SeaTunnel node.
// DiscoveredNode 表示发现的 SeaTunnel 节点。
export interface DiscoveredNode {
  pid: number;
  role: string;
  hazelcast_port: number;
  api_port: number;
  start_time: string;
  /** Process JDK when Agent resolved it (AC1f) / 进程 JDK（Agent 解析到时，AC1f） */
  java_home?: string;
}

// DiscoveredCluster represents a discovered SeaTunnel cluster.
// DiscoveredCluster 表示发现的 SeaTunnel 集群。
export interface DiscoveredCluster {
  name: string;
  install_dir: string;
  version: string;
  deployment_mode: string;
  nodes: DiscoveredNode[];
  config: Record<string, unknown>;
  discovered_at: string;
  is_new: boolean;
  existing_id: number;
}

// DiscoveryResult represents the result of a discovery operation.
// DiscoveryResult 表示发现操作的结果。
export interface DiscoveryResult {
  success: boolean;
  message: string;
  clusters: DiscoveredCluster[];
  new_count: number;
  exist_count: number;
}

// ConfirmDiscoveryRequest represents a request to confirm discovered clusters.
// ConfirmDiscoveryRequest 表示确认发现集群的请求。
export interface ConfirmDiscoveryRequest {
  cluster_ids?: string[];
  install_dirs?: string[];
  /** install_dir -> java_home for each imported cluster node (AC1e) */
  java_homes?: Record<string, string>;
}

// ConfirmDiscoveryResponse represents the response for confirming discovery.
// ConfirmDiscoveryResponse 表示确认发现的响应。
export interface ConfirmDiscoveryResponse {
  success: boolean;
  message: string;
  cluster_ids: number[];
  count: number;
}

/**
 * Discover SeaTunnel processes on a host (simplified).
 * 在主机上发现 SeaTunnel 进程（简化版）。
 * Only returns PID, role, and install_dir - no config parsing.
 * 只返回 PID、角色和安装目录 - 不解析配置。
 * @param hostId - Host ID / 主机 ID
 */
export async function discoverProcesses(hostId: number): Promise<ProcessDiscoveryResult> {
  const response = await apiClient.post<ProcessDiscoveryResult>(`/hosts/${hostId}/discover-processes`);
  return response.data;
}

/**
 * Trigger cluster discovery on a host.
 * 在主机上触发集群发现。
 * @param hostId - Host ID / 主机 ID
 * @deprecated Use discoverProcesses instead for simplified flow
 */
export async function triggerDiscovery(hostId: number): Promise<DiscoveryResult> {
  const response = await apiClient.post<DiscoveryResult>(`/hosts/${hostId}/discover`);
  return response.data;
}

/**
 * Confirm and import discovered clusters.
 * 确认并导入发现的集群。
 * @param hostId - Host ID / 主机 ID
 * @param request - Confirm request / 确认请求
 */
export async function confirmDiscovery(
  hostId: number,
  request: ConfirmDiscoveryRequest
): Promise<ConfirmDiscoveryResponse> {
  const response = await apiClient.post<ConfirmDiscoveryResponse>(
    `/hosts/${hostId}/discover/confirm`,
    request
  );
  return response.data;
}
