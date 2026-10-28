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

// 运行记录行：点行任意处打开详情并选中作业；行内只保留详情 / 恢复 / 保存点停止 / 停止；操作可用性与详情共用一套规则。
// Job run rows: clicking anywhere opens the detail and selects the job; only details / recover / savepoint stop / stop
// stay inline; action availability shares one rule set with the detail.

import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {SyncJobInstance} from '@/lib/services/sync';
import {JobRunsPanel} from '../../ConsolePanel';
import {getJobActionState} from '../../sync-studio-utils';

function job(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    id: 26,
    task_id: 11,
    task_version: 3,
    run_type: 'run',
    platform_job_id: '179312051430100000',
    engine_job_id: '179312051430100000',
    status: 'running',
    submit_spec: {cluster_id: 1},
    result_preview: {
      job_dag: {vertexInfoMap: [{vertexId: 1, type: 'source'}]},
      metrics: {
        TableSinkWriteCount: {'Sink[0].db.t1': '1', 'Sink[0].db.t2': '2'},
      },
    },
    error_message: '',
    created_by: 1,
    created_at: '',
    ...partial,
  } as SyncJobInstance;
}

function renderPanel(jobs: SyncJobInstance[], overrides: Partial<Parameters<typeof JobRunsPanel>[0]> = {}) {
  const props = {
    jobs,
    selectedJobId: null,
    currentUserId: 1,
    onSelectJob: vi.fn(),
    onRecover: vi.fn(),
    onCancel: vi.fn(),
    onSavepointStop: vi.fn(),
    onOpenDetail: vi.fn(),
    disableRecover: false,
    ...overrides,
  };
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <JobRunsPanel {...props} />
    </NextIntlClientProvider>,
  );
  return props;
}

describe('JobRunsPanel', () => {
  it('opens the detail and selects the job when the row is clicked anywhere', () => {
    const props = renderPanel([job({})]);
    fireEvent.click(screen.getByText('#26'));
    expect(props.onSelectJob).toHaveBeenCalledWith(26);
    expect(props.onOpenDetail).toHaveBeenCalledWith(expect.objectContaining({id: 26}));
  });

  it('keeps only details and quick actions inline for a running job', () => {
    const props = renderPanel([job({})]);
    const labels = zh.workbenchStudio;
    expect(screen.getByRole('button', {name: labels.viewJobDetail})).toBeTruthy();
    expect(screen.getByRole('button', {name: labels.savepointStop})).toBeTruthy();
    expect(screen.getByRole('button', {name: labels.stop})).toBeTruthy();
    // 只读入口（事件、脚本、指标、拓扑）已收进详情。/ Read-only entries (events, script, metrics, topology) moved into the detail.
    expect(screen.getAllByRole('button')).toHaveLength(4);
    expect((screen.getByRole('button', {name: labels.recover}) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByRole('button', {name: labels.stop}));
    expect(props.onCancel).toHaveBeenCalledWith(26);
    // 行内按钮不能冒泡成打开详情。/ Inline buttons must not bubble into opening the detail.
    expect(props.onOpenDetail).not.toHaveBeenCalled();
  });

  it('opens the metrics tab from the multi-table badge', () => {
    const props = renderPanel([job({})]);
    fireEvent.click(screen.getByText(zh.workbenchStudio.multiTableBadge.replace('{count}', '2')));
    expect(props.onOpenDetail).toHaveBeenCalledWith(expect.objectContaining({id: 26}), 'metrics');
  });
});

describe('getJobActionState', () => {
  const ctx = {isAdmin: false, isOwner: false, canRun: true, currentUserId: 1, disableRecover: false};

  it('lets the initiator stop an active job but not recover it', () => {
    expect(getJobActionState(job({}), ctx)).toEqual({showRecover: true, canRecover: false, showStop: true, canStop: true});
  });

  it('blocks stopping for other users and recovering previews', () => {
    expect(getJobActionState(job({created_by: 2}), ctx).canStop).toBe(false);
    expect(getJobActionState(job({run_type: 'preview', status: 'failed'}), ctx).showRecover).toBe(false);
  });

  it('allows recovering a terminal cluster job unless runs are disabled', () => {
    const failed = job({status: 'failed'});
    expect(getJobActionState(failed, ctx)).toMatchObject({showStop: false, canRecover: true});
    expect(getJobActionState(failed, {...ctx, disableRecover: true}).canRecover).toBe(false);
    expect(getJobActionState(failed, {...ctx, canRun: false}).canRecover).toBe(false);
  });
});
