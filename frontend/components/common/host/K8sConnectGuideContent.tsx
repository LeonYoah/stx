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
 * Kubernetes 主机连通性与 RBAC 权限体检引导组件：
 * 创建第 2 步及列表/操作区触发时自动运行 testK8sConnection，支持可取消探测、
 * 300ms 延迟 / 500ms 驻留的单一加载态、分类故障排障建议、动态排除当前控制面身份并一键绑定默认运行 SA。
 * Kubernetes host connectivity & RBAC onboarding guide component:
 * auto-runs abortable testK8sConnection with single-indicator delayed loading, classified error hints,
 * dynamic control-plane SA exclusion, and one-click runner ServiceAccount binding.
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  Check,
  Cloud,
  RefreshCw,
  ShieldCheck,
  Wrench,
  X,
} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/arc/button/button';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import services from '@/lib/services';
import {
  HostInfo,
  HostStatus,
  K8sTestConnectionResult,
} from '@/lib/services/host/types';
import {DEFAULT_STX_MANAGER_SA} from '@/lib/k8s-rbac-templates';
import {K8sRBACTemplateSection} from './K8sRBACTemplateSection';
import {
  getK8sConnectErrorHint,
  getK8sPermissionModeLabel,
} from './host-status';

interface K8sConnectGuideContentProps {
  host: HostInfo;
  onConnected?: (updatedHost: HostInfo) => void;
  onBackToEdit?: () => void;
  onViewDetail?: (host: HostInfo) => void;
  onClose?: () => void;
}

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') {
    return false;
  }
  const e = err as {name?: string; code?: string};
  return e.name === 'CanceledError' || e.name === 'AbortError' || e.code === 'ERR_CANCELED';
}

export function K8sConnectGuideContent({
  host,
  onConnected,
  onBackToEdit,
  onViewDetail,
  onClose,
}: K8sConnectGuideContentProps) {
  const t = useTranslations();
  const [currentHost, setCurrentHost] = useState<HostInfo>(host);
  const [testing, setTesting] = useState(true);
  const [showLoader, setShowLoader] = useState(false);
  const [elapsedSec, setElapsedSec] = useState(0);
  const [cancelled, setCancelled] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [result, setResult] = useState<K8sTestConnectionResult | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [selectedSA, setSelectedSA] = useState(
    host.k8s_default_service_account || '',
  );
  const [savingSA, setSavingSA] = useState(false);
  const [savedSAHint, setSavedSAHint] = useState(false);

  const onConnectedRef = useRef(onConnected);
  onConnectedRef.current = onConnected;
  const currentHostRef = useRef(currentHost);
  currentHostRef.current = currentHost;
  const defaultSARef = useRef(host.k8s_default_service_account);
  defaultSARef.current = host.k8s_default_service_account;
  const prevHostIdRef = useRef(host.id);
  const abortRef = useRef<AbortController | null>(null);
  const loaderShownAtRef = useRef<number | null>(null);
  const minHoldTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    setCurrentHost(host);
    // 仅在切换主机或后端明确返回已配置默认 SA 时更新选择态，避免 onConnected 回写 host 冲掉自动推断的 seatunnel-runner。
    // Only reset selectedSA when switching host IDs or receiving a configured SA, so onConnected updates don't clobber auto-detected seatunnel-runner.
    setSelectedSA((prev) => {
      if (host.k8s_default_service_account) {
        return host.k8s_default_service_account;
      }
      return host.id === prevHostIdRef.current ? prev : '';
    });
    prevHostIdRef.current = host.id;
  }, [host]);

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

  // 动态排除当前控制面自身身份（无论自定义名还是默认 stx-manager），防止引擎 Pod 误继承控制面高权限。
  // Dynamically exclude the detected control-plane SA (and default stx-manager) so engine pods never inherit control-plane rights.
  const excludedControlPlaneSAs = useMemo(() => {
    const excluded = new Set<string>([DEFAULT_STX_MANAGER_SA]);
    if (result?.control_plane_sa) {
      excluded.add(result.control_plane_sa);
    }
    return excluded;
  }, [result?.control_plane_sa]);

  const candidateSAs = useMemo(() => {
    const raw = result?.service_accounts || [];
    return raw.filter((sa) => !excludedControlPlaneSAs.has(sa));
  }, [result?.service_accounts, excludedControlPlaneSAs]);

  const runProbe = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setTesting(true);
    setCancelled(false);
    setErrorMsg('');
    setSavedSAHint(false);
    try {
      const probe = await services.host.testK8sConnection(host.id, {
        signal: controller.signal,
        timeoutMs: 20000,
      });
      if (controller.signal.aborted) {
        return;
      }
      setResult(probe);
      if (probe.permission_mode === 'insufficient') {
        setDetailsOpen(true);
      }
      const excluded = new Set<string>([DEFAULT_STX_MANAGER_SA]);
      if (probe.control_plane_sa) {
        excluded.add(probe.control_plane_sa);
      }
      const candidates = (probe.service_accounts || []).filter(
        (sa) => !excluded.has(sa),
      );
      if (probe.connected) {
        const base = currentHostRef.current;
        const updated: HostInfo = {
          ...base,
          status: HostStatus.CONNECTED,
          is_online: true,
          k8s_version: probe.server_version || base.k8s_version,
          k8s_permission_mode: probe.permission_mode || base.k8s_permission_mode,
        };
        setCurrentHost(updated);
        onConnectedRef.current?.(updated);
      }
      if (
        probe.permission_mode === 'minimal' &&
        !defaultSARef.current &&
        candidates.includes('seatunnel-runner')
      ) {
        setSelectedSA('seatunnel-runner');
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
  }, [host.id]);

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

  const handleSaveDefaultSA = async () => {
    const nextSA = selectedSA.trim();
    if (!nextSA) {
      return;
    }
    setSavingSA(true);
    try {
      const res = await services.host.updateHostSafe(currentHost.id, {
        k8s_default_service_account: nextSA,
      });
      if (res.success) {
        const updated: HostInfo = {
          ...currentHost,
          ...(res.data || {}),
          k8s_default_service_account: nextSA,
        };
        setCurrentHost(updated);
        setSavedSAHint(true);
        onConnectedRef.current?.(updated);
      }
    } finally {
      setSavingSA(false);
    }
  };

  const rbacItems = useMemo(() => result?.rbac || [], [result]);
  const requiredItems = rbacItems.filter((item) => item.scope !== 'extended');
  const extendedItems = rbacItems.filter((item) => item.scope === 'extended');
  const requiredOk = requiredItems.filter((item) => item.allowed).length;
  const extendedOk = extendedItems.filter((item) => item.allowed).length;
  const deniedRequiredLabels = requiredItems
    .filter((item) => !item.allowed)
    .map((item) => `${item.resource}(${item.verb})`);

  const namespace = currentHost.k8s_namespace || 'default';
  const configuredSA = currentHost.k8s_default_service_account?.trim() || '';
  const configuredSAReady =
    Boolean(configuredSA) &&
    (candidateSAs.length === 0 || candidateSAs.includes(configuredSA));
  const isBusy = testing || showLoader;
  const failureHint = getK8sConnectErrorHint(result?.error_code, t);

  return (
    <div className='space-y-4' data-testid='k8s-connect-guide'>
      {/* 单层扁平头部摘要：用 0.5px 细线分隔，等待期不在徽章处重复旋转 spinner / Flat hairline summary row */}
      <div className='flex flex-wrap items-center justify-between gap-2 border-b-[0.5px] border-border/80 pb-3 text-xs'>
        <div className='flex min-w-0 items-center gap-2'>
          <Cloud className='size-4 shrink-0 text-primary' aria-hidden />
          <span className='font-medium text-foreground'>{currentHost.name}</span>
          <span className='truncate font-mono text-muted-foreground'>
            {currentHost.k8s_api_url || '-'} · ns: {namespace}
          </span>
        </div>
        {!isBusy ? (
          <div className='flex flex-wrap items-center gap-1.5'>
            {cancelled ? (
              <Badge variant='outline'>{t('host.k8sGuide.cancelTest')}</Badge>
            ) : result?.connected ? (
              <>
                <Badge
                  variant='default'
                  className='gap-1 border-emerald-500/30 bg-emerald-500/15 font-medium text-emerald-700 dark:text-emerald-300'
                >
                  <Check className='size-3' aria-hidden />
                  <span className='tabular-nums'>
                    {t('host.k8sConnected')} · {result.server_version || ''}
                    {result.elapsed_ms ? ` · ${result.elapsed_ms}ms` : ''}
                  </span>
                </Badge>
                {result.permission_mode ? (
                  <Badge
                    variant={
                      result.permission_mode === 'insufficient'
                        ? 'destructive'
                        : 'outline'
                    }
                    data-testid='k8s-guide-permission-mode'
                  >
                    {getK8sPermissionModeLabel(result.permission_mode, t)}
                  </Badge>
                ) : null}
              </>
            ) : (
              <Badge variant='destructive'>{t('host.k8sConnectFailed')}</Badge>
            )}
          </div>
        ) : null}
      </div>

      {/* 主体结果区：预留最小高度防止内容到达时弹窗跳动 / Reserved min-height region to avoid layout jump */}
      <div
        className='min-h-[112px]'
        aria-busy={isBusy}
        aria-live='polite'
      >
        {isBusy ? (
          showLoader ? (
            <div className='space-y-2.5 py-2 text-xs'>
              <div className='flex items-center justify-between gap-2'>
                <span className='inline-flex items-center gap-2 text-muted-foreground'>
                  <RefreshCw
                    className='size-3.5 shrink-0 animate-spin text-primary'
                    aria-hidden
                  />
                  <span>
                    {elapsedSec >= 3
                      ? t('host.k8sGuide.testingStageRbac')
                      : t('host.k8sGuide.testing')}
                  </span>
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
              {elapsedSec >= 10 ? (
                <p className='text-amber-700 dark:text-amber-300'>
                  {t('host.k8sGuide.slowHint')}
                </p>
              ) : null}
            </div>
          ) : null
        ) : cancelled ? (
          <div className='py-2 text-xs text-muted-foreground'>
            {t('host.k8sGuide.cancelledHint')}
          </div>
        ) : !result?.connected ? (
          <div className='space-y-2 rounded-md bg-rose-500/8 px-3 py-2.5 text-xs'>
            <p className='font-medium text-rose-600 dark:text-rose-400'>
              {failureHint}
            </p>
            <p className='font-mono text-[11px] text-muted-foreground break-words'>
              {result?.message || errorMsg || t('host.k8sConnectFailed')}
            </p>
          </div>
        ) : result.permission_mode === 'insufficient' ? (
          <div className='space-y-3'>
            <div className='flex items-start gap-2 rounded-md bg-rose-500/8 px-3 py-2.5 text-xs'>
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
              runnerServiceAccount={selectedSA || 'seatunnel-runner'}
              authMode={currentHost.k8s_auth_mode}
              defaultOpen
            />
          </div>
        ) : result.permission_mode === 'minimal' ? (
          <div className='space-y-3'>
            <div className='flex items-start gap-2 rounded-md bg-amber-500/8 px-3 py-2.5 text-xs'>
              <ShieldCheck
                className='mt-0.5 size-4 shrink-0 text-amber-600'
                aria-hidden
              />
              <div className='space-y-1'>
                <p className='font-medium text-foreground'>
                  {t('host.k8sGuide.minimalTitle')}
                </p>
                <p className='text-muted-foreground'>
                  {t('host.k8sGuide.minimalDesc')}
                </p>
              </div>
            </div>

            {configuredSAReady ? (
              <p className='flex items-center gap-1.5 text-xs text-emerald-700 dark:text-emerald-300'>
                <Check className='size-3.5 shrink-0' aria-hidden />
                <span>
                  {t('host.k8sGuide.minimalSaReady', {sa: configuredSA})}
                </span>
              </p>
            ) : (
              <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3'>
                <p className='text-xs font-medium text-foreground'>
                  {t('host.k8sGuide.minimalSaSelectLabel')}
                </p>
                <div className='flex flex-wrap items-end gap-2'>
                  <div className='min-w-[220px] flex-1'>
                    {candidateSAs.length > 0 ? (
                      <Select
                        label={t('host.k8sDefaultServiceAccount')}
                        value={selectedSA}
                        onValueChange={(v) => {
                          setSelectedSA(v);
                          setSavedSAHint(false);
                        }}
                        placeholder={t('host.k8sGuide.minimalSaPlaceholder')}
                        options={candidateSAs.map((sa) => ({value: sa, label: sa}))}
                        data-testid='k8s-guide-sa-select'
                      />
                    ) : (
                      <Input
                        label={t('host.k8sDefaultServiceAccount')}
                        value={selectedSA}
                        onChange={(e) => {
                          setSelectedSA(e.target.value);
                          setSavedSAHint(false);
                        }}
                        placeholder='seatunnel-runner'
                        className='font-mono text-xs'
                        data-testid='k8s-guide-sa-input'
                      />
                    )}
                  </div>
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    loading={savingSA}
                    disabled={savingSA || !selectedSA.trim()}
                    onClick={() => void handleSaveDefaultSA()}
                    data-testid='k8s-guide-save-sa'
                  >
                    {t('host.k8sGuide.minimalSaSave')}
                  </Button>
                </div>
                {savedSAHint ? (
                  <p className='text-xs text-emerald-600'>
                    {t('host.k8sGuide.minimalSaSaved')}
                  </p>
                ) : null}
                {candidateSAs.length === 0 ? (
                  <p className='text-[11px] text-muted-foreground'>
                    {t('host.k8sGuide.minimalSaMissingHint')}
                  </p>
                ) : null}
              </div>
            )}

            <K8sRBACTemplateSection
              namespace={namespace}
              runnerServiceAccount={
                configuredSA || selectedSA.trim() || 'seatunnel-runner'
              }
              authMode={currentHost.k8s_auth_mode}
            />
          </div>
        ) : (
          <div className='flex items-start gap-2 rounded-md bg-emerald-500/8 px-3 py-2.5 text-xs'>
            <ShieldCheck
              className='mt-0.5 size-4 shrink-0 text-emerald-600'
              aria-hidden
            />
            <div className='space-y-1'>
              <p className='font-medium text-emerald-700 dark:text-emerald-300'>
                {t('host.k8sGuide.fullTitle')}
              </p>
              <p className='text-muted-foreground'>
                {t('host.k8sGuide.fullDesc')}
              </p>
            </div>
          </div>
        )}
      </div>

      {/* 可折叠 RBAC 检查明细表：直接落于画布并用 0.5px 细线分行 / Flat canvas table */}
      {!isBusy && rbacItems.length > 0 ? (
        <div className='border-t-[0.5px] border-border/80 pt-3'>
          <button
            type='button'
            className='text-xs font-medium text-muted-foreground hover:text-foreground'
            onClick={() => setDetailsOpen((open) => !open)}
            aria-expanded={detailsOpen}
            data-testid='k8s-guide-rbac-details-toggle'
          >
            {detailsOpen ? '▾' : '▸'}{' '}
            {t('host.k8sGuide.rbacDetailsToggle', {
              requiredOk,
              requiredTotal: requiredItems.length,
              extendedOk,
              extendedTotal: extendedItems.length,
            })}
          </button>
          {detailsOpen ? (
            <div className='mt-2 max-h-56 overflow-auto text-xs'>
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
                    const isExtended = item.scope === 'extended';
                    const cls = item.allowed
                      ? 'text-emerald-600'
                      : isExtended
                        ? 'text-muted-foreground'
                        : 'text-rose-600';
                    return (
                      <tr key={idx} className='last:border-0'>
                        <td className='px-2 py-1 font-mono'>{item.resource}</td>
                        <td className='px-2 py-1'>{item.verb}</td>
                        <td className='px-2 py-1 text-muted-foreground'>
                          {isExtended
                            ? t('host.k8sRbacScopeExtended')
                            : t('host.k8sRbacScopeRequired')}
                        </td>
                        <td className={`px-2 py-1 ${cls}`}>
                          {item.allowed
                            ? t('host.k8sRbacAllowed')
                            : isExtended
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

      {/* 底部操作栏 / Footer actions */}
      <div className='flex flex-wrap items-center justify-between gap-2 border-t-[0.5px] border-border/80 pt-3'>
        <div className='flex items-center gap-2'>
          {onBackToEdit ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={handleBackToEdit}
              data-testid='k8s-guide-back-edit'
            >
              <Wrench className='size-3.5' aria-hidden />
              {t('host.k8sGuide.backToEdit')}
            </Button>
          ) : null}
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => void runProbe()}
            disabled={isBusy}
            data-testid='k8s-guide-retest'
          >
            <RefreshCw className='size-3.5' aria-hidden />
            {t('host.k8sGuide.retest')}
          </Button>
        </div>
        <div className='flex items-center gap-2'>
          {onViewDetail ? (
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
        </div>
      </div>
    </div>
  );
}
