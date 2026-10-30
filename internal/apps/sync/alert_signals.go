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
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// SetAlertReporter 注入作业告警信号的接收方；未注入时 sync 不产生任何告警信号。
// SetAlertReporter injects the receiver of job alert signals; without it sync emits no alert signals.
func (s *Service) SetAlertReporter(reporter syncalert.Reporter) { s.alertReporter = reporter }

// SetTaskAlertPolicyStore 注入任务告警配置的存取实现；未注入时任务告警接口返回 503。
// SetTaskAlertPolicyStore injects the task alert config store; without it the task alert API returns 503.
func (s *Service) SetTaskAlertPolicyStore(store syncalert.TaskPolicyStore) { s.alertStore = store }

// reportAlert 发出一条信号；接收方必须非阻塞，这里不做任何等待。返回信号是否被接收（接收方不支持
// syncalert.AcceptingReporter 时总是 true）。
// reportAlert emits one signal; the receiver must not block and nothing here waits. It returns whether the signal was
// accepted (always true when the receiver does not implement syncalert.AcceptingReporter).
func (s *Service) reportAlert(signal syncalert.Signal) bool {
	if s == nil || s.alertReporter == nil {
		return false
	}
	if signal.OccurredAt.IsZero() {
		signal.OccurredAt = time.Now().UTC()
	}
	if accepting, ok := s.alertReporter.(syncalert.AcceptingReporter); ok {
		return accepting.TryReportSyncAlert(signal)
	}
	s.alertReporter.ReportSyncAlert(signal)
	return true
}

// alertSignalForJob 用作业和任务的快照组装作业级信号；数据预览不产生信号，返回 false。
// alertSignalForJob builds a job-level signal from job and task snapshots; data previews produce none (false).
func (s *Service) alertSignalForJob(ctx context.Context, kind syncalert.SignalKind, rule string, job *JobInstance, task *Task, message string) (syncalert.Signal, bool) {
	if job == nil || job.RunType == RunTypePreview || job.TaskID == 0 {
		return syncalert.Signal{}, false
	}
	if task == nil && s.repo != nil {
		task, _ = s.repo.GetTaskByID(ctx, job.TaskID)
	}
	signal := syncalert.Signal{
		Kind:          kind,
		RuleKey:       rule,
		ClusterID:     uintValue(job.SubmitSpec, "cluster_id"),
		TaskID:        job.TaskID,
		JobID:         job.ID,
		EngineJobID:   strings.TrimSpace(job.EngineJobID),
		TriggerSource: strings.TrimSpace(stringValue(job.SubmitSpec, "trigger_source")),
		RunType:       string(job.RunType),
		Message:       redactAlertMessage(message),
	}
	if task != nil {
		signal.TaskName = strings.TrimSpace(task.Name)
		signal.TaskCreatorID = task.CreatedBy
		if signal.ClusterID == 0 {
			signal.ClusterID = task.ClusterID
		}
	}
	return signal, true
}

// redactAlertMessage 脱敏并截断错误信息，避免把密码等写进告警和通知。
// redactAlertMessage redacts and truncates an error message so secrets never reach alerts or notifications.
func redactAlertMessage(message string) string {
	return syncalert.TruncateMessage(audit.RedactText(strings.TrimSpace(message)), syncalert.MaxMessageLength)
}

// reportSubmitFailed 上报 A1：提交到引擎被明确拒绝，或者找不到可用的引擎端点。参数校验、权限、409 等不经过这里。
// instance 可能还没落库（ID 为 0），这时信号不带作业 ID。
// reportSubmitFailed reports A1: the engine definitely rejected the submit or no engine endpoint is usable.
// Validation, permission and 409 errors never get here. instance may not be stored yet (ID 0); then the signal has
// no job ID.
func (s *Service) reportSubmitFailed(ctx context.Context, task *Task, instance *JobInstance, submitErr error) {
	if task == nil || instance == nil || submitErr == nil {
		return
	}
	job := *instance
	job.TaskID = task.ID
	if job.SubmitSpec == nil {
		job.SubmitSpec = JSONMap{"cluster_id": task.ClusterID}
	}
	if signal, ok := s.alertSignalForJob(ctx, syncalert.KindFire, syncalert.RuleSubmitFailed, &job, task, submitErr.Error()); ok {
		s.reportAlert(signal)
	}
}

// reportSubmitSucceeded 在实时作业提交成功时解除任务的 A1、A2、B1：实时作业以进入运行中为恢复；批作业等运行成功。
// reportSubmitSucceeded resolves the task's A1, A2 and B1 when a streaming job is submitted: streaming jobs recover
// on running, while batch jobs wait for success.
func (s *Service) reportSubmitSucceeded(ctx context.Context, task *Task, instance *JobInstance) {
	if instance == nil || instance.Status != JobStatusRunning {
		return
	}
	// 以作业实际模式为准；作业上取不到模式时才参考任务模式。
	// The job's actual mode wins; the task mode is only consulted when the job carries none.
	mode := jobRunMode(instance)
	if mode == "" && task != nil {
		mode = string(task.Mode)
	}
	if strings.EqualFold(mode, string(TaskModeBatch)) {
		return
	}
	if signal, ok := s.alertSignalForJob(ctx, syncalert.KindResolve, "", instance, task, ""); ok {
		signal.ResolveScope = syncalert.ResolveTaskRunHealthy
		s.reportAlert(signal)
	}
}

// reportJobTransition 在作业状态成功写入后上报：转为失败报 B1（提交过期已报 A2，不重复）；批作业成功、实时作业转为
// 运行中时解除 A1、A2、B1；实时作业到达终态时静默关闭 B4；越过停止截止时间的作业到达终态时解除它的 C1。
// reportJobTransition reports after a status write: turning failed reports B1 (expired submits already reported A2,
// so they are skipped); a batch success or a streaming job turning running resolves A1, A2 and B1; a streaming job
// reaching a terminal state silently closes B4; a job past its stop deadline reaching a terminal state resolves its C1.
func (s *Service) reportJobTransition(ctx context.Context, instance *JobInstance, previousStatus JobStatus) {
	if instance == nil || s.alertReporter == nil || instance.Status == previousStatus {
		return
	}
	switch {
	case instance.Status == JobStatusFailed:
		if strings.TrimSpace(stringValue(instance.SubmitSpec, submitOutcomeKey)) != submitOutcomeExpired {
			if signal, ok := s.alertSignalForJob(ctx, syncalert.KindFire, syncalert.RuleJobFailed, instance, nil, instance.ErrorMessage); ok {
				s.reportAlert(signal)
			}
		}
	case instance.Status == JobStatusSuccess && jobIsBatch(instance),
		instance.Status == JobStatusRunning && !jobIsBatch(instance):
		if signal, ok := s.alertSignalForJob(ctx, syncalert.KindResolve, "", instance, nil, ""); ok {
			signal.ResolveScope = syncalert.ResolveTaskRunHealthy
			s.reportAlert(signal)
		}
	}
	// 实时作业结束：B4 阶段静默关闭（job_ended）。 / A streaming job ended: close its B4 episode silently (job_ended).
	if isFinalNormalizedJobStatus(instance.Status) {
		s.reportCDCDelayJobEnded(ctx, instance)
	}
	if isFinalNormalizedJobStatus(instance.Status) && strings.TrimSpace(stringValue(instance.ResultPreview, cancelOverdueAtKey)) != "" {
		if signal, ok := s.alertSignalForJob(ctx, syncalert.KindResolve, "", instance, nil, ""); ok {
			signal.ResolveScope = syncalert.ResolveJobTerminal
			s.reportAlert(signal)
		}
	}
}

// reportSubmitExpired 上报 A2：提交结果一直未知，过了截止时间被判失败。
// reportSubmitExpired reports A2: an unknown submit outcome passed its deadline and was failed.
func (s *Service) reportSubmitExpired(ctx context.Context, instance *JobInstance) {
	if signal, ok := s.alertSignalForJob(ctx, syncalert.KindFire, syncalert.RuleSubmitTimeout, instance, nil, submitOutcomeExpiredMessage(instance)); ok {
		s.reportAlert(signal)
	}
}

// reportStopOverdue 上报 C1：停止请求过了截止时间作业仍未停下。
// reportStopOverdue reports C1: the job is still running after its stop deadline.
func (s *Service) reportStopOverdue(ctx context.Context, instance *JobInstance) {
	message := "job did not stop before the stop deadline " + stringValue(instance.ResultPreview, cancelDeadlineKey)
	if signal, ok := s.alertSignalForJob(ctx, syncalert.KindFire, syncalert.RuleStopTimeout, instance, nil, message); ok {
		s.reportAlert(signal)
	}
}

// reportClusterStatus 在集群对账结束后上报 D1：提前结束时发 Observe（Since 为这一段连续失败的开始）；
// 恢复或进程启动后第一次成功时发 Resolve，后者用来结束重启前遗留的告警阶段。
// reportClusterStatus reports D1 after a cluster reconcile: an early stop sends Observe (Since is when the current
// run of failures began); recovery, or the first success after process start, sends Resolve so episodes left over
// from before a restart are closed.
func (s *Service) reportClusterStatus(clusterID uint, outcome clusterReconcileOutcome, reason error) {
	if clusterID == 0 || s.alertReporter == nil {
		return
	}
	switch {
	case outcome.Aborted:
		message := ""
		if reason != nil {
			message = redactAlertMessage(reason.Error())
		}
		s.reportAlert(syncalert.Signal{
			Kind:      syncalert.KindObserve,
			RuleKey:   syncalert.RuleStatusUnavailable,
			ClusterID: clusterID,
			Message:   message,
			Since:     outcome.FirstFailureAt,
		})
	case outcome.Recovered || outcome.FirstSuccess:
		s.reportAlert(syncalert.Signal{
			Kind:         syncalert.KindResolve,
			RuleKey:      syncalert.RuleStatusUnavailable,
			ResolveScope: syncalert.ResolveClusterRule,
			ClusterID:    clusterID,
		})
	}
}

// reportEventChannelState 在事件通道状态变化时上报 D2：变为降级时 Fire，从降级恢复健康或被关闭时 Resolve。
// reportEventChannelState reports D2 when the event channel changes state: Fire when it turns degraded, Resolve when
// it leaves degraded for healthy or disabled.
func (s *Service) reportEventChannelState(clusterID uint, previous, current string) {
	if clusterID == 0 || previous == current || s.alertReporter == nil {
		return
	}
	switch {
	case current == EventChannelStateDegraded:
		s.reportAlert(syncalert.Signal{
			Kind:      syncalert.KindFire,
			RuleKey:   syncalert.RuleEventChannelDegraded,
			ClusterID: clusterID,
			Message:   "no job status event arrived within the expected window after a submit",
		})
	case previous == EventChannelStateDegraded:
		s.reportAlert(syncalert.Signal{
			Kind:         syncalert.KindResolve,
			RuleKey:      syncalert.RuleEventChannelDegraded,
			ResolveScope: syncalert.ResolveClusterRule,
			ClusterID:    clusterID,
		})
	}
}

// jobRunMode 返回作业实际的运行模式：优先取提交内容里引擎真正使用的 env.job.mode，其次是作业 / submit_spec 记录的
// 任务模式。任务元数据可能与内容不一致（如 v1 版本元数据为 batch、内容是 STREAMING），以内容为准；取不到返回空。
// jobRunMode returns the job's actual run mode: env.job.mode in the submitted content (what the engine uses) first,
// then the task mode recorded on the job / submit_spec. Task metadata can disagree with the content (e.g. a v1
// version recorded batch while its content is STREAMING), so the content wins; empty when unknown.
func jobRunMode(job *JobInstance) string {
	if job == nil {
		return ""
	}
	if match := jobModePattern.FindStringSubmatch(stringValue(job.SubmitSpec, "submitted_content")); len(match) == 2 {
		return strings.ToLower(match[1])
	}
	if mode := strings.TrimSpace(job.Mode); mode != "" {
		return mode
	}
	return strings.TrimSpace(submitSpecMode(job.SubmitSpec))
}

// jobIsBatch 判断作业是否为批作业；取不到模式时按实时作业处理（与任务默认模式一致）。
// jobIsBatch reports whether the job is a batch job; an unknown mode counts as streaming (the task default).
func jobIsBatch(job *JobInstance) bool {
	return strings.EqualFold(jobRunMode(job), string(TaskModeBatch))
}

// submitSpecMode 读取 submit_spec.mode：刚提交时是 TaskMode，从库里读出来是 string，两种都要认。
// submitSpecMode reads submit_spec.mode: it is a TaskMode right after submit and a string once loaded from the
// database, so both must be accepted.
func submitSpecMode(spec JSONMap) string {
	switch value := spec["mode"].(type) {
	case string:
		return value
	case TaskMode:
		return string(value)
	default:
		return ""
	}
}

// ResolveTaskNames 按 ID 批量查询任务名，供告警策略列表显示；实现 monitoring.SyncTaskNameResolver。
// ResolveTaskNames looks up task names by ID for the alert policy list; it implements monitoring.SyncTaskNameResolver.
func (s *Service) ResolveTaskNames(ctx context.Context, taskIDs []uint) map[uint]string {
	names := map[uint]string{}
	if s == nil || s.repo == nil {
		return names
	}
	for _, id := range taskIDs {
		if _, done := names[id]; done || id == 0 {
			continue
		}
		if task, err := s.repo.GetTaskByID(ctx, id); err == nil && task != nil {
			names[id] = strings.TrimSpace(task.Name)
		}
	}
	return names
}

// updateEventChannelStatsWithAlert 更新事件通道统计；写入预期字段（错过或满足）时比较前后状态并上报 D2。
// updateEventChannelStatsWithAlert updates event channel stats; when an expectation field (missed or satisfied) is
// written it compares the state before and after and reports D2.
func (s *Service) updateEventChannelStatsWithAlert(ctx context.Context, clusterID uint, counters map[string]int64, fields map[string]any) error {
	_, missed := fields["expect_missed_at"]
	_, satisfied := fields["expect_satisfied_at"]
	if s.alertReporter == nil || (!missed && !satisfied) {
		return s.repo.UpdateEngineEventChannelStats(ctx, clusterID, counters, fields)
	}
	before, _ := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err := s.repo.UpdateEngineEventChannelStats(ctx, clusterID, counters, fields); err != nil {
		return err
	}
	after, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil {
		return nil
	}
	s.reportEventChannelState(clusterID, eventChannelState(before), eventChannelState(after))
	return nil
}
