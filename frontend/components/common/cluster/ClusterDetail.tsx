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

import {useState, useEffect, useCallback, type KeyboardEvent} from 'react';
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
import {Separator} from '@/components/ui/separator';
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
  DialogHeader,
  DialogTitle,
  WorkbenchDialogContent,
} from '@/components/ui/dialog';
import {ScrollArea} from '@/components/ui/scroll-area';
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
  FileText,
  ExternalLink,
  MonitorSmartphone,
  FolderOpen,
  Eye,
  ChevronUp,
  Database,
  Search,
} from 'lucide-react';
import {Checkbox} from '@/components/ui/checkbox';
import {motion} from 'motion/react';
import services from '@/lib/services';
import {
  ClusterInfo,
  ClusterStatus,
  ClusterStatusInfo,
  NodeInfo,
  NodeStatus,
  HealthStatus,
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
import {EditNodeDialog} from './EditNodeDialog';
import {ClusterPlugins} from './ClusterPlugins';
import {ClusterConfigs} from './ClusterConfigs';
import {MonitorConfigPanel} from './MonitorConfigPanel';
import {ProcessEventList} from './ProcessEventList';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';

interface ClusterDetailProps {
  clusterId: number;
}

type ClusterDetailTab =
  | 'nodes'
  | 'overview'
  | 'storage'
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
 * Get health status badge variant
 * 获取健康状态徽章变体
 */
function getHealthBadgeVariant(
  status: HealthStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case HealthStatus.HEALTHY:
      return 'default';
    case HealthStatus.UNHEALTHY:
      return 'destructive';
    case HealthStatus.UNKNOWN:
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
  const [isLogDialogOpen, setIsLogDialogOpen] = useState(false);
  const [logContent, setLogContent] = useState<string>('');
  const [logLoading, setLogLoading] = useState(false);
  const [logNodeInfo, setLogNodeInfo] = useState<NodeInfo | null>(null);
  // Log query parameters / 日志查询参数
  const [logLines, setLogLines] = useState<number>(100);
  const [logMode, setLogMode] = useState<string>('tail');
  const [logFilter, setLogFilter] = useState<string>('');
  const [logDate, setLogDate] = useState<string>('');

  /**
   * Load cluster data
   * 加载集群数据
   */
  const loadClusterData = useCallback(async () => {
    setLoading(true);
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
      setLoading(false);
    }
  }, [clusterId, t]);

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
    async (kind: 'checkpoint' | 'imap', path?: string) => {
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
        setRuntimeStorageListing((prev) => ({...prev, [kind]: result.data}));
      } finally {
        setRuntimeStorageListingLoading(null);
      }
    },
    [clusterId],
  );

  const loadRuntimeStorage = useCallback(async () => {
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
        );
      }
      if (result.data.imap?.enabled) {
        void loadRuntimeStorageList('imap', result.data.imap.namespace);
      }
    } finally {
      setRuntimeStorageLoading(false);
    }
  }, [clusterId, loadRuntimeStorageList]);

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
    if (activeTab !== 'storage') {
      return;
    }
    void loadRuntimeStorage();
    void loadStxJavaProxyStatus();
  }, [activeTab, loadRuntimeStorage, loadStxJavaProxyStatus]);

  const openUpgradeTaskDetail = useCallback(
    (task: UpgradeTaskSummary) => {
      router.push(
        `/clusters/${clusterId}/upgrade/execute?planId=${task.plan_id}&taskId=${task.id}`,
      );
    },
    [clusterId, router],
  );

  const handleRefresh = useCallback(() => {
    void loadClusterData();
    void loadDiagnosticsSummary();
    void loadMonitoringOverview();
    void loadRuntimeStorage();
    void loadStxJavaProxyStatus();
    void loadUpgradeTasks(upgradeTasksPage, upgradeTasksPageSize);
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
   * Handle remove node
   * 处理移除节点
   */
  const handleRemoveNode = async () => {
    if (!nodeToRemove) {
      return;
    }

    const result = await services.cluster.removeNodeSafe(
      clusterId,
      nodeToRemove.id,
    );
    if (result.success) {
      toast.success(t('cluster.removeNodeSuccess'));
      loadClusterData();
    } else {
      toast.error(result.error || t('cluster.removeNodeError'));
    }
    setNodeToRemove(null);
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
    setIsAddNodeDialogOpen(false);
    loadClusterData();
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
    if (selectedNodeIds.size === nodes.length) {
      setSelectedNodeIds(new Set());
    } else {
      setSelectedNodeIds(new Set(nodes.map((n) => n.id)));
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
  const handleViewLogs = async (node: NodeInfo) => {
    setLogNodeInfo(node);
    setIsLogDialogOpen(true);
    // Reset parameters / 重置参数
    setLogLines(100);
    setLogMode('tail');
    setLogFilter('');
    setLogDate('');
    await fetchLogs(node.id, {lines: 100, mode: 'tail'});
  };

  /**
   * Fetch logs with parameters
   * 使用参数获取日志
   */
  const fetchLogs = async (
    nodeId: number,
    params: {lines?: number; mode?: string; filter?: string; date?: string},
  ) => {
    setLogLoading(true);
    setLogContent('');
    try {
      const result = await services.cluster.getNodeLogsSafe(
        clusterId,
        nodeId,
        params,
      );
      if (result.success && result.data) {
        setLogContent(result.data.logs || t('cluster.noLogs'));
      } else {
        setLogContent(result.error || t('cluster.getLogsError'));
      }
    } finally {
      setLogLoading(false);
    }
  };

  /**
   * Handle refresh logs with current parameters
   * 使用当前参数刷新日志
   */
  const handleRefreshLogs = () => {
    if (!logNodeInfo) {
      return;
    }
    fetchLogs(logNodeInfo.id, {
      lines: logLines,
      mode: logMode,
      filter: logFilter || undefined,
      date: logDate || undefined,
    });
  };

  /**
   * Submit log filters via Enter key
   * 通过 Enter 键提交日志过滤条件
   */
  const handleLogFilterKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') {
      return;
    }
    e.preventDefault();
    handleRefreshLogs();
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

  if (loading) {
    return (
      <div className='flex items-center justify-center py-12'>
        <Loader2 className='h-8 w-8 animate-spin text-muted-foreground' />
      </div>
    );
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
    const searchKeyword = (runtimeStorageSearch[kind] || '')
      .trim()
      .toLowerCase();
    const filteredItems = (listing?.items || []).filter((item) => {
      if (!searchKeyword) {
        return true;
      }
      const haystack = `${item.name || ''} ${item.path || ''}`.toLowerCase();
      return haystack.includes(searchKeyword);
    });
    const pageSize = 20;
    const currentPage = Math.max(1, runtimeStoragePage[kind] || 1);
    const totalPages = Math.max(1, Math.ceil(filteredItems.length / pageSize));
    const normalizedPage = Math.min(currentPage, totalPages);
    const pagedItems = filteredItems.slice(
      (normalizedPage - 1) * pageSize,
      normalizedPage * pageSize,
    );
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
          <div className='space-y-2 rounded-md border p-3'>
            <div className='flex items-center justify-between gap-2'>
              <div className='font-medium'>
                {t('cluster.runtimeStorage.fileList')}
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    void loadRuntimeStorageList(
                      spec.kind as 'checkpoint' | 'imap',
                      parentBrowsePath(
                        runtimeStorageBrowsePath[
                          spec.kind as 'checkpoint' | 'imap'
                        ],
                        spec.namespace,
                      ),
                    )
                  }
                  disabled={
                    runtimeStorageListingLoading === kind ||
                    trimTrailingSlashes(runtimeStorageBrowsePath[kind]) ===
                      trimTrailingSlashes(spec.namespace)
                  }
                >
                  <ChevronUp className='mr-2 h-4 w-4' />
                  {t('cluster.runtimeStorage.upLevel')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    void loadRuntimeStorageList(
                      kind,
                      runtimeStorageBrowsePath[kind] || spec.namespace,
                    )
                  }
                  disabled={runtimeStorageListingLoading === kind}
                >
                  {runtimeStorageListingLoading === kind ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <RefreshCw className='mr-2 h-4 w-4' />
                  )}
                  {t('common.refresh')}
                </Button>
              </div>
            </div>
            <div className='flex flex-col gap-2 md:flex-row md:items-center md:justify-between'>
              <div className='relative max-w-md flex-1'>
                <Search className='pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground' />
                <Input
                  value={runtimeStorageSearch[kind] || ''}
                  onChange={(event) => {
                    const value = event.target.value;
                    setRuntimeStorageSearch((prev) => ({
                      ...prev,
                      [kind]: value,
                    }));
                    setRuntimeStoragePage((prev) => ({...prev, [kind]: 1}));
                  }}
                  className='pl-9'
                  placeholder={t('cluster.runtimeStorage.searchEntries')}
                />
              </div>
              <div className='text-xs text-muted-foreground'>
                {t('cluster.runtimeStorage.filteredEntryCount', {
                  filtered: filteredItems.length,
                  total: listing?.items?.length || 0,
                })}
              </div>
            </div>
            <div className='text-xs text-muted-foreground break-all'>
              {runtimeStorageBrowsePath[kind] || spec.namespace || '-'}
            </div>
            {listing?.path && (
              <div className='text-xs text-muted-foreground break-all'>
                {listing?.path}
              </div>
            )}
            {filteredItems.length ? (
              <div className='space-y-2'>
                {pagedItems.map((item) => {
                  const itemPath = item.path || item.name || '';
                  const previewKey = `${kind}:${itemPath}`;
                  const isPreviewing =
                    runtimeStoragePreviewLoading === previewKey;
                  const isInspecting = checkpointInspectLoading === itemPath;
                  const isInspectingWal = imapInspectLoading === itemPath;
                  return (
                    <div
                      key={`${spec.kind}-item-${item.path}`}
                      className='space-y-2 rounded-md border px-3 py-2'
                    >
                      <div className='flex items-start justify-between gap-3'>
                        <div className='min-w-0'>
                          <div className='font-medium break-all'>
                            {item.name || item.path || '-'}
                          </div>
                          <div className='text-xs text-muted-foreground break-all'>
                            {item.path || '-'}
                          </div>
                        </div>
                        <div className='text-right text-xs text-muted-foreground'>
                          <div>
                            {item.directory
                              ? item.size_bytes && item.size_bytes > 0
                                ? `${t('cluster.runtimeStorage.directory')} · ${formatBytes(item.size_bytes)}`
                                : t('cluster.runtimeStorage.directory')
                              : formatBytes(item.size_bytes || 0)}
                          </div>
                          <div>{item.modified_at || '-'}</div>
                        </div>
                      </div>
                      <div className='flex flex-wrap gap-2'>
                        {item.directory ? (
                          <Button
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              void loadRuntimeStorageList(kind, itemPath)
                            }
                          >
                            <FolderOpen className='mr-2 h-4 w-4' />
                            {t('cluster.runtimeStorage.openDirectory')}
                          </Button>
                        ) : (
                          <>
                            <Button
                              variant='outline'
                              size='sm'
                              onClick={() =>
                                void handlePreviewRuntimeStorage(kind, itemPath)
                              }
                              disabled={isPreviewing}
                            >
                              {isPreviewing ? (
                                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                              ) : (
                                <Eye className='mr-2 h-4 w-4' />
                              )}
                              {t('cluster.runtimeStorage.preview')}
                            </Button>
                            {kind === 'checkpoint' &&
                              itemPath.endsWith('.ser') && (
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() =>
                                    void handleInspectCheckpoint(itemPath)
                                  }
                                  disabled={isInspecting}
                                >
                                  {isInspecting ? (
                                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                                  ) : (
                                    <Database className='mr-2 h-4 w-4' />
                                  )}
                                  {t(
                                    'cluster.runtimeStorage.deserializeCheckpoint',
                                  )}
                                </Button>
                              )}
                            {kind === 'imap' &&
                              itemPath.endsWith('_wal.txt') && (
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() =>
                                    void handleInspectIMAPWAL(itemPath)
                                  }
                                  disabled={isInspectingWal}
                                >
                                  {isInspectingWal ? (
                                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                                  ) : (
                                    <Database className='mr-2 h-4 w-4' />
                                  )}
                                  {t('cluster.runtimeStorage.inspectWal')}
                                </Button>
                              )}
                          </>
                        )}
                      </div>
                    </div>
                  );
                })}
                {totalPages > 1 ? (
                  <div className='flex items-center justify-between border-t pt-2'>
                    <div className='text-xs text-muted-foreground'>
                      {t('cluster.runtimeStorage.pageInfo', {
                        current: normalizedPage,
                        total: totalPages,
                      })}
                    </div>
                    <div className='flex items-center gap-2'>
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={normalizedPage <= 1}
                        onClick={() =>
                          setRuntimeStoragePage((prev) => ({
                            ...prev,
                            [kind]: Math.max(1, normalizedPage - 1),
                          }))
                        }
                      >
                        {t('cluster.runtimeStorage.previousPage')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={normalizedPage >= totalPages}
                        onClick={() =>
                          setRuntimeStoragePage((prev) => ({
                            ...prev,
                            [kind]: Math.min(totalPages, normalizedPage + 1),
                          }))
                        }
                      >
                        {t('cluster.runtimeStorage.nextPage')}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </div>
            ) : (
              <div className='text-xs text-muted-foreground'>
                {searchKeyword
                  ? t('cluster.runtimeStorage.noFilteredEntries')
                  : t('cluster.runtimeStorage.noEntries')}
              </div>
            )}
          </div>
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

  return (
    <motion.div
      className='space-y-6'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      {/* Header / 标题 */}
      <motion.div
        className='flex items-center justify-between'
        variants={itemVariants}
      >
        <div className='flex items-center gap-4'>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => router.push('/clusters')}
          >
            <ArrowLeft className='h-5 w-5' />
          </Button>
          <div>
            <h1 className='text-2xl font-bold tracking-tight'>
              {cluster.name}
            </h1>
            {cluster.description && (
              <p className='text-muted-foreground mt-1'>
                {cluster.description}
              </p>
            )}
          </div>
        </div>
        <div className='flex gap-2'>
          <Button variant='outline' onClick={handleRefresh}>
            <RefreshCw className='h-4 w-4 mr-2' />
            {t('common.refresh')}
          </Button>
          <Button
            variant='outline'
            onClick={() =>
              router.push(
                `/diagnostics?tab=errors&cluster_id=${clusterId}&source=cluster-detail`,
              )
            }
          >
            <Bug className='h-4 w-4 mr-2' />
            {t('cluster.openDiagnostics')}
          </Button>
          <Button
            variant='outline'
            onClick={() =>
              router.push(`/clusters/${clusterId}/upgrade/prepare`)
            }
          >
            <Activity className='h-4 w-4 mr-2' />
            {t('stUpgrade.entry')}
          </Button>
          <Button variant='outline' onClick={() => setIsEditDialogOpen(true)}>
            <Pencil className='h-4 w-4 mr-2' />
            {t('common.edit')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => setIsDeleteDialogOpen(true)}
            disabled={!canDelete}
          >
            <Trash2 className='h-4 w-4 mr-2' />
            {t('common.delete')}
          </Button>
        </div>
      </motion.div>

      <Separator />

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
            {/* Cluster Info / 集群信息 */}
            <Card>
              <CardHeader>
                <CardTitle>{t('cluster.clusterInfo')}</CardTitle>
              </CardHeader>
              <CardContent>
                <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.installDir')}
                    </span>
                    <p className='font-medium break-all'>
                      {cluster.install_dir || '-'}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.version')}
                    </span>
                    <p className='font-medium'>{cluster.version || '-'}</p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.hazelcastPort')}
                    </span>
                    <p className='font-medium'>
                      {nodes.find((node) => node.hazelcast_port > 0)
                        ?.hazelcast_port || '-'}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.httpPort')}
                    </span>
                    <p className='font-medium'>
                      {runtimeConfig.enableHTTP === false
                        ? t('cluster.httpDisabled')
                        : webUINode?.api_port || '-'}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.logOutputMode')}
                    </span>
                    <p className='font-medium'>
                      {runtimeConfig.jobLogMode === 'per_job'
                        ? t('cluster.logOutputModePerJob')
                        : t('cluster.logOutputModeMixed')}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.webUiStatus')}
                    </span>
                    <p className='font-medium'>
                      {webUINode
                        ? t('common.enabled')
                        : isSeatunnelVersionAtLeast(cluster.version, '2.3.9')
                          ? t('cluster.httpDisabled')
                          : t('cluster.versionUnsupported')}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.createdAt')}
                    </span>
                    <p className='font-medium'>
                      {new Date(cluster.created_at).toLocaleString()}
                    </p>
                  </div>
                  <div>
                    <span className='text-sm text-muted-foreground'>
                      {t('cluster.updatedAt')}
                    </span>
                    <p className='font-medium'>
                      {new Date(cluster.updated_at).toLocaleString()}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          {webUINode && (
            <TabsContent value='webui' className='space-y-6'>
              <Card>
                <CardHeader className='flex flex-row items-center justify-between space-y-0'>
                  <div>
                    <CardTitle className='flex items-center gap-2'>
                      <MonitorSmartphone className='h-5 w-5' />
                      {t('cluster.webUiTitle')}
                    </CardTitle>
                    <CardDescription>
                      {t('cluster.webUiDescription', {
                        host: webUINode.host_ip || webUINode.host_name || '-',
                        port: webUINode.api_port,
                      })}
                    </CardDescription>
                  </div>
                  <Button
                    variant='outline'
                    onClick={() =>
                      window.open(
                        webUIProxyURL,
                        '_blank',
                        'noopener,noreferrer',
                      )
                    }
                  >
                    <ExternalLink className='mr-2 h-4 w-4' />
                    {t('cluster.openWebUiInNewWindow')}
                  </Button>
                </CardHeader>
                <CardContent className='space-y-4'>
                  <div className='rounded-md border bg-muted/20 p-3 text-xs text-muted-foreground'>
                    {t('cluster.webUiHint')}
                  </div>
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

          <TabsContent value='storage' className='space-y-6'>
            {/* Runtime Storage / 运行时存储 */}
            <Card>
              <CardHeader className='flex flex-row items-center justify-between space-y-0'>
                <div>
                  <CardTitle>{t('cluster.runtimeStorage.title')}</CardTitle>
                  <CardDescription>
                    {t('cluster.runtimeStorage.description')}
                  </CardDescription>
                </div>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void loadRuntimeStorage()}
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
                  <div className='flex items-center justify-center py-8'>
                    <Loader2 className='h-6 w-6 animate-spin text-muted-foreground' />
                  </div>
                ) : (
                  <>
                    <div className='text-xs text-muted-foreground'>
                      {t('cluster.runtimeStorage.configSource', {
                        source: runtimeStorage?.config_source || '-',
                      })}
                    </div>
                    <Card data-testid='stx-java-proxy-card'>
                      <CardHeader className='pb-3'>
                        <div className='flex items-start justify-between gap-4'>
                          <div>
                            <CardTitle className='text-base'>
                              {t('cluster.stxJavaProxy.title')}
                            </CardTitle>
                            <CardDescription>
                              {t('cluster.stxJavaProxy.description')}
                            </CardDescription>
                          </div>
                          <div className='flex items-center gap-2'>
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
                        </div>
                      </CardHeader>
                      <CardContent className='space-y-4 text-sm'>
                        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
                          <div>
                            <div className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.deploymentNode')}
                            </div>
                            <div className='font-medium break-all'>
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
                            <div className='font-medium break-all'>
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
                            <div className='font-medium break-all'>
                              {stxJavaProxy?.log_path &&
                              stxJavaProxy.log_path.trim() !== ''
                                ? stxJavaProxy.log_path
                                : '-'}
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
                      </CardContent>
                    </Card>
                    <div className='grid gap-4 xl:grid-cols-2'>
                      {renderRuntimeStorageSpec(
                        runtimeStorage?.checkpoint,
                        t('installer.checkpointConfig'),
                      )}
                      {renderRuntimeStorageSpec(
                        runtimeStorage?.imap,
                        t('installer.runtimeStorage.imapTitle'),
                      )}
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
            {/* Cluster Actions / 集群操作 */}
            <Card>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Activity className='h-5 w-5' />
                  {t('cluster.operations')}
                  {selectedNodeIds.size > 0 && (
                    <Badge variant='secondary' className='ml-2'>
                      {t('cluster.selectedNodes', {
                        count: selectedNodeIds.size,
                      })}
                    </Badge>
                  )}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className='flex gap-2'>
                  <Button
                    variant='outline'
                    onClick={() => setConfirmBatchOp('start')}
                    disabled={selectedNodeIds.size === 0 || isOperating}
                  >
                    {isOperating ? (
                      <Loader2 className='h-4 w-4 mr-2 animate-spin' />
                    ) : (
                      <Play className='h-4 w-4 mr-2' />
                    )}
                    {t('cluster.start')}
                  </Button>
                  <Button
                    variant='outline'
                    onClick={() => setConfirmBatchOp('stop')}
                    disabled={selectedNodeIds.size === 0 || isOperating}
                  >
                    {isOperating ? (
                      <Loader2 className='h-4 w-4 mr-2 animate-spin' />
                    ) : (
                      <Square className='h-4 w-4 mr-2' />
                    )}
                    {t('cluster.stop')}
                  </Button>
                  <Button
                    variant='outline'
                    onClick={() => setConfirmBatchOp('restart')}
                    disabled={selectedNodeIds.size === 0 || isOperating}
                  >
                    {isOperating ? (
                      <Loader2 className='h-4 w-4 mr-2 animate-spin' />
                    ) : (
                      <RotateCcw className='h-4 w-4 mr-2' />
                    )}
                    {t('cluster.restart')}
                  </Button>
                </div>
                {selectedNodeIds.size === 0 && (
                  <p className='text-sm text-muted-foreground mt-2'>
                    {t('cluster.selectNodesToOperate')}
                  </p>
                )}
              </CardContent>
            </Card>

            {/* Nodes Table / 节点表格 */}
            <Card>
              <CardHeader className='flex flex-row items-center justify-between'>
                <CardTitle className='flex items-center gap-2'>
                  <Server className='h-5 w-5' />
                  {t('cluster.nodeList')}
                </CardTitle>
                <Button onClick={() => setIsAddNodeDialogOpen(true)}>
                  <Plus className='h-4 w-4 mr-2' />
                  {t('cluster.addNode')}
                </Button>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-12'>
                        <Checkbox
                          checked={
                            nodes.length > 0 &&
                            selectedNodeIds.size === nodes.length
                          }
                          onCheckedChange={toggleAllNodes}
                        />
                      </TableHead>
                      <TableHead>ID</TableHead>
                      <TableHead>{t('cluster.hostName')}</TableHead>
                      <TableHead>{t('cluster.hostIP')}</TableHead>
                      <TableHead>{t('cluster.nodeRole')}</TableHead>
                      <TableHead>{t('cluster.installDir')}</TableHead>
                      <TableHead>{t('cluster.nodeStatus')}</TableHead>
                      <TableHead>{t('cluster.processPID')}</TableHead>
                      <TableHead>{t('cluster.actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {nodes.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={9}
                          className='text-center py-8 text-muted-foreground'
                        >
                          {t('cluster.noNodes')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      nodes.map((node) => (
                        <TableRow key={node.id}>
                          <TableCell>
                            <Checkbox
                              checked={selectedNodeIds.has(node.id)}
                              onCheckedChange={() =>
                                toggleNodeSelection(node.id)
                              }
                            />
                          </TableCell>
                          <TableCell>{node.id}</TableCell>
                          <TableCell>{node.host_name || '-'}</TableCell>
                          <TableCell>{node.host_ip || '-'}</TableCell>
                          <TableCell>
                            <Badge variant='outline'>
                              {t(
                                `cluster.roles.${getRoleTranslationKey(node.role)}`,
                              )}
                            </Badge>
                          </TableCell>
                          <TableCell className='font-mono text-sm'>
                            {node.install_dir || '-'}
                          </TableCell>
                          <TableCell>
                            <Badge variant={getStatusBadgeVariant(node.status)}>
                              {t(`cluster.nodeStatuses.${node.status}`)}
                            </Badge>
                          </TableCell>
                          <TableCell>{node.process_pid || '-'}</TableCell>
                          <TableCell>
                            <div className='flex items-center gap-1'>
                              <Button
                                variant='ghost'
                                size='icon'
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
                                  <Play className='h-4 w-4 text-green-600' />
                                )}
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() =>
                                  setConfirmNodeOp({op: 'stop', node})
                                }
                                disabled={
                                  nodeOperating === node.id ||
                                  node.status !== NodeStatus.RUNNING
                                }
                                title={t('cluster.stop')}
                              >
                                <Square className='h-4 w-4 text-orange-600' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() =>
                                  setConfirmNodeOp({op: 'restart', node})
                                }
                                disabled={
                                  nodeOperating === node.id ||
                                  node.status !== NodeStatus.RUNNING
                                }
                                title={t('cluster.restart')}
                              >
                                <RotateCcw className='h-4 w-4 text-blue-600' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() => handleViewLogs(node)}
                                title={t('cluster.viewLogs')}
                              >
                                <FileText className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() => openEditNodeDialog(node)}
                                title={t('cluster.editNode')}
                              >
                                <Pencil className='h-4 w-4' />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() => setNodeToRemove(node)}
                                title={t('cluster.removeNode')}
                              >
                                <Trash2 className='h-4 w-4 text-destructive' />
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
            {/* Cluster Configs / 集群配置 */}
            <ClusterConfigs
              clusterId={clusterId}
              deploymentMode={cluster.deployment_mode}
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
                <CardDescription>
                  {t('stUpgrade.upgradeRecordsDescription')}
                </CardDescription>
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
                              {new Date(
                                upgradeTask.created_at,
                              ).toLocaleString()}
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
        onOpenChange={setIsAddNodeDialogOpen}
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

      {/* Remove Node Dialog / 移除节点对话框 */}
      <AlertDialog
        open={!!nodeToRemove}
        onOpenChange={() => setNodeToRemove(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cluster.removeNode')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('cluster.removeNodeConfirm', {
                name: nodeToRemove?.host_name || String(nodeToRemove?.id || ''),
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleRemoveNode}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

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
            <DialogTitle>{t('cluster.runtimeStorage.preview')}</DialogTitle>
            <DialogDescription className='break-all'>
              {runtimeStoragePreview?.path || '-'}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 text-sm md:grid-cols-4'>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.fileName')}
              </div>
              <div className='font-medium break-all'>
                {runtimeStoragePreview?.file_name || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.size')}
              </div>
              <div className='font-medium'>
                {formatBytes(runtimeStoragePreview?.size_bytes)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.encoding')}
              </div>
              <div className='font-medium'>
                {runtimeStoragePreview?.encoding || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.truncated')}
              </div>
              <div className='font-medium'>
                {runtimeStoragePreview?.truncated ? 'Yes' : 'No'}
              </div>
            </div>
          </div>
          <ScrollArea className='h-[50vh] rounded-md border p-3'>
            <pre className='text-xs whitespace-pre-wrap break-all'>
              {runtimeStoragePreview?.binary
                ? runtimeStoragePreview?.hex_preview || '-'
                : runtimeStoragePreview?.text_preview || '-'}
            </pre>
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <Dialog open={stxJavaProxyLogOpen} onOpenChange={setStxJavaProxyLogOpen}>
        <WorkbenchDialogContent className='sm:max-w-[1560px]'>
          <DialogHeader>
            <DialogTitle>
              {t('cluster.stxJavaProxy.viewRuntimeLog')}
            </DialogTitle>
            <DialogDescription className='break-all'>
              {stxJavaProxyLogResult?.log_path || stxJavaProxy?.log_path || '-'}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className='min-h-0 flex-1 rounded-md border p-3'>
            <pre className='text-xs whitespace-pre-wrap break-all'>
              {stxJavaProxyLogResult?.logs || '-'}
            </pre>
          </ScrollArea>
        </WorkbenchDialogContent>
      </Dialog>

      <Dialog
        open={checkpointInspectOpen}
        onOpenChange={setCheckpointInspectOpen}
      >
        <DialogContent className='max-w-5xl'>
          <DialogHeader>
            <DialogTitle>
              {t('cluster.runtimeStorage.deserializeCheckpoint')}
            </DialogTitle>
            <DialogDescription className='break-all'>
              {checkpointInspectResult?.path || '-'}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 text-sm md:grid-cols-4'>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.fileName')}
              </div>
              <div className='font-medium break-all'>
                {checkpointInspectResult?.file_name || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.size')}
              </div>
              <div className='font-medium'>
                {formatBytes(checkpointInspectResult?.size_bytes)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.encoding')}
              </div>
              <div className='font-medium'>
                {checkpointInspectResult?.encoding || '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('cluster.runtimeStorage.storageType')}
              </div>
              <div className='font-medium'>
                {checkpointInspectResult?.storage_type || '-'}
              </div>
            </div>
          </div>
          <ScrollArea className='h-[55vh] rounded-md border p-3'>
            <pre className='text-xs whitespace-pre-wrap break-all'>
              {JSON.stringify(
                {
                  pipeline_state: checkpointInspectResult?.pipeline_state,
                  completed_checkpoint:
                    checkpointInspectResult?.completed_checkpoint,
                  action_states: checkpointInspectResult?.action_states,
                  task_statistics: checkpointInspectResult?.task_statistics,
                },
                null,
                2,
              )}
            </pre>
          </ScrollArea>
        </DialogContent>
      </Dialog>

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
          <ScrollArea className='h-[55vh] rounded-md border p-3'>
            <pre className='text-xs whitespace-pre-wrap break-all'>
              {JSON.stringify(
                {
                  entries: imapInspectResult?.entries,
                },
                null,
                2,
              )}
            </pre>
          </ScrollArea>
        </DialogContent>
      </Dialog>

      {/* View Logs Dialog / 查看日志对话框 */}
      <AlertDialog open={isLogDialogOpen} onOpenChange={setIsLogDialogOpen}>
        <AlertDialogContent
          className='max-h-[90vh]'
          style={{maxWidth: '90vw', width: '1200px'}}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('cluster.viewLogs')} - {logNodeInfo?.host_name} (
              {t(
                `cluster.roles.${getRoleTranslationKey(logNodeInfo?.role || '')}`,
              )}
              )
            </AlertDialogTitle>
          </AlertDialogHeader>
          {/* Log query parameters / 日志查询参数 */}
          <div className='flex flex-wrap gap-3 items-end'>
            <div className='flex flex-col gap-1'>
              <label className='text-xs text-muted-foreground'>
                {t('cluster.logLines')}
              </label>
              <input
                type='number'
                value={logLines}
                onChange={(e) => setLogLines(Number(e.target.value) || 100)}
                onKeyDown={handleLogFilterKeyDown}
                className='w-20 h-8 px-2 text-sm border rounded-md bg-background text-foreground'
                min={1}
                max={10000}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <label className='text-xs text-muted-foreground'>
                {t('cluster.logMode')}
              </label>
              <select
                value={logMode}
                onChange={(e) => setLogMode(e.target.value)}
                className='h-8 px-2 text-sm border rounded-md bg-background text-foreground'
              >
                <option value='tail'>{t('cluster.logModeTail')}</option>
                <option value='head'>{t('cluster.logModeHead')}</option>
                <option value='all'>{t('cluster.logModeAll')}</option>
              </select>
            </div>
            <div className='flex flex-col gap-1'>
              <label className='text-xs text-muted-foreground'>
                {t('cluster.logFilter')}
              </label>
              <input
                type='text'
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                onKeyDown={handleLogFilterKeyDown}
                placeholder='grep pattern'
                className='w-32 h-8 px-2 text-sm border rounded-md bg-background text-foreground placeholder:text-muted-foreground'
              />
            </div>
            <div className='flex flex-col gap-1'>
              <label className='text-xs text-muted-foreground'>
                {t('cluster.logDate')}
              </label>
              <input
                type='text'
                value={logDate}
                onChange={(e) => setLogDate(e.target.value)}
                onKeyDown={handleLogFilterKeyDown}
                placeholder='2025-11-12-1'
                className='w-36 h-8 px-2 text-sm border rounded-md bg-background text-foreground placeholder:text-muted-foreground'
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              onClick={handleRefreshLogs}
              disabled={logLoading}
            >
              {logLoading ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <RefreshCw className='h-4 w-4' />
              )}
              <span className='ml-1'>{t('common.refresh')}</span>
            </Button>
          </div>
          <div className='overflow-auto h-[60vh] bg-muted rounded-md p-4'>
            {logLoading ? (
              <div className='flex items-center justify-center py-8'>
                <Loader2 className='h-6 w-6 animate-spin' />
              </div>
            ) : (
              <pre className='text-xs font-mono whitespace-pre-wrap'>
                {logContent}
              </pre>
            )}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.close')}</AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </motion.div>
  );
}
