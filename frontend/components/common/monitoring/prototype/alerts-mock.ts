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
 * Prototype mock for alerts and notification channels.
 * 告警列表与通知渠道的原型 mock。
 */

import type {ProtoSeverity} from './rules-mock';

export type ProtoAlertStatus = 'firing' | 'acknowledged' | 'silenced' | 'resolved';
export type ProtoAlertScope = 'cluster' | 'job';

export interface ProtoAlert {
  id: string;
  title: string;
  titleEn: string;
  ruleKey: string;
  severity: ProtoSeverity;
  scope: ProtoAlertScope;
  cluster: string;
  status: ProtoAlertStatus;
  /** 距首次触发的分钟数。Minutes since the alert started firing. */
  startedMinAgo: number;
  effectiveFrom: 'global' | 'exception';
  threshold?: string;
  /** 持续分钟数，0 表示实时。Duration in minutes; 0 means realtime. */
  forMinutes?: number;
  timeline: {kind: 'triggered' | 'notified' | 'acknowledged' | 'resolved'; minAgo: number; note?: string}[];
  deliveries: {channel: string; status: 'success' | 'failed' | 'pending'; minAgo: number}[];
}

export const PROTO_ALERTS: ProtoAlert[] = [
  {
    id: 'a-001',
    title: 'Master 不可用',
    titleEn: 'Master unavailable',
    ruleKey: 'master_unavailable',
    severity: 'critical',
    scope: 'cluster',
    cluster: 'prod-hz-01',
    status: 'firing',
    startedMinAgo: 12,
    effectiveFrom: 'exception',
    forMinutes: 1,
    timeline: [
      {kind: 'triggered', minAgo: 12},
      {kind: 'notified', minAgo: 12},
    ],
    deliveries: [
      {channel: 'ops-email', status: 'success', minAgo: 12},
      {channel: 'oncall-bot', status: 'success', minAgo: 12},
    ],
  },
  {
    id: 'a-002',
    title: 'Master 不可用',
    titleEn: 'Master unavailable',
    ruleKey: 'master_unavailable',
    severity: 'critical',
    scope: 'cluster',
    cluster: 'prod-sh-02',
    status: 'firing',
    startedMinAgo: 12,
    effectiveFrom: 'exception',
    forMinutes: 1,
    timeline: [
      {kind: 'triggered', minAgo: 12},
      {kind: 'notified', minAgo: 12},
    ],
    deliveries: [{channel: 'ops-email', status: 'success', minAgo: 12}],
  },
  {
    id: 'a-003',
    title: '节点离线',
    titleEn: 'Node offline',
    ruleKey: 'node_offline',
    severity: 'warning',
    scope: 'cluster',
    cluster: 'staging-bj-02',
    status: 'firing',
    startedMinAgo: 46,
    effectiveFrom: 'global',
    forMinutes: 2,
    timeline: [
      {kind: 'triggered', minAgo: 46},
      {kind: 'notified', minAgo: 46},
    ],
    deliveries: [{channel: 'alert-center-only', status: 'success', minAgo: 46}],
  },
  {
    id: 'a-004',
    title: '任务被拒绝',
    titleEn: 'Task rejection high',
    ruleKey: 'job_thread_pool_rejection_high',
    severity: 'critical',
    scope: 'cluster',
    cluster: 'prod-hz-01',
    status: 'acknowledged',
    startedMinAgo: 182,
    effectiveFrom: 'global',
    threshold: '> 0%', forMinutes: 5,
    timeline: [
      {kind: 'triggered', minAgo: 182},
      {kind: 'notified', minAgo: 182},
      {kind: 'acknowledged', minAgo: 95, note: 'admin'},
    ],
    deliveries: [{channel: 'oncall-bot', status: 'failed', minAgo: 182}],
  },
  {
    id: 'a-005',
    title: '作业运行失败',
    titleEn: 'Job run failed',
    ruleKey: 'sync_job_failed',
    severity: 'critical',
    scope: 'job',
    cluster: 'prod-hz-01 · orders → dwd_orders',
    status: 'firing',
    startedMinAgo: 8,
    effectiveFrom: 'global',
    forMinutes: 0,
    timeline: [
      {kind: 'triggered', minAgo: 8},
      {kind: 'notified', minAgo: 8},
    ],
    deliveries: [{channel: 'ops-email', status: 'pending', minAgo: 8}],
  },
  {
    id: 'a-006',
    title: 'CDC 延迟过高',
    titleEn: 'CDC delay high',
    ruleKey: 'sync_cdc_delay',
    severity: 'warning',
    scope: 'job',
    cluster: 'prod-hz-01 · clickstream → ods_click',
    status: 'acknowledged',
    startedMinAgo: 320,
    effectiveFrom: 'global',
    threshold: '> 120s', forMinutes: 10,
    timeline: [
      {kind: 'triggered', minAgo: 320},
      {kind: 'notified', minAgo: 320},
      {kind: 'acknowledged', minAgo: 200, note: 'devon'},
    ],
    deliveries: [{channel: 'ops-email', status: 'success', minAgo: 320}],
  },
  {
    id: 'a-007',
    title: 'JVM 堆内存高',
    titleEn: 'JVM heap usage high',
    ruleKey: 'memory_usage_high',
    severity: 'warning',
    scope: 'cluster',
    cluster: 'prod-sh-02',
    status: 'silenced',
    startedMinAgo: 640,
    effectiveFrom: 'exception',
    threshold: '> 90%', forMinutes: 10,
    timeline: [
      {kind: 'triggered', minAgo: 640},
      {kind: 'notified', minAgo: 640},
      {kind: 'acknowledged', minAgo: 600, note: 'admin · 静默 30 分钟'},
    ],
    deliveries: [{channel: 'ops-email', status: 'success', minAgo: 640}],
  },
  {
    id: 'a-008',
    title: '作业状态获取失败',
    titleEn: 'Job status unavailable',
    ruleKey: 'sync_status_unavailable',
    severity: 'warning',
    scope: 'cluster',
    cluster: 'dev-local',
    status: 'resolved',
    startedMinAgo: 1320,
    effectiveFrom: 'global',
    forMinutes: 5,
    timeline: [
      {kind: 'triggered', minAgo: 1320},
      {kind: 'notified', minAgo: 1320},
      {kind: 'resolved', minAgo: 1260},
    ],
    deliveries: [],
  },
  {
    id: 'a-009',
    title: '分区不安全',
    titleEn: 'Unsafe partition',
    ruleKey: 'split_brain_risk',
    severity: 'critical',
    scope: 'cluster',
    cluster: 'staging-bj-02',
    status: 'resolved',
    startedMinAgo: 2100,
    effectiveFrom: 'exception',
    forMinutes: 2,
    timeline: [
      {kind: 'triggered', minAgo: 2100},
      {kind: 'notified', minAgo: 2100},
      {kind: 'acknowledged', minAgo: 2050, note: 'sre-bot'},
      {kind: 'resolved', minAgo: 1990},
    ],
    deliveries: [
      {channel: 'oncall-bot', status: 'success', minAgo: 2100},
      {channel: 'ops-email', status: 'success', minAgo: 2100},
    ],
  },
  {
    id: 'a-010',
    title: 'FD 使用率高',
    titleEn: 'FD usage high',
    ruleKey: 'fd_usage_high',
    severity: 'warning',
    scope: 'cluster',
    cluster: 'prod-hz-01',
    status: 'firing',
    startedMinAgo: 75,
    effectiveFrom: 'global',
    threshold: '> 80%', forMinutes: 10,
    timeline: [
      {kind: 'triggered', minAgo: 75},
      {kind: 'notified', minAgo: 75},
    ],
    deliveries: [{channel: 'alert-center-only', status: 'success', minAgo: 75}],
  },
  {
    id: 'a-011',
    title: '作业长时间运行',
    titleEn: 'Job running too long',
    ruleKey: 'sync_job_long_running',
    severity: 'warning',
    scope: 'job',
    cluster: 'prod-sh-02 · inventory → dws_inventory',
    status: 'silenced',
    startedMinAgo: 1560,
    effectiveFrom: 'global',
    threshold: '> 6h', forMinutes: 0,
    timeline: [
      {kind: 'triggered', minAgo: 1560},
      {kind: 'acknowledged', minAgo: 1500, note: 'devon · 静默 30 分钟'},
    ],
    deliveries: [{channel: 'alert-center-only', status: 'success', minAgo: 1560}],
  },
  {
    id: 'a-012',
    title: 'Agent 离线',
    titleEn: 'Agent offline',
    ruleKey: 'agent_offline',
    severity: 'warning',
    scope: 'cluster',
    cluster: 'staging-bj-02',
    status: 'resolved',
    startedMinAgo: 2880,
    effectiveFrom: 'global',
    forMinutes: 2,
    timeline: [
      {kind: 'triggered', minAgo: 2880},
      {kind: 'notified', minAgo: 2880},
      {kind: 'resolved', minAgo: 2700},
    ],
    deliveries: [{channel: 'ops-email', status: 'success', minAgo: 2880}],
  },
];

export interface ProtoChannel {
  key: string;
  name: string;
  type: 'webhook' | 'email' | 'bot';
  target: string;
  enabled: boolean;
  health: {lastOkMinAgo: number | null; lastFailMinAgo: number | null; pending: number};
}

export const PROTO_CHANNELS: ProtoChannel[] = [
  {
    key: 'ops-email',
    name: 'ops-email',
    type: 'email',
    target: 's****@corp.example.com (+3)',
    enabled: true,
    health: {lastOkMinAgo: 8, lastFailMinAgo: null, pending: 1},
  },
  {
    key: 'oncall-bot',
    name: 'oncall-bot',
    type: 'bot',
    target: 'https://oapi.dingtalk.com/robot/send?access_token=••••',
    enabled: true,
    health: {lastOkMinAgo: 2100, lastFailMinAgo: 182, pending: 0},
  },
  {
    key: 'sre-webhook',
    name: 'sre-webhook',
    type: 'webhook',
    target: 'https://hooks.internal.example.com/alerts/••••',
    enabled: true,
    health: {lastOkMinAgo: null, lastFailMinAgo: null, pending: 0},
  },
  {
    key: 'audit-mail',
    name: 'audit-mail',
    type: 'email',
    target: 'a****@corp.example.com',
    enabled: false,
    health: {lastOkMinAgo: 4300, lastFailMinAgo: null, pending: 0},
  },
];
