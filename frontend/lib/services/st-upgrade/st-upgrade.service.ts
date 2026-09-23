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
 * SeaTunnel Upgrade Service
 * SeaTunnel 升级服务
 */

import {BaseService} from '../core/base.service';
import apiClient from '../core/api-client';
import {createWebExecutionHeaders} from '../core/execution-headers';
import type {ApiResponse} from '../core/types';
import type {
  CreatePlanRequest,
  CreatePlanResult,
  ExecutePlanRequest,
  ListUpgradeTasksQuery,
  PrecheckRequest,
  PrecheckResult,
  ServiceResult,
  TaskListData,
  TaskLogsData,
  TaskLogsQuery,
  TaskStepsData,
  UpgradePlanRecord,
  UpgradeTask,
} from './types';
import {
  sanitizePrecheckResult,
  sanitizeTaskListData,
  sanitizeTaskLogsData,
  sanitizeTaskStepsData,
  sanitizeUpgradePlanRecord,
  sanitizeUpgradeTask,
} from './normalize';

export class StUpgradeService extends BaseService {
  protected static readonly basePath = '/st-upgrade';

  private static async postWithAcceptedStatuses<T>(
    path: string,
    payload: unknown,
    acceptedStatuses: number[],
  ): Promise<T> {
    const response = await apiClient.post<ApiResponse<T>>(
      this.getFullPath(path),
      payload,
      {
        validateStatus: (status) => acceptedStatuses.includes(status),
      },
    );

    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }
    return response.data.data;
  }

  static async runPrecheck(payload: PrecheckRequest): Promise<PrecheckResult> {
    const result = await this.postWithAcceptedStatuses<PrecheckResult>(
      '/precheck',
      payload,
      [200, 409],
    );
    return sanitizePrecheckResult(result) || result;
  }

  static async createPlan(
    payload: CreatePlanRequest,
  ): Promise<CreatePlanResult> {
    const result = await this.postWithAcceptedStatuses<CreatePlanResult>(
      '/plan',
      payload,
      [201, 409],
    );
    return {
      ...result,
      precheck: sanitizePrecheckResult(result.precheck) || result.precheck,
      plan: sanitizeUpgradePlanRecord(result.plan) || result.plan,
    };
  }

  static async getPlan(planId: number): Promise<UpgradePlanRecord> {
    const result = await this.get<UpgradePlanRecord>(`/plans/${planId}`);
    return sanitizeUpgradePlanRecord(result) || result;
  }

  static async executePlan(
    payload: ExecutePlanRequest,
    idempotencyKey?: string,
    confirmationId?: string,
  ): Promise<UpgradeTask> {
    const executionHeaders = createWebExecutionHeaders(
      'stupgrade-plan-execute',
      {idempotencyKey, confirmationId},
    );
    try {
      return await this.postWithConfig<UpgradeTask>('/execute', payload, {
        headers: executionHeaders.headers,
      });
    } catch (error) {
      const confirmationData = (
        error as Error & {
          data?: {confirmation_required?: boolean; confirmation_id?: string};
        }
      ).data;
      if (
        !confirmationId &&
        confirmationData?.confirmation_required &&
        confirmationData.confirmation_id
      ) {
        return this.executePlan(
          payload,
          executionHeaders.idempotencyKey,
          confirmationData.confirmation_id,
        );
      }
      throw error;
    }
  }

  static async listTasks(query?: ListUpgradeTasksQuery): Promise<TaskListData> {
    const result = await this.get<TaskListData>(
      '/tasks',
      query as Record<string, unknown> | undefined,
    );
    const sanitized = sanitizeTaskListData(result);
    return {
      ...(sanitized || result),
      items: Array.isArray((sanitized || result).items)
        ? (sanitized || result).items
        : [],
    };
  }

  static async getTask(taskId: number): Promise<UpgradeTask> {
    const result = await this.get<UpgradeTask>(`/tasks/${taskId}`);
    return sanitizeUpgradeTask(result) || result;
  }

  static async getTaskSteps(taskId: number): Promise<TaskStepsData> {
    const result = await this.get<TaskStepsData>(`/tasks/${taskId}/steps`);
    return sanitizeTaskStepsData(result) || result;
  }

  static async getTaskLogs(
    taskId: number,
    query?: TaskLogsQuery,
  ): Promise<TaskLogsData> {
    const result = await this.get<TaskLogsData>(
      `/tasks/${taskId}/logs`,
      query as Record<string, unknown> | undefined,
    );
    return sanitizeTaskLogsData(result) || result;
  }

  static getTaskEventsUrl(taskId: number): string {
    return `/api/v1${this.basePath}/tasks/${taskId}/events/stream`;
  }

  static async runPrecheckSafe(
    payload: PrecheckRequest,
  ): Promise<ServiceResult<PrecheckResult>> {
    try {
      return {success: true, data: await this.runPrecheck(payload)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '升级预检查失败',
      };
    }
  }

  static async createPlanSafe(
    payload: CreatePlanRequest,
  ): Promise<ServiceResult<CreatePlanResult>> {
    try {
      return {success: true, data: await this.createPlan(payload)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '创建升级计划失败',
      };
    }
  }

  static async executePlanSafe(
    payload: ExecutePlanRequest,
  ): Promise<ServiceResult<UpgradeTask>> {
    try {
      return {success: true, data: await this.executePlan(payload)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '启动升级执行失败',
      };
    }
  }

  static async listTasksSafe(
    query?: ListUpgradeTasksQuery,
  ): Promise<ServiceResult<TaskListData>> {
    try {
      return {success: true, data: await this.listTasks(query)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取升级任务列表失败',
      };
    }
  }

  static async getTaskSafe(
    taskId: number,
  ): Promise<ServiceResult<UpgradeTask>> {
    try {
      return {success: true, data: await this.getTask(taskId)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取升级任务失败',
      };
    }
  }
}
