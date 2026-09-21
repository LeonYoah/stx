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
import {useMemo, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  Clock,
  Eye,
  HardDrive,
  Loader2,
  Minus,
  Plus,
  RefreshCw,
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
  if (!job) {
    return (
      <div className='text-sm text-muted-foreground'>
        {t('noCheckpointJob')}
      </div>
    );
  }
  const pipelines = checkpointSnapshot?.overview?.pipelines || [];
  const history = checkpointSnapshot?.history || [];
  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      {loading ? (
        <div className='flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground'>
          <Loader2 className='mr-2 size-4 animate-spin' />
          {t('loadingCheckpoint')}
        </div>
      ) : checkpointSnapshot?.empty_reason ? (
        <div className='flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-border/60 bg-muted/10 p-4 text-sm text-muted-foreground'>
          {checkpointSnapshot.message || t('checkpointEmpty')}
        </div>
      ) : (
        <div className='flex min-h-0 flex-1 flex-col gap-3 overflow-hidden'>
          <div className='grid min-h-0 gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]'>
            <div className='flex min-h-0 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
              <div className='flex items-center justify-between border-b border-border/50 px-3 py-2 text-sm font-medium'>
                <span>{t('checkpointOverview')}</span>
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
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background'>
                    <TableRow className='hover:bg-transparent border-border/50'>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('pipeline')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('triggered')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('completed')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('failed')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('inProgress')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('restored')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('latestCompleted')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pipelines.length > 0 ? (
                      pipelines.map((pipeline) => {
                        const failedCount = pipeline.counts?.failed ?? 0;
                        const completedCount = pipeline.counts?.completed ?? 0;
                        const inProgressCount = pipeline.counts?.inProgress ?? 0;

                        return (
                          <TableRow key={pipeline.pipelineId} className='border-border/40 hover:bg-muted/40'>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs font-medium'>{pipeline.pipelineId}</TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs text-muted-foreground'>
                              {pipeline.counts?.triggered ?? '-'}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs font-semibold text-emerald-600 dark:text-emerald-400'>
                              {completedCount}
                            </TableCell>
                            <TableCell className={cn(
                              'py-1.5 px-2.5 font-mono text-xs',
                              failedCount > 0 ? 'font-semibold text-destructive' : 'text-muted-foreground/60',
                            )}>
                              {failedCount}
                            </TableCell>
                            <TableCell className={cn(
                              'py-1.5 px-2.5 font-mono text-xs',
                              inProgressCount > 0 ? 'font-semibold text-blue-500' : 'text-muted-foreground/60',
                            )}>
                              {inProgressCount}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs text-muted-foreground'>
                              {pipeline.counts?.restored ?? '-'}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs'>
                              {pipeline.latestCompleted?.checkpointId ? (
                                <Badge variant='outline' className='rounded-sm text-[10px] px-1 py-0 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                                  #{pipeline.latestCompleted.checkpointId}
                                </Badge>
                              ) : '-'}
                            </TableCell>
                          </TableRow>
                        );
                      })
                    ) : (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-center text-muted-foreground py-6'
                        >
                          {t('checkpointEmpty')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>

            <div className='flex min-h-0 flex-col overflow-hidden rounded-lg border border-border/50 bg-background/70'>
              <div className='border-b border-border/50 px-3 py-2 text-sm font-medium'>
                {t('checkpointHistory')}
              </div>
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table>
                  <TableHeader className='sticky top-0 z-10 bg-background'>
                    <TableRow className='hover:bg-transparent border-border/50'>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('pipeline')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('checkpointId')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('checkpointStatus')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('durationMillis')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-xs'>{t('stateSize')}</TableHead>
                      <TableHead className='h-8 py-1 px-2.5 text-right text-xs'>
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
                            className='border-border/40 hover:bg-muted/40'
                          >
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs text-muted-foreground'>{item.pipelineId}</TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs font-semibold text-foreground'>
                              {checkpointId ? `#${checkpointId}` : '-'}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5'>
                              {item.checkpoint?.status ? (
                                renderCheckpointFieldValue('status', item.checkpoint.status)
                              ) : (
                                '-'
                              )}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs text-muted-foreground'>
                              {durationStr}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 font-mono text-xs text-foreground/90 font-medium'>
                              {sizeStr}
                            </TableCell>
                            <TableCell className='py-1.5 px-2.5 text-right'>
                              <Button
                                size='sm'
                                variant='outline'
                                className='h-7 px-2 text-xs'
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
                                  <Loader2 className='mr-1.5 size-3.5 animate-spin' />
                                ) : (
                                  <Eye className='mr-1.5 size-3.5' />
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
                          className='text-center text-muted-foreground py-6'
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
    unsupportedSources.length === 0
  ) {
    return null;
  }

  return (
    <CheckpointInspectSectionShell title={title}>
      <div className='space-y-4'>
        {errorMessage ? (
          <div className='rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300'>
            {errorMessage}
          </div>
        ) : null}
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
        {sourceStates.length > 0 ? (
          <div className='rounded-md border border-border/50 bg-background/70'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Source</TableHead>
                  <TableHead>Plugin</TableHead>
                  <TableHead>{currentOffsetLabel}</TableHead>
                  <TableHead>{targetLabel}</TableHead>
                  <TableHead>{splitCountLabel}</TableHead>
                  <TableHead>{progressLabel}</TableHead>
                  <TableHead>{decodeStrategyLabel}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sourceStates.map((item, index) => {
                  const summary = summarizeCheckpointSourceState(item);
                  return (
                    <TableRow
                      key={`${item.actionName || item.pluginName || index}`}
                    >
                      <TableCell className='font-medium'>
                        <span className='break-all'>
                          {String(item.actionName || item.pluginName || '-')}
                        </span>
                      </TableCell>
                      <TableCell>{formatCellValue(item.pluginName)}</TableCell>
                      <TableCell>
                        {renderCheckpointFieldValue(
                          'currentOffset',
                          summary.offset,
                        )}
                      </TableCell>
                      <TableCell>
                        {renderCheckpointFieldValue(
                          'sourceTarget',
                          summary.target,
                        )}
                      </TableCell>
                      <TableCell>{summary.splitCount}</TableCell>
                      <TableCell>{summary.progress}</TableCell>
                      <TableCell>
                        {formatCellValue(item.decodeStrategy)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        ) : null}
        {unsupportedSources.length > 0 ? (
          <details className='group rounded-lg border border-border/60 bg-background/60'>
            <CheckpointDetailsSummary className='py-3 text-sm font-medium'>
              <span>{unsupportedLabel}</span>
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

