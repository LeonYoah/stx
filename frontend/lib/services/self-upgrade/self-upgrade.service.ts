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

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import type {ApiResponse} from '../core/types';
import type {
  CheckResult,
  StageState,
  StatusResponse,
  TaskState,
} from './types';
import {SelfUpgradeApiError} from './types';

export class SelfUpgradeService extends BaseService {
  protected static readonly basePath = '/self-upgrade';

  private static unwrap<T>(
    response: {status: number; data: ApiResponse<T> & {error_code?: string}},
    accepted: number[] = [200, 201, 202],
  ): T {
    const {data, status} = response;
    if (data?.error_msg && !accepted.includes(status)) {
      throw new SelfUpgradeApiError(
        data.error_msg,
        (data as {error_code?: string}).error_code,
        status,
        data.data,
      );
    }
    if (data?.error_msg && status >= 400) {
      throw new SelfUpgradeApiError(
        data.error_msg,
        (data as {error_code?: string}).error_code,
        status,
        data.data,
      );
    }
    return data.data as T;
  }

  static async getStatus(): Promise<StatusResponse> {
    const response = await apiClient.get<ApiResponse<StatusResponse>>(
      this.getFullPath('/status'),
    );
    return this.unwrap(response);
  }

  static async check(payload: {
    simulate_failure?: string;
    force_latest?: string;
  } = {}): Promise<CheckResult> {
    const response = await apiClient.post<ApiResponse<CheckResult>>(
      this.getFullPath('/check'),
      payload,
      {validateStatus: (s) => [200, 502, 503, 409].includes(s)},
    );
    if (response.status >= 400) {
      throw new SelfUpgradeApiError(
        response.data.error_msg || 'check failed',
        (response.data as {error_code?: string}).error_code,
        response.status,
        response.data.data,
      );
    }
    return response.data.data;
  }

  static async startDownload(payload: {
    target_version?: string;
    simulate_failure?: string;
  } = {}): Promise<TaskState> {
    const response = await apiClient.post<ApiResponse<TaskState>>(
      this.getFullPath('/stage/download'),
      payload,
      {validateStatus: (s) => [200, 202, 400, 409, 507].includes(s)},
    );
    if (response.status >= 400) {
      throw new SelfUpgradeApiError(
        response.data.error_msg || 'download failed',
        (response.data as {error_code?: string}).error_code,
        response.status,
      );
    }
    return response.data.data;
  }

  static async importOffline(
    file: File,
    simulateFailure?: string,
  ): Promise<StageState> {
    const form = new FormData();
    form.append('file', file);
    if (simulateFailure) {
      form.append('simulate_failure', simulateFailure);
    }
    const response = await apiClient.post<ApiResponse<StageState>>(
      this.getFullPath('/stage/import'),
      form,
      {
        headers: {'Content-Type': 'multipart/form-data'},
        validateStatus: (s) => [200, 400, 409, 507].includes(s),
      },
    );
    if (response.status >= 400) {
      throw new SelfUpgradeApiError(
        response.data.error_msg || 'import failed',
        (response.data as {error_code?: string}).error_code,
        response.status,
      );
    }
    return response.data.data;
  }

  static async getStage(): Promise<StageState> {
    const response = await apiClient.get<ApiResponse<StageState>>(
      this.getFullPath('/stage'),
    );
    return this.unwrap(response);
  }

  static async cancelStage(): Promise<void> {
    await apiClient.post(this.getFullPath('/stage/cancel'));
  }

  static async execute(payload: {
    simulate_failure?: string;
    apply?: boolean;
  } = {}): Promise<TaskState> {
    const response = await apiClient.post<ApiResponse<TaskState>>(
      this.getFullPath('/execute'),
      payload,
      {validateStatus: (s) => [200, 202, 400, 409].includes(s)},
    );
    if (response.status >= 400) {
      throw new SelfUpgradeApiError(
        response.data.error_msg || 'execute failed',
        (response.data as {error_code?: string}).error_code,
        response.status,
      );
    }
    return response.data.data;
  }

  static async getTask(id: string): Promise<TaskState> {
    const response = await apiClient.get<ApiResponse<TaskState>>(
      this.getFullPath(`/tasks/${id}`),
    );
    return this.unwrap(response);
  }

  static async cancelTask(id: string): Promise<void> {
    await apiClient.post(this.getFullPath(`/tasks/${id}/cancel`));
  }
}
