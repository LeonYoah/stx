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

import {describe, expect, it} from 'vitest';
import {formatOnboardingLastError, formatStepStatusDetail} from '../messages';

describe('formatStepStatusDetail (#109 copy)', () => {
  const catalog: Record<string, string> = {
    checking: '正在检查…',
    checkFailed: '检查失败，请重试',
    'errors.package_missing': '还缺安装包',
    'errors.plugin_missing': '还缺插件',
    'errors.hosts_not_connected': '有服务器未连接',
    'errors.generic': '检查失败，请重试',
    missingHosts: '未连接：{hosts}',
  };
  /** Minimal ICU plural stub for en one/other used in assertions. */
  const enCatalog: Record<string, string> = {
    'errors.hosts_not_connected':
      '{count, plural, one {A server is not connected} other {Some servers are not connected}}',
    missingHosts:
      '{count, plural, one {Offline host: {hosts}} other {Offline hosts: {hosts}}}',
  };
  const resolvePlural = (
    template: string,
    values?: Record<string, string | number>,
  ) => {
    let out = template;
    if (values && typeof values.count === 'number' && out.includes('plural')) {
      const count = values.count;
      const match = out.match(
        /\{count,\s*plural,\s*one\s*\{([^}]*)\}\s*other\s*\{([^}]*)\}\}/,
      );
      if (match) {
        out = count === 1 ? match[1] : match[2];
      }
    }
    if (values) {
      for (const [k, v] of Object.entries(values)) {
        out = out.replace(`{${k}}`, String(v));
      }
    }
    return out;
  };
  const t = ((key: string, values?: Record<string, string | number>) => {
    return resolvePlural(catalog[key] || key, values);
  }) as ((
    key: string,
    values?: Record<string, string | number>,
  ) => string) & {has: (key: string) => boolean};
  t.has = (key: string) => key in catalog;

  it('shows concrete shortage copy instead of raw incomplete', () => {
    expect(formatStepStatusDetail(t, 'incomplete', 'package_missing')).toBe(
      '还缺安装包',
    );
    expect(formatStepStatusDetail(t, 'incomplete', 'plugin_missing')).toBe(
      '还缺插件',
    );
    expect(
      formatStepStatusDetail(t, 'incomplete', 'hosts_not_connected', ['b']),
    ).toBe('有服务器未连接 未连接：b');
  });

  it('uses English singular/plural for offline host detail', () => {
    // Stub next-intl ICU one/other without a full plural parser.
    // 用简易分支模拟 next-intl 的 one/other，避免在单测里嵌套解析 ICU。
    const tEn = ((key: string, values?: Record<string, string | number>) => {
      const count = typeof values?.count === 'number' ? values.count : 0;
      const hosts = values?.hosts != null ? String(values.hosts) : '';
      if (key === 'errors.hosts_not_connected') {
        return count === 1
          ? 'A server is not connected'
          : 'Some servers are not connected';
      }
      if (key === 'missingHosts') {
        return count === 1
          ? `Offline host: ${hosts}`
          : `Offline hosts: ${hosts}`;
      }
      return resolvePlural(enCatalog[key] || catalog[key] || key, values);
    }) as typeof t;
    tEn.has = (key: string) => key in enCatalog || key in catalog;
    expect(
      formatStepStatusDetail(tEn, 'incomplete', 'hosts_not_connected', ['b']),
    ).toBe('A server is not connected Offline host: b');
    expect(
      formatStepStatusDetail(tEn, 'incomplete', 'hosts_not_connected', [
        'a',
        'b',
      ]),
    ).toBe('Some servers are not connected Offline hosts: a, b');
  });

  it('maps errors to check-failed copy', () => {
    expect(formatStepStatusDetail(t, 'error', 'hosts_fetch_failed')).toBe(
      '检查失败，请重试',
    );
  });

  it('never surfaces ListHostsRequest validation prose', () => {
    const raw =
      "Key: 'ListHostsRequest.Size' Error:Field validation for 'Size' failed on the 'max' tag";
    expect(formatOnboardingLastError(t, raw)).toBe('检查失败，请重试');
  });
});
