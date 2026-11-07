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

import {usePathname, useRouter} from 'next/navigation';
import {useCallback, useEffect, useRef, useState} from 'react';
import {isLeavingWorkbenchPath} from '@/components/common/sync/sync-studio-utils';

/**
 * 工作台离开拦截：有未落盘风险时挂 beforeunload，并捕获站内离开 /workbench 的链接点击。
 * Workbench leave guard: attach beforeunload when risk exists; capture in-app links leaving /workbench.
 */
export function useWorkbenchLeaveGuard(options: {
  enabled: boolean;
  hasUnsavedRisk: boolean;
}) {
  const {enabled, hasUnsavedRisk} = options;
  const pathname = usePathname();
  const router = useRouter();
  const [leaveDialogOpen, setLeaveDialogOpen] = useState(false);
  const pendingHrefRef = useRef<string | null>(null);
  const bypassRef = useRef(false);

  const shouldGuard = enabled && hasUnsavedRisk;

  // 仅在有风险时挂载 beforeunload，避免无意义打扰。
  // Attach beforeunload only while unsaved risk exists.
  useEffect(() => {
    if (!shouldGuard) {
      return;
    }
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [shouldGuard]);

  // 捕获 Dock / 站内 Link 点击，离开工作台前弹出确认。
  // Capture Dock / in-app Link clicks and confirm before leaving workbench.
  useEffect(() => {
    if (!shouldGuard) {
      return;
    }
    const onClick = (event: MouseEvent) => {
      if (bypassRef.current) {
        return;
      }
      if (event.defaultPrevented) {
        return;
      }
      if (event.button !== 0) {
        return;
      }
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
      }
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }
      const anchor = target.closest('a[href]');
      if (!(anchor instanceof HTMLAnchorElement)) {
        return;
      }
      if (anchor.target === '_blank' || anchor.hasAttribute('download')) {
        return;
      }
      const href = anchor.getAttribute('href');
      if (!href || !isLeavingWorkbenchPath(href, pathname || '/workbench')) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      pendingHrefRef.current = href;
      setLeaveDialogOpen(true);
    };
    document.addEventListener('click', onClick, true);
    return () => document.removeEventListener('click', onClick, true);
  }, [pathname, shouldGuard]);

  const clearPending = useCallback(() => {
    pendingHrefRef.current = null;
  }, []);

  const stay = useCallback(() => {
    clearPending();
    setLeaveDialogOpen(false);
  }, [clearPending]);

  const proceedToPending = useCallback(() => {
    const href = pendingHrefRef.current;
    clearPending();
    setLeaveDialogOpen(false);
    if (!href) {
      return;
    }
    bypassRef.current = true;
    try {
      router.push(href);
    } finally {
      // 下一帧恢复，避免同一次事件冒泡再拦。
      // Restore next frame so the same event bubble is not re-blocked.
      window.setTimeout(() => {
        bypassRef.current = false;
      }, 0);
    }
  }, [clearPending, router]);

  const requestLeave = useCallback(
    (href: string) => {
      if (!shouldGuard) {
        router.push(href);
        return;
      }
      pendingHrefRef.current = href;
      setLeaveDialogOpen(true);
    },
    [router, shouldGuard],
  );

  return {
    leaveDialogOpen,
    setLeaveDialogOpen,
    stay,
    proceedToPending,
    requestLeave,
    clearPending,
  };
}
