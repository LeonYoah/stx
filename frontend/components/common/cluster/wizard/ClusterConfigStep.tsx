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
 * Cluster Configuration Step Component
 * 集群安装配置步骤组件
 *
 * High-density configuration form for package source, ports, JVM, and runtime storage.
 * 紧凑高密度的安装配置表单，涵盖安装包源、端口映射、JVM 堆内存与运行时存储。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/arc/input/input';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Button} from '@/components/arc/button/button';
import {Alert} from '@/components/arc/alert/alert';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {
  CheckCircle2,
  Download,
  ExternalLink,
  Copy,
  HardDrive,
  Database,
  Sliders,
} from 'lucide-react';
import {DeploymentMode} from '@/lib/services/cluster/types';
import {RuntimeAdvancedConfigCard} from '@/components/common/installer/RuntimeAdvancedConfigCard';
import {HelpHint} from '@/components/common/HelpHint';
import {JavaHomePicker} from '@/components/common/java/JavaHomePicker';
import type {
  MirrorSource,
  CheckpointStorageType,
  IMAPStorageType,
  PackageInfo,
  SeaTunnelVersionCapabilities,
} from '@/lib/services/installer/types';
import {ClusterDeployConfig, HostWithRole} from './types';

/** IMAP 外部存储开启时的默认后端 / Default IMAP backend when external storage is enabled */
const DEFAULT_IMAP_STORAGE: Exclude<IMAPStorageType, 'DISABLED'> = 'LOCAL_FILE';

/** 远端存储类型选项（Checkpoint/IMAP 共用）/ Remote storage options shared by Checkpoint & IMAP */
const REMOTE_STORAGE_OPTIONS = ['LOCAL_FILE', 'HDFS', 'OSS', 'S3'] as const;

interface ClusterConfigStepProps {
  /** Deploy config / 部署配置 */
  config: ClusterDeployConfig;
  /** Update config callback / 更新配置回调 */
  updateConfig: (updates: Partial<ClusterDeployConfig>) => void;
  /** Selected hosts / 已选主机 */
  selectedHosts: HostWithRole[];
  /** Local packages list / 本地包列表 */
  localPackages: PackageInfo[];
  /** Version capabilities / 版本能力 */
  versionCapabilities?: SeaTunnelVersionCapabilities | null;
  /** HTTP service supported / 是否支持 HTTP 服务 */
  httpServiceSupported: boolean;
  /** Event channel supported (SeaTunnel 3.0+) / 是否支持事件通道（SeaTunnel 3.0+） */
  eventChannelSupported: boolean;
  /** Mark the event-channel checkbox as user-controlled / 标记事件通道勾选已被用户改过 */
  onEventChannelUserSet: () => void;
  /** Apply checkpoint config to IMAP / 同步检查点配置到 IMAP */
  applyCheckpointToImap: () => void;
  /** Update one host JDK (AC1) / 更新单个主机 JDK（AC1） */
  setHostJavaHome: (hostId: number, javaHome: string) => void;
  /** Fill all selected hosts with one JDK (AC1a) / 一键填到全部已选主机（AC1a） */
  fillAllJavaHome: (javaHome: string) => void;
}

export function ClusterConfigStep({
  config,
  updateConfig,
  selectedHosts,
  localPackages,
  versionCapabilities,
  httpServiceSupported,
  eventChannelSupported,
  onEventChannelUserSet,
  applyCheckpointToImap,
  setHostJavaHome,
  fillAllJavaHome,
}: ClusterConfigStepProps) {
  const t = useTranslations();

  // Check if current version package is available locally / 检查当前版本的安装包是否在本地可用
  const currentPackage = localPackages.find(
    (pkg) => pkg.version === config.version,
  );
  const isPackageLocal = !!currentPackage;

  const checkpointNeedsSharedWarning =
    selectedHosts.length > 1 &&
    config.checkpoint.storage_type === 'LOCAL_FILE';

  /** 外部 IMAP 是否开启（非 DISABLED）/ Whether external IMAP storage is enabled */
  const imapExternalEnabled = config.imap.storage_type !== 'DISABLED';

  /** 存储类型的本地化标签 / Localized label for a storage backend */
  const storageTypeLabel = (value: string) =>
    value === 'LOCAL_FILE' ? t('installer.runtimeStorage.localFile') : value === 'OSS' ? 'Aliyun OSS' : value === 'S3' ? 'AWS S3' : value;

  /**
   * 切换外部 IMAP：关闭写回 DISABLED；开启时默认 LOCAL_FILE 并展开类型选择
   * Toggle external IMAP: off writes DISABLED; on defaults to LOCAL_FILE and reveals type fields
   */
  const handleImapExternalToggle = (enabled: boolean) => {
    if (!enabled) {
      updateConfig({
        imap: {...config.imap, storage_type: 'DISABLED'},
      });
      return;
    }
    updateConfig({
      imap: {
        ...config.imap,
        storage_type:
          config.imap.storage_type === 'DISABLED'
            ? DEFAULT_IMAP_STORAGE
            : config.imap.storage_type,
        namespace: config.imap.namespace || '/tmp/seatunnel/imap/',
      },
    });
  };

  /**
   * 渲染 HDFS / OSS / S3 远端参数表单
   * Render remote backend fields for HDFS / OSS / S3
   */
  const renderRemoteStorageFields = (
    kind: 'checkpoint' | 'imap',
    storageType: string,
  ) => {
    const store = kind === 'checkpoint' ? config.checkpoint : config.imap;
    const patch = (updates: Record<string, unknown>) => {
      if (kind === 'checkpoint') {
        updateConfig({checkpoint: {...config.checkpoint, ...updates}});
      } else {
        updateConfig({imap: {...config.imap, ...updates}});
      }
    };

    if (storageType === 'HDFS') {
      const haEnabled = Boolean(store.hdfs_ha_enabled);
      const kerberosEnabled =
        Boolean(store.kerberos_principal) ||
        Boolean(store.kerberos_keytab_file_path);

      return (
        <div className='space-y-2.5'>
          <Checkbox
            checked={haEnabled}
            onCheckedChange={(checked) =>
              patch({hdfs_ha_enabled: checked === true})
            }
            label={t('installer.hdfsHAMode')}
          />

          {!haEnabled && (
            <div className='grid grid-cols-2 gap-2.5'>
              <Input
                label={t('installer.hdfsNameNodeHost')}
                value={store.hdfs_namenode_host || ''}
                onChange={(e) => patch({hdfs_namenode_host: e.target.value})}
                placeholder='namenode.example.com'
                className='font-mono text-xs'
              />
              <Input
                label={t('installer.hdfsNameNodePort')}
                type='number'
                value={store.hdfs_namenode_port || ''}
                onChange={(e) =>
                  patch({hdfs_namenode_port: parseInt(e.target.value) || 0})
                }
                placeholder='8020'
                className='font-mono text-xs tabular-nums'
              />
            </div>
          )}

          {haEnabled && (
            <div className='space-y-2.5'>
              <div className='grid grid-cols-2 gap-2.5'>
                <Input
                  label={t('installer.hdfsNameServices')}
                  value={store.hdfs_name_services || ''}
                  onChange={(e) =>
                    patch({hdfs_name_services: e.target.value})
                  }
                  placeholder='mycluster'
                  className='font-mono text-xs'
                />
                <Input
                  label={t('installer.hdfsHANamenodes')}
                  value={store.hdfs_ha_namenodes || ''}
                  onChange={(e) => patch({hdfs_ha_namenodes: e.target.value})}
                  placeholder='nn1,nn2'
                  className='font-mono text-xs'
                />
              </div>
              <div className='grid grid-cols-2 gap-2.5'>
                <Input
                  label={t('installer.hdfsNamenodeRPCAddress1')}
                  value={store.hdfs_namenode_rpc_address_1 || ''}
                  onChange={(e) =>
                    patch({hdfs_namenode_rpc_address_1: e.target.value})
                  }
                  placeholder='nn1-host:8020'
                  className='font-mono text-xs'
                />
                <Input
                  label={t('installer.hdfsNamenodeRPCAddress2')}
                  value={store.hdfs_namenode_rpc_address_2 || ''}
                  onChange={(e) =>
                    patch({hdfs_namenode_rpc_address_2: e.target.value})
                  }
                  placeholder='nn2-host:8020'
                  className='font-mono text-xs'
                />
              </div>
            </div>
          )}

          <Input
            label={t('installer.hdfsSitePath')}
            value={store.hdfs_site_path || ''}
            onChange={(e) => patch({hdfs_site_path: e.target.value})}
            placeholder='/etc/hadoop/conf/hdfs-site.xml'
            className='font-mono text-xs'
          />

          <Checkbox
            checked={kerberosEnabled}
            onCheckedChange={(checked) => {
              if (checked === true) {
                patch({
                  kerberos_principal: store.kerberos_principal || '',
                  kerberos_keytab_file_path:
                    store.kerberos_keytab_file_path || '',
                });
                return;
              }
              patch({
                kerberos_principal: undefined,
                kerberos_keytab_file_path: undefined,
              });
            }}
            label={t('installer.hdfsKerberos')}
          />

          {kerberosEnabled && (
            <div className='grid grid-cols-2 gap-2.5'>
              <Input
                label={t('installer.kerberosPrincipal')}
                value={store.kerberos_principal || ''}
                onChange={(e) => patch({kerberos_principal: e.target.value})}
                placeholder='hdfs/namenode@EXAMPLE.COM'
                className='font-mono text-xs'
              />
              <Input
                label={t('installer.kerberosKeytabPath')}
                value={store.kerberos_keytab_file_path || ''}
                onChange={(e) =>
                  patch({kerberos_keytab_file_path: e.target.value})
                }
                placeholder='/etc/security/keytabs/hdfs.keytab'
                className='font-mono text-xs'
              />
            </div>
          )}
        </div>
      );
    }

    if (storageType === 'OSS') {
      return (
        <div className='grid grid-cols-2 gap-2.5'>
          <Input
            label={t('installer.endpoint')}
            value={store.storage_endpoint || ''}
            onChange={(e) => patch({storage_endpoint: e.target.value})}
            placeholder='oss-cn-hangzhou.aliyuncs.com'
            className='font-mono text-xs'
          />
          <Input
            label={t('installer.bucket')}
            value={store.storage_bucket || ''}
            onChange={(e) => patch({storage_bucket: e.target.value})}
            placeholder='your-bucket'
            className='font-mono text-xs'
          />
          <Input
            label='fs.oss.accessKeyId'
            type='password'
            value={store.storage_access_key || ''}
            onChange={(e) => patch({storage_access_key: e.target.value})}
            className='font-mono text-xs'
          />
          <Input
            label='fs.oss.accessKeySecret'
            type='password'
            value={store.storage_secret_key || ''}
            onChange={(e) => patch({storage_secret_key: e.target.value})}
            className='font-mono text-xs'
          />
        </div>
      );
    }

    if (storageType === 'S3') {
      const provider =
        store.s3_credentials_provider ||
        'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider';
      const instanceProfile =
        provider === 'org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider';
      return (
        <div className='space-y-2.5'>
          <div className='space-y-1'>
            <span className='block text-xs font-medium text-foreground'>
              {t('installer.s3CredentialsProvider')}
            </span>
            <SegmentedControl
              label={t('installer.s3CredentialsProvider')}
              value={provider}
              onValueChange={(value) => patch({s3_credentials_provider: value})}
              options={[
                {
                  value: 'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider',
                  label: t('installer.s3ProviderSimple'),
                },
                {
                  value: 'org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider',
                  label: t('installer.s3ProviderInstance'),
                },
              ]}
            />
          </div>
          <div className='grid grid-cols-2 gap-2.5'>
            <Input
              label={t('installer.bucket')}
              value={store.storage_bucket || ''}
              onChange={(e) => patch({storage_bucket: e.target.value})}
              placeholder='s3a://bucket'
              className='font-mono text-xs'
            />
            <Input
              label={t('installer.endpoint')}
              value={store.storage_endpoint || ''}
              onChange={(e) => patch({storage_endpoint: e.target.value})}
              placeholder='http://127.0.0.1:9000'
              className='font-mono text-xs'
            />
          </div>
          {!instanceProfile && (
            <div className='grid grid-cols-2 gap-2.5'>
              <Input
                label='fs.s3a.access.key'
                type='password'
                value={store.storage_access_key || ''}
                onChange={(e) => patch({storage_access_key: e.target.value})}
                className='font-mono text-xs'
              />
              <Input
                label='fs.s3a.secret.key'
                type='password'
                value={store.storage_secret_key || ''}
                onChange={(e) => patch({storage_secret_key: e.target.value})}
                className='font-mono text-xs'
              />
            </div>
          )}
        </div>
      );
    }

    return null;
  };

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      <div className='flex-1 min-h-0 overflow-y-auto pr-4'>
        <div className='max-w-4xl mx-auto py-1'>
          {/* 1. Package Status & Mirror / 安装包就绪与镜像源 */}
          <section className='pb-4 flex flex-wrap items-center justify-between gap-3'>
            <div className='flex items-center gap-2.5 min-w-0'>
              <Download className='h-4 w-4 shrink-0 text-muted-foreground' />
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  <span className='text-xs font-semibold'>
                    {t('cluster.wizard.packageDistribution')}
                  </span>
                  {isPackageLocal ? (
                    <SoftChip tone='emerald'>
                      <CheckCircle2 className='h-3 w-3' aria-hidden />
                      {t('cluster.wizard.localPackageReady')}
                    </SoftChip>
                  ) : (
                    <SoftChip tone='amber'>
                      {t('cluster.wizard.downloadOnDeploy')}
                    </SoftChip>
                  )}
                </div>
                <p className='text-[11px] text-muted-foreground font-mono mt-0.5 tabular-nums'>
                  {isPackageLocal
                    ? `${currentPackage.file_name} (${(currentPackage.file_size / 1024 / 1024).toFixed(1)} MB)`
                    : t('cluster.wizard.packagePullOnDeploy', {
                        version: config.version,
                      })}
                </p>
              </div>
            </div>

            <div className='flex items-center gap-3 shrink-0'>
              {!isPackageLocal && (
                <div className='flex items-center gap-1.5'>
                  <span className='text-xs text-muted-foreground'>
                    {t('cluster.wizard.mirrorSourceLabel')}
                  </span>
                  <SegmentedControl
                    label={t('cluster.wizard.mirrorSourceLabel')}
                    value={config.mirror}
                    onValueChange={(value) =>
                      updateConfig({mirror: value as MirrorSource})
                    }
                    options={[
                      {value: 'aliyun', label: t('installer.aliyun')},
                      {value: 'huaweicloud', label: t('installer.huaweicloud')},
                      {value: 'apache', label: t('installer.apache')},
                    ]}
                  />
                </div>
              )}
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => window.open('/packages', '_blank')}
              >
                {t('cluster.wizard.packageManage')}
                <ExternalLink className='size-3' aria-hidden />
              </Button>
            </div>
          </section>

          {/* JDK path per host (AC1 / AC1a / AC1b) / 每主机 JDK 路径 */}
          {selectedHosts.length > 0 && (
            <section
              className='border-t-[0.5px] border-border/80 py-4'
              data-testid='cluster-wizard-jdk-card'
            >
              <div className='flex items-center justify-between gap-2 pb-2.5'>
                <h3 className='text-xs font-semibold text-foreground'>
                  {t('javaHome.label')}
                </h3>
                {selectedHosts.length > 1 && (
                  <Button
                    type='button'
                    variant='secondary'
                    size='sm'
                    onClick={() => {
                      const first = selectedHosts[0]?.javaHome || '';
                      fillAllJavaHome(first);
                    }}
                    data-testid='cluster-wizard-jdk-fill-all'
                  >
                    {t('javaHome.fillAll')}
                  </Button>
                )}
              </div>
              <div className='divide-y-[0.5px] divide-border/80'>
                {selectedHosts.map((h) => (
                  <div key={h.host.id} className='py-2.5 space-y-2'>
                    <div className='text-xs font-medium flex items-center gap-2'>
                      <span>{h.host.name}</span>
                      {h.host.ip_address && (
                        <NeutralChip className='font-mono tabular-nums'>
                          {h.host.ip_address}
                        </NeutralChip>
                      )}
                    </div>
                    <JavaHomePicker
                      id={`jdk-host-${h.host.id}`}
                      hostId={h.host.id}
                      value={h.javaHome || ''}
                      onChange={({javaHome}) => setHostJavaHome(h.host.id, javaHome)}
                    />
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* 2. Ports & JVM / 端口与 JVM 资源 */}
          <section className='border-t-[0.5px] border-border/80 py-4 grid grid-cols-1 md:grid-cols-2 gap-6'>
            {/* Ports Config / 端口配置 */}
            <div className='space-y-2.5'>
              <h3 className='text-xs font-semibold flex items-center gap-1.5 text-foreground'>
                <Sliders className='h-3.5 w-3.5 text-muted-foreground' />
                {t('cluster.wizard.portMapping')}
              </h3>
              <div className='grid grid-cols-2 gap-2.5'>
                <Input
                  label={t('cluster.wizard.clusterCommPort')}
                  type='number'
                  value={config.clusterPort}
                  onChange={(e) =>
                    updateConfig({
                      clusterPort: parseInt(e.target.value) || 5801,
                    })
                  }
                  min={1024}
                  max={65535}
                  placeholder='5801'
                  className='font-mono text-xs tabular-nums'
                />
                {config.deploymentMode === DeploymentMode.SEPARATED && (
                  <Input
                    label={t('cluster.wizard.workerPort')}
                    type='number'
                    value={config.workerPort}
                    onChange={(e) =>
                      updateConfig({
                        workerPort: parseInt(e.target.value) || 5802,
                      })
                    }
                    min={1024}
                    max={65535}
                    placeholder='5802'
                    className='font-mono text-xs tabular-nums'
                  />
                )}
              </div>

              {/* HTTP Port / HTTP 端口 */}
              <div className='space-y-1.5 pt-2 border-t-[0.5px] border-border/80'>
                <Checkbox
                  checked={httpServiceSupported && config.runtime.enable_http}
                  disabled={!httpServiceSupported}
                  onCheckedChange={(checked) =>
                    updateConfig({
                      runtime: {
                        ...config.runtime,
                        enable_http: checked === true,
                      },
                    })
                  }
                  label={t('cluster.wizard.enableRestService')}
                />
                <Input
                  label={t('cluster.wizard.masterWebUiPort')}
                  type='number'
                  value={config.httpPort}
                  onChange={(e) =>
                    updateConfig({
                      httpPort: parseInt(e.target.value) || 8080,
                    })
                  }
                  min={1024}
                  max={65535}
                  placeholder='8080'
                  disabled={!httpServiceSupported || !config.runtime.enable_http}
                  className='font-mono text-xs tabular-nums'
                />
              </div>

              {eventChannelSupported && (
                <div className='pt-2 border-t-[0.5px] border-border/80'>
                  <Checkbox
                    data-testid='cluster-wizard-event-channel'
                    checked={config.runtime.enable_event_channel}
                    onCheckedChange={(checked) => {
                      onEventChannelUserSet();
                      updateConfig({
                        runtime: {
                          ...config.runtime,
                          enable_event_channel: checked === true,
                        },
                      });
                    }}
                    label={t('cluster.wizard.enableEventChannel')}
                  />
                </div>
              )}

              {/* stx-java-proxy Port / stx-java-proxy 端口 */}
              <div className='space-y-1.5 pt-2 border-t-[0.5px] border-border/80'>
                <p
                  className='text-[11px] leading-4 text-muted-foreground'
                  data-testid='cluster-wizard-java-proxy-desc'
                >
                  {t('installer.javaProxy.description')}
                </p>
                <Input
                  label={t('cluster.wizard.javaProxyPort')}
                  type='number'
                  value={config.javaProxyPort}
                  onChange={(e) =>
                    updateConfig({
                      javaProxyPort: parseInt(e.target.value) || 18080,
                    })
                  }
                  min={1024}
                  max={65535}
                  placeholder='18080'
                  className='font-mono text-xs tabular-nums'
                />
              </div>
            </div>

            {/* JVM Heap / JVM 堆内存（单位 MB 写进 label） */}
            <div className='space-y-2.5'>
              <h3 className='text-xs font-semibold flex items-center gap-1.5 text-foreground'>
                <HardDrive className='h-3.5 w-3.5 text-muted-foreground' />
                {t('cluster.wizard.jvmHeapConfigMb')}
              </h3>
              {config.deploymentMode === DeploymentMode.HYBRID ? (
                <Input
                  label={`${t('cluster.wizard.hybridNodeHeap')} (MB)`}
                  type='number'
                  value={config.jvm.hybrid_heap_size}
                  onChange={(e) =>
                    updateConfig({
                      jvm: {
                        ...config.jvm,
                        hybrid_heap_size: parseInt(e.target.value) || 2048,
                      },
                    })
                  }
                  min={512}
                  max={131072}
                  step={64}
                  className='font-mono text-xs tabular-nums'
                />
              ) : (
                <div className='grid grid-cols-2 gap-2.5'>
                  <Input
                    label={`${t('cluster.wizard.masterHeap')} (MB)`}
                    type='number'
                    value={config.jvm.master_heap_size}
                    onChange={(e) =>
                      updateConfig({
                        jvm: {
                          ...config.jvm,
                          master_heap_size: parseInt(e.target.value) || 2048,
                        },
                      })
                    }
                    min={512}
                    max={131072}
                    step={64}
                    className='font-mono text-xs tabular-nums'
                  />
                  <Input
                    label={`${t('cluster.wizard.workerHeap')} (MB)`}
                    type='number'
                    value={config.jvm.worker_heap_size}
                    onChange={(e) =>
                      updateConfig({
                        jvm: {
                          ...config.jvm,
                          worker_heap_size: parseInt(e.target.value) || 2048,
                        },
                      })
                    }
                    min={512}
                    max={131072}
                    step={64}
                    className='font-mono text-xs tabular-nums'
                  />
                </div>
              )}
            </div>
          </section>

          {/* 3. Runtime Engine Settings / 运行时高级引擎配置（向导内默认折叠） */}
          <RuntimeAdvancedConfigCard
            compact
            defaultCollapsed
            version={config.version}
            capabilities={versionCapabilities}
            runtime={config.runtime}
            onChange={(updates) =>
              updateConfig({
                runtime: {...config.runtime, ...updates},
              })
            }
          />

          {/* 4. Checkpoint 配置 / Checkpoint configuration */}
          <section className='border-t-[0.5px] border-border/80 py-4 space-y-3'>
            <h3 className='text-xs font-semibold flex items-center gap-1.5 text-foreground'>
              <Database className='h-3.5 w-3.5 text-muted-foreground' />
              {t('installer.checkpointConfig')}
            </h3>

            {checkpointNeedsSharedWarning && (
              <Alert
                tone='warning'
                title={t('installer.runtimeStorage.checkpointLocalWarning')}
              />
            )}

            <div className='space-y-2'>
              <div className='flex items-center gap-2'>
                <span className='text-xs font-medium text-foreground'>
                  {t('installer.storageType')}
                </span>
                {config.checkpoint.storage_type !== 'LOCAL_FILE' ? (
                  <HelpHint
                    label={t('installer.storageType')}
                    content={t(
                      'installer.runtimeStorage.remoteStorageInstallHint',
                    )}
                    data-testid='cluster-wizard-checkpoint-remote-hint'
                  />
                ) : null}
              </div>
              <SegmentedControl
                label={t('installer.storageType')}
                value={config.checkpoint.storage_type}
                onValueChange={(value) =>
                  updateConfig({
                    checkpoint: {
                      ...config.checkpoint,
                      storage_type: value as CheckpointStorageType,
                    },
                  })
                }
                options={REMOTE_STORAGE_OPTIONS.map((value) => ({
                  value,
                  label: storageTypeLabel(value),
                }))}
              />
            </div>

            <Input
              label={t('installer.namespace')}
              value={config.checkpoint.namespace}
              onChange={(e) =>
                updateConfig({
                  checkpoint: {
                    ...config.checkpoint,
                    namespace: e.target.value,
                  },
                })
              }
              placeholder={
                config.checkpoint.storage_type === 'LOCAL_FILE'
                  ? '/tmp/seatunnel/checkpoint/'
                  : '/seatunnel/checkpoint/'
              }
              className='font-mono text-xs'
            />

            {config.checkpoint.storage_type !== 'LOCAL_FILE' && (
              <div className='space-y-2 pt-2 border-t-[0.5px] border-border/80'>
                {renderRemoteStorageFields(
                  'checkpoint',
                  config.checkpoint.storage_type,
                )}
              </div>
            )}
          </section>

          {/* 5. IMAP 状态存储配置 / IMAP State Storage Configuration */}
          <section className='border-t-[0.5px] border-border/80 py-4 space-y-3'>
            <div className='flex items-center justify-between gap-3'>
              <div className='flex items-center gap-2 min-w-0'>
                <h3 className='text-xs font-semibold flex items-center gap-1.5 text-foreground'>
                  <Database className='h-3.5 w-3.5 text-muted-foreground' />
                  {t('installer.runtimeStorage.imapTitle')}
                </h3>
                <HelpHint
                  label={t('installer.runtimeStorage.imapTitle')}
                  content={t('installer.runtimeStorage.imapGuidanceMeta')}
                  data-testid='cluster-wizard-imap-hint'
                />
                <span className='text-[11px] text-muted-foreground hidden sm:inline'>
                  ({t('installer.runtimeStorage.imapSubtitle')})
                </span>
              </div>
              <div className='flex items-center gap-2.5 shrink-0'>
                {imapExternalEnabled ? (
                  <SoftChip tone='emerald'>
                    {t('installer.runtimeStorage.imapExternalOn')}
                  </SoftChip>
                ) : (
                  <NeutralChip>
                    {t('installer.runtimeStorage.imapInMemory')}
                  </NeutralChip>
                )}
                {/* 表单提交项用 Checkbox 而非 Switch / Form input uses Checkbox, not Switch */}
                <Checkbox
                  id='imap-external-toggle'
                  checked={imapExternalEnabled}
                  onCheckedChange={(checked) =>
                    handleImapExternalToggle(checked === true)
                  }
                  label={t('installer.runtimeStorage.imapEnableLabel')}
                />
              </div>
            </div>

            {/* 未开启外部存储：高信噪比纯内存说明 / When disabled: high-signal in-memory notice */}
            {!imapExternalEnabled ? (
              <Alert
                tone='info'
                title={t('installer.runtimeStorage.imapInMemoryTitle')}
              >
                <span data-testid='cluster-wizard-imap-suggestion'>
                  {t('installer.runtimeStorage.imapStreamingSuggestion')}
                </span>
              </Alert>
            ) : (
              /* 已开启外部存储：展开存储类型与配置 / When enabled: reveal storage options */
              <>
                <Alert
                  tone='info'
                  title={t('installer.runtimeStorage.imapExternalOn')}
                >
                  <div className='flex flex-wrap items-center justify-between gap-3'>
                    <span data-testid='cluster-wizard-imap-suggestion'>
                      {t('installer.runtimeStorage.imapBatchSuggestion')}
                    </span>
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={applyCheckpointToImap}
                    >
                      <Copy className='size-3' aria-hidden />
                      {t('installer.runtimeStorage.applyToImap')}
                    </Button>
                  </div>
                </Alert>

                {selectedHosts.length > 1 &&
                  config.imap.storage_type === 'LOCAL_FILE' && (
                    <Alert
                      tone='warning'
                      title={t('installer.runtimeStorage.checkpointLocalWarning')}
                    />
                  )}

                <div className='space-y-2'>
                  <div className='flex items-center gap-2'>
                    <span className='text-xs font-medium text-foreground'>
                      {t('installer.storageType')}
                    </span>
                    {config.imap.storage_type !== 'LOCAL_FILE' ? (
                      <HelpHint
                        label={t('installer.storageType')}
                        content={t(
                          'installer.runtimeStorage.remoteStorageInstallHint',
                        )}
                        data-testid='cluster-wizard-imap-remote-hint'
                      />
                    ) : null}
                  </div>
                  <SegmentedControl
                    label={t('installer.storageType')}
                    value={config.imap.storage_type}
                    onValueChange={(value) => {
                      updateConfig({
                        imap: {
                          ...config.imap,
                          storage_type: value as IMAPStorageType,
                        },
                      });
                    }}
                    options={REMOTE_STORAGE_OPTIONS.map((value) => ({
                      value,
                      label: storageTypeLabel(value),
                    }))}
                  />
                </div>

                <Input
                  label={t('installer.namespace')}
                  value={config.imap.namespace}
                  onChange={(e) =>
                    updateConfig({
                      imap: {
                        ...config.imap,
                        namespace: e.target.value,
                      },
                    })
                  }
                  placeholder={
                    config.imap.storage_type === 'LOCAL_FILE'
                      ? '/tmp/seatunnel/imap/'
                      : '/seatunnel/imap/'
                  }
                  className='font-mono text-xs'
                />

                {config.imap.storage_type !== 'LOCAL_FILE' &&
                  config.imap.storage_type !== 'DISABLED' && (
                    <div className='space-y-2 pt-2 border-t-[0.5px] border-border/80'>
                      {renderRemoteStorageFields(
                        'imap',
                        config.imap.storage_type,
                      )}
                    </div>
                  )}
              </>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

export default ClusterConfigStep;
