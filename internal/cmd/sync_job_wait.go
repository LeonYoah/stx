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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// jobWaitPollInterval 是 --wait 轮询作业状态的间隔，测试可改短。
// jobWaitPollInterval is how often --wait polls GetJob; tests may shorten it.
var jobWaitPollInterval = 5 * time.Second

// jobLogFollowInterval 是 --wait 默认跟随日志的间隔，测试可改短。
// jobLogFollowInterval is how often --wait pulls incremental job logs by default; tests may shorten it.
var jobLogFollowInterval = 3 * time.Second

const jobLogFollowChunkBytes = 64 * 1024

// defaultJobObservationTimeout 是 sync_state=stale 时 --wait 默认失败前的观察窗口。
// defaultJobObservationTimeout is the default --wait window before failing when sync_state stays stale.
const defaultJobObservationTimeout = 3 * time.Minute

type syncJobWaitResult struct {
	ID               uint   `json:"id"`
	TaskID           uint   `json:"task_id"`
	Status           string `json:"status"`
	Mode             string `json:"mode,omitempty"`
	RunType          string `json:"run_type,omitempty"`
	ExecutionID      string `json:"execution_id,omitempty"`
	ExternalRef      string `json:"external_ref,omitempty"`
	AttachGeneration int    `json:"attach_generation"`
	ErrorMessage     string `json:"error_message,omitempty"`
	SyncState        string `json:"sync_state,omitempty"`
	SyncLagMs        *int64 `json:"sync_lag_ms,omitempty"`
}

// syncJobWaitConfig 汇总 --wait 轮询、日志跟随、观察超时和 Dolphin 守护管道。
// syncJobWaitConfig collects --wait polling, log follow, observation timeout, and the Dolphin guard pipe.
type syncJobWaitConfig struct {
	Mode               string
	Timeout            time.Duration
	ObservationTimeout time.Duration
	CloseJob           bool
	FollowLogs         bool
	// Pipe 向 job-guard 写入 generation / done；非 Dolphin 提交时为 nil。
	// Pipe writes generation / done to job-guard; nil for non-Dolphin submits.
	Pipe *os.File
}

// parseCLIWaitTimeout 解析 --wait 的可选上限；空值表示一直等到作业终态。
// parseCLIWaitTimeout parses the optional --wait ceiling; empty means wait until the job ends.
func parseCLIWaitTimeout(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return 0, clioutput.NewError(clioutput.CodeUsage, "--timeout must be a duration such as 6h", clioutput.ExitUsage, false)
	}
	return timeout, nil
}

func waitSyncJob(command *cobra.Command, client *cliClient.Client, operationID string, job syncJobWaitResult, cfg syncJobWaitConfig) error {
	if job.ID == 0 {
		return clioutput.NewError(clioutput.CodeServer, "submit succeeded without a job id", clioutput.ExitServer, false)
	}
	streaming := strings.EqualFold(strings.TrimSpace(cfg.Mode), "streaming")
	ctx := command.Context()
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	events := clioutput.NewEventWriter(command.ErrOrStderr())
	logState := newJobLogFollowState()
	var staleSince time.Time
	writeGuardGeneration(cfg.Pipe, job.AttachGeneration)

	for {
		var latest syncJobWaitResult
		requestID, err := client.Request(signalCtx, http.MethodGet, jobPath(job.ID), nil, &latest)
		if err != nil {
			if signalCtx.Err() != nil && cfg.CloseJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
				writeGuardDone(cfg.Pipe)
				_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
				return clioutput.NewError(clioutput.CodeExecution, "job cancelled", clioutput.ExitExecution, false)
			}
			if isWaitTimeout(signalCtx, cfg.Timeout) && cfg.CloseJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
				writeGuardDone(cfg.Pipe)
				_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
				return clioutput.NewError(clioutput.CodeTimeout, "wait timeout; job cancel requested", clioutput.ExitTimeout, false)
			}
			return err
		}
		if latest.ID == 0 {
			latest.ID = job.ID
		}
		emitJobStatusEvent(events, operationID, requestID, job, latest)
		job = latest
		writeGuardGeneration(cfg.Pipe, job.AttachGeneration)

		if cfg.FollowLogs {
			_ = followJobLogsOnce(signalCtx, client, events, operationID, job.ID, logState)
		}

		status := strings.ToLower(strings.TrimSpace(job.Status))
		if !isSyncJobTerminal(status) && staleObservationExceeded(job, cfg.ObservationTimeout, &staleSince) {
			message := fmt.Sprintf("engine observation stale for at least %s while job status is %s", formatCLIDuration(cfg.ObservationTimeout), status)
			_ = events.Emit(clioutput.Event{
				Event:       "job_observation_lost",
				OperationID: operationID,
				RequestID:   requestID,
				Level:       "error",
				JobID:       job.ID,
				Status:      status,
				SyncState:   strings.TrimSpace(job.SyncState),
				SyncLagMs:   job.SyncLagMs,
				Message:     message,
			})
			if cfg.CloseJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
			}
			writeGuardDone(cfg.Pipe)
			_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
			return clioutput.NewError(clioutput.CodeTimeout, message, clioutput.ExitTimeout, true)
		}
		if streaming && status == "running" {
			if cfg.FollowLogs {
				drainJobLogs(signalCtx, client, events, operationID, job.ID, logState, 2)
			}
			writeGuardDone(cfg.Pipe)
			if err := renderCommandResultWithRequestID(command, operationID, requestID, job); err != nil {
				return err
			}
			return nil
		}
		if isSyncJobTerminal(status) {
			if cfg.FollowLogs {
				drainJobLogs(signalCtx, client, events, operationID, job.ID, logState, 2)
			}
			writeGuardDone(cfg.Pipe)
			if err := renderCommandResultWithRequestID(command, operationID, requestID, job); err != nil {
				return err
			}
			if status == "success" {
				return nil
			}
			message := strings.TrimSpace(job.ErrorMessage)
			if message == "" {
				message = "job ended with status " + status
			}
			return clioutput.NewError(clioutput.CodeExecution, message, clioutput.ExitExecution, false)
		}

		timer := time.NewTimer(jobWaitPollInterval)
		select {
		case <-signalCtx.Done():
			timer.Stop()
			if cfg.CloseJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
			}
			writeGuardDone(cfg.Pipe)
			_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
			if isWaitTimeout(signalCtx, cfg.Timeout) {
				return clioutput.NewError(clioutput.CodeTimeout, "wait timeout; job cancel requested", clioutput.ExitTimeout, false)
			}
			return clioutput.NewError(clioutput.CodeExecution, "job cancelled", clioutput.ExitExecution, false)
		case <-timer.C:
		}
	}
}

func emitJobStatusEvent(writer *clioutput.EventWriter, operationID, requestID string, previous, latest syncJobWaitResult) {
	if writer == nil {
		return
	}
	status := strings.TrimSpace(latest.Status)
	syncState := strings.TrimSpace(latest.SyncState)
	if status == strings.TrimSpace(previous.Status) && syncState == strings.TrimSpace(previous.SyncState) {
		return
	}
	_ = writer.Emit(clioutput.Event{
		Event:       "job_status",
		OperationID: operationID,
		RequestID:   requestID,
		Level:       "info",
		JobID:       latest.ID,
		Status:      status,
		SyncState:   syncState,
		SyncLagMs:   latest.SyncLagMs,
		Message:     status,
	})
}

func cancelSyncJob(ctx context.Context, client *cliClient.Client, jobID uint) error {
	if client == nil || jobID == 0 {
		return nil
	}
	headers := map[string]string{
		"Idempotency-Key": "cli-wait-cancel-" + time.Now().UTC().Format("20060102150405.000000000"),
		"X-STX-Confirm":   "true",
	}
	var data any
	_, err := client.RequestWithHeaders(ctx, http.MethodPost, jobPath(jobID)+"/cancel", map[string]any{"stop_with_savepoint": false}, headers, &data)
	return err
}

func jobPath(id uint) string {
	return "/api/v1/sync/jobs/" + itoaUint(id)
}

func jobLogsPath(id uint, offset string, limitBytes int) string {
	path := "/api/v1/sync/jobs/" + itoaUint(id) + "/logs?limit_bytes=" + itoaUint(uint(limitBytes))
	offset = strings.TrimSpace(offset)
	if offset != "" {
		path += "&offset=" + url.QueryEscape(offset)
	}
	return path
}

func isSyncJobTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func isWaitTimeout(ctx context.Context, timeout time.Duration) bool {
	if timeout <= 0 || ctx == nil {
		return false
	}
	return ctx.Err() == context.DeadlineExceeded
}

// staleObservationExceeded 判断非终态作业是否在 stale 观察窗内已超过上限。
// staleObservationExceeded reports whether a non-terminal job exceeded the stale observation window.
func staleObservationExceeded(job syncJobWaitResult, limit time.Duration, staleSince *time.Time) bool {
	if limit <= 0 {
		if staleSince != nil {
			*staleSince = time.Time{}
		}
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(job.SyncState), "stale") {
		if staleSince != nil {
			*staleSince = time.Time{}
		}
		return false
	}
	if job.SyncLagMs != nil && *job.SyncLagMs >= limit.Milliseconds() {
		return true
	}
	if staleSince == nil {
		return false
	}
	if staleSince.IsZero() {
		*staleSince = time.Now()
		return false
	}
	return time.Since(*staleSince) >= limit
}

func formatCLIDuration(value time.Duration) string {
	if value <= 0 {
		return "0s"
	}
	return value.String()
}

func loadTaskMode(ctx context.Context, client *cliClient.Client, taskID uint) string {
	if client == nil || taskID == 0 {
		return ""
	}
	var task struct {
		Mode string `json:"mode"`
	}
	_, err := client.Request(ctx, http.MethodGet, "/api/v1/sync/tasks/"+itoaUint(taskID), nil, &task)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(task.Mode))
}

func itoaUint(value uint) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 20)
	for value > 0 {
		digits = append(digits, byte('0'+value%10))
		value /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
