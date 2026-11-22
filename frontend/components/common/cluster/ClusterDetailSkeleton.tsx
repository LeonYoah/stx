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
 * 集群详情骨架屏：匹配重构后的 6 Tab 单层扁平画布结构，零塌陷无抖动。
 * Cluster detail skeleton matching the refactored 6-tab flat single-layer layout.
 */
import {Skeleton} from '@/components/ui/skeleton';

export function ClusterDetailSkeleton() {
  return (
    <div className='arc-compact space-y-4' data-testid='cluster-detail-skeleton'>
      {/* 头部骨架 / Header skeleton */}
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex items-center gap-3'>
          <Skeleton className='size-9 shrink-0 rounded-lg' />
          <div className='space-y-1.5'>
            <div className='flex items-center gap-2'>
              <Skeleton className='h-5 w-40' />
              <Skeleton className='h-5 w-16 rounded-md' />
              <Skeleton className='h-5 w-14 rounded-md' />
            </div>
            <Skeleton className='h-3.5 w-52' />
          </div>
        </div>
        <div className='flex items-center gap-2'>
          <Skeleton className='h-8 w-20 rounded-(--radius-control)' />
          <Skeleton className='h-8 w-20 rounded-(--radius-control)' />
          <Skeleton className='h-8 w-8 rounded-(--radius-control)' />
        </div>
      </div>

      {/* 6 个一级 Tab 骨架 / 6 primary tabs skeleton */}
      <div className='flex items-center gap-1.5'>
        {Array.from({length: 6}).map((_, i) => (
          <Skeleton key={i} className='h-8 w-16 rounded-(--radius-control)' />
        ))}
      </div>

      {/* 单层扁平画布骨架 / Flat single-layer canvas skeleton */}
      <div className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'>
        <div className='space-y-4 pb-5'>
          <Skeleton className='h-4 w-28' />
          <div className='grid gap-3 md:grid-cols-3'>
            <Skeleton className='h-9 w-full md:col-span-2' />
            <Skeleton className='h-9 w-full' />
          </div>
          <div className='grid grid-cols-2 gap-4 pt-1 md:grid-cols-4'>
            {Array.from({length: 4}).map((_, i) => (
              <div key={i} className='space-y-1.5'>
                <Skeleton className='h-3 w-16' />
                <Skeleton className='h-4 w-24' />
              </div>
            ))}
          </div>
        </div>
        <div className='space-y-3 border-t-[0.5px] border-border/80 py-5'>
          <Skeleton className='h-4 w-32' />
          <Skeleton className='h-10 w-full' />
          <Skeleton className='h-10 w-full' />
        </div>
      </div>
    </div>
  );
}
