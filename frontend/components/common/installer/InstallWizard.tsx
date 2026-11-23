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
 * Install Wizard Component
 * 安装向导组件
 *
 * Multi-step wizard for installing SeaTunnel on a host.
 * Supports triggering from host detail page or cluster detail page.
 * 多步骤向导，用于在主机上安装 SeaTunnel。
 * 支持从主机详情页或集群详情页触发。
 */

'use client';

import {useState, useCallback, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Progress} from '@/components/ui/progress';
import {cn} from '@/lib/utils';
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  X,
  Loader2,
} from 'lucide-react';
import {DisabledReasonTooltip} from '@/components/common/DisabledReasonTooltip';
import services from '@/lib/services';
import {
  heapNextDisabledKey,
  validateHeapSizeMB,
} from '@/lib/services/installer/heap';
import {
  useInstallWizard,
  usePrecheck,
  useInstallation,
} from '@/hooks/use-installer';
import {PrecheckStep} from './PrecheckStep';
import {ConfigStep} from './ConfigStep';
import {PluginSelectStep} from './PluginSelectStep';
import {InstallStep} from './InstallStep';
import {CompleteStep} from './CompleteStep';
import type {
  PrecheckResult,
  InstallationStatus,
} from '@/lib/services/installer/types';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';

// Wizard step type / 向导步骤类型
export type WizardStepType =
  | 'precheck'
  | 'config'
  | 'plugins'
  | 'install'
  | 'complete';

// Step configuration / 步骤配置
interface StepConfig {
  id: WizardStepType;
  titleKey: string;
  /** 步骤说明；与标题重复时省略 / Step description, omitted when it would just repeat the title */
  descKey?: string;
}

const WIZARD_STEPS: StepConfig[] = [
  {
    id: 'precheck',
    titleKey: 'installer.precheck',
    descKey: 'installer.precheckDesc',
  },
  {
    id: 'config',
    titleKey: 'installer.configuration',
    descKey: 'installer.configurationDesc',
  },
  {
    id: 'plugins',
    titleKey: 'installer.pluginSelection',
  },
  {
    id: 'install',
    titleKey: 'installer.installation',
    descKey: 'installer.installationDesc',
  },
  {
    id: 'complete',
    titleKey: 'installer.complete',
    descKey: 'installer.completeDesc',
  },
];

interface InstallWizardProps {
  /** Whether the dialog is open / 对话框是否打开 */
  open: boolean;
  /** Callback when dialog open state changes / 对话框打开状态变化时的回调 */
  onOpenChange: (open: boolean) => void;
  /** Host ID to install SeaTunnel on / 要安装 SeaTunnel 的主机 ID */
  hostId: number | string;
  /** Host name for display / 显示用的主机名称 */
  hostName?: string;
  /** Cluster ID if triggered from cluster page / 如果从集群页面触发，则为集群 ID */
  clusterId?: number | string;
  /** Callback when installation completes / 安装完成时的回调 */
  onComplete?: (status: InstallationStatus) => void;
  /** Initial version to select / 初始选择的版本 */
  initialVersion?: string;
  /** Initial install directory override / 初始安装目录 */
  initialInstallDir?: string;
  /** Initial cluster communication port override / 初始集群通信端口 */
  initialClusterPort?: number;
  /** Initial HTTP port override / 初始 HTTP 端口 */
  initialHttpPort?: number;
}

export function InstallWizard({
  open,
  onOpenChange,
  hostId,
  hostName,
  clusterId,
  onComplete,
  initialVersion,
  initialInstallDir,
  initialClusterPort,
  initialHttpPort,
}: InstallWizardProps) {
  const t = useTranslations();
  const [currentStepIndex, setCurrentStepIndex] = useState(0);
  const [precheckResult, setPrecheckResult] = useState<PrecheckResult | null>(
    null,
  );
  const [installationStarted, setInstallationStarted] = useState(false);
  const [hostMemoryBytes, setHostMemoryBytes] = useState<number | undefined>();
  // 主机内存请求进行中；成功/失败都会结束，避免永远卡住 Next。
  // Host-memory fetch in flight; cleared on success or failure so Next never sticks.
  const [hostMemoryLoading, setHostMemoryLoading] = useState(false);

  // Wizard state management / 向导状态管理
  const {config, updateConfig, resetWizard, buildInstallRequest} =
    useInstallWizard();

  // 拉取主机总内存，供堆校验使用（拿不到则只做范围校验）。
  // Load host total memory for heap checks; range-only when unavailable.
  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    const id = Number(hostId);
    if (!Number.isFinite(id) || id <= 0) {
      setHostMemoryBytes(undefined);
      setHostMemoryLoading(false);
      return;
    }
    setHostMemoryLoading(true);
    void services.host.getHostSafe(id).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.success && result.data?.total_memory) {
        setHostMemoryBytes(result.data.total_memory);
      } else {
        setHostMemoryBytes(undefined);
      }
      setHostMemoryLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [open, hostId]);

  // Precheck hook / 预检查 hook
  const {
    result: precheckData,
    loading: precheckLoading,
    error: precheckError,
    runPrecheck,
    reset: resetPrecheck,
  } = usePrecheck(hostId);

  // Installation hook / 安装 hook
  const {
    status: installStatus,
    loading: installLoading,
    error: installError,
    startInstallation,
    retryStep,
    cancelInstallation,
    refresh: refreshInstallStatus,
  } = useInstallation(hostId);

  // Set initial version if provided / 如果提供了初始版本则设置
  useEffect(() => {
    if (initialVersion) {
      updateConfig({version: initialVersion});
    }
  }, [initialVersion, updateConfig]);

  useEffect(() => {
    if (initialInstallDir) {
      updateConfig({installDir: initialInstallDir});
    }
  }, [initialInstallDir, updateConfig]);

  useEffect(() => {
    if (initialHttpPort) {
      updateConfig({httpPort: initialHttpPort});
    }
  }, [initialHttpPort, updateConfig]);

  useEffect(() => {
    if (initialClusterPort) {
      updateConfig({clusterPort: initialClusterPort});
    }
  }, [initialClusterPort, updateConfig]);

  // Set cluster ID if provided / 如果提供了集群 ID 则设置
  useEffect(() => {
    if (clusterId) {
      updateConfig({clusterId: String(clusterId)});
    }
  }, [clusterId, updateConfig]);

  // Update precheck result when data changes / 当数据变化时更新预检查结果
  useEffect(() => {
    if (precheckData) {
      setPrecheckResult(precheckData);
    }
  }, [precheckData]);

  // Handle installation completion / 处理安装完成
  useEffect(() => {
    if (installStatus?.status === 'success') {
      setCurrentStepIndex(4); // Move to complete step / 移动到完成步骤
      onComplete?.(installStatus);
    }
  }, [installStatus?.status, installStatus, onComplete]);

  // Current step / 当前步骤
  const currentStep = WIZARD_STEPS[currentStepIndex];

  // Calculate progress / 计算进度
  const progress = ((currentStepIndex + 1) / WIZARD_STEPS.length) * 100;

  /**
   * 「下一步」被禁用的原因；null 表示可继续。与 DisabledReasonTooltip 共用。
   * Why Next is blocked; null means proceed. Shared with DisabledReasonTooltip.
   */
  const proceedBlockReason = useCallback((): string | null => {
    const hasProfileSelectionIssue = (
      config.connector.selected_plugins || []
    ).some(
      (pluginName) =>
        pluginName === 'jdbc' &&
        (config.connector.selected_plugin_profiles?.[pluginName] || [])
          .length === 0,
    );

    switch (currentStep.id) {
      case 'precheck':
        return precheckResult?.overall_status === 'passed' ||
          precheckResult?.overall_status === 'warning'
          ? null
          : t('installer.precheckFailed');
      case 'config': {
        if (!config.version) {
          return t('installer.selectVersion');
        }
        if (config.installMode === 'offline' && !config.packagePath) {
          return t('installer.selectPackage');
        }
        const heapValues =
          config.deploymentMode === 'hybrid'
            ? [config.jvm.hybrid_heap_size]
            : [config.jvm.master_heap_size, config.jvm.worker_heap_size];
        for (const value of heapValues) {
          const heapCode = validateHeapSizeMB(value, hostMemoryBytes);
          if (heapCode) {
            return t(heapNextDisabledKey(heapCode));
          }
        }
        return null;
      }
      case 'plugins':
        return hasProfileSelectionIssue
          ? t('cluster.wizard.nextDisabledJdbc')
          : null;
      case 'install':
        return installStatus?.status === 'success' ? null : '';
      case 'complete':
        return null;
      default:
        return '';
    }
  }, [
    currentStep.id,
    precheckResult,
    config,
    installStatus,
    hostMemoryBytes,
    t,
  ]);

  // 处理中用原生 disabled；仅校验失败才 aria-disabled + 原因 Tooltip。
  // Busy uses native disabled; validation failures alone use aria-disabled + reason Tooltip.
  const nextBusy =
    precheckLoading ||
    installLoading ||
    (currentStep.id === 'config' && hostMemoryLoading);
  const nextValidationReason = nextBusy ? null : proceedBlockReason();
  const nextValidationBlocked = nextValidationReason !== null;

  // Handle next step / 处理下一步
  const handleNext = useCallback(async () => {
    if (currentStep.id === 'plugins') {
      // Start installation when moving from plugins to install step
      // 从插件步骤移动到安装步骤时开始安装
      setCurrentStepIndex(3);
      setInstallationStarted(true);
      try {
        const request = buildInstallRequest();
        await startInstallation(request);
      } catch (err) {
        console.error('Failed to start installation:', err);
      }
    } else if (currentStepIndex < WIZARD_STEPS.length - 1) {
      setCurrentStepIndex(currentStepIndex + 1);
    }
  }, [
    currentStep.id,
    currentStepIndex,
    buildInstallRequest,
    startInstallation,
  ]);

  // Handle previous step / 处理上一步
  const handlePrevious = useCallback(() => {
    if (
      currentStepIndex > 0 &&
      currentStep.id !== 'install' &&
      currentStep.id !== 'complete'
    ) {
      setCurrentStepIndex(currentStepIndex - 1);
    }
  }, [currentStepIndex, currentStep.id]);

  // Handle close / 处理关闭
  const handleClose = useCallback(() => {
    if (installationStarted && installStatus?.status === 'running') {
      // Confirm before closing during installation / 安装过程中关闭前确认
      if (!confirm(t('installer.confirmCancelInstallation'))) {
        return;
      }
      cancelInstallation();
    }
    resetWizard();
    resetPrecheck();
    setPrecheckResult(null);
    setCurrentStepIndex(0);
    setInstallationStarted(false);
    onOpenChange(false);
  }, [
    installationStarted,
    installStatus,
    t,
    cancelInstallation,
    resetWizard,
    resetPrecheck,
    onOpenChange,
  ]);

  // Handle precheck / 处理预检查
  const handleRunPrecheck = useCallback(async () => {
    const supportsHTTPService = isSeatunnelVersionAtLeast(
      config.version,
      '2.3.9',
    );
    const ports =
      supportsHTTPService && config.runtime.enable_http
        ? [config.clusterPort, config.httpPort, config.javaProxyPort]
        : [config.clusterPort, config.javaProxyPort];
    try {
      await runPrecheck({
        min_memory_mb: 2048,
        min_cpu_cores: 2,
        min_disk_space_mb: 5120,
        install_dir: config.installDir || undefined,
        ports,
        target_version: config.version || undefined,
      });
    } catch (err) {
      console.error('Precheck failed:', err);
    }
  }, [
    config.clusterPort,
    config.httpPort,
    config.javaProxyPort,
    config.installDir,
    config.runtime.enable_http,
    config.version,
    runPrecheck,
  ]);

  // Render step content / 渲染步骤内容
  const renderStepContent = () => {
    switch (currentStep.id) {
      case 'precheck':
        return (
          <PrecheckStep
            result={precheckResult}
            loading={precheckLoading}
            error={precheckError}
            onRunPrecheck={handleRunPrecheck}
          />
        );
      case 'config':
        return (
          <ConfigStep
            config={config}
            onConfigChange={updateConfig}
            hostIds={[hostId]}
            hostMemoryBytes={hostMemoryBytes}
          />
        );
      case 'plugins':
        return (
          <PluginSelectStep
            version={config.version}
            mirror={config.mirror}
            onMirrorChange={(mirror) => updateConfig({mirror})}
            selectedPlugins={config.connector.selected_plugins || []}
            selectedPluginProfiles={
              config.connector.selected_plugin_profiles || {}
            }
            onPluginsChange={(plugins: string[]) => {
              const selectedPluginSet = new Set(plugins);
              const nextProfiles = Object.fromEntries(
                Object.entries(
                  config.connector.selected_plugin_profiles || {},
                ).filter(([pluginName]) => selectedPluginSet.has(pluginName)),
              );
              updateConfig({
                connector: {
                  ...config.connector,
                  install_connectors: plugins.length > 0,
                  selected_plugins: plugins,
                  selected_plugin_profiles: nextProfiles,
                },
              });
            }}
            onPluginProfilesChange={(pluginName, profileKeys) =>
              updateConfig({
                connector: {
                  ...config.connector,
                  selected_plugin_profiles: {
                    ...(config.connector.selected_plugin_profiles || {}),
                    [pluginName]: profileKeys,
                  },
                },
              })
            }
          />
        );
      case 'install':
        return (
          <InstallStep
            hostId={hostId}
            status={installStatus}
            loading={installLoading}
            error={installError}
            onRetry={retryStep}
            onCancel={cancelInstallation}
            onRefresh={refreshInstallStatus}
          />
        );
      case 'complete':
        return (
          <CompleteStep
            status={installStatus}
            hostId={hostId}
            hostName={hostName}
            clusterId={clusterId}
            selectedPlugins={config.connector.selected_plugins || []}
            onClose={handleClose}
          />
        );
      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        data-testid='install-wizard-dialog'
        className='sm:max-w-4xl max-h-[90vh] overflow-hidden flex flex-col'
        // 没有步骤说明时显式去掉 aria-describedby，避免 Radix 告警
        // Drop aria-describedby explicitly when the step has no description
        {...(currentStep.descKey ? {} : {'aria-describedby': undefined})}
      >
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            {t('installer.installWizard')}
            {hostName && (
              <span className='text-muted-foreground font-normal'>
                - {hostName}
              </span>
            )}
          </DialogTitle>
          {currentStep.descKey ? (
            <DialogDescription>{t(currentStep.descKey)}</DialogDescription>
          ) : null}
        </DialogHeader>

        {/* Step indicator / 步骤指示器 */}
        <div className='py-4'>
          <div className='flex items-center justify-between mb-2'>
            {WIZARD_STEPS.map((step, index) => (
              <div
                key={step.id}
                className={cn(
                  'flex items-center',
                  index < WIZARD_STEPS.length - 1 && 'flex-1',
                )}
              >
                {/* Step circle / 步骤圆圈 */}
                <div
                  className={cn(
                    'flex items-center justify-center w-8 h-8 rounded-full border-2 transition-colors',
                    index < currentStepIndex &&
                      'bg-primary border-primary text-primary-foreground',
                    index === currentStepIndex && 'border-primary text-primary',
                    index > currentStepIndex &&
                      'border-muted-foreground/30 text-muted-foreground/50',
                  )}
                >
                  {index < currentStepIndex ? (
                    <CheckCircle2 className='h-5 w-5' />
                  ) : index === currentStepIndex &&
                    (installLoading || precheckLoading) ? (
                    <Loader2 className='h-4 w-4 animate-spin' />
                  ) : (
                    <span className='text-sm font-medium'>{index + 1}</span>
                  )}
                </div>

                {/* Step label / 步骤标签 */}
                <span
                  className={cn(
                    // 步骤名是原子标签，禁止被挤成「预检/查」式折行
                    // Step names are atomic labels; never let them wrap mid-word
                    'ml-2 text-sm font-medium hidden sm:inline whitespace-nowrap',
                    index === currentStepIndex && 'text-primary',
                    index !== currentStepIndex && 'text-muted-foreground',
                  )}
                >
                  {t(step.titleKey)}
                </span>

                {/* Connector line / 连接线 */}
                {index < WIZARD_STEPS.length - 1 && (
                  <div
                    className={cn(
                      'flex-1 h-0.5 mx-4',
                      index < currentStepIndex
                        ? 'bg-primary'
                        : 'bg-muted-foreground/30',
                    )}
                  />
                )}
              </div>
            ))}
          </div>
          <Progress value={progress} className='h-1' />
        </div>

        {/* Step content / 步骤内容 */}
        <div
          className='flex-1 overflow-y-auto min-h-[400px] py-4'
          data-testid={`install-wizard-step-${currentStep.id}`}
        >
          {renderStepContent()}
        </div>

        {/* Footer buttons / 底部按钮 */}
        <div className='flex items-center justify-between pt-4 border-t'>
          <Button
            variant='outline'
            onClick={handlePrevious}
            data-testid='install-wizard-previous'
            disabled={
              currentStepIndex === 0 ||
              currentStep.id === 'install' ||
              currentStep.id === 'complete'
            }
          >
            <ChevronLeft className='h-4 w-4 mr-1' />
            {t('common.previous')}
          </Button>

          <div className='flex items-center gap-2'>
            {currentStep.id !== 'complete' && (
              <Button variant='ghost' onClick={handleClose}>
                <X className='h-4 w-4 mr-1' />
                {t('common.cancel')}
              </Button>
            )}

            {currentStep.id === 'complete' ? (
              <Button onClick={handleClose}>{t('common.close')}</Button>
            ) : currentStep.id === 'install' ? null : ( // No next button during installation / 安装过程中没有下一步按钮
              <DisabledReasonTooltip reason={nextValidationReason}>
                <Button
                  onClick={() => {
                    if (nextBusy || nextValidationBlocked) {
                      return;
                    }
                    void handleNext();
                  }}
                  data-testid='install-wizard-next'
                  disabled={nextBusy}
                  aria-disabled={
                    !nextBusy && nextValidationBlocked ? 'true' : undefined
                  }
                  className={
                    !nextBusy && nextValidationBlocked
                      ? 'opacity-60'
                      : undefined
                  }
                >
                  {currentStep.id === 'plugins' ? (
                    <>
                      {t('installer.startInstallation')}
                      <ChevronRight className='h-4 w-4 ml-1' />
                    </>
                  ) : (
                    <>
                      {t('common.next')}
                      <ChevronRight className='h-4 w-4 ml-1' />
                    </>
                  )}
                </Button>
              </DisabledReasonTooltip>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default InstallWizard;
