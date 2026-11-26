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

/**
 * Cluster Deploy Wizard Component
 * 集群部署向导组件
 *
 * Multi-step wizard for deploying a new SeaTunnel cluster.
 * Refactored to follow STX console aesthetic guidelines, dialog hierarchy, and anti-noise design.
 * 多步骤向导，用于部署新的 SeaTunnel 集群。
 * 遵循 STX 控制台设计美学规范、弹窗分级标准与反冗余说明哲学。
 */

'use client';

import React, {useState, useCallback, useEffect, useMemo, useRef} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Button} from '@/components/arc/button/button';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  ConfirmDialog,
  DirtyCloseDialog,
  NeutralChip,
  useBeforeUnload,
} from '@/components/common/layout';
import {DisabledReasonTooltip} from '@/components/common/DisabledReasonTooltip';
import {motion, useReducedMotion} from 'motion/react';
import {ChevronLeft, ChevronRight, X, PlayCircle, Layers} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {usePackages} from '@/hooks/use-installer';
import {
  buildSeatunnelInstallDir,
  nextEventChannelEnabled,
  resolveSeatunnelVersion,
  resolveSeatunnelVersionCapabilities,
} from '@/lib/seatunnel-version';
import {mapCheckpointNamespaceToImap} from '@/lib/runtime-storage-namespace';
import {isValidClusterCode} from '@/lib/cluster-code';
import {HostType, HostStatus} from '@/lib/services/host/types';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';
import {
  heapNextDisabledKey,
  validateHeapSizeMB,
} from '@/lib/services/installer/heap';

import {
  WIZARD_STEPS,
  HostWithRole,
  HostPrecheckResult,
  DeployStepItem,
  ClusterDeployConfig,
  defaultClusterDeployConfig,
} from './wizard/types';
import {ClusterWizardStepper} from './wizard/ClusterWizardStepper';
import {ClusterBasicStep} from './wizard/ClusterBasicStep';
import {ClusterHostsStep} from './wizard/ClusterHostsStep';
import {ClusterConfigStep} from './wizard/ClusterConfigStep';
import {ClusterPrecheckStep} from './wizard/ClusterPrecheckStep';
import {ClusterPluginsStep} from './wizard/ClusterPluginsStep';
import {ClusterDeployStep} from './wizard/ClusterDeployStep';
import {ClusterCompleteStep} from './wizard/ClusterCompleteStep';
import {
  INITIAL_K8S_WIZARD_FORM,
  K8S_WIZARD_STEPS,
  type K8sWizardFormState,
} from './wizard/types';
import {
  K8sBasicContent,
  K8sConfigContent,
  K8sPrecheckContent,
  buildK8sCreatePayload,
  jvmHeapToMi,
  memoryToMi,
} from './wizard/K8sWizardSteps';
import {K8sResourcesStep} from './wizard/K8sResourcesStep';

type DeployInfraTarget = 'bare_metal' | 'kubernetes';

export interface ClusterDeployWizardProps {
  /** Whether the dialog is open / 对话框是否打开 */
  open: boolean;
  /** Callback when dialog open state changes / 对话框打开状态变化时的回调 */
  onOpenChange: (open: boolean) => void;
  /** Callback when deployment completes / 部署完成时的回调 */
  onComplete?: (clusterId: number) => void;
}

export function ClusterDeployWizard({
  open,
  onOpenChange,
  onComplete,
}: ClusterDeployWizardProps) {
  const t = useTranslations();
  const router = useRouter();
  const reducedMotion = useReducedMotion() ?? false;
  const [infraTarget, setInfraTarget] =
    useState<DeployInfraTarget>('bare_metal');
  const [k8sForm, setK8sForm] = useState<K8sWizardFormState>(
    INITIAL_K8S_WIZARD_FORM,
  );
  const [k8sDeploying, setK8sDeploying] = useState(false);
  const [currentStepIndex, setCurrentStepIndex] = useState(0);
  // 记录上一步索引以决定过渡方向 / Track the previous index for the slide direction
  const prevStepRef = useRef(0);
  const stepDirection = currentStepIndex < prevStepRef.current ? 'back' : 'forward';
  useEffect(() => {
    prevStepRef.current = currentStepIndex;
  }, [currentStepIndex]);
  const [config, setConfig] = useState<ClusterDeployConfig>(
    defaultClusterDeployConfig,
  );
  const [hostsWithRole, setHostsWithRole] = useState<HostWithRole[]>([]);
  const [loadingHosts, setLoadingHosts] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [deployProgress, setDeployProgress] = useState(0);
  const [deployStatus, setDeployStatus] = useState<
    'idle' | 'running' | 'success' | 'failed'
  >('idle');
  const [deployError, setDeployError] = useState<string | null>(null);
  const [deployWarnings, setDeployWarnings] = useState<string[]>([]);
  const [createdClusterId, setCreatedClusterId] = useState<number | null>(null);

  // Controlled close-confirmation state / 受控关闭确认状态
  const [deployCancelOpen, setDeployCancelOpen] = useState(false);
  const [dirtyCloseOpen, setDirtyCloseOpen] = useState(false);

  // Detailed deploy steps state / 详细部署步骤状态
  const [deploySteps, setDeploySteps] = useState<DeployStepItem[]>([]);

  // Packages hook / 安装包 hook
  const {packages, loading: packagesLoading} = usePackages();

  // Precheck state / 预检查状态
  const [precheckResults, setPrecheckResults] = useState<HostPrecheckResult[]>(
    [],
  );
  const [precheckRunning, setPrecheckRunning] = useState(false);
  // K8s 预检结果提升到向导：未通过/未运行时 precheck 步禁止进入部署。
  // The Kubernetes precheck outcome is lifted into the wizard: the precheck step
  // blocks advancing to deploy until it has run and passed.
  const [k8sPrecheckPassed, setK8sPrecheckPassed] = useState<boolean | null>(null);
  // 表单关键输入变化后预检结果作废，必须重新运行。
  // Precheck results go stale once key inputs change and must be re-run.
  useEffect(() => {
    setK8sPrecheckPassed(null);
  }, [
    k8sForm.host_id,
    k8sForm.namespace,
    k8sForm.cluster_code,
    k8sForm.name,
    k8sForm.deployment_mode,
    k8sForm.master_memory,
    k8sForm.worker_memory,
    k8sForm.hybrid_memory,
    k8sForm.jvm_xmx,
    k8sForm.event_report_enabled,
  ]);

  // Load available hosts / 加载可用主机
  const loadHosts = useCallback(async () => {
    setLoadingHosts(true);
    try {
      const result = await services.host.getHostsSafe({current: 1, size: 100});
      if (result.success && result.data) {
        // Filter hosts with Agent installed / 过滤已安装 Agent 的主机
        const availableHosts = (result.data.hosts || []).filter(
          (h) =>
            h.host_type === HostType.BARE_METAL &&
            h.status === HostStatus.CONNECTED,
        );
        setHostsWithRole(
          availableHosts.map((host) => ({
            host,
            selected: false,
            // Hybrid: single MASTER_WORKER; separated: default Worker / 混合：单一 MASTER_WORKER；分离：默认 Worker
            roles:
              config.deploymentMode === DeploymentMode.HYBRID
                ? [NodeRole.MASTER_WORKER]
                : [NodeRole.WORKER],
          })),
        );
      }
    } catch (err) {
      console.error('Failed to load hosts:', err);
    } finally {
      setLoadingHosts(false);
    }
  }, [config.deploymentMode]);

  // Load hosts when dialog opens / 对话框打开时加载主机
  useEffect(() => {
    if (open) {
      loadHosts();
    }
  }, [open, loadHosts]);

  // Current step / 当前步骤
  // K8s 跳过 plugins 步骤，统一 6 步骨架。
  // Kubernetes skips the plugins step in the unified 6-step skeleton.
  const wizardSteps = infraTarget === 'kubernetes' ? K8S_WIZARD_STEPS : WIZARD_STEPS;
  const currentStep = wizardSteps[currentStepIndex];

  // Selected hosts / 已选择的主机
  const selectedHosts = useMemo(
    () => hostsWithRole.filter((h) => h.selected),
    [hostsWithRole],
  );

  // Deploy nodes: one entry per (host, role) / 部署节点列表：每个 (host, role) 一条
  const deployNodes = useMemo(() => {
    if (config.deploymentMode === DeploymentMode.HYBRID) {
      return selectedHosts.map((h) => ({
        host: h.host,
        role: NodeRole.MASTER_WORKER,
      }));
    }
    return selectedHosts.flatMap((h) =>
      (h.roles as NodeRole[]).map((role) => ({host: h.host, role})),
    );
  }, [config.deploymentMode, selectedHosts]);

  // Local packages for offline mode / 离线模式的本地安装包
  const localPackages = useMemo(
    () => packages?.local_packages || [],
    [packages?.local_packages],
  );

  const resolvedRecommendedVersion = useMemo(
    () => resolveSeatunnelVersion(packages),
    [packages],
  );
  const versionCapabilities = useMemo(
    () => resolveSeatunnelVersionCapabilities(packages, config.version),
    [packages, config.version],
  );
  const httpServiceSupported = Boolean(
    versionCapabilities?.supports_http_service,
  );
  const eventChannelSupported = Boolean(
    versionCapabilities?.supports_event_channel,
  );
  // 用户手动改过事件通道之后，切换仍支持的版本不再把默认值盖回去。
  // After the user toggles the event channel, later supported versions keep that choice.
  const eventChannelUserSet = useRef(false);
  // 用户手动改过 REST API 开关后，切回支持版本不再重置默认 true。
  // After the user toggles the REST API switch, switching back to a supported
  // version must not reset the default.
  const httpServiceUserSet = useRef(false);

  useEffect(() => {
    if (!open || !resolvedRecommendedVersion || config.version) {
      return;
    }

    // 仅在仍是默认安装路径时随推荐版本写入；用户已改成 /tmp/... 等自定义路径时不得覆盖。
    // Only seed installDir when it still looks like the default; never overwrite a user-edited path such as /tmp/...
    setConfig((prev) => {
      const defaultTemplate = buildSeatunnelInstallDir();
      const shouldResetInstallDir =
        !prev.installDir ||
        prev.installDir === defaultTemplate ||
        prev.installDir === buildSeatunnelInstallDir(prev.version);

      return {
        ...prev,
        version: resolvedRecommendedVersion,
        installDir: shouldResetInstallDir
          ? buildSeatunnelInstallDir(resolvedRecommendedVersion)
          : prev.installDir,
      };
    });
  }, [open, resolvedRecommendedVersion, config.version]);

  useEffect(() => {
    if (!versionCapabilities) {
      return;
    }
    setConfig((prev) => ({
      ...prev,
      runtime: {
        ...prev.runtime,
        enable_http: nextEventChannelEnabled(
          versionCapabilities.supports_http_service,
          prev.runtime.enable_http,
          httpServiceUserSet.current,
        ),
        enable_event_channel: nextEventChannelEnabled(
          versionCapabilities.supports_event_channel,
          prev.runtime.enable_event_channel,
          eventChannelUserSet.current,
        ),
        // New installs default to per_job via defaultClusterDeployConfig + backend
        // default_job_log_mode; unsupported versions stay mixed.
        // 新安装通过 defaultClusterDeployConfig 与后端 default_job_log_mode 默认为 per_job；
        // 不支持的版本保持 mixed。
        job_log_mode: versionCapabilities.supports_job_log_mode
          ? prev.runtime.job_log_mode ||
            versionCapabilities.default_job_log_mode ||
            'per_job'
          : 'mixed',
      },
    }));
  }, [versionCapabilities]);


  /** Update one host's JDK path (AC1) / 更新单个主机的 JDK 路径（AC1） */
  const setHostJavaHome = useCallback(
    (hostId: number, javaHome: string, isAutoPrefill?: boolean) => {
      setHostsWithRole((prev) =>
        prev.map((h) =>
          h.host.id === hostId
            ? {...h, javaHome, javaHomeAutoPrefilled: Boolean(isAutoPrefill)}
            : h,
        ),
      );
    },
    [],
  );

  /** Apply one JDK path to all selected hosts (AC1a) / 一键把同一路径填到全部已选主机（AC1a） */
  const fillAllJavaHome = useCallback((javaHome: string) => {
    setHostsWithRole((prev) =>
      prev.map((h) =>
        h.selected ? {...h, javaHome, javaHomeAutoPrefilled: false} : h,
      ),
    );
  }, []);

  // 目标版本变更后清除自动预填的 JDK（如 3.0.0→3.1.0 清除误留的 JDK 8），切回配置步骤时按新版本重选；手动选择不受影响。
  // When target version switches, reset auto-prefilled JDKs so Step 3 re-selects for the new version while manual picks stay intact.
  const prevVersionRef = useRef<string>('');
  useEffect(() => {
    const currentVersion = config.version.trim();
    if (!currentVersion) {
      return;
    }
    if (prevVersionRef.current && prevVersionRef.current !== currentVersion) {
      setHostsWithRole((prev) =>
        prev.map((h) =>
          h.javaHomeAutoPrefilled
            ? {...h, javaHome: '', javaHomeAutoPrefilled: false}
            : h,
        ),
      );
    }
    prevVersionRef.current = currentVersion;
  }, [config.version]);

  // Run precheck for selected hosts / 运行选定主机的环境预检查
  const runPrecheck = useCallback(async () => {
    if (selectedHosts.length === 0) {
      return;
    }

    setPrecheckRunning(true);

    const initialResults: HostPrecheckResult[] = selectedHosts.map((h) => ({
      hostId: h.host.id,
      hostName: h.host.name,
      loading: true,
      result: null,
      error: null,
    }));
    setPrecheckResults(initialResults);

    const portsToCheck =
      config.deploymentMode === DeploymentMode.SEPARATED
        ? [
            config.clusterPort,
            config.workerPort,
            config.javaProxyPort,
            ...(httpServiceSupported && config.runtime.enable_http
              ? [config.httpPort]
              : []),
          ]
        : [
            config.clusterPort,
            config.javaProxyPort,
            ...(httpServiceSupported && config.runtime.enable_http
              ? [config.httpPort]
              : []),
          ];

    const promises = selectedHosts.map(async (hostWithRole) => {
      try {
        const result = await services.installer.runPrecheck(
          hostWithRole.host.id,
          {
            install_dir: config.installDir,
            ports: portsToCheck,
            java_home: hostWithRole.javaHome || undefined,
            target_version: config.version || undefined,
          },
        );
        return {
          hostId: hostWithRole.host.id,
          hostName: hostWithRole.host.name,
          loading: false,
          result,
          error: null,
        };
      } catch (err) {
        return {
          hostId: hostWithRole.host.id,
          hostName: hostWithRole.host.name,
          loading: false,
          result: null,
          error: err instanceof Error ? err.message : 'Precheck failed',
        };
      }
    });

    const results = await Promise.all(promises);
    setPrecheckResults(results);
    setPrecheckRunning(false);
  }, [
    selectedHosts,
    config.installDir,
    config.clusterPort,
    config.httpPort,
    config.workerPort,
    config.javaProxyPort,
    config.deploymentMode,
    config.runtime.enable_http,
    config.version,
    httpServiceSupported,
  ]);

  // Check if all prechecks passed / 检查是否所有预检查都通过
  const allPrechecksPassed = useMemo(() => {
    if (precheckResults.length === 0 || precheckRunning) {
      return false;
    }
    return precheckResults.every(
      (r) =>
        r.result &&
        (r.result.overall_status === 'passed' ||
          r.result.overall_status === 'warning'),
    );
  }, [precheckResults, precheckRunning]);

  const precheckHasRun = useMemo(() => {
    return precheckResults.length > 0 && !precheckRunning;
  }, [precheckResults, precheckRunning]);

  // Update config / 更新配置
  const updateConfig = useCallback((updates: Partial<ClusterDeployConfig>) => {
    setConfig((prev) => {
      if (
        updates.deploymentMode !== undefined &&
        updates.deploymentMode !== prev.deploymentMode
      ) {
        setPrecheckResults([]);
        setPrecheckRunning(false);
      }

      const next: ClusterDeployConfig = {...prev, ...updates};

      // 版本变更时：仅当安装目录仍是「上一版本默认路径」才自动跟着改；用户自定义路径保留。
      // On version change: only rewrite installDir when it still matches the previous default path.
      if (
        updates.version !== undefined &&
        updates.version !== prev.version &&
        updates.installDir === undefined
      ) {
        const previousDefaultDir = buildSeatunnelInstallDir(prev.version);
        const isDefaultInstallDir =
          !prev.installDir ||
          prev.installDir === previousDefaultDir ||
          prev.installDir === buildSeatunnelInstallDir();
        if (isDefaultInstallDir) {
          next.installDir = buildSeatunnelInstallDir(updates.version);
        } else if (prev.version && prev.installDir.includes(prev.version)) {
          // 路径里带了旧版本号时，只替换版本段，例如 /tmp/seatunnel-2.3.12 → /tmp/seatunnel-2.3.13
          // When the custom path embeds the old version, swap only that segment.
          next.installDir = prev.installDir.replace(prev.version, updates.version);
        }
      }

      return next;
    });
  }, []);

  // Synchronize checkpoint settings to IMAP / 将 Checkpoint 存储参数同步到 IMAP（末级目录 checkpoint→imap）
  const applyCheckpointToImap = useCallback(() => {
    updateConfig({
      imap: {
        storage_type: config.checkpoint.storage_type,
        namespace: mapCheckpointNamespaceToImap(config.checkpoint.namespace),
        hdfs_namenode_host: config.checkpoint.hdfs_namenode_host,
        hdfs_namenode_port: config.checkpoint.hdfs_namenode_port,
        kerberos_principal: config.checkpoint.kerberos_principal,
        kerberos_keytab_file_path: config.checkpoint.kerberos_keytab_file_path,
        hdfs_ha_enabled: config.checkpoint.hdfs_ha_enabled,
        hdfs_name_services: config.checkpoint.hdfs_name_services,
        hdfs_ha_namenodes: config.checkpoint.hdfs_ha_namenodes,
        hdfs_namenode_rpc_address_1:
          config.checkpoint.hdfs_namenode_rpc_address_1,
        hdfs_namenode_rpc_address_2:
          config.checkpoint.hdfs_namenode_rpc_address_2,
        hdfs_failover_proxy_provider:
          config.checkpoint.hdfs_failover_proxy_provider,
        storage_endpoint: config.checkpoint.storage_endpoint,
        storage_access_key: config.checkpoint.storage_access_key,
        storage_secret_key: config.checkpoint.storage_secret_key,
        storage_bucket: config.checkpoint.storage_bucket,
        hdfs_site_path: config.checkpoint.hdfs_site_path,
        disable_cache: config.checkpoint.disable_cache,
        s3_credentials_provider: config.checkpoint.s3_credentials_provider,
      },
    });
    toast.success(t('installer.runtimeStorage.applyCheckpointToImapSuccess'));
  }, [config.checkpoint, t, updateConfig]);

  // Toggle host selection / 切换主机选中状态
  const toggleHostSelection = useCallback((hostId: number) => {
    setHostsWithRole((prev) =>
      prev.map((h) => (h.host.id === hostId ? {...h, selected: !h.selected} : h)),
    );
    setPrecheckResults([]);
    setPrecheckRunning(false);
  }, []);

  // Toggle host role in separated mode / 分离模式下切换主机角色
  const toggleHostRole = useCallback((hostId: number, role: NodeRole) => {
    setHostsWithRole((prev) =>
      prev.map((h) => {
        if (h.host.id !== hostId) {
          return h;
        }
        const hasRole = h.roles.includes(role);
        if (hasRole && h.roles.length <= 1) {
          return h; // Keep at least one role / 至少保留一个角色
        }
        if (hasRole) {
          return {...h, roles: h.roles.filter((r) => r !== role)};
        }
        return {...h, roles: [...h.roles, role]};
      }),
    );
  }, []);

  /**
   * Reason the Next/Deploy button is blocked, or null when the step can proceed.
   * The same predicate drives DisabledReasonTooltip so the button never just sits dead.
   * 「下一步/开始部署」被禁用的原因；为 null 表示可继续。
   * 与 DisabledReasonTooltip 共用同一谓词，让按钮始终说明不可推进的原因。
   */
  const proceedBlockReason = useCallback((): string | null => {
    // K8s 分步校验：basic 名称+镜像、hosts 主机+namespace、config 堆内存联动红线、precheck 需通过。
    // Kubernetes per-step validation: basic needs name+image, hosts needs host+namespace,
    // config enforces the heap/container-memory red line, precheck must pass.
    if (infraTarget === 'kubernetes') {
      switch (currentStep.id) {
        case 'basic':
          if (
            k8sForm.cluster_code.length > 0 &&
            !isValidClusterCode(k8sForm.cluster_code)
          ) {
            return t('cluster.clusterCodeInvalid');
          }
          return k8sForm.name.trim().length > 0 && k8sForm.image.trim().length > 0
            ? null
            : t('cluster.k8s.nextDisabledK8s');
        case 'hosts':
          return k8sForm.host_id > 0 && k8sForm.namespace.trim().length > 0
            ? null
            : t('cluster.k8s.nextDisabledK8s');
        case 'config':
          return null;
        case 'precheck':
          // 预检未跑完或未通过一律阻断部署按钮，reason 供 tooltip 说明。
          // An unfinished or failed precheck always blocks the deploy advance;
          // the reason feeds the tooltip.
          if (k8sPrecheckPassed === null) {
            return t('cluster.k8s.precheckRunning');
          }
          return k8sPrecheckPassed ? null : t('cluster.k8s.precheckBlocked');
        case 'deploy':
          return deployStatus === 'success' ? null : '';
        case 'complete':
          return null;
        default:
          return null;
      }
    }
    const hasPluginProfileSelectionIssue = config.selectedPlugins.some(
      (pluginName) =>
        pluginName === 'jdbc' &&
        (config.selectedPluginProfiles[pluginName] || []).length === 0,
    );

    switch (currentStep.id) {
      case 'basic':
        if (
          config.clusterCode.length > 0 &&
          !isValidClusterCode(config.clusterCode)
        ) {
          return t('cluster.clusterCodeInvalid');
        }
        return config.name.trim().length > 0 && config.version.length > 0
          ? null
          : t('cluster.wizard.nextDisabledBasic');
      case 'hosts':
        if (selectedHosts.length === 0) {
          return t('cluster.wizard.nextDisabledHosts');
        }
        if (config.deploymentMode === DeploymentMode.SEPARATED) {
          const hasMaster = selectedHosts.some((h) =>
            h.roles.includes(NodeRole.MASTER),
          );
          const hasWorker = selectedHosts.some((h) =>
            h.roles.includes(NodeRole.WORKER),
          );
          return hasMaster && hasWorker
            ? null
            : t('cluster.wizard.nextDisabledRoles');
        }
        return null;
      case 'config': {
        if (
          !(
            config.version.length > 0 &&
            config.installDir.trim().length > 0 &&
            Boolean(config.checkpoint.namespace?.trim())
          )
        ) {
          return t('cluster.wizard.nextDisabledConfig');
        }
        const heapValues =
          config.deploymentMode === DeploymentMode.HYBRID
            ? [config.jvm.hybrid_heap_size]
            : [config.jvm.master_heap_size, config.jvm.worker_heap_size];
        const hostMemoryBytes = selectedHosts
          .map((h) => h.host.total_memory)
          .filter((v): v is number => typeof v === 'number' && v > 0);
        const minHostMemory =
          hostMemoryBytes.length > 0 ? Math.min(...hostMemoryBytes) : undefined;
        for (const value of heapValues) {
          const heapCode = validateHeapSizeMB(value, minHostMemory);
          if (heapCode) {
            return t(heapNextDisabledKey(heapCode));
          }
        }
        return null;
      }
      case 'precheck':
        return allPrechecksPassed
          ? null
          : t('cluster.wizard.nextDisabledPrecheck');
      case 'plugins':
        return hasPluginProfileSelectionIssue
          ? t('cluster.wizard.nextDisabledJdbc')
          : null;
      case 'deploy':
        return deployStatus === 'success' ? null : '';
      case 'complete':
        return null;
      default:
        return '';
    }
  }, [
    currentStep.id,
    config,
    selectedHosts,
    deployStatus,
    allPrechecksPassed,
    infraTarget,
    k8sForm,
    t,, k8sPrecheckPassed]);

  const canProceed = useCallback(() => proceedBlockReason() === null, [proceedBlockReason]);

  // Handle deploy execution / 处理部署执行
  const handleDeploy = useCallback(async () => {
    setDeploying(true);
    setDeployStatus('running');
    setDeployProgress(0);
    setDeployError(null);
    setDeployWarnings([]);
    setDeploySteps([]);

    const updateStep = (
      step: string,
      status: 'pending' | 'running' | 'success' | 'failed',
      message: string,
      hostName?: string,
      progress?: number,
    ) => {
      setDeploySteps((prev) => {
        const existing = prev.find(
          (s) => s.step === step && s.hostName === hostName,
        );
        if (existing) {
          return prev.map((s) =>
            s.step === step && s.hostName === hostName
              ? {...s, status, message, progress}
              : s,
          );
        }
        return [...prev, {step, status, message, hostName, progress}];
      });
    };

    const mergeWarnings = (hostLabel: string, warnings?: string[]) => {
      if (!Array.isArray(warnings) || warnings.length === 0) {
        return;
      }
      setDeployWarnings((prev) => {
        const merged = new Set(prev);
        warnings.forEach((warning) => {
          const trimmed = warning.trim();
          if (trimmed) {
            merged.add(`${hostLabel}: ${trimmed}`);
          }
        });
        return Array.from(merged);
      });
    };

    try {
      let clusterId = createdClusterId;

      // Step 1: Create cluster (skip if already created) / 步骤1：创建集群
      if (!clusterId) {
        updateStep(
          'create_cluster',
          'running',
          t('cluster.wizard.steps.creatingCluster'),
        );
        setDeployProgress(10);
        const clusterResult = await services.cluster.createClusterSafe({
          name: config.name,
          cluster_code: config.clusterCode ? config.clusterCode.trim() : undefined,
          description: config.description || undefined,
          deployment_mode: config.deploymentMode,
          version: config.version,
          install_dir: config.installDir,
          config: {
            runtime: config.runtime,
            jvm: config.jvm,
            checkpoint: config.checkpoint,
            imap: config.imap,
            ports: {
              master_hazelcast_port: config.clusterPort,
              master_api_port: config.httpPort,
              worker_port: config.workerPort,
              java_proxy_port: config.javaProxyPort,
            },
          },
        });

        if (!clusterResult.success || !clusterResult.data) {
          updateStep(
            'create_cluster',
            'failed',
            clusterResult.error || 'Failed to create cluster',
          );
          throw new Error(clusterResult.error || 'Failed to create cluster');
        }

        clusterId = clusterResult.data.id;
        setCreatedClusterId(clusterId);
        updateStep(
          'create_cluster',
          'success',
          t('cluster.wizard.steps.clusterCreated'),
        );
      }
      setDeployProgress(20);

      // Step 2: Add nodes to cluster / 步骤2：添加节点
      updateStep('add_nodes', 'running', t('cluster.wizard.steps.addingNodes'));
      for (let i = 0; i < deployNodes.length; i++) {
        const {host, role} = deployNodes[i];
        const label =
          deployNodes.length > selectedHosts.length
            ? `${host.name} (${role})`
            : host.name;
        updateStep(
          'add_node',
          'running',
          t('cluster.wizard.steps.addingNode'),
          label,
        );
        const hostJava =
          selectedHosts.find((h) => h.host.id === host.id)?.javaHome || '';
        await services.cluster.addNodeSafe(clusterId, {
          host_id: host.id,
          role,
          install_dir: config.installDir,
          java_home: hostJava || undefined,
          hazelcast_port:
            role === 'worker' &&
            config.deploymentMode === DeploymentMode.SEPARATED
              ? config.workerPort
              : config.clusterPort,
          api_port:
            role === NodeRole.MASTER || role === NodeRole.MASTER_WORKER
              ? config.httpPort
              : undefined,
        });
        updateStep(
          'add_node',
          'success',
          t('cluster.wizard.steps.nodeAdded'),
          label,
        );
        setDeployProgress(20 + ((i + 1) / deployNodes.length) * 30);
      }
      updateStep('add_nodes', 'success', t('cluster.wizard.steps.nodesAdded'));

      // Collect master/worker addresses / 收集 master/worker 地址
      const masterAddresses =
        config.deploymentMode === DeploymentMode.HYBRID
          ? deployNodes
              .map((n) => n.host.ip_address || '')
              .filter((ip) => ip !== '')
          : [
              ...new Set(
                deployNodes
                  .filter((n) => n.role === 'master')
                  .map((n) => n.host.ip_address || '')
                  .filter(Boolean),
              ),
            ];
      const workerAddresses =
        config.deploymentMode === DeploymentMode.SEPARATED
          ? [
              ...new Set(
                deployNodes
                  .filter((n) => n.role === 'worker')
                  .map((n) => n.host.ip_address || '')
                  .filter(Boolean),
              ),
            ]
          : [];

      // Step 3: Install SeaTunnel / 步骤3：在各节点上安装 SeaTunnel
      for (let i = 0; i < deployNodes.length; i++) {
        const {host, role} = deployNodes[i];
        const label =
          deployNodes.length > selectedHosts.length
            ? `${host.name} (${role})`
            : host.name;

        updateStep(
          'install',
          'running',
          t('cluster.wizard.steps.startingInstall'),
          label,
          0,
        );
        const hostJavaHome =
          selectedHosts.find((h) => h.host.id === host.id)?.javaHome || '';
        const installResult = await services.installer.startInstallation(
          host.id,
          {
            cluster_id: String(clusterId),
            version: config.version,
            install_dir: config.installDir,
            java_home: hostJavaHome || undefined,
            install_mode: 'online',
            mirror: config.mirror,
            deployment_mode: config.deploymentMode,
            node_role: role,
            master_addresses: masterAddresses,
            worker_addresses: workerAddresses,
            cluster_port: config.clusterPort,
            worker_port: config.workerPort,
            http_port: config.httpPort,
            java_proxy_port: config.javaProxyPort,
            enable_http: config.runtime.enable_http,
            enable_event_channel: eventChannelSupported
              ? config.runtime.enable_event_channel
              : false,
            dynamic_slot: config.runtime.dynamic_slot,
            slot_num: config.runtime.slot_num,
            slot_allocation_strategy: config.runtime.slot_allocation_strategy,
            job_schedule_strategy: config.runtime.job_schedule_strategy,
            history_job_expire_minutes:
              config.runtime.history_job_expire_minutes,
            scheduled_deletion_enable: config.runtime.scheduled_deletion_enable,
            job_log_mode: config.runtime.job_log_mode,
            jvm: config.jvm,
            checkpoint: config.checkpoint,
            imap: config.imap,
            connector:
              config.selectedPlugins.length > 0
                ? {
                    install_connectors: true,
                    connectors: [],
                    selected_plugins: config.selectedPlugins,
                    selected_plugin_profiles: config.selectedPluginProfiles,
                  }
                : undefined,
          },
        );

        let status = installResult;

        const updateStepsFromStatus = () => {
          mergeWarnings(label, status.warnings);
          if (status.steps && status.steps.length > 0) {
            for (const step of status.steps) {
              const stepKey = `${step.step}_${host.id}_${role}`;
              const stepStatus =
                step.status === 'success'
                  ? 'success'
                  : step.status === 'failed'
                    ? 'failed'
                    : step.status === 'running'
                      ? 'running'
                      : 'pending';
              const stepMessage = step.message || step.name || step.step;
              updateStep(
                stepKey,
                stepStatus,
                stepMessage,
                label,
                step.progress || 0,
              );
            }
          }
        };

        while (status.status === 'running') {
          await new Promise((resolve) => setTimeout(resolve, 1000));
          status = await services.installer.getInstallationStatus(host.id);
          updateStepsFromStatus();

          if (!status.steps || status.steps.length === 0) {
            const stepMessage =
              status.message ||
              status.current_step ||
              t('cluster.wizard.steps.installing');
            updateStep(
              'install',
              'running',
              stepMessage,
              label,
              status.progress || 0,
            );
          }
        }

        updateStepsFromStatus();

        if (status.status === 'failed') {
          updateStep(
            'install',
            'failed',
            status.error || t('cluster.wizard.steps.installFailed'),
            label,
          );
          throw new Error(`Installation failed on ${label}: ${status.error}`);
        }

        updateStep(
          'install',
          'success',
          t('cluster.wizard.steps.installComplete'),
          label,
          100,
        );
        setDeployProgress(50 + ((i + 1) / deployNodes.length) * 40);
      }

      // Step 4: Complete / 步骤4：完成
      setDeployProgress(100);
      setDeployStatus('success');
      toast.success(t('cluster.wizard.deploySuccess'));

      await new Promise((resolve) => setTimeout(resolve, 800));
      setCurrentStepIndex(wizardSteps.length - 1); // complete step
    } catch (err) {
      setDeployStatus('failed');
      setDeployError(err instanceof Error ? err.message : 'Deployment failed');
      toast.error(t('cluster.wizard.deployFailed'));
    } finally {
      setDeploying(false);
    }
  }, [config, selectedHosts, deployNodes, t, createdClusterId, eventChannelSupported, wizardSteps.length]);

  // Handle next step / 处理进入下一步
  const handleNext = useCallback(() => {
    if (currentStep.id === 'plugins') {
      setCurrentStepIndex(currentStepIndex + 1);
      handleDeploy();
    } else if (currentStep.id === 'precheck' && infraTarget === 'kubernetes') {
      // K8s 无 plugins 步：precheck 后进入部署步并立即启动部署。
      // Kubernetes has no plugins step: entering the deploy step starts deployment.
      setCurrentStepIndex(currentStepIndex + 1);
    } else if (currentStepIndex < wizardSteps.length - 1) {
      setCurrentStepIndex(currentStepIndex + 1);
    }
  }, [currentStep.id, currentStepIndex, handleDeploy, infraTarget, wizardSteps.length]);

  // Handle previous step / 处理返回上一步
  const handlePrevious = useCallback(() => {
    if (
      currentStepIndex > 0 &&
      currentStep.id !== 'deploy' &&
      currentStep.id !== 'complete'
    ) {
      setCurrentStepIndex(currentStepIndex - 1);
    }
  }, [currentStepIndex, currentStep.id]);

  /**
   * Whether the user has entered anything worth protecting.
   * 用户是否已有需要保护的输入（用于脏态关闭确认）。
   */
  const hasUserInput = useMemo(() => {
    if (infraTarget === 'kubernetes') {
      return (
        k8sForm.name.trim().length > 0 ||
        k8sForm.description.trim().length > 0
      );
    }
    return (
      config.name.trim().length > 0 ||
      config.description.trim().length > 0 ||
      selectedHosts.length > 0 ||
      config.selectedPlugins.length > 0
    );
  }, [
    infraTarget,
    k8sForm.name,
    k8sForm.description,
    config.name,
    config.description,
    selectedHosts.length,
    config.selectedPlugins.length,
  ]);
  const dirtyGuardActive =
    open &&
    hasUserInput &&
    deployStatus !== 'success' &&
    (infraTarget === 'kubernetes'
      ? !k8sDeploying && currentStep.id !== 'complete'
      : currentStep.id !== 'deploy' &&
        currentStep.id !== 'complete' &&
        !deploying);
  useBeforeUnload(dirtyGuardActive);

  /** Reset everything and actually close / 复位全部状态并真正关闭弹窗 */
  const closeNow = useCallback(() => {
    setInfraTarget('bare_metal');
    setK8sForm(INITIAL_K8S_WIZARD_FORM);
    setK8sDeploying(false);
    setCurrentStepIndex(0);
    setConfig(defaultClusterDeployConfig);
    setHostsWithRole([]);
    setDeployStatus('idle');
    setDeployProgress(0);
    setDeployError(null);
    setDeployWarnings([]);
    setCreatedClusterId(null);
    setPrecheckResults([]);
    setPrecheckRunning(false);
    setDeployCancelOpen(false);
    setDirtyCloseOpen(false);
    onOpenChange(false);
    // 成功创建过集群时无论走哪条关闭路径都刷新外层列表，避免手动刷新才能看到新集群。
    // Whenever a cluster was created, refresh the outer list on every close path so
    // the new cluster appears without a manual reload.
    if (createdClusterId) {
      onComplete?.(createdClusterId);
    }
  }, [onOpenChange, createdClusterId, onComplete]);



  const handleK8sDeploy = useCallback(async () => {
    if (k8sDeploying) {
      return;
    }
    setK8sDeploying(true);
    setDeployStatus('running');
    setDeployError(null);
    setDeployProgress(15);
    setDeploySteps([
      {step: 'submit', message: t('cluster.k8s.deployStepSubmit'), status: 'running'},
      {step: 'workloads', message: t('cluster.k8s.deployStepWorkloads'), status: 'pending'},
      {step: 'proxy', message: t('cluster.k8s.deployStepProxy'), status: 'pending'},
    ]);
    try {
      const payload = buildK8sCreatePayload(k8sForm);
      setDeployProgress(45);
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
      setDeploySteps((prev) =>
        prev.map((step) =>
          step.step === 'submit' ? {...step, status: 'success'} : step.step === 'workloads' ? {...step, status: 'success'} : step,
        ),
      );
      setDeployProgress(80);
      const proxyStatus = created?.proxy_status;
      setDeploySteps((prev) =>
        prev.map((step) =>
          step.step === 'proxy'
            ? {...step, status: proxyStatus && !proxyStatus.healthy ? 'failed' : 'success'}
            : step,
        ),
      );
      if (proxyStatus && !proxyStatus.healthy) {
        // 伴生 Proxy 探活异常时不阻断集群创建，展示 Pod 事件便于详情页重试。
        // Companion proxy readiness warning never blocks saving; surface pod events for retry in detail view.
        const events = (proxyStatus.pod_events || []).slice(0, 3).join(' · ');
        setDeployWarnings([
          `${t('cluster.k8s.proxyUnhealthy')}: ${proxyStatus.message || ''}${events ? ` · ${events}` : ''}`,
        ]);
      }
      setDeployProgress(100);
      setDeployStatus('success');
      if (created?.cluster?.id) {
        setCreatedClusterId(created.cluster.id);
      }
      await new Promise((resolve) => setTimeout(resolve, 800));
      setCurrentStepIndex(wizardSteps.length - 1); // complete step
    } catch (err) {
      setDeployStatus('failed');
      setDeployError(
        err instanceof Error ? err.message : t('cluster.wizard.deployFailed'),
      );
      setDeploySteps((prev) =>
        prev.map((step) =>
          step.status === 'running' || step.status === 'pending'
            ? {...step, status: 'failed'}
            : step,
        ),
      );
    } finally {
      setK8sDeploying(false);
    }
  }, [k8sDeploying, k8sForm, t, wizardSteps.length]);

  // 进入 K8s 部署步自动启动部署（仅一次，失败可手动重试）。
  // Entering the Kubernetes deploy step auto-starts deployment once; failures retry manually.
  useEffect(() => {
    if (
      infraTarget === 'kubernetes' &&
      currentStep.id === 'deploy' &&
      deployStatus === 'idle' &&
      !k8sDeploying
    ) {
      void handleK8sDeploy();
    }
  }, [infraTarget, currentStep.id, deployStatus, k8sDeploying, handleK8sDeploy]);

  /**
   * Close requests route through the right confirmation instead of a native confirm().
   * 关闭请求先走对应确认弹窗，替代浏览器原生 confirm()。
   */
  const handleClose = useCallback(() => {
    if (k8sDeploying) {
      return;
    }
    if (deploying) {
      setDeployCancelOpen(true);
      return;
    }
    if (dirtyGuardActive) {
      setDirtyCloseOpen(true);
      return;
    }
    closeNow();
  }, [k8sDeploying, deploying, dirtyGuardActive, closeNow]);

  // Handle complete callback / 处理完成并查看集群
  const handleComplete = useCallback(() => {
    handleClose();
  }, [handleClose]);

  // Render current step component / 渲染当前步骤组件
  const renderStepContent = () => {
    // K8s 各步内容：basic/hosts/config/precheck 走专属组件，deploy/complete 复用共享骨架。
    // Kubernetes step content: basic/hosts/config/precheck use dedicated components
    // while deploy/complete reuse the shared skeleton.
    if (infraTarget === 'kubernetes') {
      const patchK8s = (partial: Partial<K8sWizardFormState>) =>
        setK8sForm((prev) => ({...prev, ...partial}));
      switch (currentStep.id) {
        case 'basic':
          return <K8sBasicContent form={k8sForm} patch={patchK8s} />;
        case 'hosts':
          return (
            <K8sResourcesStep
              form={k8sForm}
              onChange={(updater) => setK8sForm(updater)}
            />
          );
        case 'config':
          return <K8sConfigContent form={k8sForm} patch={patchK8s} />;
        case 'precheck':
          return (
            <K8sPrecheckContent
              form={k8sForm}
              onResult={(result) => setK8sPrecheckPassed(result === null ? null : result.passed)}
            />
          );
        case 'deploy':
          return (
            <ClusterDeployStep
              deployStatus={deployStatus}
              deployProgress={deployProgress}
              deployError={deployError}
              deployWarnings={deployWarnings}
              deploySteps={deploySteps}
              deploying={k8sDeploying}
              onRetry={handleK8sDeploy}
              onBackToPlugins={() => {
                setDeployStatus('idle');
                setDeployProgress(0);
                setDeployError(null);
                setDeployWarnings([]);
                setDeploySteps([]);
                setCurrentStepIndex(3);
              }}
            />
          );
        case 'complete':
          return (
            <ClusterCompleteStep
              config={config}
              selectedHosts={selectedHosts}
              deployWarnings={deployWarnings}
              k8sForm={k8sForm}
              onClose={handleClose}
              onComplete={handleComplete}
            />
          );
        default:
          return null;
      }
    }
    switch (currentStep.id) {
      case 'basic':
        return (
          <ClusterBasicStep
            config={config}
            updateConfig={updateConfig}
            packages={packages}
            localPackages={localPackages}
            packagesLoading={packagesLoading}
            resolvedRecommendedVersion={resolvedRecommendedVersion}
          />
        );
      case 'hosts':
        return (
          <ClusterHostsStep
            hostsWithRole={hostsWithRole}
            loadingHosts={loadingHosts}
            deploymentMode={config.deploymentMode}
            toggleHostSelection={toggleHostSelection}
            toggleHostRole={toggleHostRole}
            selectedHosts={selectedHosts}
          />
        );
      case 'config':
        return (
          <ClusterConfigStep
            config={config}
            updateConfig={updateConfig}
            selectedHosts={selectedHosts}
            localPackages={localPackages}
            versionCapabilities={versionCapabilities}
            httpServiceSupported={httpServiceSupported}
            eventChannelSupported={eventChannelSupported}
            onEventChannelUserSet={() => {
              eventChannelUserSet.current = true;
            }}
            onHttpServiceUserSet={() => {
              httpServiceUserSet.current = true;
            }}
            applyCheckpointToImap={applyCheckpointToImap}
            setHostJavaHome={setHostJavaHome}
            fillAllJavaHome={fillAllJavaHome}
          />
        );
      case 'precheck':
        return (
          <ClusterPrecheckStep
            precheckResults={precheckResults}
            precheckRunning={precheckRunning}
            precheckHasRun={precheckHasRun}
            allPrechecksPassed={allPrechecksPassed}
            runPrecheck={runPrecheck}
            selectedHosts={selectedHosts}
            onBackToConfig={() => setCurrentStepIndex(2)}
          />
        );
      case 'plugins':
        return (
          <ClusterPluginsStep config={config} updateConfig={updateConfig} />
        );
      case 'deploy':
        return (
          <ClusterDeployStep
            deployStatus={deployStatus}
            deployProgress={deployProgress}
            deployError={deployError}
            deployWarnings={deployWarnings}
            deploySteps={deploySteps}
            deploying={deploying}
            onRetry={handleDeploy}
            onBackToPlugins={() => {
              setDeployStatus('idle');
              setDeployProgress(0);
              setDeployError(null);
              setDeployWarnings([]);
              setDeploySteps([]);
              setCurrentStepIndex(4);
            }}
          />
        );
      case 'complete':
        return (
                      <ClusterCompleteStep
            config={config}
            selectedHosts={selectedHosts}
            deployWarnings={deployWarnings}
            onClose={handleClose}
            onComplete={handleComplete}
          />
        );
      default:
        return null;
    }
  };

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            handleClose();
          }
        }}
      >
      <DialogContent
        className='arc-compact w-[min(95vw,1040px)]! h-[86vh]! max-h-[850px]!'
        bodyClassName='p-0! flex min-h-0 flex-1 flex-col overflow-hidden'
        title={
          <span className='flex items-center gap-2.5'>
            <Layers className='size-4 shrink-0 text-primary' aria-hidden />
            <span>{t('cluster.wizard.title')}</span>
            <NeutralChip className='font-mono tabular-nums'>
              {currentStepIndex + 1} / {wizardSteps.length}
            </NeutralChip>
          </span>
        }
        description={t(currentStep.descKey)}
      >
        {/* 目标基础设施切换栏 / Target infrastructure switcher */}
        {!deploying &&
          !k8sDeploying &&
          (infraTarget === 'kubernetes' ||
            (currentStep.id !== 'deploy' && currentStep.id !== 'complete')) && (
            <div className='flex shrink-0 flex-wrap items-center justify-between gap-2 border-b-[0.5px] border-border/80 px-6 py-2.5'>
              <span className='text-xs font-medium text-foreground'>
                {t('cluster.k8s.targetEnvironment')}
              </span>
              <SegmentedControl
                label={t('cluster.k8s.targetEnvironment')}
                value={infraTarget}
                onValueChange={(value) =>
                  setInfraTarget(value as DeployInfraTarget)
                }
                options={[
                  {value: 'bare_metal', label: t('host.types.bareMetal')},
                  {value: 'kubernetes', label: t('host.types.kubernetes')},
                ]}
              />
            </div>
          )}

          <>
            {/* Stepper bar / 现代化步骤条 */}
            <ClusterWizardStepper
              steps={wizardSteps}
              currentStepIndex={currentStepIndex}
              onStepClick={(index) => setCurrentStepIndex(index)}
              deploying={deploying}
              stepErrors={{
                ...(precheckHasRun && !allPrechecksPassed
                  ? {precheck: t('cluster.wizard.precheckFailed')}
                  : {}),
                ...(deployStatus === 'failed'
                  ? {deploy: t('cluster.wizard.deployFailed')}
                  : {}),
              }}
              allComplete={currentStep.id === 'complete'}
              readOnly={
                deploying ||
                currentStep.id === 'deploy' ||
                currentStep.id === 'complete'
              }
            />

            {/* Step Content / 步骤主体内容区（轻量方向性过渡） */}
            {/* 内容超高时需纵向滚动而非裁切：K8s 资源步表单字段多，86vh 内必然溢出。 */}
            {/* Content taller than the viewport must scroll rather than clip: the K8s
                resources step's long form always overflows the 86vh dialog. */}
            <div className='min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-6 py-4'>
              <motion.div
                key={currentStepIndex}
                className='min-h-0'
                initial={
                  reducedMotion
                    ? {opacity: 0}
                    : {opacity: 0, x: stepDirection === 'back' ? -12 : 12}
                }
                animate={{opacity: 1, x: 0}}
                transition={{
                  duration: reducedMotion ? 0 : 0.15,
                  ease: [0.22, 1, 0.36, 1],
                }}
              >
                {renderStepContent()}
              </motion.div>
            </div>

            {/* Footer / 底部操作栏 */}
            {currentStep.id !== 'complete' && (
              <div className='flex shrink-0 items-center justify-between border-t-[0.5px] border-border/80 px-6 py-3'>
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={handlePrevious}
                  disabled={currentStepIndex === 0 || currentStep.id === 'deploy'}
                >
                  <ChevronLeft className='size-3.5' aria-hidden />
                  {t('common.previous')}
                </Button>

                <div className='flex items-center gap-2'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={handleClose}
                  >
                    <X className='size-3.5' aria-hidden />
                    {t('common.cancel')}
                  </Button>

                  {currentStep.id !== 'deploy' && (
                    <DisabledReasonTooltip reason={proceedBlockReason()}>
                      <Button
                        type='button'
                        size='sm'
                        onClick={() => {
                          if (!canProceed() || deploying) {
                            return;
                          }
                          handleNext();
                        }}
                        aria-disabled={!canProceed() || deploying ? 'true' : undefined}
                        className={
                          !canProceed() || deploying ? 'opacity-60' : undefined
                        }
                      >
                        {currentStep.id === 'plugins' ? (
                          <>
                            {t('cluster.wizard.startDeploy')}
                            <PlayCircle className='size-3.5' aria-hidden />
                          </>
                        ) : (
                          <>
                            {t('common.next')}
                            <ChevronRight className='size-3.5' aria-hidden />
                          </>
                        )}
                      </Button>
                    </DisabledReasonTooltip>
                  )}
                </div>
              </div>
            )}
          </>

      </DialogContent>
    </Dialog>

    {/* 部署中关闭确认（替代原生 confirm）/ Deploy-cancel confirm replacing native confirm() */}
    <ConfirmDialog
      open={deployCancelOpen}
      onOpenChange={setDeployCancelOpen}
      title={t('cluster.wizard.confirmCancelTitle')}
      description={t('cluster.wizard.confirmCancelDesc')}
      cancelLabel={t('cluster.wizard.continueDeploy')}
      confirmLabel={t('cluster.wizard.abortAndClose')}
      confirmVariant='danger'
      onConfirm={closeNow}
    />

    {/* 脏态关闭保护 / Dirty-close guard for entered-but-undeployed config */}
    <DirtyCloseDialog
      open={dirtyCloseOpen}
      onDiscard={closeNow}
      onKeepEditing={() => setDirtyCloseOpen(false)}
    />
    </>
  );
}

export default ClusterDeployWizard;
