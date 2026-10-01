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
 * SeaTunnel Upgrade Service Types
 * SeaTunnel 升级服务类型定义
 */

export type PlanStatus =
  | 'draft'
  | 'ready'
  | 'blocked'
  | 'applied'
  | 'superseded';

export type ExecutionStatus =
  | 'pending'
  | 'ready'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'blocked'
  | 'skipped'
  | 'rollback_running'
  | 'rollback_succeeded'
  | 'rollback_failed'
  | 'cancelled';

export type StepCode =
  | 'PRECHECK_PACKAGE'
  | 'PRECHECK_CONNECTOR'
  | 'FREEZE_JOBS'
  | 'SAVEPOINT'
  | 'BACKUP'
  | 'DISTRIBUTE_PACKAGE'
  | 'SYNC_LIB'
  | 'SYNC_CONNECTORS'
  | 'SYNC_PLUGINS'
  | 'MERGE_CONFIG'
  | 'STOP_CLUSTER'
  | 'SWITCH_VERSION'
  | 'START_CLUSTER'
  | 'RESTART_JAVA_PROXY'
  | 'HEALTH_CHECK'
  | 'SMOKE_TEST'
  | 'COMPLETE'
  | 'ROLLBACK_PREPARE'
  | 'ROLLBACK_RESTORE'
  | 'ROLLBACK_RESTART'
  | 'ROLLBACK_VERIFY'
  | 'FAILED';

export type LogLevel = 'INFO' | 'WARN' | 'ERROR';
export type LogEventType =
  | 'started'
  | 'progress'
  | 'success'
  | 'failed'
  | 'rollback'
  | 'note';
export type CheckCategory = 'package' | 'connector' | 'node' | 'config';
export type ConfigConflictStatus = 'pending' | 'resolved';
export type AssetSource =
  | 'local_package'
  | 'uploaded_package'
  | 'mirror_download'
  | 'local_plugin'
  | 'manual_import';

export interface PackageManifest {
  version: string;
  file_name: string;
  local_path: string;
  checksum: string;
  arch: string;
  size_bytes: number;
  source: AssetSource;
  mirror?: string;
}

export interface ConnectorArtifact {
  plugin_name: string;
  artifact_id: string;
  version: string;
  category?: string;
  file_name: string;
  local_path: string;
  checksum?: string;
  source: AssetSource;
  required: boolean;
}

export interface LibraryArtifact {
  group_id: string;
  artifact_id: string;
  version: string;
  file_name: string;
  local_path: string;
  checksum?: string;
  source: AssetSource;
  scope?: string;
}

export interface PluginDependencyArtifact {
  group_id: string;
  artifact_id: string;
  version: string;
  file_name: string;
  local_path: string;
  checksum?: string;
  source: AssetSource;
  target_dir: string;
  relative_path: string;
}

export interface ConnectorManifest {
  version: string;
  replacement_mode: string;
  connectors: ConnectorArtifact[];
  libraries: LibraryArtifact[];
  plugin_dependencies: PluginDependencyArtifact[];
}

export interface ConfigConflict {
  id: string;
  config_type: string;
  path: string;
  base_value?: string;
  local_value?: string;
  target_value?: string;
  resolved_value?: string;
  status: ConfigConflictStatus;
}

export interface ConfigMergeFile {
  config_type: string;
  target_path: string;
  base_content: string;
  local_content: string;
  target_content: string;
  merged_content: string;
  conflict_count: number;
  resolved: boolean;
  conflicts: ConfigConflict[];
}

export interface ConfigMergePlan {
  ready: boolean;
  has_conflicts: boolean;
  conflict_count: number;
  files: ConfigMergeFile[];
  generated_at: string;
}

export interface NodeTarget {
  cluster_node_id: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: string;
  arch: string;
  source_version: string;
  target_version: string;
  source_install_dir: string;
  target_install_dir: string;
}

export interface PlanStep {
  sequence: number;
  code: StepCode;
  title: string;
  description: string;
  required: boolean;
}

export interface UpgradePlanSnapshot {
  cluster_id: number;
  deployment_mode?: string;
  source_version: string;
  target_version: string;
  package_manifest: PackageManifest;
  connector_manifest: ConnectorManifest;
  config_merge_plan: ConfigMergePlan;
  node_targets: NodeTarget[];
  steps: PlanStep[];
  generated_at: string;
}

export interface BlockingIssue {
  category: CheckCategory;
  code: string;
  message: string;
  blocking: boolean;
  metadata?: Record<string, string>;
}

export interface PrecheckResult {
  cluster_id: number;
  target_version: string;
  ready: boolean;
  issues: BlockingIssue[];
  package_manifest?: PackageManifest;
  connector_manifest?: ConnectorManifest;
  config_merge_plan?: ConfigMergePlan;
  node_targets: NodeTarget[];
  generated_at: string;
}

export interface PrecheckRequest {
  cluster_id: number;
  target_version: string;
  package_checksum?: string;
  package_arch?: string;
  connector_names?: string[];
  node_ids?: number[];
  target_install_dir?: string;
}

export interface CreatePlanRequest extends PrecheckRequest {
  config_merge_plan?: ConfigMergePlan;
}

export interface ExecutePlanRequest {
  plan_id: number;
}

export interface ListUpgradeTasksQuery {
  cluster_id?: number;
  page?: number;
  page_size?: number;
}

export interface UpgradePlanRecord {
  id: number;
  cluster_id: number;
  source_version: string;
  target_version: string;
  status: PlanStatus;
  blocking_issue_count: number;
  snapshot: UpgradePlanSnapshot;
  created_by: number;
  created_at: string;
  updated_at: string;
}

export interface CreatePlanResult {
  precheck: PrecheckResult;
  plan?: UpgradePlanRecord;
}

export interface UpgradeTaskStep {
  id: number;
  task_id: number;
  code: StepCode;
  sequence: number;
  status: ExecutionStatus;
  message: string;
  error: string;
  retry_count: number;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface UpgradeNodeExecution {
  id: number;
  task_id: number;
  task_step_id?: number | null;
  cluster_node_id: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: string;
  status: ExecutionStatus;
  current_step: StepCode;
  source_version: string;
  target_version: string;
  source_install_dir: string;
  target_install_dir: string;
  message: string;
  error: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface UpgradeTask {
  id: number;
  cluster_id: number;
  plan_id: number;
  source_version: string;
  target_version: string;
  status: ExecutionStatus;
  current_step: StepCode;
  failure_step?: StepCode;
  failure_reason: string;
  rollback_status: ExecutionStatus;
  rollback_reason: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_by: number;
  created_at: string;
  updated_at: string;
  plan: UpgradePlanRecord;
  steps: UpgradeTaskStep[];
  node_executions: UpgradeNodeExecution[];
}

export type UpgradeTaskSummary = Omit<
  UpgradeTask,
  'plan' | 'steps' | 'node_executions'
>;

export interface UpgradeStepLog {
  id: number;
  task_id: number;
  task_step_id?: number | null;
  node_execution_id?: number | null;
  step_code: StepCode;
  level: LogLevel;
  event_type: LogEventType;
  message: string;
  command_summary: string;
  exit_code?: number | null;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface TaskStepsData {
  task_id: number;
  steps: UpgradeTaskStep[];
  node_executions: UpgradeNodeExecution[];
}

export interface TaskLogsQuery {
  step_code?: StepCode;
  node_execution_id?: number;
  level?: LogLevel;
  page?: number;
  page_size?: number;
}

export interface TaskLogsData {
  task_id: number;
  total: number;
  page: number;
  page_size: number;
  items: UpgradeStepLog[];
}

export interface TaskListData {
  total: number;
  page: number;
  page_size: number;
  items: UpgradeTaskSummary[];
}

export type TaskEventType =
  | 'snapshot'
  | 'task_updated'
  | 'step_updated'
  | 'node_updated'
  | 'log_appended';

export interface TaskEvent {
  task_id: number;
  event_type: TaskEventType;
  timestamp: string;
  task_status?: ExecutionStatus;
  step_status?: ExecutionStatus;
  node_status?: ExecutionStatus;
  rollback_status?: ExecutionStatus;
  current_step?: StepCode;
  step_id?: number;
  step_code?: StepCode;
  node_execution_id?: number;
  host_id?: number;
  host_name?: string;
  level?: LogLevel;
  log_event_type?: LogEventType;
  message?: string;
  error?: string;
  command_summary?: string;
  failure_reason?: string;
  rollback_reason?: string;
}

export interface ServiceResult<T> {
  success: boolean;
  data?: T;
  error?: string;
}
