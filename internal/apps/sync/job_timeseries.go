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
	"strconv"
	"strings"
	gosync "sync"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// JobTimeseriesResult 是 GET /sync/jobs/:id/timeseries 的返回；无本地兜底，source 仅为 prometheus 或 none。
// JobTimeseriesResult is the payload of GET /sync/jobs/:id/timeseries; there is no local fallback, so source is only
// "prometheus" or "none".
type JobTimeseriesResult struct {
	Source                     string                    `json:"source"`
	Reason                     string                    `json:"reason,omitempty"`
	Range                      string                    `json:"range"`
	StartMs                    int64                     `json:"start_ms"`
	EndMs                      int64                     `json:"end_ms"`
	StepSeconds                int                       `json:"step_seconds"`
	EngineObservabilityEnabled bool                      `json:"engine_observability_enabled"`
	Series                     []JobTimeseriesSeries     `json:"series"`
	Annotations                []JobTimeseriesAnnotation `json:"annotations"`
}

// JobTimeseriesSeries 是一条命名时序。
// JobTimeseriesSeries is one named time series.
type JobTimeseriesSeries struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Unit   string       `json:"unit"`
	Points [][2]float64 `json:"points"` // [ts_ms, value]
}

// JobTimeseriesAnnotation 是叠加在图上的事件标记。
// JobTimeseriesAnnotation is an event marker overlaid on the charts.
type JobTimeseriesAnnotation struct {
	TsMs    int64  `json:"ts_ms"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	EventID uint   `json:"event_id,omitempty"`
}

// 时间窗工具已迁到 promquery，作业趋势与首页集群大盘共用；这里保留旧名供本包与测试使用。
// The window helpers moved to promquery so job trends and the home cluster dashboard share them; the old names stay
// for this package and its tests.
var (
	timeseriesRangeSteps     = promquery.RangeSteps
	timeseriesRangeDurations = promquery.RangeDurations
)

const timeseriesCustomRange = promquery.RangeCustom

// JobTimeseriesWindow 是自定义的起止时间窗（UTC）。
// JobTimeseriesWindow is a custom start/end window (UTC).
type JobTimeseriesWindow = promquery.Window

// ParseJobTimeseriesWindow 见 promquery.ParseWindow。
// ParseJobTimeseriesWindow: see promquery.ParseWindow.
func ParseJobTimeseriesWindow(startRaw, endRaw string, now time.Time) (*JobTimeseriesWindow, error) {
	return promquery.ParseWindow(startRaw, endRaw, now)
}

// autoTimeseriesStep 见 promquery.AutoStep。
// autoTimeseriesStep: see promquery.AutoStep.
func autoTimeseriesStep(span time.Duration) int {
	return promquery.AutoStep(span)
}

// GetJobTimeseriesForActor 在权限检查后返回作业时序（仅 Prometheus）。window 非空时优先于 rangeKey。
// GetJobTimeseriesForActor returns job timeseries after enforcing permission (Prometheus only). A non-nil window wins
// over rangeKey.
func (s *Service) GetJobTimeseriesForActor(ctx context.Context, actor executionapp.Actor, id uint, rangeKey string, window *JobTimeseriesWindow, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	instance, err := s.getJobInstanceForActor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return s.getJobTimeseries(ctx, instance, rangeKey, window, metricsCSV, step)
}

func (s *Service) getJobTimeseries(ctx context.Context, instance *JobInstance, rangeKey string, window *JobTimeseriesWindow, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	var start, end time.Time
	if window != nil {
		// 自定义时间窗：作业全程或日历选择的起止时刻。
		// Custom window: the job's lifetime or a range picked on the calendar.
		rangeKey = timeseriesCustomRange
		start, end = window.Start.UTC(), window.End.UTC()
		if step <= 0 {
			step = autoTimeseriesStep(end.Sub(start))
		}
	} else {
		rangeKey = strings.ToLower(strings.TrimSpace(rangeKey))
		if rangeKey == "" {
			rangeKey = "1h"
		}
		dur, ok := timeseriesRangeDurations[rangeKey]
		if !ok {
			return nil, fmt.Errorf("unsupported range %q (want 15m|1h|6h|24h|7d|30d|90d|1y, or start/end)", rangeKey)
		}
		if step <= 0 {
			step = timeseriesRangeSteps[rangeKey]
		}
		end = time.Now().UTC()
		start = end.Add(-dur)
	}
	engineObs := jobHasEngineObservability(instance)
	out := &JobTimeseriesResult{
		Range:                      rangeKey,
		StartMs:                    start.UnixMilli(),
		EndMs:                      end.UnixMilli(),
		StepSeconds:                step,
		EngineObservabilityEnabled: engineObs,
		Series:                     []JobTimeseriesSeries{},
		Annotations:                []JobTimeseriesAnnotation{},
	}

	if reason := promquery.UnavailableReason(); reason != "" {
		out.Source = promquery.SourceNone
		out.Reason = reason
		return out, nil
	}

	jobID := strconv.FormatUint(uint64(instance.ID), 10)
	metricKeys := parseTimeseriesMetrics(metricsCSV)
	type tsQuery struct {
		tpl     promQLTemplate
		vectors []promquery.Series
	}
	var queries []tsQuery
	for _, key := range metricKeys {
		for _, tpl := range timeseriesPromQL(key, jobID, engineObs, step) {
			queries = append(queries, tsQuery{tpl: tpl})
		}
	}
	// 默认一次趋势请求会打十几条 PromQL；并发跑，引擎内部仍受 maxConcurrentQueries 排队。
	// A default timeseries request fires a dozen PromQL queries; run them concurrently, the engine
	// still queues behind maxConcurrentQueries.
	var wg gosync.WaitGroup
	for i := range queries {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vectors, err := promquery.Range(ctx, queries[i].tpl.query, start, end, step)
			if err != nil {
				// 单条模板失败不拖垮整份响应。
				// Soft-fail per series so a single template error does not empty the whole response.
				return
			}
			queries[i].vectors = vectors
		}(i)
	}
	wg.Wait()
	for _, item := range queries {
		for _, vec := range item.vectors {
			label := item.tpl.label
			if name := strings.TrimSpace(vec.Metric["vertex"]); name != "" {
				label = name
			}
			if name := strings.TrimSpace(vec.Metric["edge"]); name != "" {
				label = name
			}
			if name := strings.TrimSpace(vec.Metric["table"]); name != "" {
				label = name
			}
			out.Series = append(out.Series, JobTimeseriesSeries{
				Key:    item.tpl.key,
				Label:  label,
				Unit:   item.tpl.unit,
				Points: vec.Points,
			})
		}
	}

	out.Source = promquery.SourcePrometheus
	out.Annotations = s.loadTimeseriesAnnotations(ctx, instance, start, end)
	return out, nil
}

type promQLTemplate struct {
	key   string
	label string
	unit  string
	query string
}

// timeseriesPromQL 生成一个指标组的查询模板；速率窗口取 max(1m, step)，仪表序列在步长超过
// 5 分钟时包 avg_over_time（忙碌度/背压）或 max_over_time（CDC 延迟、checkpoint 耗时），
// 长窗口下每个取样点覆盖整个取样间隔。
// timeseriesPromQL builds the query templates for one metric group; rate windows use
// max(1m, step) and gauge series wrap avg_over_time (busy/backpressure) or max_over_time
// (CDC delay, checkpoint duration) once the step exceeds five minutes, so long windows sample
// every point across its whole interval.
func timeseriesPromQL(metricKey, jobID string, engineObs bool, step int) []promQLTemplate {
	id := strconv.Quote(jobID)
	rateWindow := promquery.RangeWindow(step, time.Minute)
	avg := func(expr string) string { return promquery.SampleGauge(step, "avg", expr) }
	maxg := func(expr string) string { return promquery.SampleGauge(step, "max", expr) }
	switch metricKey {
	case "throughput":
		return []promQLTemplate{
			{key: "throughput_read", label: "read", unit: "records/s",
				query: fmt.Sprintf(`sum(rate(stx_job_source_received_rows_total{job_id=%s}[%s]))`, id, rateWindow)},
			{key: "throughput_write", label: "write", unit: "records/s",
				query: fmt.Sprintf(`sum(rate(stx_job_sink_write_rows_total{job_id=%s}[%s]))`, id, rateWindow)},
		}
	case "rows":
		return []promQLTemplate{
			{key: "rows_read", label: "read", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_source_received_rows_total{job_id=%s})`, id)},
			{key: "rows_write", label: "write", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_sink_write_rows_total{job_id=%s})`, id)},
			{key: "rows_committed", label: "committed", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_sink_committed_rows_total{job_id=%s})`, id)},
		}
	case "bytes_throughput":
		return []promQLTemplate{
			{key: "bytes_throughput_read", label: "read", unit: "bytes/s",
				query: fmt.Sprintf(`sum(rate(stx_job_source_received_bytes_total{job_id=%s}[%s]))`, id, rateWindow)},
			{key: "bytes_throughput_write", label: "write", unit: "bytes/s",
				query: fmt.Sprintf(`sum(rate(stx_job_sink_write_bytes_total{job_id=%s}[%s]))`, id, rateWindow)},
		}
	case "bytes":
		return []promQLTemplate{
			{key: "bytes_read", label: "read", unit: "bytes",
				query: fmt.Sprintf(`sum(stx_job_source_received_bytes_total{job_id=%s})`, id)},
			{key: "bytes_write", label: "write", unit: "bytes",
				query: fmt.Sprintf(`sum(stx_job_sink_write_bytes_total{job_id=%s})`, id)},
		}
	case "cdc_delay":
		return []promQLTemplate{{
			key: "cdc_delay", label: "cdc_delay", unit: "seconds",
			query: fmt.Sprintf(`max(%s)`, maxg(fmt.Sprintf(`stx_job_cdc_delay_seconds{job_id=%s}`, id))),
		}}
	case "checkpoint":
		return []promQLTemplate{
			{key: "checkpoint_duration", label: "duration", unit: "seconds",
				query: fmt.Sprintf(`max(%s)`, maxg(fmt.Sprintf(`stx_job_checkpoint_duration_seconds{job_id=%s}`, id)))},
			{key: "checkpoint_failed", label: "failed", unit: "count",
				query: fmt.Sprintf(`sum(stx_job_checkpoint_failed_total{job_id=%s})`, id)},
		}
	case "table":
		// 表级指标来自作业指标采集（TableSourceReceivedCount/TableSinkWriteCount），与引擎可观测性无关，
		// 不受 engineObs 约束；表名在 table 标签上，直接作为 series label 返回。导出端对缺失方向补 0，
		// 会产生 Source[N]·写 / Sink[N]·读这类恒零镜像序列——按"该方向累计 > 0"过滤掉；吞吐序列用 and
		// 挂同一过滤，这样空闲的真实表仍显示为 0 而不消失。
		// Per-table metrics come from job metrics collection (TableSourceReceivedCount/TableSinkWriteCount),
		// independent of engine observability; the table label becomes the series label. The exporter fills
		// the missing direction with 0, which produces always-zero mirror series (Source[N]·write /
		// Sink[N]·read) — filtered by "cumulative > 0 on that direction"; throughput carries the same filter
		// via `and` so an idle real table still renders as 0 instead of disappearing.
		return []promQLTemplate{
			{key: "table_throughput_read", label: "read", unit: "records/s",
				query: fmt.Sprintf(`sum by (table) (rate(stx_job_table_rows_total{job_id=%s,direction="read"}[%s])) and on(table) (sum by (table) (stx_job_table_rows_total{job_id=%s,direction="read"}) > 0)`, id, rateWindow, id)},
			{key: "table_throughput_write", label: "write", unit: "records/s",
				query: fmt.Sprintf(`sum by (table) (rate(stx_job_table_rows_total{job_id=%s,direction="write"}[%s])) and on(table) (sum by (table) (stx_job_table_rows_total{job_id=%s,direction="write"}) > 0)`, id, rateWindow, id)},
			{key: "table_rows_read", label: "read", unit: "rows",
				query: fmt.Sprintf(`sum by (table) (stx_job_table_rows_total{job_id=%s,direction="read"}) > 0`, id)},
			{key: "table_rows_write", label: "write", unit: "rows",
				query: fmt.Sprintf(`sum by (table) (stx_job_table_rows_total{job_id=%s,direction="write"}) > 0`, id)},
		}
	case "busy":
		if !engineObs {
			return nil
		}
		return []promQLTemplate{{
			key: "busy", label: "busy", unit: "ratio",
			query: avg(fmt.Sprintf(`stx_job_vertex_busy_ratio{job_id=%s}`, id)),
		}}
	case "backpressure":
		if !engineObs {
			return nil
		}
		return []promQLTemplate{{
			key: "backpressure", label: "backpressure", unit: "ratio",
			query: avg(fmt.Sprintf(`stx_job_edge_backpressure_ratio{job_id=%s}`, id)),
		}}
	default:
		return nil
	}
}

func parseTimeseriesMetrics(csv string) []string {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return []string{"throughput", "rows", "bytes_throughput", "bytes", "cdc_delay", "checkpoint", "busy", "backpressure", "table"}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (s *Service) loadTimeseriesAnnotations(ctx context.Context, instance *JobInstance, start, end time.Time) []JobTimeseriesAnnotation {
	if s == nil || s.repo == nil || instance == nil {
		return []JobTimeseriesAnnotation{}
	}
	// Reuse event listing with a generous page; filter by time client-side for simplicity.
	// 复用事件列表接口，页容量放大，时间过滤放在内存侧以保持实现简单。
	events, _, err := s.repo.ListEngineEventsByJob(ctx, instance.ID, nil, 1, 500)
	if err != nil {
		return []JobTimeseriesAnnotation{}
	}
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	out := make([]JobTimeseriesAnnotation, 0, 8)
	for _, ev := range events {
		if ev == nil {
			continue
		}
		ts := ev.EventTimeMs
		if ts < startMs || ts > endMs {
			continue
		}
		kind := strings.TrimSpace(ev.EventType)
		switch {
		case kind == EngineEventTypeJobStatus:
			kind = "status"
		case strings.HasPrefix(kind, "SCHEMA_CHANGE"):
			kind = "schema_change"
		case strings.Contains(kind, "LIFECYCLE") || strings.Contains(strings.ToLower(kind), "restart"):
			kind = "restart"
		case kind == EngineEventTypeMessageDelayed:
			continue // CDC delay is a series, not an annotation
		default:
			// keep original kind for other lifecycle events
		}
		summary := strings.TrimSpace(ev.EventType)
		if summary == "" {
			summary = kind
		}
		out = append(out, JobTimeseriesAnnotation{
			TsMs:    ts,
			Kind:    kind,
			Summary: summary,
			EventID: ev.ID,
		})
	}
	return out
}
