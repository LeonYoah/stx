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
 * Cluster Hosts Selection Step Component
 * 集群主机选择与角色分配步骤
 *
 * Clean, high-density host selector with inline role toggles for separated mode.
 * 紧凑高密度的主机选择列表，支持分离模式下行内直接切换 Master/Worker 角色。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Checkbox} from '@/components/ui/checkbox';
import {Badge} from '@/components/ui/badge';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Server, Crown, Wrench, Loader2, Cpu, HardDrive} from 'lucide-react';
import {cn} from '@/lib/utils';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';
import {HostWithRole} from './types';

interface ClusterHostsStepProps {
  /** Host list with roles / 带角色的主机列表 */
  hostsWithRole: HostWithRole[];
  /** Loading status / 加载状态 */
  loadingHosts: boolean;
  /** Current deployment mode / 当前部署模式 */
  deploymentMode: DeploymentMode;
  /** Toggle host selection callback / 切换主机选中状态回调 */
  toggleHostSelection: (hostId: number) => void;
  /** Toggle host role callback / 切换主机角色回调 */
  toggleHostRole: (hostId: number, role: NodeRole) => void;
  /** Selected hosts list / 已选择的主机列表 */
  selectedHosts: HostWithRole[];
}

export function ClusterHostsStep({
  hostsWithRole,
  loadingHosts,
  deploymentMode,
  toggleHostSelection,
  toggleHostRole,
  selectedHosts,
}: ClusterHostsStepProps) {
  const t = useTranslations();

  // Calculate master and worker counts in separated mode / 计算分离模式下 Master 和 Worker 数量
  const masterCount = selectedHosts.filter((h) =>
    h.roles.includes(NodeRole.MASTER),
  ).length;
  const workerCount = selectedHosts.filter((h) =>
    h.roles.includes(NodeRole.WORKER),
  ).length;

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      {/* Top Status Bar / 顶部状态栏 */}
      <div className='flex items-center justify-between pb-3 border-b'>
        <div className='flex items-center gap-2'>
          <span className='text-xs font-semibold uppercase tracking-wider text-muted-foreground'>
            {t('cluster.wizard.hostsPool')}
          </span>
          <Badge variant='outline' className='text-xs h-5 px-2 font-mono'>
            {t('cluster.wizard.hostsSelectedShort', {
              selected: selectedHosts.length,
              total: hostsWithRole.length,
            })}
          </Badge>
        </div>

        {/* Separated Mode Role Summary / 分离模式角色摘要 */}
        {deploymentMode === DeploymentMode.SEPARATED && (
          <div className='flex items-center gap-2 text-xs'>
            <Badge
              variant='outline'
              className={cn(
                'h-5 gap-1 font-mono transition-colors',
                masterCount > 0
                  ? 'border-amber-500/40 text-amber-700 dark:text-amber-300 bg-amber-500/10'
                  : 'text-muted-foreground border-dashed',
              )}
            >
              <Crown className='h-3 w-3 text-amber-500' />
              Master: {masterCount}
            </Badge>
            <Badge
              variant='outline'
              className={cn(
                'h-5 gap-1 font-mono transition-colors',
                workerCount > 0
                  ? 'border-blue-500/40 text-blue-700 dark:text-blue-300 bg-blue-500/10'
                  : 'text-muted-foreground border-dashed',
              )}
            >
              <Wrench className='h-3 w-3 text-blue-500' />
              Worker: {workerCount}
            </Badge>
          </div>
        )}
      </div>

      {/* Host Cards List / 主机卡片列表 */}
      <ScrollArea className='flex-1 min-h-0 pr-4 mt-3'>
        {loadingHosts ? (
          <div className='flex items-center justify-center py-16 text-muted-foreground'>
            <Loader2 className='h-6 w-6 animate-spin mr-2' />
            <span className='text-sm'>{t('cluster.wizard.loadingHosts')}</span>
          </div>
        ) : hostsWithRole.length === 0 ? (
          <div className='text-center py-16 text-muted-foreground'>
            <Server className='h-10 w-10 mx-auto mb-3 opacity-40' />
            <p className='text-sm font-medium'>
              {t('cluster.wizard.noAvailableHosts')}
            </p>
            <p className='text-xs text-muted-foreground/70 mt-1'>
              {t('cluster.wizard.noAvailableHostsDesc')}
            </p>
          </div>
        ) : (
          <div className='space-y-2'>
            {hostsWithRole.map((item) => {
              const memoryGb = (
                (item.host.total_memory || 0) /
                1024 /
                1024 /
                1024
              ).toFixed(1);
              const isMaster = item.roles.includes(NodeRole.MASTER);
              const isWorker = item.roles.includes(NodeRole.WORKER);

              return (
                <div
                  key={item.host.id}
                  onClick={() => toggleHostSelection(item.host.id)}
                  className={cn(
                    'group cursor-pointer rounded-lg border p-3 transition-all flex items-center gap-3.5',
                    item.selected
                      ? 'border-primary/60 bg-primary/[0.03] shadow-2xs ring-1 ring-primary/20'
                      : 'border-border/60 hover:border-border hover:bg-muted/30',
                  )}
                >
                  <Checkbox
                    checked={item.selected}
                    onCheckedChange={() => toggleHostSelection(item.host.id)}
                    onClick={(e) => e.stopPropagation()}
                    className='shrink-0'
                  />

                  {/* Host Basic Info / 主机基础信息 */}
                  <div className='flex-1 min-w-0 flex items-center justify-between gap-4'>
                    <div className='min-w-0'>
                      <div className='flex items-center gap-2'>
                        <span className='text-sm font-semibold truncate'>
                          {item.host.name}
                        </span>
                        <Badge
                          variant='secondary'
                          className='font-mono text-[11px] px-1.5 py-0 h-4 bg-muted text-muted-foreground'
                        >
                          {item.host.ip_address}
                        </Badge>
                      </div>

                      {/* Hardware Specs Pills / 硬件规格胶囊 */}
                      <div className='flex items-center gap-2.5 text-[11px] text-muted-foreground mt-1'>
                        <span className='flex items-center gap-1'>
                          <Cpu className='h-3 w-3 opacity-70' />
                          {t('cluster.wizard.cpuCores', {
                            count: item.host.cpu_cores ?? 0,
                          })}
                        </span>
                        <span className='text-border'>•</span>
                        <span className='flex items-center gap-1'>
                          <HardDrive className='h-3 w-3 opacity-70' />
                          {memoryGb} GB
                        </span>
                      </div>
                    </div>

                    {/* Role Toggles (Separated Mode) / 分离模式角色微切换按钮 */}
                    {item.selected &&
                      deploymentMode === DeploymentMode.SEPARATED && (
                        <div
                          className='flex items-center gap-1.5 shrink-0'
                          onClick={(e) => e.stopPropagation()}
                        >
                          <button
                            type='button'
                            onClick={() =>
                              toggleHostRole(item.host.id, NodeRole.MASTER)
                            }
                            className={cn(
                              'px-2.5 py-1 rounded-md text-xs font-medium border flex items-center gap-1 transition-all',
                              isMaster
                                ? 'bg-amber-500/15 border-amber-500/40 text-amber-700 dark:text-amber-300 font-semibold shadow-2xs'
                                : 'bg-background border-border/70 text-muted-foreground hover:bg-muted/50',
                            )}
                          >
                            <Crown className='h-3 w-3 text-amber-500' />
                            Master
                          </button>
                          <button
                            type='button'
                            onClick={() =>
                              toggleHostRole(item.host.id, NodeRole.WORKER)
                            }
                            className={cn(
                              'px-2.5 py-1 rounded-md text-xs font-medium border flex items-center gap-1 transition-all',
                              isWorker
                                ? 'bg-blue-500/15 border-blue-500/40 text-blue-700 dark:text-blue-300 font-semibold shadow-2xs'
                                : 'bg-background border-border/70 text-muted-foreground hover:bg-muted/50',
                            )}
                          >
                            <Wrench className='h-3 w-3 text-blue-500' />
                            Worker
                          </button>
                        </div>
                      )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </ScrollArea>
    </div>
  );
}

export default ClusterHostsStep;
