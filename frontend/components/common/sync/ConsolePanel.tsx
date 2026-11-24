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
  ExternalLink,
  FileText,
  Maximize2,
} from 'lucide-react';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Badge} from '@/components/ui/badge';
import {Skeleton} from '@/components/ui/skeleton';
import {Button} from '@/components/ui/button';
import type {SyncJobLogsResult, SyncJobSummary} from '@/lib/services/sync';
import {cn} from '@/lib/utils';
import {TableLoadingBar} from '@/components/common/layout';
import {JobOutcomeHints} from './JobOutcomeHints';
import {
  buildDisplayLogLines,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getEngineEndpointLabel,
  formatJobSyncLagHint,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getLogLineClass,
  getSyncJobClusterId,
  submitSpecExecutionMode,
  type LogFilterMode,
} from './sync-studio-utils';
import {formatTimeTz} from '@/lib/timezone';

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
  job: SyncJobSummary | null;
  logsResult: SyncJobLogsResult | null;
  loading: boolean;
  filterMode: LogFilterMode;
  onFilterChange: (mode: LogFilterMode) => void;
  /** 大日志弹窗入口；不传时隐藏「展开」按钮（如大盘 JobDetailHost，见该文件头说明）。/ Full log viewer entry; the expand button is hidden when omitted (e.g. the dashboard JobDetailHost, see that file's header note). */
  onExpand?: () => void;
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
              {onExpand ? (
                <Button
                  size='sm'
                  variant='ghost'
                  className='h-9 rounded-[var(--radius-control)] px-2 text-xs'
                  onClick={onExpand}
                >
                  <Maximize2 className='mr-1 size-3.5' />
                  {t('expand')}
                </Button>
              ) : null}
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
