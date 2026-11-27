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
 * 集群详情「概览」Tab：双卡平衡布局（规格与节点拓扑 + 运行时组件与运维态势），
 * 用真实健康与观测信号替代底部大块留白，保持 0.5px 细线分隔与高信噪比。
 * Cluster detail Overview tab: balanced two-card layout (profile & nodes + runtime components & ops posture),
 * replacing bottom dead space with real health and observability signals via 0.5px hairlines.
 */
import {useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {Activity, ArrowRight} from 'lucide-react';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Select} from '@/components/arc/select/select';
import {
  ConfirmDialog,
  NeutralChip,
  SoftChip,
  type ChipTone,
} from '@/components/common/layout';
import type {NodeJavaRuntimeView} from '@/components/common/java/NodeJavaHomeCell';
import {isSeatunnelVersionAtLeast} from '@/lib/seatunnel-version';
import {getClusterDeployModeLabel, isK8sInfra} from '@/lib/infra';
import {
  HealthStatus,
  NodeRole,
  type ClusterHealthReason,
  type ClusterInfo,
  type ClusterStatusInfo,
  type K8sPodInfo,
  type NodeInfo,
  type RuntimeStorageDetails,
  type StxJavaProxyStatus,
} from '@/lib/services/cluster/types';
import type {ClusterMonitoringOverviewData} from '@/lib/services/monitoring/types';
import type {
  SyncEventChannelState,
  SyncEventChannelStatus,
} from '@/lib/services/sync/types';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {NodeStatusChip} from '../ClusterStatusChip';
import type {RuntimeSubView} from './cluster-detail-tabs';
import {CopyButton} from '@/components/arc/copy-button/copy-button';

export interface ClusterOverviewTabProps {
  cluster: ClusterInfo;
  clusterStatus?: ClusterStatusInfo | null;
  nodes: NodeInfo[];
  nodeJavaRuntimes?: Record<number, NodeJavaRuntimeView>;
  webUINode: NodeInfo | null;
  enableHTTP?: boolean;
  jobLogMode?: string;
  logModeSwitching: boolean;
  runtimeStorage: RuntimeStorageDetails | null;
  stxJavaProxy: StxJavaProxyStatus | null;
  eventChannel?: SyncEventChannelStatus | null;
  monitoringOverview?: ClusterMonitoringOverviewData | null;
  /** K8s 集群节点概览以 Pod 为真相，物理机 nodes 为空。K8s node overview is pod-driven; bare-metal nodes stay empty. */
  k8sPods?: K8sPodInfo[];
  k8sPodsError?: string | null;
  onRetryK8sPods?: () => void;
  diagnosticsGroupTotal?: number;
  inspectionStarting?: boolean;
  onSwitchJobLogMode: (mode: 'per_job' | 'mixed') => Promise<void> | void;
  onGoNodes: () => void;
  onGoConfigs?: () => void;
  onGoRuntime: (view: RuntimeSubView) => void;
  onGoOps?: () => void;
  onOpenMonitoring?: () => void;
  onOpenDiagnostics?: () => void;
  onStartInspection?: () => void;
}

function getRoleKey(role: string): string {
  return role === 'master/worker' ? 'masterWorker' : role;
}

function eventChannelTone(state?: SyncEventChannelState): ChipTone {
  switch (state) {
    case 'healthy':
      return 'emerald';
    case 'degraded':
      return 'rose';
    case 'pending_restart':
      return 'amber';
    case 'waiting_events':
      return 'blue';
    default:
      return 'muted';
  }
}

function healthChipTone(health?: string): ChipTone {
  switch (health) {
    case HealthStatus.HEALTHY:
      return 'emerald';
    case HealthStatus.UNHEALTHY:
      return 'rose';
    default:
      return 'muted';
  }
}

const CJK_RE = /[\u3400-\u9fff\uf900-\ufaff]/;

// 历史版本后端把同一原因拼成 "EN / 中文"；仅当 " / " 右侧包含中文字符时才判定为双语分隔，
// 避免误切路径或 URL 里的 " / "。
// Legacy backends join one reason as "EN / 中文"; treat " / " as the bilingual separator
// only when the right side contains CJK, so paths/URLs are never split accidentally.
function localizeLegacyHealthReason(reason: string, locale: string): string {
  const idx = reason.indexOf(' / ');
  if (idx < 0) {return reason;}
  const right = reason.slice(idx + 3);
  if (!CJK_RE.test(right)) {return reason;}
  const left = reason.slice(0, idx);
  return locale.startsWith('zh') ? right.trim() : left.trim();
}

export function ClusterOverviewTab({
  cluster,
  clusterStatus,
  nodes,
  nodeJavaRuntimes,
  webUINode,
  enableHTTP,
  jobLogMode,
  logModeSwitching,
  runtimeStorage,
  stxJavaProxy,
  eventChannel,
  monitoringOverview,
  k8sPods,
  k8sPodsError,
  onRetryK8sPods,
  diagnosticsGroupTotal = 0,
  inspectionStarting = false,
  onSwitchJobLogMode,
  onGoNodes,
  onGoConfigs,
  onGoRuntime,
  onGoOps,
  onOpenMonitoring,
  onOpenDiagnostics,
  onStartInspection,
}: ClusterOverviewTabProps) {
  const t = useTranslations();
  const locale = useLocale();
  const [pendingLogMode, setPendingLogMode] = useState<
    'per_job' | 'mixed' | null
  >(null);
  const currentLogMode: 'per_job' | 'mixed' =
    jobLogMode === 'per_job' ? 'per_job' : 'mixed';


  const isK8s = isK8sInfra(cluster.infra_type);
  const deployModeLabel = getClusterDeployModeLabel(cluster.infra_type);
  const k8sCfg = ((cluster.config as Record<string, unknown> | undefined)?.['kubernetes'] || {}) as Record<string, unknown>;
  const hazelcastPort = isK8s
    ? Number(k8sCfg['hazelcast_port']) || '-'
    : nodes.find((node) => node.hazelcast_port > 0)?.hazelcast_port || '-';
  const httpPortLabel = isK8s
    ? Number(k8sCfg['master_http_port']) || '-'
    : enableHTTP === false
      ? t('cluster.httpDisabled')
      : webUINode?.api_port || '-';
  const masterCount = isK8s
    ? (k8sPods || []).filter((pod) => pod.role === 'master').length
    : nodes.filter(
        (n) => n.role === NodeRole.MASTER || n.role === NodeRole.MASTER_WORKER,
      ).length;
  const workerCount = isK8s
    ? (k8sPods || []).filter((pod) => pod.role === 'worker').length
    : nodes.filter(
        (n) => n.role === NodeRole.WORKER || n.role === NodeRole.MASTER_WORKER,
      ).length;
  const modeKey = `cluster.modes.${cluster.deployment_mode}`;
  const modeLabel = t.has(modeKey) ? t(modeKey) : cluster.deployment_mode;

  const effectiveHealth =
    clusterStatus?.health_status ?? cluster.health_status ?? HealthStatus.UNKNOWN;
  const healthReasons = (clusterStatus?.health_reasons ?? []).filter(
    (reason) => Boolean(reason?.trim()),
  );
  const healthReasonDetails = clusterStatus?.health_reason_details ?? [];

  // 结构化原因优先按 code 走 i18n 单语渲染；未知 code 回退同下标旧文案并按 locale 裁剪。
  // Structured reasons render single-language via i18n by code; unknown codes fall back
  // to the legacy string at the same index, cropped to the active locale.
  const formatHealthReasonDetail = (
    detail: ClusterHealthReason,
    legacyReason?: string,
  ): string => {
    const node = detail.node ?? '';
    switch (detail.code) {
      case 'node_offline':
        return t('cluster.overview.healthReasons.nodeOffline', {node});
      case 'process_not_running':
        return t('cluster.overview.healthReasons.processNotRunning', {
          node,
          status: detail.status ?? '',
        });
      case 'member_split_brain':
        return t('cluster.overview.healthReasons.memberSplitBrain', {
          node,
          observed: detail.observed_members ?? 0,
          expected: detail.expected_members ?? 0,
        });
      case 'engine_not_ready':
        return t('cluster.overview.healthReasons.engineNotReady', {
          node,
          detail: detail.detail ?? '',
        });
      default:
        return localizeLegacyHealthReason(
          legacyReason ?? detail.detail ?? detail.code,
          locale,
        );
    }
  };

  const healthItems: string[] =
    healthReasonDetails.length > 0
      ? healthReasonDetails.map((detail, idx) =>
          formatHealthReasonDetail(detail, healthReasons[idx]),
        )
      : healthReasons.map((reason) =>
          localizeLegacyHealthReason(reason, locale),
        );
  const onlineNodes = clusterStatus?.online_nodes ?? cluster.online_nodes ?? 0;
  const totalNodes = clusterStatus?.total_nodes ?? nodes.length;
  const activeAlerts1h = monitoringOverview?.stats.active_alerts_1h ?? 0;
  const crashed24h = monitoringOverview?.stats.crashed_events_24h ?? 0;
  const restartFailed24h =
    monitoringOverview?.stats.restart_failed_events_24h ?? 0;
  const autoMonitorEnabled = monitoringOverview?.monitor_config?.auto_monitor;

  const healthLabelKey = `cluster.healthStatuses.${effectiveHealth}`;
  const healthLabel = t.has(healthLabelKey)
    ? t(healthLabelKey)
    : effectiveHealth;

  const eventChannelStateLabel = eventChannel
    ? t(`cluster.eventChannel.state.${eventChannel.state}`)
    : t('cluster.overview.eventChannelNotConfigured');
  const eventChannelSubtext = !eventChannel
    ? '-'
    : !eventChannel.supported
      ? t('cluster.overview.eventChannelUnsupportedShort')
      : eventChannel.last_event_at
        ? t('cluster.eventChannel.lastEvent', {
            time: formatDateTimeMinuteTz(eventChannel.last_event_at),
          })
        : eventChannel.enabled
          ? eventChannel.report_non_terminal
            ? t('cluster.eventChannel.modeFull')
            : t('cluster.eventChannel.modeTerminal')
          : t('cluster.eventChannel.state.disabled');

  return (
    <div className='space-y-4' data-testid='cluster-overview-panel'>
      {/* 集群不健康时在总览顶部显式列出根因（含引擎半初始化探测结果）/ Surface health reasons at the top when unhealthy */}
      {healthItems.length > 0 ? (
        <ArcAlert
          tone='danger'
          title={t('cluster.overview.healthAlertTitle')}
          data-testid='cluster-overview-health-alert'
        >
          <ul className='mt-1.5 list-disc space-y-1 pl-4 text-xs'>
            {healthItems.map((item, idx) => (
              <li key={idx} className='break-words'>
                {item}
              </li>
            ))}
          </ul>
        </ArcAlert>
      ) : null}

      {/* 卡片 1：集群规格与节点概览 / Card 1: Cluster profile & node snapshot */}
      <div className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'>
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
                {isK8s ? t('cluster.k8s.namespace') : t('cluster.installDir')}
              </div>
              <div className='flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2 font-mono text-xs'>
                <span
                  className='min-w-0 flex-1 truncate text-foreground'
                  title={isK8s ? String(k8sCfg['namespace'] || '-') : cluster.install_dir || '-'}
                >
                  {isK8s ? String(k8sCfg['namespace'] || '-') : cluster.install_dir || '-'}
                </span>
                {isK8s || cluster.install_dir ? (
                  <CopyButton
                    value={isK8s ? String(k8sCfg['namespace'] || '') : cluster.install_dir}
                    label={t('common.copy')}
                    variant='plain'
                    iconOnly
                    className='h-5 w-5 min-h-0 shrink-0'
                  />
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
                  <CopyButton
                    value={cluster.cluster_code}
                    label={t('common.copy')}
                    variant='plain'
                    iconOnly
                    className='h-5 w-5 min-h-0 shrink-0'
                  />
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
                <NeutralChip>{deployModeLabel}</NeutralChip>
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

            {!isK8s ? (
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
            ) : null}

            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.createdAt')}
              </dt>
              <dd className='font-mono text-xs text-muted-foreground tabular-nums'>
                {formatDateTimeMinuteTz(cluster.created_at)}
              </dd>
            </div>

            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.updatedAt')}
              </dt>
              <dd className='font-mono text-xs text-muted-foreground tabular-nums'>
                {formatDateTimeMinuteTz(cluster.updated_at)}
              </dd>
            </div>
          </dl>
        </section>

        {/* 分组 2：节点概览（补齐端口与 JDK 信息，消除宽屏中间真空带）/ Group 2: Node topology preview with ports & JDK */}
        <section className='space-y-3 border-t-[0.5px] border-border/80 pt-5'>
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

          {isK8s ? (
            k8sPodsError ? (
              <div className='flex items-center gap-2 py-4'>
                <p className='text-xs text-destructive'>{k8sPodsError}</p>
                {onRetryK8sPods ? (
                  <button
                    type='button'
                    className='text-xs text-primary underline-offset-2 hover:underline'
                    onClick={onRetryK8sPods}
                    data-testid='k8s-pods-retry'
                  >
                    {t('common.retry')}
                  </button>
                ) : null}
              </div>
            ) : (k8sPods || []).length === 0 ? (
              <p className='py-4 text-xs text-muted-foreground'>
                {t('cluster.noNodes')}
              </p>
            ) : (
              <div className='divide-y-[0.5px] divide-border/70'>
                {(k8sPods || []).map((pod) => (
                  <div
                    key={pod.name}
                    className='grid grid-cols-1 items-center gap-2 py-2.5 text-xs md:grid-cols-[minmax(0,1.2fr)_minmax(0,1.4fr)_auto]'
                  >
                    <div className='flex min-w-0 items-center gap-2.5'>
                      <span className='truncate font-mono font-medium text-foreground'>
                        {pod.name}
                      </span>
                      <span className='truncate font-mono text-muted-foreground tabular-nums'>
                        {pod.pod_ip || '-'}
                      </span>
                    </div>
                    <div className='flex min-w-0 items-center gap-2 font-mono text-[11px] text-muted-foreground'>
                      {pod.role ? (
                        <span className='whitespace-nowrap'>{pod.role}</span>
                      ) : null}
                    </div>
                    <span className='justify-self-end'>
                      {pod.ready ? (
                        <SoftChip tone='emerald'>{t('cluster.k8s.filterReady')}</SoftChip>
                      ) : (
                        <SoftChip tone='rose'>{pod.phase || t('cluster.k8s.filterAbnormal')}</SoftChip>
                      )}
                    </span>
                  </div>
                ))}
              </div>
            )
          ) : nodes.length === 0 ? (
            <p className='py-4 text-xs text-muted-foreground'>
              {t('cluster.noNodes')}
            </p>
          ) : (
            <div className='divide-y-[0.5px] divide-border/70'>
              {nodes.map((node) => {
                const roleKey = `cluster.roles.${getRoleKey(node.role)}`;
                const roleLabel = t.has(roleKey) ? t(roleKey) : node.role;
                const runtime = nodeJavaRuntimes?.[node.id];
                const jdkLabel =
                  runtime?.status === 'ok' && runtime.version
                    ? t('cluster.overview.jdkMajor', {version: runtime.version})
                    : null;
                const portParts: string[] = [];
                if (node.hazelcast_port > 0) {
                  portParts.push(`HZ:${node.hazelcast_port}`);
                }
                if (node.api_port > 0) {
                  portParts.push(`HTTP:${node.api_port}`);
                }
                return (
                  <div
                    key={node.id}
                    className='grid grid-cols-1 items-center gap-2 py-2.5 text-xs md:grid-cols-[minmax(0,1.2fr)_minmax(0,1.4fr)_auto]'
                  >
                    <div className='flex min-w-0 items-center gap-2.5'>
                      <span className='truncate font-medium text-foreground'>
                        {node.host_name || `#${node.id}`}
                      </span>
                      <span className='truncate font-mono text-muted-foreground tabular-nums'>
                        {node.host_ip || '-'}
                      </span>
                    </div>

                    <div className='flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground tabular-nums'>
                      {portParts.length > 0 ? (
                        <span className='whitespace-nowrap'>
                          {portParts.join(' · ')}
                        </span>
                      ) : null}
                      {jdkLabel ? (
                        <span
                          className='whitespace-nowrap'
                          title={
                            runtime?.status === 'ok'
                              ? runtime.java_home || node.java_home
                              : node.java_home
                          }
                        >
                          {jdkLabel}
                        </span>
                      ) : null}
                    </div>

                    <div className='flex shrink-0 items-center gap-2.5 md:justify-end'>
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
      </div>

      {/* 卡片 2：运行时组件与运维态势速览 / Card 2: Runtime components & ops posture */}
      <div className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'>
        {/* 分组 1：4 项运行时组件（2×2 均衡网格，含事件通道状态）/ Group 1: 4 runtime components (2x2 balanced grid) */}
        <section className='space-y-3 pb-5'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <h3 className='text-sm font-semibold text-foreground'>
              {t('cluster.overview.runtimeAndOps')}
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

          <div className='grid gap-3 md:grid-cols-2'>
            <button
              type='button'
              onClick={() => onGoRuntime('checkpoint')}
              className='min-w-0 space-y-1.5 rounded-lg bg-muted/35 px-4 py-3 text-left transition-colors hover:bg-muted/55'
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
              className='min-w-0 space-y-1.5 rounded-lg bg-muted/35 px-4 py-3 text-left transition-colors hover:bg-muted/55'
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
              className='min-w-0 space-y-1.5 rounded-lg bg-muted/35 px-4 py-3 text-left transition-colors hover:bg-muted/55'
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

            <button
              type='button'
              onClick={() => onGoConfigs?.()}
              data-testid='cluster-overview-event-channel-summary'
              className='min-w-0 space-y-1.5 rounded-lg bg-muted/35 px-4 py-3 text-left transition-colors hover:bg-muted/55'
            >
              <div className='flex items-center justify-between gap-2'>
                <span className='text-xs font-medium text-foreground'>
                  {t('cluster.eventChannel.title')}
                </span>
                <SoftChip tone={eventChannelTone(eventChannel?.state)}>
                  {eventChannelStateLabel}
                </SoftChip>
              </div>
              <p
                className='truncate font-mono text-xs text-muted-foreground tabular-nums'
                title={eventChannelSubtext}
              >
                {eventChannelSubtext}
              </p>
            </button>
          </div>
        </section>

        {/* 分组 2：运维与观测态势摘要（复用已有监控/诊断状态，无额外空装饰）/ Group 2: Ops & observability posture */}
        <section className='space-y-3 border-t-[0.5px] border-border/80 pt-5'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <h3 className='text-sm font-semibold text-foreground'>
              {t('cluster.overview.opsStatus')}
            </h3>
            <div className='flex flex-wrap items-center gap-1.5'>
              {onStartInspection ? (
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  loading={inspectionStarting}
                  onClick={onStartInspection}
                >
                  <Activity className='size-3.5' aria-hidden />
                  {t('cluster.overview.startInspection')}
                </Button>
              ) : null}
              {onGoOps ? (
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={onGoOps}
                >
                  {t('cluster.overview.openOpsTab')}
                  <ArrowRight className='size-3.5' aria-hidden />
                </Button>
              ) : null}
            </div>
          </div>

          <dl className='grid grid-cols-2 gap-x-6 gap-y-4 text-xs md:grid-cols-4'>
            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.overview.healthSummary')}
              </dt>
              <dd className='flex flex-wrap items-center gap-2'>
                <SoftChip tone={healthChipTone(effectiveHealth)}>
                  {healthLabel}
                </SoftChip>
                <span className='font-mono text-[11px] text-muted-foreground tabular-nums'>
                  {t('cluster.overview.nodesOnlineRatio', {
                    online: onlineNodes,
                    total: totalNodes,
                  })}
                </span>
              </dd>
            </div>

            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.overview.activeAlerts1h')}
              </dt>
              <dd className='flex items-baseline gap-2 tabular-nums'>
                <span
                  className={`font-mono text-sm font-semibold ${
                    activeAlerts1h > 0
                      ? 'text-rose-500 dark:text-rose-400'
                      : 'text-foreground'
                  }`}
                >
                  {activeAlerts1h}
                </span>
                {onOpenMonitoring ? (
                  <button
                    type='button'
                    onClick={onOpenMonitoring}
                    className='text-[11px] text-muted-foreground transition-colors hover:text-foreground'
                  >
                    {t('cluster.overview.viewAlerts')}
                  </button>
                ) : null}
              </dd>
            </div>

            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.overview.errorGroups')}
              </dt>
              <dd className='flex items-baseline gap-2 tabular-nums'>
                <span
                  className={`font-mono text-sm font-semibold ${
                    diagnosticsGroupTotal > 0
                      ? 'text-amber-500 dark:text-amber-400'
                      : 'text-foreground'
                  }`}
                >
                  {diagnosticsGroupTotal}
                </span>
                {onOpenDiagnostics ? (
                  <button
                    type='button'
                    onClick={onOpenDiagnostics}
                    className='text-[11px] text-muted-foreground transition-colors hover:text-foreground'
                  >
                    {t('cluster.overview.viewErrors')}
                  </button>
                ) : null}
              </dd>
            </div>

            <div className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>
                {t('cluster.overview.autoMonitor')}
              </dt>
              <dd className='space-y-0.5'>
                <div className='text-xs font-medium text-foreground'>
                  {autoMonitorEnabled === undefined
                    ? '-'
                    : autoMonitorEnabled
                      ? t('cluster.overview.autoMonitorEnabled')
                      : t('cluster.overview.autoMonitorDisabled')}
                </div>
                <div className='truncate font-mono text-[11px] text-muted-foreground tabular-nums'>
                  {t('cluster.overview.events24h', {
                    crashed: crashed24h,
                    failed: restartFailed24h,
                  })}
                </div>
              </dd>
            </div>
          </dl>
        </section>
      </div>

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
