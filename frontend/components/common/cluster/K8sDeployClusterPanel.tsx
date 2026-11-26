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
 * Kubernetes 一键部署面板：嵌入「创建集群」向导，在选定 K8s 主机命名空间下部署 SeaTunnel 常驻集群。
 * Kubernetes one-click deploy panel embedded in ClusterDeployWizard for deploying a session cluster.
 */

import {useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  ChevronDown,
  ChevronRight,
  Cloud,
  SlidersHorizontal,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {HelpHint} from '@/components/common/HelpHint';
import {isValidClusterCode, recommendClusterCode} from '@/lib/cluster-code';
import services from '@/lib/services';
import {HostInfo, HostType} from '@/lib/services/host/types';
import {cn} from '@/lib/utils';

export interface K8sDeployFormState {
  host_id: number;
  namespace: string;
  name: string;
  cluster_code: string;
  description: string;
  image: string;
  version: string;
  master_replicas: number;
  worker_replicas: number;
  master_http_port: number;
  hazelcast_port: number;
  master_cpu: string;
  master_memory: string;
  worker_cpu: string;
  worker_memory: string;
  checkpoint_pvc: string;
  data_storage_class: string;
  data_size: string;
}

export const INITIAL_K8S_DEPLOY_FORM: K8sDeployFormState = {
  host_id: 0,
  namespace: 'default',
  name: '',
  cluster_code: '',
  description: '',
  image: 'apache/seatunnel:3.1.0',
  version: '3.1.0',
  master_replicas: 1,
  worker_replicas: 2,
  master_http_port: 8080,
  hazelcast_port: 5801,
  master_cpu: '',
  master_memory: '',
  worker_cpu: '',
  worker_memory: '',
  checkpoint_pvc: '',
  data_storage_class: '',
  data_size: '',
};

/**
 * 从镜像标签推断 SeaTunnel 版本（例如 apache/seatunnel:3.1.0 -> 3.1.0）。
 * Infer SeaTunnel version from an image tag (e.g. apache/seatunnel:3.1.0 -> 3.1.0).
 */
export function inferVersionFromImage(image: string): string {
  const trimmed = image.trim();
  if (!trimmed || trimmed.includes('@')) {
    return '';
  }
  const lastColon = trimmed.lastIndexOf(':');
  const lastSlash = trimmed.lastIndexOf('/');
  if (lastColon <= lastSlash) {
    return '';
  }
  const tag = trimmed.slice(lastColon + 1).trim();
  if (!tag || tag.toLowerCase() === 'latest') {
    return '';
  }
  return tag;
}

interface K8sDeployClusterPanelProps {
  active: boolean;
  form: K8sDeployFormState;
  onChange: (updater: (prev: K8sDeployFormState) => K8sDeployFormState) => void;
  onNavigateToHosts: () => void;
}

export function K8sDeployClusterPanel({
  active,
  form,
  onChange,
  onNavigateToHosts,
}: K8sDeployClusterPanelProps) {
  const t = useTranslations();
  const [loadingHosts, setLoadingHosts] = useState(false);
  const [k8sHosts, setK8sHosts] = useState<HostInfo[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [codeTouched, setCodeTouched] = useState(Boolean(form.cluster_code));
  const [versionTouched, setVersionTouched] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const nsSeqRef = useRef(0);

  useEffect(() => {
    if (!active) {
      setCodeTouched(false);
      setVersionTouched(false);
      return;
    }
    let cancelled = false;
    setLoadingHosts(true);
    void services.host
      .getAllHostsByTypeSafe(HostType.KUBERNETES)
      .then((res) => {
        if (cancelled) {
          return;
        }
        const items = res.hosts.filter((h) => h.host_type === HostType.KUBERNETES);
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

  useEffect(() => {
    if (!active || !form.host_id) {
      setNamespaces([]);
      return;
    }
    const seq = ++nsSeqRef.current;
    void services.host
      .listK8sNamespaces(form.host_id)
      .then((list) => {
        if (seq === nsSeqRef.current) {
          setNamespaces(list);
        }
      })
      .catch(() => {
        if (seq === nsSeqRef.current) {
          setNamespaces([]);
        }
      });
  }, [active, form.host_id]);

  const clusterCodeInvalid =
    form.cluster_code.length > 0 && !isValidClusterCode(form.cluster_code);

  if (loadingHosts) {
    return (
      <div className='mx-auto max-w-3xl py-4'>
        <Skeleton lines={6} label={t('common.loading')} />
      </div>
    );
  }

  if (k8sHosts.length === 0) {
    return (
      <div className='flex h-full items-center justify-center'>
        <EmptyState
          icon={<Cloud className='size-6' aria-hidden />}
          title={t('cluster.k8s.noK8sHosts')}
          description={t('cluster.k8s.noK8sHostsDesc')}
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
      </div>
    );
  }

  return (
    <div className='flex h-full flex-col overflow-hidden' data-testid='k8s-deploy-panel'>
      <div className='min-h-0 flex-1 overflow-y-auto pr-4'>
        <div className='mx-auto max-w-3xl space-y-5 py-2'>
          {/* 目标 K8s 主机与命名空间 / Target Kubernetes host & namespace */}
          <div className='grid grid-cols-1 items-start gap-4 md:grid-cols-2'>
            <Select
              id='k8s-deploy-host'
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
                onChange((prev) => ({
                  ...prev,
                  host_id: id,
                  namespace: picked?.k8s_namespace?.trim() || prev.namespace || 'default',
                }));
              }}
            />

            <div className='space-y-1.5'>
              <Input
                id='k8s-deploy-namespace'
                label={t('host.namespace')}
                required
                value={form.namespace}
                onChange={(e) =>
                  onChange((prev) => ({...prev, namespace: e.target.value}))
                }
                placeholder='default'
                description={
                  namespaces.length > 0
                    ? t('cluster.k8s.namespaceDeployHint')
                    : undefined
                }
                className='font-mono text-xs'
              />
              {namespaces.length > 0 ? (
                <div className='flex flex-wrap gap-1.5'>
                  {namespaces.slice(0, 8).map((ns) => (
                    <button
                      key={ns}
                      type='button'
                      onClick={() => onChange((prev) => ({...prev, namespace: ns}))}
                      className={cn(
                        'rounded border px-2 py-0.5 font-mono text-[11px] transition-colors',
                        form.namespace === ns
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border/80 text-muted-foreground hover:border-foreground/30 hover:text-foreground',
                      )}
                    >
                      {ns}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
          </div>

          {/* 基本信息：集群名称、标识、描述 / Basics: cluster name, code, description */}
          <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
            <div className='grid grid-cols-1 items-start gap-4 md:grid-cols-2'>
              <Input
                id='k8s-deploy-name'
                label={t('cluster.name')}
                required
                value={form.name}
                onChange={(e) => {
                  const nextName = e.target.value;
                  onChange((prev) => ({
                    ...prev,
                    name: nextName,
                    cluster_code: codeTouched
                      ? prev.cluster_code
                      : recommendClusterCode(nextName),
                  }));
                }}
                placeholder={t('cluster.namePlaceholder')}
                autoFocus
              />

              <div className='relative'>
                <Input
                  id='k8s-deploy-code'
                  label={`${t('cluster.clusterCode')} · ${t('common.optional')}`}
                  value={form.cluster_code}
                  onChange={(e) => {
                    setCodeTouched(true);
                    onChange((prev) => ({
                      ...prev,
                      cluster_code: e.target.value.toLowerCase().trim(),
                    }));
                  }}
                  placeholder={t('cluster.clusterCodePlaceholder')}
                  className='font-mono text-xs'
                  error={
                    clusterCodeInvalid
                      ? t('cluster.clusterCodeInvalid')
                      : undefined
                  }
                  description={
                    clusterCodeInvalid
                      ? undefined
                      : t('cluster.clusterCodeShortHint')
                  }
                />
                <div className='absolute top-0 right-0'>
                  <HelpHint
                    label={t('cluster.clusterCode')}
                    content={t('cluster.clusterCodeHint')}
                  />
                </div>
              </div>
            </div>

            <Input
              id='k8s-deploy-desc'
              label={t('cluster.descriptionLabel')}
              value={form.description}
              onChange={(e) =>
                onChange((prev) => ({...prev, description: e.target.value}))
              }
              placeholder={t('cluster.descriptionPlaceholder')}
            />
          </div>

          {/* 镜像、副本与端口 / Image, replicas & ports */}
          <div className='space-y-4 border-t-[0.5px] border-border/80 pt-4'>
            <div className='grid grid-cols-1 items-start gap-4 md:grid-cols-2'>
              <Input
                id='k8s-deploy-image'
                label={t('cluster.k8s.image')}
                required
                value={form.image}
                onChange={(e) => {
                  const nextImage = e.target.value;
                  const inferred = inferVersionFromImage(nextImage);
                  onChange((prev) => ({
                    ...prev,
                    image: nextImage,
                    version: !versionTouched && inferred ? inferred : prev.version,
                  }));
                }}
                placeholder='apache/seatunnel:3.1.0'
                className='font-mono text-xs'
              />
              <Input
                id='k8s-deploy-version'
                label={t('cluster.version')}
                value={form.version}
                onChange={(e) => {
                  setVersionTouched(true);
                  onChange((prev) => ({...prev, version: e.target.value}));
                }}
                placeholder='3.1.0'
                className='font-mono text-xs tabular-nums'
              />
            </div>

            <div className='grid grid-cols-2 gap-4 sm:grid-cols-4'>
              <Input
                id='k8s-deploy-master-replicas'
                type='number'
                label={t('cluster.k8s.masterReplicas')}
                value={String(form.master_replicas)}
                min={1}
                onChange={(e) =>
                  onChange((prev) => ({
                    ...prev,
                    master_replicas: Math.max(1, Number(e.target.value) || 1),
                  }))
                }
                className='font-mono tabular-nums'
              />
              <Input
                id='k8s-deploy-worker-replicas'
                type='number'
                label={t('cluster.k8s.workerReplicas')}
                value={String(form.worker_replicas)}
                min={1}
                onChange={(e) =>
                  onChange((prev) => ({
                    ...prev,
                    worker_replicas: Math.max(1, Number(e.target.value) || 1),
                  }))
                }
                className='font-mono tabular-nums'
              />
              <Input
                id='k8s-deploy-http-port'
                type='number'
                label={t('cluster.k8s.masterHttpPort')}
                value={String(form.master_http_port)}
                min={1}
                max={65535}
                onChange={(e) =>
                  onChange((prev) => ({
                    ...prev,
                    master_http_port: Number(e.target.value) || 8080,
                  }))
                }
                className='font-mono tabular-nums'
              />
              <Input
                id='k8s-deploy-hz-port'
                type='number'
                label={t('cluster.k8s.hazelcastPort')}
                value={String(form.hazelcast_port)}
                min={1}
                max={65535}
                onChange={(e) =>
                  onChange((prev) => ({
                    ...prev,
                    hazelcast_port: Number(e.target.value) || 5801,
                  }))
                }
                className='font-mono tabular-nums'
              />
            </div>
          </div>

          {/* 高级资源配额与持久化存储 / Advanced resources & persistent storage */}
          <div className='border-t-[0.5px] border-border/80 pt-3'>
            <button
              type='button'
              className='flex w-full items-center justify-between py-1.5 text-left text-xs font-medium text-foreground hover:text-primary'
              onClick={() => setShowAdvanced((v) => !v)}
              aria-expanded={showAdvanced}
            >
              <div className='flex items-center gap-2'>
                <SlidersHorizontal className='size-3.5 text-muted-foreground' />
                <span>{t('cluster.k8s.advancedResources')}</span>
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
                <div className='space-y-4 pt-3'>
                  <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
                    <Input
                      id='k8s-deploy-master-cpu'
                      label={t('cluster.k8s.masterCpu')}
                      value={form.master_cpu}
                      onChange={(e) =>
                        onChange((prev) => ({...prev, master_cpu: e.target.value}))
                      }
                      placeholder='1'
                      className='font-mono text-xs'
                    />
                    <Input
                      id='k8s-deploy-master-mem'
                      label={t('cluster.k8s.masterMemory')}
                      value={form.master_memory}
                      onChange={(e) =>
                        onChange((prev) => ({
                          ...prev,
                          master_memory: e.target.value,
                        }))
                      }
                      placeholder='2Gi'
                      className='font-mono text-xs'
                    />
                    <Input
                      id='k8s-deploy-worker-cpu'
                      label={t('cluster.k8s.workerCpu')}
                      value={form.worker_cpu}
                      onChange={(e) =>
                        onChange((prev) => ({...prev, worker_cpu: e.target.value}))
                      }
                      placeholder='2'
                      className='font-mono text-xs'
                    />
                    <Input
                      id='k8s-deploy-worker-mem'
                      label={t('cluster.k8s.workerMemory')}
                      value={form.worker_memory}
                      onChange={(e) =>
                        onChange((prev) => ({
                          ...prev,
                          worker_memory: e.target.value,
                        }))
                      }
                      placeholder='4Gi'
                      className='font-mono text-xs'
                    />
                  </div>
                  <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                    <Input
                      id='k8s-deploy-checkpoint-pvc'
                      label={t('cluster.k8s.checkpointPvc')}
                      value={form.checkpoint_pvc}
                      onChange={(e) =>
                        onChange((prev) => ({
                          ...prev,
                          checkpoint_pvc: e.target.value,
                        }))
                      }
                      placeholder='seatunnel-checkpoint-pvc'
                      className='font-mono text-xs'
                    />
                    <Input
                      id='k8s-deploy-storage-class'
                      label={t('cluster.k8s.dataStorageClass')}
                      value={form.data_storage_class}
                      onChange={(e) =>
                        onChange((prev) => ({
                          ...prev,
                          data_storage_class: e.target.value,
                        }))
                      }
                      placeholder='standard'
                      className='font-mono text-xs'
                    />
                    <Input
                      id='k8s-deploy-data-size'
                      label={t('cluster.k8s.dataSize')}
                      value={form.data_size}
                      onChange={(e) =>
                        onChange((prev) => ({...prev, data_size: e.target.value}))
                      }
                      placeholder='10Gi'
                      className='font-mono text-xs'
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
