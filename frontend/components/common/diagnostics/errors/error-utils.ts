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

import type {ChipTone} from '@/components/common/layout';
import type {
  DiagnosticsClusterOption,
  DiagnosticsErrorGroupListParams,
} from '@/lib/services/diagnostics';
import {formatDateTimeTz} from '@/lib/timezone';

export type ErrorHeatFilter = 'all' | 'high' | 'mid' | 'low';

// 频次分桶与服务端 min/max_occurrences 闭区间对齐：高 ≥10、中 3–9、低 <3。
// Heat buckets map onto the server-side min/max_occurrences closed range:
// high ≥10, mid 3–9, low <3.
export function heatFilterToOccurrenceRange(
  filter: ErrorHeatFilter,
): Pick<
  DiagnosticsErrorGroupListParams,
  'min_occurrences' | 'max_occurrences'
> {
  switch (filter) {
    case 'high':
      return {min_occurrences: 10};
    case 'mid':
      return {min_occurrences: 3, max_occurrences: 9};
    case 'low':
      return {max_occurrences: 2};
    default:
      return {};
  }
}

// 频次热度配色沿用旧面板的阈值：≥10 警示、≥3 预警、其余中性。
// Heat tones keep the old panel's thresholds: ≥10 danger, ≥3 warning, else neutral.
export function occurrenceHeatTone(count: number): ChipTone {
  if (count >= 10) {
    return 'rose';
  }
  if (count >= 3) {
    return 'amber';
  }
  return 'muted';
}

// 来源节点按 host 名称 / IP / 编号 + 角色 + 节点号拼「a · b · c」。
// Origin node is joined as "a · b · c" from host name / IP / id plus role and node id.
export function formatNodeOrigin(options: {
  nodeId?: number | null;
  hostId?: number | null;
  hostName?: string | null;
  hostIp?: string | null;
  role?: string | null;
}): string {
  const parts: string[] = [];
  if (options.hostName?.trim()) {
    parts.push(options.hostName.trim());
  } else if (options.hostIp?.trim()) {
    parts.push(options.hostIp.trim());
  } else if (options.hostId) {
    parts.push(`#${options.hostId}`);
  }
  if (options.role?.trim()) {
    parts.push(options.role.trim());
  }
  if (options.nodeId) {
    parts.push(`node #${options.nodeId}`);
  }
  return parts.length > 0 ? parts.join(' · ') : '-';
}

// 错误组归属集群按其 last_cluster_id 在集群选项里反查名称，查不到回退 #id。
// The owning cluster of an error group resolves its name through cluster
// options by last_cluster_id, falling back to #id.
export function resolveGroupClusterLabel(
  lastClusterId: number,
  clusterOptions: DiagnosticsClusterOption[],
): string {
  if (!lastClusterId) {
    return '-';
  }
  return (
    clusterOptions.find((c) => c.cluster_id === lastClusterId)?.cluster_name ??
    `#${lastClusterId}`
  );
}

// 非法或缺失时间回退原值 / '-'，与旧面板一致。
// Invalid or missing timestamps fall back to the raw value / '-', same as before.
export function formatErrorDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatDateTimeTz(parsed);
}
