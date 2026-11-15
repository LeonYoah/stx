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

/**
 * 首页汇总服务：指标卡与「需要处理」条共用一次请求。
 * Home summary service: the KPI cards and the "needs attention" strip share one request.
 */
export class HomeService extends BaseService {
  protected static readonly basePath = '/dashboard/home';

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
