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
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Badge} from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Download, Server, Layers, Sparkles} from 'lucide-react';
import {cn} from '@/lib/utils';
import {DeploymentMode} from '@/lib/services/cluster/types';
import {ClusterDeployConfig} from './types';
import {buildSeatunnelInstallDir} from '@/lib/seatunnel-version';
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
  /** Recommended version / 推荐版本号 */
  resolvedRecommendedVersion: string;
}

export function ClusterBasicStep({
  config,
  updateConfig,
  packages,
  localPackages,
  packagesLoading,
  resolvedRecommendedVersion,
}: ClusterBasicStepProps) {
  const t = useTranslations();

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      <ScrollArea className='flex-1 min-h-0 pr-4'>
        <div className='space-y-5 max-w-3xl mx-auto py-2'>
          {/* Cluster Name & Description / 集群名称与描述 */}
          <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
            <div className='space-y-1.5'>
              <Label htmlFor='cluster-name' className='text-sm font-medium'>
                {t('cluster.name')} <span className='text-destructive'>*</span>
              </Label>
              <Input
                id='cluster-name'
                value={config.name}
                onChange={(e) => updateConfig({name: e.target.value})}
                placeholder={t('cluster.namePlaceholder')}
                className='h-9'
                autoFocus
              />
            </div>

            <div className='space-y-1.5'>
              <Label htmlFor='cluster-desc' className='text-sm font-medium'>
                {t('cluster.descriptionLabel')}
              </Label>
              <Input
                id='cluster-desc'
                value={config.description}
                onChange={(e) => updateConfig({description: e.target.value})}
                placeholder={t('cluster.descriptionPlaceholder')}
                className='h-9'
              />
            </div>
          </div>

          {/* Deployment Mode Selection / 部署模式选择 */}
          <div className='space-y-2'>
            <Label className='text-sm font-medium'>{t('cluster.deploymentMode')}</Label>
            <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
              {/* Hybrid Mode / 混合模式 */}
              <div
                onClick={() => updateConfig({deploymentMode: DeploymentMode.HYBRID})}
                className={cn(
                  'cursor-pointer rounded-lg border p-3.5 transition-all flex flex-col justify-between gap-2',
                  config.deploymentMode === DeploymentMode.HYBRID
                    ? 'border-primary bg-primary/5 ring-1 ring-primary/20 shadow-xs'
                    : 'border-border/70 hover:border-border hover:bg-muted/30',
                )}
              >
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <div className='p-1.5 rounded-md bg-muted text-foreground'>
                      <Layers className='h-4 w-4' />
                    </div>
                    <span className='text-sm font-semibold'>{t('cluster.modes.hybrid')}</span>
                  </div>
                  <Badge variant='outline' className='text-[10px] px-1.5 py-0'>
                    {t('cluster.wizard.modeBadgeLightweight')}
                  </Badge>
                </div>
                <p className='text-xs text-muted-foreground line-clamp-1'>
                  {t('cluster.hybridDescription')}
                </p>
              </div>

              {/* Separated Mode / 分离模式 */}
              <div
                onClick={() => updateConfig({deploymentMode: DeploymentMode.SEPARATED})}
                className={cn(
                  'cursor-pointer rounded-lg border p-3.5 transition-all flex flex-col justify-between gap-2',
                  config.deploymentMode === DeploymentMode.SEPARATED
                    ? 'border-primary bg-primary/5 ring-1 ring-primary/20 shadow-xs'
                    : 'border-border/70 hover:border-border hover:bg-muted/30',
                )}
              >
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <div className='p-1.5 rounded-md bg-muted text-foreground'>
                      <Server className='h-4 w-4' />
                    </div>
                    <span className='text-sm font-semibold'>{t('cluster.modes.separated')}</span>
                  </div>
                  <Badge variant='secondary' className='text-[10px] px-1.5 py-0 bg-primary/10 text-primary border-0'>
                    <Sparkles className='h-2.5 w-2.5 mr-0.5 inline' />
                    {t('cluster.wizard.modeBadgeProduction')}
                  </Badge>
                </div>
                <p className='text-xs text-muted-foreground line-clamp-1'>
                  {t('cluster.separatedDescription')}
                </p>
              </div>
            </div>
          </div>

          {/* Version & Install Directory / 版本与安装目录 */}
          <div className='grid grid-cols-1 md:grid-cols-2 gap-4 pt-1 border-t border-border/40'>
            <div className='space-y-1.5'>
              <Label className='text-sm font-medium'>{t('installer.version')}</Label>
              <Select
                value={config.version}
                onValueChange={(value) => updateConfig({version: value})}
                disabled={packagesLoading}
              >
                <SelectTrigger className='h-9'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(
                    packages?.versions ||
                    (resolvedRecommendedVersion ? [resolvedRecommendedVersion] : [])
                  ).map((version: string) => {
                    const isLocal = localPackages.some((pkg) => pkg.version === version);
                    return (
                      <SelectItem key={version} value={version}>
                        <div className='flex items-center gap-2'>
                          <span className='font-mono text-xs'>{version}</span>
                          {isLocal && (
                            <Badge variant='outline' className='text-[10px] text-emerald-600 bg-emerald-500/10 border-emerald-500/20 py-0 h-4'>
                              <Download className='h-2.5 w-2.5 mr-1' />
                              {t('cluster.wizard.localReady')}
                            </Badge>
                          )}
                          {version === packages?.recommended_version && (
                            <Badge variant='secondary' className='text-[10px] py-0 h-4'>
                              {t('installer.recommended')}
                            </Badge>
                          )}
                        </div>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
            </div>

            <div className='space-y-1.5'>
              <Label htmlFor='installDir' className='text-sm font-medium'>
                {t('installer.installDirLabel')}
              </Label>
              <Input
                id='installDir'
                value={config.installDir}
                onChange={(e) => updateConfig({installDir: e.target.value})}
                placeholder={buildSeatunnelInstallDir(config.version || resolvedRecommendedVersion)}
                className='h-9 font-mono text-xs'
              />
            </div>
          </div>
        </div>
      </ScrollArea>
    </div>
  );
}

export default ClusterBasicStep;
