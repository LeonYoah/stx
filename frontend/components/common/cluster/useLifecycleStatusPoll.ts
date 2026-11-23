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

import {useCallback, useEffect, useRef} from 'react';
import {
  isLifecyclePollSatisfied,
  LIFECYCLE_POLL_INTERVAL_MS,
  LIFECYCLE_POLL_TIMEOUT_MS,
  type LifecycleBaselinePids,
  type LifecyclePollSnapshot,
  type LifecyclePollTarget,
} from './lifecycle-status-poll';

export interface UseLifecycleStatusPollOptions {
  /** 静默拉取并返回当前主状态快照；失败返回 null 以继续重试。 / Silent refresh; return null on failure to keep retrying. */
  loadSnapshot: () => Promise<LifecyclePollSnapshot | null>;
  intervalMs?: number;
  timeoutMs?: number;
  /** 达标时回调（例如补拉一次 JDK runtime）。 / Called when the target is satisfied (e.g. one JDK runtime refresh). */
  onSatisfied?: (snapshot: LifecyclePollSnapshot) => void;
}

export interface StartLifecyclePollOptions {
  /** 仅等待这些节点达标（单节点/批量）；省略表示全量节点。 / Wait only for these nodes; omit for all nodes. */
  scopeNodeIds?: number[];
  /**
   * 重启前各节点 PID 基准；有值时 running 达标还要求 PID 已变化。
   * Pre-restart PID baselines; when set, running satisfaction also requires PID change.
   */
  baselinePids?: LifecycleBaselinePids;
}

export interface UseLifecycleStatusPollResult {
  startLifecyclePoll: (
    target: LifecyclePollTarget,
    options?: StartLifecyclePollOptions,
  ) => void;
  stopLifecyclePoll: () => void;
}

/**
 * 启停/重启后有界轮询主状态：会话号防止重叠，卸载清理定时器，超时不伪造成功。
 * Bounded main-status poll after lifecycle ops: session id prevents overlap,
 * timers clear on unmount, timeout never fakes success.
 */
export function useLifecycleStatusPoll(
  options: UseLifecycleStatusPollOptions,
): UseLifecycleStatusPollResult {
  const {
    loadSnapshot,
    intervalMs = LIFECYCLE_POLL_INTERVAL_MS,
    timeoutMs = LIFECYCLE_POLL_TIMEOUT_MS,
    onSatisfied,
  } = options;

  const mountedRef = useRef(true);
  const sessionRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const loadSnapshotRef = useRef(loadSnapshot);
  loadSnapshotRef.current = loadSnapshot;
  const onSatisfiedRef = useRef(onSatisfied);
  onSatisfiedRef.current = onSatisfied;
  const intervalRef = useRef(intervalMs);
  intervalRef.current = intervalMs;
  const timeoutRef = useRef(timeoutMs);
  timeoutRef.current = timeoutMs;

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const stopLifecyclePoll = useCallback(() => {
    sessionRef.current += 1;
    clearTimer();
  }, [clearTimer]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopLifecyclePoll();
    };
  }, [stopLifecyclePoll]);

  const startLifecyclePoll = useCallback(
    (target: LifecyclePollTarget, options?: StartLifecyclePollOptions) => {
      // 卸载后拒绝启动，避免 await 刷新返回后再开一轮。
      // Refuse to start after unmount so a late await cannot reopen polling.
      if (!mountedRef.current) {
        return;
      }
      stopLifecyclePoll();
      if (!mountedRef.current) {
        return;
      }
      const session = sessionRef.current;
      const startedAt = Date.now();
      const scopeNodeIds = options?.scopeNodeIds;
      const baselinePids = options?.baselinePids;

      const schedule = (delayMs: number) => {
        if (!mountedRef.current || session !== sessionRef.current) {
          return;
        }
        clearTimer();
        timerRef.current = setTimeout(() => {
          void tick();
        }, delayMs);
      };

      const tick = async () => {
        if (!mountedRef.current || session !== sessionRef.current) {
          return;
        }
        const snapshot = await loadSnapshotRef.current();
        if (!mountedRef.current || session !== sessionRef.current) {
          return;
        }
        if (
          snapshot &&
          isLifecyclePollSatisfied(
            snapshot,
            target,
            scopeNodeIds,
            baselinePids,
          )
        ) {
          clearTimer();
          onSatisfiedRef.current?.(snapshot);
          return;
        }
        if (Date.now() - startedAt >= timeoutRef.current) {
          clearTimer();
          return;
        }
        schedule(intervalRef.current);
      };

      // 操作完成时调用方已拉过一拍；下一拍延后 interval，避免与自激紧挨着的双请求。
      // Caller already refreshed once on complete; delay the next tick to avoid a back-to-back pair.
      schedule(intervalRef.current);
    },
    [clearTimer, stopLifecyclePoll],
  );

  return {startLifecyclePoll, stopLifecyclePoll};
}
