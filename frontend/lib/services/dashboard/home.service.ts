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

import {BaseService} from '../core/base.service';

/**
 * 首页「需要处理」条目；viewable=false 表示当前用户无权查看该作业，只展示不可点开。
 * A home "needs attention" item; viewable=false means the user cannot view the job, so it is display-only.
 */
export interface HomeAttentionItem {
  kind: 'job_failed' | 'cdc_delay' | 'backpressure' | 'alert';
  severity: 'critical' | 'warning';
  title: string;
  detail?: string;
  value?: number;
  job_id?: number;
  task_id?: number;
  alert_id?: string;
  at: string;
  viewable: boolean;
}

/** 首页作业汇总（全局口径）/ Home job summary (global scope). */
export interface HomeJobSummary {
  running: number;
  running_streaming: number;
  running_batch: number;
  failed_24h: number;
  failed_prev_24h: number;
  write_qps: number;
  read_qps: number;
  max_cdc_delay_seconds?: number;
  max_cdc_job_id?: number;
  max_cdc_task_id?: number;
  max_cdc_job_name?: string;
  cdc_threshold_seconds: number;
  trend_source: 'prometheus' | 'none';
  trend_reason?: string;
  write_trend: [number, number][];
  cdc_trend: [number, number][];
  attention: HomeAttentionItem[];
}

export interface HomeSummary {
  generated_at: string;
  cluster_id?: number;
  jobs: HomeJobSummary | null;
  clusters: {
    total: number;
    healthy: number;
    degraded: number;
    unhealthy: number;
    nodes_online: number;
    nodes_total: number;
  };
  alerts: {active: number; critical: number};
  attention: HomeAttentionItem[];
  partial_errors?: string[];
}

/** 集群卡片 / Cluster card. */
export interface HomeClusterCard {
  id: number;
  name: string;
  version: string;
  health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | string;
  nodes_online: number;
  nodes_total: number;
  running_jobs: number;
  active_alerts: number;
  critical_alerts: number;
  max_heap_percent?: number;
  endpoints_down?: number;
}

/** 节点表一行；指标字段只在 Prometheus 可用时出现 / Node row; metric fields only with Prometheus. */
export interface HomeClusterNode {
  cluster_id: number;
  cluster_name: string;
  node_id: number;
  instance: string;
  role: string;
  status: string;
  agent_online: boolean;
  master?: boolean;
  scrape_up?: boolean;
  node_active?: boolean;
  heap_percent?: number;
  cpu_cores?: number;
  threads?: number;
  fd_percent?: number;
  uptime_seconds?: number;
}

export interface HomeClusterSignals {
  clusters_safe: number;
  clusters_total: number;
  deadlocked_threads?: number;
  fd_percent_max?: number;
  pool_reject_rate?: number;
  hz_queue_util_max?: number;
  endpoints_down: number;
}

export interface HomeDashboardSeries {
  key: 'availability' | 'heap' | 'cpu' | 'gc' | 'job_state' | string;
  label: string;
  points: [number, number][];
}

/** GET /dashboard/home/clusters 返回 / Response of GET /dashboard/home/clusters. */
export interface HomeClusterDashboard {
  source: 'prometheus' | 'none';
  reason?: string;
  range: string;
  start_ms: number;
  end_ms: number;
  step_seconds: number;
  cell_seconds: number;
  clusters: HomeClusterCard[];
  nodes: HomeClusterNode[];
  signals: HomeClusterSignals | null;
  series: HomeDashboardSeries[];
  partial_errors?: string[];
}

/**
 * 首页汇总服务：指标卡与「需要处理」条共用一次请求。
 * Home summary service: the KPI cards and the "needs attention" strip share one request.
 */
export class HomeService extends BaseService {
  protected static readonly basePath = '/dashboard/home';

  /**
   * 集群大盘：卡片、节点、趋势与异常信号；数组字段做兜底。
   * Cluster dashboard: cards, nodes, trends and signals, with array fallbacks.
   */
  static async getClusters(params?: {
    cluster_id?: number;
    range?: string;
  }): Promise<HomeClusterDashboard> {
    const data = await this.get<HomeClusterDashboard>('/clusters', params);
    return {
      ...data,
      clusters: data?.clusters ?? [],
      nodes: data?.nodes ?? [],
      series: (data?.series ?? []).map((item) => ({
        ...item,
        points: item.points ?? [],
      })),
      signals: data?.signals ?? null,
    };
  }

  static async getSummary(params?: {
    cluster_id?: number;
  }): Promise<HomeSummary> {
    const data = await this.get<HomeSummary>('/summary', params);
    // 数组兜底 / Array fallbacks.
    return {
      ...data,
      attention: data?.attention ?? [],
      jobs: data?.jobs
        ? {
            ...data.jobs,
            write_trend: data.jobs.write_trend ?? [],
            cdc_trend: data.jobs.cdc_trend ?? [],
            attention: data.jobs.attention ?? [],
          }
        : null,
    };
  }
}
