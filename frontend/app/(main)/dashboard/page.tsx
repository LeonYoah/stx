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
import {HomeDashboard} from '@/components/common/dashboard';

/**
 * 控制台首页：页面只做装配；首页状态（页签、范围、列表筛选）在 URL 上，需要 Suspense 包住 useSearchParams。
 * Console home: the page only assembles; home state (tab, scope, list filters) lives in the URL, so useSearchParams
 * needs a Suspense boundary.
 */
export default function DashboardPage() {
  return (
    <Suspense>
      <HomeDashboard />
    </Suspense>
  );
}
