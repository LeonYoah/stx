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
 * Helpers for JDBC (and similar) dependency profile selection across download/install.
 * 下载与安装流程中 JDBC（及同类）依赖场景画像选择的共用辅助。
 */

import type {Plugin} from '@/lib/services/plugin';

/** 归一化画像键：去空白、去重、排序。 / Normalize profile keys: trim, dedupe, sort. */
export function normalizeProfileKeys(keys?: string[] | null): string[] {
  return Array.from(
    new Set((keys || []).map((item) => item.trim()).filter(Boolean)),
  ).sort();
}

/**
 * Whether this plugin requires scenario profile selection before download.
 * 该插件是否必须在下载前选择场景画像。
 */
export function pluginRequiresProfileSelection(
  plugin: Pick<Plugin, 'name' | 'artifact_id'> | null | undefined,
): boolean {
  if (!plugin) {
    return false;
  }
  return plugin.artifact_id === 'connector-jdbc' || plugin.name === 'jdbc';
}

export type ResolveInstallProfileKeysInput = {
  /** 下载产物元数据中的画像 / Profiles stored on the downloaded artifact */
  artifactKeys?: string[] | null;
  /** 会话内用户勾选的画像 / In-session user selection */
  memoryKeys?: string[] | null;
};

/**
 * Resolve profiles for the install dialog: prefer downloaded-artifact keys, else memory.
 * 解析安装弹窗预填画像：优先用已下载产物上的键，否则用会话勾选。
 */
export function resolveInstallProfileKeys(
  input: ResolveInstallProfileKeysInput,
): string[] {
  const artifact = normalizeProfileKeys(input.artifactKeys);
  if (artifact.length > 0) {
    return artifact;
  }
  return normalizeProfileKeys(input.memoryKeys);
}

/**
 * Merge downloaded artifact profiles into the in-memory selection map (artifact wins).
 * 将已下载产物的画像并入会话映射（产物优先覆盖）。
 */
export function mergeArtifactProfilesIntoMemory(
  prev: Record<string, string[]>,
  artifacts: Array<{name: string; selected_profile_keys?: string[] | null}>,
): Record<string, string[]> {
  const next = {...prev};
  for (const item of artifacts) {
    const keys = normalizeProfileKeys(item.selected_profile_keys);
    if (keys.length === 0) {
      continue;
    }
    next[item.name] = keys;
  }
  return next;
}
