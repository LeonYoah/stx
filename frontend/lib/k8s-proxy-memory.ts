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
 * Proxy 容器内存推导纯函数：必须与后端 internal/pkg/k8s/java_proxy.go 的
 * deriveProxyContainerResources 严格一致，改一侧必须同步另一侧。
 * Pure proxy container memory derivation: must stay strictly in sync with the backend
 * deriveProxyContainerResources in internal/pkg/k8s/java_proxy.go — change both sides together.
 */

function parseJvmFlagMi(jvmOpts: string, flag: string): number {
  let value = 0;
  const lower = flag.toLowerCase();
  for (const piece of jvmOpts.split(/\s+/).filter(Boolean)) {
    const p = piece.trim().toLowerCase();
    if (!p.startsWith(lower)) continue;
    const raw = p.slice(lower.length);
    if (!raw) continue;
    let unit = 1;
    let num = raw;
    const last = raw[raw.length - 1];
    if (last === 'k') {
      unit = 1 / 1024;
      num = raw.slice(0, -1);
    } else if (last === 'm') {
      num = raw.slice(0, -1);
    } else if (last === 'g') {
      unit = 1024;
      num = raw.slice(0, -1);
    }
    const n = Number.parseFloat(num);
    if (!Number.isFinite(n) || n <= 0) continue;
    value = Math.ceil(n * unit);
  }
  return value;
}

function alignUpMi(v: number, align: number): number {
  return v % align === 0 ? v : (Math.floor(v / align) + 1) * align;
}

export interface ProxyContainerMemory {
  xmxMi: number;
  limitMi: number;
  requestMi: number;
  limitDisplay: string;
}

export function formatMemoryMi(mi: number): string {
  return mi % 1024 === 0 ? `${mi / 1024} GiB` : `${mi} MiB`;
}

/**
 * 由 jvm_opts 推导容器内存上限（含堆外余量），与后端公式一致：
 * limits = xmx + max(512, xmx*0.25)，向上 128Mi 对齐；未指定 -Xmx 以 512Mi 为基准。
 * Derives the container memory limit from jvm_opts using the backend formula:
 * limits = xmx + max(512, xmx*0.25) aligned up to 128Mi; a missing -Xmx means a 512Mi baseline.
 */
export function deriveProxyContainerMemory(jvmOpts: string): ProxyContainerMemory {
  let xmxMi = parseJvmFlagMi(jvmOpts, '-xmx');
  if (xmxMi <= 0) xmxMi = 512;
  let xmsMi = parseJvmFlagMi(jvmOpts, '-xms');
  if (xmsMi <= 0) xmsMi = 64;
  if (xmsMi > xmxMi) xmsMi = xmxMi;
  const overheadMi = Math.max(512, Math.ceil(xmxMi * 0.25));
  const limitMi = alignUpMi(xmxMi + overheadMi, 128);
  let requestMi = alignUpMi(Math.max(256, xmsMi + 192), 64);
  if (requestMi > limitMi) requestMi = limitMi;
  return {xmxMi, limitMi, requestMi, limitDisplay: formatMemoryMi(limitMi)};
}
