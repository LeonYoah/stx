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
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
)

// 提交/停止结果未知时写在 submit_spec / result_preview 里的键，前端和 CLI 依赖这些键名。
// Keys written into submit_spec / result_preview for unknown submit/stop outcomes; the UI and CLI rely on them.
const (
	submitOutcomeKey          = "submit_outcome"
	submitOutcomeUnknown      = "unknown"
	submitOutcomeConfirmed    = "confirmed"
	submitOutcomeExpired      = "expired"
	submitOutcomeSinceKey     = "submit_outcome_since"
	submitOutcomeAtKey        = "submit_outcome_at"
	submitDeadlineKey         = "submit_deadline"
	submitTimeoutSecondsKey   = "submit_timeout_seconds"
	submitErrorKey            = "submit_error"
	submitStopTimeoutSecsKey  = "stop_timeout_seconds"
	cancelOutcomeKey          = "cancel_outcome"
	cancelOutcomeUnknown      = "unknown"
	cancelOutcomeConfirmed    = "confirmed"
	cancelErrorKey            = "cancel_error"
	cancelRequestedAtKey      = "cancel_requested_at"
	cancelTimeoutSecondsKey   = "cancel_timeout_seconds"
	cancelDeadlineKey         = "cancel_deadline"
	cancelOverdueAtKey        = "cancel_overdue_at"
	submitTimeoutSettingKey   = "submit_timeout_minutes"
	stopTimeoutSettingKey     = "stop_timeout_minutes"
	taskTimeoutMinMinutes     = 1
	taskTimeoutMaxMinutes     = 120
	defaultSubmitTimeout      = 5 * time.Minute
	defaultStopTimeout        = 5 * time.Minute
	submitOutcomeGuardScanCap = 20
)

// TaskTimeoutDefaults 是任务详情里附带的全局超时默认值，供任务设置的占位提示使用。
// TaskTimeoutDefaults carries the global timeout defaults on task responses for the settings placeholders.
type TaskTimeoutDefaults struct {
	SubmitTimeoutSeconds int `json:"submit_timeout_seconds"`
	StopTimeoutSeconds   int `json:"stop_timeout_seconds"`
}

// currentTaskTimeoutDefaults 返回当前生效的全局超时默认值。
// currentTaskTimeoutDefaults returns the global timeout defaults currently in effect.
func currentTaskTimeoutDefaults() *TaskTimeoutDefaults {
	return &TaskTimeoutDefaults{
		SubmitTimeoutSeconds: int(globalSubmitTimeout() / time.Second),
		StopTimeoutSeconds:   int(globalStopTimeout() / time.Second),
	}
}

func globalSubmitTimeout() time.Duration {
	if config.Config == nil || config.Config.Sync.SubmitTimeoutSeconds <= 0 {
		return defaultSubmitTimeout
	}
	return time.Duration(config.Config.Sync.SubmitTimeoutSeconds) * time.Second
}

func globalStopTimeout() time.Duration {
	if config.Config == nil || config.Config.Sync.StopTimeoutSeconds <= 0 {
		return defaultStopTimeout
	}
	return time.Duration(config.Config.Sync.StopTimeoutSeconds) * time.Second
}

// parseTaskTimeoutMinutes 解析任务设置里的超时分钟数；未设置返回 ok=false，格式或范围不对返回错误。
// parseTaskTimeoutMinutes parses a timeout override in minutes; unset returns ok=false, bad values return an error.
func parseTaskTimeoutMinutes(definition JSONMap, key string) (int, bool, error) {
	if definition == nil {
		return 0, false, nil
	}
	raw, exists := definition[key]
	if !exists || raw == nil {
		return 0, false, nil
	}
	var minutes float64
	switch value := raw.(type) {
	case int:
		minutes = float64(value)
	case int64:
		minutes = float64(value)
	case float64:
		minutes = value
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, false, ErrInvalidTaskTimeout
		}
		minutes = parsed
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false, ErrInvalidTaskTimeout
		}
		minutes = parsed
	default:
		return 0, false, ErrInvalidTaskTimeout
	}
	if minutes != math.Trunc(minutes) || minutes < taskTimeoutMinMinutes || minutes > taskTimeoutMaxMinutes {
		return 0, false, ErrInvalidTaskTimeout
	}
	return int(minutes), true, nil
}

// validateTaskTimeouts 校验任务设置里的提交/停止超时覆盖值。
// validateTaskTimeouts validates the task's submit/stop timeout overrides.
func validateTaskTimeouts(definition JSONMap) error {
	for _, key := range []string{submitTimeoutSettingKey, stopTimeoutSettingKey} {
		if _, _, err := parseTaskTimeoutMinutes(definition, key); err != nil {
			return fmt.Errorf("%w: %s", err, key)
		}
	}
	return nil
}

// effectiveSubmitTimeout 返回本次运行使用的提交超时：任务快照覆盖 > 全局配置 > 默认 5 分钟。
// effectiveSubmitTimeout returns the submit timeout for this run: task snapshot override > global config > 5 minutes.
func effectiveSubmitTimeout(task *Task) time.Duration {
	if task != nil {
		if minutes, ok, err := parseTaskTimeoutMinutes(task.Definition, submitTimeoutSettingKey); err == nil && ok {
			return time.Duration(minutes) * time.Minute
		}
	}
	return globalSubmitTimeout()
}

// effectiveStopTimeout 返回停止超时：任务覆盖 > 全局配置 > 默认 5 分钟。
// effectiveStopTimeout returns the stop timeout: task override > global config > 5 minutes.
func effectiveStopTimeout(task *Task) time.Duration {
	if task != nil {
		if minutes, ok, err := parseTaskTimeoutMinutes(task.Definition, stopTimeoutSettingKey); err == nil && ok {
			return time.Duration(minutes) * time.Minute
		}
	}
	return globalStopTimeout()
}

func formatTimeoutForMessage(timeout time.Duration) string {
	if timeout%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(timeout/time.Minute))
	}
	return timeout.Round(time.Second).String()
}

func parseRFC3339(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func jsonMapInt(m JSONMap, key string) int {
	value, ok := intValue(m, key)
	if !ok {
		return 0
	}
	return value
}

func jsonMapBool(m JSONMap, key string) bool {
	if m == nil {
		return false
	}
	value, ok := m[key].(bool)
	return ok && value
}

// submitOutcomeIsUnknown 判断作业是否仍是"提交结果未知"。
// submitOutcomeIsUnknown reports whether the job's submit outcome is still unknown.
func submitOutcomeIsUnknown(job *JobInstance) bool {
	return job != nil && strings.TrimSpace(stringValue(job.SubmitSpec, submitOutcomeKey)) == submitOutcomeUnknown
}

// submitOutcomeDeadline 返回提交确认截止时间：优先读登记时写入的截止时间，旧数据退回创建时间加全局超时。
// submitOutcomeDeadline returns the confirmation deadline: the stored deadline first, falling back to creation time
// plus the global timeout for older rows.
func submitOutcomeDeadline(job *JobInstance) time.Time {
	if job == nil {
		return time.Time{}
	}
	if deadline, ok := parseRFC3339(stringValue(job.SubmitSpec, submitDeadlineKey)); ok {
		return deadline
	}
	timeout := globalSubmitTimeout()
	if seconds := jsonMapInt(job.SubmitSpec, submitTimeoutSecondsKey); seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	if since, ok := parseRFC3339(stringValue(job.SubmitSpec, submitOutcomeSinceKey)); ok {
		return since.Add(timeout)
	}
	return job.CreatedAt.Add(timeout)
}

// submitOutcomePending 判断作业是否处于"提交确认中"且还没到截止时间。
// submitOutcomePending reports whether the job is still confirming its submit and the deadline has not passed.
func submitOutcomePending(job *JobInstance, now time.Time) bool {
	return submitOutcomeIsUnknown(job) && now.Before(submitOutcomeDeadline(job))
}

func markSubmitOutcome(job *JobInstance, outcome string, now time.Time) {
	if job == nil {
		return
	}
	if job.SubmitSpec == nil {
		job.SubmitSpec = JSONMap{}
	}
	job.SubmitSpec[submitOutcomeKey] = outcome
	job.SubmitSpec[submitOutcomeAtKey] = now.UTC().Format(time.RFC3339)
}

// isRecoverRun 判断作业是否沿用旧 jobId 的 savepoint 恢复提交。
// isRecoverRun reports whether the job is a savepoint recovery that reuses the previous jobId.
func isRecoverRun(job *JobInstance) bool {
	if job == nil {
		return false
	}
	return job.RunType == RunTypeRecover || jsonMapBool(job.SubmitSpec, "start_with_savepoint")
}

// submitOutcomeExpiredMessage 是提交确认超时后写入 error_message 的固定英文文案，前端按 submit_outcome=expired 本地化展示。
// submitOutcomeExpiredMessage is the stable English error_message for an expired submit; the UI localizes it from
// submit_outcome=expired.
func submitOutcomeExpiredMessage(job *JobInstance) string {
	timeout := globalSubmitTimeout()
	if seconds := jsonMapInt(job.SubmitSpec, submitTimeoutSecondsKey); seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	return fmt.Sprintf("submit timed out: the engine did not register this job within %s; check the engine logs before resubmitting",
		formatTimeoutForMessage(timeout))
}

// recordUnknownSubmitOutcome 在提交结果未知时登记一条 pending 作业，交给对账按 jobId 确认。
// recordUnknownSubmitOutcome records a pending job when the submit outcome is unknown so reconcile can confirm it by jobId.
func (s *Service) recordUnknownSubmitOutcome(ctx context.Context, task *Task, instance *JobInstance, endpoint *EngineEndpoint, unknownErr *EngineCallUnknownError) (*JobInstance, error) {
	now := time.Now().UTC()
	timeout := effectiveSubmitTimeout(task)
	instance.Status = JobStatusPending
	instance.EngineJobID = strings.TrimSpace(instance.PlatformJobID)
	if instance.SubmitSpec == nil {
		instance.SubmitSpec = JSONMap{}
	}
	apiMode := defaultString(unknownErr.APIMode, "v2")
	instance.SubmitSpec["engine_api_mode"] = apiMode
	instance.SubmitSpec["engine_base_url"] = unknownErr.BaseURL
	if strings.EqualFold(apiMode, "v1") {
		instance.SubmitSpec["engine_legacy_base_url"] = unknownErr.BaseURL
	} else if endpoint != nil && endpoint.ContextPath != "" {
		instance.SubmitSpec["engine_context_path"] = endpoint.ContextPath
	}
	if unknownErr.EngineURL != "" {
		instance.SubmitSpec["engine_url"] = unknownErr.EngineURL
	}
	instance.SubmitSpec[submitOutcomeKey] = submitOutcomeUnknown
	instance.SubmitSpec[submitOutcomeSinceKey] = now.Format(time.RFC3339)
	instance.SubmitSpec[submitTimeoutSecondsKey] = int(timeout / time.Second)
	instance.SubmitSpec[submitDeadlineKey] = now.Add(timeout).Format(time.RFC3339)
	instance.SubmitSpec[submitErrorKey] = audit.RedactText(unknownErr.Error())
	if err := s.repo.CreateJobInstance(ctx, instance); err != nil {
		return nil, fmt.Errorf("sync: record job with unknown submit outcome: %w (submit error: %v)", err, unknownErr)
	}
	logger.WarnF(ctx, "[Sync] engine submit outcome unknown, job recorded for confirmation: job=%d task=%d engine_job_id=%s deadline=%s err=%v",
		instance.ID, instance.TaskID, instance.EngineJobID, now.Add(timeout).Format(time.RFC3339), unknownErr)
	return instance, nil
}

// guardSubmitOutcomePending 在同一任务有提交结果还在确认中时拒绝新的提交，避免同一份数据被两个作业同时写。
// guardSubmitOutcomePending refuses a new submit while the task has one still being confirmed, so two jobs never
// write the same data at once.
func (s *Service) guardSubmitOutcomePending(ctx context.Context, taskID uint) error {
	if s == nil || s.repo == nil || taskID == 0 {
		return nil
	}
	pending, err := s.repo.ListPendingJobInstancesByTaskID(ctx, taskID, submitOutcomeGuardScanCap)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, job := range pending {
		if !submitOutcomePending(job, now) {
			continue
		}
		return fmt.Errorf("%w: job #%d is still being confirmed until %s; do not resubmit",
			ErrSubmitOutcomePending, job.ID, submitOutcomeDeadline(job).UTC().Format(time.RFC3339))
	}
	return nil
}

// applyCancelDeadline 在进入取消流程时记录停止截止时间；优先使用提交时写入的停止超时。
// applyCancelDeadline records the stop deadline when cancellation starts, preferring the stop timeout captured at submit.
func (s *Service) applyCancelDeadline(ctx context.Context, instance *JobInstance, now time.Time) {
	if instance == nil {
		return
	}
	timeout := time.Duration(jsonMapInt(instance.SubmitSpec, submitStopTimeoutSecsKey)) * time.Second
	if timeout <= 0 {
		var task *Task
		if s != nil && s.repo != nil && instance.TaskID > 0 {
			if loaded, err := s.repo.GetTaskByID(ctx, instance.TaskID); err == nil {
				task = loaded
			}
		}
		timeout = effectiveStopTimeout(task)
	}
	if instance.ResultPreview == nil {
		instance.ResultPreview = JSONMap{}
	}
	instance.ResultPreview[cancelRequestedAtKey] = now.UTC().Format(time.RFC3339)
	instance.ResultPreview[cancelTimeoutSecondsKey] = int(timeout / time.Second)
	instance.ResultPreview[cancelDeadlineKey] = now.Add(timeout).UTC().Format(time.RFC3339)
	delete(instance.ResultPreview, cancelOverdueAtKey)
	delete(instance.ResultPreview, cancelOutcomeKey)
	delete(instance.ResultPreview, cancelErrorKey)
}

// cancelDeadline 返回停止截止时间；没有记录时返回零值。
// cancelDeadline returns the stop deadline, or the zero time when none was recorded.
func cancelDeadline(job *JobInstance) time.Time {
	if job == nil {
		return time.Time{}
	}
	deadline, _ := parseRFC3339(stringValue(job.ResultPreview, cancelDeadlineKey))
	return deadline
}

// isCancelOverdue 判断取消中的作业是否已经越过停止截止时间。
// isCancelOverdue reports whether a cancelling job has passed its stop deadline.
func isCancelOverdue(job *JobInstance, now time.Time) bool {
	if job == nil || !isCancellingJobStatus(job.Status) {
		return false
	}
	deadline := cancelDeadline(job)
	return !deadline.IsZero() && !now.Before(deadline)
}
