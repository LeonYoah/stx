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
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Checkbox} from '@/components/ui/checkbox';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Switch} from '@/components/ui/switch';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  CheckCircle2,
  AlertTriangle,
  Download,
  ExternalLink,
  Copy,
  HardDrive,
  Database,
  Sliders,
  Info,
} from 'lucide-react';
import {DeploymentMode} from '@/lib/services/cluster/types';
import {RuntimeAdvancedConfigCard} from '@/components/common/installer/RuntimeAdvancedConfigCard';
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
  /** Apply checkpoint config to IMAP / 同步检查点配置到 IMAP */
  applyCheckpointToImap: () => void;
}

export function ClusterConfigStep({
  config,
  updateConfig,
  selectedHosts,
  localPackages,
  versionCapabilities,
  httpServiceSupported,
  applyCheckpointToImap,
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
          <label className='flex items-center gap-2 text-xs'>
            <Checkbox
              checked={haEnabled}
              onCheckedChange={(checked) =>
                patch({hdfs_ha_enabled: checked === true})
              }
              className='h-3.5 w-3.5'
            />
            {t('installer.hdfsHAMode')}
          </label>

          {!haEnabled && (
            <div className='grid grid-cols-2 gap-2.5'>
              <div className='space-y-1'>
                <Label className='text-[11px]'>
                  {t('installer.hdfsNameNodeHost')}
                </Label>
                <Input
                  value={store.hdfs_namenode_host || ''}
                  onChange={(e) => patch({hdfs_namenode_host: e.target.value})}
                  placeholder='namenode.example.com'
                  className='h-7 text-xs font-mono'
                />
              </div>
              <div className='space-y-1'>
                <Label className='text-[11px]'>
                  {t('installer.hdfsNameNodePort')}
                </Label>
                <Input
                  type='number'
                  value={store.hdfs_namenode_port || ''}
                  onChange={(e) =>
                    patch({hdfs_namenode_port: parseInt(e.target.value) || 0})
                  }
                  placeholder='8020'
                  className='h-7 text-xs font-mono'
                />
              </div>
            </div>
          )}

          {haEnabled && (
            <div className='space-y-2.5 rounded-md border bg-muted/20 p-2.5'>
              <div className='grid grid-cols-2 gap-2.5'>
                <div className='space-y-1'>
                  <Label className='text-[11px]'>
                    {t('installer.hdfsNameServices')}
                  </Label>
                  <Input
                    value={store.hdfs_name_services || ''}
                    onChange={(e) =>
                      patch({hdfs_name_services: e.target.value})
                    }
                    placeholder='mycluster'
                    className='h-7 text-xs font-mono'
                  />
                </div>
                <div className='space-y-1'>
                  <Label className='text-[11px]'>
                    {t('installer.hdfsHANamenodes')}
                  </Label>
                  <Input
                    value={store.hdfs_ha_namenodes || ''}
                    onChange={(e) => patch({hdfs_ha_namenodes: e.target.value})}
                    placeholder='nn1,nn2'
                    className='h-7 text-xs font-mono'
                  />
                </div>
              </div>
              <div className='grid grid-cols-2 gap-2.5'>
                <div className='space-y-1'>
                  <Label className='text-[11px]'>
                    {t('installer.hdfsNamenodeRPCAddress1')}
                  </Label>
                  <Input
                    value={store.hdfs_namenode_rpc_address_1 || ''}
                    onChange={(e) =>
                      patch({hdfs_namenode_rpc_address_1: e.target.value})
                    }
                    placeholder='nn1-host:8020'
                    className='h-7 text-xs font-mono'
                  />
                </div>
                <div className='space-y-1'>
                  <Label className='text-[11px]'>
                    {t('installer.hdfsNamenodeRPCAddress2')}
                  </Label>
                  <Input
                    value={store.hdfs_namenode_rpc_address_2 || ''}
                    onChange={(e) =>
                      patch({hdfs_namenode_rpc_address_2: e.target.value})
                    }
                    placeholder='nn2-host:8020'
                    className='h-7 text-xs font-mono'
                  />
                </div>
              </div>
            </div>
          )}

          <div className='space-y-1'>
            <Label className='text-[11px]'>{t('installer.hdfsSitePath')}</Label>
            <Input
              value={store.hdfs_site_path || ''}
              onChange={(e) => patch({hdfs_site_path: e.target.value})}
              placeholder='/etc/hadoop/conf/hdfs-site.xml'
              className='h-7 text-xs font-mono'
            />
          </div>

          <label className='flex items-center gap-2 text-xs'>
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
              className='h-3.5 w-3.5'
            />
            {t('installer.hdfsKerberos')}
          </label>

          {kerberosEnabled && (
            <div className='grid grid-cols-2 gap-2.5 rounded-md border bg-muted/20 p-2.5'>
              <div className='space-y-1'>
                <Label className='text-[11px]'>
                  {t('installer.kerberosPrincipal')}
                </Label>
                <Input
                  value={store.kerberos_principal || ''}
                  onChange={(e) => patch({kerberos_principal: e.target.value})}
                  placeholder='hdfs/namenode@EXAMPLE.COM'
                  className='h-7 text-xs font-mono'
                />
              </div>
              <div className='space-y-1'>
                <Label className='text-[11px]'>
                  {t('installer.kerberosKeytabPath')}
                </Label>
                <Input
                  value={store.kerberos_keytab_file_path || ''}
                  onChange={(e) =>
                    patch({kerberos_keytab_file_path: e.target.value})
                  }
                  placeholder='/etc/security/keytabs/hdfs.keytab'
                  className='h-7 text-xs font-mono'
                />
              </div>
            </div>
          )}
        </div>
      );
    }

    if (storageType === 'OSS') {
      return (
        <div className='grid grid-cols-2 gap-2.5'>
          <div className='space-y-1'>
            <Label className='text-[11px]'>{t('installer.endpoint')}</Label>
            <Input
              value={store.storage_endpoint || ''}
              onChange={(e) => patch({storage_endpoint: e.target.value})}
              placeholder='oss-cn-hangzhou.aliyuncs.com'
              className='h-7 text-xs font-mono'
            />
          </div>
          <div className='space-y-1'>
            <Label className='text-[11px]'>{t('installer.bucket')}</Label>
            <Input
              value={store.storage_bucket || ''}
              onChange={(e) => patch({storage_bucket: e.target.value})}
              placeholder='your-bucket'
              className='h-7 text-xs font-mono'
            />
          </div>
          <div className='space-y-1'>
            <Label className='text-[11px]'>fs.oss.accessKeyId</Label>
            <Input
              type='password'
              value={store.storage_access_key || ''}
              onChange={(e) => patch({storage_access_key: e.target.value})}
              className='h-7 text-xs font-mono'
            />
          </div>
          <div className='space-y-1'>
            <Label className='text-[11px]'>fs.oss.accessKeySecret</Label>
            <Input
              type='password'
              value={store.storage_secret_key || ''}
              onChange={(e) => patch({storage_secret_key: e.target.value})}
              className='h-7 text-xs font-mono'
            />
          </div>
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
          <div className='grid grid-cols-2 gap-2.5'>
            <div className='space-y-1'>
              <Label className='text-[11px]'>{t('installer.s3CredentialsProvider')}</Label>
              <Select
                value={provider}
                onValueChange={(value) => patch({s3_credentials_provider: value})}
              >
                <SelectTrigger className='h-7 text-xs'>
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
            <div className='space-y-1'>
              <Label className='text-[11px]'>{t('installer.bucket')}</Label>
              <Input
                value={store.storage_bucket || ''}
                onChange={(e) => patch({storage_bucket: e.target.value})}
                placeholder='s3a://bucket'
                className='h-7 text-xs font-mono'
              />
            </div>
          </div>
          <div className='space-y-1'>
            <Label className='text-[11px]'>{t('installer.endpoint')}</Label>
            <Input
              value={store.storage_endpoint || ''}
              onChange={(e) => patch({storage_endpoint: e.target.value})}
              placeholder='http://127.0.0.1:9000'
              className='h-7 text-xs font-mono'
            />
          </div>
          {!instanceProfile && (
            <div className='grid grid-cols-2 gap-2.5'>
              <div className='space-y-1'>
                <Label className='text-[11px]'>fs.s3a.access.key</Label>
                <Input
                  type='password'
                  value={store.storage_access_key || ''}
                  onChange={(e) => patch({storage_access_key: e.target.value})}
                  className='h-7 text-xs font-mono'
                />
              </div>
              <div className='space-y-1'>
                <Label className='text-[11px]'>fs.s3a.secret.key</Label>
                <Input
                  type='password'
                  value={store.storage_secret_key || ''}
                  onChange={(e) => patch({storage_secret_key: e.target.value})}
                  className='h-7 text-xs font-mono'
                />
              </div>
            </div>
          )}
        </div>
      );
    }

    return null;
  };

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      <ScrollArea className='flex-1 min-h-0 pr-4'>
        <div className='space-y-4 max-w-4xl mx-auto py-1'>
          {/* 1. Package Status & Mirror Strip / 安装包就绪与镜像源胶囊条 */}
          <div className='rounded-lg border p-3 flex flex-wrap items-center justify-between gap-3 bg-card/60'>
            <div className='flex items-center gap-2.5'>
              <div className='p-1.5 rounded-md bg-muted text-muted-foreground'>
                <Download className='h-4 w-4' />
              </div>
              <div>
                <div className='flex items-center gap-2'>
                  <span className='text-xs font-semibold'>
                    {t('cluster.wizard.packageDistribution')}
                  </span>
                  {isPackageLocal ? (
                    <Badge
                      variant='outline'
                      className='text-[10px] text-emerald-600 bg-emerald-500/10 border-emerald-500/20 py-0 h-4'
                    >
                      <CheckCircle2 className='h-2.5 w-2.5 mr-1' />
                      {t('cluster.wizard.localPackageReady')}
                    </Badge>
                  ) : (
                    <Badge
                      variant='outline'
                      className='text-[10px] text-amber-600 bg-amber-500/10 border-amber-500/20 py-0 h-4'
                    >
                      {t('cluster.wizard.downloadOnDeploy')}
                    </Badge>
                  )}
                </div>
                <p className='text-[11px] text-muted-foreground font-mono mt-0.5'>
                  {isPackageLocal
                    ? `${currentPackage.file_name} (${(currentPackage.file_size / 1024 / 1024).toFixed(1)} MB)`
                    : t('cluster.wizard.packagePullOnDeploy', {
                        version: config.version,
                      })}
                </p>
              </div>
            </div>

            <div className='flex items-center gap-2 shrink-0'>
              {!isPackageLocal && (
                <div className='flex items-center gap-1.5'>
                  <span className='text-xs text-muted-foreground'>
                    {t('cluster.wizard.mirrorSourceLabel')}:
                  </span>
                  <Select
                    value={config.mirror}
                    onValueChange={(value: MirrorSource) =>
                      updateConfig({mirror: value})
                    }
                  >
                    <SelectTrigger className='h-7 text-xs w-28'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='aliyun'>
                        {t('installer.aliyun')}
                      </SelectItem>
                      <SelectItem value='huaweicloud'>
                        {t('installer.huaweicloud')}
                      </SelectItem>
                      <SelectItem value='apache'>
                        {t('installer.apache')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
              <Button
                variant='ghost'
                size='sm'
                className='h-7 text-xs px-2 text-muted-foreground'
                onClick={() => window.open('/packages', '_blank')}
              >
                {t('cluster.wizard.packageManage')}
                <ExternalLink className='h-3 w-3 ml-1' />
              </Button>
            </div>
          </div>

          {/* 2. Ports & JVM Grid / 端口与 JVM 资源紧凑网格 */}
          <div className='grid grid-cols-1 md:grid-cols-2 gap-3'>
            {/* Ports Config Card / 端口配置卡片 */}
            <Card className='shadow-none border-border/70'>
              <CardHeader className='py-2.5 px-3.5 border-b bg-muted/15'>
                <CardTitle className='text-xs font-semibold flex items-center gap-1.5'>
                  <Sliders className='h-3.5 w-3.5 text-primary' />
                  {t('cluster.wizard.portMapping')}
                </CardTitle>
              </CardHeader>
              <CardContent className='p-3.5 space-y-2.5'>
                <div className='grid grid-cols-2 gap-2.5'>
                  <div className='space-y-1'>
                    <Label className='text-xs font-medium'>
                      {t('cluster.wizard.clusterCommPort')}
                    </Label>
                    <Input
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
                      className='h-8 text-xs font-mono'
                    />
                  </div>

                  {config.deploymentMode === DeploymentMode.SEPARATED && (
                    <div className='space-y-1'>
                      <Label className='text-xs font-medium'>
                        {t('cluster.wizard.workerPort')}
                      </Label>
                      <Input
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
                        className='h-8 text-xs font-mono'
                      />
                    </div>
                  )}
                </div>

                {/* HTTP Port / HTTP 端口 */}
                <div className='space-y-1 pt-1 border-t border-border/40'>
                  <div className='flex items-center justify-between'>
                    <Label className='text-xs font-medium'>
                      {t('cluster.wizard.masterWebUiPort')}
                    </Label>
                    <label className='flex items-center gap-1.5 text-[11px] text-muted-foreground cursor-pointer'>
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
                        className='h-3.5 w-3.5'
                      />
                      <span>{t('cluster.wizard.enableRestService')}</span>
                    </label>
                  </div>
                  <Input
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
                    className='h-8 text-xs font-mono'
                  />
                </div>

                {/* stx-java-proxy Port / stx-java-proxy 端口 */}
                <div className='space-y-1 pt-1 border-t border-border/40'>
                  <Label className='text-xs font-medium'>
                    {t('cluster.wizard.javaProxyPort')}
                  </Label>
                  <Input
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
                    className='h-8 text-xs font-mono'
                  />
                  <p className='text-[11px] text-muted-foreground'>
                    {t('cluster.wizard.javaProxyPortDesc')}
                  </p>
                </div>
              </CardContent>
            </Card>

            {/* JVM Heap Size Card / JVM 内存卡片 */}
            <Card className='shadow-none border-border/70'>
              <CardHeader className='py-2.5 px-3.5 border-b bg-muted/15'>
                <CardTitle className='text-xs font-semibold flex items-center gap-1.5'>
                  <HardDrive className='h-3.5 w-3.5 text-primary' />
                  {t('cluster.wizard.jvmHeapConfigGb')}
                </CardTitle>
              </CardHeader>
              <CardContent className='p-3.5 space-y-2.5'>
                {config.deploymentMode === DeploymentMode.HYBRID ? (
                  <div className='space-y-1'>
                    <Label className='text-xs font-medium'>
                      {t('cluster.wizard.hybridNodeHeap')}
                    </Label>
                    <div className='relative'>
                      <Input
                        type='number'
                        value={config.jvm.hybrid_heap_size}
                        onChange={(e) =>
                          updateConfig({
                            jvm: {
                              ...config.jvm,
                              hybrid_heap_size: parseInt(e.target.value) || 2,
                            },
                          })
                        }
                        min={1}
                        max={128}
                        className='h-8 text-xs font-mono pr-8'
                      />
                      <span className='absolute right-2.5 top-2 text-[11px] text-muted-foreground'>
                        GB
                      </span>
                    </div>
                  </div>
                ) : (
                  <div className='grid grid-cols-2 gap-2.5'>
                    <div className='space-y-1'>
                      <Label className='text-xs font-medium'>
                        {t('cluster.wizard.masterHeap')}
                      </Label>
                      <div className='relative'>
                        <Input
                          type='number'
                          value={config.jvm.master_heap_size}
                          onChange={(e) =>
                            updateConfig({
                              jvm: {
                                ...config.jvm,
                                master_heap_size: parseInt(e.target.value) || 2,
                              },
                            })
                          }
                          min={1}
                          max={128}
                          className='h-8 text-xs font-mono pr-8'
                        />
                        <span className='absolute right-2.5 top-2 text-[11px] text-muted-foreground'>
                          GB
                        </span>
                      </div>
                    </div>
                    <div className='space-y-1'>
                      <Label className='text-xs font-medium'>
                        {t('cluster.wizard.workerHeap')}
                      </Label>
                      <div className='relative'>
                        <Input
                          type='number'
                          value={config.jvm.worker_heap_size}
                          onChange={(e) =>
                            updateConfig({
                              jvm: {
                                ...config.jvm,
                                worker_heap_size: parseInt(e.target.value) || 2,
                              },
                            })
                          }
                          min={1}
                          max={128}
                          className='h-8 text-xs font-mono pr-8'
                        />
                        <span className='absolute right-2.5 top-2 text-[11px] text-muted-foreground'>
                          GB
                        </span>
                      </div>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

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
          <Card className='shadow-none border-border/70'>
            <CardHeader className='py-2.5 px-3.5 border-b bg-muted/15'>
              <CardTitle className='text-xs font-semibold flex items-center gap-1.5'>
                <Database className='h-3.5 w-3.5 text-primary' />
                {t('installer.checkpointConfig')}
              </CardTitle>
            </CardHeader>
            <CardContent className='p-3.5 space-y-3'>
              {checkpointNeedsSharedWarning && (
                <div className='rounded-md border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs text-amber-700 dark:text-amber-300 flex items-center gap-2'>
                  <AlertTriangle className='h-4 w-4 shrink-0 text-amber-500' />
                  <span>
                    {t('installer.runtimeStorage.checkpointLocalWarning')}
                  </span>
                </div>
              )}

              <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
                <div className='space-y-1'>
                  <Label className='text-xs font-medium'>
                    {t('installer.storageType')}
                  </Label>
                  <Select
                    value={config.checkpoint.storage_type}
                    onValueChange={(value: CheckpointStorageType) =>
                      updateConfig({
                        checkpoint: {
                          ...config.checkpoint,
                          storage_type: value,
                        },
                      })
                    }
                  >
                    <SelectTrigger className='h-8 text-xs'>
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

                <div className='space-y-1'>
                  <Label className='text-xs font-medium'>
                    {t('installer.namespace')}
                  </Label>
                  <Input
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
                    className='h-8 text-xs font-mono'
                  />
                </div>
              </div>

              {config.checkpoint.storage_type !== 'LOCAL_FILE' && (
                <div className='space-y-2 pt-1 border-t border-border/40'>
                  <p className='text-[11px] text-muted-foreground leading-relaxed'>
                    {t('installer.runtimeStorage.remoteStorageInstallHint')}
                  </p>
                  {renderRemoteStorageFields(
                    'checkpoint',
                    config.checkpoint.storage_type,
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          {/* 5. IMAP 状态存储配置 / IMAP State Storage Configuration */}
          <Card className='shadow-none border-border/70'>
            <CardHeader className='py-2.5 px-3.5 border-b bg-muted/15 flex flex-row items-center justify-between gap-3'>
              <div className='flex items-center gap-2'>
                <CardTitle className='text-xs font-semibold flex items-center gap-1.5'>
                  <Database className='h-3.5 w-3.5 text-primary' />
                  {t('installer.runtimeStorage.imapTitle')}
                </CardTitle>
                <span className='text-[11px] text-muted-foreground hidden sm:inline'>
                  ({t('installer.runtimeStorage.imapSubtitle', {defaultMessage: '作业恢复元数据'})})
                </span>
              </div>
              <div className='flex items-center gap-2.5'>
                <Badge
                  variant='outline'
                  className={`text-[10px] py-0 h-4 font-normal ${
                    imapExternalEnabled
                      ? 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20'
                      : 'text-muted-foreground bg-muted/40 border-border/50'
                  }`}
                >
                  {imapExternalEnabled
                    ? t('installer.runtimeStorage.imapExternalOn')
                    : t('installer.runtimeStorage.imapInMemory')}
                </Badge>
                <div className='flex items-center gap-1.5'>
                  <Label
                    htmlFor='imap-external-toggle'
                    className='text-xs font-normal text-muted-foreground cursor-pointer select-none'
                  >
                    {t('installer.runtimeStorage.imapEnableLabel')}
                  </Label>
                  <Switch
                    id='imap-external-toggle'
                    checked={imapExternalEnabled}
                    onCheckedChange={handleImapExternalToggle}
                  />
                </div>
              </div>
            </CardHeader>

            <CardContent className='p-3.5 space-y-3'>
              {/* 未开启外部存储：高信噪比纯内存说明卡 / When disabled: high-signal in-memory notice */}
              {!imapExternalEnabled ? (
                <div className='rounded-lg border border-border/60 bg-muted/15 p-3 space-y-1.5'>
                  <div className='flex items-center gap-2 text-xs font-medium text-foreground/90'>
                    <Info className='h-4 w-4 text-sky-500 shrink-0' />
                    <span>{t('installer.runtimeStorage.imapInMemoryTitle')}</span>
                  </div>
                  <p className='text-xs text-muted-foreground leading-relaxed pl-6'>
                    {t('installer.runtimeStorage.imapInMemoryNotice')}
                  </p>
                </div>
              ) : (
                /* 已开启外部存储：展开存储类型与配置 / When enabled: reveal storage options */
                <>
                  <div className='rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-2.5 flex items-center justify-between gap-3'>
                    <div className='flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-400'>
                      <CheckCircle2 className='h-3.5 w-3.5 shrink-0' />
                      <span className='leading-tight'>
                        {t('installer.runtimeStorage.imapExternalNotice')}
                      </span>
                    </div>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={applyCheckpointToImap}
                      className='h-6 text-[11px] px-2 shrink-0 border-emerald-500/30 hover:bg-emerald-500/10'
                    >
                      <Copy className='h-3 w-3 mr-1' />
                      {t('installer.runtimeStorage.applyToImap')}
                    </Button>
                  </div>

                  {selectedHosts.length > 1 &&
                    config.imap.storage_type === 'LOCAL_FILE' && (
                      <div className='rounded-md border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs text-amber-700 dark:text-amber-300 flex items-center gap-2'>
                        <AlertTriangle className='h-4 w-4 shrink-0 text-amber-500' />
                        <span>
                          {t('installer.runtimeStorage.checkpointLocalWarning')}
                        </span>
                      </div>
                    )}

                  <div className='grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1'>
                    <div className='space-y-1'>
                      <Label className='text-xs font-medium'>
                        {t('installer.storageType')}
                      </Label>
                      <Select
                        value={config.imap.storage_type}
                        onValueChange={(value: IMAPStorageType) => {
                          if (value === 'DISABLED') {
                            handleImapExternalToggle(false);
                            return;
                          }
                          updateConfig({
                            imap: {
                              ...config.imap,
                              storage_type: value,
                            },
                          });
                        }}
                      >
                        <SelectTrigger className='h-8 text-xs'>
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

                    <div className='space-y-1'>
                      <Label className='text-xs font-medium'>
                        {t('installer.namespace')}
                      </Label>
                      <Input
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
                        className='h-8 text-xs font-mono'
                      />
                    </div>
                  </div>

                  {config.imap.storage_type !== 'LOCAL_FILE' &&
                    config.imap.storage_type !== 'DISABLED' && (
                      <div className='space-y-2 pt-2 border-t border-border/40'>
                        <p className='text-[11px] text-muted-foreground leading-relaxed'>
                          {t(
                            'installer.runtimeStorage.remoteStorageInstallHint',
                          )}
                        </p>
                        {renderRemoteStorageFields(
                          'imap',
                          config.imap.storage_type,
                        )}
                      </div>
                    )}
                </>
              )}
            </CardContent>
          </Card>
        </div>
      </ScrollArea>
    </div>
  );
}

export default ClusterConfigStep;
