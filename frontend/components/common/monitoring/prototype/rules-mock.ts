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
 * Prototype mock for the monitoring rules card grid.
 * 告警规则卡片网格原型的 mock 数据：规则清单对齐 design.md §2 目录。
 */

export type ProtoRuleEngine = 'availability' | 'event' | 'tsdb' | 'sync';
export type ProtoSeverity = 'warning' | 'critical';
export type ProtoNotifyMode = 'default' | 'custom' | 'none';
export type ProtoSegment = 'ok' | 'bad' | 'degraded';

export interface ProtoRule {
  key: string;
  /** zh / en display name provided by story copy maps. / 展示名由 story 文案表提供。 */
  name: string;
  description: string;
  engine: ProtoRuleEngine;
  enabled: boolean;
  severity: ProtoSeverity;
  threshold?: {operator: string; value: number; unit: string; min: number; max: number};
  forMinutes: number;
  notifyMode: ProtoNotifyMode;
  exceptionCount: number;
  /** 当前命中该规则的集群数。Clusters currently firing this rule. */
  firingCount: number;
  exceptions?: {cluster: string; overridden: string[]}[];
  visual: {
    /** tsdb: last-24h metric series (0..100 normalized). / 指标类：24h 序列。 */
    series?: number[];
    /** availability/sync: hourly health segments. / 可用性/同步：逐小时健康段。 */
    segments?: ProtoSegment[];
    /** event: firing timestamps within 24h (hour 0..23). / 事件类：触发时刻。 */
    events?: number[];
  };
}

/** 确定性伪随机，保证 story 每次渲染一致。Deterministic pseudo-random so the story renders identically every load. */
function seeded(seed: string): () => number {
  let h = 2166136261;
  for (let i = 0; i < seed.length; i += 1) {
    h ^= seed.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return () => {
    h = Math.imul(h ^ (h >>> 15), 2246822507);
    h = Math.imul(h ^ (h >>> 13), 3266489909);
    return ((h ^= h >>> 16) >>> 0) / 4294967296;
  };
}

function series(seedKey: string, base: number, jitter: number, spikeAt?: number): number[] {
  const rnd = seeded(seedKey);
  return Array.from({length: 48}, (_, i) => {
    const wave = Math.sin(i / 7 + base) * jitter * 0.6;
    const noise = (rnd() - 0.5) * jitter;
    const spike = spikeAt != null && i >= spikeAt ? (i - spikeAt + 1) * 6 : 0;
    return Math.max(0, Math.min(100, base + wave + noise + spike));
  });
}

function uptime(seedKey: string, badAt: number[] = [], degradedAt: number[] = []): ProtoSegment[] {
  return Array.from({length: 24}, (_, i) =>
    badAt.includes(i) ? 'bad' : degradedAt.includes(i) ? 'degraded' : 'ok',
  );
}

export const PROTO_CLUSTER_RULES: ProtoRule[] = [
  {
    key: 'node_offline',
    name: '节点离线',
    description: '有集群节点心跳丢失，需要人工确认节点状态。',
    engine: 'availability',
    enabled: true,
    severity: 'warning',
    forMinutes: 2,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 1,
    visual: {segments: uptime('node_offline', [21, 22])},
  },
  {
    key: 'master_unavailable',
    name: 'Master 不可用',
    description: 'Master 节点不可用，集群调度已中断。',
    engine: 'availability',
    enabled: true,
    severity: 'critical',
    forMinutes: 1,
    notifyMode: 'custom',
    exceptionCount: 1,
    firingCount: 2,
    exceptions: [{cluster: 'prod-hz-01', overridden: ['阈值', '通知']}],
    visual: {segments: uptime('master', [20, 21, 22, 23])},
  },
  {
    key: 'worker_insufficient',
    name: '可用 Worker 不足',
    description: '健康 Worker 数低于预期，作业可能被拒绝。',
    engine: 'availability',
    enabled: true,
    severity: 'warning',
    threshold: {operator: '<', value: 3, unit: 'workers', min: 1, max: 32},
    forMinutes: 2,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {segments: uptime('worker', [], [18])},
  },
  {
    key: 'agent_offline',
    name: 'Agent 离线',
    description: '管理 Agent 心跳超时，无法下发操作。',
    engine: 'availability',
    enabled: true,
    severity: 'warning',
    forMinutes: 2,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {segments: uptime('agent')},
  },
  {
    key: 'process_crashed',
    name: '进程崩溃',
    description: '引擎进程异常退出。',
    engine: 'event',
    enabled: true,
    severity: 'critical',
    forMinutes: 0,
    notifyMode: 'custom',
    exceptionCount: 0,
    firingCount: 0,
    visual: {events: [3, 17]},
  },
  {
    key: 'process_restart_failed',
    name: '进程重启失败',
    description: '自动重启尝试失败。',
    engine: 'event',
    enabled: true,
    severity: 'warning',
    forMinutes: 0,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {events: []},
  },
  {
    key: 'process_restart_limit_reached',
    name: '达到重启上限',
    description: '自动重启次数耗尽，必须人工介入。',
    engine: 'event',
    enabled: true,
    severity: 'critical',
    forMinutes: 0,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {events: [9]},
  },
  {
    key: 'sync_status_unavailable',
    name: '作业状态获取失败',
    description: '无法读取作业运行状态，观测能力下降。',
    engine: 'sync',
    enabled: true,
    severity: 'warning',
    forMinutes: 5,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {segments: uptime('sync_status', [], [14, 15])},
  },
  {
    key: 'split_brain_risk',
    name: '分区不安全',
    description: '集群或本地成员分区判定不安全。',
    engine: 'tsdb',
    enabled: true,
    severity: 'critical',
    forMinutes: 2,
    notifyMode: 'custom',
    exceptionCount: 2,
    firingCount: 0,
    exceptions: [
      {cluster: 'prod-hz-01', overridden: ['通知']},
      {cluster: 'staging-bj-02', overridden: ['开关']},
    ],
    visual: {series: series('split_brain', 8, 6)},
  },
  {
    key: 'deadlocked_threads_detected',
    name: '线程死锁',
    description: '检测到引擎线程死锁。',
    engine: 'tsdb',
    enabled: true,
    severity: 'critical',
    threshold: {operator: '>', value: 0, unit: '', min: 0, max: 100},
    forMinutes: 1,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('deadlock', 2, 3)},
  },
  {
    key: 'job_thread_pool_rejection_high',
    name: '任务被拒绝',
    description: '线程池拒绝率持续升高，引擎已在拒绝作业。',
    engine: 'tsdb',
    enabled: true,
    severity: 'critical',
    threshold: {operator: '>', value: 0, unit: '%', min: 0, max: 100},
    forMinutes: 5,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 1,
    visual: {series: series('reject', 6, 5, 40)},
  },
  {
    key: 'memory_usage_high',
    name: 'JVM 堆内存高',
    description: '堆内存使用持续高于阈值。',
    engine: 'tsdb',
    enabled: true,
    severity: 'warning',
    threshold: {operator: '>', value: 85, unit: '%', min: 50, max: 99},
    forMinutes: 10,
    notifyMode: 'default',
    exceptionCount: 1,
    firingCount: 0,
    exceptions: [{cluster: 'prod-hz-01', overridden: ['阈值']}],
    visual: {series: series('heap', 68, 10)},
  },
  {
    key: 'fd_usage_high',
    name: 'FD 使用率高',
    description: '文件描述符使用率持续走高。',
    engine: 'tsdb',
    enabled: true,
    severity: 'warning',
    threshold: {operator: '>', value: 80, unit: '%', min: 50, max: 99},
    forMinutes: 10,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('fd', 55, 12)},
  },
  {
    key: 'cpu_usage_high',
    name: 'CPU 高',
    description: '进程 CPU 使用率持续高于阈值。',
    engine: 'tsdb',
    enabled: false,
    severity: 'warning',
    threshold: {operator: '>', value: 80, unit: '%', min: 50, max: 99},
    forMinutes: 10,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('cpu', 45, 14)},
  },
  {
    key: 'gc_time_high',
    name: 'GC 耗时高',
    description: 'GC 时间占比持续高于阈值。',
    engine: 'tsdb',
    enabled: false,
    severity: 'warning',
    threshold: {operator: '>', value: 20, unit: '%', min: 5, max: 80},
    forMinutes: 10,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('gc', 12, 6)},
  },
  {
    key: 'job_thread_pool_queue_backlog_high',
    name: '线程池队列积压',
    description: '任务线程池队列深度持续高于阈值。',
    engine: 'tsdb',
    enabled: false,
    severity: 'warning',
    threshold: {operator: '>', value: 100, unit: '', min: 10, max: 1000},
    forMinutes: 5,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('queue', 40, 18)},
  },
  {
    key: 'coordinator_backlog_growing',
    name: '积压持续增长',
    description: '队列深度大于阈值且提交速率持续高于完成速率。',
    engine: 'tsdb',
    enabled: false,
    severity: 'warning',
    threshold: {operator: '>', value: 50, unit: '', min: 10, max: 1000},
    forMinutes: 10,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('backlog', 30, 12, 44)},
  },
  {
    key: 'failed_jobs_high',
    name: '失败作业数过高',
    description: '窗口内失败作业数超过阈值。',
    engine: 'tsdb',
    enabled: false,
    severity: 'warning',
    threshold: {operator: '>', value: 0, unit: '', min: 0, max: 50},
    forMinutes: 1,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {series: series('failed_jobs', 4, 4)},
  },
  {
    key: 'sync_event_channel_degraded',
    name: '作业事件通道降级',
    description: '作业事件通道进入降级模式，实时性下降。',
    engine: 'sync',
    enabled: false,
    severity: 'warning',
    forMinutes: 0,
    notifyMode: 'default',
    exceptionCount: 0,
    firingCount: 0,
    visual: {segments: uptime('event_channel')},
  },
  {
    key: 'cluster_restart_requested',
    name: '集群重启请求',
    description: '集群收到重启请求。',
    engine: 'event',
    enabled: false,
    severity: 'warning',
    forMinutes: 0,
    notifyMode: 'none',
    exceptionCount: 0,
    firingCount: 0,
    visual: {events: []},
  },
  {
    key: 'node_stop_requested',
    name: '节点停止请求',
    description: '节点收到停止请求。',
    engine: 'event',
    enabled: false,
    severity: 'warning',
    forMinutes: 0,
    notifyMode: 'none',
    exceptionCount: 0,
    firingCount: 0,
    visual: {events: []},
  },
];

export const PROTO_JOB_RULES = [
  {key: 'task-orders-cdc', task: 'orders → dwd_orders（CDC）', rules: ['failed', 'delay', 'long_running'], notify: 'default', enabled: true},
  {key: 'task-payments', task: 'payments → ads_payments', rules: ['failed'], notify: 'custom', enabled: true},
  {key: 'task-inventory', task: 'inventory → dws_inventory', rules: ['failed', 'delay'], notify: 'default', enabled: false},
  {key: 'task-clickstream', task: 'clickstream → ods_click', rules: ['failed'], notify: 'none', enabled: true},
] as const;

export const PROTO_CLUSTERS = ['prod-hz-01', 'prod-sh-02', 'staging-bj-02', 'dev-local'];

/** 英文规则名（按 key）。English display names keyed by rule key. */
export const RULE_NAME_EN: Record<string, string> = {
  node_offline: 'Node offline',
  master_unavailable: 'Master unavailable',
  worker_insufficient: 'Insufficient workers',
  agent_offline: 'Agent offline',
  process_crashed: 'Process crashed',
  process_restart_failed: 'Process restart failed',
  process_restart_limit_reached: 'Restart limit reached',
  sync_status_unavailable: 'Job status unavailable',
  split_brain_risk: 'Unsafe partition',
  deadlocked_threads_detected: 'Deadlocked threads',
  job_thread_pool_rejection_high: 'Task rejection high',
  memory_usage_high: 'JVM heap usage high',
  fd_usage_high: 'FD usage high',
  cpu_usage_high: 'CPU usage high',
  gc_time_high: 'GC time high',
  job_thread_pool_queue_backlog_high: 'Thread pool backlog high',
  coordinator_backlog_growing: 'Backlog growing',
  failed_jobs_high: 'Failed jobs high',
  sync_event_channel_degraded: 'Event channel degraded',
  cluster_restart_requested: 'Cluster restart requested',
  node_stop_requested: 'Node stop requested',
};
