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
	"regexp"
	"sort"
	"strings"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// 全局作业列表的排序键。空值表示旧行为（created_at DESC）。
// Sort keys of the global job list. Empty means the legacy order (created_at DESC).
const (
	jobListSortAttention  = "attention"
	jobListSortStartedAt  = "started_at"
	jobListSortCreatedAt  = "created_at"
	jobListSortFinishedAt = "finished_at"
	jobListSortDuration   = "duration"
	jobListSortRate       = "rate"
	jobListSortCDCDelay   = "cdc_delay"
)

// jobListInMemorySortLimit 是内存排序（rate / cdc_delay）最多处理的运行中作业数。
// jobListInMemorySortLimit caps how many running jobs an in-memory sort (rate / cdc_delay) handles.
const jobListInMemorySortLimit = 2000

// 状态组：前端胶囊与 CLI 用组名筛选，组到原始状态的映射只在这里维护。
// Status groups: the UI pills and CLI filter by group name; the group-to-status mapping lives only here.
const (
	JobStatusGroupRunning  = "running"
	JobStatusGroupFailed   = "failed"
	JobStatusGroupPending  = "pending"
	JobStatusGroupStopping = "stopping"
	JobStatusGroupFinished = "finished"
	JobStatusGroupCanceled = "canceled"
)

// jobStatusGroups 按「需要关注」优先级排列：失败 > 运行中 > 等待 > 停止中 > 已取消 > 已完成。
// jobStatusGroups is ordered by attention: failed > running > pending > stopping > canceled > finished.
var jobStatusGroups = []struct {
	Name     string
	Statuses []JobStatus
}{
	{JobStatusGroupFailed, []JobStatus{JobStatusFailed, JobStatusFailing}},
	{JobStatusGroupRunning, []JobStatus{JobStatusRunning, JobStatusDoingSavepoint}},
	{JobStatusGroupPending, []JobStatus{JobStatusPending}},
	{JobStatusGroupStopping, []JobStatus{JobStatusCancelRequested, JobStatusCancelling}},
	{JobStatusGroupCanceled, []JobStatus{JobStatusCanceled}},
	{JobStatusGroupFinished, []JobStatus{JobStatusSuccess, JobStatusSavepointDone}},
}

// JobLiveMetrics 是列表行的当前运行值，来自 10 秒对账写入的 result_preview.metrics 与事件运行时，不访问引擎。
// JobLiveMetrics holds a list row's current values from the 10s reconcile (result_preview.metrics) and the event
// runtime; it never calls the engine.
type JobLiveMetrics struct {
	ReadQPS         *float64 `json:"read_qps,omitempty"`
	WriteQPS        *float64 `json:"write_qps,omitempty"`
	ReadTotal       *float64 `json:"read_total,omitempty"`
	WriteTotal      *float64 `json:"write_total,omitempty"`
	CDCDelaySeconds *float64 `json:"cdc_delay_seconds,omitempty"`
	// TableCount 是按逻辑表去重后的表数量（Source[0].t 与 Sink[0].t 算同一张），>1 时前端显示多表徽章。
	// TableCount is the deduplicated logical table count (Source[0].t and Sink[0].t count as one); the frontend
	// shows the multi-table badge when it is greater than one.
	TableCount *int `json:"table_count,omitempty"`
}

// JobListOptions 是 ListJobsForActor 之外的列表装饰开关。
// JobListOptions are list decoration switches on top of ListJobsForActor.
type JobListOptions struct {
	WithCounts bool
	WithLive   bool
}

// ResolveJobStatusGroups 把逗号分隔的状态组展开成原始状态；未知组名返回 ErrInvalidJobListQuery。
// ResolveJobStatusGroups expands comma-separated group names into raw statuses; unknown names return
// ErrInvalidJobListQuery.
func ResolveJobStatusGroups(csv string) ([]JobStatus, error) {
	var out []JobStatus
	for _, raw := range strings.Split(csv, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		found := false
		for _, group := range jobStatusGroups {
			if group.Name == name {
				out = append(out, group.Statuses...)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: unknown status_group %q", ErrInvalidJobListQuery, name)
		}
	}
	return out, nil
}

// jobStatusGroupOf 返回原始状态所属的组名；不在任何组里时返回原状态。
// jobStatusGroupOf returns the group name of a raw status, or the status itself when ungrouped.
func jobStatusGroupOf(status JobStatus) string {
	for _, group := range jobStatusGroups {
		for _, item := range group.Statuses {
			if item == status {
				return group.Name
			}
		}
	}
	return string(status)
}

// jobAttentionOrderExpr 生成「需要关注优先」的 CASE 排序表达式，三库通用。
// jobAttentionOrderExpr builds the attention-first CASE expression, portable across the three databases.
func jobAttentionOrderExpr() string {
	var b strings.Builder
	b.WriteString("CASE status")
	for rank, group := range jobStatusGroups {
		for _, status := range group.Statuses {
			fmt.Fprintf(&b, " WHEN '%s' THEN %d", status, rank)
		}
	}
	fmt.Fprintf(&b, " ELSE %d END", len(jobStatusGroups))
	return b.String()
}

// normalizeJobListFilter 校验排序与模式参数；内存排序只允许在「运行中」组内使用。
// normalizeJobListFilter validates sort and mode; in-memory sorts are allowed only within the running group.
func normalizeJobListFilter(filter *JobFilter) error {
	filter.Sort = strings.ToLower(strings.TrimSpace(filter.Sort))
	filter.Order = strings.ToLower(strings.TrimSpace(filter.Order))
	if filter.Order != "" && filter.Order != "asc" && filter.Order != "desc" {
		return fmt.Errorf("%w: order must be asc or desc", ErrInvalidJobListQuery)
	}
	if filter.Mode != "" {
		mode := normalizeJobMode(filter.Mode)
		if mode == "" {
			return fmt.Errorf("%w: mode must be streaming or batch", ErrInvalidJobListQuery)
		}
		filter.Mode = mode
	}
	switch source := JobSource(strings.ToLower(strings.TrimSpace(string(filter.Source)))); source {
	case "", JobSourceSTX, JobSourceExternal:
		filter.Source = source
	default:
		return fmt.Errorf("%w: source must be stx or external", ErrInvalidJobListQuery)
	}
	switch filter.Sort {
	case "", jobListSortAttention, jobListSortStartedAt, jobListSortCreatedAt, jobListSortFinishedAt, jobListSortDuration:
		return nil
	case jobListSortRate, jobListSortCDCDelay:
		if !onlyRunningStatuses(filter.Statuses) {
			return fmt.Errorf("%w: sort %s requires status_group=running", ErrInvalidJobListQuery, filter.Sort)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported sort %q", ErrInvalidJobListQuery, filter.Sort)
	}
}

// onlyRunningStatuses 判断状态筛选非空且全部属于「运行中」组。
// onlyRunningStatuses reports whether the status filter is non-empty and entirely in the running group.
func onlyRunningStatuses(statuses []JobStatus) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, status := range statuses {
		if jobStatusGroupOf(status) != JobStatusGroupRunning {
			return false
		}
	}
	return true
}

// ListJobsWithOptions 是全局作业列表入口：可见范围与 GetJob 一致，按需补状态计数、任务名与当前值。
// ListJobsWithOptions is the global job list entry: visibility matches GetJob, and status counts, task names and
// live values are added on demand.
func (s *Service) ListJobsWithOptions(ctx context.Context, actor executionapp.Actor, filter *JobFilter, opts JobListOptions) (*JobListData, error) {
	if filter == nil {
		filter = &JobFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	if err := normalizeJobListFilter(filter); err != nil {
		return nil, err
	}
	if err := s.scopeJobFilterForActor(ctx, actor, filter); err != nil {
		return nil, err
	}

	var (
		jobs  []*JobInstance
		total int64
		err   error
	)
	if filter.Sort == jobListSortRate || filter.Sort == jobListSortCDCDelay {
		jobs, total, err = s.listJobsSortedInMemory(ctx, filter)
	} else {
		jobs, total, err = s.ListJobs(ctx, filter)
	}
	if err != nil {
		return nil, err
	}

	out := &JobListData{Total: total, Items: jobs}
	if opts.WithCounts {
		counts, err := s.repo.CountJobInstancesByStatus(ctx, filter)
		if err != nil {
			return nil, err
		}
		out.StatusCounts = map[string]int64{}
		for _, group := range jobStatusGroups {
			out.StatusCounts[group.Name] = 0
		}
		for status, count := range counts {
			out.StatusCounts[jobStatusGroupOf(status)] += count
		}
	}
	if opts.WithLive {
		if err := s.decorateJobsForList(ctx, jobs); err != nil {
			return nil, err
		}
	}
	// 列表行统一瘦身（放在 live 组装之后）：完整 result_preview / submitted_content 只走 getJob 详情返回。
	// Slim every list row (after live assembly): the full result_preview / submitted_content only ships via
	// the getJob detail.
	for _, job := range jobs {
		slimJobListRow(job)
	}
	s.decorateJobPermissions(ctx, actor, jobs)
	return out, nil
}

// scopeJobFilterForActor 设置可见范围：管理员看全部；有 task_id 且可查看该任务时看该任务全部作业；
// 其余情况看自己创建的作业与可查看任务下的作业。
// scopeJobFilterForActor sets visibility: admins see everything; with a viewable task_id, all jobs of that task;
// otherwise own jobs plus jobs under viewable tasks.
func (s *Service) scopeJobFilterForActor(ctx context.Context, actor executionapp.Actor, filter *JobFilter) error {
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	if actor.IsAdmin {
		return nil
	}
	if filter.TaskID > 0 {
		task, err := s.repo.GetTaskByID(ctx, filter.TaskID)
		if err == nil && task.CanUserView(uint(actor.UserID), false) {
			filter.IncludeAll = true
		}
		return nil
	}
	tasks, err := s.repo.ListAllTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.NodeType == TaskNodeTypeFile && task.CanUserView(uint(actor.UserID), false) {
			filter.VisibleTaskIDs = append(filter.VisibleTaskIDs, task.ID)
		}
	}
	return nil
}

// listJobsSortedInMemory 取全部匹配的运行中作业，按当前速率或 CDC 延迟排序后再分页。
// listJobsSortedInMemory loads all matching running jobs, sorts by live rate or CDC delay, then paginates.
func (s *Service) listJobsSortedInMemory(ctx context.Context, filter *JobFilter) ([]*JobInstance, int64, error) {
	all := *filter
	all.Page, all.Size, all.Sort = 1, jobListInMemorySortLimit, jobListSortAttention
	jobs, total, err := s.ListJobs(ctx, &all)
	if err != nil {
		return nil, 0, err
	}
	asc := filter.Order == "asc"
	value := func(job *JobInstance) float64 {
		if filter.Sort == jobListSortCDCDelay {
			if job.CDCDelay != nil && job.CDCDelay.Fresh {
				return float64(job.CDCDelay.LatestMs)
			}
			return -1
		}
		if v := metricNumber(jobMetrics(job), "SinkWriteQPS"); v != nil {
			return *v
		}
		return -1
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if asc {
			return value(jobs[i]) < value(jobs[j])
		}
		return value(jobs[i]) > value(jobs[j])
	})
	size := filter.Size
	if size <= 0 {
		return jobs, total, nil
	}
	start := 0
	if filter.Page > 1 {
		start = (filter.Page - 1) * size
	}
	if start >= len(jobs) {
		return []*JobInstance{}, total, nil
	}
	end := start + size
	if end > len(jobs) {
		end = len(jobs)
	}
	return jobs[start:end], total, nil
}

// decorateJobsForList 批量补任务名与当前值（一次任务名查询，不逐行访问引擎）。
// decorateJobsForList adds task names and live values in bulk (one name query, no per-row engine calls).
func (s *Service) decorateJobsForList(ctx context.Context, jobs []*JobInstance) error {
	ids := make([]uint, 0, len(jobs))
	seen := map[uint]struct{}{}
	for _, job := range jobs {
		if _, ok := seen[job.TaskID]; ok || job.TaskID == 0 {
			continue
		}
		seen[job.TaskID] = struct{}{}
		ids = append(ids, job.TaskID)
	}
	names, err := s.repo.GetTaskNamesByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		job.TaskName = names[job.TaskID]
		job.Live = buildJobLiveMetrics(job)
	}
	return nil
}

// slimJobListRow 把列表行裁剪到行展示消费的字段：result_preview 只留白名单键
// （job_status 显示态、cancel_outcome/cancel_timeout_seconds 结果提示），删掉 submit_spec.submitted_content
// （整段提交脚本，含脱敏密码）；live 已压平行需要的指标，详情由前端单独 getJob 取全量。
// slimJobListRow trims a list row to what the row displays: result_preview keeps only allow-listed keys
// (job_status display state, cancel_outcome/cancel_timeout_seconds outcome hints) and drops
// submit_spec.submitted_content (the whole submitted script incl. masked secrets); `live` already flattens
// the metrics the row needs, and the detail view fetches the full record via getJob.
func slimJobListRow(job *JobInstance) {
	if job == nil {
		return
	}
	if job.ResultPreview != nil {
		job.ResultPreview = pruneResultPreviewForList(job.ResultPreview)
	}
	if job.SubmitSpec != nil {
		delete(job.SubmitSpec, "submitted_content")
	}
}

// jobListPreviewKeepKeys 是列表行还消费的 result_preview 键（前端 JobOutcomeHints / 显示状态判定）。
// jobListPreviewKeepKeys are the result_preview keys list rows still consume (JobOutcomeHints / display status).
var jobListPreviewKeepKeys = []string{"job_status", "cancel_outcome", "cancel_timeout_seconds"}

// pruneResultPreviewForList 返回只含白名单键的精简 result_preview。
// pruneResultPreviewForList returns a reduced result_preview holding only allow-listed keys.
func pruneResultPreviewForList(preview JSONMap) JSONMap {
	keep := JSONMap{}
	for _, key := range jobListPreviewKeepKeys {
		if v, ok := preview[key]; ok {
			keep[key] = v
		}
	}
	return keep
}

// jobMetrics 取 result_preview.metrics（对账写入的引擎指标快照）。
// jobMetrics returns result_preview.metrics (the engine metric snapshot written by the reconciler).
func jobMetrics(job *JobInstance) map[string]interface{} {
	if job == nil || job.ResultPreview == nil {
		return nil
	}
	metrics, _ := job.ResultPreview["metrics"].(map[string]interface{})
	return metrics
}

// buildJobLiveMetrics 组装列表行当前值；速率只对运行中作业有意义，其余状态只给累计值。
// 回退顺序与前端 extractJobMetricSummary（sync-studio-utils.ts）逐项一致：
// 写侧先 SinkWrite* 再 SinkCommitted*，全局值缺失时汇总表级 Table* 映射。
// buildJobLiveMetrics assembles a row's live values; rates only make sense while running, other states get totals.
// The fallback order mirrors the frontend extractJobMetricSummary (sync-studio-utils.ts) one to one:
// SinkWrite* before SinkCommitted* on the write side, and Table* maps are summed when the global value is absent.
func buildJobLiveMetrics(job *JobInstance) *JobLiveMetrics {
	metrics := jobMetrics(job)
	live := &JobLiveMetrics{
		ReadTotal:  firstMetricNumber(metrics, "SourceReceivedCount"),
		WriteTotal: firstMetricNumber(metrics, "SinkWriteCount", "SinkCommittedCount"),
	}
	if live.ReadTotal == nil {
		live.ReadTotal = tableMetricSum(metrics, "TableSourceReceivedCount")
	}
	if live.WriteTotal == nil {
		live.WriteTotal = tableMetricSum(metrics, "TableSinkWriteCount", "TableSinkCommittedCount")
	}
	if n := logicalTableCount(metrics); n > 0 {
		live.TableCount = &n
	}
	if jobStatusGroupOf(job.Status) == JobStatusGroupRunning {
		live.ReadQPS = firstMetricNumber(metrics, "SourceReceivedQPS")
		if live.ReadQPS == nil {
			live.ReadQPS = tableMetricSum(metrics, "TableSourceReceivedQPS")
		}
		live.WriteQPS = firstMetricNumber(metrics, "SinkWriteQPS", "SinkCommittedQPS")
		if live.WriteQPS == nil {
			live.WriteQPS = tableMetricSum(metrics, "TableSinkWriteQPS")
		}
		if job.CDCDelay != nil && job.CDCDelay.Fresh {
			seconds := float64(job.CDCDelay.LatestMs) / 1000
			live.CDCDelaySeconds = &seconds
		}
	}
	if live.ReadTotal == nil && live.WriteTotal == nil && live.ReadQPS == nil && live.WriteQPS == nil && live.CDCDelaySeconds == nil {
		return nil
	}
	return live
}

// firstMetricNumber 按顺序取第一个存在的全局指标值。
// firstMetricNumber returns the first present global metric value in order.
func firstMetricNumber(metrics map[string]interface{}, keys ...string) *float64 {
	for _, key := range keys {
		if v := metricNumber(metrics, key); v != nil {
			return v
		}
	}
	return nil
}

// tableMetricSum 按顺序取第一个非空表级映射并对其值求和；映射为空或不存在时返回 nil。
// tableMetricSum picks the first non-empty table-level map in order and sums its values; nil when none exist.
func tableMetricSum(metrics map[string]interface{}, mapKeys ...string) *float64 {
	for _, mapKey := range mapKeys {
		raw, ok := metrics[mapKey]
		if !ok || raw == nil {
			continue
		}
		var sum float64
		var count int
		switch typed := raw.(type) {
		case map[string]interface{}:
			for _, v := range typed {
				if n := coerceMetricNumber(v); n != nil {
					sum += *n
					count++
				}
			}
		case map[string]float64:
			for _, v := range typed {
				sum += v
				count++
			}
		}
		if count > 0 {
			return &sum
		}
	}
	return nil
}

// logicalTableCount 复刻前端 countLogicalTables：收集表级指标键，剥掉 Source[N]. / Sink[N]. 前缀取末段表名，
// 转小写并去掉 archive_ 前缀后去重。
// logicalTableCount mirrors the frontend countLogicalTables: it collects table-level metric keys, strips the
// Source[N]. / Sink[N]. endpoint prefix, keeps the leaf table name lowercased without an archive_ prefix, and dedups.
func logicalTableCount(metrics map[string]interface{}) int {
	if metrics == nil {
		return 0
	}
	keys := map[string]struct{}{}
	for _, raw := range collectTableMetricKeys(metrics) {
		if logical := normalizeLogicalTableKey(raw); logical != "" {
			keys[logical] = struct{}{}
		}
	}
	return len(keys)
}

// metricTablePathRe 匹配表级指标键的端点前缀（Source[0].db.t → db.t）。
// metricTablePathRe matches the endpoint prefix of a table-level metric key (Source[0].db.t → db.t).
var metricTablePathRe = regexp.MustCompile(`(?i)^(Source|Sink)\[\d+\]\.`)

// normalizeLogicalTableKey 等价于前端 normalizePairingTableKey + extractMetricTablePath。
// normalizeLogicalTableKey is the Go equivalent of the frontend normalizePairingTableKey + extractMetricTablePath.
func normalizeLogicalTableKey(metricKey string) string {
	path := metricTablePathRe.ReplaceAllString(strings.TrimSpace(metricKey), "")
	leaf := path
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		leaf = path[idx+1:]
	}
	leaf = strings.ToLower(strings.TrimSpace(leaf))
	return strings.TrimPrefix(leaf, "archive_")
}

// queryFlag 读取布尔查询参数：1 / true / yes 为真。
// queryFlag reads a boolean query parameter: 1 / true / yes count as true.
func queryFlag(c interface{ Query(string) string }, name string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query(name))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// firstQuery 返回第一个非空的查询参数值，用于兼容同义参数名。
// firstQuery returns the first non-empty query value, for synonymous parameter names.
func firstQuery(c interface{ Query(string) string }, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.Query(name)); value != "" {
			return value
		}
	}
	return ""
}
