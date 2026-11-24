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

import dynamic from 'next/dynamic';
import {useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Copy,
  ExternalLink,
  Lightbulb,
  Search,
} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/ui/input';
import {Sparkline} from '@/components/charts/sparkline';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import {ExpandableTextPanel} from '@/components/common/layout';
import type {SyncJobInstance, SyncValidateResult} from '@/lib/services/sync';
import {
  buildMetricGroups,
  buildMetricFlowBar,
  buildPairedMetricRows,
  buildPerTableMetricRows,
  formatMetricCompactValue,
  formatMetricDisplayValue,
  formatMetricWithUnit,
  formatRateDisplay,
  getJobSubmittedScript,
  getLogLineClass,
  getMetricValue,
  isZeroMetricValue,
  summarizeMetricFlow,
  toObject,
  type MetricFlowBarSegmentKey,
  type MetricFlowTone,
  type UserFacingErrorState,
} from './sync-studio-utils';
import {useWriteRateSparkline} from './job-detail/metric-timeseries';

const MonacoEditor = dynamic(() => import('@monaco-editor/react'), {
  ssr: false,
});

/**
 * 专用的高信噪比错误诊断视图，消除重复文案，提供清晰层级与排查建议
 * High signal-to-noise error diagnostics view, eliminating repetitive text and providing actionable tips
 */
export function StudioErrorDiagnosticsView({
  error,
  className,
}: {
  error: UserFacingErrorState | null;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio');
  const [copied, setCopied] = useState(false);
  const [rawExpanded, setRawExpanded] = useState(false);

  if (!error) {
    return null;
  }

  const handleCopy = async () => {
    const textToCopy = error.raw
      ? `${error.title}\n${error.description}\n\n[Details]\n${error.raw}`
      : `${error.title}\n${error.description}`;
    try {
      await navigator.clipboard.writeText(textToCopy);
      setCopied(true);
      toast.success(t('variableValueCopied') || '已复制错误信息');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('复制失败');
    }
  };

  const getCategoryBadge = () => {
    switch (error.category) {
      case 'connector':
        return <Badge variant='outline' className='border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400'>{t('connectorMissingBadge')}</Badge>;
      case 'syntax':
        return <Badge variant='outline' className='border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400'>语法解析异常</Badge>;
      case 'schema':
        return <Badge variant='outline' className='border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'>库表结构不匹配</Badge>;
      case 'network':
        return <Badge variant='outline' className='border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400'>网络连接超时</Badge>;
      case 'auth':
        return <Badge variant='outline' className='border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400'>认证授权失败</Badge>;
      case 'runtime':
        return <Badge variant='outline' className='border-destructive/30 bg-destructive/10 text-destructive'>运行时错误</Badge>;
      default:
        return <Badge variant='outline' className='border-destructive/30 bg-destructive/10 text-destructive'>执行未通过</Badge>;
    }
  };

  return (
    <div className={cn('flex flex-col gap-3 rounded-lg border border-destructive/20 bg-destructive/[0.03] p-4', className)}>
      {/* 状态与操作横条 */}
      <div className='flex items-center justify-between gap-3'>
        <div className='flex items-center gap-2'>
          <AlertCircle className='size-4 text-destructive shrink-0' />
          <span className='text-sm font-semibold text-foreground'>{error.title}</span>
          {getCategoryBadge()}
        </div>
        <Button
          size='sm'
          variant='ghost'
          className='h-7 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground'
          onClick={handleCopy}
        >
          {copied ? <Check className='size-3.5 text-emerald-500' /> : <Copy className='size-3.5' />}
          <span>{copied ? '已复制' : '复制错误'}</span>
        </Button>
      </div>

      {/* 核心错误描述：限高内滚，避免长堆栈把弹窗撑出视口 */}
      {/* Core error text scrolls inside a cap so a long stack cannot stretch the dialog */}
      <div className='max-h-[40vh] overflow-auto rounded-md border border-destructive/15 bg-background/60 p-3 text-xs leading-relaxed text-foreground font-mono whitespace-pre-wrap break-all'>
        {error.description}
      </div>

      {/* 智能排查建议 */}
      {error.suggestion ? (
        <div className='flex items-start gap-2 rounded-md border border-amber-500/20 bg-amber-500/[0.06] p-3 text-xs text-amber-700 dark:text-amber-300'>
          <Lightbulb className='size-4 shrink-0 text-amber-600 dark:text-amber-400 mt-0.5' />
          <div className='flex-1 leading-relaxed'>
            <span className='font-medium'>排查建议：</span>
            {error.suggestion}
          </div>
        </div>
      ) : null}

      {/* 可直接修复的错误：新标签页打开，避免离开工作台丢掉未保存的草稿。 */}
      {/* Fixable errors open in a new tab so leaving the workbench never drops an unsaved draft. */}
      {error.action ? (
        <div className='flex justify-end'>
          <Button asChild size='sm' className='h-8 gap-1.5'>
            <a href={error.action.href} target='_blank' rel='noopener noreferrer'>
              {error.action.label}
              <ExternalLink className='size-3.5' />
            </a>
          </Button>
        </div>
      ) : null}

      {/* 详细技术堆栈（支持折叠） */}
      {error.raw && error.raw !== error.description ? (
        <div className='mt-1 space-y-1.5'>
          <button
            type='button'
            className='flex items-center gap-1 text-[11px] font-medium text-muted-foreground hover:text-foreground transition-colors'
            onClick={() => setRawExpanded((prev) => !prev)}
          >
            {rawExpanded ? <ChevronDown className='size-3.5' /> : <ChevronRight className='size-3.5' />}
            <span>{rawExpanded ? '收起完整调用堆栈' : '查看完整调用堆栈 (Stacktrace)'}</span>
          </button>
          {rawExpanded ? (
            <ExpandableTextPanel
              title='Stacktrace'
              content={error.raw}
              height={240}
              tone='plain'
              wrap={false}
            />
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/**
 * 运行 / 预览 / 恢复失败的限高诊断弹窗，与 DAG 失败共用同一套诊断视图。
 * Height-capped diagnostics dialog for run / preview / recover failures; same view as DAG failures.
 */
export function StudioActionErrorDialog({
  error,
  onOpenChange,
}: {
  error: UserFacingErrorState | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={error !== null} onOpenChange={onOpenChange}>
      <DialogContent
        aria-describedby={undefined}
        data-testid='studio-action-error-dialog'
        title={error?.title ?? ''}
        className='arc-compact flex h-auto max-h-[85vh]! w-[min(94vw,672px)]! flex-col overflow-hidden'
        bodyClassName='flex min-h-0 flex-1 flex-col overflow-hidden'
      >
        <div className='min-h-0 flex-1 overflow-auto pr-1'>
          <StudioErrorDiagnosticsView error={error} />
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function ValidationResultPanel({result}: {result: SyncValidateResult | null}) {
  const t = useTranslations('workbenchStudio');
  if (!result) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noValidationResults')}
      </div>
    );
  }
  // 列表字段统一兜底为数组：旧版后端在部分路径（如未定义模板变量）会返回 null。
  // Normalize list fields to arrays: older backends return null on some paths
  // (e.g. undefined template variables), which used to crash this panel.
  const errors = result.errors ?? [];
  const warnings = result.warnings ?? [];
  const checks = result.checks ?? [];
  const hasChecks = checks.length > 0;

  // 错误去重与智能过滤
  // Error deduplication and smart filtering
  const cleanSummary = (result.summary || '').replace(/^sync:\s*/, '').trim();
  const filteredErrors = errors.filter((item) => {
    const cleanItem = item.replace(/^sync:\s*/, '').trim();
    return cleanItem !== cleanSummary;
  });

  return (
    <div
      className={cn(
        'max-h-[70vh] overflow-auto pr-1',
        hasChecks
          ? 'grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]'
          : 'flex flex-col gap-4 max-w-3xl mx-auto w-full',
      )}
    >
      <div className='space-y-4'>
        {/* 校验结论卡片 */}
        <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
          <div className='flex items-center justify-between gap-3'>
            <div className='flex items-center gap-2'>
              {result.valid ? (
                <CheckCircle2 className='size-4 text-emerald-500' />
              ) : (
                <AlertCircle className='size-4 text-destructive' />
              )}
              <span className='text-sm font-medium'>{t('conclusion')}</span>
            </div>
            <Badge
              variant='outline'
              className={cn(
                'rounded-sm border px-2 py-0.5 text-[11px]',
                result.valid
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
                  : 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
              )}
            >
              {result.valid ? t('passed') : t('notPassed')}
            </Badge>
          </div>
          {cleanSummary ? (
            <div className='mt-2.5 rounded-md bg-muted/30 p-2.5 font-mono text-xs leading-relaxed text-foreground break-all'>
              {cleanSummary}
            </div>
          ) : null}
        </div>

        {/* 错误与警告列表 */}
        <div
          className={cn(
            'grid gap-4',
            hasChecks ? 'lg:grid-cols-2' : 'grid-cols-1',
          )}
        >
          {errors.length > 0 ? (
            <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
              <div className='mb-3 flex items-center justify-between'>
                <div className='text-sm font-medium flex items-center gap-1.5'>
                  <AlertCircle className='size-3.5 text-destructive' />
                  <span>{t('errors')}</span>
                </div>
                <Badge variant='outline' className='text-[10px] text-destructive'>
                  {errors.length}
                </Badge>
              </div>
              <div className='space-y-2'>
                {(filteredErrors.length > 0 ? filteredErrors : errors).map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-xs leading-relaxed text-destructive break-all font-mono'
                  >
                    {item.replace(/^sync:\s*/, '')}
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {warnings.length > 0 ? (
            <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
              <div className='mb-3 flex items-center justify-between'>
                <div className='text-sm font-medium flex items-center gap-1.5'>
                  <AlertTriangle className='size-3.5 text-amber-500' />
                  <span>{t('warnings')}</span>
                </div>
                <Badge variant='outline' className='text-[10px] text-amber-600 dark:text-amber-400'>
                  {warnings.length}
                </Badge>
              </div>
              <div className='space-y-2'>
                {warnings.map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-relaxed text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300 break-all'
                  >
                    {item}
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      </div>

      {/* 连接检查结果仅在有数据时呈现 */}
      {hasChecks ? (
        <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
          <div className='mb-3 flex items-center justify-between'>
            <div className='text-sm font-medium'>{t('connectionChecks')}</div>
            <Badge variant='outline' className='text-[10px]'>
              {checks.length}
            </Badge>
          </div>
          <div className='space-y-3'>
            {checks.map((check, index) => (
              <div
                key={`${check.node_id}-${check.connector_type}-${index}`}
                className='rounded-lg border border-border/50 bg-muted/15 p-3'
              >
                <div className='flex items-start justify-between gap-3'>
                  <div className='space-y-1'>
                    <div className='flex items-center gap-2'>
                      <span className='text-sm font-medium'>
                        {check.connector_type}
                      </span>
                      {check.probe_method ? (
                        <span className='inline-flex items-center rounded px-1.5 py-0.2 text-[10px] font-mono text-muted-foreground bg-muted/60 border border-border/40'>
                          {check.probe_method}
                        </span>
                      ) : null}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {check.node_id}
                    </div>
                  </div>
                  <Badge
                    variant='outline'
                    className={cn(
                      'rounded-sm capitalize',
                      check.status === 'success'
                        ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
                        : check.status === 'failed'
                          ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300'
                          : 'border-slate-200 bg-slate-50 text-slate-700 dark:border-slate-500/30 dark:bg-slate-500/10 dark:text-slate-300',
                    )}
                  >
                    {check.status}
                  </Badge>
                </div>
                {check.target ? (
                  <div className='mt-2 break-all rounded-md bg-muted/30 px-2 py-1 font-mono text-[11px] text-muted-foreground'>
                    {check.target}
                  </div>
                ) : null}
                <div className='mt-2 text-xs text-muted-foreground'>
                  {check.message}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** 结论句与状态点的色调：常态走静音色，异常才用 warning/danger 语义色。 / Tone colors for the conclusion and its dot: muted in normal state, warning/danger semantic tones only on anomalies. */
const FLOW_TONE_COLOR: Record<MetricFlowTone, string> = {
  neutral: 'var(--muted-foreground)',
  warning: 'var(--warning)',
  danger: 'var(--danger)',
};

/** 分段条与图例共用的段样式：已提交中性，待提交实心 warning，待写斜纹 warning。 / Segment styles shared by the flow bar and its legend: committed neutral, pending-commit solid warning, backlog hatched warning. */
const FLOW_BAR_STYLE: Record<MetricFlowBarSegmentKey, CSSProperties> = {
  committed: {background: 'var(--muted-foreground)'},
  pendingCommit: {background: 'var(--warning)'},
  backlog: {
    backgroundImage:
      'repeating-linear-gradient(45deg, color-mix(in oklab, var(--warning) 40%, transparent) 0 3px, transparent 3px 7px)',
  },
};

/** KPI 数字块：大数字 + 单位，悬停给出精确值与速率明细。 / KPI block: headline number + unit, hover for exact value and rate details. */
function MetricFlowKpi({
  label,
  value,
  unit,
  tooltip,
  children,
}: {
  label: string;
  value: string;
  unit?: string;
  tooltip?: ReactNode;
  children?: ReactNode;
}) {
  const content = (
    <div className='flex flex-col gap-1'>
      <span className='text-xs text-muted-foreground'>{label}</span>
      <span className='inline-flex items-end gap-3'>
        <span className='text-2xl font-medium leading-none tracking-tight tabular-nums text-foreground'>
          {value}
          {unit ? (
            <small className='ml-1 text-xs font-normal text-muted-foreground'>
              {unit}
            </small>
          ) : null}
        </span>
        {children}
      </span>
    </div>
  );
  if (!tooltip) {
    return content;
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className='cursor-default'>{content}</div>
      </TooltipTrigger>
      <TooltipContent side='bottom' className='text-xs'>
        {tooltip}
      </TooltipContent>
    </Tooltip>
  );
}

/** 可折叠明细行：标题 + 静音 hint，内容展开时才挂载。 / Collapsible detail row: title + muted hint; children mount only when expanded. */
function MetricFoldRow({
  title,
  hint,
  expanded,
  onToggle,
  children,
}: {
  title: string;
  hint?: string;
  expanded: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div>
      <button
        type='button'
        onClick={onToggle}
        aria-expanded={expanded}
        className='flex w-full items-center gap-2 py-1 text-left'
      >
        {expanded ? (
          <ChevronDown className='size-3.5 shrink-0 text-muted-foreground' />
        ) : (
          <ChevronRight className='size-3.5 shrink-0 text-muted-foreground' />
        )}
        <span className='text-sm font-medium text-foreground'>{title}</span>
        {hint ? (
          <span className='truncate text-xs text-muted-foreground'>{hint}</span>
        ) : null}
      </button>
      {expanded ? <div className='mt-3'>{children}</div> : null}
    </div>
  );
}

export function MetricsDialogContent({job}: {job: SyncJobInstance | null}) {
  const t = useTranslations('workbenchStudio');
  const rawMetrics = toObject(job?.result_preview?.metrics);
  const allMetricGroups = buildMetricGroups(rawMetrics, t).filter(
    (group) => group.key !== 'read' && group.key !== 'write',
  );
  // 全为 0 的扩展指标收进「更多指标」一行，非零指标照常分组展示。
  // All-zero extended metrics fold into the "more metrics" row; non-zero metrics keep their groups.
  const metricGroups = allMetricGroups
    .map((group) => ({...group, items: group.items.filter((item) => !isZeroMetricValue(item.value))}))
    .filter((group) => group.items.length > 0);
  const zeroMetricItems = allMetricGroups.flatMap((group) =>
    group.items.filter((item) => isZeroMetricValue(item.value)),
  );
  const perTableRows = buildPerTableMetricRows(rawMetrics);
  const pairedMetricRows = buildPairedMetricRows(rawMetrics);
  const [detailsExpanded, setDetailsExpanded] = useState(true);
  const [moreExpanded, setMoreExpanded] = useState(false);
  const [tableSearch, setTableSearch] = useState('');
  const sparklinePoints = useWriteRateSparkline(job);

  // 换作业时明细回到默认展开态、搜索清空。 / Per-table details reset to expanded and search clears when switching jobs.
  useEffect(() => {
    setDetailsExpanded(true);
    setMoreExpanded(false);
    setTableSearch('');
  }, [job?.id]);

  const filteredPairedRows = useMemo(() => {
    if (!tableSearch.trim()) {
      return pairedMetricRows;
    }
    const q = tableSearch.trim().toLowerCase();
    return pairedMetricRows.filter(
      (r) =>
        r.sourceTable.toLowerCase().includes(q) ||
        r.sinkTable.toLowerCase().includes(q) ||
        r.sourceNode.toLowerCase().includes(q) ||
        r.sinkNode.toLowerCase().includes(q),
    );
  }, [pairedMetricRows, tableSearch]);

  if (!job) {
    return (
      <div className='flex items-center justify-center p-8 text-sm text-muted-foreground'>
        {t('noMetrics')}
      </div>
    );
  }
  if (Object.keys(rawMetrics).length === 0) {
    return (
      <div className='flex items-center justify-center p-8 text-sm text-muted-foreground'>
        {t('noMetricsOutput')}
      </div>
    );
  }

  const status = String(job.status || '').toUpperCase();
  const flow = summarizeMetricFlow(rawMetrics, {
    failed: /FAILED|FAILING/.test(status),
  });
  const bar = buildMetricFlowBar(
    flow.readCount,
    flow.writeCount,
    flow.committedCount,
  );
  const conclusion = flow.segments
    .map((segment) => t(segment.key, segment.values))
    .join(' · ');
  const toneColor = FLOW_TONE_COLOR[flow.tone];
  const barAria = bar.segments
    .filter((segment) => segment.value > 0)
    .map(
      (segment) =>
        `${t(`metricFlowBar_${segment.key}`)} ${formatMetricDisplayValue(segment.value)}`,
    )
    .join(' · ');

  const readRowsRaw = formatMetricDisplayValue(
    getMetricValue(rawMetrics, 'SourceReceivedCount'),
  );
  const writeRowsRaw = formatMetricDisplayValue(
    getMetricValue(rawMetrics, 'SinkWriteCount'),
  );
  const committedRowsRaw = formatMetricDisplayValue(
    getMetricValue(rawMetrics, 'SinkCommittedCount'),
  );
  const readSpeed = formatMetricWithUnit(
    getMetricValue(rawMetrics, 'SourceReceivedBytesPerSeconds'),
    'bps',
  );
  const writeSpeed = formatMetricWithUnit(
    getMetricValue(rawMetrics, 'SinkWriteBytesPerSeconds'),
    'bps',
  );
  const readQps = formatMetricWithUnit(
    getMetricValue(rawMetrics, 'SourceReceivedQPS'),
    'qps',
  );

  const moreHintParts: string[] = [];
  if (perTableRows.length > 0) {
    moreHintParts.push(t('metricFlowEndpoints', {count: perTableRows.length}));
  }
  if (metricGroups.length > 0) {
    moreHintParts.push(t('metricFlowMoreGroups', {count: metricGroups.length}));
  }
  if (zeroMetricItems.length > 0) {
    moreHintParts.push(
      t('metricFlowMoreZero', {count: zeroMetricItems.length}),
    );
  }

  return (
    <div className='space-y-5 overflow-auto pr-1 pt-3'>
      {/* KPI 行：读 / 写 / 写速 + 一句状态结论。精确值、读写速率与 QPS 都收进悬停提示。 */}
      {/* KPI row: read / written / write-rate plus a single-line conclusion. Exact values, rates and QPS live in tooltips. */}
      <div className='flex flex-wrap items-end gap-x-8 gap-y-3'>
        <MetricFlowKpi
          label={t('metricFlowKpiRead')}
          value={formatMetricDisplayValue(flow.readCount)}
          unit={t('metricFlowRowsUnit')}
          tooltip={
            <>
              {t('metricFlowExact')}: {readRowsRaw} · {t('pipelineReadRate')}: {readSpeed} · QPS: {readQps}
            </>
          }
        />
        <MetricFlowKpi
          label={t('metricFlowKpiWrite')}
          value={formatMetricDisplayValue(flow.writeCount)}
          unit={t('metricFlowRowsUnit')}
          tooltip={
            <>
              {t('metricFlowExact')}: {writeRowsRaw} · {t('pipelineWriteRate')}: {writeSpeed} · {t('metricFlowCommitted')}: {committedRowsRaw}
            </>
          }
        />
        {/* 速率主值用行/秒：与旁边 sparkline 的 throughput_write（records/s）同口径，字节速率进悬停提示。 */}
        {/* The headline rate is rows/s to match the sparkline's throughput_write (records/s); bytes/s moves to the tooltip. */}
        <MetricFlowKpi
          label={t('pipelineWriteRate')}
          value={formatRateDisplay(getMetricValue(rawMetrics, 'SinkWriteQPS') as number | string | null | undefined)}
          unit={t('metricFlowRowsPerSecondUnit')}
          tooltip={
            <>
              {t('metricFlowExact')}: {formatMetricDisplayValue(getMetricValue(rawMetrics, 'SinkWriteQPS'))} · {t('pipelineWriteRate')}: {writeSpeed}
            </>
          }
        >
          {sparklinePoints ? (
            <Sparkline
              className='mb-0.5 w-28 shrink-0 text-foreground/80'
              data={sparklinePoints.map((point) => point[1])}
              labels={sparklinePoints.map((point) =>
                new Date(point[0]).toLocaleTimeString(),
              )}
              label={t('pipelineWriteRate')}
              variant='area'
              color='var(--muted-foreground)'
              height={26}
              interactive
              formatValue={(value) =>
                `${formatRateDisplay(value)} ${t('metricFlowRowsPerSecondUnit')}`
              }
            />
          ) : null}
        </MetricFlowKpi>
        <div className='ml-auto flex min-w-0 items-center gap-2 pb-0.5'>
          <span
            aria-hidden
            className='size-1.5 shrink-0 rounded-full'
            style={{background: toneColor}}
          />
          <span className='text-xs' style={{color: toneColor}}>
            {conclusion}
          </span>
        </div>
      </div>

      {/* 一根分段条替代原三端卡片：总长 = 读取量，写入拆成已提交 / 待提交，差额为待写；写超读时总长扩到写入量。 */}
      {/* One segmented bar replaces the three endpoint cards: scale is the read count (extended to
          write on overshoot), written rows split into committed / pending-commit, the gap is backlog. */}
      <div className='space-y-2'>
        <div
          role='img'
          aria-label={barAria}
          className='flex h-2.5 w-full gap-[2px] overflow-hidden rounded-full bg-muted/60'
        >
          {bar.segments.map((segment) =>
            segment.value > 0 ? (
              <div
                key={segment.key}
                className='h-full min-w-[2px] transition-[width] duration-500'
                style={{
                  width: `${segment.share * 100}%`,
                  ...FLOW_BAR_STYLE[segment.key],
                }}
              />
            ) : null,
          )}
        </div>
        <div className='flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-muted-foreground'>
          {bar.segments.map((segment) =>
            segment.value > 0 ? (
              <span key={segment.key} className='inline-flex items-center gap-1.5'>
                <span
                  aria-hidden
                  className='size-2 shrink-0 rounded-[2px]'
                  style={FLOW_BAR_STYLE[segment.key]}
                />
                {t(`metricFlowBar_${segment.key}`)}
                <span className='font-medium tabular-nums text-foreground'>
                  {formatMetricDisplayValue(segment.value)}
                </span>
              </span>
            ) : null,
          )}
          {bar.overWriteCount > 0 ? (
            <span>
              {t('pipelineExtraWrite', {
                count: formatMetricCompactValue(bar.overWriteCount),
              })}
            </span>
          ) : null}
          {bar.total === 0 ? <span>{t('metricFlowEmpty')}</span> : null}
        </div>
      </div>

      {/* 逐表明细：默认展开的配对链路表；来源/目标合并成一列「链路」，字节与提交量进悬停提示。 */}
      {/* Per-table details: the paired link table expanded by default; source/sink merge into one
          link column while bytes and committed counts move into a hover tooltip. */}
      {pairedMetricRows.length > 0 ? (
        <MetricFoldRow
          title={t('metricFlowPerTable')}
          hint={t('pipelineLinkCount', {count: pairedMetricRows.length})}
          expanded={detailsExpanded}
          onToggle={() => setDetailsExpanded((value) => !value)}
        >
          {pairedMetricRows.length > 5 ? (
            <div className='relative mb-2 w-56'>
              <Search className='absolute left-2 top-1/2 -translate-y-1/2 size-3 text-muted-foreground' />
              <Input
                value={tableSearch}
                onChange={(event) => setTableSearch(event.target.value)}
                placeholder={t('pipelineSearchTable')}
                className='h-7 pl-7 text-xs bg-background'
              />
            </div>
          ) : null}
          <div className='max-h-[320px] overflow-y-auto'>
            <Table className='table-fixed'>
              <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                <TableRow className='hover:bg-transparent border-border/50'>
                  <TableHead className='h-8 py-1 px-2 text-xs font-medium text-muted-foreground'>
                    {t('metricFlowLink')}
                  </TableHead>
                  <TableHead className='h-8 w-[88px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                    {t('metricFlowReadRows')}
                  </TableHead>
                  <TableHead className='h-8 w-[88px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                    {t('metricFlowWriteRows')}
                  </TableHead>
                  <TableHead className='h-8 w-[68px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                    QPS
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredPairedRows.length > 0 ? (
                  filteredPairedRows.map((row) => (
                    <TableRow key={row.key} className='border-border/40 hover:bg-muted/40'>
                      <TableCell className='py-1.5 px-2'>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <div className='flex min-w-0 cursor-default items-center gap-1.5 text-xs'>
                              <span className='shrink-0 text-muted-foreground'>
                                {row.sourceNode}
                              </span>
                              <span className='truncate font-mono text-foreground'>
                                {row.sourceTable}
                              </span>
                              <ArrowRight className='size-3 shrink-0 text-muted-foreground/60' />
                              <span className='shrink-0 text-muted-foreground'>
                                {row.sinkNode}
                              </span>
                              <span className='truncate font-mono text-foreground'>
                                {row.sinkTable}
                              </span>
                            </div>
                          </TooltipTrigger>
                          <TooltipContent className='max-w-[420px] text-xs'>
                            <div className='space-y-1'>
                              <div className='font-mono break-all'>
                                {row.sourceNode} · {row.sourceTable} → {row.sinkNode} · {row.sinkTable}
                              </div>
                              <div>
                                {t('metricSourceBytes')}: {row.sourceBytes} · {t('metricSinkBytes')}: {row.sinkBytes}
                              </div>
                              <div>
                                {t('metricFlowCommitted')}: {row.committedCount} ({row.committedBytes})
                                {row.countDiff !== null
                                  ? ` · ${t('metricCountDiff')}: ${row.countDiff.toLocaleString()}`
                                  : ''}
                              </div>
                            </div>
                          </TooltipContent>
                        </Tooltip>
                      </TableCell>
                      <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                        {row.sourceCount}
                      </TableCell>
                      <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                        {row.sinkCount}
                      </TableCell>
                      <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                        {formatRateDisplay(
                          row.sinkQps !== '-' ? row.sinkQps : row.sourceQps,
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={4} className='py-6 text-center text-xs text-muted-foreground'>
                      {t('metricFlowSearchEmpty', {query: tableSearch})}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        </MetricFoldRow>
      ) : null}

      {/* 更多指标：分表明细 + 非零扩展分组 + 全为 0 的指标合并进一个折叠行。 */}
      {/* More metrics: the per-endpoint table, non-zero extended groups and all-zero metrics fold into one row. */}
      {perTableRows.length > 0 || metricGroups.length > 0 || zeroMetricItems.length > 0 ? (
        <MetricFoldRow
          title={t('metricFlowMore')}
          hint={moreHintParts.join(' · ')}
          expanded={moreExpanded}
          onToggle={() => setMoreExpanded((value) => !value)}
        >
          <div className='space-y-5'>
            {perTableRows.length > 0 ? (
              <section>
                <div className='mb-1.5 flex items-center justify-between'>
                  <h4 className='text-xs font-medium text-muted-foreground'>
                    {t('metricPerTable')}
                  </h4>
                  <span className='text-xs text-muted-foreground'>
                    {t('metricFlowEndpoints', {count: perTableRows.length})}
                  </span>
                </div>
                <div className='max-h-[280px] overflow-y-auto'>
                  <Table className='table-fixed'>
                    <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                      <TableRow className='hover:bg-transparent border-border/50'>
                        <TableHead className='h-8 py-1 px-2 text-xs font-medium text-muted-foreground'>
                          {t('metricFlowLink')}
                        </TableHead>
                        <TableHead className='h-8 w-[88px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                          {t('metricFlowReadRows')}
                        </TableHead>
                        <TableHead className='h-8 w-[88px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                          {t('metricFlowWriteRows')}
                        </TableHead>
                        <TableHead className='h-8 w-[68px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                          QPS
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {perTableRows.map((row) => (
                        <TableRow key={row.rawTable} className='border-border/40 hover:bg-muted/40'>
                          <TableCell className='py-1.5 px-2'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className='flex min-w-0 cursor-default items-center gap-1.5 text-xs'>
                                  <span className='shrink-0 text-muted-foreground'>
                                    {row.nodeLabel}
                                  </span>
                                  <span className='truncate font-mono text-foreground'>
                                    {row.tablePath}
                                  </span>
                                </div>
                              </TooltipTrigger>
                              <TooltipContent className='max-w-[420px] text-xs'>
                                <div className='space-y-1'>
                                  <div className='font-mono break-all'>
                                    {row.nodeLabel} · {row.tablePath}
                                  </div>
                                  <div>
                                    {t('metricSourceBytes')}: {row.sourceBytes} · {t('metricSinkBytes')}: {row.sinkBytes}
                                  </div>
                                  <div>
                                    {t('metricFlowCommitted')}: {row.committedCount} ({row.committedBytes})
                                  </div>
                                </div>
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                            {row.sourceCount}
                          </TableCell>
                          <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                            {row.sinkCount}
                          </TableCell>
                          <TableCell className='py-1.5 px-2 text-right font-mono text-xs tabular-nums text-foreground'>
                            {formatRateDisplay(
                              row.sinkQps !== '-' ? row.sinkQps : row.sourceQps,
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </section>
            ) : null}

            {metricGroups.map((group) => (
              <section key={group.key}>
                <div className='mb-1.5 flex items-center justify-between'>
                  <h4 className='text-xs font-medium text-muted-foreground'>
                    {group.title}
                  </h4>
                  <span className='text-xs text-muted-foreground'>
                    {t('metricFlowItemCount', {count: group.items.length})}
                  </span>
                </div>
                <div className='max-h-[280px] overflow-y-auto'>
                  <Table>
                    <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                      <TableRow className='hover:bg-transparent border-border/50'>
                        <TableHead className='h-7 py-1 px-2 text-xs font-medium text-muted-foreground'>
                          {t('metric')}
                        </TableHead>
                        <TableHead className='h-7 w-[140px] py-1 px-2 text-right text-xs font-medium text-muted-foreground'>
                          {t('value')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {group.items.map((item) => (
                        <TableRow key={item.key} className='border-border/40 hover:bg-muted/30'>
                          <TableCell className='py-1.5 px-2 font-mono text-xs text-muted-foreground break-all'>
                            {item.key}
                          </TableCell>
                          <TableCell className='py-1.5 px-2 text-right font-mono text-xs font-medium tabular-nums text-foreground'>
                            {formatMetricDisplayValue(item.value)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </section>
            ))}

            {zeroMetricItems.length > 0 ? (
              <section>
                <h4 className='mb-1.5 text-xs font-medium text-muted-foreground'>
                  {t('metricFlowZeroTitle', {count: zeroMetricItems.length})}
                </h4>
                <p className='font-mono text-xs leading-6 text-muted-foreground break-all'>
                  {zeroMetricItems.map((item) => item.key).join(' · ')}
                </p>
              </section>
            ) : null}
          </div>
        </MetricFoldRow>
      ) : null}
    </div>
  );
}


export function JobScriptDialogContent({
  job,
  monacoTheme,
}: {
  job: SyncJobInstance | null;
  monacoTheme: string;
}) {
  const t = useTranslations('workbenchStudio');
  const script = getJobSubmittedScript(job);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noJobRuns')}</div>
    );
  }
  if (!script) {
    return (
      <div className='rounded-md border border-dashed border-border/60 bg-muted/20 px-4 py-6 text-sm text-muted-foreground'>
        {job.source === 'external'
          ? t('externalScriptEmpty')
          : t('noActualExecutedScript')}
      </div>
    );
  }
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <div className='flex items-center gap-2 text-xs text-muted-foreground'>
        <Badge variant='outline'>#{job.id}</Badge>
        <Badge variant='outline'>{script.format || 'hocon'}</Badge>
        <span className='truncate'>
          {job.platform_job_id || job.engine_job_id || '-'}
        </span>
      </div>
      <div className='min-h-0 flex-1 overflow-hidden rounded-lg border border-border/60'>
        <MonacoEditor
          height='100%'
          theme={monacoTheme}
          language={script.format === 'json' ? 'json' : 'shell'}
          value={script.content}
          options={{
            readOnly: true,
            minimap: {enabled: false},
            automaticLayout: true,
            fontSize: 13,
            scrollBeyondLastLine: false,
            wordWrap: 'on',
          }}
        />
      </div>
    </div>
  );
}

/**
 * 虚拟滚动日志查看器。不传 height 时铺满父容器（父级需是有高度的 flex 子项），按实测高度计算可见行数，
 * 避免固定高度在大屏弹窗里留下大片空白。
 * Virtualized log viewer. Without height it fills its parent (which must be a sized flex child) and computes the
 * visible rows from the measured height, so a fixed height no longer leaves a large blank area in tall dialogs.
 */
export function VirtualizedLogViewer({
  lines,
  height,
  emptyText,
  emptyNode,
}: {
  lines: string[];
  height?: number;
  emptyText: string;
  emptyNode?: ReactNode;
}) {
  const rowHeight = 20;
  const overscan = 24;
  const [scrollTop, setScrollTop] = useState(0);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [measuredHeight, setMeasuredHeight] = useState(600);
  const hasLines = lines.length > 0;
  useEffect(() => {
    const element = containerRef.current;
    if (height !== undefined || !element) {
      return;
    }
    const observer = new ResizeObserver(([entry]) => setMeasuredHeight(entry.contentRect.height));
    observer.observe(element);
    return () => observer.disconnect();
  }, [height, hasLines]);
  const viewportHeight = height ?? measuredHeight;
  const startIndex = Math.max(Math.floor(scrollTop / rowHeight) - overscan, 0);
  const visibleCount = Math.ceil(viewportHeight / rowHeight) + overscan * 2;
  const endIndex = Math.min(startIndex + visibleCount, lines.length);
  const visibleLines = lines.slice(startIndex, endIndex);
  if (lines.length === 0) {
    if (emptyNode) {
      return <>{emptyNode}</>;
    }
    return (
      <div className='rounded-lg border border-border/60 bg-background/80 p-4 text-sm text-muted-foreground'>
        {emptyText}
      </div>
    );
  }
  return (
    <div
      ref={containerRef}
      className={cn(
        'overflow-auto rounded-lg border border-border/60 bg-background/80 p-0 font-mono text-xs',
        height === undefined && 'h-full min-h-0',
      )}
      style={height !== undefined ? {height} : undefined}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <div style={{height: lines.length * rowHeight, position: 'relative'}}>
        <div
          style={{
            position: 'absolute',
            top: startIndex * rowHeight,
            left: 0,
            right: 0,
          }}
          className='px-4 py-3'
        >
          {visibleLines.map((line, index) => (
            <div
              key={`${startIndex + index}-${line.slice(0, 24)}`}
              className={cn('h-5 whitespace-pre', getLogLineClass(line))}
            >
              {line || ' '}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
