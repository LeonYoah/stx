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
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// 计划 DAG 不可用原因，前端按 reason 本地化说明；都以 200 + available=false 返回，不作为错误。
// Plan DAG unavailability reasons, localized by the frontend; all are returned as 200 + available=false, not errors.
const (
	PlanDAGReasonNoSubmittedContent    = "no_submitted_content"
	PlanDAGReasonLocalMode             = "local_mode"
	PlanDAGReasonConfigToolUnavailable = "config_tool_unavailable"
	PlanDAGReasonInspectFailed         = "inspect_failed"

	// planDAGMessageMaxRunes 限制失败摘要长度，避免把整段堆栈带给前端。
	// planDAGMessageMaxRunes caps the failure summary so whole stack traces never reach the frontend.
	planDAGMessageMaxRunes = 500
)

// JobPlanDAGResult 是作业提交时配置推导出的计划 DAG：表结构、列与建表计划，用来补齐引擎运行时 DAG 缺少的信息。
// JobPlanDAGResult is the plan DAG derived from the job's submitted config: table schemas, columns and save-mode
// plans that fill in what the engine's runtime DAG lacks.
type JobPlanDAGResult struct {
	JobID     uint   `json:"job_id"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	// Message 只在 inspect_failed 时出现，已脱敏并截断。
	// Message is only set for inspect_failed, already redacted and truncated.
	Message string `json:"message,omitempty"`
	// WebUIJob 与 /tasks/:id/dag 的 webui_job 同构（WebUI 兼容 jobDag），前端用同一套解析。
	// WebUIJob has the same shape as webui_job from /tasks/:id/dag (WebUI-compatible jobDag), parsed by the same code.
	WebUIJob JSONMap `json:"webui_job,omitempty"`
	// Warnings 永远是数组，不为 null。
	// Warnings is always an array, never null.
	Warnings []string `json:"warnings"`
}

// GetJobPlanDAGForActor 校验作业可见权限后返回计划 DAG。
// GetJobPlanDAGForActor returns the plan DAG after checking the job is visible to the actor.
func (s *Service) GetJobPlanDAGForActor(ctx context.Context, actor executionapp.Actor, id uint) (*JobPlanDAGResult, error) {
	job, err := s.getJobInstanceForActor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return s.getJobPlanDAG(ctx, job)
}

// getJobPlanDAG 用 submit_spec.submitted_content 调 config tool 的 WebUI DAG；输入不可变且调用很轻，所以不缓存，
// 也不写回 result_preview，避免与对账器并发写同一字段。
// getJobPlanDAG inspects submit_spec.submitted_content through the config tool's WebUI DAG. The input is immutable
// and the call is light, so nothing is cached or written back to result_preview, which the reconciler also writes.
func (s *Service) getJobPlanDAG(ctx context.Context, job *JobInstance) (*JobPlanDAGResult, error) {
	result := &JobPlanDAGResult{JobID: job.ID, Warnings: []string{}}
	if submitSpecExecutionMode(job.SubmitSpec) == "local" {
		result.Reason = PlanDAGReasonLocalMode
		return result, nil
	}
	content := strings.TrimSpace(stringValue(job.SubmitSpec, "submitted_content"))
	if content == "" {
		result.Reason = PlanDAGReasonNoSubmittedContent
		return result, nil
	}
	if s.configToolClient == nil || s.configToolResolver == nil {
		result.Reason = PlanDAGReasonConfigToolUnavailable
		return result, nil
	}

	// 任务定义可能带 proxy_base_url 覆盖，与编辑器 DAG 保持同一个 config tool。
	// The task definition may override proxy_base_url; use the same config tool as the editor DAG.
	var definition JSONMap
	if job.TaskID > 0 && s.repo != nil {
		if task, err := s.repo.GetTaskByID(ctx, job.TaskID); err == nil && task != nil {
			definition = task.Definition
		}
	}
	clusterID := clusterIDForJob(ctx, s, job, map[uint]uint{})
	endpoint, err := s.configToolResolver.ResolveConfigToolEndpoint(ctx, clusterID, definition)
	if err != nil || strings.TrimSpace(endpoint) == "" {
		result.Reason = PlanDAGReasonConfigToolUnavailable
		return result, nil
	}

	resp, err := s.configToolClient.InspectWebUIDAG(ctx, endpoint, &ConfigToolContentRequest{
		Content:       content,
		ContentFormat: normalizeSubmitFormat(stringValue(job.SubmitSpec, "submitted_format", "format")),
	})
	if err != nil || resp == nil {
		result.Reason = PlanDAGReasonInspectFailed
		if err != nil {
			result.Message = truncatePlanDAGMessage(audit.RedactText(strings.TrimSpace(err.Error())))
		}
		return result, nil
	}
	webuiJob, err := structToJSONMap(resp)
	if err != nil {
		return nil, err
	}
	result.Available = true
	result.WebUIJob = webuiJob
	result.Warnings = append(result.Warnings, resp.Warnings...)
	return result, nil
}

// truncatePlanDAGMessage 按字符截断失败摘要。
// truncatePlanDAGMessage truncates the failure summary by runes.
func truncatePlanDAGMessage(message string) string {
	runes := []rune(message)
	if len(runes) <= planDAGMessageMaxRunes {
		return message
	}
	return string(runes[:planDAGMessageMaxRunes]) + "…"
}
