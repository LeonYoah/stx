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

export type SyncJSON = Record<string, unknown>;

export type SyncTaskStatus = 'draft' | 'published' | 'archived';
export type SyncTaskMode = 'streaming' | 'batch';
export type SyncNodeType = 'folder' | 'file';
export type SyncRunType = 'preview' | 'run' | 'recover' | 'schedule';
export type SyncJobStatus =
  | 'pending'
  | 'running'
  | 'cancel_requested'
  | 'cancelling'
  | 'doing_savepoint'
  | 'failing'
  | 'success'
  | 'failed'
  | 'canceled'
  | 'savepoint_done';
export type SyncFormat = 'hocon' | 'json';

export interface SyncTask {
  id: number;
  parent_id?: number;
  node_type: SyncNodeType;
  name: string;
  description: string;
  cluster_id: number;
  engine_version: string;
  mode: SyncTaskMode;
  status: SyncTaskStatus;
  content_format: SyncFormat;
  content: string;
  job_name: string;
  definition: SyncJSON;
  sort_order: number;
  current_version: number;
  schedule_enabled?: boolean;
  schedule_cron_expr?: string;
  schedule_timezone?: string;
  schedule_last_triggered_at?: string;
  schedule_next_triggered_at?: string;
  can_edit?: boolean;
  can_run?: boolean;
  is_owner?: boolean;
  is_collaborator?: boolean;
  is_public?: boolean;
  /** 全局超时默认值，只在任务详情里返回 / Global timeout defaults, returned on task detail only. */
  timeout_defaults?: SyncTaskTimeoutDefaults;
  created_by: number;
  created_at: string;
  updated_at: string;
}

/** 任务设置占位提示用的全局超时默认值（秒）/ Global timeout defaults (seconds) for the task settings placeholders. */
export interface SyncTaskTimeoutDefaults {
  submit_timeout_seconds: number;
  stop_timeout_seconds: number;
}

/**
 * 作业级告警规则键。/ Job-level alert rule keys.
 * - sync_submit_failed：提交失败 / submit failed
 * - sync_submit_timeout：提交超时 / submit timed out
 * - sync_job_failed：作业失败 / job failed
 * - sync_stop_timeout：停止超时 / stop timed out
 */
export type SyncTaskAlertRule =
  | 'sync_submit_failed'
  | 'sync_submit_timeout'
  | 'sync_job_failed'
  | 'sync_stop_timeout';

/** 任务的作业告警配置（GET/PUT /sync/tasks/:id/alert）。/ A task's job alert config (GET/PUT /sync/tasks/:id/alert). */
export interface SyncTaskAlertConfig {
  configured: boolean;
  policy_id?: number;
  enabled: boolean;
  rules: SyncTaskAlertRule[];
  channel_ids: number[];
  receiver_user_ids: number[];
  notify_task_owner: boolean;
  cooldown_minutes: number;
  send_recovery: boolean;
}

/** 保存任务告警配置的请求体；enabled、send_recovery 默认 true，cooldown_minutes 默认 60。 */
/** Request body to save a task alert config; enabled and send_recovery default to true, cooldown_minutes to 60. */
export interface SyncTaskAlertRequest {
  enabled?: boolean;
  rules: SyncTaskAlertRule[];
  channel_ids: number[];
  receiver_user_ids?: number[];
  notify_task_owner?: boolean;
  cooldown_minutes?: number;
  send_recovery?: boolean;
}

export interface SyncTaskTreeNode {
  id: number;
  parent_id?: number;
  node_type: SyncNodeType;
  name: string;
  description: string;
  cluster_id: number;
  engine_version: string;
  mode: SyncTaskMode;
  status: SyncTaskStatus;
  content_format: SyncFormat;
  content: string;
  job_name: string;
  definition: SyncJSON;
  sort_order: number;
  current_version: number;
  schedule_enabled?: boolean;
  schedule_cron_expr?: string;
  schedule_timezone?: string;
  schedule_last_triggered_at?: string;
  schedule_next_triggered_at?: string;
  can_edit?: boolean;
  can_run?: boolean;
  is_owner?: boolean;
  is_collaborator?: boolean;
  is_public?: boolean;
  created_by?: number;
  children?: SyncTaskTreeNode[];
}

export interface SyncTaskVersion {
  id: number;
  task_id: number;
  version: number;
  name_snapshot: string;
  description_snapshot: string;
  cluster_id_snapshot: number;
  engine_version_snapshot: string;
  mode_snapshot: SyncTaskMode;
  content_format_snapshot: SyncFormat;
  content_snapshot: string;
  job_name_snapshot: string;
  definition_snapshot: SyncJSON;
  comment: string;
  created_by: number;
  created_at: string;
}

export interface SyncGlobalVariable {
  id: number;
  key: string;
  value: string;
  value_type?: 'string' | 'secret';
  description: string;
  created_by: number;
  created_at: string;
  updated_at: string;
}

export interface SyncJobLogsResult {
  mode: string;
  source: string;
  logs: string;
  /** Latest metrics snapshot from STX result_preview; not covered by log offset. */
  metrics_log?: string;
  empty_reason?: string;
  cluster_job_log_mode?: string;
  cluster_id?: number;
  next_offset?: string;
  file_size?: number;
  updated_at: string;
}

export interface SyncPreviewDataset {
  name: string;
  catalog?: SyncJSON;
  columns?: string[];
  rows?: SyncJSON[];
  page?: number;
  page_size?: number;
  total?: number;
  updated_at?: string;
}

export interface SyncPreviewTableSnapshot {
  id: number;
  table_path: string;
  columns: string[];
  row_count: number;
  rows?: SyncJSON[];
}

export interface SyncPreviewSnapshot {
  session_id: number;
  job_instance_id: number;
  platform_job_id: string;
  engine_job_id: string;
  status: string;
  empty_reason?: string;
  row_limit: number;
  total_rows: number;
  table_count: number;
  truncated: boolean;
  injected_script?: string;
  content_format?: string;
  tables: SyncPreviewTableSnapshot[];
  selected_table?: SyncPreviewTableSnapshot;
  warnings?: string[];
}

export interface SyncCheckpointRecord {
  pipelineId: number;
  checkpoint?: {
    checkpointId: number;
    checkpointType?: string;
    status?: string;
    triggerTimestamp?: number;
    completedTimestamp?: number;
    durationMillis?: number;
    stateSize?: number;
    failureReason?: string;
  };
}

export interface SyncCheckpointPipelineOverview {
  pipelineId: number;
  counts?: Record<string, number>;
  latestCompleted?: SyncCheckpointRecord['checkpoint'];
  latestFailed?: SyncCheckpointRecord['checkpoint'];
  latestSavepoint?: SyncCheckpointRecord['checkpoint'] | null;
  inProgress?: Array<{
    checkpointId: number;
    checkpointType?: string;
    triggerTimestamp?: number;
    acknowledged?: number;
    total?: number;
  }>;
  history?: SyncCheckpointRecord[];
}

export interface SyncCheckpointOverview {
  jobId: string;
  updatedAt?: number;
  pipelines: SyncCheckpointPipelineOverview[];
}

export interface SyncCheckpointSnapshot {
  job_instance_id: number;
  platform_job_id: string;
  engine_job_id: string;
  status: string;
  empty_reason?: string;
  message?: string;
  overview?: SyncCheckpointOverview;
  history?: SyncCheckpointRecord[];
}

export interface SyncJobInstance {
  id: number;
  task_id: number;
  task_version: number;
  run_type: SyncRunType;
  platform_job_id: string;
  engine_job_id: string;
  recovered_from_instance_id?: number;
  status: SyncJobStatus;
  last_synced_at?: string;
  sync_state?: 'ok' | 'degraded' | 'stale' | 'unknown';
  sync_fail_count?: number;
  sync_lag_ms?: number;
  /** 取消中作业已越过停止截止时间（后端派生）/ A cancelling job has passed its stop deadline (derived by the backend). */
  cancel_overdue?: boolean;
  mode?: SyncTaskMode;
  submit_spec: SyncJSON;
  result_preview: SyncJSON;
  cdc_delay?: SyncJobCDCDelay;
  error_message: string;
  started_at?: string;
  finished_at?: string;
  created_by: number;
  created_at: string;
  updated_at: string;
  external_ref?: string;
  attach_generation?: number;
}

export interface SyncTaskListData {
  total: number;
  items: SyncTask[];
}

export interface SyncTaskTreeData {
  items: SyncTaskTreeNode[];
}

export interface SyncTaskVersionListData {
  total: number;
  items: SyncTaskVersion[];
}

export interface SyncJobListData {
  total: number;
  items: SyncJobInstance[];
}

export interface SyncGlobalVariableListData {
  total: number;
  items: SyncGlobalVariable[];
}

export interface SyncValidateResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
  summary: string;
  resolved?: Record<string, string>;
  detected_vars?: string[];
  detected_files?: string[];
  checks?: SyncValidationCheck[];
}

export interface SyncValidationCheck {
  node_id: string;
  kind: string;
  connector_type: string;
  target?: string;
  probe_method?: string;
  status: string;
  message: string;
}

export interface SyncDagResult {
  nodes: SyncJSON[];
  edges: SyncJSON[];
  warnings?: string[];
  simple_graph?: boolean;
  webui_job?: SyncWebUIDagPreviewJob;
}

export interface SyncWebUIDagEdge {
  inputVertexId: number;
  targetVertexId: number;
}

export interface SyncSinkSaveModePreviewAction {
  phase?: string;
  actionType?: string;
  resultType?: string;
  content?: string;
  native?: boolean;
}

export interface SyncSinkSaveModePreviewTable {
  tablePath?: string;
  supported: boolean;
  completeness?: string;
  schemaSaveMode?: string;
  dataSaveMode?: string;
  actions?: SyncSinkSaveModePreviewAction[];
  warnings?: string[];
}

export interface SyncWebUIDagVertexInfo {
  vertexId: number;
  type: string;
  connectorType: string;
  tablePaths?: string[];
  tableColumns?: Record<string, string[]>;
  tableSchemas?: Record<string, SyncJSON>;
  saveModePreviews?: Record<string, SyncSinkSaveModePreviewTable>;
  saveModeWarnings?: string[];
}

export interface SyncWebUIJobDag {
  jobId: string;
  pipelineEdges: Record<string, SyncWebUIDagEdge[]>;
  vertexInfoMap: Record<string, SyncWebUIDagVertexInfo>;
  envOptions?: SyncJSON;
}

export interface SyncWebUIDagPreviewJob {
  jobId: string;
  jobName: string;
  jobStatus: string;
  errorMsg?: string;
  createTime?: string;
  finishTime?: string;
  jobDag: SyncWebUIJobDag;
  metrics?: SyncJSON;
  pluginJarsUrls?: string[];
  simpleGraph?: boolean;
  warnings?: string[];
}

/**
 * 计划 DAG 不可用原因码（后端稳定常量，前端负责本地化）。
 * Plan DAG unavailability reason codes (stable backend constants, localized by the frontend).
 */
export type SyncPlanDagReason =
  | ''
  | 'no_submitted_content'
  | 'local_mode'
  | 'config_tool_unavailable'
  | 'inspect_failed';

/**
 * 作业提交配置推导的计划 DAG；webui_job 与任务 DAG 的 webui_job 同构。
 * Plan DAG derived from a job's submitted config; webui_job has the same shape as the task DAG's webui_job.
 */
export interface SyncJobPlanDag {
  job_id: number;
  available: boolean;
  reason: SyncPlanDagReason;
  message?: string;
  webui_job?: SyncWebUIDagPreviewJob;
  warnings: string[];
}

/**
 * 实时可观测性不可用原因码（后端稳定常量，前端负责本地化）。
 * Realtime observability unavailability reason codes (stable backend constants, localized by the frontend).
 */
export type SyncRealtimeReason =
  | ''
  | 'version_unsupported'
  | 'not_cluster_mode'
  | 'not_running'
  | 'not_enabled'
  | 'master_unreachable'
  | 'no_engine_job'
  | 'collecting'
  | 'client_unsupported';

/** 瓶颈提示码 / Bottleneck hint codes. */
export type SyncRealtimeHintCode =
  | 'sink_slow'
  | 'transform_busy'
  | 'downstream_backpressure'
  | 'source_idle'
  | 'source_busy';

/** 一条瓶颈提示 / One bottleneck hint. */
export interface SyncRealtimeHint {
  code: SyncRealtimeHintCode;
  severity: 'critical' | 'warning' | 'info';
  vertex_id?: number;
  queue_id?: number;
  buckets: number;
  value: number;
}

/** 引擎原样透传的时间序列点（camelCase 字段）/ Engine time-series point passed through as-is (camelCase fields). */
export type SyncRealtimePoint = Record<string, number>;

/** 节点时间序列 / Vertex time series. */
export interface SyncRealtimeVertex {
  vertex_id: number;
  type: string;
  name: string;
  points: SyncRealtimePoint[];
}

/** 带队列的边时间序列 / Queue-backed edge time series. */
export interface SyncRealtimeEdge {
  queue_id: number;
  target_vertex_id: number;
  points: SyncRealtimePoint[];
}

/** GET /sync/jobs/:id/realtime 返回 / Response of GET /sync/jobs/:id/realtime. */
export interface SyncJobRealtime {
  available: boolean;
  reason: SyncRealtimeReason;
  job_id: number;
  engine_job_id: string;
  cluster_id: number;
  bucket_ms: number;
  window_ms: number;
  from_ms: number;
  to_ms: number;
  fetched_at_ms: number;
  webui_path: string;
  observability: SyncJSON;
  vertices: SyncRealtimeVertex[];
  edges: SyncRealtimeEdge[];
  hints: SyncRealtimeHint[];
}

/** 作业 CDC 延迟摘要（来自事件通道，内存态）/ Job CDC delay summary (from the event channel, in memory). */
export interface SyncJobCDCDelay {
  latest_ms: number;
  max_1m_ms: number;
  at: string;
  fresh: boolean;
}

/** 一条清洗后的引擎事件 / One sanitized engine event. */
export interface SyncEngineEvent {
  id: number;
  cluster_id: number;
  engine_job_id: string;
  job_instance_id?: number;
  event_type: string;
  job_status?: string;
  event_time_ms: number;
  payload: SyncJSON;
  received_at: string;
}

/** 作业事件时间线 / Job event timeline. */
export interface SyncJobEventList {
  total: number;
  events: SyncEngineEvent[];
}

/** 事件通道状态 / Event channel state. */
export type SyncEventChannelState = 'disabled' | 'pending_restart' | 'healthy' | 'degraded';

/** 事件通道写入 seatunnel.yaml 的单项结果 / Per-config outcome of writing seatunnel.yaml. */
export interface SyncEventChannelConfigResult {
  config_id: number;
  host_id?: number;
  template: boolean;
  changed: boolean;
  pushed: boolean;
  pending: boolean;
  error?: string;
}

/** 集群事件通道状态（token 只以 token_set 表示）/ Cluster event channel status (token only as token_set). */
export interface SyncEventChannelStatus {
  cluster_id: number;
  enabled: boolean;
  state: SyncEventChannelState;
  token_set: boolean;
  url: string;
  version: string;
  supported: boolean;
  report_non_terminal: boolean;
  restart_required: boolean;
  config_applied_at?: string;
  config_apply_error?: string;
  last_event_at?: string;
  last_batch_at?: string;
  received_count: number;
  duplicate_count: number;
  dropped_count: number;
  parse_error_count: number;
  auth_fail_count: number;
  expect_missed_at?: string;
  expect_satisfied_at?: string;
  configs?: SyncEventChannelConfigResult[];
}

export interface CreateSyncTaskRequest {
  parent_id?: number;
  node_type?: SyncNodeType;
  name: string;
  description?: string;
  cluster_id?: number;
  engine_version?: string;
  mode?: SyncTaskMode;
  content_format?: SyncFormat;
  content?: string;
  job_name?: string;
  sort_order?: number;
  definition?: SyncJSON;
}

export interface UpdateSyncTaskRequest extends CreateSyncTaskRequest {}

export interface SyncTaskPermissions {
  task_id: number;
  is_public: boolean;
  collaborator_ids: number[];
  can_edit: boolean;
  can_manage: boolean;
  is_owner: boolean;
  is_collaborator: boolean;
}

export interface UpdateSyncTaskPermissionsRequest {
  is_public?: boolean;
  collaborator_ids?: number[];
}

export interface PublishSyncTaskRequest {
  comment?: string;
}

export interface CreateSyncGlobalVariableRequest {
  key: string;
  value?: string;
  value_type?: 'string' | 'secret';
  description?: string;
}

export interface UpdateSyncGlobalVariableRequest extends CreateSyncGlobalVariableRequest {}

export interface PreviewSyncTaskRequest {
  row_limit?: number;
  timeout_minutes?: number;
  draft?: UpdateSyncTaskRequest;
}

export interface SyncTaskActionRequest {
  draft?: UpdateSyncTaskRequest;
  variables?: Record<string, string>;
  external_ref?: string;
  schedule_time?: string;
  close_job?: boolean;
}

export interface SchedulerConnection {
  id: number;
  name: string;
  kind: string;
  base_url: string;
  token_set?: boolean;
  default_project_code?: number;
  api_style?: string;
  dolphin_version?: string;
}

export interface SchedulerConnectionRequest {
  name?: string;
  kind?: string;
  base_url?: string;
  token?: string;
  default_project_code?: number;
  api_style?: string;
  dolphin_version?: string;
}

export interface DolphinProject {
  code: number;
  name: string;
}

export interface DolphinWorkflowSummary {
  code: number;
  name: string;
  release_state?: string;
  version?: number;
}

export interface DolphinWorkflowTask {
  code: number;
  name: string;
  task_type?: string;
  raw_script?: string;
}

export interface DolphinWorkflowRelation {
  pre_task_code: number;
  post_task_code: number;
}

// Dolphin 工作流的一条定时（独立资源，按工作流关联，有自己的上下线状态）。
// One schedule of a Dolphin workflow (a separate resource linked to the workflow, with its own release state).
export interface DolphinScheduleInfo {
  id: number;
  crontab: string;
  timezone: string;
  release_state: string;
}

// 任务配置的 STX 内置定时摘要。/ Summary of the task's configured STX built-in schedule.
export interface DolphinBuiltinSchedule {
  cron_expr: string;
  timezone: string;
  enabled: boolean;
}

export interface DolphinWorkflowDetail {
  code: number;
  name: string;
  version: number;
  release_state?: string;
  tasks: DolphinWorkflowTask[];
  relations?: DolphinWorkflowRelation[];
  // 工作流定时；schedule_error 非空表示读取失败。/ Workflow schedules; schedule_error is set when they could not be read.
  schedules: DolphinScheduleInfo[];
  schedule_error?: string;
}

export interface DolphinScriptResult {
  script: string;
  env_notes: string;
  timeout?: string;
  worker_group: string;
  environment: string;
  variables?: Record<string, string>;
}

export interface TaskSchedulerBinding {
  id: number;
  task_id: number;
  kind: string;
  connection_id: number;
  project_code: number;
  workflow_code: number;
  task_code: number;
  workflow_name?: string;
  console_url?: string;
  pushed_at?: string;
  task_version?: number;
}

export interface DolphinPushRequest {
  mode: 'new' | 'reuse' | 'update';
  connection_id: number;
  project_code?: number;
  workflow_name?: string;
  workflow_code?: number;
  upstreams?: number[];
  downstreams?: number[];
  use_builtin_schedule?: boolean;
  timeout?: string;
  variables?: Record<string, string>;
  worker_group?: string;
  environment?: string;
  dry_run?: boolean;
  expected_version?: number;
}

export interface DolphinPushResult {
  dry_run: boolean;
  mode: string;
  script?: DolphinScriptResult;
  binding?: TaskSchedulerBinding;
  workflow_code?: number;
  task_code?: number;
  version?: number;
  release_state?: string;
  would_offline?: boolean;
  cycle: boolean;
  message?: string;
  warning?: string;
  // 推送前的工作流定时与重新上线后恢复的定时 ID。/ Workflow schedules before the push and IDs restored afterwards.
  schedules?: DolphinScheduleInfo[];
  schedules_restored?: number[];
}

export interface DolphinTaskStatus {
  binding?: TaskSchedulerBinding | null;
  connection?: SchedulerConnection;
  script?: DolphinScriptResult;
  recent_jobs?: SyncJobInstance[];
  builtin_locked?: boolean;
  published_version?: number;
  pushed_version?: number;
  builtin_schedule?: DolphinBuiltinSchedule | null;
}

export interface SyncRecoverJobRequest {
  draft?: UpdateSyncTaskRequest;
}

export type SyncPluginType = 'source' | 'transform' | 'sink' | 'catalog';

export interface SyncPluginFactoryInfo {
  factory_identifier: string;
  class_name?: string;
  origin?: string;
}

export interface SyncPluginFactoryListResult {
  plugin_type: SyncPluginType;
  plugins: SyncPluginFactoryInfo[];
  warnings?: string[];
}

export interface SyncPluginOptionDescriptor {
  key: string;
  type?: string;
  element_type?: string;
  default_value?: unknown;
  description?: string;
  fallback_keys?: string[];
  enum_values?: string[];
  enum_display_values?: string[];
  required_mode?: string;
  condition_expression?: string;
  constraint_group?: string;
  origins?: string[];
  declared_classes?: string[];
  advanced: boolean;
}

export interface SyncPluginOptionSchemaResult {
  plugin_type: SyncPluginType;
  factory_identifier: string;
  options: SyncPluginOptionDescriptor[];
  /** SeaTunnel 3.0 valueConstraints；2.x 为空 */
  value_constraints?: Record<string, unknown>[];
  /** SeaTunnel 3.0 conditionRules；2.x 为空 */
  condition_rules?: Record<string, unknown>[];
  warnings?: string[];
}

export interface SyncPluginTemplateResult {
  plugin_type: SyncPluginType;
  factory_identifier: string;
  content_format: string;
  template: string;
  warnings?: string[];
}

export interface SyncPluginEnumValuesResult {
  plugin_type: SyncPluginType;
  factory_identifier: string;
  option_key: string;
  enum_values: string[];
  warnings?: string[];
}

export interface SyncPluginEnumCatalogPlugin {
  plugin_type: SyncPluginType;
  factory_identifier: string;
  options: SyncPluginOptionDescriptor[];
}

export interface SyncPluginEnumCatalogResult {
  env_options: SyncPluginOptionDescriptor[];
  plugins: SyncPluginEnumCatalogPlugin[];
  warnings?: string[];
}

export type SyncCuratedSection =
  | 'env'
  | 'source'
  | 'transform'
  | 'sink'
  | 'combo';

export interface SyncCuratedTemplateView {
  id?: number;
  builtin_id?: string;
  name: string;
  description?: string;
  section: SyncCuratedSection;
  mode: string;
  pattern: string;
  connectors?: string[];
  content: string;
  enabled: boolean;
  origin: 'builtin' | 'user' | 'override';
  has_override?: boolean;
  owner_user_id?: number;
}

export interface SyncCuratedTemplateListResult {
  items: SyncCuratedTemplateView[];
  total: number;
}

export interface SyncCuratedComboParseResult {
  env: string;
  source: string;
  transform: string;
  sink: string;
  present: SyncCuratedSection[];
  combined: string;
}


/** GET /sync/jobs/:id/timeseries 返回 / Response of GET /sync/jobs/:id/timeseries. */
export interface SyncJobTimeseries {
  source: 'prometheus' | 'none' | string;
  reason?: string;
  range: string;
  step_seconds: number;
  engine_observability_enabled: boolean;
  series: SyncJobTimeseriesSeries[];
  annotations: SyncJobTimeseriesAnnotation[];
}

export interface SyncJobTimeseriesSeries {
  key: string;
  label: string;
  unit: string;
  points: [number, number][];
}

export interface SyncJobTimeseriesAnnotation {
  ts_ms: number;
  kind: string;
  summary: string;
  event_id?: number;
}
