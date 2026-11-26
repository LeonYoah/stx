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

import apiClient from '../core/api-client';

export interface ContainerImage {
  id: number;
  name: string;
  image: string;
  seatunnel_version: string;
  image_pull_policy: string;
  image_pull_secrets: string;
  is_default: boolean;
  description: string;
  registry_host?: string;
  registry_username?: string;
  has_registry_auth?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface ContainerImageListResult {
  items: ContainerImage[];
  official_presets: ContainerImage[];
}

export interface ContainerImagePayload {
  name: string;
  image: string;
  seatunnel_version?: string;
  image_pull_policy?: string;
  image_pull_secrets?: string;
  is_default?: boolean;
  description?: string;
  registry_host?: string;
  registry_username?: string;
  registry_password?: string;
}

export async function listContainerImages(): Promise<ContainerImageListResult> {
  const response = await apiClient.get<ContainerImageListResult>('/packages/images');
  return response.data;
}

export async function createContainerImage(payload: ContainerImagePayload): Promise<ContainerImage> {
  const response = await apiClient.post<ContainerImage>('/packages/images', payload);
  return response.data;
}

export async function updateContainerImage(id: number, payload: ContainerImagePayload): Promise<ContainerImage> {
  const response = await apiClient.put<ContainerImage>(`/packages/images/${id}`, payload);
  return response.data;
}

export async function deleteContainerImage(id: number): Promise<void> {
  await apiClient.delete(`/packages/images/${id}`);
}

// resolveImageSeatunnelVersion 三级版本识别：注册镜像直返 seatunnel_version；
// 否则从镜像 tag 正则提取 x.y.z；无法识别返回空串。
// resolveImageSeatunnelVersion resolves the version in three tiers: a registered
// image returns its seatunnel_version directly; otherwise the x.y.z tag is
// extracted by regex; unrecognizable references yield an empty string.
export function resolveImageSeatunnelVersion(
  imageRef: string,
  registered?: Pick<ContainerImage, 'image' | 'seatunnel_version'>[] | null,
): string {
  const ref = (imageRef || '').trim();
  if (!ref) {return '';}
  if (registered) {
    for (const item of registered) {
      if (item.image === ref && item.seatunnel_version) {
        return item.seatunnel_version.trim();
      }
    }
  }
  const tagMatch = ref.match(/:(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)$/);
  if (tagMatch) {return tagMatch[1];}
  const digestless = ref.split('@')[0];
  const embedded = digestless.match(/(\d+\.\d+\.\d+)/);
  if (embedded && /seatunnel/i.test(digestless)) {return embedded[1];}
  return '';
}
