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
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Button} from '@/components/arc/button/button';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {Server, Crown, Wrench, Cpu, HardDrive} from 'lucide-react';
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
  const router = useRouter();

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
      <div className='flex items-center justify-between pb-3 border-b-[0.5px] border-border/80'>
        <div className='flex items-center gap-2'>
          <span className='text-sm font-medium text-foreground'>
            {t('cluster.wizard.hostsPool')}
          </span>
          <NeutralChip className='font-mono tabular-nums'>
            {t('cluster.wizard.hostsSelectedShort', {
              selected: selectedHosts.length,
              total: hostsWithRole.length,
            })}
          </NeutralChip>
        </div>

        {/* Separated Mode Role Summary / 分离模式角色摘要 */}
        {deploymentMode === DeploymentMode.SEPARATED && (
          <div className='flex items-center gap-2 text-xs'>
            {masterCount > 0 ? (
              <SoftChip tone='amber' className='font-mono tabular-nums'>
                <Crown className='h-3 w-3' aria-hidden />
                Master: {masterCount}
              </SoftChip>
            ) : (
              <NeutralChip className='font-mono tabular-nums border-dashed'>
                <Crown className='h-3 w-3' aria-hidden />
                Master: 0
              </NeutralChip>
            )}
            {workerCount > 0 ? (
              <SoftChip tone='blue' className='font-mono tabular-nums'>
                <Wrench className='h-3 w-3' aria-hidden />
                Worker: {workerCount}
              </SoftChip>
            ) : (
              <NeutralChip className='font-mono tabular-nums border-dashed'>
                <Wrench className='h-3 w-3' aria-hidden />
                Worker: 0
              </NeutralChip>
            )}
          </div>
        )}
      </div>

      {/* Host List / 主机列表 */}
      <div className='flex-1 min-h-0 overflow-y-auto pr-4 mt-3'>
        {loadingHosts ? (
          <Skeleton lines={5} label={t('cluster.wizard.loadingHosts')} />
        ) : hostsWithRole.length === 0 ? (
          <EmptyState
            icon={<Server className='size-5' aria-hidden />}
            title={t('cluster.wizard.noAvailableHosts')}
            description={t('cluster.wizard.noAvailableHostsDesc')}
            className='py-12'
            action={
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => router.push('/hosts')}
              >
                {t('cluster.goToHostsManagement')}
              </Button>
            }
          />
        ) : (
          <div className='divide-y-[0.5px] divide-border/80 border-[0.5px] border-border/80 rounded-md'>
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
                    'group cursor-pointer p-3 transition-colors flex items-center gap-3.5',
                    item.selected
                      ? 'bg-primary/[0.04]'
                      : 'hover:bg-muted/30',
                  )}
                >
                  <Checkbox
                    checked={item.selected}
                    onCheckedChange={() => toggleHostSelection(item.host.id)}
                    onClick={(e) => e.stopPropagation()}
                    aria-label={item.host.name}
                    className='shrink-0'
                  />

                  {/* Host Basic Info / 主机基础信息 */}
                  <div className='flex-1 min-w-0 flex items-center justify-between gap-4'>
                    <div className='min-w-0'>
                      <div className='flex items-center gap-2'>
                        <span className='text-sm font-semibold truncate'>
                          {item.host.name}
                        </span>
                        <NeutralChip className='font-mono tabular-nums'>
                          {item.host.ip_address}
                        </NeutralChip>
                      </div>

                      {/* Hardware Specs / 硬件规格 */}
                      <div className='flex items-center gap-2.5 text-[11px] text-muted-foreground mt-1 tabular-nums'>
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

                    {/* Role Toggles (Separated Mode) / 分离模式角色勾选 */}
                    {item.selected &&
                      deploymentMode === DeploymentMode.SEPARATED && (
                        <div
                          className='flex items-center gap-3 shrink-0'
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Checkbox
                            checked={isMaster}
                            onCheckedChange={() =>
                              toggleHostRole(item.host.id, NodeRole.MASTER)
                            }
                            label='Master'
                          />
                          <Checkbox
                            checked={isWorker}
                            onCheckedChange={() =>
                              toggleHostRole(item.host.id, NodeRole.WORKER)
                            }
                            label='Worker'
                          />
                        </div>
                      )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

export default ClusterHostsStep;
