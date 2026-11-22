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
 * 全局截断文本与路径展示原语：仅在实际溢出（或显式指定）时在悬停后即时弹出
 * 主题化 Radix Tooltip 显示完整原文（避免原生 title 延迟过长或在 WebView 中不弹）；
 * 支持 `mode='path'` 中间省略保留尾部关键路径段，以及可选的悬停一键复制。
 * Global truncated text / path display primitive: lazily detects DOM overflow on hover
 * and shows a themed Radix Tooltip with the full unwrapped content (avoiding slow or
 * suppressed native HTML title tooltips). Supports `mode='path'` middle-truncation to
 * keep trailing path segments visible, plus optional hover copy-to-clipboard.
 */
import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy} from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {toast} from '@/lib/toast';
import {cn} from '@/lib/utils';

export interface TruncatedTextProps {
  /** 完整原文；为空时回退展示 fallback / Full raw text; renders fallback when empty */
  text?: string | null;
  /** 空值占位符，默认 '-' / Empty placeholder, defaults to '-' */
  fallback?: string;
  /** 自定义悬停提示内容；默认直接展示完整原文 / Custom tooltip content; defaults to full text */
  tooltip?: ReactNode;
  /** 即使未截断也展示提示（如附加版本/状态说明时）/ Always show tooltip even if text fits */
  alwaysTooltip?: boolean;
  /** 'end' 尾部省略；'path' 路径中间省略保留末尾关键段 / 'end' tail ellipsis; 'path' middle ellipsis keeping tail segments */
  mode?: 'end' | 'path';
  /** 使用等宽字体（适合路径、ID、版本号）/ Use monospace font */
  mono?: boolean;
  /** 悬停时在右侧浮现一键复制按钮（复制完整原文）/ Reveal a subtle copy button on hover */
  copyable?: boolean;
  /** Tooltip 弹出方向 / Tooltip placement */
  side?: 'top' | 'bottom' | 'left' | 'right';
  className?: string;
  textClassName?: string;
  'data-testid'?: string;
}

/**
 * 将长路径拆分为「可截断前缀 + 保留尾部」，确保如
 * `/Library/Java/JavaVirtualMachines/jdk-26.jdk/Contents/Home` 在窄列中保留关键尾部段。
 * Split a long path into a truncatable prefix and a pinned tail so narrow columns keep
 * the distinguishing trailing segments (e.g. `.../jdk-26.jdk/Contents/Home`).
 */
export function splitPathForMiddleEllipsis(
  value: string,
): [string, string] | null {
  if (!value.includes('/') || value.length < 28) {
    return null;
  }
  const hasLeadingSlash = value.startsWith('/');
  const segments = value.split('/').filter(Boolean);
  // 短路径（<28 字符或 ≤2 段，如 /opt/jdk11、/usr/lib/jvm/java-8）保持单节点不做中间拆分。
  // Short paths (<28 chars or <=2 segments like /opt/jdk11) stay a single node without splitting.
  if (segments.length <= 2) {
    return null;
  }

  let tailCount = 1;
  let tailLen = segments[segments.length - 1].length;
  // 向上吸纳至多 3 段，直到尾部具备足够辨识度（≥18 字符），同时至少留出 1 段前缀。
  // Accumulate up to 3 trailing segments until >=18 chars for distinctiveness while keeping >=1 prefix segment.
  while (tailCount < 3 && tailCount < segments.length - 1 && tailLen < 18) {
    tailCount += 1;
    tailLen += 1 + segments[segments.length - tailCount].length;
  }

  const prefixParts = segments.slice(0, segments.length - tailCount);
  const tailParts = segments.slice(segments.length - tailCount);
  const prefix = `${hasLeadingSlash ? '/' : ''}${prefixParts.join('/')}`;
  const tail = `/${tailParts.join('/')}`;
  return [prefix, tail];
}

export function TruncatedText({
  text,
  fallback = '-',
  tooltip,
  alwaysTooltip = false,
  mode = 'end',
  mono = false,
  copyable = false,
  side = 'top',
  className,
  textClassName,
  'data-testid': testId,
}: TruncatedTextProps) {
  const t = useTranslations('common');
  const raw = text?.trim() ?? '';
  const hasValue = raw.length > 0;

  const prefixRef = useRef<HTMLSpanElement>(null);
  const fullRef = useRef<HTMLSpanElement>(null);
  const [overflowing, setOverflowing] = useState(false);
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const pathParts = useMemo(
    () => (mode === 'path' && hasValue ? splitPathForMiddleEllipsis(raw) : null),
    [mode, hasValue, raw],
  );

  const measureOverflow = useCallback(() => {
    const el = pathParts ? prefixRef.current : fullRef.current;
    if (!el) {
      return false;
    }
    const next = el.scrollWidth > el.clientWidth + 1;
    setOverflowing(next);
    return next;
  }, [pathParts]);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        setOpen(false);
        return;
      }
      if (!hasValue) {
        return;
      }
      const isTruncated = measureOverflow();
      if (alwaysTooltip || isTruncated) {
        setOpen(true);
      }
    },
    [alwaysTooltip, hasValue, measureOverflow],
  );

  const handleCopy = useCallback(
    async (event: React.MouseEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      event.preventDefault();
      if (!hasValue) {
        return;
      }
      try {
        await navigator.clipboard.writeText(raw);
        setCopied(true);
        toast.success(t('copied'));
        window.setTimeout(() => setCopied(false), 1600);
      } catch {
        // 剪贴板不可用时静默忽略 / Ignore clipboard errors
      }
    },
    [hasValue, raw, t],
  );

  if (!hasValue) {
    return (
      <span
        className={cn(
          'text-muted-foreground',
          mono && 'font-mono',
          className,
        )}
        data-testid={testId}
      >
        {fallback}
      </span>
    );
  }

  const tooltipBody = tooltip ?? (
    <span className={cn('block', mono && 'font-mono')}>{raw}</span>
  );

  const contentNode = pathParts ? (
    <span
      className={cn(
        'inline-flex min-w-0 max-w-full items-baseline overflow-hidden whitespace-nowrap align-bottom',
        mono && 'font-mono',
        textClassName,
      )}
    >
      <span ref={prefixRef} className='min-w-0 truncate'>
        {pathParts[0]}
      </span>
      <span className='shrink-0'>{pathParts[1]}</span>
    </span>
  ) : (
    <span
      ref={fullRef}
      className={cn(
        'block min-w-0 max-w-full truncate',
        mono && 'font-mono',
        textClassName,
      )}
    >
      {raw}
    </span>
  );

  return (
    <div
      className={cn(
        'group/trunc flex min-w-0 max-w-full items-center gap-1',
        className,
      )}
      data-testid={testId}
    >
      <TooltipProvider delayDuration={180}>
        <Tooltip open={open} onOpenChange={handleOpenChange}>
          <TooltipTrigger asChild>
            <span
              className={cn(
                'block min-w-0 flex-1 overflow-hidden',
                (alwaysTooltip || overflowing) && 'cursor-default',
              )}
              onPointerEnter={measureOverflow}
              onFocus={measureOverflow}
            >
              {contentNode}
            </span>
          </TooltipTrigger>
          <TooltipContent
            side={side}
            sideOffset={6}
            className='max-w-[min(90vw,480px)] whitespace-pre-wrap [overflow-wrap:anywhere] text-left leading-relaxed'
          >
            {tooltipBody}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      {copyable ? (
        <button
          type='button'
          onClick={(e) => void handleCopy(e)}
          aria-label={t('copy')}
          className={cn(
            'shrink-0 rounded p-0.5 text-muted-foreground transition-opacity hover:text-foreground focus-visible:opacity-100',
            copied
              ? 'opacity-100 text-emerald-500'
              : 'opacity-0 group-hover/trunc:opacity-100',
          )}
        >
          {copied ? (
            <Check className='size-3 text-emerald-500' aria-hidden />
          ) : (
            <Copy className='size-3' aria-hidden />
          )}
        </button>
      ) : null}
    </div>
  );
}
