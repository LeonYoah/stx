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
import {LogViewerDialog} from './LogViewerDialog';
import layout from '@/components/common/layout/list-detail.module.css';
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
  const [roleFilter, setRoleFilter] = useState<'all' | 'master' | 'worker' | 'hybrid'>('all');
  const [search, setSearch] = useState('');
  const [scaleOpen, setScaleOpen] = useState(false);
  const [scaleMaster, setScaleMaster] = useState('1');
  const [scaleWorker, setScaleWorker] = useState('1');
  const [logPod, setLogPod] = useState<string | null>(null);
  const [restartOpen, setRestartOpen] = useState(false);
  const [restartRole, setRestartRole] = useState<'master' | 'worker' | 'hybrid'>('worker');
  const [deletePodConfirm, setDeletePodConfirm] = useState<string | null>(null);

  const k8sCfg = useMemo(() => {
    const raw = (cluster?.config as Record<string, unknown> | undefined)?.['kubernetes'];
    return (raw || {}) as Record<string, unknown>;
  }, [cluster]);
  const hasMasterWorkload = Boolean(k8sCfg['master_workload_name']);
  const hasWorkerWorkload = Boolean(k8sCfg['worker_workload_name']);
  // hybrid 形态只有单一混合工作负载：扩缩容/滚动重启/角色筛选统一收敛到 hybrid，
  // 不再展示不存在的 master/worker 分离操作。
  // Hybrid topology has a single mixed workload: scale/rollout/role filtering converge
  // on hybrid instead of showing nonexistent separated master/worker operations.
  const isHybrid = String(k8sCfg['deployment_mode'] || '').toLowerCase() === 'hybrid' || Boolean(k8sCfg['hybrid_workload_name']);
  const hasHybridWorkload = Boolean(k8sCfg['hybrid_workload_name']);
  const expectedMaster = Number(k8sCfg['master_replicas']) || 1;
  const expectedWorker = Number(k8sCfg['worker_replicas']) || 1;
  const expectedHybrid = Number(k8sCfg['hybrid_replicas']) || 1;

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
    setScaleMaster(String(isHybrid ? expectedHybrid : expectedMaster));
    setScaleWorker(String(expectedWorker));
    setScaleOpen(true);
  };

  const submitScale = async () => {
    setOperating(true);
    try {
      const jobs: Promise<unknown>[] = [];
      if (isHybrid) {
        if (hasHybridWorkload && Number(scaleMaster) !== expectedHybrid) {
          jobs.push(services.cluster.scaleK8s(clusterId, 'hybrid', Number(scaleMaster)));
        }
      } else {
        if (hasMasterWorkload && Number(scaleMaster) !== expectedMaster) {
          jobs.push(services.cluster.scaleK8s(clusterId, 'master', Number(scaleMaster)));
        }
        if (hasWorkerWorkload && Number(scaleWorker) !== expectedWorker) {
          jobs.push(services.cluster.scaleK8s(clusterId, 'worker', Number(scaleWorker)));
        }
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

  const doRestart = async () => {
    setOperating(true);
    try {
      await services.cluster.rolloutRestartK8s(clusterId, restartRole);
      toast.success(t('common.success'));
      setRestartOpen(false);
      await loadPods();
      onAfterMutate?.();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  // 单 Pod 在 K8s 里没有「重启」语义：Pod 不可变，正确的单粒度操作是删除后由控制器重建。
  // A single pod has no "restart" semantics in Kubernetes: pods are immutable, so the
  // correct pod-level operation is deletion followed by controller recreation.
  const doDeletePod = async (podName: string) => {
    setOperating(true);
    try {
      await services.cluster.deleteK8sPod(clusterId, podName);
      toast.success(t('common.success'));
      setDeletePodConfirm(null);
      await loadPods();
      onAfterMutate?.();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  const openPodLogs = (podName: string) => {
    setLogPod(podName);
  };

  // kubectl logs 只有 tail 语义：行数下传给后端，grep 过滤在客户端做（结果集已由 tailLines 约束）。
  // kubectl logs only has tail semantics: the line count goes to the backend while grep
  // filtering happens client-side (the result set is already bounded by tailLines).
  const logFetcher = useCallback(
    async (params: {lines: number; filter?: string}) => {
      if (!logPod) {
        return {error: t('cluster.getLogsError')};
      }
      try {
        const content = await services.cluster.getK8sPodLogs(clusterId, logPod, params.lines);
        if (params.filter) {
          const needle = params.filter.toLowerCase();
          const filtered = content
            .split('\n')
            .filter((line) => line.toLowerCase().includes(needle))
            .join('\n');
          return {logs: filtered};
        }
        return {logs: content};
      } catch (e) {
        return {error: (e as Error).message};
      }
    },
    [clusterId, logPod, t],
  );

  return (
    <div className='space-y-3' data-testid='k8s-nodes-panel'>
      <div className={layout.toolbar}>
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
            ...(isHybrid
              ? [{value: 'hybrid', label: t('cluster.roles.hybrid')}]
              : [
                  {value: 'master', label: t('cluster.roles.master')},
                  {value: 'worker', label: t('cluster.roles.worker')},
                ]),
          ]}
        />
        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('cluster.k8s.searchPods')}
            value={search}
            onValueChange={setSearch}
            placeholder={t('cluster.k8s.searchPods')}
            clearLabel={t('cluster.list.searchClear')}
          />
        </div>
        <div className={layout.toolbarRight}>
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
          {hasMasterWorkload || hasWorkerWorkload ? (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              disabled={operating}
              data-testid='k8s-rollout-open'
              onClick={() => {
                setRestartRole(isHybrid ? 'hybrid' : hasWorkerWorkload ? 'worker' : 'master');
                setRestartOpen(true);
              }}
            >
              {t('cluster.k8s.rolloutRestart')}
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
                      onClick={() => openPodLogs(pod.name)}
                    >
                      <FileText className='size-3.5' aria-hidden />
                    </Button>
                    {pod.role === 'master' || pod.role === 'worker' || pod.role === 'hybrid' ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        iconOnly
                        aria-label={t('cluster.k8s.recreatePod')}
                        title={t('cluster.k8s.recreatePod')}
                        disabled={operating}
                        onClick={() => setDeletePodConfirm(pod.name)}
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
              {isHybrid && hasHybridWorkload ? (
                <div className='space-y-1'>
                  <label className='text-xs text-muted-foreground'>{t('cluster.roles.hybrid')}</label>
                  <Input type='number' min={0} max={50} label={t('cluster.roles.hybrid')} value={scaleMaster} onChange={(e) => setScaleMaster(e.target.value)} />
                </div>
              ) : null}
              {!isHybrid && hasMasterWorkload ? (
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

      {restartOpen ? (
        <Dialog open onOpenChange={() => setRestartOpen(false)}>
          <DialogContent title={t('cluster.k8s.rolloutRestart')} className='arc-compact w-[min(94vw,440px)]!'>
            <div className='space-y-3'>
              <SegmentedControl
                label={t('cluster.k8s.podRole')}
                value={restartRole}
                onValueChange={(v) => setRestartRole(v as typeof restartRole)}
                options={[
                  ...(hasMasterWorkload ? [{value: 'master', label: t('cluster.roles.master')}] : []),
                  ...(hasWorkerWorkload ? [{value: 'worker', label: t('cluster.roles.worker')}] : []),
                ]}
              />
              <p className='text-xs text-muted-foreground'>
                {t('cluster.k8s.rolloutRestartConfirm', {role: t(`cluster.roles.${restartRole}`)})}
              </p>
            </div>
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' onClick={() => setRestartOpen(false)}>{t('common.cancel')}</Button>
              <Button size='sm' disabled={operating} onClick={() => void doRestart()} data-testid='k8s-rollout-submit'>{t('common.confirm')}</Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}

      {deletePodConfirm ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeletePodConfirm(null);
          }}
          title={t('cluster.k8s.recreatePod')}
          description={t('cluster.k8s.recreatePodConfirm', {name: deletePodConfirm})}
          confirmLabel={t('common.confirm')}
          cancelLabel={t('common.cancel')}
          onConfirm={() => void doDeletePod(deletePodConfirm)}
        />
      ) : null}

      {logPod ? (
        <LogViewerDialog
          open
          onOpenChange={(open) => {
            if (!open) setLogPod(null);
          }}
          title={
            <span className='flex flex-wrap items-center gap-2'>
              <FileText className='size-4 shrink-0 text-primary' aria-hidden />
              <span>{t('cluster.k8s.podLogs')}</span>
              <NeutralChip className='font-mono tabular-nums'>{logPod}</NeutralChip>
            </span>
          }
          fetcher={logFetcher}
          capabilities={{modes: false, date: false}}
        />
      ) : null}
    </div>
  );
}
