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

	"github.com/gin-gonic/gin"
)

// ListAlertPolicies handles GET /api/v1/monitoring/alert-policies.
// ListAlertPolicies 处理统一告警策略列表接口。
func (h *Handler) ListAlertPolicies(c *gin.Context) {
	filter := &AlertPolicyListFilter{PolicyType: AlertPolicyBuilderKind(strings.TrimSpace(c.Query("policy_type")))}
	if raw := strings.TrimSpace(c.Query("task_id")); raw != "" {
		taskID, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || taskID == 0 {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid task_id"})
			return
		}
		filter.TaskID = uint(taskID)
	}
	data, err := h.service.ListAlertPolicies(c.Request.Context(), filter)
	if err != nil {
		c.JSON(getAlertPolicyStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListAlertPolicyExecutions handles GET /api/v1/monitoring/alert-policies/:id/executions.
// ListAlertPolicyExecutions 处理统一告警策略执行历史列表接口。
func (h *Handler) ListAlertPolicyExecutions(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid policy id"})
		return
	}

	filter := &NotificationDeliveryFilter{}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		filter.Status = NotificationDeliveryStatus(strings.ToLower(status))
		switch filter.Status {
		case NotificationDeliveryStatusPending,
			NotificationDeliveryStatusSending,
			NotificationDeliveryStatusSent,
			NotificationDeliveryStatusFailed,
			NotificationDeliveryStatusRetrying,
			NotificationDeliveryStatusCanceled:
		default:
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid status"})
			return
		}
	}
	if eventType := strings.TrimSpace(c.Query("event_type")); eventType != "" {
		filter.EventType = NotificationDeliveryEventType(strings.ToLower(eventType))
		switch filter.EventType {
		case NotificationDeliveryEventTypeFiring,
			NotificationDeliveryEventTypeResolved,
			NotificationDeliveryEventTypeTest:
		default:
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid event_type"})
			return
		}
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

	data, err := h.service.ListAlertPolicyExecutions(c.Request.Context(), uint(policyID), filter)
	if err != nil {
		c.JSON(getAlertPolicyStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// CreateAlertPolicy handles POST /api/v1/monitoring/alert-policies.
// CreateAlertPolicy 处理统一告警策略创建接口。
func (h *Handler) CreateAlertPolicy(c *gin.Context) {
	var req UpsertAlertPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.CreateAlertPolicy(c.Request.Context(), &req)
	if err != nil {
		c.JSON(getAlertPolicyStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// UpdateAlertPolicy handles PUT /api/v1/monitoring/alert-policies/:id.
// UpdateAlertPolicy 处理统一告警策略更新接口。
func (h *Handler) UpdateAlertPolicy(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid policy id"})
		return
	}

	var req UpsertAlertPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.UpdateAlertPolicy(c.Request.Context(), uint(policyID), &req)
	if err != nil {
		c.JSON(getAlertPolicyStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// DeleteAlertPolicy handles DELETE /api/v1/monitoring/alert-policies/:id.
// DeleteAlertPolicy 处理统一告警策略删除接口。
func (h *Handler) DeleteAlertPolicy(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid policy id"})
		return
	}

	if err := h.service.DeleteAlertPolicy(c.Request.Context(), uint(policyID)); err != nil {
		c.JSON(getAlertPolicyStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: gin.H{"id": uint(policyID)}})
}

func getAlertPolicyStatusCode(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrAlertPolicyNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAlertPolicyLegacyBridgeConflict),
		errors.Is(err, ErrAlertPolicySyncJobConflict):
		return http.StatusConflict
	case errors.Is(err, ErrAlertPolicyInvalidID),
		errors.Is(err, ErrAlertPolicyInvalidClusterScope),
		errors.Is(err, ErrAlertPolicyClusterNotFound),
		errors.Is(err, ErrAlertPolicyNotificationChannelNotFound),
		errors.Is(err, ErrAlertPolicyReceiverUserNotFound),
		errors.Is(err, ErrAlertPolicyReceiverUserInactive),
		errors.Is(err, ErrAlertPolicyReceiverUserEmailMissing),
		errors.Is(err, ErrAlertPolicyUnsupportedTemplate),
		errors.Is(err, ErrAlertPolicyTemplateTypeMismatch),
		errors.Is(err, ErrAlertPolicyLegacyBridgeRequiresClusterScope),
		errors.Is(err, ErrAlertPolicyLegacyRuleUnsupported),
		errors.Is(err, ErrAlertPolicySyncJobTaskRequired),
		errors.Is(err, ErrAlertPolicySyncJobInvalidRule),
		errors.Is(err, ErrAlertPolicySyncJobChannelRequired),
		errors.Is(err, ErrAlertPolicySyncJobChannelDisabled),
		errors.Is(err, ErrAlertPolicySyncJobEmailRecipientMissing),
		errors.Is(err, ErrAlertPolicySyncJobTypeChange):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
