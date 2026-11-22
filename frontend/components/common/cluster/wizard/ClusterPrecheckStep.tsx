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
import {Button} from '@/components/arc/button/button';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Alert} from '@/components/arc/alert/alert';
import {NeutralChip, SoftChip, TruncatedText} from '@/components/common/layout';
import {
  CheckCircle2,
  AlertTriangle,
  XCircle,
  RotateCw,
  Server,
  SlidersHorizontal,
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
  /** Jump back to the config step / 预检失败时返回配置步骤 */
  onBackToConfig?: () => void;
}

export function ClusterPrecheckStep({
  precheckResults,
  precheckRunning,
  precheckHasRun,
  allPrechecksPassed,
  runPrecheck,
  selectedHosts,
  onBackToConfig,
}: ClusterPrecheckStepProps) {
  const t = useTranslations();

  // Auto-run precheck upon entering this step if not yet executed
  // 进入此步骤且尚未执行检查时，自动启动预检查
  useEffect(() => {
    if (!precheckHasRun && !precheckRunning && selectedHosts.length > 0) {
      runPrecheck();
    }
  }, [precheckHasRun, precheckRunning, selectedHosts.length, runPrecheck]);


  const jdkFailures = precheckResults
    .map((r) => {
      const javaCheck = r.result?.items?.find((c) =>
        c.name?.toLowerCase().includes('java'),
      );
      if (r.error) {
        return {host: r.hostName, detail: r.error};
      }
      if (javaCheck && javaCheck.status === 'failed') {
        return {
          host: r.hostName,
          detail: javaCheck.message || '',
        };
      }
      return null;
    })
    .filter(Boolean) as Array<{host: string; detail: string}>;

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
        return null;
    }
  };

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      {/* Top Header & Action Bar / 顶部摘要与操作栏 */}
      <div className='flex items-center justify-between pb-3 border-b-[0.5px] border-border/80'>
        <div className='flex items-center gap-2.5'>
          <span className='text-sm font-medium text-foreground'>
            {t('cluster.wizard.precheckOverview')}
          </span>
          {precheckRunning ? (
            <NeutralChip className='font-mono tabular-nums'>
              {t('cluster.wizard.precheckRunning')}
            </NeutralChip>
          ) : precheckHasRun ? (
            allPrechecksPassed ? (
              <SoftChip tone='emerald'>
                <CheckCircle2 className='h-3 w-3' aria-hidden />
                {t('cluster.wizard.precheckAllPassed')}
              </SoftChip>
            ) : (
              <SoftChip tone='rose'>
                <XCircle className='h-3 w-3' aria-hidden />
                {t('cluster.wizard.precheckPartialFailed')}
              </SoftChip>
            )
          ) : null}
        </div>

        <div className='flex items-center gap-2'>
          {/* 预检失败时可直接回配置步修正 / Failed prechecks can jump straight back to config */}
          {precheckHasRun && !allPrechecksPassed && !precheckRunning && onBackToConfig && (
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={onBackToConfig}
            >
              <SlidersHorizontal className='size-3.5' aria-hidden />
              {t('cluster.wizard.backToConfigStep')}
            </Button>
          )}
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={runPrecheck}
            disabled={precheckRunning}
            loading={precheckRunning}
          >
            {!precheckRunning && <RotateCw className='size-3.5' aria-hidden />}
            {precheckRunning
              ? t('cluster.wizard.precheckCheckingAction')
              : t('cluster.wizard.rerunPrecheck')}
          </Button>
        </div>
      </div>

      {/* JDK failure summary / JDK 失败汇总 */}
      {jdkFailures.length > 0 && !precheckRunning && (
        <div className='pt-3'>
          <Alert
            tone='danger'
            title={t('javaHome.failedNodes')}
            data-testid='cluster-wizard-jdk-failures'
          >
            <ul className='list-disc pl-4 space-y-0.5'>
              {jdkFailures.map((f) => (
                <li key={f.host}>
                  {f.detail
                    ? t('javaHome.failedNodeItem', {
                        host: f.host,
                        detail: f.detail,
                      })
                    : f.host}
                </li>
              ))}
            </ul>
          </Alert>
        </div>
      )}

      {/* Host Precheck Results List / 主机检查明细列表 */}
      <div className='flex-1 min-h-0 overflow-y-auto pr-4 mt-3'>
        <div className='max-w-4xl mx-auto'>
          {precheckResults.map((hostResult, index) => {
            const overallStatus = hostResult.result?.overall_status;

            return (
              <section
                key={hostResult.hostId}
                className={cn(
                  'py-3',
                  index > 0 && 'border-t-[0.5px] border-border/80',
                )}
              >
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <Server className='h-3.5 w-3.5 text-muted-foreground' />
                    <span className='text-sm font-semibold'>
                      {hostResult.hostName}
                    </span>
                  </div>

                  {hostResult.loading ? (
                    <NeutralChip>{t('cluster.wizard.checking')}</NeutralChip>
                  ) : hostResult.error ? (
                    <SoftChip tone='rose'>
                      {t('cluster.wizard.checkError')}
                    </SoftChip>
                  ) : overallStatus === 'passed' ? (
                    <SoftChip tone='emerald'>
                      {t('cluster.wizard.precheckItemReady')}
                    </SoftChip>
                  ) : overallStatus === 'warning' ? (
                    <SoftChip tone='amber'>
                      {t('cluster.wizard.precheckItemWarning')}
                    </SoftChip>
                  ) : (
                    <SoftChip tone='rose'>
                      {t('cluster.wizard.precheckItemFailed')}
                    </SoftChip>
                  )}
                </div>

                <div className='pt-2.5'>
                  {hostResult.loading ? (
                    <Skeleton
                      lines={2}
                      label={t('cluster.wizard.precheckItemProgress')}
                    />
                  ) : hostResult.error ? (
                    <Alert tone='danger' title={hostResult.error} />
                  ) : hostResult.result ? (
                    <div className='space-y-1.5'>
                      <div className='grid grid-cols-1 sm:grid-cols-2 gap-1.5'>
                        {hostResult.result.items.map((item, idx) => (
                          <div
                            key={idx}
                            className={cn(
                              'flex items-center justify-between gap-2 p-2 rounded-md text-xs border-[0.5px]',
                              item.status === 'passed' &&
                                'border-border/80',
                              item.status === 'warning' &&
                                'border-amber-500/40 bg-amber-500/5',
                              item.status === 'failed' &&
                                'border-destructive/40 bg-destructive/5',
                            )}
                          >
                            <div className='flex items-center gap-2 min-w-0'>
                              {getStatusIcon(item.status)}
                              <span className='font-medium capitalize truncate'>
                                {item.name}
                              </span>
                            </div>
                            <TruncatedText
                              text={
                                localizeBackendText(item.message) ||
                                (item.status === 'passed'
                                  ? t('cluster.wizard.statusNormal')
                                  : t('cluster.wizard.statusAbnormal'))
                              }
                              className='max-w-[55%] text-right text-[11px] text-muted-foreground'
                            />
                          </div>
                        ))}
                      </div>

                      {hostResult.result.summary && (
                        <p className='text-[11px] text-muted-foreground pt-1.5 border-t-[0.5px] border-border/80'>
                          {localizeBackendText(hostResult.result.summary)}
                        </p>
                      )}
                    </div>
                  ) : null}
                </div>
              </section>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export default ClusterPrecheckStep;
