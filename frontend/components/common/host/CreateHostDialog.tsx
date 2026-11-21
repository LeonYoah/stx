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
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {Switch} from '@/components/ui/switch';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {HostType, CreateHostRequest, HostInfo} from '@/lib/services/host/types';
import {cn} from '@/lib/utils';
import {HostInstallGuideContent} from './HostInstallGuideContent';

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
  const [formData, setFormData] = useState<CreateHostRequest>({
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
  });


  /**
   * Reset form data
   * 重置表单数据
   */
  const resetForm = () => {
    setFormData({
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
    });
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
      // K8s requires either kubeconfig or token / K8s 需要 kubeconfig 或 token
      if (!formData.k8s_kubeconfig && !formData.k8s_token) {
        toast.error(t('host.errors.k8sCredentialsRequired'));
        return false;
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
      const result = await services.host.createHostSafe(formData);
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
          step === 2
            ? 'w-[min(calc(100vw-2rem),700px)]!'
            : 'w-[min(calc(100vw-2rem),540px)]!',
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
              <div className='space-y-2'>
                <Label htmlFor='name'>{t('host.name')} *</Label>
                <Input
                  id='name'
                  value={formData.name}
                  onChange={(e) =>
                    setFormData({...formData, name: e.target.value})
                  }
                  placeholder={t('host.namePlaceholder')}
                />
              </div>

              <div className='space-y-2'>
                <Label htmlFor='host_type'>{t('host.hostType')} *</Label>
                <Select
                  value={formData.host_type}
                  onValueChange={(value) =>
                    setFormData({...formData, host_type: value as HostType})
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={HostType.BARE_METAL}>
                      {t('host.types.bareMetal')}
                    </SelectItem>
                    <SelectItem value={HostType.DOCKER}>
                      {t('host.types.docker')}
                    </SelectItem>
                    <SelectItem value={HostType.KUBERNETES}>
                      {t('host.types.kubernetes')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

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
                <div className='space-y-2'>
                  <Label htmlFor='ip_address'>{t('host.ipAddress')} *</Label>
                  <Input
                    id='ip_address'
                    value={formData.ip_address}
                    onChange={(e) =>
                      setFormData({...formData, ip_address: e.target.value})
                    }
                    placeholder='192.168.1.100'
                  />
                </div>
              ) : null}

              {formData.host_type === HostType.DOCKER ? (
                <>
                  <div className='space-y-2'>
                    <Label htmlFor='docker_api_url'>
                      {t('host.dockerApiUrl')} *
                    </Label>
                    <Input
                      id='docker_api_url'
                      value={formData.docker_api_url}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          docker_api_url: e.target.value,
                        })
                      }
                      placeholder='tcp://192.168.1.100:2375'
                    />
                    <p className='text-xs text-muted-foreground'>
                      {t('host.dockerApiUrlHint')}
                    </p>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <Switch
                      id='docker_tls_enabled'
                      checked={formData.docker_tls_enabled}
                      onCheckedChange={(checked) =>
                        setFormData({
                          ...formData,
                          docker_tls_enabled: checked,
                        })
                      }
                    />
                    <Label htmlFor='docker_tls_enabled'>
                      {t('host.tlsEnabled')}
                    </Label>
                  </div>
                  {formData.docker_tls_enabled ? (
                    <div className='space-y-2'>
                      <Label htmlFor='docker_cert_path'>
                        {t('host.certPath')}
                      </Label>
                      <Input
                        id='docker_cert_path'
                        value={formData.docker_cert_path}
                        onChange={(e) =>
                          setFormData({
                            ...formData,
                            docker_cert_path: e.target.value,
                          })
                        }
                        placeholder='/path/to/certs'
                      />
                    </div>
                  ) : null}
                </>
              ) : null}

              {formData.host_type === HostType.KUBERNETES ? (
                <>
                  <div className='space-y-2'>
                    <Label htmlFor='k8s_api_url'>{t('host.k8sApiUrl')} *</Label>
                    <Input
                      id='k8s_api_url'
                      value={formData.k8s_api_url}
                      onChange={(e) =>
                        setFormData({...formData, k8s_api_url: e.target.value})
                      }
                      placeholder='https://kubernetes.default.svc:6443'
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='k8s_namespace'>{t('host.namespace')}</Label>
                    <Input
                      id='k8s_namespace'
                      value={formData.k8s_namespace}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          k8s_namespace: e.target.value,
                        })
                      }
                      placeholder='default'
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='k8s_kubeconfig'>
                      {t('host.kubeconfig')}
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
                      rows={3}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='k8s_token'>{t('host.token')}</Label>
                    <Textarea
                      id='k8s_token'
                      value={formData.k8s_token}
                      onChange={(e) =>
                        setFormData({...formData, k8s_token: e.target.value})
                      }
                      placeholder={t('host.tokenPlaceholder')}
                      rows={2}
                    />
                    <p className='text-xs text-muted-foreground'>
                      {t('host.k8sCredentialsHint')}
                    </p>
                  </div>
                </>
              ) : null}
            </div>

            <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
              <Button variant='outline' onClick={handleClose} disabled={loading}>
                {t('common.cancel')}
              </Button>
              <Button onClick={handleSubmit} disabled={loading}>
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
