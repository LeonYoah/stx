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
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {HostInfo, HostType} from '@/lib/services/host/types';

interface K8sClusterCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

interface DiscoveredCluster {
  name?: string;
  master_service_name?: string;
  cluster_service_name?: string;
  master_workload_name?: string;
  master_workload_kind?: string;
  worker_workload_name?: string;
  worker_workload_kind?: string;
  configmap_name?: string;
  label_selector?: string;
}

/**
 * K8s Cluster Create Dialog: adopt existing or one-click deploy
 * K8s 集群创建对话框：纳管已有集群或一键部署
 */
export function K8sClusterCreateDialog({open, onOpenChange, onSuccess}: K8sClusterCreateDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<'adopted' | 'stx_deployed'>('adopted');
  const [k8sHosts, setK8sHosts] = useState<HostInfo[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [discovered, setDiscovered] = useState<DiscoveredCluster[]>([]);
  const [form, setForm] = useState({
    name: '',
    host_id: 0,
    namespace: 'default',
    image: 'apache/seatunnel:3.1.0',
    master_replicas: 1,
    worker_replicas: 1,
    version: '3.1.0',
    master_service_name: '',
    cluster_service_name: '',
    master_workload_name: '',
    worker_workload_name: '',
    configmap_name: '',
    master_http_port: 8080,
    hazelcast_port: 5801,
  });

  const loadHosts = useCallback(async () => {
    try {
      const resp = await services.host.getHosts({current: 1, size: 200});
      const items = (resp.hosts || []).filter((h: HostInfo) => h.host_type === HostType.KUBERNETES);
      setK8sHosts(items);
    } catch {
      // 列表失败仅表现为空下拉；详情错误已由全局拦截提示。
      // List failure only shows as an empty dropdown; errors are surfaced by the global interceptor.
    }
  }, []);

  useEffect(() => {
    if (open) {
      loadHosts();
    }
  }, [open, loadHosts]);

  const loadNamespaces = useCallback(async (hostId: number) => {
    if (!hostId) return;
    try {
      const ns = await services.host.listK8sNamespaces(hostId);
      setNamespaces(ns);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }, []);

  const loadDiscovered = useCallback(async (hostId: number, namespace: string) => {
    if (!hostId || !namespace) return;
    try {
      const list = await services.host.discoverK8sClusters(hostId, namespace);
      setDiscovered(list as DiscoveredCluster[]);
    } catch {
      setDiscovered([]);
    }
  }, []);

  useEffect(() => {
    if (form.host_id) {
      loadNamespaces(form.host_id);
    }
  }, [form.host_id, loadNamespaces]);

  useEffect(() => {
    if (mode === 'adopted' && form.host_id && form.namespace) {
      loadDiscovered(form.host_id, form.namespace);
    }
  }, [mode, form.host_id, form.namespace, loadDiscovered]);

  const pickDiscovered = (d: DiscoveredCluster) => {
    setForm((f) => ({
      ...f,
      name: f.name || d.name || '',
      master_service_name: d.master_service_name || '',
      cluster_service_name: d.cluster_service_name || '',
      master_workload_name: d.master_workload_name || '',
      worker_workload_name: d.worker_workload_name || '',
      configmap_name: d.configmap_name || '',
    }));
  };

  const submit = async () => {
    if (!form.name || !form.host_id || !form.namespace) {
      toast.error(t('cluster.k8s.requiredFields'));
      return;
    }
    setLoading(true);
    try {
      const payload: Record<string, unknown> = {
        name: form.name,
        host_id: form.host_id,
        namespace: form.namespace,
        management_source: mode,
        master_http_port: form.master_http_port,
        hazelcast_port: form.hazelcast_port,
      };
      if (mode === 'stx_deployed') {
        payload.image = form.image;
        payload.master_replicas = form.master_replicas;
        payload.worker_replicas = form.worker_replicas;
        payload.version = form.version;
      } else {
        payload.master_service_name = form.master_service_name;
        payload.cluster_service_name = form.cluster_service_name;
        payload.master_workload_name = form.master_workload_name;
        payload.worker_workload_name = form.worker_workload_name;
        payload.configmap_name = form.configmap_name;
      }
      await services.cluster.createKubernetesCluster(payload);
      toast.success(t('common.success'));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const sel = 'w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t('cluster.k8s.createTitle')} className='arc-compact max-w-lg'>
        <h3 className='text-base font-semibold'>{t('cluster.k8s.createTitle')}</h3>
        <div className='mt-3 space-y-3'>
          <SegmentedControl
            value={mode}
            onValueChange={(v) => setMode(v as 'adopted' | 'stx_deployed')}
            label={t('cluster.k8s.createTitle')}
            options={[
              {value: 'adopted', label: t('cluster.k8s.modeAdopted')},
              {value: 'stx_deployed', label: t('cluster.k8s.modeDeploy')},
            ]}
          />
          <div className='space-y-1'>
            <label className='text-xs text-muted-foreground'>{t('cluster.k8s.host')} *</label>
            <select
              className={sel}
              value={form.host_id || ''}
              onChange={(e) => setForm({...form, host_id: Number(e.target.value) || 0})}
            >
              <option value=''>{t('common.pleaseSelect')}</option>
              {k8sHosts.map((h) => (
                <option key={h.id} value={h.id}>{h.name}</option>
              ))}
            </select>
          </div>
          <div className='space-y-1'>
            <label className='text-xs text-muted-foreground'>Namespace *</label>
            {namespaces.length > 0 ? (
              <select
                className={sel}
                value={form.namespace}
                onChange={(e) => setForm({...form, namespace: e.target.value})}
              >
                {namespaces.map((n) => (
                  <option key={n} value={n}>{n}</option>
                ))}
              </select>
            ) : (
              <Input
                label='Namespace'
                value={form.namespace}
                onChange={(e) => setForm({...form, namespace: e.target.value})}
                placeholder='default'
              />
            )}
          </div>
          {mode === 'adopted' && discovered.length > 0 ? (
            <div className='space-y-1'>
              <label className='text-xs text-muted-foreground'>{t('cluster.k8s.discovered')}</label>
              <div className='flex flex-wrap gap-2'>
                {discovered.map((d, i) => (
                  <Button key={i} size='sm' variant='secondary' onClick={() => pickDiscovered(d)}>
                    {d.name || d.master_service_name || `cluster-${i + 1}`}
                  </Button>
                ))}
              </div>
            </div>
          ) : null}
          <Input
            label={`${t('cluster.name')} *`}
            value={form.name}
            onChange={(e) => setForm({...form, name: e.target.value})}
          />
          {mode === 'stx_deployed' ? (
            <>
              <Input
                label={t('cluster.k8s.image')}
                value={form.image}
                onChange={(e) => setForm({...form, image: e.target.value})}
              />
              <div className='grid grid-cols-2 gap-3'>
                <Input
                  label={t('cluster.k8s.masterReplicas')}
                  type='number'
                  value={String(form.master_replicas)}
                  onChange={(e) => setForm({...form, master_replicas: Number(e.target.value) || 1})}
                />
                <Input
                  label={t('cluster.k8s.workerReplicas')}
                  type='number'
                  value={String(form.worker_replicas)}
                  onChange={(e) => setForm({...form, worker_replicas: Number(e.target.value) || 1})}
                />
              </div>
            </>
          ) : (
            <>
              <Input
                label={t('cluster.k8s.masterService')}
                value={form.master_service_name}
                onChange={(e) => setForm({...form, master_service_name: e.target.value})}
              />
              <Input
                label={t('cluster.k8s.clusterService')}
                value={form.cluster_service_name}
                onChange={(e) => setForm({...form, cluster_service_name: e.target.value})}
              />
            </>
          )}
          <div className='grid grid-cols-2 gap-3'>
            <Input
              label='master_http_port'
              type='number'
              value={String(form.master_http_port)}
              onChange={(e) => setForm({...form, master_http_port: Number(e.target.value) || 8080})}
            />
            <Input
              label='hazelcast_port'
              type='number'
              value={String(form.hazelcast_port)}
              onChange={(e) => setForm({...form, hazelcast_port: Number(e.target.value) || 5801})}
            />
          </div>
        </div>
        <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
          <Button variant='secondary' size='sm' onClick={() => onOpenChange(false)} disabled={loading}>
            {t('common.cancel')}
          </Button>
          <Button size='sm' onClick={submit} disabled={loading}>
            {loading ? t('common.creating') : t('common.create')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
