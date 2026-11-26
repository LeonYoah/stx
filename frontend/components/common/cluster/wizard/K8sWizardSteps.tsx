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

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Checkbox} from '@/components/ui/checkbox';
import {Button} from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {K8sImagePicker} from '@/components/common/k8s/K8sImagePicker';
import apiClient from '@/lib/services/core/api-client';
import type {K8sWizardFormState} from './types';

type Patch = (partial: Partial<K8sWizardFormState>) => void;

interface StepProps {
  form: K8sWizardFormState;
  patch: Patch;
}

// memoryToMi 解析 K8s 资源内存量为 Mi；无法解析返回 0。
// memoryToMi parses a Kubernetes memory quantity into Mi; unparseable yields 0.
export function memoryToMi(input: string): number {
  const match = input.trim().match(/^(\d+(?:\.\d+)?)(Mi|Gi|M|G)?$/i);
  if (!match) return 0;
  const value = parseFloat(match[1]);
  const unit = (match[2] || 'Mi').toLowerCase();
  if (unit === 'gi' || unit === 'g') return Math.round(value * 1024);
  if (unit === 'm') return Math.round(value * 1000 * 1000 / 1048576);
  return Math.round(value);
}

// jvmHeapToMi 解析 -Xmx 风格堆参数为 Mi。
// jvmHeapToMi parses an -Xmx style heap value into Mi.
export function jvmHeapToMi(input: string): number {
  const match = input.trim().match(/^(\d+)(m|g)?$/i);
  if (!match) return 0;
  const value = parseInt(match[1], 10);
  return (match[2] || 'm').toLowerCase() === 'g' ? value * 1024 : value;
}

// validateJvmVsContainer JVM 堆不得超过容器内存的 80%，返回错误文案或空串。
// validateJvmVsContainer enforces heap ≤ 80% of container memory, returning the
// error copy or an empty string.
export function validateJvmVsContainer(xmx: string, containerMemory: string): string {
  const heapMi = jvmHeapToMi(xmx);
  const containerMi = memoryToMi(containerMemory);
  if (!heapMi || !containerMi) return '';
  if (heapMi > containerMi * 0.8) {
    return `Xmx ${heapMi}Mi 超过容器内存 ${containerMi}Mi 的 80% 上限，请调低堆内存或调大容器内存`;
  }
  return '';
}

// K8sBasicContent 第 1 步：集群基础信息 + 镜像选择。
// K8sBasicContent is step 1: cluster basics plus image selection.
export function K8sBasicContent({form, patch}: StepProps) {
  const t = useTranslations();
  return (
    <div className='space-y-4' data-testid='k8s-basic-step'>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Label>{t('cluster.name')}</Label>
          <Input
            value={form.name}
            onChange={(e) => patch({name: e.target.value})}
            placeholder='seatunnel-prod'
            data-testid='k8s-name-input'
          />
        </div>
        <div className='space-y-1.5'>
          <Label>{t('cluster.k8s.version')}</Label>
          <Input value={form.version} readOnly disabled placeholder='auto' data-testid='k8s-version-input' />
        </div>
      </div>
      <div className='space-y-1.5'>
        <Label>{t('cluster.k8s.image')}</Label>
        <K8sImagePicker
          value={form.image}
          onChange={(next) =>
            patch({
              image: next.image,
              version: next.seatunnelVersion || form.version,
              image_pull_policy: next.pullPolicy,
              image_pull_secrets: next.pullSecrets || form.image_pull_secrets,
            })
          }
        />
      </div>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Label>{t('cluster.clusterCode')}</Label>
          <Input
            value={form.cluster_code}
            onChange={(e) => patch({cluster_code: e.target.value})}
            placeholder={t('common.optional')}
            data-testid='k8s-cluster-code-input'
          />
        </div>
        <div className='space-y-1.5'>
          <Label>{t('cluster.description')}</Label>
          <Input value={form.description} onChange={(e) => patch({description: e.target.value})} />
        </div>
      </div>
    </div>
  );
}

// K8sConfigContent 第 3 步：端口 + JVM 堆内存与容器内存联动校验 + 事件上报 + 存储。
// K8sConfigContent is step 3: ports, JVM heap linked with container memory,
// event reporting, and runtime storage.
export function K8sConfigContent({form, patch}: StepProps) {
  const t = useTranslations();
  const containerMemory =
    form.deployment_mode === 'hybrid' ? form.hybrid_memory : form.worker_memory;
  const heapError = validateJvmVsContainer(form.jvm_xmx, containerMemory);

  const patchStorage = (kind: 'checkpoint' | 'imap', partial: Record<string, unknown>) =>
    patch({[kind]: {...form[kind], ...partial}} as Partial<K8sWizardFormState>);

  const storageEditor = (kind: 'checkpoint' | 'imap') => {
    const storage = form[kind];
    return (
      <div className='space-y-2 rounded-lg border p-3' data-testid={`k8s-${kind}-editor`}>
        <label className='flex items-center gap-2 text-sm font-medium'>
          <Checkbox
            checked={storage.enabled}
            onCheckedChange={(checked) => patchStorage(kind, {enabled: checked === true})}
          />
          {t(`cluster.k8s.${kind}Title`)}
        </label>
        {storage.enabled ? (
          <>
            <div className='grid grid-cols-2 gap-3'>
              <Select
                value={storage.storage_type}
                onValueChange={(value) => patchStorage(kind, {storage_type: value})}
              >
                <SelectTrigger data-testid={`k8s-${kind}-type`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='LOCAL_FILE'>LOCAL_FILE</SelectItem>
                  <SelectItem value='S3'>S3</SelectItem>
                  <SelectItem value='OSS'>OSS</SelectItem>
                  <SelectItem value='HDFS'>HDFS</SelectItem>
                </SelectContent>
              </Select>
              <Input
                placeholder={t('cluster.k8s.storageNamespace')}
                value={storage.namespace}
                onChange={(e) => patchStorage(kind, {namespace: e.target.value})}
              />
            </div>
            {storage.storage_type === 'LOCAL_FILE' ? (
              <p className='rounded-md border border-warning/40 bg-warning/10 px-2.5 py-1.5 text-xs text-warning' data-testid={`k8s-${kind}-localfile-warning`}>
                {t('cluster.k8s.localFileWarning')}
              </p>
            ) : (
              <div className='grid grid-cols-2 gap-3'>
                <Input placeholder='endpoint' value={storage.endpoint} onChange={(e) => patchStorage(kind, {endpoint: e.target.value})} />
                <Input placeholder='bucket' value={storage.bucket} onChange={(e) => patchStorage(kind, {bucket: e.target.value})} />
                <Input placeholder='access key' value={storage.access_key} onChange={(e) => patchStorage(kind, {access_key: e.target.value})} />
                <Input placeholder='secret key' type='password' value={storage.secret_key} onChange={(e) => patchStorage(kind, {secret_key: e.target.value})} />
              </div>
            )}
          </>
        ) : null}
      </div>
    );
  };

  return (
    <div className='space-y-4' data-testid='k8s-config-step'>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Label>{t('cluster.k8s.masterHttpPort')}</Label>
          <Input
            type='number'
            value={form.master_http_port}
            onChange={(e) => patch({master_http_port: Number(e.target.value) || 8080})}
            data-testid='k8s-http-port'
          />
        </div>
        <div className='space-y-1.5'>
          <Label>{t('cluster.k8s.hazelcastPort')}</Label>
          <Input
            type='number'
            value={form.hazelcast_port}
            onChange={(e) => patch({hazelcast_port: Number(e.target.value) || 5801})}
            data-testid='k8s-hazelcast-port'
          />
        </div>
      </div>

      <div className='space-y-2 rounded-lg border p-3'>
        <p className='text-sm font-medium'>{t('cluster.k8s.jvmTitle')}</p>
        <div className='grid grid-cols-2 gap-3'>
          <div className='space-y-1.5'>
            <Label>Xms</Label>
            <Input value={form.jvm_xms} onChange={(e) => patch({jvm_xms: e.target.value})} placeholder='1g' data-testid='k8s-jvm-xms' />
          </div>
          <div className='space-y-1.5'>
            <Label>Xmx</Label>
            <Input value={form.jvm_xmx} onChange={(e) => patch({jvm_xmx: e.target.value})} placeholder='2g' data-testid='k8s-jvm-xmx' />
          </div>
        </div>
        {heapError ? (
          <p className='text-xs font-medium text-destructive' data-testid='k8s-jvm-heap-error'>{heapError}</p>
        ) : (
          <p className='text-xs text-muted-foreground'>{t('cluster.k8s.jvmHint', {memory: containerMemory})}</p>
        )}
      </div>

      <div className='space-y-2 rounded-lg border p-3'>
        <label className='flex items-center gap-2 text-sm font-medium'>
          <Checkbox
            checked={form.enable_http_server}
            onCheckedChange={(checked) => patch({enable_http_server: checked === true})}
          />
          {t('cluster.k8s.enableHttp')}
        </label>
        <label className='flex items-center gap-2 text-sm font-medium'>
          <Checkbox
            checked={form.event_report_enabled}
            onCheckedChange={(checked) => patch({event_report_enabled: checked === true})}
            data-testid='k8s-event-report-toggle'
          />
          {t('cluster.k8s.eventReport')}
        </label>
        <p className='text-xs text-muted-foreground'>{t('cluster.k8s.eventReportHint')}</p>
      </div>

      {storageEditor('checkpoint')}
      {storageEditor('imap')}
    </div>
  );
}

interface K8sPrecheckItem {
  name: string;
  passed: boolean;
  level: string;
  message: string;
}

interface K8sPrecheckResult {
  passed: boolean;
  items: K8sPrecheckItem[];
}

// K8sPrecheckContent 第 4 步：调真实预检端点展示分项结果。
// K8sPrecheckContent is step 4: runs the real precheck endpoint and shows items.
export function K8sPrecheckContent({form}: {form: K8sWizardFormState}) {
  const t = useTranslations();
  const [result, setResult] = useState<K8sPrecheckResult | null>(null);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');

  const run = async () => {
    setRunning(true);
    setError('');
    try {
      const response = await apiClient.post<K8sPrecheckResult>('/clusters/k8s/precheck', {
        host_id: form.host_id,
        namespace: form.namespace,
        cluster_name: form.name,
        deployment_mode: form.deployment_mode,
        image_pull_secrets: form.image_pull_secrets,
        service_account: form.service_account,
        checkpoint_pvc: form.checkpoint_pvc,
      });
      setResult(response.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setRunning(false);
    }
  };

  useEffect(() => {
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className='space-y-3' data-testid='k8s-precheck-step'>
      <div className='flex items-center justify-between'>
        <p className='text-sm text-muted-foreground'>{t('cluster.k8s.precheckDesc')}</p>
        <Button size='sm' variant='outline' onClick={() => void run()} disabled={running} data-testid='k8s-precheck-rerun'>
          {running ? t('common.loading') : t('cluster.k8s.precheckRerun')}
        </Button>
      </div>
      {error ? <p className='text-xs text-destructive'>{error}</p> : null}
      <ul className='space-y-1.5'>
        {(result?.items || []).map((item) => (
          <li
            key={item.name}
            className={`rounded-md border px-3 py-2 text-xs ${
              item.passed ? 'border-success/30 bg-success/5' : 'border-destructive/30 bg-destructive/5'
            }`}
            data-testid={`k8s-precheck-item-${item.name}`}
          >
            <span className='font-medium'>{item.name}</span>
            <span className='ml-2 text-muted-foreground'>{item.message}</span>
          </li>
        ))}
      </ul>
      {result && !result.passed ? (
        <p className='text-xs font-medium text-destructive' data-testid='k8s-precheck-blocked'>
          {t('cluster.k8s.precheckBlocked')}
        </p>
      ) : null}
    </div>
  );
}

// buildK8sCreatePayload 把向导表单映射为创建请求（与后端 K8sClusterCreateRequest 同构）。
// buildK8sCreatePayload maps the wizard form into the create request shaped like
// the backend K8sClusterCreateRequest.
export function buildK8sCreatePayload(form: K8sWizardFormState): Record<string, unknown> {
  return {
    name: form.name,
    cluster_code: form.cluster_code,
    description: form.description,
    version: form.version,
    host_id: form.host_id,
    management_source: 'stx_deployed',
    namespace: form.namespace,
    image: form.image,
    image_pull_policy: form.image_pull_policy,
    image_pull_secrets: form.image_pull_secrets,
    service_account: form.service_account,
    deployment_mode: form.deployment_mode,
    master_replicas: form.master_replicas,
    worker_replicas: form.worker_replicas,
    hybrid_replicas: form.hybrid_replicas,
    master_http_port: form.master_http_port,
    hazelcast_port: form.hazelcast_port,
    master_cpu: form.master_cpu,
    master_memory: form.master_memory,
    worker_cpu: form.worker_cpu,
    worker_memory: form.worker_memory,
    hybrid_cpu: form.hybrid_cpu,
    hybrid_memory: form.hybrid_memory,
    checkpoint_pvc: form.checkpoint_pvc,
    data_storage_class: form.data_storage_class,
    data_size: form.data_size,
    jvm: {xms: form.jvm_xms, xmx: form.jvm_xmx},
    runtime: {
      enable_http_server: form.enable_http_server,
      event_report_http: '',
    },
    checkpoint: form.checkpoint.enabled ? form.checkpoint : undefined,
    imap: form.imap.enabled ? form.imap : undefined,
  };
}
