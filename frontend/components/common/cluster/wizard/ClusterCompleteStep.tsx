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
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent} from '@/components/ui/card';
import {
  CheckCircle2,
  Layers,
  ArrowRight,
} from 'lucide-react';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';
import {ClusterDeployConfig, HostWithRole} from './types';

interface ClusterCompleteStepProps {
  /** Cluster deploy config / 集群部署配置 */
  config: ClusterDeployConfig;
  /** Selected hosts / 已选主机 */
  selectedHosts: HostWithRole[];
  /** Warning messages / 警告信息 */
  deployWarnings: string[] | string | null;
  /** Close dialog callback / 关闭向导回调 */
  onClose: () => void;
  /** View cluster callback / 查看集群回调 */
  onComplete: () => void;
}

export function ClusterCompleteStep({
  config,
  selectedHosts,
  onClose,
  onComplete,
}: ClusterCompleteStepProps) {
  const t = useTranslations();

  const masterCount =
    config.deploymentMode === DeploymentMode.HYBRID
      ? selectedHosts.length
      : selectedHosts.filter((h) => h.roles.includes(NodeRole.MASTER)).length;

  const workerCount =
    config.deploymentMode === DeploymentMode.HYBRID
      ? selectedHosts.length
      : selectedHosts.filter((h) => h.roles.includes(NodeRole.WORKER)).length;

  return (
    <div className='h-full flex flex-col items-center justify-center max-w-xl mx-auto py-4 text-center'>
      {/* Success Hero / 成功光环 */}
      <div className='w-12 h-12 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-600 flex items-center justify-center mb-3 shadow-sm'>
        <CheckCircle2 className='w-6 h-6 stroke-[2.5]' />
      </div>

      <h2 className='text-lg font-bold text-foreground'>
        {t('cluster.wizard.completeTitle')}
      </h2>
      <p className='text-xs text-muted-foreground mt-1 max-w-md'>
        {t('cluster.wizard.completeSubtitle')}
      </p>

      {/* Cluster Summary Card / 集群规格摘要卡片 */}
      <Card className='w-full mt-5 text-left border-border/70 shadow-none bg-muted/20'>
        <CardContent className='p-4 space-y-3 text-xs'>
          <div className='flex items-center justify-between pb-2.5 border-b border-border/50'>
            <div className='flex items-center gap-2'>
              <Layers className='h-4 w-4 text-primary' />
              <span className='font-semibold text-sm'>{config.name}</span>
            </div>
            <Badge variant='outline' className='font-mono text-[11px]'>
              v{config.version}
            </Badge>
          </div>

          <div className='grid grid-cols-2 gap-3 pt-0.5'>
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
              <span className='font-medium text-foreground mt-0.5 block'>
                {t('cluster.wizard.summaryHosts', {
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
              <span className='font-mono text-foreground mt-0.5 block'>
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
              <span className='font-mono text-foreground mt-0.5 block'>
                {config.runtime.enable_http
                  ? t('cluster.wizard.httpEnabled', {port: config.httpPort})
                  : t('cluster.wizard.httpDisabled')}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Action Buttons / 底部快捷按钮 */}
      <div className='flex items-center gap-3 mt-6'>
        <Button variant='outline' onClick={onClose} className='h-9 text-xs'>
          {t('cluster.wizard.finishAndClose')}
        </Button>
        <Button onClick={onComplete} className='h-9 text-xs'>
          {t('cluster.wizard.enterConsole')}
          <ArrowRight className='h-3.5 w-3.5 ml-1.5' />
        </Button>
      </div>
    </div>
  );
}

export default ClusterCompleteStep;
