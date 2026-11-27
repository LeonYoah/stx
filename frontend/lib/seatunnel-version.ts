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
 * SeaTunnel version metadata helpers
 * SeaTunnel 版本元数据辅助方法
 */

import type {
  AvailableVersions,
  SeaTunnelVersionCapabilities,
} from '@/lib/services/installer/types';

export const DEFAULT_SEATUNNEL_INSTALL_DIR_TEMPLATE =
  '/opt/seatunnel-{version}';
export const DEFAULT_SEATUNNEL_DOC_VERSION = '2.3.13';

type SeatunnelVersionMetadata = Partial<
  Pick<AvailableVersions, 'versions' | 'local_packages' | 'recommended_version'>
>;

/**
 * 合并本地安装包与可用版本目录，去重并按版本号降序排列，确保内网手动上传的新版本或自定义版本始终可选。
 * Merge local packages with catalog versions, deduplicate, and sort descending so manually uploaded intranet or custom builds are always selectable.
 */
export function resolveSelectableSeatunnelVersions(
  metadata?: SeatunnelVersionMetadata | null,
  extraVersions?: readonly (string | undefined | null)[],
): string[] {
  const unique = new Set<string>();
  const add = (value?: string | null) => {
    const trimmed = String(value || '').trim();
    if (trimmed) {
      unique.add(trimmed);
    }
  };

  for (const pkg of metadata?.local_packages || []) {
    add(pkg?.version);
  }
  for (const version of metadata?.versions || []) {
    add(version);
  }
  for (const extra of extraVersions || []) {
    add(extra);
  }
  if (unique.size === 0) {
    add(metadata?.recommended_version);
  }

  return Array.from(unique).sort((a, b) => {
    const cmp = compareSeatunnelVersions(b, a);
    return cmp !== 0 ? cmp : a.localeCompare(b);
  });
}

/**
 * 解析默认选中的 SeaTunnel 版本：存在本地安装包时优先选最新本地版本（内网即选即装），否则选最新可用版本。
 * Resolve the default SeaTunnel version: prefer the newest local package when present (ready offline), otherwise pick the newest available version.
 */
export function resolveSeatunnelVersion(
  metadata?: SeatunnelVersionMetadata | null,
): string {
  const localVersions = resolveSelectableSeatunnelVersions({
    local_packages: metadata?.local_packages,
  });
  if (localVersions.length > 0) {
    return localVersions[0];
  }
  const catalogVersions = resolveSelectableSeatunnelVersions({
    versions: metadata?.versions,
  });
  if (catalogVersions.length > 0) {
    return catalogVersions[0];
  }
  return metadata?.recommended_version?.trim() || '';
}

export function buildSeatunnelInstallDir(version?: string): string {
  return version
    ? `/opt/seatunnel-${version}`
    : DEFAULT_SEATUNNEL_INSTALL_DIR_TEMPLATE;
}

export function buildSeatunnelConnectorDocsUrl(
  version?: string,
  locale?: string,
): string {
  const normalizedVersion = version?.trim() || DEFAULT_SEATUNNEL_DOC_VERSION;
  const localePrefix = locale === 'zh' ? '/zh-CN' : '';
  return `https://seatunnel.apache.org${localePrefix}/docs/${normalizedVersion}/connectors/`;
}

export function compareSeatunnelVersions(v1?: string, v2?: string): number {
  const parts1 = String(v1 || '').trim().split('.');
  const parts2 = String(v2 || '').trim().split('.');
  const maxLen = Math.max(parts1.length, parts2.length);

  const parsePart = (part?: string): [number, string] => {
    const value = String(part || '').trim();
    if (!value) {
      return [0, ''];
    }
    const suffixIndex = value.indexOf('-');
    if (suffixIndex === -1) {
      return [Number.parseInt(value, 10) || 0, ''];
    }
    return [
      Number.parseInt(value.slice(0, suffixIndex), 10) || 0,
      value.slice(suffixIndex),
    ];
  };

  for (let index = 0; index < maxLen; index += 1) {
    const [n1, s1] = parsePart(parts1[index]);
    const [n2, s2] = parsePart(parts2[index]);
    if (n1 !== n2) {
      return n1 > n2 ? 1 : -1;
    }
    if (s1 !== s2) {
      if (!s1) {
        return 1;
      }
      if (!s2) {
        return -1;
      }
      return s1 > s2 ? 1 : -1;
    }
  }

  return 0;
}

export function isSeatunnelVersionAtLeast(version: string | undefined, baseline: string): boolean {
  return compareSeatunnelVersions(version, baseline) >= 0;
}

/**
 * 安装向导里事件通道勾选的下一个值。
 * 版本不支持时关掉；用户没动过开关时，支持的版本默认开启。
 * Next event-channel checkbox value in the install wizard.
 * Unsupported versions turn it off; a supported version defaults on until the user changes the switch.
 */
export function nextEventChannelEnabled(
  supported: boolean,
  current: boolean,
  userSet: boolean,
): boolean {
  if (!supported) {
    return false;
  }
  if (userSet) {
    return current;
  }
  return true;
}

export function resolveSeatunnelVersionCapabilities(
  metadata?: Pick<AvailableVersions, 'version_capabilities'> | null,
  version?: string,
): SeaTunnelVersionCapabilities | null {
  if (!metadata || !version) {
    return null;
  }
  return metadata.version_capabilities?.[version] ?? null;
}

/**
 * SeaTunnel 可启动的最高 JDK 主版本（JDK 24+ 因 JEP 486 移除 Subject.getSubject 必崩，与后端保持一致）。
 * Highest JDK major capable of booting SeaTunnel (JDK 24+ removed Subject.getSubject per JEP 486, aligned with backend).
 */
export const SEATUNNEL_MAX_SUPPORTED_JAVA_VERSION = 23;

/** SeaTunnel 版本的 JDK 兼容区间定义。/ JDK compatibility bounds for a SeaTunnel release. */
export interface SeatunnelJavaRequirement {
  minSupported: number;
  recommended: number[];
  maxRecommended: number;
  maxSupported: number;
}

export type SeatunnelJavaCapabilitiesHint = Pick<
  SeaTunnelVersionCapabilities,
  'min_java_version' | 'recommended_java_versions' | 'max_recommended_java_version'
>;

/**
 * 解析目标 SeaTunnel 版本的 JDK 要求；优先读取服务端下发的能力矩阵，缺失时按版本规则兜底（3.1.0+ 需 Java 11+）。
 * Resolve JDK requirements for a SeaTunnel release: prefer server-provided capabilities, falling back to version rules (3.1.0+ requires Java 11+).
 */
export function resolveSeatunnelJavaRequirement(
  version?: string,
  capabilities?: SeatunnelJavaCapabilitiesHint | null,
): SeatunnelJavaRequirement {
  if (capabilities && capabilities.min_java_version > 0) {
    const recommended = capabilities.recommended_java_versions?.length
      ? [...capabilities.recommended_java_versions]
      : [capabilities.min_java_version];
    return {
      minSupported: capabilities.min_java_version,
      recommended,
      maxRecommended:
        capabilities.max_recommended_java_version || Math.max(...recommended),
      maxSupported: SEATUNNEL_MAX_SUPPORTED_JAVA_VERSION,
    };
  }
  if (version?.trim() && isSeatunnelVersionAtLeast(version, '3.1.0')) {
    return {
      minSupported: 11,
      recommended: [11, 17],
      maxRecommended: 17,
      maxSupported: SEATUNNEL_MAX_SUPPORTED_JAVA_VERSION,
    };
  }
  return {
    minSupported: 8,
    recommended: [8, 11],
    maxRecommended: 11,
    maxSupported: SEATUNNEL_MAX_SUPPORTED_JAVA_VERSION,
  };
}

/**
 * 判断候选 JDK 主版本是否可能启动目标 SeaTunnel 版本（未知版本 <=0 按保守可用处理，不硬拦截）。
 * Return true when a candidate JDK major can boot the target SeaTunnel version (unknown <=0 is kept as a warning fallback).
 */
export function isJavaVersionCompatible(
  version: number,
  requirement: SeatunnelJavaRequirement,
): boolean {
  return (
    version <= 0 ||
    (version >= requirement.minSupported && version <= requirement.maxSupported)
  );
}

/**
 * 按目标版本的兼容优先级排序候选 JDK：推荐版本（高版本优先） > 可运行告警版本（贴近推荐上限） > 未知版本 > 已知不兼容版本。
 * Sort JDK candidates for the target version: recommended (newer major first) > supported warning (nearest maxRecommended) > unknown > hard-incompatible.
 */
export function sortJavaCandidatesForVersion<
  T extends {java_home: string; version: number},
>(candidates: readonly T[], requirement: SeatunnelJavaRequirement): T[] {
  const rankOf = (version: number): number => {
    if (requirement.recommended.includes(version)) {
      return 0;
    }
    if (
      version >= requirement.minSupported &&
      version <= requirement.maxSupported
    ) {
      return 1;
    }
    if (version <= 0) {
      return 2;
    }
    return 3;
  };

  return [...candidates].sort((a, b) => {
    const tierDiff = rankOf(a.version) - rankOf(b.version);
    if (tierDiff !== 0) {
      return tierDiff;
    }
    if (rankOf(a.version) === 0 && a.version !== b.version) {
      return b.version - a.version;
    }
    if (
      (rankOf(a.version) === 1 || rankOf(a.version) === 3) &&
      a.version !== b.version
    ) {
      return a.version - b.version;
    }
    return a.java_home.localeCompare(b.java_home);
  });
}

/**
 * 按目标 SeaTunnel 版本选出默认预选的兼容 JDK；若探测到的 JDK 全部明确不兼容则返回 null，避免误选。
 * Pick the default compatible JDK for the target SeaTunnel version; return null if every detected JDK is incompatible.
 */
export function pickDefaultJavaCandidate<
  T extends {java_home: string; version: number},
>(candidates: readonly T[], requirement: SeatunnelJavaRequirement): T | null {
  const ordered = sortJavaCandidatesForVersion(candidates, requirement);
  return (
    ordered.find((candidate) =>
      isJavaVersionCompatible(candidate.version, requirement),
    ) ?? null
  );
}
