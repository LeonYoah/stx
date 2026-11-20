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

// 工作台文件标签栏测试：验证 Radix Tabs 键盘切换、关闭按钮、中键关闭与右键上下文菜单。
// StudioFileTabs tests: Radix keyboard navigation, close button, middle-click, and context menu.

import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {StudioFileTabs} from '../StudioFileTabs';

function renderTabs(
  overrides: Partial<React.ComponentProps<typeof StudioFileTabs>> = {},
) {
  const onSelectTab = vi.fn();
  const onCloseTab = vi.fn();
  const onCloseOtherTabs = vi.fn();
  const onCloseAllTabs = vi.fn();

  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <StudioFileTabs
        tabs={[
          {id: 1, name: 'job-a', dirty: false},
          {id: 2, name: 'job-b', dirty: true},
          {id: 3, name: 'job-c', dirty: false},
        ]}
        activeTabId={1}
        onSelectTab={onSelectTab}
        onCloseTab={onCloseTab}
        onCloseOtherTabs={onCloseOtherTabs}
        onCloseAllTabs={onCloseAllTabs}
        {...overrides}
      />
    </NextIntlClientProvider>,
  );

  return {onSelectTab, onCloseTab, onCloseOtherTabs, onCloseAllTabs};
}

describe('StudioFileTabs', () => {
  it('switches tabs by click and shows unsaved indicator', () => {
    const {onSelectTab} = renderTabs();
    const tab1 = screen.getByRole('tab', {name: 'job-a'});
    const tab2 = screen.getByRole('tab', {name: 'job-b'});
    expect(tab1).toHaveAttribute('aria-selected', 'true');
    expect(tab2).toHaveAttribute('aria-selected', 'false');
    expect(screen.getByLabelText(zh.workbenchStudio.unsavedDraft)).toBeInTheDocument();

    // Radix Tabs 在指针按下或键盘方向键时触发切换 / Radix Tabs activates on mouseDown or arrow keys
    fireEvent.mouseDown(tab2, {button: 0});
    expect(onSelectTab).toHaveBeenCalledWith(2);

    fireEvent.keyDown(tab1, {key: 'ArrowRight'});
    expect(onSelectTab).toHaveBeenCalledWith(2);
  });

  it('closes by close button and middle click', () => {
    const {onCloseTab} = renderTabs();
    fireEvent.click(
      screen.getByRole('button', {
        name: `${zh.workbenchStudio.close} job-b`,
      }),
    );
    expect(onCloseTab).toHaveBeenCalledWith(2);

    const tab3 = screen.getByRole('tab', {name: 'job-c'});
    fireEvent.mouseUp(tab3);
    fireEvent(
      tab3,
      new MouseEvent('auxclick', {bubbles: true, cancelable: true, button: 1}),
    );
    expect(onCloseTab).toHaveBeenCalledWith(3);
  });

  it('opens right-click context menu for close others and close all', () => {
    const {onCloseOtherTabs, onCloseAllTabs} = renderTabs();
    const tab2 = screen.getByRole('tab', {name: 'job-b'});
    fireEvent.contextMenu(tab2, {clientX: 120, clientY: 40});

    fireEvent.click(
      screen.getByRole('menuitem', {
        name: zh.workbenchStudio.closeOtherTabs,
      }),
    );
    expect(onCloseOtherTabs).toHaveBeenCalledWith(2);

    fireEvent.contextMenu(tab2, {clientX: 120, clientY: 40});
    fireEvent.click(
      screen.getByRole('menuitem', {
        name: zh.workbenchStudio.closeAllTabs,
      }),
    );
    expect(onCloseAllTabs).toHaveBeenCalledTimes(1);
  });
});
