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
 * 错误组详情体：经验库置顶 + 元信息 + 样本摘要 + 近期事件 + 选中事件堆栈 +
 * 底部操作栏，常驻详情栏 / 抽屉形态共用（detailCol 结构由宿主给宽）。
 * Error group detail body: memory card + meta + sample summary + recent events +
 * selected-event stack + footer actions, shared by the persistent rail and the
 * drawer (the detailCol contract is width-adaptive).
 */
import {useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {ArrowUpRight, FileCode, Lightbulb, Server} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Button as UIButton} from '@/components/ui/button';
import {Skeleton} from '@/components/ui/skeleton';
import {ExpandableTextPanel} from '@/components/common/layout';
import {
  TroubleshootingMemoryCard,
  SaveMemoryDialog,
} from '@/components/common/troubleshooting';
import services from '@/lib/services';
import type {
  DiagnosticsClusterOption,
  DiagnosticsErrorEvent,
  DiagnosticsErrorGroup,
} from '@/lib/services/diagnostics';
import {cn} from '@/lib/utils';
import {SoftChip} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {
  formatErrorDateTime,
  formatNodeOrigin,
  occurrenceHeatTone,
  resolveGroupClusterLabel,
} from './error-utils';

// 堆栈证据里把 ERROR 级日志行、"Caused by" 链与异常类行提亮，方便扫读。
// Highlight ERROR-level log lines, "Caused by" chains, and exception class lines in stack evidence for scanability.
const STACK_EVIDENCE_EMPHASIS =
  /\bERROR\b|\bCaused by\b|^[\w.$]+(?:Exception|Error|Throwable)\b/;

export interface ErrorGroupDetailProps {
  /** 保留的错误组（useDrawerItem 输出），深链时可能为空。Retained group (useDrawerItem output); may be null on deep links before the fetch resolves. */
  group: DiagnosticsErrorGroup | null;
  events: DiagnosticsErrorEvent[];
  loading: boolean;
  clusterOptions: DiagnosticsClusterOption[];
  /** 经验库写后版本号，驱动命中重算。Bumped after a memory save so matches recompute. */
  memoriesVersion: number;
  onMemoriesChanged: () => void;
  onClose: () => void;
}

export function ErrorGroupDetail({
  group,
  events,
  loading,
  clusterOptions,
  memoriesVersion,
  onMemoriesChanged,
  onClose,
}: ErrorGroupDetailProps) {
  const t = useTranslations('diagnosticsCenter.errors');
  const tsT = useTranslations('troubleshooting');
  const [selectedEventId, setSelectedEventId] = useState<number | null>(null);
  const [memoryDialogOpen, setMemoryDialogOpen] = useState(false);

  // 命中方案依赖本地记忆存储 + 写后版本号，与旧面板同语义。
  // Memory matching uses the local store plus the post-save version, as before.
  const matchedMemories = useMemo(() => {
    if (!group) {
      return [];
    }
    return services.troubleshooting.findMatchingMemories({
      fingerprint: group.fingerprint,
      exception_class: group.exception_class,
      title: group.title,
      target_type: 'error',
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group, memoriesVersion]);
  const primaryMemory = matchedMemories[0] || null;

  const selectedEvent =
    events.find((item) => item.id === selectedEventId) ?? events[0];

  // 整列都是空 job 时只剩占位符，直接收掉给来源列让位。
  // An all-empty job column is pure placeholder; drop it so the source column gets the room.
  const hasJobIds = events.some((event) => Boolean(event.job_id));

  const clusterLabel = group
    ? resolveGroupClusterLabel(group.last_cluster_id, clusterOptions)
    : '-';

  return (
    <div className={layout.detailCol} data-testid='error-group-detail'>
      <div className={layout.detailScroll}>
        {loading && !group ? (
          <div className='space-y-3'>
            <Skeleton className='h-16 w-full' />
            <Skeleton className='h-28 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        ) : group ? (
          <>
            <TroubleshootingMemoryCard
              matchedMemory={primaryMemory}
              totalMatches={matchedMemories.length}
              onAddOrEdit={() => setMemoryDialogOpen(true)}
            />

            {/* 元信息：频次 / 首末次 / 来源节点 / 归属集群（按 last_cluster_id 反查，
                与当前筛选无关）。Meta: heat / first & last seen / origin node /
                owning cluster (resolved from last_cluster_id, not the filter). */}
            <div className='rounded-lg border p-3.5 space-y-2 bg-muted/15'>
              <div className='font-semibold text-foreground flex items-center gap-1.5 text-xs'>
                <Server className='h-3.5 w-3.5 text-primary' aria-hidden />
                <span>{t('affectedOriginTitle')}</span>
              </div>
              <div className='grid gap-1.5 text-xs text-muted-foreground sm:grid-cols-2'>
                <div className='flex items-center gap-1.5'>
                  <span className='text-foreground font-medium'>
                    {t('columns.occurrences')}:
                  </span>
                  <SoftChip tone={occurrenceHeatTone(group.occurrence_count)}>
                    {t('occurrencesShort', {count: group.occurrence_count})}
                  </SoftChip>
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('firstSeenLabel')}:{' '}
                  </span>
                  {formatErrorDateTime(group.first_seen_at)}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('columns.lastSeen')}:{' '}
                  </span>
                  {formatErrorDateTime(group.last_seen_at)}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('columns.node')}:{' '}
                  </span>
                  {formatNodeOrigin({
                    nodeId: group.last_node_id,
                    hostId: group.last_host_id,
                    hostName: group.last_host_name,
                    hostIp: group.last_host_ip,
                  })}
                </div>
                <div>
                  <span className='text-foreground font-medium'>
                    {t('clusterLabel')}:{' '}
                  </span>
                  {clusterLabel}
                </div>
              </div>
            </div>

            {/* 样本摘要 + 复制（CopyButton 自带失败态，无需手写 clipboard 调用）。
                Sample summary + copy (CopyButton owns the failure state; no manual clipboard call). */}
            <div className='space-y-1.5'>
              <div className='font-semibold text-foreground flex items-center justify-between text-xs'>
                <span>{t('summaryTitle')}</span>
                <CopyButton
                  value={group.sample_message || group.title}
                  label={t('copySummary')}
                  copiedLabel={t('copiedLabel')}
                  failedLabel={t('copyFailedLabel')}
                  variant='plain'
                  className='h-6 px-1.5 text-xs'
                  disabled={!group.sample_message && !group.title}
                />
              </div>
              <div className='rounded-md bg-muted/40 p-3 text-xs leading-relaxed text-foreground font-mono break-all'>
                {group.sample_message || t('noSampleMessage')}
              </div>
            </div>

            {/* 近期事件样本，点击切换下方堆栈。Recent events; selecting swaps the stack below. */}
            <div className='space-y-2 pt-1 border-t'>
              <div className='flex items-center justify-between'>
                <div className='font-semibold text-foreground text-xs'>
                  {t('recentEventsTitle')}
                </div>
                <span className='text-xs text-muted-foreground'>
                  {t('eventsCount', {count: events.length})}
                </span>
              </div>
              <div className='rounded-lg border overflow-hidden'>
                {events.length === 0 ? (
                  <div className='py-4 text-center text-muted-foreground text-xs'>
                    {t('noEvents')}
                  </div>
                ) : (
                  events.map((event) => (
                    <div
                      key={event.id}
                      role='button'
                      tabIndex={0}
                      onClick={() => setSelectedEventId(event.id)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          setSelectedEventId(event.id);
                        }
                      }}
                      className={cn(
                        'grid cursor-pointer items-center gap-2 px-2.5 py-1.5 text-xs transition-colors hover:bg-muted/40',
                        hasJobIds
                          ? 'grid-cols-[110px_minmax(0,1fr)_64px_minmax(0,1fr)]'
                          : 'grid-cols-[110px_minmax(0,1fr)_minmax(0,1fr)]',
                        selectedEvent?.id === event.id &&
                          'bg-primary/5 font-medium border-l-2 border-l-primary',
                      )}
                    >
                      <span
                        className='whitespace-nowrap text-muted-foreground font-mono truncate'
                        title={formatErrorDateTime(event.occurred_at)}
                      >
                        {formatErrorDateTime(event.occurred_at)}
                      </span>
                      <span
                        className='truncate text-muted-foreground'
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
                      </span>
                      {hasJobIds ? (
                        <span className='font-mono text-foreground truncate'>
                          {event.job_id || '-'}
                        </span>
                      ) : null}
                      <span
                        className='truncate font-mono text-muted-foreground'
                        title={event.source_file || undefined}
                      >
                        {event.source_file || '-'}
                      </span>
                    </div>
                  ))
                )}
              </div>
            </div>

            <div className='space-y-2 pt-1 border-t'>
              <div className='font-semibold text-foreground flex items-center gap-1.5 min-w-0 text-xs'>
                <FileCode
                  className='h-4 w-4 text-primary shrink-0'
                  aria-hidden
                />
                <span className='truncate'>{t('evidenceTitle')}</span>
              </div>
              <ExpandableTextPanel
                title={t('evidencePanelTitle')}
                subtitle={
                  selectedEvent?.source_file || t('evidenceDefaultSource')
                }
                content={selectedEvent?.evidence || ''}
                emptyLabel={t('noEvidence')}
                height={260}
                tone='plain'
                wrap
                emphasisPattern={STACK_EVIDENCE_EMPHASIS}
                expandLabel={t('expandEvidence')}
                copyLabel={t('copyEvidence')}
                copiedLabel={t('copiedEvidence')}
              />
            </div>
          </>
        ) : null}
      </div>

      {group ? (
        <div className={layout.detailFoot}>
          {group.last_cluster_id ? (
            <UIButton asChild variant='outline' size='sm'>
              <a
                href={`/clusters/${group.last_cluster_id}`}
                target='_blank'
                rel='noreferrer'
              >
                <ArrowUpRight className='size-3.5' aria-hidden />
                {t('goToCluster')}
              </a>
            </UIButton>
          ) : null}
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={() => setMemoryDialogOpen(true)}
          >
            <Lightbulb className='size-3.5' aria-hidden />
            {primaryMemory ? tsT('updateSolution') : tsT('recordSolution')}
          </Button>
          <span className={layout.detailFootGrow} />
          <Button type='button' variant='ghost' size='sm' onClick={onClose}>
            {t('closeDetail')}
          </Button>
        </div>
      ) : null}

      {/* 归属集群按错误组自身 last_cluster_id 预填，而不是当前筛选集群。
          The owning cluster is prefilled from the group's last_cluster_id, not
          the current filter cluster. */}
      <SaveMemoryDialog
        open={memoryDialogOpen}
        onOpenChange={setMemoryDialogOpen}
        initialData={
          group
            ? {
                id: primaryMemory?.id,
                target_type: 'error',
                fingerprint:
                  group.fingerprint || group.exception_class || group.title,
                title:
                  primaryMemory?.title ||
                  `${group.title || group.exception_class || t('defaultException')} ${tsT('defaultTitleSuffix')}`,
                error_summary: group.sample_message || group.title,
                root_cause: primaryMemory?.root_cause,
                solution: primaryMemory?.solution || '',
                preventive_tips: primaryMemory?.preventive_tips,
                tags:
                  primaryMemory?.tags ||
                  [
                    group.exception_class
                      ? group.exception_class.split('.').pop()?.toLowerCase() ||
                        ''
                      : '',
                  ].filter(Boolean),
                cluster_id: group.last_cluster_id || undefined,
                cluster_name: clusterLabel !== '-' ? clusterLabel : undefined,
                author: primaryMemory?.author,
              }
            : null
        }
        onSaved={onMemoriesChanged}
      />
    </div>
  );
}
