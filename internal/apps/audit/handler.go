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

// Package audit provides command logging and audit trail functionality for the STX Agent system.
// 审计包提供 STX Agent 系统的命令日志和审计追踪功能。
package audit

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for audit and command log operations.
// Handler 提供审计和命令日志操作的 HTTP 处理器。
type Handler struct {
	repo *Repository
}

// NewHandler creates a new Handler instance.
// NewHandler 创建一个新的 Handler 实例。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ==================== Request/Response Types 请求/响应类型 ====================

// ListCommandLogsRequest represents the request for listing command logs.
// ListCommandLogsRequest 表示获取命令日志列表的请求。
type ListCommandLogsRequest struct {
	Current     int           `json:"current" form:"current" binding:"min=1"`
	Size        int           `json:"size" form:"size" binding:"min=1,max=100"`
	CommandID   string        `json:"command_id" form:"command_id"`
	AgentID     string        `json:"agent_id" form:"agent_id"`
	HostID      *uint         `json:"host_id" form:"host_id"`
	CommandType string        `json:"command_type" form:"command_type"`
	RequestID   string        `json:"request_id" form:"request_id"`
	Status      CommandStatus `json:"status" form:"status"`
	StartTime   string        `json:"start_time" form:"start_time"`
	EndTime     string        `json:"end_time" form:"end_time"`
}

// ListCommandLogsResponse represents the response for listing command logs.
// ListCommandLogsResponse 表示获取命令日志列表的响应。
type ListCommandLogsResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Total    int64             `json:"total"`
		Commands []*CommandLogInfo `json:"commands"`
	} `json:"data"`
}

// GetCommandLogResponse represents the response for getting a command log.
// GetCommandLogResponse 表示获取命令日志详情的响应。
type GetCommandLogResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *CommandLogInfo `json:"data"`
}

// ListAuditLogsRequest represents the request for listing audit logs.
// ListAuditLogsRequest 表示获取审计日志列表的请求。
type ListAuditLogsRequest struct {
	Current      int    `json:"current" form:"current" binding:"min=1"`
	Size         int    `json:"size" form:"size" binding:"min=1,max=100"`
	UserID       *uint  `json:"user_id" form:"user_id"`
	Username     string `json:"username" form:"username"`
	Action       string `json:"action" form:"action"`
	ActionGroup  string `json:"action_group" form:"action_group"`
	ResourceType string `json:"resource_type" form:"resource_type"`
	ResourceID   string `json:"resource_id" form:"resource_id"`
	RequestID    string `json:"request_id" form:"request_id"`
	ExecutionID  string `json:"execution_id" form:"execution_id"`
	CommandID    string `json:"command_id" form:"command_id"`
	ClientType   string `json:"client_type" form:"client_type"`
	ResultStatus string `json:"result_status" form:"result_status"`
	Trigger      string `json:"trigger" form:"trigger"` // "auto" | "manual"
	StartTime    string `json:"start_time" form:"start_time"`
	EndTime      string `json:"end_time" form:"end_time"`
}

// ListAuditLogsResponse represents the response for listing audit logs.
// ListAuditLogsResponse 表示获取审计日志列表的响应。
type ListAuditLogsResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Total int64           `json:"total"`
		Logs  []*AuditLogInfo `json:"logs"`
	} `json:"data"`
}

// GetAuditLogResponse represents the response for getting an audit log.
// GetAuditLogResponse 表示获取审计日志详情的响应。
type GetAuditLogResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *AuditLogInfo `json:"data"`
}

// ==================== Command Log Handlers 命令日志处理器 ====================

// ListCommandLogs handles GET /api/v1/commands - lists command logs with filtering and pagination.
// ListCommandLogs 处理 GET /api/v1/commands - 获取命令日志列表（支持过滤和分页）。
// @Tags audit
// @Param request query ListCommandLogsRequest true "查询参数"
// @Produce json
// @Success 200 {object} ListCommandLogsResponse
// @Router /api/v1/commands [get]
// Requirements: 10.1, 10.4
func (h *Handler) ListCommandLogs(c *gin.Context) {
	req := &ListCommandLogsRequest{Current: 1, Size: 20}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListCommandLogsResponse{ErrorMsg: err.Error()})
		return
	}

	// Parse time filters - 解析时间过滤条件
	var startTime, endTime *time.Time
	if req.StartTime != "" {
		t, err := time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, ListCommandLogsResponse{
				ErrorMsg: "无效的开始时间格式，请使用 RFC3339 格式 / Invalid start_time format, use RFC3339",
			})
			return
		}
		startTime = &t
	}
	if req.EndTime != "" {
		t, err := time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, ListCommandLogsResponse{
				ErrorMsg: "无效的结束时间格式，请使用 RFC3339 格式 / Invalid end_time format, use RFC3339",
			})
			return
		}
		utc := t.UTC()
		y, m, d := utc.Date()
		endOfDay := time.Date(y, m, d, 23, 59, 59, 999999999, time.UTC)
		endTime = &endOfDay
	}

	// Build filter from request - 从请求构建过滤条件
	filter := &CommandLogFilter{
		CommandID:   req.CommandID,
		AgentID:     req.AgentID,
		HostID:      req.HostID,
		CommandType: req.CommandType,
		RequestID:   req.RequestID,
		Status:      req.Status,
		StartTime:   startTime,
		EndTime:     endTime,
		Page:        req.Current,
		PageSize:    req.Size,
	}
	user := auth.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusForbidden, ListCommandLogsResponse{ErrorMsg: "permission denied"})
		return
	}
	if !user.IsAdmin {
		ownerUserID := uint(user.ID)
		filter.CreatedBy = &ownerUserID
	}

	logs, total, err := h.repo.ListCommandLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListCommandLogsResponse{ErrorMsg: err.Error()})
		return
	}

	// Convert to response format - 转换为响应格式
	commands := make([]*CommandLogInfo, len(logs))
	for i, log := range logs {
		commands[i] = log.ToCommandLogInfo()
	}

	c.JSON(http.StatusOK, ListCommandLogsResponse{
		Data: &struct {
			Total    int64             `json:"total"`
			Commands []*CommandLogInfo `json:"commands"`
		}{
			Total:    total,
			Commands: commands,
		},
	})
}

// GetCommandLog handles GET /api/v1/commands/:id - gets a command log by ID.
// GetCommandLog 处理 GET /api/v1/commands/:id - 根据 ID 获取命令日志详情。
// @Tags audit
// @Produce json
// @Param id path int true "命令日志ID"
// @Success 200 {object} GetCommandLogResponse
// @Router /api/v1/commands/{id} [get]
// Requirements: 10.1
func (h *Handler) GetCommandLog(c *gin.Context) {
	logID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetCommandLogResponse{
			ErrorMsg: "无效的命令日志 ID / Invalid command log ID",
		})
		return
	}

	user := auth.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusForbidden, GetCommandLogResponse{ErrorMsg: "permission denied"})
		return
	}
	log, err := h.repo.GetCommandLogByIDForOwner(c.Request.Context(), uint(logID), uint(user.ID), user.IsAdmin)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetCommandLogResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetCommandLogResponse{Data: log.ToCommandLogInfo()})
}

// ==================== Audit Log Handlers 审计日志处理器 ====================

// ListAuditLogs handles GET /api/v1/audit-logs - lists audit logs with filtering and pagination.
// ListAuditLogs 处理 GET /api/v1/audit-logs - 获取审计日志列表（支持过滤和分页）。
// @Tags audit
// @Param request query ListAuditLogsRequest true "查询参数"
// @Produce json
// @Success 200 {object} ListAuditLogsResponse
// @Router /api/v1/audit-logs [get]
// Requirements: 10.4
func (h *Handler) ListAuditLogs(c *gin.Context) {
	req := &ListAuditLogsRequest{Current: 1, Size: 20}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListAuditLogsResponse{ErrorMsg: err.Error()})
		return
	}

	// Parse time filters - 解析时间过滤条件
	var startTime, endTime *time.Time
	if req.StartTime != "" {
		t, err := time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, ListAuditLogsResponse{
				ErrorMsg: "无效的开始时间格式，请使用 RFC3339 格式 / Invalid start_time format, use RFC3339",
			})
			return
		}
		startTime = &t
	}
	if req.EndTime != "" {
		t, err := time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, ListAuditLogsResponse{
				ErrorMsg: "无效的结束时间格式，请使用 RFC3339 格式 / Invalid end_time format, use RFC3339",
			})
			return
		}
		// 结束时间规范为“该日”在 UTC 的当日 23:59:59.999，避免选“今天”时漏掉当天数据（时区/存储差异）
		utc := t.UTC()
		y, m, d := utc.Date()
		endOfDay := time.Date(y, m, d, 23, 59, 59, 999999999, time.UTC)
		endTime = &endOfDay
	}

	// Build filter from request - 从请求构建过滤条件
	filter := &AuditLogFilter{
		UserID:       req.UserID,
		Username:     req.Username,
		Action:       req.Action,
		ActionGroup:  req.ActionGroup,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		RequestID:    req.RequestID,
		ExecutionID:  req.ExecutionID,
		CommandID:    req.CommandID,
		ClientType:   req.ClientType,
		ResultStatus: req.ResultStatus,
		Trigger:      req.Trigger,
		StartTime:    startTime,
		EndTime:      endTime,
		Page:         req.Current,
		PageSize:     req.Size,
	}
	user := auth.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusForbidden, ListAuditLogsResponse{ErrorMsg: "permission denied"})
		return
	}
	if user.IsAdmin {
		filter.IncludeAll = true
	} else {
		ownerUserID := uint(user.ID)
		filter.UserID = &ownerUserID
		filter.Username = ""
	}

	logs, total, err := h.repo.ListAuditLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListAuditLogsResponse{ErrorMsg: err.Error()})
		return
	}

	// Convert to response format - 转换为响应格式
	auditLogs := make([]*AuditLogInfo, len(logs))
	requestIDs := make([]string, 0, len(logs))
	for i, log := range logs {
		auditLogs[i] = log.ToAuditLogInfo()
		if log.RequestID != "" {
			requestIDs = append(requestIDs, log.RequestID)
		}
	}
	if counts, countErr := h.repo.CountCommandsByRequestIDs(c.Request.Context(), requestIDs); countErr == nil {
		for _, item := range auditLogs {
			if item != nil && item.RequestID != "" {
				item.CommandCount = int(counts[item.RequestID])
			}
		}
	}

	c.JSON(http.StatusOK, ListAuditLogsResponse{
		Data: &struct {
			Total int64           `json:"total"`
			Logs  []*AuditLogInfo `json:"logs"`
		}{
			Total: total,
			Logs:  auditLogs,
		},
	})
}

// GetAuditLog handles GET /api/v1/audit-logs/:id - gets an audit log by ID.
// GetAuditLog 处理 GET /api/v1/audit-logs/:id - 根据 ID 获取审计日志详情。
// @Tags audit
// @Produce json
// @Param id path int true "审计日志ID"
// @Success 200 {object} GetAuditLogResponse
// @Router /api/v1/audit-logs/{id} [get]
// Requirements: 10.4
func (h *Handler) GetAuditLog(c *gin.Context) {
	logID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetAuditLogResponse{
			ErrorMsg: "无效的审计日志 ID / Invalid audit log ID",
		})
		return
	}

	user := auth.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusForbidden, GetAuditLogResponse{ErrorMsg: "permission denied"})
		return
	}
	log, err := h.repo.GetAuditLogByIDForOwner(c.Request.Context(), uint(logID), uint(user.ID), user.IsAdmin)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetAuditLogResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetAuditLogResponse{Data: log.ToAuditLogInfo()})
}

// ==================== Helper Methods 辅助方法 ====================

// getStatusCodeForError returns the appropriate HTTP status code for an error.
// getStatusCodeForError 根据错误返回适当的 HTTP 状态码。
func (h *Handler) getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrCommandLogNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAuditLogNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrCommandIDDuplicate):
		return http.StatusConflict
	case errors.Is(err, ErrCommandIDEmpty),
		errors.Is(err, ErrAgentIDEmpty),
		errors.Is(err, ErrCommandTypeEmpty),
		errors.Is(err, ErrActionEmpty),
		errors.Is(err, ErrResourceTypeEmpty):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
