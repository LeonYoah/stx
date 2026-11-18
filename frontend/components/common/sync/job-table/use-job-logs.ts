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
import services from '@/lib/services';
import type {SyncJobLogsResult} from '@/lib/services/sync';
import {
  EXPANDED_LOG_CHUNK_BASE_BYTES,
  EXPANDED_LOG_CHUNK_MAX_BYTES,
  LOG_CHUNK_BASE_BYTES,
  LOG_CHUNK_MAX_BYTES,
  nextLogChunkSize,
  type LogFilterMode,
} from '../sync-studio-utils';

// 轮询间隔与工作台底部面板一致（3s）。
// Poll interval matching the workbench bottom console (3s).
const JOB_LOGS_POLL_MS = 3000;

export interface UseJobLogsOptions {
  /** 作业实例 ID；为空时不发请求。/ Job instance id; no request while null. */
  jobId: number | null;
  /** 可见性开关：false 时不加载、不轮询，进行中的请求被中止。/ Visibility gate: no load / poll while false; in-flight requests abort. */
  enabled: boolean;
  /** 日志关键字（透传后端搜索）。/ Log keyword forwarded to the backend search. */
  keyword: string;
  /** 级别筛选；'all' 不传 level 参数。/ Level filter; 'all' omits the level param. */
  level: LogFilterMode;
  /** 展开（大日志弹窗）模式：使用更大的分块上下界。/ Expanded (full log viewer) mode: bigger chunk bounds. */
  expanded?: boolean;
}

export interface UseJobLogsResult {
  result: SyncJobLogsResult | null;
  loading: boolean;
  /** 已消费的分块游标（沿用旧实现的 jobLogsOffset 状态，便于调试与展示）。/ Consumed chunk cursor (kept for parity with the old jobLogsOffset state). */
  offset: string;
  /** 手动重拉一次（enabled=false 时与旧实现一样直接返回）。/ Manual reload (no-ops while disabled, same as the old implementation). */
  reload: () => void;
  /** 复位：中止进行中请求、版本号 +1、清空结果并回到初始分块。/ Reset: abort in-flight requests, bump the request version, clear results and restore the base chunk size. */
  reset: () => void;
}

/**
 * 作业执行日志加载 hook：增量 offset 拼接、nextLogChunkSize 自适应分块、AbortController 取消、
 * 请求版本号防串（切换作业 / 关键字 / 级别后迟到的旧响应被丢弃）、enabled 时 3s 轮询。
 * 普通视图与展开弹窗各持一个实例，不再共享 ref。
 * Job execution log hook: incremental offset appending, adaptive nextLogChunkSize, AbortController
 * cancellation, a request-version guard (stale responses arriving after job / keyword / level changes
 * are dropped) and a 3s poll while enabled. The normal view and the expanded dialog each hold their
 * own instance instead of sharing refs.
 */
export function useJobLogs({
  jobId,
  enabled,
  keyword,
  level,
  expanded = false,
}: UseJobLogsOptions): UseJobLogsResult {
  const [result, setResult] = useState<SyncJobLogsResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [offset, setOffset] = useState('');
  const offsetRef = useRef('');
  const baseChunkSize = expanded
    ? EXPANDED_LOG_CHUNK_BASE_BYTES
    : LOG_CHUNK_BASE_BYTES;
  const maxChunkSize = expanded
    ? EXPANDED_LOG_CHUNK_MAX_BYTES
    : LOG_CHUNK_MAX_BYTES;
  const chunkSizeRef = useRef(baseChunkSize);
  const abortRef = useRef<AbortController | null>(null);
  const requestVersionRef = useRef(0);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    requestVersionRef.current += 1;
    setResult(null);
    setOffset('');
    offsetRef.current = '';
    chunkSizeRef.current = baseChunkSize;
    setLoading(false);
  }, [baseChunkSize]);

  // 作业 / 关键字 / 级别变化时整体复位（声明在加载副作用之前，保证下一次加载从空游标开始）。
  // Reset on job / keyword / level changes (declared before the load effect so the next load starts from an empty cursor).
  useEffect(() => {
    reset();
  }, [jobId, keyword, level, reset]);

  const load = useCallback(async () => {
    if (!jobId || !enabled) {
      return;
    }
    const requestVersion = requestVersionRef.current;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    try {
      const currentOffset = offsetRef.current;
      const data = await services.sync.getJobLogs(jobId, {
        offset: currentOffset || undefined,
        limit_bytes: chunkSizeRef.current,
        keyword: keyword.trim() || undefined,
        level: level === 'all' ? undefined : level,
        signal: controller.signal,
      });
      if (
        controller.signal.aborted ||
        requestVersionRef.current !== requestVersion
      ) {
        return;
      }
      // 有游标说明是增量块：把新块拼到已显示日志后面。
      // A cursor means this chunk is incremental: append it after the logs already on screen.
      const mergedLogs = (previousLogs?: string) =>
        currentOffset
          ? {
              ...data,
              logs: data.logs
                ? [previousLogs || '', data.logs].filter(Boolean).join('\n')
                : previousLogs || '',
            }
          : data;
      const nextOffset = data.next_offset || currentOffset;
      offsetRef.current = nextOffset;
      chunkSizeRef.current = nextLogChunkSize(
        chunkSizeRef.current,
        data.logs,
        baseChunkSize,
        maxChunkSize,
      );
      setResult((previous) => mergedLogs(previous?.logs));
      setOffset(nextOffset);
    } catch (error) {
      if (
        error instanceof Error &&
        (error.name === 'CanceledError' || error.name === 'AbortError')
      ) {
        return;
      }
      // 自动刷新偶发失败时保留已显示的日志（只在下次从头拉），避免页面闪空；从没拿到过结果时给一个空结果，结束骨架态。
      // A failed auto-refresh keeps the logs on screen (the next poll restarts from the beginning) to avoid
      // flashing empty; if nothing was ever loaded, store an empty result so the skeleton ends.
      setResult(
        (previous) => previous ?? {mode: '', source: '', logs: '', updated_at: ''},
      );
      setOffset('');
      offsetRef.current = '';
      chunkSizeRef.current = baseChunkSize;
      console.warn(
        error instanceof Error ? error.message : 'Failed to load job logs',
      );
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      setLoading(false);
    }
  }, [jobId, enabled, keyword, level, baseChunkSize, maxChunkSize]);

  // 可见时立即加载；关闭时丢弃进行中的请求（不返回数据，与旧实现一致）。
  // Load immediately while visible; drop in-flight requests when hidden (matching the old behavior).
  useEffect(() => {
    if (!jobId || !enabled) {
      abortRef.current?.abort();
      return;
    }
    void load();
  }, [jobId, enabled, load]);

  // enabled 期间每 3s 增量拉一次。
  // Poll for incremental chunks every 3s while enabled.
  useEffect(() => {
    if (!jobId || !enabled) {
      return;
    }
    const timer = window.setInterval(() => {
      void load();
    }, JOB_LOGS_POLL_MS);
    return () => window.clearInterval(timer);
  }, [jobId, enabled, load]);

  // 卸载时丢弃进行中的请求。/ Abort in-flight requests on unmount.
  useEffect(
    () => () => {
      abortRef.current?.abort();
    },
    [],
  );

  const reload = useCallback(() => {
    void load();
  }, [load]);

  return {result, loading, offset, reload, reset};
}
