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

/**
 * 只读长文本面板：原生双向滚动 + Workbench 放大 + 可选复制。
 * 禁止用 ScrollArea 包 mono 长文（默认仅纵向条，外层 overflow-hidden 会裁切）。
 *
 * Read-only long-text panel: native both-axis scroll + Workbench expand + optional copy.
 * Do not wrap mono long text in ScrollArea (vertical-only bar + overflow-hidden clips lines).
 */

import {useCallback, useMemo, useState, type CSSProperties, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy, Maximize2} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  WorkbenchDialogContent,
} from '@/components/ui/dialog';

export type ExpandableTextPanelTone = 'dark' | 'plain';

export type ExpandableTextPanelProps = {
  /** 紧凑区 / 放大弹窗标题 / Compact and expand dialog title */
  title?: ReactNode;
  /** 次要来源路径等 / Secondary path or source hint */
  subtitle?: ReactNode;
  /** 正文；空则展示 emptyLabel / Body text; empty shows emptyLabel */
  content?: string | null;
  emptyLabel?: string;
  /** 紧凑区高度，默认 260px / Compact viewport height, default 260px */
  height?: number | string;
  /** 紧凑区额外 class（如 flex-1 min-h-0）/ Extra class for compact root */
  className?: string;
  /** 滚动容器额外 class / Extra class for scroll viewport */
  bodyClassName?: string;
  tone?: ExpandableTextPanelTone;
  /**
   * false：保留换行、横向可滚；true：自动折行。
   * false: keep lines + horizontal scroll; true: wrap with break-all.
   */
  wrap?: boolean;
  showCopy?: boolean;
  showExpand?: boolean;
  /**
   * 是否展示顶栏（标题 + 操作）。无标题且关闭 copy/expand 时可关。
   * Whether to render the chrome toolbar. Can hide when embedded under a DialogHeader.
   */
  showToolbar?: boolean;
  copyLabel?: string;
  expandLabel?: string;
  copiedLabel?: string;
  /** 复制成功后的回调；默认 clipboard + toast / After copy; default clipboard + toast */
  onCopy?: (text: string) => void;
  toolbarExtra?: ReactNode;
};

function resolveHeight(height: number | string | undefined): CSSProperties {
  if (height == null) {
    return {height: 260};
  }
  if (typeof height === 'number') {
    return {height};
  }
  // 支持 '50vh' / 'flex' 语义：纯 class 时用空 style，由 bodyClassName/className 控高。
  // Support '50vh' or flex sizing via classes: leave style empty when height is a utility keyword.
  if (height === 'flex' || height === 'auto') {
    return {};
  }
  return {height};
}

/**
 * 只读长文本通用面板（滚动 + 放大）。
 * Shared read-only long-text panel with scroll and expand.
 */
export function ExpandableTextPanel({
  title,
  subtitle,
  content,
  emptyLabel,
  height = 260,
  className,
  bodyClassName,
  tone = 'dark',
  wrap = false,
  showCopy = true,
  showExpand = true,
  showToolbar,
  copyLabel,
  expandLabel,
  copiedLabel,
  onCopy,
  toolbarExtra,
}: ExpandableTextPanelProps) {
  const t = useTranslations('common');
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  const resolvedEmpty = emptyLabel ?? t('noContent');
  const resolvedCopyLabel = copyLabel ?? t('copyFullText');
  const resolvedExpandLabel = expandLabel ?? t('expandView');
  const resolvedCopiedLabel = copiedLabel ?? t('copied');
  const displayText = useMemo(() => {
    const raw = typeof content === 'string' ? content : '';
    return raw.length > 0 ? raw : resolvedEmpty;
  }, [content, resolvedEmpty]);
  const hasContent = typeof content === 'string' && content.trim().length > 0;

  const toolbarVisible =
    showToolbar ??
    Boolean(title || subtitle || showCopy || showExpand || toolbarExtra);

  const handleCopy = useCallback(() => {
    if (!hasContent) {
      return;
    }
    const text = content as string;
    if (onCopy) {
      onCopy(text);
      return;
    }
    void navigator.clipboard.writeText(text).then(
      () => {
        setCopied(true);
        toast.success(t('contentCopied'));
        window.setTimeout(() => setCopied(false), 2000);
      },
      () => {
        toast.error(t('copyFailed'));
      },
    );
  }, [content, hasContent, onCopy, t]);

  const preClassName = cn(
    'p-3.5 font-mono text-[11px] leading-relaxed select-text',
    wrap
      ? 'whitespace-pre-wrap break-all'
      : 'whitespace-pre min-w-max',
    tone === 'dark' ? 'text-zinc-300' : 'text-foreground/90',
  );

  const shellClassName = cn(
    'relative overflow-hidden rounded-lg border shadow-inner',
    tone === 'dark'
      ? 'border-zinc-800 bg-zinc-950 text-zinc-200 dark:bg-zinc-900/95'
      : 'border-border/80 bg-background text-foreground',
    className,
  );

  const chromeClassName = cn(
    'flex items-center justify-between gap-2 border-b px-3 py-1.5 text-[11px] font-mono',
    tone === 'dark'
      ? 'border-zinc-800/80 bg-zinc-900/70 text-zinc-400'
      : 'border-border/60 bg-muted/30 text-muted-foreground',
  );

  const heightStyle = resolveHeight(height);
  const isFlexHeight = height === 'flex' || height === 'auto';

  const scrollBody = (
    <div
      className={cn(
        'overflow-auto overscroll-contain',
        isFlexHeight ? 'min-h-0 flex-1' : undefined,
        bodyClassName,
      )}
      style={isFlexHeight ? undefined : heightStyle}
    >
      <pre className={preClassName}>{displayText}</pre>
    </div>
  );

  const actionButtons = (hasContent || toolbarExtra) && (
    <div className='flex items-center gap-1.5 shrink-0'>
      {toolbarExtra}
      {showExpand && hasContent ? (
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-7 text-xs gap-1'
          onClick={() => setExpanded(true)}
        >
          <Maximize2 className='h-3.5 w-3.5' />
          <span>{resolvedExpandLabel}</span>
        </Button>
      ) : null}
      {showCopy && hasContent ? (
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-7 text-xs gap-1'
          onClick={handleCopy}
        >
          {copied ? (
            <>
              <Check className='h-3.5 w-3.5 text-emerald-500' />
              <span>{resolvedCopiedLabel}</span>
            </>
          ) : (
            <>
              <Copy className='h-3.5 w-3.5' />
              <span>{resolvedCopyLabel}</span>
            </>
          )}
        </Button>
      ) : null}
    </div>
  );

  return (
    <>
      <div
        className={cn(
          shellClassName,
          isFlexHeight && 'flex min-h-0 flex-1 flex-col',
        )}
      >
        {toolbarVisible ? (
          <div className={chromeClassName}>
            <div className='min-w-0 flex flex-col gap-0.5'>
              {title ? (
                <span className='truncate text-foreground/80 dark:text-zinc-300'>
                  {title}
                </span>
              ) : null}
              {subtitle ? (
                <span className='truncate max-w-[320px] opacity-80'>
                  {subtitle}
                </span>
              ) : null}
            </div>
            {actionButtons}
          </div>
        ) : null}
        {scrollBody}
      </div>

      {showExpand ? (
        <Dialog open={expanded} onOpenChange={setExpanded}>
          <WorkbenchDialogContent className='gap-0 border border-border/80 p-0 shadow-2xl dark:border-border/60'>
            <DialogHeader className='shrink-0 space-y-1 border-b px-4 py-3 pr-12 text-left'>
              <DialogTitle className='text-base'>
                {title || resolvedExpandLabel}
              </DialogTitle>
              {subtitle ? (
                <DialogDescription className='font-mono text-xs truncate'>
                  {subtitle}
                </DialogDescription>
              ) : (
                <DialogDescription className='sr-only'>
                  {resolvedExpandLabel}
                </DialogDescription>
              )}
            </DialogHeader>
            {(showCopy || toolbarExtra) && hasContent ? (
              <div className='flex shrink-0 items-center justify-end gap-2 border-b bg-muted/20 px-4 py-2'>
                {toolbarExtra}
                {showCopy ? (
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    className='h-7 text-xs gap-1'
                    onClick={handleCopy}
                  >
                    {copied ? (
                      <>
                        <Check className='h-3.5 w-3.5 text-emerald-500' />
                        <span>{resolvedCopiedLabel}</span>
                      </>
                    ) : (
                      <>
                        <Copy className='h-3.5 w-3.5' />
                        <span>{resolvedCopyLabel}</span>
                      </>
                    )}
                  </Button>
                ) : null}
              </div>
            ) : null}
            <div
              className={cn(
                'min-h-0 flex-1 overflow-auto overscroll-contain',
                tone === 'dark' ? 'bg-zinc-950 text-zinc-200' : 'bg-background',
              )}
            >
              <pre
                className={cn(
                  preClassName,
                  'p-4 text-xs',
                  tone === 'dark' ? 'text-zinc-300' : undefined,
                )}
              >
                {displayText}
              </pre>
            </div>
          </WorkbenchDialogContent>
        </Dialog>
      ) : null}
    </>
  );
}
