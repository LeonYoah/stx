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

import {
  type KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {
  Activity,
  ArrowLeft,
  Download,
  ExternalLink,
  FileText,
  Loader2,
  Package,
} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import type {
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionFindingSeverity,
  DiagnosticsInspectionReport,
  DiagnosticsTask,
  DiagnosticsTaskNodeScope,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {WorkspaceHeader} from '@/components/common/layout/WorkspaceHeader';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Skeleton} from '@/components/ui/skeleton';
import {Switch} from '@/components/ui/switch';
import {localizeDiagnosticsText} from './text-utils';
import {
  InspectionFindingCard,
  InspectionZeroState,
} from './InspectionEvidenceView';

interface InspectionDetailPageProps {
  inspectionId: number;
}

function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

function getStatusVariant(
  status: string,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (status) {
    case 'completed':
    case 'succeeded':
      return 'default';
    case 'failed':
      return 'destructive';
    case 'running':
      return 'secondary';
    default:
      return 'outline';
  }
}

function getFindingSeverityScore(
  severity: DiagnosticsInspectionFindingSeverity,
): number {
  switch (severity) {
    case 'critical':
      return 3;
    case 'warning':
      return 2;
    case 'info':
    default:
      return 1;
  }
}

function formatNodeOrigin(options: {
  nodeId?: number | null;
  hostId?: number | null;
  hostName?: string | null;
  hostIp?: string | null;
}): string {
  const parts: string[] = [];
  if (options.hostName?.trim()) {
    parts.push(options.hostName.trim());
  } else if (options.hostIp?.trim()) {
    parts.push(options.hostIp.trim());
  } else if (options.hostId) {
    parts.push(`#${options.hostId}`);
  }
  if (options.nodeId) {
    parts.push(`node #${options.nodeId}`);
  }
  return parts.length > 0 ? parts.join(' · ') : '-';
}

const DEFAULT_BUNDLE_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: false,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
};

export default function InspectionDetailPage({
  inspectionId,
}: InspectionDetailPageProps) {
  const t = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');
  const getStatusLabel = useCallback(
    (status: string): string => {
      switch (status) {
        case 'pending':
          return t('inspections.status.pending');
        case 'running':
          return t('inspections.status.running');
        case 'completed':
        case 'succeeded':
          return t('inspections.status.completed');
        case 'failed':
        case 'cancelled':
          return t('inspections.status.failed');
        default:
          return status;
      }
    },
    [t],
  );
  const getTriggerSourceLabel = useCallback(
    (source: string): string => {
      switch (source) {
        case 'manual':
          return t('inspections.trigger.manual');
        case 'auto':
          return t('inspections.detailPage.autoTrigger');
        case 'cluster_detail':
          return t('inspections.trigger.cluster_detail');
        case 'diagnostics_workspace':
          return t('inspections.trigger.diagnostics_workspace');
        default:
          return source;
      }
    },
    [t],
  );
  const [loading, setLoading] = useState(true);
  const [report, setReport] = useState<DiagnosticsInspectionReport | null>(
    null,
  );
  const [findings, setFindings] = useState<DiagnosticsInspectionFinding[]>([]);
  const [creatingBundle, setCreatingBundle] = useState(false);
  const [bundleTask, setBundleTask] = useState<DiagnosticsTask | null>(null);
  const [pollingBundle, setPollingBundle] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [bundleOptions, setBundleOptions] = useState<DiagnosticsTaskOptions>(
    DEFAULT_BUNDLE_OPTIONS,
  );
  const [nodeScope, setNodeScope] = useState<DiagnosticsTaskNodeScope>('all');
  const [bundleLookbackMinutes, setBundleLookbackMinutes] =
    useState<number>(30);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [execLogDialogOpen, setExecLogDialogOpen] = useState(false);

  const pollBundleTask = useCallback(async (taskId: number) => {
    const result = await services.diagnostics.getTaskSafe(taskId);
    if (!result.success || !result.data) {
      return;
    }
    setBundleTask(result.data);
    const status = result.data.status;
    if (
      status === 'succeeded' ||
      status === 'failed' ||
      status === 'cancelled'
    ) {
      setPollingBundle(false);
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    }
  }, []);

  const loadDetail = useCallback(async () => {
    setLoading(true);
    try {
      const result =
        await services.diagnostics.getInspectionReportDetailSafe(inspectionId);
      if (!result.success || !result.data) {
        toast.error(result.error || t('inspections.loadDetailError'));
        setReport(null);
        setFindings([]);
        setBundleTask(null);
        return;
      }
      setReport(result.data.report);
      setFindings(result.data.findings || []);
      const related = result.data.related_diagnostic_task;
      if (related) {
        setBundleTask(related);
        const status = related.status;
        if (
          status !== 'succeeded' &&
          status !== 'failed' &&
          status !== 'cancelled'
        ) {
          if (pollTimerRef.current) {
            clearInterval(pollTimerRef.current);
            pollTimerRef.current = null;
          }
          setPollingBundle(true);
          const taskId = related.id;
          pollTimerRef.current = setInterval(() => {
            void pollBundleTask(taskId);
          }, 3000);
        }
      } else {
        setBundleTask(null);
      }
    } finally {
      setLoading(false);
    }
  }, [inspectionId, pollBundleTask, t]);

  useEffect(() => {
    void loadDetail();
  }, [loadDetail]);

  // Sort findings by severity: critical > warning > info
  const sortedFindings = useMemo(
    () =>
      [...findings].sort(
        (a, b) =>
          getFindingSeverityScore(b.severity) -
          getFindingSeverityScore(a.severity),
      ),
    [findings],
  );

  useEffect(() => {
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
      }
    };
  }, []);

  const handleConfirmAndCreateBundle = useCallback(() => {
    const base = report?.lookback_minutes || 30;
    setBundleLookbackMinutes(base < 5 || base > 1440 ? 30 : base);
    setConfirmDialogOpen(true);
  }, [report]);

  const handleCreateBundle = useCallback(async () => {
    if (!report || creatingBundle) {
      return;
    }
    if (bundleLookbackMinutes < 5 || bundleLookbackMinutes > 1440) {
      toast.error(t('inspections.lookbackRangeError'));
      return;
    }
    const firstFinding =
      findings.find((f) => f.severity === 'critical') ??
      findings.find((f) => f.severity === 'warning') ??
      findings[0];

    setCreatingBundle(true);
    setConfirmDialogOpen(false);
    try {
      const payloadScope: DiagnosticsTaskNodeScope = nodeScope || 'all';
      const result = await services.diagnostics.createTaskSafe({
        cluster_id: report.cluster_id,
        trigger_source: firstFinding ? 'inspection_finding' : 'manual',
        source_ref: firstFinding
          ? {
              inspection_report_id: report.id,
              inspection_finding_id: firstFinding.id,
            }
          : undefined,
        node_scope: payloadScope,
        options: bundleOptions,
        lookback_minutes: bundleLookbackMinutes,
        auto_start: true,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('inspections.followUp.createTaskError'));
        return;
      }
      toast.success(t('inspections.followUp.createTaskSuccess'));
      setBundleTask(result.data);
      setPollingBundle(true);
      const taskId = result.data.id;
      pollTimerRef.current = setInterval(() => {
        void pollBundleTask(taskId);
      }, 3000);
    } finally {
      setCreatingBundle(false);
    }
  }, [
    bundleLookbackMinutes,
    bundleOptions,
    creatingBundle,
    findings,
    nodeScope,
    pollBundleTask,
    report,
    t,
  ]);

  const handleBundleInputKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key !== 'Enter') {
        return;
      }
      event.preventDefault();
      void handleCreateBundle();
    },
    [handleCreateBundle],
  );

  if (loading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-10 w-48' />
        <Skeleton className='h-32 w-full' />
        <Skeleton className='h-64 w-full' />
      </div>
    );
  }

  if (!report) {
    return (
      <div className='space-y-4'>
        <Button asChild variant='ghost'>
          <Link href='/diagnostics?tab=inspections'>
            <ArrowLeft className='mr-2 h-4 w-4' />
            {t('inspections.detailPage.backToList')}
          </Link>
        </Button>
        <Card>
          <CardContent className='py-8 text-center text-muted-foreground'>
            {t('inspections.detailPage.notFound')}
          </CardContent>
        </Card>
      </div>
    );
  }

  const isCompleted = report.status === 'completed';
  const hasFindings = findings.length > 0;
  const findingCount = Math.max(report.finding_total, sortedFindings.length);

  // 页面只强调本次有依据的发现，不把零发现解释为集群整体健康。
  // Focus on findings backed by this run; zero findings do not imply overall cluster health.
  return (
    <div className='space-y-8 pb-28'>
      <WorkspaceHeader
        icon={<Activity aria-hidden='true' />}
        title={t('inspections.detailPage.title')}
        actions={
          <Button asChild variant='outline' size='sm'>
            <Link href='/diagnostics?tab=inspections'>
              <ArrowLeft aria-hidden='true' className='mr-2 h-4 w-4' />
              {t('inspections.detailPage.backToList')}
            </Link>
          </Button>
        }
      />
      <header className='border-b border-border/70 pb-7'>
        <div className='flex justify-end'>
          <span className='font-mono text-xs tabular-nums text-muted-foreground'>
            STX / {t('inspections.evidence.reportNumber', {id: report.id})}
          </span>
        </div>
        <div className='mt-8 flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between'>
          <div className='min-w-0'>
            <div className='mb-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
              <Badge variant={getStatusVariant(report.status)}>
                {getStatusLabel(report.status)}
              </Badge>
              <span>{getTriggerSourceLabel(report.trigger_source)}</span>
            </div>
            <h2 className='font-serif text-4xl leading-tight tracking-tight text-foreground sm:text-5xl'>
              {t('inspections.evidence.runTitle')}
            </h2>
            <p className='mt-3 break-words text-sm text-muted-foreground'>
              {report.cluster_name ||
                t('inspections.detailPage.clusterFallback', {
                  clusterId: report.cluster_id,
                })}
            </p>
          </div>
          {isCompleted && findingCount > 0 && (
            <div className='flex shrink-0 items-baseline gap-2 sm:flex-col sm:items-end sm:gap-0'>
              <span className='font-serif text-5xl leading-none tabular-nums text-foreground sm:text-6xl'>
                {findingCount}
              </span>
              <span className='text-xs text-muted-foreground'>
                {t('inspections.evidence.attentionCount', {
                  count: findingCount,
                })}
              </span>
            </div>
          )}
        </div>
        <div className='mt-8 flex flex-wrap gap-x-8 gap-y-2 border-t border-border/60 pt-4 text-xs text-muted-foreground'>
          {report.finished_at && (
            <span>
              {t('inspections.evidence.checkTime')} ·{' '}
              {formatDateTime(report.finished_at)}
            </span>
          )}
          <span>
            {t('inspections.detailPage.lookbackMinutes')}
            {t('inspections.lookbackValue', {
              minutes: report.lookback_minutes || 30,
            })}
          </span>
          <details className='group relative basis-full'>
            <summary className='cursor-pointer list-none font-medium text-foreground underline decoration-border underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary [&::-webkit-details-marker]:hidden'>
              {t('inspections.evidence.metadata')}{' '}
              <span aria-hidden='true'>↗</span>
            </summary>
            <div className='mt-3 grid gap-x-8 gap-y-2 text-xs text-muted-foreground sm:grid-cols-2'>
              <span>
                {t('inspections.detailPage.createdAt')}
                {formatDateTime(report.created_at)}
              </span>
              <span>
                {t('inspections.detailPage.errorThreshold')}
                {t('inspections.errorThresholdValue', {
                  count: report.error_threshold || 1,
                })}
              </span>
              {report.requested_by && (
                <span>
                  {t('inspections.requestedBy')} · {report.requested_by}
                </span>
              )}
              {report.trigger_source === 'auto' &&
                report.auto_trigger_reason && (
                  <span>
                    {t('inspections.detailPage.autoTriggerReason')}{' '}
                    {report.auto_trigger_reason}
                  </span>
                )}
              {report.summary && sortedFindings.length > 0 && (
                <span className='sm:col-span-2'>
                  {t('inspections.evidence.sourceSummary')} ·{' '}
                  {localizeDiagnosticsText(report.summary)}
                </span>
              )}
            </div>
          </details>
        </div>
        {report.error_message && (
          <p
            role='alert'
            className='mt-5 border-l-2 border-destructive pl-3 text-sm text-destructive'
          >
            {report.error_message}
          </p>
        )}
      </header>

      <section
        aria-label={t('inspections.evidence.attentionTitle')}
        className='space-y-5'
      >
        {sortedFindings.length > 0 ? (
          <>
            <div className='flex flex-wrap items-baseline justify-between gap-3 border-b border-border/70 pb-3'>
              <h2
                id='inspection-evidence-title'
                className='font-serif text-2xl text-foreground sm:text-3xl'
              >
                {t('inspections.evidence.attentionTitle')}
              </h2>
              <div className='flex flex-wrap gap-3 text-xs text-muted-foreground'>
                {report.critical_count > 0 && (
                  <span>
                    {t('inspections.severity.critical')} {report.critical_count}
                  </span>
                )}
                {report.warning_count > 0 && (
                  <span>
                    {t('inspections.severity.warning')} {report.warning_count}
                  </span>
                )}
                {report.info_count > 0 && (
                  <span>
                    {t('inspections.severity.info')} {report.info_count}
                  </span>
                )}
              </div>
            </div>
            <div className='space-y-3'>
              {sortedFindings.map((finding, index) => (
                <InspectionFindingCard
                  key={finding.id}
                  finding={finding}
                  index={index}
                  origin={formatNodeOrigin({
                    nodeId: finding.related_node_id,
                    hostId: finding.related_host_id,
                    hostName: finding.related_host_name,
                    hostIp: finding.related_host_ip,
                  })}
                />
              ))}
            </div>
          </>
        ) : isCompleted && findingCount === 0 && !report.error_message ? (
          <InspectionZeroState />
        ) : isCompleted ? (
          <div className='border border-border/70 bg-card px-6 py-10 text-sm text-muted-foreground'>
            {t('inspections.evidence.unavailable')}
          </div>
        ) : (
          <div className='border border-border/70 bg-card px-6 py-10'>
            <h2
              id='inspection-evidence-title'
              className='font-semibold text-foreground'
            >
              {t('inspections.evidence.pendingTitle')}
            </h2>
            <p className='mt-2 text-sm text-muted-foreground'>
              {report.status === 'failed'
                ? t('inspections.evidence.incompleteDescription')
                : t('inspections.evidence.pendingDescription')}
            </p>
          </div>
        )}
      </section>

      {/* Diagnostic Bundle Section */}
      {isCompleted && (hasFindings || bundleTask) ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('inspections.detailPage.bundleTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            {bundleTask ? (
              <div className='space-y-3'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant={getStatusVariant(bundleTask.status)}>
                    {getStatusLabel(bundleTask.status)}
                  </Badge>
                  <Badge variant='outline'>
                    {t('inspections.taskLabel', {id: bundleTask.id})}
                  </Badge>
                  {pollingBundle ? (
                    <span className='flex items-center gap-1 text-xs text-muted-foreground'>
                      <Loader2 className='h-3 w-3 animate-spin' />
                      {t('inspections.refreshingLabel')}
                    </span>
                  ) : null}
                </div>

                <div className='flex flex-wrap gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setExecLogDialogOpen(true)}
                  >
                    <FileText className='mr-2 h-4 w-4' />
                    {t('inspections.detailPage.viewExecutionLogs')}
                  </Button>
                  {bundleTask.status === 'succeeded' ? (
                    <>
                      <Button asChild variant='outline' size='sm'>
                        <a
                          href={services.diagnostics.getTaskHTMLUrl(
                            bundleTask.id,
                          )}
                          target='_blank'
                          rel='noopener noreferrer'
                        >
                          <ExternalLink className='mr-2 h-4 w-4' />
                          {t('inspections.detailPage.previewReport')}
                        </a>
                      </Button>
                      <Button asChild variant='outline' size='sm'>
                        <a
                          href={services.diagnostics.getTaskBundleUrl(
                            bundleTask.id,
                          )}
                          download
                        >
                          <Download className='mr-2 h-4 w-4' />
                          {t('inspections.detailPage.downloadBundle')}
                        </a>
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={handleConfirmAndCreateBundle}
                        disabled={creatingBundle}
                      >
                        {creatingBundle ? (
                          <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                        ) : (
                          <Package className='mr-2 h-4 w-4' />
                        )}
                        {t('inspections.detailPage.regenerate')}
                      </Button>
                    </>
                  ) : null}
                  {bundleTask.status === 'failed' ? (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={handleConfirmAndCreateBundle}
                      disabled={creatingBundle}
                    >
                      {creatingBundle ? (
                        <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                      ) : (
                        <Package className='mr-2 h-4 w-4' />
                      )}
                      {t('inspections.detailPage.regenerate')}
                    </Button>
                  ) : null}
                </div>
              </div>
            ) : hasFindings ? (
              <div>
                <Button
                  onClick={handleConfirmAndCreateBundle}
                  disabled={creatingBundle}
                >
                  {creatingBundle ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <Package className='mr-2 h-4 w-4' />
                  )}
                  {t('inspections.followUp.generateBundle')}
                </Button>
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {/* 生成确认弹窗 */}
      <Dialog open={confirmDialogOpen} onOpenChange={setConfirmDialogOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>
              {t('inspections.detailPage.confirmBundleTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('inspections.detailPage.confirmBundleDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label htmlFor='bundle-lookback'>
                {t('inspections.lookbackLabel')}
              </Label>
              <Input
                id='bundle-lookback'
                type='number'
                min={5}
                max={1440}
                step={5}
                value={bundleLookbackMinutes}
                onChange={(event) =>
                  setBundleLookbackMinutes(
                    Number.parseInt(event.target.value, 10) || 30,
                  )
                }
                onKeyDown={handleBundleInputKeyDown}
              />
              <p className='text-xs text-muted-foreground'>
                {t('inspections.detailPage.bundleLookbackHint')}
              </p>
            </div>
            <div className='space-y-3'>
              <div className='flex items-center justify-between rounded-lg border p-3'>
                <div>
                  <div className='font-medium'>
                    {t('inspections.detailPage.includeThreadDump')}
                  </div>
                  <div className='text-xs text-muted-foreground'>
                    {t('inspections.detailPage.includeThreadDumpHint')}
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
                />
              </div>
              <div className='flex items-center justify-between rounded-lg border p-3'>
                <div>
                  <div className='font-medium'>
                    {t('inspections.detailPage.includeJVMDump')}
                  </div>
                  <div className='text-xs text-muted-foreground'>
                    {t('inspections.detailPage.includeJVMDumpHint')}
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
                />
              </div>
              <div className='flex flex-wrap gap-2 text-xs'>
                <Button
                  type='button'
                  variant={nodeScope === 'all' ? 'default' : 'outline'}
                  size='sm'
                  onClick={() => setNodeScope('all')}
                >
                  {t('inspections.detailPage.allNodes')}
                </Button>
                <Button
                  type='button'
                  variant={nodeScope === 'related' ? 'default' : 'outline'}
                  size='sm'
                  onClick={() => setNodeScope('related')}
                >
                  {t('inspections.detailPage.relatedNodes')}
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setConfirmDialogOpen(false)}
            >
              {commonT('cancel')}
            </Button>
            <Button
              onClick={() => void handleCreateBundle()}
              disabled={creatingBundle}
            >
              {creatingBundle ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : null}
              {t('inspections.detailPage.confirmCreate')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 查看执行日志弹窗 */}
      <Dialog open={execLogDialogOpen} onOpenChange={setExecLogDialogOpen}>
        <DialogContent className='max-h-[85vh] overflow-hidden flex flex-col sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {t('inspections.detailPage.executionLogsTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('inspections.detailPage.executionLogsDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className='flex-1 overflow-y-auto space-y-3 py-2'>
            {bundleTask?.steps?.length ? (
              bundleTask.steps.map((step) => (
                <div key={step.id} className='rounded-lg border p-3 space-y-1'>
                  <div className='flex items-center gap-2'>
                    <Badge
                      variant={getStatusVariant(step.status)}
                      className='text-xs'
                    >
                      {getStatusLabel(step.status)}
                    </Badge>
                    <span className='font-mono text-xs text-muted-foreground'>
                      {step.code}
                    </span>
                  </div>
                  <div className='text-sm'>
                    {localizeDiagnosticsText(step.title) || step.description}
                  </div>
                  {step.error || step.message ? (
                    <div className='rounded bg-muted/60 px-2 py-1.5 text-xs text-muted-foreground'>
                      {step.error || step.message}
                    </div>
                  ) : null}
                </div>
              ))
            ) : bundleTask?.failure_reason ? (
              <div className='rounded-md border border-destructive/20 bg-destructive/5 p-3 text-sm text-destructive'>
                {bundleTask.failure_reason}
              </div>
            ) : (
              <p className='text-sm text-muted-foreground'>
                {t('inspections.detailPage.noExecutionSteps')}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
