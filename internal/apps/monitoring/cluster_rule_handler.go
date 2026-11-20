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

	"github.com/gin-gonic/gin"
)

// ListClusterRuleViews handles GET /api/v1/monitoring/cluster-rules.
// ListClusterRuleViews 处理集群级规则列表查询接口。
func (h *Handler) ListClusterRuleViews(c *gin.Context) {
	data, err := h.service.ListClusterRuleViews(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list cluster rules: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetClusterRuleView handles GET /api/v1/monitoring/cluster-rules/:key.
// GetClusterRuleView 处理单条集群级规则查询接口。
func (h *Handler) GetClusterRuleView(c *gin.Context) {
	data, err := h.service.GetClusterRuleView(c.Request.Context(), c.Param("key"))
	if err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to get cluster rule: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// PutClusterRule handles PUT /api/v1/monitoring/cluster-rules/:key（整层替换全局配置）。
// PutClusterRule handles PUT /api/v1/monitoring/cluster-rules/:key (replacing the whole global layer).
func (h *Handler) PutClusterRule(c *gin.Context) {
	var req clusterRuleLayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	ctx := c.Request.Context()
	key := c.Param("key")
	if _, err := h.service.SaveClusterRuleConfig(ctx, key, req.toFields()); err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to save cluster rule: " + err.Error()})
		return
	}
	data, err := h.service.GetClusterRuleView(ctx, key)
	if err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to get cluster rule: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// DeleteClusterRule handles DELETE /api/v1/monitoring/cluster-rules/:key（删全局层，恢复目录默认）。
// DeleteClusterRule handles DELETE /api/v1/monitoring/cluster-rules/:key (removing the global
// layer and restoring catalog defaults).
func (h *Handler) DeleteClusterRule(c *gin.Context) {
	ctx := c.Request.Context()
	key := c.Param("key")
	if err := h.service.DeleteClusterRuleConfig(ctx, key); err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to delete cluster rule config: " + err.Error()})
		return
	}
	data, err := h.service.GetClusterRuleView(ctx, key)
	if err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to get cluster rule: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// PutClusterRuleOverride handles PUT /api/v1/monitoring/cluster-rules/:key/clusters/:clusterId.
// PutClusterRuleOverride 处理单集群例外整层替换接口。
func (h *Handler) PutClusterRuleOverride(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("clusterId"), 10, 32)
	if err != nil || clusterID == 0 {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id"})
		return
	}
	var req clusterRuleLayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	ctx := c.Request.Context()
	key := c.Param("key")
	if _, err := h.service.SaveClusterRuleOverride(ctx, key, uint(clusterID), req.toFields()); err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to save cluster rule override: " + err.Error()})
		return
	}
	data, err := h.service.GetClusterRuleView(ctx, key)
	if err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to get cluster rule: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// DeleteClusterRuleOverride handles DELETE /api/v1/monitoring/cluster-rules/:key/clusters/:clusterId.
// DeleteClusterRuleOverride 处理单集群例外删除接口。
func (h *Handler) DeleteClusterRuleOverride(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("clusterId"), 10, 32)
	if err != nil || clusterID == 0 {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id"})
		return
	}
	ctx := c.Request.Context()
	key := c.Param("key")
	if err := h.service.DeleteClusterRuleOverride(ctx, key, uint(clusterID)); err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to delete cluster rule override: " + err.Error()})
		return
	}
	data, err := h.service.GetClusterRuleView(ctx, key)
	if err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to get cluster rule: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetAlertNotificationDefaults handles GET /api/v1/monitoring/alert-notification-defaults.
// GetAlertNotificationDefaults 处理默认通知查询接口；未配置时返回 configured=false。
func (h *Handler) GetAlertNotificationDefaults(c *gin.Context) {
	data, err := h.service.GetAlertNotificationDefaultsView(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get alert notification defaults: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// PutAlertNotificationDefaults handles PUT /api/v1/monitoring/alert-notification-defaults.
// PutAlertNotificationDefaults 处理默认通知整行替换接口。
func (h *Handler) PutAlertNotificationDefaults(c *gin.Context) {
	var req alertNotificationDefaultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid request body: " + err.Error()})
		return
	}
	var channelIDs []uint
	var receiverUserIDs []uint64
	cooldownMinutes := 0
	sendRecovery := true
	if req.ChannelIDs != nil {
		channelIDs = *req.ChannelIDs
	}
	if req.ReceiverUserIDs != nil {
		receiverUserIDs = *req.ReceiverUserIDs
	}
	if req.CooldownMinutes != nil {
		cooldownMinutes = *req.CooldownMinutes
	}
	if req.SendRecovery != nil {
		sendRecovery = *req.SendRecovery
	}
	if cooldownMinutes < 0 {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "cooldown_minutes must not be negative"})
		return
	}
	ctx := c.Request.Context()
	if _, err := h.service.SaveAlertNotificationDefaults(ctx, channelIDs, receiverUserIDs, cooldownMinutes, sendRecovery); err != nil {
		c.JSON(clusterRuleErrorStatusCode(err), Response{ErrorMsg: "Failed to save alert notification defaults: " + err.Error()})
		return
	}
	data, err := h.service.GetAlertNotificationDefaultsView(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get alert notification defaults: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// clusterRuleErrorStatusCode 把集群规则与通知错误哨兵映射为 HTTP 状态码：规则不存在 404，
// 校验类错误 400，其余 500。
// clusterRuleErrorStatusCode maps cluster-rule and notification error sentinels to HTTP codes:
// missing rule 404, validation errors 400, anything else 500.
func clusterRuleErrorStatusCode(err error) int {
	switch {
	case errors.Is(err, ErrClusterRuleNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrClusterRuleInvalidSeverity),
		errors.Is(err, ErrClusterRuleInvalidNotifyMode),
		errors.Is(err, ErrClusterRuleInvalidThreshold),
		errors.Is(err, ErrClusterRuleInvalidFor),
		errors.Is(err, ErrClusterRuleUnsupportedField),
		errors.Is(err, ErrClusterRuleChannelDisabled),
		errors.Is(err, ErrClusterRuleEmailRecipientMissing),
		errors.Is(err, ErrClusterRuleCriticalChannelRequired),
		errors.Is(err, ErrAlertPolicyNotificationChannelNotFound),
		errors.Is(err, ErrAlertPolicyReceiverUserNotFound):
		return http.StatusBadRequest
	default:
		if strings.Contains(err.Error(), "cluster id is required") {
			return http.StatusBadRequest
		}
		return http.StatusInternalServerError
	}
}
