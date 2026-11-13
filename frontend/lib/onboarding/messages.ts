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

import type {StepCheckStatus} from './types';

const ERROR_KEYS = new Set([
  'package_missing',
  'plugin_missing',
  'hosts_missing',
  'hosts_not_connected',
  'bound_nodes_empty',
  'hosts_fetch_failed',
  'hosts_page_size_invalid',
  'clusters_fetch_failed',
  'clusters_page_size_invalid',
  'list_page_size_invalid',
  'packages_fetch_failed',
  'plugins_fetch_failed',
  'cluster_status_failed',
  'target_unbound',
  'cluster_ambiguous',
  'nodes_empty',
  'nodes_not_ready',
  'job_not_running_or_success',
  'generic',
]);

type Translate = ((
  key: string,
  values?: Record<string, string | number>,
) => string) & {has?: (key: string) => boolean};

/**
 * User-facing step detail: prefer concrete shortage copy over raw "incomplete".
 * 步骤用户文案：优先具体缺项，禁止裸「未完成」/机器码。
 */
export function formatStepStatusDetail(
  t: Translate,
  status: StepCheckStatus,
  errorMessage?: string,
  missing?: string[],
): string {
  if (status === 'checking') {
    return t('checking');
  }
  if (status === 'complete') {
    return '';
  }
  if (status === 'error') {
    if (errorMessage) {
      const errorKey = `errors.${errorMessage}`;
      const known =
        ERROR_KEYS.has(errorMessage) && (!t.has || t.has(errorKey));
      if (known) {
        return t(errorKey);
      }
    }
    return t('checkFailed');
  }
  // incomplete — concrete shortage messages / 未完成：具体缺项
  const missingValues =
    missing && missing.length > 0
      ? {count: missing.length, hosts: missing.join(', ')}
      : undefined;
  if (errorMessage) {
    const errorKey = `errors.${errorMessage}`;
    const known =
      ERROR_KEYS.has(errorMessage) && (!t.has || t.has(errorKey));
    // Pass count so en ICU plurals resolve (one/other).
    // 传入 count，英文 ICU 单复数才能正确解析。
    let detail = known ? t(errorKey, missingValues) : t('errors.generic');
    if (missingValues) {
      detail = `${detail} ${t('missingHosts', missingValues)}`;
    }
    return detail;
  }
  if (missingValues) {
    return t('missingHosts', missingValues);
  }
  return t('errors.generic');
}

/**
 * Translate shell lastError codes; never surface backend validation prose.
 * 将外壳 lastError 码译为用户文案，禁止直显后端校验原文。
 */
export function formatOnboardingLastError(
  t: Translate,
  lastError: string | null | undefined,
): string | null {
  if (!lastError) {
    return null;
  }
  const errorKey = `errors.${lastError}`;
  const known = ERROR_KEYS.has(lastError) && (!t.has || t.has(errorKey));
  if (known) {
    return t(errorKey);
  }
  return t('checkFailed');
}
