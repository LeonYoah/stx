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

import {useCallback, useMemo, useState} from 'react';
import type {ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {X} from 'lucide-react';
import {cn} from '@/lib/utils';
import {Button} from '@/components/arc/button/button';
import layout from './list-detail.module.css';

// mergeIdsIntoSelection 是「本页全选/反选」的统一语义：只切给定 id，不丢其他页
// 已选项。作业列表早期用 new Set(本页) 直接覆盖，跨页勾选会被清掉——收敛到这里。
// mergeIdsIntoSelection is the shared select-all/deselect semantics for one page:
// it flips only the given ids and never drops selections made on other pages. The
// job table used to overwrite with new Set(page), losing cross-page picks.
export function mergeIdsIntoSelection<T>(
  current: ReadonlySet<T>,
  ids: readonly T[],
  on: boolean,
): Set<T> {
  const next = new Set(current);
  for (const id of ids) {
    if (on) {
      next.add(id);
    } else {
      next.delete(id);
    }
  }
  return next;
}

export interface PagedSelection<T> {
  /** 当前选中集合（受控状态本体）。/ The selected id set (the state itself). */
  ids: Set<T>;
  setIds: (next: Set<T>) => void;
  toggle: (id: T, on: boolean) => void;
  /** 本页全选/反选（merge 语义）。/ Toggle every id on the current page (merge). */
  togglePage: (on: boolean) => void;
  clear: () => void;
  pageAllSelected: boolean;
  pageSomeSelected: boolean;
}

// usePagedSelection 托管「翻页保留、按页全选」的勾选状态；pageIds 传当前渲染页的
// id 列表，跨页勾选由调用方决定怎么兑现（例如快照解析）。
// usePagedSelection hosts selection state that survives paging, with per-page
// select-all. Pass the ids of the page currently rendered; resolving picks that
// are off-page is the caller's job (e.g. snapshot lookup).
export function usePagedSelection<T>(pageIds: readonly T[]): PagedSelection<T> {
  const [ids, setIds] = useState<Set<T>>(new Set());

  const toggle = useCallback((id: T, on: boolean) => {
    setIds((prev) => mergeIdsIntoSelection(prev, [id], on));
  }, []);

  const togglePage = useCallback(
    (on: boolean) => {
      setIds((prev) => mergeIdsIntoSelection(prev, pageIds, on));
    },
    [pageIds],
  );

  const clear = useCallback(() => setIds(new Set()), []);

  const {pageAllSelected, pageSomeSelected} = useMemo(() => {
    const all = pageIds.length > 0 && pageIds.every((id) => ids.has(id));
    return {
      pageAllSelected: all,
      pageSomeSelected: !all && pageIds.some((id) => ids.has(id)),
    };
  }, [ids, pageIds]);

  return {
    ids,
    setIds,
    toggle,
    togglePage,
    clear,
    pageAllSelected,
    pageSomeSelected,
  };
}

export interface BatchBarProps {
  /** 已选条数；>0 时展示选中计数与动作区。/ Selected count; shows count+actions when >0. */
  selectedCount: number;
  /** 结果行（可选）：最后一次批量动作的回显，由调用方组好文案。/ Optional result line, copy composed by the caller. */
  result?: ReactNode;
  /** 动作按钮区（secondary 小按钮）。/ Action buttons (small secondary buttons). */
  children?: ReactNode;
  busy?: boolean;
  onClear: () => void;
  /** 覆盖默认「已选 N 条」文案（如作业列表用「个作业」）。/ Override the default "{count} selected" text. */
  selectedText?: ReactNode;
  /** 清空按钮 aria-label 覆盖。/ Override the clear-button aria-label. */
  clearLabel?: string;
  /** 批量条 data-testid。/ Optional data-testid on the bar. */
  testId?: string;
  /** 悬浮位置：footer = 分页脚上方（默认）；center = 表格可视区垂直居中。
      / Overlay placement: 'footer' floats above the pagination footer
      (default); 'center' floats at the vertical middle of the table. */
  placement?: 'footer' | 'center';
}

// BatchBar 是三个面板共用的悬浮批量条：表格底部 .batchBarWrap 锚定，
// 选中计数 + 动作区 + 结果行 + 清空 X。挂载侧只需放进相对定位容器。
// BatchBar is the shared floating batch bar: anchored via .batchBarWrap above the
// table footer, showing selected count + action slot + result line + a clear X.
// The host only needs to mount it inside a position:relative container.
export function BatchBar({
  selectedCount,
  result,
  children,
  busy = false,
  onClear,
  selectedText,
  clearLabel,
  testId,
  placement = 'footer',
}: BatchBarProps) {
  const t = useTranslations('common');
  if (selectedCount <= 0 && !result) {
    return null;
  }
  return (
    <div
      className={cn(
        layout.batchBarWrap,
        placement === 'center' && layout.batchBarWrapCenter,
      )}
    >
      <div className={layout.batchBar} role='status' data-testid={testId}>
        {selectedCount > 0 ? (
          <span>
            {selectedText ?? t('selectedCount', {count: selectedCount})}
          </span>
        ) : null}
        {result ? <span className={layout.sub}>{result}</span> : null}
        {children ? (
          <span className={layout.batchActions}>{children}</span>
        ) : null}
        <Button
          type='button'
          variant='ghost'
          size='sm'
          iconOnly
          aria-label={clearLabel ?? t('clearSelection')}
          onClick={onClear}
        >
          <X className='size-4' aria-hidden />
        </Button>
      </div>
    </div>
  );
}
