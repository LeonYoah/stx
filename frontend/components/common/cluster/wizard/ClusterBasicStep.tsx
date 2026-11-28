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
 * Cluster Basic Info Step Component
 * 集群基本信息配置步骤
 *
 * Compact form for cluster name, description, deployment mode, and install path.
 * 紧凑清晰的集群名称、描述、部署模式及安装路径表单。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {Download, Server, Layers} from 'lucide-react';
import {HelpHint} from '@/components/common/HelpHint';
import {DeploymentMode} from '@/lib/services/cluster/types';
import {ClusterDeployConfig} from './types';
import {
  buildSeatunnelInstallDir,
  resolveSelectableSeatunnelVersions,
} from '@/lib/seatunnel-version';
import {isValidClusterCode, recommendClusterCode} from '@/lib/cluster-code';
import type {PackageInfo, AvailableVersions} from '@/lib/services/installer/types';

interface ClusterBasicStepProps {
  /** Cluster deploy configuration / 集群配置状态 */
  config: ClusterDeployConfig;
  /** Update configuration callback / 更新配置回调 */
  updateConfig: (updates: Partial<ClusterDeployConfig>) => void;
  /** Packages data / 安装包数据 */
  packages?: AvailableVersions | null;
  /** Local packages list / 本地缓存包列表 */
  localPackages: PackageInfo[];
  /** Packages loading status / 安装包加载状态 */
  packagesLoading: boolean;
  /** 默认预选版本号 / Default preselected version */
  defaultVersion: string;
}

export function ClusterBasicStep({
  config,
  updateConfig,
  packages,
  localPackages,
  packagesLoading,
  defaultVersion,
}: ClusterBasicStepProps) {
  const t = useTranslations();
  const [codeTouched, setCodeTouched] = React.useState(Boolean(config.clusterCode));

  const clusterCodeInvalid =
    config.clusterCode.length > 0 && !isValidClusterCode(config.clusterCode);

  const localVersionSet = React.useMemo(
    () => new Set(localPackages.map((pkg) => pkg.version)),
    [localPackages],
  );
  const selectableVersions = React.useMemo(
    () =>
      resolveSelectableSeatunnelVersions(packages, [
        config.version,
        defaultVersion,
      ]),
    [packages, config.version, defaultVersion],
  );
  const selectedVersionIsLocal = localVersionSet.has(config.version);

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      <div className='flex-1 min-h-0 overflow-y-auto pr-4'>
        <div className='space-y-5 max-w-3xl mx-auto py-2'>
          {/* Cluster Name & Code / 集群名称与集群标识 */}
          <div className='grid grid-cols-1 items-start md:grid-cols-2 gap-4'>
            <Input
              id='cluster-name'
              label={t('cluster.name')}
              required
              value={config.name}
              onChange={(e) => {
                const newName = e.target.value;
                const updates: Partial<ClusterDeployConfig> = {name: newName};
                if (!codeTouched) {
                  updates.clusterCode = recommendClusterCode(newName);
                }
                updateConfig(updates);
              }}
              placeholder={t('cluster.namePlaceholder')}
              autoFocus
            />

            {/* 详细说明 hint 钉在 label 行右侧 / Long explanation hint pinned to the label row */}
            <div className='relative'>
              <Input
                id='cluster-code'
                label={`${t('cluster.clusterCode')} · ${t('common.optional')}`}
                value={config.clusterCode}
                onChange={(e) => {
                  setCodeTouched(true);
                  updateConfig({clusterCode: e.target.value.toLowerCase().trim()});
                }}
                placeholder={t('cluster.clusterCodePlaceholder')}
                className='font-mono text-xs'
                error={
                  clusterCodeInvalid
                    ? t('cluster.clusterCodeInvalid')
                    : undefined
                }
                description={
                  clusterCodeInvalid
                    ? undefined
                    : t('cluster.clusterCodeShortHint')
                }
              />
              <div className='absolute right-0 top-0'>
                <HelpHint
                  label={t('cluster.clusterCode')}
                  content={t('cluster.clusterCodeHint')}
                  data-testid='cluster-wizard-cluster-code-hint'
                />
              </div>
            </div>
          </div>

          {/* Description / 描述 */}
          <Input
            id='cluster-desc'
            label={t('cluster.descriptionLabel')}
            value={config.description}
            onChange={(e) => updateConfig({description: e.target.value})}
            placeholder={t('cluster.descriptionPlaceholder')}
          />

          {/* Deployment Mode Selection / 部署模式选择 */}
          <div className='space-y-2'>
            <span className='block text-xs font-medium text-foreground'>
              {t('cluster.deploymentMode')}
            </span>
            <RadioCards
              layout='grid'
              value={config.deploymentMode}
              onValueChange={(value) =>
                updateConfig({deploymentMode: value as DeploymentMode})
              }
              options={[
                {
                  value: DeploymentMode.HYBRID,
                  label: t('cluster.modes.hybrid'),
                  description: t('cluster.hybridDescription'),
                  icon: <Layers className='h-4 w-4' aria-hidden />,
                  meta: (
                    <NeutralChip>
                      {t('cluster.wizard.modeBadgeLightweight')}
                    </NeutralChip>
                  ),
                },
                {
                  value: DeploymentMode.SEPARATED,
                  label: t('cluster.modes.separated'),
                  description: t('cluster.separatedDescription'),
                  icon: <Server className='h-4 w-4' aria-hidden />,
                  meta: (
                    <SoftChip tone='blue'>
                      {t('cluster.wizard.modeBadgeProduction')}
                    </SoftChip>
                  ),
                },
              ]}
            />
          </div>

          {/* Version & Install Directory / 版本与安装目录 */}
          <div className='grid grid-cols-1 items-start md:grid-cols-2 gap-4 pt-4 border-t-[0.5px] border-border/80'>
            <div className='space-y-1.5'>
              <Select
                data-testid='cluster-wizard-version-select'
                label={t('installer.version')}
                value={config.version}
                displayValue={config.version || undefined}
                onValueChange={(value) => updateConfig({version: value})}
                disabled={packagesLoading}
                options={selectableVersions.map((version: string) => ({
                  value: version,
                  label: localVersionSet.has(version)
                    ? `${version} · ${t('cluster.wizard.localReady')}`
                    : version,
                }))}
              />
              {/* 本地已就绪指示 / Status chip when the chosen version is cached locally */}
              {selectedVersionIsLocal && (
                <div className='flex flex-wrap items-center gap-1.5'>
                  <SoftChip tone='emerald'>
                    <Download className='h-3 w-3' aria-hidden />
                    {t('cluster.wizard.localReady')}
                  </SoftChip>
                </div>
              )}
            </div>

            <Input
              id='installDir'
              label={t('installer.installDirLabel')}
              value={config.installDir}
              onChange={(e) => updateConfig({installDir: e.target.value})}
              placeholder={buildSeatunnelInstallDir(config.version || defaultVersion)}
              className='font-mono text-xs'
            />
          </div>
        </div>
      </div>
    </div>
  );
}

export default ClusterBasicStep;
