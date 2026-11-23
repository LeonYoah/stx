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

package monitor

import (
	"github.com/LeonYoah/stx/internal/pkg/locale"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for monitor configuration and events.
// Handler 处理监控配置和事件的 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler creates a new monitor handler.
// NewHandler 创建新的监控处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetMonitorConfig handles GET /api/v1/clusters/:id/monitor-config
// GetMonitorConfig 处理 GET /api/v1/clusters/:id/monitor-config
// @Summary Get monitor configuration for a cluster
// @Description Get the monitoring configuration for a specific cluster
// @Tags Monitor
// @Accept json
// @Produce json
// @Param id path int true "Cluster ID"
// @Success 200 {object} MonitorConfig
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/clusters/{id}/monitor-config [get]
func (h *Handler) GetMonitorConfig(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的集群 ID", "invalid cluster id")})
		return
	}

	config, err := h.service.GetOrCreateConfig(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateMonitorConfig handles PUT /api/v1/clusters/:id/monitor-config
// UpdateMonitorConfig 处理 PUT /api/v1/clusters/:id/monitor-config
// @Summary Update monitor configuration for a cluster
// @Description Update the monitoring configuration for a specific cluster
// @Tags Monitor
// @Accept json
// @Produce json
// @Param id path int true "Cluster ID"
// @Param config body UpdateMonitorConfigRequest true "Monitor config update"
// @Success 200 {object} MonitorConfig
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/clusters/{id}/monitor-config [put]
func (h *Handler) UpdateMonitorConfig(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的集群 ID", "invalid cluster id")})
		return
	}

	var req UpdateMonitorConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := h.service.UpdateConfig(c.Request.Context(), uint(clusterID), &req)
	if err != nil {
		if err == ErrInvalidMonitorInterval || err == ErrInvalidRestartDelay ||
			err == ErrInvalidMaxRestarts || err == ErrInvalidTimeWindow ||
			err == ErrInvalidCooldownPeriod {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// ListProcessEvents handles GET /api/v1/clusters/:id/events
// ListProcessEvents 处理 GET /api/v1/clusters/:id/events
// @Summary List process events for a cluster
// @Description Get a list of process events for a specific cluster
// @Tags Monitor
// @Accept json
// @Produce json
// @Param id path int true "Cluster ID"
// @Param event_type query string false "Event type filter"
// @Param node_id query int false "Node ID filter"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/clusters/{id}/events [get]
func (h *Handler) ListProcessEvents(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的集群 ID", "invalid cluster id")})
		return
	}

	filter := &ProcessEventFilter{
		ClusterID: uint(clusterID),
	}

	// Parse query parameters / 解析查询参数
	if eventType := c.Query("event_type"); eventType != "" {
		filter.EventType = ProcessEventType(eventType)
	}
	if nodeIDStr := c.Query("node_id"); nodeIDStr != "" {
		if nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32); err == nil {
			filter.NodeID = uint(nodeID)
		}
	}
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			filter.StartTime = &startTime
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			filter.EndTime = &endTime
		}
	}
	if pageStr := c.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil {
			filter.Page = page
		}
	}
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil {
			filter.PageSize = pageSize
		}
	}

	events, total, err := h.service.ListEvents(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events":    events,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

// GetEventStats handles GET /api/v1/clusters/:id/events/stats
// GetEventStats 处理 GET /api/v1/clusters/:id/events/stats
// @Summary Get event statistics for a cluster
// @Description Get event statistics for a specific cluster
// @Tags Monitor
// @Accept json
// @Produce json
// @Param id path int true "Cluster ID"
// @Param since query string false "Since time (RFC3339)"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/clusters/{id}/events/stats [get]
func (h *Handler) GetEventStats(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的集群 ID", "invalid cluster id")})
		return
	}

	var since *time.Time
	if sinceStr := c.Query("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = &t
		}
	}

	stats, err := h.service.GetEventStats(c.Request.Context(), uint(clusterID), since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}
