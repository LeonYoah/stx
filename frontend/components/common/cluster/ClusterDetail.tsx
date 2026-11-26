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

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Button as ArcButton} from '@/components/arc/button/button';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Checkbox as ArcCheckbox} from '@/components/arc/checkbox/checkbox';
import {
  Dialog as ArcDialog,
  DialogContent as ArcDialogContent,
} from '@/components/arc/dialog/dialog';
import {DropdownMenu as ArcDropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {Input as ArcInput} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  Tabs as ArcTabs,
  TabsContent as ArcTabsContent,
  TabsList as ArcTabsList,
  TabsTrigger as ArcTabsTrigger,
} from '@/components/arc/tabs/tabs';
import {toast} from '@/lib/toast';
import {
  ArrowLeft,
  Play,
  Square,
  RotateCcw,
  Pencil,
  Trash2,
  RefreshCw,
  Activity,
  Bug,
  File,
  FileText,
  MonitorSmartphone,
  Database,
  Layers,
  Copy,
  MoreHorizontal,
  Cpu,
  SlidersHorizontal,
  HelpCircle,
} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  ConfirmDialog,
  DirtyCloseDialog,
  ExpandableTextPanel,
  NeutralChip,
  SoftChip,
  WorkspaceHeader,
  TableLoadingBar,
} from '@/components/common/layout';
import {ClusterStatusChip} from './ClusterStatusChip';
import type {SyncEventChannelStatus} from '@/lib/services/sync/types';
import {
  CLUSTER_DETAIL_TABS,
  getLegacyClusterRouteRewrite,
  resolveClusterDetailRoute,
  type OpsSubView,
  type ClusterDetailTab,
  type RuntimeSubView,
} from './detail/cluster-detail-tabs';
import {ClusterOverviewTab} from './detail/ClusterOverviewTab';
import {ClusterNodesTab} from './detail/ClusterNodesTab';
import {ClusterOpsTab} from './detail/ClusterOpsTab';
import {CheckpointInspectDialog} from '@/components/common/sync/CheckpointPanels';
import {Skeleton} from '@/components/ui/skeleton';
import services from '@/lib/services';
import {
  ClusterInfo,
  ClusterStatus,
  ClusterStatusInfo,
  K8sPodInfo,
  NodeInfo,
  OperationType,
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
import {EditClusterDialog} from './EditClusterDialog';
import {AddNodeDialog} from './AddNodeDialog';
import type {NodeJavaRuntimeView} from '@/components/common/java/NodeJavaHomeCell';
import {RemoveNodeDialog} from './RemoveNodeDialog';
import {ScaleOperationProgress} from './scale/ScaleOperationProgress';
import type {ScaleOperationInfo} from '@/lib/services/cluster/scale.types';
import {EditNodeDialog} from './EditNodeDialog';
import {ClusterPlugins} from './ClusterPlugins';
import {ClusterConfigs} from './ClusterConfigs';
import {deriveProxyContainerMemory} from '@/lib/k8s-proxy-memory';
import {EngineEventChannelCard} from './EngineEventChannelCard';
import {SeaTunnelObserveMenu} from './SeaTunnelObserveMenu';
import {RuntimeStorageEditor} from './RuntimeStorageEditor';
import {RuntimeStorageFileExplorer} from './RuntimeStorageFileExplorer';
import {ClusterActions} from './ClusterActions';
import {ClusterDetailSkeleton} from './ClusterDetailSkeleton';
import {K8sNodesPanel} from './K8sNodesPanel';
import {K8sUpgradeImageDialog} from './K8sUpgradeImageDialog';
import {ClusterNodeLogDialog} from './ClusterNodeLogDialog';
import {
  baselinePidsFromNodes,
  isRestartLifecycleOperation,
  LIFECYCLE_POLL_TIMEOUT_MS,
  lifecycleTargetForOperation,
} from './lifecycle-status-poll';
import {
  normalizeLoadClusterDataOptions,
  refreshNodeJavaRuntimes,
  shouldClearClusterLoading,
  shouldCommitClusterLoadResult,
  type LoadClusterDataOptions,
} from './node-java-runtime-refresh';
import {useLifecycleStatusPoll} from './useLifecycleStatusPoll';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';
import {useOnboardingOptional} from '@/components/common/onboarding/OnboardingContext';

interface ClusterDetailProps {
  clusterId: number;
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
  const pathname = usePathname();
  const onboarding = useOnboardingOptional();
  const bindTargetCluster = onboarding?.bindTargetCluster;
  const onboardingReady = onboarding?.ready;
  const onboardingFinished = onboarding?.state.finished;

  // Record the cluster the user actually opened (AC1 — no arbitrary first pick).
  // 记录用户实际打开的集群（AC1——禁止随意挑第一个凑进度）。
  useEffect(() => {
    if (!onboardingReady || onboardingFinished || !(clusterId > 0)) {
      return;
    }
    bindTargetCluster?.(clusterId);
  }, [
    clusterId,
    onboardingReady,
    onboardingFinished,
    bindTargetCluster,
  ]);

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
  // 告警横幅关闭记忆：按集群持久化「已关闭的告警数」，告警数变化时重新展示
  // Banner dismissal is persisted per cluster by alert count; it resurfaces when the count changes
  const [dismissedAlertCount, setDismissedAlertCount] = useState<
    number | null
  >(null);
  const [runtimeStorage, setRuntimeStorage] =
    useState<RuntimeStorageDetails | null>(null);
  const [stxJavaProxy, setStxJavaProxy] = useState<StxJavaProxyStatus | null>(
    null,
  );
  const [eventChannel, setEventChannel] =
    useState<SyncEventChannelStatus | null>(null);
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
  // 代理状态轮询定时器：串行链式调度 + 卸载/恢复健康/重复提交立即清理，
  // 不再一次甩 12 个无人回收的 setTimeout。
  // Proxy status polling timer: serial chained scheduling with immediate cleanup on
  // unmount, recovery, or resubmission — no more 12 fire-and-forget setTimeouts.
  const proxyPollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clearProxyPollTimer = useCallback(() => {
    if (proxyPollTimerRef.current !== null) {
      clearTimeout(proxyPollTimerRef.current);
      proxyPollTimerRef.current = null;
    }
  }, []);
  useEffect(() => clearProxyPollTimer, [clearProxyPollTimer]);
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
  const searchParams = useSearchParams();
  // 守护策略数值表单脏态与待跳转目标（脏态切换 Tab/子视角走 DirtyCloseDialog）
  // Guard policy numeric dirty state & pending navigation target intercepted by DirtyCloseDialog
  const [guardDirty, setGuardDirty] = useState(false);
  const [pendingRoute, setPendingRoute] = useState<{
    tab: ClusterDetailTab;
    view?: string | null;
  } | null>(null);

  const rawTabParam = searchParams?.get('tab') ?? null;
  const rawViewParam = searchParams?.get('view') ?? null;

  const updateRouteQuery = useCallback(
    (tab: ClusterDetailTab, view?: string | null) => {
      const next = new URLSearchParams(searchParams?.toString() ?? '');
      if (tab === 'overview') {
        next.delete('tab');
      } else {
        next.set('tab', tab);
      }
      if (view) {
        next.set('view', view);
      } else {
        next.delete('view');
      }
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  const runtimeConfig = useMemo(
    () => extractClusterRuntimeConfig(cluster?.config),
    [cluster?.config],
  );
  const webUINode = useMemo(
    () => pickWebUINode(nodes, cluster?.version, runtimeConfig.enableHTTP),
    [cluster?.version, nodes, runtimeConfig.enableHTTP],
  );
  // 提前计算基础设施类型：opsView 归一化与 Pod 拉取都依赖它，且需在 early return 前就绪。
  // Infra type is computed early: both opsView normalization and pod loading depend on
  // it and it must be ready before the early returns.
  const clusterIsK8s = Boolean(cluster?.infra_type && cluster.infra_type !== 'bare_metal');
  const resolvedRoute = useMemo(
    () =>
      resolveClusterDetailRoute(rawTabParam, rawViewParam, Boolean(webUINode)),
    [rawTabParam, rawViewParam, webUINode],
  );
  const activeTab = resolvedRoute.tab;
  const runtimeView = resolvedRoute.runtimeView;
  // K8s 无 guard 子视图：深链或默认值落到 guard 时无感归一化到 diagnostics。
  // Kubernetes has no guard sub-view: deep links or defaults landing on guard
  // transparently normalize to diagnostics.
  const opsView: OpsSubView =
    clusterIsK8s && resolvedRoute.opsView === 'guard'
      ? 'diagnostics'
      : resolvedRoute.opsView;

  // 旧 ?tab=storage|proxy|webui|monitoring|diagnostics|upgrades 深链无感归一化。
  // ?tab=webui 须等集群/节点首次加载完成，避免未知可用性被写成 checkpoint。
  // Silently normalize legacy tabs; defer ?tab=webui until cluster/nodes first load finishes.
  useEffect(() => {
    const rewrite = getLegacyClusterRouteRewrite(rawTabParam, resolvedRoute, {
      clusterDataReady: !loading,
    });
    if (!rewrite) {
      return;
    }
    updateRouteQuery(rewrite.tab, rewrite.view);
  }, [loading, rawTabParam, resolvedRoute, updateRouteQuery]);

  const requestRouteChange = useCallback(
    (tab: ClusterDetailTab, view?: string | null) => {
      const leavingGuard =
        activeTab === 'ops' &&
        opsView === 'guard' &&
        guardDirty &&
        (tab !== 'ops' || (view ?? 'guard') !== 'guard');
      if (leavingGuard) {
        setPendingRoute({tab, view});
        return;
      }
      updateRouteQuery(tab, view);
    },
    [activeTab, guardDirty, opsView, updateRouteQuery],
  );

  const commitPendingRoute = useCallback(() => {
    const target = pendingRoute;
    setPendingRoute(null);
    setGuardDirty(false);
    if (target) {
      updateRouteQuery(target.tab, target.view);
    }
  }, [pendingRoute, updateRouteQuery]);

  // Node selection state / 节点选择状态
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<number>>(
    new Set(),
  );
  const [nodeOperating, setNodeOperating] = useState<number | null>(null);
  /** Per-node JDK version fetch (AC7); never blocks the page / 每节点 JDK 版本拉取（AC7），不阻塞页面 */
  const [nodeJavaRuntimes, setNodeJavaRuntimes] = useState<
    Record<number, NodeJavaRuntimeView>
  >({});
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

  // 平滑刷新状态 / Smooth refresh state
  const [refreshing, setRefreshing] = useState(false);
  // 日志输出模式切换状态 / Job log mode switching state
  const [logModeSwitching, setLogModeSwitching] = useState(false);


  const nodesRef = useRef<NodeInfo[]>([]);
  const nodeJavaRuntimesRef = useRef<Record<number, NodeJavaRuntimeView>>({});
  // 上次成功拉 JDK runtime 时对应的进程 PID；增量比较必须用它，不能用已被轮询更新的 nodesRef。
  // PID recorded when JDK runtime was last fetched; incremental compare must use this, not nodesRef (updated every poll).
  const runtimePidRef = useRef<Record<number, number>>({});
  const clusterLoadSeqRef = useRef(0);
  const clusterIdRef = useRef(clusterId);
  clusterIdRef.current = clusterId;

  /**
   * Fetch effective JDK version for nodes (AC7). Failures only mark version unavailable.
   * mode=full 全量；incremental 用 runtimePidRef 判断 PID 是否变化，变化时先保留旧 ok。
   * mode=full refreshes all; incremental uses runtimePidRef and keeps old ok while PID changes.
   */
  const loadNodeJavaRuntimes = useCallback(
    async (
      nodeList: NodeInfo[],
      mode: 'full' | 'incremental' = 'full',
    ) => {
      const result = await refreshNodeJavaRuntimes({
        nodeList,
        mode,
        previousRuntimes: nodeJavaRuntimesRef.current,
        runtimePidByNodeId: runtimePidRef.current,
        fetchRuntime: (nodeId) =>
          services.cluster.getNodeJavaRuntime(clusterId, nodeId),
      });
      nodeJavaRuntimesRef.current = result.views;
      runtimePidRef.current = result.runtimePidByNodeId;
      setNodeJavaRuntimes(result.views);
    },
    [clusterId],
  );

  /**
   * Load cluster data
   * 加载集群数据
   * @returns 成功时返回主状态快照，供启停/重启后有界轮询判断是否达标。
   *          On success returns a main-status snapshot for bounded post-lifecycle polling.
   */
  const loadClusterData = useCallback(
    async (options?: LoadClusterDataOptions) => {
      const {silent, notifyError, javaRuntimeMode} =
        normalizeLoadClusterDataOptions(options);
      const seq = ++clusterLoadSeqRef.current;
      if (!silent) {
        setLoading(true);
      }
      try {
        const [clusterResult, nodesResult, statusResult] = await Promise.all([
          services.cluster.getClusterSafe(clusterId),
          services.cluster.getNodesSafe(clusterId),
          services.cluster.getClusterStatusSafe(clusterId),
        ]);
        if (!shouldCommitClusterLoadResult(seq, clusterLoadSeqRef.current)) {
          return null;
        }

        let nextCluster: ClusterInfo | null = null;
        let nextNodes: NodeInfo[] | null = null;

        if (clusterResult.success && clusterResult.data) {
          nextCluster = clusterResult.data;
          setCluster(clusterResult.data);
        } else if (notifyError) {
          toast.error(clusterResult.error || t('cluster.loadError'));
        }

        if (nodesResult.success && nodesResult.data) {
          nextNodes = nodesResult.data;
          setNodes(nodesResult.data);
          nodesRef.current = nodesResult.data;
          if (javaRuntimeMode !== 'skip') {
            void loadNodeJavaRuntimes(nodesResult.data, javaRuntimeMode);
          }
        }

        if (statusResult.success && statusResult.data) {
          setClusterStatus(statusResult.data);
        }

        if (nextCluster && nextNodes) {
          return {
            clusterStatus: nextCluster.status,
            nodes: nextNodes,
          };
        }
        return null;
      } catch (error) {
        if (!shouldCommitClusterLoadResult(seq, clusterLoadSeqRef.current)) {
          return null;
        }
        if (notifyError) {
          toast.error(
            error instanceof Error ? error.message : t('cluster.loadError'),
          );
        }
        return null;
      } finally {
        // 序号只挡数据写入；非静默加载必须复位 loading，否则深链 rewrite 一直等 clusterDataReady。
        // Seq only gates writes; non-silent loads must clear loading or deep-link rewrite stalls.
        if (shouldClearClusterLoading(silent)) {
          setLoading(false);
        }
      }
    },
    [clusterId, t, loadNodeJavaRuntimes],
  );

  // 重启受理后待轮询达标再 toast 成功；超时则 warning，避免把受理当成成功。
  // After restart accept, toast success only when poll is satisfied; timeout → warning (accept ≠ success).
  const pendingRestartSuccessToastRef = useRef<
    'cluster' | 'node' | 'batch' | null
  >(null);

  const {startLifecyclePoll, stopLifecyclePoll} = useLifecycleStatusPoll({
    loadSnapshot: () =>
      loadClusterData({
        silent: true,
        notifyError: false,
        // 轮询跳过 JDK，避免每 2.5s 整列 loading 闪烁；达标后再补一次。
        // Skip JDK during poll to avoid 2.5s column flicker; refresh once when satisfied.
        javaRuntimeMode: 'skip',
      }),
    onSatisfied: () => {
      void loadNodeJavaRuntimes(nodesRef.current, 'incremental');
      const pending = pendingRestartSuccessToastRef.current;
      if (pending) {
        pendingRestartSuccessToastRef.current = null;
        const successKey =
          pending === 'node'
            ? 'cluster.nodeRestartSuccess'
            : pending === 'batch'
              ? 'cluster.batchRestartSuccess'
              : 'cluster.restartSuccess';
        toast.success(t(successKey));
      }
    },
    onTimeout: () => {
      if (pendingRestartSuccessToastRef.current) {
        pendingRestartSuccessToastRef.current = null;
        toast.warning(
          t('cluster.restartPollTimeout', {
            seconds: Math.round(LIFECYCLE_POLL_TIMEOUT_MS / 1000),
          }),
        );
      }
    },
  });

  // 切换集群时停止上一集群的轮询，避免串扰。
  // Stop the previous cluster's poll when switching clusters.
  useEffect(() => {
    pendingRestartSuccessToastRef.current = null;
    stopLifecyclePoll();
  }, [clusterId, stopLifecyclePoll]);

  // 启停/重启后递增，驱动节点页 ProcessEventList 有界静默轮询（与主状态轮询同窗口）。
  // Bumped after lifecycle ops so Nodes-tab ProcessEventList starts a bounded silent poll (same window as status).
  const [lifecyclePollEpoch, setLifecyclePollEpoch] = useState(0);
  const [k8sUpgradeOpen, setK8sUpgradeOpen] = useState(false);
  const [k8sPods, setK8sPods] = useState<K8sPodInfo[]>([]);

  const refreshMainStatusAfterLifecycle = useCallback(
    async (
      operation: OperationType | 'start' | 'stop' | 'restart',
      scopeNodeIds?: number[],
    ) => {
      // await 期间若已切到其它集群，丢弃本轮 baseline/轮询，避免套到新集群。
      // If the user switches clusters during await, drop this baseline/poll so it cannot apply to the new one.
      const startedForClusterId = clusterIdRef.current;
      // 重启前冻结基准 PID；若等 load 后再取，可能已是旧 PID 残留，轮询会立刻伪造成功。
      // Freeze baseline PIDs before refresh; reading after load can keep a stale PID and fake early success.
      const baselinePids = isRestartLifecycleOperation(operation)
        ? baselinePidsFromNodes(nodesRef.current, scopeNodeIds)
        : undefined;
      await loadClusterData({
        silent: true,
        notifyError: false,
        javaRuntimeMode: 'skip',
      });
      if (startedForClusterId !== clusterIdRef.current) {
        return;
      }
      // 先递增 epoch：节点页挂载时立即静默拉事件；未挂载则切回节点页首屏加载即可看到新事件。
      // Bump epoch first: silent event fetch while Nodes is mounted; otherwise the next mount load shows them.
      setLifecyclePollEpoch((prev) => prev + 1);
      startLifecyclePoll(lifecycleTargetForOperation(operation), {
        scopeNodeIds,
        baselinePids,
      });
    },
    [loadClusterData, startLifecyclePoll],
  );

  const refreshMainStatusFromEvents = useCallback(() => {
    void loadClusterData({
      silent: true,
      notifyError: false,
      // 事件联动用增量：保留 ok，仅新节点/PID 变化才拉，避免 spinner 闪。
      // Event linkage uses incremental refresh: keep ok rows, refetch only new/PID-changed nodes.
      javaRuntimeMode: 'incremental',
    });
  }, [loadClusterData]);

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
    void loadClusterData({
      silent: true,
      notifyError: true,
      javaRuntimeMode: 'incremental',
    });
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

  const loadEventChannel = useCallback(async () => {
    try {
      const data = await services.sync.getEventChannel(clusterId);
      setEventChannel(data);
    } catch {
      // 概览速览失败不阻塞主视图 / Non-blocking overview snapshot failure
      setEventChannel(null);
    }
  }, [clusterId]);

  // localStorage 键按集群隔离；挂载后恢复已关闭的告警数
  // Storage key is scoped per cluster; restore the dismissed count after mount
  const alertsBannerStorageKey = `stx:cluster-alerts-banner:${clusterId}`;
  useEffect(() => {
    setDismissedAlertCount(
      Number(window.localStorage.getItem(alertsBannerStorageKey) ?? -1),
    );
  }, [alertsBannerStorageKey]);

  // 关闭横幅并记住当前告警数 / Dismiss the banner and remember the current alert count
  const dismissAlertsBanner = useCallback(() => {
    const count = monitoringOverview?.stats.active_alerts_1h ?? 0;
    setDismissedAlertCount(count);
    window.localStorage.setItem(alertsBannerStorageKey, String(count));
  }, [monitoringOverview, alertsBannerStorageKey]);

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
          setRuntimeStorageListing((prev) => ({...prev, [kind]: cached}));
          return;
        }
      }

      setRuntimeStorageListingLoading(kind);
      try {
        if (path !== undefined) {
          setRuntimeStorageBrowsePath((prev) => ({...prev, [kind]: path}));
        }
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
        return null;
      }
      setStxJavaProxy(result.data);
      return result.data;
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
      // 触发滚动重启时新 Pod 需数十秒就绪，期间状态接口打到旧 Pod 会展示过期堆内存；
      // 间隔轮询直到新实例上报，避免详情页长期停留在旧值。
      // During a rolling restart the new pod takes tens of seconds to become ready and
      // the status endpoint keeps hitting the old pod, so poll until the fresh instance
      // reports instead of leaving the detail page pinned to stale heap values.
      if (stxJavaProxyRestartOnSave) {
        clearProxyPollTimer();
        let attempt = 0;
        const chain = async () => {
          attempt += 1;
          const status = await loadStxJavaProxyStatus();
          // 恢复健康（实例-ready 且堆内存已刷新）或超过 12 次即停止链式轮询。
          // The chained poll stops once health recovers (instance ready with refreshed
          // heap) or after 12 attempts.
          const healthy = Boolean(
            status &&
            typeof status === 'object' &&
            ((status as {healthy?: boolean}).healthy || (status as {status?: string}).status === 'running'),
          );
          if (!healthy && attempt < 12) {
            proxyPollTimerRef.current = setTimeout(() => {
              void chain();
            }, 5000);
          } else {
            proxyPollTimerRef.current = null;
          }
        };
        proxyPollTimerRef.current = setTimeout(() => {
          void chain();
        }, 5000);
      }
    } finally {
      setStxJavaProxySaving(false);
    }
  }, [
    clusterId,
    stxJavaProxyJvmOptsInput,
    stxJavaProxyRestartOnSave,
    t,
    loadStxJavaProxyStatus,
    clearProxyPollTimer,
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
    void loadEventChannel();
  }, [
    loadClusterData,
    loadDiagnosticsSummary,
    loadMonitoringOverview,
    loadRuntimeStorage,
    loadStxJavaProxyStatus,
    loadEventChannel,
  ]);

  useEffect(() => {
    void loadUpgradeTasks(upgradeTasksPage, upgradeTasksPageSize);
  }, [loadUpgradeTasks, upgradeTasksPage, upgradeTasksPageSize]);

  useEffect(() => {
    if (
      activeTab === 'runtime' &&
      (runtimeView === 'checkpoint' || runtimeView === 'imap')
    ) {
      void loadRuntimeStorage();
      return;
    }
    if (activeTab === 'runtime' && runtimeView === 'proxy') {
      void loadStxJavaProxyStatus();
    }
  }, [activeTab, runtimeView, loadRuntimeStorage, loadStxJavaProxyStatus]);

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
        loadClusterData({
          silent: true,
          notifyError: true,
          javaRuntimeMode: 'full',
        }),
        loadDiagnosticsSummary(),
        loadMonitoringOverview(),
        loadRuntimeStorage(),
        loadStxJavaProxyStatus(),
        loadEventChannel(),
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
    loadEventChannel,
    loadUpgradeTasks,
    upgradeTasksPage,
    upgradeTasksPageSize,
  ]);

  // 配置保存/同步后反向刷新存储可视化；稳定引用避免配置 Tab 死循环
  // After config save/sync, refresh storage views; stable ref avoids configs-tab fetch loops
  const handleConfigChanged = useCallback(() => {
    void loadRuntimeStorage();
    void loadEventChannel();
  }, [loadRuntimeStorage, loadEventChannel]);

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
      await loadClusterData({
        silent: true,
        notifyError: true,
        javaRuntimeMode: 'full',
      });
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
  const toggleAllNodes = (targetNodes: NodeInfo[], checked: boolean) => {
    setSelectedNodeIds((prev) => {
      const next = new Set(prev);
      if (checked) {
        targetNodes.forEach((n) => next.add(n.id));
      } else {
        targetNodes.forEach((n) => next.delete(n.id));
      }
      return next;
    });
  };

  /**
   * Handle single node start
   * 处理单个节点启动
   */
  const handleNodeStart = async (node: NodeInfo) => {
    setNodeOperating(node.id);
    // 与重启一致的受理语义：先提示受理并起轮询，不阻塞确认弹窗关闭。
    // Same accept-async semantics as restart: toast acceptance and start polling up front so the confirm dialog can close immediately.
    toast.info(t('cluster.startAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.START, [node.id]);
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
    toast.info(t('cluster.stopAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.STOP, [node.id]);
    try {
      const result = await services.cluster.stopNodeSafe(clusterId, node.id);
      if (result.success) {
        toast.success(t('cluster.nodeStopSuccess'));
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
    // t4：POST /restart 可阻塞 ~30s 后 HTTP 500，但 requested/新 PID 已异步落库。
    // 确认后立刻开轮询；不把 refresh 挂在 POST 成功之后，失败/超时也不取消。
    // t4: POST /restart can block ~30s then HTTP 500 while events/new PID land async.
    // Start poll on confirm; never gate refresh on POST success, and never cancel on failure.
    pendingRestartSuccessToastRef.current = 'node';
    toast.info(t('cluster.restartAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.RESTART, [node.id]);
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
        // 受理已提示；成功等轮询达标。自动重启仅补充托管说明。
        // Accepted already toasted; success waits for poll. Auto-restart only adds a managed-start note.
        if (isAutoRestart) {
          toast.info(t('cluster.nodeRestartSuccessAutoRestart'));
        }
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
    const scopeNodeIds = Array.from(selectedNodeIds);
    toast.info(t('cluster.startAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.START, scopeNodeIds);
    try {
      let hasAutoRestart = false;
      for (const nodeId of scopeNodeIds) {
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
    const scopeNodeIds = Array.from(selectedNodeIds);
    toast.info(t('cluster.stopAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.STOP, scopeNodeIds);
    try {
      for (const nodeId of scopeNodeIds) {
        await services.cluster.stopNodeSafe(clusterId, nodeId);
      }
      toast.success(t('cluster.batchStopSuccess'));
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
    const scopeNodeIds = Array.from(selectedNodeIds);
    // 批量重启同样先开轮询，避免被任一节点 POST 阻塞/500 卡住停留页。
    // Batch restart also starts poll first so one blocked/500 POST cannot freeze stay-on-page.
    pendingRestartSuccessToastRef.current = 'batch';
    toast.info(t('cluster.restartAccepted'));
    void refreshMainStatusAfterLifecycle(OperationType.RESTART, scopeNodeIds);
    try {
      let hasAutoRestart = false;
      let successCount = 0;
      let failedCount = 0;
      for (const nodeId of scopeNodeIds) {
        const result = await services.cluster.restartNodeSafe(
          clusterId,
          nodeId,
        );
        if (!result.success) {
          failedCount += 1;
        } else {
          successCount += 1;
        }
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
      if (failedCount > 0) {
        // 部分失败：受理结果用批量文案；取消「等待成功」toast，避免误报全成功。
        // Partial failure: use batch copy for accept outcomes; clear pending success toast.
        pendingRestartSuccessToastRef.current = null;
        toast.warning(
          t('cluster.batchRestartPartial', {
            success: successCount,
            failed: failedCount,
          }),
        );
      } else if (hasAutoRestart) {
        toast.info(t('cluster.batchRestartSuccessAutoRestart'));
      }
      // 全成功时成功 toast 留给轮询 onSatisfied。
      // On full accept success, leave the success toast to poll onSatisfied.
    } finally {
      setIsOperating(false);
    }
  };

  // hooks 必须全部位于 early return 之前，故 Pod 拉取放在这里而不是渲染区。
  // All hooks must precede the early returns, so pod loading lives here rather than
  // in the render section.
  useEffect(() => {
    if (!clusterIsK8s) {
      setK8sPods([]);
      return;
    }
    services.cluster
      .getK8sPods(clusterId)
      .then(setK8sPods)
      .catch(() => setK8sPods([]));
  }, [clusterId, clusterIsK8s, lifecyclePollEpoch]);

  if (loading && !cluster) {
    return <ClusterDetailSkeleton />;
  }

  if (!cluster) {
    return (
      <div className='arc-compact flex flex-col items-center gap-4 py-12 text-center'>
        <p className='text-sm text-muted-foreground'>{t('cluster.notFound')}</p>
        <ArcButton
          type='button'
          variant='secondary'
          size='sm'
          onClick={() => router.push('/clusters')}
        >
          <ArrowLeft className='size-3.5' aria-hidden />
          {t('cluster.backToList')}
        </ArcButton>
      </div>
    );
  }

  const isK8sCluster = Boolean(cluster?.infra_type && cluster.infra_type !== 'bare_metal');
  // 配置查找极廉价，不用 useMemo：本组件存在 early return，hooks 位置受限。
  // The config lookup is trivially cheap, so no useMemo: this component has early
  // returns that constrain hook placement.
  const k8sCfg = ((cluster?.config as Record<string, unknown> | undefined)?.['kubernetes'] || {}) as Record<string, unknown>;

  // K8s 集群删除即资源生命周期（解绑/卸载），后端允许运行态删除，前端对齐。
  // For K8s clusters deletion IS the resource lifecycle; the backend permits deleting
  // running clusters and the frontend matches it.
  const canDelete = isK8sCluster
    ? cluster.status !== ClusterStatus.DEPLOYING
    : cluster.status !== ClusterStatus.RUNNING &&
      cluster.status !== ClusterStatus.DEPLOYING;
  // WebUI 可用性门控：物理机依赖引擎节点 API 端口；K8s 由 master_http_port 决定，
  // iframe 统一走 /webui/ 反代（后端 proxyK8sWebUI 已实现 K8s 转发）。
  // WebUI availability: bare metal relies on the engine node API port; K8s depends on
  // master_http_port. The iframe always proxies through /webui/ (backend proxyK8sWebUI
  // already forwards K8s traffic).
  const hasWebUI = isK8sCluster
    ? Number(k8sCfg?.master_http_port) > 0
    : Boolean(webUINode);
  const webUIProxyURL = hasWebUI ? `/api/v1/clusters/${clusterId}/webui/` : '';
  const webUIMasterAddress = isK8sCluster
    ? `${(k8sCfg?.namespace as string | undefined) ?? '-'}/${((k8sCfg?.master_service_name || k8sCfg?.cluster_service_name) as string | undefined) ?? 'master'}:${(k8sCfg?.master_http_port as number | undefined) ?? 0}`
    : webUINode
      ? `${webUINode.host_ip || webUINode.host_name || '-'}:${webUINode.api_port}`
      : '';

  const renderRuntimeStorageSpec = (
    spec: RuntimeStorageSpec | undefined,
    label: string,
  ) => {
    if (!spec) {
      return (
        <div className='border-t-[0.5px] border-border/80 py-6 text-center text-xs text-muted-foreground'>
          {t('cluster.runtimeStorage.unavailable')}
        </div>
      );
    }
    const kind = spec.kind as 'checkpoint' | 'imap';
    const listing = runtimeStorageListing[kind];
    return (
      <div className='space-y-4 border-t-[0.5px] border-border/80 pt-5 text-sm'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-0.5'>
            <h4 className='text-sm font-semibold text-foreground'>{label}</h4>
            <p className='text-xs text-muted-foreground'>
              {spec.enabled
                ? t('cluster.runtimeStorage.mode', {
                    type: spec.storage_type || '-',
                  })
                : t('cluster.runtimeStorage.disabled')}
            </p>
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <ArcButton
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => void handleValidateRuntimeStorage(kind)}
              loading={runtimeStorageValidationLoading === kind}
            >
              <Activity className='size-3.5' aria-hidden />
              {t('cluster.runtimeStorage.validate')}
            </ArcButton>
            {spec.kind === 'imap' && spec.cleanup_supported ? (
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => setImapCleanupOpen(true)}
                loading={imapCleanupRunning}
              >
                <Trash2 className='size-3.5' aria-hidden />
                {t('cluster.runtimeStorage.cleanupImap')}
              </ArcButton>
            ) : null}
          </div>
        </div>

        <dl className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs xl:grid-cols-4'>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.runtimeStorage.path')}
            </dt>
            <dd
              className='truncate font-mono font-medium text-foreground'
              title={spec.namespace || '-'}
            >
              {spec.namespace || '-'}
            </dd>
          </div>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.runtimeStorage.endpoint')}
            </dt>
            <dd
              className='truncate font-mono font-medium text-foreground'
              title={spec.endpoint || '-'}
            >
              {spec.endpoint || '-'}
            </dd>
          </div>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.runtimeStorage.bucket')}
            </dt>
            <dd
              className='truncate font-mono font-medium text-foreground'
              title={spec.bucket || '-'}
            >
              {spec.bucket || '-'}
            </dd>
          </div>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.runtimeStorage.size')}
            </dt>
            <dd className='font-mono font-medium tabular-nums'>
              {spec.size_available
                ? formatBytes(spec.total_size_bytes)
                : t('cluster.runtimeStorage.remoteSizeUnavailable')}
            </dd>
          </div>
        </dl>

        {spec.warning ? (
          <ArcAlert tone='warning' title={spec.warning} />
        ) : null}

        {runtimeStorageValidation[kind] ? (
          <div className='space-y-2 border-t-[0.5px] border-border/80 pt-4'>
            <div className='flex items-center justify-between gap-2'>
              <span className='text-xs font-semibold text-foreground'>
                {t('cluster.runtimeStorage.validateResult')}
              </span>
              <SoftChip
                tone={
                  runtimeStorageValidation[kind]?.success ? 'emerald' : 'rose'
                }
              >
                {runtimeStorageValidation[kind]?.success
                  ? t('cluster.runtimeStorage.passed')
                  : t('cluster.runtimeStorage.failed')}
              </SoftChip>
            </div>
            {runtimeStorageValidation[kind]?.warning ? (
              <p className='text-xs text-muted-foreground'>
                {runtimeStorageValidation[kind]?.warning}
              </p>
            ) : null}
            <div className='divide-y-[0.5px] divide-border/70'>
              {runtimeStorageValidation[kind]?.hosts?.map((host) => (
                <div
                  key={`${spec.kind}-validate-${host.host_id}`}
                  className='flex items-start justify-between gap-3 py-2 text-xs'
                >
                  <div className='min-w-0'>
                    <div className='font-medium text-foreground'>
                      {host.host_name || host.host_id}
                    </div>
                    <div
                      className='truncate text-muted-foreground'
                      title={host.message}
                    >
                      {host.message}
                    </div>
                  </div>
                  <SoftChip tone={host.success ? 'emerald' : 'rose'}>
                    {host.success
                      ? t('cluster.runtimeStorage.passed')
                      : t('cluster.runtimeStorage.failed')}
                  </SoftChip>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        {spec.nodes && spec.nodes.length > 0 ? (
          <div className='divide-y-[0.5px] divide-border/70 border-t-[0.5px] border-border/80 pt-2'>
            {spec.nodes.map((node) => (
              <div
                key={`${spec.kind}-${node.host_id}-${node.node_id}`}
                className='flex items-center justify-between gap-4 py-2 text-xs'
              >
                <div className='min-w-0 flex-1'>
                  <div className='font-medium text-foreground'>
                    {node.host_name}
                  </div>
                  <div
                    className='truncate font-mono text-muted-foreground'
                    title={node.path || '-'}
                  >
                    {node.path || '-'}
                  </div>
                </div>
                <div className='shrink-0 text-right tabular-nums'>
                  <div className='font-mono font-medium'>
                    {formatBytes(node.size_bytes)}
                  </div>
                  <div className='text-muted-foreground'>
                    {node.message ||
                      (node.exists
                        ? t('cluster.runtimeStorage.pathExists')
                        : t('cluster.runtimeStorage.pathMissing'))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : null}

        {/* 类似电脑资源管理器的文件与目录浏览器；K8s 容器内 LOCAL_FILE 控制面不可达，隐藏 */}
        {/* Desktop-like file explorer; hidden for Kubernetes LOCAL_FILE which lives inside pods */}
        {!(clusterIsK8s && !spec.external) ? (
        <div className='border-t-[0.5px] border-border/80 pt-4'>
          <RuntimeStorageFileExplorer
            flat
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
            onPreviewFile={(path) =>
              void handlePreviewRuntimeStorage(kind, path)
            }
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
        </div> ) : null}
      </div>
    );
  };

  // K8s 集群判定与节点面板一致：任何非 bare_metal infra_type 都走 K8s 视图。
  // K8s detection mirrors the nodes panel: any non-bare_metal infra_type uses K8s views.

  const runtimeViewOptions: Array<{value: RuntimeSubView; label: string}> = [
    {value: 'checkpoint', label: t('cluster.list.runtimeViews.checkpoint')},
    {value: 'imap', label: t('cluster.list.runtimeViews.imap')},
    {value: 'proxy', label: t('cluster.list.runtimeViews.proxy')},
    ...(hasWebUI
      ? [{value: 'webui' as const, label: t('cluster.list.runtimeViews.webui')}]
      : []),
  ];

  const headerMoreItems = [
    {
      label: t('cluster.openDiagnostics'),
      icon: <Bug className='size-3.5' aria-hidden />,
      onSelect: () =>
        router.push(
          `/diagnostics?tab=errors&cluster_id=${clusterId}&source=cluster-detail`,
        ),
    },
    {
      label: t('stUpgrade.entry'),
      icon: <Activity className='size-3.5' aria-hidden />,
      onSelect: () => {
        // K8s 集群升级即换镜像 Tag（不可变镜像），无二进制分发升级向导。
        // K8s upgrades mean swapping the image tag (immutable image); there is no
        // binary-distribution upgrade wizard.
        if (isK8sCluster) {
          setK8sUpgradeOpen(true);
        } else {
          router.push(`/clusters/${clusterId}/upgrade/prepare`);
        }
      },
    },
    {
      label: t('common.edit'),
      icon: <Pencil className='size-3.5' aria-hidden />,
      onSelect: () => setIsEditDialogOpen(true),
    },
    {
      label: t('common.delete'),
      icon: <Trash2 className='size-3.5' aria-hidden />,
      disabled: !canDelete,
      destructive: true,
      separatorBefore: true,
      onSelect: () => setIsDeleteDialogOpen(true),
    },
  ];

  return (
    <div className='arc-compact space-y-4' data-testid='cluster-detail-page'>
      {/* 顶部平滑加载进度条 / Top smooth loading bar */}
      <TableLoadingBar loading={refreshing} />

      {/* 标准化页面头部（精简徽章与操作，去除顶部滥用的 StatPillsBar） */}
      {/* Standardized WorkspaceHeader (concise badges & actions, no multi-purpose StatPillsBar) */}
      <WorkspaceHeader
        breadcrumbs={[
          {label: t('cluster.title'), href: '/clusters'},
          {label: cluster.name},
        ]}
        icon={<Layers />}
        title={cluster.name}
        badge={
          <div className='flex flex-wrap items-center gap-1.5'>
            <ClusterStatusChip
              cluster={{
                status: cluster.status,
                online_nodes:
                  clusterStatus?.online_nodes ?? cluster.online_nodes,
                health_status:
                  clusterStatus?.health_status ?? cluster.health_status,
              }}
            />
            {cluster.version ? (
              <NeutralChip className='font-mono tabular-nums'>
                v{cluster.version}
              </NeutralChip>
            ) : null}
          </div>
        }
        actions={
          <div className='flex flex-wrap items-center gap-1.5'>
            <ClusterActions
              cluster={cluster}
              onRestartConfirmed={() => {
                pendingRestartSuccessToastRef.current = 'cluster';
                void refreshMainStatusAfterLifecycle(OperationType.RESTART);
              }}
              onOperationComplete={(operation) => {
                // 重启已在 onRestartConfirmed 开过轮询；此处勿再 refresh，以免重采 baseline。
                // Restart already started poll in onRestartConfirmed; skip refresh here to keep baseline.
                if (operation === OperationType.RESTART) {
                  return;
                }
                void refreshMainStatusAfterLifecycle(operation);
              }}
            />
            <ArcButton
              type='button'
              variant='secondary'
              size='sm'
              iconOnly
              onClick={() => void handleRefresh()}
              disabled={refreshing}
              aria-label={t('common.refresh')}
              title={t('common.refresh')}
            >
              <RefreshCw
                className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`}
                aria-hidden
              />
            </ArcButton>
            <ArcDropdownMenu
              items={headerMoreItems}
              trigger={
                <ArcButton
                  type='button'
                  variant='secondary'
                  size='sm'
                  iconOnly
                  aria-label={t('common.moreActions')}
                  title={t('common.moreActions')}
                >
                  <MoreHorizontal className='size-4' aria-hidden />
                </ArcButton>
              }
            />
          </div>
        }
      />

      {/* 可关闭单行活跃告警条（Arc Alert）/ Dismissible single-line active alerts banner */}
      {monitoringOverview &&
      monitoringOverview.stats.active_alerts_1h > 0 &&
      dismissedAlertCount !== monitoringOverview.stats.active_alerts_1h ? (
        <ArcAlert
          tone='danger'
          title={t('cluster.activeAlertsBanner.title', {
            count: monitoringOverview.stats.active_alerts_1h,
          })}
          onDismiss={dismissAlertsBanner}
        >
          <div className='mt-1 flex flex-wrap items-center justify-between gap-2'>
            <span className='text-xs text-muted-foreground'>
              {monitoringOverview.stats.restart_failed_events_24h > 0
                ? t('cluster.activeAlertsBanner.description', {
                    restartFailed:
                      monitoringOverview.stats.restart_failed_events_24h,
                  })
                : t('cluster.activeAlertsBanner.descriptionGeneric', {
                    count: monitoringOverview.stats.active_alerts_1h,
                  })}
            </span>
            <ArcButton
              type='button'
              variant='secondary'
              size='sm'
              onClick={() =>
                router.push(`/monitoring?tab=alerts&cluster_id=${clusterId}`)
              }
            >
              {t('cluster.activeAlertsBanner.action')}
            </ArcButton>
          </div>
        </ArcAlert>
      ) : null}

      {/* 严格 6 个一级 Arc Tab（概览 / 节点 / 配置 / 插件 / 运行时 / 运维） */}
      {/* Strictly 6 primary Arc tabs (overview / nodes / configs / plugins / runtime / ops) */}
      <ArcTabs
        value={activeTab}
        onValueChange={(value) => requestRouteChange(value as ClusterDetailTab)}
        className='space-y-4'
      >
        <ArcTabsList>
          {CLUSTER_DETAIL_TABS.map((tabKey) => (
            <ArcTabsTrigger
              key={tabKey}
              value={tabKey}
              data-testid={`cluster-detail-tab-${tabKey}`}
            >
              {t(`cluster.detailTabs.${tabKey}`)}
            </ArcTabsTrigger>
          ))}
        </ArcTabsList>

        <ArcTabsContent value='overview'>
          {activeTab === 'overview' ? (
            <ClusterOverviewTab
              cluster={cluster}
              clusterStatus={clusterStatus}
              k8sPods={k8sPods}
              nodes={nodes}
              nodeJavaRuntimes={nodeJavaRuntimes}
              webUINode={webUINode}
              enableHTTP={runtimeConfig.enableHTTP}
              jobLogMode={runtimeConfig.jobLogMode}
              logModeSwitching={logModeSwitching}
              runtimeStorage={runtimeStorage}
              stxJavaProxy={stxJavaProxy}
              eventChannel={eventChannel}
              monitoringOverview={monitoringOverview}
              diagnosticsGroupTotal={diagnosticsGroupTotal}
              inspectionStarting={inspectionStarting}
              onSwitchJobLogMode={handleSwitchJobLogMode}
              onGoNodes={() => requestRouteChange('nodes')}
              onGoConfigs={() => requestRouteChange('configs')}
              onGoRuntime={(view) => requestRouteChange('runtime', view)}
              onGoOps={() => requestRouteChange('ops')}
              onOpenMonitoring={() =>
                router.push(`/monitoring?tab=alerts&cluster_id=${clusterId}`)
              }
              onOpenDiagnostics={() =>
                router.push(`/diagnostics?tab=errors&cluster_id=${clusterId}`)
              }
              onStartInspection={() => void handleStartInspection()}
            />
          ) : null}
        </ArcTabsContent>

        <ArcTabsContent value='nodes'>
          {activeTab === 'nodes' && cluster?.infra_type && cluster.infra_type !== 'bare_metal' ? (
            <div className='px-1 py-2'>
              <K8sNodesPanel
                clusterId={clusterId}
                cluster={cluster}
                refreshTrigger={lifecyclePollEpoch}
                onAfterMutate={() => void loadClusterData({silent: true})}
              />
            </div>
          ) : null}
          {activeTab === 'nodes' && (!cluster?.infra_type || cluster.infra_type === 'bare_metal') ? (
            <ClusterNodesTab
              clusterId={clusterId}
              nodes={nodes}
              refreshing={refreshing}
              isOperating={isOperating}
              nodeOperating={nodeOperating}
              selectedNodeIds={selectedNodeIds}
              nodeJavaRuntimes={nodeJavaRuntimes}
              runningScaleOperation={runningScaleOperation}
              onToggleNodeSelection={toggleNodeSelection}
              onToggleAllNodes={toggleAllNodes}
              onClearSelection={() => setSelectedNodeIds(new Set())}
              onAddNode={() => setIsAddNodeDialogOpen(true)}
              onOpenScaleProgress={() => setScaleProgressOpen(true)}
              onBatchOp={(op) => setConfirmBatchOp(op)}
              onNodeOp={(op, node) => setConfirmNodeOp({op, node})}
              onViewLogs={handleViewLogs}
              onEditNode={openEditNodeDialog}
              onRemoveNode={(node) => setNodeToRemove(node)}
              onMainStatusRefresh={refreshMainStatusFromEvents}
              lifecyclePollEpoch={lifecyclePollEpoch}
            />
          ) : null}
        </ArcTabsContent>

        <ArcTabsContent value='configs' className='space-y-4'>
          {activeTab === 'configs' ? (
            <>
              <ClusterConfigs
                clusterId={clusterId}
                deploymentMode={cluster.deployment_mode}
                onConfigChanged={handleConfigChanged}
                isKubernetes={isK8sCluster}
                k8sConfigMapName={(k8sCfg?.configmap_name as string | undefined) ?? undefined}
                k8sNamespace={(k8sCfg?.namespace as string | undefined) ?? undefined}
              />
              <EngineEventChannelCard
                clusterId={clusterId}
                onConfigChanged={handleConfigChanged}
              />
            </>
          ) : null}
        </ArcTabsContent>

        <ArcTabsContent value='plugins'>
          {activeTab === 'plugins' ? (
            <ClusterPlugins clusterId={clusterId} />
          ) : null}
        </ArcTabsContent>

        <ArcTabsContent value='runtime' className='space-y-4'>
          {activeTab === 'runtime' ? (
            <div className='space-y-4' data-testid='cluster-runtime-panel'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <SegmentedControl
                  label={t('cluster.list.runtimeViews.label')}
                  value={runtimeView}
                  onValueChange={(v) =>
                    requestRouteChange('runtime', v as RuntimeSubView)
                  }
                  options={runtimeViewOptions}
                />

                {runtimeView === 'checkpoint' || runtimeView === 'imap' ? (
                  <ArcButton
                    type='button'
                    variant='secondary'
                    size='sm'
                    iconOnly
                    onClick={() => void loadRuntimeStorage(true)}
                    disabled={runtimeStorageLoading}
                    aria-label={t('common.refresh')}
                    title={t('common.refresh')}
                  >
                    <RefreshCw
                      className={`size-3.5 ${runtimeStorageLoading ? 'animate-spin' : ''}`}
                      aria-hidden
                    />
                  </ArcButton>
                ) : runtimeView === 'proxy' ? (
                  <ArcButton
                    type='button'
                    variant='secondary'
                    size='sm'
                    iconOnly
                    onClick={() => void loadStxJavaProxyStatus()}
                    disabled={stxJavaProxyLoading}
                    aria-label={t('common.refresh')}
                    title={t('common.refresh')}
                    data-testid='stx-java-proxy-refresh'
                  >
                    <RefreshCw
                      className={`size-3.5 ${stxJavaProxyLoading ? 'animate-spin' : ''}`}
                      aria-hidden
                    />
                  </ArcButton>
                ) : null}
              </div>

              {/* 运行时存储（Checkpoint / IMAP）：单层扁平画布 + 0.5px 细线 */}
              {runtimeView === 'checkpoint' || runtimeView === 'imap' ? (
                <div className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
                  <TableLoadingBar
                    loading={
                      runtimeStorageLoading ||
                      runtimeStorageListingLoading !== null
                    }
                  />
                  <div className='space-y-5 px-6 py-5'>
                    <div className='space-y-1'>
                      <h3 className='text-sm font-semibold text-foreground'>
                        {t('cluster.runtimeStorage.title')}
                      </h3>
                      <p className='truncate text-xs text-muted-foreground'>
                        {t('cluster.runtimeStorage.configSource', {
                          source: runtimeStorage?.config_source || '-',
                        })}
                      </p>
                    </div>

                    {runtimeStorageLoading && !runtimeStorage ? (
                      <div className='space-y-3 border-t-[0.5px] border-border/80 pt-5'>
                        <Skeleton className='h-8 w-full' />
                        <Skeleton className='h-40 w-full' />
                      </div>
                    ) : (
                      <div className='space-y-5 border-t-[0.5px] border-border/80 pt-5'>
                        <RuntimeStorageEditor
                          clusterId={clusterId}
                          kind={runtimeView}
                          spec={
                            runtimeView === 'checkpoint'
                              ? runtimeStorage?.configured_checkpoint ||
                                runtimeStorage?.checkpoint
                              : runtimeStorage?.configured_imap ||
                                runtimeStorage?.imap
                          }
                          onApplied={() => void loadRuntimeStorage()}
                        />
                        {runtimeView === 'checkpoint'
                          ? renderRuntimeStorageSpec(
                              runtimeStorage?.checkpoint,
                              t('installer.checkpointConfig'),
                            )
                          : renderRuntimeStorageSpec(
                              runtimeStorage?.imap,
                              t('installer.runtimeStorage.imapTitle'),
                            )}
                      </div>
                    )}
                  </div>
                </div>
              ) : null}

              {/* STX Java Proxy：单层扁平画布 + 0.5px 细线 */}
              {runtimeView === 'proxy' ? (
                <div
                  className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'
                  data-testid='stx-java-proxy-card'
                >
                  <TableLoadingBar loading={stxJavaProxyLoading} />
                  <div className='px-6 py-5'>
                    <div className='flex flex-wrap items-center justify-between gap-3 pb-4'>
                      <div className='flex items-center gap-1.5'>
                        <h3 className='text-sm font-semibold text-foreground'>
                          {t('cluster.stxJavaProxy.title')}
                        </h3>
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
                          <TooltipContent
                            side='top'
                            className='max-w-xs text-xs font-normal leading-relaxed'
                          >
                            {t('cluster.stxJavaProxy.description')}
                          </TooltipContent>
                        </Tooltip>
                      </div>
                      <SoftChip
                        tone={
                          stxJavaProxy?.healthy
                            ? 'emerald'
                            : stxJavaProxy?.running
                              ? 'amber'
                              : 'muted'
                        }
                      >
                        {stxJavaProxy?.healthy
                          ? t('cluster.stxJavaProxy.healthy')
                          : stxJavaProxy?.running
                            ? t('cluster.stxJavaProxy.unhealthy')
                            : t('cluster.stxJavaProxy.stopped')}
                      </SoftChip>
                    </div>

                    {stxJavaProxyLoading && !stxJavaProxy ? (
                      <div className='space-y-3 border-t-[0.5px] border-border/80 pt-4'>
                        <Skeleton className='h-20 w-full' />
                        <Skeleton className='h-9 w-64' />
                      </div>
                    ) : (
                      <>
                        <dl className='grid grid-cols-2 gap-x-6 gap-y-4 border-t-[0.5px] border-border/80 py-5 text-xs xl:grid-cols-4'>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.deploymentNode')}
                            </dt>
                            <dd className='truncate font-medium text-foreground'>
                              {stxJavaProxy?.host_name || '-'}
                              {stxJavaProxy?.role
                                ? ` (${t(`cluster.roles.${getRoleTranslationKey(stxJavaProxy.role)}`)})`
                                : ''}
                            </dd>
                          </div>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.endpoint')}
                            </dt>
                            <dd
                              className='truncate font-mono font-medium tabular-nums'
                              title={stxJavaProxy?.endpoint || '-'}
                            >
                              {stxJavaProxy?.endpoint || '-'}
                            </dd>
                          </div>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.pid')}
                            </dt>
                            <dd className='font-mono font-medium tabular-nums'>
                              {stxJavaProxy?.pid ?? '-'}
                            </dd>
                          </div>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.logPath')}
                            </dt>
                            <dd
                              className='truncate font-mono font-medium'
                              title={stxJavaProxy?.log_path || '-'}
                            >
                              {stxJavaProxy?.log_path?.trim()
                                ? stxJavaProxy.log_path
                                : '-'}
                            </dd>
                          </div>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.jvmMemoryMax')}
                            </dt>
                            <dd className='font-mono font-medium tabular-nums'>
                              {stxJavaProxy?.jvm_memory?.maxMemoryMb
                                ? `${stxJavaProxy.jvm_memory.maxMemoryMb} MB`
                                : '512 MB'}
                            </dd>
                          </div>
                          <div className='min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.jvmMemoryUsage')}
                            </dt>
                            <dd className='font-mono font-medium tabular-nums'>
                              {stxJavaProxy?.jvm_memory?.usedMemoryBytes != null
                                ? `${Math.round(stxJavaProxy.jvm_memory.usedMemoryBytes / (1024 * 1024))} MB / ${Math.round((stxJavaProxy.jvm_memory.totalMemoryBytes || 0) / (1024 * 1024))} MB`
                                : '-'}
                            </dd>
                          </div>
                          <div className='col-span-2 min-w-0 space-y-1'>
                            <dt className='text-muted-foreground'>
                              {t('cluster.stxJavaProxy.jvmOpts')}
                            </dt>
                            <dd
                              className='truncate font-mono font-medium'
                              title={
                                stxJavaProxy?.jvm_opts ||
                                t('cluster.stxJavaProxy.jvmOptsDefault')
                              }
                            >
                              {stxJavaProxy?.jvm_opts?.trim()
                                ? stxJavaProxy.jvm_opts
                                : t('cluster.stxJavaProxy.jvmOptsDefault')}
                            </dd>
                          </div>
                        </dl>

                        <div className='border-t-[0.5px] border-border/80 py-4 text-xs text-muted-foreground'>
                          {stxJavaProxy?.message ||
                            t('cluster.stxJavaProxy.noStatus')}
                        </div>

                        <div className='flex flex-wrap gap-2 border-t-[0.5px] border-border/80 pt-4'>
                          <ArcButton
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={handleOpenStxJavaProxyConfig}
                            disabled={
                              stxJavaProxyLoading ||
                              stxJavaProxyOperating !== null
                            }
                            data-testid='stx-java-proxy-config-btn'
                          >
                            <SlidersHorizontal className='size-3.5' aria-hidden />
                            {t('cluster.stxJavaProxy.configureMemory')}
                          </ArcButton>
                          <ArcButton
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={() => void handlePreviewStxJavaProxyLog()}
                            disabled={stxJavaProxyLoading}
                            loading={stxJavaProxyLogLoading}
                            data-testid='stx-java-proxy-view-log'
                          >
                            <FileText className='size-3.5' aria-hidden />
                            {t('cluster.stxJavaProxy.viewRuntimeLog')}
                          </ArcButton>
                          <ArcButton
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={() =>
                              void handleStxJavaProxyOperation('start')
                            }
                            disabled={stxJavaProxyOperating !== null}
                            loading={stxJavaProxyOperating === 'start'}
                            data-testid='stx-java-proxy-start'
                          >
                            <Play className='size-3.5' aria-hidden />
                            {t('cluster.stxJavaProxy.start')}
                          </ArcButton>
                          <ArcButton
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={() =>
                              void handleStxJavaProxyOperation('restart')
                            }
                            disabled={stxJavaProxyOperating !== null}
                            loading={stxJavaProxyOperating === 'restart'}
                            data-testid='stx-java-proxy-restart'
                          >
                            <RotateCcw className='size-3.5' aria-hidden />
                            {t('cluster.stxJavaProxy.restart')}
                          </ArcButton>
                          <ArcButton
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={() =>
                              void handleStxJavaProxyOperation('stop')
                            }
                            disabled={stxJavaProxyOperating !== null}
                            loading={stxJavaProxyOperating === 'stop'}
                            data-testid='stx-java-proxy-stop'
                          >
                            <Square className='size-3.5' aria-hidden />
                            {t('cluster.stxJavaProxy.stop')}
                          </ArcButton>
                        </div>
                      </>
                    )}
                  </div>
                </div>
              ) : null}

              {/* 内嵌 SeaTunnel Web UI（仅 webUINode 存在时可用） */}
              {runtimeView === 'webui' && hasWebUI ? (
                <div className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
                  <div className='flex flex-wrap items-center justify-between gap-3 border-b-[0.5px] border-border/80 px-5 py-3.5'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <MonitorSmartphone
                        className='size-4 shrink-0 text-primary'
                        aria-hidden
                      />
                      <span className='text-sm font-semibold whitespace-nowrap'>
                        {t('cluster.webUiTitle')}
                      </span>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type='button'
                            className='min-w-0 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
                            aria-label={t('cluster.webUiMasterAddressTip', {
                              address: webUIMasterAddress,
                            })}
                            data-testid='cluster-webui-master-address-trigger'
                          >
                            <span
                              className='block max-w-[260px] cursor-help truncate rounded-md bg-muted/50 px-2 py-0.5 font-mono text-[11px] tabular-nums'
                              data-testid='cluster-webui-master-address'
                            >
                              {webUIMasterAddress}
                            </span>
                          </button>
                        </TooltipTrigger>
                        <TooltipContent
                          side='top'
                          className='max-w-sm text-xs'
                        >
                          {t('cluster.webUiMasterAddressTip', {
                            address: webUIMasterAddress,
                          })}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <SeaTunnelObserveMenu
                      clusterId={clusterId}
                      variant='button'
                      className='shrink-0'
                      triggerHint={t('cluster.webUiHint')}
                    />
                  </div>
                  <iframe
                    key={webUIProxyURL}
                    title='SeaTunnel Web UI'
                    src={webUIProxyURL}
                    className='h-[860px] w-full border-0 bg-background'
                    sandbox='allow-scripts allow-same-origin allow-forms allow-popups allow-downloads'
                    referrerPolicy='strict-origin-when-cross-origin'
                  />
                </div>
              ) : null}
            </div>
          ) : null}
        </ArcTabsContent>

        <ArcTabsContent value='ops'>
          {activeTab === 'ops' ? (
            <ClusterOpsTab
              clusterId={clusterId}
              clusterName={cluster.name}
              view={opsView}
              onViewChange={(view) => requestRouteChange('ops', view)}
              onGuardDirtyChange={setGuardDirty}
              diagnosticsGroups={diagnosticsGroups}
              diagnosticsGroupTotal={diagnosticsGroupTotal}
              diagnosticsLoading={diagnosticsLoading}
              inspectionStarting={inspectionStarting}
              onStartInspection={() => void handleStartInspection()}
              upgradeTasks={upgradeTasks}
              upgradeTasksLoading={upgradeTasksLoading}
              upgradeTasksPage={upgradeTasksPage}
              upgradeTasksPageSize={upgradeTasksPageSize}
              upgradeTasksTotal={upgradeTasksTotal}
              onUpgradeTasksPageChange={setUpgradeTasksPage}
              onOpenUpgradeTask={openUpgradeTaskDetail}
              isKubernetes={isK8sCluster}
              onOpenK8sUpgrade={() => setK8sUpgradeOpen(true)}
            />
          ) : null}
        </ArcTabsContent>
      </ArcTabs>

      {isK8sCluster ? (
        <K8sUpgradeImageDialog
          open={k8sUpgradeOpen}
          onOpenChange={setK8sUpgradeOpen}
          clusterId={clusterId}
          currentImage={(k8sCfg?.image as string | undefined) ?? ''}
          onUpgraded={() => {
            void loadClusterData({silent: true});
            setLifecyclePollEpoch((n) => n + 1);
          }}
        />
      ) : null}

      <DirtyCloseDialog
        open={pendingRoute !== null}
        onDiscard={commitPendingRoute}
        onKeepEditing={() => setPendingRoute(null)}
      />

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
      <ConfirmDialog
        open={isDeleteDialogOpen}
        onOpenChange={(open) => {
          setIsDeleteDialogOpen(open);
          if (!open) {
            setForceDelete(false);
          }
        }}
        title={t('cluster.deleteCluster')}
        description={t('cluster.deleteConfirm', {name: cluster.name})}
        cancelLabel={t('common.cancel')}
        confirmLabel={t('common.delete')}
        confirmVariant='danger'
        onConfirm={handleDelete}
      >
        <p className='text-sm text-muted-foreground'>
          {t('cluster.deleteConfirmWarning')}
        </p>
        <ArcCheckbox
          checked={forceDelete}
          onCheckedChange={(v) => setForceDelete(v === true)}
          label={t('cluster.forceDeleteOption')}
        />
      </ConfirmDialog>

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
      <ArcDialog
        open={scaleProgressOpen && runningScaleOperation !== null}
        onOpenChange={setScaleProgressOpen}
      >
        <ArcDialogContent
          title={
            runningScaleOperation
              ? t(`cluster.scale.kind.${runningScaleOperation.kind}`)
              : ''
          }
          className='arc-compact w-[min(92vw,680px)]!'
        >
          {runningScaleOperation ? (
            <div className='max-h-[64vh] overflow-y-auto pr-1'>
              <ScaleOperationProgress
                clusterId={clusterId}
                operationId={runningScaleOperation.id}
                initial={runningScaleOperation}
                onFinished={handleRunningScaleFinished}
              />
            </div>
          ) : null}
        </ArcDialogContent>
      </ArcDialog>

      {/* Batch start/stop/restart confirmation / 批量启动/停止/重启二次确认 */}
      <ConfirmDialog
        open={!!confirmBatchOp}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmBatchOp(null);
          }
        }}
        title={
          confirmBatchOp === 'start'
            ? t('cluster.batchStartConfirmTitle')
            : confirmBatchOp === 'stop'
              ? t('cluster.batchStopConfirmTitle')
              : confirmBatchOp === 'restart'
                ? t('cluster.batchRestartConfirmTitle')
                : ''
        }
        description={
          confirmBatchOp === 'start'
            ? t('cluster.batchStartConfirmMessage', {
                count: selectedNodeIds.size,
              })
            : confirmBatchOp === 'stop'
              ? t('cluster.batchStopConfirmMessage', {
                  count: selectedNodeIds.size,
                })
              : confirmBatchOp === 'restart'
                ? t('cluster.batchRestartConfirmMessage', {
                    count: selectedNodeIds.size,
                  })
                : ''
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={
          confirmBatchOp === 'start'
            ? t('cluster.start')
            : confirmBatchOp === 'stop'
              ? t('cluster.stop')
              : t('cluster.restart')
        }
        confirmVariant={confirmBatchOp === 'stop' ? 'danger' : 'primary'}
        onConfirm={() => {
          // 先关弹窗后台执行，避免等待节点接口时弹窗滞留。
          // Close the dialog first and run in the background so it never lingers while waiting on node APIs.
          const op = confirmBatchOp;
          setConfirmBatchOp(null);
          if (op === 'start') {
            void handleBatchStart();
          } else if (op === 'stop') {
            void handleBatchStop();
          } else if (op === 'restart') {
            void handleBatchRestart();
          }
        }}
      />

      {/* Single node start/stop/restart confirmation / 单节点启动/停止/重启二次确认 */}
      <ConfirmDialog
        open={!!confirmNodeOp}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmNodeOp(null);
          }
        }}
        title={
          confirmNodeOp?.op === 'start'
            ? t('cluster.nodeStartConfirmTitle')
            : confirmNodeOp?.op === 'stop'
              ? t('cluster.nodeStopConfirmTitle')
              : confirmNodeOp?.op === 'restart'
                ? t('cluster.nodeRestartConfirmTitle')
                : ''
        }
        description={
          confirmNodeOp
            ? confirmNodeOp.op === 'start'
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
                  })
            : ''
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={
          confirmNodeOp?.op === 'start'
            ? t('cluster.start')
            : confirmNodeOp?.op === 'stop'
              ? t('cluster.stop')
              : t('cluster.restart')
        }
        confirmVariant={confirmNodeOp?.op === 'stop' ? 'danger' : 'primary'}
        onConfirm={() => {
          if (!confirmNodeOp) {
            return;
          }
          const {op, node} = confirmNodeOp;
          setConfirmNodeOp(null);
          if (op === 'start') {
            void handleNodeStart(node);
          } else if (op === 'stop') {
            void handleNodeStop(node);
          } else {
            void handleNodeRestart(node);
          }
        }}
      />

      <ConfirmDialog
        open={imapCleanupOpen}
        onOpenChange={setImapCleanupOpen}
        title={t('cluster.runtimeStorage.cleanupConfirmTitle')}
        description={t('cluster.runtimeStorage.cleanupConfirmDesc')}
        cancelLabel={t('common.cancel')}
        confirmLabel={t('cluster.runtimeStorage.cleanupImap')}
        confirmVariant='danger'
        loading={imapCleanupRunning}
        onConfirm={handleCleanupIMAP}
      />

      <ArcDialog
        open={runtimeStoragePreviewOpen}
        onOpenChange={setRuntimeStoragePreviewOpen}
      >
        <ArcDialogContent
          className='arc-compact w-[min(94vw,960px)]!'
          title={
            <span className='flex items-center gap-2'>
              {runtimeStoragePreview?.file_name?.endsWith('.ser') ? (
                <Database className='size-4 shrink-0 text-purple-400' />
              ) : runtimeStoragePreview?.binary ? (
                <File className='size-4 shrink-0 text-muted-foreground' />
              ) : (
                <FileText className='size-4 shrink-0 text-sky-400' />
              )}
              <span className='truncate font-mono'>
                {runtimeStoragePreview?.file_name ||
                  t('cluster.runtimeStorage.preview')}
              </span>
            </span>
          }
          description={
            <span className='flex items-center gap-1.5 font-mono text-xs'>
              <span
                className='truncate'
                title={runtimeStoragePreview?.path || '-'}
              >
                {runtimeStoragePreview?.path || '-'}
              </span>
              {runtimeStoragePreview?.path ? (
                <ArcButton
                  type='button'
                  variant='ghost'
                  size='sm'
                  iconOnly
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
                  aria-label={t('cluster.runtimeStorage.copyPath')}
                  title={t('cluster.runtimeStorage.copyPath')}
                >
                  <Copy className='size-3' aria-hidden />
                </ArcButton>
              ) : null}
            </span>
          }
        >
          <div className='space-y-4'>
            {/* 若为 Checkpoint 序列化文件，提供一键反序列化直达动作 */}
            {/* If it's a Checkpoint serialization file, provide a quick deserialize action */}
            {runtimeStoragePreview?.path?.endsWith('.ser') ? (
              <div className='flex justify-end'>
                <ArcButton
                  type='button'
                  size='sm'
                  onClick={() => {
                    const p = runtimeStoragePreview?.path;
                    if (p) {
                      setRuntimeStoragePreviewOpen(false);
                      void handleInspectCheckpoint(p);
                    }
                  }}
                >
                  <Database className='size-3.5' aria-hidden />
                  {t('cluster.runtimeStorage.deserializeCheckpoint')}
                </ArcButton>
              </div>
            ) : null}

            <dl className='grid gap-x-6 gap-y-3 border-b-[0.5px] border-border/80 pb-3.5 text-xs md:grid-cols-4'>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.fileName')}
                </dt>
                <dd
                  className='truncate font-mono font-medium text-foreground'
                  title={runtimeStoragePreview?.file_name || '-'}
                >
                  {runtimeStoragePreview?.file_name || '-'}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.size')}
                </dt>
                <dd className='font-mono font-medium tabular-nums text-foreground'>
                  {formatBytes(runtimeStoragePreview?.size_bytes)}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.encoding')}
                </dt>
                <dd className='flex items-center gap-1.5 font-mono font-medium text-foreground'>
                  <span>{runtimeStoragePreview?.encoding || '-'}</span>
                  <NeutralChip>
                    {runtimeStoragePreview?.binary ? 'Binary' : 'Text'}
                  </NeutralChip>
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.truncated')}
                </dt>
                <dd className='font-mono font-medium text-foreground'>
                  {runtimeStoragePreview?.truncated ? 'Yes (64KB max)' : 'No'}
                </dd>
              </div>
            </dl>

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
          </div>
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog
        open={stxJavaProxyLogOpen}
        onOpenChange={setStxJavaProxyLogOpen}
      >
        <ArcDialogContent
          className='arc-compact w-[min(96vw,1480px)]! h-[86vh]!'
          bodyClassName='flex min-h-0 flex-1 flex-col overflow-hidden p-4!'
          title={t('cluster.stxJavaProxy.viewRuntimeLog')}
          description={
            <span
              className='block truncate font-mono text-xs'
              title={
                stxJavaProxyLogResult?.log_path || stxJavaProxy?.log_path || '-'
              }
            >
              {stxJavaProxyLogResult?.log_path || stxJavaProxy?.log_path || '-'}
            </span>
          }
        >
          <ExpandableTextPanel
            content={stxJavaProxyLogResult?.logs || '-'}
            height='flex'
            className='h-full min-h-0'
            tone='plain'
            wrap
            showExpand={false}
          />
        </ArcDialogContent>
      </ArcDialog>

      <ArcDialog
        open={stxJavaProxyConfigOpen}
        onOpenChange={setStxJavaProxyConfigOpen}
      >
        <ArcDialogContent
          className='arc-compact w-[min(92vw,580px)]!'
          title={
            <span className='flex items-center gap-2'>
              <Cpu className='size-4 text-primary' aria-hidden />
              {t('cluster.stxJavaProxy.configDialogTitle')}
            </span>
          }
        >
          <div className='space-y-5 text-sm'>
            <div className='space-y-2'>
              <div className='text-xs font-medium text-foreground'>
                {t('cluster.stxJavaProxy.presetLabel')}
              </div>
              <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
                {[
                  {id: '512m', label: t('cluster.stxJavaProxy.preset512m')},
                  {id: '1024m', label: t('cluster.stxJavaProxy.preset1g')},
                  {id: '2048m', label: t('cluster.stxJavaProxy.preset2g')},
                  {id: '4096m', label: t('cluster.stxJavaProxy.preset4g')},
                  {id: 'custom', label: t('cluster.stxJavaProxy.presetCustom')},
                ].map((preset) => (
                  <ArcButton
                    key={preset.id}
                    type='button'
                    variant={
                      stxJavaProxyJvmPreset === preset.id
                        ? 'primary'
                        : 'secondary'
                    }
                    size='sm'
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
                  </ArcButton>
                ))}
              </div>
            </div>

            <div className='border-t-[0.5px] border-border/80 pt-4'>
              <ArcInput
                id='stx-java-proxy-jvm-opts'
                label={t('cluster.stxJavaProxy.jvmOptsField')}
                description={t('cluster.stxJavaProxy.jvmOptsHelp')}
                value={stxJavaProxyJvmOptsInput}
                onChange={(e) => {
                  setStxJavaProxyJvmOptsInput(e.target.value);
                  setStxJavaProxyJvmPreset('custom');
                }}
                placeholder={t('cluster.stxJavaProxy.jvmOptsPlaceholder')}
                className='font-mono text-xs'
                data-testid='stx-java-proxy-jvm-opts-input'
              />
            </div>

            {clusterIsK8s ? (
              <p className='text-xs text-muted-foreground' data-testid='k8s-proxy-memory-hint'>
                {t('cluster.stxJavaProxy.k8sMemorySyncHint', {
                  memory: deriveProxyContainerMemory(stxJavaProxyJvmOptsInput).limitDisplay,
                })}
              </p>
            ) : null}

            <div className='border-t-[0.5px] border-border/80 pt-4'>
              <ArcCheckbox
                id='stx-java-proxy-restart-save'
                label={t('cluster.stxJavaProxy.restartImmediately')}
                checked={stxJavaProxyRestartOnSave}
                onCheckedChange={(checked) =>
                  setStxJavaProxyRestartOnSave(checked === true)
                }
              />
            </div>

            <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <ArcButton
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => setStxJavaProxyConfigOpen(false)}
                disabled={stxJavaProxySaving}
              >
                {t('common.cancel')}
              </ArcButton>
              <ArcButton
                type='button'
                size='sm'
                onClick={() => void handleSaveStxJavaProxyConfig()}
                loading={stxJavaProxySaving}
                data-testid='stx-java-proxy-save-config-btn'
              >
                {stxJavaProxySaving
                  ? t('cluster.stxJavaProxy.saving')
                  : t('cluster.stxJavaProxy.saveAndApply')}
              </ArcButton>
            </div>
          </div>
        </ArcDialogContent>
      </ArcDialog>

      {/* 与工作台共用 Checkpoint 双看板；支持自动关联作业或手动指定连接器解析位点。
          Reuse workbench Checkpoint dual-pane dialog; supports auto job config association and manual connector decode. */}
      <CheckpointInspectDialog
        open={checkpointInspectOpen}
        onOpenChange={setCheckpointInspectOpen}
        result={checkpointInspectResult}
        t={tCheckpoint}
        onResultUpdate={setCheckpointInspectResult}
      />

      <ArcDialog open={imapInspectOpen} onOpenChange={setImapInspectOpen}>
        <ArcDialogContent
          className='arc-compact w-[min(94vw,1080px)]!'
          title={t('cluster.runtimeStorage.inspectWal')}
          description={
            <span
              className='block truncate font-mono text-xs'
              title={imapInspectResult?.path || '-'}
            >
              {imapInspectResult?.path || '-'}
            </span>
          }
        >
          <div className='space-y-4'>
            <dl className='grid gap-x-6 gap-y-3 border-b-[0.5px] border-border/80 pb-3.5 text-xs md:grid-cols-5'>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.fileName')}
                </dt>
                <dd
                  className='truncate font-mono font-medium'
                  title={imapInspectResult?.file_name || '-'}
                >
                  {imapInspectResult?.file_name || '-'}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.size')}
                </dt>
                <dd className='font-mono font-medium tabular-nums'>
                  {formatBytes(imapInspectResult?.size_bytes)}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.encoding')}
                </dt>
                <dd className='font-mono font-medium'>
                  {imapInspectResult?.encoding || '-'}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.storageType')}
                </dt>
                <dd className='font-mono font-medium'>
                  {imapInspectResult?.storage_type || '-'}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.runtimeStorage.entryCount')}
                </dt>
                <dd className='font-mono font-medium tabular-nums'>
                  {imapInspectResult?.entry_count ?? 0}
                </dd>
              </div>
            </dl>
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
          </div>
        </ArcDialogContent>
      </ArcDialog>

      {/* View Logs Dialog / 查看日志对话框 */}
      <ClusterNodeLogDialog
        open={isLogDialogOpen}
        onOpenChange={setIsLogDialogOpen}
        node={logNodeInfo}
        clusterId={clusterId}
      />
    </div>
  );
}
