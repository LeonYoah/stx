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

import {useEffect, useRef} from 'react';

/**
 * 当 active 从 true 变为 false 时调用一次 refresh。
 * 轮询在任务进入终态后会停止；若最后一次轮询恰好读到中间态（如节点仍显示回滚中），
 * 页面会一直停在该快照上，因此在转为终态时再补拉一次。
 *
 * Calls refresh once when active flips from true to false. Polling stops once the task
 * is terminal; if the last poll caught an intermediate snapshot (e.g. nodes still rolling
 * back), the page would freeze on it, so fetch once more on settle.
 */
export function useRefreshOnSettle(active: boolean, refresh: () => void): void {
  const wasActiveRef = useRef(active);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  useEffect(() => {
    const wasActive = wasActiveRef.current;
    wasActiveRef.current = active;
    if (wasActive && !active) {
      refreshRef.current();
    }
  }, [active]);
}
