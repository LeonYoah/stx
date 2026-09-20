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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Label} from '@/components/ui/label';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Checkbox} from '@/components/ui/checkbox';
import {Badge} from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Settings2,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  Cpu,
  Clock,
} from 'lucide-react';
import {cn} from '@/lib/utils';
import type {
  RuntimeEngineConfig,
  SeaTunnelVersionCapabilities,
  JobScheduleStrategy,
  SlotAllocationStrategy,
  JobLogMode,
} from '@/lib/services/installer/types';

interface RuntimeAdvancedConfigCardProps {
  version?: string;
  capabilities?: SeaTunnelVersionCapabilities | null;
  runtime: RuntimeEngineConfig;
  onChange: (updates: Partial<RuntimeEngineConfig>) => void;
  /**
   * 紧凑密度（向导内使用）/ Compact density for wizard surfaces
   */
  compact?: boolean;
  /**
   * 默认折叠，渐进式披露高级项 / Start collapsed for progressive disclosure
   */
  defaultCollapsed?: boolean;
}

/** 字段旁帮助气泡 / Inline help tooltip beside a field label */
function FieldHint({text}: {text: string}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type='button'
          className='text-muted-foreground/70 hover:text-foreground'
          aria-label={text}
        >
          <CircleHelp className='h-3.5 w-3.5' />
        </button>
      </TooltipTrigger>
      <TooltipContent className='max-w-xs text-xs leading-relaxed'>
        {text}
      </TooltipContent>
    </Tooltip>
  );
}

export function RuntimeAdvancedConfigCard({
  version,
  capabilities,
  runtime,
  onChange,
  compact = false,
  defaultCollapsed = false,
}: RuntimeAdvancedConfigCardProps) {
  const t = useTranslations();
  const [expanded, setExpanded] = useState(!defaultCollapsed);

  const showSlotSettings =
    capabilities?.supports_dynamic_slot || capabilities?.supports_slot_num;
  const showHistorySettings =
    capabilities?.supports_history_job_expire_minutes ||
    capabilities?.supports_scheduled_deletion_enable;
  const showJobLogSettings = capabilities?.supports_job_log_mode;

  const slotModeLabel = runtime.dynamic_slot
    ? t('installer.runtimeAdvanced.dynamicSlot')
    : t('installer.runtimeAdvanced.staticSlot');

  const summaryParts: string[] = [];
  if (showSlotSettings) {
    summaryParts.push(slotModeLabel);
  }
  if (capabilities?.supports_history_job_expire_minutes) {
    summaryParts.push(`${runtime.history_job_expire_minutes}m`);
  }
  if (showJobLogSettings) {
    summaryParts.push(
      runtime.job_log_mode === 'per_job'
        ? t('installer.jobLogMode.perJob')
        : t('installer.jobLogMode.mixed'),
    );
  }

  return (
    <Card className={cn(compact && 'shadow-none border-border/70')}>
      <CardHeader
        className={cn(
          'cursor-pointer select-none',
          compact
            ? 'py-2.5 px-3.5 border-b bg-muted/15'
            : 'pb-3',
        )}
        onClick={() => setExpanded((prev) => !prev)}
      >
        <div className='flex items-center justify-between gap-2'>
          <CardTitle
            className={cn(
              'flex items-center gap-2',
              compact ? 'text-xs font-semibold' : 'text-base',
            )}
          >
            {expanded ? (
              <ChevronDown className={cn(compact ? 'h-3.5 w-3.5' : 'h-4 w-4')} />
            ) : (
              <ChevronRight className={cn(compact ? 'h-3.5 w-3.5' : 'h-4 w-4')} />
            )}
            <Settings2 className={cn(compact ? 'h-3.5 w-3.5' : 'h-4 w-4')} />
            {t('installer.runtimeAdvanced.title')}
          </CardTitle>
          {!expanded && summaryParts.length > 0 && (
            <div className='flex flex-wrap items-center justify-end gap-1'>
              {summaryParts.map((part) => (
                <Badge
                  key={part}
                  variant='outline'
                  className='text-[10px] py-0 h-4 font-normal max-w-[9rem] truncate'
                >
                  {part}
                </Badge>
              ))}
            </div>
          )}
        </div>
      </CardHeader>

      {expanded && (
        <CardContent
          className={cn(compact ? 'p-3.5 space-y-3' : 'p-5 space-y-4')}
        >
          {!version ? (
            <p className='text-xs text-muted-foreground'>
              {t('installer.runtimeAdvanced.selectVersionHint')}
            </p>
          ) : (
            <div className='grid grid-cols-1 md:grid-cols-2 gap-4 divide-y md:divide-y-0 md:divide-x divide-border/40'>
              {/* 左列：Slot 调度体系 / Left Column: Slot Scheduling System */}
              <div className='space-y-3 md:pr-4'>
                <div className='flex items-center gap-1.5 pb-1.5 border-b border-border/40 text-xs font-semibold text-foreground/90'>
                  <Cpu className='h-3.5 w-3.5 text-primary' />
                  <span>{t('installer.runtimeAdvanced.slotSectionTitle')}</span>
                </div>

                {showSlotSettings ? (
                  <div className='space-y-3'>
                    {/* Slot 模式选择 / Slot mode select */}
                    <div className='space-y-1'>
                      <div className='flex items-center gap-1.5'>
                        <Label className='text-xs font-medium'>
                          {t('installer.runtimeAdvanced.slotMode')}
                        </Label>
                        <FieldHint
                          text={t('installer.runtimeAdvanced.dynamicSlotHint')}
                        />
                      </div>
                      <Select
                        value={runtime.dynamic_slot ? 'dynamic' : 'static'}
                        onValueChange={(value) =>
                          onChange({dynamic_slot: value === 'dynamic'})
                        }
                      >
                        <SelectTrigger className={cn(compact && 'h-8 text-xs')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value='dynamic'>
                            {t('installer.runtimeAdvanced.dynamicSlot')}
                          </SelectItem>
                          <SelectItem value='static'>
                            {t('installer.runtimeAdvanced.staticSlot')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    {/* Slot 分配策略 / Slot allocation strategy */}
                    {capabilities?.supports_slot_allocation_strategy ? (
                      <div className='space-y-1'>
                        <div className='flex items-center gap-1.5'>
                          <Label className='text-xs font-medium'>
                            {t(
                              'installer.runtimeAdvanced.slotAllocationStrategy',
                            )}
                          </Label>
                          <FieldHint
                            text={
                              runtime.slot_allocation_strategy === 'SYSTEM_LOAD'
                                ? t(
                                    'installer.runtimeAdvanced.slotAllocationSystemLoadDesc',
                                  )
                                : runtime.slot_allocation_strategy ===
                                    'SLOT_RATIO'
                                  ? t(
                                      'installer.runtimeAdvanced.slotAllocationSlotRatioDesc',
                                    )
                                  : t(
                                      'installer.runtimeAdvanced.slotAllocationRandomDesc',
                                    )
                            }
                          />
                        </div>
                        <Select
                          value={runtime.slot_allocation_strategy}
                          onValueChange={(value: SlotAllocationStrategy) =>
                            onChange({slot_allocation_strategy: value})
                          }
                        >
                          <SelectTrigger className={cn(compact && 'h-8 text-xs')}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='RANDOM'>
                              {t(
                                'installer.runtimeAdvanced.slotAllocationRandom',
                              )}
                            </SelectItem>
                            <SelectItem value='SYSTEM_LOAD'>
                              {t(
                                'installer.runtimeAdvanced.slotAllocationSystemLoad',
                              )}
                            </SelectItem>
                            <SelectItem value='SLOT_RATIO'>
                              {t(
                                'installer.runtimeAdvanced.slotAllocationSlotRatio',
                              )}
                            </SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    ) : null}

                    {/* 静态 Slot 额外参数 / Static slot parameters */}
                    {!runtime.dynamic_slot && (
                      <div className='grid grid-cols-2 gap-2.5 pt-0.5'>
                        {capabilities?.supports_slot_num && (
                          <div className='space-y-1'>
                            <div className='flex items-center gap-1.5'>
                              <Label className='text-xs font-medium'>
                                {t('installer.runtimeAdvanced.slotNum')}
                              </Label>
                              <FieldHint
                                text={t('installer.runtimeAdvanced.slotNumHint')}
                              />
                            </div>
                            <Input
                              type='number'
                              value={runtime.slot_num}
                              onChange={(event) =>
                                onChange({
                                  slot_num: Math.max(
                                    1,
                                    Number.parseInt(event.target.value, 10) || 1,
                                  ),
                                })
                              }
                              min={1}
                              step={1}
                              className={cn(compact && 'h-8 text-xs font-mono')}
                            />
                          </div>
                        )}

                        {capabilities?.supports_job_schedule_strategy ? (
                          <div className='space-y-1'>
                            <div className='flex items-center gap-1.5'>
                              <Label className='text-xs font-medium'>
                                {t(
                                  'installer.runtimeAdvanced.jobScheduleStrategy',
                                )}
                              </Label>
                              <FieldHint
                                text={t(
                                  'installer.runtimeAdvanced.jobScheduleHint',
                                )}
                              />
                            </div>
                            <Select
                              value={runtime.job_schedule_strategy}
                              onValueChange={(value: JobScheduleStrategy) =>
                                onChange({job_schedule_strategy: value})
                              }
                            >
                              <SelectTrigger
                                className={cn(compact && 'h-8 text-xs')}
                              >
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value='REJECT'>
                                  {t(
                                    'installer.runtimeAdvanced.jobScheduleReject',
                                  )}
                                </SelectItem>
                                <SelectItem value='WAIT'>
                                  {t(
                                    'installer.runtimeAdvanced.jobScheduleWait',
                                  )}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                        ) : null}
                      </div>
                    )}
                  </div>
                ) : null}
              </div>

              {/* 右列：作业生命周期与日志 / Right Column: Job Lifecycle & Log Mode */}
              <div className='space-y-3 pt-3 md:pt-0 md:pl-4'>
                <div className='flex items-center gap-1.5 pb-1.5 border-b border-border/40 text-xs font-semibold text-foreground/90'>
                  <Clock className='h-3.5 w-3.5 text-primary' />
                  <span>
                    {t('installer.runtimeAdvanced.historySectionTitle')}
                  </span>
                </div>

                {/* 历史作业保留时间 / History job expire minutes */}
                {showHistorySettings && capabilities?.supports_history_job_expire_minutes ? (
                  <div className='space-y-1'>
                    <div className='flex items-center gap-1.5'>
                      <Label className='text-xs font-medium'>
                        {t(
                          'installer.runtimeAdvanced.historyJobExpireMinutes',
                        )}
                      </Label>
                      <FieldHint
                        text={t(
                          'installer.runtimeAdvanced.historyJobExpireHint',
                        )}
                      />
                    </div>
                    <div className='relative'>
                      <Input
                        type='number'
                        value={runtime.history_job_expire_minutes}
                        onChange={(event) =>
                          onChange({
                            history_job_expire_minutes: Math.max(
                              1,
                              Number.parseInt(event.target.value, 10) || 1,
                            ),
                          })
                        }
                        min={1}
                        step={1}
                        className={cn(compact && 'h-8 text-xs font-mono pr-12')}
                      />
                      <span className='absolute right-2.5 top-2 text-[10px] text-muted-foreground pointer-events-none'>
                        min
                      </span>
                    </div>
                  </div>
                ) : null}

                {/* 日志输出模式 / Log output mode */}
                {showJobLogSettings ? (
                  <div className='space-y-1'>
                    <div className='flex items-center gap-1.5'>
                      <Label className='text-xs font-medium'>
                        {t('installer.jobLogMode.mode')}
                      </Label>
                      <FieldHint
                        text={
                          runtime.job_log_mode === 'per_job'
                            ? t('installer.jobLogMode.perJobHint')
                            : t('installer.jobLogMode.mixedHint')
                        }
                      />
                    </div>
                    <Select
                      value={runtime.job_log_mode}
                      onValueChange={(value: JobLogMode) =>
                        onChange({job_log_mode: value})
                      }
                    >
                      <SelectTrigger
                        data-testid='install-runtime-log-mode'
                        className={cn(compact && 'h-8 text-xs')}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='mixed'>
                          {t('installer.jobLogMode.mixed')}
                        </SelectItem>
                        <SelectItem value='per_job'>
                          {t('installer.jobLogMode.perJob')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                ) : null}

                {/* 自动删除过期作业日志 / Scheduled log deletion */}
                {capabilities?.supports_scheduled_deletion_enable ? (
                  <div className='pt-1'>
                    <label className='flex items-center gap-2 text-xs cursor-pointer select-none'>
                      <Checkbox
                        checked={runtime.scheduled_deletion_enable}
                        onCheckedChange={(checked) =>
                          onChange({
                            scheduled_deletion_enable: checked === true,
                          })
                        }
                        className='h-3.5 w-3.5'
                      />
                      <span className='text-muted-foreground hover:text-foreground transition-colors'>
                        {t(
                          'installer.runtimeAdvanced.scheduledDeletionEnable',
                        )}
                      </span>
                      <FieldHint
                        text={t(
                          'installer.runtimeAdvanced.scheduledDeletionHint',
                        )}
                      />
                    </label>
                  </div>
                ) : null}
              </div>
            </div>
          )}
        </CardContent>
      )}
    </Card>
  );
}
