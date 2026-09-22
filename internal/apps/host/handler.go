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

// Package host provides host management functionality for the STX Agent system.
// host 包提供 STX Agent 系统的主机管理功能。
package host

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for host management operations.
// Handler 提供主机管理操作的 HTTP 处理器。
type Handler struct {
	service   *Service
	auditRepo *audit.Repository
}

// NewHandler creates a new Handler instance.
// NewHandler 创建一个新的 Handler 实例。
// auditRepo may be nil; audit logging is skipped when nil.
func NewHandler(service *Service, auditRepo *audit.Repository) *Handler {
	return &Handler{service: service, auditRepo: auditRepo}
}

// ==================== Request/Response Types 请求/响应类型 ====================

// ListHostsRequest represents the request for listing hosts.
// ListHostsRequest 表示获取主机列表的请求。
type ListHostsRequest struct {
	Current     int         `json:"current" form:"current" binding:"min=1"`
	Size        int         `json:"size" form:"size" binding:"min=1,max=100"`
	Name        string      `json:"name" form:"name"`
	HostType    HostType    `json:"host_type" form:"host_type"`
	IPAddress   string      `json:"ip_address" form:"ip_address"`
	Status      HostStatus  `json:"status" form:"status"`
	AgentStatus AgentStatus `json:"agent_status" form:"agent_status"`
	IsOnline    *bool       `json:"is_online" form:"is_online"`
}

// ListHostsResponse represents the response for listing hosts.
// ListHostsResponse 表示获取主机列表的响应。
type ListHostsResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Total int64       `json:"total"`
		Hosts []*HostInfo `json:"hosts"`
	} `json:"data"`
}

// CreateHostResponse represents the response for creating a host.
// CreateHostResponse 表示创建主机的响应。
type CreateHostResponse struct {
	ErrorMsg string    `json:"error_msg"`
	Data     *HostInfo `json:"data"`
}

// GetHostResponse represents the response for getting a host.
// GetHostResponse 表示获取主机详情的响应。
type GetHostResponse struct {
	ErrorMsg string    `json:"error_msg"`
	Data     *HostInfo `json:"data"`
}

// UpdateHostResponse represents the response for updating a host.
// UpdateHostResponse 表示更新主机的响应。
type UpdateHostResponse struct {
	ErrorMsg string    `json:"error_msg"`
	Data     *HostInfo `json:"data"`
}

// DeleteHostResponse represents the response for deleting a host.
// DeleteHostResponse 表示删除主机的响应。
type DeleteHostResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     any    `json:"data"`
}

// GetInstallCommandResponse represents the response for getting install command.
// GetInstallCommandResponse 表示获取安装命令的响应。
type GetInstallCommandResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Command string `json:"command"`
	} `json:"data"`
}

// ==================== Handlers 处理器 ====================

// CreateHost handles POST /api/v1/hosts - creates a new host.
// CreateHost 处理 POST /api/v1/hosts - 创建新主机。
// @Tags hosts
// @Accept json
// @Produce json
// @Param request body CreateHostRequest true "创建主机请求"
// @Success 200 {object} CreateHostResponse
// @Router /api/v1/hosts [post]
func (h *Handler) CreateHost(c *gin.Context) {
	var req CreateHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CreateHostResponse{ErrorMsg: err.Error()})
		return
	}

	host, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, CreateHostResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"create", "host", audit.UintID(host.ID), host.Name, hostWriteAuditDetails("host.create"))
	logger.InfoF(c.Request.Context(), "[Host] 创建主机成功: %s (type: %s)", host.Name, host.HostType)
	c.JSON(http.StatusOK, CreateHostResponse{Data: host.ToHostInfo(h.service.GetHeartbeatTimeout(), h.service.GetProcessStartedAt())})
}

// ListHosts handles GET /api/v1/hosts - lists hosts with filtering and pagination.
// ListHosts 处理 GET /api/v1/hosts - 获取主机列表（支持过滤和分页）。
// @Tags hosts
// @Param request query ListHostsRequest true "查询参数"
// @Produce json
// @Success 200 {object} ListHostsResponse
// @Router /api/v1/hosts [get]
func (h *Handler) ListHosts(c *gin.Context) {
	req := &ListHostsRequest{Current: 1, Size: 20}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListHostsResponse{ErrorMsg: err.Error()})
		return
	}

	// Build filter from request
	// 从请求构建过滤条件
	filter := &HostFilter{
		Name:        req.Name,
		HostType:    req.HostType,
		IPAddress:   req.IPAddress,
		Status:      req.Status,
		AgentStatus: req.AgentStatus,
		IsOnline:    req.IsOnline,
		Page:        req.Current,
		PageSize:    req.Size,
	}

	hosts, total, err := h.service.ListWithInfo(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListHostsResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ListHostsResponse{
		Data: &struct {
			Total int64       `json:"total"`
			Hosts []*HostInfo `json:"hosts"`
		}{
			Total: total,
			Hosts: hosts,
		},
	})
}

// GetHost handles GET /api/v1/hosts/:id - gets a host by ID.
// GetHost 处理 GET /api/v1/hosts/:id - 根据 ID 获取主机详情。
// @Tags hosts
// @Produce json
// @Param id path int true "主机ID"
// @Success 200 {object} GetHostResponse
// @Router /api/v1/hosts/{id} [get]
func (h *Handler) GetHost(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetHostResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	host, err := h.service.Get(c.Request.Context(), uint(hostID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetHostResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetHostResponse{Data: host.ToHostInfo(h.service.GetHeartbeatTimeout(), h.service.GetProcessStartedAt())})
}

// UpdateHost handles PUT /api/v1/hosts/:id - updates an existing host.
// UpdateHost 处理 PUT /api/v1/hosts/:id - 更新现有主机。
// @Tags hosts
// @Accept json
// @Produce json
// @Param id path int true "主机ID"
// @Param request body UpdateHostRequest true "更新主机请求"
// @Success 200 {object} UpdateHostResponse
// @Router /api/v1/hosts/{id} [put]
func (h *Handler) UpdateHost(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, UpdateHostResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	var req UpdateHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, UpdateHostResponse{ErrorMsg: err.Error()})
		return
	}

	host, err := h.service.Update(c.Request.Context(), uint(hostID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, UpdateHostResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "host", audit.UintID(host.ID), host.Name, hostWriteAuditDetails("host.update"))
	logger.InfoF(c.Request.Context(), "[Host] 更新主机成功: %s", host.Name)
	c.JSON(http.StatusOK, UpdateHostResponse{Data: host.ToHostInfo(h.service.GetHeartbeatTimeout(), h.service.GetProcessStartedAt())})
}

// hostWriteAuditDetails 生成主机写操作的公共审计关联字段。
// hostWriteAuditDetails builds shared audit linkage fields for host writes.
func hostWriteAuditDetails(operationID string) audit.AuditDetails {
	return audit.AuditDetails{
		"trigger": "manual", "operation_id": operationID,
		"risk_level": "R1", "result_status": "succeeded",
	}
}

// DeleteHost handles DELETE /api/v1/hosts/:id - deletes a host.
// DeleteHost 处理 DELETE /api/v1/hosts/:id - 删除主机。
// @Tags hosts
// @Produce json
// @Param id path int true "主机ID"
// @Success 200 {object} DeleteHostResponse
// @Router /api/v1/hosts/{id} [delete]
func (h *Handler) DeleteHost(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, DeleteHostResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	// Get host name for logging before deletion
	// 在删除前获取主机名用于日志记录
	host, err := h.service.Get(c.Request.Context(), uint(hostID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, DeleteHostResponse{ErrorMsg: err.Error()})
		return
	}

	if err := h.service.Delete(c.Request.Context(), uint(hostID)); err != nil {
		statusCode := h.getStatusCodeForError(err)
		// If host has associated clusters, return the cluster info
		// 如果主机关联了集群，返回集群信息
		if errors.Is(err, ErrHostHasCluster) {
			clusters, _ := h.service.GetAssociatedClusters(c.Request.Context(), uint(hostID))
			c.JSON(statusCode, DeleteHostResponse{
				ErrorMsg: err.Error(),
				Data:     clusters,
			})
			return
		}
		c.JSON(statusCode, DeleteHostResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"delete", "host", audit.UintID(uint(hostID)), host.Name, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Host] 删除主机成功: %s", host.Name)
	c.JSON(http.StatusOK, DeleteHostResponse{})
}

// GetInstallCommand handles GET /api/v1/hosts/:id/install-command - gets the Agent install command.
// GetInstallCommand 处理 GET /api/v1/hosts/:id/install-command - 获取 Agent 安装命令。
// @Tags hosts
// @Produce json
// @Param id path int true "主机ID"
// @Success 200 {object} GetInstallCommandResponse
// @Router /api/v1/hosts/{id}/install-command [get]
func (h *Handler) GetInstallCommand(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetInstallCommandResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	command, err := h.service.GetInstallCommand(c.Request.Context(), uint(hostID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetInstallCommandResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetInstallCommandResponse{
		Data: &struct {
			Command string `json:"command"`
		}{
			Command: command,
		},
	})
}

// ==================== Helper Methods 辅助方法 ====================

// getStatusCodeForError returns the appropriate HTTP status code for an error.
// getStatusCodeForError 根据错误返回适当的 HTTP 状态码。
func (h *Handler) getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrHostNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrHostNameDuplicate):
		return http.StatusConflict
	case errors.Is(err, ErrHostIPDuplicate):
		return http.StatusConflict
	case errors.Is(err, ErrHostIPInvalid),
		errors.Is(err, ErrHostNameEmpty),
		errors.Is(err, ErrHostTypeInvalid),
		errors.Is(err, ErrDockerAPIURLInvalid),
		errors.Is(err, ErrK8sAPIURLInvalid),
		errors.Is(err, ErrK8sCredentialsRequired):
		return http.StatusBadRequest
	case errors.Is(err, ErrHostHasCluster):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
