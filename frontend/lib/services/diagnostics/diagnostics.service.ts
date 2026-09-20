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

import {BaseService} from '../core/base.service';
import {getCurrentLocale} from '@/lib/i18n/config';
import type {
  CreateDiagnosticsTaskRequest,
  CreateInspectionAutoPolicyRequest,
  DiagnosticsErrorEventListData,
  DiagnosticsErrorEventListParams,
  DiagnosticsErrorGroupDetailData,
  DiagnosticsErrorGroupListData,
  DiagnosticsErrorGroupListParams,
  DiagnosticsInspectionReportDetailData,
  DiagnosticsInspectionReportListData,
  DiagnosticsInspectionReportListParams,
  DiagnosticsResourceDefinition,
  DiagnosticsTask,
  DiagnosticsTaskCreateRequestOptions,
  DiagnosticsTaskListData,
  DiagnosticsTaskListParams,
  DiagnosticsTaskLogListData,
  DiagnosticsTaskLogQuery,
  InspectionAutoPolicy,
  InspectionAutoPolicyListData,
  InspectionConditionTemplate,
  StartDiagnosticsInspectionRequest,
  DiagnosticsWorkspaceBootstrapData,
  DiagnosticsWorkspaceBootstrapParams,
  UpdateInspectionAutoPolicyRequest,
} from './types';

function normalizeDiagnosticsTask(task: DiagnosticsTask): DiagnosticsTask {
  return {
    ...task,
    selected_nodes: Array.isArray(task.selected_nodes)
      ? task.selected_nodes
      : [],
    steps: Array.isArray(task.steps) ? task.steps : [],
    node_executions: Array.isArray(task.node_executions)
      ? task.node_executions
      : [],
  };
}

function withDiagnosticsLanguage(
  params?: Record<string, unknown>,
): Record<string, unknown> {
  return {
    ...(params || {}),
    lang: getCurrentLocale(),
  };
}

export class DiagnosticsService extends BaseService {
  protected static readonly basePath = '/diagnostics';

  /**
   * 获取已登记的诊断资源。
   * List registered diagnostics resources.
   */
  static async listResources(): Promise<DiagnosticsResourceDefinition[]> {
    const data = await this.get<DiagnosticsResourceDefinition[]>(
      '/resources',
      withDiagnosticsLanguage(),
    );
    return Array.isArray(data) ? data : [];
  }

  /**
   * 安全获取已登记的诊断资源。
   * Safely list registered diagnostics resources.
   */
  static async listResourcesSafe(): Promise<{
    success: boolean;
    data?: DiagnosticsResourceDefinition[];
    error?: string;
  }> {
    try {
      return {success: true, data: await this.listResources()};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载诊断资源失败',
      };
    }
  }

  /**
   * Get diagnostics workspace bootstrap payload.
   * 获取诊断中心工作台初始化数据。
   */
  static async getWorkspaceBootstrap(
    params?: DiagnosticsWorkspaceBootstrapParams,
  ): Promise<DiagnosticsWorkspaceBootstrapData> {
    return this.get<DiagnosticsWorkspaceBootstrapData>(
      '/bootstrap',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
  }

  /**
   * Safely get diagnostics workspace bootstrap payload.
   * 安全获取诊断中心工作台初始化数据。
   */
  static async getWorkspaceBootstrapSafe(
    params?: DiagnosticsWorkspaceBootstrapParams,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsWorkspaceBootstrapData;
    error?: string;
  }> {
    try {
      const data = await this.getWorkspaceBootstrap(params);
      return {success: true, data};
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : '加载诊断中心失败';
      return {success: false, error: errorMessage};
    }
  }

  /**
   * List diagnostics error groups.
   * 获取诊断错误组列表。
   */
  static async getErrorGroups(
    params?: DiagnosticsErrorGroupListParams,
  ): Promise<DiagnosticsErrorGroupListData> {
    return this.get<DiagnosticsErrorGroupListData>(
      '/errors/groups',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
  }

  /**
   * Safely list diagnostics error groups.
   * 安全获取诊断错误组列表。
   */
  static async getErrorGroupsSafe(
    params?: DiagnosticsErrorGroupListParams,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsErrorGroupListData;
    error?: string;
  }> {
    try {
      const data = await this.getErrorGroups(params);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载错误组失败',
      };
    }
  }

  /**
   * List diagnostics error events.
   * 获取诊断错误事件列表。
   */
  static async getErrorEvents(
    params?: DiagnosticsErrorEventListParams,
  ): Promise<DiagnosticsErrorEventListData> {
    return this.get<DiagnosticsErrorEventListData>(
      '/errors/events',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
  }

  /**
   * Safely list diagnostics error events.
   * 安全获取诊断错误事件列表。
   */
  static async getErrorEventsSafe(
    params?: DiagnosticsErrorEventListParams,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsErrorEventListData;
    error?: string;
  }> {
    try {
      const data = await this.getErrorEvents(params);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载错误事件失败',
      };
    }
  }

  /**
   * Get diagnostics error group detail.
   * 获取诊断错误组详情。
   */
  static async getErrorGroupDetail(
    groupId: number,
    eventLimit = 20,
    params?: Pick<
      DiagnosticsErrorEventListParams,
      'cluster_id' | 'node_id' | 'host_id' | 'role' | 'job_id'
    >,
  ): Promise<DiagnosticsErrorGroupDetailData> {
    return this.get<DiagnosticsErrorGroupDetailData>(
      `/errors/groups/${groupId}`,
      withDiagnosticsLanguage({
        event_limit: eventLimit,
        ...params,
      }),
    );
  }

  /**
   * Safely get diagnostics error group detail.
   * 安全获取诊断错误组详情。
   */
  static async getErrorGroupDetailSafe(
    groupId: number,
    eventLimit = 20,
    params?: Pick<
      DiagnosticsErrorEventListParams,
      'cluster_id' | 'node_id' | 'host_id' | 'role' | 'job_id'
    >,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsErrorGroupDetailData;
    error?: string;
  }> {
    try {
      const data = await this.getErrorGroupDetail(groupId, eventLimit, params);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载错误详情失败',
      };
    }
  }

  /**
   * Start one diagnostics inspection.
   * 发起一次诊断巡检。
   */
  static async startInspection(
    payload: StartDiagnosticsInspectionRequest,
  ): Promise<DiagnosticsInspectionReportDetailData> {
    return this.postWithParams<DiagnosticsInspectionReportDetailData>(
      '/inspections',
      payload,
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely start one diagnostics inspection.
   * 安全发起一次诊断巡检。
   */
  static async startInspectionSafe(
    payload: StartDiagnosticsInspectionRequest,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsInspectionReportDetailData;
    error?: string;
  }> {
    try {
      const data = await this.startInspection(payload);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '发起巡检失败',
      };
    }
  }

  /**
   * List diagnostics inspection reports.
   * 获取诊断巡检报告列表。
   */
  static async getInspectionReports(
    params?: DiagnosticsInspectionReportListParams,
  ): Promise<DiagnosticsInspectionReportListData> {
    return this.get<DiagnosticsInspectionReportListData>(
      '/inspections',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
  }

  /**
   * Safely list diagnostics inspection reports.
   * 安全获取诊断巡检报告列表。
   */
  static async getInspectionReportsSafe(
    params?: DiagnosticsInspectionReportListParams,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsInspectionReportListData;
    error?: string;
  }> {
    try {
      const data = await this.getInspectionReports(params);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载巡检报告失败',
      };
    }
  }

  /**
   * Get diagnostics inspection report detail.
   * 获取诊断巡检报告详情。
   */
  static async getInspectionReportDetail(
    reportId: number,
  ): Promise<DiagnosticsInspectionReportDetailData> {
    return this.get<DiagnosticsInspectionReportDetailData>(
      `/inspections/${reportId}`,
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely get diagnostics inspection report detail.
   * 安全获取诊断巡检报告详情。
   */
  static async getInspectionReportDetailSafe(reportId: number): Promise<{
    success: boolean;
    data?: DiagnosticsInspectionReportDetailData;
    error?: string;
  }> {
    try {
      const data = await this.getInspectionReportDetail(reportId);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载巡检详情失败',
      };
    }
  }

  /**
   * Create one diagnostics task.
   * 创建一条诊断任务。
   */
  static async createTask(
    payload: CreateDiagnosticsTaskRequest,
    options?: DiagnosticsTaskCreateRequestOptions,
  ): Promise<DiagnosticsTask> {
    const idempotencyKey =
      options?.idempotency_key ||
      `web-diagnostics-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    const headers: Record<string, string> = {
      'Idempotency-Key': idempotencyKey,
    };
    if (options?.confirmed !== false) {
      headers['X-STX-Confirm'] = 'true';
    }
    if (options?.confirmation_id) {
      headers['X-STX-Confirmation-ID'] = options.confirmation_id;
    }

    try {
      const data = await this.postWithConfig<DiagnosticsTask>(
        '/tasks',
        payload,
        {
          params: withDiagnosticsLanguage(),
          headers,
        },
      );
      return normalizeDiagnosticsTask(data);
    } catch (error) {
      const confirmationData = (
        error as Error & {
          data?: {confirmation_required?: boolean; confirmation_id?: string};
        }
      ).data;
      if (
        !options?.confirmation_id &&
        confirmationData?.confirmation_required &&
        confirmationData.confirmation_id
      ) {
        return this.createTask(payload, {
          ...options,
          confirmed: true,
          idempotency_key: idempotencyKey,
          confirmation_id: confirmationData.confirmation_id,
        });
      }
      throw error;
    }
  }

  /**
   * Safely create one diagnostics task.
   * 安全创建一条诊断任务。
   */
  static async createTaskSafe(
    payload: CreateDiagnosticsTaskRequest,
    options?: DiagnosticsTaskCreateRequestOptions,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsTask;
    error?: string;
  }> {
    try {
      return {success: true, data: await this.createTask(payload, options)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '创建诊断任务失败',
      };
    }
  }

  /**
   * List diagnostics tasks.
   * 获取诊断任务列表。
   */
  static async listTasks(
    params?: DiagnosticsTaskListParams,
  ): Promise<DiagnosticsTaskListData> {
    const data = await this.get<DiagnosticsTaskListData>(
      '/tasks',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
    return {
      ...data,
      items: Array.isArray(data.items) ? data.items : [],
    };
  }

  /**
   * Safely list diagnostics tasks.
   * 安全获取诊断任务列表。
   */
  static async listTasksSafe(params?: DiagnosticsTaskListParams): Promise<{
    success: boolean;
    data?: DiagnosticsTaskListData;
    error?: string;
  }> {
    try {
      return {success: true, data: await this.listTasks(params)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载诊断任务失败',
      };
    }
  }

  /**
   * Get diagnostics task detail.
   * 获取诊断任务详情。
   */
  static async getTask(taskId: number): Promise<DiagnosticsTask> {
    const data = await this.get<DiagnosticsTask>(
      `/tasks/${taskId}`,
      withDiagnosticsLanguage(),
    );
    return normalizeDiagnosticsTask(data);
  }

  /**
   * Safely get diagnostics task detail.
   * 安全获取诊断任务详情。
   */
  static async getTaskSafe(taskId: number): Promise<{
    success: boolean;
    data?: DiagnosticsTask;
    error?: string;
  }> {
    try {
      return {success: true, data: await this.getTask(taskId)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载诊断任务详情失败',
      };
    }
  }

  /**
   * Start one diagnostics task.
   * 启动一条诊断任务。
   */
  static async startTask(taskId: number): Promise<DiagnosticsTask> {
    const data = await this.postWithParams<DiagnosticsTask>(
      `/tasks/${taskId}/start`,
      undefined,
      withDiagnosticsLanguage(),
    );
    return normalizeDiagnosticsTask(data);
  }

  /**
   * Safely start one diagnostics task.
   * 安全启动一条诊断任务。
   */
  static async startTaskSafe(taskId: number): Promise<{
    success: boolean;
    data?: DiagnosticsTask;
    error?: string;
  }> {
    try {
      return {success: true, data: await this.startTask(taskId)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '启动诊断任务失败',
      };
    }
  }

  /**
   * List diagnostics task logs.
   * 获取诊断任务日志。
   */
  static async getTaskLogs(
    taskId: number,
    params?: DiagnosticsTaskLogQuery,
  ): Promise<DiagnosticsTaskLogListData> {
    const data = await this.get<DiagnosticsTaskLogListData>(
      `/tasks/${taskId}/logs`,
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
    return {
      ...data,
      items: Array.isArray(data.items) ? data.items : [],
    };
  }

  /**
   * Safely list diagnostics task logs.
   * 安全获取诊断任务日志。
   */
  static async getTaskLogsSafe(
    taskId: number,
    params?: DiagnosticsTaskLogQuery,
  ): Promise<{
    success: boolean;
    data?: DiagnosticsTaskLogListData;
    error?: string;
  }> {
    try {
      return {success: true, data: await this.getTaskLogs(taskId, params)};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载诊断任务日志失败',
      };
    }
  }

  /**
   * Get the diagnostics task SSE url.
   * 获取诊断任务 SSE 地址。
   */
  static getTaskEventsUrl(taskId: number): string {
    return `/api/v1${this.basePath}/tasks/${taskId}/events/stream?lang=${getCurrentLocale()}`;
  }

  /**
   * Get the diagnostics task HTML summary url.
   * 获取诊断任务 HTML 摘要地址。
   */
  static getTaskHTMLUrl(taskId: number, download = false): string {
    const query = new URLSearchParams({
      lang: getCurrentLocale(),
    });
    if (download) {
      query.set('download', '1');
    }
    return `/api/v1${this.basePath}/tasks/${taskId}/html?${query.toString()}`;
  }

  /**
   * Get the diagnostics task bundle download url.
   * 获取诊断任务打包下载地址。
   */
  static getTaskBundleUrl(taskId: number): string {
    return `/api/v1${this.basePath}/tasks/${taskId}/bundle`;
  }

  // ─── Auto-Inspection Policy APIs ──────────────────────────────────────────

  /**
   * List builtin condition templates.
   * 获取内置条件模板列表。
   */
  static async listBuiltinConditionTemplates(): Promise<
    InspectionConditionTemplate[]
  > {
    return this.get<InspectionConditionTemplate[]>(
      '/auto-policies/templates',
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely list builtin condition templates.
   * 安全获取内置条件模板列表。
   */
  static async listBuiltinConditionTemplatesSafe(): Promise<{
    success: boolean;
    data?: InspectionConditionTemplate[];
    error?: string;
  }> {
    try {
      const data = await this.listBuiltinConditionTemplates();
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载条件模板失败',
      };
    }
  }

  /**
   * List auto-inspection policies.
   * 获取自动巡检策略列表。
   */
  static async listAutoPolicies(params?: {
    cluster_id?: number;
    page?: number;
    page_size?: number;
  }): Promise<InspectionAutoPolicyListData> {
    return this.get<InspectionAutoPolicyListData>(
      '/auto-policies',
      withDiagnosticsLanguage(params as Record<string, unknown> | undefined),
    );
  }

  /**
   * Safely list auto-inspection policies.
   * 安全获取自动巡检策略列表。
   */
  static async listAutoPoliciesSafe(params?: {
    cluster_id?: number;
    page?: number;
    page_size?: number;
  }): Promise<{
    success: boolean;
    data?: InspectionAutoPolicyListData;
    error?: string;
  }> {
    try {
      const data = await this.listAutoPolicies(params);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '加载自动巡检策略失败',
      };
    }
  }

  /**
   * Create an auto-inspection policy.
   * 创建自动巡检策略。
   */
  static async createAutoPolicy(
    payload: CreateInspectionAutoPolicyRequest,
  ): Promise<InspectionAutoPolicy> {
    return this.postWithParams<InspectionAutoPolicy>(
      '/auto-policies',
      payload,
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely create an auto-inspection policy.
   * 安全创建自动巡检策略。
   */
  static async createAutoPolicySafe(
    payload: CreateInspectionAutoPolicyRequest,
  ): Promise<{
    success: boolean;
    data?: InspectionAutoPolicy;
    error?: string;
  }> {
    try {
      const data = await this.createAutoPolicy(payload);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '创建自动巡检策略失败',
      };
    }
  }

  /**
   * Get an auto-inspection policy by ID.
   * 获取自动巡检策略详情。
   */
  static async getAutoPolicy(id: number): Promise<InspectionAutoPolicy> {
    return this.get<InspectionAutoPolicy>(
      `/auto-policies/${id}`,
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely get an auto-inspection policy by ID.
   * 安全获取自动巡检策略详情。
   */
  static async getAutoPolicySafe(id: number): Promise<{
    success: boolean;
    data?: InspectionAutoPolicy;
    error?: string;
  }> {
    try {
      const data = await this.getAutoPolicy(id);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error:
          error instanceof Error ? error.message : '加载自动巡检策略详情失败',
      };
    }
  }

  /**
   * Update an auto-inspection policy.
   * 更新自动巡检策略。
   */
  static async updateAutoPolicy(
    id: number,
    payload: UpdateInspectionAutoPolicyRequest,
  ): Promise<InspectionAutoPolicy> {
    return this.putWithParams<InspectionAutoPolicy>(
      `/auto-policies/${id}`,
      payload,
      withDiagnosticsLanguage(),
    );
  }

  /**
   * Safely update an auto-inspection policy.
   * 安全更新自动巡检策略。
   */
  static async updateAutoPolicySafe(
    id: number,
    payload: UpdateInspectionAutoPolicyRequest,
  ): Promise<{
    success: boolean;
    data?: InspectionAutoPolicy;
    error?: string;
  }> {
    try {
      const data = await this.updateAutoPolicy(id, payload);
      return {success: true, data};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '更新自动巡检策略失败',
      };
    }
  }

  /**
   * Delete an auto-inspection policy.
   * 删除自动巡检策略。
   */
  static async deleteAutoPolicy(id: number): Promise<void> {
    await this.delete<void>(`/auto-policies/${id}`, withDiagnosticsLanguage());
  }

  /**
   * Safely delete an auto-inspection policy.
   * 安全删除自动巡检策略。
   */
  static async deleteAutoPolicySafe(id: number): Promise<{
    success: boolean;
    error?: string;
  }> {
    try {
      await this.deleteAutoPolicy(id);
      return {success: true};
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '删除自动巡检策略失败',
      };
    }
  }
}
