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

import {sanitizeConfigMergePlan} from '@/lib/services/st-upgrade';
import type {
  CheckCategory,
  ConfigConflict,
  ConfigMergeFile,
  ConfigMergePlan,
  ExecutionStatus,
  PlanStep,
  StepCode,
  TaskStepsData,
  UpgradeTaskStep,
} from '@/lib/services/st-upgrade';

const VISUAL_CONFLICT_PREFIX = 'visual:';
const VISUAL_DELETE_SENTINEL = '__ST_UPGRADE_DELETE_LINE__';

export interface MergeEditorRow {
  id: string;
  path: string;
  oldLineNumber: number | null;
  newLineNumber: number | null;
  resultLineNumber: number;
  oldValue: string;
  newValue: string;
  resultValue: string;
  resultPresent: boolean;
  changed: boolean;
  resolved: boolean;
}

export function getExecutionStatusLabel(status: ExecutionStatus): string {
  switch (status) {
    case 'succeeded':
    case 'rollback_succeeded':
      return '成功';
    case 'failed':
      return '失败';
    case 'blocked':
      return '阻断';
    case 'running':
      return '执行中';
    case 'rollback_running':
      return '回滚中';
    case 'rollback_failed':
      return '回滚失败';
    case 'ready':
      return '就绪';
    case 'skipped':
      return '跳过';
    case 'cancelled':
      return '已取消';
    case 'pending':
    default:
      return '待执行';
  }
}

/** Arc Badge 色调，对应执行状态。成功绿、失败红、进行中黄。 / Arc badge tone for an execution status. */
export function upgradeStatusTone(
  status: ExecutionStatus,
): 'neutral' | 'success' | 'warning' | 'danger' {
  switch (status) {
    case 'succeeded':
    case 'rollback_succeeded':
      return 'success';
    case 'failed':
    case 'rollback_failed':
    case 'blocked':
      return 'danger';
    case 'running':
    case 'rollback_running':
      return 'warning';
    case 'cancelled':
    case 'skipped':
    case 'ready':
    case 'pending':
    default:
      return 'neutral';
  }
}

export function getStatusBadgeVariant(
  status: ExecutionStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'succeeded':
    case 'rollback_succeeded':
      return 'default';
    case 'failed':
    case 'rollback_failed':
    case 'blocked':
      return 'destructive';
    case 'running':
    case 'rollback_running':
    case 'cancelled':
      return 'outline';
    case 'skipped':
    case 'ready':
    case 'pending':
    default:
      return 'secondary';
  }
}

export function getIssueCategoryLabel(category: CheckCategory): string {
  switch (category) {
    case 'package':
      return '安装包';
    case 'connector':
      return '连接器';
    case 'node':
      return '节点';
    case 'config':
      return '配置';
    default:
      return category;
  }
}

function normalizeConflictStatus(conflict: ConfigConflict): ConfigConflict {
  return {
    ...conflict,
    status: conflict.status === 'resolved' ? 'resolved' : 'pending',
  };
}

function normalizeComparableLine(value: string): string {
  return value.trim();
}

function areLinesEquivalent(left?: string, right?: string): boolean {
  return (
    normalizeComparableLine(left || '') === normalizeComparableLine(right || '')
  );
}

function isVisualConflict(conflict: ConfigConflict): boolean {
  return conflict.id.startsWith(VISUAL_CONFLICT_PREFIX);
}

function containsLegacyConflictMarkers(content: string): boolean {
  return (
    content.includes('<<<<<<< LOCAL') &&
    content.includes('||||||| BASE') &&
    content.includes('>>>>>>> TARGET')
  );
}

function splitConfigLines(content: string): string[] {
  const normalized = content.replace(/\r\n/g, '\n');
  if (!normalized) {
    return [];
  }
  return normalized.split('\n');
}

function buildVisualConflictId(configType: string, rowIndex: number): string {
  return `${VISUAL_CONFLICT_PREFIX}${configType}:${rowIndex}`;
}

function buildVisualConflictPath(
  oldLineNumber: number | null,
  newLineNumber: number | null,
): string {
  if (oldLineNumber !== null && newLineNumber !== null) {
    if (oldLineNumber === newLineNumber) {
      return `line ${oldLineNumber}`;
    }
    return `old ${oldLineNumber} / new ${newLineNumber}`;
  }
  if (oldLineNumber !== null) {
    return `old ${oldLineNumber}`;
  }
  if (newLineNumber !== null) {
    return `new ${newLineNumber}`;
  }
  return 'line ?';
}

interface AlignedLineRow {
  oldLineNumber: number | null;
  newLineNumber: number | null;
  oldValue: string;
  newValue: string;
  changed: boolean;
}

type DiffOp =
  | {type: 'equal'; oldIndex: number; newIndex: number}
  | {type: 'delete'; oldIndex: number}
  | {type: 'insert'; newIndex: number};

function buildAlignedLineRows(
  oldContent: string,
  newContent: string,
): AlignedLineRow[] {
  const oldLines = splitConfigLines(oldContent);
  const newLines = splitConfigLines(newContent);
  const oldComparable = oldLines.map(normalizeComparableLine);
  const newComparable = newLines.map(normalizeComparableLine);
  const dp = Array.from({length: oldLines.length + 1}, () =>
    Array<number>(newLines.length + 1).fill(0),
  );

  for (let oldIndex = oldLines.length - 1; oldIndex >= 0; oldIndex -= 1) {
    for (let newIndex = newLines.length - 1; newIndex >= 0; newIndex -= 1) {
      if (oldComparable[oldIndex] === newComparable[newIndex]) {
        dp[oldIndex][newIndex] = dp[oldIndex + 1][newIndex + 1] + 1;
      } else {
        dp[oldIndex][newIndex] = Math.max(
          dp[oldIndex + 1][newIndex],
          dp[oldIndex][newIndex + 1],
        );
      }
    }
  }

  const operations: DiffOp[] = [];
  let oldIndex = 0;
  let newIndex = 0;

  while (oldIndex < oldLines.length && newIndex < newLines.length) {
    if (oldComparable[oldIndex] === newComparable[newIndex]) {
      operations.push({type: 'equal', oldIndex, newIndex});
      oldIndex += 1;
      newIndex += 1;
      continue;
    }

    if (dp[oldIndex + 1][newIndex] >= dp[oldIndex][newIndex + 1]) {
      operations.push({type: 'delete', oldIndex});
      oldIndex += 1;
      continue;
    }

    operations.push({type: 'insert', newIndex});
    newIndex += 1;
  }

  while (oldIndex < oldLines.length) {
    operations.push({type: 'delete', oldIndex});
    oldIndex += 1;
  }
  while (newIndex < newLines.length) {
    operations.push({type: 'insert', newIndex});
    newIndex += 1;
  }

  const rows: AlignedLineRow[] = [];
  const pendingDeletes: number[] = [];
  const pendingInserts: number[] = [];

  const flushPending = () => {
    const chunkLength = Math.max(pendingDeletes.length, pendingInserts.length);
    for (let chunkIndex = 0; chunkIndex < chunkLength; chunkIndex += 1) {
      const oldLineIndex = pendingDeletes[chunkIndex];
      const newLineIndex = pendingInserts[chunkIndex];
      rows.push({
        oldLineNumber: oldLineIndex === undefined ? null : oldLineIndex + 1,
        newLineNumber: newLineIndex === undefined ? null : newLineIndex + 1,
        oldValue:
          oldLineIndex === undefined ? '' : oldLines[oldLineIndex] || '',
        newValue:
          newLineIndex === undefined ? '' : newLines[newLineIndex] || '',
        changed: true,
      });
    }
    pendingDeletes.length = 0;
    pendingInserts.length = 0;
  };

  operations.forEach((operation) => {
    if (operation.type === 'equal') {
      flushPending();
      rows.push({
        oldLineNumber: operation.oldIndex + 1,
        newLineNumber: operation.newIndex + 1,
        oldValue: oldLines[operation.oldIndex] || '',
        newValue: newLines[operation.newIndex] || '',
        changed: false,
      });
      return;
    }

    if (operation.type === 'delete') {
      pendingDeletes.push(operation.oldIndex);
      return;
    }

    pendingInserts.push(operation.newIndex);
  });

  flushPending();
  return rows;
}

function getInitialResultLine(
  row: AlignedLineRow,
  previous?: ConfigConflict,
): {value: string; present: boolean; resolved: boolean} {
  if (previous) {
    if (previous.resolved_value === VISUAL_DELETE_SENTINEL) {
      return {
        value: '',
        present: false,
        resolved: previous.status === 'resolved',
      };
    }

    return {
      value: previous.resolved_value || '',
      present:
        previous.status === 'resolved' ||
        (previous.resolved_value || '') !== '',
      resolved: previous.status === 'resolved',
    };
  }

  if (!row.changed) {
    return {
      value: row.oldValue || row.newValue || '',
      present: true,
      resolved: true,
    };
  }

  return {
    value: '',
    present: false,
    resolved: false,
  };
}

export function buildMergeEditorRows(file: ConfigMergeFile): MergeEditorRow[] {
  const alignedRows = buildAlignedLineRows(
    file.local_content,
    file.target_content,
  );
  const visualConflictMap = new Map(
    (Array.isArray(file.conflicts) ? file.conflicts : [])
      .filter(isVisualConflict)
      .map((conflict) => [conflict.id, normalizeConflictStatus(conflict)]),
  );

  return alignedRows.map((row, rowIndex) => {
    const id = buildVisualConflictId(file.config_type, rowIndex);
    const previous = visualConflictMap.get(id);
    const initial = getInitialResultLine(row, previous);

    return {
      id,
      path: buildVisualConflictPath(row.oldLineNumber, row.newLineNumber),
      oldLineNumber: row.oldLineNumber,
      newLineNumber: row.newLineNumber,
      resultLineNumber: rowIndex + 1,
      oldValue: row.oldValue,
      newValue: row.newValue,
      resultValue: initial.value,
      resultPresent: initial.present,
      changed: row.changed,
      resolved: row.changed ? initial.resolved : true,
    };
  });
}

function getUnresolvedFileConflictCount(file: ConfigMergeFile): number {
  return file.conflicts.filter(
    (conflict) =>
      !areLinesEquivalent(conflict.local_value, conflict.target_value) &&
      conflict.status !== 'resolved',
  ).length;
}

export function rebuildMergeFileFromRows(
  file: ConfigMergeFile,
  rows: MergeEditorRow[],
): ConfigMergeFile {
  const conflicts = rows.map<ConfigConflict>((row) => ({
    id: row.id,
    config_type: file.config_type,
    path: row.path,
    base_value: '',
    local_value: row.oldValue,
    target_value: row.newValue,
    resolved_value: row.resultPresent
      ? row.resultValue
      : VISUAL_DELETE_SENTINEL,
    status: row.changed && !row.resolved ? 'pending' : 'resolved',
  }));

  const mergedContent = rows
    .filter((row) => row.resultPresent)
    .map((row) => row.resultValue)
    .join('\n');
  const nextFile = {
    ...file,
    merged_content: mergedContent,
    conflicts,
  };
  const conflictCount = getUnresolvedFileConflictCount(nextFile);

  return {
    ...nextFile,
    conflict_count: conflictCount,
    resolved: conflictCount === 0,
  };
}

function normalizeMergeFile(file: ConfigMergeFile): ConfigMergeFile {
  const normalizedConflicts = Array.isArray(file.conflicts)
    ? file.conflicts.map(normalizeConflictStatus)
    : [];
  const baseFile = {
    ...file,
    conflicts: normalizedConflicts,
    merged_content: containsLegacyConflictMarkers(file.merged_content)
      ? ''
      : file.merged_content,
  };

  return rebuildMergeFileFromRows(baseFile, buildMergeEditorRows(baseFile));
}

export function normalizeMergePlan(plan: ConfigMergePlan): ConfigMergePlan {
  const sanitizedPlan = sanitizeConfigMergePlan(plan) || plan;
  const files = sanitizedPlan.files.map(normalizeMergeFile);
  const conflictCount = files.reduce(
    (total, file) => total + file.conflict_count,
    0,
  );

  return {
    ...sanitizedPlan,
    ready: conflictCount === 0,
    has_conflicts: conflictCount > 0,
    conflict_count: conflictCount,
    files,
  };
}

export function buildPlanStepMap(
  steps: PlanStep[],
): Record<StepCode, PlanStep> {
  return steps.reduce<Record<StepCode, PlanStep>>(
    (acc, step) => {
      acc[step.code] = step;
      return acc;
    },
    {} as Record<StepCode, PlanStep>,
  );
}

export function calculateExecutionProgress(steps: UpgradeTaskStep[]): number {
  const executionSteps = steps.filter(
    (step) => !step.code.startsWith('ROLLBACK_') && step.code !== 'FAILED',
  );
  if (executionSteps.length === 0) {
    return 0;
  }
  const completed = executionSteps.filter(
    (step) => step.status === 'succeeded',
  ).length;
  return Math.round((completed / executionSteps.length) * 100);
}

export function getStepNodeCount(
  step: UpgradeTaskStep,
  stepsData: TaskStepsData | null,
): number {
  if (!stepsData) {
    return 0;
  }
  return stepsData.node_executions.filter(
    (node) => node.task_step_id === step.id,
  ).length;
}
