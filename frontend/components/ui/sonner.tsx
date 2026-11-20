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

import {useEffect} from 'react';
import {
  ToastStack,
  ToastStackProvider,
  useToastStack,
} from '@/components/arc/toast-stack/toast-stack';
import {bindToastBridge, unbindToastBridge} from '@/lib/toast';

/**
 * 把 `lib/toast` 门面绑定到 Provider 内的 stack api；卸载时解绑。
 * Binds the `lib/toast` facade to the stack api living inside the provider.
 */
function ToastBridge() {
  const api = useToastStack();
  useEffect(() => {
    bindToastBridge(api);
    return () => unbindToastBridge(api);
  }, [api]);
  return null;
}

const Toaster = () => (
  <ToastStackProvider>
    <ToastBridge />
    <ToastStack position='bottom-center' />
  </ToastStackProvider>
);

export {Toaster};
