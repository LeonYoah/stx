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

import {describe, expect, it} from 'vitest';
import {createWebExecutionHeaders} from '../execution-headers';

describe('createWebExecutionHeaders', () => {
  it('默认生成幂等键并发送显式确认', () => {
    const result = createWebExecutionHeaders('package.source.fetch');

    expect(result.idempotencyKey).toMatch(/^web-package-source-fetch-/);
    expect(result.headers).toEqual({
      'Idempotency-Key': result.idempotencyKey,
      'X-STX-Confirm': 'true',
    });
  });

  it('重试时复用幂等键并携带一次性确认编号', () => {
    const result = createWebExecutionHeaders('stupgrade.plan.execute', {
      idempotencyKey: 'web-upgrade-fixed-key',
      confirmationId: 'confirmation-1',
    });

    expect(result).toEqual({
      idempotencyKey: 'web-upgrade-fixed-key',
      headers: {
        'Idempotency-Key': 'web-upgrade-fixed-key',
        'X-STX-Confirm': 'true',
        'X-STX-Confirmation-ID': 'confirmation-1',
      },
    });
  });

  it('允许调用方关闭默认显式确认', () => {
    const result = createWebExecutionHeaders('diagnostics.task.create', {
      idempotencyKey: 'web-diagnostics-fixed-key',
      confirmed: false,
    });

    expect(result.headers).toEqual({
      'Idempotency-Key': 'web-diagnostics-fixed-key',
    });
  });
});
