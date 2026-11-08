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

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it} from 'vitest';
import zhMessages from '@/lib/i18n/locales/zh.json';
import {ValidationResultPanel} from '../StudioDialogs';
import type {SyncValidateResult} from '@/lib/services/sync';

function renderPanel(result: SyncValidateResult) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zhMessages}>
      <ValidationResultPanel result={result} />
    </NextIntlClientProvider>,
  );
}

// 校验结果面板：后端返回 null 列表时不能崩溃
// Validation result panel must not crash when the backend returns null lists
describe('ValidationResultPanel', () => {
  it('renders undefined-variable results whose warnings are null', () => {
    const result = {
      valid: false,
      errors: [
        'sync: undefined template variable: {{sink_parallelism}} (未在任务自定义变量或全局变量中配置)',
      ],
      warnings: null,
      summary: 'Connection test failed: undefined template variable.',
      detected_vars: ['sink_parallelism'],
    } as unknown as SyncValidateResult;

    renderPanel(result);

    expect(screen.getByText(/sink_parallelism/)).toBeInTheDocument();
  });

  it('tolerates null errors, warnings and checks together', () => {
    const result = {
      valid: true,
      errors: null,
      warnings: null,
      checks: null,
      summary: '',
    } as unknown as SyncValidateResult;

    expect(() => renderPanel(result)).not.toThrow();
  });
});
