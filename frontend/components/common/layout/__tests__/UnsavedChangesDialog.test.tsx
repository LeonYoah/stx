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

// 通用未保存确认弹窗：覆盖规则页两按钮形态与工作台三按钮形态（默认焦点均落在安全留守键）。
// Shared UnsavedChangesDialog: covers the two-action rule variant and three-action
// workbench variant, keeping initial focus on the safe stay button in both.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {WorkbenchLeaveDialog} from '@/components/common/sync/WorkbenchLeaveDialog';
import {UnsavedChangesDialog} from '../UnsavedChangesDialog';

describe('UnsavedChangesDialog (shared)', () => {
  it('supports the two-action rule variant and focuses keep button', async () => {
    const onKeep = vi.fn();
    const onDiscard = vi.fn();
    render(
      <UnsavedChangesDialog
        open
        title='放弃未保存的修改？'
        description='说明'
        keepLabel='继续编辑'
        discardLabel='放弃修改'
        onKeepEditing={onKeep}
        onDiscard={onDiscard}
      />,
    );
    const keepBtn = await screen.findByRole('button', {name: '继续编辑'});
    await waitFor(() => expect(keepBtn).toHaveFocus());
    fireEvent.click(keepBtn);
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it('supports the three-action workbench variant via WorkbenchLeaveDialog', async () => {
    const onStay = vi.fn();
    const onSave = vi.fn();
    const onDiscard = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <WorkbenchLeaveDialog
          open
          onOpenChange={onOpenChange}
          onStay={onStay}
          onSaveAndLeave={onSave}
          onDiscardAndLeave={onDiscard}
        />
      </NextIntlClientProvider>,
    );
    const stayBtn = await screen.findByRole('button', {name: '留下继续编辑'});
    await waitFor(() => expect(stayBtn).toHaveFocus());
    fireEvent.click(screen.getByRole('button', {name: '保存并离开'}));
    expect(onSave).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', {name: '丢弃并离开'}));
    expect(onDiscard).toHaveBeenCalledTimes(1);
    fireEvent.click(stayBtn);
    expect(onStay).toHaveBeenCalledTimes(1);
  });
});
