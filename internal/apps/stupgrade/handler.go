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

package stupgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	"github.com/gin-gonic/gin"
)

// Handler 处理升级预检查与计划接口。
// Handler handles upgrade precheck and plan APIs.
type Handler struct {
	service *Service
}

// Response 统一响应结构。
// Response is the common response envelope.
type Response struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// NewHandler 创建升级处理器。
// NewHandler creates an upgrade handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RunPrecheck 处理升级预检查请求。
// RunPrecheck handles the upgrade precheck request.
func (h *Handler) RunPrecheck(c *gin.Context) {
	var req PrecheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	result, err := h.service.RunPrecheck(c.Request.Context(), &req)
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}

	statusCode := http.StatusOK
	if !result.Ready {
		statusCode = http.StatusConflict
	}
	c.JSON(statusCode, Response{Data: result})
}

// CreatePlan 处理升级计划生成请求。
// CreatePlan handles the upgrade plan creation request.
func (h *Handler) CreatePlan(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	result, err := h.service.CreatePlanFromRequest(c.Request.Context(), &req, uint(auth.GetUserIDFromContext(c)))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "st upgrade plan result is empty"})
		return
	}

	statusCode := http.StatusCreated
	if result.Precheck != nil && !result.Precheck.Ready {
		statusCode = http.StatusConflict
	}
	c.JSON(statusCode, Response{Data: result})
}

// GetPlan 获取升级计划详情。
// GetPlan gets the upgrade plan detail.
func (h *Handler) GetPlan(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid plan id"})
		return
	}

	plan, err := h.service.GetPlan(c.Request.Context(), uint(planID))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: plan})
}

// ExecutePlan 启动升级计划执行。
// ExecutePlan starts the execution of an upgrade plan.
func (h *Handler) ExecutePlan(c *gin.Context) {
	var req ExecutePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	task, err := h.service.StartPlanExecution(c.Request.Context(), req.PlanID, uint(auth.GetUserIDFromContext(c)))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, Response{Data: task})
}

// GetTask 获取升级任务详情。
// GetTask gets the upgrade task detail.
func (h *Handler) GetTask(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid task id"})
		return
	}

	task, err := h.service.GetTaskDetail(c.Request.Context(), uint(taskID))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: task})
}

// ListTasks 获取升级任务列表。
// ListTasks gets the paginated upgrade task list.
func (h *Handler) ListTasks(c *gin.Context) {
	filter, err := buildTaskListFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	items, total, err := h.service.ListTasks(c.Request.Context(), filter)
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: gin.H{
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
		"items":     items,
	}})
}

// ListTaskSteps 获取升级任务步骤与节点执行详情。
// ListTaskSteps gets the upgrade task steps and node execution details.
func (h *Handler) ListTaskSteps(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid task id"})
		return
	}

	steps, err := h.service.ListTaskSteps(c.Request.Context(), uint(taskID))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}
	nodes, err := h.service.ListNodeExecutions(c.Request.Context(), uint(taskID))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: gin.H{
		"task_id":         uint(taskID),
		"steps":           steps,
		"node_executions": nodes,
	}})
}

// ListTaskLogs 查询升级任务日志。
// ListTaskLogs queries the upgrade task logs.
func (h *Handler) ListTaskLogs(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid task id"})
		return
	}

	filter, err := buildStepLogFilter(c, uint(taskID))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	logs, total, err := h.service.ListStepLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: gin.H{
		"task_id":   uint(taskID),
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
		"items":     logs,
	}})
}

// StreamTaskEvents 通过 SSE 推送升级任务事件流。
// StreamTaskEvents pushes the upgrade task event stream via SSE.
func (h *Handler) StreamTaskEvents(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid task id"})
		return
	}

	task, err := h.service.GetTaskDetail(c.Request.Context(), uint(taskID))
	if err != nil {
		c.JSON(getStatusCodeForError(err), Response{ErrorMsg: err.Error()})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "streaming is not supported"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event TaskEvent) bool {
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: %q\n\n", marshalErr.Error())
			flusher.Flush()
			return false
		}
		_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.EventType, payload)
		flusher.Flush()
		return true
	}

	if !writeEvent(newTaskSnapshotEvent(task)) {
		return
	}

	events, unsubscribe := h.service.SubscribeTaskEvents(uint(taskID))
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if !writeEvent(event) {
				return
			}
		case <-keepAliveTicker.C:
			_, _ = fmt.Fprint(c.Writer, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrUpgradePlanNotReady):
		return http.StatusConflict
	case errors.Is(err, ErrUpgradePlanNotFound), errors.Is(err, ErrUpgradeTaskNotFound), errors.Is(err, clusterapp.ErrClusterNotFound), errors.Is(err, hostapp.ErrHostNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func buildTaskListFilter(c *gin.Context) (*TaskListFilter, error) {
	filter := &TaskListFilter{
		Page:     1,
		PageSize: 10,
	}

	if clusterID := c.Query("cluster_id"); clusterID != "" {
		value, err := strconv.ParseUint(clusterID, 10, 32)
		if err != nil || value == 0 {
			return nil, fmt.Errorf("invalid cluster_id")
		}
		filter.ClusterID = uint(value)
	}
	if page := c.Query("page"); page != "" {
		value, err := strconv.Atoi(page)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid page")
		}
		filter.Page = value
	}
	if pageSize := c.Query("page_size"); pageSize != "" {
		value, err := strconv.Atoi(pageSize)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid page_size")
		}
		filter.PageSize = value
	}
	return filter, nil
}

func buildStepLogFilter(c *gin.Context, taskID uint) (*StepLogFilter, error) {
	filter := &StepLogFilter{
		TaskID:   taskID,
		Page:     1,
		PageSize: 50,
	}
	if stepCode := c.Query("step_code"); stepCode != "" {
		filter.StepCode = StepCode(stepCode)
	}
	if level := c.Query("level"); level != "" {
		filter.Level = LogLevel(level)
	}
	if nodeExecutionID := c.Query("node_execution_id"); nodeExecutionID != "" {
		value, err := strconv.ParseUint(nodeExecutionID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid node_execution_id")
		}
		nodeID := uint(value)
		filter.NodeExecutionID = &nodeID
	}
	if page := c.Query("page"); page != "" {
		value, err := strconv.Atoi(page)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid page")
		}
		filter.Page = value
	}
	if pageSize := c.Query("page_size"); pageSize != "" {
		value, err := strconv.Atoi(pageSize)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid page_size")
		}
		filter.PageSize = value
	}
	return filter, nil
}
