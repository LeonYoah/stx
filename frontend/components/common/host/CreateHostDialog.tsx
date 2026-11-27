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
 * 新增主机引导抽屉（v2 水平 Step Indicator + 全宽单层画布）：
 * - 物理机：2 步精简创建与 Agent 部署引导；
 * - Kubernetes：3 步水平 Step Indicator（1 连接与范围 → 2 授权与凭据 → 3 预检与保存），
 *   命名空间在 Step 1 首屏与 Step 2 顶部均可直接编辑并实时联动控制面 YAML，
 *   Step 3 保存主机后再按需展开独立的 SeaTunnel 引擎运行账号预备与绑定卡片。
 * Create Host onboarding drawer (v2 horizontal Step Indicator + full-width single canvas):
 * - Bare-metal: 2-step form + Agent guide;
 * - Kubernetes: 3-step horizontal stepper (1 Access & Scope → 2 RBAC & Credentials → 3 Verify & Save),
 *   editable namespace on both Step 1 and top of Step 2 live-synced with control-plane YAML,
 *   plus optional post-save SeaTunnel engine SA prep and bind-default on Step 3.
 */

import {Fragment, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {Input} from '@/components/arc/input/input';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  CreateHostRequest,
  HostInfo,
  HostStatus,
  HostType,
  K8sAccessMode,
  K8sAuthMode,
  K8sTestConnectionResult,
} from '@/lib/services/host/types';
import {
  looksTruncatedK8sToken,
  sanitizeK8sToken,
} from '@/lib/k8s-rbac-templates';
import {cn} from '@/lib/utils';
import {HostInstallGuideContent} from './HostInstallGuideContent';
import {K8sConnectGuideContent, type K8sDraftHostInput} from './K8sConnectGuideContent';
import {K8sKubeconfigField} from './K8sKubeconfigField';
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
  k8s_namespace: 'seatunnel',
  k8s_kubeconfig: '',
  k8s_token: '',
  k8s_auth_mode: 'kubeconfig',
  k8s_access_mode: 'in_cluster',
  k8s_ca_cert: '',
  k8s_insecure_skip_tls: false,
};

type CreateFieldErrorKey =
  | 'name'
  | 'ip_address'
  | 'k8s_kubeconfig'
  | 'k8s_api_url'
  | 'k8s_token';

type K8sPrepMode = 'generate' | 'existing';

interface CreateHostDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: (createdHost?: HostInfo) => void;
  onViewDetail?: (host: HostInfo) => void;
  onDeployCluster?: (host: HostInfo) => void;
}

export function CreateHostDialog({
  open,
  onOpenChange,
  onSuccess,
  onViewDetail,
  onDeployCluster,
}: CreateHostDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [savingHost, setSavingHost] = useState(false);
  const [step, setStep] = useState<1 | 2 | 3>(1);
  const [createdHost, setCreatedHost] = useState<HostInfo | null>(null);
  const [optionalOpen, setOptionalOpen] = useState(false);
  const [k8sPrepMode, setK8sPrepMode] = useState<K8sPrepMode>('generate');
  const [k8sContext, setK8sContext] = useState('');
  const [k8sServerOverride, setK8sServerOverride] = useState('');
  const [namespaceTouched, setNamespaceTouched] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<
    Partial<Record<CreateFieldErrorKey, string>>
  >({});
  const [formData, setFormData] = useState<CreateHostRequest>(() => ({
    ...INITIAL_FORM_DATA,
  }));

  const k8sAuthMode: K8sAuthMode =
    formData.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig';
  const k8sAccessMode: K8sAccessMode =
    formData.k8s_access_mode || 'in_cluster';

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
    setK8sPrepMode('generate');
    setK8sContext('');
    setK8sServerOverride('');
    setNamespaceTouched(false);
    setSavingHost(false);
    setStep(1);
    setCreatedHost(null);
  };

  const handleClose = () => {
    resetForm();
    onOpenChange(false);
  };

  // Tip 016：表单字段校验错误就地贴在对应字段下方，不再仅用消失式 Toast 提示。
  // Tip 016: surface validation errors inline on each offending field instead of ephemeral toasts.
  const validateStep1 = (): boolean => {
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

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const validateK8sStep2 = (): boolean => {
    const errors: Partial<Record<CreateFieldErrorKey, string>> = {};
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

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmitStep1 = async () => {
    if (!validateStep1()) {
      return;
    }

    if (formData.host_type === HostType.KUBERNETES) {
      setStep(2);
      return;
    }

    setLoading(true);
    try {
      const payload: CreateHostRequest = {
        ...formData,
        name: formData.name.trim(),
        ip_address: formData.ip_address?.trim(),
      };
      const result = await services.host.createHostSafe(payload);
      if (result.success && result.data) {
        const created = result.data;
        onSuccess(created);
        setCreatedHost(created);
        setStep(2);
      } else {
        toast.error(result.error || t('host.createError'));
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('host.createError'));
    } finally {
      setLoading(false);
    }
  };

  const handleK8sStep2Next = () => {
    if (!validateK8sStep2()) {
      return;
    }
    setStep(3);
  };

  const handleSaveK8sHost = async (probe: K8sTestConnectionResult) => {
    setSavingHost(true);
    try {
      const cleanedToken = sanitizeK8sToken(formData.k8s_token);
      const payload: CreateHostRequest =
        k8sAuthMode === 'kubeconfig'
          ? {
              name: formData.name.trim(),
              host_type: HostType.KUBERNETES,
              description: formData.description?.trim() || '',
              k8s_auth_mode: 'kubeconfig',
              k8s_access_mode: k8sAccessMode,
              k8s_namespace: formData.k8s_namespace?.trim() || 'seatunnel',
              k8s_kubeconfig: formData.k8s_kubeconfig?.trim(),
              ...(k8sContext.trim() ? {k8s_context: k8sContext.trim()} : {}),
              ...(k8sServerOverride.trim()
                ? {k8s_api_url: k8sServerOverride.trim()}
                : {}),
            }
          : {
              name: formData.name.trim(),
              host_type: HostType.KUBERNETES,
              description: formData.description?.trim() || '',
              k8s_auth_mode: 'token',
              k8s_access_mode: k8sAccessMode,
              k8s_api_url: formData.k8s_api_url?.trim(),
              k8s_namespace: formData.k8s_namespace?.trim() || 'seatunnel',
              k8s_token: cleanedToken,
              k8s_insecure_skip_tls: Boolean(formData.k8s_insecure_skip_tls),
              k8s_ca_cert: formData.k8s_insecure_skip_tls
                ? ''
                : formData.k8s_ca_cert?.trim() || '',
            };
      const result = await services.host.createHostSafe(payload);
      if (!result.success || !result.data) {
        toast.error(result.error || t('host.createError'));
        return;
      }
      let finalHost: HostInfo = {
        ...result.data,
        status: HostStatus.CONNECTED,
        is_online: true,
        k8s_version: probe.server_version || result.data.k8s_version,
        k8s_permission_mode: probe.permission_mode || result.data.k8s_permission_mode,
        k8s_credential_expires_at:
          probe.credential_expires_at ?? result.data.k8s_credential_expires_at,
      };
      try {
        await services.host.testK8sConnection(result.data.id, {timeoutMs: 20000});
        const fresh = await services.host.getHostSafe?.(result.data.id);
        if (fresh?.success && fresh.data) {
          finalHost = fresh.data;
        }
      } catch {
        // 入库已完成，若服务端二次刷新失败保留预检阶段结果。
        // Persist succeeded; keep Step 3 probe metadata if follow-up refresh fails transiently.
      }
      toast.success(t('host.createSuccess'));
      setCreatedHost(finalHost);
      onSuccess(finalHost);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('host.createError'));
    } finally {
      setSavingHost(false);
    }
  };

  const isK8sWizard =
    formData.host_type === HostType.KUBERNETES ||
    createdHost?.host_type === HostType.KUBERNETES;

  const k8sDraftInput: K8sDraftHostInput = {
    name: formData.name.trim(),
    description: formData.description,
    k8s_auth_mode: k8sAuthMode,
    k8s_access_mode: k8sAccessMode,
    k8s_namespace: formData.k8s_namespace?.trim() || 'seatunnel',
    k8s_kubeconfig: formData.k8s_kubeconfig?.trim(),
    k8s_context: k8sContext.trim() || undefined,
    k8s_api_url:
      k8sAuthMode === 'kubeconfig'
        ? k8sServerOverride.trim() || undefined
        : formData.k8s_api_url?.trim(),
    k8s_token: sanitizeK8sToken(formData.k8s_token),
    k8s_ca_cert: formData.k8s_insecure_skip_tls
      ? ''
      : formData.k8s_ca_cert?.trim() || '',
    k8s_insecure_skip_tls: Boolean(formData.k8s_insecure_skip_tls),
  };

  const drawerTitle = isK8sWizard
    ? t('host.k8sGuide.title')
    : step === 2
      ? t('host.installGuide.title')
      : t('host.createHost');
  const drawerDesc = isK8sWizard
    ? t('host.k8sGuide.subtitle')
    : step === 2
      ? t('host.installGuide.subtitle')
      : undefined;
  const tokenLooksTruncated = looksTruncatedK8sToken(formData.k8s_token);

  const k8sSteps = [
    {num: 1 as const, title: t('host.k8sGuide.stepAccessTitle')},
    {num: 2 as const, title: t('host.k8sGuide.stepCredentialsTitle')},
    {num: 3 as const, title: t('host.k8sGuide.stepProbeTitle')},
  ];

  const handleStepClick = (targetStep: 1 | 2 | 3) => {
    if (createdHost || targetStep === step) {
      return;
    }
    if (targetStep < step) {
      setStep(targetStep);
      return;
    }
    if (targetStep === 2 && validateStep1()) {
      setStep(2);
    } else if (targetStep === 3 && validateStep1() && validateK8sStep2()) {
      setStep(3);
    }
  };

  return (
    <Drawer
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          handleClose();
        }
      }}
    >
      <DrawerContent
        side='right'
        title={drawerTitle}
        description={drawerDesc}
        className={cn(
          // DrawerContent portal 到 body，重挂 .arc-compact 保证密度一致；水平 Step Indicator 释放完整单层画布宽度。
          // DrawerContent portals to <body>; re-apply .arc-compact and give the single canvas full horizontal breathing room.
          'arc-compact [&>div[class*=body]]:flex [&>div[class*=body]]:flex-1 [&>div[class*=body]]:flex-col [&>div[class*=body]]:p-0!',
          isK8sWizard ? 'w-[min(94vw,760px)]!' : 'w-[min(92vw,560px)]!',
        )}
      >
        {/* 顶部整合条：主机类型切换 + 水平 Step Indicator（去掉重复的「第 X / 3 步」徽章与 228px 侧栏） */}
        {/* Unified top bar: host type selector + horizontal Step Indicator (no redundant step counter badge) */}
        <div className='flex shrink-0 flex-wrap items-center justify-between gap-3 border-b-[0.5px] border-border/80 px-5 py-3'>
          {createdHost || step > 1 ? (
            <div className='flex items-center gap-2 text-xs'>
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

          {isK8sWizard ? (
            <nav
              className='flex items-center gap-1.5'
              aria-label={t('host.k8sGuide.title')}
            >
              {k8sSteps.map((item, idx) => {
                const done = Boolean(createdHost) || step > item.num;
                const active = !createdHost && step === item.num;
                return (
                  <Fragment key={item.num}>
                    <button
                      type='button'
                      onClick={() => handleStepClick(item.num)}
                      disabled={Boolean(createdHost)}
                      data-testid={`k8s-wizard-step-${item.num}`}
                      aria-current={active ? 'step' : undefined}
                      className={cn(
                        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors',
                        active
                          ? 'bg-primary/10 font-semibold text-foreground'
                          : done
                            ? 'font-medium text-foreground/80 hover:bg-muted/60'
                            : 'text-muted-foreground hover:bg-muted/50',
                      )}
                    >
                      <span
                        className={cn(
                          'flex size-4 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold tabular-nums',
                          done
                            ? 'bg-emerald-600 text-white'
                            : active
                              ? 'bg-primary text-primary-foreground'
                              : 'border border-border text-muted-foreground',
                        )}
                      >
                        {done ? <Check className='size-2.5' aria-hidden /> : item.num}
                      </span>
                      <span className='whitespace-nowrap'>{item.title}</span>
                    </button>
                    {idx < k8sSteps.length - 1 ? (
                      <span
                        aria-hidden
                        className={cn(
                          'h-px w-4 rounded-full transition-colors',
                          done ? 'bg-emerald-500/70' : 'bg-border',
                        )}
                      />
                    ) : null}
                  </Fragment>
                );
              })}
            </nav>
          ) : (
            <Badge variant='outline' className='font-mono text-[11px] tabular-nums'>
              {t('host.k8sGuide.stepCounter', {
                current: step,
                total: 2,
              })}
            </Badge>
          )}
        </div>

        {isK8sWizard ? (
          <div className='flex min-h-0 flex-1 flex-col'>
            <div className='min-h-0 flex-1 overflow-y-auto p-5 lg:p-6'>
              {step === 1 ? (
                <div className='space-y-5' data-testid='k8s-step-1'>
                  {/* Step 1 首屏直接开放主机名与命名空间编辑 / Editable host name + namespace right on Step 1 */}
                  <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
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
                    <Input
                      id='k8s_namespace'
                      label={t('host.namespace')}
                      value={formData.k8s_namespace}
                      onChange={(e) => {
                        setNamespaceTouched(true);
                        setFormData({
                          ...formData,
                          k8s_namespace: e.target.value,
                        });
                      }}
                      placeholder='seatunnel'
                      className='font-mono text-xs'
                    />
                  </div>

                  <div className='space-y-2.5 border-t-[0.5px] border-border/80 pt-4'>
                    <Label>{t('host.k8sAccessMode')}</Label>
                    <RadioCards
                      layout='grid'
                      minColumnWidth={210}
                      value={k8sAccessMode}
                      onValueChange={(value) =>
                        setFormData({
                          ...formData,
                          k8s_access_mode: value as K8sAccessMode,
                        })
                      }
                      options={[
                        {
                          value: 'in_cluster',
                          label: (
                            <span data-testid='k8s-access-mode-in_cluster'>
                              {t('host.k8sAccessModeInCluster')}
                            </span>
                          ),
                          description: t('host.k8sAccessModeInClusterDesc'),
                        },
                        {
                          value: 'external',
                          label: (
                            <span data-testid='k8s-access-mode-external'>
                              {t('host.k8sAccessModeExternal')}
                            </span>
                          ),
                          description: t('host.k8sAccessModeExternalDesc'),
                        },
                      ]}
                    />
                    <div className='rounded-[var(--radius-control)] bg-muted/40 px-3 py-2 text-xs text-muted-foreground dark:bg-muted/25'>
                      {k8sAccessMode === 'external'
                        ? t('host.k8sAccessFlowExternal')
                        : t('host.k8sAccessFlowInCluster')}
                    </div>
                  </div>

                  <div className='space-y-2.5 border-t-[0.5px] border-border/80 pt-4'>
                    <Label>{t('host.k8sGuide.prepQuestion')}</Label>
                    <RadioCards
                      layout='grid'
                      minColumnWidth={210}
                      value={k8sPrepMode}
                      onValueChange={(value) =>
                        setK8sPrepMode(value as K8sPrepMode)
                      }
                      options={[
                        {
                          value: 'generate',
                          label: (
                            <span data-testid='k8s-prep-mode-generate'>
                              {t('host.k8sGuide.prepGenerateLabel')}
                            </span>
                          ),
                          description: t('host.k8sGuide.prepGenerateDesc'),
                        },
                        {
                          value: 'existing',
                          label: (
                            <span data-testid='k8s-prep-mode-existing'>
                              {t('host.k8sGuide.prepExistingLabel')}
                            </span>
                          ),
                          description: t('host.k8sGuide.prepExistingDesc'),
                        },
                      ]}
                    />
                  </div>

                  <div className='border-t-[0.5px] border-border/80 pt-3'>
                    <button
                      type='button'
                      className='text-xs font-medium text-muted-foreground hover:text-foreground'
                      onClick={() => setOptionalOpen((prev) => !prev)}
                      aria-expanded={optionalOpen}
                      data-testid='k8s-step1-optional-toggle'
                    >
                      {optionalOpen ? '▾' : '▸'} {t('host.optionalSettings')}
                    </button>
                    {optionalOpen ? (
                      <div className='mt-3'>
                        <Input
                          id='description'
                          label={t('host.descriptionLabel')}
                          value={formData.description || ''}
                          onChange={(e) =>
                            setFormData({...formData, description: e.target.value})
                          }
                          placeholder={t('host.descriptionPlaceholder')}
                        />
                      </div>
                    ) : null}
                  </div>
                </div>
              ) : step === 2 ? (
                <div className='space-y-4' data-testid='k8s-step-2'>
                  {/* Step 2 顶部置顶认证方式与命名空间：编辑后实时联动下方控制面 YAML 与命令 */}
                  {/* Top parameter strip on Step 2: auth mode & editable namespace live-synced with RBAC YAML below */}
                  <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 sm:items-end'>
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
                          setFieldErrors({});
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
                      onChange={(e) => {
                        setNamespaceTouched(true);
                        setFormData({
                          ...formData,
                          k8s_namespace: e.target.value,
                        });
                      }}
                      placeholder='seatunnel'
                      className='font-mono text-xs'
                    />
                  </div>

                  <K8sRBACTemplateSection
                    namespace={formData.k8s_namespace?.trim() || 'seatunnel'}
                    authMode={k8sAuthMode}
                    defaultOpen={k8sPrepMode === 'generate'}
                  />

                  <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
                    {k8sAuthMode === 'kubeconfig' ? (
                      <K8sKubeconfigField
                        id='k8s_kubeconfig'
                        label={`${t('host.kubeconfig')} *`}
                        value={formData.k8s_kubeconfig || ''}
                        onChange={(nextValue) => {
                          setFormData((prev) => ({
                            ...prev,
                            k8s_kubeconfig: nextValue,
                          }));
                          clearFieldError('k8s_kubeconfig');
                        }}
                        contextName={k8sContext}
                        onContextChange={setK8sContext}
                        serverOverride={k8sServerOverride}
                        onServerOverrideChange={setK8sServerOverride}
                        onNamespaceDetected={(ns) => {
                          if (!namespaceTouched) {
                            setFormData((prev) => ({
                              ...prev,
                              k8s_namespace: ns,
                            }));
                          }
                        }}
                        accessMode={k8sAccessMode}
                        error={fieldErrors.k8s_kubeconfig}
                        hint={t('host.k8sKubeconfigAutoHint')}
                        testIdPrefix='create-kubeconfig'
                      />
                    ) : (
                      <>
                        <Input
                          id='k8s_api_url'
                          label={`${t('host.k8sApiUrl')} *`}
                          value={formData.k8s_api_url}
                          error={fieldErrors.k8s_api_url}
                          onChange={(e) => {
                            setFormData({
                              ...formData,
                              k8s_api_url: e.target.value,
                            });
                            clearFieldError('k8s_api_url');
                          }}
                          placeholder={
                            k8sAccessMode === 'in_cluster'
                              ? 'https://kubernetes.default.svc:443'
                              : 'https://192.168.1.10:6443'
                          }
                          className='font-mono text-xs'
                        />
                        <div className='space-y-1.5'>
                          <Label htmlFor='k8s_token'>
                            {t('host.token')} *
                          </Label>
                          <Textarea
                            id='k8s_token'
                            value={formData.k8s_token}
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
                        {formData.k8s_insecure_skip_tls ? (
                          <p
                            className='text-xs text-amber-600 dark:text-amber-400'
                            data-testid='k8s-skip-tls-warning'
                          >
                            {t('host.k8sInsecureSkipTLSWarning')}
                          </p>
                        ) : (
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
                        )}
                      </>
                    )}
                  </div>
                </div>
              ) : (
                <K8sConnectGuideContent
                  draft={k8sDraftInput}
                  savedHost={createdHost}
                  onHostUpdated={(updated) => {
                    setCreatedHost(updated);
                    onSuccess(updated);
                  }}
                  onBackToEdit={() => setStep(2)}
                  onSaveHost={handleSaveK8sHost}
                  savingHost={savingHost}
                  onDeployCluster={onDeployCluster}
                  onViewDetail={onViewDetail}
                  onClose={handleClose}
                />
              )}
            </div>

            {step < 3 ? (
              <div className='flex shrink-0 items-center justify-between gap-2 border-t-[0.5px] border-border/80 px-5 py-3.5'>
                <div>
                  {step === 2 ? (
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={() => setStep(1)}
                    >
                      {t('common.previous')}
                    </Button>
                  ) : null}
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={handleClose}
                  >
                    {t('common.cancel')}
                  </Button>
                  {step === 1 ? (
                    <Button
                      type='button'
                      size='sm'
                      onClick={() => void handleSubmitStep1()}
                      data-testid='k8s-step1-next'
                    >
                      {t('host.k8sGuide.nextToCredentials')}
                    </Button>
                  ) : (
                    <Button
                      type='button'
                      size='sm'
                      onClick={handleK8sStep2Next}
                      data-testid='k8s-step2-next'
                    >
                      {t('host.k8sGuide.nextStepTest')}
                    </Button>
                  )}
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          /* 物理机两步创建与引导 / Bare-metal 2-step form & onboarding guide */
          <div className='flex min-h-0 flex-1 flex-col'>
            <div className='min-h-0 flex-1 overflow-y-auto p-5 lg:p-6'>
              {step === 1 ? (
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
                      <div className='mt-3 space-y-1.5'>
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
                    ) : null}
                  </div>
                </div>
              ) : createdHost ? (
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
              ) : null}
            </div>

            {step === 1 ? (
              <div className='flex shrink-0 items-center justify-end gap-2 border-t-[0.5px] border-border/80 px-5 py-3.5'>
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
                  onClick={() => void handleSubmitStep1()}
                  disabled={loading}
                >
                  {loading
                    ? t('common.creating')
                    : t('host.installGuide.nextStepAgent')}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </DrawerContent>
    </Drawer>
  );
}
