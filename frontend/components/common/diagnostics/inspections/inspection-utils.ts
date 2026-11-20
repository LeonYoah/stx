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
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReportStatus,
} from '@/lib/services/diagnostics';

export type InspectionStatusFilter =
  | 'all'
  | DiagnosticsInspectionReportStatus;

export type InspectionSeverityFilter = 'all' | 'critical' | 'warning' | 'info';

// 回溯预设沿用旧弹窗的档位（分钟）：15 / 30 / 1h / 6h / 24h。
// Lookback presets keep the old dialog's steps (in minutes): 15 / 30 / 1h / 6h / 24h.
export const LOOKBACK_PRESET_MINUTES = [15, 30, 60, 360, 1440] as const;

// 报告状态 → 软徽章色：失败玫瑰、运行中蓝、完成绿、排队灰，沿用旧 Badge 语义。
// Report status → soft chip tone: failed rose, running blue, completed
// emerald, pending muted — same semantics as the old Badge mapping.
export function inspectionStatusTone(status: string): ChipTone {
  switch (status) {
    case 'failed':
      return 'rose';
    case 'running':
      return 'blue';
    case 'completed':
      return 'emerald';
    default:
      return 'muted';
  }
}

// 诊断包任务状态 → 软徽章色：成功绿、失败玫瑰、运行中蓝、其余灰。
// Bundle task status → soft chip tone: succeeded emerald, failed rose,
// running blue, everything else muted.
export function taskStatusTone(status: string): ChipTone {
  switch (status) {
    case 'succeeded':
      return 'emerald';
    case 'failed':
      return 'rose';
    case 'running':
      return 'blue';
    default:
      return 'muted';
  }
}

// 诊断任务 / 步骤状态 → i18n 键（diagnosticsCenter 命名空间下）；旧面板把
// succeeded 显示为「已完成」，保持一致。
// Task / step status → i18n key under diagnosticsCenter; the old panel renders
// succeeded as "已完成", kept identical here.
export function inspectionTaskStatusKey(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'inspections.status.completed';
    case 'pending':
    case 'ready':
    case 'running':
    case 'failed':
    case 'skipped':
    case 'cancelled':
      return `tasks.status.${status}`;
    default:
      return '';
  }
}

const SEVERITY_RANK: Record<string, number> = {
  critical: 3,
  warning: 2,
  info: 1,
};

// 发现项按严重度降序展示（critical > warning > info），未知级别排最后。
// Findings render in descending severity (critical > warning > info); unknown
// levels sort last.
export function sortFindingsBySeverity(
  findings: DiagnosticsInspectionFinding[],
): DiagnosticsInspectionFinding[] {
  return [...findings].sort(
    (a, b) => (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0),
  );
}

// 只有 pending / running 需要 5s 静默轮询。
// Only pending / running reports drive the 5s silent auto-refresh.
export function isLiveInspectionStatus(status: string): boolean {
  return status === 'pending' || status === 'running';
}
