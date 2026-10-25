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

export type SelfUpgradeOverall =
  | 'idle'
  | 'checking'
  | 'available'
  | 'up_to_date'
  | 'staging'
  | 'staged'
  | 'upgrading'
  | 'succeeded'
  | 'failed'
  | 'cancelled';

export interface AssetState {
  id: string;
  name: string;
  description: string;
  required: boolean;
  glob?: boolean;
  status: string;
  size?: number;
  path?: string;
  message?: string;
  progress?: number;
}

export interface StepState {
  code: string;
  label: string;
  status: string;
  message?: string;
  error?: string;
  started_at?: string;
  ended_at?: string;
}

export interface CheckResult {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  release_url?: string;
  checked_at: string;
  message?: string;
  error_code?: string;
  // 检查后仍保留的已预置版本 / staged version kept after the check
  staged_version?: string;
  // 本次检查保留了已预置包 / the check kept the staged package
  stage_preserved?: boolean;
}

export interface StageState {
  mode?: string;
  status: string;
  progress: number;
  target_version?: string;
  assets: AssetState[];
  message?: string;
  error_code?: string;
  cancellable: boolean;
}

// ExecutionMode 显式区分真实应用与演练。/ ExecutionMode explicitly separates a real apply from a dry run.
export type ExecutionMode = 'apply' | 'dry_run';

// ProcessRestart 是一个 STX 自身进程（控制面/随包前端）的重启结果。
// ProcessRestart is the restart result of one STX-owned process (control plane / bundled frontend).
export interface ProcessRestart {
  component: 'control_plane' | 'frontend' | string;
  strategy?: string;
  status:
    | 'pending'
    | 'restarting'
    | 'restarted'
    | 'rolled_back'
    | 'failed'
    | 'skipped'
    | 'planned'
    | string;
  message?: string;
  old_pid?: number;
  new_pid?: number;
  at?: string;
}

// RestartPlanInfo 描述真实应用时会重启哪些进程（SeaTunnel 不受影响）。
// RestartPlanInfo describes which processes an apply restarts (SeaTunnel is unaffected).
export interface RestartPlanInfo {
  control_plane_strategy: string;
  control_plane_detail?: string;
  frontend_detected: boolean;
  frontend_strategy?: string;
  restarts_agents: boolean;
  restarts_proxies: boolean;
  seatunnel_affected: boolean;
}

export interface TaskState {
  id: string;
  kind: string;
  status: string;
  // 执行模式（apply / dry_run）/ execution mode (apply / dry_run)
  execution_mode?: ExecutionMode | string;
  dry_run?: boolean;
  restarts?: ProcessRestart[];
  target_version?: string;
  current_step?: string;
  failure_step?: string;
  failure_reason?: string;
  error_code?: string;
  steps?: StepState[];
  // 各节点 Agent/Proxy 升级状态 / per-node Agent/Proxy upgrade status
  agent_nodes?: AgentNodeUpgrade[];
  cancellable: boolean;
  started_at?: string;
  completed_at?: string;
}

// UpgradeHistoryRecord 记录一次完成或失败的自升级历史。
// UpgradeHistoryRecord records a completed, failed, or rolled back upgrade attempt.
export interface UpgradeHistoryRecord {
  id: string;
  started_at: string;
  completed_at: string;
  duration_ms: number;
  from_version: string;
  target_version: string;
  mode: string;
  // 执行模式；旧记录为空 / execution mode; empty for older records
  execution_mode?: ExecutionMode | string;
  dry_run?: boolean;
  restarts?: ProcessRestart[];
  deployment_type: string;
  status: 'succeeded' | 'failed' | 'rolled_back' | string;
  failure_step?: string;
  failure_reason?: string;
  error_code?: string;
  steps?: StepState[];
  agent_nodes?: AgentNodeUpgrade[];
}

// AgentNodeUpgrade 是单节点 Agent/Proxy 升级状态。
// AgentNodeUpgrade is the Agent/Proxy upgrade status of one node.
export interface AgentNodeUpgrade {
  host_id: number;
  host_name: string;
  ip_address?: string;
  agent_id?: string;
  from_version?: string;
  target_version: string;
  current_version?: string;
  status: 'pending' | 'upgrading' | 'succeeded' | 'failed' | 'rolled_back' | 'offline' | 'skipped' | string;
  message?: string;
  proxy_status?: 'updated' | 'skipped' | 'failed' | string;
  proxy_message?: string;
  // 运行中 stx-java-proxy 的重启结果 / restart result of the running stx-java-proxy
  proxy_restart?: 'restarted' | 'not_running' | 'rolled_back' | 'failed' | 'skipped' | string;
  proxy_restart_message?: string;
  updated_at?: string;
}

// AgentVersionInfo 是节点当前 Agent 版本。/ AgentVersionInfo is a node's current Agent version.
export interface AgentVersionInfo {
  host_id: number;
  host_name: string;
  ip_address?: string;
  version: string;
  online: boolean;
  up_to_date: boolean;
}

export interface StatusResponse {
  current_version: string;
  git_commit?: string;
  build_time?: string;
  latest_version?: string;
  update_available: boolean;
  checked: boolean;
  overall_status: SelfUpgradeOverall | string;
  stage: StageState;
  active_task?: TaskState | null;
  last_check?: CheckResult | null;
  mock_mode: boolean;
  deployment_type?: 'docker' | 'host' | string;
  docker_compose_upgrade_cmd?: string;
  architecture?: string;
  agents?: AgentVersionInfo[];
  agents_outdated?: number;
  restart_plan?: RestartPlanInfo | null;
}

export class SelfUpgradeApiError extends Error {
  errorCode?: string;
  status?: number;
  data?: unknown;

  constructor(message: string, errorCode?: string, status?: number, data?: unknown) {
    super(message);
    this.name = 'SelfUpgradeApiError';
    this.errorCode = errorCode;
    this.status = status;
    this.data = data;
  }
}
