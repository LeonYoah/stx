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
import {useCallback, useMemo, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  Activity,
  AlertTriangle,
  Check,
  CheckCheck,
  Clock,
  Copy,
  Database,
  Eye,
  FileCode,
  HardDrive,
  Layers,
  Loader2,
  Minus,
  Plus,
  RefreshCw,
  Terminal,
  Zap,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import type {
  RuntimeStorageCheckpointInspectResult,
  RuntimeStorageListItem,
} from '@/lib/services/cluster/types';
import type {
  SyncCheckpointSnapshot,
  SyncJobInstance,
  SyncPreviewDataset,
  SyncPreviewSnapshot,
} from '@/lib/services/sync';
import {
  buildCheckpointActionViewModels,
  buildCheckpointInspectSummary,
  buildCheckpointSubtaskRows,
  extractCheckpointFileIdentity,
  formatCellValue,
  formatCheckpointFieldValue,
  formatSizeBytes,
  getCheckpointEnumBadgeClass,
  getPreviewRowKindBadgeClass,
  summarizeCheckpointSourceState,
  summarizeCheckpointSubtaskMetrics,
} from './sync-studio-utils';

const MonacoEditor = dynamic(() => import('@monaco-editor/react'), {
  ssr: false,
});

export function renderCheckpointFieldValue(key: string, value: unknown): ReactNode {
  if (typeof value === 'string') {
    if (/status/i.test(key)) {
      const isCompleted = /completed/i.test(value);
      const isFailed = /failed/i.test(value);
      const isInProgress = /progress|saving/i.test(value);

      return (
        <Badge
          variant='outline'
          className={cn(
            'inline-flex items-center gap-1.5 rounded-sm border px-2 py-0.5 text-[11px] font-medium tracking-tight',
            getCheckpointEnumBadgeClass(value, 'status'),
          )}
        >
          <span
            className={cn(
              'size-1.5 rounded-full',
              isCompleted && 'bg-emerald-500 shadow-xs shadow-emerald-500/50',
              isFailed && 'bg-destructive shadow-xs shadow-destructive/50',
              isInProgress && 'bg-blue-500 animate-pulse',
              !isCompleted && !isFailed && !isInProgress && 'bg-muted-foreground',
            )}
          />
          <span>{value}</span>
        </Badge>
      );
    }
    if (/checkpointType/i.test(key)) {
      return (
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm border px-2 py-0.5 text-[11px] font-mono',
            getCheckpointEnumBadgeClass(value, 'checkpointType'),
          )}
        >
          {value}
        </Badge>
      );
    }
  }
  if (typeof value === 'boolean') {
    return (
      <Badge
        variant='outline'
        className={cn(
          'rounded-sm border px-2 py-0.5 text-[11px] font-mono',
          getCheckpointEnumBadgeClass(value, 'boolean'),
        )}
      >
        {value ? 'true' : 'false'}
      </Badge>
    );
  }
  return (
    <span className='break-all font-mono text-xs'>{formatCheckpointFieldValue(key, value)}</span>
  );
}

export function PreviewWorkspacePanel({
  job,
  previewSnapshot,
  datasets,
  selectedDatasetName,
  previewPage,
  loading,
  monacoTheme,
  onSelectDataset,
  onChangePage,
}: {
  job: SyncJobInstance | null;
  previewSnapshot: SyncPreviewSnapshot | null;
  datasets: SyncPreviewDataset[];
  selectedDatasetName: string;
  previewPage: number;
  loading?: boolean;
  monacoTheme: string;
  onSelectDataset: (name: string) => void;
  onChangePage: (page: number) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [previewScriptOpen, setPreviewScriptOpen] = useState(false);
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>{t('noPreviewJobs')}</div>
    );
  }
  const activeDataset =
    datasets.find((dataset) => dataset.name === selectedDatasetName) ||
    datasets[0] ||
    null;
  const columns = activeDataset?.columns || [];
  const rows = (activeDataset?.rows || []) as Array<Record<string, unknown>>;
  const previewContent =
    typeof previewSnapshot?.injected_script === 'string' &&
    previewSnapshot.injected_script
      ? previewSnapshot.injected_script
      : typeof job.result_preview?.preview_content === 'string'
        ? job.result_preview.preview_content
        : '';
  const previewContentFormat =
    typeof previewSnapshot?.content_format === 'string' &&
    previewSnapshot.content_format
      ? previewSnapshot.content_format
      : typeof job.result_preview?.content_format === 'string'
        ? job.result_preview.content_format
        : 'hocon';
  const previewEmptyMessage =
    previewSnapshot?.empty_reason === 'preview_not_ready'
      ? t('preparingPreview')
      : t('noPreviewDataFallback');
  const pageSize = Math.max(activeDataset?.page_size || 20, 1);
  const total = Math.max(activeDataset?.total || rows.length, rows.length);
  const totalPages = Math.max(Math.ceil(total / pageSize), 1);
  const currentPage = Math.min(Math.max(previewPage, 1), totalPages);
  const pageRows = rows.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  return (
    <div className='grid h-full min-h-0 gap-3 lg:grid-cols-[220px_minmax(0,1fr)]'>
      <div className='flex min-h-0 flex-col rounded-lg border border-border/50 bg-muted/10 p-3'>
        <div className='mb-3 shrink-0 text-sm font-medium'>
          {t('tableTabs')}
        </div>
        <ScrollArea className='min-h-0 flex-1'>
          <div className='space-y-2 pr-2'>
            {datasets.length > 0 ? (
              datasets.map((dataset) => (
                <Tooltip key={dataset.name}>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      title={dataset.name}
                      className={cn(
                        'flex w-full items-center justify-between rounded-md border px-3 py-2 text-left text-sm',
                        dataset.name === activeDataset?.name
                          ? 'border-primary/30 bg-primary/5'
                          : 'border-border/50 bg-background/60',
                      )}
                      onClick={() => onSelectDataset(dataset.name)}
                    >
                      <span className='min-w-0 truncate'>{dataset.name}</span>
                      <Badge variant='outline'>
                        {dataset.total ?? (dataset.rows || []).length}
                      </Badge>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent
                    side='right'
                    className='max-w-[480px] break-all'
                  >
                    {dataset.name}
                  </TooltipContent>
                </Tooltip>
              ))
            ) : (
              <div className='rounded-md border border-border/50 bg-background/60 px-3 py-2 text-sm text-muted-foreground'>
                {t('noDatasets')}
              </div>
            )}
          </div>
        </ScrollArea>
      </div>
      <div className='flex min-h-0 flex-col rounded-lg border border-border/50 bg-background/70'>
        <div className='flex items-center justify-between border-b border-border/50 px-3 py-2 text-sm font-medium'>
          <span>{t('dataTable')}</span>
          <div className='flex items-center gap-2 text-xs text-muted-foreground'>
            {previewContent ? (
              <Button
                size='sm'
                variant='outline'
                className='h-7 px-2 text-xs'
                onClick={() => setPreviewScriptOpen(true)}
              >
                {t('injectedScript')}
              </Button>
            ) : null}
            <Button
              size='sm'
              variant='outline'
              className='h-7 px-2 text-xs'
              onClick={() => onChangePage(Math.max(currentPage - 1, 1))}
              disabled={currentPage <= 1}
            >
              {t('prevPage')}
            </Button>
            <span>
              {currentPage} / {totalPages}
            </span>
            <Button
              size='sm'
              variant='outline'
              className='h-7 px-2 text-xs'
              onClick={() =>
                onChangePage(Math.min(currentPage + 1, totalPages))
              }
              disabled={currentPage >= totalPages}
            >
              {t('nextPage')}
            </Button>
          </div>
        </div>
        {loading ? (
          <div className='flex min-h-0 flex-1 items-center justify-center'>
            <div className='flex items-center gap-2 text-sm text-muted-foreground'>
              <Loader2 className='size-4 animate-spin' />
              <span>{t('preparingPreview')}</span>
            </div>
          </div>
        ) : columns.length > 0 ? (
          <div className='min-h-0 flex-1 overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  {columns.map((column) => (
                    <TableHead key={column}>{column}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {pageRows.length > 0 ? (
                  pageRows.map((row, index) => (
                    <TableRow key={index}>
                      {columns.map((column) => (
                        <TableCell key={`${index}-${column}`}>
                          {column === 'RowKind' ? (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Badge
                                  variant='outline'
                                  className={cn(
                                    'max-w-full truncate rounded-sm',
                                    getPreviewRowKindBadgeClass(
                                      formatCellValue(row[column]),
                                    ),
                                  )}
                                >
                                  {formatCellValue(row[column])}
                                </Badge>
                              </TooltipTrigger>
                              <TooltipContent>
                                {formatCellValue(row[column])}
                              </TooltipContent>
                            </Tooltip>
                          ) : (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span className='block truncate'>
                                  {formatCellValue(row[column])}
                                </span>
                              </TooltipTrigger>
                              <TooltipContent className='max-w-[480px] break-all'>
                                {formatCellValue(row[column])}
                              </TooltipContent>
                            </Tooltip>
                          )}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell
                      colSpan={columns.length}
                      className='text-center text-muted-foreground'
                    >
                      {t('noPreviewDataFallback')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        ) : (
          <div className='flex min-h-0 flex-1 items-center justify-center p-3 text-sm text-muted-foreground'>
            {previewEmptyMessage}
          </div>
        )}
      </div>
      <Dialog open={previewScriptOpen} onOpenChange={setPreviewScriptOpen}>
        <DialogContent className='flex h-[86vh] w-[94vw] max-w-[94vw] flex-col overflow-hidden sm:max-w-[1380px]'>
          <DialogHeader>
            <DialogTitle>{t('injectedScript')}</DialogTitle>
            <DialogDescription>
              {previewContentFormat.toUpperCase()}
            </DialogDescription>
          </DialogHeader>
          <div className='min-h-0 flex-1 overflow-hidden rounded-md border border-border/50'>
            <MonacoEditor
              height='100%'
              language={previewContentFormat === 'json' ? 'json' : 'ini'}
              theme={monacoTheme}
              value={previewContent}
              options={{
                readOnly: true,
                minimap: {enabled: false},
                automaticLayout: true,
                wordWrap: 'on',
                scrollBeyondLastLine: false,
                fontSize: 13,
                renderLineHighlight: 'all',
                padding: {top: 14, bottom: 14},
              }}
            />
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function CheckpointWorkspacePanel({
  job,
  checkpointSnapshot,
  loading,
  checkpointFiles,
  checkpointFilesLoading,
  onInspectCheckpointFile,
  inspectLoadingPath,
  onRefresh,
}: {
  job: SyncJobInstance | null;
  checkpointSnapshot: SyncCheckpointSnapshot | null;
  loading?: boolean;
  checkpointFiles: RuntimeStorageListItem[];
  checkpointFilesLoading?: boolean;
  onInspectCheckpointFile: (path: string) => void;
  inspectLoadingPath?: string | null;
  onRefresh: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [selectedPipelineId, setSelectedPipelineId] = useState<number | null>(null);

  const checkpointFilesByID = useMemo(() => {
    const mapping = new Map<string, RuntimeStorageListItem>();
    checkpointFiles.forEach((item: RuntimeStorageListItem) => {
      const identity = extractCheckpointFileIdentity(item.name || item.path);
      if (!identity) {
        return;
      }
      const key = `${identity.pipelineId}:${identity.checkpointId}`;
      if (!mapping.has(key)) {
        mapping.set(key, item);
      }
    });
    return mapping;
  }, [checkpointFiles]);

  const pipelines = useMemo(() => checkpointSnapshot?.overview?.pipelines || [], [checkpointSnapshot?.overview?.pipelines]);
  const history = useMemo(() => checkpointSnapshot?.history || [], [checkpointSnapshot?.history]);

  // 提取最近 24 次历史快照用于 Sparkline 脉冲波形
  const recentHistory = useMemo(() => {
    const sorted = [...history].reverse().slice(-24);
    const maxDur = Math.max(...sorted.map((h) => h.checkpoint?.durationMillis ?? 0), 10);
    const validDurs = sorted.filter((h) => typeof h.checkpoint?.durationMillis === 'number');
    const avgDur = validDurs.length > 0
      ? Math.round(validDurs.reduce((acc, h) => acc + (h.checkpoint?.durationMillis || 0), 0) / validDurs.length)
      : 0;
    return { list: sorted, maxDur, avgDur };
  }, [history]);

  if (!job) {
    return (
      <div className='flex h-full items-center justify-center text-sm text-muted-foreground'>
        {t('noCheckpointJob')}
      </div>
    );
  }

  const activePipeline =
    selectedPipelineId !== null
      ? pipelines.find((p) => p.pipelineId === selectedPipelineId) || pipelines[0]
      : pipelines[0];

  return (
    <div className='flex h-full min-h-0 flex-col gap-2.5 overflow-hidden'>
      {loading ? (
        <div className='flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground'>
          <Loader2 className='mr-2 size-4 animate-spin' />
          {t('loadingCheckpoint')}
        </div>
      ) : checkpointSnapshot?.empty_reason || (pipelines.length === 0 && history.length === 0) ? (
        <div className='flex min-h-0 flex-1 flex-col items-center justify-center p-6 text-center'>
          <div className='max-w-md space-y-3 rounded-xl border border-dashed border-border/60 bg-muted/20 p-5 text-left'>
            <div className='flex items-center justify-between gap-2'>
              <span className='font-medium text-foreground text-sm flex items-center gap-1.5'>
                <Clock className='size-4 text-muted-foreground' />
                {checkpointSnapshot?.message || t('checkpointEmpty')}
              </span>
              <Button
                size='icon'
                variant='ghost'
                className='size-7'
                disabled={loading || checkpointFilesLoading}
                onClick={onRefresh}
              >
                <RefreshCw
                  className={cn(
                    'size-3.5',
                    (loading || checkpointFilesLoading) && 'animate-spin',
                  )}
                />
              </Button>
            </div>
            <p className='text-xs text-muted-foreground leading-relaxed'>
              {t('checkpointEmptyGuide')}
            </p>
          </div>
        </div>
      ) : (
        <div className='flex min-h-0 flex-1 flex-col gap-2 overflow-hidden'>
          {/* 顶部紧凑 Pipeline 指标胶囊横幅 */}
          <div className='flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border/50 bg-background/80 px-3 py-1.5 shrink-0 shadow-xs'>
            <div className='flex flex-wrap items-center gap-2'>
              {pipelines.length > 1 ? (
                <div className='flex items-center gap-1 pr-1 border-r border-border/40'>
                  {pipelines.map((p) => (
                    <Button
                      key={p.pipelineId}
                      size='sm'
                      variant={activePipeline?.pipelineId === p.pipelineId ? 'secondary' : 'ghost'}
                      className='h-6 px-2 text-xs font-mono font-medium'
                      onClick={() => setSelectedPipelineId(p.pipelineId)}
                    >
                      Pipeline #{p.pipelineId}
                    </Button>
                  ))}
                </div>
              ) : (
                <div className='flex items-center gap-1 font-mono text-xs font-semibold text-foreground/90 pr-1 border-r border-border/40'>
                  Pipeline #{activePipeline?.pipelineId ?? 1}
                </div>
              )}

              {/* 统计指标 Pills */}
              <div className='flex items-center gap-2 text-xs font-mono'>
                <span className='text-muted-foreground'>
                  {t('triggered')}: <strong className='font-medium text-foreground'>{activePipeline?.counts?.triggered ?? '-'}</strong>
                </span>
                <span className='text-muted-foreground/40'>·</span>
                <span className='flex items-center gap-1 text-emerald-600 dark:text-emerald-400'>
                  <span className='size-1.5 rounded-full bg-emerald-500 animate-pulse' />
                  {t('completed')}: <strong className='font-semibold'>{activePipeline?.counts?.completed ?? 0}</strong>
                </span>
                <span className='text-muted-foreground/40'>·</span>
                <span className={cn(
                  'flex items-center gap-1',
                  (activePipeline?.counts?.failed ?? 0) > 0 ? 'text-destructive font-semibold' : 'text-muted-foreground',
                )}>
                  {t('failed')}: <strong className='font-medium'>{activePipeline?.counts?.failed ?? 0}</strong>
                </span>
                <span className='text-muted-foreground/40'>·</span>
                <span className={cn(
                  'flex items-center gap-1',
                  (activePipeline?.counts?.inProgress ?? 0) > 0 ? 'text-blue-500 font-semibold' : 'text-muted-foreground',
                )}>
                  {t('inProgress')}: <strong className='font-medium'>{activePipeline?.counts?.inProgress ?? 0}</strong>
                </span>
                {activePipeline?.latestCompleted?.checkpointId ? (
                  <>
                    <span className='text-muted-foreground/40'>·</span>
                    <span className='text-muted-foreground flex items-center gap-1'>
                      {t('latestCompleted')}:
                      <Badge variant='outline' className='rounded-sm text-[10px] px-1.5 py-0 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-mono font-medium'>
                        #{activePipeline.latestCompleted.checkpointId}
                      </Badge>
                    </span>
                  </>
                ) : null}
              </div>
            </div>

            <Button
              size='icon'
              variant='ghost'
              className='size-7 shrink-0'
              disabled={loading || checkpointFilesLoading}
              onClick={onRefresh}
            >
              <RefreshCw
                className={cn(
                  'size-3.5',
                  (loading || checkpointFilesLoading) && 'animate-spin',
                )}
              />
            </Button>
          </div>

          {/* Sparkline 脉冲波形条 (当有历史记录时展示) */}
          {recentHistory.list.length > 0 ? (
            <div className='flex items-center justify-between gap-3 rounded-lg border border-border/40 bg-muted/15 px-3 py-1.5 shrink-0'>
              <div className='flex items-center gap-2'>
                <span className='text-[11px] font-medium text-muted-foreground'>
                  {t('checkpointPulseTitle')}
                </span>
                <div className='flex items-end gap-1 h-5 pl-1'>
                  {recentHistory.list.map((item, idx) => {
                    const dur = item.checkpoint?.durationMillis ?? 0;
                    const height = Math.max(Math.round((dur / recentHistory.maxDur) * 18), 4);
                    const isSuccess = item.checkpoint?.status === 'COMPLETED';
                    const isFailed = item.checkpoint?.status === 'FAILED';
                    const colorClass = isSuccess
                      ? 'bg-emerald-500/80 hover:bg-emerald-400'
                      : isFailed
                        ? 'bg-rose-500 hover:bg-rose-400'
                        : 'bg-blue-500 hover:bg-blue-400';
                    return (
                      <Tooltip key={idx}>
                        <TooltipTrigger asChild>
                          <div
                            style={{ height: `${height}px` }}
                            className={cn('w-1.5 rounded-t-xs transition-all cursor-pointer', colorClass)}
                          />
                        </TooltipTrigger>
                        <TooltipContent side='top' className='text-xs font-mono'>
                          #{item.checkpoint?.checkpointId || '-'} · {dur}ms · {item.checkpoint?.status || 'UNKNOWN'}
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              </div>
              <div className='flex items-center gap-2.5 text-[11px] font-mono text-muted-foreground'>
                <span>avg: <strong className='font-medium text-foreground'>{recentHistory.avgDur}ms</strong></span>
                <span className='text-muted-foreground/40'>·</span>
                <span>max: <strong className='font-medium text-foreground'>{recentHistory.maxDur}ms</strong></span>
              </div>
            </div>
          ) : null}

          {/* 全宽 Checkpoint 历史列表 */}
          <div className='flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70 shadow-xs'>
            <div className='border-b border-border/50 px-3 py-1.5 text-xs font-semibold text-foreground/80 flex items-center justify-between'>
              <span>{t('checkpointHistory')}</span>
              <span className='font-mono text-[11px] text-muted-foreground font-normal'>
                {history.length} {t('records') || '条快照'}
              </span>
            </div>
            <div className='min-h-0 flex-1 overflow-auto'>
              <Table>
                <TableHeader className='sticky top-0 z-10 bg-background/95 backdrop-blur-xs'>
                  <TableRow className='hover:bg-transparent border-border/50'>
                    <TableHead className='h-7.5 py-1 px-2.5 text-xs font-medium'>{t('pipeline')}</TableHead>
                    <TableHead className='h-7.5 py-1 px-2.5 text-xs font-medium'>{t('checkpointId')}</TableHead>
                    <TableHead className='h-7.5 py-1 px-2.5 text-xs font-medium'>{t('checkpointStatus')}</TableHead>
                    <TableHead className='h-7.5 py-1 px-2.5 text-xs font-medium'>{t('durationMillis')}</TableHead>
                    <TableHead className='h-7.5 py-1 px-2.5 text-xs font-medium'>{t('stateSize')}</TableHead>
                    <TableHead className='h-7.5 py-1 px-2.5 text-right text-xs font-medium'>
                      {t('actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {history.length > 0 ? (
                    history.map((item, index) => {
                      const checkpointId = item.checkpoint?.checkpointId;
                      const checkpointKey =
                        checkpointId !== undefined
                          ? `${item.pipelineId}:${checkpointId}`
                          : '';
                      const matchedFile =
                        checkpointKey !== ''
                          ? checkpointFilesByID.get(checkpointKey)
                          : undefined;
                      const durationStr =
                        item.checkpoint?.durationMillis !== undefined
                          ? `${item.checkpoint.durationMillis}ms`
                          : '-';
                      const sizeStr =
                        typeof item.checkpoint?.stateSize === 'number'
                          ? formatSizeBytes(item.checkpoint.stateSize)
                          : item.checkpoint?.stateSize !== undefined
                            ? String(item.checkpoint.stateSize)
                            : '-';

                      return (
                        <TableRow
                          key={`${item.pipelineId}-${item.checkpoint?.checkpointId || index}`}
                          className='h-8.5 border-border/40 hover:bg-muted/40 transition-colors'
                        >
                          <TableCell className='py-1 px-2.5 font-mono text-xs text-muted-foreground'>
                            #{item.pipelineId}
                          </TableCell>
                          <TableCell className='py-1 px-2.5 font-mono text-xs font-semibold text-foreground'>
                            {checkpointId ? `#${checkpointId}` : '-'}
                          </TableCell>
                          <TableCell className='py-1 px-2.5'>
                            {item.checkpoint?.status ? (
                              renderCheckpointFieldValue('status', item.checkpoint.status)
                            ) : (
                              '-'
                            )}
                          </TableCell>
                          <TableCell className='py-1 px-2.5 font-mono text-xs text-muted-foreground'>
                            {durationStr}
                          </TableCell>
                          <TableCell className='py-1 px-2.5 font-mono text-xs text-foreground/90 font-medium'>
                            {sizeStr}
                          </TableCell>
                          <TableCell className='py-1 px-2.5 text-right'>
                            <Button
                              size='sm'
                              variant='ghost'
                              className='h-6 px-2 text-xs hover:bg-muted font-normal'
                              disabled={
                                !matchedFile?.path ||
                                inspectLoadingPath === matchedFile.path
                              }
                              onClick={() =>
                                matchedFile?.path &&
                                onInspectCheckpointFile(matchedFile.path)
                              }
                            >
                              {inspectLoadingPath === matchedFile?.path ? (
                                <Loader2 className='mr-1 size-3 animate-spin' />
                              ) : (
                                <Eye className='mr-1 size-3 text-muted-foreground' />
                              )}
                              {t('viewDetails')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  ) : (
                    <TableRow>
                      <TableCell
                        colSpan={6}
                        className='text-center text-muted-foreground py-8 text-xs'
                      >
                        {t('checkpointHistoryEmpty')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export function CheckpointDetailsSummary({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <summary
      className={cn(
        'flex cursor-pointer list-none items-center gap-2 px-3 py-2 marker:hidden',
        className,
      )}
    >
      <span className='inline-flex size-4 shrink-0 items-center justify-center rounded-sm border border-border/60 text-muted-foreground'>
        <Plus className='size-3 group-open:hidden' />
        <Minus className='hidden size-3 group-open:block' />
      </span>
      <div className='min-w-0 flex-1'>{children}</div>
    </summary>
  );
}

function CheckpointInspectSectionShell({
  title,
  children,
  collapsible = false,
  defaultOpen = true,
}: {
  title: string;
  children: ReactNode;
  collapsible?: boolean;
  defaultOpen?: boolean;
}) {
  if (collapsible) {
    return (
      <details
        open={defaultOpen}
        className='group rounded-lg border border-border/50 bg-background/80'
      >
        <CheckpointDetailsSummary className='border-b border-border/50 text-sm font-medium'>
          <span>{title}</span>
        </CheckpointDetailsSummary>
        <div className='p-3'>{children}</div>
      </details>
    );
  }
  return (
    <div className='rounded-lg border border-border/50 bg-background/80'>
      <div className='border-b border-border/50 px-3 py-2 text-sm font-medium'>
        {title}
      </div>
      <div className='p-3'>{children}</div>
    </div>
  );
}

function CheckpointInspectObjectSection({
  title,
  value,
  defaultOpen = false,
}: {
  title: string;
  value?: Record<string, unknown> | null;
  defaultOpen?: boolean;
}) {
  const entries = value ? Object.entries(value) : [];
  return (
    <CheckpointInspectSectionShell
      title={title}
      collapsible
      defaultOpen={defaultOpen}
    >
      {entries.length > 0 ? (
        <Table>
          <TableBody>
            {entries.map(([key, entryValue]) => (
              <TableRow key={key}>
                <TableCell className='w-[220px] font-medium'>{key}</TableCell>
                <TableCell>
                  {renderCheckpointFieldValue(key, entryValue)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </CheckpointInspectSectionShell>
  );
}

function QuickCopyButton({text}: {text: string}) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    if (!text) {
      return;
    }
    void navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <Button
      variant='ghost'
      size='icon'
      className='size-6 text-muted-foreground hover:text-foreground'
      onClick={handleCopy}
      title='复制位点'
    >
      {copied ? (
        <Check className='size-3 text-emerald-500' />
      ) : (
        <Copy className='size-3' />
      )}
    </Button>
  );
}

function formatFriendlyInspectError(raw?: string): {
  summary: string;
  detail?: string;
} {
  if (!raw) {
    return {summary: ''};
  }
  const trimmed = raw.trim();
  if (
    trimmed.includes('ClassNotFoundException') ||
    trimmed.includes('连接器未安装') ||
    trimmed.includes('缺少连接器类') ||
    trimmed.includes('Plugin not found') ||
    trimmed.includes('NoClassDefFoundError')
  ) {
    return {
      summary:
        '未检测到对应连接器插件 JAR。请将连接器 JAR 放置在集群的 connectors/ 目录下，或通过 STX 集群插件管理安装该连接器。',
      detail: trimmed,
    };
  }
  if (
    trimmed.includes('jobConfig parse failed') ||
    trimmed.includes('ConfigException')
  ) {
    const match = trimmed.match(/jobConfig parse failed:\s*([^"]+)/);
    const reason = match ? match[1].trim() : '任务脚本存在未闭合引号或保留字符';
    return {
      summary: `数据源位点深度解析已跳过：任务脚本配置解析受阻（${reason.slice(0, 120)}）。快照基础元数据与算子状态已成功加载。`,
      detail: trimmed,
    };
  }
  if (trimmed.includes('status 400') || trimmed.includes('stx-java-proxy')) {
    return {
      summary:
        '数据源位点解析服务返回状态提示，已跳过位点深度解析。底层快照元数据与算子状态已正常加载。',
      detail: trimmed,
    };
  }
  return {
    summary: trimmed,
  };
}

export function CheckpointInspectSourceHighlightsSection({
  title,
  result,
  decodeStrategyLabel,
  splitCountLabel,
  warningsLabel,
  unsupportedLabel,
  currentOffsetLabel,
  targetLabel,
  progressLabel,
}: {
  title: string;
  result: RuntimeStorageCheckpointInspectResult | null;
  decodeStrategyLabel: string;
  splitCountLabel: string;
  warningsLabel: string;
  unsupportedLabel: string;
  currentOffsetLabel: string;
  targetLabel: string;
  progressLabel: string;
}) {
  const sourceStates = Array.isArray(result?.source_state_inspect?.sources)
    ? result.source_state_inspect.sources
    : [];
  const sinks = Array.isArray(result?.source_state_inspect?.sinks)
    ? result.source_state_inspect.sinks
    : [];
  const unsupportedSources = Array.isArray(
    result?.source_state_inspect?.unsupported_sources,
  )
    ? result.source_state_inspect.unsupported_sources
    : [];
  const warnings = Array.isArray(result?.source_state_inspect?.warnings)
    ? result.source_state_inspect.warnings
    : [];
  const errorMessage = result?.source_state_inspect?.error_message?.trim();

  if (
    !errorMessage &&
    sourceStates.length === 0 &&
    sinks.length === 0 &&
    unsupportedSources.length === 0
  ) {
    return null;
  }

  return (
    <CheckpointInspectSectionShell title={title}>
      <div className='space-y-4'>
        {errorMessage ? (() => {
          const friendly = formatFriendlyInspectError(errorMessage);
          return (
            <div className='rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300'>
              <div className='flex items-start gap-2.5'>
                <AlertTriangle className='mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400' />
                <div className='min-w-0 flex-1 space-y-1.5'>
                  <div className='text-xs font-medium leading-relaxed'>
                    {friendly.summary}
                  </div>
                  {friendly.detail ? (
                    <details className='text-[11px] text-muted-foreground'>
                      <summary className='cursor-pointer text-amber-700/80 hover:underline dark:text-amber-300/80'>
                        查看技术详情 / Technical Details
                      </summary>
                      <pre className='mt-1.5 max-h-28 overflow-auto rounded bg-background/60 p-2 font-mono text-[10px] whitespace-pre-wrap break-all text-foreground'>
                        {friendly.detail}
                      </pre>
                    </details>
                  ) : null}
                </div>
              </div>
            </div>
          );
        })() : null}

        {warnings.length > 0 ? (
          <div className='rounded-lg border border-border/60 bg-muted/20 p-3'>
            <div className='mb-2 text-xs font-medium text-muted-foreground'>
              {warningsLabel}
            </div>
            <ul className='list-disc space-y-1 pl-5 text-sm'>
              {warnings.map((warning, index) => (
                <li key={`${warning}-${index}`}>{warning}</li>
              ))}
            </ul>
          </div>
        ) : null}

        {/* 1. Source 业务位点卡片体系 */}
        {sourceStates.length > 0 ? (
          <div className='space-y-4'>
            <div className='flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground'>
              <Activity className='size-3.5 text-primary' />
              <span>数据源位点 (Sources / Offsets) ({sourceStates.length})</span>
            </div>
            {sourceStates.map((item, index) => {
              const norm = item.normalizedProgress;
              if (norm) {
                const categoryStyle = (() => {
                  switch (norm.category) {
                    case 'LOG_STREAM':
                      return {
                        label: '增量日志流 (CDC / Log Stream)',
                        icon: Terminal,
                        border: 'border-emerald-500/30 dark:border-emerald-500/20',
                        bg: 'bg-emerald-500/5',
                        badge: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
                      };
                    case 'PARTITION_QUEUE':
                      return {
                        label: '分区队列 (Partition Queue)',
                        icon: Layers,
                        border: 'border-blue-500/30 dark:border-blue-500/20',
                        bg: 'bg-blue-500/5',
                        badge: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300',
                      };
                    case 'LAKE_SPLIT':
                      return {
                        label: '湖仓分片 (Lakehouse Split)',
                        icon: Database,
                        border: 'border-purple-500/30 dark:border-purple-500/20',
                        bg: 'bg-purple-500/5',
                        badge: 'border-purple-500/30 bg-purple-500/10 text-purple-700 dark:text-purple-300',
                      };
                    default:
                      return {
                        label: '通用数据源 (Generic Source)',
                        icon: Activity,
                        border: 'border-border/60',
                        bg: 'bg-background/80',
                        badge: 'border-border/60 bg-muted/20 text-foreground',
                      };
                  }
                })();
                const CategoryIcon = categoryStyle.icon;

                return (
                  <div
                    key={`${item.actionName || item.pluginName || index}`}
                    className={cn(
                      'space-y-3.5 rounded-lg border p-4 shadow-xs',
                      categoryStyle.border,
                      categoryStyle.bg,
                    )}
                  >
                    {/* 卡片头部 */}
                    <div className='flex flex-wrap items-center justify-between gap-2 border-b border-border/50 pb-3'>
                      <div className='flex items-center gap-2.5'>
                        <div className='flex size-8 items-center justify-center rounded-md bg-primary/10 text-primary'>
                          <CategoryIcon className='size-4' />
                        </div>
                        <div>
                          <div className='flex items-center gap-2'>
                            <span className='font-semibold text-sm text-foreground'>
                              {item.actionName || `Source[${index}]`}
                            </span>
                            <Badge
                              variant='outline'
                              className={cn('font-mono text-[11px]', categoryStyle.badge)}
                            >
                              {item.pluginName || 'Source'}
                            </Badge>
                          </div>
                          <div className='text-[11px] text-muted-foreground'>
                            {categoryStyle.label}
                          </div>
                        </div>
                      </div>

                      {/* 阶段徽章 */}
                      {norm.phaseBadge === 'INCREMENTAL' ? (
                        <Badge
                          variant='outline'
                          className='inline-flex items-center gap-1.5 border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-700 dark:text-emerald-300'
                        >
                          <span className='size-2 rounded-full bg-emerald-500 animate-pulse' />
                          <span>增量实时 (Incremental)</span>
                        </Badge>
                      ) : norm.phaseBadge === 'SNAPSHOT' ? (
                        <Badge
                          variant='outline'
                          className='inline-flex items-center gap-1.5 border-blue-500/40 bg-blue-500/10 px-2 py-0.5 text-xs text-blue-700 dark:text-blue-300'
                        >
                          <span className='size-2 rounded-full bg-blue-500 animate-ping' />
                          <span>全量快照中 (Snapshot)</span>
                        </Badge>
                      ) : (
                        <Badge variant='outline' className='text-xs'>
                          {norm.phaseBadge || 'RUNNING'}
                        </Badge>
                      )}
                    </div>

                    {/* 核心位点与指标高亮区 */}
                    <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                      {/* Hero: Primary Value */}
                      <div className='rounded-lg border border-border/60 bg-background/90 p-3 shadow-xs'>
                        <div className='flex items-center justify-between text-xs text-muted-foreground'>
                          <span className='font-medium'>{norm.primaryLabel}</span>
                          <QuickCopyButton text={norm.primaryValue} />
                        </div>
                        <div className='mt-1.5 break-all font-mono text-base font-bold text-foreground'>
                          {norm.primaryValue}
                        </div>
                      </div>

                      {/* Secondary Metric */}
                      <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
                        <div className='text-xs text-muted-foreground'>
                          {norm.secondaryLabel || '运行阶段'}
                        </div>
                        <div className='mt-1.5 text-sm font-semibold text-foreground'>
                          {norm.secondaryValue || '-'}
                        </div>
                      </div>

                      {/* Lag / Event Timestamp */}
                      <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
                        <div className='flex items-center justify-between text-xs text-muted-foreground'>
                          <span>业务延迟 / 事件时间</span>
                          {norm.lagSeconds !== undefined && norm.lagSeconds <= 5 && (
                            <span className='inline-flex items-center gap-1 text-[11px] font-medium text-emerald-600 dark:text-emerald-400'>
                              <span className='size-1.5 rounded-full bg-emerald-500 animate-pulse' />
                              实时
                            </span>
                          )}
                        </div>
                        <div className='mt-1.5 text-sm font-semibold text-foreground'>
                          {norm.lagSeconds !== undefined
                            ? norm.lagSeconds <= 0
                              ? '0 秒 (实时无延迟)'
                              : norm.lagSeconds < 60
                                ? `${Math.round(norm.lagSeconds)} 秒延迟`
                                : `${Math.floor(norm.lagSeconds / 60)} 分 ${Math.round(norm.lagSeconds % 60)} 秒延迟`
                            : '-'}
                        </div>
                        {norm.eventTime !== undefined && norm.eventTime > 0 && (
                          <div className='mt-1 text-[11px] font-mono text-muted-foreground truncate' title={new Date(norm.eventTime).toLocaleString()}>
                            {new Date(norm.eventTime).toLocaleString()}
                          </div>
                        )}
                      </div>
                    </div>

                    {/* 目标表 / 主题列表 */}
                    {Array.isArray(norm.targetTables) && norm.targetTables.length > 0 && (
                      <div className='flex flex-wrap items-center gap-1.5 text-xs pt-0.5'>
                        <span className='text-muted-foreground font-medium'>同步目标:</span>
                        {norm.targetTables.map((tbl) => (
                          <Badge
                            key={tbl}
                            variant='secondary'
                            className='font-mono text-[11px] bg-background/90 border border-border/50'
                          >
                            {tbl}
                          </Badge>
                        ))}
                      </div>
                    )}

                    {/* Subtask 进度明细列表 */}
                    {Array.isArray(norm.subtaskProgress) && norm.subtaskProgress.length > 0 && (() => {
                      const subtaskItems = norm.subtaskProgress;
                      const hasLag = subtaskItems.some((s) => s.lag !== undefined);
                      return (
                        <div className='overflow-hidden rounded-md border border-border/50 bg-background/70 shadow-xs'>
                          <div className='border-b border-border/50 bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground flex items-center justify-between'>
                            <span>子任务位点明细 (Subtask Progress)</span>
                            <span className='font-mono text-[11px]'>{subtaskItems.length} 个子任务</span>
                          </div>
                          <Table>
                            <TableHeader className='bg-muted/10'>
                              <TableRow>
                                <TableHead className='h-7 py-1 text-xs'>Subtask</TableHead>
                                <TableHead className='h-7 py-1 text-xs'>目标 / 分区</TableHead>
                                <TableHead className='h-7 py-1 text-xs'>当前位点 (Current Offset)</TableHead>
                                {hasLag && (
                                  <TableHead className='h-7 py-1 text-xs'>延迟 (Lag)</TableHead>
                                )}
                                <TableHead className='h-7 py-1 text-xs'>状态</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {subtaskItems.map((sub) => (
                                <TableRow key={sub.subtaskIndex} className='hover:bg-muted/20'>
                                  <TableCell className='py-1.5 font-mono text-xs font-medium'>
                                    Subtask #{sub.subtaskIndex}
                                  </TableCell>
                                  <TableCell className='py-1.5 font-mono text-xs text-muted-foreground'>
                                    {sub.target || '-'}
                                  </TableCell>
                                  <TableCell className='py-1.5 font-mono text-xs font-semibold text-foreground'>
                                    {sub.currentOffset || '-'}
                                  </TableCell>
                                  {hasLag && (
                                    <TableCell className='py-1.5 font-mono text-xs'>
                                      {sub.lag !== undefined ? `${sub.lag}` : '-'}
                                    </TableCell>
                                  )}
                                  <TableCell className='py-1.5 text-xs'>
                                    <Badge
                                      variant='outline'
                                      className='border-emerald-500/40 bg-emerald-500/10 px-1.5 py-0 text-[10px] text-emerald-700 dark:text-emerald-300'
                                    >
                                      {sub.status || 'NORMAL'}
                                    </Badge>
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        </div>
                      );
                    })()}
                  </div>
                );
              }

              // 非归一化 Source 兜底表格行
              const summary = summarizeCheckpointSourceState(item);
              return (
                <div
                  key={`${item.actionName || item.pluginName || index}`}
                  className='rounded-md border border-border/50 bg-background/70 p-3'
                >
                  <div className='flex items-center justify-between'>
                    <span className='font-semibold text-sm'>{String(item.actionName || item.pluginName || '-')}</span>
                    <Badge variant='outline'>{formatCellValue(item.pluginName)}</Badge>
                  </div>
                  <div className='mt-2 grid grid-cols-2 gap-2 text-xs sm:grid-cols-5'>
                    <div>
                      <div className='text-muted-foreground'>{currentOffsetLabel}</div>
                      <div className='mt-0.5 font-mono font-medium'>{summary.offset}</div>
                    </div>
                    <div>
                      <div className='text-muted-foreground'>{targetLabel}</div>
                      <div className='mt-0.5 font-mono font-medium'>{summary.target}</div>
                    </div>
                    <div>
                      <div className='text-muted-foreground'>{splitCountLabel}</div>
                      <div className='mt-0.5 font-mono font-medium'>{summary.splitCount}</div>
                    </div>
                    <div>
                      <div className='text-muted-foreground'>{progressLabel}</div>
                      <div className='mt-0.5 font-mono font-medium'>{summary.progress}</div>
                    </div>
                    <div>
                      <div className='text-muted-foreground'>{decodeStrategyLabel}</div>
                      <div className='mt-0.5 font-mono font-medium'>{formatCellValue(item.decodeStrategy)}</div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : null}

        {/* 2. Sink 下游写入与 2PC 事务看板 */}
        {sinks.length > 0 ? (
          <div className='space-y-4 pt-2'>
            <div className='flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground'>
              <CheckCheck className='size-3.5 text-emerald-500' />
              <span>下游提交状态 / 2PC 事务保证 (Sinks) ({sinks.length})</span>
            </div>
            {sinks.map((sink, sIdx) => {
              const sinkNorm = sink.normalizedProgress;
              return (
                <div
                  key={sink.actionName || sIdx}
                  className='space-y-3.5 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-4 shadow-xs'
                >
                  <div className='flex flex-wrap items-center justify-between gap-2 border-b border-border/50 pb-3'>
                    <div className='flex items-center gap-2.5'>
                      <div className='flex size-8 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                        <CheckCheck className='size-4' />
                      </div>
                      <div>
                        <div className='flex items-center gap-2'>
                          <span className='text-sm font-semibold text-foreground'>
                            {sink.actionName || `Sink[${sIdx}]`}
                          </span>
                          <Badge variant='outline' className='font-mono text-[11px] border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'>
                            {sink.pluginName || 'Sink'}
                          </Badge>
                        </div>
                        <div className='text-[11px] text-muted-foreground'>
                          两阶段提交事务机制 (Two-Phase Commit / 2PC)
                        </div>
                      </div>
                    </div>
                    <Badge
                      variant='outline'
                      className='gap-1.5 border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-700 dark:text-emerald-300'
                    >
                      <span className='size-1.5 rounded-full bg-emerald-500' />
                      <span>已预提交 (Prepared)</span>
                    </Badge>
                  </div>

                  <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                    <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
                      <div className='text-xs text-muted-foreground'>提交机制</div>
                      <div className='mt-1 text-sm font-semibold text-foreground'>
                        {sinkNorm?.primaryValue || '两阶段提交 (2PC / Checkpoint)'}
                      </div>
                    </div>
                    <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
                      <div className='text-xs text-muted-foreground'>写入阶段</div>
                      <div className='mt-1 text-sm font-semibold text-foreground'>
                        {sinkNorm?.secondaryValue || '已完成预提交 (Prepared)'}
                      </div>
                    </div>
                    <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
                      <div className='text-xs text-muted-foreground'>待提交数据分块</div>
                      <div className='mt-1 text-sm font-semibold text-foreground'>
                        {sink.chunksTotal ?? 1} 个分块 ({formatSizeBytes(sink.stateBytesTotal ?? 0)})
                      </div>
                    </div>
                  </div>

                  {Array.isArray(sinkNorm?.subtaskProgress) && sinkNorm.subtaskProgress.length > 0 && (
                    <div className='overflow-hidden rounded-md border border-border/50 bg-background/70 shadow-xs'>
                      <Table>
                        <TableHeader className='bg-muted/10'>
                          <TableRow>
                            <TableHead className='h-7 py-1 text-xs'>Subtask</TableHead>
                            <TableHead className='h-7 py-1 text-xs'>写入目标</TableHead>
                            <TableHead className='h-7 py-1 text-xs'>Prepared 分块</TableHead>
                            <TableHead className='h-7 py-1 text-xs'>状态大小</TableHead>
                            <TableHead className='h-7 py-1 text-xs'>事务状态</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {sinkNorm.subtaskProgress.map((sub) => (
                            <TableRow key={sub.subtaskIndex} className='hover:bg-muted/20'>
                              <TableCell className='py-1.5 font-mono text-xs font-medium'>
                                Subtask #{sub.subtaskIndex}
                              </TableCell>
                              <TableCell className='py-1.5 font-mono text-xs text-muted-foreground'>
                                {sub.target || `${sink.pluginName}-${sub.subtaskIndex}`}
                              </TableCell>
                              <TableCell className='py-1.5 font-mono text-xs font-semibold text-foreground'>
                                {sub.chunks ?? 1}
                              </TableCell>
                              <TableCell className='py-1.5 font-mono text-xs'>
                                {formatSizeBytes(sub.bytes ?? 0)}
                              </TableCell>
                              <TableCell className='py-1.5 text-xs'>
                                <Badge
                                  variant='outline'
                                  className='border-emerald-500/40 bg-emerald-500/10 px-1.5 py-0 text-[10px] text-emerald-700 dark:text-emerald-300'
                                >
                                  {sub.status || 'PREPARED'}
                                </Badge>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        ) : null}

        {/* 3. 不支持或缺失的 Source 诊断提示 */}
        {unsupportedSources.length > 0 ? (
          <details className='group rounded-lg border border-border/60 bg-background/60'>
            <CheckpointDetailsSummary className='py-3 text-sm font-medium'>
              <span>{unsupportedLabel} ({unsupportedSources.length})</span>
            </CheckpointDetailsSummary>
            <div className='border-t border-border/50 p-3'>
              <div className='space-y-3'>
                {unsupportedSources.map((item, index) => (
                  <CheckpointInspectMiniObject
                    key={`${item.actionName || index}`}
                    title={String(item.actionName || unsupportedLabel)}
                    value={item}
                  />
                ))}
              </div>
            </div>
          </details>
        ) : null}
      </div>
    </CheckpointInspectSectionShell>
  );
}

export function CheckpointInspectOverviewSection({
  result,
  t,
}: {
  result: RuntimeStorageCheckpointInspectResult | null;
  t: (key: string) => string;
}) {
  const summary = buildCheckpointInspectSummary(result);
  const completed = result?.completed_checkpoint;
  const checkpointId =
    completed?.checkpointId !== undefined && completed?.checkpointId !== null
      ? String(completed.checkpointId)
      : '-';
  const status =
    typeof completed?.status === 'string' ? completed.status : 'UNKNOWN';
  const duration =
    completed?.durationMillis !== undefined ? `${completed.durationMillis} ms` : '-';
  const stateSizeFormatted =
    typeof completed?.stateSize === 'number'
      ? formatSizeBytes(completed.stateSize)
      : completed?.stateSize !== undefined && completed?.stateSize !== null
        ? String(completed.stateSize)
        : '-';
  const checkpointType =
    typeof completed?.checkpointType === 'string'
      ? completed.checkpointType
      : 'CHECKPOINT';

  return (
    <CheckpointInspectSectionShell title={t('checkpointOverview')}>
      <div className='space-y-3.5'>
        {/* 四联现代度量指标卡片 */}
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
          {/* 1. 检查点状态与 ID */}
          <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
            <div className='flex items-center justify-between text-xs text-muted-foreground'>
              <span>检查点 ID</span>
              <span className='text-[10px] font-mono'>#{checkpointId}</span>
            </div>
            <div className='mt-2 flex items-center gap-2'>
              {renderCheckpointFieldValue('status', status)}
            </div>
          </div>

          {/* 2. 端到端耗时 */}
          <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
            <div className='flex items-center justify-between text-xs text-muted-foreground'>
              <div className='flex items-center gap-1'>
                <Clock className='size-3 text-primary' />
                <span>端到端耗时</span>
              </div>
            </div>
            <div className='mt-2 font-mono text-base font-bold text-foreground'>
              {duration}
            </div>
          </div>

          {/* 3. 状态快照体积 */}
          <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
            <div className='flex items-center justify-between text-xs text-muted-foreground'>
              <div className='flex items-center gap-1'>
                <HardDrive className='size-3 text-emerald-500' />
                <span>快照体积</span>
              </div>
            </div>
            <div className='mt-2 font-mono text-base font-bold text-foreground'>
              {stateSizeFormatted}
            </div>
          </div>

          {/* 4. 触发模式 */}
          <div className='rounded-lg border border-border/60 bg-background/80 p-3 shadow-xs'>
            <div className='flex items-center justify-between text-xs text-muted-foreground'>
              <div className='flex items-center gap-1'>
                <Zap className='size-3 text-amber-500' />
                <span>触发类型</span>
              </div>
            </div>
            <div className='mt-2'>
              {renderCheckpointFieldValue('checkpointType', checkpointType)}
            </div>
          </div>
        </div>

        {/* 详细元数据汇总表格 */}
        <div className='rounded-md border border-border/50 bg-background/70 overflow-hidden shadow-xs'>
          <div className='border-b border-border/50 bg-muted/20 px-3 py-2 flex items-center justify-between text-xs'>
            <span className='text-muted-foreground'>{t('fileName')}</span>
            <span className='break-all font-mono font-medium text-foreground max-w-[75%] truncate' title={result?.file_name}>
              {result?.file_name || '-'}
            </span>
          </div>
          <div className='overflow-x-auto'>
            <Table>
              <TableHeader className='bg-muted/10'>
                <TableRow>
                  {summary.map((item) => (
                    <TableHead key={item.label} className='h-8 py-1 text-xs font-semibold'>
                      {item.label}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow className='hover:bg-transparent'>
                  {summary.map((item) => (
                    <TableCell key={item.label} className='py-2 font-medium text-xs'>
                      {renderCheckpointFieldValue(item.key, item.value)}
                    </TableCell>
                  ))}
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>
      </div>
    </CheckpointInspectSectionShell>
  );
}

export function CheckpointInspectRawDetailsSection({
  result,
  t,
}: {
  result: RuntimeStorageCheckpointInspectResult | null;
  t: (key: string) => string;
}) {
  return (
    <details className='group rounded-lg border border-border/60 bg-background/60'>
      <CheckpointDetailsSummary className='px-4 py-3 text-sm font-medium'>
        <span>{t('rawDetails')}</span>
      </CheckpointDetailsSummary>
      <div className='space-y-4 border-t border-border/50 p-4'>
        <CheckpointInspectObjectSection
          title={t('completedCheckpoint')}
          value={result?.completed_checkpoint}
          defaultOpen={false}
        />
        <CheckpointInspectObjectSection
          title={t('pipelineState')}
          value={result?.pipeline_state}
          defaultOpen={false}
        />
      </div>
    </details>
  );
}

export function CheckpointInspectPrimaryTableSection({
  title,
  result,
  sourceStateTitle,
  decodeStrategyLabel,
  coordinatorLabel,
  rawDetailsLabel,
  unsupportedLabel,
}: {
  title: string;
  result: RuntimeStorageCheckpointInspectResult | null;
  sourceStateTitle: string;
  decodeStrategyLabel: string;
  coordinatorLabel: string;
  rawDetailsLabel: string;
  unsupportedLabel: string;
}) {
  const rows = buildCheckpointActionViewModels(result);
  return (
    <CheckpointInspectSectionShell title={title}>
      {rows.length > 0 ? (
        <div className='space-y-4'>
          <div className='hidden rounded-md border border-border/50 bg-muted/10 px-3 py-2 text-xs font-medium text-muted-foreground xl:grid xl:grid-cols-[minmax(0,2fr)_100px_100px_110px_100px_160px_160px] xl:gap-3'>
            <div>Action</div>
            <div>Parallelism</div>
            <div>Subtasks</div>
            <div>Chunks</div>
            <div>Acked</div>
            <div>Latest Ack</div>
            <div>{decodeStrategyLabel}</div>
          </div>
          {rows.map((row) => {
            const actionState = row.actionState || {};
            const statistics = row.taskStatistics || {};
            const sourceSubtasks = Array.isArray(row.sourceState?.subtasks)
              ? (row.sourceState?.subtasks as Record<string, unknown>[])
              : [];
            const subtaskRows = buildCheckpointSubtaskRows(row);
            const subtaskSummaryRows =
              summarizeCheckpointSubtaskMetrics(subtaskRows);

            return (
              <details
                key={row.key}
                className='group rounded-lg border border-border/60 bg-background/60'
              >
                <CheckpointDetailsSummary className='py-3'>
                  <div className='grid gap-2 text-sm xl:grid-cols-[minmax(0,2fr)_100px_100px_110px_100px_160px_160px] xl:items-center xl:gap-3'>
                    <div className='min-w-0 font-medium'>
                      <span className='break-all'>{row.actionName || '-'}</span>
                    </div>
                    <div>{formatCellValue(actionState.parallelism)}</div>
                    <div>{formatCellValue(actionState.subtaskCount)}</div>
                    <div>
                      {formatCellValue(actionState.coordinatorStateChunks)}
                    </div>
                    <div>
                      {formatCellValue(statistics.acknowledgedSubtasks)}
                    </div>
                    <div>
                      {renderCheckpointFieldValue(
                        'latestAckTimestamp',
                        statistics.latestAckTimestamp,
                      )}
                    </div>
                    <div>
                      {formatCellValue(row.sourceState?.decodeStrategy)}
                    </div>
                  </div>
                </CheckpointDetailsSummary>
                <div className='space-y-4 border-t border-border/50 p-3'>
                  <CheckpointInspectInlineTable
                    title='Subtasks Summary'
                    columns={[
                      {key: 'metric', label: 'Metric'},
                      {key: 'Splits', label: 'Splits'},
                      {key: 'Bytes', label: 'Bytes'},
                      {key: 'Chunks', label: 'Chunks'},
                      {key: 'State Size', label: 'State Size'},
                    ]}
                    rows={subtaskSummaryRows}
                  />

                  <CheckpointInspectInlineTable
                    title='Subtasks'
                    columns={[
                      {key: 'subtaskIndex', label: 'Subtask'},
                      {key: 'splitCount', label: 'Splits'},
                      {key: 'bytes', label: 'Bytes'},
                      {key: 'chunks', label: 'Chunks'},
                      {key: 'stateSize', label: 'State Size'},
                      {key: 'status', label: 'Status'},
                      {key: 'ackTimestamp', label: 'Ack Timestamp'},
                    ]}
                    rows={subtaskRows as unknown as Record<string, unknown>[]}
                  />

                  {row.sourceState ? (
                    <details className='group rounded-md border border-border/50 bg-muted/5'>
                      <CheckpointDetailsSummary className='py-3 text-sm font-medium'>
                        <span>
                          {sourceStateTitle} / {rawDetailsLabel}
                        </span>
                      </CheckpointDetailsSummary>
                      <div className='space-y-3 border-t border-border/50 p-3'>
                        <CheckpointInspectMiniObject
                          title={coordinatorLabel}
                          value={
                            row.sourceState.coordinator as
                              | Record<string, unknown>
                              | undefined
                          }
                        />
                        {sourceSubtasks.map((subtask, subtaskIndex) => {
                          const splits = Array.isArray(subtask.splits)
                            ? (subtask.splits as Record<string, unknown>[])
                            : [];
                          return (
                            <div
                              key={`${row.key}-split-group-${subtaskIndex}`}
                              className='rounded-md border border-border/50 p-3'
                            >
                              <div className='mb-3 text-sm font-medium'>
                                Subtask {formatCellValue(subtask.subtaskIndex)}
                              </div>
                              <div className='space-y-2'>
                                {splits.length > 0 ? (
                                  splits.map((split, splitIndex) => (
                                    <CheckpointInspectMiniObject
                                      key={`${row.key}-split-${subtaskIndex}-${splitIndex}`}
                                      title={`Split ${splitIndex + 1}`}
                                      value={split}
                                    />
                                  ))
                                ) : (
                                  <div className='text-sm text-muted-foreground'>
                                    -
                                  </div>
                                )}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </details>
                  ) : row.unsupportedSource ? (
                    <CheckpointInspectMiniObject
                      title={unsupportedLabel}
                      value={row.unsupportedSource}
                    />
                  ) : null}
                </div>
              </details>
            );
          })}
        </div>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </CheckpointInspectSectionShell>
  );
}

export function CheckpointInspectMetricCard({
  label,
  value,
  valueKey,
}: {
  label: string;
  value: unknown;
  valueKey?: string;
}) {
  return (
    <div className='rounded-lg border border-border/60 bg-muted/10 p-3'>
      <div className='text-xs text-muted-foreground'>{label}</div>
      <div className='mt-1 break-all text-sm font-medium'>
        {renderCheckpointFieldValue(valueKey || label, value)}
      </div>
    </div>
  );
}

function CheckpointInspectInlineTable({
  title,
  columns,
  rows,
}: {
  title: string;
  columns: Array<{key: string; label: string}>;
  rows: Record<string, unknown>[];
}) {
  return (
    <div className='rounded-md border border-border/50'>
      <div className='border-b border-border/50 px-3 py-2 text-xs font-medium text-muted-foreground'>
        {title}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((column) => (
              <TableHead key={column.key}>{column.label}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length > 0 ? (
            rows.map((row, rowIndex) => (
              <TableRow key={`${title}-${rowIndex}`}>
                {columns.map((column) => (
                  <TableCell key={column.key}>
                    {renderCheckpointFieldValue(column.key, row[column.key])}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell
                colSpan={columns.length}
                className='text-center text-muted-foreground'
              >
                -
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function CheckpointInspectMiniObject({
  title,
  value,
}: {
  title: string;
  value?: Record<string, unknown> | null;
}) {
  const entries = value ? Object.entries(value) : [];
  return (
    <div className='rounded-md border border-border/50 p-3'>
      <div className='mb-2 text-xs font-medium text-muted-foreground'>
        {title}
      </div>
      {entries.length > 0 ? (
        <div className='grid gap-2 md:grid-cols-2'>
          {entries.map(([key, entryValue]) => (
            <div
              key={key}
              className='rounded border border-border/40 bg-muted/10 p-2'
            >
              <div className='text-[11px] text-muted-foreground'>{key}</div>
              <div className='mt-1 break-all text-sm'>
                {renderCheckpointFieldValue(key, entryValue)}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className='text-sm text-muted-foreground'>-</div>
      )}
    </div>
  );
}

export function CheckpointInspectDialog({
  open,
  onOpenChange,
  result,
  t,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  result: RuntimeStorageCheckpointInspectResult | null;
  t: (key: string) => string;
}) {
  const [copiedPath, setCopiedPath] = useState(false);
  const [copiedJson, setCopiedJson] = useState(false);
  const [activeTab, setActiveTab] = useState('overview');

  const completed = result?.completed_checkpoint;
  const checkpointId =
    completed?.checkpointId !== undefined && completed?.checkpointId !== null
      ? String(completed.checkpointId)
      : '-';
  const status =
    typeof completed?.status === 'string' ? completed.status : 'UNKNOWN';
  const duration =
    completed?.durationMillis !== undefined
      ? `${completed.durationMillis} ms`
      : '-';
  const stateSizeFormatted =
    typeof completed?.stateSize === 'number'
      ? formatSizeBytes(completed.stateSize)
      : completed?.stateSize !== undefined && completed?.stateSize !== null
        ? String(completed.stateSize)
        : '-';

  const handleCopyPath = useCallback(() => {
    if (!result?.path) {
      return;
    }
    void navigator.clipboard.writeText(result.path);
    setCopiedPath(true);
    setTimeout(() => setCopiedPath(false), 2000);
  }, [result?.path]);

  const handleCopyJson = useCallback(() => {
    if (!result) {
      return;
    }
    void navigator.clipboard.writeText(JSON.stringify(result, null, 2));
    setCopiedJson(true);
    setTimeout(() => setCopiedJson(false), 2000);
  }, [result]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='flex h-[86vh] w-[92vw] max-w-[92vw] flex-col overflow-hidden p-0 sm:max-w-[1240px]'>
        {/* 顶部现代化标题与状态栏 */}
        <div className='border-b border-border/60 bg-muted/15 px-6 py-4'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='flex items-center gap-2.5'>
              <div className='flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary'>
                <Activity className='size-4' />
              </div>
              <div>
                <div className='flex items-center gap-2'>
                  <DialogTitle className='text-base font-semibold text-foreground'>
                    {t('checkpointFileDetails')}
                  </DialogTitle>
                  <Badge
                    variant='outline'
                    className='font-mono text-xs px-2 py-0'
                  >
                    #{checkpointId}
                  </Badge>
                  {renderCheckpointFieldValue('status', status)}
                </div>
                <DialogDescription className='sr-only'>
                  {result?.path || '-'}
                </DialogDescription>
              </div>
            </div>

            {/* 右侧关键度量胶囊 */}
            <div className='flex items-center gap-2'>
              <div className='inline-flex items-center gap-1.5 rounded-md border border-border/60 bg-background/80 px-2.5 py-1 text-xs text-muted-foreground shadow-xs'>
                <Clock className='size-3 text-primary' />
                <span className='font-mono font-medium text-foreground'>
                  {duration}
                </span>
              </div>
              <div className='inline-flex items-center gap-1.5 rounded-md border border-border/60 bg-background/80 px-2.5 py-1 text-xs text-muted-foreground shadow-xs'>
                <HardDrive className='size-3 text-emerald-500' />
                <span className='font-mono font-medium text-foreground'>
                  {stateSizeFormatted}
                </span>
              </div>
            </div>
          </div>

          {/* 路径条与一键复制 */}
          <div className='mt-3 flex items-center justify-between gap-2 rounded-md border border-border/50 bg-background/60 px-3 py-1.5 text-xs'>
            <div className='flex min-w-0 items-center gap-2'>
              <FileCode className='size-3.5 shrink-0 text-muted-foreground' />
              <span
                className='truncate font-mono text-[11px] text-muted-foreground'
                title={result?.path}
              >
                {result?.path || '-'}
              </span>
            </div>
            <div className='flex shrink-0 items-center gap-1'>
              <Button
                variant='ghost'
                size='sm'
                className='h-6 gap-1 px-2 text-[11px]'
                onClick={handleCopyPath}
                title={t('copyPath')}
              >
                {copiedPath ? (
                  <>
                    <Check className='size-3 text-emerald-500' />
                    <span className='font-medium text-emerald-600 dark:text-emerald-400'>
                      {t('copied')}
                    </span>
                  </>
                ) : (
                  <>
                    <Copy className='size-3 text-muted-foreground' />
                    <span>{t('copyPath')}</span>
                  </>
                )}
              </Button>
              <Button
                variant='ghost'
                size='sm'
                className='h-6 gap-1 px-2 text-[11px]'
                onClick={handleCopyJson}
                title={t('copyJson')}
              >
                {copiedJson ? (
                  <>
                    <Check className='size-3 text-emerald-500' />
                    <span className='font-medium text-emerald-600 dark:text-emerald-400'>
                      {t('copied')}
                    </span>
                  </>
                ) : (
                  <>
                    <FileCode className='size-3 text-muted-foreground' />
                    <span>{t('copyJson')}</span>
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>

        {/* 主体选项卡布局 */}
        <Tabs
          value={activeTab}
          onValueChange={setActiveTab}
          className='flex min-h-0 flex-1 flex-col'
        >
          <div className='border-b border-border/60 bg-background px-6 pt-2 pb-0'>
            <TabsList className='grid h-9 w-full max-w-[380px] grid-cols-2 bg-muted/40 p-0.5'>
              <TabsTrigger
                value='overview'
                className='gap-1.5 text-xs data-[state=active]:font-semibold'
              >
                <Activity className='size-3.5 text-primary' />
                {t('tabOverviewAndSources')}
              </TabsTrigger>
              <TabsTrigger
                value='actions'
                className='gap-1.5 text-xs data-[state=active]:font-semibold'
              >
                <Layers className='size-3.5 text-indigo-500' />
                {t('tabActionsAndSubtasks')}
              </TabsTrigger>
            </TabsList>
          </div>

          <div className='min-h-0 flex-1 bg-muted/10 p-6'>
            <ScrollArea className='h-full pr-3'>
              {/* Tab 1: 概览与业务位点 */}
              <TabsContent
                value='overview'
                className='mt-0 space-y-5 outline-hidden'
              >
                <CheckpointInspectOverviewSection result={result} t={t} />
                <CheckpointInspectSourceHighlightsSection
                  title={t('checkpointSourceState')}
                  result={result}
                  decodeStrategyLabel={t('decodeStrategy')}
                  splitCountLabel={t('splitCount')}
                  warningsLabel={t('warnings')}
                  unsupportedLabel={t('unsupportedSources')}
                  currentOffsetLabel={t('currentOffset')}
                  targetLabel={t('sourceTarget')}
                  progressLabel={t('sourceProgress')}
                />
              </TabsContent>

              {/* Tab 2: 算子与分片拓扑 */}
              <TabsContent
                value='actions'
                className='mt-0 space-y-5 outline-hidden'
              >
                <CheckpointInspectPrimaryTableSection
                  title={t('actions')}
                  result={result}
                  sourceStateTitle={t('checkpointSourceState')}
                  decodeStrategyLabel={t('decodeStrategy')}
                  coordinatorLabel={t('coordinator')}
                  unsupportedLabel={t('unsupportedSources')}
                  rawDetailsLabel={t('rawDetails')}
                />
              </TabsContent>
            </ScrollArea>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}


