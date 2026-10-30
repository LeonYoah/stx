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

import {act, fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import type {ReactNode} from 'react';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {normalizeJobDag, type DagVertex} from '../dag-model';
import {DagNodeDrawer} from '../DagNodeDrawer';

vi.mock('sonner', () => ({toast: {success: vi.fn()}}));

// Radix Popper 依赖 ResizeObserver，jsdom 未提供 / Radix Popper needs ResizeObserver, which jsdom lacks
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

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

  it('keeps the plan-DAG refresh control visible when available=true', async () => {
    const onRefresh = vi.fn();
    const labels = zh.workbenchStudio.jobDetail;
    render(
      wrap(
        <DagNodeDrawer
          vertex={sourceVertex()}
          mode='runtime'
          full={false}
          onToggleFull={vi.fn()}
          onClose={vi.fn()}
          plan={{loading: false, available: true, onRefresh}}
        />,
      ),
    );

    // Open schema via a table click (same path as production) so TabsContent mounts.
    // 通过点击表名进入表结构页签（与线上一致），确保 TabsContent 挂载。
    fireEvent.click(screen.getByText('db.public.mt_customers'));
    // 选中态由 Tabs 控制，不能被 Tooltip 的 data-state 覆盖 / The selected tab keeps Tabs' data-state
    expect(screen.getByTestId('dag-node-tab-schema')).toHaveAttribute('data-state', 'active');
    // 说明不再常驻，收进选表行末尾的 HelpHint / The hint moved from the panel into a HelpHint on the picker row
    expect(screen.queryByText(labels.schemaLiveHint)).toBeNull();
    const hint = screen.getByTestId('dag-node-schema-hint');
    await act(async () => {
      hint.focus();
      fireEvent.focus(hint);
    });
    expect(await screen.findByRole('tooltip')).toHaveTextContent(labels.schemaLiveHint);
    const refresh = screen.getByTestId('plan-dag-refresh');
    expect(refresh).toBeTruthy();
    fireEvent.click(refresh);
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
  it('marks the selected schema / plan tab active and only renders hints that have content', async () => {
    const labels = zh.workbenchStudio.jobDetail;
    const sink = normalizeJobDag({
      vertexInfoMap: [
        {vertexId: 2, type: 'sink', vertexName: 'pipeline-1 [Sink[0]-Jdbc]', tablePaths: ['db.public.t1'], tableColumns: {'db.public.t1': ['id']}},
      ],
    }).vertices[0];
    render(wrap(<DagNodeDrawer vertex={sink} mode='plan' full={false} onToggleFull={vi.fn()} onClose={vi.fn()} />));

    const schemaTab = screen.getByTestId('dag-node-tab-schema');
    const planTab = screen.getByTestId('dag-node-tab-plan');
    // Radix Tabs 在 mousedown 时切换 / Radix Tabs switches on mousedown
    fireEvent.mouseDown(schemaTab, {button: 0});
    expect(schemaTab).toHaveAttribute('data-state', 'active');
    expect(planTab).toHaveAttribute('data-state', 'inactive');
    // 计划态没有表结构说明：不渲染问号，页签也不带 describedby / No schema hint in plan mode: no icon, no describedby
    expect(screen.queryByTestId('dag-node-schema-hint')).toBeNull();
    expect(schemaTab).not.toHaveAttribute('aria-describedby');

    fireEvent.mouseDown(planTab, {button: 0});
    expect(planTab).toHaveAttribute('data-state', 'active');
    expect(schemaTab).toHaveAttribute('data-state', 'inactive');
    const planHint = screen.getByTestId('dag-node-plan-hint');
    await act(async () => {
      planHint.focus();
      fireEvent.focus(planHint);
    });
    expect(await screen.findByRole('tooltip')).toHaveTextContent(labels.planPreviewHint);
  });
});
