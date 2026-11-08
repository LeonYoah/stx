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
import type {SyncJobInstance, SyncPreviewDataset} from '@/lib/services/sync';
import {PreviewWorkspacePanel} from '../CheckpointPanels';

vi.mock('@monaco-editor/react', () => ({
  default: () => null,
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
    previewPage: 1,
    loading: false,
    monacoTheme: 'vs-dark',
    onSelectDataset: vi.fn(),
    onChangePage: vi.fn(),
    ...overrides,
  };
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <PreviewWorkspacePanel {...props} />
    </NextIntlClientProvider>,
  );
  return props;
}

describe('PreviewWorkspacePanel', () => {
  it('keeps table chrome when there is no preview job', () => {
    renderPreview({job: null, datasets: []});
    expect(screen.getByText(zh.workbenchStudio.noPreviewJobs)).toBeTruthy();
  });

  it('puts pagination in the table footer and sorts preview columns', () => {
    renderPreview();
    const table = screen.getByRole('table');
    expect(screen.getByRole('navigation').closest('.studio-data-table__footer')).toBeTruthy();

    const before = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[0]?.textContent);
    expect(before).toEqual(['2', '1']);

    fireEvent.click(
      screen.getByRole('button', {
        name: zh.workbenchStudio.sortColumn.replace('{column}', 'id'),
      }),
    );
    const after = within(screen.getByRole('table'))
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[0]?.textContent);
    expect(after).toEqual(['1', '2']);
  });

  it('keeps the table header while preview data is loading', () => {
    renderPreview({
      loading: true,
      datasets: [{name: 'db.t1', columns: [], rows: [], page_size: 20, total: 0}],
      selectedDatasetName: 'db.t1',
    });
    expect(
      screen.getByLabelText(zh.workbenchStudio.dataTable).getAttribute('aria-busy'),
    ).toBe('true');
    expect(document.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0);
    expect(screen.queryByText(zh.workbenchStudio.preparingPreview)).toBeNull();
  });
});
