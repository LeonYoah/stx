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
  ArrowUpRight,
  Check,
  ClipboardCheck,
  Clock,
  Download,
  ExternalLink,
  Eye,
  FileText,
  Loader2,
  Package,
  Play,
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
  DiagnosticsResourceCode,
  DiagnosticsTask,
  DiagnosticsTaskNodeScope,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card} from '@/components/ui/card';
import {Pagination} from '@/components/ui/pagination';
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
import {Switch} from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  StatPillsBar,
  type StatPillItem,
  TableLoadingBar,
  TableSkeletonRows,
} from '@/components/common/layout';
import {Skeleton} from '@/components/ui/skeleton';
import {
  animateTableRows,
  animateSheetSections,
} from '@/lib/animations/gsap-motion';
import {localizeDiagnosticsText} from './text-utils';
import {
  InspectionFindingCard,
  InspectionZeroState,
} from './InspectionEvidenceView';
import {DiagnosticResourceSelector} from './DiagnosticResourceSelector';

const DEFAULT_BUNDLE_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: false,
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

function formatNodeOrigin(options: {
  nodeId?: number | null;
  hostId?: number | null;
  hostName?: string | null;
  hostIp?: string | null;
  role?: string | null;
}): string {
  const parts: string[] = [];
  if (options.hostName?.trim()) {
    parts.push(options.hostName.trim());
  } else if (options.hostIp?.trim()) {
    parts.push(options.hostIp.trim());
  } else if (options.hostId) {
    parts.push(`#${options.hostId}`);
  }
  if (options.role?.trim()) {
    parts.push(options.role.trim());
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
  const [inspectionResources, setInspectionResources] = useState<
    DiagnosticsResourceCode[]
  >([]);
  const [reports, setReports] = useState<DiagnosticsInspectionReport[]>([]);
  const [reportTotal, setReportTotal] = useState(0);

  // 抽屉详情查看的巡检报告
  // Inspection report opened in slide-over detail sheet
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
  const [bundleOptions, setBundleOptions] = useState<DiagnosticsTaskOptions>(
    DEFAULT_BUNDLE_OPTIONS,
  );
  const [nodeScope, setNodeScope] = useState<DiagnosticsTaskNodeScope>('all');
  const [bundleLookbackMinutes, setBundleLookbackMinutes] =
    useState<number>(30);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [execLogDialogOpen, setExecLogDialogOpen] = useState(false);

  // 加载巡检报告列表
  // Fetch inspection reports list
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

  // 加载报告详情与发现项
  // Fetch inspection report detail and findings
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

  // 外部 reportId 驱动打开详情
  // Open report detail when external reportId prop is provided
  useEffect(() => {
    if (reportId) {
      setSelectedReportId(reportId);
      void loadDetail(reportId);
    }
  }, [loadDetail, reportId]);

  // 表格行进入动效
  // Stagger animation for table rows on load
  useEffect(() => {
    if (!loadingReports && reports.length > 0) {
      animateTableRows('.data-row-animate');
    }
  }, [loadingReports, reports]);

  // 抽屉滑入动效
  // Stagger animation for sections in detail sheet
  useEffect(() => {
    if (selectedReport && !loadingDetail) {
      animateSheetSections('.sheet-section-animate');
    }
  }, [loadingDetail, selectedReport]);

  const handleSelectReport = useCallback(
    (report: DiagnosticsInspectionReport) => {
      setSelectedReportId(report.id);
      setSelectedReport(report);
      onSelectReport?.(report.id);
      void loadDetail(report.id);
    },
    [loadDetail, onSelectReport],
  );

  const handleCloseSheet = useCallback(() => {
    setSelectedReportId(null);
    setSelectedReport(null);
    setFindings([]);
    setBundleTask(null);
    onSelectReport?.(null);
  }, [onSelectReport]);

  const hasFindings = findings.length > 0;

  // 统计各状态数量
  // Compute report counts for status pills
  const stats = useMemo(() => {
    let failed = 0;
    let completed = 0;
    let running = 0;
    let pending = 0;
    reports.forEach((r) => {
      if (r.status === 'failed') {
        failed += 1;
      } else if (r.status === 'completed') {
        completed += 1;
      } else if (r.status === 'running') {
        running += 1;
      } else if (r.status === 'pending') {
        pending += 1;
      }
    });
    return {
      total: reportTotal,
      failed,
      completed,
      running,
      pending,
    };
  }, [reportTotal, reports]);

  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'all',
        label: t('inspections.listPills.all'),
        count: stats.total,
      },
      {
        key: 'failed',
        label: t('inspections.listPills.failed'),
        count: stats.failed,
        variant: 'danger',
        pulse: stats.failed > 0,
      },
      {
        key: 'completed',
        label: t('inspections.listPills.completed'),
        count: stats.completed,
        variant: 'default',
      },
      {
        key: 'running',
        label: t('inspections.listPills.running'),
        count: stats.running,
        variant: 'info',
      },
      {
        key: 'pending',
        label: t('inspections.listPills.pending'),
        count: stats.pending,
        variant: 'default',
      },
    ],
    [stats, t],
  );

  // 轮询诊断包任务进度
  // Poll diagnostic bundle task status until completion
  const pollBundleTask = useCallback(
    async (taskId: number) => {
      try {
        const result = await services.diagnostics.getTaskSafe(taskId);
        if (!result.success || !result.data) {
          return;
        }
        setBundleTask(result.data);
        if (
          result.data.status === 'succeeded' ||
          result.data.status === 'failed' ||
          result.data.status === 'cancelled'
        ) {
          if (pollTimerRef.current) {
            clearInterval(pollTimerRef.current);
            pollTimerRef.current = null;
          }
          setPollingBundle(false);
          if (result.data.status === 'succeeded') {
            toast.success(t('inspections.followUp.taskCompleteSuccess'));
          } else {
            toast.error(t('inspections.followUp.taskFailed'));
          }
        }
      } catch {
        // 静默失败，等待下次轮询
        // Silently catch and retry on next interval tick
      }
    },
    [t],
  );

  useEffect(() => {
    if (!pollingBundle || !bundleTask) {
      return;
    }
    const currentTaskId = bundleTask.id;
    pollTimerRef.current = setInterval(() => {
      void pollBundleTask(currentTaskId);
    }, 2000);
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [bundleTask, pollBundleTask, pollingBundle]);

  const handleConfirmAndCreateBundle = useCallback(() => {
    setConfirmDialogOpen(true);
  }, []);

  const handleCreateBundle = useCallback(async () => {
    if (!selectedReport || creatingBundle) {
      return;
    }
    setConfirmDialogOpen(false);
    setCreatingBundle(true);
    try {
      const firstFinding = findings[0];
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
    if (inspectionResources.length === 0) {
      toast.error(t('inspections.resourceRequired'));
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
      const report = result.data.report;
      const taskResult = await services.diagnostics.createTaskSafe({
        cluster_id: clusterId,
        trigger_source: 'inspection_finding',
        source_ref: {inspection_report_id: report.id},
        node_scope: 'all',
        options: {
          include_thread_dump: inspectionResources.includes('thread_dump'),
          include_jvm_dump: inspectionResources.includes('jvm_dump'),
          jvm_dump_min_free_mb: 2048,
          selected_resources: inspectionResources,
        },
        lookback_minutes: lookbackMinutes,
        auto_start: true,
      });
      if (!taskResult.success) {
        toast.warning(
          t('inspections.resourceTaskError', {
            error: taskResult.error || t('inspections.startError'),
          }),
        );
      } else {
        toast.success(t('inspections.startSuccess'));
      }
      setStartInspectionDialogOpen(false);
      setPage(1);
      await loadReports();
      setSelectedReportId(report.id);
      onSelectReport?.(report.id);
      setSelectedReport(report);
      setFindings(result.data.findings || []);
      if (taskResult.data) {
        setBundleTask(taskResult.data);
        setPollingBundle(true);
      }
    } finally {
      setStartingInspection(false);
    }
  }, [
    clusterId,
    errorThreshold,
    inspectionResources,
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

  // 未选择集群时保留按钮反馈，避免用户面对无法解释的禁用状态。
  // Keep the action responsive without a selected cluster so the disabled state is never unexplained.
  const handleOpenStartInspection = useCallback(() => {
    if (!clusterId) {
      toast.info(t('inspections.selectClusterFirst'));
      return;
    }
    setStartInspectionDialogOpen(true);
  }, [clusterId, t]);

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

  const totalPages = Math.max(1, Math.ceil(reportTotal / 20));

  return (
    <div className='space-y-3.5'>
      {/* 统一全宽卡片（彻底消除左右不对称，释放完整横向排版空间） */}
      {/* Unified full-width card container (eliminates asymmetry, maximizes layout room) */}
      <Card className='border border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        {/* 顶部胶囊栏与操作区 / Top Stat Pills & Actions */}
        <div className='p-3 border-b bg-card/60 space-y-2.5'>
          {/* 第一行：状态胶囊分段与主要操作 */}
          {/* Row 1: Stat pills segments and primary action buttons */}
          <StatPillsBar
            items={pillItems}
            activeKey={statusFilter}
            onChange={(key) => {
              setPage(1);
              setStatusFilter(key as typeof statusFilter);
            }}
            actions={
              <>
                <Button
                  size='sm'
                  onClick={handleOpenStartInspection}
                  disabled={startingInspection}
                  className='h-7 px-2.5 text-xs gap-1.5'
                >
                  {startingInspection ? (
                    <Loader2 className='h-3.5 w-3.5 animate-spin' />
                  ) : (
                    <Play className='h-3 w-3 fill-current' />
                  )}
                  {t('inspections.startInspection')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void loadReports()}
                  disabled={loadingReports}
                  className='h-7 px-2.5 text-xs'
                >
                  <RefreshCw
                    className={cn(
                      'mr-1.5 h-3 w-3',
                      loadingReports && 'animate-spin',
                    )}
                  />
                  {commonT('refresh')}
                </Button>
              </>
            }
          />

          {/* 第二行：严重程度辅助筛选 */}
          {/* Row 2: Severity multi-dimensional filter strip */}
          <div className='flex flex-wrap items-center gap-2 pt-0.5'>
            <div className='flex items-center gap-1.5'>
              <span className='text-xs text-muted-foreground whitespace-nowrap'>
                {t('inspections.filters.severity')}:
              </span>
              <Select
                value={severityFilter}
                onValueChange={(value) => {
                  setPage(1);
                  setSeverityFilter(value as typeof severityFilter);
                }}
              >
                <SelectTrigger className='h-8 text-xs w-[135px] bg-background'>
                  <SelectValue
                    placeholder={t('inspections.filters.severity')}
                  />
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
                className='h-8 px-2 text-xs text-muted-foreground hover:text-foreground'
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
        </div>

        {/* 全宽对称高密度报告表格 / Full-width Symmetric High-Density Reports Table */}
        <TableLoadingBar loading={loadingReports && reports.length > 0} />
        <div className='overflow-x-auto flex-1'>
          <Table>
            <TableHeader>
              <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                <TableHead className='w-[80px] py-1.5 px-3 text-xs'>
                  编号
                </TableHead>
                <TableHead className='w-[105px] py-1.5 px-3 text-xs'>
                  {t('inspections.filters.status')}
                </TableHead>
                <TableHead className='w-[130px] py-1.5 px-3 text-xs'>
                  触发方式
                </TableHead>
                <TableHead className='min-w-[280px] py-1.5 px-3 text-xs'>
                  发现项与摘要概览
                </TableHead>
                <TableHead className='w-[110px] py-1.5 px-3 text-xs'>
                  回溯窗口
                </TableHead>
                <TableHead className='w-[155px] py-1.5 px-3 text-xs whitespace-nowrap'>
                  完成时间
                </TableHead>
                <TableHead className='w-[140px] py-1.5 px-3 text-xs text-right'>
                  {commonT('actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loadingReports && reports.length === 0 ? (
                <TableSkeletonRows columns={7} rows={6} />
              ) : reports.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className='h-36 text-center text-muted-foreground text-xs'
                  >
                    {t('inspections.empty')}
                  </TableCell>
                </TableRow>
              ) : (
                reports.map((report) => (
                  <TableRow
                    key={report.id}
                    className={cn(
                      'data-row-animate cursor-pointer transition-all hover:bg-muted/40 h-10',
                      loadingReports && 'opacity-50 pointer-events-none',
                      selectedReportId === report.id &&
                        'bg-primary/5 font-medium border-l-2 border-l-primary',
                    )}
                    onClick={() => handleSelectReport(report)}
                  >
                    {/* 报告编号 */}
                    <TableCell className='py-2 px-3 font-mono text-xs text-muted-foreground'>
                      #{report.id}
                    </TableCell>

                    {/* 状态 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap'>
                      <Badge
                        variant={getStatusVariant(report.status)}
                        className='text-xs'
                      >
                        {t(`inspections.status.${report.status}`)}
                      </Badge>
                    </TableCell>

                    {/* 触发方式 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground'>
                      <Badge
                        variant='outline'
                        className='text-[11px] font-normal'
                      >
                        {t(`inspections.trigger.${report.trigger_source}`)}
                      </Badge>
                    </TableCell>

                    {/* 发现项概要与严重程度徽标 */}
                    <TableCell className='py-2 px-3'>
                      <div className='space-y-1 max-w-[480px]'>
                        <div
                          className='truncate text-xs font-medium text-foreground'
                          title={
                            report.status === 'completed' &&
                            report.finding_total === 0
                              ? t('inspections.evidence.zeroShort')
                              : localizeDiagnosticsText(report.summary) ||
                                t('inspections.summaryFallback')
                          }
                        >
                          {report.status === 'completed' &&
                          report.finding_total === 0
                            ? t('inspections.evidence.zeroShort')
                            : localizeDiagnosticsText(report.summary) ||
                              t('inspections.summaryFallback')}
                        </div>

                        <div className='flex flex-wrap items-center gap-1.5'>
                          {report.critical_count > 0 && (
                            <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'>
                              <span className='h-1.5 w-1.5 rounded-full bg-rose-500' />
                              {report.critical_count} 严重
                            </span>
                          )}
                          {report.warning_count > 0 && (
                            <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'>
                              <span className='h-1.5 w-1.5 rounded-full bg-amber-500' />
                              {report.warning_count} 警告
                            </span>
                          )}
                          {report.info_count > 0 && (
                            <span className='inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20'>
                              <span className='h-1.5 w-1.5 rounded-full bg-blue-500' />
                              {report.info_count} 提示
                            </span>
                          )}
                          {report.status === 'completed' &&
                            report.finding_total === 0 && (
                              <span className='inline-flex items-center gap-1 text-[11px] text-muted-foreground'>
                                <Check aria-hidden='true' className='h-3 w-3' />
                                {t('inspections.evidence.zeroShort')}
                              </span>
                            )}
                        </div>
                      </div>
                    </TableCell>

                    {/* 回溯时间窗口 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground'>
                      <span className='inline-flex items-center gap-1 font-mono text-[11px]'>
                        <Clock className='h-3 w-3 text-muted-foreground/70' />
                        {t('inspections.lookbackValue', {
                          minutes: report.lookback_minutes || 30,
                        })}
                      </span>
                    </TableCell>

                    {/* 完成时间 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono'>
                      {formatDateTime(report.finished_at || report.created_at)}
                    </TableCell>

                    {/* 操作列 */}
                    <TableCell className='py-2 px-3 text-right whitespace-nowrap'>
                      <div className='flex items-center justify-end gap-1'>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-6.5 px-2 text-xs text-primary hover:text-primary'
                          onClick={(e) => {
                            e.stopPropagation();
                            handleSelectReport(report);
                          }}
                        >
                          <Eye className='mr-1 h-3.5 w-3.5' />
                          摘要
                        </Button>
                        <Button
                          asChild
                          variant='ghost'
                          size='sm'
                          className='h-6.5 px-2 text-xs text-muted-foreground hover:text-foreground'
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Link href={`/diagnostics/inspections/${report.id}`}>
                            完整报告
                            <ArrowUpRight className='ml-1 h-3 w-3' />
                          </Link>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {/* 底部分页栏 / Table Footer Pagination */}
        <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
          <Pagination
            currentPage={page}
            totalPages={totalPages}
            pageSize={20}
            totalItems={reportTotal}
            onPageChange={setPage}
            showPageSizeSelector={false}
          />
        </div>
      </Card>

      {/* 侧边滑出式巡检详情抽屉（告警中心同款架构） */}
      {/* Slide-over Sheet for Inspection Report Overview & Diagnostics Tasks */}
      <Sheet
        open={Boolean(selectedReportId && selectedReport)}
        onOpenChange={(open) => {
          if (!open) {
            handleCloseSheet();
          }
        }}
      >
        <SheetContent
          side='right'
          className='w-full sm:max-w-2xl p-0 flex flex-col overflow-hidden bg-background'
        >
          {/* 抽屉只保留报告定位信息，发现项先于诊断包操作。 */}
          {/* Keep report context brief; show evidence before bundle actions. */}
          <SheetHeader className='border-b border-border/70 px-5 pb-5 pt-6'>
            <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
              <Badge
                variant={
                  selectedReport
                    ? getStatusVariant(selectedReport.status)
                    : 'outline'
                }
              >
                {selectedReport
                  ? t(`inspections.status.${selectedReport.status}`)
                  : ''}
              </Badge>
              <span className='font-mono tabular-nums'>
                {selectedReport
                  ? t('inspections.evidence.reportNumber', {
                      id: selectedReport.id,
                    })
                  : ''}
              </span>
            </div>
            <SheetTitle className='pt-3 font-serif text-2xl font-normal tracking-tight text-foreground'>
              {t('inspections.detailPage.title')}
            </SheetTitle>
            <SheetDescription className='break-words text-sm'>
              {selectedReport?.cluster_name ||
                clusterName ||
                (selectedReport
                  ? t('inspections.detailPage.clusterFallback', {
                      clusterId: selectedReport.cluster_id,
                    })
                  : '')}
              {selectedReport?.finished_at
                ? ` · ${formatDateTime(selectedReport.finished_at)}`
                : ''}
            </SheetDescription>
          </SheetHeader>

          <ScrollArea className='min-h-0 flex-1 px-4 py-5 sm:px-5'>
            {loadingDetail ? (
              <div className='space-y-3' aria-busy='true'>
                <Skeleton className='h-20 w-full' />
                <Skeleton className='h-36 w-full' />
              </div>
            ) : selectedReport ? (
              <div className='space-y-6'>
                {selectedReport.error_message && (
                  <p
                    role='alert'
                    className='border-l-2 border-destructive pl-3 text-sm text-destructive'
                  >
                    {selectedReport.error_message}
                  </p>
                )}
                {findings.length > 0 ? (
                  <section
                    className='space-y-3'
                    aria-label={t('inspections.evidence.attentionTitle')}
                  >
                    <div className='flex items-baseline justify-between border-b border-border/70 pb-3'>
                      <h3 className='font-serif text-xl text-foreground'>
                        {t('inspections.evidence.attentionTitle')}
                      </h3>
                      <span className='font-mono text-xl tabular-nums text-foreground'>
                        {findings.length}
                      </span>
                    </div>
                    {[...findings]
                      .sort(
                        (a, b) =>
                          (({critical: 3, warning: 2, info: 1})[b.severity] ??
                            0) -
                          ({critical: 3, warning: 2, info: 1}[a.severity] ?? 0),
                      )
                      .map((finding, index) => (
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
                ) : selectedReport.status === 'completed' &&
                  selectedReport.finding_total === 0 &&
                  !selectedReport.error_message ? (
                  <InspectionZeroState compact />
                ) : selectedReport.status === 'completed' ? (
                  <p className='border border-border/70 bg-card px-5 py-8 text-sm text-muted-foreground'>
                    {t('inspections.evidence.unavailable')}
                  </p>
                ) : (
                  <p className='border border-border/70 bg-card px-5 py-8 text-sm text-muted-foreground'>
                    {selectedReport.status === 'failed'
                      ? t('inspections.evidence.incompleteDescription')
                      : t('inspections.evidence.pendingDescription')}
                  </p>
                )}

                {/* 诊断包是后续操作，不抢占证据的首屏。 */}
                {/* Bundle actions follow the evidence rather than leading the page. */}
                {selectedReport.status === 'completed' &&
                  (hasFindings || bundleTask) && (
                    <section className='space-y-3 border-t border-border/70 pt-5'>
                      <div className='flex items-center justify-between gap-3'>
                        <h3 className='text-sm font-semibold text-foreground'>
                          {t('inspections.detailPage.bundleTitle')}
                        </h3>
                        {bundleTask && (
                          <Badge variant={getStatusVariant(bundleTask.status)}>
                            {getTaskStatusLabel(bundleTask.status)}
                          </Badge>
                        )}
                      </div>
                      {bundleTask ? (
                        <div className='flex flex-wrap gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setExecLogDialogOpen(true)}
                          >
                            <FileText
                              aria-hidden='true'
                              className='mr-1.5 h-4 w-4'
                            />
                            {t('inspections.detailPage.viewExecutionLogs')}
                          </Button>
                          {bundleTask.status === 'succeeded' && (
                            <>
                              <Button asChild size='sm' variant='outline'>
                                <a
                                  href={services.diagnostics.getTaskHTMLUrl(
                                    bundleTask.id,
                                  )}
                                  target='_blank'
                                  rel='noopener noreferrer'
                                >
                                  <ExternalLink
                                    aria-hidden='true'
                                    className='mr-1.5 h-4 w-4'
                                  />
                                  {t('inspections.detailPage.previewReport')}
                                </a>
                              </Button>
                              <Button asChild size='sm' variant='outline'>
                                <a
                                  href={services.diagnostics.getTaskBundleUrl(
                                    bundleTask.id,
                                  )}
                                  download
                                >
                                  <Download
                                    aria-hidden='true'
                                    className='mr-1.5 h-4 w-4'
                                  />
                                  {t('inspections.detailPage.downloadBundle')}
                                </a>
                              </Button>
                            </>
                          )}
                          {(bundleTask.status === 'succeeded' ||
                            bundleTask.status === 'failed') && (
                            <Button
                              size='sm'
                              variant='outline'
                              onClick={handleConfirmAndCreateBundle}
                              disabled={creatingBundle}
                            >
                              <Package
                                aria-hidden='true'
                                className='mr-1.5 h-4 w-4'
                              />
                              {t('inspections.detailPage.regenerate')}
                            </Button>
                          )}
                          {pollingBundle && (
                            <span className='self-center text-xs text-muted-foreground'>
                              {t('inspections.refreshingLabel')}
                            </span>
                          )}
                        </div>
                      ) : (
                        <Button
                          size='sm'
                          onClick={handleConfirmAndCreateBundle}
                          disabled={creatingBundle}
                        >
                          <Package
                            aria-hidden='true'
                            className='mr-1.5 h-4 w-4'
                          />
                          {t('inspections.followUp.generateBundle')}
                        </Button>
                      )}
                    </section>
                  )}
              </div>
            ) : null}
          </ScrollArea>

          {/* 抽屉底部操作栏 / Sheet Footer Actions */}
          {selectedReport && (
            <div className='p-3 border-t bg-muted/20 flex flex-wrap items-center justify-between gap-2'>
              <Button
                asChild
                variant='default'
                size='sm'
                className='h-8 text-xs font-medium'
              >
                <Link href={`/diagnostics/inspections/${selectedReport.id}`}>
                  {t('inspections.evidence.viewFullReport')}
                  <ArrowUpRight className='ml-1.5 h-3.5 w-3.5' />
                </Link>
              </Button>
              <Button
                variant='secondary'
                size='sm'
                className='h-8 text-xs'
                onClick={handleCloseSheet}
              >
                {t('inspections.evidence.close')}
              </Button>
            </div>
          )}
        </SheetContent>
      </Sheet>

      {/* 生成现场诊断包确认弹窗 */}
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
          <div className='space-y-4 py-3 text-sm'>
            <div className='space-y-1.5'>
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
                className='h-8 text-xs'
              />
              <p className='text-xs text-muted-foreground'>
                {t('inspections.detailPage.bundleLookbackHint')}
              </p>
            </div>
            <div className='space-y-2.5'>
              <div className='flex items-center justify-between rounded-lg border p-2.5'>
                <div>
                  <div className='font-medium text-xs'>
                    {t('inspections.detailPage.includeThreadDump')}
                  </div>
                  <div className='text-[11px] text-muted-foreground'>
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
              <div className='flex items-center justify-between rounded-lg border p-2.5'>
                <div>
                  <div className='font-medium text-xs'>
                    {t('inspections.detailPage.includeJVMDump')}
                  </div>
                  <div className='text-[11px] text-muted-foreground'>
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
              <div className='flex flex-wrap gap-1.5 text-xs pt-1'>
                <Button
                  type='button'
                  variant={nodeScope === 'all' ? 'default' : 'outline'}
                  size='sm'
                  className='h-7 text-xs'
                  onClick={() => setNodeScope('all')}
                >
                  {t('inspections.detailPage.allNodes')}
                </Button>
                <Button
                  type='button'
                  variant={nodeScope === 'related' ? 'default' : 'outline'}
                  size='sm'
                  className='h-7 text-xs'
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
              size='sm'
              className='h-8 text-xs'
              onClick={() => setConfirmDialogOpen(false)}
            >
              {commonT('cancel')}
            </Button>
            <Button
              size='sm'
              className='h-8 text-xs'
              onClick={() => void handleCreateBundle()}
              disabled={creatingBundle}
            >
              {creatingBundle && (
                <Loader2 className='mr-1.5 h-3.5 w-3.5 animate-spin' />
              )}
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
          <div className='flex-1 overflow-y-auto space-y-2 py-2 text-xs'>
            {bundleTask?.steps?.length ? (
              bundleTask.steps.map((step) => (
                <div
                  key={step.id}
                  className='rounded-lg border p-2.5 space-y-1'
                >
                  <div className='flex items-center gap-2'>
                    <Badge
                      variant={getStatusVariant(step.status)}
                      className='text-xs'
                    >
                      {getTaskStatusLabel(step.status)}
                    </Badge>
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
                {t('inspections.detailPage.noExecutionSteps')}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* 发起巡检独立弹窗 / Launch Inspection Modal Dialog */}
      <Dialog
        open={startInspectionDialogOpen}
        onOpenChange={setStartInspectionDialogOpen}
      >
        <DialogContent className='max-h-[85vh] overflow-hidden border border-border/80 shadow-2xl sm:max-w-3xl'>
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

          <ScrollArea className='max-h-[calc(85vh-160px)] pr-4'>
            <div className='space-y-4 py-2 text-sm'>
              <div className='space-y-1.5'>
                <Label htmlFor='dialog-inspection-lookback' className='text-xs'>
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
                    className='h-8 text-xs'
                  />
                  <span className='text-xs text-muted-foreground whitespace-nowrap'>
                    分钟
                  </span>
                </div>
                <div className='flex flex-wrap gap-1 pt-1'>
                  {[
                    {label: '15分钟', val: 15},
                    {label: '30分钟', val: 30},
                    {label: '1小时', val: 60},
                    {label: '6小时', val: 360},
                    {label: '24小时', val: 1440},
                  ].map((preset) => (
                    <Badge
                      key={preset.val}
                      variant={
                        lookbackMinutes === preset.val ? 'default' : 'outline'
                      }
                      className='cursor-pointer text-xs transition-colors'
                      onClick={() => setLookbackMinutes(preset.val)}
                    >
                      {preset.label}
                    </Badge>
                  ))}
                </div>
                <p className='text-[11px] text-muted-foreground'>
                  {t('inspections.lookbackHint')}
                </p>
              </div>

              <div className='space-y-1.5'>
                <Label
                  htmlFor='dialog-inspection-error-threshold'
                  className='text-xs'
                >
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
                  className='h-8 text-xs'
                />
                <p className='text-[11px] text-muted-foreground'>
                  {t('inspections.errorThresholdHint')}
                </p>
              </div>

              <div className='space-y-2 border-t pt-4'>
                <Label className='text-xs'>
                  {t('inspections.resourcesLabel')}
                </Label>
                <DiagnosticResourceSelector
                  selectedResources={inspectionResources}
                  onChange={setInspectionResources}
                  disabled={startingInspection}
                />
              </div>
            </div>
          </ScrollArea>

          <DialogFooter>
            <Button
              variant='outline'
              size='sm'
              className='h-8 text-xs'
              onClick={() => setStartInspectionDialogOpen(false)}
            >
              {commonT('cancel')}
            </Button>
            <Button
              size='sm'
              className='h-8 text-xs'
              onClick={() => void handleStartInspection()}
              disabled={
                !clusterId ||
                startingInspection ||
                inspectionResources.length === 0
              }
            >
              {startingInspection ? (
                <Loader2 className='mr-1.5 h-3.5 w-3.5 animate-spin' />
              ) : (
                <Play className='mr-1.5 h-3 w-3 fill-current' />
              )}
              {t('inspections.startInspection')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
