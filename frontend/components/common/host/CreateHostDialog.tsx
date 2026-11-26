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
 * Create Host Dialog Component
 * 创建主机对话框组件
 *
 * Dialog for creating a new host with support for different host types.
 * 用于创建新主机的对话框，支持不同的主机类型。
 */

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  HostType,
  CreateHostRequest,
  HostInfo,
  K8sAuthMode,
} from '@/lib/services/host/types';
import {cn} from '@/lib/utils';
import {HostInstallGuideContent} from './HostInstallGuideContent';

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
  k8s_kubeconfig: '',
  k8s_token: '',
  k8s_auth_mode: 'kubeconfig',
  k8s_ca_cert: '',
  k8s_insecure_skip_tls: false,
};

interface CreateHostDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: (createdHost?: HostInfo) => void;
  onViewDetail?: (host: HostInfo) => void;
}

/**
 * Create Host Dialog Component
 * 创建主机对话框组件
 */
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
  const [formData, setFormData] = useState<CreateHostRequest>(() => ({
    ...INITIAL_FORM_DATA,
  }));

  const k8sAuthMode: K8sAuthMode =
    formData.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig';

  /**
   * Reset form data
   * 重置表单数据
   */
  const resetForm = () => {
    setFormData({...INITIAL_FORM_DATA});
    setStep(1);
    setCreatedHost(null);
  };

  /**
   * Handle dialog close
   * 处理对话框关闭
   */
  const handleClose = () => {
    resetForm();
    onOpenChange(false);
  };

  /**
   * Validate form data
   * 验证表单数据
   */
  const validateForm = (): boolean => {
    if (!formData.name || formData.name.trim().length === 0) {
      toast.error(t('host.errors.nameRequired'));
      return false;
    }

    if (formData.host_type === HostType.BARE_METAL) {
      if (!formData.ip_address || formData.ip_address.trim().length === 0) {
        toast.error(t('host.errors.ipRequired'));
        return false;
      }
      // Basic IP validation / 基本 IP 验证
      const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$|^([a-fA-F0-9:]+)$/;
      if (!ipRegex.test(formData.ip_address)) {
        toast.error(t('host.errors.ipInvalid'));
        return false;
      }
    }

    if (formData.host_type === HostType.DOCKER) {
      if (!formData.docker_api_url || formData.docker_api_url.trim().length === 0) {
        toast.error(t('host.errors.dockerUrlRequired'));
        return false;
      }
      // Validate Docker API URL format / 验证 Docker API URL 格式
      if (
        !formData.docker_api_url.startsWith('tcp://') &&
        !formData.docker_api_url.startsWith('unix://')
      ) {
        toast.error(t('host.errors.dockerUrlInvalid'));
        return false;
      }
    }

    if (formData.host_type === HostType.KUBERNETES) {
      if (k8sAuthMode === 'kubeconfig') {
        if (!formData.k8s_kubeconfig || formData.k8s_kubeconfig.trim().length === 0) {
          toast.error(t('host.errors.k8sKubeconfigRequired'));
          return false;
        }
      } else {
        if (!formData.k8s_api_url || formData.k8s_api_url.trim().length === 0) {
          toast.error(t('host.errors.k8sUrlRequired'));
          return false;
        }
        // Validate K8s API URL format / 验证 K8s API URL 格式
        if (
          !formData.k8s_api_url.startsWith('https://') &&
          !formData.k8s_api_url.startsWith('http://')
        ) {
          toast.error(t('host.errors.k8sUrlInvalid'));
          return false;
        }
        if (!formData.k8s_token || formData.k8s_token.trim().length === 0) {
          toast.error(t('host.errors.k8sTokenRequired'));
          return false;
        }
      }
    }

    return true;
  };

  /**
   * Handle form submit
   * 处理表单提交
   */
  const handleSubmit = async () => {
    if (!validateForm()) {return;}

    setLoading(true);
    try {
      let payload: CreateHostRequest = formData;
      if (formData.host_type === HostType.KUBERNETES) {
        // 按认证方式收敛提交字段，避免另一模式的草稿残留干扰后端校验。
        // Scope payload strictly to the active auth mode so draft fields from the other mode never leak.
        payload =
          k8sAuthMode === 'kubeconfig'
            ? {
                name: formData.name.trim(),
                host_type: HostType.KUBERNETES,
                description: formData.description,
                k8s_auth_mode: 'kubeconfig',
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_kubeconfig: formData.k8s_kubeconfig?.trim(),
              }
            : {
                name: formData.name.trim(),
                host_type: HostType.KUBERNETES,
                description: formData.description,
                k8s_auth_mode: 'token',
                k8s_api_url: formData.k8s_api_url?.trim(),
                k8s_namespace: formData.k8s_namespace?.trim() || 'default',
                k8s_token: formData.k8s_token?.trim(),
                k8s_insecure_skip_tls: Boolean(formData.k8s_insecure_skip_tls),
                k8s_ca_cert: formData.k8s_insecure_skip_tls
                  ? ''
                  : formData.k8s_ca_cert?.trim() || '',
              };
      }
      const result = await services.host.createHostSafe(payload);
      if (result.success && result.data) {
        const created = result.data;
        // 触发父级回调刷新表格数据 / Notify parent to refresh host list
        onSuccess(created);

        // 若为物理机主机，无缝进入 Step 2 Agent 部署引导 / If bare metal, transition to Step 2 guide
        if (formData.host_type === HostType.BARE_METAL) {
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

  const dialogTitle =
    step === 2 ? t('host.installGuide.title') : t('host.createHost');

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        title={dialogTitle}
        className={cn(
          'arc-compact',
          step === 2
            ? 'w-[min(calc(100vw-2rem),700px)]!'
            : 'w-[min(calc(100vw-2rem),560px)]!',
        )}
        bodyClassName='max-h-[min(85dvh,720px)] overflow-y-auto'
      >
        {step === 1 ? (
          <>
            {formData.host_type === HostType.BARE_METAL ? (
              <div className='mb-4 flex items-center gap-2 text-xs'>
                <span className='inline-flex items-center gap-1.5 font-medium text-primary'>
                  <span className='flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground'>
                    1
                  </span>
                  {t('host.installGuide.step1')}
                </span>
                <span className='h-px w-6 bg-border' />
                <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                  <span className='flex h-5 w-5 items-center justify-center rounded-full border border-muted-foreground/30 text-[11px]'>
                    2
                  </span>
                  {t('host.installGuide.step2')}
                </span>
              </div>
            ) : null}

            <div className='space-y-4'>
              <Input
                id='name'
                label={`${t('host.name')} *`}
                value={formData.name}
                onChange={(e) =>
                  setFormData({...formData, name: e.target.value})
                }
                placeholder={t('host.namePlaceholder')}
              />

              <Select
                id='host_type'
                label={`${t('host.hostType')} *`}
                value={formData.host_type}
                onValueChange={(value) => {
                  // 仅 Docker 暂未支持；物理机/虚拟机与 Kubernetes 均已开放接入。
                  // Only Docker remains unsupported; bare metal and Kubernetes are both enabled.
                  if (
                    value === HostType.BARE_METAL ||
                    value === HostType.KUBERNETES
                  ) {
                    setFormData({...formData, host_type: value});
                  }
                }}
                options={[
                  {
                    value: HostType.BARE_METAL,
                    label: t('host.types.bareMetal'),
                  },
                  {
                    value: HostType.KUBERNETES,
                    label: t('host.types.kubernetes'),
                  },
                  {
                    value: HostType.DOCKER,
                    label: t('host.types.unsupportedOption', {
                      type: t('host.types.docker'),
                    }),
                    disabled: true,
                  },
                ]}
              />

              <div className='space-y-2'>
                <Label htmlFor='description'>{t('host.descriptionLabel')}</Label>
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

              {formData.host_type === HostType.BARE_METAL ? (
                <Input
                  id='ip_address'
                  label={`${t('host.ipAddress')} *`}
                  value={formData.ip_address}
                  onChange={(e) =>
                    setFormData({...formData, ip_address: e.target.value})
                  }
                  placeholder='192.168.1.100'
                />
              ) : null}

              {formData.host_type === HostType.DOCKER ? (
                <>
                  <Input
                    id='docker_api_url'
                    label={`${t('host.dockerApiUrl')} *`}
                    description={t('host.dockerApiUrlHint')}
                    value={formData.docker_api_url}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        docker_api_url: e.target.value,
                      })
                    }
                    placeholder='tcp://192.168.1.100:2375'
                  />
                  <Switch
                    id='docker_tls_enabled'
                    label={t('host.tlsEnabled')}
                    checked={formData.docker_tls_enabled}
                    onCheckedChange={(checked) =>
                      setFormData({
                        ...formData,
                        docker_tls_enabled: checked,
                      })
                    }
                  />
                  {formData.docker_tls_enabled ? (
                    <Input
                      id='docker_cert_path'
                      label={t('host.certPath')}
                      value={formData.docker_cert_path}
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

              {formData.host_type === HostType.KUBERNETES ? (
                <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
                  <div className='space-y-1.5'>
                    <Label>{t('host.k8sAuthMode')}</Label>
                    <SegmentedControl
                      label={t('host.k8sAuthMode')}
                      value={k8sAuthMode}
                      onValueChange={(value) =>
                        setFormData({
                          ...formData,
                          k8s_auth_mode: value as K8sAuthMode,
                        })
                      }
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
                  {k8sAuthMode === 'kubeconfig' ? (
                    <div className='space-y-1.5'>
                      <Label htmlFor='k8s_kubeconfig'>
                        {t('host.kubeconfig')} *
                      </Label>
                      <Textarea
                        id='k8s_kubeconfig'
                        value={formData.k8s_kubeconfig}
                        onChange={(e) =>
                          setFormData({
                            ...formData,
                            k8s_kubeconfig: e.target.value,
                          })
                        }
                        placeholder={t('host.kubeconfigPlaceholder')}
                        className='font-mono text-xs'
                        rows={6}
                      />
                      <p className='text-xs text-muted-foreground'>
                        {t('host.k8sKubeconfigAutoHint')}
                      </p>
                    </div>
                  ) : (
                    <>
                      <Input
                        id='k8s_api_url'
                        label={`${t('host.k8sApiUrl')} *`}
                        value={formData.k8s_api_url}
                        onChange={(e) =>
                          setFormData({...formData, k8s_api_url: e.target.value})
                        }
                        placeholder='https://kubernetes.default.svc:6443'
                        className='font-mono text-xs'
                      />
                      <div className='space-y-1.5'>
                        <Label htmlFor='k8s_token'>{t('host.token')} *</Label>
                        <Textarea
                          id='k8s_token'
                          value={formData.k8s_token}
                          onChange={(e) =>
                            setFormData({...formData, k8s_token: e.target.value})
                          }
                          placeholder={t('host.tokenPlaceholder')}
                          className='font-mono text-xs'
                          rows={3}
                        />
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
                          <Label htmlFor='k8s_ca_cert'>{t('host.k8sCaCert')}</Label>
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
            </div>

            <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
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
                  ? t('common.creating')
                  : formData.host_type === HostType.BARE_METAL
                    ? t('host.installGuide.nextStepAgent')
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
                {t('host.installGuide.step1')}
              </span>
              <span className='h-px w-6 bg-border' />
              <span className='inline-flex items-center gap-1.5 font-medium text-primary'>
                <span className='flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground'>
                  2
                </span>
                {t('host.installGuide.step2')}
              </span>
            </div>
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
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
