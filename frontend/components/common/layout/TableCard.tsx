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
 * 列表页唯一主卡片（design §4.4 / §9.7）：1px var(--border) 边框、
 * var(--radius-panel) 圆角；刷新时旧数据保持原样只显示顶部加载条；首屏
 * TableSkeletonRows（300ms 后才出现、出现后至少保留 500ms）；错误留在卡片里
 * 加重试按钮（有旧数据时降为顶部提示条）；空态插槽；底部分页栏
 * border-t bg-muted/10，右侧统一用 @/components/ui/pagination。
 * The single main card of a list page (design §4.4 / §9.7): 1px var(--border)
 * frame, var(--radius-panel) corners. On refresh old data stays readable with
 * only the top hairline bar; the first paint shows TableSkeletonRows (appears
 * only after 300ms of loading, then stays at least 500ms). Errors stay inside
 * the card with a retry button (an inline strip when old data remains). Empty
 * state is a slot. The pagination footer is border-t bg-muted/10 with
 * @/components/ui/pagination on the right.
 */
import {useEffect, useRef, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {Pagination} from '@/components/ui/pagination';
import {cn} from '@/lib/utils';
import {TableLoadingBar} from './TableLoadingBar';
import {TableSkeletonRows} from './TableSkeletonRows';

/** 骨架屏延迟规则：加载超过 300ms 才显示骨架，出现后至少保留 500ms。Skeleton delay rule. */
const SKELETON_DELAY_MS = 300;
const SKELETON_MIN_STAY_MS = 500;

/**
 * 首屏骨架可见性：loading 持续超过 300ms 才亮骨架，亮起后至少保留 500ms，
 * 避免一闪而过（design §4.4 的公共延迟规则）。
 * First-screen skeleton visibility: shows only when loading lasts over 300ms,
 * then stays for at least 500ms so it never flashes (shared rule of §4.4).
 */
function useSkeletonVisible(loading: boolean, hasData: boolean): boolean {
  const wantSkeleton = loading && !hasData;
  const [visible, setVisible] = useState(false);
  const shownAt = useRef(0);
  useEffect(() => {
    if (wantSkeleton && !visible) {
      const timer = setTimeout(() => {
        shownAt.current = Date.now();
        setVisible(true);
      }, SKELETON_DELAY_MS);
      return () => clearTimeout(timer);
    }
    if (!wantSkeleton && visible) {
      const remain = SKELETON_MIN_STAY_MS - (Date.now() - shownAt.current);
      const timer = setTimeout(() => setVisible(false), Math.max(0, remain));
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [wantSkeleton, visible]);
  return visible;
}

export interface TableCardPagination {
  currentPage: number;
  totalPages: number;
  pageSize: number;
  totalItems: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  pageSizeOptions?: number[];
}

export interface TableCardProps {
  /** 加载中；有旧数据时只出顶部加载条，无数据时走骨架。Loading; bar with old data, skeleton without. */
  loading?: boolean;
  /** 是否已有（哪怕是旧的）数据。Whether any (even stale) rows exist. */
  hasData: boolean;
  /** 骨架列数 / 行数。Skeleton column and row counts. */
  skeletonColumns?: number;
  skeletonRows?: number;
  /** 错误信息；留在卡片内并提供重试。Error message kept inside the card with retry. */
  error?: string | null;
  onRetry?: () => void;
  /** 无数据时的空态内容。Empty-state slot when there is no data. */
  empty?: ReactNode;
  /** 分页；不传则不渲染底部分页栏。Pagination; the footer bar renders only when provided. */
  pagination?: TableCardPagination;
  className?: string;
  /** 包内容层的附加类（例如接管滚动）。Extra class for the content wrapper (e.g. scrolling). */
  bodyClassName?: string;
  children?: ReactNode;
}

export function TableCard({
  loading = false,
  hasData,
  skeletonColumns = 5,
  skeletonRows = 6,
  error,
  onRetry,
  empty,
  pagination,
  className,
  bodyClassName,
  children,
}: TableCardProps) {
  const t = useTranslations('monitoring');
  const skeletonVisible = useSkeletonVisible(loading, hasData);
  return (
    <div
      className={cn(
        'flex min-h-0 flex-1 flex-col overflow-hidden border border-(--border) bg-card',
        className,
      )}
      style={{borderRadius: 'var(--radius-panel)'}}
      aria-busy={loading || undefined}
    >
      {/* 刷新时旧数据保持可读可点，只亮顶部加载条（§4.4 一次等待只有一种提示）。
          On refresh old data stays interactive; only the hairline bar shows. */}
      <TableLoadingBar loading={loading && hasData} />
      {skeletonVisible ? (
        <div className='min-h-0 flex-1 overflow-hidden px-3 py-2'>
          <table className='w-full table-fixed'>
            <tbody>
              <TableSkeletonRows
                columns={skeletonColumns}
                rows={skeletonRows}
              />
            </tbody>
          </table>
        </div>
      ) : hasData ? (
        <div className={cn('flex min-h-0 flex-1 flex-col', bodyClassName)}>
          {/* 有旧数据时错误降为顶部提示条，数据保持原样。With old data the error degrades to a strip. */}
          {error ? (
            <div
              className='flex items-center gap-2 border-b border-(--border) px-4 py-2 text-xs text-destructive'
              role='alert'
            >
              <span className='min-w-0 flex-1 truncate' title={error}>
                {error}
              </span>
              {onRetry ? (
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={onRetry}
                >
                  {t('retry')}
                </Button>
              ) : null}
            </div>
          ) : null}
          {children}
        </div>
      ) : error ? (
        // 无数据：错误态占满卡片，附重试。No data: the error owns the card, with retry.
        <div
          className='flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-6'
          role='alert'
        >
          <span className='max-w-md text-center text-sm text-destructive'>
            {error}
          </span>
          {onRetry ? (
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={onRetry}
            >
              {t('retry')}
            </Button>
          ) : null}
        </div>
      ) : (
        <div className='flex min-h-0 flex-1 items-center justify-center'>
          {empty}
        </div>
      )}
      {pagination ? (
        <div className='shrink-0 border-t bg-muted/10 px-3 py-2'>
          <Pagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            pageSize={pagination.pageSize}
            totalItems={pagination.totalItems}
            onPageChange={pagination.onPageChange}
            onPageSizeChange={pagination.onPageSizeChange}
            pageSizeOptions={pagination.pageSizeOptions}
          />
        </div>
      ) : null}
    </div>
  );
}
