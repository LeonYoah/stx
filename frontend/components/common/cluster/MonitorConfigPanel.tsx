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
 * 集群守护与自愈策略面板：单层扁平画布 + 0.5px 细线分隔。
 * auto_restart 开关拨动立即持久化（仅提交 { auto_restart }，不污染未保存数值参数），
 * 带固定占位保存态与失败回滚重试；5 项数值参数保留独立保存/重置与脏态上报。
 * Cluster guard & auto-restart policy panel: flat single-layer surface with 0.5px hairlines.
 * The auto_restart Switch persists immediately via partial PUT with optimistic UI, inline
 * status, and rollback/retry on failure; numeric parameters keep Save/Reset + dirty tracking.
 */
import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Activity, Check, RefreshCw, RotateCcw, Save} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Button} from '@/components/arc/button/button';
import {Input} from '@/components/arc/input/input';
import {Switch} from '@/components/arc/switch/switch';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  getMonitorConfig,
  updateMonitorConfig,
  type MonitorConfig,
} from '@/lib/services/monitor';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {toast} from '@/lib/toast';

interface NumericFormState {
  monitor_interval: number;
  restart_delay: number;
  max_restarts: number;
  time_window: number;
  cooldown_period: number;
}

type AutoRestartState =
  | {kind: 'idle'}
  | {kind: 'saving'}
  | {kind: 'saved'}
  | {kind: 'error'; message: string; attemptedValue: boolean};

export interface MonitorConfigPanelProps {
  clusterId: number;
  clusterName?: string;
  /** 仅针对 5 项数值参数的未保存脏态回调 / Dirty-state callback for the 5 numeric parameters */
  onDirtyChange?: (dirty: boolean) => void;
}

function extractNumeric(data: MonitorConfig): NumericFormState {
  return {
    monitor_interval: data.monitor_interval,
    restart_delay: data.restart_delay,
    max_restarts: data.max_restarts,
    time_window: data.time_window,
    cooldown_period: data.cooldown_period,
  };
}

export function MonitorConfigPanel({
  clusterId,
  clusterName,
  onDirtyChange,
}: MonitorConfigPanelProps) {
  const t = useTranslations();
  const [config, setConfig] = useState<MonitorConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // auto_restart 独立状态（拨动立即生效）/ Independent state for immediate auto_restart Switch
  const [autoRestart, setAutoRestart] = useState(false);
  const [autoRestartState, setAutoRestartState] = useState<AutoRestartState>({
    kind: 'idle',
  });
  const savedTimerRef = useRef<number | null>(null);
  const toggleSeqRef = useRef(0);

  // 5 项数值策略参数 / 5 numeric policy parameters
  const [numericForm, setNumericForm] = useState<NumericFormState>({
    monitor_interval: 5,
    restart_delay: 10,
    max_restarts: 3,
    time_window: 300,
    cooldown_period: 1800,
  });
  const [savingNumeric, setSavingNumeric] = useState(false);

  const hasNumericChanges = Boolean(
    config &&
      (numericForm.monitor_interval !== config.monitor_interval ||
        numericForm.restart_delay !== config.restart_delay ||
        numericForm.max_restarts !== config.max_restarts ||
        numericForm.time_window !== config.time_window ||
        numericForm.cooldown_period !== config.cooldown_period),
  );

  useEffect(() => {
    onDirtyChange?.(hasNumericChanges);
  }, [hasNumericChanges, onDirtyChange]);

  useEffect(() => {
    return () => {
      onDirtyChange?.(false);
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
      }
    };
  }, [onDirtyChange]);

  const loadConfig = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await getMonitorConfig(clusterId);
      setConfig(data);
      setAutoRestart(data.auto_restart);
      setNumericForm(extractNumeric(data));
      setAutoRestartState({kind: 'idle'});
    } catch (error) {
      setLoadError(
        error instanceof Error ? error.message : t('monitor.loadError'),
      );
    } finally {
      setLoading(false);
    }
  }, [clusterId, t]);

  useEffect(() => {
    void loadConfig();
  }, [loadConfig]);

  // 拨动 auto_restart 立即局部提交，不覆盖未保存的数值草稿；失败自动回滚并提供重试。
  // Toggle auto_restart with immediate partial PUT without touching unsaved numeric edits; rollback + retry on error.
  const persistAutoRestart = useCallback(
    async (nextValue: boolean) => {
      const previousValue = config?.auto_restart ?? !nextValue;
      const seq = ++toggleSeqRef.current;
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
        savedTimerRef.current = null;
      }

      setAutoRestart(nextValue);
      setAutoRestartState({kind: 'saving'});

      try {
        const updated = await updateMonitorConfig(clusterId, {
          auto_restart: nextValue,
        });
        if (seq !== toggleSeqRef.current) {
          return;
        }
        setConfig((prev) =>
          prev
            ? {
                ...prev,
                auto_restart: updated.auto_restart,
                config_version: updated.config_version,
                last_sync_at: updated.last_sync_at,
                updated_at: updated.updated_at,
              }
            : updated,
        );
        setAutoRestart(updated.auto_restart);
        setAutoRestartState({kind: 'saved'});
        savedTimerRef.current = window.setTimeout(() => {
          setAutoRestartState((cur) =>
            cur.kind === 'saved' ? {kind: 'idle'} : cur,
          );
        }, 2000);
      } catch (error) {
        if (seq !== toggleSeqRef.current) {
          return;
        }
        setAutoRestart(previousValue);
        setAutoRestartState({
          kind: 'error',
          message:
            error instanceof Error ? error.message : t('monitor.saveError'),
          attemptedValue: nextValue,
        });
      }
    },
    [clusterId, config?.auto_restart, t],
  );

  const handleNumericChange = (
    field: keyof NumericFormState,
    raw: string,
    fallback: number,
  ) => {
    const parsed = Number.parseInt(raw, 10);
    setNumericForm((prev) => ({
      ...prev,
      [field]: Number.isNaN(parsed) ? fallback : parsed,
    }));
  };

  const handleSaveNumeric = async () => {
    setSavingNumeric(true);
    try {
      const updated = await updateMonitorConfig(clusterId, {
        monitor_interval: numericForm.monitor_interval,
        restart_delay: numericForm.restart_delay,
        max_restarts: numericForm.max_restarts,
        time_window: numericForm.time_window,
        cooldown_period: numericForm.cooldown_period,
      });
      setConfig(updated);
      setAutoRestart(updated.auto_restart);
      setNumericForm(extractNumeric(updated));
      toast.success(t('monitor.saveSuccess'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('monitor.saveError'),
      );
    } finally {
      setSavingNumeric(false);
    }
  };

  const handleResetNumeric = () => {
    if (config) {
      setNumericForm(extractNumeric(config));
    }
  };

  // 同构骨架屏，严禁居中单个 Spinner 替换整卡 / Matching skeleton, never a lone centered spinner
  if (loading && !config) {
    return (
      <div
        className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'
        data-testid='monitor-config-skeleton'
      >
        <div className='space-y-2 pb-5'>
          <Skeleton className='h-5 w-40' />
          <Skeleton className='h-3.5 w-72' />
        </div>
        <div className='grid gap-6 border-t-[0.5px] border-border/80 py-5 md:grid-cols-2'>
          <Skeleton className='h-10 w-full' />
          <Skeleton className='h-10 w-full' />
        </div>
        <div className='grid gap-4 border-t-[0.5px] border-border/80 pt-5 md:grid-cols-3'>
          <Skeleton className='h-16 w-full' />
          <Skeleton className='h-16 w-full' />
          <Skeleton className='h-16 w-full' />
        </div>
      </div>
    );
  }

  if (loadError && !config) {
    return (
      <div className='flex flex-col items-center justify-center gap-3 rounded-(--radius-panel) border border-(--border) bg-card px-6 py-10 text-center'>
        <p className='text-sm text-destructive'>{loadError}</p>
        <Button
          type='button'
          size='sm'
          variant='secondary'
          onClick={() => void loadConfig()}
        >
          {t('monitoring.retry')}
        </Button>
      </div>
    );
  }

  return (
    <div
      className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'
      data-testid='monitor-config-panel'
    >
      {/* 头部：标题与同步版本信息 / Header: title and sync metadata */}
      <div className='flex flex-wrap items-start justify-between gap-3 pb-5'>
        <div className='min-w-0 space-y-1'>
          <h3 className='text-sm font-semibold text-foreground'>
            {t('monitor.title')}
          </h3>
          <p className='text-xs text-muted-foreground'>
            {clusterName
              ? t('monitor.descriptionWithCluster', {name: clusterName})
              : t('monitor.description')}
          </p>
        </div>
        <div className='flex shrink-0 items-center gap-2'>
          {config?.last_sync_at ? (
            <span className='text-xs text-muted-foreground tabular-nums'>
              {t('monitor.lastSync')}: {formatDateTimeMinuteTz(config.last_sync_at)}
            </span>
          ) : null}
          {config?.config_version !== undefined ? (
            <Badge size='sm' tone='neutral' outline>
              <span className='font-mono tabular-nums'>
                v{config.config_version}
              </span>
            </Badge>
          ) : null}
        </div>
      </div>

      {/* 分组 1：守护与自动重启（拨动即刻生效，不套嵌套框）/ Group 1: Instant switches */}
      <div className='grid gap-6 border-t-[0.5px] border-border/80 py-5 md:grid-cols-2'>
        <div className='flex items-center justify-between gap-4'>
          <div className='min-w-0 space-y-0.5'>
            <div className='flex items-center gap-1.5 text-sm font-medium text-foreground'>
              <Activity className='size-4 text-muted-foreground' aria-hidden />
              <span>{t('monitor.autoMonitor')}</span>
            </div>
            <p className='text-xs text-muted-foreground'>
              {t('monitor.autoMonitorDesc')}
            </p>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type='button'
                className='shrink-0 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
                aria-label={`${t('monitor.alwaysOn')}: ${t('monitor.autoMonitorDesc')}`}
                data-testid='monitor-auto-always-on-trigger'
              >
                <Badge
                  tone='success'
                  outline
                  size='sm'
                  data-testid='monitor-auto-always-on'
                >
                  {t('monitor.alwaysOn')}
                </Badge>
              </button>
            </TooltipTrigger>
            <TooltipContent
              side='top'
              className='max-w-xs text-xs leading-relaxed'
            >
              {t('monitor.autoMonitorDesc')}
            </TooltipContent>
          </Tooltip>
        </div>

        <div className='flex items-center justify-between gap-4'>
          <div className='min-w-0 space-y-0.5'>
            <div className='flex items-center gap-1.5 text-sm font-medium text-foreground'>
              <RefreshCw
                className='size-4 text-muted-foreground'
                aria-hidden
              />
              <label htmlFor='monitor-auto-restart'>
                {t('monitor.autoRestart')}
              </label>
            </div>
            <p className='text-xs text-muted-foreground'>
              {t('monitor.autoRestartDesc')}
            </p>
            {/* 固定位置轻量即时保存反馈 / Fixed-slot inline instant-save status */}
            <div
              className='min-h-4 pt-0.5 text-xs'
              aria-live='polite'
              data-testid='monitor-auto-restart-status'
            >
              {autoRestartState.kind === 'saving' ? (
                <span className='text-muted-foreground'>
                  {t('monitor.autoRestartSaving')}
                </span>
              ) : autoRestartState.kind === 'saved' ? (
                <span className='inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400'>
                  <Check className='size-3' aria-hidden />
                  {t('monitor.autoRestartSaved')}
                </span>
              ) : autoRestartState.kind === 'error' ? (
                <span className='inline-flex items-center gap-2 text-destructive'>
                  <span className='truncate' title={autoRestartState.message}>
                    {autoRestartState.message}
                  </span>
                  <button
                    type='button'
                    className='shrink-0 underline underline-offset-2 hover:opacity-80'
                    onClick={() =>
                      void persistAutoRestart(autoRestartState.attemptedValue)
                    }
                  >
                    {t('monitoring.retry')}
                  </button>
                </span>
              ) : null}
            </div>
          </div>

          <Switch
            id='monitor-auto-restart'
            checked={autoRestart}
            disabled={autoRestartState.kind === 'saving'}
            onCheckedChange={(checked) => void persistAutoRestart(checked)}
            data-testid='monitor-auto-restart-switch'
          />
        </div>
      </div>

      {/* 分组 2：自愈策略阈值参数（显式保存/重置）/ Group 2: Threshold parameters */}
      <div className='space-y-4 border-t-[0.5px] border-border/80 pt-5'>
        <div className='grid gap-x-5 gap-y-4 md:grid-cols-2 lg:grid-cols-3'>
          <Input
            id='monitor_interval'
            type='number'
            min={1}
            max={60}
            label={`${t('monitor.monitorInterval')} (${t('monitor.seconds')})`}
            description={t('monitor.monitorIntervalHint')}
            value={String(numericForm.monitor_interval)}
            onChange={(e) =>
              handleNumericChange('monitor_interval', e.target.value, 5)
            }
            className='tabular-nums'
          />

          <Input
            id='restart_delay'
            type='number'
            min={1}
            max={300}
            label={`${t('monitor.restartDelay')} (${t('monitor.seconds')})`}
            description={t('monitor.restartDelayHint')}
            value={String(numericForm.restart_delay)}
            onChange={(e) =>
              handleNumericChange('restart_delay', e.target.value, 10)
            }
            className='tabular-nums'
          />

          <Input
            id='max_restarts'
            type='number'
            min={1}
            max={10}
            label={`${t('monitor.maxRestarts')} (${t('monitor.times')})`}
            description={t('monitor.maxRestartsHint')}
            value={String(numericForm.max_restarts)}
            onChange={(e) =>
              handleNumericChange('max_restarts', e.target.value, 3)
            }
            className='tabular-nums'
          />

          <Input
            id='time_window'
            type='number'
            min={60}
            max={3600}
            label={`${t('monitor.timeWindow')} (${t('monitor.seconds')})`}
            description={t('monitor.timeWindowHint')}
            value={String(numericForm.time_window)}
            onChange={(e) =>
              handleNumericChange('time_window', e.target.value, 300)
            }
            className='tabular-nums'
          />

          <Input
            id='cooldown_period'
            type='number'
            min={60}
            max={86400}
            label={`${t('monitor.cooldownPeriod')} (${t('monitor.seconds')})`}
            description={t('monitor.cooldownPeriodHint')}
            value={String(numericForm.cooldown_period)}
            onChange={(e) =>
              handleNumericChange('cooldown_period', e.target.value, 1800)
            }
            className='tabular-nums'
          />
        </div>

        <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={handleResetNumeric}
            disabled={!hasNumericChanges || savingNumeric}
          >
            <RotateCcw className='size-3.5' aria-hidden />
            {t('common.reset')}
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={() => void handleSaveNumeric()}
            disabled={!hasNumericChanges || savingNumeric}
            loading={savingNumeric}
          >
            <Save className='size-3.5' aria-hidden />
            {t('common.save')}
          </Button>
        </div>
      </div>
    </div>
  );
}
