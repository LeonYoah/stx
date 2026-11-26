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
import {Select} from '@/components/arc/select/select';
import {toast} from '@/lib/toast';
import services from '@/lib/services';

interface K8sConfigMapEditorProps {
  clusterId: number;
  heightClass?: string;
}

/**
 * K8s 集群 ConfigMap 查看编辑器：配置 Tab 与节点面板弹窗共用，
 * 数据真相是集群内 ConfigMap（不落库），保存后滚动重启生效。
 * Shared ConfigMap viewer/editor for K8s clusters, used by both the configs tab
 * and the nodes panel dialog. The in-cluster ConfigMap is the single source of
 * truth (nothing persisted); saving triggers a rolling restart to take effect.
 */
export function K8sConfigMapEditor({clusterId, heightClass = 'h-72'}: K8sConfigMapEditorProps) {
  const t = useTranslations();
  const [files, setFiles] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState('');
  const [content, setContent] = useState('');
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await services.cluster.getK8sConfigMap(clusterId);
      setFiles(data);
      // 用函数式更新避免闭包旧值：优先保留当前选中文件，刷新会丢弃未保存编辑（标准语义）。
      // Functional updates avoid stale closures: keep the current selection when possible;
      // a refresh discards unsaved edits (standard semantics).
      setSelected((cur) => {
        const key = cur && data[cur] !== undefined ? cur : (Object.keys(data).sort()[0] || '');
        setContent(data[key] || '');
        setDirty(false);
        return key;
      });
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    void load();
  }, [load]);

  const pick = (key: string) => {
    setSelected(key);
    setContent(files[key] || '');
    setDirty(false);
  };

  const save = async () => {
    setSaving(true);
    try {
      await services.cluster.updateK8sConfigMapFile(clusterId, selected, content, true);
      toast.success(t('common.success'));
      setDirty(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (loading && Object.keys(files).length === 0) {
    return <p className='py-6 text-center text-xs text-muted-foreground'>{t('common.loading')}</p>;
  }
  if (Object.keys(files).length === 0) {
    return <p className='py-6 text-center text-xs text-muted-foreground'>{t('common.noData')}</p>;
  }

  return (
    <div className='space-y-2' data-testid='k8s-configmap-editor'>
      <div className='flex items-center gap-2'>
        <Select
          value={selected}
          options={Object.keys(files).sort().map((k) => ({value: k, label: k}))}
          onValueChange={pick}
        />
        <Button size='sm' variant='secondary' disabled={loading} onClick={() => void load()}>
          {t('common.refresh')}
        </Button>
      </div>
      <textarea
        className={`${heightClass} w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs`}
        value={content}
        onChange={(e) => {
          setContent(e.target.value);
          setDirty(true);
        }}
      />
      <div className='flex justify-end'>
        <Button size='sm' disabled={!dirty || saving} onClick={() => void save()}>
          {t('cluster.k8s.saveAndRestart')}
        </Button>
      </div>
    </div>
  );
}
