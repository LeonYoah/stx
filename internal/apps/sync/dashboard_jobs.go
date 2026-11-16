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
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// dashboardAnnotationLimit 是作业大盘每类事件标注的上限。
// dashboardAnnotationLimit caps each kind of job dashboard annotation.
const dashboardAnnotationLimit = 50

// vertexPluginPattern 从引擎顶点名里取插件名，如「pipeline-1 [Source[0]-MySQL-CDC]」→ Source / MySQL-CDC。
// vertexPluginPattern pulls the plugin out of an engine vertex name, e.g. "pipeline-1 [Source[0]-MySQL-CDC]".
var vertexPluginPattern = regexp.MustCompile(`\[(Source|Sink|Transform)\[\d+\]-([^\]]+)\]`)

// JobBrief 是作业大盘条目需要的作业摘要。
// JobBrief is the job summary job dashboard entries need.
type JobBrief struct {
	ID        uint     `json:"job_id"`
	TaskID    uint     `json:"task_id"`
	ClusterID uint     `json:"cluster_id"`
	Name      string   `json:"name"`
	Sources   []string `json:"sources,omitempty"`
	Sinks     []string `json:"sinks,omitempty"`
	Viewable  bool     `json:"viewable"`
}

// JobCalendar 是运行日历：Runs / Failed 为「周 × 7 天」，每列周一到周日，最后一列包含 EndDate（显示时区的日期）。
// JobCalendar is the run calendar: Runs / Failed are weeks × 7 days, each column Monday to Sunday, and the last column
// contains EndDate (a date in the display timezone).
type JobCalendar struct {
	Weeks   int     `json:"weeks"`
	EndDate string  `json:"end_date"`
	Runs    [][]int `json:"runs"`
	Failed  [][]int `json:"failed"`
}

// JobAnnotation 是吞吐图上的事件标注。
// JobAnnotation is an event marker on the throughput chart.
type JobAnnotation struct {
	TsMs     int64  `json:"ts_ms"`
	Kind     string `json:"kind"` // job_failed | job_recovered | schema_change
	Title    string `json:"title"`
	JobID    uint   `json:"job_id,omitempty"`
	TaskID   uint   `json:"task_id,omitempty"`
	Viewable bool   `json:"viewable"`
}

// parseJobPlugins 从 result_preview.job_dag.vertexInfoMap 解析源 / 目标插件名（去掉 -MultiTableSink 后缀并去重）。
// parseJobPlugins reads source / sink plugin names from result_preview.job_dag.vertexInfoMap (dropping the
// -MultiTableSink suffix and de-duplicating).
func parseJobPlugins(job *JobInstance) (sources, sinks []string) {
	if job == nil || job.ResultPreview == nil {
		return nil, nil
	}
	dag, _ := job.ResultPreview["job_dag"].(map[string]interface{})
	vertices, _ := dag["vertexInfoMap"].([]interface{})
	seen := map[string]bool{}
	for _, raw := range vertices {
		vertex, _ := raw.(map[string]interface{})
		name, _ := vertex["vertexName"].(string)
		match := vertexPluginPattern.FindStringSubmatch(name)
		if len(match) != 3 {
			continue
		}
		plugin := strings.TrimSuffix(strings.TrimSpace(match[2]), "-MultiTableSink")
		key := match[1] + "/" + plugin
		if plugin == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch match[1] {
		case "Source":
			sources = append(sources, plugin)
		case "Sink":
			sinks = append(sinks, plugin)
		}
	}
	return sources, sinks
}

// DescribeJobsForDashboard 批量返回作业摘要；不存在的 ID 不出现。viewable 按当前用户的作业可见范围判定。
// DescribeJobsForDashboard returns job summaries in bulk; unknown IDs are absent. viewable follows the caller's job
// visibility scope.
func (s *Service) DescribeJobsForDashboard(ctx context.Context, actor executionapp.Actor, ids []uint) (map[uint]JobBrief, error) {
	out := map[uint]JobBrief{}
	if len(ids) == 0 {
		return out, nil
	}
	jobs, _, err := s.ListJobs(ctx, &JobFilter{IncludeAll: true, IDs: ids, Size: len(ids)})
	if err != nil {
		return nil, err
	}
	if err := s.decorateJobsForList(ctx, jobs); err != nil {
		return nil, err
	}
	scope := &JobFilter{}
	if err := s.scopeJobFilterForActor(ctx, actor, scope); err != nil {
		return nil, err
	}
	viewable := jobViewableFunc(actor, scope)
	for _, job := range jobs {
		sources, sinks := parseJobPlugins(job)
		out[job.ID] = JobBrief{
			ID: job.ID, TaskID: job.TaskID, ClusterID: job.ClusterID, Name: jobDisplayName(job),
			Sources: sources, Sinks: sinks, Viewable: viewable(job),
		}
	}
	return out, nil
}

// JobRunCalendar 统计近 weeks 周（1~52）每日运行 / 失败次数，按显示时区分天，不含预览运行。
// JobRunCalendar counts daily runs / failures over the last weeks (1-52), bucketed by display-timezone day, excluding
// preview runs.
func (s *Service) JobRunCalendar(ctx context.Context, clusterID uint, weeks int, now time.Time) (*JobCalendar, error) {
	if weeks <= 0 {
		weeks = 16
	}
	if weeks > 52 {
		weeks = 52
	}
	loc := timex.DisplayLocation()
	local := now.In(loc)
	endDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	// 每列周一开头：第一列的周一 = 结束日所在周的周一往前 weeks-1 周。
	// Columns start on Monday: the first column's Monday is the end week's Monday minus weeks-1 weeks.
	firstMonday := endDay.AddDate(0, 0, -((int(endDay.Weekday())+6)%7)-(weeks-1)*7)
	out := &JobCalendar{Weeks: weeks, EndDate: endDay.Format("2006-01-02"), Runs: make([][]int, weeks), Failed: make([][]int, weeks)}
	for w := range out.Runs {
		out.Runs[w], out.Failed[w] = make([]int, 7), make([]int, 7)
	}
	rows, err := s.repo.ListJobTimesSince(ctx, firstMonday, clusterID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		at := row.CreatedAt
		if row.StartedAt != nil {
			at = *row.StartedAt
		}
		t := at.In(loc)
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		// 按日历日相减，避开夏令时让 24 小时不等于一天。/ Subtract calendar days so DST never breaks a day.
		index := int(day.Sub(firstMonday).Hours()/24 + 0.5)
		if index < 0 || index >= weeks*7 {
			continue
		}
		out.Runs[index/7][index%7]++
		if jobStatusGroupOf(row.Status) == JobStatusGroupFailed {
			out.Failed[index/7][index%7]++
		}
	}
	return out, nil
}

// JobAnnotationsBetween 返回时间窗内的事件标注：失败的作业（按结束时间）、恢复运行（按开始时间）、表结构变更事件。
// JobAnnotationsBetween returns markers in the window: failed jobs (by finish time), recover runs (by start time) and
// schema change events.
func (s *Service) JobAnnotationsBetween(ctx context.Context, actor executionapp.Actor, clusterID uint, start, end time.Time) ([]JobAnnotation, error) {
	scope := &JobFilter{}
	if err := s.scopeJobFilterForActor(ctx, actor, scope); err != nil {
		return nil, err
	}
	viewable := jobViewableFunc(actor, scope)
	failedStatuses, _ := ResolveJobStatusGroups(JobStatusGroupFailed)
	failed, _, err := s.ListJobs(ctx, &JobFilter{
		IncludeAll: true, ClusterID: clusterID, Statuses: failedStatuses, FinishedAfter: &start, FinishedBefore: &end,
		ExcludeRunTypes: []RunType{RunTypePreview}, Size: dashboardAnnotationLimit, Sort: jobListSortStartedAt,
	})
	if err != nil {
		return nil, err
	}
	recovered, err := s.repo.ListRecoverJobsStartedBetween(ctx, start, end, clusterID, dashboardAnnotationLimit)
	if err != nil {
		return nil, err
	}
	if err := s.decorateJobsForList(ctx, append(append([]*JobInstance{}, failed...), recovered...)); err != nil {
		return nil, err
	}
	out := make([]JobAnnotation, 0, len(failed)+len(recovered))
	for _, job := range failed {
		if job.FinishedAt == nil {
			continue
		}
		out = append(out, JobAnnotation{TsMs: job.FinishedAt.UnixMilli(), Kind: "job_failed", Title: jobDisplayName(job),
			JobID: job.ID, TaskID: job.TaskID, Viewable: viewable(job)})
	}
	for _, job := range recovered {
		out = append(out, JobAnnotation{TsMs: job.StartedAt.UnixMilli(), Kind: "job_recovered", Title: jobDisplayName(job),
			JobID: job.ID, TaskID: job.TaskID, Viewable: viewable(job)})
	}
	events, err := s.repo.ListSchemaChangeEventsBetween(ctx, start, end, clusterID, dashboardAnnotationLimit)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		item := JobAnnotation{TsMs: event.EventTimeMs, Kind: "schema_change", Title: event.EventType}
		if event.JobInstanceID != nil {
			item.JobID = *event.JobInstanceID
			item.Title = fmt.Sprintf("%s · #%d", event.EventType, *event.JobInstanceID)
		}
		out = append(out, item)
	}
	return out, nil
}

// ResolveTaskNamesForActor 批量解析任务名，受当前用户可见范围约束：不可见或不存在的任务不进结果，
// 调用方回退为「#id」占位，避免经大盘拆分序列泄露任务名。
// ResolveTaskNamesForActor resolves task names in bulk scoped to the actor's visibility: tasks the actor
// cannot view (or that do not exist) are absent, so callers fall back to the "#id" placeholder instead of
// leaking task names through dashboard split series.
func (s *Service) ResolveTaskNamesForActor(ctx context.Context, actor executionapp.Actor, taskIDs []uint) map[uint]string {
	names := map[uint]string{}
	if s == nil || s.repo == nil {
		return names
	}
	for _, id := range taskIDs {
		if _, done := names[id]; done || id == 0 {
			continue
		}
		task, err := s.repo.GetTaskByID(ctx, id)
		if err != nil || task == nil || !task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
			continue
		}
		names[id] = strings.TrimSpace(task.Name)
	}
	return names
}
