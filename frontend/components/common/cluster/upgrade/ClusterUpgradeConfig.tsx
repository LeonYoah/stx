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

import {type ReactNode, useEffect, useMemo, useState} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {AlertTriangle, ArrowLeft, CheckCircle2, FileDiff, Save} from 'lucide-react';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Badge} from '@/components/arc/badge/badge';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {WorkspaceHeader} from '@/components/common/layout';
import {Textarea} from '@/components/ui/textarea';
import {Skeleton} from '@/components/ui/skeleton';
import {Tabs, TabsList, TabsTrigger} from '@/components/arc/tabs/tabs';
import services from '@/lib/services';
import {
  loadStUpgradeSession,
  patchStUpgradeSession,
} from '@/lib/st-upgrade-session';
import type {
  BlockingIssue,
  ConfigMergeFile,
  ConfigMergePlan,
  CreatePlanRequest,
} from '@/lib/services/st-upgrade';
import {cn} from '@/lib/utils';
import {UpgradeSteps} from './UpgradeSteps';
import {HelpHint} from '@/components/common/HelpHint';
import {DisabledReasonTooltip} from '@/components/common/DisabledReasonTooltip';
import {
  buildMergeEditorRows,
  getIssueCategoryLabel,
  normalizeMergePlan,
  rebuildMergeFileFromRows,
} from './utils';

interface ClusterUpgradeConfigProps {
  clusterId: number;
}

type ConflictSource = 'old' | 'new';
type MergeFileState = 'manual_pending' | 'manual_resolved' | 'no_difference';

type TranslateFn = (key: string) => string;

interface MergeFileSummary {
  state: MergeFileState;
  badgeKey: string;
  titleKey: string;
}

export function ClusterUpgradeConfig({clusterId}: ClusterUpgradeConfigProps) {
  const t = useTranslations('stUpgrade');
  const router = useRouter();

  const [request, setRequest] = useState<CreatePlanRequest | null>(null);
  const [mergePlan, setMergePlan] = useState<ConfigMergePlan | null>(null);
  const [selectedConfigType, setSelectedConfigType] = useState('');
  const [creatingPlan, setCreatingPlan] = useState(false);
  const [highlightedRowId, setHighlightedRowId] = useState<string | null>(null);
  // session 同步水合完成前显示骨架 / Skeleton until session hydrate finishes
  const [hydrated, setHydrated] = useState(false);
  // 生成计划被阻断时展示的预检查问题 / Blocking precheck issues after createPlan fails
  const [blockedIssues, setBlockedIssues] = useState<BlockingIssue[]>([]);

  useEffect(() => {
    const session = loadStUpgradeSession(clusterId);
    if (!session?.request || !session.precheck?.config_merge_plan) {
      setHydrated(true);
      return;
    }

    const initialPlan =
      session.plan?.snapshot.config_merge_plan ||
      session.precheck.config_merge_plan;

    setRequest(session.request);
    setMergePlan(normalizeMergePlan(initialPlan));
    setSelectedConfigType(initialPlan.files[0]?.config_type || '');
    // 恢复上次 createPlan 阻断时写入 session 的 issues / Restore issues from last blocked createPlan
    const sessionIssues = (session.precheck.issues || []).filter(
      (issue) => issue.blocking,
    );
    setBlockedIssues(sessionIssues);
    setHydrated(true);
  }, [clusterId]);

  useEffect(() => {
    if (!request || !mergePlan) {
      return;
    }

    const session = loadStUpgradeSession(clusterId);
    if (!session?.precheck) {
      return;
    }

    patchStUpgradeSession(clusterId, {
      request,
      precheck: {
        ...session.precheck,
        config_merge_plan: mergePlan,
      },
    });
  }, [clusterId, mergePlan, request]);

  useEffect(() => {
    if (!mergePlan?.files.length) {
      return;
    }

    const hasSelectedFile = mergePlan.files.some(
      (file) => file.config_type === selectedConfigType,
    );
    if (!hasSelectedFile) {
      setSelectedConfigType(mergePlan.files[0]?.config_type || '');
    }
  }, [mergePlan, selectedConfigType]);

  const selectedFile = useMemo(
    () =>
      mergePlan?.files.find(
        (file) => file.config_type === selectedConfigType,
      ) ||
      mergePlan?.files[0] ||
      null,
    [mergePlan, selectedConfigType],
  );

  const selectedFileSummary = useMemo(
    () => (selectedFile ? getMergeFileSummary(selectedFile) : null),
    [selectedFile],
  );
  const selectedRows = useMemo(
    () => (selectedFile ? buildMergeEditorRows(selectedFile) : []),
    [selectedFile],
  );

  const unresolvedCount = mergePlan?.conflict_count || 0;
  // 有未处理差异时不能生成计划 / Plan creation is blocked while differences remain unresolved
  const createPlanBlockedReason =
    unresolvedCount > 0 ? t('createPlanBlockedHint') : null;

  const updateMergeFile = (
    configType: string,
    updater: (current: ConfigMergeFile) => ConfigMergeFile,
  ) => {
    setMergePlan((currentPlan) => {
      if (!currentPlan) {
        return currentPlan;
      }

      const nextPlan = {
        ...currentPlan,
        files: currentPlan.files.map((file) =>
          file.config_type === configType ? updater(file) : file,
        ),
      };

      return normalizeMergePlan(nextPlan);
    });
  };

  useEffect(() => {
    if (!highlightedRowId) {
      return undefined;
    }

    const timer = window.setTimeout(() => {
      setHighlightedRowId(null);
    }, 1200);

    return () => {
      window.clearTimeout(timer);
    };
  }, [highlightedRowId]);

  const updateMergeRows = (
    configType: string,
    updater: (
      rows: ReturnType<typeof buildMergeEditorRows>,
    ) => ReturnType<typeof buildMergeEditorRows>,
  ) => {
    updateMergeFile(configType, (file) =>
      rebuildMergeFileFromRows(file, updater(buildMergeEditorRows(file))),
    );
  };

  const handleUseRowSource = (rowId: string, source: ConflictSource) => {
    if (!selectedFile) {
      return;
    }

    updateMergeRows(selectedFile.config_type, (rows) =>
      rows.map((row) => {
        if (row.id !== rowId) {
          return row;
        }

        const nextValue = source === 'old' ? row.oldValue : row.newValue;
        const nextPresent =
          source === 'old'
            ? row.oldLineNumber !== null
            : row.newLineNumber !== null;

        return {
          ...row,
          resultValue: nextValue,
          resultPresent: nextPresent,
          resolved: true,
        };
      }),
    );
    setHighlightedRowId(rowId);
  };

  const handleApplyFileSource = (source: ConflictSource) => {
    if (!selectedFile) {
      return;
    }

    updateMergeRows(selectedFile.config_type, (rows) =>
      rows.map((row) =>
        row.changed
          ? {
              ...row,
              resultValue: source === 'old' ? row.oldValue : row.newValue,
              resultPresent:
                source === 'old'
                  ? row.oldLineNumber !== null
                  : row.newLineNumber !== null,
              resolved: true,
            }
          : row,
      ),
    );

    toast.success(
      source === 'old' ? t('fileAppliedFromOld') : t('fileAppliedFromNew'),
    );
  };

  const handleEditRowValue = (rowId: string, value: string) => {
    if (!selectedFile) {
      return;
    }

    updateMergeRows(selectedFile.config_type, (rows) =>
      rows.map((row) =>
        row.id === rowId
          ? {
              ...row,
              resultValue: value,
              resultPresent: true,
              resolved: true,
            }
          : row,
      ),
    );
  };

  const handleCreatePlan = async () => {
    if (!request || !mergePlan) {
      toast.error(t('missingDraft'));
      return;
    }
    if (unresolvedCount > 0) {
      toast.error(t('resolveConflictsBeforeCreatePlan'));
      return;
    }

    setCreatingPlan(true);
    try {
      const normalizedPlan = normalizeMergePlan(mergePlan);
      const result = await services.stUpgrade.createPlanSafe({
        ...request,
        config_merge_plan: normalizedPlan,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('planCreateFailed'));
        return;
      }
      if (!result.data.plan) {
        const issues = (result.data.precheck?.issues || []).filter(
          (issue) => issue.blocking,
        );
        setBlockedIssues(issues);
        toast.warning(
          issues.length > 0
            ? t('planBlockedWithCount', {count: issues.length})
            : t('planBlocked'),
        );
        patchStUpgradeSession(clusterId, {
          clusterId,
          request,
          precheck: result.data.precheck,
          plan: undefined,
          task: undefined,
        });
        return;
      }

      setBlockedIssues([]);
      patchStUpgradeSession(clusterId, {
        clusterId,
        request,
        precheck: result.data.precheck,
        plan: result.data.plan,
        task: undefined,
      });
      toast.success(t('planCreated'));
      router.push(
        `/clusters/${clusterId}/upgrade/execute?planId=${result.data.plan.id}`,
      );
    } finally {
      setCreatingPlan(false);
    }
  };

  if (!hydrated) {
    return (
      <div
        className='upgrade-page arc-compact'
        data-testid='upgrade-config-page'
      >
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('clusterDetailBreadcrumb'),
              href: `/clusters/${clusterId}`,
            },
            {
              label: t('prepareTitle'),
              href: `/clusters/${clusterId}/upgrade/prepare`,
            },
            {
              label: t('configTitle'),
            },
          ]}
          icon={<FileDiff />}
          title={t('configTitle')}
          subtitle={<span>{t('configDescription')}</span>}
        />
        <UpgradeSteps current='config' clusterId={clusterId} />
        <div className='rounded-(--radius-panel) border border-(--border) bg-card p-5 space-y-4'>
          <Skeleton className='h-9 w-80' />
          <div className='grid grid-cols-1 gap-4 border-t-[0.5px] border-border/80 pt-4 xl:grid-cols-3'>
            <Skeleton className='h-80 w-full' />
            <Skeleton className='h-80 w-full' />
            <Skeleton className='h-80 w-full' />
          </div>
        </div>
      </div>
    );
  }

  if (!request || !mergePlan) {
    return (
      <div
        className='upgrade-page arc-compact'
        data-testid='upgrade-config-page'
      >
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('clusterDetailBreadcrumb'),
              href: `/clusters/${clusterId}`,
            },
            {
              label: t('prepareTitle'),
              href: `/clusters/${clusterId}/upgrade/prepare`,
            },
            {
              label: t('configTitle'),
            },
          ]}
          icon={<FileDiff />}
          title={t('configTitle')}
          subtitle={<span>{t('configDescription')}</span>}
        />
        <UpgradeSteps current='config' clusterId={clusterId} />
        <div className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-10'>
          <EmptyState
            icon={<FileDiff />}
            title={t('configTitle')}
            description={t('missingDraft')}
            action={
              <Button
                size='sm'
                onClick={() =>
                  router.push(`/clusters/${clusterId}/upgrade/prepare`)
                }
              >
                <ArrowLeft className='size-3.5' aria-hidden />
                {t('startFromPrepare')}
              </Button>
            }
          />
        </div>
      </div>
    );
  }

  return (
    <div
      className='upgrade-page arc-compact'
      data-testid='upgrade-config-page'
    >
      <WorkspaceHeader
        breadcrumbs={[
          {
            label: t('clusterDetailBreadcrumb'),
            href: `/clusters/${clusterId}`,
          },
          {
            label: t('prepareTitle'),
            href: `/clusters/${clusterId}/upgrade/prepare`,
          },
          {
            label: t('configTitle'),
          },
        ]}
        icon={<FileDiff />}
        title={t('configTitle')}
        subtitle={
          <span className='inline-flex flex-wrap items-center gap-1.5 tabular-nums'>
            <span>
              {t('targetVersion')}: {request.target_version}
            </span>
            <span>·</span>
            <span>
              {t('unresolvedConflicts')}: {unresolvedCount}
            </span>
            <Badge
              tone={unresolvedCount === 0 ? 'success' : 'warning'}
              outline
              dot
              size='sm'
            >
              {unresolvedCount === 0
                ? t('allConflictsResolved')
                : t('manualResolutionRequired')}
            </Badge>
            <HelpHint label={t('configTitle')} content={t('configDescription')} />
          </span>
        }
        actions={
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              variant='secondary'
              size='sm'
              onClick={() =>
                router.push(`/clusters/${clusterId}/upgrade/prepare`)
              }
            >
              <ArrowLeft className='size-3.5' aria-hidden />
              {t('backToPrepare')}
            </Button>
            {/* 只渲染一个 Button：有未处理差异时用 aria-disabled + Tooltip，状态变化不重新挂载 */}
            {/* A single Button: aria-disabled + tooltip while differences remain, never remounted */}
            <DisabledReasonTooltip reason={createPlanBlockedReason}>
              <Button
                size='sm'
                data-testid='upgrade-config-create-plan'
                aria-disabled={createPlanBlockedReason ? 'true' : undefined}
                loading={!createPlanBlockedReason && creatingPlan}
                onClick={(event) => {
                  if (createPlanBlockedReason) {
                    event.preventDefault();
                    return;
                  }
                  void handleCreatePlan();
                }}
              >
                <Save className='size-3.5' aria-hidden />
                {creatingPlan ? t('creatingPlan') : t('createPlan')}
              </Button>
            </DisabledReasonTooltip>
          </div>
        }
      />

      <UpgradeSteps current='config' clusterId={clusterId} />

      {blockedIssues.length > 0 ? (
        <section
          className='overflow-hidden rounded-(--radius-panel) border border-amber-500/40 bg-card px-5 py-4'
          data-testid='upgrade-config-blocked-issues'
        >
          <div className='flex flex-wrap items-start justify-between gap-3 pb-3'>
            <div className='space-y-0.5'>
              <h2 className='flex items-center gap-2 text-sm font-semibold text-foreground'>
                <AlertTriangle className='size-4 text-amber-600' aria-hidden />
                {t('planBlockedIssuesTitle')}
              </h2>
              <p className='text-xs text-muted-foreground'>
                {t('planBlockedIssuesDescription')}
              </p>
            </div>
            <Button
              variant='secondary'
              size='sm'
              onClick={() =>
                router.push(`/clusters/${clusterId}/upgrade/prepare`)
              }
            >
              <ArrowLeft className='size-3.5' aria-hidden />
              {t('backToPrepare')}
            </Button>
          </div>
          <div className='divide-y-[0.5px] divide-border/70 border-t-[0.5px] border-border/80'>
            {blockedIssues.map((issue) => (
              <div
                key={`${issue.category}-${issue.code}-${issue.message}`}
                className='py-2.5 text-xs'
              >
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge tone='danger' outline size='sm'>
                    {getIssueCategoryLabel(issue.category)}
                  </Badge>
                  <span className='font-mono font-medium text-foreground'>
                    {issue.code}
                  </span>
                </div>
                <div className='mt-1 text-muted-foreground'>{issue.message}</div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      <ArcAlert
        tone={unresolvedCount === 0 ? 'success' : 'warning'}
        title={
          unresolvedCount === 0
            ? t('configReadyToPlan')
            : t('configManualResolutionNotice')
        }
      />

      {/* 文件 Tab 与三栏合并器收敛到同一张通栏单层扁平画布，以 0.5px 细线分隔 */}
      {/* File tabs and 3-column merge editor unified in one full-width flat single-layer surface */}
      <section className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
        <div className='space-y-3 px-5 py-4'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <h2 className='flex items-center gap-1.5 text-sm font-semibold text-foreground'>
              {t('mergePlanFiles')}
              <HelpHint
                label={t('mergePlanFiles')}
                content={t('mergeGuide')}
                data-testid='upgrade-config-merge-guide-hint'
              />
            </h2>
            {selectedFile ? (
              <div className='flex items-center gap-2 text-xs text-muted-foreground'>
                <span
                  className='max-w-[360px] truncate font-mono'
                  title={selectedFile.target_path}
                >
                  {selectedFile.target_path}
                </span>
                <Badge
                  tone={selectedFile.conflict_count > 0 ? 'danger' : 'success'}
                  outline
                  dot
                  size='sm'
                >
                  {getFileBadgeLabel(selectedFile, t)}
                </Badge>
              </div>
            ) : null}
          </div>

          {mergePlan.files.length === 0 ? (
            <EmptyState
              icon={<FileDiff />}
              title={t('noConfigFiles')}
              description={t('mergeGuide')}
            />
          ) : (
            <Tabs
              value={selectedConfigType}
              onValueChange={setSelectedConfigType}
            >
              <TabsList aria-label={t('mergePlanFiles')}>
                {mergePlan.files.map((file) => (
                  <TabsTrigger key={file.config_type} value={file.config_type}>
                    <span>{file.config_type}</span>
                    {/* 冲突文件用红色数字角标，无冲突用状态文案 / Conflict count badge vs status label */}
                    {file.conflict_count > 0 ? (
                      <Badge
                        tone='danger'
                        outline
                        size='sm'
                        title={`${file.conflict_count} ${t('pendingResolution')}`}
                      >
                        {file.conflict_count}
                      </Badge>
                    ) : (
                      <Badge tone='success' outline size='sm'>
                        {getFileBadgeLabel(file, t)}
                      </Badge>
                    )}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
        </div>

        {selectedFile ? (
          <div className='border-t-[0.5px] border-border/80'>
            {selectedFileSummary &&
            selectedFileSummary.state !== 'manual_pending' ? (
              <div className='flex items-center gap-2 border-b-[0.5px] border-border/80 bg-emerald-500/5 px-5 py-2.5 text-xs font-medium text-emerald-700 dark:text-emerald-300'>
                <CheckCircle2 className='size-3.5 shrink-0' aria-hidden />
                <span>{t(selectedFileSummary.titleKey)}</span>
              </div>
            ) : null}

            {selectedRows.length === 0 ? (
              <div className='px-6 py-8'>
                <EmptyState
                  icon={<FileDiff />}
                  title={t('noConflicts')}
                  description={t('noConfigFiles')}
                />
              </div>
            ) : (
              <div className='overflow-x-auto'>
                <div className='min-w-[1180px]'>
                  <div className='grid grid-cols-[minmax(0,1fr)_7.5rem_minmax(0,1fr)_7.5rem_minmax(0,1fr)] border-b-[0.5px] border-border/80 bg-muted/20'>
                    <PaneHeader
                      title={t('localConfig')}
                      action={
                        <Button
                          size='sm'
                          variant='secondary'
                          onClick={() => handleApplyFileSource('old')}
                        >
                          {t('applyOldFile')}
                        </Button>
                      }
                    />
                    <div className='border-x-[0.5px] border-border/70' />
                    <PaneHeader
                      title={t('mergedResult')}
                      hint={t('mergedResultDescription')}
                    />
                    <div className='border-x-[0.5px] border-border/70' />
                    <PaneHeader
                      title={t('targetConfig')}
                      action={
                        <Button
                          size='sm'
                          variant='secondary'
                          onClick={() => handleApplyFileSource('new')}
                        >
                          {t('applyNewFile')}
                        </Button>
                      }
                    />
                  </div>
                  {selectedRows.map((row) => (
                    <MergeEditorRowItem
                      key={row.id}
                      row={row}
                      highlighted={highlightedRowId === row.id}
                      t={t}
                      onUseOld={() => handleUseRowSource(row.id, 'old')}
                      onUseNew={() => handleUseRowSource(row.id, 'new')}
                      onChangeResult={(value) =>
                        handleEditRowValue(row.id, value)
                      }
                    />
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : null}
      </section>
    </div>
  );
}

interface PaneHeaderProps {
  title: string;
  hint?: string;
  action?: ReactNode;
}

function PaneHeader({title, hint, action}: PaneHeaderProps) {
  return (
    <div className='flex min-h-11 items-center justify-between gap-2 px-3 py-2'>
      <div className='flex items-center gap-1.5 text-xs font-semibold text-foreground'>
        {title}
        {hint ? (
          <HelpHint
            label={title}
            content={hint}
            data-testid='upgrade-config-pane-hint'
          />
        ) : null}
      </div>
      {action}
    </div>
  );
}

interface MergeEditorRowItemProps {
  row: ReturnType<typeof buildMergeEditorRows>[number];
  highlighted: boolean;
  t: TranslateFn;
  onUseOld: () => void;
  onUseNew: () => void;
  onChangeResult: (value: string) => void;
}

function MergeEditorRowItem({
  row,
  highlighted,
  t,
  onUseOld,
  onUseNew,
  onChangeResult,
}: MergeEditorRowItemProps) {
  return (
    <div
      className={cn(
        'grid grid-cols-[minmax(0,1fr)_7.5rem_minmax(0,1fr)_7.5rem_minmax(0,1fr)] items-stretch border-b-[0.5px] border-border/70 last:border-b-0',
        row.changed ? 'bg-amber-500/[0.03]' : 'bg-background',
      )}
    >
      <CodeLineCell
        lineNumber={row.oldLineNumber}
        value={row.oldValue}
        variant={row.changed ? 'old' : 'neutral'}
      />
      <RowActionCell
        visible={row.changed}
        label={t('acceptOld')}
        onClick={onUseOld}
      />
      <ResultLineCell
        row={row}
        highlighted={highlighted}
        pendingLabel={t('pendingResolution')}
        removedLabel={t('removedLineLabel')}
        onChange={onChangeResult}
      />
      <RowActionCell
        visible={row.changed}
        label={t('acceptNew')}
        onClick={onUseNew}
      />
      <CodeLineCell
        lineNumber={row.newLineNumber}
        value={row.newValue}
        variant={row.changed ? 'new' : 'neutral'}
      />
    </div>
  );
}

interface RowActionCellProps {
  visible: boolean;
  label: string;
  onClick: () => void;
}

function RowActionCell({visible, label, onClick}: RowActionCellProps) {
  return (
    <div className='flex items-stretch justify-center border-x-[0.5px] border-border/70 bg-muted/15 p-2'>
      {visible ? (
        <Button size='sm' variant='secondary' onClick={onClick}>
          {label}
        </Button>
      ) : (
        <div className='w-full' />
      )}
    </div>
  );
}

interface CodeLineCellProps {
  lineNumber: number | null;
  value: string;
  variant: 'old' | 'new' | 'neutral';
}

function CodeLineCell({lineNumber, value, variant}: CodeLineCellProps) {
  return (
    <div
      className={cn(
        'grid grid-cols-[52px_minmax(0,1fr)] font-mono text-xs',
        variant === 'old' && 'bg-rose-500/10',
        variant === 'new' && 'bg-emerald-500/10',
      )}
    >
      <div className='border-r-[0.5px] border-border/70 px-2 py-2 text-right text-[11px] leading-5 text-muted-foreground tabular-nums'>
        {lineNumber || ''}
      </div>
      <div className='overflow-x-auto px-3 py-2 whitespace-pre-wrap break-words leading-5'>
        {value || ' '}
      </div>
    </div>
  );
}

interface ResultLineCellProps {
  row: ReturnType<typeof buildMergeEditorRows>[number];
  highlighted: boolean;
  pendingLabel: string;
  removedLabel: string;
  onChange: (value: string) => void;
}

function ResultLineCell({
  row,
  highlighted,
  pendingLabel,
  removedLabel,
  onChange,
}: ResultLineCellProps) {
  return (
    <div
      className={cn(
        'grid grid-cols-[52px_minmax(0,1fr)] font-mono text-xs',
        !row.changed && 'bg-muted/10',
        row.changed && !row.resolved && 'bg-amber-500/10',
        highlighted && 'bg-primary/10 transition-colors',
      )}
    >
      <div className='border-r-[0.5px] border-border/70 px-2 py-2 text-right text-[11px] leading-5 text-muted-foreground tabular-nums'>
        {row.resultLineNumber}
      </div>
      <div className='px-3 py-1.5'>
        {!row.resultPresent && row.resolved ? (
          <div className='py-1 text-xs italic text-muted-foreground'>
            {removedLabel}
          </div>
        ) : (
          <Textarea
            value={row.resultPresent ? row.resultValue : ''}
            onChange={(event) => onChange(event.target.value)}
            placeholder={row.changed ? pendingLabel : ''}
            className='min-h-[44px] resize-none border-0 bg-transparent px-0 py-1 font-mono text-xs leading-5 shadow-none focus-visible:ring-0'
          />
        )}
      </div>
    </div>
  );
}

function getFileBadgeLabel(file: ConfigMergeFile, t: TranslateFn): string {
  if (file.conflict_count > 0) {
    return `${file.conflict_count} ${t('pendingResolution')}`;
  }

  const summary = getMergeFileSummary(file);
  return t(summary.badgeKey);
}

function getMergeFileSummary(file: ConfigMergeFile): MergeFileSummary {
  if (file.conflict_count > 0) {
    return {
      state: 'manual_pending',
      badgeKey: 'pendingResolution',
      titleKey: 'configManualResolutionNotice',
    };
  }

  if (buildMergeEditorRows(file).some((row) => row.changed)) {
    return {
      state: 'manual_resolved',
      badgeKey: 'resolved',
      titleKey: 'manualMergeResolvedTitle',
    };
  }

  return {
    state: 'no_difference',
    badgeKey: 'noConflict',
    titleKey: 'noDifferenceTitle',
  };
}
