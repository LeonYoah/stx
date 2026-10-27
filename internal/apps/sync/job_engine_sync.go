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
	"fmt"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

func engineListItemToInfo(item *EngineJobListItem) *EngineJobInfo {
	if item == nil {
		return nil
	}
	return &EngineJobInfo{
		JobID:        item.JobID,
		JobName:      item.JobName,
		JobStatus:    item.JobStatus,
		CreateTime:   item.CreateTime,
		FinishedTime: item.FinishedTime,
		ErrorMsg:     item.ErrorMsg,
	}
}

func (s *Service) recordJobSyncFailure(ctx context.Context, instance *JobInstance) {
	if s == nil || s.repo == nil || instance == nil {
		return
	}
	instance.SyncFailCount++
	state := JobSyncStateDegraded
	if instance.SyncFailCount >= 3 {
		state = JobSyncStateStale
	}
	instance.SyncState = state
	_ = s.repo.UpdateJobSyncMetadata(ctx, instance.ID, map[string]any{
		"sync_state":      state,
		"sync_fail_count": instance.SyncFailCount,
	})
}

func (s *Service) applyEngineJobInfoUpdate(ctx context.Context, instance *JobInstance, info *EngineJobInfo) (*JobInstance, error) {
	if instance == nil {
		return nil, nil
	}
	if info == nil {
		return instance, nil
	}
	// 提交结果未知的作业：引擎给出真实状态才算确认；空壳交给对账判断是否过期。
	// 恢复提交沿用旧 jobId，截止前不采信可能属于上一轮运行的终态。
	// Unknown-outcome submits are confirmed only by a real engine status; empty shells are left to reconcile's expiry
	// check. Recovery submits reuse the old jobId, so terminal states that may belong to the previous run are ignored
	// until the deadline.
	submitConfirmed := false
	if submitOutcomeIsUnknown(instance) {
		status := strings.TrimSpace(info.JobStatus)
		if status == "" {
			return instance, nil
		}
		if isRecoverRun(instance) && isFinalEngineJobStatus(status) && submitOutcomePending(instance, time.Now()) {
			return instance, nil
		}
		markSubmitOutcome(instance, submitOutcomeConfirmed, time.Now())
		submitConfirmed = true
	}
	previousStatus := instance.Status
	rawEngineStatus := strings.TrimSpace(info.JobStatus)
	var observedStatus JobStatus
	if rawEngineStatus == "" {
		if previousStatus == JobStatusCancelRequested || previousStatus == JobStatusCancelling {
			observedStatus = JobStatusCanceled
		} else {
			observedStatus = previousStatus
		}
	} else {
		observedStatus = normalizeJobStatus(rawEngineStatus)
		if previousStatus == JobStatusCancelRequested || previousStatus == JobStatusCancelling || previousStatus == JobStatusDoingSavepoint {
			if observedStatus == JobStatusPending || observedStatus == JobStatusRunning {
				observedStatus = previousStatus
			} else if observedStatus == JobStatusSuccess {
				// Stop-with-savepoint completion is SAVEPOINT_DONE; plain FINISHED while cancelling → canceled.
				// 带 Savepoint 停完是 SAVEPOINT_DONE；取消过程中若报 FINISHED 则记为 canceled。
				observedStatus = JobStatusCanceled
			}
		}
	}
	if previousStatus == JobStatusRunning && observedStatus == JobStatusPending {
		observedStatus = previousStatus
	}
	if isFinalNormalizedJobStatus(previousStatus) {
		observedStatus = previousStatus
	}
	instance.Status = observedStatus
	instance.ResultPreview = mergeJobRuntimeInfo(instance.ResultPreview, info)
	terminal := isFinalNormalizedJobStatus(observedStatus)
	nowUTC := time.Now().UTC()
	if info.Metrics != nil {
		instance.ResultPreview = maybeRecordMetricsLog(
			instance.ResultPreview,
			info.Metrics,
			nowUTC,
			terminal,
		)
	} else if terminal && !hasFinalMetricsLog(instance.ResultPreview) {
		// Engine omitted metrics on this tick: still force a final block from last known structured metrics.
		// 本轮 job-info 无 metrics 时，用已存结构化指标补写终态块，避免停在中途 snapshot。
		if existing := mapValue(instance.ResultPreview, "metrics"); len(existing) > 0 {
			instance.ResultPreview = maybeRecordMetricsLog(instance.ResultPreview, existing, nowUTC, true)
		}
	}
	if isFinalNormalizedJobStatus(instance.Status) {
		if parsedFinishedAt := parseEngineJobTime(info.FinishedTime); parsedFinishedAt != nil {
			instance.FinishedAt = parsedFinishedAt
		} else if instance.FinishedAt == nil {
			now := time.Now().UTC()
			instance.FinishedAt = &now
		}
		if info.ErrorMsg != nil {
			instance.ErrorMessage = strings.TrimSpace(fmt.Sprintf("%v", info.ErrorMsg))
		}
	} else {
		instance.FinishedAt = nil
	}
	// 停止结果未知的作业到达终态即视为停止已确认。
	// A stop with an unknown outcome is confirmed once the job reaches a terminal state.
	if terminal && strings.TrimSpace(stringValue(instance.ResultPreview, cancelOutcomeKey)) == cancelOutcomeUnknown {
		instance.ResultPreview[cancelOutcomeKey] = cancelOutcomeConfirmed
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":          instance.Status,
		"result_preview":  instance.ResultPreview,
		"error_message":   instance.ErrorMessage,
		"finished_at":     instance.FinishedAt,
		"last_synced_at":  now,
		"sync_state":      JobSyncStateOK,
		"sync_fail_count": 0,
	}
	if submitConfirmed {
		updates["submit_spec"] = instance.SubmitSpec
	}
	if err := s.repo.UpdateJobStatus(ctx, instance.ID, []JobStatus{previousStatus}, updates); err != nil {
		if errors.Is(err, ErrJobStatusChanged) {
			return s.repo.GetJobInstanceByID(ctx, instance.ID)
		}
		return nil, err
	}
	instance.LastSyncedAt = &now
	instance.SyncState = JobSyncStateOK
	instance.SyncFailCount = 0
	if err := s.syncPreviewSessionTerminalState(ctx, instance); err != nil {
		return nil, err
	}
	reconcileCtx := executionapp.WithTriggerSource(ctx, executionapp.TriggerSourceReconciler)
	if err := s.syncExecutionFromJob(reconcileCtx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}
