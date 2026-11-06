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

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Collapsible as CollapsiblePrimitive} from 'radix-ui';
import {isSoleDefaultCluster} from '@/lib/cluster-preference';
import {ChevronRight, Plus} from 'lucide-react';
import {Combobox} from '@/components/arc/combobox/combobox';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Badge} from '@/components/ui/badge';
import {cn} from '@/lib/utils';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {HelpHint} from '@/components/common/HelpHint';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {SyncTaskTimeoutDefaults} from '@/lib/services/sync';
import {CustomVariablesSection} from './CustomVariablesSection';
import {TaskAlertSection} from './TaskAlertSection';
import {resolveBuiltinPreviewExpression} from './builtin-time-variables';
import {parseTaskTimeoutInput} from './sync-studio-utils';
import type {
  ExecutionMode,
  ObservabilityMode,
  TaskTimeoutKey,
  VariableRow,
} from './sync-studio-utils';

/**
 * 任务级超时输入框：本地保存输入，合法或清空时才写回任务定义，非法时就地提示。
 * Task timeout input: keeps the raw text locally and writes back only valid or cleared values, flagging bad input inline.
 */
function TaskTimeoutInput({
  label,
  value,
  defaultSeconds,
  testId,
  onChange,
  onInvalidChange,
}: {
  label: string;
  value: string;
  defaultSeconds?: number;
  testId: string;
  onChange: (minutes: number | null) => void;
  onInvalidChange?: (invalid: boolean) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [draft, setDraft] = useState(value);
  useEffect(() => {
    setDraft(value);
  }, [value]);
  const parsed = parseTaskTimeoutInput(draft);
  const invalid = parsed === undefined;
  useEffect(() => {
    onInvalidChange?.(invalid);
  }, [invalid, onInvalidChange]);
  const defaultMinutes =
    defaultSeconds && defaultSeconds > 0
      ? Math.max(1, Math.round(defaultSeconds / 60))
      : 5;
  return (
    <div className='min-w-0 space-y-1'>
      <Label className='whitespace-nowrap text-[11px] text-muted-foreground'>
        {label}
      </Label>
      <Input
        value={draft}
        inputMode='numeric'
        data-testid={testId}
        aria-invalid={invalid}
        placeholder={t('timeoutDefaultPlaceholder', {minutes: defaultMinutes})}
        className={
          invalid
            ? 'h-9 rounded-[var(--radius-control)] text-xs border-destructive focus-visible:ring-destructive/40'
            : 'h-9 rounded-[var(--radius-control)] text-xs'
        }
        onChange={(event) => {
          const next = event.target.value;
          setDraft(next);
          const minutes = parseTaskTimeoutInput(next);
          if (minutes !== undefined) {
            onChange(minutes);
          }
        }}
      />
      {invalid ? (
        <p className='text-[10px] text-destructive'>{t('timeoutInvalid')}</p>
      ) : null}
    </div>
  );
}

const TIMEOUT_OPEN_STORAGE_KEY = 'stx_studio_sidebar_timeout_open';

/** 默认分钟：默认秒缺失时用 5。 / Default minutes: 5 when default seconds are missing. */
function defaultTimeoutMinutes(defaultSeconds?: number): number {
  return defaultSeconds && defaultSeconds > 0
    ? Math.max(1, Math.round(defaultSeconds / 60))
    : 5;
}

/**
 * 工作台右侧超时区：默认折叠，摘要显示默认/自定义值；非法输入强制展开；forceMount 保草稿。
 * Workbench right-panel timeouts: collapsed by default with a default/custom summary; invalid input forces open; forceMount keeps drafts.
 */
function TimeoutSettingsSection({
  submitTimeoutMinutes,
  stopTimeoutMinutes,
  timeoutDefaults,
  onTaskTimeoutChange,
}: {
  submitTimeoutMinutes: string;
  stopTimeoutMinutes: string;
  timeoutDefaults?: SyncTaskTimeoutDefaults | null;
  onTaskTimeoutChange: (key: TaskTimeoutKey, minutes: number | null) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [open, setOpen] = useState(false);
  const [hydrated, setHydrated] = useState(false);
  const [submitInvalid, setSubmitInvalid] = useState(false);
  const [stopInvalid, setStopInvalid] = useState(false);
  const invalid = submitInvalid || stopInvalid;

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(TIMEOUT_OPEN_STORAGE_KEY);
      if (raw === '1' || raw === 'true') {
        setOpen(true);
      } else if (raw === '0' || raw === 'false') {
        setOpen(false);
      }
    } catch {
      // ignore storage errors
    }
    setHydrated(true);
  }, []);

  useEffect(() => {
    if (!hydrated || invalid) {
      return;
    }
    try {
      window.localStorage.setItem(TIMEOUT_OPEN_STORAGE_KEY, open ? '1' : '0');
    } catch {
      // ignore
    }
  }, [open, hydrated, invalid]);

  const forcedOpen = invalid || open;
  const submitDefault = defaultTimeoutMinutes(
    timeoutDefaults?.submit_timeout_seconds,
  );
  const stopDefault = defaultTimeoutMinutes(
    timeoutDefaults?.stop_timeout_seconds,
  );
  const submitParsed = parseTaskTimeoutInput(submitTimeoutMinutes);
  const stopParsed = parseTaskTimeoutInput(stopTimeoutMinutes);
  const submitCustom = submitParsed !== null && submitParsed !== undefined;
  const stopCustom = stopParsed !== null && stopParsed !== undefined;
  const custom = submitCustom || stopCustom;
  const submitLabel = submitCustom
    ? String(submitParsed)
    : String(submitDefault);
  const stopLabel = stopCustom ? String(stopParsed) : String(stopDefault);
  const summary = t('timeoutSummary', {
    submit: submitLabel,
    stop: stopLabel,
  });

  return (
    <CollapsiblePrimitive.Root
      open={forcedOpen}
      onOpenChange={(next) => {
        if (invalid) {
          setOpen(true);
          return;
        }
        setOpen(next);
      }}
      className='space-y-1.5'
    >
      <div className='flex min-w-0 items-center gap-1'>
        <CollapsiblePrimitive.Trigger
          type='button'
          data-testid='sync-timeout-section-trigger'
          className='flex min-w-0 flex-1 items-center gap-1 rounded-md px-0.5 py-0.5 text-left hover:bg-muted/40'
        >
          <ChevronRight
            className={cn(
              'size-3.5 shrink-0 text-muted-foreground transition-transform duration-200',
              forcedOpen && 'rotate-90',
            )}
          />
          <span className='min-w-0 flex-1 text-[11px] leading-4 text-muted-foreground'>
            {t('timeoutSettings')}
            {'：'}
            {custom
              ? summary
              : t('timeoutSummaryDefault', {
                  submit: submitLabel,
                  stop: stopLabel,
                })}
          </span>
          {custom ? (
            <Badge
              variant='secondary'
              className='h-4 shrink-0 px-1 text-[10px]'
            >
              {t('timeoutCustomBadge')}
            </Badge>
          ) : null}
        </CollapsiblePrimitive.Trigger>
        <HelpHint
          side='top'
          label={t('timeoutSettings')}
          content={t('timeoutSettingsHint')}
          data-testid='sync-timeout-settings-hint'
        />
      </div>
      <CollapsiblePrimitive.Content forceMount asChild>
        <div
          className={cn(
            'grid transition-[grid-template-rows] duration-200 ease-out',
            forcedOpen ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
          )}
        >
          <div className='overflow-hidden'>
            <div className='grid grid-cols-2 gap-2 pt-1'>
              <TaskTimeoutInput
                label={t('submitTimeoutSetting')}
                value={submitTimeoutMinutes}
                defaultSeconds={timeoutDefaults?.submit_timeout_seconds}
                testId='sync-submit-timeout-minutes'
                onChange={(minutes) =>
                  onTaskTimeoutChange('submit_timeout_minutes', minutes)
                }
                onInvalidChange={setSubmitInvalid}
              />
              <TaskTimeoutInput
                label={t('stopTimeoutSetting')}
                value={stopTimeoutMinutes}
                defaultSeconds={timeoutDefaults?.stop_timeout_seconds}
                testId='sync-stop-timeout-minutes'
                onChange={(minutes) =>
                  onTaskTimeoutChange('stop_timeout_minutes', minutes)
                }
                onInvalidChange={setStopInvalid}
              />
            </div>
          </div>
        </div>
      </CollapsiblePrimitive.Content>
    </CollapsiblePrimitive.Root>
  );
}

export function SettingsSidebarPanel({
  executionMode,
  observabilityMode = 'auto',
  onObservabilityModeChange,
  submitTimeoutMinutes = '',
  stopTimeoutMinutes = '',
  timeoutDefaults,
  onTaskTimeoutChange,
  alertTaskId = null,
  alertTaskName,
  alertCanEdit = false,
  alertUsers = [],
  clusterId,
  clusters,
  detectedVariables,
  customVariableRows,
  onExecutionModeChange,
  onClusterChange,
  onOpenCreateCustomVariable,
  onOpenEditCustomVariable,
  onDeleteCustomVariable,
  onCopyCustomVariableReference,
  onCopyCustomVariableValue,
  onOpenTimeVariables,
}: {
  executionMode: ExecutionMode;
  observabilityMode?: ObservabilityMode;
  onObservabilityModeChange?: (value: ObservabilityMode) => void;
  submitTimeoutMinutes?: string;
  stopTimeoutMinutes?: string;
  timeoutDefaults?: SyncTaskTimeoutDefaults | null;
  onTaskTimeoutChange?: (key: TaskTimeoutKey, minutes: number | null) => void;
  /** 已保存任务的 ID；为空时（新建未保存）不显示告警分组。/ Saved task ID; the alerts group is hidden when empty. */
  alertTaskId?: number | null;
  alertTaskName?: string;
  alertCanEdit?: boolean;
  alertUsers?: NotificationRecipientUser[];
  clusterId: string;
  clusters: ClusterInfo[];
  detectedVariables: string[];
  customVariableRows: VariableRow[];
  onExecutionModeChange: (value: ExecutionMode) => void;
  onClusterChange: (value: string) => void;
  onOpenCreateCustomVariable: () => void;
  onOpenEditCustomVariable: (item: VariableRow) => void;
  onDeleteCustomVariable: (id: string) => void;
  onCopyCustomVariableReference: (key: string) => void;
  onCopyCustomVariableValue: (value: string) => void;
  onOpenTimeVariables?: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  const tCluster = useTranslations('cluster');
  const builtinPreviewNow = useMemo(() => new Date(), []);
  return (
    <div className='flex h-full min-h-0 w-full min-w-0 flex-col'>
      {/* 上段：任务高级属性，贴着编辑器。/ Top: task properties beside the editor. */}
      <section
        aria-label={t('advancedPropertiesTask')}
        className='flex min-h-0 flex-1 flex-col'
      >
        <div className='flex shrink-0 items-center border-b border-border/60 px-3 py-2'>
          <h2 className='text-xs font-medium text-foreground'>
            {t('advancedPropertiesTask')}
          </h2>
        </div>
        <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-3 py-3'>
          <div className='space-y-3'>
            <div className='space-y-1.5'>
              <Label className='text-[11px] text-muted-foreground'>
                {t('executionMode')}
              </Label>
              <SegmentedControl
                className='w-full'
                label={t('executionMode')}
                value={executionMode}
                onValueChange={(value) =>
                  onExecutionModeChange(value as ExecutionMode)
                }
                options={[
                  {value: 'cluster', label: t('clusterMode')},
                  {value: 'local', label: t('localMode')},
                ]}
              />
            </div>

            {executionMode === 'cluster' ? (
              <Combobox
                label={t('zetaCluster')}
                placeholder={t('selectCluster')}
                emptyMessage={t('unselected')}
                value={clusterId}
                onValueChange={onClusterChange}
                options={clusters.map((cluster) => ({
                  value: String(cluster.id),
                  label: isSoleDefaultCluster(clusters, cluster.id)
                    ? `${cluster.name} · ${tCluster('defaultBadge')}`
                    : cluster.name,
                }))}
              />
            ) : null}
          </div>

          {/* 任务变量只保留一个标题。自定义变量、检测到的引用、时间变量都落在这一块里。 */}
          {/* One Task variables heading. Custom rows, detected refs, and time variables live in this block. */}
          <div className='space-y-2.5'>
            <div className='flex items-center justify-between gap-2'>
              <div className='flex min-w-0 items-center gap-1.5'>
                <span className='text-xs font-medium text-foreground'>
                  {t('taskVariables')}
                </span>
                <HelpHint
                  side='top'
                  label={t('taskVariables')}
                  content={t('taskVariablesHint')}
                  data-testid='sync-task-variables-hint'
                />
              </div>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='h-9 gap-1 rounded-[var(--radius-control)] px-2.5 text-xs'
                onClick={onOpenCreateCustomVariable}
              >
                <Plus className='size-3.5' />
                {t('add')}
              </Button>
            </div>

            <CustomVariablesSection
              embedded
              variables={customVariableRows}
              onOpenCreate={onOpenCreateCustomVariable}
              onOpenEdit={onOpenEditCustomVariable}
              onDelete={onDeleteCustomVariable}
              onCopyReference={onCopyCustomVariableReference}
              onCopyValue={onCopyCustomVariableValue}
            />

            {detectedVariables.length > 0 ? (
              <div
                className='flex flex-wrap gap-1.5'
                aria-label={t('detectedVariables')}
              >
                {detectedVariables.map((variable) => (
                  <Tooltip key={variable}>
                    <TooltipTrigger asChild>
                      <Badge
                        variant='outline'
                        className='cursor-help bg-background/80 px-1.5 py-0 font-mono text-[10px]'
                      >
                        {`{{${variable}}}`}
                      </Badge>
                    </TooltipTrigger>
                    <TooltipContent className='max-w-[320px] break-all text-xs'>
                      <div>{`{{${variable}}}`}</div>
                      {resolveBuiltinPreviewExpression(
                        variable,
                        builtinPreviewNow,
                      ) ? (
                        <div className='mt-1 text-muted-foreground'>
                          {t('builtinPreviewResult', {
                            value:
                              resolveBuiltinPreviewExpression(
                                variable,
                                builtinPreviewNow,
                              ) || '-',
                          })}
                        </div>
                      ) : null}
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            ) : null}

            {onOpenTimeVariables ? (
              <button
                type='button'
                className='inline-flex items-center gap-0.5 text-[11px] text-muted-foreground hover:text-foreground'
                onClick={onOpenTimeVariables}
              >
                <span>{t('builtinTimeVariables')}</span>
                <ChevronRight className='size-3' />
              </button>
            ) : null}
          </div>
        </div>
      </section>

      {/* 下段：运行高级属性，贴着底部控制台。/ Bottom: runtime properties beside the console. */}
      <section
        aria-label={t('advancedPropertiesRuntime')}
        className='flex min-h-0 flex-1 flex-col border-t border-border/70'
      >
        <div className='flex shrink-0 items-center border-b border-border/60 px-3 py-2'>
          <h2 className='text-xs font-medium text-foreground'>
            {t('advancedPropertiesRuntime')}
          </h2>
        </div>
        <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-3 py-3'>
          {/* 实时可观测性：仅集群模式有意义，下次提交生效 */}
          {/* Realtime observability: meaningful in cluster mode only, applies on the next submit */}
          {executionMode === 'cluster' && onObservabilityModeChange ? (
            <div className='space-y-1.5'>
              <div className='flex items-center gap-2'>
                <Label className='text-[11px] text-muted-foreground'>
                  {t('observabilitySetting')}
                </Label>
                <HelpHint
                  side='top'
                  label={t('observabilitySetting')}
                  content={t('observabilityHint')}
                  data-testid='sync-observability-hint'
                />
              </div>
              <div data-testid='sync-observability-mode'>
                <SegmentedControl
                  className='w-full'
                  label={t('observabilitySetting')}
                  value={observabilityMode}
                  onValueChange={(value) =>
                    onObservabilityModeChange(value as ObservabilityMode)
                  }
                  options={[
                    {value: 'auto', label: t('observabilityAuto')},
                    {value: 'on', label: t('observabilityOn')},
                    {value: 'off', label: t('observabilityOff')},
                  ]}
                />
              </div>
            </div>
          ) : null}

          {/* 提交/停止超时：默认折叠；留空用全局默认，跟随任务版本，重新发布后生效 */}
          {/* Submit/stop timeouts: collapsed by default; empty means the global default; versioned with the task and applied after republishing */}
          {executionMode === 'cluster' && onTaskTimeoutChange ? (
            <TimeoutSettingsSection
              submitTimeoutMinutes={submitTimeoutMinutes}
              stopTimeoutMinutes={stopTimeoutMinutes}
              timeoutDefaults={timeoutDefaults}
              onTaskTimeoutChange={onTaskTimeoutChange}
            />
          ) : null}
          {/* 作业告警：不跟随任务版本，保存即生效 / Job alerts: not versioned with the task, effective on save */}
          {alertTaskId ? (
            <TaskAlertSection
              taskId={alertTaskId}
              taskName={alertTaskName}
              clusterId={Number(clusterId) || null}
              canEdit={alertCanEdit}
              users={alertUsers}
            />
          ) : null}
        </div>
      </section>
    </div>
  );
}
