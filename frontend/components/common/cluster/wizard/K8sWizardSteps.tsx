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
import {Input} from '@/components/arc/input/input';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Button} from '@/components/arc/button/button';
import {Select} from '@/components/arc/select/select';
import {K8sImagePicker} from '@/components/common/k8s/K8sImagePicker';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import {isValidClusterCode, recommendClusterCode} from '@/lib/cluster-code';
import {Layers, Server} from 'lucide-react';
import {RuntimeAdvancedConfigCard} from '@/components/common/installer/RuntimeAdvancedConfigCard';
import {deriveProxyContainerMemory} from '@/lib/k8s-proxy-memory';
import type {RuntimeEngineConfig, SeaTunnelVersionCapabilities} from '@/lib/services/installer/types';
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


// k8sVersionCapabilities 版本能力兜底：K8s 集群版本可任意自定义（私有镜像），
// 无服务端能力矩阵时按版本规则推导，3.x 全量放行、2.3.x 仅基础项。
// k8sVersionCapabilities derives capability fallbacks: Kubernetes cluster versions can
// be arbitrary (private images), so without the server matrix infer by version rules —
// full support on 3.x, basics only on 2.3.x.
export function k8sVersionCapabilities(version: string): SeaTunnelVersionCapabilities | null {
  const trimmed = version.trim().replace(/^v/, '');
  const major = parseInt(trimmed.split('.')[0] || '0', 10);
  if (!Number.isFinite(major) || major <= 0) return null;
  const is3x = major >= 3;
  return {
    supports_dynamic_slot: true,
    supports_slot_num: true,
    supports_history_job_expire_minutes: true,
    supports_scheduled_deletion_enable: is3x,
    supports_job_schedule_strategy: is3x,
    supports_slot_allocation_strategy: is3x,
    supports_http_service: true,
    supports_job_log_mode: is3x,
    supports_event_channel: is3x,
    default_event_channel_enabled: is3x,
    default_dynamic_slot: true,
    default_static_slot_num: 4,
    default_history_job_expire_minutes: 1440,
    default_scheduled_deletion_enable: true,
    default_job_schedule_strategy: 'WAIT',
    default_slot_allocation_strategy: 'RANDOM',
    default_http_enabled: true,
    default_job_log_mode: 'mixed',
    min_java_version: is3x ? 11 : 8,
    recommended_java_versions: is3x ? [11, 17] : [8],
    max_recommended_java_version: is3x ? 17 : 8,
    dynamic_slot: true,
  } as SeaTunnelVersionCapabilities;
}

// K8sBasicContent 第 1 步：集群基础信息 + 部署形态 RadioCards + 镜像选择。
// 版本字段可编辑：镜像识别自动填充但绝不锁死，用户可手工覆盖驱动版本能力。
// K8sBasicContent is step 1: cluster basics, deployment-mode RadioCards, and image
// selection. The version field stays editable: image recognition auto-fills it but
// never locks it, so users can override and drive version capabilities.
export function K8sBasicContent({form, patch}: StepProps) {
  const t = useTranslations();
  const [codeTouched, setCodeTouched] = useState(Boolean(form.cluster_code));
  const [versionTouched, setVersionTouched] = useState(false);
  const codeInvalid = form.cluster_code.length > 0 && !isValidClusterCode(form.cluster_code);
  return (
    <div className='space-y-4' data-testid='k8s-basic-step'>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.name')}
            value={form.name}
            onChange={(e) => {
              const next: Partial<K8sWizardFormState> = {name: e.target.value};
              if (!codeTouched) {
                next.cluster_code = recommendClusterCode(e.target.value);
              }
              patch(next);
            }}
            placeholder='seatunnel-prod'
            data-testid='k8s-name-input'
          />
        </div>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.k8s.version')}
            value={form.version}
            onChange={(e) => {
              setVersionTouched(true);
              patch({version: e.target.value});
            }}
            placeholder='3.1.0'
            data-testid='k8s-version-input'
          />
          <p className='text-xs text-muted-foreground'>
            {versionTouched ? t('cluster.k8s.versionManualHint') : t('cluster.k8s.versionAutoHint')}
          </p>
        </div>
      </div>

      <div className='space-y-1.5'>
        <p className='text-sm font-medium'>{t('cluster.k8s.deploymentMode')}</p>
        <RadioCards
          layout='grid'
          value={form.deployment_mode}
          onValueChange={(value) => patch({deployment_mode: value as 'separated' | 'hybrid'})}
          options={[
            {
              value: 'separated',
              label: t('cluster.k8s.modeSeparated'),
              description: t('cluster.k8s.modeSeparatedDesc'),
              icon: <Server className='h-4 w-4' aria-hidden />,
            },
            {
              value: 'hybrid',
              label: t('cluster.k8s.modeHybrid'),
              description: t('cluster.k8s.modeHybridDesc'),
              icon: <Layers className='h-4 w-4' aria-hidden />,
            },
          ]}
        />
      </div>

      <div className='space-y-1.5'>
        <p className='text-sm font-medium'>{t('cluster.k8s.image')}</p>
        <K8sImagePicker
          value={form.image}
          onChange={(next) =>
            patch({
              image: next.image,
              version: versionTouched ? form.version : next.seatunnelVersion || form.version,
              image_pull_policy: next.pullPolicy,
              image_pull_secrets: next.pullSecrets || form.image_pull_secrets,
            })
          }
        />
      </div>
      <div className='grid grid-cols-2 gap-4'>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.clusterCode')}
            value={form.cluster_code}
            onChange={(e) => {
              setCodeTouched(true);
              patch({cluster_code: e.target.value.toLowerCase().trim()});
            }}
            placeholder={t('common.optional')}
            data-testid='k8s-cluster-code-input'
          />
          {codeInvalid ? (
            <p className='text-xs font-medium text-destructive'>{t('cluster.clusterCodeInvalid')}</p>
          ) : null}
        </div>
        <div className='space-y-1.5'>
          <Input label={t('cluster.description')} value={form.description} onChange={(e) => patch({description: e.target.value})} />
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
  // 回环地址在引擎 Pod 内解析为容器自身，前端与后端同一规则预检拦截。
  // Loopback addresses resolve to the container itself inside engine pods; the
  // frontend pre-checks with the same rule as the backend.
  const callbackUrlLoopback = (() => {
    const raw = (form.stx_callback_url || '').trim();
    if (!raw) return false;
    try {
      const host = new URL(raw).hostname.toLowerCase();
      return host === '' || host === 'localhost' || host === '::1' || host === '[::1]' || host === '0.0.0.0' || host === '::' || host.startsWith('127.');
    } catch {
      return false;
    }
  })();
  // 分角色 80% 红线校验：空值走内存联动默认（容器内存一半），不参与红线判定。
  // Per-role 80% red-line validation: empty values fall back to memory-linked
  // defaults (half the container memory) and skip red-line evaluation.
  const heapErrors = (() => {
    const check = (raw: string, memory: string): string => {
      const heapMb = parseInt(raw, 10);
      if (!Number.isFinite(heapMb) || heapMb <= 0) return '';
      const containerMi = memoryToMi(memory);
      if (!containerMi) return '';
      if (heapMb > containerMi * 0.8) {
        return t('cluster.k8s.heapRedlineError', {heap: String(heapMb), memory: String(containerMi), limit: String(Math.floor(containerMi * 0.8))});
      }
      return '';
    };
    return {
      master: check(form.master_heap_mb, form.master_memory),
      worker: check(form.worker_heap_mb, form.worker_memory),
      hybrid: check(form.hybrid_heap_mb, form.hybrid_memory),
    };
  })();
  void containerMemory;

  const patchStorage = (kind: 'checkpoint' | 'imap', partial: Record<string, unknown>) =>
    patch({[kind]: {...form[kind], ...partial}} as Partial<K8sWizardFormState>);

  const storageEditor = (kind: 'checkpoint' | 'imap') => {
    const storage = form[kind];
    return (
      <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3' data-testid={`k8s-${kind}-editor`}>
        <Checkbox
            label={t(`cluster.k8s.${kind}Title`)}
            checked={storage.enabled}
            onCheckedChange={(checked) => patchStorage(kind, {enabled: checked === true})}
          />
        {storage.enabled ? (
          <>
            <div className='grid grid-cols-2 gap-3'>
              <Select
                aria-label='storage type'
                value={storage.storage_type}
                onValueChange={(value) => patchStorage(kind, {storage_type: value})}
                options={[
                  {value: 'LOCAL_FILE', label: 'LOCAL_FILE'},
                  {value: 'S3', label: 'S3'},
                  {value: 'OSS', label: 'OSS'},
                  {value: 'HDFS', label: 'HDFS'},
                ]}
                data-testid={`k8s-${kind}-type`}
              />
              <Input
                label={t('cluster.k8s.storageNamespace')}
                placeholder={t('cluster.k8s.storageNamespace')}
                value={storage.namespace}
                onChange={(e) => patchStorage(kind, {namespace: e.target.value})}
              />
            </div>
            {storage.storage_type === 'LOCAL_FILE' ? (
              <>
                <p className='rounded-md border border-warning/40 bg-warning/10 px-2.5 py-1.5 text-xs text-warning' data-testid={`k8s-${kind}-localfile-warning`}>
                  {t('cluster.k8s.localFileWarning')}
                </p>
                {kind === 'checkpoint' ? (
                  <Input
                    label={t('cluster.k8s.checkpointPvc')}
                    placeholder={t('cluster.k8s.checkpointPvcPlaceholder')}
                    value={form.checkpoint_pvc}
                    onChange={(e) => patch({checkpoint_pvc: e.target.value})}
                    data-testid='k8s-checkpoint-pvc'
                  />
                ) : null}
              </>
            ) : (
              <div className='space-y-2'>
                <div className='grid grid-cols-2 gap-3'>
                  <Input
                    label={t('cluster.k8s.storageEndpoint')}
                    placeholder={storage.storage_type === 'HDFS' ? 'hdfs://namenode:8020' : 'endpoint'}
                    value={storage.endpoint}
                    onChange={(e) => patchStorage(kind, {endpoint: e.target.value})}
                    data-testid={`k8s-${kind}-endpoint`}
                  />
                  {storage.storage_type !== 'HDFS' ? (
                    <Input label='bucket' value={storage.bucket} onChange={(e) => patchStorage(kind, {bucket: e.target.value})} />
                  ) : null}
                  {storage.storage_type !== 'HDFS' ? (
                    <Input label='access key' value={storage.access_key} onChange={(e) => patchStorage(kind, {access_key: e.target.value})} />
                  ) : null}
                  {storage.storage_type !== 'HDFS' ? (
                    <Input label='secret key' type='password' value={storage.secret_key} onChange={(e) => patchStorage(kind, {secret_key: e.target.value})} />
                  ) : null}
                </div>
                {kind === 'imap' ? (
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    onClick={() => patchStorage('imap', {
                      storage_type: form.checkpoint.storage_type,
                      namespace: form.checkpoint.namespace,
                      endpoint: form.checkpoint.endpoint,
                      bucket: form.checkpoint.bucket,
                      access_key: form.checkpoint.access_key,
                      secret_key: form.checkpoint.secret_key,
                    })}
                    data-testid='k8s-imap-sync-from-checkpoint'
                  >
                    {t('cluster.k8s.imapSyncFromCheckpoint')}
                  </Button>
                ) : null}
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
          <Input
            label={t('cluster.k8s.masterHttpPort')}
            type='number'
            value={form.master_http_port}
            onChange={(e) => patch({master_http_port: Number(e.target.value) || 8080})}
            data-testid='k8s-http-port'
          />
        </div>
        <div className='space-y-1.5'>
          <Input
            label={t('cluster.k8s.hazelcastPort')}
            type='number'
            value={form.hazelcast_port}
            onChange={(e) => patch({hazelcast_port: Number(e.target.value) || 5801})}
            data-testid='k8s-hazelcast-port'
          />
        </div>
      </div>

      <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3'>
        <p className='text-sm font-medium'>{t('cluster.k8s.jvmTitle')}</p>
        {form.deployment_mode === 'hybrid' ? (
          <div className='space-y-1.5'>
            <Input
              label={t('cluster.k8s.hybridHeapMb')}
              type='number'
              min={256}
              value={form.hybrid_heap_mb}
              onChange={(e) => patch({hybrid_heap_mb: e.target.value})}
              placeholder={String(Math.floor(memoryToMi(form.hybrid_memory) / 2))}
              data-testid='k8s-hybrid-heap'
            />
            {heapErrors.hybrid ? (
              <p className='text-xs font-medium text-destructive' data-testid='k8s-jvm-heap-error'>{heapErrors.hybrid}</p>
            ) : (
              <p className='text-xs text-muted-foreground'>{t('cluster.k8s.heapDefaultHint', {memory: form.hybrid_memory, heap: String(Math.floor(memoryToMi(form.hybrid_memory) / 2))})}</p>
            )}
          </div>
        ) : (
          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.masterHeapMb')}
                type='number'
                min={256}
                value={form.master_heap_mb}
                onChange={(e) => patch({master_heap_mb: e.target.value})}
                placeholder={String(Math.floor(memoryToMi(form.master_memory) / 2))}
                data-testid='k8s-master-heap'
              />
              {heapErrors.master ? (
                <p className='text-xs font-medium text-destructive' data-testid='k8s-master-heap-error'>{heapErrors.master}</p>
              ) : null}
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('cluster.k8s.workerHeapMb')}
                type='number'
                min={256}
                value={form.worker_heap_mb}
                onChange={(e) => patch({worker_heap_mb: e.target.value})}
                placeholder={String(Math.floor(memoryToMi(form.worker_memory) / 2))}
                data-testid='k8s-worker-heap'
              />
              {heapErrors.worker ? (
                <p className='text-xs font-medium text-destructive' data-testid='k8s-worker-heap-error'>{heapErrors.worker}</p>
              ) : null}
            </div>
          </div>
        )}
        {!heapErrors.master && !heapErrors.worker && !heapErrors.hybrid ? (
          <p className='text-xs text-muted-foreground'>{t('cluster.k8s.heapRedlineHint')}</p>
        ) : null}
      </div>

      <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3'>
        <div className='flex items-center gap-2 text-sm font-medium' data-testid='k8s-enable-http-locked'>
          <Checkbox checked disabled label={t('cluster.k8s.enableHttp')} />
          <span className='sr-only'>{t('cluster.k8s.enableHttp')}</span>
          <span className='text-xs font-normal text-muted-foreground'>{t('cluster.k8s.enableHttpLockedHint')}</span>
        </div>
        <Checkbox
            label={t('cluster.k8s.eventReport')}
            checked={form.event_report_enabled}
            onCheckedChange={(checked) => patch({event_report_enabled: checked === true})}
            data-testid='k8s-event-report-toggle'
          />
        <p className='text-xs text-muted-foreground'>{t('cluster.k8s.eventReportHint')}</p>
        {form.event_report_enabled ? (
          <div className='space-y-1'>
            <Input
              label={t('cluster.k8s.stxCallbackUrl')}
              value={form.stx_callback_url}
              onChange={(e) => patch({stx_callback_url: e.target.value})}
              placeholder={t('cluster.k8s.stxCallbackUrlPlaceholder')}
              data-testid='k8s-stx-callback-url'
            />
            {callbackUrlLoopback ? (
              <p className='text-xs font-medium text-destructive' data-testid='k8s-callback-loopback-error'>{t('cluster.k8s.stxCallbackUrlLoopbackError')}</p>
            ) : (
              <p className='text-xs text-muted-foreground'>{t('cluster.k8s.stxCallbackUrlHint')}</p>
            )}
          </div>
        ) : null}
      </div>

      <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3' data-testid='k8s-proxy-section'>
        <p className='text-sm font-medium'>{t('cluster.k8s.proxyTitle')}</p>
        <div className='grid grid-cols-3 gap-3'>
          <div className='space-y-1.5'>
            <Select
              label={t('cluster.k8s.proxyMode')}
              value={form.java_proxy_mode}
              onValueChange={(value) => patch({java_proxy_mode: value})}
              options={[
                  {value: 'managed', label: t('cluster.k8s.proxyModeManaged')},
                  {value: 'reuse_cluster', label: t('cluster.k8s.proxyModeReuse')},
                  {value: 'external', label: t('cluster.k8s.proxyModeExternal')},
                  {value: 'disabled', label: t('cluster.k8s.proxyModeDisabled')},
                ]}
              data-testid='k8s-proxy-mode'
            />
          </div>
          <div className='space-y-1.5'>
            <Input label={t('cluster.k8s.proxyPort')} value={form.java_proxy_port} onChange={(e) => patch({java_proxy_port: e.target.value})} placeholder='18080' data-testid='k8s-proxy-port' />
          </div>
          <div className='space-y-1.5'>
            <Select
              label={t('cluster.k8s.proxyHeap')}
              value={form.java_proxy_heap}
              onValueChange={(value) => patch({java_proxy_heap: value})}
              options={['512m', '1024m', '2048m', '4096m'].map((heap) => ({value: heap, label: heap}))}
              data-testid='k8s-proxy-heap'
            />
          </div>
        </div>
        {form.java_proxy_mode === 'managed' ? (
          <p className='text-xs text-muted-foreground' data-testid='k8s-proxy-derived-memory'>
            {t('cluster.k8s.proxyDerivedMemory', {
              limit: deriveProxyContainerMemory(`-Xmx${form.java_proxy_heap}`).limitMi + 'Mi',
              request: deriveProxyContainerMemory(`-Xmx${form.java_proxy_heap}`).requestMi + 'Mi',
            })}
          </p>
        ) : null}
      </div>

      <RuntimeAdvancedConfigCard
        compact
        defaultCollapsed
        version={form.version}
        capabilities={k8sVersionCapabilities(form.version)}
        runtime={{
          dynamic_slot: form.runtime_dynamic_slot,
          slot_num: parseInt(form.runtime_slot_num, 10) || 0,
          slot_allocation_strategy: (form.runtime_slot_allocation_strategy || 'RANDOM') as RuntimeEngineConfig['slot_allocation_strategy'],
          job_schedule_strategy: (form.runtime_job_schedule_strategy || 'WAIT') as RuntimeEngineConfig['job_schedule_strategy'],
          history_job_expire_minutes: parseInt(form.runtime_history_job_expire_minutes, 10) || 1440,
          scheduled_deletion_enable: form.runtime_scheduled_deletion_enable,
          enable_http: true,
          enable_event_channel: form.event_report_enabled,
          job_log_mode: (form.runtime_job_log_mode || 'mixed') as RuntimeEngineConfig['job_log_mode'],
        }}
        onChange={(updates) => {
          const next: Partial<K8sWizardFormState> = {};
          if (updates.dynamic_slot !== undefined) next.runtime_dynamic_slot = updates.dynamic_slot;
          if (updates.slot_num !== undefined) next.runtime_slot_num = String(updates.slot_num);
          if (updates.slot_allocation_strategy !== undefined) next.runtime_slot_allocation_strategy = updates.slot_allocation_strategy;
          if (updates.job_schedule_strategy !== undefined) next.runtime_job_schedule_strategy = updates.job_schedule_strategy;
          if (updates.history_job_expire_minutes !== undefined) next.runtime_history_job_expire_minutes = String(updates.history_job_expire_minutes);
          if (updates.scheduled_deletion_enable !== undefined) next.runtime_scheduled_deletion_enable = updates.scheduled_deletion_enable;
          if (updates.job_log_mode !== undefined) next.runtime_job_log_mode = updates.job_log_mode;
          patch(next);
        }}
      />

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
export function K8sPrecheckContent({form, onResult}: {form: K8sWizardFormState; onResult?: (result: K8sPrecheckResult | null) => void}) {
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
        cluster_code: form.cluster_code,
        deployment_mode: form.deployment_mode,
        image: form.image,
        image_pull_secrets: form.image_pull_secrets,
        service_account: form.service_account,
        checkpoint_pvc: form.checkpoint_pvc,
        master_memory: form.master_memory,
        worker_memory: form.worker_memory,
        hybrid_memory: form.hybrid_memory,
        jvm_xmx: form.jvm_xmx,
        master_heap_mb: parseInt(form.master_heap_mb, 10) || 0,
        worker_heap_mb: parseInt(form.worker_heap_mb, 10) || 0,
        hybrid_heap_mb: parseInt(form.hybrid_heap_mb, 10) || 0,
        enable_event_channel: form.event_report_enabled,
        stx_callback_url: form.event_report_enabled ? form.stx_callback_url.trim() : '',
      });
      setResult(response.data);
      onResult?.(response.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      onResult?.(null);
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
        <Button size='sm' variant='secondary' onClick={() => void run()} disabled={running} data-testid='k8s-precheck-rerun'>
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
    jvm: {
      xms: form.jvm_xms,
      xmx: form.jvm_xmx,
      master_heap_mb: parseInt(form.master_heap_mb, 10) || 0,
      worker_heap_mb: parseInt(form.worker_heap_mb, 10) || 0,
      hybrid_heap_mb: parseInt(form.hybrid_heap_mb, 10) || 0,
    },
    java_proxy_mode: form.java_proxy_mode,
    java_proxy_port: parseInt(form.java_proxy_port, 10) || 18080,
    java_proxy_jvm_opts: `-Xmx${form.java_proxy_heap}`,
    enable_event_channel: form.event_report_enabled,
    stx_callback_url: form.event_report_enabled ? form.stx_callback_url.trim() : '',
    runtime: {
      dynamic_slot: form.runtime_dynamic_slot,
      ...(parseInt(form.runtime_slot_num, 10) > 0 ? {slot_num: parseInt(form.runtime_slot_num, 10)} : {}),
      ...(form.runtime_slot_allocation_strategy ? {slot_allocation_strategy: form.runtime_slot_allocation_strategy} : {}),
      ...(form.runtime_job_schedule_strategy ? {job_schedule_strategy: form.runtime_job_schedule_strategy} : {}),
      ...(parseInt(form.runtime_history_job_expire_minutes, 10) > 0 ? {history_job_expire_minutes: parseInt(form.runtime_history_job_expire_minutes, 10)} : {}),
      scheduled_deletion_enable: form.runtime_scheduled_deletion_enable,
      ...(form.runtime_job_log_mode ? {job_log_mode: form.runtime_job_log_mode} : {}),
    },
    checkpoint: form.checkpoint.enabled ? form.checkpoint : undefined,
    imap: form.imap.enabled ? form.imap : undefined,
  };
}
