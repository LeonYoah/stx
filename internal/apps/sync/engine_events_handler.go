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

package sync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	eventChannelUpdateOperationID = "sync.event-channel.update"
	eventChannelUpdateImpact      = "会修改该集群全部节点的 seatunnel.yaml（event-report-http），需滚动重启集群后生效。"
)

// EventChannelResponse 包装事件通道状态。
// EventChannelResponse wraps the event channel status.
type EventChannelResponse struct {
	ErrorMsg string              `json:"error_msg"`
	Data     *EventChannelStatus `json:"data"`
}

// EngineEventIngestResponse 包装事件接收结果。
// EngineEventIngestResponse wraps the event ingest result.
type EngineEventIngestResponse struct {
	ErrorMsg string                   `json:"error_msg"`
	Data     *EngineEventIngestResult `json:"data"`
}

// JobEventListResponse 包装作业事件时间线。
// JobEventListResponse wraps the job event timeline.
type JobEventListResponse struct {
	ErrorMsg string              `json:"error_msg"`
	Data     *JobEventListResult `json:"data"`
}

// IngestEngineEvents 处理 POST /api/v1/sync/engine-events/:cluster_id（SeaTunnel event-report-http 回调，无需登录）。
// 只有 token 错误返回 401；其余情况都返回 200，避免引擎反复重试同一批而堵住后续事件。
// IngestEngineEvents handles POST /api/v1/sync/engine-events/:cluster_id (SeaTunnel event-report-http callback,
// no login). Only a bad token returns 401; everything else returns 200 so the engine never retries one batch forever.
func (h *Handler) IngestEngineEvents(c *gin.Context) {
	clusterID, ok := parseUintParam(c, "cluster_id")
	if !ok || clusterID == 0 {
		c.JSON(http.StatusOK, EngineEventIngestResponse{Data: &EngineEventIngestResult{Ignored: "invalid_cluster"}})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, EngineEventIngestMaxBytes))
	if err != nil {
		logger.WarnF(c.Request.Context(), "[Sync] engine event batch rejected while reading: cluster=%d err=%v", clusterID, err)
		c.JSON(http.StatusOK, EngineEventIngestResponse{Data: &EngineEventIngestResult{Ignored: "body_too_large"}})
		return
	}
	// 请求结束后仍要完成落库，避免引擎超时断开导致已收到的事件丢失。
	// Persist even if the engine disconnects so events already received are not lost.
	result, err := h.service.IngestEngineEvents(context.WithoutCancel(c.Request.Context()), clusterID, c.GetHeader("Authorization"), body)
	if err != nil {
		if errors.Is(err, ErrEventChannelUnauthorized) {
			c.JSON(http.StatusUnauthorized, EngineEventIngestResponse{ErrorMsg: err.Error()})
			return
		}
		logger.ErrorF(c.Request.Context(), "[Sync] engine event batch failed: cluster=%d err=%v", clusterID, err)
		c.JSON(http.StatusInternalServerError, EngineEventIngestResponse{ErrorMsg: "failed to store engine events"})
		return
	}
	c.JSON(http.StatusOK, EngineEventIngestResponse{Data: result})
}

// GetEventChannel 处理 GET /api/v1/sync/event-channels/:cluster_id。
// GetEventChannel handles GET /api/v1/sync/event-channels/:cluster_id.
func (h *Handler) GetEventChannel(c *gin.Context) {
	clusterID, ok := parseUintParam(c, "cluster_id")
	if !ok {
		c.JSON(http.StatusBadRequest, EventChannelResponse{ErrorMsg: "invalid cluster id"})
		return
	}
	status, err := h.service.GetEventChannel(c.Request.Context(), clusterID)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), EventChannelResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, EventChannelResponse{Data: status})
}

// UpdateEventChannel 处理 PUT /api/v1/sync/event-channels/:cluster_id；CLI 与直接 API 调用走 R2 确认与幂等，网页保持原有交互。
// UpdateEventChannel handles PUT /api/v1/sync/event-channels/:cluster_id; CLI and direct API calls go through R2
// confirmation and idempotency while web requests keep their dialog-based flow.
func (h *Handler) UpdateEventChannel(c *gin.Context) {
	clusterID, ok := parseUintParam(c, "cluster_id")
	if !ok {
		c.JSON(http.StatusBadRequest, EventChannelResponse{ErrorMsg: "invalid cluster id"})
		return
	}
	var req UpdateEventChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, EventChannelResponse{ErrorMsg: err.Error()})
		return
	}
	actor := currentExecutionActor(c)
	moduleRef := strconv.FormatUint(uint64(clusterID), 10)
	item, reused, err := h.beginSyncSynchronousWrite(c, actor, eventChannelUpdateOperationID, moduleRef, executionapp.RiskLevelR2, eventChannelUpdateImpact, gin.H{"cluster_id": clusterID, "request": req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if reused {
		status, getErr := h.service.GetEventChannel(c.Request.Context(), clusterID)
		if getErr != nil {
			c.JSON(h.getStatusCodeForError(getErr), EventChannelResponse{ErrorMsg: getErr.Error()})
			return
		}
		c.JSON(http.StatusOK, EventChannelResponse{Data: status})
		return
	}
	status, err := h.service.UpdateEventChannelForActor(c.Request.Context(), actor, clusterID, &req)
	h.finishSyncSynchronousWrite(c, item, moduleRef, err)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), EventChannelResponse{ErrorMsg: err.Error()})
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "sync_event_channel", moduleRef, "", audit.AuditDetails{
			"operation_id":     eventChannelUpdateOperationID,
			"enabled":          status.Enabled,
			"rotate_token":     req.RotateToken,
			"restart_required": status.RestartRequired,
			"config_errors":    status.ConfigApplyError,
		})
	c.JSON(http.StatusOK, EventChannelResponse{Data: status})
}

// ListJobEvents 处理 GET /api/v1/sync/jobs/:id/events。
// ListJobEvents handles GET /api/v1/sync/jobs/:id/events.
func (h *Handler) ListJobEvents(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, JobEventListResponse{ErrorMsg: "invalid job id"})
		return
	}
	var types []string
	if raw := strings.TrimSpace(c.Query("types")); raw != "" {
		types = strings.Split(raw, ",")
	}
	result, err := h.service.ListJobEventsForActor(c.Request.Context(), currentExecutionActor(c), id, types,
		parsePositiveInt(c.Query("page"), 1), parsePositiveInt(c.Query("page_size"), 50))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), JobEventListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, JobEventListResponse{Data: result})
}

// beginSyncSynchronousWrite 为 CLI 与直接 API 写请求执行确认与幂等检查；网页请求跳过（与 config 写接口一致）。
// beginSyncSynchronousWrite applies confirmation and idempotency checks to CLI and direct API writes; web requests
// skip it, matching the config write endpoints.
func (h *Handler) beginSyncSynchronousWrite(c *gin.Context, actor executionapp.Actor, operationID, moduleRef string, risk executionapp.RiskLevel, impact string, payload any) (*executionapp.Execution, bool, error) {
	metadata := executionapp.MetadataFromGin(c)
	if h == nil || h.service == nil || h.service.executionService == nil || metadata.ClientType == "web" {
		return nil, false, nil
	}
	requestHash, err := executionapp.HashRequest(payload)
	if err != nil {
		return nil, false, err
	}
	return h.service.executionService.BeginSynchronous(c.Request.Context(), actor, executionapp.SynchronousInput{
		OperationID:    operationID,
		Module:         "sync",
		ModuleRef:      moduleRef,
		RequestID:      metadata.RequestID,
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		RiskLevel:      risk,
		Impact:         impact,
		Confirmed:      metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID,
		ClientType:     metadata.ClientType,
	})
}

// finishSyncSynchronousWrite 在客户端断开后仍完成公共执行记录。
// finishSyncSynchronousWrite completes the shared execution record even after the client disconnects.
func (h *Handler) finishSyncSynchronousWrite(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h == nil || h.service == nil || h.service.executionService == nil || item == nil {
		return
	}
	finishContext := context.WithoutCancel(c.Request.Context())
	if err := h.service.executionService.FinishSynchronous(finishContext, item, resultRef, runErr); err != nil {
		logger.WarnF(finishContext, "[Sync] 更新公共执行记录失败 / failed to finish execution: execution_id=%s err=%v", item.ExecutionID, err)
	}
}
