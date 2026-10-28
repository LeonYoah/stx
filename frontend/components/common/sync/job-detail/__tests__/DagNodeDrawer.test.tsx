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

// 节点抽屉：作业列表轮询会换掉 tablePaths 数组引用，用户选的表和搜索词不能因此被重置。
// Node drawer: job list polling replaces the tablePaths array, which must not reset the user's table or search text.

import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import type {ReactNode} from 'react';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {normalizeJobDag, type DagVertex} from '../dag-model';
import {DagNodeDrawer} from '../DagNodeDrawer';

vi.mock('sonner', () => ({toast: {success: vi.fn()}}));

function sourceVertex(): DagVertex {
  return normalizeJobDag({
    vertexInfoMap: [
      {
        vertexId: 1,
        type: 'source',
        vertexName: 'pipeline-1 [Source[0]-Postgres-CDC]',
        tablePaths: ['db.public.mt_customers', 'db.public.mt_orders'],
        tableColumns: {'db.public.mt_customers': ['id', 'name'], 'db.public.mt_orders': ['id', 'channel']},
      },
    ],
  }).vertices[0];
}

const wrap = (node: ReactNode) => (
  <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
    {node}
  </NextIntlClientProvider>
);

describe('DagNodeDrawer', () => {
  it('keeps the picked table and search text when polling replaces the vertex object', () => {
    const props = {mode: 'plan' as const, full: false, onToggleFull: vi.fn(), onClose: vi.fn()};
    const {rerender} = render(wrap(<DagNodeDrawer vertex={sourceVertex()} {...props} />));
    const labels = zh.workbenchStudio.jobDetail;

    fireEvent.change(screen.getByPlaceholderText(labels.searchTables), {target: {value: 'orders'}});
    fireEvent.click(screen.getByText('db.public.mt_orders'));
    expect(screen.getByText('channel')).toBeTruthy();

    // 同样的表集合、新的对象引用（模拟一次轮询）。/ Same table set, new object references (one polling tick).
    rerender(wrap(<DagNodeDrawer vertex={sourceVertex()} {...props} />));
    expect(screen.getByText('channel')).toBeTruthy();
    expect(screen.queryByText('name')).toBeNull();
  });
});
