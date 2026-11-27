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

// 镜像仓库服务测试：覆盖统一 ApiResponse 解包及 OCI parseImageRef / composeImageRef 边界。
// Registry service tests: verify ApiResponse unwrapping and OCI parseImageRef / composeImageRef edge cases.

import {beforeEach, describe, expect, it, vi} from 'vitest';
import {
  composeImageRef,
  createContainerRegistry,
  deleteContainerRegistry,
  extractVersionFromTag,
  listContainerRegistries,
  listRegistryTags,
  parseImageRef,
  probeRegistry,
  testContainerRegistry,
  updateContainerRegistry,
} from '../registry.service';

const apiMocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}));

vi.mock('@/lib/services/core/api-client', () => ({
  default: {
    get: apiMocks.get,
    post: apiMocks.post,
    put: apiMocks.put,
    delete: apiMocks.delete,
  },
}));

describe('registry.service', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('unwraps {error_msg, data} envelope across all CRUD and probe calls', async () => {
    const sample = {id: 2, host: 'harbor.corp.local', has_auth: true, probe_status: 'ok'};
    apiMocks.get.mockResolvedValueOnce({data: {error_msg: '', data: {items: [sample]}}});
    expect(await listContainerRegistries()).toEqual([sample]);

    apiMocks.post.mockResolvedValueOnce({data: {error_msg: '', data: sample}});
    expect(await createContainerRegistry({host: 'harbor.corp.local'})).toEqual(sample);

    apiMocks.put.mockResolvedValueOnce({data: {error_msg: '', data: sample}});
    expect(await updateContainerRegistry(2, {host: 'harbor.corp.local', username: 'u'})).toEqual(sample);

    apiMocks.post.mockResolvedValueOnce({data: {error_msg: '', data: sample}});
    expect(await testContainerRegistry(2)).toEqual(sample);

    apiMocks.get.mockResolvedValueOnce({data: {error_msg: '', data: {items: ['3.1.1', '3.1.0']}}});
    expect(await listRegistryTags(2, 'data/seatunnel')).toEqual(['3.1.1', '3.1.0']);

    apiMocks.post.mockResolvedValueOnce({data: {error_msg: '', data: {status: 'ok', repos: ['data/seatunnel']}}});
    expect(await probeRegistry({id: 2, host: 'harbor.corp.local', username: 'u', password: ''})).toEqual({
      status: 'ok',
      repos: ['data/seatunnel'],
    });
    expect(apiMocks.post).toHaveBeenLastCalledWith('/packages/registries/probe', {
      id: 2,
      host: 'harbor.corp.local',
      username: 'u',
      password: '',
    });

    apiMocks.delete.mockResolvedValueOnce({data: {error_msg: '', data: {success: true}}});
    await deleteContainerRegistry(2);
    expect(apiMocks.delete).toHaveBeenCalledWith('/packages/registries/2');
  });

  it('parses OCI image references cleanly including ports, localhost, docker aliases, and digests', () => {
    expect(parseImageRef('apache/seatunnel:3.1.1')).toEqual({
      host: 'docker.io',
      repo: 'apache/seatunnel',
      tag: '3.1.1',
    });
    expect(parseImageRef('docker.io/apache/seatunnel:3.1.1')).toEqual({
      host: 'docker.io',
      repo: 'apache/seatunnel',
      tag: '3.1.1',
    });
    expect(parseImageRef('index.docker.io/apache/seatunnel:3.1.1')).toEqual({
      host: 'docker.io',
      repo: 'apache/seatunnel',
      tag: '3.1.1',
    });
    expect(parseImageRef('harbor.corp.local/data/seatunnel:3.1.0-custom')).toEqual({
      host: 'harbor.corp.local',
      repo: 'data/seatunnel',
      tag: '3.1.0-custom',
    });
    expect(parseImageRef('192.168.49.4:5000/seatunnel:3.1.1')).toEqual({
      host: '192.168.49.4:5000',
      repo: 'seatunnel',
      tag: '3.1.1',
    });
    expect(parseImageRef('localhost:5000/ns/st:latest')).toEqual({
      host: 'localhost:5000',
      repo: 'ns/st',
      tag: 'latest',
    });
    expect(parseImageRef('harbor.corp.local/ns/st@sha256:abcdef')).toEqual({
      host: 'harbor.corp.local',
      repo: 'ns/st',
      tag: '@sha256:abcdef',
    });
    expect(parseImageRef('seatunnel:3.1.1')).toEqual({
      host: 'docker.io',
      repo: 'seatunnel',
      tag: '3.1.1',
    });
    expect(parseImageRef('apache/seatunnel')).toEqual({
      host: 'docker.io',
      repo: 'apache/seatunnel',
      tag: '',
    });
    expect(parseImageRef('')).toBeNull();
    expect(parseImageRef('bad ref:1.0')).toBeNull();
    expect(parseImageRef('harbor.corp.local/app:')).toBeNull();
  });

  it('composes image references and extracts semantic versions safely', () => {
    expect(composeImageRef('docker.io', 'apache/seatunnel', '3.1.1')).toBe('apache/seatunnel:3.1.1');
    expect(composeImageRef('192.168.49.4:5000', 'seatunnel', '3.1.1')).toBe('192.168.49.4:5000/seatunnel:3.1.1');
    expect(composeImageRef('harbor.corp.local', 'ns/st', '@sha256:abcdef')).toBe('harbor.corp.local/ns/st@sha256:abcdef');
    expect(extractVersionFromTag('3.1.1')).toBe('3.1.1');
    expect(extractVersionFromTag('v3.2.0-connector')).toBe('3.2.0');
    expect(extractVersionFromTag('latest')).toBe('');
    expect(extractVersionFromTag('@sha256:abcdef')).toBe('');
  });
});
