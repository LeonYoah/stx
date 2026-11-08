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
import {
  DEFAULT_BACKEND_BASE_URL,
  resolveBackendBaseURL,
} from '../backend-base-url';

describe('resolveBackendBaseURL', () => {
  it('prefers BACKEND_BASE_URL over the NEXT_PUBLIC alias', () => {
    expect(
      resolveBackendBaseURL({
        BACKEND_BASE_URL: 'http://stx-backend:17800',
        NEXT_PUBLIC_BACKEND_BASE_URL: 'http://127.0.0.1:17800',
      }),
    ).toBe('http://stx-backend:17800');
  });

  it('falls back to NEXT_PUBLIC_BACKEND_BASE_URL', () => {
    expect(
      resolveBackendBaseURL({
        NEXT_PUBLIC_BACKEND_BASE_URL: 'http://10.74.108.196:17800',
      }),
    ).toBe('http://10.74.108.196:17800');
  });

  it('uses the same-host default when unset', () => {
    expect(resolveBackendBaseURL({})).toBe(DEFAULT_BACKEND_BASE_URL);
  });

  it('trims whitespace and trailing slashes', () => {
    expect(
      resolveBackendBaseURL({
        BACKEND_BASE_URL: '  http://stx-backend:17800/  ',
      }),
    ).toBe('http://stx-backend:17800');
  });
});
