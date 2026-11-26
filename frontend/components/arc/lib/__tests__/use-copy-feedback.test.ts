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

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {writeClipboard} from '../use-copy-feedback';

function withoutClipboardApi() {
  Object.defineProperty(navigator, 'clipboard', {value: undefined, configurable: true});
}

function withClipboardApi(writeText: ReturnType<typeof vi.fn>) {
  Object.defineProperty(navigator, 'clipboard', {value: {writeText}, configurable: true});
}

let execSpy: ReturnType<typeof vi.fn>;

beforeEach(() => {
  execSpy = vi.fn().mockReturnValue(true);
  Object.defineProperty(document, 'execCommand', {value: execSpy, configurable: true, writable: true});
  document.body.innerHTML = '';
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('writeClipboard', () => {
  it('优先使用 clipboard API', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    withClipboardApi(writeText);
    await writeClipboard('hello');
    expect(writeText).toHaveBeenCalledWith('hello');
    expect(execSpy).not.toHaveBeenCalled();
  });

  it('clipboard API 缺失时回落 execCommand，textarea 挂在 body', async () => {
    withoutClipboardApi();
    await writeClipboard('fallback');
    expect(execSpy).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('复制按钮在弹窗内时 textarea 挂进该弹窗', async () => {
    withoutClipboardApi();
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    const button = document.createElement('button');
    dialog.appendChild(button);
    document.body.appendChild(dialog);
    button.focus();

    await writeClipboard('in-dialog');
    expect(execSpy).toHaveBeenCalledWith('copy');
    expect(dialog.querySelector('textarea')).toBeNull();
    expect(document.activeElement).toBe(button);
  });

  it('弹窗内复制完成后临时 textarea 已移除且焦点回到触发元素', async () => {
    withoutClipboardApi();
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    const button = document.createElement('button');
    dialog.appendChild(button);
    document.body.appendChild(dialog);
    button.focus();

    let observedParent: Node | null = null;
    execSpy.mockImplementation(() => {
      const area = document.querySelector('textarea');
      observedParent = area?.parentNode ?? null;
      expect(document.activeElement).toBe(area);
      return true;
    });

    await writeClipboard('focus-check');
    expect(observedParent).toBe(dialog);
    expect(document.activeElement).toBe(button);
  });

  it('clipboard API 拒绝时同样回落 execCommand', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'));
    withClipboardApi(writeText);
    await writeClipboard('fallback-after-deny');
    expect(execSpy).toHaveBeenCalledWith('copy');
  });

  it('两条路径都失败时抛错', async () => {
    withoutClipboardApi();
    execSpy.mockReturnValue(false);
    await expect(writeClipboard('x')).rejects.toThrow('Copy failed');
  });
});
