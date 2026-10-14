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
import {isLocale, saveLocale} from '@/lib/i18n/config';
import {
  CallbackRequest,
  LoginRequest,
  LoginResponseData,
  UpdateProfileRequest,
  UserInfoResponse,
} from './types';
import {setSystemTimezone, setUserTimezonePreference} from '@/lib/timezone';

function syncUserLocale(
  user?: {language?: string; timezone?: string; system_timezone?: string} | null,
): void {
  if (user?.language && isLocale(user.language)) {
    saveLocale(user.language);
  }
  if (user && typeof user.timezone === 'string') {
    setUserTimezonePreference(user.timezone);
  }
  if (user?.system_timezone) {
    setSystemTimezone(user.system_timezone);
  }
}

/**
 * 认证服务
 * 处理用户名密码登录和OAuth登录相关操作
 * 默认使用用户名密码登录方式
 */
export class AuthService extends BaseService {
  /**
   * API基础路径（用户名密码认证）
   */
  protected static readonly basePath = '/auth';

  /**
   * OAuth认证API基础路径（保留用于OAuth登录）
   * 注意：OAuth路径不使用basePath，需要完整路径
   */
  protected static readonly oauthBasePath = '/oauth';

  // ==================== 用户名密码登录方法（默认方式） ====================

  /**
   * 使用用户名密码登录
   * @param credentials - 登录凭证（用户名和密码）
   * @returns 登录响应数据
   */
  static async loginWithCredentials(
    credentials: LoginRequest,
  ): Promise<LoginResponseData> {
    const data = await this.post<LoginResponseData>('/login', credentials);
    syncUserLocale(data);
    return data;
  }

  /**
   * 执行用户名密码登录流程（默认登录方式）
   * @param credentials - 登录凭证
   * @param redirectTo - 登录成功后重定向的页面路径
   */
  static async login(
    credentials: LoginRequest,
    redirectTo = '/dashboard',
  ): Promise<void> {
    try {
      await this.loginWithCredentials(credentials);
      window.location.href = redirectTo;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '登录失败';
      console.error(errorMessage);
      throw new Error(errorMessage);
    }
  }

  /**
   * 获取当前用户信息
   * @returns 用户基本信息
   */
  static async getUserInfo(): Promise<UserInfoResponse['data']> {
    const data = await this.get<UserInfoResponse['data']>('/user-info');
    syncUserLocale(data);
    return data;
  }

  /**
   * 更新当前登录用户个人信息（邮箱、语言偏好）。
   * @param payload - 个人信息更新请求
   * @returns 更新后的用户信息
   */
  static async updateProfile(
    payload: UpdateProfileRequest,
  ): Promise<UserInfoResponse['data']> {
    const data = await this.put<UserInfoResponse['data']>('/profile', payload);
    syncUserLocale(data);
    return data;
  }

  /**
   * API登出请求
   */
  static async callLogoutAPI(): Promise<void> {
    return this.post('/logout', {});
  }

  /**
   * 执行完整的登出流程
   * @param redirectTo - 登出后重定向的页面路径
   */
  static async logout(redirectTo = '/'): Promise<void> {
    try {
      // 清除会话存储
      sessionStorage.removeItem('oauth_redirect_to');

      // 调用后端API
      await this.callLogoutAPI();

      // 添加登出标记
      const finalRedirect =
        redirectTo === '/login' ? '/login?logout=true' : redirectTo;

      // 重定向
      window.location.href = finalRedirect;
    } catch (error) {
      console.error(
        '登出失败:',
        error instanceof Error ? error.message : '未知错误',
      );

      // 出错时仍然重定向
      const finalRedirect =
        redirectTo === '/login' ? '/login?logout=true' : redirectTo;

      window.location.href = finalRedirect;
    }
  }

  /**
   * 检查用户是否已登录
   * @returns 用户认证状态
   */
  static async checkAuthStatus(): Promise<boolean> {
    try {
      await this.getUserInfo();
      return true;
    } catch {
      return false;
    }
  }

  // ==================== OAuth登录方法（GitHub、Google） ====================

  /**
   * 获取启用的 OAuth 提供商列表
   * @returns 启用的提供商名称数组（如 ['github', 'google']）
   */
  static async getEnabledOAuthProviders(): Promise<string[]> {
    const {default: apiClient} = await import('../core/api-client');
    const response = await apiClient.get<{data: string[]}>(
      `${this.oauthBasePath}/providers`,
    );
    return response.data.data || [];
  }

  /**
   * 获取OAuth登录URL
   * 注意：OAuth使用不同的基础路径，需要绕过basePath
   * @param provider - OAuth提供商（如 'github', 'google'）
   * @returns 登录授权URL
   */
  static async getOAuthLoginURL(provider?: string): Promise<string> {
    const path = provider
      ? `${this.oauthBasePath}/login?provider=${provider}`
      : `${this.oauthBasePath}/login`;
    // 直接使用完整路径，不经过getFullPath
    const {default: apiClient} = await import('../core/api-client');
    const response = await apiClient.get<{data: string}>(path);
    return response.data.data;
  }

  /**
   * 处理OAuth回调
   * @param params - 包含state和code的回调参数
   */
  static async handleOAuthCallback(params: CallbackRequest): Promise<void> {
    // 直接使用完整路径，不经过getFullPath
    const {default: apiClient} = await import('../core/api-client');
    await apiClient.post(`${this.oauthBasePath}/callback`, params);
  }

  /**
   * 执行OAuth登录流程
   * @param provider - OAuth提供商（如 'github', 'google'）
   * @param redirectTo - 登录成功后重定向的页面路径
   */
  static async loginWithOAuth(
    provider?: string,
    redirectTo?: string,
  ): Promise<void> {
    try {
      // 保存重定向信息到sessionStorage
      if (redirectTo) {
        sessionStorage.setItem('oauth_redirect_to', redirectTo);
      } else {
        sessionStorage.removeItem('oauth_redirect_to');
      }

      const loginURL = await this.getOAuthLoginURL(provider);
      window.location.href = loginURL;
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '获取登录URL失败';
      console.error(errorMessage);
      throw new Error(errorMessage);
    }
  }

  // ==================== 兼容性方法（保留旧API） ====================

  /**
   * 获取OAuth登录URL（兼容旧API）
   * @deprecated 请使用 getOAuthLoginURL
   * @returns 登录授权URL
   */
  static async getLoginURL(): Promise<string> {
    return this.getOAuthLoginURL();
  }

  /**
   * 处理OAuth回调（兼容旧API）
   * @deprecated 请使用 handleOAuthCallback
   * @param params - 包含state和code的回调参数
   */
  static async handleCallback(params: CallbackRequest): Promise<void> {
    return this.handleOAuthCallback(params);
  }
}
