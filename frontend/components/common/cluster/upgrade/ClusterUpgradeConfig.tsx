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
import {toast} from 'sonner';
import {AlertTriangle, ArrowLeft, FileDiff, Save} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {WorkspaceHeader} from '@/components/common/layout';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Textarea} from '@/components/ui/textarea';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import services from '@/lib/services';
import {
  loadStUpgradeSession,
  patchStUpgradeSession,
} from '@/lib/st-upgrade-session';
import type {
  ConfigMergeFile,
  ConfigMergePlan,
  CreatePlanRequest,
} from '@/lib/services/st-upgrade';
import {cn} from '@/lib/utils';
import {
  buildMergeEditorRows,
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
  descriptionKey: string;
}

export function ClusterUpgradeConfig({clusterId}: ClusterUpgradeConfigProps) {
  const t = useTranslations('stUpgrade');
  const router = useRouter();

  const [request, setRequest] = useState<CreatePlanRequest | null>(null);
  const [mergePlan, setMergePlan] = useState<ConfigMergePlan | null>(null);
  const [selectedConfigType, setSelectedConfigType] = useState('');
  const [creatingPlan, setCreatingPlan] = useState(false);
  const [highlightedRowId, setHighlightedRowId] = useState<string | null>(null);

  useEffect(() => {
    const session = loadStUpgradeSession(clusterId);
    if (!session?.request || !session.precheck?.config_merge_plan) {
      return;
    }

    const initialPlan =
      session.plan?.snapshot.config_merge_plan ||
      session.precheck.config_merge_plan;

    setRequest(session.request);
    setMergePlan(normalizeMergePlan(initialPlan));
    setSelectedConfigType(initialPlan.files[0]?.config_type || '');
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
        toast.warning(t('planBlocked'));
        patchStUpgradeSession(clusterId, {
          clusterId,
          request,
          precheck: result.data.precheck,
          plan: undefined,
          task: undefined,
        });
        return;
      }

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

  if (!request || !mergePlan) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('configTitle')}</CardTitle>
          <CardDescription>{t('missingDraft')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            onClick={() =>
              router.push(`/clusters/${clusterId}/upgrade/prepare`)
            }
          >
            <ArrowLeft className='mr-2 h-4 w-4' />
            {t('startFromPrepare')}
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className='space-y-6' data-testid='upgrade-config-page'>
      <WorkspaceHeader
        icon={<FileDiff />}
        title={t('configTitle')}
        subtitle={
          <span className='inline-flex flex-wrap items-center gap-2'>
            <span>
              {t('targetVersion')}: {request.target_version}
            </span>
            <span>·</span>
            <span>
              {t('unresolvedConflicts')}: {unresolvedCount}
            </span>
            <Badge variant={unresolvedCount === 0 ? 'default' : 'destructive'}>
              {unresolvedCount === 0
                ? t('allConflictsResolved')
                : t('manualResolutionRequired')}
            </Badge>
            <span className='basis-full text-xs'>{t('configDescription')}</span>
          </span>
        }
        actions={
          <>
            <Button
              variant='outline'
              onClick={() =>
                router.push(`/clusters/${clusterId}/upgrade/prepare`)
              }
            >
              <ArrowLeft className='mr-2 h-4 w-4' />
              {t('backToPrepare')}
            </Button>
            <Button
              data-testid='upgrade-config-create-plan'
              onClick={handleCreatePlan}
              disabled={creatingPlan || unresolvedCount > 0}
            >
              <Save className='mr-2 h-4 w-4' />
              {creatingPlan ? t('creatingPlan') : t('createPlan')}
            </Button>
          </>
        }
      />

      <div
        className={cn(
          'rounded-lg border p-4',
          unresolvedCount === 0
            ? 'border-emerald-500/30 bg-emerald-500/5'
            : 'border-amber-500/30 bg-amber-500/5',
        )}
      >
        <div className='flex items-start gap-3'>
          <AlertTriangle
            className={cn(
              'mt-0.5 h-4 w-4 shrink-0',
              unresolvedCount === 0 ? 'text-emerald-600' : 'text-amber-600',
            )}
          />
          <div className='space-y-1 text-sm'>
            <div className='font-medium'>
              {unresolvedCount === 0
                ? t('configReadyToPlan')
                : t('configManualResolutionNotice')}
            </div>
            <div className='text-muted-foreground'>
              {unresolvedCount === 0
                ? t('configReadyToPlanDescription')
                : t('configManualResolutionDescription')}
            </div>
          </div>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('mergePlanFiles')}</CardTitle>
          <CardDescription>{t('mergeGuide')}</CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          {mergePlan.files.length === 0 ? (
            <div className='rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
              {t('noConfigFiles')}
            </div>
          ) : (
            <Tabs
              value={selectedConfigType}
              onValueChange={setSelectedConfigType}
            >
              <TabsList className='flex h-auto w-full flex-wrap justify-start gap-2 bg-transparent p-0'>
                {mergePlan.files.map((file) => (
                  <TabsTrigger
                    key={file.config_type}
                    value={file.config_type}
                    className='border'
                  >
                    <span className='mr-2'>{file.config_type}</span>
                    <Badge
                      variant={
                        file.conflict_count > 0 ? 'destructive' : 'default'
                      }
                    >
                      {getFileBadgeLabel(file, t)}
                    </Badge>
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
        </CardContent>
      </Card>

      {selectedFile ? (
        <Card className='mx-auto max-w-[1800px]'>
          <CardHeader>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <CardTitle>{selectedFile.config_type}</CardTitle>
                <CardDescription>{selectedFile.target_path}</CardDescription>
              </div>
              <Badge
                variant={
                  selectedFile.conflict_count > 0 ? 'destructive' : 'default'
                }
              >
                {getFileBadgeLabel(selectedFile, t)}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className='space-y-6'>
            {selectedFileSummary &&
            selectedFileSummary.state !== 'manual_pending' ? (
              <div className='rounded-md border border-emerald-500/30 bg-emerald-500/5 p-3 text-sm'>
                <div className='font-medium text-foreground'>
                  {t(selectedFileSummary.titleKey)}
                </div>
                <div className='mt-1 text-muted-foreground'>
                  {t(selectedFileSummary.descriptionKey)}
                </div>
              </div>
            ) : null}

            <div className='grid gap-3 xl:grid-cols-[minmax(0,1fr)_92px_minmax(0,1fr)_92px_minmax(0,1fr)]'>
              <PaneHeader
                title={t('localConfig')}
                action={
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => handleApplyFileSource('old')}
                  >
                    {t('applyOldFile')}
                  </Button>
                }
              />
              <div />
              <PaneHeader
                title={t('mergedResult')}
                description={t('mergedResultDescription')}
              />
              <div />
              <PaneHeader
                title={t('targetConfig')}
                action={
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => handleApplyFileSource('new')}
                  >
                    {t('applyNewFile')}
                  </Button>
                }
              />
            </div>

            {selectedRows.length === 0 ? (
              <div className='space-y-3 rounded-lg border border-dashed p-4 text-sm'>
                <div className='font-medium text-foreground'>
                  {t('noConflicts')}
                </div>
                <div className='text-muted-foreground'>
                  {t('noConfigFiles')}
                </div>
              </div>
            ) : (
              <div className='overflow-auto rounded-lg border'>
                <div className='min-w-[1180px]'>
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
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

interface PaneHeaderProps {
  title: string;
  description?: string;
  action?: ReactNode;
}

function PaneHeader({title, description, action}: PaneHeaderProps) {
  return (
    <div className='flex min-h-12 items-start justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2'>
      <div className='space-y-1'>
        <div className='text-sm font-medium'>{title}</div>
        {description ? (
          <div className='text-xs text-muted-foreground'>{description}</div>
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
        'grid grid-cols-[minmax(0,1fr)_92px_minmax(0,1fr)_92px_minmax(0,1fr)] items-stretch border-b last:border-b-0',
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
        arrow='>>'
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
        arrow='<<'
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
  arrow: '>>' | '<<';
  onClick: () => void;
}

function RowActionCell({visible, label, arrow, onClick}: RowActionCellProps) {
  return (
    <div className='flex items-stretch justify-center border-x bg-muted/20 p-2'>
      {visible ? (
        <Button
          size='sm'
          variant='outline'
          className='h-full w-full border-dashed px-2 text-xs'
          onClick={onClick}
        >
          <span className='font-mono text-base font-semibold'>{arrow}</span>
          <span className='ml-1 whitespace-normal text-center'>{label}</span>
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
      <div className='border-r px-2 py-2 text-right text-[11px] leading-5 text-muted-foreground'>
        {lineNumber || ''}
      </div>
      <div className='px-3 py-2 whitespace-pre-wrap break-all leading-5'>
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
        'grid grid-cols-[52px_minmax(0,1fr)] border-x font-mono text-xs',
        !row.changed && 'bg-muted/10',
        row.changed && !row.resolved && 'bg-amber-500/10',
        highlighted && 'bg-primary/10 transition-colors',
      )}
    >
      <div className='border-r px-2 py-2 text-right text-[11px] leading-5 text-muted-foreground'>
        {row.resultLineNumber}
      </div>
      <div className='px-3 py-1.5'>
        {!row.resultPresent && row.resolved ? (
          <div className='rounded border border-dashed border-muted-foreground/40 px-2 py-1.5 text-xs text-muted-foreground'>
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
      descriptionKey: 'configManualResolutionDescription',
    };
  }

  if (buildMergeEditorRows(file).some((row) => row.changed)) {
    return {
      state: 'manual_resolved',
      badgeKey: 'resolved',
      titleKey: 'manualMergeResolvedTitle',
      descriptionKey: 'manualMergeResolvedDescription',
    };
  }

  return {
    state: 'no_difference',
    badgeKey: 'noConflict',
    titleKey: 'noDifferenceTitle',
    descriptionKey: 'noDifferenceDescription',
  };
}
