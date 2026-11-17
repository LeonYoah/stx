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

package monitoring

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles monitoring overview HTTP requests.
// Handler 处理监控总览 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler creates a new monitoring handler.
// NewHandler 创建新的监控处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetOverview handles GET /api/v1/monitoring/overview
// GetOverview 处理 GET /api/v1/monitoring/overview
func (h *Handler) GetOverview(c *gin.Context) {
	data, err := h.service.GetOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get monitoring overview: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// GetClusterOverview handles GET /api/v1/monitoring/clusters/:id/overview
// GetClusterOverview 处理 GET /api/v1/monitoring/clusters/:id/overview
func (h *Handler) GetClusterOverview(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id"})
		return
	}

	data, err := h.service.GetClusterOverview(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get cluster monitoring overview: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// ListAlerts 处理 GET /api/v1/monitoring/alerts：与告警中心同一套统一告警实例（本地进程、远程指标、同步作业）。
// ListAlerts handles GET /api/v1/monitoring/alerts: the same unified instances as the alert center
// (local process events, remote metrics, and sync jobs).
func (h *Handler) ListAlerts(c *gin.Context) {
	h.ListAlertInstances(c)
}

// AcknowledgeAlert 处理 POST /api/v1/monitoring/alerts/:eventId/ack：与告警中心同一套实例 ID（含 local:event:N 与纯数字事件 ID）。
// AcknowledgeAlert handles POST /api/v1/monitoring/alerts/:eventId/ack using the same instance IDs as the alert center
// (local:event:N or a numeric event ID).
func (h *Handler) AcknowledgeAlert(c *gin.Context) {
	h.withAlertInstanceID(c, c.Param("eventId"), h.AcknowledgeAlertInstance)
}

// SilenceAlert 处理 POST /api/v1/monitoring/alerts/:eventId/silence：ID 语义与 AcknowledgeAlert 相同。
// SilenceAlert handles POST /api/v1/monitoring/alerts/:eventId/silence with the same ID rules as AcknowledgeAlert.
func (h *Handler) SilenceAlert(c *gin.Context) {
	h.withAlertInstanceID(c, c.Param("eventId"), h.SilenceAlertInstance)
}

// CloseAlert 处理 POST /api/v1/monitoring/alerts/:eventId/close：关闭与告警中心同一条实例。
// CloseAlert handles POST /api/v1/monitoring/alerts/:eventId/close for the same instance as the alert center.
func (h *Handler) CloseAlert(c *gin.Context) {
	h.withAlertInstanceID(c, c.Param("eventId"), h.CloseAlertInstance)
}

// withAlertInstanceID 把 /alerts/:eventId 上的 ID 填进实例接口使用的 :id 参数。
// withAlertInstanceID copies the /alerts/:eventId value into the :id param used by instance handlers.
func (h *Handler) withAlertInstanceID(c *gin.Context, alertID string, next gin.HandlerFunc) {
	if strings.TrimSpace(c.Param("id")) == "" {
		c.Params = append(c.Params, gin.Param{Key: "id", Value: strings.TrimSpace(alertID)})
	}
	next(c)
}

// ListClusterRules handles GET /api/v1/monitoring/clusters/:id/rules
// ListClusterRules 处理 GET /api/v1/monitoring/clusters/:id/rules
func (h *Handler) ListClusterRules(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id"})
		return
	}

	data, err := h.service.ListClusterRules(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list cluster rules: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// UpdateClusterRule handles PUT /api/v1/monitoring/clusters/:id/rules/:ruleId
// UpdateClusterRule 处理 PUT /api/v1/monitoring/clusters/:id/rules/:ruleId
func (h *Handler) UpdateClusterRule(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id"})
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid rule id"})
		return
	}

	var req UpdateAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.UpdateClusterRule(c.Request.Context(), uint(clusterID), uint(ruleID), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to update cluster rule: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// GetIntegrationStatus handles GET /api/v1/monitoring/integration/status
// GetIntegrationStatus 处理 GET /api/v1/monitoring/integration/status
func (h *Handler) GetIntegrationStatus(c *gin.Context) {
	data, err := h.service.GetIntegrationStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get integration status: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetAlertPolicyCenterBootstrap handles GET /api/v1/monitoring/alert-policies/bootstrap.
// GetAlertPolicyCenterBootstrap 处理统一策略中心初始化接口。
func (h *Handler) GetAlertPolicyCenterBootstrap(c *gin.Context) {
	data, err := h.service.GetAlertPolicyCenterBootstrap(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to load alert policy center bootstrap: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListNotifiableUsers handles GET /api/v1/monitoring/notifiable-users.
// ListNotifiableUsers 处理可通知用户列表接口。
func (h *Handler) ListNotifiableUsers(c *gin.Context) {
	data, err := h.service.ListNotifiableUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list notifiable users: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListRemoteAlerts handles GET /api/v1/monitoring/remote-alerts.
// ListRemoteAlerts 处理远程告警记录查询接口。
func (h *Handler) ListRemoteAlerts(c *gin.Context) {
	if !config.Config.Observability.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "observability is disabled"})
		return
	}

	filter := &RemoteAlertFilter{}
	if clusterID := strings.TrimSpace(c.Query("cluster_id")); clusterID != "" {
		filter.ClusterID = clusterID
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		filter.Status = strings.ToLower(status)
	}
	if startTimeStr := strings.TrimSpace(c.Query("start_time")); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid start_time, expected RFC3339"})
			return
		}
		filter.StartTime = &startTime
	}
	if endTimeStr := strings.TrimSpace(c.Query("end_time")); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid end_time, expected RFC3339"})
			return
		}
		filter.EndTime = &endTime
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.StartTime.After(*filter.EndTime) {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "start_time must be earlier than end_time"})
		return
	}
	if pageStr := strings.TrimSpace(c.Query("page")); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid page"})
			return
		}
		filter.Page = page
	}
	if pageSizeStr := strings.TrimSpace(c.Query("page_size")); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid page_size"})
			return
		}
		filter.PageSize = pageSize
	}

	data, err := h.service.ListRemoteAlerts(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list remote alerts: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetClustersHealth handles GET /api/v1/clusters/health.
// GetClustersHealth 处理集群健康摘要查询接口。
func (h *Handler) GetClustersHealth(c *gin.Context) {
	if !config.Config.Observability.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "observability is disabled"})
		return
	}

	data, err := h.service.GetClustersHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get clusters health: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetPlatformHealth handles GET /api/v1/monitoring/platform-health.
// GetPlatformHealth 处理平台健康摘要查询接口。
func (h *Handler) GetPlatformHealth(c *gin.Context) {
	if !config.Config.Observability.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "observability is disabled"})
		return
	}

	data, err := h.service.GetPlatformHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get platform health: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListNotificationChannels handles GET /api/v1/monitoring/notification-channels
// ListNotificationChannels 处理 GET /api/v1/monitoring/notification-channels
func (h *Handler) ListNotificationChannels(c *gin.Context) {
	data, err := h.service.ListNotificationChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list notification channels: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// CreateNotificationChannel handles POST /api/v1/monitoring/notification-channels
// CreateNotificationChannel 处理 POST /api/v1/monitoring/notification-channels
func (h *Handler) CreateNotificationChannel(c *gin.Context) {
	var req UpsertNotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.CreateNotificationChannel(c.Request.Context(), &req)
	if err != nil {
		c.JSON(notificationChannelUpsertStatusCode(err), Response{ErrorMsg: "Failed to create notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// UpdateNotificationChannel handles PUT /api/v1/monitoring/notification-channels/:id
// UpdateNotificationChannel 处理 PUT /api/v1/monitoring/notification-channels/:id
func (h *Handler) UpdateNotificationChannel(c *gin.Context) {
	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid channel id"})
		return
	}

	var req UpsertNotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.UpdateNotificationChannel(c.Request.Context(), uint(channelID), &req)
	if err != nil {
		c.JSON(notificationChannelUpsertStatusCode(err), Response{ErrorMsg: "Failed to update notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// notificationChannelUpsertStatusCode 把渠道创建 / 更新错误映射为 HTTP 状态码：校验错误 400，渠道不存在 404，其余 500。
// notificationChannelUpsertStatusCode maps channel create / update errors to HTTP codes: validation 400, missing
// channel 404, anything else 500.
func notificationChannelUpsertStatusCode(err error) int {
	switch {
	case errors.Is(err, ErrInvalidNotificationChannel):
		return http.StatusBadRequest
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, ErrNotificationChannelNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// DeleteNotificationChannel handles DELETE /api/v1/monitoring/notification-channels/:id
// DeleteNotificationChannel 处理 DELETE /api/v1/monitoring/notification-channels/:id
func (h *Handler) DeleteNotificationChannel(c *gin.Context) {
	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid channel id"})
		return
	}

	if err := h.service.DeleteNotificationChannel(c.Request.Context(), uint(channelID)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to delete notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: gin.H{"id": channelID}})
}
