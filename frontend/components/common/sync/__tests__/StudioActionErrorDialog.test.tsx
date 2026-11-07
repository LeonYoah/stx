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

// 运行失败的长错误必须落在限高弹窗里，不能把弹窗撑出视口。
// A long run failure must stay inside a height-capped dialog.

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {StudioActionErrorDialog} from '../StudioDialogs';

vi.mock('sonner', () => ({
  toast: {success: vi.fn(), error: vi.fn()},
}));

const longError = `Caused by: java.lang.RuntimeException: boom\n${'at org.apache.seatunnel.Engine.fail(Engine.java:1)\n'.repeat(80)}`;

describe('StudioActionErrorDialog', () => {
  it('caps a long failure inside the dialog instead of growing with the stack', () => {
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <StudioActionErrorDialog
          error={{
            title: '运行失败',
            description: longError,
            category: 'runtime',
          }}
          onOpenChange={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    const dialog = screen.getByTestId('studio-action-error-dialog');
    expect(dialog.className).toContain('max-h-[85vh]');
    expect(dialog.className).toContain('overflow-hidden');
    expect(screen.getByText(/Caused by: java.lang.RuntimeException: boom/)).toBeTruthy();
    const description = screen.getByText(/Caused by: java.lang.RuntimeException: boom/);
    expect(description.className).toContain('max-h-[40vh]');
    expect(description.className).toContain('overflow-auto');
  });
});
