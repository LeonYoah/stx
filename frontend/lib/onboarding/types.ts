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
 * STX onboarding checklist persisted state and step ids (issue #109 simplified AC).
 * STX 上手清单持久化状态与步骤 id（#109 简化验收）。
 */

/** Onboarding path / 上手路径 */
export type OnboardingPath = 'existing' | 'new';

/**
 * UI display mode for the floating guide.
 * 浮窗显示三态：展开 / 进度胶囊 / 完全隐藏。
 */
export type OnboardingDisplayMode = 'expanded' | 'collapsed' | 'hidden';

/** Step ids shared across paths (no proxy step) / 跨路径步骤 id（无 Proxy 步） */
export const ONBOARDING_STEP_IDS = {
  resources: 'resources',
  hostsAgents: 'hosts_agents',
  cluster: 'cluster',
  firstScript: 'first_script',
} as const;

export type OnboardingStepId =
  (typeof ONBOARDING_STEP_IDS)[keyof typeof ONBOARDING_STEP_IDS];

/**
 * Per-user onboarding persistence payload (v2 — no plan card fields).
 * 按用户隔离的上手进度载荷（v2：无计划卡片字段）。
 */
export type OnboardingPersistedState = {
  version: 2;
  path: OnboardingPath | null;
  displayMode: OnboardingDisplayMode;
  /**
   * Cluster the user actually opened/created in real flows (not an arbitrary pick).
   * 用户在真实流程中打开/创建的集群（禁止随意挑第一个凑进度）。
   */
  targetClusterId: number | null;
  /**
   * Finished latch: once true, guide stays closed despite later Agent outages.
   * 完成闩锁：为 true 后不因后续 Agent 离线复活引导。
   */
  finished: boolean;
  /** Step ids completed at finish time (informational) / 完成时的步骤快照 */
  completedStepIds: string[];
};

export const DEFAULT_ONBOARDING_STATE: OnboardingPersistedState = {
  version: 2,
  path: null,
  displayMode: 'expanded',
  targetClusterId: null,
  finished: false,
  completedStepIds: [],
};

export type StepCheckStatus =
  | 'checking'
  | 'incomplete'
  | 'complete'
  | 'error';

export type StepCheckResult = {
  id: OnboardingStepId;
  status: StepCheckStatus;
  /** Missing host names / ids for multi-host messaging / 多主机缺失说明 */
  missing?: string[];
  errorMessage?: string;
  /** Extra flags for UI / 额外展示字段 */
  meta?: Record<string, string | boolean | number | null>;
};
