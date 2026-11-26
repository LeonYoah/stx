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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {FileText, RefreshCw, RotateCw} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {ConfirmDialog, NeutralChip, SoftChip, TableCard} from '@/components/common/layout';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import type {K8sPodInfo} from '@/lib/services/cluster/types';
import type {ClusterInfo} from '@/lib/services/cluster/types';

interface K8sNodesPanelProps {
  clusterId: number;
  cluster?: ClusterInfo | null;
  refreshTrigger?: number;
  onAfterMutate?: () => void;
}

/**
 * K8s 集群节点面板：与物理机节点 Tab 同一套视觉语言（筛选工具栏 + 表格 + 行内操作），
 * 唯一主按钮是「调整副本数」；配置/升级/Proxy 一律归属于各自 Tab，不在此堆砌。
 * K8s cluster nodes panel sharing the bare-metal nodes tab visual language (filter
 * toolbar + table + row actions) with a single primary "adjust replicas" button;
 * config/upgrade/proxy concerns live in their own tabs and are never stacked here.
 */
export function K8sNodesPanel({clusterId, cluster, refreshTrigger, onAfterMutate}: K8sNodesPanelProps) {
  const t = useTranslations();
  const [pods, setPods] = useState<K8sPodInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [operating, setOperating] = useState(false);
  const [statusFilter, setStatusFilter] = useState<'all' | 'ready' | 'abnormal'>('all');
  const [roleFilter, setRoleFilter] = useState<'all' | 'master' | 'worker'>('all');
  const [search, setSearch] = useState('');
  const [scaleOpen, setScaleOpen] = useState(false);
  const [scaleMaster, setScaleMaster] = useState('1');
  const [scaleWorker, setScaleWorker] = useState('1');
  const [logPod, setLogPod] = useState<string | null>(null);
  const [logText, setLogText] = useState('');
  const [logLoading, setLogLoading] = useState(false);
  const [restartConfirm, setRestartConfirm] = useState<'master' | 'worker' | null>(null);

  const k8sCfg = useMemo(() => {
    const raw = (cluster?.config as Record<string, unknown> | undefined)?.['kubernetes'];
    return (raw || {}) as Record<string, unknown>;
  }, [cluster]);
  const hasMasterWorkload = Boolean(k8sCfg['master_workload_name']);
  const hasWorkerWorkload = Boolean(k8sCfg['worker_workload_name']);
  const expectedMaster = Number(k8sCfg['master_replicas']) || 1;
  const expectedWorker = Number(k8sCfg['worker_replicas']) || 1;

  const loadPods = useCallback(async () => {
    setLoading(true);
    try {
      const list = await services.cluster.getK8sPods(clusterId);
      setPods(list);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    void loadPods();
  }, [loadPods]);

  useEffect(() => {
    if (refreshTrigger && refreshTrigger > 0) {
      void loadPods();
    }
  }, [refreshTrigger, loadPods]);

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return pods.filter((pod) => {
      if (statusFilter === 'ready' && !(pod.ready && pod.phase === 'Running')) return false;
      if (statusFilter === 'abnormal' && pod.ready && pod.phase === 'Running') return false;
      if (roleFilter !== 'all' && pod.role !== roleFilter) return false;
      if (keyword) {
        const hay = `${pod.name} ${pod.pod_ip} ${pod.node_name}`.toLowerCase();
        if (!hay.includes(keyword)) return false;
      }
      return true;
    });
  }, [pods, statusFilter, roleFilter, search]);

  const hasActiveFilter = statusFilter !== 'all' || roleFilter !== 'all' || search.trim() !== '';

  const openScale = () => {
    setScaleMaster(String(expectedMaster));
    setScaleWorker(String(expectedWorker));
    setScaleOpen(true);
  };

  const submitScale = async () => {
    setOperating(true);
    try {
      const jobs: Promise<unknown>[] = [];
      if (hasMasterWorkload && Number(scaleMaster) !== expectedMaster) {
        jobs.push(services.cluster.scaleK8s(clusterId, 'master', Number(scaleMaster)));
      }
      if (hasWorkerWorkload && Number(scaleWorker) !== expectedWorker) {
        jobs.push(services.cluster.scaleK8s(clusterId, 'worker', Number(scaleWorker)));
      }
      if (jobs.length === 0) {
        setScaleOpen(false);
        return;
      }
      await Promise.all(jobs);
      toast.success(t('common.success'));
      setScaleOpen(false);
      await loadPods();
      onAfterMutate?.();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  const doRestart = async (role: 'master' | 'worker') => {
    setOperating(true);
    try {
      await services.cluster.rolloutRestartK8s(clusterId, role);
      toast.success(t('common.success'));
      setRestartConfirm(null);
      await loadPods();
      onAfterMutate?.();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  const openPodLogs = async (podName: string) => {
    setLogPod(podName);
    setLogText('');
    setLogLoading(true);
    try {
      const content = await services.cluster.getK8sPodLogs(clusterId, podName);
      setLogText(content);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLogLoading(false);
    }
  };

  return (
    <div className='space-y-3' data-testid='k8s-nodes-panel'>
      <div className='flex flex-wrap items-center gap-2'>
        <SegmentedControl
          label={t('cluster.k8s.filterStatus')}
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as typeof statusFilter)}
          options={[
            {value: 'all', label: t('cluster.k8s.filterAll')},
            {value: 'ready', label: t('cluster.k8s.filterReady')},
            {value: 'abnormal', label: t('cluster.k8s.filterAbnormal')},
          ]}
        />
        <SegmentedControl
          label={t('cluster.k8s.podRole')}
          value={roleFilter}
          onValueChange={(v) => setRoleFilter(v as typeof roleFilter)}
          options={[
            {value: 'all', label: t('cluster.k8s.filterAll')},
            {value: 'master', label: t('cluster.roles.master')},
            {value: 'worker', label: t('cluster.roles.worker')},
          ]}
        />
        <SearchField
          label={t('common.search')}
          value={search}
          onValueChange={setSearch}
          placeholder={t('cluster.k8s.searchPods')}
          className='w-56'
        />
        <div className='ml-auto flex items-center gap-2'>
          {hasActiveFilter ? (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => {
                setStatusFilter('all');
                setRoleFilter('all');
                setSearch('');
              }}
            >
              {t('common.clearFilters')}
            </Button>
          ) : null}
          <Button type='button' size='sm' onClick={openScale} disabled={operating} data-testid='k8s-scale-open'>
            {t('cluster.k8s.adjustReplicas')}
          </Button>
        </div>
      </div>

      <TableCard
        loading={loading}
        hasData={filtered.length > 0}
        skeletonColumns={6}
        skeletonRows={4}
        empty={
          <EmptyState
            title={t('common.noData')}
            description={hasActiveFilter ? t('plugin.noMatchingPluginsDesc') : ''}
          />
        }
      >
        <div className='overflow-x-auto'>
          <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
            <TableHeader>
              <TableRow>
                <TableHead className='pl-4'>{t('cluster.k8s.podName')}</TableHead>
                <TableHead>{t('cluster.k8s.podRole')}</TableHead>
                <TableHead>{t('cluster.k8s.podNode')}</TableHead>
                <TableHead>{t('cluster.k8s.podIP')}</TableHead>
                <TableHead>{t('cluster.k8s.podPhase')}</TableHead>
                <TableHead>{t('cluster.k8s.podRestarts')}</TableHead>
                <TableHead className='pr-4 text-right'>{t('plugin.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
            {filtered.map((pod) => (
              <TableRow key={pod.name} data-testid={`k8s-pod-row-${pod.name}`}>
                <TableCell className='pl-4 font-mono text-xs'>{pod.name}</TableCell>
                <TableCell>
                  {pod.role ? <NeutralChip>{t(`cluster.roles.${pod.role}`)}</NeutralChip> : <span className='text-xs text-muted-foreground'>—</span>}
                </TableCell>
                <TableCell className='text-xs'>{pod.node_name || '-'}</TableCell>
                <TableCell className='font-mono text-xs tabular-nums'>{pod.pod_ip || '-'}</TableCell>
                <TableCell>
                  <SoftChip tone={pod.ready && pod.phase === 'Running' ? 'emerald' : 'rose'}>
                    {pod.ready && pod.phase === 'Running' ? t('cluster.k8s.filterReady') : pod.phase || t('cluster.k8s.filterAbnormal')}
                  </SoftChip>
                </TableCell>
                <TableCell className='text-xs tabular-nums'>{pod.restarts}</TableCell>
                <TableCell className='pr-4'>
                  <div className='flex justify-end gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('cluster.k8s.podLogs')}
                      title={t('cluster.k8s.podLogs')}
                      onClick={() => void openPodLogs(pod.name)}
                    >
                      <FileText className='size-3.5' aria-hidden />
                    </Button>
                    {pod.role === 'master' || pod.role === 'worker' ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        iconOnly
                        aria-label={t('cluster.k8s.rolloutRestart')}
                        title={t('cluster.k8s.rolloutRestart')}
                        disabled={operating}
                        onClick={() => setRestartConfirm(pod.role as 'master' | 'worker')}
                      >
                        <RotateCw className='size-3.5' aria-hidden />
                      </Button>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            </TableBody>
          </Table>
        </div>
      </TableCard>

      {scaleOpen ? (
        <Dialog open onOpenChange={() => setScaleOpen(false)}>
          <DialogContent title={t('cluster.k8s.adjustReplicas')} className='arc-compact w-[min(94vw,480px)]!'>
            <div className='space-y-3'>
              {hasMasterWorkload ? (
                <div className='space-y-1'>
                  <label className='text-xs text-muted-foreground'>{t('cluster.roles.master')}</label>
                  <Input type='number' min={0} max={50} label={t('cluster.roles.master')} value={scaleMaster} onChange={(e) => setScaleMaster(e.target.value)} />
                </div>
              ) : null}
              {hasWorkerWorkload ? (
                <div className='space-y-1'>
                  <label className='text-xs text-muted-foreground'>{t('cluster.roles.worker')}</label>
                  <Input type='number' min={0} max={50} label={t('cluster.roles.worker')} value={scaleWorker} onChange={(e) => setScaleWorker(e.target.value)} />
                </div>
              ) : null}
              <p className='text-xs text-muted-foreground'>{t('cluster.k8s.scaleHint')}</p>
            </div>
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' onClick={() => setScaleOpen(false)}>{t('common.cancel')}</Button>
              <Button size='sm' disabled={operating} onClick={() => void submitScale()} data-testid='k8s-scale-submit'>{t('common.confirm')}</Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}

      {restartConfirm ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setRestartConfirm(null);
          }}
          title={t('cluster.k8s.rolloutRestart')}
          description={t('cluster.k8s.rolloutRestartConfirm', {role: t(`cluster.roles.${restartConfirm}`)})}
          confirmLabel={t('common.confirm')}
          cancelLabel={t('common.cancel')}
          onConfirm={() => void doRestart(restartConfirm)}
        />
      ) : null}

      {logPod ? (
        <Dialog open onOpenChange={() => setLogPod(null)}>
          <DialogContent title={`${t('cluster.k8s.podLogs')} · ${logPod}`} className='arc-compact w-[min(94vw,820px)]!'>
            <pre className='h-96 overflow-auto rounded-md border border-border/60 bg-muted/30 p-3 font-mono text-xs whitespace-pre-wrap'>
              {logLoading ? t('common.loading') : (logText || t('common.noData'))}
            </pre>
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' disabled={logLoading} onClick={() => void openPodLogs(logPod)}>
                <RefreshCw className='size-3.5' aria-hidden />
                {t('common.refresh')}
              </Button>
              <Button size='sm' variant='secondary' onClick={() => setLogPod(null)}>
                {t('common.close')}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </div>
  );
}
