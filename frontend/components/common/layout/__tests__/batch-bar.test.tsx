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

import {act, render, renderHook, screen, fireEvent} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import layout from '../list-detail.module.css';
import {
  BatchBar,
  mergeIdsIntoSelection,
  usePagedSelection,
} from '../batch-bar';

describe('mergeIdsIntoSelection', () => {
  it('adds and removes only the given ids, keeping off-page picks', () => {
    // 第 1 页勾了 99，当前页 [1,2] 全选：99 必须留下。
    // With 99 picked on another page, selecting [1,2] keeps it.
    expect(mergeIdsIntoSelection(new Set([99]), [1, 2], true)).toEqual(
      new Set([99, 1, 2]),
    );
    // 反选同样只切本页。
    // Deselect also only flips the page ids.
    expect(mergeIdsIntoSelection(new Set([99, 1, 2]), [1, 2], false)).toEqual(
      new Set([99]),
    );
    expect(mergeIdsIntoSelection(new Set(), [], true)).toEqual(new Set());
  });
});

describe('usePagedSelection', () => {
  it('toggles single ids and the whole page with indeterminate in between', () => {
    const {result} = renderHook(() => usePagedSelection([1, 2, 3]));

    act(() => result.current.toggle(1, true));
    expect(result.current.ids).toEqual(new Set([1]));
    expect(result.current.pageSomeSelected).toBe(true);
    expect(result.current.pageAllSelected).toBe(false);

    act(() => result.current.togglePage(true));
    expect(result.current.ids).toEqual(new Set([1, 2, 3]));
    expect(result.current.pageAllSelected).toBe(true);

    act(() => result.current.togglePage(false));
    expect(result.current.ids).toEqual(new Set());
    act(() => result.current.toggle(2, true));
    act(() => result.current.clear());
    expect(result.current.ids.size).toBe(0);
  });
});

describe('BatchBar', () => {
  const wrap = (node: React.ReactNode) => (
    <NextIntlClientProvider locale='zh' messages={zh}>
      {node}
    </NextIntlClientProvider>
  );

  it('renders nothing when nothing is selected and no result', () => {
    const {container} = render(
      wrap(<BatchBar selectedCount={0} onClear={vi.fn()} />),
    );
    expect(container.firstChild).toBeNull();
  });

  it('shows the shared count text, actions and clears via the X button', () => {
    const onClear = vi.fn();
    render(
      wrap(
        <BatchBar selectedCount={2} onClear={onClear}>
          <button type='button'>动作</button>
        </BatchBar>,
      ),
    );
    expect(screen.getByText('已选 2 条')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', {name: '清空选择'}));
    expect(onClear).toHaveBeenCalled();
  });

  it('shows the result line after the selection is cleared', () => {
    render(
      wrap(
        <BatchBar
          selectedCount={0}
          result='已删除 1 条，跳过 1 条'
          onClear={vi.fn()}
        />,
      ),
    );
    expect(screen.getByText('已删除 1 条，跳过 1 条')).toBeTruthy();
  });

  it('applies the centered overlay class only for placement="center"', () => {
    const {container, unmount} = render(
      wrap(
        <BatchBar selectedCount={1} placement='center' onClear={vi.fn()} />,
      ),
    );
    const centered = container.firstElementChild;
    expect(centered?.classList.contains(layout.batchBarWrap)).toBe(true);
    expect(centered?.classList.contains(layout.batchBarWrapCenter)).toBe(true);
    unmount();

    const {container: footerBox} = render(
      wrap(<BatchBar selectedCount={1} onClear={vi.fn()} />),
    );
    const bottom = footerBox.firstElementChild;
    expect(bottom?.classList.contains(layout.batchBarWrap)).toBe(true);
    expect(bottom?.classList.contains(layout.batchBarWrapCenter)).toBe(false);
  });
});
