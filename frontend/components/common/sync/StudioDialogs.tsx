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
import {useEffect, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {ChevronDown, ChevronRight} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
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
  formatMetricDisplayValue,
  getJobSubmittedScript,
  getLogLineClass,
  toObject,
} from './sync-studio-utils';

const MonacoEditor = dynamic(() => import('@monaco-editor/react'), {
  ssr: false,
});

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
  return (
    <div className='grid max-h-[70vh] gap-4 overflow-auto pr-1 lg:grid-cols-[minmax(0,1fr)_360px]'>
      <div className='space-y-4'>
        <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
          <div className='flex items-center justify-between gap-3'>
            <div className='text-sm font-medium'>{t('conclusion')}</div>
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
          <div className='mt-2 text-sm text-muted-foreground'>
            {result.summary}
          </div>
        </div>

        <div className='grid gap-4 lg:grid-cols-2'>
          <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
            <div className='mb-3 text-sm font-medium'>{t('errors')}</div>
            {result.errors.length > 0 ? (
              <div className='space-y-2'>
                {result.errors.map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive'
                  >
                    {item}
                  </div>
                ))}
              </div>
            ) : (
              <div className='text-sm text-muted-foreground'>
                {t('noErrors')}
              </div>
            )}
          </div>
          <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
            <div className='mb-3 text-sm font-medium'>{t('warnings')}</div>
            {result.warnings.length > 0 ? (
              <div className='space-y-2'>
                {result.warnings.map((item, index) => (
                  <div
                    key={`${item}-${index}`}
                    className='rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
                  >
                    {item}
                  </div>
                ))}
              </div>
            ) : (
              <div className='text-sm text-muted-foreground'>
                {t('noWarnings')}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className='rounded-lg border border-border/60 bg-background/80 p-4'>
        <div className='mb-3 text-sm font-medium'>{t('connectionChecks')}</div>
        {checks.length > 0 ? (
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
                <div className='mt-2 text-sm text-muted-foreground'>
                  {check.message}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className='text-sm text-muted-foreground'>
            {t('noConnectionChecks')}
          </div>
        )}
      </div>
    </div>
  );
}

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
  useEffect(() => {
    setPerTableExpanded(shouldExpandPerTableByDefault);
  }, [job?.id, shouldExpandPerTableByDefault]);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noMetrics')}</div>
    );
  }
  if (metricGroups.length === 0) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noMetricsOutput')}
      </div>
    );
  }
  return (
    <div className='space-y-4 overflow-auto pr-1'>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {metricHighlights.map((item) => (
          <div
            key={item.label}
            className='rounded-lg border border-border/60 bg-background/80 p-4'
          >
            <div className='text-xs text-muted-foreground'>{item.label}</div>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className='mt-2 text-2xl font-semibold tracking-tight'>
                  {item.value}
                </div>
              </TooltipTrigger>
              <TooltipContent>{item.raw}</TooltipContent>
            </Tooltip>
          </div>
        ))}
      </div>

      {perTableRows.length > 0 ? (
        <div className='overflow-hidden rounded-lg border border-border/60 bg-background/80'>
          {pairedMetricRows.length > 0 ? (
            <>
              <div className='border-b border-border/50 bg-muted/20 px-3 py-2 text-sm font-medium'>
                {t('metricMappedView')}
              </div>
              <div className='max-h-[240px] overflow-auto border-b border-border/50'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow>
                      <TableHead>{t('metricPairSourceNode')}</TableHead>
                      <TableHead>{t('metricPairSourceTable')}</TableHead>
                      <TableHead>{t('metricPairSinkNode')}</TableHead>
                      <TableHead>{t('metricPairSinkTable')}</TableHead>
                      <TableHead>{t('metricSourceRows')}</TableHead>
                      <TableHead>{t('metricSourceBytes')}</TableHead>
                      <TableHead>{t('metricSourceQps')}</TableHead>
                      <TableHead>{t('metricSinkRows')}</TableHead>
                      <TableHead>{t('metricSinkBytes')}</TableHead>
                      <TableHead>{t('metricSinkQps')}</TableHead>
                      <TableHead>{t('metricCommittedRows')}</TableHead>
                      <TableHead>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pairedMetricRows.map((row) => (
                      <TableRow key={row.key}>
                        <TableCell className='text-xs font-medium'>
                          {row.sourceNode}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.sourceTable}
                        </TableCell>
                        <TableCell className='text-xs font-medium'>
                          {row.sinkNode}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.sinkTable}
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedCount}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
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
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : null}
          <div className='flex items-center justify-between gap-2 border-b border-border/50 bg-muted/20 px-3 py-2'>
            <div className='text-sm font-medium'>{t('metricPerTable')}</div>
            {pairedMetricRows.length > 0 ? (
              <button
                type='button'
                className='inline-flex items-center gap-1 rounded-md border border-border/60 bg-background px-2 py-1 text-xs text-muted-foreground hover:text-foreground'
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
              <div className='flex flex-wrap gap-2 border-b border-border/50 bg-background px-3 py-2 text-xs'>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-emerald-500' />
                  <span>{t('metricLegendSource')}</span>
                </div>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-blue-500' />
                  <span>{t('metricLegendWrite')}</span>
                </div>
                <div className='inline-flex items-center gap-2 rounded-md border border-border/50 px-2 py-1'>
                  <span className='size-2 rounded-full bg-amber-500' />
                  <span>{t('metricLegendCommitted')}</span>
                </div>
              </div>
              <div className='max-h-[320px] overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                    <TableRow>
                      <TableHead>{t('node')}</TableHead>
                      <TableHead>{t('table')}</TableHead>
                      <TableHead>{t('metricSourceRows')}</TableHead>
                      <TableHead>{t('metricSourceBytes')}</TableHead>
                      <TableHead>{t('metricSourceQps')}</TableHead>
                      <TableHead>{t('metricSinkRows')}</TableHead>
                      <TableHead>{t('metricSinkBytes')}</TableHead>
                      <TableHead>{t('metricSinkQps')}</TableHead>
                      <TableHead>{t('metricCommittedRows')}</TableHead>
                      <TableHead>{t('metricCommittedBytes')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {perTableRows.map((row) => (
                      <TableRow
                        key={row.rawTable}
                        className={cn(
                          row.rowTone === 'source' &&
                            'bg-emerald-50/50 dark:bg-emerald-500/5',
                          row.rowTone === 'sink' &&
                            'bg-blue-50/50 dark:bg-blue-500/5',
                        )}
                      >
                        <TableCell className='text-xs font-medium'>
                          {row.nodeLabel}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {row.tablePath}
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-emerald-700 dark:text-emerald-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sourceQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sourceQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkCount}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkBytes}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkBytes}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-blue-700 dark:text-blue-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.sinkQps}</span>
                            </TooltipTrigger>
                            <TooltipContent>{row.sinkQps}</TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>{row.committedCount}</span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {row.committedCount}
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='text-xs text-amber-700 dark:text-amber-300'>
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
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : (
            <div className='px-3 py-3 text-sm text-muted-foreground'>
              {t('perTableMetricsCollapsed')}
            </div>
          )}
        </div>
      ) : null}

      <div className='grid gap-4 lg:grid-cols-2'>
        {metricGroups.map((group) => (
          <div
            key={group.key}
            className='overflow-hidden rounded-lg border border-border/60 bg-background/80'
          >
            <div className='border-b border-border/50 bg-muted/20 px-3 py-2 text-sm font-medium'>
              {group.title}
            </div>
            <div className='max-h-[360px] overflow-auto'>
              <Table>
                <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/90'>
                  <TableRow>
                    <TableHead>{t('metric')}</TableHead>
                    <TableHead>{t('value')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {group.items.map((item) => (
                    <TableRow key={item.key}>
                      <TableCell className='font-mono text-xs'>
                        {item.key}
                      </TableCell>
                      <TableCell className='text-xs'>
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
