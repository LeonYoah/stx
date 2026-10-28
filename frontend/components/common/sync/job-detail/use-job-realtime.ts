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

import {useEffect, useRef, useState} from 'react';
import services from '@/lib/services';
import type {SyncJobPlanDag, SyncJobRealtime} from '@/lib/services/sync';

// 轮询间隔与引擎采集桶（5 秒）对齐。
// Poll interval aligned with the engine collection bucket (5s).
export const REALTIME_POLL_MS = 5000;

export interface JobRealtimeState {
  data: SyncJobRealtime | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
}

/**
 * 拉取作业实时指标：enabled 为假（如不在拓扑页签）时不请求；运行中每 5 秒轮询，页面不可见时暂停，回到可见立即刷新。
 * Fetch job realtime metrics: nothing is requested while disabled (e.g. off the topology tab); running jobs poll
 * every 5s, pausing while the page is hidden and refreshing as soon as it becomes visible again.
 */
export function useJobRealtime(jobId: number | null, enabled: boolean, running: boolean): JobRealtimeState {
  const [state, setState] = useState<JobRealtimeState>({data: null, loading: true, refreshing: false, error: null});
  const loadedRef = useRef(false);

  useEffect(() => {
    loadedRef.current = false;
    setState({data: null, loading: true, refreshing: false, error: null});
  }, [jobId]);

  useEffect(() => {
    if (!jobId || !enabled) {
      return;
    }
    let controller: AbortController | null = null;
    let timer: number | null = null;
    let disposed = false;

    const load = async () => {
      if (disposed || document.visibilityState !== 'visible') {
        return;
      }
      controller?.abort();
      controller = new AbortController();
      if (loadedRef.current) {
        setState((prev) => ({...prev, refreshing: true}));
      }
      try {
        const data = await services.sync.getJobRealtime(jobId, {signal: controller.signal});
        if (!disposed) {
          setState({data, loading: false, refreshing: false, error: null});
        }
      } catch (err) {
        if (!disposed && !(err instanceof DOMException && err.name === 'AbortError')) {
          setState((prev) => ({
            ...prev,
            loading: false,
            refreshing: false,
            error: err instanceof Error ? err.message : String(err),
          }));
        }
      } finally {
        loadedRef.current = true;
      }
    };

    void load();
    if (running) {
      timer = window.setInterval(() => void load(), REALTIME_POLL_MS);
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        void load();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      disposed = true;
      controller?.abort();
      if (timer !== null) {
        window.clearInterval(timer);
      }
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [jobId, enabled, running]);

  return state;
}

// 计划 DAG 会话缓存：同一作业在一次页面会话内只解析一次，抽屉提供手动刷新。
// Plan DAG session cache: each job is resolved once per page session; the drawer offers a manual refresh.
const planDagCache = new Map<number, SyncJobPlanDag>();

export interface JobPlanDagState {
  data: SyncJobPlanDag | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

/**
 * 打开详情时后台读取计划 DAG，不阻塞拓扑首屏；结果按作业缓存。
 * Load the plan DAG in the background when the detail opens without blocking the topology; results are cached per job.
 */
export function useJobPlanDag(jobId: number | null, enabled: boolean): JobPlanDagState {
  const [data, setData] = useState<SyncJobPlanDag | null>(() => (jobId ? planDagCache.get(jobId) ?? null : null));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    if (!jobId || !enabled) {
      return;
    }
    const cached = planDagCache.get(jobId);
    if (cached && nonce === 0) {
      setData(cached);
      setError(null);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    services.sync
      .getJobPlanDag(jobId, {signal: controller.signal})
      .then((result) => {
        planDagCache.set(jobId, result);
        setData(result);
      })
      .catch((err) => {
        if (!(err instanceof DOMException && err.name === 'AbortError') && !controller.signal.aborted) {
          setError(err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [jobId, enabled, nonce]);

  useEffect(() => {
    setData(jobId ? planDagCache.get(jobId) ?? null : null);
    setNonce(0);
  }, [jobId]);

  return {
    data,
    loading,
    error,
    refresh: () => {
      if (jobId) {
        planDagCache.delete(jobId);
      }
      setNonce((value) => value + 1);
    },
  };
}
