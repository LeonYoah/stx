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

import type {useTranslations} from 'next-intl';
import {cn} from '@/lib/utils';
import type {
  RuntimeStorageCheckpointInspectJobConfig,
  RuntimeStorageCheckpointInspectResult,
} from '@/lib/services/cluster/types';
import type {
  SyncFormat,
  SyncJobInstance,
  SyncJSON,
  SyncPluginFactoryInfo,
  SyncPluginType,
  SyncPreviewDataset,
  SyncTask,
  SyncTaskTreeNode,
} from '@/lib/services/sync';
export type { SyncTaskTreeNode, SyncTask };
import type {CustomVariableItem} from './CustomVariableDialog';
import type {TaskScheduleValue} from './TaskScheduleSidebarPanel';

// ==========================================
// 状态与类型定义
// State and Type Definitions
// ==========================================

export interface EditorState {
  id?: number;
  parentId?: number;
  name: string;
  description: string;
  clusterId: string;
  contentFormat: SyncFormat;
  content: string;
  definition: SyncJSON;
  currentVersion: number;
  status: string;
  createdBy?: number;
  canEdit?: boolean;
  canRun?: boolean;
  isOwner?: boolean;
  isCollaborator?: boolean;
  isPublic?: boolean;
}

export interface TreeContextMenuState {
  open: boolean;
  x: number;
  y: number;
  kind: 'root' | 'folder' | 'file';
  node: SyncTaskTreeNode | null;
}

export interface TreeDialogState {
  open: boolean;
  action: 'create-folder' | 'create-file' | 'rename' | 'move' | 'delete' | null;
  targetNode: SyncTaskTreeNode | null;
  name: string;
  targetParentId: number | null;
}

export interface OpenFileTab {
  id: number;
  name: string;
}

export interface EditorDraftState {
  editor: EditorState;
  customVariableRows: VariableRow[];
  dirty: boolean;
  baselineEditor: EditorState;
  baselineCustomVariableRows: VariableRow[];
}

export interface PersistedWorkspaceTabs {
  openTabIds: number[];
  activeTabId: number | null;
}

export type VariableRow = CustomVariableItem;

export interface VariableDraft {
  key: string;
  value: string;
}

export interface PreviewRunDialogState {
  open: boolean;
  rowLimit: string;
  timeoutMinutes: string;
}

export type ErrorCategory =
  | 'schema'
  | 'network'
  | 'auth'
  | 'syntax'
  | 'runtime'
  | 'general';

export interface UserFacingErrorState {
  title: string;
  description: string;
  category?: ErrorCategory;
  suggestion?: string;
  raw?: string;
}

// 右侧属性边栏选项卡类型：设置 / 定时调度 / 版本历史 / 全局变量
// Right sidebar tab types: settings / schedule / versions / globals
export type RightSidebarTab = 'settings' | 'schedule' | 'versions' | 'globals';
export type BottomConsoleTab = 'jobs' | 'logs' | 'preview' | 'checkpoint';
export type ExecutionMode = 'cluster' | 'local';
export type LogFilterMode = 'all' | 'warn' | 'error';
export type PendingActionKind = 'dag' | 'preview' | 'test_connections' | 'recover';

export interface TemplatePluginItem {
  value: string;
  label: string;
  origin?: string;
}

export type OptionMetadataMap = Record<string, any>;

export type PluginEnumCatalogMap = Partial<
  Record<SyncPluginType | 'env', Record<string, OptionMetadataMap>>
>;

export type MetricGroupKey =
  | 'read'
  | 'write'
  | 'throughput'
  | 'latency'
  | 'status'
  | 'other';

// ==========================================
// 常量定义
// Constants
// ==========================================

export const LOG_CHUNK_BASE_BYTES = 64 * 1024;
export const LOG_CHUNK_MAX_BYTES = 1024 * 1024;
export const EXPANDED_LOG_CHUNK_BASE_BYTES = 256 * 1024;
export const EXPANDED_LOG_CHUNK_MAX_BYTES = 2 * 1024 * 1024;
export const WORKSPACE_TABS_STORAGE_KEY = 'data-sync-studio:workspace-tabs';

export const EMPTY_EDITOR: EditorState = {
  name: '',
  description: '',
  clusterId: '',
  contentFormat: 'hocon',
  content: '',
  definition: {},
  currentVersion: 0,
  status: 'draft',
  canEdit: true,
  canRun: true,
  isOwner: true,
  isCollaborator: false,
  isPublic: true,
};

export const WORKSPACE_NAME_PATTERN = /^[\p{L}\p{N}._-]+$/u;

export const ENV_OPTION_METADATA: Record<
  string,
  {
    description: string;
    enumValues?: string[];
    defaultValue?: string | number;
    requiredMode?: string;
  }
> = {
  'job.mode': {
    description: 'SeaTunnel 作业模式',
    enumValues: ['BATCH', 'STREAMING'],
    defaultValue: 'BATCH',
    requiredMode: 'OPTIONAL',
  },
  'savemode.execute.location': {
    description: 'SaveMode 执行位置',
    enumValues: ['CLUSTER', 'ENGINE'],
    defaultValue: 'CLUSTER',
    requiredMode: 'OPTIONAL',
  },
  parallelism: {
    description: '作业并行度',
    defaultValue: 1,
    requiredMode: 'OPTIONAL',
  },
  'job.retry.times': {
    description: '失败重试次数',
    defaultValue: 0,
    requiredMode: 'OPTIONAL',
  },
  'job.retry.interval.seconds': {
    description: '重试间隔秒数',
    defaultValue: 3,
    requiredMode: 'OPTIONAL',
  },
  'min-pause': {
    description: 'Checkpoint 最小间隔',
    defaultValue: -1,
    requiredMode: 'OPTIONAL',
  },
  'checkpoint.interval': {
    description: 'Checkpoint 间隔毫秒',
    defaultValue: 10000,
    requiredMode: 'OPTIONAL',
  },
  'checkpoint.timeout': {
    description: 'Checkpoint 超时毫秒',
    defaultValue: 30000,
    requiredMode: 'OPTIONAL',
  },
};

// ==========================================
// Monaco / HOCON 语言辅助函数
// Monaco / HOCON Language Helper Functions
// ==========================================

export function isCursorInsideValueRegion(
  lineContent: string,
  column: number,
): boolean {
  const equalsIndex = lineContent.indexOf('=');
  if (equalsIndex < 0) {
    return false;
  }
  const prefix = lineContent.slice(0, equalsIndex);
  if (!/^\s*(?:#+\s*)?[A-Za-z0-9_.-]+\s*$/.test(prefix)) {
    return false;
  }
  const commentIndex = lineContent.indexOf('#', equalsIndex);
  if (commentIndex >= 0 && column > commentIndex + 1) {
    return false;
  }
  return column >= equalsIndex + 2;
}

export function formatMetadataValue(value: unknown): string {
  if (value === undefined) {
    return '';
  }
  if (typeof value === 'string') {
    return JSON.stringify(value);
  }
  if (
    value === null ||
    typeof value === 'number' ||
    typeof value === 'boolean'
  ) {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

export function resolveEnumSuggestionItems(metadata: any): Array<{
  label: string;
  value: string;
}> {
  let values = Array.isArray(metadata?.enum_values)
    ? metadata.enum_values
    : [];
  let displays = Array.isArray(metadata?.enum_display_values)
    ? metadata.enum_display_values
    : [];
  if (
    (!values || values.length === 0) &&
    (metadata?.type === 'boolean' ||
      metadata?.type === 'Boolean' ||
      typeof metadata?.defaultValue === 'boolean' ||
      typeof metadata?.default_value === 'boolean')
  ) {
    values = ['true', 'false'];
    displays = ['true', 'false'];
  }
  return values.map((value: string, index: number) => ({
    label: displays[index] || value,
    value,
  }));
}

export interface OptionAssignmentContext {
  isAssignment: boolean;
  optionKey: string | null;
  inValueRegion: boolean;
  bounds: {
    quoted: boolean;
    startColumn: number;
    endColumn: number;
    value: string;
  } | null;
}

export function resolveOptionAssignmentContext(
  lineContent: string,
  column: number,
): OptionAssignmentContext {
  const equalsIndex = lineContent.indexOf('=');
  if (equalsIndex < 0) {
    return {
      isAssignment: false,
      optionKey: null,
      inValueRegion: false,
      bounds: null,
    };
  }

  const leftSide = lineContent.slice(0, equalsIndex);
  const keyMatch = leftSide.match(/^\s*(?:#+\s*)?([A-Za-z0-9_.-]+)\s*$/);
  if (!keyMatch) {
    return {
      isAssignment: false,
      optionKey: null,
      inValueRegion: false,
      bounds: null,
    };
  }
  const optionKey = keyMatch[1];

  const commentIndex = lineContent.indexOf('#', equalsIndex);
  if (commentIndex >= 0 && column > commentIndex + 1) {
    return {
      isAssignment: true,
      optionKey,
      inValueRegion: false,
      bounds: null,
    };
  }

  const inValueRegion = column >= equalsIndex + 2;
  const bounds = resolveEnumValueBounds(lineContent, 0);

  let boundsWithValue: OptionAssignmentContext['bounds'] = null;
  if (bounds) {
    const rawSnippet = lineContent.slice(
      Math.max(0, bounds.startColumn - 1),
      Math.max(0, bounds.endColumn - 1),
    );
    boundsWithValue = {
      ...bounds,
      value: bounds.quoted ? rawSnippet : rawSnippet.trim(),
    };
  }

  return {
    isAssignment: true,
    optionKey,
    inValueRegion,
    bounds: boundsWithValue,
  };
}

export function resolveEnumValueBounds(lineContent: string, lineNumber: number) {
  const assignmentMatch = lineContent.match(
    /^(\s*(?:#+\s*)?[A-Za-z0-9_.-]+\s*=\s*)(.*)$/,
  );
  if (!assignmentMatch) {
    return null;
  }
  const valueOffset = assignmentMatch[1].length + 1;
  const rawValue = assignmentMatch[2] || '';
  const quotedMatch = rawValue.match(/^"([^"]*)"?/);
  if (quotedMatch) {
    const content = quotedMatch[1] || '';
    return {
      quoted: true,
      startColumn: valueOffset + 1,
      endColumn: valueOffset + 1 + content.length,
    };
  }
  const unquotedEnd = rawValue.search(/\s|#/);
  const contentLength = unquotedEnd >= 0 ? unquotedEnd : rawValue.length;
  return {
    quoted: false,
    startColumn: valueOffset,
    endColumn: valueOffset + contentLength,
  };
}

export function resolveEnumSuggestRange(position: {
  lineNumber: number;
  column: number;
}) {
  return {
    startLineNumber: position.lineNumber,
    endLineNumber: position.lineNumber,
    startColumn: position.column,
    endColumn: position.column,
  };
}

export function ensureSyncHoconLanguage(monaco: any) {
  const languageId = 'sync-hocon';
  const languages = monaco.languages.getLanguages?.() || [];
  if (!languages.some((item: any) => item.id === languageId)) {
    monaco.languages.register({id: languageId});
    monaco.languages.setMonarchTokensProvider(languageId, {
      tokenizer: {
        root: [
          [/^\s*#.*$/, 'comment'],
          [/^\s*(env)(?=\s*\{)/, 'keyword.env'],
          [/^\s*(source)(?=\s*\{)/, 'keyword.source'],
          [/^\s*(transform)(?=\s*\{)/, 'keyword.transform'],
          [/^\s*(sink)(?=\s*\{)/, 'keyword.sink'],
          [/[{}[\]]/, '@brackets'],
          [/[,:=]/, 'delimiter'],
          [/"(?:[^"\\]|\\.)*"/, 'string'],
          [/[A-Za-z_][\w.-]*/, 'identifier'],
          [/-?\d+(?:\.\d+)?/, 'number'],
        ],
      },
    });
    monaco.languages.setLanguageConfiguration(languageId, {
      comments: {lineComment: '#'},
      autoClosingPairs: [
        {open: '{', close: '}'},
        {open: '[', close: ']'},
        {open: '"', close: '"'},
      ],
      surroundingPairs: [
        {open: '{', close: '}'},
        {open: '[', close: ']'},
        {open: '"', close: '"'},
      ],
      brackets: [
        ['{', '}'],
        ['[', ']'],
      ],
    });
  }
  monaco.editor.defineTheme('sync-hocon-light', {
    base: 'vs',
    inherit: true,
    rules: [
      {token: 'keyword.env', foreground: '7c3aed', fontStyle: 'bold'},
      {token: 'keyword.source', foreground: '0f766e', fontStyle: 'bold'},
      {token: 'keyword.transform', foreground: 'b45309', fontStyle: 'bold'},
      {token: 'keyword.sink', foreground: '1d4ed8', fontStyle: 'bold'},
      {token: 'string', foreground: 'b91c1c'},
      {token: 'comment', foreground: '6b7280'},
    ],
    colors: {},
  });
  monaco.editor.defineTheme('sync-hocon-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      {token: 'keyword.env', foreground: 'c084fc', fontStyle: 'bold'},
      {token: 'keyword.source', foreground: '2dd4bf', fontStyle: 'bold'},
      {token: 'keyword.transform', foreground: 'fbbf24', fontStyle: 'bold'},
      {token: 'keyword.sink', foreground: '60a5fa', fontStyle: 'bold'},
      {token: 'string', foreground: 'fca5a5'},
      {token: 'comment', foreground: '9ca3af'},
    ],
    colors: {},
  });

  // 注册高对比度暗色与亮色差异对比主题，彻底解决暗黑模式下差异行对比度过低、看不清的问题
  // Register high-contrast dark and light diff themes to resolve dim diff lines in dark mode
  monaco.editor.defineTheme('sync-diff-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      {token: 'keyword.env', foreground: 'c084fc', fontStyle: 'bold'},
      {token: 'keyword.source', foreground: '2dd4bf', fontStyle: 'bold'},
      {token: 'keyword.transform', foreground: 'fbbf24', fontStyle: 'bold'},
      {token: 'keyword.sink', foreground: '60a5fa', fontStyle: 'bold'},
      {token: 'string', foreground: 'fca5a5'},
      {token: 'comment', foreground: '9ca3af'},
    ],
    colors: {
      'diffEditor.insertedLineBackground': '#10b98124',
      'diffEditor.insertedTextBackground': '#10b98144',
      'diffEditor.removedLineBackground': '#ef444424',
      'diffEditor.removedTextBackground': '#ef444444',
      'diffEditor.diagonalFill': '#27272a80',
      'diffEditorGutter.insertedLineBackground': '#10b9813b',
      'diffEditorGutter.removedLineBackground': '#ef44443b',
      'diffEditorOverview.insertedForeground': '#10b981cc',
      'diffEditorOverview.removedForeground': '#ef4444cc',
    },
  });

  monaco.editor.defineTheme('sync-diff-light', {
    base: 'vs',
    inherit: true,
    rules: [
      {token: 'keyword.env', foreground: '7c3aed', fontStyle: 'bold'},
      {token: 'keyword.source', foreground: '0f766e', fontStyle: 'bold'},
      {token: 'keyword.transform', foreground: 'b45309', fontStyle: 'bold'},
      {token: 'keyword.sink', foreground: '1d4ed8', fontStyle: 'bold'},
      {token: 'string', foreground: 'b91c1c'},
      {token: 'comment', foreground: '6b7280'},
    ],
    colors: {
      'diffEditor.insertedLineBackground': '#10b9811f',
      'diffEditor.insertedTextBackground': '#10b9813b',
      'diffEditor.removedLineBackground': '#ef44441f',
      'diffEditor.removedTextBackground': '#ef44443b',
      'diffEditor.diagonalFill': '#e4e4e780',
      'diffEditorGutter.insertedLineBackground': '#10b98133',
      'diffEditorGutter.removedLineBackground': '#ef444433',
      'diffEditorOverview.insertedForeground': '#10b981',
      'diffEditorOverview.removedForeground': '#ef4444',
    },
  });
}

// ==========================================
// 通用与对比工具函数
// General and Comparison Utilities
// ==========================================

export function getSyncJobClusterId(job: SyncJobInstance | null): number | null {
  const raw = job?.submit_spec?.cluster_id;
  if (typeof raw === 'number' && Number.isFinite(raw) && raw > 0) {
    return raw;
  }
  if (typeof raw === 'string' && raw.trim() !== '') {
    const parsed = Number(raw);
    if (Number.isFinite(parsed) && parsed > 0) {
      return parsed;
    }
  }
  return null;
}

export function formatSizeBytes(bytes?: number | null): string {
  if (!bytes || bytes <= 0) {
    return '-';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`;
}

export function normalizeVariableRowsForCompare(rows: VariableRow[]): Record<string, unknown>[] {
  return rows.map((row) => ({
    key: row.key.trim(),
    value: row.value,
    type: row.type || 'string',
    description: row.description || '',
  }));
}

export function normalizeEditorForCompare(
  editor: EditorState,
): Record<string, unknown> {
  return {
    parentId: editor.parentId ?? null,
    name: editor.name.trim(),
    description: editor.description.trim(),
    clusterId: editor.clusterId,
    contentFormat: editor.contentFormat,
    content: editor.content,
    definition: editor.definition ?? {},
  };
}

export function isEditorDraftDirty(
  editor: EditorState,
  rows: VariableRow[],
  baselineEditor: EditorState,
  baselineRows: VariableRow[],
): boolean {
  return (
    JSON.stringify(normalizeEditorForCompare(editor)) !==
      JSON.stringify(normalizeEditorForCompare(baselineEditor)) ||
    JSON.stringify(normalizeVariableRowsForCompare(rows)) !==
      JSON.stringify(normalizeVariableRowsForCompare(baselineRows))
  );
}

// ==========================================
// Checkpoint 徽章与字段格式化
// Checkpoint Badges and Field Formatters
// ==========================================

export function getCheckpointStatusBadgeClass(status?: string): string {
  switch ((status || '').toUpperCase()) {
    case 'COMPLETED':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400';
    case 'FAILED':
      return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400';
    case 'CANCELED':
      return 'border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400';
    default:
      return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400';
  }
}

export function getCheckpointEnumBadgeClass(
  value?: string | boolean | null,
  kind: 'status' | 'checkpointType' | 'boolean' = 'status',
): string {
  if (kind === 'boolean') {
    return value
      ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
      : 'border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400';
  }
  const normalized = String(value || '')
    .trim()
    .toUpperCase();
  if (kind === 'checkpointType') {
    switch (normalized) {
      case 'CHECKPOINT_TYPE':
        return 'border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400';
      case 'SAVEPOINT_TYPE':
        return 'border-violet-500/30 bg-violet-500/10 text-violet-600 dark:text-violet-400';
      case 'COMPLETED_POINT_TYPE':
        return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400';
      default:
        return 'border-border/60 bg-muted/50 text-muted-foreground';
    }
  }
  switch (normalized) {
    case 'COMPLETED':
    case 'FINISHED':
    case 'SAVEPOINT_DONE':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400';
    case 'RUNNING':
    case 'DOING_SAVEPOINT':
      return 'border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400';
    case 'FAILED':
      return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400';
    case 'CANCELED':
      return 'border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400';
    case 'CREATED':
    case 'SCHEDULED':
    case 'DEPLOYING':
    case 'INITIALIZING':
    case 'PENDING':
      return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400';
    default:
      return 'border-border/60 bg-muted/50 text-muted-foreground';
  }
}

export function formatCheckpointFieldValue(
  key: string,
  value: unknown,
): string | null {
  if (value === null || value === undefined || value === '') {
    return '-';
  }
  if (typeof value === 'number') {
    if (/timestamp/i.test(key)) {
      return value > 0 ? new Date(value).toLocaleString() : '-';
    }
    if (/state(size|bytes)/i.test(key)) {
      return formatSizeBytes(value);
    }
    return String(value);
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}


export function buildCheckpointInspectSummary(
  result: RuntimeStorageCheckpointInspectResult | null,
): Array<{label: string; key: string; value: unknown}> {
  const completed = result?.completed_checkpoint || {};
  const pipeline = result?.pipeline_state || {};
  return [
    {
      label: 'Checkpoint ID',
      key: 'checkpointId',
      value: completed.checkpointId,
    },
    {
      label: 'Checkpoint Type',
      key: 'checkpointType',
      value: completed.checkpointType,
    },
    {
      label: 'Pipeline',
      key: 'pipelineId',
      value: completed.pipelineId ?? pipeline.pipelineId,
    },
    {label: 'Job ID', key: 'jobId', value: completed.jobId ?? pipeline.jobId},
    {
      label: 'Triggered',
      key: 'triggerTimestamp',
      value: completed.triggerTimestamp,
    },
    {
      label: 'Completed',
      key: 'completedTimestamp',
      value: completed.completedTimestamp,
    },
    {label: 'State Size', key: 'stateBytes', value: pipeline.stateBytes},
    {
      label: 'Task States',
      key: 'taskStateCount',
      value: completed.taskStateCount,
    },
  ];
}

export function extractCheckpointFileIdentity(
  name?: string,
): {pipelineId: number; checkpointId: number} | null {
  if (!name) {
    return null;
  }
  const fileName = name.split('/').pop() || name;
  const baseName = fileName.replace(/\.[^.]+$/, '');
  const segments = baseName.split('-');
  if (segments.length < 4) {
    return null;
  }
  const pipelineId = Number(segments[segments.length - 2]);
  const checkpointId = Number(segments[segments.length - 1]);
  if (!Number.isInteger(pipelineId) || !Number.isInteger(checkpointId)) {
    return null;
  }
  return {pipelineId, checkpointId};
}

export function nextLogChunkSize(
  current: number,
  logs: string,
  min: number,
  max: number,
): number {
  const actualBytes = new TextEncoder().encode(logs || '').length;
  if (actualBytes >= Math.floor(current * 0.8) && current < max) {
    return Math.min(max, current * 2);
  }
  if (
    actualBytes > 0 &&
    actualBytes <= Math.floor(current * 0.25) &&
    current > min
  ) {
    return Math.max(min, Math.floor(current / 2));
  }
  return current;
}

export function buildCopiedWorkspaceName(
  tree: SyncTaskTreeNode[],
  parentId: number | null,
  originalName: string,
) {
  const dotIndex = originalName.lastIndexOf('.');
  const hasExtension = dotIndex > 0 && dotIndex < originalName.length - 1;
  const base = hasExtension ? originalName.slice(0, dotIndex) : originalName;
  const ext = hasExtension ? originalName.slice(dotIndex) : '';
  let candidate = `${base}_copy${ext}`;
  let counter = 2;
  while (hasDuplicateWorkspaceName(tree, parentId, candidate)) {
    candidate = `${base}_copy_${counter}${ext}`;
    counter += 1;
  }
  return candidate;
}

export function getPendingActionLabel(
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
  actionPending: PendingActionKind | null,
) {
  switch (actionPending) {
    case 'dag':
      return t('buildingDag');
    case 'preview':
      return t('preparingPreview');
    case 'recover':
      return t('recoveringJob');
    case 'test_connections':
      return t('testingConnections');
    default:
      return '';
  }
}

// ==========================================
// 树操作与过滤工具函数
// Tree Manipulation and Filtering Utilities
// ==========================================

export function toObject(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return {};
}

export function flattenTree(nodes: SyncTaskTreeNode[]): SyncTaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTree(node.children || [])]);
}

export function collectFolderIds(nodes: SyncTaskTreeNode[]): number[] {
  return flattenTree(nodes)
    .filter((node) => node.node_type === 'folder')
    .map((node) => node.id);
}

export function findTreeNode(
  nodes: SyncTaskTreeNode[],
  nodeId: number,
): SyncTaskTreeNode | null {
  for (const node of nodes) {
    if (node.id === nodeId) {
      return node;
    }
    const child = findTreeNode(node.children || [], nodeId);
    if (child) {
      return child;
    }
  }
  return null;
}

export function isTreeDescendant(
  nodes: SyncTaskTreeNode[],
  ancestorId: number,
  candidateId: number,
): boolean {
  const ancestor = findTreeNode(nodes, ancestorId);
  if (!ancestor) {
    return false;
  }
  return flattenTree(ancestor.children || []).some(
    (node) => node.id === candidateId,
  );
}

export function listMoveTargets(
  nodes: SyncTaskTreeNode[],
  source: SyncTaskTreeNode | null,
  rootLabel: string,
): Array<{label: string; value: number | null; depth: number}> {
  const buildPathLabel = (target: SyncTaskTreeNode): string => {
    const segments: string[] = [target.name];
    let cursor = target.parent_id
      ? findTreeNode(nodes, target.parent_id)
      : null;
    while (cursor) {
      segments.unshift(cursor.name);
      cursor = cursor.parent_id ? findTreeNode(nodes, cursor.parent_id) : null;
    }
    return `/${segments.join('/')}`;
  };
  const folders = flattenTree(nodes).filter(
    (node) => node.node_type === 'folder',
  );
  const options: Array<{label: string; value: number | null; depth: number}> =
    source?.node_type === 'file'
      ? []
      : [{label: rootLabel, value: null, depth: 0}];
  for (const folder of folders) {
    if (source) {
      if (folder.id === source.id) {
        continue;
      }
      if (
        source.node_type === 'folder' &&
        isTreeDescendant(nodes, source.id, folder.id)
      ) {
        continue;
      }
    }
    options.push({
      label: buildPathLabel(folder),
      value: folder.id,
      depth: buildPathLabel(folder).split('/').filter(Boolean).length,
    });
  }
  return options;
}

/**
 * 获取树节点的面包屑路径分段列表
 * Retrieve breadcrumb path segments for a tree node
 */
export function getNodeBreadcrumbSegments(
  nodes: SyncTaskTreeNode[],
  targetId: number | null,
): string[] {
  if (!targetId) {
    return [];
  }
  const target = findTreeNode(nodes, targetId);
  if (!target) {
    return [];
  }
  const segments: string[] = [target.name];
  let cursor = target.parent_id ? findTreeNode(nodes, target.parent_id) : null;
  while (cursor) {
    segments.unshift(cursor.name);
    cursor = cursor.parent_id ? findTreeNode(nodes, cursor.parent_id) : null;
  }
  return segments;
}

export function patchTreeNode(
  nodes: SyncTaskTreeNode[],
  task: SyncTask,
): SyncTaskTreeNode[] {
  return nodes.map((node) => {
    if (node.id === task.id) {
      return {
        ...node,
        parent_id: task.parent_id,
        node_type: task.node_type,
        name: task.name,
        description: task.description,
        cluster_id: task.cluster_id,
        content_format: task.content_format,
        content: task.content,
        definition: task.definition,
        current_version: task.current_version,
        status: task.status,
        job_name: task.job_name,
      };
    }
    if (node.children && node.children.length > 0) {
      return {...node, children: patchTreeNode(node.children, task)};
    }
    return node;
  });
}

export function filterTree(
  nodes: SyncTaskTreeNode[],
  keyword: string,
): SyncTaskTreeNode[] {
  const trimmed = keyword.trim().toLowerCase();
  if (!trimmed) {
    return nodes;
  }
  return nodes
    .map((node) => {
      const children = filterTree(node.children || [], keyword);
      const matched = node.name.toLowerCase().includes(trimmed);
      if (matched || children.length > 0) {
        return {...node, children};
      }
      return null;
    })
    .filter(Boolean) as SyncTaskTreeNode[];
}

// ==========================================
// 变量与调度校验工具函数
// Variable and Schedule Validation Utilities
// ==========================================

export function detectVariables(content: string): string[] {
  const matches = [...content.matchAll(/\{\{\s*([^{}]+?)\s*\}\}/g)];
  return Array.from(
    new Set(
      matches.map((match) => match[1]?.trim()).filter(Boolean) as string[],
    ),
  ).sort();
}

export function isReservedBuiltinVariableKey(key: string): boolean {
  const trimmed = key.trim();
  if (!trimmed) {
    return false;
  }
  const fixed = new Set([
    'system.biz.date',
    'system.biz.curdate',
    'system.datetime',
    'system.task.execute.path',
    'system.task.instance.id',
    'system.task.definition.name',
    'system.task.definition.code',
    'system.workflow.instance.id',
    'system.workflow.definition.name',
    'system.workflow.definition.code',
    'system.project.name',
    'system.project.code',
  ]);
  if (fixed.has(trimmed)) {
    return true;
  }
  return /(yyyy|MM|dd|HH|mm|ss|add_months|this_day|last_day|year_week|month_first_day|month_last_day|week_first_day|week_last_day)/.test(
    trimmed,
  );
}

// 校验自定义变量列表的合法性（保留字与重名检查）
// Validate custom variable rows (check reserved keywords and duplicates)
export function validateCustomVariableRows(
  rows: VariableRow[],
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): string | null {
  const seenKeys = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) {
      continue;
    }
    if (isReservedBuiltinVariableKey(key)) {
      return t('reservedBuiltinVariableKey', {key: `{{${key}}}`});
    }
    const lower = key.toLowerCase();
    if (seenKeys.has(lower)) {
      return t('duplicateCustomVariableKey');
    }
    seenKeys.add(lower);
  }
  return null;
}

export function extractTaskScheduleValue(definition: SyncJSON): TaskScheduleValue {
  const schedule = toObject(toObject(definition).schedule);
  return {
    enabled: Boolean(schedule.enabled),
    cron_expr:
      typeof schedule.cron_expr === 'string'
        ? String(schedule.cron_expr)
        : '0 0 * * *',
    timezone:
      typeof schedule.timezone === 'string' &&
      schedule.timezone.trim().length > 0
        ? String(schedule.timezone)
        : 'Asia/Shanghai',
  };
}

export function validateWorkspaceName(
  name: string,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): string | null {
  const trimmed = name.trim();
  if (!trimmed) {
    return t('nameRequired');
  }
  if (!WORKSPACE_NAME_PATTERN.test(trimmed)) {
    return t('workspaceNameInvalid');
  }
  return null;
}

export function listSiblingNames(
  tree: SyncTaskTreeNode[],
  parentId: number | null,
  excludeId?: number | null,
): string[] {
  const nodes =
    parentId == null ? tree : findTreeNode(tree, parentId)?.children || [];
  return nodes
    .filter((node) => node.id !== excludeId)
    .map((node) => node.name.trim().toLowerCase());
}

export function hasDuplicateWorkspaceName(
  tree: SyncTaskTreeNode[],
  parentId: number | null,
  name: string,
  excludeId?: number | null,
): boolean {
  const normalized = name.trim().toLowerCase();
  if (!normalized) {
    return false;
  }
  return listSiblingNames(tree, parentId, excludeId).includes(normalized);
}

export function formatSyncUserFacingError(
  error: unknown,
  fallbackTitle: string,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): UserFacingErrorState {
  const rawMessage = error instanceof Error ? error.message : String(error || t('unknownError'));
  // 清洗引擎或包装层的套娃前缀
  // Clean nested wrapper prefixes from engine or RPC layers
  const cleanMessage = rawMessage
    .replace(/^sync:\s*/, '')
    .replace(/^java\.lang\.RuntimeException:\s*/, '')
    .replace(/^org\.apache\.seatunnel\.common\.exception\.\w+:\s*/, '')
    .trim();

  // 1. 未发布保存检查
  if (rawMessage.includes('sync: task has not been published')) {
    return {
      title: t('saveRequiredTitle'),
      description: t('saveRequiredDescription'),
      category: 'general',
      suggestion: '任务尚未保存或发布，请先点击保存（Ctrl/Cmd + S）后再进行操作。',
    };
  }

  // 2. 库表未找到或 Schema 映射失败
  if (
    /table.*(?:doesn't exist|not found|resolution failed)/i.test(cleanMessage) ||
    /Unknown table/i.test(cleanMessage) ||
    /schema.*not found/i.test(cleanMessage)
  ) {
    return {
      title: fallbackTitle,
      description: cleanMessage,
      category: 'schema',
      suggestion: '请核对任务配置中的 Source/Sink 库表名（database-names / table-names）是否拼写正确，并确认目标库中该表确实已创建。',
      raw: rawMessage.length > cleanMessage.length + 30 ? rawMessage : undefined,
    };
  }

  // 3. 网络连通性或超时
  if (
    /connection.*(?:refused|timed out|reset)/i.test(cleanMessage) ||
    /Communications link failure/i.test(cleanMessage) ||
    /failed to connect/i.test(cleanMessage)
  ) {
    return {
      title: fallbackTitle,
      description: cleanMessage,
      category: 'network',
      suggestion: '请检查目标数据源的主机地址（Host）和端口（Port）是否可正常连接，确认 SeaTunnel 节点与数据源之间未被安全组或防火墙拦截。',
      raw: rawMessage.length > cleanMessage.length + 30 ? rawMessage : undefined,
    };
  }

  // 4. 账号认证或权限
  if (
    /access denied/i.test(cleanMessage) ||
    /authentication failed/i.test(cleanMessage) ||
    /password/i.test(cleanMessage)
  ) {
    return {
      title: fallbackTitle,
      description: cleanMessage,
      category: 'auth',
      suggestion: '请检查数据源连接配置中的用户名与密码，确认该账号已被授予目标数据库的访问及数据读写权限。',
      raw: rawMessage.length > cleanMessage.length + 30 ? rawMessage : undefined,
    };
  }

  // 5. HOCON 语法结构错误
  if (
    /configparsefailed/i.test(cleanMessage) ||
    /ConfigException/i.test(cleanMessage) ||
    /syntax error/i.test(cleanMessage) ||
    /expecting/i.test(cleanMessage)
  ) {
    return {
      title: t('configParseFailedTitle'),
      description: cleanMessage,
      category: 'syntax',
      suggestion: '请检查任务配置的语法结构，特别注意大括号、中括号成对匹配以及字符串引号闭合情况。',
      raw: rawMessage.length > cleanMessage.length + 30 ? rawMessage : undefined,
    };
  }

  // 6. 默认通用/运行时错误
  return {
    title: fallbackTitle,
    description: cleanMessage || t('unknownError'),
    category: 'runtime',
    raw: rawMessage.length > cleanMessage.length + 30 ? rawMessage : undefined,
  };
}

// 将 definition 中的 custom_variables 与 custom_variable_types 转换为表格行列表
// Convert custom_variables and custom_variable_types from task definition into variable rows
export function toVariableRows(
  value: unknown,
  typesValue?: unknown,
): VariableRow[] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return [];
  }
  const typesMap =
    typesValue && typeof typesValue === 'object' && !Array.isArray(typesValue)
      ? (typesValue as Record<string, unknown>)
      : {};
  return Object.entries(value as Record<string, unknown>)
    .filter(([key]) => Boolean(key.trim()))
    .map(([key, item], index) => {
      const type = typesMap[key] === 'secret' ? 'secret' : 'string';
      return {
        id: `${key}-${index}`,
        key,
        value: typeof item === 'string' ? item : String(item ?? ''),
        type,
      };
    });
}

// 将自定义变量列表转换为任务定义中的键值映射
// Convert custom variable rows to key-value record for task definition
export function fromVariableRows(rows: VariableRow[]): Record<string, string> {
  const result: Record<string, string> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) {
      continue;
    }
    result[key] = row.value;
  }
  return result;
}

// 将自定义变量列表转换为任务定义中的类型映射
// Convert custom variable rows to type mapping record for task definition
export function fromVariableTypes(
  rows: VariableRow[],
): Record<string, 'string' | 'secret'> {
  const result: Record<string, 'string' | 'secret'> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) {
      continue;
    }
    result[key] = row.type === 'secret' ? 'secret' : 'string';
  }
  return result;
}

export function getExecutionMode(definition: SyncJSON | undefined): ExecutionMode {
  const value = definition?.execution_mode;
  if (value === 'local') {
    return 'local';
  }
  return 'cluster';
}

// ==========================================
// 预览与作业状态工具函数
// Preview and Job State Utilities
// ==========================================

export function extractPreviewRows(
  resultPreview: SyncJSON | undefined,
): Array<Record<string, unknown>> {
  const rows = resultPreview?.rows;
  if (!Array.isArray(rows)) {
    return [];
  }
  return rows.filter(
    (item) => item && typeof item === 'object' && !Array.isArray(item),
  ) as Array<Record<string, unknown>>;
}

export function extractPreviewDatasets(
  resultPreview: SyncJSON | undefined,
): SyncPreviewDataset[] {
  const datasets = resultPreview?.datasets;
  if (Array.isArray(datasets)) {
    return datasets
      .filter(
        (item) => item && typeof item === 'object' && !Array.isArray(item),
      )
      .map((item, index) => {
        const mapped = item as SyncJSON;
        const rows = Array.isArray(mapped.rows)
          ? (mapped.rows.filter(
              (row) => row && typeof row === 'object' && !Array.isArray(row),
            ) as SyncJSON[])
          : [];
        const explicitColumns = Array.isArray(mapped.columns)
          ? mapped.columns.map((column) => String(column))
          : rows.length > 0
            ? Object.keys(rows[0])
            : [];
        return {
          name:
            typeof mapped.name === 'string'
              ? mapped.name
              : `dataset-${index + 1}`,
          catalog: toObject(mapped.catalog),
          columns: explicitColumns,
          rows,
          page: typeof mapped.page === 'number' ? mapped.page : 1,
          page_size:
            typeof mapped.page_size === 'number'
              ? mapped.page_size
              : rows.length || 20,
          total: typeof mapped.total === 'number' ? mapped.total : rows.length,
          updated_at:
            typeof mapped.updated_at === 'string'
              ? mapped.updated_at
              : undefined,
        } satisfies SyncPreviewDataset;
      });
  }
  const rows = extractPreviewRows(resultPreview);
  const columns = extractPreviewColumns(rows, resultPreview);
  if (rows.length === 0 && columns.length === 0) {
    return [];
  }
  return [
    {
      name: 'preview_dataset',
      catalog: {},
      columns,
      rows,
      page: 1,
      page_size: rows.length || 20,
      total: rows.length,
    },
  ];
}

export function extractPreviewColumns(
  rows: Array<Record<string, unknown>>,
  resultPreview: SyncJSON | undefined,
): string[] {
  const explicit = resultPreview?.columns;
  if (Array.isArray(explicit)) {
    return explicit.map((item) => String(item));
  }
  if (rows.length > 0) {
    return Object.keys(rows[0]);
  }
  return [];
}

export function formatCellValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '-';
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}

export function getEngineAPIMode(job: SyncJobInstance | null): string {
  const mode = job?.submit_spec?.engine_api_mode;
  if (typeof mode === 'string' && mode.trim()) {
    return mode.trim().toLowerCase();
  }
  return 'v2';
}

export function submitSpecExecutionMode(spec: SyncJSON | undefined): ExecutionMode {
  if (spec?.execution_mode === 'local') {
    return 'local';
  }
  return 'cluster';
}

export function getEngineEndpointLabel(job: SyncJobInstance | null): string {
  if (job && submitSpecExecutionMode(job.submit_spec) === 'local') {
    const installDir = job.submit_spec?.install_dir;
    return typeof installDir === 'string' && installDir.trim()
      ? installDir.trim()
      : 'local-agent';
  }
  const baseURL = job?.submit_spec?.engine_base_url;
  if (typeof baseURL === 'string' && baseURL.trim()) {
    return baseURL.trim();
  }
  return '-';
}

export function getJobStatusBadgeClass(status: string): string {
  switch (
    String(status || '')
      .trim()
      .toUpperCase()
  ) {
    case 'SUCCESS':
    case 'FINISHED':
    case 'SAVEPOINT_DONE':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400';
    case 'RUNNING':
      return 'border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400';
    case 'DOING_SAVEPOINT':
      return 'border-violet-500/30 bg-violet-500/10 text-violet-600 dark:text-violet-400';
    case 'FAILED':
      return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400';
    case 'FAILING':
      return 'border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400';
    case 'CANCELING':
      return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400';
    case 'CANCELED':
    case 'CANCELLED':
      return 'border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400';
    case 'PENDING':
    case 'CREATED':
    case 'SCHEDULED':
    case 'STARTING':
    case 'SUBMITTED':
    case 'RECONCILING':
      return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400';
    default:
      return 'border-border/60 bg-muted/50 text-muted-foreground';
  }
}

export function getJobStatusLabel(status: string): string {
  switch (
    String(status || '')
      .trim()
      .toUpperCase()
  ) {
    case 'SUCCESS':
    case 'FINISHED':
      return 'Success';
    case 'SAVEPOINT_DONE':
      return 'Savepoint Done';
    case 'RUNNING':
      return 'Running';
    case 'DOING_SAVEPOINT':
      return 'Doing Savepoint';
    case 'FAILED':
      return 'Failed';
    case 'FAILING':
      return 'Failing';
    case 'CANCELING':
      return 'Canceling';
    case 'CANCELED':
      return 'Canceled';
    case 'PENDING':
      return 'Pending';
    case 'CREATED':
      return 'Created';
    case 'SCHEDULED':
      return 'Scheduled';
    case 'STARTING':
      return 'Starting';
    case 'SUBMITTED':
      return 'Submitted';
    case 'RECONCILING':
      return 'Reconciling';
    default:
      return status || '-';
  }
}

export function getDisplayJobLifecycleStatus(job: SyncJobInstance | null): string {
  if (!job) {
    return '-';
  }
  const rawJobStatus = String(toObject(job.result_preview).job_status || '')
    .trim()
    .toUpperCase();
  if (rawJobStatus) {
    return rawJobStatus;
  }
  return String(job.status || '-');
}

export function formatJobDateTime(value: string | null | undefined): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '-';
  }
  return date.toLocaleString();
}

export function formatJobDuration(
  startedAt: string | null | undefined,
  finishedAt: string | null | undefined,
): string {
  if (!startedAt) {
    return '-';
  }
  const start = new Date(startedAt);
  if (Number.isNaN(start.getTime())) {
    return '-';
  }
  const end = finishedAt ? new Date(finishedAt) : new Date();
  if (Number.isNaN(end.getTime())) {
    return '-';
  }
  const durationMs = Math.max(0, end.getTime() - start.getTime());
  const seconds = Math.floor(durationMs / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) {
    return `${minutes}m ${String(remainingSeconds).padStart(2, '0')}s`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${String(remainingMinutes).padStart(2, '0')}m ${String(remainingSeconds).padStart(2, '0')}s`;
}

export function getRunModeLabel(
  job: SyncJobInstance,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): string {
  const runType = String(job.run_type || '')
    .trim()
    .toLowerCase();
  if (runType === 'preview') {
    return t('runModePreview');
  }
  if (runType === 'schedule' || runType === 'scheduled') {
    return t('runModeSchedule');
  }
  const submitSpec = toObject(job.submit_spec);
  const triggerSource = String(
    submitSpec.trigger_source || submitSpec.trigger_mode || '',
  )
    .trim()
    .toLowerCase();
  if (triggerSource === 'schedule' || triggerSource === 'scheduled') {
    return t('runModeSchedule');
  }
  return t('runModeManual');
}

export function normalizeJobLifecycleStatus(
  status: string | null | undefined,
): string {
  return String(status || '')
    .trim()
    .toUpperCase();
}

export function isJobLifecycleActive(status: string | null | undefined): boolean {
  switch (normalizeJobLifecycleStatus(status)) {
    case 'PENDING':
    case 'CREATED':
    case 'SCHEDULED':
    case 'STARTING':
    case 'SUBMITTED':
    case 'RECONCILING':
    case 'RUNNING':
    case 'DOING_SAVEPOINT':
    case 'CANCELING':
      return true;
    default:
      return false;
  }
}

export function isJobLifecycleTerminal(status: string | null | undefined): boolean {
  switch (normalizeJobLifecycleStatus(status)) {
    case 'SUCCESS':
    case 'FINISHED':
    case 'SAVEPOINT_DONE':
    case 'FAILED':
    case 'FAILING':
    case 'CANCELED':
    case 'CANCELLED':
      return true;
    default:
      return false;
  }
}

export function canRecoverFromJob(job: SyncJobInstance | null): boolean {
  if (!job || job.run_type === 'preview') {
    return false;
  }
  if (submitSpecExecutionMode(job.submit_spec) === 'local') {
    return false;
  }
  if (!String(job.platform_job_id || '').trim()) {
    return false;
  }
  return isJobLifecycleTerminal(getDisplayJobLifecycleStatus(job));
}

export function getJobSubmittedScript(job: SyncJobInstance | null): {
  content: string;
  format: string;
} | null {
  if (!job) {
    return null;
  }
  const submitSpec = toObject(job.submit_spec);
  const submittedContent = normalizeStoredScriptContent(
    submitSpec.submitted_content,
  );
  if (submittedContent) {
    return {
      content: submittedContent,
      format: String(
        submitSpec.submitted_format || submitSpec.format || 'hocon',
      ),
    };
  }
  const previewContent = String(
    toObject(job.result_preview).preview_content || '',
  ).trim();
  if (previewContent) {
    return {
      content: previewContent,
      format: String(
        toObject(job.result_preview).content_format ||
          submitSpec.format ||
          'hocon',
      ),
    };
  }
  return null;
}

export function buildCheckpointInspectJobConfig(
  job: SyncJobInstance | null,
  editor: EditorState,
  customVariables: VariableRow[],
): RuntimeStorageCheckpointInspectJobConfig | undefined {
  const submittedScript = getJobSubmittedScript(job);
  if (submittedScript?.content?.trim()) {
    return {
      content: submittedScript.content,
      content_format: submittedScript.format || 'hocon',
      variables: {},
    };
  }
  if (!editor.content.trim()) {
    return undefined;
  }
  return {
    content: editor.content,
    content_format: editor.contentFormat || 'hocon',
    variables: fromVariableRows(customVariables),
  };
}

export function normalizeCheckpointActionIdentity(value: unknown): string {
  const raw = String(value || '').trim();
  if (!raw) {
    return '';
  }
  const bracketMatch = raw.match(/\[(.+)\]$/);
  return (bracketMatch?.[1] || raw).trim();
}

export type CheckpointActionViewModel = {
  key: string;
  actionName: string;
  actionState?: Record<string, unknown>;
  taskStatistics?: Record<string, unknown>;
  sourceState?: Record<string, unknown>;
  unsupportedSource?: Record<string, unknown>;
};

export function buildCheckpointActionViewModels(
  result: RuntimeStorageCheckpointInspectResult | null,
): CheckpointActionViewModel[] {
  const actionStates = Array.isArray(result?.action_states)
    ? result.action_states
    : [];
  const taskStatistics = Array.isArray(result?.task_statistics)
    ? result.task_statistics
    : [];
  const sourceStates = Array.isArray(result?.source_state_inspect?.sources)
    ? result.source_state_inspect.sources
    : [];
  const unsupportedSources = Array.isArray(
    result?.source_state_inspect?.unsupported_sources,
  )
    ? result.source_state_inspect.unsupported_sources
    : [];
  const actionMap = new Map<string, CheckpointActionViewModel>();

  const ensureEntry = (name: unknown): CheckpointActionViewModel => {
    const actionName = normalizeCheckpointActionIdentity(name);
    const key = actionName || `unknown-${actionMap.size}`;
    const existing = actionMap.get(key);
    if (existing) {
      return existing;
    }
    const created: CheckpointActionViewModel = {
      key,
      actionName,
    };
    actionMap.set(key, created);
    return created;
  };

  actionStates.forEach((item) => {
    const entry = ensureEntry(item.name);
    entry.actionState = item;
  });
  taskStatistics.forEach((item) => {
    const entry = ensureEntry(item.jobVertexId);
    entry.taskStatistics = item;
  });
  sourceStates.forEach((item) => {
    const entry = ensureEntry(item.actionName);
    entry.sourceState = item;
  });
  unsupportedSources.forEach((item) => {
    const entry = ensureEntry(item.actionName);
    entry.unsupportedSource = item;
  });

  return Array.from(actionMap.values()).sort((left, right) =>
    left.actionName.localeCompare(right.actionName),
  );
}

export type CheckpointSourceSummary = {
  target: string;
  offset: string;
  splitCount: number;
  progress: string;
};

export type CheckpointSubtaskViewRow = {
  subtaskIndex: number;
  splitCount: number;
  bytes: number;
  chunks: number;
  stateSize: number;
  status: string;
  ackTimestamp: unknown;
};

export function summarizeCheckpointSourceState(
  sourceState?: Record<string, unknown> | null,
): CheckpointSourceSummary {
  const subtasks = Array.isArray(sourceState?.subtasks)
    ? (sourceState?.subtasks as Record<string, unknown>[])
    : [];
  const firstSubtask = subtasks[0] || {};
  const firstSplits = Array.isArray(firstSubtask.splits)
    ? (firstSubtask.splits as Record<string, unknown>[])
    : [];
  const firstSplit = firstSplits[0] || {};
  const coordinator = toObject(sourceState?.coordinator);
  const snapshotPhase = toObject(coordinator.snapshotPhase);
  const incrementalPhase = toObject(coordinator.incrementalPhase);
  const startupOffset = toObject(firstSplit.startupOffset);
  const offsetValues = toObject(startupOffset.values);
  const tableIds = Array.isArray(firstSplit.tableIds)
    ? (firstSplit.tableIds as unknown[])
        .map((item) => String(item || '').trim())
        .filter(Boolean)
    : [];
  const processedTables = Array.isArray(snapshotPhase.alreadyProcessedTables)
    ? (snapshotPhase.alreadyProcessedTables as unknown[])
        .map((item) => String(item || '').trim())
        .filter(Boolean)
    : [];
  const pendingTables = Array.isArray(coordinator.pendingTables)
    ? (coordinator.pendingTables as unknown[])
        .map((item) => String(item || '').trim())
        .filter(Boolean)
    : [];
  const tableOffsets = toObject(coordinator.tableOffsets);

  let offset = '-';
  if (offsetValues.file || offsetValues.pos) {
    offset = `${String(offsetValues.file || '').trim()}:${String(
      offsetValues.pos || '',
    ).trim()}`.replace(/:$/, '');
  } else if (offsetValues.scn) {
    offset = `SCN ${String(offsetValues.scn)}`;
  } else if (offsetValues.lsn) {
    offset = `LSN ${String(offsetValues.lsn)}`;
  } else if (offsetValues.commit_lsn) {
    offset = `LSN ${String(offsetValues.commit_lsn)}`;
  } else if (offsetValues.resumeToken) {
    offset = `resumeToken ${String(offsetValues.resumeToken).slice(0, 24)}`;
  } else if (offsetValues.resolvedTs) {
    offset = `resolvedTs ${String(offsetValues.resolvedTs)}`;
  } else if (firstSplit.resolvedTs !== undefined) {
    offset = `resolvedTs ${String(firstSplit.resolvedTs)}`;
  } else if (firstSplit.latestConsumedId) {
    offset = String(firstSplit.latestConsumedId);
  } else if (firstSplit.startCursor) {
    offset = String(firstSplit.startCursor);
  } else if (firstSplit.currentOffset !== undefined) {
    offset = String(firstSplit.currentOffset);
  } else if (offsetValues.timestamp) {
    offset = String(offsetValues.timestamp);
  } else if (firstSplit.startOffset || firstSplit.currentOffset) {
    offset = String(firstSplit.currentOffset || firstSplit.startOffset);
  } else if (firstSplit.recordOffset !== undefined) {
    offset = `record ${String(firstSplit.recordOffset)}`;
  } else if (coordinator.currentSnapshotId !== undefined) {
    offset = `snapshot ${String(coordinator.currentSnapshotId)}`;
  } else if (coordinator.resolvedTs !== undefined) {
    offset = `resolvedTs ${String(coordinator.resolvedTs)}`;
  }

  let target = '-';
  if (tableIds.length > 0) {
    target = tableIds.join(', ');
  } else if (firstSplit.topic) {
    target = [
      String(firstSplit.topic),
      firstSplit.partition !== undefined
        ? `partition ${String(firstSplit.partition)}`
        : '',
    ]
      .filter(Boolean)
      .join(' / ');
  } else if (firstSplit.project || firstSplit.logStore) {
    target = [firstSplit.project, firstSplit.logStore, firstSplit.shardId]
      .filter(
        (item) => item !== undefined && item !== null && String(item) !== '',
      )
      .map((item) => String(item))
      .join(' / ');
  } else if (firstSplit.database || firstSplit.table) {
    target = [firstSplit.database, firstSplit.table]
      .filter(
        (item) => item !== undefined && item !== null && String(item) !== '',
      )
      .map((item) => String(item))
      .join('.');
  } else if (firstSplit.tableName) {
    target = String(firstSplit.tableName);
  } else if (firstSplit.tablePath) {
    target = String(firstSplit.tablePath);
  } else if (firstSplit.tableId) {
    target = String(firstSplit.tableId);
  } else if (processedTables.length > 0) {
    target = processedTables.join(', ');
  } else if (pendingTables.length > 0) {
    target = pendingTables.join(', ');
  } else if (firstSplit.splitId) {
    target = String(firstSplit.splitId);
  }

  let progress = '-';
  if (processedTables.length > 0) {
    progress = `${processedTables.length} tables`;
  } else if (pendingTables.length > 0) {
    progress = `${pendingTables.length} pending`;
  } else if (Object.keys(tableOffsets).length > 0) {
    progress = `${Object.keys(tableOffsets).length} table offsets`;
  } else if (coordinator.pendingSplitCount !== undefined) {
    progress = `${String(coordinator.pendingSplitCount)} pending splits`;
  } else if (coordinator.assignedSplitCount !== undefined) {
    progress = `${String(coordinator.assignedSplitCount)} assigned`;
  } else if (incrementalPhase.className) {
    progress = String(incrementalPhase.className).split('.').pop() || '-';
  }

  return {
    target,
    offset,
    splitCount: subtasks.reduce(
      (sum, item) => sum + Number(item.splitCount || 0),
      0,
    ),
    progress,
  };
}

export function toCheckpointNumber(value: unknown): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

export function buildCheckpointSubtaskRows(
  row: CheckpointActionViewModel,
): CheckpointSubtaskViewRow[] {
  const actionState = row.actionState || {};
  const statistics = row.taskStatistics || {};
  const sourceState = row.sourceState || {};
  const stateSubtasks = Array.isArray(actionState.subtasks)
    ? (actionState.subtasks as Record<string, unknown>[])
    : [];
  const statSubtasks = Array.isArray(statistics.subtasks)
    ? (statistics.subtasks as Record<string, unknown>[])
    : [];
  const sourceSubtasks = Array.isArray(sourceState.subtasks)
    ? (sourceState.subtasks as Record<string, unknown>[])
    : [];
  const rowMap = new Map<number, CheckpointSubtaskViewRow>();

  const ensureRow = (index: number): CheckpointSubtaskViewRow => {
    const normalized = Number.isFinite(index) ? index : rowMap.size;
    const existing = rowMap.get(normalized);
    if (existing) {
      return existing;
    }
    const created: CheckpointSubtaskViewRow = {
      subtaskIndex: normalized,
      splitCount: 0,
      bytes: 0,
      chunks: 0,
      stateSize: 0,
      status: '-',
      ackTimestamp: undefined,
    };
    rowMap.set(normalized, created);
    return created;
  };

  stateSubtasks.forEach((item, index) => {
    const rowItem = ensureRow(toCheckpointNumber(item.index ?? index));
    rowItem.bytes = toCheckpointNumber(item.bytes);
    rowItem.chunks = toCheckpointNumber(item.chunks);
  });
  statSubtasks.forEach((item, index) => {
    const rowItem = ensureRow(toCheckpointNumber(item.subtaskIndex ?? index));
    rowItem.stateSize = toCheckpointNumber(item.stateSize);
    rowItem.status = String(item.status || '-');
    rowItem.ackTimestamp = item.ackTimestamp;
  });
  sourceSubtasks.forEach((item, index) => {
    const rowItem = ensureRow(toCheckpointNumber(item.subtaskIndex ?? index));
    rowItem.splitCount = toCheckpointNumber(item.splitCount);
    rowItem.bytes = Math.max(rowItem.bytes, toCheckpointNumber(item.bytes));
  });

  return Array.from(rowMap.values()).sort(
    (left, right) => left.subtaskIndex - right.subtaskIndex,
  );
}

export function summarizeCheckpointSubtaskMetrics(rows: CheckpointSubtaskViewRow[]) {
  const metrics: Array<{
    key: keyof Pick<
      CheckpointSubtaskViewRow,
      'splitCount' | 'bytes' | 'chunks' | 'stateSize'
    >;
    label: string;
  }> = [
    {key: 'splitCount', label: 'Splits'},
    {key: 'bytes', label: 'Bytes'},
    {key: 'chunks', label: 'Chunks'},
    {key: 'stateSize', label: 'State Size'},
  ];
  const aggregate = (mode: 'min' | 'avg' | 'max') => {
    const entry: Record<string, unknown> = {metric: mode};
    metrics.forEach(({key, label}) => {
      const values = rows.map((item) => Number(item[key] || 0));
      if (values.length === 0) {
        entry[label] = 0;
        return;
      }
      if (mode === 'min') {
        entry[label] = Math.min(...values);
        return;
      }
      if (mode === 'max') {
        entry[label] = Math.max(...values);
        return;
      }
      entry[label] = Math.round(
        values.reduce((sum, value) => sum + value, 0) / values.length,
      );
    });
    return entry;
  };
  return [aggregate('min'), aggregate('avg'), aggregate('max')];
}

export function normalizeStoredScriptContent(value: unknown): string {
  const raw = String(value || '').trim();
  if (!raw) {
    return '';
  }
  if (raw.includes('\n') || raw.includes('\r')) {
    return raw;
  }
  if (!/^[A-Za-z0-9+/=]+$/.test(raw) || raw.length % 4 !== 0) {
    return raw;
  }
  try {
    if (typeof window === 'undefined') {
      return raw;
    }
    const decoded = window.atob(raw);
    if (/[\x00-\x08\x0B\x0C\x0E-\x1F]/.test(decoded)) {
      return raw;
    }
    if (
      decoded.includes('env {') ||
      decoded.includes('source {') ||
      decoded.includes('sink {') ||
      decoded.includes('transform {')
    ) {
      return decoded;
    }
    return raw;
  } catch {
    return raw;
  }
}

export function parseMetricNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

export function extractJobMetricSummary(job: SyncJobInstance): {
  readCount: number | null;
  writeCount: number | null;
  averageSpeed: number | null;
  metricCount: number;
} {
  const metrics = toObject(job.result_preview?.metrics);
  const readCount = parseMetricNumber(metrics.SourceReceivedCount);
  const writeCount =
    parseMetricNumber(metrics.SinkWriteCount) ??
    parseMetricNumber(metrics.SinkCommittedCount);
  const readQps = parseMetricNumber(metrics.SourceReceivedQPS);
  const writeQps =
    parseMetricNumber(metrics.SinkWriteQPS) ??
    parseMetricNumber(metrics.SinkCommittedQPS);
  let averageSpeed: number | null = null;
  if (readQps !== null && writeQps !== null) {
    averageSpeed = (readQps + writeQps) / 2;
  } else if (readQps !== null) {
    averageSpeed = readQps;
  } else if (writeQps !== null) {
    averageSpeed = writeQps;
  }
  return {
    readCount,
    writeCount,
    averageSpeed,
    metricCount: Object.keys(metrics).length,
  };
}

export function formatMetricValue(value: number | null, digits = 0): string {
  if (value === null) {
    return '-';
  }
  return digits > 0 ? value.toFixed(digits) : String(Math.round(value));
}

export function formatMetricDisplayValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '-';
  }
  if (typeof value === 'number') {
    return value.toLocaleString();
  }
  return String(value);
}

export function formatMetricCompactValue(value: number): string {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)} B`;
  }
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)} M`;
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)} K`;
  }
  return String(Math.round(value));
}

export function buildDisplayLogLines(logs: string, maxLines: number): string[] {
  const lines = splitLogLines(logs);
  if (lines.length <= maxLines) {
    return lines;
  }
  return lines.slice(lines.length - maxLines);
}

export function splitLogLines(logs: string): string[] {
  return logs.split('\n').filter((line) => line.trim() !== '');
}

export function getLogLineClass(line: string): string {
  const upper = line.toUpperCase();
  if (upper.includes(' ERROR ') || upper.includes('ERROR')) {
    return 'text-red-600 dark:text-red-400';
  }
  if (upper.includes(' WARN ') || upper.includes('WARNING')) {
    return 'text-amber-600 dark:text-amber-400';
  }
  return 'text-muted-foreground';
}

export function getPreviewRowKindBadgeClass(value: string): string {
  const normalized = value.trim().toUpperCase();
  switch (normalized) {
    case 'INSERT':
    case '+I':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400';
    case 'UPDATE':
    case 'UPDATE_AFTER':
    case '+U':
      return 'border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400';
    case 'DELETE':
    case 'UPDATE_BEFORE':
    case '-D':
    case '-U':
      return 'border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400';
    default:
      return 'border-border/60 bg-muted/50 text-muted-foreground';
  }
}

export function extractEditorState(task?: SyncTask | null): EditorState {
  if (!task) {
    return EMPTY_EDITOR;
  }
  return {
    id: task.id,
    parentId: task.parent_id,
    name: task.name || '',
    description: task.description || '',
    clusterId: task.cluster_id ? String(task.cluster_id) : '',
    contentFormat: 'hocon',
    content: task.content || '',
    definition: task.definition || {},
    currentVersion: task.current_version || 0,
    status: task.status || 'draft',
    createdBy: task.created_by,
    canEdit: task.can_edit ?? true,
    canRun: task.can_run ?? true,
    isOwner: task.is_owner ?? true,
    isCollaborator: task.is_collaborator ?? false,
    isPublic: task.is_public ?? true,
  };
}

export function extractEditorStateFromTreeNode(
  task?: SyncTaskTreeNode | null,
): EditorState {
  if (!task) {
    return EMPTY_EDITOR;
  }
  return {
    id: task.id,
    parentId: task.parent_id,
    name: task.name,
    description: task.description || '',
    clusterId: task.cluster_id ? String(task.cluster_id) : '',
    contentFormat: 'hocon',
    content: task.content || '',
    definition: task.definition || {},
    currentVersion: task.current_version || 0,
    status: task.status || 'draft',
    createdBy: task.created_by,
    canEdit: task.can_edit ?? true,
    canRun: task.can_run ?? true,
    isOwner: task.is_owner ?? true,
    isCollaborator: task.is_collaborator ?? false,
    isPublic: task.is_public ?? true,
  };
}

// 从任务定义中解析自定义变量列表
// Extract custom variable rows from task definition
export function extractVariableRowsFromDefinition(
  definition: SyncJSON,
): VariableRow[] {
  return toVariableRows(
    definition?.custom_variables,
    definition?.custom_variable_types,
  );
}

export function resolveFolderParent(
  selectedNodeId: number | null,
  tree: SyncTaskTreeNode[],
): number | null {
  if (!selectedNodeId) {
    return null;
  }
  const node = flattenTree(tree).find((item) => item.id === selectedNodeId);
  if (!node) {
    return null;
  }
  return node.node_type === 'folder' ? node.id : node.parent_id || null;
}

export function resolveDefaultPreviewHTTPSinkURL(): string {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}/api/v1/sync/preview/collect`;
  }
  return 'http://127.0.0.1:17800/api/v1/sync/preview/collect';
}

export function buildDefaultContent(format: SyncFormat): string {
  return (
    'env {\n' +
    '  job.mode = "BATCH"\n' +
    '  parallelism = 1\n' +
    '  job.retry.times = 0\n' +
    '  job.retry.interval.seconds = 3\n' +
    '  min-pause = -1\n' +
    '  savemode.execute.location = "CLUSTER"\n' +
    '  \n' +
    '  # Checkpoint configuration\n' +
    '  checkpoint.interval = 10000\n' +
    '  checkpoint.timeout = 30000\n' +
    '}\n' +
    '\n' +
    'source {\n' +
    '  FakeSource {\n' +
    '    result_table_name = "fake_source_table"\n' +
    '    row.num = 16\n' +
    '    schema = {\n' +
    '      fields {\n' +
    '        id = "bigint"\n' +
    '        name = "string"\n' +
    '        age = "int"\n' +
    '      }\n' +
    '    }\n' +
    '  }\n' +
    '}\n' +
    '\n' +
    'transform {\n' +
    '}\n' +
    '\n' +
    'sink {\n' +
    '  Console {\n' +
    '    source_table_name = "fake_source_table"\n' +
    '  }\n' +
    '}\n'
  );
}

export function formatMetricWithUnit(
  value: unknown,
  unit: 'rows' | 'qps' | 'bps',
): string {
  if (value === null || value === undefined || value === '') {
    return '-';
  }
  const raw =
    typeof value === 'string' || typeof value === 'number'
      ? Number(value)
      : Number.NaN;
  if (!Number.isFinite(raw)) {
    return formatMetricDisplayValue(value);
  }
  const compact = formatMetricCompactValue(raw);
  if (unit === 'rows') {
    return `${compact} rows`;
  }
  if (unit === 'qps') {
    return `${compact} QPS`;
  }
  return `${compact} B/s`;
}

export function getMetricValue(
  metrics: Record<string, unknown>,
  key: string,
): unknown {
  return metrics[key];
}

export function buildMetricHighlights(
  metrics: Record<string, unknown>,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): Array<{label: string; value: string; raw: string}> {
  return [
    {
      label: t('metricHighlightSourceRows'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SourceReceivedCount'),
        'rows',
      ),
      raw: formatMetricDisplayValue(
        getMetricValue(metrics, 'SourceReceivedCount'),
      ),
    },
    {
      label: t('metricHighlightSinkRows'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SinkWriteCount'),
        'rows',
      ),
      raw: formatMetricDisplayValue(getMetricValue(metrics, 'SinkWriteCount')),
    },
    {
      label: t('metricHighlightCommittedRows'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SinkCommittedCount'),
        'rows',
      ),
      raw: formatMetricDisplayValue(
        getMetricValue(metrics, 'SinkCommittedCount'),
      ),
    },
    {
      label: t('metricHighlightReadSpeed'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SourceReceivedBytesPerSeconds'),
        'bps',
      ),
      raw: formatMetricDisplayValue(
        getMetricValue(metrics, 'SourceReceivedBytesPerSeconds'),
      ),
    },
    {
      label: t('metricHighlightWriteSpeed'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SinkWriteBytesPerSeconds'),
        'bps',
      ),
      raw: formatMetricDisplayValue(
        getMetricValue(metrics, 'SinkWriteBytesPerSeconds'),
      ),
    },
    {
      label: t('metricHighlightWriteQps'),
      value: formatMetricWithUnit(
        getMetricValue(metrics, 'SinkWriteQPS'),
        'qps',
      ),
      raw: formatMetricDisplayValue(getMetricValue(metrics, 'SinkWriteQPS')),
    },
  ];
}

export function toStringMetricMap(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>).map(([key, item]) => [
      key,
      formatMetricDisplayValue(item),
    ]),
  );
}

export function buildPerTableMetricRows(metrics: Record<string, unknown>) {
  const sourceCount = toStringMetricMap(metrics.TableSourceReceivedCount);
  const sourceBytes = toStringMetricMap(metrics.TableSourceReceivedBytes);
  const sourceQps = toStringMetricMap(metrics.TableSourceReceivedQPS);
  const sinkCount = toStringMetricMap(metrics.TableSinkWriteCount);
  const sinkBytes = toStringMetricMap(metrics.TableSinkWriteBytes);
  const sinkQps = toStringMetricMap(metrics.TableSinkWriteQPS);
  const committedCount = toStringMetricMap(metrics.TableSinkCommittedCount);
  const committedBytes = toStringMetricMap(metrics.TableSinkCommittedBytes);

  const allTables = Array.from(
    new Set([
      ...Object.keys(sourceCount),
      ...Object.keys(sourceBytes),
      ...Object.keys(sourceQps),
      ...Object.keys(sinkCount),
      ...Object.keys(sinkBytes),
      ...Object.keys(sinkQps),
      ...Object.keys(committedCount),
      ...Object.keys(committedBytes),
    ]),
  ).sort();

  return allTables.map((table) => {
    const match = table.match(/^(Source|Sink)\[(\d+)\]\.(.+)$/);
    const nodeType = match?.[1] || 'Table';
    const nodeIndex = match?.[2] ? Number(match[2]) + 1 : null;
    const tablePath = match?.[3] || table;
    return {
      rawTable: table,
      nodeLabel: nodeIndex !== null ? `${nodeType} #${nodeIndex}` : nodeType,
      rowTone:
        nodeType === 'Source'
          ? 'source'
          : nodeType === 'Sink'
            ? 'sink'
            : 'neutral',
      tablePath,
      sourceCount: sourceCount[table] || '-',
      sourceBytes: sourceBytes[table] || '-',
      sourceQps: sourceQps[table] || '-',
      sinkCount: sinkCount[table] || '-',
      sinkBytes: sinkBytes[table] || '-',
      sinkQps: sinkQps[table] || '-',
      committedCount: committedCount[table] || '-',
      committedBytes: committedBytes[table] || '-',
    };
  });
}

export function normalizePairingTableKey(tablePath: string): string {
  const leaf =
    tablePath.split('.').pop()?.trim().toLowerCase() ||
    tablePath.trim().toLowerCase();
  return leaf.replace(/^archive_/, '');
}

export function buildPairedMetricRows(metrics: Record<string, unknown>) {
  const perTableRows = buildPerTableMetricRows(metrics);
  const sourceBuckets = new Map<string, typeof perTableRows>();
  const sinkBuckets = new Map<string, typeof perTableRows>();
  for (const row of perTableRows) {
    const key = normalizePairingTableKey(row.tablePath);
    if (row.rowTone === 'source') {
      const current = sourceBuckets.get(key) || [];
      current.push(row);
      sourceBuckets.set(key, current);
    } else if (row.rowTone === 'sink') {
      const current = sinkBuckets.get(key) || [];
      current.push(row);
      sinkBuckets.set(key, current);
    }
  }
  const keys = Array.from(
    new Set([
      ...Array.from(sourceBuckets.keys()),
      ...Array.from(sinkBuckets.keys()),
    ]),
  ).sort();
  return keys
    .map((key) => {
      const sourceRows = sourceBuckets.get(key) || [];
      const sinkRows = sinkBuckets.get(key) || [];
      if (sourceRows.length === 1 && sinkRows.length === 1) {
        const source = sourceRows[0];
        const sink = sinkRows[0];
        return {
          key,
          sourceNode: source.nodeLabel,
          sourceTable: source.tablePath,
          sinkNode: sink.nodeLabel,
          sinkTable: sink.tablePath,
          sourceCount: source.sourceCount,
          sourceBytes: source.sourceBytes,
          sourceQps: source.sourceQps,
          sinkCount: sink.sinkCount,
          sinkBytes: sink.sinkBytes,
          sinkQps: sink.sinkQps,
          committedCount: sink.committedCount,
          committedBytes: sink.committedBytes,
        };
      }
      return null;
    })
    .filter((item): item is NonNullable<typeof item> => item !== null);
}

export function classifyMetricGroup(key: string): MetricGroupKey {
  const normalized = key.toLowerCase();
  if (
    normalized.includes('source') ||
    normalized.includes('read') ||
    normalized.includes('receive')
  ) {
    return 'read';
  }
  if (
    normalized.includes('sink') ||
    normalized.includes('write') ||
    normalized.includes('commit')
  ) {
    return 'write';
  }
  if (
    normalized.includes('qps') ||
    normalized.includes('tps') ||
    normalized.includes('speed') ||
    normalized.includes('rate') ||
    normalized.includes('throughput')
  ) {
    return 'throughput';
  }
  if (
    normalized.includes('latency') ||
    normalized.includes('delay') ||
    normalized.includes('duration') ||
    normalized.includes('cost')
  ) {
    return 'latency';
  }
  if (
    normalized.includes('status') ||
    normalized.includes('error') ||
    normalized.includes('fail') ||
    normalized.includes('retry')
  ) {
    return 'status';
  }
  return 'other';
}

export function buildMetricGroups(
  metrics: unknown,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): Array<{
  key: MetricGroupKey;
  title: string;
  items: Array<{key: string; value: unknown}>;
}> {
  const rawMetrics = Object.entries(toObject(metrics));
  const groups: Record<MetricGroupKey, Array<{key: string; value: unknown}>> = {
    read: [],
    write: [],
    throughput: [],
    latency: [],
    status: [],
    other: [],
  };
  for (const [key, value] of rawMetrics) {
    groups[classifyMetricGroup(key)].push({key, value});
  }
  const metadata: Array<{key: MetricGroupKey; title: string}> = [
    {key: 'read', title: t('metricGroupRead')},
    {key: 'write', title: t('metricGroupWrite')},
    {key: 'throughput', title: t('metricGroupThroughput')},
    {key: 'latency', title: t('metricGroupLatency')},
    {key: 'status', title: t('metricGroupStatus')},
    {key: 'other', title: t('metricGroupOther')},
  ];
  return metadata
    .map((item) => ({
      ...item,
      items: groups[item.key].sort((left, right) =>
        left.key.localeCompare(right.key),
      ),
    }))
    .filter((item) => item.items.length > 0);
}

export function normalizePluginIdentity(value?: string | null): string {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '');
}

export function buildTemplatePluginItems(
  plugins: SyncPluginFactoryInfo[],
): TemplatePluginItem[] {
  return (plugins || [])
    .map((item) => {
      return {
        value: item.factory_identifier,
        label: item.factory_identifier,
        origin: item.origin,
      };
    })
    .filter(
      (item) =>
        normalizePluginIdentity(item.value) !==
        normalizePluginIdentity('MultiTableSink'),
    )
    .sort((left, right) => left.label.localeCompare(right.label));
}

export function resolveEditorPluginContext(
  content: string,
  lineNumber: number,
): {
  pluginType: SyncPluginType | null;
  factoryIdentifier: string | null;
} {
  const lines = content.split('\n').slice(0, Math.max(lineNumber, 1));
  const blockStack: string[] = [];
  let pluginType: SyncPluginType | null = null;
  let factoryIdentifier: string | null = null;

  for (const rawLine of lines) {
    const line = rawLine.replace(/#.*$/, '').trim();
    if (!line) {
      continue;
    }
    const opens = (line.match(/\{/g) || []).length;
    const closes = (line.match(/\}/g) || []).length;
    const typeMatch = line.match(/^(source|transform|sink|catalog)\s*\{$/i);
    if (typeMatch) {
      pluginType = typeMatch[1].toLowerCase() as SyncPluginType;
      blockStack.push(pluginType);
      continue;
    }
    if (
      pluginType &&
      !factoryIdentifier &&
      blockStack.length === 1 &&
      opens > 0 &&
      closes === 0
    ) {
      const pluginMatch = line.match(/^([A-Za-z0-9_.-]+)\s*\{$/);
      if (pluginMatch) {
        factoryIdentifier = pluginMatch[1];
        blockStack.push(factoryIdentifier);
        continue;
      }
    }
    for (let index = 0; index < opens; index += 1) {
      blockStack.push('{');
    }
    for (let index = 0; index < closes; index += 1) {
      const popped = blockStack.pop();
      if (popped && factoryIdentifier && popped === factoryIdentifier) {
        factoryIdentifier = null;
      } else if (popped && pluginType && popped === pluginType) {
        pluginType = null;
      }
    }
  }

  return {pluginType, factoryIdentifier};
}

export function findTopLevelBlockInsertOffset(
  content: string,
  pluginType: SyncPluginType,
): number | null {
  const lines = content.split('\n');
  let depth = 0;
  let insideTarget = false;
  let offset = 0;

  for (const rawLine of lines) {
    const line = rawLine.replace(/#.*$/, '').trim();
    const opens = (line.match(/\{/g) || []).length;
    const closes = (line.match(/\}/g) || []).length;
    if (!insideTarget && depth === 0 && line === `${pluginType} {`) {
      insideTarget = true;
      depth += opens - closes;
      offset += rawLine.length + 1;
      continue;
    }
    if (insideTarget && depth === 1 && closes > 0) {
      return offset;
    }
    depth += opens - closes;
    offset += rawLine.length + 1;
  }

  return null;
}

export function resolveOptionKeyFromLine(
  lineContent: string,
  column: number,
): {
  key: string | null;
  startColumn: number;
  endColumn: number;
} {
  const commentedMatch = lineContent.match(/^(\s*#+\s*)([A-Za-z0-9_.-]+)/);
  if (commentedMatch?.[2]) {
    const key = commentedMatch[2];
    const startColumn = commentedMatch[1].length + 1;
    const endColumn = startColumn + key.length;
    if (column >= startColumn && column <= endColumn) {
      return {key, startColumn, endColumn};
    }
    return {key: null, startColumn, endColumn};
  }

  const assignmentMatch = lineContent.match(/^(\s*)([A-Za-z0-9_.-]+)\s*=/);
  if (!assignmentMatch || !assignmentMatch[2]) {
    return {key: null, startColumn: column, endColumn: column};
  }
  const key = assignmentMatch[2];
  const startColumn = assignmentMatch[1].length + 1;
  const endColumn = startColumn + key.length;
  if (column < startColumn || column > endColumn) {
    return {key: null, startColumn, endColumn};
  }
  return {key, startColumn, endColumn};
}

export function buildInsertedTemplateContent(
  content: string,
  pluginType: SyncPluginType,
  pluginBlock: string,
): {
  nextContent: string;
  startOffset: number;
  endOffset: number;
} {
  const existingInsertOffset = findTopLevelBlockInsertOffset(
    content,
    pluginType,
  );
  if (existingInsertOffset !== null) {
    const insertText = `  ${pluginBlock.replace(/\n/g, '\n  ')}\n`;
    const nextContent =
      content.slice(0, existingInsertOffset) +
      insertText +
      content.slice(existingInsertOffset);
    return {
      nextContent,
      startOffset: existingInsertOffset,
      endOffset: existingInsertOffset + insertText.length - 1,
    };
  }

  const prefix = content.trim().length > 0 ? '\n\n' : '';
  const wrappedBlock = `${pluginType} {\n  ${pluginBlock.replace(
    /\n/g,
    '\n  ',
  )}\n}`;
  const nextContent = `${content}${prefix}${wrappedBlock}`;
  return {
    nextContent,
    startOffset: content.length + prefix.length,
    endOffset: nextContent.length,
  };
}
