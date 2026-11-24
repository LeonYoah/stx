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

// 列表行瘦身后的摘要契约：isFullJobRecord 区分摘要 / 全量，mergeJobSummaryWithFull 做字段级合并，
// buildCheckpointInspectJobConfig 只认完整记录里的提交脚本（编辑器脚本仅作无提交内容时的兜底）。
// The post-slimming summary contract: isFullJobRecord tells summary from full record,
// mergeJobSummaryWithFull merges field-wise, and buildCheckpointInspectJobConfig trusts the submitted
// script from a full record (the editor script is only the no-submitted-content fallback).

import {describe, expect, it} from 'vitest';
import type {
  SyncJobInstance,
  SyncJobSummary,
} from '@/lib/services/sync';
import {
  buildCheckpointInspectJobConfig,
  EMPTY_EDITOR,
  isFullJobRecord,
  mergeJobSummaryWithFull,
  type EditorState,
} from '../sync-studio-utils';

const T0 = '2026-01-01T00:00:00Z';
const T1 = '2026-01-01T00:00:01Z';
const T2 = '2026-01-01T00:00:02Z';

function summary(partial: Partial<SyncJobSummary>): SyncJobSummary {
  return {
    id: 1,
    task_id: 11,
    run_type: 'run',
    status: 'running',
    submit_spec: {job_name: 'job-1'},
    result_preview: {job_status: 'RUNNING'},
    error_message: '',
    created_by: 1,
    created_at: T0,
    updated_at: T0,
    ...partial,
  } as SyncJobSummary;
}

function full(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    ...summary({}),
    submit_spec: {job_name: 'job-1', submitted_content: 'env { parallel = 1 }'},
    result_preview: {
      job_status: 'RUNNING',
      job_dag: {vertices: [{id: 'v1'}]},
      metrics: {SourceReceivedCount: 10},
    },
    ...partial,
  } as SyncJobInstance;
}

function editor(partial: Partial<EditorState> = {}): EditorState {
  return {
    ...EMPTY_EDITOR,
    content: 'env { edited = true }',
    contentFormat: 'hocon',
    ...partial,
  };
}

describe('isFullJobRecord', () => {
  it('摘要行（白名单键）不算完整记录', () => {
    expect(isFullJobRecord(summary({}))).toBe(false);
    expect(isFullJobRecord(null)).toBe(false);
    expect(
      isFullJobRecord(
        summary({
          result_preview: {
            job_status: 'RUNNING',
            cancel_outcome: 'unknown',
            cancel_timeout_seconds: 60,
          },
        }),
      ),
    ).toBe(false);
  });

  it('带重字段的行视为完整记录', () => {
    expect(isFullJobRecord(full({}))).toBe(true);
    // 只有 submitted_content 也够（提交前失败的作业可能没有任何指标）。/ submitted_content alone
    // suffices — a job that failed before submitting may carry no metrics at all.
    expect(
      isFullJobRecord(
        summary({
          submit_spec: {
            job_name: 'job-1',
            submitted_content: 'env {}',
          } as SyncJobInstance['submit_spec'],
        }),
      ),
    ).toBe(true);
  });
});

describe('mergeJobSummaryWithFull', () => {
  it('无全量时返回摘要视图', () => {
    const merged = mergeJobSummaryWithFull(summary({status: 'running'}), null);
    expect(merged.status).toBe('running');
    expect(merged.result_preview?.job_dag).toBeUndefined();
  });

  it('摘要更新时轻字段取行的、全量重字段保留', () => {
    const merged = mergeJobSummaryWithFull(
      summary({
        status: 'canceled',
        updated_at: T2,
        live: {read_total: 5},
        result_preview: {job_status: 'CANCELED'},
      }),
      full({updated_at: T1}),
    );
    expect(merged.status).toBe('canceled');
    expect(merged.live).toEqual({read_total: 5});
    expect(merged.result_preview?.job_status).toBe('CANCELED');
    expect(merged.result_preview?.job_dag).toEqual({vertices: [{id: 'v1'}]});
    expect(merged.submit_spec?.submitted_content).toBe(
      'env { parallel = 1 }',
    );
  });

  it('摘要较新时其白名单键集合权威：缺失的键视为已清除，不残留全量旧值', () => {
    // 重试停止时后端清空 cancel_outcome，新摘要不再带该键；详情不能继续显示旧的 reverted 标记。
    // A stop retry clears cancel_outcome server-side so the newer summary omits it; the detail must not
    // keep showing the stale reverted badge.
    const merged = mergeJobSummaryWithFull(
      summary({
        updated_at: T2,
        submit_spec: {job_name: 'job-1'},
        result_preview: {job_status: 'RUNNING'},
      }),
      full({
        updated_at: T1,
        submit_spec: {
          job_name: 'job-1',
          submit_outcome: 'confirming',
          submitted_content: 'env { parallel = 1 }',
        },
        result_preview: {
          job_status: 'RUNNING',
          cancel_outcome: 'reverted',
          job_dag: {vertices: [{id: 'v1'}]},
        },
      }),
    );
    expect(merged.result_preview).not.toHaveProperty('cancel_outcome');
    expect(merged.submit_spec).not.toHaveProperty('submit_outcome');
    expect(merged.result_preview?.job_dag).toEqual({vertices: [{id: 'v1'}]});
    expect(merged.submit_spec?.submitted_content).toBe('env { parallel = 1 }');
  });

  it('摘要比全量旧时全量整体胜出，重字段仍在', () => {
    const merged = mergeJobSummaryWithFull(
      summary({status: 'canceled', updated_at: T0}),
      full({status: 'running', updated_at: T2}),
    );
    expect(merged.status).toBe('running');
    expect(merged.updated_at).toBe(T2);
    expect(merged.result_preview?.job_dag).toBeTruthy();
    expect(merged.submit_spec?.submitted_content).toBeTruthy();
  });
});

describe('buildCheckpointInspectJobConfig', () => {
  it('有提交内容时用提交脚本，不用编辑器当前脚本', () => {
    const config = buildCheckpointInspectJobConfig(
      full({
        submit_spec: {
          submitted_content: 'env { submitted = true }',
          submitted_format: 'hocon',
        },
      }),
      editor({content: 'env { edited = true }'}),
      [],
    );
    expect(config?.content).toBe('env { submitted = true }');
    expect(config?.content_format).toBe('hocon');
  });

  it('完整记录确实没有 submitted_content 时才回退编辑器脚本', () => {
    const config = buildCheckpointInspectJobConfig(
      full({submit_spec: {job_name: 'job-1'}, result_preview: {}}),
      editor({content: 'env { edited = true }'}),
      [],
    );
    expect(config?.content).toBe('env { edited = true }');
  });

  it('编辑器为空且无提交内容时不构造 job_config', () => {
    const config = buildCheckpointInspectJobConfig(
      full({submit_spec: {job_name: 'job-1'}, result_preview: {}}),
      editor({content: ''}),
      [],
    );
    expect(config).toBeUndefined();
  });
});
