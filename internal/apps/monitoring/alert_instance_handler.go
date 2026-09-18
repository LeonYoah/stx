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
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

// ListAlertInstances handles GET /api/v1/monitoring/alert-instances.
// ListAlertInstances 处理统一告警实例列表查询接口。
func (h *Handler) ListAlertInstances(c *gin.Context) {
	filter := &AlertInstanceFilter{}

	if sourceType := strings.TrimSpace(c.Query("source_type")); sourceType != "" {
		parsed := AlertSourceType(sourceType)
		if parsed != AlertSourceTypeLocalProcessEvent && parsed != AlertSourceTypeRemoteAlertmanager {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid source_type"})
			return
		}
		filter.SourceType = parsed
	}
	if clusterID := strings.TrimSpace(c.Query("cluster_id")); clusterID != "" {
		filter.ClusterID = clusterID
	}
	if severity := strings.TrimSpace(c.Query("severity")); severity != "" {
		parsed := AlertSeverity(strings.ToLower(severity))
		if parsed != AlertSeverityWarning && parsed != AlertSeverityCritical {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid severity"})
			return
		}
		filter.Severity = parsed
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		parsed := AlertDisplayStatus(strings.ToLower(status))
		if parsed != AlertDisplayStatusFiring &&
			parsed != AlertDisplayStatusResolved &&
			parsed != AlertDisplayStatusClosed {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid status"})
			return
		}
		filter.Status = parsed
	}
	if lifecycleStatus := strings.TrimSpace(c.Query("lifecycle_status")); lifecycleStatus != "" {
		parsed := AlertLifecycleStatus(strings.ToLower(lifecycleStatus))
		if parsed != AlertLifecycleStatusFiring && parsed != AlertLifecycleStatusResolved {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid lifecycle_status"})
			return
		}
		filter.LifecycleStatus = parsed
	}
	if handlingStatus := strings.TrimSpace(c.Query("handling_status")); handlingStatus != "" {
		parsed := AlertHandlingStatus(strings.ToLower(handlingStatus))
		if parsed != AlertHandlingStatusPending &&
			parsed != AlertHandlingStatusAcknowledged &&
			parsed != AlertHandlingStatusSilenced &&
			parsed != AlertHandlingStatusClosed {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid handling_status"})
			return
		}
		filter.HandlingStatus = parsed
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

	data, err := h.service.ListAlertInstances(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list alert instances: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// AcknowledgeAlertInstance handles POST /api/v1/monitoring/alert-instances/:id/ack.
// AcknowledgeAlertInstance 处理统一告警实例确认接口。
func (h *Handler) AcknowledgeAlertInstance(c *gin.Context) {
	alertID := strings.TrimSpace(c.Param("id"))
	if alertID == "" {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid alert id"})
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

	data, err := h.service.AcknowledgeAlertInstance(c.Request.Context(), alertID, operator, req.Note)
	if err != nil {
		c.JSON(getAlertInstanceStatusCode(err), Response{ErrorMsg: "Failed to acknowledge alert instance: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// SilenceAlertInstance handles POST /api/v1/monitoring/alert-instances/:id/silence.
// SilenceAlertInstance 处理统一告警实例静默接口。
func (h *Handler) SilenceAlertInstance(c *gin.Context) {
	alertID := strings.TrimSpace(c.Param("id"))
	if alertID == "" {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid alert id"})
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

	data, err := h.service.SilenceAlertInstance(c.Request.Context(), alertID, operator, req.DurationMinutes, req.Note)
	if err != nil {
		c.JSON(getAlertInstanceStatusCode(err), Response{ErrorMsg: "Failed to silence alert instance: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// CloseAlertInstance handles POST /api/v1/monitoring/alert-instances/:id/close.
// CloseAlertInstance 处理统一告警实例关闭接口。
func (h *Handler) CloseAlertInstance(c *gin.Context) {
	alertID := strings.TrimSpace(c.Param("id"))
	if alertID == "" {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid alert id"})
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

	data, err := h.service.CloseAlertInstance(c.Request.Context(), alertID, operator, req.Note)
	if err != nil {
		c.JSON(getAlertInstanceStatusCode(err), Response{ErrorMsg: "Failed to close alert instance: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

func getAlertInstanceStatusCode(err error) int {
	switch {
	case errors.Is(err, ErrAlertInstanceInvalidID):
		return http.StatusBadRequest
	case errors.Is(err, ErrAlertInstanceNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAlertInstanceAlreadyClosed),
		errors.Is(err, ErrAlertInstanceNotClosable):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
