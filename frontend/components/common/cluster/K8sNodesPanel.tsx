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
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import type {K8sPodInfo} from '@/lib/services/cluster/types';

interface K8sNodesPanelProps {
  clusterId: number;
}

/**
 * K8s 集群节点面板：Pod 投影表 + 扩缩容/滚动重启/镜像升级/ConfigMap 查看编辑。
 * K8s cluster nodes panel: pod projection table plus scale / rollout restart /
 * image upgrade / ConfigMap view & edit.
 */
export function K8sNodesPanel({clusterId}: K8sNodesPanelProps) {
  const t = useTranslations();
  const [pods, setPods] = useState<K8sPodInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [scaleRole, setScaleRole] = useState<'master' | 'worker' | null>(null);
  const [scaleReplicas, setScaleReplicas] = useState('1');
  const [upgradeOpen, setUpgradeOpen] = useState(false);
  const [upgradeImage, setUpgradeImage] = useState('');
  const [cmOpen, setCmOpen] = useState(false);
  const [cmFiles, setCmFiles] = useState<Record<string, string>>({});
  const [cmSelected, setCmSelected] = useState('');
  const [cmContent, setCmContent] = useState('');
  const [cmDirty, setCmDirty] = useState(false);
  const [operating, setOperating] = useState(false);

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

  const doScale = async () => {
    if (!scaleRole) return;
    setOperating(true);
    try {
      await services.cluster.scaleK8s(clusterId, scaleRole, Number(scaleReplicas) || 1);
      toast.success(t('common.success'));
      setScaleRole(null);
      void loadPods();
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
      void loadPods();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  const doUpgrade = async () => {
    if (!upgradeImage.trim()) return;
    setOperating(true);
    try {
      await services.cluster.upgradeK8sImage(clusterId, upgradeImage.trim());
      toast.success(t('common.success'));
      setUpgradeOpen(false);
      void loadPods();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setOperating(false);
    }
  };

  const openConfigMap = async () => {
    setCmOpen(true);
    try {
      const files = await services.cluster.getK8sConfigMap(clusterId);
      setCmFiles(files);
      const first = Object.keys(files).sort()[0] || '';
      setCmSelected(first);
      setCmContent(first ? files[first] : '');
      setCmDirty(false);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const sel = 'rounded-md border border-input bg-transparent px-2 py-1.5 text-sm';

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Button size='sm' variant='secondary' disabled={operating} onClick={() => { setScaleRole('worker'); setScaleReplicas(String(pods.filter((p) => p.role === 'worker').length || 1)); }}>
          {t('cluster.k8s.scaleWorker')}
        </Button>
        <Button size='sm' variant='secondary' disabled={operating} onClick={() => { setScaleRole('master'); setScaleReplicas(String(pods.filter((p) => p.role === 'master').length || 1)); }}>
          {t('cluster.k8s.scaleMaster')}
        </Button>
        <Button size='sm' variant='secondary' disabled={operating} onClick={() => void doRestart('worker')}>
          {t('cluster.k8s.restartWorker')}
        </Button>
        <Button size='sm' variant='secondary' disabled={operating} onClick={() => void doRestart('master')}>
          {t('cluster.k8s.restartMaster')}
        </Button>
        <Button size='sm' variant='secondary' disabled={operating} onClick={() => setUpgradeOpen(true)}>
          {t('cluster.k8s.upgradeImage')}
        </Button>
        <Button size='sm' variant='secondary' onClick={() => void openConfigMap()}>
          ConfigMap
        </Button>
        <Button size='sm' variant='secondary' disabled={loading} onClick={() => void loadPods()}>
          {t('common.refresh')}
        </Button>
      </div>

      <div className='overflow-x-auto rounded-lg border border-border/70'>
        <table className='w-full text-sm'>
          <thead>
            <tr className='border-b border-border/70 text-left text-xs text-muted-foreground'>
              <th className='px-3 py-2'>{t('cluster.k8s.podName')}</th>
              <th className='px-3 py-2'>{t('cluster.k8s.podRole')}</th>
              <th className='px-3 py-2'>{t('cluster.k8s.podReady')}</th>
              <th className='px-3 py-2'>{t('cluster.k8s.podPhase')}</th>
              <th className='px-3 py-2'>IP</th>
              <th className='px-3 py-2'>{t('cluster.k8s.podRestarts')}</th>
            </tr>
          </thead>
          <tbody>
            {pods.map((pod) => (
              <tr key={pod.name} className='border-b border-border/40 last:border-0'>
                <td className='px-3 py-2 font-mono text-xs'>{pod.name}</td>
                <td className='px-3 py-2'>{pod.role}</td>
                <td className='px-3 py-2'>{pod.ready ? '✓' : '—'}</td>
                <td className='px-3 py-2'>{pod.phase}</td>
                <td className='px-3 py-2 font-mono text-xs'>{pod.pod_ip}</td>
                <td className='px-3 py-2'>{pod.restarts}</td>
              </tr>
            ))}
            {pods.length === 0 && !loading ? (
              <tr><td colSpan={6} className='px-3 py-6 text-center text-muted-foreground'>{t('common.noData')}</td></tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {scaleRole ? (
        <Dialog open onOpenChange={() => setScaleRole(null)}>
          <DialogContent title={t('cluster.k8s.scaleTitle')} className='arc-compact w-[min(94vw,420px)]!'>
            <Input
              label={t('cluster.k8s.replicas')}
              type='number'
              value={scaleReplicas}
              onChange={(e) => setScaleReplicas(e.target.value)}
            />
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' onClick={() => setScaleRole(null)}>{t('common.cancel')}</Button>
              <Button size='sm' disabled={operating} onClick={() => void doScale()}>{t('common.confirm')}</Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}

      {upgradeOpen ? (
        <Dialog open onOpenChange={() => setUpgradeOpen(false)}>
          <DialogContent title={t('cluster.k8s.upgradeImage')} className='arc-compact w-[min(94vw,480px)]!'>
            <p className='mb-2 text-xs text-muted-foreground'>{t('cluster.k8s.upgradeHint')}</p>
            <Input
              label={t('cluster.k8s.image')}
              value={upgradeImage}
              onChange={(e) => setUpgradeImage(e.target.value)}
              placeholder='apache/seatunnel:3.1.0'
            />
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' onClick={() => setUpgradeOpen(false)}>{t('common.cancel')}</Button>
              <Button size='sm' disabled={operating} onClick={() => void doUpgrade()}>{t('common.confirm')}</Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}

      {cmOpen ? (
        <Dialog open onOpenChange={() => setCmOpen(false)}>
          <DialogContent title='ConfigMap' className='arc-compact w-[min(94vw,720px)]!'>
            <div className='space-y-2'>
              <select
                className={sel}
                value={cmSelected}
                onChange={(e) => {
                  setCmSelected(e.target.value);
                  setCmContent(cmFiles[e.target.value] || '');
                  setCmDirty(false);
                }}
              >
                {Object.keys(cmFiles).sort().map((k) => (
                  <option key={k} value={k}>{k}</option>
                ))}
              </select>
              <textarea
                className='h-72 w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs'
                value={cmContent}
                onChange={(e) => { setCmContent(e.target.value); setCmDirty(true); }}
              />
            </div>
            <div className='mt-4 flex justify-end gap-2'>
              <Button size='sm' variant='secondary' onClick={() => setCmOpen(false)}>{t('common.close')}</Button>
              <Button
                size='sm'
                disabled={!cmDirty || operating}
                onClick={() => {
                  setOperating(true);
                  services.cluster.updateK8sConfigMapFile(clusterId, cmSelected, cmContent, true)
                    .then(() => { toast.success(t('common.success')); setCmDirty(false); })
                    .catch((e) => toast.error((e as Error).message))
                    .finally(() => setOperating(false));
                }}
              >
                {t('cluster.k8s.saveAndRestart')}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </div>
  );
}
