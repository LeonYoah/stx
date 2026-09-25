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

/**
 * STX 产品版本信息（与后端 /api/v1/version 对齐）。
 * STX product version info aligned with backend /api/v1/version.
 */
export interface StxVersionInfo {
  version: string;
  git_commit: string;
  build_time: string;
  min_cli_version: string;
}

/**
 * 产品版本服务：读取服务端真实构建版本。
 * Product version service: reads the server's real build version.
 */
export class CapabilityService extends BaseService {
  protected static readonly basePath = '';

  /**
   * 获取当前服务端产品版本（公开接口，无需登录）。
   * Fetch the current server product version (public; no login required).
   */
  static async getVersion(): Promise<StxVersionInfo> {
    return this.get<StxVersionInfo>('/version');
  }
}
