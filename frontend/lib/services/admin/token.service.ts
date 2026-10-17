/**
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {BaseService} from '../core/base.service';

export type CLITokenStatus = 'active' | 'expired' | 'revoked';

export interface CLITokenInfo {
  id: number;
  user_id: number;
  username: string;
  name: string;
  token_prefix: string;
  expires_at?: string | null;
  revoked_at?: string | null;
  last_used_at?: string | null;
  created_at: string;
  permanent: boolean;
  status: CLITokenStatus | string;
}

export interface ListCLITokensParams {
  current: number;
  size: number;
  user_id?: number;
  username?: string;
  name?: string;
  status?: string;
}

export interface ListCLITokensResponse {
  total: number;
  active: number;
  expired: number;
  revoked: number;
  tokens: CLITokenInfo[];
}

export interface CreateCLITokenRequest {
  user_id: number;
  name: string;
  expires_in: string;
}

export interface CreateCLITokenResult extends CLITokenInfo {
  token?: string;
}

/**
 * 管理员 CLI 令牌服务。
 * Administrator CLI token service.
 */
export class AdminTokenService extends BaseService {
  protected static readonly basePath = '/admin/cli-tokens';

  /**
   * 列出令牌元数据，不含原文。
   * List token metadata without plaintext.
   */
  static async listTokens(
    params: ListCLITokensParams,
  ): Promise<ListCLITokensResponse> {
    const queryParams = new URLSearchParams();
    queryParams.append('current', params.current.toString());
    queryParams.append('size', params.size.toString());
    if (params.user_id) {
      queryParams.append('user_id', params.user_id.toString());
    }
    if (params.username) {
      queryParams.append('username', params.username);
    }
    if (params.name) {
      queryParams.append('name', params.name);
    }
    if (params.status) {
      queryParams.append('status', params.status);
    }
    const data = await this.get<ListCLITokensResponse>(
      `?${queryParams.toString()}`,
    );
    return {
      total: data?.total ?? 0,
      active: data?.active ?? 0,
      expired: data?.expired ?? 0,
      revoked: data?.revoked ?? 0,
      tokens: data?.tokens ?? [],
    };
  }

  /**
   * 签发令牌，原文只返回一次。
   * Issue a token; plaintext is returned once.
   */
  static async createToken(
    data: CreateCLITokenRequest,
  ): Promise<CreateCLITokenResult> {
    return this.post<CreateCLITokenResult>('', data);
  }

  /**
   * 吊销令牌。
   * Revoke a token.
   */
  static async revokeToken(id: number): Promise<CLITokenInfo> {
    return this.post<CLITokenInfo>(`/${id}/revoke`);
  }
}
