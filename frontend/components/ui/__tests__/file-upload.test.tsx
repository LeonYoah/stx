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

import React from 'react';
import {describe, expect, it, vi} from 'vitest';
import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';

import {FileUpload, type FileUploadHandle} from '@/components/ui/file-upload';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import zh from '@/lib/i18n/locales/zh.json';

function renderUpload(ui: React.ReactElement) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      {ui}
    </NextIntlClientProvider>,
  );
}

function makeFile(name = 'pkg.tar.gz', type = 'application/gzip') {
  return new File(['content'], name, {type});
}

describe('FileUpload', () => {
  it('button 形态选择文件并支持 resetOnSelect', () => {
    const onFileChange = vi.fn();
    renderUpload(
      <FileUpload
        variant='button'
        accept='.tar.gz'
        resetOnSelect
        buttonLabel='选择离线包'
        data-testid='fu'
        onFileChange={onFileChange}
      />,
    );

    const input = screen.getByTestId('fu-input') as HTMLInputElement;
    expect(input).toHaveAttribute('type', 'file');
    expect(input).toHaveAttribute('accept', '.tar.gz');

    const file = makeFile();
    fireEvent.change(input, {target: {files: [file]}});
    expect(onFileChange).toHaveBeenCalledWith(file);
    expect(input.value).toBe('');
  });

  it('dropzone 展示已选文件名并支持清除', () => {
    const onFileChange = vi.fn();
    const file = makeFile('seatunnel-bin.tar.gz');
    renderUpload(
      <FileUpload
        variant='dropzone'
        value={file}
        data-testid='fu'
        onFileChange={onFileChange}
      />,
    );

    expect(screen.getByText('seatunnel-bin.tar.gz')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('fu-clear'));
    expect(onFileChange).toHaveBeenCalledWith(null);
  });

  it('disabled / uploading 时不可打开选择器', () => {
    const ref = React.createRef<FileUploadHandle>();
    renderUpload(
      <FileUpload
        ref={ref}
        variant='button'
        uploading
        data-testid='fu'
        onFileChange={vi.fn()}
      />,
    );

    const input = screen.getByTestId('fu-input') as HTMLInputElement;
    const clickSpy = vi.spyOn(input, 'click');
    ref.current?.open();
    expect(clickSpy).not.toHaveBeenCalled();
    expect(screen.getByTestId('fu-trigger')).toBeDisabled();
  });

  it('展示错误提示（i18n 无关的透传文案）', () => {
    renderUpload(
      <FileUpload
        variant='input'
        error='文件格式不正确'
        data-testid='fu'
        onFileChange={vi.fn()}
      />,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('文件格式不正确');
  });

  it('menu-item 形态包含隐藏 file input', () => {
    renderUpload(
      <FileUpload
        variant='menu-item'
        accept='.tar.gz'
        resetOnSelect
        data-testid='fu-menu'
        onFileChange={vi.fn()}
      >
        <span>上传源码</span>
      </FileUpload>,
    );
    expect(screen.getByText('上传源码')).toBeInTheDocument();
    expect(screen.getByTestId('fu-menu-input')).toHaveAttribute('type', 'file');
  });

  it('menu-item 将 forwarded ref 挂到 label DOM，选文件不抛错', () => {
    let node: HTMLLabelElement | null = null;
    const onFileChange = vi.fn();
    const refCb = ((value: unknown) => {
      node = value as HTMLLabelElement | null;
    }) as React.Ref<FileUploadHandle>;

    renderUpload(
      <FileUpload
        ref={refCb}
        variant='menu-item'
        accept='.tar.gz'
        resetOnSelect
        data-testid='fu-menu'
        onFileChange={onFileChange}
      >
        <span>上传源码包</span>
      </FileUpload>,
    );

    // asChild / Slot 需要 DOM 节点，不能是 {open, clearInput}
    // asChild / Slot needs a DOM node, not an imperative {open, clearInput} object
    expect(node).toBeInstanceOf(HTMLLabelElement);
    expect(node).toBe(screen.getByTestId('fu-menu'));

    const input = screen.getByTestId('fu-menu-input') as HTMLInputElement;
    const file = makeFile('source.tar.gz');
    expect(() => {
      fireEvent.change(input, {target: {files: [file]}});
    }).not.toThrow();
    expect(onFileChange).toHaveBeenCalledWith(file);
  });

  it('menu-item 在 DropdownMenuItem asChild 下选文件不抛错且可关闭菜单', () => {
    const onFileChange = vi.fn();
    const onOpenChange = vi.fn();
    renderUpload(
      <DropdownMenu open onOpenChange={onOpenChange}>
        <DropdownMenuTrigger asChild>
          <button type='button'>更多</button>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem asChild>
            <FileUpload
              variant='menu-item'
              accept='.tar.gz'
              resetOnSelect
              data-testid='fu-menu'
              onFileChange={onFileChange}
            >
              <span>上传源码包</span>
            </FileUpload>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>,
    );

    const input = screen.getByTestId('fu-menu-input') as HTMLInputElement;
    expect(() => {
      fireEvent.change(input, {target: {files: [makeFile('src.tar.gz')]}});
    }).not.toThrow();
    expect(onFileChange).toHaveBeenCalled();

    // 点击菜单外或 Escape 应能正常关闭（ref 指向 DOM 后 Radix 焦点逻辑可用）
    // Escape / outside click can close once the ref points at a real DOM node
    fireEvent.keyDown(document, {key: 'Escape'});
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
