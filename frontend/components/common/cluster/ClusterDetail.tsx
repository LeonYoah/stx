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

/**
 * Cluster Detail Component
 * 集群详情组件
 *
 * Displays detailed information about a cluster including nodes and status.
 * 显示集群的详细信息，包括节点和状态。
 */

import {useState, useEffect, useCallback, useRef} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Input} from '@/components/ui/input';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Pagination} from '@/components/ui/pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  WorkbenchDialogContent,
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {toast} from 'sonner';
import {
  ArrowLeft,
  Play,
  Square,
  RotateCcw,
  Pencil,
  Trash2,
  Plus,
  RefreshCw,
  Server,
  Activity,
  Bug,
  AlertTriangle,
  Loader2,
  File,
  FileText,
  MonitorSmartphone,
  FolderOpen,
  Eye,
  ChevronUp,
  Database,
  Search,
  Layers,
  Copy,
  Check,
  MoreHorizontal,
  Cpu,
  SlidersHorizontal,
  HelpCircle,
} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {Checkbox} from '@/components/ui/checkbox';
import {motion} from 'motion/react';
import {
  ExpandableTextPanel,
  WorkspaceHeader,
  StatPillsBar,
  TableLoadingBar,
  TableSkeletonRows,
} from '@/components/common/layout';
import {CheckpointInspectDialog} from '@/components/common/sync/CheckpointPanels';
import {Skeleton} from '@/components/ui/skeleton';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import services from '@/lib/services';
import {
  ClusterInfo,
  ClusterStatus,
  ClusterStatusInfo,
  NodeInfo,
  NodeStatus,
  NodeRole,
  RuntimeStorageDetails,
  RuntimeStorageSpec,
  RuntimeStorageValidationResult,
  RuntimeStorageListResult,
  RuntimeStoragePreviewResult,
  RuntimeStorageCheckpointInspectResult,
  RuntimeStorageIMAPInspectResult,
  StxJavaProxyLogPreviewResult,
  StxJavaProxyStatus,
} from '@/lib/services/cluster/types';
import type {UpgradeTaskSummary} from '@/lib/services/st-upgrade';
import type {DiagnosticsErrorGroup} from '@/lib/services/diagnostics';
import type {ClusterMonitoringOverviewData} from '@/lib/services/monitoring';
import {
  getExecutionStatusLabel,
  getStatusBadgeVariant as getUpgradeStatusBadgeVariant,
} from './upgrade/utils';
import {EditClusterDialog} from './EditClusterDialog';
import {AddNodeDialog} from './AddNodeDialog';
import {RemoveNodeDialog} from './RemoveNodeDialog';
import {ScaleOperationProgress} from './scale/ScaleOperationProgress';
import type {ScaleOperationInfo} from '@/lib/services/cluster/scale.types';
import {EditNodeDialog} from './EditNodeDialog';
import {ClusterPlugins} from './ClusterPlugins';
import {ClusterConfigs} from './ClusterConfigs';
import {EngineEventChannelCard} from './EngineEventChannelCard';
import {SeaTunnelObserveMenu} from './SeaTunnelObserveMenu';
import {RuntimeStorageEditor} from './RuntimeStorageEditor';
import {RuntimeStorageFileExplorer} from './RuntimeStorageFileExplorer';
import {MonitorConfigPanel} from './MonitorConfigPanel';
import {ProcessEventList} from './ProcessEventList';
import {ClusterActions} from './ClusterActions';
import {ClusterDetailSkeleton} from './ClusterDetailSkeleton';
import {ClusterNodeLogDialog} from './ClusterNodeLogDialog';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';
import {formatDateTimeTz} from '@/lib/timezone';

interface ClusterDetailProps {
  clusterId: number;
}

type ClusterDetailTab =
  | 'nodes'
  | 'overview'
  | 'storage'
  | 'proxy'
  | 'monitoring'
  | 'webui'
  | 'plugins'
  | 'configs'
  | 'diagnostics'
  | 'upgrades';

/**
 * Get status badge variant
 * 获取状态徽章变体
 */
function getStatusBadgeVariant(
  status: ClusterStatus | NodeStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case ClusterStatus.RUNNING:
    case NodeStatus.RUNNING:
      return 'default';
    case ClusterStatus.CREATED:
    case ClusterStatus.STOPPED:
    case NodeStatus.PENDING:
    case NodeStatus.STOPPED:
      return 'secondary';
    case ClusterStatus.DEPLOYING:
    case NodeStatus.INSTALLING:
      return 'outline';
    case ClusterStatus.ERROR:
    case NodeStatus.ERROR:
      return 'destructive';
    case NodeStatus.OFFLINE:
      return 'outline';
    default:
      return 'secondary';
  }
}


/**
 * Get role translation key
 * 获取角色翻译键
 * Handles special case for "master/worker" role
 * 处理 "master/worker" 角色的特殊情况
 */
function getRoleTranslationKey(role: string): string {
  if (!role || typeof role !== 'string') {
    return 'undefined';
  }
  if (role === 'master/worker') {
    return 'masterWorker';
  }
  if (role === 'master' || role === 'worker') {
    return role;
  }
  return 'undefined';
}

function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) {
    return '-';
  }
  if (bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function trimTrailingSlashes(value?: string): string {
  if (!value) {
    return '';
  }
  return value.replace(/\/+$/, '');
}

function parentBrowsePath(currentPath?: string, rootPath?: string): string {
  const current = trimTrailingSlashes(currentPath);
  const root = trimTrailingSlashes(rootPath);
  if (!current) {
    return rootPath || '';
  }
  if (!root || current === root) {
    return rootPath || currentPath || '';
  }
  const index = current.lastIndexOf('/');
  if (index < 0) {
    return rootPath || '';
  }
  const parent = current.slice(0, index);
  if (!parent || (root && parent.length < root.length)) {
    return rootPath || '';
  }
  return parent;
}

interface ClusterRuntimeConfigView {
  enableHTTP?: boolean;
  jobLogMode?: string;
}

function extractClusterRuntimeConfig(
  config?: ClusterInfo['config'] | null,
): ClusterRuntimeConfigView {
  if (!config || typeof config !== 'object') {
    return {};
  }
  const runtime =
    'runtime' in config && config.runtime && typeof config.runtime === 'object'
      ? (config.runtime as Record<string, unknown>)
      : null;
  if (!runtime) {
    return {};
  }
  return {
    enableHTTP:
      typeof runtime.enable_http === 'boolean'
        ? runtime.enable_http
        : undefined,
    jobLogMode:
      typeof runtime.job_log_mode === 'string'
        ? runtime.job_log_mode
        : undefined,
  };
}

function pickWebUINode(
  nodes: NodeInfo[],
  clusterVersion?: string,
  enableHTTP?: boolean,
): NodeInfo | null {
  if (!isSeatunnelVersionAtLeast(clusterVersion || '', '2.3.9')) {
    return null;
  }
  if (enableHTTP === false) {
    return null;
  }

  const candidates = nodes.filter(
    (node) =>
      node.api_port > 0 &&
      (node.role === 'master' || node.role === 'master/worker'),
  );
  return (
    candidates.find((node) => node.is_online !== false) || candidates[0] || null
  );
}

/**
 * Cluster Detail Component
 * 集群详情组件
 */
export function ClusterDetail({clusterId}: ClusterDetailProps) {
  const t = useTranslations();
  // 复用工作台 Checkpoint 详情弹窗文案 / Reuse workbench Checkpoint dialog copy
  const tCheckpoint = useTranslations('workbenchStudio');
  const router = useRouter();

  // Data state / 数据状态
  const [cluster, setCluster] = useState<ClusterInfo | null>(null);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [clusterStatus, setClusterStatus] = useState<ClusterStatusInfo | null>(
    null,
  );
  const [upgradeTasks, setUpgradeTasks] = useState<UpgradeTaskSummary[]>([]);
  const [upgradeTasksLoading, setUpgradeTasksLoading] = useState(false);
  const [upgradeTasksTotal, setUpgradeTasksTotal] = useState(0);
  const [upgradeTasksPage, setUpgradeTasksPage] = useState(1);
  const [upgradeTasksPageSize] = useState(10);
  const [diagnosticsGroups, setDiagnosticsGroups] = useState<
    DiagnosticsErrorGroup[]
  >([]);
  const [diagnosticsGroupTotal, setDiagnosticsGroupTotal] = useState(0);
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
  const [monitoringOverview, setMonitoringOverview] =
    useState<ClusterMonitoringOverviewData | null>(null);
  const [runtimeStorage, setRuntimeStorage] =
    useState<RuntimeStorageDetails | null>(null);
  const [stxJavaProxy, setStxJavaProxy] = useState<StxJavaProxyStatus | null>(
    null,
  );
  const [stxJavaProxyLoading, setStxJavaProxyLoading] = useState(false);
  const [stxJavaProxyLogLoading, setStxJavaProxyLogLoading] = useState(false);
  const [stxJavaProxyLogOpen, setStxJavaProxyLogOpen] = useState(false);
  const [stxJavaProxyLogResult, setStxJavaProxyLogResult] =
    useState<StxJavaProxyLogPreviewResult | null>(null);
  const [stxJavaProxyOperating, setStxJavaProxyOperating] = useState<
    'start' | 'stop' | 'restart' | null
  >(null);
  const [stxJavaProxyConfigOpen, setStxJavaProxyConfigOpen] = useState(false);
  const [stxJavaProxyJvmPreset, setStxJavaProxyJvmPreset] = useState<
    '512m' | '1024m' | '2048m' | '4096m' | 'custom'
  >('512m');
  const [stxJavaProxyJvmOptsInput, setStxJavaProxyJvmOptsInput] = useState('');
  const [stxJavaProxyRestartOnSave, setStxJavaProxyRestartOnSave] =
    useState(true);
  const [stxJavaProxySaving, setStxJavaProxySaving] = useState(false);
  const [runtimeStorageLoading, setRuntimeStorageLoading] = useState(false);
  const [runtimeStorageValidationLoading, setRuntimeStorageValidationLoading] =
    useState<'checkpoint' | 'imap' | null>(null);
  const [runtimeStorageValidation, setRuntimeStorageValidation] = useState<
    Partial<Record<'checkpoint' | 'imap', RuntimeStorageValidationResult>>
  >({});
  const [runtimeStorageListingLoading, setRuntimeStorageListingLoading] =
    useState<'checkpoint' | 'imap' | null>(null);
  const [runtimeStorageListing, setRuntimeStorageListing] = useState<
    Partial<Record<'checkpoint' | 'imap', RuntimeStorageListResult>>
  >({});
  // 运行时存储目录列表轻量内存缓存，提升在各层级目录间前进/后退/点击面包屑时的瞬时感知。
  // Lightweight in-memory directory listing cache to make back/forward navigation instant.
  const runtimeStorageListCacheRef = useRef<Map<string, RuntimeStorageListResult>>(
    new Map(),
  );
  const [runtimeStorageBrowsePath, setRuntimeStorageBrowsePath] = useState<
    Partial<Record<'checkpoint' | 'imap', string>>
  >({});
  const [runtimeStorageSearch, setRuntimeStorageSearch] = useState<
    Partial<Record<'checkpoint' | 'imap', string>>
  >({});
  const [runtimeStoragePage, setRuntimeStoragePage] = useState<
    Partial<Record<'checkpoint' | 'imap', number>>
  >({});
  const [runtimeStoragePreviewLoading, setRuntimeStoragePreviewLoading] =
    useState<string | null>(null);
  const [runtimeStoragePreview, setRuntimeStoragePreview] =
    useState<RuntimeStoragePreviewResult | null>(null);
  const [runtimeStoragePreviewOpen, setRuntimeStoragePreviewOpen] =
    useState(false);
  const [checkpointInspectLoading, setCheckpointInspectLoading] = useState<
    string | null
  >(null);
  const [checkpointInspectResult, setCheckpointInspectResult] =
    useState<RuntimeStorageCheckpointInspectResult | null>(null);
  const [checkpointInspectOpen, setCheckpointInspectOpen] = useState(false);
  const [imapInspectLoading, setImapInspectLoading] = useState<string | null>(
    null,
  );
  const [imapInspectResult, setImapInspectResult] =
    useState<RuntimeStorageIMAPInspectResult | null>(null);
  const [imapInspectOpen, setImapInspectOpen] = useState(false);
  const [imapCleanupOpen, setImapCleanupOpen] = useState(false);
  const [imapCleanupRunning, setImapCleanupRunning] = useState(false);
  const [inspectionStarting, setInspectionStarting] = useState(false);
  const [loading, setLoading] = useState(true);
  const [isOperating, setIsOperating] = useState(false);

  // Dialog state / 对话框状态
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [isAddNodeDialogOpen, setIsAddNodeDialogOpen] = useState(false);
  const [isEditNodeDialogOpen, setIsEditNodeDialogOpen] = useState(false);
  const [nodeToEdit, setNodeToEdit] = useState<NodeInfo | null>(null);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [forceDelete, setForceDelete] = useState(false);
  const [nodeToRemove, setNodeToRemove] = useState<NodeInfo | null>(null);
  const [activeTab, setActiveTab] = useState<ClusterDetailTab>('overview');

  // Node selection state / 节点选择状态
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<number>>(
    new Set(),
  );
  const [nodeOperating, setNodeOperating] = useState<number | null>(null);
  // Confirmation for batch start/stop/restart (user must confirm)
  const [confirmBatchOp, setConfirmBatchOp] = useState<
    'start' | 'stop' | 'restart' | null
  >(null);
  // Confirmation for single node start/stop/restart
  const [confirmNodeOp, setConfirmNodeOp] = useState<{
    op: 'start' | 'stop' | 'restart';
    node: NodeInfo;
  } | null>(null);
  // 日志查看弹窗状态 / Node log dialog state
  const [isLogDialogOpen, setIsLogDialogOpen] = useState(false);
  const [logNodeInfo, setLogNodeInfo] = useState<NodeInfo | null>(null);

  // 平滑刷新与节点筛选状态 / Smooth refresh and node filter state
  const [refreshing, setRefreshing] = useState(false);
  const [nodeSearchTerm, setNodeSearchTerm] = useState('');
  const [nodeStatusFilter, setNodeStatusFilter] = useState<string>('all');
  // 存储 Tab 内 Checkpoint / IMAP 焦点切换 / Checkpoint vs IMAP focus inside storage tab
  const [storageFocus, setStorageFocus] = useState<'checkpoint' | 'imap'>(
    'checkpoint',
  );
  const [copiedInstallDir, setCopiedInstallDir] = useState(false);
  // 日志输出模式切换状态 / Job log mode switching state
  const [logModeSwitching, setLogModeSwitching] = useState(false);

  /**
   * Load cluster data
   * 加载集群数据
   */
  const loadClusterData = useCallback(
    async (isSilent = false) => {
      if (!isSilent) {
        setLoading(true);
      }
      try {
        const [clusterResult, nodesResult, statusResult] = await Promise.all([
          services.cluster.getClusterSafe(clusterId),
          services.cluster.getNodesSafe(clusterId),
          services.cluster.getClusterStatusSafe(clusterId),
        ]);

        if (clusterResult.success && clusterResult.data) {
          setCluster(clusterResult.data);
        } else {
          toast.error(clusterResult.error || t('cluster.loadError'));
        }

        if (nodesResult.success && nodesResult.data) {
          setNodes(nodesResult.data);
        }

        if (statusResult.success && statusResult.data) {
          setClusterStatus(statusResult.data);
        }
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t('cluster.loadError'),
        );
      } finally {
        if (!isSilent) {
          setLoading(false);
        }
      }
    },
    [clusterId, t],
  );

  // 进行中的伸缩操作（页面刷新后恢复）/ Scaling operation in progress (restored after a page refresh)
  const [runningScaleOperation, setRunningScaleOperation] =
    useState<ScaleOperationInfo | null>(null);
  const [scaleProgressOpen, setScaleProgressOpen] = useState(false);

  /**
   * 查询当前集群进行中的伸缩操作；失败时静默（横幅只是辅助入口）。
   * Load the scaling operation running on this cluster; failures are silent (the banner is only a shortcut).
   */
  const loadRunningScaleOperation = useCallback(async () => {
    try {
      const running = await services.clusterScale.listOperations(
        clusterId,
        'running',
        1,
      );
      setRunningScaleOperation(running[0] ?? null);
    } catch {
      setRunningScaleOperation(null);
    }
  }, [clusterId]);

  useEffect(() => {
    void loadRunningScaleOperation();
  }, [loadRunningScaleOperation]);

  /**
   * 横幅中的操作结束：刷新集群数据并收起横幅。
   * The banner's operation finished: refresh cluster data and hide the banner.
   */
  const handleRunningScaleFinished = (operation: ScaleOperationInfo) => {
    loadClusterData(true);
    if (operation.status !== 'running') {
      setRunningScaleOperation(operation.status === 'failed' ? operation : null);
    }
  };

  const loadDiagnosticsSummary = useCallback(async () => {
    setDiagnosticsLoading(true);
    try {
      const result = await services.diagnostics.getErrorGroupsSafe({
        cluster_id: clusterId,
        page: 1,
        page_size: 5,
      });
      if (!result.success || !result.data) {
        setDiagnosticsGroups([]);
        setDiagnosticsGroupTotal(0);
        return;
      }
      setDiagnosticsGroups(result.data.items || []);
      setDiagnosticsGroupTotal(result.data.total || 0);
    } finally {
      setDiagnosticsLoading(false);
    }
  }, [clusterId]);

  const loadMonitoringOverview = useCallback(async () => {
    const result = await services.monitoring.getClusterOverviewSafe(clusterId);
    if (!result.success || !result.data) {
      setMonitoringOverview(null);
      return;
    }
    setMonitoringOverview(result.data);
  }, [clusterId]);

  const loadRuntimeStorageList = useCallback(
    async (
      kind: 'checkpoint' | 'imap',
      path?: string,
      forceRefresh = false,
    ) => {
      const cacheKey = `${clusterId}:${kind}:${path ?? ''}`;
      if (!forceRefresh && runtimeStorageListCacheRef.current.has(cacheKey)) {
        const cached = runtimeStorageListCacheRef.current.get(cacheKey);
        if (cached) {
          if (path !== undefined) {
            setRuntimeStorageBrowsePath((prev) => ({...prev, [kind]: path}));
          }
          setRuntimeStoragePage((prev) => ({...prev, [kind]: 1}));
          setRuntimeStorageListing((prev) => ({...prev, [kind]: cached}));
          return;
        }
      }

      setRuntimeStorageListingLoading(kind);
      try {
        if (path !== undefined) {
          setRuntimeStorageBrowsePath((prev) => ({...prev, [kind]: path}));
        }
        setRuntimeStoragePage((prev) => ({...prev, [kind]: 1}));
        const result = await services.cluster.listRuntimeStorageSafe(
          clusterId,
          kind,
          {
            path,
            limit: 200,
          },
        );
        if (!result.success || !result.data) {
          setRuntimeStorageListing((prev) => ({...prev, [kind]: undefined}));
          return;
        }
        runtimeStorageListCacheRef.current.set(cacheKey, result.data);
        setRuntimeStorageListing((prev) => ({...prev, [kind]: result.data}));
      } finally {
        setRuntimeStorageListingLoading(null);
      }
    },
    [clusterId],
  );

  const loadRuntimeStorage = useCallback(
    async (forceRefresh = false) => {
      if (forceRefresh) {
        runtimeStorageListCacheRef.current.clear();
      }
      setRuntimeStorageLoading(true);
      try {
        const result = await services.cluster.getRuntimeStorageSafe(clusterId);
        if (!result.success || !result.data) {
          setRuntimeStorage(null);
          return;
        }
        setRuntimeStorage(result.data);
        setRuntimeStorageBrowsePath({
          checkpoint: result.data.checkpoint?.namespace || '',
          imap: result.data.imap?.namespace || '',
        });
        if (result.data.checkpoint?.enabled) {
          void loadRuntimeStorageList(
            'checkpoint',
            result.data.checkpoint.namespace,
            forceRefresh,
          );
        }
        if (result.data.imap?.enabled) {
          void loadRuntimeStorageList(
            'imap',
            result.data.imap.namespace,
            forceRefresh,
          );
        }
      } finally {
        setRuntimeStorageLoading(false);
      }
    },
    [clusterId, loadRuntimeStorageList],
  );

  const handlePreviewRuntimeStorage = useCallback(
    async (kind: 'checkpoint' | 'imap', path: string) => {
      setRuntimeStoragePreviewLoading(`${kind}:${path}`);
      try {
        const result = await services.cluster.previewRuntimeStorageSafe(
          clusterId,
          kind,
          {
            path,
            max_bytes: 64 * 1024,
          },
        );
        if (!result.success || !result.data) {
          toast.error(
            result.error || t('cluster.runtimeStorage.previewFailed'),
          );
          return;
        }
        setRuntimeStoragePreview(result.data);
        setRuntimeStoragePreviewOpen(true);
      } finally {
        setRuntimeStoragePreviewLoading(null);
      }
    },
    [clusterId, t],
  );

  const handleInspectCheckpoint = useCallback(
    async (path: string) => {
      setCheckpointInspectLoading(path);
      try {
        const result =
          await services.cluster.inspectCheckpointRuntimeStorageSafe(
            clusterId,
            {path},
          );
        if (!result.success || !result.data) {
          toast.error(
            result.error || t('cluster.runtimeStorage.inspectFailed'),
          );
          return;
        }
        setCheckpointInspectResult(result.data);
        setCheckpointInspectOpen(true);
      } finally {
        setCheckpointInspectLoading(null);
      }
    },
    [clusterId, t],
  );

  const handleInspectIMAPWAL = useCallback(
    async (path: string) => {
      setImapInspectLoading(path);
      try {
        const result = await services.cluster.inspectIMAPRuntimeStorageSafe(
          clusterId,
          path,
        );
        if (!result.success || !result.data) {
          toast.error(
            result.error || t('cluster.runtimeStorage.inspectWalFailed'),
          );
          return;
        }
        setImapInspectResult(result.data);
        setImapInspectOpen(true);
      } finally {
        setImapInspectLoading(null);
      }
    },
    [clusterId, t],
  );

  const loadStxJavaProxyStatus = useCallback(async () => {
    setStxJavaProxyLoading(true);
    try {
      const result =
        await services.cluster.getStxJavaProxyStatusSafe(clusterId);
      if (!result.success || !result.data) {
        setStxJavaProxy(null);
        return;
      }
      setStxJavaProxy(result.data);
    } finally {
      setStxJavaProxyLoading(false);
    }
  }, [clusterId]);

  const handleStxJavaProxyOperation = useCallback(
    async (operation: 'start' | 'stop' | 'restart') => {
      setStxJavaProxyOperating(operation);
      try {
        const result =
          operation === 'start'
            ? await services.cluster.startStxJavaProxySafe(clusterId)
            : operation === 'stop'
              ? await services.cluster.stopStxJavaProxySafe(clusterId)
              : await services.cluster.restartStxJavaProxySafe(clusterId);
        if (!result.success || !result.data) {
          toast.error(
            result.error || t(`cluster.stxJavaProxy.${operation}Error`),
          );
          return;
        }
        setStxJavaProxy(result.data);
        toast.success(
          result.data.message || t(`cluster.stxJavaProxy.${operation}Success`),
        );
      } finally {
        setStxJavaProxyOperating(null);
      }
    },
    [clusterId, t],
  );

  const handlePreviewStxJavaProxyLog = useCallback(async () => {
    setStxJavaProxyLogLoading(true);
    try {
      const result = await services.cluster.previewStxJavaProxyServiceLogSafe(
        clusterId,
        {
          lines: 300,
        },
      );
      if (!result.success || !result.data) {
        toast.error(
          result.error || t('cluster.stxJavaProxy.viewRuntimeLogError'),
        );
        return;
      }
      setStxJavaProxyLogResult(result.data);
      setStxJavaProxyLogOpen(true);
    } finally {
      setStxJavaProxyLogLoading(false);
    }
  }, [clusterId, t]);

  const handleOpenStxJavaProxyConfig = useCallback(() => {
    const currentOpts = (stxJavaProxy?.jvm_opts || '').trim();
    setStxJavaProxyJvmOptsInput(currentOpts);
    setStxJavaProxyRestartOnSave(true);
    if (!currentOpts || currentOpts.includes('-Xmx512m')) {
      setStxJavaProxyJvmPreset('512m');
    } else if (
      currentOpts.includes('-Xmx1024m') ||
      currentOpts.includes('-Xmx1g') ||
      currentOpts.includes('-Xmx1G')
    ) {
      setStxJavaProxyJvmPreset('1024m');
    } else if (
      currentOpts.includes('-Xmx2048m') ||
      currentOpts.includes('-Xmx2g') ||
      currentOpts.includes('-Xmx2G')
    ) {
      setStxJavaProxyJvmPreset('2048m');
    } else if (
      currentOpts.includes('-Xmx4096m') ||
      currentOpts.includes('-Xmx4g') ||
      currentOpts.includes('-Xmx4G')
    ) {
      setStxJavaProxyJvmPreset('4096m');
    } else {
      setStxJavaProxyJvmPreset('custom');
    }
    setStxJavaProxyConfigOpen(true);
  }, [stxJavaProxy?.jvm_opts]);

  const handleSelectJvmPreset = useCallback(
    (preset: '512m' | '1024m' | '2048m' | '4096m' | 'custom') => {
      setStxJavaProxyJvmPreset(preset);
      switch (preset) {
        case '512m':
          setStxJavaProxyJvmOptsInput('-Xms64m -Xmx512m');
          break;
        case '1024m':
          setStxJavaProxyJvmOptsInput('-Xms128m -Xmx1024m');
          break;
        case '2048m':
          setStxJavaProxyJvmOptsInput('-Xms256m -Xmx2048m');
          break;
        case '4096m':
          setStxJavaProxyJvmOptsInput('-Xms512m -Xmx4096m');
          break;
        case 'custom':
          break;
      }
    },
    [],
  );

  const handleSaveStxJavaProxyConfig = useCallback(async () => {
    setStxJavaProxySaving(true);
    try {
      const result = await services.cluster.updateStxJavaProxyConfigSafe(
        clusterId,
        {
          jvm_opts: stxJavaProxyJvmOptsInput.trim(),
          restart: stxJavaProxyRestartOnSave,
        },
      );
      if (!result.success || !result.data) {
        toast.error(
          result.error || t('cluster.stxJavaProxy.configFailed'),
        );
        return;
      }
      setStxJavaProxy(result.data);
      toast.success(
        result.data.message || t('cluster.stxJavaProxy.configSuccess'),
      );
      setStxJavaProxyConfigOpen(false);
      void loadStxJavaProxyStatus();
    } finally {
      setStxJavaProxySaving(false);
    }
  }, [
    clusterId,
    stxJavaProxyJvmOptsInput,
    stxJavaProxyRestartOnSave,
    t,
    loadStxJavaProxyStatus,
  ]);

  const handleStartInspection = useCallback(async () => {
    setInspectionStarting(true);
    try {
      const result = await services.diagnostics.startInspectionSafe({
        cluster_id: clusterId,
        trigger_source: 'cluster_detail',
      });
      if (!result.success || !result.data?.report) {
        toast.error(
          result.error || t('diagnosticsCenter.inspections.startError'),
        );
        return;
      }
      toast.success(t('diagnosticsCenter.inspections.startSuccess'));
      router.push(
        `/diagnostics?tab=inspections&cluster_id=${clusterId}&report_id=${result.data.report.id}&source=cluster-detail`,
      );
    } finally {
      setInspectionStarting(false);
    }
  }, [clusterId, router, t]);

  /**
   * Load upgrade task history
   * 加载升级任务记录
   */
  const loadUpgradeTasks = useCallback(
    async (page: number, pageSize: number) => {
      setUpgradeTasksLoading(true);
      try {
        const result = await services.stUpgrade.listTasksSafe({
          cluster_id: clusterId,
          page,
          page_size: pageSize,
        });
        if (!result.success || !result.data) {
          toast.error(result.error || t('stUpgrade.loadUpgradeRecordsFailed'));
          return;
        }
        setUpgradeTasks(result.data.items);
        setUpgradeTasksTotal(result.data.total);
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('stUpgrade.loadUpgradeRecordsFailed'),
        );
      } finally {
        setUpgradeTasksLoading(false);
      }
    },
    [clusterId, t],
  );

  useEffect(() => {
    void loadClusterData();
    void loadDiagnosticsSummary();
    void loadMonitoringOverview();
    void loadRuntimeStorage();
    void loadStxJavaProxyStatus();
  }, [
    loadClusterData,
    loadDiagnosticsSummary,
    loadMonitoringOverview,
    loadRuntimeStorage,
    loadStxJavaProxyStatus,
  ]);

  useEffect(() => {
    void loadUpgradeTasks(upgradeTasksPage, upgradeTasksPageSize);
  }, [loadUpgradeTasks, upgradeTasksPage, upgradeTasksPageSize]);

  useEffect(() => {
    if (activeTab === 'storage') {
      void loadRuntimeStorage();
      return;
    }
    if (activeTab === 'proxy') {
      void loadStxJavaProxyStatus();
    }
  }, [activeTab, loadRuntimeStorage, loadStxJavaProxyStatus]);

  const openUpgradeTaskDetail = useCallback(
    (task: UpgradeTaskSummary) => {
      router.push(
        `/clusters/${clusterId}/upgrade/execute?planId=${task.plan_id}&taskId=${task.id}`,
      );
    },
    [clusterId, router],
  );

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        loadClusterData(true),
        loadDiagnosticsSummary(),
        loadMonitoringOverview(),
        loadRuntimeStorage(),
        loadStxJavaProxyStatus(),
        loadUpgradeTasks(upgradeTasksPage, upgradeTasksPageSize),
      ]);
    } finally {
      setRefreshing(false);
    }
  }, [
    loadClusterData,
    loadDiagnosticsSummary,
    loadMonitoringOverview,
    loadRuntimeStorage,
    loadStxJavaProxyStatus,
    loadUpgradeTasks,
    upgradeTasksPage,
    upgradeTasksPageSize,
  ]);

  // 配置保存/同步后反向刷新存储可视化；稳定引用避免配置 Tab 死循环
  // After config save/sync, refresh storage views; stable ref avoids configs-tab fetch loops
  const handleConfigChanged = useCallback(() => {
    void loadRuntimeStorage();
  }, [loadRuntimeStorage]);

  const handleCleanupIMAP = useCallback(async () => {
    setImapCleanupRunning(true);
    try {
      const result = await services.cluster.cleanupIMAPStorageSafe(clusterId);
      if (!result.success || !result.data) {
        toast.error(result.error || t('cluster.runtimeStorage.cleanupFailed'));
        return;
      }
      toast.success(
        result.data.success
          ? t('cluster.runtimeStorage.cleanupSuccess')
          : t('cluster.runtimeStorage.cleanupWarning'),
      );
      void loadRuntimeStorage();
    } finally {
      setImapCleanupRunning(false);
      setImapCleanupOpen(false);
    }
  }, [clusterId, loadRuntimeStorage, t]);

  const handleValidateRuntimeStorage = useCallback(
    async (kind: 'checkpoint' | 'imap') => {
      setRuntimeStorageValidationLoading(kind);
      try {
        const result = await services.cluster.validateRuntimeStorageSafe(
          clusterId,
          kind,
        );
        if (!result.success || !result.data) {
          toast.error(
            result.error || t('cluster.runtimeStorage.validateFailed'),
          );
          return;
        }
        setRuntimeStorageValidation((prev) => ({...prev, [kind]: result.data}));
        toast.success(
          result.data.success
            ? t('cluster.runtimeStorage.validateSuccess')
            : t('cluster.runtimeStorage.validateWarning'),
        );
      } finally {
        setRuntimeStorageValidationLoading(null);
      }
    },
    [clusterId, t],
  );

  /**
   * Handle delete cluster
   * 处理删除集群
   */
  const handleDelete = async () => {
    const result = await services.cluster.deleteClusterSafe(clusterId, {
      forceDelete,
    });
    if (result.success) {
      toast.success(t('cluster.deleteSuccess'));
      router.push('/clusters');
    } else {
      toast.error(result.error || t('cluster.deleteError'));
    }
    setIsDeleteDialogOpen(false);
    setForceDelete(false);
  };

  /**
   * 节点移除（缩容成功或强制移除记录）后刷新集群数据。
   * Refresh cluster data after a node is removed (scale-in or forced record removal).
   */
  const handleNodeRemoved = () => {
    loadClusterData();
    void loadRunningScaleOperation();
  };

  /**
   * Handle cluster updated
   * 处理集群更新完成
   */
  const handleClusterUpdated = () => {
    setIsEditDialogOpen(false);
    loadClusterData();
    toast.success(t('cluster.updateSuccess'));
  };

  /**
   * Handle node added
   * 处理节点添加完成
   */
  const handleNodeAdded = () => {
    // 扩容向导在执行完成后仍保持打开以展示结果，这里只刷新数据。
    // The scale-out wizard stays open after completion to show the result; only refresh data here.
    loadClusterData();
    void loadRunningScaleOperation();
    toast.success(t('cluster.addNodeSuccess'));
  };

  /**
   * Handle node edited
   * 处理节点编辑完成
   */
  const handleNodeEdited = () => {
    setIsEditNodeDialogOpen(false);
    setNodeToEdit(null);
    loadClusterData();
    toast.success(t('cluster.editNodeSuccess'));
  };

  /**
   * 切换作业日志输出模式（单 Job 独立日志模式 vs 共享混合日志模式）
   * Switch cluster job log output mode (per_job vs mixed)
   */
  const handleSwitchJobLogMode = async (targetMode: 'per_job' | 'mixed') => {
    if (logModeSwitching) {return;}
    setLogModeSwitching(true);
    try {
      const res = await services.cluster.switchJobLogModeSafe(clusterId, targetMode);
      if (!res.success) {
        toast.error(res.error || t('config.switchLogModeError'));
        return;
      }
      toast.success(
        targetMode === 'per_job'
          ? t('config.switchToPerJobSuccess')
          : t('config.switchToMixedSuccess'),
      );
      await loadClusterData(true);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('config.switchLogModeError'),
      );
    } finally {
      setLogModeSwitching(false);
    }
  };

  /**
   * Open edit node dialog
   * 打开编辑节点对话框
   */
  const openEditNodeDialog = (node: NodeInfo) => {
    setNodeToEdit(node);
    setIsEditNodeDialogOpen(true);
  };

  /**
   * Toggle node selection
   * 切换节点选择
   */
  const toggleNodeSelection = (nodeId: number) => {
    setSelectedNodeIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(nodeId)) {
        newSet.delete(nodeId);
      } else {
        newSet.add(nodeId);
      }
      return newSet;
    });
  };

  /**
   * Toggle all nodes selection
   * 切换全选
   */
  const toggleAllNodes = () => {
    if (
      filteredNodes.length > 0 &&
      filteredNodes.every((n) => selectedNodeIds.has(n.id))
    ) {
      const next = new Set(selectedNodeIds);
      filteredNodes.forEach((n) => next.delete(n.id));
      setSelectedNodeIds(next);
    } else {
      const next = new Set(selectedNodeIds);
      filteredNodes.forEach((n) => next.add(n.id));
      setSelectedNodeIds(next);
    }
  };

  /**
   * Handle single node start
   * 处理单个节点启动
   */
  const handleNodeStart = async (node: NodeInfo) => {
    setNodeOperating(node.id);
    try {
      const result = await services.cluster.startNodeSafe(clusterId, node.id);
      if (result.success) {
        // Check if auto-restart is managing the startup (check both message and node_results)
        // 检查是否由自动重启托管启动（检查 message 和 node_results）
        const isAutoRestart =
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) =>
              nr.message?.includes('auto-restart') ||
              nr.message?.includes('auto-start'),
          );
        toast.success(
          isAutoRestart
            ? t('cluster.nodeStartSuccessAutoRestart')
            : t('cluster.nodeStartSuccess'),
        );
        loadClusterData();
      } else {
        toast.error(result.error || t('cluster.nodeStartError'));
      }
    } finally {
      setNodeOperating(null);
    }
  };

  /**
   * Handle single node stop
   * 处理单个节点停止
   */
  const handleNodeStop = async (node: NodeInfo) => {
    setNodeOperating(node.id);
    try {
      const result = await services.cluster.stopNodeSafe(clusterId, node.id);
      if (result.success) {
        toast.success(t('cluster.nodeStopSuccess'));
        loadClusterData();
      } else {
        toast.error(result.error || t('cluster.nodeStopError'));
      }
    } finally {
      setNodeOperating(null);
    }
  };

  /**
   * Handle single node restart
   * 处理单个节点重启
   */
  const handleNodeRestart = async (node: NodeInfo) => {
    setNodeOperating(node.id);
    try {
      const result = await services.cluster.restartNodeSafe(clusterId, node.id);
      if (result.success) {
        // Check if auto-restart is managing the startup (check both message and node_results)
        // 检查是否由自动重启托管启动（检查 message 和 node_results）
        const isAutoRestart =
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) =>
              nr.message?.includes('auto-restart') ||
              nr.message?.includes('auto-start'),
          );
        toast.success(
          isAutoRestart
            ? t('cluster.nodeRestartSuccessAutoRestart')
            : t('cluster.nodeRestartSuccess'),
        );
        loadClusterData();
      } else {
        toast.error(result.error || t('cluster.nodeRestartError'));
      }
    } finally {
      setNodeOperating(null);
    }
  };

  /**
   * Handle view node logs
   * 处理查看节点日志
   */
  const handleViewLogs = (node: NodeInfo) => {
    setLogNodeInfo(node);
    setIsLogDialogOpen(true);
  };

  /**
   * Handle batch start selected nodes
   * 处理批量启动选中节点
   */
  const handleBatchStart = async () => {
    if (selectedNodeIds.size === 0) {
      toast.warning(t('cluster.selectNodesFirst'));
      return;
    }
    setIsOperating(true);
    try {
      let hasAutoRestart = false;
      for (const nodeId of selectedNodeIds) {
        const result = await services.cluster.startNodeSafe(clusterId, nodeId);
        // Check both message and node_results / 检查 message 和 node_results
        if (
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) =>
              nr.message?.includes('auto-restart') ||
              nr.message?.includes('auto-start'),
          )
        ) {
          hasAutoRestart = true;
        }
      }
      toast.success(
        hasAutoRestart
          ? t('cluster.batchStartSuccessAutoRestart')
          : t('cluster.batchStartSuccess'),
      );
      loadClusterData();
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle batch stop selected nodes
   * 处理批量停止选中节点
   */
  const handleBatchStop = async () => {
    if (selectedNodeIds.size === 0) {
      toast.warning(t('cluster.selectNodesFirst'));
      return;
    }
    setIsOperating(true);
    try {
      for (const nodeId of selectedNodeIds) {
        await services.cluster.stopNodeSafe(clusterId, nodeId);
      }
      toast.success(t('cluster.batchStopSuccess'));
      loadClusterData();
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle batch restart selected nodes
   * 处理批量重启选中节点
   */
  const handleBatchRestart = async () => {
    if (selectedNodeIds.size === 0) {
      toast.warning(t('cluster.selectNodesFirst'));
      return;
    }
    setIsOperating(true);
    try {
      let hasAutoRestart = false;
      for (const nodeId of selectedNodeIds) {
        const result = await services.cluster.restartNodeSafe(
          clusterId,
          nodeId,
        );
        // Check both message and node_results / 检查 message 和 node_results
        if (
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) =>
              nr.message?.includes('auto-restart') ||
              nr.message?.includes('auto-start'),
          )
        ) {
          hasAutoRestart = true;
        }
      }
      toast.success(
        hasAutoRestart
          ? t('cluster.batchRestartSuccessAutoRestart')
          : t('cluster.batchRestartSuccess'),
      );
      loadClusterData();
    } finally {
      setIsOperating(false);
    }
  };

  if (loading && !cluster) {
    return <ClusterDetailSkeleton />;
  }

  if (!cluster) {
    return (
      <div className='text-center py-12'>
        <p className='text-muted-foreground'>{t('cluster.notFound')}</p>
        <Button
          variant='outline'
          className='mt-4'
          onClick={() => router.push('/clusters')}
        >
          <ArrowLeft className='h-4 w-4 mr-2' />
          {t('cluster.backToList')}
        </Button>
      </div>
    );
  }

  const canDelete =
    cluster.status !== ClusterStatus.RUNNING &&
    cluster.status !== ClusterStatus.DEPLOYING;
  const runtimeConfig = extractClusterRuntimeConfig(cluster.config);
  const webUINode = pickWebUINode(
    nodes,
    cluster.version,
    runtimeConfig.enableHTTP,
  );
  const webUIProxyURL = webUINode ? `/api/v1/clusters/${clusterId}/webui/` : '';
  const upgradeTaskTotalPages = Math.max(
    1,
    Math.ceil(upgradeTasksTotal / upgradeTasksPageSize),
  );

  const containerVariants = {
    hidden: {opacity: 0},
    visible: {
      opacity: 1,
      transition: {duration: 0.5, staggerChildren: 0.1},
    },
  };

  const itemVariants = {
    hidden: {opacity: 0, y: 20},
    visible: {opacity: 1, y: 0, transition: {duration: 0.6}},
  };

  const renderRuntimeStorageSpec = (
    spec: RuntimeStorageSpec | undefined,
    label: string,
  ) => {
    if (!spec) {
      return (
        <div className='rounded-lg border border-dashed p-4 text-sm text-muted-foreground'>
          {t('cluster.runtimeStorage.unavailable')}
        </div>
      );
    }
    const kind = spec.kind as 'checkpoint' | 'imap';
    const listing = runtimeStorageListing[kind];
    return (
      <Card>
        <CardHeader className='pb-3'>
          <CardTitle className='text-base'>{label}</CardTitle>
          <CardDescription>
            {spec.enabled
              ? t('cluster.runtimeStorage.mode', {
                  type: spec.storage_type || '-',
                })
              : t('cluster.runtimeStorage.disabled')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-3 text-sm'>
          <div className='flex justify-end'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void handleValidateRuntimeStorage(kind)}
              disabled={runtimeStorageValidationLoading === kind}
            >
              {runtimeStorageValidationLoading === kind ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <Activity className='mr-2 h-4 w-4' />
              )}
              {t('cluster.runtimeStorage.validate')}
            </Button>
          </div>
          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.path')}
              </div>
              <div className='font-medium break-all'>
                {spec.namespace || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.endpoint')}
              </div>
              <div className='font-medium break-all'>
                {spec.endpoint || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.bucket')}
              </div>
              <div className='font-medium break-all'>{spec.bucket || '-'}</div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.size')}
              </div>
              <div className='font-medium'>
                {spec.size_available
                  ? formatBytes(spec.total_size_bytes)
                  : t('cluster.runtimeStorage.remoteSizeUnavailable')}
              </div>
            </div>
          </div>
          {spec.warning && (
            <div className='rounded-md border border-amber-200 bg-amber-50 p-3 text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100'>
              {spec.warning}
            </div>
          )}
          {runtimeStorageValidation[kind] && (
            <div className='space-y-2 rounded-md border p-3'>
              <div className='flex items-center justify-between gap-2'>
                <div className='font-medium'>
                  {t('cluster.runtimeStorage.validateResult')}
                </div>
                <Badge
                  variant={
                    runtimeStorageValidation[kind]?.success
                      ? 'default'
                      : 'destructive'
                  }
                >
                  {runtimeStorageValidation[kind]?.success
                    ? t('cluster.runtimeStorage.passed')
                    : t('cluster.runtimeStorage.failed')}
                </Badge>
              </div>
              {runtimeStorageValidation[kind]?.warning && (
                <div className='text-xs text-muted-foreground'>
                  {runtimeStorageValidation[kind]?.warning}
                </div>
              )}
              <div className='space-y-2'>
                {runtimeStorageValidation[kind]?.hosts?.map((host) => (
                  <div
                    key={`${spec.kind}-validate-${host.host_id}`}
                    className='flex items-start justify-between gap-3 rounded-md border px-3 py-2'
                  >
                    <div className='min-w-0'>
                      <div className='font-medium'>
                        {host.host_name || host.host_id}
                      </div>
                      <div className='text-xs text-muted-foreground break-all'>
                        {host.message}
                      </div>
                    </div>
                    <Badge variant={host.success ? 'default' : 'destructive'}>
                      {host.success
                        ? t('cluster.runtimeStorage.passed')
                        : t('cluster.runtimeStorage.failed')}
                    </Badge>
                  </div>
                ))}
              </div>
            </div>
          )}
          {spec.nodes && spec.nodes.length > 0 && (
            <div className='space-y-2'>
              {spec.nodes.map((node) => (
                <div
                  key={`${spec.kind}-${node.host_id}-${node.node_id}`}
                  className='flex items-center justify-between rounded-md border px-3 py-2'
                >
                  <div className='min-w-0'>
                    <div className='font-medium'>{node.host_name}</div>
                    <div className='text-xs text-muted-foreground break-all'>
                      {node.path || '-'}
                    </div>
                  </div>
                  <div className='text-right'>
                    <div className='font-medium'>
                      {formatBytes(node.size_bytes)}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {node.message ||
                        (node.exists
                          ? t('cluster.runtimeStorage.pathExists')
                          : t('cluster.runtimeStorage.pathMissing'))}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
          {/* 类似电脑资源管理器的文件与目录浏览器 */}
          {/* Desktop-like file explorer for runtime storage inspection & navigation */}
          <RuntimeStorageFileExplorer
            clusterId={clusterId}
            kind={kind}
            spec={spec}
            listing={listing}
            loading={runtimeStorageListingLoading === kind}
            currentPath={runtimeStorageBrowsePath[kind]}
            onNavigate={(path) => void loadRuntimeStorageList(kind, path)}
            onRefresh={() =>
              void loadRuntimeStorageList(
                kind,
                runtimeStorageBrowsePath[kind] || spec.namespace,
                true,
              )
            }
            onPreviewFile={(path) => void handlePreviewRuntimeStorage(kind, path)}
            previewLoadingKey={runtimeStoragePreviewLoading}
            onInspectCheckpoint={
              kind === 'checkpoint'
                ? (path) => void handleInspectCheckpoint(path)
                : undefined
            }
            inspectCheckpointLoadingPath={checkpointInspectLoading}
            onInspectIMAPWAL={
              kind === 'imap'
                ? (path) => void handleInspectIMAPWAL(path)
                : undefined
            }
            inspectIMAPWALLoadingPath={imapInspectLoading}
          />
          {spec.kind === 'imap' && spec.cleanup_supported && (
            <div className='flex justify-end'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => setImapCleanupOpen(true)}
                disabled={imapCleanupRunning}
              >
                {imapCleanupRunning ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <Trash2 className='mr-2 h-4 w-4' />
                )}
                {t('cluster.runtimeStorage.cleanupImap')}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    );
  };

  // 节点过滤计算 / Filtered nodes calculation
  const filteredNodes = nodes.filter((node) => {
    const matchesSearch =
      !nodeSearchTerm ||
      (node.host_name &&
        node.host_name.toLowerCase().includes(nodeSearchTerm.toLowerCase())) ||
      (node.host_ip && node.host_ip.includes(nodeSearchTerm)) ||
      (node.role &&
        node.role.toLowerCase().includes(nodeSearchTerm.toLowerCase())) ||
      String(node.id).includes(nodeSearchTerm);

    const matchesStatus =
      nodeStatusFilter === 'all' ||
      (nodeStatusFilter === 'running' && node.status === NodeStatus.RUNNING) ||
      (nodeStatusFilter === 'offline' && node.status === NodeStatus.OFFLINE) ||
      (nodeStatusFilter === 'stopped' && node.status === NodeStatus.STOPPED);

    return matchesSearch && matchesStatus;
  });

  return (
    <motion.div
      className='space-y-6'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      {/* 顶部平滑加载进度条 / Top smooth loading bar */}
      <TableLoadingBar loading={refreshing} />

      {/* 标准化页面头部 / Standardized Workspace Header */}
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('cluster.title'),
              href: '/clusters',
            },
            {
              label: cluster.name,
            },
          ]}
          icon={<Layers />}
          title={cluster.name}
          badge={
            <div className='flex items-center gap-1.5 flex-wrap'>
              <Badge variant={getStatusBadgeVariant(cluster.status)}>
                {t(`cluster.statuses.${cluster.status}`)}
              </Badge>
              {clusterStatus?.health_status && (
                <Badge
                  variant={
                    clusterStatus.health_status === 'healthy'
                      ? 'default'
                      : 'destructive'
                  }
                  className={`flex items-center gap-1 text-[11px] font-normal ${
                    clusterStatus.health_status === 'healthy'
                      ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
                      : 'bg-destructive/15 text-destructive border-destructive/30'
                  }`}
                >
                  <span
                    className={`size-1.5 rounded-full ${
                      clusterStatus.health_status === 'healthy'
                        ? 'bg-emerald-500'
                        : 'bg-destructive animate-pulse'
                    }`}
                  />
                  {t(`cluster.healthStatuses.${clusterStatus.health_status}`) ||
                    clusterStatus.health_status}
                </Badge>
              )}
              {cluster.version && (
                <Badge
                  variant='outline'
                  className='font-mono text-xs text-muted-foreground'
                >
                  v{cluster.version}
                </Badge>
              )}
              {cluster.cluster_code && (
                <Badge
                  variant='outline'
                  className='font-mono text-xs bg-muted/40 text-muted-foreground'
                  title={t('cluster.clusterCodeHint')}
                >
                  {cluster.cluster_code}
                </Badge>
              )}
            </div>
          }
          subtitle={
            cluster.description ? (
              <span className='truncate max-w-lg'>{cluster.description}</span>
            ) : null
          }
          actions={
            <div className='flex items-center gap-2 flex-wrap'>
              {/* 整集群生命周期操作 / Cluster lifecycle actions */}
              <ClusterActions
                cluster={cluster}
                onOperationComplete={() => void loadClusterData(true)}
              />
              <Button
                variant='outline'
                size='sm'
                onClick={handleRefresh}
                disabled={refreshing}
                className='h-8'
                title={t('common.refresh')}
              >
                <RefreshCw
                  className={`h-3.5 w-3.5 mr-1.5 ${refreshing ? 'animate-spin' : ''}`}
                />
                {t('common.refresh')}
              </Button>

              {/* 更多辅助与低频/高危操作收纳进下拉菜单 */}
              {/* Secondary, low-frequency, and dangerous actions grouped into more dropdown */}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant='outline'
                    size='sm'
                    className='h-8 px-2.5'
                    aria-label={t('common.moreActions')}
                  >
                    <MoreHorizontal className='h-4 w-4 mr-1' />
                    <span className='text-xs'>{t('common.more')}</span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end' className='w-44'>
                  <DropdownMenuItem
                    onClick={() =>
                      router.push(
                        `/diagnostics?tab=errors&cluster_id=${clusterId}&source=cluster-detail`,
                      )
                    }
                  >
                    <Bug className='h-3.5 w-3.5 mr-2 text-muted-foreground' />
                    {t('cluster.openDiagnostics')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() =>
                      router.push(`/clusters/${clusterId}/upgrade/prepare`)
                    }
                  >
                    <Activity className='h-3.5 w-3.5 mr-2 text-muted-foreground' />
                    {t('stUpgrade.entry')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setIsEditDialogOpen(true)}>
                    <Pencil className='h-3.5 w-3.5 mr-2 text-muted-foreground' />
                    {t('common.edit')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    disabled={!canDelete}
                    onClick={() => setIsDeleteDialogOpen(true)}
                    className='text-destructive focus:text-destructive'
                  >
                    <Trash2 className='h-3.5 w-3.5 mr-2' />
                    {t('common.delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          }
        />
      </motion.div>

      {/* 状态胶囊汇总与快捷切换栏 / Status Pills Summary Bar */}
      <motion.div variants={itemVariants}>
        <StatPillsBar
          items={[
            {
              key: 'all',
              label: t('cluster.statTotalNodes'),
              count: clusterStatus?.total_nodes ?? nodes.length,
              icon: <Server className='size-3.5' />,
            },
            {
              key: 'online',
              label: t('cluster.statOnlineNodes'),
              count:
                clusterStatus?.online_nodes ??
                nodes.filter((n) => n.status === NodeStatus.RUNNING).length,
              variant: 'success',
            },
            {
              key: 'offline',
              label: t('cluster.statOfflineNodes'),
              count:
                clusterStatus?.offline_nodes ??
                nodes.filter((n) => n.status === NodeStatus.OFFLINE).length,
              variant:
                (clusterStatus?.offline_nodes ?? 0) > 0 ||
                nodes.some((n) => n.status === NodeStatus.OFFLINE)
                  ? 'danger'
                  : 'default',
              pulse:
                (clusterStatus?.offline_nodes ?? 0) > 0 ||
                nodes.some((n) => n.status === NodeStatus.OFFLINE),
            },
            {
              key: 'master',
              label: t('cluster.statMasterNodes'),
              count: nodes.filter(
                (n) =>
                  n.role === NodeRole.MASTER ||
                  n.role === NodeRole.MASTER_WORKER,
              ).length,
              variant: 'info',
            },
            {
              key: 'worker',
              label: t('cluster.statWorkerNodes'),
              count: nodes.filter(
                (n) =>
                  n.role === NodeRole.WORKER ||
                  n.role === NodeRole.MASTER_WORKER,
              ).length,
              variant: 'info',
            },
            ...(diagnosticsGroupTotal > 0
              ? [
                  {
                    key: 'diagnostics' as const,
                    label: t('cluster.statDiagnostics'),
                    count: diagnosticsGroupTotal,
                    variant: 'warning' as const,
                  },
                ]
              : []),
            ...((monitoringOverview?.stats.active_alerts_1h ?? 0) > 0
              ? [
                  {
                    key: 'alerts' as const,
                    label: t('cluster.statAlerts'),
                    count: monitoringOverview!.stats.active_alerts_1h,
                    variant: 'danger' as const,
                    pulse: true,
                  },
                ]
              : []),
          ]}
          activeKey={activeTab === 'nodes' ? nodeStatusFilter : activeTab}
          onChange={(key) => {
            if (key === 'all') {
              setActiveTab('nodes');
              setNodeStatusFilter('all');
              setNodeSearchTerm('');
            } else if (key === 'online') {
              setActiveTab('nodes');
              setNodeStatusFilter('running');
              setNodeSearchTerm('');
            } else if (key === 'offline') {
              setActiveTab('nodes');
              setNodeStatusFilter('offline');
              setNodeSearchTerm('');
            } else if (key === 'master') {
              setActiveTab('nodes');
              setNodeStatusFilter('all');
              setNodeSearchTerm('master');
            } else if (key === 'worker') {
              setActiveTab('nodes');
              setNodeStatusFilter('all');
              setNodeSearchTerm('worker');
            } else if (key === 'diagnostics') {
              setActiveTab('diagnostics');
            } else if (key === 'alerts') {
              router.push(`/monitoring?tab=alerts&cluster_id=${clusterId}`);
            }
          }}
        />
      </motion.div>

      {monitoringOverview && monitoringOverview.stats.active_alerts_1h > 0 && (
        <motion.div variants={itemVariants}>
          <Card className='border-destructive/40 bg-destructive/5'>
            <CardContent className='flex flex-col gap-4 py-4 md:flex-row md:items-center md:justify-between'>
              <div className='space-y-1'>
                <div className='flex items-center gap-2 text-sm font-medium text-destructive'>
                  <AlertTriangle className='h-4 w-4' />
                  {t('cluster.activeAlertsBanner.title', {
                    count: monitoringOverview.stats.active_alerts_1h,
                  })}
                </div>
                <p className='text-sm text-muted-foreground'>
                  {t('cluster.activeAlertsBanner.description', {
                    restartFailed:
                      monitoringOverview.stats.restart_failed_events_24h,
                  })}
                </p>
              </div>
              <Button
                variant='outline'
                onClick={() =>
                  router.push(`/monitoring?tab=alerts&cluster_id=${clusterId}`)
                }
              >
                <AlertTriangle className='mr-2 h-4 w-4' />
                {t('cluster.activeAlertsBanner.action')}
              </Button>
            </CardContent>
          </Card>
        </motion.div>
      )}

      <motion.div variants={itemVariants}>
        <Tabs
          value={activeTab}
          onValueChange={(value) => setActiveTab(value as ClusterDetailTab)}
          className='space-y-4'
        >
          <TabsList className='h-auto w-full flex-wrap justify-start gap-1 rounded-xl p-1'>
            <TabsTrigger
              value='overview'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-overview'
            >
              {t('cluster.detailTabs.overview')}
            </TabsTrigger>
            <TabsTrigger
              value='nodes'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-nodes'
            >
              {t('cluster.detailTabs.nodes')}
            </TabsTrigger>
            <TabsTrigger
              value='storage'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-storage'
            >
              {t('cluster.detailTabs.storage')}
            </TabsTrigger>
            <TabsTrigger
              value='proxy'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-proxy'
            >
              {t('cluster.detailTabs.proxy')}
            </TabsTrigger>
            <TabsTrigger
              value='monitoring'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-monitoring'
            >
              {t('cluster.detailTabs.monitoring')}
            </TabsTrigger>
            {webUINode && (
              <TabsTrigger
                value='webui'
                className='flex-none px-3'
                data-testid='cluster-detail-tab-webui'
              >
                {t('cluster.detailTabs.webui')}
              </TabsTrigger>
            )}
            <TabsTrigger
              value='plugins'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-plugins'
            >
              {t('cluster.detailTabs.plugins')}
            </TabsTrigger>
            <TabsTrigger
              value='configs'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-configs'
            >
              {t('cluster.detailTabs.configs')}
            </TabsTrigger>
            <TabsTrigger
              value='diagnostics'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-diagnostics'
            >
              {t('cluster.detailTabs.diagnostics')}
            </TabsTrigger>
            <TabsTrigger
              value='upgrades'
              className='flex-none px-3'
              data-testid='cluster-detail-tab-upgrades'
            >
              {t('cluster.detailTabs.upgrades')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value='overview' className='space-y-6'>
            {/* Cluster Profile & Service Ports / 集群基本规格与网络端口 */}
            <Card className='border rounded-xl bg-card/40 shadow-xs'>
              <CardHeader className='pb-3'>
                <CardTitle className='text-base font-semibold'>
                  {t('cluster.clusterInfo')}
                </CardTitle>
                <CardDescription className='text-xs'>
                  {t('cluster.descriptionLabel')}: {cluster.description || '-'}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
                  {/* 安装目录 / Install directory */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <div className='flex items-center justify-between'>
                      <span className='text-xs text-muted-foreground'>
                        {t('cluster.installDir')}
                      </span>
                      {cluster.install_dir && (
                        <button
                          type='button'
                          onClick={async () => {
                            await navigator.clipboard.writeText(
                              cluster.install_dir,
                            );
                            setCopiedInstallDir(true);
                            toast.success(t('cluster.logCopied'));
                            setTimeout(() => setCopiedInstallDir(false), 2000);
                          }}
                          className='text-muted-foreground hover:text-foreground transition-colors p-0.5'
                          title={t('common.copy')}
                        >
                          {copiedInstallDir ? (
                            <Check className='size-3 text-emerald-500' />
                          ) : (
                            <Copy className='size-3' />
                          )}
                        </button>
                      )}
                    </div>
                    <p className='font-mono text-xs font-medium break-all'>
                      {cluster.install_dir || '-'}
                    </p>
                  </div>

                  {/* 版本 / Version */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.version')}
                    </span>
                    <p className='font-medium text-sm flex items-center gap-1.5'>
                      <span className='font-mono font-semibold'>
                        {cluster.version || '-'}
                      </span>
                      <Badge variant='outline' className='text-[10px] uppercase'>
                        {t(`cluster.modes.${cluster.deployment_mode}`) ||
                          cluster.deployment_mode}
                      </Badge>
                    </p>
                  </div>

                  {/* Hazelcast 端口 / Hazelcast port */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.hazelcastPort')}
                    </span>
                    <p className='font-mono text-sm font-medium'>
                      {nodes.find((node) => node.hazelcast_port > 0)
                        ?.hazelcast_port || '-'}
                    </p>
                  </div>

                  {/* HTTP API 端口 / HTTP API port */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.httpPort')}
                    </span>
                    <p className='font-mono text-sm font-medium'>
                      {runtimeConfig.enableHTTP === false
                        ? t('cluster.httpDisabled')
                        : webUINode?.api_port || '-'}
                    </p>
                  </div>

                  {/* 日志输出模式 / Log output mode */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1.5'>
                    <div className='flex items-center justify-between gap-1.5'>
                      <span className='text-xs text-muted-foreground'>
                        {t('cluster.logOutputMode')}
                      </span>
                      {logModeSwitching && (
                        <Loader2 className='size-3 animate-spin text-muted-foreground' />
                      )}
                    </div>
                    <Select
                      value={runtimeConfig.jobLogMode || 'mixed'}
                      disabled={logModeSwitching}
                      onValueChange={(val: 'per_job' | 'mixed') => {
                        void handleSwitchJobLogMode(val);
                      }}
                    >
                      <SelectTrigger className='h-7 text-xs font-medium bg-background/80'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='per_job' className='text-xs'>
                          {t('config.jobLogModePerJobBadge')}
                        </SelectItem>
                        <SelectItem value='mixed' className='text-xs'>
                          {t('config.jobLogModeMixedBadge')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {/* Web UI 状态 / Web UI status */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.webUiStatus')}
                    </span>
                    <p className='font-medium text-sm'>
                      {webUINode
                        ? t('common.enabled')
                        : isSeatunnelVersionAtLeast(cluster.version, '2.3.9')
                          ? t('cluster.httpDisabled')
                          : t('cluster.versionUnsupported')}
                    </p>
                  </div>

                  {/* 创建时间 / Created at */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.createdAt')}
                    </span>
                    <p className='font-mono text-xs text-muted-foreground'>
                      {formatDateTimeTz(cluster.created_at)}
                    </p>
                  </div>

                  {/* 更新时间 / Updated at */}
                  <div className='p-3 rounded-lg border bg-muted/10 space-y-1'>
                    <span className='text-xs text-muted-foreground'>
                      {t('cluster.updatedAt')}
                    </span>
                    <p className='font-mono text-xs text-muted-foreground'>
                      {formatDateTimeTz(cluster.updated_at)}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Node Topology Snapshot / 节点拓扑速览 */}
            <Card className='border rounded-xl bg-card/40 shadow-xs'>
              <CardHeader className='flex flex-row items-center justify-between pb-3'>
                <div className='space-y-0.5'>
                  <CardTitle className='flex items-center gap-2 text-base font-semibold'>
                    <Server className='size-4 text-primary' />
                    <span>{t('cluster.topologyPreview')}</span>
                    <Badge variant='secondary' className='text-xs font-mono'>
                      {nodes.length}
                    </Badge>
                  </CardTitle>
                  <CardDescription className='text-xs'>
                    {t('cluster.roles.master')}:{' '}
                    {
                      nodes.filter(
                        (n) =>
                          n.role === NodeRole.MASTER ||
                          n.role === NodeRole.MASTER_WORKER,
                      ).length
                    }{' '}
                    · {t('cluster.roles.worker')}:{' '}
                    {
                      nodes.filter(
                        (n) =>
                          n.role === NodeRole.WORKER ||
                          n.role === NodeRole.MASTER_WORKER,
                      ).length
                    }
                  </CardDescription>
                </div>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => setActiveTab('nodes')}
                  className='text-xs h-8 text-primary hover:text-primary/80'
                >
                  {t('cluster.viewAllNodes')} &rarr;
                </Button>
              </CardHeader>
              <CardContent>
                {nodes.length === 0 ? (
                  <div className='text-center py-6 text-sm text-muted-foreground'>
                    {t('cluster.noNodes')}
                  </div>
                ) : (
                  <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
                    {nodes.map((node) => (
                      <div
                        key={node.id}
                        className='p-3 rounded-lg border bg-background/50 hover:bg-muted/30 transition-colors flex flex-col justify-between gap-2 shadow-2xs'
                      >
                        <div className='flex items-start justify-between gap-2'>
                          <div className='min-w-0 flex-1'>
                            <div className='font-medium text-sm truncate' title={node.host_name || node.host_ip}>
                              {node.host_name || `#${node.id}`}
                            </div>
                            <div className='font-mono text-xs text-muted-foreground truncate'>
                              {node.host_ip || '-'}
                            </div>
                          </div>
                          <Badge
                            variant={getStatusBadgeVariant(node.status)}
                            className='text-[10px] shrink-0'
                          >
                            {t(`cluster.nodeStatuses.${node.status}`)}
                          </Badge>
                        </div>
                        <div className='pt-2 border-t border-border/40 flex items-center justify-between text-xs text-muted-foreground'>
                          <Badge variant='outline' className='text-[10px]'>
                            {t(
                              `cluster.roles.${getRoleTranslationKey(node.role)}`,
                            )}
                          </Badge>
                          <span className='font-mono text-[11px]'>
                            PID: {node.process_pid || '-'}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Runtime Storage & Java Proxy Snapshot / 运行时存储与代理速览 */}
            <Card className='border rounded-xl bg-card/40 shadow-xs'>
              <CardHeader className='flex flex-row items-center justify-between pb-3'>
                <div>
                  <CardTitle className='flex items-center gap-2 text-base font-semibold'>
                    <Database className='size-4 text-primary' />
                    <span>{t('cluster.storageSummary')}</span>
                  </CardTitle>
                </div>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => setActiveTab('storage')}
                  className='text-xs h-8 text-primary hover:text-primary/80'
                >
                  {t('cluster.manageStorage')} &rarr;
                </Button>
              </CardHeader>
              <CardContent>
                <div className='grid gap-3 md:grid-cols-3'>
                  {/* Checkpoint Storage 摘要 / Checkpoint storage summary */}
                  <div className='p-3.5 rounded-lg border bg-muted/10 space-y-1.5'>
                    <div className='flex items-center justify-between'>
                      <span className='text-xs font-semibold'>
                        {t('installer.checkpointConfig')}
                      </span>
                      <Badge variant='outline' className='text-[10px] uppercase font-mono'>
                        {runtimeStorage?.checkpoint?.storage_type || 'Local'}
                      </Badge>
                    </div>
                    <p className='text-xs text-muted-foreground font-mono break-all truncate' title={runtimeStorage?.checkpoint?.namespace || '-'}>
                      {runtimeStorage?.checkpoint?.namespace || '-'}
                    </p>
                  </div>

                  {/* IMAP Storage 摘要 / IMAP storage summary */}
                  <div className='p-3.5 rounded-lg border bg-muted/10 space-y-1.5'>
                    <div className='flex items-center justify-between'>
                      <span className='text-xs font-semibold'>
                        {t('installer.runtimeStorage.imapTitle')}
                      </span>
                      <Badge variant='outline' className='text-[10px] uppercase font-mono'>
                        {runtimeStorage?.imap?.storage_type || 'Local'}
                      </Badge>
                    </div>
                    <p className='text-xs text-muted-foreground font-mono break-all truncate' title={runtimeStorage?.imap?.namespace || '-'}>
                      {runtimeStorage?.imap?.namespace || '-'}
                    </p>
                  </div>

                  {/* STX Java Proxy 摘要 / STX Java Proxy summary */}
                  <button
                    type='button'
                    onClick={() => setActiveTab('proxy')}
                    className='p-3.5 rounded-lg border bg-muted/10 space-y-1.5 text-left hover:bg-muted/20 transition-colors cursor-pointer'
                    data-testid='cluster-overview-proxy-summary'
                  >
                    <div className='flex items-center justify-between'>
                      <span className='text-xs font-semibold'>
                        {t('cluster.stxJavaProxy.title')}
                      </span>
                      <Badge
                        variant={
                          stxJavaProxy?.healthy
                            ? 'default'
                            : stxJavaProxy?.running
                              ? 'outline'
                              : 'secondary'
                        }
                        className='text-[10px]'
                      >
                        {stxJavaProxy?.healthy
                          ? t('cluster.stxJavaProxy.healthy')
                          : stxJavaProxy?.running
                            ? t('cluster.stxJavaProxy.unhealthy')
                            : t('cluster.stxJavaProxy.stopped')}
                      </Badge>
                    </div>
                    <p className='text-xs text-muted-foreground font-mono break-all truncate' title={stxJavaProxy?.endpoint || '-'}>
                      {stxJavaProxy?.endpoint || '-'}
                    </p>
                  </button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          {webUINode && (
            <TabsContent value='webui' className='space-y-6'>
              <Card>
                <CardHeader className='flex flex-row items-center justify-between space-y-0'>
                  <CardTitle className='flex min-w-0 items-center gap-2'>
                    <MonitorSmartphone className='h-5 w-5 shrink-0' />
                    {t('cluster.webUiTitle')}
                    {/* 主节点地址收成标题旁 Badge / Master address shown as a badge next to the title */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Badge
                          variant='outline'
                          className='cursor-help whitespace-nowrap font-mono text-[11px] font-normal'
                          data-testid='cluster-webui-master-address'
                        >
                          {`${webUINode.host_ip || webUINode.host_name || '-'}:${webUINode.api_port}`}
                        </Badge>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('cluster.webUiMasterAddress')}
                      </TooltipContent>
                    </Tooltip>
                  </CardTitle>
                  {/* 通用观测入口：代理或原生地址（原生地址由后端按 active master 给出）；嵌入方式说明进按钮 Tooltip */}
                  {/* Shared observation entry: proxy or native address (native picked by the backend, preferring the active master); embedding note lives in the button tooltip */}
                  <SeaTunnelObserveMenu
                    clusterId={clusterId}
                    variant='button'
                    triggerHint={t('cluster.webUiHint')}
                  />
                </CardHeader>
                <CardContent className='space-y-4'>
                  <div className='overflow-hidden rounded-xl border bg-background'>
                    <iframe
                      key={webUIProxyURL}
                      title='SeaTunnel Web UI'
                      src={webUIProxyURL}
                      className='h-[900px] w-full border-0'
                      sandbox='allow-scripts allow-same-origin allow-forms allow-popups allow-downloads'
                      referrerPolicy='strict-origin-when-cross-origin'
                    />
                  </div>
                </CardContent>
              </Card>
            </TabsContent>
          )}

          <TabsContent value='storage' className='space-y-4'>
            {/* 存储 Tab：Checkpoint / IMAP 焦点切换 / Storage tab focuses one store at a time */}
            <Card className='border rounded-xl relative overflow-hidden bg-card/40 shadow-xs flex flex-col flex-1 min-h-[480px]'>
              <TableLoadingBar
                loading={
                  runtimeStorageLoading ||
                  runtimeStorageListingLoading !== null
                }
              />
              <CardHeader className='flex flex-row items-center justify-between space-y-0 gap-3'>
                <div className='min-w-0'>
                  <CardTitle className='text-base'>
                    {t('cluster.runtimeStorage.title')}
                  </CardTitle>
                  <div className='mt-1 truncate text-xs text-muted-foreground'>
                    {t('cluster.runtimeStorage.configSource', {
                      source: runtimeStorage?.config_source || '-',
                    })}
                  </div>
                </div>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void loadRuntimeStorage(true)}
                  disabled={runtimeStorageLoading}
                >
                  {runtimeStorageLoading ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <RefreshCw className='mr-2 h-4 w-4' />
                  )}
                  {t('common.refresh')}
                </Button>
              </CardHeader>
              <CardContent className='space-y-4'>
                {runtimeStorageLoading && !runtimeStorage ? (
                  <div className='space-y-3 py-2'>
                    <Skeleton className='h-8 w-full' />
                    <Skeleton className='h-40 w-full' />
                  </div>
                ) : (
                  <div
                    className={
                      runtimeStorageLoading
                        ? 'pointer-events-none space-y-4 opacity-60 transition-opacity duration-200'
                        : 'space-y-4'
                    }
                  >
                    <StatPillsBar
                      activeKey={storageFocus}
                      onChange={(key) =>
                        setStorageFocus(key as 'checkpoint' | 'imap')
                      }
                      items={[
                        {
                          key: 'checkpoint',
                          label: t('installer.checkpointConfig'),
                          count:
                            runtimeStorage?.checkpoint?.storage_type || '-',
                        },
                        {
                          key: 'imap',
                          label: t('installer.runtimeStorage.imapTitle'),
                          count: runtimeStorage?.imap?.storage_type || '-',
                        },
                      ]}
                    />
                    <RuntimeStorageEditor
                      clusterId={clusterId}
                      kind={storageFocus}
                      spec={
                        storageFocus === 'checkpoint'
                          ? runtimeStorage?.configured_checkpoint || runtimeStorage?.checkpoint
                          : runtimeStorage?.configured_imap || runtimeStorage?.imap
                      }
                      onApplied={() => void loadRuntimeStorage()}
                    />
                    {storageFocus === 'checkpoint' &&
                      renderRuntimeStorageSpec(
                        runtimeStorage?.checkpoint,
                        t('installer.checkpointConfig'),
                      )}
                    {storageFocus === 'imap' &&
                      renderRuntimeStorageSpec(
                        runtimeStorage?.imap,
                        t('installer.runtimeStorage.imapTitle'),
                      )}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value='proxy' className='space-y-4'>
            {/* STX Java Proxy 独立管理页 / Dedicated STX Java Proxy management tab */}
            <Card
              className='border rounded-xl relative overflow-hidden bg-card/40 shadow-xs flex flex-col flex-1 min-h-[360px]'
              data-testid='stx-java-proxy-card'
            >
              <TableLoadingBar loading={stxJavaProxyLoading} />
              <CardHeader className='flex flex-row items-start justify-between gap-4 space-y-0'>
                <div className='min-w-0'>
                  <CardTitle className='flex items-center gap-1.5 text-base'>
                    <span>{t('cluster.stxJavaProxy.title')}</span>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type='button'
                          className='text-muted-foreground/70 hover:text-foreground'
                          aria-label={t('cluster.stxJavaProxy.description')}
                        >
                          <HelpCircle className='size-3.5' />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs text-xs font-normal leading-relaxed'>
                        {t('cluster.stxJavaProxy.description')}
                      </TooltipContent>
                    </Tooltip>
                  </CardTitle>
                </div>
                <div className='flex items-center gap-2 shrink-0'>
                  <Badge
                    variant={
                      stxJavaProxy?.healthy
                        ? 'default'
                        : stxJavaProxy?.running
                          ? 'outline'
                          : 'secondary'
                    }
                  >
                    {stxJavaProxy?.healthy
                      ? t('cluster.stxJavaProxy.healthy')
                      : stxJavaProxy?.running
                        ? t('cluster.stxJavaProxy.unhealthy')
                        : t('cluster.stxJavaProxy.stopped')}
                  </Badge>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void loadStxJavaProxyStatus()}
                    disabled={stxJavaProxyLoading}
                    data-testid='stx-java-proxy-refresh'
                  >
                    {stxJavaProxyLoading ? (
                      <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                    ) : (
                      <RefreshCw className='mr-2 h-4 w-4' />
                    )}
                    {t('common.refresh')}
                  </Button>
                </div>
              </CardHeader>
              <CardContent
                className={
                  stxJavaProxyLoading && stxJavaProxy
                    ? 'pointer-events-none space-y-4 text-sm opacity-60 transition-opacity duration-200'
                    : 'space-y-4 text-sm'
                }
              >
                {stxJavaProxyLoading && !stxJavaProxy ? (
                  <div className='space-y-3 py-2'>
                    <Skeleton className='h-20 w-full' />
                    <Skeleton className='h-10 w-64' />
                  </div>
                ) : (
                  <>
                    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.deploymentNode')}
                        </div>
                        <div className='font-medium truncate'>
                          {stxJavaProxy?.host_name || '-'}
                          {stxJavaProxy?.role
                            ? ` (${t(`cluster.roles.${getRoleTranslationKey(stxJavaProxy.role)}`)})`
                            : ''}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.endpoint')}
                        </div>
                        <div className='font-medium truncate'>
                          {stxJavaProxy?.endpoint || '-'}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.pid')}
                        </div>
                        <div className='font-medium'>
                          {stxJavaProxy?.pid ?? '-'}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.logPath')}
                        </div>
                        <div className='font-medium truncate'>
                          {stxJavaProxy?.log_path &&
                          stxJavaProxy.log_path.trim() !== ''
                            ? stxJavaProxy.log_path
                            : '-'}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.jvmMemoryMax')}
                        </div>
                        <div className='font-medium font-mono'>
                          {stxJavaProxy?.jvm_memory?.maxMemoryMb
                            ? `${stxJavaProxy.jvm_memory.maxMemoryMb} MB`
                            : '512 MB (默认)'}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.jvmMemoryUsage')}
                        </div>
                        <div className='font-medium font-mono'>
                          {stxJavaProxy?.jvm_memory?.usedMemoryBytes != null
                            ? `${Math.round(stxJavaProxy.jvm_memory.usedMemoryBytes / (1024 * 1024))} MB / ${Math.round((stxJavaProxy.jvm_memory.totalMemoryBytes || 0) / (1024 * 1024))} MB`
                            : '-'}
                        </div>
                      </div>
                      <div className='md:col-span-2'>
                        <div className='text-muted-foreground'>
                          {t('cluster.stxJavaProxy.jvmOpts')}
                        </div>
                        <div
                          className='font-medium font-mono truncate'
                          title={
                            stxJavaProxy?.jvm_opts ||
                            t('cluster.stxJavaProxy.jvmOptsDefault')
                          }
                        >
                          {stxJavaProxy?.jvm_opts &&
                          stxJavaProxy.jvm_opts.trim() !== ''
                            ? stxJavaProxy.jvm_opts
                            : t('cluster.stxJavaProxy.jvmOptsDefault')}
                        </div>
                      </div>
                    </div>
                    <div className='rounded-md border bg-muted/20 p-3 text-sm'>
                      {stxJavaProxy?.message ||
                        t('cluster.stxJavaProxy.noStatus')}
                    </div>
                    <div className='flex flex-wrap gap-2'>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={handleOpenStxJavaProxyConfig}
                        disabled={
                          stxJavaProxyLoading || stxJavaProxyOperating !== null
                        }
                        data-testid='stx-java-proxy-config-btn'
                      >
                        <SlidersHorizontal className='mr-2 h-4 w-4' />
                        {t('cluster.stxJavaProxy.configureMemory')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => void handlePreviewStxJavaProxyLog()}
                        disabled={
                          stxJavaProxyLoading || stxJavaProxyLogLoading
                        }
                        data-testid='stx-java-proxy-view-log'
                      >
                        {stxJavaProxyLogLoading ? (
                          <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                        ) : (
                          <FileText className='mr-2 h-4 w-4' />
                        )}
                        {t('cluster.stxJavaProxy.viewRuntimeLog')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          void handleStxJavaProxyOperation('start')
                        }
                        disabled={stxJavaProxyOperating !== null}
                        data-testid='stx-java-proxy-start'
                      >
                        {stxJavaProxyOperating === 'start' ? (
                          <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                        ) : (
                          <Play className='mr-2 h-4 w-4' />
                        )}
                        {t('cluster.stxJavaProxy.start')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          void handleStxJavaProxyOperation('restart')
                        }
                        disabled={stxJavaProxyOperating !== null}
                        data-testid='stx-java-proxy-restart'
                      >
                        {stxJavaProxyOperating === 'restart' ? (
                          <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                        ) : (
                          <RotateCcw className='mr-2 h-4 w-4' />
                        )}
                        {t('cluster.stxJavaProxy.restart')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          void handleStxJavaProxyOperation('stop')
                        }
                        disabled={stxJavaProxyOperating !== null}
                        data-testid='stx-java-proxy-stop'
                      >
                        {stxJavaProxyOperating === 'stop' ? (
                          <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                        ) : (
                          <Square className='mr-2 h-4 w-4' />
                        )}
                        {t('cluster.stxJavaProxy.stop')}
                      </Button>
                    </div>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value='monitoring' className='space-y-6'>
            {/* Monitor Config / 监控配置 */}
            <MonitorConfigPanel
              clusterId={clusterId}
              clusterName={cluster.name}
            />
          </TabsContent>

          <TabsContent value='nodes' className='space-y-6'>
            {/* Integrated Node Management Card / 一体化节点管理表格卡片 */}
            <Card className='border rounded-xl relative overflow-hidden bg-card/40 shadow-xs flex flex-col flex-1 min-h-[480px]'>
              <CardHeader className='flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-3 border-b bg-muted/10'>
                <div className='flex items-center gap-2 flex-wrap'>
                  <CardTitle className='flex items-center gap-2 text-base font-semibold'>
                    <Server className='h-4 w-4 text-primary' />
                    <span>{t('cluster.nodeList')}</span>
                  </CardTitle>
                  <Badge variant='secondary' className='text-xs font-mono'>
                    {filteredNodes.length} / {nodes.length}
                  </Badge>
                  {selectedNodeIds.size > 0 && (
                    <Badge variant='outline' className='text-xs font-medium'>
                      {t('cluster.selectedNodes', {
                        count: selectedNodeIds.size,
                      })}
                    </Badge>
                  )}
                </div>

                <div className='flex items-center gap-2 flex-wrap'>
                  {/* 批量操作工具栏（按需显隐） / Batch operations toolbar (progressive disclosure) */}
                  {selectedNodeIds.size > 0 && (
                    <div className='flex items-center gap-1.5 p-1 bg-background/90 rounded-lg border shadow-2xs'>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => setConfirmBatchOp('start')}
                        disabled={isOperating}
                        className='h-7 text-xs text-emerald-600 dark:text-emerald-400'
                      >
                        {isOperating ? (
                          <Loader2 className='h-3.5 w-3.5 mr-1 animate-spin' />
                        ) : (
                          <Play className='h-3.5 w-3.5 mr-1' />
                        )}
                        {t('cluster.start')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => setConfirmBatchOp('stop')}
                        disabled={isOperating}
                        className='h-7 text-xs text-amber-600 dark:text-amber-400'
                      >
                        {isOperating ? (
                          <Loader2 className='h-3.5 w-3.5 mr-1 animate-spin' />
                        ) : (
                          <Square className='h-3.5 w-3.5 mr-1' />
                        )}
                        {t('cluster.stop')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => setConfirmBatchOp('restart')}
                        disabled={isOperating}
                        className='h-7 text-xs text-blue-600 dark:text-blue-400'
                      >
                        {isOperating ? (
                          <Loader2 className='h-3.5 w-3.5 mr-1 animate-spin' />
                        ) : (
                          <RotateCcw className='h-3.5 w-3.5 mr-1' />
                        )}
                        {t('cluster.restart')}
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => setSelectedNodeIds(new Set())}
                        className='h-7 text-xs text-muted-foreground hover:text-foreground'
                      >
                        {t('cluster.clearSelection')}
                      </Button>
                    </div>
                  )}

                  {/* 节点搜索过滤框 / Node search input */}
                  <div className='relative w-48 sm:w-56'>
                    <Search className='absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground' />
                    <Input
                      placeholder={t('cluster.searchNodes')}
                      value={nodeSearchTerm}
                      onChange={(e) => setNodeSearchTerm(e.target.value)}
                      className='pl-8 h-8 text-xs'
                    />
                  </div>

                  <Button
                    size='sm'
                    onClick={() => setIsAddNodeDialogOpen(true)}
                    className='h-8'
                  >
                    <Plus className='h-3.5 w-3.5 mr-1.5' />
                    {t('cluster.addNode')}
                  </Button>
                </div>
              </CardHeader>
              {runningScaleOperation && (
                <div className='mx-4 mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-primary/30 bg-primary/5 px-3 py-2 text-sm'>
                  <span className='flex items-center gap-2'>
                    {runningScaleOperation.status === 'running' ? (
                      <Loader2 className='h-4 w-4 animate-spin text-primary' />
                    ) : (
                      <AlertTriangle className='h-4 w-4 text-red-600' />
                    )}
                    {runningScaleOperation.status === 'running'
                      ? t('cluster.scale.inProgressBanner', {
                          kind: t(`cluster.scale.kind.${runningScaleOperation.kind}`),
                          step: t(`cluster.scale.steps.${runningScaleOperation.current_step}`),
                        })
                      : t('cluster.scale.operationFailed', {
                          error: runningScaleOperation.error || '-',
                        })}
                  </span>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setScaleProgressOpen(true)}
                  >
                    {t('cluster.scale.viewProgress')}
                  </Button>
                </div>
              )}
              <CardContent className='p-0 flex-1'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-12 pl-4'>
                        <Checkbox
                          checked={
                            filteredNodes.length > 0 &&
                            filteredNodes.every((n) =>
                              selectedNodeIds.has(n.id),
                            )
                          }
                          onCheckedChange={toggleAllNodes}
                        />
                      </TableHead>
                      <TableHead className='w-16'>ID</TableHead>
                      <TableHead>{t('cluster.hostName')}</TableHead>
                      <TableHead>{t('cluster.hostIP')}</TableHead>
                      <TableHead>{t('cluster.nodeRole')}</TableHead>
                      <TableHead>{t('cluster.installDir')}</TableHead>
                      <TableHead>{t('cluster.nodeStatus')}</TableHead>
                      <TableHead>{t('cluster.processPID')}</TableHead>
                      <TableHead className='text-right pr-4'>
                        {t('cluster.actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredNodes.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={9}
                          className='text-center py-12 text-muted-foreground text-sm'
                        >
                          {nodeSearchTerm || nodeStatusFilter !== 'all'
                            ? t('common.noMatchingData')
                            : t('cluster.noNodes')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      filteredNodes.map((node) => (
                        <TableRow key={node.id} className='hover:bg-muted/20'>
                          <TableCell className='pl-4'>
                            <Checkbox
                              checked={selectedNodeIds.has(node.id)}
                              onCheckedChange={() =>
                                toggleNodeSelection(node.id)
                              }
                            />
                          </TableCell>
                          <TableCell className='font-mono text-xs whitespace-nowrap text-muted-foreground'>
                            #{node.id}
                          </TableCell>
                          <TableCell className='font-medium whitespace-nowrap'>
                            {node.host_name || '-'}
                          </TableCell>
                          <TableCell className='font-mono text-xs whitespace-nowrap'>
                            {node.host_ip || '-'}
                          </TableCell>
                          <TableCell>
                            <Badge variant='outline' className='text-xs'>
                              {t(
                                `cluster.roles.${getRoleTranslationKey(node.role)}`,
                              )}
                            </Badge>
                          </TableCell>
                          <TableCell className='font-mono text-xs max-w-[180px]'>
                            <div
                              className='truncate'
                              title={node.install_dir || '-'}
                            >
                              {node.install_dir || '-'}
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge variant={getStatusBadgeVariant(node.status)} className='text-xs'>
                              {t(`cluster.nodeStatuses.${node.status}`)}
                            </Badge>
                          </TableCell>
                          <TableCell className='font-mono text-xs whitespace-nowrap'>
                            {node.process_pid || '-'}
                          </TableCell>
                          <TableCell className='text-right pr-4'>
                            <div className='flex items-center gap-0.5 justify-end'>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-emerald-600 hover:text-emerald-700 hover:bg-emerald-50 dark:hover:bg-emerald-950/30'
                                onClick={() =>
                                  setConfirmNodeOp({op: 'start', node})
                                }
                                disabled={
                                  nodeOperating === node.id ||
                                  node.status === NodeStatus.RUNNING ||
                                  node.status === NodeStatus.OFFLINE
                                }
                                title={t('cluster.start')}
                              >
                                {nodeOperating === node.id ? (
                                  <Loader2 className='h-4 w-4 animate-spin' />
                                ) : (
                                  <Play className='h-4 w-4' />
                                )}
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-amber-600 hover:text-amber-700 hover:bg-amber-50 dark:hover:bg-amber-950/30'
                                onClick={() =>
                                  setConfirmNodeOp({op: 'stop', node})
                                }
                                disabled={
                                  nodeOperating === node.id ||
                                  node.status !== NodeStatus.RUNNING
                                }
                                title={t('cluster.stop')}
                              >
                                <Square className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-blue-600 hover:text-blue-700 hover:bg-blue-50 dark:hover:bg-blue-950/30'
                                onClick={() =>
                                  setConfirmNodeOp({op: 'restart', node})
                                }
                                disabled={
                                  nodeOperating === node.id ||
                                  node.status !== NodeStatus.RUNNING
                                }
                                title={t('cluster.restart')}
                              >
                                <RotateCcw className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-muted-foreground hover:text-foreground'
                                onClick={() => handleViewLogs(node)}
                                title={t('cluster.viewLogs')}
                              >
                                <FileText className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-muted-foreground hover:text-foreground'
                                onClick={() => openEditNodeDialog(node)}
                                title={t('cluster.editNode')}
                              >
                                <Pencil className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                className='h-8 w-8 text-destructive/80 hover:text-destructive hover:bg-destructive/10'
                                onClick={() => setNodeToRemove(node)}
                                title={t('cluster.removeNode')}
                              >
                                <Trash2 className='h-4 w-4' />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

            {/* Process Events / 进程事件 */}
            <ProcessEventList clusterId={clusterId} />
          </TabsContent>

          <TabsContent value='plugins' className='space-y-4'>
            {/* Installed Plugins / 已安装插件 */}
            <ClusterPlugins clusterId={clusterId} />
          </TabsContent>

          <TabsContent value='configs' className='space-y-4'>
            {/* SeaTunnel 事件上报通道（改写 seatunnel.yaml）/ SeaTunnel event report channel (rewrites seatunnel.yaml) */}
            <EngineEventChannelCard clusterId={clusterId} onConfigChanged={handleConfigChanged} />
            {/* Cluster Configs / 集群配置 */}
            <ClusterConfigs
              clusterId={clusterId}
              deploymentMode={cluster.deployment_mode}
              onConfigChanged={handleConfigChanged}
            />
          </TabsContent>

          <TabsContent value='diagnostics' className='space-y-6'>
            {/* Diagnostics Summary / 诊断摘要 */}
            <Card>
              <CardHeader className='flex flex-row items-center justify-between space-y-0'>
                <div>
                  <CardTitle>{t('diagnosticsCenter.errors.title')}</CardTitle>
                  <CardDescription>
                    {t('diagnosticsCenter.errors.clusterScopedHint', {
                      name: cluster.name,
                    })}
                  </CardDescription>
                </div>
                <div className='flex flex-wrap items-center gap-2'>
                  <Button
                    onClick={() => void handleStartInspection()}
                    disabled={inspectionStarting}
                  >
                    {inspectionStarting ? (
                      <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                    ) : (
                      <Activity className='mr-2 h-4 w-4' />
                    )}
                    {t('diagnosticsCenter.inspections.startInspection')}
                  </Button>
                  <Button
                    variant='outline'
                    onClick={() =>
                      router.push(
                        `/diagnostics?tab=errors&cluster_id=${clusterId}&source=cluster-detail-summary`,
                      )
                    }
                  >
                    <Bug className='h-4 w-4 mr-2' />
                    {t('cluster.openDiagnostics')}
                  </Button>
                </div>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant='outline'>
                    {t('diagnosticsCenter.errors.matchedGroups', {
                      count: diagnosticsGroupTotal,
                    })}
                  </Badge>
                </div>
                {diagnosticsLoading ? (
                  <div className='space-y-2'>
                    <div className='h-10 rounded-md bg-muted/60' />
                    <div className='h-10 rounded-md bg-muted/60' />
                    <div className='h-10 rounded-md bg-muted/60' />
                  </div>
                ) : diagnosticsGroups.length === 0 ? (
                  <div className='rounded-lg border border-dashed p-4 text-sm text-muted-foreground'>
                    {t('diagnosticsCenter.errors.empty')}
                  </div>
                ) : (
                  <div className='space-y-3'>
                    {diagnosticsGroups.map((group) => (
                      <button
                        key={group.id}
                        type='button'
                        className='flex w-full items-start justify-between rounded-lg border p-3 text-left transition-colors hover:bg-muted/30'
                        onClick={() =>
                          router.push(
                            `/diagnostics?tab=errors&cluster_id=${clusterId}&group_id=${group.id}&source=cluster-detail-summary`,
                          )
                        }
                      >
                        <div className='min-w-0 space-y-1'>
                          <div className='truncate font-medium'>
                            {group.title ||
                              group.sample_message ||
                              group.fingerprint}
                          </div>
                          <div className='truncate text-sm text-muted-foreground'>
                            {group.exception_class ||
                              group.sample_message ||
                              '-'}
                          </div>
                        </div>
                        <div className='ml-4 flex shrink-0 items-center gap-2'>
                          <Badge
                            variant={
                              group.occurrence_count >= 10
                                ? 'destructive'
                                : group.occurrence_count >= 3
                                  ? 'secondary'
                                  : 'outline'
                            }
                          >
                            {group.occurrence_count}
                          </Badge>
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value='upgrades' className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Activity className='h-5 w-5' />
                  {t('stUpgrade.upgradeRecordsTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent className='space-y-4'>
                {upgradeTasksLoading ? (
                  <div className='flex items-center justify-center py-10 text-muted-foreground'>
                    <Loader2 className='h-5 w-5 animate-spin' />
                  </div>
                ) : upgradeTasks.length === 0 ? (
                  <div className='rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground'>
                    {t('stUpgrade.noUpgradeRecords')}
                  </div>
                ) : (
                  <>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>ID</TableHead>
                          <TableHead>{t('stUpgrade.sourceVersion')}</TableHead>
                          <TableHead>{t('stUpgrade.targetVersion')}</TableHead>
                          <TableHead>{t('stUpgrade.taskStatus')}</TableHead>
                          <TableHead>{t('stUpgrade.rollbackStatus')}</TableHead>
                          <TableHead>{t('stUpgrade.currentStep')}</TableHead>
                          <TableHead>{t('stUpgrade.createdAt')}</TableHead>
                          <TableHead className='text-right'>
                            {t('common.actions')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {upgradeTasks.map((upgradeTask) => (
                          <TableRow
                            key={upgradeTask.id}
                            className='cursor-pointer'
                            onClick={() => openUpgradeTaskDetail(upgradeTask)}
                          >
                            <TableCell className='font-mono text-xs'>
                              #{upgradeTask.id}
                            </TableCell>
                            <TableCell>
                              {upgradeTask.source_version || '-'}
                            </TableCell>
                            <TableCell>
                              {upgradeTask.target_version || '-'}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={getUpgradeStatusBadgeVariant(
                                  upgradeTask.status,
                                )}
                              >
                                {getExecutionStatusLabel(upgradeTask.status)}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={getUpgradeStatusBadgeVariant(
                                  upgradeTask.rollback_status,
                                )}
                              >
                                {getExecutionStatusLabel(
                                  upgradeTask.rollback_status,
                                )}
                              </Badge>
                            </TableCell>
                            <TableCell className='font-mono text-xs'>
                              {upgradeTask.current_step || '-'}
                            </TableCell>
                            <TableCell className='text-muted-foreground'>
                              {formatDateTimeTz(
                                upgradeTask.created_at,
                              )}
                            </TableCell>
                            <TableCell className='text-right'>
                              <Button
                                variant='outline'
                                size='sm'
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openUpgradeTaskDetail(upgradeTask);
                                }}
                              >
                                {t('stUpgrade.viewUpgradeDetail')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>

                    <Pagination
                      currentPage={upgradeTasksPage}
                      totalPages={upgradeTaskTotalPages}
                      pageSize={upgradeTasksPageSize}
                      totalItems={upgradeTasksTotal}
                      onPageChange={setUpgradeTasksPage}
                      showPageSizeSelector={false}
                      showTotalItems={true}
                    />
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </motion.div>

      {/* Edit Cluster Dialog / 编辑集群对话框 */}
      <EditClusterDialog
        open={isEditDialogOpen}
        onOpenChange={setIsEditDialogOpen}
        cluster={cluster}
        onSuccess={handleClusterUpdated}
      />

      {/* Add Node Dialog / 添加节点对话框 */}
      <AddNodeDialog
        open={isAddNodeDialogOpen}
        onOpenChange={(next) => {
          setIsAddNodeDialogOpen(next);
          // 执行中关闭弹窗时由横幅接管进度 / The banner takes over progress when closed mid-run
          if (!next) {
            void loadRunningScaleOperation();
          }
        }}
        clusterId={clusterId}
        deploymentMode={cluster.deployment_mode}
        clusterConfig={cluster.config}
        clusterInstallDir={cluster.install_dir}
        existingNodes={nodes}
        onSuccess={handleNodeAdded}
      />

      {/* Edit Node Dialog / 编辑节点对话框 */}
      <EditNodeDialog
        open={isEditNodeDialogOpen}
        onOpenChange={setIsEditNodeDialogOpen}
        node={nodeToEdit}
        deploymentMode={cluster.deployment_mode}
        clusterConfig={cluster.config}
        clusterInstallDir={cluster.install_dir}
        onSuccess={handleNodeEdited}
      />

      {/* Delete Cluster Dialog / 删除集群对话框 */}
      <AlertDialog
        open={isDeleteDialogOpen}
        onOpenChange={(open) => {
          setIsDeleteDialogOpen(open);
          if (!open) {
            setForceDelete(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cluster.deleteCluster')}</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div className='space-y-3'>
                <p>{t('cluster.deleteConfirm', {name: cluster.name})}</p>
                <p className='text-sm text-muted-foreground'>
                  {t('cluster.deleteConfirmWarning')}
                </p>
                <label className='flex items-center gap-2 cursor-pointer'>
                  <Checkbox
                    checked={forceDelete}
                    onCheckedChange={(v) => setForceDelete(v === true)}
                  />
                  <span className='text-sm'>
                    {t('cluster.forceDeleteOption')}
                  </span>
                </label>
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Remove Node (scale-in) Dialog / 移除节点（缩容）对话框 */}
      <RemoveNodeDialog
        clusterId={clusterId}
        node={nodeToRemove}
        onOpenChange={(next) => {
          if (!next) {
            setNodeToRemove(null);
            void loadRunningScaleOperation();
          }
        }}
        onRemoved={handleNodeRemoved}
      />

      {/* Running scaling operation progress / 进行中的伸缩操作进度 */}
      <Dialog
        open={scaleProgressOpen && runningScaleOperation !== null}
        onOpenChange={setScaleProgressOpen}
      >
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {runningScaleOperation
                ? t(`cluster.scale.kind.${runningScaleOperation.kind}`)
                : ''}
            </DialogTitle>
          </DialogHeader>
          {runningScaleOperation && (
            <div className='max-h-[64vh] overflow-y-auto pr-1'>
              <ScaleOperationProgress
                clusterId={clusterId}
                operationId={runningScaleOperation.id}
                initial={runningScaleOperation}
                onFinished={handleRunningScaleFinished}
              />
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Batch start/stop/restart confirmation / 批量启动/停止/重启二次确认 */}
      <AlertDialog
        open={!!confirmBatchOp}
        onOpenChange={() => setConfirmBatchOp(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmBatchOp === 'start' &&
                t('cluster.batchStartConfirmTitle')}
              {confirmBatchOp === 'stop' && t('cluster.batchStopConfirmTitle')}
              {confirmBatchOp === 'restart' &&
                t('cluster.batchRestartConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmBatchOp === 'start' &&
                t('cluster.batchStartConfirmMessage', {
                  count: selectedNodeIds.size,
                })}
              {confirmBatchOp === 'stop' &&
                t('cluster.batchStopConfirmMessage', {
                  count: selectedNodeIds.size,
                })}
              {confirmBatchOp === 'restart' &&
                t('cluster.batchRestartConfirmMessage', {
                  count: selectedNodeIds.size,
                })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                if (confirmBatchOp === 'start') {
                  await handleBatchStart();
                } else if (confirmBatchOp === 'stop') {
                  await handleBatchStop();
                } else if (confirmBatchOp === 'restart') {
                  await handleBatchRestart();
                }
                setConfirmBatchOp(null);
              }}
            >
              {confirmBatchOp === 'start' && t('cluster.start')}
              {confirmBatchOp === 'stop' && t('cluster.stop')}
              {confirmBatchOp === 'restart' && t('cluster.restart')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Single node start/stop/restart confirmation / 单节点启动/停止/重启二次确认 */}
      <AlertDialog
        open={!!confirmNodeOp}
        onOpenChange={() => setConfirmNodeOp(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmNodeOp?.op === 'start' &&
                t('cluster.nodeStartConfirmTitle')}
              {confirmNodeOp?.op === 'stop' &&
                t('cluster.nodeStopConfirmTitle')}
              {confirmNodeOp?.op === 'restart' &&
                t('cluster.nodeRestartConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmNodeOp &&
                (confirmNodeOp.op === 'start'
                  ? t('cluster.nodeStartConfirmMessage', {
                      name:
                        confirmNodeOp.node.host_name ||
                        confirmNodeOp.node.host_ip ||
                        `#${confirmNodeOp.node.id}`,
                    })
                  : confirmNodeOp.op === 'stop'
                    ? t('cluster.nodeStopConfirmMessage', {
                        name:
                          confirmNodeOp.node.host_name ||
                          confirmNodeOp.node.host_ip ||
                          `#${confirmNodeOp.node.id}`,
                      })
                    : t('cluster.nodeRestartConfirmMessage', {
                        name:
                          confirmNodeOp.node.host_name ||
                          confirmNodeOp.node.host_ip ||
                          `#${confirmNodeOp.node.id}`,
                      }))}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                if (!confirmNodeOp) {
                  return;
                }
                if (confirmNodeOp.op === 'start') {
                  await handleNodeStart(confirmNodeOp.node);
                } else if (confirmNodeOp.op === 'stop') {
                  await handleNodeStop(confirmNodeOp.node);
                } else {
                  await handleNodeRestart(confirmNodeOp.node);
                }
                setConfirmNodeOp(null);
              }}
            >
              {confirmNodeOp?.op === 'start' && t('cluster.start')}
              {confirmNodeOp?.op === 'stop' && t('cluster.stop')}
              {confirmNodeOp?.op === 'restart' && t('cluster.restart')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={imapCleanupOpen} onOpenChange={setImapCleanupOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('cluster.runtimeStorage.cleanupConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('cluster.runtimeStorage.cleanupConfirmDesc')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleCleanupIMAP()}>
              {imapCleanupRunning && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {t('cluster.runtimeStorage.cleanupImap')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={runtimeStoragePreviewOpen}
        onOpenChange={setRuntimeStoragePreviewOpen}
      >
        <DialogContent className='max-w-4xl'>
          <DialogHeader>
            <div className='flex items-center justify-between gap-3 pr-6'>
              <div className='min-w-0'>
                <DialogTitle className='flex items-center gap-2'>
                  {runtimeStoragePreview?.file_name?.endsWith('.ser') ? (
                    <Database className='size-5 text-purple-400' />
                  ) : runtimeStoragePreview?.binary ? (
                    <File className='size-5 text-muted-foreground' />
                  ) : (
                    <FileText className='size-5 text-sky-400' />
                  )}
                  <span className='truncate font-mono'>
                    {runtimeStoragePreview?.file_name ||
                      t('cluster.runtimeStorage.preview')}
                  </span>
                </DialogTitle>
                <DialogDescription className='mt-1 break-all font-mono text-xs text-muted-foreground flex items-center gap-1.5'>
                  <span className='truncate'>
                    {runtimeStoragePreview?.path || '-'}
                  </span>
                  {runtimeStoragePreview?.path && (
                    <Button
                      variant='ghost'
                      size='icon'
                      className='size-5 shrink-0 text-muted-foreground hover:text-foreground'
                      onClick={() => {
                        if (runtimeStoragePreview?.path) {
                          void navigator.clipboard.writeText(
                            runtimeStoragePreview.path,
                          );
                          toast.success(
                            t('cluster.runtimeStorage.copyPathSuccess'),
                          );
                        }
                      }}
                      title={t('cluster.runtimeStorage.copyPath')}
                    >
                      <Copy className='size-3' />
                    </Button>
                  )}
                </DialogDescription>
              </div>

              {/* 若为 Checkpoint 序列化文件，提供一键反序列化直达动作 */}
              {/* If it's a Checkpoint serialization file, provide a quick deserialize action */}
              {runtimeStoragePreview?.path?.endsWith('.ser') && (
                <Button
                  size='sm'
                  variant='default'
                  className='bg-purple-600 hover:bg-purple-700 text-white shrink-0 active:scale-95'
                  onClick={() => {
                    const p = runtimeStoragePreview?.path;
                    if (p) {
                      setRuntimeStoragePreviewOpen(false);
                      void handleInspectCheckpoint(p);
                    }
                  }}
                >
                  <Database className='mr-1.5 size-3.5' />
                  {t('cluster.runtimeStorage.deserializeCheckpoint')}
                </Button>
              )}
            </div>
          </DialogHeader>

          <div className='grid gap-3 rounded-lg border bg-muted/20 p-3 text-xs md:grid-cols-4'>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.fileName')}
              </div>
              <div className='font-mono font-medium break-all text-foreground mt-0.5'>
                {runtimeStoragePreview?.file_name || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.size')}
              </div>
              <div className='font-mono font-medium text-foreground mt-0.5'>
                {formatBytes(runtimeStoragePreview?.size_bytes)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.encoding')}
              </div>
              <div className='font-mono font-medium text-foreground mt-0.5 flex items-center gap-1.5'>
                <span>{runtimeStoragePreview?.encoding || '-'}</span>
                <Badge variant='outline' className='text-[10px] px-1 py-0'>
                  {runtimeStoragePreview?.binary ? 'Binary' : 'Text'}
                </Badge>
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.truncated')}
              </div>
              <div className='font-mono font-medium text-foreground mt-0.5'>
                {runtimeStoragePreview?.truncated ? 'Yes (64KB max)' : 'No'}
              </div>
            </div>
          </div>

          <ExpandableTextPanel
            title={t('cluster.runtimeStorage.fileName')}
            subtitle={runtimeStoragePreview?.file_name || '-'}
            content={
              runtimeStoragePreview?.binary
                ? runtimeStoragePreview?.hex_preview || '-'
                : runtimeStoragePreview?.text_preview || '-'
            }
            height='50vh'
            tone='plain'
            wrap
          />
        </DialogContent>
      </Dialog>

      <Dialog open={stxJavaProxyLogOpen} onOpenChange={setStxJavaProxyLogOpen}>
        <WorkbenchDialogContent className='sm:max-w-[1560px] gap-0 p-0'>
          <DialogHeader className='shrink-0 space-y-1 border-b px-6 py-4 pr-12 text-left'>
            <DialogTitle>
              {t('cluster.stxJavaProxy.viewRuntimeLog')}
            </DialogTitle>
            <DialogDescription className='break-all'>
              {stxJavaProxyLogResult?.log_path || stxJavaProxy?.log_path || '-'}
            </DialogDescription>
          </DialogHeader>
          <div className='flex min-h-0 flex-1 flex-col p-4'>
            <ExpandableTextPanel
              content={stxJavaProxyLogResult?.logs || '-'}
              height='flex'
              className='h-full min-h-0'
              tone='plain'
              wrap
              showExpand={false}
            />
          </div>
        </WorkbenchDialogContent>
      </Dialog>

      <Dialog
        open={stxJavaProxyConfigOpen}
        onOpenChange={setStxJavaProxyConfigOpen}
      >
        <DialogContent className='max-w-xl sm:max-w-xl'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <Cpu className='h-5 w-5 text-primary' />
              {t('cluster.stxJavaProxy.configDialogTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('cluster.stxJavaProxy.configDialogDescription')}
            </DialogDescription>
          </DialogHeader>

          <div className='space-y-5 py-3 text-sm'>
            <div className='space-y-2'>
              <Label className='font-medium'>
                {t('cluster.stxJavaProxy.presetLabel')}
              </Label>
              <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
                {[
                  {id: '512m', label: t('cluster.stxJavaProxy.preset512m')},
                  {id: '1024m', label: t('cluster.stxJavaProxy.preset1g')},
                  {id: '2048m', label: t('cluster.stxJavaProxy.preset2g')},
                  {id: '4096m', label: t('cluster.stxJavaProxy.preset4g')},
                  {id: 'custom', label: t('cluster.stxJavaProxy.presetCustom')},
                ].map((preset) => (
                  <Button
                    key={preset.id}
                    type='button'
                    variant={
                      stxJavaProxyJvmPreset === preset.id
                        ? 'default'
                        : 'outline'
                    }
                    size='sm'
                    className='justify-start text-xs font-normal'
                    onClick={() =>
                      handleSelectJvmPreset(
                        preset.id as
                          | '512m'
                          | '1024m'
                          | '2048m'
                          | '4096m'
                          | 'custom',
                      )
                    }
                  >
                    {preset.label}
                  </Button>
                ))}
              </div>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='stx-java-proxy-jvm-opts' className='font-medium'>
                {t('cluster.stxJavaProxy.jvmOptsField')}
              </Label>
              <Input
                id='stx-java-proxy-jvm-opts'
                value={stxJavaProxyJvmOptsInput}
                onChange={(e) => {
                  setStxJavaProxyJvmOptsInput(e.target.value);
                  setStxJavaProxyJvmPreset('custom');
                }}
                placeholder={t('cluster.stxJavaProxy.jvmOptsPlaceholder')}
                className='font-mono text-xs'
                data-testid='stx-java-proxy-jvm-opts-input'
              />
              <p className='text-xs text-muted-foreground'>
                {t('cluster.stxJavaProxy.jvmOptsHelp')}
              </p>
            </div>

            <div className='flex items-start space-x-2 pt-2 border-t'>
              <Checkbox
                id='stx-java-proxy-restart-save'
                checked={stxJavaProxyRestartOnSave}
                onCheckedChange={(checked) =>
                  setStxJavaProxyRestartOnSave(Boolean(checked))
                }
              />
              <div className='grid gap-1 leading-none'>
                <Label
                  htmlFor='stx-java-proxy-restart-save'
                  className='text-sm font-medium cursor-pointer'
                >
                  {t('cluster.stxJavaProxy.restartImmediately')}
                </Label>
                <p className='text-xs text-muted-foreground'>
                  {t('cluster.stxJavaProxy.restartImmediatelyHelp')}
                </p>
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => setStxJavaProxyConfigOpen(false)}
              disabled={stxJavaProxySaving}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleSaveStxJavaProxyConfig()}
              disabled={stxJavaProxySaving}
              data-testid='stx-java-proxy-save-config-btn'
            >
              {stxJavaProxySaving ? (
                <>
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  {t('cluster.stxJavaProxy.saving')}
                </>
              ) : (
                t('cluster.stxJavaProxy.saveAndApply')
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 与工作台共用 Checkpoint 双看板；支持自动关联作业或手动指定连接器解析位点。
          Reuse workbench Checkpoint dual-pane dialog; supports auto job config association and manual connector decode. */}
      <CheckpointInspectDialog
        open={checkpointInspectOpen}
        onOpenChange={setCheckpointInspectOpen}
        result={checkpointInspectResult}
        t={tCheckpoint}
        onResultUpdate={setCheckpointInspectResult}
      />

      <Dialog open={imapInspectOpen} onOpenChange={setImapInspectOpen}>
        <DialogContent className='max-w-5xl'>
          <DialogHeader>
            <DialogTitle>{t('cluster.runtimeStorage.inspectWal')}</DialogTitle>
            <DialogDescription className='break-all'>
              {imapInspectResult?.path || '-'}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 text-sm md:grid-cols-5'>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.fileName')}
              </div>
              <div className='font-medium break-all'>
                {imapInspectResult?.file_name || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.size')}
              </div>
              <div className='font-medium'>
                {formatBytes(imapInspectResult?.size_bytes)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.encoding')}
              </div>
              <div className='font-medium'>
                {imapInspectResult?.encoding || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.storageType')}
              </div>
              <div className='font-medium'>
                {imapInspectResult?.storage_type || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.entryCount')}
              </div>
              <div className='font-medium'>
                {imapInspectResult?.entry_count ?? 0}
              </div>
            </div>
          </div>
          <ExpandableTextPanel
            title={t('cluster.runtimeStorage.inspectWal')}
            subtitle={imapInspectResult?.path || '-'}
            content={JSON.stringify(
              {
                entries: imapInspectResult?.entries,
              },
              null,
              2,
            )}
            height='55vh'
            tone='plain'
            wrap
          />
        </DialogContent>
      </Dialog>

      {/* View Logs Dialog / 查看日志对话框 */}
      <ClusterNodeLogDialog
        open={isLogDialogOpen}
        onOpenChange={setIsLogDialogOpen}
        node={logNodeInfo}
        clusterId={clusterId}
      />
    </motion.div>
  );
}
