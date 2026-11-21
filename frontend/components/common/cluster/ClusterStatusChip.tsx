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

'use client';

/**
 * 集群与节点语义状态徽章：复用公共 SoftChip，消除硬编码色点。
 * Semantic status chips for clusters and nodes, reusing shared SoftChip instead of ad-hoc colors.
 */
import {useTranslations} from 'next-intl';
import {SoftChip, type ChipTone} from '@/components/common/layout';
import {
  ClusterStatus,
  NodeStatus,
  type ClusterInfo,
} from '@/lib/services/cluster/types';
import {cn} from '@/lib/utils';

const DOT_TONES: Record<ChipTone, string> = {
  emerald: 'bg-emerald-500',
  blue: 'bg-blue-500',
  amber: 'bg-amber-500',
  rose: 'bg-rose-500',
  muted: 'bg-muted-foreground/60',
};

/**
 * 运行中但 0 节点在线或后端标记 unhealthy 时，按异常态高亮显示。
 * Highlight running clusters with 0 online nodes or explicit unhealthy status as unhealthy.
 */
export function isClusterRunningUnhealthy(
  cluster: Pick<ClusterInfo, 'status' | 'online_nodes' | 'health_status'>,
): boolean {
  return (
    cluster.status === ClusterStatus.RUNNING &&
    (cluster.online_nodes === 0 || cluster.health_status === 'unhealthy')
  );
}

function resolveClusterTone(
  cluster: Pick<ClusterInfo, 'status' | 'online_nodes' | 'health_status'>,
): {tone: ChipTone; pulse?: boolean} {
  if (isClusterRunningUnhealthy(cluster)) {
    return {tone: 'rose'};
  }
  switch (cluster.status) {
    case ClusterStatus.RUNNING:
      return {tone: 'emerald'};
    case ClusterStatus.DEPLOYING:
      return {tone: 'blue', pulse: true};
    case ClusterStatus.ERROR:
      return {tone: 'rose'};
    case ClusterStatus.CREATED:
    case ClusterStatus.STOPPED:
    default:
      return {tone: 'muted'};
  }
}

export function ClusterStatusChip({
  cluster,
  className,
}: {
  cluster: Pick<ClusterInfo, 'status' | 'online_nodes' | 'health_status'>;
  className?: string;
}) {
  const t = useTranslations();
  const unhealthy = isClusterRunningUnhealthy(cluster);
  const {tone, pulse} = resolveClusterTone(cluster);
  const key = unhealthy
    ? 'cluster.healthStatuses.unhealthy'
    : `cluster.statuses.${cluster.status}`;
  const label = t.has(key) ? t(key) : cluster.status;

  return (
    <SoftChip tone={tone} className={cn('font-medium', className)}>
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          DOT_TONES[tone],
          pulse && 'animate-pulse',
        )}
        aria-hidden
      />
      <span className='whitespace-nowrap'>{label}</span>
    </SoftChip>
  );
}

function resolveNodeTone(status: NodeStatus): {tone: ChipTone; pulse?: boolean} {
  switch (status) {
    case NodeStatus.RUNNING:
      return {tone: 'emerald'};
    case NodeStatus.INSTALLING:
    case NodeStatus.PENDING:
      return {tone: 'blue', pulse: status === NodeStatus.INSTALLING};
    case NodeStatus.ERROR:
    case NodeStatus.OFFLINE:
      return {tone: 'rose'};
    case NodeStatus.STOPPED:
    default:
      return {tone: 'muted'};
  }
}

export function NodeStatusChip({
  status,
  className,
}: {
  status: NodeStatus;
  className?: string;
}) {
  const t = useTranslations();
  const {tone, pulse} = resolveNodeTone(status);
  const key = `cluster.nodeStatuses.${status}`;
  const label = t.has(key) ? t(key) : status;

  return (
    <SoftChip tone={tone} className={cn('font-medium', className)}>
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          DOT_TONES[tone],
          pulse && 'animate-pulse',
        )}
        aria-hidden
      />
      <span className='whitespace-nowrap'>{label}</span>
    </SoftChip>
  );
}
