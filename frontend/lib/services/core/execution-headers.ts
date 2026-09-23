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

export interface WebExecutionHeadersOptions {
  idempotencyKey?: string;
  confirmationId?: string;
  confirmed?: boolean;
}

export interface WebExecutionHeaders {
  idempotencyKey: string;
  headers: Record<string, string>;
}

/**
 * 为接入公共执行协议的 Web 写请求生成确认和幂等请求头。
 * Build confirmation and idempotency headers for web writes using the shared execution protocol.
 */
export function createWebExecutionHeaders(
  operation: string,
  options: WebExecutionHeadersOptions = {},
): WebExecutionHeaders {
  const idempotencyKey =
    options.idempotencyKey || createWebIdempotencyKey(operation);
  const headers: Record<string, string> = {
    'Idempotency-Key': idempotencyKey,
  };

  if (options.confirmed !== false) {
    headers['X-STX-Confirm'] = 'true';
  }
  if (options.confirmationId) {
    headers['X-STX-Confirmation-ID'] = options.confirmationId;
  }

  return {idempotencyKey, headers};
}

/**
 * 生成一次 Web 用户动作使用的唯一幂等键，重试同一请求时应复用返回值。
 * Create one unique idempotency key for a web action; retries of that request must reuse it.
 */
function createWebIdempotencyKey(operation: string): string {
  const safeOperation = operation.replace(/[^0-9A-Za-z_-]/g, '-');
  const randomPart =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 12)}`;
  return `web-${safeOperation}-${randomPart}`;
}
