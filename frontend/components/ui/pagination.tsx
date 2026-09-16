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

/**
 * Pagination Component
 * 分页组件
 */

'use client';

import * as React from 'react';
import { useTranslations } from 'next-intl';
import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

interface PaginationProps {
  currentPage: number;
  totalPages: number;
  pageSize: number;
  totalItems: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  pageSizeOptions?: number[];
  showPageSizeSelector?: boolean;
  showTotalItems?: boolean;
  className?: string;
}

/**
 * Pagination Component
 * 分页组件 - 支持页码切换、每页数量选择与完整国际化
 */
export function Pagination({
  currentPage,
  totalPages,
  pageSize,
  totalItems,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50, 100],
  showPageSizeSelector = true,
  showTotalItems = true,
  className,
}: PaginationProps) {
  const t = useTranslations('common');
  const safeTotalPages = Math.max(1, totalPages || 1);

  // Generate page numbers to display / 生成要显示的页码
  const getPageNumbers = () => {
    const pages: (number | 'ellipsis')[] = [];
    const maxVisiblePages = 5;

    if (safeTotalPages <= maxVisiblePages + 2) {
      // Show all pages if total is small / 如果总页数较少则显示所有页码
      for (let i = 1; i <= safeTotalPages; i++) {
        pages.push(i);
      }
    } else {
      // Always show first page / 始终显示第一页
      pages.push(1);

      if (currentPage > 3) {
        pages.push('ellipsis');
      }

      // Show pages around current page / 显示当前页附近的页码
      const start = Math.max(2, currentPage - 1);
      const end = Math.min(safeTotalPages - 1, currentPage + 1);

      for (let i = start; i <= end; i++) {
        pages.push(i);
      }

      if (currentPage < safeTotalPages - 2) {
        pages.push('ellipsis');
      }

      // Always show last page / 始终显示最后一页
      pages.push(safeTotalPages);
    }

    return pages;
  };

  const pageNumbers = getPageNumbers();

  return (
    <div className={cn('flex items-center justify-between gap-4 flex-wrap', className)}>
      {/* Total items display / 总数显示 */}
      {showTotalItems && (
        <div className="text-sm text-muted-foreground">
          {t('totalItems', { total: totalItems })}
        </div>
      )}

      <div className="flex items-center gap-2">
        {/* Page size selector / 每页数量选择器 */}
        {showPageSizeSelector && onPageSizeChange && (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('perPage')}</span>
            <Select
              value={String(pageSize)}
              onValueChange={(value) => onPageSizeChange(Number(value))}
            >
              <SelectTrigger className="h-8 min-w-[70px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span className="text-sm text-muted-foreground">{t('itemsUnit')}</span>
          </div>
        )}

        {/* Page navigation / 页码导航 */}
        <nav className="flex items-center gap-1">
          {/* Previous button / 上一页按钮 */}
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8"
            onClick={() => onPageChange(currentPage - 1)}
            disabled={currentPage <= 1}
            title={t('prevPage')}
            aria-label={t('prevPage')}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>

          {/* Page numbers / 页码 */}
          {pageNumbers.map((page, index) => {
            if (page === 'ellipsis') {
              return (
                <span
                  key={`ellipsis-${index}`}
                  className="flex h-8 w-8 items-center justify-center"
                >
                  <MoreHorizontal className="h-4 w-4 text-muted-foreground" />
                </span>
              );
            }

            return (
              <Button
                key={page}
                variant={currentPage === page ? 'default' : 'outline'}
                size="icon"
                className="h-8 w-8"
                onClick={() => onPageChange(page)}
              >
                {page}
              </Button>
            );
          })}

          {/* Next button / 下一页按钮 */}
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8"
            onClick={() => onPageChange(currentPage + 1)}
            disabled={currentPage >= safeTotalPages}
            title={t('nextPage')}
            aria-label={t('nextPage')}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </nav>
      </div>
    </div>
  );
}
