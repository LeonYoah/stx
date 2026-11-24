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
import {useSearchParams} from 'next/navigation';
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
import {toast} from '@/lib/toast';
import {
  AlertTriangle,
  Check,
  Copy,
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
  FileStack,
  Tags,
  BarChart3,
  Maximize2,
  Minimize2,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  PlugZap,
  Play,
  RefreshCw,
  FolderTree,
  Save,
  Settings,
  Square,
  Trash2,
  Upload,
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
import './studio-chrome.css';
import {Button as ArcButton} from '@/components/arc/button/button';
import {ButtonGroup} from '@/components/arc/button-group/button-group';
import {
  Dialog as ArcDialog,
  DialogContent as ArcDialogContent,
} from '@/components/arc/dialog/dialog';
import {Input as ArcField} from '@/components/arc/input/input';
import {ExpandingButtonGroup} from '@/components/arc/expanding-button-group/expanding-button-group';
import {ExpandingSearch} from '@/components/arc/expanding-search/expanding-search';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {useAuth} from '@/hooks/use-auth';
import {useWorkbenchLeaveGuard} from '@/hooks/use-workbench-leave-guard';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {
  RuntimeStorageCheckpointInspectJobConfig,
  RuntimeStorageCheckpointInspectResult,
} from '@/lib/services/cluster/types';
import type {
  CreateSyncTaskRequest,
  SyncCuratedSection,
  SyncCuratedTemplateView,
  SyncDagResult,
  SyncFormat,
  SyncGlobalVariable,
  SyncJobSummary,
  SyncPluginFactoryInfo,
  SyncPluginType,
  SyncPreviewDataset,
  SyncPreviewSnapshot,
  SyncTask,
  SyncTaskTimeoutDefaults,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {HELP_HINT_DELAY_MS, HelpHint} from '@/components/common/HelpHint';
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
import {JobDetailDialog} from '@/components/common/sync/job-detail/JobDetailDialog';
import type {JobDetailTab} from '@/components/common/sync/job-detail/types';
import {
  JobTable,
  JobTableToolbar,
  mergeKnownJobs,
  retainedActiveJobIds,
  useJobCheckpoint,
  useJobDetail,
  useJobList,
  useJobLogs,
  useJobSparklines,
  useJobTableQuery,
  type JobActorContext,
  type JobRowAction,
} from './job-table';
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

// 拓扑（React Flow + dagre）只在打开 DAG 时加载。/ The topology (React Flow + dagre) loads only when a DAG opens.
const JobTopology = dynamic(() => import('@/components/common/sync/job-detail/JobTopology'), {
  ssr: false,
});


import {
  SidebarIconTab,
  StudioSidebarShell,
} from './StudioSidebarShell';
import {DraftSaveStatusBadge} from './DraftSaveStatusBadge';
import {StudioFileTabs} from './StudioFileTabs';
import {WorkbenchLeaveDialog} from './WorkbenchLeaveDialog';
import {SettingsSidebarPanel} from './SettingsSidebarPanel';
import {CuratedTemplatesManagePanel} from './CuratedTemplatesManagePanel';
import {
  VersionSidebarPanel,
  SimplePagination,
} from './VersionSidebarPanel';
import {
  ConsolePanel,
  MixedLogModeBanner,
} from './ConsolePanel';
import {
  PreviewWorkspacePanel,
  CheckpointWorkspacePanel,
  CheckpointInspectDialog,
} from './CheckpointPanels';
import {
  ValidationResultPanel,
  StudioActionErrorDialog,
  StudioErrorDiagnosticsView,
  VirtualizedLogViewer,
} from './StudioDialogs';
import {
  TreeView,
} from './TreeView';
import {StudioSplitHandle} from './StudioSplitHandle';
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
  WORKSPACE_NAME_PATTERN,
  WORKSPACE_TABS_STORAGE_KEY,
  DEFAULT_STUDIO_LAYOUT_PREFS,
  STUDIO_CONSOLE_HEIGHT_COMPACT,
  STUDIO_CONSOLE_HEIGHT_EXPANDED,
  STUDIO_CONSOLE_HEIGHT_MAX,
  STUDIO_CONSOLE_HEIGHT_MIN,
  STUDIO_LEFT_WIDTH_MAX,
  STUDIO_LEFT_WIDTH_MIN,
  STUDIO_RIGHT_WIDTH_MAX,
  STUDIO_RIGHT_WIDTH_MIN,
  buildStudioGridTemplateColumns,
  buildStudioGridTemplateRows,
  clampStudioConsoleHeight,
  clampStudioLeftWidth,
  clampStudioRightWidth,
  readStudioLayoutPrefs,
  writeStudioLayoutPrefs,
  buildCheckpointActionViewModels,
  buildCheckpointInspectJobConfig,
  buildCheckpointInspectSummary,
  buildCheckpointSubtaskRows,
  buildCopiedWorkspaceName,
  buildDisplayLogLines,
  buildInsertedTemplateContent,
  buildInsertedCuratedContent,
  buildTaskPayloadFromEditor,
  buildMetricGroups,
  hasWorkbenchUnsavedRisk,
  type DraftSaveStatus,
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
  formatValueConstraintHints,
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
  getObservabilityMode,
  getTaskTimeoutMinutes,
  isCancelOutcomeUnknown,
  isSubmitOutcomeUnknown,
  type ObservabilityMode,
  type TaskTimeoutKey,
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
  studioSaveSegment,
  isJobLifecycleActive,
  isJobLifecycleTerminal,
  isReservedBuiltinVariableKey,
  isTreeDescendant,
  listMoveTargets,
  listSiblingNames,
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
  findSyncJobsByIdInput,
  isCursorAtFinishedEnumValue,
  resolveOptionAssignmentContext,
  resolveOptionKeyFromLine,
  resolveVariableCompletionContext,
  resolveVariableSuggestions,
  splitLogLines,
  summarizeCheckpointSourceState,
  summarizeCheckpointSubtaskMetrics,
  toCheckpointNumber,
  toObject,
  toStringMetricMap,
  toVariableRows,
  validateCustomVariableRows,
  validateWorkspaceName,
} from './sync-studio-utils';
import {parseJobDeepLink, type JobDeepLink} from './task-alert-utils';
import {
  buildOptionRuleDiagnostics,
  extractPluginBlockSnapshots,
  rankOptionKeyCompletions,
  resolveOptionKeyCompletionContext,
  toOptionRuleAssistSchema,
} from './sync-option-rule-assist';

const OPTION_RULE_MARKER_OWNER = 'stx-option-rule';

// Gravity UI 的 curly-brackets，16px 实心花括号。HeroUI 文档用这套图标，这里只内联这一枚，不引入整包。
// Gravity UI curly-brackets, a 16px filled brace pair. HeroUI docs use that set; only this glyph is inlined.
function CurlyBracketsIcon({className}: {className?: string}) {
  return (
    <svg
      viewBox='0 0 16 16'
      fill='currentColor'
      aria-hidden='true'
      className={className}
    >
      <path
        fillRule='evenodd'
        clipRule='evenodd'
        d='M3.413 3.421A.84.84 0 0 1 4.25 2.5a.75.75 0 0 0 0-1.5a2.34 2.34 0 0 0-2.33 2.563l.199 2.096a.9.9 0 0 1-.677.957l-.139.035a1.39 1.39 0 0 0 0 2.698l.14.035a.9.9 0 0 1 .676.957l-.2 2.096A2.34 2.34 0 0 0 4.25 15a.75.75 0 0 0 0-1.5a.84.84 0 0 1-.837-.921l.2-2.096A2.4 2.4 0 0 0 2.04 8a2.4 2.4 0 0 0 1.571-2.483zm9.175 9.158a.84.84 0 0 1-.838.921a.75.75 0 0 0 0 1.5a2.34 2.34 0 0 0 2.33-2.563l-.199-2.096a.9.9 0 0 1 .677-.957l.139-.035a1.39 1.39 0 0 0 0-2.698l-.14-.035a.9.9 0 0 1-.676-.957l.2-2.096A2.34 2.34 0 0 0 11.75 1a.75.75 0 0 0 0 1.5c.496 0 .884.427.838.921l-.2 2.096A2.4 2.4 0 0 0 13.959 8a2.4 2.4 0 0 0-1.571 2.483z'
      />
    </svg>
  );
}

/** 工作台底部作业表每页条数（父 design §5：task=20）/ Workbench job table page size (parent design §5). */
const STUDIO_JOB_PAGE_SIZE = 20;
/** 作业表无活动作业时的轮询间隔 / Job table poll interval while no active job is on the page. */
const STUDIO_JOB_POLL_MS = 30_000;
/** 作业表当前页有活动作业时的轮询间隔 / Faster poll interval while the page holds active jobs. */
const STUDIO_JOB_ACTIVE_POLL_MS = 5_000;

export function DataSyncStudio() {
  const t = useTranslations('workbenchStudio');
  const {resolvedTheme} = useTheme();
  const {user: currentUser} = useAuth();
  const tAlert = useTranslations('syncAlert');
  // 作业深链 ?task=&job=（来自告警通知）：只在首次加载工作区时使用一次。
  // Job deep link ?task=&job= (from alert notifications): used once on the first workspace load.
  const searchParams = useSearchParams();
  const deepLinkRef = useRef<JobDeepLink | null>(parseJobDeepLink(searchParams));
  // 已处理过的深链按 taskId:jobId 记 key；初始值标记挂载时由 loadWorkspace 消费的那条，防止重放。
  // Applied deep links are keyed by taskId:jobId; the initial value marks the one consumed by
  // loadWorkspace at mount so it is not replayed.
  const appliedDeepLinkKeyRef = useRef<string>(
    (() => {
      const initial = parseJobDeepLink(searchParams);
      return initial ? `${initial.taskId}:${initial.jobId ?? 0}` : '';
    })(),
  );
  // handleSelectNode 在渲染期重新赋值，保证深链重放总调用最新闭包。
  // handleSelectNode is re-assigned into this ref every render so deep-link replay always calls
  // the latest closure.
  const selectNodeRef = useRef<((node: SyncTaskTreeNode) => void) | undefined>(undefined);
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
  const modelContentDisposableRef = useRef<any>(null);
  const optionRuleMarkersTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
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
  // 任务详情带回的全局超时默认值，只用于任务设置的占位提示
  // Global timeout defaults from the task detail, used only for the task settings placeholders
  const [taskTimeoutDefaults, setTaskTimeoutDefaults] = useState<SyncTaskTimeoutDefaults | null>(null);
  const [tree, setTree] = useState<SyncTaskTreeNode[]>([]);
  const [keyword, setKeyword] = useState('');
  // localStorage 只能在挂载后读：初始化器里读会让客户端首渲染与 SSR 不一致，触发 hydration mismatch。
  // Read localStorage only after mount: reading it in the initializer makes the first client render
  // diverge from SSR HTML and triggers a hydration mismatch.
  const [treeFilterScope, setTreeFilterScope] =
    useState<TreeFilterScope>('mine_and_public');
  useEffect(() => {
    const saved = localStorage.getItem('stx_studio_tree_filter_scope');
    // 「可见」三档里的 only_mine 已从界面拿掉；旧值并入「我的」（自己创建 + 协作）。
    // The old three-way "Visible" filter is gone; only_mine migrates into Mine (created + collaborated).
    if (saved === 'all') {
      setTreeFilterScope('all');
    } else if (saved === 'only_mine') {
      try {
        localStorage.setItem(
          'stx_studio_tree_filter_scope',
          'mine_and_public',
        );
      } catch {}
    }
  }, []);
  const handleFilterScopeChange = useCallback((scope: TreeFilterScope) => {
    setTreeFilterScope(scope);
    try {
      localStorage.setItem('stx_studio_tree_filter_scope', scope);
    } catch {}
  }, []);
  // VSCode Quick Open 快捷检索浮层状态 (Cmd+P)
  // VSCode Quick Open command palette state (Cmd+P)
  const [quickOpenOpen, setQuickOpenOpen] = useState(false);
  // 顶栏搜索槽位固定 280px：字段从左往右在槽内展开，绝对定位的壳不会盖住保存/运行。
  // Top-bar search keeps a 280px slot so the absolutely positioned shell never covers Save/Run.
  const [selectedNodeId, setSelectedNodeId] = useState<number | null>(null);
  const [selectedFolderId, setSelectedFolderId] = useState<number | null>(null);
  // 树节点行内重命名、新建与拖拽状态（VSCode 风格交互）
  // Tree node inline rename, creation, and drag-drop states (VSCode-style interaction)
  const [renamingNodeId, setRenamingNodeId] = useState<number | null>(null);
  const [creatingNode, setCreatingNode] = useState<{
    parentId: number | null;
    nodeType: 'file' | 'folder';
  } | null>(null);
  // 资源操作菜单里点了「新建」时为 true：菜单关闭后不把焦点还给触发按钮，
  // 否则行内输入框刚聚焦就被抢走焦点，空值 blur 会直接取消新建。
  // True when "new folder/file" was picked from the explorer menu: skip the menu's
  // focus restore on close, or the inline input loses focus and its empty blur cancels creation.
  const skipExplorerMenuFocusRestoreRef = useRef(false);
  const [draggingNodeId, setDraggingNodeId] = useState<number | null>(null);
  const [dragOverFolderId, setDragOverFolderId] = useState<number | null>(null);
  const [editor, setEditor] = useState<EditorState>(EMPTY_EDITOR);
  const [dagResult, setDagResult] = useState<SyncDagResult | null>(null);
  const [dagError, setDagError] = useState<UserFacingErrorState | null>(null);
  const [dagOpen, setDagOpen] = useState(false);
  // 运行 / 预览 / 恢复失败走限高诊断弹窗，不把整段堆栈塞进 toast。
  // Run / preview / recover failures use the height-capped dialog instead of a toast.
  const [actionError, setActionError] = useState<UserFacingErrorState | null>(null);
  // 查看运行中/已结束作业的实时执行拓扑
  // View execution DAG of running or finished jobs
  // 作业详情：只记作业 ID，作业对象从 jobs 取，随轮询刷新状态。
  // Job detail: only the job id is kept; the job itself comes from jobs so its status refreshes with polling.
  const [detailJobId, setDetailJobId] = useState<number | null>(null);
  const [detailTab, setDetailTab] = useState<JobDetailTab>('topology');
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
  const [jobs, setJobs] = useState<SyncJobSummary[]>([]);
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const activeJobsPollInFlightRef = useRef(false);
  // retainedJobs：表格中选中 / 打开详情时留下的作业快照（D5 附带风险兜底）。翻页离开选中不丢，
  // 且不进 jobs —— jobs 只承载「任务运行状态」业务判定。
  // retainedJobs: snapshots of jobs selected or detail-opened in the table (the D5 safeguard). Selection
  // survives paging, and the snapshots never join `jobs` — `jobs` stays the run-state source of truth.
  const [retainedJobs, setRetainedJobs] = useState<
    Map<number, SyncJobSummary>
  >(() => new Map());
  // 深链置顶作业 ID：表格第一页无筛选时置顶显示。/ Deep-linked job pinned on the table's first page.
  const [pinnedJobId, setPinnedJobId] = useState<number | null>(null);
  // loadJobs 每次完成后递增，驱动作业表立即重拉（D5 refreshKey）。
  // Bumped on every loadJobs completion; the table reloads immediately (D5 refreshKey).
  const [jobTableRefreshKey, setJobTableRefreshKey] = useState(0);
  const [saving, setSaving] = useState(false);
  // 保存中单独占一段；成功后的「已保存」由草稿是否变脏决定，不再用定时器收回。
  // Pending is its own segment. Saved after success follows draft dirtiness and is not cleared by a timer.
  const [saveAck, setSaveAck] = useState<'idle' | 'pending'>('idle');
  // 顶栏草稿保存状态：只跟手动保存 / 离开前落盘走。
  // Top-bar draft save status follows manual save and leave flush only.
  const [draftSaveStatus, setDraftSaveStatus] =
    useState<DraftSaveStatus>('idle');
  const [leaveActionBusy, setLeaveActionBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [actionPending, setActionPending] = useState<PendingActionKind | null>(
    null,
  );
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
  const [checkpointInspectDialogOpen, setCheckpointInspectDialogOpen] =
    useState(false);
  const [checkpointInspectDialogLoading, setCheckpointInspectDialogLoading] =
    useState<string | null>(null);
  const [checkpointInspectDialogResult, setCheckpointInspectDialogResult] =
    useState<RuntimeStorageCheckpointInspectResult | null>(null);
  const [recoverSourceId, setRecoverSourceId] = useState<string>('');
  const [previewDatasetName, setPreviewDatasetName] = useState('');
  const [openTabs, setOpenTabs] = useState<OpenFileTab[]>([]);
  const [editorDrafts, setEditorDrafts] = useState<
    Record<number, EditorDraftState>
  >({});
  const editorDraftsRef = useRef(editorDrafts);
  const [expandedFolderIds, setExpandedFolderIds] = useState<number[]>([]);

  // 底部控制台全屏与还原状态
  // Full-screen and restore state for the bottom console
  const [isConsoleMaximized, setIsConsoleMaximized] = useState(false);

  // 左右侧栏独立开合：无记录时左展开、右收起；之后记住上次（#110；无专注模式）。
  // 首帧固定用默认值。useState 里读 localStorage 会让「客户端已记住右侧展开」和
  // 服务端默认「右侧收起」对不上，水合改掉开关的 aria-label 和图标，Next 弹出红色报错层，按钮也点不了。
  // Independent left/right collapse: no record → left open / right collapsed; then remember (#110; no Focus Mode).
  // First paint stays on the defaults. Reading localStorage in useState mismatches SSR when the
  // client remembers an open right sidebar, hydrates a different toggle label and icon, and Next
  // covers the page with its red error overlay so the button cannot be clicked.
  const [isLeftSidebarCollapsed, setIsLeftSidebarCollapsed] = useState(
    DEFAULT_STUDIO_LAYOUT_PREFS.leftCollapsed,
  );
  const [isRightSidebarCollapsed, setIsRightSidebarCollapsed] = useState(
    DEFAULT_STUDIO_LAYOUT_PREFS.rightCollapsed,
  );
  const [leftSidebarWidth, setLeftSidebarWidth] = useState(
    DEFAULT_STUDIO_LAYOUT_PREFS.leftWidth,
  );
  const [rightSidebarWidth, setRightSidebarWidth] = useState(
    DEFAULT_STUDIO_LAYOUT_PREFS.rightWidth,
  );
  const [consoleHeight, setConsoleHeight] = useState(
    DEFAULT_STUDIO_LAYOUT_PREFS.consoleHeight,
  );
  const [isStudioLayoutResizing, setIsStudioLayoutResizing] = useState(false);
  const studioLayoutHydratedRef = useRef(false);
  const isConsoleExpanded =
    consoleHeight >= STUDIO_CONSOLE_HEIGHT_EXPANDED;

  useEffect(() => {
    // 挂载后再套用上次开合与尺寸，并跳过这次写入，避免把默认值盖掉已记住的偏好。
    // Apply the remembered layout after mount, and skip that write so defaults do not clobber it.
    if (!studioLayoutHydratedRef.current) {
      studioLayoutHydratedRef.current = true;
      const prefs = readStudioLayoutPrefs();
      setIsLeftSidebarCollapsed(prefs.leftCollapsed);
      setIsRightSidebarCollapsed(prefs.rightCollapsed);
      setLeftSidebarWidth(prefs.leftWidth);
      setRightSidebarWidth(prefs.rightWidth);
      setConsoleHeight(prefs.consoleHeight);
      return;
    }
    if (isStudioLayoutResizing) {
      return;
    }
    writeStudioLayoutPrefs({
      leftCollapsed: isLeftSidebarCollapsed,
      rightCollapsed: isRightSidebarCollapsed,
      leftWidth: leftSidebarWidth,
      rightWidth: rightSidebarWidth,
      consoleHeight,
    });
  }, [
    consoleHeight,
    isLeftSidebarCollapsed,
    isRightSidebarCollapsed,
    isStudioLayoutResizing,
    leftSidebarWidth,
    rightSidebarWidth,
  ]);

  // 顶栏两端开关始终挂载：点击收起后焦点自然留在该按钮上（#110 AC11 / Codex P2）
  // Topbar-end toggles stay mounted so focus remains on the control after collapse (#110 AC11 / Codex P2)
  const handleToggleLeftSidebar = useCallback(() => {
    setIsLeftSidebarCollapsed((prev) => !prev);
  }, []);

  const handleToggleRightSidebar = useCallback(() => {
    setIsRightSidebarCollapsed((prev) => !prev);
  }, []);

  const handleToggleConsoleHeight = useCallback(() => {
    setConsoleHeight((prev) =>
      prev >= STUDIO_CONSOLE_HEIGHT_EXPANDED
        ? STUDIO_CONSOLE_HEIGHT_COMPACT
        : STUDIO_CONSOLE_HEIGHT_EXPANDED,
    );
  }, []);

  const handleStudioLayoutDragStart = useCallback(() => {
    setIsStudioLayoutResizing(true);
  }, []);

  const handleStudioLayoutDragEnd = useCallback(() => {
    setIsStudioLayoutResizing(false);
  }, []);

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
  // 干净的已有文件显示已保存；右侧栏或正文一改，立刻回到保存。
  // A clean existing file shows Saved. Any edit in the body or the right sidebar returns to Save.
  const saveSegment = studioSaveSegment({
    pending: saveAck === 'pending' || draftSaveStatus === 'saving',
    hasFile: Boolean(editor.id),
    dirty: isCurrentDirty,
  });

  // 已保存版本中的定时配置，用于判断定时是否有未保存改动
  // Schedule from the saved baseline, used to tell whether the schedule has unsaved changes
  const savedScheduleValue = useMemo(() => {
    const draft = editor.id ? editorDrafts[editor.id] : undefined;
    const baseline = draft?.dirty
      ? draft.baselineEditor.definition
      : editor.definition;
    return extractTaskScheduleValue(baseline || {});
  }, [editor.id, editor.definition, editorDrafts]);
  const isScheduleUnsaved = (value: TaskScheduleValue) =>
    Boolean(editor.id) &&
    (value.enabled !== savedScheduleValue.enabled ||
      value.cron_expr !== savedScheduleValue.cron_expr ||
      value.timezone !== savedScheduleValue.timezone);

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

  // 顶栏快速打开的远程命中：数字输入防抖查后端，覆盖未加载作业（全局而非当前任务）。
  // Remote hits for the top-bar quick open: numeric input queries the backend
  // debounced so jobs outside the loaded set are found globally, not per task.
  const [quickOpenQuery, setQuickOpenQuery] = useState('');
  const [remoteJobs, setRemoteJobs] = useState<SyncJobSummary[]>([]);
  useEffect(() => {
    if (!/^\d+$/.test(quickOpenQuery.trim().replace(/^#+/, ''))) {
      setRemoteJobs([]);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      findSyncJobsByIdInput(
        (params) => services.sync.listJobs(params),
        quickOpenQuery,
      )
        .then((items) => {
          if (!cancelled) {
            setRemoteJobs(items);
          }
        })
        .catch(() => {
          if (!cancelled) {
            setRemoteJobs([]);
          }
        });
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [quickOpenQuery]);

  const quickOpenItems = useMemo(() => {
    const files = flattenTree(tree)
      .filter((node) => node.node_type === 'file')
      .map((node) => ({
        id: `file-${node.id}`,
        title: node.name,
        meta: getNodeBreadcrumbSegments(tree, node.id).slice(0, -1).join(' / '),
        group: t('matchingFiles'),
        keywords: [String(node.id)],
      }));
    const toJobItem = (job: SyncJobSummary) => ({
      id: `job-${job.id}`,
      title: `#${job.id}`,
      meta: findTreeNode(tree, job.task_id)?.name || job.platform_job_id || '',
      group: t('matchingJobs'),
      keywords: [String(job.id), `#${job.id}`, job.platform_job_id || ''].filter(Boolean),
    });
    const jobItems = jobs.slice(0, 50).map(toJobItem);
    // 远端命中与已加载作业按内部 id 去重，避免同一行出现两次。
    // Remote hits are deduped against loaded jobs by internal id so the same row never appears twice.
    const remoteItems = remoteJobs
      .filter((job) => !jobs.some((loaded) => loaded.id === job.id))
      .map(toJobItem);
    return [...files, ...jobItems, ...remoteItems];
  }, [jobs, remoteJobs, t, tree]);

  const [customVariableRows, setCustomVariableRows] = useState<VariableRow[]>(
    [],
  );
  // 自定义变量弹窗开闭与正在编辑项状态
  // Custom variable dialog open state and current editing item
  const [customVariableDialogOpen, setCustomVariableDialogOpen] =
    useState(false);
  const [editingCustomVariable, setEditingCustomVariable] =
    useState<VariableRow | null>(null);
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
  // 另存精选模板弹窗状态
  // Save-as curated template dialog state
  const [curatedSaveDialogOpen, setCuratedSaveDialogOpen] = useState(false);
  const [curatedSaveName, setCuratedSaveName] = useState('');
  const [curatedSaveDescription, setCuratedSaveDescription] = useState('');
  const [curatedSaveSection, setCuratedSaveSection] =
    useState<SyncCuratedSection>('combo');
  const [curatedSaveContent, setCuratedSaveContent] = useState('');
  const [curatedSaving, setCuratedSaving] = useState(false);
  const [curatedRefreshToken, setCuratedRefreshToken] = useState(0);
  // combo 整文替换确认弹窗
  // Confirm dialog before combo replace-all insert
  const [curatedReplaceDialogOpen, setCuratedReplaceDialogOpen] =
    useState(false);
  const [pendingCuratedInsert, setPendingCuratedInsert] =
    useState<SyncCuratedTemplateView | null>(null);
  const saveAsCuratedFromSelectionRef = useRef<() => void>(() => {});

  const restoredWorkspaceTabsRef = useRef<PersistedWorkspaceTabs | null>(null);
  const customVariableRowsRef = useRef<VariableRow[]>([]);

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
  // —— 底部「作业」页签：JobTable task 预设（服务端分页 / 筛选 / 排序），与 jobs 业务数组双轨（D5）。——
  // —— Bottom "Jobs" tab: the JobTable task preset (server paging / filtering / sorting), kept on a separate
  // track from the `jobs` business array (D5). ——
  const {
    query: jobTableQuery,
    patchQuery: patchJobTableQuery,
    resetQuery: resetJobTableQuery,
  } = useJobTableQuery();
  const jobTableList = useJobList({
    scope: {kind: 'task', taskId: selectedNodeId ?? 0},
    query: jobTableQuery,
    pageSize: STUDIO_JOB_PAGE_SIZE,
    pollMs: STUDIO_JOB_POLL_MS,
    activePollMs: STUDIO_JOB_ACTIVE_POLL_MS,
    refreshKey: jobTableRefreshKey,
    pinnedJobId: pinnedJobId ?? undefined,
    // jobs 里更新的副本（3s getJob 轮询）按 id 覆盖表格行，行状态与底部面板一致。
    // Fresher copies in `jobs` (from the 3s getJob poll) overlay table rows so row state matches the console.
    overlay: jobs,
    // 未打开任务时不发列表请求。/ No list request while no task is open.
    enabled: selectedNodeId !== null,
  });
  const jobTableSparklines = useJobSparklines(
    jobTableList.items,
    jobTableList.updatedAt,
  );

  // selectedJob / detailJob 从 knownJobs 解析：jobs（最新）→ 表格当前页 → retained 快照。
  // 否则翻到第 2 页选中作业时选中会错误回退到 jobs[0]（D5 附带风险）。
  // selectedJob / detailJob resolve through knownJobs: `jobs` (freshest) → current table page → retained
  // snapshots. Otherwise selecting a job on page 2 would wrongly fall back to jobs[0] (the D5 risk).
  const knownJobs = useMemo(
    () => mergeKnownJobs(jobs, jobTableList.items, retainedJobs),
    [jobs, jobTableList.items, retainedJobs],
  );
  /**
   * 选中或打开详情时把该行快照放进 retained 集合，翻页 / 刷新后仍可解析。
   * Retain a row snapshot on select / detail-open so it still resolves after paging or refreshes.
   */
  const retainJob = useCallback((job: SyncJobSummary | null) => {
    if (!job) {
      return;
    }
    setRetainedJobs((prev) => {
      const next = new Map(prev);
      next.set(job.id, job);
      return next;
    });
  }, []);
  const selectedJob = useMemo(
    () =>
      (selectedJobId === null ? null : (knownJobs.get(selectedJobId) ?? null)) ??
      jobs[0] ??
      null,
    [knownJobs, jobs, selectedJobId],
  );
  const detailJobRow = useMemo(
    () =>
      detailJobId === null ? null : (knownJobs.get(detailJobId) ?? null),
    [knownJobs, detailJobId],
  );
  // 详情自持完整记录：摘要行只合并展示 / 状态字段，重字段由 getJob 补齐；active 刷新走 jobs 3s 轮询，
  // 这里不再开第二个定时器。
  // The detail owns its full record: summary rows merge display / status fields only while heavy fields
  // come from getJob; active refresh rides on the 3s jobs poll, so no second timer starts here.
  const detailJob = useJobDetail({job: detailJobRow, enabled: detailJobId !== null});
  // 日志与 Checkpoint 在底部面板或作业详情对应页签可见时都要加载，两处共用同一份数据。
  // Logs and checkpoint load when visible in either the bottom console or the job detail tab; both share one copy.
  const logsVisible = bottomConsoleTab === 'logs' || (detailJob !== null && detailTab === 'logs');
  const checkpointVisible =
    bottomConsoleTab === 'checkpoint' || (detailJob !== null && detailTab === 'checkpoint');

  // 日志 / Checkpoint 数据由共享 hook 提供（抽自原内联实现，大盘 JobDetailHost 复用同一套）：
  // 普通视图与展开弹窗各持一个 useJobLogs 实例，仍共用 selectedJobId 这一份作业指针。
  // Log / checkpoint data comes from the shared hooks (extracted from the former inline implementation,
  // reused by the dashboard JobDetailHost): the normal view and the expanded dialog each hold their own
  // useJobLogs instance, still sharing the same selectedJobId pointer.
  const {result: jobLogs, loading: logsLoading} = useJobLogs({
    jobId: selectedJobId,
    enabled: logsVisible,
    keyword: logSearchTerm,
    level: logFilterMode,
  });
  const {result: expandedJobLogs, loading: expandedLogsLoading} = useJobLogs({
    jobId: selectedJobId,
    enabled: logsDialogOpen,
    keyword: logSearchTerm,
    level: logFilterMode,
    expanded: true,
  });
  const {
    snapshot: checkpointSnapshot,
    loading: checkpointLoading,
    files: checkpointFiles,
    filesLoading: checkpointFilesLoading,
    reload: reloadCheckpoint,
  } = useJobCheckpoint({job: selectedJob, enabled: checkpointVisible});

  /**
   * 打开作业详情：同时选中该作业并留一份 retained 快照，让日志 / Checkpoint 跟随它（翻页离开仍可解析）。
   * Open the job detail: select the job and keep a retained snapshot so logs / checkpoint follow it
   * (still resolvable after paging away).
   */
  const openJobDetail = useCallback((job: SyncJobSummary, tab: JobDetailTab = 'topology') => {
    retainJob(job);
    setSelectedJobId(job.id);
    setDetailJobId(job.id);
    setDetailTab(tab);
    // 完整记录由 useJobDetail 按 jobId 拉取并合并摘要更新（见上方 detailJob），这里不再回填 retainedJobs。
    // The full record is fetched and merged by useJobDetail per jobId (see detailJob above); no
    // retainedJobs backfill happens here anymore.
  }, [retainJob]);
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
        : [],
    [previewSnapshot],
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
  // 选中 / 详情作业不在 jobs 中（如第 2 页选中的旧作业）但仍活动：补进 3s 轮询，只写 retained 快照。
  // Selected / detail jobs outside `jobs` (e.g. an older run picked on page 2) but still active join the 3s
  // poll; results only update the retained snapshot.
  const retainedPollJobIds = useMemo(
    () => retainedActiveJobIds(knownJobs, jobs, [selectedJobId, detailJobId]),
    [knownJobs, jobs, selectedJobId, detailJobId],
  );
  // 作业表行操作的任务级权限上下文（父 design §6 工作台行）。/ Task-level actor context for row actions.
  const jobTableActor = useMemo<JobActorContext>(
    () => ({
      isAdmin: currentUser?.is_admin ?? false,
      isOwner: editor.isOwner ?? true,
      canRun: editor.canRun ?? true,
      currentUserId: currentUser?.id,
      disableRecover: hasActiveRun || hasActivePreview,
    }),
    [
      currentUser?.is_admin,
      currentUser?.id,
      editor.isOwner,
      editor.canRun,
      hasActiveRun,
      hasActivePreview,
    ],
  );
  /**
   * 作业表行操作：详情之外的三个动作接回既有的停止 / savepoint 停止 / 恢复实现。
   * Job table row actions: the three non-detail actions map onto the existing stop / savepoint-stop /
   * recover handlers.
   */
  const handleJobTableAction = (action: JobRowAction, job: SyncJobSummary) => {
    retainJob(job);
    if (action === 'recover') {
      handleRecoverFromHistory(job.id);
      return;
    }
    void handleCancelJob(job.id, action === 'savepointStop');
  };
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
    editorDraftsRef.current = editorDrafts;
  }, [editorDrafts]);

  useEffect(() => {
    currentClusterIdRef.current = editor.clusterId;
  }, [editor.clusterId]);

  // 「已保存」提示短暂展示后回到 idle，避免顶栏常驻噪音。
  // Fade the Saved badge back to idle so the top bar stays quiet.
  useEffect(() => {
    if (draftSaveStatus !== 'saved') {
      return;
    }
    const timer = window.setTimeout(() => {
      setDraftSaveStatus((prev) => (prev === 'saved' ? 'idle' : prev));
    }, 2500);
    return () => window.clearTimeout(timer);
  }, [draftSaveStatus]);

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
        // 深链指向的任务不存在或没有权限：提示一次，按原来的方式打开工作区。
        // The deep-linked task is missing or not visible: warn once and open the workspace as usual.
        const pendingLink = deepLinkRef.current;
        if (pendingLink && pendingLink.taskId === preferredFileId && nextSelected !== preferredFileId) {
          deepLinkRef.current = null;
          toast.warning(tAlert('deepLinkTaskNotFound', {id: pendingLink.taskId}));
        }

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
          if (treeTask) {
            syncOpenTabs(treeTask);
          }
          const task = await services.sync.getTask(nextSelected);
          setTaskTimeoutDefaults(task.timeout_defaults ?? null);
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
      const mapped: Record<string, any> = Object.fromEntries(
        (result.options || []).map((item) => [item.key, item]),
      );
      // 挂载 3.0 约束元数据，供 hover / 后续诊断使用（不污染真实 option key）。
      // Attach 3.0 constraint metadata for hover / future diagnostics (reserved keys).
      mapped.__schema_meta__ = {
        value_constraints: result.value_constraints || [],
        condition_rules: result.condition_rules || [],
      };
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
      // 日志 / Checkpoint 随 selectedJobId 置空由 hook 内部复位，无需手动清空。
      // Logs / checkpoint reset inside the hooks as selectedJobId goes null; no manual clearing needed.
      setSelectedJobId(null);
      setPinnedJobId(null);
      setJobTableRefreshKey((key) => key + 1);
      return;
    }
    try {
      const data = await services.sync.listJobs({
        current: 1,
        size: 50,
        task_id: taskId,
      });
      let items = data.items || [];
      // 深链要选中的作业不在最近 50 条里时，单独读取并放到列表最前面。
      // When the deep-linked job is not among the latest 50, fetch it and put it first.
      const link = deepLinkRef.current;
      let deepLinkedJobId: number | null = null;
      if (link && link.taskId === taskId) {
        deepLinkRef.current = null;
        setBottomConsoleTab('jobs');
        if (link.jobId) {
          let linkedJob = items.find((item) => item.id === link.jobId) ?? null;
          if (!linkedJob) {
            const job = await services.sync.getJob(link.jobId).catch(() => null);
            if (job && job.task_id === taskId) {
              items = [job, ...items];
              linkedJob = job;
            }
          }
          if (linkedJob) {
            deepLinkedJobId = link.jobId;
            // 深链作业也进 retained：下一次 loadJobs 只剩最新 50 条时详情 / 选中仍可解析。
            // Retain the deep-linked job too: once the next loadJobs trims `jobs` to the latest 50,
            // the detail / selection keeps resolving.
            retainJob(linkedJob);
          } else {
            toast.warning(tAlert('deepLinkJobNotFound', {id: link.jobId}));
          }
        }
        // 深链作业置顶（仅表格第一页无筛选时生效）。/ Pin the deep-linked job (page 1, unfiltered only).
        setPinnedJobId(deepLinkedJobId);
      }
      setJobs(items);
      // 通知作业表立即重拉，与运行 / 停止 / 恢复 / 切换任务后的 jobs 保持同步（D5 refreshKey）。
      // Nudge the job table to reload so it stays in sync with `jobs` after run / stop / recover /
      // task switch (D5 refreshKey).
      setJobTableRefreshKey((key) => key + 1);
      // 告警深链带作业时直接打开作业详情。/ A deep link carrying a job opens its detail directly.
      if (deepLinkedJobId) {
        setDetailJobId(deepLinkedJobId);
        setDetailTab('topology');
      }
      setSelectedJobId((prev) => {
        if (deepLinkedJobId) {
          return deepLinkedJobId;
        }
        if (prev && items.some((item) => item.id === prev)) {
          return prev;
        }
        return items[0]?.id || null;
      });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('loadRunsFailed'));
    }
  }, [t, tAlert, retainJob]);

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

  const handleInspectCheckpointFile = useCallback(
    async (path: string) => {
      const clusterId = getSyncJobClusterId(selectedJob);
      if (!clusterId || !path || !selectedJob) {
        toast.error(t('checkpointInspectUnavailable'));
        return;
      }
      setCheckpointInspectDialogLoading(path);
      try {
        // 检查用的是「提交脚本」，列表摘要行的 submit_spec 已不含 submitted_content，必须按 id 取
        // 全量；取不到就报错，不拿编辑器当前脚本冒充提交脚本。
        // Inspection runs on the submitted script, but a list summary row's submit_spec no longer
        // carries submitted_content — fetch the full record by id; on failure we surface the error
        // instead of passing the editor script off as the submitted one.
        const fullJob = await services.sync.getJob(selectedJob.id);
        const jobConfig = buildCheckpointInspectJobConfig(
          fullJob,
          editor,
          customVariableRows,
        );
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
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('checkpointInspectFailed'),
        );
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

  // 工作区只启动加载一次：StrictMode 挂载期 effect 双调用或依赖抖动导致的第二次 loadWorkspace
  // 会在首次选任务→loadJobs 回填后再执行 setJobs([])，而 selectedNodeId 不变不再触发重载，
  // 运行记录被永久清空（表现为点了作业失败深链却「找不到运行记录」）。
  // The workspace must boot only once: a second loadWorkspace (StrictMode double-effect or dep
  // churn) runs setJobs([]) after the first run already refilled jobs via loadJobs, and with
  // selectedNodeId unchanged no reload fires — leaving the runs list permanently empty (the
  // "deep link opens but no run record" bug).
  const workspaceBootedRef = useRef(false);
  useEffect(() => {
    if (workspaceBootedRef.current) {
      return;
    }
    workspaceBootedRef.current = true;
    void loadWorkspace(deepLinkRef.current?.taskId ?? null);
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

  // 切换任务：重置作业表查询（page=1 清空筛选）、深链置顶与 retained 快照，并关闭详情弹窗
  // （旧行为是 jobs 清空后 detailJob 解析不到、弹窗自然关闭；有了 retained 快照后需要显式清掉）。
  // Task switch: reset the table query (page 1 + filters), the deep-link pin and the retained snapshots,
  // and close the detail dialog (it used to close once `jobs` emptied and detailJob stopped resolving;
  // with retained snapshots it must be cleared explicitly).
  useEffect(() => {
    resetJobTableQuery();
    setPinnedJobId(null);
    setRetainedJobs(new Map());
    setDetailJobId(null);
  }, [selectedNodeId, resetJobTableQuery]);

  useEffect(() => {
    if (previewDatasets.length === 0) {
      setPreviewDatasetName('');
      return;
    }
    if (
      !previewDatasets.some((dataset) => dataset.name === previewDatasetName)
    ) {
      setPreviewDatasetName(previewDatasets[0]?.name || '');
    }
  }, [previewDatasetName, previewDatasets]);

  useEffect(() => {
    if (!previewJob) {
      setPreviewSnapshot(null);
      return;
    }
    void loadPreviewSnapshot(previewJob.id, previewDatasetName || undefined);
  }, [previewJob?.id, previewDatasetName, loadPreviewSnapshot]);

  // 同步编辑器定义中的自定义变量与类型到状态
  // Sync custom variables and types from editor definition into state
  useEffect(() => {
    const rows = toVariableRows(
      editor.definition?.custom_variables,
      editor.definition?.custom_variable_types,
    );
    setCustomVariableRows(rows);
  }, [selectedNodeId, editor.currentVersion]);

  // 日志与 Checkpoint 的清空已收敛进 useJobLogs / useJobCheckpoint（jobId、关键字、级别变化时各自复位），
  // 这里只保留预览快照的复位。
  // Log / checkpoint resets now live inside useJobLogs / useJobCheckpoint (each resets when jobId, keyword
  // or level changes); only the preview snapshot reset remains here.
  useEffect(() => {
    setPreviewSnapshot(null);
  }, [selectedJobId, logFilterMode, logSearchTerm]);

  // 活动作业 3s 刷新：jobs 中的活动作业照旧；不在 jobs 的 retained 活动作业（选中 / 详情、翻页离开）
  // 只回写 retained 快照，绝不进 jobs —— activeJobs / hasActiveRun 等判定只看 jobs（D5）。
  // Active jobs refresh every 3s: active jobs inside `jobs` as before; retained active jobs outside `jobs`
  // (selected / detail after paging away) only write back their snapshots — never `jobs` — so activeJobs /
  // hasActiveRun keep depending solely on `jobs` (D5).
  useEffect(() => {
    if (activeJobs.length === 0 && retainedPollJobIds.length === 0) {
      return;
    }
    const timer = window.setInterval(() => {
      if (activeJobsPollInFlightRef.current) {
        return;
      }
      activeJobsPollInFlightRef.current = true;
      void (async () => {
        try {
          const pollIds = new Set<number>([
            ...activeJobs.map((job) => job.id),
            ...retainedPollJobIds,
          ]);
          const refreshed = await Promise.all(
            [...pollIds].map((id) => services.sync.getJob(id)),
          );
          const freshById = new Map(refreshed.map((job) => [job.id, job]));
          setJobs((current) =>
            current.map((job) => freshById.get(job.id) || job),
          );
          const retainedUpdates = refreshed.filter((job) =>
            retainedPollJobIds.includes(job.id),
          );
          if (retainedUpdates.length > 0) {
            setRetainedJobs((prev) => {
              const next = new Map(prev);
              for (const job of retainedUpdates) {
                next.set(job.id, job);
              }
              return next;
            });
          }
        } catch {
          // 忽略瞬时轮询抖动，保持 Studio 可继续操作。
          // Ignore transient polling errors to keep the studio usable.
        } finally {
          activeJobsPollInFlightRef.current = false;
        }
      })();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [activeJobs, retainedPollJobIds]);

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

  // 执行精选插入（已确认 combo 替换或无需确认）
  // Perform curated insert (combo replace already confirmed or not needed)
  const performInsertCuratedTemplate = useCallback(
    async (item: SyncCuratedTemplateView) => {
      const section = (item.section || 'combo') as SyncCuratedSection;
      const loadingToastId = toast.loading(t('loadingCurated'));
      try {
        const clusterIdNum = editor.clusterId
          ? Number(editor.clusterId)
          : undefined;
        const rendered = await services.sync.renderCuratedTemplate({
          builtin_id: item.builtin_id,
          id: item.id,
          cluster_id:
            clusterIdNum && Number.isFinite(clusterIdNum)
              ? clusterIdNum
              : undefined,
        });
        const curatedContent = (rendered.content || item.content || '').trim();
        if (!curatedContent) {
          throw new Error(t('curatedInsertFailed'));
        }
        setEditor((prev) => {
          const inserted = buildInsertedCuratedContent(
            prev.content || '',
            section,
            curatedContent,
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
        toast.success(t('curatedInserted'));
      } catch (error) {
        toast.dismiss(loadingToastId);
        toast.error(
          error instanceof Error ? error.message : t('curatedInsertFailed'),
        );
      }
    },
    [editor.clusterId, markEditorDraft, t],
  );

  // 插入精选模板：combo 且编辑器非空时先弹确认框
  // Insert curated template: confirm first when combo would replace non-empty editor
  const insertCuratedTemplate = useCallback(
    async (item: SyncCuratedTemplateView) => {
      const section = (item.section || 'combo') as SyncCuratedSection;
      if (section === 'combo' && (editor.content || '').trim()) {
        setPendingCuratedInsert(item);
        setCuratedReplaceDialogOpen(true);
        return;
      }
      await performInsertCuratedTemplate(item);
    },
    [editor.content, performInsertCuratedTemplate],
  );

  const handleConfirmCuratedReplace = useCallback(() => {
    const item = pendingCuratedInsert;
    setCuratedReplaceDialogOpen(false);
    setPendingCuratedInsert(null);
    if (item) {
      void performInsertCuratedTemplate(item);
    }
  }, [pendingCuratedInsert, performInsertCuratedTemplate]);

  // 打开另存精选弹窗（由选区正文驱动）
  // Open save-as curated dialog from selected content
  const openSaveAsCuratedFromContent = useCallback(
    async (rawInput: string, nameHint?: string) => {
      const raw = rawInput || '';
      if (!raw.trim()) {
        toast.error(t('curatedSaveEmpty'));
        return;
      }
      try {
        const parsed = await services.sync.parseCuratedCombo({content: raw});
        const present = (parsed.present || []).filter(
          (s): s is SyncCuratedSection =>
            s === 'env' ||
            s === 'source' ||
            s === 'transform' ||
            s === 'sink',
        );
        const section: SyncCuratedSection =
          present.length >= 2
            ? 'combo'
            : present.length === 1
              ? present[0]
              : 'combo';
        const sectionFragments: Record<
          Exclude<SyncCuratedSection, 'combo'>,
          string
        > = {
          env: parsed.env || '',
          source: parsed.source || '',
          transform: parsed.transform || '',
          sink: parsed.sink || '',
        };
        const content =
          section === 'combo'
            ? (parsed.combined || raw).trim()
            : (sectionFragments[section] || raw).trim();
        setCuratedSaveContent(content);
        setCuratedSaveSection(section);
        const baseName = (nameHint || '').trim();
        setCuratedSaveName(
          baseName
            ? `${baseName} · curated`
            : `curated-${section}-${Date.now().toString(36)}`,
        );
        setCuratedSaveDescription('');
        setCuratedSaveDialogOpen(true);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('curatedSaveFailed'),
        );
      }
    },
    [t],
  );

  // 编辑器选区右键「另存为精选」：必须先选中文本
  // Editor selection context-menu save-as curated: selection required
  const handleOpenSaveAsCuratedFromSelection = useCallback(async () => {
    const monacoEditor = editorInstanceRef.current;
    const model = monacoEditor?.getModel?.();
    const selection = monacoEditor?.getSelection?.();
    if (!model || !selection || selection.isEmpty?.()) {
      toast.error(t('curatedSaveNeedSelection'));
      return;
    }
    const raw = model.getValueInRange(selection) || '';
    await openSaveAsCuratedFromContent(raw, editor.name);
  }, [editor.name, openSaveAsCuratedFromContent, t]);

  saveAsCuratedFromSelectionRef.current = () => {
    void handleOpenSaveAsCuratedFromSelection();
  };

  // 确认另存为新的精选模板
  // Confirm save-as a new curated template
  const handleConfirmSaveAsCuratedTemplate = useCallback(async () => {
    const name = curatedSaveName.trim();
    const content = curatedSaveContent.trim();
    if (!name || !content) {
      toast.error(t('curatedSaveEmpty'));
      return;
    }
    setCuratedSaving(true);
    try {
      await services.sync.createCuratedTemplate({
        name,
        description: curatedSaveDescription.trim() || undefined,
        section: curatedSaveSection,
        content,
      });
      setCuratedSaveDialogOpen(false);
      setCuratedRefreshToken((n) => n + 1);
      toast.success(t('curatedSaved'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('curatedSaveFailed'),
      );
    } finally {
      setCuratedSaving(false);
    }
  }, [
    curatedSaveContent,
    curatedSaveDescription,
    curatedSaveName,
    curatedSaveSection,
    t,
  ]);

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

  const refreshOptionRuleMarkers = useCallback(async () => {
    const monaco = monacoInstanceRef.current;
    const editor = editorInstanceRef.current;
    const model = editor?.getModel?.();
    if (!monaco || !model || !currentClusterIdRef.current) {
      return;
    }
    const content = model.getValue?.() || '';
    const snapshots = extractPluginBlockSnapshots(content);
    const markers: any[] = [];
    for (const snapshot of snapshots) {
      try {
        const schemaMap = await ensurePluginSchemaRef.current(
          snapshot.pluginType,
          snapshot.factoryIdentifier,
        );
        const diagnostics = buildOptionRuleDiagnostics(
          toOptionRuleAssistSchema(schemaMap),
          snapshot,
        );
        for (const item of diagnostics) {
          markers.push({
            severity:
              item.severity === 'error'
                ? monaco.MarkerSeverity.Error
                : monaco.MarkerSeverity.Warning,
            message: item.message,
            startLineNumber: item.lineNumber,
            startColumn: item.startColumn,
            endLineNumber: item.lineNumber,
            endColumn: item.endColumn,
            source: 'option-rule',
          });
        }
      } catch {
        // 单个插件 schema 失败不阻断其余诊断
      }
    }
    monaco.editor.setModelMarkers(model, OPTION_RULE_MARKER_OWNER, markers);
  }, []);

  const scheduleOptionRuleMarkersRefresh = useCallback(() => {
    if (optionRuleMarkersTimerRef.current) {
      clearTimeout(optionRuleMarkersTimerRef.current);
    }
    optionRuleMarkersTimerRef.current = setTimeout(() => {
      void refreshOptionRuleMarkers();
    }, 350);
  }, [refreshOptionRuleMarkers]);

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
      triggerCharacters: ['=', ' ', '"', '{', '}', '.', '_'],
      provideCompletionItems: async (
        model: any,
        position: {lineNumber: number; column: number},
      ) => {
        const lineContent = model.getLineContent(position.lineNumber);
        const fullContent = model.getValue();

        // 1. 严格仅在 {{ 变量占位符内部才触发变量建议
        // Only suggest variables inside {{ }} placeholders.
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

        // 2. 插件块内写 option key：按 conditionRules 收敛排序
        // Rank option-key completions via conditionRules inside plugin blocks.
        const keyCtx = resolveOptionKeyCompletionContext(
          fullContent,
          position.lineNumber,
          position.column,
        );
        if (
          keyCtx.inKeyRegion &&
          keyCtx.snapshot &&
          currentClusterIdRef.current
        ) {
          try {
            const schemaMap = await ensurePluginSchemaRef.current(
              keyCtx.snapshot.pluginType,
              keyCtx.snapshot.factoryIdentifier,
            );
            const assistSchema = toOptionRuleAssistSchema(schemaMap);
            const ranks = rankOptionKeyCompletions(
              assistSchema,
              keyCtx.snapshot.values,
            );
            const prefix = (keyCtx.prefix || '').toLowerCase();
            const normalizedPrefix = prefix.replace(/[-_.]/g, '');
            const word = model.getWordUntilPosition(position);
            const range = {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: word?.startColumn || position.column,
              endColumn: word?.endColumn || position.column,
            };
            const suggestions = ranks
              .filter((item) => {
                if (!prefix) {
                  return true;
                }
                const keyLower = item.key.toLowerCase();
                const keyNormalized = keyLower.replace(/[-_.]/g, '');
                return (
                  keyLower.startsWith(prefix) ||
                  keyLower.includes(prefix) ||
                  (normalizedPrefix.length > 0 &&
                    keyNormalized.includes(normalizedPrefix))
                );
              })
              .slice(0, 80)
              .map((item, index) => ({
                label: item.key,
                kind: monaco.languages.CompletionItemKind.Property,
                detail: item.active
                  ? item.description || '当前模式相关'
                  : item.description || '其他可选字段',
                insertText: `${item.key} = `,
                filterText: item.key,
                sortText: `${item.sortBucket}-${String(index).padStart(4, '0')}-${item.key}`,
                range,
              }));
            if (suggestions.length > 0) {
              return {suggestions};
            }
          } catch {
            // fall through to enum path
          }
        }

        // 3. 属性赋值右侧：枚举值提示
        // Value-side enum suggestions.
        const assignmentCtx = resolveOptionAssignmentContext(
          lineContent,
          position.column,
        );
        if (!assignmentCtx.inValueRegion || !assignmentCtx.optionKey) {
          return {suggestions: []};
        }
        const optionKey = assignmentCtx.optionKey;
        const context = resolveEditorPluginContext(
          fullContent,
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
        // 已写完枚举且光标在值末尾时不再弹出补全，避免 Enter 再次填充当前值。
        // Skip suggestions when sitting on a finished enum so Enter inserts a newline.
        if (
          isCursorAtFinishedEnumValue(
            assignmentCtx.bounds,
            position.column,
            enumValues,
          )
        ) {
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
          let schemaMeta: any = null;
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
            context.pluginType &&
            context.factoryIdentifier &&
            currentClusterIdRef.current
          ) {
            try {
              const schema = await ensurePluginSchemaRef.current(
                context.pluginType,
                context.factoryIdentifier,
              );
              schemaMeta = schema.__schema_meta__ || null;
              if (!metadata) {
                metadata = schema[optionKey] || null;
              }
            } catch {
              // ignore schema fetch failures for hover
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
          if (metadata.condition_expression || metadata.conditionExpression) {
            lines.push(
              '',
              `条件：\`${String(
                metadata.condition_expression || metadata.conditionExpression,
              )}\``,
            );
          }
          const constraintHints = formatValueConstraintHints(
            optionKey,
            schemaMeta?.value_constraints,
          );
          if (constraintHints.length > 0) {
            lines.push('', `值约束：${constraintHints.join('；')}`);
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
      // 选区右键：另存为精选模板
      // Selection context menu: save as curated template
      try {
        instance.addAction?.({
          id: 'stx.sync.saveAsCurated',
          label: t('saveAsCurated'),
          precondition: 'editorHasSelection',
          contextMenuGroupId: '9_cutcopypaste',
          contextMenuOrder: 1.5,
          run: () => {
            saveAsCuratedFromSelectionRef.current();
          },
        });
      } catch {
        // Monaco action may already exist after remount; ignore.
        // 编辑器重挂载时动作可能已存在，忽略。
      }
      contentChangeDisposableRef.current?.dispose?.();
      modelContentDisposableRef.current?.dispose?.();
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
          const keyCtx = resolveOptionKeyCompletionContext(
            model.getValue?.() || '',
            position.lineNumber,
            position.column,
          );
          if (!inValueRegion && !keyCtx.inKeyRegion) {
            return;
          }
          const shouldTrigger =
            ['=', ' ', '"', '.', '_'].includes(typedText) ||
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
      modelContentDisposableRef.current = instance.onDidChangeModelContent?.(
        () => {
          scheduleOptionRuleMarkersRefresh();
        },
      );
      scheduleOptionRuleMarkersRefresh();
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
    [registerEditorAssistProviders, scheduleOptionRuleMarkersRefresh, t],
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
      modelContentDisposableRef.current?.dispose?.();
      cursorPositionChangeDisposableRef.current?.dispose?.();
      cursorSelectionChangeDisposableRef.current?.dispose?.();
      if (optionRuleMarkersTimerRef.current) {
        clearTimeout(optionRuleMarkersTimerRef.current);
      }
    };
  }, []);

  const buildTaskPayload = useCallback(
    (): CreateSyncTaskRequest =>
      buildTaskPayloadFromEditor(editor, customVariableRows),
    [customVariableRows, editor],
  );

  const persistCurrentFile = useCallback(
    async (publishAfterSave = false) => {
      if (!editor.name.trim()) {
        toast.error(t('fileNameRequired'));
        return null;
      }
      // 仅发布时强制要求正文；草稿允许空内容保存。
      // Require content only when publishing; drafts may persist empty content.
      if (publishAfterSave && !editor.content.trim()) {
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
      setDraftSaveStatus('saving');
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
        setDraftSaveStatus('saved');
        return task;
      } catch (error) {
        setDraftSaveStatus('error');
        toast.error(
          error instanceof Error ? error.message : t('saveFileFailed'),
        );
        return null;
      } finally {
        setSaving(false);
      }
    },
    [buildTaskPayload, editor, loadWorkspace, markEditorDraft, syncOpenTabs, t],
  );

  /**
   * 将内存中所有可编辑 dirty 草稿落盘（离开前「保存并离开」）。
   * Persist every editable dirty in-memory draft (Save & Leave).
   */
  const persistAllDirtyDrafts = useCallback(async (): Promise<boolean> => {
    const drafts = editorDraftsRef.current;
    const dirtyEntries = Object.entries(drafts).filter(
      ([, draft]) =>
        Boolean(draft?.dirty) && draft.editor.canEdit !== false,
    );
    // 先保存当前编辑器（含未写入 drafts 竞态时的最新内容）。
    // Persist the active editor first (latest content, including races not yet in drafts).
    if (editor.id && editor.canEdit !== false && isCurrentDirty) {
      const current = await persistCurrentFile(false);
      if (!current) {
        return false;
      }
    }
    for (const [idText, draft] of dirtyEntries) {
      const taskId = Number(idText);
      if (editor.id && taskId === editor.id) {
        continue;
      }
      if (!draft.editor.name.trim()) {
        toast.error(t('fileNameRequired'));
        setDraftSaveStatus('error');
        return false;
      }
      const customVariableError = validateCustomVariableRows(
        draft.customVariableRows,
        t,
      );
      if (customVariableError) {
        toast.error(customVariableError);
        setDraftSaveStatus('error');
        return false;
      }
      setDraftSaveStatus('saving');
      setSaving(true);
      try {
        const payload = buildTaskPayloadFromEditor(
          draft.editor,
          draft.customVariableRows,
        );
        const task = await services.sync.updateTask(taskId, payload);
        const savedEditor = extractEditorState(task);
        const savedRows = extractVariableRowsFromDefinition(
          task.definition || {},
        );
        markEditorDraft(taskId, savedEditor, savedRows, false);
        setTree((current) => patchTreeNode(current, task));
      } catch (error) {
        setDraftSaveStatus('error');
        toast.error(
          error instanceof Error ? error.message : t('saveFileFailed'),
        );
        return false;
      } finally {
        setSaving(false);
      }
    }
    setDraftSaveStatus('saved');
    return true;
  }, [
    editor.canEdit,
    editor.id,
    isCurrentDirty,
    markEditorDraft,
    persistCurrentFile,
    t,
  ]);

  const discardAllDirtyDrafts = useCallback(() => {
    setEditorDrafts({});
    setDraftSaveStatus('idle');
  }, []);

  const hasUnsavedRisk = useMemo(
    () =>
      hasWorkbenchUnsavedRisk({
        drafts: editorDrafts,
        saveStatus: draftSaveStatus,
      }),
    [draftSaveStatus, editorDrafts],
  );

  // 只读当前文件且无其它可编辑脏草稿时不拦截（AC6）。
  // Skip leave guard when the active file is read-only and no other editable dirty drafts (AC6).
  const leaveGuardEnabled = useMemo(() => {
    if (editor.canEdit === false) {
      return Object.values(editorDrafts).some(
        (draft) => Boolean(draft?.dirty) && draft.editor.canEdit !== false,
      );
    }
    return true;
  }, [editor.canEdit, editorDrafts]);

  const leaveGuard = useWorkbenchLeaveGuard({
    enabled: leaveGuardEnabled,
    hasUnsavedRisk,
  });

  const handleLeaveStay = useCallback(() => {
    leaveGuard.stay();
  }, [leaveGuard]);

  const handleLeaveSaveAndLeave = useCallback(async () => {
    if (leaveActionBusy) {
      return;
    }
    setLeaveActionBusy(true);
    try {
      const ok = await persistAllDirtyDrafts();
      if (!ok) {
        return;
      }
      leaveGuard.proceedToPending();
    } finally {
      setLeaveActionBusy(false);
    }
  }, [leaveActionBusy, leaveGuard, persistAllDirtyDrafts]);

  const handleLeaveDiscardAndLeave = useCallback(() => {
    if (leaveActionBusy) {
      return;
    }
    discardAllDirtyDrafts();
    leaveGuard.proceedToPending();
  }, [discardAllDirtyDrafts, leaveActionBusy, leaveGuard]);

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
          content: '',
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
            content: '',
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
      // 文件夹只记录选中；展开/折叠交给 beUI FileTree 的 expandedIds。
      // Folders only record selection; expand/collapse is owned by beUI FileTree.
      setSelectedFolderId(node.id);
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
    // 打开文件后底部控制台默认停在任务列表，而不是日志。
    // Opening a file keeps the bottom console on the jobs list, not logs.
    setBottomConsoleTab('jobs');
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

  // 每次渲染同步最新 handleSelectNode 到 ref，供深链重放 effect 调用。
  // Sync the latest handleSelectNode into the ref every render for the deep-link replay effect.
  useEffect(() => {
    selectNodeRef.current = handleSelectNode;
  });

  // 深链在已挂载的 /workbench 上再次导航也要生效：loadWorkspace 只在挂载时消费一次
  // deepLinkRef，停留在工作台时再点 ?task&job 链接只会改 searchParams 而不会重挂载，旧链接
  // 会被静默丢弃（表现为「点了告警却找不到运行记录」）。这里监听参数变化重放深链。
  // Deep links must also apply on in-place navigation to an already-mounted /workbench:
  // loadWorkspace consumes deepLinkRef only at mount, so clicking another ?task&job link while
  // staying on the page only changes searchParams without remounting, silently dropping the link
  // (users click an alert and never see the run record). Replay such links on params change.
  useEffect(() => {
    const link = parseJobDeepLink(searchParams);
    if (!link) {
      return;
    }
    const key = `${link.taskId}:${link.jobId ?? 0}`;
    if (appliedDeepLinkKeyRef.current === key) {
      return;
    }
    const node = findTreeNode(tree, link.taskId);
    if (!node || node.node_type !== 'file') {
      // 工作区仍在加载时不判失败：effect 依赖 tree/loading，树就绪后会重跑。
      // Do not warn while the workspace is still loading: this effect re-runs once tree/loading settle.
      if (!loading) {
        appliedDeepLinkKeyRef.current = key;
        toast.warning(tAlert('deepLinkTaskNotFound', {id: link.taskId}));
      }
      return;
    }
    appliedDeepLinkKeyRef.current = key;
    deepLinkRef.current = link;
    void selectNodeRef.current?.(node);
  }, [searchParams, tree, loading, tAlert]);

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
  };

  const handleSave = async () => {
    if (editor.id && !editor.canEdit) {
      toast.error(t('readOnlySaveTooltip'));
      // 拒绝后保存段回到空闲，不会误显示已保存。
      // Reject so the save segment returns to idle instead of showing saved.
      throw new Error(t('readOnlySaveTooltip'));
    }
    // 成功靠顶栏「已保存」，不再额外 toast。
    // Success is the top-bar Saved state; do not add a second toast.
    await persistCurrentFile(false);
  };

  // Button Group 不会自己画 pending。保存中走 pending；结束后文案跟草稿脏状态走。
  // Button Group does not draw pending itself. Pending covers the request; afterwards the label follows draft dirtiness.
  const handleToolbarSave = async () => {
    if (saveAck === 'pending' || draftSaveStatus === 'saving') {
      return;
    }
    setSaveAck('pending');
    try {
      await handleSave();
    } catch {
      // 失败时草稿仍脏，回到空闲后文案是「保存」。
      // On failure the draft stays dirty, so idle renders as Save.
    } finally {
      setSaveAck('idle');
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
      // 详情已在限高弹窗里，不再把整段异常塞进 toast。
      // Details stay in the height-capped dialog; do not dump the full exception into a toast.
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
      // 校验通过时轻提示成功，未通过时详情已在 ValidationDialog 内展示，避免双重弹窗。
      // Toast success when passed; failure details are shown in ValidationDialog to avoid duplicate popups.
      if (result.valid) {
        toast.success(t('validatePassed'));
      }
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
      // 错误已在 ValidationDialog 居中模态展示，不再重复弹出 toast 气泡，避免双重弹窗。
      // Error is displayed in the modal ValidationDialog, omit duplicate toast to prevent double popups.
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
      // 测试通过时轻提示成功，未通过时详情已在 ValidationDialog 内展示，避免双重弹窗。
      // Toast success when passed; failure details are shown in ValidationDialog to avoid duplicate popups.
      if (result.valid) {
        toast.success(t('connectionsPassed'));
      }
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
      // 错误已在 ValidationDialog 居中模态展示，不再重复弹出 toast 气泡，避免双重弹窗。
      // Error is displayed in the modal ValidationDialog, omit duplicate toast to prevent double popups.
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
      retainJob(job);
      setSelectedJobId(job.id);
      setBottomConsoleTab('preview');
      toast.success(t('previewSubmitted'));
    } catch (error) {
      // 长错误进限高诊断弹窗，避免 toast 被堆栈撑满视口。
      // Long failures go to the height-capped dialog so a stack trace cannot stretch the toast.
      setActionError(
        formatSyncUserFacingError(error, t('previewFailed'), t, {preview: true}),
      );
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
      // 新提交的作业留个快照：即便下一次 loadJobs 把它挤出最新 50 条，选中仍可解析。
      // Snapshot the submitted job: even if the next loadJobs pushes it out of the latest 50, the
      // selection keeps resolving.
      retainJob(job);
      setSelectedJobId(job.id);
      if (mode === 'recover') {
        setRecoverSourceId('');
      }
      setBottomConsoleTab('jobs');
      // 结果未知时提示"确认中，勿重复提交"，不当作成功
      // An unknown outcome warns "confirming, do not resubmit" instead of reporting success
      if (isSubmitOutcomeUnknown(job)) {
        toast.warning(
          mode === 'recover'
            ? t('recoverOutcomeUnknownToast')
            : t('submitOutcomeUnknownToast'),
        );
      } else {
        toast.success(
          mode === 'recover' ? t('recoverSubmitted') : t('runSubmitted'),
        );
      }
    } catch (error) {
      // 长错误进限高诊断弹窗，避免 toast 被堆栈撑满视口。
      // Long failures go to the height-capped dialog so a stack trace cannot stretch the toast.
      setActionError(formatSyncUserFacingError(error, t('runFailed'), t));
    } finally {
      setActionPending((current) => (current === 'recover' ? null : current));
    }
  };

  const handleCancelJob = async (jobId: number, stopWithSavepoint = false) => {
    try {
      const cancelled = await services.sync.cancelJob(jobId, {
        stop_with_savepoint: stopWithSavepoint,
      });
      await loadJobs(editor.id || null);
      if (isCancelOutcomeUnknown(cancelled)) {
        toast.warning(t('cancelOutcomeUnknownToast'));
      } else {
        toast.success(
          stopWithSavepoint ? t('savepointStopTriggered') : t('taskStopped'),
        );
      }
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
        retainJob(job);
        setSelectedJobId(job.id);
        setBottomConsoleTab('jobs');
        toast.success(t('recoverSubmitted'));
      } catch (error) {
        // 长错误进限高诊断弹窗，避免 toast 被堆栈撑满视口。
        // Long failures go to the height-capped dialog so a stack trace cannot stretch the toast.
        setActionError(
          formatSyncUserFacingError(error, t('recoverActionLabel'), t),
        );
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

  // 实时可观测性设置写入任务定义，下次提交生效
  // The realtime observability setting is stored in the task definition and applies on the next submit
  const handleObservabilityModeChange = (value: ObservabilityMode) => {
    updateEditor('definition', {
      ...editor.definition,
      observability: value,
    });
  };

  // 提交/停止超时写入任务定义；清空时删除键，回到全局默认
  // Submit/stop timeouts are stored in the task definition; clearing removes the key to fall back to the global default
  const handleTaskTimeoutChange = (key: TaskTimeoutKey, minutes: number | null) => {
    const next = {...editor.definition};
    if (minutes === null) {
      delete next[key];
    } else {
      next[key] = minutes;
    }
    updateEditor('definition', next);
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
      // 入力は #N・内部 id・platform/engine job id のいずれでも受け付ける。
      // Accept #N, internal id, and platform/engine job id alike.
      const raw = trimmed.replace(/^#+/, '');
      let job = null;
      if (/^\d+$/.test(raw)) {
        job =
          (
            await findSyncJobsByIdInput(
              (params) => services.sync.listJobs(params),
              raw,
              {firstOnly: true},
            )
          )[0] ?? null;
      }
      // 非数字入力（タスク名等）の最終手段：キーワード照合。数字は id 系で尽きているので混ぜない。
      // Last resort for non-numeric input (task names): keyword match; digits already exhausted id forms.
      if (!job && !/^\d+$/.test(raw)) {
        const byKeyword = await services.sync.listJobs({
          current: 1,
          size: 1,
          keyword: raw,
        });
        job = byKeyword.items?.[0] || null;
      }
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
      // 定位到的作业可能不在最新 50 条：快照防止选中解析错误回退到 jobs[0]。
      // The located job may sit outside the latest 50: a snapshot keeps the selection from falling back to jobs[0].
      retainJob(job);
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
      const collaboratorIds = Array.isArray(editor.definition?.collaborator_ids)
        ? (editor.definition.collaborator_ids as number[])
        : Array.isArray(editor.definition?.collaborators)
          ? (editor.definition.collaborators as number[])
          : [];
      const permissions = await services.sync.updateTaskPermissions(editor.id, {
        is_public: editor.isPublic ?? true,
        collaborator_ids: collaboratorIds,
      });
      const nextDefinition = {
        ...(currentTask.definition || {}),
        is_public: permissions.is_public,
        collaborator_ids: permissions.collaborator_ids,
        collaborators: permissions.collaborator_ids,
      };
      const updatedTask = {
        ...currentTask,
        is_public: permissions.is_public,
        can_edit: permissions.can_edit,
        is_owner: permissions.is_owner,
        is_collaborator: permissions.is_collaborator,
        definition: nextDefinition,
      };
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

  // 顶栏 Savepoint 恢复是否不可用（本地模式 / 有进行中作业 / 无恢复源）
  // Whether top-bar Savepoint Recover is unavailable (local mode / active job / no recover source)
  const isSavepointRecoverDisabled =
    executionMode === 'local' ||
    hasActiveRun ||
    hasActivePreview ||
    actionPending !== null ||
    preferredRecoverSourceId === null;

  return (
    <div className='arc-compact -mx-2 flex h-[calc(100vh-96px)] min-h-[780px] flex-col gap-2 bg-background/10 lg:-mx-3'>
      <Card className='relative z-20 gap-0 border-border/60 bg-background/85 py-0 shadow-xs'>
        <CardContent className='studio-topbar grid h-12 min-w-0 grid-cols-[minmax(0,max-content)_280px_minmax(max-content,1fr)] items-center gap-3 overflow-visible px-3'>
          {/* 左侧：顶栏最左端左栏开关（#110 AC11）+ 面包屑 / 状态 */}
          {/* Left: leftmost left-sidebar toggle (#110 AC11) + breadcrumb / status */}
          <div className='flex min-w-0 items-center gap-2 text-xs'>
            <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
              <TooltipTrigger asChild>
                <Button
                  size='sm'
                  variant='ghost'
                  aria-label={
                    isLeftSidebarCollapsed
                      ? t('expandLeftSidebar')
                      : t('collapseLeftSidebar')
                  }
                  aria-pressed={!isLeftSidebarCollapsed}
                  data-testid='studio-toggle-left-sidebar'
                  className={cn(
                    'size-9 shrink-0 rounded-[var(--radius-control)] px-0 text-muted-foreground hover:bg-muted hover:text-foreground',
                    // 对齐 Cursor：侧栏开关无描边，展开时只换底色。
                    // Match Cursor: panel toggles have no border; open state is a quiet fill.
                    !isLeftSidebarCollapsed && 'bg-muted text-foreground',
                  )}
                  onClick={handleToggleLeftSidebar}
                >
                  {isLeftSidebarCollapsed ? (
                    <PanelLeftOpen className='size-3.5' />
                  ) : (
                    <PanelLeftClose className='size-3.5' />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {isLeftSidebarCollapsed
                  ? t('expandLeftSidebar')
                  : t('collapseLeftSidebar')}
              </TooltipContent>
            </Tooltip>
            {activeTaskBreadcrumbs.length > 0 ? (
              <div className='flex min-w-0 items-center gap-1.5'>
                {activeTaskBreadcrumbs.slice(0, -1).map((seg, idx) => (
                  <span
                    key={idx}
                    className='flex items-center gap-1.5 text-muted-foreground'
                  >
                    <span className='max-w-[120px] truncate' title={seg}>
                      {seg}
                    </span>
                    <span className='text-muted-foreground/40'>/</span>
                  </span>
                ))}
                <span
                  className='max-w-[180px] truncate font-semibold text-foreground'
                  title={
                    activeTaskBreadcrumbs[activeTaskBreadcrumbs.length - 1]
                  }
                >
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
                    {/* 版本 + 发布状态合并微胶囊 / Merged version + publish status micro-pill */}
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
                      <span className='font-mono'>
                        v{editor.currentVersion || 1}
                      </span>
                      ·
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

                  </div>
                ) : null}
              </div>
            ) : (
              <span className='text-xs text-muted-foreground'>
                {t('noOpenFiles')}
              </span>
            )}
          </div>

          {/* 搜索槽固定 280px 并靠左：壳从左往右在槽内展开，不会盖住保存/运行。 */}
          {/* Fixed 280px slot, left of the actions. The shell grows right inside the slot, never over Save/Run. */}
          <div className='relative z-20 w-[280px] shrink-0 justify-self-start'>
            <ExpandingSearch
              label={t('quickOpen')}
              placeholder={t('searchFileOrJobId')}
              items={quickOpenItems}
              suggestions={quickOpenItems
                .filter((item) => item.id.startsWith('job-'))
                .slice(0, 6)}
              suggestionsLabel={t('recentSuggestions')}
              emptyHint={t('noMatchingFileOrJob')}
              anchor='start'
              expandedWidth={280}
              maxResults={8}
              onSelect={(item) => {
                if (item.id.startsWith('job-')) {
                  void handleLocateJobId(item.id.slice(4));
                  return;
                }
                const node = findTreeNode(tree, Number(item.id.slice(5)));
                if (node) {
                  void handleQuickOpenFile(node);
                }
              }}
              onSubmit={(query) => {
                // 候補に載らない入力（未読み込みの Job ID 等）も Enter で定位に回す。
                // Route input with no matching suggestion (e.g. an unloaded Job ID) to locate on Enter.
                void handleLocateJobId(query);
              }}
              onQueryChange={setQuickOpenQuery}
            />
          </div>

          {/* 右侧按大厂工具栏分组：状态无描边，探查一组，提交/运行一组，侧栏开关 ghost。 */}
          {/* Right cluster like Figma/Cursor: status has no chrome, inspect is one group, persist/run is one group, sidebar is ghost. */}
          <div className='flex min-w-max flex-nowrap items-center justify-end gap-3'>
            {/* 集群是状态，不是按钮，不再套胶囊边框。 */}
            {/* Cluster is status, not a control, so it does not get a pill border. */}
            <div
              className='flex h-9 min-w-0 max-w-[11rem] shrink items-center gap-1.5 px-1 text-xs text-muted-foreground'
              title={
                executionMode === 'local'
                  ? t('localMode')
                  : currentCluster?.name || t('unassignedCluster')
              }
            >
              {executionMode === 'local' ? (
                <>
                  <Cpu className='size-3 shrink-0' />
                  <span className='truncate font-medium'>{t('localMode')}</span>
                </>
              ) : (
                <>
                  <Globe2 className='size-3 shrink-0' />
                  <span className='truncate font-medium'>
                    {currentCluster?.name || t('unassignedCluster')}
                  </span>
                </>
              )}
            </div>

            {/* 探查动作：Arc 展开按钮组。打开结果的动作不写 doneLabel，避免误显示已完成。 */}
            {/* Inspect actions: Arc expanding group. Actions that open a result omit doneLabel so they do not flash as finished. */}
            <ExpandingButtonGroup
              size='sm'
              label={t('inspectActions')}
              defaultExpanded='test'
              items={[
                {
                  id: 'test',
                  label: t('inspectTest'),
                  icon: <PlugZap />,
                  disabled:
                    saving ||
                    !editor.name.trim() ||
                    actionPending !== null ||
                    !editor.canEdit,
                  onSelect: () => handleTestConnections(),
                },
                {
                  id: 'dag',
                  label: 'DAG',
                  icon: <GitBranch />,
                  disabled:
                    saving || !editor.name.trim() || actionPending !== null,
                  onSelect: () => handleBuildDag(),
                },
                {
                  id: 'preview',
                  label: t('preview'),
                  icon: <Eye />,
                  disabled:
                    saving ||
                    hasActiveRun ||
                    hasActivePreview ||
                    actionPending !== null ||
                    !editor.canRun,
                  onSelect: () => handlePreview(),
                },
              ]}
            />

            {/* 仅在失败时展示可重试徽章；保存中/已保存由 Arc 保存段承担，避免双指示。 */}
            {/* Show a retryable badge only on failure; Arc save segment covers saving/saved. */}
            {draftSaveStatus === 'error' ? (
              <DraftSaveStatusBadge
                status={draftSaveStatus}
                onRetry={() => {
                  if (editor.id && !editor.canEdit) {
                    return;
                  }
                  void persistCurrentFile(false);
                }}
              />
            ) : null}

            {/* 保存/发布/运行/停止同一条 Button Group，Savepoint 变体进末尾菜单，避免四颗独立描边按钮。 */}
            {/* Save, publish, run, and stop share one Button Group. Savepoint variants live in the trailing menu so the bar is not four outlined pills. */}
            <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
              <TooltipTrigger asChild>
                <span>
                  <ButtonGroup
                    label={t('toolbarActions')}
                    size='md'
                    items={[
                      {
                        id: 'save',
                        label:
                          saveSegment === 'pending'
                            ? t('savePending')
                            : saveSegment === 'saved'
                              ? t('saveDone')
                              : t('save'),
                        reserve: [t('save'), t('savePending'), t('saveDone')],
                        icon: saveSegment === 'saved' ? <Check /> : <Save />,
                        disabled:
                          saving ||
                          saveAck === 'pending' ||
                          draftSaveStatus === 'saving' ||
                          !editor.name.trim() ||
                          !editor.canEdit,
                        onSelect: () => {
                          void handleToolbarSave();
                        },
                      },
                      {
                        id: 'publish',
                        label: t('publish'),
                        icon: <Upload />,
                        disabled:
                          saving ||
                          publishing ||
                          saveAck === 'pending' ||
                          draftSaveStatus === 'saving' ||
                          !editor.name.trim() ||
                          !editor.canEdit,
                        onSelect: handleOpenPublishDialog,
                      },
                      {
                        id: 'run',
                        label: t('run'),
                        icon: <Play />,
                        disabled:
                          saving ||
                          hasActiveRun ||
                          hasActivePreview ||
                          !editor.canRun,
                        onSelect: () => {
                          void handleRun('run');
                        },
                      },
                      {
                        id: 'stop',
                        label: t('stop'),
                        icon: <Square />,
                        disabled: activeJobs.length === 0,
                        onSelect: () => {
                          void handleStopActiveJob('normal');
                        },
                      },
                    ]}
                    menu={{
                      label: t('moreActions'),
                      items: [
                        {
                          id: 'recover',
                          label: t('savepointRecover'),
                          disabled: isSavepointRecoverDisabled,
                          onSelect: () => {
                            void handleRun('recover');
                          },
                        },
                        {
                          id: 'savepoint-stop',
                          label: t('savepointStop'),
                          disabled:
                            executionMode === 'local' ||
                            activeJobs.length === 0,
                          onSelect: () => {
                            void handleStopActiveJob('savepoint');
                          },
                        },
                      ],
                    }}
                  />
                </span>
              </TooltipTrigger>
              {!editor.canEdit ? (
                <TooltipContent>{t('readOnlySaveTooltip')}</TooltipContent>
              ) : !editor.canRun ? (
                <TooltipContent>{t('readOnlyRunTooltip')}</TooltipContent>
              ) : null}
            </Tooltip>

            {/* 顶栏最右端：右侧高级配置开合（始终可见，#110 AC11） */}
            {/* Rightmost topbar: right advanced sidebar toggle (always visible, #110 AC11) */}
            <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
              <TooltipTrigger asChild>
                <Button
                  size='sm'
                  variant='ghost'
                  aria-label={
                    isRightSidebarCollapsed
                      ? t('expandRightSidebar')
                      : t('collapseRightSidebar')
                  }
                  aria-pressed={!isRightSidebarCollapsed}
                  data-testid='studio-toggle-right-sidebar'
                  className={cn(
                    'size-9 shrink-0 rounded-[var(--radius-control)] px-0 text-muted-foreground hover:bg-muted hover:text-foreground',
                    !isRightSidebarCollapsed && 'bg-muted text-foreground',
                  )}
                  onClick={handleToggleRightSidebar}
                >
                  {isRightSidebarCollapsed ? (
                    <PanelRightOpen className='size-3.5' />
                  ) : (
                    <PanelRightClose className='size-3.5' />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {isRightSidebarCollapsed
                  ? t('expandRightSidebar')
                  : t('collapseRightSidebar')}
              </TooltipContent>
            </Tooltip>
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
          // 三列两行；分隔条叠在卡片接缝上，不另占 6px 轨道。
          // Three columns, two rows; dividers overlay card seams instead of a 6px track.
          'relative grid min-h-0 min-w-0 flex-1',
          !isStudioLayoutResizing &&
            'transition-[grid-template-columns,grid-template-rows] duration-200',
        )}
        style={{
          gridTemplateColumns: buildStudioGridTemplateColumns(
            isLeftSidebarCollapsed,
            isRightSidebarCollapsed,
            leftSidebarWidth,
            rightSidebarWidth,
          ),
          gridTemplateRows: buildStudioGridTemplateRows(
            isConsoleMaximized,
            consoleHeight,
          ),
          columnGap: 0,
          rowGap: 0,
        }}
      >
        <Card
          data-studio-left-sidebar=''
          className={cn(
            'col-start-1 row-start-1 row-span-2 min-w-0 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs',
            isLeftSidebarCollapsed
              ? 'pointer-events-none border-transparent opacity-0'
              : null,
          )}
          aria-hidden={isLeftSidebarCollapsed || undefined}
          // inert：收起后移出 Tab 序（入口在顶栏，#110 AC11 / Codex P2）
          // inert: drop collapsed subtree from tab order (toggle stays on topbar, #110 AC11 / Codex P2)
          inert={isLeftSidebarCollapsed ? true : undefined}
        >
          <CardContent className='flex h-full min-h-0 min-w-0 flex-col p-0'>
            {/* 目录栏只放标题；新建/展开/刷新进菜单，避免四枚图标挤掉「资源」。 */}
            {/* Keep the explorer title row uncrowded; create/expand/refresh live in one menu. */}
            <div className='flex h-9 shrink-0 items-center justify-between gap-2 border-b border-border/60 bg-muted/15 px-2.5'>
              <div className='flex min-w-0 items-center gap-1.5 text-[11px] font-medium text-foreground/80'>
                <FolderTree className='size-3.5 shrink-0 text-primary/80' />
                <span className='truncate'>{t('resources')}</span>
                <span className='rounded-full bg-muted/60 px-1.5 py-0.2 font-mono text-[10px] font-normal text-muted-foreground'>
                  {fileCount}
                </span>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type='button'
                    aria-label={t('explorerActions')}
                    className='flex size-7 shrink-0 items-center justify-center rounded-[var(--radius-control)] text-muted-foreground transition-colors hover:bg-muted/80 hover:text-foreground'
                  >
                    <MoreHorizontal className='size-3.5' />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align='start'
                  side='bottom'
                  className='min-w-48'
                  onCloseAutoFocus={(event) => {
                    // 新建时焦点留给行内输入框；其余菜单项照常把焦点还给触发按钮。
                    // Leave focus on the inline create input; other items restore focus to the trigger as usual.
                    if (skipExplorerMenuFocusRestoreRef.current) {
                      skipExplorerMenuFocusRestoreRef.current = false;
                      event.preventDefault();
                    }
                  }}
                >
                  <DropdownMenuItem
                    onSelect={() => {
                      const folderNode = selectedFolderId
                        ? findTreeNode(tree, selectedFolderId)
                        : null;
                      skipExplorerMenuFocusRestoreRef.current = true;
                      handleInlineCreateFolder(folderNode);
                    }}
                  >
                    <FolderPlus className='size-3.5' />
                    {t('newFolder')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() => {
                      const folderNode = selectedFolderId
                        ? findTreeNode(tree, selectedFolderId)
                        : null;
                      if (!folderNode) {
                        toast.error(t('selectFolderBeforeCreateFile'));
                        return;
                      }
                      skipExplorerMenuFocusRestoreRef.current = true;
                      handleInlineCreateFile(folderNode);
                    }}
                  >
                    <FilePlus2 className='size-3.5' />
                    {t('newFile')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={handleToggleCollapseAll}>
                    <ChevronsUpDown className='size-3.5' />
                    {expandedFolderIds.length > 0
                      ? t('collapseAll')
                      : t('expandAll')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() => {
                      void loadWorkspace(selectedNodeId);
                    }}
                  >
                    <RefreshCw
                      className={cn('size-3.5', loading && 'animate-spin')}
                    />
                    {t('refresh')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            {/* 快捷过滤输入框与范围筛选 */}
            {/* Quick search/filter input and scope selector */}
            <div className='studio-explorer-filter space-y-1.5 border-b border-border/60 bg-muted/10 px-2 py-1.5'>
              <div className='studio-search-field'>
                <SearchField
                  label={t('filterFilesLabel')}
                  value={keyword}
                  onValueChange={setKeyword}
                  placeholder={t('filterFiles')}
                  clearLabel={t('clearSearch')}
                />
              </div>
              {/* 任务过滤：我的=自己创建 + 协作的；全部=不过滤（含他人任务）。去掉含糊的「可见」。 */}
              {/* Mine = created + collaborated. All = unfiltered incl. others'. The old "Visible" label is gone. */}
              <SegmentedControl
                label={t('treeFilterMineAndPublicTooltip')}
                value={treeFilterScope}
                onValueChange={(value) =>
                  handleFilterScopeChange(value as TreeFilterScope)
                }
                options={[
                  {
                    value: 'mine_and_public',
                    label: t('treeFilterMineAndPublic'),
                  },
                  {value: 'all', label: t('treeFilterAll')},
                ]}
              />
            </div>

            {/* 文件树滚动区域 */}
            {/* File tree scrollable viewport */}
            <ScrollArea
              className='min-h-0 min-w-0 flex-1 overflow-x-hidden'
              onContextMenu={(event) =>
                openTreeContextMenu(event, 'root', null)
              }
            >
              <div className='min-w-0 px-1 py-1.5'>
                {loading ? (
                  <div className='flex items-center gap-2 p-3 text-xs text-muted-foreground'>
                    <Loader2 className='size-3.5 animate-spin' />
                    <span>{t('loading')}</span>
                  </div>
                ) : filteredTree.length === 0 ? (
                  <div className='flex flex-col items-start gap-2 p-3'>
                    <p className='text-xs text-muted-foreground'>
                      {t('emptyWorkspace')}
                    </p>
                    <button
                      type='button'
                      aria-label={t('newFolder')}
                      onClick={() => handleInlineCreateFolder(null)}
                      className='text-xs font-medium text-foreground/80 underline-offset-4 hover:underline'
                    >
                      {t('newFolder')}
                    </button>
                  </div>
                ) : (
                  <TreeView
                    nodes={filteredTree}
                    selectedNodeId={selectedNodeId}
                    selectedFolderId={selectedFolderId}
                    expandedFolderIds={expandedFolderIds}
                    onExpandedChange={setExpandedFolderIds}
                    onSelect={handleSelectNode}
                    onContextMenu={openTreeContextMenu}
                    dirtyNodeIds={dirtyNodeIds}
                    renamingNodeId={renamingNodeId}
                    onRenameStart={handleRenameStart}
                    onRenameCommit={handleRenameCommit}
                    onRenameCancel={handleRenameCancel}
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

        {!isLeftSidebarCollapsed ? (
          <StudioSplitHandle
            orientation='vertical'
            label={t('resizeExplorer')}
            value={leftSidebarWidth}
            min={STUDIO_LEFT_WIDTH_MIN}
            max={STUDIO_LEFT_WIDTH_MAX}
            onChange={(next) => setLeftSidebarWidth(clampStudioLeftWidth(next))}
            onDragStart={handleStudioLayoutDragStart}
            onDragEnd={handleStudioLayoutDragEnd}
            style={{left: leftSidebarWidth}}
          />
        ) : null}

        <Card
          className={cn(
            'col-start-2 row-start-1 min-w-0 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs',
            isConsoleMaximized && 'hidden',
          )}
        >
          <CardContent className='flex h-full min-h-0 min-w-0 flex-col p-0'>
            {/* 编辑器打开文件标签栏（Radix Tabs + Motion 指示条 + 右键菜单） */}
            {/* Workbench file tabs (Radix Tabs + Motion indicator + context menu) */}
            <StudioFileTabs
              tabs={openTabs.map((tab) => ({
                id: tab.id,
                name: tab.name,
                dirty: Boolean(editorDrafts[tab.id]?.dirty),
              }))}
              activeTabId={selectedNodeId}
              onSelectTab={(tabId) => void handleSelectTab(tabId)}
              onCloseTab={(tabId) => void handleCloseTab(tabId)}
              onCloseOtherTabs={handleCloseOtherTabs}
              onCloseAllTabs={handleCloseAllTabs}
            />
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

        {!isRightSidebarCollapsed ? (
          <StudioSplitHandle
            orientation='vertical'
            invert
            label={t('resizeAdvanced')}
            value={rightSidebarWidth}
            min={STUDIO_RIGHT_WIDTH_MIN}
            max={STUDIO_RIGHT_WIDTH_MAX}
            onChange={(next) =>
              setRightSidebarWidth(clampStudioRightWidth(next))
            }
            onDragStart={handleStudioLayoutDragStart}
            onDragEnd={handleStudioLayoutDragEnd}
            style={{right: rightSidebarWidth}}
          />
        ) : null}

        <StudioSidebarShell
          data-studio-right-sidebar=''
          className={cn(
            'col-start-3 row-start-1 row-span-2 min-w-0',
            isRightSidebarCollapsed
              ? 'pointer-events-none border-transparent opacity-0'
              : null,
          )}
          ariaHidden={isRightSidebarCollapsed}
          inert={isRightSidebarCollapsed}
          contentMode={
            rightSidebarTab === 'templates' || rightSidebarTab === 'settings'
              ? 'fill'
              : 'scroll'
          }
          flush={
            rightSidebarTab === 'settings' || rightSidebarTab === 'templates'
          }
          rail={
            <>
              <SidebarIconTab
                active={rightSidebarTab === 'settings'}
                icon={<Settings className='size-4' />}
                label={t('settings')}
                onClick={() => setRightSidebarTab('settings')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'templates'}
                icon={<FileStack className='size-4' />}
                label={t('curatedManage')}
                onClick={() => setRightSidebarTab('templates')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'schedule'}
                icon={<Clock3 className='size-4' />}
                label={t('taskSchedule')}
                onClick={() => setRightSidebarTab('schedule')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'versions'}
                icon={<Tags className='size-4' />}
                label={t('versionManagement')}
                onClick={() => setRightSidebarTab('versions')}
              />
              <SidebarIconTab
                active={rightSidebarTab === 'globals'}
                icon={<CurlyBracketsIcon className='size-4' />}
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
              observabilityMode={getObservabilityMode(editor.definition)}
              onObservabilityModeChange={handleObservabilityModeChange}
              submitTimeoutMinutes={getTaskTimeoutMinutes(editor.definition, 'submit_timeout_minutes')}
              stopTimeoutMinutes={getTaskTimeoutMinutes(editor.definition, 'stop_timeout_minutes')}
              timeoutDefaults={taskTimeoutDefaults}
              onTaskTimeoutChange={handleTaskTimeoutChange}
              alertTaskId={editor.id || null}
              alertTaskName={editor.name}
              alertCanEdit={editor.canEdit ?? true}
              alertUsers={workspaceUsers}
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
          ) : rightSidebarTab === 'templates' ? (
            <CuratedTemplatesManagePanel
              refreshToken={curatedRefreshToken}
              clusterId={editor.clusterId}
              onChanged={() => setCuratedRefreshToken((n) => n + 1)}
              onInsert={(item) => void insertCuratedTemplate(item)}
              pluginInsert={{
                executionMode,
                clusterId: editor.clusterId,
                loading: pluginPanelLoading,
                pendingType: pluginTemplatePendingType,
                loadingText: pluginTemplateLoadingText,
                sourceItems: sourceTemplateItems,
                transformItems: transformTemplateItems,
                sinkItems: sinkTemplateItems,
                onInsertPlugin: (pluginType, factoryIdentifier) =>
                  void insertPluginTemplate(pluginType, factoryIdentifier),
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
              hasUnsavedChanges={isScheduleUnsaved(
                extractTaskScheduleValue(editor.definition || {}),
              )}
              taskId={editor.id}
              canEdit={editor.canEdit ?? true}
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

        {!isConsoleMaximized ? (
          <StudioSplitHandle
            orientation='horizontal'
            invert
            label={t('resizeConsole')}
            value={consoleHeight}
            min={STUDIO_CONSOLE_HEIGHT_MIN}
            max={STUDIO_CONSOLE_HEIGHT_MAX}
            onChange={(next) =>
              setConsoleHeight(clampStudioConsoleHeight(next))
            }
            onDragStart={handleStudioLayoutDragStart}
            onDragEnd={handleStudioLayoutDragEnd}
            style={{
              left: isLeftSidebarCollapsed ? 0 : leftSidebarWidth,
              right: isRightSidebarCollapsed ? 0 : rightSidebarWidth,
              bottom: consoleHeight,
            }}
          />
        ) : null}

        <Card
          className={cn(
            'col-start-2 min-w-0 gap-0 overflow-hidden border-border/60 bg-background/85 py-0 shadow-xs',
            isConsoleMaximized ? 'row-start-1 row-span-2' : 'row-start-2',
          )}
        >
          <CardContent className='flex h-full min-h-0 min-w-0 flex-col p-0'>
            {/* 控制台顶部横向选项卡与操作区 */}
            {/* Console top horizontal tab switcher and action area */}
            <div className='studio-sidebar-chrome flex min-h-9 shrink-0 items-center justify-between gap-2 border-b border-border/60 bg-muted/20 px-2 py-1'>
              {/* 底部四个视图用同一套分段控件，不再手写一套胶囊 Tab。 */}
              {/* The four console views share the same SegmentedControl instead of a handmade pill row. */}
              <SegmentedControl
                className='min-w-0'
                label={t('jobs')}
                value={bottomConsoleTab}
                onValueChange={(value) =>
                  setBottomConsoleTab(value as BottomConsoleTab)
                }
                options={[
                  {
                    value: 'jobs',
                    label: t('jobs'),
                    accessory:
                      activeJobs.length > 0 ? (
                        <span className='ml-1 inline-block size-1.5 rounded-full bg-emerald-500 animate-pulse' />
                      ) : undefined,
                  },
                  {value: 'logs', label: t('logs')},
                  {value: 'preview', label: t('preview')},
                  {value: 'checkpoint', label: t('checkpoint')},
                ]}
              />

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
                          'size-7 rounded-[var(--radius-control)] text-muted-foreground hover:text-foreground',
                          isConsoleExpanded && 'text-primary bg-primary/10',
                        )}
                        onClick={handleToggleConsoleHeight}
                      >
                        <ChevronsUpDown className='size-3.5' />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side='left'>
                      {isConsoleExpanded
                        ? t('collapseConsoleHeight')
                        : t('expandConsoleHeight')}
                    </TooltipContent>
                  </Tooltip>
                ) : null}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size='icon'
                      variant='ghost'
                      className='size-7 rounded-[var(--radius-control)] text-muted-foreground hover:text-foreground'
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
            <div
              className={cn(
                'min-h-0 flex-1',
                bottomConsoleTab === 'jobs' || bottomConsoleTab === 'preview'
                  ? 'overflow-hidden'
                  : 'overflow-auto p-3',
              )}
            >
              {bottomConsoleTab === 'jobs' ? (
                // 作业表（task 预设）：单行工具栏在上，表格占据剩余空间（内部横向滚动）。
                // Job table (task preset): the single-row toolbar on top, the table fills the rest
                // (horizontal scroll is handled inside).
                <div className='flex h-full min-h-0 flex-col'>
                  <JobTableToolbar
                    preset='task'
                    query={jobTableQuery}
                    onQueryChange={patchJobTableQuery}
                    statusCounts={jobTableList.statusCounts}
                    testIdPrefix='studio-job'
                  />
                  <JobTable
                    preset='task'
                    data={jobTableList}
                    query={jobTableQuery}
                    onQueryChange={patchJobTableQuery}
                    pageSize={STUDIO_JOB_PAGE_SIZE}
                    sparklines={jobTableSparklines}
                    actor={jobTableActor}
                    initiatorUsers={workspaceUsers}
                    currentUserId={currentUser?.id}
                    currentUsername={
                      currentUser?.username || currentUser?.nickname
                    }
                    activeJobId={selectedJobId}
                    onRowActivate={openJobDetail}
                    onOpenDetail={openJobDetail}
                    onAction={handleJobTableAction}
                    testIdPrefix='studio-job'
                  />
                </div>
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
                  loading={
                    actionPending === 'preview' || previewSnapshotLoading
                  }
                  monacoTheme={monacoTheme}
                  onSelectDataset={setPreviewDatasetName}
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
                  onRefresh={reloadCheckpoint}
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

      <ArcDialog
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
        <ArcDialogContent
          className='arc-compact h-[90vh]! w-[min(96vw,1280px)]!'
          bodyClassName='flex min-h-0 flex-1 flex-col p-0! overflow-hidden'
          title={t('taskSchedule')}
        >
          <div className='min-h-0 flex-1 overflow-y-auto px-5 py-4'>
            <TaskScheduleSidebarPanel
              value={scheduleDraft}
              lastTriggeredAt={
                selectedScheduleNode?.schedule_last_triggered_at
              }
              nextTriggeredAt={
                selectedScheduleNode?.schedule_next_triggered_at
              }
              onChange={handleScheduleDraftChange}
              hasUnsavedChanges={isScheduleUnsaved(scheduleDraft)}
              taskId={editor.id}
              canEdit={editor.canEdit ?? true}
              className='mx-auto w-full max-w-6xl'
            />
          </div>
          <div className='flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3.5'>
            <ArcButton
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => {
                setScheduleDraft(
                  extractTaskScheduleValue(editor.definition || {}),
                );
                setScheduleDialogOpen(false);
              }}
            >
              {t('cancel')}
            </ArcButton>
            <ArcButton
              type='button'
              variant='primary'
              size='sm'
              onClick={handleConfirmScheduleDialog}
            >
              {t('confirm')}
            </ArcButton>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      {/* combo 精选整文替换确认 */}
      {/* Confirm before combo curated replace-all */}
      <ArcDialog
        open={curatedReplaceDialogOpen}
        onOpenChange={(open) => {
          setCuratedReplaceDialogOpen(open);
          if (!open) {
            setPendingCuratedInsert(null);
          }
        }}
      >
        <ArcDialogContent
          role='alertdialog'
          showCloseButton={false}
          className='arc-compact w-[min(calc(100vw-2rem),420px)]!'
          onPointerDownOutside={(event) => event.preventDefault()}
          title={t('curatedReplaceTitle')}
          description={t('curatedReplaceConfirm')}
        >
          <div className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
            <ArcButton
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => {
                setCuratedReplaceDialogOpen(false);
                setPendingCuratedInsert(null);
              }}
            >
              {t('cancel')}
            </ArcButton>
            <ArcButton
              type='button'
              variant='primary'
              size='sm'
              onClick={handleConfirmCuratedReplace}
            >
              {t('curatedReplaceConfirmAction')}
            </ArcButton>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      {/* 另存精选模板弹窗 */}
      {/* Save-as curated template dialog */}
      <ArcDialog
        open={curatedSaveDialogOpen}
        onOpenChange={(open) => {
          if (!curatedSaving) {
            setCuratedSaveDialogOpen(open);
          }
        }}
      >
        <ArcDialogContent
          className='arc-compact w-[min(calc(100vw-2rem),460px)]!'
          aria-describedby={undefined}
          title={t('saveAsCurated')}
        >
          <div className='grid gap-4'>
            <div className='grid gap-2'>
              <div className='flex items-center gap-2'>
                <Label htmlFor='curated-save-name'>
                  {t('curatedSaveName')}
                </Label>
                <HelpHint
                  label={t('saveAsCurated')}
                  content={t('curatedVarsHint')}
                  data-testid='curated-save-vars-hint'
                />
              </div>
              <Input
                id='curated-save-name'
                value={curatedSaveName}
                onChange={(e) => setCuratedSaveName(e.target.value)}
                disabled={curatedSaving}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    void handleConfirmSaveAsCuratedTemplate();
                  }
                }}
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='curated-save-desc'>
                {t('curatedSaveDescription')}
              </Label>
              <Input
                id='curated-save-desc'
                value={curatedSaveDescription}
                onChange={(e) => setCuratedSaveDescription(e.target.value)}
                disabled={curatedSaving}
                placeholder={t('curatedSaveDescriptionPlaceholder')}
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('curatedSaveSection')}</Label>
              <Select
                value={curatedSaveSection}
                onValueChange={(value) =>
                  setCuratedSaveSection(value as SyncCuratedSection)
                }
                disabled={curatedSaving}
              >
                <SelectTrigger className='h-9 w-full text-sm'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='env'>env</SelectItem>
                  <SelectItem value='source'>source</SelectItem>
                  <SelectItem value='transform'>transform</SelectItem>
                  <SelectItem value='sink'>sink</SelectItem>
                  <SelectItem value='combo'>combo</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className='flex items-center justify-end gap-2 border-t border-border/50 pt-4'>
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => setCuratedSaveDialogOpen(false)}
                disabled={curatedSaving}
              >
                {t('cancel')}
              </ArcButton>
              <ArcButton
                type='button'
                variant='primary'
                size='sm'
                loading={curatedSaving}
                onClick={() => void handleConfirmSaveAsCuratedTemplate()}
                disabled={curatedSaving || !curatedSaveName.trim()}
              >
                {t('confirm')}
              </ArcButton>
            </div>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      {/* 发布新版本弹窗 */}
      {/* Publish new version dialog */}
      <ArcDialog
        open={publishDialogOpen}
        onOpenChange={(open) => {
          if (!publishing) {
            setPublishDialogOpen(open);
          }
        }}
      >
        <ArcDialogContent
          className='arc-compact w-[min(calc(100vw-2rem),440px)]!'
          aria-describedby={undefined}
          // 打开时焦点直接进备注输入框，避免自动聚焦到标题旁的说明按钮而弹出 Tooltip
          // Focus the comment input on open so auto-focus doesn't land on the help button and pop its Tooltip
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            document.getElementById('publish-version-comment')?.focus();
          }}
          title={
            // HelpCircle 放在标题文案外侧避免进入弹窗默认可访问名称，保持 Arc Dialog 标题视觉。
            // Keep HelpCircle aligned beside the title inside Arc Dialog's header slot.
            <span className='flex items-center gap-1.5'>
              <span>{t('publishNewVersion')}</span>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type='button'
                    className='text-muted-foreground/60 hover:text-foreground'
                    aria-label={t('helpLabel')}
                  >
                    <HelpCircle className='size-3.5' />
                  </button>
                </TooltipTrigger>
                <TooltipContent
                  side='bottom'
                  className='max-w-[320px] text-xs font-normal leading-relaxed'
                >
                  {t('versionManagementDesc')}
                </TooltipContent>
              </Tooltip>
            </span>
          }
        >
          <div className='space-y-4'>
            {/* 弹窗里的字段和确认也走 Arc，不再混 shadcn 主按钮。 */}
            {/* The dialog field and confirm actions stay Arc instead of mixing a shadcn primary. */}
            <ArcField
              id='publish-version-comment'
              label={t('publishComment')}
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
            <div className='flex items-center justify-end gap-2 border-t border-border/50 pt-4'>
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => setPublishDialogOpen(false)}
                disabled={publishing}
              >
                {t('cancel')}
              </ArcButton>
              <ArcButton
                type='button'
                variant='primary'
                size='sm'
                loading={publishing}
                onClick={() => void handleConfirmPublish()}
              >
                {t('confirm')}
              </ArcButton>
            </div>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      <StudioActionErrorDialog
        error={actionError}
        onOpenChange={(open) => {
          if (!open) {
            setActionError(null);
          }
        }}
      />

      <ArcDialog open={validationOpen} onOpenChange={setValidationOpen}>
        <ArcDialogContent
          className={cn(
            'arc-compact max-h-[85vh]!',
            (validationResult?.checks?.length ?? 0) > 0
              ? 'w-[min(94vw,1100px)]!'
              : 'w-[min(94vw,680px)]!',
          )}
          title={validationTitle}
          description={
            validationResult?.summary &&
            validationResult.summary.replace(/^sync:\s*/, '').trim() !==
              validationTitle.trim()
              ? validationResult.summary.replace(/^sync:\s*/, '')
              : undefined
          }
        >
          <ValidationResultPanel result={validationResult} />
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog
        open={previewRunDialog.open}
        onOpenChange={(open) =>
          setPreviewRunDialog((current) => ({...current, open}))
        }
      >
        <ArcDialogContent
          className='arc-compact w-[min(calc(100vw-2rem),448px)]!'
          title={t('previewSettings')}
          description={t('previewRowLimitDesc')}
        >
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
            <div className='flex justify-end gap-2 border-t border-border/50 pt-4'>
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
                onClick={() =>
                  setPreviewRunDialog((current) => ({...current, open: false}))
                }
              >
                {t('cancel')}
              </ArcButton>
              <ArcButton
                type='button'
                variant='primary'
                size='sm'
                onClick={() => void handleConfirmPreview()}
              >
                {t('startPreview')}
              </ArcButton>
            </div>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog
        open={dagOpen}
        onOpenChange={(open) => {
          setDagOpen(open);
          if (!open) {
            setDagError(null);
          }
        }}
      >
        <ArcDialogContent
          className={cn(
            'arc-compact flex flex-col overflow-hidden',
            dagError
              ? 'h-auto max-h-[85vh]! w-[min(94vw,672px)]!'
              : 'h-[88vh]! w-[min(92vw,1400px)]!',
          )}
          bodyClassName={
            dagError
              ? 'flex min-h-0 flex-1 flex-col overflow-hidden'
              : 'flex min-h-0 flex-1 flex-col p-0! overflow-hidden'
          }
          title={dagError ? 'DAG 生成未通过' : t('dagPreview')}
          description={
            !dagError
              ? t('dagSummary', {
                  nodes: dagNodes.length,
                  edges: dagEdges.length,
                })
              : undefined
          }
        >
          {dagError ? (
            <div className='min-h-0 flex-1 overflow-auto pr-1'>
              <StudioErrorDiagnosticsView error={dagError} />
            </div>
          ) : dagWebUIJob ? (
            <div className='min-h-0 flex-1 px-5 pb-4 pt-3'>
              {/* 编辑器 DAG 与作业详情共用拓扑组件（plan 模式：无指标，列信息与建表计划在节点抽屉里）。 */}
              {/* The editor DAG shares the topology with the job detail (plan mode: no metrics; columns and save-mode plans live in the node drawer). */}
              <JobTopology mode='plan' jobDag={dagWebUIJob.jobDag} warnings={dagWarnings} />
            </div>
          ) : (
            <div className='min-h-0 flex-1 overflow-auto p-5'>
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
            </div>
          )}
        </ArcDialogContent>
      </ArcDialog>

      <JobDetailDialog
        job={detailJob}
        open={detailJob !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDetailJobId(null);
          }
        }}
        tab={detailTab}
        onTabChange={setDetailTab}
        actionContext={{
          isAdmin: currentUser?.is_admin ?? false,
          isOwner: editor.isOwner ?? true,
          canRun: editor.canRun ?? true,
          currentUserId: currentUser?.id,
          disableRecover: hasActiveRun || hasActivePreview,
        }}
        onRecover={handleRecoverFromHistory}
        onCancel={handleCancelJob}
        onSavepointStop={(jobId) => void handleCancelJob(jobId, true)}
        monacoTheme={monacoTheme}
        logsSlot={
          <ConsolePanel
            job={detailJob}
            logsResult={jobLogs}
            loading={logsLoading}
            filterMode={logFilterMode}
            onFilterChange={setLogFilterMode}
            onExpand={() => {
              setLogsDialogOpen(true);
            }}
          />
        }
        checkpointSlot={
          <CheckpointWorkspacePanel
            job={detailJob}
            checkpointSnapshot={checkpointSnapshot}
            loading={checkpointLoading}
            checkpointFiles={checkpointFiles}
            checkpointFilesLoading={checkpointFilesLoading}
            onInspectCheckpointFile={handleInspectCheckpointFile}
            inspectLoadingPath={checkpointInspectDialogLoading}
            onRefresh={reloadCheckpoint}
          />
        }
      />

      <ArcDialog
        open={Boolean(versionPreview)}
        onOpenChange={(open) => {
          if (!open) {
            setVersionPreview(null);
          }
        }}
      >
        <ArcDialogContent
          className='arc-compact w-[min(94vw,1024px)]!'
          title={`${t('versionPreview')}${versionPreview ? ` v${versionPreview.version}` : ''}`}
        >
          <pre className='max-h-[70vh] overflow-auto rounded-lg border p-4 text-xs text-muted-foreground'>
            {versionPreview
              ? versionPreview.content_snapshot
              : t('noVersionContent')}
          </pre>
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog
        open={Boolean(compareVersion)}
        onOpenChange={(open) => {
          if (!open) {
            setCompareVersion(null);
          }
        }}
      >
        <ArcDialogContent
          className='arc-compact h-[88vh]! w-[min(96vw,1320px)]!'
          bodyClassName='flex min-h-0 flex-1 flex-col p-0! overflow-hidden'
          title={`${t('versionCompare')}${compareVersion ? ` v${compareVersion.version}` : ''}`}
        >
          <div className='mx-5 mt-4 flex flex-wrap items-center justify-between gap-3 rounded-t-lg border border-b-0 border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground'>
            <div className='flex items-center gap-2'>
              <GitCompareArrows className='size-4 text-primary' />
              <Badge variant='outline'>Diff</Badge>
              <Badge variant='outline'>{t('readOnly')}</Badge>
              <Badge variant='outline'>Side by Side</Badge>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <div className='flex items-center gap-2 rounded-md border border-border/60 bg-background/70 px-2 py-1'>
                <LayoutPanelTop className='size-3.5' />
                <span>
                  v{compareVersion?.version || '-'} /{' '}
                  {compareVersion?.name_snapshot || t('historicalVersion')}
                </span>
              </div>
              <div className='flex items-center gap-2 rounded-md border border-border/60 bg-background/70 px-2 py-1'>
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
          <div className='mx-5 mb-5 min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
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
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog open={logsDialogOpen} onOpenChange={setLogsDialogOpen}>
        <ArcDialogContent
          className='arc-compact h-[88vh]! w-[min(96vw,1400px)]!'
          bodyClassName='flex min-h-0 flex-1 flex-col gap-3 overflow-hidden'
          aria-describedby={undefined}
          title={t('logViewer')}
        >
          <div className='studio-sidebar-chrome flex flex-wrap items-center justify-between gap-2'>
            <div className='flex items-center gap-2'>
              <div className='studio-search-field w-[320px]'>
                <SearchField
                  label={t('searchLogKeyword')}
                  value={logSearchTerm}
                  onValueChange={setLogSearchTerm}
                  placeholder={t('searchLogKeyword')}
                  clearLabel={t('clearSearch')}
                />
              </div>
              <SegmentedControl
                label={t('logs')}
                value={logFilterMode}
                onValueChange={(value) =>
                  setLogFilterMode(value as LogFilterMode)
                }
                options={[
                  {value: 'all', label: t('all')},
                  {value: 'warn', label: 'WARN'},
                  {value: 'error', label: 'ERROR'},
                ]}
              />
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
          {/* 日志区占满弹窗剩余高度，不再固定 620px 在大屏上留白。/ Fill the dialog's remaining height instead of a fixed 620px. */}
          <div className='min-h-0 flex-1'>
          <VirtualizedLogViewer
            lines={splitLogLines(expandedJobLogs?.logs || jobLogs?.logs || '')}
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
          </div>
        </ArcDialogContent>
      </ArcDialog>

      <CheckpointInspectDialog
        open={checkpointInspectDialogOpen}
        onOpenChange={setCheckpointInspectDialogOpen}
        result={checkpointInspectDialogResult}
        t={t}
        onResultUpdate={setCheckpointInspectDialogResult}
      />

      <ArcDialog
        open={treeDialog.open}
        onOpenChange={(open) => setTreeDialog((prev) => ({...prev, open}))}
      >
        <ArcDialogContent
          className='arc-compact w-[min(calc(100vw-2rem),448px)]!'
          title={
            treeDialog.action === 'create-folder'
              ? t('newFolder')
              : treeDialog.action === 'create-file'
                ? t('newFile')
                : treeDialog.action === 'move'
                  ? t('moveTo')
                  : treeDialog.action === 'delete'
                    ? t('deleteConfirm')
                    : t('rename')
          }
        >
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
            <div className='flex justify-end gap-2 border-t border-border/50 pt-4'>
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
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
              </ArcButton>
              <ArcButton
                type='button'
                variant={treeDialog.action === 'delete' ? 'danger' : 'primary'}
                size='sm'
                onClick={() => void handleTreeDialogSubmit()}
              >
                {t('confirm')}
              </ArcButton>
            </div>
          </div>
        </ArcDialogContent>
      </ArcDialog>

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
        onToggleCollapseAll={handleToggleCollapseAll}
        foldersExpanded={expandedFolderIds.length > 0}
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

      {/* 未落盘草稿离开确认 / Leave confirm when drafts are still unsaved */}
      <WorkbenchLeaveDialog
        open={leaveGuard.leaveDialogOpen}
        busy={leaveActionBusy || draftSaveStatus === 'saving'}
        onOpenChange={(open) => {
          if (!open) {
            handleLeaveStay();
          }
        }}
        onStay={handleLeaveStay}
        onSaveAndLeave={() => {
          void handleLeaveSaveAndLeave();
        }}
        onDiscardAndLeave={handleLeaveDiscardAndLeave}
      />
    </div>
  );
}
