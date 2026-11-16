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

export interface PolledResource<T> {
  data: T | null;
  /** 首次加载（还没有任何数据）/ First load (no data yet). */
  loading: boolean;
  /** 已有数据时的后台刷新 / Background refresh while data is shown. */
  refreshing: boolean;
  error: string | null;
  updatedAt: number | null;
  reload: () => void;
}

/**
 * 按间隔轮询一个资源：页面不可见时暂停，回到可见立即刷新；key 变化时清空旧数据重新加载。
 * 刷新失败保留上一份数据，只更新 error，避免整块塌陷。
 * Poll a resource on an interval: pause while the page is hidden and refresh as soon as it is visible again; a new key
 * clears the old data and reloads. A failed refresh keeps the previous data and only sets error, so nothing collapses.
 */
export function usePolledResource<T>(
  key: string,
  loader: (signal: AbortSignal) => Promise<T>,
  intervalMs: number,
  enabled = true,
): PolledResource<T> {
  const [state, setState] = useState<Omit<PolledResource<T>, 'reload'>>({
    data: null,
    loading: true,
    refreshing: false,
    error: null,
    updatedAt: null,
  });
  const loaderRef = useRef(loader);
  loaderRef.current = loader;
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick((value) => value + 1), []);

  useEffect(() => {
    setState({
      data: null,
      loading: true,
      refreshing: false,
      error: null,
      updatedAt: null,
    });
  }, [key]);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    let disposed = false;
    let controller: AbortController | null = null;
    const load = async () => {
      if (disposed || document.visibilityState !== 'visible') {
        return;
      }
      controller?.abort();
      controller = new AbortController();
      setState((prev) =>
        prev.data === null ? prev : {...prev, refreshing: true},
      );
      try {
        const data = await loaderRef.current(controller.signal);
        if (!disposed) {
          setState({
            data,
            loading: false,
            refreshing: false,
            error: null,
            updatedAt: Date.now(),
          });
        }
      } catch (err) {
        if (
          disposed ||
          (err instanceof DOMException && err.name === 'AbortError')
        ) {
          return;
        }
        setState((prev) => ({
          ...prev,
          loading: false,
          refreshing: false,
          error: err instanceof Error ? err.message : String(err),
        }));
      }
    };
    void load();
    // 间隔为 0 时只拉一次（如固定的自定义时间窗）。/ An interval of 0 loads once (e.g. a fixed custom window).
    const timer =
      intervalMs > 0 ? window.setInterval(() => void load(), intervalMs) : null;
    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        void load();
      }
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      disposed = true;
      controller?.abort();
      if (timer !== null) {
        window.clearInterval(timer);
      }
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [key, intervalMs, enabled, tick]);

  return {...state, reload};
}
