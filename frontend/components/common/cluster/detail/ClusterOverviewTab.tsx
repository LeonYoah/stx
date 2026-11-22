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
 * 集群详情「概览」Tab：单层扁平画布 + 留白与 0.5px 细线自然分隔规格、节点拓扑与运行时速览。
 * Cluster detail Overview tab: flat single-layer surface separating profile, node topology,
 * and runtime storage/proxy snapshot with whitespace and 0.5px hairlines.
 */
import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {ArrowRight, Check, Copy} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Select} from '@/components/arc/select/select';
import {ConfirmDialog, NeutralChip, SoftChip} from '@/components/common/layout';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';
import {
  NodeRole,
  type ClusterInfo,
  type NodeInfo,
  type RuntimeStorageDetails,
  type StxJavaProxyStatus,
} from '@/lib/services/cluster/types';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {toast} from '@/lib/toast';
import {NodeStatusChip} from '../ClusterStatusChip';
import type {RuntimeSubView} from './cluster-detail-tabs';

export interface ClusterOverviewTabProps {
  cluster: ClusterInfo;
  nodes: NodeInfo[];
  webUINode: NodeInfo | null;
  enableHTTP?: boolean;
  jobLogMode?: string;
  logModeSwitching: boolean;
  runtimeStorage: RuntimeStorageDetails | null;
  stxJavaProxy: StxJavaProxyStatus | null;
  onSwitchJobLogMode: (mode: 'per_job' | 'mixed') => Promise<void> | void;
  onGoNodes: () => void;
  onGoRuntime: (view: RuntimeSubView) => void;
}

function getRoleKey(role: string): string {
  return role === 'master/worker' ? 'masterWorker' : role;
}

export function ClusterOverviewTab({
  cluster,
  nodes,
  webUINode,
  enableHTTP,
  jobLogMode,
  logModeSwitching,
  runtimeStorage,
  stxJavaProxy,
  onSwitchJobLogMode,
  onGoNodes,
  onGoRuntime,
}: ClusterOverviewTabProps) {
  const t = useTranslations();
  const [copiedField, setCopiedField] = useState<'dir' | 'code' | null>(null);
  const [pendingLogMode, setPendingLogMode] = useState<
    'per_job' | 'mixed' | null
  >(null);
  const currentLogMode: 'per_job' | 'mixed' =
    jobLogMode === 'per_job' ? 'per_job' : 'mixed';

  const handleCopy = async (value: string, field: 'dir' | 'code') => {
    try {
      await navigator.clipboard.writeText(value);
      setCopiedField(field);
      toast.success(t('cluster.logCopied'));
      window.setTimeout(() => setCopiedField(null), 1800);
    } catch {
      // 剪贴板不可用时静默忽略 / Ignore clipboard errors
    }
  };

  const hazelcastPort =
    nodes.find((node) => node.hazelcast_port > 0)?.hazelcast_port || '-';
  const httpPortLabel =
    enableHTTP === false
      ? t('cluster.httpDisabled')
      : webUINode?.api_port || '-';
  const masterCount = nodes.filter(
    (n) => n.role === NodeRole.MASTER || n.role === NodeRole.MASTER_WORKER,
  ).length;
  const workerCount = nodes.filter(
    (n) => n.role === NodeRole.WORKER || n.role === NodeRole.MASTER_WORKER,
  ).length;
  const modeKey = `cluster.modes.${cluster.deployment_mode}`;
  const modeLabel = t.has(modeKey) ? t(modeKey) : cluster.deployment_mode;

  return (
    <div
      className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'
      data-testid='cluster-overview-panel'
    >
      {/* 分组 1：集群规格与运行参数（无嵌套小盒）/ Group 1: Cluster profile & runtime params */}
      <section className='space-y-4 pb-5'>
        <div className='space-y-1'>
          <h3 className='text-sm font-semibold text-foreground'>
            {t('cluster.clusterInfo')}
          </h3>
          {cluster.description ? (
            <p className='text-xs text-muted-foreground'>
              {cluster.description}
            </p>
          ) : null}
        </div>

        {/* 单行安装目录与集群编码条，无描边仅轻底色 / Single-row install path bar, subtle fill without border */}
        <div className='grid gap-3 md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]'>
          <div className='min-w-0 space-y-1'>
            <div className='text-xs text-muted-foreground'>
              {t('cluster.installDir')}
            </div>
            <div className='flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2 font-mono text-xs'>
              <span
                className='min-w-0 flex-1 truncate text-foreground'
                title={cluster.install_dir || '-'}
              >
                {cluster.install_dir || '-'}
              </span>
              {cluster.install_dir ? (
                <button
                  type='button'
                  onClick={() => void handleCopy(cluster.install_dir, 'dir')}
                  className='shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground'
                  aria-label={t('common.copy')}
                  title={t('common.copy')}
                >
                  {copiedField === 'dir' ? (
                    <Check className='size-3.5 text-emerald-500' aria-hidden />
                  ) : (
                    <Copy className='size-3.5' aria-hidden />
                  )}
                </button>
              ) : null}
            </div>
          </div>

          <div className='min-w-0 space-y-1'>
            <div className='text-xs text-muted-foreground'>
              {t('cluster.clusterCode')}
            </div>
            <div className='flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2 font-mono text-xs'>
              <span
                className='min-w-0 flex-1 truncate text-foreground'
                title={cluster.cluster_code || '-'}
              >
                {cluster.cluster_code || '-'}
              </span>
              {cluster.cluster_code ? (
                <button
                  type='button'
                  onClick={() =>
                    void handleCopy(cluster.cluster_code || '', 'code')
                  }
                  className='shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground'
                  aria-label={t('common.copy')}
                  title={t('common.copy')}
                >
                  {copiedField === 'code' ? (
                    <Check className='size-3.5 text-emerald-500' aria-hidden />
                  ) : (
                    <Copy className='size-3.5' aria-hidden />
                  )}
                </button>
              ) : null}
            </div>
          </div>
        </div>

        {/* 扁平 KV 网格 / Flat KV grid */}
        <dl className='grid grid-cols-2 gap-x-6 gap-y-4 pt-1 text-xs md:grid-cols-4'>
          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.version')}</dt>
            <dd className='flex flex-wrap items-center gap-1.5 font-mono text-sm font-medium tabular-nums'>
              <span>{cluster.version || '-'}</span>
              <NeutralChip>{modeLabel}</NeutralChip>
            </dd>
          </div>

          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.hazelcastPort')}
            </dt>
            <dd className='font-mono text-sm font-medium tabular-nums'>
              {hazelcastPort}
            </dd>
          </div>

          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.httpPort')}</dt>
            <dd className='font-mono text-sm font-medium tabular-nums'>
              {httpPortLabel}
            </dd>
          </div>

          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>
              {t('cluster.webUiStatus')}
            </dt>
            <dd className='text-sm font-medium'>
              {webUINode
                ? t('common.enabled')
                : isSeatunnelVersionAtLeast(cluster.version, '2.3.9')
                  ? t('cluster.httpDisabled')
                  : t('cluster.versionUnsupported')}
            </dd>
          </div>

          <div className='col-span-2 min-w-0 space-y-1.5'>
            <dt className='text-muted-foreground'>
              {t('cluster.logOutputMode')}
            </dt>
            <dd className='max-w-[280px]'>
              {/* 切换日志模式会改写全集群 log4j2.properties 并需重启生效，使用下拉框配合二次确认防误触。
                  Switching log mode rewrites log4j2.properties cluster-wide and requires a restart; use Select + ConfirmDialog to prevent accidental clicks. */}
              <Select
                aria-label={t('cluster.logOutputMode')}
                data-testid='cluster-overview-log-mode-select'
                value={currentLogMode}
                disabled={logModeSwitching}
                onValueChange={(val) => {
                  if (
                    !logModeSwitching &&
                    (val === 'per_job' || val === 'mixed') &&
                    val !== currentLogMode
                  ) {
                    setPendingLogMode(val);
                  }
                }}
                options={[
                  {
                    value: 'per_job',
                    label: t('config.jobLogModePerJobBadge'),
                  },
                  {
                    value: 'mixed',
                    label: t('config.jobLogModeMixedBadge'),
                  },
                ]}
              />
            </dd>
          </div>

          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.createdAt')}</dt>
            <dd className='font-mono text-xs text-muted-foreground tabular-nums'>
              {formatDateTimeMinuteTz(cluster.created_at)}
            </dd>
          </div>

          <div className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{t('cluster.updatedAt')}</dt>
            <dd className='font-mono text-xs text-muted-foreground tabular-nums'>
              {formatDateTimeMinuteTz(cluster.updated_at)}
            </dd>
          </div>
        </dl>
      </section>

      {/* 分组 2：节点拓扑速览（0.5px 细线分隔，去小卡片边框）/ Group 2: Node topology preview */}
      <section className='space-y-3 border-t-[0.5px] border-border/80 py-5'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-baseline gap-2'>
            <h3 className='text-sm font-semibold text-foreground'>
              {t('cluster.topologyPreview')}
            </h3>
            <span className='text-xs text-muted-foreground tabular-nums'>
              {t('cluster.roles.master')}: {masterCount} ·{' '}
              {t('cluster.roles.worker')}: {workerCount}
            </span>
          </div>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={onGoNodes}
          >
            {t('cluster.viewAllNodes')}
            <ArrowRight className='size-3.5' aria-hidden />
          </Button>
        </div>

        {nodes.length === 0 ? (
          <p className='py-4 text-xs text-muted-foreground'>
            {t('cluster.noNodes')}
          </p>
        ) : (
          <div className='divide-y-[0.5px] divide-border/70'>
            {nodes.map((node) => {
              const roleKey = `cluster.roles.${getRoleKey(node.role)}`;
              const roleLabel = t.has(roleKey) ? t(roleKey) : node.role;
              return (
                <div
                  key={node.id}
                  className='flex flex-wrap items-center justify-between gap-3 py-2.5 text-xs'
                >
                  <div className='flex min-w-0 flex-1 items-center gap-3'>
                    <span className='truncate font-medium text-foreground'>
                      {node.host_name || `#${node.id}`}
                    </span>
                    <span className='truncate font-mono text-muted-foreground tabular-nums'>
                      {node.host_ip || '-'}
                    </span>
                  </div>
                  <div className='flex shrink-0 items-center gap-2.5'>
                    <NeutralChip>{roleLabel}</NeutralChip>
                    <span className='w-20 text-right font-mono text-[11px] text-muted-foreground tabular-nums'>
                      PID {node.process_pid || '-'}
                    </span>
                    <NodeStatusChip status={node.status} />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* 分组 3：运行时存储与 Java Proxy 速览（0.5px 细线分隔）/ Group 3: Runtime storage & proxy */}
      <section className='space-y-3 border-t-[0.5px] border-border/80 pt-5'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='text-sm font-semibold text-foreground'>
            {t('cluster.storageSummary')}
          </h3>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => onGoRuntime('checkpoint')}
          >
            {t('cluster.manageStorage')}
            <ArrowRight className='size-3.5' aria-hidden />
          </Button>
        </div>

        <div className='grid gap-4 md:grid-cols-3 md:gap-0 md:divide-x-[0.5px] md:divide-border/80'>
          <button
            type='button'
            onClick={() => onGoRuntime('checkpoint')}
            className='min-w-0 space-y-1.5 text-left transition-colors hover:opacity-80 md:pr-5'
          >
            <div className='flex items-center justify-between gap-2'>
              <span className='text-xs font-medium text-foreground'>
                {t('installer.checkpointConfig')}
              </span>
              <NeutralChip className='font-mono uppercase'>
                {runtimeStorage?.checkpoint?.storage_type || 'Local'}
              </NeutralChip>
            </div>
            <p
              className='truncate font-mono text-xs text-muted-foreground'
              title={runtimeStorage?.checkpoint?.namespace || '-'}
            >
              {runtimeStorage?.checkpoint?.namespace || '-'}
            </p>
          </button>

          <button
            type='button'
            onClick={() => onGoRuntime('imap')}
            className='min-w-0 space-y-1.5 text-left transition-colors hover:opacity-80 md:px-5'
          >
            <div className='flex items-center justify-between gap-2'>
              <span className='text-xs font-medium text-foreground'>
                {t('installer.runtimeStorage.imapTitle')}
              </span>
              <NeutralChip className='font-mono uppercase'>
                {runtimeStorage?.imap?.storage_type || 'Local'}
              </NeutralChip>
            </div>
            <p
              className='truncate font-mono text-xs text-muted-foreground'
              title={runtimeStorage?.imap?.namespace || '-'}
            >
              {runtimeStorage?.imap?.namespace || '-'}
            </p>
          </button>

          <button
            type='button'
            onClick={() => onGoRuntime('proxy')}
            data-testid='cluster-overview-proxy-summary'
            className='min-w-0 space-y-1.5 text-left transition-colors hover:opacity-80 md:pl-5'
          >
            <div className='flex items-center justify-between gap-2'>
              <span className='text-xs font-medium text-foreground'>
                {t('cluster.stxJavaProxy.title')}
              </span>
              <SoftChip
                tone={
                  stxJavaProxy?.healthy
                    ? 'emerald'
                    : stxJavaProxy?.running
                      ? 'amber'
                      : 'muted'
                }
                className='font-medium'
              >
                {stxJavaProxy?.healthy
                  ? t('cluster.stxJavaProxy.healthy')
                  : stxJavaProxy?.running
                    ? t('cluster.stxJavaProxy.unhealthy')
                    : t('cluster.stxJavaProxy.stopped')}
              </SoftChip>
            </div>
            <p
              className='truncate font-mono text-xs text-muted-foreground tabular-nums'
              title={stxJavaProxy?.endpoint || '-'}
            >
              {stxJavaProxy?.endpoint || '-'}
            </p>
          </button>
        </div>
      </section>

      <ConfirmDialog
        open={pendingLogMode !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingLogMode(null);
          }
        }}
        title={t('config.switchLogModeConfirmTitle')}
        description={
          pendingLogMode
            ? t('config.switchLogModeConfirmDesc', {
                mode:
                  pendingLogMode === 'per_job'
                    ? t('config.jobLogModePerJobBadge')
                    : t('config.jobLogModeMixedBadge'),
              })
            : ''
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={t('config.switchLogModeConfirmAction')}
        loading={logModeSwitching}
        onConfirm={async () => {
          if (!pendingLogMode) {
            return;
          }
          await onSwitchJobLogMode(pendingLogMode);
          setPendingLogMode(null);
        }}
      />
    </div>
  );
}
