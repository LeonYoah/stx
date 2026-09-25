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

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
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
	ErrorMsg string       `json:"error_msg"`
	Data     *JobInstance `json:"data"`
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
	task, err := h.service.CreateTask(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskResponse{Data: sanitizeTaskForResponse(task)})
}

// ListTasks handles GET /api/v1/sync/tasks.
func (h *Handler) ListTasks(c *gin.Context) {
	filter := &TaskFilter{Name: c.Query("name"), Page: parsePositiveInt(c.Query("current"), 1), Size: parsePositiveInt(c.Query("size"), 200)}
	if status := c.Query("status"); status != "" {
		filter.Status = TaskStatus(status)
	}
	tasks, total, err := h.service.ListTasks(c.Request.Context(), filter)
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
	c.JSON(http.StatusOK, TaskResponse{Data: sanitizeTaskForResponse(task)})
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
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
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
	job, err := h.service.SubmitTaskForActor(c.Request.Context(), currentExecutionActor(c), id, req.Draft, syncExecutionRequest(c, map[string]any{"task_id": id, "draft": req.Draft}))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
}

// ListJobs handles GET /api/v1/sync/jobs.
func (h *Handler) ListJobs(c *gin.Context) {
	filter := &JobFilter{
		Page:          parsePositiveInt(c.Query("current"), 1),
		Size:          parsePositiveInt(c.Query("size"), 50),
		PlatformJobID: strings.TrimSpace(c.Query("platform_job_id")),
		EngineJobID:   strings.TrimSpace(c.Query("engine_job_id")),
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
	jobs, total, err := h.service.ListJobsForActor(c.Request.Context(), currentExecutionActor(c), filter)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobListResponse{Data: &JobListData{Total: total, Items: sanitizeJobsForResponse(jobs)}})
}

// GetJob handles GET /api/v1/sync/jobs/:id.
func (h *Handler) GetJob(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobResponse{ErrorMsg: "invalid job id"})
		return
	}
	job, err := h.service.GetJobForActor(c.Request.Context(), currentExecutionActor(c), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobResponse{Data: sanitizeJobForResponse(job)})
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
	sourceJob, err := h.service.GetJobForActor(c.Request.Context(), actor, id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
		return
	}
	if sourceJob.TaskID > 0 {
		task, err := h.service.GetTaskForActor(c.Request.Context(), actor, sourceJob.TaskID)
		if err != nil {
			c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
			return
		}
		if !task.CanUserRun(uint(actor.UserID), actor.IsAdmin) {
			c.JSON(http.StatusForbidden, JobResponse{ErrorMsg: ErrTaskReadOnly.Error()})
			return
		}
	}
	job, err := h.service.RecoverJobWithExecution(c.Request.Context(), id, getCurrentUserID(c), req.Draft, syncExecutionRequest(c, map[string]any{"source_job_id": id, "draft": req.Draft}))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobResponse{ErrorMsg: err.Error()})
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

func (h *Handler) getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrTaskNotFound), errors.Is(err, ErrTaskVersionNotFound), errors.Is(err, ErrJobInstanceNotFound), errors.Is(err, ErrGlobalVariableNotFound), errors.Is(err, ErrCuratedTemplateNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrTaskReadOnly), errors.Is(err, ErrTaskPermissionDenied), errors.Is(err, ErrGlobalVariablePermissionDenied), errors.Is(err, ErrCuratedTemplatePermissionDenied), errors.Is(err, ErrCuratedTemplateBuiltinReadonly):
		return http.StatusForbidden
	case errors.Is(err, ErrTaskNameRequired), errors.Is(err, ErrTaskNameInvalid), errors.Is(err, ErrTaskParentCycle), errors.Is(err, ErrRootFileNotAllowed), errors.Is(err, ErrInvalidTaskMode), errors.Is(err, ErrInvalidTaskStatus), errors.Is(err, ErrInvalidRunType), errors.Is(err, ErrInvalidPreviewMode), errors.Is(err, ErrTaskDefinitionEmpty), errors.Is(err, ErrPreviewHTTPSinkEmpty), errors.Is(err, ErrTaskNotPublished), errors.Is(err, ErrInvalidNodeType), errors.Is(err, ErrParentTaskNotFolder), errors.Is(err, ErrFolderContentUnsupported), errors.Is(err, ErrTaskNotFile), errors.Is(err, ErrInvalidContentFormat), errors.Is(err, ErrRecoverSourceRequired), errors.Is(err, ErrLocalClusterRequired), errors.Is(err, ErrLocalSavepointUnsupported), errors.Is(err, ErrPreviewPayloadInvalid), errors.Is(err, ErrGlobalVariableKeyRequired), errors.Is(err, ErrGlobalVariableKeyInvalid), errors.Is(err, ErrReservedBuiltinVariableKey), errors.Is(err, ErrExecutionTargetClusterMismatch), errors.Is(err, ErrMaskedSecretCannotBeRestored), errors.Is(err, ErrInvalidTaskCollaborator), errors.Is(err, ErrCuratedTemplateNameRequired), errors.Is(err, ErrCuratedTemplateContentRequired), errors.Is(err, ErrCuratedTemplateSectionInvalid), errors.Is(err, ErrCuratedBuiltinIDRequired):
		return http.StatusBadRequest
	case errors.Is(err, ErrTaskArchived), errors.Is(err, ErrJobAlreadyFinished), errors.Is(err, ErrJobStatusChanged), errors.Is(err, ErrGlobalVariableKeyDuplicate), errors.Is(err, ErrTaskNameDuplicate), errors.Is(err, executionapp.ErrIdempotencyConflict):
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
