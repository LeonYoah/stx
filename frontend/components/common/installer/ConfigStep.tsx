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
 * Configuration Step Component
 * 配置步骤组件
 *
 * Allows users to configure installation options.
 * 允许用户配置安装选项。
 */

'use client';

import {useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {usePackages} from '@/hooks/use-installer';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Label} from '@/components/ui/label';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import {cn} from '@/lib/utils';
import {mapCheckpointNamespaceToImap} from '@/lib/runtime-storage-namespace';
import {
  Cloud,
  HardDrive,
  Server,
  Layers,
  Settings,
  Database,
  Loader2,
  AlertTriangle,
  CheckCircle2,
  Info,
} from 'lucide-react';
import type {
  InstallMode,
  MirrorSource,
  DeploymentMode,
  NodeRole,
  JVMConfig,
  CheckpointConfig,
  IMAPConfig,
  CheckpointStorageType,
  IMAPStorageType,
  ConnectorConfig,
  RuntimeEngineConfig,
  RuntimeStorageValidationKind,
  RuntimeStorageValidationResult,
} from '@/lib/services/installer/types';
import {installerService} from '@/lib/services/installer';
import {Button} from '@/components/ui/button';
import {toast} from 'sonner';
import {RuntimeAdvancedConfigCard} from '@/components/common/installer/RuntimeAdvancedConfigCard';
import {
  buildSeatunnelInstallDir,
  resolveSeatunnelVersionCapabilities,
} from '@/lib/seatunnel-version';

interface InstallWizardConfig {
  version: string;
  installDir: string;
  installMode: InstallMode;
  mirror: MirrorSource;
  packagePath: string;
  deploymentMode: DeploymentMode;
  nodeRole: NodeRole;
  clusterId: string;
  clusterPort: number;
  httpPort: number;
  /** Managed stx-java-proxy listen port / 托管 stx-java-proxy 监听端口 */
  javaProxyPort: number;
  runtime: RuntimeEngineConfig;
  jvm: JVMConfig;
  checkpoint: CheckpointConfig;
  imap: IMAPConfig;
  connector: ConnectorConfig;
}

interface ConfigStepProps {
  /** Current configuration / 当前配置 */
  config: InstallWizardConfig;
  /** Callback when configuration changes / 配置变化时的回调 */
  onConfigChange: (updates: Partial<InstallWizardConfig>) => void;
  /** Host IDs used for connectivity validation / 用于联通性校验的主机 ID 列表 */
  hostIds?: Array<number | string>;
}

export function ConfigStep({
  config,
  onConfigChange,
  hostIds = [],
}: ConfigStepProps) {
  const t = useTranslations();
  const {packages, loading: packagesLoading} = usePackages();
  const [storageValidation, setStorageValidation] = useState<
    Partial<
      Record<RuntimeStorageValidationKind, RuntimeStorageValidationResult>
    >
  >({});
  const [validatingKind, setValidatingKind] =
    useState<RuntimeStorageValidationKind | null>(null);

  // Get available versions / 获取可用版本
  const availableVersions = packages?.versions || [];
  const localPackages = packages?.local_packages || [];
  const recommendedVersion = packages?.recommended_version;
  const versionCapabilities = useMemo(
    () => resolveSeatunnelVersionCapabilities(packages, config.version),
    [packages, config.version],
  );

  // Handle JVM config change / 处理 JVM 配置变化
  const handleJvmChange = (key: keyof JVMConfig, value: number) => {
    onConfigChange({
      jvm: {...config.jvm, [key]: value},
    });
  };

  // Handle checkpoint config change / 处理检查点配置变化
  const handleCheckpointChange = (
    key: keyof CheckpointConfig,
    value: string | number | boolean | undefined,
  ) => {
    onConfigChange({
      checkpoint: {...config.checkpoint, [key]: value},
    });
  };

  const handleImapChange = (
    key: keyof IMAPConfig,
    value: string | number | boolean | undefined,
  ) => {
    onConfigChange({
      imap: {...config.imap, [key]: value},
    });
  };

  const applyCheckpointToImap = () => {
    const nextImap: IMAPConfig = {
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
      hdfs_site_path: config.checkpoint.hdfs_site_path,
      disable_cache: config.checkpoint.disable_cache,
      storage_endpoint: config.checkpoint.storage_endpoint,
      storage_access_key: config.checkpoint.storage_access_key,
      storage_secret_key: config.checkpoint.storage_secret_key,
      storage_bucket: config.checkpoint.storage_bucket,
      s3_credentials_provider: config.checkpoint.s3_credentials_provider,
    };
    onConfigChange({imap: nextImap});
    setStorageValidation((prev) => ({...prev, imap: undefined}));
    toast.success(t('installer.runtimeStorage.applyCheckpointToImapSuccess'));
  };

  const getRuntimeStorageMissingFields = (kind: RuntimeStorageValidationKind) => {
    const target = kind === 'checkpoint' ? config.checkpoint : config.imap;
    const fields: string[] = [];
    if (!target.namespace?.trim()) {
      fields.push(t('installer.path'));
    }
    switch (target.storage_type) {
      case 'S3':
      case 'OSS':
        if (!target.storage_endpoint?.trim()) {
        fields.push(t('installer.endpoint'));
      }
        if (!target.storage_access_key?.trim()) {
        fields.push(t('installer.accessKey'));
      }
        if (!target.storage_secret_key?.trim()) {
        fields.push(t('installer.secretKey'));
      }
        if (!target.storage_bucket?.trim()) {
        fields.push(t('installer.bucket'));
      }
        break;
      case 'HDFS':
        if (target.hdfs_ha_enabled) {
          if (!target.hdfs_name_services?.trim()) {
          fields.push('NameService');
        }
          if (!target.hdfs_ha_namenodes?.trim()) {
          fields.push('HA Namenodes');
        }
          if (!target.hdfs_namenode_rpc_address_1?.trim()) {
          fields.push('RPC Address 1');
        }
          if (!target.hdfs_namenode_rpc_address_2?.trim()) {
          fields.push('RPC Address 2');
        }
        } else {
          if (!target.hdfs_namenode_host?.trim()) {
          fields.push('NameNode Host');
        }
          if (!target.hdfs_namenode_port || Number(target.hdfs_namenode_port) <= 0) {
          fields.push('NameNode Port');
        }
        }
        break;
      case 'DISABLED':
        return [];
      default:
        break;
    }
    return fields;
  };

  const applyImapToCheckpoint = () => {
    if (config.imap.storage_type === 'DISABLED') {
      toast.warning(t('installer.runtimeStorage.applyImapDisabledWarning'));
      return;
    }
    const nextCheckpoint: CheckpointConfig = {
      storage_type: config.imap.storage_type,
      namespace: config.imap.namespace,
      hdfs_namenode_host: config.imap.hdfs_namenode_host,
      hdfs_namenode_port: config.imap.hdfs_namenode_port,
      kerberos_principal: config.imap.kerberos_principal,
      kerberos_keytab_file_path: config.imap.kerberos_keytab_file_path,
      hdfs_ha_enabled: config.imap.hdfs_ha_enabled,
      hdfs_name_services: config.imap.hdfs_name_services,
      hdfs_ha_namenodes: config.imap.hdfs_ha_namenodes,
      hdfs_namenode_rpc_address_1: config.imap.hdfs_namenode_rpc_address_1,
      hdfs_namenode_rpc_address_2: config.imap.hdfs_namenode_rpc_address_2,
      hdfs_failover_proxy_provider: config.imap.hdfs_failover_proxy_provider,
      storage_endpoint: config.imap.storage_endpoint,
      storage_access_key: config.imap.storage_access_key,
      storage_secret_key: config.imap.storage_secret_key,
      storage_bucket: config.imap.storage_bucket,
    };
    onConfigChange({checkpoint: nextCheckpoint});
    setStorageValidation((prev) => ({...prev, checkpoint: undefined}));
    toast.success(t('installer.runtimeStorage.applyImapToCheckpointSuccess'));
  };

  const numericHostIds = useMemo(
    () =>
      hostIds
        .map((id) => Number(id))
        .filter((id) => Number.isFinite(id) && id > 0),
    [hostIds],
  );

  const validateStorage = async (kind: RuntimeStorageValidationKind) => {
    if (numericHostIds.length === 0) {
      toast.warning(t('installer.runtimeStorage.noHostsSelected'));
      return;
    }
    const missingFields = getRuntimeStorageMissingFields(kind);
    if (missingFields.length > 0) {
      toast.warning(
        t('installer.runtimeStorage.requiredFieldsMissing', {
          fields: missingFields.join(', '),
        }),
      );
      return;
    }
    try {
      setValidatingKind(kind);
      const result = await installerService.validateRuntimeStorage({
        host_ids: numericHostIds,
        kind,
        checkpoint: kind === 'checkpoint' ? config.checkpoint : undefined,
        imap: kind === 'imap' ? config.imap : undefined,
      });
      setStorageValidation((prev) => ({...prev, [kind]: result}));
      if (result.success) {
        toast.success(t('installer.runtimeStorage.validationPassed'));
      } else {
        toast.warning(t('installer.runtimeStorage.validationWarning'));
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('installer.runtimeStorage.validationFailed'),
      );
    } finally {
      setValidatingKind(null);
    }
  };

  const checkpointNeedsSharedWarning =
    numericHostIds.length > 1 &&
    config.checkpoint.storage_type === 'LOCAL_FILE';
  const checkpointLocalRecommended =
    numericHostIds.length <= 1 &&
    config.checkpoint.storage_type !== 'LOCAL_FILE';

  const imapExternalEnabled = config.imap.storage_type !== 'DISABLED';
  const imapLocalRecommended =
    numericHostIds.length <= 1 && config.imap.storage_type !== 'DISABLED';
  const httpSupported = Boolean(versionCapabilities?.supports_http_service);

  return (
    <ScrollArea className='h-[500px] pr-4'>
      <div className='space-y-6'>
        {/* Install Mode / 安装模式 */}
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='text-base flex items-center gap-2'>
              <Cloud className='h-4 w-4' />
              {t('installer.installMode')}
            </CardTitle>
            <CardDescription>
              {t('installer.installModeDesc') ||
                'Choose how to install SeaTunnel'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className='grid grid-cols-2 gap-4'>
              {/* Online mode / 在线模式 */}
              <div
                className={cn(
                  'relative flex flex-col p-4 rounded-lg border-2 cursor-pointer transition-colors',
                  config.installMode === 'online'
                    ? 'border-primary bg-primary/5'
                    : 'border-muted hover:border-muted-foreground/50',
                )}
                onClick={() => onConfigChange({installMode: 'online'})}
              >
                <div className='flex items-center gap-2 mb-2'>
                  <Cloud className='h-5 w-5 text-primary' />
                  <span className='font-medium'>{t('installer.online')}</span>
                </div>
                <p className='text-sm text-muted-foreground'>
                  {t('installer.onlineDesc')}
                </p>
              </div>

              {/* Offline mode / 离线模式 */}
              <div
                className={cn(
                  'relative flex flex-col p-4 rounded-lg border-2 cursor-pointer transition-colors',
                  config.installMode === 'offline'
                    ? 'border-primary bg-primary/5'
                    : 'border-muted hover:border-muted-foreground/50',
                )}
                onClick={() => onConfigChange({installMode: 'offline'})}
              >
                <div className='flex items-center gap-2 mb-2'>
                  <HardDrive className='h-5 w-5 text-primary' />
                  <span className='font-medium'>{t('installer.offline')}</span>
                </div>
                <p className='text-sm text-muted-foreground'>
                  {t('installer.offlineDesc')}
                </p>
              </div>
            </div>

            {/* Online mode options / 在线模式选项 */}
            {config.installMode === 'online' && (
              <div className='mt-4 space-y-4'>
                {/* Mirror source / 镜像源 */}
                <div className='space-y-2'>
                  <Label>{t('installer.mirrorSource')}</Label>
                  <Select
                    value={config.mirror}
                    onValueChange={(value: MirrorSource) =>
                      onConfigChange({mirror: value})
                    }
                  >
                    <SelectTrigger data-testid='install-config-mirror'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='aliyun'>
                        {t('installer.mirrors.aliyun')}
                      </SelectItem>
                      <SelectItem value='huaweicloud'>
                        {t('installer.mirrors.huaweicloud')}
                      </SelectItem>
                      <SelectItem value='apache'>
                        {t('installer.mirrors.apache')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {/* Version selection / 版本选择 */}
                <div className='space-y-2'>
                  <Label>{t('installer.version')}</Label>
                  <Select
                    value={config.version}
                    onValueChange={(value) => onConfigChange({version: value})}
                    disabled={packagesLoading}
                  >
                    <SelectTrigger data-testid='install-config-version'>
                      {packagesLoading ? (
                        <div className='flex items-center gap-2'>
                          <Loader2 className='h-4 w-4 animate-spin' />
                          <span>{t('common.loading')}</span>
                        </div>
                      ) : (
                        <SelectValue
                          placeholder={
                            t('installer.selectVersion') || 'Select version'
                          }
                        />
                      )}
                    </SelectTrigger>
                    <SelectContent>
                      {availableVersions.map((version) => (
                        <SelectItem key={version} value={version}>
                          <div className='flex items-center gap-2'>
                            {version}
                            {version === recommendedVersion && (
                              <Badge variant='secondary' className='text-xs'>
                                {t('installer.recommended')}
                              </Badge>
                            )}
                          </div>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}

            {/* Offline mode options / 离线模式选项 */}
            {config.installMode === 'offline' && (
              <div className='mt-4 space-y-4'>
                {/* Local package selection / 本地安装包选择 */}
                <div className='space-y-2'>
                  <Label>{t('installer.selectPackage')}</Label>
                  {localPackages.length > 0 ? (
                    <Select
                      value={config.version}
                      onValueChange={(value) => {
                        const pkg = localPackages.find(
                          (p) => p.version === value,
                        );
                        onConfigChange({
                          version: value,
                          packagePath: pkg?.local_path || '',
                        });
                      }}
                    >
                      <SelectTrigger data-testid='install-config-offline-package'>
                        <SelectValue
                          placeholder={t('installer.selectPackage')}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {localPackages.map((pkg) => (
                          <SelectItem key={pkg.version} value={pkg.version}>
                            <div className='flex items-center gap-2'>
                              {pkg.version}
                              <span className='text-xs text-muted-foreground'>
                                ({(pkg.file_size / 1024 / 1024).toFixed(1)} MB)
                              </span>
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <p className='text-sm text-muted-foreground'>
                      {t('installer.noLocalPackages')}
                    </p>
                  )}
                </div>
              </div>
            )}

            <div className='mt-4 space-y-2'>
              <Label htmlFor='install-config-install-dir'>
                {t('installer.installDirLabel')}
              </Label>
              <Input
                id='install-config-install-dir'
                data-testid='install-config-install-dir'
                value={config.installDir}
                onChange={(event) =>
                  onConfigChange({installDir: event.target.value})
                }
                placeholder={buildSeatunnelInstallDir(config.version)}
              />
              <p className='text-xs text-muted-foreground'>
                {t('installer.installDirDesc')}
              </p>
            </div>
          </CardContent>
        </Card>

        {/* Deployment Mode / 部署模式 */}
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='text-base flex items-center gap-2'>
              <Layers className='h-4 w-4' />
              {t('installer.deploymentMode')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className='grid grid-cols-2 gap-4'>
              {/* Hybrid mode / 混合模式 */}
              <div
                className={cn(
                  'relative flex flex-col p-4 rounded-lg border-2 cursor-pointer transition-colors',
                  config.deploymentMode === 'hybrid'
                    ? 'border-primary bg-primary/5'
                    : 'border-muted hover:border-muted-foreground/50',
                )}
                onClick={() => onConfigChange({deploymentMode: 'hybrid'})}
              >
                <div className='flex items-center gap-2 mb-2'>
                  <Server className='h-5 w-5 text-primary' />
                  <span className='font-medium'>{t('installer.hybrid')}</span>
                </div>
                <p className='text-sm text-muted-foreground'>
                  {t('installer.hybridDesc')}
                </p>
              </div>

              {/* Separated mode / 分离模式 */}
              <div
                className={cn(
                  'relative flex flex-col p-4 rounded-lg border-2 cursor-pointer transition-colors',
                  config.deploymentMode === 'separated'
                    ? 'border-primary bg-primary/5'
                    : 'border-muted hover:border-muted-foreground/50',
                )}
                onClick={() => onConfigChange({deploymentMode: 'separated'})}
              >
                <div className='flex items-center gap-2 mb-2'>
                  <Layers className='h-5 w-5 text-primary' />
                  <span className='font-medium'>
                    {t('installer.separated')}
                  </span>
                </div>
                <p className='text-sm text-muted-foreground'>
                  {t('installer.separatedDesc')}
                </p>
              </div>
            </div>

            {/* Node role (for separated mode) / 节点角色（分离模式） */}
            {config.deploymentMode === 'separated' && (
              <div className='mt-4 space-y-2'>
                <Label>{t('installer.nodeRole')}</Label>
                <Select
                  value={config.nodeRole}
                  onValueChange={(value: NodeRole) =>
                    onConfigChange({nodeRole: value})
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='master'>
                      {t('installer.master')}
                    </SelectItem>
                    <SelectItem value='worker'>
                      {t('installer.worker')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
          </CardContent>
        </Card>

        {/* JVM Configuration / JVM 配置 */}
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='text-base flex items-center gap-2'>
              <Settings className='h-4 w-4' />
              {t('installer.jvmConfig')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className='grid grid-cols-1 md:grid-cols-3 gap-4'>
              {config.deploymentMode === 'hybrid' ? (
                <div className='space-y-2'>
                  <Label>{t('installer.hybridHeapSize')} (GB)</Label>
                  <Input
                    data-testid='install-jvm-hybrid-heap'
                    type='number'
                    value={config.jvm.hybrid_heap_size}
                    onChange={(e) =>
                      handleJvmChange(
                        'hybrid_heap_size',
                        parseInt(e.target.value) || 0,
                      )
                    }
                    min={1}
                    max={64}
                    step={1}
                  />
                </div>
              ) : (
                <>
                  <div className='space-y-2'>
                    <Label>{t('installer.masterHeapSize')} (GB)</Label>
                    <Input
                      data-testid='install-jvm-master-heap'
                      type='number'
                      value={config.jvm.master_heap_size}
                      onChange={(e) =>
                        handleJvmChange(
                          'master_heap_size',
                          parseInt(e.target.value) || 0,
                        )
                      }
                      min={1}
                      max={64}
                      step={1}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label>{t('installer.workerHeapSize')} (GB)</Label>
                    <Input
                      data-testid='install-jvm-worker-heap'
                      type='number'
                      value={config.jvm.worker_heap_size}
                      onChange={(e) =>
                        handleJvmChange(
                          'worker_heap_size',
                          parseInt(e.target.value) || 0,
                        )
                      }
                      min={1}
                      max={64}
                      step={1}
                    />
                  </div>
                </>
              )}
            </div>
          </CardContent>
        </Card>

        <RuntimeAdvancedConfigCard
          version={config.version}
          capabilities={versionCapabilities}
          runtime={config.runtime}
          onChange={(updates) =>
            onConfigChange({
              runtime: {...config.runtime, ...updates},
            })
          }
        />

        {httpSupported && (
          <Card>
            <CardHeader className='pb-3'>
              <CardTitle className='text-base flex items-center gap-2'>
                <Server className='h-4 w-4' />
                {t('installer.httpService.title')}
              </CardTitle>
              <CardDescription>
                {t('installer.httpService.description')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <label className='flex items-start gap-3 rounded-md border p-3'>
                <Checkbox
                  data-testid='install-runtime-http-enable'
                  checked={config.runtime.enable_http}
                  onCheckedChange={(checked) =>
                    onConfigChange({
                      runtime: {
                        ...config.runtime,
                        enable_http: checked === true,
                      },
                    })
                  }
                />
                <div className='space-y-1'>
                  <div className='text-sm font-medium'>
                    {t('installer.httpService.enableLabel')}
                  </div>
                  <p className='text-xs text-muted-foreground'>
                    {t('installer.httpService.enableHint')}
                  </p>
                </div>
              </label>
              <div className='space-y-2'>
                <Label>{t('installer.httpService.port')}</Label>
                <Input
                  data-testid='install-runtime-http-port'
                  type='number'
                  value={config.httpPort}
                  onChange={(event) =>
                    onConfigChange({
                      httpPort: Math.max(
                        1,
                        Number.parseInt(event.target.value, 10) || 8080,
                      ),
                    })
                  }
                  min={1}
                  max={65535}
                  step={1}
                  disabled={!config.runtime.enable_http}
                />
                <p className='text-xs text-muted-foreground'>
                  {t('installer.httpService.portHint')}
                </p>
              </div>
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='text-base flex items-center gap-2'>
              <Server className='h-4 w-4' />
              {t('installer.javaProxy.title')}
            </CardTitle>
            <CardDescription>
              {t('installer.javaProxy.description')}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-2'>
            <Label>{t('installer.javaProxy.port')}</Label>
            <Input
              data-testid='install-java-proxy-port'
              type='number'
              value={config.javaProxyPort}
              onChange={(event) =>
                onConfigChange({
                  javaProxyPort: Math.max(
                    1,
                    Number.parseInt(event.target.value, 10) || 18080,
                  ),
                })
              }
              min={1}
              max={65535}
              step={1}
            />
            <p className='text-xs text-muted-foreground'>
              {t('installer.javaProxy.portHint')}
            </p>
          </CardContent>
        </Card>


        {/* Checkpoint Configuration / 检查点配置 */}
        <Card>
          <CardHeader className='pb-3'>
            <div className='flex items-start justify-between gap-3'>
              <div>
                <CardTitle className='text-base flex items-center gap-2'>
                  <Database className='h-4 w-4' />
                  {t('installer.checkpointConfig')}
                </CardTitle>
                <CardDescription>
                  {t('installer.runtimeStorage.checkpointDescription')}
                </CardDescription>
              </div>
              <Button
                variant='outline'
                size='sm'
                onClick={applyCheckpointToImap}
              >
                {t('installer.runtimeStorage.applyToImap')}
              </Button>
            </div>
          </CardHeader>
          <CardContent className='space-y-4'>
            {checkpointNeedsSharedWarning && (
              <div className='rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100'>
                <div className='flex items-start gap-2'>
                  <AlertTriangle className='mt-0.5 h-4 w-4 shrink-0' />
                  <div>
                    {t('installer.runtimeStorage.checkpointLocalWarning')}
                  </div>
                </div>
              </div>
            )}
            {checkpointLocalRecommended && (
              <div className='rounded-md border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900 dark:border-sky-900/40 dark:bg-sky-950/30 dark:text-sky-100'>
                <div className='flex items-start gap-2'>
                  <Info className='mt-0.5 h-4 w-4 shrink-0' />
                  <div>
                    {t('installer.runtimeStorage.checkpointLocalRecommended')}
                  </div>
                </div>
              </div>
            )}
            <div className='space-y-2'>
              <Label>{t('installer.storageType')}</Label>
              <Select
                value={config.checkpoint.storage_type}
                onValueChange={(value: CheckpointStorageType) =>
                  handleCheckpointChange('storage_type', value)
                }
              >
                <SelectTrigger data-testid='install-checkpoint-storage-type'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='LOCAL_FILE'>
                    {t('installer.runtimeStorage.localFile')}
                  </SelectItem>
                  <SelectItem value='HDFS'>HDFS</SelectItem>
                  <SelectItem value='OSS'>Aliyun OSS</SelectItem>
                  <SelectItem value='S3'>AWS S3</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className='space-y-2'>
              <Label>{t('installer.namespace')}</Label>
              <Input
                data-testid='install-checkpoint-namespace'
                value={config.checkpoint.namespace}
                onChange={(e) =>
                  handleCheckpointChange('namespace', e.target.value)
                }
                placeholder='/tmp/seatunnel/checkpoint/'
              />
            </div>

            {config.checkpoint.storage_type === 'HDFS' && (
              <div className='space-y-4'>
                <div className='flex items-center space-x-2'>
                  <Checkbox
                    id='checkpoint-hdfs-ha-enabled'
                    checked={config.checkpoint.hdfs_ha_enabled || false}
                    onCheckedChange={(checked) =>
                      handleCheckpointChange(
                        'hdfs_ha_enabled',
                        checked === true,
                      )
                    }
                  />
                  <Label htmlFor='checkpoint-hdfs-ha-enabled'>
                    {t('installer.hdfsHAMode')}
                  </Label>
                </div>
                {!config.checkpoint.hdfs_ha_enabled && (
                  <div className='grid grid-cols-2 gap-4'>
                    <div className='space-y-2'>
                      <Label>{t('installer.hdfsNameNodeHost')}</Label>
                      <Input
                        value={config.checkpoint.hdfs_namenode_host || ''}
                        onChange={(e) =>
                          handleCheckpointChange(
                            'hdfs_namenode_host',
                            e.target.value,
                          )
                        }
                        placeholder='namenode.example.com'
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('installer.hdfsNameNodePort')}</Label>
                      <Input
                        type='number'
                        value={config.checkpoint.hdfs_namenode_port || ''}
                        onChange={(e) =>
                          handleCheckpointChange(
                            'hdfs_namenode_port',
                            parseInt(e.target.value) || 0,
                          )
                        }
                        placeholder='8020'
                      />
                    </div>
                  </div>
                )}
                {config.checkpoint.hdfs_ha_enabled && (
                  <div className='space-y-3 rounded-md border bg-muted/30 p-3'>
                    <div className='grid grid-cols-2 gap-4'>
                      <div className='space-y-2'>
                        <Label>{t('installer.hdfsNameServices')}</Label>
                        <Input
                          value={config.checkpoint.hdfs_name_services || ''}
                          onChange={(e) =>
                            handleCheckpointChange(
                              'hdfs_name_services',
                              e.target.value,
                            )
                          }
                          placeholder='mycluster'
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('installer.hdfsHANamenodes')}</Label>
                        <Input
                          value={config.checkpoint.hdfs_ha_namenodes || ''}
                          onChange={(e) =>
                            handleCheckpointChange(
                              'hdfs_ha_namenodes',
                              e.target.value,
                            )
                          }
                          placeholder='nn1,nn2'
                        />
                      </div>
                    </div>
                    <div className='grid grid-cols-2 gap-4'>
                      <div className='space-y-2'>
                        <Label>{t('installer.hdfsNamenodeRPCAddress1')}</Label>
                        <Input
                          value={
                            config.checkpoint.hdfs_namenode_rpc_address_1 || ''
                          }
                          onChange={(e) =>
                            handleCheckpointChange(
                              'hdfs_namenode_rpc_address_1',
                              e.target.value,
                            )
                          }
                          placeholder='nn1-host:8020'
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('installer.hdfsNamenodeRPCAddress2')}</Label>
                        <Input
                          value={
                            config.checkpoint.hdfs_namenode_rpc_address_2 || ''
                          }
                          onChange={(e) =>
                            handleCheckpointChange(
                              'hdfs_namenode_rpc_address_2',
                              e.target.value,
                            )
                          }
                          placeholder='nn2-host:8020'
                        />
                      </div>
                    </div>
                  </div>
                )}
                <div className='flex items-center space-x-2'>
                  <Checkbox
                    id='checkpoint-hdfs-kerberos'
                    checked={
                      Boolean(config.checkpoint.kerberos_principal) ||
                      Boolean(config.checkpoint.kerberos_keytab_file_path)
                    }
                    onCheckedChange={(checked) => {
                      if (checked === true) {
                        handleCheckpointChange(
                          'kerberos_principal',
                          config.checkpoint.kerberos_principal || '',
                        );
                        handleCheckpointChange(
                          'kerberos_keytab_file_path',
                          config.checkpoint.kerberos_keytab_file_path || '',
                        );
                        return;
                      }
                      handleCheckpointChange('kerberos_principal', undefined);
                      handleCheckpointChange(
                        'kerberos_keytab_file_path',
                        undefined,
                      );
                    }}
                  />
                  <Label htmlFor='checkpoint-hdfs-kerberos'>
                    {t('installer.hdfsKerberos')}
                  </Label>
                </div>
                {(Boolean(config.checkpoint.kerberos_principal) ||
                  Boolean(config.checkpoint.kerberos_keytab_file_path)) && (
                  <div className='grid grid-cols-2 gap-4 rounded-md border bg-muted/30 p-3'>
                    <div className='space-y-2'>
                      <Label>{t('installer.kerberosPrincipal')}</Label>
                      <Input
                        value={config.checkpoint.kerberos_principal || ''}
                        onChange={(e) =>
                          handleCheckpointChange(
                            'kerberos_principal',
                            e.target.value,
                          )
                        }
                        placeholder='hdfs/namenode@EXAMPLE.COM'
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('installer.kerberosKeytabPath')}</Label>
                      <Input
                        value={
                          config.checkpoint.kerberos_keytab_file_path || ''
                        }
                        onChange={(e) =>
                          handleCheckpointChange(
                            'kerberos_keytab_file_path',
                            e.target.value,
                          )
                        }
                        placeholder='/etc/security/keytabs/hdfs.keytab'
                      />
                    </div>
                  </div>
                )}

                <div className='space-y-2'>
                  <Label>{t('installer.hdfsSitePath')}</Label>
                  <Input
                    value={config.checkpoint.hdfs_site_path || ''}
                    onChange={(e) =>
                      handleCheckpointChange('hdfs_site_path', e.target.value)
                    }
                    placeholder='/etc/hadoop/conf/hdfs-site.xml'
                  />
                </div>
              </div>
            )}

            {config.checkpoint.storage_type === 'OSS' && (
              <div className='space-y-4'>
                <div className='rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-200'>
                  {t('installer.runtimeStorage.ossLibHint')}
                </div>
                <div className='space-y-2'>
                  <Label>{t('installer.endpoint')}</Label>
                  <Input
                    data-testid='install-checkpoint-endpoint'
                    value={config.checkpoint.storage_endpoint || ''}
                    onChange={(e) =>
                      handleCheckpointChange('storage_endpoint', e.target.value)
                    }
                    placeholder='oss-cn-hangzhou.aliyuncs.com'
                  />
                </div>
                <div className='grid grid-cols-2 gap-4'>
                  <div className='space-y-2'>
                    <Label>fs.oss.accessKeyId</Label>
                    <Input
                      data-testid='install-checkpoint-access-key'
                      type='password'
                      value={config.checkpoint.storage_access_key || ''}
                      onChange={(e) =>
                        handleCheckpointChange('storage_access_key', e.target.value)
                      }
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label>fs.oss.accessKeySecret</Label>
                    <Input
                      data-testid='install-checkpoint-secret-key'
                      type='password'
                      value={config.checkpoint.storage_secret_key || ''}
                      onChange={(e) =>
                        handleCheckpointChange('storage_secret_key', e.target.value)
                      }
                    />
                  </div>
                </div>
                <div className='space-y-2'>
                  <Label>{t('installer.bucket')}</Label>
                  <Input
                    data-testid='install-checkpoint-bucket'
                    value={config.checkpoint.storage_bucket || ''}
                    onChange={(e) =>
                      handleCheckpointChange('storage_bucket', e.target.value)
                    }
                    placeholder='your-bucket'
                  />
                </div>
              </div>
            )}

            {config.checkpoint.storage_type === 'S3' && (
              <div className='space-y-4'>
                <div className='space-y-2'>
                  <Label>{t('installer.s3CredentialsProvider')}</Label>
                  <Select
                    value={
                      config.checkpoint.s3_credentials_provider ||
                      'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider'
                    }
                    onValueChange={(value) =>
                      handleCheckpointChange('s3_credentials_provider', value)
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider'>
                        {t('installer.s3ProviderSimple')}
                      </SelectItem>
                      <SelectItem value='org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider'>
                        {t('installer.s3ProviderInstance')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className='space-y-2'>
                  <Label>{t('installer.endpoint')}</Label>
                  <Input
                    data-testid='install-checkpoint-endpoint'
                    value={config.checkpoint.storage_endpoint || ''}
                    onChange={(e) =>
                      handleCheckpointChange('storage_endpoint', e.target.value)
                    }
                    placeholder='http://127.0.0.1:9000'
                  />
                </div>
                {(config.checkpoint.s3_credentials_provider ||
                  'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider') !==
                  'org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider' && (
                  <div className='grid grid-cols-2 gap-4'>
                    <div className='space-y-2'>
                      <Label>fs.s3a.access.key</Label>
                      <Input
                        data-testid='install-checkpoint-access-key'
                        type='password'
                        value={config.checkpoint.storage_access_key || ''}
                        onChange={(e) =>
                          handleCheckpointChange('storage_access_key', e.target.value)
                        }
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label>fs.s3a.secret.key</Label>
                      <Input
                        data-testid='install-checkpoint-secret-key'
                        type='password'
                        value={config.checkpoint.storage_secret_key || ''}
                        onChange={(e) =>
                          handleCheckpointChange('storage_secret_key', e.target.value)
                        }
                      />
                    </div>
                  </div>
                )}
                <div className='space-y-2'>
                  <Label>{t('installer.bucket')}</Label>
                  <Input
                    data-testid='install-checkpoint-bucket'
                    value={config.checkpoint.storage_bucket || ''}
                    onChange={(e) =>
                      handleCheckpointChange('storage_bucket', e.target.value)
                    }
                    placeholder='s3a://bucket'
                  />
                </div>
              </div>
            )}

            <div className='flex items-center justify-between rounded-md border p-3'>
              <div className='space-y-1'>
                <div className='text-sm font-medium'>
                  {t('installer.runtimeStorage.validateConnectivity')}
                </div>
                <div className='text-xs text-muted-foreground'>
                  {t('installer.runtimeStorage.validationHint')}
                </div>
              </div>
              <Button
                data-testid='install-checkpoint-validate'
                variant='outline'
                size='sm'
                onClick={() => validateStorage('checkpoint')}
                disabled={validatingKind === 'checkpoint'}
              >
                {validatingKind === 'checkpoint' && (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                )}
                {t('installer.runtimeStorage.validateNow')}
              </Button>
            </div>

            {storageValidation.checkpoint && (
              <div
                data-testid='install-checkpoint-validation-result'
                className='rounded-md border p-3 text-sm space-y-2'
              >
                <div className='flex items-center gap-2 font-medium'>
                  {storageValidation.checkpoint.success ? (
                    <CheckCircle2 className='h-4 w-4 text-green-600' />
                  ) : (
                    <AlertTriangle className='h-4 w-4 text-amber-600' />
                  )}
                  <span>
                    {storageValidation.checkpoint.success
                      ? t('installer.runtimeStorage.validationPassed')
                      : t('installer.runtimeStorage.validationWarning')}
                  </span>
                </div>
                {storageValidation.checkpoint.warning && (
                  <div className='text-muted-foreground'>
                    {storageValidation.checkpoint.warning}
                  </div>
                )}
                <div className='space-y-1'>
                  {storageValidation.checkpoint.hosts.map((host) => (
                    <div key={host.host_id}>
                      <span className='font-medium'>
                        {host.host_name || host.host_id}
                      </span>
                      {' · '}
                      <span>{host.message}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* IMAP Configuration / IMAP 配置 */}
        <Card>
          <CardHeader className='pb-3'>
            <div className='flex items-start justify-between gap-3'>
              <div>
                <CardTitle className='text-base flex items-center gap-2'>
                  <Database className='h-4 w-4' />
                  {t('installer.runtimeStorage.imapTitle')}
                </CardTitle>
                <CardDescription>
                  {t('installer.runtimeStorage.imapDescription')}
                </CardDescription>
              </div>
              <Button
                variant='outline'
                size='sm'
                onClick={applyImapToCheckpoint}
                disabled={config.imap.storage_type === 'DISABLED'}
              >
                {t('installer.runtimeStorage.applyToCheckpoint')}
              </Button>
            </div>
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='rounded-md border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900 dark:border-sky-900/40 dark:bg-sky-950/30 dark:text-sky-100'>
              <div className='flex items-start gap-2'>
                <Info className='mt-0.5 h-4 w-4 shrink-0' />
                <div className='space-y-1'>
                  <p>{t('installer.runtimeStorage.imapGuidanceMeta')}</p>
                  <p>{t('installer.runtimeStorage.imapGuidanceBatch')}</p>
                  <p>{t('installer.runtimeStorage.imapGuidanceStreaming')}</p>
                </div>
              </div>
            </div>
            {imapLocalRecommended && (
              <div className='rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100'>
                <div className='flex items-start gap-2'>
                  <AlertTriangle className='mt-0.5 h-4 w-4 shrink-0' />
                  <div>{t('installer.runtimeStorage.imapLocalHint')}</div>
                </div>
              </div>
            )}

            <div className='space-y-2'>
              <Label>{t('installer.storageType')}</Label>
              <Select
                value={config.imap.storage_type}
                onValueChange={(value: IMAPStorageType) =>
                  handleImapChange('storage_type', value)
                }
              >
                <SelectTrigger data-testid='install-imap-storage-type'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='DISABLED'>
                    {t('installer.runtimeStorage.imapDisabled')}
                  </SelectItem>
                  <SelectItem value='LOCAL_FILE'>
                    {t('installer.runtimeStorage.localFile')}
                  </SelectItem>
                  <SelectItem value='HDFS'>HDFS</SelectItem>
                  <SelectItem value='OSS'>Aliyun OSS</SelectItem>
                  <SelectItem value='S3'>AWS S3</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {config.imap.storage_type !== 'DISABLED' && (
              <>
                <div className='space-y-2'>
                  <Label>{t('installer.namespace')}</Label>
                  <Input
                    data-testid='install-imap-namespace'
                    value={config.imap.namespace}
                    onChange={(e) =>
                      handleImapChange('namespace', e.target.value)
                    }
                    placeholder='/tmp/seatunnel/imap/'
                  />
                </div>

                {config.imap.storage_type === 'HDFS' && (
                  <div className='space-y-4'>
                    <div className='flex items-center space-x-2'>
                      <Checkbox
                        id='imap-hdfs-ha-enabled'
                        checked={config.imap.hdfs_ha_enabled || false}
                        onCheckedChange={(checked) =>
                          handleImapChange('hdfs_ha_enabled', checked === true)
                        }
                      />
                      <Label htmlFor='imap-hdfs-ha-enabled'>
                        {t('installer.hdfsHAMode')}
                      </Label>
                    </div>
                    {!config.imap.hdfs_ha_enabled && (
                      <div className='grid grid-cols-2 gap-4'>
                        <div className='space-y-2'>
                          <Label>{t('installer.hdfsNameNodeHost')}</Label>
                          <Input
                            value={config.imap.hdfs_namenode_host || ''}
                            onChange={(e) =>
                              handleImapChange(
                                'hdfs_namenode_host',
                                e.target.value,
                              )
                            }
                            placeholder='namenode.example.com'
                          />
                        </div>
                        <div className='space-y-2'>
                          <Label>{t('installer.hdfsNameNodePort')}</Label>
                          <Input
                            type='number'
                            value={config.imap.hdfs_namenode_port || ''}
                            onChange={(e) =>
                              handleImapChange(
                                'hdfs_namenode_port',
                                parseInt(e.target.value) || 0,
                              )
                            }
                            placeholder='8020'
                          />
                        </div>
                      </div>
                    )}
                    {config.imap.hdfs_ha_enabled && (
                      <div className='space-y-3 rounded-md border bg-muted/30 p-3'>
                        <div className='grid grid-cols-2 gap-4'>
                          <div className='space-y-2'>
                            <Label>{t('installer.hdfsNameServices')}</Label>
                            <Input
                              value={config.imap.hdfs_name_services || ''}
                              onChange={(e) =>
                                handleImapChange(
                                  'hdfs_name_services',
                                  e.target.value,
                                )
                              }
                              placeholder='mycluster'
                            />
                          </div>
                          <div className='space-y-2'>
                            <Label>{t('installer.hdfsHANamenodes')}</Label>
                            <Input
                              value={config.imap.hdfs_ha_namenodes || ''}
                              onChange={(e) =>
                                handleImapChange(
                                  'hdfs_ha_namenodes',
                                  e.target.value,
                                )
                              }
                              placeholder='nn1,nn2'
                            />
                          </div>
                        </div>
                        <div className='grid grid-cols-2 gap-4'>
                          <div className='space-y-2'>
                            <Label>
                              {t('installer.hdfsNamenodeRPCAddress1')}
                            </Label>
                            <Input
                              value={
                                config.imap.hdfs_namenode_rpc_address_1 || ''
                              }
                              onChange={(e) =>
                                handleImapChange(
                                  'hdfs_namenode_rpc_address_1',
                                  e.target.value,
                                )
                              }
                              placeholder='nn1-host:8020'
                            />
                          </div>
                          <div className='space-y-2'>
                            <Label>
                              {t('installer.hdfsNamenodeRPCAddress2')}
                            </Label>
                            <Input
                              value={
                                config.imap.hdfs_namenode_rpc_address_2 || ''
                              }
                              onChange={(e) =>
                                handleImapChange(
                                  'hdfs_namenode_rpc_address_2',
                                  e.target.value,
                                )
                              }
                              placeholder='nn2-host:8020'
                            />
                          </div>
                        </div>
                      </div>
                    )}
                    <div className='flex items-center space-x-2'>
                      <Checkbox
                        id='imap-hdfs-kerberos'
                        checked={
                          Boolean(config.imap.kerberos_principal) ||
                          Boolean(config.imap.kerberos_keytab_file_path)
                        }
                        onCheckedChange={(checked) => {
                          if (checked === true) {
                            handleImapChange(
                              'kerberos_principal',
                              config.imap.kerberos_principal || '',
                            );
                            handleImapChange(
                              'kerberos_keytab_file_path',
                              config.imap.kerberos_keytab_file_path || '',
                            );
                            return;
                          }
                          handleImapChange('kerberos_principal', undefined);
                          handleImapChange(
                            'kerberos_keytab_file_path',
                            undefined,
                          );
                        }}
                      />
                      <Label htmlFor='imap-hdfs-kerberos'>
                        {t('installer.hdfsKerberos')}
                      </Label>
                    </div>
                    {(Boolean(config.imap.kerberos_principal) ||
                      Boolean(config.imap.kerberos_keytab_file_path)) && (
                      <div className='grid grid-cols-2 gap-4 rounded-md border bg-muted/30 p-3'>
                        <div className='space-y-2'>
                          <Label>{t('installer.kerberosPrincipal')}</Label>
                          <Input
                            value={config.imap.kerberos_principal || ''}
                            onChange={(e) =>
                              handleImapChange(
                                'kerberos_principal',
                                e.target.value,
                              )
                            }
                            placeholder='hdfs/namenode@EXAMPLE.COM'
                          />
                        </div>
                        <div className='space-y-2'>
                          <Label>{t('installer.kerberosKeytabPath')}</Label>
                          <Input
                            value={config.imap.kerberos_keytab_file_path || ''}
                            onChange={(e) =>
                              handleImapChange(
                                'kerberos_keytab_file_path',
                                e.target.value,
                              )
                            }
                            placeholder='/etc/security/keytabs/hdfs.keytab'
                          />
                        </div>
                      </div>
                    )}

                    <div className='space-y-2'>
                      <Label>{t('installer.hdfsSitePath')}</Label>
                      <Input
                        value={config.imap.hdfs_site_path || ''}
                        onChange={(e) =>
                          handleImapChange('hdfs_site_path', e.target.value)
                        }
                        placeholder='/etc/hadoop/conf/hdfs-site.xml'
                      />
                    </div>
                  </div>
                )}

                {config.imap.storage_type === 'OSS' && (
                  <div className='space-y-4'>
                    <div className='rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-200'>
                      {t('installer.runtimeStorage.ossLibHint')}
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('installer.endpoint')}</Label>
                      <Input
                        data-testid='install-imap-endpoint'
                        value={config.imap.storage_endpoint || ''}
                        onChange={(e) =>
                          handleImapChange('storage_endpoint', e.target.value)
                        }
                        placeholder='oss-cn-hangzhou.aliyuncs.com'
                      />
                    </div>
                    <div className='grid grid-cols-2 gap-4'>
                      <div className='space-y-2'>
                        <Label>fs.oss.accessKeyId</Label>
                        <Input
                          data-testid='install-imap-access-key'
                          type='password'
                          value={config.imap.storage_access_key || ''}
                          onChange={(e) =>
                            handleImapChange('storage_access_key', e.target.value)
                          }
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label>fs.oss.accessKeySecret</Label>
                        <Input
                          data-testid='install-imap-secret-key'
                          type='password'
                          value={config.imap.storage_secret_key || ''}
                          onChange={(e) =>
                            handleImapChange('storage_secret_key', e.target.value)
                          }
                        />
                      </div>
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('installer.bucket')}</Label>
                      <Input
                        data-testid='install-imap-bucket'
                        value={config.imap.storage_bucket || ''}
                        onChange={(e) =>
                          handleImapChange('storage_bucket', e.target.value)
                        }
                        placeholder='your-bucket'
                      />
                    </div>
                  </div>
                )}

                {config.imap.storage_type === 'S3' && (
                  <div className='space-y-4'>
                    <div className='space-y-2'>
                      <Label>{t('installer.s3CredentialsProvider')}</Label>
                      <Select
                        value={
                          config.imap.s3_credentials_provider ||
                          'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider'
                        }
                        onValueChange={(value) =>
                          handleImapChange('s3_credentials_provider', value)
                        }
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value='org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider'>
                            {t('installer.s3ProviderSimple')}
                          </SelectItem>
                          <SelectItem value='org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider'>
                            {t('installer.s3ProviderInstance')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('installer.endpoint')}</Label>
                      <Input
                        data-testid='install-imap-endpoint'
                        value={config.imap.storage_endpoint || ''}
                        onChange={(e) =>
                          handleImapChange('storage_endpoint', e.target.value)
                        }
                        placeholder='http://127.0.0.1:9000'
                      />
                    </div>
                    {(config.imap.s3_credentials_provider ||
                      'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider') !==
                      'org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider' && (
                      <div className='grid grid-cols-2 gap-4'>
                        <div className='space-y-2'>
                          <Label>fs.s3a.access.key</Label>
                          <Input
                            data-testid='install-imap-access-key'
                            type='password'
                            value={config.imap.storage_access_key || ''}
                            onChange={(e) =>
                              handleImapChange('storage_access_key', e.target.value)
                            }
                          />
                        </div>
                        <div className='space-y-2'>
                          <Label>fs.s3a.secret.key</Label>
                          <Input
                            data-testid='install-imap-secret-key'
                            type='password'
                            value={config.imap.storage_secret_key || ''}
                            onChange={(e) =>
                              handleImapChange('storage_secret_key', e.target.value)
                            }
                          />
                        </div>
                      </div>
                    )}
                    <div className='space-y-2'>
                      <Label>{t('installer.bucket')}</Label>
                      <Input
                        data-testid='install-imap-bucket'
                        value={config.imap.storage_bucket || ''}
                        onChange={(e) =>
                          handleImapChange('storage_bucket', e.target.value)
                        }
                        placeholder='s3a://bucket'
                      />
                    </div>
                  </div>
                )}
              </>
            )}

            {imapExternalEnabled && (
              <>
                <div className='flex items-center justify-between rounded-md border p-3'>
                  <div className='space-y-1'>
                    <div className='text-sm font-medium'>
                      {t('installer.runtimeStorage.validateConnectivity')}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {t('installer.runtimeStorage.validationHint')}
                    </div>
                  </div>
                  <Button
                    data-testid='install-imap-validate'
                    variant='outline'
                    size='sm'
                    onClick={() => validateStorage('imap')}
                    disabled={validatingKind === 'imap'}
                  >
                    {validatingKind === 'imap' && (
                      <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                    )}
                    {t('installer.runtimeStorage.validateNow')}
                  </Button>
                </div>

                {storageValidation.imap && (
                  <div
                    data-testid='install-imap-validation-result'
                    className='rounded-md border p-3 text-sm space-y-2'
                  >
                    <div className='flex items-center gap-2 font-medium'>
                      {storageValidation.imap.success ? (
                        <CheckCircle2 className='h-4 w-4 text-green-600' />
                      ) : (
                        <AlertTriangle className='h-4 w-4 text-amber-600' />
                      )}
                      <span>
                        {storageValidation.imap.success
                          ? t('installer.runtimeStorage.validationPassed')
                          : t('installer.runtimeStorage.validationWarning')}
                      </span>
                    </div>
                    {storageValidation.imap.warning && (
                      <div className='text-muted-foreground'>
                        {storageValidation.imap.warning}
                      </div>
                    )}
                    <div className='space-y-1'>
                      {storageValidation.imap.hosts.map((host) => (
                        <div key={host.host_id}>
                          <span className='font-medium'>
                            {host.host_name || host.host_id}
                          </span>
                          {' · '}
                          <span>{host.message}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  );
}

export default ConfigStep;
