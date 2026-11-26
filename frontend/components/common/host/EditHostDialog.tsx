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
 * Edit Host Dialog Component
 * 编辑主机对话框组件
 *
 * Dialog for editing an existing host.
 * 用于编辑现有主机的对话框。
 */

import {useState, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Switch} from '@/components/arc/switch/switch';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {Badge} from '@/components/ui/badge';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  HostInfo,
  HostType,
  K8sAuthMode,
  UpdateHostRequest,
} from '@/lib/services/host/types';

interface EditHostDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  host: HostInfo;
  onSuccess: () => void;
}

/**
 * Edit Host Dialog Component
 * 编辑主机对话框组件
 */
export function EditHostDialog({
  open,
  onOpenChange,
  host,
  onSuccess,
}: EditHostDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState<UpdateHostRequest>({});

  const k8sAuthMode: K8sAuthMode =
    formData.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig';

  /**
   * Initialize form data when host changes
   * 当主机变化时初始化表单数据
   */
  useEffect(() => {
    if (host) {
      setFormData({
        name: host.name,
        description: host.description || '',
        ip_address: host.ip_address,
        docker_api_url: host.docker_api_url,
        docker_tls_enabled: host.docker_tls_enabled,
        k8s_api_url: host.k8s_api_url || '',
        k8s_namespace: host.k8s_namespace || 'default',
        k8s_auth_mode: host.k8s_auth_mode === 'token' ? 'token' : 'kubeconfig',
        k8s_kubeconfig: '',
        k8s_token: '',
        k8s_ca_cert: '',
        k8s_insecure_skip_tls: Boolean(host.k8s_insecure_skip_tls),
      });
    }
  }, [host]);


  /**
   * Handle dialog close
   * 处理对话框关闭
   */
  const handleClose = () => {
    onOpenChange(false);
  };

  /**
   * Validate form data
   * 验证表单数据
   */
  const validateForm = (): boolean => {
    if (formData.name !== undefined && formData.name.trim().length === 0) {
      toast.error(t('host.errors.nameRequired'));
      return false;
    }

    if (host.host_type === HostType.BARE_METAL && formData.ip_address !== undefined) {
      if (formData.ip_address.trim().length === 0) {
        toast.error(t('host.errors.ipRequired'));
        return false;
      }
      const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$|^([a-fA-F0-9:]+)$/;
      if (!ipRegex.test(formData.ip_address)) {
        toast.error(t('host.errors.ipInvalid'));
        return false;
      }
    }

    if (host.host_type === HostType.DOCKER && formData.docker_api_url !== undefined) {
      if (formData.docker_api_url.trim().length === 0) {
        toast.error(t('host.errors.dockerUrlRequired'));
        return false;
      }
      if (
        !formData.docker_api_url.startsWith('tcp://') &&
        !formData.docker_api_url.startsWith('unix://')
      ) {
        toast.error(t('host.errors.dockerUrlInvalid'));
        return false;
      }
    }

    if (host.host_type === HostType.KUBERNETES) {
      if (k8sAuthMode === 'kubeconfig') {
        if (
          !host.has_k8s_kubeconfig &&
          (!formData.k8s_kubeconfig || formData.k8s_kubeconfig.trim().length === 0)
        ) {
          toast.error(t('host.errors.k8sKubeconfigRequired'));
          return false;
        }
      } else {
        const apiUrl = (formData.k8s_api_url || '').trim();
        if (apiUrl.length === 0) {
          toast.error(t('host.errors.k8sUrlRequired'));
          return false;
        }
        if (!apiUrl.startsWith('https://') && !apiUrl.startsWith('http://')) {
          toast.error(t('host.errors.k8sUrlInvalid'));
          return false;
        }
        if (
          !host.has_k8s_token &&
          (!formData.k8s_token || formData.k8s_token.trim().length === 0)
        ) {
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
    if (!validateForm()) {
      return;
    }

    setLoading(true);
    try {
      let payload: UpdateHostRequest = formData;
      if (host.host_type === HostType.KUBERNETES) {
        // 仅提交当前认证方式对应字段，防止切换模式时另一模式的空/旧值污染请求。
        // Send only fields belonging to the active auth mode so switching modes stays clean.
        if (k8sAuthMode === 'kubeconfig') {
          payload = {
            name: formData.name,
            description: formData.description,
            k8s_auth_mode: 'kubeconfig',
            k8s_namespace: formData.k8s_namespace?.trim() || 'default',
            ...(formData.k8s_kubeconfig?.trim()
              ? {k8s_kubeconfig: formData.k8s_kubeconfig.trim()}
              : {}),
          };
        } else {
          const skipTLS = Boolean(formData.k8s_insecure_skip_tls);
          payload = {
            name: formData.name,
            description: formData.description,
            k8s_auth_mode: 'token',
            k8s_api_url: formData.k8s_api_url?.trim(),
            k8s_namespace: formData.k8s_namespace?.trim() || 'default',
            k8s_insecure_skip_tls: skipTLS,
            ...(formData.k8s_token?.trim()
              ? {k8s_token: formData.k8s_token.trim()}
              : {}),
            ...(!skipTLS && formData.k8s_ca_cert?.trim()
              ? {k8s_ca_cert: formData.k8s_ca_cert.trim()}
              : {}),
          };
        }
      }
      const result = await services.host.updateHostSafe(host.id, payload);
      if (result.success) {
        onSuccess();
      } else {
        toast.error(result.error || t('host.updateError'));
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('host.updateError'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        title={
          <span className='inline-flex items-center gap-2'>
            {t('host.editHost')}
            <Badge variant='outline'>
              {t(
                `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
              )}
            </Badge>
          </span>
        }
        className='arc-compact w-[min(calc(100vw-2rem),560px)]!'
        bodyClassName='max-h-[min(85dvh,720px)] overflow-y-auto'
      >
        <div className='space-y-4'>
          <Input
            id='edit-name'
            label={`${t('host.name')} *`}
            value={formData.name || ''}
            onChange={(e) => setFormData({...formData, name: e.target.value})}
            placeholder={t('host.namePlaceholder')}
          />

          <div className='space-y-2'>
            <Label htmlFor='edit-description'>{t('host.descriptionLabel')}</Label>
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

          {host.host_type === HostType.BARE_METAL ? (
            <Input
              id='edit-ip_address'
              label={`${t('host.ipAddress')} *`}
              value={formData.ip_address || ''}
              onChange={(e) =>
                setFormData({...formData, ip_address: e.target.value})
              }
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
                onChange={(e) =>
                  setFormData({...formData, docker_api_url: e.target.value})
                }
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
            <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
              <div className='space-y-1.5'>
                <Label>{t('host.k8sAuthMode')}</Label>
                <SegmentedControl
                  label={t('host.k8sAuthMode')}
                  value={k8sAuthMode}
                  onValueChange={(v) =>
                    setFormData({
                      ...formData,
                      k8s_auth_mode: v as K8sAuthMode,
                    })
                  }
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
                  setFormData({...formData, k8s_namespace: e.target.value})
                }
                placeholder='default'
                className='font-mono text-xs'
              />
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
                  <div className='space-y-1.5'>
                    <Label htmlFor='edit-k8s_kubeconfig'>
                      {host.has_k8s_kubeconfig
                        ? t('host.kubeconfig')
                        : `${t('host.kubeconfig')} *`}
                    </Label>
                    <Textarea
                      id='edit-k8s_kubeconfig'
                      value={formData.k8s_kubeconfig || ''}
                      onChange={(e) =>
                        setFormData({...formData, k8s_kubeconfig: e.target.value})
                      }
                      placeholder={t('host.kubeconfigPlaceholder')}
                      className='font-mono text-xs'
                      rows={6}
                    />
                    <p className='text-xs text-muted-foreground'>
                      {host.has_k8s_kubeconfig
                        ? t('host.k8sKubeconfigEditHint')
                        : t('host.k8sKubeconfigAutoHint')}
                    </p>
                  </div>
                </>
              ) : (
                <>
                  <Input
                    id='edit-k8s_api_url'
                    label={`${t('host.k8sApiUrl')} *`}
                    value={formData.k8s_api_url || ''}
                    onChange={(e) =>
                      setFormData({...formData, k8s_api_url: e.target.value})
                    }
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
                      onChange={(e) =>
                        setFormData({...formData, k8s_token: e.target.value})
                      }
                      placeholder={t('host.tokenPlaceholder')}
                      className='font-mono text-xs'
                      rows={3}
                    />
                    {host.has_k8s_token ? (
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
                          setFormData({...formData, k8s_ca_cert: e.target.value})
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
            {loading ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
