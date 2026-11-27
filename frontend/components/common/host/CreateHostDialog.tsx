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
 * 新增主机两步引导弹窗：先通过分段控件确定主机类型，再按类型呈现精简必填字段与行内校验，
 * 提交后无缝进入第 2 步（物理机 Agent 部署引导 / K8s 连通与 RBAC 权限体检）。
 * Two-step Create Host dialog: select host type first via SegmentedControl, collect focused fields
 * with inline validation, then transition into Step 2 (bare-metal Agent guide or K8s RBAC probe).
 */

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  HostType,
  CreateHostRequest,
  UpdateHostRequest,
  HostInfo,
  K8sAuthMode,
} from '@/lib/services/host/types';
import {
  looksTruncatedK8sToken,
  renderK8sExportKubeconfigCommand,
  sanitizeK8sToken,
} from '@/lib/k8s-rbac-templates';
import {HostInstallGuideContent} from './HostInstallGuideContent';
import {K8sConnectGuideContent} from './K8sConnectGuideContent';
import {K8sRBACTemplateSection} from './K8sRBACTemplateSection';

const INITIAL_FORM_DATA: CreateHostRequest = {
  name: '',
  host_type: HostType.BARE_METAL,
  description: '',
  ip_address: '',
  ssh_port: 22,
  docker_api_url: '',
  docker_tls_enabled: false,
  docker_cert_path: '',
  k8s_api_url: '',
  k8s_namespace: 'default',
  k8s_default_service_account: '',
  k8s_kubeconfig: '',
  k8s_token: '',
  k8s_auth_mode: 'kubeconfig',
  k8s_ca_cert: '',
  k8s_insecure_skip_tls: false,
};

type CreateFieldErrorKey =
  | 'name'
  | 'ip_address'
  | 'k8s_kubeconfig'
  | 'k8s_api_url'
  | 'k8s_token';

interface CreateHostDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: (createdHost?: HostInfo) => void;
  onViewDetail?: (host: HostInfo) => void;
}

export function CreateHostDialog({
  open,
  onOpenChange,
  onSuccess,
  onViewDetail,
}: CreateHostDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [step, setStep] = useState<1 | 2>(1);
  const [createdHost, setCreatedHost] = useState<HostInfo | null>(null);
  const [optionalOpen, setOptionalOpen] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<
    Partial<Record<CreateFieldErrorKey, string>>
  >({});
  const [formData, setFormData] = useState<CreateHostRequest>(() => ({
    ...INITIAL_FORM_DATA,
  }));

  const k8sAuthMode: K8sAuthMode =
    formData.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig';
  const exportKubeconfigCmd = renderK8sExportKubeconfigCommand();

  const clearFieldError = (key: CreateFieldErrorKey) => {
    setFieldErrors((prev) => {
      if (!prev[key]) {
        return prev;
      }
      const next = {...prev};
      delete next[key];
      return next;
    });
  };

  const resetForm = () => {
    setFormData({...INITIAL_FORM_DATA});
    setFieldErrors({});
    setOptionalOpen(false);
    setStep(1);
    setCreatedHost(null);
  };

  const handleClose = () => {
    resetForm();
    onOpenChange(false);
  };

  // Tip 016：表单字段校验错误就地贴在对应字段下方，不再仅用消失式 Toast 提示。
  // Tip 016: surface validation errors inline on each offending field instead of ephemeral toasts.
  const validateForm = (): boolean => {
    const errors: Partial<Record<CreateFieldErrorKey, string>> = {};
    if (!formData.name || formData.name.trim().length === 0) {
      errors.name = t('host.errors.nameRequired');
    }

    if (formData.host_type === HostType.BARE_METAL) {
      if (!formData.ip_address || formData.ip_address.trim().length === 0) {
        errors.ip_address = t('host.errors.ipRequired');
      } else {
        const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$|^([a-fA-F0-9:]+)$/;
        if (!ipRegex.test(formData.ip_address.trim())) {
          errors.ip_address = t('host.errors.ipInvalid');
        }
      }
    }

    if (formData.host_type === HostType.KUBERNETES) {
      if (k8sAuthMode === 'kubeconfig') {
        if (!formData.k8s_kubeconfig || formData.k8s_kubeconfig.trim().length === 0) {
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
        if (sanitizeK8sToken(formData.k8s_token).length === 0) {
          errors.k8s_token = t('host.errors.k8sTokenRequired');
        }
      }
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) {
      return;
    }

    setLoading(true);
    try {
      const cleanedToken = sanitizeK8sToken(formData.k8s_token);
      // 从 Step 2「返回修改凭据」后再次提交，直接更新当前主机并重新进入连通体检，避免重复建主机。
      // When resubmitting after "Edit Credentials" from Step 2, update the existing host instead of creating a duplicate.
      if (createdHost && formData.host_type === HostType.KUBERNETES) {
        const defaultSA = formData.k8s_default_service_account?.trim() || '';
        const updatePayload: UpdateHostRequest =
          k8sAuthMode === 'kubeconfig'
            ? {
                name: formData.name.trim(),
                description: formData.description,
                k8s_auth_mode: 'kubeconfig',
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_kubeconfig: formData.k8s_kubeconfig?.trim(),
                k8s_default_service_account: defaultSA,
              }
            : {
                name: formData.name.trim(),
                description: formData.description,
                k8s_auth_mode: 'token',
                k8s_api_url: formData.k8s_api_url?.trim(),
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_token: cleanedToken,
                k8s_insecure_skip_tls: Boolean(formData.k8s_insecure_skip_tls),
                k8s_ca_cert: formData.k8s_insecure_skip_tls
                  ? ''
                  : formData.k8s_ca_cert?.trim() || '',
                k8s_default_service_account: defaultSA,
              };
        const result = await services.host.updateHostSafe(
          createdHost.id,
          updatePayload,
        );
        if (result.success) {
          const updated: HostInfo = result.data || {
            ...createdHost,
            ...updatePayload,
          };
          setCreatedHost(updated);
          onSuccess(updated);
          setStep(2);
        } else {
          toast.error(result.error || t('host.updateError'));
        }
        return;
      }

      let payload: CreateHostRequest = formData;
      if (formData.host_type === HostType.BARE_METAL) {
        payload = {
          ...formData,
          name: formData.name.trim(),
          ip_address: formData.ip_address?.trim(),
        };
      } else if (formData.host_type === HostType.KUBERNETES) {
        // 按认证方式收敛提交字段，避免另一模式的草稿残留干扰后端校验。
        // Scope payload strictly to the active auth mode so draft fields from the other mode never leak.
        const defaultSA = formData.k8s_default_service_account?.trim() || '';
        payload =
          k8sAuthMode === 'kubeconfig'
            ? {
                name: formData.name.trim(),
                host_type: HostType.KUBERNETES,
                description: formData.description,
                k8s_auth_mode: 'kubeconfig',
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_kubeconfig: formData.k8s_kubeconfig?.trim(),
                ...(defaultSA ? {k8s_default_service_account: defaultSA} : {}),
              }
            : {
                name: formData.name.trim(),
                host_type: HostType.KUBERNETES,
                description: formData.description,
                k8s_auth_mode: 'token',
                k8s_api_url: formData.k8s_api_url?.trim(),
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_token: cleanedToken,
                k8s_insecure_skip_tls: Boolean(formData.k8s_insecure_skip_tls),
                k8s_ca_cert: formData.k8s_insecure_skip_tls
                  ? ''
                  : formData.k8s_ca_cert?.trim() || '',
                ...(defaultSA ? {k8s_default_service_account: defaultSA} : {}),
              };
      }
      const result = await services.host.createHostSafe(payload);
      if (result.success && result.data) {
        const created = result.data;
        onSuccess(created);

        // 物理机进入 Step 2 Agent 部署引导；Kubernetes 进入 Step 2 连通性与 RBAC 权限体检。
        // Bare metal transitions to Step 2 Agent guide; Kubernetes transitions to Step 2 connectivity & RBAC check.
        if (
          formData.host_type === HostType.BARE_METAL ||
          formData.host_type === HostType.KUBERNETES
        ) {
          setCreatedHost(created);
          setStep(2);
        } else {
          toast.success(t('host.createSuccess'));
          handleClose();
        }
      } else {
        toast.error(result.error || t('host.createError'));
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('host.createError'));
    } finally {
      setLoading(false);
    }
  };

  const isK8sStep2 = createdHost?.host_type === HostType.KUBERNETES;
  const dialogTitle =
    step === 2
      ? isK8sStep2
        ? t('host.k8sGuide.title')
        : t('host.installGuide.title')
      : t('host.createHost');
  const step1Label =
    formData.host_type === HostType.KUBERNETES
      ? t('host.k8sGuide.step1')
      : t('host.installGuide.step1');
  const step2Label =
    formData.host_type === HostType.KUBERNETES
      ? t('host.k8sGuide.step2')
      : t('host.installGuide.step2');
  const tokenLooksTruncated = looksTruncatedK8sToken(formData.k8s_token);

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        title={dialogTitle}
        className='arc-compact w-[min(calc(100vw-2rem),640px)]!'
        bodyClassName='max-h-[min(85dvh,720px)] overflow-y-auto'
      >
        {step === 1 ? (
          <>
            {/* 先选主机类型（Tip 020 分段控件），再展示对应类型的两步路径与字段 / Choose host type first */}
            <div className='mb-4 flex flex-wrap items-center justify-between gap-3 border-b-[0.5px] border-border/80 pb-3.5'>
              {createdHost ? (
                <div className='flex items-center gap-2 text-xs'>
                  <span className='text-muted-foreground'>{t('host.hostType')}</span>
                  <Badge variant='outline'>
                    {t(
                      `host.types.${formData.host_type === HostType.BARE_METAL ? 'bareMetal' : formData.host_type}`,
                    )}
                  </Badge>
                </div>
              ) : (
                <SegmentedControl
                  label={t('host.hostType')}
                  value={formData.host_type || HostType.BARE_METAL}
                  onValueChange={(value) => {
                    if (
                      value === HostType.BARE_METAL ||
                      value === HostType.KUBERNETES
                    ) {
                      setFormData((prev) => ({...prev, host_type: value}));
                      setFieldErrors({});
                    }
                  }}
                  options={[
                    {
                      value: HostType.BARE_METAL,
                      label: t('host.types.bareMetal'),
                      testId: 'host-type-bare-metal',
                    },
                    {
                      value: HostType.KUBERNETES,
                      label: t('host.types.kubernetes'),
                      testId: 'host-type-kubernetes',
                    },
                  ]}
                />
              )}
              <div className='flex items-center gap-2 text-xs'>
                <span className='inline-flex items-center gap-1.5 font-medium text-primary'>
                  <span className='flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground'>
                    1
                  </span>
                  {step1Label}
                </span>
                <span className='h-px w-5 bg-border' />
                <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                  <span className='flex h-5 w-5 items-center justify-center rounded-full border border-muted-foreground/30 text-[11px]'>
                    2
                  </span>
                  {step2Label}
                </span>
              </div>
            </div>

            <div className='space-y-4'>
              <Input
                id='name'
                label={`${t('host.name')} *`}
                value={formData.name}
                error={fieldErrors.name}
                onChange={(e) => {
                  setFormData({...formData, name: e.target.value});
                  clearFieldError('name');
                }}
                placeholder={t('host.namePlaceholder')}
              />

              {formData.host_type === HostType.BARE_METAL ? (
                <Input
                  id='ip_address'
                  label={`${t('host.ipAddress')} *`}
                  value={formData.ip_address}
                  error={fieldErrors.ip_address}
                  onChange={(e) => {
                    setFormData({...formData, ip_address: e.target.value});
                    clearFieldError('ip_address');
                  }}
                  placeholder='192.168.1.100'
                />
              ) : null}

              {formData.host_type === HostType.KUBERNETES ? (
                <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
                  <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
                    <div className='space-y-1.5'>
                      <Label>{t('host.k8sAuthMode')}</Label>
                      <SegmentedControl
                        label={t('host.k8sAuthMode')}
                        value={k8sAuthMode}
                        onValueChange={(value) => {
                          setFormData({
                            ...formData,
                            k8s_auth_mode: value as K8sAuthMode,
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
                      id='k8s_namespace'
                      label={t('host.namespace')}
                      value={formData.k8s_namespace}
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

                  {k8sAuthMode === 'kubeconfig' ? (
                    <div className='space-y-2'>
                      <Label htmlFor='k8s_kubeconfig'>
                        {t('host.kubeconfig')} *
                      </Label>
                      <Textarea
                        id='k8s_kubeconfig'
                        value={formData.k8s_kubeconfig}
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
                          {t('host.k8sKubeconfigAutoHint')}
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
                  ) : (
                    <>
                      <Input
                        id='k8s_api_url'
                        label={`${t('host.k8sApiUrl')} *`}
                        value={formData.k8s_api_url}
                        error={fieldErrors.k8s_api_url}
                        onChange={(e) => {
                          setFormData({...formData, k8s_api_url: e.target.value});
                          clearFieldError('k8s_api_url');
                        }}
                        placeholder='https://kubernetes.default.svc:6443'
                        className='font-mono text-xs'
                      />
                      <div className='space-y-1.5'>
                        <Label htmlFor='k8s_token'>{t('host.token')} *</Label>
                        <Textarea
                          id='k8s_token'
                          value={formData.k8s_token}
                          aria-invalid={Boolean(fieldErrors.k8s_token)}
                          onChange={(e) => {
                            setFormData({...formData, k8s_token: e.target.value});
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
                        ) : tokenLooksTruncated ? (
                          <p
                            className='text-xs text-amber-600 dark:text-amber-400'
                            data-testid='k8s-token-truncated-warning'
                          >
                            {t('host.k8sTokenTruncatedWarning')}
                          </p>
                        ) : null}
                      </div>
                      <Checkbox
                        id='k8s_insecure_skip_tls'
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
                          <Label htmlFor='k8s_ca_cert'>
                            {t('host.k8sCaCert')}
                          </Label>
                          <Textarea
                            id='k8s_ca_cert'
                            value={formData.k8s_ca_cert}
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
                        </div>
                      ) : null}
                    </>
                  )}
                </div>
              ) : null}

              {/* 低频可选字段收进折叠区，收敛 Step 1 首屏决策点 / Collapse low-frequency optional fields */}
              <div className='border-t-[0.5px] border-border/80 pt-3'>
                <button
                  type='button'
                  className='text-xs font-medium text-muted-foreground hover:text-foreground'
                  onClick={() => setOptionalOpen((prev) => !prev)}
                  aria-expanded={optionalOpen}
                  data-testid='create-host-optional-toggle'
                >
                  {optionalOpen ? '▾' : '▸'} {t('host.optionalSettings')}
                </button>
                {optionalOpen ? (
                  <div className='mt-3 space-y-3'>
                    <div className='space-y-1.5'>
                      <Label htmlFor='description'>
                        {t('host.descriptionLabel')}
                      </Label>
                      <Textarea
                        id='description'
                        value={formData.description}
                        onChange={(e) =>
                          setFormData({...formData, description: e.target.value})
                        }
                        placeholder={t('host.descriptionPlaceholder')}
                        rows={2}
                      />
                    </div>
                    {formData.host_type === HostType.KUBERNETES ? (
                      <div className='space-y-1'>
                        <Input
                          id='k8s_default_service_account'
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
                    ) : null}
                  </div>
                ) : null}
              </div>

              {formData.host_type === HostType.KUBERNETES ? (
                <K8sRBACTemplateSection
                  namespace={formData.k8s_namespace?.trim() || 'default'}
                  runnerServiceAccount={
                    formData.k8s_default_service_account?.trim() ||
                    'seatunnel-runner'
                  }
                  authMode={k8sAuthMode}
                />
              ) : null}
            </div>

            <div className='mt-5 flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={handleClose}
                disabled={loading}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                loading={loading}
                onClick={() => void handleSubmit()}
                disabled={loading}
              >
                {loading
                  ? createdHost
                    ? t('common.loading')
                    : t('common.creating')
                  : formData.host_type === HostType.BARE_METAL
                    ? t('host.installGuide.nextStepAgent')
                    : formData.host_type === HostType.KUBERNETES
                      ? createdHost
                        ? t('host.k8sGuide.saveAndRetest')
                        : t('host.k8sGuide.nextStepTest')
                      : t('common.create')}
              </Button>
            </div>
          </>
        ) : createdHost ? (
          <>
            <div className='mb-3 flex items-center gap-2 text-xs'>
              <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                <span className='flex h-5 w-5 items-center justify-center rounded-full bg-muted text-[11px] text-muted-foreground'>
                  ✓
                </span>
                {isK8sStep2
                  ? t('host.k8sGuide.step1')
                  : t('host.installGuide.step1')}
              </span>
              <span className='h-px w-6 bg-border' />
              <span className='inline-flex items-center gap-1.5 font-medium text-primary'>
                <span className='flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground'>
                  2
                </span>
                {isK8sStep2
                  ? t('host.k8sGuide.step2')
                  : t('host.installGuide.step2')}
              </span>
            </div>
            {isK8sStep2 ? (
              <K8sConnectGuideContent
                host={createdHost}
                onConnected={(updated) => {
                  setCreatedHost(updated);
                  setFormData((prev) => ({
                    ...prev,
                    k8s_default_service_account:
                      updated.k8s_default_service_account ??
                      prev.k8s_default_service_account,
                  }));
                  onSuccess(updated);
                }}
                onBackToEdit={() => setStep(1)}
                onViewDetail={(targetHost) => {
                  handleClose();
                  onViewDetail?.(targetHost);
                }}
                onClose={handleClose}
              />
            ) : (
              <HostInstallGuideContent
                host={createdHost}
                onConnected={(updated) => {
                  setCreatedHost(updated);
                  onSuccess(updated);
                }}
                onViewDetail={(targetHost) => {
                  handleClose();
                  onViewDetail?.(targetHost);
                }}
                onClose={handleClose}
              />
            )}
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
