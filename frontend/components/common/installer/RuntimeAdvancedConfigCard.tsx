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
import {Input} from '@/components/arc/input/input';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {NeutralChip} from '@/components/common/layout';
import {HelpHint} from '@/components/common/HelpHint';
import {
  Settings2,
  ChevronDown,
  ChevronRight,
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

  const body = !version ? (
    <p className='text-xs text-muted-foreground'>
      {t('installer.runtimeAdvanced.selectVersionHint')}
    </p>
  ) : (
    <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
      {/* 左列：Slot 调度体系 / Left Column: Slot Scheduling System */}
      <div className='space-y-3'>
        <div className='flex items-center gap-1.5 pb-1.5 border-b-[0.5px] border-border/80 text-xs font-semibold text-foreground/90'>
          <Cpu className='h-3.5 w-3.5 text-primary' />
          <span>{t('installer.runtimeAdvanced.slotSectionTitle')}</span>
        </div>

        {showSlotSettings ? (
          <div className='space-y-3'>
            {/* Slot 模式选择 / Slot mode segmented control */}
            <div className='space-y-1.5'>
              <div className='flex items-center gap-1.5'>
                <span className='text-xs font-medium'>
                  {t('installer.runtimeAdvanced.slotMode')}
                </span>
                <HelpHint
                  label={t('installer.runtimeAdvanced.slotMode')}
                  content={t('installer.runtimeAdvanced.dynamicSlotHint')}
                />
              </div>
              <SegmentedControl
                label={t('installer.runtimeAdvanced.slotMode')}
                value={runtime.dynamic_slot ? 'dynamic' : 'static'}
                onValueChange={(value) =>
                  onChange({dynamic_slot: value === 'dynamic'})
                }
                options={[
                  {
                    value: 'dynamic',
                    label: t('installer.runtimeAdvanced.dynamicSlot'),
                  },
                  {
                    value: 'static',
                    label: t('installer.runtimeAdvanced.staticSlot'),
                  },
                ]}
              />
            </div>

            {/* Slot 分配策略 / Slot allocation strategy */}
            {capabilities?.supports_slot_allocation_strategy ? (
              <div className='space-y-1.5'>
                <div className='flex items-center gap-1.5'>
                  <span className='text-xs font-medium'>
                    {t('installer.runtimeAdvanced.slotAllocationStrategy')}
                  </span>
                  <HelpHint
                    label={t(
                      'installer.runtimeAdvanced.slotAllocationStrategy',
                    )}
                    content={
                      runtime.slot_allocation_strategy === 'SYSTEM_LOAD'
                        ? t(
                            'installer.runtimeAdvanced.slotAllocationSystemLoadDesc',
                          )
                        : runtime.slot_allocation_strategy === 'SLOT_RATIO'
                          ? t(
                              'installer.runtimeAdvanced.slotAllocationSlotRatioDesc',
                            )
                          : t(
                              'installer.runtimeAdvanced.slotAllocationRandomDesc',
                            )
                    }
                  />
                </div>
                <SegmentedControl
                  label={t('installer.runtimeAdvanced.slotAllocationStrategy')}
                  value={runtime.slot_allocation_strategy}
                  onValueChange={(value) =>
                    onChange({
                      slot_allocation_strategy: value as SlotAllocationStrategy,
                    })
                  }
                  options={[
                    {
                      value: 'RANDOM',
                      label: t(
                        'installer.runtimeAdvanced.slotAllocationRandom',
                      ),
                    },
                    {
                      value: 'SYSTEM_LOAD',
                      label: t(
                        'installer.runtimeAdvanced.slotAllocationSystemLoad',
                      ),
                    },
                    {
                      value: 'SLOT_RATIO',
                      label: t(
                        'installer.runtimeAdvanced.slotAllocationSlotRatio',
                      ),
                    },
                  ]}
                />
              </div>
            ) : null}

            {/* 静态 Slot 额外参数：纵向排布避免 SegmentedControl 在半列窄宽内被裁切 / Stack vertically so SegmentedControl never clips in a narrow half-column */}
            {!runtime.dynamic_slot && (
              <div className='space-y-3 pt-0.5'>
                {capabilities?.supports_slot_num && (
                  <div className='relative'>
                    <Input
                      label={t('installer.runtimeAdvanced.slotNum')}
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
                      className='font-mono text-xs tabular-nums'
                    />
                    <div className='absolute right-0 top-0'>
                      <HelpHint
                        label={t('installer.runtimeAdvanced.slotNum')}
                        content={t('installer.runtimeAdvanced.slotNumHint')}
                      />
                    </div>
                  </div>
                )}

                {capabilities?.supports_job_schedule_strategy ? (
                  <div className='space-y-1.5'>
                    <div className='flex items-center gap-1.5'>
                      <span className='text-xs font-medium'>
                        {t('installer.runtimeAdvanced.jobScheduleStrategy')}
                      </span>
                      <HelpHint
                        label={t(
                          'installer.runtimeAdvanced.jobScheduleStrategy',
                        )}
                        content={t(
                          'installer.runtimeAdvanced.jobScheduleHint',
                        )}
                      />
                    </div>
                    <SegmentedControl
                      label={t(
                        'installer.runtimeAdvanced.jobScheduleStrategy',
                      )}
                      value={runtime.job_schedule_strategy}
                      onValueChange={(value) =>
                        onChange({
                          job_schedule_strategy: value as JobScheduleStrategy,
                        })
                      }
                      options={[
                        {
                          value: 'REJECT',
                          label: t(
                            'installer.runtimeAdvanced.jobScheduleReject',
                          ),
                        },
                        {
                          value: 'WAIT',
                          label: t(
                            'installer.runtimeAdvanced.jobScheduleWait',
                          ),
                        },
                      ]}
                    />
                  </div>
                ) : null}
              </div>
            )}
          </div>
        ) : null}
      </div>

      {/* 右列：作业生命周期与日志 / Right Column: Job Lifecycle & Log Mode */}
      <div className='space-y-3 pt-3 md:pt-0'>
        <div className='flex items-center gap-1.5 pb-1.5 border-b-[0.5px] border-border/80 text-xs font-semibold text-foreground/90'>
          <Clock className='h-3.5 w-3.5 text-primary' />
          <span>{t('installer.runtimeAdvanced.historySectionTitle')}</span>
        </div>

        {/* 历史作业保留时间 / History job expire minutes */}
        {showHistorySettings &&
        capabilities?.supports_history_job_expire_minutes ? (
          <div className='relative'>
            <Input
              label={`${t('installer.runtimeAdvanced.historyJobExpireMinutes')} (min)`}
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
              className='font-mono text-xs tabular-nums'
            />
            <div className='absolute right-0 top-0'>
              <HelpHint
                label={t('installer.runtimeAdvanced.historyJobExpireMinutes')}
                content={t('installer.runtimeAdvanced.historyJobExpireHint')}
              />
            </div>
          </div>
        ) : null}

        {/* 日志输出模式 / Log output mode */}
        {showJobLogSettings ? (
          <div className='space-y-1.5' data-testid='install-runtime-log-mode'>
            <div className='flex items-center gap-1.5'>
              <span className='text-xs font-medium'>
                {t('installer.jobLogMode.mode')}
              </span>
              <HelpHint
                label={t('installer.jobLogMode.mode')}
                content={
                  runtime.job_log_mode === 'per_job'
                    ? t('installer.jobLogMode.perJobHint')
                    : t('installer.jobLogMode.mixedHint')
                }
              />
            </div>
            <SegmentedControl
              label={t('installer.jobLogMode.mode')}
              value={runtime.job_log_mode}
              onValueChange={(value) =>
                onChange({job_log_mode: value as JobLogMode})
              }
              options={[
                {
                  value: 'mixed',
                  label: t('installer.jobLogMode.mixed'),
                },
                {
                  value: 'per_job',
                  label: t('installer.jobLogMode.perJob'),
                },
              ]}
            />
          </div>
        ) : null}

        {/* 自动删除过期作业日志 / Scheduled log deletion */}
        {capabilities?.supports_scheduled_deletion_enable ? (
          <div className='flex items-center gap-1.5 pt-1'>
            <Checkbox
              checked={runtime.scheduled_deletion_enable}
              onCheckedChange={(checked) =>
                onChange({scheduled_deletion_enable: checked === true})
              }
              label={t('installer.runtimeAdvanced.scheduledDeletionEnable')}
            />
            <HelpHint
              label={t('installer.runtimeAdvanced.scheduledDeletionEnable')}
              content={t('installer.runtimeAdvanced.scheduledDeletionHint')}
            />
          </div>
        ) : null}
      </div>
    </div>
  );

  const header = (
    <button
      type='button'
      className={cn(
        'flex w-full items-center justify-between gap-2 text-left',
        compact ? 'pb-1' : 'pb-3',
      )}
      onClick={() => setExpanded((prev) => !prev)}
      aria-expanded={expanded}
    >
      <span
        className={cn(
          'flex items-center gap-2',
          compact ? 'text-xs font-semibold' : 'text-base font-semibold',
        )}
      >
        {expanded ? (
          <ChevronDown className={compact ? 'h-3.5 w-3.5' : 'h-4 w-4'} />
        ) : (
          <ChevronRight className={compact ? 'h-3.5 w-3.5' : 'h-4 w-4'} />
        )}
        <Settings2 className={compact ? 'h-3.5 w-3.5' : 'h-4 w-4'} />
        {t('installer.runtimeAdvanced.title')}
      </span>
      {!expanded && summaryParts.length > 0 && (
        <span className='flex flex-wrap items-center justify-end gap-1'>
          {summaryParts.map((part) => (
            <NeutralChip key={part} className='max-w-[9rem] truncate'>
              {part}
            </NeutralChip>
          ))}
        </span>
      )}
    </button>
  );

  // 平滑折叠：grid-rows 过渡 / Smooth collapse via grid-rows transition
  const collapsible = (
    <div
      className={cn(
        'grid transition-all duration-200 ease-[cubic-bezier(0.22,1,0.36,1)]',
        expanded ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
      )}
    >
      <div className='overflow-hidden'>
        <div className={compact ? 'pt-2.5' : 'pt-2'}>{body}</div>
      </div>
    </div>
  );

  // compact 模式压平为单层分区；普通模式保留圆角容器外观
  // compact flattens to a single-layer section; normal mode keeps the card shell
  if (compact) {
    return (
      <section className='border-t-[0.5px] border-border/80 py-4'>
        {header}
        {collapsible}
      </section>
    );
  }

  return (
    <div className='rounded-lg border bg-card p-5'>
      {header}
      {collapsible}
    </div>
  );
}
