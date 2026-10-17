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

package cmd

import (
	"context"
	"net/http"
	"strings"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

type syncJobLogsPayload struct {
	Logs              string `json:"logs"`
	NextOffset        string `json:"next_offset"`
	EmptyReason       string `json:"empty_reason"`
	ClusterJobLogMode string `json:"cluster_job_log_mode"`
}

type jobLogFollowState struct {
	offset           string
	lastPoll         time.Time
	notices          map[string]struct{}
	lastFailure      time.Time
	failureBackoff   time.Duration
}

func newJobLogFollowState() *jobLogFollowState {
	return &jobLogFollowState{
		notices:        make(map[string]struct{}),
		failureBackoff: jobLogFollowInterval,
	}
}

func followJobLogsOnce(ctx context.Context, client *cliClient.Client, events *clioutput.EventWriter, operationID string, jobID uint, state *jobLogFollowState) error {
	if client == nil || state == nil || jobID == 0 {
		return nil
	}
	now := time.Now()
	if !state.lastPoll.IsZero() && now.Sub(state.lastPoll) < jobLogFollowInterval {
		return nil
	}
	if !state.lastFailure.IsZero() && now.Sub(state.lastFailure) < state.failureBackoff {
		return nil
	}
	state.lastPoll = now
	return pullAndEmitJobLogs(ctx, client, events, operationID, jobID, state)
}

func drainJobLogs(ctx context.Context, client *cliClient.Client, events *clioutput.EventWriter, operationID string, jobID uint, state *jobLogFollowState, attempts int) {
	if client == nil || state == nil || attempts <= 0 {
		return
	}
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return
		}
		before := state.offset
		_ = pullAndEmitJobLogs(ctx, client, events, operationID, jobID, state)
		if strings.TrimSpace(state.offset) == strings.TrimSpace(before) {
			return
		}
	}
}

func pullAndEmitJobLogs(ctx context.Context, client *cliClient.Client, events *clioutput.EventWriter, operationID string, jobID uint, state *jobLogFollowState) error {
	var payload syncJobLogsPayload
	requestID, err := client.Request(ctx, http.MethodGet, jobLogsPath(jobID, state.offset, jobLogFollowChunkBytes), nil, &payload)
	if err != nil {
		state.lastFailure = time.Now()
		if state.failureBackoff < 30*time.Second {
			state.failureBackoff *= 2
		}
		return err
	}
	state.lastFailure = time.Time{}
	state.failureBackoff = jobLogFollowInterval

	if next := strings.TrimSpace(payload.NextOffset); next != "" {
		state.offset = next
	}

	emitLogNotices(events, operationID, requestID, jobID, state, payload)

	if chunk := payload.Logs; strings.TrimSpace(chunk) != "" {
		_ = events.Emit(clioutput.Event{
			Event:       "log_chunk",
			OperationID: operationID,
			RequestID:   requestID,
			Level:       "info",
			JobID:       jobID,
			NextOffset:  state.offset,
			Logs:        chunk,
		})
	}
	return nil
}

func emitLogNotices(events *clioutput.EventWriter, operationID, requestID string, jobID uint, state *jobLogFollowState, payload syncJobLogsPayload) {
	if events == nil || state == nil {
		return
	}
	reason := strings.TrimSpace(payload.EmptyReason)
	if reason == "mixed_log_mode" || strings.TrimSpace(payload.ClusterJobLogMode) == "mixed" {
		reason = "mixed_log_mode"
	}
	if reason == "" {
		return
	}
	if _, seen := state.notices[reason]; seen {
		return
	}
	state.notices[reason] = struct{}{}
	message := reason
	if reason == "mixed_log_mode" {
		message = "cluster uses mixed log mode; per-job log follow is unavailable"
	}
	_ = events.Emit(clioutput.Event{
		Event:       "log_notice",
		OperationID: operationID,
		RequestID:   requestID,
		Level:       "warning",
		JobID:       jobID,
		EmptyReason: reason,
		Message:     message,
	})
}
