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

// 镜像仓库服务：仓库实体（host+凭据+探测缓存）是 K8s 部署/升级的唯一镜像来源。
// Container registry service: the registry entity (host + credentials + probe cache)
// is the sole image source for Kubernetes deploys and upgrades.
import apiClient from '@/lib/services/core/api-client';

export interface ContainerRegistry {
  id: number;
  host: string;
  username?: string;
  is_system?: boolean;
  has_auth?: boolean;
  catalog_repos?: string[];
  probe_scheme?: string;
  probe_status?: string;
  probe_error?: string;
  probe_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ContainerRegistryPayload {
  host: string;
  username?: string;
  password?: string;
}

export async function listContainerRegistries(): Promise<ContainerRegistry[]> {
  const response = await apiClient.get<{items: ContainerRegistry[]}>('/packages/registries');
  return response.data.items || [];
}

export async function createContainerRegistry(payload: ContainerRegistryPayload): Promise<ContainerRegistry> {
  const response = await apiClient.post<ContainerRegistry>('/packages/registries', payload);
  return response.data;
}

export async function updateContainerRegistry(id: number, payload: ContainerRegistryPayload): Promise<ContainerRegistry> {
  const response = await apiClient.put<ContainerRegistry>(`/packages/registries/${id}`, payload);
  return response.data;
}

export async function deleteContainerRegistry(id: number): Promise<void> {
  await apiClient.delete(`/packages/registries/${id}`);
}

export async function testContainerRegistry(id: number): Promise<ContainerRegistry> {
  const response = await apiClient.post<ContainerRegistry>(`/packages/registries/${id}/test`, {});
  return response.data;
}

export async function listRegistryTags(id: number, repo: string): Promise<string[]> {
  const response = await apiClient.get<{items: string[]}>(`/packages/registries/${id}/tags`, {
    params: {repo},
  });
  return response.data.items || [];
}

// extractVersionFromTag 从 tag 提取确定性语义版本：3.1.1 原样、3.1.1-connector-full
// 提取主版本、latest/nightly 等返回空串（调用方据此告警）。
// extractVersionFromTag extracts a deterministic semver from a tag: 3.1.1 passes
// through, 3.1.1-connector-full yields the 3.1.1 core, latest/nightly yield an
// empty string so callers can warn.
export function extractVersionFromTag(tag: string): string {
  const trimmed = (tag || '').trim();
  const match = trimmed.match(/^v?(\d+\.\d+(?:\.\d+)?)(?:[-+].*)?$/);
  return match ? match[1] : '';
}

// composeImageRef 三层合成镜像地址：docker.io 特殊路径（repo 预填 apache/seatunnel），
// 私仓一律 host/repo:tag。
// composeImageRef composes the image reference from the three tiers.
export function composeImageRef(host: string, repo: string, tag: string): string {
  const cleanHost = (host || '').trim().replace(/\/+$/, '');
  const cleanRepo = (repo || '').trim().replace(/^\/+|\/+$/g, '');
  const cleanTag = (tag || '').trim();
  if (!cleanHost || !cleanRepo || !cleanTag) {return '';}
  if (cleanHost === 'docker.io') {return `${cleanRepo}:${cleanTag}`;}
  return `${cleanHost}/${cleanRepo}:${cleanTag}`;
}
