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
	"fmt"
	"regexp"
	"strings"
	"time"
)

var unreplacedPlaceholderPattern = regexp.MustCompile(`^\$\{.+\}$`)

const (
	dolphinExternalRefPrefix  = "ds:"
	scheduleTimeCompactLayout = "20060102150405"
)

// SubmitOptions 描述一次正式提交的可选覆盖项，空值保持原有提交行为。
// SubmitOptions holds optional per-submit overrides; empty values keep legacy submit behavior.
type SubmitOptions struct {
	Draft        *TaskDraftPayload
	Variables    map[string]string
	ExternalRef  string
	ScheduleTime time.Time
	CloseJob     *bool
}

// CancelByRefRequest 按外部引用停止作业。
// CancelByRefRequest stops a job by its external scheduler reference.
type CancelByRefRequest struct {
	ExternalRef      string `json:"external_ref"`
	AttachGeneration int    `json:"attach_generation"`
}

// parseSubmitOptions 把 HTTP 请求转换成提交选项，并拒绝未替换的 ${...} 变量值。
// parseSubmitOptions converts an HTTP request into submit options and rejects leftover ${...} values.
func parseSubmitOptions(req TaskActionRequest) (*SubmitOptions, error) {
	opts := &SubmitOptions{
		Draft:       req.Draft,
		ExternalRef: strings.TrimSpace(req.ExternalRef),
		CloseJob:    req.CloseJob,
	}
	if len(req.Variables) > 0 {
		opts.Variables = make(map[string]string, len(req.Variables))
		for key, value := range req.Variables {
			name := strings.TrimSpace(key)
			if name == "" {
				continue
			}
			if isUnreplacedPlaceholder(value) {
				return nil, fmt.Errorf("%w: %s=%s", ErrUnreplacedVariable, name, strings.TrimSpace(value))
			}
			opts.Variables[name] = value
		}
	}
	scheduleTime, err := parseScheduleTime(req.ScheduleTime)
	if err != nil {
		return nil, err
	}
	opts.ScheduleTime = scheduleTime
	return opts, nil
}

// parseScheduleTime 接受 RFC3339 或 yyyyMMddHHmmss；空字符串表示使用当前时间。
// parseScheduleTime accepts RFC3339 or yyyyMMddHHmmss; an empty value means "use now".
func parseScheduleTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.ParseInLocation(scheduleTimeCompactLayout, value, time.Local); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("%w: %s", ErrInvalidScheduleTime, value)
}

func isUnreplacedPlaceholder(value string) bool {
	return unreplacedPlaceholderPattern.MatchString(strings.TrimSpace(value))
}

func isActiveJobStatus(status JobStatus) bool {
	switch status {
	case JobStatusPending, JobStatusRunning, JobStatusCancelRequested, JobStatusCancelling:
		return true
	default:
		return false
	}
}

func isCancellingJobStatus(status JobStatus) bool {
	return status == JobStatusCancelRequested || status == JobStatusCancelling
}

// parseDolphinExternalRef 解析 ds:{workflowInstanceId}:{taskCode}。
// parseDolphinExternalRef parses ds:{workflowInstanceId}:{taskCode}.
func parseDolphinExternalRef(ref string) (workflowInstanceID, taskCode string, ok bool) {
	value := strings.TrimSpace(ref)
	if !strings.HasPrefix(strings.ToLower(value), dolphinExternalRefPrefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(value[len(dolphinExternalRefPrefix):])
	workflowInstanceID, taskCode, found := strings.Cut(rest, ":")
	workflowInstanceID = strings.TrimSpace(workflowInstanceID)
	taskCode = strings.TrimSpace(taskCode)
	if !found || workflowInstanceID == "" || taskCode == "" {
		return "", "", false
	}
	return workflowInstanceID, taskCode, true
}

func (opts *SubmitOptions) variableOverrides() map[string]string {
	if opts == nil {
		return nil
	}
	return opts.Variables
}

func (opts *SubmitOptions) externalRef() string {
	if opts == nil {
		return ""
	}
	return strings.TrimSpace(opts.ExternalRef)
}

func (opts *SubmitOptions) scheduleTimeOrNow() time.Time {
	if opts != nil && !opts.ScheduleTime.IsZero() {
		return opts.ScheduleTime
	}
	return time.Now()
}

func applySubmitOptionsToInstance(instance *JobInstance, opts *SubmitOptions) {
	if instance == nil || opts == nil {
		return
	}
	if ref := opts.externalRef(); ref != "" {
		instance.ExternalRef = ref
		if instance.AttachGeneration <= 0 {
			instance.AttachGeneration = 1
		}
	}
	if instance.SubmitSpec == nil {
		instance.SubmitSpec = JSONMap{}
	}
	if opts.CloseJob != nil {
		instance.SubmitSpec["close_job"] = *opts.CloseJob
	}
	if !opts.ScheduleTime.IsZero() {
		instance.SubmitSpec["schedule_time"] = opts.ScheduleTime.Format(time.RFC3339)
	}
}
