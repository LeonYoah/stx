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
		if previousStatus == JobStatusCancelRequested || previousStatus == JobStatusCancelling {
			if observedStatus == JobStatusPending || observedStatus == JobStatusRunning {
				observedStatus = previousStatus
			} else if observedStatus == JobStatusSuccess {
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
	if info.Metrics != nil {
		instance.ResultPreview = maybeRecordMetricsLog(
			instance.ResultPreview,
			info.Metrics,
			time.Now().UTC(),
			isFinalNormalizedJobStatus(observedStatus),
		)
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
	if err := s.syncExecutionFromJob(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}
