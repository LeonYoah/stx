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
 * Create Cluster Dialog Component
 * 创建集群对话框组件
 *
 * Dialog for registering an existing SeaTunnel cluster.
 * Supports process discovery to auto-fill cluster info from running processes.
 * 用于注册已有 SeaTunnel 集群的对话框。
 * 支持进程发现以从运行中的进程自动填充集群信息。
 */

import {useState, useEffect, useRef} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Alert} from '@/components/arc/alert/alert';
import {JavaHomePicker} from '@/components/common/java/JavaHomePicker';
import {HelpHint} from '@/components/common/HelpHint';
import {Textarea} from '@/components/ui/textarea';
import {
  DirtyCloseDialog,
  NeutralChip,
  SoftChip,
  TruncatedText,
  useBeforeUnload,
} from '@/components/common/layout';
import {
  Search,
  Server,
  Cpu,
  ChevronDown,
  ChevronRight,
  SlidersHorizontal,
} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {DeploymentMode, CreateClusterRequest} from '@/lib/services/cluster/types';
import {HostInfo, AgentStatus} from '@/lib/services/host/types';
import {DiscoveredProcess} from '@/lib/services/discovery/discovery.service';
import {
  INITIAL_K8S_ADOPT_FORM,
  K8sAdoptClusterSection,
  type K8sAdoptFormErrors,
  type K8sAdoptFormState,
} from './K8sAdoptClusterSection';

type RegisterInfraTarget = 'bare_metal' | 'kubernetes';

interface CreateClusterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

/**
 * Create Cluster Dialog Component
 * 创建集群对话框组件
 */
export function CreateClusterDialog({open, onOpenChange, onSuccess}: CreateClusterDialogProps) {
  const t = useTranslations();
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [loadingHosts, setLoadingHosts] = useState(false);
  const [infraTarget, setInfraTarget] =
    useState<RegisterInfraTarget>('bare_metal');
  const [k8sForm, setK8sForm] = useState<K8sAdoptFormState>(() => ({
    ...INITIAL_K8S_ADOPT_FORM,
  }));
  const [k8sErrors, setK8sErrors] = useState<K8sAdoptFormErrors>({});

  // Form state / 表单状态
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [deploymentMode, setDeploymentMode] = useState<DeploymentMode>(DeploymentMode.HYBRID);
  const [version, setVersion] = useState('');

  // Host and discovery state / 主机和发现状态
  const [availableHosts, setAvailableHosts] = useState<HostInfo[]>([]);
  const [selectedHostIds, setSelectedHostIds] = useState<Set<number>>(new Set());
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  // Map of hostId -> discovered processes / 主机ID -> 发现的进程映射
  const [discoveredProcessesByHost, setDiscoveredProcessesByHost] = useState<
    Map<number, DiscoveredProcess[]>
  >(new Map());
  const [selectedProcesses, setSelectedProcesses] = useState<Set<string>>(new Set());
  // 扫描失败的主机数，就地告警展示 / Failed-host count shown as an inline warning
  const [failedHostCount, setFailedHostCount] = useState(0);
  // 是否已执行过至少一次发现，用于 0 进程时给出就地提示 / Tracks completed scan so 0 results show inline feedback
  const [hasDiscovered, setHasDiscovered] = useState(false);
  /** process key -> java_home (AC1e/AC1f) / 进程键到 JDK */
  const [javaHomesByKey, setJavaHomesByKey] = useState<Record<string, string>>({});

  // Advanced settings state (stx-java-proxy & JVM) / 高级配置状态（stx-java-proxy 与 JVM 参数）
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [autoStartProxy, setAutoStartProxy] = useState(true);
  const [proxyPort, setProxyPort] = useState<number>(18080);
  const [proxyJvmOpts, setProxyJvmOpts] = useState<string>('-Xms128m -Xmx512m');
  const [proxyJvmPreset, setProxyJvmPreset] = useState<string>('512m');

  // Inline validation & dirty-close state / 就地校验与脏态关闭状态
  const [formErrors, setFormErrors] = useState<{name?: string; version?: string; nodes?: string}>({});
  const [dirtyCloseOpen, setDirtyCloseOpen] = useState(false);
  const nodesSectionRef = useRef<HTMLDivElement>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const versionInputRef = useRef<HTMLInputElement>(null);

  const dirty =
    infraTarget === 'kubernetes'
      ? k8sForm.name.trim().length > 0 ||
        k8sForm.description.trim().length > 0 ||
        k8sForm.master_service_name.trim().length > 0 ||
        k8sForm.cluster_service_name.trim().length > 0 ||
        k8sForm.master_workload_name.trim().length > 0
      : name.trim().length > 0 ||
        description.trim().length > 0 ||
        version.trim().length > 0 ||
        selectedHostIds.size > 0 ||
        selectedProcesses.size > 0;
  useBeforeUnload(open && dirty);

  /**
   * Load available hosts when dialog opens
   * 对话框打开时加载可用主机
   */
  useEffect(() => {
    if (open) {
      loadAvailableHosts();
    }
  }, [open]);

  /**
   * Load available hosts with Agent installed
   * 加载已安装 Agent 的可用主机
   */
  const loadAvailableHosts = async () => {
    setLoadingHosts(true);
    try {
      const result = await services.host.getHostsSafe({
        current: 1,
        size: 100,
        agent_status: AgentStatus.INSTALLED,
      });

      if (result.success && result.data) {
        setAvailableHosts(result.data.hosts || []);
      } else {
        setAvailableHosts([]);
      }
    } finally {
      setLoadingHosts(false);
    }
  };

  /**
   * Reset form
   * 重置表单
   */
  const resetForm = () => {
    setInfraTarget('bare_metal');
    setK8sForm({...INITIAL_K8S_ADOPT_FORM});
    setK8sErrors({});
    setName('');
    setDescription('');
    setDeploymentMode(DeploymentMode.HYBRID);
    setVersion('');
    setSelectedHostIds(new Set());
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());
    setFailedHostCount(0);
    setHasDiscovered(false);
    setJavaHomesByKey({});
    setFormErrors({});
    setShowAdvanced(false);
    setAutoStartProxy(true);
    setProxyPort(18080);
    setProxyJvmOpts('-Xms128m -Xmx512m');
    setProxyJvmPreset('512m');
  };

  /**
   * Toggle host selection
   * 切换主机选择
   */
  const toggleHostSelection = (hostId: number) => {
    const newSelected = new Set(selectedHostIds);
    if (newSelected.has(hostId)) {
      newSelected.delete(hostId);
      // 取消主机时同步清理该主机下的发现进程与勾选键，避免已选计数失真
      // Clear discovered processes and selected keys for the unchecked host to keep counts accurate
      const newProcesses = new Map(discoveredProcessesByHost);
      newProcesses.delete(hostId);
      setDiscoveredProcessesByHost(newProcesses);
      const prefix = `${hostId}-`;
      setSelectedProcesses((prev) => {
        const next = new Set<string>();
        prev.forEach((key) => {
          if (!key.startsWith(prefix)) {
            next.add(key);
          }
        });
        return next;
      });
      setJavaHomesByKey((prev) =>
        Object.fromEntries(
          Object.entries(prev).filter(([key]) => !key.startsWith(prefix)),
        ),
      );
    } else {
      newSelected.add(hostId);
    }
    setSelectedHostIds(newSelected);
  };

  /**
   * Select all hosts
   * 全选主机
   */
  const selectAllHosts = () => {
    setSelectedHostIds(new Set(availableHosts.map((h) => h.id)));
  };

  /**
   * Deselect all hosts
   * 取消全选主机
   */
  const deselectAllHosts = () => {
    setSelectedHostIds(new Set());
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());
    setFailedHostCount(0);
    setHasDiscovered(false);
  };

  /**
   * Handle process discovery for all selected hosts
   * 处理所有选中主机的进程发现
   */
  const handleDiscoverProcesses = async () => {
    if (selectedHostIds.size === 0) {
      return;
    }

    setDiscoveryLoading(true);
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());
    setFailedHostCount(0);

    const newProcessesByHost = new Map<number, DiscoveredProcess[]>();
    let totalProcesses = 0;
    let failedHosts = 0;

    try {
      // Discover processes on all selected hosts in parallel
      // 并行在所有选中的主机上发现进程
      const hostIds = Array.from(selectedHostIds);
      const results = await Promise.allSettled(
        hostIds.map((hostId) => services.discovery.discoverProcesses(hostId))
      );

      results.forEach((result, index) => {
        const hostId = hostIds[index];
        if (result.status === 'fulfilled' && result.value.success && result.value.processes) {
          newProcessesByHost.set(hostId, result.value.processes);
          totalProcesses += result.value.processes.length;
        } else {
          failedHosts++;
        }
      });

      setDiscoveredProcessesByHost(newProcessesByHost);
      setFailedHostCount(failedHosts);
      setHasDiscovered(true);

      if (totalProcesses > 0) {
        // Auto-select all discovered processes by default to save user manual clicks
        // 发现后默认全选所有扫描出的进程，减少用户逐个点击的成本
        const allKeys = new Set<string>();
        newProcessesByHost.forEach((procs, hId) => {
          procs.forEach((p) => allKeys.add(getProcessKey(hId, p)));
        });
        setSelectedProcesses(allKeys);
        setFormErrors((prev) => ({...prev, nodes: undefined}));
        // Prefill process JDK when Agent reported it (AC1f) / 有进程 JDK 时预填（AC1f）
        const seeded: Record<string, string> = {};
        newProcessesByHost.forEach((procs, hId) => {
          procs.forEach((proc) => {
            const key = getProcessKey(hId, proc);
            if (proc.java_home) {
              seeded[key] = proc.java_home;
            }
          });
        });
        setJavaHomesByKey(seeded);

        // Auto-detect deployment mode from all discovered processes
        // 从所有发现的进程自动检测部署模式
        const allProcesses = Array.from(newProcessesByHost.values()).flat();
        autoDetectDeploymentMode(allProcesses);
      }
    } catch {
      toast.error(t('discovery.discoverError'));
    } finally {
      setDiscoveryLoading(false);
    }
  };

  /**
   * Auto-detect deployment mode and version from discovered processes
   * 从发现的进程自动检测部署模式和版本
   */
  const autoDetectDeploymentMode = (processes: DiscoveredProcess[]) => {
    const hasMaster = processes.some((p) => p.role === 'master');
    const hasWorker = processes.some((p) => p.role === 'worker');
    const hasHybrid = processes.some((p) => p.role === 'hybrid');

    if (hasMaster || hasWorker) {
      setDeploymentMode(DeploymentMode.SEPARATED);
    } else if (hasHybrid) {
      setDeploymentMode(DeploymentMode.HYBRID);
    }

    // Auto-fill version from first process with valid version (always update if found)
    // 从第一个有效版本的进程自动填充版本（发现后始终更新）
    const processWithVersion = processes.find((p) => p.version && p.version !== 'unknown');
    if (processWithVersion) {
      setVersion(processWithVersion.version);
      setFormErrors((prev) => ({...prev, version: undefined}));
    }
  };

  /**
   * Toggle process selection
   * 切换进程选择
   */
  const toggleProcessSelection = (processKey: string) => {
    const newSelected = new Set(selectedProcesses);
    if (newSelected.has(processKey)) {
      newSelected.delete(processKey);
    } else {
      newSelected.add(processKey);
    }
    setSelectedProcesses(newSelected);
    setFormErrors((prev) => ({...prev, nodes: undefined}));
  };

  /**
   * Select all discovered processes
   * 全选所有发现的进程
   */
  const selectAllProcesses = () => {
    const allKeys = new Set<string>();
    discoveredProcessesByHost.forEach((procs, hId) => {
      procs.forEach((p) => allKeys.add(getProcessKey(hId, p)));
    });
    setSelectedProcesses(allKeys);
    setFormErrors((prev) => ({...prev, nodes: undefined}));
  };

  /**
   * Deselect all processes
   * 取消全选进程
   */
  const deselectAllProcesses = () => {
    setSelectedProcesses(new Set());
  };

  /**
   * Toggle all processes for a specific host
   * 切换特定主机下所有进程的选择
   */
  const toggleHostProcesses = (hostId: number) => {
    const hostProcs = discoveredProcessesByHost.get(hostId) || [];
    const hostKeys = hostProcs.map((p) => getProcessKey(hostId, p));
    const allSelected = hostKeys.every((k) => selectedProcesses.has(k));
    const next = new Set(selectedProcesses);
    if (allSelected) {
      hostKeys.forEach((k) => next.delete(k));
    } else {
      hostKeys.forEach((k) => next.add(k));
    }
    setSelectedProcesses(next);
    setFormErrors((prev) => ({...prev, nodes: undefined}));
  };

  /**
   * Parse selected process key to get hostId, pid, installDir
   * 解析选中的进程键获取主机ID、PID、安装目录
   */
  const parseProcessKey = (key: string) => {
    const parts = key.split('-');
    const hostId = parseInt(parts[0], 10);
    const pid = parseInt(parts[1], 10);
    const installDir = parts.slice(2).join('-'); // install_dir may contain '-'
    return {hostId, pid, installDir};
  };

  /**
   * Get selected processes with host info
   * 获取选中的进程及其主机信息
   */
  const getSelectedProcessesWithHost = () => {
    const result: Array<{
      hostId: number;
      pid: number;
      role: string;
      installDir: string;
      hazelcastPort: number;
      apiPort: number;
      clusterName?: string;
      processKey: string;
      javaHome?: string;
    }> = [];

    selectedProcesses.forEach((key) => {
      const {hostId, pid, installDir} = parseProcessKey(key);
      const processes = discoveredProcessesByHost.get(hostId);
      if (processes) {
        const proc = processes.find((p) => p.pid === pid && p.install_dir === installDir);
        if (proc) {
          result.push({
            hostId,
            pid: proc.pid,
            role: proc.role,
            installDir: proc.install_dir,
            hazelcastPort: proc.hazelcast_port || 0,
            apiPort: proc.api_port || 0,
            clusterName: proc.cluster_name,
            processKey: key,
            javaHome: javaHomesByKey[key] || proc.java_home || '',
          });
        }
      }
    });

    return result;
  };

  /**
   * Inline validation: fill every error at once, then move focus to the first one.
   * 就地校验：一次性填好全部错误，再把焦点移到第一处。
   */
  const validateForm = (selectedCount: number) => {
    const errors: {name?: string; version?: string; nodes?: string} = {};
    if (!name.trim()) {
      errors.name = t('cluster.nameRequired');
    }
    if (!version.trim()) {
      errors.version = t('cluster.versionRequired');
    }
    if (selectedCount === 0) {
      errors.nodes = t('cluster.selectAtLeastOneNode');
    } else if (deploymentMode === DeploymentMode.SEPARATED) {
      const selectedInfo = getSelectedProcessesWithHost();
      const hasMaster = selectedInfo.some((p) => p.role === 'master');
      const hasWorker = selectedInfo.some((p) => p.role === 'worker');
      if (!hasMaster || !hasWorker) {
        errors.nodes = t('cluster.separatedRequiresMasterAndWorker');
      }
    }
    return errors;
  };

  /**
   * Handle submit
   * 处理提交
   */
  const handleSubmit = async () => {
    if (infraTarget === 'kubernetes') {
      const nextErrors: K8sAdoptFormErrors = {};
      if (!k8sForm.host_id) {
        nextErrors.host = t('cluster.k8s.hostRequired');
      }
      if (!k8sForm.namespace.trim()) {
        nextErrors.namespace = t('cluster.k8s.namespaceRequired');
      }
      if (!k8sForm.name.trim()) {
        nextErrors.name = t('cluster.nameRequired');
      }
      setK8sErrors(nextErrors);
      if (nextErrors.host || nextErrors.namespace || nextErrors.name) {
        return;
      }

      setLoading(true);
      try {
        const payload: Record<string, unknown> = {
          name: k8sForm.name.trim(),
          description: k8sForm.description.trim() || undefined,
          host_id: k8sForm.host_id,
          namespace: k8sForm.namespace.trim(),
          management_source: 'adopted',
          master_service_name: k8sForm.master_service_name.trim(),
          cluster_service_name: k8sForm.cluster_service_name.trim(),
          master_workload_name: k8sForm.master_workload_name.trim(),
          worker_workload_name: k8sForm.worker_workload_name.trim(),
          configmap_name: k8sForm.configmap_name.trim(),
          label_selector: k8sForm.label_selector.trim(),
          master_http_port: k8sForm.master_http_port,
          hazelcast_port: k8sForm.hazelcast_port,
        };
        if (k8sForm.master_workload_kind) {
          payload.master_workload_kind = k8sForm.master_workload_kind;
        }
        if (k8sForm.worker_workload_kind) {
          payload.worker_workload_kind = k8sForm.worker_workload_kind;
        }
        if (k8sForm.image.trim()) {
          payload.image = k8sForm.image.trim();
        }
        const created = (await services.cluster.createKubernetesCluster(
          payload,
        )) as
          | {
              cluster?: {id?: number};
              proxy_status?: {
                healthy?: boolean;
                message?: string;
                pod_events?: string[];
              };
            }
          | undefined;
        const proxyStatus = created?.proxy_status;
        if (proxyStatus && !proxyStatus.healthy) {
          // 探活失败不阻断保存，直接提示事件以便在集群详情页一键重试。
          // Proxy probe failure never blocks saving; surface pod events so the user can retry in detail view.
          const events = (proxyStatus.pod_events || []).slice(0, 3).join(' · ');
          toast.warning(
            `${t('cluster.k8s.proxyUnhealthy')}: ${proxyStatus.message || ''}${events ? ` · ${events}` : ''}`,
          );
        } else {
          toast.success(t('cluster.createSuccess'));
        }
        resetForm();
        onSuccess();
      } catch (err) {
        toast.error(
          err instanceof Error ? err.message : t('cluster.createError'),
        );
      } finally {
        setLoading(false);
      }
      return;
    }

    const selectedProcessInfo = getSelectedProcessesWithHost();
    const errors = validateForm(selectedProcessInfo.length);
    setFormErrors(errors);
    if (errors.nodes || errors.name || errors.version) {
      // 按文档顺序聚焦首个错误：节点区在最上方，其次是名称，最后是版本。
      // Focus the first error in document order: the nodes section is on top, then name, then version.
      if (errors.nodes) {
        nodesSectionRef.current?.scrollIntoView({block: 'start'});
      } else if (errors.name) {
        nameInputRef.current?.focus();
      } else {
        versionInputRef.current?.focus();
      }
      return;
    }

    setLoading(true);
    try {
      // Build nodes from selected processes / 从选中的进程构建节点
      // In separated mode, each process (master/worker) is a separate node
      // 在分离模式下，每个进程（master/worker）是一个独立的节点
      // Only merge if same role on same host+installDir (shouldn't happen normally)
      // 只有在同一主机+安装目录上有相同角色时才合并（正常情况下不会发生）
      const nodeMap = new Map<
        string,
        {
          hostId: number;
          installDir: string;
          role: string;
          pids: number[];
          hazelcastPort: number;
          apiPort: number;
          clusterName?: string;
          javaHome?: string;
        }
      >();

      selectedProcessInfo.forEach((proc) => {
        // Key includes role to keep master and worker as separate nodes
        // 键包含角色以保持 master 和 worker 作为独立节点
        const nodeKey = `${proc.hostId}-${proc.installDir}-${proc.role}`;
        const existing = nodeMap.get(nodeKey);
        if (existing) {
          existing.pids.push(proc.pid);
          // Use the port if available / 如果有端口则使用
          if (proc.hazelcastPort > 0) {
            existing.hazelcastPort = proc.hazelcastPort;
          }
          if (proc.apiPort > 0) {
            existing.apiPort = proc.apiPort;
          }
          if (!existing.clusterName && proc.clusterName) {
            existing.clusterName = proc.clusterName;
          }
          if (!existing.javaHome && proc.javaHome) {
            existing.javaHome = proc.javaHome;
          }
        } else {
          nodeMap.set(nodeKey, {
            hostId: proc.hostId,
            installDir: proc.installDir,
            role: proc.role,
            pids: [proc.pid],
            hazelcastPort: proc.hazelcastPort,
            apiPort: proc.apiPort,
            clusterName: proc.clusterName,
            javaHome: proc.javaHome,
          });
        }
      });

      // Convert to nodes array with discovered PID / 转换为包含发现 PID 的节点数组
      const nodes = Array.from(nodeMap.values()).map((node) => ({
        host_id: node.hostId,
        install_dir: node.installDir,
        role: node.role,
        pid: node.pids[0] || undefined,
        hazelcast_port: node.hazelcastPort > 0 ? node.hazelcastPort : undefined,
        api_port: node.apiPort > 0 ? node.apiPort : undefined,
        cluster_name: node.clusterName,
        java_home: node.javaHome || undefined,
      }));

      const effectivePort = Number(proxyPort) > 0 ? Number(proxyPort) : 18080;
      const effectiveJvmOpts = proxyJvmOpts.trim() || '-Xms128m -Xmx512m';

      const data: CreateClusterRequest = {
        name: name.trim(),
        description: description.trim() || undefined,
        deployment_mode: deploymentMode,
        version: version.trim() || undefined,
        install_dir: selectedProcessInfo[0]?.installDir || undefined,
        config: {
          stx_java_proxy: {
            port: effectivePort,
            jvm_opts: effectiveJvmOpts,
            auto_start: autoStartProxy,
          },
          ports: {
            java_proxy_port: effectivePort,
          },
        },
        nodes: nodes.length > 0 ? nodes : undefined,
      };

      const result = await services.cluster.createClusterSafe(data);

      if (result.success) {
        toast.success(t('cluster.createSuccess'));
        // Trigger initial config sync and java-proxy startup in background
        // 后台静默触发首次集群配置拉取与 stx-java-proxy 辅助服务拉起
        if (result.data?.id) {
          const createdClusterId = result.data.id;
          const targetNode =
            selectedProcessInfo.find((p) => p.role === 'master' || p.role === 'hybrid') ||
            selectedProcessInfo[0];
          if (targetNode) {
            services.config
              .initClusterConfigsSafe(createdClusterId, targetNode.hostId, targetNode.installDir)
              .catch(() => {});
          }

          if (autoStartProxy) {
            services.cluster.startStxJavaProxySafe(createdClusterId).catch(() => {});
          }
        }
        resetForm();
        onSuccess();
      } else {
        toast.error(result.error || t('cluster.createError'));
      }
    } finally {
      setLoading(false);
    }
  };

  /**
   * Handle close: confirm first when the form is dirty.
   * 处理关闭：表单有未保存输入时先弹脏态确认。
   */
  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen) {
      if (loading) {
        return;
      }
      if (dirty) {
        setDirtyCloseOpen(true);
        return;
      }
      resetForm();
    }
    onOpenChange(nextOpen);
  };

  /** Discard edits and close / 放弃修改并关闭 */
  const discardAndClose = () => {
    setDirtyCloseOpen(false);
    resetForm();
    onOpenChange(false);
  };

  /**
   * Get process key for selection (includes hostId)
   * 获取进程选择的键（包含主机ID）
   */
  const getProcessKey = (hostId: number, proc: DiscoveredProcess) =>
    `${hostId}-${proc.pid}-${proc.install_dir}`;

  /**
   * Get host name by id
   * 根据ID获取主机名
   */
  const getHostName = (hostId: number) => {
    const host = availableHosts.find((h) => h.id === hostId);
    return host ? `${host.name} (${host.ip_address})` : `Host ${hostId}`;
  };

  const totalDiscoveredProcesses = Array.from(
    discoveredProcessesByHost.values(),
  ).reduce((sum, list) => sum + list.length, 0);

  return (
    <>
      <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        className='arc-compact w-[min(94vw,860px)]!'
        title={t('cluster.registerCluster')}
      >
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center justify-between gap-2 border-b-[0.5px] border-border/80 pb-3'>
            <span className='text-xs font-medium text-foreground'>
              {t('cluster.k8s.targetEnvironment')}
            </span>
            <SegmentedControl
              label={t('cluster.k8s.targetEnvironment')}
              value={infraTarget}
              onValueChange={(value) => {
                setInfraTarget(value as RegisterInfraTarget);
                setFormErrors({});
                setK8sErrors({});
              }}
              options={[
                {value: 'bare_metal', label: t('host.types.bareMetal')},
                {value: 'kubernetes', label: t('host.types.kubernetes')},
              ]}
            />
          </div>

          <div className='max-h-[64vh] space-y-5 overflow-y-auto pr-1'>
            {infraTarget === 'kubernetes' ? (
              <K8sAdoptClusterSection
                active={open && infraTarget === 'kubernetes'}
                form={k8sForm}
                errors={k8sErrors}
                onChange={setK8sForm}
                onClearError={(field) =>
                  setK8sErrors((prev) => ({...prev, [field]: undefined}))
                }
                onNavigateToHosts={() => {
                  resetForm();
                  onOpenChange(false);
                  router.push('/hosts');
                }}
              />
            ) : (
              <>
            {/* Step 1: pick hosts & discover processes / 第一步：选择主机并发现进程 */}
            <section ref={nodesSectionRef} className='space-y-3'>
              <div className='flex items-center justify-between'>
                <span className='text-xs font-medium text-foreground'>
                  {t('discovery.selectHostsToDiscover')}
                </span>
                {availableHosts.length > 0 && (
                  <div className='flex items-center gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={selectAllHosts}
                    >
                      {t('discovery.selectAllHosts')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={deselectAllHosts}
                    >
                      {t('discovery.deselectAllHosts')}
                    </Button>
                  </div>
                )}
              </div>

              {/* Missed-node reminder / 漏选节点提醒 */}
              <Alert
                tone='info'
                title={t('discovery.missedNodesReminderTitle')}
              >
                {t('discovery.missedNodesReminderDesc')}
              </Alert>

              {formErrors.nodes ? (
                <Alert tone='danger' title={formErrors.nodes} />
              ) : null}

              {loadingHosts ? (
                <Skeleton lines={4} label={t('common.loading')} />
              ) : availableHosts.length === 0 ? (
                <EmptyState
                  icon={<Server className='size-5' aria-hidden />}
                  title={t('cluster.noAvailableHosts')}
                  description={t('cluster.noAvailableHostsDesc')}
                  className='py-6'
                  action={
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={() => {
                        resetForm();
                        onOpenChange(false);
                        router.push('/hosts');
                      }}
                    >
                      {t('cluster.goToHostsManagement')}
                    </Button>
                  }
                />
              ) : (
                <div className='space-y-2'>
                  {/* Host list with checkboxes / 带复选框的主机列表 */}
                  <div className='max-h-[150px] divide-y-[0.5px] divide-border/80 overflow-y-auto rounded-md border border-border/80'>
                    {availableHosts.map((host) => {
                      const isSelected = selectedHostIds.has(host.id);
                      return (
                        <div
                          key={host.id}
                          className={`flex cursor-pointer items-center gap-3 p-2 transition-colors hover:bg-muted/50 ${
                            isSelected ? 'bg-primary/10' : ''
                          }`}
                          onClick={() => toggleHostSelection(host.id)}
                        >
                          <Checkbox
                            checked={isSelected}
                            onCheckedChange={() => toggleHostSelection(host.id)}
                            onClick={(e) => e.stopPropagation()}
                            aria-label={host.name}
                          />
                          <Server className='h-4 w-4 text-muted-foreground' />
                          <span className='font-medium'>{host.name}</span>
                          {host.ip_address && (
                            <span className='font-mono text-sm text-muted-foreground tabular-nums'>
                              ({host.ip_address})
                            </span>
                          )}
                        </div>
                      );
                    })}
                  </div>
                  {/* Discover button / 发现按钮 */}
                  <Button
                    type='button'
                    variant='secondary'
                    size='sm'
                    onClick={handleDiscoverProcesses}
                    disabled={selectedHostIds.size === 0 || discoveryLoading}
                    loading={discoveryLoading}
                    className='w-full'
                  >
                    <Search className='size-3.5' aria-hidden />
                    {t('discovery.discoverProcessesOnHosts', {
                      count: selectedHostIds.size,
                    })}
                  </Button>
                </div>
              )}

              {/* Hosts that failed discovery / 发现失败的主机提示 */}
              {failedHostCount > 0 ? (
                <Alert
                  tone='warning'
                  title={t('discovery.someHostsFailed', {count: failedHostCount})}
                />
              ) : null}

              {/* 扫描完成但未发现任何进程时的就地反馈 / Inline feedback when scan succeeds with zero processes */}
              {hasDiscovered &&
              !discoveryLoading &&
              totalDiscoveredProcesses === 0 &&
              failedHostCount === 0 ? (
                <Alert tone='info' title={t('discovery.noProcessesFound')} />
              ) : null}

              {/* Discovered Processes by Host / 按主机分组的发现进程 */}
              {totalDiscoveredProcesses > 0 && (
                <div className='space-y-2'>
                  <div className='flex items-center justify-between'>
                    <span className='text-xs font-medium text-foreground'>
                      {t('discovery.discoveredProcesses')}
                    </span>
                    <div className='flex items-center gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={selectAllProcesses}
                      >
                        {t('discovery.selectAllProcesses')}
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={deselectAllProcesses}
                      >
                        {t('discovery.deselectAllProcesses')}
                      </Button>
                    </div>
                  </div>
                  <div className='max-h-[250px] overflow-y-auto rounded-md border border-border/80'>
                    {Array.from(discoveredProcessesByHost.entries()).map(
                      ([hostId, processes]) => (
                        <div key={hostId} className='border-b last:border-b-0'>
                          {/* Host header / 主机标题 */}
                          <div className='sticky top-0 z-10 flex items-center justify-between bg-muted/50 px-3 py-2'>
                            <div className='flex items-center gap-2'>
                              <Server className='h-4 w-4 text-muted-foreground' />
                              <span className='text-sm font-medium'>
                                {getHostName(hostId)}
                              </span>
                              <span className='text-xs text-muted-foreground tabular-nums'>
                                ({processes.length} {t('discovery.processes')})
                              </span>
                            </div>
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              onClick={(e) => {
                                e.stopPropagation();
                                toggleHostProcesses(hostId);
                              }}
                            >
                              {t('discovery.selectAllOnHost')}
                            </Button>
                          </div>
                          {/* Processes / 进程列表 */}
                          <div className='divide-y-[0.5px] divide-border/80'>
                            {processes.map((proc) => {
                              const key = getProcessKey(hostId, proc);
                              const isSelected = selectedProcesses.has(key);
                              return (
                                <div
                                  key={key}
                                  className={`flex cursor-pointer items-center gap-3 p-3 transition-colors hover:bg-muted/50 ${
                                    isSelected ? 'bg-primary/10' : ''
                                  }`}
                                  onClick={() => toggleProcessSelection(key)}
                                >
                                  <Checkbox
                                    checked={isSelected}
                                    onCheckedChange={() => toggleProcessSelection(key)}
                                    onClick={(e) => e.stopPropagation()}
                                    aria-label={`${getHostName(hostId)} PID ${proc.pid}`}
                                  />
                                  <Cpu className='h-4 w-4 flex-shrink-0 text-muted-foreground' />
                                  <div className='min-w-0 flex-1'>
                                    <div className='flex flex-wrap items-center gap-2'>
                                      <span className='font-mono font-medium tabular-nums'>
                                        PID: {proc.pid}
                                      </span>
                                      <SoftChip
                                        tone={
                                          proc.role === 'master'
                                            ? 'blue'
                                            : proc.role === 'worker'
                                              ? 'emerald'
                                              : 'muted'
                                        }
                                      >
                                        {proc.role}
                                      </SoftChip>
                                      {proc.version &&
                                        proc.version !== 'unknown' && (
                                          <NeutralChip className='font-mono tabular-nums'>
                                            v{proc.version}
                                          </NeutralChip>
                                        )}
                                    </div>
                                    <TruncatedText
                                      text={proc.install_dir}
                                      mode='path'
                                      mono
                                      copyable
                                      className='mt-1 text-xs text-muted-foreground'
                                    />
                                    {/* Port info / 端口信息 */}
                                    <div className='mt-1 flex items-center gap-3 font-mono text-xs text-muted-foreground tabular-nums'>
                                      {proc.hazelcast_port > 0 && (
                                        <span>
                                          Hazelcast: {proc.hazelcast_port}
                                        </span>
                                      )}
                                      {proc.role !== 'worker' &&
                                        proc.api_port > 0 && (
                                          <span>API: {proc.api_port}</span>
                                        )}
                                    </div>
                                    {isSelected && (
                                      <div
                                        className='mt-2'
                                        onClick={(e) => e.stopPropagation()}
                                      >
                                        <JavaHomePicker
                                          hostId={hostId}
                                          seatunnelVersion={version}
                                          value={
                                            javaHomesByKey[key] ||
                                            proc.java_home ||
                                            ''
                                          }
                                          preferredJavaHome={proc.java_home}
                                          onChange={({javaHome}) =>
                                            setJavaHomesByKey((prev) => ({
                                              ...prev,
                                              [key]: javaHome,
                                            }))
                                          }
                                        />
                                      </div>
                                    )}
                                  </div>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      ),
                    )}
                  </div>
                  <p className='text-xs text-muted-foreground tabular-nums'>
                    {t('discovery.selectedCount', {
                      count: selectedProcesses.size,
                    })}
                  </p>
                </div>
              )}
            </section>

            {/* Step 2: cluster basics / 第二步：集群基本信息 */}
            <section className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
              {/* Cluster Name / 集群名称 */}
              <Input
                ref={nameInputRef}
                id='name'
                label={t('cluster.name')}
                required
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setFormErrors((prev) => ({...prev, name: undefined}));
                }}
                placeholder={t('cluster.namePlaceholder')}
                error={formErrors.name}
              />

              {/* Description / 描述 */}
              <div className='space-y-1.5'>
                <label
                  htmlFor='description'
                  className='block text-xs font-medium text-foreground'
                >
                  {t('cluster.descriptionLabel')}
                </label>
                <Textarea
                  id='description'
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t('cluster.descriptionPlaceholder')}
                  rows={2}
                />
              </div>

              {/* Deployment Mode / 部署模式 */}
              <div className='space-y-2'>
                <span className='block text-xs font-medium text-foreground'>
                  {t('cluster.deploymentMode')}{' '}
                  <span className='text-destructive'>*</span>
                </span>
                <SegmentedControl
                  label={t('cluster.deploymentMode')}
                  value={deploymentMode}
                  onValueChange={(value) =>
                    setDeploymentMode(value as DeploymentMode)
                  }
                  options={[
                    {
                      value: DeploymentMode.HYBRID,
                      label: t('cluster.modes.hybrid'),
                    },
                    {
                      value: DeploymentMode.SEPARATED,
                      label: t('cluster.modes.separated'),
                    },
                  ]}
                />
                <p className='text-xs text-muted-foreground'>
                  {deploymentMode === DeploymentMode.HYBRID
                    ? t('cluster.hybridDescription')
                    : t('cluster.separatedDescription')}
                </p>
              </div>

              {/* Version / 版本 */}
              <Input
                ref={versionInputRef}
                id='version'
                label={t('cluster.version')}
                required
                value={version}
                onChange={(e) => {
                  setVersion(e.target.value);
                  setFormErrors((prev) => ({...prev, version: undefined}));
                }}
                placeholder={t('cluster.versionPlaceholder')}
                className='font-mono tabular-nums'
                error={formErrors.version}
              />

              {/* Advanced Settings Section / 高级配置部分 */}
              <div className='border-t-[0.5px] border-border/80 pt-3'>
                <button
                  type='button'
                  className='flex w-full items-center justify-between py-1.5 text-left text-xs font-medium text-foreground hover:text-primary'
                  onClick={() => setShowAdvanced(!showAdvanced)}
                  aria-expanded={showAdvanced}
                >
                  <div className='flex items-center gap-2'>
                    <SlidersHorizontal className='h-3.5 w-3.5 text-muted-foreground' />
                    <span>{t('cluster.advancedSettings')}</span>
                    <span className='font-mono text-[10px] text-muted-foreground'>
                      (STX Java Proxy)
                    </span>
                  </div>
                  {showAdvanced ? (
                    <ChevronDown className='h-3.5 w-3.5 text-muted-foreground' />
                  ) : (
                    <ChevronRight className='h-3.5 w-3.5 text-muted-foreground' />
                  )}
                </button>

                {/* 平滑展开：grid-rows 过渡避免高度跳变 / Smooth expand via grid-rows transition */}
                <div
                  className={`grid transition-all duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] ${
                    showAdvanced
                      ? 'grid-rows-[1fr] opacity-100'
                      : 'grid-rows-[0fr] opacity-0'
                  }`}
                >
                  <div className='overflow-hidden'>
                    <div className='space-y-4 pt-3'>
                      {/* Proxy Service Description & Toggle / 代理服务说明与开关 */}
                      <div className='space-y-2'>
                        <div className='flex items-center gap-2'>
                          <span className='text-xs font-semibold text-foreground'>
                            {t('cluster.stxJavaProxy.title')}
                          </span>
                          <HelpHint
                            label={t('cluster.stxJavaProxy.title')}
                            content={t('cluster.stxJavaProxyDesc')}
                            data-testid='register-cluster-java-proxy-hint'
                          />
                          <NeutralChip>
                            {t('cluster.wizard.modeBadgeProduction')}
                          </NeutralChip>
                        </div>
                        <div className='flex items-center gap-2 pt-1'>
                          <Checkbox
                            id='auto-start-proxy'
                            checked={autoStartProxy}
                            onCheckedChange={(checked) =>
                              setAutoStartProxy(checked === true)
                            }
                            label={t('cluster.autoStartProxy')}
                          />
                          <HelpHint
                            label={t('cluster.autoStartProxy')}
                            content={t('cluster.autoStartProxyHelp')}
                            data-testid='register-cluster-auto-start-proxy-hint'
                          />
                        </div>
                      </div>

                      {/* Proxy Port / 代理端口 */}
                      <Input
                        id='proxy-port'
                        type='number'
                        label={t('cluster.proxyPort')}
                        description={t('cluster.proxyPortHelp')}
                        value={proxyPort}
                        onChange={(e) =>
                          setProxyPort(parseInt(e.target.value, 10) || 0)
                        }
                        min={1024}
                        max={65535}
                        placeholder={t('cluster.proxyPortPlaceholder') || '18080'}
                        className='font-mono tabular-nums'
                      />

                      {/* JVM Options / JVM 参数与预设 */}
                      <div className='space-y-2'>
                        <span className='block text-xs font-medium text-foreground'>
                          {t('cluster.stxJavaProxy.presetLabel')}
                        </span>
                        <SegmentedControl
                          label={t('cluster.stxJavaProxy.presetLabel')}
                          value={proxyJvmPreset}
                          onValueChange={(value) => {
                            setProxyJvmPreset(value);
                            const presetOpts: Record<string, string> = {
                              '512m': '-Xms128m -Xmx512m',
                              '1024m': '-Xms256m -Xmx1024m',
                              '2048m': '-Xms512m -Xmx2048m',
                            };
                            if (presetOpts[value]) {
                              setProxyJvmOpts(presetOpts[value]);
                            }
                          }}
                          options={[
                            {value: '512m', label: '512M'},
                            {value: '1024m', label: '1024M'},
                            {value: '2048m', label: '2048M'},
                            {
                              value: 'custom',
                              label: t('cluster.stxJavaProxy.presetCustom'),
                            },
                          ]}
                        />
                        <Input
                          id='proxy-jvm-opts'
                          label={t('cluster.proxyJvmOpts')}
                          value={proxyJvmOpts}
                          onChange={(e) => {
                            setProxyJvmOpts(e.target.value);
                            setProxyJvmPreset('custom');
                          }}
                          placeholder='-Xms128m -Xmx512m'
                          className='font-mono text-xs'
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </section>
              </>
            )}
          </div>

          <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => handleClose(false)}
              disabled={loading}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={handleSubmit}
              loading={loading}
            >
              {t('cluster.registerCluster')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
    <DirtyCloseDialog
      open={dirtyCloseOpen}
      onDiscard={discardAndClose}
      onKeepEditing={() => setDirtyCloseOpen(false)}
    />
    </>
  );
}
