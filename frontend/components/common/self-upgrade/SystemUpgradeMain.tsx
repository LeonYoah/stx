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
import {
  ArrowUpCircle,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock,
  Copy,
  History,
  Loader2,
  Package,
  RefreshCw,
  Upload,
  XCircle,
} from 'lucide-react';
import {WorkspaceHeader} from '@/components/common/layout/WorkspaceHeader';
import {TableLoadingBar} from '@/components/common/layout/TableLoadingBar';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Badge} from '@/components/ui/badge';
import {Progress} from '@/components/ui/progress';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {Skeleton} from '@/components/ui/skeleton';
import services from '@/lib/services';
import {
  SelfUpgradeApiError,
  buildExecutePayload,
  type ExecutionMode,
  type ProcessRestart,
  type StatusResponse,
  type UpgradeHistoryRecord,
} from '@/lib/services/self-upgrade';
import {cn} from '@/lib/utils';
import {AgentNodeList, AgentVersionsPanel} from './AgentNodesPanel';
import {ExecuteActions} from './ExecuteActions';
import {toast} from 'sonner';

// 去掉可选的 v 前缀，便于比较与展示版本号
// Strip an optional leading v so versions compare and render consistently
function normalizeVersion(version: string): string {
  const trimmed = version.trim();
  return /^[vV]\d/.test(trimmed) ? trimmed.slice(1) : trimmed;
}

function statusPillClass(status: string): string {
  switch (status) {
    case 'available':
    case 'staged':
      return 'bg-primary/10 text-primary border-primary/20';
    case 'up_to_date':
    case 'succeeded':
      return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20';
    case 'staging':
    case 'upgrading':
    case 'checking':
      return 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20';
    case 'failed':
      return 'bg-destructive/10 text-destructive border-destructive/20';
    default:
      return 'bg-muted text-muted-foreground border-border';
  }
}

// ExecutionModeBadge 标出一次执行是真实应用还是演练。/ ExecutionModeBadge marks a run as a real apply or a dry run.
function ExecutionModeBadge({
  mode,
  dryRun,
  t,
}: {
  mode?: string;
  dryRun?: boolean;
  t: ReturnType<typeof useTranslations>;
}) {
  if (!mode && !dryRun) {
    return null;
  }
  const isDryRun = dryRun || mode === 'dry_run';
  return (
    <Badge
      variant='outline'
      data-testid='execution-mode-badge'
      className={cn(
        'text-[10px]',
        isDryRun
          ? 'bg-sky-500/10 text-sky-700 dark:text-sky-400 border-sky-500/20'
          : 'bg-primary/10 text-primary border-primary/20',
      )}
    >
      {isDryRun ? t('modeDryRun') : t('modeApply')}
    </Badge>
  );
}

// ProcessRestartList 展示控制面/前端的重启结果。/ ProcessRestartList shows control plane / frontend restart results.
function ProcessRestartList({
  restarts,
  t,
}: {
  restarts?: ProcessRestart[];
  t: ReturnType<typeof useTranslations>;
}) {
  if (!restarts?.length) {
    return null;
  }
  const statusLabel: Record<string, string> = {
    planned: t('restartPlanned'),
    pending: t('restartPending'),
    restarting: t('restartRestarting'),
    restarted: t('restartRestarted'),
    rolled_back: t('restartRolledBack'),
    failed: t('restartFailed'),
    skipped: t('restartSkipped'),
  };
  return (
    <div className='space-y-1.5' data-testid='process-restarts'>
      <div className='text-xs font-medium'>{t('restartsTitle')}</div>
      <div className='divide-y rounded-md border text-xs'>
        {restarts.map((r) => (
          <div key={r.component} className='flex flex-wrap items-center gap-2 px-3 py-2'>
            <div className='min-w-0 flex-1'>
              <div className='font-medium'>
                {r.component === 'frontend'
                  ? t('componentFrontend')
                  : t('componentControlPlane')}
                {r.strategy ? (
                  <span className='ml-1.5 text-muted-foreground'>({r.strategy})</span>
                ) : null}
              </div>
              {r.message ? (
                <div className='text-muted-foreground break-words'>{r.message}</div>
              ) : null}
            </div>
            {r.old_pid || r.new_pid ? (
              <span className='font-mono tabular-nums text-muted-foreground shrink-0'>
                PID {r.old_pid || '—'} → {r.new_pid || '—'}
              </span>
            ) : null}
            <Badge
              variant='outline'
              className={cn(
                'text-[10px] shrink-0',
                r.status === 'restarted' &&
                  'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20',
                (r.status === 'rolled_back' || r.status === 'restarting') &&
                  'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20',
                r.status === 'failed' &&
                  'bg-destructive/10 text-destructive border-destructive/20',
              )}
            >
              {statusLabel[r.status] ?? r.status}
            </Badge>
          </div>
        ))}
      </div>
    </div>
  );
}

function assetStatusLabel(
  status: string,
  t: ReturnType<typeof useTranslations>,
): string {
  switch (status) {
    case 'ready':
      return t('assetReady');
    case 'loading':
      return t('assetLoading');
    case 'failed':
      return t('assetFailed');
    default:
      return t('assetPending');
  }
}

export function SystemUpgradeMain() {
  const t = useTranslations('selfUpgrade');
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [errorBanner, setErrorBanner] = useState<string | null>(null);
  const [stageTab, setStageTab] = useState<'online' | 'offline'>('online');
  const [reconnecting, setReconnecting] = useState(false);
  const [history, setHistory] = useState<UpgradeHistoryRecord[]>([]);
  const [expandedRecordId, setExpandedRecordId] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadHistory = useCallback(async () => {
    try {
      const list = await services.selfUpgrade.getHistory();
      setHistory(list ?? []);
    } catch {
      // ignore
    }
  }, []);

  const loadStatus = useCallback(async (opts?: {background?: boolean}) => {
    if (!opts?.background) {
      setRefreshing(true);
    }
    try {
      const data = await services.selfUpgrade.getStatus();
      setStatus(data);
      setErrorBanner(null);
      setReconnecting(false);
      if (data.overall_status === 'succeeded' || data.overall_status === 'failed') {
        void loadHistory();
      }
    } catch (err) {
      if (status?.overall_status === 'upgrading' || busy) {
        // 升级重启期间网络中断属正常现象，展示优雅重连等待，不误刷红色全局错误
        // Disconnection during restart is expected; display graceful reconnecting overlay
        setReconnecting(true);
      } else {
        const msg = err instanceof Error ? err.message : t('loadFailed');
        setErrorBanner(msg);
      }
    } finally {
      setInitialLoading(false);
      setRefreshing(false);
    }
  }, [busy, status?.overall_status, t, loadHistory]);

  useEffect(() => {
    void loadStatus();
    void loadHistory();
  }, [loadStatus, loadHistory]);

  // Poll while staging / upgrading
  useEffect(() => {
    const active =
      status?.overall_status === 'staging' ||
      status?.overall_status === 'upgrading' ||
      status?.stage?.status === 'staging' ||
      status?.active_task?.status === 'staging' ||
      status?.active_task?.status === 'upgrading';
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    if (active) {
      pollRef.current = setInterval(() => {
        void loadStatus({background: true});
      }, 800);
    }
    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
      }
    };
  }, [status?.overall_status, status?.stage?.status, status?.active_task?.status, loadStatus]);

  const overallLabel = useMemo(() => {
    if (!status) return t('statusIdle');
    const map: Record<string, string> = {
      idle: t('statusIdle'),
      checking: t('statusChecking'),
      available: t('statusAvailable'),
      up_to_date: t('statusUpToDate'),
      staging: t('statusStaging'),
      staged: t('statusStaged'),
      upgrading: t('statusUpgrading'),
      succeeded: t('statusSucceeded'),
      failed: t('statusFailed'),
      cancelled: t('statusCancelled'),
    };
    return map[status.overall_status] || status.overall_status;
  }, [status, t]);

  const isDocker = status?.deployment_type === 'docker';

  // 已预置版本与最新可用版本不一致（例如已离线导入 X，远端已有 Y）
  // The staged version differs from the latest available one (e.g. staged X, available Y)
  const stagedVersion = status?.stage?.status === 'staged' ? status.stage.target_version : undefined;
  const newerThanStaged =
    !!stagedVersion &&
    !!status?.latest_version &&
    normalizeVersion(stagedVersion) !== normalizeVersion(status.latest_version);

  // 已预置包会在重新检查后保留；只有存在不同的新版本时才允许显式重新预置替换
  // A staged package survives re-checks; re-staging is allowed only to explicitly replace it with a different version
  const canStage =
    !isDocker &&
    !!status?.checked &&
    !!status?.update_available &&
    status.stage.status !== 'staging' &&
    (status.stage.status !== 'staged' || newerThanStaged) &&
    status.overall_status !== 'upgrading' &&
    status.overall_status !== 'succeeded';

  const canUpgrade =
    !isDocker &&
    status?.stage.status === 'staged' &&
    status.overall_status !== 'upgrading' &&
    status.overall_status !== 'succeeded';

  const handleCheck = async (simulate?: string) => {
    setBusy(true);
    setErrorBanner(null);
    try {
      const result = await services.selfUpgrade.check(
        simulate ? {simulate_failure: simulate} : {},
      );
      if (result.error_code === 'network_unreachable') {
        setErrorBanner(t('errNetworkUnreachable'));
        setStageTab('offline');
      } else if (result.error_code === 'check_fail') {
        setErrorBanner(t('errCheckFail'));
      } else if (result.stage_preserved && result.staged_version) {
        if (normalizeVersion(result.staged_version) === normalizeVersion(result.latest_version)) {
          toast.success(t('stagedKept', {version: normalizeVersion(result.staged_version)}));
        } else {
          toast.success(
            t('stagedKeptNewerAvailable', {
              staged: normalizeVersion(result.staged_version),
              latest: normalizeVersion(result.latest_version),
            }),
          );
        }
      } else if (!result.update_available) {
        toast.success(t('alreadyLatest'));
      } else {
        toast.success(t('updateFound', {version: result.latest_version}));
      }
      await loadStatus({background: true});
    } catch (err) {
      const apiErr = err as SelfUpgradeApiError;
      if (apiErr.errorCode === 'network_unreachable') {
        setErrorBanner(t('errNetworkUnreachable'));
        setStageTab('offline');
      } else if (apiErr.errorCode === 'check_fail') {
        setErrorBanner(t('errCheckFail'));
      } else if (apiErr.errorCode === 'forbidden_non_admin') {
        setErrorBanner(t('errForbidden'));
      } else {
        setErrorBanner(apiErr.message || t('errCheckFail'));
      }
      await loadStatus({background: true});
    } finally {
      setBusy(false);
    }
  };

  const handleDownload = async (simulate?: string) => {
    setBusy(true);
    setErrorBanner(null);
    try {
      await services.selfUpgrade.startDownload(
        simulate ? {simulate_failure: simulate} : {},
      );
      toast.message(t('downloadStarted'));
      await loadStatus({background: true});
    } catch (err) {
      const apiErr = err as SelfUpgradeApiError;
      const code = apiErr.errorCode;
      if (code === 'network_unreachable') {
        setErrorBanner(t('errNetworkUnreachable'));
        setStageTab('offline');
      } else if (code === 'disk_full') setErrorBanner(t('errDiskFull'));
      else if (code === 'concurrent_upgrade') setErrorBanner(t('errConcurrent'));
      else if (code === 'already_latest') setErrorBanner(t('alreadyLatest'));
      else setErrorBanner(apiErr.message || t('errDownload'));
    } finally {
      setBusy(false);
    }
  };

  const handleImport = async (file: File, simulate?: string) => {
    setBusy(true);
    setErrorBanner(null);
    try {
      await services.selfUpgrade.importOffline(file, simulate);
      toast.success(t('importDone'));
      await loadStatus({background: true});
    } catch (err) {
      const apiErr = err as SelfUpgradeApiError;
      if (apiErr.errorCode === 'bad_offline_import') {
        setErrorBanner(t('errBadImport'));
      } else if (apiErr.errorCode === 'disk_full') {
        setErrorBanner(t('errDiskFull'));
      } else if (apiErr.errorCode === 'concurrent_upgrade') {
        setErrorBanner(t('errConcurrent'));
      } else {
        setErrorBanner(apiErr.message || t('errBadImport'));
      }
    } finally {
      setBusy(false);
    }
  };

  // handleExecute 发送显式 mode：apply 真实应用，dry_run 只演练。
  // handleExecute sends an explicit mode: apply really upgrades, dry_run only rehearses.
  const handleExecute = async (mode: ExecutionMode, simulate?: string) => {
    setBusy(true);
    setErrorBanner(null);
    try {
      await services.selfUpgrade.execute(buildExecutePayload(mode, simulate));
      toast.message(mode === 'dry_run' ? t('dryRunStarted') : t('upgradeStarted'));
      await loadStatus({background: true});
    } catch (err) {
      const apiErr = err as SelfUpgradeApiError;
      if (apiErr.errorCode === 'not_staged') setErrorBanner(t('errNotStaged'));
      else if (apiErr.errorCode === 'concurrent_upgrade')
        setErrorBanner(t('errConcurrent'));
      else setErrorBanner(apiErr.message || t('errExecute'));
    } finally {
      setBusy(false);
    }
  };

  const handleCancelStage = async () => {
    try {
      await services.selfUpgrade.cancelStage();
      toast.message(t('cancelled'));
      await loadStatus({background: true});
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('cancelFailed'));
    }
  };

  if (initialLoading && !status) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-12 w-full max-w-md' />
        <Skeleton className='h-40 w-full' />
        <Skeleton className='h-56 w-full' />
      </div>
    );
  }

  const steps = status?.active_task?.steps || [];
  const failureCode =
    status?.active_task?.error_code || status?.stage?.error_code || null;

  return (
    <div className='space-y-4 relative'>
      <TableLoadingBar loading={Boolean(refreshing || busy)} />

      <WorkspaceHeader
        icon={<ArrowUpCircle className='h-5 w-5' />}
        title={t('title')}
        subtitle={t('subtitle')}
        actions={
          <div className='flex flex-wrap items-center gap-2'>
            {/* 检查更新是整页第一步，用主色实心按钮突出，避免被误认为普通刷新而漏点 */}
            {/* Checking for updates is the first step of the page; a solid primary button keeps it from being mistaken for a plain refresh */}
            <Button
              size='sm'
              className='active:scale-[0.98]'
              data-testid='self-upgrade-check'
              disabled={busy || status?.overall_status === 'upgrading'}
              onClick={() => void handleCheck()}
            >
              <RefreshCw className={cn('h-4 w-4 mr-1.5', busy && 'animate-spin')} />
              {t('checkUpdate')}
            </Button>
            {status?.mock_mode ? (
              <Badge variant='outline' className='text-xs font-normal'>
                {t('mockMode')}
              </Badge>
            ) : null}
          </div>
        }
      />

      {/* 阶段向导指示条 / Step Wizard Indicator */}
      <div className='grid grid-cols-2 md:grid-cols-4 gap-2'>
        {[
          {
            key: 'check',
            label: t('step1Check'),
            active: true,
            done: !!status?.checked,
          },
          {
            key: 'stage',
            label: t('step2Stage'),
            active: !!status?.checked && !!status?.update_available,
            done:
              status?.stage?.status === 'staged' ||
              status?.overall_status === 'upgrading' ||
              status?.overall_status === 'succeeded',
            running: status?.stage?.status === 'staging',
          },
          {
            key: 'execute',
            label: t('step3Execute'),
            active: status?.stage?.status === 'staged' || status?.overall_status === 'upgrading',
            done: status?.overall_status === 'succeeded',
            running: status?.overall_status === 'upgrading',
          },
          {
            key: 'verify',
            label: t('step4Verify'),
            active: status?.overall_status === 'succeeded',
            done: status?.overall_status === 'succeeded',
          },
        ].map((step) => (
          <div
            key={step.key}
            className={cn(
              'rounded-md border px-3 py-2 text-xs flex items-center justify-between transition-colors',
              step.done
                ? 'border-emerald-500/30 bg-emerald-500/5 text-foreground'
                : step.running
                  ? 'border-amber-500/30 bg-amber-500/5 text-amber-700 dark:text-amber-300 font-medium'
                  : step.active
                    ? 'border-primary/30 bg-primary/5 text-foreground font-medium'
                    : 'border-border/50 bg-muted/20 text-muted-foreground opacity-75',
            )}
          >
            <span className='truncate'>{step.label}</span>
            {step.done ? (
              <Check className='h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0' />
            ) : step.running ? (
              <Loader2 className='h-3.5 w-3.5 animate-spin text-amber-600 dark:text-amber-400 shrink-0' />
            ) : null}
          </div>
        ))}
      </div>

      {/* 服务平滑重启等待遮罩 / Graceful Reconnection Banner */}
      {reconnecting ? (
        <div className='rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 flex items-center gap-3 animate-pulse'>
          <Loader2 className='h-5 w-5 animate-spin text-amber-600 shrink-0' />
          <div>
            <div className='font-medium text-sm text-amber-900 dark:text-amber-200'>
              {status?.active_task?.current_step === 'restart'
                ? t('reconnectingRestart')
                : t('reconnecting')}
            </div>
            <div className='text-xs text-muted-foreground'>{t('reconnectingHint')}</div>
          </div>
        </div>
      ) : null}

      {errorBanner ? (
        <div className='flex gap-3 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2.5 text-sm'>
          <XCircle className='h-4 w-4 text-destructive shrink-0 mt-0.5' />
          <div>
            <div className='font-medium text-destructive'>{t('errorTitle')}</div>
            <div className='text-muted-foreground'>{errorBanner}</div>
          </div>
        </div>
      ) : null}

      {failureCode ? (
        <div className='rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2.5 text-sm space-y-1'>
          <div className='font-medium'>{t('failureCode', {code: failureCode})}</div>
          <div className='text-muted-foreground'>
            {status?.active_task?.failure_reason || status?.stage?.message}
          </div>
        </div>
      ) : null}

      {/* Docker 容器环境引导卡片 / Docker Container Mode Guidance */}
      {status?.deployment_type === 'docker' ? (
        <Card className='border border-blue-500/30 bg-blue-500/5 shadow-sm'>
          <CardContent className='p-4 space-y-2.5'>
            <div className='flex items-center gap-2'>
              <Badge variant='outline' className='bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/30 font-medium'>
                {t('dockerMode')}
              </Badge>
              <span className='font-semibold text-sm text-foreground'>{t('dockerNoticeTitle')}</span>
            </div>
            <p className='text-xs text-muted-foreground leading-relaxed'>
              {t('dockerNoticeDesc')}
            </p>
            <div className='flex items-center justify-between gap-2 rounded-md bg-muted/80 px-3 py-2 font-mono text-xs border border-border/50'>
              <code className='truncate text-foreground select-all'>
                {status.docker_compose_upgrade_cmd || 'docker compose pull && docker compose up -d'}
              </code>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-7 shrink-0 text-xs gap-1 hover:bg-background'
                onClick={() => {
                  navigator.clipboard.writeText(status.docker_compose_upgrade_cmd || 'docker compose pull && docker compose up -d');
                  toast.success(t('dockerNoticeCopied'));
                }}
              >
                <Copy className='h-3.5 w-3.5' />
                {t('dockerNoticeCopy')}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {/* Version status */}
      <Card className='border shadow-sm'>
        <CardHeader className='py-3 px-4 border-b'>
          <CardTitle className='text-sm font-semibold'>{t('versionCard')}</CardTitle>
        </CardHeader>
        <CardContent className='p-4'>
          <div className='flex flex-wrap items-center gap-6'>
            <div>
              <div className='text-xs text-muted-foreground'>{t('currentVersion')}</div>
              <div className='font-mono text-lg font-semibold tabular-nums'>
                v{status?.current_version || '—'}
              </div>
            </div>
            <div>
              <div className='text-xs text-muted-foreground'>{t('latestVersion')}</div>
              {/* 最新版本只在检查后才有值（服务重启即清空）；未检查时就地给出检查入口，而不是只显示一个“—” */}
              {/* The latest version only exists after a check (cleared on restart); when unchecked, offer the check in place instead of a bare "—" */}
              {status && !status.checked && !status.latest_version ? (
                <Button
                  variant='link'
                  size='sm'
                  className='h-7 px-0 text-sm'
                  data-testid='self-upgrade-check-inline'
                  disabled={busy || status.overall_status === 'upgrading'}
                  onClick={() => void handleCheck()}
                >
                  {t('checkUpdate')}
                </Button>
              ) : (
                <div className='font-mono text-lg font-semibold tabular-nums'>
                  {status?.latest_version ? `v${status.latest_version}` : '—'}
                </div>
              )}
            </div>
            {status?.architecture ? (
              <div>
                <div className='text-xs text-muted-foreground'>{t('architecture')}</div>
                <div className='font-mono text-xs font-medium tabular-nums mt-1 text-muted-foreground'>
                  {status.architecture}
                </div>
              </div>
            ) : null}
            <div className='ml-auto flex items-center gap-2'>
              <Badge variant='outline' className='text-xs font-normal'>
                {status?.deployment_type === 'docker' ? t('dockerMode') : t('hostMode')}
              </Badge>
              <Badge
                variant='outline'
                className={cn(statusPillClass(status?.overall_status || 'idle'))}
              >
                {overallLabel}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Stage assets */}
      <Card className='border shadow-sm'>
        <CardHeader className='py-3 px-4 border-b'>
          <CardTitle className='text-sm font-semibold'>{t('stageCard')}</CardTitle>
        </CardHeader>
        <CardContent className='p-4 space-y-4'>
          <Tabs
            value={stageTab}
            onValueChange={(v) => setStageTab(v as 'online' | 'offline')}
          >
            <TabsList>
              <TabsTrigger value='online'>{t('tabOnline')}</TabsTrigger>
              <TabsTrigger value='offline'>{t('tabOffline')}</TabsTrigger>
            </TabsList>
            <TabsContent value='online' className='space-y-3 mt-3'>
              <div className='flex flex-wrap gap-2'>
                <Button
                  size='sm'
                  className='active:scale-[0.98]'
                  disabled={!canStage || busy}
                  onClick={() => void handleDownload()}
                >
                  <Package className='h-4 w-4 mr-1.5' />
                  {t('download')}
                </Button>
                {status?.stage.cancellable ? (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => void handleCancelStage()}
                  >
                    {t('cancel')}
                  </Button>
                ) : null}
              </div>
              {isDocker ? (
                <p className='text-xs text-blue-600 dark:text-blue-400 font-medium'>{t('dockerStageDisabled')}</p>
              ) : !canStage ? (
                <p className='text-xs text-muted-foreground'>{t('stageHintDisabled')}</p>
              ) : (
                <p className='text-xs text-muted-foreground'>{t('stageHintOnline')}</p>
              )}
            </TabsContent>
            <TabsContent value='offline' className='space-y-3 mt-3'>
              <input
                ref={fileRef}
                type='file'
                accept='.tar.gz,.tgz,.zip'
                className='hidden'
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void handleImport(f);
                  e.target.value = '';
                }}
              />
              <div
                className='border-2 border-dashed border-border/70 hover:border-primary/50 hover:bg-muted/20 rounded-lg p-5 flex flex-col items-center justify-center gap-2 cursor-pointer transition-colors text-center'
                onClick={() => fileRef.current?.click()}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => {
                  e.preventDefault();
                  const f = e.dataTransfer.files?.[0];
                  if (f) void handleImport(f);
                }}
              >
                <Upload className='h-6 w-6 text-muted-foreground' />
                <div className='text-xs font-medium text-foreground'>{t('dropHint')}</div>
                <Button
                  size='sm'
                  variant='secondary'
                  className='active:scale-[0.98] mt-1'
                  disabled={!canStage || busy}
                  onClick={(e) => {
                    e.stopPropagation();
                    fileRef.current?.click();
                  }}
                >
                  {t('import')}
                </Button>
              </div>
              {isDocker ? (
                <p className='text-xs text-blue-600 dark:text-blue-400 font-medium'>{t('dockerStageDisabled')}</p>
              ) : (
                <p className='text-xs text-muted-foreground'>{t('stageHintOffline')}</p>
              )}
            </TabsContent>
          </Tabs>

          {(status?.stage.progress ?? 0) > 0 && status?.stage.status !== 'idle' ? (
            <div className='space-y-1.5'>
              <div className='flex justify-between text-xs text-muted-foreground'>
                <span>{status?.stage.message || t('staging')}</span>
                <span className='font-mono tabular-nums'>{status?.stage.progress}%</span>
              </div>
              <Progress value={status?.stage.progress || 0} />
            </div>
          ) : null}

          <div className='divide-y rounded-md border'>
            {(status?.stage.assets || []).map((asset) => (
              <div
                key={asset.id}
                className='flex items-center gap-3 px-3 py-2.5 text-sm'
              >
                <div
                  className={cn(
                    'flex h-8 w-8 items-center justify-center rounded-md border',
                    asset.status === 'ready' && 'bg-emerald-500/10 border-emerald-500/30',
                    asset.status === 'loading' && 'bg-amber-500/10 border-amber-500/30',
                    asset.status === 'failed' && 'bg-destructive/10 border-destructive/30',
                  )}
                >
                  {asset.status === 'ready' ? (
                    <CheckCircle2 className='h-4 w-4 text-emerald-600' />
                  ) : asset.status === 'loading' ? (
                    <Loader2 className='h-4 w-4 animate-spin text-amber-600' />
                  ) : asset.status === 'failed' ? (
                    <XCircle className='h-4 w-4 text-destructive' />
                  ) : (
                    <Package className='h-4 w-4 text-muted-foreground' />
                  )}
                </div>
                <div className='min-w-0 flex-1'>
                  <div className='font-mono text-xs font-medium truncate'>{asset.name}</div>
                  <div className='text-xs text-muted-foreground truncate'>
                    {asset.description}
                    {asset.message ? ` · ${asset.message}` : ''}
                  </div>
                </div>
                <div className='text-xs text-muted-foreground shrink-0'>
                  {assetStatusLabel(asset.status, t)}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Execute */}
      <Card className='border shadow-sm'>
        <CardHeader className='py-3 px-4 border-b flex-row items-center justify-between space-y-0'>
          <CardTitle className='text-sm font-semibold'>{t('executeCard')}</CardTitle>
          <ExecuteActions
            targetVersion={stagedVersion}
            canUpgrade={canUpgrade}
            busy={busy}
            restartPlan={status?.restart_plan}
            agents={status?.agents}
            onExecute={(mode) => handleExecute(mode)}
          />
        </CardHeader>
        <CardContent className='p-4 space-y-3'>
          {status?.active_task?.kind === 'execute' ? (
            <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
              <span>{t('lastRunMode')}</span>
              <ExecutionModeBadge
                mode={status.active_task.execution_mode}
                dryRun={status.active_task.dry_run}
                t={t}
              />
            </div>
          ) : null}
          {status?.active_task?.dry_run && status.active_task.status === 'succeeded' ? (
            <div
              className='rounded-md border border-sky-500/30 bg-sky-500/5 px-3 py-2.5 text-sm space-y-0.5'
              data-testid='dry-run-result'
            >
              <div className='font-medium text-sky-800 dark:text-sky-300'>
                {t('dryRunDoneTitle')}
              </div>
              <div className='text-xs text-muted-foreground'>{t('dryRunDoneDesc')}</div>
            </div>
          ) : null}
          <p className='text-xs text-muted-foreground'>
            {isDocker
              ? t('dockerExecuteDisabled')
              : canUpgrade
                ? t('executeHintReady')
                : t('executeHintBlocked')}
          </p>
          {!isDocker && newerThanStaged && stagedVersion && status?.latest_version ? (
            <p className='text-xs text-amber-700 dark:text-amber-300'>
              {t('stagedOlderThanLatest', {
                staged: normalizeVersion(stagedVersion),
                latest: normalizeVersion(status.latest_version),
              })}
            </p>
          ) : null}
          <div className='grid gap-2 sm:grid-cols-2'>
            {(steps.length
              ? steps
              : [
                  {code: 'control_plane', label: t('stepControlPlane'), status: 'pending'},
                  {code: 'agent', label: t('stepAgent'), status: 'pending'},
                  {code: 'proxy', label: t('stepProxy'), status: 'pending'},
                  {code: 'restart', label: t('stepRestart'), status: 'pending'},
                  {code: 'verify', label: t('stepVerify'), status: 'pending'},
                ]
            ).map((step) => (
              <div
                key={step.code}
                className={cn(
                  'rounded-md border px-3 py-2.5 flex items-center justify-between gap-2',
                  step.status === 'succeeded' && 'border-emerald-500/30 bg-emerald-500/5',
                  step.status === 'running' && 'border-amber-500/30 bg-amber-500/5',
                  step.status === 'failed' && 'border-destructive/30 bg-destructive/5',
                  step.status === 'partial' && 'border-amber-500/30 bg-amber-500/5',
                )}
              >
                <div className='min-w-0'>
                  <div className='text-sm font-medium truncate'>{step.label}</div>
                  {step.error || step.message ? (
                    <div className='text-xs text-muted-foreground truncate'>
                      {step.error || step.message}
                    </div>
                  ) : null}
                </div>
                <Badge variant='outline' className='shrink-0 text-[10px]'>
                  {step.status === 'succeeded'
                    ? t('stepDone')
                    : step.status === 'running'
                      ? t('stepRunning')
                      : step.status === 'failed'
                        ? t('stepFailed')
                        : step.status === 'partial'
                          ? t('stepPartial')
                          : t('stepPending')}
                </Badge>
              </div>
            ))}
          </div>

          <ProcessRestartList restarts={status?.active_task?.restarts} t={t} />
          {status?.active_task?.agent_nodes?.length ? (
            <AgentNodeList nodes={status.active_task.agent_nodes} />
          ) : null}
          <AgentVersionsPanel
            agents={status?.agents ?? []}
            outdated={status?.agents_outdated ?? 0}
            controlPlaneVersion={status?.current_version ?? ''}
          />

          {/* Dev failure injectors for local smoke — only in mock mode */}
          {status?.mock_mode ? (
            <details className='pt-2 border-t space-y-2 group'>
              <summary className='cursor-pointer text-xs font-medium text-muted-foreground hover:text-foreground flex items-center justify-between select-none py-1'>
                <span>{t('devToolsTitle')}</span>
                <ChevronDown className='h-3.5 w-3.5 transition-transform group-open:rotate-180' />
              </summary>
              <div className='flex flex-wrap gap-1.5 pt-1.5'>
                <Button size='sm' variant='outline' onClick={() => void handleCheck('check_fail')}>
                  {t('simCheckFail')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() => void handleCheck('already_latest')}
                >
                  {t('simAlreadyLatest')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={!canStage}
                  onClick={() => void handleDownload('download_partial')}
                >
                  {t('simDownloadPartial')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={!canStage}
                  onClick={() => {
                    const blob = new Blob(['x'], {type: 'application/gzip'});
                    const file = new File([blob], 'bad.bin', {type: 'application/gzip'});
                    void handleImport(file, 'bad_offline_import');
                  }}
                >
                  {t('simBadImport')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={!canUpgrade}
                  onClick={() => void handleExecute('apply', 'step_fail_agent')}
                >
                  {t('simStepFailAgent')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={!canUpgrade}
                  onClick={() => void handleExecute('apply', 'verify_fail')}
                >
                  {t('simVerifyFail')}
                </Button>
              </div>
            </details>
          ) : null}
        </CardContent>
      </Card>

      {/* 升级历史记录 / Upgrade History */}
      <Card className='border shadow-sm'>
        <CardHeader className='py-3 px-4 border-b flex-row items-center justify-between space-y-0'>
          <div className='flex items-center gap-2'>
            <History className='h-4 w-4 text-muted-foreground' />
            <CardTitle className='text-sm font-semibold'>{t('historyCard')}</CardTitle>
          </div>
          {history.length > 0 ? (
            <Badge variant='outline' className='text-xs font-normal tabular-nums'>
              {history.length}
            </Badge>
          ) : null}
        </CardHeader>
        <CardContent className='p-0'>
          {history.length === 0 ? (
            <div className='py-8 text-center text-xs text-muted-foreground'>
              {t('historyEmpty')}
            </div>
          ) : (
            <div className='divide-y divide-border/60'>
              {history.map((record) => {
                const isExpanded = expandedRecordId === record.id;
                const isSuccess = record.status === 'succeeded';
                const isRolledBack = record.status === 'rolled_back';
                const durationSec = (record.duration_ms / 1000).toFixed(1);

                return (
                  <div key={record.id} className='p-3 sm:p-4 hover:bg-muted/10 transition-colors space-y-2'>
                    <div className='flex flex-wrap items-center justify-between gap-2'>
                      <div className='flex items-center gap-2 flex-wrap'>
                        <span className='font-mono text-xs font-semibold px-2 py-0.5 rounded border bg-muted/30'>
                          v{record.from_version || '1.0.0'} → v{record.target_version}
                        </span>
                        <ExecutionModeBadge
                          mode={record.execution_mode}
                          dryRun={record.dry_run}
                          t={t}
                        />
                        <Badge variant='outline' className='text-[10px] uppercase'>
                          {record.mode || 'online'}
                        </Badge>
                        <Badge variant='outline' className='text-[10px] uppercase'>
                          {record.deployment_type || 'host'}
                        </Badge>
                      </div>

                      <div className='flex items-center gap-2 ml-auto'>
                        <span className='text-xs text-muted-foreground flex items-center gap-1 tabular-nums'>
                          <Clock className='h-3 w-3' />
                          {durationSec}s
                        </span>
                        <span className='text-xs text-muted-foreground hidden sm:inline tabular-nums'>
                          {record.completed_at ? new Date(record.completed_at).toLocaleString() : '—'}
                        </span>
                        <Badge
                          variant='outline'
                          className={cn(
                            'text-xs font-medium',
                            isSuccess && 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20',
                            isRolledBack && 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20',
                            !isSuccess && !isRolledBack && 'bg-destructive/10 text-destructive border-destructive/20',
                          )}
                        >
                          {isSuccess && record.dry_run
                            ? t('historyDryRunDone')
                            : isSuccess
                            ? t('statusSucceeded')
                            : isRolledBack
                              ? t('historyRolledBack')
                              : t('statusFailed')}
                        </Badge>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-7 w-7 p-0'
                          onClick={() => setExpandedRecordId(isExpanded ? null : record.id)}
                        >
                          <ChevronDown
                            className={cn('h-3.5 w-3.5 transition-transform duration-200', isExpanded && 'rotate-180')}
                          />
                        </Button>
                      </div>
                    </div>

                    {/* 失败与回滚信息提示 / Failure & Rollback hint */}
                    {(record.failure_step || record.failure_reason) ? (
                      <div className='rounded bg-destructive/10 border border-destructive/20 px-2.5 py-1.5 text-xs text-destructive flex items-start gap-1.5'>
                        <XCircle className='h-3.5 w-3.5 shrink-0 mt-0.5' />
                        <div>
                          <span className='font-semibold'>
                            {t('historyFailedStep')}: {record.failure_step}
                          </span>
                          {record.failure_reason ? (
                            <span className='ml-1.5 opacity-90'>{record.failure_reason}</span>
                          ) : null}
                          {isRolledBack ? (
                            <span className='ml-2 font-medium text-amber-700 dark:text-amber-400'>
                              ({t('historyRolledBack')})
                            </span>
                          ) : null}
                        </div>
                      </div>
                    ) : null}

                    {/* 展开的步骤明细 / Expanded Steps Breakdown */}
                    {isExpanded && record.steps && record.steps.length > 0 ? (
                      <div className='pt-2 mt-2 border-t border-border/50 grid gap-1.5 sm:grid-cols-2 text-xs'>
                        {record.steps.map((st) => (
                          <div
                            key={st.code}
                            className={cn(
                              'rounded border px-2.5 py-1.5 flex items-center justify-between gap-1.5',
                              st.status === 'succeeded' && 'bg-emerald-500/5 border-emerald-500/20',
                              st.status === 'failed' && 'bg-destructive/5 border-destructive/20',
                              st.status === 'pending' && 'bg-muted/10 border-border/40 text-muted-foreground',
                            )}
                          >
                            <span className='font-medium truncate'>{st.label || st.code}</span>
                            <Badge variant='outline' className='text-[10px] shrink-0'>
                              {st.status === 'succeeded'
                                ? t('stepDone')
                                : st.status === 'failed'
                                  ? t('stepFailed')
                                  : st.status === 'partial'
                                    ? t('stepPartial')
                                    : t('stepPending')}
                            </Badge>
                          </div>
                        ))}
                      </div>
                    ) : null}
                    {isExpanded ? <ProcessRestartList restarts={record.restarts} t={t} /> : null}
                    {isExpanded && record.agent_nodes && record.agent_nodes.length > 0 ? (
                      <AgentNodeList nodes={record.agent_nodes} />
                    ) : null}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
