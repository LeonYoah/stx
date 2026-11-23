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

/** 历史按 GB 存储的上限（1–128）；更大值按 MB 解释。/ Legacy GB-storage ceiling (1–128); larger values mean MB. */
export const LEGACY_HEAP_GB_MAX = 128;
/** 最小堆（MB）。/ Minimum heap in MB. */
export const MIN_HEAP_MB = 512;
/** 最大堆（MB，128 GiB）。/ Maximum heap in MB (128 GiB). */
export const MAX_HEAP_MB = 128 * 1024;

/**
 * 把历史 GB 或新 MB 堆值规范为 MB。仅用于回填历史值或后端存量数据，勿对用户已按 MB 输入的值调用。
 * Normalize a legacy-GB or new-MB heap value to MB. Use only when hydrating legacy/backend values — not on user MB input.
 */
export function coerceHeapSizeToMB(value: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return value;
  }
  if (value <= LEGACY_HEAP_GB_MAX) {
    return value * 1024;
  }
  return value;
}

export type HeapValidationCode =
  | 'below_min'
  | 'above_max'
  | 'exceeds_host_memory';

/**
 * 校验已是 MB 的堆大小；hostMemoryBytes 有值时不得 ≥ 主机总内存。
 * Validate a heap size that is already in MB; when hostMemoryBytes is set it must stay below host RAM.
 */
export function validateHeapSizeMB(
  value: number,
  hostMemoryBytes?: number,
): HeapValidationCode | null {
  if (!Number.isFinite(value) || value < MIN_HEAP_MB) {
    return 'below_min';
  }
  if (value > MAX_HEAP_MB) {
    return 'above_max';
  }
  if (
    typeof hostMemoryBytes === 'number' &&
    hostMemoryBytes > 0 &&
    value >= Math.floor(hostMemoryBytes / (1024 * 1024))
  ) {
    return 'exceeds_host_memory';
  }
  return null;
}

/** 字段级错误文案键（cluster.*）。/ Field-level message keys under cluster.*. */
export function heapFieldErrorKey(
  code: HeapValidationCode,
):
  | 'cluster.jvmHeapSizeRequired'
  | 'cluster.jvmHeapSizeInvalid'
  | 'cluster.jvmHeapSizeExceedsHost' {
  switch (code) {
    case 'below_min':
      return 'cluster.jvmHeapSizeRequired';
    case 'above_max':
      return 'cluster.jvmHeapSizeInvalid';
    case 'exceeds_host_memory':
      return 'cluster.jvmHeapSizeExceedsHost';
  }
}

/** 向导「下一步」禁用原因文案键（cluster.wizard.*）。/ Wizard next-button block reason keys under cluster.wizard.*. */
export function heapNextDisabledKey(
  code: HeapValidationCode,
):
  | 'cluster.wizard.nextDisabledHeapBelowMin'
  | 'cluster.wizard.nextDisabledHeapAboveMax'
  | 'cluster.wizard.nextDisabledHeapExceedsHost' {
  switch (code) {
    case 'below_min':
      return 'cluster.wizard.nextDisabledHeapBelowMin';
    case 'above_max':
      return 'cluster.wizard.nextDisabledHeapAboveMax';
    case 'exceeds_host_memory':
      return 'cluster.wizard.nextDisabledHeapExceedsHost';
  }
}
