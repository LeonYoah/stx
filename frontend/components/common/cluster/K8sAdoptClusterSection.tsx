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
 * Kubernetes 已有集群纳管区块：嵌入「注册集群」对话框，支持按 K8s 主机与命名空间扫描并自动回填。
 * Kubernetes adopt-existing section embedded in Register Cluster dialog with namespace discovery & backfill.
 */

import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  ChevronDown,
  ChevronRight,
  Cloud,
  Search,
  SlidersHorizontal,
} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {Textarea} from '@/components/ui/textarea';
import services from '@/lib/services';
import {HostInfo, HostType} from '@/lib/services/host/types';
import {cn} from '@/lib/utils';

export interface K8sAdoptFormState {
  host_id: number;
  namespace: string;
  name: string;
  description: string;
  master_service_name: string;
  cluster_service_name: string;
  master_workload_name: string;
  worker_workload_name: string;
  master_workload_kind: string;
  worker_workload_kind: string;
  configmap_name: string;
  label_selector: string;
  image: string;
  master_http_port: number;
  hazelcast_port: number;
}

export const INITIAL_K8S_ADOPT_FORM: K8sAdoptFormState = {
  host_id: 0,
  namespace: 'default',
  name: '',
  description: '',
  master_service_name: '',
  cluster_service_name: '',
  master_workload_name: '',
  worker_workload_name: '',
  master_workload_kind: '',
  worker_workload_kind: '',
  configmap_name: '',
  label_selector: '',
  image: '',
  master_http_port: 8080,
  hazelcast_port: 5801,
};

export interface K8sAdoptFormErrors {
  host?: string;
  namespace?: string;
  name?: string;
}

export interface DiscoveredK8sCluster {
  base_name?: string;
  namespace?: string;
  master?: {
    kind?: string;
    name?: string;
    replicas?: number;
    ready_replicas?: number;
    image?: string;
  };
  worker?: {
    kind?: string;
    name?: string;
    replicas?: number;
    ready_replicas?: number;
    image?: string;
  };
  services?: string[];
  configmap_name?: string;
  label_selector?: string;
  master_http_port?: number;
  hazelcast_port?: number;
}

interface K8sAdoptClusterSectionProps {
  active: boolean;
  form: K8sAdoptFormState;
  errors: K8sAdoptFormErrors;
  onChange: (updater: (prev: K8sAdoptFormState) => K8sAdoptFormState) => void;
  onClearError: (field: keyof K8sAdoptFormErrors) => void;
  onNavigateToHosts: () => void;
}

function candidateKey(item: DiscoveredK8sCluster, idx: number): string {
  return (
    item.base_name ||
    item.master?.name ||
    item.worker?.name ||
    `candidate-${idx}`
  );
}

export function K8sAdoptClusterSection({
  active,
  form,
  errors,
  onChange,
  onClearError,
  onNavigateToHosts,
}: K8sAdoptClusterSectionProps) {
  const t = useTranslations();
  const [loadingHosts, setLoadingHosts] = useState(false);
  const [k8sHosts, setK8sHosts] = useState<HostInfo[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [discovering, setDiscovering] = useState(false);
  const [hasScanned, setHasScanned] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [discovered, setDiscovered] = useState<DiscoveredK8sCluster[]>([]);
  const [selectedCandidate, setSelectedCandidate] = useState<string | null>(
    null,
  );
  const [showAdvanced, setShowAdvanced] = useState(false);

  const nsSeqRef = useRef(0);
  const scanSeqRef = useRef(0);
  const autoFilledNameRef = useRef<string>('');

  // 加载 K8s 主机列表，首个可用主机自动预选以减少点击。
  // Load K8s hosts and preselect the first available host to cut extra clicks.
  useEffect(() => {
    if (!active) {
      return;
    }
    let cancelled = false;
    setLoadingHosts(true);
    void services.host
      .getHostsSafe({
        current: 1,
        size: 200,
        host_type: HostType.KUBERNETES,
      })
      .then((res) => {
        if (cancelled) {
          return;
        }
        const items = (res.success && res.data?.hosts ? res.data.hosts : []).filter(
          (h) => h.host_type === HostType.KUBERNETES,
        );
        setK8sHosts(items);
        if (items.length > 0) {
          onChange((prev) => {
            if (prev.host_id > 0 && items.some((h) => h.id === prev.host_id)) {
              return prev;
            }
            const first = items[0];
            return {
              ...prev,
              host_id: first.id,
              namespace: first.k8s_namespace?.trim() || prev.namespace || 'default',
            };
          });
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingHosts(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [active, onChange]);

  // 切换主机时拉取可见命名空间；无集群级 list 权限时静默回退为手工输入框。
  // Fetch visible namespaces on host switch; fall back silently to text input on namespace-scoped RBAC.
  useEffect(() => {
    if (!active || !form.host_id) {
      setNamespaces([]);
      return;
    }
    const seq = ++nsSeqRef.current;
    void services.host
      .listK8sNamespaces(form.host_id)
      .then((list) => {
        if (seq !== nsSeqRef.current) {
          return;
        }
        setNamespaces(list);
      })
      .catch(() => {
        if (seq === nsSeqRef.current) {
          setNamespaces([]);
        }
      });
  }, [active, form.host_id]);

  const runDiscovery = useCallback(async (hostId: number, namespace: string) => {
    const ns = namespace.trim();
    if (!hostId || !ns) {
      return;
    }
    const seq = ++scanSeqRef.current;
    setDiscovering(true);
    setScanError(null);
    try {
      const list = (await services.host.discoverK8sClusters(
        hostId,
        ns,
      )) as DiscoveredK8sCluster[];
      if (seq !== scanSeqRef.current) {
        return;
      }
      setDiscovered(Array.isArray(list) ? list : []);
      setHasScanned(true);
    } catch (err) {
      if (seq !== scanSeqRef.current) {
        return;
      }
      setDiscovered([]);
      setHasScanned(true);
      setScanError(err instanceof Error ? err.message : t('discovery.discoverError'));
    } finally {
      if (seq === scanSeqRef.current) {
        setDiscovering(false);
      }
    }
  }, [t]);

  // 主机或命名空间变化时自动扫描当前命名空间下的 SeaTunnel 工作负载。
  // Auto-scan the chosen namespace whenever host or namespace changes.
  useEffect(() => {
    if (!active || !form.host_id || !form.namespace.trim()) {
      return;
    }
    setSelectedCandidate(null);
    const timer = window.setTimeout(() => {
      void runDiscovery(form.host_id, form.namespace);
    }, 200);
    return () => window.clearTimeout(timer);
  }, [active, form.host_id, form.namespace, runDiscovery]);

  const pickDiscovered = (d: DiscoveredK8sCluster, key: string) => {
    // master service 命名通常含 master；cluster service 为 headless 基础名，取剩余候选。
    // Master service usually contains "master"; cluster service is the headless base name from remaining candidates.
    const svcs = d.services || [];
    const masterSvc = svcs.find((s) => s.toLowerCase().includes('master')) || '';
    const clusterSvc =
      svcs.find((s) => s !== masterSvc && !s.toLowerCase().includes('proxy')) ||
      '';
    const suggestedName = d.base_name || d.master?.name || '';
    setSelectedCandidate(key);
    onClearError('name');
    onChange((prev) => {
      const shouldReplaceName =
        !prev.name.trim() || prev.name === autoFilledNameRef.current;
      if (shouldReplaceName && suggestedName) {
        autoFilledNameRef.current = suggestedName;
      }
      return {
        ...prev,
        name: shouldReplaceName ? suggestedName : prev.name,
        master_service_name: masterSvc,
        cluster_service_name: clusterSvc,
        master_workload_name: d.master?.name || '',
        worker_workload_name: d.worker?.name || '',
        // 透传 StatefulSet / Deployment 类型，避免纳管 Deployment 集群时被默认回退为 StatefulSet。
        // Preserve StatefulSet/Deployment kinds so adopted Deployment clusters never fall back to StatefulSet.
        master_workload_kind: d.master?.kind || '',
        worker_workload_kind: d.worker?.kind || '',
        configmap_name: d.configmap_name || '',
        label_selector: d.label_selector || '',
        master_http_port: d.master_http_port || prev.master_http_port || 8080,
        hazelcast_port: d.hazelcast_port || prev.hazelcast_port || 5801,
        // 同步回填发现到的镜像：纳管 Deployment 集群时伴生 Proxy 依赖此字段兜底。
        // Backfill the discovered image as fallback for companion proxy on Deployment clusters.
        image: d.master?.image || d.worker?.image || '',
      };
    });
  };

  if (loadingHosts) {
    return <Skeleton lines={5} label={t('common.loading')} />;
  }

  if (k8sHosts.length === 0) {
    return (
      <EmptyState
        icon={<Cloud className='size-5' aria-hidden />}
        title={t('cluster.k8s.noK8sHosts')}
        description={t('cluster.k8s.noK8sHostsDesc')}
        className='py-8'
        action={
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={onNavigateToHosts}
          >
            {t('cluster.goToHostsManagement')}
          </Button>
        }
      />
    );
  }

  return (
    <div className='space-y-5' data-testid='k8s-adopt-section'>
      {/* 第一段：选择 K8s 主机与命名空间并发现已有集群 / Step 1: pick K8s host + namespace & discover */}
      <section className='space-y-3'>
        <div className='grid grid-cols-1 items-start gap-3 sm:grid-cols-[1fr_1fr_auto]'>
          <div className='space-y-1'>
            <Select
              id='k8s-adopt-host'
              label={`${t('cluster.k8s.host')} *`}
              value={form.host_id ? String(form.host_id) : ''}
              placeholder={t('common.pleaseSelect')}
              options={k8sHosts.map((h) => ({
                value: String(h.id),
                label: h.k8s_api_url ? `${h.name} (${h.k8s_api_url})` : h.name,
              }))}
              onValueChange={(v) => {
                const id = Number(v) || 0;
                const picked = k8sHosts.find((h) => h.id === id);
                onClearError('host');
                onChange((prev) => ({
                  ...prev,
                  host_id: id,
                  namespace: picked?.k8s_namespace?.trim() || prev.namespace || 'default',
                }));
              }}
            />
            {errors.host ? (
              <p className='text-xs text-destructive' role='alert'>
                {errors.host}
              </p>
            ) : null}
          </div>

          {namespaces.length > 0 ? (
            <div className='space-y-1'>
              <Select
                id='k8s-adopt-namespace'
                label={`${t('host.namespace')} *`}
                value={form.namespace}
                options={
                  namespaces.includes(form.namespace) || !form.namespace
                    ? namespaces.map((n) => ({value: n, label: n}))
                    : [
                        {value: form.namespace, label: form.namespace},
                        ...namespaces.map((n) => ({value: n, label: n})),
                      ]
                }
                onValueChange={(v) => {
                  onClearError('namespace');
                  onChange((prev) => ({...prev, namespace: v}));
                }}
              />
              {errors.namespace ? (
                <p className='text-xs text-destructive' role='alert'>
                  {errors.namespace}
                </p>
              ) : null}
            </div>
          ) : (
            <Input
              id='k8s-adopt-namespace-input'
              label={t('host.namespace')}
              required
              value={form.namespace}
              error={errors.namespace}
              onChange={(e) => {
                onClearError('namespace');
                onChange((prev) => ({...prev, namespace: e.target.value}));
              }}
              placeholder='default'
              className='font-mono text-xs'
            />
          )}

          <div className='sm:pt-5'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              loading={discovering}
              disabled={!form.host_id || !form.namespace.trim() || discovering}
              onClick={() => void runDiscovery(form.host_id, form.namespace)}
            >
              <Search className='size-3.5' aria-hidden />
              {t('cluster.k8s.scanNamespace')}
            </Button>
          </div>
        </div>

        {scanError ? <Alert tone='warning' title={scanError} /> : null}

        {discovering ? (
          <Skeleton lines={2} label={t('common.loading')} />
        ) : discovered.length > 0 ? (
          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-2'>
              <span className='text-xs font-medium text-foreground'>
                {t('cluster.k8s.discovered')}
              </span>
              <span className='text-xs text-muted-foreground'>
                {t('cluster.k8s.discoveredHint')}
              </span>
            </div>
            <div className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
              {discovered.map((item, idx) => {
                const key = candidateKey(item, idx);
                const selected = selectedCandidate === key;
                return (
                  <button
                    key={key}
                    type='button'
                    data-testid={`k8s-discovered-card-${idx}`}
                    onClick={() => pickDiscovered(item, key)}
                    className={cn(
                      'flex flex-col gap-1.5 rounded-md border px-3 py-2.5 text-left transition-colors',
                      selected
                        ? 'border-primary bg-primary/8'
                        : 'border-border/80 hover:border-foreground/30 hover:bg-muted/40',
                    )}
                  >
                    <div className='flex items-center justify-between gap-2'>
                      <span className='truncate text-xs font-semibold text-foreground'>
                        {item.base_name || item.master?.name || `cluster-${idx + 1}`}
                      </span>
                      {item.master?.kind ? (
                        <NeutralChip className='font-mono text-[10px]'>
                          {item.master.kind}
                        </NeutralChip>
                      ) : null}
                    </div>
                    <div className='flex flex-wrap items-center gap-1.5 font-mono text-[11px] text-muted-foreground tabular-nums'>
                      {item.master ? (
                        <SoftChip tone='blue'>
                          M {item.master.ready_replicas ?? 0}/{item.master.replicas ?? 0}
                        </SoftChip>
                      ) : null}
                      {item.worker ? (
                        <SoftChip tone='emerald'>
                          W {item.worker.ready_replicas ?? 0}/{item.worker.replicas ?? 0}
                        </SoftChip>
                      ) : null}
                      {item.master_http_port ? (
                        <span>HTTP {item.master_http_port}</span>
                      ) : null}
                    </div>
                    {item.master?.image || item.worker?.image ? (
                      <span className='truncate font-mono text-[11px] text-muted-foreground'>
                        {item.master?.image || item.worker?.image}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </div>
          </div>
        ) : hasScanned && !scanError ? (
          <Alert tone='info' title={t('cluster.k8s.noDiscoveredClusters')} />
        ) : null}
      </section>

      {/* 第二段：集群基本信息与服务端口 / Step 2: cluster basics & service endpoints */}
      <section className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
        <Input
          id='k8s-adopt-name'
          label={t('cluster.name')}
          required
          value={form.name}
          error={errors.name}
          onChange={(e) => {
            onClearError('name');
            onChange((prev) => ({...prev, name: e.target.value}));
          }}
          placeholder={t('cluster.namePlaceholder')}
        />

        <div className='space-y-1.5'>
          <label
            htmlFor='k8s-adopt-desc'
            className='block text-xs font-medium text-foreground'
          >
            {t('cluster.descriptionLabel')}
          </label>
          <Textarea
            id='k8s-adopt-desc'
            value={form.description}
            onChange={(e) =>
              onChange((prev) => ({...prev, description: e.target.value}))
            }
            placeholder={t('cluster.descriptionPlaceholder')}
            rows={2}
          />
        </div>

        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
          <Input
            id='k8s-adopt-master-svc'
            label={t('cluster.k8s.masterService')}
            value={form.master_service_name}
            onChange={(e) =>
              onChange((prev) => ({...prev, master_service_name: e.target.value}))
            }
            placeholder='seatunnel-master'
            className='font-mono text-xs'
          />
          <Input
            id='k8s-adopt-cluster-svc'
            label={t('cluster.k8s.clusterService')}
            value={form.cluster_service_name}
            onChange={(e) =>
              onChange((prev) => ({
                ...prev,
                cluster_service_name: e.target.value,
              }))
            }
            placeholder='seatunnel'
            className='font-mono text-xs'
          />
          <Input
            id='k8s-adopt-http-port'
            type='number'
            label={t('cluster.k8s.masterHttpPort')}
            value={String(form.master_http_port)}
            onChange={(e) =>
              onChange((prev) => ({
                ...prev,
                master_http_port: Number(e.target.value) || 8080,
              }))
            }
            min={1}
            max={65535}
            className='font-mono tabular-nums'
          />
          <Input
            id='k8s-adopt-hz-port'
            type='number'
            label={t('cluster.k8s.hazelcastPort')}
            value={String(form.hazelcast_port)}
            onChange={(e) =>
              onChange((prev) => ({
                ...prev,
                hazelcast_port: Number(e.target.value) || 5801,
              }))
            }
            min={1}
            max={65535}
            className='font-mono tabular-nums'
          />
        </div>

        {/* 高级工作负载与镜像配置 / Advanced workload & image overrides */}
        <div className='border-t-[0.5px] border-border/80 pt-3'>
          <button
            type='button'
            className='flex w-full items-center justify-between py-1.5 text-left text-xs font-medium text-foreground hover:text-primary'
            onClick={() => setShowAdvanced((v) => !v)}
            aria-expanded={showAdvanced}
          >
            <div className='flex items-center gap-2'>
              <SlidersHorizontal className='size-3.5 text-muted-foreground' />
              <span>{t('cluster.k8s.advancedWorkload')}</span>
            </div>
            {showAdvanced ? (
              <ChevronDown className='size-3.5 text-muted-foreground' />
            ) : (
              <ChevronRight className='size-3.5 text-muted-foreground' />
            )}
          </button>

          <div
            className={cn(
              'grid transition-all duration-200 ease-[cubic-bezier(0.22,1,0.36,1)]',
              showAdvanced
                ? 'grid-rows-[1fr] opacity-100'
                : 'grid-rows-[0fr] opacity-0',
            )}
          >
            <div className='overflow-hidden'>
              <div className='space-y-3 pt-3'>
                <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                  <Input
                    id='k8s-adopt-master-wl'
                    label={t('cluster.k8s.masterWorkloadName')}
                    value={form.master_workload_name}
                    onChange={(e) =>
                      onChange((prev) => ({
                        ...prev,
                        master_workload_name: e.target.value,
                      }))
                    }
                    className='font-mono text-xs'
                  />
                  <Select
                    id='k8s-adopt-master-kind'
                    label={t('cluster.k8s.masterWorkloadKind')}
                    value={form.master_workload_kind || 'StatefulSet'}
                    options={[
                      {value: 'StatefulSet', label: 'StatefulSet'},
                      {value: 'Deployment', label: 'Deployment'},
                    ]}
                    onValueChange={(v) =>
                      onChange((prev) => ({...prev, master_workload_kind: v}))
                    }
                  />
                  <Input
                    id='k8s-adopt-worker-wl'
                    label={t('cluster.k8s.workerWorkloadName')}
                    value={form.worker_workload_name}
                    onChange={(e) =>
                      onChange((prev) => ({
                        ...prev,
                        worker_workload_name: e.target.value,
                      }))
                    }
                    className='font-mono text-xs'
                  />
                  <Select
                    id='k8s-adopt-worker-kind'
                    label={t('cluster.k8s.workerWorkloadKind')}
                    value={form.worker_workload_kind || 'StatefulSet'}
                    options={[
                      {value: 'StatefulSet', label: 'StatefulSet'},
                      {value: 'Deployment', label: 'Deployment'},
                    ]}
                    onValueChange={(v) =>
                      onChange((prev) => ({...prev, worker_workload_kind: v}))
                    }
                  />
                  <Input
                    id='k8s-adopt-configmap'
                    label={t('cluster.k8s.configmapName')}
                    value={form.configmap_name}
                    onChange={(e) =>
                      onChange((prev) => ({
                        ...prev,
                        configmap_name: e.target.value,
                      }))
                    }
                    className='font-mono text-xs'
                  />
                  <Input
                    id='k8s-adopt-label-selector'
                    label={t('cluster.k8s.labelSelector')}
                    value={form.label_selector}
                    onChange={(e) =>
                      onChange((prev) => ({
                        ...prev,
                        label_selector: e.target.value,
                      }))
                    }
                    placeholder='app=seatunnel'
                    className='font-mono text-xs'
                  />
                </div>
                <Input
                  id='k8s-adopt-image'
                  label={t('cluster.k8s.image')}
                  description={t('cluster.k8s.optionalImageHint')}
                  value={form.image}
                  onChange={(e) =>
                    onChange((prev) => ({...prev, image: e.target.value}))
                  }
                  placeholder='apache/seatunnel:3.1.0'
                  className='font-mono text-xs'
                />
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
