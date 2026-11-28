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

import {describe, expect, it, vi} from 'vitest';
import type {ToastStackApi} from '@/components/arc/toast-stack/toast-stack';
import {bindToastBridge, toast, unbindToastBridge} from '@/lib/toast';

describe('toast facade', () => {
  it('passes a caller-provided stable id through for deduplication', () => {
    const api: ToastStackApi = {
      toast: vi.fn((options) => options.id ?? 'generated'),
      update: vi.fn(),
      dismiss: vi.fn(),
    };
    bindToastBridge(api);
    try {
      const first = toast.warning('warning', {id: 'same-warning'});
      const second = toast.warning('warning', {id: 'same-warning'});

      expect(first).toBe('same-warning');
      expect(second).toBe('same-warning');
      expect(api.toast).toHaveBeenCalledTimes(2);
      expect(api.toast).toHaveBeenNthCalledWith(
        1,
        expect.objectContaining({id: 'same-warning'}),
      );
      expect(api.toast).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({id: 'same-warning'}),
      );
    } finally {
      unbindToastBridge(api);
    }
  });
});
