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

'use client';

import {useMonaco} from '@monaco-editor/react';
import dynamic from 'next/dynamic';
import {useTranslations} from 'next-intl';
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
} from 'react';
import {useTheme} from 'next-themes';
import {toast} from 'sonner';
import {
  Bug,
  Check,
  Copy,
  ChevronDown,
  ChevronRight,
  Columns2,
  Folder,
  FolderPlus,
  FileCode2,
  FilePlus2,
  GitBranch,
  BarChart3,
  Maximize2,
  Play,
  RefreshCw,
  Save,
  Search,
  SquareTerminal,
  FolderTree,
  Database,
  ListTree,
  Square,
  Trash2,
  Funnel,
  GitCompareArrows,
  LayoutPanelTop,
  Globe2,
  MoreHorizontal,
  Pencil,
  Plus,
  Minus,
  Loader2,
  Eye,
  Clock3,
} from 'lucide-react';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {
  RuntimeStorageCheckpointInspectJobConfig,
  RuntimeStorageCheckpointInspectResult,
  RuntimeStorageListItem,
} from '@/lib/services/cluster/types';
import type {
  CreateSyncTaskRequest,
  SyncCheckpointSnapshot,
  SyncDagResult,
  SyncFormat,
  SyncGlobalVariable,
  SyncJobInstance,
  SyncJobLogsResult,
  SyncJSON,
  SyncPluginFactoryInfo,
  SyncPluginType,
  SyncPreviewDataset,
  SyncPreviewSnapshot,
  SyncTask,
  SyncTaskTreeNode,
  SyncTaskVersion,
  SyncValidateResult,
} from '@/lib/services/sync';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {WebUiDagPreview} from '@/components/common/sync/WebUiDagPreview';
import {
  TaskScheduleSidebarPanel,
  type TaskScheduleValue,
} from '@/components/common/sync/TaskScheduleSidebarPanel';
import {GlobalVariableDialog} from './GlobalVariableDialog';
import {GlobalVariablesSidebarPanel} from './GlobalVariablesSidebarPanel';
import {
  CustomVariableDialog,
  type CustomVariableItem,
} from './CustomVariableDialog';
import {CustomVariablesSection} from './CustomVariablesSection';
import type {CreateSyncGlobalVariableRequest} from '@/lib/services/sync';

const MonacoEditor = dynamic(() => import('@monaco-editor/react'), {
  ssr: false,
});

const MonacoDiffEditor = dynamic(
  () => import('@monaco-editor/react').then((module) => module.DiffEditor),
  {
    ssr: false,
  },
);

interface EditorState {
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
}

interface TreeContextMenuState {
  open: boolean;
  x: number;
  y: number;
  kind: 'root' | 'folder' | 'file';
  node: SyncTaskTreeNode | null;
}

interface TreeDialogState {
  open: boolean;
  action: 'create-folder' | 'create-file' | 'rename' | 'move' | 'delete' | null;
  targetNode: SyncTaskTreeNode | null;
  name: string;
  targetParentId: number | null;
}

interface OpenFileTab {
  id: number;
  name: string;
}

interface EditorDraftState {
  editor: EditorState;
  customVariableRows: VariableRow[];
  dirty: boolean;
  baselineEditor: EditorState;
  baselineCustomVariableRows: VariableRow[];
}

interface PersistedWorkspaceTabs {
  openTabIds: number[];
  activeTabId: number | null;
}

type VariableRow = CustomVariableItem;

interface VariableDraft {
  key: string;
  value: string;
}

interface PreviewRunDialogState {
  open: boolean;
  rowLimit: string;
  timeoutMinutes: string;
}

interface UserFacingErrorState {
  title: string;
  description: string;
  raw?: string;
}

type RightSidebarTab = 'settings' | 'versions' | 'globals';
type BottomConsoleTab = 'jobs' | 'logs' | 'preview' | 'checkpoint';
type ExecutionMode = 'cluster' | 'local';
type LogFilterMode = 'all' | 'warn' | 'error';
type PendingActionKind = 'dag' | 'preview' | 'test_connections' | 'recover';

interface TemplatePluginItem {
  value: string;
  label: string;
  origin?: string;
}

type OptionMetadataMap = Record<string, any>;

type PluginEnumCatalogMap = Partial<
  Record<SyncPluginType | 'env', Record<string, OptionMetadataMap>>
>;

function isCursorInsideValueRegion(
  lineContent: string,
  column: number,
): boolean {
  const equalsIndex = lineContent.indexOf('=');
  if (equalsIndex < 0) {
    return false;
  }
  const prefix = lineContent.slice(0, equalsIndex);
  if (!/^\s*#*\s*[A-Za-z0-9_.-]+\s*$/.test(prefix)) {
    return false;
  }
  return column >= equalsIndex + 2;
}

function formatMetadataValue(value: unknown): string {
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

function resolveEnumSuggestionItems(metadata: any): Array<{
  label: string;
  value: string;
}> {
  const values = Array.isArray(metadata?.enum_values)
    ? metadata.enum_values
    : [];
  const displays = Array.isArray(metadata?.enum_display_values)
    ? metadata.enum_display_values
    : [];
  return values.map((value: string, index: number) => ({
    label: displays[index] || value,
    value,
  }));
}

function resolveEnumValueBounds(lineContent: string, lineNumber: number) {
  const assignmentMatch = lineContent.match(
    /^(\s*[A-Za-z0-9_.-]+\s*=\s*)(.*)$/,
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

function resolveEnumSuggestRange(position: {
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

function ensureSyncHoconLanguage(monaco: any) {
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
}

const ENV_OPTION_METADATA: Record<
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

const LOG_CHUNK_BASE_BYTES = 64 * 1024;
const LOG_CHUNK_MAX_BYTES = 1024 * 1024;
const EXPANDED_LOG_CHUNK_BASE_BYTES = 256 * 1024;
const EXPANDED_LOG_CHUNK_MAX_BYTES = 2 * 1024 * 1024;
const WORKSPACE_TABS_STORAGE_KEY = 'data-sync-studio:workspace-tabs';

function getSyncJobClusterId(job: SyncJobInstance | null): number | null {
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

function formatSizeBytes(bytes?: number | null): string {
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

function normalizeVariableRowsForCompare(rows: VariableRow[]): Record<string, unknown>[] {
  return rows.map((row) => ({
    key: row.key.trim(),
    value: row.value,
    type: row.type || 'string',
    description: row.description || '',
  }));
}

function normalizeEditorForCompare(
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

function isEditorDraftDirty(
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

function getCheckpointStatusBadgeClass(status?: string): string {
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

function getCheckpointEnumBadgeClass(
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

function formatCheckpointFieldValue(
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

function renderCheckpointFieldValue(key: string, value: unknown): ReactNode {
  if (typeof value === 'string') {
    if (/status/i.test(key)) {
      return (
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm border px-2 py-0.5 text-[11px]',
            getCheckpointEnumBadgeClass(value, 'status'),
          )}
        >
          {value}
        </Badge>
      );
    }
    if (/checkpointType/i.test(key)) {
      return (
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm border px-2 py-0.5 text-[11px]',
            getCheckpointEnumBadgeClass(value, 'checkpointType'),
          )}
        >
          {value}
        </Badge>
      );
    }
  }
  if (typeof value === 'boolean') {
    return (
      <Badge
        variant='outline'
        className={cn(
          'rounded-sm border px-2 py-0.5 text-[11px]',
          getCheckpointEnumBadgeClass(value, 'boolean'),
        )}
      >
        {value ? 'true' : 'false'}
      </Badge>
    );
  }
  return (
    <span className='break-all'>{formatCheckpointFieldValue(key, value)}</span>
  );
}

function buildCheckpointInspectSummary(
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

function extractCheckpointFileIdentity(
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

function nextLogChunkSize(
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

function buildCopiedWorkspaceName(
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

function getPendingActionLabel(
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
type MetricGroupKey =
  | 'read'
  | 'write'
  | 'throughput'
  | 'latency'
  | 'status'
  | 'other';

const EMPTY_EDITOR: EditorState = {
  name: '',
  description: '',
  clusterId: '',
  contentFormat: 'hocon',
  content: '',
  definition: {},
  currentVersion: 0,
  status: 'draft',
};

const WORKSPACE_NAME_PATTERN = /^[\p{L}\p{N}._-]+$/u;

function toObject(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return {};
}

function flattenTree(nodes: SyncTaskTreeNode[]): SyncTaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTree(node.children || [])]);
}

function collectFolderIds(nodes: SyncTaskTreeNode[]): number[] {
  return flattenTree(nodes)
    .filter((node) => node.node_type === 'folder')
    .map((node) => node.id);
}

function findTreeNode(
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

function isTreeDescendant(
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

function listMoveTargets(
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

function patchTreeNode(
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

function filterTree(
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

function detectVariables(content: string): string[] {
  const matches = [...content.matchAll(/\{\{\s*([^{}]+?)\s*\}\}/g)];
  return Array.from(
    new Set(
      matches.map((match) => match[1]?.trim()).filter(Boolean) as string[],
    ),
  ).sort();
}

function isReservedBuiltinVariableKey(key: string): boolean {
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
function validateCustomVariableRows(
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

function padTimeUnit(value: number): string {
  return String(value).padStart(2, '0');
}

function formatBuiltinPreviewDate(date: Date, pattern: string): string {
  return pattern
    .replaceAll('yyyy', String(date.getFullYear()))
    .replaceAll('MM', padTimeUnit(date.getMonth() + 1))
    .replaceAll('dd', padTimeUnit(date.getDate()))
    .replaceAll('HH', padTimeUnit(date.getHours()))
    .replaceAll('mm', padTimeUnit(date.getMinutes()))
    .replaceAll('ss', padTimeUnit(date.getSeconds()));
}

function addMonths(date: Date, months: number): Date {
  const next = new Date(date.getTime());
  next.setMonth(next.getMonth() + months);
  return next;
}

function startOfWeek(date: Date): Date {
  const next = new Date(date.getTime());
  const day = next.getDay() === 0 ? 7 : next.getDay();
  next.setDate(next.getDate() - day + 1);
  return next;
}

function yearWeek(date: Date, weekStart = 1): {year: number; week: number} {
  const next = new Date(date.getTime());
  const jsWeekStart = weekStart === 7 ? 0 : weekStart;
  const day = next.getDay();
  const diff = (7 + day - jsWeekStart) % 7;
  next.setDate(next.getDate() - diff);
  const first = new Date(
    next.getFullYear(),
    0,
    1,
    next.getHours(),
    next.getMinutes(),
    next.getSeconds(),
    next.getMilliseconds(),
  );
  const firstDay = first.getDay();
  const firstDiff = (7 + firstDay - jsWeekStart) % 7;
  first.setDate(first.getDate() - firstDiff);
  const week =
    Math.floor((next.getTime() - first.getTime()) / (7 * 24 * 60 * 60 * 1000)) +
    1;
  return {year: next.getFullYear(), week};
}

function resolveBuiltinPreviewExpression(
  expr: string,
  now = new Date(),
): string | null {
  const trimmed = expr.trim();
  if (!trimmed) {
    return null;
  }
  if (trimmed === 'system.biz.date') {
    const prev = new Date(now.getTime());
    prev.setDate(prev.getDate() - 1);
    return formatBuiltinPreviewDate(prev, 'yyyyMMdd');
  }
  if (trimmed === 'system.biz.curdate') {
    return formatBuiltinPreviewDate(now, 'yyyyMMdd');
  }
  if (trimmed === 'system.datetime') {
    return formatBuiltinPreviewDate(now, 'yyyyMMddHHmmss');
  }
  if (trimmed === 'system.project.name') {
    return 'STX';
  }
  if (trimmed === 'system.project.code') {
    return 'stx';
  }
  if (/^add_months\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^add_months\((.+),(.+)\)$/);
    if (!match) return null;
    const format = match[1].trim();
    const offset = Number(match[2].trim());
    if (!Number.isFinite(offset)) return null;
    return formatBuiltinPreviewDate(addMonths(now, offset), format);
  }
  if (/^this_day\((.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^this_day\((.+)\)$/);
    return match ? formatBuiltinPreviewDate(now, match[1].trim()) : null;
  }
  if (/^last_day\((.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^last_day\((.+)\)$/);
    if (!match) return null;
    const prev = new Date(now.getTime());
    prev.setDate(prev.getDate() - 1);
    return formatBuiltinPreviewDate(prev, match[1].trim());
  }
  if (/^month_first_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^month_first_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = addMonths(now, Number(match[2].trim()));
    const first = new Date(
      target.getFullYear(),
      target.getMonth(),
      1,
      target.getHours(),
      target.getMinutes(),
      target.getSeconds(),
    );
    return formatBuiltinPreviewDate(first, match[1].trim());
  }
  if (/^month_last_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^month_last_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = addMonths(now, Number(match[2].trim()) + 1);
    const last = new Date(
      target.getFullYear(),
      target.getMonth(),
      0,
      target.getHours(),
      target.getMinutes(),
      target.getSeconds(),
    );
    return formatBuiltinPreviewDate(last, match[1].trim());
  }
  if (/^week_first_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^week_first_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = new Date(now.getTime());
    target.setDate(target.getDate() + Number(match[2].trim()) * 7);
    return formatBuiltinPreviewDate(startOfWeek(target), match[1].trim());
  }
  if (/^week_last_day\((.+),(.+)\)$/.test(trimmed)) {
    const match = trimmed.match(/^week_last_day\((.+),(.+)\)$/);
    if (!match) return null;
    const target = new Date(now.getTime());
    target.setDate(target.getDate() + Number(match[2].trim()) * 7);
    const end = startOfWeek(target);
    end.setDate(end.getDate() + 6);
    return formatBuiltinPreviewDate(end, match[1].trim());
  }
  if (
    /^year_week\((.+)\)$/.test(trimmed) ||
    /^year_week\((.+),(.+)\)$/.test(trimmed)
  ) {
    const match = trimmed.match(/^year_week\((.+?)(?:,(.+))?\)$/);
    if (!match) return null;
    const format = match[1].trim();
    const weekStart = match[2] ? Number(match[2].trim()) : 1;
    const result = yearWeek(now, Number.isFinite(weekStart) ? weekStart : 1);
    return format
      .replaceAll('yyyy', String(result.year))
      .replaceAll('MM', padTimeUnit(result.week));
  }
  const offsetMatch = trimmed.match(/^(.+?)([+-])(\d+(?:\/\d+)*)$/);
  if (offsetMatch) {
    const [, format, sign, rawOffset] = offsetMatch;
    const [first, ...rest] = rawOffset.split('/');
    const offset = rest.reduce(
      (acc, value) => acc / Number(value),
      Number(first),
    );
    const hours = (sign === '-' ? -1 : 1) * offset * 24;
    const target = new Date(now.getTime() + hours * 60 * 60 * 1000);
    return formatBuiltinPreviewDate(target, format.trim());
  }
  if (/(yyyy|MM|dd|HH|mm|ss)/.test(trimmed)) {
    return formatBuiltinPreviewDate(now, trimmed);
  }
  return null;
}

function extractTaskScheduleValue(definition: SyncJSON): TaskScheduleValue {
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

const BUILTIN_TIME_VARIABLE_ITEMS = [
  {expr: 'system.biz.curdate', descKey: 'builtinSystemBizCurdateDesc'},
  {expr: 'system.biz.date', descKey: 'builtinSystemBizDateDesc'},
  {expr: 'system.datetime', descKey: 'builtinSystemDateTimeDesc'},
  {expr: 'yyyyMMdd', descKey: 'builtinFormatDesc'},
  {expr: 'yyyy-MM-dd', descKey: 'builtinFormatDesc'},
  {expr: 'yyyyMMdd+1', descKey: 'builtinOffsetDesc'},
  {expr: 'add_months(yyyyMMdd,-1)', descKey: 'builtinAddMonthsDesc'},
  {expr: 'this_day(yyyy-MM-dd)', descKey: 'builtinThisDayDesc'},
  {expr: 'last_day(yyyy-MM-dd)', descKey: 'builtinLastDayDesc'},
  {expr: 'year_week(yyyy-MM-dd)', descKey: 'builtinYearWeekDesc'},
  {expr: 'month_first_day(yyyy-MM-dd,0)', descKey: 'builtinMonthFirstDayDesc'},
  {expr: 'month_last_day(yyyy-MM-dd,0)', descKey: 'builtinMonthLastDayDesc'},
  {expr: 'week_first_day(yyyy-MM-dd,0)', descKey: 'builtinWeekFirstDayDesc'},
  {expr: 'week_last_day(yyyy-MM-dd,0)', descKey: 'builtinWeekLastDayDesc'},
] as const;

function validateWorkspaceName(
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

function listSiblingNames(
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

function hasDuplicateWorkspaceName(
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

function formatSyncUserFacingError(
  error: unknown,
  fallbackTitle: string,
  t: ReturnType<typeof useTranslations<'workbenchStudio'>>,
): UserFacingErrorState {
  const message = error instanceof Error ? error.message : t('unknownError');
  if (message.includes('sync: task has not been published')) {
    return {
      title: t('saveRequiredTitle'),
      description: t('saveRequiredDescription'),
      raw: error instanceof Error ? error.message : message,
    };
  }
  if (message.includes('sync: 配置解析失败')) {
    return {
      title: t('configParseFailedTitle'),
      description: message.replace(/^sync:\s*/, ''),
      raw: error instanceof Error ? error.message : message,
    };
  }
  if (message.includes('sync: DAG 解析失败')) {
    return {
      title: fallbackTitle,
      description: message.replace(/^sync:\s*/, ''),
      raw: error instanceof Error ? error.message : message,
    };
  }
  return {
    title: fallbackTitle,
    description: message,
    raw: error instanceof Error ? error.message : undefined,
  };
}

// 将 definition 中的 custom_variables 与 custom_variable_types 转换为表格行列表
// Convert custom_variables and custom_variable_types from task definition into variable rows
function toVariableRows(
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
function fromVariableRows(rows: VariableRow[]): Record<string, string> {
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
function fromVariableTypes(
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

function getExecutionMode(definition: SyncJSON | undefined): ExecutionMode {
  const value = definition?.execution_mode;
  if (value === 'local') {
    return 'local';
  }
  return 'cluster';
}

function extractPreviewRows(
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

function extractPreviewDatasets(
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

function extractPreviewColumns(
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

function formatCellValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '-';
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}

function getEngineAPIMode(job: SyncJobInstance | null): string {
  const mode = job?.submit_spec?.engine_api_mode;
  if (typeof mode === 'string' && mode.trim()) {
    return mode.trim().toLowerCase();
  }
  return 'v2';
}

function submitSpecExecutionMode(spec: SyncJSON | undefined): ExecutionMode {
  if (spec?.execution_mode === 'local') {
    return 'local';
  }
  return 'cluster';
}

function getEngineEndpointLabel(job: SyncJobInstance | null): string {
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

function getJobStatusBadgeClass(status: string): string {
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

function getJobStatusLabel(status: string): string {
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
    case 'CANCELLED':
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

function getDisplayJobLifecycleStatus(job: SyncJobInstance | null): string {
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

function formatJobDateTime(value: string | null | undefined): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '-';
  }
  return date.toLocaleString();
}

function formatJobDuration(
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

function getRunModeLabel(
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

function normalizeJobLifecycleStatus(
  status: string | null | undefined,
): string {
  return String(status || '')
    .trim()
    .toUpperCase();
}

function isJobLifecycleActive(status: string | null | undefined): boolean {
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

function isJobLifecycleTerminal(status: string | null | undefined): boolean {
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

function canRecoverFromJob(job: SyncJobInstance | null): boolean {
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

function getJobSubmittedScript(job: SyncJobInstance | null): {
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

function buildCheckpointInspectJobConfig(
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

function normalizeCheckpointActionIdentity(value: unknown): string {
  const raw = String(value || '').trim();
  if (!raw) {
    return '';
  }
  const bracketMatch = raw.match(/\[(.+)\]$/);
  return (bracketMatch?.[1] || raw).trim();
}

type CheckpointActionViewModel = {
  key: string;
  actionName: string;
  actionState?: Record<string, unknown>;
  taskStatistics?: Record<string, unknown>;
  sourceState?: Record<string, unknown>;
  unsupportedSource?: Record<string, unknown>;
};

function buildCheckpointActionViewModels(
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

type CheckpointSourceSummary = {
  target: string;
  offset: string;
  splitCount: number;
  progress: string;
};

type CheckpointSubtaskViewRow = {
  subtaskIndex: number;
  splitCount: number;
  bytes: number;
  chunks: number;
  stateSize: number;
  status: string;
  ackTimestamp: unknown;
};

function summarizeCheckpointSourceState(
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

function toCheckpointNumber(value: unknown): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function buildCheckpointSubtaskRows(
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

function summarizeCheckpointSubtaskMetrics(rows: CheckpointSubtaskViewRow[]) {
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

function normalizeStoredScriptContent(value: unknown): string {
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

function parseMetricNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function extractJobMetricSummary(job: SyncJobInstance): {
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

function formatMetricValue(value: number | null, digits = 0): string {
  if (value === null) {
    return '-';
  }
  return digits > 0 ? value.toFixed(digits) : String(Math.round(value));
}

function buildDisplayLogLines(logs: string, maxLines: number): string[] {
  const lines = splitLogLines(logs);
  if (lines.length <= maxLines) {
    return lines;
  }
  return lines.slice(lines.length - maxLines);
}

function splitLogLines(logs: string): string[] {
  return logs.split('\n').filter((line) => line.trim() !== '');
}

function getLogLineClass(line: string): string {
  const upper = line.toUpperCase();
  if (upper.includes(' ERROR ') || upper.includes('ERROR')) {
    return 'text-red-600 dark:text-red-400';
  }
  if (upper.includes(' WARN ') || upper.includes('WARNING')) {
    return 'text-amber-600 dark:text-amber-400';
  }
  return 'text-muted-foreground';
}

function getPreviewRowKindBadgeClass(value: string): string {
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

function extractEditorState(task?: SyncTask | null): EditorState {
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
  };
}

function extractEditorStateFromTreeNode(
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
  };
}

// 从任务定义中解析自定义变量列表
// Extract custom variable rows from task definition
function extractVariableRowsFromDefinition(
  definition: SyncJSON,
): VariableRow[] {
  return toVariableRows(
    definition?.custom_variables,
    definition?.custom_variable_types,
  );
}

function resolveFolderParent(
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

function resolveDefaultPreviewHTTPSinkURL(): string {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}/api/v1/sync/preview/collect`;
  }
  return 'http://127.0.0.1:17800/api/v1/sync/preview/collect';
}

function buildDefaultContent(format: SyncFormat): string {
  return (
    'env {\n' +
    '  job.mode = "BATCH"\n' +
    '  parallelism = 1\n' +
    '  job.retry.times = 0\n' +
    '  job.retry.interval.seconds = 3\n' +
    '  min-pause = -1\n' +
    '  savemode.execute.location = "CLUSTER"\n' +
    '  \n' +
    '  ## limit speed\n' +
    '  # read_limit.rows_per_second = 1000\n' +
    '  # read_limit.bytes_per_second = 1048576\n' +
    '  \n' +
    '  ## checkpoint \n' +
    '  # checkpoint.interval = 10000\n' +
    '  # checkpoint.timeout = 30000\n' +
    '}\n\n' +
    'source {\n' +
    '}\n\n' +
    'transform {\n' +
    '}\n\n' +
    'sink {\n' +
    '  Console {}\n' +
    '}\n'
  );
}

function formatMetricDisplayValue(value: unknown): string {
  if (value === null || value === undefined || value === '') {
    return '-';
  }
  if (typeof value === 'number') {
    return Number.isInteger(value) ? String(value) : value.toFixed(2);
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}

function formatMetricCompactValue(value: unknown): string {
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
  if (Math.abs(raw) >= 1000000) {
    return `${(raw / 1000000).toFixed(2)}M`;
  }
  if (Math.abs(raw) >= 1000) {
    return `${(raw / 1000).toFixed(2)}K`;
  }
  return Number.isInteger(raw) ? String(raw) : raw.toFixed(2);
}

function formatMetricWithUnit(
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

function getMetricValue(
  metrics: Record<string, unknown>,
  key: string,
): unknown {
  return metrics[key];
}

function buildMetricHighlights(
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

function toStringMetricMap(value: unknown): Record<string, string> {
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

function buildPerTableMetricRows(metrics: Record<string, unknown>) {
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

function normalizePairingTableKey(tablePath: string): string {
  const leaf =
    tablePath.split('.').pop()?.trim().toLowerCase() ||
    tablePath.trim().toLowerCase();
  return leaf.replace(/^archive_/, '');
}

function buildPairedMetricRows(metrics: Record<string, unknown>) {
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

function classifyMetricGroup(key: string): MetricGroupKey {
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

function buildMetricGroups(
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

function normalizePluginIdentity(value?: string | null): string {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '');
}

function buildTemplatePluginItems(
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

function resolveEditorPluginContext(
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

function findTopLevelBlockInsertOffset(
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

function resolveOptionKeyFromLine(
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

function buildInsertedTemplateContent(
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

export function DataSyncStudio() {
  const t = useTranslations('workbenchStudio');
  const {resolvedTheme} = useTheme();
  const monacoFromHook = useMonaco();
  const editorInstanceRef = useRef<any>(null);
  const monacoInstanceRef = useRef<any>(null);
  const completionDisposableRef = useRef<any>(null);
  const hoverDisposableRef = useRef<any>(null);
  const contentChangeDisposableRef = useRef<any>(null);
  const cursorPositionChangeDisposableRef = useRef<any>(null);
  const cursorSelectionChangeDisposableRef = useRef<any>(null);
  const lastSuggestTriggerRef = useRef('');
  const enumCompletionCommandIdRef = useRef('sync.applyEnumCompletion');
  const enumCompletionCommandRegisteredRef = useRef(false);
  const monacoLanguageReadyRef = useRef(false);
  const currentClusterIdRef = useRef('');
  const pendingTemplateSelectionRef = useRef<{
    startOffset: number;
    endOffset: number;
  } | null>(null);
  const pluginSchemaCacheRef = useRef<Record<string, Record<string, any>>>({});
  const pluginListCacheRef = useRef<Record<string, SyncPluginFactoryInfo[]>>(
    {},
  );
  const templateCacheRef = useRef<Record<string, string>>({});
  const enumCatalogCacheRef = useRef<Record<string, PluginEnumCatalogMap>>({});
  const loadPluginEnumCatalogRef = useRef<
    (clusterId: number) => Promise<PluginEnumCatalogMap>
  >(async () => ({}));
  const ensurePluginSchemaRef = useRef<
    (
      pluginType: SyncPluginType,
      factoryIdentifier: string,
    ) => Promise<Record<string, any>>
  >(async () => ({}));
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [tree, setTree] = useState<SyncTaskTreeNode[]>([]);
  const [keyword, setKeyword] = useState('');
  const [jobIdLookupInput, setJobIdLookupInput] = useState('');
  const [selectedNodeId, setSelectedNodeId] = useState<number | null>(null);
  const [selectedFolderId, setSelectedFolderId] = useState<number | null>(null);
  const [editor, setEditor] = useState<EditorState>(EMPTY_EDITOR);
  const [dagResult, setDagResult] = useState<SyncDagResult | null>(null);
  const [dagError, setDagError] = useState<UserFacingErrorState | null>(null);
  const [dagOpen, setDagOpen] = useState(false);
  const [validationOpen, setValidationOpen] = useState(false);
  const [validationTitle, setValidationTitle] = useState('');
  const [validationResult, setValidationResult] =
    useState<SyncValidateResult | null>(null);
  const selectedScheduleNode = useMemo(
    () => (editor.id ? findTreeNode(tree, editor.id) : null),
    [editor.id, tree],
  );

  const [versions, setVersions] = useState<SyncTaskVersion[]>([]);
  const [globalVariables, setGlobalVariables] = useState<SyncGlobalVariable[]>(
    [],
  );
  const [versionTotal, setVersionTotal] = useState(0);
  const [globalVariableTotal, setGlobalVariableTotal] = useState(0);
  const [versionPage, setVersionPage] = useState(1);
  const [globalVariablePage, setGlobalVariablePage] = useState(1);
  const [rightSidebarTab, setRightSidebarTab] =
    useState<RightSidebarTab>('settings');
  const [scheduleDialogOpen, setScheduleDialogOpen] = useState(false);
  const [scheduleDraft, setScheduleDraft] = useState<TaskScheduleValue>(() =>
    extractTaskScheduleValue({}),
  );
  const [bottomConsoleTab, setBottomConsoleTab] =
    useState<BottomConsoleTab>('jobs');
  const [versionPreview, setVersionPreview] = useState<SyncTaskVersion | null>(
    null,
  );
  const [compareVersion, setCompareVersion] = useState<SyncTaskVersion | null>(
    null,
  );
  const [jobs, setJobs] = useState<SyncJobInstance[]>([]);
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const [jobLogs, setJobLogs] = useState<SyncJobLogsResult | null>(null);
  const [expandedJobLogs, setExpandedJobLogs] =
    useState<SyncJobLogsResult | null>(null);
  const [jobLogsOffset, setJobLogsOffset] = useState('');
  const [expandedJobLogsOffset, setExpandedJobLogsOffset] = useState('');
  const jobLogsOffsetRef = useRef('');
  const expandedJobLogsOffsetRef = useRef('');
  const jobLogChunkSizeRef = useRef(LOG_CHUNK_BASE_BYTES);
  const expandedJobLogChunkSizeRef = useRef(EXPANDED_LOG_CHUNK_BASE_BYTES);
  const jobLogsAbortRef = useRef<AbortController | null>(null);
  const expandedJobLogsAbortRef = useRef<AbortController | null>(null);
  const jobLogsRequestVersionRef = useRef(0);
  const [logsLoading, setLogsLoading] = useState(false);
  const [expandedLogsLoading, setExpandedLogsLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [actionPending, setActionPending] = useState<PendingActionKind | null>(
    null,
  );
  const [jobScriptOpen, setJobScriptOpen] = useState(false);
  const [jobScriptTarget, setJobScriptTarget] =
    useState<SyncJobInstance | null>(null);
  const [previewRunDialog, setPreviewRunDialog] =
    useState<PreviewRunDialogState>({
      open: false,
      rowLimit: String(
        Math.min(
          Math.max(
            Number(toObject(EMPTY_EDITOR.definition).preview_row_limit) || 100,
            1,
          ),
          10000,
        ),
      ),
      timeoutMinutes: String(
        Math.min(
          Math.max(
            Number(toObject(EMPTY_EDITOR.definition).preview_timeout_minutes) ||
              10,
            1,
          ),
          24 * 60,
        ),
      ),
    });
  const [previewSnapshot, setPreviewSnapshot] =
    useState<SyncPreviewSnapshot | null>(null);
  const [previewSnapshotLoading, setPreviewSnapshotLoading] = useState(false);
  const [checkpointSnapshot, setCheckpointSnapshot] =
    useState<SyncCheckpointSnapshot | null>(null);
  const [checkpointLoading, setCheckpointLoading] = useState(false);
  const [checkpointFiles, setCheckpointFiles] = useState<
    RuntimeStorageListItem[]
  >([]);
  const [checkpointFilesLoading, setCheckpointFilesLoading] = useState(false);
  const [checkpointInspectDialogOpen, setCheckpointInspectDialogOpen] =
    useState(false);
  const [checkpointInspectDialogLoading, setCheckpointInspectDialogLoading] =
    useState<string | null>(null);
  const [checkpointInspectDialogResult, setCheckpointInspectDialogResult] =
    useState<RuntimeStorageCheckpointInspectResult | null>(null);
  const [recoverSourceId, setRecoverSourceId] = useState<string>('');
  const [previewDatasetName, setPreviewDatasetName] = useState('');
  const [previewPage, setPreviewPage] = useState(1);
  const [openTabs, setOpenTabs] = useState<OpenFileTab[]>([]);
  const [editorDrafts, setEditorDrafts] = useState<
    Record<number, EditorDraftState>
  >({});
  const [expandedFolderIds, setExpandedFolderIds] = useState<number[]>([]);
  const [customVariableRows, setCustomVariableRows] = useState<VariableRow[]>(
    [],
  );
  // 自定义变量弹窗开闭与正在编辑项状态
  // Custom variable dialog open state and current editing item
  const [customVariableDialogOpen, setCustomVariableDialogOpen] =
    useState(false);
  const [editingCustomVariable, setEditingCustomVariable] =
    useState<VariableRow | null>(null);
  const [jobMetricsDialogOpen, setJobMetricsDialogOpen] = useState(false);
  const [metricsDialogJob, setMetricsDialogJob] =
    useState<SyncJobInstance | null>(null);
  const [logsDialogOpen, setLogsDialogOpen] = useState(false);
  const [logFilterMode, setLogFilterMode] = useState<LogFilterMode>('all');
  const [logSearchTerm, setLogSearchTerm] = useState('');
  const [pluginPanelLoading, setPluginPanelLoading] = useState(false);
  const [pluginTemplateLoadingText, setPluginTemplateLoadingText] = useState<
    string | null
  >(null);
  const [pluginTemplatePendingType, setPluginTemplatePendingType] =
    useState<SyncPluginType | null>(null);
  const [pluginFactories, setPluginFactories] = useState<
    Record<'source' | 'transform' | 'sink', SyncPluginFactoryInfo[]>
  >({
    source: [],
    transform: [],
    sink: [],
  });
  const [treeMenu, setTreeMenu] = useState<TreeContextMenuState>({
    open: false,
    x: 0,
    y: 0,
    kind: 'root',
    node: null,
  });
  const [treeDialog, setTreeDialog] = useState<TreeDialogState>({
    open: false,
    action: null,
    targetNode: null,
    name: '',
    targetParentId: null,
  });
  const [globalVariableDialogOpen, setGlobalVariableDialogOpen] =
    useState(false);
  const [editingGlobalVariable, setEditingGlobalVariable] =
    useState<SyncGlobalVariable | null>(null);
  const restoredWorkspaceTabsRef = useRef<PersistedWorkspaceTabs | null>(null);
  const customVariableRowsRef = useRef<VariableRow[]>([]);
  const tabStripRef = useRef<HTMLDivElement | null>(null);
  const tabButtonRefs = useRef<Record<number, HTMLButtonElement | null>>({});

  if (
    restoredWorkspaceTabsRef.current === null &&
    typeof window !== 'undefined'
  ) {
    try {
      const raw = window.localStorage.getItem(WORKSPACE_TABS_STORAGE_KEY);
      if (raw) {
        const parsed = JSON.parse(raw) as Partial<PersistedWorkspaceTabs>;
        restoredWorkspaceTabsRef.current = {
          openTabIds: Array.isArray(parsed.openTabIds)
            ? parsed.openTabIds
                .map((value) => Number(value))
                .filter((value) => Number.isInteger(value) && value > 0)
            : [],
          activeTabId:
            typeof parsed.activeTabId === 'number' &&
            Number.isInteger(parsed.activeTabId) &&
            parsed.activeTabId > 0
              ? parsed.activeTabId
              : null,
        };
      } else {
        restoredWorkspaceTabsRef.current = {openTabIds: [], activeTabId: null};
      }
    } catch {
      restoredWorkspaceTabsRef.current = {openTabIds: [], activeTabId: null};
    }
  }

  const filteredTree = useMemo(
    () => filterTree(tree, keyword),
    [tree, keyword],
  );
  const detectedVariables = useMemo(
    () => detectVariables(editor.content),
    [editor.content],
  );
  const previewJob = useMemo(
    () => jobs.find((job) => job.run_type === 'preview') || null,
    [jobs],
  );
  const runJobs = useMemo(
    () =>
      jobs.filter(
        (job) =>
          job.run_type === 'run' ||
          job.run_type === 'recover' ||
          job.run_type === 'schedule',
      ),
    [jobs],
  );
  const recoverableRunJobs = useMemo(
    () => runJobs.filter((job) => canRecoverFromJob(job)),
    [runJobs],
  );
  const preferredRecoverSourceId = useMemo(() => {
    const selected = Number(recoverSourceId);
    if (Number.isInteger(selected) && selected > 0) {
      return selected;
    }
    return recoverableRunJobs[0]?.id ?? null;
  }, [recoverSourceId, recoverableRunJobs]);
  const selectedJob = useMemo(
    () => jobs.find((job) => job.id === selectedJobId) || jobs[0] || null,
    [jobs, selectedJobId],
  );
  const previewDatasets = useMemo(
    () =>
      previewSnapshot
        ? previewSnapshot.tables.map((table) => ({
            name: table.table_path,
            columns: table.columns,
            rows:
              table.table_path === previewSnapshot.selected_table?.table_path
                ? previewSnapshot.selected_table.rows || []
                : [],
            total: table.row_count,
            page: 1,
            page_size: Math.max(
              previewSnapshot.selected_table?.rows?.length || 0,
              1,
            ),
          }))
        : extractPreviewDatasets(previewJob?.result_preview),
    [previewJob, previewSnapshot],
  );
  const selectedPreviewDataset = useMemo(() => {
    if (previewDatasets.length === 0) {
      return null;
    }
    return (
      previewDatasets.find((dataset) => dataset.name === previewDatasetName) ||
      previewDatasets[0]
    );
  }, [previewDatasetName, previewDatasets]);
  const activeJobs = useMemo(
    () =>
      jobs.filter((job) =>
        isJobLifecycleActive(getDisplayJobLifecycleStatus(job)),
      ),
    [jobs],
  );
  const hasActivePreview = activeJobs.some((job) => job.run_type === 'preview');
  const hasActiveRun = activeJobs.some(
    (job) =>
      job.run_type === 'run' ||
      job.run_type === 'recover' ||
      job.run_type === 'schedule',
  );
  const dagNodes = useMemo(
    () => (Array.isArray(dagResult?.nodes) ? dagResult?.nodes : []),
    [dagResult],
  );
  const dagEdges = useMemo(
    () => (Array.isArray(dagResult?.edges) ? dagResult?.edges : []),
    [dagResult],
  );
  const dagWarnings = useMemo(
    () => (Array.isArray(dagResult?.warnings) ? dagResult.warnings : []),
    [dagResult],
  );
  const dagWebUIJob = dagResult?.webui_job ?? null;
  const monacoTheme = resolvedTheme === 'light' ? 'vs' : 'vs-dark';
  const executionMode = useMemo(
    () => getExecutionMode(editor.definition),
    [editor.definition],
  );
  const fileCount = useMemo(
    () => flattenTree(tree).filter((node) => node.node_type === 'file').length,
    [tree],
  );
  const moveTargetOptions = useMemo(
    () => listMoveTargets(tree, treeDialog.targetNode, t('rootFolder')),
    [tree, treeDialog.targetNode],
  );

  useEffect(() => {
    customVariableRowsRef.current = customVariableRows;
  }, [customVariableRows]);

  useEffect(() => {
    currentClusterIdRef.current = editor.clusterId;
  }, [editor.clusterId]);

  const markEditorDraft = useCallback(
    (
      taskId: number,
      nextEditor: EditorState,
      nextRows: VariableRow[],
      dirty: boolean,
    ) => {
      setEditorDrafts((current) => {
        const existing = current[taskId];
        const baselineEditor = dirty
          ? existing?.baselineEditor || nextEditor
          : nextEditor;
        const baselineRows = dirty
          ? existing?.baselineCustomVariableRows || nextRows
          : nextRows;
        const computedDirty = dirty
          ? isEditorDraftDirty(
              nextEditor,
              nextRows,
              baselineEditor,
              baselineRows,
            )
          : false;
        return {
          ...current,
          [taskId]: {
            editor: nextEditor,
            customVariableRows: nextRows,
            dirty: computedDirty,
            baselineEditor,
            baselineCustomVariableRows: baselineRows,
          },
        };
      });
    },
    [],
  );

  const applyDraftOrLoadedState = useCallback(
    (
      taskId: number,
      fallbackEditor: EditorState,
      fallbackRows: VariableRow[],
    ) => {
      const draft = editorDrafts[taskId];
      if (draft) {
        setEditor(draft.editor);
        setCustomVariableRows(draft.customVariableRows);
        return true;
      }
      setEditor(fallbackEditor);
      setCustomVariableRows(fallbackRows);
      return false;
    },
    [editorDrafts],
  );

  const syncOpenTabs = useCallback((task: Pick<SyncTask, 'id' | 'name'>) => {
    setOpenTabs((current) => {
      const next = current.filter((tab) => tab.id !== task.id);
      return [...next, {id: task.id, name: task.name || `#${task.id}`}];
    });
  }, []);

  const loadWorkspace = useCallback(
    async (preferredFileId?: number | null) => {
      setLoading(true);
      try {
        const [clusterData, treeData] = await Promise.all([
          services.cluster.getClusters({current: 1, size: 100}),
          services.sync.getTree(),
        ]);
        const items = treeData.items || [];
        setClusters(clusterData.clusters || []);
        setTree(items);

        const allFiles = flattenTree(items).filter(
          (node) => node.node_type === 'file',
        );
        const restoredTabs = (
          restoredWorkspaceTabsRef.current?.openTabIds || []
        )
          .map((id) => allFiles.find((node) => node.id === id))
          .filter((node): node is SyncTaskTreeNode => Boolean(node))
          .map((node) => ({id: node.id, name: node.name || `#${node.id}`}));
        const restoredActiveId =
          restoredWorkspaceTabsRef.current?.activeTabId ?? null;
        const nextSelected =
          preferredFileId &&
          allFiles.find((node) => node.id === preferredFileId)?.id
            ? preferredFileId
            : restoredActiveId &&
                allFiles.find((node) => node.id === restoredActiveId)?.id
              ? restoredActiveId
              : null;

        setOpenTabs(restoredTabs);
        setSelectedNodeId(nextSelected);
        if (nextSelected) {
          const treeTask =
            allFiles.find((node) => node.id === nextSelected) || null;
          applyDraftOrLoadedState(
            nextSelected,
            extractEditorStateFromTreeNode(treeTask),
            extractVariableRowsFromDefinition(treeTask?.definition || {}),
          );
          setJobs([]);
          setSelectedJobId(null);
          setJobLogs(null);
          setExpandedJobLogs(null);
          setCheckpointSnapshot(null);
          if (treeTask) {
            syncOpenTabs(treeTask);
          }
          const task = await services.sync.getTask(nextSelected);
          const loadedEditor = extractEditorState(task);
          const loadedRows = extractVariableRowsFromDefinition(
            task.definition || {},
          );
          const usedDraft = applyDraftOrLoadedState(
            nextSelected,
            loadedEditor,
            loadedRows,
          );
          if (!usedDraft) {
            markEditorDraft(nextSelected, loadedEditor, loadedRows, false);
          }
          syncOpenTabs(task);
        } else {
          setEditor(EMPTY_EDITOR);
          setCustomVariableRows(extractVariableRowsFromDefinition({}));
        }
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('loadStudioFailed'),
        );
      } finally {
        setLoading(false);
      }
    },
    [syncOpenTabs],
  );

  const loadPluginPanelData = useCallback(
    async (clusterId: number) => {
      const clusterKey = String(clusterId);
      setPluginPanelLoading(true);
      try {
        const fetchRuntime = (pluginType: SyncPluginType) => {
          const cacheKey = `${clusterKey}:${pluginType}`;
          if (pluginListCacheRef.current[cacheKey]) {
            return Promise.resolve(pluginListCacheRef.current[cacheKey]);
          }
          return services.sync
            .listPluginFactories({
              cluster_id: clusterId,
              plugin_type: pluginType,
            })
            .then((result) => {
              const items = result.plugins || [];
              pluginListCacheRef.current[cacheKey] = items;
              return items;
            });
        };
        const [sourceItems, transformItems, sinkItems] = await Promise.all([
          fetchRuntime('source'),
          fetchRuntime('transform'),
          fetchRuntime('sink'),
        ]);
        void loadPluginEnumCatalogRef.current(clusterId).catch((error) => {
          console.warn(
            '[sync] plugin enum preload skipped',
            error instanceof Error ? error.message : error,
          );
        });
        setPluginFactories({
          source: sourceItems || [],
          transform: transformItems || [],
          sink: sinkItems || [],
        });
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('loadPluginTemplatesFailed'),
        );
        setPluginFactories({source: [], transform: [], sink: []});
      } finally {
        setPluginPanelLoading(false);
      }
    },
    [t],
  );

  useEffect(() => {
    if (executionMode !== 'cluster' || !editor.clusterId) {
      setPluginFactories({source: [], transform: [], sink: []});
      return;
    }
    void loadPluginPanelData(Number(editor.clusterId));
  }, [editor.clusterId, executionMode, loadPluginPanelData]);

  const sourceTemplateItems = useMemo(
    () => buildTemplatePluginItems(pluginFactories.source),
    [pluginFactories.source],
  );
  const sinkTemplateItems = useMemo(
    () => buildTemplatePluginItems(pluginFactories.sink),
    [pluginFactories.sink],
  );
  const transformTemplateItems = useMemo(
    () =>
      (pluginFactories.transform || [])
        .map((item) => ({
          value: item.factory_identifier,
          label: item.factory_identifier,
          origin: item.origin,
        }))
        .sort((left, right) => left.label.localeCompare(right.label)),
    [pluginFactories.transform],
  );

  const ensurePluginSchema = useCallback(
    async (pluginType: SyncPluginType, factoryIdentifier: string) => {
      if (!editor.clusterId) {
        return {};
      }
      const cacheKey = `${editor.clusterId}:${pluginType}:${factoryIdentifier}`;
      if (pluginSchemaCacheRef.current[cacheKey]) {
        return pluginSchemaCacheRef.current[cacheKey];
      }
      const result = await services.sync.getPluginOptions({
        cluster_id: Number(editor.clusterId),
        plugin_type: pluginType,
        factory_identifier: factoryIdentifier,
        include_supplement: true,
      });
      const mapped = Object.fromEntries(
        (result.options || []).map((item) => [item.key, item]),
      );
      pluginSchemaCacheRef.current[cacheKey] = mapped;
      return mapped;
    },
    [editor.clusterId],
  );

  useEffect(() => {
    ensurePluginSchemaRef.current = ensurePluginSchema;
  }, [ensurePluginSchema]);

  const loadPluginEnumCatalog = useCallback(async (clusterId: number) => {
    const cacheKey = String(clusterId);
    if (enumCatalogCacheRef.current[cacheKey]) {
      return enumCatalogCacheRef.current[cacheKey];
    }
    const result = await services.sync.listPluginEnumCatalog({
      cluster_id: clusterId,
      include_supplement: true,
    });
    const mapped: PluginEnumCatalogMap = {};
    for (const plugin of result.plugins || []) {
      const pluginType = plugin.plugin_type;
      if (!mapped[pluginType]) {
        mapped[pluginType] = {};
      }
      mapped[pluginType]![plugin.factory_identifier] = Object.fromEntries(
        (plugin.options || []).map((item) => [item.key, item]),
      );
    }
    if ((result.env_options || []).length > 0) {
      mapped.env = {
        __env__: Object.fromEntries(
          (result.env_options || []).map((item) => [item.key, item]),
        ),
      } as any;
    }
    enumCatalogCacheRef.current[cacheKey] = mapped;
    return mapped;
  }, []);

  useEffect(() => {
    loadPluginEnumCatalogRef.current = loadPluginEnumCatalog;
  }, [loadPluginEnumCatalog]);

  const loadJobs = useCallback(async (taskId: number | null) => {
    if (!taskId) {
      setJobs([]);
      setSelectedJobId(null);
      setJobLogs(null);
      setExpandedJobLogs(null);
      return;
    }
    try {
      const data = await services.sync.listJobs({
        current: 1,
        size: 50,
        task_id: taskId,
      });
      const items = data.items || [];
      setJobs(items);
      setSelectedJobId((prev) => {
        if (prev && items.some((item) => item.id === prev)) {
          return prev;
        }
        return items[0]?.id || null;
      });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadRunsFailed'));
    }
  }, []);

  const loadPreviewSnapshot = useCallback(
    async (jobId: number | null, tablePath?: string, silent = false) => {
      if (!jobId) {
        setPreviewSnapshot(null);
        return;
      }
      if (!silent) {
        setPreviewSnapshotLoading(true);
      }
      try {
        const snapshot = await services.sync.getPreviewSnapshot(jobId, {
          table_path: tablePath || undefined,
        });
        setPreviewSnapshot(snapshot);
      } catch (error) {
        if (!silent) {
          console.warn(
            error instanceof Error ? error.message : t('noPreviewDataFallback'),
          );
        }
      } finally {
        if (!silent) {
          setPreviewSnapshotLoading(false);
        }
      }
    },
    [t],
  );

  const loadCheckpointSnapshot = useCallback(
    async (jobId: number | null, silent = false) => {
      if (!jobId) {
        setCheckpointSnapshot(null);
        return;
      }
      if (!silent) {
        setCheckpointLoading(true);
      }
      try {
        const snapshot = await services.sync.getJobCheckpoint(jobId, {
          limit: 20,
        });
        setCheckpointSnapshot(snapshot);
      } catch (error) {
        if (!silent) {
          toast.error(
            error instanceof Error ? error.message : t('loadCheckpointFailed'),
          );
        }
        setCheckpointSnapshot(null);
      } finally {
        if (!silent) {
          setCheckpointLoading(false);
        }
      }
    },
    [t],
  );

  const loadCheckpointFiles = useCallback(
    async (job: SyncJobInstance | null, silent = false) => {
      const clusterId = getSyncJobClusterId(job);
      const engineJobId = (
        job?.engine_job_id ||
        job?.platform_job_id ||
        ''
      ).trim();
      if (
        !job ||
        submitSpecExecutionMode(job.submit_spec) === 'local' ||
        !clusterId ||
        !engineJobId
      ) {
        setCheckpointFiles([]);
        return;
      }
      if (!silent) {
        setCheckpointFilesLoading(true);
      }
      try {
        const runtimeStorageResult =
          await services.cluster.getRuntimeStorageSafe(clusterId);
        const namespace = runtimeStorageResult.success
          ? runtimeStorageResult.data?.checkpoint?.namespace?.trim() || ''
          : '';
        if (!namespace) {
          setCheckpointFiles([]);
          return;
        }
        const browsePath = `${namespace.replace(/\/+$/, '')}/${engineJobId}`;
        const listResult = await services.cluster.listRuntimeStorageSafe(
          clusterId,
          'checkpoint',
          {
            path: browsePath,
            recursive: true,
            limit: 200,
          },
        );
        if (!listResult.success || !listResult.data) {
          setCheckpointFiles([]);
          return;
        }
        const items = Array.isArray(listResult.data.items)
          ? listResult.data.items
              .filter((item) => !item.directory && !!item.path)
              .sort((left, right) =>
                (right.modified_at || '').localeCompare(left.modified_at || ''),
              )
          : [];
        setCheckpointFiles(items);
      } finally {
        if (!silent) {
          setCheckpointFilesLoading(false);
        }
      }
    },
    [],
  );

  const handleInspectCheckpointFile = useCallback(
    async (path: string) => {
      const clusterId = getSyncJobClusterId(selectedJob);
      if (!clusterId || !path) {
        toast.error(t('checkpointInspectUnavailable'));
        return;
      }
      const jobConfig = buildCheckpointInspectJobConfig(
        selectedJob,
        editor,
        customVariableRows,
      );
      setCheckpointInspectDialogLoading(path);
      try {
        const result =
          await services.cluster.inspectCheckpointRuntimeStorageSafe(
            clusterId,
            {
              path,
              job_config: jobConfig,
            },
          );
        if (!result.success || !result.data) {
          toast.error(result.error || t('checkpointInspectFailed'));
          return;
        }
        setCheckpointInspectDialogResult(result.data);
        setCheckpointInspectDialogOpen(true);
      } finally {
        setCheckpointInspectDialogLoading(null);
      }
    },
    [customVariableRows, editor, selectedJob, t],
  );

  const loadVersions = useCallback(
    async (taskId: number | null) => {
      if (!taskId) {
        setVersions([]);
        setVersionTotal(0);
        return;
      }
      try {
        const data = await services.sync.listVersions(taskId, {
          current: versionPage,
          size: 10,
        });
        setVersions(data.items || []);
        setVersionTotal(data.total || 0);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('loadVersionsFailed'),
        );
      }
    },
    [versionPage],
  );

  const loadGlobalVariables = useCallback(async () => {
    try {
      const data = await services.sync.listGlobalVariables({
        current: globalVariablePage,
        size: 8,
      });
      setGlobalVariables(data.items || []);
      setGlobalVariableTotal(data.total || 0);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('loadGlobalVariablesFailed'),
      );
    }
  }, [globalVariablePage]);

  useEffect(() => {
    void loadWorkspace();
  }, [loadWorkspace]);

  useEffect(() => {
    void loadGlobalVariables();
  }, [loadGlobalVariables]);

  useEffect(() => {
    const folderIds = new Set(collectFolderIds(tree));
    setExpandedFolderIds((current) =>
      current.filter((id) => folderIds.has(id)),
    );
  }, [tree]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    const payload: PersistedWorkspaceTabs = {
      openTabIds: openTabs.map((tab) => tab.id),
      activeTabId: selectedNodeId,
    };
    window.localStorage.setItem(
      WORKSPACE_TABS_STORAGE_KEY,
      JSON.stringify(payload),
    );
  }, [openTabs, selectedNodeId]);

  useEffect(() => {
    if (!editor.id) {
      return;
    }
    setOpenTabs((current) =>
      current.map((tab) =>
        tab.id === editor.id
          ? {id: tab.id, name: editor.name || tab.name}
          : tab,
      ),
    );
  }, [editor.id, editor.name]);

  useEffect(() => {
    if (!selectedNodeId) {
      return;
    }
    const strip = tabStripRef.current;
    const activeTab = tabButtonRefs.current[selectedNodeId];
    if (!strip || !activeTab) {
      return;
    }
    const stripRect = strip.getBoundingClientRect();
    const tabRect = activeTab.getBoundingClientRect();
    const isOutOfView =
      tabRect.left < stripRect.left || tabRect.right > stripRect.right;
    if (!isOutOfView) {
      return;
    }
    const targetLeft =
      activeTab.offsetLeft - strip.clientWidth / 2 + activeTab.clientWidth / 2;
    strip.scrollTo({
      left: Math.max(0, targetLeft),
      behavior: 'smooth',
    });
  }, [openTabs, selectedNodeId]);

  useEffect(() => {
    if (selectedNodeId) {
      void loadJobs(selectedNodeId);
      void loadVersions(selectedNodeId);
    } else {
      setVersions([]);
      setVersionTotal(0);
    }
  }, [selectedNodeId, loadJobs, loadVersions]);

  useEffect(() => {
    setVersionPage(1);
  }, [selectedNodeId]);

  useEffect(() => {
    if (previewDatasets.length === 0) {
      setPreviewDatasetName('');
      setPreviewPage(1);
      return;
    }
    if (
      !previewDatasets.some((dataset) => dataset.name === previewDatasetName)
    ) {
      setPreviewDatasetName(previewDatasets[0]?.name || '');
      setPreviewPage(1);
    }
  }, [previewDatasetName, previewDatasets]);

  useEffect(() => {
    if (!previewJob) {
      setPreviewSnapshot(null);
      return;
    }
    void loadPreviewSnapshot(previewJob.id, previewDatasetName || undefined);
  }, [previewJob?.id, previewDatasetName, loadPreviewSnapshot]);

  useEffect(() => {
    if (bottomConsoleTab !== 'checkpoint') {
      return;
    }
    void loadCheckpointSnapshot(selectedJobId);
  }, [bottomConsoleTab, selectedJobId, loadCheckpointSnapshot]);

  useEffect(() => {
    if (bottomConsoleTab !== 'checkpoint') {
      return;
    }
    void loadCheckpointFiles(selectedJob);
  }, [bottomConsoleTab, loadCheckpointFiles, selectedJob]);

  // 同步编辑器定义中的自定义变量与类型到状态
  // Sync custom variables and types from editor definition into state
  useEffect(() => {
    const rows = toVariableRows(
      editor.definition?.custom_variables,
      editor.definition?.custom_variable_types,
    );
    setCustomVariableRows(rows);
  }, [selectedNodeId, editor.currentVersion]);

  useEffect(() => {
    jobLogsAbortRef.current?.abort();
    expandedJobLogsAbortRef.current?.abort();
    jobLogsRequestVersionRef.current += 1;
    setJobLogs(null);
    setExpandedJobLogs(null);
    setPreviewSnapshot(null);
    setCheckpointSnapshot(null);
    setJobLogsOffset('');
    setExpandedJobLogsOffset('');
    jobLogsOffsetRef.current = '';
    expandedJobLogsOffsetRef.current = '';
    jobLogChunkSizeRef.current = LOG_CHUNK_BASE_BYTES;
    expandedJobLogChunkSizeRef.current = EXPANDED_LOG_CHUNK_BASE_BYTES;
  }, [selectedJobId, logFilterMode, logSearchTerm]);

  const loadSelectedJobLogs = useCallback(
    async (all = false) => {
      if (!selectedJobId || (bottomConsoleTab !== 'logs' && !all)) {
        return;
      }
      const requestVersion = jobLogsRequestVersionRef.current;
      const abortRef = all ? expandedJobLogsAbortRef : jobLogsAbortRef;
      const currentOffsetRef = all
        ? expandedJobLogsOffsetRef
        : jobLogsOffsetRef;
      const chunkSizeRef = all
        ? expandedJobLogChunkSizeRef
        : jobLogChunkSizeRef;
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      if (all) {
        setExpandedLogsLoading(true);
      } else {
        setLogsLoading(true);
      }
      try {
        const currentOffset = currentOffsetRef.current;
        const result = await services.sync.getJobLogs(selectedJobId, {
          offset: currentOffset || undefined,
          limit_bytes: chunkSizeRef.current,
          keyword: logSearchTerm.trim() || undefined,
          level: logFilterMode === 'all' ? undefined : logFilterMode,
          signal: controller.signal,
        });
        if (
          controller.signal.aborted ||
          jobLogsRequestVersionRef.current !== requestVersion
        ) {
          return;
        }
        const mergedLogs = (previousLogs?: string) =>
          currentOffset
            ? {
                ...result,
                logs: result.logs
                  ? [previousLogs || '', result.logs].filter(Boolean).join('\n')
                  : previousLogs || '',
              }
            : result;
        const nextOffset = result.next_offset || currentOffset;
        currentOffsetRef.current = nextOffset;
        chunkSizeRef.current = nextLogChunkSize(
          chunkSizeRef.current,
          result.logs,
          all ? EXPANDED_LOG_CHUNK_BASE_BYTES : LOG_CHUNK_BASE_BYTES,
          all ? EXPANDED_LOG_CHUNK_MAX_BYTES : LOG_CHUNK_MAX_BYTES,
        );
        if (all) {
          setExpandedJobLogs((previous) => mergedLogs(previous?.logs));
          setExpandedJobLogsOffset(nextOffset);
        } else {
          setJobLogs((previous) => mergedLogs(previous?.logs));
          setJobLogsOffset(nextOffset);
        }
      } catch (error) {
        if (
          error instanceof Error &&
          (error.name === 'CanceledError' || error.name === 'AbortError')
        ) {
          return;
        }
        if (all) {
          setExpandedJobLogs(null);
          setExpandedJobLogsOffset('');
          expandedJobLogsOffsetRef.current = '';
          expandedJobLogChunkSizeRef.current = EXPANDED_LOG_CHUNK_BASE_BYTES;
        } else {
          setJobLogs(null);
          setJobLogsOffset('');
          jobLogsOffsetRef.current = '';
          jobLogChunkSizeRef.current = LOG_CHUNK_BASE_BYTES;
        }
        console.warn(
          error instanceof Error ? error.message : t('loadJobLogsFailed'),
        );
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
        }
        if (all) {
          setExpandedLogsLoading(false);
        } else {
          setLogsLoading(false);
        }
      }
    },
    [bottomConsoleTab, logFilterMode, logSearchTerm, selectedJobId],
  );

  useEffect(() => {
    if (!selectedJobId || bottomConsoleTab !== 'logs') {
      return;
    }
    void loadSelectedJobLogs();
  }, [
    bottomConsoleTab,
    loadSelectedJobLogs,
    selectedJobId,
    logFilterMode,
    logSearchTerm,
  ]);

  useEffect(() => {
    if (!logsDialogOpen || !selectedJobId) {
      expandedJobLogsAbortRef.current?.abort();
      return;
    }
    void loadSelectedJobLogs(true);
  }, [logsDialogOpen, selectedJobId, loadSelectedJobLogs]);

  useEffect(() => {
    if (bottomConsoleTab === 'logs') {
      return;
    }
    jobLogsAbortRef.current?.abort();
  }, [bottomConsoleTab]);

  useEffect(() => {
    return () => {
      jobLogsAbortRef.current?.abort();
      expandedJobLogsAbortRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (activeJobs.length === 0) {
      return;
    }
    const timer = window.setInterval(() => {
      void (async () => {
        try {
          const refreshed = await Promise.all(
            activeJobs.map((job) => services.sync.getJob(job.id)),
          );
          setJobs((current) =>
            current.map(
              (job) => refreshed.find((item) => item.id === job.id) || job,
            ),
          );
        } catch {
          // 忽略瞬时轮询抖动，保持 Studio 可继续操作。
          // Ignore transient polling errors to keep the studio usable.
        }
      })();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [activeJobs]);

  useEffect(() => {
    if (
      !previewJob ||
      (previewJob.status !== 'pending' && previewJob.status !== 'running')
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadPreviewSnapshot(
        previewJob.id,
        previewDatasetName || undefined,
        true,
      );
    }, 1500);
    return () => window.clearInterval(timer);
  }, [
    previewDatasetName,
    previewJob?.id,
    previewJob?.status,
    loadPreviewSnapshot,
  ]);

  useEffect(() => {
    if (!selectedJobId || bottomConsoleTab !== 'logs') {
      return;
    }
    const timer = window.setInterval(() => {
      void loadSelectedJobLogs();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [bottomConsoleTab, loadSelectedJobLogs, selectedJobId]);

  useEffect(() => {
    if (!logsDialogOpen || !selectedJobId) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadSelectedJobLogs(true);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [logsDialogOpen, loadSelectedJobLogs, selectedJobId]);

  useEffect(() => {
    if (
      bottomConsoleTab !== 'checkpoint' ||
      !selectedJobId ||
      !selectedJob ||
      (selectedJob.status !== 'pending' && selectedJob.status !== 'running')
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadCheckpointSnapshot(selectedJobId, true);
      void loadCheckpointFiles(selectedJob, true);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [
    bottomConsoleTab,
    loadCheckpointFiles,
    loadCheckpointSnapshot,
    selectedJob,
    selectedJobId,
  ]);

  useEffect(() => {
    if (!treeMenu.open) {
      return;
    }
    const handleClose = () => {
      setTreeMenu((prev) => ({...prev, open: false}));
    };
    window.addEventListener('click', handleClose);
    window.addEventListener('scroll', handleClose, true);
    return () => {
      window.removeEventListener('click', handleClose);
      window.removeEventListener('scroll', handleClose, true);
    };
  }, [treeMenu.open]);

  const updateEditor = <K extends keyof EditorState>(
    key: K,
    value: EditorState[K],
  ) => {
    setEditor((prev) => {
      const next = {...prev, [key]: value};
      if (next.id) {
        markEditorDraft(next.id, next, customVariableRowsRef.current, true);
      }
      return next;
    });
  };

  const insertPluginTemplate = useCallback(
    async (pluginType: SyncPluginType, factoryIdentifier: string) => {
      if (!editor.clusterId) {
        toast.error(t('selectClusterFirst'));
        return;
      }
      const clusterId = Number(editor.clusterId);
      const cacheKey = `${clusterId}:${pluginType}:${factoryIdentifier}`;
      setPluginTemplatePendingType(pluginType);
      setPluginTemplateLoadingText(factoryIdentifier);
      const loadingToastId = toast.loading(
        t('generatingPluginTemplate', {plugin: factoryIdentifier}),
      );
      try {
        await ensurePluginSchema(pluginType, factoryIdentifier);
        const template =
          templateCacheRef.current[cacheKey] ||
          (
            await services.sync.renderPluginTemplate({
              cluster_id: clusterId,
              plugin_type: pluginType,
              factory_identifier: factoryIdentifier,
              include_comments: false,
              include_advanced: false,
              include_supplement: true,
            })
          ).template;
        templateCacheRef.current[cacheKey] = template;
        setEditor((prev) => {
          const existing = prev.content || '';
          const inserted = buildInsertedTemplateContent(
            existing,
            pluginType,
            template,
          );
          pendingTemplateSelectionRef.current = {
            startOffset: inserted.startOffset,
            endOffset: inserted.endOffset,
          };
          const nextEditor = {...prev, content: inserted.nextContent};
          if (nextEditor.id) {
            markEditorDraft(
              nextEditor.id,
              nextEditor,
              customVariableRowsRef.current,
              true,
            );
          }
          return nextEditor;
        });
        toast.dismiss(loadingToastId);
      } catch (error) {
        toast.dismiss(loadingToastId);
        toast.error(
          error instanceof Error
            ? error.message
            : t('insertPluginTemplateFailed'),
        );
      } finally {
        setPluginTemplatePendingType(null);
        setPluginTemplateLoadingText(null);
      }
    },
    [editor.clusterId, ensurePluginSchema, markEditorDraft, t],
  );

  useEffect(() => {
    if (!pendingTemplateSelectionRef.current || !editorInstanceRef.current) {
      return;
    }
    const model = editorInstanceRef.current.getModel?.();
    const monaco = monacoInstanceRef.current;
    if (!model || !monaco) {
      return;
    }
    const pending = pendingTemplateSelectionRef.current;
    pendingTemplateSelectionRef.current = null;
    const start = model.getPositionAt(pending.startOffset);
    const end = model.getPositionAt(pending.endOffset);
    editorInstanceRef.current.focus?.();
    editorInstanceRef.current.setSelection?.(
      new monaco.Selection(
        start.lineNumber,
        start.column,
        end.lineNumber,
        end.column,
      ),
    );
    editorInstanceRef.current.revealLineNearTop?.(start.lineNumber);
  }, [editor.content]);

  const registerEditorAssistProviders = useCallback((monaco: any) => {
    monacoInstanceRef.current = monaco;
    if (typeof window !== 'undefined') {
      (window as typeof window & {monaco?: any}).monaco = monaco;
    }
    if (!monacoLanguageReadyRef.current) {
      ensureSyncHoconLanguage(monaco);
      monacoLanguageReadyRef.current = true;
    }
    completionDisposableRef.current?.dispose?.();
    hoverDisposableRef.current?.dispose?.();
    if (!enumCompletionCommandRegisteredRef.current) {
      monaco.editor.registerCommand(
        enumCompletionCommandIdRef.current,
        (
          _accessor: unknown,
          payload?: {lineNumber?: number; value?: string},
        ) => {
          const editor = editorInstanceRef.current;
          const model = editor?.getModel?.();
          if (
            !editor ||
            !model ||
            !payload?.lineNumber ||
            payload.value == null
          ) {
            return;
          }
          const lineContent = model.getLineContent(payload.lineNumber);
          const bounds = resolveEnumValueBounds(
            lineContent,
            payload.lineNumber,
          );
          if (!bounds) {
            return;
          }
          const renderedValue = bounds.quoted
            ? payload.value
            : JSON.stringify(payload.value);
          editor.executeEdits?.('sync-enum-completion', [
            {
              range: {
                startLineNumber: payload.lineNumber,
                endLineNumber: payload.lineNumber,
                startColumn: bounds.startColumn,
                endColumn: bounds.endColumn,
              },
              text: renderedValue,
            },
          ]);
        },
      );
      enumCompletionCommandRegisteredRef.current = true;
    }
    completionDisposableRef.current =
      monaco.languages.registerCompletionItemProvider('sync-hocon', {
        triggerCharacters: ['=', ' ', '"'],
        provideCompletionItems: async (
          model: any,
          position: {lineNumber: number; column: number},
        ) => {
          const lineContent = model.getLineContent(position.lineNumber);
          const linePrefix = model
            .getLineContent(position.lineNumber)
            .slice(0, Math.max(position.column - 1, 0));
          const keyMatch = linePrefix.match(
            /^\s*([A-Za-z0-9_.-]+)\s*=\s*(?:"[^"]*)?$/,
          );
          if (!keyMatch) {
            return {suggestions: []};
          }
          const optionKey = keyMatch[1];
          const context = resolveEditorPluginContext(
            model.getValue(),
            position.lineNumber,
          );
          let metadata: any = null;
          const enumCatalog =
            enumCatalogCacheRef.current[currentClusterIdRef.current] || {};
          if (context.pluginType && context.factoryIdentifier) {
            metadata =
              enumCatalog[context.pluginType]?.[context.factoryIdentifier]?.[
                optionKey
              ] || null;
          } else if (
            enumCatalog.env?.__env__?.[optionKey]?.enum_values?.length
          ) {
            metadata = enumCatalog.env.__env__[optionKey];
          }
          if (
            !(metadata?.enum_values || []).length &&
            context.pluginType &&
            context.factoryIdentifier &&
            currentClusterIdRef.current
          ) {
            try {
              const schema = await ensurePluginSchemaRef.current(
                context.pluginType,
                context.factoryIdentifier,
              );
              metadata = schema[optionKey] || null;
            } catch {
              metadata = null;
            }
          }
          if (
            !(metadata?.enum_values || []).length &&
            ENV_OPTION_METADATA[optionKey]?.enumValues
          ) {
            metadata = {
              enum_values: ENV_OPTION_METADATA[optionKey].enumValues || [],
              enum_display_values:
                ENV_OPTION_METADATA[optionKey].enumValues || [],
            };
          }
          const enumItems = resolveEnumSuggestionItems(metadata);
          const enumValues = enumItems.map((item) => item.value);
          if (!enumValues?.length) {
            return {suggestions: []};
          }
          const currentWord = model.getWordUntilPosition(position)?.word || '';
          return {
            suggestions: enumItems.map((item) => ({
              label: item.label,
              detail:
                item.label !== item.value ? `插入值: ${item.value}` : undefined,
              kind: monaco.languages.CompletionItemKind.EnumMember,
              insertText: '',
              filterText: [currentWord, optionKey, item.label, item.value]
                .filter(Boolean)
                .join(' '),
              range: resolveEnumSuggestRange(position),
              command: {
                id: enumCompletionCommandIdRef.current,
                title: 'Apply enum completion',
                arguments: [
                  {
                    lineNumber: position.lineNumber,
                    value: item.value,
                  },
                ],
              },
            })),
          };
        },
      });
    hoverDisposableRef.current = monaco.languages.registerHoverProvider(
      'sync-hocon',
      {
        provideHover: async (
          model: any,
          position: {lineNumber: number; column: number},
        ) => {
          const lineContent = model.getLineContent(position.lineNumber);
          const optionKeyInfo = resolveOptionKeyFromLine(
            lineContent,
            position.column,
          );
          if (!optionKeyInfo.key) {
            return null;
          }
          const optionKey = optionKeyInfo.key;
          let metadata: any = ENV_OPTION_METADATA[optionKey] || null;
          const context = resolveEditorPluginContext(
            model.getValue(),
            position.lineNumber,
          );
          const enumCatalog =
            enumCatalogCacheRef.current[currentClusterIdRef.current] || {};
          if (!metadata && enumCatalog.env?.__env__?.[optionKey]) {
            metadata = enumCatalog.env.__env__[optionKey];
          }
          if (!metadata && context.pluginType && context.factoryIdentifier) {
            metadata =
              enumCatalog[context.pluginType]?.[context.factoryIdentifier]?.[
                optionKey
              ] || null;
          }
          if (
            !metadata &&
            context.pluginType &&
            context.factoryIdentifier &&
            currentClusterIdRef.current
          ) {
            try {
              const schema = await ensurePluginSchemaRef.current(
                context.pluginType,
                context.factoryIdentifier,
              );
              metadata = schema[optionKey] || null;
            } catch {
              metadata = null;
            }
          }
          if (!metadata) {
            return null;
          }
          const lines = [`**${optionKey}**`];
          if (metadata.description) {
            lines.push('', String(metadata.description));
          }
          if (metadata.default_value !== undefined) {
            lines.push(
              '',
              `默认值：\`${formatMetadataValue(metadata.default_value)}\``,
            );
          }
          if (metadata.required_mode) {
            lines.push('', `必填模式：\`${String(metadata.required_mode)}\``);
          }
          if (metadata.enum_values?.length || metadata.enumValues?.length) {
            const values = resolveEnumSuggestionItems({
              enum_values: metadata.enum_values || metadata.enumValues || [],
              enum_display_values:
                metadata.enum_display_values ||
                metadata.enumDisplayValues ||
                [],
            });
            lines.push(
              '',
              `枚举值：\`${values
                .map((item) =>
                  item.label !== item.value
                    ? `${item.label} => ${item.value}`
                    : item.value,
                )
                .join('`, `')}\``,
            );
          }
          return {
            range: new monaco.Range(
              position.lineNumber,
              optionKeyInfo.startColumn,
              position.lineNumber,
              optionKeyInfo.endColumn,
            ),
            contents: [{value: lines.join('\n')}],
          };
        },
      },
    );
  }, []);

  const handleEditorBeforeMount = useCallback(
    (monaco: any) => {
      registerEditorAssistProviders(monaco);
    },
    [registerEditorAssistProviders],
  );

  const handleEditorMount = useCallback(
    (instance: any, monaco: any) => {
      editorInstanceRef.current = instance;
      registerEditorAssistProviders(monaco);
      contentChangeDisposableRef.current?.dispose?.();
      cursorPositionChangeDisposableRef.current?.dispose?.();
      cursorSelectionChangeDisposableRef.current?.dispose?.();
      contentChangeDisposableRef.current = instance.onDidType?.(
        (typedText: string) => {
          const position = instance.getPosition?.();
          const model = instance.getModel?.();
          if (!position || !model) {
            return;
          }
          const linePrefix = model
            .getLineContent(position.lineNumber)
            .slice(0, Math.max(position.column - 1, 0));
          const inValueRegion = /^\s*[A-Za-z0-9_.-]+\s*=\s*(?:"[^"]*)?$/.test(
            linePrefix,
          );
          if (!inValueRegion) {
            return;
          }
          const shouldTrigger =
            ['=', ' ', '"'].includes(typedText) ||
            /^[A-Za-z0-9_.-]$/.test(typedText);
          if (!shouldTrigger) {
            return;
          }
          if (typedText === ' ' && !/=\s+$/.test(linePrefix)) {
            return;
          }
          setTimeout(() => {
            instance.trigger?.(
              'sync-plugin-completion',
              'editor.action.triggerSuggest',
              {},
            );
          }, 0);
        },
      );
      cursorPositionChangeDisposableRef.current =
        instance.onDidChangeCursorPosition?.((event: any) => {
          const model = instance.getModel?.();
          const position = event?.position;
          if (!model || !position) {
            return;
          }
          const lineContent = model.getLineContent(position.lineNumber);
          if (!isCursorInsideValueRegion(lineContent, position.column)) {
            return;
          }
          const triggerKey = `${position.lineNumber}:${position.column}:${lineContent}`;
          if (lastSuggestTriggerRef.current === triggerKey) {
            return;
          }
          lastSuggestTriggerRef.current = triggerKey;
          setTimeout(() => {
            instance.trigger?.(
              'sync-plugin-cursor',
              'editor.action.triggerSuggest',
              {},
            );
          }, 0);
        });
      cursorSelectionChangeDisposableRef.current =
        instance.onDidChangeCursorSelection?.((event: any) => {
          const model = instance.getModel?.();
          const position = event?.selection?.getPosition?.();
          if (!model || !position) {
            return;
          }
          const lineContent = model.getLineContent(position.lineNumber);
          if (!isCursorInsideValueRegion(lineContent, position.column)) {
            return;
          }
          const triggerKey = `selection:${position.lineNumber}:${position.column}:${lineContent}`;
          if (lastSuggestTriggerRef.current === triggerKey) {
            return;
          }
          lastSuggestTriggerRef.current = triggerKey;
          setTimeout(() => {
            instance.trigger?.(
              'sync-plugin-selection',
              'editor.action.triggerSuggest',
              {},
            );
          }, 0);
        });
    },
    [registerEditorAssistProviders],
  );

  useEffect(() => {
    if (!monacoFromHook) {
      return;
    }
    registerEditorAssistProviders(monacoFromHook);
  }, [monacoFromHook, registerEditorAssistProviders]);

  useEffect(() => {
    return () => {
      completionDisposableRef.current?.dispose?.();
      hoverDisposableRef.current?.dispose?.();
      contentChangeDisposableRef.current?.dispose?.();
      cursorPositionChangeDisposableRef.current?.dispose?.();
      cursorSelectionChangeDisposableRef.current?.dispose?.();
    };
  }, []);

  const buildTaskPayload = useCallback(
    (): CreateSyncTaskRequest => ({
      parent_id: editor.parentId,
      node_type: 'file',
      name: editor.name.trim(),
      description: editor.description.trim(),
      cluster_id: editor.clusterId ? Number(editor.clusterId) : 0,
      content_format: 'hocon',
      content: editor.content,
      job_name: editor.name.trim(),
      definition: {
        ...editor.definition,
        custom_variables: fromVariableRows(customVariableRows),
        custom_variable_types: fromVariableTypes(customVariableRows),
        execution_mode: getExecutionMode(editor.definition),
        preview_mode: 'source',
        preview_output_format: 'hocon',
        preview_row_limit:
          Number(toObject(editor.definition).preview_row_limit) > 0
            ? Math.min(
                Number(toObject(editor.definition).preview_row_limit),
                10000,
              )
            : 100,
        preview_timeout_minutes:
          Number(toObject(editor.definition).preview_timeout_minutes) > 0
            ? Math.min(
                Number(toObject(editor.definition).preview_timeout_minutes),
                24 * 60,
              )
            : 10,
        preview_http_sink: {
          url:
            typeof toObject(toObject(editor.definition).preview_http_sink)
              .url === 'string'
              ? String(
                  toObject(toObject(editor.definition).preview_http_sink).url,
                )
              : resolveDefaultPreviewHTTPSinkURL(),
          array_mode: false,
        },
      },
    }),
    [customVariableRows, editor],
  );

  const persistCurrentFile = useCallback(
    async (publishAfterSave = false) => {
      if (!editor.name.trim()) {
        toast.error(t('fileNameRequired'));
        return null;
      }
      if (!editor.content.trim()) {
        toast.error(t('fileContentRequired'));
        return null;
      }
      const customVariableError = validateCustomVariableRows(
        customVariableRowsRef.current,
        t,
      );
      if (customVariableError) {
        toast.error(customVariableError);
        return null;
      }
      setSaving(true);
      try {
        const payload = buildTaskPayload();
        let task: SyncTask;
        if (editor.id) {
          task = await services.sync.updateTask(editor.id, payload);
        } else {
          task = await services.sync.createTask(payload);
        }
        const isNewTask = !editor.id;
        if (publishAfterSave) {
          await services.sync.publishTask(task.id, {
            comment: 'publish from data sync studio',
          });
          task = await services.sync.getTask(task.id);
        }
        const savedEditor = extractEditorState(task);
        const savedRows = extractVariableRowsFromDefinition(
          task.definition || {},
        );
        setEditor(savedEditor);
        setCustomVariableRows(savedRows);
        markEditorDraft(task.id, savedEditor, savedRows, false);
        syncOpenTabs(task);
        setSelectedNodeId(task.id);
        if (isNewTask) {
          await loadWorkspace(task.id);
        } else {
          setTree((current) => patchTreeNode(current, task));
        }
        return task;
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('saveFileFailed'),
        );
        return null;
      } finally {
        setSaving(false);
      }
    },
    [buildTaskPayload, editor, loadWorkspace, markEditorDraft, syncOpenTabs],
  );

  const openTreeDialog = useCallback(
    (
      action: TreeDialogState['action'],
      targetNode: SyncTaskTreeNode | null,
      initialName = '',
    ) => {
      const defaultParentId =
        action === 'move'
          ? targetNode?.parent_id || null
          : action === 'create-folder' || action === 'create-file'
            ? targetNode?.node_type === 'folder'
              ? targetNode.id
              : targetNode?.parent_id || null
            : null;
      setTreeMenu((prev) => ({...prev, open: false}));
      setTreeDialog({
        open: true,
        action,
        targetNode,
        name: initialName,
        targetParentId: defaultParentId,
      });
    },
    [],
  );

  const openTreeContextMenu = useCallback(
    (
      event: MouseEvent,
      kind: TreeContextMenuState['kind'],
      node: SyncTaskTreeNode | null,
    ) => {
      event.preventDefault();
      event.stopPropagation();
      setTreeMenu({
        open: true,
        x: event.clientX,
        y: event.clientY,
        kind,
        node,
      });
    },
    [],
  );

  const handleTreeDialogSubmit = async () => {
    const name = treeDialog.name.trim();
    if (treeDialog.action !== 'move') {
      const nameError = validateWorkspaceName(name, t);
      if (treeDialog.action !== 'delete' && nameError) {
        toast.error(nameError);
        return;
      }
    }
    try {
      if (treeDialog.action === 'create-folder') {
        const parentId = treeDialog.targetParentId || null;
        if (hasDuplicateWorkspaceName(tree, parentId, name)) {
          toast.error(t('duplicateWorkspaceName'));
          return;
        }
        await services.sync.createTask({
          parent_id: parentId || undefined,
          node_type: 'folder',
          name,
          content_format: 'hocon',
        });
        toast.success(t('folderCreated'));
        await loadWorkspace(selectedNodeId);
      } else if (treeDialog.action === 'create-file') {
        const parentId = treeDialog.targetParentId || null;
        if (!parentId) {
          toast.error(t('rootFileCreationBlocked'));
          return;
        }
        if (hasDuplicateWorkspaceName(tree, parentId, name)) {
          toast.error(t('duplicateWorkspaceName'));
          return;
        }
        const task = await services.sync.createTask({
          parent_id: parentId,
          node_type: 'file',
          name,
          cluster_id: editor.clusterId ? Number(editor.clusterId) : 0,
          content_format: 'hocon',
          content: buildDefaultContent('hocon'),
          definition: {},
        });
        toast.success(t('fileCreated'));
        syncOpenTabs(task);
        await loadWorkspace(task.id);
      } else if (treeDialog.action === 'rename' && treeDialog.targetNode) {
        const siblingParentId =
          treeDialog.targetNode.parent_id == null
            ? null
            : treeDialog.targetNode.parent_id;
        if (
          hasDuplicateWorkspaceName(
            tree,
            siblingParentId,
            name,
            treeDialog.targetNode.id,
          )
        ) {
          toast.error(t('duplicateWorkspaceName'));
          return;
        }
        const current = await services.sync.getTask(treeDialog.targetNode.id);
        await services.sync.updateTask(treeDialog.targetNode.id, {
          parent_id: current.parent_id,
          node_type: current.node_type,
          name,
          description: current.description,
          cluster_id: current.cluster_id,
          content_format: current.content_format,
          content: current.content,
          definition: current.definition,
        });
        toast.success(t('nameUpdated'));
        await loadWorkspace(treeDialog.targetNode.id);
      } else if (treeDialog.action === 'move' && treeDialog.targetNode) {
        const current = await services.sync.getTask(treeDialog.targetNode.id);
        await services.sync.updateTask(treeDialog.targetNode.id, {
          parent_id: treeDialog.targetParentId || undefined,
          node_type: current.node_type,
          name: current.name,
          description: current.description,
          cluster_id: current.cluster_id,
          content_format: current.content_format,
          content: current.content,
          definition: current.definition,
        });
        toast.success(t('moveCompleted'));
        await loadWorkspace(treeDialog.targetNode.id);
      } else if (treeDialog.action === 'delete' && treeDialog.targetNode) {
        if (name !== treeDialog.targetNode.name) {
          toast.error(t('deleteNameMismatch'));
          return;
        }
        await services.sync.deleteTask(treeDialog.targetNode.id);
        setEditorDrafts((current) => {
          const next = {...current};
          delete next[treeDialog.targetNode!.id];
          return next;
        });
        setOpenTabs((current) =>
          current.filter((tab) => tab.id !== treeDialog.targetNode?.id),
        );
        if (selectedNodeId === treeDialog.targetNode.id) {
          setSelectedNodeId(null);
          setEditor(EMPTY_EDITOR);
          setJobs([]);
          setSelectedJobId(null);
        }
        toast.success(
          treeDialog.targetNode.node_type === 'folder'
            ? t('folderDeleted')
            : t('fileDeleted'),
        );
        await loadWorkspace();
      }
      setTreeDialog({
        open: false,
        action: null,
        targetNode: null,
        name: '',
        targetParentId: null,
      });
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('operationFailed'),
      );
    }
  };

  const handleCopyFile = async (node: SyncTaskTreeNode | null) => {
    if (!node || node.node_type !== 'file') {
      return;
    }
    try {
      const current = await services.sync.getTask(node.id);
      const parentId = current.parent_id || undefined;
      const copiedName = buildCopiedWorkspaceName(
        tree,
        current.parent_id ?? null,
        current.name,
      );
      const copiedTask = await services.sync.createTask({
        parent_id: parentId,
        node_type: 'file',
        name: copiedName,
        description: current.description,
        cluster_id: current.cluster_id,
        content_format: current.content_format,
        content: current.content,
        definition: current.definition,
      });
      toast.success(t('fileCopied'));
      syncOpenTabs(copiedTask);
      await loadWorkspace(copiedTask.id);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('copyFileFailed'));
    }
  };

  const handleSelectNode = async (node: SyncTaskTreeNode) => {
    if (node.node_type === 'folder') {
      setSelectedFolderId(node.id);
      setExpandedFolderIds((current) =>
        current.includes(node.id)
          ? current.filter((id) => id !== node.id)
          : [...current, node.id],
      );
      return;
    }
    setSelectedNodeId(node.id);
    setSelectedFolderId(node.parent_id || null);
    applyDraftOrLoadedState(
      node.id,
      extractEditorStateFromTreeNode(node),
      extractVariableRowsFromDefinition(node.definition || {}),
    );
    syncOpenTabs(node);
    setDagResult(null);
    setJobs([]);
    setSelectedJobId(null);
    setJobLogs(null);
    setExpandedJobLogs(null);
    setCheckpointSnapshot(null);
    setBottomConsoleTab('logs');
    try {
      const task = await services.sync.getTask(node.id);
      const loadedEditor = extractEditorState(task);
      const loadedRows = extractVariableRowsFromDefinition(
        task.definition || {},
      );
      const usedDraft = applyDraftOrLoadedState(
        node.id,
        loadedEditor,
        loadedRows,
      );
      if (!usedDraft) {
        markEditorDraft(node.id, loadedEditor, loadedRows, false);
      }
      syncOpenTabs(task);
      await loadJobs(node.id);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadFileFailed'));
    }
  };

  const handleSelectTab = async (taskId: number) => {
    setSelectedNodeId(taskId);
    const treeTask = findTreeNode(tree, taskId);
    if (treeTask && treeTask.node_type === 'file') {
      applyDraftOrLoadedState(
        taskId,
        extractEditorStateFromTreeNode(treeTask),
        extractVariableRowsFromDefinition(treeTask.definition || {}),
      );
      setSelectedFolderId(treeTask.parent_id || null);
      setJobs([]);
      setSelectedJobId(null);
      setJobLogs(null);
      setExpandedJobLogs(null);
      setCheckpointSnapshot(null);
    }
    try {
      const task = await services.sync.getTask(taskId);
      const loadedEditor = extractEditorState(task);
      const loadedRows = extractVariableRowsFromDefinition(
        task.definition || {},
      );
      const usedDraft = applyDraftOrLoadedState(
        taskId,
        loadedEditor,
        loadedRows,
      );
      if (!usedDraft) {
        markEditorDraft(taskId, loadedEditor, loadedRows, false);
      }
      setSelectedFolderId(task.parent_id || null);
      await loadJobs(taskId);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadFileFailed'));
    }
  };

  const handleCloseTab = async (taskId: number) => {
    setOpenTabs((current) => current.filter((tab) => tab.id !== taskId));
    if (selectedNodeId !== taskId) {
      return;
    }
    const remaining = openTabs.filter((tab) => tab.id !== taskId);
    const nextTab = remaining[remaining.length - 1];
    if (nextTab) {
      await handleSelectTab(nextTab.id);
    } else {
      setSelectedNodeId(null);
      setEditor(EMPTY_EDITOR);
      setCustomVariableRows(extractVariableRowsFromDefinition({}));
      setJobs([]);
      setSelectedJobId(null);
      setJobLogs(null);
      setExpandedJobLogs(null);
      setCheckpointSnapshot(null);
    }
  };

  const handleSave = async () => {
    const task = await persistCurrentFile(true);
    if (task) {
      await loadVersions(task.id);
      toast.success(t('savedNewVersion'));
    }
  };

  const runPreflightValidation = async (
    taskId: number,
    actionLabel: string,
    draft?: ReturnType<typeof buildTaskPayload>,
  ) => {
    const result = await services.sync.validateTask(
      taskId,
      draft ? {draft} : {},
    );
    if (!result.valid) {
      setValidationTitle(t('validateConfigTitle'));
      setValidationResult(result);
      setValidationOpen(true);
      toast.error(t('preflightValidationFailed', {action: actionLabel}));
      return false;
    }
    return true;
  };

  const ensureDraftActionContext = useCallback(
    (actionLabel: string) => {
      if (!editor.id) {
        toast.error(t('saveBeforeAction', {action: actionLabel}));
        return null;
      }
      const customVariableError = validateCustomVariableRows(
        customVariableRowsRef.current,
        t,
      );
      if (customVariableError) {
        toast.error(customVariableError);
        return null;
      }
      return {taskId: editor.id, draft: buildTaskPayload()};
    },
    [buildTaskPayload, editor.id, t],
  );

  const handleBuildDag = async () => {
    const actionContext = ensureDraftActionContext(t('dagActionLabel'));
    if (!actionContext) {
      return;
    }
    setActionPending('dag');
    try {
      const passed = await runPreflightValidation(
        actionContext.taskId,
        t('dagActionLabel'),
        actionContext.draft,
      );
      if (!passed) {
        return;
      }
      const result = await services.sync.buildDag(actionContext.taskId, {
        draft: actionContext.draft,
      });
      setDagResult(result);
      setDagError(null);
      setDagOpen(true);
      toast.success(t('dagGenerated'));
    } catch (error) {
      setDagResult(null);
      setDagError(
        formatSyncUserFacingError(error, t('dagParseFailedTitle'), t),
      );
      setDagOpen(true);
      toast.error(error instanceof Error ? error.message : t('dagBuildFailed'));
    } finally {
      setActionPending((current) => (current === 'dag' ? null : current));
    }
  };

  const handleValidateConfig = async () => {
    const actionContext = ensureDraftActionContext(t('validateConfigTitle'));
    if (!actionContext) {
      return;
    }
    try {
      const result = await services.sync.validateTask(actionContext.taskId, {
        draft: actionContext.draft,
      });
      setValidationTitle(t('validateConfigTitle'));
      setValidationResult(result);
      setValidationOpen(true);
      toast.success(
        result.valid ? t('validatePassed') : t('validateCompleted'),
      );
    } catch (error) {
      const uiError = formatSyncUserFacingError(
        error,
        t('validateFailedTitle'),
        t,
      );
      setValidationTitle(uiError.title);
      setValidationResult({
        valid: false,
        errors: [uiError.description],
        warnings: [],
        summary: uiError.title,
      });
      setValidationOpen(true);
      toast.error(uiError.description);
    }
  };

  const handleTestConnections = async () => {
    const actionContext = ensureDraftActionContext(t('testConnections'));
    if (!actionContext) {
      return;
    }
    setActionPending('test_connections');
    try {
      const result = await services.sync.testConnections(actionContext.taskId, {
        draft: actionContext.draft,
      });
      setValidationTitle(t('testConnections'));
      setValidationResult(result);
      setValidationOpen(true);
      toast.success(
        result.valid ? t('connectionsPassed') : t('connectionsCompleted'),
      );
    } catch (error) {
      const uiError = formatSyncUserFacingError(
        error,
        t('testConnectionsFailedTitle'),
        t,
      );
      setValidationTitle(uiError.title);
      setValidationResult({
        valid: false,
        errors: [uiError.description],
        warnings: [],
        summary: uiError.title,
      });
      setValidationOpen(true);
      toast.error(uiError.description);
    } finally {
      setActionPending((current) =>
        current === 'test_connections' ? null : current,
      );
    }
  };

  const handlePreview = async () => {
    if (hasActiveRun || hasActivePreview) {
      toast.error(t('waitForActiveRun'));
      return;
    }
    const currentLimit = Number(toObject(editor.definition).preview_row_limit);
    setPreviewRunDialog({
      open: true,
      rowLimit: String(currentLimit > 0 ? Math.min(currentLimit, 10000) : 100),
      timeoutMinutes: String(
        Number(toObject(editor.definition).preview_timeout_minutes) > 0
          ? Math.min(
              Number(toObject(editor.definition).preview_timeout_minutes),
              24 * 60,
            )
          : 10,
      ),
    });
  };

  const handleConfirmPreview = async () => {
    const parsedRowLimit = Number(previewRunDialog.rowLimit);
    const normalizedRowLimit =
      Number.isFinite(parsedRowLimit) && parsedRowLimit > 0
        ? Math.min(Math.floor(parsedRowLimit), 10000)
        : 100;
    const parsedTimeoutMinutes = Number(previewRunDialog.timeoutMinutes);
    const normalizedTimeoutMinutes =
      Number.isFinite(parsedTimeoutMinutes) && parsedTimeoutMinutes > 0
        ? Math.min(Math.floor(parsedTimeoutMinutes), 24 * 60)
        : 10;
    const nextDefinition = {
      ...editor.definition,
      preview_row_limit: normalizedRowLimit,
      preview_timeout_minutes: normalizedTimeoutMinutes,
    };
    setEditor((prev) => ({...prev, definition: nextDefinition}));
    setPreviewRunDialog((current) => ({...current, open: false}));
    if (!editor.id) {
      toast.error(t('saveBeforeAction', {action: t('previewActionLabel')}));
      return;
    }
    const baseDraft = buildTaskPayload();
    const draft = {
      ...baseDraft,
      definition: {
        ...toObject(baseDraft.definition),
        ...nextDefinition,
      },
    };
    setActionPending('preview');
    try {
      const job = await services.sync.previewTask(editor.id, {
        row_limit: normalizedRowLimit,
        timeout_minutes: normalizedTimeoutMinutes,
        draft,
      });
      await loadJobs(editor.id);
      setSelectedJobId(job.id);
      setBottomConsoleTab('preview');
      toast.success(t('previewSubmitted'));
    } catch (error) {
      const uiError = formatSyncUserFacingError(error, t('previewFailed'), t);
      toast.error(uiError.description);
    } finally {
      setActionPending((current) => (current === 'preview' ? null : current));
    }
  };

  const handleRun = async (
    mode: 'run' | 'recover',
    sourceJobId?: number | null,
  ) => {
    if (hasActiveRun || hasActivePreview) {
      toast.error(t('waitForActiveRun'));
      return;
    }
    const actionLabel =
      mode === 'recover' ? t('recoverActionLabel') : t('runActionLabel');
    const actionContext = ensureDraftActionContext(actionLabel);
    if (!actionContext) {
      return;
    }
    try {
      const resolvedRecoverSourceId =
        mode === 'recover'
          ? (sourceJobId ?? preferredRecoverSourceId ?? null)
          : null;
      if (mode === 'recover' && !resolvedRecoverSourceId) {
        throw new Error(t('noRecoverSource'));
      }
      if (mode === 'recover') {
        setActionPending('recover');
      }
      const job =
        mode === 'recover'
          ? await services.sync.recoverJob(Number(resolvedRecoverSourceId), {
              draft: actionContext.draft,
            })
          : await services.sync.submitTask(actionContext.taskId, {
              draft: actionContext.draft,
            });
      await loadJobs(actionContext.taskId);
      setSelectedJobId(job.id);
      if (mode === 'recover') {
        setRecoverSourceId('');
      }
      setBottomConsoleTab('jobs');
      toast.success(
        mode === 'recover' ? t('recoverSubmitted') : t('runSubmitted'),
      );
    } catch (error) {
      const uiError = formatSyncUserFacingError(error, t('runFailed'), t);
      toast.error(uiError.description);
    } finally {
      setActionPending((current) => (current === 'recover' ? null : current));
    }
  };

  const handleCancelJob = async (jobId: number, stopWithSavepoint = false) => {
    try {
      await services.sync.cancelJob(jobId, {
        stop_with_savepoint: stopWithSavepoint,
      });
      await loadJobs(editor.id || null);
      toast.success(
        stopWithSavepoint ? t('savepointStopTriggered') : t('taskStopped'),
      );
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('cancelTaskFailed'),
      );
    }
  };

  const handleStopActiveJob = async (mode: 'normal' | 'savepoint') => {
    const activeJob =
      selectedJob &&
      isJobLifecycleActive(getDisplayJobLifecycleStatus(selectedJob))
        ? selectedJob
        : activeJobs[0];
    if (!activeJob) {
      toast.error(t('noActiveJob'));
      return;
    }
    await handleCancelJob(activeJob.id, mode === 'savepoint');
  };

  const handleRecoverFromHistory = (jobId: number) => {
    void (async () => {
      setActionPending('recover');
      try {
        const job = await services.sync.recoverJob(jobId);
        if (editor.id) {
          await loadJobs(editor.id);
        }
        setSelectedJobId(job.id);
        setBottomConsoleTab('jobs');
        toast.success(t('recoverSubmitted'));
      } catch (error) {
        const uiError = formatSyncUserFacingError(
          error,
          t('recoverActionLabel'),
          t,
        );
        toast.error(uiError.description);
      } finally {
        setActionPending((current) => (current === 'recover' ? null : current));
      }
    })();
  };

  const handleExecutionModeChange = (value: ExecutionMode) => {
    updateEditor('definition', {
      ...editor.definition,
      execution_mode: value,
    });
  };

  const openScheduleDialog = () => {
    setScheduleDraft(extractTaskScheduleValue(editor.definition || {}));
    setScheduleDialogOpen(true);
  };

  const handleScheduleDraftChange = (value: TaskScheduleValue) => {
    setScheduleDraft(value);
  };

  const handleConfirmScheduleDialog = () => {
    handleScheduleChange(scheduleDraft);
    setScheduleDialogOpen(false);
  };

  const handleScheduleChange = (value: TaskScheduleValue) => {
    updateEditor('definition', {
      ...editor.definition,
      schedule: {
        enabled: value.enabled,
        cron_expr: value.cron_expr,
        timezone: value.timezone,
      },
    });
  };

  // 同步自定义变量到编辑器定义与草稿中
  // Sync custom variables to editor definition and draft
  const syncCustomVariablesToEditor = useCallback(
    (rows: VariableRow[]) => {
      setCustomVariableRows(rows);
      customVariableRowsRef.current = rows;
      const nextDefinition = {
        ...editor.definition,
        custom_variables: fromVariableRows(rows),
        custom_variable_types: fromVariableTypes(rows),
      };
      const nextEditor = {...editor, definition: nextDefinition};
      setEditor(nextEditor);
      if (nextEditor.id) {
        markEditorDraft(nextEditor.id, nextEditor, rows, true);
      }
    },
    [editor, markEditorDraft],
  );

  // 打开新建自定义变量弹窗
  // Open create custom variable dialog
  const handleOpenCreateCustomVariable = useCallback(() => {
    setEditingCustomVariable(null);
    setCustomVariableDialogOpen(true);
  }, []);

  // 打开编辑自定义变量弹窗
  // Open edit custom variable dialog
  const handleOpenEditCustomVariable = useCallback((item: VariableRow) => {
    setEditingCustomVariable(item);
    setCustomVariableDialogOpen(true);
  }, []);

  // 保存自定义变量（新建或更新）
  // Save custom variable (create or update)
  const handleSaveCustomVariable = useCallback(
    (payload: {
      key: string;
      value: string;
      type: 'string' | 'secret';
      description?: string;
    }) => {
      let nextRows: VariableRow[];
      if (editingCustomVariable) {
        nextRows = customVariableRows.map((row) =>
          row.id === editingCustomVariable.id
            ? {
                ...row,
                key: payload.key.trim(),
                value: payload.value,
                type: payload.type,
                description: payload.description,
              }
            : row,
        );
        toast.success(t('customVariableUpdated'));
      } else {
        const newRow: VariableRow = {
          id: `custom-var-${Date.now()}`,
          key: payload.key.trim(),
          value: payload.value,
          type: payload.type,
          description: payload.description,
        };
        nextRows = [...customVariableRows, newRow];
        toast.success(t('customVariableCreated'));
      }
      syncCustomVariablesToEditor(nextRows);
      setCustomVariableDialogOpen(false);
      setEditingCustomVariable(null);
    },
    [customVariableRows, editingCustomVariable, syncCustomVariablesToEditor, t],
  );

  // 删除自定义变量
  // Delete custom variable
  const handleDeleteCustomVariable = useCallback(
    (rowId: string) => {
      const nextRows = customVariableRows.filter((row) => row.id !== rowId);
      syncCustomVariablesToEditor(nextRows);
      toast.success(t('customVariableDeleted'));
      if (editingCustomVariable?.id === rowId) {
        setEditingCustomVariable(null);
        setCustomVariableDialogOpen(false);
      }
    },
    [customVariableRows, editingCustomVariable, syncCustomVariablesToEditor, t],
  );

  const handleSaveGlobalVariable = async (
    item: SyncGlobalVariable | null,
    payload: CreateSyncGlobalVariableRequest,
  ) => {
    try {
      if (isReservedBuiltinVariableKey(payload.key)) {
        throw new Error(
          t('reservedBuiltinVariableKey', {key: `{{${payload.key.trim()}}}`}),
        );
      }
      if (item) {
        await services.sync.updateGlobalVariable(item.id, payload);
        toast.success(t('globalVariableUpdated'));
      } else {
        await services.sync.createGlobalVariable(payload);
        toast.success(t('globalVariableCreated'));
      }
      setGlobalVariableDialogOpen(false);
      setEditingGlobalVariable(null);
      await loadGlobalVariables();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('saveGlobalVariableFailed'),
      );
      throw error;
    }
  };

  const handleOpenCreateGlobalVariable = useCallback(() => {
    setEditingGlobalVariable(null);
    setGlobalVariableDialogOpen(true);
  }, []);

  const handleOpenEditGlobalVariable = useCallback(
    (item: SyncGlobalVariable) => {
      setEditingGlobalVariable(item);
      setGlobalVariableDialogOpen(true);
    },
    [],
  );

  const handleCopyVariableReference = useCallback(
    (key: string) => {
      void copyToClipboard(
        `{{${key}}}`,
        t('referenceCopied', {key: `{{${key}}}`}),
      );
    },
    [t],
  );

  const handleDeleteGlobalVariable = async (id: number) => {
    try {
      await services.sync.deleteGlobalVariable(id);
      toast.success(t('globalVariableDeleted'));
      if (editingGlobalVariable?.id === id) {
        setEditingGlobalVariable(null);
        setGlobalVariableDialogOpen(false);
      }
      if (globalVariables.length === 1 && globalVariablePage > 1) {
        setGlobalVariablePage((current) => Math.max(1, current - 1));
        return;
      }
      await loadGlobalVariables();
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('deleteGlobalVariableFailed'),
      );
    }
  };

  const copyToClipboard = async (value: string, successText: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(successText);
    } catch {
      toast.error(t('copyFailed'));
    }
  };

  const handleRollbackVersion = async (versionId: number) => {
    if (!editor.id) {
      return;
    }
    try {
      const task = await services.sync.rollbackVersion(editor.id, versionId);
      const rolledEditor = extractEditorState(task);
      const rolledRows = extractVariableRowsFromDefinition(
        task.definition || {},
      );
      setEditor(rolledEditor);
      setCustomVariableRows(rolledRows);
      markEditorDraft(task.id, rolledEditor, rolledRows, false);
      setTree((current) => patchTreeNode(current, task));
      await loadVersions(task.id);
      toast.success(t('rollbackVersionSuccess'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('rollbackVersionFailed'),
      );
    }
  };

  const handleDeleteVersion = async (versionId: number) => {
    if (!editor.id) {
      return;
    }
    try {
      await services.sync.deleteVersion(editor.id, versionId);
      if (versions.length === 1 && versionPage > 1) {
        setVersionPage((current) => Math.max(1, current - 1));
        return;
      }
      await loadVersions(editor.id);
      toast.success(t('versionDeleted'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('deleteVersionFailed'),
      );
    }
  };

  const handleLocateJobId = async () => {
    const trimmed = jobIdLookupInput.trim();
    if (!trimmed) {
      return;
    }
    try {
      const result = await services.sync.listJobs({
        current: 1,
        size: 1,
        platform_job_id: trimmed,
      });
      const job = result.items?.[0] || null;
      if (!job) {
        toast.error(t('jobIdNotFound'));
        return;
      }
      const targetNode = findTreeNode(tree, job.task_id);
      if (!targetNode || targetNode.node_type !== 'file') {
        toast.error(t('loadFileFailed'));
        return;
      }
      const ancestorFolderIds: number[] = [];
      let cursor = targetNode.parent_id
        ? findTreeNode(tree, targetNode.parent_id)
        : null;
      while (cursor) {
        if (cursor.node_type === 'folder') {
          ancestorFolderIds.unshift(cursor.id);
        }
        cursor = cursor.parent_id ? findTreeNode(tree, cursor.parent_id) : null;
      }
      setExpandedFolderIds((current) =>
        Array.from(new Set([...current, ...ancestorFolderIds])),
      );
      await handleSelectNode(targetNode);
      setSelectedJobId(job.id);
      setBottomConsoleTab('jobs');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadRunsFailed'));
    }
  };

  return (
    <div className='-mx-2 flex h-[calc(100vh-96px)] min-h-[780px] flex-col gap-2 bg-background/10 lg:-mx-3'>
      <Card className='gap-0 border-border/60 bg-background/85 py-0 shadow-sm'>
        <CardContent className='flex h-14 items-center justify-between gap-3 px-4 py-2'>
          <div className='flex min-w-0 items-center gap-3'>
            <FolderTree className='size-4 shrink-0 text-primary' />
            <div className='relative w-[240px]'>
              <Search className='absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground' />
              <Input
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                className='h-9 border-border/60 bg-background pl-9 text-sm'
                placeholder={t('searchWorkspace')}
              />
            </div>
            <div className='flex items-center gap-2'>
              <div className='relative w-[180px]'>
                <Search className='absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground' />
                <Input
                  value={jobIdLookupInput}
                  onChange={(event) =>
                    setJobIdLookupInput(event.target.value.replace(/\D/g, ''))
                  }
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      void handleLocateJobId();
                    }
                  }}
                  className='h-9 border-border/60 bg-background pl-9 text-sm'
                  placeholder={t('jobId')}
                  inputMode='numeric'
                />
              </div>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='h-9 px-3'
                onClick={() => void handleLocateJobId()}
              >
                <Search className='mr-1.5 size-4' />
                {t('search')}
              </Button>
            </div>
          </div>

          <div className='flex flex-wrap items-center justify-end gap-2'>
            <Badge variant='outline' className='h-9 rounded-md px-3 text-sm'>
              HOCON
            </Badge>
            <Button
              size='sm'
              className='h-9 px-3'
              variant='outline'
              onClick={handleSave}
              disabled={saving || !editor.name.trim()}
            >
              <Save className='mr-1.5 size-4' />
              {t('save')}
            </Button>
            <Button
              size='sm'
              className='h-9 px-3'
              variant='outline'
              onClick={handleTestConnections}
              disabled={saving || !editor.name.trim() || actionPending !== null}
            >
              {actionPending === 'test_connections' ? (
                <Loader2 className='mr-1.5 size-4 animate-spin' />
              ) : (
                <Database className='mr-1.5 size-4' />
              )}
              {actionPending === 'test_connections'
                ? t('testingConnections')
                : t('testConnections')}
            </Button>
            <Button
              size='sm'
              className='h-9 px-3'
              variant='outline'
              onClick={handleBuildDag}
              disabled={saving || !editor.name.trim() || actionPending !== null}
            >
              {actionPending === 'dag' ? (
                <Loader2 className='mr-1.5 size-4 animate-spin' />
              ) : (
                <GitBranch className='mr-1.5 size-4' />
              )}
              {actionPending === 'dag' ? t('buildingDag') : 'DAG'}
            </Button>
            <Button
              size='sm'
              className='h-9 px-3'
              variant='outline'
              onClick={handlePreview}
              disabled={
                saving ||
                hasActiveRun ||
                hasActivePreview ||
                actionPending !== null
              }
            >
              {actionPending === 'preview' ? (
                <Loader2 className='mr-1.5 size-4 animate-spin' />
              ) : (
                <Bug className='mr-1.5 size-4' />
              )}
              {actionPending === 'preview'
                ? t('preparingPreview')
                : t('preview')}
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size='sm'
                  className='h-9 px-3'
                  disabled={saving || hasActiveRun || hasActivePreview}
                >
                  <Play className='mr-1.5 size-4' />
                  {t('run')}
                  <ChevronDown className='ml-1.5 size-4' />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end'>
                <DropdownMenuItem onClick={() => void handleRun('run')}>
                  {t('run')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={
                    executionMode === 'local' ||
                    hasActiveRun ||
                    hasActivePreview ||
                    actionPending !== null ||
                    preferredRecoverSourceId === null
                  }
                  onClick={() => void handleRun('recover')}
                >
                  {t('savepointRecover')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size='sm'
                  className='h-9 px-3'
                  variant='outline'
                  disabled={activeJobs.length === 0}
                >
                  <Square className='mr-1.5 size-4' />
                  {t('stop')}
                  <ChevronDown className='ml-1.5 size-4' />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end'>
                <DropdownMenuItem
                  onClick={() => void handleStopActiveJob('normal')}
                >
                  {t('normalStop')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={executionMode === 'local'}
                  onClick={() => void handleStopActiveJob('savepoint')}
                >
                  {t('savepointStop')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </CardContent>
      </Card>

      {actionPending ? (
        <div className='flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-sm text-primary shadow-sm'>
          <Loader2 className='size-4 animate-spin' />
          <span>{getPendingActionLabel(t, actionPending)}</span>
          <span className='text-muted-foreground'>
            {t('actionPendingHint')}
          </span>
        </div>
      ) : null}

      <div className='grid min-h-0 flex-1 grid-cols-[220px_minmax(0,1fr)_304px] grid-rows-[minmax(0,1fr)_260px] gap-2'>
        <Card className='row-start-1 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm'>
          <CardContent className='flex h-full min-h-0 flex-col p-0'>
            <div className='flex items-center justify-between border-b border-border/50 px-3 py-2'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <Folder className='size-4 text-primary' />
                {t('resources')}
              </div>
              <div className='flex items-center gap-1'>
                <Badge variant='outline' className='rounded-sm'>
                  {fileCount}
                </Badge>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size='icon'
                      variant='ghost'
                      className='size-7'
                      aria-label={t('newFolder')}
                      onClick={() =>
                        openTreeDialog(
                          'create-folder',
                          selectedFolderId
                            ? findTreeNode(tree, selectedFolderId)
                            : null,
                        )
                      }
                    >
                      <FolderPlus className='size-4' />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('newFolder')}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size='icon'
                      variant='ghost'
                      className='size-7'
                      aria-label={t('newFile')}
                      onClick={() => {
                        const folderNode = selectedFolderId
                          ? findTreeNode(tree, selectedFolderId)
                          : null;
                        if (!folderNode) {
                          toast.error(t('selectFolderBeforeCreateFile'));
                          return;
                        }
                        openTreeDialog('create-file', folderNode);
                      }}
                    >
                      <FilePlus2 className='size-4' />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('newFile')}</TooltipContent>
                </Tooltip>
              </div>
            </div>
            <ScrollArea
              className='min-h-0 flex-1'
              onContextMenu={(event) =>
                openTreeContextMenu(event, 'root', null)
              }
            >
              <div className='px-2 py-2'>
                {loading ? (
                  <div className='p-3 text-sm text-muted-foreground'>
                    {t('loading')}
                  </div>
                ) : filteredTree.length === 0 ? (
                  <div className='p-3 text-sm text-muted-foreground'>
                    {t('emptyWorkspace')}
                  </div>
                ) : (
                  <TreeView
                    nodes={filteredTree}
                    selectedNodeId={selectedNodeId}
                    selectedFolderId={selectedFolderId}
                    expandedFolderIds={expandedFolderIds}
                    onSelect={handleSelectNode}
                    onContextMenu={openTreeContextMenu}
                  />
                )}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>

        <Card className='row-start-1 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm'>
          <CardContent className='flex h-full min-h-0 flex-col p-0'>
            <div
              ref={tabStripRef}
              className='flex min-h-9 items-end gap-0 overflow-x-auto border-b border-border/50 bg-background/80 px-1'
            >
              {openTabs.length > 0 ? (
                openTabs.map((tab) => (
                  <button
                    key={tab.id}
                    ref={(node) => {
                      tabButtonRefs.current[tab.id] = node;
                    }}
                    type='button'
                    className={cn(
                      'group -mb-px flex h-8 items-center gap-1.5 border-b-2 px-3 text-xs transition-colors',
                      selectedNodeId === tab.id
                        ? 'border-primary bg-primary/5 text-foreground'
                        : 'border-transparent text-muted-foreground hover:bg-muted/40 hover:text-foreground',
                    )}
                    onClick={() => void handleSelectTab(tab.id)}
                  >
                    <FileCode2 className='size-3.5' />
                    {editorDrafts[tab.id]?.dirty ? (
                      <span
                        aria-label={t('unsavedDraft')}
                        className='size-2 rounded-full bg-amber-500'
                      />
                    ) : null}
                    <span className='max-w-[180px] truncate'>{tab.name}</span>
                    <span
                      className='rounded px-1 text-[10px] opacity-60 transition hover:bg-muted hover:opacity-100'
                      onClick={(event) => {
                        event.stopPropagation();
                        void handleCloseTab(tab.id);
                      }}
                    >
                      ×
                    </span>
                  </button>
                ))
              ) : (
                <div className='px-3 py-2 text-xs text-muted-foreground'>
                  {t('noOpenFiles')}
                </div>
              )}
            </div>
            <div className='min-h-0 flex-1'>
              <MonacoEditor
                height='100%'
                language={
                  editor.contentFormat === 'json' ? 'json' : 'sync-hocon'
                }
                theme={
                  editor.contentFormat === 'json'
                    ? monacoTheme
                    : resolvedTheme === 'light'
                      ? 'sync-hocon-light'
                      : 'sync-hocon-dark'
                }
                value={editor.content}
                beforeMount={handleEditorBeforeMount}
                onMount={handleEditorMount}
                onChange={(value) => updateEditor('content', value || '')}
                options={{
                  minimap: {enabled: true},
                  fontSize: 13,
                  wordWrap: 'on',
                  quickSuggestions: {
                    other: false,
                    comments: false,
                    strings: true,
                  },
                  suggestOnTriggerCharacters: true,
                  wordBasedSuggestions: 'off',
                  automaticLayout: true,
                  scrollBeyondLastLine: false,
                  smoothScrolling: true,
                  tabSize: 2,
                  renderLineHighlight: 'all',
                  padding: {top: 14, bottom: 14},
                }}
              />
            </div>
          </CardContent>
        </Card>

        <StudioSidebarShell
          className='row-span-2'
          rail={
            <>
              <SidebarIconTab
                active={rightSidebarTab === 'settings'}
                icon={<Database className='size-4' />}
                label={t('settings')}
                onClick={() => setRightSidebarTab('settings')}
              />
              <SidebarIconTab
                active={scheduleDialogOpen}
                icon={<Clock3 className='size-4' />}
                label={t('taskSchedule')}
                onClick={openScheduleDialog}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'versions'}
                icon={<GitBranch className='size-4' />}
                label={t('versionManagement')}
                onClick={() => setRightSidebarTab('versions')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'globals'}
                icon={<Globe2 className='size-4' />}
                label={t('globalVariables')}
                onClick={() => setRightSidebarTab('globals')}
              />
            </>
          }
        >
          {rightSidebarTab === 'settings' ? (
            <SettingsSidebarPanel
              executionMode={executionMode}
              clusterId={editor.clusterId}
              clusters={clusters}
              detectedVariables={detectedVariables}
              customVariableRows={customVariableRows}
              onExecutionModeChange={handleExecutionModeChange}
              onClusterChange={(value) =>
                updateEditor('clusterId', value === '__empty__' ? '' : value)
              }
              pluginPanelLoading={pluginPanelLoading}
              pluginTemplatePendingType={pluginTemplatePendingType}
              pluginTemplateLoadingText={pluginTemplateLoadingText}
              sourceTemplateItems={sourceTemplateItems}
              transformTemplateItems={transformTemplateItems}
              sinkTemplateItems={sinkTemplateItems}
              onInsertPluginTemplate={(pluginType, factoryIdentifier) =>
                void insertPluginTemplate(pluginType, factoryIdentifier)
              }
              onOpenCreateCustomVariable={handleOpenCreateCustomVariable}
              onOpenEditCustomVariable={handleOpenEditCustomVariable}
              onDeleteCustomVariable={handleDeleteCustomVariable}
              onCopyCustomVariableReference={handleCopyVariableReference}
              onCopyCustomVariableValue={(value) =>
                void copyToClipboard(value, t('variableValueCopied'))
              }
            />
          ) : rightSidebarTab === 'versions' ? (
            <VersionSidebarPanel
              taskId={editor.id}
              currentVersion={editor.currentVersion}
              versions={versions}
              total={versionTotal}
              page={versionPage}
              pageSize={10}
              onPageChange={setVersionPage}
              onPreview={setVersionPreview}
              onCompare={setCompareVersion}
              onRollback={(versionId) => void handleRollbackVersion(versionId)}
              onDelete={(versionId) => void handleDeleteVersion(versionId)}
            />
          ) : (
            <GlobalVariablesSidebarPanel
              variables={globalVariables}
              total={globalVariableTotal}
              page={globalVariablePage}
              pageSize={8}
              onPageChange={setGlobalVariablePage}
              onOpenCreate={handleOpenCreateGlobalVariable}
              onOpenEdit={handleOpenEditGlobalVariable}
              onDelete={(id) => void handleDeleteGlobalVariable(id)}
              onCopyValue={(value) =>
                void copyToClipboard(value, t('variableValueCopied'))
              }
              onCopyReference={handleCopyVariableReference}
            />
          )}
        </StudioSidebarShell>

        <GlobalVariableDialog
          open={globalVariableDialogOpen}
          onOpenChange={(open) => {
            setGlobalVariableDialogOpen(open);
            if (!open) {
              setEditingGlobalVariable(null);
            }
          }}
          variable={editingGlobalVariable}
          onSave={handleSaveGlobalVariable}
        />

        <CustomVariableDialog
          open={customVariableDialogOpen}
          onOpenChange={(open) => {
            setCustomVariableDialogOpen(open);
            if (!open) {
              setEditingCustomVariable(null);
            }
          }}
          variable={editingCustomVariable}
          existingKeys={customVariableRows.map((item) => item.key)}
          onSave={handleSaveCustomVariable}
        />

        <Dialog
          open={scheduleDialogOpen}
          onOpenChange={(open) => {
            setScheduleDialogOpen(open);
            if (open) {
              setScheduleDraft(
                extractTaskScheduleValue(editor.definition || {}),
              );
            }
          }}
        >
          <DialogContent className='flex h-[90vh] w-[min(96vw,1280px)] max-w-none flex-col overflow-hidden p-0'>
            <DialogHeader className='border-b border-border/60 px-6 py-4'>
              <DialogTitle>{t('taskSchedule')}</DialogTitle>
            </DialogHeader>
            <div className='min-h-0 flex-1 overflow-y-auto px-6 py-4'>
              <TaskScheduleSidebarPanel
                value={scheduleDraft}
                lastTriggeredAt={
                  selectedScheduleNode?.schedule_last_triggered_at
                }
                nextTriggeredAt={
                  selectedScheduleNode?.schedule_next_triggered_at
                }
                onChange={handleScheduleDraftChange}
                className='mx-auto w-full max-w-6xl'
              />
            </div>
            <DialogFooter className='border-t border-border/60 px-6 py-4'>
              <Button
                type='button'
                variant='outline'
                onClick={() => {
                  setScheduleDraft(
                    extractTaskScheduleValue(editor.definition || {}),
                  );
                  setScheduleDialogOpen(false);
                }}
              >
                {t('cancel')}
              </Button>
              <Button type='button' onClick={handleConfirmScheduleDialog}>
                {t('confirm')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Card className='col-span-2 row-start-2 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm'>
          <CardContent className='flex h-full min-h-0 p-0'>
            <div className='flex w-12 shrink-0 flex-col items-center gap-2 border-r border-border/50 bg-muted/10 py-3'>
              <SidebarIconTab
                active={bottomConsoleTab === 'jobs'}
                icon={<ListTree className='size-4' />}
                label={t('jobs')}
                onClick={() => setBottomConsoleTab('jobs')}
              />
              <SidebarIconTab
                active={bottomConsoleTab === 'logs'}
                icon={<SquareTerminal className='size-4' />}
                label={t('logs')}
                onClick={() => setBottomConsoleTab('logs')}
              />
              <SidebarIconTab
                active={bottomConsoleTab === 'preview'}
                icon={<Bug className='size-4' />}
                label={t('preview')}
                onClick={() => setBottomConsoleTab('preview')}
              />
              <SidebarIconTab
                active={bottomConsoleTab === 'checkpoint'}
                icon={<Columns2 className='size-4' />}
                label={t('checkpoint')}
                onClick={() => setBottomConsoleTab('checkpoint')}
              />
            </div>
            <div className='min-h-0 flex-1 p-3'>
              {bottomConsoleTab === 'jobs' ? (
                <JobRunsPanel
                  jobs={jobs}
                  selectedJobId={selectedJobId}
                  onSelectJob={setSelectedJobId}
                  onRecover={handleRecoverFromHistory}
                  onCancel={handleCancelJob}
                  onSavepointStop={(jobId) => void handleCancelJob(jobId, true)}
                  onViewMetrics={(job) => {
                    setMetricsDialogJob(job);
                    setJobMetricsDialogOpen(true);
                  }}
                  onViewScript={(job) => {
                    setJobScriptTarget(job);
                    setJobScriptOpen(true);
                  }}
                  disableRecover={hasActiveRun || hasActivePreview}
                />
              ) : bottomConsoleTab === 'logs' ? (
                <ConsolePanel
                  job={selectedJob}
                  logsResult={jobLogs}
                  loading={logsLoading}
                  filterMode={logFilterMode}
                  onFilterChange={setLogFilterMode}
                  onExpand={() => {
                    setLogsDialogOpen(true);
                  }}
                />
              ) : bottomConsoleTab === 'preview' ? (
                <PreviewWorkspacePanel
                  job={previewJob}
                  previewSnapshot={previewSnapshot}
                  datasets={previewDatasets}
                  selectedDatasetName={selectedPreviewDataset?.name || ''}
                  previewPage={previewPage}
                  loading={
                    actionPending === 'preview' || previewSnapshotLoading
                  }
                  monacoTheme={monacoTheme}
                  onSelectDataset={(name) => {
                    setPreviewDatasetName(name);
                    setPreviewPage(1);
                  }}
                  onChangePage={setPreviewPage}
                />
              ) : (
                <CheckpointWorkspacePanel
                  job={selectedJob}
                  checkpointSnapshot={checkpointSnapshot}
                  loading={checkpointLoading}
                  checkpointFiles={checkpointFiles}
                  checkpointFilesLoading={checkpointFilesLoading}
                  onInspectCheckpointFile={handleInspectCheckpointFile}
                  inspectLoadingPath={checkpointInspectDialogLoading}
                  onRefresh={() => {
                    void loadCheckpointSnapshot(selectedJobId);
                    void loadCheckpointFiles(selectedJob);
                  }}
                />
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={validationOpen} onOpenChange={setValidationOpen}>
        <DialogContent className='w-[94vw] max-w-[94vw] sm:max-w-[1240px]'>
          <DialogHeader>
            <DialogTitle>{validationTitle}</DialogTitle>
            <DialogDescription>
              {validationResult?.summary || t('validationSummaryFallback')}
            </DialogDescription>
          </DialogHeader>
          <ValidationResultPanel result={validationResult} />
        </DialogContent>
      </Dialog>

      <Dialog
        open={previewRunDialog.open}
        onOpenChange={(open) =>
          setPreviewRunDialog((current) => ({...current, open}))
        }
      >
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('previewSettings')}</DialogTitle>
            <DialogDescription>{t('previewRowLimitDesc')}</DialogDescription>
          </DialogHeader>
          <div className='grid gap-3'>
            <div className='grid gap-2'>
              <Label htmlFor='preview-row-limit'>{t('previewRowLimit')}</Label>
              <Input
                id='preview-row-limit'
                type='number'
                min={1}
                max={10000}
                value={previewRunDialog.rowLimit}
                onChange={(event) =>
                  setPreviewRunDialog((current) => ({
                    ...current,
                    rowLimit: event.target.value,
                  }))
                }
              />
              <div className='text-xs text-muted-foreground'>
                {t('previewRowLimitWarning')}
              </div>
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='preview-timeout-minutes'>
                {t('previewTimeoutMinutes')}
              </Label>
              <Input
                id='preview-timeout-minutes'
                type='number'
                min={1}
                max={1440}
                value={previewRunDialog.timeoutMinutes}
                onChange={(event) =>
                  setPreviewRunDialog((current) => ({
                    ...current,
                    timeoutMinutes: event.target.value,
                  }))
                }
              />
              <div className='text-xs text-muted-foreground'>
                {t('previewTimeoutMinutesDesc')}
              </div>
            </div>
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                onClick={() =>
                  setPreviewRunDialog((current) => ({...current, open: false}))
                }
              >
                {t('cancel')}
              </Button>
              <Button onClick={() => void handleConfirmPreview()}>
                {t('startPreview')}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={dagOpen}
        onOpenChange={(open) => {
          setDagOpen(open);
          if (!open) {
            setDagError(null);
          }
        }}
      >
        <DialogContent className='flex h-[88vh] w-[96vw] max-w-[96vw] flex-col overflow-hidden gap-0 p-0 sm:max-w-[1400px]'>
          <DialogHeader className='px-6 pt-6'>
            <DialogTitle>{t('dagPreview')}</DialogTitle>
            <DialogDescription>
              {dagError
                ? t('dagParseErrorDescription')
                : t('dagSummary', {
                    nodes: dagNodes.length,
                    edges: dagEdges.length,
                  })}
            </DialogDescription>
          </DialogHeader>
          <div className='min-h-0 flex-1 overflow-auto px-6 pb-6'>
            <div className='space-y-4'>
              {dagError ? (
                <Card className='border-destructive/30 bg-destructive/5'>
                  <CardHeader className='pb-3'>
                    <CardTitle className='text-sm text-destructive'>
                      {dagError.title}
                    </CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-3 text-sm'>
                    <div>{dagError.description}</div>
                    {dagError.raw ? (
                      <pre className='max-h-[360px] overflow-auto rounded-lg border border-destructive/20 bg-background/80 p-3 text-xs text-muted-foreground'>
                        {dagError.raw}
                      </pre>
                    ) : null}
                  </CardContent>
                </Card>
              ) : null}
              {dagWarnings.length > 0 ? (
                <div className='flex flex-wrap gap-2'>
                  {dagWarnings.map((warning, index) => (
                    <Badge key={`${warning}-${index}`} variant='outline'>
                      {warning}
                    </Badge>
                  ))}
                </div>
              ) : null}
              {!dagError && dagWebUIJob ? (
                <WebUiDagPreview job={dagWebUIJob} />
              ) : !dagError ? (
                <Card>
                  <CardHeader className='pb-3'>
                    <CardTitle className='text-sm'>{t('rawDagJson')}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <pre className='max-h-[560px] overflow-auto whitespace-pre-wrap break-all text-xs text-muted-foreground'>
                      {JSON.stringify(dagResult, null, 2)}
                    </pre>
                  </CardContent>
                </Card>
              ) : null}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(versionPreview)}
        onOpenChange={(open) => {
          if (!open) {
            setVersionPreview(null);
          }
        }}
      >
        <DialogContent className='max-w-5xl'>
          <DialogHeader>
            <DialogTitle>
              {t('versionPreview')}{' '}
              {versionPreview ? `v${versionPreview.version}` : ''}
            </DialogTitle>
          </DialogHeader>
          <pre className='max-h-[70vh] overflow-auto rounded-lg border p-4 text-xs text-muted-foreground'>
            {versionPreview
              ? versionPreview.content_snapshot
              : t('noVersionContent')}
          </pre>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(compareVersion)}
        onOpenChange={(open) => {
          if (!open) {
            setCompareVersion(null);
          }
        }}
      >
        <DialogContent className='flex h-[88vh] w-[96vw] max-w-[96vw] flex-col overflow-hidden p-0 gap-0 sm:max-w-[1320px]'>
          <DialogHeader>
            <DialogTitle className='px-6 pt-6'>
              {t('versionCompare')}{' '}
              {compareVersion ? `v${compareVersion.version}` : ''}
            </DialogTitle>
          </DialogHeader>
          <div className='mx-6 flex flex-wrap items-center justify-between gap-3 rounded-t-lg border border-b-0 border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground'>
            <div className='flex items-center gap-2'>
              <GitCompareArrows className='size-4 text-primary' />
              <Badge variant='outline'>Diff</Badge>
              <Badge variant='outline'>{t('readOnly')}</Badge>
              <Badge variant='outline'>Side by Side</Badge>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <div className='flex items-center gap-2 rounded-md border border-border/50 bg-background/70 px-2 py-1'>
                <LayoutPanelTop className='size-3.5' />
                <span>
                  v{compareVersion?.version || '-'} /{' '}
                  {compareVersion?.name_snapshot || t('historicalVersion')}
                </span>
              </div>
              <div className='flex items-center gap-2 rounded-md border border-border/50 bg-background/70 px-2 py-1'>
                <Columns2 className='size-3.5' />
                <span>
                  {t('currentEditing')} / {editor.name || t('unnamedFile')}
                </span>
              </div>
            </div>
          </div>
          <div className='mx-6 mb-6 min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
            <MonacoDiffEditor
              height='100%'
              theme={monacoTheme}
              language={
                (compareVersion?.content_format_snapshot ||
                  editor.contentFormat) === 'json'
                  ? 'json'
                  : 'shell'
              }
              original={compareVersion?.content_snapshot || ''}
              modified={editor.content || ''}
              options={{
                renderSideBySide: true,
                automaticLayout: true,
                readOnly: true,
                minimap: {enabled: false},
                fontSize: 13,
                scrollBeyondLastLine: false,
              }}
            />
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={jobMetricsDialogOpen}
        onOpenChange={setJobMetricsDialogOpen}
      >
        <DialogContent className='flex h-[86vh] w-[94vw] max-w-[94vw] flex-col overflow-hidden sm:max-w-[1380px]'>
          <DialogHeader>
            <DialogTitle>
              {t('metricsDetails')}{' '}
              {metricsDialogJob ? `#${metricsDialogJob.id}` : ''}
            </DialogTitle>
          </DialogHeader>
          <MetricsDialogContent job={metricsDialogJob} />
        </DialogContent>
      </Dialog>

      <Dialog
        open={jobScriptOpen}
        onOpenChange={(open) => {
          setJobScriptOpen(open);
          if (!open) {
            setJobScriptTarget(null);
          }
        }}
      >
        <DialogContent className='flex h-[86vh] w-[92vw] max-w-[92vw] flex-col overflow-hidden sm:max-w-[1200px]'>
          <DialogHeader>
            <DialogTitle>
              {t('actualExecutedScript')}{' '}
              {jobScriptTarget ? `#${jobScriptTarget.id}` : ''}
            </DialogTitle>
          </DialogHeader>
          <JobScriptDialogContent
            job={jobScriptTarget}
            monacoTheme={monacoTheme}
          />
        </DialogContent>
      </Dialog>

      <Dialog open={logsDialogOpen} onOpenChange={setLogsDialogOpen}>
        <DialogContent className='flex h-[88vh] w-[96vw] max-w-[96vw] flex-col overflow-hidden sm:max-w-[1400px]'>
          <DialogHeader>
            <DialogTitle>{t('logViewer')}</DialogTitle>
            <DialogDescription>{t('logViewerDesc')}</DialogDescription>
          </DialogHeader>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='flex items-center gap-2'>
              <Input
                value={logSearchTerm}
                onChange={(event) => setLogSearchTerm(event.target.value)}
                placeholder={t('searchLogKeyword')}
                className='h-8 w-[320px]'
              />
              <div className='flex items-center gap-1 rounded-md border border-border/50 bg-background px-1 py-1'>
                <Funnel className='ml-1 size-3.5 text-muted-foreground' />
                {(['all', 'warn', 'error'] as LogFilterMode[]).map((mode) => (
                  <button
                    key={mode}
                    type='button'
                    className={cn(
                      'rounded px-2 py-1 text-xs',
                      logFilterMode === mode
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground',
                    )}
                    onClick={() => setLogFilterMode(mode)}
                  >
                    {mode === 'all' ? t('all') : mode.toUpperCase()}
                  </button>
                ))}
              </div>
            </div>
            <div className='flex items-center gap-2 text-xs text-muted-foreground'>
              <span>
                {expandedLogsLoading
                  ? t('loadingAllLogs')
                  : t('totalLines', {
                      count: splitLogLines(
                        expandedJobLogs?.logs || jobLogs?.logs || '',
                      ).length,
                    })}
              </span>
            </div>
          </div>
          <VirtualizedLogViewer
            lines={splitLogLines(expandedJobLogs?.logs || jobLogs?.logs || '')}
            height={620}
            emptyText={t('noLogs')}
          />
        </DialogContent>
      </Dialog>

      <Dialog
        open={checkpointInspectDialogOpen}
        onOpenChange={setCheckpointInspectDialogOpen}
      >
        <DialogContent className='flex h-[84vh] w-[88vw] max-w-[88vw] flex-col overflow-hidden sm:max-w-[1180px]'>
          <DialogHeader>
            <DialogTitle>{t('checkpointFileDetails')}</DialogTitle>
            <DialogDescription className='break-all'>
              {checkpointInspectDialogResult?.path || '-'}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className='min-h-0 flex-1 rounded-md border border-border/50 bg-muted/10 p-3'>
            <div className='space-y-4'>
              <CheckpointInspectOverviewSection
                result={checkpointInspectDialogResult}
                t={t}
              />
              <CheckpointInspectSourceHighlightsSection
                title={t('checkpointSourceState')}
                result={checkpointInspectDialogResult}
                decodeStrategyLabel={t('decodeStrategy')}
                splitCountLabel={t('splitCount')}
                warningsLabel={t('warnings')}
                unsupportedLabel={t('unsupportedSources')}
                currentOffsetLabel={t('currentOffset')}
                targetLabel={t('sourceTarget')}
                progressLabel={t('sourceProgress')}
              />
              <CheckpointInspectPrimaryTableSection
                title={t('actions')}
                result={checkpointInspectDialogResult}
                sourceStateTitle={t('checkpointSourceState')}
                decodeStrategyLabel={t('decodeStrategy')}
                coordinatorLabel={t('coordinator')}
                unsupportedLabel={t('unsupportedSources')}
                rawDetailsLabel={t('rawDetails')}
              />
              <CheckpointInspectRawDetailsSection
                result={checkpointInspectDialogResult}
                t={t}
              />
            </div>
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <Dialog
        open={treeDialog.open}
        onOpenChange={(open) => setTreeDialog((prev) => ({...prev, open}))}
      >
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>
              {treeDialog.action === 'create-folder'
                ? t('newFolder')
                : treeDialog.action === 'create-file'
                  ? t('newFile')
                  : treeDialog.action === 'move'
                    ? t('moveTo')
                    : treeDialog.action === 'delete'
                      ? t('deleteConfirm')
                      : t('rename')}
            </DialogTitle>
          </DialogHeader>
          <div className='grid gap-3'>
            {treeDialog.action === 'delete' ? (
              <div className='grid gap-3'>
                <div className='rounded-lg border border-border/40 bg-muted/10 p-3 text-sm'>
                  {t('willDelete')}
                  {treeDialog.targetNode?.node_type === 'folder'
                    ? t('folder')
                    : t('file')}
                  <span className='mx-1 font-medium'>
                    {treeDialog.targetNode?.name}
                  </span>
                  {treeDialog.targetNode?.node_type === 'folder'
                    ? t('deleteFolderDesc')
                    : t('deleteFileDesc')}
                </div>
                <div className='grid gap-2'>
                  <Label htmlFor='tree-dialog-delete-name'>
                    {t('typeNameToDelete')}
                  </Label>
                  <Input
                    id='tree-dialog-delete-name'
                    value={treeDialog.name}
                    onChange={(event) =>
                      setTreeDialog((prev) => ({
                        ...prev,
                        name: event.target.value,
                      }))
                    }
                    placeholder={treeDialog.targetNode?.name || t('inputName')}
                  />
                </div>
              </div>
            ) : treeDialog.action === 'move' ? (
              <div className='grid gap-2'>
                <Label>{t('targetFolder')}</Label>
                <div className='max-h-[320px] overflow-auto rounded-md border border-border/60 bg-muted/10 p-2'>
                  <div className='space-y-1'>
                    {moveTargetOptions.map((option) => (
                      <button
                        key={option.value ?? 'root'}
                        type='button'
                        className={cn(
                          'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent',
                          treeDialog.targetParentId === option.value
                            ? 'bg-accent text-accent-foreground'
                            : 'text-muted-foreground',
                        )}
                        style={{paddingLeft: `${8 + option.depth * 12}px`}}
                        onClick={() =>
                          setTreeDialog((prev) => ({
                            ...prev,
                            targetParentId: option.value,
                          }))
                        }
                      >
                        <Folder className='size-4 shrink-0' />
                        <span className='truncate'>{option.label}</span>
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className='grid gap-2'>
                <Label htmlFor='tree-dialog-name'>{t('name')}</Label>
                <Input
                  id='tree-dialog-name'
                  value={treeDialog.name}
                  onChange={(event) =>
                    setTreeDialog((prev) => ({
                      ...prev,
                      name: event.target.value,
                    }))
                  }
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      void handleTreeDialogSubmit();
                    }
                  }}
                  placeholder={t('workspaceNamePlaceholder')}
                />
              </div>
            )}
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                onClick={() =>
                  setTreeDialog({
                    open: false,
                    action: null,
                    targetNode: null,
                    name: '',
                    targetParentId: null,
                  })
                }
              >
                {t('cancel')}
              </Button>
              <Button onClick={() => void handleTreeDialogSubmit()}>
                {t('confirm')}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {treeMenu.open ? (
        <div
          className='fixed z-50 min-w-[160px] rounded-md border bg-popover p-1 shadow-md'
          style={{left: treeMenu.x, top: treeMenu.y}}
        >
          {treeMenu.kind === 'root' || treeMenu.kind === 'folder' ? (
            <>
              <button
                type='button'
                className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
                onClick={() => openTreeDialog('create-folder', treeMenu.node)}
              >
                {t('newFolder')}
              </button>
              {treeMenu.kind === 'folder' ? (
                <button
                  type='button'
                  className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
                  onClick={() => openTreeDialog('create-file', treeMenu.node)}
                >
                  {t('newFile')}
                </button>
              ) : null}
            </>
          ) : null}
          {treeMenu.kind !== 'root' ? (
            <button
              type='button'
              className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
              onClick={() =>
                openTreeDialog(
                  'rename',
                  treeMenu.node,
                  treeMenu.node?.name || '',
                )
              }
            >
              {t('rename')}
            </button>
          ) : null}
          {treeMenu.kind !== 'root' ? (
            <button
              type='button'
              className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
              onClick={() => openTreeDialog('move', treeMenu.node)}
            >
              {t('moveTo')}
            </button>
          ) : null}
          {treeMenu.kind === 'file' ? (
            <button
              type='button'
              className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
              onClick={() => void handleCopyFile(treeMenu.node)}
            >
              <Copy className='mr-2 size-4' />
              {t('copyFile')}
            </button>
          ) : null}
          {treeMenu.kind !== 'root' ? (
            <button
              type='button'
              className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm text-destructive hover:bg-accent'
              onClick={() => openTreeDialog('delete', treeMenu.node)}
            >
              <Trash2 className='mr-2 size-4' />
              {t('delete')}
            </button>
          ) : null}
          <button
            type='button'
            className='flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent'
            onClick={() => {
              setTreeMenu((prev) => ({...prev, open: false}));
              void loadWorkspace(selectedNodeId);
            }}
          >
            <RefreshCw className='mr-2 size-4' />
            {t('refresh')}
          </button>
        </div>
      ) : null}
    </div>
  );
}

function TreeView({
  nodes,
  selectedNodeId,
  selectedFolderId,
  expandedFolderIds,
  onSelect,
  onContextMenu,
  depth = 0,
}: {
  nodes: SyncTaskTreeNode[];
  selectedNodeId: number | null;
  selectedFolderId: number | null;
  expandedFolderIds: number[];
  onSelect: (node: SyncTaskTreeNode) => void;
  onContextMenu: (
    event: MouseEvent,
    kind: 'folder' | 'file',
    node: SyncTaskTreeNode,
  ) => void;
  depth?: number;
}) {
  return (
    <div className='space-y-0 py-0.5'>
      {nodes.map((node) => {
        const selected =
          node.id === selectedNodeId || node.id === selectedFolderId;
        const isExpanded = expandedFolderIds.includes(node.id);
        const hasChildren = Boolean(node.children && node.children.length > 0);
        return (
          <div key={node.id}>
            <button
              type='button'
              className={`flex w-full items-center justify-between rounded-sm border border-transparent px-2 py-1 text-left text-xs transition hover:bg-muted/70 ${selected ? 'border-primary/20 bg-primary/10 text-primary' : ''}`}
              style={{paddingLeft: `${depth * 12 + 6}px`}}
              onClick={() => onSelect(node)}
              onContextMenu={(event) =>
                onContextMenu(event, node.node_type, node)
              }
            >
              <span className='flex min-w-0 items-center gap-2'>
                {node.node_type === 'folder' ? (
                  <>
                    {hasChildren ? (
                      isExpanded ? (
                        <ChevronDown className='size-3.5 shrink-0' />
                      ) : (
                        <ChevronRight className='size-3.5 shrink-0' />
                      )
                    ) : (
                      <span className='inline-block size-3.5 shrink-0' />
                    )}
                    <Folder className='size-3.5 shrink-0' />
                  </>
                ) : (
                  <>
                    <span className='inline-block size-3.5 shrink-0' />
                    <FileCode2 className='size-3.5 shrink-0' />
                  </>
                )}
                <span className='truncate'>{node.name}</span>
              </span>
              {node.node_type === 'file' ? (
                <Badge
                  variant='outline'
                  className='h-5 rounded-sm px-1.5 text-[10px]'
                >
                  {node.current_version > 0
                    ? `v${node.current_version}`
                    : 'draft'}
                </Badge>
              ) : null}
            </button>
            {hasChildren && isExpanded ? (
              <TreeView
                nodes={node.children || []}
                selectedNodeId={selectedNodeId}
                selectedFolderId={selectedFolderId}
                expandedFolderIds={expandedFolderIds}
                onSelect={onSelect}
                onContextMenu={onContextMenu}
                depth={depth + 1}
              />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function SidebarIconTab({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type='button'
          aria-label={label}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-md border transition-colors',
            active
              ? 'border-primary/40 bg-primary/10 text-primary'
              : 'border-transparent text-muted-foreground hover:bg-muted hover:text-foreground',
          )}
          onClick={onClick}
        >
          {icon}
        </button>
      </TooltipTrigger>
      <TooltipContent side='left'>{label}</TooltipContent>
    </Tooltip>
  );
}

function StudioSidebarShell({
  children,
  rail,
  className,
}: {
  children: ReactNode;
  rail: ReactNode;
  className?: string;
}) {
  return (
    <Card
      className={cn(
        'row-start-1 gap-0 overflow-hidden border-border/60 bg-background/75 py-0 shadow-sm',
        className,
      )}
    >
      <CardContent className='grid h-full min-h-0 min-w-0 grid-cols-[minmax(0,1fr)_40px] p-0'>
        <div className='min-h-0 min-w-0 overflow-hidden'>
          <ScrollArea className='h-full'>
            <div className='min-w-0 p-3'>{children}</div>
          </ScrollArea>
        </div>
        <div className='flex min-h-0 w-10 shrink-0 flex-col items-center gap-2 border-l border-border/50 bg-muted/10 py-3'>
          {rail}
        </div>
      </CardContent>
    </Card>
  );
}

function TemplatePluginSelect({
  label,
  placeholder,
  items,
  disabled,
  loading,
  loadingText,
  onSelect,
}: {
  label: string;
  placeholder: string;
  items: TemplatePluginItem[];
  disabled?: boolean;
  loading?: boolean;
  loadingText?: string | null;
  onSelect: (value: string) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [open, setOpen] = useState(false);
  const selectedLabel = loading ? loadingText || placeholder : placeholder;

  return (
    <div className='space-y-1.5'>
      <Label className='text-[11px] text-muted-foreground'>{label}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            disabled={disabled}
            className='w-full justify-between'
          >
            <span className='truncate text-left'>{selectedLabel}</span>
            {loading ? (
              <Loader2 className='ml-2 size-4 shrink-0 animate-spin opacity-70' />
            ) : (
              <ChevronDown className='ml-2 size-4 shrink-0 opacity-50' />
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className='w-[var(--radix-popover-trigger-width)] min-w-0 p-0'>
          <Command>
            <CommandInput placeholder={`${placeholder}...`} />
            <CommandList>
              <CommandEmpty>{t('noMatchingPlugins')}</CommandEmpty>
              {items.map((item) => (
                <CommandItem
                  key={`${label}:${item.value}`}
                  value={`${item.label} ${item.value}`}
                  onSelect={() => {
                    setOpen(false);
                    onSelect(item.value);
                  }}
                >
                  <Check className='size-4 opacity-0' />
                  <span className='truncate'>{item.label}</span>
                </CommandItem>
              ))}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}

function SettingsSidebarPanel({
  executionMode,
  clusterId,
  clusters,
  pluginPanelLoading,
  pluginTemplatePendingType,
  pluginTemplateLoadingText,
  sourceTemplateItems,
  transformTemplateItems,
  sinkTemplateItems,
  detectedVariables,
  customVariableRows,
  onExecutionModeChange,
  onClusterChange,
  onInsertPluginTemplate,
  onOpenCreateCustomVariable,
  onOpenEditCustomVariable,
  onDeleteCustomVariable,
  onCopyCustomVariableReference,
  onCopyCustomVariableValue,
}: {
  executionMode: ExecutionMode;
  clusterId: string;
  clusters: ClusterInfo[];
  pluginPanelLoading: boolean;
  pluginTemplatePendingType: SyncPluginType | null;
  pluginTemplateLoadingText: string | null;
  sourceTemplateItems: TemplatePluginItem[];
  transformTemplateItems: TemplatePluginItem[];
  sinkTemplateItems: TemplatePluginItem[];
  detectedVariables: string[];
  customVariableRows: VariableRow[];
  onExecutionModeChange: (value: ExecutionMode) => void;
  onClusterChange: (value: string) => void;
  onInsertPluginTemplate: (
    pluginType: SyncPluginType,
    factoryIdentifier: string,
  ) => void;
  onOpenCreateCustomVariable: () => void;
  onOpenEditCustomVariable: (item: VariableRow) => void;
  onDeleteCustomVariable: (id: string) => void;
  onCopyCustomVariableReference: (key: string) => void;
  onCopyCustomVariableValue: (value: string) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const builtinPreviewNow = useMemo(() => new Date(), []);
  return (
    <div className='mx-auto min-w-0 max-w-[236px] space-y-4'>
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground'>
          {t('settings')}
        </div>
        <div className='space-y-2'>
          <Label className='text-xs'>{t('executionMode')}</Label>
          <Select
            value={executionMode}
            onValueChange={(value) =>
              onExecutionModeChange(value as ExecutionMode)
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className='w-[var(--radix-select-trigger-width)] min-w-0'>
              <SelectItem value='cluster'>{t('clusterMode')}</SelectItem>
              <SelectItem value='local'>{t('localMode')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {executionMode === 'cluster' ? (
        <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
          <Label className='mb-2 block text-xs'>{t('zetaCluster')}</Label>
          <Select
            value={clusterId || '__empty__'}
            onValueChange={onClusterChange}
          >
            <SelectTrigger className='w-full'>
              <SelectValue placeholder={t('selectCluster')} />
            </SelectTrigger>
            <SelectContent className='w-[var(--radix-select-trigger-width)] min-w-0'>
              <SelectItem value='__empty__'>{t('unselected')}</SelectItem>
              {clusters.map((cluster) => (
                <SelectItem key={cluster.id} value={String(cluster.id)}>
                  {cluster.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}

      {executionMode === 'cluster' ? (
        <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
          <Label className='mb-3 block text-xs'>{t('pluginTemplates')}</Label>
          <div className='space-y-3'>
            <TemplatePluginSelect
              disabled={!clusterId || pluginPanelLoading}
              items={sourceTemplateItems}
              label={t('sourceTemplate')}
              loading={pluginTemplatePendingType === 'source'}
              loadingText={
                pluginTemplatePendingType === 'source'
                  ? pluginTemplateLoadingText
                  : null
              }
              placeholder={t('selectSourcePlugin')}
              onSelect={(value) => onInsertPluginTemplate('source', value)}
            />
            <TemplatePluginSelect
              disabled={!clusterId || pluginPanelLoading}
              items={transformTemplateItems}
              label={t('transformTemplate')}
              loading={pluginTemplatePendingType === 'transform'}
              loadingText={
                pluginTemplatePendingType === 'transform'
                  ? pluginTemplateLoadingText
                  : null
              }
              placeholder={t('selectTransformPlugin')}
              onSelect={(value) => onInsertPluginTemplate('transform', value)}
            />
            <TemplatePluginSelect
              disabled={!clusterId || pluginPanelLoading}
              items={sinkTemplateItems}
              label={t('sinkTemplate')}
              loading={pluginTemplatePendingType === 'sink'}
              loadingText={
                pluginTemplatePendingType === 'sink'
                  ? pluginTemplateLoadingText
                  : null
              }
              placeholder={t('selectSinkPlugin')}
              onSelect={(value) => onInsertPluginTemplate('sink', value)}
            />
            <p className='text-[11px] leading-5 text-muted-foreground'>
              {!clusterId
                ? t('selectClusterFirst')
                : pluginPanelLoading
                  ? t('loadingPluginTemplates')
                  : pluginTemplateLoadingText
                    ? t('generatingPluginTemplate', {
                        plugin: pluginTemplateLoadingText,
                      })
                    : t('pluginTemplateHint')}
            </p>
          </div>
        </div>
      ) : null}

      {/* 自定义变量管理区域 */}
      {/* Custom variables management section */}
      <CustomVariablesSection
        variables={customVariableRows}
        onOpenCreate={onOpenCreateCustomVariable}
        onOpenEdit={onOpenEditCustomVariable}
        onDelete={onDeleteCustomVariable}
        onCopyReference={onCopyCustomVariableReference}
        onCopyValue={onCopyCustomVariableValue}
      />

      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <Label className='mb-2 block text-xs'>{t('detectedVariables')}</Label>
        <div className='flex flex-wrap gap-2'>
          {detectedVariables.length > 0 ? (
            detectedVariables.map((variable) => (
              <Tooltip key={variable}>
                <TooltipTrigger asChild>
                  <Badge variant='outline'>{`{{${variable}}}`}</Badge>
                </TooltipTrigger>
                <TooltipContent className='max-w-[320px] break-all text-xs'>
                  <div>{`{{${variable}}}`}</div>
                  {resolveBuiltinPreviewExpression(
                    variable,
                    builtinPreviewNow,
                  ) ? (
                    <div className='mt-1 text-muted-foreground'>
                      {t('builtinPreviewResult', {
                        value:
                          resolveBuiltinPreviewExpression(
                            variable,
                            builtinPreviewNow,
                          ) || '-',
                      })}
                    </div>
                  ) : null}
                </TooltipContent>
              </Tooltip>
            ))
          ) : (
            <span className='text-xs text-muted-foreground'>
              {t('noDetectedVariables')}
            </span>
          )}
        </div>
      </div>

      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='mb-2 flex items-center gap-2'>
          <Label className='block text-xs'>{t('builtinTimeVariables')}</Label>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type='button'
                size='icon'
                variant='ghost'
                className='size-6 text-muted-foreground'
              >
                <Eye className='size-3.5' />
              </Button>
            </TooltipTrigger>
            <TooltipContent className='max-w-[320px] text-xs leading-5'>
              {t('builtinTimeVariablesHint')}
            </TooltipContent>
          </Tooltip>
        </div>
        <div className='flex flex-wrap gap-2'>
          {BUILTIN_TIME_VARIABLE_ITEMS.map((item) => (
            <Tooltip key={item.expr}>
              <TooltipTrigger asChild>
                <Badge variant='secondary' className='cursor-help'>
                  {`{{${item.expr}}}`}
                </Badge>
              </TooltipTrigger>
              <TooltipContent className='max-w-[360px] break-all text-xs leading-5'>
                <div>{t(item.descKey)}</div>
                <div className='mt-1 text-muted-foreground'>
                  {t('builtinPreviewResult', {
                    value:
                      resolveBuiltinPreviewExpression(
                        item.expr,
                        builtinPreviewNow,
                      ) || '-',
                  })}
                </div>
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      </div>
    </div>
  );
}



function VersionSidebarPanel({
  taskId,
  currentVersion,
  versions,
  total,
  page,
  pageSize,
  onPageChange,
  onPreview,
  onCompare,
  onRollback,
  onDelete,
}: {
  taskId?: number;
  currentVersion: number;
  versions: SyncTaskVersion[];
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPreview: (version: SyncTaskVersion) => void;
  onCompare: (version: SyncTaskVersion) => void;
  onRollback: (versionId: number) => void;
  onDelete: (versionId: number) => void;
}) {
  const t = useTranslations('workbenchStudio');
  if (!taskId) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('selectFileToViewVersions')}
      </div>
    );
  }
  return (
    <div className='space-y-3'>
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='flex items-center justify-between gap-2'>
          <div>
            <div className='text-[11px] uppercase tracking-wide text-muted-foreground'>
              {t('versionManagement')}
            </div>
            <div className='mt-1 text-lg font-semibold'>v{currentVersion}</div>
          </div>
          <Badge variant='outline'>
            {t('totalItems', {count: versions.length})}
          </Badge>
        </div>
        <p className='mt-2 text-xs leading-5 text-muted-foreground'>
          {t('versionManagementDesc')}
        </p>
      </div>
      <div className='space-y-2'>
        {versions.length > 0 ? (
          versions.map((version) => (
            <div
              key={version.id}
              className='rounded-lg border border-border/50 bg-background/70 p-3'
            >
              <div className='flex items-center justify-between gap-2'>
                <div>
                  <div className='text-sm font-medium'>v{version.version}</div>
                  <div className='text-[11px] text-muted-foreground'>
                    {new Date(version.created_at).toLocaleString()}
                  </div>
                </div>
                <Badge
                  variant={
                    version.version === currentVersion ? 'secondary' : 'outline'
                  }
                >
                  #{version.id}
                </Badge>
              </div>
              <div className='mt-3 grid grid-cols-2 gap-2'>
                <Button
                  size='sm'
                  variant='outline'
                  className='h-8 text-xs'
                  onClick={() => onPreview(version)}
                >
                  {t('preview')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  className='h-8 text-xs'
                  onClick={() => onCompare(version)}
                >
                  {t('compare')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  className='h-8 text-xs'
                  onClick={() => onRollback(version.id)}
                >
                  {t('rollback')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  className='h-8 text-xs'
                  onClick={() => onDelete(version.id)}
                >
                  {t('delete')}
                </Button>
              </div>
            </div>
          ))
        ) : (
          <div className='text-sm text-muted-foreground'>
            {t('noVersionHistory')}
          </div>
        )}
      </div>
      <SimplePagination
        total={total}
        page={page}
        pageSize={pageSize}
        onPageChange={onPageChange}
      />
    </div>
  );
}

function SimplePagination({
  total,
  page,
  pageSize,
  onPageChange,
}: {
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const totalPages = Math.max(1, Math.ceil(total / Math.max(pageSize, 1)));
  if (total <= pageSize) {
    return null;
  }
  return (
    <div className='flex items-center justify-between gap-2 rounded-lg border border-border/50 bg-muted/10 px-3 py-2 text-xs text-muted-foreground'>
      <span>{t('paginationSummary', {page, totalPages, total})}</span>
      <div className='flex items-center gap-2'>
        <Button
          size='sm'
          variant='outline'
          className='h-7 px-2 text-xs'
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t('prevPage')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          className='h-7 px-2 text-xs'
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t('nextPage')}
        </Button>
      </div>
    </div>
  );
}

function ConsolePanel({
  job,
  logsResult,
  loading,
  filterMode,
  onFilterChange,
  onExpand,
}: {
  job: SyncJobInstance | null;
  logsResult: SyncJobLogsResult | null;
  loading: boolean;
  filterMode: LogFilterMode;
  onFilterChange: (mode: LogFilterMode) => void;
  onExpand: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  if (!job) {
    return <div className='text-sm text-muted-foreground'>{t('noLogs')}</div>;
  }
  const displayStatus = getDisplayJobLifecycleStatus(job);
  const renderedLines = buildDisplayLogLines(logsResult?.logs || '', 800);
  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-2'>
      <div className='flex flex-wrap items-center gap-2 rounded-lg border border-border/50 bg-background/70 px-3 py-2 text-xs'>
        <Badge variant='outline'>#{job.id}</Badge>
        <Badge variant='outline'>{job.run_type}</Badge>
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm border px-2 py-0.5 text-[11px]',
            getJobStatusBadgeClass(displayStatus),
          )}
        >
          {getJobStatusLabel(displayStatus)}
        </Badge>
        <Badge variant='outline'>
          {getEngineAPIMode(job) === 'v1'
            ? 'Legacy REST V1'
            : submitSpecExecutionMode(job.submit_spec) === 'local'
              ? 'Local Agent'
              : 'REST V2'}
        </Badge>
        <span className='min-w-0 flex-1 truncate text-muted-foreground'>
          {getEngineEndpointLabel(job)}
        </span>
        <span className='text-muted-foreground'>
          {loading
            ? t('loading')
            : logsResult?.updated_at
              ? new Date(logsResult.updated_at).toLocaleTimeString()
              : '-'}
        </span>
      </div>
      <div className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
        <div className='sticky top-0 z-10 shrink-0 border-b border-border/50 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85'>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-2 text-xs text-muted-foreground'>
            <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
              <span className='shrink-0'>{t('liveLogs')}</span>
              {job.error_message ? (
                <Badge
                  className='rounded-sm border-red-500/30 bg-red-500/10 text-[10px] text-red-600 dark:text-red-400'
                  variant='outline'
                >
                  {t('hasErrors')}
                </Badge>
              ) : null}
            </div>
            <div className='flex items-center justify-self-end gap-2 whitespace-nowrap'>
              <div className='flex items-center gap-1 rounded-md border border-border/50 bg-background px-1 py-1'>
                {(['all', 'warn', 'error'] as LogFilterMode[]).map((mode) => (
                  <button
                    key={mode}
                    type='button'
                    className={cn(
                      'rounded px-2 py-0.5 text-[11px]',
                      filterMode === mode
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground',
                    )}
                    onClick={() => onFilterChange(mode)}
                  >
                    {mode === 'all' ? t('all') : mode.toUpperCase()}
                  </button>
                ))}
              </div>
              <Button
                size='sm'
                variant='ghost'
                className='h-7 px-1.5 text-xs'
                onClick={onExpand}
              >
                <Maximize2 className='mr-1 size-3.5' />
                {t('expand')}
              </Button>
            </div>
          </div>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-border/50 px-3 py-2 text-[11px] text-muted-foreground'>
            <div className='flex min-w-0 items-center gap-3 overflow-hidden'>
              <span className='truncate'>
                {t('jobId')}: {job.platform_job_id || job.engine_job_id || '-'}
              </span>
              {job.engine_job_id &&
              job.platform_job_id &&
              job.engine_job_id !== job.platform_job_id ? (
                <span className='truncate'>
                  {t('engineJobId')}: {job.engine_job_id}
                </span>
              ) : null}
            </div>
            <span className='justify-self-end whitespace-nowrap'>
              {t('logFocusHint')}
            </span>
          </div>
        </div>
        <div className='min-h-0 min-w-0 flex-1 overflow-auto p-3 font-mono text-xs'>
          {renderedLines.length > 0 ? (
            renderedLines.map((line, index) => (
              <div
                key={`${index}-${line.slice(0, 24)}`}
                className={cn(
                  'max-w-full whitespace-pre-wrap break-all',
                  getLogLineClass(line),
                )}
              >
                {line}
              </div>
            ))
          ) : (
            <div className='text-muted-foreground'>{t('noLogs')}</div>
          )}
        </div>
      </div>
    </div>
  );
}

function JobRunsPanel({
  jobs,
  selectedJobId,
  onSelectJob,
  onRecover,
  onCancel,
  onSavepointStop,
  onViewMetrics,
  onViewScript,
  disableRecover,
}: {
  jobs: SyncJobInstance[];
  selectedJobId: number | null;
  onSelectJob: (jobId: number) => void;
  onRecover: (jobId: number) => void;
  onCancel: (jobId: number) => void;
  onSavepointStop: (jobId: number) => void;
  onViewMetrics: (job: SyncJobInstance) => void;
  onViewScript: (job: SyncJobInstance) => void;
  disableRecover: boolean;
}) {
  const t = useTranslations('workbenchStudio');
  if (jobs.length === 0) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noJobRuns')}</div>
    );
  }
  return (
    <div className='h-full overflow-auto rounded-lg border border-border/50 bg-background/70'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('task')}</TableHead>
            <TableHead>{t('runMode')}</TableHead>
            <TableHead>{t('status')}</TableHead>
            <TableHead>{t('channel')}</TableHead>
            <TableHead>{t('startedAt')}</TableHead>
            <TableHead>{t('finishedAt')}</TableHead>
            <TableHead>{t('duration')}</TableHead>
            <TableHead>{t('metrics')}</TableHead>
            <TableHead className='text-right'>{t('actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {jobs.map((job) => {
            const summary = extractJobMetricSummary(job);
            const displayStatus = getDisplayJobLifecycleStatus(job);
            return (
              <TableRow
                key={job.id}
                className={cn(selectedJobId === job.id ? 'bg-primary/5' : '')}
                onClick={() => onSelectJob(job.id)}
              >
                <TableCell>
                  <div className='font-medium'>#{job.id}</div>
                  <div className='text-xs text-muted-foreground'>
                    {job.platform_job_id || '-'}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant='outline' className='rounded-sm text-[11px]'>
                    {getRunModeLabel(job, t)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge
                    variant='outline'
                    className={cn(
                      'rounded-sm border px-2 py-0.5 text-[11px]',
                      getJobStatusBadgeClass(displayStatus),
                    )}
                  >
                    {getJobStatusLabel(displayStatus)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge variant='outline' className='rounded-sm text-[11px]'>
                    {submitSpecExecutionMode(job.submit_spec) === 'local'
                      ? 'Local Agent'
                      : getEngineAPIMode(job) === 'v1'
                        ? 'Legacy REST V1'
                        : 'REST V2'}
                  </Badge>
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDateTime(job.started_at)}
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDateTime(job.finished_at)}
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDuration(job.started_at, job.finished_at)}
                </TableCell>
                <TableCell>
                  <div className='space-y-0.5 text-xs'>
                    <div>
                      {t('read')} {formatMetricValue(summary.readCount)}
                    </div>
                    <div>
                      {t('write')} {formatMetricValue(summary.writeCount)}
                    </div>
                    <div>
                      {t('averageSpeed')}{' '}
                      {formatMetricValue(summary.averageSpeed, 1)}/s
                    </div>
                  </div>
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-2'>
                    <Button
                      size='icon'
                      variant='outline'
                      className='size-8'
                      aria-label={t('viewExecutedScript')}
                      onClick={(event) => {
                        event.stopPropagation();
                        onViewScript(job);
                      }}
                    >
                      <FileCode2 className='size-4' />
                    </Button>
                    <Button
                      size='icon'
                      variant='outline'
                      className='size-8'
                      aria-label={t('viewMetrics')}
                      onClick={(event) => {
                        event.stopPropagation();
                        onViewMetrics(job);
                      }}
                    >
                      <BarChart3 className='size-4' />
                    </Button>
                    {job.run_type !== 'preview' ? (
                      <Button
                        size='sm'
                        variant='outline'
                        className='h-8 text-xs'
                        disabled={disableRecover || !canRecoverFromJob(job)}
                        onClick={(event) => {
                          event.stopPropagation();
                          onRecover(job.id);
                        }}
                      >
                        {t('recover')}
                      </Button>
                    ) : null}
                    {isJobLifecycleActive(getDisplayJobLifecycleStatus(job)) ? (
                      <>
                        <Button
                          size='sm'
                          variant='outline'
                          className='h-8 text-xs'
                          onClick={(event) => {
                            event.stopPropagation();
                            onSavepointStop(job.id);
                          }}
                        >
                          {t('savepointStop')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          className='h-8 text-xs'
                          onClick={(event) => {
                            event.stopPropagation();
                            onCancel(job.id);
                          }}
                        >
                          {t('stop')}
                        </Button>
                      </>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function PreviewWorkspacePanel({
  job,
  previewSnapshot,
  datasets,
  selectedDatasetName,
  previewPage,
  loading,
  monacoTheme,
  onSelectDataset,
  onChangePage,
}: {
  job: SyncJobInstance | null;
  previewSnapshot: SyncPreviewSnapshot | null;
  datasets: SyncPreviewDataset[];
  selectedDatasetName: string;
  previewPage: number;
  loading?: boolean;
  monacoTheme: string;
  onSelectDataset: (name: string) => void;
  onChangePage: (page: number) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [previewScriptOpen, setPreviewScriptOpen] = useState(false);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noPreviewJobs')}</div>
    );
  }
  const activeDataset =
    datasets.find((dataset) => dataset.name === selectedDatasetName) ||
    datasets[0] ||
    null;
  const columns = activeDataset?.columns || [];
  const rows = (activeDataset?.rows || []) as Array<Record<string, unknown>>;
  const previewContent =
    typeof previewSnapshot?.injected_script === 'string' &&
    previewSnapshot.injected_script
      ? previewSnapshot.injected_script
      : typeof job.result_preview?.preview_content === 'string'
        ? job.result_preview.preview_content
        : '';
  const previewContentFormat =
    typeof previewSnapshot?.content_format === 'string' &&
    previewSnapshot.content_format
      ? previewSnapshot.content_format
      : typeof job.result_preview?.content_format === 'string'
        ? job.result_preview.content_format
        : 'hocon';
  const previewEmptyMessage =
    previewSnapshot?.empty_reason === 'preview_not_ready'
      ? t('preparingPreview')
      : t('noPreviewDataFallback');
  const pageSize = Math.max(activeDataset?.page_size || 20, 1);
  const total = Math.max(activeDataset?.total || rows.length, rows.length);
  const totalPages = Math.max(Math.ceil(total / pageSize), 1);
  const currentPage = Math.min(Math.max(previewPage, 1), totalPages);
  const pageRows = rows.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  return (
    <div className='grid h-full min-h-0 gap-3 lg:grid-cols-[220px_minmax(0,1fr)]'>
      <div className='flex min-h-0 flex-col rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='mb-3 shrink-0 text-sm font-medium'>
          {t('tableTabs')}
        </div>
        <ScrollArea className='min-h-0 flex-1'>
          <div className='space-y-2 pr-2'>
            {datasets.length > 0 ? (
              datasets.map((dataset) => (
                <Tooltip key={dataset.name}>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      title={dataset.name}
                      className={cn(
                        'flex w-full items-center justify-between rounded-md border px-3 py-2 text-left text-sm',
                        dataset.name === activeDataset?.name
                          ? 'border-primary/30 bg-primary/5'
                          : 'border-border/50 bg-background/60',
                      )}
                      onClick={() => onSelectDataset(dataset.name)}
                    >
                      <span className='min-w-0 truncate'>{dataset.name}</span>
                      <Badge variant='outline'>
                        {dataset.total ?? (dataset.rows || []).length}
                      </Badge>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent
                    side='right'
                    className='max-w-[480px] break-all'
                  >
                    {dataset.name}
                  </TooltipContent>
                </Tooltip>
              ))
            ) : (
              <div className='rounded-md border border-border/50 bg-background/60 px-3 py-2 text-sm text-muted-foreground'>
                {t('noDatasets')}
              </div>
            )}
          </div>
        </ScrollArea>
      </div>
      <div className='flex min-h-0 flex-col rounded-lg border border-border/50 bg-background/70'>
        <div className='flex items-center justify-between border-b border-border/50 px-3 py-2 text-sm font-medium'>
          <span>{t('dataTable')}</span>
          <div className='flex items-center gap-2 text-xs text-muted-foreground'>
            {previewContent ? (
              <Button
                size='sm'
                variant='outline'
                className='h-7 px-2 text-xs'
                onClick={() => setPreviewScriptOpen(true)}
              >
                {t('injectedScript')}
              </Button>
            ) : null}
            <Button
              size='sm'
              variant='outline'
              className='h-7 px-2 text-xs'
              onClick={() => onChangePage(Math.max(currentPage - 1, 1))}
              disabled={currentPage <= 1}
            >
              {t('prevPage')}
            </Button>
            <span>
              {currentPage} / {totalPages}
            </span>
            <Button
              size='sm'
              variant='outline'
              className='h-7 px-2 text-xs'
              onClick={() =>
                onChangePage(Math.min(currentPage + 1, totalPages))
              }
              disabled={currentPage >= totalPages}
            >
              {t('nextPage')}
            </Button>
          </div>
        </div>
        {loading ? (
          <div className='flex min-h-0 flex-1 items-center justify-center'>
            <div className='flex items-center gap-2 text-sm text-muted-foreground'>
              <Loader2 className='size-4 animate-spin' />
              <span>{t('preparingPreview')}</span>
            </div>
          </div>
        ) : columns.length > 0 ? (
          <div className='min-h-0 flex-1 overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  {columns.map((column) => (
                    <TableHead key={column}>{column}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {pageRows.length > 0 ? (
                  pageRows.map((row, index) => (
                    <TableRow key={index}>
                      {columns.map((column) => (
                        <TableCell key={`${index}-${column}`}>
                          {column === 'RowKind' ? (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Badge
                                  variant='outline'
                                  className={cn(
                                    'max-w-full truncate rounded-sm',
                                    getPreviewRowKindBadgeClass(
                                      formatCellValue(row[column]),
                                    ),
                                  )}
                                >
                                  {formatCellValue(row[column])}
                                </Badge>
                              </TooltipTrigger>
                              <TooltipContent>
                                {formatCellValue(row[column])}
                              </TooltipContent>
                            </Tooltip>
                          ) : (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span className='block truncate'>
                                  {formatCellValue(row[column])}
                                </span>
                              </TooltipTrigger>
                              <TooltipContent className='max-w-[480px] break-all'>
                                {formatCellValue(row[column])}
                              </TooltipContent>
                            </Tooltip>
                          )}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell
                      colSpan={columns.length}
                      className='text-center text-muted-foreground'
                    >
                      {t('noPreviewDataFallback')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        ) : (
          <div className='flex min-h-0 flex-1 items-center justify-center p-3 text-sm text-muted-foreground'>
            {previewEmptyMessage}
          </div>
        )}
      </div>
      <Dialog open={previewScriptOpen} onOpenChange={setPreviewScriptOpen}>
        <DialogContent className='flex h-[86vh] w-[94vw] max-w-[94vw] flex-col overflow-hidden sm:max-w-[1380px]'>
          <DialogHeader>
            <DialogTitle>{t('injectedScript')}</DialogTitle>
            <DialogDescription>
              {previewContentFormat.toUpperCase()}
            </DialogDescription>
          </DialogHeader>
          <div className='min-h-0 flex-1 overflow-hidden rounded-md border border-border/50'>
            <MonacoEditor
              height='100%'
              language={previewContentFormat === 'json' ? 'json' : 'ini'}
              theme={monacoTheme}
              value={previewContent}
              options={{
                readOnly: true,
                minimap: {enabled: false},
                automaticLayout: true,
                wordWrap: 'on',
                scrollBeyondLastLine: false,
                fontSize: 13,
                renderLineHighlight: 'all',
                padding: {top: 14, bottom: 14},
              }}
            />
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CheckpointWorkspacePanel({
  job,
  checkpointSnapshot,
  loading,
  checkpointFiles,
  checkpointFilesLoading,
  onInspectCheckpointFile,
  inspectLoadingPath,
  onRefresh,
}: {
  job: SyncJobInstance | null;
  checkpointSnapshot: SyncCheckpointSnapshot | null;
  loading?: boolean;
  checkpointFiles: RuntimeStorageListItem[];
  checkpointFilesLoading?: boolean;
  onInspectCheckpointFile: (path: string) => void;
  inspectLoadingPath?: string | null;
  onRefresh: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  const checkpointFilesByID = useMemo(() => {
    const mapping = new Map<string, RuntimeStorageListItem>();
    checkpointFiles.forEach((item: RuntimeStorageListItem) => {
      const identity = extractCheckpointFileIdentity(item.name || item.path);
      if (!identity) {
        return;
      }
      const key = `${identity.pipelineId}:${identity.checkpointId}`;
      if (!mapping.has(key)) {
        mapping.set(key, item);
      }
    });
    return mapping;
  }, [checkpointFiles]);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noCheckpointJob')}
      </div>
    );
  }
  const pipelines = checkpointSnapshot?.overview?.pipelines || [];
  const history = checkpointSnapshot?.history || [];
  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      {loading ? (
        <div className='flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground'>
          <Loader2 className='mr-2 size-4 animate-spin' />
          {t('loadingCheckpoint')}
        </div>
      ) : checkpointSnapshot?.empty_reason ? (
        <div className='flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-border/60 bg-muted/10 p-4 text-sm text-muted-foreground'>
          {checkpointSnapshot.message || t('checkpointEmpty')}
        </div>
      ) : (
        <div className='flex min-h-0 flex-1 flex-col gap-3 overflow-hidden'>
          <div className='grid min-h-0 gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]'>
            <div className='flex min-h-0 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
              <div className='flex items-center justify-between border-b border-border/50 px-3 py-2 text-sm font-medium'>
                <span>{t('checkpointOverview')}</span>
                <Button
                  size='icon'
                  variant='ghost'
                  className='size-7'
                  onClick={onRefresh}
                >
                  <RefreshCw className='size-3.5' />
                </Button>
              </div>
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background'>
                    <TableRow>
                      <TableHead>{t('pipeline')}</TableHead>
                      <TableHead>{t('triggered')}</TableHead>
                      <TableHead>{t('completed')}</TableHead>
                      <TableHead>{t('failed')}</TableHead>
                      <TableHead>{t('inProgress')}</TableHead>
                      <TableHead>{t('restored')}</TableHead>
                      <TableHead>{t('latestCompleted')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pipelines.length > 0 ? (
                      pipelines.map((pipeline) => (
                        <TableRow key={pipeline.pipelineId}>
                          <TableCell>{pipeline.pipelineId}</TableCell>
                          <TableCell>
                            {pipeline.counts?.triggered ?? '-'}
                          </TableCell>
                          <TableCell>
                            {pipeline.counts?.completed ?? '-'}
                          </TableCell>
                          <TableCell>
                            {pipeline.counts?.failed ?? '-'}
                          </TableCell>
                          <TableCell>
                            {pipeline.counts?.inProgress ?? '-'}
                          </TableCell>
                          <TableCell>
                            {pipeline.counts?.restored ?? '-'}
                          </TableCell>
                          <TableCell>
                            {pipeline.latestCompleted?.checkpointId
                              ? `#${pipeline.latestCompleted.checkpointId}`
                              : '-'}
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-center text-muted-foreground'
                        >
                          {t('checkpointEmpty')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>

            <div className='flex min-h-0 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
              <div className='border-b border-border/50 px-3 py-2 text-sm font-medium'>
                {t('checkpointHistory')}
              </div>
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background'>
                    <TableRow>
                      <TableHead>{t('pipeline')}</TableHead>
                      <TableHead>{t('checkpointId')}</TableHead>
                      <TableHead>{t('checkpointStatus')}</TableHead>
                      <TableHead>{t('durationMillis')}</TableHead>
                      <TableHead>{t('stateSize')}</TableHead>
                      <TableHead className='text-right'>
                        {t('actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {history.length > 0 ? (
                      history.map((item, index) => {
                        const checkpointId = item.checkpoint?.checkpointId;
                        const checkpointKey =
                          checkpointId !== undefined
                            ? `${item.pipelineId}:${checkpointId}`
                            : '';
                        const matchedFile =
                          checkpointKey !== ''
                            ? checkpointFilesByID.get(checkpointKey)
                            : undefined;
                        return (
                          <TableRow
                            key={`${item.pipelineId}-${item.checkpoint?.checkpointId || index}`}
                          >
                            <TableCell>{item.pipelineId}</TableCell>
                            <TableCell>
                              {checkpointId ? `#${checkpointId}` : '-'}
                            </TableCell>
                            <TableCell>
                              {item.checkpoint?.status ? (
                                <Badge
                                  variant='outline'
                                  className={cn(
                                    'rounded-sm border px-2 py-0.5 text-[11px]',
                                    getCheckpointStatusBadgeClass(
                                      item.checkpoint.status,
                                    ),
                                  )}
                                >
                                  {item.checkpoint.status}
                                </Badge>
                              ) : (
                                '-'
                              )}
                            </TableCell>
                            <TableCell>
                              {item.checkpoint?.durationMillis ?? '-'}
                            </TableCell>
                            <TableCell>
                              {item.checkpoint?.stateSize ?? '-'}
                            </TableCell>
                            <TableCell className='text-right'>
                              <Button
                                size='sm'
                                variant='outline'
                                className='h-8 text-xs'
                                disabled={
                                  !matchedFile?.path ||
                                  inspectLoadingPath === matchedFile.path
                                }
                                onClick={() =>
                                  matchedFile?.path &&
                                  onInspectCheckpointFile(matchedFile.path)
                                }
                              >
                                {inspectLoadingPath === matchedFile?.path ? (
                                  <Loader2 className='mr-2 size-3.5 animate-spin' />
                                ) : (
                                  <Eye className='mr-2 size-3.5' />
                                )}
                                {t('viewDetails')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        );
                      })
                    ) : (
                      <TableRow>
                        <TableCell
                          colSpan={6}
                          className='text-center text-muted-foreground'
                        >
                          {t('checkpointHistoryEmpty')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function CheckpointDetailsSummary({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <summary
      className={cn(
        'flex cursor-pointer list-none items-center gap-2 px-3 py-2 marker:hidden',
        className,
      )}
    >
      <span className='inline-flex size-4 shrink-0 items-center justify-center rounded-sm border border-border/60 text-muted-foreground'>
        <Plus className='size-3 group-open:hidden' />
        <Minus className='hidden size-3 group-open:block' />
      </span>
      <div className='min-w-0 flex-1'>{children}</div>
    </summary>
  );
}

function CheckpointInspectSectionShell({
  title,
  children,
  collapsible = false,
  defaultOpen = true,
}: {
  title: string;
  children: ReactNode;
  collapsible?: boolean;
  defaultOpen?: boolean;
}) {
  if (collapsible) {
    return (
      <details
        open={defaultOpen}
        className='group rounded-lg border border-border/50 bg-background/80'
      >
        <CheckpointDetailsSummary className='border-b border-border/50 text-sm font-medium'>
          <span>{title}</span>
        </CheckpointDetailsSummary>
        <div className='p-3'>{children}</div>
      </details>
    );
  }
  return (
    <div className='rounded-lg border border-border/50 bg-background/80'>
      <div className='border-b border-border/50 px-3 py-2 text-sm font-medium'>
        {title}
      </div>
      <div className='p-3'>{children}</div>
    </div>
  );
}

function CheckpointInspectObjectSection({
  title,
  value,
  defaultOpen = false,
}: {
  title: string;
  value?: Record<string, unknown> | null;
  defaultOpen?: boolean;
}) {
  const entries = value ? Object.entries(value) : [];
  return (
    <CheckpointInspectSectionShell
      title={title}
      collapsible
      defaultOpen={defaultOpen}
    >
      {entries.length > 0 ? (
        <Table>
          <TableBody>
            {entries.map(([key, entryValue]) => (
              <TableRow key={key}>
                <TableCell className='w-[220px] font-medium'>{key}</TableCell>
                <TableCell>
                  {renderCheckpointFieldValue(key, entryValue)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </CheckpointInspectSectionShell>
  );
}

function CheckpointInspectSourceHighlightsSection({
  title,
  result,
  decodeStrategyLabel,
  splitCountLabel,
  warningsLabel,
  unsupportedLabel,
  currentOffsetLabel,
  targetLabel,
  progressLabel,
}: {
  title: string;
  result: RuntimeStorageCheckpointInspectResult | null;
  decodeStrategyLabel: string;
  splitCountLabel: string;
  warningsLabel: string;
  unsupportedLabel: string;
  currentOffsetLabel: string;
  targetLabel: string;
  progressLabel: string;
}) {
  const sourceStates = Array.isArray(result?.source_state_inspect?.sources)
    ? result.source_state_inspect.sources
    : [];
  const unsupportedSources = Array.isArray(
    result?.source_state_inspect?.unsupported_sources,
  )
    ? result.source_state_inspect.unsupported_sources
    : [];
  const warnings = Array.isArray(result?.source_state_inspect?.warnings)
    ? result.source_state_inspect.warnings
    : [];
  const errorMessage = result?.source_state_inspect?.error_message?.trim();

  if (
    !errorMessage &&
    sourceStates.length === 0 &&
    unsupportedSources.length === 0
  ) {
    return null;
  }

  return (
    <CheckpointInspectSectionShell title={title}>
      <div className='space-y-4'>
        {errorMessage ? (
          <div className='rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300'>
            {errorMessage}
          </div>
        ) : null}
        {warnings.length > 0 ? (
          <div className='rounded-lg border border-border/60 bg-muted/20 p-3'>
            <div className='mb-2 text-xs font-medium text-muted-foreground'>
              {warningsLabel}
            </div>
            <ul className='list-disc space-y-1 pl-5 text-sm'>
              {warnings.map((warning, index) => (
                <li key={`${warning}-${index}`}>{warning}</li>
              ))}
            </ul>
          </div>
        ) : null}
        {sourceStates.length > 0 ? (
          <div className='rounded-md border border-border/50 bg-background/70'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Source</TableHead>
                  <TableHead>Plugin</TableHead>
                  <TableHead>{currentOffsetLabel}</TableHead>
                  <TableHead>{targetLabel}</TableHead>
                  <TableHead>{splitCountLabel}</TableHead>
                  <TableHead>{progressLabel}</TableHead>
                  <TableHead>{decodeStrategyLabel}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sourceStates.map((item, index) => {
                  const summary = summarizeCheckpointSourceState(item);
                  return (
                    <TableRow
                      key={`${item.actionName || item.pluginName || index}`}
                    >
                      <TableCell className='font-medium'>
                        <span className='break-all'>
                          {String(item.actionName || item.pluginName || '-')}
                        </span>
                      </TableCell>
                      <TableCell>{formatCellValue(item.pluginName)}</TableCell>
                      <TableCell>
                        {renderCheckpointFieldValue(
                          'currentOffset',
                          summary.offset,
                        )}
                      </TableCell>
                      <TableCell>
                        {renderCheckpointFieldValue(
                          'sourceTarget',
                          summary.target,
                        )}
                      </TableCell>
                      <TableCell>{summary.splitCount}</TableCell>
                      <TableCell>{summary.progress}</TableCell>
                      <TableCell>
                        {formatCellValue(item.decodeStrategy)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        ) : null}
        {unsupportedSources.length > 0 ? (
          <details className='group rounded-lg border border-border/60 bg-background/60'>
            <CheckpointDetailsSummary className='py-3 text-sm font-medium'>
              <span>{unsupportedLabel}</span>
            </CheckpointDetailsSummary>
            <div className='border-t border-border/50 p-3'>
              <div className='space-y-3'>
                {unsupportedSources.map((item, index) => (
                  <CheckpointInspectMiniObject
                    key={`${item.actionName || index}`}
                    title={String(item.actionName || unsupportedLabel)}
                    value={item}
                  />
                ))}
              </div>
            </div>
          </details>
        ) : null}
      </div>
    </CheckpointInspectSectionShell>
  );
}

function CheckpointInspectOverviewSection({
  result,
  t,
}: {
  result: RuntimeStorageCheckpointInspectResult | null;
  t: (key: string) => string;
}) {
  const summary = buildCheckpointInspectSummary(result);
  return (
    <CheckpointInspectSectionShell title={t('checkpointOverview')}>
      <div className='rounded-md border border-border/50 bg-background/70'>
        <div className='border-b border-border/50'>
          <Table>
            <TableBody>
              <TableRow>
                <TableCell className='w-[140px] text-muted-foreground'>
                  {t('fileName')}
                </TableCell>
                <TableCell className='break-all font-medium'>
                  {result?.file_name || '-'}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
        <div className='overflow-x-auto'>
          <Table>
            <TableHeader>
              <TableRow>
                {summary.map((item) => (
                  <TableHead key={item.label}>{item.label}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                {summary.map((item) => (
                  <TableCell key={item.label} className='font-medium'>
                    {renderCheckpointFieldValue(item.key, item.value)}
                  </TableCell>
                ))}
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>
    </CheckpointInspectSectionShell>
  );
}

function CheckpointInspectRawDetailsSection({
  result,
  t,
}: {
  result: RuntimeStorageCheckpointInspectResult | null;
  t: (key: string) => string;
}) {
  return (
    <details className='group rounded-lg border border-border/60 bg-background/60'>
      <CheckpointDetailsSummary className='px-4 py-3 text-sm font-medium'>
        <span>{t('rawDetails')}</span>
      </CheckpointDetailsSummary>
      <div className='space-y-4 border-t border-border/50 p-4'>
        <CheckpointInspectObjectSection
          title={t('completedCheckpoint')}
          value={result?.completed_checkpoint}
          defaultOpen={false}
        />
        <CheckpointInspectObjectSection
          title={t('pipelineState')}
          value={result?.pipeline_state}
          defaultOpen={false}
        />
      </div>
    </details>
  );
}

function CheckpointInspectPrimaryTableSection({
  title,
  result,
  sourceStateTitle,
  decodeStrategyLabel,
  coordinatorLabel,
  rawDetailsLabel,
  unsupportedLabel,
}: {
  title: string;
  result: RuntimeStorageCheckpointInspectResult | null;
  sourceStateTitle: string;
  decodeStrategyLabel: string;
  coordinatorLabel: string;
  rawDetailsLabel: string;
  unsupportedLabel: string;
}) {
  const rows = buildCheckpointActionViewModels(result);
  return (
    <CheckpointInspectSectionShell title={title}>
      {rows.length > 0 ? (
        <div className='space-y-4'>
          <div className='hidden rounded-md border border-border/50 bg-muted/10 px-3 py-2 text-xs font-medium text-muted-foreground xl:grid xl:grid-cols-[minmax(0,2fr)_100px_100px_110px_100px_160px_160px] xl:gap-3'>
            <div>Action</div>
            <div>Parallelism</div>
            <div>Subtasks</div>
            <div>Chunks</div>
            <div>Acked</div>
            <div>Latest Ack</div>
            <div>{decodeStrategyLabel}</div>
          </div>
          {rows.map((row) => {
            const actionState = row.actionState || {};
            const statistics = row.taskStatistics || {};
            const sourceSubtasks = Array.isArray(row.sourceState?.subtasks)
              ? (row.sourceState?.subtasks as Record<string, unknown>[])
              : [];
            const subtaskRows = buildCheckpointSubtaskRows(row);
            const subtaskSummaryRows =
              summarizeCheckpointSubtaskMetrics(subtaskRows);

            return (
              <details
                key={row.key}
                className='group rounded-lg border border-border/60 bg-background/60'
              >
                <CheckpointDetailsSummary className='py-3'>
                  <div className='grid gap-2 text-sm xl:grid-cols-[minmax(0,2fr)_100px_100px_110px_100px_160px_160px] xl:items-center xl:gap-3'>
                    <div className='min-w-0 font-medium'>
                      <span className='break-all'>{row.actionName || '-'}</span>
                    </div>
                    <div>{formatCellValue(actionState.parallelism)}</div>
                    <div>{formatCellValue(actionState.subtaskCount)}</div>
                    <div>
                      {formatCellValue(actionState.coordinatorStateChunks)}
                    </div>
                    <div>
                      {formatCellValue(statistics.acknowledgedSubtasks)}
                    </div>
                    <div>
                      {renderCheckpointFieldValue(
                        'latestAckTimestamp',
                        statistics.latestAckTimestamp,
                      )}
                    </div>
                    <div>
                      {formatCellValue(row.sourceState?.decodeStrategy)}
                    </div>
                  </div>
                </CheckpointDetailsSummary>
                <div className='space-y-4 border-t border-border/50 p-3'>
                  <CheckpointInspectInlineTable
                    title='Subtasks Summary'
                    columns={[
                      {key: 'metric', label: 'Metric'},
                      {key: 'Splits', label: 'Splits'},
                      {key: 'Bytes', label: 'Bytes'},
                      {key: 'Chunks', label: 'Chunks'},
                      {key: 'State Size', label: 'State Size'},
                    ]}
                    rows={subtaskSummaryRows}
                  />

                  <CheckpointInspectInlineTable
                    title='Subtasks'
                    columns={[
                      {key: 'subtaskIndex', label: 'Subtask'},
                      {key: 'splitCount', label: 'Splits'},
                      {key: 'bytes', label: 'Bytes'},
                      {key: 'chunks', label: 'Chunks'},
                      {key: 'stateSize', label: 'State Size'},
                      {key: 'status', label: 'Status'},
                      {key: 'ackTimestamp', label: 'Ack Timestamp'},
                    ]}
                    rows={subtaskRows as unknown as Record<string, unknown>[]}
                  />

                  {row.sourceState ? (
                    <details className='group rounded-md border border-border/50 bg-muted/5'>
                      <CheckpointDetailsSummary className='py-3 text-sm font-medium'>
                        <span>
                          {sourceStateTitle} / {rawDetailsLabel}
                        </span>
                      </CheckpointDetailsSummary>
                      <div className='space-y-3 border-t border-border/50 p-3'>
                        <CheckpointInspectMiniObject
                          title={coordinatorLabel}
                          value={
                            row.sourceState.coordinator as
                              | Record<string, unknown>
                              | undefined
                          }
                        />
                        {sourceSubtasks.map((subtask, subtaskIndex) => {
                          const splits = Array.isArray(subtask.splits)
                            ? (subtask.splits as Record<string, unknown>[])
                            : [];
                          return (
                            <div
                              key={`${row.key}-split-group-${subtaskIndex}`}
                              className='rounded-md border border-border/50 p-3'
                            >
                              <div className='mb-3 text-sm font-medium'>
                                Subtask {formatCellValue(subtask.subtaskIndex)}
                              </div>
                              <div className='space-y-2'>
                                {splits.length > 0 ? (
                                  splits.map((split, splitIndex) => (
                                    <CheckpointInspectMiniObject
                                      key={`${row.key}-split-${subtaskIndex}-${splitIndex}`}
                                      title={`Split ${splitIndex + 1}`}
                                      value={split}
                                    />
                                  ))
                                ) : (
                                  <div className='text-sm text-muted-foreground'>
                                    -
                                  </div>
                                )}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </details>
                  ) : row.unsupportedSource ? (
                    <CheckpointInspectMiniObject
                      title={unsupportedLabel}
                      value={row.unsupportedSource}
                    />
                  ) : null}
                </div>
              </details>
            );
          })}
        </div>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </CheckpointInspectSectionShell>
  );
}

function CheckpointInspectMetricCard({
  label,
  value,
  valueKey,
}: {
  label: string;
  value: unknown;
  valueKey?: string;
}) {
  return (
    <div className='rounded-lg border border-border/60 bg-muted/10 p-3'>
      <div className='text-xs text-muted-foreground'>{label}</div>
      <div className='mt-1 break-all text-sm font-medium'>
        {renderCheckpointFieldValue(valueKey || label, value)}
      </div>
    </div>
  );
}

function CheckpointInspectInlineTable({
  title,
  columns,
  rows,
}: {
  title: string;
  columns: Array<{key: string; label: string}>;
  rows: Record<string, unknown>[];
}) {
  return (
    <div className='rounded-md border border-border/50'>
      <div className='border-b border-border/50 px-3 py-2 text-xs font-medium text-muted-foreground'>
        {title}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((column) => (
              <TableHead key={column.key}>{column.label}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length > 0 ? (
            rows.map((row, rowIndex) => (
              <TableRow key={`${title}-${rowIndex}`}>
                {columns.map((column) => (
                  <TableCell key={column.key}>
                    {renderCheckpointFieldValue(column.key, row[column.key])}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell
                colSpan={columns.length}
                className='text-center text-muted-foreground'
              >
                -
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function CheckpointInspectMiniObject({
  title,
  value,
}: {
  title: string;
  value?: Record<string, unknown> | null;
}) {
  const entries = value ? Object.entries(value) : [];
  return (
    <div className='rounded-md border border-border/50 p-3'>
      <div className='mb-2 text-xs font-medium text-muted-foreground'>
        {title}
      </div>
      {entries.length > 0 ? (
        <div className='grid gap-2 md:grid-cols-2'>
          {entries.map(([key, entryValue]) => (
            <div
              key={key}
              className='rounded border border-border/40 bg-muted/10 p-2'
            >
              <div className='text-[11px] text-muted-foreground'>{key}</div>
              <div className='mt-1 break-all text-sm'>
                {renderCheckpointFieldValue(key, entryValue)}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </div>
  );
}

function ValidationResultPanel({result}: {result: SyncValidateResult | null}) {
  const t = useTranslations('workbenchStudio');
  if (!result) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noValidationResults')}
      </div>
    );
  }
  const checks = result.checks || [];
  return (
    <div className='grid max-h-[70vh] gap-4 overflow-auto pr-1 lg:grid-cols-[minmax(0,1fr)_360px]'>
      <div className='space-y-4'>
        <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
          <div className='flex items-center justify-between gap-3'>
            <div className='text-sm font-medium'>{t('conclusion')}</div>
            <Badge
              variant='outline'
              className={cn(
                'rounded-sm border px-2 py-0.5 text-[11px]',
                result.valid
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
                  : 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
              )}
            >
              {result.valid ? t('passed') : t('notPassed')}
            </Badge>
          </div>
          <div className='mt-2 text-sm text-muted-foreground'>
            {result.summary}
          </div>
        </div>

        <div className='grid gap-4 lg:grid-cols-2'>
          <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
            <div className='mb-3 text-sm font-medium'>{t('errors')}</div>
            {result.errors.length > 0 ? (
              <div className='space-y-2'>
                {result.errors.map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive'
                  >
                    {item}
                  </div>
                ))}
              </div>
            ) : (
              <div className='text-sm text-muted-foreground'>
                {t('noErrors')}
              </div>
            )}
          </div>
          <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
            <div className='mb-3 text-sm font-medium'>{t('warnings')}</div>
            {result.warnings.length > 0 ? (
              <div className='space-y-2'>
                {result.warnings.map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
                  >
                    {item}
                  </div>
                ))}
              </div>
            ) : (
              <div className='text-sm text-muted-foreground'>
                {t('noWarnings')}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
        <div className='mb-3 text-sm font-medium'>{t('connectionChecks')}</div>
        {checks.length > 0 ? (
          <div className='space-y-3'>
            {checks.map((check, index) => (
              <div
                key={`${check.node_id}-${check.connector_type}-${index}`}
                className='rounded-lg border border-border/50 bg-muted/15 p-3'
              >
                <div className='flex items-start justify-between gap-3'>
                  <div className='space-y-1'>
                    <div className='text-sm font-medium'>
                      {check.connector_type}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {check.node_id}
                    </div>
                  </div>
                  <Badge
                    variant='outline'
                    className={cn(
                      'rounded-sm capitalize',
                      check.status === 'success'
                        ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
                        : check.status === 'failed'
                          ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300'
                          : 'border-slate-200 bg-slate-50 text-slate-700 dark:border-slate-500/30 dark:bg-slate-500/10 dark:text-slate-300',
                    )}
                  >
                    {check.status}
                  </Badge>
                </div>
                {check.target ? (
                  <div className='mt-2 break-all rounded-md bg-muted/30 px-2 py-1 font-mono text-[11px] text-muted-foreground'>
                    {check.target}
                  </div>
                ) : null}
                <div className='mt-2 text-sm text-muted-foreground'>
                  {check.message}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className='text-sm text-muted-foreground'>
            {t('noConnectionChecks')}
          </div>
        )}
      </div>
    </div>
  );
}

function MetricsDialogContent({job}: {job: SyncJobInstance | null}) {
  const t = useTranslations('workbenchStudio');
  const rawMetrics = toObject(job?.result_preview?.metrics);
  const metricGroups = buildMetricGroups(rawMetrics, t).filter(
    (group) => group.key !== 'read' && group.key !== 'write',
  );
  const metricHighlights = buildMetricHighlights(rawMetrics, t);
  const perTableRows = buildPerTableMetricRows(rawMetrics);
  const pairedMetricRows = buildPairedMetricRows(rawMetrics);
  const shouldExpandPerTableByDefault = pairedMetricRows.length === 0;
  const [perTableExpanded, setPerTableExpanded] = useState(
    shouldExpandPerTableByDefault,
  );
  useEffect(() => {
    setPerTableExpanded(shouldExpandPerTableByDefault);
  }, [job?.id, shouldExpandPerTableByDefault]);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noMetrics')}</div>
    );
  }
  if (metricGroups.length === 0) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noMetricsOutput')}
      </div>
    );
  }
  return (
    <div className='space-y-4 overflow-auto pr-1'>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {metricHighlights.map((item) => (
          <div
            key={item.label}
            className='rounded-lg border border-border/60 bg-background/80 p-4'
          >
            <div className='text-xs text-muted-foreground'>{item.label}</div>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className='mt-2 text-2xl font-semibold tracking-tight'>
                  {item.value}
                </div>
              </TooltipTrigger>
              <TooltipContent>{item.raw}</TooltipContent>
            </Tooltip>
          </div>
        ))}
      </div>

      {perTableRows.length > 0 ? (
        <div className='overflow-hidden rounded-lg border border-border/60 bg-background/80'>
          {pairedMetricRows.length > 0 ? (
            <>
              <div className='border-b border-border/50 bg-muted/20 px-3 py-2 text-sm font-medium'>
                {t('metricMappedView')}
              </div>
              <div className='max-h-[240px] overflow-auto border-b border-border/50'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow>
                      <TableHead>{t('metricPairSourceNode')}</TableHead>
                      <TableHead>{t('metricPairSourceTable')}</TableHead>
                      <TableHead>{t('metricPairSinkNode')}</TableHead>
                      <TableHead>{t('metricPairSinkTable')}</TableHead>
                      <TableHead>{t('metricSourceRows')}</TableHead>
                      <TableHead>{t('metricSourceBytes')}</TableHead>
                      <TableHead>{t('metricSourceQps')}</TableHead>
                      <TableHead>{t('metricSinkRows')}</TableHead>
                      <TableHead>{t('metricSinkBytes')}</TableHead>
                      <TableHead>{t('metricSinkQps')}</TableHead>
                      <TableHead>{t('metricCommittedRows')}</TableHead>
                      <TableHead>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pairedMetricRows.map((row) => (
                      <TableRow key={row.key}>
                        <TableCell className='text-xs font-medium'>
                          {row.sourceNode}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.sourceTable}
                        </TableCell>
                        <TableCell className='text-xs font-medium'>
                          {row.sinkNode}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.sinkTable}
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedCount}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedBytes}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : null}
          <div className='flex items-center justify-between gap-2 border-b border-border/50 bg-muted/20 px-3 py-2'>
            <div className='text-sm font-medium'>{t('metricPerTable')}</div>
            {pairedMetricRows.length > 0 ? (
              <button
                type='button'
                className='inline-flex items-center gap-1 rounded-md border border-border/60 bg-background px-2 py-1 text-xs text-muted-foreground hover:text-foreground'
                onClick={() => setPerTableExpanded((value) => !value)}
              >
                {perTableExpanded ? (
                  <ChevronDown className='size-3.5' />
                ) : (
                  <ChevronRight className='size-3.5' />
                )}
                <span>
                  {perTableExpanded
                    ? t('collapsePerTableMetrics')
                    : t('expandPerTableMetrics')}
                </span>
              </button>
            ) : null}
          </div>
          {perTableExpanded ? (
            <>
              <div className='flex flex-wrap gap-2 border-b border-border/50 bg-background px-3 py-2 text-xs'>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-emerald-500' />
                  <span>{t('metricLegendSource')}</span>
                </div>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-blue-500' />
                  <span>{t('metricLegendWrite')}</span>
                </div>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-amber-500' />
                  <span>{t('metricLegendCommitted')}</span>
                </div>
              </div>
              <div className='max-h-[320px] overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow>
                      <TableHead>{t('node')}</TableHead>
                      <TableHead>{t('table')}</TableHead>
                      <TableHead>{t('metricSourceRows')}</TableHead>
                      <TableHead>{t('metricSourceBytes')}</TableHead>
                      <TableHead>{t('metricSourceQps')}</TableHead>
                      <TableHead>{t('metricSinkRows')}</TableHead>
                      <TableHead>{t('metricSinkBytes')}</TableHead>
                      <TableHead>{t('metricSinkQps')}</TableHead>
                      <TableHead>{t('metricCommittedRows')}</TableHead>
                      <TableHead>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {perTableRows.map((row) => (
                      <TableRow
                        key={row.rawTable}
                        className={cn(
                          row.rowTone === 'source' &&
                            'bg-emerald-50/50 dark:bg-emerald-500/5',
                          row.rowTone === 'sink' &&
                            'bg-blue-50/50 dark:bg-blue-500/5',
                        )}
                      >
                        <TableCell className='text-xs font-medium'>
                          {row.nodeLabel}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.tablePath}
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedCount}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedBytes}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : (
            <div className='px-3 py-3 text-sm text-muted-foreground'>
              {t('perTableMetricsCollapsed')}
            </div>
          )}
        </div>
      ) : null}

      <div className='grid gap-4 lg:grid-cols-2'>
        {metricGroups.map((group) => (
          <div
            key={group.key}
            className='overflow-hidden rounded-lg border border-border/60 bg-background/80'
          >
            <div className='border-b border-border/50 bg-muted/20 px-3 py-2 text-sm font-medium'>
              {group.title}
            </div>
            <div className='max-h-[360px] overflow-auto'>
              <Table>
                <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                  <TableRow>
                    <TableHead>{t('metric')}</TableHead>
                    <TableHead>{t('value')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {group.items.map((item) => (
                    <TableRow key={item.key}>
                      <TableCell className='font-mono text-xs'>
                        {item.key}
                      </TableCell>
                      <TableCell className='text-xs'>
                        {formatMetricDisplayValue(item.value)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function JobScriptDialogContent({
  job,
  monacoTheme,
}: {
  job: SyncJobInstance | null;
  monacoTheme: string;
}) {
  const t = useTranslations('workbenchStudio');
  const script = getJobSubmittedScript(job);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noJobRuns')}</div>
    );
  }
  if (!script) {
    return (
      <div className='rounded-md border border-dashed border-border/60 bg-muted/20 px-4 py-6 text-sm text-muted-foreground'>
        {t('noActualExecutedScript')}
      </div>
    );
  }
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <div className='flex items-center gap-2 text-xs text-muted-foreground'>
        <Badge variant='outline'>#{job.id}</Badge>
        <Badge variant='outline'>{script.format || 'hocon'}</Badge>
        <span className='truncate'>
          {job.platform_job_id || job.engine_job_id || '-'}
        </span>
      </div>
      <div className='min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
        <MonacoEditor
          height='100%'
          theme={monacoTheme}
          language={script.format === 'json' ? 'json' : 'shell'}
          value={script.content}
          options={{
            readOnly: true,
            minimap: {enabled: false},
            automaticLayout: true,
            fontSize: 13,
            scrollBeyondLastLine: false,
            wordWrap: 'on',
          }}
        />
      </div>
    </div>
  );
}

function VirtualizedLogViewer({
  lines,
  height,
  emptyText,
}: {
  lines: string[];
  height: number;
  emptyText: string;
}) {
  const rowHeight = 20;
  const overscan = 24;
  const [scrollTop, setScrollTop] = useState(0);
  const startIndex = Math.max(Math.floor(scrollTop / rowHeight) - overscan, 0);
  const visibleCount = Math.ceil(height / rowHeight) + overscan * 2;
  const endIndex = Math.min(startIndex + visibleCount, lines.length);
  const visibleLines = lines.slice(startIndex, endIndex);
  if (lines.length === 0) {
    return (
      <div className='rounded-lg border border-border/60 bg-background/80 p-4 text-sm text-muted-foreground'>
        {emptyText}
      </div>
    );
  }
  return (
    <div
      className='overflow-auto rounded-lg border border-border/60 bg-background/80 p-0 font-mono text-xs'
      style={{height}}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <div style={{height: lines.length * rowHeight, position: 'relative'}}>
        <div
          style={{
            position: 'absolute',
            top: startIndex * rowHeight,
            left: 0,
            right: 0,
          }}
          className='px-4 py-3'
        >
          {visibleLines.map((line, index) => (
            <div
              key={`${startIndex + index}-${line.slice(0, 24)}`}
              className={cn('h-5 whitespace-pre', getLogLineClass(line))}
            >
              {line || ' '}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
