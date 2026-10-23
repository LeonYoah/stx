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
import apiClient from '../core/api-client';
import {createWebExecutionHeaders} from '../core/execution-headers';
import type {ApiResponse} from '../core/types';
import type {
  CreateSyncGlobalVariableRequest,
  CreateSyncTaskRequest,
  PreviewSyncTaskRequest,
  PublishSyncTaskRequest,
  SyncDagResult,
  SyncGlobalVariable,
  SyncGlobalVariableListData,
  SyncJobInstance,
  SyncJobListData,
  SyncJobLogsResult,
  SyncCheckpointSnapshot,
  SyncPreviewSnapshot,
  SyncTask,
  SyncTaskPermissions,
  SyncTaskListData,
  SyncTaskTreeData,
  SyncTaskVersion,
  SyncTaskVersionListData,
  SyncValidateResult,
  SyncTaskActionRequest,
  SyncRecoverJobRequest,
  UpdateSyncGlobalVariableRequest,
  UpdateSyncTaskRequest,
  UpdateSyncTaskPermissionsRequest,
  SyncPluginType,
  SyncPluginFactoryListResult,
  SyncPluginOptionSchemaResult,
  SyncPluginTemplateResult,
  SyncPluginEnumValuesResult,
  SyncPluginEnumCatalogResult,
  SyncCuratedTemplateListResult,
  SyncCuratedTemplateView,
  SyncCuratedComboParseResult,
  SchedulerConnection,
  SchedulerConnectionRequest,
  DolphinProject,
  DolphinWorkflowSummary,
  DolphinWorkflowDetail,
  DolphinScriptResult,
  DolphinPushRequest,
  DolphinPushResult,
  DolphinTaskStatus,
} from './types';

export class SyncService extends BaseService {
  protected static readonly basePath = '/sync';

  static async getTree(): Promise<SyncTaskTreeData> {
    return this.get<SyncTaskTreeData>('/tree');
  }

  static async listTasks(params?: {
    current?: number;
    size?: number;
    status?: string;
    name?: string;
  }): Promise<SyncTaskListData> {
    return this.get<SyncTaskListData>('/tasks', params);
  }

  static async getTask(taskId: number): Promise<SyncTask> {
    return this.get<SyncTask>(`/tasks/${taskId}`);
  }

  static async createTask(request: CreateSyncTaskRequest): Promise<SyncTask> {
    return this.post<SyncTask>('/tasks', request);
  }

  static async updateTask(
    taskId: number,
    request: UpdateSyncTaskRequest,
  ): Promise<SyncTask> {
    return this.put<SyncTask>(`/tasks/${taskId}`, request);
  }

  // 只读取共享权限，不加载任务正文。
  // Get only sharing permissions, without loading task content.
  static async getTaskPermissions(taskId: number): Promise<SyncTaskPermissions> {
    return this.get<SyncTaskPermissions>(`/tasks/${taskId}/permissions`);
  }

  // 只更新共享权限，不创建新的任务正文版本。
  // Update only sharing permissions, without creating a new task-content version.
  static async updateTaskPermissions(
    taskId: number,
    request: UpdateSyncTaskPermissionsRequest,
  ): Promise<SyncTaskPermissions> {
    const executionHeaders = createWebExecutionHeaders('sync-task-permissions-update');
    return this.putWithConfig<SyncTaskPermissions>(
      `/tasks/${taskId}/permissions`,
      request,
      {headers: executionHeaders.headers},
    );
  }

  static async deleteTask(taskId: number): Promise<{deleted: boolean}> {
    return this.delete<{deleted: boolean}>(`/tasks/${taskId}`);
  }

  static async listGlobalVariables(params?: {
    current?: number;
    size?: number;
  }): Promise<SyncGlobalVariableListData> {
    return this.get<SyncGlobalVariableListData>('/global-variables', params);
  }

  static async createGlobalVariable(
    request: CreateSyncGlobalVariableRequest,
  ): Promise<SyncGlobalVariable> {
    return this.post<SyncGlobalVariable>('/global-variables', request);
  }

  static async updateGlobalVariable(
    id: number,
    request: UpdateSyncGlobalVariableRequest,
  ): Promise<SyncGlobalVariable> {
    return this.put<SyncGlobalVariable>(`/global-variables/${id}`, request);
  }

  static async deleteGlobalVariable(id: number): Promise<{deleted: boolean}> {
    return this.delete<{deleted: boolean}>(`/global-variables/${id}`);
  }

  static async publishTask(
    taskId: number,
    request?: PublishSyncTaskRequest,
  ): Promise<SyncTaskVersion> {
    return this.post<SyncTaskVersion>(
      `/tasks/${taskId}/publish`,
      request || {},
    );
  }

  static async listVersions(
    taskId: number,
    params?: {
      current?: number;
      size?: number;
    },
  ): Promise<SyncTaskVersionListData> {
    return this.get<SyncTaskVersionListData>(
      `/tasks/${taskId}/versions`,
      params,
    );
  }

  static async rollbackVersion(
    taskId: number,
    versionId: number,
  ): Promise<SyncTask> {
    return this.post<SyncTask>(
      `/tasks/${taskId}/versions/${versionId}/rollback`,
      {},
    );
  }

  static async deleteVersion(
    taskId: number,
    versionId: number,
  ): Promise<{deleted: boolean}> {
    return this.delete<{deleted: boolean}>(
      `/tasks/${taskId}/versions/${versionId}`,
    );
  }

  static async validateTask(
    taskId: number,
    request?: SyncTaskActionRequest,
  ): Promise<SyncValidateResult> {
    return this.post<SyncValidateResult>(
      `/tasks/${taskId}/validate`,
      request || {},
    );
  }

  static async testConnections(
    taskId: number,
    request?: SyncTaskActionRequest,
  ): Promise<SyncValidateResult> {
    return this.post<SyncValidateResult>(
      `/tasks/${taskId}/test-connections`,
      request || {},
    );
  }

  static async buildDag(
    taskId: number,
    request?: SyncTaskActionRequest,
  ): Promise<SyncDagResult> {
    return this.post<SyncDagResult>(`/tasks/${taskId}/dag`, request || {});
  }

  static async previewTask(
    taskId: number,
    request?: PreviewSyncTaskRequest,
  ): Promise<SyncJobInstance> {
    return this.post<SyncJobInstance>(
      `/tasks/${taskId}/preview`,
      request || {},
    );
  }

  static async submitTask(
    taskId: number,
    request?: SyncTaskActionRequest,
  ): Promise<SyncJobInstance> {
    return this.post<SyncJobInstance>(`/tasks/${taskId}/submit`, request || {});
  }

  static async listJobs(params?: {
    current?: number;
    size?: number;
    task_id?: number;
    run_type?: string;
    platform_job_id?: string;
    engine_job_id?: string;
  }): Promise<SyncJobListData> {
    return this.get<SyncJobListData>('/jobs', params);
  }

  static async getJob(jobId: number): Promise<SyncJobInstance> {
    return this.get<SyncJobInstance>(`/jobs/${jobId}`);
  }

  static async getPreviewSnapshot(
    jobId: number,
    params?: {table_path?: string},
  ): Promise<SyncPreviewSnapshot> {
    return this.get<SyncPreviewSnapshot>(`/jobs/${jobId}/preview`, params);
  }

  static async getJobCheckpoint(
    jobId: number,
    params?: {pipeline_id?: number; limit?: number; status?: string},
  ): Promise<SyncCheckpointSnapshot> {
    return this.get<SyncCheckpointSnapshot>(
      `/jobs/${jobId}/checkpoint`,
      params,
    );
  }

  static async getJobLogs(
    jobId: number,
    params?: {
      offset?: string;
      limit_bytes?: number;
      keyword?: string;
      level?: 'warn' | 'error' | 'all';
      signal?: AbortSignal;
    },
  ): Promise<SyncJobLogsResult> {
    const {signal, ...query} = params || {};
    const response = await apiClient.get<ApiResponse<SyncJobLogsResult>>(
      `${this.basePath}/jobs/${jobId}/logs`,
      {params: query, signal},
    );
    return response.data.data;
  }

  static async recoverJob(
    jobId: number,
    request?: SyncRecoverJobRequest,
  ): Promise<SyncJobInstance> {
    return this.post<SyncJobInstance>(`/jobs/${jobId}/recover`, request || {});
  }

  static async cancelJob(
    jobId: number,
    request?: {stop_with_savepoint?: boolean},
  ): Promise<SyncJobInstance> {
    return this.post<SyncJobInstance>(`/jobs/${jobId}/cancel`, request || {});
  }
  static async listPluginFactories(request: {
    cluster_id: number;
    plugin_type: SyncPluginType;
  }): Promise<SyncPluginFactoryListResult> {
    return this.post<SyncPluginFactoryListResult>('/plugins/list', request);
  }

  static async getPluginOptions(request: {
    cluster_id: number;
    plugin_type: SyncPluginType;
    factory_identifier: string;
    include_supplement?: boolean;
  }): Promise<SyncPluginOptionSchemaResult> {
    return this.post<SyncPluginOptionSchemaResult>('/plugins/options', request);
  }

  static async renderPluginTemplate(request: {
    cluster_id: number;
    plugin_type: SyncPluginType;
    factory_identifier: string;
    include_supplement?: boolean;
    include_comments?: boolean;
    include_advanced?: boolean;
  }): Promise<SyncPluginTemplateResult> {
    return this.post<SyncPluginTemplateResult>('/plugins/template', request);
  }

  static async listPluginEnumValues(request: {
    cluster_id: number;
    plugin_type: SyncPluginType;
    factory_identifier: string;
    option_key: string;
    include_supplement?: boolean;
  }): Promise<SyncPluginEnumValuesResult> {
    return this.post<SyncPluginEnumValuesResult>(
      '/plugins/enum-values',
      request,
    );
  }

  static async listPluginEnumCatalog(request: {
    cluster_id: number;
    include_supplement?: boolean;
  }): Promise<SyncPluginEnumCatalogResult> {
    return this.post<SyncPluginEnumCatalogResult>(
      '/plugins/enum-catalog',
      request,
    );
  }

  static async listCuratedTemplates(request?: {
    section?: string;
    mode?: string;
    q?: string;
    origin?: string;
  }): Promise<SyncCuratedTemplateListResult> {
    return this.post<SyncCuratedTemplateListResult>(
      '/curated-templates/list',
      request || {},
    );
  }

  static async createCuratedTemplate(request: {
    name: string;
    description?: string;
    section?: string;
    mode?: string;
    pattern?: string;
    connectors?: string[];
    content: string;
    builtin_id?: string;
  }): Promise<SyncCuratedTemplateView> {
    return this.post<SyncCuratedTemplateView>('/curated-templates', request);
  }

  static async updateCuratedTemplate(
    id: number,
    request: {
      name?: string;
      description?: string;
      section?: string;
      mode?: string;
      pattern?: string;
      connectors?: string[];
      content?: string;
      enabled?: boolean;
    },
  ): Promise<SyncCuratedTemplateView> {
    return this.put<SyncCuratedTemplateView>(
      `/curated-templates/${id}`,
      request,
    );
  }

  static async forkCuratedTemplate(request: {
    builtin_id: string;
    name?: string;
    description?: string;
  }): Promise<SyncCuratedTemplateView> {
    return this.post<SyncCuratedTemplateView>(
      '/curated-templates/fork',
      request,
    );
  }

  static async deleteCuratedTemplate(id: number): Promise<void> {
    await this.delete(`/curated-templates/${id}`);
  }

  static async parseCuratedCombo(request: {
    content: string;
  }): Promise<SyncCuratedComboParseResult> {
    return this.post<SyncCuratedComboParseResult>(
      '/curated-templates/parse-combo',
      request,
    );
  }

  static async renderCuratedTemplate(request: {
    builtin_id?: string;
    id?: number;
    cluster_id?: number;
  }): Promise<{content: string}> {
    return this.post<{content: string}>('/curated-templates/render', request);
  }

  // 列出脱敏后的调度器连接。
  // List scheduler connections with tokens masked.
  static async listSchedulerConnections(): Promise<{
    items: SchedulerConnection[];
  }> {
    const data = await this.get<{items?: SchedulerConnection[]} | SchedulerConnection[]>(
      '/scheduler-connections',
    );
    if (Array.isArray(data)) {
      return {items: data ?? []};
    }
    return {items: data?.items ?? []};
  }

  // 保存一条 Dolphin 连接；token 只在创建时提交。
  // Create a Dolphin connection; the token is submitted only at create time.
  static async createSchedulerConnection(
    request: SchedulerConnectionRequest,
  ): Promise<SchedulerConnection> {
    const executionHeaders = createWebExecutionHeaders(
      'sync-scheduler-connection-create',
    );
    return this.postWithConfig<SchedulerConnection>(
      '/scheduler-connections',
      request,
      {headers: executionHeaders.headers},
    );
  }

  // 列出连接上的 Dolphin 项目，空结果归一成 []。
  // List Dolphin projects on a connection and normalize a missing list to [].
  static async listDolphinProjects(
    connectionId: number,
  ): Promise<DolphinProject[]> {
    const data = await this.get<DolphinProject[]>(
      `/scheduler-connections/${connectionId}/projects`,
    );
    return data ?? [];
  }

  // 列出项目下的工作流定义，空结果归一成 []。
  // List workflow definitions in a project and normalize a missing list to [].
  static async listDolphinWorkflows(
    connectionId: number,
    projectCode: number,
  ): Promise<DolphinWorkflowSummary[]> {
    const data = await this.get<DolphinWorkflowSummary[]>(
      `/scheduler-connections/${connectionId}/projects/${projectCode}/workflows`,
    );
    return data ?? [];
  }

  // 读取工作流定义，并把 tasks/relations 的 null 归一成 []。
  // Load a workflow definition and normalize null tasks/relations to [].
  static async getDolphinWorkflow(
    connectionId: number,
    projectCode: number,
    workflowCode: number,
  ): Promise<DolphinWorkflowDetail> {
    const data = await this.get<DolphinWorkflowDetail>(
      `/scheduler-connections/${connectionId}/projects/${projectCode}/workflows/${workflowCode}`,
    );
    return {
      ...data,
      tasks: data?.tasks ?? [],
      relations: data?.relations ?? [],
      schedules: data?.schedules ?? [],
    };
  }

  // 读取任务的 Dolphin 绑定状态；recent_jobs 兜底为空数组。
  // Load Dolphin binding status for a task; recent_jobs falls back to [].
  static async getDolphinTaskStatus(
    taskId: number,
  ): Promise<DolphinTaskStatus> {
    const data = await this.get<DolphinTaskStatus>(`/tasks/${taskId}/dolphin`);
    return {
      ...data,
      recent_jobs: data?.recent_jobs ?? [],
    };
  }

  // 生成可粘贴到 Dolphin SHELL 节点的脚本和环境说明。
  // Generate the SHELL script and worker env notes for a Dolphin node.
  static async getDolphinScript(taskId: number): Promise<DolphinScriptResult> {
    return this.get<DolphinScriptResult>(`/tasks/${taskId}/dolphin/script`);
  }

  // 把任务推送到 Dolphin；写操作走公共执行确认头。
  // Push a task into Dolphin; writes use the shared execution confirmation headers.
  static async pushDolphin(
    taskId: number,
    request: DolphinPushRequest,
  ): Promise<DolphinPushResult> {
    const executionHeaders = createWebExecutionHeaders('sync-task-dolphin-push');
    const result = await this.postWithConfig<DolphinPushResult>(
      `/tasks/${taskId}/dolphin/push`,
      request,
      {headers: executionHeaders.headers},
    );
    return {
      ...result,
      schedules: result?.schedules ?? [],
      schedules_restored: result?.schedules_restored ?? [],
    };
  }

  // 只删除 STX 侧绑定，不改 Dolphin 节点，也不吊销管理员签发的登录令牌。
  // Remove the STX binding without changing Dolphin nodes or revoking admin-issued login tokens.
  static async unbindDolphin(taskId: number): Promise<{unbound: boolean}> {
    const executionHeaders = createWebExecutionHeaders(
      'sync-task-dolphin-unbind',
    );
    return this.postWithConfig<{unbound: boolean}>(
      `/tasks/${taskId}/dolphin/unbind`,
      {},
      {headers: executionHeaders.headers},
    );
  }
}
