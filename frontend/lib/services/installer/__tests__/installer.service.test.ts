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

import {afterEach, describe, expect, it, vi} from 'vitest';
import apiClient from '../../core/api-client';
import {
  cancelDownload,
  deletePackage,
  fetchSourcePackage,
  uploadPackage,
  uploadSourcePackage,
} from '../installer.service';
import type {DownloadTask, PackageInfo} from '../types';

const packageInfo: PackageInfo = {
  version: '2.3.13',
  file_name: 'apache-seatunnel-2.3.13-bin.tar.gz',
  file_size: 1024,
  download_urls: {
    apache: '',
    aliyun: '',
    huaweicloud: '',
  },
  is_local: true,
  has_source: true,
  source_download_urls: {
    apache: '',
    aliyun: '',
    huaweicloud: '',
  },
};

const downloadTask: DownloadTask = {
  id: 'download-1',
  version: '2.3.13',
  mirror: 'apache',
  download_url: '',
  status: 'cancelled',
  progress: 10,
  downloaded_bytes: 100,
  total_bytes: 1000,
  speed: 0,
  start_time: '2026-09-20T00:00:00Z',
  source_requested: true,
};

function expectR1Headers(headers: unknown): void {
  expect(headers).toEqual(
    expect.objectContaining({
      'Idempotency-Key': expect.stringMatching(/^web-/),
      'X-STX-Confirm': 'true',
    }),
  );
}

describe('installer service shared execution headers', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('联网补充源码时发送幂等键和显式确认', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: {error_msg: '', data: packageInfo},
    });

    await expect(fetchSourcePackage('2.3.13', 'apache')).resolves.toEqual(
      packageInfo,
    );

    const [, payload, config] = post.mock.calls[0];
    expect(payload).toEqual({mirror: 'apache'});
    expectR1Headers(config?.headers);
    expect(config?.timeout).toBe(10 * 60 * 1000);
  });

  it('上传源码和取消下载时发送公共执行请求头', async () => {
    const post = vi
      .spyOn(apiClient, 'post')
      .mockResolvedValueOnce({data: {error_msg: '', data: packageInfo}})
      .mockResolvedValueOnce({data: {error_msg: '', data: downloadTask}});

    await uploadSourcePackage(
      '2.3.13',
      new File(['source'], 'apache-seatunnel-2.3.13-src.tar.gz'),
    );
    await cancelDownload('2.3.13');

    expectR1Headers(post.mock.calls[0][2]?.headers);
    expect(post.mock.calls[0][2]?.headers).toEqual(
      expect.objectContaining({'Content-Type': 'multipart/form-data'}),
    );
    expectR1Headers(post.mock.calls[1][2]?.headers);
  });

  it('删除安装包时发送公共执行请求头', async () => {
    const remove = vi.spyOn(apiClient, 'delete').mockResolvedValue({
      data: {error_msg: '', data: null},
    });

    await deletePackage('2.3.13');

    expectR1Headers(remove.mock.calls[0][1]?.headers);
  });

  it('每个上传分片使用稳定且互不相同的幂等键', async () => {
    const post = vi
      .spyOn(apiClient, 'post')
      .mockResolvedValueOnce({
        data: {
          error_msg: '',
          data: {
            completed: false,
            received_chunks: 1,
            total_chunks: 2,
          },
        },
      })
      .mockResolvedValueOnce({
        data: {
          error_msg: '',
          data: {
            completed: true,
            received_chunks: 2,
            total_chunks: 2,
            package: packageInfo,
          },
        },
      });
    const content = new Uint8Array(8 * 1024 * 1024 + 1);

    await uploadPackage(
      new File([content], 'apache-seatunnel-2.3.13-bin.tar.gz'),
      '2.3.13',
    );

    const firstHeaders = post.mock.calls[0][2]?.headers as Record<
      string,
      string
    >;
    const secondHeaders = post.mock.calls[1][2]?.headers as Record<
      string,
      string
    >;
    expectR1Headers(firstHeaders);
    expectR1Headers(secondHeaders);
    expect(firstHeaders['Idempotency-Key']).toMatch(/-0$/);
    expect(secondHeaders['Idempotency-Key']).toMatch(/-1$/);
    expect(firstHeaders['Idempotency-Key']).not.toBe(
      secondHeaders['Idempotency-Key'],
    );
  });
});
