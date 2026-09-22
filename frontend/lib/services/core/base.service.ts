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

import apiClient from './api-client';
import type {AxiosRequestConfig} from 'axios';
import {ApiResponse} from './types';

/**
 * 服务基类
 * 提供通用的HTTP方法封装
 */
export abstract class BaseService {
  /**
   * API基础路径
   */
  protected static basePath = '';

  /**
   * 获取完整的API路径
   * @param path - API路径
   */
  protected static getFullPath(path: string): string {
    return `${this.basePath}${path}`;
  }

  /**
   * GET请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param params - 查询参数
   * @returns 响应数据
   */
  protected static async get<T>(
    path: string,
    params?: Record<string, unknown>,
  ): Promise<T> {
    const response = await apiClient.get<ApiResponse<T>>(
      this.getFullPath(path),
      {params},
    );
    return response.data.data;
  }

  /**
   * POST请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param data - 请求数据
   * @returns 响应数据
   */
  protected static async post<T>(path: string, data?: unknown): Promise<T> {
    const response = await apiClient.post<ApiResponse<T>>(
      this.getFullPath(path),
      data,
    );
    return response.data.data;
  }

  /**
   * 带查询参数的POST请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param data - 请求数据
   * @param params - 查询参数
   * @returns 响应数据
   */
  protected static async postWithParams<T>(
    path: string,
    data?: unknown,
    params?: Record<string, unknown>,
  ): Promise<T> {
    const response = await apiClient.post<ApiResponse<T>>(
      this.getFullPath(path),
      data,
      {params},
    );
    return response.data.data;
  }

  /**
   * 使用完整 Axios 配置发送 POST 请求。
   * Send a POST request with a complete Axios configuration.
   */
  protected static async postWithConfig<T>(
    path: string,
    data?: unknown,
    config?: AxiosRequestConfig,
  ): Promise<T> {
    const response = await apiClient.post<ApiResponse<T>>(
      this.getFullPath(path),
      data,
      config,
    );
    return response.data.data;
  }

  /**
   * PUT请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param data - 请求数据
   * @returns 响应数据
   */
  protected static async put<T>(path: string, data?: unknown): Promise<T> {
    const response = await apiClient.put<ApiResponse<T>>(
      this.getFullPath(path),
      data,
    );
    return response.data.data;
  }

  /**
   * 使用完整 Axios 配置发送 PUT 请求。
   * Send a PUT request with a complete Axios configuration.
   */
  protected static async putWithConfig<T>(
    path: string,
    data?: unknown,
    config?: AxiosRequestConfig,
  ): Promise<T> {
    const response = await apiClient.put<ApiResponse<T>>(
      this.getFullPath(path),
      data,
      config,
    );
    return response.data.data;
  }

  /**
   * 带查询参数的PUT请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param data - 请求数据
   * @param params - 查询参数
   * @returns 响应数据
   */
  protected static async putWithParams<T>(
    path: string,
    data?: unknown,
    params?: Record<string, unknown>,
  ): Promise<T> {
    const response = await apiClient.put<ApiResponse<T>>(
      this.getFullPath(path),
      data,
      {params},
    );
    return response.data.data;
  }

  /**
   * DELETE请求
   * @template T - 响应数据类型
   * @param path - API路径
   * @param params - 查询参数
   * @returns 响应数据
   */
  protected static async delete<T>(
    path: string,
    params?: Record<string, unknown>,
  ): Promise<T> {
    const response = await apiClient.delete<ApiResponse<T>>(
      this.getFullPath(path),
      {params},
    );
    return response.data.data;
  }
}
