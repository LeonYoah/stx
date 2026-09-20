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

export type DiagnosticsTabKey = 'errors' | 'inspections' | 'memories';
export type DiagnosticsInspectionReportStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed';
export type DiagnosticsInspectionTriggerSource =
  | 'manual'
  | 'cluster_detail'
  | 'diagnostics_workspace'
  | 'auto';
export type DiagnosticsInspectionFindingSeverity =
  | 'info'
  | 'warning'
  | 'critical';

export interface DiagnosticsWorkspaceTab {
  key: DiagnosticsTabKey;
  label: string;
  description: string;
}

export interface DiagnosticsClusterOption {
  cluster_id: number;
  cluster_name: string;
}

export interface DiagnosticsEntryContext {
  cluster_id?: number;
  cluster_name?: string;
  source?: string;
  alert_id?: string;
}

export interface DiagnosticsWorkspaceBoundary {
  key: string;
  title: string;
  description: string;
}

export interface DiagnosticsWorkspaceBootstrapData {
  generated_at: string;
  default_tab: DiagnosticsTabKey;
  tabs: DiagnosticsWorkspaceTab[];
  cluster_options: DiagnosticsClusterOption[];
  entry_context?: DiagnosticsEntryContext | null;
  boundaries: DiagnosticsWorkspaceBoundary[];
}

export interface DiagnosticsWorkspaceBootstrapParams {
  cluster_id?: number;
  source?: string;
  alert_id?: string;
}

export interface DiagnosticsErrorGroup {
  id: number;
  fingerprint: string;
  fingerprint_version: string;
  title: string;
  exception_class: string;
  sample_message: string;
  occurrence_count: number;
  first_seen_at: string;
  last_seen_at: string;
  last_cluster_id: number;
  last_node_id: number;
  last_host_id: number;
  last_host_name?: string;
  last_host_ip?: string;
}

export interface DiagnosticsErrorEvent {
  id: number;
  error_group_id: number;
  fingerprint: string;
  cluster_id: number;
  node_id: number;
  host_id: number;
  host_name?: string;
  host_ip?: string;
  agent_id: string;
  role: string;
  install_dir: string;
  source_file: string;
  source_kind: string;
  job_id: string;
  occurred_at: string;
  message: string;
  exception_class: string;
  evidence: string;
  cursor_start: number;
  cursor_end: number;
}

export interface DiagnosticsErrorGroupListData {
  items: DiagnosticsErrorGroup[];
  total: number;
  page: number;
  page_size: number;
}

export interface DiagnosticsErrorEventListData {
  items: DiagnosticsErrorEvent[];
  total: number;
  page: number;
  page_size: number;
}

export interface DiagnosticsErrorGroupDetailData {
  group: DiagnosticsErrorGroup;
  events: DiagnosticsErrorEvent[];
}

export interface DiagnosticsErrorGroupListParams {
  cluster_id?: number;
  node_id?: number;
  host_id?: number;
  role?: string;
  job_id?: string;
  keyword?: string;
  exception_class?: string;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface DiagnosticsErrorEventListParams {
  group_id?: number;
  cluster_id?: number;
  node_id?: number;
  host_id?: number;
  role?: string;
  job_id?: string;
  keyword?: string;
  exception_class?: string;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface DiagnosticsInspectionReport {
  id: number;
  cluster_id: number;
  status: DiagnosticsInspectionReportStatus;
  trigger_source: DiagnosticsInspectionTriggerSource;
  lookback_minutes: number;
  error_threshold: number;
  requested_by_user_id: number;
  requested_by: string;
  summary: string;
  error_message: string;
  finding_total: number;
  critical_count: number;
  warning_count: number;
  info_count: number;
  started_at?: string | null;
  finished_at?: string | null;
  created_at: string;
  updated_at: string;
  auto_trigger_reason?: string;
  cluster_name?: string;
}

export interface DiagnosticsInspectionFinding {
  id: number;
  report_id: number;
  cluster_id: number;
  severity: DiagnosticsInspectionFindingSeverity;
  category: string;
  check_code: string;
  check_name: string;
  summary: string;
  recommendation: string;
  evidence_summary: string;
  related_node_id: number;
  related_host_id: number;
  related_host_name?: string;
  related_host_ip?: string;
  related_error_group_id: number;
  related_alert_id: string;
  created_at: string;
  updated_at: string;
}

export interface DiagnosticsInspectionReportListData {
  items: DiagnosticsInspectionReport[];
  total: number;
  page: number;
  page_size: number;
}

export interface DiagnosticsInspectionReportDetailData {
  report: DiagnosticsInspectionReport;
  findings: DiagnosticsInspectionFinding[];
  related_diagnostic_task?: DiagnosticsTask | null;
}

export interface DiagnosticsInspectionReportListParams {
  cluster_id?: number;
  status?: DiagnosticsInspectionReportStatus;
  trigger_source?: DiagnosticsInspectionTriggerSource;
  severity?: DiagnosticsInspectionFindingSeverity;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface StartDiagnosticsInspectionRequest {
  cluster_id: number;
  trigger_source?: DiagnosticsInspectionTriggerSource;
  lookback_minutes?: number;
  error_threshold?: number;
}

export type DiagnosticsTaskStatus =
  | 'pending'
  | 'ready'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'skipped'
  | 'cancelled';

export type DiagnosticsTaskSourceType =
  | 'manual'
  | 'error_group'
  | 'inspection_finding'
  | 'alert';

export type DiagnosticsTaskStepCode =
  | 'COLLECT_ERROR_CONTEXT'
  | 'COLLECT_PROCESS_EVENTS'
  | 'COLLECT_ALERT_SNAPSHOT'
  | 'COLLECT_CONFIG_SNAPSHOT'
  | 'COLLECT_LOG_SAMPLE'
  | 'COLLECT_THREAD_DUMP'
  | 'COLLECT_JVM_DUMP'
  | 'ASSEMBLE_MANIFEST'
  | 'RENDER_HTML_SUMMARY'
  | 'COMPLETE';

export type DiagnosticsLogLevel = 'INFO' | 'WARN' | 'ERROR';

export type DiagnosticsLogEventType =
  | 'started'
  | 'progress'
  | 'success'
  | 'failed'
  | 'note';

export type DiagnosticsTaskEventType =
  | 'snapshot'
  | 'task_updated'
  | 'step_updated'
  | 'node_updated'
  | 'log_appended';

export interface DiagnosticsTaskSourceRef {
  error_group_id?: number;
  inspection_report_id?: number;
  inspection_finding_id?: number;
  alert_id?: string;
}

export interface DiagnosticsTaskOptions {
  include_thread_dump: boolean;
  include_jvm_dump: boolean;
  jvm_dump_min_free_mb?: number;
}

export interface DiagnosticsTaskNodeTarget {
  cluster_node_id: number;
  node_id: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: string;
  agent_id: string;
  install_dir: string;
}

export interface DiagnosticsTaskStep {
  id: number;
  task_id: number;
  code: DiagnosticsTaskStepCode;
  sequence: number;
  title: string;
  description: string;
  status: DiagnosticsTaskStatus;
  message: string;
  error: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DiagnosticsTaskNodeExecution {
  id: number;
  task_id: number;
  task_step_id?: number | null;
  cluster_node_id: number;
  node_id: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: string;
  agent_id: string;
  install_dir: string;
  status: DiagnosticsTaskStatus;
  current_step: DiagnosticsTaskStepCode;
  message: string;
  error: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DiagnosticsTaskLog {
  id: number;
  task_id: number;
  task_step_id?: number | null;
  node_execution_id?: number | null;
  step_code: DiagnosticsTaskStepCode;
  level: DiagnosticsLogLevel;
  event_type: DiagnosticsLogEventType;
  message: string;
  command_summary: string;
  metadata?: Record<string, unknown> | null;
  created_at: string;
}

export interface DiagnosticsTaskSummary {
  id: number;
  cluster_id: number;
  trigger_source: DiagnosticsTaskSourceType;
  status: DiagnosticsTaskStatus;
  lookback_minutes?: number;
  current_step: DiagnosticsTaskStepCode;
  failure_step: DiagnosticsTaskStepCode;
  failure_reason: string;
  summary: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_by: number;
  created_by_name: string;
  created_at: string;
  updated_at: string;
}

export interface DiagnosticsTask {
  id: number;
  cluster_id: number;
  trigger_source: DiagnosticsTaskSourceType;
  source_ref: DiagnosticsTaskSourceRef;
  options: DiagnosticsTaskOptions;
  status: DiagnosticsTaskStatus;
  lookback_minutes?: number;
  current_step: DiagnosticsTaskStepCode;
  failure_step: DiagnosticsTaskStepCode;
  failure_reason: string;
  selected_nodes: DiagnosticsTaskNodeTarget[];
  summary: string;
  bundle_dir: string;
  manifest_path: string;
  index_path: string;
  started_at?: string | null;
  completed_at?: string | null;
  created_by: number;
  created_by_name: string;
  created_at: string;
  updated_at: string;
  steps: DiagnosticsTaskStep[];
  node_executions: DiagnosticsTaskNodeExecution[];
}

export interface DiagnosticsTaskListParams {
  cluster_id?: number;
  trigger_source?: DiagnosticsTaskSourceType;
  status?: DiagnosticsTaskStatus;
  page?: number;
  page_size?: number;
}

export interface DiagnosticsTaskListData {
  items: DiagnosticsTaskSummary[];
  total: number;
  page: number;
  page_size: number;
}

export interface DiagnosticsTaskLogQuery {
  step_code?: DiagnosticsTaskStepCode;
  node_execution_id?: number;
  level?: DiagnosticsLogLevel;
  page?: number;
  page_size?: number;
}

export interface DiagnosticsTaskLogListData {
  items: DiagnosticsTaskLog[];
  total: number;
  page: number;
  page_size: number;
}

export type DiagnosticsTaskNodeScope = 'all' | 'related' | 'custom';

export interface CreateDiagnosticsTaskRequest {
  cluster_id?: number;
  trigger_source?: DiagnosticsTaskSourceType;
  source_ref?: DiagnosticsTaskSourceRef;
  node_scope?: DiagnosticsTaskNodeScope;
  selected_node_ids?: number[];
  options?: DiagnosticsTaskOptions;
  summary?: string;
  lookback_minutes?: number;
  auto_start?: boolean;
}

export interface DiagnosticsTaskEvent {
  task_id: number;
  event_type: DiagnosticsTaskEventType;
  timestamp: string;
  task_status?: DiagnosticsTaskStatus;
  step_status?: DiagnosticsTaskStatus;
  node_status?: DiagnosticsTaskStatus;
  current_step?: DiagnosticsTaskStepCode;
  step_id?: number;
  step_code?: DiagnosticsTaskStepCode;
  node_execution_id?: number;
  host_id?: number;
  host_name?: string;
  level?: DiagnosticsLogLevel;
  log_event_type?: DiagnosticsLogEventType;
  message?: string;
  error?: string;
  command_summary?: string;
  failure_reason?: string;
}

// ─── Auto-Inspection Policy Types ────────────────────────────────────────────

export type InspectionConditionCategory =
  | 'java_error'
  | 'prometheus'
  | 'error_rate'
  | 'node_unhealthy'
  | 'alert_firing'
  | 'schedule';

export interface InspectionConditionTemplate {
  code: string;
  category: InspectionConditionCategory;
  name: string;
  description: string;
  default_threshold: number;
  default_window_minutes: number;
  default_cron_expr: string;
  immediate_on_match: boolean;
}

export interface InspectionConditionItem {
  template_code: string;
  enabled: boolean;
  threshold_override?: number | null;
  window_minutes_override?: number | null;
  cron_expr_override?: string;
  extra_keywords?: string[];
}

export interface InspectionAutoPolicy {
  id: number;
  cluster_id: number;
  name: string;
  enabled: boolean;
  conditions: InspectionConditionItem[];
  cooldown_minutes: number;
  auto_create_task: boolean;
  auto_start_task: boolean;
  task_options: DiagnosticsTaskOptions;
  created_at: string;
  updated_at: string;
}

export interface InspectionAutoPolicyListData {
  items: InspectionAutoPolicy[];
  total: number;
  page: number;
  page_size: number;
}

export interface CreateInspectionAutoPolicyRequest {
  cluster_id: number;
  name: string;
  enabled?: boolean;
  conditions: InspectionConditionItem[];
  cooldown_minutes?: number;
  auto_create_task?: boolean;
  auto_start_task?: boolean;
  task_options?: DiagnosticsTaskOptions;
}

export interface UpdateInspectionAutoPolicyRequest {
  name?: string;
  enabled?: boolean;
  conditions?: InspectionConditionItem[];
  cooldown_minutes?: number;
  auto_create_task?: boolean;
  auto_start_task?: boolean;
  task_options?: DiagnosticsTaskOptions;
}
