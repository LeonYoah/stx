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

/**
 * sonner API 兼容的 toast 门面：底层由 Arc `toast-stack` 渲染。
 * 桥接组件挂载前的调用先排队，挂载后按序回放，保证事件处理器中随时可调用。
 * sonner-compatible toast facade backed by the Arc `toast-stack`.
 * Calls made before the bridge mounts are queued and replayed in order, so
 * event handlers may call `toast.*` at any time.
 */
'use client';

import type {ToastStackApi, ToastType} from '@/components/arc/toast-stack/toast-stack';

export interface ToastCompatOptions {
  id?: string;
  description?: string;
  duration?: number;
}

type Bridge = ToastStackApi | null;

let bridge: Bridge = null;
const pending: Array<(api: ToastStackApi) => void> = [];

function enqueue(fn: (api: ToastStackApi) => void) {
  if (bridge) {
    fn(bridge);
  } else {
    pending.push(fn);
  }
}

let seq = 0;

function show(type: ToastType, title: string, opts?: ToastCompatOptions): string {
  const id = opts?.id ?? `arc-toast-${++seq}`;
  enqueue((api) =>
    api.toast({
      id,
      type,
      title,
      description: opts?.description,
      duration: opts?.duration,
    }),
  );
  return id;
}

export const toast = {
  success: (title: string, opts?: ToastCompatOptions) =>
    show('success', title, opts),
  error: (title: string, opts?: ToastCompatOptions) => show('error', title, opts),
  warning: (title: string, opts?: ToastCompatOptions) =>
    show('warning', title, opts),
  info: (title: string, opts?: ToastCompatOptions) => show('info', title, opts),
  message: (title: string, opts?: ToastCompatOptions) =>
    show('info', title, opts),
  loading: (title: string, opts?: ToastCompatOptions) =>
    show('loading', title, opts),
  dismiss: (id?: string | number) =>
    enqueue((api) => api.dismiss(id === undefined ? undefined : String(id))),
};

/** 由 Toaster 内部的桥接组件调用；外部代码不应直接使用。/ Internal: wired by the toaster bridge. */
export function bindToastBridge(api: ToastStackApi) {
  bridge = api;
  const queued = pending.splice(0);
  for (const fn of queued) {
    fn(api);
  }
}

/** 卸载时解绑，避免桥接失效后仍写进旧队列。/ Internal: release the bridge on unmount. */
export function unbindToastBridge(api: ToastStackApi) {
  if (bridge === api) {
    bridge = null;
  }
}
