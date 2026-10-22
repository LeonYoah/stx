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

import {afterEach, describe, expect, it, vi} from 'vitest';
import apiClient from '../../core/api-client';
import {
  ClusterScaleService,
  getScaleInProgressDetail,
  normalizeScaleOperation,
  normalizeScalePlan,
} from '../scale.service';
import type {ScaleOperationInfo, ScalePlan} from '../scale.types';

/**
 * 构造一个数组字段全部为 null 的计划（模拟 Go nil slice）。
 * Build a plan whose array fields are all null (simulating Go nil slices).
 */
function nullArraysPlan(): ScalePlan {
  return {
    kind: 'scale_out',
    cluster_id: 1,
    deployment_mode: 'hybrid',
    cluster_version: '3.0.0',
    target: {
      host_id: 2,
      host_name: 't2',
      host_ip: '10.66.0.1',
      role: 'master/worker',
      install_dir: '/opt/seatunnel',
      hazelcast_port: 5801,
      api_port: 8080,
      worker_port: 5802,
      address: '10.66.0.1:5801',
    },
    fingerprint: 'sha256:abc',
    members_before: null,
    members_after: null,
    checks: null,
    loopback_nodes: null,
    requires_restart_ack: false,
    plugins_to_replay: [{name: 'jdbc', profile_keys: null}],
    running_jobs: null,
    running_jobs_known: true,
    requires_jobs_ack: false,
    checkpoint_storage: undefined,
    java_proxy_follow: undefined,
    steps: null,
    can_submit: true,
    blocking_reasons: null,
    warnings: null,
  } as unknown as ScalePlan;
}

describe('normalizeScalePlan / normalizeScaleOperation', () => {
  it('把 null 数组兜底为 [] / coerces null arrays to []', () => {
    const plan = normalizeScalePlan(nullArraysPlan());
    expect(plan.members_before).toEqual([]);
    expect(plan.members_after).toEqual([]);
    expect(plan.checks).toEqual([]);
    expect(plan.loopback_nodes).toEqual([]);
    expect(plan.running_jobs).toEqual([]);
    expect(plan.steps).toEqual([]);
    expect(plan.blocking_reasons).toEqual([]);
    expect(plan.warnings).toEqual([]);
    expect(plan.plugins_to_replay[0].profile_keys).toEqual([]);
    expect(plan.checkpoint_storage).toBeNull();
    expect(plan.java_proxy_follow).toBeNull();
  });

  it('规范化操作详情中的嵌套计划 / normalizes the nested plan of an operation', () => {
    const operation = normalizeScaleOperation({
      id: 7,
      node_ids: null,
      warnings: null,
      steps: null,
      plan: nullArraysPlan(),
    } as unknown as ScaleOperationInfo);
    expect(operation.node_ids).toEqual([]);
    expect(operation.warnings).toEqual([]);
    expect(operation.steps).toEqual([]);
    expect(operation.plan?.checks).toEqual([]);
  });
});

describe('ClusterScaleService', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('计划请求带 lang 参数 / plan requests carry the lang parameter', async () => {
    const post = vi
      .spyOn(apiClient, 'post')
      .mockResolvedValueOnce({data: {error_msg: '', data: nullArraysPlan()}});

    const plan = await ClusterScaleService.planScaleOut(1, {host_id: 2});

    expect(post.mock.calls[0][0]).toBe('/clusters/1/scale-out/plan');
    expect(post.mock.calls[0][1]).toEqual({host_id: 2});
    expect(post.mock.calls[0][2]?.params).toHaveProperty('lang');
    expect(plan.checks).toEqual([]);
  });

  it('R2 首次取得确认编号后复用幂等键重试一次 / R2 retries once with the same key', async () => {
    const confirmationError = Object.assign(
      new Error('confirmation required'),
      {
        data: {confirmation_required: true, confirmation_id: 'confirm-1'},
      },
    );
    const result = {execution_id: 'ex-1', operation_id: 5, operation: null};
    const post = vi
      .spyOn(apiClient, 'post')
      .mockRejectedValueOnce(confirmationError)
      .mockResolvedValueOnce({data: {error_msg: '', data: result}});

    await expect(
      ClusterScaleService.submitScaleIn(1, 3, {
        plan_fingerprint: 'sha256:abc',
        jobs_risk_ack: true,
      }),
    ).resolves.toEqual(result);

    expect(post).toHaveBeenCalledTimes(2);
    expect(post.mock.calls[0][0]).toBe('/clusters/1/nodes/3/scale-in');
    const first = post.mock.calls[0][2]?.headers as Record<string, string>;
    const second = post.mock.calls[1][2]?.headers as Record<string, string>;
    expect(first['Idempotency-Key']).toMatch(/^web-cluster-scale-in-/);
    expect(first['X-STX-Confirmation-ID']).toBeUndefined();
    expect(second['Idempotency-Key']).toBe(first['Idempotency-Key']);
    expect(second['X-STX-Confirmation-ID']).toBe('confirm-1');
  });

  it('带确认编号仍失败时不再重试 / does not retry again after a confirmed failure', async () => {
    const error = Object.assign(new Error('confirmation expired'), {
      data: {confirmation_required: true, confirmation_id: 'confirm-2'},
    });
    const post = vi.spyOn(apiClient, 'post').mockRejectedValue(error);

    await expect(
      ClusterScaleService.submitScaleOut(1, {
        host_id: 2,
        plan_fingerprint: 'sha256:abc',
        loopback_restart_ack: false,
      }),
    ).rejects.toBe(error);
    // 首次 + 带确认编号的一次重试 / first attempt + one confirmed retry
    expect(post).toHaveBeenCalledTimes(2);
  });

  it('重试操作发送 R1 确认请求头 / retry sends R1 confirmation headers', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValueOnce({
      data: {
        error_msg: '',
        data: {execution_id: 'ex-2', operation_id: 9, operation: null},
      },
    });

    await ClusterScaleService.retryOperation(1, 9);

    expect(post.mock.calls[0][0]).toBe('/clusters/1/scale-operations/9/retry');
    const headers = post.mock.calls[0][2]?.headers as Record<string, string>;
    expect(headers['X-STX-Confirm']).toBe('true');
    expect(headers['Idempotency-Key']).toMatch(/^web-cluster-scale-retry-/);
  });

  it('列表接口 items 为 null 时返回 [] / list returns [] when items is null', async () => {
    vi.spyOn(apiClient, 'get').mockResolvedValueOnce({
      data: {error_msg: '', data: {items: null}},
    });
    await expect(
      ClusterScaleService.listOperations(1, 'running', 1),
    ).resolves.toEqual([]);
  });
});

describe('getScaleInProgressDetail', () => {
  it('从 409 错误中读取进行中操作 / reads the running operation from a 409 error', () => {
    const error = Object.assign(new Error('in progress'), {
      status: 409,
      data: {
        operation_id: 4,
        kind: 'scale_in',
        created_by: 3,
        created_by_name: 'alice',
      },
    });
    expect(getScaleInProgressDetail(error)).toEqual({
      operation_id: 4,
      kind: 'scale_in',
      created_by: 3,
      created_by_name: 'alice',
    });
  });

  it('其他错误返回 null / returns null for other errors', () => {
    expect(getScaleInProgressDetail(new Error('boom'))).toBeNull();
    expect(
      getScaleInProgressDetail(
        Object.assign(new Error('x'), {data: {confirmation_required: true}}),
      ),
    ).toBeNull();
  });
});
