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

/**
 * API基础响应格式
 * @template T - 响应数据类型
 */
export interface ApiResponse<T = unknown> {
  /** 错误信息，成功时为空字符串 */
  error_msg: string;
  /** 响应数据 */
  data: T;
}

/**
 * 分页数据响应格式
 * @template T - 列表项类型
 */
export interface PaginatedResponse<T = unknown> {
  /** 总数量 */
  total: number;
  /** 结果列表 */
  results: T[];
}

/**
 * API错误响应
 */
export interface ApiError {
  /** 错误信息 */
  error_msg: string;
  /** 可选的结构化错误数据 / Optional structured error data */
  data?: unknown;
}

/**
 * 用户信任等级
 */
/* eslint-disable no-unused-vars */
export enum TrustLevel {
  /** 新用户 */
  NEW_USER = 0,
  /** 基本用户 */
  BASIC_USER = 1,
  /** 成员 */
  USER = 2,
  /** 活跃用户 */
  ACTIVE_USER = 3,
  /** 领导者 */
  LEADER = 4,
}

/**
 * 基础用户信息
 */
export interface BasicUserInfo {
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
  /** 信任等级 */
  trust_level: TrustLevel;
  /** 头像URL */
  avatar_url: string;
  /** 用户分数 */
  score: number;
  /** 是否为管理员 */
  is_admin?: boolean;
  /** 是否激活 */
  is_active?: boolean;
}
