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
 * Monitoring Center Types
 * 监控中心类型定义
 */

import type {MonitorConfig, ProcessEvent} from '@/lib/services/monitor';

export interface EventStats {
  started: number;
  stopped: number;
  crashed: number;
  restarted: number;
  restart_failed: number;
  restart_limit_reached: number;
  node_offline?: number;
  node_recovered?: number;
}

export interface MonitoringOverviewStats {
  total_clusters: number;
  healthy_clusters: number;
  unhealthy_clusters: number;
  unknown_clusters: number;
  total_nodes: number;
  online_nodes: number;
  offline_nodes: number;
  crashed_events_24h: number;
  restart_failed_events_24h: number;
  active_alerts_1h: number;
}

export interface ClusterMonitoringSummary {
  cluster_id: number;
  cluster_name: string;
  status: string;
  health_status: string;
  total_nodes: number;
  online_nodes: number;
  offline_nodes: number;
  crashed_events_24h: number;
  restart_failed_events_24h: number;
  active_alerts_1h: number;
  last_event_at?: string | null;
}

export interface MonitoringOverviewData {
  generated_at: string;
  stats: MonitoringOverviewStats;
  event_stats_24h: EventStats;
  clusters: ClusterMonitoringSummary[];
}

export interface ClusterBaseInfo {
  cluster_id: number;
  cluster_name: string;
  status: string;
  health_status: string;
}

export interface ClusterMonitoringDetailStats {
  total_nodes: number;
  online_nodes: number;
  offline_nodes: number;
  crashed_events_24h: number;
  restart_failed_events_24h: number;
  active_alerts_1h: number;
}

export interface NodeSnapshot {
  node_id: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: string;
  status: string;
  is_online: boolean;
  process_pid: number;
}

export interface ClusterMonitoringOverviewData {
  generated_at: string;
  cluster: ClusterBaseInfo;
  stats: ClusterMonitoringDetailStats;
  event_stats_24h: EventStats;
  event_stats_1h: EventStats;
  monitor_config: MonitorConfig | null;
  nodes: NodeSnapshot[];
  recent_events: ProcessEvent[];
}

export type AlertSeverity = 'warning' | 'critical';
export type AlertStatus = 'firing' | 'acknowledged' | 'silenced';

export interface AlertStats {
  firing: number;
  acknowledged: number;
  silenced: number;
}

export interface AlertEvent {
  alert_id: string;
  event_id: number;
  cluster_id: number;
  cluster_name: string;
  node_id: number;
  host_id: number;
  hostname: string;
  ip: string;
  event_type: string;
  severity: AlertSeverity;
  status: AlertStatus;
  rule_key: string;
  rule_name: string;
  process_name: string;
  pid: number;
  role: string;
  details: string;
  created_at: string;
  acknowledged_by?: string;
  acknowledged_at?: string | null;
  silenced_by?: string;
  silenced_until?: string | null;
  latest_action_note?: string;
}

export interface AlertListData {
  generated_at: string;
  page: number;
  page_size: number;
  total: number;
  stats: AlertStats;
  alerts: AlertEvent[];
}

export type AlertSourceType = 'platform_health' | 'metric' | 'job';
export type AlertLifecycleStatus = 'firing' | 'resolved';
export type AlertDisplayStatus = 'firing' | 'resolved' | 'closed';
export type AlertHandlingStatus =
  | 'pending'
  | 'acknowledged'
  | 'silenced'
  | 'closed';

export interface AlertInstanceStats {
  firing: number;
  resolved: number;
  closed: number;
}

/**
 * Alert rule catalog entry: rule key, display name and category.
 * 告警规则目录项：规则键、显示名与告警类别。
 */
export interface AlertRuleCatalogItem {
  rule_key: string;
  name: string;
  category: AlertSourceType;
}

export interface AlertInstanceSourceRef {
  event_id?: number;
  fingerprint?: string;
  event_type?: string;
  process_name?: string;
  hostname?: string;
  receiver?: string;
  env?: string;
  /** 映射到目录规则前的存储告警名（旧 YAML 规则名） / Stored alert name before catalog-rule mapping (legacy YAML rule name) */
  original_alert_name?: string;
  /** 以下字段只用于 sync 阶段（alert_id 以 sync: 前缀标识，类别按规则归属）。 / The fields below are only set for sync episodes (alert_id carries the sync: prefix; category follows the rule). */
  task_id?: number;
  task_name?: string;
  job_id?: number;
  engine_job_id?: string;
  trigger_source?: string;
  occurrence_count?: number;
  link?: string;
  /**
   * sync 告警阶段的结束原因：recovered、job_ended、channel_unavailable、rule_removed、rule_changed、policy_deleted、closed。
   * Why a sync episode ended: recovered, job_ended, channel_unavailable, rule_removed, rule_changed, policy_deleted, closed.
   */
  resolve_reason?: string;
  /** B4 无法确认超过 24 小时，已停止提醒 / B4 unconfirmable for over 24h, reminders stopped */
  reminders_paused?: boolean;
}

export interface AlertInstance {
  alert_id: string;
  source_type: AlertSourceType;
  cluster_id: string;
  cluster_name: string;
  severity: AlertSeverity | string;
  alert_name: string;
  alert_name_en?: string;
  rule_key: string;
  summary: string;
  description: string;
  status: AlertDisplayStatus;
  lifecycle_status: AlertLifecycleStatus;
  handling_status: AlertHandlingStatus;
  created_at: string;
  firing_at: string;
  resolved_at?: string | null;
  last_seen_at: string;
  acknowledged_by?: string;
  acknowledged_at?: string | null;
  silenced_by?: string;
  silenced_until?: string | null;
  closed_by?: string;
  closed_at?: string | null;
  latest_note?: string;
  source_ref?: AlertInstanceSourceRef | null;
  scope?: AlertScope;
}

export interface AlertInstanceListData {
  generated_at: string;
  page: number;
  page_size: number;
  total: number;
  stats: AlertInstanceStats;
  alerts: AlertInstance[];
}

export interface AlertInstanceFilterParams {
  source_type?: AlertSourceType;
  scope?: AlertScope;
  cluster_id?: string;
  severity?: AlertSeverity;
  status?: AlertDisplayStatus;
  lifecycle_status?: AlertLifecycleStatus;
  handling_status?: AlertHandlingStatus;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface AlertInstanceActionResult {
  alert_id: string;
  status?: AlertDisplayStatus;
  handling_status: AlertHandlingStatus;
  acknowledged_by?: string;
  acknowledged_at?: string | null;
  silenced_by?: string;
  silenced_until?: string | null;
  closed_by?: string;
  closed_at?: string | null;
  latest_note?: string;
}

export interface AlertFilterParams {
  cluster_id?: number;
  status?: AlertStatus;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface AcknowledgeAlertRequest {
  note?: string;
}

export interface SilenceAlertRequest {
  duration_minutes: number;
  note?: string;
}

export interface AlertActionResult {
  event_id: number;
  status: AlertStatus;
  acknowledged_by?: string;
  acknowledged_at?: string | null;
  silenced_by?: string;
  silenced_until?: string | null;
  latest_action_note?: string;
}

export type RemoteAlertStatus = 'firing' | 'resolved' | string;

export interface RemoteAlertEvent {
  id: number;
  fingerprint: string;
  status: RemoteAlertStatus;
  receiver: string;
  alert_name: string;
  severity: string;
  cluster_id: string;
  cluster_name: string;
  env: string;
  summary: string;
  description: string;
  starts_at: number;
  ends_at: number;
  resolved_at?: string | null;
  last_received_at: string;
  created_at: string;
  updated_at: string;
}

export interface RemoteAlertListData {
  generated_at: string;
  page: number;
  page_size: number;
  total: number;
  alerts: RemoteAlertEvent[];
}

export interface RemoteAlertFilterParams {
  cluster_id?: string;
  status?: string;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface ClusterHealthItem {
  cluster_id: number;
  cluster_name: string;
  status: string;
  health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | string;
  total_nodes: number;
  online_nodes: number;
  offline_nodes: number;
  active_alerts: number;
  critical_alerts: number;
}

export interface ClusterHealthData {
  generated_at: string;
  total: number;
  clusters: ClusterHealthItem[];
}

export interface PlatformHealthData {
  generated_at: string;
  health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | string;
  total_clusters: number;
  healthy_clusters: number;
  degraded_clusters: number;
  unhealthy_clusters: number;
  unknown_clusters: number;
  active_alerts: number;
  critical_alerts: number;
}




export interface IntegrationComponentStatus {
  name: string;
  url: string;
  healthy: boolean;
  status_code: number;
  error?: string;
}

export interface IntegrationStatusData {
  generated_at: string;
  components: IntegrationComponentStatus[];
}

export type AlertPolicyBuilderKind =
  | 'platform_health'
  | 'metrics_template'
  | 'sync_job';

export type AlertPolicyExecutionStatus =
  | 'idle'
  | 'matched'
  | 'sent'
  | 'failed'
  | 'partial';




export interface NotificationRecipientUser {
  id: number;
  username: string;
  nickname: string;
  email: string;
  is_admin: boolean;
}


export interface AlertPolicyCondition {
  metric_key: string;
  operator: string;
  threshold: string;
  window_minutes: number;
}

export interface AlertPolicy {
  id: number;
  name: string;
  description: string;
  policy_type: AlertPolicyBuilderKind | string;
  template_key?: string;
  legacy_rule_key?: string;
  cluster_id?: string;
  severity: AlertSeverity;
  enabled: boolean;
  cooldown_minutes: number;
  send_recovery: boolean;
  conditions: AlertPolicyCondition[];
  notification_channel_ids: number[];
  receiver_user_ids: number[];
  match_count: number;
  delivery_count: number;
  last_matched_at?: string | null;
  last_delivered_at?: string | null;
  last_execution_status: AlertPolicyExecutionStatus | string;
  last_execution_error?: string;
  created_at: string;
  updated_at: string;
  /** 以下字段只用于作业告警策略（sync_job）。 / The fields below are only used by job alert policies (sync_job). */
  task_id?: number;
  task_name?: string;
  notify_task_owner?: boolean;
}

/** 策略列表筛选条件。 / Policy list filters. */
export interface AlertPolicyListFilter {
  policy_type?: AlertPolicyBuilderKind;
  task_id?: number;
}

export interface AlertPolicyListData {
  generated_at: string;
  total: number;
  policies: AlertPolicy[];
}

export interface UpsertAlertPolicyRequest {
  name: string;
  description?: string;
  policy_type: AlertPolicyBuilderKind;
  template_key?: string;
  legacy_rule_key?: string;
  cluster_id?: string;
  severity: AlertSeverity;
  enabled?: boolean;
  cooldown_minutes?: number;
  send_recovery?: boolean;
  conditions?: AlertPolicyCondition[];
  notification_channel_ids?: number[];
  receiver_user_ids?: number[];
  /** 只用于作业告警策略（sync_job），创建后不可修改。/ Only for job alert policies (sync_job); immutable. */
  task_id?: number;
  notify_task_owner?: boolean;
}

export type NotificationChannelType =
  | 'webhook'
  | 'email'
  | 'wecom'
  | 'dingtalk'
  | 'feishu';

export type NotificationEmailSecurity = 'none' | 'starttls' | 'ssl';

export interface NotificationChannelEmailConfig {
  protocol: string;
  security: NotificationEmailSecurity;
  host: string;
  port: number;
  username?: string;
  password?: string;
  from: string;
  from_name?: string;
  recipients: string[];
}

export interface NotificationChannelConfig {
  email?: NotificationChannelEmailConfig | null;
}

export interface NotificationChannel {
  id: number;
  name: string;
  type: NotificationChannelType;
  enabled: boolean;
  endpoint: string;
  secret: string;
  config?: NotificationChannelConfig | null;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface NotificationChannelListData {
  generated_at: string;
  total: number;
  channels: NotificationChannel[];
}

export interface UpsertNotificationChannelRequest {
  name: string;
  type: NotificationChannelType;
  enabled?: boolean;
  endpoint?: string;
  secret?: string;
  /**
   * 清空已保存的签名密钥（secret 为空或占位符时生效）。
   * Clears the stored signing secret (applies when secret is empty or the placeholder).
   */
  clear_secret?: boolean;
  config?: NotificationChannelConfig | null;
  description?: string;
}

export interface NotificationChannelTestResult {
  channel_id: number;
  delivery_id: number;
  status: string;
  receiver?: string;
  sent_at?: string | null;
  last_error?: string;
  status_code?: number;
  response_body?: string;
}

export interface NotificationChannelTestRequest {
  receiver_user_id?: number;
}

export interface NotificationChannelDraftTestRequest {
  channel: UpsertNotificationChannelRequest;
  receiver_user_id?: number;
  /**
   * 正在编辑的已保存渠道；草稿里的脱敏密钥 / 地址由后端用真实值补齐。
   * The saved channel being edited; the backend fills masked secrets / addresses in the draft with the real values.
   */
  channel_id?: number;
}

/**
 * 连接测试请求：渠道草稿 + 可选的已保存渠道 ID。
 * Connection test request: a channel draft plus the optional saved channel ID.
 */
export interface NotificationChannelConnectionTestRequest
  extends UpsertNotificationChannelRequest {
  channel_id?: number;
}

export interface NotificationChannelConnectionTestResult {
  status: string;
  checked_at?: string | null;
  last_error?: string;
}

export interface NotifiableUserListData {
  generated_at: string;
  users: NotificationRecipientUser[];
  default_receiver_user_ids: number[];
}



export type NotificationDeliveryEventType =
  | 'firing'
  | 'resolved'
  | 'test'
  | 'reminder';export type NotificationDeliveryStatus =
  | 'pending'
  | 'sending'
  | 'sent'
  | 'failed'
  | 'retrying'
  | 'canceled'
  | 'exhausted';

export interface NotificationDelivery {
  id: number;
  alert_id: string;
  source_type: string;
  source_key: string;
  policy_id: number;
  cluster_id?: string;
  cluster_name?: string;
  alert_name?: string;
  channel_id: number;
  channel_name?: string;
  event_type: NotificationDeliveryEventType | string;
  status: NotificationDeliveryStatus | string;
  attempt_count: number;
  last_error?: string;
  response_status_code?: number;
  sent_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface NotificationDeliveryListData {
  generated_at: string;
  page: number;
  page_size: number;
  total: number;
  deliveries: NotificationDelivery[];
}

export interface NotificationDeliveryFilterParams {
  policy_id?: number;
  channel_id?: number;
  status?: NotificationDeliveryStatus;
  event_type?: NotificationDeliveryEventType;
  cluster_id?: string;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}


/**
 * 告警实例范围：job 任务级，其余归为 cluster 集群级。
 * Alert instance scope: "job" for job-level alerts, everything else is "cluster".
 */
export type AlertScope = 'cluster' | 'job';

export type ClusterRuleNotifyMode = 'default' | 'custom' | 'none';

/**
 * 集群规则阈值视图：value 为生效值、default 为目录默认值。
 * Cluster rule threshold view: value is the effective value, default the catalog default.
 */
export interface ClusterRuleThreshold {
  // null 表示"等于部署数"语义（worker_insufficient） / null carries the "equals deployed count" semantic (worker_insufficient)
  value: number | null;
  default: number | null;
  unit: string;
  operator: string;
  min: number;
  max: number;
}

/**
 * 一层配置（全局或例外）的用户原值：null 字段表示继承上一层。
 * One raw config layer (global or override): null fields inherit the layer above.
 */
export interface ClusterRuleLayer {
  enabled: boolean | null;
  threshold: number | null;
  for_seconds: number | null;
  severity: AlertSeverity | null;
  notify_mode: ClusterRuleNotifyMode | null;
  channel_ids: number[];
  receiver_user_ids: number[];
}

export interface ClusterRuleOverrideEffective {
  enabled: boolean;
  severity: AlertSeverity;
  threshold: number | null;
  for_seconds: number;
  external_channel_missing: boolean;
}

export interface ClusterRuleOverride extends ClusterRuleLayer {
  cluster_id: number;
  cluster_name: string;
  effective: ClusterRuleOverrideEffective;
}

/**
 * GET /monitoring/cluster-rules 返回的单条集群规则视图：顶层为生效值，global/overrides 为用户原值。
 * One cluster rule view: top level carries effective values, global/overrides the raw layers.
 */
export interface ClusterRule {
  key: string;
  name: string;
  name_en: string;
  description: string;
  description_en?: string;
  scope: 'cluster';
  default_enabled: boolean;
  enabled: boolean;
  severity: AlertSeverity;
  threshold: ClusterRuleThreshold | null;
  for_seconds: number;
  default_for_seconds: number;
  for_editable: boolean;
  notify_mode: ClusterRuleNotifyMode;
  channel_ids: number[];
  receiver_user_ids: number[];
  has_global_config: boolean;
  global: ClusterRuleLayer;
  overrides: ClusterRuleOverride[];
  override_count: number;
  external_channel_missing: boolean;
}

/**
 * PUT /monitoring/cluster-rules/:key[/clusters/:clusterId] 请求体：整层替换，字段省略或 null = 继承。
 * PUT body for one layer: whole-layer replacement, omitted or null fields inherit.
 */
export interface ClusterRuleLayerRequest {
  enabled?: boolean | null;
  threshold?: number | null;
  for_seconds?: number | null;
  severity?: AlertSeverity | null;
  notify_mode?: ClusterRuleNotifyMode | null;
  channel_ids?: number[] | null;
  receiver_user_ids?: number[] | null;
}

export interface AlertNotificationDefaults {
  channel_ids: number[];
  receiver_user_ids: number[];
  cooldown_minutes: number;
  send_recovery: boolean;
  configured: boolean;
}

export type UpdateAlertNotificationDefaultsRequest = Omit<
  AlertNotificationDefaults,
  'configured'
>;

/* ------------------------------------------------------------------ */
/* ------------------------------------------------------------------ */
/* 平台自监控快照 / Platform self-monitoring snapshot                  */
/* ------------------------------------------------------------------ */

/** Server 侧内嵌 TSDB 概况 / Server-side embedded TSDB overview. */
export interface PlatformTSDBStatus {
  head_series?: number;
  blocks_bytes?: number;
  compactions_failed?: number;
}

/** STX server 进程与控制面指标快照 / STX server process & control-plane snapshot. */
export interface PlatformServerSnapshot {
  version?: string;
  uptime_seconds?: number;
  rss_bytes?: number;
  heap_inuse_bytes?: number;
  goroutines?: number;
  /** 近 5 分钟 HTTP 请求速率（req/s） / HTTP request rate over 5m (req/s). */
  http_rps?: number;
  /** 近 5 分钟 5xx 占比（0-1） / 5xx ratio over 5m (0-1). */
  http_5xx_ratio?: number;
  /** 近 5 分钟 HTTP P95 时延（秒） / HTTP P95 latency over 5m (s). */
  http_p95_seconds?: number;
  db_open?: number;
  db_in_use?: number;
  /** 近 5 分钟连接池等待速率 / Connection-pool wait rate over 5m. */
  db_wait_count_rate?: number;
  agents_connected?: number;
  tsdb?: PlatformTSDBStatus;
}

/** 单个 Agent 进程指标快照；旧版 Agent 不上报时指标字段为 null。
 *  Per-Agent process snapshot; metric fields stay null for legacy agents. */
export interface PlatformAgentSnapshot {
  host_id?: number;
  host_name?: string;
  version?: string;
  rss_bytes?: number | null;
  /** CPU 占用核数（rate 值，1.0 = 1 核） / CPU cores in use (rate; 1.0 = one core). */
  cpu_ratio?: number | null;
  goroutines?: number | null;
  uptime_seconds?: number | null;
  /** 近 1 小时失败指令数 / Failed commands over the last hour. */
  command_failures_1h?: number | null;
  /** RFC3339 最近指标上报时间 / RFC3339 timestamp of the last metrics report. */
  last_report_at?: string | null;
  online: boolean;
}

/** 快照携带的固定趋势序列（[毫秒时间戳, 值]）/ Fixed trend series in the snapshot ([epoch ms, value]). */
export interface PlatformTrends {
  rss_bytes?: [number, number][];
  heap_inuse_bytes?: [number, number][];
  http_rps?: [number, number][];
  http_5xx_ratio?: [number, number][];
}

/** GET /api/v1/monitoring/platform 返回的平台快照 / Platform snapshot returned by GET /api/v1/monitoring/platform. */
export interface PlatformSnapshotData {
  available: boolean;
  reason?: string;
  server?: PlatformServerSnapshot;
  agents: PlatformAgentSnapshot[];
  trends?: PlatformTrends;
}
