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
 * 巡检报告详情体：元信息 + 按严重度排序的发现项 + 零发现 / 未完成说明 +
 * 诊断包区（生成 / 重新生成 / 执行日志 / 预览 / 下载，含 2s 轮询）+ 底部操作，
 * 常驻详情栏 / 抽屉形态共用（detailCol 结构由宿主给宽）。
 * Inspection report detail body: meta + severity-sorted findings + zero /
 * incomplete states + the bundle section (generate / regenerate / execution
 * logs / preview / download with 2s polling) + footer actions, shared by the
 * persistent rail and the drawer (the detailCol contract is width-adaptive).
 */
import {useCallback, useEffect, useRef, useState} from 'react';
import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {
  ArrowUpRight,
  Download,
  ExternalLink,
  FileText,
  Package,
} from 'lucide-react';
import {toast} from '@/lib/toast';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {NumberField} from '@/components/arc/number-field/number-field';
import {Switch} from '@/components/arc/switch/switch';
import {Button as UIButton} from '@/components/ui/button';
import {Skeleton} from '@/components/ui/skeleton';
import {HelpHint} from '@/components/common/HelpHint';
import {
  InspectionFindingCard,
  InspectionZeroState,
} from '../InspectionEvidenceView';
import {localizeDiagnosticsText} from '../text-utils';
import {formatErrorDateTime, formatNodeOrigin} from '../errors/error-utils';
import services from '@/lib/services';
import type {
  DiagnosticsClusterOption,
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReport,
  DiagnosticsTask,
  DiagnosticsTaskNodeScope,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';
import {SoftChip} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {
  inspectionStatusTone,
  inspectionTaskStatusKey,
  sortFindingsBySeverity,
  taskStatusTone,
} from './inspection-utils';

const DEFAULT_BUNDLE_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: false,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
};

// 诊断包任务只有 pending / ready / running 需要 2s 轮询。
// Only pending / ready / running bundle tasks need the 2s poll.
const LIVE_TASK_STATUSES = new Set(['pending', 'ready', 'running']);

export interface InspectionReportDetailProps {
  /** 保留的报告（useDrawerItem 输出），深链时可能为空。Retained report (useDrawerItem output); may be null on deep links before the fetch resolves. */
  report: DiagnosticsInspectionReport | null;
  findings: DiagnosticsInspectionFinding[];
  bundleTask: DiagnosticsTask | null;
  loading: boolean;
  clusterOptions: DiagnosticsClusterOption[];
  onBundleTaskChange: (task: DiagnosticsTask) => void;
  onClose: () => void;
}

export function InspectionReportDetail({
  report,
  findings,
  bundleTask,
  loading,
  clusterOptions,
  onBundleTaskChange,
  onClose,
}: InspectionReportDetailProps) {
  const t = useTranslations('diagnosticsCenter.inspections');
  const tc = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');
  const [bundleOptions, setBundleOptions] = useState<DiagnosticsTaskOptions>(
    DEFAULT_BUNDLE_OPTIONS,
  );
  const [nodeScope, setNodeScope] = useState<DiagnosticsTaskNodeScope>('all');
  const [bundleLookbackMinutes, setBundleLookbackMinutes] = useState(30);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [execLogDialogOpen, setExecLogDialogOpen] = useState(false);
  const [creatingBundle, setCreatingBundle] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const taskIsLive = bundleTask
    ? LIVE_TASK_STATUSES.has(bundleTask.status)
    : false;

  const pollBundleTask = useCallback(
    async (taskId: number) => {
      try {
        const result = await services.diagnostics.getTaskSafe(taskId);
        if (!result.success || !result.data) {
          return;
        }
        onBundleTaskChange(result.data);
        if (!LIVE_TASK_STATUSES.has(result.data.status)) {
          if (result.data.status === 'succeeded') {
            toast.success(t('followUp.taskCompleteSuccess'));
          } else {
            toast.error(t('followUp.taskFailed'));
          }
        }
      } catch {
        // 静默失败，等待下次轮询。
        // Silently catch and retry on the next interval tick.
      }
    },
    [onBundleTaskChange, t],
  );

  // 任务到达终态后 effect 清理即停轮询。
  // Polling stops when the effect cleans up once the task reaches a terminal state.
  useEffect(() => {
    if (!taskIsLive || !bundleTask) {
      return;
    }
    const taskId = bundleTask.id;
    pollTimerRef.current = setInterval(() => {
      void pollBundleTask(taskId);
    }, 2000);
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [bundleTask, pollBundleTask, taskIsLive]);

  const handleCreateBundle = useCallback(async () => {
    if (!report || creatingBundle) {
      return;
    }
    setConfirmDialogOpen(false);
    setCreatingBundle(true);
    try {
      const firstFinding = findings[0];
      const result = await services.diagnostics.createTaskSafe({
        cluster_id: report.cluster_id,
        trigger_source: firstFinding ? 'inspection_finding' : 'manual',
        source_ref: firstFinding
          ? {
              inspection_report_id: report.id,
              inspection_finding_id: firstFinding.id,
            }
          : undefined,
        node_scope: nodeScope || 'all',
        options: bundleOptions,
        lookback_minutes: bundleLookbackMinutes,
        auto_start: true,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('followUp.createTaskError'));
        return;
      }
      toast.success(t('followUp.createTaskSuccess'));
      onBundleTaskChange(result.data);
    } finally {
      setCreatingBundle(false);
    }
  }, [
    bundleLookbackMinutes,
    bundleOptions,
    creatingBundle,
    findings,
    nodeScope,
    onBundleTaskChange,
    report,
    t,
  ]);

  const clusterLabel = report
    ? (clusterOptions.find((c) => c.cluster_id === report.cluster_id)
        ?.cluster_name ??
      report.cluster_name ??
      `#${report.cluster_id}`)
    : '-';
  const hasFindings = findings.length > 0;

  return (
    <div className={layout.detailCol} data-testid='inspection-report-detail'>
      <div className={layout.detailScroll}>
        {loading && !report ? (
          <div className='space-y-3'>
            <Skeleton className='h-16 w-full' />
            <Skeleton className='h-28 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        ) : report ? (
          <>
            {/* 元信息：状态 / 完成时间 / 回溯窗口 / 错误阈值 / 发起人 / 归属集群
                （按报告自身 cluster_id 反查，与当前筛选无关）。
                Meta: status / finished / window / threshold / requested by /
                owning cluster (resolved from the report's own cluster_id, not
                the current filter). */}
            <div className='rounded-lg border p-3.5 space-y-2 bg-muted/15'>
              <div className='flex items-center justify-between gap-2'>
                <span className='font-mono text-xs tabular-nums text-muted-foreground'>
                  {t('evidence.reportNumber', {id: report.id})}
                </span>
                <SoftChip tone={inspectionStatusTone(report.status)}>
                  {t(`status.${report.status}`)}
                </SoftChip>
              </div>
              <div className='grid gap-1.5 text-xs text-muted-foreground sm:grid-cols-2'>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('clusterLabel')}:{' '}
                  </span>
                  {clusterLabel}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('finishedAt')}:{' '}
                  </span>
                  {formatErrorDateTime(report.finished_at || report.created_at)}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('detailPage.lookbackMinutes')}{' '}
                  </span>
                  {t('lookbackValue', {minutes: report.lookback_minutes || 30})}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('detailPage.errorThreshold')}{' '}
                  </span>
                  {t('errorThresholdValue', {
                    count: report.error_threshold,
                  })}
                </div>
                <div className='sm:col-span-2'>
                  <span className='text-foreground font-medium'>
                    {t('requestedBy')}:{' '}
                  </span>
                  {report.requested_by || '-'}
                </div>
              </div>
            </div>

            {report.error_message ? (
              <p
                role='alert'
                className='border-l-2 border-destructive pl-3 text-sm text-destructive'
              >
                {report.error_message}
              </p>
            ) : null}

            {hasFindings ? (
              <section
                className='space-y-3'
                aria-label={t('evidence.attentionTitle')}
              >
                <div className='flex items-baseline justify-between border-b border-border/70 pb-3'>
                  <h3 className='font-serif text-xl text-foreground'>
                    {t('evidence.attentionTitle')}
                  </h3>
                  <span className='font-mono text-xl tabular-nums text-foreground'>
                    {findings.length}
                  </span>
                </div>
                {sortFindingsBySeverity(findings).map((finding, index) => (
                  <InspectionFindingCard
                    key={finding.id}
                    finding={finding}
                    index={index}
                    compact
                    origin={formatNodeOrigin({
                      nodeId: finding.related_node_id,
                      hostId: finding.related_host_id,
                      hostName: finding.related_host_name,
                      hostIp: finding.related_host_ip,
                    })}
                  />
                ))}
              </section>
            ) : report.status === 'completed' &&
              report.finding_total === 0 &&
              !report.error_message ? (
              <InspectionZeroState compact />
            ) : report.status === 'completed' ? (
              <p className='border border-border/70 bg-card px-5 py-8 text-sm text-muted-foreground'>
                {t('evidence.unavailable')}
              </p>
            ) : (
              <p className='border border-border/70 bg-card px-5 py-8 text-sm text-muted-foreground'>
                {report.status === 'failed'
                  ? t('evidence.incompleteDescription')
                  : t('evidence.pendingDescription')}
              </p>
            )}

            {/* 诊断包是后续操作，不抢占证据的首屏。
                Bundle actions follow the evidence rather than leading the page. */}
            {report.status === 'completed' &&
            (hasFindings || bundleTask) ? (
              <section className='space-y-3 border-t border-border/70 pt-5'>
                <div className='flex items-center justify-between gap-3'>
                  <h3 className='text-sm font-semibold text-foreground'>
                    {t('detailPage.bundleTitle')}
                  </h3>
                  {bundleTask ? (
                    <SoftChip tone={taskStatusTone(bundleTask.status)}>
                      {tc('inspections.taskLabel', {id: bundleTask.id})}{' '}
                      {inspectionTaskStatusKey(bundleTask.status)
                        ? tc(inspectionTaskStatusKey(bundleTask.status))
                        : bundleTask.status}
                    </SoftChip>
                  ) : null}
                </div>
                {bundleTask ? (
                  <div className='flex flex-wrap items-center gap-2'>
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => setExecLogDialogOpen(true)}
                    >
                      <FileText className='size-3.5' aria-hidden />
                      {t('detailPage.viewExecutionLogs')}
                    </Button>
                    {bundleTask.status === 'succeeded' ? (
                      <>
                        <UIButton asChild variant='outline' size='sm'>
                          <a
                            href={services.diagnostics.getTaskHTMLUrl(
                              bundleTask.id,
                            )}
                            target='_blank'
                            rel='noopener noreferrer'
                          >
                            <ExternalLink
                              className='size-3.5'
                              aria-hidden
                            />
                            {t('detailPage.previewReport')}
                          </a>
                        </UIButton>
                        <UIButton asChild variant='outline' size='sm'>
                          <a
                            href={services.diagnostics.getTaskBundleUrl(
                              bundleTask.id,
                            )}
                            download
                          >
                            <Download className='size-3.5' aria-hidden />
                            {t('detailPage.downloadBundle')}
                          </a>
                        </UIButton>
                      </>
                    ) : null}
                    {bundleTask.status === 'succeeded' ||
                    bundleTask.status === 'failed' ? (
                      <Button
                        type='button'
                        size='sm'
                        variant='secondary'
                        onClick={() => setConfirmDialogOpen(true)}
                        disabled={creatingBundle}
                      >
                        <Package className='size-3.5' aria-hidden />
                        {t('detailPage.regenerate')}
                      </Button>
                    ) : null}
                    {taskIsLive ? (
                      <span className='text-xs text-muted-foreground'>
                        {t('refreshingLabel')}
                      </span>
                    ) : null}
                  </div>
                ) : (
                  <Button
                    type='button'
                    size='sm'
                    onClick={() => setConfirmDialogOpen(true)}
                    disabled={creatingBundle}
                  >
                    <Package className='size-3.5' aria-hidden />
                    {t('followUp.generateBundle')}
                  </Button>
                )}
              </section>
            ) : null}
          </>
        ) : null}
      </div>

      {report ? (
        <div className={layout.detailFoot}>
          <UIButton asChild variant='outline' size='sm'>
            <Link href={`/diagnostics/inspections/${report.id}`}>
              {t('evidence.viewFullReport')}
              <ArrowUpRight className='size-3.5' aria-hidden />
            </Link>
          </UIButton>
          <span className={layout.detailFootGrow} />
          <Button type='button' variant='ghost' size='sm' onClick={onClose}>
            {t('evidence.close')}
          </Button>
        </div>
      ) : null}

      {/* 生成现场诊断包确认弹窗 */}
      <Dialog open={confirmDialogOpen} onOpenChange={setConfirmDialogOpen}>
        <DialogContent
          title={t('detailPage.confirmBundleTitle')}
          description={t('detailPage.confirmBundleDescription')}
          className='w-[min(calc(100vw-2rem),440px)]!'
        >
          <div className='space-y-4 text-sm'>
            <NumberField
              label={t('detailPage.lookbackField')}
              labelAddon={
                <HelpHint
                  label={t('detailPage.lookbackField')}
                  content={t('detailPage.bundleLookbackHint')}
                  data-testid='bundle-lookback-hint'
                />
              }
              value={bundleLookbackMinutes}
              onValueChange={setBundleLookbackMinutes}
              min={5}
              max={1440}
              step={5}
              suffix={t('minutesSuffix')}
            />
            <div className='space-y-2.5'>
              <div className='flex items-center justify-between rounded-lg border p-2.5'>
                <div>
                  <div className='font-medium text-xs'>
                    {t('detailPage.includeThreadDump')}
                  </div>
                  <div className='text-[11px] text-muted-foreground'>
                    {t('detailPage.includeThreadDumpHint')}
                  </div>
                </div>
                <Switch
                  checked={bundleOptions.include_thread_dump}
                  onCheckedChange={(checked) =>
                    setBundleOptions((c) => ({
                      ...c,
                      include_thread_dump: checked,
                    }))
                  }
                  aria-label={t('detailPage.includeThreadDump')}
                />
              </div>
              <div className='flex items-center justify-between rounded-lg border p-2.5'>
                <div>
                  <div className='font-medium text-xs'>
                    {t('detailPage.includeJVMDump')}
                  </div>
                  <div className='text-[11px] text-muted-foreground'>
                    {t('detailPage.includeJVMDumpHint')}
                  </div>
                </div>
                <Switch
                  checked={bundleOptions.include_jvm_dump}
                  onCheckedChange={(checked) =>
                    setBundleOptions((c) => ({
                      ...c,
                      include_jvm_dump: checked,
                    }))
                  }
                  aria-label={t('detailPage.includeJVMDump')}
                />
              </div>
              <SegmentedControl
                label={t('detailPage.nodeScopeLabel')}
                value={nodeScope}
                onValueChange={(v) =>
                  setNodeScope(v as DiagnosticsTaskNodeScope)
                }
                options={[
                  {value: 'all', label: t('detailPage.allNodes')},
                  {value: 'related', label: t('detailPage.relatedNodes')},
                ]}
              />
            </div>
          </div>
          <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setConfirmDialogOpen(false)}
            >
              {commonT('cancel')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={() => void handleCreateBundle()}
              disabled={creatingBundle}
            >
              {t('detailPage.confirmCreate')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* 查看执行日志弹窗 */}
      <Dialog open={execLogDialogOpen} onOpenChange={setExecLogDialogOpen}>
        <DialogContent
          title={t('detailPage.executionLogsTitle')}
          description={t('detailPage.executionLogsDescription')}
          className='w-[min(calc(100vw-2rem),680px)]!'
          bodyClassName='max-h-[min(70dvh,520px)] overflow-y-auto'
        >
          <div className='space-y-2 text-xs'>
            {bundleTask?.steps?.length ? (
              bundleTask.steps.map((step) => (
                <div
                  key={step.id}
                  className='rounded-lg border p-2.5 space-y-1'
                >
                  <div className='flex items-center gap-2'>
                    <SoftChip tone={taskStatusTone(step.status)}>
                      {inspectionTaskStatusKey(step.status)
                        ? tc(inspectionTaskStatusKey(step.status))
                        : step.status}
                    </SoftChip>
                    <span className='font-mono text-[11px] text-muted-foreground'>
                      {step.code}
                    </span>
                  </div>
                  <div className='text-xs font-medium'>
                    {localizeDiagnosticsText(step.title) || step.description}
                  </div>
                  {step.error || step.message ? (
                    <div className='rounded bg-muted/60 px-2 py-1 text-[11px] text-muted-foreground font-mono'>
                      {step.error || step.message}
                    </div>
                  ) : null}
                </div>
              ))
            ) : bundleTask?.failure_reason ? (
              <div className='rounded-md border border-destructive/20 bg-destructive/5 p-3 text-xs text-destructive'>
                {bundleTask.failure_reason}
              </div>
            ) : (
              <p className='text-xs text-muted-foreground'>
                {t('detailPage.noExecutionSteps')}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
