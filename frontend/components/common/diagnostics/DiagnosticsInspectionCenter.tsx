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
  Check,
  ClipboardCheck,
  Clock,
  Copy,
  Download,
  ExternalLink,
  FileText,
  Loader2,
  Package,
  Play,
  Plus,
  RefreshCw,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionFindingSeverity,
  DiagnosticsInspectionReport,
  DiagnosticsInspectionReportStatus,
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
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Skeleton} from '@/components/ui/skeleton';
import {Switch} from '@/components/ui/switch';
import {localizeDiagnosticsText} from './text-utils';

const DEFAULT_BUNDLE_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: true,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
};

type DiagnosticsInspectionCenterProps = {
  clusterId?: number;
  clusterName?: string;
  reportId?: number;
  onSelectReport?: (reportId: number | null) => void;
};

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

function getSeverityVariant(
  severity: DiagnosticsInspectionFindingSeverity,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (severity) {
    case 'critical':
      return 'destructive';
    case 'warning':
      return 'secondary';
    case 'info':
    default:
      return 'outline';
  }
}

// 获取巡检发现严重程度的现代样式类（高对比度、暗黑模式适配）
// Get modern style class for inspection finding severity (high contrast, dark mode compatible)
function getFindingSeverityBadgeClass(severity: DiagnosticsInspectionFindingSeverity): string {
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

export function DiagnosticsInspectionCenter({
  clusterId,
  clusterName,
  reportId,
  onSelectReport,
}: DiagnosticsInspectionCenterProps) {
  const t = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');
  const getTaskStatusLabel = useCallback(
    (status: string): string => {
      switch (status) {
        case 'pending':
          return t('tasks.status.pending');
        case 'ready':
          return t('tasks.status.ready');
        case 'running':
          return t('tasks.status.running');
        case 'succeeded':
          return t('inspections.status.completed');
        case 'failed':
          return t('tasks.status.failed');
        case 'skipped':
          return t('tasks.status.skipped');
        case 'cancelled':
          return t('tasks.status.cancelled');
        default:
          return status;
      }
    },
    [t],
  );

  const [statusFilter, setStatusFilter] = useState<
    'all' | DiagnosticsInspectionReportStatus
  >('all');
  const [severityFilter, setSeverityFilter] = useState<
    'all' | DiagnosticsInspectionFindingSeverity
  >('all');
  const [page, setPage] = useState(1);
  const [loadingReports, setLoadingReports] = useState(true);
  const [startingInspection, setStartingInspection] = useState(false);
  const [startInspectionDialogOpen, setStartInspectionDialogOpen] =
    useState(false);
  const [lookbackMinutes, setLookbackMinutes] = useState(30);
  const [errorThreshold, setErrorThreshold] = useState(1);
  const [reports, setReports] = useState<DiagnosticsInspectionReport[]>([]);
  const [reportTotal, setReportTotal] = useState(0);
  const [selectedReportId, setSelectedReportId] = useState<number | null>(
    reportId ?? null,
  );
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedReport, setSelectedReport] =
    useState<DiagnosticsInspectionReport | null>(null);
  const [findings, setFindings] = useState<DiagnosticsInspectionFinding[]>([]);
  const [bundleTask, setBundleTask] = useState<DiagnosticsTask | null>(null);
  const [creatingBundle, setCreatingBundle] = useState(false);
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

  const loadReports = useCallback(async () => {
    setLoadingReports(true);
    try {
      const result = await services.diagnostics.getInspectionReportsSafe({
        cluster_id: clusterId,
        status: statusFilter !== 'all' ? statusFilter : undefined,
        severity: severityFilter !== 'all' ? severityFilter : undefined,
        page,
        page_size: 20,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('inspections.loadReportsError'));
        setReports([]);
        setReportTotal(0);
        return;
      }
      setReports(result.data.items || []);
      setReportTotal(result.data.total || 0);
    } finally {
      setLoadingReports(false);
    }
  }, [clusterId, page, severityFilter, statusFilter, t]);

  const loadDetail = useCallback(
    async (nextReportId: number) => {
      setLoadingDetail(true);
      try {
        const result =
          await services.diagnostics.getInspectionReportDetailSafe(
            nextReportId,
          );
        if (!result.success || !result.data) {
          toast.error(result.error || t('inspections.loadDetailError'));
          setSelectedReport(null);
          setFindings([]);
          setBundleTask(null);
          return;
        }
        setSelectedReport(result.data.report);
        setFindings(result.data.findings || []);
        setBundleTask(result.data.related_diagnostic_task || null);
      } finally {
        setLoadingDetail(false);
      }
    },
    [t],
  );

  useEffect(() => {
    void loadReports();
  }, [loadReports]);

  useEffect(() => {
    if (reports.length === 0) {
      setSelectedReportId(null);
      setSelectedReport(null);
      setFindings([]);
      setBundleTask(null);
      return;
    }
    if (
      !selectedReportId ||
      !reports.some((item) => item.id === selectedReportId)
    ) {
      setSelectedReportId(reports[0].id);
      onSelectReport?.(reports[0].id);
    }
  }, [onSelectReport, reports, selectedReportId]);

  useEffect(() => {
    if (
      !selectedReportId ||
      !reports.some((item) => item.id === selectedReportId)
    ) {
      return;
    }
    void loadDetail(selectedReportId);
  }, [loadDetail, reports, selectedReportId]);

  useEffect(() => {
    setSelectedReportId(reportId ?? null);
  }, [reportId]);

  const groupedFindings = useMemo(
    () =>
      findings.reduce<
        Record<
          DiagnosticsInspectionFindingSeverity,
          DiagnosticsInspectionFinding[]
        >
      >(
        (accumulator, finding) => {
          const bucket = accumulator[finding.severity] || [];
          bucket.push(finding);
          accumulator[finding.severity] = bucket;
          return accumulator;
        },
        {
          critical: [],
          warning: [],
          info: [],
        },
      ),
    [findings],
  );
  const hasFindings = findings.length > 0;
  const totalPages = Math.max(1, Math.ceil(reportTotal / 20));

  const pollBundleTask = useCallback(async (taskId: number) => {
    const result = await services.diagnostics.getTaskSafe(taskId);
    if (!result.success || !result.data) {
      return;
    }
    setBundleTask(result.data);
    if (['succeeded', 'failed', 'cancelled'].includes(result.data.status)) {
      setPollingBundle(false);
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    }
  }, []);

  useEffect(() => {
    if (!bundleTask || !selectedReportId || bundleTask.id === 0) {
      return;
    }
    const status = bundleTask.status;
    if (['succeeded', 'failed', 'cancelled'].includes(status)) {
      return;
    }
    setPollingBundle(true);
    const taskId = bundleTask.id;
    pollTimerRef.current = setInterval(() => void pollBundleTask(taskId), 3000);
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [bundleTask, pollBundleTask, selectedReportId]);

  const handleConfirmAndCreateBundle = useCallback(() => {
    const base =
      selectedReport?.lookback_minutes || lookbackMinutes || 30;
    setBundleLookbackMinutes(
      base < 5 || base > 1440 ? 30 : base,
    );
    setConfirmDialogOpen(true);
  }, [lookbackMinutes, selectedReport]);

  const handleCreateBundle = useCallback(async () => {
    if (!selectedReport || creatingBundle) {
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
      const result = await services.diagnostics.createTaskSafe({
        cluster_id: selectedReport.cluster_id,
        trigger_source: firstFinding ? 'inspection_finding' : 'manual',
        source_ref: firstFinding
          ? {
              inspection_report_id: selectedReport.id,
              inspection_finding_id: firstFinding.id,
            }
          : undefined,
        node_scope: nodeScope || 'all',
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
    } finally {
      setCreatingBundle(false);
    }
  }, [
    bundleLookbackMinutes,
    bundleOptions,
    creatingBundle,
    findings,
    nodeScope,
    selectedReport,
    t,
  ]);

  const handleStartInspection = useCallback(async () => {
    if (!clusterId || startingInspection) {
      return;
    }
    if (lookbackMinutes < 5 || lookbackMinutes > 1440) {
      toast.error(t('inspections.lookbackRangeError'));
      return;
    }
    if (errorThreshold < 1 || errorThreshold > 1000) {
      toast.error(t('inspections.errorThresholdRangeError'));
      return;
    }
    setStartingInspection(true);
    try {
      const result = await services.diagnostics.startInspectionSafe({
        cluster_id: clusterId,
        trigger_source: 'diagnostics_workspace',
        lookback_minutes: lookbackMinutes,
        error_threshold: errorThreshold,
      });
      if (!result.success || !result.data?.report) {
        toast.error(result.error || t('inspections.startError'));
        return;
      }
      toast.success(t('inspections.startSuccess'));
      setStartInspectionDialogOpen(false);
      setPage(1);
      await loadReports();
      setSelectedReportId(result.data.report.id);
      onSelectReport?.(result.data.report.id);
      setSelectedReport(result.data.report);
      setFindings(result.data.findings || []);
    } finally {
      setStartingInspection(false);
    }
  }, [
    clusterId,
    errorThreshold,
    loadReports,
    lookbackMinutes,
    onSelectReport,
    startingInspection,
    t,
  ]);

  const handleInspectionInputKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key !== 'Enter') {
        return;
      }
      event.preventDefault();
      void handleStartInspection();
    },
    [handleStartInspection],
  );

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

  return (
    <div className='grid gap-4 xl:grid-cols-[minmax(0,1.08fr)_minmax(360px,0.92fr)] xl:items-start'>
      <div className='space-y-4'>
        <Card className='border shadow-xs'>
          <CardHeader className='space-y-4 p-4 sm:p-5'>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <CardTitle className='text-base font-semibold'>{t('inspections.title')}</CardTitle>
                <div className='mt-0.5 text-xs text-muted-foreground'>
                  {clusterId
                    ? t('inspections.clusterScopedHint', {
                        name: clusterName || `#${clusterId}`,
                      })
                    : t('inspections.globalHint')}
                </div>
              </div>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge variant='outline' className='text-xs font-normal'>
                  {t('inspections.matchedReports', {count: reportTotal})}
                </Badge>
                <Button variant='outline' size='sm' onClick={() => void loadReports()}>
                  <RefreshCw className='mr-1.5 h-3.5 w-3.5' />
                  {commonT('refresh')}
                </Button>
                <Button
                  size='sm'
                  onClick={() => setStartInspectionDialogOpen(true)}
                  disabled={!clusterId || startingInspection}
                  className='gap-1.5'
                >
                  {startingInspection ? (
                    <Loader2 className='h-4 w-4 animate-spin' />
                  ) : (
                    <Play className='h-3.5 w-3.5 fill-current' />
                  )}
                  {t('inspections.startInspection')}
                </Button>
              </div>
            </div>

            {/* 报告筛选过滤工具条 / Inspection Reports Filter Toolbar */}
            <div className='flex flex-wrap items-center gap-3 pt-2 border-t'>
              <div className='flex items-center gap-2'>
                <Label className='text-xs text-muted-foreground whitespace-nowrap'>{t('inspections.filters.status')}</Label>
                <Select
                  value={statusFilter}
                  onValueChange={(value) => {
                    setPage(1);
                    setStatusFilter(value as typeof statusFilter);
                  }}
                >
                  <SelectTrigger className='h-8 text-xs w-[130px]'>
                    <SelectValue placeholder={t('inspections.filters.status')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('inspections.filters.allStatuses')}
                    </SelectItem>
                    <SelectItem value='completed'>
                      {t('inspections.status.completed')}
                    </SelectItem>
                    <SelectItem value='failed'>
                      {t('inspections.status.failed')}
                    </SelectItem>
                    <SelectItem value='running'>
                      {t('inspections.status.running')}
                    </SelectItem>
                    <SelectItem value='pending'>
                      {t('inspections.status.pending')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className='flex items-center gap-2'>
                <Label className='text-xs text-muted-foreground whitespace-nowrap'>{t('inspections.filters.severity')}</Label>
                <Select
                  value={severityFilter}
                  onValueChange={(value) => {
                    setPage(1);
                    setSeverityFilter(value as typeof severityFilter);
                  }}
                >
                  <SelectTrigger className='h-8 text-xs w-[130px]'>
                    <SelectValue placeholder={t('inspections.filters.severity')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('inspections.filters.allSeverities')}
                    </SelectItem>
                    <SelectItem value='critical'>
                      {t('inspections.severity.critical')}
                    </SelectItem>
                    <SelectItem value='warning'>
                      {t('inspections.severity.warning')}
                    </SelectItem>
                    <SelectItem value='info'>
                      {t('inspections.severity.info')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {(statusFilter !== 'all' || severityFilter !== 'all') && (
                <Button
                  variant='ghost'
                  size='sm'
                  className='h-8 text-xs text-muted-foreground hover:text-foreground'
                  onClick={() => {
                    setStatusFilter('all');
                    setSeverityFilter('all');
                    setPage(1);
                  }}
                >
                  <X className='mr-1 h-3 w-3' />
                  重置筛选
                </Button>
              )}
            </div>
          </CardHeader>
        </Card>

        <Card className='border shadow-xs flex flex-col overflow-hidden xl:h-[calc(100vh-250px)] xl:min-h-[650px]'>
          <CardHeader className='p-4 pb-3'>
            <CardTitle className='text-base font-semibold'>{t('inspections.listTitle')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-1 min-h-0 flex-col space-y-4 p-4 pt-0'>
            {loadingReports ? (
              <div className='space-y-3'>
                <Skeleton className='h-24 w-full' />
                <Skeleton className='h-24 w-full' />
                <Skeleton className='h-24 w-full' />
              </div>
            ) : reports.length === 0 ? (
              <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                {t('inspections.empty')}
              </div>
            ) : (
              <>
                <ScrollArea className='min-h-0 flex-1 pr-3'>
                  <div className='space-y-3'>
                    {reports.map((report) => (
                      <button
                        key={report.id}
                        type='button'
                        className={cn(
                          'flex w-full flex-col gap-2.5 rounded-lg border p-4 text-left transition-all',
                          selectedReportId === report.id
                            ? 'border-primary bg-primary/5 shadow-xs font-medium'
                            : 'hover:bg-muted/30',
                        )}
                        onClick={() => {
                          setSelectedReportId(report.id);
                          onSelectReport?.(report.id);
                        }}
                      >
                        <div className='flex flex-wrap items-center justify-between gap-2'>
                          <div className='flex items-center gap-2'>
                            <Badge variant={getStatusVariant(report.status)} className='text-xs'>
                              {t(`inspections.status.${report.status}`)}
                            </Badge>
                            <span className='font-mono text-xs text-muted-foreground'>#{report.id}</span>
                          </div>
                          <Badge variant='outline' className='text-[11px] font-normal'>
                            {t(`inspections.trigger.${report.trigger_source}`)}
                          </Badge>
                        </div>
                        <div className='min-w-0 flex-1'>
                          <div
                            className='line-clamp-2 text-sm font-medium leading-snug'
                            title={
                              localizeDiagnosticsText(report.summary) ||
                              t('inspections.summaryFallback')
                            }
                          >
                            {localizeDiagnosticsText(report.summary) ||
                              t('inspections.summaryFallback')}
                          </div>

                          {/* 巡检发现严重程度分布徽章 / Findings Breakdown Badges */}
                          <div className='flex flex-wrap items-center gap-1.5 mt-2'>
                            {report.critical_count > 0 && (
                              <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'>
                                <span className='h-1.5 w-1.5 rounded-full bg-rose-500' />
                                {report.critical_count} 严重
                              </span>
                            )}
                            {report.warning_count > 0 && (
                              <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'>
                                <span className='h-1.5 w-1.5 rounded-full bg-amber-500' />
                                {report.warning_count} 警告
                              </span>
                            )}
                            {report.info_count > 0 && (
                              <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20'>
                                <span className='h-1.5 w-1.5 rounded-full bg-blue-500' />
                                {report.info_count} 提示
                              </span>
                            )}
                            {report.finding_total === 0 && (
                              <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 font-medium'>
                                <Check className='h-3 w-3' />
                                无异常
                              </span>
                            )}
                          </div>
                        </div>

                        <div className='mt-1 flex items-center justify-between text-[11px] text-muted-foreground border-t pt-2'>
                          <span className='flex items-center gap-1'>
                            <Clock className='h-3 w-3' />
                            {t('inspections.lookbackValue', {
                              minutes: report.lookback_minutes || 30,
                            })}
                          </span>
                          <span>
                            {formatDateTime(report.finished_at || report.created_at)}
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                </ScrollArea>

                <div className='flex items-center justify-between text-sm'>
                  <div className='text-muted-foreground'>
                    {t('errors.pageSummary', {page, totalPages})}
                  </div>
                  <div className='flex items-center gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        setPage((current) => Math.max(1, current - 1))
                      }
                      disabled={page <= 1}
                    >
                      {t('errors.previous')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        setPage((current) =>
                          current >= totalPages ? current : current + 1,
                        )
                      }
                      disabled={page >= totalPages}
                    >
                      {t('errors.next')}
                    </Button>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className='border shadow-xs flex flex-col overflow-hidden xl:h-[calc(100vh-250px)] xl:min-h-[650px]'>
        <CardHeader className='p-4 pb-3'>
          <CardTitle className='text-base font-semibold'>{t('inspections.detailTitle')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-1 min-h-0 flex-col space-y-4 p-4 pt-0'>
          {loadingDetail ? (
            <div className='space-y-3'>
              <Skeleton className='h-20 w-full' />
              <Skeleton className='h-48 w-full' />
              <Skeleton className='h-56 w-full' />
            </div>
          ) : !selectedReport ? (
            <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
              {t('inspections.selectReport')}
            </div>
          ) : (
            <>
              <div className='flex min-h-0 flex-1 flex-col space-y-3'>
                {selectedReport.status === 'completed' && hasFindings ? (
                  <div className='rounded-lg border bg-muted/20 p-4 space-y-3'>
                    {bundleTask ? (
                      <>
                        <div className='flex flex-wrap items-center gap-2'>
                          <Badge variant={getStatusVariant(bundleTask.status)}>
                            {getTaskStatusLabel(bundleTask.status)}
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
                                  href={services.diagnostics.getTaskHTMLUrl(bundleTask.id)}
                                  target='_blank'
                                  rel='noopener noreferrer'
                                >
                                  <ExternalLink className='mr-2 h-4 w-4' />
                                  {t('inspections.detailPage.previewReport')}
                                </a>
                              </Button>
                              <Button asChild variant='outline' size='sm'>
                                <a
                                  href={services.diagnostics.getTaskBundleUrl(bundleTask.id)}
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
                          ) : bundleTask.status === 'failed' ? (
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
                      </>
                    ) : (
                      <>
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
                        <p className='text-xs text-muted-foreground'>
                          {t('inspections.followUp.generateHint')}
                        </p>
                      </>
                    )}
                  </div>
                ) : selectedReport.status === 'completed' && !hasFindings ? (
                  <div className='rounded-lg border bg-muted/20 p-4'>
                    <p className='text-sm text-muted-foreground'>
                      {t('inspections.followUp.noFindingsDescription')}
                    </p>
                  </div>
                ) : null}

                <div className='text-sm font-medium'>
                  {t('inspections.findingsTitle')}
                </div>
                {findings.length === 0 ? (
                  <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('inspections.noFindings')}
                  </div>
                ) : (
                  <ScrollArea className='min-h-0 flex-1 pr-3'>
                    <div className='space-y-4'>
                      {(['critical', 'warning', 'info'] as const).map(
                        (severity) => {
                          const severityItems = groupedFindings[severity];
                          if (!severityItems || severityItems.length === 0) {
                            return null;
                          }
                          return (
                            <div key={severity} className='space-y-3'>
                              <div className='flex items-center gap-2'>
                                <Badge
                                  variant='outline'
                                  className={cn('text-xs', getFindingSeverityBadgeClass(severity))}
                                >
                                  {t(`inspections.severity.${severity}`)}
                                </Badge>
                                <span className='text-xs text-muted-foreground font-mono'>
                                  {severityItems.length}
                                </span>
                              </div>
                              {severityItems.map((finding) => (
                                <div
                                  key={finding.id}
                                  className='rounded-lg border p-4 space-y-3 bg-card/60'
                                >
                                  <div className='flex flex-wrap items-center gap-2'>
                                    <Badge
                                      variant='outline'
                                      className={cn('text-xs', getFindingSeverityBadgeClass(finding.severity))}
                                    >
                                      {t(
                                        `inspections.severity.${finding.severity}`,
                                      )}
                                    </Badge>
                                    <Badge variant='outline' className='text-xs'>
                                      {finding.category}
                                    </Badge>
                                    <Badge variant='outline' className='text-xs font-mono'>
                                      {finding.check_code}
                                    </Badge>
                                  </div>
                                  <div className='space-y-2 text-sm'>
                                    <div className='font-medium'>
                                      {localizeDiagnosticsText(
                                        finding.check_name || finding.summary,
                                      )}
                                    </div>
                                    <div className='text-muted-foreground text-xs leading-relaxed'>
                                      {localizeDiagnosticsText(finding.summary)}
                                    </div>
                                    {finding.evidence_summary ? (
                                      <div className='rounded-md bg-muted/40 p-3 text-xs font-mono text-muted-foreground'>
                                        {localizeDiagnosticsText(
                                          finding.evidence_summary,
                                        )}
                                      </div>
                                    ) : null}
                                    {finding.recommendation ? (
                                      <div className='rounded-md border border-primary/20 bg-primary/5 p-3 text-xs space-y-1.5'>
                                        <div className='flex items-center justify-between font-medium text-foreground'>
                                          <span>排查与修复建议</span>
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
                                          {localizeDiagnosticsText(
                                            finding.recommendation,
                                          )}
                                        </div>
                                      </div>
                                    ) : null}
                                    <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground pt-1 border-t'>
                                      <span>
                                        {t('inspections.nodeLabel')}:{' '}
                                        {formatNodeOrigin({
                                          nodeId: finding.related_node_id,
                                          hostId: finding.related_host_id,
                                          hostName: finding.related_host_name,
                                          hostIp: finding.related_host_ip,
                                        })}
                                      </span>
                                      {finding.related_error_group_id > 0 ? (
                                        <Link
                                          href={`/diagnostics?tab=errors&cluster_id=${selectedReport.cluster_id}&group_id=${finding.related_error_group_id}&source=inspection-finding`}
                                          className='text-primary hover:underline'
                                        >
                                          {t('inspections.actions.viewErrorGroup')} &rarr;
                                        </Link>
                                      ) : null}
                                    </div>
                                  </div>
                                </div>
                              ))}
                            </div>
                          );
                        },
                      )}
                    </div>
                  </ScrollArea>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

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
                {t('inspections.detailPage.lookbackMinutes')}
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
                      {getTaskStatusLabel(step.status)}
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

      {/* 发起巡检独立弹窗 / Launch Inspection Modal Dialog */}
      <Dialog open={startInspectionDialogOpen} onOpenChange={setStartInspectionDialogOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <ClipboardCheck className='h-5 w-5 text-primary' />
              <span>{t('inspections.startInspection')}</span>
            </DialogTitle>
            <DialogDescription>
              {clusterId
                ? t('inspections.clusterScopedHint', {
                    name: clusterName || `#${clusterId}`,
                  })
                : t('inspections.globalHint')}
            </DialogDescription>
          </DialogHeader>

          <div className='space-y-4 py-3 text-sm'>
            <div className='space-y-1.5'>
              <Label htmlFor='dialog-inspection-lookback'>
                {t('inspections.lookbackLabel')}
              </Label>
              <div className='flex items-center gap-2'>
                <Input
                  id='dialog-inspection-lookback'
                  type='number'
                  min={5}
                  max={1440}
                  step={5}
                  value={lookbackMinutes}
                  onChange={(event) =>
                    setLookbackMinutes(
                      Number.parseInt(event.target.value, 10) || 30,
                    )
                  }
                  onKeyDown={handleInspectionInputKeyDown}
                  className='h-9'
                />
                <span className='text-xs text-muted-foreground whitespace-nowrap'>分钟</span>
              </div>
              {/* 快捷时长选择药丸 / Quick Preset Pills */}
              <div className='flex flex-wrap gap-1.5 pt-1'>
                {[
                  {label: '15分钟', val: 15},
                  {label: '30分钟', val: 30},
                  {label: '1小时', val: 60},
                  {label: '6小时', val: 360},
                  {label: '24小时', val: 1440},
                ].map((preset) => (
                  <Badge
                    key={preset.val}
                    variant={lookbackMinutes === preset.val ? 'default' : 'outline'}
                    className='cursor-pointer text-xs transition-colors'
                    onClick={() => setLookbackMinutes(preset.val)}
                  >
                    {preset.label}
                  </Badge>
                ))}
              </div>
              <p className='text-xs text-muted-foreground'>
                {t('inspections.lookbackHint')}
              </p>
            </div>

            <div className='space-y-1.5'>
              <Label htmlFor='dialog-inspection-error-threshold'>
                {t('inspections.errorThresholdLabel')}
              </Label>
              <Input
                id='dialog-inspection-error-threshold'
                type='number'
                min={1}
                max={1000}
                step={1}
                value={errorThreshold}
                onChange={(event) =>
                  setErrorThreshold(
                    Number.parseInt(event.target.value, 10) || 1,
                  )
                }
                onKeyDown={handleInspectionInputKeyDown}
                className='h-9'
              />
              <p className='text-xs text-muted-foreground'>
                {t('inspections.errorThresholdHint')}
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setStartInspectionDialogOpen(false)}
            >
              {commonT('cancel')}
            </Button>
            <Button
              onClick={() => void handleStartInspection()}
              disabled={!clusterId || startingInspection}
            >
              {startingInspection ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <Play className='mr-2 h-4 w-4 fill-current' />
              )}
              {t('inspections.startInspection')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
