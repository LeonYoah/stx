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

import {ApiResponse, BasicUserInfo} from '../core/types';

/**
 * 用户名密码登录请求参数
 */
export interface LoginRequest {
  /** 用户名 */
  username: string;
  /** 密码 */
  password: string;
}

/**
 * 登录响应数据
 */
export interface LoginResponseData {
  /** 用户ID */
  id: number;
  /** 用户名 */
  username: string;
  /** 昵称 */
  nickname: string;
  /** 邮箱 */
  email?: string;
  /** 语言偏好 */
  language?: 'zh' | 'en';
  /** 展示时区（IANA），空串表示跟随浏览器 / Display timezone, '' = follow browser */
  timezone?: string;
  /** 系统默认展示时区 / System default display timezone */
  system_timezone?: string;
  /** 头像 */
  avatar_url?: string;
  /** 是否管理员 */
  is_admin: boolean;
  /** 是否激活 */
  is_active?: boolean;
}

/**
 * 登录响应
 */
export type LoginResponse = ApiResponse<LoginResponseData>;

/**
 * OAuth登录URL响应
 */
export type GetLoginURLResponse = ApiResponse<string>;

/**
 * OAuth回调请求参数
 */
export interface CallbackRequest {
  /** OAuth状态码，用于验证请求合法性 */
  state: string;
  /** 授权码，用于获取访问令牌 */
  code: string;
}

/**
 * OAuth回调响应
 */
export type CallbackResponse = ApiResponse<null>;

/**
 * 用户信息响应
 */
export type UserInfoResponse = ApiResponse<BasicUserInfo>;

/**
 * 更新个人信息请求
 */
export interface UpdateProfileRequest {
  /** 邮箱 */
  email?: string;
  /** 语言偏好 */
  language?: 'zh' | 'en';
  /** 展示时区（IANA），空串表示跟随浏览器 / Display timezone, '' = follow browser */
  timezone?: string;
}

/**
 * 更新个人信息响应
 */
export type UpdateProfileResponse = ApiResponse<BasicUserInfo>;
