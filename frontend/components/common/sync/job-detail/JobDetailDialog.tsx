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

import {useMemo, useState, type ReactNode} from 'react';
import dynamic from 'next/dynamic';
import {AlertTriangle, Copy} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {Skeleton} from '@/components/ui/skeleton';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {ExpandableTextPanel} from '@/components/common/layout';
import {SeaTunnelObserveMenu} from '@/components/common/cluster/SeaTunnelObserveMenu';
import {cn} from '@/lib/utils';
import type {SyncJobInstance} from '@/lib/services/sync';
import {JobEventTimelineContent} from '../JobEventTimelineDialog';
import {JobOutcomeHints} from '../JobOutcomeHints';
import {JobScriptDialogContent} from '../StudioDialogs';
import {JobMetricsPanel} from './JobMetricsPanel';
import {
  formatJobDateTime,
  formatJobDuration,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getJobActionState,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getRealtimeEntry,
  getRunModeLabel,
  getSyncJobClusterId,
  submitSpecExecutionMode,
} from '../sync-studio-utils';
import {copyText} from './copy-text';
import {normalizeJobDag} from './dag-model';
import type {JobDetailTab} from './types';

// React Flow 与 dagre 只在打开拓扑时加载，不进工作台主包。
// React Flow and dagre load only when the topology opens, keeping them out of the workbench main bundle.
const JobTopology = dynamic(() => import('./JobTopology'), {
  ssr: false,
  loading: () => <Skeleton className='h-full w-full rounded-lg' />,
});

// 「配置」页签展示的提交字段（不含提交正文，正文在「脚本」页签）。不展示 submit_spec.mode：它取自任务元数据，
// 可能与提交内容里引擎实际使用的 env.job.mode 不一致，后者已在 env 配置中列出。
// Submit fields shown on the Config tab (the submitted body itself lives on the Script tab). submit_spec.mode is left
// out: it comes from task metadata and can disagree with env.job.mode, which the engine actually uses and which is
// already listed under env options.
const SUBMIT_SPEC_FIELDS = [
  'job_name',
  'format',
  'cluster_id',
  'engine_api_mode',
  'engine_base_url',
  'engine_version',
  'trigger_source',
  'start_with_savepoint',
  'stop_timeout_seconds',
  'target_host_id',
] as const;

export interface JobDetailActionContext {
  isAdmin: boolean;
  isOwner: boolean;
  canRun: boolean;
  currentUserId?: number;
  disableRecover: boolean;
}

function stringifyValue(value: unknown): string {
  if (value === null || value === undefined || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : JSON.stringify(value);
}

/**
 * 作业详情（对照 Flink 作业页）：头部是状态、时间与操作，页签为拓扑、指标、Checkpoint、日志、事件、配置、脚本。
 * 只渲染当前页签；日志与 Checkpoint 由工作台以插槽传入，与底部面板共用同一份数据。
 * Job detail (modeled on Flink's job page): the header holds status, times and actions; tabs are topology, metrics,
 * checkpoint, logs, events, config and script. Only the active tab renders; logs and checkpoint are slots from the
 * workbench, sharing data with the bottom console.
 */
export function JobDetailDialog({
  job,
  open,
  onOpenChange,
  tab,
  onTabChange,
  actionContext,
  onRecover,
  onCancel,
  onSavepointStop,
  logsSlot,
  checkpointSlot,
  monacoTheme,
}: {
  job: SyncJobInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tab: JobDetailTab;
  onTabChange: (tab: JobDetailTab) => void;
  actionContext: JobDetailActionContext;
  onRecover: (jobId: number) => void;
  onCancel: (jobId: number) => void;
  onSavepointStop: (jobId: number) => void;
  logsSlot: ReactNode;
  checkpointSlot: ReactNode;
  monacoTheme: string;
}) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const tStudio = useTranslations('workbenchStudio');
  const [errorExpanded, setErrorExpanded] = useState(false);

  const jobDag = job?.result_preview?.job_dag;
  const envOptions = useMemo(() => normalizeJobDag(jobDag).envOptions, [jobDag]);

  if (!job) {
    return null;
  }

  const displayStatus = getDisplayJobLifecycleStatus(job);
  const realtimeEntry = getRealtimeEntry(job);
  const actions = getJobActionState(job, actionContext);
  const clusterId = getSyncJobClusterId(job);
  const engineJobId = String(job.engine_job_id || '').trim();
  const local = submitSpecExecutionMode(job.submit_spec) === 'local';
  const submitSpec = (job.submit_spec ?? {}) as Record<string, unknown>;
  const errorMessage = String(job.error_message || '').trim();

  const meta: Array<{label: string; value: string; mono?: boolean; copy?: boolean}> = [
    {label: t('meta.platformJobId'), value: job.platform_job_id || '-', mono: true, copy: Boolean(job.platform_job_id)},
    {label: t('meta.engineJobId'), value: engineJobId || '-', mono: true, copy: Boolean(engineJobId)},
    {label: t('meta.mode'), value: `${getRunModeLabel(job, tStudio)}${job.task_version > 0 ? ` · v${job.task_version}` : ''}`},
    {label: t('meta.channel'), value: local ? 'Local' : getEngineAPIMode(job) === 'v1' ? 'V1' : 'REST V2'},
    {label: t('meta.startTime'), value: formatJobDateTime(job.started_at), mono: true},
    ...(job.finished_at ? [{label: t('meta.endTime'), value: formatJobDateTime(job.finished_at), mono: true}] : []),
    {label: t('meta.duration'), value: formatJobDuration(job.started_at, job.finished_at), mono: true},
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='flex h-[88vh] w-[92vw] max-w-[92vw] flex-col gap-0 overflow-hidden border border-border/80 p-0 shadow-2xl sm:max-w-[1400px] dark:border-border/60'
        data-testid='sync-job-detail'
      >
        <DialogHeader className='space-y-2 border-b border-border/60 px-5 pb-3 pt-4 text-left'>
          <div className='flex flex-wrap items-center gap-2 pr-8'>
            <DialogTitle className='text-base'>{t('title', {id: job.id})}</DialogTitle>
            <Badge variant='outline' className={cn('rounded-sm px-2 py-0 text-[11px]', getJobStatusBadgeClass(displayStatus))}>
              {getJobStatusLabel(displayStatus)}
            </Badge>
            <JobOutcomeHints job={job} />
            <div className='ml-auto flex items-center gap-1.5'>
              {clusterId && engineJobId && !local ? (
                <SeaTunnelObserveMenu clusterId={clusterId} engineJobId={engineJobId} variant='button' />
              ) : null}
              {actions.showRecover ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>
                      <Button size='sm' variant='outline' className='h-8 text-xs' disabled={!actions.canRecover} onClick={() => onRecover(job.id)}>
                        {tStudio('recover')}
                      </Button>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>
                    {!actionContext.canRun
                      ? tStudio('readOnlyRecoverTooltip')
                      : tStudio('historyRecoverTooltip')}
                  </TooltipContent>
                </Tooltip>
              ) : null}
              {actions.showStop ? (
                <>
                  <Button size='sm' variant='outline' className='h-8 text-xs' disabled={!actions.canStop} onClick={() => onSavepointStop(job.id)}>
                    {tStudio('savepointStop')}
                  </Button>
                  <Button size='sm' variant='outline' className='h-8 text-xs text-destructive hover:text-destructive' disabled={!actions.canStop} onClick={() => onCancel(job.id)}>
                    {tStudio('stop')}
                  </Button>
                </>
              ) : null}
            </div>
          </div>
          <DialogDescription asChild>
            <dl className='flex flex-wrap gap-x-5 gap-y-1 text-xs'>
              {meta.map((item) => (
                <div key={item.label} className='flex min-w-0 items-center gap-1.5'>
                  <dt className='whitespace-nowrap text-muted-foreground'>{item.label}</dt>
                  <dd className={cn('min-w-0 max-w-[260px] truncate text-foreground', item.mono && 'font-mono')} title={item.value}>
                    {item.value}
                  </dd>
                  {item.copy ? (
                    <button type='button' className='text-muted-foreground hover:text-foreground' onClick={() => copyText(item.value, t('copied'))} aria-label={t('copy')}>
                      <Copy className='size-3' />
                    </button>
                  ) : null}
                </div>
              ))}
            </dl>
          </DialogDescription>
          {errorMessage ? (
            <div className='rounded-md border border-destructive/30 bg-destructive/5 px-2.5 py-1.5 text-xs'>
              <div className='flex items-start gap-2'>
                <AlertTriangle className='mt-0.5 size-3.5 shrink-0 text-destructive' />
                <span className={cn('min-w-0 flex-1 text-destructive', !errorExpanded && 'truncate')} title={errorExpanded ? undefined : errorMessage}>
                  {errorExpanded ? t('errorTitle') : errorMessage}
                </span>
                <button type='button' className='shrink-0 text-muted-foreground hover:text-foreground' onClick={() => setErrorExpanded((value) => !value)}>
                  {errorExpanded ? t('collapse') : t('expand')}
                </button>
              </div>
              {errorExpanded ? (
                <ExpandableTextPanel content={errorMessage} height={180} wrap className='mt-1.5' />
              ) : null}
            </div>
          ) : null}
        </DialogHeader>

        <Tabs value={tab} onValueChange={(value) => onTabChange(value as JobDetailTab)} className='flex min-h-0 flex-1 flex-col gap-0'>
          <TabsList className='mx-5 mt-2.5 h-8 w-fit'>
            <TabsTrigger value='topology' className='text-xs'>{t('tabs.topology')}</TabsTrigger>
            <TabsTrigger value='metrics' className='text-xs'>{t('tabs.metrics')}</TabsTrigger>
            <TabsTrigger value='checkpoint' className='text-xs'>{t('tabs.checkpoint')}</TabsTrigger>
            <TabsTrigger value='logs' className='text-xs'>{t('tabs.logs')}</TabsTrigger>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <TabsTrigger value='events' className='text-xs' disabled={!realtimeEntry.supported}>
                    {t('tabs.events')}
                  </TabsTrigger>
                </span>
              </TooltipTrigger>
              {!realtimeEntry.supported ? <TooltipContent>{t('eventsUnsupported')}</TooltipContent> : null}
            </Tooltip>
            <TabsTrigger value='config' className='text-xs'>{t('tabs.config')}</TabsTrigger>
            <TabsTrigger value='script' className='text-xs'>{t('tabs.script')}</TabsTrigger>
          </TabsList>

          <div className='min-h-0 flex-1 px-5 pb-4 pt-2.5'>
            <TabsContent value='topology' className='mt-0 h-full'>
              {jobDag ? (
                <JobTopology
                  mode='runtime'
                  jobDag={jobDag}
                  metrics={job.result_preview?.metrics}
                  jobId={job.id}
                  realtimeEnabled={tab === 'topology' && realtimeEntry.supported && realtimeEntry.running}
                  running={realtimeEntry.running}
                />
              ) : (
                <div className='flex h-full items-center justify-center text-sm text-muted-foreground'>{t('noRuntimeDag')}</div>
              )}
            </TabsContent>
            <TabsContent value='metrics' className='mt-0 flex h-full min-h-0 flex-col overflow-hidden'>
              {tab === 'metrics' ? <JobMetricsPanel job={job} active={tab === 'metrics'} /> : null}
            </TabsContent>
            <TabsContent value='checkpoint' className='mt-0 h-full min-h-0 overflow-auto'>
              {tab === 'checkpoint' ? checkpointSlot : null}
            </TabsContent>
            <TabsContent value='logs' className='mt-0 h-full min-h-0 overflow-hidden'>
              {tab === 'logs' ? logsSlot : null}
            </TabsContent>
            <TabsContent value='events' className='mt-0 h-full min-h-0'>
              <JobEventTimelineContent job={job} active={tab === 'events' && realtimeEntry.supported} />
            </TabsContent>
            <TabsContent value='config' className='mt-0 h-full min-h-0 overflow-auto'>
              <div className='grid gap-4 lg:grid-cols-2'>
                <section>
                  <h3 className='mb-1.5 text-xs font-semibold'>{t('config.env')}</h3>
                  <dl className='divide-y divide-border/50 rounded-md border border-border/60 text-xs'>
                    {Object.keys(envOptions).length === 0 ? (
                      <div className='px-2.5 py-2 text-muted-foreground'>{t('config.empty')}</div>
                    ) : (
                      Object.entries(envOptions).map(([key, value]) => (
                        <div key={key} className='grid grid-cols-[minmax(140px,40%)_1fr] gap-2 px-2.5 py-1.5'>
                          <dt className='break-all font-mono text-muted-foreground'>{key}</dt>
                          <dd className='break-all font-mono'>{value}</dd>
                        </div>
                      ))
                    )}
                  </dl>
                </section>
                <section>
                  <h3 className='mb-1.5 text-xs font-semibold'>{t('config.submit')}</h3>
                  <dl className='divide-y divide-border/50 rounded-md border border-border/60 text-xs'>
                    {SUBMIT_SPEC_FIELDS.filter((key) => submitSpec[key] !== undefined).map((key) => (
                      <div key={key} className='grid grid-cols-[minmax(140px,40%)_1fr] gap-2 px-2.5 py-1.5'>
                        <dt className='break-all font-mono text-muted-foreground'>{key}</dt>
                        <dd className='break-all font-mono'>{stringifyValue(submitSpec[key])}</dd>
                      </div>
                    ))}
                  </dl>
                </section>
              </div>
            </TabsContent>
            <TabsContent value='script' className='mt-0 flex h-full min-h-0 flex-col overflow-hidden'>
              {tab === 'script' ? <JobScriptDialogContent job={job} monacoTheme={monacoTheme} /> : null}
            </TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
