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

package operation

import (
	"fmt"
	"strings"
)

// syncOperationSpecs 登记数据同步工作台（Workbench/DataSyncStudio）的全部 API 操作。
// syncOperationSpecs registers all API operations for the Data Sync Workbench / DataSyncStudio.
func syncOperationSpecs() []OperationSpec {
	var specs []OperationSpec

	taskIDInput := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Sync task ID"}}
	jobIDInput := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Job instance ID"}}
	variableIDInput := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Global variable ID"}}
	versionIDInputs := []InputSpec{
		{Name: "id", Location: InputPath, Required: true, Description: "Sync task ID"},
		{Name: "versionId", Location: InputPath, Required: true, Description: "Task version ID"},
	}

	taskPaginationInputs := []InputSpec{
		{Name: "page", Location: InputQuery, Required: false, Description: "Page number starting from 1"},
		{Name: "page_size", Location: InputQuery, Required: false, Description: "Number of items per page"},
		{Name: "keyword", Location: InputQuery, Required: false, Description: "Search keyword for task name"},
		{Name: "cluster_id", Location: InputQuery, Required: false, Description: "Filter by cluster ID"},
		{Name: "type", Location: InputQuery, Required: false, Description: "Filter by task type (folder/file)"},
		{Name: "status", Location: InputQuery, Required: false, Description: "Filter by task status"},
	}

	jobPaginationInputs := []InputSpec{
		{Name: "page", Location: InputQuery, Required: false, Description: "Page number starting from 1"},
		{Name: "page_size", Location: InputQuery, Required: false, Description: "Number of items per page"},
		{Name: "task_id", Location: InputQuery, Required: false, Description: "Filter by sync task ID"},
		{Name: "status", Location: InputQuery, Required: false, Description: "Filter by job status (RUNNING/FINISHED/FAILED/CANCELED)"},
	}

	jobLogsInputs := []InputSpec{
		{Name: "id", Location: InputPath, Required: true, Description: "Job instance ID"},
		{Name: "lines", Location: InputQuery, Required: false, Description: "Number of log lines to retrieve"},
		{Name: "all", Location: InputQuery, Required: false, Description: "Retrieve all log content"},
		{Name: "level", Location: InputQuery, Required: false, Description: "Filter logs by level (DEBUG/INFO/WARN/ERROR)"},
		{Name: "keyword", Location: InputQuery, Required: false, Description: "Filter logs by keyword"},
		{Name: "offset", Location: InputQuery, Required: false, Description: "Offset token for incremental reading"},
		{Name: "limit_bytes", Location: InputQuery, Required: false, Description: "Maximum bytes to return"},
	}

	checkpointInputs := []InputSpec{
		{Name: "id", Location: InputPath, Required: true, Description: "Job instance ID"},
		{Name: "pipeline_id", Location: InputQuery, Required: false, Description: "Filter by pipeline ID"},
		{Name: "limit", Location: InputQuery, Required: false, Description: "Maximum number of checkpoint records"},
		{Name: "status", Location: InputQuery, Required: false, Description: "Filter by checkpoint status"},
	}

	previewSnapshotInputs := []InputSpec{
		{Name: "id", Location: InputPath, Required: true, Description: "Job instance ID"},
		{Name: "table_path", Location: InputQuery, Required: false, Description: "Table path for preview table"},
	}

	variablePaginationInputs := []InputSpec{
		{Name: "page", Location: InputQuery, Required: false, Description: "Page number starting from 1"},
		{Name: "page_size", Location: InputQuery, Required: false, Description: "Number of items per page"},
		{Name: "keyword", Location: InputQuery, Required: false, Description: "Search keyword for variable key or remark"},
	}

	// 1. Task Tree & Tasks
	specs = append(specs,
		syncOp("sync.tree", []string{"sync", "tree"}, "Get sync workspace task tree", "GET", "/api/v1/sync/tree", RiskR0, "", nil, "stx sync tree", true, false),
		syncOp("sync.task.list", []string{"sync", "task", "list"}, "List sync tasks", "GET", "/api/v1/sync/tasks", RiskR0, "", taskPaginationInputs, "stx sync task list --cluster_id 1", true, false),
		syncOp("sync.task.get", []string{"sync", "task", "get"}, "Get one sync task", "GET", "/api/v1/sync/tasks/:id", RiskR0, "", taskIDInput, "stx sync task get 1", true, false),
		syncOp("sync.task.create", []string{"sync", "task", "create"}, "Create a sync task or folder", "POST", "/api/v1/sync/tasks", RiskR1, "创建同步任务或目录节点。", syncWriteInputs(false, nil), "stx sync task create --name my-task --confirm", false, false),
		syncOp("sync.task.update", []string{"sync", "task", "update"}, "Update a sync task", "PUT", "/api/v1/sync/tasks/:id", RiskR1, "更新同步任务的基础信息或配置内容。", syncWriteInputs(false, taskIDInput), "stx sync task update 1 --confirm", false, false),
		syncOp("sync.task.permissions.get", []string{"sync", "task", "permissions", "get"}, "Get sync task sharing permissions", "GET", "/api/v1/sync/tasks/:id/permissions", RiskR0, "", taskIDInput, "stx sync task permissions get 1", false, false),
		syncOp("sync.task.permissions.update", []string{"sync", "task", "permissions", "update"}, "Update sync task sharing permissions", "PUT", "/api/v1/sync/tasks/:id/permissions", RiskR1, "修改任务公开状态和协作者列表，不会修改任务正文，也不会发布新版本。", syncWriteInputs(true, taskIDInput), "stx sync task permissions update 1 --request-file permissions.json --confirm", false, false),
		syncOp("sync.task.delete", nil, "Delete a sync task", "DELETE", "/api/v1/sync/tasks/:id", RiskR2, "删除同步任务及其历史版本，删除后不可恢复。", syncWriteInputs(false, taskIDInput), "", false, false),
		syncOp("sync.task.publish", []string{"sync", "task", "publish"}, "Publish a sync task version", "POST", "/api/v1/sync/tasks/:id/publish", RiskR1, "将当前任务配置冻结发布为一个新的历史版本。", syncWriteInputs(false, taskIDInput), "stx sync task publish 1 --confirm", false, false),
	)

	// 2. Task Versions
	specs = append(specs,
		syncOp("sync.task.version.list", []string{"sync", "task", "version", "list"}, "List historical versions of a sync task", "GET", "/api/v1/sync/tasks/:id/versions", RiskR0, "", taskIDInput, "stx sync task version list 1", true, false),
		syncOp("sync.task.version.rollback", []string{"sync", "task", "version", "rollback"}, "Rollback a sync task to a historical version", "POST", "/api/v1/sync/tasks/:id/versions/:versionId/rollback", RiskR1, "将任务当前草稿内容回滚至指定历史版本。", syncWriteInputs(false, versionIDInputs), "stx sync task version rollback 1 2 --confirm", false, false),
		syncOp("sync.task.version.delete", nil, "Delete a historical version of a sync task", "DELETE", "/api/v1/sync/tasks/:id/versions/:versionId", RiskR2, "删除指定的任务历史版本，不可撤回。", syncWriteInputs(false, versionIDInputs), "", false, false),
	)

	// 3. Task DAG, Validation, Preview & Submit
	specs = append(specs,
		syncOp("sync.task.validate", []string{"sync", "task", "validate"}, "Validate a sync task configuration", "POST", "/api/v1/sync/tasks/:id/validate", RiskR0, "", taskIDInput, "stx sync task validate 1", false, false),
		syncOp("sync.task.test-connections", []string{"sync", "task", "test-connections"}, "Test data source connections in a sync task", "POST", "/api/v1/sync/tasks/:id/test-connections", RiskR0, "", taskIDInput, "stx sync task test-connections 1", false, false),
		syncOp("sync.task.dag", []string{"sync", "task", "dag"}, "Get DAG graph for a sync task", "POST", "/api/v1/sync/tasks/:id/dag", RiskR0, "", taskIDInput, "stx sync task dag 1", false, false),
		syncOp("sync.task.preview", []string{"sync", "task", "preview"}, "Start a preview sampling session for a sync task", "POST", "/api/v1/sync/tasks/:id/preview", RiskR1, "启动一次轻量数据抽样预览会话，消耗短暂的引擎计算资源。", syncWriteInputs(false, taskIDInput), "stx sync task preview 1 --confirm", false, true),
		syncOp("sync.task.preview-savemode", []string{"sync", "task", "preview-savemode"}, "Preview sink SaveMode SQL and actions for a sync task", "POST", "/api/v1/sync/tasks/:id/preview/sink-savemode", RiskR0, "", taskIDInput, "stx sync task preview-savemode 1", false, false),
		syncOp("sync.task.submit", []string{"sync", "task", "submit"}, "Submit a sync task to execute on the cluster", "POST", "/api/v1/sync/tasks/:id/submit", RiskR2, "提交分布式 SeaTunnel 作业运行，占用集群 Worker 槽位与资源。", syncWriteInputs(false, taskIDInput), "stx sync task submit 1 --confirm", false, true),
	)

	// 4. Job Management & Checkpoint / Savepoint
	specs = append(specs,
		syncOp("sync.job.list", []string{"sync", "job", "list"}, "List SeaTunnel job instances", "GET", "/api/v1/sync/jobs", RiskR0, "", jobPaginationInputs, "stx sync job list --status RUNNING", true, false),
		syncOp("sync.job.get", []string{"sync", "job", "get"}, "Get one SeaTunnel job instance", "GET", "/api/v1/sync/jobs/:id", RiskR0, "", jobIDInput, "stx sync job get 1", true, false),
		syncOp("sync.job.logs", []string{"sync", "job", "logs"}, "Get SeaTunnel job execution logs", "GET", "/api/v1/sync/jobs/:id/logs", RiskR0, "", jobLogsInputs, "stx sync job logs 1 --lines 200", true, false),
		syncOp("sync.job.checkpoint", []string{"sync", "job", "checkpoint"}, "Get checkpoint snapshot and metrics for a SeaTunnel job", "GET", "/api/v1/sync/jobs/:id/checkpoint", RiskR0, "", checkpointInputs, "stx sync job checkpoint 1", true, false),
		syncOp("sync.job.preview", []string{"sync", "job", "preview"}, "Get preview data snapshot for a SeaTunnel job", "GET", "/api/v1/sync/jobs/:id/preview", RiskR0, "", previewSnapshotInputs, "stx sync job preview 1", true, false),
		syncOp("sync.job.cancel", []string{"sync", "job", "cancel"}, "Cancel a running SeaTunnel job", "POST", "/api/v1/sync/jobs/:id/cancel", RiskR2, "终止集群中正在运行的作业，可指定 --savepoint 保存当前状态。", syncWriteInputs(false, jobIDInput), "stx sync job cancel 1 --confirm", false, false),
		syncOp("sync.job.recover", []string{"sync", "job", "recover"}, "Recover a stopped or failed job from checkpoint or savepoint", "POST", "/api/v1/sync/jobs/:id/recover", RiskR2, "从最后 Checkpoint 或 Savepoint 恢复作业，重新提交至集群运行。", syncWriteInputs(false, jobIDInput), "stx sync job recover 1 --confirm", false, true),
	)

	// 5. Global Variables
	specs = append(specs,
		syncOp("sync.variable.list", []string{"sync", "variable", "list"}, "List global sync variables", "GET", "/api/v1/sync/global-variables", RiskR0, "", variablePaginationInputs, "stx sync variable list", true, false),
		syncOp("sync.variable.create", []string{"sync", "variable", "create"}, "Create a global sync variable", "POST", "/api/v1/sync/global-variables", RiskR1, "创建全局变量，供所有同步任务引用。", syncWriteInputs(false, nil), "stx sync variable create --confirm", false, false),
		syncOp("sync.variable.update", []string{"sync", "variable", "update"}, "Update a global sync variable", "PUT", "/api/v1/sync/global-variables/:id", RiskR1, "更新全局变量值，会影响后续引用该变量的任务运行。", syncWriteInputs(false, variableIDInput), "stx sync variable update 1 --confirm", false, false),
		syncOp("sync.variable.delete", nil, "Delete a global sync variable", "DELETE", "/api/v1/sync/global-variables/:id", RiskR1, "删除全局变量，引用该变量的任务可能解析失败。", syncWriteInputs(false, variableIDInput), "", false, false),
	)

	// 6. Plugins & Template Metadata
	specs = append(specs,
		syncOp("sync.plugin.list", []string{"sync", "plugin", "list"}, "List supported sync plugin factories", "POST", "/api/v1/sync/plugins/list", RiskR0, "", nil, "stx sync plugin list", false, false),
		syncOp("sync.plugin.options", []string{"sync", "plugin", "options"}, "Get plugin option configuration schema", "POST", "/api/v1/sync/plugins/options", RiskR0, "", nil, "stx sync plugin options", false, false),
		syncOp("sync.plugin.template", []string{"sync", "plugin", "template"}, "Render plugin configuration template", "POST", "/api/v1/sync/plugins/template", RiskR0, "", nil, "stx sync plugin template", false, false),
		syncOp("sync.plugin.enum-values", []string{"sync", "plugin", "enum-values"}, "Get dynamic enum values for plugin options", "POST", "/api/v1/sync/plugins/enum-values", RiskR0, "", nil, "stx sync plugin enum-values", false, false),
		syncOp("sync.plugin.enum-catalog", []string{"sync", "plugin", "enum-catalog"}, "Get dynamic enum catalog for plugin options", "POST", "/api/v1/sync/plugins/enum-catalog", RiskR0, "", nil, "stx sync plugin enum-catalog", false, false),
	)

	return specs
}

func syncWriteInputs(requireBody bool, pathInputs []InputSpec) []InputSpec {
	inputs := append([]InputSpec{}, pathInputs...)
	if requireBody {
		inputs = append(inputs, InputSpec{Name: "request", Location: InputBody, Required: true, Description: "Request payload"})
	}
	inputs = append(inputs,
		InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key for write operations"},
		InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"},
	)
	return inputs
}

func syncOp(id string, commandPath []string, summary, method, route string, risk RiskLevel, impactMsg string, inputs []InputSpec, example string, generated bool, isAsync bool) OperationSpec {
	var impactSpec *ImpactSpec
	if strings.TrimSpace(impactMsg) != "" {
		impactSpec = &ImpactSpec{Level: risk, Message: impactMsg}
	}
	return OperationSpec{
		ID:           id,
		CommandPath:  commandPath,
		Summary:      summary,
		GeneratedCLI: generated,
		Method:       method,
		Route:        route,
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         risk,
		Revision:     1,
		Async:        isAsync,
		SupportsPick: true,
		Impact:       impactSpec,
		Input:        inputs,
		Example:      example,
		OutputExample: fmt.Sprintf(`{
  "api_version": "v1",
  "operation_id": %q,
  "request_id": "req_example",
  "data": {},
  "result_meta": {"complete": true}
}`, id),
	}
}
