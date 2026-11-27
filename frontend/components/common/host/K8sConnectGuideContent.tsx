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
 * Kubernetes 控制面连通性与 RBAC 权限体检引导组件（v2 Structured Output 版）：
 * - 保存前：以单张结构化验证卡展示 1 行 4 阶段流水线 + 2x2 键值字段（身份/权限范围/RBAC 核验/到期）+ 行内明细；
 *   保留 Tip 017 的 300ms/500ms 防闪烁延迟、固定最小高度防跳动与随时取消；
 * - 保存后：展示结构化接入完成卡（直达部署集群/主机详情），并紧跟可选「预备 SeaTunnel 引擎运行账号」结构化卡片，
 *   保持双 SA 清单完全隔离的同时允许一键检测并设为本主机默认以供后续部署自动复用。
 * Kubernetes control-plane probe & post-save onboarding component (v2 structured-output layout):
 * renders a single structured verification card before saving (1-row 4-stage pipeline + 2x2 KV grid + inline details),
 * and a post-save completion card with an optional isolated SeaTunnel engine SA prep & bind-default card.
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  Check,
  Cloud,
  RefreshCw,
  Wrench,
  X,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/arc/button/button';
import services from '@/lib/services';
import {
  HostInfo,
  HostStatus,
  K8sAccessMode,
  K8sAuthMode,
  K8sTestConnectionResult,
} from '@/lib/services/host/types';
import {formatDateTimeTz} from '@/lib/timezone';
import {K8sEngineSATemplateSection} from './K8sEngineSATemplateSection';
import {K8sRBACTemplateSection} from './K8sRBACTemplateSection';
import {
  getK8sConnectErrorHint,
  getK8sCredentialExpiryState,
  getK8sPermissionModeLabel,
  getK8sRbacScopeLabel,
  isK8sRbacRequired,
} from './host-status';

export interface K8sDraftHostInput {
  name: string;
  description?: string;
  k8s_auth_mode: K8sAuthMode;
  k8s_access_mode: K8sAccessMode;
  k8s_namespace: string;
  k8s_kubeconfig?: string;
  k8s_context?: string;
  k8s_api_url?: string;
  k8s_token?: string;
  k8s_ca_cert?: string;
  k8s_insecure_skip_tls?: boolean;
}

interface K8sConnectGuideContentProps {
  host?: HostInfo;
  draft?: K8sDraftHostInput;
  savedHost?: HostInfo | null;
  onConnected?: (updatedHost: HostInfo) => void;
  onHostUpdated?: (updatedHost: HostInfo) => void;
  onBackToEdit?: () => void;
  onSaveHost?: (probe: K8sTestConnectionResult) => Promise<void>;
  savingHost?: boolean;
  onDeployCluster?: (host: HostInfo) => void;
  onViewDetail?: (host: HostInfo) => void;
  onClose?: () => void;
}

type StageStatus = 'pending' | 'running' | 'ok' | 'fail';

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') {
    return false;
  }
  const e = err as {name?: string; code?: string};
  return (
    e.name === 'CanceledError' ||
    e.name === 'AbortError' ||
    e.code === 'ERR_CANCELED'
  );
}

// resolveStageStatuses 将异步探测周期与后端错误码映射为 4 阶段状态指示（Tip 017）。
// resolveStageStatuses maps live probe timing and classified backend errors onto the 4-stage pipeline.
function resolveStageStatuses(params: {
  isBusy: boolean;
  elapsedSec: number;
  cancelled: boolean;
  result: K8sTestConnectionResult | null;
  errorMsg: string;
}): StageStatus[] {
  const {isBusy, elapsedSec, cancelled, result, errorMsg} = params;
  if (cancelled) {
    return ['pending', 'pending', 'pending', 'pending'];
  }
  if (isBusy) {
    if (elapsedSec >= 3) {
      return ['ok', 'ok', 'ok', 'running'];
    }
    return ['ok', 'running', 'pending', 'pending'];
  }
  if (!result && errorMsg) {
    return ['ok', 'fail', 'pending', 'pending'];
  }
  if (!result) {
    return ['pending', 'pending', 'pending', 'pending'];
  }
  if (!result.connected) {
    switch (result.error_code) {
      case 'invalid_config':
        return ['fail', 'pending', 'pending', 'pending'];
      case 'unauthorized':
        return ['ok', 'ok', 'fail', 'pending'];
      default:
        return ['ok', 'fail', 'pending', 'pending'];
    }
  }
  if (result.permission_mode === 'insufficient') {
    return ['ok', 'ok', 'ok', 'fail'];
  }
  return ['ok', 'ok', 'ok', 'ok'];
}

export function K8sConnectGuideContent({
  host,
  draft,
  savedHost = null,
  onConnected,
  onHostUpdated,
  onBackToEdit,
  onSaveHost,
  savingHost = false,
  onDeployCluster,
  onViewDetail,
  onClose,
}: K8sConnectGuideContentProps) {
  const t = useTranslations();
  const isDraft = Boolean(draft && !host);
  const [currentHost, setCurrentHost] = useState<HostInfo | undefined>(host);
  const [effectiveSavedHost, setEffectiveSavedHost] = useState<HostInfo | null>(
    savedHost,
  );
  const [testing, setTesting] = useState(true);
  const [showLoader, setShowLoader] = useState(false);
  const [elapsedSec, setElapsedSec] = useState(0);
  const [cancelled, setCancelled] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [result, setResult] = useState<K8sTestConnectionResult | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [enginePrepOpen, setEnginePrepOpen] = useState(true);

  const onConnectedRef = useRef(onConnected);
  onConnectedRef.current = onConnected;
  const currentHostRef = useRef(currentHost);
  currentHostRef.current = currentHost;
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const abortRef = useRef<AbortController | null>(null);
  const loaderShownAtRef = useRef<number | null>(null);
  const minHoldTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    setCurrentHost(host);
  }, [host]);

  useEffect(() => {
    setEffectiveSavedHost(savedHost);
  }, [savedHost]);

  // Tip 017：加载超过 300ms 才显示指示器，显现后至少驻留 500ms 防闪烁；并按 3s / 10s 更新阶段与慢响应提示。
  // Tip 017: delay loader by 300ms and hold at least 500ms once visible; track elapsed seconds for stage & slow hints.
  useEffect(() => {
    if (minHoldTimerRef.current) {
      clearTimeout(minHoldTimerRef.current);
      minHoldTimerRef.current = null;
    }
    if (!testing) {
      if (loaderShownAtRef.current !== null) {
        const shownFor = Date.now() - loaderShownAtRef.current;
        const remain = Math.max(0, 500 - shownFor);
        if (remain > 0) {
          minHoldTimerRef.current = setTimeout(() => {
            loaderShownAtRef.current = null;
            setShowLoader(false);
          }, remain);
          return () => {
            if (minHoldTimerRef.current) {
              clearTimeout(minHoldTimerRef.current);
              minHoldTimerRef.current = null;
            }
          };
        }
      }
      loaderShownAtRef.current = null;
      setShowLoader(false);
      return;
    }

    setElapsedSec(0);
    const delayTimer = setTimeout(() => {
      loaderShownAtRef.current = Date.now();
      setShowLoader(true);
    }, 300);
    const tickTimer = setInterval(() => {
      setElapsedSec((prev) => prev + 1);
    }, 1000);
    return () => {
      clearTimeout(delayTimer);
      clearInterval(tickTimer);
    };
  }, [testing]);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      if (minHoldTimerRef.current) {
        clearTimeout(minHoldTimerRef.current);
      }
    };
  }, []);

  const draftProbeKey = draft
    ? JSON.stringify([
        draft.k8s_auth_mode,
        draft.k8s_access_mode,
        draft.k8s_namespace,
        draft.k8s_kubeconfig,
        draft.k8s_context,
        draft.k8s_api_url,
        draft.k8s_token,
        draft.k8s_ca_cert,
        draft.k8s_insecure_skip_tls,
      ])
    : '';

  const runProbe = useCallback(async () => {
    void draftProbeKey;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setTesting(true);
    setCancelled(false);
    setErrorMsg('');
    try {
      const currentDraft = draftRef.current;
      const probe =
        !host?.id && currentDraft
          ? await services.host.testK8sDraftConnection(
              {
                k8s_auth_mode: currentDraft.k8s_auth_mode,
                k8s_namespace: currentDraft.k8s_namespace || 'default',
                k8s_kubeconfig: currentDraft.k8s_kubeconfig,
                k8s_context: currentDraft.k8s_context,
                k8s_api_url: currentDraft.k8s_api_url,
                k8s_token: currentDraft.k8s_token,
                k8s_ca_cert: currentDraft.k8s_ca_cert,
                k8s_insecure_skip_tls: currentDraft.k8s_insecure_skip_tls,
              },
              {signal: controller.signal, timeoutMs: 20000},
            )
          : host?.id
            ? await services.host.testK8sConnection(host.id, {
                signal: controller.signal,
                timeoutMs: 20000,
              })
            : null;
      if (!probe || controller.signal.aborted) {
        return;
      }
      setResult(probe);
      if (probe.permission_mode === 'insufficient') {
        setDetailsOpen(true);
      }
      if (probe.connected && host?.id) {
        const base = currentHostRef.current;
        if (base) {
          const updated: HostInfo = {
            ...base,
            status: HostStatus.CONNECTED,
            is_online: true,
            k8s_version: probe.server_version || base.k8s_version,
            k8s_permission_mode: probe.permission_mode || base.k8s_permission_mode,
            k8s_credential_expires_at:
              probe.credential_expires_at ?? base.k8s_credential_expires_at,
          };
          setCurrentHost(updated);
          onConnectedRef.current?.(updated);
        }
      }
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setResult(null);
      setErrorMsg(err instanceof Error ? err.message : String(err));
    } finally {
      if (abortRef.current === controller) {
        setTesting(false);
      }
    }
  }, [host?.id, draftProbeKey]);

  useEffect(() => {
    void runProbe();
  }, [runProbe]);

  const handleCancelProbe = () => {
    abortRef.current?.abort();
    abortRef.current = null;
    loaderShownAtRef.current = null;
    setShowLoader(false);
    setTesting(false);
    setCancelled(true);
  };

  const handleBackToEdit = () => {
    abortRef.current?.abort();
    onBackToEdit?.();
  };

  const rbacItems = useMemo(() => result?.rbac || [], [result]);
  const requiredItems = rbacItems.filter((item) => isK8sRbacRequired(item));
  const extendedItems = rbacItems.filter((item) => !isK8sRbacRequired(item));
  const requiredOk = requiredItems.filter((item) => item.allowed).length;
  const extendedOk = extendedItems.filter((item) => item.allowed).length;
  const deniedRequiredLabels = requiredItems
    .filter((item) => !item.allowed)
    .map((item) => `${item.resource}(${item.verb})`);

  const displayName =
    effectiveSavedHost?.name || draft?.name || currentHost?.name || '-';
  const namespace =
    effectiveSavedHost?.k8s_namespace ||
    draft?.k8s_namespace ||
    currentHost?.k8s_namespace ||
    'seatunnel';
  const effectiveAuthMode: K8sAuthMode =
    draft?.k8s_auth_mode || currentHost?.k8s_auth_mode || 'kubeconfig';

  const isBusy = testing || showLoader;
  const failureHint = getK8sConnectErrorHint(result?.error_code, t);
  const canSaveDraft = Boolean(
    isDraft &&
      !effectiveSavedHost &&
      onSaveHost &&
      !isBusy &&
      result?.connected &&
      result.permission_mode !== 'insufficient',
  );
  const expiryTimestamp =
    result?.credential_expires_at || currentHost?.k8s_credential_expires_at;
  const expiryState = getK8sCredentialExpiryState(expiryTimestamp);
  const stageStatuses = resolveStageStatuses({
    isBusy,
    elapsedSec,
    cancelled,
    result,
    errorMsg,
  });
  const stageLabels = [
    t('host.k8sGuide.stageFormat'),
    t('host.k8sGuide.stageNetwork'),
    t('host.k8sGuide.stageAuth'),
    t('host.k8sGuide.stageRbac'),
  ];
  const boundRunnerSA = effectiveSavedHost?.k8s_default_service_account?.trim() || '';

  return (
    <div className='space-y-4' data-testid='k8s-connect-guide'>
      {effectiveSavedHost ? (
        <>
          {/* 保存完成后的结构化结果卡：字段芯片 + 直达操作 / Structured post-save completion card */}
          <div
            className='flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-control)] bg-emerald-500/10 px-4 py-3 text-xs'
            data-testid='k8s-guide-saved-banner'
          >
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <span className='inline-flex items-center gap-1.5 font-semibold text-emerald-700 dark:text-emerald-300'>
                <Check className='size-3.5' aria-hidden />
                {t('host.k8sGuide.savedBannerTitle')}
              </span>
              <Badge variant='outline' className='font-mono text-[11px]'>
                {effectiveSavedHost.name}
              </Badge>
              <Badge variant='outline' className='font-mono text-[11px]'>
                ns: {namespace}
              </Badge>
              <Badge
                variant={boundRunnerSA ? 'default' : 'outline'}
                className={
                  boundRunnerSA
                    ? 'border-emerald-500/30 bg-emerald-500/15 font-mono text-[11px] text-emerald-700 dark:text-emerald-300'
                    : 'text-[11px]'
                }
                data-testid='k8s-guide-saved-engine-sa-chip'
              >
                {boundRunnerSA
                  ? t('host.k8sEngineSaBoundBadge', {sa: boundRunnerSA})
                  : t('host.k8sEngineSaAutoLabel')}
              </Badge>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              {onViewDetail ? (
                <Button
                  type='button'
                  size='sm'
                  variant='secondary'
                  onClick={() => {
                    onClose?.();
                    onViewDetail(effectiveSavedHost);
                  }}
                  data-testid='k8s-guide-view-detail'
                >
                  {t('host.installGuide.viewHostDetail')}
                </Button>
              ) : null}
              {onDeployCluster ? (
                <Button
                  type='button'
                  size='sm'
                  onClick={() => {
                    onClose?.();
                    onDeployCluster(effectiveSavedHost);
                  }}
                  data-testid='k8s-guide-deploy-cluster'
                >
                  {t('host.k8sGuide.deployClusterAction')}
                </Button>
              ) : null}
            </div>
          </div>

          {/* 可选预备 SeaTunnel 引擎运行账号结构化卡片：与控制面 YAML 独立，绑定后后续建集群自动复用 */}
          {/* Optional SeaTunnel engine SA prep card: isolated from control-plane YAML, reused on deploy once bound */}
          <div
            className='overflow-hidden rounded-[var(--radius-control)] bg-muted/35 dark:bg-muted/25'
            data-testid='k8s-guide-engine-sa-card'
          >
            <div className='flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-xs'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <span className='font-medium text-foreground'>
                  {t('host.k8sEngineSaPrepTitle')}
                </span>
                <Badge variant='outline' className='text-[11px]'>
                  {t('host.k8sEngineSaPrepBadge')}
                </Badge>
                {boundRunnerSA ? (
                  <Badge
                    variant='default'
                    className='border-emerald-500/30 bg-emerald-500/15 font-mono text-[11px] text-emerald-700 dark:text-emerald-300'
                  >
                    {t('host.k8sEngineSaBoundBadge', {sa: boundRunnerSA})}
                  </Badge>
                ) : null}
              </div>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={() => setEnginePrepOpen((prev) => !prev)}
                data-testid='k8s-guide-engine-sa-toggle'
              >
                {enginePrepOpen
                  ? `${t('host.k8sEngineSaCollapse')} ▾`
                  : `${t('host.k8sEngineSaExpand')} ▸`}
              </Button>
            </div>
            {enginePrepOpen ? (
              <div className='space-y-3 border-t-[0.5px] border-border/70 px-4 py-3'>
                <p className='rounded-md bg-background/70 px-3 py-2 text-xs text-muted-foreground dark:bg-background/40'>
                  {t('host.k8sEngineSaSectionDesc')}
                </p>
                <K8sEngineSATemplateSection
                  collapsible={false}
                  namespace={namespace}
                  hostId={effectiveSavedHost.id}
                  initialRunnerSA={boundRunnerSA || undefined}
                  onBoundDefaultSA={(updated) => {
                    setEffectiveSavedHost(updated);
                    onHostUpdated?.(updated);
                  }}
                />
              </div>
            ) : null}
          </div>
        </>
      ) : (
        /* 保存前 / 既有主机重测：单张 Structured Output 预检卡片 / Structured verification card */
        <div
          className='overflow-hidden rounded-[var(--radius-control)] bg-muted/35 dark:bg-muted/25'
          aria-busy={isBusy}
          aria-live='polite'
        >
          {/* 卡片头部：主机名 + 命名空间 + 草稿标记 + 连通结论 / Card header */}
          <div className='flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-xs'>
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <Cloud className='size-4 shrink-0 text-primary' aria-hidden />
              <span className='font-medium text-foreground'>{displayName}</span>
              <Badge variant='outline' className='font-mono text-[11px]'>
                ns: {namespace}
              </Badge>
              {isDraft ? (
                <Badge
                  variant='outline'
                  className='text-[11px] text-muted-foreground'
                  data-testid='k8s-guide-draft-notice'
                >
                  {t('host.k8sGuide.notPersistedNotice')}
                </Badge>
              ) : null}
            </div>
            {!isBusy ? (
              <div className='flex flex-wrap items-center gap-1.5'>
                {cancelled ? (
                  <Badge variant='outline'>{t('host.k8sGuide.cancelTest')}</Badge>
                ) : result?.connected ? (
                  <Badge
                    variant={
                      result.permission_mode === 'insufficient'
                        ? 'destructive'
                        : 'default'
                    }
                    className={
                      result.permission_mode === 'insufficient'
                        ? 'gap-1 text-[11px] tabular-nums'
                        : 'gap-1 border-emerald-500/30 bg-emerald-500/15 font-medium text-emerald-700 tabular-nums dark:text-emerald-300'
                    }
                  >
                    {result.permission_mode === 'insufficient' ? (
                      <AlertTriangle className='size-3' aria-hidden />
                    ) : (
                      <Check className='size-3' aria-hidden />
                    )}
                    <span>
                      {result.permission_mode === 'insufficient'
                        ? t('host.k8sPermissionModeInsufficient')
                        : t('host.k8sConnected')}
                      {result.server_version ? ` · ${result.server_version}` : ''}
                      {result.elapsed_ms ? ` · ${result.elapsed_ms}ms` : ''}
                    </span>
                  </Badge>
                ) : (
                  <Badge variant='destructive'>{t('host.k8sConnectFailed')}</Badge>
                )}
              </div>
            ) : null}
          </div>

          {/* 1 行 4 阶段微缩流水线（横向 4 格，节省 ~120px 垂直空间） / Compact 1-row 4-stage strip */}
          <div className='border-t-[0.5px] border-border/70 px-4 py-2.5'>
            <div
              className='grid grid-cols-2 gap-2 sm:grid-cols-4'
              data-testid='k8s-guide-stage-list'
            >
              {stageLabels.map((label, idx) => {
                const st = stageStatuses[idx] || 'pending';
                return (
                  <div
                    key={label}
                    className='flex items-center justify-between gap-2 rounded-md bg-background/80 px-2.5 py-1.5 text-xs dark:bg-background/40'
                  >
                    <span
                      className={
                        st === 'pending'
                          ? 'truncate text-muted-foreground'
                          : 'truncate font-medium text-foreground'
                      }
                    >
                      {idx + 1} {label}
                    </span>
                    {st === 'running' ? (
                      <RefreshCw
                        className='size-3 shrink-0 animate-spin text-primary'
                        aria-label={t('host.k8sGuide.stageRunning')}
                      />
                    ) : st === 'ok' ? (
                      <span
                        className='shrink-0 font-semibold text-emerald-600 dark:text-emerald-400'
                        aria-label={t('host.k8sGuide.stagePassed')}
                      >
                        ✓
                      </span>
                    ) : st === 'fail' ? (
                      <span
                        className='shrink-0 font-semibold text-rose-600 dark:text-rose-400'
                        aria-label={t('host.k8sGuide.stageFailed')}
                      >
                        ✕
                      </span>
                    ) : (
                      <span
                        className='shrink-0 text-muted-foreground'
                        aria-label={t('host.k8sGuide.stageWaiting')}
                      >
                        ○
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* 结果主体（预留最小高度防止内容到达跳动 / Reserved min-height body） */}
          <div className='min-h-[112px] border-t-[0.5px] border-border/70 px-4 py-3'>
            {isBusy && showLoader ? (
              <div className='flex flex-wrap items-center justify-between gap-2 py-2 text-xs'>
                <span className='text-muted-foreground'>
                  {elapsedSec >= 10
                    ? t('host.k8sGuide.slowHint')
                    : elapsedSec >= 3
                      ? t('host.k8sGuide.testingStageRbac')
                      : t('host.k8sGuide.testing')}
                </span>
                <Button
                  type='button'
                  size='sm'
                  variant='ghost'
                  onClick={handleCancelProbe}
                  data-testid='k8s-guide-cancel'
                >
                  <X className='size-3.5' aria-hidden />
                  {t('host.k8sGuide.cancelTest')}
                </Button>
              </div>
            ) : cancelled ? (
              <p className='py-2 text-xs text-muted-foreground'>
                {t('host.k8sGuide.cancelledHint')}
              </p>
            ) : !isBusy && !result?.connected ? (
              <div className='space-y-1.5 rounded-md bg-rose-500/10 px-3 py-2.5 text-xs'>
                <p className='font-medium text-rose-600 dark:text-rose-400'>
                  {failureHint}
                </p>
                <p className='font-mono text-[11px] text-muted-foreground break-words'>
                  {result?.message || errorMsg || t('host.k8sConnectFailed')}
                </p>
              </div>
            ) : !isBusy && result?.permission_mode === 'insufficient' ? (
              <div className='space-y-3'>
                <div className='flex items-start gap-2 rounded-md bg-rose-500/10 px-3 py-2.5 text-xs'>
                  <AlertTriangle
                    className='mt-0.5 size-4 shrink-0 text-rose-600'
                    aria-hidden
                  />
                  <div className='space-y-1'>
                    <p className='font-medium text-rose-600 dark:text-rose-400'>
                      {t('host.k8sGuide.insufficientTitle')}
                    </p>
                    <p className='text-muted-foreground'>
                      {t('host.k8sGuide.insufficientDesc', {
                        permissions: deniedRequiredLabels.join(', '),
                      })}
                    </p>
                  </div>
                </div>
                <K8sRBACTemplateSection
                  namespace={namespace}
                  authMode={effectiveAuthMode}
                  defaultOpen
                />
              </div>
            ) : !isBusy && result?.connected ? (
              /* Structured Output 2x2 键值网格（零冗余长句） / Structured 2x2 KV grid */
              <div className='space-y-3'>
                <dl className='grid grid-cols-1 gap-x-6 gap-y-3 text-xs sm:grid-cols-2'>
                  <div>
                    <dt className='text-[11px] text-muted-foreground'>
                      {t('host.k8sGuide.detectedControlPlaneSa')}
                    </dt>
                    <dd className='mt-0.5 font-mono font-medium text-foreground'>
                      {result.control_plane_sa || '-'}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-[11px] text-muted-foreground'>
                      {t('host.k8sGuide.permissionScopeLabel')}
                    </dt>
                    <dd className='mt-0.5'>
                      <Badge
                        variant='outline'
                        className='text-[11px]'
                        data-testid='k8s-guide-permission-mode'
                      >
                        {result.permission_mode
                          ? getK8sPermissionModeLabel(result.permission_mode, t)
                          : '-'}
                      </Badge>
                    </dd>
                  </div>
                  <div>
                    <dt className='text-[11px] text-muted-foreground'>
                      {t('host.k8sGuide.rbacSummaryLabel')}
                    </dt>
                    <dd className='mt-0.5 flex flex-wrap items-center gap-2 tabular-nums'>
                      <span className='font-medium text-foreground'>
                        {t('host.k8sGuide.rbacSummaryValue', {
                          requiredOk,
                          requiredTotal: requiredItems.length,
                          extendedOk,
                          extendedTotal: extendedItems.length,
                        })}
                      </span>
                      {rbacItems.length > 0 ? (
                        <button
                          type='button'
                          className='text-[11px] font-medium text-primary hover:underline'
                          onClick={() => setDetailsOpen((open) => !open)}
                          aria-expanded={detailsOpen}
                          data-testid='k8s-guide-rbac-details-toggle'
                        >
                          {t('host.k8sGuide.rbacDetailsToggle')}{' '}
                          {detailsOpen ? '▾' : '▸'}
                        </button>
                      ) : null}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-[11px] text-muted-foreground'>
                      {t('host.k8sCredentialExpiry')}
                    </dt>
                    <dd className='mt-0.5 flex flex-wrap items-center gap-1.5 font-mono tabular-nums'>
                      <span className='text-foreground'>
                        {expiryTimestamp
                          ? formatDateTimeTz(expiryTimestamp)
                          : t('host.k8sCredentialExpiryUnknown')}
                      </span>
                      {expiryState === 'expired' ? (
                        <Badge variant='destructive' className='text-[11px]'>
                          {t('host.k8sCredentialExpired')}
                        </Badge>
                      ) : expiryState === 'expiring_soon' ? (
                        <Badge variant='secondary' className='text-[11px]'>
                          {t('host.k8sCredentialExpiringSoon')}
                        </Badge>
                      ) : null}
                    </dd>
                  </div>
                </dl>

                {detailsOpen && rbacItems.length > 0 ? (
                  <div className='max-h-52 overflow-auto border-t-[0.5px] border-border/70 pt-2 text-xs'>
                    <table className='w-full [&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                      <thead>
                        <tr className='text-left text-muted-foreground'>
                          <th className='px-2 py-1.5 font-medium'>
                            {t('host.k8sRbacResource')}
                          </th>
                          <th className='px-2 py-1.5 font-medium'>
                            {t('host.k8sRbacVerb')}
                          </th>
                          <th className='px-2 py-1.5 font-medium'>
                            {t('host.k8sRbacScope')}
                          </th>
                          <th className='px-2 py-1.5 font-medium'>
                            {t('host.k8sRbacPrecheck')}
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {rbacItems.map((item, idx) => {
                          const isOptionalOrExtended = !isK8sRbacRequired(item);
                          const cls = item.allowed
                            ? 'text-emerald-600'
                            : isOptionalOrExtended
                              ? 'text-muted-foreground'
                              : 'text-rose-600';
                          return (
                            <tr key={idx} className='last:border-0'>
                              <td className='px-2 py-1 font-mono'>{item.resource}</td>
                              <td className='px-2 py-1'>{item.verb}</td>
                              <td className='px-2 py-1 text-muted-foreground'>
                                {getK8sRbacScopeLabel(item.scope, t)}
                              </td>
                              <td className={`px-2 py-1 ${cls}`}>
                                {item.allowed
                                  ? t('host.k8sRbacAllowed')
                                  : isOptionalOrExtended
                                    ? t('host.k8sRbacOptionalDenied')
                                    : t('host.k8sRbacDenied')}
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      )}

      {/* 底部操作栏 / Footer actions */}
      <div className='flex flex-wrap items-center justify-between gap-2 border-t-[0.5px] border-border/80 pt-3'>
        <div className='flex items-center gap-2'>
          {onBackToEdit && !effectiveSavedHost ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={handleBackToEdit}
              disabled={savingHost}
              data-testid='k8s-guide-back-edit'
            >
              <Wrench className='size-3.5' aria-hidden />
              {t('host.k8sGuide.backToEdit')}
            </Button>
          ) : null}
          {!effectiveSavedHost ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={() => void runProbe()}
              disabled={isBusy || savingHost}
              data-testid='k8s-guide-retest'
            >
              <RefreshCw className='size-3.5' aria-hidden />
              {t('host.k8sGuide.retest')}
            </Button>
          ) : null}
        </div>
        <div className='flex items-center gap-2'>
          {isDraft && onSaveHost && !effectiveSavedHost ? (
            <Button
              type='button'
              size='sm'
              loading={savingHost}
              disabled={!canSaveDraft || savingHost}
              onClick={() => {
                if (result) {
                  void onSaveHost(result);
                }
              }}
              data-testid='k8s-guide-save-host'
            >
              {savingHost
                ? t('host.k8sGuide.savingHost')
                : t('host.k8sGuide.saveHost')}
            </Button>
          ) : null}
          {(!isDraft && onViewDetail && currentHost) || (effectiveSavedHost && onClose) ? (
            <>
              {!isDraft && onViewDetail && currentHost ? (
                <Button
                  type='button'
                  size='sm'
                  variant='secondary'
                  onClick={() => onViewDetail(currentHost)}
                >
                  {t('host.installGuide.viewHostDetail')}
                </Button>
              ) : null}
              {onClose ? (
                <Button type='button' size='sm' onClick={onClose}>
                  {t('host.installGuide.done')}
                </Button>
              ) : null}
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}
