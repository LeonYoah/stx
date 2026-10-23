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

/**
 * 集群伸缩服务：扩容 / 缩容计划（R0）、提交（R2 两步确认）、操作查询与重试（R1）。
 * Cluster scaling service: scale-out / scale-in plans (R0), submission (R2 two-step confirmation),
 * operation queries and retry (R1).
 */

import {BaseService} from '../core/base.service';
import {createWebExecutionHeaders} from '../core/execution-headers';
import {getCurrentLocale} from '@/lib/i18n/config';
import type {
  ScaleInProgressDetail,
  ScaleInSubmitRequest,
  ScaleOperationInfo,
  ScaleOperationStatus,
  ScaleOutRequest,
  ScaleOutSubmitRequest,
  ScalePlan,
  ScaleSubmitResult,
} from './scale.types';

/** 携带后端结构化 data 的错误 / Error carrying the backend's structured data */
type ErrorWithData = Error & {
  data?: {
    confirmation_required?: boolean;
    confirmation_id?: string;
  } & Partial<ScaleInProgressDetail>;
};

/**
 * 把可能为 null 的数组兜底为 []（Go nil slice 会序列化为 null）。
 * Coerce possibly-null arrays to [] (Go nil slices serialize as null).
 */
function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

/**
 * 规范化伸缩计划，保证 UI 直接 map 的数组字段不为 null。
 * Normalize a scaling plan so array fields mapped by the UI are never null.
 */
export function normalizeScalePlan(plan: ScalePlan): ScalePlan {
  return {
    ...plan,
    members_before: asArray(plan.members_before),
    members_after: asArray(plan.members_after),
    checks: asArray(plan.checks),
    loopback_nodes: asArray(plan.loopback_nodes),
    plugins_to_replay: asArray(plan.plugins_to_replay).map((item) => ({
      ...item,
      profile_keys: asArray(item.profile_keys),
    })),
    running_jobs: asArray(plan.running_jobs),
    steps: asArray(plan.steps),
    blocking_reasons: asArray(plan.blocking_reasons),
    warnings: asArray(plan.warnings),
    checkpoint_storage: plan.checkpoint_storage ?? null,
    java_proxy_follow: plan.java_proxy_follow ?? null,
    rolling_restart: plan.rolling_restart
      ? {...plan.rolling_restart, nodes: asArray(plan.rolling_restart.nodes)}
      : null,
  };
}

/**
 * 规范化伸缩操作详情。
 * Normalize a scaling operation detail.
 */
export function normalizeScaleOperation(
  operation: ScaleOperationInfo,
): ScaleOperationInfo {
  return {
    ...operation,
    node_ids: asArray(operation.node_ids),
    warnings: asArray(operation.warnings),
    steps: asArray(operation.steps),
    plan: operation.plan ? normalizeScalePlan(operation.plan) : null,
  };
}

function normalizeSubmitResult(result: ScaleSubmitResult): ScaleSubmitResult {
  return {
    ...result,
    operation: result.operation
      ? normalizeScaleOperation(result.operation)
      : null,
  };
}

/**
 * 读取 409 cluster_scale_in_progress 携带的进行中操作信息；不是该错误时返回 null。
 * Read the running-operation details carried by 409 cluster_scale_in_progress; null for other errors.
 */
export function getScaleInProgressDetail(
  error: unknown,
): ScaleInProgressDetail | null {
  const data = (error as ErrorWithData | undefined)?.data;
  if (data && typeof data.operation_id === 'number' && data.operation_id > 0) {
    return {
      operation_id: data.operation_id,
      kind: data.kind ?? 'scale_out',
      created_by: data.created_by ?? 0,
      created_by_name: data.created_by_name ?? '',
    };
  }
  return null;
}

export class ClusterScaleService extends BaseService {
  protected static readonly basePath = '/clusters';

  /**
   * 计算扩容计划（只读）。
   * Compute a scale-out plan (read-only).
   */
  static async planScaleOut(
    clusterId: number,
    request: ScaleOutRequest,
  ): Promise<ScalePlan> {
    const plan = await this.postWithParams<ScalePlan>(
      `/${clusterId}/scale-out/plan`,
      request,
      {lang: getCurrentLocale()},
    );
    return normalizeScalePlan(plan);
  }

  /**
   * 计算缩容计划（只读）。
   * Compute a scale-in plan (read-only).
   */
  static async planScaleIn(
    clusterId: number,
    nodeId: number,
  ): Promise<ScalePlan> {
    const plan = await this.postWithParams<ScalePlan>(
      `/${clusterId}/nodes/${nodeId}/scale-in/plan`,
      {},
      {lang: getCurrentLocale()},
    );
    return normalizeScalePlan(plan);
  }

  /**
   * 提交扩容（R2）：首次请求取得确认编号后复用同一幂等键重试一次。
   * Submit scale-out (R2): after the first request returns a confirmation ID, retry once with the same
   * idempotency key.
   */
  static async submitScaleOut(
    clusterId: number,
    request: ScaleOutSubmitRequest,
  ): Promise<ScaleSubmitResult> {
    return this.postR2('cluster-scale-out', `/${clusterId}/scale-out`, request);
  }

  /**
   * 提交缩容（R2）。
   * Submit scale-in (R2).
   */
  static async submitScaleIn(
    clusterId: number,
    nodeId: number,
    request: ScaleInSubmitRequest,
  ): Promise<ScaleSubmitResult> {
    return this.postR2(
      'cluster-scale-in',
      `/${clusterId}/nodes/${nodeId}/scale-in`,
      request,
    );
  }

  /**
   * 从失败步骤重试伸缩操作（R1）。
   * Retry a scaling operation from its failed step (R1).
   */
  static async retryOperation(
    clusterId: number,
    operationId: number,
  ): Promise<ScaleSubmitResult> {
    const executionHeaders = createWebExecutionHeaders('cluster-scale-retry');
    const result = await this.postWithConfig<ScaleSubmitResult>(
      `/${clusterId}/scale-operations/${operationId}/retry`,
      {},
      {headers: executionHeaders.headers, params: {lang: getCurrentLocale()}},
    );
    return normalizeSubmitResult(result);
  }

  /**
   * 获取伸缩操作详情（含步骤）。
   * Get a scaling operation detail (with steps).
   */
  static async getOperation(
    clusterId: number,
    operationId: number,
  ): Promise<ScaleOperationInfo> {
    const operation = await this.get<ScaleOperationInfo>(
      `/${clusterId}/scale-operations/${operationId}`,
    );
    return normalizeScaleOperation(operation);
  }

  /**
   * 列出伸缩操作；页面刷新后用 status=running 恢复进行中的操作。
   * List scaling operations; after a page refresh, status=running restores the operation in progress.
   */
  static async listOperations(
    clusterId: number,
    status?: ScaleOperationStatus,
    limit = 20,
  ): Promise<ScaleOperationInfo[]> {
    const result = await this.get<{items: ScaleOperationInfo[] | null}>(
      `/${clusterId}/scale-operations`,
      {status, limit},
    );
    return asArray(result?.items).map(normalizeScaleOperation);
  }

  /**
   * R2 两步确认：第一次请求返回 confirmation_id 时，复用同一幂等键只重试一次。
   * R2 two-step confirmation: when the first request returns a confirmation_id, retry exactly once with the
   * same idempotency key.
   */
  private static async postR2(
    operation: string,
    path: string,
    body: unknown,
    idempotencyKey?: string,
    confirmationId?: string,
  ): Promise<ScaleSubmitResult> {
    const executionHeaders = createWebExecutionHeaders(operation, {
      idempotencyKey,
      confirmationId,
    });
    try {
      const result = await this.postWithConfig<ScaleSubmitResult>(path, body, {
        headers: executionHeaders.headers,
        params: {lang: getCurrentLocale()},
      });
      return normalizeSubmitResult(result);
    } catch (error) {
      const data = (error as ErrorWithData).data;
      if (
        !confirmationId &&
        data?.confirmation_required &&
        data.confirmation_id
      ) {
        return this.postR2(
          operation,
          path,
          body,
          executionHeaders.idempotencyKey,
          data.confirmation_id,
        );
      }
      throw error;
    }
  }
}
