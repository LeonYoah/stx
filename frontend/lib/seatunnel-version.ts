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

export function resolveSeatunnelVersion(
  metadata?: Pick<AvailableVersions, 'recommended_version' | 'versions'> | null,
): string {
  return metadata?.recommended_version || metadata?.versions?.[0] || '';
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
