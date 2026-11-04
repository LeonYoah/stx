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

'use client';

import * as React from 'react';
import {AlertCircle, File as FileIcon, Loader2, Upload, X} from 'lucide-react';
import {useTranslations} from 'next-intl';

import {Button, buttonVariants} from '@/components/ui/button';
import {cn} from '@/lib/utils';
import type {VariantProps} from 'class-variance-authority';

/**
 * 合并多个 ref，避免 asChild（DropdownMenuItem）拿到的 DOM ref 被 imperative handle 覆盖。
 * Merge refs so an asChild DOM ref is not overwritten by an imperative handle.
 */
function composeRefs<T>(
  ...refs: Array<React.Ref<T> | undefined | null>
): React.RefCallback<T> {
  return (node) => {
    for (const ref of refs) {
      if (!ref) {
        continue;
      }
      if (typeof ref === 'function') {
        ref(node);
      } else {
        (ref as React.MutableRefObject<T | null>).current = node;
      }
    }
  };
}

/** 统一上传入口的展示形态 / Visual variants for the shared file picker */
export type FileUploadVariant = 'dropzone' | 'button' | 'input' | 'menu-item';

export type FileUploadHandle = {
  /** 打开系统文件选择器 / Open the native file picker */
  open: () => void;
  /** 清空原生 input 值 / Clear the native input value */
  clearInput: () => void;
};

export type FileUploadProps = {
  id?: string;
  accept?: string;
  disabled?: boolean;
  /** 上传中：禁用交互，按钮形态可显示转圈 / Busy: block interaction; button variant may show a spinner */
  uploading?: boolean;
  /**
   * 受控选中文件（用于展示文件名 / 清除）。fire-and-forget 场景可不传。
   * Controlled selected file for display / clear. Omit for fire-and-forget uploads.
   */
  value?: File | null;
  /** 选中或清除文件时回调；清除时传 null / Called on select or clear; null means cleared */
  onFileChange?: (file: File | null) => void;
  /**
   * 选中后立刻清空原生 input，便于再次选同一文件（立即上传场景）。
   * Clear the native input after each pick so the same file can be re-selected (immediate upload).
   */
  resetOnSelect?: boolean;
  /** 内联错误文案；父级负责校验逻辑 / Inline error text; parents own validation */
  error?: string | null;
  variant?: FileUploadVariant;
  /** 是否展示已选文件名（input/dropzone 默认开，button/menu-item 默认关） */
  showSelectedFile?: boolean;
  /** 是否展示清除按钮（需有 value） / Show clear when a controlled value is set */
  showClear?: boolean;
  /** 是否展示文件大小（dropzone 选中态） / Show file size in dropzone selected state */
  showFileSize?: boolean;
  /** dropzone 空态主文案 / Dropzone empty-state title */
  emptyLabel?: React.ReactNode;
  /** dropzone 空态次要提示 / Dropzone empty-state hint */
  emptyHint?: React.ReactNode;
  /** button 形态按钮文案 / Button-variant label */
  buttonLabel?: React.ReactNode;
  buttonVariant?: VariantProps<typeof buttonVariants>['variant'];
  buttonSize?: VariantProps<typeof buttonVariants>['size'];
  /** 覆盖默认空态内容（仍保留拖放与点击选文件） / Replace default empty chrome; keep drag/click */
  dropzoneContent?: React.ReactNode;
  /** menu-item 形态的触发内容（图标 + 文案） / Menu-item trigger content */
  children?: React.ReactNode;
  className?: string;
  inputClassName?: string;
  'data-testid'?: string;
};

/**
 * 全站统一的文件选择入口：唯一允许出现原生 `<input type="file">` 的组件。
 * Shared file picker — the only place native `<input type="file">` is allowed.
 *
 * 由原 react-dropzone 演示组件重构为 shadcn Button + 原生 input / DnD，符合 ui-conventions。
 * Refactored from the old react-dropzone demo into shadcn Button + native input / DnD per ui-conventions.
 *
 * 校验、上传请求与 toast 仍由调用方负责；本组件只负责选文件、展示、清除与禁用态。
 * Validation, upload requests, and toasts stay in the caller; this component picks, shows, clears, and disables.
 */
export const FileUpload = React.forwardRef<FileUploadHandle, FileUploadProps>(
  function FileUpload(
    {
      id,
      accept,
      disabled = false,
      uploading = false,
      value,
      onFileChange,
      resetOnSelect = false,
      error = null,
      variant = 'button',
      showSelectedFile,
      showClear,
      showFileSize = true,
      emptyLabel,
      emptyHint,
      buttonLabel,
      buttonVariant = 'default',
      buttonSize = 'default',
      dropzoneContent,
      children,
      className,
      inputClassName,
      'data-testid': testId,
    },
    ref,
  ) {
    const t = useTranslations('common.fileUpload');
    const inputRef = React.useRef<HTMLInputElement>(null);
    const [dragActive, setDragActive] = React.useState(false);
    const generatedId = React.useId();
    const inputId = id ?? generatedId;

    const isDisabled = disabled || uploading;
    const shouldShowSelected =
      showSelectedFile ?? (variant === 'dropzone' || variant === 'input');
    const shouldShowClear =
      showClear ?? (variant === 'dropzone' && value != null);

    const openPicker = React.useCallback(() => {
      if (isDisabled) {
        return;
      }
      inputRef.current?.click();
    }, [isDisabled]);

    const clearInput = React.useCallback(() => {
      if (inputRef.current) {
        inputRef.current.value = '';
      }
    }, []);

    // menu-item 走 asChild，必须把 forwarded ref 挂到 label DOM；其它形态才暴露 imperative API
    // menu-item uses asChild — forward the ref to the label DOM; other variants expose the handle API
    const imperativeRef = variant === 'menu-item' ? null : ref;
    React.useImperativeHandle(
      imperativeRef,
      () => ({
        open: openPicker,
        clearInput,
      }),
      [openPicker, clearInput],
    );

    const emitFile = React.useCallback(
      (file: File | null) => {
        onFileChange?.(file);
        if (resetOnSelect || file == null) {
          clearInput();
        }
      },
      [onFileChange, resetOnSelect, clearInput],
    );

    const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0] ?? null;
      if (file) {
        emitFile(file);
      } else if (!resetOnSelect) {
        // 用户取消选择时不强制清空受控 value / Cancelling the picker must not clear a controlled value
        clearInput();
      }
    };

    const handleClear = (event?: React.MouseEvent) => {
      event?.preventDefault();
      event?.stopPropagation();
      if (isDisabled) {
        return;
      }
      emitFile(null);
    };

    const handleDrag = (event: React.DragEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (isDisabled) {
        return;
      }
      if (event.type === 'dragenter' || event.type === 'dragover') {
        setDragActive(true);
      } else if (event.type === 'dragleave') {
        setDragActive(false);
      }
    };

    const handleDrop = (event: React.DragEvent) => {
      event.preventDefault();
      event.stopPropagation();
      setDragActive(false);
      if (isDisabled) {
        return;
      }
      const file = event.dataTransfer.files?.[0] ?? null;
      if (file) {
        emitFile(file);
      }
    };

    const hiddenInput = (
      <input
        ref={inputRef}
        id={inputId}
        type='file'
        accept={accept}
        disabled={isDisabled}
        className='sr-only'
        tabIndex={-1}
        aria-hidden={variant === 'input' ? undefined : true}
        data-testid={testId ? `${testId}-input` : undefined}
        onChange={handleInputChange}
      />
    );

    const errorBlock =
      error != null && error !== '' ? (
        <div
          role='alert'
          className='flex items-center gap-2 text-destructive text-sm'
          data-testid={testId ? `${testId}-error` : undefined}
        >
          <AlertCircle className='h-4 w-4 shrink-0' />
          <span className='min-w-0 break-words'>{error}</span>
        </div>
      ) : null;

    const selectedBanner =
      shouldShowSelected && value ? (
        <div
          className='rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground'
          data-testid={testId ? `${testId}-selected` : undefined}
        >
          <span className='whitespace-nowrap'>{t('selectedFile')}: </span>
          <span className='font-medium text-foreground break-all'>
            {value.name}
          </span>
        </div>
      ) : null;

    if (variant === 'menu-item') {
      // 样式交给 DropdownMenuItem asChild；composeRefs 保留 Slot 传入的 DOM ref
      // DropdownMenuItem asChild owns chrome; composeRefs keeps the Slot DOM ref
      return (
        <label
          ref={composeRefs(ref as React.Ref<HTMLLabelElement>)}
          className={cn(
            'relative cursor-pointer',
            isDisabled && 'pointer-events-none opacity-50',
            className,
          )}
          data-testid={testId}
        >
          {children}
          {hiddenInput}
        </label>
      );
    }

    if (variant === 'input') {
      // 直接渲染带 shadcn Input 样式的 file input，保证 ref / open() 可用
      // Render a file input with shadcn Input styles so ref / open() keep working
      return (
        <div className={cn('space-y-2', className)} data-testid={testId}>
          <input
            ref={inputRef}
            id={inputId}
            type='file'
            accept={accept}
            disabled={isDisabled}
            data-slot='input'
            data-testid={testId ? `${testId}-input` : undefined}
            className={cn(
              'file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input flex h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
              'focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]',
              'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive',
              inputClassName,
            )}
            aria-invalid={error ? true : undefined}
            onChange={handleInputChange}
          />
          {selectedBanner}
          {errorBlock}
        </div>
      );
    }

    if (variant === 'button') {
      return (
        <div className={cn('space-y-2', className)} data-testid={testId}>
          {hiddenInput}
          <Button
            type='button'
            variant={buttonVariant}
            size={buttonSize}
            disabled={isDisabled}
            className={cn(buttonSize === 'default' && 'w-full')}
            data-testid={testId ? `${testId}-trigger` : undefined}
            onClick={openPicker}
          >
            {uploading ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Upload className='h-4 w-4' />
            )}
            {buttonLabel ?? t('chooseFile')}
          </Button>
          {selectedBanner}
          {errorBlock}
        </div>
      );
    }

    // dropzone
    return (
      <div className={cn('space-y-2', className)} data-testid={testId}>
        <div
          role='button'
          tabIndex={isDisabled ? -1 : 0}
          aria-disabled={isDisabled || undefined}
          aria-label={
            typeof emptyLabel === 'string' ? emptyLabel : t('chooseFile')
          }
          className={cn(
            'relative rounded-lg border-2 border-dashed p-5 text-center transition-colors',
            dragActive
              ? 'border-primary bg-primary/5'
              : 'border-muted-foreground/25 hover:border-primary/50 hover:bg-muted/20',
            value ? 'bg-muted/50' : '',
            isDisabled
              ? 'pointer-events-none cursor-not-allowed opacity-50'
              : 'cursor-pointer',
          )}
          onClick={openPicker}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              openPicker();
            }
          }}
          onDragEnter={handleDrag}
          onDragLeave={handleDrag}
          onDragOver={handleDrag}
          onDrop={handleDrop}
        >
          {hiddenInput}
          {value ? (
            <div className='flex items-center justify-center gap-3'>
              <FileIcon className='h-8 w-8 shrink-0 text-primary' />
              <div className='min-w-0 text-left'>
                <p className='truncate font-medium' title={value.name}>
                  {value.name}
                </p>
                {showFileSize ? (
                  <p className='text-sm text-muted-foreground whitespace-nowrap'>
                    {(value.size / 1024 / 1024).toFixed(2)} MB
                  </p>
                ) : null}
              </div>
              {shouldShowClear ? (
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  aria-label={t('clearFile')}
                  disabled={isDisabled}
                  data-testid={testId ? `${testId}-clear` : undefined}
                  onClick={handleClear}
                >
                  <X className='h-4 w-4' />
                </Button>
              ) : null}
            </div>
          ) : dropzoneContent != null ? (
            dropzoneContent
          ) : (
            <div className='space-y-2'>
              <Upload className='mx-auto h-10 w-10 text-muted-foreground' />
              <p className='text-muted-foreground'>
                {emptyLabel ?? t('dragDropOrClick')}
              </p>
              {emptyHint ? (
                <p className='text-xs text-muted-foreground'>{emptyHint}</p>
              ) : null}
              {uploading ? (
                <p className='text-xs text-muted-foreground'>{t('uploading')}</p>
              ) : null}
            </div>
          )}
        </div>
        {variant !== 'dropzone' || !value ? selectedBanner : null}
        {errorBlock}
      </div>
    );
  },
);

FileUpload.displayName = 'FileUpload';
