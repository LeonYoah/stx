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
  descKey: string;
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
    descKey: 'installer.pluginSelectionDesc',
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

  // Wizard state management / 向导状态管理
  const {config, updateConfig, resetWizard, buildInstallRequest} =
    useInstallWizard();

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

  // Check if can proceed to next step / 检查是否可以进入下一步
  const canProceed = useCallback(() => {
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
        // Can proceed if precheck passed or has only warnings / 如果预检查通过或只有警告则可以继续
        return (
          precheckResult?.overall_status === 'passed' ||
          precheckResult?.overall_status === 'warning'
        );
      case 'config':
        // Must have version selected / 必须选择版本
        if (!config.version) {
          return false;
        }
        // If offline mode, must have package path / 如果是离线模式，必须有安装包路径
        if (config.installMode === 'offline' && !config.packagePath) {
          return false;
        }
        return true;
      case 'plugins':
        // Plugin selection is optional, but selected JDBC profiles must be explicit / 插件选择可选，但已选择的 JDBC 必须显式选择场景
        return !hasProfileSelectionIssue;
      case 'install':
        // Cannot proceed during installation / 安装过程中不能继续
        return installStatus?.status === 'success';
      case 'complete':
        return true;
      default:
        return false;
    }
  }, [currentStep.id, precheckResult, config, installStatus]);

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
    onOpenChange
  ]);

  // Handle precheck / 处理预检查
  const handleRunPrecheck = useCallback(async () => {
    const supportsHTTPService = isSeatunnelVersionAtLeast(config.version, '2.3.9');
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
        className='max-w-4xl max-h-[90vh] overflow-hidden flex flex-col'
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
          <DialogDescription>{t(currentStep.descKey)}</DialogDescription>
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
                    'ml-2 text-sm font-medium hidden sm:inline',
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
            ) : currentStep.id ===
              'install' ? // No next button during installation / 安装过程中没有下一步按钮
            null : (
              <Button
                onClick={handleNext}
                data-testid='install-wizard-next'
                disabled={!canProceed() || precheckLoading || installLoading}
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
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default InstallWizard;
