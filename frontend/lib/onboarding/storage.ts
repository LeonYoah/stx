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

import {
  DEFAULT_ONBOARDING_STATE,
  type OnboardingPersistedState,
} from './types';

/** localStorage key prefix; always append user.id / 键前缀，必须附加 user.id */
export const ONBOARDING_STORAGE_PREFIX = 'stx:onboarding:v1:';

/**
 * Build per-user storage key.
 * 构造按用户隔离的存储键。
 */
export function onboardingStorageKey(userId: number): string {
  return `${ONBOARDING_STORAGE_PREFIX}${userId}`;
}

function isDisplayMode(value: unknown): value is OnboardingPersistedState['displayMode'] {
  return value === 'expanded' || value === 'collapsed' || value === 'hidden';
}

function isPath(value: unknown): value is OnboardingPersistedState['path'] {
  return value === 'existing' || value === 'new' || value === null;
}

/**
 * Migrate/sanitize persisted JSON to v2; drop legacy plan fields so they cannot lock or false-complete.
 * 将持久化 JSON 迁移/清洗为 v2；丢弃旧计划字段，禁止其锁死或误完成。
 */
export function parseOnboardingState(raw: string | null): OnboardingPersistedState {
  if (!raw) {
    return {...DEFAULT_ONBOARDING_STATE};
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (!parsed || (parsed.version !== 1 && parsed.version !== 2)) {
      return {...DEFAULT_ONBOARDING_STATE};
    }
    // Keep only v2-relevant fields; ignore plannedVersion/plugins/hosts/proxyAcknowledged/fingerprint.
    // 只保留 v2 字段；忽略计划版本/插件/主机/Proxy 已了解/指纹。
    const path = isPath(parsed.path) ? parsed.path : null;
    const displayMode = isDisplayMode(parsed.displayMode)
      ? parsed.displayMode
      : 'expanded';
    const targetClusterId =
      typeof parsed.targetClusterId === 'number' && parsed.targetClusterId > 0
        ? parsed.targetClusterId
        : null;
    const finished = !!parsed.finished;
    const completedStepIds = Array.isArray(parsed.completedStepIds)
      ? parsed.completedStepIds.filter(
          (id): id is string => typeof id === 'string' && id !== 'proxy',
        )
      : [];
    return {
      version: 2,
      path,
      displayMode,
      targetClusterId,
      finished,
      completedStepIds,
    };
  } catch {
    return {...DEFAULT_ONBOARDING_STATE};
  }
}

/**
 * Load onboarding state for a user from localStorage.
 * 从 localStorage 加载指定用户的上手状态。
 */
export function loadOnboardingState(userId: number): OnboardingPersistedState {
  if (typeof window === 'undefined') {
    return {...DEFAULT_ONBOARDING_STATE};
  }
  try {
    return parseOnboardingState(
      window.localStorage.getItem(onboardingStorageKey(userId)),
    );
  } catch {
    return {...DEFAULT_ONBOARDING_STATE};
  }
}

/**
 * Persist onboarding state for a user (always writes v2 shape).
 * 持久化指定用户的上手状态（始终写入 v2 形状）。
 */
export function saveOnboardingState(
  userId: number,
  state: OnboardingPersistedState,
): void {
  if (typeof window === 'undefined') {
    return;
  }
  try {
    const payload: OnboardingPersistedState = {
      version: 2,
      path: state.path,
      displayMode: state.displayMode,
      targetClusterId: state.targetClusterId,
      finished: state.finished,
      completedStepIds: state.completedStepIds.filter((id) => id !== 'proxy'),
    };
    window.localStorage.setItem(
      onboardingStorageKey(userId),
      JSON.stringify(payload),
    );
  } catch {
    // ignore quota / private mode
  }
}
