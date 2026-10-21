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

import type {AgentVersionInfo, ExecutionMode, RestartPlanInfo} from './types';

// ExecutePayload 是 /self-upgrade/execute 的请求体：mode 必填，避免"默认是否应用"的歧义。
// ExecutePayload is the /self-upgrade/execute body: mode is required so "does it apply by default" is never ambiguous.
export interface ExecutePayload {
  mode: ExecutionMode;
  simulate_failure?: string;
}

// buildExecutePayload 把按钮动作映射为请求体。/ buildExecutePayload maps a button action to the request body.
export function buildExecutePayload(
  mode: ExecutionMode,
  simulateFailure?: string,
): ExecutePayload {
  return simulateFailure ? {mode, simulate_failure: simulateFailure} : {mode};
}

// UpgradeNodeCounts 是确认框展示的节点数量。/ UpgradeNodeCounts are the node counts shown in the confirm dialog.
export interface UpgradeNodeCounts {
  total: number;
  online: number;
  offline: number;
  // 需要升级（版本与目标不同）的节点数 / nodes not on the target version yet
  toUpgrade: number;
  // 其中离线、重连后才升级的节点数 / of which offline and upgraded on reconnect
  offlineToUpgrade: number;
}

function normalize(v?: string): string {
  const s = (v ?? '').trim();
  return /^[vV]\d/.test(s) ? s.slice(1) : s;
}

export function countUpgradeNodes(
  agents: AgentVersionInfo[] | undefined,
  targetVersion?: string,
): UpgradeNodeCounts {
  const list = agents ?? [];
  const target = normalize(targetVersion);
  const pending = list.filter(
    (a) => !target || normalize(a.version) !== target,
  );
  return {
    total: list.length,
    online: list.filter((a) => a.online).length,
    offline: list.filter((a) => !a.online).length,
    toUpgrade: pending.length,
    offlineToUpgrade: pending.filter((a) => !a.online).length,
  };
}

// RestartingProcess 是确认框中"将会重启的进程"一项。/ RestartingProcess is one "will restart" entry in the confirm dialog.
export type RestartingProcess =
  | 'controlPlane'
  | 'frontend'
  | 'agents'
  | 'proxies';

export function restartingProcesses(
  plan?: RestartPlanInfo | null,
): RestartingProcess[] {
  const out: RestartingProcess[] = ['controlPlane'];
  if (plan?.frontend_detected) {out.push('frontend');}
  if (plan?.restarts_agents ?? true) {out.push('agents');}
  if (plan?.restarts_proxies ?? true) {out.push('proxies');}
  return out;
}
