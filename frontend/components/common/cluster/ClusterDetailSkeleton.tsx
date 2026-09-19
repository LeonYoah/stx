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
 * Cluster Detail Skeleton Component
 * 集群详情骨架屏组件
 *
 * Provides a zero-collapse visual loading state for the cluster detail page.
 * 为集群详情页提供零塌陷视觉加载状态，避免高度崩塌闪烁。
 */

import {Card, CardContent, CardHeader} from '@/components/ui/card';
import {Skeleton} from '@/components/ui/skeleton';

export function ClusterDetailSkeleton() {
  return (
    <div className='space-y-6 animate-pulse'>
      {/* 头部骨架 / Header skeleton */}
      <div className='flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex items-center gap-2.5'>
          {/* 主题图标占位 / Theme icon placeholder */}
          <Skeleton className='size-9 shrink-0 rounded-lg' />
          <div className='space-y-1.5'>
            <div className='flex items-center gap-2'>
              {/* 标题占位 / Title placeholder */}
              <Skeleton className='h-6 w-44' />
              {/* 状态徽章占位 / Status badge placeholder */}
              <Skeleton className='h-5 w-16 rounded-full' />
              <Skeleton className='h-5 w-14 rounded-full' />
            </div>
            {/* 副标题与面包屑占位 / Subtitle and breadcrumbs placeholder */}
            <Skeleton className='h-3.5 w-64' />
          </div>
        </div>
        {/* 操作按钮组占位 / Action buttons group placeholder */}
        <div className='flex items-center gap-2 flex-wrap'>
          <Skeleton className='h-8 w-16 rounded-md' />
          <Skeleton className='h-8 w-16 rounded-md' />
          <Skeleton className='h-8 w-20 rounded-md' />
          <Skeleton className='h-8 w-16 rounded-md' />
        </div>
      </div>

      {/* 状态胶囊栏骨架 / StatPillsBar skeleton */}
      <div className='flex items-center gap-2 flex-wrap'>
        <Skeleton className='h-7 w-20 rounded-md' />
        <Skeleton className='h-7 w-24 rounded-md' />
        <Skeleton className='h-7 w-20 rounded-md' />
        <Skeleton className='h-7 w-20 rounded-md' />
        <Skeleton className='h-7 w-24 rounded-md' />
      </div>

      {/* 标签栏骨架 / TabsList skeleton */}
      <div className='flex items-center gap-1 p-1 bg-muted/40 rounded-xl w-full max-w-2xl'>
        <Skeleton className='h-8 w-16 rounded-lg' />
        <Skeleton className='h-8 w-16 rounded-lg' />
        <Skeleton className='h-8 w-16 rounded-lg' />
        <Skeleton className='h-8 w-16 rounded-lg' />
        <Skeleton className='h-8 w-16 rounded-lg' />
        <Skeleton className='h-8 w-16 rounded-lg' />
      </div>

      {/* 主卡片骨架（概览网格 / 表格占位） / Main card skeleton */}
      <Card className='border rounded-xl bg-card/40 shadow-xs'>
        <CardHeader className='pb-4'>
          <Skeleton className='h-5 w-32' />
          <Skeleton className='h-3.5 w-60' />
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
            {Array.from({length: 8}).map((_, i) => (
              <div key={i} className='space-y-2 p-3 rounded-lg border bg-muted/10'>
                <Skeleton className='h-3 w-20' />
                <Skeleton className='h-5 w-32' />
              </div>
            ))}
          </div>
          <div className='pt-4 border-t border-border/40'>
            <Skeleton className='h-32 w-full rounded-lg' />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
