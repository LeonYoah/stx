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
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

const (
	taskAlertUpdateOperationID = "sync.task.alert.set"
	taskAlertUpdateImpact      = "修改该任务的作业告警配置，保存后立即生效（不需要重新发布）。"
	taskAlertDeleteOperationID = "sync.task.alert.delete"
	taskAlertDeleteImpact      = "删除该任务的作业告警配置，之后不再产生新告警，正在告警中的阶段会被关闭。"
)

// TaskAlertRequest 是 PUT /api/v1/sync/tasks/:id/alert 的请求体；enabled、send_recovery 默认 true，cooldown_minutes 默认 60。
// TaskAlertRequest is the body of PUT /api/v1/sync/tasks/:id/alert; enabled and send_recovery default to true and
// cooldown_minutes to 60.
type TaskAlertRequest struct {
	Enabled         *bool    `json:"enabled"`
	Rules           []string `json:"rules"`
	ChannelIDs      []uint   `json:"channel_ids"`
	ReceiverUserIDs []uint   `json:"receiver_user_ids"`
	NotifyTaskOwner bool     `json:"notify_task_owner"`
	CooldownMinutes *int     `json:"cooldown_minutes"`
	SendRecovery    *bool    `json:"send_recovery"`
	// CDCDelayThresholdSeconds 是 B4 阈值（秒），范围 10~86400，超出返回 400。不填时保留已保存的值（新建时为默认 60）；
	// 明确填 0 表示恢复默认 60。
	// CDCDelayThresholdSeconds is the B4 threshold in seconds, range 10..86400, else 400. Omitted keeps the stored
	// value (the default 60 on create); an explicit 0 resets to the default 60.
	CDCDelayThresholdSeconds *int `json:"cdc_delay_threshold_seconds"`
	// CDCDelayDurationMinutes 是 B4 持续时间（分钟），范围 1~60，超出返回 400。不填时保留已保存的值（新建时为默认 5）；
	// 明确填 0 表示恢复默认 5。
	// CDCDelayDurationMinutes is the B4 duration in minutes, range 1..60, else 400. Omitted keeps the stored value
	// (the default 5 on create); an explicit 0 resets to the default 5.
	CDCDelayDurationMinutes *int `json:"cdc_delay_duration_minutes"`
}

// toTaskConfig 补齐默认值后转成告警配置；stored 是已保存的配置（没有时为 nil），不填的 B4 参数沿用它的值，
// 老客户端（不认识 cdc_delay_*）保存一次不会把自定义的阈值、持续时间冲回默认值。
// toTaskConfig fills defaults and converts the request into an alert config; stored is the saved config (nil when
// none) and omitted B4 parameters keep its values, so an old client that does not know cdc_delay_* never resets a
// custom threshold or duration to the defaults.
func (r *TaskAlertRequest) toTaskConfig(stored *syncalert.TaskConfig) *syncalert.TaskConfig {
	cfg := &syncalert.TaskConfig{
		Enabled:         true,
		Rules:           r.Rules,
		ChannelIDs:      r.ChannelIDs,
		ReceiverUserIDs: r.ReceiverUserIDs,
		NotifyTaskOwner: r.NotifyTaskOwner,
		CooldownMinutes: syncalert.DefaultCooldownMinutes,
		SendRecovery:    true,
	}
	// 0 由 Normalize 补成默认值。 / Normalize turns 0 into the default.
	if stored != nil && stored.Configured {
		cfg.CDCDelayThresholdSeconds = stored.CDCDelayThresholdSeconds
		cfg.CDCDelayDurationMinutes = stored.CDCDelayDurationMinutes
	}
	if r.CDCDelayThresholdSeconds != nil {
		cfg.CDCDelayThresholdSeconds = *r.CDCDelayThresholdSeconds
	}
	if r.CDCDelayDurationMinutes != nil {
		cfg.CDCDelayDurationMinutes = *r.CDCDelayDurationMinutes
	}
	if r.Enabled != nil {
		cfg.Enabled = *r.Enabled
	}
	if r.CooldownMinutes != nil {
		cfg.CooldownMinutes = *r.CooldownMinutes
	}
	if r.SendRecovery != nil {
		cfg.SendRecovery = *r.SendRecovery
	}
	return cfg
}

// TaskAlertResponse 是任务告警接口的响应。
// TaskAlertResponse is the response of the task alert API.
type TaskAlertResponse struct {
	ErrorMsg string                `json:"error_msg"`
	Data     *syncalert.TaskConfig `json:"data"`
}

// loadTaskForAlert 读取文件任务并检查权限：edit 为 true 时要求可编辑，否则要求可查看。
// loadTaskForAlert loads a file task and checks permission: edit requires edit rights, otherwise view rights.
func (s *Service) loadTaskForAlert(ctx context.Context, actor executionapp.Actor, taskID uint, edit bool) (*Task, error) {
	if s.alertStore == nil {
		return nil, ErrTaskAlertUnavailable
	}
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	allowed := task.CanUserView(uint(actor.UserID), actor.IsAdmin)
	if edit {
		allowed = task.CanUserEdit(uint(actor.UserID), actor.IsAdmin)
	}
	if !allowed {
		return nil, ErrTaskPermissionDenied
	}
	return task, nil
}

// GetTaskAlertForActor 返回任务告警配置；没有配置时 configured=false。能看到任务的人都能查看。
// GetTaskAlertForActor returns the task alert config (configured=false when none); anyone who can view the task may
// read it.
func (s *Service) GetTaskAlertForActor(ctx context.Context, actor executionapp.Actor, taskID uint) (*syncalert.TaskConfig, error) {
	if _, err := s.loadTaskForAlert(ctx, actor, taskID, false); err != nil {
		return nil, err
	}
	return s.alertStore.GetTaskAlertConfig(ctx, taskID)
}

// UpdateTaskAlertForActor 新建或修改任务告警配置，立即生效，不跟随任务版本；需要任务编辑权限。
// UpdateTaskAlertForActor creates or updates the task alert config; it takes effect immediately, is not versioned
// with the task and requires edit rights.
func (s *Service) UpdateTaskAlertForActor(ctx context.Context, actor executionapp.Actor, taskID uint, req *TaskAlertRequest) (*syncalert.TaskConfig, error) {
	task, err := s.loadTaskForAlert(ctx, actor, taskID, true)
	if err != nil {
		return nil, err
	}
	if req == nil {
		req = &TaskAlertRequest{}
	}
	var stored *syncalert.TaskConfig
	if req.CDCDelayThresholdSeconds == nil || req.CDCDelayDurationMinutes == nil {
		if stored, err = s.alertStore.GetTaskAlertConfig(ctx, task.ID); err != nil {
			return nil, err
		}
	}
	ref := syncalert.TaskRef{ID: task.ID, Name: task.Name, ClusterID: task.ClusterID, CreatorID: task.CreatedBy}
	defer s.invalidateCDCDelayConfig(task.ID)
	return s.alertStore.SaveTaskAlertConfig(ctx, ref, req.toTaskConfig(stored))
}

// DeleteTaskAlertForActor 删除任务告警配置并结束告警中的阶段；需要任务编辑权限。
// DeleteTaskAlertForActor deletes the task alert config and ends firing episodes; it requires edit rights.
func (s *Service) DeleteTaskAlertForActor(ctx context.Context, actor executionapp.Actor, taskID uint) error {
	if _, err := s.loadTaskForAlert(ctx, actor, taskID, true); err != nil {
		return err
	}
	defer s.invalidateCDCDelayConfig(taskID)
	return s.alertStore.DeleteTaskAlertConfig(ctx, taskID)
}

// GetTaskAlert 处理 GET /api/v1/sync/tasks/:id/alert。
// GetTaskAlert handles GET /api/v1/sync/tasks/:id/alert.
// @Summary 获取任务的作业告警配置
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Success 200 {object} TaskAlertResponse
// @Failure 400 {object} TaskAlertResponse
// @Failure 403 {object} TaskAlertResponse
// @Failure 404 {object} TaskAlertResponse
// @Failure 503 {object} TaskAlertResponse
// @Router /api/v1/sync/tasks/{id}/alert [get]
func (h *Handler) GetTaskAlert(c *gin.Context) {
	taskID, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskAlertResponse{ErrorMsg: "invalid task id"})
		return
	}
	cfg, err := h.service.GetTaskAlertForActor(c.Request.Context(), currentExecutionActor(c), taskID)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskAlertResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, TaskAlertResponse{Data: cfg})
}

// UpdateTaskAlert 处理 PUT /api/v1/sync/tasks/:id/alert；CLI 与直接 API 调用走 R1 确认与幂等，网页保持原有交互。
// UpdateTaskAlert handles PUT /api/v1/sync/tasks/:id/alert; CLI and direct API calls go through R1 confirmation and
// idempotency while web requests keep their own flow.
// @Summary 保存任务的作业告警配置（立即生效）
// @Tags Sync
// @Accept json
// @Produce json
// @Param id path int true "任务 ID"
// @Param request body TaskAlertRequest true "作业告警配置"
// @Success 200 {object} TaskAlertResponse
// @Failure 400 {object} TaskAlertResponse
// @Failure 403 {object} TaskAlertResponse
// @Failure 404 {object} TaskAlertResponse
// @Failure 409 {object} TaskAlertResponse
// @Failure 428 {object} TaskAlertResponse
// @Failure 503 {object} TaskAlertResponse
// @Router /api/v1/sync/tasks/{id}/alert [put]
func (h *Handler) UpdateTaskAlert(c *gin.Context) {
	taskID, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskAlertResponse{ErrorMsg: "invalid task id"})
		return
	}
	var req TaskAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, TaskAlertResponse{ErrorMsg: err.Error()})
		return
	}
	actor := currentExecutionActor(c)
	moduleRef := strconv.FormatUint(uint64(taskID), 10)
	item, reused, err := h.beginSyncSynchronousWrite(c, actor, taskAlertUpdateOperationID, moduleRef, executionapp.RiskLevelR1, taskAlertUpdateImpact, gin.H{"task_id": taskID, "request": req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if reused {
		cfg, getErr := h.service.GetTaskAlertForActor(c.Request.Context(), actor, taskID)
		if getErr != nil {
			c.JSON(h.getStatusCodeForError(getErr), TaskAlertResponse{ErrorMsg: getErr.Error()})
			return
		}
		c.JSON(http.StatusOK, TaskAlertResponse{Data: cfg})
		return
	}
	cfg, err := h.service.UpdateTaskAlertForActor(c.Request.Context(), actor, taskID, &req)
	h.finishSyncSynchronousWrite(c, item, moduleRef, err)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), TaskAlertResponse{ErrorMsg: err.Error()})
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "sync_task_alert", moduleRef, "", audit.AuditDetails{
			"operation_id":                taskAlertUpdateOperationID,
			"policy_id":                   cfg.PolicyID,
			"enabled":                     cfg.Enabled,
			"rules":                       cfg.Rules,
			"channel_ids":                 cfg.ChannelIDs,
			"receiver_user_ids":           cfg.ReceiverUserIDs,
			"notify_task_owner":           cfg.NotifyTaskOwner,
			"cooldown_minutes":            cfg.CooldownMinutes,
			"send_recovery":               cfg.SendRecovery,
			"cdc_delay_threshold_seconds": cfg.CDCDelayThresholdSeconds,
			"cdc_delay_duration_minutes":  cfg.CDCDelayDurationMinutes,
		})
	c.JSON(http.StatusOK, TaskAlertResponse{Data: cfg})
}

// DeleteTaskAlert 处理 DELETE /api/v1/sync/tasks/:id/alert；只在网页和 API 提供，CLI 用 set --disable 停用。
// DeleteTaskAlert handles DELETE /api/v1/sync/tasks/:id/alert; only offered in the web UI and API, the CLI disables
// with set --disable.
// @Summary 删除任务的作业告警配置
// @Tags Sync
// @Produce json
// @Param id path int true "任务 ID"
// @Success 200 {object} TaskAlertResponse
// @Failure 400 {object} TaskAlertResponse
// @Failure 403 {object} TaskAlertResponse
// @Failure 404 {object} TaskAlertResponse
// @Failure 428 {object} TaskAlertResponse
// @Failure 503 {object} TaskAlertResponse
// @Router /api/v1/sync/tasks/{id}/alert [delete]
func (h *Handler) DeleteTaskAlert(c *gin.Context) {
	taskID, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, TaskAlertResponse{ErrorMsg: "invalid task id"})
		return
	}
	actor := currentExecutionActor(c)
	moduleRef := strconv.FormatUint(uint64(taskID), 10)
	item, reused, err := h.beginSyncSynchronousWrite(c, actor, taskAlertDeleteOperationID, moduleRef, executionapp.RiskLevelR1, taskAlertDeleteImpact, gin.H{"task_id": taskID})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if !reused {
		err = h.service.DeleteTaskAlertForActor(c.Request.Context(), actor, taskID)
		h.finishSyncSynchronousWrite(c, item, moduleRef, err)
		if err != nil {
			c.JSON(h.getStatusCodeForError(err), TaskAlertResponse{ErrorMsg: err.Error()})
			return
		}
		_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
			"delete", "sync_task_alert", moduleRef, "", audit.AuditDetails{"operation_id": taskAlertDeleteOperationID})
	}
	c.JSON(http.StatusOK, TaskAlertResponse{Data: &syncalert.TaskConfig{Configured: false, Rules: []string{}, ChannelIDs: []uint{}, ReceiverUserIDs: []uint{}}})
}
