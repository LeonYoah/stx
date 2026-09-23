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
 * Cluster Precheck Step Component
 * 集群环境预检查步骤组件
 *
 * Automated, streamlined host environment and port precheck step.
 * 自动化、紧凑流畅的主机环境、端口与依赖项预检查步骤。
 */

'use client';

import React, {useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Loader2,
  RotateCw,
  Server,
} from 'lucide-react';
import {cn} from '@/lib/utils';
import {localizeBackendText} from '@/lib/i18n/localize-text';
import type {CheckStatus} from '@/lib/services/installer/types';
import {HostPrecheckResult, HostWithRole} from './types';

interface ClusterPrecheckStepProps {
  /** Precheck results per host / 各主机的预检查结果 */
  precheckResults: HostPrecheckResult[];
  /** Whether precheck is running / 是否正在运行检查 */
  precheckRunning: boolean;
  /** Whether precheck has completed / 是否已完成检查 */
  precheckHasRun: boolean;
  /** Whether all hosts passed / 是否所有主机均通过 */
  allPrechecksPassed: boolean;
  /** Trigger precheck function / 触发预检查方法 */
  runPrecheck: () => Promise<void>;
  /** Selected hosts list / 选中主机列表 */
  selectedHosts: HostWithRole[];
}

export function ClusterPrecheckStep({
  precheckResults,
  precheckRunning,
  precheckHasRun,
  allPrechecksPassed,
  runPrecheck,
  selectedHosts,
}: ClusterPrecheckStepProps) {
  const t = useTranslations();

  // Auto-run precheck upon entering this step if not yet executed
  // 进入此步骤且尚未执行检查时，自动启动预检查
  useEffect(() => {
    if (!precheckHasRun && !precheckRunning && selectedHosts.length > 0) {
      runPrecheck();
    }
  }, [precheckHasRun, precheckRunning, selectedHosts.length, runPrecheck]);

  const getStatusIcon = (status: CheckStatus) => {
    switch (status) {
      case 'passed':
        return (
          <CheckCircle2 className='h-3.5 w-3.5 text-emerald-500 shrink-0' />
        );
      case 'failed':
        return <XCircle className='h-3.5 w-3.5 text-red-500 shrink-0' />;
      case 'warning':
        return (
          <AlertTriangle className='h-3.5 w-3.5 text-amber-500 shrink-0' />
        );
      default:
        return (
          <Loader2 className='h-3.5 w-3.5 animate-spin text-muted-foreground shrink-0' />
        );
    }
  };

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      {/* Top Header & Action Bar / 顶部摘要与操作栏 */}
      <div className='flex items-center justify-between pb-3 border-b'>
        <div className='flex items-center gap-2.5'>
          <span className='text-xs font-semibold uppercase tracking-wider text-muted-foreground'>
            {t('cluster.wizard.precheckOverview')}
          </span>
          {precheckRunning ? (
            <Badge
              variant='outline'
              className='h-5 text-[11px] gap-1 text-primary border-primary/30'
            >
              <Loader2 className='h-3 w-3 animate-spin' />
              {t('cluster.wizard.precheckRunning')}
            </Badge>
          ) : precheckHasRun ? (
            allPrechecksPassed ? (
              <Badge
                variant='outline'
                className='h-5 text-[11px] gap-1 text-emerald-600 bg-emerald-500/10 border-emerald-500/30 font-medium'
              >
                <CheckCircle2 className='h-3 w-3' />
                {t('cluster.wizard.precheckAllPassed')}
              </Badge>
            ) : (
              <Badge
                variant='outline'
                className='h-5 text-[11px] gap-1 text-destructive bg-destructive/10 border-destructive/30 font-medium'
              >
                <XCircle className='h-3 w-3' />
                {t('cluster.wizard.precheckPartialFailed')}
              </Badge>
            )
          ) : null}
        </div>

        <Button
          variant='outline'
          size='sm'
          onClick={runPrecheck}
          disabled={precheckRunning}
          className='h-7 text-xs px-2.5'
        >
          <RotateCw
            className={cn('h-3 w-3 mr-1.5', precheckRunning && 'animate-spin')}
          />
          {precheckRunning
            ? t('cluster.wizard.precheckCheckingAction')
            : t('cluster.wizard.rerunPrecheck')}
        </Button>
      </div>

      {/* Host Precheck Results List / 主机检查明细列表 */}
      <ScrollArea className='flex-1 min-h-0 pr-4 mt-3'>
        <div className='space-y-3 max-w-4xl mx-auto'>
          {precheckResults.map((hostResult) => {
            const overallStatus = hostResult.result?.overall_status;
            const hasError =
              !hostResult.loading &&
              (overallStatus === 'failed' || hostResult.error);

            return (
              <Card
                key={hostResult.hostId}
                className={cn(
                  'shadow-none transition-all',
                  hasError
                    ? 'border-destructive/40 bg-destructive/[0.02]'
                    : 'border-border/70',
                )}
              >
                <CardHeader className='py-2.5 px-3.5 border-b bg-muted/10 flex flex-row items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <Server className='h-3.5 w-3.5 text-muted-foreground' />
                    <CardTitle className='text-xs font-semibold'>
                      {hostResult.hostName}
                    </CardTitle>
                  </div>

                  {hostResult.loading ? (
                    <Badge
                      variant='outline'
                      className='text-[10px] h-4 gap-1 text-muted-foreground'
                    >
                      <Loader2 className='h-2.5 w-2.5 animate-spin' />
                      {t('cluster.wizard.checking')}
                    </Badge>
                  ) : hostResult.error ? (
                    <Badge variant='destructive' className='text-[10px] h-4'>
                      {t('cluster.wizard.checkError')}
                    </Badge>
                  ) : overallStatus === 'passed' ? (
                    <Badge
                      variant='outline'
                      className='text-[10px] h-4 text-emerald-600 bg-emerald-500/10 border-emerald-500/20'
                    >
                      {t('cluster.wizard.precheckItemReady')}
                    </Badge>
                  ) : overallStatus === 'warning' ? (
                    <Badge
                      variant='outline'
                      className='text-[10px] h-4 text-amber-600 bg-amber-500/10 border-amber-500/20'
                    >
                      {t('cluster.wizard.precheckItemWarning')}
                    </Badge>
                  ) : (
                    <Badge variant='destructive' className='text-[10px] h-4'>
                      {t('cluster.wizard.precheckItemFailed')}
                    </Badge>
                  )}
                </CardHeader>

                <CardContent className='p-3'>
                  {hostResult.loading ? (
                    <div className='flex items-center justify-center py-4 text-xs text-muted-foreground'>
                      <Loader2 className='h-4 w-4 animate-spin mr-2' />
                      {t('cluster.wizard.precheckItemProgress')}
                    </div>
                  ) : hostResult.error ? (
                    <div className='p-2.5 rounded-md bg-destructive/10 text-destructive text-xs'>
                      {hostResult.error}
                    </div>
                  ) : hostResult.result ? (
                    <div className='space-y-1.5'>
                      <div className='grid grid-cols-1 sm:grid-cols-2 gap-1.5'>
                        {hostResult.result.items.map((item, idx) => (
                          <div
                            key={idx}
                            className={cn(
                              'flex items-center justify-between p-2 rounded-md text-xs border',
                              item.status === 'passed' &&
                                'bg-background border-border/60',
                              item.status === 'warning' &&
                                'bg-amber-500/5 border-amber-500/30 text-amber-900 dark:text-amber-200',
                              item.status === 'failed' &&
                                'bg-destructive/5 border-destructive/30 text-destructive',
                            )}
                          >
                            <div className='flex items-center gap-2 min-w-0'>
                              {getStatusIcon(item.status)}
                              <span className='font-medium capitalize truncate'>
                                {item.name}
                              </span>
                            </div>
                            <span className='text-[11px] text-muted-foreground truncate max-w-[55%] text-right'>
                              {localizeBackendText(item.message) ||
                                (item.status === 'passed'
                                  ? t('cluster.wizard.statusNormal')
                                  : t('cluster.wizard.statusAbnormal'))}
                            </span>
                          </div>
                        ))}
                      </div>

                      {hostResult.result.summary && (
                        <p className='text-[11px] text-muted-foreground pt-1.5 border-t border-border/40'>
                          {localizeBackendText(hostResult.result.summary)}
                        </p>
                      )}
                    </div>
                  ) : null}
                </CardContent>
              </Card>
            );
          })}
        </div>
      </ScrollArea>
    </div>
  );
}

export default ClusterPrecheckStep;
