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

import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import services from '@/lib/services';
import type {RuntimeStorageListItem} from '@/lib/services/cluster/types';
import type {
  SyncCheckpointSnapshot,
  SyncJobInstance,
} from '@/lib/services/sync';
import {
  getSyncJobClusterId,
  submitSpecExecutionMode,
} from '../sync-studio-utils';

// 快照条数上限与轮询间隔，沿用工作台底部面板的取值。
// Snapshot row cap and poll interval, matching the workbench bottom console.
const CHECKPOINT_SNAPSHOT_LIMIT = 20;
const CHECKPOINT_POLL_MS = 3000;

export interface UseJobCheckpointOptions {
  /** 当前作业；为空或无效（local 模式 / 无集群 / 无引擎作业号）时文件列表置空。/ Current job; the file list stays empty for null or invalid jobs (local mode / no cluster / no engine job id). */
  job: SyncJobInstance | null;
  /** 页签可见性开关：false 时不加载、不轮询。/ Tab visibility gate: no load / poll while false. */
  enabled: boolean;
}

export interface UseJobCheckpointResult {
  snapshot: SyncCheckpointSnapshot | null;
  loading: boolean;
  files: RuntimeStorageListItem[];
  filesLoading: boolean;
  /** 手动刷新（非静默：带 loading 与错误 toast），对应面板的 onRefresh。/ Manual refresh (non-silent: loading flag + error toast), wired to the panel's onRefresh. */
  reload: () => void;
}

/**
 * 作业 Checkpoint 数据 hook：快照（limit 20）+ 运行时存储文件列表；作业 pending/running 且页签可见时 3s 静默轮询。
 * 带请求版本号防串：切换作业后迟到的旧响应会被丢弃（旧实现存在旧作业文件回写新作业的窗口）。
 * Job checkpoint hook: snapshot (limit 20) + runtime storage file list; silently polls every 3s while the job is
 * pending/running and the tab is visible. A request-version guard drops stale responses arriving after a job
 * switch (the old implementation had a window where the previous job's files could be written back).
 */
export function useJobCheckpoint({
  job,
  enabled,
}: UseJobCheckpointOptions): UseJobCheckpointResult {
  const t = useTranslations('workbenchStudio');
  const [snapshot, setSnapshot] = useState<SyncCheckpointSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [files, setFiles] = useState<RuntimeStorageListItem[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const requestVersionRef = useRef(0);
  const jobId = job?.id ?? null;

  // 作业切换时复位快照与文件列表（旧实现只清快照，文件会残留到新请求返回）。
  // Reset snapshot and file list on job switch (the old implementation only cleared the snapshot; stale
  // files lingered until the next response landed).
  useEffect(() => {
    requestVersionRef.current += 1;
    setSnapshot(null);
    setFiles([]);
    setLoading(false);
    setFilesLoading(false);
  }, [jobId]);

  const loadSnapshot = useCallback(
    async (id: number | null, silent = false) => {
      if (!id) {
        setSnapshot(null);
        return;
      }
      const requestVersion = requestVersionRef.current;
      if (!silent) {
        setLoading(true);
      }
      try {
        const data = await services.sync.getJobCheckpoint(id, {
          limit: CHECKPOINT_SNAPSHOT_LIMIT,
        });
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        setSnapshot(data);
      } catch (error) {
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        if (!silent) {
          toast.error(
            error instanceof Error ? error.message : t('loadCheckpointFailed'),
          );
        }
        setSnapshot(null);
      } finally {
        if (!silent && requestVersionRef.current === requestVersion) {
          setLoading(false);
        }
      }
    },
    [t],
  );

  const loadFiles = useCallback(
    async (target: SyncJobInstance | null, silent = false) => {
      const clusterId = getSyncJobClusterId(target);
      const engineJobId = (
        target?.engine_job_id ||
        target?.platform_job_id ||
        ''
      ).trim();
      if (
        !target ||
        submitSpecExecutionMode(target.submit_spec) === 'local' ||
        !clusterId ||
        !engineJobId
      ) {
        setFiles([]);
        return;
      }
      const requestVersion = requestVersionRef.current;
      if (!silent) {
        setFilesLoading(true);
      }
      try {
        const runtimeStorageResult =
          await services.cluster.getRuntimeStorageSafe(clusterId);
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        const namespace = runtimeStorageResult.success
          ? runtimeStorageResult.data?.checkpoint?.namespace?.trim() || ''
          : '';
        if (!namespace) {
          setFiles([]);
          return;
        }
        const browsePath = `${namespace.replace(/\/+$/, '')}/${engineJobId}`;
        const listResult = await services.cluster.listRuntimeStorageSafe(
          clusterId,
          'checkpoint',
          {
            path: browsePath,
            recursive: true,
            limit: 200,
          },
        );
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        if (!listResult.success || !listResult.data) {
          setFiles([]);
          return;
        }
        const items = Array.isArray(listResult.data.items)
          ? listResult.data.items
              .filter((item) => !item.directory && !!item.path)
              .sort((left, right) =>
                (right.modified_at || '').localeCompare(left.modified_at || ''),
              )
          : [];
        setFiles(items);
      } finally {
        if (!silent && requestVersionRef.current === requestVersion) {
          setFilesLoading(false);
        }
      }
    },
    [],
  );

  // 页签可见时立即加载快照与文件列表（与原 checkpointVisible 副作用一致）。
  // Load snapshot and file list as soon as the tab becomes visible (matching the old checkpointVisible effects).
  useEffect(() => {
    if (!enabled) {
      return;
    }
    void loadSnapshot(jobId);
  }, [enabled, jobId, loadSnapshot]);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    void loadFiles(job);
  }, [enabled, job, loadFiles]);

  // 作业 pending/running 且页签可见时 3s 静默轮询。
  // Poll silently every 3s while the job is pending/running and the tab is visible.
  useEffect(() => {
    if (
      !enabled ||
      !job ||
      (job.status !== 'pending' && job.status !== 'running')
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadSnapshot(job.id, true);
      void loadFiles(job, true);
    }, CHECKPOINT_POLL_MS);
    return () => window.clearInterval(timer);
  }, [enabled, job, loadSnapshot, loadFiles]);

  const reload = useCallback(() => {
    void loadSnapshot(jobId);
    void loadFiles(job);
  }, [jobId, job, loadSnapshot, loadFiles]);

  return {snapshot, loading, files, filesLoading, reload};
}
