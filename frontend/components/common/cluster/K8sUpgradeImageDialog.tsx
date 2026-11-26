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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import {K8sImagePicker} from '@/components/common/k8s/K8sImagePicker';
import {toast} from '@/lib/toast';
import services from '@/lib/services';

interface K8sUpgradeImageDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  clusterId: number;
  currentImage: string;
  onUpgraded?: () => void;
}

/**
 * K8s 集群统一升级入口：遵循不可变镜像原则，滚动升级 Master/Worker 与 STX Java Proxy
 * 至新镜像 Tag——K8s 集群不存在"二进制分发"概念，升级即换镜像。
 * Unified upgrade entry for K8s clusters: following the immutable-image principle,
 * rolls Master/Worker and the STX Java Proxy to a new image tag — there is no binary
 * distribution concept on K8s, upgrading means swapping the image.
 */
export function K8sUpgradeImageDialog({
  open,
  onOpenChange,
  clusterId,
  currentImage,
  onUpgraded,
}: K8sUpgradeImageDialogProps) {
  const t = useTranslations();
  const [target, setTarget] = useState('');
  const [targetVersion, setTargetVersion] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const submit = async () => {
    const image = target.trim();
    if (!image) {
      return;
    }
    setSubmitting(true);
    try {
      await services.cluster.upgradeK8sImage(clusterId, image, targetVersion || undefined);
      toast.success(t('common.success'));
      onOpenChange(false);
      setTarget('');
      onUpgraded?.();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t('cluster.k8s.upgradeImage')} className='arc-compact w-[min(94vw,520px)]!' data-testid='k8s-upgrade-image-dialog'>
        <div className='space-y-3'>
          <Input label={t('cluster.k8s.upgradeCurrentImage')} value={currentImage || '-'} readOnly disabled />
          <div className='space-y-1'>
            <span className='text-xs font-medium text-foreground'>{t('cluster.k8s.upgradeTargetImage')}</span>
            <K8sImagePicker
              value={target}
              onChange={(next) => {
                setTarget(next.image);
                // 识别出版本时随升级同步 cluster.Version。
                // The resolved version syncs cluster.Version alongside the upgrade.
                setTargetVersion(next.seatunnelVersion);
              }}
            />
          </div>
          <p className='text-xs text-muted-foreground'>{t('cluster.k8s.upgradeImmutableDesc')}</p>
        </div>
        <div className='mt-4 flex justify-end gap-2'>
          <Button size='sm' variant='secondary' onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            size='sm'
            disabled={submitting || !target.trim() || target.trim() === currentImage}
            loading={submitting}
            onClick={() => void submit()}
            data-testid='k8s-upgrade-submit'
          >
            {t('common.confirm')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
