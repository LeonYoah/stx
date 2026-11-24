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

package sync

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// Handler 提供数据同步工作台 HTTP 处理器。
// Handler provides HTTP handlers for sync studio APIs.
type Handler struct {
	service   *Service
	auditRepo *audit.Repository
}

// NewHandler creates a new sync handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// SetAuditRepository 配置工作台写操作使用的审计仓库。
// SetAuditRepository configures the audit repository for sync writes.
func (h *Handler) SetAuditRepository(repo *audit.Repository) { h.auditRepo = repo }

type TaskResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *Task  `json:"data"`
}
type TaskListResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *TaskListData `json:"data"`
}
type TaskTreeResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *TaskTreeData `json:"data"`
}
type TaskVersionResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *TaskVersion `json:"data"`
}
type TaskPermissionsResponse struct {
	ErrorMsg string           `json:"error_msg"`
	Data     *TaskPermissions `json:"data"`
}
type TaskVersionListResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *TaskVersionListData `json:"data"`
}
type GlobalVariableResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *GlobalVariable `json:"data"`
}
type GlobalVariableListResponse struct {
	ErrorMsg string                  `json:"error_msg"`
	Data     *GlobalVariableListData `json:"data"`
}
type ValidateResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *ValidateResult `json:"data"`
}
type DAGResponse struct {
	ErrorMsg string     `json:"error_msg"`
	Data     *DAGResult `json:"data"`
}
type JobResponse struct {
	// ErrorCode 只在有专门处理的失败上下发（如 connector_missing），CLI 与前端据此给出指引。
	// ErrorCode is only set for failures with dedicated handling (e.g. connector_missing) so CLI and UI can guide the user.
	ErrorCode string       `json:"error_code,omitempty"`
	ErrorMsg  string       `json:"error_msg"`
	Data      *JobInstance `json:"data"`
}

// JobRealtimeResponse 包装作业实时指标结果。
// JobRealtimeResponse wraps the job realtime metrics result.
type JobRealtimeResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *JobRealtimeResult `json:"data"`
}

// JobPlanDAGResponse 包装作业计划 DAG 结果。
// JobPlanDAGResponse wraps the job plan DAG result.
type JobPlanDAGResponse struct {
	ErrorMsg string            `json:"error_msg"`
	Data     *JobPlanDAGResult `json:"data"`
}

// JobTimeseriesResponse 包装作业时序查询结果。
// JobTimeseriesResponse wraps the job timeseries query result.
type JobTimeseriesResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *JobTimeseriesResult `json:"data"`
}

// JobSparklinesResponse 是 sparklines 接口的响应外壳。
// JobSparklinesResponse is the envelope of the sparklines endpoint.
type JobSparklinesResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *JobSparklinesResult `json:"data"`
}
type JobListResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *JobListData `json:"data"`
}
type JobLogsResponse struct {
	ErrorMsg string         `json:"error_msg"`
	Data     *JobLogsResult `json:"data"`
}
type PreviewSnapshotResponse struct {
	ErrorMsg string           `json:"error_msg"`
	Data     *PreviewSnapshot `json:"data"`
}
type CheckpointSnapshotResponse struct {
	ErrorMsg string              `json:"error_msg"`
	Data     *CheckpointSnapshot `json:"data"`
}
type BasicResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// CreateTask handles POST /api/v1/sync/tasks.
func (h *Handler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: err.Error()})
		return
	}
	actor := currentExecutionActor(c)
	task, err := h.service.CreateTask(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskResponse{ErrorMsg: err.Error()})
		return
	}
	task.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	c.JSON(http.StatusOK, TaskResponse{Data: sanitizeTaskForResponse(task)})
}

// ListTasks handles GET /api/v1/sync/tasks.
func (h *Handler) ListTasks(c *gin.Context) {
	filter := &TaskFilter{Name: c.Query("name"), Page: parsePositiveInt(c.Query("current"), 1), Size: parsePositiveInt(c.Query("size"), 200)}
	if status := c.Query("status"); status != "" {
		filter.Status = TaskStatus(status)
	}
	tasks, total, err := h.service.ListTasksForActor(c.Request.Context(), currentExecutionActor(c), filter)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskListResponse{Data: &TaskListData{Total: total, Items: sanitizeTasksForResponse(tasks)}})
}

// GetTaskTree handles GET /api/v1/sync/tree.
func (h *Handler) GetTaskTree(c *gin.Context) {
	items, err := h.service.GetTaskTreeForActor(c.Request.Context(), currentExecutionActor(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskTreeResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskTreeResponse{Data: &TaskTreeData{Items: sanitizeTaskTreeForResponse(items)}})
}

// GetTask handles GET /api/v1/sync/tasks/:id.
func (h *Handler) GetTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: "invalid task id"})
		return
	}
	task, err := h.service.GetTaskForActor(c.Request.Context(), currentExecutionActor(c), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskResponse{ErrorMsg: err.Error()})
		return
	}
	response := sanitizeTaskForResponse(task)
	response.TimeoutDefaults = currentTaskTimeoutDefaults()
	c.JSON(http.StatusOK, TaskResponse{Data: response})
}

// GetTaskPermissions 处理 GET /api/v1/sync/tasks/:id/permissions。
// GetTaskPermissions handles GET /api/v1/sync/tasks/:id/permissions.
// @Summary 获取同步任务共享权限
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Success 200 {object} TaskPermissionsResponse
// @Failure 400 {object} TaskPermissionsResponse
// @Failure 403 {object} TaskPermissionsResponse
// @Failure 404 {object} TaskPermissionsResponse
// @Router /api/v1/sync/tasks/{id}/permissions [get]
func (h *Handler) GetTaskPermissions(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskPermissionsResponse{ErrorMsg: "invalid task id"})
		return
	}
	permissions, err := h.service.GetTaskPermissionsForActor(c.Request.Context(), currentExecutionActor(c), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskPermissionsResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskPermissionsResponse{Data: permissions})
}

// UpdateTaskPermissions 处理 PUT /api/v1/sync/tasks/:id/permissions。
// UpdateTaskPermissions handles PUT /api/v1/sync/tasks/:id/permissions.
// @Summary 更新同步任务共享权限
// @Tags Sync
// @Accept json
// @Produce json
// @Param id path int true "任务 ID"
// @Param request body UpdateTaskPermissionsRequest true "共享权限请求"
// @Success 200 {object} TaskPermissionsResponse
// @Failure 400 {object} TaskPermissionsResponse
// @Failure 403 {object} TaskPermissionsResponse
// @Failure 404 {object} TaskPermissionsResponse
// @Router /api/v1/sync/tasks/{id}/permissions [put]
func (h *Handler) UpdateTaskPermissions(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskPermissionsResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req UpdateTaskPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, TaskPermissionsResponse{ErrorMsg: err.Error()})
		return
	}
	before, after, err := h.service.UpdateTaskPermissionsForActor(c.Request.Context(), currentExecutionActor(c), id, &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskPermissionsResponse{ErrorMsg: err.Error()})
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "sync_task_permissions", strconv.FormatUint(uint64(id), 10), "", audit.AuditDetails{
			"operation_id": "sync.task.permissions.update",
			"before":       before,
			"after":        after,
		})
	c.JSON(http.StatusOK, TaskPermissionsResponse{Data: after})
}

// ListGlobalVariables handles GET /api/v1/sync/global-variables.
func (h *Handler) ListGlobalVariables(c *gin.Context) {
	page := parsePositiveInt(c.Query("current"), 1)
	size := parsePositiveInt(c.Query("size"), 20)
	items, total, err := h.service.ListGlobalVariablesPaginated(c.Request.Context(), page, size)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), GlobalVariableListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GlobalVariableListResponse{Data: &GlobalVariableListData{Total: total, Items: items}})
}

// CreateGlobalVariable 处理 POST /api/v1/sync/global-variables。
// CreateGlobalVariable handles POST /api/v1/sync/global-variables.
func (h *Handler) CreateGlobalVariable(c *gin.Context) {
	var req CreateGlobalVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, GlobalVariableResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.CreateGlobalVariable(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), GlobalVariableResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GlobalVariableResponse{Data: item})
}

// UpdateGlobalVariable handles PUT /api/v1/sync/global-variables/:id.
func (h *Handler) UpdateGlobalVariable(c *gin.Context) {
	actor := currentExecutionActor(c)
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, GlobalVariableResponse{ErrorMsg: "invalid global variable id"})
		return
	}
	var req UpdateGlobalVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, GlobalVariableResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.UpdateGlobalVariableForActor(c.Request.Context(), actor, id, &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), GlobalVariableResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GlobalVariableResponse{Data: item})
}

// DeleteGlobalVariable handles DELETE /api/v1/sync/global-variables/:id.
func (h *Handler) DeleteGlobalVariable(c *gin.Context) {
	actor := currentExecutionActor(c)
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: "invalid global variable id"})
		return
	}
	if err := h.service.DeleteGlobalVariableForActor(c.Request.Context(), actor, id); err != nil {
		c.JSON(h.getStatusCodeForError(err), BasicResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, BasicResponse{Data: gin.H{"deleted": true}})
}

// UpdateTask handles PUT /api/v1/sync/tasks/:id.
func (h *Handler) UpdateTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: err.Error()})
		return
	}
	task, err := h.service.UpdateTaskForActor(c.Request.Context(), currentExecutionActor(c), id, &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskResponse{ErrorMsg: err.Error()})
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "sync_task", strconv.FormatUint(uint64(id), 10), task.Name, audit.AuditDetails{
			"operation_id": "sync.task.update",
			"field_scope":  "task_content",
		})
	c.JSON(http.StatusOK, TaskResponse{Data: sanitizeTaskForResponse(task)})
}

// DeleteTask handles DELETE /api/v1/sync/tasks/:id.
func (h *Handler) DeleteTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: "invalid task id"})
		return
	}
	if err := h.service.DeleteTaskForActor(c.Request.Context(), currentExecutionActor(c), id); err != nil {
		c.JSON(h.getStatusCodeForError(err), BasicResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, BasicResponse{Data: gin.H{"deleted": true}})
}

// PublishTask handles POST /api/v1/sync/tasks/:id/publish.
func (h *Handler) PublishTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskVersionResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req PublishTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, TaskVersionResponse{ErrorMsg: err.Error()})
		return
	}
	_, version, err := h.service.PublishTaskForActor(c.Request.Context(), currentExecutionActor(c), id, req.Comment, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskVersionResponse{ErrorMsg: err.Error()})
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"publish", "sync_task", strconv.FormatUint(uint64(id), 10), "", audit.AuditDetails{
			"operation_id": "sync.task.publish",
			"version_id":   version.ID,
			"version":      version.Version,
		})
	c.JSON(http.StatusOK, TaskVersionResponse{Data: sanitizeTaskVersionForResponse(version)})
}

// ListTaskVersions handles GET /api/v1/sync/tasks/:id/versions.
func (h *Handler) ListTaskVersions(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskVersionListResponse{ErrorMsg: "invalid task id"})
		return
	}
	page := parsePositiveInt(c.Query("current"), 1)
	size := parsePositiveInt(c.Query("size"), 10)
	versions, total, err := h.service.ListTaskVersionsPaginatedForActor(c.Request.Context(), currentExecutionActor(c), id, page, size)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskVersionListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskVersionListResponse{Data: &TaskVersionListData{Total: total, Items: sanitizeTaskVersionsForResponse(versions)}})
}

// RollbackTaskVersion handles POST /api/v1/sync/tasks/:id/versions/:versionId/rollback.
func (h *Handler) RollbackTaskVersion(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: "invalid task id"})
		return
	}
	versionID, ok := parseUintParam(c, "versionId")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskResponse{ErrorMsg: "invalid version id"})
		return
	}
	task, err := h.service.RollbackTaskVersionForActor(c.Request.Context(), currentExecutionActor(c), id, versionID)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskResponse{Data: sanitizeTaskForResponse(task)})
}

// DeleteTaskVersion handles DELETE /api/v1/sync/tasks/:id/versions/:versionId.
func (h *Handler) DeleteTaskVersion(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: "invalid task id"})
		return
	}
	versionID, ok := parseUintParam(c, "versionId")
	if !ok {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: "invalid version id"})
		return
	}
	if err := h.service.DeleteTaskVersionForActor(c.Request.Context(), currentExecutionActor(c), id, versionID); err != nil {
		c.JSON(h.getStatusCodeForError(err), BasicResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, BasicResponse{Data: gin.H{"deleted": true}})
}

// ValidateTask handles POST /api/v1/sync/tasks/:id/validate.
func (h *Handler) ValidateTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, ValidateResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req TaskActionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ValidateResponse{ErrorMsg: err.Error()})
			return
		}
	}
	result, err := h.service.ValidateTaskForActor(c.Request.Context(), currentExecutionActor(c), id, req.Draft)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), ValidateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ValidateResponse{Data: sanitizeValidateResultForResponse(result)})
}

// TestTaskConnections handles POST /api/v1/sync/tasks/:id/test-connections.
func (h *Handler) TestTaskConnections(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, ValidateResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req TaskActionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ValidateResponse{ErrorMsg: err.Error()})
			return
		}
	}
	result, err := h.service.TestTaskConnectionsForActor(c.Request.Context(), currentExecutionActor(c), id, req.Draft)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), ValidateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ValidateResponse{Data: sanitizeValidateResultForResponse(result)})
}

// GetTaskDAG handles POST /api/v1/sync/tasks/:id/dag.
func (h *Handler) GetTaskDAG(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DAGResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req TaskActionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, DAGResponse{ErrorMsg: err.Error()})
			return
		}
	}
	result, err := h.service.BuildTaskDAGForActor(c.Request.Context(), currentExecutionActor(c), id, req.Draft)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DAGResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DAGResponse{Data: sanitizeDAGResultForResponse(result)})
}

// PreviewTask handles POST /api/v1/sync/tasks/:id/preview.
func (h *Handler) PreviewTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req PreviewTaskRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
			return
		}
	}
	job, err := h.service.PreviewTaskForActor(c.Request.Context(), currentExecutionActor(c), id, &req, syncExecutionRequest(c, map[string]any{"task_id": id, "request": req}))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorCode: syncErrorCode(err), ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// SubmitTask handles POST /api/v1/sync/tasks/:id/submit.
func (h *Handler) SubmitTask(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req TaskActionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
			return
		}
	}
	opts, err := parseSubmitOptions(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
		return
	}
	execReq := syncExecutionRequest(c, map[string]any{
		"task_id":       id,
		"draft":         req.Draft,
		"variables":     req.Variables,
		"external_ref":  req.ExternalRef,
		"schedule_time": req.ScheduleTime,
	})
	job, err := h.service.SubmitTaskForActorWithOptions(c.Request.Context(), currentExecutionActor(c), id, opts, execReq)
	details := audit.AuditDetails{
		"operation_id": "sync.task.submit",
		"request_id":   execReq.RequestID,
	}
	if strings.TrimSpace(req.ExternalRef) != "" {
		details["external_ref"] = strings.TrimSpace(req.ExternalRef)
	}
	if err != nil {
		details["result_status"] = "failed"
		details["error"] = err.Error()
		_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
			"submit", "sync_task", strconv.FormatUint(uint64(id), 10), "", details)
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorCode: syncErrorCode(err), ErrorMsg: err.Error()})
		return
	}
	details["result_status"] = "success"
	if job != nil {
		details["execution_id"] = job.ExecutionID
		details["job_id"] = job.ID
		if job.EngineJobID != "" {
			details["engine_job_id"] = job.EngineJobID
		}
		if job.PlatformJobID != "" {
			details["platform_job_id"] = job.PlatformJobID
		}
		if submitOutcomeIsUnknown(job) {
			details["submit_outcome"] = submitOutcomeUnknown
		}
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"submit", "sync_task", strconv.FormatUint(uint64(id), 10), "", details)
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// ListJobs handles GET /api/v1/sync/jobs.
// ListJobs 返回作业列表；不带扩展参数时与旧接口一致，扩展参数用于首页全局作业列表。
// ListJobs returns job instances; without the extended parameters it behaves as before, and they serve the home
// page's global job list.
// @Summary 作业实例列表（支持全局筛选、排序与当前值）
// @Tags Sync
// @Produce json
// @Param current query int false "页码（CLI 用 page）" default(1)
// @Param size query int false "每页条数（CLI 用 page_size）" default(50)
// @Param task_id query int false "任务 ID"
// @Param status query string false "原始状态，逗号分隔"
// @Param status_group query string false "状态组，逗号分隔：running|failed|pending|stopping|canceled|finished"
// @Param mode query string false "streaming|batch"
// @Param source query string false "stx|external"
// @Param cluster_id query int false "集群 ID"
// @Param keyword query string false "任务名（模糊）、引擎作业 ID 或平台作业 ID（精确）"
// @Param exclude_run_type query string false "排除的运行类型，逗号分隔，如 preview"
// @Param sort query string false "attention|started_at|created_at|finished_at|duration|rate|cdc_delay（后两者需 status_group=running）"
// @Param order query string false "asc|desc" default(desc)
// @Param with_counts query int false "1 = 返回各状态组计数"
// @Param with_live query int false "1 = 返回任务名与当前读写速率、条数与逻辑表数"
// @Param range query string false "活跃窗口预设：15m|1h|6h|24h|7d（窗口内运行过的作业，运行中恒命中）"
// @Param start query string false "自定义活跃窗口起点（unix 秒/毫秒或 RFC3339），与 end 一起用时优先于 range"
// @Param end query string false "自定义活跃窗口终点（unix 秒/毫秒或 RFC3339）"
// @Success 200 {object} JobListResponse
// @Failure 400 {object} JobListResponse
// @Router /api/v1/sync/jobs [get]
func (h *Handler) ListJobs(c *gin.Context) {
	filter := &JobFilter{
		// current / size 是网页端参数名，page / page_size 是 CLI 登记的参数名，两套都认。
		// current / size are the web parameter names and page / page_size the CLI ones; both are accepted.
		Page:          parsePositiveInt(firstQuery(c, "current", "page"), 1),
		Size:          parsePositiveInt(firstQuery(c, "size", "page_size"), 50),
		PlatformJobID: strings.TrimSpace(c.Query("platform_job_id")),
		EngineJobID:   strings.TrimSpace(c.Query("engine_job_id")),
	}
	// job_id は UI の #N 表示＝内部数値 ID の完全一致検索（クイック定位用）。
	// job_id filters by the internal numeric id (the UI's #N display) for quick locate.
	if raw := strings.TrimSpace(c.Query("job_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: "invalid job_id"})
			return
		}
		filter.JobID = uint(parsed)
	}
	if taskID := c.Query("task_id"); taskID != "" {
		parsed, err := strconv.ParseUint(taskID, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: "invalid task_id"})
			return
		}
		filter.TaskID = uint(parsed)
	}
	if runType := c.Query("run_type"); runType != "" {
		filter.RunType = RunType(runType)
	}
	// 全局作业列表的扩展参数；都不传时与旧接口完全一致。
	// Extended global job list parameters; with none set the endpoint behaves exactly as before.
	if raw := strings.TrimSpace(c.Query("status_group")); raw != "" {
		statuses, err := ResolveJobStatusGroups(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: err.Error()})
			return
		}
		filter.Statuses = append(filter.Statuses, statuses...)
	}
	for _, raw := range strings.Split(c.Query("status"), ",") {
		if status := strings.TrimSpace(raw); status != "" {
			filter.Statuses = append(filter.Statuses, JobStatus(strings.ToLower(status)))
		}
	}
	for _, raw := range strings.Split(c.Query("exclude_run_type"), ",") {
		if runType := strings.TrimSpace(raw); runType != "" {
			filter.ExcludeRunTypes = append(filter.ExcludeRunTypes, RunType(strings.ToLower(runType)))
		}
	}
	if clusterID := strings.TrimSpace(c.Query("cluster_id")); clusterID != "" {
		parsed, err := strconv.ParseUint(clusterID, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		filter.ClusterID = uint(parsed)
	}
	filter.Mode = strings.TrimSpace(c.Query("mode"))
	filter.Source = JobSource(c.Query("source"))
	filter.Keyword = strings.TrimSpace(c.Query("keyword"))
	filter.Sort = c.Query("sort")
	filter.Order = c.Query("order")
	// 活跃窗口：自定义 start/end 优先，其次 range 预设（服务端按请求时刻滚动求窗，轮询时窗口随时间滑动）。
	// Active window: custom start/end wins over a range preset (resolved server-side per request, so polls
	// see a rolling window).
	if window, err := promquery.ParseWindow(c.Query("start"), c.Query("end"), time.Now()); err != nil {
		c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: err.Error()})
		return
	} else if window != nil {
		filter.ActiveFrom, filter.ActiveTo = &window.Start, &window.End
	} else if rangeKey := strings.TrimSpace(c.Query("range")); rangeKey != "" {
		if _, start, end, err := promquery.ResolveWindow(rangeKey, nil, time.Now()); err != nil {
			c.JSON(http.StatusBadRequest, JobListResponse{ErrorMsg: err.Error()})
			return
		} else {
			filter.ActiveFrom, filter.ActiveTo = &start, &end
		}
	}
	opts := JobListOptions{WithCounts: queryFlag(c, "with_counts"), WithLive: queryFlag(c, "with_live")}
	data, err := h.service.ListJobsWithOptions(c.Request.Context(), currentExecutionActor(c), filter, opts)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobListResponse{ErrorMsg: err.Error()})
		return
	}
	data.Items = sanitizeJobsForResponse(data.Items)
	c.JSON(http.StatusOK, JobListResponse{Data: data})
}

// GetJobSparklines 批量返回作业近一段时间的读 / 写速率迷你曲线（Prometheus）。
// GetJobSparklines returns recent read / write rate sparklines for many jobs (Prometheus).
// @Summary 批量获取作业速率迷你曲线
// @Tags Sync
// @Produce json
// @Param ids query string true "逗号分隔的作业 ID，最多 100 个；无权查看的 ID 被忽略"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param metric query string false "write|read" default(write)
// @Success 200 {object} JobSparklinesResponse
// @Failure 400 {object} JobSparklinesResponse
// @Router /api/v1/sync/jobs/sparklines [get]
func (h *Handler) GetJobSparklines(c *gin.Context) {
	ids, err := ParseJobIDList(c.Query("ids"))
	if err != nil {
		c.JSON(http.StatusBadRequest, JobSparklinesResponse{ErrorMsg: err.Error()})
		return
	}
	data, err := h.service.GetJobSparklinesForActor(c.Request.Context(), currentExecutionActor(c), ids, c.Query("range"), c.Query("metric"))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobSparklinesResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobSparklinesResponse{Data: data})
}

// GetJob handles GET /api/v1/sync/jobs/:id.
func (h *Handler) GetJob(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid job id"})
		return
	}
	fresh := strings.EqualFold(strings.TrimSpace(c.Query("fresh")), "1") ||
		strings.EqualFold(strings.TrimSpace(c.Query("fresh")), "true")
	job, err := h.service.GetJobForActor(c.Request.Context(), currentExecutionActor(c), id, fresh)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// GetJobRealtime 处理 GET /api/v1/sync/jobs/:id/realtime：返回引擎实时指标与瓶颈提示，引擎不可用时以 available=false 表达。
// GetJobRealtime handles GET /api/v1/sync/jobs/:id/realtime: it returns engine realtime metrics and bottleneck
// hints, expressing engine unavailability as available=false.
// @Summary 获取作业实时指标与瓶颈提示
// @Tags Sync
// @Produce json
// @Param id path int true "作业 ID"
// @Param window_ms query int false "时间窗口（毫秒），超出引擎上限时截断"
// @Success 200 {object} JobRealtimeResponse
// @Failure 400 {object} JobRealtimeResponse
// @Failure 403 {object} JobRealtimeResponse
// @Failure 404 {object} JobRealtimeResponse
// @Router /api/v1/sync/jobs/{id}/realtime [get]
func (h *Handler) GetJobRealtime(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobRealtimeResponse{ErrorMsg: "invalid job id"})
		return
	}
	windowMs := int64(parseNonNegativeInt(c.Query("window_ms"), 0))
	result, err := h.service.GetJobRealtimeForActor(c.Request.Context(), currentExecutionActor(c), id, windowMs)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobRealtimeResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobRealtimeResponse{Data: result})
}

// GetJobTimeseries 处理 GET /api/v1/sync/jobs/:id/timeseries：按 PromQL 模板查 Prometheus，无本地兜底。
// GetJobTimeseries handles GET /api/v1/sync/jobs/:id/timeseries: queries Prometheus via PromQL templates with no local fallback.
// @Summary 获取作业指标时序
// @Tags Sync
// @Produce json
// @Param id path int true "作业 ID"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param step query int false "步长（秒），0 表示按范围自动"
// @Param metrics query string false "逗号分隔指标：throughput,rows,cdc_delay,checkpoint,busy,backpressure,table"
// @Success 200 {object} JobTimeseriesResponse
// @Failure 400 {object} JobTimeseriesResponse
// @Failure 403 {object} JobTimeseriesResponse
// @Failure 404 {object} JobTimeseriesResponse
// @Router /api/v1/sync/jobs/{id}/timeseries [get]
func (h *Handler) GetJobTimeseries(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobTimeseriesResponse{ErrorMsg: "invalid job id"})
		return
	}
	rangeKey := strings.TrimSpace(c.Query("range"))
	// start/end 给出时按自定义时间窗查询（作业全程、日历选择），优先于 range。
	// When start/end are given, query that custom window (job lifetime, calendar pick); it wins over range.
	window, err := ParseJobTimeseriesWindow(c.Query("start"), c.Query("end"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, JobTimeseriesResponse{ErrorMsg: err.Error()})
		return
	}
	step := parseNonNegativeInt(c.Query("step"), 0)
	metrics := strings.TrimSpace(c.Query("metrics"))
	result, err := h.service.GetJobTimeseriesForActor(c.Request.Context(), currentExecutionActor(c), id, rangeKey, window, metrics, step)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobTimeseriesResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobTimeseriesResponse{Data: result})
}

// GetJobPlanDAG 处理 GET /api/v1/sync/jobs/:id/plan-dag：用提交时的配置推导表结构与建表计划，拿不到时以 available=false 表达。
// GetJobPlanDAG handles GET /api/v1/sync/jobs/:id/plan-dag: it derives table schemas and save-mode plans from the
// submitted config, expressing unavailability as available=false.
// @Summary 获取作业提交配置推导的计划 DAG（表结构与建表计划）
// @Tags Sync
// @Produce json
// @Param id path int true "作业 ID"
// @Success 200 {object} JobPlanDAGResponse
// @Failure 400 {object} JobPlanDAGResponse
// @Failure 403 {object} JobPlanDAGResponse
// @Failure 404 {object} JobPlanDAGResponse
// @Router /api/v1/sync/jobs/{id}/plan-dag [get]
func (h *Handler) GetJobPlanDAG(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobPlanDAGResponse{ErrorMsg: "invalid job id"})
		return
	}
	result, err := h.service.GetJobPlanDAGForActor(c.Request.Context(), currentExecutionActor(c), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobPlanDAGResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobPlanDAGResponse{Data: result})
}

// GetJobLogs handles GET /api/v1/sync/jobs/:id/logs.
func (h *Handler) GetJobLogs(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobLogsResponse{ErrorMsg: "invalid job id"})
		return
	}
	if _, exists := c.GetQuery("lines"); exists {
		c.JSON(http.StatusBadRequest, JobLogsResponse{ErrorMsg: "lines is no longer supported; use offset and limit_bytes"})
		return
	}
	if _, exists := c.GetQuery("all"); exists {
		c.JSON(http.StatusBadRequest, JobLogsResponse{ErrorMsg: "all is no longer supported; use offset and limit_bytes"})
		return
	}
	result, err := h.service.GetJobLogsForActor(
		c.Request.Context(),
		currentExecutionActor(c),
		id,
		c.Query("offset"),
		parseNonNegativeInt(c.Query("limit_bytes"), 0),
		c.Query("keyword"),
		c.Query("level"),
	)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobLogsResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobLogsResponse{Data: sanitizeJobLogsForResponse(result)})
}

// GetPreviewSnapshot handles GET /api/v1/sync/jobs/:id/preview.
func (h *Handler) GetPreviewSnapshot(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, PreviewSnapshotResponse{ErrorMsg: "invalid job id"})
		return
	}
	result, err := h.service.GetPreviewSnapshotForActor(c.Request.Context(), currentExecutionActor(c), id, c.Query("table_path"))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), PreviewSnapshotResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, PreviewSnapshotResponse{Data: sanitizePreviewSnapshotForResponse(result)})
}

// GetJobCheckpointSnapshot handles GET /api/v1/sync/jobs/:id/checkpoint.
func (h *Handler) GetJobCheckpointSnapshot(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, CheckpointSnapshotResponse{ErrorMsg: "invalid job id"})
		return
	}
	var pipelineID *int
	if raw := strings.TrimSpace(c.Query("pipeline_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, CheckpointSnapshotResponse{ErrorMsg: "invalid pipeline_id"})
			return
		}
		pipelineID = &parsed
	}
	result, err := h.service.GetJobCheckpointSnapshotForActor(
		c.Request.Context(),
		currentExecutionActor(c),
		id,
		pipelineID,
		parsePositiveInt(c.Query("limit"), 20),
		c.Query("status"),
	)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CheckpointSnapshotResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CheckpointSnapshotResponse{Data: result})
}

// CollectPreview handles POST /api/v1/sync/preview/collect.
func (h *Handler) CollectPreview(c *gin.Context) {
	rawBody, _ := c.GetRawData()
	c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
	var req PreviewCollectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: err.Error()})
		return
	}
	if strings.TrimSpace(req.PlatformJobID) == "" {
		req.PlatformJobID = strings.TrimSpace(c.Query("platform_job_id"))
	}
	if strings.TrimSpace(req.EngineJobID) == "" {
		req.EngineJobID = strings.TrimSpace(c.Query("engine_job_id"))
	}
	if req.RowLimit <= 0 {
		req.RowLimit = parseNonNegativeInt(c.Query("row_limit"), 0)
	}
	if !validatePreviewCollectAuthToken(strings.TrimSpace(req.PlatformJobID), strings.TrimSpace(c.GetHeader("X-Preview-Token"))) {
		c.JSON(http.StatusUnauthorized, BasicResponse{ErrorMsg: "unauthorized preview collect request"})
		return
	}
	normalizePreviewCollectRequest(&req, rawBody)
	if err := h.service.CollectPreview(c.Request.Context(), &req); err != nil {
		c.JSON(h.getStatusCodeForError(err), BasicResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, BasicResponse{Data: gin.H{"collected": true}})
}

func validatePreviewCollectAuthToken(platformJobID, token string) bool {
	platformJobID = strings.TrimSpace(platformJobID)
	token = strings.TrimSpace(token)
	secret := strings.TrimSpace(config.Config.App.SessionSecret)
	if platformJobID == "" || token == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("preview-collect:"))
	_, _ = mac.Write([]byte(platformJobID))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(token))
}

// RecoverJob handles POST /api/v1/sync/jobs/:id/recover.
func (h *Handler) RecoverJob(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid job id"})
		return
	}
	var req RecoverJobRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
			return
		}
	}
	actor := currentExecutionActor(c)
	sourceJob, err := h.service.GetJobForActor(c.Request.Context(), actor, id, false)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	if sourceJob.Source == JobSourceExternal || sourceJob.TaskID == 0 {
		c.JSON(h.getStatusCodeForError(ErrRecoverSourceRequired), JobResponse{ErrorCode: syncErrorCode(ErrRecoverSourceRequired), ErrorMsg: ErrRecoverSourceRequired.Error()})
		return
	}
	if !sourceJob.CanRecover {
		c.JSON(http.StatusForbidden, JobResponse{ErrorMsg: ErrTaskReadOnly.Error()})
		return
	}
	job, err := h.service.RecoverJobWithExecution(c.Request.Context(), id, getCurrentUserID(c), req.Draft, syncExecutionRequest(c, map[string]any{"source_job_id": id, "draft": req.Draft}))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorCode: syncErrorCode(err), ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// CancelJob handles POST /api/v1/sync/jobs/:id/cancel.
func (h *Handler) CancelJob(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid job id"})
		return
	}
	var req CancelJobRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
		return
	}
	job, err := h.service.CancelJobForActor(c.Request.Context(), currentExecutionActor(c), id, req.StopWithSavepoint)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// CancelJobByRef 处理 POST /api/v1/sync/jobs/cancel-by-ref。
// CancelJobByRef handles POST /api/v1/sync/jobs/cancel-by-ref.
// @Summary 按外部调度引用停止作业
// @Tags Sync
// @Accept json
// @Produce json
// @Param request body CancelByRefRequest true "外部引用与接管序号"
// @Success 200 {object} JobResponse
// @Failure 400 {object} JobResponse
// @Failure 409 {object} JobResponse
// @Router /api/v1/sync/jobs/cancel-by-ref [post]
func (h *Handler) CancelJobByRef(c *gin.Context) {
	var req CancelByRefRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: err.Error()})
		return
	}
	job, err := h.service.CancelJobByRef(c.Request.Context(), currentExecutionActor(c), req.ExternalRef, req.AttachGeneration)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

func (h *Handler) getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrTaskNotFound), errors.Is(err, ErrTaskVersionNotFound), errors.Is(err, ErrJobInstanceNotFound), errors.Is(err, ErrGlobalVariableNotFound), errors.Is(err, ErrCuratedTemplateNotFound), errors.Is(err, ErrSchedulerConnectionNotFound), errors.Is(err, ErrSchedulerBindingNotFound), errors.Is(err, ErrEventChannelNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrEventChannelUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, syncalert.ErrInvalidConfig):
		return http.StatusBadRequest
	case errors.Is(err, syncalert.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrTaskAlertUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrTaskReadOnly), errors.Is(err, ErrTaskPermissionDenied), errors.Is(err, ErrEventChannelPermissionDenied), errors.Is(err, ErrGlobalVariablePermissionDenied), errors.Is(err, ErrCuratedTemplatePermissionDenied), errors.Is(err, ErrCuratedTemplateBuiltinReadonly):
		return http.StatusForbidden
	case errors.Is(err, ErrTaskNameRequired), errors.Is(err, ErrTaskNameInvalid), errors.Is(err, ErrTaskParentCycle), errors.Is(err, ErrRootFileNotAllowed), errors.Is(err, ErrInvalidTaskMode), errors.Is(err, ErrInvalidTaskStatus), errors.Is(err, ErrInvalidRunType), errors.Is(err, ErrInvalidPreviewMode), errors.Is(err, ErrTaskDefinitionEmpty), errors.Is(err, ErrPreviewHTTPSinkEmpty), errors.Is(err, ErrTaskNotPublished), errors.Is(err, ErrInvalidNodeType), errors.Is(err, ErrParentTaskNotFolder), errors.Is(err, ErrFolderContentUnsupported), errors.Is(err, ErrTaskNotFile), errors.Is(err, ErrInvalidContentFormat), errors.Is(err, ErrRecoverSourceRequired), errors.Is(err, ErrLocalClusterRequired), errors.Is(err, ErrLocalSavepointUnsupported), errors.Is(err, ErrPreviewPayloadInvalid), errors.Is(err, ErrGlobalVariableKeyRequired), errors.Is(err, ErrGlobalVariableKeyInvalid), errors.Is(err, ErrReservedBuiltinVariableKey), errors.Is(err, ErrExecutionTargetClusterMismatch), errors.Is(err, ErrMaskedSecretCannotBeRestored), errors.Is(err, ErrInvalidTaskCollaborator), errors.Is(err, ErrInvalidTaskAccessLevel), errors.Is(err, ErrCuratedTemplateNameRequired), errors.Is(err, ErrCuratedTemplateContentRequired), errors.Is(err, ErrCuratedTemplateSectionInvalid), errors.Is(err, ErrCuratedBuiltinIDRequired), errors.Is(err, ErrUnreplacedVariable), errors.Is(err, ErrUndefinedTemplateVariable), errors.Is(err, ErrInvalidScheduleTime), errors.Is(err, ErrSchedulerConnectionInvalid), errors.Is(err, ErrDolphinPushInvalid), errors.Is(err, ErrDolphinCycle), errors.Is(err, ErrScheduleLockedByDolphin), errors.Is(err, ErrDolphinNodeMissing), errors.Is(err, ErrDolphinUnsupportedVersion), errors.Is(err, ErrEventChannelVersionUnsupported), errors.Is(err, ErrEventChannelExternalURLMissing), errors.Is(err, ErrEventChannelSecretMissing), errors.Is(err, ErrInvalidTaskTimeout), errors.Is(err, ErrInvalidJobListQuery):
		return http.StatusBadRequest
	case errors.Is(err, ErrTaskArchived), errors.Is(err, ErrJobAlreadyFinished), errors.Is(err, ErrJobStatusChanged), errors.Is(err, ErrGlobalVariableKeyDuplicate), errors.Is(err, ErrTaskNameDuplicate), errors.Is(err, executionapp.ErrIdempotencyConflict), errors.Is(err, ErrAttachGenerationMismatch), errors.Is(err, ErrJobCancellingTimeout), errors.Is(err, ErrDolphinVersionConflict), errors.Is(err, ErrSchedulerBindingExists), errors.Is(err, ErrDolphinPushNoNewVersion), errors.Is(err, ErrSubmitOutcomePending):
		return http.StatusConflict
	case errors.Is(err, ErrConnectorMissing), errors.Is(err, ErrConnectorInitFailed):
		// 集群状态不满足任务前提，重试不会成功；409 让 CLI 归为冲突而不是可重试的服务端错误。
		// The cluster does not meet the job's precondition and retrying will not help; 409 keeps the CLI from treating it as a retryable server error.
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(value), true
}

func parsePositiveInt(raw string, defaultValue int) int {
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func parseNonNegativeInt(raw string, defaultValue int) int {
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return defaultValue
	}
	return value
}

func isTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func getCurrentUserID(c *gin.Context) uint {
	if c == nil {
		return 0
	}
	if user := auth.GetUserFromContext(c); user != nil {
		return uint(user.ID)
	}
	value, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	switch v := value.(type) {
	case uint:
		return v
	case int:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}

func currentExecutionActor(c *gin.Context) executionapp.Actor {
	user := auth.GetUserFromContext(c)
	if user == nil {
		return executionapp.Actor{UserID: uint64(getCurrentUserID(c))}
	}
	return executionapp.Actor{UserID: uint64(user.ID), IsAdmin: user.IsAdmin}
}

func syncExecutionRequest(c *gin.Context, value any) ExecutionRequest {
	metadata := executionapp.MetadataFromGin(c)
	requestHash, err := executionapp.HashRequest(value)
	if err != nil {
		requestHash = executionapp.HashString("sync-request")
	}
	return ExecutionRequest{
		RequestID:      metadata.RequestID,
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		ClientType:     metadata.ClientType,
	}
}
