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

	"github.com/gin-gonic/gin"
)

// TestNotificationChannel handles POST /api/v1/monitoring/notification-channels/:id/test
// TestNotificationChannel 处理通知渠道测试发送接口。
func (h *Handler) TestNotificationChannel(c *gin.Context) {
	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid channel id"})
		return
	}

	var req NotificationChannelTestRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}

	data, err := h.service.TestNotificationChannel(c.Request.Context(), uint(channelID), &req)
	if err != nil {
		c.JSON(getNotificationChannelTestStatusCode(err), Response{ErrorMsg: "Failed to test notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// TestNotificationChannelDraft handles POST /api/v1/monitoring/notification-channels/test
// TestNotificationChannelDraft 处理未保存通知渠道草稿测试发送接口。
func (h *Handler) TestNotificationChannelDraft(c *gin.Context) {
	var req NotificationChannelDraftTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.TestNotificationChannelDraft(c.Request.Context(), &req)
	if err != nil {
		c.JSON(getNotificationChannelTestStatusCode(err), Response{ErrorMsg: "Failed to test notification channel: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// TestNotificationChannelConnection handles POST /api/v1/monitoring/notification-channels/test-connection
// TestNotificationChannelConnection 处理通知渠道连接测试接口。
func (h *Handler) TestNotificationChannelConnection(c *gin.Context) {
	var req NotificationChannelConnectionTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.TestSavedNotificationChannelConnection(c.Request.Context(), req.ChannelID, &req.UpsertNotificationChannelRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to test notification channel connection: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListNotificationDeliveries handles GET /api/v1/monitoring/notification-deliveries
// ListNotificationDeliveries 处理通知投递历史列表接口。
func (h *Handler) ListNotificationDeliveries(c *gin.Context) {
	filter := &NotificationDeliveryFilter{}
	if policyIDStr := strings.TrimSpace(c.Query("policy_id")); policyIDStr != "" {
		policyID, err := strconv.ParseUint(policyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid policy_id"})
			return
		}
		filter.PolicyID = uint(policyID)
	}
	if channelIDStr := strings.TrimSpace(c.Query("channel_id")); channelIDStr != "" {
		channelID, err := strconv.ParseUint(channelIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid channel_id"})
			return
		}
		filter.ChannelID = uint(channelID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		filter.Status = NotificationDeliveryStatus(strings.ToLower(status))
		switch filter.Status {
		case NotificationDeliveryStatusPending,
			NotificationDeliveryStatusSending,
			NotificationDeliveryStatusSent,
			NotificationDeliveryStatusFailed,
			NotificationDeliveryStatusRetrying,
			NotificationDeliveryStatusCanceled,
			NotificationDeliveryStatusExhausted:
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
			NotificationDeliveryEventTypeTest,
			NotificationDeliveryEventTypeReminder:
		default:
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid event_type"})
			return
		}
	}
	if clusterID := strings.TrimSpace(c.Query("cluster_id")); clusterID != "" {
		filter.ClusterID = clusterID
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

	data, err := h.service.ListNotificationDeliveries(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list notification deliveries: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

func getNotificationChannelTestStatusCode(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrAlertPolicyReceiverUserNotFound),
		errors.Is(err, ErrAlertPolicyReceiverUserInactive),
		errors.Is(err, ErrAlertPolicyReceiverUserEmailMissing),
		errors.Is(err, ErrInvalidNotificationChannel):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotificationChannelNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
