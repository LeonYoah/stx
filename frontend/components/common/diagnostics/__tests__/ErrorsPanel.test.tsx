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

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  DiagnosticsErrorEvent,
  DiagnosticsErrorGroup,
} from '@/lib/services/diagnostics';
import {ErrorsPanel} from '../errors/ErrorsPanel';

const getErrorGroupsSafe = vi.hoisted(() => vi.fn());
const getErrorGroupDetailSafe = vi.hoisted(() => vi.fn());
const findMatchingMemories = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {
    diagnostics: {
      getErrorGroupsSafe,
      getErrorGroupDetailSafe,
    },
    troubleshooting: {findMatchingMemories},
  },
}));
vi.mock('@/lib/i18n', () => ({useLocale: () => ({locale: 'zh-CN'})}));

function group(overrides: Partial<DiagnosticsErrorGroup>): DiagnosticsErrorGroup {
  return {
    id: 1,
    fingerprint: `fp-${overrides.id ?? 1}`,
    fingerprint_version: 'v1',
    title: 'Task failed',
    exception_class: 'org.apache.seatunnel.TaskExecuteException',
    sample_message: 'job failed on worker',
    occurrence_count: 1,
    first_seen_at: '2026-11-01T08:00:00Z',
    last_seen_at: '2026-11-02T09:30:00Z',
    last_cluster_id: 1,
    last_node_id: 11,
    last_host_id: 21,
    last_host_name: 'worker-a',
    last_host_ip: '10.0.0.21',
    ...overrides,
  };
}

function event(overrides: Partial<DiagnosticsErrorEvent>): DiagnosticsErrorEvent {
  return {
    id: 100,
    error_group_id: 1,
    fingerprint: 'fp-1',
    cluster_id: 1,
    node_id: 11,
    host_id: 21,
    host_name: 'worker-a',
    host_ip: '10.0.0.21',
    agent_id: 'agent-a',
    role: 'worker',
    install_dir: '/opt/seatunnel',
    source_file: '/opt/seatunnel/logs/engine.log',
    source_kind: 'engine',
    job_id: '100',
    occurred_at: '2026-11-02T09:30:00Z',
    message: 'boom',
    exception_class: 'org.apache.seatunnel.TaskExecuteException',
    evidence: 'java.lang.RuntimeException: boom\n\tat Engine.run(Engine.java:1)',
    cursor_start: 0,
    cursor_end: 10,
    ...overrides,
  };
}

const groups: DiagnosticsErrorGroup[] = [
  group({id: 1, title: 'hot failure', occurrence_count: 15}),
  group({id: 2, title: 'warm failure', occurrence_count: 5}),
  group({id: 3, title: 'cold failure', occurrence_count: 1, last_cluster_id: 9}),
];

const clusterOptions = [
  {cluster_id: 1, cluster_name: 'stx-prod'},
  {cluster_id: 2, cluster_name: 'stx-dev'},
];

function renderPanel(props?: {
  clusterId?: number;
  groupId?: number;
  onSelectGroup?: (id: number | null) => void;
}) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <ErrorsPanel
        clusterId={props?.clusterId}
        clusterOptions={clusterOptions}
        groupId={props?.groupId}
        onSelectGroup={props?.onSelectGroup}
      />
    </NextIntlClientProvider>,
  );
}

describe('ErrorsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // JSDOM 没有 matchMedia；narrow=false 让详情走常驻栏而不是抽屉。
    // JSDOM lacks matchMedia; narrow=false keeps the detail in the persistent rail.
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });
    getErrorGroupsSafe.mockResolvedValue({
      success: true,
      data: {items: groups, total: groups.length, page: 1, page_size: 20},
    });
    getErrorGroupDetailSafe.mockImplementation((id: number) =>
      Promise.resolve({
        success: true,
        data: {
          group: groups.find((g) => g.id === id),
          events: [event({error_group_id: id})],
        },
      }),
    );
    findMatchingMemories.mockReturnValue([]);
  });

  it('renders error rows with heat chips and the solution badge', async () => {
    findMatchingMemories.mockImplementation(
      ({fingerprint}: {fingerprint: string}) =>
        fingerprint === 'fp-1' ? [{id: 'm1'}] : [],
    );
    renderPanel();
    expect(await screen.findByTestId('error-row-1')).toBeVisible();
    expect(screen.getByTestId('error-row-2')).toBeVisible();
    expect(screen.getByTestId('error-row-3')).toBeVisible();
    // 频次软徽章：≥10 15 次、中频 5 次、低频 1 次。
    // Heat chips: 15 hits high, 5 mid, 1 low.
    expect(
      within(screen.getByTestId('error-row-1')).getByText('15 次'),
    ).toBeVisible();
    expect(
      within(screen.getByTestId('error-row-1')).getByText('已有方案'),
    ).toBeVisible();
    expect(
      within(screen.getByTestId('error-row-1')).getByText(/worker-a/),
    ).toBeVisible();
  });

  it('sends min/max_occurrences when a frequency segment is selected and resets to page 1', async () => {
    getErrorGroupsSafe.mockResolvedValue({
      success: true,
      data: {items: groups, total: 45, page: 1, page_size: 20},
    });
    renderPanel();
    await screen.findByTestId('error-row-1');

    // 翻到第 2 页后切「高频」：请求带 min_occurrences=10 且回第 1 页。
    // Paginate, then pick "高频": the request carries min_occurrences=10 at page 1.
    fireEvent.click(screen.getByRole('button', {name: '2'}));
    await waitFor(() =>
      expect(getErrorGroupsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({page: 2}),
      ),
    );
    fireEvent.click(screen.getByRole('button', {name: /高频/}));
    await waitFor(() =>
      expect(getErrorGroupsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({
          page: 1,
          min_occurrences: 10,
        }),
      ),
    );

    // 「中频」带闭区间 3–9。
    // "中频" carries the closed range 3–9.
    fireEvent.click(screen.getByRole('button', {name: /中频/}));
    await waitFor(() =>
      expect(getErrorGroupsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({min_occurrences: 3, max_occurrences: 9}),
      ),
    );
  });

  it('submits the keyword on Enter', async () => {
    renderPanel();
    await screen.findByTestId('error-row-1');
    fireEvent.change(
      screen.getByPlaceholderText('搜索错误摘要、异常类、来源文件'),
      {target: {value: 'oom'}},
    );
    fireEvent.keyDown(
      screen.getByPlaceholderText('搜索错误摘要、异常类、来源文件'),
      {key: 'Enter'},
    );
    await waitFor(() =>
      expect(getErrorGroupsSafe).toHaveBeenLastCalledWith(
        expect.objectContaining({keyword: 'oom', page: 1}),
      ),
    );
  });

  it('opens the detail on row click, resolves the owning cluster, and syncs group_id', async () => {
    const onSelectGroup = vi.fn();
    renderPanel({onSelectGroup});
    fireEvent.click(await screen.findByTestId('error-row-3'));

    await waitFor(() =>
      expect(getErrorGroupDetailSafe).toHaveBeenCalledWith(3, 20, {
        cluster_id: undefined,
      }),
    );
    expect(onSelectGroup).toHaveBeenCalledWith(3);
    expect(await screen.findByTestId('detail-rail')).toBeVisible();
    // 归属集群按组自身 last_cluster_id（9）反查，未知集群回落 #9。
    // The owning cluster resolves via the group's own last_cluster_id; unknown falls back to #9.
    expect(
      within(await screen.findByTestId('error-group-detail')).getByText('#9'),
    ).toBeVisible();
    // 「前往受影响集群」不依赖筛选集群。
    // "前往受影响集群" does not depend on the filter cluster.
    expect(
      screen.getByRole('link', {name: /前往受影响集群/}),
    ).toHaveAttribute('href', '/clusters/9');
  });

  it('resolves a known cluster name in the detail', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTestId('error-row-1'));
    expect(
      within(await screen.findByTestId('error-group-detail')).getByText(
        'stx-prod',
      ),
    ).toBeVisible();
  });

  it('navigates between rows with J/K while the detail is open', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTestId('error-row-1'));
    await screen.findByTestId('detail-rail');
    await waitFor(() =>
      expect(getErrorGroupDetailSafe).toHaveBeenCalledWith(
        1,
        20,
        expect.anything(),
      ),
    );

    // J/K 监听挂在 window 上；在 body 上派发可冒泡且 target 是元素。
    // J/K listeners live on window; dispatching on body bubbles up and keeps a
    // real element as the target.
    fireEvent.keyDown(document.body, {key: 'j'});
    await waitFor(() =>
      expect(getErrorGroupDetailSafe).toHaveBeenCalledWith(
        2,
        20,
        expect.anything(),
      ),
    );
    fireEvent.keyDown(document.body, {key: 'k'});
    await waitFor(() =>
      expect(getErrorGroupDetailSafe).toHaveBeenLastCalledWith(
        1,
        20,
        expect.anything(),
      ),
    );
  });

  it('deep-links into a group detail via the groupId prop', async () => {
    const onSelectGroup = vi.fn();
    renderPanel({groupId: 2, onSelectGroup});
    await waitFor(() =>
      expect(getErrorGroupDetailSafe).toHaveBeenCalledWith(
        2,
        20,
        expect.anything(),
      ),
    );
    expect(await screen.findByTestId('detail-rail')).toBeVisible();
  });

  it('shows the error state and retries the list load', async () => {
    getErrorGroupsSafe.mockResolvedValue({success: false, error: 'boom'});
    renderPanel();
    expect(await screen.findByText('boom')).toBeVisible();

    getErrorGroupsSafe.mockResolvedValue({
      success: true,
      data: {items: groups, total: groups.length, page: 1, page_size: 20},
    });
    fireEvent.click(screen.getByRole('button', {name: '重试'}));
    expect(await screen.findByTestId('error-row-1')).toBeVisible();
  });
});
