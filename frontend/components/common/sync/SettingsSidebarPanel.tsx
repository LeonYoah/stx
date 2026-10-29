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
import {isSoleDefaultCluster} from '@/lib/cluster-preference';
import {
  Check,
  ChevronDown,
  ChevronRight,
  Clock3,
  Code2,
  Cpu,
  HelpCircle,
  Layers,
  Loader2,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {
  SyncCuratedTemplateView,
  SyncPluginType,
  SyncTaskTimeoutDefaults,
} from '@/lib/services/sync';
import {CustomVariablesSection} from './CustomVariablesSection';
import {TaskAlertSection} from './TaskAlertSection';
import {CuratedTemplatesPanel} from './CuratedTemplatesPanel';
import {
  resolveBuiltinPreviewExpression,
} from './builtin-time-variables';
import {parseTaskTimeoutInput} from './sync-studio-utils';
import type {
  ExecutionMode,
  ObservabilityMode,
  TaskTimeoutKey,
  TemplatePluginItem,
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
}: {
  label: string;
  value: string;
  defaultSeconds?: number;
  testId: string;
  onChange: (minutes: number | null) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [draft, setDraft] = useState(value);
  useEffect(() => {
    setDraft(value);
  }, [value]);
  const parsed = parseTaskTimeoutInput(draft);
  const invalid = parsed === undefined;
  const defaultMinutes = defaultSeconds && defaultSeconds > 0 ? Math.max(1, Math.round(defaultSeconds / 60)) : 5;
  return (
    <div className='min-w-0 space-y-1'>
      <Label className='whitespace-nowrap text-[11px] text-muted-foreground'>{label}</Label>
      <Input
        value={draft}
        inputMode='numeric'
        data-testid={testId}
        aria-invalid={invalid}
        placeholder={t('timeoutDefaultPlaceholder', {minutes: defaultMinutes})}
        className={invalid ? 'h-8 text-xs border-destructive focus-visible:ring-destructive/40' : 'h-8 text-xs'}
        onChange={(event) => {
          const next = event.target.value;
          setDraft(next);
          const minutes = parseTaskTimeoutInput(next);
          if (minutes !== undefined) {
            onChange(minutes);
          }
        }}
      />
      {invalid ? <p className='text-[10px] text-destructive'>{t('timeoutInvalid')}</p> : null}
    </div>
  );
}

export function TemplatePluginSelect({
  label,
  placeholder,
  items,
  disabled,
  loading,
  loadingText,
  onSelect,
}: {
  label: string;
  placeholder: string;
  items: TemplatePluginItem[];
  disabled?: boolean;
  loading?: boolean;
  loadingText?: string | null;
  onSelect: (value: string) => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [open, setOpen] = useState(false);
  const selectedLabel = loading ? loadingText || placeholder : placeholder;

  return (
    <div className='space-y-1.5'>
      <Label className='text-[11px] text-muted-foreground'>{label}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            disabled={disabled}
            className='w-full justify-between'
          >
            <span className='truncate text-left'>{selectedLabel}</span>
            {loading ? (
              <Loader2 className='ml-2 size-4 shrink-0 animate-spin opacity-70' />
            ) : (
              <ChevronDown className='ml-2 size-4 shrink-0 opacity-50' />
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className='w-[var(--radix-popover-trigger-width)] min-w-0 p-0'>
          <Command>
            <CommandInput placeholder={`${placeholder}...`} />
            <CommandList>
              <CommandEmpty>{t('noMatchingPlugins')}</CommandEmpty>
              {items.map((item) => (
                <CommandItem
                  key={`${label}:${item.value}`}
                  value={`${item.label} ${item.value}`}
                  onSelect={() => {
                    setOpen(false);
                    onSelect(item.value);
                  }}
                >
                  <Check className='size-4 opacity-0' />
                  <span className='truncate'>{item.label}</span>
                </CommandItem>
              ))}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
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
  pluginPanelLoading,
  pluginTemplatePendingType,
  pluginTemplateLoadingText,
  sourceTemplateItems,
  transformTemplateItems,
  sinkTemplateItems,
  detectedVariables,
  customVariableRows,
  onExecutionModeChange,
  onClusterChange,
  onInsertPluginTemplate,
  onInsertCuratedTemplate,
  curatedRefreshToken = 0,
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
  pluginPanelLoading: boolean;
  pluginTemplatePendingType: SyncPluginType | null;
  pluginTemplateLoadingText: string | null;
  sourceTemplateItems: TemplatePluginItem[];
  transformTemplateItems: TemplatePluginItem[];
  sinkTemplateItems: TemplatePluginItem[];
  detectedVariables: string[];
  customVariableRows: VariableRow[];
  onExecutionModeChange: (value: ExecutionMode) => void;
  onClusterChange: (value: string) => void;
  onInsertPluginTemplate: (
    pluginType: SyncPluginType,
    factoryIdentifier: string,
  ) => void;
  onInsertCuratedTemplate: (item: SyncCuratedTemplateView) => void;
  curatedRefreshToken?: number;
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
    <div className='min-w-0 w-full space-y-3.5'>
      {/* 运行与集群环境配置（置顶） */}
      {/* Execution mode and cluster environment settings (top) */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3 space-y-2.5'>
        <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
          <Cpu className='size-3.5 text-primary' />
          <span>{t('executionAndCluster')}</span>
        </div>
        <div className='space-y-1.5'>
          <Label className='text-[11px] text-muted-foreground'>{t('executionMode')}</Label>
          <Select
            value={executionMode}
            onValueChange={(value) =>
              onExecutionModeChange(value as ExecutionMode)
            }
          >
            <SelectTrigger className='h-8 w-full text-xs'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className='w-[var(--radix-select-trigger-width)] min-w-0'>
              <SelectItem value='cluster'>{t('clusterMode')}</SelectItem>
              <SelectItem value='local'>{t('localMode')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {executionMode === 'cluster' ? (
          <div className='space-y-1.5 pt-2 border-t border-border/40'>
            <Label className='text-[11px] text-muted-foreground'>{t('zetaCluster')}</Label>
            <Select
              value={clusterId || '__empty__'}
              onValueChange={onClusterChange}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue placeholder={t('selectCluster')} />
              </SelectTrigger>
              <SelectContent className='w-[var(--radix-select-trigger-width)] min-w-0'>
                <SelectItem value='__empty__'>{t('unselected')}</SelectItem>
                {clusters.map((cluster) => (
                  <SelectItem key={cluster.id} value={String(cluster.id)}>
                    {cluster.name}
                    {isSoleDefaultCluster(clusters, cluster.id)
                      ? ` · ${tCluster('defaultBadge')}`
                      : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}

        {/* 实时可观测性：仅集群模式有意义，下次提交生效 */}
        {/* Realtime observability: meaningful in cluster mode only, applies on the next submit */}
        {executionMode === 'cluster' && onObservabilityModeChange ? (
          <div className='space-y-1.5 pt-2 border-t border-border/40'>
            <div className='flex items-center gap-1'>
              <Label className='text-[11px] text-muted-foreground'>
                {t('observabilitySetting')}
              </Label>
              <Tooltip>
                <TooltipTrigger asChild>
                  <HelpCircle className='size-3 cursor-help text-muted-foreground/70' />
                </TooltipTrigger>
                <TooltipContent side='top' className='max-w-[260px] text-xs'>
                  {t('observabilityHint')}
                </TooltipContent>
              </Tooltip>
            </div>
            <Select
              value={observabilityMode}
              onValueChange={(value) =>
                onObservabilityModeChange(value as ObservabilityMode)
              }
            >
              <SelectTrigger className='h-8 w-full text-xs' data-testid='sync-observability-mode'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent className='w-[var(--radix-select-trigger-width)] min-w-0'>
                <SelectItem value='auto'>{t('observabilityAuto')}</SelectItem>
                <SelectItem value='on'>{t('observabilityOn')}</SelectItem>
                <SelectItem value='off'>{t('observabilityOff')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        ) : null}

        {/* 提交/停止超时：留空用全局默认，跟随任务版本，重新发布后生效 */}
        {/* Submit/stop timeouts: empty means the global default; versioned with the task and applied after republishing */}
        {executionMode === 'cluster' && onTaskTimeoutChange ? (
          <div className='space-y-1.5 pt-2 border-t border-border/40'>
            <div className='flex items-center gap-1'>
              <Label className='text-[11px] text-muted-foreground'>{t('timeoutSettings')}</Label>
              <Tooltip>
                <TooltipTrigger asChild>
                  <HelpCircle className='size-3 cursor-help text-muted-foreground/70' />
                </TooltipTrigger>
                <TooltipContent side='top' className='max-w-[260px] text-xs'>
                  {t('timeoutSettingsHint')}
                </TooltipContent>
              </Tooltip>
            </div>
            <div className='grid grid-cols-2 gap-2'>
              <TaskTimeoutInput
                label={t('submitTimeoutSetting')}
                value={submitTimeoutMinutes}
                defaultSeconds={timeoutDefaults?.submit_timeout_seconds}
                testId='sync-submit-timeout-minutes'
                onChange={(minutes) => onTaskTimeoutChange('submit_timeout_minutes', minutes)}
              />
              <TaskTimeoutInput
                label={t('stopTimeoutSetting')}
                value={stopTimeoutMinutes}
                defaultSeconds={timeoutDefaults?.stop_timeout_seconds}
                testId='sync-stop-timeout-minutes'
                onChange={(minutes) => onTaskTimeoutChange('stop_timeout_minutes', minutes)}
              />
            </div>
          </div>
        ) : null}
        {/* 作业告警：不跟随任务版本，保存即生效 / Job alerts: not versioned with the task, effective on save */}
        {alertTaskId ? (
          <div className='space-y-1.5 border-t border-border/40 pt-2.5'>
            <TaskAlertSection
              taskId={alertTaskId}
              taskName={alertTaskName}
              clusterId={Number(clusterId) || null}
              canEdit={alertCanEdit}
              users={alertUsers}
            />
          </div>
        ) : null}
      </div>

      {/* 模板：精选列表 + 预览，再是原始默认参数 */}
      {/* Templates: curated list + preview, then raw defaults */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3 space-y-2.5'>
        <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
          <Layers className='size-3.5 text-primary' />
          <span>{t('templates')}</span>
        </div>
        <Tabs defaultValue='curated' className='w-full'>
          <TabsList className='grid h-8 w-full grid-cols-2'>
            <TabsTrigger value='curated' className='text-[11px]'>
              {t('curatedTemplates')}
            </TabsTrigger>
            <TabsTrigger value='raw' className='text-[11px]'>
              {t('rawDefaultParamTemplates')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='curated' className='mt-2.5'>
            <CuratedTemplatesPanel
              onInsert={onInsertCuratedTemplate}
              clusterId={clusterId}
              refreshToken={curatedRefreshToken}
            />
          </TabsContent>
          <TabsContent value='raw' className='mt-2.5 space-y-2.5'>
            {executionMode === 'cluster' ? (
              <>
                <TemplatePluginSelect
                  disabled={!clusterId || pluginPanelLoading}
                  items={sourceTemplateItems}
                  label={t('sourceTemplate')}
                  loading={pluginTemplatePendingType === 'source'}
                  loadingText={
                    pluginTemplatePendingType === 'source'
                      ? pluginTemplateLoadingText
                      : null
                  }
                  placeholder={t('selectSourcePlugin')}
                  onSelect={(value) => onInsertPluginTemplate('source', value)}
                />
                <TemplatePluginSelect
                  disabled={!clusterId || pluginPanelLoading}
                  items={transformTemplateItems}
                  label={t('transformTemplate')}
                  loading={pluginTemplatePendingType === 'transform'}
                  loadingText={
                    pluginTemplatePendingType === 'transform'
                      ? pluginTemplateLoadingText
                      : null
                  }
                  placeholder={t('selectTransformPlugin')}
                  onSelect={(value) =>
                    onInsertPluginTemplate('transform', value)
                  }
                />
                <TemplatePluginSelect
                  disabled={!clusterId || pluginPanelLoading}
                  items={sinkTemplateItems}
                  label={t('sinkTemplate')}
                  loading={pluginTemplatePendingType === 'sink'}
                  loadingText={
                    pluginTemplatePendingType === 'sink'
                      ? pluginTemplateLoadingText
                      : null
                  }
                  placeholder={t('selectSinkPlugin')}
                  onSelect={(value) => onInsertPluginTemplate('sink', value)}
                />
                <p className='text-[11px] leading-5 text-muted-foreground'>
                  {!clusterId
                    ? t('selectClusterFirst')
                    : pluginPanelLoading
                      ? t('loadingPluginTemplates')
                      : pluginTemplateLoadingText
                        ? t('generatingPluginTemplate', {
                            plugin: pluginTemplateLoadingText,
                          })
                        : t('rawDefaultParamHint')}
                </p>
              </>
            ) : (
              <p className='text-[11px] leading-5 text-muted-foreground'>
                {t('rawDefaultNeedsCluster')}
              </p>
            )}
          </TabsContent>
        </Tabs>
      </div>

      {/* 任务变量管理（自定义、模板提取与内置时间变量） */}
      {/* Task variables management (custom, detected, and builtin time variables) */}
      <div className='rounded-lg border border-border/50 bg-muted/10 p-3 space-y-3'>
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
            <Code2 className='size-3.5 text-primary' />
            <span>{t('taskVariables')}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <HelpCircle className='size-3.5 text-muted-foreground/70 hover:text-foreground cursor-help transition-colors' />
              </TooltipTrigger>
              <TooltipContent
                side='top'
                className='max-w-[280px] text-xs leading-relaxed'
              >
                {t('taskVariablesHint')}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* 自定义变量列表 */}
        {/* Custom variables */}
        <CustomVariablesSection
          variables={customVariableRows}
          onOpenCreate={onOpenCreateCustomVariable}
          onOpenEdit={onOpenEditCustomVariable}
          onDelete={onDeleteCustomVariable}
          onCopyReference={onCopyCustomVariableReference}
          onCopyValue={onCopyCustomVariableValue}
        />

        {/* 模板提取的占位符变量 */}
        {/* Detected placeholder variables in template */}
        <div className='space-y-1.5 pt-2 border-t border-border/40'>
          <Label className='block text-[11px] text-muted-foreground font-medium'>
            {t('detectedVariables')}
          </Label>
          <div className='flex flex-wrap gap-1.5'>
            {detectedVariables.length > 0 ? (
              detectedVariables.map((variable) => (
                <Tooltip key={variable}>
                  <TooltipTrigger asChild>
                    <Badge
                      variant='outline'
                      className='font-mono text-[10px] px-1.5 py-0 cursor-help bg-background/80'
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
              ))
            ) : (
              <span className='text-xs text-muted-foreground'>
                {t('noDetectedVariables')}
              </span>
            )}
          </div>
        </div>

        {/* 内置时间系统变量快捷入口 */}
        {/* Built-in time system variables quick entry */}
        {onOpenTimeVariables ? (
          <div className='flex items-center justify-between pt-2 border-t border-border/40 text-[11px] text-muted-foreground'>
            <div className='flex items-center gap-1.5 font-medium'>
              <Clock3 className='size-3 text-sky-500' />
              <span>{t('builtinTimeVariables')}</span>
            </div>
            <Button
              type='button'
              size='sm'
              variant='ghost'
              className='h-5 px-1.5 text-[10px] text-primary hover:text-primary/80 gap-0.5'
              onClick={onOpenTimeVariables}
            >
              <span>{t('viewDetails')}</span>
              <ChevronRight className='size-3' />
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

