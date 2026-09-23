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

import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  BarChart3,
  ExternalLink,
  FileCode2,
  FileText,
  GitBranch,
  Maximize2,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {SyncJobInstance, SyncJobLogsResult} from '@/lib/services/sync';
import {cn} from '@/lib/utils';
import {
  buildDisplayLogLines,
  canRecoverFromJob,
  extractJobMetricSummary,
  formatJobDateTime,
  formatJobDuration,
  formatMetricValue,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getEngineEndpointLabel,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getLogLineClass,
  getRunModeLabel,
  getSyncJobClusterId,
  isJobLifecycleActive,
  submitSpecExecutionMode,
  type LogFilterMode,
} from './sync-studio-utils';

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
  const renderedLines = buildDisplayLogLines(logsResult?.logs || '', 800);
  const isMixedLogMode =
    logsResult?.empty_reason === 'mixed_log_mode' ||
    logsResult?.cluster_job_log_mode === 'mixed';
  const clusterId = logsResult?.cluster_id || getSyncJobClusterId(job);

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-2'>
      <div className='flex flex-wrap items-center gap-2 rounded-lg border border-border/50 bg-background/70 px-3 py-2 text-xs'>
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
        <span className='text-muted-foreground'>
          {loading
            ? t('loading')
            : logsResult?.updated_at
              ? new Date(logsResult.updated_at).toLocaleTimeString()
              : '-'}
        </span>
      </div>
      <div className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
        <div className='sticky top-0 z-10 shrink-0 border-b border-border/50 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85'>
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
              <div className='flex items-center gap-1 rounded-md border border-border/50 bg-background px-1 py-1'>
                {(['all', 'warn', 'error'] as LogFilterMode[]).map((mode) => (
                  <button
                    key={mode}
                    type='button'
                    className={cn(
                      'rounded px-2 py-0.5 text-[11px]',
                      filterMode === mode
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground',
                    )}
                    onClick={() => onFilterChange(mode)}
                  >
                    {mode === 'all' ? t('all') : mode.toUpperCase()}
                  </button>
                ))}
              </div>
              <Button
                size='sm'
                variant='ghost'
                className='h-7 px-1.5 text-xs'
                onClick={onExpand}
              >
                <Maximize2 className='mr-1 size-3.5' />
                {t('expand')}
              </Button>
            </div>
          </div>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-border/50 px-3 py-2 text-[11px] text-muted-foreground'>
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
  onViewMetrics,
  onViewScript,
  onViewJobDag,
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
  onViewMetrics: (job: SyncJobInstance) => void;
  onViewScript: (job: SyncJobInstance) => void;
  onViewJobDag?: (job: SyncJobInstance) => void;
  disableRecover: boolean;
}) {
  const t = useTranslations('workbenchStudio');
  if (jobs.length === 0) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noJobRuns')}</div>
    );
  }
  return (
    <div className='h-full overflow-auto rounded-lg border border-border/50 bg-background/70'>
      <Table className='min-w-[760px]'>
        <TableHeader>
          <TableRow className='hover:bg-transparent border-border/50'>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('task')}</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('status')}</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>模式 / 通道</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('initiator')}</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>时序 / 耗时</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('metrics')}</TableHead>
            <TableHead className='h-8 py-1 px-2.5 text-right text-xs'>{t('actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {jobs.map((job) => {
            const summary = extractJobMetricSummary(job);
            const displayStatus = getDisplayJobLifecycleStatus(job);
            const startedStr = formatJobDateTime(job.started_at);
            const finishedStr = formatJobDateTime(job.finished_at);
            const durationStr = formatJobDuration(job.started_at, job.finished_at);
            // 提取开始时间中的时间部分（当天仅显示时分秒）
            const timeOnly = startedStr.includes(' ') ? startedStr.split(' ')[1] : startedStr;

            return (
              <TableRow
                key={job.id}
                className={cn(
                  'cursor-pointer transition-colors border-border/40 hover:bg-muted/40',
                  selectedJobId === job.id ? 'bg-primary/5 font-medium' : '',
                )}
                onClick={() => onSelectJob(job.id)}
              >
                {/* 任务 ID 与作业平台号紧凑行 */}
                <TableCell className='py-1.5 px-2.5 whitespace-nowrap'>
                  <div className='flex items-center gap-1.5'>
                    <span className='font-semibold text-foreground text-xs'>#{job.id}</span>
                    {job.platform_job_id ? (
                      <span
                        className='font-mono text-[10px] text-muted-foreground/70 max-w-[100px] truncate'
                        title={job.platform_job_id}
                      >
                        {job.platform_job_id}
                      </span>
                    ) : null}
                  </div>
                </TableCell>

                {/* 状态胶囊 */}
                <TableCell className='py-1.5 px-2.5 whitespace-nowrap'>
                  <Badge
                    variant='outline'
                    className={cn(
                      'rounded-sm border px-2 py-0 text-[11px] leading-tight',
                      getJobStatusBadgeClass(displayStatus),
                    )}
                  >
                    {getJobStatusLabel(displayStatus)}
                  </Badge>
                </TableCell>

                {/* 模式与通道合并 */}
                <TableCell className='py-1.5 px-2.5 whitespace-nowrap'>
                  <div className='flex items-center gap-1'>
                    <Badge variant='outline' className='rounded-sm text-[10px] px-1 py-0'>
                      {getRunModeLabel(job, t)}
                    </Badge>
                    <span className='text-[10px] text-muted-foreground font-mono'>
                      {submitSpecExecutionMode(job.submit_spec) === 'local'
                        ? 'Local'
                        : getEngineAPIMode(job) === 'v1'
                          ? 'V1'
                          : 'REST V2'}
                    </span>
                  </div>
                </TableCell>

                {/* 发起人 */}
                <TableCell className='py-1.5 px-2.5 text-xs whitespace-nowrap'>
                  <div className='flex items-center gap-1'>
                    <span className='text-foreground text-[11px] font-medium'>
                      {(() => {
                        if (!job.created_by) return '-';
                        if (currentUserId && job.created_by === currentUserId) {
                          return currentUsername || 'admin';
                        }
                        const matched = workspaceUsers.find((u) => u.id === job.created_by);
                        if (matched?.username) return matched.username;
                        if (matched?.nickname) return matched.nickname;
                        if (job.created_by === 1) return 'admin';
                        return `User #${job.created_by}`;
                      })()}
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
                </TableCell>

                {/* 时序与耗时合并 */}
                <TableCell className='py-1.5 px-2.5 whitespace-nowrap'>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <div className='flex items-center gap-1.5 cursor-default'>
                        <span className='font-mono text-[11px] text-foreground/90' title={startedStr}>
                          {timeOnly}
                        </span>
                        <Badge
                          variant='secondary'
                          className='h-4 rounded px-1 font-mono text-[10px] font-normal text-muted-foreground'
                        >
                          {durationStr}
                        </Badge>
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side='top' className='space-y-1 font-mono text-xs'>
                      <div>开始时间: {startedStr}</div>
                      <div>结束时间: {finishedStr}</div>
                      <div>累计耗时: {durationStr}</div>
                    </TooltipContent>
                  </Tooltip>
                </TableCell>

                {/* 指标紧凑单行化，支持 SeaTunnel 3.0 多表任务胶囊提示与直达 */}
                {/* Compact single-line metrics, supporting SeaTunnel 3.0 multi-table badge and quick navigation */}
                <TableCell className='py-1.5 px-2.5 whitespace-nowrap'>
                  <div className='font-mono text-[11px] text-muted-foreground flex items-center gap-1.5'>
                    {/* 读写数字与前缀等色，不再加粗高亮，降低视觉噪音 / Metrics values match muted color, no bold highlight */}
                    <span>读 {formatMetricValue(summary.readCount)}</span>
                    <span className='text-muted-foreground/40'>·</span>
                    <span>写 {formatMetricValue(summary.writeCount)}</span>
                    {typeof summary.averageSpeed === 'number' && summary.averageSpeed > 0 ? (
                      <>
                        <span className='text-muted-foreground/40'>·</span>
                        <span>{formatMetricValue(summary.averageSpeed, 1)}/s</span>
                      </>
                    ) : null}
                    {summary.tableCount > 1 ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge
                            variant='outline'
                            className='cursor-pointer rounded-sm border-border/60 bg-muted/50 px-1 py-0 font-sans text-[10px] text-muted-foreground hover:bg-muted transition-colors'
                            onClick={(e) => {
                              e.stopPropagation();
                              onViewMetrics(job);
                            }}
                          >
                            {t('multiTableBadge', {count: summary.tableCount})}
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs'>
                          {t('viewMultiTableMetricsHint')}
                        </TooltipContent>
                      </Tooltip>
                    ) : null}
                  </div>
                </TableCell>

                {/* 操作按钮 */}
                {/* Action buttons */}
                <TableCell className='py-1.5 px-2.5 text-right whitespace-nowrap'>
                  <div className='flex justify-end items-center gap-1.5'>
                    {Boolean(job.result_preview?.job_dag) ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            size='icon'
                            variant='ghost'
                            className='size-7 text-muted-foreground hover:text-foreground'
                            aria-label={t('viewExecutionDag')}
                            onClick={(event) => {
                              event.stopPropagation();
                              onViewJobDag?.(job);
                            }}
                          >
                            <GitBranch className='size-3.5' />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs'>
                          {t('viewExecutionDag')}
                        </TooltipContent>
                      </Tooltip>
                    ) : null}
                    {/* 查看实际执行脚本按钮加 Tooltip / Wrap script button with Tooltip */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size='icon'
                          variant='ghost'
                          className='size-7 text-muted-foreground hover:text-foreground'
                          aria-label={t('viewExecutedScript')}
                          onClick={(event) => {
                            event.stopPropagation();
                            onViewScript(job);
                          }}
                        >
                          <FileCode2 className='size-3.5' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('viewExecutedScript')}
                      </TooltipContent>
                    </Tooltip>
                    {/* 查看运行指标按钮加 Tooltip / Wrap metrics button with Tooltip */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size='icon'
                          variant='ghost'
                          className='size-7 text-muted-foreground hover:text-foreground'
                          aria-label={t('viewMetrics')}
                          onClick={(event) => {
                            event.stopPropagation();
                            onViewMetrics(job);
                          }}
                        >
                          <BarChart3 className='size-3.5' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='text-xs'>
                        {t('viewMetrics')}
                      </TooltipContent>
                    </Tooltip>
                    {job.run_type !== 'preview' ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span>
                            <Button
                              size='sm'
                              variant='outline'
                              className='h-7 px-2 text-xs'
                              disabled={
                                disableRecover ||
                                !canRecoverFromJob(job) ||
                                !canRun
                              }
                              onClick={(event) => {
                                event.stopPropagation();
                                onRecover(job.id);
                              }}
                            >
                              {t('recover')}
                            </Button>
                          </span>
                        </TooltipTrigger>
                        {!canRun ? (
                          <TooltipContent>
                            {t('readOnlyRecoverTooltip')}
                          </TooltipContent>
                        ) : null}
                      </Tooltip>
                    ) : null}
                    {isJobLifecycleActive(
                      getDisplayJobLifecycleStatus(job),
                    ) ? (
                      (() => {
                        const canCancel =
                          isAdmin ||
                          isOwner ||
                          (currentUserId !== undefined &&
                            job.created_by === currentUserId);
                        return (
                          <>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    className='h-8 text-xs'
                                    disabled={!canCancel}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      onSavepointStop(job.id);
                                    }}
                                  >
                                    {t('savepointStop')}
                                  </Button>
                                </span>
                              </TooltipTrigger>
                              {!canCancel ? (
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
                                    variant='outline'
                                    className='h-8 text-xs'
                                    disabled={!canCancel}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      onCancel(job.id);
                                    }}
                                  >
                                    {t('stop')}
                                  </Button>
                                </span>
                              </TooltipTrigger>
                              {!canCancel ? (
                                <TooltipContent>
                                  {t('readOnlyCancelTooltip')}
                                </TooltipContent>
                              ) : null}
                            </Tooltip>
                          </>
                        );
                      })()
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

