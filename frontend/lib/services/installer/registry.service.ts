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

// 镜像仓库服务：仓库实体（host + 默认镜像路径 + 凭据 + 连通态）与镜像引用解析/组装辅助。
// Container registry service: manages registry entities (host + default repo +
// credentials + probe status) and parses/composes OCI image references.
import apiClient from '@/lib/services/core/api-client';
import type {ApiResponse} from '@/lib/services/core/types';

export interface ContainerRegistry {
  id: number;
  host: string;
  default_repo?: string;
  username?: string;
  is_system?: boolean;
  has_auth?: boolean;
  probe_scheme?: string;
  probe_status?: string;
  probe_error?: string;
  probe_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ContainerRegistryPayload {
  host: string;
  default_repo?: string;
  username?: string;
  password?: string;
}

export interface ContainerRegistryProbePayload extends ContainerRegistryPayload {
  /** 编辑态复用已存储密文时的仓库 ID / Registry ID when editing and keeping stored password */
  id?: number;
}

export interface ParsedImageRef {
  host: string;
  repo: string;
  tag: string;
}

export async function listContainerRegistries(): Promise<ContainerRegistry[]> {
  const response = await apiClient.get<ApiResponse<{items: ContainerRegistry[]}>>('/packages/registries');
  return response.data?.data?.items || [];
}

export async function createContainerRegistry(payload: ContainerRegistryPayload): Promise<ContainerRegistry> {
  const response = await apiClient.post<ApiResponse<ContainerRegistry>>('/packages/registries', payload);
  return response.data.data;
}

export async function updateContainerRegistry(id: number, payload: ContainerRegistryPayload): Promise<ContainerRegistry> {
  const response = await apiClient.put<ApiResponse<ContainerRegistry>>(`/packages/registries/${id}`, payload);
  return response.data.data;
}

export async function deleteContainerRegistry(id: number): Promise<void> {
  await apiClient.delete<ApiResponse<{success: boolean}>>(`/packages/registries/${id}`);
}

export async function testContainerRegistry(id: number): Promise<ContainerRegistry> {
  const response = await apiClient.post<ApiResponse<ContainerRegistry>>(`/packages/registries/${id}/test`, {});
  return response.data.data;
}

export async function listRegistryTags(id: number, repo: string): Promise<string[]> {
  const response = await apiClient.get<ApiResponse<{items: string[]}>>(`/packages/registries/${id}/tags`, {
    params: {repo},
  });
  return response.data?.data?.items || [];
}

export interface RegistryProbeResult {
  status: string;
  scheme?: string;
  error?: string;
}

// probeRegistry 注册/编辑前不落库测试连通性，对应后端 /registries/probe。
// probeRegistry tests connectivity before saving without persisting.
export async function probeRegistry(payload: ContainerRegistryProbePayload): Promise<RegistryProbeResult> {
  const response = await apiClient.post<ApiResponse<RegistryProbeResult>>('/packages/registries/probe', payload);
  return response.data.data;
}

// extractVersionFromTag 从 tag 提取确定性语义版本：3.1.1 原样、3.1.1-connector-full
// 提取主版本、latest/nightly/digest 等返回空串（调用方据此告警）。
// extractVersionFromTag extracts a deterministic semver from a tag: 3.1.1 passes
// through, 3.1.1-connector-full yields 3.1.1, and latest/nightly/digests yield ''.
export function extractVersionFromTag(tag: string): string {
  const trimmed = (tag || '').trim();
  if (!trimmed || trimmed.startsWith('@')) {
    return '';
  }
  const match = trimmed.match(/^v?(\d+\.\d+(?:\.\d+)?)(?:[-+].*)?$/);
  return match ? match[1] : '';
}

function normalizeDockerHost(rawHost: string): string {
  const lower = rawHost.toLowerCase();
  if (lower === 'index.docker.io' || lower === 'registry-1.docker.io') {
    return 'docker.io';
  }
  return lower;
}

// parseImageRef 按 OCI 规则拆解完整镜像引用为 host / repo / tag（或 @sha256 digest）。
// 遵循「首个 / 前段含 .、: 或为 localhost 才是 registry host，否则归为 docker.io」规范，
// 正确保留带端口私仓（如 192.168.49.4:5000/ns/app:tag）的 host。
// parseImageRef splits a full image reference into host/repo/tag following OCI rules
// (first segment is a host only if it contains '.', ':', or equals 'localhost').
export function parseImageRef(raw: string): ParsedImageRef | null {
  const trimmed = (raw || '').trim();
  if (!trimmed || /\s/.test(trimmed)) {
    return null;
  }

  let refPart = trimmed;
  let tag = '';
  const atIndex = trimmed.indexOf('@');
  if (atIndex >= 0) {
    tag = trimmed.slice(atIndex);
    refPart = trimmed.slice(0, atIndex);
    if (!refPart || !tag || tag === '@') {
      return null;
    }
  } else {
    const lastSlash = refPart.lastIndexOf('/');
    const colonAfterSlash = refPart.indexOf(':', lastSlash + 1);
    if (colonAfterSlash > 0) {
      tag = refPart.slice(colonAfterSlash + 1);
      refPart = refPart.slice(0, colonAfterSlash);
      if (!tag) {
        return null;
      }
    }
  }

  const slashIndex = refPart.indexOf('/');
  if (slashIndex < 0) {
    return {host: 'docker.io', repo: refPart, tag};
  }

  const first = refPart.slice(0, slashIndex);
  const rest = refPart.slice(slashIndex + 1);
  if (!rest) {
    return null;
  }

  if (first.includes('.') || first.includes(':') || first.toLowerCase() === 'localhost') {
    return {host: normalizeDockerHost(first), repo: rest, tag};
  }
  return {host: 'docker.io', repo: refPart, tag};
}

// composeImageRef 三层合成镜像地址：docker.io 省略默认前缀，私仓保留 host/repo；
// 若 tag 以 @ 开头则按 digest 拼接。
// composeImageRef composes an image reference from host, repo, and tag (or @digest).
export function composeImageRef(host: string, repo: string, tag: string): string {
  const cleanHost = (host || '').trim().replace(/\/+$/, '');
  const cleanRepo = (repo || '').trim().replace(/^\/+|\/+$/g, '');
  const cleanTag = (tag || '').trim();
  if (!cleanHost || !cleanRepo || !cleanTag) {
    return '';
  }
  const base = cleanHost === 'docker.io' ? cleanRepo : `${cleanHost}/${cleanRepo}`;
  if (cleanTag.startsWith('@')) {
    return `${base}${cleanTag}`;
  }
  return `${base}:${cleanTag}`;
}
