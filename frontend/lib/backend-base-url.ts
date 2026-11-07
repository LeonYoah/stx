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

/** 控制面 API 默认地址（同机部署）。 / Default control-plane API URL for same-host deploys. */
export const DEFAULT_BACKEND_BASE_URL = 'http://127.0.0.1:17800';

type EnvLike = Record<string, string | undefined>;

/**
 * 解析前端服务端转发用的后端地址。
 * Resolve the backend origin used by the frontend server-side /api proxy.
 *
 * 优先 `BACKEND_BASE_URL`（服务端变量）；兼容旧名 `NEXT_PUBLIC_BACKEND_BASE_URL`。
 * Prefer server-only `BACKEND_BASE_URL`; keep `NEXT_PUBLIC_BACKEND_BASE_URL` as an alias.
 */
export function resolveBackendBaseURL(
  env: EnvLike = process.env as EnvLike,
): string {
  const raw =
    trimEnv(env.BACKEND_BASE_URL) ||
    trimEnv(env.NEXT_PUBLIC_BACKEND_BASE_URL) ||
    DEFAULT_BACKEND_BASE_URL;
  return raw.replace(/\/+$/, '');
}

function trimEnv(value: string | undefined): string {
  return typeof value === 'string' ? value.trim() : '';
}
