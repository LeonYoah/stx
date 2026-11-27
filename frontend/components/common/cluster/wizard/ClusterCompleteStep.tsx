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
 * Cluster Complete Step Component
 * 集群创建完成步骤组件
 *
 * Polished summary card with topology breakdown and immediate navigation CTA.
 * 精致的集群拓扑概要卡片与直达集群控制台的操作入口。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Alert} from '@/components/arc/alert/alert';
import {NeutralChip} from '@/components/common/layout';
import {
  CheckCircle2,
  Layers,
  ArrowRight,
} from 'lucide-react';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';
import {ClusterDeployConfig, HostWithRole, K8sWizardFormState} from './types';

interface ClusterCompleteStepProps {
  /** Cluster deploy config / 集群部署配置 */
  config: ClusterDeployConfig;
  /** Selected hosts / 已选主机 */
  selectedHosts: HostWithRole[];
  /** Warning messages / 警告信息 */
  deployWarnings: string[] | string | null;
  /** K8s 向导表单：存在时按 K8s 语义渲染摘要（命名空间/形态/副本），不走物理主机计数。
      K8s wizard form state: when present the summary renders Kubernetes semantics
      (namespace/topology/replicas) instead of physical host counts. */
  k8sForm?: K8sWizardFormState;
  /** Close dialog callback / 关闭向导回调 */
  onClose: () => void;
  /** View cluster callback / 查看集群回调 */
  onComplete: () => void;
}

export function ClusterCompleteStep({
  config,
  selectedHosts,
  deployWarnings,
  k8sForm,
  onClose,
  onComplete,
}: ClusterCompleteStepProps) {
  const t = useTranslations();
  const warningsList = Array.isArray(deployWarnings)
    ? deployWarnings
    : typeof deployWarnings === 'string' && deployWarnings.trim()
      ? [deployWarnings]
      : [];

  const masterCount =
    config.deploymentMode === DeploymentMode.HYBRID
      ? selectedHosts.length
      : selectedHosts.filter((h) => h.roles.includes(NodeRole.MASTER)).length;

  const workerCount =
    config.deploymentMode === DeploymentMode.HYBRID
      ? selectedHosts.length
      : selectedHosts.filter((h) => h.roles.includes(NodeRole.WORKER)).length;

  return (
    <div className='h-full overflow-y-auto'>
      <div className='flex flex-col items-center max-w-xl mx-auto py-4 text-center'>
        {/* Success Hero / 成功标识 */}
        <div className='w-12 h-12 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-600 flex items-center justify-center mb-3'>
          <CheckCircle2 className='w-6 h-6 stroke-[2.5]' />
        </div>

        <h2 className='text-lg font-bold text-foreground'>
          {t('cluster.wizard.completeTitle')}
        </h2>

        {/* Deploy Warnings / 部署告警（完成页同样展示） */}
        {warningsList.length > 0 && (
          <div className='w-full mt-4 text-left'>
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

        {/* Cluster Summary / 集群规格摘要（单层画布 + 0.5px 细线） */}
        <div className='w-full mt-5 text-left'>
          <div className='flex items-center justify-between pb-2.5 border-b-[0.5px] border-border/80'>
            <div className='flex items-center gap-2 min-w-0'>
              <Layers className='h-4 w-4 shrink-0 text-primary' />
              <span className='font-semibold text-sm truncate'>{config.name}</span>
            </div>
            <NeutralChip className='font-mono tabular-nums'>
              v{config.version}
            </NeutralChip>
          </div>

          <div className='grid grid-cols-2 gap-3 pt-3 text-xs'>
            <div>
              <span className='text-muted-foreground block text-[11px]'>
                {t('cluster.wizard.summaryMode')}
              </span>
              <span className='font-medium text-foreground mt-0.5 block'>
                {config.deploymentMode === DeploymentMode.HYBRID
                  ? t('installer.hybrid')
                  : t('installer.separated')}
              </span>
            </div>

            <div>
              <span className='text-muted-foreground block text-[11px]'>
                {t('cluster.wizard.summaryScale')}
              </span>
              <span className='font-medium text-foreground mt-0.5 block tabular-nums'>
                {k8sForm
                  ? t('cluster.wizard.summaryK8sScale', {
                      namespace: k8sForm.namespace,
                      mode: k8sForm.deployment_mode,
                      replicas:
                        k8sForm.deployment_mode === 'hybrid'
                          ? k8sForm.hybrid_replicas
                          : `${k8sForm.master_replicas}+${k8sForm.worker_replicas}`,
                    })
                  : t('cluster.wizard.summaryHosts', {
                      count: selectedHosts.length,
                      master: masterCount,
                      worker: workerCount,
                    })}
              </span>
            </div>

            <div>
              <span className='text-muted-foreground block text-[11px]'>
                {t('cluster.wizard.summaryPorts')}
              </span>
              <span className='font-mono text-foreground mt-0.5 block tabular-nums'>
                Hazelcast: {config.clusterPort}
                {config.deploymentMode === DeploymentMode.SEPARATED &&
                  ` | Worker: ${config.workerPort}`}
                {` | Proxy: ${config.javaProxyPort}`}
              </span>
            </div>

            <div>
              <span className='text-muted-foreground block text-[11px]'>
                {t('cluster.wizard.summaryHttp')}
              </span>
              <span className='font-mono text-foreground mt-0.5 block tabular-nums'>
                {config.runtime.enable_http
                  ? t('cluster.wizard.httpEnabled', {port: config.httpPort})
                  : t('cluster.wizard.httpDisabled')}
              </span>
            </div>

            {isSeatunnelVersionAtLeast(config.version, '3.0.0') && (
              <div>
                <span className='text-muted-foreground block text-[11px]'>
                  {t('cluster.wizard.summaryEventChannel')}
                </span>
                <span className='font-medium text-foreground mt-0.5 block'>
                  {(k8sForm ? k8sForm.event_report_enabled : config.runtime.enable_event_channel)
                    ? t('cluster.wizard.eventChannelEnabled')
                    : t('cluster.wizard.eventChannelDisabled')}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Action Buttons / 底部快捷按钮 */}
        <div className='flex items-center gap-3 mt-6'>
          <Button type='button' variant='secondary' size='sm' onClick={onClose}>
            {t('cluster.wizard.finishAndClose')}
          </Button>
          <Button type='button' size='sm' onClick={onComplete}>
            {t('cluster.wizard.enterConsole')}
            <ArrowRight className='size-3.5' aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  );
}

export default ClusterCompleteStep;
