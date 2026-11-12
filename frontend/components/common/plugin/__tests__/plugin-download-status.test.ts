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
 * Unit tests for failed-download message resolution.
 * 下载失败文案解析单元测试。
 */

import {describe, expect, it} from 'vitest';
import {resolvePluginDownloadFailureMessage} from '../plugin-download-status';

describe('resolvePluginDownloadFailureMessage', () => {
  it('prefers error over message', () => {
    expect(
      resolvePluginDownloadFailureMessage('boom', 'hint', 'fallback'),
    ).toBe('boom');
  });

  it('falls back to message when error is empty', () => {
    expect(
      resolvePluginDownloadFailureMessage('', 'from-message', 'fallback'),
    ).toBe('from-message');
  });

  it('uses fallback when both error and message are empty', () => {
    expect(resolvePluginDownloadFailureMessage('', '', 'fallback')).toBe(
      'fallback',
    );
    expect(resolvePluginDownloadFailureMessage(null, undefined, 'hint')).toBe(
      'hint',
    );
    // 空字符串不能被 Boolean() 当成有失败文案。
    // An empty string must not be treated as a present failure message by Boolean().
    expect(
      Boolean(resolvePluginDownloadFailureMessage('', '', 'hint')),
    ).toBe(true);
  });
});
