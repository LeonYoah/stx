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
 * Edit Cluster Dialog Component
 * 编辑集群对话框组件
 *
 * Dialog for editing an existing SeaTunnel cluster.
 * 用于编辑现有 SeaTunnel 集群的对话框。
 */

import {useState, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Loader2} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import {ClusterInfo, UpdateClusterRequest} from '@/lib/services/cluster/types';

interface EditClusterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  cluster: ClusterInfo;
  onSuccess: () => void;
}

/**
 * Edit Cluster Dialog Component
 * 编辑集群对话框组件
 */
export function EditClusterDialog({
  open,
  onOpenChange,
  cluster,
  onSuccess,
}: EditClusterDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);

  // Form state / 表单状态
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [version, setVersion] = useState('');

  /**
   * Initialize form with cluster data
   * 使用集群数据初始化表单
   */
  useEffect(() => {
    if (open && cluster) {
      setName(cluster.name);
      setDescription(cluster.description || '');
      setVersion(cluster.version || '');
    }
  }, [open, cluster]);

  /**
   * Handle submit
   * 处理提交
   */
  const handleSubmit = async () => {
    // Validate required fields / 验证必填字段
    if (!name.trim()) {
      toast.error(t('cluster.nameRequired'));
      return;
    }

    setLoading(true);
    try {
      const data: UpdateClusterRequest = {
        name: name.trim(),
        description: description.trim() || undefined,
        version: version.trim() || undefined,
      };

      const result = await services.cluster.updateClusterSafe(cluster.id, data);

      if (result.success) {
        toast.success(t('cluster.updateSuccess'));
        onSuccess();
      } else {
        toast.error(result.error || t('cluster.updateError'));
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-[500px]'>
        <DialogHeader>
          <DialogTitle>{t('cluster.editCluster')}</DialogTitle>
          <DialogDescription>{t('cluster.editClusterDescription')}</DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label htmlFor='edit-name'>
              {t('cluster.name')} <span className='text-destructive'>*</span>
            </Label>
            <Input
              id='edit-name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('cluster.namePlaceholder')}
            />
          </div>

          <div className='space-y-2'>
            <Label>{t('cluster.clusterCode')}</Label>
            <Input
              value={cluster.cluster_code || '-'}
              disabled
              className='bg-muted font-mono text-xs'
            />
            <p className='text-xs text-muted-foreground'>
              {t('cluster.clusterCodeHint')}
            </p>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='edit-description'>{t('cluster.descriptionLabel')}</Label>
            <Textarea
              id='edit-description'
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('cluster.descriptionPlaceholder')}
              rows={2}
            />
          </div>

          <div className='space-y-2'>
            <Label>{t('cluster.deploymentMode')}</Label>
            <Input
              value={t(`cluster.modes.${cluster.deployment_mode}`)}
              disabled
              className='bg-muted'
            />
            <p className='text-xs text-muted-foreground'>
              {t('cluster.deploymentModeCannotChange')}
            </p>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='edit-version'>{t('cluster.version')}</Label>
            <Input
              id='edit-version'
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder={t('cluster.versionPlaceholder')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)} disabled={loading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading && <Loader2 className='h-4 w-4 mr-2 animate-spin' />}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
