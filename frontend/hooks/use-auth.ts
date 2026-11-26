/**
 * Copyright 2024 Apache Software Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {useState, useEffect, useCallback, useRef} from 'react';
import services from '@/lib/services';
import {BasicUserInfo} from '@/lib/services/core';

/**
 * 认证状态
 */
interface AuthState {
  /** 用户是否已认证 */
  isAuthenticated: boolean;
  /** 用户信息 */
  user: BasicUserInfo | null;
  /** 认证过程加载状态 */
  isLoading: boolean;
  /** 错误信息 */
  error: string | null;
}

/**
 * 认证Hook返回值
 */
interface UseAuthReturn extends AuthState {
  /** 使用用户名密码登录（默认方式） */
  loginWithCredentials: (
    username: string,
    password: string,
    redirectTo?: string,
  ) => Promise<void>;
  /** 使用 OAuth 登录（备选方式） */
  loginWithOAuth: (provider?: string, redirectTo?: string) => Promise<void>;
  /** 执行登出 */
  logout: (redirectTo?: string) => Promise<void>;
  /** 清除错误 */
  clearError: () => void;
  /** 检查认证状态 */
  checkAuthStatus: (forceRefresh?: boolean) => Promise<void>;
  /** @deprecated 使用 loginWithOAuth 代替 */
  login: (redirectTo?: string) => Promise<void>;
}

/** 缓存过期时间（毫秒） */
const CACHE_EXPIRY = 30000;

/** 用户信息缓存 */
const userInfoCache: {
  data: BasicUserInfo | null;
  timestamp: number;
  promise: Promise<BasicUserInfo> | null;
} = {
  data: null,
  timestamp: 0,
  promise: null,
};

/**
 * 用户认证状态管理Hook
 * 支持用户名密码登录（默认）和 OAuth 登录（备选）
 */
export function useAuth(): UseAuthReturn {
  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    user: null,
    isLoading: true,
    error: null,
  });

  const isMounted = useRef(true);

  /**
   * 检查用户认证状态
   * 实现请求缓存、防抖和请求合并
   */
  const checkAuthStatus = useCallback(async (forceRefresh = false) => {
    try {
      if (!isMounted.current) {return;}

      if (!state.isLoading) {
        setState((prev) => ({...prev, isLoading: true, error: null}));
      }

      if (forceRefresh) {
        userInfoCache.data = null;
        userInfoCache.timestamp = 0;
        userInfoCache.promise = null;
      }

      // 使用缓存数据（如果缓存有效）
      const now = Date.now();
      if (
        !forceRefresh &&
        userInfoCache.data &&
        now - userInfoCache.timestamp < CACHE_EXPIRY
      ) {
        if (isMounted.current) {
          setState({
            isAuthenticated: true,
            user: userInfoCache.data,
            isLoading: false,
            error: null,
          });
        }
        return;
      }

      // 使用进行中的请求（请求合并）
      if (!forceRefresh && userInfoCache.promise) {
        try {
          const userInfo = await userInfoCache.promise;
          if (isMounted.current) {
            setState({
              isAuthenticated: true,
              user: userInfo,
              isLoading: false,
              error: null,
            });
          }
        } catch (error) {
          if (isMounted.current) {
            setState({
              isAuthenticated: false,
              user: null,
              isLoading: false,
              error:
                error instanceof Error ? error.message : '获取用户信息失败',
            });
          }
        } finally {
          userInfoCache.promise = null;
        }
        return;
      }

      // 创建新请求并缓存Promise
      userInfoCache.promise = services.auth.getUserInfo();

      try {
        const userInfo = await userInfoCache.promise;

        // 更新缓存
        userInfoCache.data = userInfo;
        userInfoCache.timestamp = Date.now();

        if (isMounted.current) {
          setState({
            isAuthenticated: true,
            user: userInfo,
            isLoading: false,
            error: null,
          });
        }
      } catch (error) {
        // 清除缓存
        userInfoCache.data = null;
        userInfoCache.timestamp = 0;

        if (isMounted.current) {
          setState({
            isAuthenticated: false,
            user: null,
            isLoading: false,
            error: error instanceof Error ? error.message : '获取用户信息失败',
          });
        }
      } finally {
        // 重置请求状态
        userInfoCache.promise = null;
      }
    } catch (error) {
      userInfoCache.promise = null;

      if (isMounted.current) {
        setState({
          isAuthenticated: false,
          user: null,
          isLoading: false,
          error: error instanceof Error ? error.message : '认证状态检查失败',
        });
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // 避免循环依赖导致的重复请求

  /**
   * 使用用户名密码登录（默认登录方式）
   * @param username - 用户名
   * @param password - 密码
   * @param redirectTo - 登录成功后重定向的URL
   */
  const loginWithCredentials = useCallback(
    async (username: string, password: string, redirectTo?: string) => {
      try {
        if (isMounted.current) {
          setState((prev) => ({...prev, isLoading: true, error: null}));
        }
        await services.auth.login({username, password}, redirectTo);
      } catch (error) {
        if (isMounted.current) {
          setState((prev) => ({
            ...prev,
            isLoading: false,
            error: error instanceof Error ? error.message : '登录失败',
          }));
        }
        throw error;
      }
    },
    [],
  );

  /**
   * 使用 OAuth 登录（备选登录方式）
   * @param provider - OAuth 提供商（如 'github', 'google'）
   * @param redirectTo - 登录成功后重定向的URL
   */
  const loginWithOAuth = useCallback(
    async (provider?: string, redirectTo?: string) => {
      try {
        if (isMounted.current) {
          setState((prev) => ({...prev, isLoading: true, error: null}));
        }
        await services.auth.loginWithOAuth(provider, redirectTo);
      } catch (error) {
        if (isMounted.current) {
          setState((prev) => ({
            ...prev,
            isLoading: false,
            error: error instanceof Error ? error.message : '登录失败',
          }));
        }
        throw error;
      }
    },
    [],
  );

  /**
   * 执行 OAuth 登录操作（兼容旧API）
   * @deprecated 请使用 loginWithOAuth
   * @param redirectTo - 登录成功后重定向的URL
   */
  const login = useCallback(
    async (redirectTo?: string) => {
      return loginWithOAuth(undefined, redirectTo);
    },
    [loginWithOAuth],
  );

  /**
   * 执行登出操作
   * @param redirectTo - 登出后重定向的URL
   */
  const logout = useCallback(async (redirectTo = '/login') => {
    try {
      if (isMounted.current) {
        setState((prev) => ({...prev, isLoading: true}));
      }

      // 清除缓存
      userInfoCache.data = null;
      userInfoCache.timestamp = 0;
      userInfoCache.promise = null;

      // 调用登出方法（会处理API调用和重定向）
      await services.auth.logout(redirectTo);

      if (isMounted.current) {
        setState({
          isAuthenticated: false,
          user: null,
          isLoading: false,
          error: null,
        });
      }
    } catch (error) {
      // 这段代码通常不会执行，因为logout会在出错时也执行重定向
      if (isMounted.current) {
        setState((prev) => ({
          ...prev,
          isLoading: false,
          error: error instanceof Error ? error.message : '登出失败',
        }));
      }
    }
  }, []);

  /**
   * 清除错误信息
   */
  const clearError = useCallback(() => {
    if (isMounted.current) {
      setState((prev) => ({...prev, error: null}));
    }
  }, []);

  // 初始化时检查认证状态
  useEffect(() => {
    isMounted.current = true;

    // 使用requestAnimationFrame延迟执行认证检查，避免与布局渲染冲突
    const timer = requestAnimationFrame(() => {
      checkAuthStatus();
    });

    return () => {
      isMounted.current = false;
      cancelAnimationFrame(timer);
    };
  }, [checkAuthStatus]);

  return {
    isAuthenticated: state.isAuthenticated,
    user: state.user,
    isLoading: state.isLoading,
    error: state.error,
    loginWithCredentials,
    loginWithOAuth,
    login,
    logout,
    clearError,
    checkAuthStatus,
  };
}
