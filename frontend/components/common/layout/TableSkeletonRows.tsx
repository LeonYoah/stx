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

import React from 'react';
import {TableCell, TableRow} from '@/components/ui/table';
import {Skeleton} from '@/components/ui/skeleton';

export interface TableSkeletonRowsProps {
  // 表格列数
  // Number of table columns
  columns: number;
  // 渲染骨架行数（默认 5 行）
  // Number of skeleton rows to render (defaults to 5)
  rows?: number;
  // 行高样式（默认 h-10）
  // Row height class name (defaults to h-10)
  rowHeight?: string;
}

// 通用表格骨架行组件（初次无数据加载时瞬间撑开表格真实尺寸，杜绝 CLS 布局抖动）
// Universal table skeleton rows component (instantly establishes table height on initial load to eliminate CLS)
export function TableSkeletonRows({
  columns,
  rows = 5,
  rowHeight = 'h-10',
}: TableSkeletonRowsProps) {
  return (
    <>
      {Array.from({length: rows}).map((_, rowIndex) => (
        <TableRow key={rowIndex} className={`${rowHeight} hover:bg-transparent border-b/50`}>
          {Array.from({length: columns}).map((_, colIndex) => (
            <TableCell key={colIndex} className='py-2.5 px-3'>
              <Skeleton
                className={`h-4 ${
                  colIndex === 0
                    ? 'w-[75%]'
                    : colIndex === columns - 1
                      ? 'w-12 ml-auto'
                      : 'w-[60%]'
                }`}
              />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  );
}
