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

import {Suspense} from 'react';
import {Metadata} from 'next';
import InspectionDetailPage from '@/components/common/diagnostics/InspectionDetailPage';
import {Skeleton} from '@/components/ui/skeleton';

export const metadata: Metadata = {
  title: '巡检详情',
};

interface Props {
  params: Promise<{id: string}>;
}

export default async function Page({params}: Props) {
  const {id} = await params;
  const inspectionId = parseInt(id, 10);
  return (
    <Suspense
      fallback={
        // 保留报告首屏占位，避免路由加载时版面跳动。
        // Reserve the report shape during route loading to prevent layout shifts.
        <div className='space-y-6' aria-busy='true'>
          <Skeleton className='h-8 w-40' />
          <Skeleton className='h-44 w-full' />
          <Skeleton className='h-56 w-full' />
        </div>
      }
    >
      <InspectionDetailPage inspectionId={inspectionId} />
    </Suspense>
  );
}
