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
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type SchedulerConnectionResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *SchedulerConnection `json:"data"`
}

type SchedulerConnectionListResponse struct {
	ErrorMsg string                       `json:"error_msg"`
	Data     *SchedulerConnectionListData `json:"data"`
}

type DolphinProjectListResponse struct {
	ErrorMsg string           `json:"error_msg"`
	Data     []DolphinProject `json:"data"`
}

type DolphinWorkflowListResponse struct {
	ErrorMsg string                   `json:"error_msg"`
	Data     []DolphinWorkflowSummary `json:"data"`
}

type DolphinWorkflowDetailResponse struct {
	ErrorMsg string                 `json:"error_msg"`
	Data     *DolphinWorkflowDetail `json:"data"`
}

type DolphinScriptResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *DolphinScriptResult `json:"data"`
}

type DolphinPushResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *DolphinPushResult `json:"data"`
}

type DolphinTaskStatusResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *DolphinTaskStatus `json:"data"`
}

// ListSchedulerConnections 处理 GET /api/v1/sync/scheduler-connections。
// ListSchedulerConnections handles GET /api/v1/sync/scheduler-connections.
// @Summary 列出调度器连接
// @Tags Sync
// @Produce json
// @Success 200 {object} SchedulerConnectionListResponse
// @Router /api/v1/sync/scheduler-connections [get]
func (h *Handler) ListSchedulerConnections(c *gin.Context) {
	items, err := h.service.ListSchedulerConnections(c.Request.Context())
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), SchedulerConnectionListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, SchedulerConnectionListResponse{Data: &SchedulerConnectionListData{Items: items}})
}

// CreateSchedulerConnection 处理 POST /api/v1/sync/scheduler-connections。
// CreateSchedulerConnection handles POST /api/v1/sync/scheduler-connections.
// @Summary 创建调度器连接
// @Tags Sync
// @Accept json
// @Produce json
// @Param request body SchedulerConnectionRequest true "连接信息"
// @Success 200 {object} SchedulerConnectionResponse
// @Failure 400 {object} SchedulerConnectionResponse
// @Router /api/v1/sync/scheduler-connections [post]
func (h *Handler) CreateSchedulerConnection(c *gin.Context) {
	var req SchedulerConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, SchedulerConnectionResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.CreateSchedulerConnection(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), SchedulerConnectionResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, SchedulerConnectionResponse{Data: item})
}

// UpdateSchedulerConnection 处理 PUT /api/v1/sync/scheduler-connections/:id。
// UpdateSchedulerConnection handles PUT /api/v1/sync/scheduler-connections/:id.
// @Summary 更新调度器连接
// @Tags Sync
// @Accept json
// @Produce json
// @Param id path int true "连接 ID"
// @Param request body SchedulerConnectionRequest true "连接信息，空 token 表示保留原值"
// @Success 200 {object} SchedulerConnectionResponse
// @Failure 400 {object} SchedulerConnectionResponse
// @Failure 404 {object} SchedulerConnectionResponse
// @Router /api/v1/sync/scheduler-connections/{id} [put]
func (h *Handler) UpdateSchedulerConnection(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, SchedulerConnectionResponse{ErrorMsg: "invalid connection id"})
		return
	}
	var req SchedulerConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, SchedulerConnectionResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.UpdateSchedulerConnection(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), SchedulerConnectionResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, SchedulerConnectionResponse{Data: item})
}

// ListDolphinProjects 处理 GET /api/v1/sync/scheduler-connections/:id/projects。
// ListDolphinProjects handles GET /api/v1/sync/scheduler-connections/:id/projects.
// @Summary 列出 Dolphin 项目
// @Tags Sync
// @Produce json
// @Param id path int true "连接 ID"
// @Success 200 {object} DolphinProjectListResponse
// @Failure 400 {object} DolphinProjectListResponse
// @Router /api/v1/sync/scheduler-connections/{id}/projects [get]
func (h *Handler) ListDolphinProjects(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinProjectListResponse{ErrorMsg: "invalid connection id"})
		return
	}
	items, err := h.service.ListDolphinProjects(c.Request.Context(), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinProjectListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinProjectListResponse{Data: items})
}

// ListDolphinWorkflows 处理 GET /api/v1/sync/scheduler-connections/:id/projects/:projectCode/workflows。
// ListDolphinWorkflows handles GET /api/v1/sync/scheduler-connections/:id/projects/:projectCode/workflows.
// @Summary 列出 Dolphin 工作流
// @Tags Sync
// @Produce json
// @Param id path int true "连接 ID"
// @Param projectCode path int true "项目 code"
// @Success 200 {object} DolphinWorkflowListResponse
// @Failure 400 {object} DolphinWorkflowListResponse
// @Router /api/v1/sync/scheduler-connections/{id}/projects/{projectCode}/workflows [get]
func (h *Handler) ListDolphinWorkflows(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinWorkflowListResponse{ErrorMsg: "invalid connection id"})
		return
	}
	projectCode, ok := parseInt64Param(c, "projectCode")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinWorkflowListResponse{ErrorMsg: "invalid project code"})
		return
	}
	items, err := h.service.ListDolphinWorkflows(c.Request.Context(), id, projectCode)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinWorkflowListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinWorkflowListResponse{Data: items})
}

// GetDolphinWorkflow 处理 GET /api/v1/sync/scheduler-connections/:id/projects/:projectCode/workflows/:workflowCode。
// GetDolphinWorkflow handles GET /api/v1/sync/scheduler-connections/:id/projects/:projectCode/workflows/:workflowCode.
// @Summary 读取 Dolphin 工作流定义
// @Tags Sync
// @Produce json
// @Param id path int true "连接 ID"
// @Param projectCode path int true "项目 code"
// @Param workflowCode path int true "工作流 code"
// @Success 200 {object} DolphinWorkflowDetailResponse
// @Failure 400 {object} DolphinWorkflowDetailResponse
// @Router /api/v1/sync/scheduler-connections/{id}/projects/{projectCode}/workflows/{workflowCode} [get]
func (h *Handler) GetDolphinWorkflow(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinWorkflowDetailResponse{ErrorMsg: "invalid connection id"})
		return
	}
	projectCode, ok := parseInt64Param(c, "projectCode")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinWorkflowDetailResponse{ErrorMsg: "invalid project code"})
		return
	}
	workflowCode, ok := parseInt64Param(c, "workflowCode")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinWorkflowDetailResponse{ErrorMsg: "invalid workflow code"})
		return
	}
	item, err := h.service.GetDolphinWorkflow(c.Request.Context(), id, projectCode, workflowCode)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinWorkflowDetailResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinWorkflowDetailResponse{Data: item})
}

// GetDolphinTaskStatus 处理 GET /api/v1/sync/tasks/:id/dolphin。
// GetDolphinTaskStatus handles GET /api/v1/sync/tasks/:id/dolphin.
// @Summary 获取任务的 Dolphin 绑定状态
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Success 200 {object} DolphinTaskStatusResponse
// @Failure 400 {object} DolphinTaskStatusResponse
// @Failure 404 {object} DolphinTaskStatusResponse
// @Router /api/v1/sync/tasks/{id}/dolphin [get]
func (h *Handler) GetDolphinTaskStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinTaskStatusResponse{ErrorMsg: "invalid task id"})
		return
	}
	item, err := h.service.GetDolphinTaskStatus(c.Request.Context(), currentExecutionActor(c), id)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinTaskStatusResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinTaskStatusResponse{Data: item})
}

// GetDolphinScript 处理 GET /api/v1/sync/tasks/:id/dolphin/script。
// GetDolphinScript handles GET /api/v1/sync/tasks/:id/dolphin/script.
// @Summary 生成 Dolphin SHELL 节点脚本
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Param worker_group query string false "Dolphin worker group"
// @Param environment query string false "Dolphin worker environment"
// @Success 200 {object} DolphinScriptResponse
// @Failure 400 {object} DolphinScriptResponse
// @Router /api/v1/sync/tasks/{id}/dolphin/script [get]
func (h *Handler) GetDolphinScript(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinScriptResponse{ErrorMsg: "invalid task id"})
		return
	}
	variables := map[string]string{}
	for _, item := range c.QueryArray("var") {
		key, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		variables[strings.TrimSpace(key)] = value
	}
	item, err := h.service.GetDolphinScript(
		c.Request.Context(),
		currentExecutionActor(c),
		id,
		variables,
		c.Query("worker_group"),
		c.Query("environment"),
	)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinScriptResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinScriptResponse{Data: item})
}

// PushDolphin 处理 POST /api/v1/sync/tasks/:id/dolphin/push。
// PushDolphin handles POST /api/v1/sync/tasks/:id/dolphin/push.
// @Summary 把同步任务推送到 DolphinScheduler
// @Tags Sync
// @Accept json
// @Produce json
// @Param id path int true "任务 ID"
// @Param request body DolphinPushRequest true "推送请求"
// @Success 200 {object} DolphinPushResponse
// @Failure 400 {object} DolphinPushResponse
// @Failure 409 {object} DolphinPushResponse
// @Router /api/v1/sync/tasks/{id}/dolphin/push [post]
func (h *Handler) PushDolphin(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, DolphinPushResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req DolphinPushRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, DolphinPushResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.PushDolphin(c.Request.Context(), currentExecutionActor(c), id, &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), DolphinPushResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DolphinPushResponse{Data: item})
}

// UnbindDolphin 处理 POST /api/v1/sync/tasks/:id/dolphin/unbind。
// UnbindDolphin handles POST /api/v1/sync/tasks/:id/dolphin/unbind.
// @Summary 解除任务与 Dolphin 的绑定
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Success 200 {object} BasicResponse
// @Failure 400 {object} BasicResponse
// @Router /api/v1/sync/tasks/{id}/dolphin/unbind [post]
func (h *Handler) UnbindDolphin(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, BasicResponse{ErrorMsg: "invalid task id"})
		return
	}
	if err := h.service.UnbindDolphin(c.Request.Context(), currentExecutionActor(c), id); err != nil {
		c.JSON(h.getStatusCodeForError(err), BasicResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, BasicResponse{Data: gin.H{"unbound": true}})
}

func parseInt64Param(c *gin.Context, name string) (int64, bool) {
	value, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}
