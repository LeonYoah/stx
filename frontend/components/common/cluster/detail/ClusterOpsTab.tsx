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
 * 集群详情「运维」Tab：以 Arc SegmentedControl 承载「守护与自愈 | 诊断异常 | 升级记录」三视图，
 * 单层扁平画布 + 0.5px 细线分隔。
 * Cluster detail Operations tab: Arc SegmentedControl switching Guard & Auto-Restart,
 * Diagnostics, and Upgrade Records in flat single-layer 0.5px hairline surfaces.
 */
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Activity, ArrowUpRight, Bug} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {SoftChip, TableCard} from '@/components/common/layout';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type {DiagnosticsErrorGroup} from '@/lib/services/diagnostics';
import type {UpgradeTaskSummary} from '@/lib/services/st-upgrade';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {MonitorConfigPanel} from '../MonitorConfigPanel';
import {getExecutionStatusLabel, upgradeStatusTone} from '../upgrade/utils';
import type {OpsSubView} from './cluster-detail-tabs';

export interface ClusterOpsTabProps {
  clusterId: number;
  clusterName: string;
  view: OpsSubView;
  onViewChange: (view: OpsSubView) => void;
  onGuardDirtyChange: (dirty: boolean) => void;
  // 诊断摘要 / Diagnostics summary
  diagnosticsGroups: DiagnosticsErrorGroup[];
  diagnosticsGroupTotal: number;
  diagnosticsLoading: boolean;
  inspectionStarting: boolean;
  onStartInspection: () => void;
  // 升级记录 / Upgrade records
  upgradeTasks: UpgradeTaskSummary[];
  upgradeTasksLoading: boolean;
  upgradeTasksPage: number;
  upgradeTasksPageSize: number;
  upgradeTasksTotal: number;
  onUpgradeTasksPageChange: (page: number) => void;
  onOpenUpgradeTask: (task: UpgradeTaskSummary) => void;
  /** K8s 集群升级走镜像 Tag 替换而非二进制升级向导。K8s upgrades swap the image tag instead of the binary upgrade wizard. */
  isKubernetes?: boolean;
  onOpenK8sUpgrade?: () => void;
}

export function ClusterOpsTab({
  clusterId,
  clusterName,
  view,
  onViewChange,
  onGuardDirtyChange,
  diagnosticsGroups,
  diagnosticsGroupTotal,
  diagnosticsLoading,
  inspectionStarting,
  onStartInspection,
  upgradeTasks,
  upgradeTasksLoading,
  upgradeTasksPage,
  upgradeTasksPageSize,
  upgradeTasksTotal,
  onUpgradeTasksPageChange,
  onOpenUpgradeTask,
  isKubernetes = false,
  onOpenK8sUpgrade,
}: ClusterOpsTabProps) {
  const openUpgradeEntry = () => {
    if (isKubernetes) {
      onOpenK8sUpgrade?.();
    } else {
      router.push(`/clusters/${clusterId}/upgrade/prepare`);
    }
  };
  const t = useTranslations();
  const router = useRouter();
  const upgradeTotalPages = Math.max(
    1,
    Math.ceil(upgradeTasksTotal / upgradeTasksPageSize),
  );

  return (
    <div className='space-y-4' data-testid='cluster-ops-panel'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <SegmentedControl
          label={t('cluster.list.opsViews.label')}
          value={view}
          onValueChange={(v) => onViewChange(v as OpsSubView)}
          options={[
            // K8s 无主机 Agent，进程自愈由 kubelet 原生托管，「守护与自愈」配置只会空转误导。
            // Kubernetes has no host agent; self-healing is kubelet-native so the guard
            // view would idle and mislead, hence it is hidden.
            ...(isKubernetes
              ? []
              : [{value: 'guard', label: t('cluster.list.opsViews.guard')}]),
            {
              value: 'diagnostics',
              label: t('cluster.list.opsViews.diagnostics'),
            },
            {value: 'upgrades', label: t('cluster.list.opsViews.upgrades')},
          ]}
        />

        {view === 'upgrades' ? (
          <Button
            type='button'
            size='sm'
            onClick={openUpgradeEntry}
            data-testid='cluster-ops-upgrade-entry'
          >
            <Activity className='size-3.5' aria-hidden />
            {t('stUpgrade.entry')}
          </Button>
        ) : null}
      </div>

      {view === 'guard' && !isKubernetes ? (
        <MonitorConfigPanel
          clusterId={clusterId}
          clusterName={clusterName}
          onDirtyChange={onGuardDirtyChange}
        />
      ) : null}

      {view === 'diagnostics' ? (
        <div
          className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'
          data-testid='cluster-ops-diagnostics'
        >
          <div className='flex flex-wrap items-start justify-between gap-3 pb-4'>
            <div className='min-w-0 space-y-1'>
              <div className='flex items-center gap-2'>
                <h3 className='text-sm font-semibold text-foreground'>
                  {t('diagnosticsCenter.errors.title')}
                </h3>
                <Badge tone='neutral' outline size='sm'>
                  <span className='tabular-nums'>
                    {t('diagnosticsCenter.errors.matchedGroups', {
                      count: diagnosticsGroupTotal,
                    })}
                  </span>
                </Badge>
              </div>
              <p className='text-xs text-muted-foreground'>
                {t('diagnosticsCenter.errors.clusterScopedHint', {
                  name: clusterName,
                })}
              </p>
            </div>
            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              <Button
                type='button'
                size='sm'
                onClick={onStartInspection}
                loading={inspectionStarting}
              >
                <Activity className='size-3.5' aria-hidden />
                {t('diagnosticsCenter.inspections.startInspection')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={() =>
                  router.push(
                    `/diagnostics?tab=errors&cluster_id=${clusterId}&source=cluster-detail-summary`,
                  )
                }
              >
                <Bug className='size-3.5' aria-hidden />
                {t('cluster.openDiagnostics')}
              </Button>
            </div>
          </div>

          <div className='border-t-[0.5px] border-border/80 pt-3'>
            {diagnosticsLoading ? (
              <div className='space-y-2 py-2'>
                <Skeleton className='h-12 w-full' />
                <Skeleton className='h-12 w-full' />
                <Skeleton className='h-12 w-full' />
              </div>
            ) : diagnosticsGroups.length === 0 ? (
              <EmptyState
                icon={<Bug className='size-6' aria-hidden />}
                title={t('diagnosticsCenter.errors.empty')}
                description={t('diagnosticsCenter.errors.clusterScopedHint', {
                  name: clusterName,
                })}
                action={
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    onClick={onStartInspection}
                    loading={inspectionStarting}
                  >
                    <Activity className='size-3.5' aria-hidden />
                    {t('diagnosticsCenter.inspections.startInspection')}
                  </Button>
                }
              />
            ) : (
              <div className='divide-y-[0.5px] divide-border/70'>
                {diagnosticsGroups.map((group) => (
                  <button
                    key={group.id}
                    type='button'
                    className='flex w-full items-start justify-between gap-4 py-3 text-left transition-colors hover:bg-muted/30'
                    onClick={() =>
                      router.push(
                        `/diagnostics?tab=errors&cluster_id=${clusterId}&group_id=${group.id}&source=cluster-detail-summary`,
                      )
                    }
                  >
                    <div className='min-w-0 flex-1 space-y-0.5'>
                      <div className='truncate text-sm font-medium text-foreground'>
                        {group.title || group.sample_message || group.fingerprint}
                      </div>
                      <div className='truncate font-mono text-xs text-muted-foreground'>
                        {group.exception_class || group.sample_message || '-'}
                      </div>
                    </div>
                    <div className='flex shrink-0 items-center gap-2'>
                      <SoftChip
                        tone={
                          group.occurrence_count >= 10
                            ? 'rose'
                            : group.occurrence_count >= 3
                              ? 'amber'
                              : 'muted'
                        }
                        className='tabular-nums'
                      >
                        {group.occurrence_count}
                      </SoftChip>
                      <ArrowUpRight
                        className='size-3.5 text-muted-foreground'
                        aria-hidden
                      />
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      ) : null}

      {view === 'upgrades' ? (
        <TableCard
          loading={upgradeTasksLoading}
          hasData={upgradeTasks.length > 0}
          skeletonColumns={8}
          skeletonRows={5}
          pagination={
            upgradeTasksTotal > 0
              ? {
                  currentPage: upgradeTasksPage,
                  totalPages: upgradeTotalPages,
                  pageSize: upgradeTasksPageSize,
                  totalItems: upgradeTasksTotal,
                  onPageChange: onUpgradeTasksPageChange,
                }
              : undefined
          }
          empty={
            <EmptyState
              icon={<Activity className='size-6' aria-hidden />}
              title={t('stUpgrade.noUpgradeRecords')}
              description={t('stUpgrade.prepareDescription')}
              action={
                <Button
                  type='button'
                  size='sm'
                  onClick={() =>
                    openUpgradeEntry()
                  }
                >
                  <Activity className='size-3.5' aria-hidden />
                  {t('stUpgrade.entry')}
                </Button>
              }
            />
          }
        >
          <div className='overflow-x-auto'>
            <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('stUpgrade.sourceVersion')}</TableHead>
                  <TableHead>{t('stUpgrade.targetVersion')}</TableHead>
                  <TableHead>{t('stUpgrade.taskStatus')}</TableHead>
                  <TableHead>{t('stUpgrade.rollbackStatus')}</TableHead>
                  <TableHead>{t('stUpgrade.currentStep')}</TableHead>
                  <TableHead>{t('stUpgrade.createdAt')}</TableHead>
                  <TableHead className='text-right'>
                    {t('common.actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {upgradeTasks.map((upgradeTask) => (
                  <TableRow
                    key={upgradeTask.id}
                    className='cursor-pointer hover:bg-muted/25'
                    onClick={() => onOpenUpgradeTask(upgradeTask)}
                  >
                    <TableCell className='font-mono text-xs tabular-nums'>
                      #{upgradeTask.id}
                    </TableCell>
                    <TableCell className='font-mono text-xs tabular-nums'>
                      {upgradeTask.source_version || '-'}
                    </TableCell>
                    <TableCell className='font-mono text-xs tabular-nums'>
                      {upgradeTask.target_version || '-'}
                    </TableCell>
                    <TableCell>
                      <Badge
                        tone={upgradeStatusTone(upgradeTask.status)}
                        outline
                        dot
                        size='sm'
                      >
                        {getExecutionStatusLabel(upgradeTask.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge
                        tone={upgradeStatusTone(upgradeTask.rollback_status)}
                        outline
                        dot
                        size='sm'
                      >
                        {getExecutionStatusLabel(upgradeTask.rollback_status)}
                      </Badge>
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {upgradeTask.current_step || '-'}
                    </TableCell>
                    <TableCell className='text-xs text-muted-foreground tabular-nums'>
                      {formatDateTimeMinuteTz(upgradeTask.created_at)}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={(event) => {
                          event.stopPropagation();
                          onOpenUpgradeTask(upgradeTask);
                        }}
                      >
                        {t('stUpgrade.viewUpgradeDetail')}
                        <ArrowUpRight className='size-3.5' aria-hidden />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </TableCard>
      ) : null}
    </div>
  );
}
