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
 * 将 Checkpoint namespace 映射为 IMAP namespace：末级目录 checkpoint → imap
 * Map Checkpoint namespace to IMAP by renaming the last path segment checkpoint → imap
 *
 * 例 / Examples:
 * - /tmp/seatunnel/checkpoint/ → /tmp/seatunnel/imap/
 * - /seatunnel/checkpoint → /seatunnel/imap
 * - /data/ck → /data/ck（末级不是 checkpoint 则原样返回）
 */
export function mapCheckpointNamespaceToImap(namespace: string): string {
  const raw = (namespace || '').trim();
  if (!raw) {
    return '/tmp/seatunnel/imap/';
  }

  const hasTrailingSlash = raw.endsWith('/');
  const trimmed = hasTrailingSlash ? raw.slice(0, -1) : raw;
  const parts = trimmed.split('/');
  const last = parts[parts.length - 1] || '';

  if (last.toLowerCase() === 'checkpoint') {
    parts[parts.length - 1] = 'imap';
  }

  const joined = parts.join('/');
  return hasTrailingSlash || raw.endsWith('/') ? `${joined}/` : joined;
}
