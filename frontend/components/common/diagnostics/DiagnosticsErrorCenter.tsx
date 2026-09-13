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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy, FileCode, RefreshCw, Search, X} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  DiagnosticsErrorEvent,
  DiagnosticsErrorGroup,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Skeleton} from '@/components/ui/skeleton';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

type DiagnosticsErrorCenterProps = {
  clusterId?: number;
  clusterName?: string;
  groupId?: number;
  onSelectGroup?: (groupId: number | null) => void;
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

function resolveOccurrenceVariant(
  count: number,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  if (count >= 10) {
    return 'destructive';
  }
  if (count >= 3) {
    return 'secondary';
  }
  return 'outline';
}

// 获取错误频次热度样式（高频突出警示，中频预警，低频常规）
// Get error occurrence heat styling (high: prominent red, mid: warning amber, low: subtle gray)
function getOccurrenceHeatClass(count: number): string {
  if (count >= 10) {
    return 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/30 font-semibold';
  }
  if (count >= 3) {
    return 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30 font-medium';
  }
  return 'bg-muted text-muted-foreground border-border/80';
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

export function DiagnosticsErrorCenter({
  clusterId,
  clusterName,
  groupId,
  onSelectGroup,
}: DiagnosticsErrorCenterProps) {
  const t = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');

  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [loadingGroups, setLoadingGroups] = useState(true);
  const [groups, setGroups] = useState<DiagnosticsErrorGroup[]>([]);
  const [groupTotal, setGroupTotal] = useState(0);
  const [selectedGroupId, setSelectedGroupId] = useState<number | null>(groupId ?? null);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState<DiagnosticsErrorGroup | null>(
    null,
  );
  const [groupEvents, setGroupEvents] = useState<DiagnosticsErrorEvent[]>([]);
  const [selectedEventId, setSelectedEventId] = useState<number | null>(null);
  const [copiedEvidence, setCopiedEvidence] = useState(false);
  const groupsRequestIdRef = useRef(0);
  const detailRequestIdRef = useRef(0);

  // 复制异常堆栈到剪贴板
  // Copy exception evidence stack trace to clipboard
  const handleCopyEvidence = useCallback((text?: string | null) => {
    if (!text) {
      return;
    }
    navigator.clipboard.writeText(text);
    setCopiedEvidence(true);
    toast.success('堆栈信息已复制到剪贴板');
    setTimeout(() => setCopiedEvidence(false), 2000);
  }, []);

  const clearSelectedGroupDetail = useCallback(() => {
    detailRequestIdRef.current += 1;
    setLoadingDetail(false);
    setSelectedGroup(null);
    setGroupEvents([]);
    setSelectedEventId(null);
  }, []);

  const loadGroups = useCallback(async () => {
    const requestId = groupsRequestIdRef.current + 1;
    groupsRequestIdRef.current = requestId;
    setLoadingGroups(true);
    try {
      const result = await services.diagnostics.getErrorGroupsSafe({
        cluster_id: clusterId,
        keyword: keyword || undefined,
        page,
        page_size: 20,
      });
      if (groupsRequestIdRef.current !== requestId) {
        return;
      }
      if (!result.success || !result.data) {
        toast.error(result.error || t('errors.loadGroupsError'));
        setGroups([]);
        setGroupTotal(0);
        return;
      }
      setGroups(result.data.items || []);
      setGroupTotal(result.data.total || 0);
    } finally {
      if (groupsRequestIdRef.current === requestId) {
        setLoadingGroups(false);
      }
    }
  }, [clusterId, keyword, page, t]);

  const loadGroupDetail = useCallback(async (groupId: number) => {
    const requestId = detailRequestIdRef.current + 1;
    detailRequestIdRef.current = requestId;
    setLoadingDetail(true);
    try {
      const result = await services.diagnostics.getErrorGroupDetailSafe(
        groupId,
        20,
        {
          cluster_id: clusterId,
        },
      );
      if (detailRequestIdRef.current !== requestId) {
        return;
      }
      if (!result.success || !result.data) {
        toast.error(result.error || t('errors.loadDetailError'));
        setSelectedGroup(null);
        setGroupEvents([]);
        setSelectedEventId(null);
        return;
      }
      setSelectedGroup(result.data.group);
      setGroupEvents(result.data.events || []);
      setSelectedEventId(result.data.events?.[0]?.id ?? null);
    } finally {
      if (detailRequestIdRef.current === requestId) {
        setLoadingDetail(false);
      }
    }
  }, [clusterId, t]);

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  useEffect(() => {
    if (groups.length === 0) {
      setSelectedGroupId(null);
      clearSelectedGroupDetail();
      return;
    }
    if (!selectedGroupId || !groups.some((item) => item.id === selectedGroupId)) {
      clearSelectedGroupDetail();
      setSelectedGroupId(groups[0].id);
      onSelectGroup?.(groups[0].id);
    }
  }, [clearSelectedGroupDetail, groups, onSelectGroup, selectedGroupId]);

  useEffect(() => {
    if (!selectedGroupId || !groups.some((item) => item.id === selectedGroupId)) {
      return;
    }
    void loadGroupDetail(selectedGroupId);
  }, [groups, loadGroupDetail, selectedGroupId]);

  useEffect(() => {
    clearSelectedGroupDetail();
  }, [clearSelectedGroupDetail, clusterId]);

  useEffect(() => {
    setSelectedGroupId(groupId ?? null);
    if (!groupId) {
      clearSelectedGroupDetail();
    }
  }, [clearSelectedGroupDetail, groupId]);

  const selectedEvent = useMemo(
    () => groupEvents.find((item) => item.id === selectedEventId) ?? groupEvents[0],
    [groupEvents, selectedEventId],
  );

  const totalPages = Math.max(1, Math.ceil(groupTotal / 20));

  return (
    <div className='grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]'>
      <div className='space-y-4'>
        <Card>
          <CardHeader className='space-y-3'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
              <div>
                <CardTitle>{t('errors.title')}</CardTitle>
                <div className='mt-1 text-sm text-muted-foreground'>
                  {clusterId
                    ? t('errors.clusterScopedHint', {
                        name: clusterName || `#${clusterId}`,
                      })
                    : t('errors.globalHint')}
                </div>
              </div>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge variant='outline'>
                  {t('errors.matchedGroups', {count: groupTotal})}
                </Badge>
                <Button variant='outline' onClick={() => void loadGroups()}>
                  <RefreshCw className='mr-2 h-4 w-4' />
                  {commonT('refresh')}
                </Button>
              </div>
            </div>
            <form
              className='flex flex-col gap-2 sm:flex-row'
              onSubmit={(event) => {
                event.preventDefault();
                setPage(1);
                setKeyword(keywordInput.trim());
              }}
            >
              <div className='relative flex-1 min-w-[220px] max-w-xl'>
                <Search className='absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground' />
                <Input
                  value={keywordInput}
                  onChange={(event) => setKeywordInput(event.target.value)}
                  placeholder={t('errors.keywordPlaceholder')}
                  className='pl-9 pr-8 h-9'
                />
                {keywordInput ? (
                  <button
                    type='button'
                    onClick={() => {
                      setKeywordInput('');
                      setKeyword('');
                      setPage(1);
                    }}
                    className='absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                  >
                    <X className='h-3.5 w-3.5' />
                  </button>
                ) : null}
              </div>
              <Button type='submit' size='sm' className='h-9'>
                {t('errors.search')}
              </Button>
            </form>
          </CardHeader>
        </Card>

        <Card className='border shadow-xs'>
          <CardHeader>
            <CardTitle>{t('errors.groupListTitle')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-4'>
            {loadingGroups ? (
              <div className='space-y-3'>
                <Skeleton className='h-10 w-full' />
                <Skeleton className='h-10 w-full' />
                <Skeleton className='h-10 w-full' />
              </div>
            ) : groups.length === 0 ? (
              <div className='rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                {t('errors.empty')}
              </div>
            ) : (
              <>
                <Table className='table-fixed'>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-[38%]'>
                        {t('errors.columns.group')}
                      </TableHead>
                      <TableHead className='w-[19%]'>
                        {t('errors.columns.exception')}
                      </TableHead>
                      <TableHead className='w-[20%]'>
                        {t('errors.columns.node')}
                      </TableHead>
                      <TableHead className='w-[96px] whitespace-nowrap'>
                        {t('errors.columns.occurrences')}
                      </TableHead>
                      <TableHead className='w-[180px] whitespace-nowrap'>
                        {t('errors.columns.lastSeen')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groups.map((group) => (
                      <TableRow
                        key={group.id}
                        className={cn(
                          'cursor-pointer transition-colors',
                          selectedGroupId === group.id
                            ? 'bg-primary/5 font-medium border-l-2 border-l-primary'
                            : 'hover:bg-muted/30',
                        )}
                        onClick={() => {
                          if (selectedGroupId !== group.id) {
                            clearSelectedGroupDetail();
                          }
                          setSelectedGroupId(group.id);
                          onSelectGroup?.(group.id);
                        }}
                      >
                        <TableCell className='w-[38%] max-w-0'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <div className='overflow-hidden'>
                                <div
                                  className='truncate font-medium'
                                  title={group.title || '-'}
                                >
                                  {group.title || '-'}
                                </div>
                                <div
                                  className='mt-1 truncate text-xs text-muted-foreground'
                                  title={
                                    group.sample_message || group.fingerprint
                                  }
                                >
                                  {group.sample_message || group.fingerprint}
                                </div>
                              </div>
                            </TooltipTrigger>
                            <TooltipContent
                              side='top'
                              align='start'
                              className='max-w-[760px] break-all'
                            >
                              <div className='space-y-1 text-xs'>
                                <div>{group.title || '-'}</div>
                                <div className='text-muted-foreground'>
                                  {group.sample_message || group.fingerprint}
                                </div>
                              </div>
                            </TooltipContent>
                          </Tooltip>
                        </TableCell>
                        <TableCell className='w-[19%] max-w-0 text-sm text-muted-foreground'>
                          <div
                            className='truncate'
                            title={group.exception_class || '-'}
                          >
                            {group.exception_class || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='max-w-0 text-sm text-muted-foreground'>
                          <div
                            className='truncate'
                            title={formatNodeOrigin({
                              nodeId: group.last_node_id,
                              hostId: group.last_host_id,
                              hostName: group.last_host_name,
                              hostIp: group.last_host_ip,
                            })}
                          >
                            {formatNodeOrigin({
                              nodeId: group.last_node_id,
                              hostId: group.last_host_id,
                              hostName: group.last_host_name,
                              hostIp: group.last_host_ip,
                            })}
                          </div>
                        </TableCell>
                        <TableCell className='whitespace-nowrap'>
                          <Badge
                            variant='outline'
                            className={cn('text-xs font-mono', getOccurrenceHeatClass(group.occurrence_count))}
                          >
                            {group.occurrence_count}
                          </Badge>
                        </TableCell>
                        <TableCell className='whitespace-nowrap text-sm text-muted-foreground'>
                          {formatDateTime(group.last_seen_at)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
                <div className='flex items-center justify-between text-sm'>
                  <div className='text-muted-foreground'>
                    {t('errors.pageSummary', {
                      page,
                      totalPages,
                    })}
                  </div>
                  <div className='flex items-center gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setPage((current) => Math.max(1, current - 1))}
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

      <Card className='border shadow-xs flex flex-col'>
        <CardHeader>
          <CardTitle>{t('errors.detailTitle')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-4 flex-1'>
          {loadingDetail ? (
            <div className='space-y-3'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-32 w-full' />
              <Skeleton className='h-56 w-full' />
            </div>
          ) : !selectedGroup ? (
            <div className='rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground'>
              {t('errors.selectGroup')}
            </div>
          ) : (
            <>
              <div className='space-y-3 rounded-lg border p-4 bg-muted/10'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge
                    variant='outline'
                    className={cn('text-xs font-mono', getOccurrenceHeatClass(selectedGroup.occurrence_count))}
                  >
                    {t('errors.columns.occurrences')}: {selectedGroup.occurrence_count}
                  </Badge>
                  <Badge variant='outline' className='text-xs'>
                    {t('errors.columns.lastSeen')}: {formatDateTime(selectedGroup.last_seen_at)}
                  </Badge>
                  <Badge variant='outline' className='text-xs'>
                    {t('errors.columns.node')}: {formatNodeOrigin({
                      nodeId: selectedGroup.last_node_id,
                      hostId: selectedGroup.last_host_id,
                      hostName: selectedGroup.last_host_name,
                      hostIp: selectedGroup.last_host_ip,
                    })}
                  </Badge>
                </div>
                <div>
                  <div className='break-all text-sm font-semibold'>
                    {selectedGroup.title || '-'}
                  </div>
                  <div className='mt-1 text-xs font-mono text-muted-foreground break-all'>
                    {selectedGroup.exception_class || selectedGroup.fingerprint}
                  </div>
                </div>
                <div className='rounded-md bg-muted/40 p-3 text-xs leading-relaxed text-muted-foreground font-mono'>
                  {selectedGroup.sample_message || t('errors.noSampleMessage')}
                </div>
              </div>

              <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <div className='text-sm font-medium'>
                    {t('errors.recentEventsTitle')}
                  </div>
                  <Badge variant='secondary' className='text-xs'>
                    {groupEvents.length} 条记录
                  </Badge>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('errors.columns.time')}</TableHead>
                      <TableHead>{t('errors.columns.node')}</TableHead>
                      <TableHead>{t('errors.columns.job')}</TableHead>
                      <TableHead>{t('errors.columns.source')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groupEvents.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={4}
                          className='text-center text-sm text-muted-foreground'
                        >
                          {t('errors.noEvents')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      groupEvents.map((event) => (
                        <TableRow
                          key={event.id}
                          className={cn(
                            'cursor-pointer transition-colors',
                            selectedEventId === event.id
                              ? 'bg-primary/5 font-medium border-l-2 border-l-primary'
                              : 'hover:bg-muted/30',
                          )}
                          onClick={() => setSelectedEventId(event.id)}
                        >
                          <TableCell className='text-xs text-muted-foreground whitespace-nowrap'>
                            {formatDateTime(event.occurred_at)}
                          </TableCell>
                          <TableCell className='max-w-[180px] text-xs text-muted-foreground'>
                            <div
                              className='truncate'
                              title={formatNodeOrigin({
                                nodeId: event.node_id,
                                hostId: event.host_id,
                                hostName: event.host_name,
                                hostIp: event.host_ip,
                                role: event.role,
                              })}
                            >
                              {formatNodeOrigin({
                                nodeId: event.node_id,
                                hostId: event.host_id,
                                hostName: event.host_name,
                                hostIp: event.host_ip,
                                role: event.role,
                              })}
                            </div>
                          </TableCell>
                          <TableCell className='text-xs font-mono'>{event.job_id || '-'}</TableCell>
                          <TableCell className='max-w-[180px] text-xs text-muted-foreground'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className='truncate font-mono'>{event.source_file}</div>
                              </TooltipTrigger>
                              <TooltipContent
                                side='top'
                                align='start'
                                className='max-w-[720px] break-all'
                              >
                                {event.source_file}
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>

              <div className='space-y-3'>
                <div className='rounded-lg border p-3.5 text-xs bg-muted/15 space-y-2'>
                  <div className='font-medium text-foreground'>{t('errors.selectedEventTitle')}</div>
                  <div className='grid gap-1.5 text-muted-foreground sm:grid-cols-2'>
                    <div>
                      <span className='text-foreground font-medium'>{t('errors.columns.node')}: </span>
                      {formatNodeOrigin({
                        nodeId: selectedEvent?.node_id,
                        hostId: selectedEvent?.host_id,
                        hostName: selectedEvent?.host_name,
                        hostIp: selectedEvent?.host_ip,
                        role: selectedEvent?.role,
                      })}
                    </div>
                    <div>
                      <span className='text-foreground font-medium'>Agent: </span>
                      <span className='font-mono'>{selectedEvent?.agent_id || '-'}</span>
                    </div>
                    <div>
                      <span className='text-foreground font-medium'>Job: </span>
                      <span className='font-mono'>{selectedEvent?.job_id || '-'}</span>
                    </div>
                    <div>
                      <span className='text-foreground font-medium'>{t('errors.columns.source')}: </span>
                      <span className='font-mono'>{selectedEvent?.source_file || '-'}</span>
                    </div>
                  </div>
                  {selectedEvent?.message ? (
                    <div className='mt-2 rounded bg-muted/40 p-2 text-muted-foreground font-mono text-xs'>
                      {selectedEvent.message}
                    </div>
                  ) : null}
                </div>

                {/* 异常堆栈与证据监控阅读器 / Stack Trace & Evidence Reader */}
                <div className='space-y-2'>
                  <div className='flex items-center justify-between'>
                    <div className='text-sm font-medium flex items-center gap-2'>
                      <FileCode className='h-4 w-4 text-muted-foreground' />
                      <span>{t('errors.evidenceTitle')}</span>
                    </div>
                    {selectedEvent?.evidence ? (
                      <Button
                        variant='outline'
                        size='sm'
                        className='h-7 text-xs gap-1.5'
                        onClick={() => handleCopyEvidence(selectedEvent.evidence)}
                      >
                        {copiedEvidence ? (
                          <>
                            <Check className='h-3.5 w-3.5 text-emerald-500' />
                            <span>已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className='h-3.5 w-3.5' />
                            <span>复制堆栈</span>
                          </>
                        )}
                      </Button>
                    ) : null}
                  </div>
                  <div className='relative rounded-lg border border-zinc-800 bg-zinc-950 dark:bg-zinc-900/90 text-zinc-200 overflow-hidden shadow-inner'>
                    <div className='flex items-center justify-between px-3.5 py-2 border-b border-zinc-800/80 bg-zinc-900/60 text-[11px] text-zinc-400 font-mono'>
                      <span>Stack Trace / Error Evidence</span>
                      <span className='truncate max-w-[240px]'>{selectedEvent?.source_file || 'Standard Output'}</span>
                    </div>
                    <ScrollArea className='h-[360px]'>
                      <pre className='p-4 font-mono text-xs leading-relaxed text-zinc-300 whitespace-pre-wrap break-words select-text'>
                        {selectedEvent?.evidence || t('errors.noEvidence')}
                      </pre>
                    </ScrollArea>
                  </div>
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
