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
import {useEffect, useMemo, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  ArrowDownToLine,
  ArrowRight,
  BarChart3,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  Copy,
  Database,
  Gauge,
  Lightbulb,
  Search,
  Zap,
} from 'lucide-react';
import {toast} from 'sonner';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
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
import type {SyncJobInstance, SyncValidateResult} from '@/lib/services/sync';
import {
  buildMetricGroups,
  buildMetricHighlights,
  buildPairedMetricRows,
  buildPerTableMetricRows,
  formatJobDuration,
  formatMetricDisplayValue,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getJobSubmittedScript,
  getLogLineClass,
  getMetricValue,
  toObject,
  type UserFacingErrorState,
} from './sync-studio-utils';

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

      {/* 核心错误描述 */}
      <div className='rounded-md border border-destructive/15 bg-background/60 p-3 text-xs leading-relaxed text-foreground font-mono whitespace-pre-wrap break-all'>
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
            <pre className='max-h-60 overflow-auto rounded border border-border/50 bg-muted/40 p-3 font-mono text-[11px] leading-relaxed text-muted-foreground'>
              {error.raw}
            </pre>
          ) : null}
        </div>
      ) : null}
    </div>
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
  const checks = result.checks || [];
  const hasChecks = checks.length > 0;

  // 错误去重与智能过滤
  // Error deduplication and smart filtering
  const cleanSummary = (result.summary || '').replace(/^sync:\s*/, '').trim();
  const filteredErrors = (result.errors || []).filter((item) => {
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
          {result.errors.length > 0 ? (
            <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
              <div className='mb-3 flex items-center justify-between'>
                <div className='text-sm font-medium flex items-center gap-1.5'>
                  <AlertCircle className='size-3.5 text-destructive' />
                  <span>{t('errors')}</span>
                </div>
                <Badge variant='outline' className='text-[10px] text-destructive'>
                  {result.errors.length}
                </Badge>
              </div>
              <div className='space-y-2'>
                {(filteredErrors.length > 0 ? filteredErrors : result.errors).map((item, index) => (
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

          {result.warnings.length > 0 ? (
            <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
              <div className='mb-3 flex items-center justify-between'>
                <div className='text-sm font-medium flex items-center gap-1.5'>
                  <AlertTriangle className='size-3.5 text-amber-500' />
                  <span>{t('warnings')}</span>
                </div>
                <Badge variant='outline' className='text-[10px] text-amber-600 dark:text-amber-400'>
                  {result.warnings.length}
                </Badge>
              </div>
              <div className='space-y-2'>
                {result.warnings.map((item, index) => (
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
                    <div className='text-sm font-medium'>
                      {check.connector_type}
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

const METRIC_CARD_STYLES: Record<
  string,
  {
    gradient: string;
    icon: typeof Database;
    iconBg: string;
    iconColor: string;
    badgeLabel: string;
  }
> = {
  sourceRows: {
    gradient: 'from-emerald-500 to-teal-400',
    icon: Database,
    iconBg: 'bg-emerald-500/10 dark:bg-emerald-500/20',
    iconColor: 'text-emerald-600 dark:text-emerald-400',
    badgeLabel: '入站读取',
  },
  sinkRows: {
    gradient: 'from-indigo-500 to-blue-400',
    icon: ArrowDownToLine,
    iconBg: 'bg-indigo-500/10 dark:bg-indigo-500/20',
    iconColor: 'text-indigo-600 dark:text-indigo-400',
    badgeLabel: '出站写入',
  },
  committedRows: {
    gradient: 'from-purple-500 to-pink-400',
    icon: CheckCircle2,
    iconBg: 'bg-purple-500/10 dark:bg-purple-500/20',
    iconColor: 'text-purple-600 dark:text-purple-400',
    badgeLabel: '确认落盘',
  },
  readSpeed: {
    gradient: 'from-cyan-500 to-teal-400',
    icon: Activity,
    iconBg: 'bg-cyan-500/10 dark:bg-cyan-500/20',
    iconColor: 'text-cyan-600 dark:text-cyan-400',
    badgeLabel: '读取速率',
  },
  writeSpeed: {
    gradient: 'from-amber-500 to-orange-400',
    icon: Zap,
    iconBg: 'bg-amber-500/10 dark:bg-amber-500/20',
    iconColor: 'text-amber-600 dark:text-amber-400',
    badgeLabel: '写入速率',
  },
  writeQps: {
    gradient: 'from-rose-500 to-red-400',
    icon: Gauge,
    iconBg: 'bg-rose-500/10 dark:bg-rose-500/20',
    iconColor: 'text-rose-600 dark:text-rose-400',
    badgeLabel: '处理频次',
  },
};

export function MetricsDialogContent({job}: {job: SyncJobInstance | null}) {
  const t = useTranslations('workbenchStudio');
  const rawMetrics = toObject(job?.result_preview?.metrics);
  const metricGroups = buildMetricGroups(rawMetrics, t).filter(
    (group) => group.key !== 'read' && group.key !== 'write',
  );
  const metricHighlights = buildMetricHighlights(rawMetrics, t);
  const perTableRows = buildPerTableMetricRows(rawMetrics);
  const pairedMetricRows = buildPairedMetricRows(rawMetrics);
  const shouldExpandPerTableByDefault = pairedMetricRows.length === 0;
  const [perTableExpanded, setPerTableExpanded] = useState(
    shouldExpandPerTableByDefault,
  );
  const [tableSearch, setTableSearch] = useState('');

  useEffect(() => {
    setPerTableExpanded(shouldExpandPerTableByDefault);
  }, [job?.id, shouldExpandPerTableByDefault]);

  // 表名过滤联动
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

  const filteredPerTableRows = useMemo(() => {
    if (!tableSearch.trim()) {
      return perTableRows;
    }
    const q = tableSearch.trim().toLowerCase();
    return perTableRows.filter(
      (r) =>
        r.tablePath.toLowerCase().includes(q) ||
        r.nodeLabel.toLowerCase().includes(q),
    );
  }, [perTableRows, tableSearch]);

  if (!job) {
    return (
      <div className='flex items-center justify-center p-8 text-sm text-muted-foreground'>
        {t('noMetrics')}
      </div>
    );
  }
  if (metricGroups.length === 0 && metricHighlights.length === 0) {
    return (
      <div className='flex items-center justify-center p-8 text-sm text-muted-foreground'>
        {t('noMetricsOutput')}
      </div>
    );
  }

  // 计算数据流达成度（写入量 / 读取量）
  const srcCount = Number(getMetricValue(rawMetrics, 'SourceReceivedCount')) || 0;
  const sinkCount = Number(getMetricValue(rawMetrics, 'SinkWriteCount')) || 0;
  const syncRatio =
    srcCount > 0 ? Math.min(100, Math.max(0, Math.round((sinkCount / srcCount) * 100))) : null;

  return (
    <div className='space-y-4 overflow-auto pr-1'>
      {/* 1. 作业执行时况上下文条 */}
      <div className='flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border/60 bg-muted/20 px-3.5 py-2.5 shadow-xs'>
        <div className='flex items-center gap-2.5 flex-wrap'>
          <div className='flex items-center gap-1.5'>
            <span className='font-mono text-xs font-semibold text-foreground'>
              #{job.id}
            </span>
            {job.engine_job_id ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type='button'
                    className='inline-flex items-center rounded border border-border/60 bg-background px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground hover:text-foreground'
                    onClick={() => {
                      void navigator.clipboard.writeText(String(job.engine_job_id));
                      toast.success(t('copied'));
                    }}
                  >
                    Engine: {job.engine_job_id}
                    <Copy className='ml-1 size-2.5' />
                  </button>
                </TooltipTrigger>
                <TooltipContent>{t('copy')}</TooltipContent>
              </Tooltip>
            ) : null}
          </div>

          <div className='h-3.5 w-px bg-border/60' />

          {/* 状态指示灯与 Badge */}
          {(() => {
            const rawStatus = String(job.status || '').toUpperCase();
            const isSuccess = /SUCCESS|FINISHED|SAVEPOINT_DONE/.test(rawStatus);
            const isFailed = /FAILED|FAILING/.test(rawStatus);
            const isRunning = /RUNNING|DOING_SAVEPOINT/.test(rawStatus);
            return (
              <Badge
                variant='outline'
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-sm border px-2 py-0.5 text-[11px] font-medium tracking-tight',
                  getJobStatusBadgeClass(job.status),
                )}
              >
                <span
                  className={cn(
                    'size-1.5 rounded-full',
                    isSuccess && 'bg-emerald-500 shadow-xs shadow-emerald-500/50',
                    isFailed && 'bg-destructive shadow-xs shadow-destructive/50',
                    isRunning && 'bg-sky-500 animate-pulse',
                    !isSuccess && !isFailed && !isRunning && 'bg-muted-foreground',
                  )}
                />
                <span>{getJobStatusLabel(job.status)}</span>
              </Badge>
            );
          })()}

          <Badge variant='outline' className='font-mono text-[10px] px-1.5 py-0 uppercase'>
            {job.run_type || 'SYNC'}
          </Badge>
        </div>

        {/* 耗时与达成率 */}
        <div className='flex items-center gap-4 text-xs text-muted-foreground'>
          <div className='flex items-center gap-1 font-mono'>
            <Clock className='size-3 text-primary/70' />
            <span className='font-medium text-foreground'>
              {formatJobDuration(job.started_at, job.finished_at)}
            </span>
          </div>

          {syncRatio !== null ? (
            <div className='flex items-center gap-1.5'>
              <span className='text-[11px] text-muted-foreground'>吞吐达成:</span>
              <div className='flex items-center gap-1.5'>
                <div className='h-1.5 w-16 overflow-hidden rounded-full bg-muted'>
                  <div
                    className={cn(
                      'h-full rounded-full transition-all duration-300',
                      syncRatio >= 100 ? 'bg-emerald-500' : 'bg-primary',
                    )}
                    style={{width: `${syncRatio}%`}}
                  />
                </div>
                <span
                  className={cn(
                    'font-mono text-[11px] font-semibold',
                    syncRatio >= 100 ? 'text-emerald-600 dark:text-emerald-400' : 'text-foreground',
                  )}
                >
                  {syncRatio}%
                </span>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      {/* 2. 六联现代数据流度量磁贴 (Metric Highlights) */}
      <div className='grid gap-3 grid-cols-2 md:grid-cols-3 xl:grid-cols-6'>
        {metricHighlights.map((item) => {
          const style = METRIC_CARD_STYLES[item.key] || {
            gradient: 'from-primary to-primary/80',
            icon: BarChart3,
            iconBg: 'bg-primary/10',
            iconColor: 'text-primary',
            badgeLabel: '指标',
          };
          const IconComp = style.icon;

          return (
            <div
              key={item.label}
              className='relative overflow-hidden rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs transition-all hover:border-border hover:shadow-xs'
            >
              {/* 顶部彩色微渐变条 */}
              <div
                className={cn(
                  'absolute inset-x-0 top-0 h-[2.5px] bg-gradient-to-r',
                  style.gradient,
                )}
              />

              {/* 顶部图标与类型胶囊 */}
              <div className='flex items-center justify-between gap-1.5'>
                <div className='flex items-center gap-1.5 min-w-0'>
                  <div
                    className={cn(
                      'flex size-5 items-center justify-center rounded-sm shrink-0',
                      style.iconBg,
                      style.iconColor,
                    )}
                  >
                    <IconComp className='size-3' />
                  </div>
                  <span
                    className='truncate text-[11px] font-medium text-muted-foreground'
                    title={item.label}
                  >
                    {item.label}
                  </span>
                </div>
                <Badge
                  variant='secondary'
                  className='h-4 px-1 text-[9px] font-normal text-muted-foreground shrink-0'
                >
                  {style.badgeLabel}
                </Badge>
              </div>

              {/* 主数值 */}
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className='mt-2 font-mono text-xl font-bold tracking-tight text-foreground truncate'>
                    {item.value}
                  </div>
                </TooltipTrigger>
                <TooltipContent side='bottom' className='font-mono text-xs'>
                  精确原始值: {item.raw}
                </TooltipContent>
              </Tooltip>

              <div className='mt-0.5 text-[10px] font-mono text-muted-foreground/70 truncate'>
                原始: {item.raw}
              </div>
            </div>
          );
        })}
      </div>

      {/* 3. 映射链路表与分表统计表 */}
      {perTableRows.length > 0 ? (
        <div className='overflow-hidden rounded-lg border border-border/60 bg-background/80 shadow-xs'>
          {pairedMetricRows.length > 0 ? (
            <>
              {/* 映射表头部工具栏 */}
              <div className='flex items-center justify-between gap-3 border-b border-border/50 bg-muted/20 px-3 py-2'>
                <div className='flex items-center gap-2'>
                  <span className='text-xs font-semibold text-foreground'>
                    {t('metricMappedView')}
                  </span>
                  <Badge variant='outline' className='text-[10px] font-mono px-1.5 py-0'>
                    {filteredPairedRows.length} 条链路
                  </Badge>
                </div>
                <div className='relative w-44'>
                  <Search className='absolute left-2 top-1/2 -translate-y-1/2 size-3 text-muted-foreground' />
                  <Input
                    value={tableSearch}
                    onChange={(e) => setTableSearch(e.target.value)}
                    placeholder='按表名搜索...'
                    className='h-6 pl-7 text-[11px] bg-background'
                  />
                </div>
              </div>

              <div className='max-h-[260px] overflow-auto border-b border-border/50'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow className='hover:bg-transparent border-border/50'>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('metricPairSourceNode')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('metricPairSourceTable')}</TableHead>
                      <TableHead className='h-8 py-1 px-1 text-xs text-center w-6'></TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('metricPairSinkNode')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('metricPairSinkTable')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceBytes')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceQps')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkBytes')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkQps')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-amber-600 dark:text-amber-400'>{t('metricCommittedRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-amber-600 dark:text-amber-400'>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredPairedRows.length > 0 ? (
                      filteredPairedRows.map((row) => (
                        <TableRow key={row.key} className='border-border/40 hover:bg-muted/40'>
                          <TableCell className='py-1.5 px-2.5 text-xs font-medium text-muted-foreground'>
                            {row.sourceNode}
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs font-semibold text-foreground'>
                            {row.sourceTable}
                          </TableCell>
                          <TableCell className='py-1.5 px-1 text-center text-muted-foreground/50'>
                            <ArrowRight className='size-3 inline' />
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 text-xs font-medium text-muted-foreground'>
                            {row.sinkNode}
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs font-semibold text-foreground'>
                            {row.sinkTable}
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceCount}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceBytes}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceQps}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceQps}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkCount}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkBytes}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkQps}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkQps}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-amber-700 dark:text-amber-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.committedCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.committedCount}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-amber-700 dark:text-amber-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.committedBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.committedBytes}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={13} className='text-center py-6 text-xs text-muted-foreground'>
                          未找到匹配表名 &quot;{tableSearch}&quot; 的映射链路
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : null}

          {/* 分表明细表头部与折叠开关 */}
          <div className='flex items-center justify-between gap-2 border-b border-border/50 bg-muted/20 px-3 py-2'>
            <div className='flex items-center gap-2'>
              <span className='text-xs font-semibold text-foreground'>{t('metricPerTable')}</span>
              <Badge variant='outline' className='text-[10px] font-mono px-1.5 py-0'>
                {filteredPerTableRows.length} / {perTableRows.length} 表
              </Badge>
            </div>
            {pairedMetricRows.length > 0 ? (
              <button
                type='button'
                className='inline-flex items-center gap-1 rounded-md border border-border/60 bg-background px-2 py-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer'
                onClick={() => setPerTableExpanded((value) => !value)}
              >
                {perTableExpanded ? (
                  <ChevronDown className='size-3.5' />
                ) : (
                  <ChevronRight className='size-3.5' />
                )}
                <span>
                  {perTableExpanded
                    ? t('collapsePerTableMetrics')
                    : t('expandPerTableMetrics')}
                </span>
              </button>
            ) : null}
          </div>

          {perTableExpanded ? (
            <>
              <div className='flex flex-wrap items-center justify-between gap-2 border-b border-border/50 bg-background px-3 py-2 text-xs'>
                <div className='flex flex-wrap gap-2'>
                  <div className='inline-flex items-center gap-1.5 rounded-md border border-border/50 px-2 py-0.5 text-[11px]'>
                    <span className='size-2 rounded-full bg-emerald-500' />
                    <span>{t('metricLegendSource')}</span>
                  </div>
                  <div className='inline-flex items-center gap-1.5 rounded-md border border-border/50 px-2 py-0.5 text-[11px]'>
                    <span className='size-2 rounded-full bg-blue-500' />
                    <span>{t('metricLegendWrite')}</span>
                  </div>
                  <div className='inline-flex items-center gap-1.5 rounded-md border border-border/50 px-2 py-0.5 text-[11px]'>
                    <span className='size-2 rounded-full bg-amber-500' />
                    <span>{t('metricLegendCommitted')}</span>
                  </div>
                </div>

                {pairedMetricRows.length === 0 ? (
                  <div className='relative w-44'>
                    <Search className='absolute left-2 top-1/2 -translate-y-1/2 size-3 text-muted-foreground' />
                    <Input
                      value={tableSearch}
                      onChange={(e) => setTableSearch(e.target.value)}
                      placeholder='按表名搜索...'
                      className='h-6 pl-7 text-[11px] bg-background'
                    />
                  </div>
                ) : null}
              </div>

              <div className='max-h-[320px] overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow className='hover:bg-transparent border-border/50'>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('node')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold'>{t('table')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceBytes')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>{t('metricSourceQps')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkBytes')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-indigo-600 dark:text-indigo-400'>{t('metricSinkQps')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-amber-600 dark:text-amber-400'>{t('metricCommittedRows')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs font-semibold text-amber-600 dark:text-amber-400'>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredPerTableRows.length > 0 ? (
                      filteredPerTableRows.map((row) => (
                        <TableRow
                          key={row.rawTable}
                          className={cn(
                            'border-border/40 hover:bg-muted/40',
                            row.rowTone === 'source' &&
                              'bg-emerald-50/30 dark:bg-emerald-500/5',
                            row.rowTone === 'sink' &&
                              'bg-blue-50/30 dark:bg-blue-500/5',
                          )}
                        >
                          <TableCell className='py-1.5 px-2.5 text-xs font-medium text-muted-foreground'>
                            {row.nodeLabel}
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs font-semibold text-foreground'>
                            {row.tablePath}
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceCount}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceBytes}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-emerald-700 dark:text-emerald-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sourceQps}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sourceQps}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkCount}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkBytes}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-blue-700 dark:text-blue-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.sinkQps}</span>
                              </TooltipTrigger>
                              <TooltipContent>{row.sinkQps}</TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-amber-700 dark:text-amber-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.committedCount}</span>
                              </TooltipTrigger>
                              <TooltipContent>
                                {row.committedCount}
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell className='py-1.5 px-2.5 font-mono text-xs text-amber-700 dark:text-amber-300'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>{row.committedBytes}</span>
                              </TooltipTrigger>
                              <TooltipContent>
                                {row.committedBytes}
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={10} className='text-center py-6 text-xs text-muted-foreground'>
                          未找到匹配表名 &quot;{tableSearch}&quot; 的分表统计
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : (
            <div className='px-3 py-3 text-xs text-muted-foreground'>
              {t('perTableMetricsCollapsed')}
            </div>
          )}
        </div>
      ) : null}

      {/* 4. 扩展分类度量（JVM、引擎特有指标等） */}
      {metricGroups.length > 0 ? (
        <div className='grid gap-4 lg:grid-cols-2'>
          {metricGroups.map((group) => (
            <div
              key={group.key}
              className='overflow-hidden rounded-lg border border-border/60 bg-background/80 shadow-xs'
            >
              <div className='border-b border-border/50 bg-muted/20 px-3 py-2 text-xs font-semibold text-foreground flex items-center justify-between'>
                <span>{group.title}</span>
                <Badge variant='outline' className='text-[10px] font-mono px-1 py-0'>
                  {group.items.length} 项
                </Badge>
              </div>
              <div className='max-h-[300px] overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow className='hover:bg-transparent border-border/50'>
                      <TableHead className='h-7 py-1 px-2.5 text-xs'>{t('metric')}</TableHead>
                      <TableHead className='h-7 py-1 px-2.5 text-xs text-right'>{t('value')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {group.items.map((item) => (
                      <TableRow key={item.key} className='border-border/40 hover:bg-muted/30'>
                        <TableCell className='py-1.5 px-2.5 font-mono text-xs text-muted-foreground'>
                          {item.key}
                        </TableCell>
                        <TableCell className='py-1.5 px-2.5 text-xs font-mono text-right font-medium text-foreground'>
                          {formatMetricDisplayValue(item.value)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          ))}
        </div>
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
        {t('noActualExecutedScript')}
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

export function VirtualizedLogViewer({
  lines,
  height,
  emptyText,
  emptyNode,
}: {
  lines: string[];
  height: number;
  emptyText: string;
  emptyNode?: ReactNode;
}) {
  const rowHeight = 20;
  const overscan = 24;
  const [scrollTop, setScrollTop] = useState(0);
  const startIndex = Math.max(Math.floor(scrollTop / rowHeight) - overscan, 0);
  const visibleCount = Math.ceil(height / rowHeight) + overscan * 2;
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
      className='overflow-auto rounded-lg border border-border/60 bg-background/80 p-0 font-mono text-xs'
      style={{height}}
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
