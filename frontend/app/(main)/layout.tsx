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

import {ManagementBar} from '@/components/common/layout/ManagementBar';
import {memo} from 'react';

const MemoizedManagementBar = memo(ManagementBar);

export default function ProjectLayout({children}: {children: React.ReactNode}) {
  return (
    <div className='min-h-screen flex flex-col'>
      <MemoizedManagementBar />
      <div className='flex flex-1 flex-col'>
        <div className='@container/main flex flex-1 flex-col gap-2'>
          {/* Main content container: keep all pages consistent width/padding */}
          <div className='flex flex-col gap-4 mb-8 w-full max-w-none px-3 sm:px-4 lg:px-6 xl:px-8 2xl:px-10 py-6 md:gap-6'>
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
