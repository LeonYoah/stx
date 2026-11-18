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

/** 作业大盘里的一项 / One job dashboard entry. */
export interface HomeJobRank {
  job_id: number;
  task_id?: number;
  cluster_id?: number;
  name: string;
  value: number;
  viewable: boolean;
  max_seconds?: number;
  failures?: number;
  kind?: 'busy' | 'backpressure';
  vertex?: string;
}

/** GET /dashboard/home/jobs 返回 / Response of GET /dashboard/home/jobs. */
export interface HomeJobDashboard {
  source: 'prometheus' | 'none';
  reason?: string;
  range: string;
  start_ms: number;
  end_ms: number;
  step_seconds: number;
  window_seconds: number;
  throughput: {
    key: 'read' | 'write' | 'read_bytes' | 'write_bytes';
    points: [number, number][];
  }[];
  annotations: {
    ts_ms: number;
    kind: 'job_failed' | 'job_recovered' | 'schema_change';
    title: string;
    job_id?: number;
    task_id?: number;
    viewable: boolean;
  }[];
  top_write: HomeJobRank[];
  top_cdc: HomeJobRank[];
  flow: {source: string; sink: string; value: number}[];
  checkpoints: HomeJobRank[];
  hotspots: HomeJobRank[];
  calendar: {
    weeks: number;
    end_date: string;
    runs: number[][];
    failed: number[][];
  } | null;
  partial_errors?: string[];
}

/** 一个时间窗内四个方向的读写量 / Totals for the four directions within one window. */
export interface HomeVolumeCounts {
  read_rows: number;
  write_rows: number;
  read_bytes: number;
  write_bytes: number;
}

/** 指标卡用总量：当前窗口 / 今日 / 本月；起止毫秒按显示时区。/ KPI totals: window / today / month. */
export interface HomeVolumeTotals {
  window: HomeVolumeCounts;
  today: HomeVolumeCounts;
  month: HomeVolumeCounts;
  today_start_ms: number;
  month_start_ms: number;
}

/** 日 / 月桶；partial=true 表示被时间窗裁掉一头。/ Day/month bucket; partial means window-clipped. */
export interface HomeVolumeBucket {
  start_ms: number;
  end_ms: number;
  label: string;
  partial: boolean;
}

/** 「指标 × 分组」的桶值序列，totals 与 buckets 下标对齐。/ Metric × group series aligned to buckets. */
export interface HomeVolumeSeries {
  metric: 'read_rows' | 'write_rows' | 'read_bytes' | 'write_bytes' | string;
  group_id: string;
  totals: number[];
}

/** GET /dashboard/home/volume 返回 / Response of GET /dashboard/home/volume. */
export interface HomeVolume {
  source: 'prometheus' | 'none';
  reason?: string;
  range: string;
  start_ms: number;
  end_ms: number;
  granularity?: 'quarter_hour' | 'hour' | 'day' | 'month';
  split: 'none' | 'cluster' | 'task';
  oldest_sample_ms: number;
  totals: HomeVolumeTotals | null;
  buckets: HomeVolumeBucket[];
  series: HomeVolumeSeries[];
  groups?: Record<string, string>;
  partial_errors?: string[];
}

/** 集群细节面板内一条序列：instance 定色，label 是维度名。/ One series in a detail panel: instance picks the color, label is the dimension name. */
export interface HomeDetailSeries {
  instance: string;
  label: string;
  points: [number, number][];
}

/** 集群细节面板：单位 + 按节点（可带维度）的序列 + 可选参考线。/ Detail panel: unit + per-node series + optional reference line. */
export interface HomeDetailPanel {
  key: string;
  unit: 'bytes' | 'percent' | 'count' | 'per_second' | 'ms' | string;
  reference?: number;
  series: HomeDetailSeries[];
}

/** 趋势上的事件标注。/ Event marker on the trends. */
export interface HomeDetailEvent {
  ts_ms: number;
  kind: 'node_restart' | 'members_change' | string;
  label: string;
}

/** GET /dashboard/home/clusters/detail 返回 / Response of GET /dashboard/home/clusters/detail. */
export interface HomeClusterDetail {
  source: 'prometheus' | 'none';
  reason?: string;
  range: string;
  start_ms: number;
  end_ms: number;
  step_seconds: number;
  group: 'jvm' | 'gc' | 'sched' | 'store' | 'proc' | string;
  panels: HomeDetailPanel[];
  events: HomeDetailEvent[];
  partial_errors?: string[];
}

/** 单台主机条目与面板组（E3）。/ One host entry plus its panel set (E3). */
export interface HomeHostEntry {
  id: number;
  name: string;
  ip: string;
  host_type: string;
  agent_status: string;
  cpu_cores: number;
  total_memory_bytes: number;
  last_heartbeat_ms?: number;
  /** 开机时间（stx_host_boot_time_seconds），用于卡片头「已运行 N 天」。/ Boot time (stx_host_boot_time_seconds) feeding the header's "up N days". */
  boot_time_ms?: number;
  metrics_supported: boolean;
  panels: HomeDetailPanel[];
}

/** 主机视图负载。/ Host view payload. */
export interface HomeHostDashboard {
  source: string;
  reason?: string;
  range: string;
  step_seconds: number;
  start_ms: number;
  end_ms: number;
  cluster_id?: number;
  hosts: HomeHostEntry[];
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
    start?: number;
    end?: number;
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

  /**
   * 作业大盘；数组字段做兜底。
   * Job dashboard, with array fallbacks.
   */
  static async getJobs(params?: {
    cluster_id?: number;
    range?: string;
    start?: number;
    end?: number;
  }): Promise<HomeJobDashboard> {
    const data = await this.get<HomeJobDashboard>('/jobs', params);
    return {
      ...data,
      throughput: (data?.throughput ?? []).map((item) => ({
        ...item,
        points: item.points ?? [],
      })),
      annotations: data?.annotations ?? [],
      top_write: data?.top_write ?? [],
      top_cdc: data?.top_cdc ?? [],
      flow: data?.flow ?? [],
      checkpoints: data?.checkpoints ?? [],
      hotspots: data?.hotspots ?? [],
      calendar: data?.calendar ?? null,
    };
  }

  /**
   * 读写量：不传 granularity 时只回 totals（指标卡）；granularity=auto 时由后端按窗口
   * 跨度自动选桶粒度，数组字段做兜底。
   * Data volume: totals only without granularity; granularity=auto lets the backend pick the
   * bucket size from the window span, with array fallbacks.
   */
  static async getVolume(params?: {
    cluster_id?: number;
    range?: string;
    start?: number;
    end?: number;
    granularity?: 'auto' | 'quarter_hour' | 'hour' | 'day' | 'month';
    split?: 'none' | 'cluster' | 'task';
  }): Promise<HomeVolume> {
    const data = await this.get<HomeVolume>('/volume', params);
    return {
      ...data,
      buckets: data?.buckets ?? [],
      series: (data?.series ?? []).map((item) => ({
        ...item,
        totals: item.totals ?? [],
      })),
      groups: data?.groups ?? {},
      totals: data?.totals ?? null,
      partial_errors: data?.partial_errors ?? [],
    };
  }

  /**
   * 集群细节分组：group 必传，只回该组面板；数组字段做兜底。
   * Cluster detail groups: group is required and only that group's panels return, with array fallbacks.
   */
  static async getClustersDetail(params: {
    cluster_id?: number;
    range?: string;
    start?: number;
    end?: number;
    group: 'jvm' | 'gc' | 'sched' | 'store' | 'proc';
  }): Promise<HomeClusterDetail> {
    const data = await this.get<HomeClusterDetail>('/clusters/detail', params);
    return {
      ...data,
      panels: (data?.panels ?? []).map((item) => ({
        ...item,
        series: (item.series ?? []).map((entry) => ({
          ...entry,
          points: entry.points ?? [],
        })),
      })),
      events: data?.events ?? [],
      partial_errors: data?.partial_errors ?? [],
    };
  }

  /**
   * 主机视图：清单 + 每台主机一组 stx_host_* 面板；数组字段做兜底。
   * Host view: inventory plus one stx_host_* panel set per host, with array fallbacks.
   */
  static async getHosts(params?: {
    cluster_id?: number;
    range?: string;
    start?: number;
    end?: number;
  }): Promise<HomeHostDashboard> {
    const data = await this.get<HomeHostDashboard>('/hosts', params);
    return {
      ...data,
      hosts: (data?.hosts ?? []).map((item) => ({
        ...item,
        panels: (item.panels ?? []).map((panel) => ({
          ...panel,
          series: (panel.series ?? []).map((entry) => ({
            ...entry,
            points: entry.points ?? [],
          })),
        })),
      })),
      partial_errors: data?.partial_errors ?? [],
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
