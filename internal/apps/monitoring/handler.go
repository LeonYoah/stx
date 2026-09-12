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
	"io"
	"net/http"
	"net/http/httputil"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/gin-gonic/gin"
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

// ListAlerts handles GET /api/v1/monitoring/alerts
// ListAlerts 处理 GET /api/v1/monitoring/alerts
func (h *Handler) ListAlerts(c *gin.Context) {
	filter := &AlertFilter{}

	if clusterIDStr := c.Query("cluster_id"); clusterIDStr != "" {
		clusterID, err := strconv.ParseUint(clusterIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster_id"})
			return
		}
		filter.ClusterID = uint(clusterID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		parsedStatus := AlertStatus(status)
		if parsedStatus != AlertStatusFiring && parsedStatus != AlertStatusAcknowledged && parsedStatus != AlertStatusSilenced {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid status"})
			return
		}
		filter.Status = parsedStatus
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
	if pageStr := c.Query("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid page"})
			return
		}
		filter.Page = page
	}
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid page_size"})
			return
		}
		filter.PageSize = pageSize
	}

	data, err := h.service.ListAlerts(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list alerts: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// AcknowledgeAlert handles POST /api/v1/monitoring/alerts/:eventId/ack
// AcknowledgeAlert 处理 POST /api/v1/monitoring/alerts/:eventId/ack
func (h *Handler) AcknowledgeAlert(c *gin.Context) {
	eventID, err := strconv.ParseUint(c.Param("eventId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid event id"})
		return
	}

	req := &AcknowledgeAlertRequest{}
	if err := c.ShouldBindJSON(req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}

	operator := "unknown"
	if user := auth.GetUserFromContext(c); user != nil && user.Username != "" {
		operator = user.Username
	}

	data, err := h.service.AcknowledgeAlert(c.Request.Context(), uint(eventID), operator, req.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to acknowledge alert: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
}

// SilenceAlert handles POST /api/v1/monitoring/alerts/:eventId/silence
// SilenceAlert 处理 POST /api/v1/monitoring/alerts/:eventId/silence
func (h *Handler) SilenceAlert(c *gin.Context) {
	eventID, err := strconv.ParseUint(c.Param("eventId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid event id"})
		return
	}

	req := &SilenceAlertRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	operator := "unknown"
	if user := auth.GetUserFromContext(c); user != nil && user.Username != "" {
		operator = user.Username
	}

	data, err := h.service.SilenceAlert(c.Request.Context(), uint(eventID), operator, req.DurationMinutes, req.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to silence alert: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: data})
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

// GetPrometheusDiscovery handles GET /api/v1/monitoring/prometheus/discovery.
// GetPrometheusDiscovery 处理 Prometheus HTTP SD 接口。
func (h *Handler) GetPrometheusDiscovery(c *gin.Context) {
	if !config.Config.Observability.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "observability is disabled"})
		return
	}

	data, err := h.service.BuildPrometheusSDTargets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to build prometheus discovery targets: " + err.Error()})
		return
	}

	// Prometheus HTTP SD requires plain JSON target-group array as response body.
	// Prometheus HTTP SD 要求响应体是纯目标组数组，不能包裹统一 Response 结构。
	c.JSON(http.StatusOK, data)
}

// AlertmanagerWebhook handles POST /api/v1/monitoring/alertmanager/webhook.
// AlertmanagerWebhook 处理 Alertmanager webhook 告警推送。
func (h *Handler) AlertmanagerWebhook(c *gin.Context) {
	if !config.Config.Observability.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "observability is disabled"})
		return
	}

	var payload AlertmanagerWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid webhook payload: " + err.Error()})
		return
	}

	result, err := h.service.HandleAlertmanagerWebhook(c.Request.Context(), &payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to process alertmanager webhook: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: result})
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
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to create notification channel: " + err.Error()})
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
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to update notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
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

// ProxyGrafana handles reverse proxy for Grafana UI.
// ProxyGrafana 处理 Grafana UI 反向代理。
func (h *Handler) ProxyGrafana(c *gin.Context) {
	if isGrafanaLiveWSRequest(c.Request.URL.Path) {
		// 监控中心默认关闭 Grafana Live，避免 iframe 中持续 WS 重连带来的噪音和开销。
		c.Status(http.StatusNoContent)
		return
	}

	target, err := parseProxyTarget(config.Config.Observability.Grafana.URL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, Response{ErrorMsg: "invalid observability.grafana.url: " + err.Error()})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		// Keep original path/query so Grafana sub-path config can work directly.
		// 保留原始路径/查询参数，匹配 Grafana 子路径配置。
		req.URL.Path = c.Request.URL.Path
		req.URL.RawPath = c.Request.URL.RawPath
		req.URL.RawQuery = c.Request.URL.RawQuery
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
		req.Header.Set("X-Forwarded-Proto", resolveRequestScheme(c.Request))
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, Response{ErrorMsg: "grafana proxy error: " + proxyErr.Error()})
		}
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func parseProxyTarget(raw string) (*neturl.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("empty url")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	u, err := neturl.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("missing scheme or host")
	}
	return u, nil
}

func resolveRequestScheme(r *http.Request) string {
	if r == nil {
		return "http"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func isGrafanaLiveWSRequest(path string) bool {
	return strings.Contains(path, "/api/live/ws")
}
