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

import {fireEvent, render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeAll, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  SyncJobInstance,
  SyncPreviewDataset,
  SyncPreviewSnapshot,
} from '@/lib/services/sync';
import {PreviewWorkspacePanel} from '../CheckpointPanels';

vi.mock('@monaco-editor/react', () => ({
  default: () => null,
}));

// jsdom 没有布局，beUI Table 的虚拟列表量不出任何可见行；测试里把虚拟项展开成全部行。
// jsdom has no layout, so the beUI table virtualizer measures zero visible rows;
// expand every row in tests.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({
    count,
    estimateSize,
  }: {
    count: number;
    estimateSize: () => number;
  }) => ({
    getVirtualItems: () =>
      Array.from({length: count}, (_, index) => ({
        index,
        key: index,
        start: index * estimateSize(),
        end: (index + 1) * estimateSize(),
        size: estimateSize(),
      })),
    getTotalSize: () => count * estimateSize(),
  }),
}));

beforeAll(() => {
  window.matchMedia = vi.fn().mockReturnValue({
    matches: true,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }) as never;
});

function previewJob(): SyncJobInstance {
  return {
    id: 41,
    task_id: 11,
    task_version: 1,
    run_type: 'preview',
    platform_job_id: 'p-41',
    engine_job_id: 'p-41',
    status: 'finished',
    submit_spec: {cluster_id: 1},
    result_preview: {},
    error_message: '',
    created_by: 1,
    created_at: '2026-02-01T00:00:00Z',
  } as unknown as SyncJobInstance;
}

function previewSnapshot(overrides: Partial<SyncPreviewSnapshot> = {}) {
  return {
    session_id: 1,
    job_instance_id: 41,
    platform_job_id: 'p-41',
    engine_job_id: 'p-41',
    status: 'finished',
    row_limit: 10000,
    total_rows: 2,
    table_count: 1,
    truncated: false,
    tables: [],
    ...overrides,
  } as SyncPreviewSnapshot;
}

function renderPreview(
  overrides: Partial<Parameters<typeof PreviewWorkspacePanel>[0]> = {},
) {
  const datasets: SyncPreviewDataset[] = [
    {
      name: 'db.t1',
      columns: ['id', 'name'],
      rows: [
        {id: 2, name: 'beta'},
        {id: 1, name: 'alpha'},
      ],
      page_size: 20,
      total: 2,
    },
  ];
  const props = {
    job: previewJob(),
    previewSnapshot: null,
    datasets,
    selectedDatasetName: 'db.t1',
    loading: false,
    monacoTheme: 'vs-dark',
    onSelectDataset: vi.fn(),
    ...overrides,
  };
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <PreviewWorkspacePanel {...props} />
    </NextIntlClientProvider>,
  );
  return props;
}

function firstCellTexts() {
  return within(screen.getByRole('table'))
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('cell')[0]?.textContent);
}

describe('PreviewWorkspacePanel', () => {
  it('keeps table chrome when there is no preview job', () => {
    renderPreview({job: null, datasets: []});
    expect(screen.getByText(zh.workbenchStudio.noPreviewJobs)).toBeTruthy();
  });

  it('shows the row count in the footer and sorts preview columns', () => {
    renderPreview();
    expect(firstCellTexts()).toEqual(['2', '1']);

    const rowCount = screen.getByText(
      zh.workbenchStudio.previewRowCount.replace('{count}', '2'),
    );
    expect(rowCount.closest('.studio-data-table__footer')).toBeTruthy();

    // beUI 排序按钮的可访问名就是列名。 / The beUI sort button's accessible name is the column name.
    fireEvent.click(screen.getByRole('button', {name: 'id'}));
    expect(firstCellTexts()).toEqual(['1', '2']);
  });

  it('keeps the table header while preview data is loading', () => {
    renderPreview({
      loading: true,
      datasets: [
        {name: 'db.t1', columns: ['id', 'name'], rows: [], page_size: 20, total: 0},
      ],
    });
    expect(
      screen.getByLabelText(zh.workbenchStudio.dataTable).getAttribute('aria-busy'),
    ).toBe('true');
    // beUI 骨架行是 animate-pulse 占位条，不是 data-slot="skeleton"。
    // beUI skeleton rows are animate-pulse bars, not data-slot="skeleton".
    expect(document.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', {name: 'id'})).toBeTruthy();
    expect(screen.queryByText(zh.workbenchStudio.preparingPreview)).toBeNull();
  });

  it('distinguishes NULL from empty strings', () => {
    renderPreview({
      datasets: [
        {
          name: 'db.t1',
          columns: ['id', 'name'],
          rows: [{id: null, name: ''}],
          page_size: 20,
          total: 1,
        },
      ],
    });
    expect(screen.getByText('NULL')).toBeTruthy();
    expect(
      screen.getByText(zh.workbenchStudio.previewEmptyString),
    ).toBeTruthy();
  });

  it('shows the row-limit hint when the preview snapshot is truncated', () => {
    renderPreview({
      previewSnapshot: previewSnapshot({truncated: true, row_limit: 20}),
    });
    expect(
      screen.getByText(
        zh.workbenchStudio.previewRowLimitReached.replace('{limit}', '20'),
      ),
    ).toBeTruthy();
  });
});
