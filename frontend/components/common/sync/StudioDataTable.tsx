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

import type {
  ButtonHTMLAttributes,
  HTMLAttributes,
  ReactNode,
  TdHTMLAttributes,
  ThHTMLAttributes,
} from 'react';
import {ArrowDown, ArrowUp} from 'lucide-react';
import {cn} from '@/lib/utils';
import './studio-chrome.css';

export type StudioSortDirection = 'asc' | 'desc';

export type StudioSortDescriptor<K extends string = string> = {
  key: K;
  direction: StudioSortDirection;
};

/**
 * 切换当前列排序：第一次用 initial，之后在升序/降序间翻转。
 * Toggle the active column sort: first click uses initial, then flips asc/desc.
 */
export function nextStudioSort<K extends string>(
  current: StudioSortDescriptor<K> | null,
  key: K,
  initial: StudioSortDirection = 'asc',
): StudioSortDescriptor<K> {
  if (!current || current.key !== key) {
    return {key, direction: initial};
  }
  return {key, direction: current.direction === 'asc' ? 'desc' : 'asc'};
}

function isEmptySortValue(value: unknown): boolean {
  return value === null || value === undefined || value === '';
}

/**
 * 空值沉底；数字和可解析日期按数值比，其余按数字感知字符串比。
 * Empty values sink; numbers and parseable dates compare numerically, everything else uses numeric-aware text.
 */
export function compareStudioValues(left: unknown, right: unknown): number {
  if (isEmptySortValue(left) && isEmptySortValue(right)) {
    return 0;
  }
  if (isEmptySortValue(left)) {
    return 1;
  }
  if (isEmptySortValue(right)) {
    return -1;
  }
  if (typeof left === 'number' && typeof right === 'number') {
    return left - right;
  }
  const leftText = String(left);
  const rightText = String(right);
  const looksLikeDate = (value: string) =>
    /\d{4}-\d{2}-\d{2}/.test(value) || /\dT\d/.test(value);
  if (looksLikeDate(leftText) && looksLikeDate(rightText)) {
    const leftTime = Date.parse(leftText);
    const rightTime = Date.parse(rightText);
    if (Number.isFinite(leftTime) && Number.isFinite(rightTime)) {
      return leftTime - rightTime;
    }
  }
  const leftNum = typeof left === 'number' ? left : Number(left);
  const rightNum = typeof right === 'number' ? right : Number(right);
  if (
    Number.isFinite(leftNum) &&
    Number.isFinite(rightNum) &&
    leftText.trim() !== '' &&
    rightText.trim() !== ''
  ) {
    if (leftNum !== rightNum) {
      return leftNum - rightNum;
    }
  }
  return leftText.localeCompare(rightText, undefined, {
    numeric: true,
    sensitivity: 'base',
  });
}

/**
 * 按当前描述符排序；没有描述符时保持原顺序。
 * Sort a copy by the active descriptor; keep the original order when none is set.
 */
export function applyStudioSort<T>(
  rows: T[],
  sort: StudioSortDescriptor | null,
  valueOf: (row: T, key: string) => unknown,
): T[] {
  if (!sort) {
    return rows;
  }
  const copy = rows.slice();
  copy.sort((left, right) => {
    const cmp = compareStudioValues(
      valueOf(left, sort.key),
      valueOf(right, sort.key),
    );
    return sort.direction === 'desc' ? -cmp : cmp;
  });
  return copy;
}

export function StudioTable({
  label,
  busy = false,
  className,
  children,
}: {
  label: string;
  busy?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={cn('studio-data-table', className)}
      data-slot='studio-data-table'
      data-busy={busy ? 'true' : undefined}
      aria-label={label}
      aria-busy={busy || undefined}
    >
      {children}
    </div>
  );
}

export function StudioTableScroll({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn('studio-data-table__scroll', className)}>{children}</div>
  );
}

export function StudioTableContent({
  className,
  minWidth,
  children,
}: {
  className?: string;
  minWidth?: number | string;
  children: ReactNode;
}) {
  return (
    <table
      className={cn('studio-data-table__content', className)}
      style={minWidth === undefined ? undefined : {minWidth}}
    >
      {children}
    </table>
  );
}

export function StudioTableHeader({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <thead className={cn('studio-data-table__header', className)}>
      <tr>{children}</tr>
    </thead>
  );
}

export function StudioTableColumn({
  className,
  align = 'start',
  allowsSorting = false,
  sortDirection,
  children,
  ...props
}: Omit<ThHTMLAttributes<HTMLTableCellElement>, 'align'> & {
  align?: 'start' | 'end';
  allowsSorting?: boolean;
  sortDirection?: StudioSortDirection;
}) {
  const ariaSort = allowsSorting
    ? sortDirection === 'asc'
      ? 'ascending'
      : sortDirection === 'desc'
        ? 'descending'
        : 'none'
    : undefined;
  return (
    <th
      className={cn(
        'studio-data-table__column',
        align === 'end' && 'studio-data-table__column--end',
        className,
      )}
      aria-sort={ariaSort}
      data-allows-sorting={allowsSorting ? 'true' : undefined}
      {...props}
    >
      {children}
    </th>
  );
}

export function StudioSortableColumnHeader({
  label,
  sortDirection,
  onSort,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  sortDirection?: StudioSortDirection;
  onSort: () => void;
}) {
  return (
    <button
      type='button'
      className={cn('group studio-data-table__sort', className)}
      onClick={onSort}
      {...props}
    >
      <span className='min-w-0 truncate'>{label}</span>
      <span className='studio-data-table__sort-icon' aria-hidden='true'>
        {sortDirection === 'asc' ? (
          <ArrowUp className='size-3' />
        ) : sortDirection === 'desc' ? (
          <ArrowDown className='size-3' />
        ) : (
          <ArrowUp className='size-3 opacity-0 group-hover:opacity-40' />
        )}
      </span>
    </button>
  );
}

export function StudioTableBody({
  className,
  dimmed = false,
  children,
}: {
  className?: string;
  dimmed?: boolean;
  children: ReactNode;
}) {
  return (
    <tbody
      className={cn(
        'studio-data-table__body',
        dimmed && 'studio-data-table__body--dimmed',
        className,
      )}
    >
      {children}
    </tbody>
  );
}

export function StudioTableRow({
  className,
  selected = false,
  children,
  ...props
}: HTMLAttributes<HTMLTableRowElement> & {selected?: boolean}) {
  return (
    <tr
      className={cn('studio-data-table__row', className)}
      data-selected={selected ? 'true' : undefined}
      {...props}
    >
      {children}
    </tr>
  );
}

export function StudioTableCell({
  className,
  align = 'start',
  children,
  ...props
}: Omit<TdHTMLAttributes<HTMLTableCellElement>, 'align'> & {
  align?: 'start' | 'end';
}) {
  return (
    <td
      className={cn(
        'studio-data-table__cell',
        align === 'end' && 'studio-data-table__cell--end',
        className,
      )}
      {...props}
    >
      {children}
    </td>
  );
}

export function StudioTableEmptyRow({
  colSpan,
  children,
}: {
  colSpan: number;
  children: ReactNode;
}) {
  return (
    <tr className='studio-data-table__row studio-data-table__row--empty'>
      <td className='studio-data-table__empty' colSpan={colSpan}>
        {children}
      </td>
    </tr>
  );
}

export function StudioTableEmpty({children}: {children: ReactNode}) {
  return <div className='studio-data-table__empty'>{children}</div>;
}

export function StudioTableFooter({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn('studio-data-table__footer', className)}>{children}</div>
  );
}
