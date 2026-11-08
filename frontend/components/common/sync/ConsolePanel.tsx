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

import {useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  ExternalLink,
  FileText,
  Maximize2,
  PanelRightOpen,
} from 'lucide-react';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Badge} from '@/components/ui/badge';
import {Skeleton} from '@/components/ui/skeleton';
import {Button} from '@/components/ui/button';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {SyncJobInstance, SyncJobLogsResult} from '@/lib/services/sync';
import {cn} from '@/lib/utils';
import {TableLoadingBar} from '@/components/common/layout';
import {JobOutcomeHints} from './JobOutcomeHints';
import {
  StudioSortableColumnHeader,
  StudioTable,
  StudioTableBody,
  StudioTableCell,
  StudioTableColumn,
  StudioTableContent,
  StudioTableEmptyRow,
  StudioTableFooter,
  StudioTableHeader,
  StudioTableRow,
  StudioTableScroll,
  applyStudioSort,
  nextStudioSort,
  type StudioSortDescriptor,
} from './StudioDataTable';
import {
  buildDisplayLogLines,
  extractJobMetricSummary,
  formatJobDateTime,
  formatJobDuration,
  formatMetricValue,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getEngineEndpointLabel,
  formatJobSyncLagHint,
  getJobStatusBadgeClass,
  getJobActionState,
  getJobStatusLabel,
  getLogLineClass,
  getRunModeLabel,
  getSyncJobClusterId,
  submitSpecExecutionMode,
  type LogFilterMode,
} from './sync-studio-utils';
import {formatTimeTz} from '@/lib/timezone';
import type {JobDetailTab} from './job-detail/types';

/**
 * 混合日志模式提示卡片
 * Mixed log mode notification banner
 */
export function MixedLogModeBanner({
  clusterId,
}: {
  clusterId?: number | null;
  onSwitched?: () => void;
}) {
  const t = useTranslations('workbenchStudio');

  return (
    <div className='flex h-full min-h-[180px] flex-col items-center justify-center p-6 text-center'>
      <div className='max-w-lg space-y-3 rounded-xl border border-amber-500/30 bg-amber-500/5 p-5 text-left shadow-sm'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-2 font-medium text-amber-600 dark:text-amber-400 text-sm'>
            <AlertTriangle className='size-4 shrink-0' />
            <span>{t('mixedLogModeTitle')}</span>
          </div>
          <Badge
            variant='outline'
            className='border-amber-500/30 bg-amber-500/10 text-[11px] text-amber-600 dark:text-amber-400'
          >
            {t('mixedLogModeBadge')}
          </Badge>
        </div>
        <p className='text-xs text-muted-foreground leading-relaxed'>
          {t('mixedLogModeDescription')}
        </p>
        {clusterId ? (
          <div className='pt-1'>
            <Button size='sm' variant='outline' className='h-8 text-xs' asChild>
              <a
                href={`/clusters/${clusterId}`}
                target='_blank'
                rel='noreferrer'
              >
                <ExternalLink className='mr-1.5 size-3.5' />
                {t('viewClusterDetail')}
              </a>
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

/**
 * 暂无日志输出时的诊断引导卡片
 * Empty logs diagnostic banner
 */
export function JobLogsEmptyBanner({
  clusterId,
}: {
  clusterId?: number | null;
}) {
  const t = useTranslations('workbenchStudio');

  return (
    <div className='flex h-full min-h-[180px] flex-col items-center justify-center p-6 text-center'>
      <div className='max-w-lg space-y-3 rounded-xl border border-border/60 bg-muted/20 p-5 text-left shadow-sm'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-2 font-medium text-foreground text-sm'>
            <FileText className='size-4 shrink-0 text-muted-foreground' />
            <span>{t('noLogsTitle')}</span>
          </div>
          <Badge
            variant='outline'
            className='border-border/60 bg-background/50 text-[11px] text-muted-foreground'
          >
            {t('noLogsBadge')}
          </Badge>
        </div>
        <p className='text-xs text-muted-foreground leading-relaxed'>
          {t('noLogsGuideHint')}
        </p>
        <div className='pt-1 flex items-center gap-2'>
          <Button size='sm' variant='outline' className='h-8 text-xs' asChild>
            <a
              href={clusterId ? `/clusters/${clusterId}` : '/clusters'}
              target='_blank'
              rel='noreferrer'
            >
              <ExternalLink className='mr-1.5 size-3.5' />
              {t('switchLogModeInCluster')}
            </a>
          </Button>
        </div>
      </div>
    </div>
  );
}

export function ConsolePanel({
  job,
  logsResult,
  loading,
  filterMode,
  onFilterChange,
  onExpand,
}: {
  job: SyncJobInstance | null;
  logsResult: SyncJobLogsResult | null;
  loading: boolean;
  filterMode: LogFilterMode;
  onFilterChange: (mode: LogFilterMode) => void;
  onExpand: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  if (!job) {
    return <div className='text-sm text-muted-foreground'>{t('noLogs')}</div>;
  }
  const displayStatus = getDisplayJobLifecycleStatus(job);
  const combinedLogs = [logsResult?.logs || '', logsResult?.metrics_log || '']
    .filter((part) => part.trim() !== '')
    .join("\n");
  const renderedLines = buildDisplayLogLines(combinedLogs, 800);
  const isMixedLogMode =
    logsResult?.empty_reason === 'mixed_log_mode' ||
    logsResult?.cluster_job_log_mode === 'mixed';
  const clusterId = logsResult?.cluster_id || getSyncJobClusterId(job);

  return (
    <div className='studio-sidebar-chrome flex h-full min-h-0 min-w-0 flex-col gap-2'>
      <div className='flex flex-wrap items-center gap-2 rounded-lg border border-border/60 bg-background/70 px-3 py-2 text-xs'>
        <Badge variant='outline'>#{job.id}</Badge>
        <Badge variant='outline'>{job.run_type}</Badge>
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm border px-2 py-0.5 text-[11px]',
            getJobStatusBadgeClass(displayStatus),
          )}
        >
          {getJobStatusLabel(displayStatus)}
        </Badge>
        <JobOutcomeHints job={job} />
        {formatJobSyncLagHint(job) ? (
          <Badge variant='outline' className='text-amber-600 dark:text-amber-400'>
            {formatJobSyncLagHint(job)}
          </Badge>
        ) : null}
        <Badge variant='outline'>
          {getEngineAPIMode(job) === 'v1'
            ? 'Legacy REST V1'
            : submitSpecExecutionMode(job.submit_spec) === 'local'
              ? 'Local Agent'
              : 'REST V2'}
        </Badge>
        <span className='min-w-0 flex-1 truncate text-muted-foreground'>
          {getEngineEndpointLabel(job)}
        </span>
        {/* 有结果后固定显示更新时间，自动刷新时不再在“加载中”和时间之间来回跳。 */}
        {/* Once a result exists keep showing its update time instead of flipping to "loading" on every refresh. */}
        <span className='text-muted-foreground'>
          {logsResult?.updated_at
            ? formatTimeTz(logsResult.updated_at)
            : loading
              ? t('loading')
              : '-'}
        </span>
      </div>
      <div className='relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/60 bg-background/70'>
        <TableLoadingBar loading={loading && logsResult !== null} />
        <div className='sticky top-0 z-10 shrink-0 border-b border-border/60 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85'>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-2 text-xs text-muted-foreground'>
            <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
              <span className='shrink-0'>{t('liveLogs')}</span>
              {job.error_message ? (
                <Badge
                  className='rounded-sm border-red-500/30 bg-red-500/10 text-[10px] text-red-600 dark:text-red-400'
                  variant='outline'
                >
                  {t('hasErrors')}
                </Badge>
              ) : null}
            </div>
            <div className='flex items-center justify-self-end gap-2 whitespace-nowrap'>
              <SegmentedControl
                label={t('logs')}
                value={filterMode}
                onValueChange={(value) =>
                  onFilterChange(value as LogFilterMode)
                }
                options={[
                  {value: 'all', label: t('all')},
                  {value: 'warn', label: 'WARN'},
                  {value: 'error', label: 'ERROR'},
                ]}
              />
              <Button
                size='sm'
                variant='ghost'
                className='h-9 rounded-[var(--radius-control)] px-2 text-xs'
                onClick={onExpand}
              >
                <Maximize2 className='mr-1 size-3.5' />
                {t('expand')}
              </Button>
            </div>
          </div>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-border/60 px-3 py-2 text-[11px] text-muted-foreground'>
            <div className='flex min-w-0 items-center gap-3 overflow-hidden'>
              <span className='truncate'>
                {t('jobId')}: {job.platform_job_id || job.engine_job_id || '-'}
              </span>
              {job.engine_job_id &&
              job.platform_job_id &&
              job.engine_job_id !== job.platform_job_id ? (
                <span className='truncate'>
                  {t('engineJobId')}: {job.engine_job_id}
                </span>
              ) : null}
            </div>
            <span className='justify-self-end whitespace-nowrap'>
              {t('logFocusHint')}
            </span>
          </div>
        </div>
        <div className='min-h-0 min-w-0 flex-1 overflow-auto p-3 font-mono text-xs'>
          {renderedLines.length > 0 ? (
            renderedLines.map((line, index) => (
              <div
                key={`${index}-${line.slice(0, 24)}`}
                className={cn(
                  'max-w-full whitespace-pre-wrap break-all',
                  getLogLineClass(line),
                )}
              >
                {line}
              </div>
            ))
          ) : logsResult === null ? (
            // 还没拿到第一份结果：显示骨架，不能先给出“暂无日志 / 切换日志模式”的结论。
            // No result yet: show a skeleton rather than concluding "no logs / switch log mode" prematurely.
            <div className='space-y-2' data-testid='sync-job-logs-skeleton'>
              {Array.from({length: 8}).map((_, index) => (
                <Skeleton key={index} className='h-3.5' style={{width: `${92 - ((index * 17) % 40)}%`}} />
              ))}
            </div>
          ) : isMixedLogMode ? (
            <MixedLogModeBanner clusterId={clusterId} />
          ) : (
            <JobLogsEmptyBanner clusterId={clusterId} />
          )}
        </div>
      </div>
    </div>
  );
}

type JobSortKey = 'id' | 'status' | 'started' | 'metrics';

function resolveJobInitiatorName(
  job: SyncJobInstance,
  currentUserId?: number,
  currentUsername?: string,
  workspaceUsers: NotificationRecipientUser[] = [],
): string {
  if (!job.created_by) {
    return '-';
  }
  if (currentUserId && job.created_by === currentUserId) {
    return currentUsername || 'admin';
  }
  const matched = workspaceUsers.find((user) => user.id === job.created_by);
  if (matched?.username) {
    return matched.username;
  }
  if (matched?.nickname) {
    return matched.nickname;
  }
  if (job.created_by === 1) {
    return 'admin';
  }
  return `User #${job.created_by}`;
}

export function JobRunsPanel({
  jobs,
  selectedJobId,
  currentUserId,
  currentUsername,
  isAdmin = false,
  isOwner = true,
  canRun = true,
  workspaceUsers = [],
  onSelectJob,
  onRecover,
  onCancel,
  onSavepointStop,
  onOpenDetail,
  disableRecover,
}: {
  jobs: SyncJobInstance[];
  selectedJobId: number | null;
  currentUserId?: number;
  currentUsername?: string;
  isAdmin?: boolean;
  isOwner?: boolean;
  canRun?: boolean;
  workspaceUsers?: NotificationRecipientUser[];
  onSelectJob: (jobId: number) => void;
  onRecover: (jobId: number) => void;
  onCancel: (jobId: number) => void;
  onSavepointStop: (jobId: number) => void;
  /** 打开作业详情，可指定初始页签。/ Open the job detail, optionally on a given tab. */
  onOpenDetail: (job: SyncJobInstance, tab?: JobDetailTab) => void;
  disableRecover: boolean;
}) {
  const t = useTranslations('workbenchStudio');
  const [sort, setSort] = useState<StudioSortDescriptor<JobSortKey> | null>(
    null,
  );
  const sortedJobs = useMemo(
    () =>
      applyStudioSort(jobs, sort, (job, key) => {
        if (key === 'id') {
          return job.id;
        }
        if (key === 'status') {
          return getDisplayJobLifecycleStatus(job);
        }
        if (key === 'started') {
          return job.started_at || job.created_at;
        }
        const summary = extractJobMetricSummary(job);
        return summary.readCount ?? summary.writeCount ?? 0;
      }),
    [jobs, sort],
  );

  const renderSortable = (
    key: JobSortKey,
    label: string,
    initial: 'asc' | 'desc' = 'asc',
  ) => (
    <StudioSortableColumnHeader
      label={label}
      sortDirection={sort?.key === key ? sort.direction : undefined}
      aria-label={t('sortColumn', {column: label})}
      onSort={() => setSort((current) => nextStudioSort(current, key, initial))}
    />
  );

  return (
    <StudioTable label={t('jobs')}>
      <StudioTableScroll>
        <StudioTableContent minWidth={840}>
          <StudioTableHeader>
            <StudioTableColumn allowsSorting sortDirection={sort?.key === 'id' ? sort.direction : undefined}>
              {renderSortable('id', t('task'))}
            </StudioTableColumn>
            <StudioTableColumn allowsSorting sortDirection={sort?.key === 'status' ? sort.direction : undefined}>
              {renderSortable('status', t('status'))}
            </StudioTableColumn>
            <StudioTableColumn>{t('jobModeChannel')}</StudioTableColumn>
            <StudioTableColumn>{t('initiator')}</StudioTableColumn>
            <StudioTableColumn allowsSorting sortDirection={sort?.key === 'started' ? sort.direction : undefined}>
              {renderSortable('started', t('jobTimeline'), 'desc')}
            </StudioTableColumn>
            <StudioTableColumn allowsSorting sortDirection={sort?.key === 'metrics' ? sort.direction : undefined}>
              {renderSortable('metrics', t('metrics'), 'desc')}
            </StudioTableColumn>
            <StudioTableColumn align='end'>{t('actions')}</StudioTableColumn>
          </StudioTableHeader>
          <StudioTableBody>
            {sortedJobs.length === 0 ? (
              <StudioTableEmptyRow colSpan={7}>{t('noJobRuns')}</StudioTableEmptyRow>
            ) : (
              sortedJobs.map((job) => {
                const summary = extractJobMetricSummary(job);
                const displayStatus = getDisplayJobLifecycleStatus(job);
                const startedStr = formatJobDateTime(job.started_at);
                const finishedStr = formatJobDateTime(job.finished_at);
                const durationStr = formatJobDuration(
                  job.started_at,
                  job.finished_at,
                );
                const actions = getJobActionState(job, {
                  isAdmin,
                  isOwner,
                  canRun,
                  currentUserId,
                  disableRecover,
                });
                const actionButtonClass =
                  'h-7 gap-1 px-2 text-xs text-muted-foreground hover:bg-muted/70 hover:text-foreground';

                return (
                  <StudioTableRow
                    key={job.id}
                    selected={selectedJobId === job.id}
                    className='cursor-pointer'
                    onClick={() => {
                      // 点击行任意处：选中作业（底部日志 / Checkpoint 跟随）并打开详情。
                      // Clicking anywhere on the row selects the job (bottom logs / checkpoint follow) and opens its detail.
                      onSelectJob(job.id);
                      onOpenDetail(job);
                    }}
                  >
                    <StudioTableCell>
                      <div className='flex items-center gap-1.5'>
                        <span className='text-xs font-medium text-foreground'>
                          #{job.id}
                        </span>
                        {job.platform_job_id ? (
                          <span
                            className='max-w-[100px] truncate font-mono text-[10px] text-muted-foreground/70'
                            title={job.platform_job_id}
                          >
                            {job.platform_job_id}
                          </span>
                        ) : null}
                      </div>
                    </StudioTableCell>
                    <StudioTableCell>
                      <Badge
                        variant='outline'
                        className={cn(
                          'rounded-sm border px-2 py-0 text-[11px] leading-tight',
                          getJobStatusBadgeClass(displayStatus),
                        )}
                      >
                        {getJobStatusLabel(displayStatus)}
                      </Badge>
                      <JobOutcomeHints job={job} className='ml-1 align-middle' />
                    </StudioTableCell>
                    <StudioTableCell>
                      <div className='flex items-center gap-1'>
                        <Badge
                          variant='outline'
                          className='rounded-sm px-1 py-0 text-[10px]'
                          title={job.external_ref || undefined}
                        >
                          {getRunModeLabel(job, t)}
                        </Badge>
                        {job.task_version > 0 ? (
                          <span className='font-mono text-[10px] text-muted-foreground'>
                            v{job.task_version}
                          </span>
                        ) : null}
                        <span className='font-mono text-[10px] text-muted-foreground'>
                          {submitSpecExecutionMode(job.submit_spec) === 'local'
                            ? 'Local'
                            : getEngineAPIMode(job) === 'v1'
                              ? 'V1'
                              : 'REST V2'}
                        </span>
                      </div>
                    </StudioTableCell>
                    <StudioTableCell>
                      <div className='flex items-center gap-1'>
                        <span className='text-[11px] font-medium text-foreground'>
                          {resolveJobInitiatorName(
                            job,
                            currentUserId,
                            currentUsername,
                            workspaceUsers,
                          )}
                        </span>
                        {currentUserId && job.created_by === currentUserId ? (
                          <Badge
                            variant='secondary'
                            className='h-3.5 px-1 text-[9px] font-normal text-muted-foreground'
                          >
                            {t('you')}
                          </Badge>
                        ) : null}
                      </div>
                    </StudioTableCell>
                    <StudioTableCell>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div className='flex cursor-default items-center gap-1.5'>
                            <span
                              className='font-mono text-[11px] text-foreground/90'
                              title={startedStr}
                            >
                              {startedStr}
                            </span>
                            <Badge
                              variant='secondary'
                              className='h-4 rounded px-1 font-mono text-[10px] font-normal text-muted-foreground'
                            >
                              {durationStr}
                            </Badge>
                          </div>
                        </TooltipTrigger>
                        <TooltipContent
                          side='top'
                          className='space-y-1 font-mono text-xs'
                        >
                          <div>
                            {t('startedAt')}: {startedStr}
                          </div>
                          <div>
                            {t('finishedAt')}: {finishedStr}
                          </div>
                          <div>
                            {t('duration')}: {durationStr}
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    </StudioTableCell>
                    <StudioTableCell>
                      <div className='flex items-center gap-1.5 font-mono text-[11px] text-muted-foreground'>
                        <span>
                          {t('read')} {formatMetricValue(summary.readCount)}
                        </span>
                        <span className='text-muted-foreground/40'>·</span>
                        <span>
                          {t('write')} {formatMetricValue(summary.writeCount)}
                        </span>
                        {typeof summary.averageSpeed === 'number' &&
                        summary.averageSpeed > 0 ? (
                          <>
                            <span className='text-muted-foreground/40'>·</span>
                            <span>
                              {formatMetricValue(summary.averageSpeed, 1)}/s
                            </span>
                          </>
                        ) : null}
                        {job.cdc_delay?.fresh ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Badge
                                variant='outline'
                                className={cn(
                                  'rounded-sm px-1 py-0 font-sans text-[10px]',
                                  job.cdc_delay.latest_ms >= 60000
                                    ? 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300'
                                    : 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300',
                                )}
                                data-testid='sync-job-cdc-delay'
                              >
                                {t('cdcDelayBadge', {
                                  value: `${(job.cdc_delay.latest_ms / 1000).toFixed(1)}s`,
                                })}
                              </Badge>
                            </TooltipTrigger>
                            <TooltipContent side='top' className='text-xs'>
                              {t('cdcDelayTooltip', {
                                max: `${(job.cdc_delay.max_1m_ms / 1000).toFixed(1)}s`,
                                time: formatTimeTz(job.cdc_delay.at),
                              })}
                            </TooltipContent>
                          </Tooltip>
                        ) : null}
                        {summary.tableCount > 1 ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Badge
                                variant='outline'
                                className='cursor-pointer rounded-sm border-border/60 bg-muted/50 px-1 py-0 font-sans text-[10px] text-muted-foreground transition-colors hover:bg-muted'
                                onClick={(event) => {
                                  event.stopPropagation();
                                  onSelectJob(job.id);
                                  onOpenDetail(job, 'metrics');
                                }}
                              >
                                {t('multiTableBadge', {
                                  count: summary.tableCount,
                                })}
                              </Badge>
                            </TooltipTrigger>
                            <TooltipContent side='top' className='text-xs'>
                              {t('viewMultiTableMetricsHint')}
                            </TooltipContent>
                          </Tooltip>
                        ) : null}
                      </div>
                    </StudioTableCell>
                    <StudioTableCell align='end'>
                      <div className='flex items-center justify-end gap-0.5'>
                        <Button
                          size='sm'
                          variant='ghost'
                          className={actionButtonClass}
                          aria-label={t('viewJobDetail')}
                          onClick={(event) => {
                            event.stopPropagation();
                            onSelectJob(job.id);
                            onOpenDetail(job);
                          }}
                        >
                          <PanelRightOpen className='size-3.5' />
                          {t('jobDetailShort')}
                        </Button>
                        {actions.showRecover ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>
                                <Button
                                  size='sm'
                                  variant='ghost'
                                  className={actionButtonClass}
                                  disabled={!actions.canRecover}
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    onRecover(job.id);
                                  }}
                                >
                                  {t('recover')}
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {!canRun
                                ? t('readOnlyRecoverTooltip')
                                : t('historyRecoverTooltip')}
                            </TooltipContent>
                          </Tooltip>
                        ) : null}
                        {actions.showStop ? (
                          <>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>
                                  <Button
                                    size='sm'
                                    variant='ghost'
                                    className={actionButtonClass}
                                    disabled={!actions.canStop}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      onSavepointStop(job.id);
                                    }}
                                  >
                                    {t('savepointStop')}
                                  </Button>
                                </span>
                              </TooltipTrigger>
                              {!actions.canStop ? (
                                <TooltipContent>
                                  {t('readOnlyCancelTooltip')}
                                </TooltipContent>
                              ) : null}
                            </Tooltip>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>
                                  <Button
                                    size='sm'
                                    variant='ghost'
                                    className={actionButtonClass}
                                    disabled={!actions.canStop}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      onCancel(job.id);
                                    }}
                                  >
                                    {t('stop')}
                                  </Button>
                                </span>
                              </TooltipTrigger>
                              {!actions.canStop ? (
                                <TooltipContent>
                                  {t('readOnlyCancelTooltip')}
                                </TooltipContent>
                              ) : null}
                            </Tooltip>
                          </>
                        ) : null}
                      </div>
                    </StudioTableCell>
                  </StudioTableRow>
                );
              })
            )}
          </StudioTableBody>
        </StudioTableContent>
      </StudioTableScroll>
      <StudioTableFooter>
        <span>{t('totalItems', {count: jobs.length})}</span>
      </StudioTableFooter>
    </StudioTable>
  );
}

