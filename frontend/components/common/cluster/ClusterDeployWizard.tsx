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

import React, {useState, useCallback, useEffect, useMemo} from 'react';
import {useTranslations} from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {ChevronLeft, ChevronRight, X, PlayCircle, Layers} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import {usePackages} from '@/hooks/use-installer';
import {
  buildSeatunnelInstallDir,
  resolveSeatunnelVersion,
  resolveSeatunnelVersionCapabilities,
} from '@/lib/seatunnel-version';
import {mapCheckpointNamespaceToImap} from '@/lib/runtime-storage-namespace';
import {HostType, HostStatus} from '@/lib/services/host/types';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';

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
  const [currentStepIndex, setCurrentStepIndex] = useState(0);
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

  // Detailed deploy steps state / 详细部署步骤状态
  const [deploySteps, setDeploySteps] = useState<DeployStepItem[]>([]);

  // Packages hook / 安装包 hook
  const {packages, loading: packagesLoading} = usePackages();

  // Precheck state / 预检查状态
  const [precheckResults, setPrecheckResults] = useState<HostPrecheckResult[]>(
    [],
  );
  const [precheckRunning, setPrecheckRunning] = useState(false);

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
  const currentStep = WIZARD_STEPS[currentStepIndex];

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
        enable_http: versionCapabilities.supports_http_service
          ? prev.runtime.enable_http
          : false,
        job_log_mode: versionCapabilities.supports_job_log_mode
          ? prev.runtime.job_log_mode ||
            versionCapabilities.default_job_log_mode
          : 'mixed',
      },
    }));
  }, [versionCapabilities]);

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

  // Validation check before proceeding / 步骤推进前置条件校验
  const canProceed = useCallback(() => {
    const hasPluginProfileSelectionIssue = config.selectedPlugins.some(
      (pluginName) =>
        pluginName === 'jdbc' &&
        (config.selectedPluginProfiles[pluginName] || []).length === 0,
    );

    switch (currentStep.id) {
      case 'basic':
        return config.name.trim().length > 0 && config.version.length > 0;
      case 'hosts':
        if (selectedHosts.length === 0) {
          return false;
        }
        if (config.deploymentMode === DeploymentMode.SEPARATED) {
          const hasMaster = selectedHosts.some((h) =>
            h.roles.includes(NodeRole.MASTER),
          );
          const hasWorker = selectedHosts.some((h) =>
            h.roles.includes(NodeRole.WORKER),
          );
          return hasMaster && hasWorker;
        }
        return true;
      case 'config':
        return (
          config.version.length > 0 &&
          config.installDir.trim().length > 0 &&
          Boolean(config.checkpoint.namespace?.trim())
        );
      case 'precheck':
        return allPrechecksPassed;
      case 'plugins':
        return !hasPluginProfileSelectionIssue;
      case 'deploy':
        return deployStatus === 'success';
      case 'complete':
        return true;
      default:
        return false;
    }
  }, [
    currentStep.id,
    config,
    selectedHosts,
    deployStatus,
    allPrechecksPassed,
  ]);

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
        await services.cluster.addNodeSafe(clusterId, {
          host_id: host.id,
          role,
          install_dir: config.installDir,
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
        const installResult = await services.installer.startInstallation(
          host.id,
          {
            cluster_id: String(clusterId),
            version: config.version,
            install_dir: config.installDir,
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
      setCurrentStepIndex(6); // complete step
    } catch (err) {
      setDeployStatus('failed');
      setDeployError(err instanceof Error ? err.message : 'Deployment failed');
      toast.error(t('cluster.wizard.deployFailed'));
    } finally {
      setDeploying(false);
    }
  }, [config, selectedHosts, deployNodes, t, createdClusterId]);

  // Handle next step / 处理进入下一步
  const handleNext = useCallback(() => {
    if (currentStep.id === 'plugins') {
      setCurrentStepIndex(currentStepIndex + 1);
      handleDeploy();
    } else if (currentStepIndex < WIZARD_STEPS.length - 1) {
      setCurrentStepIndex(currentStepIndex + 1);
    }
  }, [currentStep.id, currentStepIndex, handleDeploy]);

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

  // Handle close dialog / 处理关闭弹窗
  const handleClose = useCallback(() => {
    if (deploying) {
      if (!confirm(t('cluster.wizard.confirmCancel'))) {
        return;
      }
    }
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
    onOpenChange(false);
  }, [deploying, t, onOpenChange]);

  // Handle complete callback / 处理完成并查看集群
  const handleComplete = useCallback(() => {
    if (createdClusterId) {
      onComplete?.(createdClusterId);
    }
    handleClose();
  }, [createdClusterId, onComplete, handleClose]);

  // Render current step component / 渲染当前步骤组件
  const renderStepContent = () => {
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
            applyCheckpointToImap={applyCheckpointToImap}
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
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className='w-full sm:max-w-4xl lg:max-w-5xl h-[86vh] max-h-[850px] p-0 gap-0 overflow-hidden flex flex-col border shadow-2xl rounded-xl bg-background'>
        {/* Header / 弹窗头部 */}
        <DialogHeader className='px-6 py-3.5 border-b bg-muted/20 flex flex-row items-center justify-between space-y-0'>
          <div className='flex items-center gap-2.5'>
            <div className='p-1.5 rounded-md bg-primary/10 text-primary'>
              <Layers className='h-4 w-4' />
            </div>
            <div>
              <DialogTitle className='text-sm font-semibold'>
                {t('cluster.wizard.title')}
              </DialogTitle>
              <DialogDescription className='sr-only'>
                {t(currentStep.descKey)}
              </DialogDescription>
            </div>
          </div>
          <div className='flex items-center gap-2'>
            <Badge variant='outline' className='text-[11px] font-mono h-5 px-2'>
              步骤 {currentStepIndex + 1} / {WIZARD_STEPS.length}
            </Badge>
          </div>
        </DialogHeader>

        {/* Stepper bar / 现代化步骤条 */}
        <ClusterWizardStepper
          steps={WIZARD_STEPS}
          currentStepIndex={currentStepIndex}
          onStepClick={(index) => setCurrentStepIndex(index)}
          deploying={deploying}
        />

        {/* Step Content / 步骤主体内容区 */}
        <div className='flex-1 overflow-hidden min-h-0 px-6 py-4'>
          {renderStepContent()}
        </div>

        {/* Footer / 底部操作栏 */}
        {currentStep.id !== 'complete' && (
          <div className='border-t px-6 py-3 bg-muted/10 flex items-center justify-between shrink-0'>
            <Button
              variant='outline'
              size='sm'
              onClick={handlePrevious}
              disabled={currentStepIndex === 0 || currentStep.id === 'deploy'}
              className='h-8 text-xs'
            >
              <ChevronLeft className='h-3.5 w-3.5 mr-1' />
              {t('common.previous')}
            </Button>

            <div className='flex items-center gap-2'>
              <Button
                variant='ghost'
                size='sm'
                onClick={handleClose}
                disabled={deploying}
                className='h-8 text-xs'
              >
                <X className='h-3.5 w-3.5 mr-1' />
                {t('common.cancel')}
              </Button>

              {currentStep.id !== 'deploy' && (
                <Button
                  size='sm'
                  onClick={handleNext}
                  disabled={!canProceed() || deploying}
                  className='h-8 text-xs'
                >
                  {currentStep.id === 'plugins' ? (
                    <>
                      {t('cluster.wizard.startDeploy')}
                      <PlayCircle className='h-3.5 w-3.5 ml-1.5' />
                    </>
                  ) : (
                    <>
                      {t('common.next')}
                      <ChevronRight className='h-3.5 w-3.5 ml-1.5' />
                    </>
                  )}
                </Button>
              )}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

export default ClusterDeployWizard;
