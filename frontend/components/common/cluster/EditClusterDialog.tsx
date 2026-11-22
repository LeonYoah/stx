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
 * 编辑集群弹窗：迁移至 Arc Dialog（.arc-compact）+ 单层画布 0.5px 细线 + 脏态拦截。
 * Edit cluster dialog: Arc Dialog (.arc-compact) + flat single-layer 0.5px dividers + DirtyCloseDialog.
 */
import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import {DirtyCloseDialog, NeutralChip} from '@/components/common/layout';
import {Textarea} from '@/components/ui/textarea';
import services from '@/lib/services';
import type {
  ClusterInfo,
  UpdateClusterRequest,
} from '@/lib/services/cluster/types';
import {toast} from '@/lib/toast';

interface EditClusterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  cluster: ClusterInfo;
  onSuccess: () => void;
}

export function EditClusterDialog({
  open,
  onOpenChange,
  cluster,
  onSuccess,
}: EditClusterDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [version, setVersion] = useState('');
  const [nameError, setNameError] = useState<string | undefined>(undefined);

  useEffect(() => {
    if (open && cluster) {
      setName(cluster.name);
      setDescription(cluster.description || '');
      setVersion(cluster.version || '');
      setNameError(undefined);
      setConfirmClose(false);
    }
  }, [open, cluster]);

  const isDirty =
    name !== cluster.name ||
    description !== (cluster.description || '') ||
    version !== (cluster.version || '');

  const handleOpenChange = (next: boolean) => {
    if (!next && isDirty && !loading) {
      setConfirmClose(true);
      return;
    }
    onOpenChange(next);
  };

  const handleSubmit = async () => {
    if (!name.trim()) {
      setNameError(t('cluster.nameRequired'));
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

  const modeKey = `cluster.modes.${cluster.deployment_mode}`;
  const modeLabel = t.has(modeKey) ? t(modeKey) : cluster.deployment_mode;

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent
          title={t('cluster.editCluster')}
          description={t('cluster.editClusterDescription')}
          className='arc-compact w-[min(92vw,520px)]!'
        >
          <div className='space-y-4'>
            {/* 只读标识信息条，替代置灰不可编辑输入框 / Read-only metadata strip instead of disabled inputs */}
            <div className='flex flex-wrap items-center gap-x-4 gap-y-2 border-b-[0.5px] border-border/80 pb-4 text-xs'>
              <div className='flex items-center gap-1.5'>
                <span className='text-muted-foreground'>
                  {t('cluster.clusterCode')}:
                </span>
                <span className='font-mono font-medium text-foreground'>
                  {cluster.cluster_code || '-'}
                </span>
              </div>
              <div className='flex items-center gap-1.5'>
                <span className='text-muted-foreground'>
                  {t('cluster.deploymentMode')}:
                </span>
                <NeutralChip>{modeLabel}</NeutralChip>
              </div>
            </div>

            <div className='space-y-4'>
              <Input
                id='edit-name'
                label={t('cluster.name')}
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  if (nameError) {
                    setNameError(undefined);
                  }
                }}
                placeholder={t('cluster.namePlaceholder')}
                error={nameError}
                required
              />

              <div className='space-y-1.5'>
                <label
                  htmlFor='edit-description'
                  className='block text-xs font-medium text-foreground'
                >
                  {t('cluster.descriptionLabel')}
                </label>
                <Textarea
                  id='edit-description'
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t('cluster.descriptionPlaceholder')}
                  rows={2}
                />
              </div>

              <Input
                id='edit-version'
                label={t('cluster.version')}
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                placeholder={t('cluster.versionPlaceholder')}
                className='font-mono tabular-nums'
              />
            </div>

            <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => handleOpenChange(false)}
                disabled={loading}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                onClick={() => void handleSubmit()}
                disabled={loading}
                loading={loading}
              >
                {t('common.save')}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <DirtyCloseDialog
        open={confirmClose}
        onDiscard={() => {
          setConfirmClose(false);
          onOpenChange(false);
        }}
        onKeepEditing={() => setConfirmClose(false)}
      />
    </>
  );
}
