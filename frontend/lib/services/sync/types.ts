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
  | 'success'
  | 'failed'
  | 'canceled';
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
  created_by: number;
  created_at: string;
  updated_at: string;
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
  submit_spec: SyncJSON;
  result_preview: SyncJSON;
  error_message: string;
  started_at?: string;
  finished_at?: string;
  created_by: number;
  created_at: string;
  updated_at: string;
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
