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
 * Monitor Service - 监控配置服务
 * Provides API calls for monitor configuration and process events.
 * 提供监控配置和进程事件的 API 调用。
 * Requirements: 5.1, 5.4, 6.4
 */

import apiClient from '../core/api-client';

// MonitorConfig represents the monitoring configuration for a cluster.
// MonitorConfig 表示集群的监控配置。
export interface MonitorConfig {
  id: number;
  cluster_id: number;
  auto_monitor: boolean;
  auto_restart: boolean;
  monitor_interval: number;
  restart_delay: number;
  max_restarts: number;
  time_window: number;
  cooldown_period: number;
  config_version: number;
  last_sync_at: string | null;
  created_at: string;
  updated_at: string;
}

// UpdateMonitorConfigRequest represents a request to update monitor config.
// UpdateMonitorConfigRequest 表示更新监控配置的请求。
export interface UpdateMonitorConfigRequest {
  auto_monitor?: boolean;
  auto_restart?: boolean;
  monitor_interval?: number;
  restart_delay?: number;
  max_restarts?: number;
  time_window?: number;
  cooldown_period?: number;
}

// ProcessEventType represents the type of process event.
// ProcessEventType 表示进程事件类型。
export type ProcessEventType =
  | 'started'
  | 'stopped'
  | 'crashed'
  | 'restarted'
  | 'restart_failed'
  | 'restart_limit_reached'
  | 'cluster_restart_requested'
  | 'node_restart_requested'
  | 'node_stop_requested'
  | 'node_offline'
  | 'node_recovered'
  | 'agent_offline'
  | 'agent_recovered'
  | 'master_unavailable'
  | 'master_recovered'
  | 'worker_insufficient'
  | 'worker_recovered';

// ProcessEvent represents a process lifecycle event.
// ProcessEvent 表示进程生命周期事件。
export interface ProcessEvent {
  id: number;
  cluster_id: number;
  node_id: number;
  host_id: number;
  event_type: ProcessEventType;
  pid: number;
  process_name: string;
  install_dir: string;
  role: string;
  details: string;
  created_at: string;
  hostname?: string; // 主机名 / Hostname
  ip?: string; // 主机 IP / Host IP
}

// ProcessEventFilter represents filter criteria for querying process events.
// ProcessEventFilter 表示查询进程事件的过滤条件。
export interface ProcessEventFilter {
  event_type?: ProcessEventType;
  node_id?: number;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

// ProcessEventListResponse represents the response for listing process events.
// ProcessEventListResponse 表示进程事件列表的响应。
export interface ProcessEventListResponse {
  events: ProcessEvent[];
  total: number;
  page: number;
  page_size: number;
}

// EventStats represents event statistics.
// EventStats 表示事件统计。
export type EventStats = Record<ProcessEventType, number>;

/**
 * Get monitor configuration for a cluster.
 * 获取集群的监控配置。
 * @param clusterId - Cluster ID / 集群 ID
 */
export async function getMonitorConfig(clusterId: number): Promise<MonitorConfig> {
  const response = await apiClient.get<MonitorConfig>(`/clusters/${clusterId}/monitor-config`);
  return response.data;
}

/**
 * Update monitor configuration for a cluster.
 * 更新集群的监控配置。
 * @param clusterId - Cluster ID / 集群 ID
 * @param config - Config update request / 配置更新请求
 */
export async function updateMonitorConfig(
  clusterId: number,
  config: UpdateMonitorConfigRequest
): Promise<MonitorConfig> {
  const response = await apiClient.put<MonitorConfig>(`/clusters/${clusterId}/monitor-config`, config);
  return response.data;
}

/**
 * List process events for a cluster.
 * 获取集群的进程事件列表。
 * @param clusterId - Cluster ID / 集群 ID
 * @param filter - Filter criteria / 过滤条件
 */
export async function listProcessEvents(
  clusterId: number,
  filter?: ProcessEventFilter
): Promise<ProcessEventListResponse> {
  const params = new URLSearchParams();
  if (filter?.event_type) {
    params.append('event_type', filter.event_type);
  }
  if (filter?.node_id) {
    params.append('node_id', String(filter.node_id));
  }
  if (filter?.start_time) {
    params.append('start_time', filter.start_time);
  }
  if (filter?.end_time) {
    params.append('end_time', filter.end_time);
  }
  if (filter?.page) {
    params.append('page', String(filter.page));
  }
  if (filter?.page_size) {
    params.append('page_size', String(filter.page_size));
  }

  const queryString = params.toString();
  const url = `/clusters/${clusterId}/events${queryString ? `?${queryString}` : ''}`;
  const response = await apiClient.get<ProcessEventListResponse>(url);
  return response.data;
}

/**
 * Get event statistics for a cluster.
 * 获取集群的事件统计。
 * @param clusterId - Cluster ID / 集群 ID
 * @param since - Since time (RFC3339) / 起始时间
 */
export async function getEventStats(clusterId: number, since?: string): Promise<EventStats> {
  const params = since ? `?since=${encodeURIComponent(since)}` : '';
  const response = await apiClient.get<EventStats>(`/clusters/${clusterId}/events/stats${params}`);
  return response.data;
}
