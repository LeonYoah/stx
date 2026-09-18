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
 * Cluster Deployment Step Component
 * 集群执行部署步骤组件
 *
 * Sleek deployment dashboard showing live installation progress and node steps.
 * 现代风格的部署监控面板，实时显示集群节点安装推进进度与任务日志。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Progress} from '@/components/ui/progress';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Loader2,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  RotateCw,
  ChevronLeft,
  Server,
  Activity,
} from 'lucide-react';
import {cn} from '@/lib/utils';
import {DeployStepItem} from './types';

interface ClusterDeployStepProps {
  /** Deploy status / 部署状态 */
  deployStatus: 'idle' | 'running' | 'success' | 'failed';
  /** Deployment overall progress percentage / 部署总体进度百分比 */
  deployProgress: number;
  /** Deploy error message / 部署错误信息 */
  deployError: string | null;
  /** Deploy warning messages / 部署警告信息 */
  deployWarnings: string[] | string | null;
  /** Detailed deploy steps list / 详细步骤列表 */
  deploySteps: DeployStepItem[];
  /** Whether currently deploying / 是否正在部署 */
  deploying: boolean;
  /** Retry deployment callback / 重试部署回调 */
  onRetry: () => void;
  /** Go back to adjust configuration / 返回配置阶段回调 */
  onBackToPlugins: () => void;
}

export function ClusterDeployStep({
  deployStatus,
  deployProgress,
  deployError,
  deployWarnings,
  deploySteps,
  deploying,
  onRetry,
  onBackToPlugins,
}: ClusterDeployStepProps) {
  const t = useTranslations();
  const warningsList = Array.isArray(deployWarnings)
    ? deployWarnings
    : typeof deployWarnings === 'string' && deployWarnings.trim()
      ? [deployWarnings]
      : [];

  return (
    <div className='h-full flex flex-col overflow-hidden max-w-4xl mx-auto'>
      {/* Top Status & Progress Bar / 顶部状态与进度条 */}
      <div className='rounded-lg border p-4 bg-card/60 space-y-3 shrink-0'>
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-3'>
            {deployStatus === 'running' && (
              <div className='p-2 rounded-lg bg-primary/10 text-primary'>
                <Loader2 className='h-5 w-5 animate-spin' />
              </div>
            )}
            {deployStatus === 'success' && (
              <div className='p-2 rounded-lg bg-emerald-500/10 text-emerald-600'>
                <CheckCircle2 className='h-5 w-5' />
              </div>
            )}
            {deployStatus === 'failed' && (
              <div className='p-2 rounded-lg bg-destructive/10 text-destructive'>
                <XCircle className='h-5 w-5' />
              </div>
            )}
            <div>
              <h3 className='text-sm font-semibold'>
                {deployStatus === 'running' &&
                  t('cluster.wizard.deployingCluster')}
                {deployStatus === 'success' &&
                  t('cluster.wizard.deploySuccess')}
                {deployStatus === 'failed' && t('cluster.wizard.deployFailed')}
              </h3>
              <p className='text-xs text-muted-foreground mt-0.5'>
                {deployStatus === 'running' &&
                  t('cluster.wizard.deployingClusterDesc')}
                {deployStatus === 'success' &&
                  t('cluster.wizard.deploySuccessNodes')}
                {deployStatus === 'failed' &&
                  (deployError || t('cluster.wizard.deployFailedGeneric'))}
              </p>
            </div>
          </div>

          <div className='text-right'>
            <span className='font-mono text-lg font-bold'>{deployProgress}%</span>
          </div>
        </div>

        <Progress value={deployProgress} className='h-2 transition-all duration-300' />

        {/* Failure Actions / 失败时的快捷操作 */}
        {deployStatus === 'failed' && (
          <div className='flex items-center gap-2 pt-1'>
            <Button
              variant='outline'
              size='sm'
              onClick={onBackToPlugins}
              disabled={deploying}
              className='h-8 text-xs'
            >
              <ChevronLeft className='h-3.5 w-3.5 mr-1' />
              {t('cluster.wizard.backToConfig')}
            </Button>
            <Button
              size='sm'
              onClick={onRetry}
              disabled={deploying}
              className='h-8 text-xs'
            >
              <RotateCw className='h-3.5 w-3.5 mr-1' />
              {t('cluster.wizard.retryDeploy')}
            </Button>
          </div>
        )}
      </div>

      {/* Warnings Banner / 部署警告条 */}
      {warningsList.length > 0 && (
        <div className='mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-800 dark:text-amber-200 shrink-0'>
          <div className='flex items-start gap-2'>
            <AlertTriangle className='h-4 w-4 shrink-0 text-amber-600 mt-0.5' />
            <div className='space-y-1 min-w-0'>
              <span className='font-semibold'>
                {t('cluster.wizard.deployWarnings')}:
              </span>
              {warningsList.map((warning, idx) => (
                <p key={idx} className='text-[11px] font-mono break-all'>
                  {warning}
                </p>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Step Progress Log Stream / 部署步骤任务流 */}
      <div className='flex-1 min-h-0 flex flex-col mt-3 border rounded-lg overflow-hidden bg-muted/10'>
        <div className='px-3.5 py-2 border-b bg-muted/20 flex items-center justify-between'>
          <span className='text-xs font-semibold text-muted-foreground flex items-center gap-1.5'>
            <Activity className='h-3.5 w-3.5' />
            {t('cluster.wizard.deployDetailLog')}
          </span>
          <span className='text-[11px] font-mono text-muted-foreground'>
            {t('cluster.wizard.deployStepsDone', {
              done: deploySteps.filter((s) => s.status === 'success').length,
              total: deploySteps.length,
            })}
          </span>
        </div>

        <ScrollArea className='flex-1 min-h-0 p-3'>
          <div className='space-y-1.5'>
            {deploySteps.length === 0 ? (
              <div className='text-center py-8 text-xs text-muted-foreground'>
                <Loader2 className='h-4 w-4 animate-spin mx-auto mb-2 opacity-50' />
                {t('cluster.wizard.deployPreparing')}
              </div>
            ) : (
              deploySteps.map((item, idx) => (
                <div
                  key={`${item.step}-${item.hostName || idx}`}
                  className={cn(
                    'flex items-center gap-2.5 px-3 py-2 rounded-md text-xs border transition-colors',
                    item.status === 'running' &&
                      'bg-primary/5 border-primary/20 text-primary',
                    item.status === 'success' &&
                      'bg-background border-border/60 text-foreground',
                    item.status === 'failed' &&
                      'bg-destructive/10 border-destructive/30 text-destructive',
                    item.status === 'pending' &&
                      'bg-muted/30 border-transparent text-muted-foreground/60',
                  )}
                >
                  {item.status === 'running' && (
                    <Loader2 className='h-3.5 w-3.5 animate-spin shrink-0 text-primary' />
                  )}
                  {item.status === 'success' && (
                    <CheckCircle2 className='h-3.5 w-3.5 shrink-0 text-emerald-500' />
                  )}
                  {item.status === 'failed' && (
                    <XCircle className='h-3.5 w-3.5 shrink-0 text-destructive' />
                  )}
                  {item.status === 'pending' && (
                    <div className='h-3.5 w-3.5 rounded-full border border-muted-foreground/40 shrink-0' />
                  )}

                  {item.hostName && (
                    <Badge
                      variant='outline'
                      className='font-mono text-[10px] h-4 px-1.5 shrink-0 bg-background'
                    >
                      <Server className='h-2.5 w-2.5 mr-1 opacity-70' />
                      {item.hostName}
                    </Badge>
                  )}

                  <span className='font-mono truncate flex-1'>{item.message}</span>
                </div>
              ))
            )}
          </div>
        </ScrollArea>
      </div>
    </div>
  );
}

export default ClusterDeployStep;
