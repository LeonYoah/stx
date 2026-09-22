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
  fillPreferredClusterId,
  rememberPreferredClusterId,
} from '@/lib/cluster-preference';
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
  AlertTriangle,
  Bug,
  Check,
  Copy,
  ChevronDown,
  ChevronRight,
  ChevronsUpDown,
  Columns2,
  ExternalLink,
  Cpu,
  Folder,
  FolderOpen,
  FolderPlus,
  FileCode2,
  FilePlus2,
  GitBranch,
  GitCommit,
  BarChart3,
  Layers,
  Maximize2,
  Minimize2,
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
  Lock,
  Shield,
  Users,
  UserCheck,
  Share2,
  Workflow,
  WandSparkles,
  Code2,
  HelpCircle,
  X,
} from 'lucide-react';
import {useAuth} from '@/hooks/use-auth';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
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
  BUILTIN_TIME_VARIABLE_ITEMS,
  resolveBuiltinPreviewExpression,
} from './builtin-time-variables';
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


import {
  SidebarIconTab,
  StudioSidebarShell,
} from './StudioSidebarShell';
import {
  SettingsSidebarPanel,
  TemplatePluginSelect,
} from './SettingsSidebarPanel';
import {
  VersionSidebarPanel,
  SimplePagination,
} from './VersionSidebarPanel';
import {
  ConsolePanel,
  JobRunsPanel,
  MixedLogModeBanner,
} from './ConsolePanel';
import {
  PreviewWorkspacePanel,
  CheckpointWorkspacePanel,
  CheckpointInspectDialog,
} from './CheckpointPanels';
import {
  ValidationResultPanel,
  StudioErrorDiagnosticsView,
  MetricsDialogContent,
  JobScriptDialogContent,
  VirtualizedLogViewer,
} from './StudioDialogs';
import {
  TreeView,
} from './TreeView';
import { StudioTreeContextMenu } from './StudioTreeContextMenu';
import { StudioSharePopover } from './StudioSharePopover';
import { StudioQuickOpenDialog } from './StudioQuickOpenDialog';
import {
  type BottomConsoleTab,
  type CheckpointActionViewModel,
  type CheckpointSourceSummary,
  type CheckpointSubtaskViewRow,
  type EditorDraftState,
  type EditorState,
  type ExecutionMode,
  type LogFilterMode,
  type MetricGroupKey,
  type OpenFileTab,
  type OptionMetadataMap,
  type PendingActionKind,
  type PersistedWorkspaceTabs,
  type PluginEnumCatalogMap,
  type PreviewRunDialogState,
  type RightSidebarTab,
  type TemplatePluginItem,
  type TreeContextMenuState,
  type TreeDialogState,
  type TreeFilterScope,
  type UserFacingErrorState,
  type VariableDraft,
  type VariableRow,
  EMPTY_EDITOR,
  ENV_OPTION_METADATA,
  EXPANDED_LOG_CHUNK_BASE_BYTES,
  EXPANDED_LOG_CHUNK_MAX_BYTES,
  LOG_CHUNK_BASE_BYTES,
  LOG_CHUNK_MAX_BYTES,
  WORKSPACE_NAME_PATTERN,
  WORKSPACE_TABS_STORAGE_KEY,
  buildCheckpointActionViewModels,
  buildCheckpointInspectJobConfig,
  buildCheckpointInspectSummary,
  buildCheckpointSubtaskRows,
  buildCopiedWorkspaceName,
  buildDefaultContent,
  buildDisplayLogLines,
  buildInsertedTemplateContent,
  buildMetricGroups,
  buildMetricHighlights,
  buildPairedMetricRows,
  buildPerTableMetricRows,
  buildTemplatePluginItems,
  canRecoverFromJob,
  classifyMetricGroup,
  collectFolderIds,
  detectVariables,
  ensureSyncHoconLanguage,
  extractCheckpointFileIdentity,
  extractEditorState,
  extractEditorStateFromTreeNode,
  extractJobMetricSummary,
  extractPreviewColumns,
  extractPreviewDatasets,
  extractPreviewRows,
  extractTaskScheduleValue,
  extractVariableRowsFromDefinition,
  filterTree,
  findTopLevelBlockInsertOffset,
  findTreeNode,
  flattenTree,
  formatCellValue,
  formatCheckpointFieldValue,
  formatJobDateTime,
  formatJobDuration,
  formatMetadataValue,
  formatMetricCompactValue,
  formatMetricDisplayValue,
  formatMetricValue,
  formatMetricWithUnit,
  formatSizeBytes,
  formatSyncUserFacingError,
  fromVariableRows,
  fromVariableTypes,
  getCheckpointEnumBadgeClass,
  getCheckpointStatusBadgeClass,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getEngineEndpointLabel,
  getExecutionMode,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getJobSubmittedScript,
  getLogLineClass,
  getMetricValue,
  getNodeBreadcrumbSegments,
  getPendingActionLabel,
  getPreviewRowKindBadgeClass,
  getRunModeLabel,
  getSyncJobClusterId,
  hasDuplicateWorkspaceName,
  isCursorInsideValueRegion,
  isEditorDraftDirty,
  isJobLifecycleActive,
  isJobLifecycleTerminal,
  isReservedBuiltinVariableKey,
  isTreeDescendant,
  listMoveTargets,
  listSiblingNames,
  nextLogChunkSize,
  normalizeCheckpointActionIdentity,
  normalizeEditorForCompare,
  normalizeJobLifecycleStatus,
  normalizePairingTableKey,
  normalizePluginIdentity,
  normalizeStoredScriptContent,
  normalizeVariableRowsForCompare,
  parseMetricNumber,
  patchTreeNode,
  resolveDefaultPreviewHTTPSinkURL,
  resolveEditorPluginContext,
  resolveEnumSuggestRange,
  resolveEnumSuggestionItems,
  resolveEnumValueBounds,
  resolveFolderParent,
  resolveOptionAssignmentContext,
  resolveOptionKeyFromLine,
  resolveVariableCompletionContext,
  resolveVariableSuggestions,
  splitLogLines,
  submitSpecExecutionMode,
  summarizeCheckpointSourceState,
  summarizeCheckpointSubtaskMetrics,
  toCheckpointNumber,
  toObject,
  toStringMetricMap,
  toVariableRows,
  validateCustomVariableRows,
  validateWorkspaceName,
} from './sync-studio-utils';
export function DataSyncStudio() {
  const t = useTranslations('workbenchStudio');
  const {resolvedTheme} = useTheme();
  const {user: currentUser} = useAuth();
  const [workspaceUsers, setWorkspaceUsers] = useState<
    NotificationRecipientUser[]
  >([]);

  useEffect(() => {
    void services.monitoring
      .listNotifiableUsers()
      .then((data) => {
        if (Array.isArray(data.users)) {
          setWorkspaceUsers(data.users);
        }
      })
      .catch(() => {});
  }, []);

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
  const [treeFilterScope, setTreeFilterScope] = useState<TreeFilterScope>(() => {
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('stx_studio_tree_filter_scope');
      if (saved === 'all' || saved === 'mine_and_public' || saved === 'only_mine') {
        return saved;
      }
    }
    return 'mine_and_public';
  });
  const handleFilterScopeChange = useCallback((scope: TreeFilterScope) => {
    setTreeFilterScope(scope);
    try {
      localStorage.setItem('stx_studio_tree_filter_scope', scope);
    } catch {}
  }, []);
  // VSCode Quick Open 快捷检索浮层状态 (Cmd+P)
  // VSCode Quick Open command palette state (Cmd+P)
  const [quickOpenOpen, setQuickOpenOpen] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<number | null>(null);
  const [selectedFolderId, setSelectedFolderId] = useState<number | null>(null);
  // 树节点行内重命名、新建与拖拽状态（VSCode 风格交互）
  // Tree node inline rename, creation, and drag-drop states (VSCode-style interaction)
  const [renamingNodeId, setRenamingNodeId] = useState<number | null>(null);
  const [creatingNode, setCreatingNode] = useState<{
    parentId: number | null;
    nodeType: 'file' | 'folder';
  } | null>(null);
  const [draggingNodeId, setDraggingNodeId] = useState<number | null>(null);
  const [dragOverFolderId, setDragOverFolderId] = useState<number | null>(null);
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
  const allGlobalVariablesRef = useRef<SyncGlobalVariable[]>([]);
  const [versionTotal, setVersionTotal] = useState(0);
  const [globalVariableTotal, setGlobalVariableTotal] = useState(0);
  const [versionPage, setVersionPage] = useState(1);
  const [globalVariablePage, setGlobalVariablePage] = useState(1);
  const [rightSidebarTab, setRightSidebarTab] =
    useState<RightSidebarTab>('settings');
  // 全局变量侧边栏默认激活 Tab（'all' 全部 | 'time' 内置时间 | 'string' 文本 | 'secret' 保密）
  // Global variables sidebar default active Tab ('all' All | 'time' Built-in Time | 'string' Text | 'secret' Secret)
  const [globalVariablesDefaultTab, setGlobalVariablesDefaultTab] =
    useState<'all' | 'string' | 'time' | 'secret'>('all');
  const [scheduleDialogOpen, setScheduleDialogOpen] = useState(false);
  const [scheduleDraft, setScheduleDraft] = useState<TaskScheduleValue>(() =>
    extractTaskScheduleValue({}),
  );
  const [publishDialogOpen, setPublishDialogOpen] = useState(false);
  const [publishComment, setPublishComment] = useState('');
  const [publishing, setPublishing] = useState(false);
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

  // 底部控制台全屏与还原状态
  // Full-screen and restore state for the bottom console
  const [isConsoleMaximized, setIsConsoleMaximized] = useState(false);

  // 底部控制台高度档位：默认 260px，展开 390px
  // Bottom console height mode: default 260px, expanded 390px
  const [isConsoleExpanded, setIsConsoleExpanded] = useState(false);

  // 当前激活任务的面包屑路径列表
  // Breadcrumb segments for the currently active task
  const activeTaskBreadcrumbs = useMemo(
    () => (editor.id ? getNodeBreadcrumbSegments(tree, editor.id) : []),
    [editor.id, tree],
  );

  // 当前任务绑定的集群信息（用于环境指示胶囊）
  // Cluster information associated with current task (used in environment indicator capsule)
  const currentCluster = useMemo(
    () => clusters.find((c) => c.id === Number(editor.clusterId)) || null,
    [clusters, editor.clusterId],
  );

  // 当前编辑器文件是否存在未保存改动
  // Whether current active editor has unsaved changes
  const isCurrentDirty = Boolean(editor.id && editorDrafts[editor.id]?.dirty);

  // 存在未保存草稿的任务 ID 集合
  // Set of task IDs that have unsaved drafts
  const dirtyNodeIds = useMemo(
    () =>
      Object.entries(editorDrafts)
        .filter(([, draft]) => draft?.dirty)
        .map(([id]) => Number(id)),
    [editorDrafts],
  );

  // 收集所有文件夹 ID，用于一键折叠或展开全部目录
  // Collect all folder IDs for collapse-all / expand-all toggles
  const allFolderIds = useMemo(() => {
    return flattenTree(tree)
      .filter((n) => n.node_type === 'folder')
      .map((n) => n.id);
  }, [tree]);

  // 一键折叠或展开全部目录
  // Toggle collapse or expand all folders in the explorer
  const handleToggleCollapseAll = useCallback(() => {
    setExpandedFolderIds((prev) => (prev.length > 0 ? [] : allFolderIds));
  }, [allFolderIds]);

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
  const tabButtonRefs = useRef<Record<number, HTMLElement | null>>({});

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
    () => filterTree(tree, keyword, treeFilterScope, currentUser?.id),
    [tree, keyword, treeFilterScope, currentUser?.id],
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

  // 唯一集群或已记住偏好时，自动填入编辑器空的集群选择，避免各处重复手选。
  // Auto-fill empty editor cluster from sole/default or remembered preference.
  useEffect(() => {
    if (clusters.length === 0) {
      return;
    }
    setEditor((prev) => {
      const nextClusterId = fillPreferredClusterId(prev.clusterId, clusters);
      if (!nextClusterId || nextClusterId === prev.clusterId) {
        return prev;
      }
      return {...prev, clusterId: nextClusterId};
    });
  }, [clusters]);

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
        const loadedClusters = clusterData.clusters || [];
        setClusters(loadedClusters);
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
          const preferredClusterId = fillPreferredClusterId(
            loadedEditor.clusterId,
            loadedClusters,
          );
          const editorWithPreferred =
            preferredClusterId && preferredClusterId !== loadedEditor.clusterId
              ? {...loadedEditor, clusterId: preferredClusterId}
              : loadedEditor;
          const loadedRows = extractVariableRowsFromDefinition(
            task.definition || {},
          );
          const usedDraft = applyDraftOrLoadedState(
            nextSelected,
            editorWithPreferred,
            loadedRows,
          );
          if (!usedDraft) {
            markEditorDraft(
              nextSelected,
              editorWithPreferred,
              loadedRows,
              false,
            );
          }
          syncOpenTabs(task);
        } else {
          const preferredClusterId = fillPreferredClusterId('', loadedClusters);
          setEditor(
            preferredClusterId
              ? {...EMPTY_EDITOR, clusterId: preferredClusterId}
              : EMPTY_EDITOR,
          );
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
      const [data, allData] = await Promise.all([
        services.sync.listGlobalVariables({
          current: globalVariablePage,
          size: 8,
        }),
        services.sync.listGlobalVariables({
          current: 1,
          size: 1000,
        }),
      ]);
      setGlobalVariables(data.items || []);
      setGlobalVariableTotal(data.total || 0);
      allGlobalVariablesRef.current = allData.items || [];
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('loadGlobalVariablesFailed'),
      );
    }
  }, [globalVariablePage, t]);

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
          const isBoolean =
            payload.value === 'true' || payload.value === 'false';
          let renderedValue = payload.value;
          if (bounds.quoted) {
            renderedValue = payload.value;
          } else if (isBoolean) {
            renderedValue = payload.value;
          } else {
            renderedValue = /^[A-Za-z0-9_.-]+$/.test(payload.value)
              ? payload.value
              : JSON.stringify(payload.value);
          }
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
          editor.setPosition?.({
            lineNumber: payload.lineNumber,
            column: bounds.startColumn + renderedValue.length,
          });
        },
      );
      enumCompletionCommandRegisteredRef.current = true;
    }
    const completionProvider = {
      triggerCharacters: ['=', ' ', '"', '{', '}'],
      provideCompletionItems: async (
        model: any,
        position: {lineNumber: number; column: number},
      ) => {
        const lineContent = model.getLineContent(position.lineNumber);

        // 1. 严格仅在 {{ 变量占位符内部（如 {{、{{}}、键入 {{ 后的标识符模糊匹配）才触发变量建议
        const varCtx = resolveVariableCompletionContext(
          lineContent,
          position.column,
        );
        if (varCtx.inVariable) {
          const suggestions = resolveVariableSuggestions(
            monaco,
            varCtx,
            customVariableRowsRef.current,
            allGlobalVariablesRef.current,
            position,
          );
          return {suggestions};
        }

        // 2. 检测属性赋值上下文（仅针对支持的属性提供枚举提示，普通内容区域不干扰弹窗）
        const assignmentCtx = resolveOptionAssignmentContext(
          lineContent,
          position.column,
        );
        if (!assignmentCtx.inValueRegion || !assignmentCtx.optionKey) {
          return {suggestions: []};
        }
        const optionKey = assignmentCtx.optionKey;
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
        const currentValue = assignmentCtx.bounds?.value || '';
        const isSittingOnEnumValue = enumValues.some(
          (val) => val.toLowerCase() === currentValue.toLowerCase(),
        );

        return {
          suggestions: enumItems.map((item, index) => {
            const filterText = isSittingOnEnumValue || !currentWord
              ? [currentWord, item.label, item.value].filter(Boolean).join(' ')
              : [item.label, item.value].filter(Boolean).join(' ');

            return {
              label: item.label,
              detail:
                item.label !== item.value ? `插入值: ${item.value}` : undefined,
              kind: monaco.languages.CompletionItemKind.EnumMember,
              insertText: '',
              filterText,
              sortText: String(index).padStart(4, '0'),
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
            };
          }),
        };
      },
    };

    const d1 = monaco.languages.registerCompletionItemProvider(
      'sync-hocon',
      completionProvider,
    );
    const d2 = monaco.languages.registerCompletionItemProvider(
      'json',
      completionProvider,
    );
    completionDisposableRef.current = {
      dispose: () => {
        d1?.dispose?.();
        d2?.dispose?.();
      },
    };
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
          const lineContent = model.getLineContent(position.lineNumber);
          const inValueRegion = isCursorInsideValueRegion(
            lineContent,
            position.column,
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
        is_public:
          editor.definition?.is_public !== undefined
            ? editor.definition.is_public
            : (editor.isPublic ?? true),
        collaborators: Array.isArray(editor.definition?.collaborators)
          ? editor.definition.collaborators
          : Array.isArray(editor.definition?.collaborator_ids)
            ? editor.definition.collaborator_ids
            : [],
        collaborator_ids: Array.isArray(editor.definition?.collaborator_ids)
          ? editor.definition.collaborator_ids
          : Array.isArray(editor.definition?.collaborators)
            ? editor.definition.collaborators
            : [],
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

  // ==========================================
  // 树节点 VSCode 风格操作处理函数
  // Tree Node VSCode-Style Action Handlers
  // ==========================================

  const handleRenameStart = useCallback((node: SyncTaskTreeNode) => {
    setRenamingNodeId(node.id);
  }, []);

  const handleRenameCancel = useCallback(() => {
    setRenamingNodeId(null);
  }, []);

  const handleRenameCommit = useCallback(
    async (node: SyncTaskTreeNode, newName: string) => {
      const name = newName.trim();
      const nameError = validateWorkspaceName(name, t);
      if (nameError) {
        toast.error(nameError);
        return;
      }
      const siblingParentId = node.parent_id == null ? null : node.parent_id;
      if (hasDuplicateWorkspaceName(tree, siblingParentId, name, node.id)) {
        toast.error(t('duplicateWorkspaceName'));
        return;
      }
      try {
        const current = await services.sync.getTask(node.id);
        await services.sync.updateTask(node.id, {
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
        setRenamingNodeId(null);
        await loadWorkspace(node.id);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('operationFailed'),
        );
      }
    },
    [loadWorkspace, t, tree],
  );

  const handleInlineCreateFile = useCallback((parentNode: SyncTaskTreeNode) => {
    setCreatingNode({ parentId: parentNode.id, nodeType: 'file' });
    setExpandedFolderIds((prev) =>
      prev.includes(parentNode.id) ? prev : [...prev, parentNode.id],
    );
  }, []);

  const handleInlineCreateFolder = useCallback(
    (parentNode: SyncTaskTreeNode | null) => {
      setCreatingNode({ parentId: parentNode?.id || null, nodeType: 'folder' });
      if (parentNode) {
        setExpandedFolderIds((prev) =>
          prev.includes(parentNode.id) ? prev : [...prev, parentNode.id],
        );
      }
    },
    [],
  );

  const handleInlineCreateCancel = useCallback(() => {
    setCreatingNode(null);
  }, []);

  const handleInlineCreateCommit = useCallback(
    async (name: string) => {
      if (!creatingNode) return;
      const trimmed = name.trim();
      const nameError = validateWorkspaceName(trimmed, t);
      if (nameError) {
        toast.error(nameError);
        return;
      }
      const parentId = creatingNode.parentId;
      if (creatingNode.nodeType === 'file' && !parentId) {
        toast.error(t('rootFileCreationBlocked'));
        return;
      }
      if (hasDuplicateWorkspaceName(tree, parentId, trimmed)) {
        toast.error(t('duplicateWorkspaceName'));
        return;
      }
      try {
        if (creatingNode.nodeType === 'folder') {
          await services.sync.createTask({
            parent_id: parentId || undefined,
            node_type: 'folder',
            name: trimmed,
            content_format: 'hocon',
          });
          toast.success(t('folderCreated'));
          setCreatingNode(null);
          await loadWorkspace(selectedNodeId);
        } else {
          const task = await services.sync.createTask({
            parent_id: parentId || undefined,
            node_type: 'file',
            name: trimmed,
            cluster_id: editor.clusterId ? Number(editor.clusterId) : 0,
            content_format: 'hocon',
            content: buildDefaultContent('hocon'),
            definition: {},
          });
          toast.success(t('fileCreated'));
          setCreatingNode(null);
          syncOpenTabs(task);
          await loadWorkspace(task.id);
        }
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('operationFailed'),
        );
      }
    },
    [creatingNode, editor.clusterId, loadWorkspace, selectedNodeId, syncOpenTabs, t, tree],
  );

  const handleInlineDelete = useCallback(
    (node: SyncTaskTreeNode) => {
      openTreeDialog('delete', node);
    },
    [openTreeDialog],
  );

  const handleDragStart = useCallback((node: SyncTaskTreeNode) => {
    setDraggingNodeId(node.id);
  }, []);

  const handleDragOver = useCallback(
    (targetFolder: SyncTaskTreeNode) => {
      if (!draggingNodeId || draggingNodeId === targetFolder.id) return;
      const draggedNode = findTreeNode(tree, draggingNodeId);
      if (!draggedNode) return;
      if (draggedNode.parent_id === targetFolder.id) return;
      if (
        draggedNode.node_type === 'folder' &&
        isTreeDescendant(tree, draggedNode.id, targetFolder.id)
      ) {
        return;
      }
      setDragOverFolderId(targetFolder.id);
    },
    [draggingNodeId, tree],
  );

  const handleDragLeave = useCallback(
    (targetFolder: SyncTaskTreeNode) => {
      if (dragOverFolderId === targetFolder.id) {
        setDragOverFolderId(null);
      }
    },
    [dragOverFolderId],
  );

  const handleDrop = useCallback(
    async (targetFolder: SyncTaskTreeNode) => {
      setDragOverFolderId(null);
      if (!draggingNodeId || draggingNodeId === targetFolder.id) return;
      const draggedNode = findTreeNode(tree, draggingNodeId);
      if (!draggedNode) return;
      if (draggedNode.parent_id === targetFolder.id) return;
      if (
        draggedNode.node_type === 'folder' &&
        isTreeDescendant(tree, draggedNode.id, targetFolder.id)
      ) {
        toast.error(t('cannotMoveToChild'));
        return;
      }
      if (hasDuplicateWorkspaceName(tree, targetFolder.id, draggedNode.name, draggedNode.id)) {
        toast.error(t('duplicateWorkspaceName'));
        return;
      }
      try {
        const current = await services.sync.getTask(draggedNode.id);
        await services.sync.updateTask(draggedNode.id, {
          parent_id: targetFolder.id,
          node_type: current.node_type,
          name: current.name,
          description: current.description,
          cluster_id: current.cluster_id,
          content_format: current.content_format,
          content: current.content,
          definition: current.definition,
        });
        toast.success(t('moveCompleted'));
        setDraggingNodeId(null);
        setExpandedFolderIds((prev) =>
          prev.includes(targetFolder.id) ? prev : [...prev, targetFolder.id],
        );
        await loadWorkspace(draggedNode.id);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('operationFailed'),
        );
      } finally {
        setDraggingNodeId(null);
        setDragOverFolderId(null);
      }
    },
    [draggingNodeId, loadWorkspace, t, tree],
  );

  const handleDragEnd = useCallback(() => {
    setDraggingNodeId(null);
    setDragOverFolderId(null);
  }, []);

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

  // 关闭其他打开的标签页
  // Close other open tabs except the target task ID
  const handleCloseOtherTabs = (keepTaskId: number) => {
    setOpenTabs((current) => current.filter((tab) => tab.id === keepTaskId));
    if (selectedNodeId !== keepTaskId) {
      void handleSelectTab(keepTaskId);
    }
  };

  // 关闭全部标签页并重置编辑器状态
  // Close all open tabs and reset editor workspace state
  const handleCloseAllTabs = () => {
    setOpenTabs([]);
    setSelectedNodeId(null);
    setEditor(EMPTY_EDITOR);
    setCustomVariableRows(extractVariableRowsFromDefinition({}));
    setJobs([]);
    setSelectedJobId(null);
    setJobLogs(null);
    setExpandedJobLogs(null);
    setCheckpointSnapshot(null);
  };

  const handleSave = async () => {
    if (editor.id && !editor.canEdit) {
      toast.error(t('readOnlySaveTooltip'));
      return;
    }
    const task = await persistCurrentFile(false);
    if (task) {
      toast.success(t('saveFileSuccess'));
    }
  };

  const handleOpenPublishDialog = () => {
    if (editor.id && !editor.canEdit) {
      toast.error(t('readOnlySaveTooltip'));
      return;
    }
    if (!editor.name.trim()) {
      toast.error(t('fileNameRequired'));
      return;
    }
    setPublishComment('');
    setPublishDialogOpen(true);
  };

  const handleConfirmPublish = async () => {
    if (publishing) {
      return;
    }
    setPublishing(true);
    try {
      // 先保存最新内容再发布版本
      // Save current file draft first, then create version snapshot
      const task = await persistCurrentFile(false);
      if (!task) {
        return;
      }
      await services.sync.publishTask(task.id, {
        comment: publishComment.trim() || 'publish from data sync studio',
      });
      const refreshedTask = await services.sync.getTask(task.id);
      const savedEditor = extractEditorState(refreshedTask);
      setEditor(savedEditor);
      setTree((current) => patchTreeNode(current, refreshedTask));
      await loadVersions(task.id);
      setPublishDialogOpen(false);
      toast.success(t('publishVersionSuccess'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('publishVersionFailed'),
      );
    } finally {
      setPublishing(false);
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
    if (editor.id && !editor.canEdit) {
      toast.error(t('readOnlyTestConnTooltip'));
      return;
    }
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
    if (editor.id && !editor.canRun) {
      toast.error(t('readOnlyPreviewTooltip'));
      return;
    }
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
    if (editor.id && !editor.canRun) {
      toast.error(t('readOnlyRunTooltip'));
      return;
    }
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
    async (payload: {
      key: string;
      value: string;
      type: 'string' | 'secret';
      description?: string;
    }) => {
      let nextRows: VariableRow[];
      const isEditing = Boolean(editingCustomVariable);
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
      } else {
        const newRow: VariableRow = {
          id: `custom-var-${Date.now()}`,
          key: payload.key.trim(),
          value: payload.value,
          type: payload.type,
          description: payload.description,
        };
        nextRows = [...customVariableRows, newRow];
      }
      syncCustomVariablesToEditor(nextRows);
      setCustomVariableDialogOpen(false);
      setEditingCustomVariable(null);

      // 若当前已打开已保存的任务文件，立即自动将自定义变量持久化存储到数据库
      // If current task is already persisted in DB, immediately sync and persist custom variables to database
      if (editor.id) {
        try {
          const taskPayload = {
            ...buildTaskPayload(),
            definition: {
              ...editor.definition,
              custom_variables: fromVariableRows(nextRows),
              custom_variable_types: fromVariableTypes(nextRows),
            },
          };
          const updatedTask = await services.sync.updateTask(
            editor.id,
            taskPayload,
          );
          const savedEditor = extractEditorState(updatedTask);
          const savedRows = extractVariableRowsFromDefinition(
            updatedTask.definition || {},
          );
          setEditor(savedEditor);
          setCustomVariableRows(savedRows);
          customVariableRowsRef.current = savedRows;
          markEditorDraft(updatedTask.id, savedEditor, savedRows, false);
          setTree((current) => patchTreeNode(current, updatedTask));
          toast.success(
            isEditing ? t('customVariableUpdated') : t('customVariableCreated'),
          );
        } catch (err) {
          toast.error(err instanceof Error ? err.message : t('saveFileFailed'));
        }
      } else {
        toast.success(
          isEditing ? t('customVariableUpdated') : t('customVariableCreated'),
        );
      }
    },
    [
      buildTaskPayload,
      customVariableRows,
      editingCustomVariable,
      editor,
      markEditorDraft,
      syncCustomVariablesToEditor,
      t,
    ],
  );

  // 删除自定义变量
  // Delete custom variable
  const handleDeleteCustomVariable = useCallback(
    async (rowId: string) => {
      const nextRows = customVariableRows.filter((row) => row.id !== rowId);
      syncCustomVariablesToEditor(nextRows);
      if (editingCustomVariable?.id === rowId) {
        setEditingCustomVariable(null);
        setCustomVariableDialogOpen(false);
      }

      if (editor.id) {
        try {
          const taskPayload = {
            ...buildTaskPayload(),
            definition: {
              ...editor.definition,
              custom_variables: fromVariableRows(nextRows),
              custom_variable_types: fromVariableTypes(nextRows),
            },
          };
          const updatedTask = await services.sync.updateTask(
            editor.id,
            taskPayload,
          );
          const savedEditor = extractEditorState(updatedTask);
          const savedRows = extractVariableRowsFromDefinition(
            updatedTask.definition || {},
          );
          setEditor(savedEditor);
          setCustomVariableRows(savedRows);
          customVariableRowsRef.current = savedRows;
          markEditorDraft(updatedTask.id, savedEditor, savedRows, false);
          setTree((current) => patchTreeNode(current, updatedTask));
          toast.success(t('customVariableDeleted'));
        } catch (err) {
          toast.error(err instanceof Error ? err.message : t('operationFailed'));
        }
      } else {
        toast.success(t('customVariableDeleted'));
      }
    },
    [
      buildTaskPayload,
      customVariableRows,
      editingCustomVariable,
      editor,
      markEditorDraft,
      syncCustomVariablesToEditor,
      t,
    ],
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

  // 根据 Job ID 快速定位文件并选中作业实例
  // Quickly locate task file and select job instance by Job ID
  const handleLocateJobId = async (customId?: string) => {
    const trimmed = (customId ?? '').trim();
    if (!trimmed) {
      return false;
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
        return false;
      }
      const targetNode = findTreeNode(tree, job.task_id);
      if (!targetNode || targetNode.node_type !== 'file') {
        toast.error(t('loadFileFailed'));
        return false;
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
      toast.success(t('jobIdLocated'));
      return true;
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadRunsFailed'));
      return false;
    }
  };

  // 快速打开工作区文件（展开其所有父级目录并选中加载）
  // Quickly open a workspace file (expand ancestor folders and select node)
  const handleQuickOpenFile = useCallback(
    async (node: SyncTaskTreeNode) => {
      const ancestorFolderIds: number[] = [];
      let cursor = node.parent_id ? findTreeNode(tree, node.parent_id) : null;
      while (cursor) {
        if (cursor.node_type === 'folder') {
          ancestorFolderIds.unshift(cursor.id);
        }
        cursor = cursor.parent_id ? findTreeNode(tree, cursor.parent_id) : null;
      }
      setExpandedFolderIds((current) =>
        Array.from(new Set([...current, ...ancestorFolderIds])),
      );
      await handleSelectNode(node);
    },
    [tree, handleSelectNode],
  );

  // 更新任务共享与共建者设置（纯内存更新，不污染文件未保存草稿状态）
  // Update task sharing and collaborator permissions (in-memory only, does not dirty file draft)
  const handleUpdateSharing = useCallback(
    (isPublic: boolean, collaboratorIds: number[]) => {
      const nextDef = {
        ...editor.definition,
        is_public: isPublic,
        collaborator_ids: collaboratorIds,
        collaborators: collaboratorIds,
      };
      setEditor((prev) => ({
        ...prev,
        isPublic,
        definition: nextDef,
      }));
    },
    [editor.definition],
  );

  // 独立保存任务共享与权限设置（仅更新权限字段，不触发文件保存、不校验文件内容、不发布新代码版本）
  // Independently save task sharing & permissions (only updates permission metadata, without triggering file save, code validation, or publishing new versions)
  const handleSavePermissions = useCallback(async () => {
    if (!editor.id) {
      return;
    }
    try {
      const currentTask = await services.sync.getTask(editor.id);
      const nextDefinition = {
        ...(currentTask.definition || {}),
        ...(editor.definition || {}),
        is_public: editor.isPublic ?? true,
        collaborator_ids: Array.isArray(editor.definition?.collaborator_ids)
          ? editor.definition.collaborator_ids
          : Array.isArray(editor.definition?.collaborators)
            ? editor.definition.collaborators
            : [],
        collaborators: Array.isArray(editor.definition?.collaborators)
          ? editor.definition.collaborators
          : Array.isArray(editor.definition?.collaborator_ids)
            ? editor.definition.collaborator_ids
            : [],
      };
      const updatedTask = await services.sync.updateTask(editor.id, {
        parent_id: currentTask.parent_id,
        node_type: currentTask.node_type,
        name: currentTask.name,
        description: currentTask.description,
        cluster_id: currentTask.cluster_id,
        engine_version: currentTask.engine_version,
        mode: currentTask.mode,
        content_format: currentTask.content_format,
        content: currentTask.content, // 保持数据库已有文件内容，不覆写用户正在编辑的未保存草稿
        job_name: currentTask.job_name,
        sort_order: currentTask.sort_order,
        definition: nextDefinition,
      });
      setEditor((prev) => ({
        ...prev,
        isPublic: updatedTask.is_public ?? editor.isPublic,
        definition: updatedTask.definition || nextDefinition,
      }));
      setTree((current) => patchTreeNode(current, updatedTask));
      toast.success(t('savePermissionsSuccess'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('savePermissionsFailed'),
      );
    }
  }, [editor.id, editor.definition, editor.isPublic, t]);

  return (
    <div className='-mx-2 flex h-[calc(100vh-96px)] min-h-[780px] flex-col gap-2 bg-background/10 lg:-mx-3'>
      <Card className='gap-0 border-border/60 bg-background/85 py-0 shadow-xs'>
        <CardContent className='flex h-12 items-center justify-between gap-3 px-3 py-1.5'>
          {/* 左侧：任务面包屑导航与状态指示 / 权限角色胶囊 */}
          {/* Left: Task breadcrumb navigation, status indicator, version & permission badges */}
          <div className='flex min-w-0 items-center gap-2 text-xs'>
            <div className='flex size-7 shrink-0 items-center justify-center rounded-md border border-border/50 bg-muted/30 text-primary'>
              <Layers className='size-3.5' />
            </div>
            {activeTaskBreadcrumbs.length > 0 ? (
              <div className='flex min-w-0 items-center gap-1.5'>
                {activeTaskBreadcrumbs.slice(0, -1).map((seg, idx) => (
                  <span
                    key={idx}
                    className='flex items-center gap-1.5 text-muted-foreground'
                  >
                    <span className='max-w-[120px] truncate'>{seg}</span>
                    <span className='text-muted-foreground/40'>/</span>
                  </span>
                ))}
                <span className='max-w-[180px] truncate font-semibold text-foreground'>
                  {activeTaskBreadcrumbs[activeTaskBreadcrumbs.length - 1]}
                </span>
                {isCurrentDirty ? (
                  <span
                    className='size-2 shrink-0 rounded-full bg-amber-500'
                    title={t('unsavedChanges')}
                  />
                ) : null}

                {editor.id ? (
                  <div className='flex items-center gap-1.5 shrink-0'>
                    {/* 版本号 / Version */}
                    <span className='rounded bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground'>
                      v{editor.currentVersion || 1}
                    </span>

                    {/* 发布状态微胶囊 / Publish status micro-pill */}
                    <span
                      className={cn(
                        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-normal leading-none',
                        editor.status === 'published'
                          ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                          : 'bg-muted/60 text-muted-foreground',
                      )}
                    >
                      <span
                        className={cn(
                          'size-1.5 rounded-full',
                          editor.status === 'published'
                            ? 'bg-emerald-500'
                            : 'bg-muted-foreground/50',
                        )}
                      />
                      {editor.status === 'published' ? t('published') : t('draft')}
                    </span>

                    {/* 只读锁定标识 / Read-only locked badge */}
                    {!editor.canEdit ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className='inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-normal text-amber-600 dark:text-amber-400'>
                            <Lock className='size-2.5' />
                            {t('readOnlyLocked')}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>{t('readOnlyBannerText')}</TooltipContent>
                      </Tooltip>
                    ) : null}

                    {/* 顶层任务共享与权限协作 / Top-level task sharing & permissions popover */}
                    <StudioSharePopover
                      isOwner={editor.isOwner ?? true}
                      isCollaborator={editor.isCollaborator ?? false}
                      canEdit={editor.canEdit ?? true}
                      isPublic={editor.isPublic ?? true}
                      createdBy={editor.createdBy}
                      collaboratorIds={
                        Array.isArray(editor.definition?.collaborator_ids)
                          ? (editor.definition.collaborator_ids as number[])
                          : Array.isArray(editor.definition?.collaborators)
                            ? (editor.definition.collaborators as number[])
                            : []
                      }
                      workspaceUsers={workspaceUsers}
                      isAdmin={currentUser?.is_admin ?? false}
                      onUpdateSharing={handleUpdateSharing}
                      onSavePermissions={handleSavePermissions}
                    />

                    {/* 格式标签 / Format tag */}
                    <span className='rounded bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] uppercase text-muted-foreground/70 tracking-wider'>
                      {editor.contentFormat || 'hocon'}
                    </span>
                  </div>
                ) : (
                  <span className='rounded bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] uppercase text-muted-foreground/70 tracking-wider'>
                    {editor.contentFormat || 'hocon'}
                  </span>
                )}
              </div>
            ) : (
              <span className='text-xs text-muted-foreground'>
                {t('noOpenFiles')}
              </span>
            )}
          </div>

          {/* 中间：全局文件与 Job ID 快速检索跳转栏 (VSCode Quick Open 风格) */}
          {/* Center: Global file and Job ID quick jump search bar (VSCode Quick Open style) */}
          <div className='mx-2 hidden flex-1 max-w-sm items-center md:flex'>
            <button
              type='button'
              onClick={() => setQuickOpenOpen(true)}
              className='group relative flex h-7.5 w-full items-center gap-2 rounded-md border border-border/50 bg-muted/20 px-2.5 text-xs text-muted-foreground transition-all hover:border-border/80 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40'
            >
              <Search className='size-3.5 text-muted-foreground/60 transition-colors group-hover:text-foreground' />
              <span className='truncate text-[11px] text-muted-foreground/70 group-hover:text-muted-foreground'>
                {t('searchFileOrJobId')}
              </span>
              <kbd className='pointer-events-none ml-auto hidden select-none rounded border border-border/60 bg-muted/60 px-1.5 py-0.5 font-mono text-[9px] text-muted-foreground/80 sm:inline-flex items-center gap-0.5'>
                <span className='text-[10px]'>⌘</span>P
              </kbd>
            </button>
          </div>

          {/* 右侧：执行环境指示与操作按钮梯队 */}
          {/* Right: Execution environment capsule and hierarchical action buttons */}
          <div className='flex flex-wrap items-center justify-end gap-1.5'>
            {/* 环境指示胶囊 */}
            {/* Environment indicator capsule */}
            <div
              className='flex h-7 items-center gap-1.5 rounded-full border border-border/60 bg-muted/30 px-2.5 text-xs text-muted-foreground'
              title={t('executionTarget')}
            >
              {executionMode === 'local' ? (
                <>
                  <Cpu className='size-3 text-primary' />
                  <span className='text-[11px] font-medium'>{t('localMode')}</span>
                </>
              ) : (
                <>
                  <Globe2 className='size-3 text-sky-500' />
                  <span className='max-w-[110px] truncate text-[11px] font-medium'>
                    {currentCluster?.name || t('unassignedCluster')}
                  </span>
                </>
              )}
            </div>

            <div className='mx-0.5 h-4 w-px bg-border/60' />

            {/* 校验与探查动作组 */}
            {/* Verification & inspection actions */}
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    size='sm'
                    variant='ghost'
                    className='h-7 gap-1.5 px-2 text-xs font-normal text-muted-foreground hover:text-foreground'
                    onClick={handleTestConnections}
                    disabled={
                      saving ||
                      !editor.name.trim() ||
                      actionPending !== null ||
                      !editor.canEdit
                    }
                  >
                    {actionPending === 'test_connections' ? (
                      <Loader2 className='size-3.5 animate-spin' />
                    ) : (
                      <Database className='size-3.5' />
                    )}
                    {actionPending === 'test_connections'
                      ? t('testingConnections')
                      : t('testConnections')}
                  </Button>
                </span>
              </TooltipTrigger>
              {!editor.canEdit ? (
                <TooltipContent>{t('readOnlyTestConnTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            <Button
              size='sm'
              variant='ghost'
              className='h-7 gap-1.5 px-2 text-xs font-normal text-muted-foreground hover:text-foreground'
              onClick={handleBuildDag}
              disabled={saving || !editor.name.trim() || actionPending !== null}
            >
              {actionPending === 'dag' ? (
                <Loader2 className='size-3.5 animate-spin' />
              ) : (
                <GitBranch className='size-3.5' />
              )}
              {actionPending === 'dag' ? t('buildingDag') : 'DAG'}
            </Button>

            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    size='sm'
                    variant='ghost'
                    className='h-7 gap-1.5 px-2 text-xs font-normal text-muted-foreground hover:text-foreground'
                    onClick={handlePreview}
                    disabled={
                      saving ||
                      hasActiveRun ||
                      hasActivePreview ||
                      actionPending !== null ||
                      !editor.canRun
                    }
                  >
                    {actionPending === 'preview' ? (
                      <Loader2 className='size-3.5 animate-spin' />
                    ) : (
                      <Eye className='size-3.5' />
                    )}
                    {actionPending === 'preview'
                      ? t('preparingPreview')
                      : t('preview')}
                  </Button>
                </span>
              </TooltipTrigger>
              {!editor.canRun ? (
                <TooltipContent>{t('readOnlyPreviewTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            <div className='mx-0.5 h-4 w-px bg-border/60' />

            {/* 核心保存与执行动作组 */}
            {/* Core save and execution action buttons */}
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    size='sm'
                    variant='outline'
                    className={cn(
                      'h-7 gap-1.5 px-2.5 text-xs transition-colors',
                      isCurrentDirty
                        ? 'border-amber-500/40 bg-amber-500/10 text-amber-700 hover:bg-amber-500/20 dark:text-amber-300'
                        : 'text-foreground',
                    )}
                    onClick={handleSave}
                    disabled={saving || !editor.name.trim() || !editor.canEdit}
                  >
                    <Save className='size-3.5' />
                    {t('save')}
                  </Button>
                </span>
              </TooltipTrigger>
              {!editor.canEdit ? (
                <TooltipContent>{t('readOnlySaveTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    size='sm'
                    variant='outline'
                    className='h-7 gap-1.5 px-2 text-xs font-normal text-muted-foreground hover:text-foreground'
                    onClick={handleOpenPublishDialog}
                    disabled={
                      saving ||
                      publishing ||
                      !editor.name.trim() ||
                      !editor.canEdit
                    }
                  >
                    <GitCommit className='size-3.5 text-primary' />
                    <span>{t('publishNewVersion')}</span>
                  </Button>
                </span>
              </TooltipTrigger>
              {!editor.canEdit ? (
                <TooltipContent>{t('readOnlySaveTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        size='sm'
                        className='h-7 gap-1 bg-primary px-2.5 text-xs font-medium shadow-xs hover:bg-primary/90'
                        disabled={
                          saving ||
                          hasActiveRun ||
                          hasActivePreview ||
                          !editor.canRun
                        }
                      >
                        <Play className='size-3.5' />
                        <span>{t('run')}</span>
                        <ChevronDown className='size-3 opacity-70' />
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
                </span>
              </TooltipTrigger>
              {!editor.canRun ? (
                <TooltipContent>{t('readOnlyRunTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size='sm'
                  variant='outline'
                  className={cn(
                    'h-7 gap-1 px-2.5 text-xs transition-colors',
                    activeJobs.length > 0
                      ? 'border-rose-500/40 text-rose-600 hover:bg-rose-500/10 dark:text-rose-400'
                      : 'text-muted-foreground',
                  )}
                  disabled={activeJobs.length === 0}
                >
                  <Square className='size-3.5' />
                  <span>{t('stop')}</span>
                  <ChevronDown className='size-3 opacity-70' />
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

      <div
        className={cn(
          'grid min-h-0 flex-1 grid-cols-[240px_minmax(0,1fr)_360px] gap-2 transition-all duration-200',
          isConsoleMaximized
            ? 'grid-rows-[0px_minmax(0,1fr)]'
            : isConsoleExpanded
              ? 'grid-rows-[minmax(0,1fr)_390px]'
              : 'grid-rows-[minmax(0,1fr)_260px]',
        )}
      >
        <Card className='col-start-1 row-start-1 row-span-2 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs'>
          <CardContent className='flex h-full min-h-0 flex-col p-0'>
            {/* 资源管理器标题与工具栏 (VSCode 风格) */}
            {/* Explorer title and action toolbar (VSCode style) */}
            <div className='flex h-8.5 shrink-0 items-center justify-between border-b border-border/50 bg-muted/15 px-2.5'>
              <div className='flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-foreground/80'>
                <FolderTree className='size-3.5 text-primary/80' />
                <span>{t('resources')}</span>
                <span className='rounded-full bg-muted/60 px-1.5 py-0.2 font-mono text-[10px] font-normal text-muted-foreground'>
                  {fileCount}
                </span>
              </div>
              <div className='flex items-center gap-0.5'>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      aria-label={t('newFolder')}
                      onClick={() => {
                        const folderNode = selectedFolderId
                          ? findTreeNode(tree, selectedFolderId)
                          : null;
                        handleInlineCreateFolder(folderNode);
                      }}
                      className='flex size-6 items-center justify-center rounded-[4px] text-muted-foreground/70 transition-colors hover:bg-muted/80 hover:text-foreground active:scale-95'
                    >
                      <FolderPlus className='size-3.5' />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='text-xs'>{t('newFolder')}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      aria-label={t('newFile')}
                      onClick={() => {
                        const folderNode = selectedFolderId
                          ? findTreeNode(tree, selectedFolderId)
                          : null;
                        if (!folderNode) {
                          toast.error(t('selectFolderBeforeCreateFile'));
                          return;
                        }
                        handleInlineCreateFile(folderNode);
                      }}
                      className='flex size-6 items-center justify-center rounded-[4px] text-muted-foreground/70 transition-colors hover:bg-muted/80 hover:text-foreground active:scale-95'
                    >
                      <FilePlus2 className='size-3.5' />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='text-xs'>{t('newFile')}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      aria-label={expandedFolderIds.length > 0 ? t('collapseAll') : t('expandAll')}
                      onClick={handleToggleCollapseAll}
                      className='flex size-6 items-center justify-center rounded-[4px] text-muted-foreground/70 transition-colors hover:bg-muted/80 hover:text-foreground active:scale-95'
                    >
                      <ChevronsUpDown className='size-3.5' />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='text-xs'>
                    {expandedFolderIds.length > 0
                      ? t('collapseAll')
                      : t('expandAll')}
                  </TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      aria-label={t('refresh')}
                      onClick={() => void loadWorkspace(selectedNodeId)}
                      className='flex size-6 items-center justify-center rounded-[4px] text-muted-foreground/70 transition-colors hover:bg-muted/80 hover:text-foreground active:scale-95'
                    >
                      <RefreshCw
                        className={cn('size-3.5', loading && 'animate-spin')}
                      />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='text-xs'>{t('refresh')}</TooltipContent>
                </Tooltip>
              </div>
            </div>
            {/* 快捷过滤输入框与范围筛选 */}
            {/* Quick search/filter input and scope selector */}
            <div className='border-b border-border/40 bg-muted/10 px-2 py-1.5 space-y-1.5'>
              <div className='relative flex items-center'>
                <Search className='pointer-events-none absolute left-2 size-3 text-muted-foreground/60' />
                <Input
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  className='h-6 rounded border-border/50 bg-background/80 pl-6 pr-6 text-xs placeholder:text-muted-foreground/50 focus-visible:ring-1 focus-visible:ring-primary/40'
                  placeholder={t('filterFiles')}
                />
                {keyword ? (
                  <button
                    type='button'
                    onClick={() => setKeyword('')}
                    className='absolute right-1.5 text-muted-foreground/60 hover:text-foreground'
                  >
                    <X className='size-3' />
                  </button>
                ) : null}
              </div>

              {/* 任务过滤范围切换：我的与公开（默认，过滤非自己任务）/ 全部 / 仅我创建 */}
              {/* Task filter scope pills: Mine & Public (default, filters non-owned) / All / Only Mine */}
              <div className='flex items-center rounded-md bg-muted/50 p-0.5 text-[11px] border border-border/40'>
                <button
                  type='button'
                  onClick={() => handleFilterScopeChange('mine_and_public')}
                  className={cn(
                    'flex-1 rounded-[3px] py-0.5 text-center font-medium transition-all',
                    treeFilterScope === 'mine_and_public'
                      ? 'bg-background text-foreground shadow-2xs font-semibold'
                      : 'text-muted-foreground/80 hover:text-foreground',
                  )}
                  title={t('treeFilterMineAndPublicTooltip')}
                >
                  {t('treeFilterMineAndPublic')}
                </button>
                <button
                  type='button'
                  onClick={() => handleFilterScopeChange('all')}
                  className={cn(
                    'flex-1 rounded-[3px] py-0.5 text-center font-medium transition-all',
                    treeFilterScope === 'all'
                      ? 'bg-background text-foreground shadow-2xs font-semibold'
                      : 'text-muted-foreground/80 hover:text-foreground',
                  )}
                >
                  {t('treeFilterAll')}
                </button>
                <button
                  type='button'
                  onClick={() => handleFilterScopeChange('only_mine')}
                  className={cn(
                    'flex-1 rounded-[3px] py-0.5 text-center font-medium transition-all',
                    treeFilterScope === 'only_mine'
                      ? 'bg-background text-foreground shadow-2xs font-semibold'
                      : 'text-muted-foreground/80 hover:text-foreground',
                  )}
                >
                  {t('treeFilterOnlyMine')}
                </button>
              </div>
            </div>

            {/* 文件树滚动区域 */}
            {/* File tree scrollable viewport */}
            <ScrollArea
              className='min-h-0 flex-1'
              onContextMenu={(event) =>
                openTreeContextMenu(event, 'root', null)
              }
            >
              <div className='px-1 py-1.5'>
                {loading ? (
                  <div className='flex items-center gap-2 p-3 text-xs text-muted-foreground'>
                    <Loader2 className='size-3.5 animate-spin' />
                    <span>{t('loading')}</span>
                  </div>
                ) : filteredTree.length === 0 ? (
                  <div className='p-3 text-xs text-muted-foreground'>
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
                    dirtyNodeIds={dirtyNodeIds}
                    renamingNodeId={renamingNodeId}
                    onRenameStart={handleRenameStart}
                    onRenameCommit={handleRenameCommit}
                    onRenameCancel={handleRenameCancel}
                    onCreateFile={handleInlineCreateFile}
                    onCreateFolder={handleInlineCreateFolder}
                    onDelete={handleInlineDelete}
                    creatingNode={creatingNode}
                    onCreateCommit={handleInlineCreateCommit}
                    onCreateCancel={handleInlineCreateCancel}
                    draggingNodeId={draggingNodeId}
                    dragOverFolderId={dragOverFolderId}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragLeave={handleDragLeave}
                    onDrop={handleDrop}
                    onDragEnd={handleDragEnd}
                  />
                )}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>

        <Card
          className={cn(
            'col-start-2 row-start-1 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs transition-all duration-200',
            isConsoleMaximized && 'hidden',
          )}
        >
          <CardContent className='flex h-full min-h-0 flex-col p-0'>
            {/* VS Code 风格平铺直角标签页栏 */}
            {/* VS Code style flat rectangular tab bar */}
            <div className='flex h-9 shrink-0 items-stretch justify-between border-b border-border/40 bg-muted/25 px-0 select-none overflow-hidden'>
              <div
                ref={tabStripRef}
                className='flex h-full flex-1 items-stretch overflow-x-auto scrollbar-none'
              >
                {openTabs.length > 0 ? (
                  openTabs.map((tab) => {
                    const isTabActive = selectedNodeId === tab.id;
                    const isTabDirty = Boolean(editorDrafts[tab.id]?.dirty);
                    return (
                      <div
                        key={tab.id}
                        ref={(node) => {
                          tabButtonRefs.current[tab.id] = node;
                        }}
                        role='tab'
                        aria-selected={isTabActive}
                        tabIndex={0}
                        className={cn(
                          'group relative flex h-full items-center gap-2 border-r border-border/40 px-3 text-xs select-none cursor-pointer transition-colors',
                          isTabActive
                            ? '-mb-px border-b border-b-background bg-background font-medium text-foreground z-10'
                            : 'bg-transparent text-muted-foreground/80 hover:bg-muted/40 hover:text-foreground',
                        )}
                        onClick={() => void handleSelectTab(tab.id)}
                        onAuxClick={(e) => {
                          if (e.button === 1) {
                            e.preventDefault();
                            void handleCloseTab(tab.id);
                          }
                        }}
                      >
                        {/* 激活状态顶部 2px 主题色高光指示条 */}
                        {/* 2px primary accent bar on top for active tab */}
                        {isTabActive ? (
                          <span className='absolute inset-x-0 top-0 h-[2px] bg-primary' />
                        ) : null}

                        <FileCode2
                          className={cn(
                            'size-3.5 shrink-0',
                            isTabActive
                              ? 'text-primary'
                              : 'text-muted-foreground/60',
                          )}
                        />
                        <span className='max-w-[150px] truncate' title={tab.name}>
                          {tab.name}
                        </span>

                        <div className='relative ml-0.5 flex size-4 shrink-0 items-center justify-center'>
                          {isTabDirty ? (
                            <span
                              aria-label={t('unsavedDraft')}
                              title={t('unsavedChanges')}
                              className='size-2 rounded-full bg-amber-500 transition-all group-hover:scale-0 group-hover:opacity-0'
                            />
                          ) : null}
                          <button
                            type='button'
                            aria-label={`${t('close')} ${tab.name}`}
                            className={cn(
                              'absolute inset-0 flex items-center justify-center rounded-xs text-muted-foreground transition-all hover:bg-muted-foreground/20 hover:text-foreground',
                              isTabDirty
                                ? 'scale-0 opacity-0 group-hover:scale-100 group-hover:opacity-100'
                                : isTabActive
                                  ? 'opacity-60 hover:opacity-100'
                                  : 'opacity-0 group-hover:opacity-60 hover:!opacity-100',
                            )}
                            onClick={(event) => {
                              event.stopPropagation();
                              void handleCloseTab(tab.id);
                            }}
                          >
                            <X className='size-2.5' />
                          </button>
                        </div>
                      </div>
                    );
                  })
                ) : (
                  <div className='flex items-center px-3 text-xs text-muted-foreground/60'>
                    {t('noOpenFiles')}
                  </div>
                )}
              </div>

              {/* 标签栏快捷菜单（关闭其他 / 关闭全部） */}
              {/* Tab bar action dropdown (close others / close all) */}
              {openTabs.length > 0 ? (
                <div className='flex items-center px-1 border-l border-border/30 bg-muted/10'>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        size='icon'
                        variant='ghost'
                        className='size-7 shrink-0 text-muted-foreground hover:text-foreground'
                      >
                        <MoreHorizontal className='size-3.5' />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align='end'>
                      {selectedNodeId ? (
                        <DropdownMenuItem
                          disabled={openTabs.length <= 1}
                          onClick={() => handleCloseOtherTabs(selectedNodeId)}
                        >
                          {t('closeOtherTabs')}
                        </DropdownMenuItem>
                      ) : null}
                      <DropdownMenuItem onClick={handleCloseAllTabs}>
                        {t('closeAllTabs')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              ) : null}
            </div>
            {/* VS Code 风格文件路径面包屑条 */}
            {/* VS Code style file path breadcrumb bar */}
            {openTabs.length > 0 && activeTaskBreadcrumbs.length > 0 ? (
              <div className='flex h-6 shrink-0 items-center gap-1.5 border-b border-border/30 bg-background/50 px-3 text-[11px] text-muted-foreground overflow-x-auto scrollbar-none'>
                {activeTaskBreadcrumbs.slice(0, -1).map((segment, index) => (
                  <span key={index} className='flex items-center gap-1.5'>
                    <span className='max-w-[120px] truncate hover:text-foreground transition-colors cursor-default'>
                      {segment}
                    </span>
                    <ChevronRight className='size-3 shrink-0 text-muted-foreground/40' />
                  </span>
                ))}
                <span className='flex items-center gap-1 font-medium text-foreground max-w-[180px] truncate'>
                  <FileCode2 className='size-3 text-primary shrink-0' />
                  <span className='truncate'>
                    {activeTaskBreadcrumbs[activeTaskBreadcrumbs.length - 1]}
                  </span>
                </span>
                {isCurrentDirty ? (
                  <span
                    className='size-1.5 shrink-0 rounded-full bg-amber-500'
                    title={t('unsavedChanges')}
                  />
                ) : null}
                <span className='ml-1 font-mono text-[10px] uppercase text-muted-foreground/50'>
                  ({editor.contentFormat || 'hocon'})
                </span>
              </div>
            ) : null}

            {/* 任务只读锁定横幅提示 */}
            {/* Read-only locked warning banner */}
            {editor.id && !editor.canEdit ? (
              <div className='flex items-center justify-between border-b border-amber-500/20 bg-amber-500/10 px-3 py-1.5 text-xs text-amber-600 dark:text-amber-400 shrink-0'>
                <div className='flex items-center gap-2'>
                  <Lock className='size-3.5 shrink-0' />
                  <span>{t('readOnlyBannerText')}</span>
                </div>
                <Badge
                  variant='outline'
                  className='border-amber-500/30 bg-amber-500/15 text-[10px] text-amber-600 dark:text-amber-400'
                >
                  {t('readOnlyLocked')}
                </Badge>
              </div>
            ) : null}

            {/* Monaco 编辑器或空工作区引导画布 */}
            {/* Monaco Editor or Empty Workspace Guide Canvas */}
            <div className='min-h-0 flex-1 bg-background'>
              {openTabs.length === 0 ? (
                <div className='flex h-full min-h-[380px] select-none flex-col items-center justify-center gap-3 p-8 text-center'>
                  <div className='flex size-14 items-center justify-center rounded-2xl border border-dashed border-border/80 bg-muted/20 text-muted-foreground/70'>
                    <FileCode2 className='size-7' />
                  </div>
                  <div className='max-w-xs space-y-1'>
                    <h3 className='text-sm font-medium text-foreground'>
                      {t('noOpenFiles')}
                    </h3>
                    <p className='text-xs leading-relaxed text-muted-foreground/80'>
                      {t('noOpenFilesDesc')}
                    </p>
                  </div>
                  <Button
                    size='sm'
                    variant='outline'
                    className='mt-2 h-7 gap-1.5 border-dashed text-xs shadow-none'
                    onClick={() => {
                      const folderNode = selectedFolderId
                        ? findTreeNode(tree, selectedFolderId)
                        : null;
                      if (folderNode) {
                        openTreeDialog('create-file', folderNode);
                        return;
                      }
                      const firstFolder = flattenTree(tree).find(
                        (n) => n.node_type === 'folder',
                      );
                      if (firstFolder) {
                        openTreeDialog('create-file', firstFolder);
                        return;
                      }
                      openTreeDialog('create-folder', null);
                    }}
                  >
                    <Plus className='size-3.5' />
                    {t('createTask')}
                  </Button>
                </div>
              ) : (
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
                      other: true,
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
                    readOnly: Boolean(editor.id && !editor.canEdit),
                  }}
                />
              )}
            </div>
          </CardContent>
        </Card>

        <StudioSidebarShell
          className='col-start-3 row-start-1 row-span-2'
          rail={
            <>
              <SidebarIconTab
                active={rightSidebarTab === 'settings'}
                icon={<Database className='size-4' />}
                label={t('settings')}
                onClick={() => setRightSidebarTab('settings')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'schedule'}
                icon={<Clock3 className='size-4' />}
                label={t('taskSchedule')}
                onClick={() => setRightSidebarTab('schedule')}
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
                onClick={() => {
                  setGlobalVariablesDefaultTab('all');
                  setRightSidebarTab('globals');
                }}
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
              onClusterChange={(value) => {
                const next =
                  value === '__empty__' ? '' : value;
                if (next) {
                  rememberPreferredClusterId(next);
                }
                updateEditor('clusterId', next);
              }}
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
              onOpenTimeVariables={() => {
                setGlobalVariablesDefaultTab('time');
                setRightSidebarTab('globals');
              }}
            />
          ) : rightSidebarTab === 'schedule' ? (
            <TaskScheduleSidebarPanel
              value={extractTaskScheduleValue(editor.definition || {})}
              lastTriggeredAt={
                selectedScheduleNode?.schedule_last_triggered_at
              }
              nextTriggeredAt={
                selectedScheduleNode?.schedule_next_triggered_at
              }
              onChange={handleScheduleChange}
              onOpenAdvanced={() => {
                setScheduleDraft(
                  extractTaskScheduleValue(editor.definition || {}),
                );
                setScheduleDialogOpen(true);
              }}
            />
          ) : rightSidebarTab === 'versions' ? (
            <VersionSidebarPanel
              taskId={editor.id}
              canEdit={editor.canEdit ?? true}
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
              onPublish={handleOpenPublishDialog}
            />
          ) : (
            <GlobalVariablesSidebarPanel
              variables={globalVariables}
              total={globalVariableTotal}
              page={globalVariablePage}
              pageSize={8}
              isAdmin={currentUser?.is_admin ?? false}
              currentUserId={currentUser?.id}
              defaultTab={globalVariablesDefaultTab}
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

        <Card
          className={cn(
            'col-start-2 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs transition-all duration-200',
            isConsoleMaximized ? 'row-start-1 row-span-2' : 'row-start-2',
          )}
        >
          <CardContent className='flex h-full min-h-0 flex-col p-0'>
            {/* 控制台顶部横向选项卡与操作区 */}
            {/* Console top horizontal tab switcher and action area */}
            <div className='flex h-9 shrink-0 items-center justify-between border-b border-border/50 bg-muted/20 px-2'>
              <div className='flex items-center gap-1'>
                <button
                  type='button'
                  aria-label={t('jobs')}
                  className={cn(
                    'flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-xs font-medium transition-colors',
                    bottomConsoleTab === 'jobs'
                      ? 'bg-background text-foreground shadow-xs'
                      : 'text-muted-foreground hover:bg-background/50 hover:text-foreground',
                  )}
                  onClick={() => setBottomConsoleTab('jobs')}
                >
                  <ListTree className='size-3.5' />
                  <span>{t('jobs')}</span>
                  {activeJobs.length > 0 ? (
                    <span className='size-1.5 rounded-full bg-emerald-500 animate-pulse' />
                  ) : null}
                </button>
                <button
                  type='button'
                  aria-label={t('logs')}
                  className={cn(
                    'flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-xs font-medium transition-colors',
                    bottomConsoleTab === 'logs'
                      ? 'bg-background text-foreground shadow-xs'
                      : 'text-muted-foreground hover:bg-background/50 hover:text-foreground',
                  )}
                  onClick={() => setBottomConsoleTab('logs')}
                >
                  <SquareTerminal className='size-3.5' />
                  <span>{t('logs')}</span>
                </button>
                <button
                  type='button'
                  aria-label={t('preview')}
                  className={cn(
                    'flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-xs font-medium transition-colors',
                    bottomConsoleTab === 'preview'
                      ? 'bg-background text-foreground shadow-xs'
                      : 'text-muted-foreground hover:bg-background/50 hover:text-foreground',
                  )}
                  onClick={() => setBottomConsoleTab('preview')}
                >
                  <Bug className='size-3.5' />
                  <span>{t('preview')}</span>
                </button>
                <button
                  type='button'
                  aria-label={t('checkpoint')}
                  className={cn(
                    'flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-xs font-medium transition-colors',
                    bottomConsoleTab === 'checkpoint'
                      ? 'bg-background text-foreground shadow-xs'
                      : 'text-muted-foreground hover:bg-background/50 hover:text-foreground',
                  )}
                  onClick={() => setBottomConsoleTab('checkpoint')}
                >
                  <Columns2 className='size-3.5' />
                  <span>{t('checkpoint')}</span>
                </button>
              </div>

              {/* 右侧：全屏与还原切换 */}
              {/* Right: Full-screen and restore toggle */}
              <div className='flex items-center gap-1.5'>
                {!isConsoleMaximized ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        size='icon'
                        variant='ghost'
                        className={cn(
                          'size-6 text-muted-foreground hover:text-foreground',
                          isConsoleExpanded && 'text-primary bg-primary/10',
                        )}
                        onClick={() => setIsConsoleExpanded((prev) => !prev)}
                      >
                        <ChevronsUpDown className='size-3.5' />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side='left'>
                      {isConsoleExpanded ? '收缩控制台高度 (260px)' : '扩展控制台高度 (390px)'}
                    </TooltipContent>
                  </Tooltip>
                ) : null}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size='icon'
                      variant='ghost'
                      className='size-6 text-muted-foreground hover:text-foreground'
                      onClick={() => setIsConsoleMaximized((prev) => !prev)}
                    >
                      {isConsoleMaximized ? (
                        <Minimize2 className='size-3.5' />
                      ) : (
                        <Maximize2 className='size-3.5' />
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side='left'>
                    {isConsoleMaximized
                      ? t('restoreConsole')
                      : t('maximizeConsole')}
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>

            {/* 控制台内容面板 */}
            {/* Console content panel */}
            <div className='min-h-0 flex-1 overflow-auto p-3'>
              {bottomConsoleTab === 'jobs' ? (
                <JobRunsPanel
                  jobs={jobs}
                  selectedJobId={selectedJobId}
                  currentUserId={currentUser?.id}
                  currentUsername={currentUser?.username || currentUser?.nickname}
                  isAdmin={currentUser?.is_admin ?? false}
                  isOwner={editor.isOwner ?? true}
                  canRun={editor.canRun ?? true}
                  workspaceUsers={workspaceUsers}
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

      {/* 发布新版本弹窗 */}
      {/* Publish new version dialog */}
      <Dialog
        open={publishDialogOpen}
        onOpenChange={(open) => {
          if (!publishing) {
            setPublishDialogOpen(open);
          }
        }}
      >
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('publishNewVersion')}</DialogTitle>
            <DialogDescription>
              {t('versionManagementDesc')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-4 py-2'>
            <div className='grid gap-2'>
              <Label htmlFor='publish-version-comment'>
                {t('publishComment')}
              </Label>
              <Input
                id='publish-version-comment'
                value={publishComment}
                onChange={(e) => setPublishComment(e.target.value)}
                placeholder={t('publishCommentPlaceholder')}
                disabled={publishing}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    void handleConfirmPublish();
                  }
                }}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => setPublishDialogOpen(false)}
              disabled={publishing}
            >
              {t('cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleConfirmPublish()}
              disabled={publishing}
              className='gap-1.5'
            >
              {publishing ? (
                <Loader2 className='size-3.5 animate-spin' />
              ) : (
                <GitCommit className='size-3.5' />
              )}
              <span>{t('confirm')}</span>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={validationOpen} onOpenChange={setValidationOpen}>
        <DialogContent
          className={cn(
            'w-[94vw] max-w-[94vw] transition-all',
            (validationResult?.checks?.length ?? 0) > 0
              ? 'sm:max-w-[1100px]'
              : 'sm:max-w-[680px]',
          )}
        >
          <DialogHeader>
            <DialogTitle>{validationTitle}</DialogTitle>
            {validationResult?.summary &&
            validationResult.summary.replace(/^sync:\s*/, '').trim() !==
              validationTitle.trim() ? (
              <DialogDescription className='truncate'>
                {validationResult.summary.replace(/^sync:\s*/, '')}
              </DialogDescription>
            ) : null}
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
        <DialogContent
          className={cn(
            'flex flex-col overflow-hidden transition-all',
            dagError
              ? 'h-auto max-h-[85vh] w-[94vw] max-w-2xl p-6'
              : 'h-[88vh] w-[96vw] max-w-[96vw] gap-0 p-0 sm:max-w-[1400px]',
          )}
        >
          <DialogHeader className={cn(dagError ? 'pb-2' : 'px-6 pt-6')}>
            <DialogTitle>
              {dagError ? 'DAG 生成未通过' : t('dagPreview')}
            </DialogTitle>
            {!dagError ? (
              <DialogDescription>
                {t('dagSummary', {
                  nodes: dagNodes.length,
                  edges: dagEdges.length,
                })}
              </DialogDescription>
            ) : null}
          </DialogHeader>

          {dagError ? (
            <div className='min-h-0 flex-1 overflow-auto pr-1'>
              <StudioErrorDiagnosticsView error={dagError} />
            </div>
          ) : (
            <div className='min-h-0 flex-1 overflow-auto px-6 pb-6'>
              <div className='space-y-4'>
                {dagWarnings.length > 0 ? (
                  <div className='flex flex-wrap gap-2'>
                    {dagWarnings.map((warning, index) => (
                      <Badge key={`${warning}-${index}`} variant='outline'>
                        {warning}
                      </Badge>
                    ))}
                  </div>
                ) : null}
                {dagWebUIJob ? (
                  <WebUiDagPreview job={dagWebUIJob} />
                ) : (
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
                )}
              </div>
            </div>
          )}
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
              <div className='flex items-center gap-1.5 text-xs ml-auto'>
                <span className='inline-flex items-center gap-1 rounded border border-emerald-500/30 bg-emerald-500/15 px-2 py-0.5 font-mono text-[11px] font-medium text-emerald-600 dark:text-emerald-400'>
                  + 新增
                </span>
                <span className='inline-flex items-center gap-1 rounded border border-red-500/30 bg-red-500/15 px-2 py-0.5 font-mono text-[11px] font-medium text-red-600 dark:text-red-400'>
                  - 删除
                </span>
              </div>
            </div>
          </div>
          <div className='mx-6 mb-6 min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
            <MonacoDiffEditor
              height='100%'
              theme={
                resolvedTheme === 'light'
                  ? 'sync-diff-light'
                  : 'sync-diff-dark'
              }
              language={
                (compareVersion?.content_format_snapshot ||
                  editor.contentFormat) === 'json'
                  ? 'json'
                  : 'sync-hocon'
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
                diffAlgorithm: 'advanced',
                renderIndicators: true,
                ignoreTrimWhitespace: false,
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
            emptyNode={
              expandedJobLogs?.empty_reason === 'mixed_log_mode' ||
              jobLogs?.empty_reason === 'mixed_log_mode' ||
              expandedJobLogs?.cluster_job_log_mode === 'mixed' ||
              jobLogs?.cluster_job_log_mode === 'mixed' ? (
                <MixedLogModeBanner
                  clusterId={
                    expandedJobLogs?.cluster_id ||
                    jobLogs?.cluster_id ||
                    getSyncJobClusterId(selectedJob)
                  }
                />
              ) : undefined
            }
          />
        </DialogContent>
      </Dialog>

      <CheckpointInspectDialog
        open={checkpointInspectDialogOpen}
        onOpenChange={setCheckpointInspectDialogOpen}
        result={checkpointInspectDialogResult}
        t={t}
      />

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

      {/* 资源树精致 VSCode 风格右键菜单 */}
      {/* Studio Tree refined VSCode-style context menu */}
      <StudioTreeContextMenu
        menuState={treeMenu}
        onClose={() => setTreeMenu((prev) => ({ ...prev, open: false }))}
        onCreateFile={(parent) => {
          if (parent) {
            handleInlineCreateFile(parent);
          }
        }}
        onCreateFolder={(parent) => {
          handleInlineCreateFolder(parent);
        }}
        onRename={(node) => {
          handleRenameStart(node);
        }}
        onMove={(node) => {
          openTreeDialog('move', node);
        }}
        onCopyFile={(node) => {
          void handleCopyFile(node);
        }}
        onDelete={(node) => {
          openTreeDialog('delete', node);
        }}
        onRefresh={() => {
          void loadWorkspace(selectedNodeId);
        }}
      />

      {/* VSCode 风格 Quick Open 多结果浮层 (Cmd+P) */}
      {/* VSCode-style Quick Open multi-result palette (Cmd+P) */}
      <StudioQuickOpenDialog
        open={quickOpenOpen}
        onOpenChange={setQuickOpenOpen}
        tree={tree}
        recentJobs={jobs.map((j) => ({
          id: j.id,
          task_id: j.task_id,
          platform_job_id: j.platform_job_id,
          status: j.status,
          task_name: findTreeNode(tree, j.task_id)?.name || j.platform_job_id,
        }))}
        onSelectFile={handleQuickOpenFile}
        onSelectJob={async (jobId) => {
          await handleLocateJobId(String(jobId));
        }}
        onNewFile={() => {
          const folderNode = selectedFolderId
            ? findTreeNode(tree, selectedFolderId)
            : null;
          if (folderNode) {
            handleInlineCreateFile(folderNode);
          } else {
            toast.error(t('selectFolderBeforeCreateFile'));
          }
        }}
        onNewFolder={() => {
          const folderNode = selectedFolderId
            ? findTreeNode(tree, selectedFolderId)
            : null;
          handleInlineCreateFolder(folderNode);
        }}
      />
    </div>
  );
}

