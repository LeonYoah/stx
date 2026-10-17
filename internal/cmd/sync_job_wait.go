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
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// jobWaitPollInterval 是 --wait 轮询作业接口的间隔，测试可改短。
// jobWaitPollInterval is how often --wait polls GetJob; tests may shorten it.
var jobWaitPollInterval = 5 * time.Second

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

func waitSyncJob(command *cobra.Command, client *cliClient.Client, operationID string, job syncJobWaitResult, mode string, timeout time.Duration, closeJob bool, pipe *os.File) error {
	if job.ID == 0 {
		return clioutput.NewError(clioutput.CodeServer, "submit succeeded without a job id", clioutput.ExitServer, false)
	}
	streaming := strings.EqualFold(strings.TrimSpace(mode), "streaming")
	ctx := command.Context()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	writeGuardGeneration(pipe, job.AttachGeneration)

	for {
		var latest syncJobWaitResult
		requestID, err := client.Request(signalCtx, "GET", jobPath(job.ID), nil, &latest)
		if err != nil {
			if signalCtx.Err() != nil && closeJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
				writeGuardDone(pipe)
				_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
				return clioutput.NewError(clioutput.CodeExecution, "job cancelled", clioutput.ExitExecution, false)
			}
			if isWaitTimeout(signalCtx, timeout) && closeJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
				writeGuardDone(pipe)
				_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
				return clioutput.NewError(clioutput.CodeTimeout, "wait timeout; job cancel requested", clioutput.ExitTimeout, false)
			}
			return err
		}
		if latest.ID == 0 {
			latest.ID = job.ID
		}
		job = latest
		writeGuardGeneration(pipe, job.AttachGeneration)

		status := strings.ToLower(strings.TrimSpace(job.Status))
		if streaming && status == "running" {
			writeGuardDone(pipe)
			if err := renderCommandResultWithRequestID(command, operationID, requestID, job); err != nil {
				return err
			}
			return nil
		}
		if isSyncJobTerminal(status) {
			writeGuardDone(pipe)
			if err := renderCommandResultWithRequestID(command, operationID, requestID, job); err != nil {
				return err
			}
			if status == "success" {
				return nil
			}
			return clioutput.NewError(clioutput.CodeExecution, "job ended with status "+status, clioutput.ExitExecution, false)
		}

		timer := time.NewTimer(jobWaitPollInterval)
		select {
		case <-signalCtx.Done():
			timer.Stop()
			if closeJob {
				_ = cancelSyncJob(command.Context(), client, job.ID)
			}
			writeGuardDone(pipe)
			_ = renderCommandResultWithRequestID(command, operationID, requestID, job)
			if isWaitTimeout(signalCtx, timeout) {
				return clioutput.NewError(clioutput.CodeTimeout, "wait timeout; job cancel requested", clioutput.ExitTimeout, false)
			}
			return clioutput.NewError(clioutput.CodeExecution, "job cancelled", clioutput.ExitExecution, false)
		case <-timer.C:
		}
	}
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
	_, err := client.RequestWithHeaders(ctx, "POST", jobPath(jobID)+"/cancel", map[string]any{"stop_with_savepoint": false}, headers, &data)
	return err
}

func jobPath(id uint) string {
	return "/api/v1/sync/jobs/" + itoaUint(id)
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

func loadTaskMode(ctx context.Context, client *cliClient.Client, taskID uint) string {
	if client == nil || taskID == 0 {
		return ""
	}
	var task struct {
		Mode string `json:"mode"`
	}
	_, err := client.Request(ctx, "GET", "/api/v1/sync/tasks/"+itoaUint(taskID), nil, &task)
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
