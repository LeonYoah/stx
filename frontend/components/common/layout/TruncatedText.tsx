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
 * 全局截断文本与路径展示原语：基于 useOverflowTooltip 实现静止态零浮层实例开销，
 * 仅在实际溢出（或显式指定 alwaysTooltip）时在悬停 180ms 后弹出主题化浮层显示完整原文；
 * 支持 `mode='path'` 路径中间省略保留末尾关键段，以及可选的悬停一键复制（仅开启时订阅 i18n）。
 * Global truncated text / path primitive: built on useOverflowTooltip for zero rest-state
 * overlay overhead, lazily revealing full content in a themed portal only on actual overflow
 * (or alwaysTooltip). Supports `mode='path'` middle-truncation and opt-in hover copy.
 */
import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy} from 'lucide-react';
import {toast} from '@/lib/toast';
import {cn} from '@/lib/utils';
import {
  useOverflowTooltip,
  type OverflowTooltipSide,
} from './OverflowTooltip';
import {writeClipboard} from '@/components/arc/lib/use-copy-feedback';

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
  /** 挂 data-detail-name 供 DetailHost 飞入动画定位 / Attach data-detail-name for DetailHost fly-in */
  detailName?: boolean;
  /** Tooltip 弹出方向 / Tooltip placement */
  side?: OverflowTooltipSide;
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

/** 仅在开启 copyable 时挂载，避免普通列表单元格普遍订阅 useTranslations。
    Mounted only when copyable is enabled so regular table cells skip subscribing to next-intl. */
function CopyTextButton({value}: {value: string}) {
  const t = useTranslations('common');
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(
    async (event: MouseEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      event.preventDefault();
      try {
        await writeClipboard(value);
        setCopied(true);
        toast.success(t('copied'));
        window.setTimeout(() => setCopied(false), 1600);
      } catch {
        // 剪贴板不可用时静默忽略 / Ignore clipboard errors
      }
    },
    [t, value],
  );

  return (
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
  );
}

export function TruncatedText({
  text,
  fallback = '-',
  tooltip,
  alwaysTooltip = false,
  mode = 'end',
  mono = false,
  copyable = false,
  detailName = false,
  side = 'top',
  className,
  textClassName,
  'data-testid': testId,
}: TruncatedTextProps) {
  const raw = text?.trim() ?? '';
  const hasValue = raw.length > 0;

  const prefixRef = useRef<HTMLSpanElement>(null);
  const fullRef = useRef<HTMLSpanElement>(null);
  const anchorRef = useRef<HTMLSpanElement>(null);

  const pathParts = useMemo(
    () => (mode === 'path' && hasValue ? splitPathForMiddleEllipsis(raw) : null),
    [mode, hasValue, raw],
  );

  const tooltipBody = useMemo(
    () =>
      !hasValue
        ? null
        : (tooltip ?? (
            <span className={cn('block', mono && 'font-mono')}>{raw}</span>
          )),
    [hasValue, mono, raw, tooltip],
  );

  const {triggerProps, portal} = useOverflowTooltip({
    targetRef: pathParts ? prefixRef : fullRef,
    anchorRef: pathParts ? anchorRef : fullRef,
    content: tooltipBody,
    alwaysShow: alwaysTooltip,
    side,
    disabled: !hasValue,
  });

  if (!hasValue) {
    return (
      <span
        className={cn(
          'text-muted-foreground',
          mono && 'font-mono',
          className,
        )}
        data-testid={testId}
        data-detail-name={detailName ? '' : undefined}
      >
        {fallback}
      </span>
    );
  }

  const textElement = pathParts ? (
    <span
      ref={anchorRef}
      {...triggerProps}
      data-testid={copyable ? undefined : testId}
      data-detail-name={detailName ? '' : undefined}
      className={cn(
        'inline-flex min-w-0 max-w-full items-baseline overflow-hidden whitespace-nowrap align-bottom',
        mono && 'font-mono',
        !copyable && className,
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
      {...triggerProps}
      data-testid={copyable ? undefined : testId}
      data-detail-name={detailName ? '' : undefined}
      className={cn(
        'block min-w-0 max-w-full truncate',
        mono && 'font-mono',
        !copyable && className,
        textClassName,
      )}
    >
      {raw}
    </span>
  );

  if (!copyable) {
    return (
      <>
        {textElement}
        {portal}
      </>
    );
  }

  return (
    <div
      className={cn(
        'group/trunc flex min-w-0 max-w-full items-center gap-1',
        className,
      )}
      data-testid={testId}
    >
      <span className='block min-w-0 flex-1 overflow-hidden'>{textElement}</span>
      {portal}
      <CopyTextButton value={raw} />
    </div>
  );
}
