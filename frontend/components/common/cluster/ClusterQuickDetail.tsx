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
 * 集群右侧常驻速览栏内容：单层扁平画布 + 留白与 0.5px 细线自然分组，
 * 供集群列表页 DetailHost 展示核心规格、节点健康速览、活跃告警与完整控制台入口。
 * Cluster persistent rail quick-detail view: flat single-layer surface separated by
 * whitespace and 0.5px hairlines, displaying specs, node health preview, alerts, and console entry.
 */
import {useCallback, useEffect, useRef, useState} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {
  Activity,
  ArrowRight,
  Boxes,
  Bug,
  FileCode2,
  Pencil,
  Play,
  RotateCcw,
  Square,
  Trash2,
} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {NeutralChip} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {Skeleton} from '@/components/ui/skeleton';
import services from '@/lib/services';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {
  OperationType,
  type ClusterInfo,
  type ClusterStatusInfo,
  type NodeInfo,
} from '@/lib/services/cluster/types';
import type {ClusterMonitoringOverviewData} from '@/lib/services/monitoring';
import {ClusterStatusChip, NodeStatusChip} from './ClusterStatusChip';
import {getClusterActionFlags} from './useClusterLifecycle';
import {getClusterDeployModeLabel} from '@/lib/infra';
import {CopyButton} from '@/components/arc/copy-button/copy-button';

const MAX_PREVIEW_NODES = 5;

export interface ClusterQuickDetailProps {
  cluster: ClusterInfo;
  operatingAction: OperationType | 'delete' | null;
  onEdit: (cluster: ClusterInfo) => void;
  onLifecycle: (cluster: ClusterInfo, operation: OperationType) => void;
  onDelete: (cluster: ClusterInfo) => void;
}

function getRoleKey(role: string): string {
  return role === 'master/worker' ? 'masterWorker' : role;
}

function CopyValueButton({
  value,
  label,
}: {
  value: string;
  label: string;
}) {
  return (
    <CopyButton
      value={value}
      label={label}
      variant='plain'
      iconOnly
      className='h-6 w-6 min-h-0 shrink-0'
    />
  );
}

export function ClusterQuickDetail({
  cluster,
  operatingAction,
  onEdit,
  onLifecycle,
  onDelete,
}: ClusterQuickDetailProps) {
  const t = useTranslations();
  const router = useRouter();

  const [detail, setDetail] = useState<ClusterInfo>(cluster);
  const [statusInfo, setStatusInfo] = useState<ClusterStatusInfo | null>(null);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [monitoringOverview, setMonitoringOverview] =
    useState<ClusterMonitoringOverviewData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const reqSeqRef = useRef(0);

  useEffect(() => {
    setDetail(cluster);
  }, [cluster]);

  const loadSnapshot = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    setError(null);
    try {
      const [clusterRes, statusRes, nodesRes, monitorRes] = await Promise.all([
        services.cluster.getClusterSafe(cluster.id),
        services.cluster.getClusterStatusSafe(cluster.id),
        services.cluster.getNodesSafe(cluster.id),
        services.monitoring
          .getClusterOverviewSafe(cluster.id)
          .catch(() => ({success: false, data: undefined})),
      ]);

      if (seq !== reqSeqRef.current) {
        return;
      }

      if (clusterRes.success && clusterRes.data) {
        setDetail(clusterRes.data);
      } else if (!clusterRes.success) {
        setError(clusterRes.error || t('cluster.list.detailLoadError'));
      }

      if (statusRes.success && statusRes.data) {
        setStatusInfo(statusRes.data);
      } else {
        setStatusInfo(null);
      }

      if (nodesRes.success && Array.isArray(nodesRes.data)) {
        setNodes(nodesRes.data);
      } else {
        setNodes([]);
      }

      if (monitorRes.success && monitorRes.data) {
        setMonitoringOverview(monitorRes.data);
      } else {
        setMonitoringOverview(null);
      }
    } finally {
      if (seq === reqSeqRef.current) {
        setLoading(false);
      }
    }
  }, [cluster.id, t]);

  useEffect(() => {
    void loadSnapshot();
  }, [loadSnapshot]);

  const mergedCluster: ClusterInfo = {
    ...detail,
    online_nodes: statusInfo?.online_nodes ?? detail.online_nodes,
    health_status: statusInfo?.health_status ?? detail.health_status,
    node_count: statusInfo?.total_nodes ?? detail.node_count,
  };

  const {canStart, canStop, canRestart, canDelete} =
    getClusterActionFlags(mergedCluster);
  const busy = operatingAction !== null;
  const totalNodes = statusInfo?.total_nodes ?? mergedCluster.node_count ?? 0;
  const onlineNodes =
    statusInfo?.online_nodes ?? mergedCluster.online_nodes ?? 0;
  const activeAlerts = monitoringOverview?.stats.active_alerts_1h ?? 0;
  const previewNodes = nodes.slice(0, MAX_PREVIEW_NODES);
  const modeKey = `cluster.modes.${mergedCluster.deployment_mode}`;
  const modeLabel = t.has(modeKey) ? t(modeKey) : mergedCluster.deployment_mode;
  const deployModeLabel = getClusterDeployModeLabel(mergedCluster.infra_type);

  return (
    <div className={layout.detailScroll} data-testid='cluster-quick-detail'>
      {/* 顶部主动作与生命周期控制 / Primary console entry and lifecycle actions */}
      <div className='space-y-3 border-b-[0.5px] border-border/80 pb-4'>
        <div className='flex flex-wrap items-center gap-2'>
          <ClusterStatusChip cluster={mergedCluster} />
          <NeutralChip>{deployModeLabel}</NeutralChip>
          <NeutralChip>{modeLabel}</NeutralChip>
          {mergedCluster.version ? (
            <NeutralChip className='font-mono tabular-nums'>
              v{mergedCluster.version}
            </NeutralChip>
          ) : null}
        </div>
        {mergedCluster.description ? (
          <p className='text-xs leading-relaxed text-muted-foreground'>
            {mergedCluster.description}
          </p>
        ) : null}
        <div className='flex flex-wrap items-center gap-1.5 pt-1'>
          <Button
            type='button'
            size='sm'
            onClick={() => router.push(`/clusters/${mergedCluster.id}`)}
            data-testid='cluster-quick-open-console'
          >
            {t('cluster.list.openConsole')}
            <ArrowRight className='size-3.5' aria-hidden />
          </Button>
          {canStart ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={busy}
              loading={operatingAction === OperationType.START}
              onClick={() => onLifecycle(mergedCluster, OperationType.START)}
            >
              <Play className='size-3.5' aria-hidden />
              {t('cluster.start')}
            </Button>
          ) : null}
          {canStop ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={busy}
              loading={operatingAction === OperationType.STOP}
              onClick={() => onLifecycle(mergedCluster, OperationType.STOP)}
            >
              <Square className='size-3.5' aria-hidden />
              {t('cluster.stop')}
            </Button>
          ) : null}
          {canRestart ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={busy}
              loading={operatingAction === OperationType.RESTART}
              onClick={() => onLifecycle(mergedCluster, OperationType.RESTART)}
            >
              <RotateCcw className='size-3.5' aria-hidden />
              {t('cluster.restart')}
            </Button>
          ) : null}
          <Button
            type='button'
            size='sm'
            variant='ghost'
            iconOnly
            aria-label={t('common.edit')}
            title={t('common.edit')}
            onClick={() => onEdit(mergedCluster)}
          >
            <Pencil className='size-3.5' aria-hidden />
          </Button>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            iconOnly
            disabled={!canDelete || busy}
            aria-label={t('common.delete')}
            title={t('common.delete')}
            onClick={() => onDelete(mergedCluster)}
          >
            <Trash2 className='size-3.5 text-destructive' aria-hidden />
          </Button>
        </div>
      </div>

      {/* 活跃告警单行提示 / Active alerts inline alert */}
      {activeAlerts > 0 ? (
        <div className='border-b-[0.5px] border-border/80 py-4'>
          <Alert
            tone='danger'
            title={t('cluster.activeAlertsBanner.title', {count: activeAlerts})}
          >
            <div className='mt-2'>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={() =>
                  router.push(
                    `/monitoring?tab=alerts&cluster_id=${mergedCluster.id}`,
                  )
                }
              >
                {t('cluster.activeAlertsBanner.action')}
              </Button>
            </div>
          </Alert>
        </div>
      ) : null}

      {/* 加载错误内联重试 / Inline error with retry */}
      {error ? (
        <div className='flex items-center justify-between gap-2 border-b-[0.5px] border-border/80 py-3 text-xs text-destructive'>
          <span className='min-w-0 truncate' title={error}>
            {error}
          </span>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={() => void loadSnapshot()}
          >
            {t('monitoring.retry')}
          </Button>
        </div>
      ) : null}

      {/* 分组 1：核心规格 / Section 1: Core specifications */}
      <section className='space-y-3 border-b-[0.5px] border-border/80 py-4'>
        <h4 className='text-xs font-medium text-muted-foreground'>
          {t('cluster.list.quickSpecs')}
        </h4>
        <dl className='grid grid-cols-2 gap-x-4 gap-y-3 text-xs'>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.clusterCode')}</dt>
            <dd className='flex items-center gap-1 font-mono font-medium'>
              <span
                className='min-w-0 truncate'
                title={mergedCluster.cluster_code || '-'}
              >
                {mergedCluster.cluster_code || '-'}
              </span>
              {mergedCluster.cluster_code ? (
                <CopyValueButton
                  value={mergedCluster.cluster_code}
                  label={t('common.copy')}
                />
              ) : null}
            </dd>
          </div>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.updatedAt')}</dt>
            <dd className='truncate font-medium tabular-nums'>
              {mergedCluster.updated_at
                ? formatDateTimeMinuteTz(mergedCluster.updated_at)
                : '-'}
            </dd>
          </div>
          <div className='col-span-2 min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.installDir')}</dt>
            <dd className='flex items-center gap-1.5 rounded-md bg-muted/50 px-2.5 py-1.5 font-mono text-xs'>
              <span
                className='min-w-0 flex-1 truncate'
                title={mergedCluster.install_dir || '-'}
              >
                {mergedCluster.install_dir || '-'}
              </span>
              {mergedCluster.install_dir ? (
                <CopyValueButton
                  value={mergedCluster.install_dir}
                  label={t('common.copy')}
                />
              ) : null}
            </dd>
          </div>
        </dl>
      </section>

      {/* 分组 2：节点健康速览 / Section 2: Node health preview */}
      <section className='space-y-3 border-b-[0.5px] border-border/80 py-4'>
        <div className='flex items-center justify-between gap-2'>
          <h4 className='text-xs font-medium text-muted-foreground'>
            {t('cluster.list.quickNodes')}
          </h4>
          <span className='text-xs font-medium tabular-nums'>
            <span
              className={
                totalNodes > 0 && onlineNodes < totalNodes
                  ? 'text-rose-600 dark:text-rose-400'
                  : 'text-foreground'
              }
            >
              {onlineNodes}
            </span>
            <span className='text-muted-foreground'> / {totalNodes}</span>
          </span>
        </div>

        {loading && nodes.length === 0 ? (
          <div className='space-y-2'>
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-8 w-full' />
          </div>
        ) : previewNodes.length === 0 ? (
          <p className='py-2 text-xs text-muted-foreground'>
            {t('cluster.noNodes')}
          </p>
        ) : (
          <div className='divide-y-[0.5px] divide-border/70'>
            {previewNodes.map((node) => {
              const roleKey = `cluster.roles.${getRoleKey(node.role)}`;
              const roleLabel = t.has(roleKey) ? t(roleKey) : node.role;
              return (
                <div
                  key={node.id}
                  className='flex items-center justify-between gap-2 py-2 text-xs'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='truncate font-medium'>
                      {node.host_name || node.host_ip || `#${node.id}`}
                    </div>
                    <div className='truncate font-mono text-[11px] text-muted-foreground tabular-nums'>
                      {node.host_ip || '-'}
                    </div>
                  </div>
                  <div className='flex shrink-0 items-center gap-1.5'>
                    <NeutralChip>{roleLabel}</NeutralChip>
                    <NodeStatusChip status={node.status} />
                  </div>
                </div>
              );
            })}
          </div>
        )}

        <div>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={() =>
              router.push(`/clusters/${mergedCluster.id}?tab=nodes`)
            }
          >
            {t('cluster.viewAllNodes')}
            <ArrowRight className='size-3.5' aria-hidden />
          </Button>
        </div>
      </section>

      {/* 分组 3：快捷入口 / Section 3: Quick links */}
      <section className='space-y-2.5 pt-4'>
        <h4 className='text-xs font-medium text-muted-foreground'>
          {t('cluster.list.quickLinks')}
        </h4>
        <div className='flex flex-wrap gap-1.5'>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() =>
              router.push(`/clusters/${mergedCluster.id}?tab=configs`)
            }
          >
            <FileCode2 className='size-3.5' aria-hidden />
            {t('cluster.detailTabs.configs')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() =>
              router.push(`/clusters/${mergedCluster.id}?tab=plugins`)
            }
          >
            <Boxes className='size-3.5' aria-hidden />
            {t('cluster.detailTabs.plugins')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() =>
              router.push(
                `/diagnostics?tab=errors&cluster_id=${mergedCluster.id}&source=cluster-list`,
              )
            }
          >
            <Bug className='size-3.5' aria-hidden />
            {t('cluster.openDiagnostics')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() =>
              router.push(`/clusters/${mergedCluster.id}/upgrade/prepare`)
            }
          >
            <Activity className='size-3.5' aria-hidden />
            {t('stUpgrade.entry')}
          </Button>
        </div>
      </section>
    </div>
  );
}
