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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  BarChart3,
  ExternalLink,
  FileCode2,
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
            <div className='text-muted-foreground'>{t('noLogs')}</div>
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
  disableRecover,
}: {
  jobs: SyncJobInstance[];
  selectedJobId: number | null;
  currentUserId?: number;
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
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('task')}</TableHead>
            <TableHead>{t('runMode')}</TableHead>
            <TableHead>{t('status')}</TableHead>
            <TableHead>{t('channel')}</TableHead>
            <TableHead>{t('initiator')}</TableHead>
            <TableHead>{t('startedAt')}</TableHead>
            <TableHead>{t('finishedAt')}</TableHead>
            <TableHead>{t('duration')}</TableHead>
            <TableHead>{t('metrics')}</TableHead>
            <TableHead className='text-right'>{t('actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {jobs.map((job) => {
            const summary = extractJobMetricSummary(job);
            const displayStatus = getDisplayJobLifecycleStatus(job);
            return (
              <TableRow
                key={job.id}
                className={cn(selectedJobId === job.id ? 'bg-primary/5' : '')}
                onClick={() => onSelectJob(job.id)}
              >
                <TableCell>
                  <div className='font-medium'>#{job.id}</div>
                  <div className='text-xs text-muted-foreground'>
                    {job.platform_job_id || '-'}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant='outline' className='rounded-sm text-[11px]'>
                    {getRunModeLabel(job, t)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge
                    variant='outline'
                    className={cn(
                      'rounded-sm border px-2 py-0.5 text-[11px]',
                      getJobStatusBadgeClass(displayStatus),
                    )}
                  >
                    {getJobStatusLabel(displayStatus)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge variant='outline' className='rounded-sm text-[11px]'>
                    {submitSpecExecutionMode(job.submit_spec) === 'local'
                      ? 'Local Agent'
                      : getEngineAPIMode(job) === 'v1'
                        ? 'Legacy REST V1'
                        : 'REST V2'}
                  </Badge>
                </TableCell>
                <TableCell className='text-xs'>
                  <div className='flex items-center gap-1'>
                    <span className='font-medium text-foreground'>
                      {job.created_by
                        ? workspaceUsers.find((u) => u.id === job.created_by)
                            ?.username || `User #${job.created_by}`
                        : '-'}
                    </span>
                    {currentUserId && job.created_by === currentUserId ? (
                      <Badge
                        variant='secondary'
                        className='h-4 px-1 text-[10px] font-normal text-muted-foreground'
                      >
                        {t('you')}
                      </Badge>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDateTime(job.started_at)}
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDateTime(job.finished_at)}
                </TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {formatJobDuration(job.started_at, job.finished_at)}
                </TableCell>
                <TableCell>
                  <div className='space-y-0.5 text-xs'>
                    <div>
                      {t('read')} {formatMetricValue(summary.readCount)}
                    </div>
                    <div>
                      {t('write')} {formatMetricValue(summary.writeCount)}
                    </div>
                    <div>
                      {t('averageSpeed')}{' '}
                      {formatMetricValue(summary.averageSpeed, 1)}/s
                    </div>
                  </div>
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-2'>
                    <Button
                      size='icon'
                      variant='outline'
                      className='size-8'
                      aria-label={t('viewExecutedScript')}
                      onClick={(event) => {
                        event.stopPropagation();
                        onViewScript(job);
                      }}
                    >
                      <FileCode2 className='size-4' />
                    </Button>
                    <Button
                      size='icon'
                      variant='outline'
                      className='size-8'
                      aria-label={t('viewMetrics')}
                      onClick={(event) => {
                        event.stopPropagation();
                        onViewMetrics(job);
                      }}
                    >
                      <BarChart3 className='size-4' />
                    </Button>
                    {job.run_type !== 'preview' ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span>
                            <Button
                              size='sm'
                              variant='outline'
                              className='h-8 text-xs'
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

