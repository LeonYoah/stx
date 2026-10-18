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

export interface TaskState {
  id: string;
  kind: string;
  status: string;
  target_version?: string;
  current_step?: string;
  failure_step?: string;
  failure_reason?: string;
  error_code?: string;
  steps?: StepState[];
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
  deployment_type: string;
  status: 'succeeded' | 'failed' | 'rolled_back' | string;
  failure_step?: string;
  failure_reason?: string;
  error_code?: string;
  steps?: StepState[];
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
