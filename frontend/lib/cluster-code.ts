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
 * Cluster Code Utility Functions
 * 集群标识工具函数
 */

const CLUSTER_CODE_REGEX = /^[a-z0-9]{2,32}$/;

/**
 * Validates cluster code format (2-32 lowercase letters and digits)
 * 校验集群标识格式（2-32位纯小写字母与数字）
 */
export function isValidClusterCode(code: string): boolean {
  return CLUSTER_CODE_REGEX.test(code);
}

/**
 * Recommend a cluster code based on the cluster name
 * 基于集群名称推荐集群标识（保留英文字母与数字，转小写并截断）
 */
export function recommendClusterCode(name: string): string {
  if (!name) return '';
  const cleaned = name
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
    .slice(0, 32);
  return cleaned;
}
