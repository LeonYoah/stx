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

// 通用排序菜单：触发按钮回显当前选项；菜单项是 sort+order 组合的 radio；禁用项保留并给出原因。
// Generic sort menu: the trigger echoes the active option; items are radio entries of sort+order
// pairs; disabled options stay listed with their reason.

import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {SortMenu, type SortMenuOption} from '../SortMenu';

const OPTIONS: SortMenuOption[] = [
  {sort: 'started_at', order: 'desc', label: '开始时间（新→旧）'},
  {sort: 'started_at', order: 'asc', label: '开始时间（旧→新）'},
  {sort: 'duration', order: 'desc', label: '耗时（长→短）'},
  {sort: 'duration', order: 'asc', label: '耗时（短→长）'},
  {
    sort: 'rate',
    order: 'desc',
    label: '写入速率（高→低）',
    disabledReason: '选中「运行中」后可按速率或延迟排序',
  },
];

function renderMenu(
  overrides: {onSelect?: (sort: string, order: string) => void} = {},
) {
  const onSelect = overrides.onSelect ?? vi.fn();
  render(
    <SortMenu
      label='排序'
      options={OPTIONS}
      value={{sort: 'started_at', order: 'desc'}}
      onSelect={onSelect}
      testId='sort-menu'
    />,
  );
  fireEvent.pointerDown(screen.getByTestId('sort-menu'));
  return {onSelect};
}

describe('SortMenu', () => {
  it('shows the active option on the trigger and checks it in the menu', async () => {
    renderMenu();
    const trigger = screen.getByTestId('sort-menu');
    expect(trigger.textContent).toContain('排序');
    expect(trigger.textContent).toContain('开始时间（新→旧）');

    const menu = await screen.findByRole('menu');
    const checked = menu.querySelector(
      '[role="menuitemradio"][aria-checked="true"]',
    );
    expect(checked?.textContent).toBe('开始时间（新→旧）');
  });

  it('emits the sort and order of the picked option', async () => {
    const {onSelect} = renderMenu();
    fireEvent.click(
      await screen.findByRole('menuitemradio', {name: '耗时（长→短）'}),
    );
    expect(onSelect).toHaveBeenCalledWith('duration', 'desc');
  });

  it('keeps a disabled option listed with its reason instead of hiding it', async () => {
    const {onSelect} = renderMenu();
    const rate = await screen.findByRole('menuitemradio', {
      name: '写入速率（高→低）',
    });
    expect(rate.getAttribute('aria-disabled')).toBe('true');
    fireEvent.click(rate);
    expect(onSelect).not.toHaveBeenCalled();
  });
});
