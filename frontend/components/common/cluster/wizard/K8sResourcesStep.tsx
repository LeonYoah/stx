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

const MEMORY_TIERS = ['1Gi', '2Gi', '4Gi', '8Gi', '16Gi'];

// deriveHeapHint 与后端 1/2 推导口径一致的只读提示：Xmx=容器内存/2，Xms=Xmx/2。
// deriveHeapHint mirrors the backend 1/2 derivation for the read-only hint:
// Xmx = container memory / 2, Xms = Xmx / 2.
function deriveHeapHint(memory: string): string {
  const match = /^\s*(\d+(?:\.\d+)?)(Mi|Gi)\s*$/i.exec(memory || '');
  if (!match) return '';
  let mi = parseFloat(match[1]);
  if (/gi/i.test(match[2])) mi *= 1024;
  const xmx = Math.floor(mi / 2);
  if (xmx < 256) return '';
  return `-Xms${Math.floor(xmx / 2)}m -Xmx${xmx}m`;
}

// MemoryTierField 内存档位化输入：常用档位下拉 + 自定义输入兜底 + 堆推导只读小字。
// MemoryTierField is the tiered memory input: quick-pick dropdown, custom-input
// fallback, and the read-only heap derivation hint.
function MemoryTierField({label, value, onChange, testId, customLabel, hintLabel}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
  testId?: string;
  customLabel: string;
  hintLabel: string;
}) {
  const isTier = MEMORY_TIERS.includes(value);
  const tierValue = isTier ? value : '__custom__';
  const hint = deriveHeapHint(value);
  return (
    <div className='space-y-1.5'>
      <Select
        label={label}
        value={tierValue}
        onValueChange={(next) => {
          if (next !== '__custom__') onChange(next);
          else if (isTier) onChange('');
        }}
        options={[...MEMORY_TIERS.map((tier) => ({value: tier, label: tier})), {value: '__custom__', label: customLabel}]}
        data-testid={testId}
      />
      {!isTier ? (
        <Input
          label={`${label} · ${customLabel}`}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder='3Gi'
          data-testid={testId ? `${testId}-custom` : undefined}
        />
      ) : null}
      {hint ? <p className='text-xs text-muted-foreground'>{hintLabel.replace('{hint}', hint)}</p> : null}
    </div>
  );
}


// NamespaceObjectField 高级选项对象选择：namespace 内已有对象下拉快选，
// 查询不到或用户需留空/手填时退化为普通输入。
// NamespaceObjectField picks existing namespace objects from a dropdown when
// available, falling back to a plain input for empty/manual values.
function NamespaceObjectField({label, value, onChange, placeholder, objects, testId}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
  placeholder: string;
  objects: string[];
  testId?: string;
}) {
  if (objects.length > 0) {
    return (
      <div className='space-y-1.5'>
        <Select
          label={label}
          value={value}
          onValueChange={onChange}
          options={objects.map((name) => ({value: name, label: name}))}
          placeholder={placeholder}
          data-testid={testId}
        />
      </div>
    );
  }
  return (
    <div className='space-y-1.5'>
      <Input label={label} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} data-testid={testId} />
    </div>
  );
}

// K8sResourcesStep 统一向导第 2 步（K8s）：目标 K8s 主机 + namespace + 形态资源配比。
// K8sResourcesStep is wizard step 2 for Kubernetes: target host, namespace, and
// topology resource sizing.
export function K8sResourcesStep({form, onChange}: K8sResourcesStepProps) {
  const t = useTranslations();
  const [hosts, setHosts] = useState<HostInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [hostError, setHostError] = useState<string | null>(null);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [secrets, setSecrets] = useState<string[]>([]);
  const [serviceAccounts, setServiceAccounts] = useState<string[]>([]);
  const [pvcs, setPvcs] = useState<string[]>([]);

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

  // 仅 1 个可用 K8s 主机时自动默认选中，省掉无信息的单选项点击。
  // Auto-select when exactly one K8s host is available — a single-option click
  // carries no information.
  useEffect(() => {
    if (!loading && hosts.length === 1 && !form.host_id) {
      patch({host_id: hosts[0].id});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hosts, loading]);

  // namespace 下拉随主机选择加载；清单仅供快选，用户仍可输入新建。
  // The namespace dropdown loads with the chosen host; the list is a quick-pick
  // only — typing a new namespace stays supported.
  useEffect(() => {
    if (!form.host_id) {
      setNamespaces([]);
      return;
    }
    services.host
      .listK8sNamespaces(form.host_id)
      .then((list) => {
        if (Array.isArray(list)) setNamespaces(list);
      })
      .catch(() => {
        setNamespaces([]);
      });
  }, [form.host_id]);

  // 高级选项对象清单随 host+namespace 联动加载；失败保持空表退化为手填。
  // Advanced object lists load with host+namespace; failures keep empty lists so
  // fields degrade to manual input.
  useEffect(() => {
    if (!form.host_id || !form.namespace.trim()) {
      setSecrets([]);
      setServiceAccounts([]);
      setPvcs([]);
      return;
    }
    const hostId = form.host_id;
    const namespace = form.namespace.trim();
    const load = (kind: string, setter: (names: string[]) => void) => {
      services.host
        .listK8sNamespaceObjects(hostId, namespace, kind)
        .then((list) => {
          if (Array.isArray(list)) setter(list);
        })
        .catch(() => setter([]));
    };
    load('secrets', setSecrets);
    load('serviceaccounts', setServiceAccounts);
    load('pvcs', setPvcs);
  }, [form.host_id, form.namespace]);

  const separated = form.deployment_mode !== 'hybrid';
  const heapHintText = (memory: string) => t('cluster.k8s.heapDerivationHint', {hint: deriveHeapHint(memory) || '-'});

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
          <p className='text-[11px] text-muted-foreground'>{t('cluster.k8s.hostVersionHint')}</p>
        </div>
        <div className='space-y-1.5'>
          {namespaces.length > 0 && namespaces.includes(form.namespace) ? (
            <Select
              label='Namespace'
              value={form.namespace}
              onValueChange={(value) => patch({namespace: value})}
              options={namespaces.map((ns) => ({value: ns, label: ns}))}
              data-testid='k8s-namespace-select'
            />
          ) : (
            <Input
              label='Namespace'
              value={form.namespace}
              onChange={(event) => patch({namespace: event.target.value})}
              placeholder='default'
              data-testid='k8s-namespace-input'
            />
          )}
          {namespaces.length > 0 && !namespaces.includes(form.namespace) && form.namespace ? (
            <p className='text-xs text-muted-foreground'>{t('cluster.k8s.namespaceCreateNew')}: {form.namespace}</p>
          ) : null}
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
            <MemoryTierField
              label={t('cluster.k8s.masterMemory')}
              value={form.master_memory}
              onChange={(next) => patch({master_memory: next})}
              testId='k8s-master-memory'
              customLabel={t('cluster.k8s.memoryTierCustom')}
              hintLabel={heapHintText(form.master_memory)}
            />
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
            <MemoryTierField
              label={t('cluster.k8s.workerMemory')}
              value={form.worker_memory}
              onChange={(next) => patch({worker_memory: next})}
              testId='k8s-worker-memory'
              customLabel={t('cluster.k8s.memoryTierCustom')}
              hintLabel={heapHintText(form.worker_memory)}
            />
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
          <MemoryTierField
            label={t('cluster.k8s.hybridMemory')}
            value={form.hybrid_memory}
            onChange={(next) => patch({hybrid_memory: next})}
            testId='k8s-hybrid-memory'
            customLabel={t('cluster.k8s.memoryTierCustom')}
            hintLabel={heapHintText(form.hybrid_memory)}
          />
        </div>
      )}

      <div className='border-t-[0.5px] border-border/80 pt-3'>
        <button
          type='button'
          className='text-xs text-muted-foreground hover:text-foreground'
          onClick={() => setAdvancedOpen((open) => !open)}
          data-testid='k8s-advanced-toggle'
        >
          {advancedOpen ? '▾' : '▸'} {t('cluster.k8s.advancedOptions')}
        </button>
        {advancedOpen ? (
          <div className='mt-3 grid grid-cols-3 gap-3'>
            <NamespaceObjectField
              label={t('cluster.k8s.checkpointPvc')}
              value={form.checkpoint_pvc}
              onChange={(next) => patch({checkpoint_pvc: next})}
              placeholder={t('cluster.k8s.optionalPlaceholder')}
              objects={pvcs}
              testId='k8s-checkpoint-pvc'
            />
            <NamespaceObjectField
              label={t('cluster.k8s.imagePullSecrets')}
              value={form.image_pull_secrets}
              onChange={(next) => patch({image_pull_secrets: next})}
              placeholder={t('cluster.k8s.optionalPlaceholder')}
              objects={secrets}
            />
            <NamespaceObjectField
              label={t('cluster.k8s.serviceAccount')}
              value={form.service_account}
              onChange={(next) => patch({service_account: next})}
              placeholder={t('cluster.k8s.optionalPlaceholder')}
              objects={serviceAccounts}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
}
