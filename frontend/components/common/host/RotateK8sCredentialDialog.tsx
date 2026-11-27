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
 * Kubernetes 主机凭据安全轮换弹窗：
 * 先通过无状态预检接口（testK8sDraftConnection）验证新 Kubeconfig / Token 的连通性与 RBAC，
 * 仅在校验通过后启用「确认替换凭据」，防止错误凭据覆盖在线集群配置。
 * Safe Kubernetes credential rotation dialog: validates replacement credentials via stateless
 * testK8sDraftConnection before enabling the Apply action so invalid credentials never overwrite live hosts.
 */

import {useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, KeyRound} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import services from '@/lib/services';
import {toast} from '@/lib/toast';
import {
  HostInfo,
  K8sAuthMode,
  K8sTestConnectionResult,
  UpdateHostRequest,
} from '@/lib/services/host/types';
import {looksTruncatedK8sToken, sanitizeK8sToken} from '@/lib/k8s-rbac-templates';
import {formatDateTimeTz} from '@/lib/timezone';
import {K8sKubeconfigField} from './K8sKubeconfigField';
import {getK8sConnectErrorHint, getK8sPermissionModeLabel} from './host-status';

interface RotateK8sCredentialDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  host: HostInfo;
  onRotated?: (updatedHost: HostInfo) => void;
}

export function RotateK8sCredentialDialog({
  open,
  onOpenChange,
  host,
  onRotated,
}: RotateK8sCredentialDialogProps) {
  const t = useTranslations();
  const [authMode, setAuthMode] = useState<K8sAuthMode>(
    host.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig',
  );
  const [kubeconfig, setKubeconfig] = useState('');
  const [k8sContext, setK8sContext] = useState('');
  const [serverOverride, setServerOverride] = useState('');
  const [apiUrl, setApiUrl] = useState(host.k8s_api_url || '');
  const [token, setToken] = useState('');
  const [caCert, setCaCert] = useState('');
  const [skipTls, setSkipTls] = useState(Boolean(host.k8s_insecure_skip_tls));

  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [probeResult, setProbeResult] = useState<K8sTestConnectionResult | null>(
    null,
  );
  const [verified, setVerified] = useState(false);
  const [errorText, setErrorText] = useState('');
  const abortRef = useRef<AbortController | null>(null);

  const invalidateProbe = () => {
    setVerified(false);
    setProbeResult(null);
    setErrorText('');
  };

  const resetDialog = () => {
    abortRef.current?.abort();
    setAuthMode(host.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig');
    setKubeconfig('');
    setK8sContext('');
    setServerOverride('');
    setApiUrl(host.k8s_api_url || '');
    setToken('');
    setCaCert('');
    setSkipTls(Boolean(host.k8s_insecure_skip_tls));
    setTesting(false);
    setSaving(false);
    setProbeResult(null);
    setVerified(false);
    setErrorText('');
  };

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetDialog();
    }
    onOpenChange(nextOpen);
  };

  const handleTestNewCredential = async () => {
    setErrorText('');
    if (authMode === 'kubeconfig') {
      if (!kubeconfig.trim()) {
        setErrorText(t('host.errors.k8sKubeconfigRequired'));
        return;
      }
    } else {
      const trimmedUrl = apiUrl.trim();
      if (!trimmedUrl) {
        setErrorText(t('host.errors.k8sUrlRequired'));
        return;
      }
      if (!trimmedUrl.startsWith('https://') && !trimmedUrl.startsWith('http://')) {
        setErrorText(t('host.errors.k8sUrlInvalid'));
        return;
      }
      if (!sanitizeK8sToken(token)) {
        setErrorText(t('host.errors.k8sTokenRequired'));
        return;
      }
    }

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setTesting(true);
    setVerified(false);
    try {
      const res = await services.host.testK8sDraftConnection(
        authMode === 'kubeconfig'
          ? {
              host_id: host.id,
              k8s_auth_mode: 'kubeconfig',
              k8s_namespace: host.k8s_namespace || 'default',
              k8s_kubeconfig: kubeconfig.trim(),
              k8s_context: k8sContext.trim() || undefined,
              k8s_api_url: serverOverride.trim() || undefined,
            }
          : {
              host_id: host.id,
              k8s_auth_mode: 'token',
              k8s_namespace: host.k8s_namespace || 'default',
              k8s_api_url: apiUrl.trim(),
              k8s_token: sanitizeK8sToken(token),
              k8s_insecure_skip_tls: skipTls,
              k8s_ca_cert: skipTls ? '' : caCert.trim() || undefined,
            },
        {signal: controller.signal, timeoutMs: 20000},
      );
      if (controller.signal.aborted) {
        return;
      }
      setProbeResult(res);
      setVerified(Boolean(res.connected && res.permission_mode !== 'insufficient'));
    } catch (err) {
      if (!controller.signal.aborted) {
        setErrorText(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (abortRef.current === controller) {
        setTesting(false);
      }
    }
  };

  const handleApplyRotation = async () => {
    if (!verified) {
      return;
    }
    setSaving(true);
    try {
      const payload: UpdateHostRequest =
        authMode === 'kubeconfig'
          ? {
              k8s_auth_mode: 'kubeconfig',
              k8s_kubeconfig: kubeconfig.trim(),
              ...(k8sContext.trim() ? {k8s_context: k8sContext.trim()} : {}),
              ...(serverOverride.trim() ? {k8s_api_url: serverOverride.trim()} : {}),
            }
          : {
              k8s_auth_mode: 'token',
              k8s_api_url: apiUrl.trim(),
              k8s_token: sanitizeK8sToken(token),
              k8s_insecure_skip_tls: skipTls,
              ...(!skipTls && caCert.trim() ? {k8s_ca_cert: caCert.trim()} : {}),
            };
      const updateRes = await services.host.updateHostSafe(host.id, payload);
      if (!updateRes.success) {
        toast.error(updateRes.error || t('host.updateError'));
        return;
      }
      let nextHost = updateRes.data || host;
      try {
        await services.host.testK8sConnection(host.id, {timeoutMs: 20000});
        const fresh = await services.host.getHostSafe(host.id);
        if (fresh.success && fresh.data) {
          nextHost = fresh.data;
        }
      } catch {
        // 保存已成功但刷新探活瞬态失败时仍回传更新后的主机 / Return updated host even if refresh probe hits transient error.
      }
      toast.success(t('host.k8sRotateSuccess'));
      onRotated?.(nextHost);
      handleClose(false);
    } finally {
      setSaving(false);
    }
  };

  const truncated = looksTruncatedK8sToken(token);

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        title={t('host.k8sRotateTitle')}
        className='arc-compact w-[min(calc(100vw-2rem),600px)]!'
        bodyClassName='max-h-[min(85dvh,720px)] overflow-y-auto'
      >
        <div className='space-y-4' data-testid='k8s-rotate-dialog'>
          <p className='text-xs text-muted-foreground'>{t('host.k8sRotateDesc')}</p>

          <div className='space-y-1.5'>
            <Label>{t('host.k8sAuthMode')}</Label>
            <SegmentedControl
              label={t('host.k8sAuthMode')}
              value={authMode}
              onValueChange={(v) => {
                setAuthMode(v as K8sAuthMode);
                invalidateProbe();
              }}
              options={[
                {value: 'kubeconfig', label: 'Kubeconfig'},
                {value: 'token', label: 'Token'},
              ]}
            />
          </div>

          {authMode === 'kubeconfig' ? (
            <K8sKubeconfigField
              id='rotate_k8s_kubeconfig'
              label={`${t('host.kubeconfig')} *`}
              value={kubeconfig}
              onChange={(next) => {
                setKubeconfig(next);
                invalidateProbe();
              }}
              contextName={k8sContext}
              onContextChange={(ctx) => {
                setK8sContext(ctx);
                invalidateProbe();
              }}
              serverOverride={serverOverride}
              onServerOverrideChange={(url) => {
                setServerOverride(url);
                invalidateProbe();
              }}
              accessMode={host.k8s_access_mode || 'in_cluster'}
              hint={t('host.k8sKubeconfigAutoHint')}
              testIdPrefix='rotate-kubeconfig'
            />
          ) : (
            <div className='space-y-3'>
              <Input
                id='rotate_k8s_api_url'
                label={`${t('host.k8sApiUrl')} *`}
                value={apiUrl}
                onChange={(e) => {
                  setApiUrl(e.target.value);
                  invalidateProbe();
                }}
                placeholder='https://10.0.0.10:6443'
                className='font-mono text-xs'
              />
              <div className='space-y-1.5'>
                <Label htmlFor='rotate_k8s_token'>{`${t('host.token')} *`}</Label>
                <Textarea
                  id='rotate_k8s_token'
                  value={token}
                  onChange={(e) => {
                    setToken(e.target.value);
                    invalidateProbe();
                  }}
                  placeholder={t('host.tokenPlaceholder')}
                  className='font-mono text-xs'
                  rows={3}
                />
                {truncated ? (
                  <p className='text-xs text-amber-600 dark:text-amber-400'>
                    {t('host.k8sTokenTruncatedWarning')}
                  </p>
                ) : null}
              </div>
              <Checkbox
                id='rotate_k8s_skip_tls'
                checked={skipTls}
                onCheckedChange={(checked) => {
                  setSkipTls(checked === true);
                  invalidateProbe();
                }}
                label={t('host.k8sInsecureSkipTLS')}
              />
              {!skipTls ? (
                <div className='space-y-1.5'>
                  <Label htmlFor='rotate_k8s_ca_cert'>{t('host.k8sCaCert')}</Label>
                  <Textarea
                    id='rotate_k8s_ca_cert'
                    value={caCert}
                    onChange={(e) => {
                      setCaCert(e.target.value);
                      invalidateProbe();
                    }}
                    placeholder={t('host.k8sCaCertPlaceholder')}
                    className='font-mono text-xs'
                    rows={3}
                  />
                </div>
              ) : null}
            </div>
          )}

          {errorText ? (
            <p className='text-xs text-destructive'>{errorText}</p>
          ) : null}

          {probeResult ? (
            <div
              className='space-y-2 rounded-[var(--radius-control)] border-[0.5px] border-border/80 bg-muted/20 p-3 text-xs'
              data-testid='k8s-rotate-probe-result'
            >
              {probeResult.connected ? (
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge
                    variant='default'
                    className='gap-1 border-emerald-500/30 bg-emerald-500/15 font-medium text-emerald-700 dark:text-emerald-300'
                  >
                    <Check className='size-3' aria-hidden />
                    <span>
                      {t('host.k8sConnected')} · {probeResult.server_version || ''}
                    </span>
                  </Badge>
                  {probeResult.permission_mode ? (
                    <Badge
                      variant={
                        probeResult.permission_mode === 'insufficient'
                          ? 'destructive'
                          : 'secondary'
                      }
                    >
                      {getK8sPermissionModeLabel(probeResult.permission_mode, t)}
                    </Badge>
                  ) : null}
                  {probeResult.credential_expires_at ? (
                    <span className='font-mono text-muted-foreground tabular-nums'>
                      {t('host.k8sCredentialExpiry')}:{' '}
                      {formatDateTimeTz(probeResult.credential_expires_at)}
                    </span>
                  ) : null}
                </div>
              ) : (
                <div className='space-y-1 text-rose-600 dark:text-rose-400'>
                  <p className='font-medium'>
                    {getK8sConnectErrorHint(probeResult.error_code, t)}
                  </p>
                  <p className='font-mono text-[11px] text-muted-foreground break-words'>
                    {probeResult.message || ''}
                  </p>
                </div>
              )}
              {probeResult.connected &&
              probeResult.permission_mode === 'insufficient' ? (
                <p className='text-rose-600 dark:text-rose-400'>
                  {t('host.k8sGuide.insufficientTitle')}
                </p>
              ) : null}
            </div>
          ) : (
            <p className='text-xs text-muted-foreground'>
              {t('host.k8sRotateTestFirstHint')}
            </p>
          )}

          <div className='flex items-center justify-between gap-2 border-t-[0.5px] border-border/80 pt-3'>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              loading={testing}
              disabled={testing || saving}
              onClick={() => void handleTestNewCredential()}
              data-testid='k8s-rotate-test-btn'
            >
              <KeyRound className='size-3.5' aria-hidden />
              {t('host.k8sRotateTestNew')}
            </Button>
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={() => handleClose(false)}
                disabled={saving}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                loading={saving}
                disabled={!verified || saving}
                onClick={() => void handleApplyRotation()}
                data-testid='k8s-rotate-apply-btn'
              >
                {t('host.k8sRotateApply')}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
