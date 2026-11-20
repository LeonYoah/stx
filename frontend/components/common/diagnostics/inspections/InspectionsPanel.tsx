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
 * 巡检面板：「巡检记录 | 自动巡检」分段视图。记录视图 = 状态分段（服务端筛选，
 * 无分桶计数）+ 严重度下拉 + 集群 Combobox + TableCard 列表 + 常驻详情栏 /
 * 抽屉；自动视图复用 PoliciesPanel。report_id / policy_id / auto_policy_id /
 * view 均随 URL 深链；当前页存在 pending / running 报告时每 5s 静默刷新；
 * 策略表单脏时切视图走未保存确认。
 * Inspections panel: a "records | auto" segmented view. The records view keeps
 * status segments (server-side, no bucket counts) + severity select + cluster
 * Combobox + TableCard list + persistent rail / drawer; the auto view reuses
 * PoliciesPanel. report_id / policy_id / auto_policy_id / view stay deep-linked
 * via the URL; the list silently refreshes every 5s while pending / running
 * reports remain; switching views with a dirty policy form confirms first.
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Play, RefreshCw, Settings2, Trash2, X} from 'lucide-react';
import {toast} from '@/lib/toast';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Select} from '@/components/arc/select/select';
import {Tooltip} from '@/components/arc/tooltip/tooltip';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  DiagnosticsClusterOption,
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReport,
  DiagnosticsTask,
} from '@/lib/services/diagnostics';
import {cn} from '@/lib/utils';
import {DiagnosticsClusterFilter} from '../DiagnosticsClusterFilter';
import {PoliciesPanel} from '../policies/PoliciesPanel';
import {InspectionReportDetail} from './InspectionReportDetail';
import {InspectionCleanupSettingsDialog} from './InspectionCleanupSettingsDialog';
import {StartInspectionDialog} from './StartInspectionDialog';
import {
  inspectionStatusTone,
  isLiveInspectionStatus,
  type InspectionSeverityFilter,
  type InspectionStatusFilter,
} from './inspection-utils';
import {formatErrorDateTime} from '../errors/error-utils';
import {localizeDiagnosticsText} from '../text-utils';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {DirtyCloseDialog} from '@/components/common/layout';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {BatchBar, usePagedSelection} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';

const PAGE_SIZE = 10;
const AUTO_REFRESH_MS = 5000;
const AUTO_POLICIES_FETCH_SIZE = 100;

export type InspectionsView = 'records' | 'auto';

export interface InspectionsPanelProps {
  /** 工作台所选集群：列表按集群预过滤，也是「发起巡检」的目标集群。Selected workspace cluster; pre-filters the list and is the target of "run inspection". */
  clusterId?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** URL 深链的目标巡检报告。Deep-linked inspection report id from the URL. */
  reportId?: number;
  onSelectReport?: (reportId: number | null) => void;
  /** 工具栏集群筛选变化时回填 cluster_id。Writes cluster_id back through the toolbar filter. */
  onClusterChange?: (clusterId: number | undefined) => void;
  /** 当前分段视图（records | auto），由工作台按 URL view 解析。Current segment, resolved from the URL view param. */
  view?: InspectionsView;
  onViewChange?: (view: InspectionsView) => void;
  /** URL 深链的目标自动巡检策略（auto 视图详情）。Deep-linked auto policy id for the auto view. */
  policyId?: number;
  onSelectPolicy?: (policyId: number | null) => void;
  /** 记录视图的 auto_policy_id 过滤。Records view auto_policy_id filter. */
  autoPolicyId?: number;
  onAutoPolicyFilter?: (policyId: number | null) => void;
  /** 「自动 · 规则名」徽章点击：跳到自动视图并打开策略。Auto chip click: jump to the auto view and open the policy. */
  onOpenPolicy?: (policyId: number) => void;
  /** 策略详情「查看触发记录」：回记录视图并应用 auto_policy_id。Policy detail "view trigger records": back to records filtered by the policy. */
  onViewPolicyRecords?: (policyId: number) => void;
  /** 向工作台上报策略表单脏态（顶层标签切换拦截用）。Reports policy-form dirty state for top-level tab interception. */
  onDirtyChange?: (dirty: boolean) => void;
}

export function InspectionsPanel({
  clusterId,
  clusterOptions,
  reportId,
  onSelectReport,
  onClusterChange,
  view = 'records',
  onViewChange,
  policyId,
  onSelectPolicy,
  autoPolicyId,
  onAutoPolicyFilter,
  onOpenPolicy,
  onViewPolicyRecords,
  onDirtyChange,
}: InspectionsPanelProps) {
  const t = useTranslations('diagnosticsCenter.inspections');
  const commonT = useTranslations('common');
  const narrow = useDetailNarrow();

  const [statusFilter, setStatusFilter] =
    useState<InspectionStatusFilter>('all');
  const [severityFilter, setSeverityFilter] =
    useState<InspectionSeverityFilter>('all');
  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState('');
  const searchRef = useRef<HTMLInputElement>(null);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reports, setReports] = useState<DiagnosticsInspectionReport[]>([]);
  const [reportTotal, setReportTotal] = useState(0);
  // 自动策略 id → 名称映射：记录行的「自动 · 规则名」徽章与 auto_policy_id
  // 过滤徽章共用，只在进入本面板时拉一次。
  // Auto policy id -> name map shared by the "auto · <name>" row chip and the
  // auto_policy_id filter badge; fetched once on panel mount.
  const [policyNames, setPolicyNames] = useState<Map<number, string>>(
    new Map(),
  );
  // 策略表单脏态（auto 视图上报）+ 切视图拦截目标。Dirty policy form reported
  // by the auto view plus the suspended target of a view switch.
  const [policiesDirty, setPoliciesDirty] = useState(false);
  const [pendingView, setPendingView] = useState<InspectionsView | null>(null);

  const [selectedReportId, setSelectedReportId] = useState<number | null>(
    reportId ?? null,
  );
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedReport, setSelectedReport] =
    useState<DiagnosticsInspectionReport | null>(null);
  const [findings, setFindings] = useState<DiagnosticsInspectionFinding[]>([]);
  const [bundleTask, setBundleTask] = useState<DiagnosticsTask | null>(null);
  const [startDialogOpen, setStartDialogOpen] = useState(false);
  const [cleanupSettingsOpen, setCleanupSettingsOpen] = useState(false);
  // 勾选集合 + 批量删除状态；跨页勾选保留在 Set 里直到执行或手动清空。
  // Checkbox selection plus batch-delete state; selections live in the Set
  // across pages until executed or cleared.
  const pageIds = useMemo(() => reports.map((r) => r.id), [reports]);
  const {
    ids: selectedIds,
    setIds: setSelectedIds,
    toggle: toggleSelect,
    togglePage: toggleSelectAllPage,
    pageAllSelected,
    pageSomeSelected,
  } = usePagedSelection(pageIds);
  const [batchBusy, setBatchBusy] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [batchResult, setBatchResult] = useState<{
    deleted: number;
    skipped: number;
  } | null>(null);
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
          auto_policy_id: autoPolicyId,
          keyword: keyword || undefined,
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
    [autoPolicyId, clusterId, keyword, page, severityFilter, statusFilter, t],
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

  // 面板挂载时拉一次策略列表，只为徽章取名字（auto 视图自带独立拉取）。
  // Fetch the policy list once on mount — only the chips need names here; the
  // auto view loads its own copy.
  useEffect(() => {
    let active = true;
    void services.diagnostics
      .listAutoPoliciesSafe({page_size: AUTO_POLICIES_FETCH_SIZE})
      .then((result) => {
        if (!active || !result.success || !result.data) {
          return;
        }
        setPolicyNames(
          new Map(
            (result.data.items || []).map((policy) => [
              policy.id,
              policy.name,
            ]),
          ),
        );
      });
    return () => {
      active = false;
    };
  }, []);

  // 策略表单脏态上抛给工作台（顶层标签拦截）；视图切换本身在 requestView 里拦截。
  // Policy-form dirty state is forwarded to the workspace for tab interception;
  // view switches are intercepted locally in requestView.
  useEffect(() => {
    onDirtyChange?.(policiesDirty);
  }, [policiesDirty, onDirtyChange]);

  const reportPoliciesDirty = useCallback((dirty: boolean) => {
    setPoliciesDirty(dirty);
  }, []);

  const requestView = useCallback(
    (next: InspectionsView) => {
      if (next === view) {
        return;
      }
      if (policiesDirty) {
        setPendingView(next);
        return;
      }
      onViewChange?.(next);
    },
    [view, policiesDirty, onViewChange],
  );

  const commitPendingView = useCallback(() => {
    const target = pendingView;
    setPendingView(null);
    setPoliciesDirty(false);
    if (target) {
      onViewChange?.(target);
    }
  }, [pendingView, onViewChange]);

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

  // 删除走服务端逐条判定：在跑记录/绑定任务未终态的会被跳过并回传原因。
  // Deletion is decided per record server-side: running reports or bound tasks
  // that are not terminal come back skipped with a reason.
  const runBatchDelete = useCallback(async () => {
    const ids = Array.from(selectedIds);
    if (ids.length === 0 || batchBusy) {
      return;
    }
    setBatchBusy(true);
    try {
      const result =
        await services.diagnostics.batchDeleteInspectionReportsSafe(ids);
      if (!result.success || !result.data) {
        toast.error(result.error || t('batchDeleteError'));
        return;
      }
      const {deleted, skipped, results} = result.data;
      const deletedIds = new Set(
        (results || [])
          .filter((item) => item.result === 'deleted')
          .map((item) => item.id),
      );
      // 打开中的详情行若被删掉，同步关闭详情栏。
      // Close the detail rail when the open row was among the deleted ones.
      if (selectedReportId != null && deletedIds.has(selectedReportId)) {
        handleCloseDetail();
      }
      setSelectedIds(new Set());
      setBatchResult({deleted, skipped});
      if (skipped > 0) {
        toast.warning(
          t('batchDeletePartial', {deleted, skipped}),
        );
      } else {
        toast.success(t('batchDeleteSuccess', {count: deleted}));
      }
      setPage(1);
      void loadReports();
    } finally {
      setBatchBusy(false);
    }
  }, [
    batchBusy,
    handleCloseDetail,
    loadReports,
    selectedIds,
    selectedReportId,
    t,
  ]);

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

  // 「/」聚焦搜索，与错误面板一致。「/」 focuses the search field, same as the errors panel.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        e.key === '/' &&
        !el?.closest('input, textarea, select, [contenteditable="true"]')
      ) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const commitKeyword = useCallback(() => {
    setKeyword(keywordInput.trim());
    setPage(1);
  }, [keywordInput]);

  const filtersActive =
    statusFilter !== 'all' ||
    severityFilter !== 'all' ||
    autoPolicyId != null ||
    keyword !== '';
  const resetAllFilters = useCallback(() => {
    setStatusFilter('all');
    setSeverityFilter('all');
    setKeywordInput('');
    setKeyword('');
    onAutoPolicyFilter?.(null);
    setPage(1);
  }, [onAutoPolicyFilter]);
  const seg = (key: InspectionStatusFilter, label: string) => ({
    value: key,
    label,
  });

  // 策略名展示：优先映射表命中，退化 #id（策略可能已被删除）。
  // Policy display name: map hit first, #id fallback for deleted policies.
  const policyLabel = useCallback(
    (id: number) => policyNames.get(id) ?? `#${id}`,
    [policyNames],
  );

  // 分段切换同一 layoutGroup：记录 / 自动两处挂载点共享过渡（同通知面板）。
  // Shared layoutGroup gives the records / auto mount points one morph
  // transition, mirroring the notify panel.
  const viewSwitch = (
    <SegmentedControl
      label={t('viewLabel')}
      value={view}
      onValueChange={(v) => requestView(v as InspectionsView)}
      layoutGroup='diagnostics-view'
      options={[
        {value: 'records', label: t('viewRecords')},
        {value: 'auto', label: t('viewAuto')},
      ]}
    />
  );

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

  if (view === 'auto') {
    return (
      <>
        <PoliciesPanel
          clusterId={clusterId}
          clusterOptions={clusterOptions}
          onDirtyChange={reportPoliciesDirty}
          viewSwitch={viewSwitch}
          focusPolicyId={policyId}
          onSelectPolicy={onSelectPolicy}
          onViewRecords={onViewPolicyRecords}
          onClusterChange={onClusterChange}
        />
        <DirtyCloseDialog
          open={pendingView !== null}
          onDiscard={commitPendingView}
          onKeepEditing={() => setPendingView(null)}
        />
      </>
    );
  }

  return (
    <div className={layout.page} style={{overflowY: 'auto'}}>
      <div className={layout.toolbar}>
        {viewSwitch}
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
        <div className={layout.toolbarSearch}>
          <SearchField
            ref={searchRef}
            label={t('searchLabel')}
            value={keywordInput}
            onValueChange={(v) => {
              setKeywordInput(v);
              // 清空输入即移除已提交关键字；新关键字仍按 Enter 提交。
              // Clearing the input also drops the committed keyword; new
              // keywords still submit on Enter.
              if (v === '' && keyword !== '') {
                setKeyword('');
                setPage(1);
              }
            }}
            placeholder={t('keywordPlaceholder')}
            clearLabel={t('searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitKeyword();
              } else if (e.key === 'Escape' && keywordInput) {
                setKeywordInput('');
                if (keyword !== '') {
                  setKeyword('');
                  setPage(1);
                }
              }
            }}
          />
        </div>
        <div className={cn(layout.filterCtrl, layout.filterWsm)}>
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
        <DiagnosticsClusterFilter
          value={clusterId}
          clusterOptions={clusterOptions}
          onChange={onClusterChange}
        />
        {autoPolicyId ? (
          <SoftChip tone='blue' className='gap-1'>
            {t('policyFilterChip', {name: policyLabel(autoPolicyId)})}
            <button
              type='button'
              aria-label={commonT('clearFilters')}
              className='inline-flex items-center'
              onClick={() => onAutoPolicyFilter?.(null)}
              data-testid='inspection-policy-filter-clear'
            >
              <X className='size-3' aria-hidden />
            </button>
          </SoftChip>
        ) : null}
        {filtersActive ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={resetAllFilters}
          >
            <X className='size-3' aria-hidden />
            {t('resetFilters')}
          </Button>
        ) : null}
        <div className={layout.toolbarRight}>
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
            onClick={() => setCleanupSettingsOpen(true)}
            data-testid='inspection-cleanup-settings'
          >
            <Settings2 className='size-3.5' aria-hidden />
            {t('cleanupSettings.title')}
          </Button>
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

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='inspections'>
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
                      onClick={resetAllFilters}
                    >
                      {commonT('clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            {/* rowHead 内含全选 checkbox，不能再挂 aria-hidden（SR 会找不到它）。
                rowHead now contains the select-all checkbox, so aria-hidden is
                off — otherwise screen readers would miss the control. */}
            <div className={layout.rowHead}>
              <Checkbox
                checked={
                  pageAllSelected
                    ? true
                    : pageSomeSelected
                      ? 'indeterminate'
                      : false
                }
                onCheckedChange={(v) => toggleSelectAllPage(v === true)}
                aria-label={t('selectAll')}
              />
              <span>{t('columns.id')}</span>
              <span>{t('columns.trigger')}</span>
              <span>{t('columns.status')}</span>
              <span>{t('columns.findings')}</span>
              <span>{t('columns.lookback')}</span>
              <span>{t('columns.finishedAt')}</span>
            </div>
            <div className={layout.tableScroll}>
              {reports.map((report) => (
                <div
                  key={report.id}
                  data-mrow={report.id}
                  data-testid={`inspection-row-${report.id}`}
                  role='button'
                  tabIndex={0}
                  onClick={(e) => {
                    // 勾选框/徽章点击不触发行详情。
                    // Checkbox/chip clicks must not open the row detail.
                    if (
                      (e.target as HTMLElement).closest(
                        'button, a, input, select, [role="menu"], [role="checkbox"]',
                      )
                    ) {
                      return;
                    }
                    openReportDetail(report);
                  }}
                  onKeyDown={(e) => onRowKeyDown(e, report.id)}
                  className={cn(
                    layout.row,
                    selectedReportId === report.id && layout.rowActive,
                  )}
                >
                  <Checkbox
                    checked={selectedIds.has(report.id)}
                    onCheckedChange={(v) =>
                      toggleSelect(report.id, v === true)
                    }
                    aria-label={t('selectRow', {id: report.id})}
                    data-testid={`inspection-select-${report.id}`}
                  />
                  <span className={cn(layout.sub, 'font-mono')}>
                    #{report.id}
                  </span>
                  {report.trigger_source === 'auto' && report.auto_policy_id ? (
                    /* 自动触发的报告挂「自动 · 规则名」徽章，点击直达该策略详情。
                       Auto-triggered reports show an "auto · <policy>" chip that
                       jumps straight to the policy detail. */
                    <button
                      type='button'
                      className={cn(layout.chipBtn, 'min-w-0 max-w-full')}
                      title={t('trigger.autoChip', {
                        name: policyLabel(report.auto_policy_id),
                      })}
                      onClick={(e) => {
                        e.stopPropagation();
                        onOpenPolicy?.(report.auto_policy_id as number);
                      }}
                      data-testid={`inspection-auto-chip-${report.id}`}
                    >
                      {/* 策略名长于触发列时文字截断，完整名见按钮 title；不截断会溢出压到状态列。
                          Truncate long policy names to the trigger column (full name stays on the button title); otherwise the chip bleeds over the status column. */}
                      <SoftChip
                        tone='blue'
                        className='shrink min-w-0 max-w-full'
                      >
                        <span className='min-w-0 truncate'>
                          {t('trigger.autoChip', {
                            name: policyLabel(report.auto_policy_id),
                          })}
                        </span>
                      </SoftChip>
                    </button>
                  ) : (
                    <NeutralChip
                      title={t(`trigger.${report.trigger_source}`)}
                    >
                      {t(`trigger.${report.trigger_source}`)}
                    </NeutralChip>
                  )}
                  <SoftChip tone={inspectionStatusTone(report.status)}>
                    {t(`status.${report.status}`)}
                  </SoftChip>
                  <div className={layout.rowName}>
                    <span
                      className={layout.name}
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
                    <span className={layout.sub}>
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
                    className={cn(layout.sub, 'whitespace-nowrap font-mono')}
                  >
                    {t('lookbackValue', {
                      minutes: report.lookback_minutes || 30,
                    })}
                  </span>
                  <span
                    className={cn(layout.sub, 'whitespace-nowrap font-mono')}
                  >
                    {formatErrorDateTime(
                      report.finished_at || report.created_at,
                    )}
                  </span>
                </div>
              ))}
            </div>
          </TableCard>

          <BatchBar
            selectedCount={selectedIds.size}
            busy={batchBusy}
            result={
              batchResult
                ? t('batchDeleteResult', {
                    deleted: batchResult.deleted,
                    skipped: batchResult.skipped,
                  })
                : null
            }
            onClear={() => {
              setSelectedIds(new Set());
              setBatchResult(null);
            }}
          >
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={batchBusy}
              onClick={() => setDeleteConfirmOpen(true)}
              data-testid='inspection-batch-delete'
            >
              <Trash2 className='size-3.5' aria-hidden />
              {commonT('delete')}
            </Button>
          </BatchBar>
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
          />
        </DetailHost>
      </div>

      <StartInspectionDialog
        clusterId={clusterId}
        open={startDialogOpen}
        onOpenChange={setStartDialogOpen}
        onStarted={handleInspectionStarted}
      />

      <InspectionCleanupSettingsDialog
        open={cleanupSettingsOpen}
        onOpenChange={setCleanupSettingsOpen}
      />

      <AlertDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('batchDeleteConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('batchDeleteConfirmDesc', {count: selectedIds.size})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={batchBusy}>
              {commonT('cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={batchBusy}
              onClick={(e) => {
                e.preventDefault();
                setDeleteConfirmOpen(false);
                void runBatchDelete();
              }}
              data-testid='inspection-batch-delete-confirm'
            >
              {commonT('delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <DirtyCloseDialog
        open={pendingView !== null}
        onDiscard={commitPendingView}
        onKeepEditing={() => setPendingView(null)}
      />
    </div>
  );
}
