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
  HostStatus,
  HostType,
  type HostInfo,
  type K8sConnectErrorCode,
  type K8sPermissionMode,
} from '@/lib/services/host/types';

// getHostStatusLabel 按主机类型统一映射状态文案：非物理机处于 pending 时展示「待连接」，
// 避免 Kubernetes/Docker 主机误显示「未安装 Agent」。
// getHostStatusLabel returns a host-type-aware status label: non-bare-metal pending hosts
// render "Pending" instead of misleading "Not Installed".
export function getHostStatusLabel(
  host: Pick<HostInfo, 'host_type' | 'status'>,
  t: (key: string) => string,
): string {
  if (
    host.status === HostStatus.PENDING &&
    host.host_type !== HostType.BARE_METAL
  ) {
    return t('host.statuses.pendingConnection');
  }
  // 唯一合法收口点：已在上文对非物理机 pending 做分支映射。
  // Canonical helper: non-bare-metal pending is already handled above.
  // eslint-disable-next-line no-restricted-syntax
  return t(`host.statuses.${host.status}`);
}

// getK8sPermissionModeLabel 把权限模式枚举映射为可读文案（全面授权 / 最小权限 / 权限不足）。
// getK8sPermissionModeLabel formats K8s permission modes for display.
export function getK8sPermissionModeLabel(
  mode: K8sPermissionMode | undefined,
  t: (key: string) => string,
): string {
  switch (mode) {
    case 'full':
      return t('host.k8sPermissionModeFull');
    case 'minimal':
      return t('host.k8sPermissionModeMinimal');
    case 'insufficient':
      return t('host.k8sPermissionModeInsufficient');
    default:
      return '-';
  }
}

// getK8sConnectErrorHint 将后端分类错误码静态映射到可操作排障提示，供引导弹窗与详情页复用。
// getK8sConnectErrorHint maps classified backend probe error codes to actionable hints.
export function getK8sConnectErrorHint(
  code: K8sConnectErrorCode | undefined,
  t: (key: string) => string,
): string {
  switch (code) {
    case 'timeout':
      return t('host.k8sGuide.errors.timeout');
    case 'tls_error':
      return t('host.k8sGuide.errors.tls_error');
    case 'unauthorized':
      return t('host.k8sGuide.errors.unauthorized');
    case 'unreachable':
      return t('host.k8sGuide.errors.unreachable');
    case 'invalid_config':
      return t('host.k8sGuide.errors.invalid_config');
    default:
      return t('host.k8sGuide.errors.unknown');
  }
}
