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
 * 主机详情与原地编辑体：供 DetailHost 插槽使用（支持只读浏览 ↔ 原地编辑切换 + 未保存脏态上报）。
 * Host detail & in-place edit body for the DetailHost slot (view ↔ edit toggle with dirty state guard).
 */

import {useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Label} from '@/components/ui/label';
import {Progress} from '@/components/ui/progress';
import {Skeleton} from '@/components/ui/skeleton';
import {Textarea} from '@/components/ui/textarea';
import {
  Clock,
  Cpu,
  HardDrive,
  MemoryStick,
  Pencil,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Switch} from '@/components/arc/switch/switch';
import {useBeforeUnload} from '@/components/common/layout';
import services from '@/lib/services';
import {toast} from '@/lib/toast';
import {
  HostInfo,
  HostStatus,
  HostType,
  K8sAuthMode,
  K8sTestConnectionResult,
  UpdateHostRequest,
} from '@/lib/services/host/types';
import {
  looksTruncatedK8sToken,
  renderK8sExportKubeconfigCommand,
  sanitizeK8sToken,
} from '@/lib/k8s-rbac-templates';
import {formatDateTimeTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import layout from '@/components/common/layout/list-detail.module.css';
import {K8sRBACTemplateSection} from './K8sRBACTemplateSection';
import {
  getHostStatusLabel,
  getK8sConnectErrorHint,
  getK8sPermissionModeLabel,
} from './host-status';

interface HostDetailProps {
  host: HostInfo | null;
  loading?: boolean;
  editing: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  onUpdated?: (updatedHost?: HostInfo) => void;
}

type EditFieldErrorKey =
  | 'name'
  | 'ip_address'
  | 'docker_api_url'
  | 'k8s_kubeconfig'
  | 'k8s_api_url'
  | 'k8s_token';

function formatBytes(bytes: number | undefined): string {
  if (!bytes) {
    return '-';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let unitIndex = 0;
  let value = bytes;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return `${value.toFixed(1)} ${units[unitIndex]}`;
}

function formatDateTime(dateStr: string | null | undefined): string {
  if (!dateStr) {
    return '-';
  }
  return formatDateTimeTz(dateStr);
}

function getStatusBadgeVariant(
  status: HostStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case HostStatus.CONNECTED:
      return 'default';
    case HostStatus.PENDING:
      return 'secondary';
    case HostStatus.OFFLINE:
      return 'outline';
    case HostStatus.ERROR:
      return 'destructive';
    default:
      return 'secondary';
  }
}

/**
 * 从安装命令生成卸载命令，移除安装专属参数（如 --host-id、--auth-token）
 * Generate uninstall command from install command, stripping install-only flags
 */
function getUninstallCommand(installCmd: string): string {
  if (!installCmd) {
    return '';
  }
  return installCmd
    .replace('/install.sh', '/uninstall.sh')
    .replace(/\s+--host-id(?:=\S+|\s+\S+)/g, '')
    .replace(/\s+--auth-token(?:=\S+|\s+\S+)/g, '')
    .replace(/\s+--control-plane-addr(?:=\S+|\s+\S+)/g, '')
    .trim();
}

function buildEditForm(host: HostInfo | null): UpdateHostRequest {
  if (!host) {
    return {};
  }
  return {
    name: host.name,
    description: host.description || '',
    ip_address: host.ip_address || '',
    docker_api_url: host.docker_api_url || '',
    docker_tls_enabled: Boolean(host.docker_tls_enabled),
    docker_cert_path: '',
    k8s_api_url: host.k8s_api_url || '',
    k8s_namespace: host.k8s_namespace || 'default',
    k8s_default_service_account: host.k8s_default_service_account || '',
    k8s_auth_mode: host.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig',
    k8s_kubeconfig: '',
    k8s_token: '',
    k8s_ca_cert: '',
    k8s_insecure_skip_tls: Boolean(host.k8s_insecure_skip_tls),
  };
}

export function HostDetail({
  host,
  loading = false,
  editing,
  onStartEdit,
  onCancelEdit,
  onDirtyChange,
  onUpdated,
}: HostDetailProps) {
  const t = useTranslations();
  const [k8sTesting, setK8sTesting] = useState(false);
  const [k8sTestResult, setK8sTestResult] =
    useState<K8sTestConnectionResult | null>(null);
  const [installCommand, setInstallCommand] = useState<string>('');
  const [loadingCommand, setLoadingCommand] = useState(false);

  const [saving, setSaving] = useState(false);
  const [formData, setFormData] = useState<UpdateHostRequest>(() =>
    buildEditForm(host),
  );
  const [baseline, setBaseline] = useState<UpdateHostRequest>(() =>
    buildEditForm(host),
  );
  const [fieldErrors, setFieldErrors] = useState<
    Partial<Record<EditFieldErrorKey, string>>
  >({});

  const k8sAbortRef = useRef<AbortController | null>(null);
  const lastHostIdRef = useRef<number | null>(host?.id ?? null);
  const wasEditingRef = useRef<boolean>(editing);

  const k8sAuthMode: K8sAuthMode =
    formData.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig';
  const exportKubeconfigCmd = renderK8sExportKubeconfigCommand();

  const isDirty = useMemo(() => {
    if (!editing || !host) {
      return false;
    }
    return JSON.stringify(formData) !== JSON.stringify(baseline);
  }, [editing, host, formData, baseline]);

  useBeforeUnload(isDirty);

  useEffect(() => {
    onDirtyChange?.(isDirty);
  }, [isDirty, onDirtyChange]);

  // 切换主机、刚进入编辑态或尚未修改时收到后台最新详情，均同步表单初值，避免覆写用户已输入内容。
  // Re-seed form on host switch, entering edit mode, or clean background refresh without clobbering dirty input.
  useEffect(() => {
    if (!host) {
      return;
    }
    const hostChanged = lastHostIdRef.current !== host.id;
    const enteredEdit = editing && !wasEditingRef.current;
    lastHostIdRef.current = host.id;
    wasEditingRef.current = editing;
    if (hostChanged || enteredEdit || !isDirty) {
      const next = buildEditForm(host);
      setFormData(next);
      setBaseline(next);
      setFieldErrors({});
      if (hostChanged) {
        setK8sTestResult(null);
      }
    }
  }, [host, editing, isDirty]);

  useEffect(() => {
    return () => {
      k8sAbortRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (!host || host.host_type !== HostType.BARE_METAL) {
      setInstallCommand('');
      return;
    }
    let cancelled = false;
    setLoadingCommand(true);
    void services.host
      .getInstallCommandSafe(host.id)
      .then((result) => {
        if (cancelled) {
          return;
        }
        if (result.success && result.data) {
          setInstallCommand(result.data.command);
        } else {
          setInstallCommand('');
        }
      })
      .catch(() => {
        if (!cancelled) {
          setInstallCommand('');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingCommand(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [host]);

  const clearFieldError = (key: EditFieldErrorKey) => {
    setFieldErrors((prev) => {
      if (!prev[key]) {
        return prev;
      }
      const next = {...prev};
      delete next[key];
      return next;
    });
  };

  const runK8sTest = async () => {
    if (!host) {
      return;
    }
    k8sAbortRef.current?.abort();
    const controller = new AbortController();
    k8sAbortRef.current = controller;
    setK8sTesting(true);
    try {
      const result = await services.host.testK8sConnection(host.id, {
        signal: controller.signal,
        timeoutMs: 20000,
      });
      if (controller.signal.aborted) {
        return;
      }
      setK8sTestResult(result);
      if (result.connected) {
        onUpdated?.();
      }
    } catch (e) {
      if (controller.signal.aborted) {
        return;
      }
      toast.error((e as Error).message);
    } finally {
      if (k8sAbortRef.current === controller) {
        setK8sTesting(false);
      }
    }
  };

  const validateEditForm = (): boolean => {
    if (!host) {
      return false;
    }
    const errors: Partial<Record<EditFieldErrorKey, string>> = {};
    if (formData.name !== undefined && formData.name.trim().length === 0) {
      errors.name = t('host.errors.nameRequired');
    }

    if (host.host_type === HostType.BARE_METAL && formData.ip_address !== undefined) {
      if (formData.ip_address.trim().length === 0) {
        errors.ip_address = t('host.errors.ipRequired');
      } else {
        const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$|^([a-fA-F0-9:]+)$/;
        if (!ipRegex.test(formData.ip_address.trim())) {
          errors.ip_address = t('host.errors.ipInvalid');
        }
      }
    }

    if (host.host_type === HostType.DOCKER && formData.docker_api_url !== undefined) {
      if (formData.docker_api_url.trim().length === 0) {
        errors.docker_api_url = t('host.errors.dockerUrlRequired');
      } else if (
        !formData.docker_api_url.startsWith('tcp://') &&
        !formData.docker_api_url.startsWith('unix://')
      ) {
        errors.docker_api_url = t('host.errors.dockerUrlInvalid');
      }
    }

    if (host.host_type === HostType.KUBERNETES) {
      if (k8sAuthMode === 'kubeconfig') {
        if (
          !host.has_k8s_kubeconfig &&
          (!formData.k8s_kubeconfig || formData.k8s_kubeconfig.trim().length === 0)
        ) {
          errors.k8s_kubeconfig = t('host.errors.k8sKubeconfigRequired');
        }
      } else {
        const apiUrl = (formData.k8s_api_url || '').trim();
        if (apiUrl.length === 0) {
          errors.k8s_api_url = t('host.errors.k8sUrlRequired');
        } else if (
          !apiUrl.startsWith('https://') &&
          !apiUrl.startsWith('http://')
        ) {
          errors.k8s_api_url = t('host.errors.k8sUrlInvalid');
        }
        if (
          !host.has_k8s_token &&
          sanitizeK8sToken(formData.k8s_token).length === 0
        ) {
          errors.k8s_token = t('host.errors.k8sTokenRequired');
        }
      }
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSave = async () => {
    if (!host || !validateEditForm()) {
      return;
    }

    setSaving(true);
    try {
      let payload: UpdateHostRequest = formData;
      if (host.host_type === HostType.BARE_METAL) {
        payload = {
          name: formData.name?.trim(),
          description: formData.description,
          ip_address: formData.ip_address?.trim(),
        };
      } else if (host.host_type === HostType.DOCKER) {
        payload = {
          name: formData.name?.trim(),
          description: formData.description,
          docker_api_url: formData.docker_api_url?.trim(),
          docker_tls_enabled: Boolean(formData.docker_tls_enabled),
          ...(formData.docker_tls_enabled && formData.docker_cert_path?.trim()
            ? {docker_cert_path: formData.docker_cert_path.trim()}
            : {}),
        };
      } else if (host.host_type === HostType.KUBERNETES) {
        // 仅提交当前认证方式对应字段，防止切换模式时另一模式的空/旧值污染请求。
        // Send only fields belonging to the active auth mode so switching modes stays clean.
        const nextDefaultSA = formData.k8s_default_service_account?.trim() || '';
        const prevDefaultSA = (host.k8s_default_service_account || '').trim();
        const defaultSAPatch =
          nextDefaultSA !== prevDefaultSA
            ? {k8s_default_service_account: nextDefaultSA}
            : {};
        if (k8sAuthMode === 'kubeconfig') {
          payload = {
            name: formData.name?.trim(),
            description: formData.description,
            k8s_auth_mode: 'kubeconfig',
            k8s_namespace: formData.k8s_namespace?.trim() || 'default',
            ...defaultSAPatch,
            ...(formData.k8s_kubeconfig?.trim()
              ? {k8s_kubeconfig: formData.k8s_kubeconfig.trim()}
              : {}),
          };
        } else {
          const skipTLS = Boolean(formData.k8s_insecure_skip_tls);
          const cleanedToken = sanitizeK8sToken(formData.k8s_token);
          payload = {
            name: formData.name?.trim(),
            description: formData.description,
            k8s_auth_mode: 'token',
            k8s_api_url: formData.k8s_api_url?.trim(),
            k8s_namespace: formData.k8s_namespace?.trim() || 'default',
            ...defaultSAPatch,
            k8s_insecure_skip_tls: skipTLS,
            ...(cleanedToken ? {k8s_token: cleanedToken} : {}),
            ...(!skipTLS && formData.k8s_ca_cert?.trim()
              ? {k8s_ca_cert: formData.k8s_ca_cert.trim()}
              : {}),
          };
        }
      }

      const result = await services.host.updateHostSafe(host.id, payload);
      if (result.success) {
        const nextHost = result.data;
        const resetSeed = buildEditForm(nextHost || host);
        setFormData(resetSeed);
        setBaseline(resetSeed);
        setFieldErrors({});
        onDirtyChange?.(false);
        toast.success(t('host.updateSuccess'));
        onUpdated?.(nextHost);
      } else {
        toast.error(result.error || t('host.updateError'));
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('host.updateError'));
    } finally {
      setSaving(false);
    }
  };

  const editTokenLooksTruncated = looksTruncatedK8sToken(formData.k8s_token);

  return (
    <div className={layout.detailCol} data-testid='host-detail'>
      <div className={layout.detailScroll}>
        {loading && !host ? (
          <div className='space-y-3 p-1'>
            <Skeleton className='h-16 w-full' />
            <Skeleton className='h-28 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        ) : host ? (
          editing ? (
            /* 原地编辑态：复用 DetailHost 单层画布，按 0.5px 细线分隔，不再叠加模态弹窗。
               In-place edit mode: flat single canvas inside DetailHost instead of stacking a modal dialog. */
            <div className='space-y-4' data-testid='host-detail-edit-form'>
              <section className={cn(layout.detailSection, layout.detailSectionFirst)}>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant={getStatusBadgeVariant(host.status)}>
                    {getHostStatusLabel(host, t)}
                  </Badge>
                  <Badge
                    variant='outline'
                    className='font-mono text-xs text-muted-foreground'
                  >
                    {t(
                      `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                    )}
                  </Badge>
                </div>
              </section>

              <section className={layout.detailSection}>
                <h3 className={layout.sectionTitle}>{t('host.basicInfo')}</h3>
                <div className='space-y-4'>
                  <Input
                    id='edit-name'
                    label={`${t('host.name')} *`}
                    value={formData.name || ''}
                    error={fieldErrors.name}
                    onChange={(e) => {
                      setFormData({...formData, name: e.target.value});
                      clearFieldError('name');
                    }}
                    placeholder={t('host.namePlaceholder')}
                  />

                  <div className='space-y-1.5'>
                    <Label htmlFor='edit-description'>
                      {t('host.descriptionLabel')}
                    </Label>
                    <Textarea
                      id='edit-description'
                      value={formData.description || ''}
                      onChange={(e) =>
                        setFormData({...formData, description: e.target.value})
                      }
                      placeholder={t('host.descriptionPlaceholder')}
                      rows={2}
                    />
                  </div>
                </div>
              </section>

              <section className={layout.detailSection}>
                <h3 className={layout.sectionTitle}>{t('host.connectionInfo')}</h3>
                <div className='space-y-4'>
                  {host.host_type === HostType.BARE_METAL ? (
                    <Input
                      id='edit-ip_address'
                      label={`${t('host.ipAddress')} *`}
                      value={formData.ip_address || ''}
                      error={fieldErrors.ip_address}
                      onChange={(e) => {
                        setFormData({...formData, ip_address: e.target.value});
                        clearFieldError('ip_address');
                      }}
                      placeholder='192.168.1.100'
                    />
                  ) : null}

                  {host.host_type === HostType.DOCKER ? (
                    <>
                      <Input
                        id='edit-docker_api_url'
                        label={`${t('host.dockerApiUrl')} *`}
                        description={t('host.dockerApiUrlHint')}
                        value={formData.docker_api_url || ''}
                        error={fieldErrors.docker_api_url}
                        onChange={(e) => {
                          setFormData({
                            ...formData,
                            docker_api_url: e.target.value,
                          });
                          clearFieldError('docker_api_url');
                        }}
                        placeholder='tcp://192.168.1.100:2375'
                      />
                      <Switch
                        id='edit-docker_tls_enabled'
                        label={t('host.tlsEnabled')}
                        checked={formData.docker_tls_enabled || false}
                        onCheckedChange={(checked) =>
                          setFormData({...formData, docker_tls_enabled: checked})
                        }
                      />
                      {formData.docker_tls_enabled ? (
                        <Input
                          id='edit-docker_cert_path'
                          label={t('host.certPath')}
                          value={formData.docker_cert_path || ''}
                          onChange={(e) =>
                            setFormData({
                              ...formData,
                              docker_cert_path: e.target.value,
                            })
                          }
                          placeholder='/path/to/certs'
                        />
                      ) : null}
                    </>
                  ) : null}

                  {host.host_type === HostType.KUBERNETES ? (
                    <>
                      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
                        <div className='space-y-1.5'>
                          <Label>{t('host.k8sAuthMode')}</Label>
                          <SegmentedControl
                            label={t('host.k8sAuthMode')}
                            value={k8sAuthMode}
                            onValueChange={(v) => {
                              setFormData({
                                ...formData,
                                k8s_auth_mode: v as K8sAuthMode,
                              });
                              setFieldErrors((prev) => ({name: prev.name}));
                            }}
                            options={[
                              {value: 'kubeconfig', label: 'Kubeconfig'},
                              {value: 'token', label: 'Token'},
                            ]}
                          />
                        </div>
                        <Input
                          id='edit-k8s_namespace'
                          label={t('host.namespace')}
                          value={formData.k8s_namespace || ''}
                          onChange={(e) =>
                            setFormData({
                              ...formData,
                              k8s_namespace: e.target.value,
                            })
                          }
                          placeholder='default'
                          className='font-mono text-xs'
                        />
                      </div>

                      <div className='space-y-1'>
                        <Input
                          id='edit-k8s_default_service_account'
                          label={t('host.k8sDefaultServiceAccount')}
                          value={formData.k8s_default_service_account || ''}
                          onChange={(e) =>
                            setFormData({
                              ...formData,
                              k8s_default_service_account: e.target.value,
                            })
                          }
                          placeholder='seatunnel-runner'
                          className='font-mono text-xs'
                        />
                        <p className='text-[11px] text-muted-foreground'>
                          {t('host.k8sDefaultServiceAccountHint')}
                        </p>
                      </div>

                      {k8sAuthMode === 'kubeconfig' ? (
                        <>
                          {host.k8s_api_url ? (
                            <div className='flex items-baseline justify-between gap-3 border-b-[0.5px] border-border/80 pb-2 text-xs'>
                              <span className='shrink-0 text-muted-foreground'>
                                {t('host.k8sApiUrl')}
                              </span>
                              <span className='truncate font-mono text-foreground'>
                                {host.k8s_api_url}
                              </span>
                            </div>
                          ) : null}
                          <div className='space-y-2'>
                            <Label htmlFor='edit-k8s_kubeconfig'>
                              {host.has_k8s_kubeconfig
                                ? t('host.kubeconfig')
                                : `${t('host.kubeconfig')} *`}
                            </Label>
                            <Textarea
                              id='edit-k8s_kubeconfig'
                              value={formData.k8s_kubeconfig || ''}
                              aria-invalid={Boolean(fieldErrors.k8s_kubeconfig)}
                              onChange={(e) => {
                                setFormData({
                                  ...formData,
                                  k8s_kubeconfig: e.target.value,
                                });
                                clearFieldError('k8s_kubeconfig');
                              }}
                              placeholder={t('host.kubeconfigPlaceholder')}
                              className='font-mono text-xs'
                              rows={6}
                            />
                            {fieldErrors.k8s_kubeconfig ? (
                              <p className='text-xs text-destructive'>
                                {fieldErrors.k8s_kubeconfig}
                              </p>
                            ) : (
                              <p className='text-xs text-muted-foreground'>
                                {host.has_k8s_kubeconfig
                                  ? t('host.k8sKubeconfigEditHint')
                                  : t('host.k8sKubeconfigAutoHint')}
                              </p>
                            )}
                            <div className='flex items-center justify-between gap-2 rounded-md bg-muted/50 px-2.5 py-1.5 text-[11px] dark:bg-muted/30'>
                              <div className='min-w-0 space-y-0.5'>
                                <div className='text-muted-foreground'>
                                  {t('host.k8sKubeconfigQuickCmdLabel')}
                                </div>
                                <code className='block truncate font-mono text-foreground'>
                                  {exportKubeconfigCmd}
                                </code>
                              </div>
                              <CopyButton
                                value={exportKubeconfigCmd}
                                label={t('common.copy')}
                                variant='plain'
                                className='h-7 shrink-0 px-2 text-xs'
                              />
                            </div>
                          </div>
                        </>
                      ) : (
                        <>
                          <Input
                            id='edit-k8s_api_url'
                            label={`${t('host.k8sApiUrl')} *`}
                            value={formData.k8s_api_url || ''}
                            error={fieldErrors.k8s_api_url}
                            onChange={(e) => {
                              setFormData({
                                ...formData,
                                k8s_api_url: e.target.value,
                              });
                              clearFieldError('k8s_api_url');
                            }}
                            placeholder='https://kubernetes.default.svc:6443'
                            className='font-mono text-xs'
                          />
                          <div className='space-y-1.5'>
                            <Label htmlFor='edit-k8s_token'>
                              {host.has_k8s_token
                                ? t('host.token')
                                : `${t('host.token')} *`}
                            </Label>
                            <Textarea
                              id='edit-k8s_token'
                              value={formData.k8s_token || ''}
                              aria-invalid={Boolean(fieldErrors.k8s_token)}
                              onChange={(e) => {
                                setFormData({
                                  ...formData,
                                  k8s_token: e.target.value,
                                });
                                clearFieldError('k8s_token');
                              }}
                              placeholder={t('host.tokenPlaceholder')}
                              className='font-mono text-xs'
                              rows={3}
                            />
                            {fieldErrors.k8s_token ? (
                              <p className='text-xs text-destructive'>
                                {fieldErrors.k8s_token}
                              </p>
                            ) : editTokenLooksTruncated ? (
                              <p className='text-xs text-amber-600 dark:text-amber-400'>
                                {t('host.k8sTokenTruncatedWarning')}
                              </p>
                            ) : host.has_k8s_token ? (
                              <p className='text-xs text-muted-foreground'>
                                {t('host.leaveEmptyToKeep')}
                              </p>
                            ) : null}
                          </div>
                          <Checkbox
                            id='edit-k8s_insecure_skip_tls'
                            checked={Boolean(formData.k8s_insecure_skip_tls)}
                            onCheckedChange={(checked) =>
                              setFormData({
                                ...formData,
                                k8s_insecure_skip_tls: checked === true,
                              })
                            }
                            label={t('host.k8sInsecureSkipTLS')}
                          />
                          {!formData.k8s_insecure_skip_tls ? (
                            <div className='space-y-1.5'>
                              <Label htmlFor='edit-k8s_ca_cert'>
                                {t('host.k8sCaCert')}
                              </Label>
                              <Textarea
                                id='edit-k8s_ca_cert'
                                value={formData.k8s_ca_cert || ''}
                                onChange={(e) =>
                                  setFormData({
                                    ...formData,
                                    k8s_ca_cert: e.target.value,
                                  })
                                }
                                placeholder={t('host.k8sCaCertPlaceholder')}
                                className='font-mono text-xs'
                                rows={3}
                              />
                              <p className='text-xs text-muted-foreground'>
                                {t('host.leaveEmptyToKeep')}
                              </p>
                            </div>
                          ) : null}
                        </>
                      )}

                      <K8sRBACTemplateSection
                        namespace={formData.k8s_namespace?.trim() || 'default'}
                        runnerServiceAccount={
                          formData.k8s_default_service_account?.trim() ||
                          'seatunnel-runner'
                        }
                        authMode={k8sAuthMode}
                      />
                    </>
                  ) : null}
                </div>
              </section>
            </div>
          ) : (
            <>
              {/* 单层扁平布局：DetailHost 已是唯一外框，各组信息仅用留白 + 0.5px 细线分隔。
                  Single-layer flat layout: DetailHost is the only frame; groups use whitespace + 0.5px hairlines. */}
              <section className={cn(layout.detailSection, layout.detailSectionFirst)}>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge
                    variant={getStatusBadgeVariant(host.status)}
                    className='gap-1.5'
                  >
                    {host.status === HostStatus.CONNECTED ? (
                      <span className='relative flex size-1.5'>
                        <span className='absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                        <span className='relative inline-flex size-1.5 rounded-full bg-emerald-500' />
                      </span>
                    ) : null}
                    {getHostStatusLabel(host, t)}
                  </Badge>
                  <Badge
                    variant='outline'
                    className='font-mono text-xs text-muted-foreground'
                  >
                    {t(
                      `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                    )}
                  </Badge>
                  {host.host_type === HostType.KUBERNETES && host.k8s_permission_mode ? (
                    <Badge
                      variant={
                        host.k8s_permission_mode === 'insufficient'
                          ? 'destructive'
                          : 'secondary'
                      }
                      className='text-[11px]'
                    >
                      {getK8sPermissionModeLabel(host.k8s_permission_mode, t)}
                    </Badge>
                  ) : null}
                </div>
                <p className='text-xs text-muted-foreground'>
                  {host.description || t('host.noDescription')}
                </p>
              </section>

              {host.host_type !== HostType.KUBERNETES ? (
                <section className={layout.detailSection}>
                  <div className={layout.detailSectionHead}>
                    <h3 className={layout.sectionTitle}>{t('host.resources')}</h3>
                    {host.last_check ? (
                      <span className='inline-flex items-center gap-1 text-xs text-muted-foreground'>
                        <Clock className='size-3' aria-hidden />
                        {formatDateTime(host.last_check)}
                      </span>
                    ) : null}
                  </div>
                  <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                    <div className='space-y-1.5'>
                      <div className='flex items-center justify-between text-xs'>
                        <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                          <Cpu className='size-3.5 text-blue-500' aria-hidden />
                          CPU
                        </span>
                        <span className='font-mono font-medium tabular-nums'>
                          {host.cpu_usage?.toFixed(1) || 0}%
                        </span>
                      </div>
                      <Progress value={host.cpu_usage || 0} className='h-1.5' />
                      <div className='truncate text-[11px] text-muted-foreground'>
                        {host.cpu_cores ? `${host.cpu_cores} ${t('host.cores')}` : '-'}
                      </div>
                    </div>
                    <div className='space-y-1.5'>
                      <div className='flex items-center justify-between text-xs'>
                        <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                          <MemoryStick
                            className='size-3.5 text-emerald-500'
                            aria-hidden
                          />
                          {t('host.memory')}
                        </span>
                        <span className='font-mono font-medium tabular-nums'>
                          {host.memory_usage?.toFixed(1) || 0}%
                        </span>
                      </div>
                      <Progress value={host.memory_usage || 0} className='h-1.5' />
                      <div className='truncate text-[11px] text-muted-foreground'>
                        {host.total_memory
                          ? `${formatBytes(host.total_memory)} ${t('host.total')}`
                          : '-'}
                      </div>
                    </div>
                    <div className='space-y-1.5'>
                      <div className='flex items-center justify-between text-xs'>
                        <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                          <HardDrive
                            className='size-3.5 text-purple-500'
                            aria-hidden
                          />
                          {t('host.disk')}
                        </span>
                        <span className='font-mono font-medium tabular-nums'>
                          {host.disk_usage?.toFixed(1) || 0}%
                        </span>
                      </div>
                      <Progress value={host.disk_usage || 0} className='h-1.5' />
                      <div className='truncate text-[11px] text-muted-foreground'>
                        {host.total_disk
                          ? `${formatBytes(host.total_disk)} ${t('host.total')}`
                          : '-'}
                      </div>
                    </div>
                  </div>
                </section>
              ) : null}

              <section className={layout.detailSection}>
                <h3 className={layout.sectionTitle}>{t('host.basicInfo')}</h3>
                <dl className={layout.detailKvGrid}>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.hostType')}</dt>
                    <dd className={layout.detailKvVal}>
                      {t(
                        `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                      )}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.status')}</dt>
                    <dd className={layout.detailKvVal}>
                      {getHostStatusLabel(host, t)}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.createdAt')}</dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                      {formatDateTime(host.created_at)}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.updatedAt')}</dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                      {formatDateTime(host.updated_at)}
                    </dd>
                  </div>
                </dl>
              </section>

              <section className={layout.detailSection}>
                <h3 className={layout.sectionTitle}>{t('host.connectionInfo')}</h3>
                {host.host_type === HostType.BARE_METAL ? (
                  <dl className={layout.detailKvGrid}>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>{t('host.ipAddress')}</dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.ip_address || '-'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('host.agentVersion')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.agent_version || '-'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>{t('host.osType')}</dt>
                      <dd className={layout.detailKvVal}>
                        {host.os_type
                          ? `${host.os_type} (${host.arch || 'x86_64'})`
                          : '-'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('host.lastHeartbeat')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                        {formatDateTime(host.last_heartbeat)}
                      </dd>
                    </div>
                  </dl>
                ) : null}
                {host.host_type === HostType.DOCKER ? (
                  <dl className={layout.detailKvGrid}>
                    <div className={cn(layout.detailKvItem, layout.detailKvSpan)}>
                      <dt className={layout.detailKvKey}>
                        {t('host.dockerApiUrl')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                        {host.docker_api_url || '-'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('host.tlsEnabled')}
                      </dt>
                      <dd className={layout.detailKvVal}>
                        {host.docker_tls_enabled ? t('common.yes') : t('common.no')}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('host.dockerVersion')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.docker_version || '-'}
                      </dd>
                    </div>
                  </dl>
                ) : null}
                {host.host_type === HostType.KUBERNETES ? (
                  <dl className={layout.detailKvGrid}>
                    <div className={cn(layout.detailKvItem, layout.detailKvSpan)}>
                      <dt className={layout.detailKvKey}>{t('host.k8sApiUrl')}</dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                        {host.k8s_api_url || '-'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>{t('host.k8sAuthMode')}</dt>
                      <dd className={layout.detailKvVal}>
                        {host.k8s_auth_mode === 'token' ? 'Token' : 'Kubeconfig'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>{t('host.namespace')}</dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.k8s_namespace || 'default'}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('host.k8sVersion')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.k8s_version || '-'}
                      </dd>
                    </div>
                    <div className={cn(layout.detailKvItem, layout.detailKvSpan)}>
                      <dt className={layout.detailKvKey}>
                        {t('host.k8sDefaultServiceAccount')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono')}>
                        {host.k8s_default_service_account || '-'}
                      </dd>
                    </div>
                  </dl>
                ) : null}
              </section>

              {host.host_type === HostType.KUBERNETES ? (
                <section className={layout.detailSection}>
                  <h3 className={layout.sectionTitle}>{t('host.testK8sConnection')}</h3>
                  <div className='space-y-3'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <Button
                        type='button'
                        size='sm'
                        variant='secondary'
                        loading={k8sTesting}
                        disabled={k8sTesting}
                        onClick={() => void runK8sTest()}
                      >
                        {t('host.testK8sConnection')}
                      </Button>
                      {k8sTestResult?.connected && k8sTestResult.permission_mode ? (
                        <Badge
                          variant={
                            k8sTestResult.permission_mode === 'insufficient'
                              ? 'destructive'
                              : 'secondary'
                          }
                          className='text-[11px]'
                        >
                          {getK8sPermissionModeLabel(
                            k8sTestResult.permission_mode,
                            t,
                          )}
                        </Badge>
                      ) : null}
                    </div>
                    {k8sTestResult ? (
                      <div className='space-y-2 text-xs'>
                        {k8sTestResult.connected ? (
                          <p className='text-emerald-600 tabular-nums'>
                            {`${t('host.k8sConnected')} · ${k8sTestResult.server_version || ''} · ${k8sTestResult.elapsed_ms}ms`}
                          </p>
                        ) : (
                          <div className='space-y-1 rounded-md bg-rose-500/8 px-3 py-2'>
                            <p className='font-medium text-rose-600 dark:text-rose-400'>
                              {getK8sConnectErrorHint(k8sTestResult.error_code, t)}
                            </p>
                            <p className='font-mono text-[11px] text-muted-foreground break-words'>
                              {k8sTestResult.message || ''}
                            </p>
                          </div>
                        )}
                        {k8sTestResult.rbac && k8sTestResult.rbac.length > 0 ? (
                          <div className='overflow-x-auto'>
                            <table className='w-full [&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                              <thead>
                                <tr className='text-left text-muted-foreground'>
                                  <th className='px-2 py-1 font-medium'>
                                    {t('host.k8sRbacResource')}
                                  </th>
                                  <th className='px-2 py-1 font-medium'>
                                    {t('host.k8sRbacVerb')}
                                  </th>
                                  <th className='px-2 py-1 font-medium'>
                                    {t('host.k8sRbacScope')}
                                  </th>
                                  <th className='px-2 py-1 font-medium'>
                                    {t('host.k8sRbacPrecheck')}
                                  </th>
                                </tr>
                              </thead>
                              <tbody>
                                {k8sTestResult.rbac.map((item, i) => {
                                  const isExtended = item.scope === 'extended';
                                  const statusClass = item.allowed
                                    ? 'text-emerald-600'
                                    : isExtended
                                      ? 'text-muted-foreground'
                                      : 'text-rose-600';
                                  return (
                                    <tr key={i} className='last:border-0'>
                                      <td className='px-2 py-1 font-mono'>
                                        {item.resource}
                                      </td>
                                      <td className='px-2 py-1'>{item.verb}</td>
                                      <td className='px-2 py-1 text-muted-foreground'>
                                        {isExtended
                                          ? t('host.k8sRbacScopeExtended')
                                          : t('host.k8sRbacScopeRequired')}
                                      </td>
                                      <td className={`px-2 py-1 ${statusClass}`}>
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
                    <K8sRBACTemplateSection
                      namespace={host.k8s_namespace || 'default'}
                      runnerServiceAccount={
                        host.k8s_default_service_account || 'seatunnel-runner'
                      }
                      authMode={host.k8s_auth_mode}
                    />
                  </div>
                </section>
              ) : null}

              {host.host_type === HostType.BARE_METAL ? (
                <section className={layout.detailSection}>
                  <h3 className={layout.sectionTitle}>
                    {t('host.installCommand')} / {t('host.uninstallCommand')}
                  </h3>
                  {host.status === HostStatus.PENDING ? (
                    <p className='flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300'>
                      <span className='mt-1.5 size-1.5 shrink-0 animate-pulse rounded-full bg-amber-500' />
                      <span>{t('host.installGuide.pendingAlert')}</span>
                    </p>
                  ) : null}
                  <div className='space-y-1.5'>
                    <div className='flex items-center justify-between text-xs text-muted-foreground'>
                      <span>{t('host.installCommand')}</span>
                      {installCommand ? (
                        <CopyButton
                          value={installCommand}
                          label={t('common.copy')}
                          variant='plain'
                          className='h-7 px-2 text-xs'
                        />
                      ) : null}
                    </div>
                    <pre className='m-0 select-all overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-foreground dark:bg-muted/30'>
                      {loadingCommand
                        ? `${t('common.loading')}...`
                        : installCommand || t('host.noInstallCommand')}
                    </pre>
                  </div>
                  {installCommand ? (
                    <div className='space-y-1.5'>
                      <div className='flex items-center justify-between text-xs text-muted-foreground'>
                        <span>{t('host.uninstallCommand')}</span>
                        <CopyButton
                          value={getUninstallCommand(installCommand)}
                          label={t('common.copy')}
                          variant='plain'
                          className='h-7 px-2 text-xs'
                        />
                      </div>
                      <pre className='m-0 select-all overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-muted-foreground dark:bg-muted/30'>
                        {getUninstallCommand(installCommand)}
                      </pre>
                      <p className='text-[11px] text-muted-foreground'>
                        {t('host.uninstallCommandTip')}
                      </p>
                    </div>
                  ) : null}
                </section>
              ) : null}
            </>
          )
        ) : (
          <p className='p-4 text-sm text-muted-foreground'>{t('host.loadError')}</p>
        )}
      </div>
      {host ? (
        <div className={layout.detailFoot}>
          <span className={layout.detailFootGrow} />
          {editing ? (
            <>
              <Button
                type='button'
                size='sm'
                variant='secondary'
                onClick={onCancelEdit}
                disabled={saving}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                loading={saving}
                disabled={saving}
                onClick={() => void handleSave()}
              >
                {t('common.save')}
              </Button>
            </>
          ) : (
            <Button type='button' size='sm' onClick={onStartEdit}>
              <Pencil className='size-3.5' aria-hidden />
              {t('common.edit')}
            </Button>
          )}
        </div>
      ) : null}
    </div>
  );
}
