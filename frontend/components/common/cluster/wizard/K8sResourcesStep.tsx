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

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Button} from '@/components/arc/button/button';
import services from '@/lib/services';
import type {HostInfo} from '@/lib/services/host/types';
import type {K8sWizardFormState} from './types';

interface K8sResourcesStepProps {
  form: K8sWizardFormState;
  onChange: (updater: (prev: K8sWizardFormState) => K8sWizardFormState) => void;
}

// K8sResourcesStep 统一向导第 2 步（K8s）：目标 K8s 主机 + namespace + 形态资源配比。
// K8sResourcesStep is wizard step 2 for Kubernetes: target host, namespace, and
// topology resource sizing.
export function K8sResourcesStep({form, onChange}: K8sResourcesStepProps) {
  const t = useTranslations();
  const [hosts, setHosts] = useState<HostInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [hostError, setHostError] = useState<string | null>(null);

  // 主机清单加载失败必须显式可重试，禁止静默吞错后留下无法解释的空选项。
  // Host list failures surface a retry action; never swallow errors silently.
  const loadHosts = useCallback(() => {
    setLoading(true);
    setHostError(null);
    services.host
      .getAllHostsByTypeSafe('kubernetes')
      .then((result) => {
        if (result?.hosts) setHosts(result.hosts);
      })
      .catch((err) => {
        setHostError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    loadHosts();
  }, [loadHosts]);

  const patch = (partial: Partial<K8sWizardFormState>) =>
    onChange((prev) => ({...prev, ...partial}));

  const separated = form.deployment_mode !== 'hybrid';

  return (
    <div className='space-y-4' data-testid='k8s-resources-step'>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Select
            label={t('cluster.k8s.targetHost')}
            value={form.host_id ? String(form.host_id) : ''}
            onValueChange={(value) => patch({host_id: Number(value)})}
            disabled={loading}
            placeholder={t('cluster.k8s.selectHost')}
            options={hosts.map((host) => ({value: String(host.id), label: host.name}))}
            data-testid='k8s-host-select'
          />
          {hostError ? (
            <div className='flex items-center gap-2'>
              <p className='text-xs text-destructive'>{hostError}</p>
              <Button type='button' variant='secondary' size='sm' onClick={loadHosts}>
                {t('common.retry')}
              </Button>
            </div>
          ) : null}
        </div>
        <div className='space-y-1.5'>
          <Input
            label='Namespace'
            value={form.namespace}
            onChange={(event) => patch({namespace: event.target.value})}
            placeholder='default'
            data-testid='k8s-namespace-input'
          />
        </div>
      </div>

      {separated ? (
        <div className='space-y-3'>
          <div className='grid grid-cols-3 gap-3'>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.masterReplicas')}
                type='number'
                min={1}
                value={form.master_replicas}
                onChange={(event) => patch({master_replicas: Number(event.target.value) || 1})}
                data-testid='k8s-master-replicas'
              />
            </div>
            <div className='space-y-1.5'>
              <Input label={t('cluster.k8s.masterCpu')} value={form.master_cpu} onChange={(e) => patch({master_cpu: e.target.value})} placeholder='0.5' />
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.masterMemory')}
                value={form.master_memory}
                onChange={(e) => patch({master_memory: e.target.value})}
                placeholder='2Gi'
                data-testid='k8s-master-memory'
              />
            </div>
          </div>
          <div className='grid grid-cols-3 gap-3'>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.workerReplicas')}
                type='number'
                min={1}
                value={form.worker_replicas}
                onChange={(event) => patch({worker_replicas: Number(event.target.value) || 1})}
                data-testid='k8s-worker-replicas'
              />
            </div>
            <div className='space-y-1.5'>
              <Input label={t('cluster.k8s.workerCpu')} value={form.worker_cpu} onChange={(e) => patch({worker_cpu: e.target.value})} placeholder='1' />
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.workerMemory')}
                value={form.worker_memory}
                onChange={(e) => patch({worker_memory: e.target.value})}
                placeholder='2Gi'
                data-testid='k8s-worker-memory'
              />
            </div>
          </div>
        </div>
      ) : (
        <div className='grid grid-cols-3 gap-3'>
          <div className='space-y-1.5'>
            <Input
              label={t('cluster.k8s.hybridReplicas')}
              type='number'
              min={1}
              value={form.hybrid_replicas}
              onChange={(event) => patch({hybrid_replicas: Number(event.target.value) || 1})}
              data-testid='k8s-hybrid-replicas'
            />
          </div>
          <div className='space-y-1.5'>
            <Input label={t('cluster.k8s.hybridCpu')} value={form.hybrid_cpu} onChange={(e) => patch({hybrid_cpu: e.target.value})} placeholder='1' />
          </div>
          <div className='space-y-1.5'>
            <Input
              label={t('cluster.k8s.hybridMemory')}
              value={form.hybrid_memory}
              onChange={(e) => patch({hybrid_memory: e.target.value})}
              placeholder='4Gi'
              data-testid='k8s-hybrid-memory'
            />
          </div>
        </div>
      )}

      <div className='grid grid-cols-3 gap-3 border-t pt-3'>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.k8s.checkpointPvc')}
            value={form.checkpoint_pvc}
            onChange={(e) => patch({checkpoint_pvc: e.target.value})}
            placeholder='optional'
            data-testid='k8s-checkpoint-pvc'
          />
        </div>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.k8s.imagePullSecrets')}
            value={form.image_pull_secrets}
            onChange={(e) => patch({image_pull_secrets: e.target.value})}
            placeholder='optional, comma separated'
          />
        </div>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.k8s.serviceAccount')}
            value={form.service_account}
            onChange={(e) => patch({service_account: e.target.value})}
            placeholder='optional'
          />
        </div>
      </div>
    </div>
  );
}
