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
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {Switch} from '@/components/ui/switch';
import {Badge} from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import {toast} from 'sonner';
import services from '@/lib/services';
import {HostInfo, HostType, UpdateHostRequest} from '@/lib/services/host/types';

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
        k8s_api_url: host.k8s_api_url,
        k8s_namespace: host.k8s_namespace,
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

    if (host.host_type === HostType.KUBERNETES && formData.k8s_api_url !== undefined) {
      if (formData.k8s_api_url.trim().length === 0) {
        toast.error(t('host.errors.k8sUrlRequired'));
        return false;
      }
      if (
        !formData.k8s_api_url.startsWith('https://') &&
        !formData.k8s_api_url.startsWith('http://')
      ) {
        toast.error(t('host.errors.k8sUrlInvalid'));
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
    if (!validateForm()) {
      return;
    }

    setLoading(true);
    try {
      const result = await services.host.updateHostSafe(host.id, formData);
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
      <DialogContent className='sm:max-w-[500px] max-h-[90vh] overflow-y-auto'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            {t('host.editHost')}
            <Badge variant='outline'>
              {t(`host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`)}
            </Badge>
          </DialogTitle>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          {/* Host Name / 主机名称 */}
          <div className='space-y-2'>
            <Label htmlFor='edit-name'>{t('host.name')} *</Label>
            <Input
              id='edit-name'
              value={formData.name || ''}
              onChange={(e) => setFormData({...formData, name: e.target.value})}
              placeholder={t('host.namePlaceholder')}
            />
          </div>

          {/* Description / 描述 */}
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

          {/* Bare Metal Fields / 物理机字段 */}
          {host.host_type === HostType.BARE_METAL && (
            <>
              <div className='space-y-2'>
                <Label htmlFor='edit-ip_address'>{t('host.ipAddress')} *</Label>
                <Input
                  id='edit-ip_address'
                  value={formData.ip_address || ''}
                  onChange={(e) =>
                    setFormData({...formData, ip_address: e.target.value})
                  }
                  placeholder='192.168.1.100'
                />
              </div>
            </>
          )}

          {/* Docker Fields / Docker 字段 */}
          {host.host_type === HostType.DOCKER && (
            <>
              <div className='space-y-2'>
                <Label htmlFor='edit-docker_api_url'>{t('host.dockerApiUrl')} *</Label>
                <Input
                  id='edit-docker_api_url'
                  value={formData.docker_api_url || ''}
                  onChange={(e) =>
                    setFormData({...formData, docker_api_url: e.target.value})
                  }
                  placeholder='tcp://192.168.1.100:2375'
                />
                <p className='text-xs text-muted-foreground'>
                  {t('host.dockerApiUrlHint')}
                </p>
              </div>
              <div className='flex items-center space-x-2'>
                <Switch
                  id='edit-docker_tls_enabled'
                  checked={formData.docker_tls_enabled || false}
                  onCheckedChange={(checked) =>
                    setFormData({...formData, docker_tls_enabled: checked})
                  }
                />
                <Label htmlFor='edit-docker_tls_enabled'>{t('host.tlsEnabled')}</Label>
              </div>
              {formData.docker_tls_enabled && (
                <div className='space-y-2'>
                  <Label htmlFor='edit-docker_cert_path'>{t('host.certPath')}</Label>
                  <Input
                    id='edit-docker_cert_path'
                    value={formData.docker_cert_path || ''}
                    onChange={(e) =>
                      setFormData({...formData, docker_cert_path: e.target.value})
                    }
                    placeholder='/path/to/certs'
                  />
                </div>
              )}
            </>
          )}

          {/* Kubernetes Fields / Kubernetes 字段 */}
          {host.host_type === HostType.KUBERNETES && (
            <>
              <div className='space-y-2'>
                <Label htmlFor='edit-k8s_api_url'>{t('host.k8sApiUrl')} *</Label>
                <Input
                  id='edit-k8s_api_url'
                  value={formData.k8s_api_url || ''}
                  onChange={(e) =>
                    setFormData({...formData, k8s_api_url: e.target.value})
                  }
                  placeholder='https://kubernetes.default.svc:6443'
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='edit-k8s_namespace'>{t('host.namespace')}</Label>
                <Input
                  id='edit-k8s_namespace'
                  value={formData.k8s_namespace || ''}
                  onChange={(e) =>
                    setFormData({...formData, k8s_namespace: e.target.value})
                  }
                  placeholder='default'
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='edit-k8s_kubeconfig'>{t('host.kubeconfig')}</Label>
                <Textarea
                  id='edit-k8s_kubeconfig'
                  value={formData.k8s_kubeconfig || ''}
                  onChange={(e) =>
                    setFormData({...formData, k8s_kubeconfig: e.target.value})
                  }
                  placeholder={t('host.kubeconfigPlaceholder')}
                  rows={3}
                />
                <p className='text-xs text-muted-foreground'>
                  {t('host.leaveEmptyToKeep')}
                </p>
              </div>
              <div className='space-y-2'>
                <Label htmlFor='edit-k8s_token'>{t('host.token')}</Label>
                <Textarea
                  id='edit-k8s_token'
                  value={formData.k8s_token || ''}
                  onChange={(e) =>
                    setFormData({...formData, k8s_token: e.target.value})
                  }
                  placeholder={t('host.tokenPlaceholder')}
                  rows={2}
                />
                <p className='text-xs text-muted-foreground'>
                  {t('host.leaveEmptyToKeep')}
                </p>
              </div>
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={handleClose} disabled={loading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading ? t('common.saving') : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
