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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// homeSummaryFailedLimit / homeBackpressureThreshold 控制首页「需要处理」里的作业类条目。
// homeSummaryFailedLimit / homeBackpressureThreshold bound the job items in the home "needs attention" strip.
const (
	homeSummaryFailedLimit    = 5
	homeBackpressureThreshold = 0.7
	homeTrendStepSeconds      = 60
)

// HomeAttentionItem 是「需要处理」条里的一项；Viewable=false 时前端只展示、不可点开。
// HomeAttentionItem is one entry of the "needs attention" strip; Viewable=false means display-only in the UI.
type HomeAttentionItem struct {
	Kind     string    `json:"kind"`     // job_failed | cdc_delay | backpressure | alert
	Severity string    `json:"severity"` // critical | warning
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	Value    *float64  `json:"value,omitempty"`
	JobID    uint      `json:"job_id,omitempty"`
	TaskID   uint      `json:"task_id,omitempty"`
	AlertID  string    `json:"alert_id,omitempty"`
	At       time.Time `json:"at"`
	Viewable bool      `json:"viewable"`
}

// HomeJobSummary 是首页作业类汇总：计数与速率为全局口径（所有登录用户一致），趋势来自 Prometheus。
// HomeJobSummary is the home page job summary: counts and rates are global (same for every signed-in user) and the
// trends come from Prometheus.
type HomeJobSummary struct {
	Running          int64 `json:"running"`
	RunningStreaming int64 `json:"running_streaming"`
	RunningBatch     int64 `json:"running_batch"`
	Failed24h        int64 `json:"failed_24h"`
	FailedPrev24h    int64 `json:"failed_prev_24h"`

	WriteQPS float64 `json:"write_qps"`
	ReadQPS  float64 `json:"read_qps"`

	MaxCDCDelaySeconds *float64 `json:"max_cdc_delay_seconds,omitempty"`
	MaxCDCJobID        uint     `json:"max_cdc_job_id,omitempty"`
	MaxCDCTaskID       uint     `json:"max_cdc_task_id,omitempty"`
	MaxCDCJobName      string   `json:"max_cdc_job_name,omitempty"`
	CDCThresholdSecond int      `json:"cdc_threshold_seconds"`

	TrendSource string              `json:"trend_source"`
	TrendReason string              `json:"trend_reason,omitempty"`
	WriteTrend  [][2]float64        `json:"write_trend"`
	CDCTrend    [][2]float64        `json:"cdc_trend"`
	Attention   []HomeAttentionItem `json:"attention"`
}

// GetHomeJobSummary 汇总首页作业指标。clusterID=0 表示全部集群；预览运行不计入。
// GetHomeJobSummary aggregates the home page job metrics. clusterID=0 means all clusters; preview runs are excluded.
func (s *Service) GetHomeJobSummary(ctx context.Context, actor executionapp.Actor, clusterID uint, now time.Time) (*HomeJobSummary, error) {
	out := &HomeJobSummary{
		CDCThresholdSecond: syncalert.DefaultCDCDelayThresholdSeconds,
		WriteTrend:         [][2]float64{},
		CDCTrend:           [][2]float64{},
		Attention:          []HomeAttentionItem{},
	}
	base := JobFilter{IncludeAll: true, ClusterID: clusterID, ExcludeRunTypes: []RunType{RunTypePreview}}

	runningStatuses, _ := ResolveJobStatusGroups(JobStatusGroupRunning)
	running := base
	running.Statuses, running.Size, running.Sort = runningStatuses, jobListInMemorySortLimit, jobListSortStartedAt
	runningJobs, total, err := s.ListJobs(ctx, &running)
	if err != nil {
		return nil, err
	}
	out.Running = total
	var cdcJob *JobInstance
	for _, job := range runningJobs {
		if effectiveJobMode(job) == string(TaskModeBatch) {
			out.RunningBatch++
		} else {
			out.RunningStreaming++
		}
		metrics := jobMetrics(job)
		if v := metricNumber(metrics, "SinkWriteQPS"); v != nil {
			out.WriteQPS += *v
		}
		if v := metricNumber(metrics, "SourceReceivedQPS"); v != nil {
			out.ReadQPS += *v
		}
		if job.CDCDelay != nil && job.CDCDelay.Fresh && (cdcJob == nil || job.CDCDelay.LatestMs > cdcJob.CDCDelay.LatestMs) {
			cdcJob = job
		}
	}

	failedStatuses, _ := ResolveJobStatusGroups(JobStatusGroupFailed)
	countFailed := func(from, to time.Time) (int64, error) {
		f := base
		f.Statuses, f.FinishedAfter, f.FinishedBefore = failedStatuses, &from, &to
		f.Size = 1
		_, n, err := s.ListJobs(ctx, &f)
		return n, err
	}
	if out.Failed24h, err = countFailed(now.Add(-24*time.Hour), now.Add(time.Minute)); err != nil {
		return nil, err
	}
	if out.FailedPrev24h, err = countFailed(now.Add(-48*time.Hour), now.Add(-24*time.Hour)); err != nil {
		return nil, err
	}
	recentFailed := base
	from := now.Add(-24 * time.Hour)
	recentFailed.Statuses, recentFailed.FinishedAfter = failedStatuses, &from
	recentFailed.Size, recentFailed.Sort = homeSummaryFailedLimit, jobListSortStartedAt
	failedJobs, _, err := s.ListJobs(ctx, &recentFailed)
	if err != nil {
		return nil, err
	}

	// 可见范围只决定条目能否点开，不影响计数（全局口径）。
	// Visibility only decides whether an item can be opened; counts stay global.
	scope := &JobFilter{}
	if err := s.scopeJobFilterForActor(ctx, actor, scope); err != nil {
		return nil, err
	}
	viewable := jobViewableFunc(actor, scope)

	nameJobs := append([]*JobInstance{}, failedJobs...)
	if cdcJob != nil {
		nameJobs = append(nameJobs, cdcJob)
	}
	if err := s.decorateJobsForList(ctx, nameJobs); err != nil {
		return nil, err
	}
	for _, job := range failedJobs {
		at := job.UpdatedAt
		if job.FinishedAt != nil {
			at = *job.FinishedAt
		}
		out.Attention = append(out.Attention, HomeAttentionItem{
			Kind: "job_failed", Severity: "critical", Title: jobDisplayName(job), Detail: firstLine(job.ErrorMessage),
			JobID: job.ID, TaskID: job.TaskID, At: at, Viewable: viewable(job),
		})
	}
	if cdcJob != nil {
		seconds := float64(cdcJob.CDCDelay.LatestMs) / 1000
		out.MaxCDCDelaySeconds = &seconds
		out.MaxCDCJobID, out.MaxCDCTaskID, out.MaxCDCJobName = cdcJob.ID, cdcJob.TaskID, jobDisplayName(cdcJob)
		if seconds >= float64(out.CDCThresholdSecond) {
			value := seconds
			out.Attention = append(out.Attention, HomeAttentionItem{
				Kind: "cdc_delay", Severity: "warning", Title: jobDisplayName(cdcJob), Value: &value,
				JobID: cdcJob.ID, TaskID: cdcJob.TaskID, At: cdcJob.CDCDelay.At, Viewable: viewable(cdcJob),
			})
		}
	}

	s.fillHomeTrends(ctx, out, clusterID, now, runningJobs, viewable)
	sort.SliceStable(out.Attention, func(i, j int) bool {
		if out.Attention[i].Severity != out.Attention[j].Severity {
			return out.Attention[i].Severity == "critical"
		}
		return out.Attention[i].At.After(out.Attention[j].At)
	})
	return out, nil
}

// fillHomeTrends 从 Prometheus 取近 1 小时写入速率与最大 CDC 延迟趋势，并补高背压条目；不可用时只标明原因。
// fillHomeTrends loads the last hour of write rate and max CDC delay from Prometheus and adds high-backpressure items;
// when Prometheus is unavailable it only records why.
func (s *Service) fillHomeTrends(ctx context.Context, out *HomeJobSummary, clusterID uint, now time.Time, running []*JobInstance, viewable func(*JobInstance) bool) {
	if reason := promquery.UnavailableReason(); reason != "" {
		out.TrendSource, out.TrendReason = promquery.SourceNone, reason
		return
	}
	out.TrendSource = promquery.SourcePrometheus
	selector := ""
	if clusterID > 0 {
		selector = fmt.Sprintf(`cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	start := now.Add(-time.Hour)
	if series, err := promquery.Range(ctx, fmt.Sprintf(`sum(rate(stx_job_sink_write_rows_total{%s}[1m]))`, selector), start, now, homeTrendStepSeconds); err == nil && len(series) > 0 {
		out.WriteTrend = series[0].Points
	} else if err != nil {
		out.TrendReason = "prometheus_error"
	}
	if series, err := promquery.Range(ctx, fmt.Sprintf(`max(stx_job_cdc_delay_seconds{%s})`, selector), start, now, homeTrendStepSeconds); err == nil && len(series) > 0 {
		out.CDCTrend = series[0].Points
	}
	samples, err := promquery.Instant(ctx, fmt.Sprintf(`topk(3, max by (job_id, edge) (stx_job_edge_backpressure_ratio{%s}) > %g)`, selector, homeBackpressureThreshold), now)
	if err != nil || len(samples) == 0 {
		return
	}
	byID := map[string]*JobInstance{}
	for _, job := range running {
		byID[strconv.FormatUint(uint64(job.ID), 10)] = job
	}
	_ = s.decorateJobsForList(ctx, running) // 只用于取任务名；失败时条目退回显示作业 ID。/ Names only; items fall back to the job ID.
	for _, sample := range samples {
		job := byID[sample.Metric["job_id"]]
		if job == nil {
			continue
		}
		value := sample.Value
		out.Attention = append(out.Attention, HomeAttentionItem{
			Kind: "backpressure", Severity: "warning", Title: jobDisplayName(job), Detail: sample.Metric["edge"], Value: &value,
			JobID: job.ID, TaskID: job.TaskID, At: time.UnixMilli(sample.TsMs).UTC(), Viewable: viewable(job),
		})
	}
}

// jobViewableFunc 返回判定函数：管理员全可见；否则自己创建的或可查看任务下的作业可见。
// jobViewableFunc returns a predicate: admins see all; others see their own jobs and jobs under viewable tasks.
func jobViewableFunc(actor executionapp.Actor, scope *JobFilter) func(*JobInstance) bool {
	if actor.IsAdmin {
		return func(*JobInstance) bool { return true }
	}
	tasks := make(map[uint]struct{}, len(scope.VisibleTaskIDs))
	for _, id := range scope.VisibleTaskIDs {
		tasks[id] = struct{}{}
	}
	return func(job *JobInstance) bool {
		if job == nil {
			return false
		}
		if job.CreatedBy == uint(actor.UserID) {
			return true
		}
		_, ok := tasks[job.TaskID]
		return ok
	}
}

// jobDisplayName 优先用提交时的作业名，其次任务名，最后用作业 ID。
// jobDisplayName prefers the submitted job name, then the task name, then the job ID.
func jobDisplayName(job *JobInstance) string {
	if name := strings.TrimSpace(stringValue(job.SubmitSpec, "job_name")); name != "" {
		return name
	}
	if job.TaskName != "" {
		return job.TaskName
	}
	return "#" + strconv.FormatUint(uint64(job.ID), 10)
}

// firstLine 取错误信息首行（去敏后），供条目副标题展示。
// firstLine returns the first line of an error (redacted) for an item's subtitle.
func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if runes := []rune(text); len(runes) > 200 {
		text = string(runes[:200])
	}
	return audit.RedactText(text)
}

// RunningJobCountsByCluster 供首页集群卡片显示每个集群的运行中作业数（全局口径）。
// RunningJobCountsByCluster feeds the per-cluster running job count on the home cluster cards (global scope).
func (s *Service) RunningJobCountsByCluster(ctx context.Context) (map[uint]int64, error) {
	return s.repo.CountRunningJobsByCluster(ctx)
}
