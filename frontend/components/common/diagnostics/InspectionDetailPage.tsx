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

import {type KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {
  ArrowLeft,
  Check,
  Copy,
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

// 获取巡检发现严重程度的现代样式类（高对比度、暗黑模式适配）
// Get modern style class for inspection finding severity (high contrast, dark mode compatible)
function getSeverityBadgeClass(severity: DiagnosticsInspectionFindingSeverity): string {
  switch (severity) {
    case 'critical':
      return 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20 font-semibold';
    case 'warning':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 font-medium';
    case 'info':
    default:
      return 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20';
  }
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
  const getSeverityLabel = useCallback(
    (severity: DiagnosticsInspectionFindingSeverity): string => {
      switch (severity) {
        case 'critical':
          return t('inspections.severity.critical');
        case 'warning':
          return t('inspections.severity.warning');
        case 'info':
          return t('inspections.severity.info');
        default:
          return severity;
      }
    },
    [t],
  );
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
  const [report, setReport] = useState<DiagnosticsInspectionReport | null>(null);
  const [findings, setFindings] = useState<DiagnosticsInspectionFinding[]>([]);
  const [creatingBundle, setCreatingBundle] = useState(false);
  const [bundleTask, setBundleTask] = useState<DiagnosticsTask | null>(null);
  const [pollingBundle, setPollingBundle] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [bundleOptions, setBundleOptions] =
    useState<DiagnosticsTaskOptions>(DEFAULT_BUNDLE_OPTIONS);
  const [nodeScope, setNodeScope] =
    useState<DiagnosticsTaskNodeScope>('all');
  const [bundleLookbackMinutes, setBundleLookbackMinutes] =
    useState<number>(30);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [execLogDialogOpen, setExecLogDialogOpen] = useState(false);

  const pollBundleTask = useCallback(
    async (taskId: number) => {
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
    },
    [],
  );

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
    setBundleLookbackMinutes(
      base < 5 || base > 1440 ? 30 : base,
    );
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

  return (
    <div className='space-y-4'>
      {/* Header */}
      <div className='flex items-center gap-3'>
        <Button asChild variant='ghost' size='sm'>
          <Link href='/diagnostics?tab=inspections'>
            <ArrowLeft className='mr-2 h-4 w-4' />
            {t('inspections.detailPage.backToList')}
          </Link>
        </Button>
        <h1 className='text-2xl font-bold tracking-tight'>
          {t('inspections.detailPage.title')}
        </h1>
        <Badge variant='outline'>#{report.id}</Badge>
      </div>

      {/* Status Banner */}
      <Card>
        <CardContent className='space-y-3 pt-6'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant={getStatusVariant(report.status)}>
              {getStatusLabel(report.status)}
            </Badge>
            <Badge variant='outline'>
              {getTriggerSourceLabel(report.trigger_source)}
            </Badge>
            {report.cluster_name ? (
              <Badge variant='outline'>{report.cluster_name}</Badge>
            ) : (
              <Badge variant='outline'>
                {t('inspections.detailPage.clusterFallback', {
                  clusterId: report.cluster_id,
                })}
              </Badge>
            )}
          </div>
          {report.trigger_source === 'auto' && report.auto_trigger_reason ? (
            <div className='rounded-md border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400'>
              <span className='font-medium'>{t('inspections.detailPage.autoTriggerReason')} </span>
              {report.auto_trigger_reason}
            </div>
          ) : null}
          <div className='grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-5'>
            <div>
              <span className='text-muted-foreground'>
                {t('inspections.detailPage.createdAt')}
              </span>
              {formatDateTime(report.created_at)}
            </div>
            <div>
              <span className='text-muted-foreground'>
                {t('inspections.detailPage.finishedAt')}
              </span>
              {formatDateTime(report.finished_at)}
            </div>
            <div>
              <span className='text-muted-foreground'>
                {t('inspections.detailPage.lookbackMinutes')}
              </span>
              {t('inspections.lookbackValue', {
                minutes: report.lookback_minutes || 30,
              })}
            </div>
            <div>
              <span className='text-muted-foreground'>
                {t('inspections.detailPage.errorThreshold')}
              </span>
              {t('inspections.errorThresholdValue', {
                count: report.error_threshold || 1,
              })}
            </div>
            <div>
              <span className='text-muted-foreground'>
                {t('inspections.requestedBy')}：
              </span>
              {report.requested_by || '-'}
            </div>
          </div>
          {report.summary ? (
            <div className='text-sm'>
              {localizeDiagnosticsText(report.summary)}
            </div>
          ) : null}
          {report.error_message ? (
            <div className='rounded-md border border-destructive/20 bg-destructive/5 p-3 text-sm text-destructive'>
              {report.error_message}
            </div>
          ) : null}
          {/* 巡检发现统计指标胶囊 */}
          {/* Inspection findings count badges */}
          <div className='flex flex-wrap items-center gap-2 pt-1 border-t'>
            <span className='text-xs text-muted-foreground mr-1'>
              {t('inspections.countSummary')}：
            </span>
            <Badge variant='outline' className='text-xs font-mono font-medium'>
              共 {report.finding_total} 项
            </Badge>
            {report.critical_count > 0 && (
              <Badge variant='outline' className='border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400 text-xs font-mono font-medium'>
                {t('inspections.severity.critical')} {report.critical_count}
              </Badge>
            )}
            {report.warning_count > 0 && (
              <Badge variant='outline' className='border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-xs font-mono font-medium'>
                {t('inspections.severity.warning')} {report.warning_count}
              </Badge>
            )}
            {report.info_count > 0 && (
              <Badge variant='outline' className='border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400 text-xs font-mono font-medium'>
                {t('inspections.severity.info')} {report.info_count}
              </Badge>
            )}
            {report.finding_total === 0 && (
              <Badge variant='outline' className='border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-xs font-medium'>
                健康正常
              </Badge>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Findings Section */}
      <Card>
        <CardHeader>
          <CardTitle>{t('inspections.findingsTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
          {sortedFindings.length === 0 ? (
            <div className='flex items-center justify-center rounded-lg border border-dashed p-8 text-sm text-muted-foreground'>
              {t('inspections.noFindings')}
            </div>
          ) : (
            <div className='space-y-4'>
              {sortedFindings.map((finding) => (
                <div
                  key={finding.id}
                  className='rounded-lg border p-4 space-y-3'
                >
                  <div className='flex flex-wrap items-center gap-2'>
                    <Badge
                      variant='outline'
                      className={getSeverityBadgeClass(finding.severity)}
                    >
                      {getSeverityLabel(finding.severity)}
                    </Badge>
                    <Badge variant='outline'>{finding.category}</Badge>
                    <Badge variant='outline'>{finding.check_code}</Badge>
                  </div>
                  <div className='font-medium'>
                    {localizeDiagnosticsText(
                      finding.check_name || finding.summary,
                    )}
                  </div>
                  <div className='text-sm text-muted-foreground'>
                    {localizeDiagnosticsText(finding.summary)}
                  </div>
                  {finding.evidence_summary ? (
                    <div className='rounded-md bg-muted/40 p-3 text-sm text-muted-foreground'>
                      {localizeDiagnosticsText(finding.evidence_summary)}
                    </div>
                  ) : null}
                  {/* 排查与修复建议（支持一键复制） */}
                  {/* Investigation and remediation recommendation (supports one-click copy) */}
                  {finding.recommendation ? (
                    <div className='rounded-md border border-primary/20 bg-primary/5 p-3 text-xs space-y-1.5'>
                      <div className='flex items-center justify-between font-medium text-foreground'>
                        <span>{t('inspections.detailPage.recommendation')}</span>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-6 text-xs px-1.5 gap-1 text-muted-foreground hover:text-foreground'
                          onClick={() => {
                            navigator.clipboard.writeText(
                              localizeDiagnosticsText(finding.recommendation) || '',
                            );
                            toast.success('排查与修复建议已复制');
                          }}
                        >
                          <Copy className='h-3 w-3' />
                          <span>复制建议</span>
                        </Button>
                      </div>
                      <div className='text-muted-foreground leading-relaxed'>
                        {localizeDiagnosticsText(finding.recommendation)}
                      </div>
                    </div>
                  ) : null}
                  <div className='text-xs text-muted-foreground'>
                    {t('inspections.nodeLabel')}：
                    {formatNodeOrigin({
                      nodeId: finding.related_node_id,
                      hostId: finding.related_host_id,
                      hostName: finding.related_host_name,
                      hostIp: finding.related_host_ip,
                    })}
                  </div>
                  {finding.related_error_group_id > 0 ? (
                    <Button asChild size='sm' variant='outline'>
                      <Link
                        href={`/diagnostics?tab=errors&cluster_id=${finding.cluster_id}&group_id=${finding.related_error_group_id}&source=inspection-finding`}
                      >
                        {t('inspections.actions.viewErrorGroup')} &rarr;
                      </Link>
                    </Button>
                  ) : null}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Diagnostic Bundle Section */}
      {isCompleted ? (
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
              <div className='space-y-3'>
                <p className='text-sm text-muted-foreground'>
                  {t('inspections.detailPage.hasFindingsHint', {
                    count: findings.length,
                  })}
                </p>
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
            ) : (
              <p className='text-sm text-muted-foreground'>
                {t('inspections.detailPage.noBundleNeeded')}
              </p>
            )}
          </CardContent>
        </Card>
      ) : null}

      {/* 生成确认弹窗 */}
      <Dialog open={confirmDialogOpen} onOpenChange={setConfirmDialogOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('inspections.detailPage.confirmBundleTitle')}</DialogTitle>
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
              {t('common.cancel')}
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
            <DialogTitle>{t('inspections.detailPage.executionLogsTitle')}</DialogTitle>
            <DialogDescription>
              {t('inspections.detailPage.executionLogsDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className='flex-1 overflow-y-auto space-y-3 py-2'>
            {bundleTask?.steps?.length ? (
              bundleTask.steps.map((step) => (
                <div
                  key={step.id}
                  className='rounded-lg border p-3 space-y-1'
                >
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
                  {(step.error || step.message) ? (
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
