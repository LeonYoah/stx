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
  MAX_HEAP_MB,
  MIN_HEAP_MB,
  coerceHeapSizeToMB,
  validateHeapSizeMB,
} from '../heap';

describe('coerceHeapSizeToMB', () => {
  it('converts legacy GB values', () => {
    expect(coerceHeapSizeToMB(1)).toBe(1024);
    expect(coerceHeapSizeToMB(2)).toBe(2048);
  });

  it('keeps new MB values', () => {
    expect(coerceHeapSizeToMB(512)).toBe(512);
    expect(coerceHeapSizeToMB(2048)).toBe(2048);
  });
});

describe('validateHeapSizeMB', () => {
  it('accepts 512MB and rejects below min', () => {
    expect(validateHeapSizeMB(512)).toBeNull();
    expect(validateHeapSizeMB(MIN_HEAP_MB - 1)).toBe('below_min');
  });

  it('rejects above max', () => {
    expect(validateHeapSizeMB(MAX_HEAP_MB + 1)).toBe('above_max');
  });

  it('rejects heap that meets or exceeds host memory', () => {
    const hostBytes = 1024 * 1024 * 1024; // 1 GiB
    expect(validateHeapSizeMB(1024, hostBytes)).toBe('exceeds_host_memory');
    expect(validateHeapSizeMB(512, hostBytes)).toBeNull();
  });

  it('treats legacy 2 as 2048MB for host checks', () => {
    const hostBytes = 1024 * 1024 * 1024; // 1 GiB — 2GB legacy would exceed
    expect(validateHeapSizeMB(2, hostBytes)).toBe('exceeds_host_memory');
  });
});
