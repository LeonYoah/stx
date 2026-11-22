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
import {Button} from '@/components/arc/button/button';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Alert} from '@/components/arc/alert/alert';
import {NeutralChip, TruncatedText} from '@/components/common/layout';
import {
  Loader2,
  CheckCircle2,
  XCircle,
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
      {/* Top Status & Progress / 顶部状态与进度（单层画布，0.5px 细线分隔） */}
      <div className='space-y-3 shrink-0 pb-3 border-b-[0.5px] border-border/80'>
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-3'>
            {deployStatus === 'running' && (
              <Loader2 className='h-5 w-5 animate-spin text-primary' />
            )}
            {deployStatus === 'success' && (
              <CheckCircle2 className='h-5 w-5 text-emerald-600' />
            )}
            {deployStatus === 'failed' && (
              <XCircle className='h-5 w-5 text-destructive' />
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
            <span className='font-mono text-lg font-bold tabular-nums'>
              {deployProgress}%
            </span>
          </div>
        </div>

        {/* 轻量进度条（Arc 无对应原语，保持简单 div 实现）/ Lightweight bar — Arc has no progress primitive */}
        <div
          role='progressbar'
          aria-valuenow={deployProgress}
          aria-valuemin={0}
          aria-valuemax={100}
          className='h-1.5 w-full overflow-hidden rounded-full bg-muted'
        >
          <div
            className={cn(
              'h-full rounded-full transition-[width] duration-300',
              deployStatus === 'failed' ? 'bg-destructive' : 'bg-primary',
            )}
            style={{width: `${Math.min(100, Math.max(0, deployProgress))}%`}}
          />
        </div>

        {/* Failure Actions / 失败时的快捷操作 */}
        {deployStatus === 'failed' && (
          <div className='flex items-center gap-2 pt-1'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={onBackToPlugins}
              disabled={deploying}
            >
              <ChevronLeft className='size-3.5' aria-hidden />
              {t('cluster.wizard.backToConfig')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={onRetry}
              disabled={deploying}
            >
              <RotateCw className='size-3.5' aria-hidden />
              {t('cluster.wizard.retryDeploy')}
            </Button>
          </div>
        )}
      </div>

      {/* Warnings / 部署警告（正常换行，不用 break-all） */}
      {warningsList.length > 0 && (
        <div className='pt-3 shrink-0'>
          <Alert tone='warning' title={t('cluster.wizard.deployWarnings')}>
            <div className='space-y-1'>
              {warningsList.map((warning, idx) => (
                <p key={idx} className='font-mono text-[11px] break-words'>
                  {warning}
                </p>
              ))}
            </div>
          </Alert>
        </div>
      )}

      {/* Step Progress Log Stream / 部署步骤任务流（原生滚动，不用 ScrollArea） */}
      <div className='flex-1 min-h-0 flex flex-col mt-3'>
        <div className='py-2 border-b-[0.5px] border-border/80 flex items-center justify-between'>
          <span className='text-xs font-semibold text-muted-foreground flex items-center gap-1.5'>
            <Activity className='h-3.5 w-3.5' />
            {t('cluster.wizard.deployDetailLog')}
          </span>
          <span className='text-[11px] font-mono text-muted-foreground tabular-nums'>
            {t('cluster.wizard.deployStepsDone', {
              done: deploySteps.filter((s) => s.status === 'success').length,
              total: deploySteps.length,
            })}
          </span>
        </div>

        <div className='flex-1 min-h-0 overflow-y-auto py-3'>
          <div className='space-y-1.5'>
            {deploySteps.length === 0 ? (
              <Skeleton
                lines={4}
                label={t('cluster.wizard.deployPreparing')}
              />
            ) : (
              deploySteps.map((item, idx) => (
                <div
                  key={`${item.step}-${item.hostName || idx}`}
                  className={cn(
                    'flex items-center gap-2.5 px-3 py-2 rounded-md text-xs border-[0.5px] transition-colors',
                    item.status === 'running' &&
                      'bg-primary/5 border-primary/30 text-primary',
                    item.status === 'success' &&
                      'border-border/80 text-foreground',
                    item.status === 'failed' &&
                      'bg-destructive/10 border-destructive/40 text-destructive',
                    item.status === 'pending' &&
                      'border-transparent text-muted-foreground/60',
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
                    <NeutralChip className='font-mono shrink-0'>
                      <Server className='h-3 w-3 opacity-70' aria-hidden />
                      {item.hostName}
                    </NeutralChip>
                  )}

                  <TruncatedText
                    text={item.message}
                    mono
                    className='flex-1 min-w-0'
                  />
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default ClusterDeployStep;
