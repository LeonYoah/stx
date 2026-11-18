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

import {useCallback, useState} from 'react';
import {DEFAULT_JOB_LIST_QUERY, type JobListQuery} from './types';

/**
 * 本地查询状态（工作台宿主用）：与 URL 版同样的 patch 语义（浅合并），筛选重置页码由工具栏内部处理；
 * 切换任务时宿主调 resetQuery 回到默认值。
 * Local query state for the workbench host: same shallow-merge patch semantics as the URL version (the toolbar
 * resets the page itself); the host calls resetQuery when the task changes.
 */
export function useJobTableQuery(initial?: Partial<JobListQuery>): {
  query: JobListQuery;
  patchQuery: (patch: Partial<JobListQuery>) => void;
  resetQuery: (next?: Partial<JobListQuery>) => void;
} {
  const [query, setQuery] = useState<JobListQuery>({
    ...DEFAULT_JOB_LIST_QUERY,
    ...initial,
  });
  const patchQuery = useCallback((patch: Partial<JobListQuery>) => {
    setQuery((prev) => ({...prev, ...patch}));
  }, []);
  const resetQuery = useCallback((next?: Partial<JobListQuery>) => {
    setQuery({...DEFAULT_JOB_LIST_QUERY, ...next});
  }, []);
  return {query, patchQuery, resetQuery};
}
