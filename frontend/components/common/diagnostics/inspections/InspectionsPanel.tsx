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
 * 巡检面板：状态分段（服务端筛选，无分桶计数）+ 严重度下拉 + TableCard 列表 +
 * 常驻详情栏 / 抽屉，与错误面板、巡检策略面板同款布局；report_id 随 URL 深链；
 * 当前页存在 pending / running 报告时每 5s 静默刷新。
 * Inspection panel: status segments (server-side, no bucket counts) + severity
 * select + TableCard list + persistent rail / drawer, same layout as the
 * errors and policies panels; report_id stays deep-linked via the URL; the
 * list silently refreshes every 5s while the current page has pending /
 * running reports.
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Play, RefreshCw, X} from 'lucide-react';
import {toast} from '@/lib/toast';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Select} from '@/components/arc/select/select';
import {Tooltip} from '@/components/arc/tooltip/tooltip';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  DiagnosticsClusterOption,
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReport,
  DiagnosticsTask,
} from '@/lib/services/diagnostics';
import {cn} from '@/lib/utils';
import {InspectionReportDetail} from './InspectionReportDetail';
import {StartInspectionDialog} from './StartInspectionDialog';
import {
  inspectionStatusTone,
  isLiveInspectionStatus,
  type InspectionSeverityFilter,
  type InspectionStatusFilter,
} from './inspection-utils';
import {formatErrorDateTime} from '../errors/error-utils';
import {localizeDiagnosticsText} from '../text-utils';
// shortcut: 暂直接复用告警中心的详情宿主 / 列表样式 / 软徽章；告警中心组件的并行改动落地后，统一提升到
// common/layout。
// Temporary direct reuse of the monitoring detail host, list styles and soft chips; promote to common/layout
// once the in-flight monitoring component changes land.
import {DetailHost, useDetailNarrow} from '../../monitoring/detail-host';
import {NeutralChip, SoftChip} from '../../monitoring/status-chips';
import styles from '../../monitoring/monitoring.module.css';

const PAGE_SIZE = 20;
const AUTO_REFRESH_MS = 5000;

export interface InspectionsPanelProps {
  /** 工作台所选集群：列表按集群预过滤，也是「发起巡检」的目标集群。Selected workspace cluster; pre-filters the list and is the target of "run inspection". */
  clusterId?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** URL 深链的目标巡检报告。Deep-linked inspection report id from the URL. */
  reportId?: number;
  onSelectReport?: (reportId: number | null) => void;
}

export function InspectionsPanel({
  clusterId,
  clusterOptions,
  reportId,
  onSelectReport,
}: InspectionsPanelProps) {
  const t = useTranslations('diagnosticsCenter.inspections');
  const commonT = useTranslations('common');
  const narrow = useDetailNarrow();

  const [statusFilter, setStatusFilter] =
    useState<InspectionStatusFilter>('all');
  const [severityFilter, setSeverityFilter] =
    useState<InspectionSeverityFilter>('all');
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
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
  const [startDialogOpen, setStartDialogOpen] = useState(false);
  const reportsRequestIdRef = useRef(0);
  const detailRequestIdRef = useRef(0);

  const drawerOpen = selectedReportId !== null;
  const drawerReport = useDrawerItem(selectedReport);

  const clearSelectedReportDetail = useCallback(() => {
    detailRequestIdRef.current += 1;
    setLoadingDetail(false);
    setSelectedReport(null);
    setFindings([]);
    setBundleTask(null);
  }, []);

  // silent 刷新不置 loading / 不清空列表：自动轮询不应打断正在看的行。
  // A silent refresh skips the skeleton and keeps rows: background polling must
  // not interrupt what the user is reading.
  const loadReports = useCallback(
    async (options?: {silent?: boolean}) => {
      const requestId = reportsRequestIdRef.current + 1;
      reportsRequestIdRef.current = requestId;
      if (!options?.silent) {
        setLoading(true);
      }
      try {
        const result = await services.diagnostics.getInspectionReportsSafe({
          cluster_id: clusterId,
          status: statusFilter !== 'all' ? statusFilter : undefined,
          severity: severityFilter !== 'all' ? severityFilter : undefined,
          page,
          page_size: PAGE_SIZE,
        });
        if (reportsRequestIdRef.current !== requestId) {
          return;
        }
        if (!result.success || !result.data) {
          // 静默轮询失败不弹提示（5 秒一次会刷屏），保留旧行等下一轮。
          // Silent polling failures stay quiet (a 5s cadence would spam toasts); keep rows for the next tick.
          if (options?.silent) {
            return;
          }
          setError(result.error || t('loadReportsError'));
          setReports([]);
          setReportTotal(0);
          return;
        }
        setError(null);
        setReports(result.data.items || []);
        setReportTotal(result.data.total || 0);
      } finally {
        if (reportsRequestIdRef.current === requestId) {
          setLoading(false);
        }
      }
    },
    [clusterId, page, severityFilter, statusFilter, t],
  );

  const loadReportDetail = useCallback(
    async (targetReportId: number, options?: {silent?: boolean}) => {
      const requestId = detailRequestIdRef.current + 1;
      detailRequestIdRef.current = requestId;
      if (!options?.silent) {
        setLoadingDetail(true);
      }
      try {
        const result =
          await services.diagnostics.getInspectionReportDetailSafe(
            targetReportId,
          );
        if (detailRequestIdRef.current !== requestId) {
          return;
        }
        if (!result.success || !result.data) {
          toast.error(result.error || t('loadDetailError'));
          return;
        }
        setSelectedReport(result.data.report);
        setFindings(result.data.findings || []);
        setBundleTask(result.data.related_diagnostic_task || null);
      } finally {
        if (detailRequestIdRef.current === requestId) {
          setLoadingDetail(false);
        }
      }
    },
    [t],
  );

  useEffect(() => {
    void loadReports();
  }, [loadReports]);

  // URL 深链：外部传入 report_id 时打开对应详情。
  // Deep link: an external report_id opens that report's detail.
  useEffect(() => {
    if (reportId) {
      setSelectedReportId(reportId);
      void loadReportDetail(reportId);
    }
  }, [reportId, loadReportDetail]);

  // 当前页存在 pending / running 报告时每 5s 静默刷新，完成态出现即停。
  // While the current page holds pending / running reports, refresh silently
  // every 5s; stops once none remain.
  const hasLiveReports = useMemo(
    () => reports.some((r) => isLiveInspectionStatus(r.status)),
    [reports],
  );
  useEffect(() => {
    if (!hasLiveReports) {
      return;
    }
    const timer = setInterval(() => {
      void loadReports({silent: true});
    }, AUTO_REFRESH_MS);
    return () => clearInterval(timer);
  }, [hasLiveReports, loadReports]);

  // 打开的报告仍处 pending / running 时同步静默刷新详情。
  // An open report still pending / running refreshes its detail silently too.
  const detailIsLive = selectedReport
    ? isLiveInspectionStatus(selectedReport.status)
    : false;
  useEffect(() => {
    if (!detailIsLive || selectedReportId == null) {
      return;
    }
    const id = selectedReportId;
    const timer = setInterval(() => {
      void loadReportDetail(id, {silent: true});
    }, AUTO_REFRESH_MS);
    return () => clearInterval(timer);
  }, [detailIsLive, selectedReportId, loadReportDetail]);

  const openReportDetail = useCallback(
    (report: DiagnosticsInspectionReport) => {
      setSelectedReportId(report.id);
      setSelectedReport(report);
      onSelectReport?.(report.id);
      void loadReportDetail(report.id);
    },
    [loadReportDetail, onSelectReport],
  );

  const handleCloseDetail = useCallback(() => {
    setSelectedReportId(null);
    clearSelectedReportDetail();
    onSelectReport?.(null);
  }, [clearSelectedReportDetail, onSelectReport]);

  const handleInspectionStarted = useCallback(
    (result: {
      report: DiagnosticsInspectionReport;
      findings: DiagnosticsInspectionFinding[];
      task: DiagnosticsTask | null;
    }) => {
      setPage(1);
      void loadReports();
      setSelectedReportId(result.report.id);
      setSelectedReport(result.report);
      setFindings(result.findings);
      setBundleTask(result.task);
      onSelectReport?.(result.report.id);
    },
    [loadReports, onSelectReport],
  );

  const totalPages = Math.max(1, Math.ceil(reportTotal / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);

  const activeIndex = selectedReport
    ? reports.findIndex((r) => r.id === selectedReport.id)
    : -1;
  const detailNav =
    selectedReport && activeIndex >= 0
      ? {
          index: activeIndex,
          total: reports.length,
          onPrev: () =>
            activeIndex > 0 && openReportDetail(reports[activeIndex - 1]),
          onNext: () =>
            activeIndex < reports.length - 1 &&
            openReportDetail(reports[activeIndex + 1]),
        }
      : undefined;

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    id: number,
  ) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const target = reports.find((r) => r.id === id);
      if (target) {
        openReportDetail(target);
      }
    }
  };

  const filtersActive = statusFilter !== 'all' || severityFilter !== 'all';
  const seg = (key: InspectionStatusFilter, label: string) => ({
    value: key,
    label,
  });

  const startButton = (
    <Button
      type='button'
      size='sm'
      onClick={() => setStartDialogOpen(true)}
      disabled={!clusterId}
      data-testid='inspection-start'
    >
      <Play className='size-3 fill-current' aria-hidden />
      {t('startInspection')}
    </Button>
  );

  return (
    <div className={styles.page} style={{overflowY: 'auto'}}>
      <div className={styles.toolbar}>
        <SegmentedControl
          label={t('filters.status')}
          value={statusFilter}
          onValueChange={(v) => {
            setStatusFilter(v as InspectionStatusFilter);
            setPage(1);
          }}
          options={[
            seg('all', t('statusAll')),
            seg('failed', t('status.failed')),
            seg('completed', t('status.completed')),
            seg('running', t('status.running')),
            seg('pending', t('status.pending')),
          ]}
        />
        <div className={cn(styles.filterCtrl, styles.filterWsm)}>
          <Select
            label={t('filters.severity')}
            value={severityFilter}
            onValueChange={(v) => {
              setSeverityFilter(v as InspectionSeverityFilter);
              setPage(1);
            }}
            displayValue={
              severityFilter === 'all' ? t('filters.severity') : undefined
            }
            options={[
              {value: 'all', label: t('filters.allSeverities')},
              {value: 'critical', label: t('severity.critical')},
              {value: 'warning', label: t('severity.warning')},
              {value: 'info', label: t('severity.info')},
            ]}
          />
        </div>
        {filtersActive ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={() => {
              setStatusFilter('all');
              setSeverityFilter('all');
              setPage(1);
            }}
          >
            <X className='size-3' aria-hidden />
            {t('resetFilters')}
          </Button>
        ) : null}
        <div className={styles.toolbarRight}>
          {/* 未选集群时保留禁用态但用 Tooltip 说明原因。
              Without a selected cluster the action stays disabled and the
              tooltip explains why. */}
          {clusterId ? (
            startButton
          ) : (
            <Tooltip content={t('selectClusterFirst')}>
              <span className='inline-flex'>{startButton}</span>
            </Tooltip>
          )}
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => void loadReports()}
            disabled={loading}
            data-testid='inspections-refresh'
          >
            <RefreshCw
              className={cn('size-3.5', loading && 'animate-spin')}
              aria-hidden
            />
            {commonT('refresh')}
          </Button>
        </div>
      </div>

      <div className={styles.split}>
        <div className={styles.listCol} data-mcols='inspections'>
          {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
              Single main card; skeleton/error/empty states are owned by TableCard. */}
          <TableCard
            loading={loading}
            hasData={reports.length > 0}
            skeletonColumns={6}
            skeletonRows={6}
            error={error}
            onRetry={() => void loadReports()}
            pagination={{
              currentPage: safePage,
              totalPages,
              pageSize: PAGE_SIZE,
              totalItems: reportTotal,
              onPageChange: setPage,
            }}
            empty={
              <EmptyState
                title={filtersActive ? t('empty') : t('emptyAll')}
                description={
                  filtersActive ? t('emptyDesc') : t('emptyAllDesc')
                }
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setStatusFilter('all');
                        setSeverityFilter('all');
                        setPage(1);
                      }}
                    >
                      {commonT('clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={styles.rowHead} aria-hidden>
              <span>{t('columns.id')}</span>
              <span>{t('columns.trigger')}</span>
              <span>{t('columns.status')}</span>
              <span>{t('columns.findings')}</span>
              <span>{t('columns.lookback')}</span>
              <span>{t('columns.finishedAt')}</span>
            </div>
            <div className={styles.tableScroll}>
              {reports.map((report) => (
                <div
                  key={report.id}
                  data-mrow={report.id}
                  data-testid={`inspection-row-${report.id}`}
                  role='button'
                  tabIndex={0}
                  onClick={() => openReportDetail(report)}
                  onKeyDown={(e) => onRowKeyDown(e, report.id)}
                  className={cn(
                    styles.row,
                    selectedReportId === report.id && styles.rowActive,
                  )}
                >
                  <span className={cn(styles.sub, 'font-mono')}>
                    #{report.id}
                  </span>
                  <NeutralChip
                    title={t(`trigger.${report.trigger_source}`)}
                  >
                    {t(`trigger.${report.trigger_source}`)}
                  </NeutralChip>
                  <SoftChip tone={inspectionStatusTone(report.status)}>
                    {t(`status.${report.status}`)}
                  </SoftChip>
                  <div className={styles.rowName}>
                    <span
                      className={styles.name}
                      title={
                        report.status === 'completed' &&
                        report.finding_total === 0
                          ? t('evidence.zeroShort')
                          : localizeDiagnosticsText(report.summary) ||
                            t('summaryFallback')
                      }
                    >
                      {report.status === 'completed' &&
                      report.finding_total === 0
                        ? t('evidence.zeroShort')
                        : localizeDiagnosticsText(report.summary) ||
                          t('summaryFallback')}
                    </span>
                    <span className={styles.sub}>
                      <span className='inline-flex flex-wrap items-center gap-1.5'>
                        {report.critical_count > 0 ? (
                          <SoftChip tone='rose'>
                            {t('findingCount.critical', {
                              count: report.critical_count,
                            })}
                          </SoftChip>
                        ) : null}
                        {report.warning_count > 0 ? (
                          <SoftChip tone='amber'>
                            {t('findingCount.warning', {
                              count: report.warning_count,
                            })}
                          </SoftChip>
                        ) : null}
                        {report.info_count > 0 ? (
                          <SoftChip tone='blue'>
                            {t('findingCount.info', {
                              count: report.info_count,
                            })}
                          </SoftChip>
                        ) : null}
                        {report.status === 'completed' &&
                        report.finding_total === 0 ? (
                          <span className='inline-flex items-center gap-1 text-muted-foreground'>
                            <Check className='size-3' aria-hidden />
                            {t('evidence.zeroShort')}
                          </span>
                        ) : null}
                      </span>
                    </span>
                  </div>
                  <span
                    className={cn(styles.sub, 'whitespace-nowrap font-mono')}
                  >
                    {t('lookbackValue', {
                      minutes: report.lookback_minutes || 30,
                    })}
                  </span>
                  <span
                    className={cn(styles.sub, 'whitespace-nowrap font-mono')}
                  >
                    {formatErrorDateTime(
                      report.finished_at || report.created_at,
                    )}
                  </span>
                </div>
              ))}
            </div>
          </TableCard>
        </div>

        <DetailHost
          open={drawerOpen}
          hasContent={drawerReport != null || loadingDetail}
          onOpenChange={(o) => !o && handleCloseDetail()}
          narrow={narrow}
          title={t('detailTitle')}
          meta={
            drawerReport
              ? t('evidence.reportNumber', {id: drawerReport.id})
              : undefined
          }
          nav={detailNav}
          wide
          drawerWidth='w-[min(92vw,640px)]'
        >
          <InspectionReportDetail
            key={drawerReport?.id ?? 'pending'}
            report={drawerReport ?? null}
            findings={findings}
            bundleTask={bundleTask}
            loading={loadingDetail}
            clusterOptions={clusterOptions}
            onBundleTaskChange={setBundleTask}
            onClose={handleCloseDetail}
          />
        </DetailHost>
      </div>

      <StartInspectionDialog
        clusterId={clusterId}
        open={startDialogOpen}
        onOpenChange={setStartDialogOpen}
        onStarted={handleInspectionStarted}
      />
    </div>
  );
}
