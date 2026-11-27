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
 * 全局部署模式与基础设施类型判定单一真源（镜像后端 internal/pkg/infra）：
 * 统一映射 Standalone / Kubernetes Session / Kubernetes Application / 预留 YARN Session & Application，
 * 避免业务组件手写 infra_type !== 'bare_metal' 导致引入 YARN 或 Application 时误判为 K8s。
 * Single source of truth for cluster deployment modes and infrastructure predicates (mirrors Go internal/pkg/infra):
 * normalizes Standalone / Kubernetes Session / Kubernetes Application / reserved YARN modes so business components
 * never compare raw infra_type literals.
 */

export type RuntimePlatform = 'standalone' | 'kubernetes' | 'yarn';
export type ClusterExecutionMode = 'session' | 'application';
export type ClusterDeployMode =
  | 'standalone'
  | 'kubernetes_session'
  | 'kubernetes_application'
  | 'yarn_session'
  | 'yarn_application';

const BARE_METAL_LITERAL = 'bare_metal';
const LEGACY_K8S_LITERAL = 'kubernetes';

// 归一化 infra_type：空值按 bare_metal（Standalone）处理。
// Normalize raw infra_type strings; empty legacy values default to bare_metal (Standalone).
export function normalizeInfraType(infraType?: string | null): string {
  const trimmed = (infraType || '').trim();
  return trimmed || BARE_METAL_LITERAL;
}

export function isBareMetalInfra(infraType?: string | null): boolean {
  return normalizeInfraType(infraType) === BARE_METAL_LITERAL;
}

export function isK8sInfra(infraType?: string | null): boolean {
  const eff = normalizeInfraType(infraType);
  return eff === LEGACY_K8S_LITERAL || eff.startsWith('kubernetes_');
}

export function isK8sSessionInfra(infraType?: string | null): boolean {
  const eff = normalizeInfraType(infraType);
  return eff === 'kubernetes_session' || eff === LEGACY_K8S_LITERAL;
}

export function isYarnInfra(infraType?: string | null): boolean {
  const eff = normalizeInfraType(infraType);
  return eff === 'yarn' || eff.startsWith('yarn_');
}

export function isApplicationInfra(infraType?: string | null): boolean {
  const eff = normalizeInfraType(infraType);
  return (
    eff === 'kubernetes_application_template' ||
    eff === 'kubernetes_application_instance' ||
    eff === 'yarn_application_template' ||
    eff === 'yarn_application_instance'
  );
}

export function isSessionInfra(infraType?: string | null): boolean {
  return !isApplicationInfra(infraType);
}

export function getRuntimePlatform(infraType?: string | null): RuntimePlatform {
  if (isK8sInfra(infraType)) {
    return 'kubernetes';
  }
  if (isYarnInfra(infraType)) {
    return 'yarn';
  }
  return 'standalone';
}

export function getClusterExecutionMode(
  infraType?: string | null,
): ClusterExecutionMode {
  return isApplicationInfra(infraType) ? 'application' : 'session';
}

export function getClusterDeployMode(
  infraType?: string | null,
): ClusterDeployMode {
  const platform = getRuntimePlatform(infraType);
  const isApp = isApplicationInfra(infraType);
  if (platform === 'kubernetes') {
    return isApp ? 'kubernetes_application' : 'kubernetes_session';
  }
  if (platform === 'yarn') {
    return isApp ? 'yarn_application' : 'yarn_session';
  }
  return 'standalone';
}

// 行业通用英文专有名词（对齐 Apache Flink / SeaTunnel MasterType），中英文界面统一展示。
// Canonical industry-standard proper nouns (aligned with Apache Flink / SeaTunnel MasterType).
const DEPLOY_MODE_LABELS: Record<ClusterDeployMode, string> = {
  standalone: 'Standalone',
  kubernetes_session: 'Kubernetes Session',
  kubernetes_application: 'Kubernetes Application',
  yarn_session: 'YARN Session',
  yarn_application: 'YARN Application',
};

export function getClusterDeployModeLabel(infraType?: string | null): string {
  return DEPLOY_MODE_LABELS[getClusterDeployMode(infraType)];
}
