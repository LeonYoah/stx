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

import axios, {AxiosError, AxiosResponse} from 'axios';
import {ApiError, ApiResponse} from './types';
import {getCurrentLocale} from '@/lib/i18n/config';
import {translateApiError} from '@/lib/i18n/api-error-text';

/**
 * API客户端实例
 * API client instance
 * 统一处理请求配置、响应解析和错误处理
 * Unified handling of request configuration, response parsing, and error handling
 */
const apiClient = axios.create({
  baseURL: '/api/v1', // API 基础路径 / API base path
  timeout: 60000, // 60 seconds for plugin fetching from Maven / 60秒超时，用于从 Maven 获取插件
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
});

/**
 * 请求拦截器
 * 确保所有请求带上凭证
 */
export function applyRequestLocale(config: {
  headers?: Record<string, unknown>;
}): void {
  const headers = (config.headers ??= {});
  if (headers['Accept-Language'] == null) {
    headers['Accept-Language'] = getCurrentLocale();
  }
}

apiClient.interceptors.request.use(
  (config) => {
    config.withCredentials = true;
    applyRequestLocale(config);
    return config;
  },
  (error) => Promise.reject(error),
);

/**
 * 未授权时跳转到登录页（不强制 OAuth，用户可在登录页选择 admin 或 OAuth）
 * On 401, redirect to login page so user can choose admin or OAuth (fixes: admin-only without OAuth enabled)
 * @param currentPath - 当前路径，登录成功后通过 redirect 参数回到该页
 */
function redirectToLogin(currentPath: string): Promise<never> {
  if (
    !currentPath.startsWith('/login') &&
    !currentPath.startsWith('/callback')
  ) {
    const redirect =
      currentPath === '/' || currentPath === ''
        ? '/login'
        : `/login?redirect=${encodeURIComponent(currentPath)}`;
    window.location.href = redirect;
  }

  return new Promise<never>(() => {});
}

/**
 * 响应拦截器
 * 处理API响应和统一错误处理
 */
apiClient.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => response,
  (error: AxiosError<ApiError>) => {
    // 处理401未授权错误
    // 注意：登录接口 /auth/login 返回 401 表示凭证错误，应直接 reject 显示错误信息，不触发 OAuth 重定向
    const isLoginRequest = error.config?.url?.includes('/auth/login') ?? false;
    if (error.response?.status === 401 && !isLoginRequest) {
      return redirectToLogin(window.location.pathname);
    }

    // 处理后端返回的错误信息
    if (error.response?.data?.error_msg) {
      const apiError = new Error(
        translateApiError(
          error.response.data.error_code,
          error.response.data.error_msg,
        ),
      );
      apiError.name = 'ApiError';
      // 保留结构化错误数据，供需要二次确认的写操作继续提交。
      // Preserve structured error data so confirmed writes can be retried safely.
      Object.assign(apiError, {
        status: error.response.status,
        data: error.response.data.data,
        // 专门处理的失败（如 connector_missing）带 error_code，供页面给出对应指引。
        // Failures with dedicated handling (e.g. connector_missing) carry error_code so pages can show matching guidance.
        errorCode: error.response.data.error_code,
      });
      return Promise.reject(apiError);
    }

    // 处理网络错误
    if (error.code === 'ECONNABORTED') {
      return Promise.reject(
        new Error(translateApiError('network.timeout', '网络请求失败')),
      );
    }

    // 处理权限错误
    if (error.response?.status === 403) {
      return Promise.reject(
        new Error(translateApiError('network.forbidden', '权限不足')),
      );
    }

    // 处理请求体过大（常见于网关上传限制）
    if (error.response?.status === 413) {
      return Promise.reject(
        new Error(
          translateApiError(
            'network.payloadTooLarge',
            '上传文件过大或被网关限制，请使用更小文件或改用服务器下载',
          ),
        ),
      );
    }

    // 处理服务器错误
    if (error.response && error.response.status >= 500) {
      return Promise.reject(
        new Error(translateApiError('network.serverError', '服务器内部错误，请稍后重试')),
      );
    }

    return Promise.reject(
      new Error(
        error.message || translateApiError('network.failed', '网络请求失败'),
      ),
    );
  },
);

export default apiClient;
